# Desktop UI Improvements Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement eight desktop UI fixes/enhancements plus a shared `ToolOpenMenu` ("툴 열기") component in the Wails desktop app.

**Architecture:** All logic lives in the React frontend (`apps/desktop/frontend/src`) except a single new GUI-only Go binding (`OpenPathInProgram`) in `apps/desktop/app.go`. A new presentational `ToolOpenMenu` dropdown (folder/terminal/tool) is consumed by the Workspace page, File Explorer, and the worktree-detail Status tab; its tool label comes from a shared `useDefaultTool` hook backed by `localStorage["agentsafe.program"]`.

**Tech Stack:** React 18 + TypeScript, Tailwind, lucide-react, xterm.js, Wails v2 (Go bindings), Go 1.x.

## Global Constraints

- **Verification model (no frontend test runner exists):** the frontend has only `tsc && vite build` (no vitest/jest). Each frontend task's automated gate is `cd apps/desktop/frontend && pnpm exec tsc --noEmit` (must pass with zero errors) plus the explicit manual checks listed. The Go task's gate is `go build ./apps/desktop && go vet ./apps/desktop`. Do not add a new test framework (YAGNI; not requested).
- **TypeScript strictness:** unused imports/locals fail the type-check. When removing UI, also remove now-unused imports and helpers.
- **CLI/GUI parity (AGENTS.md):** parity is required only for new `internal/` capabilities. The one new binding is a GUI-only open helper (like existing `OpenInEditor`/`OpenPathVSCode`/`OpenWorkspaceVSCode`), so **no CLI command** is added.
- **Windows is first-class.** Do not change `.gitattributes` line-ending rules.
- **Default tool key:** `localStorage["agentsafe.program"]`, default `"code"`. Change event name: `"agentsafe:tool-changed"`.
- **Existing reusable bindings** (do not reinvent): `api.OpenWorkspaceFolder()`, `api.OpenFeatureFolder(name)`, `api.OpenPath(path)`, `api.TerminalOpen(path)`, `api.TerminalOpenWithProgram(path, program)`, `api.SelectProgram()`, `api.CopyText(text)`.
- **Commit after every task.** Branch is `perf/status-load-parallel` (already feature branch; commit directly).

---

## File Structure

- `apps/desktop/app.go` — add `OpenPathInProgram` binding (Task 1).
- `apps/desktop/frontend/src/lib/api.ts` — add `OpenPathInProgram` type (Task 1).
- `apps/desktop/frontend/src/lib/tool.ts` — **new**: tool presets + `useDefaultTool` (Task 2).
- `apps/desktop/frontend/src/components/ToolOpenMenu.tsx` — **new** component (Task 3).
- `apps/desktop/frontend/src/i18n/translations.ts` — add keys (Tasks 3,4,9).
- `apps/desktop/frontend/src/pages/SettingsPage.tsx` — default-tool card (Task 4).
- `apps/desktop/frontend/src/App.tsx` — tab de-dup (Task 5), header toggle (Task 6), terminal view (Task 7), terminal-tab height (Task 8).
- `apps/desktop/frontend/src/pages/WorkspacePage.tsx` — ToolOpenMenu + in-app terminal (Task 7).
- `apps/desktop/frontend/src/components/TerminalPanel.tsx` — height fit (Task 8).
- `apps/desktop/frontend/src/pages/FileExplorerPage.tsx` — height fit (Task 8) + ToolOpenMenu (Task 10).
- `apps/desktop/frontend/src/pages/FeatureDetailPage.tsx` — pane-height fill (Task 8) + Status tab redesign (Task 9).
- `apps/desktop/frontend/src/pages/WorktreeTemplatesPage.tsx` — drag-drop fix (Task 11) + folder collapse fix (Task 12).

---

### Task 1: Backend `OpenPathInProgram` binding

**Files:**
- Modify: `apps/desktop/app.go` (add method after `OpenInEditor`, ~line 2774)
- Modify: `apps/desktop/frontend/src/lib/api.ts:75` (add type after `OpenPathVSCode`)

**Interfaces:**
- Produces (Go): `func (a *App) OpenPathInProgram(path, program string) (string, error)`
- Produces (TS): `api.OpenPathInProgram(path: string, program: string): Promise<string>`

- [ ] **Step 1: Add the Go method**

In `apps/desktop/app.go`, immediately after the `OpenInEditor` method, add:

```go
// OpenPathInProgram opens an absolute (in-workspace) path in the given program
// (e.g. "code" or "cursor"). When program is empty it returns the resolved path
// only. GUI-only helper used by the tool-open menu for worktree/agent/workspace
// paths; mirrors OpenInEditor but takes an explicit path instead of a feature.
func (a *App) OpenPathInProgram(path, program string) (string, error) {
	root, err := a.requireRoot()
	if err != nil {
		return "", err
	}
	target, err := workspacePath(root, path)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(program) == "" {
		return target, nil
	}
	if err := launchProgram(target, program); err != nil {
		return "", err
	}
	return target, nil
}
```

- [ ] **Step 2: Add the TS binding type**

In `apps/desktop/frontend/src/lib/api.ts`, after the `OpenPathVSCode(path: string): Promise<string>;` line, add:

```ts
  OpenPathInProgram(path: string, program: string): Promise<string>;
```

- [ ] **Step 3: Build & vet the Go binding**

Run: `cd C:/agentspace/agentsafe && go build ./apps/desktop && go vet ./apps/desktop`
Expected: no output (success).

- [ ] **Step 4: Type-check the frontend**

Run: `cd C:/agentspace/agentsafe/apps/desktop/frontend && pnpm exec tsc --noEmit`
Expected: no errors.

- [ ] **Step 5: Commit**

```bash
cd C:/agentspace/agentsafe
git add apps/desktop/app.go apps/desktop/frontend/src/lib/api.ts
git commit -m "feat(desktop): add OpenPathInProgram binding for tool-open menu"
```

---

### Task 2: `useDefaultTool` hook + tool presets

**Files:**
- Create: `apps/desktop/frontend/src/lib/tool.ts`

**Interfaces:**
- Produces: `TOOL_PRESETS: { value: string; label: string }[]`, `TOOL_PRESET_VALUES: string[]`, `TOOL_CHANGED_EVENT: string`, `getDefaultTool(): string`, `setDefaultTool(value: string): void`, `toolLabel(value: string): string`, `useDefaultTool(): { value: string; label: string }`

- [ ] **Step 1: Create the module**

Create `apps/desktop/frontend/src/lib/tool.ts`:

```ts
// Shared "default tool" state: the editor/program used by the tool-open menu and
// the feature page. Backed by localStorage["agentsafe.program"] (default "code")
// and a window event so every consumer updates live when Settings changes it.
import { useEffect, useState } from "react";

export const TOOL_CHANGED_EVENT = "agentsafe:tool-changed";
const TOOL_KEY = "agentsafe.program";

export const TOOL_PRESETS: { value: string; label: string }[] = [
  { value: "code", label: "VS Code" },
  { value: "cursor", label: "Cursor" },
  { value: "subl", label: "Sublime Text" },
  { value: "idea", label: "IntelliJ IDEA" },
  { value: "webstorm", label: "WebStorm" },
];

export const TOOL_PRESET_VALUES = TOOL_PRESETS.map((p) => p.value);

export function getDefaultTool(): string {
  try {
    return localStorage.getItem(TOOL_KEY) || "code";
  } catch {
    return "code";
  }
}

export function setDefaultTool(value: string): void {
  try {
    localStorage.setItem(TOOL_KEY, value);
  } catch {
    /* localStorage unavailable */
  }
  window.dispatchEvent(new Event(TOOL_CHANGED_EVENT));
}

// toolLabel resolves a stored value to a display label: a preset label, else the
// basename of an executable path (without .app/.exe).
export function toolLabel(value: string): string {
  const preset = TOOL_PRESETS.find((p) => p.value === value);
  if (preset) return preset.label;
  const base = value.split(/[\\/]/).pop() || value;
  return base.replace(/\.(app|exe)$/i, "");
}

export function useDefaultTool(): { value: string; label: string } {
  const [value, setValue] = useState(getDefaultTool);
  useEffect(() => {
    const onChange = () => setValue(getDefaultTool());
    window.addEventListener(TOOL_CHANGED_EVENT, onChange);
    return () => window.removeEventListener(TOOL_CHANGED_EVENT, onChange);
  }, []);
  return { value, label: toolLabel(value) };
}
```

- [ ] **Step 2: Type-check**

Run: `cd C:/agentspace/agentsafe/apps/desktop/frontend && pnpm exec tsc --noEmit`
Expected: no errors.

- [ ] **Step 3: Commit**

```bash
cd C:/agentspace/agentsafe
git add apps/desktop/frontend/src/lib/tool.ts
git commit -m "feat(desktop): add useDefaultTool hook and tool presets"
```

---

### Task 3: `ToolOpenMenu` component + i18n

**Files:**
- Create: `apps/desktop/frontend/src/components/ToolOpenMenu.tsx`
- Modify: `apps/desktop/frontend/src/i18n/translations.ts` (add `toolOpen.*` keys to both `en` and `ko`)

**Interfaces:**
- Consumes: `useDefaultTool` from `@/lib/tool` (Task 2)
- Produces: `ToolOpenMenu` with props `{ onFolder: () => void; onTerminal: () => void; onTool: () => void; disabled?: boolean; align?: "start" | "end" }`

- [ ] **Step 1: Add i18n keys**

In `apps/desktop/frontend/src/i18n/translations.ts`, add to the `en` dict (near the `terminal.*`/`split.*` block):

```ts
  "toolOpen.title": "Open with",
  "toolOpen.folder": "Folder",
  "toolOpen.terminal": "Terminal",
```

Add to the `ko` dict the same keys:

```ts
  "toolOpen.title": "툴 열기",
  "toolOpen.folder": "폴더",
  "toolOpen.terminal": "터미널",
```

- [ ] **Step 2: Create the component**

Create `apps/desktop/frontend/src/components/ToolOpenMenu.tsx`:

```tsx
import { type ReactNode, useEffect, useRef, useState } from "react";
import { ExternalLink, Folder, Terminal as TerminalIcon } from "lucide-react";
import { Button } from "@/components/ui/button";
import { useI18n } from "@/i18n/I18nProvider";
import { useDefaultTool } from "@/lib/tool";
import { cn } from "@/lib/utils";

// ToolOpenMenu renders an external-open icon button that opens a dropdown with
// three actions: folder / terminal / <configured tool>. It is purely
// presentational — each host wires the actions to its own context.
export function ToolOpenMenu({
  onFolder,
  onTerminal,
  onTool,
  disabled,
  align = "end",
}: {
  onFolder: () => void;
  onTerminal: () => void;
  onTool: () => void;
  disabled?: boolean;
  align?: "start" | "end";
}) {
  const { t } = useI18n();
  const { label } = useDefaultTool();
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    if (!open) return;
    const onDown = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setOpen(false);
    };
    window.addEventListener("mousedown", onDown);
    window.addEventListener("keydown", onKey);
    return () => {
      window.removeEventListener("mousedown", onDown);
      window.removeEventListener("keydown", onKey);
    };
  }, [open]);

  const choose = (fn: () => void) => {
    setOpen(false);
    fn();
  };

  return (
    <div ref={ref} className="relative inline-block">
      <Button
        variant="outline"
        size="sm"
        disabled={disabled}
        onClick={() => setOpen((v) => !v)}
        title={t("toolOpen.title")}
        aria-haspopup="menu"
        aria-expanded={open}
      >
        <ExternalLink className="size-4" /> {t("toolOpen.title")}
      </Button>
      {open && (
        <div
          role="menu"
          className={cn(
            "absolute z-50 mt-1 min-w-40 overflow-hidden rounded-md border bg-card text-card-foreground p-1 shadow-md",
            align === "end" ? "right-0" : "left-0"
          )}
        >
          <ToolMenuItem
            icon={<Folder className="size-4" />}
            label={t("toolOpen.folder")}
            onClick={() => choose(onFolder)}
          />
          <ToolMenuItem
            icon={<TerminalIcon className="size-4" />}
            label={t("toolOpen.terminal")}
            onClick={() => choose(onTerminal)}
          />
          <ToolMenuItem
            icon={<ExternalLink className="size-4" />}
            label={label}
            onClick={() => choose(onTool)}
          />
        </div>
      )}
    </div>
  );
}

function ToolMenuItem({
  icon,
  label,
  onClick,
}: {
  icon: ReactNode;
  label: string;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      role="menuitem"
      onClick={onClick}
      className="flex w-full items-center gap-2 rounded px-2 py-1.5 text-left text-sm hover:bg-accent hover:text-accent-foreground"
    >
      {icon}
      <span className="truncate">{label}</span>
    </button>
  );
}
```

- [ ] **Step 3: Type-check**

Run: `cd C:/agentspace/agentsafe/apps/desktop/frontend && pnpm exec tsc --noEmit`
Expected: no errors.

- [ ] **Step 4: Commit**

```bash
cd C:/agentspace/agentsafe
git add apps/desktop/frontend/src/components/ToolOpenMenu.tsx apps/desktop/frontend/src/i18n/translations.ts
git commit -m "feat(desktop): add ToolOpenMenu dropdown component"
```

---

### Task 4: Item 8 — Default tool setting in Settings

**Files:**
- Modify: `apps/desktop/frontend/src/pages/SettingsPage.tsx`
- Modify: `apps/desktop/frontend/src/i18n/translations.ts` (add `settings.defaultTool*` keys to `en` and `ko`)

**Interfaces:**
- Consumes: `getDefaultTool`, `setDefaultTool`, `toolLabel`, `TOOL_PRESETS`, `TOOL_PRESET_VALUES` from `@/lib/tool`; `api.SelectProgram`

- [ ] **Step 1: Add i18n keys**

In `translations.ts` `en` dict (near other `settings.*` keys):

```ts
  "settings.defaultToolTitle": "Default tool",
  "settings.defaultToolDesc": "Editor/program used by the \"Open with\" menu across the app.",
  "settings.toolPick": "Pick a program…",
```

In `ko` dict:

```ts
  "settings.defaultToolTitle": "기본 툴",
  "settings.defaultToolDesc": "앱 전체의 '툴 열기' 메뉴에서 사용할 편집기/프로그램입니다.",
  "settings.toolPick": "직접 선택…",
```

- [ ] **Step 2: Add imports**

In `SettingsPage.tsx`, add to the existing lucide import a `Wrench` icon, and add a new import line:

Add `Wrench` to the `lucide-react` import list (line 2 region). Then after the `api`/`errMessage` import add:

```ts
import {
  TOOL_PRESETS,
  TOOL_PRESET_VALUES,
  getDefaultTool,
  setDefaultTool,
  toolLabel,
} from "@/lib/tool";
```

- [ ] **Step 3: Add state + handler in `SettingsPage`**

Inside `SettingsPage`, after the `devMode` state, add:

```ts
  const [tool, setTool] = useState(getDefaultTool);

  async function changeTool(v: string) {
    if (v === "__pick__") {
      try {
        const sel = await api.SelectProgram();
        if (!sel) return;
        setDefaultTool(sel);
        setTool(sel);
        notify(t("settings.saved"), "success");
      } catch (e) {
        notify(errMessage(e), "error");
      }
      return;
    }
    setDefaultTool(v);
    setTool(v);
    notify(t("settings.saved"), "success");
  }
```

- [ ] **Step 4: Add the Settings card**

In `SettingsPage`'s returned JSX, insert this card immediately after the "Default terminal" `</Card>`:

```tsx
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Wrench className="size-5" /> {t("settings.defaultToolTitle")}
          </CardTitle>
          <CardDescription>{t("settings.defaultToolDesc")}</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="space-y-1.5">
            <Label htmlFor="defaultTool">{t("settings.defaultToolTitle")}</Label>
            <select
              id="defaultTool"
              value={TOOL_PRESET_VALUES.includes(tool) ? tool : "__custom__"}
              onChange={(e) => changeTool(e.target.value)}
              className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
            >
              {TOOL_PRESETS.map((p) => (
                <option key={p.value} value={p.value}>
                  {p.label}
                </option>
              ))}
              {!TOOL_PRESET_VALUES.includes(tool) && (
                <option value="__custom__">{toolLabel(tool)}</option>
              )}
              <option value="__pick__">{t("settings.toolPick")}</option>
            </select>
          </div>
        </CardContent>
      </Card>
```

- [ ] **Step 5: Type-check**

Run: `cd C:/agentspace/agentsafe/apps/desktop/frontend && pnpm exec tsc --noEmit`
Expected: no errors.

- [ ] **Step 6: Manual verify (in `wails dev` or built app)**

- Open Settings → "기본 툴" shows VS Code by default.
- Choose Cursor → toast "saved"; reopening Settings keeps Cursor.
- Choose "직접 선택…" → native picker opens; selecting an executable stores it and the select shows its basename.

- [ ] **Step 7: Commit**

```bash
cd C:/agentspace/agentsafe
git add apps/desktop/frontend/src/pages/SettingsPage.tsx apps/desktop/frontend/src/i18n/translations.ts
git commit -m "feat(desktop): add default tool setting (item 8)"
```

---

### Task 5: Item 3 — Clicking an open menu switches to its tab (no reorder)

**Files:**
- Modify: `apps/desktop/frontend/src/App.tsx` (`openView`, ~lines 726-748)

**Interfaces:**
- Consumes: existing `paneContainingTab`, `viewId`, `openTabs`, `layout`, `setActivePaneId`, `setLayout`, `moveTabToPane`

- [ ] **Step 1: Replace `openView` body to activate an existing tab in place**

Replace the existing `openView` definition with:

```tsx
  const openView = useCallback(
    (view: View, options?: { force?: boolean }) => {
      if (!options?.force && isWorkspaceDependent(view) && !opened) return;
      const id = viewId(view);
      // If the tab already exists, just activate it where it lives — do NOT
      // move/reinsert it (that reorders tabs on every menu click).
      const existingPaneId = paneContainingTab(layout, id);
      if (existingPaneId && openTabs[id]) {
        setActivePaneId(existingPaneId);
        setLayout((prev) =>
          prev.panes[existingPaneId]
            ? {
                ...prev,
                panes: {
                  ...prev.panes,
                  [existingPaneId]: {
                    ...prev.panes[existingPaneId],
                    activeTabId: id,
                  },
                },
              }
            : prev
        );
        return;
      }
      setOpenTabs((prev) => ({
        ...prev,
        [id]: prev[id] ?? { id, view, closable: true },
      }));
      setLayout((prev) => {
        const targetPaneId = prev.panes[activePaneId] ? activePaneId : firstPaneId(prev);
        const next = moveTabToPane(
          prev,
          { tabId: id, paneId: firstPaneId(prev) },
          targetPaneId
        );
        setActivePaneId(targetPaneId);
        return next;
      });
    },
    [activePaneId, opened, layout, openTabs]
  );
```

- [ ] **Step 2: Type-check**

Run: `cd C:/agentspace/agentsafe/apps/desktop/frontend && pnpm exec tsc --noEmit`
Expected: no errors.

- [ ] **Step 3: Manual verify**

- Open Workspace, Worktrees, Settings tabs (in that order).
- Click the Workspace menu again → it activates the existing Workspace tab and the tab order stays Workspace, Worktrees, Settings (no reordering, no duplicate).

- [ ] **Step 4: Commit**

```bash
cd C:/agentspace/agentsafe
git add apps/desktop/frontend/src/App.tsx
git commit -m "fix(desktop): activate existing tab on menu click without reordering (item 3)"
```

---

### Task 6: Item 1 — Header nav-collapse toggle

**Files:**
- Modify: `apps/desktop/frontend/src/App.tsx` (header JSX ~1285-1288; remove bottom toggle ~1248-1268 and hidden reveal ~1270-1282; imports ~16-25)

**Interfaces:**
- Consumes: existing `nextSidebarMode`, `sidebarToggleLabel`

- [ ] **Step 1: Swap sidebar-only chevron imports for `PanelLeft`**

In the `lucide-react` import block at the top of `App.tsx`, remove `ChevronLeft`, `ChevronRight`, and `ChevronsLeft`, and add `PanelLeft`. (Keep `ChevronDown` etc. if used elsewhere — these three are only used by the controls being removed.)

- [ ] **Step 2: Add the toggle to the header**

Replace the header element:

```tsx
        <header className="flex items-center justify-end gap-2 border-b px-6 py-3">
          <LogConsoleButton />
          <ThemeToggle />
        </header>
```

with:

```tsx
        <header className="flex items-center justify-between gap-2 border-b px-6 py-3">
          <button
            type="button"
            onClick={nextSidebarMode}
            title={sidebarToggleLabel}
            aria-label={sidebarToggleLabel}
            className="inline-flex size-9 items-center justify-center rounded-md text-muted-foreground hover:bg-accent hover:text-accent-foreground"
          >
            <PanelLeft className="size-5" />
          </button>
          <div className="flex items-center gap-2">
            <LogConsoleButton />
            <ThemeToggle />
          </div>
        </header>
```

- [ ] **Step 3: Remove the bottom-of-sidebar toggle**

Delete the entire `{!sidebarHidden && (` block that renders the bottom toggle button (the one with `absolute bottom-4 ...` and the `ChevronLeft`/`ChevronsLeft` icons), i.e. the block currently at ~lines 1248-1268.

- [ ] **Step 4: Remove the hidden-state edge reveal button**

Delete the entire `{sidebarHidden && (` block that renders the fixed left-edge reveal button (the `group fixed bottom-6 left-0 ...` div with `ChevronRight`), i.e. the block currently at ~lines 1270-1282.

- [ ] **Step 5: Type-check**

Run: `cd C:/agentspace/agentsafe/apps/desktop/frontend && pnpm exec tsc --noEmit`
Expected: no errors (confirms no leftover references to removed icons).

- [ ] **Step 6: Manual verify**

- The header top-left shows a panel icon. Clicking it cycles sidebar full → icons → hidden → full.
- When hidden, the header button still toggles it back (no edge reveal button needed).

- [ ] **Step 7: Commit**

```bash
cd C:/agentspace/agentsafe
git add apps/desktop/frontend/src/App.tsx
git commit -m "feat(desktop): move nav-collapse toggle to header (item 1)"
```

---

### Task 7: Item 2 — Workspace terminal as a new app tab

**Files:**
- Modify: `apps/desktop/frontend/src/App.tsx` (View union ~46-55; `viewId` ~135-144; `titleForView` ~590-613; `iconForView` ~615-635; `renderView` ~879-958; `closeTabs` ~781-800; add `openTerminalTab`; pass prop to `WorkspacePage`)
- Modify: `apps/desktop/frontend/src/pages/WorkspacePage.tsx` (Props + summary card buttons)

**Interfaces:**
- Consumes: `ToolOpenMenu` (Task 3), `useDefaultTool` (Task 2), `api.OpenPathInProgram` (Task 1), `api.TerminalOpen`, `api.OpenWorkspaceFolder`, `api.TerminalClose`
- Produces (App→WorkspacePage prop): `onOpenTerminal: (session: TerminalSession) => void`

- [ ] **Step 1: Extend the `View` union**

In `App.tsx`, add a terminal variant to the `View` type:

```tsx
type View =
  | { kind: "workspace" }
  | { kind: "features" }
  | { kind: "feature"; name: string }
  | { kind: "templates" }
  | { kind: "explorer" }
  | { kind: "agentsec" }
  | { kind: "backups" }
  | { kind: "history"; feature?: string }
  | { kind: "settings" }
  | { kind: "terminal"; id: string; path: string; title: string };
```

- [ ] **Step 2: Handle the terminal kind in `viewId`, `titleForView`, `iconForView`**

In `viewId`, add a case before `default`:

```tsx
    case "terminal":
      return `terminal:${view.id}`;
```

In `titleForView`, add:

```tsx
      case "terminal":
        return view.title;
```

In `iconForView`, add (uses the existing `Terminal` icon — add `Terminal` to the lucide import if not present):

```tsx
      case "terminal":
        return Terminal;
```

(Add `Terminal` to the `lucide-react` import block in `App.tsx`.)

- [ ] **Step 3: Render the terminal view**

In `renderView`, add a case (import `TerminalPanel` at top of `App.tsx`: `import { TerminalPanel } from "@/components/TerminalPanel";`):

```tsx
      case "terminal":
        return <TerminalPanel id={view.id} path={view.path} className="flex h-full flex-col" />;
```

- [ ] **Step 4: Add `openTerminalTab`**

After the `openView` definition, add:

```tsx
  const openTerminalTab = useCallback(
    (session: TerminalSession) => {
      openView({
        kind: "terminal",
        id: session.id,
        path: session.path,
        title: session.title || "Terminal",
      });
    },
    [openView]
  );
```

- [ ] **Step 5: Close the pty when a terminal tab closes**

In `closeTabs`, at the very start of the function (before the `setOpenTabs` call), add:

```tsx
    for (const tabId of tabIds) {
      const view = openTabs[tabId]?.view;
      if (view?.kind === "terminal") void api.TerminalClose(view.id);
    }
```

- [ ] **Step 6: Pass `onOpenTerminal` to `WorkspacePage`**

In `renderView`'s `case "workspace"`, add the prop:

```tsx
      case "workspace":
        return (
          <WorkspacePage
            config={config}
            root={root}
            onLoaded={onWorkspaceLoaded}
            onChanged={refreshConfig}
            onOpenTerminal={openTerminalTab}
          />
        );
```

- [ ] **Step 7: Update `WorkspacePage` props + imports**

In `WorkspacePage.tsx`, add to the imports:

```ts
import { ToolOpenMenu } from "@/components/ToolOpenMenu";
import { useDefaultTool } from "@/lib/tool";
import type { Config, RepoRuntimeState, TerminalSession } from "@/lib/types";
```

(Replace the existing `import type { Config, RepoRuntimeState } from "@/lib/types";` line with the one above.)

Update the `Props` interface to add:

```ts
  onOpenTerminal: (session: TerminalSession) => void;
```

Update the component signature: `export function WorkspacePage({ config, root, onLoaded, onChanged, onOpenTerminal }: Props) {`

Add the tool hook near the other hooks: `const tool = useDefaultTool();`

- [ ] **Step 8: Add the workspace terminal handler**

In `WorkspacePage`, add:

```ts
  async function openWorkspaceTerminalTab() {
    try {
      setBusy(true);
      const s = await api.TerminalOpen(root);
      if (s.external) {
        notify(t("toast.openedWorkspaceTerminal", { path: s.path }), "success");
        return;
      }
      onOpenTerminal(s);
    } catch (e) {
      notify(errMessage(e), "error");
    } finally {
      setBusy(false);
    }
  }
```

- [ ] **Step 9: Replace the three summary buttons with `ToolOpenMenu`**

In the summary `CardHeader`, replace the `<div className="flex shrink-0 flex-wrap justify-end gap-2">…</div>` block (the three Folder/Terminal/VSCode buttons) with:

```tsx
          <div className="flex shrink-0 justify-end">
            <ToolOpenMenu
              disabled={busy}
              onFolder={() =>
                openWorkspace(
                  () => api.OpenWorkspaceFolder(),
                  "toast.openedWorkspaceFolder"
                )
              }
              onTerminal={openWorkspaceTerminalTab}
              onTool={() =>
                openWorkspace(
                  () => api.OpenPathInProgram("", tool.value),
                  "toast.openedPath"
                )
              }
            />
          </div>
```

(Remove the now-unused `Terminal` and `Code2` lucide imports from `WorkspacePage.tsx` if they are no longer referenced; `FolderOpen` is still used by the onboarding view.)

- [ ] **Step 10: Type-check**

Run: `cd C:/agentspace/agentsafe/apps/desktop/frontend && pnpm exec tsc --noEmit`
Expected: no errors.

- [ ] **Step 11: Manual verify**

- Workspace summary card shows a single "툴 열기" dropdown.
- Folder → reveals the workspace folder. Tool → opens it in the configured tool.
- Terminal → a new terminal **tab** opens in the pane area, rooted at the workspace; typing works; closing the tab kills the pty (no leak).

- [ ] **Step 12: Commit**

```bash
cd C:/agentspace/agentsafe
git add apps/desktop/frontend/src/App.tsx apps/desktop/frontend/src/pages/WorkspacePage.tsx
git commit -m "feat(desktop): open workspace terminal as in-app tab via ToolOpenMenu (item 2)"
```

---

### Task 8: Item 4 — Embedded terminals fit the pane height

**Files:**
- Modify: `apps/desktop/frontend/src/components/TerminalPanel.tsx` (default className ~227)
- Modify: `apps/desktop/frontend/src/pages/FileExplorerPage.tsx` (grid ~383; right Card ~399; tab bar ~400; main/editor/terminal regions ~473-551)
- Modify: `apps/desktop/frontend/src/pages/FeatureDetailPage.tsx` (root wrapper + terminal tab block ~810-874, closing `</div>` ~1471)

**Interfaces:**
- No new exports. The fix is layout-only: terminals fill their parent (`h-full`) instead of viewport math.

- [ ] **Step 1: Make `TerminalPanel` fill its parent by default**

In `TerminalPanel.tsx`, change the outer slot default className:

```tsx
    <div ref={slotRef} className={className ?? "flex h-full flex-col"}>
```

(Was `className ?? "flex h-[calc(100vh-12rem)] flex-col"`.)

- [ ] **Step 2: File Explorer — make the page and right card fill the pane**

In `FileExplorerPage.tsx`:

Change the outer grid:

```tsx
    <div className="grid h-full min-h-0 grid-cols-[minmax(260px,360px)_1fr] gap-4">
```

(Was `grid h-[calc(100vh-9rem)] ...`.)

Change the right card to a flex column:

```tsx
      <Card className="flex min-h-0 min-w-0 flex-col overflow-hidden">
```

(Was `<Card className="min-w-0 overflow-hidden">`.)

Add `shrink-0` to the tab bar container:

```tsx
        <div className="flex shrink-0 items-center gap-1 overflow-x-auto border-b bg-muted/30 px-3 pt-2">
```

- [ ] **Step 3: File Explorer — fill/scroll the three content regions**

Wrap the `activeTab === "main"` branch contents in a scroll container. Replace:

```tsx
        {activeTab === "main" ? (
          <>
            <CardHeader>
```

with:

```tsx
        {activeTab === "main" ? (
          <div className="min-h-0 flex-1 overflow-auto">
            <CardHeader>
```

and replace its closing `</>` (the one right before `) : activeEditor ? (`) with `</div>`.

Change the editor branch container from `flex h-[calc(100vh-12rem)] flex-col` to fill:

```tsx
          <div className="flex min-h-0 flex-1 flex-col">
```

Wrap the terminal branch so it fills:

```tsx
        ) : activeTerminal ? (
          <div className="min-h-0 flex-1 overflow-hidden">
            <TerminalPanel id={activeTerminal.id} path={activeTerminal.path} />
          </div>
        ) : null}
```

- [ ] **Step 4: Feature detail — fill the pane and isolate the terminal tab**

In `FeatureDetailPage.tsx`, add a derived flag right before the `return (`:

```tsx
  const isTerminalTab = tab.startsWith("terminal:");
```

Change the root wrapper from `<div className="space-y-5">` to:

```tsx
    <div className="flex h-full min-h-0 flex-col gap-5">
```

Immediately after the header `<div className="flex items-center gap-3">…</div>` block (the back button + tab bar, ends ~line 864), the body must become a flex child. Replace the existing terminal block:

```tsx
      {tab.startsWith("terminal:") && (() => {
        const id = tab.slice("terminal:".length);
        const active = terminalTabs.find((terminal) => terminal.id === id);
        return active ? (
          <Card className="overflow-hidden">
            <TerminalPanel id={active.id} path={active.path} />
          </Card>
        ) : null;
      })()}
```

with:

```tsx
      {isTerminalTab ? (
        (() => {
          const id = tab.slice("terminal:".length);
          const active = terminalTabs.find((terminal) => terminal.id === id);
          return active ? (
            <div className="min-h-0 flex-1">
              <Card className="flex h-full flex-col overflow-hidden">
                <TerminalPanel id={active.id} path={active.path} className="flex h-full flex-col" />
              </Card>
            </div>
          ) : null;
        })()
      ) : (
        <div className="min-h-0 flex-1 overflow-auto pr-1">
```

Then, after the last non-terminal tab block (the `{tab === "settings" && (…)}` block closes ~line 1429) and BEFORE the `{fileView && (` modal block, close the scroll wrapper with:

```tsx
        </div>
      )}
```

(The `{fileView && (…)}` modal stays as the last child of the root flex `<div>`, since it is fixed-position.)

- [ ] **Step 5: Type-check**

Run: `cd C:/agentspace/agentsafe/apps/desktop/frontend && pnpm exec tsc --noEmit`
Expected: no errors.

- [ ] **Step 6: Manual verify (especially with a split pane)**

- File Explorer: open a terminal tab; it fills the right card. Split the pane horizontally (drag a tab to the bottom edge) → the terminal shrinks to the pane and the page does **not** grow a scrollbar.
- Feature detail: open a terminal tab; it fills the pane. Work/Status tabs still scroll normally inside the pane.
- Workspace terminal tab (Task 7) fills its pane too.

- [ ] **Step 7: Commit**

```bash
cd C:/agentspace/agentsafe
git add apps/desktop/frontend/src/components/TerminalPanel.tsx apps/desktop/frontend/src/pages/FileExplorerPage.tsx apps/desktop/frontend/src/pages/FeatureDetailPage.tsx
git commit -m "fix(desktop): fit embedded terminals to pane height (item 4)"
```

---

### Task 9: Item 7 — Status tab redesign (worktree + agent)

**Files:**
- Modify: `apps/desktop/frontend/src/pages/FeatureDetailPage.tsx` (`tab === "status"` block ~1132-1324; add handlers near other open handlers ~594-644; add `useDefaultTool`; remove unused `agentOpenTarget`/`openAgentTarget`/`openProgram`)
- Modify: `apps/desktop/frontend/src/i18n/translations.ts` (add `feature.create`, `feature.regenerate`, `feature.worktreeCardDesc` to `en` and `ko`)

**Interfaces:**
- Consumes: `ToolOpenMenu` (Task 3), `useDefaultTool` (Task 2), `api.OpenPathInProgram` (Task 1), `api.OpenPath`, `api.TerminalOpenWithProgram`; existing `copyPath`, `openFeatureFolder`, `rebase`, `loadStatus`, `loadDiff`, `prepare`, `del`, `addTerminalTab`, `openTerminal`, `terminalProgram`

- [ ] **Step 1: Add i18n keys**

`en` dict:

```ts
  "feature.create": "Create",
  "feature.regenerate": "Regenerate",
  "feature.worktreeCardDesc": "Feature worktree branch and path.",
```

`ko` dict:

```ts
  "feature.create": "생성",
  "feature.regenerate": "재생성",
  "feature.worktreeCardDesc": "기능 워크트리의 브랜치와 경로입니다.",
```

- [ ] **Step 2: Imports + tool hook**

In `FeatureDetailPage.tsx`, add `import { ToolOpenMenu } from "@/components/ToolOpenMenu";` and `import { useDefaultTool } from "@/lib/tool";`. Inside the component (near `const { notify } = useToast();`) add `const tool = useDefaultTool();`.

- [ ] **Step 3: Add the new open handlers**

Near the existing `openFeatureFolder`/`copyPath` handlers, add:

```tsx
  const openWorktreeTerminal = () =>
    run(async () => {
      if (!featurePaths?.worktreePath) return;
      const s = await api.TerminalOpenWithProgram(
        featurePaths.worktreePath,
        terminalProgram.trim()
      );
      if (s.external) {
        notify(t("toast.openedPath", { path: s.path }), "success");
        return;
      }
      addTerminalTab(s, `Terminal · ${name}`);
      notify(t("toast.openedEmbeddedTerminal", { path: s.path }), "success");
    });

  const openWorktreeTool = () =>
    run(async () => {
      if (!featurePaths?.worktreePath) return;
      const p = await api.OpenPathInProgram(featurePaths.worktreePath, tool.value);
      notify(t("toast.openedPath", { path: p }), "success");
    });

  const openAgentFolder = () =>
    run(async () => {
      if (!featurePaths?.agentPath) return;
      const p = await api.OpenPath(featurePaths.agentPath);
      notify(t("toast.openedPath", { path: p }), "success");
    });

  const openAgentTool = () =>
    run(async () => {
      if (!featurePaths?.agentPath) return;
      const p = await api.OpenPathInProgram(featurePaths.agentPath, tool.value);
      notify(t("toast.openedPath", { path: p }), "success");
    });
```

- [ ] **Step 4: Replace the worktree card header**

Replace the worktree `CardHeader` (the `<CardHeader className="space-y-4">` opening the `tab === "status"` block, through its closing `</CardHeader>` at ~line 1165) with:

```tsx
            <CardHeader className="space-y-4">
              <div className="flex items-start justify-between gap-4">
                <div className="min-w-0">
                  <div className="flex items-center gap-2">
                    <CardTitle>워크트리</CardTitle>
                    <Badge variant="secondary" className="font-mono">
                      {status?.branch ?? "-"}
                    </Badge>
                  </div>
                  <CardDescription>{t("feature.worktreeCardDesc")}</CardDescription>
                </div>
                <Badge variant="outline">{status?.feature ?? name}</Badge>
              </div>
              {featurePaths?.worktreePath && (
                <div className="flex items-center gap-2 rounded-md border bg-muted/30 p-2">
                  <span
                    className="min-w-0 flex-1 truncate font-mono text-xs"
                    title={featurePaths.worktreePath}
                  >
                    {featurePaths.worktreePath}
                  </span>
                  <Button
                    variant="ghost"
                    size="icon"
                    className="size-8"
                    title={t("feature.copyPath")}
                    disabled={busy}
                    onClick={() => copyPath(featurePaths?.worktreePath)}
                  >
                    <Copy className="size-4" />
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon"
                    className="size-8"
                    title={t("feature.openWorktreeFolder")}
                    disabled={busy || !featurePaths?.worktreePath}
                    onClick={openFeatureFolder}
                  >
                    <FolderOpen className="size-4" />
                  </Button>
                </div>
              )}
              <div className="flex flex-wrap items-center gap-2">
                <Button variant="outline" size="sm" onClick={rebase} disabled={busy || statusLoading}>
                  <GitMerge className="size-4" /> {t("feature.rebase")}
                </Button>
                <Button variant="outline" size="sm" onClick={loadStatus} disabled={statusLoading}>
                  {statusLoading ? <Loader2 className="size-4 animate-spin" /> : <RefreshCw className="size-4" />}
                  {statusLoading ? t("common.loading") : t("common.refresh")}
                </Button>
                <ToolOpenMenu
                  disabled={busy || !featurePaths?.worktreePath}
                  onFolder={openFeatureFolder}
                  onTerminal={openWorktreeTerminal}
                  onTool={openWorktreeTool}
                />
              </div>
            </CardHeader>
```

- [ ] **Step 5: Replace the agent card header**

Replace the agent `CardHeader` (the `<CardHeader className="space-y-4">` that contains `<CardTitle>에이전트</CardTitle>`, through its closing `</CardHeader>` at ~line 1273) with:

```tsx
            <CardHeader className="space-y-4">
              <div>
                <CardTitle>에이전트</CardTitle>
                <CardDescription>에이전트 폴더 생성 상태와 저장소별 준비 상태입니다.</CardDescription>
              </div>
              {featurePaths?.agentPath && (
                <div className="flex items-center gap-2 rounded-md border bg-muted/30 p-2">
                  <span
                    className="min-w-0 flex-1 truncate font-mono text-xs"
                    title={featurePaths.agentPath}
                  >
                    {featurePaths.agentPath}
                  </span>
                  <Button
                    variant="ghost"
                    size="icon"
                    className="size-8"
                    title={t("feature.copyPath")}
                    disabled={busy}
                    onClick={() => copyPath(featurePaths?.agentPath)}
                  >
                    <Copy className="size-4" />
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon"
                    className="size-8"
                    title={t("feature.openFolder")}
                    disabled={busy || !featurePaths?.agentPath}
                    onClick={openAgentFolder}
                  >
                    <FolderOpen className="size-4" />
                  </Button>
                </div>
              )}
              <div className="flex flex-wrap items-center gap-2">
                <Button
                  size="sm"
                  onClick={prepare}
                  disabled={busy || diffLoading || agentStatusLoading}
                  variant={agentReady ? "outline" : "default"}
                >
                  {agentStatusLoading ? <Loader2 className="size-4 animate-spin" /> : <Wand2 className="size-4" />}
                  {agentStatusLoading
                    ? t("common.loading")
                    : agentReady
                      ? t("feature.regenerate")
                      : t("feature.create")}
                </Button>
                {agentReady && (
                  <>
                    <Button variant="outline" size="sm" onClick={() => loadDiff(true)} disabled={busy || diffLoading}>
                      {diffLoading ? <Loader2 className="size-4 animate-spin" /> : <RefreshCw className="size-4" />}
                      {t("common.refresh")}
                    </Button>
                    <Button variant="destructive" size="sm" onClick={del} disabled={busy}>
                      <Trash2 className="size-4" /> {t("common.delete")}
                    </Button>
                    <ToolOpenMenu
                      disabled={busy || !featurePaths?.agentPath}
                      onFolder={openAgentFolder}
                      onTerminal={openTerminal}
                      onTool={openAgentTool}
                    />
                  </>
                )}
              </div>
            </CardHeader>
```

- [ ] **Step 6: Remove now-unused agent open-target code**

Delete the `agentOpenTarget` state (`const [agentOpenTarget, setAgentOpenTarget] = useState<…>("terminal");`), the `openAgentTarget` function, and the `openProgram` function (it was only used by `openAgentTarget`). Do not remove `program`, `programLabel`, or `openRepoProgram` — those are still used by the worktree repo rows.

- [ ] **Step 7: Type-check**

Run: `cd C:/agentspace/agentsafe/apps/desktop/frontend && pnpm exec tsc --noEmit`
Expected: no errors (confirms no leftover references to removed `agentOpenTarget`/`openAgentTarget`/`openProgram`).

- [ ] **Step 8: Manual verify**

- Status tab → Worktree card: title shows branch as a tag; a path row has icon-only copy + folder buttons; button row = rebase / refresh / 툴 열기. The 툴 열기 terminal opens a worktree-rooted terminal tab; tool opens the worktree in the configured tool.
- Agent card: agent path row with icon copy + folder; button row = 생성/재생성 (Korean) / 새로고침 / 삭제 / 툴 열기. Tool/terminal/folder act on the agent path.

- [ ] **Step 9: Commit**

```bash
cd C:/agentspace/agentsafe
git add apps/desktop/frontend/src/pages/FeatureDetailPage.tsx apps/desktop/frontend/src/i18n/translations.ts
git commit -m "feat(desktop): redesign worktree status tab with ToolOpenMenu (item 7)"
```

---

### Task 10: File Explorer — replace open buttons with `ToolOpenMenu`

**Files:**
- Modify: `apps/desktop/frontend/src/pages/FileExplorerPage.tsx` (action buttons ~498-519; imports; remove `openVSCode`)

**Interfaces:**
- Consumes: `ToolOpenMenu` (Task 3), `useDefaultTool` (Task 2), `api.OpenPathInProgram` (Task 1); existing `openSelected`, `openEmbeddedTerminal`

- [ ] **Step 1: Imports + tool hook**

Add `import { ToolOpenMenu } from "@/components/ToolOpenMenu";` and `import { useDefaultTool } from "@/lib/tool";`. Inside `FileExplorerPage`, add `const tool = useDefaultTool();` near the other hooks.

- [ ] **Step 2: Add the tool handler; remove `openVSCode`**

Delete the `openVSCode` function. Add:

```tsx
  async function openTool() {
    if (!selected) return;
    try {
      const p = await api.OpenPathInProgram(selected.path, tool.value);
      notify(t("toast.openedPath", { path: p }), "success");
    } catch (e) {
      notify(errMessage(e), "error");
    }
  }
```

- [ ] **Step 3: Replace the action buttons**

Replace the three buttons — `Open` (`openSelected`), `Open VSCode` (`openVSCode`), and `Open Terminal` (`openEmbeddedTerminal`) — with a single `ToolOpenMenu`, keeping the editor / copy / delete buttons. The action row becomes:

```tsx
                  <div className="flex flex-wrap gap-2">
                    <ToolOpenMenu
                      align="start"
                      disabled={busy || !selected}
                      onFolder={openSelected}
                      onTerminal={openEmbeddedTerminal}
                      onTool={openTool}
                    />
                    {!selected.isDir && (
                      <Button variant="outline" onClick={() => openEditor(selected)} disabled={busy}>
                        <File className="size-4" /> {t("explorer.openEditor")}
                      </Button>
                    )}
                    <Button variant="outline" onClick={copyPath} disabled={busy}>
                      <Copy className="size-4" /> {t("feature.copyPath")}
                    </Button>
                    <Button variant="destructive" onClick={removeSelected} disabled={busy}>
                      <Trash2 className="size-4" /> {t("common.delete")}
                    </Button>
                  </div>
```

(Remove the now-unused `Code2` import. Keep `FolderOpen` — still used in `TreeItem`. Keep `TerminalIcon` — used in the terminal tab list.)

- [ ] **Step 4: Type-check**

Run: `cd C:/agentspace/agentsafe/apps/desktop/frontend && pnpm exec tsc --noEmit`
Expected: no errors.

- [ ] **Step 5: Manual verify**

- Select a folder → 툴 열기 dropdown: Folder opens it, Terminal opens an embedded terminal tab (fills height per Task 8), tool opens it in the configured tool.
- Editor/Copy/Delete buttons still work; the editor button only shows for files.

- [ ] **Step 6: Commit**

```bash
cd C:/agentspace/agentsafe
git add apps/desktop/frontend/src/pages/FileExplorerPage.tsx
git commit -m "feat(desktop): use ToolOpenMenu in file explorer"
```

---

### Task 11: Item 5 — Template drag-and-drop upload

**Files:**
- Modify: `apps/desktop/frontend/src/pages/WorktreeTemplatesPage.tsx` (add `pageRef`; widen drop hit-test ~327-340; drop handler ~639-651)

**Interfaces:**
- Consumes: existing `importPaths`, `runtime()` event `workspace:file-drop`

**Root cause:** In the Wails WebView the HTML5 `file.path` is always empty, so the `onDrop` handler imports nothing. The working channel is the already-wired `workspace:file-drop` event (real OS paths), but its hit-test only accepts drops landing inside the small dashed zone (`dropRef`), so most drops are ignored. Fix: accept a drop anywhere on the templates page (multi-pane disambiguation still works because the event carries the drop coordinates).

- [ ] **Step 1: Add a page-level ref**

In `WorktreeTemplatesPage`, add near `dropRef`:

```tsx
  const pageRef = useRef<HTMLDivElement | null>(null);
```

- [ ] **Step 2: Attach it to the outer page container**

Change the outer return container to carry the ref:

```tsx
    <div
      ref={pageRef}
      className="grid h-[calc(100vh-9rem)] grid-cols-[minmax(260px,360px)_1fr] gap-4"
    >
```

- [ ] **Step 3: Widen the file-drop hit-test**

Replace the `workspace:file-drop` subscription body:

```tsx
  useEffect(() => {
    const rt = runtime();
    if (!rt) return;
    return rt.EventsOn("workspace:file-drop", (...data: unknown[]) => {
      const payload = data[0] as { x?: number; y?: number; paths?: string[] };
      if (!payload?.paths?.length) return;
      const target =
        typeof payload.x === "number" && typeof payload.y === "number"
          ? document.elementFromPoint(payload.x, payload.y)
          : null;
      // Accept a drop anywhere on this page (so users can drop onto the whole
      // panel, not just the dashed zone). The coordinate check still scopes the
      // drop to this page's pane when multiple pages are open side by side.
      if (target && pageRef.current && !pageRef.current.contains(target)) return;
      void importPaths(payload.paths);
    });
  }, [selectedFolder.id, isRootSelected]);
```

- [ ] **Step 4: Remove the dead HTML5 `.path` extraction**

In the dropzone `<div>`, replace the `onDrop` handler so it no longer reads `file.path` (which is always empty), keeping only drag affordance:

```tsx
                onDragOver={(e) => e.preventDefault()}
                onDrop={(e) => e.preventDefault()}
```

- [ ] **Step 5: Type-check**

Run: `cd C:/agentspace/agentsafe/apps/desktop/frontend && pnpm exec tsc --noEmit`
Expected: no errors.

- [ ] **Step 6: Manual verify (in the built/`wails dev` app — drag-drop needs the native shell)**

- Select a non-root destination (e.g. a repository under features).
- Drag files/a folder from the OS file manager onto the templates page → they import into the selected folder and the list refreshes.
- With the templates page open in one split pane and another page in the other, a drop on the templates pane imports; a drop on the other pane does not.
- If a drop still does nothing, use superpowers:systematic-debugging: log the `workspace:file-drop` payload; if the event never fires, set `CSSDropProperty: "--wails-drop-target"` / `CSSDropValue: "drop"` in `apps/desktop/main.go`'s `options.DragAndDrop` and retest.

- [ ] **Step 7: Commit**

```bash
cd C:/agentspace/agentsafe
git add apps/desktop/frontend/src/pages/WorktreeTemplatesPage.tsx
git commit -m "fix(desktop): make template drag-and-drop upload to selected folder (item 5)"
```

---

### Task 12: Item 6 — Template detail folder collapse

**Files:**
- Modify: `apps/desktop/frontend/src/pages/WorktreeTemplatesPage.tsx` (`load` seeding ~189-193; `TemplateFileNode` `open` derivation ~499)

**Interfaces:**
- Consumes: existing `expandedTemplateNodes`, `nodeKey`, `toggleTemplateNode`

**Root cause:** the displayed template root row forces `open = … || isRoot`, so its folder chevron renders but clicking never collapses it (it is always open). Fix: drop the `|| isRoot` force and instead seed the *effective root* key as expanded by default, so the root is expanded initially but collapsible, while nested folders keep working.

- [ ] **Step 1: Seed the effective-root key in `load`**

In the `load` callback, replace the `setExpandedTemplateNodes` seeding block:

```tsx
      setExpandedTemplateNodes((prev) => {
        const next = new Set(prev);
        for (const tree of trees ?? []) {
          const effectiveRoot =
            tree.root.children?.length === 1 ? tree.root.children[0] : tree.root;
          next.add(nodeKey(tree.template.id, effectiveRoot.relPath));
        }
        return next;
      });
```

- [ ] **Step 2: Remove the `|| isRoot` force in `TemplateFileNode`**

Change the `open` derivation:

```tsx
    const open = expandedTemplateNodes.has(key);
```

(Was `const open = expandedTemplateNodes.has(key) || isRoot;`.)

- [ ] **Step 3: Type-check**

Run: `cd C:/agentspace/agentsafe/apps/desktop/frontend && pnpm exec tsc --noEmit`
Expected: no errors.

- [ ] **Step 4: Manual verify**

- Open a template with folders in the detail list. Each template renders expanded by default.
- Click a folder row (root or nested) → it collapses; click again → it expands. Collapsed state persists across folder selection (until a refresh re-seeds the root).
- If folder collapse still misbehaves, use superpowers:systematic-debugging: log `key`, `open`, and the `expandedTemplateNodes` set on click to confirm the toggle path before adjusting.

- [ ] **Step 5: Commit**

```bash
cd C:/agentspace/agentsafe
git add apps/desktop/frontend/src/pages/WorktreeTemplatesPage.tsx
git commit -m "fix(desktop): allow collapsing folders in template detail tree (item 6)"
```

---

## Final verification

- [ ] **Full build**

Run: `cd C:/agentspace/agentsafe/apps/desktop/frontend && pnpm build`
Expected: `tsc` clean + `vite build` succeeds.

Run: `cd C:/agentspace/agentsafe && go build ./apps/desktop && go vet ./apps/desktop`
Expected: no output.

- [ ] **App smoke test** (`wails dev` from `apps/desktop`, or `make build-desktop`): walk every item's manual-verify checklist once more in the running app, confirming the new binding `OpenPathInProgram` is exposed (the tool action opens paths in the configured tool).
