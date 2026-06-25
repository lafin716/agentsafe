import {
  useCallback,
  useEffect,
  Fragment,
  useRef,
  useState,
  type CSSProperties,
  type DragEvent,
  type PointerEvent as ReactPointerEvent,
} from "react";
import {
  Archive,
  FileText,
  FolderGit2,
  FolderOpen,
  History,
  LayoutGrid,
  PanelLeft,
  Settings,
  ShieldCheck,
  Terminal,
  X,
  type LucideIcon,
} from "lucide-react";
import { api, errMessage } from "@/lib/api";
import type { Config, TerminalSession } from "@/lib/types";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { useToast } from "@/components/ui/toast";
import { useI18n } from "@/i18n/I18nProvider";
import { WorkspaceSwitcher } from "@/components/WorkspaceSwitcher";
import { ThemeToggle } from "@/components/ThemeToggle";
import { LogConsoleButton } from "@/components/ui/log-console";
import { TerminalPanel } from "@/components/TerminalPanel";
import { WorkspacePage } from "@/pages/WorkspacePage";
import { FeaturesPage } from "@/pages/FeaturesPage";
import { FeatureDetailPage, type FeatureDetailTab } from "@/pages/FeatureDetailPage";
import { SettingsPage } from "@/pages/SettingsPage";
import { AgentSecurityPage } from "@/pages/AgentSecurityPage";
import { BackupsPage } from "@/pages/BackupsPage";
import { HistoryPage } from "@/pages/HistoryPage";
import { WorktreeTemplatesPage } from "@/pages/WorktreeTemplatesPage";
import { FileExplorerPage } from "@/pages/FileExplorerPage";
import agentsafeLogo from "@/assets/agentsafe-logo.png";

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

type SidebarMode = "full" | "icons" | "hidden";
type DropEdge = "left" | "right" | "top" | "bottom";

type AppTab = {
  id: string;
  view: View;
  closable: boolean;
};

type PaneModel = {
  id: string;
  tabIds: string[];
  activeTabId: string;
};

type PaneLayout = {
  panes: Record<string, PaneModel>;
  rows: string[][];
};

type SplitSizes = {
  rowSizes: number[];
  columnSizesByRow: Record<string, number[]>;
};

type DraggedTab = {
  tabId: string;
  paneId: string;
};

type DropHint = {
  paneId: string;
  edge: DropEdge;
};

// Per-workspace window state. `Persisted` is JSON-serializable and saved to
// localStorage keyed by workspace root; `Session` holds live terminal/agent
// sessions and is kept in memory only (not serializable).
type PersistedWindowState = {
  openTabs: Record<string, AppTab>;
  layout: PaneLayout;
  splitSizes: SplitSizes;
  activePaneId: string;
};

type SessionWindowState = {
  featureTerminalTabs: Record<string, TerminalSession[]>;
  featureAgentSessions: Record<string, TerminalSession | null>;
  featureActiveTabs: Record<string, FeatureDetailTab>;
  explorerTerminals: TerminalSession[];
  explorerActiveTab: string;
  featuresExpanded: Set<string>;
  featuresActiveTerminal: Record<string, string>;
  featuresTerminalHeights: Record<string, number>;
};

const workspaceTab: AppTab = {
  id: "workspace",
  view: { kind: "workspace" },
  closable: true,
};

const initialLayout: PaneLayout = {
  panes: {
    "pane-1": { id: "pane-1", tabIds: ["workspace"], activeTabId: "workspace" },
  },
  rows: [["pane-1"]],
};

function emptyLayout(paneId = "pane-1"): PaneLayout {
  return {
    panes: {
      [paneId]: { id: paneId, tabIds: [], activeTabId: "" },
    },
    rows: [[paneId]],
  };
}

function viewId(view: View): string {
  switch (view.kind) {
    case "feature":
      return `feature:${view.name}`;
    case "history":
      return view.feature ? `history:${view.feature}` : "history";
    case "terminal":
      return `terminal:${view.id}`;
    default:
      return view.kind;
  }
}

function isWorkspaceDependent(view: View): boolean {
  return !["workspace", "settings"].includes(view.kind);
}

function loadSidebarMode(): SidebarMode {
  try {
    const saved = localStorage.getItem("agentsafe.sidebarMode");
    return saved === "full" || saved === "icons" || saved === "hidden"
      ? saved
      : "full";
  } catch {
    return "full";
  }
}

function cloneLayout(layout: PaneLayout): PaneLayout {
  return {
    panes: Object.fromEntries(
      Object.entries(layout.panes).map(([id, pane]) => [
        id,
        { ...pane, tabIds: [...pane.tabIds] },
      ])
    ),
    rows: layout.rows.map((row) => [...row]),
  };
}

function firstPaneId(layout: PaneLayout): string {
  return layout.rows.flat().find((id) => !!layout.panes[id]) ?? "pane-1";
}

function paneContainingTab(layout: PaneLayout, tabId: string): string | null {
  return layout.rows.flat().find((id) => layout.panes[id]?.tabIds.includes(tabId)) ?? null;
}

function panePosition(layout: PaneLayout, paneId: string) {
  for (let rowIndex = 0; rowIndex < layout.rows.length; rowIndex += 1) {
    const columnIndex = layout.rows[rowIndex].indexOf(paneId);
    if (columnIndex >= 0) return { rowIndex, columnIndex };
  }
  return null;
}

function normalizeLayout(layout: PaneLayout): PaneLayout {
  const rows = layout.rows
    .map((row) => row.filter((id) => !!layout.panes[id]))
    .filter((row) => row.length > 0)
    .slice(0, 2)
    .map((row) => row.slice(0, 2));
  const validIds = new Set(rows.flat());
  const panes = Object.fromEntries(
    Object.entries(layout.panes).filter(([id]) => validIds.has(id))
  ) as Record<string, PaneModel>;

  if (Object.keys(panes).length === 0) return emptyLayout();

  for (const pane of Object.values(panes)) {
    if (!pane.tabIds.includes(pane.activeTabId)) pane.activeTabId = pane.tabIds[0] ?? "";
  }

  return { panes, rows };
}

function cleanupLayout(layout: PaneLayout, preservePaneId?: string): PaneLayout {
  const normalized = normalizeLayout(layout);
  const paneIds = normalized.rows.flat();
  const nonEmptyIds = paneIds.filter((id) => normalized.panes[id]?.tabIds.length > 0);
  const emptyIds = paneIds.filter((id) => normalized.panes[id]?.tabIds.length === 0);

  if (nonEmptyIds.length === 0) {
    const paneId = preservePaneId && normalized.panes[preservePaneId] ? preservePaneId : paneIds[0] ?? "pane-1";
    return emptyLayout(paneId);
  }

  const keepEmptyId =
    preservePaneId && emptyIds.includes(preservePaneId) ? preservePaneId : undefined;
  const keepIds = new Set([...nonEmptyIds, ...(keepEmptyId ? [keepEmptyId] : [])]);
  const rows = normalized.rows
    .map((row) => row.filter((id) => keepIds.has(id)))
    .filter((row) => row.length > 0);
  const panes = Object.fromEntries(
    Object.entries(normalized.panes).filter(([id]) => keepIds.has(id))
  ) as Record<string, PaneModel>;

  return normalizeLayout({ panes, rows });
}

function removeTabFromPane(pane: PaneModel, tabId: string): PaneModel {
  const index = pane.tabIds.indexOf(tabId);
  if (index < 0) return { ...pane, tabIds: [...pane.tabIds] };

  const tabIds = pane.tabIds.filter((id) => id !== tabId);
  const activeTabId =
    pane.activeTabId === tabId ? tabIds[Math.max(0, index - 1)] ?? tabIds[0] ?? "" : pane.activeTabId;
  return { ...pane, tabIds, activeTabId };
}

function insertTab(tabIds: string[], tabId: string, beforeTabId?: string): string[] {
  const without = tabIds.filter((id) => id !== tabId);
  if (!beforeTabId) return [...without, tabId];
  const targetIndex = without.indexOf(beforeTabId);
  if (targetIndex < 0) return [...without, tabId];
  return [...without.slice(0, targetIndex), tabId, ...without.slice(targetIndex)];
}

function computeDropEdge(e: DragEvent<HTMLElement>): DropEdge | null {
  const rect = e.currentTarget.getBoundingClientRect();
  const x = e.clientX - rect.left;
  const y = e.clientY - rect.top;
  const horizontalEdgeSize = rect.width * 0.24;
  const verticalEdgeSize = rect.height * 0.24;
  if (x <= horizontalEdgeSize) return "left";
  if (x >= rect.width - horizontalEdgeSize) return "right";
  if (y <= verticalEdgeSize) return "top";
  if (y >= rect.height - verticalEdgeSize) return "bottom";
  return null;
}

function clampSplit(value: number): number {
  return Math.min(80, Math.max(20, value));
}

function normalizePair(values?: number[]): number[] {
  if (!values || values.length < 2) return [50, 50];
  const first = clampSplit(values[0]);
  return [first, 100 - first];
}

function normalizeSplitSizes(layout: PaneLayout, sizes: SplitSizes): SplitSizes {
  const rowCount = layout.rows.length;
  const rowSizes = rowCount <= 1 ? [100] : normalizePair(sizes.rowSizes);
  const columnSizesByRow: Record<string, number[]> = {};
  layout.rows.forEach((row, rowIndex) => {
    const key = String(rowIndex);
    columnSizesByRow[key] =
      row.length <= 1 ? [100] : normalizePair(sizes.columnSizesByRow[key]);
  });
  return { rowSizes, columnSizesByRow };
}

const WINDOWS_KEY = "agentsafe.workspaceWindows";

function defaultSplitSizes(): SplitSizes {
  return { rowSizes: [100], columnSizesByRow: {} };
}

function freshPersisted(): PersistedWindowState {
  const layout = cloneLayout(initialLayout);
  return {
    openTabs: { workspace: workspaceTab },
    layout,
    splitSizes: normalizeSplitSizes(layout, defaultSplitSizes()),
    activePaneId: "pane-1",
  };
}

function freshSession(): SessionWindowState {
  return {
    featureTerminalTabs: {},
    featureAgentSessions: {},
    featureActiveTabs: {},
    explorerTerminals: [],
    explorerActiveTab: "main",
    featuresExpanded: new Set(),
    featuresActiveTerminal: {},
    featuresTerminalHeights: {},
  };
}

// Make a restored/persisted window state internally consistent: drop tab
// references that no longer exist, normalize the pane layout, prune orphan
// tabs, and fix the active pane / split sizes.
function reconcileWindowState(state: Partial<PersistedWindowState>): PersistedWindowState {
  const fallback = freshPersisted();
  const openTabs =
    state.openTabs && typeof state.openTabs === "object"
      ? ({ ...state.openTabs } as Record<string, AppTab>)
      : fallback.openTabs;
  const rawLayout =
    state.layout && state.layout.panes && Array.isArray(state.layout.rows)
      ? state.layout
      : fallback.layout;
  const filtered: PaneLayout = {
    panes: Object.fromEntries(
      Object.entries(rawLayout.panes).map(([id, pane]) => {
        const tabIds = (pane.tabIds ?? []).filter((tabId) => !!openTabs[tabId]);
        return [
          id,
          {
            ...pane,
            tabIds,
            activeTabId: tabIds.includes(pane.activeTabId) ? pane.activeTabId : tabIds[0] ?? "",
          },
        ];
      })
    ) as Record<string, PaneModel>,
    rows: rawLayout.rows.map((row) => [...row]),
  };
  const layout = cleanupLayout(filtered);
  const referenced = new Set(Object.values(layout.panes).flatMap((pane) => pane.tabIds));
  const prunedTabs = Object.fromEntries(
    Object.entries(openTabs).filter(([id]) => referenced.has(id))
  ) as Record<string, AppTab>;
  const activePaneId =
    state.activePaneId && layout.panes[state.activePaneId]
      ? state.activePaneId
      : firstPaneId(layout);
  const splitSizes = normalizeSplitSizes(layout, state.splitSizes ?? defaultSplitSizes());
  return { openTabs: prunedTabs, layout, splitSizes, activePaneId };
}

function loadWorkspaceWindows(): Record<string, PersistedWindowState> {
  try {
    const raw = localStorage.getItem(WINDOWS_KEY);
    if (!raw) return {};
    const parsed = JSON.parse(raw) as Record<string, Partial<PersistedWindowState>>;
    if (!parsed || typeof parsed !== "object") return {};
    const out: Record<string, PersistedWindowState> = {};
    for (const [root, state] of Object.entries(parsed)) {
      if (!root) continue;
      out[root] = reconcileWindowState(state ?? {});
    }
    return out;
  } catch {
    return {};
  }
}

function saveWorkspaceWindows(map: Record<string, PersistedWindowState>) {
  try {
    localStorage.setItem(WINDOWS_KEY, JSON.stringify(map));
  } catch {
    /* localStorage unavailable */
  }
}

export default function App() {
  const { notify } = useToast();
  const { t } = useI18n();
  const paneIdCounter = useRef(2);
  const [config, setConfig] = useState<Config | null>(null);
  const [root, setRoot] = useState("");
  const [openTabs, setOpenTabs] = useState<Record<string, AppTab>>({
    workspace: workspaceTab,
  });
  const [layout, setLayout] = useState<PaneLayout>(() => cloneLayout(initialLayout));
  const [splitSizes, setSplitSizes] = useState<SplitSizes>(() =>
    normalizeSplitSizes(initialLayout, defaultSplitSizes())
  );
  const [activePaneId, setActivePaneId] = useState("pane-1");
  const [draggedTab, setDraggedTab] = useState<DraggedTab | null>(null);
  const [dropHint, setDropHint] = useState<DropHint | null>(null);
  const [sidebarMode, setSidebarMode] = useState<SidebarMode>(loadSidebarMode);
  const [featureTerminalTabs, setFeatureTerminalTabs] = useState<
    Record<string, TerminalSession[]>
  >({});
  const [featureAgentSessions, setFeatureAgentSessions] = useState<
    Record<string, TerminalSession | null>
  >({});
  const [featureActiveTabs, setFeatureActiveTabs] = useState<
    Record<string, FeatureDetailTab>
  >({});
  const [explorerTerminals, setExplorerTerminals] = useState<TerminalSession[]>([]);
  const [explorerActiveTab, setExplorerActiveTab] = useState("main");
  const [featuresExpanded, setFeaturesExpanded] = useState<Set<string>>(new Set());
  const [featuresActiveTerminal, setFeaturesActiveTerminal] = useState<
    Record<string, string>
  >({});
  const [featuresTerminalHeights, setFeaturesTerminalHeights] = useState<
    Record<string, number>
  >({});

  // Mirror of the live window state, refreshed every render, so callbacks can
  // snapshot the current workspace's windows without going stale.
  const live: PersistedWindowState & SessionWindowState = {
    openTabs,
    layout,
    splitSizes,
    activePaneId,
    featureTerminalTabs,
    featureAgentSessions,
    featureActiveTabs,
    explorerTerminals,
    explorerActiveTab,
    featuresExpanded,
    featuresActiveTerminal,
    featuresTerminalHeights,
  };
  const liveRef = useRef(live);
  liveRef.current = live;

  // Per-workspace window stores, keyed by workspace root. `persistedStore` is
  // also mirrored to localStorage; `sessionStore` is in-memory only.
  const persistedStoreRef = useRef<Record<string, PersistedWindowState>>(
    loadWorkspaceWindows()
  );
  const sessionStoreRef = useRef<Record<string, SessionWindowState>>({});
  const activeRootRef = useRef<string>("");

  const applyPersisted = useCallback((state: PersistedWindowState) => {
    setOpenTabs(state.openTabs);
    setLayout(state.layout);
    setSplitSizes(state.splitSizes);
    setActivePaneId(state.activePaneId);
  }, []);

  const applySession = useCallback((state: SessionWindowState) => {
    setFeatureTerminalTabs(state.featureTerminalTabs);
    setFeatureAgentSessions(state.featureAgentSessions);
    setFeatureActiveTabs(state.featureActiveTabs);
    setExplorerTerminals(state.explorerTerminals);
    setExplorerActiveTab(state.explorerActiveTab);
    setFeaturesExpanded(state.featuresExpanded);
    setFeaturesActiveTerminal(state.featuresActiveTerminal);
    setFeaturesTerminalHeights(state.featuresTerminalHeights);
  }, []);

  // Snapshot the outgoing workspace's windows and restore the incoming one's.
  // Returns true if a non-empty saved layout was restored.
  const switchWorkspaceWindows = useCallback(
    (newRoot: string): boolean => {
      const prevRoot = activeRootRef.current;
      if (prevRoot === newRoot) {
        return Object.keys(liveRef.current.openTabs).length > 0;
      }
      if (prevRoot) {
        const snap = liveRef.current;
        persistedStoreRef.current[prevRoot] = reconcileWindowState({
          openTabs: snap.openTabs,
          layout: snap.layout,
          splitSizes: snap.splitSizes,
          activePaneId: snap.activePaneId,
        });
        sessionStoreRef.current[prevRoot] = {
          featureTerminalTabs: snap.featureTerminalTabs,
          featureAgentSessions: snap.featureAgentSessions,
          featureActiveTabs: snap.featureActiveTabs,
          explorerTerminals: snap.explorerTerminals,
          explorerActiveTab: snap.explorerActiveTab,
          featuresExpanded: snap.featuresExpanded,
          featuresActiveTerminal: snap.featuresActiveTerminal,
          featuresTerminalHeights: snap.featuresTerminalHeights,
        };
        saveWorkspaceWindows(persistedStoreRef.current);
      }
      const savedP = newRoot ? persistedStoreRef.current[newRoot] : undefined;
      const savedS = newRoot ? sessionStoreRef.current[newRoot] : undefined;
      applyPersisted(savedP ?? freshPersisted());
      applySession(savedS ?? freshSession());
      setDraggedTab(null);
      setDropHint(null);
      activeRootRef.current = newRoot;
      return !!savedP && Object.keys(savedP.openTabs).length > 0;
    },
    [applyPersisted, applySession]
  );

  const opened = !!config;

  const refreshConfig = useCallback(async () => {
    try {
      const cfg = await api.GetConfig();
      setConfig(cfg);
      setRoot(cfg.Workspace.Root);
      // Restore this workspace's windows on first load; a no-op when the same
      // workspace is just re-read after a config change.
      switchWorkspaceWindows(cfg.Workspace.Root);
    } catch {
      setConfig(null);
    }
  }, [switchWorkspaceWindows]);

  useEffect(() => {
    (async () => {
      try {
        const current = await api.CurrentRoot();
        if (current) await refreshConfig();
      } catch {
        /* bindings not ready in pure browser preview */
      }
    })();
  }, [refreshConfig]);

  // Push the saved developer-mode preference to the backend log level on boot,
  // so program logs start at debug detail when developer mode is enabled.
  useEffect(() => {
    try {
      const on = localStorage.getItem("agentsafe.devMode") === "true";
      void api.SetLogLevel(on ? "debug" : "info").catch(() => {});
    } catch {
      /* bindings or localStorage unavailable */
    }
  }, []);

  useEffect(() => {
    try {
      localStorage.setItem("agentsafe.sidebarMode", sidebarMode);
    } catch {
      /* localStorage unavailable */
    }
  }, [sidebarMode]);

  useEffect(() => {
    setSplitSizes((prev) => normalizeSplitSizes(layout, prev));
  }, [layout]);

  // Persist the current workspace's windows (tabs/layout/splits) keyed by root.
  useEffect(() => {
    const root = activeRootRef.current;
    if (!root) return;
    persistedStoreRef.current[root] = reconcileWindowState({
      openTabs,
      layout,
      splitSizes,
      activePaneId,
    });
    saveWorkspaceWindows(persistedStoreRef.current);
  }, [openTabs, layout, splitSizes, activePaneId]);

  // After a workspace is (re)loaded, prune restored tabs for features that no
  // longer exist in this workspace.
  useEffect(() => {
    if (!root) return;
    void pruneStaleTabs();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [root]);

  const nav: Array<{ view: View; label: string; icon: LucideIcon }> = [
    { view: { kind: "workspace" }, label: t("nav.workspace"), icon: FolderGit2 },
    { view: { kind: "features" }, label: t("nav.features"), icon: LayoutGrid },
    { view: { kind: "templates" }, label: t("nav.worktreeTemplates"), icon: FileText },
    { view: { kind: "explorer" }, label: t("nav.fileExplorer"), icon: FolderOpen },
    { view: { kind: "agentsec" }, label: t("nav.agentSecurity"), icon: ShieldCheck },
    { view: { kind: "backups" }, label: t("nav.backups"), icon: Archive },
    { view: { kind: "history" }, label: t("nav.history"), icon: History },
    { view: { kind: "settings" }, label: t("nav.settings"), icon: Settings },
  ];

  function nextPaneId() {
    const id = `pane-${paneIdCounter.current}`;
    paneIdCounter.current += 1;
    return id;
  }

  function titleForView(view: View): string {
    switch (view.kind) {
      case "workspace":
        return t("header.workspace");
      case "features":
        return t("header.features");
      case "feature":
        return t("header.feature", { name: view.name });
      case "templates":
        return t("header.worktreeTemplates");
      case "explorer":
        return t("header.fileExplorer");
      case "agentsec":
        return t("header.agentSecurity");
      case "backups":
        return t("header.backups");
      case "history":
        return view.feature
          ? `${t("header.history")} · ${view.feature}`
          : t("header.history");
      case "settings":
        return t("header.settings");
      case "terminal":
        return view.title;
    }
  }

  function iconForView(view: View): LucideIcon {
    switch (view.kind) {
      case "workspace":
        return FolderGit2;
      case "features":
      case "feature":
        return LayoutGrid;
      case "templates":
        return FileText;
      case "explorer":
        return FolderOpen;
      case "agentsec":
        return ShieldCheck;
      case "backups":
        return Archive;
      case "history":
        return History;
      case "settings":
        return Settings;
      case "terminal":
        return Terminal;
    }
  }

  function moveTabToPane(
    currentLayout: PaneLayout,
    dragged: DraggedTab,
    targetPaneId: string,
    beforeTabId?: string
  ): PaneLayout {
    const next = cloneLayout(currentLayout);
    const targetPane = next.panes[targetPaneId] ?? next.panes[firstPaneId(next)];
    if (!targetPane) return emptyLayout();
    const resolvedTargetPaneId = targetPane.id;

    if (dragged.paneId === resolvedTargetPaneId) {
      targetPane.tabIds = insertTab(targetPane.tabIds, dragged.tabId, beforeTabId);
      targetPane.activeTabId = dragged.tabId;
      return cleanupLayout(next, resolvedTargetPaneId);
    }

    for (const paneId of Object.keys(next.panes)) {
      next.panes[paneId] = removeTabFromPane(next.panes[paneId], dragged.tabId);
    }

    const currentTargetPane = next.panes[resolvedTargetPaneId] ?? targetPane;
    next.panes[resolvedTargetPaneId] = {
      ...currentTargetPane,
      tabIds: insertTab(currentTargetPane.tabIds, dragged.tabId, beforeTabId),
      activeTabId: dragged.tabId,
    };
    return cleanupLayout(next, resolvedTargetPaneId);
  }

  function neighborPaneForEdge(currentLayout: PaneLayout, paneId: string, edge: DropEdge): string {
    const position = panePosition(currentLayout, paneId);
    if (!position) return paneId;
    const { rowIndex, columnIndex } = position;
    if (edge === "left") return currentLayout.rows[rowIndex][columnIndex - 1] ?? paneId;
    if (edge === "right") return currentLayout.rows[rowIndex][columnIndex + 1] ?? paneId;
    if (edge === "top") {
      const row = currentLayout.rows[rowIndex - 1];
      return row?.[Math.min(columnIndex, row.length - 1)] ?? paneId;
    }
    const row = currentLayout.rows[rowIndex + 1];
    return row?.[Math.min(columnIndex, row.length - 1)] ?? paneId;
  }

  function splitTabToEdge(
    currentLayout: PaneLayout,
    dragged: DraggedTab,
    targetPaneId: string,
    edge: DropEdge
  ) {
    const position = panePosition(currentLayout, targetPaneId);
    if (!position) return currentLayout;

    const { rowIndex } = position;
    const targetRow = currentLayout.rows[rowIndex];
    const paneCount = currentLayout.rows.flat().length;
    const canSplitHorizontal =
      (edge === "left" || edge === "right") && targetRow.length < 2 && paneCount < 4;
    const canSplitVertical =
      (edge === "top" || edge === "bottom") && currentLayout.rows.length < 2 && paneCount < 4;

    if (!canSplitHorizontal && !canSplitVertical) {
      const neighborPaneId = neighborPaneForEdge(currentLayout, targetPaneId, edge);
      return moveTabToPane(currentLayout, dragged, neighborPaneId);
    }

    const next = cloneLayout(currentLayout);
    for (const paneId of Object.keys(next.panes)) {
      next.panes[paneId] = removeTabFromPane(next.panes[paneId], dragged.tabId);
    }
    const newPaneId = nextPaneId();
    next.panes[newPaneId] = { id: newPaneId, tabIds: [dragged.tabId], activeTabId: dragged.tabId };

    if (edge === "left" || edge === "right") {
      const nextPosition = panePosition(next, targetPaneId);
      if (!nextPosition) return cleanupLayout(next, targetPaneId);
      const row = next.rows[nextPosition.rowIndex];
      const insertAt = edge === "left" ? nextPosition.columnIndex : nextPosition.columnIndex + 1;
      row.splice(insertAt, 0, newPaneId);
    } else {
      const nextPosition = panePosition(next, targetPaneId);
      if (!nextPosition) return cleanupLayout(next, targetPaneId);
      const insertAt = edge === "top" ? nextPosition.rowIndex : nextPosition.rowIndex + 1;
      next.rows.splice(insertAt, 0, [newPaneId]);
    }

    return cleanupLayout(next, targetPaneId);
  }

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

  const onWorkspaceLoaded = useCallback(
    (cfg: Config) => {
      const newRoot = cfg.Workspace.Root;
      const restored = switchWorkspaceWindows(newRoot);
      setConfig(cfg);
      setRoot(newRoot);
      // Only force the features view for a brand-new workspace; otherwise keep
      // the restored tabs/active tab as they were.
      if (!restored) openView({ kind: "features" }, { force: true });
    },
    [openView, switchWorkspaceWindows]
  );

  const onRemovedActive = useCallback(() => {
    const removed = activeRootRef.current;
    if (removed) {
      delete persistedStoreRef.current[removed];
      delete sessionStoreRef.current[removed];
      saveWorkspaceWindows(persistedStoreRef.current);
    }
    activeRootRef.current = "";
    setConfig(null);
    setRoot("");
    setOpenTabs({});
    setLayout(emptyLayout());
    setActivePaneId("pane-1");
    setDraggedTab(null);
    setDropHint(null);
    applySession(freshSession());
  }, [applySession]);

  function closeTabs(tabIds: string[]) {
    if (tabIds.length === 0) return;
    for (const tabId of tabIds) {
      const view = openTabs[tabId]?.view;
      if (view?.kind === "terminal") void api.TerminalClose(view.id);
    }
    setOpenTabs((prev) => {
      const next = { ...prev };
      for (const tabId of tabIds) delete next[tabId];
      return next;
    });
    setLayout((prev) => {
      const next = cloneLayout(prev);
      for (const currentPaneId of Object.keys(next.panes)) {
        let pane = next.panes[currentPaneId];
        for (const tabId of tabIds) pane = removeTabFromPane(pane, tabId);
        next.panes[currentPaneId] = pane;
      }
      const normalized = cleanupLayout(next);
      const fallbackPaneId = normalized.panes[activePaneId] ? activePaneId : firstPaneId(normalized);
      setActivePaneId(fallbackPaneId);
      return normalized;
    });
  }

  function closeTab(tabId: string) {
    closeTabs([tabId]);
  }

  // Drop restored feature/history tabs whose feature no longer exists.
  async function pruneStaleTabs() {
    try {
      const res = await api.ListFeatures();
      const names = new Set((res.features ?? []).map((f) => f.name));
      const staleIds = Object.values(liveRef.current.openTabs)
        .filter((tab) => {
          if (tab.view.kind === "feature") return !names.has(tab.view.name);
          if (tab.view.kind === "history")
            return tab.view.feature ? !names.has(tab.view.feature) : false;
          return false;
        })
        .map((tab) => tab.id);
      closeTabs(staleIds);
    } catch {
      /* feature list unavailable; leave tabs as-is */
    }
  }

  function nextSidebarMode() {
    setSidebarMode((mode) =>
      mode === "full" ? "icons" : mode === "icons" ? "hidden" : "full"
    );
  }

  function navIsActive(navView: View, activeView: View): boolean {
    if (navView.kind === activeView.kind) return true;
    return navView.kind === "features" && activeView.kind === "feature";
  }

  const sidebarFull = sidebarMode === "full";
  const sidebarIcons = sidebarMode === "icons";
  const sidebarHidden = sidebarMode === "hidden";
  const sidebarToggleLabel =
    sidebarMode === "full"
      ? t("sidebar.collapseToIcons")
      : sidebarMode === "icons"
        ? t("sidebar.hide")
        : t("sidebar.expand");

  function sanitizeTabsForClosedWorkspace() {
    if (opened) return;
    setOpenTabs((prev) => {
      const next = Object.fromEntries(
        Object.entries(prev).filter(([, tab]) => !isWorkspaceDependent(tab.view))
      );
      return next;
    });
    setLayout((prev) => {
      const next = cloneLayout(prev);
      for (const paneId of Object.keys(next.panes)) {
        const pane = next.panes[paneId];
        const tabIds: string[] = pane.tabIds.filter(
          (id) => id === "workspace" || id === "settings"
        );
        next.panes[paneId] = {
          ...pane,
          tabIds,
          activeTabId: tabIds.includes(pane.activeTabId) ? pane.activeTabId : tabIds[0] ?? "",
        };
      }
      const normalized = cleanupLayout(next, activePaneId);
      const fallbackPaneId = firstPaneId(normalized);
      setActivePaneId(fallbackPaneId);
      return normalized;
    });
  }

  useEffect(() => {
    sanitizeTabsForClosedWorkspace();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [opened]);

  function renderView(view: View) {
    switch (view.kind) {
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
      case "features":
        return opened ? (
          <FeaturesPage
            onOpen={(name) => openView({ kind: "feature", name })}
            terminalTabs={featureTerminalTabs}
            setTerminalTabs={setFeatureTerminalTabs}
            expanded={featuresExpanded}
            setExpanded={setFeaturesExpanded}
            activeTerminal={featuresActiveTerminal}
            setActiveTerminal={setFeaturesActiveTerminal}
            heights={featuresTerminalHeights}
            setHeights={setFeaturesTerminalHeights}
          />
        ) : null;
      case "feature":
        return (
          <FeatureDetailPage
            name={view.name}
            onBack={() => openView({ kind: "features" })}
            onViewHistory={(feature) => openView({ kind: "history", feature })}
            tab={featureActiveTabs[view.name] ?? "work"}
            setTab={(next) =>
              setFeatureActiveTabs((prev) => {
                const current = prev[view.name] ?? "work";
                const value =
                  typeof next === "function" ? next(current as FeatureDetailTab) : next;
                return { ...prev, [view.name]: value };
              })
            }
            terminalTabs={featureTerminalTabs[view.name] ?? []}
            setTerminalTabs={(next) =>
              setFeatureTerminalTabs((prev) => {
                const current = prev[view.name] ?? [];
                const value = typeof next === "function" ? next(current) : next;
                return { ...prev, [view.name]: value };
              })
            }
            agentSession={featureAgentSessions[view.name] ?? null}
            setAgentSession={(next) =>
              setFeatureAgentSessions((prev) => {
                const current = prev[view.name] ?? null;
                const value = typeof next === "function" ? next(current) : next;
                return { ...prev, [view.name]: value };
              })
            }
          />
        );
      case "templates":
        return <WorktreeTemplatesPage config={config} />;
      case "explorer":
        return (
          <FileExplorerPage
            config={config}
            terminals={explorerTerminals}
            setTerminals={setExplorerTerminals}
            activeTab={explorerActiveTab}
            setActiveTab={setExplorerActiveTab}
          />
        );
      case "agentsec":
        return <AgentSecurityPage config={config} />;
      case "history":
        return <HistoryPage config={config} feature={view.feature} />;
      case "backups":
        return <BackupsPage config={config} />;
      case "settings":
        return <SettingsPage config={config} onChanged={refreshConfig} />;
      case "terminal":
        return <TerminalPanel id={view.id} path={view.path} className="flex h-full flex-col" />;
    }
  }

  function beginTabDrag(e: DragEvent<HTMLElement>, tabId: string, paneId: string) {
    e.dataTransfer.effectAllowed = "move";
    e.dataTransfer.setData("text/plain", tabId);
    setDraggedTab({ tabId, paneId });
  }

  function dropTabOnTab(e: DragEvent<HTMLElement>, paneId: string, beforeTabId: string) {
    e.preventDefault();
    e.stopPropagation();
    if (!draggedTab) return;
    setLayout((prev) => {
      const next = moveTabToPane(prev, draggedTab, paneId, beforeTabId);
      setActivePaneId(paneId);
      return next;
    });
    setDraggedTab(null);
    setDropHint(null);
  }

  function dropTabOnTabBar(e: DragEvent<HTMLElement>, paneId: string) {
    e.preventDefault();
    e.stopPropagation();
    if (!draggedTab) return;
    setLayout((prev) => {
      const next = moveTabToPane(prev, draggedTab, paneId);
      setActivePaneId(paneId);
      return next;
    });
    setDraggedTab(null);
    setDropHint(null);
  }

  function dragOverPane(e: DragEvent<HTMLElement>, paneId: string) {
    if (!draggedTab) return;
    e.preventDefault();
    const edge = computeDropEdge(e);
    setDropHint(edge ? { paneId, edge } : null);
  }

  function dropTabOnPane(e: DragEvent<HTMLElement>, paneId: string) {
    if (!draggedTab) return;
    e.preventDefault();
    const edge = computeDropEdge(e);
    setLayout((prev) => {
      const next = edge
        ? splitTabToEdge(prev, draggedTab, paneId, edge)
        : moveTabToPane(prev, draggedTab, paneId);
      const targetPaneId = paneContainingTab(next, draggedTab.tabId) ?? paneId;
      setActivePaneId(next.panes[targetPaneId] ? targetPaneId : firstPaneId(next));
      return next;
    });
    setDraggedTab(null);
    setDropHint(null);
  }

  function startRowResize(e: ReactPointerEvent<HTMLDivElement>) {
    e.preventDefault();
    const container = e.currentTarget.parentElement;
    if (!container) return;
    const rect = container.getBoundingClientRect();
    const onMove = (ev: PointerEvent) => {
      const next = clampSplit(((ev.clientY - rect.top) / rect.height) * 100);
      setSplitSizes((prev) => ({
        ...prev,
        rowSizes: [next, 100 - next],
      }));
    };
    const onUp = () => {
      window.removeEventListener("pointermove", onMove);
      window.removeEventListener("pointerup", onUp);
    };
    window.addEventListener("pointermove", onMove);
    window.addEventListener("pointerup", onUp);
  }

  function startColumnResize(rowIndex: number, e: ReactPointerEvent<HTMLDivElement>) {
    e.preventDefault();
    const row = e.currentTarget.parentElement;
    if (!row) return;
    const rect = row.getBoundingClientRect();
    const key = String(rowIndex);
    const onMove = (ev: PointerEvent) => {
      const next = clampSplit(((ev.clientX - rect.left) / rect.width) * 100);
      setSplitSizes((prev) => ({
        ...prev,
        columnSizesByRow: {
          ...prev.columnSizesByRow,
          [key]: [next, 100 - next],
        },
      }));
    };
    const onUp = () => {
      window.removeEventListener("pointermove", onMove);
      window.removeEventListener("pointerup", onUp);
    };
    window.addEventListener("pointermove", onMove);
    window.addEventListener("pointerup", onUp);
  }

  function renderDropHint(paneId: string) {
    if (!dropHint || dropHint.paneId !== paneId) return null;
    return (
      <div className="pointer-events-none absolute inset-0 z-20 bg-primary/5">
        <div
          className={cn(
            "absolute rounded-md border-2 border-primary bg-primary/15",
            dropHint.edge === "left" && "inset-y-2 left-2 w-1/4",
            dropHint.edge === "right" && "inset-y-2 right-2 w-1/4",
            dropHint.edge === "top" && "inset-x-2 top-2 h-1/4",
            dropHint.edge === "bottom" && "inset-x-2 bottom-2 h-1/4"
          )}
        />
      </div>
    );
  }

  function renderPane(pane: PaneModel, style?: CSSProperties) {
    const paneActive = activePaneId === pane.id;
    const activeTabId = pane.tabIds.includes(pane.activeTabId) ? pane.activeTabId : pane.tabIds[0];
    const activeTab = activeTabId ? openTabs[activeTabId] : undefined;
    return (
      <section
        key={pane.id}
        data-terminal-fullscreen-root
        style={style}
        className={cn(
          "relative flex h-full min-h-0 min-w-0 shrink-0 grow-0 flex-col overflow-hidden rounded-md border bg-background",
          paneActive && "ring-1 ring-primary/50"
        )}
        onMouseDown={() => setActivePaneId(pane.id)}
        onDragOver={(e) => dragOverPane(e, pane.id)}
        onDragLeave={(e) => {
          if (!e.currentTarget.contains(e.relatedTarget as Node | null)) setDropHint(null);
        }}
        onDrop={(e) => dropTabOnPane(e, pane.id)}
      >
        {renderDropHint(pane.id)}
        <div
          className="flex shrink-0 items-end gap-1 overflow-x-auto border-b bg-card px-3 pt-2"
          onDragOver={(e) => {
            if (draggedTab) e.preventDefault();
          }}
          onDrop={(e) => dropTabOnTabBar(e, pane.id)}
        >
          {pane.tabIds.map((tabId) => {
            const tab = openTabs[tabId];
            if (!tab) return null;
            const Icon = iconForView(tab.view);
            const active = activeTabId === tab.id;
            return (
              <div
                key={tab.id}
                draggable
                onDragStart={(e) => beginTabDrag(e, tab.id, pane.id)}
                onDragEnd={() => {
                  setDraggedTab(null);
                  setDropHint(null);
                }}
                onDragOver={(e) => {
                  if (draggedTab) e.preventDefault();
                }}
                onDrop={(e) => dropTabOnTab(e, pane.id, tab.id)}
                className={cn(
                  "group flex max-w-64 cursor-grab items-center gap-1 rounded-t-md border border-b-0 px-2 py-1.5 text-sm active:cursor-grabbing",
                  active ? "bg-background font-medium" : "bg-muted/30 text-muted-foreground"
                )}
                title={titleForView(tab.view)}
              >
                <button
                  type="button"
                  className="flex min-w-0 items-center gap-1.5"
                  onClick={() => {
                    setActivePaneId(pane.id);
                    setLayout((prev) => ({
                      ...prev,
                      panes: {
                        ...prev.panes,
                        [pane.id]: { ...prev.panes[pane.id], activeTabId: tab.id },
                      },
                    }));
                  }}
                >
                  <Icon className="size-3.5 shrink-0" />
                  <span className="truncate">{titleForView(tab.view)}</span>
                </button>
                {tab.closable && (
                  <button
                    type="button"
                    className="rounded p-0.5 opacity-60 hover:bg-accent hover:opacity-100"
                    onMouseDown={(e) => e.stopPropagation()}
                    onClick={(e) => {
                      e.stopPropagation();
                      closeTab(tab.id);
                    }}
                    title={t("common.close")}
                  >
                    <X className="size-3.5" />
                  </button>
                )}
              </div>
            );
          })}
        </div>
        <div className="min-h-0 flex-1 overflow-auto p-6">
          {activeTab ? (
            renderView(activeTab.view)
          ) : (
            <div className="flex h-full items-center justify-center rounded-md border border-dashed text-sm text-muted-foreground">
              열린 탭이 없습니다. 좌측 메뉴에서 열거나 탭을 드래그하세요.
            </div>
          )}
        </div>
      </section>
    );
  }

  const activePane = layout.panes[activePaneId] ?? layout.panes[firstPaneId(layout)];
  const activeTab = activePane?.activeTabId ? openTabs[activePane.activeTabId] : undefined;
  const effectiveSplitSizes = normalizeSplitSizes(layout, splitSizes);

  return (
    <div className="flex h-screen w-screen overflow-hidden bg-background text-foreground">
      <aside
        className={cn(
          "relative flex shrink-0 flex-col bg-card transition-[width] duration-300 ease-in-out",
          sidebarHidden ? "w-0 border-r-0" : "border-r",
          sidebarFull && "w-56",
          sidebarIcons && "w-16"
        )}
      >
        {!sidebarHidden && (
          <div className="flex min-h-0 flex-1 flex-col overflow-hidden pb-16">
            <div
              className={cn(
                "flex items-center gap-2 py-4",
                sidebarFull ? "px-4" : "justify-center px-2"
              )}
            >
              <img
                src={agentsafeLogo}
                alt=""
                className="size-7 object-contain"
                aria-hidden="true"
              />
              {sidebarFull && <span className="text-base font-semibold">agentsafe</span>}
            </div>
            {sidebarFull && (
              <div className="pb-3">
                <WorkspaceSwitcher
                  config={config}
                  onSwitched={onWorkspaceLoaded}
                  onRemovedActive={onRemovedActive}
                />
              </div>
            )}
            <nav
              className={cn(
                "flex flex-col gap-1",
                sidebarFull ? "px-2" : "items-center px-2"
              )}
            >
              {nav.map((item) => {
                const disabled = isWorkspaceDependent(item.view) && !opened;
                const active = activeTab ? navIsActive(item.view, activeTab.view) : false;
                return (
                  <button
                    key={viewId(item.view)}
                    disabled={disabled}
                    title={sidebarIcons ? item.label : undefined}
                    aria-label={item.label}
                    onClick={() => openView(item.view)}
                    className={cn(
                      "flex items-center rounded-md text-sm transition-colors",
                      sidebarFull ? "w-full gap-2 px-3 py-2" : "size-10 justify-center",
                      active
                        ? "bg-secondary font-medium text-secondary-foreground"
                        : "text-muted-foreground hover:bg-accent hover:text-accent-foreground",
                      disabled && "cursor-not-allowed opacity-40 hover:bg-transparent"
                    )}
                  >
                    <item.icon className="size-4 shrink-0" />
                    {sidebarFull && <span className="truncate">{item.label}</span>}
                  </button>
                );
              })}
            </nav>
          </div>
        )}
      </aside>

      <main className="flex min-w-0 flex-1 flex-col overflow-hidden">
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

        <div className="flex min-h-0 flex-1 flex-col p-2">
          {layout.rows.map((row, rowIndex) => {
            const rowSize = effectiveSplitSizes.rowSizes[rowIndex] ?? 100;
            const columnSizes =
              effectiveSplitSizes.columnSizesByRow[String(rowIndex)] ?? [100];
            return (
              <Fragment key={row.join("-") || rowIndex}>
                <div
                  className="flex min-h-0 shrink-0 grow-0"
                  style={{ flexBasis: `${rowSize}%` }}
                >
                  {row.map((paneId, columnIndex) => {
                    const pane = layout.panes[paneId];
                    const columnSize = columnSizes[columnIndex] ?? 100;
                    return (
                      <Fragment key={paneId}>
                        {pane
                          ? renderPane(pane, { flexBasis: `${columnSize}%` })
                          : null}
                        {columnIndex < row.length - 1 && (
                          <div
                            role="separator"
                            aria-orientation="vertical"
                            className="group flex w-2 shrink-0 cursor-col-resize items-center justify-center"
                            title={t("split.resizeColumn")}
                            onPointerDown={(e) => startColumnResize(rowIndex, e)}
                          >
                            <div className="h-full w-0.5 rounded bg-border group-hover:bg-primary/60" />
                          </div>
                        )}
                      </Fragment>
                    );
                  })}
                </div>
                {rowIndex < layout.rows.length - 1 && (
                  <div
                    role="separator"
                    aria-orientation="horizontal"
                    className="group flex h-2 shrink-0 cursor-row-resize items-center justify-center"
                    title={t("split.resizeRow")}
                    onPointerDown={startRowResize}
                  >
                    <div className="h-0.5 w-full rounded bg-border group-hover:bg-primary/60" />
                  </div>
                )}
              </Fragment>
            );
          })}
        </div>
      </main>
    </div>
  );
}
