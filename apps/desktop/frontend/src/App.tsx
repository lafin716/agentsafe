import { useCallback, useEffect, useRef, useState, type DragEvent } from "react";
import {
  Archive,
  ChevronLeft,
  ChevronRight,
  ChevronsLeft,
  FileText,
  FolderGit2,
  FolderOpen,
  History,
  LayoutGrid,
  RefreshCw,
  Settings,
  ShieldCheck,
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
  | { kind: "settings" };

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

type DraggedTab = {
  tabId: string;
  paneId: string;
};

type DropHint = {
  paneId: string;
  edge: DropEdge;
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

  const opened = !!config;

  const refreshConfig = useCallback(async () => {
    try {
      const cfg = await api.GetConfig();
      setConfig(cfg);
      setRoot(cfg.Workspace.Root);
    } catch {
      setConfig(null);
    }
  }, []);

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

  useEffect(() => {
    try {
      localStorage.setItem("agentsafe.sidebarMode", sidebarMode);
    } catch {
      /* localStorage unavailable */
    }
  }, [sidebarMode]);

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
      setOpenTabs((prev) =>
        ({
          ...prev,
          [id]: prev[id] ?? { id, view, closable: true },
        })
      );
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
    [activePaneId, opened]
  );

  const onWorkspaceLoaded = useCallback(
    (cfg: Config) => {
      setConfig(cfg);
      setRoot(cfg.Workspace.Root);
      openView({ kind: "features" }, { force: true });
    },
    [openView]
  );

  const onRemovedActive = useCallback(() => {
    setConfig(null);
    setRoot("");
    setOpenTabs({});
    setLayout(emptyLayout());
    setActivePaneId("pane-1");
    setDraggedTab(null);
    setDropHint(null);
  }, []);

  function closeTab(tabId: string, paneId: string) {
    setOpenTabs((prev) => {
      const next = { ...prev };
      delete next[tabId];
      return next;
    });
    setLayout((prev) => {
      const next = cloneLayout(prev);
      for (const currentPaneId of Object.keys(next.panes)) {
        next.panes[currentPaneId] = removeTabFromPane(next.panes[currentPaneId], tabId);
      }
      const normalized = cleanupLayout(next, paneId);
      const fallbackPaneId = normalized.panes[activePaneId] ? activePaneId : firstPaneId(normalized);
      setActivePaneId(fallbackPaneId);
      return normalized;
    });
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

  function renderPane(pane: PaneModel) {
    const paneActive = activePaneId === pane.id;
    const activeTabId = pane.tabIds.includes(pane.activeTabId) ? pane.activeTabId : pane.tabIds[0];
    const activeTab = activeTabId ? openTabs[activeTabId] : undefined;
    return (
      <section
        key={pane.id}
        data-terminal-fullscreen-root
        className={cn(
          "relative flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden rounded-md border bg-background",
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
                      closeTab(tab.id, pane.id);
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
  const ActiveIcon = activeTab ? iconForView(activeTab.view) : FolderOpen;

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
        {!sidebarHidden && (
          <button
            type="button"
            onClick={nextSidebarMode}
            title={sidebarToggleLabel}
            aria-label={sidebarToggleLabel}
            className={cn(
              "absolute bottom-4 z-20 flex h-10 items-center justify-center rounded-lg border bg-background text-muted-foreground shadow-sm transition-colors hover:bg-accent hover:text-accent-foreground",
              sidebarFull ? "left-4 right-4 gap-2 px-3" : "left-3 size-10"
            )}
          >
            {sidebarFull ? (
              <>
                <ChevronLeft className="size-4" />
                <span className="text-xs font-medium">{t("sidebar.collapse")}</span>
              </>
            ) : (
              <ChevronsLeft className="size-4" />
            )}
          </button>
        )}
      </aside>
      {sidebarHidden && (
        <div className="group fixed bottom-6 left-0 z-40 flex h-14 w-10 items-center">
          <button
            type="button"
            onClick={nextSidebarMode}
            title={sidebarToggleLabel}
            aria-label={sidebarToggleLabel}
            className="flex h-10 w-12 -translate-x-8 items-center justify-center rounded-r-lg border bg-background text-muted-foreground shadow-md transition-transform duration-200 ease-out hover:bg-accent hover:text-accent-foreground group-hover:translate-x-0 focus:translate-x-0 focus:outline-none focus:ring-2 focus:ring-ring"
          >
            <ChevronRight className="size-4" />
          </button>
        </div>
      )}

      <main className="flex min-w-0 flex-1 flex-col overflow-hidden">
        <header className="flex items-center justify-between border-b px-6 py-3">
          <div className="flex min-w-0 items-center gap-2">
            <ActiveIcon className="size-5 shrink-0 text-muted-foreground" />
            <h1 className="truncate text-lg font-semibold">
              {activeTab ? titleForView(activeTab.view) : "열린 탭 없음"}
            </h1>
          </div>
          {opened && (
            <Button
              variant="outline"
              size="sm"
              onClick={async () => {
                try {
                  await refreshConfig();
                  notify(t("toast.reloaded"), "success");
                } catch (e) {
                  notify(errMessage(e), "error");
                }
              }}
            >
              <RefreshCw className="size-4" /> {t("common.reload")}
            </Button>
          )}
        </header>

        <div className="flex min-h-0 flex-1 flex-col gap-2 p-2">
          {layout.rows.map((row, rowIndex) => (
            <div key={row.join("-") || rowIndex} className="flex min-h-0 flex-1 gap-2">
              {row.map((paneId) => {
                const pane = layout.panes[paneId];
                return pane ? renderPane(pane) : null;
              })}
            </div>
          ))}
        </div>
      </main>
    </div>
  );
}
