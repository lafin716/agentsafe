package git

import (
	"os"
	"path/filepath"
	"testing"
)

// writeFile creates path with content, making parent directories as needed.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestResolveGitDirForAPlainRepository(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := resolveGitDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(root, ".git"); got != want {
		t.Errorf("gitdir = %q, want %q", got, want)
	}
}

func TestResolveGitDirFollowsAWorktreeGitdirFile(t *testing.T) {
	root := t.TempDir()
	// A Repo Worktree's .git is a file pointing at the Main Clone's per-worktree
	// admin directory. git writes the path with forward slashes even on Windows.
	target := filepath.Join(root, "main", "api", ".git", "worktrees", "feat-9a1")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	wt := filepath.Join(root, "feature", "feat-9a1", "api")
	writeFile(t, filepath.Join(wt, ".git"),
		"gitdir: "+filepath.ToSlash(target)+"\n")

	got, err := resolveGitDir(wt)
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Clean(target) {
		t.Errorf("gitdir = %q, want %q", got, filepath.Clean(target))
	}
}

func TestResolveGitDirAcceptsARelativeGitdirFile(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "admin")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	wt := filepath.Join(root, "wt")
	writeFile(t, filepath.Join(wt, ".git"), "gitdir: ../admin\n")

	got, err := resolveGitDir(wt)
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Clean(target) {
		t.Errorf("gitdir = %q, want %q", got, filepath.Clean(target))
	}
}

func TestIntegrationStateOfReportsNoneForACleanWorktree(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	state, err := IntegrationStateOf(root)
	if err != nil {
		t.Fatal(err)
	}
	if state.InProgress() {
		t.Errorf("state = %+v, want none", state)
	}
	if state.Kind != IntegrationNone {
		t.Errorf("Kind = %q, want empty", state.Kind)
	}
}

func TestIntegrationStateOfReadsAnInterruptedRebase(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".git", "rebase-merge")
	writeFile(t, filepath.Join(dir, "head-name"), "refs/heads/feat/login\n")
	writeFile(t, filepath.Join(dir, "onto"), "95ae8906bde16d6adfcf87f8bc307fa1e12ff858\n")
	writeFile(t, filepath.Join(dir, "msgnum"), "2\n")
	writeFile(t, filepath.Join(dir, "end"), "3\n")

	state, err := IntegrationStateOf(root)
	if err != nil {
		t.Fatal(err)
	}
	if state.Kind != IntegrationRebase {
		t.Fatalf("Kind = %q, want %q", state.Kind, IntegrationRebase)
	}
	if state.Branch != "feat/login" {
		t.Errorf("Branch = %q, want feat/login (refs/heads/ stripped)", state.Branch)
	}
	if state.Onto != "95ae8906bde16d6adfcf87f8bc307fa1e12ff858" {
		t.Errorf("Onto = %q", state.Onto)
	}
	if state.Step != 2 || state.Total != 3 {
		t.Errorf("progress = %d/%d, want 2/3", state.Step, state.Total)
	}
}

func TestIntegrationStateOfReadsTheApplyRebaseBackend(t *testing.T) {
	root := t.TempDir()
	// `git rebase --apply` uses rebase-apply/ with next+last instead of
	// rebase-merge/ with msgnum+end.
	dir := filepath.Join(root, ".git", "rebase-apply")
	writeFile(t, filepath.Join(dir, "head-name"), "refs/heads/feat/billing\n")
	writeFile(t, filepath.Join(dir, "onto"), "abc123\n")
	writeFile(t, filepath.Join(dir, "next"), "1\n")
	writeFile(t, filepath.Join(dir, "last"), "4\n")

	state, err := IntegrationStateOf(root)
	if err != nil {
		t.Fatal(err)
	}
	if state.Kind != IntegrationRebase {
		t.Fatalf("Kind = %q, want %q", state.Kind, IntegrationRebase)
	}
	if state.Branch != "feat/billing" {
		t.Errorf("Branch = %q", state.Branch)
	}
	if state.Step != 1 || state.Total != 4 {
		t.Errorf("progress = %d/%d, want 1/4", state.Step, state.Total)
	}
}

func TestIntegrationStateOfReadsAnInterruptedMerge(t *testing.T) {
	root := t.TempDir()
	gitDir := filepath.Join(root, ".git")
	writeFile(t, filepath.Join(gitDir, "MERGE_HEAD"),
		"bc2789b3a00b1437ebd9427e4403fdf3149be7bc\n")
	writeFile(t, filepath.Join(gitDir, "MERGE_MSG"),
		"Merge branch 'origin/main' into feat/login\n\n# Conflicts:\n#\tf.txt\n")
	writeFile(t, filepath.Join(gitDir, "HEAD"), "ref: refs/heads/feat/login\n")

	state, err := IntegrationStateOf(root)
	if err != nil {
		t.Fatal(err)
	}
	if state.Kind != IntegrationMerge {
		t.Fatalf("Kind = %q, want %q", state.Kind, IntegrationMerge)
	}
	if state.Branch != "feat/login" {
		t.Errorf("Branch = %q, want feat/login from HEAD", state.Branch)
	}
	if state.Onto != "bc2789b3a00b1437ebd9427e4403fdf3149be7bc" {
		t.Errorf("Onto = %q, want the MERGE_HEAD sha", state.Onto)
	}
	if state.Summary != "Merge branch 'origin/main' into feat/login" {
		t.Errorf("Summary = %q, want the MERGE_MSG subject line", state.Summary)
	}
}

func TestIntegrationStateOfPrefersRebaseOverAStaleMergeHead(t *testing.T) {
	root := t.TempDir()
	gitDir := filepath.Join(root, ".git")
	// A conflicted rebase leaves MERGE_MSG behind (git writes it while
	// auto-merging), so merge detection must key on MERGE_HEAD and rebase must
	// still win when both look present.
	writeFile(t, filepath.Join(gitDir, "MERGE_HEAD"), "deadbeef\n")
	dir := filepath.Join(gitDir, "rebase-merge")
	writeFile(t, filepath.Join(dir, "head-name"), "refs/heads/feat/login\n")
	writeFile(t, filepath.Join(dir, "msgnum"), "1\n")
	writeFile(t, filepath.Join(dir, "end"), "1\n")

	state, err := IntegrationStateOf(root)
	if err != nil {
		t.Fatal(err)
	}
	if state.Kind != IntegrationRebase {
		t.Errorf("Kind = %q, want %q", state.Kind, IntegrationRebase)
	}
}

func TestIntegrationStateOfCostsNoGitSubprocess(t *testing.T) {
	// The state is read from the filesystem so it can be included in the
	// per-repository status the Features screens load on every render. A repo
	// whose git binary could not possibly run (no executable git dir contents)
	// still answers.
	root := t.TempDir()
	dir := filepath.Join(root, ".git", "rebase-merge")
	writeFile(t, filepath.Join(dir, "head-name"), "refs/heads/x\n")
	writeFile(t, filepath.Join(dir, "msgnum"), "1\n")
	writeFile(t, filepath.Join(dir, "end"), "2\n")

	state, err := IntegrationStateOf(root)
	if err != nil {
		t.Fatal(err)
	}
	if !state.InProgress() {
		t.Fatal("want in-progress")
	}
	// ConflictPaths is filled by IntegrationConflicts, which does shell out.
	if state.ConflictPaths == nil {
		t.Error("ConflictPaths = nil, want empty slice for JSON friendliness")
	}
	if len(state.ConflictPaths) != 0 {
		t.Errorf("ConflictPaths = %v, want empty", state.ConflictPaths)
	}
}

func TestIntegrationStateOfIgnoresAMissingRepository(t *testing.T) {
	state, err := IntegrationStateOf(filepath.Join(t.TempDir(), "nope"))
	if err != nil {
		t.Fatalf("want no error for a path that is not a repository, got %v", err)
	}
	if state.InProgress() {
		t.Errorf("state = %+v, want none", state)
	}
}
