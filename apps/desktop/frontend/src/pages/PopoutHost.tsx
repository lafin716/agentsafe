import { useCallback, useEffect, useMemo, useState } from "react";
import { Loader2 } from "lucide-react";
import type { View } from "@/App";
import type { Config, TerminalSession } from "@/lib/types";
import { api } from "@/lib/api";
import { TerminalPanel } from "@/components/TerminalPanel";
import { WorkspacePage } from "@/pages/WorkspacePage";
import { FeaturesPage } from "@/pages/FeaturesPage";
import { FeatureDetailPage, type FeatureDetailTab } from "@/pages/FeatureDetailPage";
import { FileExplorerPage } from "@/pages/FileExplorerPage";
import { SettingsPage } from "@/pages/SettingsPage";
import { AgentSecurityPage } from "@/pages/AgentSecurityPage";
import { BackupsPage } from "@/pages/BackupsPage";
import { HistoryPage } from "@/pages/HistoryPage";
import { WorktreeTemplatesPage } from "@/pages/WorktreeTemplatesPage";

function titleForView(view: View): string {
  switch (view.kind) {
    case "feature":
      return view.name;
    case "history":
      return view.feature ? `History · ${view.feature}` : "History";
    case "terminal":
      return view.title || "Terminal";
    default:
      return view.kind;
  }
}

// PopoutHost renders a single detached view inside a popout window. It reuses
// the same page components as the main window (App.renderView) but supplies
// their session state locally, since the popout is an independent window backed
// by the same Go process over the bridge. Internal navigation swaps the view.
export function PopoutHost({ initial }: { initial: View }) {
  const [view, setView] = useState<View>(initial);
  const [config, setConfig] = useState<Config | null>(null);
  const [configLoaded, setConfigLoaded] = useState(false);

  // Per-view session state (local to this window).
  const [featureTab, setFeatureTab] = useState<FeatureDetailTab>("work");
  const [featureTerminals, setFeatureTerminals] = useState<TerminalSession[]>([]);
  const [featureAgentSession, setFeatureAgentSession] = useState<TerminalSession | null>(null);
  const [featuresTerminals, setFeaturesTerminals] = useState<Record<string, TerminalSession[]>>({});
  const [featuresExpanded, setFeaturesExpanded] = useState<Set<string>>(new Set());
  const [featuresActiveTerminal, setFeaturesActiveTerminal] = useState<Record<string, string>>({});
  const [featuresHeights, setFeaturesHeights] = useState<Record<string, number>>({});
  const [explorerTerminals, setExplorerTerminals] = useState<TerminalSession[]>([]);
  const [explorerActiveTab, setExplorerActiveTab] = useState("main");

  const refreshConfig = useCallback(async () => {
    try {
      setConfig(await api.GetConfig());
    } catch {
      /* no workspace open is a valid state */
    } finally {
      setConfigLoaded(true);
    }
  }, []);

  useEffect(() => {
    void refreshConfig();
  }, [refreshConfig]);

  useEffect(() => {
    document.title = `${titleForView(view)} · agentsafe`;
  }, [view]);

  const root = useMemo(() => config?.Workspace?.Root ?? "", [config]);

  // A terminal view needs no workspace/config; render it immediately so a
  // detached terminal works even before the config round-trip resolves.
  if (view.kind === "terminal") {
    return (
      <div className="h-screen w-screen overflow-hidden bg-background text-foreground">
        <TerminalPanel id={view.id} path={view.path} className="flex h-full flex-col" />
      </div>
    );
  }

  if (!configLoaded) {
    return (
      <div className="flex h-screen w-screen items-center justify-center gap-2 bg-background text-sm text-muted-foreground">
        <Loader2 className="size-5 animate-spin" /> Loading…
      </div>
    );
  }

  function body() {
    switch (view.kind) {
      case "workspace":
        return (
          <WorkspacePage
            config={config}
            root={root}
            onLoaded={(cfg) => setConfig(cfg)}
            onChanged={refreshConfig}
            onOpenTerminal={(s) =>
              setView({ kind: "terminal", id: s.id, path: s.path, title: s.title })
            }
          />
        );
      case "features":
        return (
          <FeaturesPage
            onOpen={(name) => setView({ kind: "feature", name })}
            terminalTabs={featuresTerminals}
            setTerminalTabs={setFeaturesTerminals}
            expanded={featuresExpanded}
            setExpanded={setFeaturesExpanded}
            activeTerminal={featuresActiveTerminal}
            setActiveTerminal={setFeaturesActiveTerminal}
            heights={featuresHeights}
            setHeights={setFeaturesHeights}
          />
        );
      case "feature":
        return (
          <FeatureDetailPage
            name={view.name}
            onBack={() => setView({ kind: "features" })}
            onViewHistory={(feature) => setView({ kind: "history", feature })}
            tab={featureTab}
            setTab={setFeatureTab}
            terminalTabs={featureTerminals}
            setTerminalTabs={setFeatureTerminals}
            agentSession={featureAgentSession}
            setAgentSession={setFeatureAgentSession}
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

  return (
    <div className="h-screen w-screen overflow-auto bg-background p-6 text-foreground">
      {body()}
    </div>
  );
}
