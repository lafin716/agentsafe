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

// 같은 이름의 로컬 브랜치가 이미 있을 때 세 가지 ExistingBranchPolicy 가 각각
// 기대대로 동작하는지 확인한다.
func TestCreateWithExistingLocalBranchPolicies(t *testing.T) {
	// error: 아무것도 만들지 않고, 대안을 안내하는 오류를 낸다.
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

	// reuse: 기존 로컬 브랜치를 그대로 체크아웃하고, 원격 짝이 없으므로 업스트림은
	// base(origin/main)로 잡힌다.
	t.Run("reuse", func(t *testing.T) {
		root, cfg := testWorkspace(t, "repo")
		repoPath := config.RepoPath(root, "repo")
		testGit(t, repoPath, "branch", "feature/demo")

		if err := CreateWithOptions(root, cfg, "demo", CreateOptions{
			Base: "main", ExistingBranch: ExistingBranchReuse,
		}); err != nil {
			t.Fatal(err)
		}
		worktree := config.WorktreePath(root, config.FeatureKey("demo"), "repo")
		if branch, err := aggit.CurrentBranch(worktree); err != nil || branch != "feature/demo" {
			t.Fatalf("branch = %q, err = %v", branch, err)
		}
		if upstream, err := aggit.Upstream(worktree, "feature/demo"); err != nil || upstream != "origin/main" {
			t.Fatalf("upstream = %q, err = %v; want origin/main", upstream, err)
		}
	})

	// recreate: 기존 브랜치를 지우고 base 에서 새로 만들므로, 이전 브랜치에만 있던
	// 커밋(feature.txt)은 새 worktree 에 남지 않는다.
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
		if _, err := os.Stat(filepath.Join(config.WorktreePath(root, config.FeatureKey("demo"), "repo"), "feature.txt")); !os.IsNotExist(err) {
			t.Fatalf("recreated branch retained old commit, stat err = %v", err)
		}
	})
}

// 새로 만든 feature 브랜치는 첫 push 전까지 base(origin/main)를 추적하고, push 로
// 원격 브랜치가 생기고 나면 업스트림이 origin/<branch> 로 바뀐다.
func TestCreateTracksBaseUntilFirstPush(t *testing.T) {
	root, cfg := testWorkspace(t, "repo")
	if err := CreateWithOptions(root, cfg, "demo", CreateOptions{
		Base: "main", ExistingBranch: ExistingBranchError,
	}); err != nil {
		t.Fatal(err)
	}

	worktree := config.WorktreePath(root, config.FeatureKey("demo"), "repo")
	if upstream, err := aggit.Upstream(worktree, "feature/demo"); err != nil || upstream != "origin/main" {
		t.Fatalf("upstream = %q, err = %v; want origin/main", upstream, err)
	}

	res, err := Push(root, "demo", "", PushOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Failed() {
		t.Fatalf("push reported a failure: %+v", res.Repositories)
	}
	if res.Pushed() != 1 {
		t.Fatalf("pushed = %d, want 1 (result: %+v)", res.Pushed(), res.Repositories)
	}
	if !aggit.RemoteBranchExists(worktree, "feature/demo") {
		t.Fatal("first push did not create origin/feature/demo")
	}
	if upstream, err := aggit.Upstream(worktree, "feature/demo"); err != nil || upstream != "origin/feature/demo" {
		t.Fatalf("upstream after push = %q, err = %v; want origin/feature/demo", upstream, err)
	}
}

// 한글 이름으로 feature 를 만들면 브랜치 이름에는 원래 이름이 그대로 쓰이지만,
// 디스크 폴더는 ASCII 폴더 키를 쓴다(에디터 호환성). 메타데이터의 경로와 실제
// 폴더가 일치하는지도 확인한다.
func TestCreateUsesFeatHashForWorktreeFolder(t *testing.T) {
	root, cfg := testWorkspace(t, "repo")
	name := "테스트2"
	if err := CreateWithOptions(root, cfg, name, CreateOptions{
		Base: "main", ExistingBranch: ExistingBranchError,
	}); err != nil {
		t.Fatal(err)
	}
	meta, err := Load(root, name)
	if err != nil {
		t.Fatal(err)
	}
	key := config.FeatureKey(name)
	if meta.FolderKey() != key {
		t.Fatalf("folder key = %q, want %q", meta.FolderKey(), key)
	}
	if meta.Branch != "feature/"+name {
		t.Fatalf("branch = %q, want %q", meta.Branch, "feature/"+name)
	}
	want := filepath.ToSlash(filepath.Join("feature", key, "repo"))
	if len(meta.Repositories) != 1 || meta.Repositories[0].WorktreePath != want {
		t.Fatalf("worktree path = %+v, want %s", meta.Repositories, want)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(want))); err != nil {
		t.Fatalf("worktree folder missing: %v", err)
	}
}

// 레포 2개 중 뒤쪽에만 브랜치 충돌이 있을 때, 사전 점검은 모든 레포를 훑어 충돌을
// 보고하되 디렉터리는 하나도 만들지 않는다. 이어서 error 정책으로 생성을 시도하면
// 실패하는데, 이때 앞쪽 레포의 worktree 도 만들어지지 않아야 한다(부분 생성 방지).
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
	if _, err := os.Stat(config.WorktreePath(root, config.FeatureKey("demo"), "one")); !os.IsNotExist(err) {
		t.Fatalf("first repository worktree was partially created, stat err = %v", err)
	}
}

// 다른 worktree 가 이미 그 브랜치를 체크아웃하고 있으면 reuse 도 recreate 도 쓸 수
// 없으므로, 사전 점검이 Blocked 로 표시하고 이유를 남긴다.
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

// 로컬에는 없고 원격에만 남아 있는 feature 브랜치도 reuse 로 이어받을 수 있다.
// 원격 브랜치의 내용이 worktree 에 그대로 나오고, 업스트림은 origin/<branch> 가 된다.
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
	if _, err := os.Stat(filepath.Join(config.WorktreePath(root, config.FeatureKey("demo"), "repo"), "remote.txt")); err != nil {
		t.Fatalf("remote branch content missing: %v", err)
	}
	worktree := config.WorktreePath(root, config.FeatureKey("demo"), "repo")
	if upstream, err := aggit.Upstream(worktree, "feature/demo"); err != nil || upstream != "origin/feature/demo" {
		t.Fatalf("upstream = %q, err = %v; want origin/feature/demo", upstream, err)
	}
}

// 레포 추가와 재생성의 전체 흐름을 순서대로 검증한다. (1) 레포를 추가하면 feature
// 리비전이 오르고 에이전트 워크스페이스는 재준비가 필요해진다. (2) 커밋되지 않은
// 변경이 있는 worktree 의 재생성은 거부되며 파일도 리비전도 그대로다. (3) Force 를
// 주면 재생성되어 변경이 사라지고 리비전이 한 번 더 오른다.
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
	if err := os.MkdirAll(filepath.Join(root, "agent", config.FeatureKey("demo")), 0o755); err != nil {
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

	secondWorktree := config.WorktreePath(root, config.FeatureKey("demo"), "two")
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

// worktree 는 만들어졌지만 메타데이터 저장 직전에 실패한 이전 시도를 재시도하는
// 상황이다. 대상 경로의 worktree 를 입양해 오류 없이 진행하고, 메타데이터를 저장한
// 뒤 업스트림까지 설정해야 한다(재시도가 멱등해야 한다).
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

// 메타데이터에 저장된 브랜치 이름이 현재 설정의 접두사 규칙과 다르더라도(예전
// 접두사로 만든 feature), 설정에서 새로 계산하지 않고 메타데이터의 브랜치를 그대로
// 써야 한다.
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

// 메인 클론이 feature 브랜치를 체크아웃하고 있으면 그 브랜치를 worktree 에 붙일 수
// 없다. 클론이 깨끗하다면 메인 클론을 base 로 되돌리고 feature 브랜치를 worktree 로
// 옮겨야 한다.
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

// base 도 레포의 DefaultBranch 도 비어 있으면 레포가 지금 체크아웃한 브랜치를 base
// 로 삼는다. 그 브랜치에만 있는 파일이 worktree 에 나타나는지로 확인한다.
func TestCreateUsesCurrentRepositoryBranchWhenBaseAndDefaultAreEmpty(t *testing.T) {
	root, cfg := testWorkspace(t, "repo")
	cfg.Repositories[0].DefaultBranch = ""
	repoPath := config.RepoPath(root, "repo")
	testGit(t, repoPath, "checkout", "-b", "release")
	if err := os.WriteFile(filepath.Join(repoPath, "release.txt"), []byte("release"), 0o644); err != nil {
		t.Fatal(err)
	}
	testGit(t, repoPath, "add", ".")
	testGit(t, repoPath, "commit", "-m", "release")

	if err := CreateWithOptions(root, cfg, "demo", CreateOptions{
		ExistingBranch: ExistingBranchError,
	}); err != nil {
		t.Fatal(err)
	}
	meta, err := Load(root, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if len(meta.Repositories) != 1 || meta.Repositories[0].BaseBranch != "release" {
		t.Fatalf("base branch = %+v, want release", meta.Repositories)
	}
	if _, err := os.Stat(filepath.Join(config.WorktreePath(root, config.FeatureKey("demo"), "repo"), "release.txt")); err != nil {
		t.Fatalf("worktree was not based on current branch: %v", err)
	}
}

// testWorkspace 는 실제 git 을 쓰는 테스트 환경을 통째로 준비한다. bare 원격을 만들고,
// 초기 커밋이 있는 seed 저장소를 push 한 뒤, 그 원격을 workspace root 안으로 클론해
// 두고 대응하는 config.Config 를 함께 돌려준다. 반환하는 root 는 t.TempDir 이라
// 테스트가 끝나면 자동으로 정리된다.
func testWorkspace(t *testing.T, repoName string) (string, config.Config) {
	t.Helper()
	setTestGitTimeout(t)
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

// testGit 은 dir 에서 git 명령을 실행하고, 실패하면 즉시 테스트를 중단한다.
// dir 이 비어 있으면 명령의 인자에 담긴 경로를 대상으로 동작한다(init, clone 등).
func testGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	if _, err := aggit.Run(dir, args...); err != nil {
		t.Fatalf("git %s: %v", strings.Join(args, " "), err)
	}
}

// setTestGitTimeout 은 git 호출 타임아웃을 짧게 잡아, 네트워크나 인증 프롬프트에
// 걸린 명령이 테스트를 오래 붙잡고 있지 않게 한다. 외부에서 이미 지정했다면 그
// 값을 존중한다.
func setTestGitTimeout(t *testing.T) {
	t.Helper()
	if os.Getenv("AGENTSAFE_GIT_TIMEOUT_SECONDS") == "" {
		t.Setenv("AGENTSAFE_GIT_TIMEOUT_SECONDS", "10")
	}
}

// StatusData 의 병렬 워커 풀을 레포 여러 개로 돌려, 결과가 m.Repositories 순서를
// 그대로 유지하는지 확인한다. 원격 없이 `git init` 만 한 worktree 를 쓰므로, 다른
// worktree 테스트들이 필요로 하는 원격 push 세팅 없이 돌아간다.
func TestStatusDataPreservesRepoOrder(t *testing.T) {
	setTestGitTimeout(t)
	root := t.TempDir()
	names := []string{"a1", "a2", "a3"}
	var repos []RepoMeta
	for _, n := range names {
		wtRel := filepath.ToSlash(filepath.Join("feature", "demo", n))
		testGit(t, "", "init", "-b", "feature/demo", filepath.Join(root, filepath.FromSlash(wtRel)))
		repos = append(repos, RepoMeta{
			Name: n, WorktreePath: wtRel, Branch: "feature/demo", BaseBranch: "main",
		})
	}
	if err := Save(root, Metadata{
		Name: "demo", Key: "demo", Branch: "feature/demo", Repositories: repos,
	}); err != nil {
		t.Fatal(err)
	}

	res, err := StatusData(root, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Repositories) != len(names) {
		t.Fatalf("repos = %d, want %d", len(res.Repositories), len(names))
	}
	for i, n := range names {
		got := res.Repositories[i]
		if got.Name != n {
			t.Fatalf("repo[%d] = %q, want %q (parallel status must preserve order)", i, got.Name, n)
		}
		if got.Error != "" {
			t.Errorf("repo %q unexpected status error: %s", n, got.Error)
		}
		if got.Ahead != 0 {
			t.Errorf("repo %q ahead = %d, want 0 (no remote)", n, got.Ahead)
		}
	}
}
