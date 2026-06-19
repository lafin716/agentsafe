import { useCallback, useEffect, useState } from "react";
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

export default function App() {
  const { notify } = useToast();
  const { t } = useI18n();
  const [config, setConfig] = useState<Config | null>(null);
  const [root, setRoot] = useState("");
  const [view, setView] = useState<View>({ kind: "workspace" });
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
  // Worktree-list inline terminal UI state. Lifted to App level so the inline
  // panel (and its tabs) survive navigation away from the features list.
  const [featuresExpanded, setFeaturesExpanded] = useState<Set<string>>(new Set());
  const [featuresActiveTerminal, setFeaturesActiveTerminal] = useState<
    Record<string, string>
  >({});
  const [featuresTerminalHeights, setFeaturesTerminalHeights] = useState<
    Record<string, number>
  >({});

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
    // On launch, try to restore an already-open workspace (if any).
    (async () => {
      try {
        const current = await api.CurrentRoot();
        if (current) await refreshConfig();
      } catch {
        /* bindings not ready in pure browser preview */
      }
    })();
  }, [refreshConfig]);

  const onWorkspaceLoaded = useCallback((cfg: Config) => {
    setConfig(cfg);
    setRoot(cfg.Workspace.Root);
    setView({ kind: "features" });
  }, []);

  const onRemovedActive = useCallback(() => {
    setConfig(null);
    setRoot("");
    setView({ kind: "workspace" });
  }, []);

  const opened = !!config;
  const sidebarFull = sidebarMode === "full";
  const sidebarIcons = sidebarMode === "icons";
  const sidebarHidden = sidebarMode === "hidden";

  useEffect(() => {
    try {
      localStorage.setItem("agentsafe.sidebarMode", sidebarMode);
    } catch {
      /* localStorage unavailable */
    }
  }, [sidebarMode]);

  function nextSidebarMode() {
    setSidebarMode((mode) =>
      mode === "full" ? "icons" : mode === "icons" ? "hidden" : "full"
    );
  }

  const sidebarToggleLabel =
    sidebarMode === "full"
      ? t("sidebar.collapseToIcons")
      : sidebarMode === "icons"
        ? t("sidebar.hide")
        : t("sidebar.expand");

  const nav = [
    { id: "workspace" as const, label: t("nav.workspace"), icon: FolderGit2 },
    { id: "features" as const, label: t("nav.features"), icon: LayoutGrid },
    { id: "templates" as const, label: t("nav.worktreeTemplates"), icon: FileText },
    { id: "explorer" as const, label: t("nav.fileExplorer"), icon: FolderOpen },
    {
      id: "agentsec" as const,
      label: t("nav.agentSecurity"),
      icon: ShieldCheck,
    },
    { id: "backups" as const, label: t("nav.backups"), icon: Archive },
    { id: "history" as const, label: t("nav.history"), icon: History },
    { id: "settings" as const, label: t("nav.settings"), icon: Settings },
  ];

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
                const active =
                  view.kind === item.id ||
                  (item.id === "features" && view.kind === "feature");
                const disabled =
                  (item.id === "features" ||
                    item.id === "templates" ||
                    item.id === "explorer" ||
                    item.id === "agentsec" ||
                    item.id === "backups" ||
                    item.id === "history") &&
                  !opened;
                return (
                  <button
                    key={item.id}
                    disabled={disabled}
                    title={sidebarIcons ? item.label : undefined}
                    aria-label={item.label}
                    onClick={() => setView({ kind: item.id })}
                    className={cn(
                      "flex items-center rounded-md text-sm transition-colors",
                      sidebarFull
                        ? "w-full gap-2 px-3 py-2"
                        : "size-10 justify-center",
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

      <main className="flex flex-1 flex-col overflow-hidden">
        <header className="flex items-center justify-between border-b px-6 py-3">
          <h1 className="text-lg font-semibold">
            {view.kind === "workspace" && t("header.workspace")}
            {view.kind === "features" && t("header.features")}
            {view.kind === "feature" && t("header.feature", { name: view.name })}
            {view.kind === "templates" && t("header.worktreeTemplates")}
            {view.kind === "explorer" && t("header.fileExplorer")}
            {view.kind === "agentsec" && t("header.agentSecurity")}
            {view.kind === "backups" && t("header.backups")}
            {view.kind === "history" && t("header.history")}
            {view.kind === "settings" && t("header.settings")}
          </h1>
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

        <div className="flex-1 overflow-auto p-6">
          {view.kind === "workspace" && (
            <WorkspacePage
              config={config}
              root={root}
              onLoaded={onWorkspaceLoaded}
              onChanged={refreshConfig}
            />
          )}
          {view.kind === "features" && opened && (
            <FeaturesPage
              onOpen={(name) => setView({ kind: "feature", name })}
              terminalTabs={featureTerminalTabs}
              setTerminalTabs={setFeatureTerminalTabs}
              expanded={featuresExpanded}
              setExpanded={setFeaturesExpanded}
              activeTerminal={featuresActiveTerminal}
              setActiveTerminal={setFeaturesActiveTerminal}
              heights={featuresTerminalHeights}
              setHeights={setFeaturesTerminalHeights}
            />
          )}
          {view.kind === "feature" && (
            <FeatureDetailPage
              name={view.name}
              onBack={() => setView({ kind: "features" })}
              onViewHistory={(feature) => setView({ kind: "history", feature })}
              tab={featureActiveTabs[view.name] ?? "work"}
              setTab={(next) =>
                setFeatureActiveTabs((prev) => {
                  const current = prev[view.name] ?? "work";
                  const value =
                    typeof next === "function"
                      ? next(current as FeatureDetailTab)
                      : next;
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
          )}
          {view.kind === "templates" && <WorktreeTemplatesPage config={config} />}
          {view.kind === "explorer" && (
            <FileExplorerPage
              config={config}
              terminals={explorerTerminals}
              setTerminals={setExplorerTerminals}
              activeTab={explorerActiveTab}
              setActiveTab={setExplorerActiveTab}
            />
          )}
          {view.kind === "agentsec" && <AgentSecurityPage config={config} />}
          {view.kind === "history" && (
            <HistoryPage
              config={config}
              feature={view.kind === "history" ? view.feature : undefined}
            />
          )}
          {view.kind === "backups" && <BackupsPage config={config} />}
          {view.kind === "settings" && (
            <SettingsPage config={config} onChanged={refreshConfig} />
          )}
        </div>
      </main>
    </div>
  );
}
