import { useCallback, useEffect, useState } from "react";
import {
  Archive,
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
import type { Config } from "@/lib/types";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { useToast } from "@/components/ui/toast";
import { useI18n } from "@/i18n/I18nProvider";
import { WorkspaceSwitcher } from "@/components/WorkspaceSwitcher";
import { WorkspacePage } from "@/pages/WorkspacePage";
import { FeaturesPage } from "@/pages/FeaturesPage";
import { FeatureDetailPage } from "@/pages/FeatureDetailPage";
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

export default function App() {
  const { notify } = useToast();
  const { t } = useI18n();
  const [config, setConfig] = useState<Config | null>(null);
  const [root, setRoot] = useState("");
  const [view, setView] = useState<View>({ kind: "workspace" });

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
      <aside className="flex w-56 shrink-0 flex-col border-r bg-card">
        <div className="flex items-center gap-2 px-4 py-4">
          <img
            src={agentsafeLogo}
            alt=""
            className="size-7 object-contain"
            aria-hidden="true"
          />
          <span className="text-base font-semibold">agentsafe</span>
        </div>
        <div className="pb-3">
          <WorkspaceSwitcher
            config={config}
            onSwitched={onWorkspaceLoaded}
            onRemovedActive={onRemovedActive}
          />
        </div>
        <nav className="flex flex-col gap-1 px-2">
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
                onClick={() => setView({ kind: item.id })}
                className={cn(
                  "flex items-center gap-2 rounded-md px-3 py-2 text-sm transition-colors",
                  active
                    ? "bg-secondary font-medium text-secondary-foreground"
                    : "text-muted-foreground hover:bg-accent hover:text-accent-foreground",
                  disabled && "cursor-not-allowed opacity-40 hover:bg-transparent"
                )}
              >
                <item.icon className="size-4" />
                {item.label}
              </button>
            );
          })}
        </nav>
      </aside>

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
            <FeaturesPage onOpen={(name) => setView({ kind: "feature", name })} />
          )}
          {view.kind === "feature" && (
            <FeatureDetailPage
              name={view.name}
              onBack={() => setView({ kind: "features" })}
              onViewHistory={(feature) => setView({ kind: "history", feature })}
            />
          )}
          {view.kind === "templates" && <WorktreeTemplatesPage config={config} />}
          {view.kind === "explorer" && <FileExplorerPage config={config} />}
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
