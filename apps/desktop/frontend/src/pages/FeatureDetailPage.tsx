import {
  type Dispatch,
  type SetStateAction,
  useCallback,
  useEffect,
  useRef,
  useState,
} from "react";
import {
  AlertTriangle,
  AppWindow,
  ArrowLeft,
  Boxes,
  ChevronDown,
  ChevronRight,
  Copy,
  ExternalLink,
  Eye,
  FolderOpen,
  GitCommit,
  GitMerge,
  History,
  Loader2,
  Play,
  Plus,
  RefreshCw,
  RotateCcw,
  Sparkles,
  Terminal,
  Trash2,
  Upload,
  Wand2,
  X,
} from "lucide-react";
import { api, errMessage } from "@/lib/api";
import type {
  Change,
  ChangeFileView,
  DiffResult,
  FeatureDeleteResult,
  FeatureMetadata,
  FeaturePathsResult,
  FeatureStatusResult,
  RepoFileStatus,
  RequestResult,
  RequestResults,
  Repository,
  TerminalSession,
} from "@/lib/types";
import { TerminalPanel, runtime } from "@/components/TerminalPanel";
import { ToolOpenMenu } from "@/components/ToolOpenMenu";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { useToast } from "@/components/ui/toast";
import { useConfirm } from "@/components/ui/confirm";
import { useI18n } from "@/i18n/I18nProvider";
import { useDefaultTool } from "@/lib/tool";

interface Props {
  name: string;
  onBack: () => void;
  onViewHistory: (feature: string) => void;
  tab: FeatureDetailTab;
  setTab: Dispatch<SetStateAction<FeatureDetailTab>>;
  terminalTabs: TerminalSession[];
  setTerminalTabs: Dispatch<SetStateAction<TerminalSession[]>>;
  agentSession: TerminalSession | null;
  setAgentSession: Dispatch<SetStateAction<TerminalSession | null>>;
}

type PrimaryTab = "work" | "status" | "settings";
export type FeatureDetailTab = PrimaryTab | `terminal:${string}`;

export function FeatureDetailPage({
  name,
  onBack,
  onViewHistory,
  tab,
  setTab,
  terminalTabs,
  setTerminalTabs,
  agentSession,
  setAgentSession,
}: Props) {
  const { notify } = useToast();
  const { t } = useI18n();
  const confirm = useConfirm();
  const tool = useDefaultTool();
  const [advancedOpen, setAdvancedOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const [statusLoading, setStatusLoading] = useState(true);
  const [diffLoading, setDiffLoading] = useState(false);
  const [diffLoaded, setDiffLoaded] = useState(false);
  const [diffAutoAttempted, setDiffAutoAttempted] = useState(false);
  const diffLoadingRef = useRef(false);
  const [deleteBranch, setDeleteBranch] = useState(false);
  const [dangerOpen, setDangerOpen] = useState(false);
  const [featureMeta, setFeatureMeta] = useState<FeatureMetadata | null>(null);
  const [featurePaths, setFeaturePaths] = useState<FeaturePathsResult | null>(null);
  const [configuredRepos, setConfiguredRepos] = useState<Repository[]>([]);
  const [repoPolicy, setRepoPolicy] = useState("reuse");

  const [status, setStatus] = useState<FeatureStatusResult | null>(null);
  const [diff, setDiff] = useState<DiffResult | null>(null);
  const [openDiffRepos, setOpenDiffRepos] = useState<Set<string>>(new Set());
  // Per-repo expand/collapse for the worktree status changed-file lists.
  const [openStatusRepos, setOpenStatusRepos] = useState<Set<string>>(new Set());
  const [fileView, setFileView] = useState<{
    repo: string;
    change: Change;
    loading: boolean;
    data?: ChangeFileView;
    error?: string;
  } | null>(null);
  // Sync-history stack depth per repository, for the count badges.
  const [histCounts, setHistCounts] = useState<Record<string, number>>({});
  // Local override of prepared state. null = no user action yet (fall back to
  // backend status); true/false once the user prepares or deletes. Keeping it
  // separate from status avoids a status reload clobbering the value right
  // after a successful prepare.
  const [prepared, setPrepared] = useState<boolean | null>(null);
  const [preparingRepo, setPreparingRepo] = useState<string | null>(null);

  // sync options
  const [includeRisky, setIncludeRisky] = useState(false);
  const [allowMasked, setAllowMasked] = useState(false);
  const [dryRun, setDryRun] = useState(false);

  // deliver
  const [commitMsg, setCommitMsg] = useState("");
  // Per-repository commit messages for the per-repo commit area, keyed by repo name.
  const [repoMsgs, setRepoMsgs] = useState<Record<string, string>>({});
  // Repo currently being committed/pushed ("*" for an all-repos action, the repo
  // name for a single one, null when idle) so only the active row shows a spinner.
  const [actingRepo, setActingRepo] = useState<string | null>(null);
  const [mrTitle, setMrTitle] = useState("");
  const [requests, setRequests] = useState<RequestResults | null>(null);
  const [program, setProgram] = useState(() => {
    try {
      return localStorage.getItem("agentsafe.program") || "code";
    } catch {
      return "code";
    }
  });
  const [terminalProgram, setTerminalProgram] = useState(() => {
    try {
      return localStorage.getItem("agentsafe.terminalProgram") || "powershell";
    } catch {
      return "powershell";
    }
  });
  // Whether re-preparing backs up the existing agent workspace (default true).
  const [backupOnPrepare, setBackupOnPrepare] = useState(() => {
    try {
      return localStorage.getItem("agentsafe.backupOnPrepare") !== "false";
    } catch {
      return true;
    }
  });

  // Managed agent run: command launched in an embedded terminal whose exit
  // triggers a diff refresh and the "agent finished" sync prompt.
  const [agentCommand, setAgentCommand] = useState(() => {
    try {
      return localStorage.getItem("agentsafe.agentCommand") || "claude";
    } catch {
      return "claude";
    }
  });
  const [agentFinished, setAgentFinished] = useState(false);

  function changeBackupOnPrepare(v: boolean) {
    setBackupOnPrepare(v);
    try {
      localStorage.setItem("agentsafe.backupOnPrepare", v ? "true" : "false");
    } catch {
      /* ignore */
    }
  }

  function changeAgentCommand(v: string) {
    setAgentCommand(v);
    try {
      localStorage.setItem("agentsafe.agentCommand", v);
    } catch {
      /* ignore */
    }
  }

  function changeTerminalProgram(v: string) {
    setTerminalProgram(v);
    try {
      localStorage.setItem("agentsafe.terminalProgram", v);
    } catch {
      /* ignore */
    }
  }

  const loadStatus = useCallback(async () => {
    setStatusLoading(true);
    try {
      setStatus(await api.FeatureStatus(name));
    } catch (e) {
      notify(errMessage(e), "error");
    } finally {
      setStatusLoading(false);
    }
  }, [name, notify]);

  const loadDiff = useCallback(async (showSuccess = false) => {
    if (diffLoadingRef.current) return;
    diffLoadingRef.current = true;
    setDiffLoading(true);
    try {
      const next = await api.AgentDiff(name, "");
      setDiff(next);
      setDiffLoaded(true);
      if (showSuccess) {
        const count = (next.repositories ?? []).reduce(
          (total, repo) => total + (repo.changes?.length ?? 0),
          0
        );
        notify(t("toast.diffRefreshed", { count }), "success");
      }
    } catch (e) {
      notify(errMessage(e), "error");
    } finally {
      diffLoadingRef.current = false;
      setDiffLoading(false);
    }
  }, [name, notify, t]);

  const loadCounts = useCallback(async () => {
    try {
      setHistCounts(await api.SyncHistoryCounts(name));
    } catch {
      /* badges are best-effort */
    }
  }, [name]);

  const loadRepoManager = useCallback(async () => {
    try {
      const [meta, repos, paths] = await Promise.all([
        api.LoadFeature(name),
        api.ListRepos(),
        api.FeaturePaths(name),
      ]);
      setFeatureMeta(meta);
      setFeaturePaths(paths);
      setConfiguredRepos(repos ?? []);
    } catch (e) {
      notify(errMessage(e), "error");
    }
  }, [name, notify]);

  useEffect(() => {
    setStatus(null);
    setDiff(null);
    setDiffLoaded(false);
    setDiffAutoAttempted(false);
    setPrepared(null);
    setFeaturePaths(null);
    setAgentFinished(false);
    setOpenDiffRepos(new Set());
    setOpenStatusRepos(new Set());
  }, [name]);

  useEffect(() => {
    loadStatus();
    loadCounts();
    loadRepoManager();
  }, [loadStatus, loadCounts, loadRepoManager]);

  useEffect(() => {
    if (!tab.startsWith("terminal:")) return;
    const id = tab.slice("terminal:".length);
    if (!terminalTabs.some((terminal) => terminal.id === id)) {
      setTab("work");
    }
  }, [setTab, tab, terminalTabs]);

  const agentStatusLoading = prepared === null && statusLoading && !status;
  const agentReady = prepared ?? (status?.agentReady ?? false);
  const agentMissing = !agentStatusLoading && !agentReady;

  useEffect(() => {
    if (
      tab === "work" &&
      agentReady &&
      !diffLoaded &&
      !diffLoading &&
      !diffAutoAttempted
    ) {
      setDiffAutoAttempted(true);
      void loadDiff();
    }
  }, [
    agentReady,
    diffAutoAttempted,
    diffLoaded,
    diffLoading,
    loadDiff,
    tab,
  ]);

  // When a managed agent run for this feature exits, refresh the diff/status and
  // surface the "agent finished" banner so the user can review and sync.
  useEffect(() => {
    const rt = runtime();
    if (!rt) return;
    const off = rt.EventsOn("agent:exit", (...data: unknown[]) => {
      const payload = data[0] as { feature?: string };
      if (payload?.feature !== name) return;
      setAgentFinished(true);
      notify(t("toast.agentFinished"), "success");
      void Promise.all([loadDiff(), loadStatus(), loadCounts()]);
    });
    return off;
  }, [name, notify, t, loadDiff, loadStatus, loadCounts]);

  useEffect(() => {
    if (!agentSession) return;
    let cancelled = false;
    void api
      .TerminalSnapshot(agentSession.id)
      .then((snapshot) => {
        if (cancelled || !snapshot.closed) return;
        setAgentFinished(true);
        void Promise.all([loadDiff(), loadStatus(), loadCounts()]);
      })
      .catch(() => {
        /* session may have been closed explicitly */
      });
    return () => {
      cancelled = true;
    };
  }, [agentSession, loadCounts, loadDiff, loadStatus]);

  async function run(fn: () => Promise<void>) {
    setBusy(true);
    try {
      await fn();
    } catch (e) {
      notify(errMessage(e), "error");
    } finally {
      setBusy(false);
    }
  }

  const prepare = () =>
    run(async () => {
      const meta = await api.AgentPrepare(name, backupOnPrepare);
      const repos = meta.repositories ?? [];
      const copied = repos.reduce((n, r) => n + r.copiedFiles, 0);
      notify(t("toast.agentPrepared", { count: copied }), "success");
      setPrepared(true);
      setDiffLoaded(false);
      setDiffAutoAttempted(true);
      await Promise.all([loadDiff(), loadStatus()]);
      setDiffLoaded(true);
      setTab("work");
    });

  // syncAndCommit applies the reviewed agent changes to the worktrees and, unless
  // it is a dry run, commits them with the bulk message in a single action.
  const syncAndCommit = () =>
    run(async () => {
      await api.SyncAndCommit(name, dryRun ? "" : commitMsg.trim(), {
        repo: "",
        dryRun,
        includeRisky,
        allowMaskedSync: allowMasked,
      });
      notify(
        dryRun ? t("toast.dryRunCompleted") : t("toast.syncCommitted"),
        "success"
      );
      if (!dryRun) {
        setCommitMsg("");
        setAgentFinished(false);
      }
      await Promise.all([loadDiff(), loadStatus(), loadCounts()]);
    });

  const sync = () =>
    run(async () => {
      await api.AgentSync(name, {
        repo: "",
        dryRun,
        includeRisky,
        allowMaskedSync: allowMasked,
      });
      notify(
        dryRun ? t("toast.dryRunCompleted") : t("toast.syncCompleted"),
        "success"
      );
      await Promise.all([loadDiff(), loadStatus(), loadCounts()]);
    });

  // syncCommitPush runs the whole delivery pipeline in one action: sync reviewed
  // agent changes back to the worktrees, commit them (templated message when the
  // field is empty), then push every branch. risky/masked files stay gated by the
  // toggles above, so with them off the sync aborts before any commit or push.
  const syncCommitPush = () =>
    run(async () => {
      await api.SyncCommitPush(name, dryRun ? "" : commitMsg.trim(), {
        repo: "",
        dryRun,
        includeRisky,
        allowMaskedSync: allowMasked,
      });
      notify(
        dryRun ? t("toast.dryRunCompleted") : t("toast.syncCommitPushed"),
        "success"
      );
      if (!dryRun) {
        setCommitMsg("");
        setAgentFinished(false);
      }
      await Promise.all([loadDiff(), loadStatus(), loadCounts()]);
    });

  const restoreFromWorktree = (repo: string, path: string) =>
    run(async () => {
      await api.AgentRestoreFromWorktree(name, repo, path);
      notify(t("toast.agentRestoredFromWorktree", { path }), "success");
      await loadDiff();
    });

  const openChangeFileView = (repo: string, change: Change) => {
    setFileView({ repo, change, loading: true });
    api
      .AgentChangeFileView(name, repo, change.path)
      .then((data) => setFileView({ repo, change, loading: false, data }))
      .catch((e) =>
        setFileView({ repo, change, loading: false, error: errMessage(e) })
      );
  };

  const del = () =>
    run(async () => {
      await api.AgentDelete(name);
      setDiff(null);
      setDiffLoaded(false);
      setDiffAutoAttempted(false);
      setPrepared(false);
      notify(t("toast.agentDeleted"), "success");
      await loadStatus();
    });

  const deleteFeature = () =>
    run(async () => {
      if (
        !(await confirm({
          message: t("feature.deleteConfirm", { name }),
          danger: true,
        }))
      )
        return;
      let result: FeatureDeleteResult | undefined;
      try {
        result = await api.FeatureDelete(name, deleteBranch, false);
      } catch (e) {
        // Offer a force delete when a worktree has uncommitted changes.
        if (/uncommitted|changes/i.test(errMessage(e))) {
          if (
            !(await confirm({
              message: t("feature.deleteForceConfirm"),
              danger: true,
            }))
          )
            return;
          result = await api.FeatureDelete(name, deleteBranch, true);
        } else {
          throw e;
        }
      }
      for (const warning of result?.warnings ?? []) {
        notify(warning, "error");
      }
      notify(t("toast.featureDeleted"), "success");
      onBack();
    });

  // Display name for a program command/path (drop directories and ".app").
  function programLabel(prog: string) {
    const base = prog.split(/[\\/]/).pop() || prog;
    return base.replace(/\.app$/i, "");
  }

  function terminalTabId(id: string): FeatureDetailTab {
    return `terminal:${id}`;
  }

  function addTerminalTab(session: TerminalSession, title?: string) {
    const next = title ? { ...session, title } : session;
    setTerminalTabs((prev) =>
      prev.some((tab) => tab.id === next.id) ? prev : [...prev, next]
    );
    setTab(terminalTabId(next.id));
  }

  const openTerminal = (prog?: string) =>
    run(async () => {
      if (!featurePaths?.agentPath) return;
      const session = await api.TerminalOpenWithProgram(
        featurePaths.agentPath,
        (prog ?? terminalProgram).trim()
      );
      if (session.external) {
        notify(t("toast.openedPath", { path: session.path }), "success");
        return;
      }
      addTerminalTab(session, `Terminal · ${name}`);
      notify(t("toast.openedEmbeddedTerminal", { path: session.path }), "success");
    });

  const openAgentCommandTerminal = () =>
    run(async () => {
      const session = await api.AgentRunWithProgram(
        name,
        agentCommand.trim(),
        terminalProgram.trim()
      );
      if (session.external) {
        notify(t("toast.openedPath", { path: session.path }), "success");
        return;
      }
      addTerminalTab(session, agentCommand.trim() || `Terminal · ${name}`);
      setAgentFinished(false);
      notify(t("toast.agentStarted"), "success");
    });

  // runAgent launches the configured agent command in an embedded terminal; its
  // exit is detected via the "agent:exit" event subscribed above.
  const runAgent = () =>
    run(async () => {
      const session = await api.AgentRunWithProgram(
        name,
        agentCommand.trim(),
        terminalProgram.trim()
      );
      if (session.external) {
        setAgentSession(null);
        notify(t("toast.openedPath", { path: session.path }), "success");
        return;
      }
      setAgentSession(session);
      addTerminalTab(session, agentCommand.trim() || `Terminal · ${name}`);
      setAgentFinished(false);
      notify(t("toast.agentStarted"), "success");
    });

  const closeAgentTerminal = () =>
    run(async () => {
      if (agentSession) {
        await api.TerminalClose(agentSession.id);
      }
      setAgentSession(null);
      setTerminalTabs((prev) => prev.filter((tab) => tab.id !== agentSession?.id));
      if (agentSession) {
        setTab((prev) => (prev === terminalTabId(agentSession.id) ? "work" : prev));
      }
    });

  const closeTerminalTab = (id: string) =>
    run(async () => {
      await api.TerminalClose(id);
      setTerminalTabs((prev) => prev.filter((tab) => tab.id !== id));
      if (agentSession?.id === id) setAgentSession(null);
      setTab((prev) => (prev === terminalTabId(id) ? "work" : prev));
    });

  const openRepoFolder = (repo: string) =>
    run(async () => {
      const p = await api.OpenRepoFolder(name, repo);
      notify(t("toast.openedPath", { path: p }), "success");
    });

  const openFeatureFolder = () =>
    run(async () => {
      const p = await api.OpenFeatureFolder(name);
      notify(t("toast.openedPath", { path: p }), "success");
    });

  const openWorktreeTerminal = (prog?: string) =>
    run(async () => {
      if (!featurePaths?.worktreePath) return;
      const s = await api.TerminalOpenWithProgram(
        featurePaths.worktreePath,
        (prog ?? terminalProgram).trim()
      );
      if (s.external) {
        notify(t("toast.openedPath", { path: s.path }), "success");
        return;
      }
      addTerminalTab(s, `Terminal · ${name}`);
      notify(t("toast.openedEmbeddedTerminal", { path: s.path }), "success");
    });

  const openWorktreeTool = (prog?: string) =>
    run(async () => {
      if (!featurePaths?.worktreePath) return;
      const p = await api.OpenPathInProgram(featurePaths.worktreePath, prog ?? tool.value);
      notify(t("toast.openedPath", { path: p }), "success");
    });

  // Pick an arbitrary executable, then open the given path with it.
  const browseAndOpen = (path?: string) =>
    run(async () => {
      if (!path) return;
      const sel = await api.SelectProgram();
      if (!sel) return;
      const p = await api.OpenPathInProgram(path, sel);
      notify(t("toast.openedPath", { path: p }), "success");
    });

  const openAgentFolder = () =>
    run(async () => {
      if (!featurePaths?.agentPath) return;
      const p = await api.OpenPath(featurePaths.agentPath);
      notify(t("toast.openedPath", { path: p }), "success");
    });

  const openAgentTool = (prog?: string) =>
    run(async () => {
      if (!featurePaths?.agentPath) return;
      const p = await api.OpenPathInProgram(featurePaths.agentPath, prog ?? tool.value);
      notify(t("toast.openedPath", { path: p }), "success");
    });

  // Per-repo open helpers for the status-tab repo rows.
  const openRepoWorktreeTerminal = (repoName: string, prog?: string) =>
    run(async () => {
      const path = repoPathsByName.get(repoName)?.worktreePath;
      if (!path) return;
      const s = await api.TerminalOpenWithProgram(path, (prog ?? terminalProgram).trim());
      if (s.external) {
        notify(t("toast.openedPath", { path: s.path }), "success");
        return;
      }
      addTerminalTab(s, `Terminal · ${repoName}`);
      notify(t("toast.openedEmbeddedTerminal", { path: s.path }), "success");
    });

  const openRepoAgentFolder = (repoName: string) =>
    run(async () => {
      const path = repoPathsByName.get(repoName)?.agentPath;
      if (!path) return;
      const p = await api.OpenPath(path);
      notify(t("toast.openedPath", { path: p }), "success");
    });

  const openRepoAgentTerminal = (repoName: string, prog?: string) =>
    run(async () => {
      const path = repoPathsByName.get(repoName)?.agentPath;
      if (!path) return;
      const s = await api.TerminalOpenWithProgram(path, (prog ?? terminalProgram).trim());
      if (s.external) {
        notify(t("toast.openedPath", { path: s.path }), "success");
        return;
      }
      addTerminalTab(s, `Terminal · ${repoName}`);
      notify(t("toast.openedEmbeddedTerminal", { path: s.path }), "success");
    });

  const openRepoAgentTool = (repoName: string, prog?: string) =>
    run(async () => {
      const path = repoPathsByName.get(repoName)?.agentPath;
      if (!path) return;
      const p = await api.OpenPathInProgram(path, prog ?? tool.value);
      notify(t("toast.openedPath", { path: p }), "success");
    });

  const copyPath = (path?: string) =>
    run(async () => {
      if (!path) return;
      await api.CopyText(path);
      notify(t("toast.copiedPath"), "success");
    });

  const applyWorktreeTemplates = () =>
    run(async () => {
      await api.ApplyWorktreeTemplates(name);
      notify(t("toast.templatesApplied"), "success");
      await Promise.all([loadStatus(), loadRepoManager()]);
    });

  const applyAgentTemplates = () =>
    run(async () => {
      await api.ApplyAgentTemplates(name);
      notify(t("toast.templatesApplied"), "success");
      await loadDiff(true);
    });

  const openRepoProgram = (repo: string, prog?: string) =>
    run(async () => {
      const p = await api.OpenRepoInProgram(name, repo, (prog ?? program).trim());
      notify(t("toast.openedPath", { path: p }), "success");
    });

  // Pick an arbitrary executable, then open the given repo's worktree with it.
  const browseAndOpenRepo = (repo: string) =>
    run(async () => {
      const sel = await api.SelectProgram();
      if (!sel) return;
      const p = await api.OpenRepoInProgram(name, repo, sel);
      notify(t("toast.openedPath", { path: p }), "success");
    });

  const chooseProgram = () =>
    run(async () => {
      const sel = await api.SelectProgram();
      if (!sel) return;
      setProgram(sel);
      try {
        localStorage.setItem("agentsafe.program", sel);
      } catch {
        /* ignore */
      }
      notify(t("toast.programSelected", { program: programLabel(sel) }), "success");
    });

  // doCommit commits a single repo (repoName set) or all repos (repoName "")
  // with the given message, clearing the relevant input and refreshing status.
  const doCommit = (repoName: string, message: string) =>
    run(async () => {
      setActingRepo(repoName || "*");
      try {
        await api.Commit(name, message.trim(), repoName);
        notify(t("toast.committed"), "success");
        if (repoName) {
          setRepoMsgs((m) => ({ ...m, [repoName]: "" }));
        } else {
          setCommitMsg("");
        }
        await loadStatus();
      } finally {
        setActingRepo(null);
      }
    });

  const push = (repoName = "") =>
    run(async () => {
      setActingRepo(repoName || "*");
      try {
        await api.Push(name, repoName);
        notify(t("toast.pushed"), "success");
        await loadStatus();
      } finally {
        setActingRepo(null);
      }
    });

  const rebase = () =>
    run(async () => {
      const res = await api.RebaseFeature(name, "");
      const repos = res.repositories ?? [];
      const failed = repos.filter((r) => r.status === "failed");
      const rebased = repos.filter((r) => r.status === "rebased");
      if (failed.length > 0) {
        notify(
          t("toast.rebaseConflict", {
            repos: failed.map((r) => r.name).join(", "),
          }),
          "error"
        );
      } else {
        notify(t("toast.rebased", { count: rebased.length }), "success");
      }
      await loadStatus();
    });

  const prepareRepo = (repoName: string) =>
    run(async () => {
      setPreparingRepo(repoName);
      try {
        const meta = await api.AgentPrepareRepo(name, repoName, backupOnPrepare);
        const item = (meta.repositories ?? []).find((r) => r.name === repoName);
        notify(
          t("toast.agentRepoPrepared", {
            repo: repoName,
            count: item?.copiedFiles ?? 0,
          }),
          "success"
        );
        setPrepared(null);
        setDiff(null);
        setDiffLoaded(false);
        setDiffAutoAttempted(false);
        await loadStatus();
      } finally {
        setPreparingRepo(null);
      }
    });

  const addFeatureRepo = (repoName: string) =>
    run(async () => {
      try {
        await api.FeatureRepoAdd(name, repoName, repoPolicy, false);
      } catch (e) {
        if (/already exists/i.test(errMessage(e))) {
          if (
            !(await confirm({
              message: t("feature.repoAddForceConfirm", { repo: repoName }),
              danger: true,
            }))
          )
            return;
          await api.FeatureRepoAdd(name, repoName, repoPolicy, true);
        } else {
          throw e;
        }
      }
      notify(t("toast.featureRepoAdded", { repo: repoName }), "success");
      await Promise.all([loadStatus(), loadRepoManager()]);
    });

  const recreateFeatureRepo = (repoName: string) =>
    run(async () => {
      if (
        !(await confirm({
          message: t("feature.repoRecreateConfirm", { repo: repoName }),
          danger: repoPolicy === "recreate",
        }))
      )
        return;
      try {
        await api.FeatureRepoRecreate(name, repoName, repoPolicy, false);
      } catch (e) {
        if (/uncommitted|changes/i.test(errMessage(e))) {
          if (
            !(await confirm({
              message: t("feature.repoRecreateForceConfirm", { repo: repoName }),
              danger: true,
            }))
          )
            return;
          await api.FeatureRepoRecreate(name, repoName, repoPolicy, true);
        } else {
          throw e;
        }
      }
      notify(t("toast.featureRepoRecreated", { repo: repoName }), "success");
      await Promise.all([loadStatus(), loadRepoManager()]);
    });

  const sendRequests = () =>
    run(async () => {
      const res = await api.CreateMergeRequests(name, mrTitle.trim());
      setRequests(res);
      // Open browser-fallback items in the default browser.
      for (const item of res.items ?? []) {
        if (item.method === "browser" && item.url) {
          await api.OpenURL(item.url);
        }
      }
      notify(t("toast.requestsSent"), "success");
    });

  const tabs: { id: PrimaryTab; label: string }[] = [
    { id: "work", label: "작업" },
    { id: "status", label: "상태" },
    { id: "settings", label: "설정" },
  ];
  const repoPathsByName = new Map(
    (featurePaths?.repositories ?? []).map((repo) => [repo.name, repo])
  );
  const statusReposByName = new Map(
    (status?.repositories ?? []).map((repo) => [repo.name, repo])
  );
  const configuredRepoByName = new Map(
    (configuredRepos ?? []).map((repo) => [repo.Name, repo])
  );
  const worktreeRepoNames = Array.from(
    new Set([
      ...(status?.repositories ?? []).map((repo) => repo.name),
      ...(featureMeta?.repositories ?? []).map((repo) => repo.name),
      ...(configuredRepos ?? []).map((repo) => repo.Name),
    ])
  );
  const agentRepos = status?.repositories ?? [];
  const agentChangeCount = (diff?.repositories ?? []).reduce(
    (n, r) => n + (r.changes?.length ?? 0),
    0
  );

  const isTerminalTab = tab.startsWith("terminal:");

  return (
    <div className="flex h-full min-h-0 flex-col gap-5">
      <div className="flex items-center gap-3">
        <Button variant="ghost" size="sm" onClick={onBack}>
          <ArrowLeft className="size-4" /> {t("common.back")}
        </Button>
        <div className="inline-flex rounded-lg border bg-card p-1">
          {tabs.map((t) => (
            <button
              key={t.id}
              onClick={() => setTab(t.id)}
              className={
                "rounded-md px-3 py-1.5 text-sm transition-colors " +
                (tab === t.id
                  ? "bg-secondary font-medium text-secondary-foreground"
                  : "text-muted-foreground hover:text-foreground")
              }
            >
              {t.label}
            </button>
          ))}
          {terminalTabs.map((terminal) => {
            const id = terminalTabId(terminal.id);
            return (
              <div
                key={terminal.id}
                className={
                  "flex items-center gap-1 rounded-md px-2 py-1.5 text-sm transition-colors " +
                  (tab === id
                    ? "bg-secondary font-medium text-secondary-foreground"
                    : "text-muted-foreground hover:text-foreground")
                }
                title={terminal.path}
              >
                <button
                  type="button"
                  className="flex min-w-0 items-center gap-1"
                  onClick={() => setTab(id)}
                >
                  <Terminal className="size-3.5 shrink-0" />
                  <span className="max-w-32 truncate">{terminal.title}</span>
                </button>
                <button
                  type="button"
                  className="rounded p-0.5 opacity-70 hover:bg-accent hover:opacity-100"
                  onClick={() => void closeTerminalTab(terminal.id)}
                  title={t("common.close")}
                >
                  <X className="size-3" />
                </button>
              </div>
            );
          })}
        </div>
      </div>

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
      {tab === "work" && (
        <div className="space-y-5">
          <Card>
            <CardHeader>
              <CardTitle>개요 정보</CardTitle>
              <CardDescription>에이전트 워크스페이스 관련 작업만 표시합니다.</CardDescription>
            </CardHeader>
            <CardContent className="space-y-3">
              <div className="flex items-center justify-between gap-3 rounded-md border bg-muted/30 p-3">
                <div className="min-w-0">
                  <div className="text-xs font-medium text-muted-foreground">에이전트 경로</div>
                  <div className="truncate font-mono text-xs" title={featurePaths?.agentPath}>
                    {featurePaths?.agentPath || "-"}
                  </div>
                </div>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => copyPath(featurePaths?.agentPath)}
                  disabled={busy || !featurePaths?.agentPath}
                >
                  <Copy className="size-4" /> {t("feature.copyPath")}
                </Button>
              </div>
              <div className="flex flex-wrap items-end gap-2 rounded-md border p-3">
                <div className="min-w-56 flex-1 space-y-1.5">
                  <Label htmlFor="agentCmdOverview">에이전트 명령</Label>
                  <Input
                    id="agentCmdOverview"
                    value={agentCommand}
                    onChange={(e) => changeAgentCommand(e.target.value)}
                    placeholder="claude"
                  />
                </div>
                <Button
                  onClick={openAgentCommandTerminal}
                  disabled={busy || agentStatusLoading || !agentReady || !agentCommand.trim()}
                >
                  <Terminal className="size-4" /> 에이전트 명령 터미널 열기
                </Button>
              </div>
              {agentStatusLoading ? (
                <div className="flex items-center gap-2 rounded-md border border-dashed p-3 text-sm text-muted-foreground">
                  <Loader2 className="size-4 animate-spin" />
                  {t("feature.loadingAgent")}
                </div>
              ) : agentMissing ? (
                <div className="rounded-md border border-amber-300 bg-amber-50 p-3 text-sm text-amber-900">
                  에이전트 폴더가 없습니다. 상태 탭에서 먼저 생성하세요.
                </div>
              ) : null}
            </CardContent>
          </Card>

          {agentReady && (
            <>
              <Card>
                <CardHeader className="flex-row items-center justify-between space-y-0">
                  <div>
                    <CardTitle>변경 정보</CardTitle>
                    <CardDescription>
                      {diffLoading && !diff
                        ? t("feature.loadingDiff")
                        : diff
                        ? t("feature.changeCount", { count: agentChangeCount })
                        : t("feature.diffHint")}
                    </CardDescription>
                  </div>
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => loadDiff(true)}
                    disabled={busy || diffLoading}
                  >
                    {diffLoading ? (
                      <Loader2 className="size-4 animate-spin" />
                    ) : (
                      <RefreshCw className="size-4" />
                    )}
                    {diffLoading ? t("feature.refreshingDiff") : t("feature.refreshDiff")}
                  </Button>
                </CardHeader>
                <CardContent className="space-y-4">
                  {diffLoading && !diff ? (
                    <LoadingState label={t("feature.loadingDiff")} />
                  ) : (diff?.repositories ?? []).map((r) => {
                    const changes = r.changes ?? [];
                    const open = openDiffRepos.has(r.name);
                    return (
                      <div key={r.name} className="rounded-md border">
                        <button
                          type="button"
                          className="flex w-full items-center justify-between gap-3 px-3 py-2 text-left hover:bg-accent/50"
                          onClick={() =>
                            setOpenDiffRepos((prev) => {
                              const next = new Set(prev);
                              if (next.has(r.name)) next.delete(r.name);
                              else next.add(r.name);
                              return next;
                            })
                          }
                          aria-expanded={open}
                        >
                          <div className="flex min-w-0 items-center gap-2">
                            {open ? <ChevronDown className="size-4" /> : <ChevronRight className="size-4" />}
                            <span className="truncate font-medium">{r.name}</span>
                            <Badge variant={changes.length > 0 ? "secondary" : "outline"}>
                              {t("feature.repoChanges", { count: changes.length })}
                            </Badge>
                            {(histCounts[r.name] ?? 0) > 0 && (
                              <Badge variant="secondary">
                                {t("feature.syncHistoryBadge", { count: histCounts[r.name] })}
                              </Badge>
                            )}
                          </div>
                        </button>
                        {open && (
                          changes.length === 0 ? (
                            <p className="border-t p-3 text-sm text-muted-foreground">{t("feature.noChanges")}</p>
                          ) : (
                            <ul className="divide-y border-t">
                              {changes.map((c, i) => (
                                <ChangeRow
                                  key={c.path + i}
                                  change={c}
                                  onView={() => openChangeFileView(r.name, c)}
                                  onRestore={() => restoreFromWorktree(r.name, c.path)}
                                  disabled={busy || diffLoading}
                                />
                              ))}
                            </ul>
                          )
                        )}
                      </div>
                    );
                  })}
                  <div className="space-y-3 rounded-md border bg-muted/30 p-3">
                    <div>
                      <div className="font-medium">Sync to current delivery area</div>
                      <p className="text-xs text-muted-foreground">
                        모의실행, 위험 파일 포함, 마스킹 동기화 허용 옵션을 선택한 뒤 동기화합니다.
                      </p>
                    </div>
                    <div className="flex flex-wrap items-center gap-4 text-sm">
                      <Toggle checked={dryRun} onChange={setDryRun} label={t("feature.dryRun")} />
                      <Toggle checked={includeRisky} onChange={setIncludeRisky} label={t("feature.includeRisky")} />
                      <Toggle checked={allowMasked} onChange={setAllowMasked} label={t("feature.allowMasked")} />
                      <Button variant="outline" size="sm" onClick={sync} disabled={busy}>
                        <Upload className="size-4" /> {dryRun ? t("feature.previewSync") : t("feature.sync")}
                      </Button>
                      <Button size="sm" onClick={syncCommitPush} disabled={busy}>
                        <Upload className="size-4" /> {t("feature.syncCommitPush")}
                      </Button>
                    </div>
                  </div>
                </CardContent>
              </Card>

              <Card>
                <CardHeader className="flex-row items-center justify-between space-y-0">
                  <div>
                    <CardTitle>전달</CardTitle>
                    <CardDescription>Git 커밋, 푸시 기능만 제공합니다.</CardDescription>
                  </div>
                  <Button variant="outline" size="sm" onClick={loadStatus} disabled={statusLoading || busy}>
                    {statusLoading ? <Loader2 className="size-4 animate-spin" /> : <RefreshCw className="size-4" />}
                    {statusLoading ? t("common.loading") : t("common.refresh")}
                  </Button>
                </CardHeader>
                <CardContent className="space-y-4">
                  {(() => {
                    const repos = status?.repositories ?? [];
                    const anyDirty = repos.some((r) => (r.changes ?? []).length > 0);
                    const anyPushable = repos.some((r) => (r.ahead ?? 0) > 0);
                    const actionable = repos.filter(
                      (r) => (r.changes ?? []).length > 0 || (r.ahead ?? 0) > 0 || !!r.error
                    );
                    return (
                      <>
                        <div className="space-y-1.5">
                          <Label htmlFor="cm">{t("feature.messageLabel")}</Label>
                          <Input
                            id="cm"
                            value={commitMsg}
                            onChange={(e) => setCommitMsg(e.target.value)}
                            placeholder="feat: add coupon v2"
                          />
                        </div>
                        <div className="flex flex-wrap gap-2">
                          <Button
                            onClick={() => doCommit("", commitMsg)}
                            disabled={busy || !anyDirty || !commitMsg.trim()}
                          >
                            {actingRepo === "*" ? <Loader2 className="size-4 animate-spin" /> : <GitCommit className="size-4" />}
                            {t("feature.commitAll")}
                          </Button>
                          <Button variant="secondary" onClick={() => push()} disabled={busy || !anyPushable}>
                            <Upload className="size-4" /> {t("feature.pushAll")}
                          </Button>
                        </div>
                        <div className="divide-y rounded-md border">
                          {statusLoading && !status ? (
                            <LoadingState label={t("feature.loadingStatus")} />
                          ) : actionable.length === 0 ? (
                            <p className="p-3 text-sm text-muted-foreground">{t("feature.noChangesToCommit")}</p>
                          ) : (
                            actionable.map((r) => {
                              const dirty = (r.changes ?? []).length > 0;
                              const ahead = r.ahead ?? 0;
                              const msg = repoMsgs[r.name] ?? "";
                              return (
                                <div key={r.name} className="space-y-3 p-3">
                                  <div className="flex flex-wrap items-center gap-2">
                                    <Boxes className="size-4 text-muted-foreground" />
                                    <span className="font-medium">{r.name}</span>
                                    {dirty && <Badge variant="warning">{t("feature.repoChanges", { count: (r.changes ?? []).length })}</Badge>}
                                    {ahead > 0 && <Badge variant="secondary">{t("feature.repoAhead", { count: ahead })}</Badge>}
                                  </div>
                                  {r.error && (
                                    <div className="rounded-md border border-destructive/40 bg-destructive/5 p-3 text-xs text-destructive">
                                      {r.error}
                                    </div>
                                  )}
                                  <div className="flex items-end gap-2">
                                    <div className="flex-1 space-y-1.5">
                                      <Label htmlFor={"cm-" + r.name}>{t("feature.messageLabel")}</Label>
                                      <Input
                                        id={"cm-" + r.name}
                                        value={msg}
                                        onChange={(e) => setRepoMsgs((m) => ({ ...m, [r.name]: e.target.value }))}
                                        placeholder="feat: ..."
                                      />
                                    </div>
                                    <Button onClick={() => doCommit(r.name, msg)} disabled={busy || !dirty || !msg.trim()}>
                                      {actingRepo === r.name ? <Loader2 className="size-4 animate-spin" /> : <GitCommit className="size-4" />}
                                      {t("feature.commitRepo")}
                                    </Button>
                                    <Button variant="secondary" onClick={() => push(r.name)} disabled={busy || ahead === 0}>
                                      <Upload className="size-4" /> {t("feature.pushRepo")}
                                    </Button>
                                  </div>
                                </div>
                              );
                            })
                          )}
                        </div>
                      </>
                    );
                  })()}
                </CardContent>
              </Card>
            </>
          )}
        </div>
      )}

      {tab === "status" && (
        <div className="space-y-5">
          <Card>
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
                  onToolBrowse={() => browseAndOpen(featurePaths?.worktreePath)}
                />
              </div>
            </CardHeader>
            <CardContent className="space-y-4">
              {statusLoading && !status ? (
                <LoadingState label={t("feature.loadingStatus")} />
              ) : worktreeRepoNames.length === 0 ? (
                <p className="text-sm text-muted-foreground">{t("feature.noRepos")}</p>
              ) : (
                <div className="divide-y rounded-md border">
                  {worktreeRepoNames.map((repoName) => {
                    const r = statusReposByName.get(repoName);
                    const configured = configuredRepoByName.get(repoName);
                    const included = (featureMeta?.repositories ?? []).some((item) => item.name === repoName);
                    const changes = r?.changes ?? [];
                    const hasChanges = changes.length > 0;
                    const open = openStatusRepos.has(repoName);
                    return (
                      <div key={repoName} className="space-y-3 p-3">
                        <div className="flex items-center justify-between gap-3">
                          <div className="min-w-0">
                            <div className="flex min-w-0 items-center gap-2">
                              {hasChanges ? (
                                <button
                                  type="button"
                                  className="flex size-4 shrink-0 items-center justify-center text-muted-foreground hover:text-foreground"
                                  onClick={() =>
                                    setOpenStatusRepos((prev) => {
                                      const next = new Set(prev);
                                      if (next.has(repoName)) next.delete(repoName);
                                      else next.add(repoName);
                                      return next;
                                    })
                                  }
                                  aria-expanded={open}
                                  title={open ? t("feature.collapse") : t("feature.expand")}
                                >
                                  {open ? <ChevronDown className="size-4" /> : <ChevronRight className="size-4" />}
                                </button>
                              ) : (
                                <Boxes className="size-4 shrink-0 text-muted-foreground" />
                              )}
                              <span className="truncate font-medium">{repoName}</span>
                              {hasChanges && <Badge variant="warning">{t("feature.repoChanges", { count: changes.length })}</Badge>}
                              {!included && <Badge variant="outline">미추가</Badge>}
                              {included && r?.status.trim() === "" && <Badge variant="outline">{t("feature.clean")}</Badge>}
                              {(histCounts[repoName] ?? 0) > 0 && (
                                <Badge variant="secondary">{t("feature.syncHistoryBadge", { count: histCounts[repoName] })}</Badge>
                              )}
                            </div>
                            <div className="mt-1 truncate text-xs text-muted-foreground">{configured?.DefaultBranch || "-"}</div>
                          </div>
                          <div className="flex shrink-0 flex-wrap items-center justify-end gap-1">
                            {included ? (
                              <Button variant="outline" size="sm" disabled={busy} onClick={() => recreateFeatureRepo(repoName)}>
                                <RotateCcw className="size-4" /> {t("feature.repoRecreate")}
                              </Button>
                            ) : (
                              <Button size="sm" disabled={busy} onClick={() => addFeatureRepo(repoName)}>
                                <Plus className="size-4" /> {t("feature.repoAdd")}
                              </Button>
                            )}
                            <ToolOpenMenu
                              iconOnly
                              disabled={busy || !included}
                              onFolder={() => openRepoFolder(repoName)}
                              onTerminal={(prog) => openRepoWorktreeTerminal(repoName, prog)}
                              onTool={(prog) => openRepoProgram(repoName, prog)}
                              onToolBrowse={() => browseAndOpenRepo(repoName)}
                            />
                          </div>
                        </div>
                        {r?.error ? (
                          <div className="rounded-md border border-destructive/40 bg-destructive/5 p-3 text-xs text-destructive">{r.error}</div>
                        ) : hasChanges && open ? (
                          <ul className="divide-y rounded-md border">
                            {changes.map((change, i) => (
                              <RepoStatusRow key={change.code + "-" + change.path + "-" + i} change={change} />
                            ))}
                          </ul>
                        ) : null}
                      </div>
                    );
                  })}
                </div>
              )}
            </CardContent>
          </Card>

          <Card>
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
                      onToolBrowse={() => browseAndOpen(featurePaths?.agentPath)}
                    />
                  </>
                )}
              </div>
            </CardHeader>
            <CardContent className="space-y-4">
              {status?.agentNeedsPrepare && (
                <div className="rounded-md border border-amber-300 bg-amber-50 p-3 text-sm text-amber-900">
                  {t("feature.agentNeedsPrepare")}
                </div>
              )}
              {statusLoading && !status ? (
                <LoadingState label={t("feature.loadingAgent")} />
              ) : agentRepos.length === 0 ? (
                <p className="text-sm text-muted-foreground">{t("feature.noRepos")}</p>
              ) : (
                <div className="divide-y rounded-md border">
                  {agentRepos.map((repo) => {
                    const paths = repoPathsByName.get(repo.name);
                    return (
                      <div key={repo.name} className="flex items-center justify-between gap-3 p-3">
                        <div className="min-w-0">
                          <div className="font-medium">{repo.name}</div>
                          {paths?.agentPath && (
                            <div className="mt-1 truncate font-mono text-xs text-muted-foreground" title={paths.agentPath}>
                              {t("feature.repoAgentPath")}: {paths.agentPath}
                            </div>
                          )}
                          <div className="mt-1">
                            {!repo.agentReady ? (
                              <Badge variant="outline">{t("feature.agentRepoMissing")}</Badge>
                            ) : repo.agentNeedsPrepare ? (
                              <Badge variant="warning">{t("feature.agentRepoStale")}</Badge>
                            ) : (
                              <Badge variant="success">{t("feature.agentRepoReady")}</Badge>
                            )}
                          </div>
                        </div>
                        <div className="flex shrink-0 items-center gap-2">
                          <Button variant="ghost" size="sm" className="w-8 px-0" title={t("feature.copyPath")} disabled={busy || !paths?.agentPath} onClick={() => copyPath(paths?.agentPath)}>
                            <Copy className="size-4" />
                          </Button>
                          <Button variant={repo.agentReady ? "outline" : "default"} size="sm" disabled={busy} onClick={() => prepareRepo(repo.name)}>
                            {preparingRepo === repo.name ? <Loader2 className="size-4 animate-spin" /> : <Wand2 className="size-4" />}
                            {repo.agentReady ? t("feature.agentRepoRegenerate") : t("feature.agentRepoPrepare")}
                          </Button>
                          <ToolOpenMenu
                            iconOnly
                            disabled={busy || !repo.agentReady || !paths?.agentPath}
                            onFolder={() => openRepoAgentFolder(repo.name)}
                            onTerminal={(prog) => openRepoAgentTerminal(repo.name, prog)}
                            onTool={(prog) => openRepoAgentTool(repo.name, prog)}
                            onToolBrowse={() => browseAndOpen(paths?.agentPath)}
                          />
                        </div>
                      </div>
                    );
                  })}
                </div>
              )}
            </CardContent>
          </Card>
        </div>
      )}

      {tab === "settings" && (
        <div className="space-y-5">
          <Card>
            <CardHeader>
              <CardTitle>터미널 설정</CardTitle>
              <CardDescription>에이전트 명령 터미널의 프로그램과 기본 명령을 설정합니다.</CardDescription>
            </CardHeader>
            <CardContent className="grid gap-4 md:grid-cols-2">
              <div className="space-y-1.5">
                <Label htmlFor="terminalProgram">터미널 프로그램</Label>
                <select
                  id="terminalProgram"
                  value={terminalProgram}
                  onChange={(e) => changeTerminalProgram(e.target.value)}
                  className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
                >
                  <option value="powershell">PowerShell</option>
                  <option value="pwsh">PowerShell 7</option>
                  <option value="cmd">Command Prompt</option>
                  <option value="git-bash">Git Bash</option>
                  <option value="wt">Windows Terminal</option>
                  <option value="default">System default</option>
                </select>
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="agentCmdSettings">터미널 기본 명령</Label>
                <Input id="agentCmdSettings" value={agentCommand} onChange={(e) => changeAgentCommand(e.target.value)} placeholder="claude" />
                <p className="text-xs text-muted-foreground">에이전트 폴더로 이동한 뒤 실행할 명령입니다. 기본값은 claude입니다.</p>
              </div>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>워크트리 설정</CardTitle>
              <CardDescription>동일한 브랜치 옵션과 워크트리 템플릿 적용을 관리합니다.</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="space-y-1.5">
                <Label htmlFor="repoPolicy">동일한 브랜치 워크트리 생성 옵션</Label>
                <select
                  id="repoPolicy"
                  value={repoPolicy}
                  onChange={(e) => setRepoPolicy(e.target.value)}
                  className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
                >
                  <option value="error">{t("features.existingBranchError")}</option>
                  <option value="reuse">{t("features.existingBranchReuse")}</option>
                  <option value="recreate">{t("features.existingBranchRecreate")}</option>
                </select>
                <p className="text-xs text-muted-foreground">{t("features.existingBranchHint." + repoPolicy)}</p>
              </div>
              <Button variant="outline" onClick={applyWorktreeTemplates} disabled={busy || statusLoading}>
                <Wand2 className="size-4" /> 워크트리 템플릿 적용
              </Button>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>에이전트 설정</CardTitle>
              <CardDescription>에이전트 템플릿 적용과 재생성 옵션을 관리합니다.</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <Toggle checked={backupOnPrepare} onChange={changeBackupOnPrepare} label={t("feature.backupOnPrepare")} />
              <Button variant="outline" onClick={applyAgentTemplates} disabled={busy || diffLoading || agentStatusLoading || !agentReady}>
                <Wand2 className="size-4" /> 에이전트 템플릿 적용
              </Button>
            </CardContent>
          </Card>

          <Card className={dangerOpen ? "border-destructive/40" : "border-amber-300/60"}>
            <button
              type="button"
              className="flex w-full items-center justify-between gap-4 p-6 text-left"
              aria-expanded={dangerOpen}
              onClick={() => {
                setDangerOpen((open) => {
                  if (open) setDeleteBranch(false);
                  return !open;
                });
              }}
            >
              <div className="flex min-w-0 items-start gap-3">
                <AlertTriangle className="mt-0.5 size-5 shrink-0 text-amber-600" />
                <div>
                  <CardTitle className={dangerOpen ? "text-destructive" : ""}>위험구역</CardTitle>
                  <CardDescription className="mt-1">{dangerOpen ? t("feature.deleteDesc") : t("feature.dangerCollapsed")}</CardDescription>
                </div>
              </div>
              <ChevronDown className={"size-5 shrink-0 text-muted-foreground transition-transform " + (dangerOpen ? "rotate-180" : "")} />
            </button>
            {dangerOpen && (
              <CardContent className="space-y-4 border-t pt-5">
                <div className="rounded-md border border-destructive/30 bg-destructive/5 p-3 text-sm text-destructive">{t("feature.deleteDesc")}</div>
                <Toggle checked={deleteBranch} onChange={setDeleteBranch} label={t("feature.deleteBranch")} />
                <Button variant="destructive" onClick={deleteFeature} disabled={busy}>
                  <Trash2 className="size-4" /> {t("feature.delete")}
                </Button>
              </CardContent>
            )}
          </Card>
        </div>
      )}
        </div>
      )}
      {fileView && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4"
          onClick={() => setFileView(null)}
        >
          <div
            className="flex max-h-[90vh] w-full max-w-6xl flex-col overflow-hidden rounded-lg border bg-card shadow-xl"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="flex items-start justify-between gap-4 border-b p-4">
              <div className="min-w-0">
                <div className="flex flex-wrap items-center gap-2">
                  <h2 className="truncate text-lg font-semibold">{fileView.change.path}</h2>
                  {fileView.change.risky && <Badge variant="warning">{t("feature.risky")}</Badge>}
                  {fileView.change.masked && <Badge variant="destructive">{t("feature.masked")}</Badge>}
                </div>
                <p className="mt-1 text-sm text-muted-foreground">
                  {t("feature.fileViewDesc", { repo: fileView.repo })}
                </p>
              </div>
              <Button variant="ghost" size="icon" onClick={() => setFileView(null)} title={t("common.close")}>
                <X className="size-4" />
              </Button>
            </div>
            <div className="min-h-0 flex-1 overflow-auto p-4">
              {fileView.loading ? (
                <LoadingState label={t("common.loading")} />
              ) : fileView.error ? (
                <div className="rounded-md border border-destructive/40 bg-destructive/5 p-3 text-sm text-destructive">
                  {fileView.error}
                </div>
              ) : fileView.data ? (
                <div className="grid gap-4 lg:grid-cols-2">
                  <FileViewSidePanel title={t("feature.fileViewAgent")} side={fileView.data.agent} />
                  <FileViewSidePanel title={t("feature.fileViewWorktree")} side={fileView.data.worktree} />
                </div>
              ) : null}
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

function ChangeRow({
  change,
  onView,
  onRestore,
  disabled,
}: {
  change: Change;
  onView: () => void;
  onRestore: () => void;
  disabled: boolean;
}) {
  const { t } = useI18n();
  const type = change.type.toLowerCase();
  const typeColor =
    type === "added"
      ? "text-emerald-600"
      : type === "deleted"
        ? "text-destructive"
        : "text-amber-600";
  return (
    <li className="flex items-center justify-between gap-3 px-3 py-2 text-sm">
      <div className="flex items-center gap-2 truncate">
        <span className={"w-16 shrink-0 font-mono text-xs " + typeColor}>
          {change.type}
        </span>
        <span className="truncate">{change.path}</span>
      </div>
      <div className="flex shrink-0 gap-1">
        {change.risky && <Badge variant="warning">{t("feature.risky")}</Badge>}
        {change.masked && <Badge variant="destructive">{t("feature.masked")}</Badge>}
        <Button
          variant="ghost"
          size="sm"
          className="h-7"
          disabled={disabled}
          onClick={onView}
          title={t("feature.viewFileDiff")}
        >
          <Eye className="size-3.5" /> {t("feature.viewFileDiffShort")}
        </Button>
        <Button
          variant="ghost"
          size="sm"
          className="h-7"
          disabled={disabled}
          onClick={onRestore}
          title={t("feature.restoreFromWorktree")}
        >
          <RotateCcw className="size-3.5" /> {t("feature.restoreFromWorktreeShort")}
        </Button>
      </div>
    </li>
  );
}

function FileViewSidePanel({
  title,
  side,
}: {
  title: string;
  side: { path: string; exists: boolean; content?: string; error?: string };
}) {
  const { t } = useI18n();
  return (
    <div className="min-w-0 overflow-hidden rounded-md border">
      <div className="border-b bg-muted/40 px-3 py-2">
        <div className="font-medium">{title}</div>
        <div className="truncate font-mono text-xs text-muted-foreground" title={side.path}>
          {side.path}
        </div>
      </div>
      {!side.exists ? (
        <div className="p-4 text-sm text-muted-foreground">{t("feature.fileMissing")}</div>
      ) : side.error ? (
        <div className="p-4 text-sm text-destructive">{side.error}</div>
      ) : (
        <pre className="max-h-[60vh] overflow-auto whitespace-pre-wrap break-words bg-slate-950 p-3 font-mono text-xs text-slate-100">
          {side.content ?? ""}
        </pre>
      )}
    </div>
  );
}

function RepoStatusRow({ change }: { change: RepoFileStatus }) {
  const { t } = useI18n();
  const variant =
    change.type === "added"
      ? "success"
      : change.type === "deleted" || change.type === "conflict"
        ? "destructive"
        : change.type === "modified" || change.type === "renamed"
          ? "warning"
          : "outline";
  return (
    <li className="flex items-center gap-3 px-3 py-2 text-sm">
      <Badge variant={variant} title={change.code} className="w-16 justify-center">
        {t(`feature.status.${change.type}`)}
      </Badge>
      <span className="min-w-0 truncate font-mono text-xs">{change.path}</span>
    </li>
  );
}

function LoadingState({ label }: { label: string }) {
  return (
    <div className="flex min-h-28 items-center justify-center gap-2 rounded-md border border-dashed text-sm text-muted-foreground">
      <Loader2 className="size-5 animate-spin" />
      <span>{label}</span>
    </div>
  );
}

function RequestRow({ item }: { item: RequestResult }) {
  const { t } = useI18n();
  const providerLabel =
    item.provider === "github"
      ? "GitHub"
      : item.provider === "gitlab"
        ? "GitLab"
        : t("request.providerUnknown");
  const methodVariant =
    item.method === "api"
      ? "success"
      : item.method === "skipped"
        ? "destructive"
        : "secondary";
  const methodLabel =
    item.method === "api"
      ? t("request.methodApi")
      : item.method === "skipped"
        ? t("request.methodSkipped")
        : t("request.methodBrowser");
  return (
    <li className="flex items-center justify-between gap-3 px-3 py-2 text-sm">
      <div className="min-w-0">
        <div className="flex items-center gap-2">
          <span className="font-medium">{item.repo}</span>
          <Badge variant="outline">{providerLabel}</Badge>
          <Badge variant={methodVariant}>{methodLabel}</Badge>
        </div>
        <div className="mt-0.5 truncate text-xs text-muted-foreground">
          {item.branch} ??{item.target}
          {item.error ? ` 쨌 ${item.error}` : ""}
        </div>
      </div>
      {item.url && (
        <Button
          variant="ghost"
          size="sm"
          className="shrink-0"
          onClick={() => api.OpenURL(item.url)}
        >
          <ExternalLink className="size-4" /> {t("request.open")}
        </Button>
      )}
    </li>
  );
}

function Toggle({
  checked,
  onChange,
  label,
}: {
  checked: boolean;
  onChange: (v: boolean) => void;
  label: string;
}) {
  return (
    <label className="flex items-center gap-2">
      <input
        type="checkbox"
        checked={checked}
        onChange={(e) => onChange(e.target.checked)}
        className="size-4"
      />
      {label}
    </label>
  );
}
