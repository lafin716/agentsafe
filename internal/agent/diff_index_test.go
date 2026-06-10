package agent

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/agentsafe/agentsafe/internal/config"
	"github.com/agentsafe/agentsafe/internal/feature"
	"github.com/agentsafe/agentsafe/internal/fsutil"
)

func writeIndexedFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func snapshotFile(t *testing.T, path string) FileSnapshot {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	hash, err := fsutil.SHA256File(path)
	if err != nil {
		t.Fatal(err)
	}
	return FileSnapshot{Size: info.Size(), ModTimeNano: info.ModTime().UnixNano(), Hash: hash}
}

func TestCompareIndexedSkipsHashesForUnchangedFiles(t *testing.T) {
	agentDir := filepath.Join(t.TempDir(), "agent")
	worktreeDir := filepath.Join(t.TempDir(), "worktree")
	writeIndexedFile(t, filepath.Join(agentDir, "same.txt"), "same")
	writeIndexedFile(t, filepath.Join(worktreeDir, "same.txt"), "same")
	index := map[string]FileIndexEntry{
		"same.txt": {
			Agent:    snapshotFile(t, filepath.Join(agentDir, "same.txt")),
			Worktree: snapshotFile(t, filepath.Join(worktreeDir, "same.txt")),
		},
	}
	hashCalls := 0
	changes, err := compare("repo", agentDir, worktreeDir, NewIgnoreMatcher(nil), nil, index, func(path string) (string, error) {
		hashCalls++
		return fsutil.SHA256File(path)
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 0 {
		t.Fatalf("unexpected changes: %#v", changes)
	}
	if hashCalls != 0 {
		t.Fatalf("hashed %d unchanged files, want 0", hashCalls)
	}
}

func TestCompareIndexedSkipsPreparedMaskedDifference(t *testing.T) {
	agentDir := filepath.Join(t.TempDir(), "agent")
	worktreeDir := filepath.Join(t.TempDir(), "worktree")
	agentPath := filepath.Join(agentDir, "secret.txt")
	worktreePath := filepath.Join(worktreeDir, "secret.txt")
	writeIndexedFile(t, agentPath, "__MASKED__")
	writeIndexedFile(t, worktreePath, "top-secret")
	index := map[string]FileIndexEntry{
		"secret.txt": {
			Agent:    snapshotFile(t, agentPath),
			Worktree: snapshotFile(t, worktreePath),
		},
	}
	hashCalls := 0
	changes, err := compare(
		"repo",
		agentDir,
		worktreeDir,
		NewIgnoreMatcher(nil),
		map[string]bool{"secret.txt": true},
		index,
		func(path string) (string, error) {
			hashCalls++
			return fsutil.SHA256File(path)
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 0 {
		t.Fatalf("unexpected changes: %#v", changes)
	}
	if hashCalls != 0 {
		t.Fatalf("hashed prepared masked file %d time(s), want 0", hashCalls)
	}
}

func TestCompareIndexedDetectsCandidates(t *testing.T) {
	agentDir := filepath.Join(t.TempDir(), "agent")
	worktreeDir := filepath.Join(t.TempDir(), "worktree")
	for _, name := range []string{"same-size.txt", "size.txt", "deleted.txt"} {
		writeIndexedFile(t, filepath.Join(agentDir, name), "base")
		writeIndexedFile(t, filepath.Join(worktreeDir, name), "base")
	}
	index := map[string]FileIndexEntry{}
	for _, name := range []string{"same-size.txt", "size.txt", "deleted.txt"} {
		index[name] = FileIndexEntry{
			Agent:    snapshotFile(t, filepath.Join(agentDir, name)),
			Worktree: snapshotFile(t, filepath.Join(worktreeDir, name)),
		}
	}

	writeIndexedFile(t, filepath.Join(agentDir, "same-size.txt"), "edit")
	writeIndexedFile(t, filepath.Join(agentDir, "size.txt"), "longer")
	if err := os.Remove(filepath.Join(agentDir, "deleted.txt")); err != nil {
		t.Fatal(err)
	}
	writeIndexedFile(t, filepath.Join(agentDir, "added.txt"), "new")
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(filepath.Join(agentDir, "same-size.txt"), future, future); err != nil {
		t.Fatal(err)
	}

	changes, err := CompareIndexed("repo", agentDir, worktreeDir, NewIgnoreMatcher(nil), nil, index)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]ChangeType{}
	for _, change := range changes {
		got[change.Path] = change.Type
	}
	want := map[string]ChangeType{
		"added.txt":     Added,
		"deleted.txt":   Deleted,
		"same-size.txt": Modified,
		"size.txt":      Modified,
	}
	if len(got) != len(want) {
		t.Fatalf("changes = %#v, want %#v", got, want)
	}
	for path, typ := range want {
		if got[path] != typ {
			t.Errorf("%s = %s, want %s", path, got[path], typ)
		}
	}
}

func TestCompareIndexedFallsBackWithoutIndex(t *testing.T) {
	agentDir := filepath.Join(t.TempDir(), "agent")
	worktreeDir := filepath.Join(t.TempDir(), "worktree")
	writeIndexedFile(t, filepath.Join(agentDir, "file.txt"), "agent")
	writeIndexedFile(t, filepath.Join(worktreeDir, "file.txt"), "tree")

	changes, err := CompareIndexed("repo", agentDir, worktreeDir, NewIgnoreMatcher(nil), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || changes[0].Type != Modified {
		t.Fatalf("changes = %#v, want one modified file", changes)
	}
}

func TestPrepareStoresFileIndex(t *testing.T) {
	root := t.TempDir()
	featureName := "indexed"
	repoName := "repo"
	worktreeRel := filepath.Join("feature", featureName, repoName)
	worktree := filepath.Join(root, worktreeRel)
	writeIndexedFile(t, filepath.Join(worktree, "file.txt"), "content")
	if err := feature.Save(root, feature.Metadata{
		Name: featureName,
		Repositories: []feature.RepoMeta{{
			Name:         repoName,
			WorktreePath: filepath.ToSlash(worktreeRel),
		}},
	}); err != nil {
		t.Fatal(err)
	}

	if err := Init(root, config.Default(root, "test"), featureName, PrepareOptions{}); err != nil {
		t.Fatal(err)
	}
	meta := LoadPrepareMetadata(root, featureName)
	index := preparedFileIndex(meta, repoName)
	entry, ok := index["file.txt"]
	if !ok {
		t.Fatalf("file index missing file.txt: %#v", index)
	}
	if entry.Agent.Hash == "" || entry.Worktree.Hash == "" {
		t.Fatalf("file index hashes are empty: %#v", entry)
	}
	if entry.Agent.Hash != entry.Worktree.Hash {
		t.Fatalf("unmasked prepared hashes differ: %#v", entry)
	}
}
