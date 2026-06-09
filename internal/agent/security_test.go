package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/agentsafe/agentsafe/internal/config"
)

func TestLoadSecurityUnified(t *testing.T) {
	dir := t.TempDir()
	yaml := `ignore:
  - .env
  - "*.pem"
mask:
  - name: AWS
    type: regex
    pattern: AKIA[0-9A-Z]{16}
    replacement: __MASKED__
`
	if err := os.WriteFile(filepath.Join(dir, "agentsafe.yaml"), []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}
	sf := LoadSecurity(config.Config{}, dir)
	if len(sf.Ignore) != 2 || sf.Ignore[0] != ".env" || sf.Ignore[1] != "*.pem" {
		t.Fatalf("unexpected ignore: %v", sf.Ignore)
	}
	if len(sf.Mask) != 1 || sf.Mask[0].Name != "AWS" || sf.Mask[0].Type != "regex" {
		t.Fatalf("unexpected mask: %+v", sf.Mask)
	}
}

func TestLoadSecurityLegacyFallback(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".agentignore"), []byte("# c\n.env\n*.pem\n"), 0644); err != nil {
		t.Fatal(err)
	}
	maskJSON := `{"rules":[{"name":"AWS","type":"regex","pattern":"AKIA[0-9A-Z]{16}","replacement":"__MASKED__"}]}`
	if err := os.WriteFile(filepath.Join(dir, "mask.json"), []byte(maskJSON), 0644); err != nil {
		t.Fatal(err)
	}
	sf := LoadSecurity(config.Config{}, dir)
	// LoadIgnoreFiles preserves all lines (comments stripped later by the matcher).
	found := map[string]bool{}
	for _, p := range sf.Ignore {
		found[p] = true
	}
	if !found[".env"] || !found["*.pem"] {
		t.Fatalf("legacy ignore not loaded: %v", sf.Ignore)
	}
	if len(sf.Mask) != 1 || sf.Mask[0].Name != "AWS" {
		t.Fatalf("legacy mask not loaded: %+v", sf.Mask)
	}
}

func TestEnsureSecurityFileMigratesLegacy(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".agentignore"), []byte(".env\n*.pem\n"), 0644); err != nil {
		t.Fatal(err)
	}
	maskJSON := `{"rules":[{"name":"AWS","type":"plain","pattern":"secret","replacement":"__MASKED__"}]}`
	if err := os.WriteFile(filepath.Join(dir, "mask.json"), []byte(maskJSON), 0644); err != nil {
		t.Fatal(err)
	}
	if err := EnsureSecurityFile(config.Config{}, dir); err != nil {
		t.Fatal(err)
	}
	unified := filepath.Join(dir, "agentsafe.yaml")
	if _, err := os.Stat(unified); err != nil {
		t.Fatalf("expected agentsafe.yaml to be created: %v", err)
	}
	// Legacy files are left in place (non-destructive).
	if _, err := os.Stat(filepath.Join(dir, ".agentignore")); err != nil {
		t.Fatalf("legacy ignore should remain: %v", err)
	}
	sf, err := LoadSecurityFile(unified)
	if err != nil {
		t.Fatal(err)
	}
	if len(sf.Mask) != 1 || sf.Mask[0].Name != "AWS" {
		t.Fatalf("migrated mask wrong: %+v", sf.Mask)
	}
}

func TestEnsureSecurityFileIdempotentAndNoLegacy(t *testing.T) {
	dir := t.TempDir()
	// No files at all: must not create anything.
	if err := EnsureSecurityFile(config.Config{}, dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "agentsafe.yaml")); !os.IsNotExist(err) {
		t.Fatalf("should not create agentsafe.yaml without legacy files")
	}

	// Existing unified file: must be left untouched.
	unified := filepath.Join(dir, "agentsafe.yaml")
	original := "ignore:\n  - keep-me\n"
	if err := os.WriteFile(unified, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".agentignore"), []byte(".env\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := EnsureSecurityFile(config.Config{}, dir); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(unified)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != original {
		t.Fatalf("existing unified file was overwritten: %q", string(b))
	}
}
