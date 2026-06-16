package workspacebundle

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentsafe/agentsafe/internal/config"
	"github.com/agentsafe/agentsafe/internal/wttemplate"
)

func TestExportImportWorkspaceBundle(t *testing.T) {
	root := t.TempDir()
	cfg, err := config.InitWorkspace(root, "source")
	if err != nil {
		t.Fatal(err)
	}
	cfg.Repositories = []config.Repository{{Name: "api", URL: "https://example.com/api.git", DefaultBranch: "main"}}
	if err := config.Save(root, cfg); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "main", "api"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "feature", "demo", "api"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "agent", "demo", "api"), 0o755); err != nil {
		t.Fatal(err)
	}
	templateSrc := filepath.Join(root, "CLAUDE.md")
	if err := os.WriteFile(templateSrc, []byte("template"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := wttemplate.ImportFiles(root, []string{templateSrc}); err != nil {
		t.Fatal(err)
	}

	zipPath := filepath.Join(t.TempDir(), "bundle.zip")
	if err := Export(root, zipPath); err != nil {
		t.Fatal(err)
	}
	assertZipExcludes(t, zipPath, []string{"main/", "feature/", "agent/", ".agentsafe/features/", ".agentsafe/sessions/", ".agentsafe/history/"})

	dst := filepath.Join(t.TempDir(), "imported")
	imported, err := Import(zipPath, dst)
	if err != nil {
		t.Fatal(err)
	}
	if imported.Workspace.Root != dst {
		t.Fatalf("root = %q, want %q", imported.Workspace.Root, dst)
	}
	if len(imported.Repositories) != 1 || imported.Repositories[0].Name != "api" {
		t.Fatalf("repositories not imported: %#v", imported.Repositories)
	}
	if _, err := os.Stat(filepath.Join(dst, "agentsafe.yaml")); err != nil {
		t.Fatalf("security file missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, ".agentsafe", "worktree-templates", "templates.json")); err != nil {
		t.Fatalf("template metadata missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, "main", "api")); !os.IsNotExist(err) {
		t.Fatalf("source clone should not be imported, err=%v", err)
	}
}

func TestImportRejectsExistingWorkspaceAndNonEmptyTarget(t *testing.T) {
	root := t.TempDir()
	if _, err := config.InitWorkspace(root, "source"); err != nil {
		t.Fatal(err)
	}
	zipPath := filepath.Join(t.TempDir(), "bundle.zip")
	if err := Export(root, zipPath); err != nil {
		t.Fatal(err)
	}
	existing := t.TempDir()
	if _, err := config.InitWorkspace(existing, "existing"); err != nil {
		t.Fatal(err)
	}
	if _, err := Import(zipPath, existing); err == nil {
		t.Fatal("expected existing workspace import rejection")
	}
	nonEmpty := t.TempDir()
	if err := os.WriteFile(filepath.Join(nonEmpty, "note.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Import(zipPath, nonEmpty); err == nil {
		t.Fatal("expected non-empty target import rejection")
	}
}

func assertZipExcludes(t *testing.T, zipPath string, prefixes []string) {
	t.Helper()
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	for _, f := range zr.File {
		name := filepath.ToSlash(f.Name)
		for _, prefix := range prefixes {
			if strings.HasPrefix(name, prefix) {
				t.Fatalf("zip unexpectedly contains %s", name)
			}
		}
	}
}
