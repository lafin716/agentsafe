package repo

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/agentsafe/agentsafe/internal/config"
	"github.com/agentsafe/agentsafe/internal/feature"
	aggit "github.com/agentsafe/agentsafe/internal/git"
)

func tips(names ...string) []aggit.RefTip {
	out := make([]aggit.RefTip, 0, len(names))
	for _, n := range names {
		out = append(out, aggit.RefTip{Name: n, SHA: "sha-" + n})
	}
	return out
}

func TestManagedRefsCoversBothSidesOfTheBaseAndFeatureBranches(t *testing.T) {
	worktrees := []BranchWorktree{
		{Branch: "feat/login", BaseBranch: "main"},
		{Branch: "feat/billing", BaseBranch: "main"},
	}
	refs := tips("main", "origin/main", "feat/login", "origin/feat/login",
		"feat/billing", "origin/release-2.3", "dependabot/npm/lodash")

	got := managedRefs("main", worktrees, refs, nil)

	want := []string{
		"main", "origin/main",
		"feat/login", "origin/feat/login",
		"feat/billing",
	}
	if len(got) != len(want) {
		t.Fatalf("managedRefs = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("managedRefs = %v, want %v", got, want)
		}
	}
}

func TestManagedRefsDropsRefsThatDoNotResolve(t *testing.T) {
	// origin/feat/login has never been pushed, and the base has never been
	// fetched. Both must be dropped rather than handed to git as unknown revs.
	worktrees := []BranchWorktree{{Branch: "feat/login", BaseBranch: "main"}}
	refs := tips("main", "feat/login")

	got := managedRefs("main", worktrees, refs, nil)

	for _, name := range got {
		if name == "origin/main" || name == "origin/feat/login" {
			t.Errorf("managedRefs included the non-existent %q: %v", name, got)
		}
	}
	if len(got) != 2 {
		t.Errorf("managedRefs = %v, want just the two local refs", got)
	}
}

func TestManagedRefsIncludesAFeatureWithItsOwnBaseBranch(t *testing.T) {
	// A Feature branched from a release branch rather than the repository default.
	worktrees := []BranchWorktree{{Branch: "feat/hotfix", BaseBranch: "release-2.3"}}
	refs := tips("main", "origin/main", "feat/hotfix", "release-2.3", "origin/release-2.3")

	got := managedRefs("main", worktrees, refs, nil)

	for _, want := range []string{"release-2.3", "origin/release-2.3"} {
		if !isManaged(want, got) {
			t.Errorf("managedRefs = %v, want it to include %q", got, want)
		}
	}
}

func TestManagedRefsAddsExtraRefsWithoutDuplicating(t *testing.T) {
	worktrees := []BranchWorktree{{Branch: "feat/login", BaseBranch: "main"}}
	refs := tips("main", "feat/login", "origin/release-2.3")

	got := managedRefs("main", worktrees, refs, []string{"origin/release-2.3", "main"})

	if !isManaged("origin/release-2.3", got) {
		t.Errorf("managedRefs = %v, want the extra ref", got)
	}
	seen := map[string]int{}
	for _, name := range got {
		seen[name]++
	}
	for name, count := range seen {
		if count > 1 {
			t.Errorf("managedRefs repeated %q %d times: %v", name, count, got)
		}
	}
}

// setupGraphWorkspace builds a workspace with one cloned repository and a Repo
// Worktree for a Feature branch that has diverged from the base branch.
func setupGraphWorkspace(t *testing.T) (root string, cfg config.Config) {
	t.Helper()
	root, seed, cfg := setupRemoteRepository(t, "backend")
	if err := PullOne(root, cfg, "backend"); err != nil {
		t.Fatal(err)
	}
	repoPath := config.RepoPath(root, "backend")

	// A second commit on the base branch, so the Feature has something to be
	// behind by.
	if err := os.WriteFile(filepath.Join(seed, "base.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, seed, "add", ".")
	runGit(t, seed, "commit", "-m", "base moves on")
	runGit(t, seed, "push", "origin", "main")
	if err := PullOne(root, cfg, "backend"); err != nil {
		t.Fatal(err)
	}

	// A Repo Worktree on its own branch, with a commit of its own.
	worktreeRel := filepath.ToSlash(filepath.Join("feature", "feat-1", "backend"))
	worktreeAbs := filepath.Join(root, worktreeRel)
	if err := aggit.AddWorktree(repoPath, worktreeAbs, "feat/login", "main", true); err != nil {
		t.Fatal(err)
	}
	runGit(t, worktreeAbs, "config", "user.email", "test@example.com")
	runGit(t, worktreeAbs, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(worktreeAbs, "login.txt"), []byte("login\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, worktreeAbs, "add", ".")
	runGit(t, worktreeAbs, "commit", "-m", "feature work")

	meta := feature.Metadata{
		Name:       "login-revamp",
		Key:        "feat-1",
		Branch:     "feat/login",
		BaseBranch: "main",
		Repositories: []feature.RepoMeta{{
			Name: "backend", WorktreePath: worktreeRel,
			Branch: "feat/login", BaseBranch: "main",
		}},
	}
	if err := feature.Save(root, meta); err != nil {
		t.Fatal(err)
	}
	return root, cfg
}

func TestLoadCommitGraphMarksTheRepoWorktreeOwningEachBranch(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping real-git test under -short")
	}
	root, cfg := setupGraphWorkspace(t)

	graph, err := LoadCommitGraph(root, cfg, "backend", CommitGraphOptions{})
	if err != nil {
		t.Fatal(err)
	}

	if graph.BaseBranch != "main" {
		t.Errorf("BaseBranch = %q, want main", graph.BaseBranch)
	}
	if graph.CurrentBranch != "main" {
		t.Errorf("CurrentBranch = %q, want main (the Main Clone is never moved)", graph.CurrentBranch)
	}
	if len(graph.Worktrees) != 1 {
		t.Fatalf("Worktrees = %+v, want one", graph.Worktrees)
	}
	wt := graph.Worktrees[0]
	if wt.Branch != "feat/login" || wt.Feature != "login-revamp" {
		t.Errorf("worktree = %+v, want feat/login → login-revamp", wt)
	}
	if wt.Integration.InProgress() {
		t.Errorf("worktree integration = %+v, want none", wt.Integration)
	}

	// One `git log` in the Main Clone sees the Feature branch, because the Repo
	// Worktree shares its object database.
	subjects := map[string]bool{}
	for _, c := range graph.Commits {
		subjects[c.Subject] = true
	}
	for _, want := range []string{"initial", "base moves on", "feature work"} {
		if !subjects[want] {
			t.Errorf("commit %q missing; graph has %v", want, subjects)
		}
	}
}

func TestLoadCommitGraphReportsRefsOlderThanTheCommitLimit(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping real-git test under -short")
	}
	root, cfg := setupGraphWorkspace(t)

	// One commit fits in the window; the other managed branch tips do not. They
	// must be reported rather than silently absent — otherwise the graph reads as
	// "that branch does not exist".
	graph, err := LoadCommitGraph(root, cfg, "backend", CommitGraphOptions{Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(graph.Commits) != 1 {
		t.Fatalf("commit count = %d, want 1", len(graph.Commits))
	}
	if !graph.Truncated {
		t.Error("Truncated = false, want true when the read stopped at the limit")
	}
	if len(graph.OutsideWindow) == 0 {
		t.Fatal("OutsideWindow is empty, want the branch tips that were not drawn")
	}
	drawn := map[string]bool{}
	for _, c := range graph.Commits {
		drawn[c.SHA] = true
	}
	for _, ref := range graph.OutsideWindow {
		if drawn[ref.SHA] {
			t.Errorf("ref %q is reported outside the window but its tip was drawn", ref.Name)
		}
	}

	// Asking for that ref explicitly brings it back into the graph.
	name := graph.OutsideWindow[0].Name
	wider, err := LoadCommitGraph(root, cfg, "backend", CommitGraphOptions{
		Limit: 50, ExtraRefs: []string{name},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, ref := range wider.OutsideWindow {
		if ref.Name == name {
			t.Errorf("%q still reported outside the window after being requested", name)
		}
	}
}

func TestLoadCommitGraphSurfacesAnInterruptedIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping real-git test under -short")
	}
	root, cfg := setupGraphWorkspace(t)
	worktree := filepath.Join(root, "feature", "feat-1", "backend")

	// Make the rebase conflict: both sides touch the same file.
	if err := os.WriteFile(filepath.Join(worktree, "base.txt"), []byte("feature\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, worktree, "commit", "-am", "touch base.txt")
	if err := aggit.RebaseOnto(worktree, "main"); err == nil {
		t.Skip("rebase did not conflict on this git version; nothing to assert")
	}

	graph, err := LoadCommitGraph(root, cfg, "backend", CommitGraphOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(graph.Worktrees) != 1 {
		t.Fatalf("Worktrees = %+v, want one", graph.Worktrees)
	}
	state := graph.Worktrees[0].Integration
	if state.Kind != aggit.IntegrationRebase {
		t.Errorf("Integration = %+v, want a rebase in progress", state)
	}
	if state.Branch != "feat/login" {
		t.Errorf("Integration.Branch = %q, want feat/login", state.Branch)
	}
}

func TestLoadCommitGraphRefusesAnUnknownOrUnclonedRepository(t *testing.T) {
	root, seed, cfg := setupRemoteRepository(t, "backend")
	_ = seed

	if _, err := LoadCommitGraph(root, cfg, "nope", CommitGraphOptions{}); err == nil {
		t.Error("want an error for a repository that is not configured")
	}
	// Configured but never pulled: the message has to say what to do, not fail
	// deep inside git.
	if _, err := LoadCommitGraph(root, cfg, "backend", CommitGraphOptions{}); err == nil {
		t.Error("want an error for a repository that is not cloned yet")
	}
}
