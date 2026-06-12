package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/agentsafe/agentsafe/internal/config"
	"github.com/agentsafe/agentsafe/internal/feature"
)

func TestPrepareRepositoryPreservesOtherRepositoriesAndMetadata(t *testing.T) {
	root := t.TempDir()
	name := "demo"
	repos := []feature.RepoMeta{
		{Name: "one", WorktreePath: filepath.ToSlash(filepath.Join("feature", name, "one")), Revision: 1},
		{Name: "two", WorktreePath: filepath.ToSlash(filepath.Join("feature", name, "two")), Revision: 2},
	}
	for _, repo := range repos {
		path := filepath.Join(root, filepath.FromSlash(repo.WorktreePath))
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, repo.Name+".txt"), []byte(repo.Name), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := feature.Save(root, feature.Metadata{Name: name, Revision: 2, Repositories: repos}); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default(root, "test")
	cfg.Agent.DefaultExclude = nil

	if _, err := PrepareRepository(root, cfg, name, "one", PrepareOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(config.AgentPath(root, name, "one"), "agent-edit.txt"), []byte("edit"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := PrepareRepository(root, cfg, name, "two", PrepareOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(config.AgentPath(root, name, "one"), "agent-edit.txt")); err != nil {
		t.Fatalf("other repository agent folder was changed: %v", err)
	}

	meta := LoadPrepareMetadata(root, name)
	if len(meta.Repositories) != 2 {
		t.Fatalf("prepared repositories = %d, want 2", len(meta.Repositories))
	}
	if !allRepositoriesCurrent(feature.Metadata{Repositories: repos}, meta) {
		t.Fatal("expected all repository metadata to be current")
	}
}

func TestValidatePreparedRepositoriesRejectsMissingAndStale(t *testing.T) {
	root := t.TempDir()
	name := "demo"
	fm := feature.Metadata{Name: name, Repositories: []feature.RepoMeta{
		{Name: "one", Revision: 1},
		{Name: "two", Revision: 2},
	}}
	pm := PrepareMetadata{Repositories: []PrepareRepo{
		{Name: "one", WorktreeRevision: 1},
		{Name: "two", WorktreeRevision: 1},
	}}
	for _, repo := range []string{"one", "two"} {
		if err := os.MkdirAll(config.AgentPath(root, name, repo), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := validatePreparedRepositories(root, name, fm, pm, ""); err == nil {
		t.Fatal("expected stale repository validation error")
	}
	if err := validatePreparedRepositories(root, name, fm, pm, "one"); err != nil {
		t.Fatalf("current filtered repository should pass: %v", err)
	}
}
