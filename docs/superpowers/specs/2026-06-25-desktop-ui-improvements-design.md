# Desktop UI Improvements — Design

Date: 2026-06-25
Branch base: `perf/status-load-parallel`
Scope: `apps/desktop/frontend/src/**` plus one new desktop Go binding in `apps/desktop/app.go`.

## Summary

Eight UI fixes/enhancements for the Wails desktop app, plus one new shared
component (`ToolOpenMenu`, "툴 열기"). All real logic stays in the React
frontend except a single new GUI-only Go binding (`OpenPathInProgram`). No CLI
parity is required because these are GUI-only "open in editor / reveal folder"
conveniences, consistent with existing desktop-only helpers
(`OpenInEditor`, `OpenPathVSCode`, `OpenWorkspaceVSCode`).

## Decisions captured from brainstorming

- **Nav toggle (item 1):** Move the cycle toggle to the header top-left; remove
  the bottom-of-sidebar toggle and the hidden-state edge reveal button (single
  source of truth).
- **Workspace terminal (item 2):** Open the in-app terminal as a **new app tab**
  in the main tab/pane system (not an inline panel).
- **ToolOpenMenu reuse (item 3 question):** Use the new component in the status
  tab (required) **and** the Workspace page and the File Explorer page.
- **Default tool (item 8):** A **preset dropdown** (VS Code, Cursor, …) **plus a
  "직접 선택" option** that opens a native program picker for when the CLI
  command is not on PATH. Stored in `localStorage["agentsafe.program"]`
  (default `code`), shared with the existing feature-page program logic.
- **7.2 agent button row:** Drop "에이전트 템플릿 적용" from the agent area's
  button row (it remains available in the feature Settings tab) to match the
  4-button spec (재생성 / 새로고침 / 삭제 / 툴 열기).

---

## New component: `ToolOpenMenu`

File: `apps/desktop/frontend/src/components/ToolOpenMenu.tsx`

A presentational dropdown trigger:

- Renders an external-open icon button (`ExternalLink`).
- Clicking toggles a dropdown with three items: **폴더 / 터미널 / `<tool>`**.
  The third item's label is the configured default tool (e.g. "VS Code").
- Props: `onFolder(): void`, `onTerminal(): void`, `onTool(): void`,
  `disabled?: boolean`, `align?: "start" | "end"`, optional `size`/`variant` to
  match button styling.
- The component is pure: hosts wire each action to the correct context. It does
  not call backend bindings itself except via the host callbacks.
- Closes on outside click / Escape.

### `useDefaultTool` hook

File: `apps/desktop/frontend/src/lib/tool.ts`.

- Reads `localStorage["agentsafe.program"]` (default `code`).
- Maps known values to labels via a preset table
  (`code` → "VS Code", `cursor` → "Cursor", `subl` → "Sublime Text",
  `idea` → "IntelliJ IDEA", `webstorm` → "WebStorm"); arbitrary executable
  paths fall back to the basename (minus `.app`/`.exe`).
- Subscribes to a `window` event `agentsafe:tool-changed` so Settings changes
  propagate live to every open `ToolOpenMenu`.
- Returns `{ value, label }`.

### Backend binding

`apps/desktop/app.go`:

```go
// OpenPathInProgram opens an absolute (in-workspace) path in the given program.
func (a *App) OpenPathInProgram(path, program string) (string, error)
```

- Resolves/validates `path` with the existing `workspacePath(root, path)`
  (accepts absolute paths inside the workspace root) and launches via the
  existing `launchProgram(target, program)`. Returns the resolved path.
- Regenerate Wails TS bindings (`make build-desktop` / wails dev codegen) so
  `api.OpenPathInProgram` is available.

---

## Item 1 — Header nav-collapse toggle

File: `App.tsx`.

- Add a toggle button to the header (`<header>`), left-aligned, that calls the
  existing `nextSidebarMode()` (cycles `full → icons → hidden → full`).
- Header layout changes from `justify-end` to `justify-between`: left = nav
  toggle, right = existing `LogConsoleButton` + `ThemeToggle`.
- The button icon reflects the next mode and uses the existing
  `sidebarToggleLabel` for title/aria-label.
- Remove the bottom-of-sidebar toggle button block and the hidden-state edge
  reveal button block (the two `<button onClick={nextSidebarMode}>` regions and
  the `{sidebarHidden && (...)}` fixed reveal control).
- Sidebar still renders its modes (full/icons/hidden) exactly as today; only the
  control that switches them moves.

## Item 2 — Workspace page terminal as a new app tab

Files: `App.tsx`, `WorkspacePage.tsx`.

- Extend the `View` union with `{ kind: "terminal"; id: string; path: string;
  title: string }`.
- `viewId` for a terminal view returns `terminal:${id}` (unique per session).
- `titleForView` returns the terminal title; `iconForView` returns a terminal
  icon.
- `renderView` renders `<TerminalPanel id={view.id} path={view.path} />` for the
  terminal kind, height-fit per item 4.
- Add `openTerminalTab(session: TerminalSession)` in `App` that registers the
  view + opens it as a tab via the existing `openView`/tab plumbing (creating
  the `AppTab` entry then activating it).
- On terminal-tab close, close the pty: in `closeTabs`, when a removed tab is a
  terminal view, call `api.TerminalClose(id)` (best-effort).
- `WorkspacePage` receives an `onOpenTerminal(session)` prop. Its terminal
  action: `const s = await api.TerminalOpen(root); onOpenTerminal(s);` (falls
  back to a toast if `s.external`).
- Replace the Workspace header's three buttons (Folder / Terminal / VS Code)
  with a single `ToolOpenMenu`:
  - `onFolder` → `api.OpenWorkspaceFolder()`
  - `onTerminal` → open new app tab (above)
  - `onTool` → `api.OpenPathInProgram("", tool)` (empty path = workspace root)
    — replaces the hardcoded `OpenWorkspaceVSCode`.

## Item 3 — Clicking an open menu switches to its tab (no reorder)

File: `App.tsx`, `openView`.

- Current bug: `openView` always calls `moveTabToPane`, which re-inserts the tab
  and reorders/moves it to the active pane even when it already exists.
- Fix: compute `existingPaneId = paneContainingTab(layout, id)`. If found, only
  set that pane active and set its `activeTabId = id` (no `moveTabToPane`, no
  reinsert). If not found, keep the current create-and-place behavior.
- Preserves drag-to-reorder/split (those paths are untouched).

## Item 4 — Embedded terminals fit the pane height (no scroll)

Files: `TerminalPanel.tsx`, `FileExplorerPage.tsx`, `FeatureDetailPage.tsx`,
terminal app-tab rendering in `App.tsx`.

- The pane content wrapper in `renderPane` is already
  `min-h-0 flex-1 overflow-auto p-6`; the problem is pages/terminals using
  `h-[calc(100vh-…)]` viewport math that doesn't match actual pane height
  (panes can be split), causing overflow → page scroll.
- `TerminalPanel`: default container becomes `h-full` (flex column) instead of
  `h-[calc(100vh-12rem)]`, so it fills whatever height its parent gives.
- File Explorer: the terminal/editor content region fills the card height via
  flex/`h-full` rather than `h-[calc(100vh-12rem)]`; the page grid keeps a
  bounded height so the terminal never pushes the page taller than the pane.
- Feature detail terminal tab: wrap `TerminalPanel` in a height-fit container
  (`h-full` within a bounded parent) so the terminal tab occupies the pane
  height without causing the outer page to scroll.
- Workspace terminal app-tab: same height-fit treatment.
- Scope per request: worktree detail page + file explorer (workspace terminal
  tab follows the same rule for consistency).

## Item 5 — Template drag-and-drop upload

File: `WorktreeTemplatesPage.tsx` (and verify `main.go` DnD options).

- Root cause: the HTML5 `onDrop` reads `file.path`, which is always empty in the
  Wails WebView, so nothing uploads. The reliable mechanism is the already-wired
  `runtime.OnFileDrop` → `workspace:file-drop` event (carries real OS paths).
- Fix:
  - Drive uploads solely from the `workspace:file-drop` subscription.
  - Make hit-testing robust: when the templates page is active and a non-root
    folder is selected, route the dropped paths to `importPaths(...)` for the
    selected folder. Accept a drop landing anywhere on the dropzone card (use
    `dropRef` containment, but tolerate the coordinate/`elementFromPoint`
    fallback returning a child).
  - Keep the visual dropzone; its HTML5 handlers only `preventDefault` for drag
    affordance and no longer attempt `file.path` extraction.
  - Verify `options.DragAndDrop` (`EnableFileDrop: true` is set) and, if needed,
    set `CSSDropProperty`/`CSSDropValue` to match the `--wails-drop-target`
    style already on `dropRef`.
- Confirm exact failure via systematic-debugging before finalizing.

## Item 6 — Template detail folder collapse

File: `WorktreeTemplatesPage.tsx`, `TemplateFileNode`.

- Reported: folders in the template detail file list cannot be collapsed (stay
  expanded).
- Debug the `open` derivation (`expandedTemplateNodes.has(key) || isRoot`) and
  the toggle handlers; ensure nested directories toggle and persist a collapsed
  state. Likely contributors to verify: `isRoot`/`effectiveRoot` single-child
  collapsing, key stability (`nodeKey`), and that `load()` only seeds the root
  key (not nested folders).
- Use systematic-debugging; deliver a minimal fix that makes folder rows
  reliably expand/collapse.

## Item 7 — Status tab redesign

File: `FeatureDetailPage.tsx`, the `tab === "status"` block.

### 7.1 Worktree card

- Title row: "워크트리" + branch as an inline `Badge` (from `status?.branch`).
  The feature-name badge stays on the right; the branch becomes a tag beside the
  title.
- New worktree-path row: the path (`featurePaths.worktreePath`) with **icon-only**
  buttons on the right:
  - copy → `copyPath(featurePaths.worktreePath)`
  - folder → `openFeatureFolder()`
- Button area: **base-branch rebase** (`rebase`) / **refresh** (`loadStatus`) /
  **`ToolOpenMenu`**:
  - `onFolder` → `openFeatureFolder()`
  - `onTerminal` → open a feature terminal tab at the worktree path
    (`api.TerminalOpenWithProgram(worktreePath, terminalProgram)` →
    `addTerminalTab(...)`)
  - `onTool` → `api.OpenPathInProgram(worktreePath, tool)`

### 7.2 Agent card

- Show the agent-space root path (`featurePaths.agentPath`) with **icon-only**
  copy + folder buttons on the right:
  - copy → `copyPath(featurePaths.agentPath)`
  - folder → `api.OpenPath(featurePaths.agentPath)` (absolute in-workspace path;
    opens the agent root in the OS file manager — no new binding needed)
- Button area: **재생성 (regenerate, localized)** / **새로고침** / **삭제** /
  **`ToolOpenMenu`**:
  - regenerate → existing `prepare` (label localized: "생성" when missing,
    "재생성" when ready; remove English "Create"/"Regenerate")
  - refresh → `loadDiff(true)` (or `loadStatus`, matching current refresh)
  - delete → existing `del`
  - `ToolOpenMenu`: `onFolder` → `api.OpenPath(agentPath)`; `onTerminal` →
    existing `openTerminal` (agent path); `onTool` →
    `api.OpenPathInProgram(agentPath, tool)`
- The current open `<select>` + 열기 button is removed (replaced by
  `ToolOpenMenu`). "에이전트 템플릿 적용" is removed from this row (stays in the
  Settings tab).

## Item 8 — Default tool setting

File: `SettingsPage.tsx`.

- New card "기본 툴" (default tool):
  - A `<select>` of presets: VS Code (`code`), Cursor (`cursor`),
    Sublime Text (`subl`), IntelliJ IDEA (`idea`), WebStorm (`webstorm`), plus a
    "직접 선택…" option.
  - Choosing a preset writes `localStorage["agentsafe.program"]` and dispatches
    `window.dispatchEvent(new Event("agentsafe:tool-changed"))`.
  - Choosing "직접 선택…" calls `api.SelectProgram()`; on a non-empty result,
    store the returned path the same way (so an off-PATH editor still works).
  - The select reflects the current value: a preset when it matches, otherwise a
    synthesized "직접 선택" entry showing the chosen program's basename.
  - Default `code` when unset.
- This value is the single source for every `ToolOpenMenu`'s `<tool>` item and is
  already consumed by the feature page's `program` logic.

---

## i18n

Add Korean/English keys (in `i18n/translations.ts`) for new labels:
`toolOpen.folder`, `toolOpen.terminal`, `toolOpen.openTitle`,
`settings.defaultToolTitle`, `settings.defaultToolDesc`, `settings.toolPick`,
`feature.regenerate` / `feature.create` (localized), and any new status-tab
labels. Reuse existing keys where present (`feature.copyPath`,
`feature.openWorktreeFolder`, `feature.rebase`, `common.refresh`,
`common.delete`).

## Out of scope / non-goals

- No changes to core pipeline packages (`internal/*`) beyond none required.
- No CLI command additions (GUI-only open helpers).
- No change to terminal pty lifecycle except wiring close for the new
  workspace terminal app-tab.

## Testing / verification

- `pnpm build` (`tsc && vite build`) in `apps/desktop/frontend` must pass
  (type-checks the new component, props, and view union).
- `go build ./apps/desktop` (or `make build-desktop`) for the new binding.
- Manual verification of each item in the running app (nav toggle cycling,
  workspace terminal tab open/close, re-click menu does not reorder, terminal
  height with split panes, template drop upload into selected folder, folder
  collapse, status-tab layout + tool menu actions, default-tool dropdown +
  custom pick reflected in tool menus).
