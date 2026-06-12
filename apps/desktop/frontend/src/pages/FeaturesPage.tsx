import { useCallback, useEffect, useState } from "react";
import { ChevronRight, Code2, FolderOpen, Plus, RefreshCw, Terminal, Trash2 } from "lucide-react";
import { api, errMessage } from "@/lib/api";
import { useConfirm } from "@/components/ui/confirm";
import type { FeatureDeleteResult, FeatureEntry } from "@/lib/types";
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
  const [base, setBase] = useState("");
  const [existingBranch, setExistingBranch] = useState("error");

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
    try {
      setBusy(true);
      await api.CreateFeature(name.trim(), base.trim(), existingBranch);
      notify(t("toast.featureCreated", { name: name.trim() }), "success");
      setName("");
      setBase("");
      setExistingBranch("error");
      await load();
    } catch (err) {
      notify(errMessage(err), "error");
    } finally {
      setBusy(false);
    }
  }

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
            <div className="space-y-1.5">
              <Label htmlFor="fb">{t("features.baseLabel")}</Label>
              <Input
                id="fb"
                value={base}
                onChange={(e) => setBase(e.target.value)}
                placeholder="develop"
              />
            </div>
            <div className="space-y-1.5 sm:col-span-2">
              <Label htmlFor="existingBranch">{t("features.existingBranchLabel")}</Label>
              <select
                id="existingBranch"
                value={existingBranch}
                onChange={(e) => setExistingBranch(e.target.value)}
                className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
              >
                <option value="error">{t("features.existingBranchError")}</option>
                <option value="reuse">{t("features.existingBranchReuse")}</option>
                <option value="recreate">{t("features.existingBranchRecreate")}</option>
              </select>
              <p className="text-xs text-muted-foreground">
                {t(`features.existingBranchHint.${existingBranch}`)}
              </p>
            </div>
            <div className="sm:col-span-2">
              <Button type="submit" disabled={busy}>
                <Plus className="size-4" /> {t("features.createButton")}
              </Button>
            </div>
          </form>
        </CardContent>
      </Card>
    </div>
  );
}
