package agent

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/agentsafe/agentsafe/internal/config"
	"github.com/agentsafe/agentsafe/internal/feature"
	"github.com/agentsafe/agentsafe/internal/fsutil"
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
	result := map[string][]Change{}
	for _, r := range fm.Repositories {
		if repoFilter != "" && r.Name != repoFilter {
			continue
		}
		pats := []string{".git/"}
		pats = append(pats, cfg.Agent.DefaultExclude...)
		matcher := NewIgnoreMatcher(pats)
		source := config.AgentPath(root, featureName, r.Name)
		target := filepath.Join(root, r.WorktreePath)
		ch, err := Compare(r.Name, source, target, matcher, maskedMap(pm, r.Name))
		if err != nil {
			return nil, err
		}
		hashes := preparedHashes(pm, r.Name)
		filtered := ch[:0]
		for _, c := range ch {
			if c.Masked && hashes != nil {
				if h, err := fsutil.SHA256File(filepath.Join(source, filepath.FromSlash(c.Path))); err == nil && hashes[c.Path] == h {
					continue
				}
			}
			filtered = append(filtered, c)
		}
		ch = filtered
		result[r.Name] = ch
	}
	return result, nil
}

func Sync(root string, cfg config.Config, featureName string, opt Options) error {
	byRepo, err := Diff(root, cfg, featureName, opt.Repo)
	if err != nil {
		return err
	}
	PrintChanges(featureName, byRepo)
	if opt.DryRun {
		fmt.Println("dry-run: no files changed")
		return nil
	}
	blocked := false
	for _, changes := range byRepo {
		for _, c := range changes {
			if c.Risky && !opt.IncludeRisky {
				fmt.Printf("blocked risky file: [%s] %s\n", c.Repo, c.Path)
				blocked = true
			}
			if c.Masked && !opt.AllowMaskedSync {
				fmt.Printf("blocked masked file: [%s] %s\n", c.Repo, c.Path)
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
	fmt.Printf("synced %d change(s)\n", applied)
	return nil
}
