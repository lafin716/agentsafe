package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/agentsafe/agentsafe/internal/config"
	"github.com/agentsafe/agentsafe/internal/feature"
	"github.com/agentsafe/agentsafe/internal/fsutil"
	"github.com/agentsafe/agentsafe/internal/output"
)

type PrepareMetadata struct {
	Feature         string        `json:"feature"      yaml:"feature"`
	PreparedAt      string        `json:"preparedAt"   yaml:"preparedAt"`
	FeatureRevision int           `json:"featureRevision,omitempty" yaml:"featureRevision,omitempty"`
	Repositories    []PrepareRepo `json:"repositories" yaml:"repositories"`
}
type PrepareRepo struct {
	Name             string                    `json:"name"                     yaml:"name"`
	Source           string                    `json:"source"                   yaml:"source"`
	Agent            string                    `json:"agent"                    yaml:"agent"`
	CopiedFiles      int                       `json:"copiedFiles"              yaml:"copiedFiles"`
	IgnoredFiles     int                       `json:"ignoredFiles"             yaml:"ignoredFiles"`
	MaskedFiles      []string                  `json:"maskedFiles"              yaml:"maskedFiles"`
	PreparedHashes   map[string]string         `json:"preparedHashes,omitempty" yaml:"preparedHashes,omitempty"`
	FileIndex        map[string]FileIndexEntry `json:"fileIndex,omitempty" yaml:"fileIndex,omitempty"`
	WorktreeRevision int                       `json:"worktreeRevision,omitempty" yaml:"worktreeRevision,omitempty"`
}

// FileSnapshot is the stat/hash information captured at prepare time. Diff
// uses the cheap stat fields as an index and hashes only files whose metadata
// may have changed.
type FileSnapshot struct {
	Size        int64  `json:"size"        yaml:"size"`
	ModTimeNano int64  `json:"modTimeNano" yaml:"modTimeNano"`
	Hash        string `json:"hash"        yaml:"hash"`
}

type FileIndexEntry struct {
	Agent    FileSnapshot `json:"agent"    yaml:"agent"`
	Worktree FileSnapshot `json:"worktree" yaml:"worktree"`
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

func preparedFileIndex(pm PrepareMetadata, repoName string) map[string]FileIndexEntry {
	for _, r := range pm.Repositories {
		if r.Name == repoName {
			return r.FileIndex
		}
	}
	return nil
}

// PrepareOptions controls how Init treats an existing agent workspace when
// re-preparing. Backup renames the existing copy to a timestamped ".bak-"
// directory; otherwise it is deleted.
type PrepareOptions struct {
	Backup bool
}

func Init(root string, cfg config.Config, featureName string, opt PrepareOptions) error {
	fm, err := feature.Load(root, featureName)
	if err != nil {
		return err
	}
	meta := PrepareMetadata{Feature: featureName, PreparedAt: time.Now().Format(time.RFC3339), FeatureRevision: fm.Revision}
	// Migrate any legacy .agentignore/mask.json at the workspace root into the
	// unified agentsafe.yaml before loading security config.
	_ = EnsureSecurityFile(cfg, root)
	secRoot := LoadSecurity(cfg, root)
	output.Printf("Agent workspace prepared: agent/%s\n\n", featureName)
	for _, r := range fm.Repositories {
		pr, err := prepareRepository(root, cfg, fm.FolderKey(), r, opt, secRoot)
		if err != nil {
			return err
		}
		meta.Repositories = append(meta.Repositories, pr)
		output.Printf("[%s]\ncopied: %d files\nignored: %d files\nmasked: %d files\n\n", pr.Name, pr.CopiedFiles, pr.IgnoredFiles, len(pr.MaskedFiles))
	}
	return savePrepareMetadata(root, featureName, meta)
}

// PrepareRepository creates or replaces one repository's sanitized agent
// folder while preserving all other repository folders and metadata.
func PrepareRepository(root string, cfg config.Config, featureName, repoName string, opt PrepareOptions) (PrepareMetadata, error) {
	fm, err := feature.Load(root, featureName)
	if err != nil {
		return PrepareMetadata{}, err
	}
	var repoMeta *feature.RepoMeta
	for i := range fm.Repositories {
		if fm.Repositories[i].Name == repoName {
			repoMeta = &fm.Repositories[i]
			break
		}
	}
	if repoMeta == nil {
		return PrepareMetadata{}, fmt.Errorf("repository %q is not part of feature %q", repoName, featureName)
	}
	if st, err := os.Stat(filepath.Join(root, repoMeta.WorktreePath)); err != nil || !st.IsDir() {
		return PrepareMetadata{}, fmt.Errorf("worktree for repository %q does not exist", repoName)
	}
	_ = EnsureSecurityFile(cfg, root)
	secRoot := LoadSecurity(cfg, root)
	pr, err := prepareRepository(root, cfg, fm.FolderKey(), *repoMeta, opt, secRoot)
	if err != nil {
		return PrepareMetadata{}, err
	}

	meta := LoadPrepareMetadata(root, featureName)
	if meta.Feature == "" {
		meta.Feature = featureName
	}
	meta.PreparedAt = time.Now().Format(time.RFC3339)
	replaced := false
	for i := range meta.Repositories {
		if meta.Repositories[i].Name == repoName {
			meta.Repositories[i] = pr
			replaced = true
			break
		}
	}
	if !replaced {
		meta.Repositories = append(meta.Repositories, pr)
	}
	if allRepositoriesCurrent(fm, meta) {
		meta.FeatureRevision = fm.Revision
	}
	if err := savePrepareMetadata(root, featureName, meta); err != nil {
		return PrepareMetadata{}, err
	}
	return meta, nil
}

func prepareRepository(root string, cfg config.Config, folderKey string, r feature.RepoMeta, opt PrepareOptions, secRoot SecurityFile) (PrepareRepo, error) {
	source := filepath.Join(root, r.WorktreePath)
	target := config.AgentPath(root, folderKey, r.Name)
	resetTarget(target, opt.Backup)
	secSource := LoadSecurity(cfg, source)
	pats := []string{".git/"}
	pats = append(pats, cfg.Agent.DefaultExclude...)
	pats = append(pats, secRoot.Ignore...)
	pats = append(pats, secSource.Ignore...)
	matcher := NewIgnoreMatcher(pats)
	mask := MaskFile{Rules: secSource.Mask}
	if len(mask.Rules) == 0 {
		mask = MaskFile{Rules: secRoot.Mask}
	}
	pr := PrepareRepo{
		Name: r.Name, Source: r.WorktreePath,
		Agent:          filepath.ToSlash(filepath.Join("agent", folderKey, r.Name)),
		PreparedHashes: map[string]string{}, FileIndex: map[string]FileIndexEntry{},
		WorktreeRevision: r.Revision,
	}
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
			if out2, c2 := mask.ApplyKeyPaths(out, strings.ToLower(filepath.Ext(path))); c2 {
				out, changed = out2, true
			}
			if changed {
				pr.MaskedFiles = append(pr.MaskedFiles, rel)
			}
			if err := fsutil.WriteFile(dst, []byte(out), info.Mode().Perm()); err != nil {
				return err
			}
		} else if err := fsutil.CopyFile(path, dst, info.Mode().Perm()); err != nil {
			return err
		}
		worktreeHash, err := fsutil.SHA256File(path)
		if err != nil {
			return err
		}
		agentHash, err := fsutil.SHA256File(dst)
		if err != nil {
			return err
		}
		dstInfo, err := os.Stat(dst)
		if err != nil {
			return err
		}
		pr.PreparedHashes[rel] = agentHash
		pr.FileIndex[rel] = FileIndexEntry{
			Agent:    FileSnapshot{Size: dstInfo.Size(), ModTimeNano: dstInfo.ModTime().UnixNano(), Hash: agentHash},
			Worktree: FileSnapshot{Size: info.Size(), ModTimeNano: info.ModTime().UnixNano(), Hash: worktreeHash},
		}
		pr.CopiedFiles++
		return nil
	})
	if err != nil {
		return PrepareRepo{}, err
	}
	output.Printf("[%s]\ncopied: %d files\nignored: %d files\nmasked: %d files\n\n", pr.Name, pr.CopiedFiles, pr.IgnoredFiles, len(pr.MaskedFiles))
	return pr, nil
}

func allRepositoriesCurrent(fm feature.Metadata, pm PrepareMetadata) bool {
	prepared := map[string]int{}
	for _, r := range pm.Repositories {
		prepared[r.Name] = r.WorktreeRevision
	}
	for _, r := range fm.Repositories {
		revision, ok := prepared[r.Name]
		if !ok || (r.Revision > 0 && revision != r.Revision) {
			return false
		}
	}
	return true
}

func savePrepareMetadata(root, featureName string, meta PrepareMetadata) error {
	b, _ := json.MarshalIndent(meta, "", "  ")
	if err := os.MkdirAll(filepath.Dir(config.SessionMetaPath(root, featureName)), 0755); err != nil {
		return err
	}
	return os.WriteFile(config.SessionMetaPath(root, featureName), b, 0644)
}

// resetTarget clears an existing agent workspace directory before a fresh
// prepare. When backup is true the existing directory is renamed to a
// timestamped ".bak-" sibling; otherwise it is removed.
func resetTarget(path string, backup bool) {
	if st, err := os.Stat(path); err == nil && st.IsDir() {
		if backup {
			bak := fmt.Sprintf("%s.bak-%s", path, time.Now().Format("20060102150405"))
			_ = os.Rename(path, bak)
		} else {
			_ = os.RemoveAll(path)
		}
	}
	_ = os.MkdirAll(path, 0755)
}
