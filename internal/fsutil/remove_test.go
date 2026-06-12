package fsutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestForceRemoveAllRemovesReadOnlyFiles(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "clone")
	if err := os.MkdirAll(filepath.Join(target, "objects"), 0o755); err != nil {
		t.Fatal(err)
	}
	ro := filepath.Join(target, "objects", "pack.idx")
	if err := os.WriteFile(ro, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Mark the file read-only, mimicking Git pack/idx files on Windows.
	if err := os.Chmod(ro, 0o400); err != nil {
		t.Fatal(err)
	}

	if err := ForceRemoveAll(target); err != nil {
		t.Fatalf("ForceRemoveAll: %v", err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("target still exists, stat err = %v", err)
	}
}

func TestIsEmptyDir(t *testing.T) {
	dir := t.TempDir()
	empty, err := IsEmptyDir(dir)
	if err != nil || !empty {
		t.Fatalf("expected empty dir, got empty=%v err=%v", empty, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "f"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	empty, err = IsEmptyDir(dir)
	if err != nil || empty {
		t.Fatalf("expected non-empty dir, got empty=%v err=%v", empty, err)
	}
	if _, err := IsEmptyDir(filepath.Join(dir, "missing")); err == nil {
		t.Fatal("expected error for missing dir")
	}
}
