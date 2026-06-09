import { useCallback, useEffect, useRef, useState } from "react";
import {
  Check,
  ChevronsUpDown,
  FolderOpen,
  Sparkles,
  Trash2,
} from "lucide-react";
import { api, errMessage } from "@/lib/api";
import type { Config, WorkspaceEntry } from "@/lib/types";
import { cn } from "@/lib/utils";
import { useToast } from "@/components/ui/toast";
import { useConfirm } from "@/components/ui/confirm";
import { useI18n } from "@/i18n/I18nProvider";

interface Props {
  config: Config | null;
  // Called after a workspace is opened/switched/initialized.
  onSwitched: (cfg: Config) => void;
  // Called when the active workspace was removed from the registry.
  onRemovedActive: () => void;
}

export function WorkspaceSwitcher({
  config,
  onSwitched,
  onRemovedActive,
}: Props) {
  const { notify } = useToast();
  const confirm = useConfirm();
  const { t } = useI18n();
  const [open, setOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const [workspaces, setWorkspaces] = useState<WorkspaceEntry[]>([]);
  const ref = useRef<HTMLDivElement>(null);

  const activeRoot = config?.Workspace.Root ?? "";

  const refresh = useCallback(async () => {
    try {
      const list = await api.ListWorkspaces();
      setWorkspaces(list ?? []);
    } catch {
      setWorkspaces([]);
    }
  }, []);

  useEffect(() => {
    refresh();
  }, [refresh, config]);

  // Close the dropdown on outside click.
  useEffect(() => {
    if (!open) return;
    function onClick(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        setOpen(false);
      }
    }
    document.addEventListener("mousedown", onClick);
    return () => document.removeEventListener("mousedown", onClick);
  }, [open]);

  async function switchTo(path: string) {
    if (path === activeRoot) {
      setOpen(false);
      return;
    }
    try {
      setBusy(true);
      const cfg = await api.OpenWorkspace(path);
      onSwitched(cfg);
      notify(t("toast.openedWorkspace", { name: cfg.Workspace.Name }), "success");
      setOpen(false);
    } catch (e) {
      notify(errMessage(e), "error");
    } finally {
      setBusy(false);
    }
  }

  async function openFolder() {
    try {
      setBusy(true);
      const dir = await api.SelectWorkspaceDir();
      if (!dir) return;
      const cfg = await api.OpenWorkspace(dir);
      onSwitched(cfg);
      notify(t("toast.openedWorkspace", { name: cfg.Workspace.Name }), "success");
      setOpen(false);
      await refresh();
    } catch (e) {
      notify(errMessage(e), "error");
    } finally {
      setBusy(false);
    }
  }

  async function initNew() {
    try {
      setBusy(true);
      const dir = await api.SelectWorkspaceDir();
      if (!dir) return;
      const name = window.prompt(t("switcher.promptName")) ?? "";
      const cfg = await api.InitWorkspace(dir, name.trim());
      onSwitched(cfg);
      notify(
        t("toast.initializedWorkspace", { name: cfg.Workspace.Name }),
        "success"
      );
      setOpen(false);
      await refresh();
    } catch (e) {
      notify(errMessage(e), "error");
    } finally {
      setBusy(false);
    }
  }

  async function remove(e: React.MouseEvent, ws: WorkspaceEntry) {
    e.stopPropagation();
    if (
      !(await confirm({
        message: t("switcher.confirmRemove", { name: ws.name }),
        danger: true,
      }))
    ) {
      return;
    }
    try {
      setBusy(true);
      const wasActive = ws.path === activeRoot;
      await api.RemoveWorkspace(ws.path);
      await refresh();
      notify(t("toast.workspaceRemoved"), "success");
      if (wasActive) onRemovedActive();
    } catch (err) {
      notify(errMessage(err), "error");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="relative px-2" ref={ref}>
      <button
        onClick={() => setOpen((v) => !v)}
        className="flex w-full items-center justify-between gap-2 rounded-md border bg-background px-3 py-2 text-left text-sm hover:bg-accent"
      >
        <span className="min-w-0">
          <span className="block truncate font-medium text-foreground">
            {config ? config.Workspace.Name : t("switcher.select")}
          </span>
          {config && (
            <span className="block truncate text-xs text-muted-foreground">
              {config.Workspace.Root}
            </span>
          )}
        </span>
        <ChevronsUpDown className="size-4 shrink-0 text-muted-foreground" />
      </button>

      {open && (
        <div className="absolute left-2 right-2 z-20 mt-1 overflow-hidden rounded-md border bg-card shadow-md">
          <div className="max-h-64 overflow-auto py-1">
            {workspaces.length === 0 ? (
              <div className="px-3 py-2 text-xs text-muted-foreground">
                {t("switcher.empty")}
              </div>
            ) : (
              workspaces.map((ws) => {
                const active = ws.path === activeRoot;
                return (
                  <div
                    key={ws.path}
                    onClick={() => switchTo(ws.path)}
                    className={cn(
                      "group flex cursor-pointer items-center gap-2 px-3 py-2 text-sm hover:bg-accent",
                      active && "bg-secondary/60"
                    )}
                  >
                    <Check
                      className={cn(
                        "size-4 shrink-0",
                        active ? "opacity-100" : "opacity-0"
                      )}
                    />
                    <span className="min-w-0 flex-1">
                      <span className="block truncate font-medium">
                        {ws.name}
                      </span>
                      <span className="block truncate text-xs text-muted-foreground">
                        {ws.path}
                      </span>
                    </span>
                    <button
                      onClick={(e) => remove(e, ws)}
                      disabled={busy}
                      title={t("switcher.removeTitle")}
                      className="shrink-0 rounded p-1 text-muted-foreground opacity-0 hover:bg-destructive/10 hover:text-destructive group-hover:opacity-100"
                    >
                      <Trash2 className="size-4" />
                    </button>
                  </div>
                );
              })
            )}
          </div>
          <div className="border-t p-1">
            <button
              onClick={openFolder}
              disabled={busy}
              className="flex w-full items-center gap-2 rounded-md px-3 py-2 text-sm text-muted-foreground hover:bg-accent hover:text-accent-foreground"
            >
              <FolderOpen className="size-4" /> {t("switcher.openFolder")}
            </button>
            <button
              onClick={initNew}
              disabled={busy}
              className="flex w-full items-center gap-2 rounded-md px-3 py-2 text-sm text-muted-foreground hover:bg-accent hover:text-accent-foreground"
            >
              <Sparkles className="size-4" /> {t("switcher.initNew")}
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
