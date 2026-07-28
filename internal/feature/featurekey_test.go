package feature

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/agentsafe/agentsafe/internal/config"
)

// Key 가 없는 옛 메타데이터는 Name 을 폴더 키로 쓰고, Key 가 있으면 그쪽이 우선한다.
func TestFolderKeyFallsBackToName(t *testing.T) {
	if got := (Metadata{Name: "테스트이름"}).FolderKey(); got != "테스트이름" {
		t.Errorf("FolderKey() with empty Key = %q, want Name", got)
	}
	if got := (Metadata{Name: "테스트이름", Key: "feat-abc123"}).FolderKey(); got != "feat-abc123" {
		t.Errorf("FolderKey() = %q, want Key", got)
	}
}

// 한글처럼 ASCII 가 아닌 이름도 config.FeatureKey 로 안전한 폴더 키가 되고, 같은
// 키를 쓰는 폴더가 이미 있으면 `-2` 접미사를 붙여 충돌을 피한다.
func TestUniqueFeatureKeyUsesFeatHashAndResolvesCollisions(t *testing.T) {
	root := t.TempDir()
	name := "테스트2"
	want := config.FeatureKey(name)

	first := uniqueFeatureKey(root, name)
	if first != want {
		t.Fatalf("uniqueFeatureKey returned %q, want %q", first, want)
	}
	// 첫 feature 의 worktree 폴더가 만들어진 상황을 흉내 낸다.
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
