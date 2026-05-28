package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/agentsafe/agentsafe/internal/config"
	"github.com/agentsafe/agentsafe/internal/feature"
	"github.com/agentsafe/agentsafe/internal/fsutil"
	"github.com/agentsafe/agentsafe/internal/output"
)

type PrepareMetadata struct {
	Feature      string        `json:"feature"      yaml:"feature"`
	PreparedAt   string        `json:"preparedAt"   yaml:"preparedAt"`
	Repositories []PrepareRepo `json:"repositories" yaml:"repositories"`
}
type PrepareRepo struct {
	Name           string            `json:"name"                     yaml:"name"`
	Source         string            `json:"source"                   yaml:"source"`
	Agent          string            `json:"agent"                    yaml:"agent"`
	CopiedFiles    int               `json:"copiedFiles"              yaml:"copiedFiles"`
	IgnoredFiles   int               `json:"ignoredFiles"             yaml:"ignoredFiles"`
	MaskedFiles    []string          `json:"maskedFiles"              yaml:"maskedFiles"`
	PreparedHashes map[string]string `json:"preparedHashes,omitempty" yaml:"preparedHashes,omitempty"`
}

func LoadPrepareMetadata(root, featureName string) PrepareMetadata {
	b, err := os.ReadFile(config.SessionMetaPath(root, featureName))
	if err != nil {
		return PrepareMetadata{}
	}
	var m PrepareMetadata
	_ = json.Unmarshal(b, &m)
	return m
}

func maskedMap(pm PrepareMetadata, repoName string) map[string]bool {
	out := map[string]bool{}
	for _, r := range pm.Repositories {
		if r.Name == repoName {
			for _, f := range r.MaskedFiles {
				out[f] = true
			}
		}
	}
	return out
}

func preparedHashes(pm PrepareMetadata, repoName string) map[string]string {
	for _, r := range pm.Repositories {
		if r.Name == repoName {
			return r.PreparedHashes
		}
	}
	return nil
}

func Init(root string, cfg config.Config, featureName string) error {
	fm, err := feature.Load(root, featureName)
	if err != nil {
		return err
	}
	meta := PrepareMetadata{Feature: featureName, PreparedAt: time.Now().Format(time.RFC3339)}
	output.Printf("Agent workspace prepared: agent/%s\n\n", featureName)
	for _, r := range fm.Repositories {
		source := filepath.Join(root, r.WorktreePath)
		target := config.AgentPath(root, featureName, r.Name)
		backupExisting(target)
		pats := []string{".git/"}
		pats = append(pats, cfg.Agent.DefaultExclude...)
		pats = append(pats, LoadIgnoreFiles(filepath.Join(root, cfg.Agent.IgnoreFileName), filepath.Join(source, cfg.Agent.IgnoreFileName))...)
		matcher := NewIgnoreMatcher(pats)
		mask := LoadFirstMask(filepath.Join(source, cfg.Agent.MaskFileName), filepath.Join(root, cfg.Agent.MaskFileName))
		pr := PrepareRepo{Name: r.Name, Source: r.WorktreePath, Agent: filepath.ToSlash(filepath.Join("agent", featureName, r.Name)), PreparedHashes: map[string]string{}}
		err := filepath.WalkDir(source, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if path == source {
				return nil
			}
			rel, _ := filepath.Rel(source, path)
			rel = filepath.ToSlash(rel)
			if matcher.Match(rel, d.IsDir()) {
				pr.IgnoredFiles++
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			dst := filepath.Join(target, filepath.FromSlash(rel))
			if d.IsDir() {
				return os.MkdirAll(dst, 0755)
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			if info.Mode()&os.ModeSymlink != 0 {
				pr.IgnoredFiles++
				return nil
			}
			if fsutil.IsTextFile(path) {
				b, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				out, changed := mask.Apply(string(b))
				if changed {
					pr.MaskedFiles = append(pr.MaskedFiles, rel)
				}
				if err := fsutil.WriteFile(dst, []byte(out), info.Mode().Perm()); err != nil {
					return err
				}
			} else if err := fsutil.CopyFile(path, dst, info.Mode().Perm()); err != nil {
				return err
			}
			if h, err := fsutil.SHA256File(dst); err == nil {
				pr.PreparedHashes[rel] = h
			}
			pr.CopiedFiles++
			return nil
		})
		if err != nil {
			return err
		}
		meta.Repositories = append(meta.Repositories, pr)
		output.Printf("[%s]\ncopied: %d files\nignored: %d files\nmasked: %d files\n\n", pr.Name, pr.CopiedFiles, pr.IgnoredFiles, len(pr.MaskedFiles))
	}
	b, _ := json.MarshalIndent(meta, "", "  ")
	if err := os.MkdirAll(filepath.Dir(config.SessionMetaPath(root, featureName)), 0755); err != nil {
		return err
	}
	return os.WriteFile(config.SessionMetaPath(root, featureName), b, 0644)
}

func backupExisting(path string) {
	if st, err := os.Stat(path); err == nil && st.IsDir() {
		bak := fmt.Sprintf("%s.bak-%s", path, time.Now().Format("20060102150405"))
		_ = os.Rename(path, bak)
	}
	_ = os.MkdirAll(path, 0755)
}
