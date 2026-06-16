import { useCallback, useEffect, useState } from "react";
import {
  ChevronDown,
  ChevronRight,
  Code2,
  Copy,
  File,
  Folder,
  FolderOpen,
  RefreshCw,
  Trash2,
} from "lucide-react";
import { api, errMessage } from "@/lib/api";
import type { Config, WorkspaceTreeNode } from "@/lib/types";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { useToast } from "@/components/ui/toast";
import { useConfirm } from "@/components/ui/confirm";
import { useI18n } from "@/i18n/I18nProvider";
import { cn } from "@/lib/utils";

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

export function FileExplorerPage({ config }: Props) {
  const { t } = useI18n();
  const { notify } = useToast();
  const confirm = useConfirm();
  const [root, setRoot] = useState<WorkspaceTreeNode | null>(null);
  const [selected, setSelected] = useState<WorkspaceTreeNode | null>(null);
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  const [busy, setBusy] = useState(false);

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
      notify(t("toast.pathDeleted"), "success");
      await loadRoot();
    } catch (e) {
      notify(errMessage(e), "error");
    } finally {
      setBusy(false);
    }
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
          onDoubleClick={() => expand(node)}
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

      <Card>
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
      </Card>
    </div>
  );
}
