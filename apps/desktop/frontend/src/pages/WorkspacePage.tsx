import { useCallback, useEffect, useState } from "react";
import {
  DownloadCloud,
  FolderOpen,
  Loader2,
  Plus,
  RefreshCw,
  Sparkles,
  Trash2,
} from "lucide-react";
import { api, errMessage } from "@/lib/api";
import type { Config, RepoRuntimeState, TerminalSession } from "@/lib/types";
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
  onOpenTerminal: (session: TerminalSession) => void;
}

type AuthRequired = {
  code: "authentication_required";
  repo: string;
  host: string;
  protocol: "https";
};

type CredentialPrompt = AuthRequired & {
  resolve: (value: { username: string; secret: string } | null) => void;
};

function authRequired(error: unknown): AuthRequired | null {
  const message = errMessage(error);
  const prefix = "AGENTSAFE_AUTH_REQUIRED:";
  const index = message.indexOf(prefix);
  if (index < 0) return null;
  try {
    const value = JSON.parse(message.slice(index + prefix.length)) as AuthRequired;
    return value.code === "authentication_required" && value.host ? value : null;
  } catch {
    return null;
  }
}

export function WorkspacePage({ config, root, onLoaded, onChanged, onOpenTerminal }: Props) {
  const { notify } = useToast();
  const confirm = useConfirm();
  const { t } = useI18n();
  const [busy, setBusy] = useState(false);
  const [actingRepo, setActingRepo] = useState<string | null>(null);
  const [repoLocalStates, setRepoLocalStates] = useState<Record<string, boolean>>({});
  const [repoRuntimeStates, setRepoRuntimeStates] = useState<Record<string, RepoRuntimeState>>({});
  const [selectedRemoteBranches, setSelectedRemoteBranches] = useState<Record<string, string>>({});
  const [repoStatesLoading, setRepoStatesLoading] = useState(false);
  const [credentialPrompt, setCredentialPrompt] = useState<CredentialPrompt | null>(null);
  const [initName, setInitName] = useState("");

  // Add-repo form
  const [repoName, setRepoName] = useState("");
  const [repoUrl, setRepoUrl] = useState("");

  const loadRepoStates = useCallback(async () => {
    if (!config) {
      setRepoLocalStates({});
      setRepoRuntimeStates({});
      return;
    }
    setRepoStatesLoading(true);
    try {
      const [local, runtime] = await Promise.all([
        api.RepoLocalStates(),
        api.RepoRuntimeStates(),
      ]);
      setRepoLocalStates(local);
      const runtimeByName = Object.fromEntries(runtime.map((item) => [item.name, item]));
      setRepoRuntimeStates(runtimeByName);
      setSelectedRemoteBranches((prev) => {
        const next = { ...prev };
        for (const item of runtime) {
          const branches = item.remoteBranches ?? [];
          if (!next[item.name] || !branches.includes(next[item.name])) {
            next[item.name] = branches.includes(item.currentBranch)
              ? item.currentBranch
              : branches[0] ?? "";
          }
        }
        return next;
      });
    } catch (e) {
      notify(errMessage(e), "error");
    } finally {
      setRepoStatesLoading(false);
    }
  }, [config, notify]);

  useEffect(() => {
    void loadRepoStates();
  }, [loadRepoStates, root]);

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
      await api.AddRepo(repoName.trim(), repoUrl.trim(), "");
      setRepoName("");
      setRepoUrl("");
      await onChanged();
      await loadRepoStates();
      notify(t("toast.repoAdded"), "success");
    } catch (err) {
      notify(errMessage(err), "error");
    } finally {
      setBusy(false);
    }
  }

  async function removeRepo(name: string) {
    let deleteFiles = false;
    const ok = await confirm({
      message: t("confirm.removeRepo", { name }),
      danger: true,
      checkbox: {
        label: t("confirm.removeRepoDeleteFiles", { name }),
        onChange: (c) => {
          deleteFiles = c;
        },
      },
    });
    if (!ok) {
      return;
    }
    try {
      setBusy(true);
      await api.RemoveRepo(name, deleteFiles);
      await onChanged();
      notify(t("toast.repoRemoved"), "success");
    } catch (err) {
      notify(errMessage(err), "error");
    } finally {
      setBusy(false);
    }
  }

  async function pull() {
    if (!config) return;
    setBusy(true);
    let failed = 0;
    try {
      for (const repository of config.Repositories ?? []) {
        setActingRepo(repository.Name);
        try {
          await pullRepoWithAuthentication(repository.Name);
        } catch (e) {
          failed++;
          notify(errMessage(e), "error");
        }
      }
      await loadRepoStates();
      if (failed === 0) {
        notify(t("toast.pullCompleted"), "success");
      } else {
        notify(t("toast.pullCompletedWithFailures", { count: failed }), "error");
      }
    } finally {
      setActingRepo(null);
      setBusy(false);
    }
  }

  async function pullRepo(name: string) {
    try {
      setActingRepo(name);
      const cloned = !repoLocalStates[name];
      await pullRepoWithAuthentication(name);
      await loadRepoStates();
      notify(
        cloned
          ? t("toast.repoCloned", { name })
          : t("toast.repoPulled", { name }),
        "success"
      );
    } catch (e) {
      notify(errMessage(e), "error");
    } finally {
      setActingRepo(null);
    }
  }

  async function pullRepoWithAuthentication(name: string) {
    try {
      await api.PullRepo(name);
      return;
    } catch (error) {
      const auth = authRequired(error);
      if (!auth) throw error;
      const credentials = await requestCredentials(auth);
      if (!credentials) {
        throw new Error(t("gitAuth.cancelled", { repo: name }));
      }
      await api.SetGitCredentials(auth.host, credentials.username, credentials.secret);
      try {
        await api.PullRepo(name);
      } catch (retryError) {
        if (authRequired(retryError)) {
          throw new Error(t("gitAuth.failed", { repo: name }));
        }
        throw retryError;
      }
    }
  }

  function requestCredentials(auth: AuthRequired) {
    return new Promise<{ username: string; secret: string } | null>((resolve) => {
      setCredentialPrompt({ ...auth, resolve });
    });
  }

  async function openWorkspace(
    action: () => Promise<string>,
    successKey: string
  ) {
    try {
      setBusy(true);
      const path = await action();
      notify(t(successKey, { path }), "success");
    } catch (e) {
      notify(errMessage(e), "error");
    } finally {
      setBusy(false);
    }
  }

  async function openWorkspaceTerminalTab(terminalProgram?: string) {
    try {
      setBusy(true);
      const s = terminalProgram
        ? await api.TerminalOpenWithProgram(root, terminalProgram)
        : await api.TerminalOpen(root);
      if (s.external) {
        notify(t("toast.openedWorkspaceTerminal", { path: s.path }), "success");
        return;
      }
      onOpenTerminal(s);
    } catch (e) {
      notify(errMessage(e), "error");
    } finally {
      setBusy(false);
    }
  }

  async function openWorkspaceTool(toolCommand: string) {
    try {
      setBusy(true);
      const path = await api.OpenPathInProgram("", toolCommand);
      notify(t("toast.openedPath", { path }), "success");
    } catch (e) {
      notify(errMessage(e), "error");
      throw e;
    } finally {
      setBusy(false);
    }
  }

  async function browseAndOpenWorkspaceTool() {
    let toolCommand: string;
    try {
      toolCommand = await api.SelectProgram();
    } catch (e) {
      notify(errMessage(e), "error");
      throw e;
    }
    if (!toolCommand) return;
    await openWorkspaceTool(toolCommand);
  }

  async function checkoutRepo(name: string) {
    const branch = selectedRemoteBranches[name];
    if (!branch) return;
    try {
      setActingRepo(name);
      await api.CheckoutRepoBranch(name, branch);
      await loadRepoStates();
      notify(t("toast.repoCheckedOut", { name, branch }), "success");
    } catch (e) {
      notify(errMessage(e), "error");
    } finally {
      setActingRepo(null);
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
    <>
    <div className="space-y-6">
      <Card>
        <CardHeader className="flex-row items-start justify-between gap-4 space-y-0">
          <div className="min-w-0">
            <CardTitle>{config.Workspace.Name}</CardTitle>
            <CardDescription className="break-all">{root}</CardDescription>
          </div>
          <div className="flex shrink-0 justify-end">
            <ToolOpenMenu
              location="workspace"
              disabled={busy}
              onFolder={() =>
                openWorkspace(
                  () => api.OpenWorkspaceFolder(),
                  "toast.openedWorkspaceFolder"
                )
              }
              onTerminal={openWorkspaceTerminalTab}
              onTool={openWorkspaceTool}
              onToolBrowse={browseAndOpenWorkspaceTool}
            />
          </div>
        </CardHeader>
      </Card>

      <Card>
        <CardHeader className="flex-row items-center justify-between space-y-0">
          <div>
            <CardTitle>{t("repo.title")}</CardTitle>
            <CardDescription>
              {t("repo.countConfigured", { count: repos.length })}
            </CardDescription>
          </div>
          <Button
            variant="outline"
            size="sm"
            onClick={pull}
            disabled={busy || actingRepo !== null}
          >
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
                  <TableHead>{t("repo.colBranch")}</TableHead>
                  <TableHead>{t("repo.colCheckout")}</TableHead>
                  <TableHead>{t("repo.colUrl")}</TableHead>
                  <TableHead>{t("repo.colAction")}</TableHead>
                  <TableHead className="w-10"></TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {repos.map((r) => (
                  <TableRow key={r.Name}>
                    <TableCell className="font-medium">{r.Name}</TableCell>
                    <TableCell>
                      {repoStatesLoading ? (
                        <span className="text-muted-foreground">{t("common.loading")}</span>
                      ) : repoRuntimeStates[r.Name]?.currentBranch ? (
                        <span className="font-mono text-xs">{repoRuntimeStates[r.Name].currentBranch}</span>
                      ) : repoRuntimeStates[r.Name]?.local ? (
                        <span className="text-muted-foreground">{t("repo.detached")}</span>
                      ) : (
                        <span className="text-muted-foreground">—</span>
                      )}
                    </TableCell>
                    <TableCell>
                      <div className="flex items-center gap-2">
                        <select
                          className="h-8 min-w-32 rounded-md border bg-background px-2 text-xs"
                          disabled={
                            busy ||
                            repoStatesLoading ||
                            actingRepo === r.Name ||
                            !(repoRuntimeStates[r.Name]?.remoteBranches ?? []).length
                          }
                          value={selectedRemoteBranches[r.Name] ?? ""}
                          onChange={(e) =>
                            setSelectedRemoteBranches((prev) => ({
                              ...prev,
                              [r.Name]: e.target.value,
                            }))
                          }
                        >
                          {(repoRuntimeStates[r.Name]?.remoteBranches ?? []).length === 0 && (
                            <option value="">{t("repo.noRemoteBranches")}</option>
                          )}
                          {(repoRuntimeStates[r.Name]?.remoteBranches ?? []).map((branch) => (
                            <option key={branch} value={branch}>
                              origin/{branch}
                            </option>
                          ))}
                        </select>
                        <Button
                          variant="outline"
                          size="sm"
                          disabled={
                            busy ||
                            repoStatesLoading ||
                            actingRepo === r.Name ||
                            !selectedRemoteBranches[r.Name]
                          }
                          onClick={() => checkoutRepo(r.Name)}
                        >
                          {actingRepo === r.Name ? (
                            <Loader2 className="size-4 animate-spin" />
                          ) : (
                            <RefreshCw className="size-4" />
                          )}
                          {t("repo.checkout")}
                        </Button>
                      </div>
                    </TableCell>
                    <TableCell className="max-w-xs truncate text-muted-foreground">
                      {r.URL}
                    </TableCell>
                    <TableCell>
                      <Button
                        variant="outline"
                        size="sm"
                        disabled={busy || repoStatesLoading || actingRepo === r.Name}
                        onClick={() => pullRepo(r.Name)}
                      >
                        {actingRepo === r.Name || repoStatesLoading ? (
                          <Loader2 className="size-4 animate-spin" />
                        ) : repoLocalStates[r.Name] ? (
                          <RefreshCw className="size-4" />
                        ) : (
                          <DownloadCloud className="size-4" />
                        )}
                        {repoLocalStates[r.Name]
                          ? t("repo.pullOne")
                          : t("repo.cloneOne")}
                      </Button>
                    </TableCell>
                    <TableCell>
                      <Button
                        variant="ghost"
                        size="icon"
                        className="size-8 text-muted-foreground hover:text-destructive"
                        disabled={busy || actingRepo === r.Name}
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
            <div className="sm:col-span-2">
              <Button type="submit" disabled={busy}>
                <Plus className="size-4" /> {t("repo.addButton")}
              </Button>
            </div>
          </form>
        </CardContent>
      </Card>
    </div>
    {credentialPrompt && (
      <CredentialDialog
        request={credentialPrompt}
        onClose={(value) => {
          credentialPrompt.resolve(value);
          setCredentialPrompt(null);
        }}
      />
    )}
    </>
  );
}

function CredentialDialog({
  request,
  onClose,
}: {
  request: CredentialPrompt;
  onClose: (value: { username: string; secret: string } | null) => void;
}) {
  const { t } = useI18n();
  const [username, setUsername] = useState("");
  const [secret, setSecret] = useState("");

  return (
    <div
      className="fixed inset-0 z-[70] flex items-center justify-center bg-black/50 p-4"
      onClick={() => onClose(null)}
    >
      <form
        className="w-full max-w-md rounded-lg border bg-card p-5 shadow-xl"
        onClick={(e) => e.stopPropagation()}
        onSubmit={(e) => {
          e.preventDefault();
          if (username.trim() && secret) {
            onClose({ username: username.trim(), secret });
          }
        }}
      >
        <h2 className="text-base font-semibold">{t("gitAuth.title")}</h2>
        <p className="mt-1 text-sm text-muted-foreground">
          {t("gitAuth.description", { repo: request.repo, host: request.host })}
        </p>
        <div className="mt-4 space-y-3">
          <div className="space-y-1.5">
            <Label htmlFor="gitAuthUsername">{t("gitAuth.username")}</Label>
            <Input
              id="gitAuthUsername"
              autoFocus
              autoComplete="username"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
            />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="gitAuthSecret">{t("gitAuth.secret")}</Label>
            <Input
              id="gitAuthSecret"
              type="password"
              autoComplete="current-password"
              value={secret}
              onChange={(e) => setSecret(e.target.value)}
            />
          </div>
          <p className="text-xs text-muted-foreground">{t("gitAuth.sessionOnly")}</p>
        </div>
        <div className="mt-5 flex justify-end gap-2">
          <Button type="button" variant="outline" onClick={() => onClose(null)}>
            {t("common.cancel")}
          </Button>
          <Button type="submit" disabled={!username.trim() || !secret}>
            {t("gitAuth.retry")}
          </Button>
        </div>
      </form>
    </div>
  );
}
