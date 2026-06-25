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

type Metadata struct {
	Name string `json:"name"`
	// Key is the ASCII-safe identifier used for on-disk worktree/agent folders
	// (root/feature/<key>, root/agent/<key>). It is derived from Name at create
	// time so folders never contain characters (e.g. Hangul) that break editors
	// like IntelliJ. Empty for features created before this field existed, in
	// which case FolderKey falls back to Name.
	Key          string     `json:"key,omitempty"`
	Branch       string     `json:"branch"`
	BaseBranch   string     `json:"baseBranch"`
	CreatedAt    string     `json:"createdAt"`
	Revision     int        `json:"revision,omitempty"`
	Repositories []RepoMeta `json:"repositories"`
}

// FolderKey returns the ASCII folder key for the feature, falling back to Name
// for features created before Key was introduced.
func (m Metadata) FolderKey() string {
	if m.Key != "" {
		return m.Key
	}
	return m.Name
}

type RepoMeta struct {
	Name         string `json:"name"`
	WorktreePath string `json:"worktreePath"`
	Branch       string `json:"branch"`
	BaseBranch   string `json:"baseBranch"`
	Revision     int    `json:"revision,omitempty"`
}

func BranchName(cfg config.Config, featureName string) string {
	return cfg.Git.BranchPrefix + featureName
}

type ExistingBranchPolicy string

const (
	ExistingBranchError    ExistingBranchPolicy = "error"
	ExistingBranchReuse    ExistingBranchPolicy = "reuse"
	ExistingBranchRecreate ExistingBranchPolicy = "recreate"
)

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

type CreateOptions struct {
	Base           string
	ExistingBranch ExistingBranchPolicy
}

type CreateCheck struct {
	Name         string                  `json:"name"`
	Branch       string                  `json:"branch"`
	HasConflicts bool                    `json:"hasConflicts"`
	Blocked      bool                    `json:"blocked"`
	Repositories []RepositoryCreateCheck `json:"repositories"`
}

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

type RepositoryWorktreeOptions struct {
	ExistingBranch ExistingBranchPolicy
	Recreate       bool
	Force          bool
}

func Load(root, name string) (Metadata, error) {
	b, err := os.ReadFile(config.FeatureMetaPath(root, name))
	if err != nil {
		return Metadata{}, err
	}
	var m Metadata
	return m, json.Unmarshal(b, &m)
}

func Save(root string, m Metadata) error {
	if err := os.MkdirAll(filepath.Dir(config.FeatureMetaPath(root, m.Name)), 0755); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(m, "", "  ")
	return os.WriteFile(config.FeatureMetaPath(root, m.Name), b, 0644)
}

// Create is retained for callers using the former force flag.
func Create(root string, cfg config.Config, name, base string, force bool) error {
	policy := ExistingBranchError
	if force {
		policy = ExistingBranchRecreate
	}
	return CreateWithOptions(root, cfg, name, CreateOptions{Base: base, ExistingBranch: policy})
}

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

// CheckCreate inspects every configured repository before feature creation.
// It does not create, delete, or check out branches or worktrees.
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
			// Inspect the remote without updating remote-tracking refs.
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

// uniqueFeatureKey derives an ASCII folder key from name (config.FeatureKey)
// and ensures it does not collide with an existing feature's folder by
// appending a numeric suffix when needed.
func uniqueFeatureKey(root, name string) string {
	base := config.FeatureKey(name)
	candidate := base
	for i := 2; featureKeyTaken(root, candidate); i++ {
		candidate = fmt.Sprintf("%s-%d", base, i)
	}
	return candidate
}

// featureKeyTaken reports whether a worktree/agent folder or an existing
// feature's metadata already uses key as its folder key.
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
		// Discover remote-only feature branches, including a remote counterpart
		// for an existing local branch whose remote-tracking ref is stale.
		_ = aggit.FetchAll(repoPath)
		remote = aggit.RemoteBranchExists(repoPath, branch)
	}

	_ = aggit.WorktreePrune(repoPath)
	if inUse := aggit.WorktreeForBranch(repoPath, branch); inUse != "" {
		// A previous attempt may have successfully created the target worktree
		// but failed before feature metadata was saved. Adopt that exact
		// worktree so retrying the repository add is idempotent.
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
		// A newly cloned repository can have the feature branch checked out in
		// its main clone (for example when the remote HEAD points at it). Move a
		// clean main clone back to the base branch so the feature branch can be
		// attached to the requested worktree.
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

// configureWorktreeUpstream makes a new feature branch track its base branch
// until its first push. Reused remote feature branches track their matching
// origin branch, while a reused local branch keeps an existing valid upstream.
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

// ConfigureRepositoryWorktree adds a repository missing from a feature or
// recreates an existing repository worktree without touching the other repos.
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
		// Adding a repository that is not yet part of the feature. A worktree
		// folder may still linger on disk from a repository created earlier with
		// the same name; git worktree add would then fail with "already exists".
		// A folder that is the feature branch's own worktree is adoptable, so
		// only block leftover/foreign folders.
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

// DeleteOptions controls feature deletion. DeleteBranch also removes the local
// feature branch in each repository; Force removes worktrees that have
// uncommitted changes (otherwise deletion is refused).
type DeleteOptions struct {
	DeleteBranch bool
	Force        bool
}

// DeleteResult reports non-fatal cleanup failures. A feature deletion keeps
// going after these failures so other repositories and feature artifacts are
// still removed.
type DeleteResult struct {
	Warnings []string `json:"warnings" yaml:"warnings"`
}

// Delete removes a feature's worktrees and all of its artifacts: the feature
// metadata, the agent workspace, session metadata, and sync history. When
// DeleteBranch is set, the local feature branch is removed from each repo too.
// Unless Force is set, deletion is refused if any worktree has uncommitted
// changes (nothing is removed in that case, to avoid a partial delete).
func Delete(root, name string, opt DeleteOptions) error {
	_, err := DeleteWithResult(root, name, opt)
	return err
}

// DeleteWithResult performs the same deletion as Delete and returns warnings
// for cleanup steps that failed without aborting the overall deletion.
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

	// Clean up all feature artifacts. Every path is attempted even if an earlier
	// removal failed.
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

type FeatureListResult struct {
	Features []FeatureEntry `json:"features" yaml:"features"`
}

type FeatureEntry struct {
	Name       string `json:"name"       yaml:"name"`
	Branch     string `json:"branch"     yaml:"branch"`
	BaseBranch string `json:"baseBranch" yaml:"baseBranch"`
	RepoCount  int    `json:"repoCount"  yaml:"repoCount"`
	AgentReady bool   `json:"agentReady" yaml:"agentReady"`
}

type FeatureStatusResult struct {
	Feature           string       `json:"feature"      yaml:"feature"`
	Branch            string       `json:"branch"       yaml:"branch"`
	AgentReady        bool         `json:"agentReady"   yaml:"agentReady"`
	AgentNeedsPrepare bool         `json:"agentNeedsPrepare" yaml:"agentNeedsPrepare"`
	Repositories      []RepoStatus `json:"repositories" yaml:"repositories"`
}

type RepoStatus struct {
	Name              string           `json:"name"              yaml:"name"`
	Status            string           `json:"status"            yaml:"status"`
	Changes           []RepoFileStatus `json:"changes"           yaml:"changes"`
	AgentReady        bool             `json:"agentReady"        yaml:"agentReady"`
	AgentNeedsPrepare bool             `json:"agentNeedsPrepare" yaml:"agentNeedsPrepare"`
	// Ahead is the number of commits the feature branch has that are not yet
	// pushed (to origin/<branch> when it exists, otherwise relative to the base
	// branch). It is what a push would publish.
	Ahead int    `json:"ahead"             yaml:"ahead"`
	Error string `json:"error,omitempty"   yaml:"error,omitempty"`
}

// unpushedCount returns how many commits on the feature branch are not yet
// pushed. When origin/<branch> exists it counts commits ahead of it; otherwise
// (branch never pushed) it counts the commits the branch adds over its base.
func unpushedCount(path, branch, base string) int {
	if branch == "" {
		return 0
	}
	// Try candidate ranges in the original priority order (origin/<branch>, then
	// origin/<base>, then the local base) and use the first ref that resolves.
	// Letting rev-list's own error report a missing ref drops the separate
	// RemoteBranchExists guard spawns — each git subprocess is ~2s on Windows
	// with AV scanning, and status is on the worktree-detail hot path.
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

type RepoFileStatus struct {
	Code string `json:"code" yaml:"code"`
	Type string `json:"type" yaml:"type"`
	Path string `json:"path" yaml:"path"`
}

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
	// Each repo's status runs several git subprocesses; on Windows each spawn is
	// comparatively expensive, so repos are scanned in parallel (bounded) and the
	// per-repo working-tree status and unpushed-count run concurrently. Results
	// are written by index to preserve m.Repositories order; readiness is reduced
	// after the pool joins so there is no shared mutable aggregation in flight.
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

// repoStatusFor computes one repository's status. The working-tree status and
// the unpushed-count are independent git invocations, so they run concurrently
// and the per-repo wall time is the slower of the two rather than their sum.
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
			// Legacy metadata has revision 0; it remains valid until this
			// repository's worktree receives its first revision.
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

// Commit commits the worktree changes in each repository. When repoFilter is
// non-empty, only that repository is committed; otherwise every repository is.
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

type RebaseRepoResult struct {
	Name       string `json:"name"       yaml:"name"`
	Branch     string `json:"branch"     yaml:"branch"`
	BaseBranch string `json:"baseBranch" yaml:"baseBranch"`
	Status     string `json:"status"     yaml:"status"` // rebased | up-to-date | skipped | failed
	Detail     string `json:"detail"     yaml:"detail"`
}
type RebaseResult struct {
	Feature      string             `json:"feature"      yaml:"feature"`
	Repositories []RebaseRepoResult `json:"repositories" yaml:"repositories"`
}

// Rebase replays each feature worktree's branch onto the latest base branch
// (origin/<base> when available). Worktrees with uncommitted changes are
// skipped, and a rebase that hits conflicts is aborted so the worktree is left
// untouched. repoFilter, when non-empty, limits the operation to one repository.
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

		_ = aggit.Fetch(p) // best-effort; rebase falls back to local refs

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

// Push pushes each repository's feature branch to origin. When repoFilter is
// non-empty, only that repository is pushed; otherwise every repository is.
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
