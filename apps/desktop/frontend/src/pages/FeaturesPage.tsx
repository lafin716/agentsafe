import { useCallback, useEffect, useState } from "react";
import { ChevronRight, Code2, FolderOpen, Plus, RefreshCw, Terminal, Trash2 } from "lucide-react";
import { api, errMessage } from "@/lib/api";
import { useConfirm } from "@/components/ui/confirm";
import type { FeatureCreateCheck, FeatureDeleteResult, FeatureEntry } from "@/lib/types";
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
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { useToast } from "@/components/ui/toast";
import { useI18n } from "@/i18n/I18nProvider";

interface Props {
  onOpen: (name: string) => void;
}

export function FeaturesPage({ onOpen }: Props) {
  const { notify } = useToast();
  const { t } = useI18n();
  const confirm = useConfirm();
  const [features, setFeatures] = useState<FeatureEntry[]>([]);
  const [busy, setBusy] = useState(false);
  const [name, setName] = useState("");
  const [createCheck, setCreateCheck] = useState<FeatureCreateCheck | null>(null);
  const [existingBranch, setExistingBranch] = useState<"reuse" | "recreate">("reuse");

  const load = useCallback(async () => {
    try {
      const res = await api.ListFeatures();
      setFeatures(res.features ?? []);
    } catch (e) {
      notify(errMessage(e), "error");
    }
  }, [notify]);

  useEffect(() => {
    load();
  }, [load]);

  async function openTerminal(name: string) {
    try {
      await api.OpenInTerminal(name);
      notify(t("toast.openedTerminal", { name }), "success");
    } catch (e) {
      notify(errMessage(e), "error");
    }
  }

  async function openVSCode(name: string) {
    try {
      await api.OpenInEditor(name, "code");
      notify(t("toast.openedVSCode", { name }), "success");
    } catch (e) {
      notify(errMessage(e), "error");
    }
  }

  async function openFolder(name: string) {
    try {
      const p = await api.OpenFeatureFolder(name);
      notify(t("toast.openedPath", { path: p }), "success");
    } catch (e) {
      notify(errMessage(e), "error");
    }
  }

  async function deleteFeature(name: string) {
    if (!(await confirm({ message: t("feature.deleteConfirm", { name }), danger: true })))
      return;
    try {
      setBusy(true);
      let result: FeatureDeleteResult | undefined;
      try {
        result = await api.FeatureDelete(name, false, false);
      } catch (e) {
        const msg = errMessage(e);
        // Offer a force delete when the worktree has uncommitted changes.
        if (/uncommitted|changes/i.test(msg)) {
          if (
            !(await confirm({
              message: t("feature.deleteForceConfirm"),
              danger: true,
            }))
          )
            return;
          result = await api.FeatureDelete(name, false, true);
        } else {
          throw e;
        }
      }
      for (const warning of result?.warnings ?? []) {
        notify(warning, "error");
      }
      notify(t("toast.featureDeleted"), "success");
      await load();
    } catch (e) {
      notify(errMessage(e), "error");
    } finally {
      setBusy(false);
    }
  }

  async function create(e: React.FormEvent) {
    e.preventDefault();
    const featureName = name.trim();
    try {
      setBusy(true);
      const check = await api.CheckFeatureCreation(featureName, "");
      if (check.hasConflicts || check.blocked) {
        setExistingBranch("reuse");
        setCreateCheck(check);
      } else {
        await completeCreate(featureName, "error");
      }
    } catch (err) {
      notify(errMessage(err), "error");
    } finally {
      setBusy(false);
    }
  }

  async function completeCreate(
    featureName: string,
    policy: "error" | "reuse" | "recreate"
  ) {
    await api.CreateFeature(featureName, "", policy);
    notify(t("toast.featureCreated", { name: featureName }), "success");
    setName("");
    setCreateCheck(null);
    setExistingBranch("reuse");
    await load();
    if (
      await confirm({
        title: t("features.prepareAgentTitle"),
        message: t("features.prepareAgentConfirm", { name: featureName }),
        confirmLabel: t("features.prepareAgentYes"),
      })
    ) {
      const meta = await api.AgentPrepare(featureName, true);
      const copied = (meta.repositories ?? []).reduce((n, r) => n + r.copiedFiles, 0);
      notify(t("toast.agentPrepared", { count: copied }), "success");
    }
    onOpen(featureName);
  }

  async function continueCreate() {
    if (!createCheck || createCheck.blocked) return;
    if (
      existingBranch === "recreate" &&
      !(await confirm({
        title: t("features.preflightRecreateTitle"),
        message: t("features.preflightRecreateConfirm"),
        confirmLabel: t("features.preflightContinue"),
        danger: true,
      }))
    )
      return;
    try {
      setBusy(true);
      await completeCreate(createCheck.name, existingBranch);
    } catch (err) {
      // State can change after the initial check. Refresh the conflict view
      // instead of leaving the user with a generic retry error.
      try {
        const refreshed = await api.CheckFeatureCreation(createCheck.name, "");
        if (refreshed.hasConflicts || refreshed.blocked) {
          setCreateCheck(refreshed);
        } else {
          notify(errMessage(err), "error");
        }
      } catch {
        notify(errMessage(err), "error");
      }
    } finally {
      setBusy(false);
    }
  }

  const conflictRepos = createCheck?.repositories?.filter((repo) => repo.conflict || repo.blockedReason) ?? [];
  const canReuse = conflictRepos.every((repo) => !repo.conflict || repo.canReuse);
  const canRecreate = conflictRepos.every((repo) => !repo.conflict || repo.canRecreate);

  return (
    <div className="space-y-6">
      <Card>
        <CardHeader className="flex-row items-center justify-between space-y-0">
          <div>
            <CardTitle>{t("features.title")}</CardTitle>
            <CardDescription>
              {t("features.count", { count: features.length })}
            </CardDescription>
          </div>
          <Button variant="outline" size="sm" onClick={load}>
            <RefreshCw className="size-4" /> {t("common.refresh")}
          </Button>
        </CardHeader>
        <CardContent>
          {features.length === 0 ? (
            <p className="text-sm text-muted-foreground">{t("features.empty")}</p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("features.colFeature")}</TableHead>
                  <TableHead>{t("features.colBranch")}</TableHead>
                  <TableHead>{t("features.colRepos")}</TableHead>
                  <TableHead>{t("features.colAgent")}</TableHead>
                  <TableHead></TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {features.map((f) => (
                  <TableRow
                    key={f.name}
                    className="cursor-pointer"
                    onClick={() => onOpen(f.name)}
                  >
                    <TableCell className="font-medium">{f.name}</TableCell>
                    <TableCell>{f.branch}</TableCell>
                    <TableCell>{f.repoCount}</TableCell>
                    <TableCell>
                      {f.agentReady ? (
                        <Badge variant="success">{t("features.agentReady")}</Badge>
                      ) : (
                        <Badge variant="outline">{t("features.agentNone")}</Badge>
                      )}
                    </TableCell>
                    <TableCell>
                      <div className="flex items-center justify-end gap-1">
                        <Button
                          variant="ghost"
                          size="icon"
                          title={t("features.openFolder")}
                          onClick={(e) => {
                            e.stopPropagation();
                            openFolder(f.name);
                          }}
                        >
                          <FolderOpen className="size-4" />
                        </Button>
                        <Button
                          variant="ghost"
                          size="icon"
                          title={t("features.openTerminal")}
                          onClick={(e) => {
                            e.stopPropagation();
                            openTerminal(f.name);
                          }}
                        >
                          <Terminal className="size-4" />
                        </Button>
                        <Button
                          variant="ghost"
                          size="icon"
                          title={t("features.openVSCode")}
                          onClick={(e) => {
                            e.stopPropagation();
                            openVSCode(f.name);
                          }}
                        >
                          <Code2 className="size-4" />
                        </Button>
                        <Button
                          variant="ghost"
                          size="icon"
                          title={t("feature.delete")}
                          disabled={busy}
                          onClick={(e) => {
                            e.stopPropagation();
                            deleteFeature(f.name);
                          }}
                        >
                          <Trash2 className="size-4 text-destructive" />
                        </Button>
                        <ChevronRight className="size-4 text-muted-foreground" />
                      </div>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>{t("features.createTitle")}</CardTitle>
          <CardDescription>{t("features.createDesc")}</CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={create} className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <div className="space-y-1.5">
              <Label htmlFor="fn">{t("features.nameLabel")}</Label>
              <Input
                id="fn"
                required
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="coupon-v2"
              />
            </div>
            <div className="sm:col-span-2">
              <Button type="submit" disabled={busy}>
                <Plus className="size-4" /> {t("features.createButton")}
              </Button>
            </div>
          </form>
        </CardContent>
      </Card>

      {createCheck && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4"
          onClick={() => !busy && setCreateCheck(null)}
        >
          <div
            className="w-full max-w-2xl rounded-lg border bg-card p-5 shadow-xl"
            onClick={(e) => e.stopPropagation()}
          >
            <h2 className="text-lg font-semibold">{t("features.preflightTitle")}</h2>
            <p className="mt-1 text-sm text-muted-foreground">
              {t("features.preflightDesc", { branch: createCheck.branch })}
            </p>

            <div className="mt-4 max-h-64 divide-y overflow-auto rounded-md border">
              {conflictRepos.map((repo) => (
                <div key={repo.name} className="space-y-1 p-3 text-sm">
                  <div className="flex items-center justify-between gap-3">
                    <span className="font-medium">{repo.name}</span>
                    {repo.blockedReason ? (
                      <Badge variant="destructive">{t("features.preflightBlocked")}</Badge>
                    ) : (
                      <Badge variant="outline">{t("features.preflightExisting")}</Badge>
                    )}
                  </div>
                  <p className="text-xs text-muted-foreground">
                    {[
                      repo.localBranch && t("features.preflightLocal"),
                      repo.remoteBranch && t("features.preflightRemote"),
                      repo.checkedOutAt &&
                        t("features.preflightCheckedOut", { path: repo.checkedOutAt }),
                    ]
                      .filter(Boolean)
                      .join(" · ")}
                  </p>
                  {repo.blockedReason && (
                    <p className="text-xs text-destructive">{repo.blockedReason}</p>
                  )}
                </div>
              ))}
            </div>

            {!createCheck.blocked && (
              <div className="mt-4 space-y-2">
                <label className="flex items-start gap-3 rounded-md border p-3 text-sm">
                  <input
                    type="radio"
                    name="branchPolicy"
                    value="reuse"
                    checked={existingBranch === "reuse"}
                    disabled={!canReuse}
                    onChange={() => setExistingBranch("reuse")}
                  />
                  <span>
                    <strong>{t("features.existingBranchReuse")}</strong>
                    <span className="mt-0.5 block text-xs text-muted-foreground">
                      {t("features.existingBranchHint.reuse")}
                    </span>
                  </span>
                </label>
                <label className="flex items-start gap-3 rounded-md border p-3 text-sm">
                  <input
                    type="radio"
                    name="branchPolicy"
                    value="recreate"
                    checked={existingBranch === "recreate"}
                    disabled={!canRecreate}
                    onChange={() => setExistingBranch("recreate")}
                  />
                  <span>
                    <strong>{t("features.existingBranchRecreate")}</strong>
                    <span className="mt-0.5 block text-xs text-muted-foreground">
                      {t("features.existingBranchHint.recreate")}
                    </span>
                  </span>
                </label>
              </div>
            )}

            <div className="mt-5 flex justify-end gap-2">
              <Button variant="outline" disabled={busy} onClick={() => setCreateCheck(null)}>
                {createCheck.blocked ? t("common.close") : t("common.cancel")}
              </Button>
              {!createCheck.blocked && (
                <Button
                  variant={existingBranch === "recreate" ? "destructive" : "default"}
                  disabled={busy || (existingBranch === "reuse" ? !canReuse : !canRecreate)}
                  onClick={continueCreate}
                >
                  {t("features.preflightContinue")}
                </Button>
              )}
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
