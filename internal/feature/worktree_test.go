package feature

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentsafe/agentsafe/internal/config"
	aggit "github.com/agentsafe/agentsafe/internal/git"
)

func TestCreateWithExistingLocalBranchPolicies(t *testing.T) {
	t.Run("error", func(t *testing.T) {
		root, cfg := testWorkspace(t, "repo")
		repoPath := config.RepoPath(root, "repo")
		testGit(t, repoPath, "branch", "feature/demo")

		err := CreateWithOptions(root, cfg, "demo", CreateOptions{
			Base: "main", ExistingBranch: ExistingBranchError,
		})
		if err == nil || !strings.Contains(err.Error(), "choose reuse or recreate") {
			t.Fatalf("expected existing branch error, got %v", err)
		}
	})

	t.Run("reuse", func(t *testing.T) {
		root, cfg := testWorkspace(t, "repo")
		repoPath := config.RepoPath(root, "repo")
		testGit(t, repoPath, "branch", "feature/demo")

		if err := CreateWithOptions(root, cfg, "demo", CreateOptions{
			Base: "main", ExistingBranch: ExistingBranchReuse,
		}); err != nil {
			t.Fatal(err)
		}
		worktree := config.WorktreePath(root, "demo", "repo")
		if branch, err := aggit.CurrentBranch(worktree); err != nil || branch != "feature/demo" {
			t.Fatalf("branch = %q, err = %v", branch, err)
		}
		if upstream, err := aggit.Upstream(worktree, "feature/demo"); err != nil || upstream != "origin/main" {
			t.Fatalf("upstream = %q, err = %v; want origin/main", upstream, err)
		}
	})

	t.Run("recreate", func(t *testing.T) {
		root, cfg := testWorkspace(t, "repo")
		repoPath := config.RepoPath(root, "repo")
		testGit(t, repoPath, "checkout", "-b", "feature/demo")
		if err := os.WriteFile(filepath.Join(repoPath, "feature.txt"), []byte("old"), 0o644); err != nil {
			t.Fatal(err)
		}
		testGit(t, repoPath, "add", ".")
		testGit(t, repoPath, "commit", "-m", "feature commit")
		testGit(t, repoPath, "checkout", "main")

		if err := CreateWithOptions(root, cfg, "demo", CreateOptions{
			Base: "main", ExistingBranch: ExistingBranchRecreate,
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(config.WorktreePath(root, "demo", "repo"), "feature.txt")); !os.IsNotExist(err) {
			t.Fatalf("recreated branch retained old commit, stat err = %v", err)
		}
	})
}

func TestCreateTracksBaseUntilFirstPush(t *testing.T) {
	root, cfg := testWorkspace(t, "repo")
	if err := CreateWithOptions(root, cfg, "demo", CreateOptions{
		Base: "main", ExistingBranch: ExistingBranchError,
	}); err != nil {
		t.Fatal(err)
	}

	worktree := config.WorktreePath(root, "demo", "repo")
	if upstream, err := aggit.Upstream(worktree, "feature/demo"); err != nil || upstream != "origin/main" {
		t.Fatalf("upstream = %q, err = %v; want origin/main", upstream, err)
	}

	if err := Push(root, "demo", ""); err != nil {
		t.Fatal(err)
	}
	if !aggit.RemoteBranchExists(worktree, "feature/demo") {
		t.Fatal("first push did not create origin/feature/demo")
	}
	if upstream, err := aggit.Upstream(worktree, "feature/demo"); err != nil || upstream != "origin/feature/demo" {
		t.Fatalf("upstream after push = %q, err = %v; want origin/feature/demo", upstream, err)
	}
}

func TestCheckCreateReportsAllExistingBranchesWithoutCreatingWorktrees(t *testing.T) {
	root, firstCfg := testWorkspace(t, "one")
	secondRoot, secondCfg := testWorkspace(t, "two")
	if err := os.Rename(config.RepoPath(secondRoot, "two"), config.RepoPath(root, "two")); err != nil {
		t.Fatal(err)
	}
	cfg := firstCfg
	cfg.Repositories = append(cfg.Repositories, secondCfg.Repositories[0])
	testGit(t, config.RepoPath(root, "two"), "branch", "feature/demo")

	check, err := CheckCreate(root, cfg, "demo", "main")
	if err != nil {
		t.Fatal(err)
	}
	if !check.HasConflicts || check.Blocked {
		t.Fatalf("unexpected check result: %+v", check)
	}
	if len(check.Repositories) != 2 {
		t.Fatalf("repositories = %d, want 2", len(check.Repositories))
	}
	if check.Repositories[0].Conflict {
		t.Fatalf("repository one unexpectedly conflicted: %+v", check.Repositories[0])
	}
	if !check.Repositories[1].LocalBranch || !check.Repositories[1].CanReuse || !check.Repositories[1].CanRecreate {
		t.Fatalf("repository two conflict not reported correctly: %+v", check.Repositories[1])
	}
	if _, err := os.Stat(filepath.Join(root, "feature")); !os.IsNotExist(err) {
		t.Fatalf("preflight created feature directory, stat err = %v", err)
	}

	err = CreateWithOptions(root, cfg, "demo", CreateOptions{
		Base: "main", ExistingBranch: ExistingBranchError,
	})
	if err == nil || !strings.Contains(err.Error(), "choose reuse or recreate") {
		t.Fatalf("expected existing branch error, got %v", err)
	}
	if _, err := os.Stat(config.WorktreePath(root, "demo", "one")); !os.IsNotExist(err) {
		t.Fatalf("first repository worktree was partially created, stat err = %v", err)
	}
}

func TestCheckCreateBlocksBranchUsedByAnotherWorktree(t *testing.T) {
	root, cfg := testWorkspace(t, "repo")
	repoPath := config.RepoPath(root, "repo")
	other := filepath.Join(t.TempDir(), "other")
	testGit(t, repoPath, "worktree", "add", other, "-b", "feature/demo", "main")

	check, err := CheckCreate(root, cfg, "demo", "main")
	if err != nil {
		t.Fatal(err)
	}
	repo := check.Repositories[0]
	if !check.Blocked || repo.BlockedReason == "" || repo.CanReuse || repo.CanRecreate {
		t.Fatalf("checked-out branch should be blocked: %+v", check)
	}
}

func TestCreateReusesRemoteOnlyBranch(t *testing.T) {
	root, cfg := testWorkspace(t, "repo")
	repoPath := config.RepoPath(root, "repo")
	testGit(t, repoPath, "checkout", "-b", "feature/demo")
	if err := os.WriteFile(filepath.Join(repoPath, "remote.txt"), []byte("remote"), 0o644); err != nil {
		t.Fatal(err)
	}
	testGit(t, repoPath, "add", ".")
	testGit(t, repoPath, "commit", "-m", "remote feature")
	testGit(t, repoPath, "push", "-u", "origin", "feature/demo")
	testGit(t, repoPath, "checkout", "main")
	testGit(t, repoPath, "branch", "-D", "feature/demo")

	if err := CreateWithOptions(root, cfg, "demo", CreateOptions{
		Base: "main", ExistingBranch: ExistingBranchReuse,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(config.WorktreePath(root, "demo", "repo"), "remote.txt")); err != nil {
		t.Fatalf("remote branch content missing: %v", err)
	}
	worktree := config.WorktreePath(root, "demo", "repo")
	if upstream, err := aggit.Upstream(worktree, "feature/demo"); err != nil || upstream != "origin/feature/demo" {
		t.Fatalf("upstream = %q, err = %v; want origin/feature/demo", upstream, err)
	}
}

func TestConfigureRepositoryWorktreeAddAndRecreate(t *testing.T) {
	root, firstCfg := testWorkspace(t, "one")
	secondRoot, secondCfg := testWorkspace(t, "two")
	if err := os.Rename(config.RepoPath(secondRoot, "two"), config.RepoPath(root, "two")); err != nil {
		t.Fatal(err)
	}
	cfg := firstCfg
	cfg.Repositories = append(cfg.Repositories, secondCfg.Repositories[0])

	if err := CreateWithOptions(root, firstCfg, "demo", CreateOptions{
		Base: "main", ExistingBranch: ExistingBranchError,
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "agent", "demo"), 0o755); err != nil {
		t.Fatal(err)
	}
	session := struct {
		FeatureRevision int `json:"featureRevision"`
	}{FeatureRevision: 1}
	b, _ := json.Marshal(session)
	if err := os.MkdirAll(filepath.Dir(config.SessionMetaPath(root, "demo")), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config.SessionMetaPath(root, "demo"), b, 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := ConfigureRepositoryWorktree(root, cfg, "demo", "two", RepositoryWorktreeOptions{
		ExistingBranch: ExistingBranchReuse,
	}); err != nil {
		t.Fatal(err)
	}
	meta, err := Load(root, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if len(meta.Repositories) != 2 || meta.Revision != 2 {
		t.Fatalf("metadata after add = %+v", meta)
	}
	status, err := StatusData(root, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if !status.AgentNeedsPrepare {
		t.Fatal("expected agent workspace to require prepare after repository add")
	}

	secondWorktree := config.WorktreePath(root, "demo", "two")
	if err := os.WriteFile(filepath.Join(secondWorktree, "dirty.txt"), []byte("dirty"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ConfigureRepositoryWorktree(root, cfg, "demo", "two", RepositoryWorktreeOptions{
		ExistingBranch: ExistingBranchReuse, Recreate: true,
	}); err == nil || !strings.Contains(err.Error(), "uncommitted changes") {
		t.Fatalf("expected dirty worktree refusal, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(secondWorktree, "dirty.txt")); err != nil {
		t.Fatalf("dirty worktree changed after refusal: %v", err)
	}
	meta, _ = Load(root, "demo")
	if meta.Revision != 2 {
		t.Fatalf("revision changed after refused recreate: %d", meta.Revision)
	}

	if _, err := ConfigureRepositoryWorktree(root, cfg, "demo", "two", RepositoryWorktreeOptions{
		ExistingBranch: ExistingBranchReuse, Recreate: true, Force: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(secondWorktree, "dirty.txt")); !os.IsNotExist(err) {
		t.Fatalf("forced recreate retained dirty file, stat err = %v", err)
	}
	meta, _ = Load(root, "demo")
	if meta.Revision != 3 {
		t.Fatalf("revision = %d, want 3", meta.Revision)
	}
}

func TestConfigureRepositoryWorktreeAdoptsExistingTargetWorktree(t *testing.T) {
	root, cfg := testWorkspace(t, "repo")
	name := "demo"
	branch := "custom/demo"
	if err := Save(root, Metadata{Name: name, Branch: branch, Revision: 1}); err != nil {
		t.Fatal(err)
	}
	dest := config.WorktreePath(root, name, "repo")
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}
	testGit(t, config.RepoPath(root, "repo"), "worktree", "add", dest, "-b", branch, "main")
	_, _ = aggit.Run(dest, "branch", "--unset-upstream", branch)

	rm, err := ConfigureRepositoryWorktree(root, cfg, name, "repo", RepositoryWorktreeOptions{
		ExistingBranch: ExistingBranchReuse,
	})
	if err != nil {
		t.Fatalf("retry should adopt the existing target worktree: %v", err)
	}
	if rm.Branch != branch {
		t.Fatalf("branch = %q, want feature metadata branch %q", rm.Branch, branch)
	}
	meta, err := Load(root, name)
	if err != nil {
		t.Fatal(err)
	}
	if len(meta.Repositories) != 1 || meta.Repositories[0].Name != "repo" {
		t.Fatalf("repository metadata not saved after adoption: %+v", meta.Repositories)
	}
	if upstream, err := aggit.Upstream(dest, branch); err != nil || upstream != "origin/main" {
		t.Fatalf("adopted branch upstream = %q, err = %v; want origin/main", upstream, err)
	}
}

func TestConfigureRepositoryWorktreeUsesMetadataBranch(t *testing.T) {
	root, cfg := testWorkspace(t, "repo")
	name := "demo"
	branch := "legacy-prefix/demo"
	if err := Save(root, Metadata{Name: name, Branch: branch, Revision: 1}); err != nil {
		t.Fatal(err)
	}

	rm, err := ConfigureRepositoryWorktree(root, cfg, name, "repo", RepositoryWorktreeOptions{
		ExistingBranch: ExistingBranchReuse,
	})
	if err != nil {
		t.Fatal(err)
	}
	if rm.Branch != branch {
		t.Fatalf("branch = %q, want %q", rm.Branch, branch)
	}
	current, err := aggit.CurrentBranch(config.WorktreePath(root, name, "repo"))
	if err != nil || current != branch {
		t.Fatalf("worktree branch = %q, err = %v", current, err)
	}
}

func TestConfigureRepositoryWorktreeMovesFeatureBranchOutOfMainClone(t *testing.T) {
	root, cfg := testWorkspace(t, "repo")
	name := "demo"
	branch := "feature/demo"
	repoPath := config.RepoPath(root, "repo")
	testGit(t, repoPath, "checkout", "-b", branch)
	if err := Save(root, Metadata{Name: name, Branch: branch, Revision: 1}); err != nil {
		t.Fatal(err)
	}

	if _, err := ConfigureRepositoryWorktree(root, cfg, name, "repo", RepositoryWorktreeOptions{
		ExistingBranch: ExistingBranchReuse,
	}); err != nil {
		t.Fatal(err)
	}
	mainBranch, err := aggit.CurrentBranch(repoPath)
	if err != nil || mainBranch != "main" {
		t.Fatalf("main clone branch = %q, err = %v", mainBranch, err)
	}
	worktreeBranch, err := aggit.CurrentBranch(config.WorktreePath(root, name, "repo"))
	if err != nil || worktreeBranch != branch {
		t.Fatalf("feature worktree branch = %q, err = %v", worktreeBranch, err)
	}
}

func testWorkspace(t *testing.T, repoName string) (string, config.Config) {
	t.Helper()
	root := t.TempDir()
	remote := filepath.Join(t.TempDir(), repoName+".git")
	testGit(t, "", "init", "--bare", remote)
	seed := filepath.Join(t.TempDir(), "seed")
	testGit(t, "", "init", "-b", "main", seed)
	testGit(t, seed, "config", "user.email", "test@example.com")
	testGit(t, seed, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(seed, "README.md"), []byte(repoName), 0o644); err != nil {
		t.Fatal(err)
	}
	testGit(t, seed, "add", ".")
	testGit(t, seed, "commit", "-m", "initial")
	testGit(t, seed, "remote", "add", "origin", remote)
	testGit(t, seed, "push", "-u", "origin", "main")

	repoPath := config.RepoPath(root, repoName)
	if err := os.MkdirAll(filepath.Dir(repoPath), 0o755); err != nil {
		t.Fatal(err)
	}
	testGit(t, root, "clone", remote, repoPath)
	testGit(t, repoPath, "config", "user.email", "test@example.com")
	testGit(t, repoPath, "config", "user.name", "Test User")
	testGit(t, repoPath, "checkout", "main")

	cfg := config.Default(root, "test")
	cfg.Git.DefaultBaseBranch = "main"
	cfg.Git.BranchPrefix = "feature/"
	cfg.Repositories = []config.Repository{{
		Name: repoName, URL: remote, DefaultBranch: "main",
	}}
	return root, cfg
}

func testGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	if _, err := aggit.Run(dir, args...); err != nil {
		t.Fatalf("git %s: %v", strings.Join(args, " "), err)
	}
}
