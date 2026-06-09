import { useCallback, useEffect, useState } from "react";
import {
  Archive,
  History,
  RefreshCw,
  RotateCcw,
  Trash,
  Trash2,
} from "lucide-react";
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
import type { BackupEntry, Config } from "@/lib/types";

interface Props {
  config: Config | null;
}

function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  const units = ["KB", "MB", "GB"];
  let n = bytes / 1024;
  let i = 0;
  while (n >= 1024 && i < units.length - 1) {
    n /= 1024;
    i++;
  }
  return `${n.toFixed(1)} ${units[i]}`;
}

function formatDate(s: string): string {
  const d = new Date(s);
  return isNaN(d.getTime()) ? s : d.toLocaleString();
}

export function BackupsPage({ config }: Props) {
  const { t } = useI18n();
  const { notify } = useToast();
  const confirm = useConfirm();
  const [backups, setBackups] = useState<BackupEntry[]>([]);
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    try {
      setBackups(await api.ListBackups());
    } catch (e) {
      notify(errMessage(e), "error");
    }
  }, [notify]);

  useEffect(() => {
    if (config) load();
  }, [config, load]);

  if (!config) {
    return (
      <div className="max-w-3xl space-y-6">
        <Card>
          <CardHeader>
            <CardTitle>{t("nav.backups")}</CardTitle>
            <CardDescription>{t("backups.noWorkspace")}</CardDescription>
          </CardHeader>
        </Card>
      </div>
    );
  }

  async function restore(b: BackupEntry) {
    if (!(await confirm({ message: t("backups.restoreConfirm", { repo: b.repo }) })))
      return;
    try {
      setBusy(true);
      await api.RestoreBackup(b.path);
      notify(t("toast.backupRestored"), "success");
      await load();
    } catch (e) {
      notify(errMessage(e), "error");
    } finally {
      setBusy(false);
    }
  }

  async function remove(b: BackupEntry) {
    if (
      !(await confirm({
        message: t("backups.deleteConfirm", { repo: b.repo }),
        danger: true,
      }))
    )
      return;
    try {
      setBusy(true);
      await api.DeleteBackup(b.path);
      notify(t("toast.backupDeleted"), "success");
      await load();
    } catch (e) {
      notify(errMessage(e), "error");
    } finally {
      setBusy(false);
    }
  }

  async function removeAll() {
    if (!(await confirm({ message: t("backups.deleteAllConfirm"), danger: true })))
      return;
    try {
      setBusy(true);
      const n = await api.DeleteAllBackups();
      notify(t("toast.backupsAllDeleted", { count: n }), "success");
      await load();
    } catch (e) {
      notify(errMessage(e), "error");
    } finally {
      setBusy(false);
    }
  }

  // Group backups by feature for display.
  const byFeature = new Map<string, BackupEntry[]>();
  for (const b of backups) {
    const arr = byFeature.get(b.feature) ?? [];
    arr.push(b);
    byFeature.set(b.feature, arr);
  }

  return (
    <div className="max-w-3xl space-y-6">
      <Card>
        <CardHeader className="flex-row items-center justify-between space-y-0">
          <div>
            <CardTitle className="flex items-center gap-2">
              <Archive className="size-5" /> {t("backups.title")}
            </CardTitle>
            <CardDescription>{t("backups.desc")}</CardDescription>
          </div>
          <div className="flex gap-2">
            <Button variant="outline" size="sm" onClick={load} disabled={busy}>
              <RefreshCw className="size-4" /> {t("common.refresh")}
            </Button>
            <Button
              variant="destructive"
              size="sm"
              onClick={removeAll}
              disabled={busy || backups.length === 0}
            >
              <Trash className="size-4" /> {t("backups.deleteAll")}
            </Button>
          </div>
        </CardHeader>
        <CardContent className="space-y-5">
          {backups.length === 0 && (
            <p className="text-sm text-muted-foreground">{t("backups.empty")}</p>
          )}
          {[...byFeature.entries()].map(([feature, items]) => (
            <div key={feature} className="space-y-2">
              <div className="text-sm font-semibold text-muted-foreground">
                {feature}
              </div>
              <ul className="divide-y rounded-md border">
                {items.map((b) => (
                  <li
                    key={b.path}
                    className="flex items-center justify-between gap-3 px-3 py-2 text-sm"
                  >
                    <div className="min-w-0">
                      <div className="flex items-center gap-2">
                        <span className="font-medium">{b.repo}</span>
                        <Badge variant="outline">
                          <History className="mr-1 size-3" />
                          {formatDate(b.createdAt)}
                        </Badge>
                      </div>
                      <div className="mt-0.5 text-xs text-muted-foreground">
                        {t("backups.colFiles", { count: b.files })} ·{" "}
                        {formatSize(b.size)}
                      </div>
                    </div>
                    <div className="flex shrink-0 gap-1">
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() => restore(b)}
                        disabled={busy}
                      >
                        <RotateCcw className="size-4" /> {t("backups.restore")}
                      </Button>
                      <Button
                        variant="destructive"
                        size="sm"
                        onClick={() => remove(b)}
                        disabled={busy}
                      >
                        <Trash2 className="size-4" /> {t("backups.delete")}
                      </Button>
                    </div>
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
