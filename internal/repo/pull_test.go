package repo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentsafe/agentsafe/internal/config"
	aggit "github.com/agentsafe/agentsafe/internal/git"
)

func TestPullOneClonesAndPulls(t *testing.T) {
	root, seed, cfg := setupRemoteRepository(t, "backend")
	dest := config.RepoPath(root, "backend")

	if err := PullOne(root, cfg, "backend"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dest, "README.md")); err != nil {
		t.Fatalf("clone did not create repository content: %v", err)
	}

	if err := os.WriteFile(filepath.Join(seed, "new.txt"), []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, seed, "add", ".")
	runGit(t, seed, "commit", "-m", "remote update")
	runGit(t, seed, "push", "origin", "main")

	if err := PullOne(root, cfg, "backend"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dest, "new.txt")); err != nil {
		t.Fatalf("pull did not fast-forward repository: %v", err)
	}
}

func TestPullOneDirtyRepositoryFetchesWithoutPulling(t *testing.T) {
	root, seed, cfg := setupRemoteRepository(t, "backend")
	dest := config.RepoPath(root, "backend")
	if err := PullOne(root, cfg, "backend"); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(dest, "README.md"), []byte("local change"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(seed, "remote.txt"), []byte("remote"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, seed, "add", ".")
	runGit(t, seed, "commit", "-m", "remote update")
	runGit(t, seed, "push", "origin", "main")

	if err := PullOne(root, cfg, "backend"); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(dest, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "local change" {
		t.Fatalf("dirty file was changed: %q", content)
	}
	if _, err := os.Stat(filepath.Join(dest, "remote.txt")); !os.IsNotExist(err) {
		t.Fatalf("dirty repository was unexpectedly pulled, stat err = %v", err)
	}
	if !aggit.RemoteBranchExists(dest, "main") {
		t.Fatal("expected fetch to update the remote branch")
	}
}

func TestPullOneRejectsUnknownRepository(t *testing.T) {
	err := PullOne(t.TempDir(), config.Default(t.TempDir(), "test"), "missing")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected repository not found error, got %v", err)
	}
}

func setupRemoteRepository(t *testing.T, name string) (root, seed string, cfg config.Config) {
	t.Helper()
	setTestGitTimeout(t)
	root = t.TempDir()
	remote := filepath.Join(t.TempDir(), name+".git")
	runGit(t, "", "init", "--bare", remote)
	seed = filepath.Join(t.TempDir(), "seed")
	runGit(t, "", "init", "-b", "main", seed)
	runGit(t, seed, "config", "user.email", "test@example.com")
	runGit(t, seed, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(seed, "README.md"), []byte(name), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, seed, "add", ".")
	runGit(t, seed, "commit", "-m", "initial")
	runGit(t, seed, "remote", "add", "origin", remote)
	runGit(t, seed, "push", "-u", "origin", "main")
	runGit(t, remote, "symbolic-ref", "HEAD", "refs/heads/main")

	cfg = config.Default(root, "test")
	cfg.Git.DefaultBaseBranch = "main"
	cfg.Repositories = []config.Repository{{
		Name: name, URL: remote, DefaultBranch: "main",
	}}
	return root, seed, cfg
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	if _, err := aggit.Run(dir, args...); err != nil {
		t.Fatalf("git %s: %v", strings.Join(args, " "), err)
	}
}

func setTestGitTimeout(t *testing.T) {
	t.Helper()
	if os.Getenv("AGENTSAFE_GIT_TIMEOUT_SECONDS") == "" {
		t.Setenv("AGENTSAFE_GIT_TIMEOUT_SECONDS", "10")
	}
}
