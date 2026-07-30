package git

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Result struct {
	Command string
	Dir     string
	Stdout  string
	Stderr  string
}

type FileStatus struct {
	Code string
	Type string
	Path string
}

type Error struct {
	Result  Result
	Err     error
	Timeout bool
}

func (e *Error) Error() string {
	if e.Timeout {
		return fmt.Sprintf("git command timed out: %s\nReason: command exceeded timeout\nstdout: %s\nstderr: %s\nSuggestion: check Git authentication/network, then retry. agentsafe runs Git non-interactively to avoid hanging.", e.Result.Command, e.Result.Stdout, e.Result.Stderr)
	}
	return fmt.Sprintf("git command failed: %s\nReason: %v\nstdout: %s\nstderr: %s", e.Result.Command, e.Err, e.Result.Stdout, e.Result.Stderr)
}

func timeout() time.Duration {
	seconds := 120
	if raw := os.Getenv("AGENTSAFE_GIT_TIMEOUT_SECONDS"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			seconds = n
		}
	}
	return time.Duration(seconds) * time.Second
}

func Run(dir string, args ...string) (Result, error) {
	return run(dir, nil, args...)
}

// RunWithHTTPAuth runs Git with a command-scoped HTTP Basic authorization
// header. The secret is passed only through the child environment and is never
// included in Result.Command, stdout, stderr, or task logs.
func RunWithHTTPAuth(dir, remoteURL, username, secret string, args ...string) (Result, error) {
	scope := "http"
	if u, err := url.Parse(remoteURL); err == nil && u.Scheme != "" && u.Host != "" {
		scope = "http." + u.Scheme + "://" + u.Host + "/"
	}
	value := "Authorization: Basic " + base64.StdEncoding.EncodeToString([]byte(username+":"+secret))
	return run(dir, []string{
		"GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=" + scope + ".extraHeader",
		"GIT_CONFIG_VALUE_0=" + value,
	}, args...)
}

func run(dir string, extraEnv []string, args ...string) (Result, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout())
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		"GCM_INTERACTIVE=never",
	)
	cmd.Env = append(cmd.Env, extraEnv...)
	hideWindow(cmd) // suppress the per-process console window on Windows GUI apps
	var out, er bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &er
	err := cmd.Run()
	res := Result{Command: "git " + strings.Join(args, " "), Dir: dir, Stdout: out.String(), Stderr: er.String()}
	if err != nil {
		return res, &Error{Result: res, Err: err, Timeout: ctx.Err() == context.DeadlineExceeded}
	}
	return res, nil
}

// IsAuthenticationError reports whether Git failed because an HTTPS remote
// requires or rejected credentials.
func IsAuthenticationError(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	markers := []string{
		"authentication failed",
		"could not read username",
		"could not read password",
		"terminal prompts disabled",
		"invalid username or password",
		"access denied",
		"http 401",
		"http 403",
		"returned error: 401",
		"returned error: 403",
	}
	for _, marker := range markers {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func Output(dir string, args ...string) (string, error) {
	r, err := Run(dir, args...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(r.Stdout), nil
}

func Fetch(repoPath string) error { _, err := Run(repoPath, "fetch", "origin"); return err }

// NormalizeBranchName strips ref-namespace and "origin/" prefixes from a branch
// reference, returning the bare branch name. Repeated "origin/" prefixes are
// collapsed so a malformed value like "origin/origin/main" becomes "main" — this
// is what keeps a stray "origin/"-prefixed input from producing phantom
// refs/remotes/origin/origin… refs.
func NormalizeBranchName(ref string) string {
	b := strings.TrimSpace(ref)
	b = strings.TrimPrefix(b, "refs/remotes/")
	b = strings.TrimPrefix(b, "refs/heads/")
	for strings.HasPrefix(b, "origin/") {
		b = strings.TrimPrefix(b, "origin/")
	}
	return b
}

// FetchBranch fetches just one branch from origin (updating FETCH_HEAD and the
// matching origin/<branch> ref). Cheaper than a full fetch when only the base
// branch is needed.
func FetchBranch(repoPath, branch string) error {
	_, err := Run(repoPath, "fetch", "origin", NormalizeBranchName(branch))
	return err
}
func FetchAll(repoPath string) error {
	_, err := Run(repoPath, "fetch", "--all", "--prune")
	return err
}
func Checkout(repoPath, branch string) error { _, err := Run(repoPath, "checkout", branch); return err }
func ListRemoteBranches(repoPath string) ([]string, error) {
	out, err := Output(repoPath, "for-each-ref", "--format=%(refname:short)", "refs/remotes/origin")
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var branches []string
	for _, line := range strings.Split(strings.ReplaceAll(out, "\r\n", "\n"), "\n") {
		branch := NormalizeBranchName(line)
		// Skip the symbolic HEAD ref and any phantom "origin/origin…" refs that a
		// malformed fetch may have left behind locally.
		if branch == "" || branch == "HEAD" || branch == "origin" || seen[branch] {
			continue
		}
		seen[branch] = true
		branches = append(branches, branch)
	}
	return branches, nil
}

// PruneStaleOriginRefs deletes malformed refs/remotes/origin/origin… refs that a
// previous buggy fetch may have created locally. They never correspond to a real
// remote branch and otherwise surface as a phantom "origin/origin" entry that
// fails to check out. Best-effort: callers can ignore the error.
func PruneStaleOriginRefs(repoPath string) error {
	out, err := Output(repoPath, "for-each-ref", "--format=%(refname)", "refs/remotes/origin/origin")
	if err != nil {
		return err
	}
	for _, line := range strings.Split(strings.ReplaceAll(out, "\r\n", "\n"), "\n") {
		ref := strings.TrimSpace(line)
		if ref == "" {
			continue
		}
		if _, err := Run(repoPath, "update-ref", "-d", ref); err != nil {
			return err
		}
	}
	return nil
}
func CheckoutRemoteBranch(repoPath, remoteBranch string) error {
	branch := NormalizeBranchName(remoteBranch)
	if branch == "" || branch == "HEAD" {
		return fmt.Errorf("invalid remote branch %q", remoteBranch)
	}
	if _, err := Run(repoPath, "check-ref-format", "--branch", branch); err != nil {
		return err
	}
	if !RemoteBranchExists(repoPath, branch) {
		if err := FetchAll(repoPath); err != nil {
			return err
		}
	}
	if !RemoteBranchExists(repoPath, branch) {
		return fmt.Errorf("remote branch origin/%s not found", branch)
	}
	if LocalBranchExists(repoPath, branch) {
		return Checkout(repoPath, branch)
	}
	_, err := Run(repoPath, "checkout", "-b", branch, "--track", "origin/"+branch)
	return err
}
func Pull(repoPath, remote, branch string) error {
	_, err := Run(repoPath, "pull", "--ff-only", remote, branch)
	return err
}
func LocalBranchExists(repoPath, branch string) bool {
	_, err := Run(repoPath, "rev-parse", "--verify", "refs/heads/"+branch)
	return err == nil
}
func RemoteBranchExists(repoPath, branch string) bool {
	_, err := Run(repoPath, "rev-parse", "--verify", "refs/remotes/origin/"+branch)
	return err == nil
}
func RemoteBranchExistsAtOrigin(repoPath, branch string) bool {
	_, err := Run(repoPath, "ls-remote", "--exit-code", "--heads", "origin", "refs/heads/"+branch)
	return err == nil
}
func StatusShort(path string) (string, error)   { return Output(path, "status", "--short") }
func CurrentBranch(path string) (string, error) { return Output(path, "branch", "--show-current") }
func HasChanges(path string) bool               { s, err := StatusShort(path); return err == nil && s != "" }
func Upstream(path, branch string) (string, error) {
	return Output(path, "rev-parse", "--abbrev-ref", "--symbolic-full-name", branch+"@{upstream}")
}
func SetUpstream(path, branch, upstream string) error {
	_, err := Run(path, "branch", "--set-upstream-to", upstream, branch)
	return err
}

// StatusFiles returns both the porcelain output and a structured representation
// for UI consumers. Unlike Output, it preserves the leading status columns.
func StatusFiles(path string) (string, []FileStatus, error) {
	r, err := Run(path, "status", "--porcelain=v1")
	if err != nil {
		return "", nil, err
	}
	raw := strings.TrimRight(r.Stdout, "\r\n")
	return raw, ParseStatusPorcelain(raw), nil
}

// ParseStatusPorcelain parses `git status --porcelain=v1` output.
func ParseStatusPorcelain(raw string) []FileStatus {
	statuses := []FileStatus{}
	for _, line := range strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n") {
		if len(line) < 3 {
			continue
		}
		code := line[:2]
		path := strings.TrimSpace(line[3:])
		if i := strings.LastIndex(path, " -> "); i >= 0 {
			path = path[i+4:]
		}
		statuses = append(statuses, FileStatus{
			Code: code,
			Type: statusType(code),
			Path: path,
		})
	}
	return statuses
}

func statusType(code string) string {
	switch {
	case code == "??":
		return "added"
	case strings.Contains(code, "U") ||
		code == "DD" || code == "AU" || code == "UD" || code == "UA" ||
		code == "DU" || code == "AA":
		return "conflict"
	case strings.Contains(code, "R"):
		return "renamed"
	case strings.Contains(code, "D"):
		return "deleted"
	case strings.Contains(code, "A"):
		return "added"
	case strings.Contains(code, "M") || strings.Contains(code, "T"):
		return "modified"
	default:
		return "other"
	}
}

func CommitAll(path, message string) error {
	if _, err := Run(path, "add", "."); err != nil {
		return err
	}
	_, err := Run(path, "commit", "-m", message)
	return err
}
func Push(path, branch string) error { _, err := Run(path, "push", "-u", "origin", branch); return err }
func DeleteLocalBranch(repoPath, branch string) error {
	_, err := Run(repoPath, "branch", "-D", branch)
	return err
}
func RebaseOnto(worktreePath, upstream string) error {
	_, err := Run(worktreePath, "rebase", upstream)
	return err
}
func RebaseAbort(worktreePath string) error {
	_, err := Run(worktreePath, "rebase", "--abort")
	return err
}
func HeadSHA(path string) (string, error) { return Output(path, "rev-parse", "HEAD") }

// RevListCount returns the number of commits in the given range expression
// (e.g. "origin/main..HEAD"), used to detect how many commits are unpushed.
func RevListCount(path, rangeExpr string) (int, error) {
	out, err := Output(path, "rev-list", "--count", rangeExpr)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(out))
}
func RemoveWorktree(repoPath, dest string, force bool) error {
	args := []string{"worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	_, err := Run(repoPath, append(args, dest)...)
	return err
}
func WorktreePrune(repoPath string) error {
	_, err := Run(repoPath, "worktree", "prune")
	return err
}

// WorktreeForBranch returns the registered worktree path currently checking
// out branch, or an empty string when the branch is not in use.
func WorktreeForBranch(repoPath, branch string) string {
	out, err := Output(repoPath, "worktree", "list", "--porcelain")
	if err != nil {
		return ""
	}
	var path string
	for _, line := range strings.Split(strings.ReplaceAll(out, "\r\n", "\n"), "\n") {
		switch {
		case strings.HasPrefix(line, "worktree "):
			path = strings.TrimPrefix(line, "worktree ")
		case line == "branch refs/heads/"+branch:
			return path
		case line == "":
			path = ""
		}
	}
	return ""
}

// IsTracked reports whether path is tracked by the Git repository containing
// it. A path that lives outside any work tree is reported as untracked instead
// of an error, so callers can ask about arbitrary workspace paths. A directory
// counts as tracked when at least one tracked file lives beneath it.
func IsTracked(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	dir, pathspec := path, ""
	if !info.IsDir() {
		dir, pathspec = filepath.Dir(path), filepath.Base(path)
	}
	// A non-work-tree directory (no repository, or a bare one) has nothing tracked.
	if inside, err := Output(dir, "rev-parse", "--is-inside-work-tree"); err != nil || inside != "true" {
		return false, nil
	}
	// ls-files reads the index, so a staged-but-uncommitted file counts as
	// tracked. The global --literal-pathspecs stops names holding glob
	// characters (weird[1].txt) from matching anything but themselves, and -z
	// makes the output NUL-separated whatever core.quotePath says. Without a
	// pathspec git lists the whole subtree of dir, which answers the directory
	// case.
	args := []string{"--literal-pathspecs", "ls-files", "-z"}
	if pathspec != "" {
		args = append(args, "--", pathspec)
	}
	res, err := Run(dir, args...)
	if err != nil {
		return false, err
	}
	// Any listed name means tracked; only the separators are left to strip.
	return strings.Trim(res.Stdout, "\x00") != "", nil
}

func AddWorktree(repoPath, dest, branch, start string, create bool) error {
	if create {
		_, err := Run(repoPath, "worktree", "add", dest, "-b", branch, start)
		return err
	}
	_, err := Run(repoPath, "worktree", "add", dest, branch)
	return err
}
