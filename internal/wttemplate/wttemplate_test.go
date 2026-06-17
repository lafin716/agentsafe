package wttemplate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	aggit "github.com/agentsafe/agentsafe/internal/git"
)

func TestApplyTargetsAndOverwrite(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "CLAUDE.md")
	if err := os.WriteFile(src, []byte("template"), 0o644); err != nil {
		t.Fatal(err)
	}
	added, err := ImportFiles(root, []string{src})
	if err != nil {
		t.Fatal(err)
	}
	item := added[0]
	item.TargetMode = TargetSelectedRepos
	item.RepoNames = []string{"api"}
	if err := Update(root, item); err != nil {
		t.Fatal(err)
	}
	api := filepath.Join(root, "feature", "demo", "api")
	web := filepath.Join(root, "feature", "demo", "web")
	if err := os.MkdirAll(api, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(web, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Apply(root, "demo", []Repo{{Name: "api", WorktreePath: api}, {Name: "web", WorktreePath: web}}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(api, "CLAUDE.md")); err != nil {
		t.Fatalf("selected repo did not receive template: %v", err)
	}
	if _, err := os.Stat(filepath.Join(web, "CLAUDE.md")); !os.IsNotExist(err) {
		t.Fatalf("unselected repo received template, err=%v", err)
	}

	if err := os.WriteFile(filepath.Join(api, "CLAUDE.md"), []byte("local"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Apply(root, "demo", []Repo{{Name: "api", WorktreePath: api}}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(api, "CLAUDE.md"))
	if string(b) != "local" {
		t.Fatalf("expected no overwrite, got %q", string(b))
	}
	item.Overwrite = true
	if err := Update(root, item); err != nil {
		t.Fatal(err)
	}
	if err := Apply(root, "demo", []Repo{{Name: "api", WorktreePath: api}}); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(filepath.Join(api, "CLAUDE.md"))
	if string(b) != "template" {
		t.Fatalf("expected overwrite, got %q", string(b))
	}
}

func TestImportFolderCopiesFolderItselfToFeatureRoot(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "tools")
	if err := os.MkdirAll(filepath.Join(src, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "bin", "run.sh"), []byte("run"), 0o755); err != nil {
		t.Fatal(err)
	}
	item, err := ImportFolder(root, src)
	if err != nil {
		t.Fatal(err)
	}
	item.TargetMode = TargetFeatureRoot
	if err := Update(root, item); err != nil {
		t.Fatal(err)
	}
	if err := Apply(root, "demo", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "feature", "demo", "tools", "bin", "run.sh")); err != nil {
		t.Fatalf("folder template not copied to feature root: %v", err)
	}
}

func TestImportPathsAcceptsFilesAndFolders(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "AGENTS.md")
	if err := os.WriteFile(file, []byte("file"), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "tools")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "run.sh"), []byte("run"), 0o755); err != nil {
		t.Fatal(err)
	}
	added, err := ImportPaths(root, []string{file, dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(added) != 2 {
		t.Fatalf("expected two imported templates, got %d", len(added))
	}
	if _, err := os.Stat(filepath.Join(FilesDir(root), added[0].ID, "AGENTS.md")); err != nil {
		t.Fatalf("file template missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(FilesDir(root), added[1].ID, "tools", "run.sh")); err != nil {
		t.Fatalf("folder template missing: %v", err)
	}
}

func TestReadWriteTemplateFile(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "AGENTS.md")
	if err := os.WriteFile(src, []byte("initial"), 0o644); err != nil {
		t.Fatal(err)
	}
	added, err := ImportFiles(root, []string{src})
	if err != nil {
		t.Fatal(err)
	}
	got, err := ReadTemplateFile(root, added[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if got != "initial" {
		t.Fatalf("expected initial content, got %q", got)
	}
	if err := WriteTemplateFile(root, added[0].ID, "edited"); err != nil {
		t.Fatal(err)
	}
	got, err = ReadTemplateFile(root, added[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if got != "edited" {
		t.Fatalf("expected edited content, got %q", got)
	}
}

func TestReadTemplateFileRejectsFolderTemplate(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "tools")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "run.sh"), []byte("run"), 0o644); err != nil {
		t.Fatal(err)
	}
	item, err := ImportFolder(root, src)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ReadTemplateFile(root, item.ID); err == nil {
		t.Fatal("expected folder template read to fail")
	}
}

func TestApplyAgentTargets(t *testing.T) {
	root := t.TempDir()
	rootFile := filepath.Join(root, "AGENTS.md")
	repoFile := filepath.Join(root, "CLAUDE.md")
	if err := os.WriteFile(rootFile, []byte("agent root"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(repoFile, []byte("agent repo"), 0o644); err != nil {
		t.Fatal(err)
	}
	added, err := ImportFiles(root, []string{rootFile, repoFile})
	if err != nil {
		t.Fatal(err)
	}
	added[0].TargetMode = TargetAgentRoot
	if err := Update(root, added[0]); err != nil {
		t.Fatal(err)
	}
	added[1].TargetMode = TargetAgentSelectedRepos
	added[1].RepoNames = []string{"api"}
	if err := Update(root, added[1]); err != nil {
		t.Fatal(err)
	}
	api := filepath.Join(root, "agent", "demo", "api")
	web := filepath.Join(root, "agent", "demo", "web")
	if err := ApplyAgent(root, "demo", []Repo{{Name: "api", WorktreePath: api}, {Name: "web", WorktreePath: web}}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "agent", "demo", "AGENTS.md")); err != nil {
		t.Fatalf("agent root template missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(api, "CLAUDE.md")); err != nil {
		t.Fatalf("selected agent repo template missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(web, "CLAUDE.md")); !os.IsNotExist(err) {
		t.Fatalf("unselected agent repo received template, err=%v", err)
	}
}

func TestApplyWorkspaceRootTargetsOnlyWorkspaceRoot(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "ROOT.md")
	if err := os.WriteFile(src, []byte("root"), 0o644); err != nil {
		t.Fatal(err)
	}
	added, err := ImportFiles(root, []string{src})
	if err != nil {
		t.Fatal(err)
	}
	item := added[0]
	item.TargetMode = TargetWorkspaceRoot
	if err := Update(root, item); err != nil {
		t.Fatal(err)
	}
	if err := Apply(root, "demo", []Repo{{Name: "api", WorktreePath: filepath.Join(root, "feature", "demo", "api")}}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "feature", "demo", "ROOT.md")); !os.IsNotExist(err) {
		t.Fatalf("workspace root template was applied to feature root, err=%v", err)
	}
	if err := ApplyWorkspaceRoot(root); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "ROOT.md")); err != nil {
		t.Fatalf("workspace root template missing: %v", err)
	}
}

func TestClearDeletesTemplateStore(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "AGENTS.md")
	if err := os.WriteFile(src, []byte("file"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ImportFiles(root, []string{src}); err != nil {
		t.Fatal(err)
	}
	if err := Clear(root); err != nil {
		t.Fatal(err)
	}
	items, err := List(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("templates after clear = %d, want 0", len(items))
	}
}

func TestAgentIgnorePatternsIncludesAgentRepoTemplates(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "CLAUDE.md")
	dir := filepath.Join(root, "tools")
	if err := os.WriteFile(file, []byte("file"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "run.sh"), []byte("run"), 0o644); err != nil {
		t.Fatal(err)
	}
	added, err := ImportPaths(root, []string{file, dir})
	if err != nil {
		t.Fatal(err)
	}
	added[0].TargetMode = TargetAgentAllRepos
	if err := Update(root, added[0]); err != nil {
		t.Fatal(err)
	}
	added[1].TargetMode = TargetAgentSelectedRepos
	added[1].RepoNames = []string{"api"}
	if err := Update(root, added[1]); err != nil {
		t.Fatal(err)
	}
	api, err := AgentIgnorePatterns(root, "api")
	if err != nil {
		t.Fatal(err)
	}
	if !containsString(api, "/CLAUDE.md") || !containsString(api, "/tools/") {
		t.Fatalf("api ignore patterns = %#v, want file and folder templates", api)
	}
	web, err := AgentIgnorePatterns(root, "web")
	if err != nil {
		t.Fatal(err)
	}
	if !containsString(web, "/CLAUDE.md") || containsString(web, "/tools/") {
		t.Fatalf("web ignore patterns = %#v, want all-repo only", web)
	}
}

func TestWorktreeRepoTemplatesAreAddedToGitInfoExclude(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	testGit(t, "", "init", "-b", "main", repo)
	testGit(t, repo, "config", "user.email", "test@example.com")
	testGit(t, repo, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("readme"), 0o644); err != nil {
		t.Fatal(err)
	}
	testGit(t, repo, "add", ".")
	testGit(t, repo, "commit", "-m", "initial")

	src := filepath.Join(root, "CLAUDE.md")
	if err := os.WriteFile(src, []byte("template"), 0o644); err != nil {
		t.Fatal(err)
	}
	added, err := ImportFiles(root, []string{src})
	if err != nil {
		t.Fatal(err)
	}
	item := added[0]
	item.TargetMode = TargetAllRepos
	if err := Update(root, item); err != nil {
		t.Fatal(err)
	}
	if err := Apply(root, "demo", []Repo{{Name: "repo", WorktreePath: repo}}); err != nil {
		t.Fatal(err)
	}
	status, err := aggit.StatusShort(repo)
	if err != nil {
		t.Fatal(err)
	}
	if status != "" {
		t.Fatalf("template file should be git-ignored, status=%q", status)
	}
	excludePath, err := aggit.Output(repo, "rev-parse", "--git-path", "info/exclude")
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(excludePath) {
		excludePath = filepath.Join(repo, excludePath)
	}
	b, err := os.ReadFile(excludePath)
	if err != nil {
		t.Fatal(err)
	}
	if !containsString([]string{string(b)}, "/CLAUDE.md") {
		t.Fatalf("exclude missing template path: %s", string(b))
	}
}

func containsString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle || (len(values) == 1 && strings.Contains(value, needle)) {
			return true
		}
	}
	return false
}

func testGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	if _, err := aggit.Run(dir, args...); err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
}
