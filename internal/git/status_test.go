package git

import (
	"os"
	"path/filepath"
	"strings"
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

func TestNormalizeBranchName(t *testing.T) {
	cases := map[string]string{
		"main":                         "main",
		"origin/main":                  "main",
		"origin/origin":                "origin",
		"origin/origin/main":           "main",
		"refs/remotes/origin/main":     "main",
		"refs/heads/main":              "main",
		"  origin/feature/x  ":         "feature/x",
		"refs/remotes/origin/origin/x": "x",
	}
	for in, want := range cases {
		if got := NormalizeBranchName(in); got != want {
			t.Errorf("NormalizeBranchName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFetchBranchDoesNotCreatePhantomOriginRef(t *testing.T) {
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
	testGit(t, "", "clone", remote, clone)

	// Passing an "origin/"-prefixed base must not create refs/remotes/origin/origin…
	if err := FetchBranch(clone, "origin/main"); err != nil {
		t.Fatal(err)
	}
	refs, err := Output(clone, "for-each-ref", "--format=%(refname)", "refs/remotes/origin")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(refs, "refs/remotes/origin/origin") {
		t.Fatalf("phantom origin ref created: %s", refs)
	}

	branches, err := ListRemoteBranches(clone)
	if err != nil {
		t.Fatal(err)
	}
	if contains(branches, "origin") {
		t.Fatalf("remote branches must not contain bare \"origin\": %#v", branches)
	}
	if !contains(branches, "main") {
		t.Fatalf("remote branches = %#v, want main", branches)
	}
}

func TestPruneStaleOriginRefs(t *testing.T) {
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
	testGit(t, "", "clone", remote, clone)

	// Simulate the phantom ref a previous buggy fetch would have left behind.
	head, err := Output(clone, "rev-parse", "refs/remotes/origin/main")
	if err != nil {
		t.Fatal(err)
	}
	testGit(t, clone, "update-ref", "refs/remotes/origin/origin", strings.TrimSpace(head))

	if branches, err := ListRemoteBranches(clone); err != nil || contains(branches, "origin") {
		t.Fatalf("pre-prune list = %#v, err = %v (must hide bare origin)", branches, err)
	}
	if err := PruneStaleOriginRefs(clone); err != nil {
		t.Fatal(err)
	}
	refs, err := Output(clone, "for-each-ref", "--format=%(refname)", "refs/remotes/origin")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(refs, "refs/remotes/origin/origin") {
		t.Fatalf("phantom ref not pruned: %s", refs)
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
