import { type CSSProperties, useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  ChevronDown,
  ChevronRight,
  Edit3,
  FileText,
  Folder,
  FolderOpen,
  FolderPlus,
  RefreshCw,
  Save,
  Trash2,
  Upload,
  X,
} from "lucide-react";
import { api, errMessage } from "@/lib/api";
import type { Config, Repository, WorktreeTemplate, WorktreeTemplateTargetMode } from "@/lib/types";
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
import { Textarea } from "@/components/ui/textarea";
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
import { cn } from "@/lib/utils";
import { runtime } from "@/components/TerminalPanel";

interface Props {
  config: Config | null;
}

type LogicalFolder = {
  id: string;
  label: string;
  description: string;
  targetMode: WorktreeTemplateTargetMode;
  repoNames: string[];
  children?: LogicalFolder[];
};

type EditorState = {
  id: string;
  name: string;
  content: string;
};

function buildTree(repos: Repository[], t: (key: string) => string): LogicalFolder {
  const featureRepos = repos.map((repo) => ({
    id: `features/${repo.Name}`,
    label: repo.Name,
    description: `${t("templates.featuresFolder")} / ${repo.Name}`,
    targetMode: "selectedRepos",
    repoNames: [repo.Name],
  }));
  const agentRepos = repos.map((repo) => ({
    id: `agents/${repo.Name}`,
    label: repo.Name,
    description: `${t("templates.agentsFolder")} / ${repo.Name}`,
    targetMode: "agentSelectedRepos",
    repoNames: [repo.Name],
  }));
  return {
    id: "root",
    label: t("templates.rootFolder"),
    description: t("templates.rootFolderDesc"),
    targetMode: "workspaceRoot",
    repoNames: [],
    children: [
      {
        id: "features",
        label: t("templates.featuresFolder"),
        description: t("templates.featuresFolderDesc"),
        targetMode: "allRepos",
        repoNames: [],
        children: featureRepos,
      },
      {
        id: "agents",
        label: t("templates.agentsFolder"),
        description: t("templates.agentsFolderDesc"),
        targetMode: "agentAllRepos",
        repoNames: [],
        children: agentRepos,
      },
    ],
  };
}

function flattenFolders(node: LogicalFolder): LogicalFolder[] {
  return [node, ...(node.children ?? []).flatMap(flattenFolders)];
}

function matchesFolder(item: WorktreeTemplate, folder: LogicalFolder): boolean {
  if (item.targetMode !== folder.targetMode) return false;
  if (folder.repoNames.length === 0) return true;
  return folder.repoNames.every((repo) => (item.repoNames ?? []).includes(repo));
}

function targetLabel(item: WorktreeTemplate, t: (key: string) => string): string {
  switch (item.targetMode) {
    case "featureRoot":
      return t("templates.targetFeatureRoot");
    case "workspaceRoot":
      return t("templates.targetWorkspaceRoot");
    case "allRepos":
      return t("templates.targetAllRepos");
    case "selectedRepos":
      return `${t("templates.targetSelectedRepos")}: ${(item.repoNames ?? []).join(", ")}`;
    case "agentRoot":
      return t("templates.targetAgentRoot");
    case "agentAllRepos":
      return t("templates.targetAgentAllRepos");
    case "agentSelectedRepos":
      return `${t("templates.targetAgentSelectedRepos")}: ${(item.repoNames ?? []).join(", ")}`;
    default:
      return String(item.targetMode);
  }
}

export function WorktreeTemplatesPage({ config }: Props) {
  const { t } = useI18n();
  const { notify } = useToast();
  const confirm = useConfirm();
  const [templates, setTemplates] = useState<WorktreeTemplate[]>([]);
  const [repos, setRepos] = useState<Repository[]>([]);
  const [selectedFolderId, setSelectedFolderId] = useState("root");
  const [expanded, setExpanded] = useState<Set<string>>(new Set(["root", "features", "agents"]));
  const [selectedTemplateId, setSelectedTemplateId] = useState("");
  const [editor, setEditor] = useState<EditorState | null>(null);
  const [busy, setBusy] = useState(false);
  const dropRef = useRef<HTMLDivElement | null>(null);

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

  const tree = useMemo(() => buildTree(repos, t), [repos, t]);
  const folders = useMemo(() => flattenFolders(tree), [tree]);
  const selectedFolder = folders.find((folder) => folder.id === selectedFolderId) ?? tree;
  const visibleTemplates = useMemo(
    () => templates.filter((item) => matchesFolder(item, selectedFolder)),
    [templates, selectedFolder]
  );
  const selectedTemplate = visibleTemplates.find((item) => item.id === selectedTemplateId) ?? visibleTemplates[0];

  useEffect(() => {
    if (!visibleTemplates.some((item) => item.id === selectedTemplateId)) {
      setSelectedTemplateId(visibleTemplates[0]?.id ?? "");
    }
  }, [selectedTemplateId, visibleTemplates]);

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

  function withSelectedTarget(item: WorktreeTemplate): WorktreeTemplate {
    return {
      ...item,
      targetMode: selectedFolder.targetMode,
      repoNames: selectedFolder.repoNames,
    };
  }

  async function save(item: WorktreeTemplate, quiet = false) {
    try {
      setBusy(true);
      await api.UpdateWorktreeTemplate({
        ...item,
        repoNames: item.repoNames ?? [],
      });
      if (!quiet) notify(t("toast.templateSaved"), "success");
      await load();
    } catch (e) {
      notify(errMessage(e), "error");
      await load();
    } finally {
      setBusy(false);
    }
  }

  async function patchAndSave(item: WorktreeTemplate, patch: Partial<WorktreeTemplate>) {
    const next = { ...item, ...patch, repoNames: item.repoNames ?? [] };
    patchLocal(item.id, patch);
    await save(next, true);
  }

  async function assignImported(items: WorktreeTemplate[]) {
    if (items.length === 0) return;
    await Promise.all(items.map((item) => api.UpdateWorktreeTemplate(withSelectedTarget(item))));
    setSelectedTemplateId(items[0].id);
  }

  async function importFiles() {
    try {
      setBusy(true);
      const added = await api.ImportWorktreeTemplateFiles();
      await assignImported(added ?? []);
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
      if (added?.id) {
        await assignImported([added]);
        notify(t("toast.templatesImported", { count: 1 }), "success");
      }
      await load();
    } catch (e) {
      notify(errMessage(e), "error");
    } finally {
      setBusy(false);
    }
  }

  async function importPaths(paths: string[]) {
    const clean = paths.filter((path) => path.trim().length > 0);
    if (clean.length === 0) return;
    try {
      setBusy(true);
      const added = await api.ImportWorktreeTemplatePaths(clean);
      await assignImported(added ?? []);
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

  useEffect(() => {
    const rt = runtime();
    if (!rt) return;
    return rt.EventsOn("workspace:file-drop", (...data: unknown[]) => {
      const payload = data[0] as { x?: number; y?: number; paths?: string[] };
      if (!payload?.paths?.length) return;
      const target =
        typeof payload.x === "number" && typeof payload.y === "number"
          ? document.elementFromPoint(payload.x, payload.y)
          : null;
      if (target && !dropRef.current?.contains(target)) return;
      void importPaths(payload.paths);
    });
  }, [selectedFolder.id]);

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
      if (editor?.id === item.id) setEditor(null);
      notify(t("toast.templateDeleted"), "success");
      await load();
    } catch (e) {
      notify(errMessage(e), "error");
    } finally {
      setBusy(false);
    }
  }

  async function clearAll() {
    if (
      !(await confirm({
        message: t("templates.clearConfirm"),
        danger: true,
      }))
    )
      return;
    try {
      setBusy(true);
      await api.ClearWorktreeTemplates();
      setEditor(null);
      setSelectedTemplateId("");
      notify(t("toast.templatesCleared"), "success");
      await load();
    } catch (e) {
      notify(errMessage(e), "error");
    } finally {
      setBusy(false);
    }
  }

  async function openEditor(item: WorktreeTemplate) {
    try {
      setBusy(true);
      const content = await api.ReadWorktreeTemplateFile(item.id);
      setEditor({ id: item.id, name: item.name, content });
    } catch (e) {
      notify(errMessage(e), "error");
    } finally {
      setBusy(false);
    }
  }

  async function saveEditor() {
    if (!editor) return;
    try {
      setBusy(true);
      await api.SaveWorktreeTemplateFile(editor.id, editor.content);
      notify(t("templates.editorSaved"), "success");
    } catch (e) {
      notify(errMessage(e), "error");
    } finally {
      setBusy(false);
    }
  }

  function FolderNode({ node, depth }: { node: LogicalFolder; depth: number }) {
    const open = expanded.has(node.id);
    const active = selectedFolder.id === node.id;
    const hasChildren = (node.children ?? []).length > 0;
    return (
      <div>
        <button
          type="button"
          className={cn(
            "flex w-full items-center gap-1 rounded px-2 py-1.5 text-left text-sm hover:bg-accent",
            active && "bg-secondary font-medium text-secondary-foreground"
          )}
          style={{ paddingLeft: 8 + depth * 16 }}
          onClick={() => setSelectedFolderId(node.id)}
        >
          <span
            className="flex size-4 items-center justify-center"
            onClick={(e) => {
              e.stopPropagation();
              if (!hasChildren) return;
              const next = new Set(expanded);
              if (next.has(node.id)) next.delete(node.id);
              else next.add(node.id);
              setExpanded(next);
            }}
          >
            {hasChildren ? open ? <ChevronDown className="size-4" /> : <ChevronRight className="size-4" /> : null}
          </span>
          {open && hasChildren ? (
            <FolderOpen className="size-4 text-amber-600" />
          ) : (
            <Folder className="size-4 text-amber-600" />
          )}
          <span className="truncate">{node.label}</span>
        </button>
        {hasChildren && open && (
          <div>
            {(node.children ?? []).map((child) => (
              <FolderNode key={child.id} node={child} depth={depth + 1} />
            ))}
          </div>
        )}
      </div>
    );
  }

  return (
    <div className="grid h-[calc(100vh-9rem)] grid-cols-[minmax(240px,320px)_1fr] gap-4">
      <Card className="overflow-hidden">
        <CardHeader>
          <CardTitle>{t("templates.logicalTree")}</CardTitle>
          <CardDescription>{t("templates.logicalTreeDesc")}</CardDescription>
        </CardHeader>
        <CardContent className="h-full overflow-auto pb-24">
          <FolderNode node={tree} depth={0} />
        </CardContent>
      </Card>

      <div className="min-w-0 space-y-4 overflow-auto pr-1">
        <Card>
          <CardHeader className="space-y-3">
            <div className="flex flex-wrap items-start justify-between gap-3">
              <div>
                <CardTitle>{selectedFolder.label}</CardTitle>
                <CardDescription>{selectedFolder.description}</CardDescription>
              </div>
              <Button variant="outline" size="sm" onClick={load} disabled={busy}>
                <RefreshCw className="size-4" /> {t("common.refresh")}
              </Button>
            </div>
            <div className="flex flex-wrap gap-2">
              <Button size="sm" onClick={importFiles} disabled={busy}>
                <Upload className="size-4" /> {t("templates.addFiles")}
              </Button>
              <Button size="sm" onClick={importFolder} disabled={busy}>
                <FolderPlus className="size-4" /> {t("templates.addFolder")}
              </Button>
              <Button variant="outline" size="sm" onClick={openTemplateFolder}>
                <FolderOpen className="size-4" /> {t("templates.openFolder")}
              </Button>
              <Button variant="destructive" size="sm" onClick={clearAll} disabled={busy || templates.length === 0}>
                <Trash2 className="size-4" /> {t("templates.clearAll")}
              </Button>
            </div>
          </CardHeader>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>{t("templates.uploadTitle")}</CardTitle>
            <CardDescription>{t("templates.uploadDesc")}</CardDescription>
          </CardHeader>
          <CardContent>
            <div
              ref={dropRef}
              data-template-dropzone="true"
              style={{ "--wails-drop-target": "drop" } as CSSProperties}
              className="flex min-h-36 flex-col items-center justify-center gap-3 rounded-lg border border-dashed bg-muted/30 p-6 text-center"
              onDragOver={(e) => e.preventDefault()}
              onDrop={(e) => {
                e.preventDefault();
                const paths = Array.from(e.dataTransfer.files)
                  .map((file) => (file as File & { path?: string }).path ?? "")
                  .filter(Boolean);
                void importPaths(paths);
              }}
            >
              <Upload className="size-8 text-muted-foreground" />
              <div>
                <div className="font-medium">{t("templates.dropFiles")}</div>
                <p className="mt-1 text-sm text-muted-foreground">
                  {t("templates.dropFilesHint")}
                </p>
              </div>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>{t("templates.listTitle")}</CardTitle>
            <CardDescription>{t("templates.listDesc")}</CardDescription>
          </CardHeader>
          <CardContent>
            {visibleTemplates.length === 0 ? (
              <p className="text-sm text-muted-foreground">{t("templates.emptyInFolder")}</p>
            ) : (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t("repo.colName")}</TableHead>
                    <TableHead>{t("templates.sourcePath")}</TableHead>
                    <TableHead>{t("templates.enabled")}</TableHead>
                    <TableHead>{t("templates.overwrite")}</TableHead>
                    <TableHead className="text-right">{t("repo.colAction")}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {visibleTemplates.map((item) => (
                    <TableRow key={item.id}>
                      <TableCell className="min-w-48">
                        <div className="flex items-center gap-2">
                          <FileText className="size-4 text-muted-foreground" />
                          <span className="font-medium">{item.name}</span>
                          <Badge variant={item.enabled ? "success" : "outline"}>
                            {item.enabled ? t("templates.enabled") : t("templates.disabled")}
                          </Badge>
                        </div>
                      </TableCell>
                      <TableCell className="max-w-72 truncate font-mono text-xs text-muted-foreground" title={item.sourcePath}>
                        {item.sourcePath}
                      </TableCell>
                      <TableCell>
                        <Button
                          variant={item.enabled ? "secondary" : "outline"}
                          size="sm"
                          onClick={() => patchAndSave(item, { enabled: !item.enabled })}
                          disabled={busy}
                        >
                          {item.enabled ? t("templates.enabled") : t("templates.disabled")}
                        </Button>
                      </TableCell>
                      <TableCell>
                        <Button
                          variant={item.overwrite ? "secondary" : "outline"}
                          size="sm"
                          onClick={() => patchAndSave(item, { overwrite: !item.overwrite })}
                          disabled={busy}
                        >
                          {item.overwrite ? t("templates.overwriteOn") : t("templates.overwriteOff")}
                        </Button>
                      </TableCell>
                      <TableCell>
                        <div className="flex justify-end gap-2">
                          <Button variant="outline" size="sm" onClick={() => openEditor(item)} disabled={busy}>
                            <Edit3 className="size-4" /> {t("templates.openEditor")}
                          </Button>
                          <Button variant="destructive" size="sm" onClick={() => remove(item)} disabled={busy}>
                            <Trash2 className="size-4" /> {t("common.delete")}
                          </Button>
                        </div>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            )}
          </CardContent>
        </Card>

        {editor && (
          <Card>
            <CardHeader className="flex-row items-start justify-between space-y-0">
              <div>
                <CardTitle>{t("templates.editorTitle")}: {editor.name}</CardTitle>
                <CardDescription>{t("templates.editorDesc")}</CardDescription>
              </div>
              <Button variant="ghost" size="icon" onClick={() => setEditor(null)}>
                <X className="size-4" />
              </Button>
            </CardHeader>
            <CardContent className="space-y-3">
              <Textarea
                value={editor.content}
                onChange={(e) => setEditor({ ...editor, content: e.target.value })}
                className="min-h-[360px]"
              />
              <div className="flex justify-end gap-2">
                <Button variant="outline" onClick={() => setEditor(null)}>
                  {t("common.close")}
                </Button>
                <Button onClick={saveEditor} disabled={busy}>
                  <Save className="size-4" /> {t("templates.saveFile")}
                </Button>
              </div>
            </CardContent>
          </Card>
        )}
      </div>
    </div>
  );
}
