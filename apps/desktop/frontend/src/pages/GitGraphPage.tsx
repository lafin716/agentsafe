import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  AlertTriangle,
  ChevronDown,
  Copy,
  Download,
  FolderOpen,
  GitBranch,
  GitMerge,
  Loader2,
  Plus,
  RefreshCw,
  RotateCcw,
  Terminal as TerminalIcon,
  Upload,
  X,
} from "lucide-react";
import { api, errMessage } from "@/lib/api";
import type {
  BranchWorktree,
  CommitFileChange,
  CommitGraph,
  Config,
  GraphCommit,
  GraphRef,
  IntegrationReadiness,
  RefTip,
  TerminalSession,
} from "@/lib/types";
import { layoutGraph, laneColor, laneX, linkPath } from "@/lib/gitgraph";
import { summarizeIntegration } from "@/lib/integration";
import { summarizePush } from "@/lib/push";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { useToast } from "@/components/ui/toast";
import { useI18n } from "@/i18n/I18nProvider";
import { runtime } from "@/components/TerminalPanel";

// The Commit Graph for one repository.
//
// Scope is a repository, not a Feature: commits only relate to each other inside
// one object database. Because every Repo Worktree is created from its Main Clone
// with `git worktree add`, they share that database, so this one read already
// shows every Feature branch — and each is marked with the Repo Worktree that has
// it checked out.
//
// Everything that writes history runs in a Repo Worktree. The Main Clone is never
// rebased, merged into, or committed to; see docs/adr/0001.

const ROW_HEIGHT = 34;
const LANE_WIDTH = 14;
const MAX_GUTTER_LANES = 12;

type Props = {
  config: Config | null;
  /** Repository this tab is pinned to. Empty means "pick one". */
  repo?: string;
  /** Switching repository opens/activates that repository's own tab. */
  onSelectRepo: (repo: string) => void;
  onOpenTerminal: (session: TerminalSession) => void;
  onOpenFeature: (feature: string) => void;
  /** Opens the create-feature flow with a starting point pre-filled. */
  onCreateFeature: (base: string) => void;
};

type PendingIntegration = {
  kind: "rebase" | "merge";
  /** Feature owning the branch being moved. */
  feature: string;
  branch: string;
  /** Ref to integrate from — the one the user clicked. */
  upstream: string;
  readiness: IntegrationReadiness | null;
  allRepos: boolean;
};

type MenuState =
  | { kind: "commit"; x: number; y: number; commit: GraphCommit }
  | { kind: "ref"; x: number; y: number; ref: GraphRef; sha: string }
  | null;

export function GitGraphPage({
  config,
  repo,
  onSelectRepo,
  onOpenTerminal,
  onOpenFeature,
  onCreateFeature,
}: Props) {
  const { notify } = useToast();
  const { t } = useI18n();
  const repos = config?.Repositories ?? [];
  const selected = repo || repos[0]?.Name || "";

  const [graph, setGraph] = useState<CommitGraph | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [allBranches, setAllBranches] = useState(() =>
    loadAllBranches(config?.Workspace.Root ?? "", selected)
  );
  const [limit, setLimit] = useState(0);
  const [extraRefs, setExtraRefs] = useState<string[]>([]);
  const [selectedSha, setSelectedSha] = useState("");
  const [files, setFiles] = useState<Record<string, CommitFileChange[]>>({});
  const [filesLoading, setFilesLoading] = useState(false);
  const [menu, setMenu] = useState<MenuState>(null);
  const [pending, setPending] = useState<PendingIntegration | null>(null);
  const [busy, setBusy] = useState(false);
  const [loadedAt, setLoadedAt] = useState<number | null>(null);

  const root = config?.Workspace.Root ?? "";

  const load = useCallback(async () => {
    if (!selected) return;
    setLoading(true);
    setError("");
    try {
      const next = await api.RepoCommitGraph(
        selected,
        allBranches,
        limit,
        extraRefs
      );
      setGraph(next);
      setLoadedAt(Date.now());
    } catch (e) {
      setGraph(null);
      setError(errMessage(e));
    } finally {
      setLoading(false);
    }
  }, [selected, allBranches, limit, extraRefs]);

  useEffect(() => {
    void load();
  }, [load]);

  // Anything that moves a branch elsewhere in the app — a commit, push, sync or
  // rebase from the worktree screens — ends as a task. Re-reading on task:end is
  // what keeps an open graph tab from showing a stale picture, without polling a
  // git subprocess on a timer.
  useEffect(() => {
    const off = runtime()?.EventsOn("task:end", () => {
      void load();
    });
    return () => off?.();
  }, [load]);

  useEffect(() => {
    saveAllBranches(root, selected, allBranches);
  }, [root, selected, allBranches]);

  // Reset the window when the repository or ref selection changes: a limit or
  // extra ref chosen for one repository means nothing for the next.
  const lastKey = useRef("");
  useEffect(() => {
    const key = `${root}::${selected}`;
    if (lastKey.current && lastKey.current !== key) {
      setLimit(0);
      setExtraRefs([]);
      setSelectedSha("");
      setFiles({});
    }
    lastKey.current = key;
  }, [root, selected]);

  useEffect(() => {
    if (!menu) return;
    const dismiss = () => setMenu(null);
    window.addEventListener("click", dismiss);
    window.addEventListener("resize", dismiss);
    return () => {
      window.removeEventListener("click", dismiss);
      window.removeEventListener("resize", dismiss);
    };
  }, [menu]);

  const commits = graph?.commits ?? [];
  const layout = useMemo(() => layoutGraph(commits), [commits]);
  const gutterLanes = Math.min(Math.max(layout.laneCount, 1), MAX_GUTTER_LANES);
  const gutterWidth = gutterLanes * LANE_WIDTH + LANE_WIDTH;

  // Branch → Repo Worktree, so a ref can say which Feature owns it and whether
  // an Interrupted Integration is open there.
  const worktreeByBranch = useMemo(() => {
    const map = new Map<string, BranchWorktree>();
    for (const wt of graph?.worktrees ?? []) map.set(wt.branch, wt);
    return map;
  }, [graph]);

  const openIntegrations = (graph?.worktrees ?? []).filter(
    (wt) => wt.integration?.kind
  );

  const selectCommit = useCallback(
    async (sha: string) => {
      setSelectedSha(sha);
      if (files[sha] || !selected) return;
      setFilesLoading(true);
      try {
        const changes = await api.CommitFileChanges(selected, sha);
        setFiles((prev) => ({ ...prev, [sha]: changes ?? [] }));
      } catch (e) {
        notify(errMessage(e), "error");
      } finally {
        setFilesLoading(false);
      }
    },
    [files, selected, notify]
  );

  const startIntegration = useCallback(
    async (kind: "rebase" | "merge", branch: string, upstream: string) => {
      const wt = worktreeByBranch.get(branch);
      if (!wt) {
        notify(t("git.noWorktreeForBranch", { branch }), "error");
        return;
      }
      setPending({
        kind,
        feature: wt.feature,
        branch,
        upstream,
        readiness: null,
        allRepos: false,
      });
      try {
        // Checked when the dialog opens, not on every graph read: comparing agent
        // workspaces walks the filesystem.
        const readiness = await api.CheckIntegration(wt.feature, "");
        setPending((prev) => (prev ? { ...prev, readiness } : prev));
      } catch (e) {
        notify(errMessage(e), "error");
        setPending(null);
      }
    },
    [worktreeByBranch, notify, t]
  );

  const runIntegration = useCallback(async () => {
    if (!pending) return;
    const { kind, feature, upstream, allRepos } = pending;
    setBusy(true);
    try {
      const res =
        kind === "rebase"
          ? await api.RebaseFeature(feature, allRepos ? "" : selected, upstream)
          : await api.MergeFeature(feature, allRepos ? "" : selected, upstream);
      const s = summarizeIntegration(res);
      notify(t(s.messageKey, s.params), s.level);
      setPending(null);
      await load();
    } catch (e) {
      notify(errMessage(e), "error");
    } finally {
      setBusy(false);
    }
  }, [pending, selected, notify, t, load]);

  const resolveIntegration = useCallback(
    async (wt: BranchWorktree, action: "continue" | "abort") => {
      setBusy(true);
      try {
        const res =
          action === "continue"
            ? await api.ContinueIntegration(wt.feature, selected)
            : await api.AbortIntegration(wt.feature, selected);
        const s = summarizeIntegration(res);
        notify(t(s.messageKey, s.params), s.level);
        await load();
      } catch (e) {
        notify(errMessage(e), "error");
      } finally {
        setBusy(false);
      }
    },
    [selected, notify, t, load]
  );

  const push = useCallback(
    async (branch: string, force: boolean) => {
      const wt = worktreeByBranch.get(branch);
      if (!wt) return;
      setBusy(true);
      try {
        // The result is per repository; a failed push is reported in it rather
        // than thrown, so claiming success without looking would be wrong.
        const res = await api.PushFeature(wt.feature, selected, force);
        const summary = summarizePush(res);
        if (summary.failed.length > 0) {
          notify(t(summary.messageKey, summary.params), summary.level);
        } else {
          notify(t(force ? "git.forcePushed" : "git.pushed", { branch }), "success");
        }
        await load();
      } catch (e) {
        notify(errMessage(e), "error");
      } finally {
        setBusy(false);
      }
    },
    [worktreeByBranch, selected, notify, t, load]
  );

  const openWorktreeTerminal = useCallback(
    async (wt: BranchWorktree) => {
      try {
        const session = await api.TerminalOpen(joinPath(root, wt.path));
        onOpenTerminal(session);
      } catch (e) {
        notify(errMessage(e), "error");
      }
    },
    [root, onOpenTerminal, notify]
  );

  const fetchRepo = useCallback(async () => {
    setBusy(true);
    try {
      // Reuses the workspace Pull for this repository, so credentials, progress
      // and the auth dialog behave exactly as they do on the workspace screen.
      await api.PullRepo(selected);
      await load();
    } catch (e) {
      notify(errMessage(e), "error");
    } finally {
      setBusy(false);
    }
  }, [selected, load, notify]);

  if (repos.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">{t("git.noRepos")}</p>
    );
  }

  const selectedCommit = commits.find((c) => c.sha === selectedSha);

  return (
    <div className="flex h-full min-h-0 flex-col gap-3">
      <header className="flex flex-wrap items-center gap-2">
        <label className="sr-only" htmlFor="graph-repo">
          {t("git.repoLabel")}
        </label>
        <div className="relative">
          <select
            id="graph-repo"
            value={selected}
            onChange={(e) => onSelectRepo(e.target.value)}
            className="h-9 appearance-none rounded-md border bg-background pl-3 pr-8 text-sm"
          >
            {repos.map((r) => (
              <option key={r.Name} value={r.Name}>
                {r.Name}
              </option>
            ))}
          </select>
          <ChevronDown className="pointer-events-none absolute right-2 top-2.5 size-4 opacity-50" />
        </div>

        <label className="flex items-center gap-1.5 text-sm text-muted-foreground">
          <input
            type="checkbox"
            checked={allBranches}
            onChange={(e) => setAllBranches(e.target.checked)}
          />
          {t("git.allBranches")}
        </label>

        <div className="ml-auto flex items-center gap-2">
          {loadedAt && !loading && (
            <span className="text-xs text-muted-foreground">
              {t("git.refreshedAt", { time: formatTime(loadedAt) })}
            </span>
          )}
          <Button variant="outline" size="sm" onClick={() => void load()} disabled={loading}>
            <RefreshCw className={cn("size-4", loading && "animate-spin")} />
            {t("common.refresh")}
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={() => void fetchRepo()}
            disabled={busy}
            title={t("git.fetchHint")}
          >
            <Download className="size-4" />
            {t("git.fetch")}
          </Button>
        </div>
      </header>

      {graph && (
        <p className="text-xs text-muted-foreground">
          {t("git.mainCloneOn", {
            branch: graph.currentBranch || "—",
            base: graph.baseBranch,
          })}
        </p>
      )}

      {openIntegrations.map((wt) => (
        <InterruptedIntegrationBanner
          key={wt.branch}
          worktree={wt}
          busy={busy}
          onContinue={() => void resolveIntegration(wt, "continue")}
          onAbort={() => void resolveIntegration(wt, "abort")}
          onOpenTerminal={() => void openWorktreeTerminal(wt)}
        />
      ))}

      {graph && (graph.outsideWindow ?? []).length > 0 && (
        <OutsideWindowNotice
          refs={graph.outsideWindow ?? []}
          limit={graph.limit}
          onLoadRef={(name) =>
            setExtraRefs((prev) => (prev.includes(name) ? prev : [...prev, name]))
          }
        />
      )}

      {error && (
        <div className="rounded-md border border-destructive/40 bg-destructive/10 p-3 text-sm">
          {error}
        </div>
      )}

      <div className="flex min-h-0 flex-1 flex-col overflow-hidden rounded-md border">
        <div className="min-h-0 flex-1 overflow-auto">
          {loading && commits.length === 0 ? (
            <div className="flex items-center gap-2 p-6 text-sm text-muted-foreground">
              <Loader2 className="size-4 animate-spin" />
              {t("git.loading")}
            </div>
          ) : commits.length === 0 ? (
            <p className="p-6 text-sm text-muted-foreground">{t("git.empty")}</p>
          ) : (
            <table className="w-full border-collapse text-sm">
              <tbody>
                {layout.rows.map((row) => {
                  const active = row.commit.sha === selectedSha;
                  return (
                    <tr
                      key={`${row.commit.sha}-${row.lane}`}
                      onClick={() => void selectCommit(row.commit.sha)}
                      onContextMenu={(e) => {
                        e.preventDefault();
                        setMenu({
                          kind: "commit",
                          x: e.clientX,
                          y: e.clientY,
                          commit: row.commit,
                        });
                      }}
                      className={cn(
                        "cursor-pointer border-b last:border-b-0",
                        active ? "bg-secondary" : "hover:bg-accent/50"
                      )}
                    >
                      <td
                        className="p-0 align-top"
                        style={{ width: gutterWidth, minWidth: gutterWidth }}
                      >
                        <svg
                          width={gutterWidth}
                          height={ROW_HEIGHT}
                          className="block"
                          aria-hidden="true"
                        >
                          {row.links.map((link, i) => (
                            <path
                              key={i}
                              d={linkPath(link, ROW_HEIGHT, LANE_WIDTH)}
                              fill="none"
                              strokeWidth={2}
                              stroke={laneColor(
                                link.fromNode ? link.toLane : link.fromLane
                              )}
                            />
                          ))}
                          <circle
                            cx={laneX(row.lane, LANE_WIDTH)}
                            cy={ROW_HEIGHT / 2}
                            r={row.commit.isHead ? 5 : 4}
                            fill={
                              row.commit.isHead
                                ? "hsl(var(--background))"
                                : laneColor(row.lane)
                            }
                            stroke={laneColor(row.lane)}
                            strokeWidth={row.commit.isHead ? 2.5 : 1}
                          />
                        </svg>
                      </td>
                      <td className="whitespace-nowrap py-1 pr-3 align-middle">
                        <div className="flex items-center gap-1">
                          {(row.commit.refs ?? []).map((ref) => (
                            <RefChip
                              key={`${ref.kind}:${ref.name}`}
                              ref_={ref}
                              worktree={worktreeByBranch.get(ref.name)}
                              onContextMenu={(e) => {
                                e.preventDefault();
                                e.stopPropagation();
                                setMenu({
                                  kind: "ref",
                                  x: e.clientX,
                                  y: e.clientY,
                                  ref,
                                  sha: row.commit.sha,
                                });
                              }}
                            />
                          ))}
                        </div>
                      </td>
                      <td className="max-w-0 truncate py-1 pr-3 align-middle">
                        {row.commit.subject}
                      </td>
                      <td className="whitespace-nowrap py-1 pr-3 align-middle text-xs text-muted-foreground">
                        {row.commit.authorName}
                      </td>
                      <td className="whitespace-nowrap py-1 pr-3 align-middle text-xs text-muted-foreground">
                        {relativeTime(row.commit.authorDate, t)}
                      </td>
                      <td className="whitespace-nowrap py-1 pr-3 align-middle font-mono text-xs text-muted-foreground">
                        {row.commit.sha.slice(0, 7)}
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          )}
        </div>

        {graph?.truncated && (
          <div className="flex shrink-0 justify-center border-t p-2">
            <Button
              variant="outline"
              size="sm"
              onClick={() => setLimit((graph.limit || 300) + 300)}
              disabled={loading}
            >
              {t("git.loadMore", { count: 300 })}
            </Button>
          </div>
        )}
      </div>

      {selectedCommit && (
        <CommitDetail
          commit={selectedCommit}
          files={files[selectedCommit.sha]}
          loading={filesLoading}
          onClose={() => setSelectedSha("")}
          onCopy={(text) => void api.CopyText(text)}
        />
      )}

      {menu?.kind === "commit" && (
        <ContextMenu x={menu.x} y={menu.y}>
          <MenuItem
            icon={Plus}
            label={t("git.createFeatureHere")}
            onClick={() => onCreateFeature(menu.commit.sha)}
          />
          <MenuItem
            icon={Copy}
            label={t("git.copySha")}
            onClick={() => void api.CopyText(menu.commit.sha)}
          />
        </ContextMenu>
      )}

      {menu?.kind === "ref" && (
        <RefMenu
          x={menu.x}
          y={menu.y}
          ref_={menu.ref}
          worktrees={graph?.worktrees ?? []}
          worktreeByBranch={worktreeByBranch}
          t={t}
          onRebase={(branch) => void startIntegration("rebase", branch, menu.ref.name)}
          onMerge={(branch) => void startIntegration("merge", branch, menu.ref.name)}
          onPush={(branch, force) => void push(branch, force)}
          onOpenFeature={onOpenFeature}
          onCheckoutMainClone={async () => {
            setBusy(true);
            try {
              await api.CheckoutRepoBranch(selected, menu.ref.name);
              await load();
            } catch (e) {
              notify(errMessage(e), "error");
            } finally {
              setBusy(false);
            }
          }}
        />
      )}

      {pending && (
        <IntegrationDialog
          pending={pending}
          repo={selected}
          busy={busy}
          onToggleAllRepos={(allRepos) =>
            setPending((prev) => (prev ? { ...prev, allRepos } : prev))
          }
          onCancel={() => setPending(null)}
          onConfirm={() => void runIntegration()}
          onOpenFeature={onOpenFeature}
        />
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------

function RefChip({
  ref_,
  worktree,
  onContextMenu,
}: {
  ref_: GraphRef;
  worktree?: BranchWorktree;
  onContextMenu: (e: React.MouseEvent) => void;
}) {
  const variant =
    ref_.kind === "tag"
      ? "outline"
      : ref_.kind === "remote"
        ? "secondary"
        : "default";
  return (
    <span onContextMenu={onContextMenu} className="inline-flex items-center">
      <Badge variant={variant} className="gap-1 font-mono text-[11px]">
        {ref_.kind === "tag" ? null : <GitBranch className="size-3" />}
        {ref_.name}
        {worktree && (
          <span
            className="opacity-70"
            title={worktree.feature}
          >
            ▸{worktree.feature}
          </span>
        )}
      </Badge>
    </span>
  );
}

function InterruptedIntegrationBanner({
  worktree,
  busy,
  onContinue,
  onAbort,
  onOpenTerminal,
}: {
  worktree: BranchWorktree;
  busy: boolean;
  onContinue: () => void;
  onAbort: () => void;
  onOpenTerminal: () => void;
}) {
  const { t } = useI18n();
  const state = worktree.integration;
  const paths = state.conflictPaths ?? [];
  return (
    <div className="rounded-md border border-amber-500/50 bg-amber-500/10 p-3 text-sm">
      <div className="flex items-center gap-2 font-medium">
        <AlertTriangle className="size-4 shrink-0" />
        {t("git.integrationInProgress", {
          kind: state.kind ?? "",
          branch: state.branch || worktree.branch,
          feature: worktree.feature,
        })}
      </div>
      <p className="mt-1 text-xs text-muted-foreground">
        {state.total
          ? t("git.integrationProgress", {
              step: state.step ?? 0,
              total: state.total,
              count: paths.length,
            })
          : t("git.integrationConflictCount", { count: paths.length })}
      </p>
      {paths.length > 0 && (
        <ul className="mt-2 space-y-0.5 font-mono text-xs">
          {paths.slice(0, 10).map((p) => (
            <li key={p}>• {p}</li>
          ))}
          {paths.length > 10 && (
            <li className="text-muted-foreground">
              {t("git.andMore", { count: paths.length - 10 })}
            </li>
          )}
        </ul>
      )}
      <div className="mt-3 flex flex-wrap gap-2">
        <Button size="sm" variant="outline" onClick={onOpenTerminal} disabled={busy}>
          <TerminalIcon className="size-4" />
          {t("git.openTerminal")}
        </Button>
        <Button size="sm" onClick={onContinue} disabled={busy}>
          {t("git.continueIntegration")}
        </Button>
        <Button size="sm" variant="outline" onClick={onAbort} disabled={busy}>
          <RotateCcw className="size-4" />
          {t("git.abortIntegration")}
        </Button>
      </div>
    </div>
  );
}

function OutsideWindowNotice({
  refs,
  limit,
  onLoadRef,
}: {
  refs: RefTip[];
  limit: number;
  onLoadRef: (name: string) => void;
}) {
  const { t } = useI18n();
  return (
    <div className="rounded-md border bg-muted/40 p-3 text-xs">
      <p className="font-medium">
        {t("git.outsideWindow", { count: refs.length, limit })}
      </p>
      <div className="mt-1.5 flex flex-wrap gap-1.5">
        {refs.map((ref) => (
          <button
            key={ref.name}
            type="button"
            onClick={() => onLoadRef(ref.name)}
            className="rounded border px-1.5 py-0.5 font-mono hover:bg-accent"
            title={t("git.loadRefTip", { name: ref.name })}
          >
            {ref.name}
          </button>
        ))}
      </div>
    </div>
  );
}

function CommitDetail({
  commit,
  files,
  loading,
  onClose,
  onCopy,
}: {
  commit: GraphCommit;
  files?: CommitFileChange[];
  loading: boolean;
  onClose: () => void;
  onCopy: (text: string) => void;
}) {
  const { t } = useI18n();
  return (
    <div className="shrink-0 overflow-auto rounded-md border p-3" style={{ maxHeight: "40%" }}>
      <div className="flex items-start justify-between gap-2">
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <code className="font-mono text-xs">{commit.sha}</code>
            <button
              type="button"
              onClick={() => onCopy(commit.sha)}
              className="rounded p-0.5 text-muted-foreground hover:bg-accent"
              title={t("git.copySha")}
            >
              <Copy className="size-3.5" />
            </button>
          </div>
          <p className="mt-1 text-xs text-muted-foreground">
            {commit.authorName} &lt;{commit.authorEmail}&gt; ·{" "}
            {formatDate(commit.authorDate)}
          </p>
          <p className="mt-0.5 font-mono text-xs text-muted-foreground">
            {t("git.parents")}:{" "}
            {commit.parents.length === 0
              ? t("git.rootCommit")
              : commit.parents.map((p) => p.slice(0, 7)).join(" ")}
          </p>
        </div>
        <button
          type="button"
          onClick={onClose}
          className="rounded p-1 text-muted-foreground hover:bg-accent"
          title={t("common.close")}
        >
          <X className="size-4" />
        </button>
      </div>

      <p className="mt-2 whitespace-pre-wrap text-sm">{commit.subject}</p>

      <div className="mt-3 border-t pt-2">
        {loading && !files ? (
          <p className="flex items-center gap-2 text-xs text-muted-foreground">
            <Loader2 className="size-3.5 animate-spin" />
            {t("git.loadingFiles")}
          </p>
        ) : !files || files.length === 0 ? (
          <p className="text-xs text-muted-foreground">{t("git.noFiles")}</p>
        ) : (
          <ul className="space-y-0.5 font-mono text-xs">
            {files.map((f) => (
              <li key={f.path} className="flex gap-2">
                <span
                  className={cn(
                    "w-3 shrink-0 font-semibold",
                    f.status === "A" && "text-emerald-600 dark:text-emerald-400",
                    f.status === "D" && "text-rose-600 dark:text-rose-400",
                    f.status === "M" && "text-amber-600 dark:text-amber-400"
                  )}
                >
                  {f.status}
                </span>
                <span className="truncate">
                  {f.oldPath ? `${f.oldPath} → ${f.path}` : f.path}
                </span>
              </li>
            ))}
          </ul>
        )}
      </div>
    </div>
  );
}

function ContextMenu({
  x,
  y,
  children,
}: {
  x: number;
  y: number;
  children: React.ReactNode;
}) {
  return (
    <div
      className="fixed z-50 min-w-56 rounded-md border bg-popover p-1 shadow-md"
      style={{ left: x, top: y }}
      onClick={(e) => e.stopPropagation()}
    >
      {children}
    </div>
  );
}

function MenuItem({
  icon: Icon,
  label,
  onClick,
  disabled,
}: {
  icon: typeof GitBranch;
  label: string;
  onClick: () => void;
  disabled?: boolean;
}) {
  return (
    <button
      type="button"
      disabled={disabled}
      onClick={onClick}
      className={cn(
        "flex w-full items-center gap-2 rounded px-2 py-1.5 text-left text-sm",
        disabled
          ? "cursor-not-allowed opacity-40"
          : "hover:bg-accent hover:text-accent-foreground"
      )}
    >
      <Icon className="size-4 shrink-0" />
      <span className="truncate">{label}</span>
    </button>
  );
}

// The branch menu is deliberately asymmetric. A ref offers to move a Feature's
// branch onto it, never the reverse: merging a Feature into its Base Branch
// locally would leave the Main Clone diverged from origin and break the
// --ff-only pull it is kept current with (docs/adr/0001). Creating and deleting
// branches stays with the Feature flows, which keep the worktree and metadata in
// step.
function RefMenu({
  x,
  y,
  ref_,
  worktrees,
  worktreeByBranch,
  t,
  onRebase,
  onMerge,
  onPush,
  onOpenFeature,
  onCheckoutMainClone,
}: {
  x: number;
  y: number;
  ref_: GraphRef;
  worktrees: BranchWorktree[];
  worktreeByBranch: Map<string, BranchWorktree>;
  t: (key: string, params?: Record<string, string | number>) => string;
  onRebase: (branch: string) => void;
  onMerge: (branch: string) => void;
  onPush: (branch: string, force: boolean) => void;
  onOpenFeature: (feature: string) => void;
  onCheckoutMainClone: () => void;
}) {
  const own = worktreeByBranch.get(ref_.name);
  if (own) {
    const blocked = !!own.integration?.kind;
    return (
      <ContextMenu x={x} y={y}>
        <MenuItem
          icon={GitMerge}
          label={t("git.rebaseOntoBase", { base: own.baseBranch })}
          disabled={blocked}
          onClick={() => onRebase(ref_.name)}
        />
        <MenuItem
          icon={GitMerge}
          label={t("git.mergeBaseInto", { base: own.baseBranch })}
          disabled={blocked}
          onClick={() => onMerge(ref_.name)}
        />
        <div className="my-1 h-px bg-border" />
        <MenuItem
          icon={Upload}
          label={t("git.push")}
          disabled={blocked}
          onClick={() => onPush(ref_.name, false)}
        />
        <MenuItem
          icon={Upload}
          label={t("git.forcePush")}
          disabled={blocked}
          onClick={() => onPush(ref_.name, true)}
        />
        <div className="my-1 h-px bg-border" />
        <MenuItem
          icon={FolderOpen}
          label={t("git.openFeature", { feature: own.feature })}
          onClick={() => onOpenFeature(own.feature)}
        />
      </ContextMenu>
    );
  }

  // A ref with no Repo Worktree here — a Base Branch, another team's branch, or a
  // tag. It can be a target to move a Feature onto, and the Main Clone can be
  // switched to it (a pointer move that writes no history).
  const targets = worktrees.filter((wt) => !wt.integration?.kind);
  return (
    <ContextMenu x={x} y={y}>
      {targets.length === 0 ? (
        <p className="px-2 py-1.5 text-xs text-muted-foreground">
          {t("git.noWorktreesToMove")}
        </p>
      ) : (
        targets.map((wt) => (
          <MenuItem
            key={`rebase-${wt.branch}`}
            icon={GitMerge}
            label={t("git.rebaseBranchOntoHere", { branch: wt.branch })}
            onClick={() => onRebase(wt.branch)}
          />
        ))
      )}
      {targets.length > 0 && <div className="my-1 h-px bg-border" />}
      {targets.map((wt) => (
        <MenuItem
          key={`merge-${wt.branch}`}
          icon={GitMerge}
          label={t("git.mergeHereIntoBranch", { branch: wt.branch })}
          onClick={() => onMerge(wt.branch)}
        />
      ))}
      {ref_.kind !== "tag" && (
        <>
          <div className="my-1 h-px bg-border" />
          <MenuItem
            icon={GitBranch}
            label={t("git.checkoutMainClone", { name: ref_.name })}
            onClick={onCheckoutMainClone}
          />
        </>
      )}
    </ContextMenu>
  );
}

function IntegrationDialog({
  pending,
  repo,
  busy,
  onToggleAllRepos,
  onCancel,
  onConfirm,
  onOpenFeature,
}: {
  pending: PendingIntegration;
  repo: string;
  busy: boolean;
  onToggleAllRepos: (allRepos: boolean) => void;
  onCancel: () => void;
  onConfirm: () => void;
  onOpenFeature: (feature: string) => void;
}) {
  const { t } = useI18n();
  const repos = pending.readiness?.repositories ?? [];
  const thisRepo = repos.find((r) => r.repo === repo);
  const others = repos.filter((r) => r.repo !== repo);
  const loading = pending.readiness === null;
  // The graph shows one repository, so that is what changes by default. Other
  // repositories of the same Feature come along only if asked for.
  const blocked = pending.allRepos
    ? repos.length > 0 && repos.every((r) => r.blocked)
    : !!thisRepo?.blocked;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4">
      <div className="w-full max-w-lg rounded-lg border bg-background p-4 shadow-lg">
        <h2 className="text-base font-semibold">
          {t(
            pending.kind === "rebase"
              ? "git.confirmRebaseTitle"
              : "git.confirmMergeTitle",
            { branch: pending.branch, upstream: pending.upstream }
          )}
        </h2>
        <p className="mt-1 text-xs text-muted-foreground">
          {t("git.confirmFeature", { feature: pending.feature })}
        </p>

        {loading ? (
          <p className="mt-4 flex items-center gap-2 text-sm text-muted-foreground">
            <Loader2 className="size-4 animate-spin" />
            {t("git.checkingReadiness")}
          </p>
        ) : (
          <div className="mt-4 space-y-2">
            {thisRepo && <ReadinessRow row={thisRepo} onOpenFeature={onOpenFeature} feature={pending.feature} primary />}
            {others.length > 0 && (
              <label className="mt-2 flex items-start gap-2 text-sm">
                <input
                  type="checkbox"
                  className="mt-1"
                  checked={pending.allRepos}
                  onChange={(e) => onToggleAllRepos(e.target.checked)}
                />
                <span>
                  {t("git.alsoOtherRepos", { count: others.length })}
                  <span className="mt-1 block space-y-1">
                    {others.map((r) => (
                      <ReadinessRow
                        key={r.repo}
                        row={r}
                        feature={pending.feature}
                        onOpenFeature={onOpenFeature}
                      />
                    ))}
                  </span>
                </span>
              </label>
            )}
          </div>
        )}

        <p className="mt-4 rounded-md bg-muted/50 p-2 text-xs text-muted-foreground">
          {t("git.conflictPolicyNote")}
        </p>

        <div className="mt-4 flex justify-end gap-2">
          <Button variant="outline" onClick={onCancel} disabled={busy}>
            {t("common.cancel")}
          </Button>
          <Button onClick={onConfirm} disabled={busy || loading || blocked}>
            {busy && <Loader2 className="size-4 animate-spin" />}
            {t(pending.kind === "rebase" ? "git.rebase" : "git.merge")}
          </Button>
        </div>
      </div>
    </div>
  );
}

function ReadinessRow({
  row,
  feature,
  primary,
  onOpenFeature,
}: {
  row: { repo: string; blocked: boolean; reason?: string; agentChanges: number; staleAfter: boolean };
  feature: string;
  primary?: boolean;
  onOpenFeature: (feature: string) => void;
}) {
  const { t } = useI18n();
  return (
    <span className="block text-xs">
      <span className={cn("font-medium", primary && "text-sm")}>
        {row.blocked ? "⊘" : "✓"} {row.repo}
      </span>{" "}
      {row.blocked ? (
        <>
          <span className="text-destructive">{row.reason}</span>{" "}
          {row.agentChanges > 0 && (
            <button
              type="button"
              className="underline"
              onClick={() => onOpenFeature(feature)}
            >
              {t("git.reviewChanges")}
            </button>
          )}
        </>
      ) : (
        <span className="text-muted-foreground">
          {row.staleAfter ? t("git.willNeedPrepare") : t("git.ready")}
        </span>
      )}
    </span>
  );
}

// ---------------------------------------------------------------------------

function joinPath(root: string, rel: string): string {
  if (!root) return rel;
  return `${root.replace(/[\\/]+$/, "")}/${rel}`.replace(/\//g, "\\");
}

function allBranchesKey(root: string, repo: string): string {
  return `agentsafe.graphAllBranches::${root}::${repo}`;
}

function loadAllBranches(root: string, repo: string): boolean {
  try {
    return localStorage.getItem(allBranchesKey(root, repo)) === "true";
  } catch {
    return false;
  }
}

function saveAllBranches(root: string, repo: string, value: boolean) {
  if (!root || !repo) return;
  try {
    localStorage.setItem(allBranchesKey(root, repo), String(value));
  } catch {
    /* localStorage unavailable */
  }
}

function formatTime(ms: number): string {
  return new Date(ms).toLocaleTimeString();
}

function formatDate(iso: string): string {
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? iso : d.toLocaleString();
}

function relativeTime(
  iso: string,
  t: (key: string, params?: Record<string, string | number>) => string
): string {
  const then = new Date(iso).getTime();
  if (Number.isNaN(then)) return iso;
  const minutes = Math.max(0, Math.round((Date.now() - then) / 60000));
  if (minutes < 60) return t("time.minutesAgo", { count: minutes });
  const hours = Math.round(minutes / 60);
  if (hours < 24) return t("time.hoursAgo", { count: hours });
  return t("time.daysAgo", { count: Math.round(hours / 24) });
}
