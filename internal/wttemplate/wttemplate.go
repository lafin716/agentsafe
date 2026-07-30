package wttemplate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

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

type TemplateTree struct {
	Template Template         `json:"template"`
	Root     TemplateTreeNode `json:"root"`
}

type TemplateTreeNode struct {
	Name     string             `json:"name"`
	RelPath  string             `json:"relPath"`
	IsDir    bool               `json:"isDir"`
	Size     int64              `json:"size"`
	Files    int                `json:"files"`
	Folders  int                `json:"folders"`
	Children []TemplateTreeNode `json:"children"`
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

func ListTrees(root string) ([]TemplateTree, error) {
	templates, err := List(root)
	if err != nil {
		return nil, err
	}
	out := make([]TemplateTree, 0, len(templates))
	for _, t := range templates {
		dir := filepath.Join(FilesDir(root), t.ID)
		node, err := templateTreeNode(dir, dir)
		if err != nil {
			if os.IsNotExist(err) {
				node = TemplateTreeNode{Name: t.Name, IsDir: true}
			} else {
				return nil, err
			}
		}
		node.Name = t.Name
		out = append(out, TemplateTree{Template: t, Root: node})
	}
	return out, nil
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

// RegisterOptions describes the destination and flags applied to a template
// created by RegisterPath. An empty TargetMode asks for the destination to be
// inferred from where the source lives, in which case WorkspaceRepos must name
// the workspace's repositories so repository areas can be recognized.
type RegisterOptions struct {
	TargetMode     string
	RepoNames      []string
	Overwrite      bool
	Enabled        bool
	WorkspaceRepos []string
}

// RegisterPath registers an existing workspace file or folder as a worktree
// template, copying it into the template store. A source path that already
// backs a template is rejected, because the second template would keep writing
// the same source to a destination the first one already covers.
func RegisterPath(root, src string, opts RegisterOptions) (Template, error) {
	src = strings.TrimSpace(src)
	if src == "" {
		return Template{}, fmt.Errorf("path is required")
	}
	info, err := os.Stat(src)
	if err != nil {
		return Template{}, err
	}
	existing, found, err := FindBySourcePath(root, src)
	if err != nil {
		return Template{}, err
	}
	if found {
		return Template{}, fmt.Errorf("%s is already registered as template %q", src, existing.Name)
	}
	var t Template
	if info.IsDir() {
		t, err = ImportFolder(root, src)
	} else {
		var added []Template
		added, err = ImportFiles(root, []string{src})
		if err == nil {
			t = added[0]
		}
	}
	if err != nil {
		return Template{}, err
	}
	t.TargetMode, t.RepoNames = opts.TargetMode, dedupeStrings(opts.RepoNames)
	if t.TargetMode == "" {
		t.TargetMode, t.RepoNames = InferTarget(root, src, opts.WorkspaceRepos)
	}
	t.Overwrite = opts.Overwrite
	t.Enabled = opts.Enabled
	if err := Update(root, t); err != nil {
		return Template{}, err
	}
	return t, nil
}

// FindBySourcePath returns the template registered for src, if any. Paths are
// compared after normalization, so a trailing separator, a relative spelling,
// or different casing on Windows still resolves to the same template.
func FindBySourcePath(root, src string) (Template, bool, error) {
	templates, err := List(root)
	if err != nil {
		return Template{}, false, err
	}
	for _, t := range templates {
		if samePath(t.SourcePath, src) {
			return t, true, nil
		}
	}
	return Template{}, false, nil
}

// InferTarget guesses the destination of a source path from where it sits in
// the workspace: a repository area maps to that repository, a feature or agent
// root to that root, and everything else — including a path outside the
// workspace — to the workspace root. repoNames tells repository folders apart
// from other content, so it must list the workspace's repositories.
func InferTarget(root, src string, repoNames []string) (string, []string) {
	segments := workspaceSegments(root, src)
	if len(segments) < 2 {
		return TargetWorkspaceRoot, []string{}
	}
	rootMode, repoMode := TargetFeatureRoot, TargetSelectedRepos
	switch {
	case sameName(segments[0], "main"):
		if containsRepo(repoNames, segments[1]) {
			return TargetSelectedRepos, []string{segments[1]}
		}
		return TargetWorkspaceRoot, []string{}
	case sameName(segments[0], "agent"):
		rootMode, repoMode = TargetAgentRoot, TargetAgentSelectedRepos
	case !sameName(segments[0], "feature"):
		return TargetWorkspaceRoot, []string{}
	}
	// feature/<key> and agent/<key> hold one folder per repository next to
	// whatever the feature or agent root itself carries.
	if len(segments) == 2 {
		return rootMode, []string{}
	}
	if containsRepo(repoNames, segments[2]) {
		return repoMode, []string{segments[2]}
	}
	if len(segments) == 3 {
		return rootMode, []string{}
	}
	return TargetWorkspaceRoot, []string{}
}

// WorkspaceRepoNames lists the repositories configured for a workspace, ready to
// hand to InferTarget. A workspace whose config cannot be read has no
// recognizable repository folders, so inference falls back to the workspace root
// instead of failing the registration.
func WorkspaceRepoNames(root string) []string {
	cfg, err := config.Load(root)
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(cfg.Repositories))
	for _, r := range cfg.Repositories {
		names = append(names, r.Name)
	}
	return names
}

// workspaceSegments splits the path of src relative to the workspace root into
// its path elements, or returns nil when src is the root itself or lives
// outside it.
func workspaceSegments(root, src string) []string {
	rel, err := filepath.Rel(cleanPath(root), cleanPath(src))
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil
	}
	return strings.Split(rel, string(filepath.Separator))
}

func samePath(a, b string) bool { return sameName(cleanPath(a), cleanPath(b)) }

// cleanPath makes a path comparable: absolute where the working directory
// allows it, without redundant or trailing separators.
func cleanPath(p string) string {
	p = strings.TrimSpace(p)
	abs, err := filepath.Abs(p)
	if err != nil {
		return filepath.Clean(p)
	}
	return abs
}

// sameName compares two path elements the way the local filesystem does, so a
// differently cased spelling of the same name still matches on Windows.
func sameName(a, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
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

func ReadTemplateTreeFile(root, id, relPath string) (string, error) {
	path, err := templateTreeFilePath(root, id, relPath)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("cannot edit a directory")
	}
	if info.Size() > maxEditableTemplateFileSize {
		return "", fmt.Errorf("file is too large to edit in the app (max 2 MB)")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if !utf8.Valid(b) || hasNUL(b) {
		return "", fmt.Errorf("file is not valid UTF-8 text")
	}
	return string(b), nil
}

func WriteTemplateTreeFile(root, id, relPath, content string) error {
	path, err := templateTreeFilePath(root, id, relPath)
	if err != nil {
		return err
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("cannot edit a directory")
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

// SourceEntry is one file or directory within a template's stored source tree,
// with its path relative to the template root (slash-separated).
type SourceEntry struct {
	RelPath string
	IsDir   bool
}

// SourceEntries lists every file and directory under a template's source tree
// (FilesDir/<id>), each as a path relative to that root. Used to map applied
// template files in a workspace back to their template origin.
func SourceEntries(root, id string) ([]SourceEntry, error) {
	base := filepath.Join(FilesDir(root), id)
	out := []SourceEntry{}
	err := filepath.WalkDir(base, func(p string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, relErr := filepath.Rel(base, p)
		if relErr != nil || rel == "." {
			return relErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		out = append(out, SourceEntry{RelPath: filepath.ToSlash(rel), IsDir: entry.IsDir()})
		return nil
	})
	if err != nil {
		if os.IsNotExist(err) {
			return []SourceEntry{}, nil
		}
		return nil, err
	}
	return out, nil
}

// Destinations returns the directories a template is applied into for the given
// feature. Exported wrapper around the internal destination logic; repos must
// carry worktree paths for worktree-target modes and agent paths for
// agent-target modes (see the desktop repo builders).
func Destinations(root, featureKey string, repos []Repo, t Template) []string {
	return destinations(root, featureKey, repos, t)
}

// IsAgentTarget reports whether a template target mode applies to agent areas
// (agent root / agent repos) rather than worktree areas.
func IsAgentTarget(mode string) bool {
	return isAgentTarget(mode)
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

const maxEditableTemplateFileSize int64 = 2 * 1024 * 1024

func templateTreeFilePath(root, id, relPath string) (string, error) {
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
	clean := filepath.Clean(filepath.FromSlash(strings.TrimSpace(relPath)))
	if clean == "." || clean == "" || filepath.IsAbs(clean) || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || clean == ".." {
		return "", fmt.Errorf("invalid template file path %q", relPath)
	}
	base := filepath.Join(FilesDir(root), id)
	target := filepath.Join(base, clean)
	absBase, err := filepath.Abs(base)
	if err != nil {
		return "", err
	}
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return "", err
	}
	if err := fsutil.EnsureInside(absBase, absTarget); err != nil {
		return "", err
	}
	return absTarget, nil
}

func templateTreeNode(base, target string) (TemplateTreeNode, error) {
	info, err := os.Stat(target)
	if err != nil {
		return TemplateTreeNode{}, err
	}
	rel, _ := filepath.Rel(base, target)
	if rel == "." {
		rel = ""
	}
	node := TemplateTreeNode{
		Name:    filepath.Base(target),
		RelPath: filepath.ToSlash(rel),
		IsDir:   info.IsDir(),
		Size:    info.Size(),
	}
	if !info.IsDir() {
		node.Files = 1
		return node, nil
	}
	entries, err := os.ReadDir(target)
	if err != nil {
		return node, err
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir() != entries[j].IsDir() {
			return entries[i].IsDir()
		}
		return strings.ToLower(entries[i].Name()) < strings.ToLower(entries[j].Name())
	})
	node.Children = []TemplateTreeNode{}
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		child, err := templateTreeNode(base, filepath.Join(target, entry.Name()))
		if err != nil {
			continue
		}
		if child.IsDir {
			node.Folders++
		}
		node.Files += child.Files
		node.Folders += child.Folders
		node.Children = append(node.Children, child)
	}
	return node, nil
}

func hasNUL(b []byte) bool {
	for _, c := range b {
		if c == 0 {
			return true
		}
	}
	return false
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

// containsRepo reports whether repoName is one of names, comparing the way the
// filesystem does so a repository folder's on-disk spelling matches its
// configured name.
func containsRepo(names []string, repoName string) bool {
	for _, name := range names {
		if sameName(name, repoName) {
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
