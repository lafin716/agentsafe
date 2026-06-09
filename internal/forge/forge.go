// Package forge abstracts git hosting providers (GitHub, GitLab) so the rest of
// agentsafe can create pull/merge requests without caring which host a given
// repository lives on. The provider is detected from a repository's clone URL.
package forge

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Kind identifies a hosting provider. Empty means unknown/unsupported.
type Kind string

const (
	GitHub  Kind = "github"
	GitLab  Kind = "gitlab"
	Unknown Kind = ""
)

// Detect infers the provider from a repository clone URL's host.
func Detect(repoURL string) Kind {
	host, _, _, err := ParseHostOwnerRepo(repoURL)
	if err != nil {
		// Fall back to a substring check on the raw URL.
		host = strings.ToLower(repoURL)
	}
	host = strings.ToLower(host)
	switch {
	case strings.Contains(host, "github"):
		return GitHub
	case strings.Contains(host, "gitlab"):
		return GitLab
	default:
		return Unknown
	}
}

// ParseHostOwnerRepo splits a clone URL into host and "owner/repo" path.
// It accepts https URLs and scp-style SSH URLs (git@host:owner/repo.git).
// For GitLab subgroups the owner retains the full path (group/sub/repo -> owner
// "group/sub", repo "repo").
func ParseHostOwnerRepo(repoURL string) (host, owner, repo string, err error) {
	raw := strings.TrimSpace(repoURL)
	var path string
	if strings.HasPrefix(raw, "git@") || (!strings.Contains(raw, "://") && strings.Contains(raw, ":")) {
		// scp-style: [user@]host:path
		at := strings.Index(raw, "@")
		rest := raw
		if at >= 0 {
			rest = raw[at+1:]
		}
		colon := strings.Index(rest, ":")
		if colon < 0 {
			return "", "", "", fmt.Errorf("invalid ssh url %q", repoURL)
		}
		host = rest[:colon]
		path = rest[colon+1:]
	} else {
		u, e := url.Parse(raw)
		if e != nil || u.Host == "" {
			return "", "", "", fmt.Errorf("invalid url %q", repoURL)
		}
		host = u.Host
		path = u.Path
	}
	path = strings.TrimPrefix(path, "/")
	path = strings.TrimSuffix(path, ".git")
	path = strings.TrimSuffix(path, "/")
	if path == "" || !strings.Contains(path, "/") {
		return "", "", "", fmt.Errorf("cannot derive owner/repo from %q", repoURL)
	}
	idx := strings.LastIndex(path, "/")
	owner = path[:idx]
	repo = path[idx+1:]
	if owner == "" || repo == "" {
		return "", "", "", fmt.Errorf("cannot derive owner/repo from %q", repoURL)
	}
	return host, owner, repo, nil
}

// webBase returns the https web base URL for the project, e.g.
// https://github.com/acme/backend (no trailing slash).
func webBase(host, owner, repo string) string {
	return "https://" + host + "/" + owner + "/" + repo
}

// NewRequestURL builds the provider's "create PR/MR" web page URL with the
// branches and title pre-filled, for the browser fallback flow.
func NewRequestURL(repoURL, source, target, title string) (string, error) {
	host, owner, repo, err := ParseHostOwnerRepo(repoURL)
	if err != nil {
		return "", err
	}
	base := webBase(host, owner, repo)
	switch Detect(repoURL) {
	case GitHub:
		q := url.Values{}
		q.Set("expand", "1")
		if title != "" {
			q.Set("title", title)
		}
		return fmt.Sprintf("%s/compare/%s...%s?%s",
			base, url.PathEscape(target), url.PathEscape(source), q.Encode()), nil
	case GitLab:
		q := url.Values{}
		q.Set("merge_request[source_branch]", source)
		q.Set("merge_request[target_branch]", target)
		if title != "" {
			q.Set("merge_request[title]", title)
		}
		return base + "/-/merge_requests/new?" + q.Encode(), nil
	default:
		return "", fmt.Errorf("unknown provider for %q", repoURL)
	}
}

// CreateOptions holds everything needed to create a request via API.
type CreateOptions struct {
	RepoURL string
	Source  string // head branch
	Target  string // base branch
	Title   string
	Body    string
	Token   string
	// APIBaseURL optionally overrides the API endpoint (for self-hosted GitLab:
	// the instance base URL; for GHE it is derived from the host).
	APIBaseURL string
}

// CreateResult is the outcome of a successful API creation.
type CreateResult struct {
	URL string // web URL of the created PR/MR
}

var httpClient = &http.Client{Timeout: 30 * time.Second}

// Create opens a pull/merge request through the provider's REST API.
func Create(kind Kind, opt CreateOptions) (CreateResult, error) {
	switch kind {
	case GitHub:
		return createGitHub(opt)
	case GitLab:
		return createGitLab(opt)
	default:
		return CreateResult{}, fmt.Errorf("unknown provider")
	}
}

func createGitHub(opt CreateOptions) (CreateResult, error) {
	host, owner, repo, err := ParseHostOwnerRepo(opt.RepoURL)
	if err != nil {
		return CreateResult{}, err
	}
	apiBase := "https://api.github.com"
	if host != "github.com" && !strings.HasSuffix(host, ".github.com") {
		apiBase = "https://" + host + "/api/v3" // GitHub Enterprise
	}
	endpoint := fmt.Sprintf("%s/repos/%s/%s/pulls", apiBase, owner, repo)
	payload := map[string]string{
		"title": opt.Title,
		"head":  opt.Source,
		"base":  opt.Target,
		"body":  opt.Body,
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return CreateResult{}, err
	}
	req.Header.Set("Authorization", "Bearer "+opt.Token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return CreateResult{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusCreated {
		var out struct {
			HTMLURL string `json:"html_url"`
		}
		_ = json.Unmarshal(raw, &out)
		return CreateResult{URL: out.HTMLURL}, nil
	}
	return CreateResult{}, fmt.Errorf("github: %s", apiErrorMessage(raw, resp.Status))
}

func createGitLab(opt CreateOptions) (CreateResult, error) {
	host, owner, repo, err := ParseHostOwnerRepo(opt.RepoURL)
	if err != nil {
		return CreateResult{}, err
	}
	apiBase := strings.TrimRight(opt.APIBaseURL, "/")
	if apiBase == "" {
		apiBase = "https://" + host
	}
	projectPath := url.PathEscape(owner + "/" + repo) // encodes the slash too
	endpoint := fmt.Sprintf("%s/api/v4/projects/%s/merge_requests", apiBase, projectPath)
	payload := map[string]string{
		"source_branch": opt.Source,
		"target_branch": opt.Target,
		"title":         opt.Title,
		"description":   opt.Body,
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return CreateResult{}, err
	}
	req.Header.Set("PRIVATE-TOKEN", opt.Token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return CreateResult{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusOK {
		var out struct {
			WebURL string `json:"web_url"`
		}
		_ = json.Unmarshal(raw, &out)
		return CreateResult{URL: out.WebURL}, nil
	}
	return CreateResult{}, fmt.Errorf("gitlab: %s", apiErrorMessage(raw, resp.Status))
}

// apiErrorMessage extracts a human-readable message from a provider error body.
func apiErrorMessage(raw []byte, status string) string {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err == nil {
		if msg, ok := m["message"]; ok {
			// GitHub returns {message, errors:[{message}]}; GitLab uses message too.
			if errs, ok := m["errors"].([]any); ok && len(errs) > 0 {
				if first, ok := errs[0].(map[string]any); ok {
					if em, ok := first["message"].(string); ok && em != "" {
						return fmt.Sprintf("%v (%v)", msg, em)
					}
				}
			}
			return fmt.Sprintf("%v", msg)
		}
		// GitLab validation errors come back as {message:{base:[...]}} or {error:...}.
		if e, ok := m["error"]; ok {
			return fmt.Sprintf("%v", e)
		}
	}
	if len(raw) > 0 {
		return status + ": " + strings.TrimSpace(string(raw))
	}
	return status
}
