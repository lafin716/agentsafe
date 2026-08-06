package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/agentsafe/agentsafe/internal/applog"
	"github.com/agentsafe/agentsafe/internal/config"
	"github.com/agentsafe/agentsafe/internal/feature"
	"github.com/agentsafe/agentsafe/internal/fsutil"
	"github.com/agentsafe/agentsafe/internal/output"
	"github.com/agentsafe/agentsafe/internal/ui"
	"github.com/agentsafe/agentsafe/internal/wttemplate"
)

type Options struct {
	Repo string
	// Paths narrows the sync to specific files inside Repo — one Agent Change
	// Resolution rather than all of them. Paths are repo-relative and
	// slash-separated, matching Change.Path. Requires Repo: a bare filename is
	// ambiguous across repositories, and resolving the wrong one would write a
	// file into a worktree the user was not looking at.
	//
	// Everything else about a sync is unchanged — the risky/masked gate, the
	// RecordSync rollback snapshot and the history entry all apply to the subset,
	// which is why this is a filter on the change set rather than its own copy
	// routine.
	Paths           []string
	DryRun          bool
	IncludeRisky    bool
	AllowMaskedSync bool
	Yes             bool
}

func Diff(root string, cfg config.Config, featureName, repoFilter string) (map[string][]Change, error) {
	diffStart := time.Now()
	fm, err := feature.Load(root, featureName)
	if err != nil {
		return nil, err
	}
	pm := LoadPrepareMetadata(root, featureName)
	if err := validatePreparedRepositories(root, featureName, fm, pm, repoFilter); err != nil {
		return nil, err
	}
	result := map[string][]Change{}
	type job struct {
		name         string
		worktreePath string
	}
	jobs := make(chan job)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error
	workerCount := len(fm.Repositories)
	if workerCount > 4 {
		workerCount = 4
	}
	if workerCount == 0 {
		return result, nil
	}
	worker := func() {
		defer wg.Done()
		for r := range jobs {
			mu.Lock()
			failed := firstErr != nil
			mu.Unlock()
			if failed {
				continue
			}
			repoStart := time.Now()
			var gitignoreMs int64
			var gitignorePaths int
			pats := []string{".git/"}
			pats = append(pats, cfg.Agent.DefaultExclude...)
			templatePats, templateErr := wttemplate.AgentIgnorePatterns(root, r.name)
			if templateErr != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = templateErr
				}
				mu.Unlock()
				continue
			}
			pats = append(pats, templatePats...)
			secRoot := LoadSecurity(cfg, root)
			secSource := LoadSecurity(cfg, filepath.Join(root, r.worktreePath))
			pats = append(pats, secRoot.Ignore...)
			pats = append(pats, secSource.Ignore...)
			source := config.AgentPath(root, fm.FolderKey(), r.name)
			target := filepath.Join(root, r.worktreePath)
			// Honor the feature worktree's own .gitignore so agent build output
			// (e.g. a freshly built, possibly nested build/ dir) is not detected as
			// an ADDED change and synced back. Scan both the agent copy (where new
			// artifacts live) and the worktree (so an already-ignored worktree file
			// is not misread as DELETED) against the worktree's ignore rules.
			if cfg.Agent.GitignoreEnabled() {
				giStart := time.Now()
				gi, giErr := gitIgnoredPatterns(target, []string{source, target})
				gitignoreMs = time.Since(giStart).Milliseconds()
				if giErr != nil {
					mu.Lock()
					if firstErr == nil {
						firstErr = giErr
					}
					mu.Unlock()
					continue
				}
				pats = append(pats, gi...)
				gitignorePaths = len(gi)
				if len(gi) > 0 {
					output.Printf("[%s] .gitignore excluded %d path(s) from sync\n", r.name, len(gi))
				}
			}
			matcher := NewIgnoreMatcher(pats)
			index := preparedFileIndex(pm, r.name)
			compareStart := time.Now()
			ch, filesHashed, compareErr := CompareIndexed(
				r.name,
				source,
				target,
				matcher,
				maskedMap(pm, r.name),
				index,
			)
			compareMs := time.Since(compareStart).Milliseconds()
			if compareErr != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = compareErr
				}
				mu.Unlock()
				continue
			}
			hashes := preparedHashes(pm, r.name)
			filtered := ch[:0]
			for _, c := range ch {
				if c.Masked && hashes != nil {
					if h, hashErr := fsutil.SHA256File(filepath.Join(source, filepath.FromSlash(c.Path))); hashErr == nil && hashes[c.Path] == h {
						continue
					}
				}
				filtered = append(filtered, c)
			}
			mu.Lock()
			result[r.name] = filtered
			mu.Unlock()
			applog.Info("diff repo timing",
				"feature", featureName,
				"repo", r.name,
				"gitignoreMs", gitignoreMs,
				"gitignorePaths", gitignorePaths,
				"compareMs", compareMs,
				"filesHashed", filesHashed,
				"indexed", len(index) > 0,
				"changes", len(filtered),
				"totalMs", time.Since(repoStart).Milliseconds(),
			)
		}
	}
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go worker()
	}
	for _, r := range fm.Repositories {
		if repoFilter != "" && r.Name != repoFilter {
			continue
		}
		jobs <- job{name: r.Name, worktreePath: r.WorktreePath}
	}
	close(jobs)
	wg.Wait()
	if firstErr != nil {
		return nil, firstErr
	}
	applog.Info("diff completed",
		"feature", featureName,
		"repos", len(result),
		"ms", time.Since(diffStart).Milliseconds(),
	)
	return result, nil
}

func validatePreparedRepositories(root, featureName string, fm feature.Metadata, pm PrepareMetadata, repoFilter string) error {
	prepared := map[string]PrepareRepo{}
	for _, r := range pm.Repositories {
		prepared[r.Name] = r
	}
	var missing []string
	for _, r := range fm.Repositories {
		if repoFilter != "" && r.Name != repoFilter {
			continue
		}
		pr, ok := prepared[r.Name]
		if !ok {
			missing = append(missing, r.Name)
			continue
		}
		if st, err := os.Stat(config.AgentPath(root, fm.FolderKey(), r.Name)); err != nil || !st.IsDir() {
			missing = append(missing, r.Name)
			continue
		}
		if r.Revision > 0 && pr.WorktreeRevision != r.Revision {
			missing = append(missing, r.Name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("agent prepare required for repository(s): %s", strings.Join(missing, ", "))
	}
	return nil
}

func Sync(root string, cfg config.Config, featureName string, opt Options) error {
	// A worktree mid-rebase is a partial replay corresponding to no commit;
	// writing reviewed changes into it would tangle them with a conflict the
	// user has not finished resolving (docs/adr/0002).
	if err := GuardIntegrationInProgress(root, featureName, opt.Repo); err != nil {
		return err
	}
	byRepo, err := Diff(root, cfg, featureName, opt.Repo)
	if err != nil {
		return err
	}
	byRepo, err = filterChangesByPath(byRepo, opt.Repo, opt.Paths)
	if err != nil {
		return err
	}
	PrintChanges(featureName, byRepo)
	if opt.DryRun {
		output.Println("dry-run: no files changed")
		return nil
	}
	blocked := false
	for _, changes := range byRepo {
		for _, c := range changes {
			if c.Risky && !opt.IncludeRisky {
				output.Printf("blocked risky file: [%s] %s\n", c.Repo, c.Path)
				blocked = true
			}
			if c.Masked && !opt.AllowMaskedSync {
				output.Printf("blocked masked file: [%s] %s\n", c.Repo, c.Path)
				blocked = true
			}
		}
	}
	if blocked {
		return fmt.Errorf("sync blocked; use --include-risky and/or --allow-masked-sync only after careful review")
	}
	if !ui.Confirm("Proceed with sync?", opt.Yes) {
		return fmt.Errorf("sync cancelled")
	}
	fm, err := feature.Load(root, featureName)
	if err != nil {
		return err
	}
	applied := 0
	for _, r := range fm.Repositories {
		if opt.Repo != "" && r.Name != opt.Repo {
			continue
		}
		changes := byRepo[r.Name]
		dstRoot := filepath.Join(root, r.WorktreePath)
		// Snapshot the worktree before applying so the sync can be rolled back.
		if err := RecordSync(root, featureName, r.Name, dstRoot, changes); err != nil {
			return err
		}
		for _, c := range changes {
			src := filepath.Join(config.AgentPath(root, fm.FolderKey(), r.Name), filepath.FromSlash(c.Path))
			dst := filepath.Join(dstRoot, filepath.FromSlash(c.Path))
			if err := fsutil.EnsureInside(dstRoot, dst); err != nil {
				return err
			}
			switch c.Type {
			case Deleted:
				if err := os.Remove(dst); err != nil && !os.IsNotExist(err) {
					return err
				}
			default:
				info, err := os.Stat(src)
				if err != nil {
					return err
				}
				if err := fsutil.CopyFile(src, dst, info.Mode().Perm()); err != nil {
					return err
				}
			}
			applied++
		}
	}
	output.Printf("synced %d change(s)\n", applied)
	return nil
}

// filterChangesByPath narrows a change set to the requested repo-relative paths.
// An empty paths list means "everything", which is the whole-repository case.
//
// A path with no matching Agent Change is an error rather than a silent no-op:
// the caller asked to resolve a specific change, and having it quietly do
// nothing would read as "resolved" in the UI while the agent copy still differs.
func filterChangesByPath(byRepo map[string][]Change, repoName string, paths []string) (map[string][]Change, error) {
	if len(paths) == 0 {
		return byRepo, nil
	}
	if repoName == "" {
		return nil, fmt.Errorf("a repository is required when syncing specific paths")
	}
	wanted := make(map[string]bool, len(paths))
	for _, p := range paths {
		if p = filepath.ToSlash(strings.TrimSpace(p)); p != "" {
			wanted[p] = true
		}
	}
	if len(wanted) == 0 {
		return nil, fmt.Errorf("no valid paths given")
	}
	kept := []Change{}
	for _, c := range byRepo[repoName] {
		if wanted[c.Path] {
			kept = append(kept, c)
			delete(wanted, c.Path)
		}
	}
	if len(wanted) > 0 {
		missing := make([]string, 0, len(wanted))
		for p := range wanted {
			missing = append(missing, p)
		}
		sort.Strings(missing)
		return nil, fmt.Errorf("no agent change for path(s) in repository %s: %s",
			repoName, strings.Join(missing, ", "))
	}
	return map[string][]Change{repoName: kept}, nil
}

// SyncAndCommit syncs reviewed agent changes back to worktrees, then commits
// them with the given message. A dry-run or an empty message skips the commit,
// so callers can reuse it for a preview. The sync step reuses Sync, preserving
// the risky/masked guards, rollback snapshot, and confirmation behaviour.
func SyncAndCommit(root string, cfg config.Config, featureName, message string, opt Options) error {
	if err := Sync(root, cfg, featureName, opt); err != nil {
		return err
	}
	if opt.DryRun || strings.TrimSpace(message) == "" {
		return nil
	}
	return feature.Commit(root, featureName, message, opt.Repo)
}

// SyncCommitPush chains sync → commit → push into one operation. When message is
// empty the workspace's Commit Message Template supplies one. risky/masked files
// stay gated by opt: with IncludeRisky/AllowMaskedSync false, Sync aborts before
// any commit or push so masked secrets never reach the worktree. A dry run stops
// after the sync diff. feature.Commit/Push are no-ops ("clean"/"nothing to
// push") when there is nothing to do, so an empty change set is not an error.
func SyncCommitPush(root string, cfg config.Config, featureName, message string, opt Options) error {
	if strings.TrimSpace(message) == "" {
		// Rendered once here rather than per repository: every variable comes from
		// Feature metadata or the clock, so one message serves them all and
		// feature.Commit keeps taking a plain string.
		values, err := CommitMessageValuesFor(root, featureName, time.Now())
		if err != nil {
			return err
		}
		message = CommitMessageFor(cfg.Git.CommitMessageTemplate, values)
	}
	if err := SyncAndCommit(root, cfg, featureName, message, opt); err != nil {
		return err
	}
	if opt.DryRun {
		return nil
	}
	res, err := feature.Push(root, featureName, opt.Repo, feature.PushOptions{})
	if err != nil {
		return err
	}
	// The sync and the commit already happened, so a failed push is not a reason
	// to undo anything — but it is a reason for this call to fail. Reporting
	// success while a branch never reached origin is exactly what the per-repo
	// result exists to prevent.
	if res.Failed() {
		return fmt.Errorf("synced and committed, but the push failed: %s",
			strings.Join(res.FailureSummaries(), "; "))
	}
	return nil
}

// CommitMessageValuesFor reads the Feature's metadata into the substitutions a
// Commit Message Template can use.
func CommitMessageValuesFor(root, featureName string, now time.Time) (CommitMessageValues, error) {
	fm, err := feature.Load(root, featureName)
	if err != nil {
		return CommitMessageValues{}, err
	}
	return CommitMessageValues{
		Feature: fm.Name,
		Branch:  fm.Branch,
		Base:    fm.BaseBranch,
		Now:     now,
	}, nil
}

// DefaultCommitMessage is the built-in fallback used when the workspace has no
// Commit Message Template, or has one that cannot be rendered. now is passed in
// so a preview and the commit it previews agree.
func DefaultCommitMessage(featureName string, now time.Time) string {
	return fmt.Sprintf("agent(%s): auto-sync %s", featureName, now.Format(time.RFC3339))
}

// RestoreFromWorktree overwrites one file in the prepared agent workspace with
// the current worktree version. If the file no longer exists in the worktree,
// the agent copy is removed. This lets the desktop UI dismiss an agent diff
// entry that was caused by direct worktree edits.
func RestoreFromWorktree(root string, cfg config.Config, featureName, repoName, relPath string) error {
	relPath = filepath.ToSlash(strings.TrimSpace(relPath))
	if relPath == "" || filepath.IsAbs(relPath) || strings.HasPrefix(relPath, "../") || strings.Contains(relPath, "/../") || relPath == ".." {
		return fmt.Errorf("invalid file path %q", relPath)
	}
	// Mid-integration the worktree file holds conflict markers, so copying it
	// into the agent workspace would poison the agent copy (docs/adr/0002).
	if err := GuardIntegrationInProgress(root, featureName, repoName); err != nil {
		return err
	}
	fm, err := feature.Load(root, featureName)
	if err != nil {
		return err
	}
	pm := LoadPrepareMetadata(root, featureName)
	if err := validatePreparedRepositories(root, featureName, fm, pm, repoName); err != nil {
		return err
	}
	var repoMeta *feature.RepoMeta
	for i := range fm.Repositories {
		if fm.Repositories[i].Name == repoName {
			repoMeta = &fm.Repositories[i]
			break
		}
	}
	if repoMeta == nil {
		return fmt.Errorf("repository %q is not part of feature %q", repoName, featureName)
	}
	agentRoot := config.AgentPath(root, fm.FolderKey(), repoName)
	worktreeRoot := filepath.Join(root, filepath.FromSlash(repoMeta.WorktreePath))
	agentPath := filepath.Join(agentRoot, filepath.FromSlash(relPath))
	worktreePath := filepath.Join(worktreeRoot, filepath.FromSlash(relPath))
	if err := fsutil.EnsureInside(agentRoot, agentPath); err != nil {
		return err
	}
	if err := fsutil.EnsureInside(worktreeRoot, worktreePath); err != nil {
		return err
	}
	info, err := os.Stat(worktreePath)
	if err != nil {
		if os.IsNotExist(err) {
			if removeErr := os.Remove(agentPath); removeErr != nil && !os.IsNotExist(removeErr) {
				return removeErr
			}
			return nil
		}
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("cannot restore directory %q", relPath)
	}
	if err := os.MkdirAll(filepath.Dir(agentPath), 0755); err != nil {
		return err
	}
	return fsutil.CopyFile(worktreePath, agentPath, info.Mode().Perm())
}

// RestoreRepoFromWorktree overwrites every changed file in one repository's
// prepared agent workspace with its current worktree version, returning the
// number of files restored. It reuses Diff (scoped to the repo) for the change
// list and the per-file RestoreFromWorktree for each file.
func RestoreRepoFromWorktree(root string, cfg config.Config, featureName, repoName string) (int, error) {
	if err := GuardIntegrationInProgress(root, featureName, repoName); err != nil {
		return 0, err
	}
	changes, err := Diff(root, cfg, featureName, repoName)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, c := range changes[repoName] {
		if err := RestoreFromWorktree(root, cfg, featureName, repoName, c.Path); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}
