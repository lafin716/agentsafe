package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"gopkg.in/yaml.v3"

	"github.com/agentsafe/agentsafe/internal/agent"
	"github.com/agentsafe/agentsafe/internal/config"
	"github.com/agentsafe/agentsafe/internal/feature"
	"github.com/agentsafe/agentsafe/internal/forge"
	"github.com/agentsafe/agentsafe/internal/fsutil"
	aggit "github.com/agentsafe/agentsafe/internal/git"
	"github.com/agentsafe/agentsafe/internal/output"
	"github.com/agentsafe/agentsafe/internal/registry"
	"github.com/agentsafe/agentsafe/internal/repo"
	"github.com/agentsafe/agentsafe/internal/workspacebundle"
	"github.com/agentsafe/agentsafe/internal/wttemplate"
	"github.com/agentsafe/agentsafe/packages/core"
)

// App exposes agentsafe's core packages to the Wails frontend.
// It is a thin binding layer: every method delegates to the same internal/
// functions the CLI uses, so GUI and CLI share identical behavior.
type App struct {
	ctx  context.Context
	root string // currently opened workspace root ("" when none)

	taskMu       sync.Mutex // serializes tracked tasks so their output never interleaves
	taskSeq      int        // monotonic id for progress tasks
	credentialMu sync.Mutex
	credentials  map[string]gitCredential // HTTPS credentials, memory-only for this app process
	terminalMu   sync.Mutex
	terminalSeq  int
	terminals    map[string]*terminalProcess
}

type gitCredential struct {
	Username string
	Secret   string
}

type terminalProcess struct {
	pty  ptySession
	path string
	// feature is set when this session runs a managed agent for a feature, so its
	// exit can be reported via the "agent:exit" event. Empty for plain shells.
	feature string
}

func NewApp() *App {
	return &App{credentials: map[string]gitCredential{}, terminals: map[string]*terminalProcess{}}
}

// runTask runs a long-running operation while streaming its output to the
// frontend progress box. It emits "task:start" before, forwards each output
// chunk as "task:log", and emits "task:end" (status done|error) after. Tracked
// tasks are serialized so their log streams never interleave; Wails dispatches
// each binding call on its own goroutine, so this does not freeze the UI.
func (a *App) runTask(label string, fn func() error) error {
	a.taskMu.Lock()
	defer a.taskMu.Unlock()
	a.taskSeq++
	id := a.taskSeq
	runtime.EventsEmit(a.ctx, "task:start", map[string]any{
		"id":        id,
		"label":     label,
		"startedAt": time.Now().UnixMilli(),
	})
	output.SetSink(func(chunk string) {
		runtime.EventsEmit(a.ctx, "task:log", map[string]any{"id": id, "chunk": chunk})
	})
	err := fn()
	output.SetSink(nil)
	status, msg := "done", ""
	if err != nil {
		status, msg = "error", err.Error()
	}
	runtime.EventsEmit(a.ctx, "task:end", map[string]any{"id": id, "status": status, "error": msg})
	return err
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	runtime.OnFileDrop(ctx, func(x, y int, paths []string) {
		runtime.EventsEmit(ctx, "workspace:file-drop", map[string]any{
			"x":     x,
			"y":     y,
			"paths": paths,
		})
	})
	// Restore the last active workspace from the registry (if it still loads).
	if r, err := registry.Load(); err == nil && r.Active != "" {
		if root, _, err := config.LoadFrom(r.Active); err == nil {
			a.root = root
		}
	}
}

// requireRoot returns the opened workspace root or an error when none is open.
func (a *App) requireRoot() (string, error) {
	if a.root == "" {
		return "", fmt.Errorf("no workspace is open; open or initialize one first")
	}
	return a.root, nil
}

// ---- Workspace ----

// SelectWorkspaceDir opens a native directory picker and returns the chosen path.
func (a *App) SelectWorkspaceDir() (string, error) {
	return runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Select agentsafe workspace",
	})
}

// SelectProgram opens a native file picker so the user can choose the program
// used to open agent workspaces (e.g. an editor). Returns the chosen path
// ("" when cancelled).
func (a *App) SelectProgram() (string, error) {
	opts := runtime.OpenDialogOptions{Title: "Select a program"}
	if goruntime.GOOS == "darwin" {
		opts.DefaultDirectory = "/Applications"
		opts.Filters = []runtime.FileFilter{
			{DisplayName: "Applications", Pattern: "*.app"},
		}
	}
	return runtime.OpenFileDialog(a.ctx, opts)
}

// OpenWorkspace discovers the agentsafe root from the given path and loads config.
func (a *App) OpenWorkspace(path string) (config.Config, error) {
	root, cfg, err := config.LoadFrom(path)
	if err != nil {
		return config.Config{}, err
	}
	a.root = root
	_ = registry.Add(cfg.Workspace.Name, root)
	return cfg, nil
}

// InitWorkspace initializes a new workspace at path and opens it.
func (a *App) InitWorkspace(path, name string) (config.Config, error) {
	if path == "" {
		return config.Config{}, fmt.Errorf("path is required")
	}
	cfg, err := config.InitWorkspace(path, name)
	if err != nil {
		return config.Config{}, err
	}
	a.root = cfg.Workspace.Root
	_ = registry.Add(cfg.Workspace.Name, cfg.Workspace.Root)
	return cfg, nil
}

// ListWorkspaces returns the workspaces registered with the app.
func (a *App) ListWorkspaces() ([]registry.Entry, error) {
	entries, err := registry.List()
	if err != nil {
		return nil, err
	}
	if entries == nil {
		return []registry.Entry{}, nil
	}
	return entries, nil
}

// RemoveWorkspace unregisters a workspace from the app. It does not touch the
// workspace directory on disk. When the removed workspace is the open one, the
// open workspace is cleared.
func (a *App) RemoveWorkspace(path string) error {
	if err := registry.Remove(path); err != nil {
		return err
	}
	if a.root == path {
		a.root = ""
	}
	return nil
}

// CurrentRoot reports the currently opened workspace root (empty if none).
func (a *App) CurrentRoot() string { return a.root }

// RunCore exposes the shared Go core service to the Wails frontend.
func (a *App) RunCore(text string) (*core.RunResult, error) {
	return core.NewService().Run(context.Background(), core.RunInput{Text: text})
}

// GetConfig returns the config for the open workspace.
func (a *App) GetConfig() (config.Config, error) {
	root, err := a.requireRoot()
	if err != nil {
		return config.Config{}, err
	}
	return config.Load(root)
}

func (a *App) ExportWorkspaceBundle() (string, error) {
	root, err := a.requireRoot()
	if err != nil {
		return "", err
	}
	cfg, err := config.Load(root)
	if err != nil {
		return "", err
	}
	name := strings.TrimSpace(cfg.Workspace.Name)
	if name == "" {
		name = filepath.Base(root)
	}
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "Export agentsafe workspace settings",
		DefaultFilename: "agentsafe-workspace-" + safeFilename(name) + ".zip",
		Filters:         []runtime.FileFilter{{DisplayName: "ZIP archive", Pattern: "*.zip"}},
	})
	if err != nil || path == "" {
		return "", err
	}
	if !strings.HasSuffix(strings.ToLower(path), ".zip") {
		path += ".zip"
	}
	if err := workspacebundle.Export(root, path); err != nil {
		return "", err
	}
	return path, nil
}

func (a *App) ImportWorkspaceBundle() (config.Config, error) {
	zipPath, err := a.SelectWorkspaceBundleFile()
	if err != nil || zipPath == "" {
		return config.Config{}, err
	}
	target, err := a.SelectWorkspaceBundleTargetDir()
	if err != nil || target == "" {
		return config.Config{}, err
	}
	return a.ImportWorkspaceBundleFrom(zipPath, target)
}

func (a *App) SelectWorkspaceBundleFile() (string, error) {
	return runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title:   "Import agentsafe workspace settings",
		Filters: []runtime.FileFilter{{DisplayName: "ZIP archive", Pattern: "*.zip"}},
	})
}

func (a *App) SelectWorkspaceBundleTargetDir() (string, error) {
	return runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Select empty target folder",
	})
}

func (a *App) ImportWorkspaceBundleFrom(zipPath, target string) (config.Config, error) {
	cfg, err := workspacebundle.Import(zipPath, target)
	if err != nil {
		return config.Config{}, err
	}
	a.root = cfg.Workspace.Root
	_ = registry.Add(cfg.Workspace.Name, cfg.Workspace.Root)
	return cfg, nil
}

// ---- Worktree templates ----

func (a *App) ListWorktreeTemplates() ([]wttemplate.Template, error) {
	root, err := a.requireRoot()
	if err != nil {
		return nil, err
	}
	return wttemplate.List(root)
}

func (a *App) ImportWorktreeTemplateFiles() ([]wttemplate.Template, error) {
	root, err := a.requireRoot()
	if err != nil {
		return nil, err
	}
	paths, err := runtime.OpenMultipleFilesDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Select worktree template files",
	})
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return []wttemplate.Template{}, nil
	}
	return wttemplate.ImportFiles(root, paths)
}

func (a *App) ImportWorktreeTemplateFolder() (wttemplate.Template, error) {
	root, err := a.requireRoot()
	if err != nil {
		return wttemplate.Template{}, err
	}
	path, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Select worktree template folder",
	})
	if err != nil || path == "" {
		return wttemplate.Template{}, err
	}
	return wttemplate.ImportFolder(root, path)
}

func (a *App) ImportWorktreeTemplatePaths(paths []string) ([]wttemplate.Template, error) {
	root, err := a.requireRoot()
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return []wttemplate.Template{}, nil
	}
	return wttemplate.ImportPaths(root, paths)
}

func (a *App) UpdateWorktreeTemplate(t wttemplate.Template) error {
	root, err := a.requireRoot()
	if err != nil {
		return err
	}
	return wttemplate.Update(root, t)
}

func (a *App) DeleteWorktreeTemplate(id string) error {
	root, err := a.requireRoot()
	if err != nil {
		return err
	}
	return wttemplate.Delete(root, id)
}

func (a *App) ClearWorktreeTemplates() error {
	root, err := a.requireRoot()
	if err != nil {
		return err
	}
	return wttemplate.Clear(root)
}

func (a *App) ReadWorktreeTemplateFile(id string) (string, error) {
	root, err := a.requireRoot()
	if err != nil {
		return "", err
	}
	return wttemplate.ReadTemplateFile(root, id)
}

func (a *App) SaveWorktreeTemplateFile(id, content string) error {
	root, err := a.requireRoot()
	if err != nil {
		return err
	}
	return wttemplate.WriteTemplateFile(root, id, content)
}

func (a *App) OpenWorktreeTemplateFolder() (string, error) {
	root, err := a.requireRoot()
	if err != nil {
		return "", err
	}
	dir := wttemplate.BaseDir(root)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	if err := revealInFileManager(dir); err != nil {
		return "", err
	}
	return dir, nil
}

func (a *App) ApplyWorktreeTemplates(name string) error {
	root, err := a.requireRoot()
	if err != nil {
		return err
	}
	fm, err := feature.Load(root, name)
	if err != nil {
		return err
	}
	return a.runTask("Apply worktree templates: "+name, func() error {
		return wttemplate.Apply(root, fm.FolderKey(), desktopWorktreeTemplateRepos(root, fm.Repositories))
	})
}

func (a *App) ApplyWorkspaceRootTemplates() error {
	root, err := a.requireRoot()
	if err != nil {
		return err
	}
	return a.runTask("Apply workspace root templates", func() error {
		return wttemplate.ApplyWorkspaceRoot(root)
	})
}

func (a *App) ApplyAgentTemplates(name string) error {
	root, err := a.requireRoot()
	if err != nil {
		return err
	}
	fm, err := feature.Load(root, name)
	if err != nil {
		return err
	}
	return a.runTask("Apply agent templates: "+name, func() error {
		return wttemplate.ApplyAgent(root, fm.FolderKey(), desktopAgentTemplateRepos(root, fm.FolderKey(), fm.Repositories))
	})
}

// ---- Workspace explorer ----

type WorkspaceTreeNode struct {
	Name        string              `json:"name"`
	Path        string              `json:"path"`
	RelPath     string              `json:"relPath"`
	IsDir       bool                `json:"isDir"`
	Size        int64               `json:"size"`
	ModTime     string              `json:"modTime"`
	FeatureName string              `json:"featureName,omitempty"`
	Branch      string              `json:"branch,omitempty"`
	Children    []WorkspaceTreeNode `json:"children"`
}

func (a *App) WorkspaceTree(path string) (WorkspaceTreeNode, error) {
	root, err := a.requireRoot()
	if err != nil {
		return WorkspaceTreeNode{}, err
	}
	target, err := workspacePath(root, path)
	if err != nil {
		return WorkspaceTreeNode{}, err
	}
	return treeNode(root, target, workspaceTreeLabels(root))
}

func (a *App) OpenPath(path string) (string, error) {
	root, err := a.requireRoot()
	if err != nil {
		return "", err
	}
	target, err := workspacePath(root, path)
	if err != nil {
		return "", err
	}
	if err := openOSPath(target); err != nil {
		return "", err
	}
	return target, nil
}

func (a *App) OpenPathVSCode(path string) (string, error) {
	root, err := a.requireRoot()
	if err != nil {
		return "", err
	}
	target, err := workspacePath(root, path)
	if err != nil {
		return "", err
	}
	cmd := exec.Command("code", target)
	if err := cmd.Start(); err != nil {
		return "", err
	}
	return target, nil
}

func (a *App) DeleteWorkspacePath(path string) error {
	root, err := a.requireRoot()
	if err != nil {
		return err
	}
	target, err := workspacePath(root, path)
	if err != nil {
		return err
	}
	if err := ensureExplorerDeleteAllowed(root, target); err != nil {
		return err
	}
	return fsutil.ForceRemoveAll(target)
}

const maxEditableWorkspaceFileSize int64 = 2 * 1024 * 1024

func (a *App) ReadWorkspaceFile(path string) (string, error) {
	root, err := a.requireRoot()
	if err != nil {
		return "", err
	}
	target, err := workspacePath(root, path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(target)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("cannot edit a directory")
	}
	if info.Size() > maxEditableWorkspaceFileSize {
		return "", fmt.Errorf("file is too large to edit in the app (max 2 MB)")
	}
	b, err := os.ReadFile(target)
	if err != nil {
		return "", err
	}
	if hasNUL(b) || !utf8.Valid(b) {
		return "", fmt.Errorf("binary files cannot be edited in the app")
	}
	return string(b), nil
}

func (a *App) SaveWorkspaceFile(path, content string) error {
	root, err := a.requireRoot()
	if err != nil {
		return err
	}
	target, err := workspacePath(root, path)
	if err != nil {
		return err
	}
	info, err := os.Stat(target)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("cannot edit a directory")
	}
	return os.WriteFile(target, []byte(content), info.Mode().Perm())
}

func hasNUL(b []byte) bool {
	for _, c := range b {
		if c == 0 {
			return true
		}
	}
	return false
}

type TerminalSession struct {
	ID       string `json:"id"`
	Path     string `json:"path"`
	Title    string `json:"title"`
	External bool   `json:"external,omitempty"`
}

func (a *App) TerminalOpen(path string) (TerminalSession, error) {
	root, err := a.requireRoot()
	if err != nil {
		return TerminalSession{}, err
	}
	target, err := workspacePath(root, path)
	if err != nil {
		return TerminalSession{}, err
	}
	if info, statErr := os.Stat(target); statErr == nil && !info.IsDir() {
		target = filepath.Dir(target)
	} else if statErr != nil {
		return TerminalSession{}, statErr
	}
	shell, args := defaultShell()
	session, err := a.startPTY(target, "", shell, args)
	if err != nil && isPTYUnsupported(err) {
		if _, openErr := openTerminalAt(target); openErr != nil {
			return TerminalSession{}, openErr
		}
		return TerminalSession{ID: "external", Path: target, Title: filepath.Base(target), External: true}, nil
	}
	return session, err
}

// startPTY launches argv in a pty rooted at target, registers it as a tracked
// terminal, and streams its output. When feature is non-empty the session is
// tagged as a managed agent run so its exit is also reported via "agent:exit".
func (a *App) startPTY(target, feature, name string, args []string) (TerminalSession, error) {
	ptyProc, err := startPTYProcess(target, name, args, 80, 24)
	if err != nil {
		return TerminalSession{}, err
	}
	a.terminalMu.Lock()
	a.terminalSeq++
	id := fmt.Sprintf("term-%d", a.terminalSeq)
	if a.terminals == nil {
		a.terminals = map[string]*terminalProcess{}
	}
	a.terminals[id] = &terminalProcess{pty: ptyProc, path: target, feature: feature}
	a.terminalMu.Unlock()

	go a.pipeTerminalOutput(id, ptyProc, feature)
	return TerminalSession{ID: id, Path: target, Title: filepath.Base(target)}, nil
}

func (a *App) TerminalWrite(id, data string) error {
	tp, err := a.terminal(id)
	if err != nil {
		return err
	}
	_, err = io.WriteString(tp.pty, data)
	return err
}

func (a *App) TerminalResize(id string, cols, rows int) error {
	tp, err := a.terminal(id)
	if err != nil {
		return err
	}
	if cols <= 0 || rows <= 0 {
		return nil
	}
	return tp.pty.Resize(cols, rows)
}

func (a *App) TerminalClose(id string) error {
	tp, err := a.terminal(id)
	if err != nil {
		return nil
	}
	a.removeTerminal(id)
	_ = tp.pty.Kill()
	_ = tp.pty.Close()
	return nil
}

func (a *App) terminal(id string) (*terminalProcess, error) {
	a.terminalMu.Lock()
	defer a.terminalMu.Unlock()
	tp := a.terminals[id]
	if tp == nil {
		return nil, fmt.Errorf("terminal %q not found", id)
	}
	return tp, nil
}

func (a *App) removeTerminal(id string) {
	a.terminalMu.Lock()
	delete(a.terminals, id)
	a.terminalMu.Unlock()
}

func (a *App) pipeTerminalOutput(id string, ptyProc ptySession, feature string) {
	buf := make([]byte, 8192)
	for {
		n, err := ptyProc.Read(buf)
		if n > 0 {
			runtime.EventsEmit(a.ctx, "terminal:data", map[string]any{
				"id":   id,
				"data": string(buf[:n]),
			})
		}
		if err != nil {
			break
		}
	}
	waitErr := ptyProc.Wait()
	a.removeTerminal(id)
	status := "closed"
	message := ""
	if waitErr != nil {
		status = "error"
		message = waitErr.Error()
	}
	runtime.EventsEmit(a.ctx, "terminal:close", map[string]any{
		"id": id, "status": status, "error": message,
	})
	// Managed agent run finished — signal the feature so the UI can refresh the
	// diff and prompt the user to sync.
	if feature != "" {
		runtime.EventsEmit(a.ctx, "agent:exit", map[string]any{
			"id": id, "feature": feature, "status": status, "error": message,
		})
	}
	_ = ptyProc.Close()
}

func defaultShell() (string, []string) {
	if goruntime.GOOS == "windows" {
		if shell := os.Getenv("COMSPEC"); shell != "" {
			return shell, nil
		}
		return "cmd.exe", nil
	}
	if shell := os.Getenv("SHELL"); shell != "" {
		return shell, []string{"-l"}
	}
	return "/bin/sh", []string{"-l"}
}

// commandShell wraps a command line so it runs through the user's login shell,
// resolving PATH/aliases and keeping a TTY for interactive agents.
func commandShell(command string) (string, []string) {
	if goruntime.GOOS == "windows" {
		if shell := os.Getenv("COMSPEC"); shell != "" {
			return shell, []string{"/c", command}
		}
		return "cmd.exe", []string{"/c", command}
	}
	if shell := os.Getenv("SHELL"); shell != "" {
		return shell, []string{"-lc", command}
	}
	return "/bin/sh", []string{"-lc", command}
}

// ---- Repositories ----

func (a *App) ListRepos() ([]config.Repository, error) {
	cfg, err := a.GetConfig()
	if err != nil {
		return nil, err
	}
	if cfg.Repositories == nil {
		return []config.Repository{}, nil
	}
	return cfg.Repositories, nil
}

func (a *App) AddRepo(name, url, defaultBranch string) error {
	root, err := a.requireRoot()
	if err != nil {
		return err
	}
	cfg, err := config.Load(root)
	if err != nil {
		return err
	}
	_, err = config.AddRepository(root, cfg, config.Repository{
		Name: name, URL: url, DefaultBranch: defaultBranch,
	})
	return err
}

// RemoveRepo drops a repository from the open workspace's config. When
// deleteFiles is true it also deletes the cloned files (main/<repo> and the
// repo's feature worktrees, agent copies and sync history); otherwise only
// config.yaml is edited and cloned directories are left in place.
func (a *App) RemoveRepo(name string, deleteFiles bool) error {
	root, err := a.requireRoot()
	if err != nil {
		return err
	}
	cfg, err := config.Load(root)
	if err != nil {
		return err
	}
	_, _, err = repo.Remove(root, cfg, name, deleteFiles)
	return err
}

func (a *App) Pull() error {
	root, err := a.requireRoot()
	if err != nil {
		return err
	}
	cfg, err := config.Load(root)
	if err != nil {
		return err
	}
	return a.runTask("Pull repositories", func() error { return repo.PullAll(root, cfg) })
}

// RepoLocalStates reports whether each configured repository has a path under
// main/. A present non-Git path is still reported as present so PullRepo can
// surface the Git error without overwriting user files.
func (a *App) RepoLocalStates() (map[string]bool, error) {
	root, err := a.requireRoot()
	if err != nil {
		return nil, err
	}
	cfg, err := config.Load(root)
	if err != nil {
		return nil, err
	}
	states := make(map[string]bool, len(cfg.Repositories))
	for _, r := range cfg.Repositories {
		path := config.RepoPath(root, r.Name)
		st, statErr := os.Stat(path)
		present := statErr == nil
		// An empty leftover directory is treated as not cloned so the UI shows
		// the Clone action; non-empty non-Git paths stay "present" so PullRepo
		// can surface a Git error without overwriting user files.
		if present && st.IsDir() {
			if empty, e := fsutil.IsEmptyDir(path); e == nil && empty {
				present = false
			}
		}
		states[r.Name] = present
	}
	return states, nil
}

type RepoRuntimeState struct {
	Name           string   `json:"name"`
	Local          bool     `json:"local"`
	CurrentBranch  string   `json:"currentBranch"`
	RemoteBranches []string `json:"remoteBranches"`
	Error          string   `json:"error,omitempty"`
}

func (a *App) RepoRuntimeStates() ([]RepoRuntimeState, error) {
	root, err := a.requireRoot()
	if err != nil {
		return nil, err
	}
	cfg, err := config.Load(root)
	if err != nil {
		return nil, err
	}
	out := make([]RepoRuntimeState, 0, len(cfg.Repositories))
	for _, r := range cfg.Repositories {
		state := RepoRuntimeState{Name: r.Name, RemoteBranches: []string{}}
		repoPath := config.RepoPath(root, r.Name)
		if st, statErr := os.Stat(repoPath); statErr != nil || !st.IsDir() {
			state.Local = false
			if statErr != nil && !os.IsNotExist(statErr) {
				state.Error = statErr.Error()
			}
			out = append(out, state)
			continue
		} else if empty, emptyErr := fsutil.IsEmptyDir(repoPath); emptyErr == nil && empty {
			state.Local = false
			out = append(out, state)
			continue
		}
		state.Local = true
		// Remove any phantom origin/origin… refs left by an earlier buggy fetch so
		// they neither show in the dropdown nor get selected for checkout.
		_ = aggit.PruneStaleOriginRefs(repoPath)
		if current, branchErr := aggit.CurrentBranch(repoPath); branchErr == nil {
			state.CurrentBranch = current
		} else {
			state.Error = branchErr.Error()
		}
		if branches, branchErr := aggit.ListRemoteBranches(repoPath); branchErr == nil {
			state.RemoteBranches = branches
		} else if state.Error == "" {
			state.Error = branchErr.Error()
		}
		out = append(out, state)
	}
	return out, nil
}

func (a *App) CheckoutRepoBranch(name, remoteBranch string) error {
	root, err := a.requireRoot()
	if err != nil {
		return err
	}
	cfg, err := config.Load(root)
	if err != nil {
		return err
	}
	if _, ok := findRepo(cfg, name); !ok {
		return fmt.Errorf("repository %q not found", name)
	}
	repoPath := config.RepoPath(root, name)
	return a.runTask("Checkout repository branch: "+name, func() error {
		return aggit.CheckoutRemoteBranch(repoPath, remoteBranch)
	})
}

// PullRepo clones a missing repository or fetches/pulls an existing one.
func (a *App) PullRepo(name string) error {
	root, err := a.requireRoot()
	if err != nil {
		return err
	}
	cfg, err := config.Load(root)
	if err != nil {
		return err
	}
	rc, ok := findRepo(cfg, name)
	if !ok {
		return fmt.Errorf("repository %q not found", name)
	}
	host, https := httpsHost(rc.URL)
	credential := gitCredential{}
	if https {
		a.credentialMu.Lock()
		credential = a.credentials[host]
		a.credentialMu.Unlock()
	}
	return a.runTask("Pull repository: "+name, func() error {
		pullErr := repo.PullOneWithCredentials(root, cfg, name, credential.Username, credential.Secret)
		if pullErr == nil || !https || !aggit.IsAuthenticationError(pullErr) {
			return pullErr
		}
		if credential.Secret != "" {
			a.credentialMu.Lock()
			delete(a.credentials, host)
			a.credentialMu.Unlock()
		}
		payload, _ := json.Marshal(map[string]string{
			"code": "authentication_required", "repo": name, "host": host, "protocol": "https",
		})
		return fmt.Errorf("AGENTSAFE_AUTH_REQUIRED:%s", payload)
	})
}

// SetGitCredentials stores HTTPS credentials in memory for the current app
// process. They are never written to config or a credential helper.
func (a *App) SetGitCredentials(host, username, secret string) error {
	host = strings.ToLower(strings.TrimSpace(host))
	username = strings.TrimSpace(username)
	if host == "" || username == "" || secret == "" {
		return fmt.Errorf("host, username, and password/token are required")
	}
	if strings.ContainsAny(host, `/\@`) {
		return fmt.Errorf("invalid credential host")
	}
	a.credentialMu.Lock()
	if a.credentials == nil {
		a.credentials = map[string]gitCredential{}
	}
	a.credentials[host] = gitCredential{Username: username, Secret: secret}
	a.credentialMu.Unlock()
	return nil
}

func httpsHost(raw string) (string, bool) {
	u, err := url.Parse(raw)
	if err != nil || !strings.EqualFold(u.Scheme, "https") || u.Hostname() == "" {
		return "", false
	}
	host := strings.ToLower(u.Hostname())
	if u.Port() != "" {
		host += ":" + u.Port()
	}
	return host, true
}

// ---- Features ----

func (a *App) ListFeatures() (feature.FeatureListResult, error) {
	root, err := a.requireRoot()
	if err != nil {
		return feature.FeatureListResult{}, err
	}
	return feature.ListData(root)
}

func (a *App) CreateFeature(name, base, existingBranch string) error {
	root, err := a.requireRoot()
	if err != nil {
		return err
	}
	cfg, err := config.Load(root)
	if err != nil {
		return err
	}
	policy, err := feature.ParseExistingBranchPolicy(existingBranch)
	if err != nil {
		return err
	}
	return a.runTask("Create feature: "+name, func() error {
		return feature.CreateWithOptions(root, cfg, name, feature.CreateOptions{
			Base: base, ExistingBranch: policy,
		})
	})
}

func (a *App) CheckFeatureCreation(name, base string) (feature.CreateCheck, error) {
	root, err := a.requireRoot()
	if err != nil {
		return feature.CreateCheck{}, err
	}
	cfg, err := config.Load(root)
	if err != nil {
		return feature.CreateCheck{}, err
	}
	var result feature.CreateCheck
	err = a.runTask("Check feature: "+name, func() error {
		var checkErr error
		result, checkErr = feature.CheckCreate(root, cfg, name, base)
		return checkErr
	})
	return result, err
}

func (a *App) FeatureRepoAdd(name, repoName, existingBranch string, force bool) (feature.RepoMeta, error) {
	return a.configureFeatureRepo(name, repoName, existingBranch, false, force)
}

func (a *App) FeatureRepoRecreate(name, repoName, existingBranch string, force bool) (feature.RepoMeta, error) {
	return a.configureFeatureRepo(name, repoName, existingBranch, true, force)
}

func (a *App) configureFeatureRepo(name, repoName, existingBranch string, recreate, force bool) (feature.RepoMeta, error) {
	root, err := a.requireRoot()
	if err != nil {
		return feature.RepoMeta{}, err
	}
	cfg, err := config.Load(root)
	if err != nil {
		return feature.RepoMeta{}, err
	}
	policy, err := feature.ParseExistingBranchPolicy(existingBranch)
	if err != nil {
		return feature.RepoMeta{}, err
	}
	var result feature.RepoMeta
	err = a.runTask("Configure worktree: "+taskTarget(name, repoName), func() error {
		var configureErr error
		result, configureErr = feature.ConfigureRepositoryWorktree(root, cfg, name, repoName, feature.RepositoryWorktreeOptions{
			ExistingBranch: policy,
			Recreate:       recreate,
			Force:          force,
		})
		return configureErr
	})
	return result, err
}

// RebaseFeature rebases the feature's worktrees onto their base branch.
// repoFilter, when non-empty, limits the operation to one repository.
func (a *App) RebaseFeature(name, repoFilter string) (feature.RebaseResult, error) {
	root, err := a.requireRoot()
	if err != nil {
		return feature.RebaseResult{}, err
	}
	cfg, err := config.Load(root)
	if err != nil {
		return feature.RebaseResult{}, err
	}
	var res feature.RebaseResult
	err = a.runTask("Rebase: "+name, func() error {
		var e error
		res, e = feature.Rebase(root, cfg, name, repoFilter)
		return e
	})
	return res, err
}

// FeatureDelete removes a feature's worktrees and artifacts. deleteBranch also
// removes the local feature branch; force removes worktrees with uncommitted
// changes (otherwise refused).
func (a *App) FeatureDelete(name string, deleteBranch, force bool) (feature.DeleteResult, error) {
	root, err := a.requireRoot()
	if err != nil {
		return feature.DeleteResult{}, err
	}
	var result feature.DeleteResult
	err = a.runTask("Delete feature: "+name, func() error {
		var deleteErr error
		result, deleteErr = feature.DeleteWithResult(root, name, feature.DeleteOptions{DeleteBranch: deleteBranch, Force: force})
		return deleteErr
	})
	return result, err
}

func (a *App) FeatureStatus(name string) (feature.FeatureStatusResult, error) {
	root, err := a.requireRoot()
	if err != nil {
		return feature.FeatureStatusResult{}, err
	}
	return feature.StatusData(root, name)
}

func (a *App) LoadFeature(name string) (feature.Metadata, error) {
	root, err := a.requireRoot()
	if err != nil {
		return feature.Metadata{}, err
	}
	return feature.Load(root, name)
}

// ---- Agent ----

// AgentPrepare builds (or rebuilds) the sanitized agent workspace for a
// feature. When backup is true an existing workspace is preserved as a
// timestamped ".bak-" directory; otherwise it is replaced.
func (a *App) AgentPrepare(name string, backup bool) (agent.PrepareMetadata, error) {
	root, err := a.requireRoot()
	if err != nil {
		return agent.PrepareMetadata{}, err
	}
	cfg, err := config.Load(root)
	if err != nil {
		return agent.PrepareMetadata{}, err
	}
	var meta agent.PrepareMetadata
	err = a.runTask("Prepare agent: "+name, func() error {
		if e := agent.Init(root, cfg, name, agent.PrepareOptions{Backup: backup}); e != nil {
			return e
		}
		meta = agent.LoadPrepareMetadata(root, name)
		return nil
	})
	return meta, err
}

// AgentPrepareRepo prepares only one repository's sanitized agent folder.
func (a *App) AgentPrepareRepo(name, repoName string, backup bool) (agent.PrepareMetadata, error) {
	root, err := a.requireRoot()
	if err != nil {
		return agent.PrepareMetadata{}, err
	}
	cfg, err := config.Load(root)
	if err != nil {
		return agent.PrepareMetadata{}, err
	}
	var meta agent.PrepareMetadata
	err = a.runTask("Prepare agent: "+taskTarget(name, repoName), func() error {
		var prepareErr error
		meta, prepareErr = agent.PrepareRepository(root, cfg, name, repoName, agent.PrepareOptions{Backup: backup})
		return prepareErr
	})
	return meta, err
}

// RepoDiff groups agent changes per repository for the frontend.
type RepoDiff struct {
	Name    string         `json:"name"`
	Changes []agent.Change `json:"changes"`
}

// DiffResult mirrors the CLI's structured diff output.
type DiffResult struct {
	Feature      string     `json:"feature"`
	Repositories []RepoDiff `json:"repositories"`
}

func (a *App) AgentDiff(name, repoFilter string) (DiffResult, error) {
	root, err := a.requireRoot()
	if err != nil {
		return DiffResult{}, err
	}
	cfg, err := config.Load(root)
	if err != nil {
		return DiffResult{}, err
	}
	byRepo, err := agent.Diff(root, cfg, name, repoFilter)
	if err != nil {
		return DiffResult{}, err
	}
	result := DiffResult{Feature: name, Repositories: []RepoDiff{}}
	repos := make([]string, 0, len(byRepo))
	for r := range byRepo {
		repos = append(repos, r)
	}
	sort.Strings(repos)
	for _, r := range repos {
		changes := byRepo[r]
		if changes == nil {
			changes = []agent.Change{}
		}
		result.Repositories = append(result.Repositories, RepoDiff{Name: r, Changes: changes})
	}
	return result, nil
}

// SyncOptions is the frontend-facing subset of agent.Options.
type SyncOptions struct {
	Repo            string `json:"repo"`
	DryRun          bool   `json:"dryRun"`
	IncludeRisky    bool   `json:"includeRisky"`
	AllowMaskedSync bool   `json:"allowMaskedSync"`
}

func (a *App) AgentSync(name string, opt SyncOptions) error {
	root, err := a.requireRoot()
	if err != nil {
		return err
	}
	cfg, err := config.Load(root)
	if err != nil {
		return err
	}
	// Yes:true — the GUI shows the diff before syncing, so no interactive prompt.
	return a.runTask("Sync: "+name, func() error {
		return agent.Sync(root, cfg, name, agent.Options{
			Repo:            opt.Repo,
			DryRun:          opt.DryRun,
			IncludeRisky:    opt.IncludeRisky,
			AllowMaskedSync: opt.AllowMaskedSync,
			Yes:             true,
		})
	})
}

// SyncAndCommit syncs reviewed agent changes back to the worktrees and, unless
// it is a dry run or the message is empty, commits them with the given message.
// The diff is shown in the UI beforehand, so the sync runs non-interactively.
func (a *App) SyncAndCommit(name, message string, opt SyncOptions) error {
	root, err := a.requireRoot()
	if err != nil {
		return err
	}
	cfg, err := config.Load(root)
	if err != nil {
		return err
	}
	return a.runTask("Sync & commit: "+name, func() error {
		return agent.SyncAndCommit(root, cfg, name, message, agent.Options{
			Repo:            opt.Repo,
			DryRun:          opt.DryRun,
			IncludeRisky:    opt.IncludeRisky,
			AllowMaskedSync: opt.AllowMaskedSync,
			Yes:             true,
		})
	})
}

func (a *App) AgentRestoreFromWorktree(name, repoName, path string) error {
	root, err := a.requireRoot()
	if err != nil {
		return err
	}
	cfg, err := config.Load(root)
	if err != nil {
		return err
	}
	return agent.RestoreFromWorktree(root, cfg, name, repoName, path)
}

func (a *App) AgentDelete(name string) error {
	root, err := a.requireRoot()
	if err != nil {
		return err
	}
	return agent.Delete(root, name)
}

// ---- Sync history ----

// SyncHistoryEntry is one sync record for the frontend, with a CanRollback flag
// set only on the newest entry of each (feature, repo) stack.
type SyncHistoryEntry struct {
	ID          string             `json:"id"`
	Feature     string             `json:"feature"`
	Repo        string             `json:"repo"`
	SyncedAt    string             `json:"syncedAt"`
	ChangeCount int                `json:"changeCount"`
	Changes     []agent.SyncChange `json:"changes"`
	CanRollback bool               `json:"canRollback"`
}

// AllSyncHistory returns every sync record across features/repos, newest first.
// Only the latest entry per (feature, repo) is rollbackable (LIFO stack).
func (a *App) AllSyncHistory() ([]SyncHistoryEntry, error) {
	root, err := a.requireRoot()
	if err != nil {
		return nil, err
	}
	recs, err := agent.ListAllHistory(root)
	if err != nil {
		return nil, err
	}
	topSeen := map[string]bool{}
	out := []SyncHistoryEntry{}
	for _, r := range recs {
		key := r.Feature + "\x00" + r.Repo
		changes := r.Changes
		if changes == nil {
			changes = []agent.SyncChange{}
		}
		entry := SyncHistoryEntry{
			ID:          r.ID,
			Feature:     r.Feature,
			Repo:        r.Repo,
			SyncedAt:    r.SyncedAt,
			ChangeCount: len(changes),
			Changes:     changes,
		}
		if !topSeen[key] {
			entry.CanRollback = true
			topSeen[key] = true
		}
		out = append(out, entry)
	}
	return out, nil
}

// RollbackSync undoes the most recent sync for a repository, restoring its
// worktree to the pre-sync state.
func (a *App) RollbackSync(name, repo, id string) error {
	root, err := a.requireRoot()
	if err != nil {
		return err
	}
	meta, err := feature.Load(root, name)
	if err != nil {
		return err
	}
	for _, r := range meta.Repositories {
		if r.Name == repo {
			return agent.Rollback(root, name, repo, id, filepath.Join(root, r.WorktreePath))
		}
	}
	return fmt.Errorf("repository %q not found in feature %q", repo, name)
}

// SyncHistoryCounts returns the sync-history stack depth per repository for a
// feature, used for the count badges.
func (a *App) SyncHistoryCounts(name string) (map[string]int, error) {
	root, err := a.requireRoot()
	if err != nil {
		return nil, err
	}
	meta, err := feature.Load(root, name)
	if err != nil {
		return nil, err
	}
	out := map[string]int{}
	for _, r := range meta.Repositories {
		out[r.Name] = agent.HistoryDepth(root, name, r.Name)
	}
	return out, nil
}

// ---- Agent security (ignore / mask) ----

// loadSecurity migrates any legacy split files and returns the unified security
// config for the workspace root.
func (a *App) loadSecurity() (string, config.Config, agent.SecurityFile, error) {
	root, err := a.requireRoot()
	if err != nil {
		return "", config.Config{}, agent.SecurityFile{}, err
	}
	cfg, err := config.Load(root)
	if err != nil {
		return "", config.Config{}, agent.SecurityFile{}, err
	}
	_ = agent.EnsureSecurityFile(cfg, root)
	return root, cfg, agent.LoadSecurity(cfg, root), nil
}

// GetAgentIgnore returns the ignore patterns from the unified agentsafe.yaml as
// newline-separated text (gitignore-style, one per line, "#" comments kept).
func (a *App) GetAgentIgnore() (string, error) {
	_, _, sf, err := a.loadSecurity()
	if err != nil {
		return "", err
	}
	return strings.Join(sf.Ignore, "\n"), nil
}

// SaveAgentIgnore writes the ignore patterns into the unified agentsafe.yaml,
// preserving the existing mask rules.
func (a *App) SaveAgentIgnore(content string) error {
	root, cfg, sf, err := a.loadSecurity()
	if err != nil {
		return err
	}
	sf.Ignore = splitLines(content)
	return agent.WriteSecurity(cfg, root, sf)
}

// GetMaskFile returns the masking rules from the unified agentsafe.yaml. Returns
// an empty rule set when none are defined.
func (a *App) GetMaskFile() (agent.MaskFile, error) {
	_, _, sf, err := a.loadSecurity()
	if err != nil {
		return agent.MaskFile{}, err
	}
	rules := sf.Mask
	if rules == nil {
		rules = []agent.MaskRule{}
	}
	return agent.MaskFile{Rules: rules}, nil
}

// SaveMaskFile writes the masking rules into the unified agentsafe.yaml,
// preserving the existing ignore patterns.
func (a *App) SaveMaskFile(m agent.MaskFile) error {
	root, cfg, sf, err := a.loadSecurity()
	if err != nil {
		return err
	}
	sf.Mask = m.Rules
	return agent.WriteSecurity(cfg, root, sf)
}

// SecurityTemplate is a stack-specific agentsafe.yaml preset surfaced in the UI.
type SecurityTemplate struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Description string `json:"description"`
	IgnoreCount int    `json:"ignoreCount"`
	MaskCount   int    `json:"maskCount"`
}

// ListSecurityTemplates returns the available stack templates.
func (a *App) ListSecurityTemplates() ([]SecurityTemplate, error) {
	out := []SecurityTemplate{}
	for _, t := range agent.TemplateList() {
		out = append(out, SecurityTemplate{
			Key:         t.Key,
			Label:       t.Label,
			Description: t.Description,
			IgnoreCount: t.IgnoreCount,
			MaskCount:   t.MaskCount,
		})
	}
	return out, nil
}

// ApplySecurityTemplates merges (or replaces) the named templates into the
// workspace's agentsafe.yaml.
func (a *App) ApplySecurityTemplates(keys []string, replace bool) error {
	root, err := a.requireRoot()
	if err != nil {
		return err
	}
	cfg, err := config.Load(root)
	if err != nil {
		return err
	}
	_, err = agent.ApplyTemplates(cfg, root, keys, replace)
	return err
}

// splitLines splits textarea content into trimmed-of-CR lines, dropping a single
// trailing empty line so a round-trip through the editor is stable.
func splitLines(content string) []string {
	if content == "" {
		return []string{}
	}
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// AgentSecurityBundle is the legacy JSON export/import format. It is retained so
// previously exported bundles can still be imported; new exports use the
// unified agentsafe.yaml directly.
type AgentSecurityBundle struct {
	Version int            `json:"version"`
	Ignore  string         `json:"ignore"`
	Mask    agent.MaskFile `json:"mask"`
}

// ExportAgentSecurity writes the workspace's unified agentsafe.yaml to a file
// chosen via a native save dialog. Returns the saved path, or "" when the
// dialog is cancelled.
func (a *App) ExportAgentSecurity() (string, error) {
	_, _, sf, err := a.loadSecurity()
	if err != nil {
		return "", err
	}
	data, err := yaml.Marshal(sf)
	if err != nil {
		return "", err
	}
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "Export Agent security settings",
		DefaultFilename: "agentsafe.yaml",
		Filters:         []runtime.FileFilter{{DisplayName: "YAML", Pattern: "*.yaml;*.yml"}},
	})
	if err != nil || path == "" {
		return "", err
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", err
	}
	return path, nil
}

// ImportAgentSecurity reads an exported security file (chosen via a native open
// dialog) and overwrites the workspace's unified agentsafe.yaml. It accepts the
// unified YAML format and, for backward compatibility, the legacy JSON bundle.
// Returns the imported path, or "" when the dialog is cancelled.
func (a *App) ImportAgentSecurity() (string, error) {
	root, cfg, _, err := a.loadSecurity()
	if err != nil {
		return "", err
	}
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Import Agent security settings",
		Filters: []runtime.FileFilter{
			{DisplayName: "Security config", Pattern: "*.yaml;*.yml;*.json"},
		},
	})
	if err != nil || path == "" {
		return "", err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sf, err := parseImportedSecurity(b)
	if err != nil {
		return "", err
	}
	if err := agent.WriteSecurity(cfg, root, sf); err != nil {
		return "", err
	}
	return path, nil
}

// parseImportedSecurity decodes an imported security config, accepting the
// unified YAML format and the legacy JSON bundle.
func parseImportedSecurity(b []byte) (agent.SecurityFile, error) {
	// Legacy JSON bundle ({"version":1,"ignore":"...","mask":{...}}).
	var bundle AgentSecurityBundle
	if err := json.Unmarshal(b, &bundle); err == nil && bundle.Version >= 1 {
		return agent.SecurityFile{Ignore: splitLines(bundle.Ignore), Mask: bundle.Mask.Rules}, nil
	}
	// Unified YAML (also parses unified JSON, since JSON is valid YAML).
	var sf agent.SecurityFile
	if err := yaml.Unmarshal(b, &sf); err != nil {
		return agent.SecurityFile{}, fmt.Errorf("invalid settings file: %w", err)
	}
	return sf, nil
}

// ---- Backups ----

// backupSuffix separates a repo name from its timestamp in a backup directory
// name: agent/<feature>/<repo>.bak-<YYYYMMDDHHMMSS>.
const backupSuffix = ".bak-"

// BackupEntry describes one backed-up agent workspace copy.
type BackupEntry struct {
	Feature   string `json:"feature"`
	Repo      string `json:"repo"`
	Path      string `json:"path"`      // slash path relative to the workspace root
	CreatedAt string `json:"createdAt"` // RFC3339 when parseable, else the raw stamp
	Size      int64  `json:"size"`
	Files     int    `json:"files"`
}

// ListBackups scans agent/<feature>/ for ".bak-" directories and returns them
// newest-first.
func (a *App) ListBackups() ([]BackupEntry, error) {
	root, err := a.requireRoot()
	if err != nil {
		return nil, err
	}
	agentDir := filepath.Join(root, "agent")
	features, err := os.ReadDir(agentDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []BackupEntry{}, nil
		}
		return nil, err
	}
	out := []BackupEntry{}
	for _, fe := range features {
		if !fe.IsDir() {
			continue
		}
		featureName := fe.Name()
		entries, err := os.ReadDir(filepath.Join(agentDir, featureName))
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			idx := strings.LastIndex(e.Name(), backupSuffix)
			if idx < 0 {
				continue
			}
			repo := e.Name()[:idx]
			stamp := e.Name()[idx+len(backupSuffix):]
			created := stamp
			if ts, perr := time.ParseInLocation("20060102150405", stamp, time.Local); perr == nil {
				created = ts.Format(time.RFC3339)
			}
			abs := filepath.Join(agentDir, featureName, e.Name())
			size, files := dirStats(abs)
			rel, _ := filepath.Rel(root, abs)
			out = append(out, BackupEntry{
				Feature:   featureName,
				Repo:      repo,
				Path:      filepath.ToSlash(rel),
				CreatedAt: created,
				Size:      size,
				Files:     files,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	return out, nil
}

// dirStats returns the total byte size and file count under a directory.
func dirStats(dir string) (int64, int) {
	var size int64
	var files int
	_ = filepath.WalkDir(dir, func(_ string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if info, ierr := d.Info(); ierr == nil {
			size += info.Size()
			files++
		}
		return nil
	})
	return size, files
}

// safeBackupPath validates a frontend-supplied relative backup path and returns
// its absolute form. It rejects paths that escape the workspace's agent/
// directory or are not ".bak-" backups, guarding against deleting live data.
func (a *App) safeBackupPath(root, rel string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(rel))
	if clean == "" || strings.Contains(clean, "..") {
		return "", fmt.Errorf("invalid backup path")
	}
	if !strings.HasPrefix(clean, "agent"+string(filepath.Separator)) {
		return "", fmt.Errorf("backup path must be under agent/")
	}
	if !strings.Contains(filepath.Base(clean), backupSuffix) {
		return "", fmt.Errorf("not a backup directory")
	}
	abs := filepath.Join(root, clean)
	st, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if !st.IsDir() {
		return "", fmt.Errorf("backup path is not a directory")
	}
	return abs, nil
}

// DeleteAllBackups removes every ".bak-" backup directory in the workspace and
// returns how many were deleted.
func (a *App) DeleteAllBackups() (int, error) {
	root, err := a.requireRoot()
	if err != nil {
		return 0, err
	}
	list, err := a.ListBackups()
	if err != nil {
		return 0, err
	}
	n := 0
	for _, b := range list {
		abs, err := a.safeBackupPath(root, b.Path)
		if err != nil {
			continue
		}
		if os.RemoveAll(abs) == nil {
			n++
		}
	}
	return n, nil
}

// DeleteBackup removes a backup directory.
func (a *App) DeleteBackup(path string) error {
	root, err := a.requireRoot()
	if err != nil {
		return err
	}
	abs, err := a.safeBackupPath(root, path)
	if err != nil {
		return err
	}
	return os.RemoveAll(abs)
}

// RestoreBackup promotes a backup to the live agent workspace, replacing the
// current copy (agent/<feature>/<repo>). The backup directory is consumed.
func (a *App) RestoreBackup(path string) error {
	root, err := a.requireRoot()
	if err != nil {
		return err
	}
	abs, err := a.safeBackupPath(root, path)
	if err != nil {
		return err
	}
	base := filepath.Base(abs)
	idx := strings.LastIndex(base, backupSuffix)
	if idx < 0 {
		return fmt.Errorf("not a backup directory")
	}
	live := filepath.Join(filepath.Dir(abs), base[:idx])
	if err := os.RemoveAll(live); err != nil {
		return err
	}
	return os.Rename(abs, live)
}

// ---- Commit / Push / MR ----

// Commit commits the worktree changes for a feature. When repoFilter is
// non-empty, only that repository is committed; otherwise every repository is.
func (a *App) Commit(name, message, repoFilter string) error {
	root, err := a.requireRoot()
	if err != nil {
		return err
	}
	return a.runTask("Commit: "+taskTarget(name, repoFilter), func() error {
		return feature.Commit(root, name, message, repoFilter)
	})
}

// Push pushes a feature's branches. When repoFilter is non-empty, only that
// repository is pushed; otherwise every repository is.
func (a *App) Push(name, repoFilter string) error {
	root, err := a.requireRoot()
	if err != nil {
		return err
	}
	return a.runTask("Push: "+taskTarget(name, repoFilter), func() error {
		return feature.Push(root, name, repoFilter)
	})
}

// taskTarget formats a progress-task label target as "<feature>/<repo>" when a
// repo filter is set, or just "<feature>" for an all-repos operation.
func taskTarget(name, repoFilter string) string {
	if repoFilter != "" {
		return name + "/" + repoFilter
	}
	return name
}

// RequestResult is the per-repository outcome of creating a merge/pull request.
type RequestResult struct {
	Repo     string `json:"repo"`
	Provider string `json:"provider"` // "github" | "gitlab" | "" (unknown)
	Method   string `json:"method"`   // "api" | "browser" | "skipped"
	URL      string `json:"url"`
	Branch   string `json:"branch"`
	Target   string `json:"target"`
	Error    string `json:"error,omitempty"`
}

// RequestResults groups the per-repo outcomes for a worktree.
type RequestResults struct {
	Feature string          `json:"feature"`
	Items   []RequestResult `json:"items"`
}

// CreateMergeRequests opens a PR (GitHub) or MR (GitLab) for each repository in
// the worktree. Provider is detected from each repo's URL. When the provider's
// env token is set the request is created via API; otherwise a browser URL is
// returned for the frontend to open.
func (a *App) CreateMergeRequests(name, title string) (RequestResults, error) {
	root, err := a.requireRoot()
	if err != nil {
		return RequestResults{}, err
	}
	cfg, err := config.Load(root)
	if err != nil {
		return RequestResults{}, err
	}
	meta, err := feature.Load(root, name)
	if err != nil {
		return RequestResults{}, err
	}
	res := RequestResults{Feature: name, Items: []RequestResult{}}
	err = a.runTask("Create merge requests: "+name, func() error {
		for _, rm := range meta.Repositories {
			output.Printf("[%s] creating request...\n", rm.Name)
			rc, ok := findRepo(cfg, rm.Name)
			item := RequestResult{Repo: rm.Name, Branch: rm.Branch}
			if !ok {
				item.Method = "skipped"
				item.Error = "repository not found in config"
				res.Items = append(res.Items, item)
				continue
			}
			kind := forge.Detect(rc.URL)
			item.Provider = string(kind)
			target := rc.DefaultBranch
			if target == "" {
				target = cfg.Git.DefaultBaseBranch
			}
			ttl := title
			if ttl == "" {
				ttl = fmt.Sprintf("[%s] %s", name, rm.Name)
			}
			item.Target = target

			if kind == forge.Unknown {
				item.Method = "skipped"
				item.Error = "unknown provider"
				res.Items = append(res.Items, item)
				continue
			}

			tokenEnv, apiBase := providerSettings(cfg, kind)
			webURL, _ := forge.NewRequestURL(rc.URL, rm.Branch, target, ttl)
			token := os.Getenv(tokenEnv)
			if token != "" {
				created, cerr := forge.Create(kind, forge.CreateOptions{
					RepoURL: rc.URL, Source: rm.Branch, Target: target,
					Title: ttl, Token: token, APIBaseURL: apiBase,
				})
				if cerr != nil {
					item.Method = "browser"
					item.URL = webURL
					item.Error = cerr.Error()
				} else {
					item.Method = "api"
					item.URL = created.URL
				}
			} else {
				item.Method = "browser"
				item.URL = webURL
			}
			res.Items = append(res.Items, item)
		}
		return nil
	})
	return res, err
}

// OpenURL opens a URL in the user's default browser.
func (a *App) OpenURL(url string) error {
	if url == "" {
		return fmt.Errorf("empty url")
	}
	runtime.BrowserOpenURL(a.ctx, url)
	return nil
}

// CopyText copies arbitrary text to the system clipboard.
func (a *App) CopyText(text string) error {
	return runtime.ClipboardSetText(a.ctx, text)
}

// findRepo returns the config entry for a repo by name.
func findRepo(cfg config.Config, name string) (config.Repository, bool) {
	for _, r := range cfg.Repositories {
		if r.Name == name {
			return r, true
		}
	}
	return config.Repository{}, false
}

// providerSettings resolves the env-var name and API base URL for a provider,
// falling back to defaults so configs missing a github/gitlab section still work.
func providerSettings(cfg config.Config, kind forge.Kind) (tokenEnv, apiBase string) {
	switch kind {
	case forge.GitHub:
		tokenEnv = cfg.GitHub.TokenEnv
		if tokenEnv == "" {
			tokenEnv = "GITHUB_TOKEN"
		}
		apiBase = "" // derived from host inside forge
	case forge.GitLab:
		tokenEnv = cfg.GitLab.TokenEnv
		if tokenEnv == "" {
			tokenEnv = "GITLAB_TOKEN"
		}
		apiBase = cfg.GitLab.BaseURL
	}
	return tokenEnv, apiBase
}

// ---- Git settings ----

// SaveGitSettings persists Git/GitLab/GitHub configuration for the open
// workspace. Base URLs are trimmed of trailing slashes; empty provider fields
// fall back to defaults so a section is never left blank.
func (a *App) SaveGitSettings(git config.GitConfig, gitlab config.GitLabConfig, github config.GitHubConfig) error {
	root, err := a.requireRoot()
	if err != nil {
		return err
	}
	cfg, err := config.Load(root)
	if err != nil {
		return err
	}
	gitlab.BaseURL = strings.TrimRight(strings.TrimSpace(gitlab.BaseURL), "/")
	if github.TokenEnv == "" {
		github.TokenEnv = "GITHUB_TOKEN"
	}
	if gitlab.TokenEnv == "" {
		gitlab.TokenEnv = "GITLAB_TOKEN"
	}
	cfg.Git = git
	cfg.GitLab = gitlab
	cfg.GitHub = github
	return config.Save(root, cfg)
}

// RepoDiag is a per-repository diagnostic entry.
type RepoDiag struct {
	Name         string   `json:"name"`
	URL          string   `json:"url"`
	Provider     string   `json:"provider"`
	TokenEnvName string   `json:"tokenEnvName"`
	TokenPresent bool     `json:"tokenPresent"`
	Issues       []string `json:"issues"`
}

// GitDiag is the result of DiagnoseGit: global issues plus per-repo findings.
type GitDiag struct {
	Issues []string   `json:"issues"`
	Repos  []RepoDiag `json:"repos"`
}

// DiagnoseGit runs fast (no network) checks for common Git configuration
// mistakes: malformed base URLs, missing branch-prefix slash, unknown providers,
// and missing env tokens.
func (a *App) DiagnoseGit() (GitDiag, error) {
	root, err := a.requireRoot()
	if err != nil {
		return GitDiag{}, err
	}
	cfg, err := config.Load(root)
	if err != nil {
		return GitDiag{}, err
	}
	diag := GitDiag{Issues: []string{}, Repos: []RepoDiag{}}

	if cfg.Git.BranchPrefix != "" && !strings.HasSuffix(cfg.Git.BranchPrefix, "/") {
		diag.Issues = append(diag.Issues, "git.branchPrefix should end with '/'")
	}
	checkBase := func(label, base string) {
		if base == "" {
			return
		}
		if !strings.HasPrefix(base, "http://") && !strings.HasPrefix(base, "https://") {
			diag.Issues = append(diag.Issues, label+" base URL should start with https://")
		}
		if strings.HasSuffix(base, "/") {
			diag.Issues = append(diag.Issues, label+" base URL should not end with '/'")
		}
	}
	checkBase("GitLab", cfg.GitLab.BaseURL)

	for _, r := range cfg.Repositories {
		kind := forge.Detect(r.URL)
		rd := RepoDiag{Name: r.Name, URL: r.URL, Provider: string(kind), Issues: []string{}}
		if kind == forge.Unknown {
			rd.Issues = append(rd.Issues, "unknown provider (not github/gitlab)")
		} else {
			tokenEnv, _ := providerSettings(cfg, kind)
			rd.TokenEnvName = tokenEnv
			rd.TokenPresent = os.Getenv(tokenEnv) != ""
			if !rd.TokenPresent {
				rd.Issues = append(rd.Issues, "env "+tokenEnv+" not set (will use browser)")
			}
		}
		if r.DefaultBranch == "" {
			rd.Issues = append(rd.Issues, "no default branch (falls back to git.defaultBaseBranch)")
		}
		diag.Repos = append(diag.Repos, rd)
	}
	return diag, nil
}

// ---- Editor ----

// FeaturePathRepo contains the resolved paths for one repository in a feature.
type FeaturePathRepo struct {
	Name         string `json:"name"`
	WorktreePath string `json:"worktreePath"`
	AgentPath    string `json:"agentPath"`
}

// FeaturePathsResult contains the resolved feature-level and per-repository
// worktree/agent paths for display and copy actions in the desktop UI.
type FeaturePathsResult struct {
	Feature      string            `json:"feature"`
	WorktreePath string            `json:"worktreePath"`
	AgentPath    string            `json:"agentPath"`
	Repositories []FeaturePathRepo `json:"repositories"`
}

// FeaturePaths resolves a feature's worktree and agent paths without opening
// anything. The paths are absolute OS-native filesystem paths.
func (a *App) FeaturePaths(name string) (FeaturePathsResult, error) {
	root, err := a.requireRoot()
	if err != nil {
		return FeaturePathsResult{}, err
	}
	fm, err := feature.Load(root, name)
	if err != nil {
		return FeaturePathsResult{}, err
	}
	key := fm.FolderKey()
	out := FeaturePathsResult{
		Feature:      fm.Name,
		WorktreePath: filepath.Join(root, "feature", key),
		AgentPath:    filepath.Join(root, "agent", key),
		Repositories: []FeaturePathRepo{},
	}
	for _, r := range fm.Repositories {
		out.Repositories = append(out.Repositories, FeaturePathRepo{
			Name:         r.Name,
			WorktreePath: filepath.Join(root, filepath.FromSlash(r.WorktreePath)),
			AgentPath:    config.AgentPath(root, key, r.Name),
		})
	}
	return out, nil
}

func desktopWorktreeTemplateRepos(root string, repos []feature.RepoMeta) []wttemplate.Repo {
	out := make([]wttemplate.Repo, 0, len(repos))
	for _, r := range repos {
		out = append(out, wttemplate.Repo{
			Name:         r.Name,
			WorktreePath: filepath.Join(root, filepath.FromSlash(r.WorktreePath)),
		})
	}
	return out
}

func desktopAgentTemplateRepos(root, featureKey string, repos []feature.RepoMeta) []wttemplate.Repo {
	out := make([]wttemplate.Repo, 0, len(repos))
	for _, r := range repos {
		out = append(out, wttemplate.Repo{
			Name:         r.Name,
			WorktreePath: config.AgentPath(root, featureKey, r.Name),
		})
	}
	return out
}

// OpenWorkspaceFolder opens the current workspace root in the system file
// manager.
func (a *App) OpenWorkspaceFolder() (string, error) {
	root, err := a.requireRoot()
	if err != nil {
		return "", err
	}
	if err := revealInFileManager(root); err != nil {
		return "", err
	}
	return root, nil
}

// revealInFileManager opens dir in the OS file manager (Finder/Explorer/xdg).
func revealInFileManager(dir string) error {
	var cmd *exec.Cmd
	switch goruntime.GOOS {
	case "darwin":
		cmd = exec.Command("open", dir)
	case "windows":
		cmd = exec.Command("explorer.exe", dir)
	default:
		cmd = exec.Command("xdg-open", dir)
	}
	return cmd.Start()
}

func openOSPath(path string) error {
	var cmd *exec.Cmd
	switch goruntime.GOOS {
	case "darwin":
		cmd = exec.Command("open", path)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", path)
	default:
		cmd = exec.Command("xdg-open", path)
	}
	return cmd.Start()
}

func workspacePath(root, path string) (string, error) {
	target := root
	if strings.TrimSpace(path) != "" {
		if filepath.IsAbs(path) {
			target = path
		} else {
			target = filepath.Join(root, path)
		}
	}
	target, err := filepath.Abs(target)
	if err != nil {
		return "", err
	}
	if err := fsutil.EnsureInside(root, target); err != nil {
		return "", err
	}
	return target, nil
}

type workspaceTreeLabel struct {
	FeatureName string
	Branch      string
}

func workspaceTreeLabels(root string) map[string]workspaceTreeLabel {
	labels := map[string]workspaceTreeLabel{}
	list, err := feature.ListData(root)
	if err != nil {
		return labels
	}
	for _, item := range list.Features {
		meta, err := feature.Load(root, item.Name)
		if err != nil {
			continue
		}
		labels[filepath.ToSlash(filepath.Join("feature", meta.FolderKey()))] = workspaceTreeLabel{
			FeatureName: meta.Name,
			Branch:      meta.Branch,
		}
		labels[filepath.ToSlash(filepath.Join("agent", meta.FolderKey()))] = workspaceTreeLabel{
			FeatureName: meta.Name,
			Branch:      meta.Branch,
		}
	}
	return labels
}

func applyWorkspaceTreeLabel(node *WorkspaceTreeNode, labels map[string]workspaceTreeLabel) {
	if label, ok := labels[node.RelPath]; ok {
		node.FeatureName = label.FeatureName
		node.Branch = label.Branch
	}
}

func treeNode(root, target string, labels map[string]workspaceTreeLabel) (WorkspaceTreeNode, error) {
	info, err := os.Stat(target)
	if err != nil {
		return WorkspaceTreeNode{}, err
	}
	rel, _ := filepath.Rel(root, target)
	if rel == "." {
		rel = ""
	}
	node := WorkspaceTreeNode{
		Name:    filepath.Base(target),
		Path:    target,
		RelPath: filepath.ToSlash(rel),
		IsDir:   info.IsDir(),
		Size:    info.Size(),
		ModTime: info.ModTime().Format(time.RFC3339),
	}
	applyWorkspaceTreeLabel(&node, labels)
	if !info.IsDir() {
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
	node.Children = []WorkspaceTreeNode{}
	for _, entry := range entries {
		childPath := filepath.Join(target, entry.Name())
		childInfo, err := entry.Info()
		if err != nil {
			continue
		}
		childRel, _ := filepath.Rel(root, childPath)
		child := WorkspaceTreeNode{
			Name:    entry.Name(),
			Path:    childPath,
			RelPath: filepath.ToSlash(childRel),
			IsDir:   entry.IsDir(),
			Size:    childInfo.Size(),
			ModTime: childInfo.ModTime().Format(time.RFC3339),
		}
		applyWorkspaceTreeLabel(&child, labels)
		node.Children = append(node.Children, child)
	}
	return node, nil
}

func ensureExplorerDeleteAllowed(root, target string) error {
	if sameAbsPath(root, target) {
		return fmt.Errorf("cannot delete the workspace root")
	}
	protected := []string{
		filepath.Join(root, config.DirName),
		filepath.Join(root, config.DirName, config.ConfigFileName),
		filepath.Join(root, config.DirName, "features"),
		filepath.Join(root, config.DirName, "sessions"),
		filepath.Join(root, config.DirName, "history"),
	}
	for _, p := range protected {
		if sameAbsPath(target, p) || pathContains(target, p) || pathContains(p, target) {
			return fmt.Errorf("protected agentsafe path cannot be deleted: %s", target)
		}
	}
	return nil
}

func pathContains(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

func sameAbsPath(a, b string) bool {
	aa, errA := filepath.Abs(filepath.Clean(a))
	bb, errB := filepath.Abs(filepath.Clean(b))
	if errA != nil || errB != nil {
		return false
	}
	if realA, err := filepath.EvalSymlinks(aa); err == nil {
		aa = realA
	}
	if realB, err := filepath.EvalSymlinks(bb); err == nil {
		bb = realB
	}
	if goruntime.GOOS == "windows" {
		return strings.EqualFold(aa, bb)
	}
	return aa == bb
}

func safeFilename(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-.")
	if out == "" {
		return "workspace"
	}
	return out
}

// launchProgram opens path with program. A program picked via SelectProgram is
// an absolute path (a .app bundle on macOS); launch it with `open -a`. A bare
// command (e.g. "code") is run directly from PATH.
func launchProgram(path, program string) error {
	var e *exec.Cmd
	if goruntime.GOOS == "darwin" && (strings.HasSuffix(program, ".app") || strings.Contains(program, "/")) {
		e = exec.Command("open", "-a", program, path)
	} else {
		e = exec.Command(program, path)
	}
	e.Stdout, e.Stderr, e.Stdin = os.Stdout, os.Stderr, os.Stdin
	return e.Start()
}

// featureRepoWorktree resolves the on-disk worktree path for one repository of
// a feature (feature/<key>/<repo>), using the stored relative WorktreePath.
func featureRepoWorktree(root, name, repo string) (string, error) {
	fm, err := feature.Load(root, name)
	if err != nil {
		return "", err
	}
	for _, r := range fm.Repositories {
		if r.Name == repo {
			return filepath.Join(root, filepath.FromSlash(r.WorktreePath)), nil
		}
	}
	return "", fmt.Errorf("repository %q is not part of feature %q", repo, name)
}

// OpenFeatureFolder reveals a feature's worktree root (feature/<key>) in the
// OS file manager.
func (a *App) OpenFeatureFolder(name string) (string, error) {
	root, err := a.requireRoot()
	if err != nil {
		return "", err
	}
	fm, err := feature.Load(root, name)
	if err != nil {
		return "", err
	}
	p := filepath.Join(root, "feature", fm.FolderKey())
	if err := revealInFileManager(p); err != nil {
		return "", err
	}
	return p, nil
}

// OpenRepoFolder reveals one repository's worktree (feature/<key>/<repo>) in
// the OS file manager.
func (a *App) OpenRepoFolder(name, repo string) (string, error) {
	root, err := a.requireRoot()
	if err != nil {
		return "", err
	}
	p, err := featureRepoWorktree(root, name, repo)
	if err != nil {
		return "", err
	}
	if err := revealInFileManager(p); err != nil {
		return "", err
	}
	return p, nil
}

// OpenRepoInProgram opens one repository's worktree in the given program. When
// program is empty it returns the path only.
func (a *App) OpenRepoInProgram(name, repo, program string) (string, error) {
	root, err := a.requireRoot()
	if err != nil {
		return "", err
	}
	p, err := featureRepoWorktree(root, name, repo)
	if err != nil {
		return "", err
	}
	if program == "" {
		return p, nil
	}
	if err := launchProgram(p, program); err != nil {
		return "", err
	}
	return p, nil
}

// OpenWorkspaceTerminal opens the current workspace root in the system
// terminal.
func (a *App) OpenWorkspaceTerminal() (string, error) {
	root, err := a.requireRoot()
	if err != nil {
		return "", err
	}
	return openTerminalAt(root)
}

// OpenWorkspaceVSCode opens the current workspace root in VSCode.
func (a *App) OpenWorkspaceVSCode() (string, error) {
	root, err := a.requireRoot()
	if err != nil {
		return "", err
	}
	cmd := exec.Command("code", root)
	if err := cmd.Start(); err != nil {
		return "", err
	}
	return root, nil
}

// OpenInEditor opens the agent workspace for a feature in the given editor
// (e.g. "code" or "cursor"). When editor is empty it returns the path only.
func (a *App) OpenInEditor(name, editor string) (string, error) {
	root, err := a.requireRoot()
	if err != nil {
		return "", err
	}
	fm, err := feature.Load(root, name)
	if err != nil {
		return "", err
	}
	p := filepath.Join(root, "agent", fm.FolderKey())
	if editor == "" {
		return p, nil
	}
	if err := launchProgram(p, editor); err != nil {
		return "", err
	}
	return p, nil
}

// OpenInTerminal opens the agent workspace for a feature in the system terminal.
func (a *App) OpenInTerminal(name string) (string, error) {
	root, err := a.requireRoot()
	if err != nil {
		return "", err
	}
	fm, err := feature.Load(root, name)
	if err != nil {
		return "", err
	}
	return openTerminalAt(filepath.Join(root, "agent", fm.FolderKey()))
}

// OpenAgentCommandTerminal opens the feature's agent workspace in a configured
// terminal and runs command after changing to the agent directory. The
// terminalProgram value is a UI preference such as powershell, pwsh, cmd,
// git-bash, wt, or default.
func (a *App) OpenAgentCommandTerminal(name, terminalProgram, command string) (string, error) {
	root, err := a.requireRoot()
	if err != nil {
		return "", err
	}
	fm, err := feature.Load(root, name)
	if err != nil {
		return "", err
	}
	dir := filepath.Join(root, "agent", fm.FolderKey())
	return openTerminalCommandAt(dir, terminalProgram, command)
}

// AgentRun launches the given command in an embedded pty terminal rooted at the
// feature's agent workspace. The session is tagged with the feature so its exit
// emits "agent:exit", letting the UI refresh the diff and prompt a sync.
func (a *App) AgentRun(name, command string) (TerminalSession, error) {
	root, err := a.requireRoot()
	if err != nil {
		return TerminalSession{}, err
	}
	if strings.TrimSpace(command) == "" {
		return TerminalSession{}, fmt.Errorf("agent command is required")
	}
	fm, err := feature.Load(root, name)
	if err != nil {
		return TerminalSession{}, err
	}
	dir := filepath.Join(root, "agent", fm.FolderKey())
	shell, args := commandShell(command)
	session, err := a.startPTY(dir, name, shell, args)
	if err != nil && isPTYUnsupported(err) {
		if _, openErr := openTerminalCommandAt(dir, "default", command); openErr != nil {
			return TerminalSession{}, openErr
		}
		return TerminalSession{ID: "external", Path: dir, Title: filepath.Base(dir), External: true}, nil
	}
	return session, err
}

func openTerminalAt(dir string) (string, error) {
	var cmd *exec.Cmd
	switch goruntime.GOOS {
	case "darwin":
		cmd = exec.Command("open", "-a", "Terminal", dir)
	case "windows":
		cmd = windowsStart("/D", dir, "cmd", "/K")
	default:
		cmd = exec.Command("x-terminal-emulator", "--working-directory="+dir)
	}
	if err := cmd.Start(); err != nil {
		return "", err
	}
	return dir, nil
}

func openTerminalCommandAt(dir, terminalProgram, command string) (string, error) {
	terminalProgram = strings.TrimSpace(terminalProgram)
	command = strings.TrimSpace(command)
	if terminalProgram == "" || terminalProgram == "default" || command == "" {
		return openTerminalAt(dir)
	}

	var cmd *exec.Cmd
	switch goruntime.GOOS {
	case "windows":
		cmd = windowsTerminalCommand(dir, terminalProgram, command)
	case "darwin":
		if terminalProgram == "Terminal" || terminalProgram == "terminal" {
			cmd = exec.Command(
				"osascript",
				"-e",
				fmt.Sprintf(`tell application "Terminal" to do script "cd %s; %s"`, shellQuote(dir), escapeAppleScript(command)),
			)
		} else {
			cmd = exec.Command("open", "-a", terminalProgram, dir)
		}
	default:
		if terminalProgram == "x-terminal-emulator" || terminalProgram == "terminal" {
			cmd = exec.Command("x-terminal-emulator", "--working-directory="+dir, "-e", "sh", "-lc", command+"; exec sh")
		} else {
			cmd = exec.Command(terminalProgram, dir)
		}
	}
	if cmd == nil {
		return openTerminalAt(dir)
	}
	if err := cmd.Start(); err != nil {
		return openTerminalAt(dir)
	}
	return dir, nil
}

func windowsTerminalCommand(dir, terminalProgram, command string) *exec.Cmd {
	psCmd := fmt.Sprintf("Set-Location -LiteralPath %s; %s", psQuote(dir), command)
	switch strings.ToLower(terminalProgram) {
	case "powershell":
		return windowsStart("powershell", "-NoExit", "-Command", psCmd)
	case "pwsh":
		return windowsStart("pwsh", "-NoExit", "-Command", psCmd)
	case "cmd":
		return windowsStart("cmd", "/K", fmt.Sprintf("cd /d %s && %s", cmdQuote(dir), command))
	case "wt", "windows-terminal":
		return exec.Command("wt", "-d", dir, "powershell", "-NoExit", "-Command", command)
	case "git-bash":
		bash := `C:\Program Files\Git\git-bash.exe`
		if _, err := os.Stat(bash); err != nil {
			bash = `C:\Program Files (x86)\Git\git-bash.exe`
		}
		return windowsStart(bash, "--cd="+dir, "-c", command+"; exec bash")
	default:
		return windowsStart(terminalProgram, dir)
	}
}

func windowsStart(args ...string) *exec.Cmd {
	startArgs := append([]string{"/c", "start", ""}, args...)
	return exec.Command("cmd", startArgs...)
}

func psQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

func cmdQuote(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

func escapeAppleScript(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	return strings.ReplaceAll(s, `"`, `\"`)
}
