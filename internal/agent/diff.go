package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/agentsafe/agentsafe/internal/fsutil"
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
	size int64
	hash string
}

func scanFiles(root string, matcher IgnoreMatcher) (map[string]fileInfo, error) {
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
		h, err := fsutil.SHA256File(path)
		if err != nil {
			return err
		}
		out[rel] = fileInfo{size: info.Size(), hash: h}
		return nil
	})
	return out, err
}

func Compare(repoName, source, target string, matcher IgnoreMatcher, masked map[string]bool) ([]Change, error) {
	s, err := scanFiles(source, matcher)
	if err != nil {
		return nil, err
	}
	t, err := scanFiles(target, matcher)
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
		switch {
		case sok && !tok:
			changes = append(changes, Change{Repo: repoName, Type: Added, Path: k, Risky: IsRisky(k), Masked: masked[k]})
		case !sok && tok:
			changes = append(changes, Change{Repo: repoName, Type: Deleted, Path: k, Risky: IsRisky(k), Masked: masked[k]})
		case si.size != ti.size || si.hash != ti.hash:
			changes = append(changes, Change{Repo: repoName, Type: Modified, Path: k, Risky: IsRisky(k), Masked: masked[k]})
		}
	}
	return changes, nil
}

func PrintChanges(feature string, byRepo map[string][]Change) {
	fmt.Printf("Feature: %s\n\n", feature)
	repos := make([]string, 0, len(byRepo))
	for r := range byRepo {
		repos = append(repos, r)
	}
	sort.Strings(repos)
	for _, r := range repos {
		fmt.Printf("[%s]\n", r)
		if len(byRepo[r]) == 0 {
			fmt.Println("NO CHANGES")
			fmt.Println()
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
			fmt.Printf("%-8s %s%s\n", c.Type, c.Path, flags)
		}
		fmt.Println()
	}
}

func IsRisky(rel string) bool {
	return NewIgnoreMatcher([]string{".env", ".env.*", "*.pem", "*.key", "*.p12", "*.jks", "application-secret.yml", "application-local.yml", "agentsafe.yaml", "mask.json", ".agentignore", "secrets.yml", "credentials.yml"}).Match(rel, false)
}
