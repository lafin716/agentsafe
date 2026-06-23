package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/agentsafe/agentsafe/internal/config"
	"github.com/agentsafe/agentsafe/internal/fsutil"
)

// Preview file dispositions.
const (
	PreviewIgnored = "ignored" // excluded from the agent copy by an ignore rule
	PreviewMasked  = "masked"  // copied, but content rewritten by a mask rule
	PreviewCopied  = "copied"  // copied as-is
)

// MaskMatch describes a single masking rule that would alter a file, with the
// number of replacements it would make.
type MaskMatch struct {
	Name    string `json:"name"    yaml:"name"`
	Type    string `json:"type"    yaml:"type"`
	Pattern string `json:"pattern" yaml:"pattern"`
	Count   int    `json:"count"   yaml:"count"`
}

// PreviewEntry is the predicted disposition of one path under the current
// (saved) ignore/mask policy. It is produced without writing anything.
type PreviewEntry struct {
	Path          string      `json:"path"                    yaml:"path"`
	IsDir         bool        `json:"isDir"                   yaml:"isDir"`
	Status        string      `json:"status"                  yaml:"status"`
	IgnorePattern string      `json:"ignorePattern,omitempty" yaml:"ignorePattern,omitempty"`
	MaskMatches   []MaskMatch `json:"maskMatches,omitempty"   yaml:"maskMatches,omitempty"`
	Replacements  int         `json:"replacements,omitempty"  yaml:"replacements,omitempty"`
	Binary        bool        `json:"binary,omitempty"        yaml:"binary,omitempty"`
}

// PreviewResult is the full non-destructive scan of a repository's main clone.
type PreviewResult struct {
	Repo    string         `json:"repo"    yaml:"repo"`
	Source  string         `json:"source"  yaml:"source"`
	Ignored int            `json:"ignored" yaml:"ignored"`
	Masked  int            `json:"masked"  yaml:"masked"`
	Copied  int            `json:"copied"  yaml:"copied"`
	Total   int            `json:"total"   yaml:"total"`
	Entries []PreviewEntry `json:"entries" yaml:"entries"`
}

// previewPolicy resolves the main-clone source directory and builds the same
// ignore matcher and mask rules that prepareRepository would use, merged from
// the workspace root and the per-repo source. It does not modify anything.
func previewPolicy(root string, cfg config.Config, repoName string) (string, IgnoreMatcher, MaskFile, error) {
	source := config.RepoPath(root, repoName)
	if st, err := os.Stat(source); err != nil || !st.IsDir() {
		return "", IgnoreMatcher{}, MaskFile{}, fmt.Errorf("repository %q is not pulled (missing %s)", repoName, filepath.ToSlash(filepath.Join("main", repoName)))
	}
	_ = EnsureSecurityFile(cfg, root)
	secRoot := LoadSecurity(cfg, root)
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
	return source, matcher, mask, nil
}

// ScanPreview walks the repository's main clone and predicts, per path, whether
// the current saved policy would ignore, mask, or copy it — mirroring
// prepareRepository's walk but writing nothing. Ignored directories are recorded
// as a single entry and their contents are not enumerated, matching prepare's
// IgnoredFiles accounting.
func ScanPreview(root string, cfg config.Config, repoName string) (PreviewResult, error) {
	source, matcher, mask, err := previewPolicy(root, cfg, repoName)
	if err != nil {
		return PreviewResult{}, err
	}
	res := PreviewResult{Repo: repoName, Source: filepath.ToSlash(filepath.Join("main", repoName)), Entries: []PreviewEntry{}}
	err = filepath.WalkDir(source, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == source {
			return nil
		}
		rel, _ := filepath.Rel(source, path)
		rel = filepath.ToSlash(rel)
		if pat, ok := matcher.MatchWhich(rel, d.IsDir()); ok {
			res.Entries = append(res.Entries, PreviewEntry{Path: rel, IsDir: d.IsDir(), Status: PreviewIgnored, IgnorePattern: pat})
			res.Ignored++
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
		if info.Mode()&os.ModeSymlink != 0 {
			res.Entries = append(res.Entries, PreviewEntry{Path: rel, Status: PreviewIgnored, IgnorePattern: "(symlink)"})
			res.Ignored++
			return nil
		}
		entry := PreviewEntry{Path: rel, Status: PreviewCopied}
		if fsutil.IsTextFile(path) {
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			content := string(b)
			ext := strings.ToLower(filepath.Ext(path))
			out, changed := mask.Apply(content)
			if _, c2 := mask.ApplyKeyPaths(out, ext); c2 {
				changed = true
			}
			if changed {
				entry.Status = PreviewMasked
				entry.MaskMatches, entry.Replacements = countMaskMatches(mask, content, ext)
			}
		} else {
			entry.Binary = true
		}
		switch entry.Status {
		case PreviewMasked:
			res.Masked++
		default:
			res.Copied++
		}
		res.Entries = append(res.Entries, entry)
		return nil
	})
	if err != nil {
		return PreviewResult{}, err
	}
	res.Total = len(res.Entries)
	return res, nil
}

// PreviewFileDiff returns the original content of a single file and the content
// after the current saved mask policy is applied, for an on-demand before/after
// comparison. It reads only; nothing is written.
func PreviewFileDiff(root string, cfg config.Config, repoName, rel string) (before, after string, err error) {
	source, _, mask, err := previewPolicy(root, cfg, repoName)
	if err != nil {
		return "", "", err
	}
	full := filepath.Join(source, filepath.FromSlash(rel))
	if err := fsutil.EnsureInside(source, full); err != nil {
		return "", "", err
	}
	b, err := os.ReadFile(full)
	if err != nil {
		return "", "", err
	}
	before = string(b)
	out, _ := mask.Apply(before)
	if out2, c2 := mask.ApplyKeyPaths(out, strings.ToLower(filepath.Ext(full))); c2 {
		out = out2
	}
	return before, out, nil
}

// countMaskMatches reports, per rule, how many replacements it would make in
// content. Counts are computed against the original content independently per
// rule (a best-effort breakdown for display); the masked/copied status itself is
// decided by the real sequential Apply in ScanPreview.
func countMaskMatches(m MaskFile, content, ext string) ([]MaskMatch, int) {
	var matches []MaskMatch
	total := 0
	for _, r := range m.Rules {
		c := 0
		switch strings.ToLower(r.Type) {
		case "plain":
			repl := r.Replacement
			if repl == "" {
				repl = "__MASKED__"
			}
			if r.Pattern != "" && r.Pattern != repl {
				c = strings.Count(content, r.Pattern)
			}
		case "regex":
			if re, err := regexp.Compile(r.Pattern); err == nil {
				c = len(re.FindAllStringIndex(content, -1))
			}
		case "keypath", "key":
			if _, changed := (MaskFile{Rules: []MaskRule{r}}).ApplyKeyPaths(content, ext); changed {
				c = 1
			}
		}
		if c > 0 {
			matches = append(matches, MaskMatch{Name: r.Name, Type: r.Type, Pattern: r.Pattern, Count: c})
			total += c
		}
	}
	return matches, total
}
