package feature

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/agentsafe/agentsafe/internal/config"
)

func TestDeleteWithResultContinuesWhenGitWorktreeRemovalFails(t *testing.T) {
	root := t.TempDir()
	name := "broken-worktrees"
	repos := []RepoMeta{
		{Name: "one", WorktreePath: filepath.ToSlash(filepath.Join("feature", name, "one")), Branch: "feature/broken"},
		{Name: "two", WorktreePath: filepath.ToSlash(filepath.Join("feature", name, "two")), Branch: "feature/broken"},
	}
	if err := Save(root, Metadata{Name: name, Repositories: repos}); err != nil {
		t.Fatal(err)
	}

	for _, repo := range repos {
		dir := filepath.Join(root, filepath.FromSlash(repo.WorktreePath))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte(repo.Name), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	artifactPaths := []string{
		filepath.Join(root, "agent", name, "one"),
		filepath.Join(config.HistoryDir(root), name, "one"),
	}
	for _, path := range artifactPaths {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Dir(config.SessionMetaPath(root, name)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config.SessionMetaPath(root, name), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := DeleteWithResult(root, name, DeleteOptions{Force: true})
	if err != nil {
		t.Fatalf("DeleteWithResult returned fatal error: %v", err)
	}
	if len(result.Warnings) == 0 {
		t.Fatal("expected warnings for missing git repositories")
	}

	removed := []string{
		filepath.Join(root, "feature", name),
		config.FeatureMetaPath(root, name),
		filepath.Join(root, "agent", name),
		config.SessionMetaPath(root, name),
		filepath.Join(config.HistoryDir(root), name),
	}
	for _, path := range removed {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("expected %s to be removed, stat error = %v", path, err)
		}
	}
}
