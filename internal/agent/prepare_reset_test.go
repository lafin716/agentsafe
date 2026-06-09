package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResetTargetBackup(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "repo")
	if err := os.MkdirAll(target, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "f.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	resetTarget(target, true)

	// target is recreated empty
	if entries, _ := os.ReadDir(target); len(entries) != 0 {
		t.Errorf("target should be empty after reset, got %d entries", len(entries))
	}
	// a .bak- sibling holds the old content
	siblings, _ := os.ReadDir(dir)
	found := false
	for _, e := range siblings {
		if e.IsDir() && filepathBaseHasBak(e.Name()) {
			found = true
			if _, err := os.Stat(filepath.Join(dir, e.Name(), "f.txt")); err != nil {
				t.Errorf("backup missing original file: %v", err)
			}
		}
	}
	if !found {
		t.Errorf("expected a .bak- backup directory")
	}
}

func TestResetTargetDelete(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "repo")
	if err := os.MkdirAll(target, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "f.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	resetTarget(target, false)

	if entries, _ := os.ReadDir(target); len(entries) != 0 {
		t.Errorf("target should be empty after reset, got %d entries", len(entries))
	}
	// no backup sibling created
	siblings, _ := os.ReadDir(dir)
	for _, e := range siblings {
		if e.IsDir() && filepathBaseHasBak(e.Name()) {
			t.Errorf("no backup expected, found %s", e.Name())
		}
	}
}

func filepathBaseHasBak(name string) bool {
	return len(name) >= 5 && (name != "repo") && containsBak(name)
}

func containsBak(s string) bool {
	for i := 0; i+5 <= len(s); i++ {
		if s[i:i+5] == ".bak-" {
			return true
		}
	}
	return false
}
