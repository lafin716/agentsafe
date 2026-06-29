import * as React from "react";
import {
  AlertCircle,
  CheckCircle2,
  Copy,
  FileText,
  FolderOpen,
  Loader2,
  ScrollText,
  Trash2,
  X,
} from "lucide-react";
import { cn } from "@/lib/utils";
import { api } from "@/lib/api";
import { useI18n } from "@/i18n/I18nProvider";

// A persistent, app-wide console. It records the full output of every task
// (prepare / diff / sync / pull / …) streamed via task:start / task:log /
// task:end, AND the app's own program log (startup, terminals, events, errors)
// streamed via log:entry from the applog tap. Unlike the transient TaskProgress
// toast, it keeps history so the user can review verbose logs at any time. The
// full, persistent log file lives on disk; the header buttons open it.

type TaskStatus = "running" | "done" | "error";

export type LogEntry = {
  id: number;
  label: string;
  status: TaskStatus;
  error: string;
  log: string;
  startedAt: number;
  endedAt: number | null;
};

// AppLogRecord mirrors internal/applog.Entry as emitted over the log:entry event.
type AppLogRecord = {
  time?: string;
  level?: string;
  msg?: string;
  attrs?: Record<string, unknown>;
};

type LogConsoleContextValue = {
  entries: LogEntry[];
  appLog: string;
  open: boolean;
  setOpen: (value: boolean) => void;
  toggle: () => void;
  clear: () => void;
  runningCount: number;
};

const LogConsoleContext = React.createContext<LogConsoleContextValue | null>(null);

export function useLogConsole(): LogConsoleContextValue {
  const ctx = React.useContext(LogConsoleContext);
  if (!ctx) throw new Error("useLogConsole must be used within LogConsoleProvider");
  return ctx;
}

type WailsRuntime = {
  EventsOn: (event: string, cb: (...data: unknown[]) => void) => () => void;
};

function runtime(): WailsRuntime | null {
  const rt = (window as unknown as { runtime?: WailsRuntime }).runtime;
  return rt && typeof rt.EventsOn === "function" ? rt : null;
}

const MAX_ENTRIES = 200;
const MAX_APP_LINES = 1000;

const APP_KEY = "app";

function formatAppLine(e: AppLogRecord): string {
  // RFC3339 "2006-01-02T15:04:05.000Z07:00" -> HH:MM:SS.mmm
  const time = e.time ? e.time.slice(11, 23) : "";
  const level = (e.level ?? "info").toUpperCase().padEnd(5);
  const attrs =
    e.attrs && Object.keys(e.attrs).length > 0 ? " " + JSON.stringify(e.attrs) : "";
  return `${time} ${level} ${e.msg ?? ""}${attrs}`;
}

export function LogConsoleProvider({ children }: { children: React.ReactNode }) {
  const [entries, setEntries] = React.useState<LogEntry[]>([]);
  const [appLogLines, setAppLogLines] = React.useState<string[]>([]);
  const [open, setOpen] = React.useState(false);

  React.useEffect(() => {
    const rt = runtime();
    if (!rt) return;

    const offStart = rt.EventsOn("task:start", (...data: unknown[]) => {
      const p = data[0] as { id: number; label: string; startedAt?: number };
      setEntries((prev) => {
        const next = [
          ...prev,
          {
            id: p.id,
            label: p.label,
            status: "running" as TaskStatus,
            error: "",
            log: "",
            startedAt: p.startedAt ?? Date.now(),
            endedAt: null,
          },
        ];
        return next.length > MAX_ENTRIES ? next.slice(next.length - MAX_ENTRIES) : next;
      });
    });

    const offLog = rt.EventsOn("task:log", (...data: unknown[]) => {
      const p = data[0] as { id: number; chunk: string };
      setEntries((prev) =>
        prev.map((e) => (e.id === p.id ? { ...e, log: e.log + p.chunk } : e))
      );
    });

    const offEnd = rt.EventsOn("task:end", (...data: unknown[]) => {
      const p = data[0] as { id: number; status: TaskStatus; error: string };
      setEntries((prev) =>
        prev.map((e) =>
          e.id === p.id
            ? { ...e, status: p.status, error: p.error ?? "", endedAt: Date.now() }
            : e
        )
      );
    });

    const offEntry = rt.EventsOn("log:entry", (...data: unknown[]) => {
      const e = data[0] as AppLogRecord;
      setAppLogLines((prev) => {
        const next = [...prev, formatAppLine(e)];
        return next.length > MAX_APP_LINES ? next.slice(next.length - MAX_APP_LINES) : next;
      });
    });

    return () => {
      offStart();
      offLog();
      offEnd();
      offEntry();
    };
  }, []);

  const value = React.useMemo<LogConsoleContextValue>(
    () => ({
      entries,
      appLog: appLogLines.join("\n"),
      open,
      setOpen,
      toggle: () => setOpen((v) => !v),
      clear: () => {
        setEntries([]);
        setAppLogLines([]);
      },
      runningCount: entries.reduce((n, e) => (e.status === "running" ? n + 1 : n), 0),
    }),
    [entries, appLogLines, open]
  );

  return (
    <LogConsoleContext.Provider value={value}>{children}</LogConsoleContext.Provider>
  );
}

function StatusIcon({ status }: { status: TaskStatus }) {
  if (status === "running")
    return <Loader2 className="size-3.5 shrink-0 animate-spin text-primary" />;
  if (status === "done")
    return <CheckCircle2 className="size-3.5 shrink-0 text-emerald-600" />;
  return <AlertCircle className="size-3.5 shrink-0 text-destructive" />;
}

function formatDuration(entry: LogEntry): string {
  const end = entry.endedAt ?? Date.now();
  const ms = Math.max(0, end - entry.startedAt);
  return ms < 1000 ? `${ms}ms` : `${(ms / 1000).toFixed(1)}s`;
}

// LogConsoleButton is the header trigger; it shows a spinner while any task runs.
export function LogConsoleButton() {
  const { t } = useI18n();
  const { toggle, runningCount } = useLogConsole();
  return (
    <button
      type="button"
      onClick={toggle}
      title={t("logs.title")}
      aria-label={t("logs.title")}
      className="relative inline-flex h-7 items-center gap-1.5 rounded-md border bg-muted px-2.5 text-xs text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground"
    >
      <ScrollText className="size-3.5" />
      <span>{t("logs.title")}</span>
      {runningCount > 0 && <Loader2 className="size-3 animate-spin text-primary" />}
    </button>
  );
}

// LogConsolePanel is the reusable, frameless log console body (task list + detail
// pane). It is rendered inside the in-app modal (LogConsoleWindow). `headerActions`
// lets the host inject its own buttons (close) into the header.
export function LogConsolePanel({
  headerActions,
  className,
}: {
  headerActions?: React.ReactNode;
  className?: string;
}) {
  const { t } = useI18n();
  const { entries, appLog, clear } = useLogConsole();
  const [selectedKey, setSelectedKey] = React.useState<string | null>(null);
  const [copied, setCopied] = React.useState(false);
  const logRef = React.useRef<HTMLPreElement | null>(null);

  // Default selection: the most recent task, or the app log when no tasks ran.
  const effectiveKey =
    selectedKey ??
    (entries.length > 0 ? `t-${entries[entries.length - 1].id}` : APP_KEY);
  const isApp = effectiveKey === APP_KEY;

  const selectedTask = React.useMemo(() => {
    if (isApp) return null;
    const id = Number(effectiveKey.slice(2));
    return entries.find((e) => e.id === id) ?? null;
  }, [entries, effectiveKey, isApp]);

  const title = isApp ? t("logs.appLog") : selectedTask?.label ?? "";
  const detail = isApp
    ? appLog
    : selectedTask
      ? [
          selectedTask.log.trimEnd(),
          selectedTask.error && !selectedTask.log.includes(selectedTask.error)
            ? selectedTask.error
            : "",
        ]
          .filter(Boolean)
          .join("\n\n")
      : "";

  React.useEffect(() => {
    if (logRef.current) logRef.current.scrollTop = logRef.current.scrollHeight;
  }, [detail]);

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(detail);
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch {
      /* clipboard unavailable */
    }
  };

  const openFile = () => {
    void api.OpenLogFile().catch(() => {});
  };
  const openFolder = () => {
    void api.OpenLogFolder().catch(() => {});
  };

  return (
    <div className={cn("flex flex-col overflow-hidden bg-card", className)}>
      <div className="flex items-center gap-3 border-b px-5 py-3">
        <ScrollText className="size-4 text-primary" />
        <h2 className="flex-1 text-sm font-semibold">{t("logs.title")}</h2>
        <button
          type="button"
          onClick={openFile}
          className="flex items-center gap-1 rounded-md px-2 py-1 text-xs text-muted-foreground hover:bg-muted hover:text-foreground"
          title={t("logs.openFile")}
        >
          <FileText className="size-3.5" />
          {t("logs.openFile")}
        </button>
        <button
          type="button"
          onClick={openFolder}
          className="flex items-center gap-1 rounded-md px-2 py-1 text-xs text-muted-foreground hover:bg-muted hover:text-foreground"
          title={t("logs.openFolder")}
        >
          <FolderOpen className="size-3.5" />
          {t("logs.openFolder")}
        </button>
        <button
          type="button"
          onClick={clear}
          disabled={entries.length === 0 && appLog === ""}
          className="flex items-center gap-1 rounded-md px-2 py-1 text-xs text-muted-foreground hover:bg-muted hover:text-foreground disabled:opacity-40"
          title={t("logs.clear")}
        >
          <Trash2 className="size-3.5" />
          {t("logs.clear")}
        </button>
        {headerActions}
      </div>

      <div className="flex min-h-0 flex-1">
        <ul className="w-72 shrink-0 overflow-auto border-r">
          <li>
            <button
              type="button"
              onClick={() => setSelectedKey(APP_KEY)}
              className={cn(
                "flex w-full items-center gap-2 border-b px-3 py-2 text-left text-xs hover:bg-muted/60",
                isApp && "bg-muted"
              )}
            >
              <ScrollText className="size-3.5 shrink-0 text-primary" />
              <span className="min-w-0 flex-1 truncate font-medium">
                {t("logs.appLog")}
              </span>
            </button>
          </li>
          {[...entries].reverse().map((entry) => (
            <li key={entry.id}>
              <button
                type="button"
                onClick={() => setSelectedKey(`t-${entry.id}`)}
                className={cn(
                  "flex w-full items-center gap-2 border-b px-3 py-2 text-left text-xs hover:bg-muted/60",
                  !isApp && selectedTask?.id === entry.id && "bg-muted"
                )}
              >
                <StatusIcon status={entry.status} />
                <span className="min-w-0 flex-1 truncate">{entry.label}</span>
                <span className="shrink-0 text-[10px] text-muted-foreground">
                  {formatDuration(entry)}
                </span>
              </button>
            </li>
          ))}
        </ul>
        <div className="flex min-w-0 flex-1 flex-col">
          <div className="flex items-center gap-2 border-b px-4 py-2">
            <span className="min-w-0 flex-1 truncate text-xs font-medium">{title}</span>
            <button
              type="button"
              onClick={copy}
              className="flex items-center gap-1 rounded-md px-2 py-1 text-xs text-muted-foreground hover:bg-muted hover:text-foreground"
              title={t("logs.copy")}
            >
              <Copy className="size-3.5" />
              {copied ? t("logs.copied") : t("logs.copy")}
            </button>
          </div>
          <pre
            ref={logRef}
            className="min-h-0 flex-1 overflow-auto whitespace-pre-wrap break-words bg-muted/30 p-4 text-xs leading-relaxed"
          >
            {detail || t("logs.empty")}
          </pre>
        </div>
      </div>
    </div>
  );
}

export function LogConsoleWindow() {
  const { t } = useI18n();
  const { open, setOpen } = useLogConsole();

  React.useEffect(() => {
    if (!open) return;
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") setOpen(false);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open, setOpen]);

  if (!open) return null;

  return (
    <div
      className="fixed inset-0 z-[80] flex items-center justify-center bg-black/55 p-4"
      onClick={() => setOpen(false)}
    >
      <div
        className="flex h-[80vh] w-full max-w-5xl"
        onClick={(event) => event.stopPropagation()}
      >
        <LogConsolePanel
          className="h-full w-full rounded-lg border shadow-2xl"
          headerActions={
            <button
              type="button"
              onClick={() => setOpen(false)}
              className="rounded-md p-1 text-muted-foreground hover:bg-muted hover:text-foreground"
              title={t("common.close")}
            >
              <X className="size-5" />
            </button>
          }
        />
      </div>
    </div>
  );
}
