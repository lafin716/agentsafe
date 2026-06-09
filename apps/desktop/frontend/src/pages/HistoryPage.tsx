import { useCallback, useEffect, useState } from "react";
import { History, Lock, RefreshCw, RotateCcw } from "lucide-react";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { useToast } from "@/components/ui/toast";
import { useConfirm } from "@/components/ui/confirm";
import { useI18n } from "@/i18n/I18nProvider";
import { api, errMessage } from "@/lib/api";
import type { Config, SyncHistoryEntry } from "@/lib/types";
import { cn } from "@/lib/utils";

interface Props {
  config: Config | null;
  feature?: string;
}

function formatDate(s: string): string {
  const d = new Date(s);
  return isNaN(d.getTime()) ? s : d.toLocaleString();
}

export function HistoryPage({ config, feature }: Props) {
  const { t } = useI18n();
  const { notify } = useToast();
  const confirm = useConfirm();
  const [entries, setEntries] = useState<SyncHistoryEntry[]>([]);
  const [busy, setBusy] = useState(false);
  const [expanded, setExpanded] = useState<Record<string, boolean>>({});

  const load = useCallback(async () => {
    try {
      const all = await api.AllSyncHistory();
      setEntries(feature ? all.filter((e) => e.feature === feature) : all);
    } catch (e) {
      notify(errMessage(e), "error");
    }
  }, [feature, notify]);

  useEffect(() => {
    if (config) load();
  }, [config, load]);

  if (!config) {
    return (
      <div className="max-w-3xl space-y-6">
        <Card>
          <CardHeader>
            <CardTitle>{t("nav.history")}</CardTitle>
            <CardDescription>{t("history.noWorkspace")}</CardDescription>
          </CardHeader>
        </Card>
      </div>
    );
  }

  async function rollback(e: SyncHistoryEntry) {
    if (
      !(await confirm({
        message: t("history.rollbackConfirm", { repo: e.repo }),
        danger: true,
      }))
    )
      return;
    try {
      setBusy(true);
      await api.RollbackSync(e.feature, e.repo, e.id);
      notify(t("toast.rolledBack"), "success");
      await load();
    } catch (err) {
      notify(errMessage(err), "error");
    } finally {
      setBusy(false);
    }
  }

  // Group entries by feature + repo (newest-first order preserved per group).
  const groups: { feature: string; repo: string; items: SyncHistoryEntry[] }[] =
    [];
  const index = new Map<string, number>();
  for (const e of entries) {
    const key = `${e.feature}/${e.repo}`;
    let i = index.get(key);
    if (i === undefined) {
      i = groups.length;
      index.set(key, i);
      groups.push({ feature: e.feature, repo: e.repo, items: [] });
    }
    groups[i].items.push(e);
  }

  return (
    <div className="max-w-3xl space-y-6">
      <Card>
        <CardHeader className="flex-row items-center justify-between space-y-0">
          <div>
            <CardTitle className="flex items-center gap-2">
              <History className="size-5" /> {t("history.title")}
            </CardTitle>
            <CardDescription>{t("history.desc")}</CardDescription>
          </div>
          <Button variant="outline" size="sm" onClick={load} disabled={busy}>
            <RefreshCw className="size-4" /> {t("common.refresh")}
          </Button>
        </CardHeader>
        <CardContent className="space-y-5">
          {entries.length === 0 && (
            <p className="text-sm text-muted-foreground">{t("history.empty")}</p>
          )}
          {groups.map((g) => (
            <div key={`${g.feature}/${g.repo}`} className="space-y-2">
              <div className="text-sm font-semibold text-muted-foreground">
                {g.feature} / {g.repo}
              </div>
              <ul className="divide-y rounded-md border">
                {g.items.map((e) => (
                  <li key={e.id} className="px-3 py-2 text-sm">
                    <div className="flex items-center justify-between gap-3">
                      <button
                        className="min-w-0 text-left"
                        onClick={() =>
                          setExpanded((m) => ({ ...m, [e.id]: !m[e.id] }))
                        }
                      >
                        <div className="flex items-center gap-2">
                          <span className="font-medium">
                            {formatDate(e.syncedAt)}
                          </span>
                          <Badge variant="outline">
                            {t("history.colChanges", { count: e.changeCount })}
                          </Badge>
                        </div>
                      </button>
                      {e.canRollback ? (
                        <Button
                          variant="outline"
                          size="sm"
                          onClick={() => rollback(e)}
                          disabled={busy}
                        >
                          <RotateCcw className="size-4" /> {t("history.rollback")}
                        </Button>
                      ) : (
                        <span
                          className="flex items-center gap-1 text-xs text-muted-foreground"
                          title={t("history.locked")}
                        >
                          <Lock className="size-3" /> {t("history.locked")}
                        </span>
                      )}
                    </div>
                    {expanded[e.id] && (e.changes ?? []).length > 0 && (
                      <ul className="mt-2 divide-y rounded-md border bg-muted/30">
                        {(e.changes ?? []).map((c, i) => (
                          <li
                            key={c.path + i}
                            className="flex items-center gap-2 px-3 py-1.5 text-xs"
                          >
                            <span
                              className={cn(
                                "w-20 shrink-0 font-mono",
                                c.type === "ADDED"
                                  ? "text-emerald-600"
                                  : c.type === "DELETED"
                                    ? "text-destructive"
                                    : "text-amber-600"
                              )}
                            >
                              {c.type}
                            </span>
                            <span className="truncate">{c.path}</span>
                          </li>
                        ))}
                      </ul>
                    )}
                  </li>
                ))}
              </ul>
            </div>
          ))}
        </CardContent>
      </Card>
    </div>
  );
}
