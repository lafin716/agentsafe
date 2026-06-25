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
import type {
  Config,
  Repository,
  WorktreeTemplate,
  WorktreeTemplateTargetMode,
  WorktreeTemplateTree,
  WorktreeTemplateTreeNode,
} from "@/lib/types";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Textarea } from "@/components/ui/textarea";
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
  templateId: string;
  relPath: string;
  name: string;
  content: string;
  savedContent: string;
};

type Counts = { files: number; folders: number };

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
        description: "Templates copied to each feature root folder.",
        targetMode: "featureRoot",
        repoNames: [],
        children: [
          {
            id: "features/all-repos",
            label: "All repositories",
            description: t("templates.featuresFolderDesc"),
            targetMode: "allRepos",
            repoNames: [],
          },
          ...featureRepos,
        ],
      },
      {
        id: "agents",
        label: t("templates.agentsFolder"),
        description: "Templates copied to each agent root folder.",
        targetMode: "agentRoot",
        repoNames: [],
        children: [
          {
            id: "agents/all-repos",
            label: "All agent repositories",
            description: t("templates.agentsFolderDesc"),
            targetMode: "agentAllRepos",
            repoNames: [],
          },
          ...agentRepos,
        ],
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

function nodeKey(templateId: string, relPath: string) {
  return `${templateId}:${relPath || "."}`;
}

function countForFolder(trees: WorktreeTemplateTree[], folder: LogicalFolder): Counts {
  return trees.reduce(
    (sum, tree) => {
      if (!matchesFolder(tree.template, folder)) return sum;
      return {
        files: sum.files + (tree.root.files ?? 0),
        folders: sum.folders + (tree.root.folders ?? 0),
      };
    },
    { files: 0, folders: 0 }
  );
}

function CountBadges({ counts }: { counts: Counts }) {
  if (counts.files === 0 && counts.folders === 0) return null;
  return (
    <span className="ml-auto flex shrink-0 items-center gap-1">
      <Badge variant="secondary" className="gap-1 px-1.5 text-[10px]">
        <FileText className="size-3" /> {counts.files}
      </Badge>
      <Badge variant="outline" className="gap-1 px-1.5 text-[10px]">
        <Folder className="size-3" /> {counts.folders}
      </Badge>
    </span>
  );
}

export function WorktreeTemplatesPage({ config }: Props) {
  const { t } = useI18n();
  const { notify } = useToast();
  const confirm = useConfirm();
  const [templateTrees, setTemplateTrees] = useState<WorktreeTemplateTree[]>([]);
  const [repos, setRepos] = useState<Repository[]>([]);
  const [selectedFolderId, setSelectedFolderId] = useState("root");
  const [expandedFolders, setExpandedFolders] = useState<Set<string>>(new Set(["root", "features", "agents"]));
  const [expandedTemplateNodes, setExpandedTemplateNodes] = useState<Set<string>>(new Set());
  const [editor, setEditor] = useState<EditorState | null>(null);
  const [busy, setBusy] = useState(false);
  const dropRef = useRef<HTMLDivElement | null>(null);
  const pageRef = useRef<HTMLDivElement | null>(null);

  const templates = useMemo(() => templateTrees.map((tree) => tree.template), [templateTrees]);

  const load = useCallback(async () => {
    if (!config) return;
    try {
      const [trees, configured] = await Promise.all([
        api.ListWorktreeTemplateTrees(),
        api.ListRepos(),
      ]);
      setTemplateTrees(trees ?? []);
      setRepos(configured ?? []);
      setExpandedTemplateNodes((prev) => {
        const next = new Set(prev);
        for (const tree of trees ?? []) {
          const effectiveRoot =
            tree.root.children?.length === 1 ? tree.root.children[0] : tree.root;
          next.add(nodeKey(tree.template.id, effectiveRoot.relPath));
        }
        return next;
      });
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
  const isRootSelected = selectedFolder.id === "root";
  const visibleTrees = useMemo(
    () => templateTrees.filter((item) => matchesFolder(item.template, selectedFolder)),
    [templateTrees, selectedFolder]
  );

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
    setTemplateTrees((items) =>
      items.map((tree) =>
        tree.template.id === id ? { ...tree, template: { ...tree.template, ...patch } } : tree
      )
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
    if (isRootSelected) {
      notify("Select a feature, repository, agent root, or agent repository before uploading.", "error");
      return;
    }
    await Promise.all(items.map((item) => api.UpdateWorktreeTemplate(withSelectedTarget(item))));
  }

  async function importFiles() {
    if (isRootSelected) return;
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
    if (isRootSelected) return;
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
    if (isRootSelected) {
      notify("Upload is disabled on the root folder. Select a destination below root first.", "error");
      return;
    }
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
      // Accept a drop anywhere on this page (so users can drop onto the whole
      // panel, not just the dashed zone). The coordinate check still scopes the
      // drop to this page's pane when multiple pages are open side by side.
      if (target && pageRef.current && !pageRef.current.contains(target)) return;
      void importPaths(payload.paths);
    });
  }, [selectedFolder.id, isRootSelected]);

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
      if (editor?.templateId === item.id) setEditor(null);
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
      setExpandedTemplateNodes(new Set());
      notify(t("toast.templatesCleared"), "success");
      await load();
    } catch (e) {
      notify(errMessage(e), "error");
    } finally {
      setBusy(false);
    }
  }

  async function openEditor(item: WorktreeTemplate, node: WorktreeTemplateTreeNode) {
    if (node.isDir || !node.relPath) return;
    try {
      setBusy(true);
      const content = await api.ReadWorktreeTemplateTreeFile(item.id, node.relPath);
      setEditor({
        templateId: item.id,
        relPath: node.relPath,
        name: `${item.name} / ${node.relPath}`,
        content,
        savedContent: content,
      });
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
      await api.SaveWorktreeTemplateTreeFile(
        editor.templateId,
        editor.relPath,
        editor.content
      );
      setEditor({ ...editor, savedContent: editor.content });
      notify(t("templates.editorSaved"), "success");
      await load();
    } catch (e) {
      notify(errMessage(e), "error");
    } finally {
      setBusy(false);
    }
  }

  function toggleTemplateNode(id: string) {
    const next = new Set(expandedTemplateNodes);
    if (next.has(id)) next.delete(id);
    else next.add(id);
    setExpandedTemplateNodes(next);
  }

  function FolderNode({ node, depth }: { node: LogicalFolder; depth: number }) {
    const open = expandedFolders.has(node.id);
    const active = selectedFolder.id === node.id;
    const hasChildren = (node.children ?? []).length > 0;
    const counts = countForFolder(templateTrees, node);
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
              const next = new Set(expandedFolders);
              if (next.has(node.id)) next.delete(node.id);
              else next.add(node.id);
              setExpandedFolders(next);
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
          {node.id !== "root" && <CountBadges counts={counts} />}
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

  function TemplateFileNode({
    tree,
    node,
    depth,
    isRoot = false,
  }: {
    tree: WorktreeTemplateTree;
    node: WorktreeTemplateTreeNode;
    depth: number;
    isRoot?: boolean;
  }) {
    const key = nodeKey(tree.template.id, node.relPath);
    const open = expandedTemplateNodes.has(key);
    const hasChildren = (node.children ?? []).length > 0;
    const displayName = isRoot ? tree.template.name : node.name;
    return (
      <div>
        <div
          className={cn(
            "flex w-full items-center gap-2 rounded px-2 py-1.5 text-sm hover:bg-accent/70",
            editor?.templateId === tree.template.id && editor.relPath === node.relPath && "bg-secondary"
          )}
          style={{ paddingLeft: 8 + depth * 16 }}
        >
          <button
            type="button"
            className="flex size-4 shrink-0 items-center justify-center"
            onClick={() => node.isDir && toggleTemplateNode(key)}
          >
            {node.isDir && hasChildren ? open ? <ChevronDown className="size-4" /> : <ChevronRight className="size-4" /> : null}
          </button>
          <button
            type="button"
            className="flex min-w-0 flex-1 items-center gap-2 text-left"
            onClick={() => (node.isDir ? toggleTemplateNode(key) : openEditor(tree.template, node))}
            onDoubleClick={() => !node.isDir && openEditor(tree.template, node)}
          >
            {node.isDir ? (
              open ? <FolderOpen className="size-4 shrink-0 text-amber-600" /> : <Folder className="size-4 shrink-0 text-amber-600" />
            ) : (
              <FileText className="size-4 shrink-0 text-muted-foreground" />
            )}
            <span className="truncate font-medium">{displayName}</span>
          </button>
          {isRoot && (
            <div className="ml-auto flex shrink-0 flex-wrap items-center gap-1">
              <Button
                variant={tree.template.enabled ? "secondary" : "outline"}
                size="sm"
                onClick={() => patchAndSave(tree.template, { enabled: !tree.template.enabled })}
                disabled={busy}
              >
                {tree.template.enabled ? t("templates.enabled") : t("templates.disabled")}
              </Button>
              <Button
                variant={tree.template.overwrite ? "secondary" : "outline"}
                size="sm"
                onClick={() => patchAndSave(tree.template, { overwrite: !tree.template.overwrite })}
                disabled={busy}
              >
                {tree.template.overwrite ? t("templates.overwriteOn") : t("templates.overwriteOff")}
              </Button>
              <Button variant="destructive" size="sm" onClick={() => remove(tree.template)} disabled={busy}>
                <Trash2 className="size-4" /> {t("common.delete")}
              </Button>
            </div>
          )}
          {!node.isDir && (
            <Button variant="ghost" size="sm" onClick={() => openEditor(tree.template, node)} disabled={busy}>
              <Edit3 className="size-4" /> {t("templates.openEditor")}
            </Button>
          )}
        </div>
        {node.isDir && open && hasChildren && (
          <div>
            {(node.children ?? []).map((child) => (
              <TemplateFileNode key={child.relPath} tree={tree} node={child} depth={depth + 1} />
            ))}
          </div>
        )}
      </div>
    );
  }

  return (
    <div
      ref={pageRef}
      className="grid h-[calc(100vh-9rem)] grid-cols-[minmax(260px,360px)_1fr] gap-4"
    >
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
              {!isRootSelected && (
                <>
                  <Button size="sm" onClick={importFiles} disabled={busy}>
                    <Upload className="size-4" /> {t("templates.addFiles")}
                  </Button>
                  <Button size="sm" onClick={importFolder} disabled={busy}>
                    <FolderPlus className="size-4" /> {t("templates.addFolder")}
                  </Button>
                </>
              )}
              <Button variant="outline" size="sm" onClick={openTemplateFolder}>
                <FolderOpen className="size-4" /> {t("templates.openFolder")}
              </Button>
              <Button variant="destructive" size="sm" onClick={clearAll} disabled={busy || templates.length === 0}>
                <Trash2 className="size-4" /> {t("templates.clearAll")}
              </Button>
            </div>
          </CardHeader>
        </Card>

        {isRootSelected ? (
          <Card>
            <CardHeader>
              <CardTitle>How to use template folders</CardTitle>
              <CardDescription>Uploads are disabled on the root folder.</CardDescription>
            </CardHeader>
            <CardContent className="space-y-3 text-sm text-muted-foreground">
              <p>Select a destination below Root before uploading templates.</p>
              <ul className="list-disc space-y-1 pl-5">
                <li>Select <span className="font-medium text-foreground">features</span> to copy files to each feature root.</li>
                <li>Select a repository under features to copy files to that worktree repository.</li>
                <li>Select <span className="font-medium text-foreground">agents</span> to copy files to each agent root.</li>
                <li>Select an agent repository to copy files to that sanitized repository folder.</li>
              </ul>
            </CardContent>
          </Card>
        ) : (
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
                onDrop={(e) => e.preventDefault()}
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
        )}

        {(!isRootSelected || visibleTrees.length > 0) && (
          <Card>
            <CardHeader>
              <CardTitle>{t("templates.listTitle")}</CardTitle>
              <CardDescription>{t("templates.listDesc")}</CardDescription>
            </CardHeader>
            <CardContent>
              {visibleTrees.length === 0 ? (
                <p className="text-sm text-muted-foreground">{t("templates.emptyInFolder")}</p>
              ) : (
                <div className="space-y-2 rounded-md border p-2">
                  {visibleTrees.map((tree) => {
                    const effectiveRoot =
                      tree.root.children?.length === 1 ? tree.root.children[0] : tree.root;
                    return (
                      <TemplateFileNode
                        key={tree.template.id}
                        tree={tree}
                        node={effectiveRoot}
                        depth={0}
                        isRoot
                      />
                    );
                  })}
                </div>
              )}
            </CardContent>
          </Card>
        )}

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
                className="min-h-[360px] font-mono text-sm"
              />
              <div className="flex justify-end gap-2">
                <Button variant="outline" onClick={() => setEditor(null)}>
                  {t("common.close")}
                </Button>
                <Button onClick={saveEditor} disabled={busy || editor.content === editor.savedContent}>
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
