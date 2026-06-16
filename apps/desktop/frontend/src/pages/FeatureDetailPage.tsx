import { useCallback, useEffect, useRef, useState } from "react";
import {
  AlertTriangle,
  AppWindow,
  ArrowLeft,
  Boxes,
  ChevronDown,
  Copy,
  ExternalLink,
  FolderOpen,
  GitCommit,
  GitMerge,
  History,
  Loader2,
  Plus,
  RefreshCw,
  RotateCcw,
  Terminal,
  Trash2,
  Upload,
  Wand2,
} from "lucide-react";
import { api, errMessage } from "@/lib/api";
import type {
  Change,
  DiffResult,
  FeatureDeleteResult,
  FeatureMetadata,
  FeaturePathsResult,
  FeatureStatusResult,
  RepoFileStatus,
  RequestResult,
  RequestResults,
  Repository,
} from "@/lib/types";
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

interface Props {
  name: string;
  onBack: () => void;
  onViewHistory: (feature: string) => void;
}

type Tab = "status" | "agent" | "deliver";

export function FeatureDetailPage({ name, onBack, onViewHistory }: Props) {
  const { notify } = useToast();
  const { t } = useI18n();
  const confirm = useConfirm();
  const [tab, setTab] = useState<Tab>("status");
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
  // Whether re-preparing backs up the existing agent workspace (default true).
  const [backupOnPrepare, setBackupOnPrepare] = useState(() => {
    try {
      return localStorage.getItem("agentsafe.backupOnPrepare") !== "false";
    } catch {
      return true;
    }
  });

  function changeBackupOnPrepare(v: boolean) {
    setBackupOnPrepare(v);
    try {
      localStorage.setItem("agentsafe.backupOnPrepare", v ? "true" : "false");
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
  }, [name]);

  useEffect(() => {
    loadStatus();
    loadCounts();
    loadRepoManager();
  }, [loadStatus, loadCounts, loadRepoManager]);

  const agentReady = prepared ?? (status?.agentReady ?? false);

  useEffect(() => {
    if (
      tab === "agent" &&
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
      setTab("agent");
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

  const openTerminal = () =>
    run(async () => {
      const p = await api.OpenInTerminal(name);
      notify(t("toast.openedPath", { path: p }), "success");
    });

  const openProgram = () =>
    run(async () => {
      const p = await api.OpenInEditor(name, program.trim());
      notify(t("toast.openedPath", { path: p }), "success");
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

  const openRepoProgram = (repo: string) =>
    run(async () => {
      const p = await api.OpenRepoInProgram(name, repo, program.trim());
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

  const tabs: { id: Tab; label: string }[] = [
    { id: "status", label: t("feature.tabStatus") },
    { id: "agent", label: t("feature.tabAgent") },
    { id: "deliver", label: t("feature.tabDeliver") },
  ];
  const repoPathsByName = new Map(
    (featurePaths?.repositories ?? []).map((repo) => [repo.name, repo])
  );

  return (
    <div className="space-y-5">
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
        </div>
      </div>

      {tab === "status" && (
        <Card>
          <CardHeader className="flex-row items-center justify-between space-y-0">
            <div className="min-w-0">
              <CardTitle>{status?.feature ?? name}</CardTitle>
              <CardDescription className="space-y-1">
                <div>{t("feature.branchLabel", { branch: status?.branch ?? "—" })}</div>
                {featurePaths?.worktreePath && (
                  <div
                    className="max-w-xl truncate font-mono text-xs"
                    title={featurePaths.worktreePath}
                  >
                    {t("feature.worktreePath")}: {featurePaths.worktreePath}
                  </div>
                )}
              </CardDescription>
            </div>
            <div className="flex flex-wrap items-center justify-end gap-2">
              <Button
                variant="outline"
                size="sm"
                onClick={openFeatureFolder}
                disabled={busy || !featurePaths?.worktreePath}
              >
                <FolderOpen className="size-4" /> {t("feature.openWorktreeFolder")}
              </Button>
              <Button
                variant="outline"
                size="sm"
                onClick={() => copyPath(featurePaths?.worktreePath)}
                disabled={busy || !featurePaths?.worktreePath}
              >
                <Copy className="size-4" /> {t("feature.copyPath")}
              </Button>
              <Button
                variant="outline"
                size="sm"
                onClick={applyWorktreeTemplates}
                disabled={busy || statusLoading}
              >
                <Wand2 className="size-4" /> {t("feature.applyWorktreeTemplates")}
              </Button>
              <Button
                variant="outline"
                size="sm"
                onClick={chooseProgram}
                disabled={busy}
                title={program}
              >
                <AppWindow className="size-4" /> {t("feature.selectProgram")} (
                {programLabel(program)})
              </Button>
              <Button
                variant="outline"
                size="sm"
                onClick={rebase}
                disabled={busy || statusLoading}
              >
                <GitMerge className="size-4" /> {t("feature.rebase")}
              </Button>
              <Button
                variant="outline"
                size="sm"
                onClick={loadStatus}
                disabled={statusLoading}
              >
                {statusLoading ? (
                  <Loader2 className="size-4 animate-spin" />
                ) : (
                  <RefreshCw className="size-4" />
                )}
                {statusLoading ? t("common.loading") : t("common.refresh")}
              </Button>
            </div>
          </CardHeader>
          <CardContent className="space-y-4">
            {statusLoading && !status ? (
              <LoadingState label={t("feature.loadingStatus")} />
            ) : (status?.repositories ?? []).map((r) => (
              <div key={r.name}>
                <div className="mb-1 flex items-center justify-between gap-2">
                  <div className="flex min-w-0 items-center gap-2">
                    <Boxes className="size-4 shrink-0 text-muted-foreground" />
                    <span className="truncate font-medium">{r.name}</span>
                    {r.status.trim() === "" && (
                      <Badge variant="outline">{t("feature.clean")}</Badge>
                    )}
                    {(histCounts[r.name] ?? 0) > 0 && (
                      <Badge variant="secondary">
                        {t("feature.syncHistoryBadge", {
                          count: histCounts[r.name],
                        })}
                      </Badge>
                    )}
                  </div>
                  <div className="flex shrink-0 items-center gap-1">
                    <Button
                      variant="ghost"
                      size="icon"
                      title={t("feature.openFolder")}
                      disabled={busy}
                      onClick={() => openRepoFolder(r.name)}
                    >
                      <FolderOpen className="size-4" />
                    </Button>
                    <Button
                      variant="ghost"
                      size="icon"
                      title={`${t("feature.openProgram")} (${programLabel(program)})`}
                      disabled={busy}
                      onClick={() => openRepoProgram(r.name)}
                    >
                      <ExternalLink className="size-4" />
                    </Button>
                  </div>
                </div>
                {r.error ? (
                  <div className="rounded-md border border-destructive/40 bg-destructive/5 p-3 text-xs text-destructive">
                    {r.error}
                  </div>
                ) : (
                  (r.changes ?? []).length > 0 && (
                    <ul className="divide-y rounded-md border">
                      {(r.changes ?? []).map((change, i) => (
                        <RepoStatusRow
                          key={`${change.code}-${change.path}-${i}`}
                          change={change}
                        />
                      ))}
                    </ul>
                  )
                )}
              </div>
            ))}
            {!statusLoading && (status?.repositories ?? []).length === 0 && (
              <p className="text-sm text-muted-foreground">{t("feature.noRepos")}</p>
            )}
          </CardContent>
        </Card>
      )}

      {tab === "status" && (
        <Card>
          <CardHeader>
            <CardTitle>{t("feature.repoManagerTitle")}</CardTitle>
            <CardDescription>{t("feature.repoManagerDesc")}</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="space-y-1.5">
              <Label htmlFor="repoPolicy">{t("features.existingBranchLabel")}</Label>
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
              <p className="text-xs text-muted-foreground">
                {t(`features.existingBranchHint.${repoPolicy}`)}
              </p>
            </div>
            <div className="divide-y rounded-md border">
              {configuredRepos.map((repo) => {
                const included = (featureMeta?.repositories ?? []).some(
                  (item) => item.name === repo.Name
                );
                return (
                  <div
                    key={repo.Name}
                    className="flex items-center justify-between gap-3 p-3"
                  >
                    <div className="min-w-0">
                      <div className="font-medium">{repo.Name}</div>
                      <div className="truncate text-xs text-muted-foreground">
                        {repo.DefaultBranch}
                      </div>
                    </div>
                    {included ? (
                      <Button
                        variant="outline"
                        size="sm"
                        disabled={busy}
                        onClick={() => recreateFeatureRepo(repo.Name)}
                      >
                        <RotateCcw className="size-4" />
                        {t("feature.repoRecreate")}
                      </Button>
                    ) : (
                      <Button
                        size="sm"
                        disabled={busy}
                        onClick={() => addFeatureRepo(repo.Name)}
                      >
                        <Plus className="size-4" />
                        {t("feature.repoAdd")}
                      </Button>
                    )}
                  </div>
                );
              })}
              {configuredRepos.length === 0 && (
                <p className="p-3 text-sm text-muted-foreground">
                  {t("feature.noRepos")}
                </p>
              )}
            </div>
          </CardContent>
        </Card>
      )}

      {tab === "status" && (
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
                <CardTitle className={dangerOpen ? "text-destructive" : ""}>
                  {t("feature.dangerZone")}
                </CardTitle>
                <CardDescription className="mt-1">
                  {dangerOpen
                    ? t("feature.deleteDesc")
                    : t("feature.dangerCollapsed")}
                </CardDescription>
              </div>
            </div>
            <ChevronDown
              className={
                "size-5 shrink-0 text-muted-foreground transition-transform " +
                (dangerOpen ? "rotate-180" : "")
              }
            />
          </button>
          {dangerOpen && (
            <CardContent className="space-y-4 border-t pt-5">
              <div className="rounded-md border border-destructive/30 bg-destructive/5 p-3 text-sm text-destructive">
                {t("feature.deleteDesc")}
              </div>
              <Toggle
                checked={deleteBranch}
                onChange={setDeleteBranch}
                label={t("feature.deleteBranch")}
              />
              <Button variant="destructive" onClick={deleteFeature} disabled={busy}>
                <Trash2 className="size-4" /> {t("feature.delete")}
              </Button>
            </CardContent>
          )}
        </Card>
      )}

      {tab === "agent" && (
        <div className="space-y-5">
          <Card>
            <CardHeader>
              <CardTitle>{t("feature.agentTitle")}</CardTitle>
              <CardDescription>{t("feature.agentDesc")}</CardDescription>
            </CardHeader>
            <CardContent className="space-y-3">
              {featurePaths?.agentPath && (
                <div className="flex items-center justify-between gap-3 rounded-md border bg-muted/30 p-3">
                  <div className="min-w-0">
                    <div className="text-xs font-medium text-muted-foreground">
                      {t("feature.agentPath")}
                    </div>
                    <div className="truncate font-mono text-xs" title={featurePaths.agentPath}>
                      {featurePaths.agentPath}
                    </div>
                  </div>
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => copyPath(featurePaths.agentPath)}
                    disabled={busy}
                  >
                    <Copy className="size-4" /> {t("feature.copyPath")}
                  </Button>
                </div>
              )}
              {status?.agentNeedsPrepare && (
                <div className="rounded-md border border-amber-300 bg-amber-50 p-3 text-sm text-amber-900">
                  {t("feature.agentNeedsPrepare")}
                </div>
              )}
              {statusLoading && !status ? (
                <LoadingState label={t("feature.loadingAgent")} />
              ) : (
                <>
              <div className="flex flex-wrap items-center gap-2">
              <Button onClick={prepare} disabled={busy || diffLoading}>
                <Wand2 className="size-4" />{" "}
                {agentReady ? t("feature.regenerate") : t("feature.prepare")}
              </Button>
              {agentReady && (
                <>
                  <Button
                    variant="outline"
                    onClick={() => loadDiff(true)}
                    disabled={busy || diffLoading}
                  >
                    {diffLoading ? (
                      <Loader2 className="size-4 animate-spin" />
                    ) : (
                      <RefreshCw className="size-4" />
                    )}
                    {diffLoading
                      ? t("feature.refreshingDiff")
                      : t("feature.refreshDiff")}
                  </Button>
                  <Button
                    variant="outline"
                    onClick={applyAgentTemplates}
                    disabled={busy || diffLoading}
                  >
                    <Wand2 className="size-4" /> {t("feature.applyAgentTemplates")}
                  </Button>
                  <Button
                    variant="secondary"
                    onClick={openTerminal}
                    disabled={busy}
                  >
                    <Terminal className="size-4" /> {t("feature.terminal")}
                  </Button>
                  <Button
                    variant="secondary"
                    onClick={openProgram}
                    disabled={busy}
                    title={program}
                  >
                    <ExternalLink className="size-4" /> {t("feature.openProgram")}{" "}
                    ({programLabel(program)})
                  </Button>
                  <Button
                    variant="outline"
                    onClick={chooseProgram}
                    disabled={busy}
                  >
                    <AppWindow className="size-4" /> {t("feature.selectProgram")}
                  </Button>
                  <Button
                    variant="outline"
                    onClick={() => onViewHistory(name)}
                    disabled={busy}
                  >
                    <History className="size-4" /> {t("feature.viewHistory")}
                  </Button>
                  <Button variant="destructive" onClick={del} disabled={busy}>
                    <Trash2 className="size-4" /> {t("common.delete")}
                  </Button>
                </>
              )}
              </div>
              <Toggle
                checked={backupOnPrepare}
                onChange={changeBackupOnPrepare}
                label={t("feature.backupOnPrepare")}
              />
              <div className="divide-y rounded-md border">
                {(status?.repositories ?? []).map((repo) => {
                  const paths = repoPathsByName.get(repo.name);
                  return (
                    <div
                      key={repo.name}
                      className="flex items-center justify-between gap-3 p-3"
                    >
                      <div className="min-w-0">
                        <div className="font-medium">{repo.name}</div>
                        {paths?.agentPath && (
                          <div
                            className="mt-1 truncate font-mono text-xs text-muted-foreground"
                            title={paths.agentPath}
                          >
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
                        <Button
                          variant="ghost"
                          size="icon"
                          title={t("feature.copyPath")}
                          disabled={busy || !paths?.agentPath}
                          onClick={() => copyPath(paths?.agentPath)}
                        >
                          <Copy className="size-4" />
                        </Button>
                        <Button
                          variant={repo.agentReady ? "outline" : "default"}
                          size="sm"
                          disabled={busy}
                          onClick={() => prepareRepo(repo.name)}
                        >
                          {preparingRepo === repo.name ? (
                            <Loader2 className="size-4 animate-spin" />
                          ) : (
                            <Wand2 className="size-4" />
                          )}
                          {repo.agentReady
                            ? t("feature.agentRepoRegenerate")
                            : t("feature.agentRepoPrepare")}
                        </Button>
                      </div>
                    </div>
                  );
                })}
              </div>
                </>
              )}
            </CardContent>
          </Card>

          {agentReady && (
          <>
          <Card>
            <CardHeader>
              <CardTitle>{t("feature.diffTitle")}</CardTitle>
              <CardDescription>
                {diffLoading && !diff
                  ? t("feature.loadingDiff")
                  : diff
                  ? t("feature.changeCount", {
                      count: (diff.repositories ?? []).reduce(
                        (n, r) => n + (r.changes?.length ?? 0),
                        0
                      ),
                    })
                  : t("feature.diffHint")}
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              {diffLoading && !diff ? (
                <LoadingState label={t("feature.loadingDiff")} />
              ) : (diff?.repositories ?? []).map((r) => (
                <div key={r.name}>
                  <div className="mb-1 flex items-center gap-2 font-medium">
                    {r.name}
                    {(histCounts[r.name] ?? 0) > 0 && (
                      <Badge variant="secondary">
                        {t("feature.syncHistoryBadge", {
                          count: histCounts[r.name],
                        })}
                      </Badge>
                    )}
                  </div>
                  {(r.changes ?? []).length === 0 ? (
                    <p className="text-sm text-muted-foreground">{t("feature.noChanges")}</p>
                  ) : (
                    <ul className="divide-y rounded-md border">
                      {(r.changes ?? []).map((c, i) => (
                        <ChangeRow key={c.path + i} change={c} />
                      ))}
                    </ul>
                  )}
                </div>
              ))}
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>{t("feature.syncTitle")}</CardTitle>
              <CardDescription>{t("feature.syncDesc")}</CardDescription>
            </CardHeader>
            <CardContent className="space-y-3">
              <div className="flex flex-wrap gap-4 text-sm">
                <Toggle
                  checked={dryRun}
                  onChange={setDryRun}
                  label={t("feature.dryRun")}
                />
                <Toggle
                  checked={includeRisky}
                  onChange={setIncludeRisky}
                  label={t("feature.includeRisky")}
                />
                <Toggle
                  checked={allowMasked}
                  onChange={setAllowMasked}
                  label={t("feature.allowMasked")}
                />
              </div>
              <Button onClick={sync} disabled={busy}>
                <Upload className="size-4" />{" "}
                {dryRun ? t("feature.previewSync") : t("feature.sync")}
              </Button>
            </CardContent>
          </Card>
          </>
          )}
        </div>
      )}

      {tab === "deliver" && (
        <div className="space-y-5">
          {(() => {
            const repos = status?.repositories ?? [];
            const anyDirty = repos.some((r) => (r.changes ?? []).length > 0);
            const anyPushable = repos.some((r) => (r.ahead ?? 0) > 0);
            const actionable = repos.filter(
              (r) => (r.changes ?? []).length > 0 || (r.ahead ?? 0) > 0 || !!r.error
            );
            return (
              <>
                {/* Bulk commit / push */}
                <Card>
                  <CardHeader className="flex-row items-center justify-between space-y-0">
                    <div>
                      <CardTitle>{t("feature.bulkCommitTitle")}</CardTitle>
                      <CardDescription>{t("feature.bulkCommitDesc")}</CardDescription>
                    </div>
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={loadStatus}
                      disabled={statusLoading || busy}
                    >
                      {statusLoading ? (
                        <Loader2 className="size-4 animate-spin" />
                      ) : (
                        <RefreshCw className="size-4" />
                      )}
                      {statusLoading ? t("common.loading") : t("common.refresh")}
                    </Button>
                  </CardHeader>
                  <CardContent className="space-y-4">
                    <div className="space-y-1.5">
                      <Label htmlFor="cm">{t("feature.messageLabel")}</Label>
                      <Input
                        id="cm"
                        value={commitMsg}
                        onChange={(e) => setCommitMsg(e.target.value)}
                        placeholder="feat: add coupon v2"
                      />
                    </div>
                    <div className="flex gap-2">
                      <Button
                        onClick={() => doCommit("", commitMsg)}
                        disabled={busy || !commitMsg.trim() || !anyDirty}
                      >
                        {actingRepo === "*" ? (
                          <Loader2 className="size-4 animate-spin" />
                        ) : (
                          <GitCommit className="size-4" />
                        )}
                        {t("feature.commitAll")}
                      </Button>
                      <Button
                        variant="secondary"
                        onClick={() => push()}
                        disabled={busy || !anyPushable}
                      >
                        <Upload className="size-4" /> {t("feature.pushAll")}
                      </Button>
                    </div>
                  </CardContent>
                </Card>

                {/* Per-repository commit / push */}
                <Card>
                  <CardHeader>
                    <CardTitle>{t("feature.perRepoCommitTitle")}</CardTitle>
                    <CardDescription>{t("feature.perRepoCommitDesc")}</CardDescription>
                  </CardHeader>
                  <CardContent className="space-y-4">
                    {statusLoading && !status ? (
                      <LoadingState label={t("feature.loadingStatus")} />
                    ) : actionable.length === 0 ? (
                      <p className="text-sm text-muted-foreground">
                        {t("feature.noChangesToCommit")}
                      </p>
                    ) : (
                      actionable.map((r) => {
                        const changeCount = (r.changes ?? []).length;
                        const dirty = changeCount > 0;
                        const ahead = r.ahead ?? 0;
                        const msg = repoMsgs[r.name] ?? "";
                        const rowBusy = actingRepo === r.name;
                        return (
                          <div key={r.name} className="space-y-2 rounded-md border p-3">
                            <div className="flex items-center gap-2">
                              <Boxes className="size-4 text-muted-foreground" />
                              <span className="font-medium">{r.name}</span>
                              {dirty && (
                                <Badge variant="warning">
                                  {t("feature.repoChanges", { count: changeCount })}
                                </Badge>
                              )}
                              {ahead > 0 && (
                                <Badge variant="secondary">
                                  {t("feature.repoAhead", { count: ahead })}
                                </Badge>
                              )}
                            </div>
                            {r.error ? (
                              <div className="rounded-md border border-destructive/40 bg-destructive/5 p-3 text-xs text-destructive">
                                {r.error}
                              </div>
                            ) : dirty ? (
                              <ul className="divide-y rounded-md border">
                                {(r.changes ?? []).map((change, i) => (
                                  <RepoStatusRow
                                    key={`${change.code}-${change.path}-${i}`}
                                    change={change}
                                  />
                                ))}
                              </ul>
                            ) : null}
                            <div className="flex items-end gap-2">
                              <div className="flex-1 space-y-1.5">
                                <Label htmlFor={`cm-${r.name}`}>
                                  {t("feature.messageLabel")}
                                </Label>
                                <Input
                                  id={`cm-${r.name}`}
                                  value={msg}
                                  onChange={(e) =>
                                    setRepoMsgs((m) => ({ ...m, [r.name]: e.target.value }))
                                  }
                                  placeholder="feat: ..."
                                />
                              </div>
                              <Button
                                onClick={() => doCommit(r.name, msg)}
                                disabled={busy || !dirty || !msg.trim()}
                              >
                                {rowBusy ? (
                                  <Loader2 className="size-4 animate-spin" />
                                ) : (
                                  <GitCommit className="size-4" />
                                )}
                                {t("feature.commitRepo")}
                              </Button>
                              <Button
                                variant="secondary"
                                onClick={() => push(r.name)}
                                disabled={busy || ahead === 0}
                              >
                                <Upload className="size-4" /> {t("feature.pushRepo")}
                              </Button>
                            </div>
                          </div>
                        );
                      })
                    )}
                  </CardContent>
                </Card>
              </>
            );
          })()}

          <Card>
            <CardHeader>
              <CardTitle>{t("feature.mrTitle")}</CardTitle>
              <CardDescription>{t("feature.mrDesc")}</CardDescription>
            </CardHeader>
            <CardContent className="space-y-3">
              <div className="space-y-1.5">
                <Label htmlFor="mti">{t("feature.mrTitleLabel")}</Label>
                <Input
                  id="mti"
                  value={mrTitle}
                  onChange={(e) => setMrTitle(e.target.value)}
                  placeholder={`[${name}] ...`}
                />
              </div>
              <Button onClick={sendRequests} disabled={busy}>
                <GitMerge className="size-4" /> {t("feature.generateMr")}
              </Button>
              {requests && (
                <ul className="divide-y rounded-md border">
                  {(requests.items ?? []).length === 0 ? (
                    <li className="px-3 py-2 text-sm text-muted-foreground">
                      {t("request.empty")}
                    </li>
                  ) : (
                    (requests.items ?? []).map((r) => (
                      <RequestRow key={r.repo} item={r} />
                    ))
                  )}
                </ul>
              )}
            </CardContent>
          </Card>
        </div>
      )}
    </div>
  );
}

function ChangeRow({ change }: { change: Change }) {
  const { t } = useI18n();
  const typeColor =
    change.type === "added"
      ? "text-emerald-600"
      : change.type === "deleted"
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
      </div>
    </li>
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
          {item.branch} → {item.target}
          {item.error ? ` · ${item.error}` : ""}
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
