package feature

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFolderKeyFallsBackToName(t *testing.T) {
	if got := (Metadata{Name: "한글이름"}).FolderKey(); got != "한글이름" {
		t.Errorf("FolderKey() with empty Key = %q, want Name", got)
	}
	if got := (Metadata{Name: "한글이름", Key: "feature-abc123"}).FolderKey(); got != "feature-abc123" {
		t.Errorf("FolderKey() = %q, want Key", got)
	}
}

func TestUniqueFeatureKeyResolvesCollisions(t *testing.T) {
	root := t.TempDir()

	first := uniqueFeatureKey(root, "쿠폰")
	if first == "" {
		t.Fatal("uniqueFeatureKey returned empty")
	}
	// Simulate the worktree folder being created for the first feature.
	if err := os.MkdirAll(filepath.Join(root, "feature", first), 0o755); err != nil {
		t.Fatal(err)
	}

	second := uniqueFeatureKey(root, "쿠폰")
	if second == first {
		t.Errorf("expected a distinct key on collision, got %q twice", first)
	}
	if second != first+"-2" {
		t.Errorf("second key = %q, want %q", second, first+"-2")
	}
}
