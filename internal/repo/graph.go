package repo

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/agentsafe/agentsafe/internal/config"
	"github.com/agentsafe/agentsafe/internal/feature"
	aggit "github.com/agentsafe/agentsafe/internal/git"
)

// A Commit Graph is read for one repository at a time. Commits only relate to
// each other inside a single object database, so there is no graph that spans a
// workspace — and none is needed: every Repo Worktree of a repository is created
// from its Main Clone with `git worktree add`, so they share one object database
// and a single `git log` in main/<repo> already sees every Feature branch.
//
// That property is what keeps this affordable. Reading a graph costs two git
// subprocesses regardless of how many Features exist, which matters because a
// subprocess runs into the seconds on a Windows machine with antivirus.

// DefaultCommitLimit is how far back a graph reads before the caller has to ask
// for more.
const DefaultCommitLimit = 300

type CommitGraphOptions struct {
	// AllBranches widens the read from the managed refs to every ref in the
	// repository. Off by default: a real repository can carry hundreds of origin
	// refs that bury the branches agentsafe manages.
	AllBranches bool
	// Limit caps how many commits are read. Zero means DefaultCommitLimit.
	Limit int
	// ExtraRefs are additional refs to include, which is how "load this branch's
	// tip" works for a ref whose tip fell outside the window.
	ExtraRefs []string
}

// BranchWorktree ties a branch in the graph to the Repo Worktree that has it
// checked out, so the graph can mark which branches are live work rather than
// just names.
type BranchWorktree struct {
	Branch  string `json:"branch"  yaml:"branch"`
	Feature string `json:"feature" yaml:"feature"`
	// Path is relative to the workspace root, matching RepoMeta.WorktreePath.
	Path string `json:"path" yaml:"path"`
	// Integration is any Interrupted Integration open in this Repo Worktree.
	Integration aggit.IntegrationState `json:"integration" yaml:"integration"`
	BaseBranch  string                 `json:"baseBranch"  yaml:"baseBranch"`
}

type CommitGraph struct {
	Repo string `json:"repo" yaml:"repo"`
	// BaseBranch is the repository's configured default, which Features branch
	// from unless they say otherwise.
	BaseBranch string `json:"baseBranch" yaml:"baseBranch"`
	// CurrentBranch is what the Main Clone has checked out. It is never rebased,
	// merged into, or committed to (docs/adr/0001); it is shown so the user can
	// see which branch a Pull would fast-forward.
	CurrentBranch string           `json:"currentBranch" yaml:"currentBranch"`
	Commits       []aggit.Commit   `json:"commits"       yaml:"commits"`
	Refs          []aggit.RefTip   `json:"refs"          yaml:"refs"`
	Worktrees     []BranchWorktree `json:"worktrees"     yaml:"worktrees"`
	// OutsideWindow lists refs whose tip is not among Commits — branches that
	// exist but are older than the commit limit. Reporting them is what keeps the
	// graph from reading as "this branch does not exist".
	OutsideWindow []aggit.RefTip `json:"outsideWindow" yaml:"outsideWindow"`
	Limit         int            `json:"limit"         yaml:"limit"`
	AllBranches   bool           `json:"allBranches"   yaml:"allBranches"`
	// Truncated is true when the read stopped at Limit, so older commits exist.
	Truncated bool `json:"truncated" yaml:"truncated"`
}

// LoadCommitGraph reads one repository's Commit Graph together with the Repo
// Worktree markers that make it meaningful in a workspace.
func LoadCommitGraph(root string, cfg config.Config, repoName string, opts CommitGraphOptions) (CommitGraph, error) {
	rc, ok := findRepository(cfg, repoName)
	if !ok {
		return CommitGraph{}, fmt.Errorf("repository %q not found", repoName)
	}
	repoPath := config.RepoPath(root, repoName)
	if st, err := os.Stat(repoPath); err != nil || !st.IsDir() {
		return CommitGraph{}, fmt.Errorf("repository %q is not cloned yet; pull it first", repoName)
	}
	limit := opts.Limit
	if limit <= 0 {
		limit = DefaultCommitLimit
	}
	base := rc.DefaultBranch
	if base == "" {
		base = cfg.Git.DefaultBaseBranch
	}

	graph := CommitGraph{
		Repo:        repoName,
		BaseBranch:  base,
		Limit:       limit,
		AllBranches: opts.AllBranches,
		Commits:     []aggit.Commit{},
		Refs:        []aggit.RefTip{},
		Worktrees:   []BranchWorktree{},
	}
	graph.Worktrees = worktreeBranches(root, repoName)
	graph.CurrentBranch, _ = aggit.CurrentBranch(repoPath)

	refs, err := aggit.ListRefTips(repoPath)
	if err != nil {
		return graph, err
	}
	graph.Refs = refs

	managed := managedRefs(base, graph.Worktrees, refs, opts.ExtraRefs)
	commits, err := aggit.Log(repoPath, aggit.LogRefArgs(managed, opts.AllBranches), limit)
	if err != nil {
		return graph, err
	}
	graph.Commits = commits
	graph.Truncated = len(commits) >= limit

	inWindow := make(map[string]bool, len(commits))
	for _, c := range commits {
		inWindow[c.SHA] = true
	}
	graph.OutsideWindow = []aggit.RefTip{}
	for _, ref := range refs {
		// Only refs the graph was asked to show can be "missing" from it; the
		// hundreds of refs excluded by the managed-set default are not surprising
		// by their absence.
		if !opts.AllBranches && !isManaged(ref.Name, managed) {
			continue
		}
		if !inWindow[ref.SHA] {
			graph.OutsideWindow = append(graph.OutsideWindow, ref)
		}
	}
	return graph, nil
}

// worktreeBranches finds every Feature branch that has a Repo Worktree for this
// repository, with any Interrupted Integration open in it. Reads metadata and the
// filesystem only — no git subprocess per worktree.
func worktreeBranches(root, repoName string) []BranchWorktree {
	all, err := feature.LoadAll(root)
	if err != nil {
		return []BranchWorktree{}
	}
	out := []BranchWorktree{}
	for _, m := range all {
		for _, r := range m.Repositories {
			if r.Name != repoName || r.Branch == "" {
				continue
			}
			wt := BranchWorktree{
				Branch:     r.Branch,
				Feature:    m.Name,
				Path:       r.WorktreePath,
				BaseBranch: r.BaseBranch,
			}
			// The conflicted paths are what makes an Interrupted Integration
			// actionable on the graph page (docs/adr/0002) — without them the
			// banner can only say that something is wrong, not what. Listing
			// them costs a subprocess, so it is paid only for a worktree that
			// actually has one open.
			if state, err := aggit.IntegrationStateWithConflicts(
				filepath.Join(root, r.WorktreePath)); err == nil {
				wt.Integration = state
			}
			out = append(out, wt)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Branch < out[j].Branch })
	return out
}

// managedRefs is the default ref selection: the Base Branch and every Feature
// branch with a Repo Worktree here, each on both sides where the ref exists.
// Names that do not resolve are dropped so a never-fetched origin ref does not
// have to be tolerated downstream.
func managedRefs(base string, worktrees []BranchWorktree, refs []aggit.RefTip, extra []string) []string {
	exists := make(map[string]bool, len(refs))
	for _, ref := range refs {
		exists[ref.Name] = true
	}
	seen := map[string]bool{}
	out := []string{}
	add := func(name string) {
		if name == "" || seen[name] || !exists[name] {
			return
		}
		seen[name] = true
		out = append(out, name)
	}
	addBothSides := func(branch string) {
		add(branch)
		add("origin/" + branch)
	}
	addBothSides(base)
	for _, wt := range worktrees {
		addBothSides(wt.Branch)
		if wt.BaseBranch != base {
			addBothSides(wt.BaseBranch)
		}
	}
	for _, name := range extra {
		add(name)
	}
	return out
}

func isManaged(name string, managed []string) bool {
	for _, m := range managed {
		if m == name {
			return true
		}
	}
	return false
}

func findRepository(cfg config.Config, name string) (config.Repository, bool) {
	for _, r := range cfg.Repositories {
		if r.Name == name {
			return r, true
		}
	}
	return config.Repository{}, false
}

// PrintCommitGraph renders a graph the way `git log --graph --oneline` does, for
// the CLI's text mode. Structured output emits the graph as-is instead.
func PrintCommitGraph(graph CommitGraph) {
	fmt.Printf("Repository: %s\nBase branch: %s\n", graph.Repo, graph.BaseBranch)
	if graph.CurrentBranch != "" {
		fmt.Printf("Main clone on: %s\n", graph.CurrentBranch)
	}
	if len(graph.Worktrees) > 0 {
		fmt.Println("Repo worktrees:")
		for _, wt := range graph.Worktrees {
			line := fmt.Sprintf("  %s → %s", wt.Branch, wt.Feature)
			if wt.Integration.InProgress() {
				line += fmt.Sprintf("  [%s in progress]", wt.Integration.Kind)
			}
			fmt.Println(line)
		}
	}
	if len(graph.OutsideWindow) > 0 {
		fmt.Printf("Outside the newest %d commits:", graph.Limit)
		for _, ref := range graph.OutsideWindow {
			fmt.Printf(" %s", ref.Name)
		}
		fmt.Println()
	}
	fmt.Println()
	for _, c := range graph.Commits {
		decor := ""
		if len(c.Refs) > 0 {
			names := make([]string, 0, len(c.Refs))
			for _, r := range c.Refs {
				names = append(names, r.Name)
			}
			decor = " (" + strings.Join(names, ", ") + ")"
		}
		fmt.Printf("* %s%s %s\n", aggit.ShortSHA(c.SHA), decor, c.Subject)
	}
	if graph.Truncated {
		fmt.Printf("\n(stopped at %d commits; raise --limit for more)\n", graph.Limit)
	}
}
