package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/agentsafe/agentsafe/internal/config"
	"github.com/agentsafe/agentsafe/internal/feature"
)

// prepareForSync sets up a prepared feature with a single repo whose agent copy
// has been edited, returning the worktree path so callers can assert on it.
func prepareForSync(t *testing.T, root string, cfg config.Config) string {
	t.Helper()
	featureName, repoName := "demo", "repo"
	worktreeRel := filepath.Join("feature", featureName, repoName)
	worktree := filepath.Join(root, worktreeRel)
	writeIndexedFile(t, filepath.Join(worktree, "file.txt"), "content")
	if err := feature.Save(root, feature.Metadata{
		Name: featureName, Key: featureName,
		Repositories: []feature.RepoMeta{{
			Name:         repoName,
			WorktreePath: filepath.ToSlash(worktreeRel),
		}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := Init(root, cfg, featureName, PrepareOptions{}); err != nil {
		t.Fatal(err)
	}
	// Edit the agent copy so there is a change to sync back.
	agentDir := config.AgentPath(root, featureName, repoName)
	writeIndexedFile(t, filepath.Join(agentDir, "file.txt"), "edited")
	return worktree
}

// With an empty message the sync still applies, but the commit is skipped, so no
// git operations are required.
func TestSyncAndCommitSyncsAndSkipsCommitWithoutMessage(t *testing.T) {
	root := t.TempDir()
	cfg := config.Default(root, "ws")
	worktree := prepareForSync(t, root, cfg)

	if err := SyncAndCommit(root, cfg, "demo", "", Options{Yes: true}); err != nil {
		t.Fatalf("SyncAndCommit: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(worktree, "file.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "edited" {
		t.Fatalf("worktree file = %q, want edited (sync did not apply)", got)
	}
}

// A dry run neither applies the sync nor attempts a commit.
func TestSyncAndCommitDryRunDoesNothing(t *testing.T) {
	root := t.TempDir()
	cfg := config.Default(root, "ws")
	worktree := prepareForSync(t, root, cfg)

	if err := SyncAndCommit(root, cfg, "demo", "feat: change", Options{Yes: true, DryRun: true}); err != nil {
		t.Fatalf("SyncAndCommit dry-run: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(worktree, "file.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "content" {
		t.Fatalf("worktree file = %q, want content (dry-run must not apply)", got)
	}
}
