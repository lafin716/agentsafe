package feature

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/agentsafe/agentsafe/internal/config"
)

func TestFolderKeyFallsBackToName(t *testing.T) {
	if got := (Metadata{Name: "테스트이름"}).FolderKey(); got != "테스트이름" {
		t.Errorf("FolderKey() with empty Key = %q, want Name", got)
	}
	if got := (Metadata{Name: "테스트이름", Key: "feat-abc123"}).FolderKey(); got != "feat-abc123" {
		t.Errorf("FolderKey() = %q, want Key", got)
	}
}

func TestUniqueFeatureKeyUsesFeatHashAndResolvesCollisions(t *testing.T) {
	root := t.TempDir()
	name := "테스트2"
	want := config.FeatureKey(name)

	first := uniqueFeatureKey(root, name)
	if first != want {
		t.Fatalf("uniqueFeatureKey returned %q, want %q", first, want)
	}
	// Simulate the worktree folder being created for the first feature.
	if err := os.MkdirAll(filepath.Join(root, "feature", first), 0o755); err != nil {
		t.Fatal(err)
	}

	second := uniqueFeatureKey(root, name)
	if second == first {
		t.Errorf("expected a distinct key on collision, got %q twice", first)
	}
	if second != first+"-2" {
		t.Errorf("second key = %q, want %q", second, first+"-2")
	}
}
