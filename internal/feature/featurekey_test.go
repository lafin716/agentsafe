package feature

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFolderKeyFallsBackToName(t *testing.T) {
	if got := (Metadata{Name: "테스트이름"}).FolderKey(); got != "테스트이름" {
		t.Errorf("FolderKey() with empty Key = %q, want Name", got)
	}
	if got := (Metadata{Name: "테스트이름", Key: "feature-abc123"}).FolderKey(); got != "feature-abc123" {
		t.Errorf("FolderKey() = %q, want Key", got)
	}
}

func TestUniqueFeatureKeyResolvesCollisions(t *testing.T) {
	root := t.TempDir()

	first := uniqueFeatureKey(root, "테스트2")
	if first != "테스트2" {
		t.Fatalf("uniqueFeatureKey returned %q, want original name", first)
	}
	// Simulate the worktree folder being created for the first feature.
	if err := os.MkdirAll(filepath.Join(root, "feature", first), 0o755); err != nil {
		t.Fatal(err)
	}

	second := uniqueFeatureKey(root, "테스트2")
	if second == first {
		t.Errorf("expected a distinct key on collision, got %q twice", first)
	}
	if second != first+"-2" {
		t.Errorf("second key = %q, want %q", second, first+"-2")
	}
}
