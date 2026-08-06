package git

import (
	"path/filepath"
	"strings"
	"testing"
)

// These exercise the real git binary: the fixtures in integration_test.go were
// derived by probing git's on-disk layout, and these keep that reading honest
// across git versions.
//
// A git subprocess costs seconds on a Windows machine running antivirus, so each
// test here covers a whole lifecycle rather than one assertion, and the set is
// skipped under -short.

func requireRealGit(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping real-git test under -short; the fixture tests in " +
			"integration_test.go cover the same parsing")
	}
	// Identity through the environment rather than two `git config` calls per
	// repository — run() inherits os.Environ().
	t.Setenv("GIT_AUTHOR_NAME", "Test")
	t.Setenv("GIT_AUTHOR_EMAIL", "test@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "Test")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@example.com")
}

// conflictRepo builds a repository where feat and main both rewrite the same
// line, so rebasing or merging one onto the other conflicts.
func conflictRepo(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "repo")
	testGit(t, "", "init", "-b", "main", dir)
	writeFile(t, filepath.Join(dir, "f.txt"), "a\nb\nc\n")
	testGit(t, dir, "add", ".")
	testGit(t, dir, "commit", "-m", "base")
	testGit(t, dir, "checkout", "-b", "feat")
	writeFile(t, filepath.Join(dir, "f.txt"), "a\nFEAT\nc\n")
	testGit(t, dir, "commit", "-am", "feat side")
	testGit(t, dir, "checkout", "main")
	writeFile(t, filepath.Join(dir, "f.txt"), "a\nMAIN\nc\n")
	testGit(t, dir, "commit", "-am", "main side")
	testGit(t, dir, "checkout", "feat")
	return dir
}

func TestRebaseConflictLifecycle(t *testing.T) {
	requireRealGit(t)
	repo := conflictRepo(t)
	original, _ := HeadSHA(repo)

	// Rebase leaves the conflict in place rather than aborting (docs/adr/0002).
	if err := RebaseOnto(repo, "main"); err == nil {
		t.Fatal("RebaseOnto: want a conflict error")
	}
	state, err := IntegrationStateWithConflicts(repo)
	if err != nil {
		t.Fatal(err)
	}
	if state.Kind != IntegrationRebase {
		t.Fatalf("Kind = %q, want %q", state.Kind, IntegrationRebase)
	}
	if state.Branch != "feat" {
		t.Errorf("Branch = %q, want feat", state.Branch)
	}
	if state.Total != 1 {
		t.Errorf("Total = %d, want 1", state.Total)
	}
	if len(state.ConflictPaths) != 1 || state.ConflictPaths[0] != "f.txt" {
		t.Errorf("ConflictPaths = %v, want [f.txt]", state.ConflictPaths)
	}
	// This is the state the app used to misreport: mid-rebase the HEAD is
	// detached, so the branch name every status screen reads comes back empty.
	// The IntegrationState is what they have to show instead.
	if branch, _ := CurrentBranch(repo); branch != "" {
		t.Errorf("CurrentBranch = %q during a rebase, want empty", branch)
	}

	// Abort restores the branch exactly.
	if err := RebaseAbort(repo); err != nil {
		t.Fatalf("RebaseAbort: %v", err)
	}
	if state, _ := IntegrationStateOf(repo); state.InProgress() {
		t.Errorf("state = %+v, want none after abort", state)
	}
	if after, _ := HeadSHA(repo); after != original {
		t.Errorf("HEAD = %q after abort, want the original %q", after, original)
	}

	// Resolving and continuing finishes the rebase and reattaches the branch.
	if err := RebaseOnto(repo, "main"); err == nil {
		t.Fatal("RebaseOnto: want a conflict error on the second run too")
	}
	writeFile(t, filepath.Join(repo, "f.txt"), "a\nRESOLVED\nc\n")
	testGit(t, repo, "add", "f.txt")
	if err := RebaseContinue(repo); err != nil {
		t.Fatalf("RebaseContinue: %v", err)
	}
	if state, _ := IntegrationStateOf(repo); state.InProgress() {
		t.Errorf("state = %+v, want none after continue", state)
	}
	if branch, _ := CurrentBranch(repo); branch != "feat" {
		t.Errorf("CurrentBranch = %q, want feat", branch)
	}
}

func TestMergeConflictLifecycle(t *testing.T) {
	requireRealGit(t)
	repo := conflictRepo(t)

	if err := MergeOnto(repo, "main"); err == nil {
		t.Fatal("MergeOnto: want a conflict error")
	}
	state, err := IntegrationStateWithConflicts(repo)
	if err != nil {
		t.Fatal(err)
	}
	if state.Kind != IntegrationMerge {
		t.Fatalf("Kind = %q, want %q", state.Kind, IntegrationMerge)
	}
	// Unlike a rebase, a merge keeps the branch checked out.
	if state.Branch != "feat" {
		t.Errorf("Branch = %q, want feat", state.Branch)
	}
	if len(state.ConflictPaths) != 1 || state.ConflictPaths[0] != "f.txt" {
		t.Errorf("ConflictPaths = %v, want [f.txt]", state.ConflictPaths)
	}
	if state.Summary == "" {
		t.Error("Summary is empty, want the merge message subject")
	}

	if err := MergeAbort(repo); err != nil {
		t.Fatalf("MergeAbort: %v", err)
	}
	if state, _ := IntegrationStateOf(repo); state.InProgress() {
		t.Errorf("state = %+v, want none after abort", state)
	}
}

func TestIntegrationStateReadsARepoWorktreeNotJustAMainClone(t *testing.T) {
	requireRealGit(t)
	// The case that matters for agentsafe: the conflict happens in a Repo
	// Worktree, whose .git is a file pointing into the Main Clone.
	main := conflictRepo(t)
	testGit(t, main, "checkout", "main")
	worktree := filepath.Join(filepath.Dir(main), "wt")
	if err := AddWorktree(main, worktree, "feat", "", false); err != nil {
		t.Fatalf("AddWorktree: %v", err)
	}

	if err := RebaseOnto(worktree, "main"); err == nil {
		t.Fatal("RebaseOnto: want a conflict error")
	}

	state, err := IntegrationStateWithConflicts(worktree)
	if err != nil {
		t.Fatal(err)
	}
	if state.Kind != IntegrationRebase {
		t.Fatalf("Kind = %q, want %q", state.Kind, IntegrationRebase)
	}
	if state.Branch != "feat" {
		t.Errorf("Branch = %q, want feat", state.Branch)
	}
	if len(state.ConflictPaths) != 1 {
		t.Errorf("ConflictPaths = %v, want one entry", state.ConflictPaths)
	}
	// A conflict in one worktree leaves the Main Clone untouched.
	if state, _ := IntegrationStateOf(main); state.InProgress() {
		t.Errorf("Main Clone state = %+v, want none", state)
	}
}

func TestLogAndRefsAgainstARealRepository(t *testing.T) {
	requireRealGit(t)
	repo := conflictRepo(t) // on feat, with main diverged from a shared root
	testGit(t, repo, "tag", "v1.0")

	// A Base Branch that was never fetched must not blank the graph, so
	// origin/main is included deliberately even though it does not exist.
	commits, err := Log(repo, LogRefArgs([]string{"main", "origin/main", "feat"}, false), 50)
	if err != nil {
		t.Fatalf("Log with a missing ref: %v", err)
	}
	if len(commits) != 3 {
		t.Fatalf("commit count = %d, want 3 (root + both sides)", len(commits))
	}
	bySubject := map[string]Commit{}
	for _, c := range commits {
		bySubject[c.Subject] = c
	}
	feat, ok := bySubject["feat side"]
	if !ok {
		t.Fatalf("no 'feat side' commit in %v", bySubject)
	}
	if !feat.IsHead {
		t.Error("'feat side' IsHead = false, want true (feat is checked out)")
	}
	if len(feat.Parents) != 1 {
		t.Errorf("'feat side' Parents = %v, want one", feat.Parents)
	}
	kinds := []string{}
	for _, r := range feat.Refs {
		kinds = append(kinds, string(r.Kind)+":"+r.Name)
	}
	if !contains(kinds, "head:feat") {
		t.Errorf("'feat side' refs = %v, want head:feat", kinds)
	}
	if base := bySubject["base"]; len(base.Parents) != 0 {
		t.Errorf("root commit Parents = %v, want empty", base.Parents)
	}

	tips, err := ListRefTips(repo)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]RefTip{}
	for _, tip := range tips {
		byName[tip.Name] = tip
	}
	for name, kind := range map[string]RefKind{
		"main": RefHead, "feat": RefHead, "v1.0": RefTag,
	} {
		if byName[name].Kind != kind {
			t.Errorf("ref %q = %+v, want kind %q", name, byName[name], kind)
		}
	}
	head, _ := HeadSHA(repo)
	if byName["feat"].SHA != strings.TrimSpace(head) {
		t.Errorf("feat tip = %q, want HEAD %q", byName["feat"].SHA, head)
	}

	// name-status against the same repository: an add and a delete in one commit.
	writeFile(t, filepath.Join(repo, "added.txt"), "new\n")
	testGit(t, repo, "add", "added.txt")
	testGit(t, repo, "rm", "-q", "f.txt")
	testGit(t, repo, "commit", "-m", "swap files")
	head, _ = HeadSHA(repo)
	changes, err := CommitFiles(repo, head)
	if err != nil {
		t.Fatal(err)
	}
	byPath := map[string]string{}
	for _, c := range changes {
		byPath[c.Path] = c.Status
	}
	if byPath["added.txt"] != "A" || byPath["f.txt"] != "D" {
		t.Errorf("name-status = %v, want added.txt:A and f.txt:D", byPath)
	}
}
