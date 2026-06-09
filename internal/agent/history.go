package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"github.com/agentsafe/agentsafe/internal/config"
	"github.com/agentsafe/agentsafe/internal/fsutil"
)

// SyncChange records one file operation applied during a sync.
type SyncChange struct {
	Path string `json:"path"`
	Type string `json:"type"` // ADDED | MODIFIED | DELETED
}

// SyncRecord is one entry in a repository's sync-history stack. Each record has
// a backup/ directory holding the prior worktree content of MODIFIED/DELETED
// files so the sync can be rolled back.
type SyncRecord struct {
	ID       string       `json:"id"`
	Feature  string       `json:"feature"`
	Repo     string       `json:"repo"`
	SyncedAt string       `json:"syncedAt"`
	Changes  []SyncChange `json:"changes"`
}

const historyManifest = "manifest.json"

// RecordSync snapshots the worktree files a sync is about to overwrite/delete
// and writes a manifest, pushing a new entry onto the repo's history stack.
// It must be called BEFORE the changes are applied to the worktree.
func RecordSync(root, feature, repo, worktreeRoot string, changes []Change) error {
	if len(changes) == 0 {
		return nil
	}
	id := strconv.FormatInt(time.Now().UnixNano(), 10)
	entryDir := config.HistoryEntryDir(root, feature, repo, id)
	if err := os.MkdirAll(entryDir, 0755); err != nil {
		return err
	}
	rec := SyncRecord{
		ID:       id,
		Feature:  feature,
		Repo:     repo,
		SyncedAt: time.Now().Format(time.RFC3339),
	}
	for _, c := range changes {
		rec.Changes = append(rec.Changes, SyncChange{Path: c.Path, Type: string(c.Type)})
		if c.Type == Modified || c.Type == Deleted {
			src := filepath.Join(worktreeRoot, filepath.FromSlash(c.Path))
			info, err := os.Stat(src)
			if err != nil {
				continue // nothing to back up
			}
			dst := filepath.Join(entryDir, "backup", filepath.FromSlash(c.Path))
			if err := fsutil.CopyFile(src, dst, info.Mode().Perm()); err != nil {
				return err
			}
		}
	}
	b, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(entryDir, historyManifest), b, 0644)
}

func loadRecord(dir string) (SyncRecord, bool) {
	b, err := os.ReadFile(filepath.Join(dir, historyManifest))
	if err != nil {
		return SyncRecord{}, false
	}
	var rec SyncRecord
	if json.Unmarshal(b, &rec) != nil {
		return SyncRecord{}, false
	}
	return rec, true
}

func sortRecords(recs []SyncRecord) {
	sort.Slice(recs, func(i, j int) bool {
		if recs[i].SyncedAt != recs[j].SyncedAt {
			return recs[i].SyncedAt > recs[j].SyncedAt
		}
		return recs[i].ID > recs[j].ID
	})
}

// ListHistory returns a repository's sync records, newest first.
func ListHistory(root, feature, repo string) ([]SyncRecord, error) {
	dir := config.HistoryRepoDir(root, feature, repo)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []SyncRecord{}, nil
		}
		return nil, err
	}
	out := []SyncRecord{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if rec, ok := loadRecord(filepath.Join(dir, e.Name())); ok {
			out = append(out, rec)
		}
	}
	sortRecords(out)
	return out, nil
}

// ListAllHistory collects sync records across every feature/repo, newest first.
func ListAllHistory(root string) ([]SyncRecord, error) {
	base := config.HistoryDir(root)
	features, err := os.ReadDir(base)
	if err != nil {
		if os.IsNotExist(err) {
			return []SyncRecord{}, nil
		}
		return nil, err
	}
	out := []SyncRecord{}
	for _, fe := range features {
		if !fe.IsDir() {
			continue
		}
		repos, err := os.ReadDir(filepath.Join(base, fe.Name()))
		if err != nil {
			continue
		}
		for _, re := range repos {
			if !re.IsDir() {
				continue
			}
			recs, err := ListHistory(root, fe.Name(), re.Name())
			if err != nil {
				continue
			}
			out = append(out, recs...)
		}
	}
	sortRecords(out)
	return out, nil
}

// HistoryDepth is the number of sync records in a repository's stack.
func HistoryDepth(root, feature, repo string) int {
	recs, _ := ListHistory(root, feature, repo)
	return len(recs)
}

// Rollback undoes the most recent sync for a repository, restoring the worktree
// to its pre-sync state and popping the entry. Only the top of the stack may be
// rolled back.
func Rollback(root, feature, repo, id, worktreeRoot string) error {
	recs, err := ListHistory(root, feature, repo)
	if err != nil {
		return err
	}
	if len(recs) == 0 {
		return fmt.Errorf("no sync history for %s/%s", feature, repo)
	}
	top := recs[0]
	if top.ID != id {
		return fmt.Errorf("only the most recent sync can be rolled back")
	}
	entryDir := config.HistoryEntryDir(root, feature, repo, id)
	for _, c := range top.Changes {
		dst := filepath.Join(worktreeRoot, filepath.FromSlash(c.Path))
		if err := fsutil.EnsureInside(worktreeRoot, dst); err != nil {
			return err
		}
		switch c.Type {
		case string(Added):
			if err := os.Remove(dst); err != nil && !os.IsNotExist(err) {
				return err
			}
		default: // MODIFIED | DELETED -> restore the backed-up content
			backup := filepath.Join(entryDir, "backup", filepath.FromSlash(c.Path))
			info, err := os.Stat(backup)
			if err != nil {
				continue // nothing was backed up (source missing at sync time)
			}
			if err := fsutil.CopyFile(backup, dst, info.Mode().Perm()); err != nil {
				return err
			}
		}
	}
	return os.RemoveAll(entryDir)
}
