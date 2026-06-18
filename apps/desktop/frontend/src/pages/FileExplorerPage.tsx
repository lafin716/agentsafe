import {
  type Dispatch,
  type SetStateAction,
  useCallback,
  useEffect,
  useMemo,
  useState,
} from "react";
import {
  ChevronDown,
  ChevronRight,
  Code2,
  Copy,
  File,
  Folder,
  FolderOpen,
  RefreshCw,
  Save,
  Terminal as TerminalIcon,
  Trash2,
  X,
} from "lucide-react";
import { api, errMessage } from "@/lib/api";
import type { Config, TerminalSession, WorkspaceTreeNode } from "@/lib/types";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Textarea } from "@/components/ui/textarea";
import { useToast } from "@/components/ui/toast";
import { useConfirm } from "@/components/ui/confirm";
import { useI18n } from "@/i18n/I18nProvider";
import { cn } from "@/lib/utils";
import { TerminalPanel } from "@/components/TerminalPanel";

interface Props {
  config: Config | null;
  terminals: TerminalSession[];
  setTerminals: Dispatch<SetStateAction<TerminalSession[]>>;
  activeTab: string;
  setActiveTab: Dispatch<SetStateAction<string>>;
}

type EditorTab = {
  id: string;
  title: string;
  path: string;
  content: string;
  savedContent: string;
};

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

function replaceNode(
  node: WorkspaceTreeNode,
  path: string,
  next: WorkspaceTreeNode
): WorkspaceTreeNode {
  if (node.path === path) return next;
  return {
    ...node,
    children: (node.children ?? []).map((child) => replaceNode(child, path, next)),
  };
}

function editorId(path: string): string {
  return `editor:${path}`;
}

function defaultTerminalProgram(): string {
  try {
    return localStorage.getItem("agentsafe.terminalProgram") || "powershell";
  } catch {
    return "powershell";
  }
}

export function FileExplorerPage({
  config,
  terminals,
  setTerminals,
  activeTab,
  setActiveTab,
}: Props) {
  const { t } = useI18n();
  const { notify } = useToast();
  const confirm = useConfirm();
  const [root, setRoot] = useState<WorkspaceTreeNode | null>(null);
  const [selected, setSelected] = useState<WorkspaceTreeNode | null>(null);
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  const [busy, setBusy] = useState(false);
  const [editors, setEditors] = useState<EditorTab[]>([]);

  const activeEditor = useMemo(
    () => editors.find((tab) => tab.id === activeTab) ?? null,
    [activeTab, editors]
  );
  const activeTerminal = useMemo(
    () => terminals.find((tab) => tab.id === activeTab) ?? null,
    [activeTab, terminals]
  );

  useEffect(() => {
    if (activeTab === "main" || activeEditor || activeTerminal) return;
    setActiveTab("main");
  }, [activeEditor, activeTab, activeTerminal, setActiveTab]);

  const loadRoot = useCallback(async () => {
    if (!config) return;
    try {
      const node = await api.WorkspaceTree("");
      setRoot(node);
      setSelected(node);
      setExpanded(new Set([node.path]));
    } catch (e) {
      notify(errMessage(e), "error");
    }
  }, [config, notify]);

  useEffect(() => {
    void loadRoot();
  }, [loadRoot]);

  if (!config) {
    return (
      <Card className="max-w-3xl">
        <CardHeader>
          <CardTitle>{t("nav.fileExplorer")}</CardTitle>
          <CardDescription>{t("explorer.noWorkspace")}</CardDescription>
        </CardHeader>
      </Card>
    );
  }

  async function expand(node: WorkspaceTreeNode) {
    if (!node.isDir || !root) return;
    const nextExpanded = new Set(expanded);
    if (expanded.has(node.path)) {
      nextExpanded.delete(node.path);
      setExpanded(nextExpanded);
      return;
    }
    try {
      const loaded = await api.WorkspaceTree(node.path);
      setRoot(replaceNode(root, node.path, loaded));
      setSelected(loaded);
      nextExpanded.add(node.path);
      setExpanded(nextExpanded);
    } catch (e) {
      notify(errMessage(e), "error");
    }
  }

  async function openSelected() {
    if (!selected) return;
    try {
      const path = await api.OpenPath(selected.path);
      notify(t("toast.openedPath", { path }), "success");
    } catch (e) {
      notify(errMessage(e), "error");
    }
  }

  async function openVSCode() {
    if (!selected) return;
    try {
      const path = await api.OpenPathVSCode(selected.path);
      notify(t("toast.openedPath", { path }), "success");
    } catch (e) {
      notify(errMessage(e), "error");
    }
  }

  async function copyPath() {
    if (!selected) return;
    try {
      await api.CopyText(selected.path);
      notify(t("toast.copiedPath"), "success");
    } catch (e) {
      notify(errMessage(e), "error");
    }
  }

  async function removeSelected() {
    if (!selected) return;
    if (
      !(await confirm({
        message: t("explorer.deleteConfirm", { path: selected.relPath || selected.path }),
        danger: true,
      }))
    )
      return;
    try {
      setBusy(true);
      await api.DeleteWorkspacePath(selected.path);
      setEditors((prev) => prev.filter((tab) => tab.path !== selected.path));
      if (activeTab === editorId(selected.path)) setActiveTab("main");
      notify(t("toast.pathDeleted"), "success");
      await loadRoot();
    } catch (e) {
      notify(errMessage(e), "error");
    } finally {
      setBusy(false);
    }
  }

  async function openEditor(node: WorkspaceTreeNode) {
    if (node.isDir) return;
    const id = editorId(node.path);
    if (editors.some((tab) => tab.id === id)) {
      setActiveTab(id);
      return;
    }
    try {
      setBusy(true);
      const content = await api.ReadWorkspaceFile(node.path);
      setEditors((prev) => [
        ...prev,
        { id, title: node.name, path: node.path, content, savedContent: content },
      ]);
      setActiveTab(id);
    } catch (e) {
      notify(errMessage(e), "error");
    } finally {
      setBusy(false);
    }
  }

  async function saveEditor(tab: EditorTab) {
    try {
      setBusy(true);
      await api.SaveWorkspaceFile(tab.path, tab.content);
      setEditors((prev) =>
        prev.map((item) =>
          item.id === tab.id ? { ...item, savedContent: tab.content } : item
        )
      );
      if (selected?.path === tab.path) {
        const refreshed = await api.WorkspaceTree(tab.path);
        setSelected(refreshed);
      }
      notify(t("explorer.fileSaved"), "success");
    } catch (e) {
      notify(errMessage(e), "error");
    } finally {
      setBusy(false);
    }
  }

  async function closeEditor(tab: EditorTab) {
    if (
      tab.content !== tab.savedContent &&
      !(await confirm({ message: t("explorer.unsavedConfirm", { name: tab.title }) }))
    ) {
      return;
    }
    setEditors((prev) => prev.filter((item) => item.id !== tab.id));
    setActiveTab((prev) => (prev === tab.id ? "main" : prev));
  }

  async function openEmbeddedTerminal() {
    if (!selected) return;
    try {
      setBusy(true);
      const session = await api.TerminalOpenWithProgram(
        selected.path,
        defaultTerminalProgram()
      );
      if (session.external) {
        notify(t("toast.openedPath", { path: session.path }), "success");
        return;
      }
      setTerminals((prev) => {
        if (prev.some((tab) => tab.id === session.id)) return prev;
        return [...prev, session];
      });
      setActiveTab(session.id);
      notify(t("toast.openedEmbeddedTerminal", { path: session.path }), "success");
    } catch (e) {
      notify(errMessage(e), "error");
    } finally {
      setBusy(false);
    }
  }

  async function closeTerminal(id: string) {
    try {
      await api.TerminalClose(id);
    } catch {
      /* terminal may already be closed */
    }
    setTerminals((prev) => prev.filter((tab) => tab.id !== id));
    setActiveTab((prev) => (prev === id ? "main" : prev));
  }

  function renameTerminal(id: string) {
    const tab = terminals.find((item) => item.id === id);
    if (!tab) return;
    const next = window.prompt(t("explorer.terminalRenamePrompt"), tab.title);
    if (!next?.trim()) return;
    setTerminals((prev) =>
      prev.map((item) => (item.id === id ? { ...item, title: next.trim() } : item))
    );
  }

  function TreeItem({ node, depth }: { node: WorkspaceTreeNode; depth: number }) {
    const open = expanded.has(node.path);
    const active = selected?.path === node.path;
    return (
      <div>
        <button
          type="button"
          className={cn(
            "flex w-full items-center gap-1 rounded px-2 py-1 text-left text-sm hover:bg-accent",
            active && "bg-secondary font-medium text-secondary-foreground"
          )}
          style={{ paddingLeft: 8 + depth * 16 }}
          onClick={() => setSelected(node)}
          onDoubleClick={() => {
            if (node.isDir) void expand(node);
            else void openEditor(node);
          }}
        >
          {node.isDir ? (
            <span
              className="flex size-4 items-center justify-center"
              onClick={(e) => {
                e.stopPropagation();
                void expand(node);
              }}
            >
              {open ? <ChevronDown className="size-4" /> : <ChevronRight className="size-4" />}
            </span>
          ) : (
            <span className="size-4" />
          )}
          {node.isDir ? (
            open ? <FolderOpen className="size-4 text-amber-600" /> : <Folder className="size-4 text-amber-600" />
          ) : (
            <File className="size-4 text-muted-foreground" />
          )}
          <span className="truncate">{node.name}</span>
          {node.branch && (
            <span
              className="ml-auto max-w-32 truncate rounded bg-secondary px-1.5 py-0.5 text-[10px] text-secondary-foreground"
              title={node.featureName ? `${node.featureName}: ${node.branch}` : node.branch}
            >
              {node.branch}
            </span>
          )}
        </button>
        {node.isDir && open && (
          <div>
            {(node.children ?? []).map((child) => (
              <TreeItem key={child.path} node={child} depth={depth + 1} />
            ))}
          </div>
        )}
      </div>
    );
  }

  return (
    <div className="grid h-[calc(100vh-9rem)] grid-cols-[minmax(260px,360px)_1fr] gap-4">
      <Card className="overflow-hidden">
        <CardHeader className="flex-row items-center justify-between space-y-0">
          <div>
            <CardTitle>{t("explorer.title")}</CardTitle>
            <CardDescription>{t("explorer.desc")}</CardDescription>
          </div>
          <Button variant="outline" size="sm" onClick={loadRoot}>
            <RefreshCw className="size-4" />
          </Button>
        </CardHeader>
        <CardContent className="h-full overflow-auto pb-24">
          {root ? <TreeItem node={root} depth={0} /> : null}
        </CardContent>
      </Card>

      <Card className="min-w-0 overflow-hidden">
        <div className="flex items-center gap-1 overflow-x-auto border-b bg-muted/30 px-3 pt-2">
          <button
            type="button"
            className={cn(
              "rounded-t-md border border-b-0 px-3 py-1.5 text-sm",
              activeTab === "main" ? "bg-background font-medium" : "bg-transparent text-muted-foreground"
            )}
            onClick={() => setActiveTab("main")}
          >
            {t("explorer.mainTab")}
          </button>
          {editors.map((tab) => {
            const dirty = tab.content !== tab.savedContent;
            return (
              <div
                key={tab.id}
                className={cn(
                  "group flex max-w-56 items-center gap-2 rounded-t-md border border-b-0 px-3 py-1.5 text-sm",
                  activeTab === tab.id ? "bg-background font-medium" : "bg-transparent text-muted-foreground"
                )}
                title={tab.path}
              >
                <button
                  type="button"
                  className="flex min-w-0 flex-1 items-center gap-2"
                  onClick={() => setActiveTab(tab.id)}
                >
                  <File className="size-3.5 shrink-0" />
                  <span className="truncate">{dirty ? "*" : ""}{tab.title}</span>
                </button>
                <button
                  type="button"
                  className="rounded p-0.5 opacity-60 hover:bg-accent hover:opacity-100"
                  onClick={() => void closeEditor(tab)}
                  title={t("common.close")}
                >
                  <X className="size-3.5" />
                </button>
              </div>
            );
          })}
          {terminals.map((tab) => (
            <div
              key={tab.id}
              className={cn(
                "group flex max-w-56 items-center gap-2 rounded-t-md border border-b-0 px-3 py-1.5 text-sm",
                activeTab === tab.id ? "bg-background font-medium" : "bg-transparent text-muted-foreground"
              )}
              onContextMenu={(e) => {
                e.preventDefault();
                renameTerminal(tab.id);
              }}
              title={tab.path}
            >
              <button
                type="button"
                className="flex min-w-0 flex-1 items-center gap-2"
                onClick={() => setActiveTab(tab.id)}
              >
                <TerminalIcon className="size-3.5 shrink-0" />
                <span className="truncate">{tab.title}</span>
              </button>
              <button
                type="button"
                className="rounded p-0.5 opacity-60 hover:bg-accent hover:opacity-100"
                onClick={() => void closeTerminal(tab.id)}
                title={t("common.close")}
              >
                <X className="size-3.5" />
              </button>
            </div>
          ))}
        </div>
        {activeTab === "main" ? (
          <>
            <CardHeader>
              <CardTitle>{selected?.name ?? t("explorer.noneSelected")}</CardTitle>
              <CardDescription className="break-all font-mono">
                {selected?.path ?? t("explorer.selectHint")}
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              {selected && (
                <>
                  <div className="grid gap-2 rounded-md border p-3 text-sm md:grid-cols-2">
                    <div>
                      <span className="text-muted-foreground">{t("explorer.type")}:</span>{" "}
                      {selected.isDir ? t("explorer.folder") : t("explorer.file")}
                    </div>
                    <div>
                      <span className="text-muted-foreground">{t("explorer.size")}:</span>{" "}
                      {selected.isDir ? "—" : formatSize(selected.size)}
                    </div>
                    <div className="md:col-span-2">
                      <span className="text-muted-foreground">{t("explorer.modified")}:</span>{" "}
                      {formatDate(selected.modTime)}
                    </div>
                  </div>
                  <div className="flex flex-wrap gap-2">
                    <Button variant="outline" onClick={openSelected} disabled={busy}>
                      <FolderOpen className="size-4" /> {t("explorer.open")}
                    </Button>
                    <Button variant="outline" onClick={openVSCode} disabled={busy}>
                      <Code2 className="size-4" /> {t("explorer.openVSCode")}
                    </Button>
                    {!selected.isDir && (
                      <Button variant="outline" onClick={() => openEditor(selected)} disabled={busy}>
                        <File className="size-4" /> {t("explorer.openEditor")}
                      </Button>
                    )}
                    <Button variant="outline" onClick={openEmbeddedTerminal} disabled={busy}>
                      <TerminalIcon className="size-4" /> {t("explorer.openTerminal")}
                    </Button>
                    <Button variant="outline" onClick={copyPath} disabled={busy}>
                      <Copy className="size-4" /> {t("feature.copyPath")}
                    </Button>
                    <Button variant="destructive" onClick={removeSelected} disabled={busy}>
                      <Trash2 className="size-4" /> {t("common.delete")}
                    </Button>
                  </div>
                </>
              )}
            </CardContent>
          </>
        ) : activeEditor ? (
          <div className="flex h-[calc(100vh-12rem)] flex-col">
            <div className="flex items-center justify-between gap-3 border-b px-3 py-2">
              <div className="min-w-0">
                <div className="truncate text-sm font-medium">{activeEditor.title}</div>
                <div className="truncate font-mono text-xs text-muted-foreground" title={activeEditor.path}>
                  {activeEditor.path}
                </div>
              </div>
              <Button size="sm" onClick={() => void saveEditor(activeEditor)} disabled={busy || activeEditor.content === activeEditor.savedContent}>
                <Save className="size-4" /> {t("common.save")}
              </Button>
            </div>
            <Textarea
              value={activeEditor.content}
              onChange={(e) =>
                setEditors((prev) =>
                  prev.map((tab) =>
                    tab.id === activeEditor.id ? { ...tab, content: e.target.value } : tab
                  )
                )
              }
              className="min-h-0 flex-1 resize-none rounded-none border-0 font-mono text-sm focus-visible:ring-0"
            />
          </div>
        ) : activeTerminal ? (
          <TerminalPanel id={activeTerminal.id} path={activeTerminal.path} />
        ) : null}
      </Card>
    </div>
  );
}
