package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/agentsafe/agentsafe/internal/config"
	"github.com/agentsafe/agentsafe/internal/feature"
	"github.com/agentsafe/agentsafe/internal/fsutil"
	"github.com/agentsafe/agentsafe/internal/output"
	"github.com/agentsafe/agentsafe/internal/ui"
	"github.com/agentsafe/agentsafe/internal/wttemplate"
)

type Options struct {
	Repo            string
	DryRun          bool
	IncludeRisky    bool
	AllowMaskedSync bool
	Yes             bool
}

func Diff(root string, cfg config.Config, featureName, repoFilter string) (map[string][]Change, error) {
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
			matcher := NewIgnoreMatcher(pats)
			source := config.AgentPath(root, fm.FolderKey(), r.name)
			target := filepath.Join(root, r.worktreePath)
			ch, compareErr := CompareIndexed(
				r.name,
				source,
				target,
				matcher,
				maskedMap(pm, r.name),
				preparedFileIndex(pm, r.name),
			)
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
	byRepo, err := Diff(root, cfg, featureName, opt.Repo)
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

// RestoreFromWorktree overwrites one file in the prepared agent workspace with
// the current worktree version. If the file no longer exists in the worktree,
// the agent copy is removed. This lets the desktop UI dismiss an agent diff
// entry that was caused by direct worktree edits.
func RestoreFromWorktree(root string, cfg config.Config, featureName, repoName, relPath string) error {
	relPath = filepath.ToSlash(strings.TrimSpace(relPath))
	if relPath == "" || filepath.IsAbs(relPath) || strings.HasPrefix(relPath, "../") || strings.Contains(relPath, "/../") || relPath == ".." {
		return fmt.Errorf("invalid file path %q", relPath)
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
