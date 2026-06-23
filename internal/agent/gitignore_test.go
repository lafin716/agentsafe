package agent

import (
	"path/filepath"
	"sort"
	"testing"

	"github.com/agentsafe/agentsafe/internal/config"
	"github.com/agentsafe/agentsafe/internal/feature"
)

// TestGitignoreMatchSemantics exercises the in-process pattern engine: any-depth
// basename matching, anchoring, directory-only rules, globs, and negation.
func TestGitignoreMatchSemantics(t *testing.T) {
	scopes := []scopedRules{{scope: "", rules: compileGitignore([]string{
		"# a comment",
		"",
		"build/",
		"*.log",
		"/root-only",
		"src/generated",
		"**/coverage",
		"node_modules/",
		"*.tmp",
		"!keep.tmp",
	})}}
	cases := []struct {
		path  string
		isDir bool
		want  bool
	}{
		{"build", true, true},               // build/ at the root
		{"a/build", true, true},             // build/ at any depth
		{"a/b/build", true, true},           // build/ deeply nested
		{"build", false, false},             // build/ is directory-only: a file named build is kept
		{"app.log", false, true},            // *.log
		{"a/b/app.log", false, true},        // *.log at any depth
		{"root-only", false, true},          // /root-only anchored at root
		{"a/root-only", false, false},       // ...so not matched deeper
		{"src/generated", false, true},      // anchored path (not dir-only) matches a file too
		{"a/src/generated", false, false},   // anchored to root only
		{"coverage", true, true},            // **/coverage at the root
		{"x/y/coverage", true, true},        // **/coverage nested
		{"node_modules", true, true},        // node_modules/ at root
		{"a/node_modules", true, true},      // node_modules/ nested
		{"foo.tmp", false, true},            // *.tmp
		{"keep.tmp", false, false},          // !keep.tmp re-includes
		{"readme.md", false, false},         // unmatched
	}
	for _, c := range cases {
		if got := gitignoreMatch(scopes, c.path, c.isDir); got != c.want {
			t.Errorf("gitignoreMatch(%q, dir=%v) = %v, want %v", c.path, c.isDir, got, c.want)
		}
	}
}

// TestGitignoreNestedScopeOverride checks that a deeper .gitignore overrides a
// shallower one (a child !rule re-includes a file the root would ignore).
func TestGitignoreNestedScopeOverride(t *testing.T) {
	scopes := []scopedRules{
		{scope: "", rules: compileGitignore([]string{"*.log"})},
		{scope: "frontend", rules: compileGitignore([]string{"!important.log"})},
	}
	if !gitignoreMatch(scopes[:1], "app.log", false) {
		t.Error("root app.log should be ignored")
	}
	if !gitignoreMatch(scopes, "frontend/other.log", false) {
		t.Error("frontend/other.log should be ignored by the root *.log")
	}
	if gitignoreMatch(scopes, "frontend/important.log", false) {
		t.Error("frontend/important.log should be re-included by the nested !rule")
	}
}

// TestGitIgnoredPatternsEndToEnd walks a tree with a root and a nested
// .gitignore and asserts the collected ignored paths, including pruning.
func TestGitIgnoredPatternsEndToEnd(t *testing.T) {
	root := t.TempDir()
	writeIndexedFile(t, filepath.Join(root, ".gitignore"), "build/\n*.log\n")
	writeIndexedFile(t, filepath.Join(root, "frontend", ".gitignore"), "dist/\n!important.log\n")
	writeIndexedFile(t, filepath.Join(root, "app.log"), "x")
	writeIndexedFile(t, filepath.Join(root, "build", "artifact.o"), "x")
	writeIndexedFile(t, filepath.Join(root, "src", "main.go"), "x")
	writeIndexedFile(t, filepath.Join(root, "frontend", "app.js"), "x")
	writeIndexedFile(t, filepath.Join(root, "frontend", "dist", "bundle.js"), "x")
	writeIndexedFile(t, filepath.Join(root, "frontend", "important.log"), "x")
	writeIndexedFile(t, filepath.Join(root, "frontend", "other.log"), "x")

	got, err := gitIgnoredPatterns(root, []string{root})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(got)
	want := []string{"app.log", "build", "frontend/dist", "frontend/other.log"}
	if len(got) != len(want) {
		t.Fatalf("ignored = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ignored = %v, want %v", got, want)
		}
	}
}

// prepareBuiltFeature sets up a single-repo feature whose worktree has a
// .gitignore ignoring build/. It prepares the agent copy, then simulates a coding
// agent running a build (nested build/ trees appear only in the agent copy) plus
// one legitimate source edit. No real Git repo is needed. Returns the agent dir.
func prepareBuiltFeature(t *testing.T, root string, cfg config.Config) string {
	t.Helper()
	featureName, repoName := "demo", "repo"
	worktreeRel := filepath.ToSlash(filepath.Join("feature", featureName, repoName))
	worktree := filepath.Join(root, filepath.FromSlash(worktreeRel))

	writeIndexedFile(t, filepath.Join(worktree, ".gitignore"), "build/\n")
	writeIndexedFile(t, filepath.Join(worktree, "src", "main.go"), "package main\n")

	if err := feature.Save(root, feature.Metadata{
		Name: featureName, Key: featureName,
		Repositories: []feature.RepoMeta{{Name: repoName, WorktreePath: worktreeRel}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := Init(root, cfg, featureName, PrepareOptions{}); err != nil {
		t.Fatal(err)
	}

	agentDir := config.AgentPath(root, featureName, repoName)
	writeIndexedFile(t, filepath.Join(agentDir, "src", "build", "out.o"), "obj")
	writeIndexedFile(t, filepath.Join(agentDir, "build", "app.jar"), "jar")
	writeIndexedFile(t, filepath.Join(agentDir, "src", "main.go"), "package main // edited\n")
	return agentDir
}

// With RespectGitignore on (the default), agent build output is excluded from the
// diff via the worktree's .gitignore while a genuine source edit is preserved.
func TestDiffHonorsFeatureGitignore(t *testing.T) {
	root := t.TempDir()
	cfg := config.Default(root, "ws")
	cfg.Agent.DefaultExclude = nil // isolate: prove the .gitignore does the filtering

	prepareBuiltFeature(t, root, cfg)

	changes, err := Diff(root, cfg, "demo", "repo")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]ChangeType{}
	for _, c := range changes["repo"] {
		got[c.Path] = c.Type
	}
	if len(got) != 1 || got["src/main.go"] != Modified {
		t.Fatalf("expected only src/main.go modified, got %#v", got)
	}
}

// With RespectGitignore disabled, behavior reverts: the gitignored build output
// reappears as ADDED changes (DefaultExclude is cleared so nothing else masks it).
func TestDiffIgnoresFeatureGitignoreWhenDisabled(t *testing.T) {
	root := t.TempDir()
	cfg := config.Default(root, "ws")
	cfg.Agent.DefaultExclude = nil
	off := false
	cfg.Agent.RespectGitignore = &off

	prepareBuiltFeature(t, root, cfg)

	changes, err := Diff(root, cfg, "demo", "repo")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]ChangeType{}
	for _, c := range changes["repo"] {
		got[c.Path] = c.Type
	}
	for _, p := range []string{"build/app.jar", "src/build/out.o"} {
		if got[p] != Added {
			t.Errorf("expected %q to appear as ADDED when gitignore is off, got %#v", p, got)
		}
	}
	if got["src/main.go"] != Modified {
		t.Errorf("source edit should still sync, got %#v", got)
	}
}
