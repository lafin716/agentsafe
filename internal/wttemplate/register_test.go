package wttemplate

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRegisterPathFileInfersRepoTarget(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "main", "api", "CLAUDE.md")
	writeRegisterTestFile(t, src, "template")

	item, err := RegisterPath(root, src, RegisterOptions{Enabled: true, WorkspaceRepos: []string{"api"}})
	if err != nil {
		t.Fatal(err)
	}
	if item.Name != "CLAUDE.md" || item.SourcePath != src {
		t.Fatalf("unexpected template %+v", item)
	}
	if item.TargetMode != TargetSelectedRepos || strings.Join(item.RepoNames, ",") != "api" {
		t.Fatalf("expected the api repository destination, got %s %v", item.TargetMode, item.RepoNames)
	}
	if !item.Enabled || item.Overwrite {
		t.Fatalf("expected an enabled template that does not overwrite, got %+v", item)
	}
	if _, err := os.Stat(filepath.Join(FilesDir(root), item.ID, "CLAUDE.md")); err != nil {
		t.Fatalf("source was not copied into the store: %v", err)
	}

	stored, err := List(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 || stored[0].TargetMode != TargetSelectedRepos {
		t.Fatalf("destination was not persisted: %+v", stored)
	}
}

func TestRegisterPathFolderCopiesTree(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "feature", "feat-1", "tools")
	writeRegisterTestFile(t, filepath.Join(src, "bin", "run.sh"), "run")

	item, err := RegisterPath(root, src, RegisterOptions{Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if item.Name != "tools" {
		t.Fatalf("expected the folder name, got %q", item.Name)
	}
	if _, err := os.Stat(filepath.Join(FilesDir(root), item.ID, "tools", "bin", "run.sh")); err != nil {
		t.Fatalf("folder was not copied into the store: %v", err)
	}
}

func TestRegisterPathRejectsAlreadyRegisteredSource(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "AGENTS.md")
	writeRegisterTestFile(t, src, "template")

	if _, err := RegisterPath(root, src, RegisterOptions{Enabled: true}); err != nil {
		t.Fatal(err)
	}
	_, err := RegisterPath(root, src, RegisterOptions{Enabled: true})
	if err == nil {
		t.Fatal("expected the second registration to fail")
	}
	if !strings.Contains(err.Error(), "AGENTS.md") {
		t.Fatalf("error should name the existing template, got %v", err)
	}
	stored, err := List(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 {
		t.Fatalf("expected the store to hold one template, got %d", len(stored))
	}
}

// A source registered once must stay registered however its path is spelled,
// otherwise the duplicate guard is trivially bypassed.
func TestFindBySourcePathNormalizesSpelling(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "tools")
	writeRegisterTestFile(t, filepath.Join(src, "run.sh"), "run")
	if _, err := RegisterPath(root, src, RegisterOptions{Enabled: true}); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		path string
		want bool
	}{
		{"exact", src, true},
		{"trailing separator", src + string(filepath.Separator), true},
		{"redundant elements", filepath.Join(src, "..", "tools"), true},
		{"upper case", filepath.Join(root, "TOOLS"), runtime.GOOS == "windows"},
		{"other path", filepath.Join(root, "other"), false},
	}
	for _, tc := range cases {
		_, found, err := FindBySourcePath(root, tc.path)
		if err != nil {
			t.Fatalf("FindBySourcePath(%s): %v", tc.name, err)
		}
		if found != tc.want {
			t.Errorf("FindBySourcePath(%s) found = %v, want %v", tc.name, found, tc.want)
		}
	}
}

func TestFindBySourcePathWithoutStore(t *testing.T) {
	root := t.TempDir()
	item, found, err := FindBySourcePath(root, filepath.Join(root, "AGENTS.md"))
	if err != nil {
		t.Fatalf("expected no error without a store, got %v", err)
	}
	if found || item.ID != "" {
		t.Fatalf("expected no match without a store, got %+v", item)
	}
}

func TestRegisterPathKeepsExplicitOptions(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "main", "api", "AGENTS.md")
	writeRegisterTestFile(t, src, "template")

	item, err := RegisterPath(root, src, RegisterOptions{
		TargetMode:     TargetAgentSelectedRepos,
		RepoNames:      []string{"web", "web", ""},
		Overwrite:      true,
		WorkspaceRepos: []string{"api", "web"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if item.TargetMode != TargetAgentSelectedRepos || strings.Join(item.RepoNames, ",") != "web" {
		t.Fatalf("explicit destination was not applied: %s %v", item.TargetMode, item.RepoNames)
	}
	if !item.Overwrite || item.Enabled {
		t.Fatalf("expected an overwriting, disabled template, got %+v", item)
	}
	stored, err := List(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 || stored[0].TargetMode != TargetAgentSelectedRepos || stored[0].Enabled {
		t.Fatalf("explicit options were not persisted: %+v", stored)
	}
}

func TestRegisterPathMissingSource(t *testing.T) {
	root := t.TempDir()
	if _, err := RegisterPath(root, filepath.Join(root, "absent.md"), RegisterOptions{Enabled: true}); err == nil {
		t.Fatal("expected an error for a missing path")
	}
	if _, err := RegisterPath(root, "  ", RegisterOptions{Enabled: true}); err == nil {
		t.Fatal("expected an error for a blank path")
	}
}

func TestInferTarget(t *testing.T) {
	root := t.TempDir()
	repos := []string{"api", "web"}

	cases := []struct {
		rel       string
		wantMode  string
		wantRepos string
	}{
		{"agentsafe.yaml", TargetWorkspaceRoot, ""},
		{"main", TargetWorkspaceRoot, ""},
		{filepath.Join("main", "api"), TargetSelectedRepos, "api"},
		{filepath.Join("main", "api", "src", "app.go"), TargetSelectedRepos, "api"},
		{filepath.Join("main", "other"), TargetWorkspaceRoot, ""},
		{filepath.Join("feature", "feat-1"), TargetFeatureRoot, ""},
		{filepath.Join("feature", "feat-1", "CLAUDE.md"), TargetFeatureRoot, ""},
		{filepath.Join("feature", "feat-1", "web"), TargetSelectedRepos, "web"},
		{filepath.Join("feature", "feat-1", "web", "docs", "guide.md"), TargetSelectedRepos, "web"},
		{filepath.Join("feature", "feat-1", "tools", "run.sh"), TargetWorkspaceRoot, ""},
		{filepath.Join("agent", "feat-1"), TargetAgentRoot, ""},
		{filepath.Join("agent", "feat-1", "AGENTS.md"), TargetAgentRoot, ""},
		{filepath.Join("agent", "feat-1", "api"), TargetAgentSelectedRepos, "api"},
		{filepath.Join("agent", "feat-1", "api", "src", "app.go"), TargetAgentSelectedRepos, "api"},
		{filepath.Join("other", "thing.txt"), TargetWorkspaceRoot, ""},
	}
	for _, tc := range cases {
		mode, names := InferTarget(root, filepath.Join(root, tc.rel), repos)
		if mode != tc.wantMode || strings.Join(names, ",") != tc.wantRepos {
			t.Errorf("InferTarget(%s) = %s %v, want %s %q", tc.rel, mode, names, tc.wantMode, tc.wantRepos)
		}
	}

	for _, path := range []string{root, t.TempDir(), filepath.Join(root, "..", "elsewhere", "file.txt")} {
		mode, names := InferTarget(root, path, repos)
		if mode != TargetWorkspaceRoot || len(names) != 0 {
			t.Errorf("InferTarget(%s) = %s %v, want the workspace root", path, mode, names)
		}
	}
}

func writeRegisterTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
