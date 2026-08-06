package agent

import (
	"os"
	"path/filepath"
	"testing"
)

// writeFile is a small test helper.
func writeHistoryFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestRecordSyncAndRollback(t *testing.T) {
	root := t.TempDir()
	feature, repo := "feat", "myrepo"
	wt := filepath.Join(root, "feature", feature, repo)

	// Pre-sync worktree state: keep.txt modified, gone.txt to be deleted.
	writeHistoryFile(t, filepath.Join(wt, "keep.txt"), "old")
	writeHistoryFile(t, filepath.Join(wt, "gone.txt"), "bye")

	changes := []Change{
		{Repo: repo, Type: Modified, Path: "keep.txt"},
		{Repo: repo, Type: Deleted, Path: "gone.txt"},
		{Repo: repo, Type: Added, Path: "new.txt"},
	}

	// Snapshot, then apply the sync to the worktree.
	if err := RecordSync(root, feature, repo, wt, changes); err != nil {
		t.Fatalf("RecordSync: %v", err)
	}
	writeHistoryFile(t, filepath.Join(wt, "keep.txt"), "new")           // modified
	os.Remove(filepath.Join(wt, "gone.txt"))                     // deleted
	writeHistoryFile(t, filepath.Join(wt, "new.txt"), "added")          // added

	recs, err := ListHistory(root, feature, repo)
	if err != nil || len(recs) != 1 {
		t.Fatalf("expected 1 record, got %d (%v)", len(recs), err)
	}
	if HistoryDepth(root, feature, repo) != 1 {
		t.Fatalf("depth != 1")
	}

	// Non-top rollback is rejected.
	if err := Rollback(root, feature, repo, "bogus", wt); err == nil {
		t.Fatalf("expected rejection of non-top rollback")
	}

	// Roll back the top entry.
	if err := Rollback(root, feature, repo, recs[0].ID, wt); err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	if b, _ := os.ReadFile(filepath.Join(wt, "keep.txt")); string(b) != "old" {
		t.Errorf("keep.txt not restored: %q", string(b))
	}
	if b, _ := os.ReadFile(filepath.Join(wt, "gone.txt")); string(b) != "bye" {
		t.Errorf("gone.txt not recreated: %q", string(b))
	}
	if _, err := os.Stat(filepath.Join(wt, "new.txt")); !os.IsNotExist(err) {
		t.Errorf("new.txt should be removed on rollback")
	}
	if HistoryDepth(root, feature, repo) != 0 {
		t.Errorf("stack should be empty after rollback")
	}
}
