package git

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsTrackedFile(t *testing.T) {
	repo := trackedTestRepo(t)
	writeTrackedTestFile(t, filepath.Join(repo, "committed.txt"), "committed")
	writeTrackedTestFile(t, filepath.Join(repo, ".gitignore"), "ignored.txt\n")
	testGit(t, repo, "add", "committed.txt", ".gitignore")
	testGit(t, repo, "commit", "-m", "initial")
	writeTrackedTestFile(t, filepath.Join(repo, "staged.txt"), "staged")
	testGit(t, repo, "add", "staged.txt")
	writeTrackedTestFile(t, filepath.Join(repo, "untracked.txt"), "untracked")
	writeTrackedTestFile(t, filepath.Join(repo, "ignored.txt"), "ignored")

	cases := []struct {
		name string
		want bool
	}{
		{"committed.txt", true},
		{"staged.txt", true},
		{"untracked.txt", false},
		{"ignored.txt", false},
	}
	for _, tc := range cases {
		got, err := IsTracked(filepath.Join(repo, tc.name))
		if err != nil {
			t.Fatalf("IsTracked(%s): %v", tc.name, err)
		}
		if got != tc.want {
			t.Errorf("IsTracked(%s) = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestIsTrackedFolder(t *testing.T) {
	repo := trackedTestRepo(t)
	writeTrackedTestFile(t, filepath.Join(repo, "tracked", "file.txt"), "tracked")
	writeTrackedTestFile(t, filepath.Join(repo, "nested", "deep", "file.txt"), "nested")
	testGit(t, repo, "add", ".")
	testGit(t, repo, "commit", "-m", "initial")
	if err := os.MkdirAll(filepath.Join(repo, "untracked"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTrackedTestFile(t, filepath.Join(repo, "untracked", "file.txt"), "untracked")

	cases := []struct {
		name string
		want bool
	}{
		{"tracked", true},
		{"nested", true},
		{"untracked", false},
	}
	for _, tc := range cases {
		got, err := IsTracked(filepath.Join(repo, tc.name))
		if err != nil {
			t.Fatalf("IsTracked(%s): %v", tc.name, err)
		}
		if got != tc.want {
			t.Errorf("IsTracked(%s) = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// Names holding glob characters must only match themselves; a globbed pathspec
// would let the tracked weird1.txt answer for the untracked weird[1].txt.
func TestIsTrackedLiteralPathspec(t *testing.T) {
	repo := trackedTestRepo(t)
	writeTrackedTestFile(t, filepath.Join(repo, "weird1.txt"), "decoy")
	testGit(t, repo, "add", ".")
	testGit(t, repo, "commit", "-m", "initial")
	glob := filepath.Join(repo, "weird[1].txt")
	writeTrackedTestFile(t, glob, "glob")

	got, err := IsTracked(glob)
	if err != nil {
		t.Fatalf("IsTracked untracked glob name: %v", err)
	}
	if got {
		t.Error("untracked weird[1].txt reported as tracked")
	}

	testGit(t, repo, "--literal-pathspecs", "add", "--", "weird[1].txt")
	testGit(t, repo, "commit", "-m", "glob")
	got, err = IsTracked(glob)
	if err != nil {
		t.Fatalf("IsTracked tracked glob name: %v", err)
	}
	if !got {
		t.Error("tracked weird[1].txt reported as untracked")
	}
}

func TestIsTrackedOutsideWorkTree(t *testing.T) {
	dir := t.TempDir()
	writeTrackedTestFile(t, filepath.Join(dir, "loose.txt"), "loose")

	for _, path := range []string{dir, filepath.Join(dir, "loose.txt")} {
		got, err := IsTracked(path)
		if err != nil {
			t.Fatalf("IsTracked(%s): %v", path, err)
		}
		if got {
			t.Errorf("IsTracked(%s) = true outside a work tree", path)
		}
	}
}

func TestIsTrackedMissingPath(t *testing.T) {
	repo := trackedTestRepo(t)
	if _, err := IsTracked(filepath.Join(repo, "absent.txt")); err == nil {
		t.Fatal("expected an error for a missing path")
	}
}

func trackedTestRepo(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "repo")
	testGit(t, "", "init", dir)
	testGit(t, dir, "config", "user.email", "test@example.com")
	testGit(t, dir, "config", "user.name", "Test")
	return dir
}

func writeTrackedTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
