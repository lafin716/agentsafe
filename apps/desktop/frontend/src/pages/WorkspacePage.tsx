import { useState } from "react";
import { DownloadCloud, FolderOpen, Plus, Sparkles, Trash2 } from "lucide-react";
import { api, errMessage } from "@/lib/api";
import type { Config } from "@/lib/types";
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
import { useConfirm } from "@/components/ui/confirm";
import { useI18n } from "@/i18n/I18nProvider";

interface Props {
  config: Config | null;
  root: string;
  onLoaded: (cfg: Config) => void;
  onChanged: () => void | Promise<void>;
}

export function WorkspacePage({ config, root, onLoaded, onChanged }: Props) {
  const { notify } = useToast();
  const confirm = useConfirm();
  const { t } = useI18n();
  const [busy, setBusy] = useState(false);
  const [initName, setInitName] = useState("");

  // Add-repo form
  const [repoName, setRepoName] = useState("");
  const [repoUrl, setRepoUrl] = useState("");
  const [repoType, setRepoType] = useState("");
  const [repoBranch, setRepoBranch] = useState("");

  async function pickAndOpen() {
    try {
      setBusy(true);
      const dir = await api.SelectWorkspaceDir();
      if (!dir) return;
      const cfg = await api.OpenWorkspace(dir);
      onLoaded(cfg);
      notify(t("toast.openedWorkspace", { name: cfg.Workspace.Name }), "success");
    } catch (e) {
      notify(errMessage(e), "error");
    } finally {
      setBusy(false);
    }
  }

  async function pickAndInit() {
    try {
      setBusy(true);
      const dir = await api.SelectWorkspaceDir();
      if (!dir) return;
      const cfg = await api.InitWorkspace(dir, initName.trim());
      onLoaded(cfg);
      notify(
        t("toast.initializedWorkspace", { name: cfg.Workspace.Name }),
        "success"
      );
    } catch (e) {
      notify(errMessage(e), "error");
    } finally {
      setBusy(false);
    }
  }

  async function addRepo(e: React.FormEvent) {
    e.preventDefault();
    try {
      setBusy(true);
      await api.AddRepo(
        repoName.trim(),
        repoUrl.trim(),
        repoType.trim(),
        repoBranch.trim()
      );
      setRepoName("");
      setRepoUrl("");
      setRepoType("");
      setRepoBranch("");
      await onChanged();
      notify(t("toast.repoAdded"), "success");
    } catch (err) {
      notify(errMessage(err), "error");
    } finally {
      setBusy(false);
    }
  }

  async function removeRepo(name: string) {
    if (
      !(await confirm({ message: t("confirm.removeRepo", { name }), danger: true }))
    ) {
      return;
    }
    try {
      setBusy(true);
      await api.RemoveRepo(name);
      await onChanged();
      notify(t("toast.repoRemoved"), "success");
    } catch (err) {
      notify(errMessage(err), "error");
    } finally {
      setBusy(false);
    }
  }

  async function pull() {
    try {
      setBusy(true);
      await api.Pull();
      notify(t("toast.pullCompleted"), "success");
    } catch (e) {
      notify(errMessage(e), "error");
    } finally {
      setBusy(false);
    }
  }

  if (!config) {
    return (
      <div className="mx-auto max-w-2xl space-y-4">
        <Card>
          <CardHeader>
            <CardTitle>{t("workspace.openTitle")}</CardTitle>
            <CardDescription>{t("workspace.openDesc")}</CardDescription>
          </CardHeader>
          <CardContent>
            <Button onClick={pickAndOpen} disabled={busy}>
              <FolderOpen className="size-4" /> {t("workspace.chooseFolder")}
            </Button>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>{t("workspace.initTitle")}</CardTitle>
            <CardDescription>{t("workspace.initDesc")}</CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            <div className="space-y-1.5">
              <Label htmlFor="initName">{t("workspace.nameLabel")}</Label>
              <Input
                id="initName"
                placeholder="my-service"
                value={initName}
                onChange={(e) => setInitName(e.target.value)}
              />
            </div>
            <Button onClick={pickAndInit} disabled={busy} variant="secondary">
              <Sparkles className="size-4" /> {t("workspace.chooseInit")}
            </Button>
          </CardContent>
        </Card>
      </div>
    );
  }

  const repos = config.Repositories ?? [];

  return (
    <div className="space-y-6">
      <Card>
        <CardHeader>
          <CardTitle>{config.Workspace.Name}</CardTitle>
          <CardDescription className="break-all">{root}</CardDescription>
        </CardHeader>
        <CardContent className="grid grid-cols-2 gap-3 text-sm sm:grid-cols-4">
          <Info label={t("workspace.baseBranch")} value={config.Git.DefaultBaseBranch} />
          <Info label={t("workspace.branchPrefix")} value={config.Git.BranchPrefix} />
          <Info label={t("workspace.gitlab")} value={config.GitLab.BaseURL} />
          <Info label={t("workspace.target")} value={config.GitLab.TargetBranch} />
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="flex-row items-center justify-between space-y-0">
          <div>
            <CardTitle>{t("repo.title")}</CardTitle>
            <CardDescription>
              {t("repo.countConfigured", { count: repos.length })}
            </CardDescription>
          </div>
          <Button variant="outline" size="sm" onClick={pull} disabled={busy}>
            <DownloadCloud className="size-4" /> {t("repo.pullAll")}
          </Button>
        </CardHeader>
        <CardContent>
          {repos.length === 0 ? (
            <p className="text-sm text-muted-foreground">{t("repo.empty")}</p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("repo.colName")}</TableHead>
                  <TableHead>{t("repo.colType")}</TableHead>
                  <TableHead>{t("repo.colBranch")}</TableHead>
                  <TableHead>{t("repo.colUrl")}</TableHead>
                  <TableHead className="w-10"></TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {repos.map((r) => (
                  <TableRow key={r.Name}>
                    <TableCell className="font-medium">{r.Name}</TableCell>
                    <TableCell>
                      {r.Type ? (
                        <Badge variant="secondary">{r.Type}</Badge>
                      ) : (
                        <span className="text-muted-foreground">—</span>
                      )}
                    </TableCell>
                    <TableCell>{r.DefaultBranch || "—"}</TableCell>
                    <TableCell className="max-w-xs truncate text-muted-foreground">
                      {r.URL}
                    </TableCell>
                    <TableCell>
                      <Button
                        variant="ghost"
                        size="icon"
                        className="size-8 text-muted-foreground hover:text-destructive"
                        disabled={busy}
                        title={t("repo.removeTitle")}
                        onClick={() => removeRepo(r.Name)}
                      >
                        <Trash2 className="size-4" />
                      </Button>
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
          <CardTitle>{t("repo.addTitle")}</CardTitle>
        </CardHeader>
        <CardContent>
          <form
            onSubmit={addRepo}
            className="grid grid-cols-1 gap-3 sm:grid-cols-2"
          >
            <div className="space-y-1.5">
              <Label htmlFor="rn">{t("repo.nameLabel")}</Label>
              <Input
                id="rn"
                required
                value={repoName}
                onChange={(e) => setRepoName(e.target.value)}
                placeholder="backend"
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="ru">{t("repo.urlLabel")}</Label>
              <Input
                id="ru"
                required
                value={repoUrl}
                onChange={(e) => setRepoUrl(e.target.value)}
                placeholder="https://gitlab.example.com/company/backend.git"
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="rt">{t("repo.typeLabel")}</Label>
              <Input
                id="rt"
                value={repoType}
                onChange={(e) => setRepoType(e.target.value)}
                placeholder="backend / frontend"
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="rb">{t("repo.branchLabel")}</Label>
              <Input
                id="rb"
                value={repoBranch}
                onChange={(e) => setRepoBranch(e.target.value)}
                placeholder="develop"
              />
            </div>
            <div className="sm:col-span-2">
              <Button type="submit" disabled={busy}>
                <Plus className="size-4" /> {t("repo.addButton")}
              </Button>
            </div>
          </form>
        </CardContent>
      </Card>
    </div>
  );
}

function Info({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-md border bg-background p-3">
      <div className="text-xs text-muted-foreground">{label}</div>
      <div className="mt-1 truncate font-medium">{value || "—"}</div>
    </div>
  );
}
