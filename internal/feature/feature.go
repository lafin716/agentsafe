package feature

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/agentsafe/agentsafe/internal/applog"
	"github.com/agentsafe/agentsafe/internal/config"
	aggit "github.com/agentsafe/agentsafe/internal/git"
	"github.com/agentsafe/agentsafe/internal/output"
	"github.com/agentsafe/agentsafe/internal/ui"
	"github.com/agentsafe/agentsafe/internal/wttemplate"
)

// Metadata 는 feature 하나의 영속 상태다. root/<config.DirName>/features/<name>.json
// 에 저장되며, 어떤 브랜치를 어떤 base 위에 만들었고 어떤 레포의 worktree가 딸려
// 있는지를 기록한다.
type Metadata struct {
	Name string `json:"name"`
	// Key 는 디스크상의 worktree/agent 폴더(root/feature/<key>, root/agent/<key>)에
	// 쓰이는 ASCII 전용 식별자다. 생성 시점에 Name 에서 파생하므로, IntelliJ 같은
	// 에디터를 깨뜨리는 문자(예: 한글)가 폴더명에 절대 들어가지 않는다. 이 필드가
	// 생기기 전에 만들어진 feature 에서는 비어 있으며, 그 경우 FolderKey 가 Name
	// 으로 폴백한다.
	Key          string     `json:"key,omitempty"`
	Branch       string     `json:"branch"`
	BaseBranch   string     `json:"baseBranch"`
	CreatedAt    string     `json:"createdAt"`
	Revision     int        `json:"revision,omitempty"`
	Repositories []RepoMeta `json:"repositories"`
}

// FolderKey 는 feature 의 ASCII 폴더 키를 반환한다. Key 도입 이전에 만들어진
// feature 라면 Name 으로 폴백한다.
func (m Metadata) FolderKey() string {
	if m.Key != "" {
		return m.Key
	}
	return m.Name
}

// RepoMeta 는 feature 에 속한 레포 하나의 worktree 정보다. WorktreePath 는 root
// 기준 상대 경로를 슬래시 형태로 담는다.
type RepoMeta struct {
	Name         string `json:"name"`
	WorktreePath string `json:"worktreePath"`
	Branch       string `json:"branch"`
	BaseBranch   string `json:"baseBranch"`
	Revision     int    `json:"revision,omitempty"`
}

// BranchName 은 설정된 접두사를 붙여 feature 브랜치 이름을 만든다.
func BranchName(cfg config.Config, featureName string) string {
	return cfg.Git.BranchPrefix + featureName
}

// ExistingBranchPolicy 는 만들려는 feature 브랜치가 이미 존재할 때의 처리 방식이다.
type ExistingBranchPolicy string

const (
	// ExistingBranchError 는 기존 브랜치를 발견하면 아무것도 하지 않고 실패한다.
	ExistingBranchError ExistingBranchPolicy = "error"
	// ExistingBranchReuse 는 기존 브랜치를 그대로 체크아웃해 이어서 쓴다.
	ExistingBranchReuse ExistingBranchPolicy = "reuse"
	// ExistingBranchRecreate 는 기존 로컬 브랜치를 지우고 base 에서 새로 만든다.
	// 원격 브랜치는 건드리지 않는다.
	ExistingBranchRecreate ExistingBranchPolicy = "recreate"
)

// ParseExistingBranchPolicy 는 사용자 입력을 정책 값으로 변환한다. 빈 문자열은
// ExistingBranchError 로 해석한다.
func ParseExistingBranchPolicy(raw string) (ExistingBranchPolicy, error) {
	policy := ExistingBranchPolicy(strings.ToLower(strings.TrimSpace(raw)))
	if policy == "" {
		policy = ExistingBranchError
	}
	switch policy {
	case ExistingBranchError, ExistingBranchReuse, ExistingBranchRecreate:
		return policy, nil
	default:
		return "", fmt.Errorf("invalid existing branch policy %q (expected error, reuse, or recreate)", raw)
	}
}

// CreateOptions 는 feature 생성 옵션이다. Base 가 비어 있으면 각 레포의 현재
// 브랜치를 base 로 삼는다.
type CreateOptions struct {
	Base           string
	ExistingBranch ExistingBranchPolicy
}

// CreateCheck 는 생성 전 사전 점검(preflight) 결과다. HasConflicts 는 어느 레포든
// 브랜치가 이미 있다는 뜻이고, Blocked 는 어떤 정책으로도 진행할 수 없다는 뜻이다.
type CreateCheck struct {
	Name         string                  `json:"name"`
	Branch       string                  `json:"branch"`
	HasConflicts bool                    `json:"hasConflicts"`
	Blocked      bool                    `json:"blocked"`
	Repositories []RepositoryCreateCheck `json:"repositories"`
}

// RepositoryCreateCheck 는 레포 하나의 사전 점검 결과다. CheckedOutAt 은 해당
// 브랜치를 이미 체크아웃하고 있는 worktree 경로이며(없으면 빈 문자열),
// BlockedReason 이 채워져 있으면 그 레포는 어떤 정책으로도 진행할 수 없다.
type RepositoryCreateCheck struct {
	Name          string `json:"name"`
	BaseBranch    string `json:"baseBranch"`
	LocalBranch   bool   `json:"localBranch"`
	RemoteBranch  bool   `json:"remoteBranch"`
	CheckedOutAt  string `json:"checkedOutAt,omitempty"`
	Conflict      bool   `json:"conflict"`
	CanReuse      bool   `json:"canReuse"`
	CanRecreate   bool   `json:"canRecreate"`
	BlockedReason string `json:"blockedReason,omitempty"`
}

// RepositoryWorktreeOptions 는 ConfigureRepositoryWorktree 의 동작을 정한다.
// Recreate 가 false 면 feature 에 없는 레포를 새로 추가하고, true 면 이미 있는
// worktree 를 다시 만든다. Force 는 커밋되지 않은 변경이 있어도 강행한다.
type RepositoryWorktreeOptions struct {
	ExistingBranch ExistingBranchPolicy
	Recreate       bool
	Force          bool
}

// Load 는 feature 메타데이터를 디스크에서 읽어온다.
func Load(root, name string) (Metadata, error) {
	b, err := os.ReadFile(config.FeatureMetaPath(root, name))
	if err != nil {
		return Metadata{}, err
	}
	var m Metadata
	return m, json.Unmarshal(b, &m)
}

// Save 는 feature 메타데이터를 디스크에 기록하며, 상위 디렉터리가 없으면 만든다.
func Save(root string, m Metadata) error {
	if err := os.MkdirAll(filepath.Dir(config.FeatureMetaPath(root, m.Name)), 0755); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(m, "", "  ")
	return os.WriteFile(config.FeatureMetaPath(root, m.Name), b, 0644)
}

// Create 는 예전 force 플래그를 쓰는 호출자를 위해 남겨둔 래퍼다.
func Create(root string, cfg config.Config, name, base string, force bool) error {
	policy := ExistingBranchError
	if force {
		policy = ExistingBranchRecreate
	}
	return CreateWithOptions(root, cfg, name, CreateOptions{Base: base, ExistingBranch: policy})
}

// CreateWithOptions 는 feature 를 생성한다. 먼저 CheckCreate 로 모든 레포를 점검해
// 정책상 진행 가능한지 확인한 뒤, 레포별 worktree 를 만들고 메타데이터를 저장하고
// worktree 템플릿을 적용한다. 중간에 실패하면 그 시점에서 멈추므로 일부 레포만
// 만들어진 상태가 남을 수 있다 — 그래서 사전 점검을 먼저 돌린다.
func CreateWithOptions(root string, cfg config.Config, name string, opt CreateOptions) error {
	policy, err := ParseExistingBranchPolicy(string(opt.ExistingBranch))
	if err != nil {
		return err
	}
	check, err := CheckCreate(root, cfg, name, opt.Base)
	if err != nil {
		return err
	}
	if err := validateCreatePolicy(check, policy); err != nil {
		return err
	}
	branch := BranchName(cfg, name)
	key := uniqueFeatureKey(root, name)
	meta := Metadata{Name: name, Key: key, Branch: branch, BaseBranch: opt.Base, CreatedAt: time.Now().Format(time.RFC3339), Revision: 1}
	for i, r := range cfg.Repositories {
		repoBase := check.Repositories[i].BaseBranch
		rm, err := createRepositoryWorktree(root, key, r, branch, repoBase, policy)
		if err != nil {
			return err
		}
		meta.Repositories = append(meta.Repositories, rm)
	}
	if err := Save(root, meta); err != nil {
		return err
	}
	return wttemplate.Apply(root, key, templateRepos(root, meta.Repositories))
}

// CheckCreate 는 feature 생성 전에 설정된 모든 레포를 점검한다. 브랜치나 worktree
// 를 생성·삭제·체크아웃하지 않는 읽기 전용 동작이다.
func CheckCreate(root string, cfg config.Config, name, base string) (CreateCheck, error) {
	if err := config.ValidateFeatureName(name); err != nil {
		return CreateCheck{}, err
	}
	if _, err := os.Stat(config.FeatureMetaPath(root, name)); err == nil {
		return CreateCheck{}, fmt.Errorf("feature %q already exists", name)
	} else if !os.IsNotExist(err) {
		return CreateCheck{}, err
	}

	branch := BranchName(cfg, name)
	result := CreateCheck{Name: name, Branch: branch, Repositories: []RepositoryCreateCheck{}}
	for _, repo := range cfg.Repositories {
		repoPath := config.RepoPath(root, repo.Name)
		item := RepositoryCreateCheck{Name: repo.Name, CanReuse: true, CanRecreate: true}
		if _, err := os.Stat(repoPath); err != nil {
			item.BlockedReason = fmt.Sprintf("repository is not cloned at %s; run `agentsafe pull`", repoPath)
			item.CanReuse = false
			item.CanRecreate = false
			result.Blocked = true
			result.Repositories = append(result.Repositories, item)
			continue
		}

		item.BaseBranch = base
		if item.BaseBranch == "" {
			current, err := aggit.CurrentBranch(repoPath)
			if err != nil || current == "" {
				item.BlockedReason = "repository is in detached HEAD state; specify a base branch"
				item.CanReuse = false
				item.CanRecreate = false
				result.Blocked = true
				result.Repositories = append(result.Repositories, item)
				continue
			}
			item.BaseBranch = current
		}

		item.LocalBranch = aggit.LocalBranchExists(repoPath, branch)
		item.RemoteBranch = aggit.RemoteBranchExists(repoPath, branch)
		if !item.RemoteBranch {
			// 원격 추적 ref 를 갱신하지 않고 원격 상태만 들여다본다.
			item.RemoteBranch = aggit.RemoteBranchExistsAtOrigin(repoPath, branch)
		}
		item.CheckedOutAt = aggit.WorktreeForBranch(repoPath, branch)
		item.Conflict = item.LocalBranch || item.RemoteBranch || item.CheckedOutAt != ""
		if item.Conflict {
			result.HasConflicts = true
		}

		if item.CheckedOutAt != "" {
			switch {
			case samePath(item.CheckedOutAt, repoPath):
				if aggit.HasChanges(repoPath) {
					item.BlockedReason = "feature branch is checked out in the main clone with uncommitted changes"
					item.CanReuse = false
					item.CanRecreate = false
				} else if item.BaseBranch == branch {
					item.BlockedReason = "feature branch is checked out in the main clone; specify a different base branch"
					item.CanReuse = false
					item.CanRecreate = false
				}
			default:
				item.BlockedReason = fmt.Sprintf("feature branch is already checked out in worktree %s", item.CheckedOutAt)
				item.CanReuse = false
				item.CanRecreate = false
			}
		}
		if item.BlockedReason != "" {
			result.Blocked = true
		}
		result.Repositories = append(result.Repositories, item)
	}
	return result, nil
}

// validateCreatePolicy 는 사전 점검 결과를 선택한 정책과 대조해, 진행하면 실패할
// 레포가 하나라도 있으면 오류를 돌려준다.
func validateCreatePolicy(check CreateCheck, policy ExistingBranchPolicy) error {
	for _, repo := range check.Repositories {
		if repo.BlockedReason != "" {
			return fmt.Errorf("repository %s: %s", repo.Name, repo.BlockedReason)
		}
		if !repo.Conflict {
			continue
		}
		switch policy {
		case ExistingBranchError:
			return fmt.Errorf("branch %s already exists in repository %s; choose reuse or recreate", check.Branch, repo.Name)
		case ExistingBranchReuse:
			if !repo.CanReuse {
				return fmt.Errorf("branch %s cannot be reused in repository %s", check.Branch, repo.Name)
			}
		case ExistingBranchRecreate:
			if !repo.CanRecreate {
				return fmt.Errorf("branch %s cannot be recreated in repository %s", check.Branch, repo.Name)
			}
		}
	}
	return nil
}

// uniqueFeatureKey 는 name 에서 ASCII 폴더 키를 만들고(config.FeatureKey), 기존
// feature 의 폴더와 겹치면 숫자 접미사를 붙여 충돌을 피한다.
func uniqueFeatureKey(root, name string) string {
	base := config.FeatureKey(name)
	candidate := base
	for i := 2; featureKeyTaken(root, candidate); i++ {
		candidate = fmt.Sprintf("%s-%d", base, i)
	}
	return candidate
}

// featureKeyTaken 은 key 가 이미 worktree/agent 폴더로 쓰이고 있거나 기존 feature
// 메타데이터의 폴더 키로 등록되어 있는지 확인한다.
func featureKeyTaken(root, key string) bool {
	for _, dir := range []string{"feature", "agent"} {
		if st, err := os.Stat(filepath.Join(root, dir, key)); err == nil && st.IsDir() {
			return true
		}
	}
	metaDir := filepath.Join(root, config.DirName, "features")
	entries, err := os.ReadDir(metaDir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		b, readErr := os.ReadFile(filepath.Join(metaDir, e.Name()))
		if readErr != nil {
			continue
		}
		var m Metadata
		if json.Unmarshal(b, &m) == nil && m.FolderKey() == key {
			return true
		}
	}
	return false
}

// createRepositoryWorktree 는 레포 하나에 feature 브랜치 worktree 를 붙인다.
// base 를 fetch 해 최신 시작점을 확보하고, 로컬/원격에 이미 있는 브랜치는 policy
// 에 따라 처리한 뒤, 브랜치가 base 를 추적하도록 업스트림을 설정한다.
func createRepositoryWorktree(root, featureName string, repo config.Repository, branch, base string, policy ExistingBranchPolicy) (RepoMeta, error) {
	repoPath := config.RepoPath(root, repo.Name)
	dest := config.WorktreePath(root, featureName, repo.Name)
	rel, _ := filepath.Rel(root, dest)
	output.Printf("[%s] creating worktree %s\n", repo.Name, rel)
	if _, err := os.Stat(repoPath); err != nil {
		return RepoMeta{}, fmt.Errorf("repository %s is not cloned at %s; run `agentsafe pull`", repo.Name, repoPath)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return RepoMeta{}, err
	}

	start := base
	output.Printf("  fetch origin %s (non-interactive, timeout controlled by AGENTSAFE_GIT_TIMEOUT_SECONDS)...\n", base)
	if err := aggit.FetchBranch(repoPath, base); err != nil {
		output.Printf("  warning: fetch failed, using local %s: %v\n", base, err)
	} else {
		start = "FETCH_HEAD"
	}

	local := aggit.LocalBranchExists(repoPath, branch)
	remote := aggit.RemoteBranchExists(repoPath, branch)
	if !remote && (!local || aggit.RemoteBranchExistsAtOrigin(repoPath, branch)) {
		// 원격에만 있는 feature 브랜치를 찾아낸다. 로컬 브랜치는 있는데 원격 추적
		// ref 가 낡아 원격 짝을 놓치는 경우도 여기서 걸러진다.
		_ = aggit.FetchAll(repoPath)
		remote = aggit.RemoteBranchExists(repoPath, branch)
	}

	_ = aggit.WorktreePrune(repoPath)
	if inUse := aggit.WorktreeForBranch(repoPath, branch); inUse != "" {
		// 이전 시도가 대상 worktree 는 만들었지만 메타데이터 저장 전에 실패했을 수
		// 있다. 바로 그 worktree 를 입양해서 레포 추가 재시도가 멱등하게 동작하도록
		// 한다.
		if samePath(inUse, dest) {
			if current, err := aggit.CurrentBranch(dest); err == nil && current == branch {
				output.Printf("  worktree already exists at target, adopting branch %s\n", branch)
				if err := configureWorktreeUpstream(dest, branch, base, remote, true); err != nil {
					return RepoMeta{}, fmt.Errorf("failed to configure upstream for branch %s in repository %s: %w", branch, repo.Name, err)
				}
				return RepoMeta{
					Name:         repo.Name,
					WorktreePath: filepath.ToSlash(rel),
					Branch:       branch,
					BaseBranch:   base,
					Revision:     1,
				}, nil
			}
		}
		// 갓 클론한 레포는 메인 클론에 feature 브랜치가 체크아웃되어 있을 수 있다
		// (예: 원격 HEAD 가 그 브랜치를 가리킬 때). 메인 클론이 깨끗하다면 base
		// 브랜치로 되돌려서, feature 브랜치를 요청된 worktree 에 붙일 수 있게 한다.
		if samePath(inUse, repoPath) && policy != ExistingBranchError {
			if aggit.HasChanges(repoPath) {
				return RepoMeta{}, fmt.Errorf("branch %s is checked out in the main clone %s, which has uncommitted changes; commit or stash them first", branch, inUse)
			}
			output.Printf("  switching main clone from %s to base branch %s\n", branch, base)
			if err := aggit.Checkout(repoPath, base); err != nil {
				return RepoMeta{}, fmt.Errorf("branch %s is checked out in the main clone and switching it to %s failed: %w", branch, base, err)
			}
		} else {
			return RepoMeta{}, fmt.Errorf("branch %s is already checked out in worktree %s", branch, inUse)
		}
	}

	create := true
	preserveExistingUpstream := false
	trackFeatureBranch := false
	switch {
	case local && policy == ExistingBranchError:
		return RepoMeta{}, fmt.Errorf("branch %s already exists in repository %s; choose reuse or recreate", branch, repo.Name)
	case (local || remote) && policy == ExistingBranchReuse:
		if local {
			output.Printf("  reusing existing local branch %s\n", branch)
			create = false
			start = branch
			preserveExistingUpstream = !remote
			trackFeatureBranch = remote
		} else {
			output.Printf("  creating tracking branch %s from origin/%s\n", branch, branch)
			start = "origin/" + branch
			trackFeatureBranch = true
		}
	case (local || remote) && policy == ExistingBranchRecreate:
		if local {
			output.Printf("  deleting existing local branch %s\n", branch)
			if err := aggit.DeleteLocalBranch(repoPath, branch); err != nil {
				return RepoMeta{}, fmt.Errorf("failed to delete branch %s in repository %s: %w", branch, repo.Name, err)
			}
		}
		if remote {
			output.Printf("  warning: remote branch origin/%s is preserved\n", branch)
		}
	case remote:
		return RepoMeta{}, fmt.Errorf("remote branch %s already exists in repository %s; choose reuse or recreate", branch, repo.Name)
	}

	if create {
		output.Printf("  creating new branch %s from %s\n", branch, start)
	}
	if err := aggit.AddWorktree(repoPath, dest, branch, start, create); err != nil {
		return RepoMeta{}, fmt.Errorf("failed to create worktree for repository %s: %w", repo.Name, err)
	}
	if err := configureWorktreeUpstream(dest, branch, base, trackFeatureBranch, preserveExistingUpstream); err != nil {
		return RepoMeta{}, fmt.Errorf("failed to configure upstream for branch %s in repository %s: %w", branch, repo.Name, err)
	}
	return RepoMeta{
		Name:         repo.Name,
		WorktreePath: filepath.ToSlash(rel),
		Branch:       branch,
		BaseBranch:   base,
		Revision:     1,
	}, nil
}

// configureWorktreeUpstream 은 새 feature 브랜치가 첫 push 전까지 base 브랜치를
// 추적하게 만든다. 원격에서 재사용한 feature 브랜치는 짝이 되는 origin 브랜치를
// 추적하고, 재사용한 로컬 브랜치는 이미 설정된 유효한 업스트림을 그대로 둔다.
func configureWorktreeUpstream(path, branch, base string, trackFeatureBranch, preserveExisting bool) error {
	if preserveExisting {
		if upstream, err := aggit.Upstream(path, branch); err == nil && upstream != "" {
			return nil
		}
	}

	targetBranch := base
	if trackFeatureBranch {
		targetBranch = branch
	}
	targetBranch = aggit.NormalizeBranchName(targetBranch)
	if targetBranch == "" {
		return nil
	}
	if !aggit.RemoteBranchExists(path, targetBranch) {
		output.Printf("  warning: origin/%s not found; branch %s has no upstream\n", targetBranch, branch)
		return nil
	}
	target := "origin/" + targetBranch
	if err := aggit.SetUpstream(path, branch, target); err != nil {
		return err
	}
	output.Printf("  branch %s now tracks %s\n", branch, target)
	return nil
}

// ConfigureRepositoryWorktree 는 feature 에 빠져 있는 레포를 추가하거나, 이미 있는
// 레포의 worktree 를 다시 만든다. 어느 쪽이든 다른 레포는 건드리지 않는다.
func ConfigureRepositoryWorktree(root string, cfg config.Config, featureName, repoName string, opt RepositoryWorktreeOptions) (RepoMeta, error) {
	meta, err := Load(root, featureName)
	if err != nil {
		return RepoMeta{}, err
	}
	var repoCfg config.Repository
	foundCfg := false
	for _, r := range cfg.Repositories {
		if r.Name == repoName {
			repoCfg, foundCfg = r, true
			break
		}
	}
	if !foundCfg {
		return RepoMeta{}, fmt.Errorf("repository %q is not configured", repoName)
	}
	existingIndex := -1
	for i, r := range meta.Repositories {
		if r.Name == repoName {
			existingIndex = i
			break
		}
	}
	if opt.Recreate && existingIndex < 0 {
		return RepoMeta{}, fmt.Errorf("repository %q is not part of feature %q", repoName, featureName)
	}
	if !opt.Recreate && existingIndex >= 0 {
		return RepoMeta{}, fmt.Errorf("repository %q is already part of feature %q", repoName, featureName)
	}
	policy, err := ParseExistingBranchPolicy(string(opt.ExistingBranch))
	if err != nil {
		return RepoMeta{}, err
	}
	if opt.Recreate && policy == ExistingBranchError {
		return RepoMeta{}, fmt.Errorf("repository %q already has a feature branch; choose reuse or recreate", repoName)
	}

	if opt.Recreate {
		old := meta.Repositories[existingIndex]
		dest := filepath.Join(root, filepath.FromSlash(old.WorktreePath))
		if st, statErr := os.Stat(dest); statErr == nil && st.IsDir() {
			if !opt.Force && aggit.HasChanges(dest) {
				return RepoMeta{}, fmt.Errorf("worktree for repository %s has uncommitted changes; commit/stash or use force", repoName)
			}
			if err := aggit.RemoveWorktree(config.RepoPath(root, repoName), dest, opt.Force); err != nil {
				return RepoMeta{}, fmt.Errorf("failed to remove worktree for repository %s: %w", repoName, err)
			}
		} else {
			_ = aggit.WorktreePrune(config.RepoPath(root, repoName))
			_ = os.RemoveAll(dest)
		}
	}

	base := repoCfg.DefaultBranch
	if existingIndex >= 0 && meta.Repositories[existingIndex].BaseBranch != "" {
		base = meta.Repositories[existingIndex].BaseBranch
	}
	if base == "" {
		current, err := aggit.CurrentBranch(config.RepoPath(root, repoName))
		if err != nil || current == "" {
			base = cfg.Git.DefaultBaseBranch
		} else {
			base = current
		}
	}
	branch := meta.Branch
	if branch == "" {
		branch = BranchName(cfg, featureName)
	}

	if !opt.Recreate {
		// 아직 feature 에 속하지 않은 레포를 추가하는 경로다. 같은 이름으로 전에
		// 만들었던 worktree 폴더가 디스크에 남아 있을 수 있고, 그 상태로는
		// git worktree add 가 "already exists" 로 실패한다. 그 폴더가 feature
		// 브랜치 자신의 worktree 라면 입양 가능하므로, 남은 폴더나 남의 폴더만
		// 막는다.
		repoPath := config.RepoPath(root, repoName)
		dest := config.WorktreePath(root, meta.FolderKey(), repoName)
		if st, statErr := os.Stat(dest); statErr == nil && st.IsDir() {
			adoptable := samePath(aggit.WorktreeForBranch(repoPath, branch), dest)
			if adoptable {
				if current, cerr := aggit.CurrentBranch(dest); cerr != nil || current != branch {
					adoptable = false
				}
			}
			if !adoptable {
				if !opt.Force {
					return RepoMeta{}, fmt.Errorf("worktree directory already exists for repository %s at %s; delete it and recreate", repoName, dest)
				}
				_ = aggit.WorktreePrune(repoPath)
				if err := aggit.RemoveWorktree(repoPath, dest, true); err != nil {
					_ = os.RemoveAll(dest)
				}
			}
		}
	}

	rm, err := createRepositoryWorktree(root, meta.FolderKey(), repoCfg, branch, base, policy)
	if err != nil {
		return RepoMeta{}, err
	}
	if existingIndex >= 0 {
		rm.Revision = meta.Repositories[existingIndex].Revision + 1
		if rm.Revision == 1 {
			rm.Revision = 2
		}
		meta.Repositories[existingIndex] = rm
	} else {
		meta.Repositories = append(meta.Repositories, rm)
	}
	meta.Revision++
	if meta.Revision == 1 && meta.CreatedAt == "" {
		meta.CreatedAt = time.Now().Format(time.RFC3339)
	}
	if err := Save(root, meta); err != nil {
		return RepoMeta{}, err
	}
	if err := wttemplate.ApplyToRepos(root, templateRepos(root, []RepoMeta{rm})); err != nil {
		return RepoMeta{}, err
	}
	return rm, nil
}

// templateRepos 는 RepoMeta 의 상대 worktree 경로를 절대 경로로 바꿔
// wttemplate 이 받는 형태로 변환한다.
func templateRepos(root string, repos []RepoMeta) []wttemplate.Repo {
	out := make([]wttemplate.Repo, 0, len(repos))
	for _, r := range repos {
		out = append(out, wttemplate.Repo{
			Name:         r.Name,
			WorktreePath: filepath.Join(root, filepath.FromSlash(r.WorktreePath)),
		})
	}
	return out
}

// samePath 는 두 경로가 같은 위치를 가리키는지 비교한다. 심볼릭 링크를 해석하고,
// Windows 에서는 대소문자를 구분하지 않는다.
func samePath(a, b string) bool {
	aa, errA := filepath.Abs(filepath.Clean(a))
	bb, errB := filepath.Abs(filepath.Clean(b))
	if errA != nil || errB != nil {
		return false
	}
	if realA, err := filepath.EvalSymlinks(aa); err == nil {
		aa = realA
	}
	if realB, err := filepath.EvalSymlinks(bb); err == nil {
		bb = realB
	}
	if runtime.GOOS == "windows" {
		return strings.EqualFold(aa, bb)
	}
	return aa == bb
}

// DeleteOptions 는 feature 삭제 동작을 제어한다. DeleteBranch 를 켜면 각 레포의
// 로컬 feature 브랜치까지 지우고, Force 는 커밋되지 않은 변경이 있는 worktree 도
// 제거한다(끄면 그런 경우 삭제를 거부한다).
type DeleteOptions struct {
	DeleteBranch bool
	Force        bool
}

// DeleteResult 는 치명적이지 않은 정리 실패를 모아 보고한다. 이런 실패가 나도
// 삭제는 계속 진행되므로, 나머지 레포와 feature 산출물은 그대로 제거된다.
type DeleteResult struct {
	Warnings []string `json:"warnings" yaml:"warnings"`
}

// Delete 는 feature 의 worktree 와 모든 산출물(feature 메타데이터, 에이전트
// 워크스페이스, 세션 메타데이터, 동기화 이력)을 제거한다. DeleteBranch 를 켜면 각
// 레포의 로컬 feature 브랜치도 지운다. Force 가 아니라면 worktree 중 하나라도
// 커밋되지 않은 변경이 있을 때 삭제를 거부한다(부분 삭제를 피하려고 아무것도
// 지우지 않는다).
func Delete(root, name string, opt DeleteOptions) error {
	_, err := DeleteWithResult(root, name, opt)
	return err
}

// DeleteWithResult 는 Delete 와 같은 삭제를 수행하되, 전체 삭제를 중단시키지는
// 않은 채 실패한 정리 단계들을 경고 목록으로 돌려준다.
func DeleteWithResult(root, name string, opt DeleteOptions) (DeleteResult, error) {
	result := DeleteResult{Warnings: []string{}}
	m, err := Load(root, name)
	if err != nil {
		return result, err
	}

	if !opt.Force {
		var dirty []string
		for _, r := range m.Repositories {
			dest := filepath.Join(root, filepath.FromSlash(r.WorktreePath))
			if st, e := os.Stat(dest); e == nil && st.IsDir() && aggit.HasChanges(dest) {
				dirty = append(dirty, r.Name)
			}
		}
		if len(dirty) > 0 {
			return result, fmt.Errorf("worktree(s) have uncommitted changes: %s; commit/stash or use force", strings.Join(dirty, ", "))
		}
	}

	warn := func(message string) {
		result.Warnings = append(result.Warnings, message)
		output.Printf("  warning: %s\n", message)
	}

	for _, r := range m.Repositories {
		repoPath := config.RepoPath(root, r.Name)
		dest := filepath.Join(root, filepath.FromSlash(r.WorktreePath))
		output.Printf("[%s] removing worktree %s\n", r.Name, r.WorktreePath)
		if _, e := os.Stat(dest); e == nil {
			if err := aggit.RemoveWorktree(repoPath, dest, opt.Force); err != nil {
				warn(fmt.Sprintf("[%s] git worktree remove failed: %v", r.Name, err))
				if err := os.RemoveAll(dest); err != nil {
					warn(fmt.Sprintf("[%s] failed to remove worktree directory: %v", r.Name, err))
				}
				if err := aggit.WorktreePrune(repoPath); err != nil {
					warn(fmt.Sprintf("[%s] git worktree prune failed: %v", r.Name, err))
				}
			}
		} else {
			if e != nil && !os.IsNotExist(e) {
				warn(fmt.Sprintf("[%s] failed to inspect worktree directory: %v", r.Name, e))
			}
			if err := aggit.WorktreePrune(repoPath); err != nil {
				warn(fmt.Sprintf("[%s] git worktree prune failed: %v", r.Name, err))
			}
			if err := os.RemoveAll(dest); err != nil {
				warn(fmt.Sprintf("[%s] failed to remove worktree directory: %v", r.Name, err))
			}
		}
		if opt.DeleteBranch {
			output.Printf("[%s] deleting local branch %s\n", r.Name, r.Branch)
			if err := aggit.DeleteLocalBranch(repoPath, r.Branch); err != nil {
				warn(fmt.Sprintf("[%s] could not delete branch %s: %v", r.Name, r.Branch, err))
			}
		}
	}

	// feature 산출물을 모두 정리한다. 앞선 삭제가 실패해도 나머지 경로는 빠짐없이
	// 시도한다.
	output.Printf("removing feature metadata and agent artifacts for %s\n", name)
	cleanup := []struct {
		label string
		path  string
		all   bool
	}{
		{"feature directory", filepath.Join(root, "feature", m.FolderKey()), true},
		{"feature metadata", config.FeatureMetaPath(root, name), false},
		{"agent workspace", filepath.Join(root, "agent", m.FolderKey()), true},
		{"session metadata", config.SessionMetaPath(root, name), false},
		{"sync history", filepath.Join(config.HistoryDir(root), name), true},
	}
	for _, item := range cleanup {
		var err error
		if item.all {
			err = os.RemoveAll(item.path)
		} else {
			err = os.Remove(item.path)
			if os.IsNotExist(err) {
				err = nil
			}
		}
		if err != nil {
			warn(fmt.Sprintf("failed to remove %s: %v", item.label, err))
		}
	}
	return result, nil
}

// FeatureListResult 는 feature 목록 조회 결과다.
type FeatureListResult struct {
	Features []FeatureEntry `json:"features" yaml:"features"`
}

// FeatureEntry 는 목록에 표시되는 feature 한 줄이다. AgentReady 는 에이전트
// 워크스페이스 폴더가 존재하는지를 뜻한다.
type FeatureEntry struct {
	Name       string `json:"name"       yaml:"name"`
	Branch     string `json:"branch"     yaml:"branch"`
	BaseBranch string `json:"baseBranch" yaml:"baseBranch"`
	RepoCount  int    `json:"repoCount"  yaml:"repoCount"`
	AgentReady bool   `json:"agentReady" yaml:"agentReady"`
}

// FeatureStatusResult 는 feature 하나의 상태 요약이다. AgentReady 는 모든 레포가
// 준비된 경우에만 true 이고, AgentNeedsPrepare 는 worktree 가 바뀌어 에이전트
// 워크스페이스를 다시 준비해야 하는 레포가 하나라도 있으면 true 다.
type FeatureStatusResult struct {
	Feature           string       `json:"feature"      yaml:"feature"`
	Branch            string       `json:"branch"       yaml:"branch"`
	AgentReady        bool         `json:"agentReady"   yaml:"agentReady"`
	AgentNeedsPrepare bool         `json:"agentNeedsPrepare" yaml:"agentNeedsPrepare"`
	Repositories      []RepoStatus `json:"repositories" yaml:"repositories"`
}

// RepoStatus 는 레포 하나의 worktree 상태다.
type RepoStatus struct {
	Name              string           `json:"name"              yaml:"name"`
	Status            string           `json:"status"            yaml:"status"`
	Changes           []RepoFileStatus `json:"changes"           yaml:"changes"`
	AgentReady        bool             `json:"agentReady"        yaml:"agentReady"`
	AgentNeedsPrepare bool             `json:"agentNeedsPrepare" yaml:"agentNeedsPrepare"`
	// Ahead 는 feature 브랜치에서 아직 push 되지 않은 커밋 수다(origin/<branch> 가
	// 있으면 그 기준, 없으면 base 브랜치 기준). 지금 push 하면 올라갈 분량이다.
	Ahead int    `json:"ahead"             yaml:"ahead"`
	Error string `json:"error,omitempty"   yaml:"error,omitempty"`
}

// unpushedCount 는 feature 브랜치에서 아직 push 되지 않은 커밋 수를 센다.
// origin/<branch> 가 있으면 그보다 앞선 커밋을 세고, 없으면(한 번도 push 하지 않은
// 브랜치) base 대비 추가된 커밋을 센다.
func unpushedCount(path, branch, base string) int {
	if branch == "" {
		return 0
	}
	// 후보 범위를 원래 우선순위대로(origin/<branch> → origin/<base> → 로컬 base)
	// 시도하고, 처음으로 해석되는 ref 를 쓴다. ref 가 없다는 사실을 rev-list 자체의
	// 오류로 판단하면 RemoteBranchExists 를 따로 띄우지 않아도 된다 — AV 검사가
	// 도는 Windows 에서는 git 하위 프로세스 하나당 약 2초가 들고, status 는
	// worktree 상세 화면의 핫 패스에 있다.
	if n, err := aggit.RevListCount(path, "origin/"+branch+"..HEAD"); err == nil {
		return n
	}
	if base != "" {
		if n, err := aggit.RevListCount(path, "origin/"+base+"..HEAD"); err == nil {
			return n
		}
		if n, err := aggit.RevListCount(path, base+"..HEAD"); err == nil {
			return n
		}
	}
	return 0
}

// RepoFileStatus 는 worktree 안에서 변경된 파일 하나를 나타낸다. Code 는 git 의
// 상태 코드다.
type RepoFileStatus struct {
	Code string `json:"code" yaml:"code"`
	Type string `json:"type" yaml:"type"`
	Path string `json:"path" yaml:"path"`
}

// ListData 는 저장된 모든 feature 메타데이터를 읽어 목록으로 돌려준다. 읽거나
// 파싱할 수 없는 파일은 건너뛴다.
func ListData(root string) (FeatureListResult, error) {
	dir := filepath.Join(root, config.DirName, "features")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return FeatureListResult{}, err
	}
	var features []FeatureEntry
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		b, _ := os.ReadFile(filepath.Join(dir, e.Name()))
		var m Metadata
		if json.Unmarshal(b, &m) == nil {
			ready := false
			if st, err := os.Stat(filepath.Join(root, "agent", m.FolderKey())); err == nil && st.IsDir() {
				ready = true
			}
			features = append(features, FeatureEntry{
				Name:       m.Name,
				Branch:     m.Branch,
				BaseBranch: m.BaseBranch,
				RepoCount:  len(m.Repositories),
				AgentReady: ready,
			})
		}
	}
	return FeatureListResult{Features: features}, nil
}

// List 는 feature 목록을 사람이 읽는 표 형태로 출력한다.
func List(root string) error {
	data, err := ListData(root)
	if err != nil {
		return err
	}
	rows := [][]string{}
	for _, f := range data.Features {
		ready := "no"
		if f.AgentReady {
			ready = "yes"
		}
		rows = append(rows, []string{f.Name, f.Branch, f.BaseBranch, fmt.Sprint(f.RepoCount), ready})
	}
	ui.PrintRows([]string{"FEATURE", "BRANCH", "BASE", "REPOS", "AGENT_READY"}, rows)
	return nil
}

// StatusData 는 feature 에 속한 모든 레포의 상태를 모아 반환한다. 세션
// 메타데이터에 기록된 준비 리비전과 현재 worktree 리비전을 비교해, 에이전트
// 워크스페이스를 다시 준비해야 하는지도 함께 판단한다.
func StatusData(root, name string) (FeatureStatusResult, error) {
	m, err := Load(root, name)
	if err != nil {
		return FeatureStatusResult{}, err
	}
	result := FeatureStatusResult{Feature: m.Name, Branch: m.Branch}
	b, _ := os.ReadFile(config.SessionMetaPath(root, name))
	var prepared struct {
		FeatureRevision int `json:"featureRevision"`
		Repositories    []struct {
			Name             string `json:"name"`
			WorktreeRevision int    `json:"worktreeRevision"`
		} `json:"repositories"`
	}
	_ = json.Unmarshal(b, &prepared)
	preparedRepos := map[string]int{}
	for _, r := range prepared.Repositories {
		preparedRepos[r.Name] = r.WorktreeRevision
	}
	statusStart := time.Now()
	// 레포 하나의 상태를 구하는 데 git 하위 프로세스가 여러 개 필요하고, Windows
	// 에서는 프로세스 생성 비용이 비교적 크다. 그래서 레포들을 (개수를 제한한)
	// 병렬로 훑고, 레포별 작업 트리 상태와 미push 커밋 수도 동시에 구한다. 결과는
	// 인덱스로 써서 m.Repositories 순서를 보존하고, 준비 여부 집계는 풀이 끝난 뒤에
	// 하므로 동시에 변경되는 공유 상태가 없다.
	results := make([]RepoStatus, len(m.Repositories))
	folderKey := m.FolderKey()
	workerCount := len(m.Repositories)
	if workerCount > 4 {
		workerCount = 4
	}
	if workerCount > 0 {
		jobs := make(chan int)
		var wg sync.WaitGroup
		for w := 0; w < workerCount; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := range jobs {
					results[i] = repoStatusFor(root, name, folderKey, preparedRepos, m.Repositories[i])
				}
			}()
		}
		for i := range m.Repositories {
			jobs <- i
		}
		close(jobs)
		wg.Wait()
	}

	allReady := len(m.Repositories) > 0
	for _, rs := range results {
		result.Repositories = append(result.Repositories, rs)
		if !rs.AgentReady {
			allReady = false
		}
		if rs.AgentNeedsPrepare {
			result.AgentNeedsPrepare = true
		}
	}
	result.AgentReady = allReady
	applog.Info("status completed",
		"feature", name,
		"repos", len(m.Repositories),
		"ms", time.Since(statusStart).Milliseconds(),
	)
	return result, nil
}

// repoStatusFor 는 레포 하나의 상태를 계산한다. 작업 트리 상태와 미push 커밋 수는
// 서로 독립적인 git 호출이라 동시에 실행하며, 그래서 레포당 소요 시간은 둘의 합이
// 아니라 더 느린 쪽이 된다.
func repoStatusFor(root, name, folderKey string, preparedRepos map[string]int, r RepoMeta) RepoStatus {
	p := filepath.Join(root, r.WorktreePath)
	repoStatus := RepoStatus{Name: r.Name, Changes: []RepoFileStatus{}}

	var (
		s             string
		files         []aggit.FileStatus
		statusErr     error
		statusFilesMs int64
		unpushedMs    int64
		ahead         int
		wg            sync.WaitGroup
	)
	wg.Add(2)
	go func() {
		defer wg.Done()
		t := time.Now()
		s, files, statusErr = aggit.StatusFiles(p)
		statusFilesMs = time.Since(t).Milliseconds()
	}()
	go func() {
		defer wg.Done()
		t := time.Now()
		ahead = unpushedCount(p, r.Branch, r.BaseBranch)
		unpushedMs = time.Since(t).Milliseconds()
	}()
	wg.Wait()

	repoStatus.Status = s
	if st, statErr := os.Stat(config.AgentPath(root, folderKey, r.Name)); statErr == nil && st.IsDir() {
		if revision, ok := preparedRepos[r.Name]; ok {
			repoStatus.AgentReady = true
			// 예전 메타데이터는 리비전이 0인데, 이 레포의 worktree 가 첫 리비전을
			// 받기 전까지는 그대로 유효한 것으로 본다.
			repoStatus.AgentNeedsPrepare = r.Revision > 0 && revision != r.Revision
		}
	}
	if !repoStatus.AgentReady {
		repoStatus.AgentNeedsPrepare = true
	}

	if statusErr != nil {
		repoStatus.Status = "ERROR: " + statusErr.Error()
		repoStatus.Error = statusErr.Error()
	} else {
		for _, file := range files {
			repoStatus.Changes = append(repoStatus.Changes, RepoFileStatus{
				Code: file.Code,
				Type: file.Type,
				Path: file.Path,
			})
		}
		repoStatus.Ahead = ahead
	}

	applog.Info("status repo timing",
		"feature", name,
		"repo", r.Name,
		"statusFilesMs", statusFilesMs,
		"unpushedMs", unpushedMs,
		"changes", len(repoStatus.Changes),
	)
	return repoStatus
}

// Status 는 feature 상태를 사람이 읽는 형태로 출력한다.
func Status(root, name string) error {
	data, err := StatusData(root, name)
	if err != nil {
		return err
	}
	fmt.Printf("Feature: %s\nBranch: %s\n\n", data.Feature, data.Branch)
	for _, r := range data.Repositories {
		fmt.Printf("[%s]\n", r.Name)
		if r.Status == "" {
			fmt.Println("clean")
		} else {
			fmt.Println(r.Status)
		}
		fmt.Println()
	}
	return nil
}

// Commit 은 각 레포의 worktree 변경을 커밋한다. repoFilter 가 비어 있지 않으면 그
// 레포만, 비어 있으면 모든 레포를 대상으로 한다.
func Commit(root, name, message, repoFilter string) error {
	if message == "" {
		return fmt.Errorf("commit message is required (-m)")
	}
	m, err := Load(root, name)
	if err != nil {
		return err
	}
	for _, r := range m.Repositories {
		if repoFilter != "" && r.Name != repoFilter {
			continue
		}
		p := filepath.Join(root, r.WorktreePath)
		output.Printf("[%s] ", r.Name)
		if !aggit.HasChanges(p) {
			output.Println("clean, skipped")
			continue
		}
		if err := aggit.CommitAll(p, message); err != nil {
			output.Printf("failed: %v\n", err)
		} else {
			output.Println("committed")
		}
	}
	return nil
}

// RebaseRepoResult 는 레포 하나의 리베이스 결과다. Detail 은 사용자에게 보여줄
// 설명이다.
type RebaseRepoResult struct {
	Name       string `json:"name"       yaml:"name"`
	Branch     string `json:"branch"     yaml:"branch"`
	BaseBranch string `json:"baseBranch" yaml:"baseBranch"`
	Status     string `json:"status"     yaml:"status"` // rebased | up-to-date | skipped | failed
	Detail     string `json:"detail"     yaml:"detail"`
}

// RebaseResult 는 feature 전체의 리베이스 결과다.
type RebaseResult struct {
	Feature      string             `json:"feature"      yaml:"feature"`
	Repositories []RebaseRepoResult `json:"repositories" yaml:"repositories"`
}

// Rebase 는 각 feature worktree 의 브랜치를 최신 base 브랜치(가능하면
// origin/<base>) 위로 다시 얹는다. 커밋되지 않은 변경이 있는 worktree 는 건너뛰고,
// 충돌이 난 리베이스는 abort 해서 worktree 를 원래대로 남겨둔다. repoFilter 가 비어
// 있지 않으면 그 레포 하나만 처리한다.
func Rebase(root string, cfg config.Config, name, repoFilter string) (RebaseResult, error) {
	m, err := Load(root, name)
	if err != nil {
		return RebaseResult{}, err
	}
	result := RebaseResult{Feature: m.Name}
	for _, r := range m.Repositories {
		if repoFilter != "" && r.Name != repoFilter {
			continue
		}
		base := r.BaseBranch
		if base == "" {
			base = cfg.Git.DefaultBaseBranch
		}
		rr := RebaseRepoResult{Name: r.Name, Branch: r.Branch, BaseBranch: base}
		p := filepath.Join(root, r.WorktreePath)

		if aggit.HasChanges(p) {
			rr.Status = "skipped"
			rr.Detail = "uncommitted changes; commit or stash first"
			result.Repositories = append(result.Repositories, rr)
			continue
		}

		_ = aggit.Fetch(p) // 실패해도 무방하다. 리베이스는 로컬 ref 로 폴백한다.

		upstream := base
		if aggit.RemoteBranchExists(p, base) {
			upstream = "origin/" + base
		} else if !aggit.LocalBranchExists(p, base) {
			rr.Status = "skipped"
			rr.Detail = fmt.Sprintf("base branch %q not found", base)
			result.Repositories = append(result.Repositories, rr)
			continue
		}

		before, _ := aggit.HeadSHA(p)
		if err := aggit.RebaseOnto(p, upstream); err != nil {
			_ = aggit.RebaseAbort(p)
			rr.Status = "failed"
			rr.Detail = fmt.Sprintf("rebase onto %s failed (conflict); aborted, resolve manually", upstream)
			result.Repositories = append(result.Repositories, rr)
			continue
		}
		after, _ := aggit.HeadSHA(p)
		if before == after {
			rr.Status = "up-to-date"
			rr.Detail = fmt.Sprintf("already based on %s", upstream)
		} else {
			rr.Status = "rebased"
			rr.Detail = fmt.Sprintf("rebased onto %s", upstream)
		}
		result.Repositories = append(result.Repositories, rr)
	}
	return result, nil
}

// Push 는 각 레포의 feature 브랜치를 origin 으로 push 한다. repoFilter 가 비어 있지
// 않으면 그 레포만, 비어 있으면 모든 레포를 대상으로 한다. 올릴 커밋이 없는 레포는
// 건너뛴다.
func Push(root, name, repoFilter string) error {
	m, err := Load(root, name)
	if err != nil {
		return err
	}
	for _, r := range m.Repositories {
		if repoFilter != "" && r.Name != repoFilter {
			continue
		}
		p := filepath.Join(root, r.WorktreePath)
		remoteExists := aggit.RemoteBranchExists(p, r.Branch)
		if remoteExists && unpushedCount(p, r.Branch, r.BaseBranch) == 0 {
			output.Printf("[%s] nothing to push, skipped\n", r.Name)
			continue
		}
		output.Printf("[%s] pushing %s\n", r.Name, r.Branch)
		if err := aggit.Push(p, r.Branch); err != nil {
			output.Printf("failed: %v\n", err)
		} else {
			output.Println("pushed")
		}
	}
	return nil
}
