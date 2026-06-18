package repo

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/agentsafe/agentsafe/internal/config"
	"github.com/agentsafe/agentsafe/internal/feature"
	"github.com/agentsafe/agentsafe/internal/fsutil"
	aggit "github.com/agentsafe/agentsafe/internal/git"
	"github.com/agentsafe/agentsafe/internal/ui"
)

func List(cfg config.Config) {
	rows := [][]string{}
	for _, r := range cfg.Repositories {
		rows = append(rows, []string{r.Name, r.URL})
	}
	ui.PrintRows([]string{"NAME", "URL"}, rows)
}

func EnsureConfigured(cfg config.Config) error {
	if len(cfg.Repositories) == 0 {
		return fmt.Errorf("no repositories configured; use `agentsafe repo add ...` first")
	}
	return nil
}

// RemoveResult reports non-fatal cleanup warnings collected while deleting
// cloned files. Removal keeps going after these so the config entry is still
// dropped and other artifacts are still cleaned.
type RemoveResult struct {
	Warnings []string `json:"warnings" yaml:"warnings"`
}

// Remove drops a repository from the workspace config. When deleteFiles is true
// it also deletes the cloned files: the main clone (main/<repo>) and, for every
// feature that includes the repo, its worktree, agent copy and sync history,
// dropping the repo from each feature's metadata so no broken worktree is left
// behind. The whole main clone is removed, so git worktree deregistration is
// unnecessary (the registration disappears with the clone).
func Remove(root string, cfg config.Config, name string, deleteFiles bool) (config.Config, RemoveResult, error) {
	var res RemoveResult
	found := false
	for _, r := range cfg.Repositories {
		if r.Name == name {
			found = true
			break
		}
	}
	if !found {
		return cfg, res, fmt.Errorf("repository %q not found", name)
	}

	if deleteFiles {
		repoPath := config.RepoPath(root, name)
		if list, err := feature.ListData(root); err == nil {
			for _, fe := range list.Features {
				m, err := feature.Load(root, fe.Name)
				if err != nil {
					continue
				}
				idx := -1
				for i, rm := range m.Repositories {
					if rm.Name == name {
						idx = i
						break
					}
				}
				if idx < 0 {
					continue
				}
				rm := m.Repositories[idx]
				folderKey := m.FolderKey()
				// Remove the registered worktree via git first (it handles its
				// own read-only files and deregisters the worktree), then force
				// remove any leftovers. This mirrors feature.DeleteWithResult.
				worktree := config.WorktreePath(root, folderKey, name)
				if rm.WorktreePath != "" {
					worktree = filepath.Join(root, filepath.FromSlash(rm.WorktreePath))
				}
				if _, statErr := os.Stat(worktree); statErr == nil {
					if err := aggit.RemoveWorktree(repoPath, worktree, true); err != nil {
						_ = aggit.WorktreePrune(repoPath)
						if err := fsutil.ForceRemoveAll(worktree); err != nil {
							res.Warnings = append(res.Warnings, fmt.Sprintf("%s: %v", worktree, err))
						}
					}
				}
				for _, p := range []string{
					config.AgentPath(root, folderKey, name),
					config.HistoryRepoDir(root, fe.Name, name),
					config.HistoryRepoDir(root, folderKey, name),
				} {
					if err := fsutil.ForceRemoveAll(p); err != nil {
						res.Warnings = append(res.Warnings, fmt.Sprintf("%s: %v", p, err))
					}
				}
				m.Repositories = append(m.Repositories[:idx], m.Repositories[idx+1:]...)
				if err := feature.Save(root, m); err != nil {
					res.Warnings = append(res.Warnings, fmt.Sprintf("feature %s metadata: %v", fe.Name, err))
				}
				// Drop now-empty parent directories so no stray folder is left.
				removeIfEmpty(filepath.Join(root, "feature", folderKey))
				removeIfEmpty(filepath.Join(root, "feature", fe.Name))
				removeIfEmpty(filepath.Join(root, "agent", folderKey))
				removeIfEmpty(filepath.Join(root, "agent", fe.Name))
			}
		}
		if err := fsutil.ForceRemoveAll(repoPath); err != nil {
			res.Warnings = append(res.Warnings, fmt.Sprintf("%s: %v", repoPath, err))
		}
	}

	newCfg, err := config.RemoveRepository(root, cfg, name)
	return newCfg, res, err
}

// removeIfEmpty deletes dir only when it exists and is empty.
func removeIfEmpty(dir string) {
	if empty, err := fsutil.IsEmptyDir(dir); err == nil && empty {
		_ = os.Remove(dir)
	}
}
