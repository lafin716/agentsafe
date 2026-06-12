package repo

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/agentsafe/agentsafe/internal/config"
	"github.com/agentsafe/agentsafe/internal/feature"
)

func TestPullOneClonesIntoEmptyDirectory(t *testing.T) {
	root, _, cfg := setupRemoteRepository(t, "backend")
	dest := config.RepoPath(root, "backend")
	// Simulate a leftover empty directory from a failed clone / partial removal.
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := PullOne(root, cfg, "backend"); err != nil {
		t.Fatalf("PullOne into empty dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "README.md")); err != nil {
		t.Fatalf("expected clone to populate empty directory: %v", err)
	}
}

func TestRemoveDeletesClonedFilesAndWorktree(t *testing.T) {
	root, _, cfg := setupRemoteRepository(t, "backend")
	if err := PullOne(root, cfg, "backend"); err != nil {
		t.Fatal(err)
	}
	if err := feature.CreateWithOptions(root, cfg, "feat", feature.CreateOptions{Base: "main"}); err != nil {
		t.Fatal(err)
	}

	worktree := config.WorktreePath(root, "feat", "backend")
	if _, err := os.Stat(worktree); err != nil {
		t.Fatalf("expected worktree to exist: %v", err)
	}

	newCfg, res, err := Remove(root, cfg, "backend", true)
	if err != nil {
		t.Fatalf("Remove: %v (warnings: %v)", err, res.Warnings)
	}
	if len(res.Warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", res.Warnings)
	}
	if len(newCfg.Repositories) != 0 {
		t.Fatalf("repository still in config: %+v", newCfg.Repositories)
	}
	for _, p := range []string{
		config.RepoPath(root, "backend"),
		worktree,
		filepath.Join(root, "feature", "feat"),
	} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatalf("path %s should be removed, stat err = %v", p, err)
		}
	}
	m, err := feature.Load(root, "feat")
	if err != nil {
		t.Fatalf("load feature: %v", err)
	}
	if len(m.Repositories) != 0 {
		t.Fatalf("repo not dropped from feature metadata: %+v", m.Repositories)
	}
}
