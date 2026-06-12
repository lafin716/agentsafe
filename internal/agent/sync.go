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
			matcher := NewIgnoreMatcher(pats)
			source := config.AgentPath(root, featureName, r.name)
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
		if st, err := os.Stat(config.AgentPath(root, featureName, r.Name)); err != nil || !st.IsDir() {
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
			src := filepath.Join(config.AgentPath(root, featureName, r.Name), filepath.FromSlash(c.Path))
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
