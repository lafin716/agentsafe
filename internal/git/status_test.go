package git

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseStatusPorcelain(t *testing.T) {
	raw := "?? new.txt\n M modified.txt\nD  deleted.txt\nR  old.txt -> renamed.txt\nUU conflict.txt\n!! ignored.txt\n"
	got := ParseStatusPorcelain(raw)
	want := []FileStatus{
		{Code: "??", Type: "added", Path: "new.txt"},
		{Code: " M", Type: "modified", Path: "modified.txt"},
		{Code: "D ", Type: "deleted", Path: "deleted.txt"},
		{Code: "R ", Type: "renamed", Path: "renamed.txt"},
		{Code: "UU", Type: "conflict", Path: "conflict.txt"},
		{Code: "!!", Type: "other", Path: "ignored.txt"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d entries, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("entry %d = %#v, want %#v", i, got[i], want[i])
		}
	}
}

func TestCheckoutRemoteBranchCreatesLocalTrackingBranch(t *testing.T) {
	remote := filepath.Join(t.TempDir(), "remote.git")
	work := filepath.Join(t.TempDir(), "work")
	clone := filepath.Join(t.TempDir(), "clone")
	testGit(t, "", "init", "--bare", remote)
	testGit(t, "", "clone", remote, work)
	testGit(t, work, "config", "user.email", "test@example.com")
	testGit(t, work, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(work, "README.md"), []byte("main"), 0o644); err != nil {
		t.Fatal(err)
	}
	testGit(t, work, "add", ".")
	testGit(t, work, "commit", "-m", "initial")
	testGit(t, work, "branch", "-M", "main")
	testGit(t, work, "push", "-u", "origin", "main")
	testGit(t, work, "checkout", "-b", "release/v1")
	if err := os.WriteFile(filepath.Join(work, "release.txt"), []byte("release"), 0o644); err != nil {
		t.Fatal(err)
	}
	testGit(t, work, "add", ".")
	testGit(t, work, "commit", "-m", "release")
	testGit(t, work, "push", "-u", "origin", "release/v1")
	testGit(t, "", "clone", remote, clone)
	testGit(t, clone, "fetch", "--all", "--prune")

	branches, err := ListRemoteBranches(clone)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(branches, "release/v1") {
		t.Fatalf("remote branches = %#v, want release/v1", branches)
	}
	if err := CheckoutRemoteBranch(clone, "origin/release/v1"); err != nil {
		t.Fatal(err)
	}
	if current, err := CurrentBranch(clone); err != nil || current != "release/v1" {
		t.Fatalf("current branch = %q, err = %v", current, err)
	}
	if upstream, err := Upstream(clone, "release/v1"); err != nil || upstream != "origin/release/v1" {
		t.Fatalf("upstream = %q, err = %v", upstream, err)
	}
}

func testGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	if _, err := Run(dir, args...); err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
}

func contains(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}
