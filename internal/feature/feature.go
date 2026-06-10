package feature

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/agentsafe/agentsafe/internal/config"
	aggit "github.com/agentsafe/agentsafe/internal/git"
	"github.com/agentsafe/agentsafe/internal/output"
	"github.com/agentsafe/agentsafe/internal/ui"
)

type Metadata struct {
	Name         string     `json:"name"`
	Branch       string     `json:"branch"`
	BaseBranch   string     `json:"baseBranch"`
	CreatedAt    string     `json:"createdAt"`
	Repositories []RepoMeta `json:"repositories"`
}
type RepoMeta struct {
	Name         string `json:"name"`
	WorktreePath string `json:"worktreePath"`
	Branch       string `json:"branch"`
	BaseBranch   string `json:"baseBranch"`
}

func BranchName(cfg config.Config, featureName string) string {
	return cfg.Git.BranchPrefix + featureName
}

func Load(root, name string) (Metadata, error) {
	b, err := os.ReadFile(config.FeatureMetaPath(root, name))
	if err != nil {
		return Metadata{}, err
	}
	var m Metadata
	return m, json.Unmarshal(b, &m)
}

func Save(root string, m Metadata) error {
	if err := os.MkdirAll(filepath.Dir(config.FeatureMetaPath(root, m.Name)), 0755); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(m, "", "  ")
	return os.WriteFile(config.FeatureMetaPath(root, m.Name), b, 0644)
}

func Create(root string, cfg config.Config, name, base string, force bool) error {
	if err := config.ValidateFeatureName(name); err != nil {
		return err
	}
	branch := BranchName(cfg, name)
	meta := Metadata{Name: name, Branch: branch, BaseBranch: base, CreatedAt: time.Now().Format(time.RFC3339)}
	for _, r := range cfg.Repositories {
		repoPath := config.RepoPath(root, r.Name)
		dest := config.WorktreePath(root, name, r.Name)
		rel, _ := filepath.Rel(root, dest)
		output.Printf("[%s] creating worktree %s\n", r.Name, rel)
		if _, err := os.Stat(repoPath); err != nil {
			return fmt.Errorf("repository %s is not cloned at %s; run `agentsafe pull`", r.Name, repoPath)
		}

		// determine base for this repo: explicit flag > current branch
		repoBase := base
		if repoBase == "" {
			cur, err := aggit.CurrentBranch(repoPath)
			if err != nil || cur == "" {
				return fmt.Errorf("repository %s is in detached HEAD state; use --base to specify a branch", r.Name)
			}
			repoBase = cur
		}

		// Fetch only the base branch and create the worktree directly from the
		// freshly fetched tip (FETCH_HEAD). This avoids a full fetch and the
		// extra checkout that `pull` performs on the main clone; the main clone
		// itself is kept current by `agentsafe pull`.
		start := repoBase
		output.Printf("  fetch origin %s (non-interactive, timeout controlled by AGENTSAFE_GIT_TIMEOUT_SECONDS)...\n", repoBase)
		if err := aggit.FetchBranch(repoPath, repoBase); err != nil {
			output.Printf("  warning: fetch failed, using local %s: %v\n", repoBase, err)
		} else {
			start = "FETCH_HEAD"
		}

		if _, err := os.Stat(dest); err == nil {
			output.Println("  worktree already exists, skipping")
		} else {
			if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
				return err
			}
			local := aggit.LocalBranchExists(repoPath, branch)
			remote := aggit.RemoteBranchExists(repoPath, branch)

			if remote && !local {
				return fmt.Errorf("remote branch %s already exists; delete it manually and retry", branch)
			}
			if local && !force {
				return fmt.Errorf("branch %s already exists in repository %s; use -f to force recreate", branch, r.Name)
			}
			if local && force {
				output.Printf("  deleting existing local branch %s\n", branch)
				if err := aggit.DeleteLocalBranch(repoPath, branch); err != nil {
					return fmt.Errorf("failed to delete branch %s in repository %s: %w", branch, r.Name, err)
				}
			}

			output.Printf("  creating new branch %s from %s\n", branch, repoBase)
			if err := aggit.AddWorktree(repoPath, dest, branch, start, true); err != nil {
				return fmt.Errorf("failed to create worktree for repository %s: %w", r.Name, err)
			}
		}
		meta.Repositories = append(meta.Repositories, RepoMeta{
			Name:         r.Name,
			WorktreePath: filepath.ToSlash(rel),
			Branch:       branch,
			BaseBranch:   repoBase,
		})
	}
	return Save(root, meta)
}

// DeleteOptions controls feature deletion. DeleteBranch also removes the local
// feature branch in each repository; Force removes worktrees that have
// uncommitted changes (otherwise deletion is refused).
type DeleteOptions struct {
	DeleteBranch bool
	Force        bool
}

// Delete removes a feature's worktrees and all of its artifacts: the feature
// metadata, the agent workspace, session metadata, and sync history. When
// DeleteBranch is set, the local feature branch is removed from each repo too.
// Unless Force is set, deletion is refused if any worktree has uncommitted
// changes (nothing is removed in that case, to avoid a partial delete).
func Delete(root, name string, opt DeleteOptions) error {
	m, err := Load(root, name)
	if err != nil {
		return err
	}

	if !opt.Force {
		var dirty []string
		for _, r := range m.Repositories {
			dest := filepath.Join(root, filepath.FromSlash(r.WorktreePath))
			if st, e := os.Stat(dest); e == nil && st.IsDir() && aggit.HasChanges(dest) {
				dirty = append(dirty, r.Name)
			}
		}
		if len(dirty) > 0 {
			return fmt.Errorf("worktree(s) have uncommitted changes: %s; commit/stash or use force", strings.Join(dirty, ", "))
		}
	}

	for _, r := range m.Repositories {
		repoPath := config.RepoPath(root, r.Name)
		dest := filepath.Join(root, filepath.FromSlash(r.WorktreePath))
		output.Printf("[%s] removing worktree %s\n", r.Name, r.WorktreePath)
		if _, e := os.Stat(dest); e == nil {
			if err := aggit.RemoveWorktree(repoPath, dest, opt.Force); err != nil {
				return fmt.Errorf("[%s] failed to remove worktree: %w", r.Name, err)
			}
		} else {
			_ = aggit.WorktreePrune(repoPath)
			_ = os.RemoveAll(dest)
		}
		if opt.DeleteBranch {
			output.Printf("[%s] deleting local branch %s\n", r.Name, r.Branch)
			if err := aggit.DeleteLocalBranch(repoPath, r.Branch); err != nil {
				output.Printf("  warning: could not delete branch %s: %v\n", r.Branch, err)
			}
		}
	}

	// Clean up all feature artifacts (best-effort; missing paths are ignored).
	output.Printf("removing feature metadata and agent artifacts for %s\n", name)
	_ = os.RemoveAll(filepath.Join(root, "feature", name))
	_ = os.Remove(config.FeatureMetaPath(root, name))
	_ = os.RemoveAll(filepath.Join(root, "agent", name))
	_ = os.Remove(config.SessionMetaPath(root, name))
	_ = os.RemoveAll(filepath.Join(config.HistoryDir(root), name))
	return nil
}

type FeatureListResult struct {
	Features []FeatureEntry `json:"features" yaml:"features"`
}

type FeatureEntry struct {
	Name       string `json:"name"       yaml:"name"`
	Branch     string `json:"branch"     yaml:"branch"`
	BaseBranch string `json:"baseBranch" yaml:"baseBranch"`
	RepoCount  int    `json:"repoCount"  yaml:"repoCount"`
	AgentReady bool   `json:"agentReady" yaml:"agentReady"`
}

type FeatureStatusResult struct {
	Feature      string       `json:"feature"      yaml:"feature"`
	Branch       string       `json:"branch"       yaml:"branch"`
	AgentReady   bool         `json:"agentReady"   yaml:"agentReady"`
	Repositories []RepoStatus `json:"repositories" yaml:"repositories"`
}

type RepoStatus struct {
	Name   string `json:"name"   yaml:"name"`
	Status string `json:"status" yaml:"status"`
}

func ListData(root string) (FeatureListResult, error) {
	dir := filepath.Join(root, config.DirName, "features")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return FeatureListResult{}, err
	}
	var features []FeatureEntry
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		b, _ := os.ReadFile(filepath.Join(dir, e.Name()))
		var m Metadata
		if json.Unmarshal(b, &m) == nil {
			ready := false
			if st, err := os.Stat(filepath.Join(root, "agent", m.Name)); err == nil && st.IsDir() {
				ready = true
			}
			features = append(features, FeatureEntry{
				Name:       m.Name,
				Branch:     m.Branch,
				BaseBranch: m.BaseBranch,
				RepoCount:  len(m.Repositories),
				AgentReady: ready,
			})
		}
	}
	return FeatureListResult{Features: features}, nil
}

func List(root string) error {
	data, err := ListData(root)
	if err != nil {
		return err
	}
	rows := [][]string{}
	for _, f := range data.Features {
		ready := "no"
		if f.AgentReady {
			ready = "yes"
		}
		rows = append(rows, []string{f.Name, f.Branch, f.BaseBranch, fmt.Sprint(f.RepoCount), ready})
	}
	ui.PrintRows([]string{"FEATURE", "BRANCH", "BASE", "REPOS", "AGENT_READY"}, rows)
	return nil
}

func StatusData(root, name string) (FeatureStatusResult, error) {
	m, err := Load(root, name)
	if err != nil {
		return FeatureStatusResult{}, err
	}
	result := FeatureStatusResult{Feature: m.Name, Branch: m.Branch}
	if st, err := os.Stat(filepath.Join(root, "agent", m.Name)); err == nil && st.IsDir() {
		result.AgentReady = true
	}
	for _, r := range m.Repositories {
		p := filepath.Join(root, r.WorktreePath)
		s, err := aggit.StatusShort(p)
		if err != nil {
			s = "ERROR: " + err.Error()
		}
		result.Repositories = append(result.Repositories, RepoStatus{Name: r.Name, Status: s})
	}
	return result, nil
}

func Status(root, name string) error {
	data, err := StatusData(root, name)
	if err != nil {
		return err
	}
	fmt.Printf("Feature: %s\nBranch: %s\n\n", data.Feature, data.Branch)
	for _, r := range data.Repositories {
		fmt.Printf("[%s]\n", r.Name)
		if r.Status == "" {
			fmt.Println("clean")
		} else {
			fmt.Println(r.Status)
		}
		fmt.Println()
	}
	return nil
}

func Commit(root, name, message string) error {
	if message == "" {
		return fmt.Errorf("commit message is required (-m)")
	}
	m, err := Load(root, name)
	if err != nil {
		return err
	}
	for _, r := range m.Repositories {
		p := filepath.Join(root, r.WorktreePath)
		output.Printf("[%s] ", r.Name)
		if !aggit.HasChanges(p) {
			output.Println("clean, skipped")
			continue
		}
		if err := aggit.CommitAll(p, message); err != nil {
			output.Printf("failed: %v\n", err)
		} else {
			output.Println("committed")
		}
	}
	return nil
}

type RebaseRepoResult struct {
	Name       string `json:"name"       yaml:"name"`
	Branch     string `json:"branch"     yaml:"branch"`
	BaseBranch string `json:"baseBranch" yaml:"baseBranch"`
	Status     string `json:"status"     yaml:"status"` // rebased | up-to-date | skipped | failed
	Detail     string `json:"detail"     yaml:"detail"`
}
type RebaseResult struct {
	Feature      string             `json:"feature"      yaml:"feature"`
	Repositories []RebaseRepoResult `json:"repositories" yaml:"repositories"`
}

// Rebase replays each feature worktree's branch onto the latest base branch
// (origin/<base> when available). Worktrees with uncommitted changes are
// skipped, and a rebase that hits conflicts is aborted so the worktree is left
// untouched. repoFilter, when non-empty, limits the operation to one repository.
func Rebase(root string, cfg config.Config, name, repoFilter string) (RebaseResult, error) {
	m, err := Load(root, name)
	if err != nil {
		return RebaseResult{}, err
	}
	result := RebaseResult{Feature: m.Name}
	for _, r := range m.Repositories {
		if repoFilter != "" && r.Name != repoFilter {
			continue
		}
		base := r.BaseBranch
		if base == "" {
			base = cfg.Git.DefaultBaseBranch
		}
		rr := RebaseRepoResult{Name: r.Name, Branch: r.Branch, BaseBranch: base}
		p := filepath.Join(root, r.WorktreePath)

		if aggit.HasChanges(p) {
			rr.Status = "skipped"
			rr.Detail = "uncommitted changes; commit or stash first"
			result.Repositories = append(result.Repositories, rr)
			continue
		}

		_ = aggit.Fetch(p) // best-effort; rebase falls back to local refs

		upstream := base
		if aggit.RemoteBranchExists(p, base) {
			upstream = "origin/" + base
		} else if !aggit.LocalBranchExists(p, base) {
			rr.Status = "skipped"
			rr.Detail = fmt.Sprintf("base branch %q not found", base)
			result.Repositories = append(result.Repositories, rr)
			continue
		}

		before, _ := aggit.HeadSHA(p)
		if err := aggit.RebaseOnto(p, upstream); err != nil {
			_ = aggit.RebaseAbort(p)
			rr.Status = "failed"
			rr.Detail = fmt.Sprintf("rebase onto %s failed (conflict); aborted, resolve manually", upstream)
			result.Repositories = append(result.Repositories, rr)
			continue
		}
		after, _ := aggit.HeadSHA(p)
		if before == after {
			rr.Status = "up-to-date"
			rr.Detail = fmt.Sprintf("already based on %s", upstream)
		} else {
			rr.Status = "rebased"
			rr.Detail = fmt.Sprintf("rebased onto %s", upstream)
		}
		result.Repositories = append(result.Repositories, rr)
	}
	return result, nil
}

func Push(root, name string) error {
	m, err := Load(root, name)
	if err != nil {
		return err
	}
	for _, r := range m.Repositories {
		p := filepath.Join(root, r.WorktreePath)
		output.Printf("[%s] pushing %s\n", r.Name, r.Branch)
		if err := aggit.Push(p, r.Branch); err != nil {
			output.Printf("failed: %v\n", err)
		} else {
			output.Println("pushed")
		}
	}
	return nil
}
