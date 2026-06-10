import { useCallback, useEffect, useState } from "react";
import {
  AppWindow,
  ArrowLeft,
  Boxes,
  ExternalLink,
  GitCommit,
  GitMerge,
  History,
  RefreshCw,
  Terminal,
  Trash2,
  Upload,
  Wand2,
} from "lucide-react";
import { api, errMessage } from "@/lib/api";
import type {
  Change,
  DiffResult,
  FeatureStatusResult,
  RequestResult,
  RequestResults,
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
  const [tab, setTab] = useState<Tab>("status");
  const [busy, setBusy] = useState(false);

  const [status, setStatus] = useState<FeatureStatusResult | null>(null);
  const [diff, setDiff] = useState<DiffResult | null>(null);
  // Sync-history stack depth per repository, for the count badges.
  const [histCounts, setHistCounts] = useState<Record<string, number>>({});
  // Local override of prepared state. null = no user action yet (fall back to
  // backend status); true/false once the user prepares or deletes. Keeping it
  // separate from status avoids a status reload clobbering the value right
  // after a successful prepare.
  const [prepared, setPrepared] = useState<boolean | null>(null);

  // sync options
  const [includeRisky, setIncludeRisky] = useState(false);
  const [allowMasked, setAllowMasked] = useState(false);
  const [dryRun, setDryRun] = useState(false);

  // deliver
  const [commitMsg, setCommitMsg] = useState("");
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
    try {
      setStatus(await api.FeatureStatus(name));
    } catch (e) {
      notify(errMessage(e), "error");
    }
  }, [name, notify]);

  const loadDiff = useCallback(async () => {
    try {
      setDiff(await api.AgentDiff(name, ""));
    } catch (e) {
      notify(errMessage(e), "error");
    }
  }, [name, notify]);

  const loadCounts = useCallback(async () => {
    try {
      setHistCounts(await api.SyncHistoryCounts(name));
    } catch {
      /* badges are best-effort */
    }
  }, [name]);

  useEffect(() => {
    loadStatus();
    loadCounts();
  }, [loadStatus, loadCounts]);

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
      await Promise.all([loadDiff(), loadStatus()]);
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
      setPrepared(false);
      notify(t("toast.agentDeleted"), "success");
      await loadStatus();
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

  const commit = () =>
    run(async () => {
      await api.Commit(name, commitMsg.trim());
      notify(t("toast.committed"), "success");
      setCommitMsg("");
      await loadStatus();
    });

  const push = () =>
    run(async () => {
      await api.Push(name);
      notify(t("toast.pushed"), "success");
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

  const agentReady = prepared ?? (status?.agentReady ?? false);

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
            <div>
              <CardTitle>{status?.feature ?? name}</CardTitle>
              <CardDescription>
                {t("feature.branchLabel", { branch: status?.branch ?? "—" })}
              </CardDescription>
            </div>
            <div className="flex items-center gap-2">
              <Button
                variant="outline"
                size="sm"
                onClick={rebase}
                disabled={busy}
              >
                <GitMerge className="size-4" /> {t("feature.rebase")}
              </Button>
              <Button variant="outline" size="sm" onClick={loadStatus}>
                <RefreshCw className="size-4" /> {t("common.refresh")}
              </Button>
            </div>
          </CardHeader>
          <CardContent className="space-y-4">
            {(status?.repositories ?? []).map((r) => (
              <div key={r.name}>
                <div className="mb-1 flex items-center gap-2">
                  <Boxes className="size-4 text-muted-foreground" />
                  <span className="font-medium">{r.name}</span>
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
                {r.status.trim() !== "" && (
                  <pre className="overflow-auto rounded-md border bg-muted/40 p-3 text-xs">
                    {r.status}
                  </pre>
                )}
              </div>
            ))}
            {(status?.repositories ?? []).length === 0 && (
              <p className="text-sm text-muted-foreground">{t("feature.noRepos")}</p>
            )}
          </CardContent>
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
              <div className="flex flex-wrap items-center gap-2">
              <Button onClick={prepare} disabled={busy}>
                <Wand2 className="size-4" />{" "}
                {agentReady ? t("feature.regenerate") : t("feature.prepare")}
              </Button>
              {agentReady && (
                <>
                  <Button variant="outline" onClick={loadDiff} disabled={busy}>
                    <RefreshCw className="size-4" /> {t("feature.refreshDiff")}
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
            </CardContent>
          </Card>

          {agentReady && (
          <>
          <Card>
            <CardHeader>
              <CardTitle>{t("feature.diffTitle")}</CardTitle>
              <CardDescription>
                {diff
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
              {(diff?.repositories ?? []).map((r) => (
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
          <Card>
            <CardHeader>
              <CardTitle>{t("feature.commitTitle")}</CardTitle>
            </CardHeader>
            <CardContent className="space-y-3">
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
                <Button onClick={commit} disabled={busy || !commitMsg.trim()}>
                  <GitCommit className="size-4" /> {t("feature.commitAll")}
                </Button>
                <Button variant="secondary" onClick={push} disabled={busy}>
                  <Upload className="size-4" /> {t("feature.pushAll")}
                </Button>
              </div>
            </CardContent>
          </Card>

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
