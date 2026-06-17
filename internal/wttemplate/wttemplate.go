package wttemplate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/agentsafe/agentsafe/internal/config"
	"github.com/agentsafe/agentsafe/internal/fsutil"
	aggit "github.com/agentsafe/agentsafe/internal/git"
	"github.com/agentsafe/agentsafe/internal/output"
)

const (
	TargetWorkspaceRoot      = "workspaceRoot"
	TargetFeatureRoot        = "featureRoot"
	TargetAllRepos           = "allRepos"
	TargetSelectedRepos      = "selectedRepos"
	TargetAgentRoot          = "agentRoot"
	TargetAgentAllRepos      = "agentAllRepos"
	TargetAgentSelectedRepos = "agentSelectedRepos"
)

type Template struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	SourcePath string   `json:"sourcePath"`
	Enabled    bool     `json:"enabled"`
	TargetMode string   `json:"targetMode"`
	RepoNames  []string `json:"repoNames"`
	Overwrite  bool     `json:"overwrite"`
}

type Store struct {
	Templates []Template `json:"templates"`
}

type Repo struct {
	Name         string
	WorktreePath string
}

func BaseDir(root string) string   { return filepath.Join(root, config.DirName, "worktree-templates") }
func FilesDir(root string) string  { return filepath.Join(BaseDir(root), "files") }
func StorePath(root string) string { return filepath.Join(BaseDir(root), "templates.json") }

func List(root string) ([]Template, error) {
	store, err := load(root)
	if err != nil {
		return nil, err
	}
	if store.Templates == nil {
		return []Template{}, nil
	}
	return store.Templates, nil
}

func ImportFiles(root string, paths []string) ([]Template, error) {
	store, err := load(root)
	if err != nil {
		return nil, err
	}
	added := []Template{}
	for _, src := range paths {
		if strings.TrimSpace(src) == "" {
			continue
		}
		info, err := os.Stat(src)
		if err != nil {
			return nil, err
		}
		if info.IsDir() {
			return nil, fmt.Errorf("%s is a directory; use folder import", src)
		}
		t := defaultTemplate(src)
		dst := filepath.Join(FilesDir(root), t.ID, filepath.Base(src))
		if err := fsutil.CopyFile(src, dst, info.Mode().Perm()); err != nil {
			return nil, err
		}
		store.Templates = append(store.Templates, t)
		added = append(added, t)
	}
	if err := save(root, store); err != nil {
		return nil, err
	}
	return added, nil
}

func ImportPaths(root string, paths []string) ([]Template, error) {
	added := []Template{}
	for _, src := range paths {
		if strings.TrimSpace(src) == "" {
			continue
		}
		info, err := os.Stat(src)
		if err != nil {
			return nil, err
		}
		if info.IsDir() {
			item, err := ImportFolder(root, src)
			if err != nil {
				return nil, err
			}
			added = append(added, item)
			continue
		}
		items, err := ImportFiles(root, []string{src})
		if err != nil {
			return nil, err
		}
		added = append(added, items...)
	}
	return added, nil
}

func ImportFolder(root, src string) (Template, error) {
	info, err := os.Stat(src)
	if err != nil {
		return Template{}, err
	}
	if !info.IsDir() {
		return Template{}, fmt.Errorf("%s is not a directory", src)
	}
	store, err := load(root)
	if err != nil {
		return Template{}, err
	}
	t := defaultTemplate(src)
	dst := filepath.Join(FilesDir(root), t.ID, filepath.Base(src))
	if err := copyTree(src, dst, true); err != nil {
		return Template{}, err
	}
	store.Templates = append(store.Templates, t)
	if err := save(root, store); err != nil {
		return Template{}, err
	}
	return t, nil
}

func Update(root string, next Template) error {
	if err := validateTemplate(next); err != nil {
		return err
	}
	store, err := load(root)
	if err != nil {
		return err
	}
	for i := range store.Templates {
		if store.Templates[i].ID == next.ID {
			store.Templates[i] = next
			return save(root, store)
		}
	}
	return fmt.Errorf("template %q not found", next.ID)
}

func Delete(root, id string) error {
	store, err := load(root)
	if err != nil {
		return err
	}
	out := store.Templates[:0]
	found := false
	for _, t := range store.Templates {
		if t.ID == id {
			found = true
			continue
		}
		out = append(out, t)
	}
	if !found {
		return fmt.Errorf("template %q not found", id)
	}
	store.Templates = out
	if err := save(root, store); err != nil {
		return err
	}
	return fsutil.ForceRemoveAll(filepath.Join(FilesDir(root), id))
}

func Clear(root string) error {
	return fsutil.ForceRemoveAll(BaseDir(root))
}

func ReadTemplateFile(root, id string) (string, error) {
	path, err := singleTemplateFilePath(root, id)
	if err != nil {
		return "", err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func WriteTemplateFile(root, id, content string) error {
	path, err := singleTemplateFilePath(root, id)
	if err != nil {
		return err
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), info.Mode().Perm())
}

func Apply(root, featureKey string, repos []Repo) error {
	return apply(root, featureKey, repos, isWorktreeTarget)
}

func ApplyWorkspaceRoot(root string) error {
	templates, err := List(root)
	if err != nil {
		return err
	}
	for _, t := range templates {
		if !t.Enabled || t.TargetMode != TargetWorkspaceRoot {
			continue
		}
		src := filepath.Join(FilesDir(root), t.ID)
		if _, err := os.Stat(src); err != nil {
			return fmt.Errorf("template %s source missing: %w", t.Name, err)
		}
		output.Printf("[template] applying %s to %s\n", t.Name, root)
		if err := copyTree(src, root, t.Overwrite); err != nil {
			return fmt.Errorf("apply template %s: %w", t.Name, err)
		}
	}
	return nil
}

func ApplyAgent(root, featureKey string, repos []Repo) error {
	return apply(root, featureKey, repos, isAgentTarget)
}

func apply(root, featureKey string, repos []Repo, include func(string) bool) error {
	templates, err := List(root)
	if err != nil {
		return err
	}
	if len(templates) == 0 {
		return nil
	}
	for _, t := range templates {
		if !t.Enabled || !include(t.TargetMode) {
			continue
		}
		src := filepath.Join(FilesDir(root), t.ID)
		if _, err := os.Stat(src); err != nil {
			return fmt.Errorf("template %s source missing: %w", t.Name, err)
		}
		for _, dst := range destinations(root, featureKey, repos, t) {
			output.Printf("[template] applying %s to %s\n", t.Name, dst)
			if err := copyTree(src, dst, t.Overwrite); err != nil {
				return fmt.Errorf("apply template %s: %w", t.Name, err)
			}
			if isRepoWorktreeTarget(t.TargetMode) {
				if err := addTemplateExcludes(dst, templateIgnorePatterns(root, t)); err != nil {
					return fmt.Errorf("ignore template %s: %w", t.Name, err)
				}
			}
		}
	}
	return nil
}

func ApplyToRepos(root string, repos []Repo) error {
	return applyToRepos(root, repos, TargetAllRepos, TargetSelectedRepos)
}

func ApplyToAgentRepos(root string, repos []Repo) error {
	return applyToRepos(root, repos, TargetAgentAllRepos, TargetAgentSelectedRepos)
}

func applyToRepos(root string, repos []Repo, allMode, selectedMode string) error {
	templates, err := List(root)
	if err != nil {
		return err
	}
	for _, t := range templates {
		if !t.Enabled || (t.TargetMode != allMode && t.TargetMode != selectedMode) {
			continue
		}
		src := filepath.Join(FilesDir(root), t.ID)
		for _, dst := range repoDestinations(repos, t, allMode) {
			output.Printf("[template] applying %s to %s\n", t.Name, dst)
			if err := copyTree(src, dst, t.Overwrite); err != nil {
				return fmt.Errorf("apply template %s: %w", t.Name, err)
			}
			if allMode == TargetAllRepos {
				if err := addTemplateExcludes(dst, templateIgnorePatterns(root, t)); err != nil {
					return fmt.Errorf("ignore template %s: %w", t.Name, err)
				}
			}
		}
	}
	return nil
}

func destinations(root, featureKey string, repos []Repo, t Template) []string {
	switch t.TargetMode {
	case TargetFeatureRoot:
		return []string{filepath.Join(root, "feature", featureKey)}
	case TargetAgentRoot:
		return []string{filepath.Join(root, "agent", featureKey)}
	case TargetAllRepos, TargetSelectedRepos:
		return repoDestinations(repos, t, TargetAllRepos)
	case TargetAgentAllRepos, TargetAgentSelectedRepos:
		return repoDestinations(repos, t, TargetAgentAllRepos)
	case TargetWorkspaceRoot:
		return []string{root}
	default:
		return nil
	}
}

func repoDestinations(repos []Repo, t Template, allMode string) []string {
	selected := map[string]bool{}
	for _, name := range t.RepoNames {
		selected[name] = true
	}
	out := []string{}
	for _, r := range repos {
		if t.TargetMode == allMode || selected[r.Name] {
			out = append(out, r.WorktreePath)
		}
	}
	return out
}

func defaultTemplate(src string) Template {
	base := filepath.Base(src)
	return Template{
		ID:         fmt.Sprintf("%d-%s", time.Now().UnixNano(), sanitizeID(base)),
		Name:       base,
		SourcePath: src,
		Enabled:    true,
		TargetMode: TargetAllRepos,
		RepoNames:  []string{},
		Overwrite:  false,
	}
}

func load(root string) (Store, error) {
	b, err := os.ReadFile(StorePath(root))
	if os.IsNotExist(err) {
		return Store{Templates: []Template{}}, nil
	}
	if err != nil {
		return Store{}, err
	}
	var s Store
	if err := json.Unmarshal(b, &s); err != nil {
		return Store{}, err
	}
	for i := range s.Templates {
		normalize(&s.Templates[i])
	}
	return s, nil
}

func save(root string, s Store) error {
	if err := os.MkdirAll(BaseDir(root), 0o755); err != nil {
		return err
	}
	for i := range s.Templates {
		normalize(&s.Templates[i])
		if err := validateTemplate(s.Templates[i]); err != nil {
			return err
		}
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(StorePath(root), b, 0o644)
}

func normalize(t *Template) {
	if t.TargetMode == "" {
		t.TargetMode = TargetAllRepos
	}
	if t.RepoNames == nil {
		t.RepoNames = []string{}
	}
}

func validateTemplate(t Template) error {
	if t.ID == "" {
		return fmt.Errorf("template id is required")
	}
	if t.Name == "" {
		return fmt.Errorf("template name is required")
	}
	switch t.TargetMode {
	case TargetWorkspaceRoot, TargetFeatureRoot, TargetAllRepos, TargetSelectedRepos, TargetAgentRoot, TargetAgentAllRepos, TargetAgentSelectedRepos:
		return nil
	default:
		return fmt.Errorf("invalid template target mode %q", t.TargetMode)
	}
}

func singleTemplateFilePath(root, id string) (string, error) {
	if strings.TrimSpace(id) == "" {
		return "", fmt.Errorf("template id is required")
	}
	templates, err := List(root)
	if err != nil {
		return "", err
	}
	found := false
	for _, t := range templates {
		if t.ID == id {
			found = true
			break
		}
	}
	if !found {
		return "", fmt.Errorf("template %q not found", id)
	}
	dir := filepath.Join(FilesDir(root), id)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	if len(entries) != 1 {
		return "", fmt.Errorf("template %q is not a single file template", id)
	}
	entry := entries[0]
	if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("template %q is a folder template; open the template folder to edit it", id)
	}
	return filepath.Join(dir, entry.Name()), nil
}

func isWorktreeTarget(mode string) bool {
	return mode == TargetFeatureRoot || mode == TargetAllRepos || mode == TargetSelectedRepos
}

func isAgentTarget(mode string) bool {
	return mode == TargetAgentRoot || mode == TargetAgentAllRepos || mode == TargetAgentSelectedRepos
}

func isRepoWorktreeTarget(mode string) bool {
	return mode == TargetAllRepos || mode == TargetSelectedRepos
}

func AgentIgnorePatterns(root, repoName string) ([]string, error) {
	templates, err := List(root)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, t := range templates {
		if !t.Enabled {
			continue
		}
		switch t.TargetMode {
		case TargetAgentAllRepos:
			out = append(out, templateIgnorePatterns(root, t)...)
		case TargetAgentSelectedRepos:
			if containsRepo(t.RepoNames, repoName) {
				out = append(out, templateIgnorePatterns(root, t)...)
			}
		}
	}
	return dedupeStrings(out), nil
}

func templateIgnorePatterns(root string, t Template) []string {
	dir := filepath.Join(FilesDir(root), t.ID)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := filepath.ToSlash(entry.Name())
		if entry.IsDir() {
			name += "/"
		}
		out = append(out, "/"+name)
	}
	return out
}

func addTemplateExcludes(worktreePath string, patterns []string) error {
	if len(patterns) == 0 {
		return nil
	}
	if _, err := os.Stat(filepath.Join(worktreePath, ".git")); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	excludePath, err := aggit.Output(worktreePath, "rev-parse", "--git-path", "info/exclude")
	if err != nil {
		return err
	}
	if !filepath.IsAbs(excludePath) {
		excludePath = filepath.Join(worktreePath, filepath.FromSlash(excludePath))
	}
	if err := os.MkdirAll(filepath.Dir(excludePath), 0o755); err != nil {
		return err
	}
	existingBytes, err := os.ReadFile(excludePath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	existing := string(existingBytes)
	var b strings.Builder
	if existing != "" {
		b.WriteString(existing)
		if !strings.HasSuffix(existing, "\n") {
			b.WriteByte('\n')
		}
	}
	wroteHeader := false
	for _, p := range dedupeStrings(patterns) {
		if strings.Contains("\n"+existing+"\n", "\n"+p+"\n") {
			continue
		}
		if !wroteHeader && !strings.Contains(existing, "# agentsafe worktree templates") {
			b.WriteString("# agentsafe worktree templates\n")
			wroteHeader = true
		}
		b.WriteString(p)
		b.WriteByte('\n')
	}
	if b.String() == existing {
		return nil
	}
	return os.WriteFile(excludePath, []byte(b.String()), 0o644)
}

func containsRepo(names []string, repoName string) bool {
	for _, name := range names {
		if name == repoName {
			return true
		}
	}
	return false
}

func dedupeStrings(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func copyTree(srcRoot, dstRoot string, overwrite bool) error {
	return filepath.WalkDir(srcRoot, func(src string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(srcRoot, src)
		if err != nil || rel == "." {
			return err
		}
		dst := filepath.Join(dstRoot, rel)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return os.MkdirAll(dst, info.Mode().Perm())
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if _, err := os.Stat(dst); err == nil && !overwrite {
			return nil
		} else if err != nil && !os.IsNotExist(err) {
			return err
		}
		return fsutil.CopyFile(src, dst, info.Mode().Perm())
	})
}

func sanitizeID(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-.")
	if out == "" {
		return "template"
	}
	return out
}
