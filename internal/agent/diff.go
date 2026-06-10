package agent

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/agentsafe/agentsafe/internal/fsutil"
	"github.com/agentsafe/agentsafe/internal/output"
)

type ChangeType string

const (
	Added    ChangeType = "ADDED"
	Modified ChangeType = "MODIFIED"
	Deleted  ChangeType = "DELETED"
)

type Change struct {
	Repo   string     `json:"repo"   yaml:"repo"`
	Type   ChangeType `json:"type"   yaml:"type"`
	Path   string     `json:"path"   yaml:"path"`
	Risky  bool       `json:"risky"  yaml:"risky"`
	Masked bool       `json:"masked" yaml:"masked"`
}

type fileInfo struct {
	size        int64
	modTimeNano int64
	hash        string
}

type hashFileFunc func(string) (string, error)

func scanFiles(root string, matcher IgnoreMatcher, withHashes bool, hashFile hashFileFunc) (map[string]fileInfo, error) {
	out := map[string]fileInfo{}
	if _, err := os.Stat(root); err != nil {
		return out, nil
	}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if matcher.Match(rel, d.IsDir()) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		h := ""
		if withHashes {
			h, err = hashFile(path)
			if err != nil {
				return err
			}
		}
		out[rel] = fileInfo{size: info.Size(), modTimeNano: info.ModTime().UnixNano(), hash: h}
		return nil
	})
	return out, err
}

func Compare(repoName, source, target string, matcher IgnoreMatcher, masked map[string]bool) ([]Change, error) {
	return compare(repoName, source, target, matcher, masked, nil, fsutil.SHA256File)
}

// CompareIndexed uses prepare-time stat metadata as a Git-like index. Files
// whose size and modification time still match both snapshots need no content
// reads; only possible changes are hashed for confirmation.
func CompareIndexed(repoName, source, target string, matcher IgnoreMatcher, masked map[string]bool, index map[string]FileIndexEntry) ([]Change, error) {
	if len(index) == 0 {
		return Compare(repoName, source, target, matcher, masked)
	}
	return compare(repoName, source, target, matcher, masked, index, fsutil.SHA256File)
}

func compare(repoName, source, target string, matcher IgnoreMatcher, masked map[string]bool, index map[string]FileIndexEntry, hashFile hashFileFunc) ([]Change, error) {
	withHashes := len(index) == 0
	s, err := scanFiles(source, matcher, withHashes, hashFile)
	if err != nil {
		return nil, err
	}
	t, err := scanFiles(target, matcher, withHashes, hashFile)
	if err != nil {
		return nil, err
	}
	keys := map[string]bool{}
	for k := range s {
		keys[k] = true
	}
	for k := range t {
		keys[k] = true
	}
	var sorted []string
	for k := range keys {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)
	var changes []Change
	for _, k := range sorted {
		si, sok := s[k]
		ti, tok := t[k]
		var change *Change
		switch {
		case sok && !tok:
			c := Change{Repo: repoName, Type: Added, Path: k, Risky: IsRisky(k), Masked: masked[k]}
			change = &c
		case !sok && tok:
			c := Change{Repo: repoName, Type: Deleted, Path: k, Risky: IsRisky(k), Masked: masked[k]}
			change = &c
		case withHashes:
			if si.size != ti.size || si.hash != ti.hash {
				c := Change{Repo: repoName, Type: Modified, Path: k, Risky: IsRisky(k), Masked: masked[k]}
				change = &c
			}
		default:
			baseline, indexed := index[k]
			if indexed &&
				si.size == baseline.Agent.Size &&
				si.modTimeNano == baseline.Agent.ModTimeNano &&
				ti.size == baseline.Worktree.Size &&
				ti.modTimeNano == baseline.Worktree.ModTimeNano &&
				(baseline.Agent.Hash == baseline.Worktree.Hash || masked[k]) {
				continue
			}
			if si.size != ti.size {
				c := Change{Repo: repoName, Type: Modified, Path: k, Risky: IsRisky(k), Masked: masked[k]}
				change = &c
				break
			}
			sourceHash, err := hashFile(filepath.Join(source, filepath.FromSlash(k)))
			if err != nil {
				return nil, err
			}
			targetHash, err := hashFile(filepath.Join(target, filepath.FromSlash(k)))
			if err != nil {
				return nil, err
			}
			si.hash, ti.hash = sourceHash, targetHash
			if sourceHash != targetHash {
				c := Change{Repo: repoName, Type: Modified, Path: k, Risky: IsRisky(k), Masked: masked[k]}
				change = &c
			}
		}
		if change == nil {
			continue
		}
		// Preserve the existing masking rule: an unchanged prepared agent copy
		// is not a user edit, even when it differs from the live worktree.
		if change.Masked && sok {
			currentHash := si.hash
			if currentHash == "" {
				currentHash, err = hashFile(filepath.Join(source, filepath.FromSlash(k)))
				if err != nil {
					return nil, err
				}
			}
			if baseline, ok := index[k]; ok && baseline.Agent.Hash == currentHash {
				continue
			}
		}
		changes = append(changes, *change)
	}
	return changes, nil
}

func PrintChanges(feature string, byRepo map[string][]Change) {
	output.Printf("Feature: %s\n\n", feature)
	repos := make([]string, 0, len(byRepo))
	for r := range byRepo {
		repos = append(repos, r)
	}
	sort.Strings(repos)
	for _, r := range repos {
		output.Printf("[%s]\n", r)
		if len(byRepo[r]) == 0 {
			output.Println("NO CHANGES")
			output.Println()
			continue
		}
		for _, c := range byRepo[r] {
			flags := ""
			if c.Risky {
				flags += " RISKY"
			}
			if c.Masked {
				flags += " MASKED"
			}
			output.Printf("%-8s %s%s\n", c.Type, c.Path, flags)
		}
		output.Println()
	}
}

func IsRisky(rel string) bool {
	return NewIgnoreMatcher([]string{".env", ".env.*", "*.pem", "*.key", "*.p12", "*.jks", "application-secret.yml", "application-local.yml", "agentsafe.yaml", "mask.json", ".agentignore", "secrets.yml", "credentials.yml"}).Match(rel, false)
}
