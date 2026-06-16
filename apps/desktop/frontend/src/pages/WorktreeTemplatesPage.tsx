import { useCallback, useEffect, useState } from "react";
import { FolderOpen, FolderPlus, RefreshCw, Save, Trash2, Upload } from "lucide-react";
import { api, errMessage } from "@/lib/api";
import type { Config, Repository, WorktreeTemplate } from "@/lib/types";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { useToast } from "@/components/ui/toast";
import { useConfirm } from "@/components/ui/confirm";
import { useI18n } from "@/i18n/I18nProvider";

interface Props {
  config: Config | null;
}

export function WorktreeTemplatesPage({ config }: Props) {
  const { t } = useI18n();
  const { notify } = useToast();
  const confirm = useConfirm();
  const [templates, setTemplates] = useState<WorktreeTemplate[]>([]);
  const [repos, setRepos] = useState<Repository[]>([]);
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    if (!config) return;
    try {
      const [items, configured] = await Promise.all([
        api.ListWorktreeTemplates(),
        api.ListRepos(),
      ]);
      setTemplates(items ?? []);
      setRepos(configured ?? []);
    } catch (e) {
      notify(errMessage(e), "error");
    }
  }, [config, notify]);

  useEffect(() => {
    void load();
  }, [load]);

  if (!config) {
    return (
      <Card className="max-w-3xl">
        <CardHeader>
          <CardTitle>{t("nav.worktreeTemplates")}</CardTitle>
          <CardDescription>{t("templates.noWorkspace")}</CardDescription>
        </CardHeader>
      </Card>
    );
  }

  function patchLocal(id: string, patch: Partial<WorktreeTemplate>) {
    setTemplates((items) =>
      items.map((item) => (item.id === id ? { ...item, ...patch } : item))
    );
  }

  async function save(item: WorktreeTemplate) {
    try {
      setBusy(true);
      await api.UpdateWorktreeTemplate({
        ...item,
        repoNames: item.repoNames ?? [],
      });
      notify(t("toast.templateSaved"), "success");
      await load();
    } catch (e) {
      notify(errMessage(e), "error");
    } finally {
      setBusy(false);
    }
  }

  async function importFiles() {
    try {
      setBusy(true);
      const added = await api.ImportWorktreeTemplateFiles();
      if ((added ?? []).length > 0) {
        notify(t("toast.templatesImported", { count: added.length }), "success");
      }
      await load();
    } catch (e) {
      notify(errMessage(e), "error");
    } finally {
      setBusy(false);
    }
  }

  async function importFolder() {
    try {
      setBusy(true);
      const added = await api.ImportWorktreeTemplateFolder();
      if (added?.id) notify(t("toast.templatesImported", { count: 1 }), "success");
      await load();
    } catch (e) {
      notify(errMessage(e), "error");
    } finally {
      setBusy(false);
    }
  }

  async function openTemplateFolder() {
    try {
      const path = await api.OpenWorktreeTemplateFolder();
      notify(t("toast.openedPath", { path }), "success");
    } catch (e) {
      notify(errMessage(e), "error");
    }
  }

  async function remove(item: WorktreeTemplate) {
    if (
      !(await confirm({
        message: t("templates.deleteConfirm", { name: item.name }),
        danger: true,
      }))
    )
      return;
    try {
      setBusy(true);
      await api.DeleteWorktreeTemplate(item.id);
      notify(t("toast.templateDeleted"), "success");
      await load();
    } catch (e) {
      notify(errMessage(e), "error");
    } finally {
      setBusy(false);
    }
  }

  function toggleRepo(item: WorktreeTemplate, repo: string, checked: boolean) {
    const current = new Set(item.repoNames ?? []);
    if (checked) current.add(repo);
    else current.delete(repo);
    patchLocal(item.id, { repoNames: [...current] });
  }

  return (
    <div className="max-w-5xl space-y-6">
      <Card>
        <CardHeader className="flex-row items-center justify-between space-y-0">
          <div>
            <CardTitle>{t("templates.title")}</CardTitle>
            <CardDescription>{t("templates.desc")}</CardDescription>
          </div>
          <div className="flex flex-wrap gap-2">
            <Button variant="outline" size="sm" onClick={load} disabled={busy}>
              <RefreshCw className="size-4" /> {t("common.refresh")}
            </Button>
            <Button variant="outline" size="sm" onClick={openTemplateFolder}>
              <FolderOpen className="size-4" /> {t("templates.openFolder")}
            </Button>
            <Button size="sm" onClick={importFiles} disabled={busy}>
              <Upload className="size-4" /> {t("templates.addFiles")}
            </Button>
            <Button size="sm" onClick={importFolder} disabled={busy}>
              <FolderPlus className="size-4" /> {t("templates.addFolder")}
            </Button>
          </div>
        </CardHeader>
        <CardContent className="space-y-4">
          {templates.length === 0 && (
            <p className="text-sm text-muted-foreground">{t("templates.empty")}</p>
          )}
          <div className="space-y-3">
            {templates.map((item) => (
              <div key={item.id} className="rounded-md border p-4">
                <div className="flex items-start justify-between gap-3">
                  <div className="min-w-0 flex-1 space-y-3">
                    <div className="flex items-center gap-2">
                      <Input
                        value={item.name}
                        onChange={(e) => patchLocal(item.id, { name: e.target.value })}
                        className="max-w-sm"
                      />
                      <Badge variant={item.enabled ? "success" : "outline"}>
                        {item.enabled ? t("templates.enabled") : t("templates.disabled")}
                      </Badge>
                    </div>
                    <div className="truncate font-mono text-xs text-muted-foreground" title={item.sourcePath}>
                      {item.sourcePath}
                    </div>
                    <div className="grid gap-3 md:grid-cols-3">
                      <label className="flex items-center gap-2 text-sm">
                        <input
                          type="checkbox"
                          checked={item.enabled}
                          onChange={(e) => patchLocal(item.id, { enabled: e.target.checked })}
                        />
                        {t("templates.enabled")}
                      </label>
                      <label className="flex items-center gap-2 text-sm">
                        <input
                          type="checkbox"
                          checked={item.overwrite}
                          onChange={(e) => patchLocal(item.id, { overwrite: e.target.checked })}
                        />
                        {t("templates.overwrite")}
                      </label>
                      <select
                        value={item.targetMode}
                        onChange={(e) =>
                          patchLocal(item.id, { targetMode: e.target.value })
                        }
                        className="h-9 rounded-md border border-input bg-background px-3 text-sm"
                      >
                        <option value="allRepos">{t("templates.targetAllRepos")}</option>
                        <option value="selectedRepos">{t("templates.targetSelectedRepos")}</option>
                        <option value="featureRoot">{t("templates.targetFeatureRoot")}</option>
                        <option value="agentAllRepos">{t("templates.targetAgentAllRepos")}</option>
                        <option value="agentSelectedRepos">{t("templates.targetAgentSelectedRepos")}</option>
                        <option value="agentRoot">{t("templates.targetAgentRoot")}</option>
                      </select>
                    </div>
                    {(item.targetMode === "selectedRepos" ||
                      item.targetMode === "agentSelectedRepos") && (
                      <div className="flex flex-wrap gap-3 rounded-md bg-muted/40 p-3">
                        {repos.map((repo) => (
                          <label key={repo.Name} className="flex items-center gap-2 text-sm">
                            <input
                              type="checkbox"
                              checked={(item.repoNames ?? []).includes(repo.Name)}
                              onChange={(e) => toggleRepo(item, repo.Name, e.target.checked)}
                            />
                            {repo.Name}
                          </label>
                        ))}
                        {repos.length === 0 && (
                          <span className="text-sm text-muted-foreground">
                            {t("feature.noRepos")}
                          </span>
                        )}
                      </div>
                    )}
                  </div>
                  <div className="flex shrink-0 gap-2">
                    <Button variant="outline" size="sm" onClick={() => save(item)} disabled={busy}>
                      <Save className="size-4" /> {t("agentsec.save")}
                    </Button>
                    <Button variant="destructive" size="sm" onClick={() => remove(item)} disabled={busy}>
                      <Trash2 className="size-4" /> {t("common.delete")}
                    </Button>
                  </div>
                </div>
              </div>
            ))}
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
