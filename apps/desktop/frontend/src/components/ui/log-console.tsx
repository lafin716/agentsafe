import * as React from "react";
import {
  AlertCircle,
  CheckCircle2,
  Copy,
  Loader2,
  ScrollText,
  Trash2,
  X,
} from "lucide-react";
import { cn } from "@/lib/utils";
import { useI18n } from "@/i18n/I18nProvider";

// A persistent, app-wide console that records the full output of every task
// (prepare / diff / sync / pull / …) streamed from the backend via the
// task:start / task:log / task:end events. Unlike the transient TaskProgress
// toast, it keeps history so the user can review verbose logs at any time.

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

type LogConsoleContextValue = {
  entries: LogEntry[];
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

export function LogConsoleProvider({ children }: { children: React.ReactNode }) {
  const [entries, setEntries] = React.useState<LogEntry[]>([]);
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

    return () => {
      offStart();
      offLog();
      offEnd();
    };
  }, []);

  const value = React.useMemo<LogConsoleContextValue>(
    () => ({
      entries,
      open,
      setOpen,
      toggle: () => setOpen((v) => !v),
      clear: () => setEntries([]),
      runningCount: entries.reduce((n, e) => (e.status === "running" ? n + 1 : n), 0),
    }),
    [entries, open]
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

export function LogConsoleWindow() {
  const { t } = useI18n();
  const { entries, open, setOpen, clear } = useLogConsole();
  const [selectedId, setSelectedId] = React.useState<number | null>(null);
  const [copied, setCopied] = React.useState(false);
  const logRef = React.useRef<HTMLPreElement | null>(null);

  const selected = React.useMemo(() => {
    if (entries.length === 0) return null;
    const byId =
      selectedId != null ? entries.find((e) => e.id === selectedId) : undefined;
    return byId ?? entries[entries.length - 1];
  }, [entries, selectedId]);

  const detail = selected
    ? [
        selected.log.trimEnd(),
        selected.error && !selected.log.includes(selected.error) ? selected.error : "",
      ]
        .filter(Boolean)
        .join("\n\n")
    : "";

  React.useEffect(() => {
    if (open && logRef.current) logRef.current.scrollTop = logRef.current.scrollHeight;
  }, [detail, open]);

  React.useEffect(() => {
    if (!open) return;
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") setOpen(false);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open, setOpen]);

  if (!open) return null;

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(detail);
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch {
      /* clipboard unavailable */
    }
  };

  return (
    <div
      className="fixed inset-0 z-[80] flex items-center justify-center bg-black/55 p-4"
      onClick={() => setOpen(false)}
    >
      <div
        className="flex h-[80vh] w-full max-w-5xl flex-col overflow-hidden rounded-lg border bg-card shadow-2xl"
        onClick={(event) => event.stopPropagation()}
      >
        <div className="flex items-center gap-3 border-b px-5 py-3">
          <ScrollText className="size-4 text-primary" />
          <h2 className="flex-1 text-sm font-semibold">{t("logs.title")}</h2>
          <button
            type="button"
            onClick={clear}
            disabled={entries.length === 0}
            className="flex items-center gap-1 rounded-md px-2 py-1 text-xs text-muted-foreground hover:bg-muted hover:text-foreground disabled:opacity-40"
            title={t("logs.clear")}
          >
            <Trash2 className="size-3.5" />
            {t("logs.clear")}
          </button>
          <button
            type="button"
            onClick={() => setOpen(false)}
            className="rounded-md p-1 text-muted-foreground hover:bg-muted hover:text-foreground"
            title={t("common.close")}
          >
            <X className="size-5" />
          </button>
        </div>

        {entries.length === 0 ? (
          <div className="flex flex-1 items-center justify-center text-sm text-muted-foreground">
            {t("logs.empty")}
          </div>
        ) : (
          <div className="flex min-h-0 flex-1">
            <ul className="w-72 shrink-0 overflow-auto border-r">
              {[...entries].reverse().map((entry) => (
                <li key={entry.id}>
                  <button
                    type="button"
                    onClick={() => setSelectedId(entry.id)}
                    className={cn(
                      "flex w-full items-center gap-2 border-b px-3 py-2 text-left text-xs hover:bg-muted/60",
                      selected?.id === entry.id && "bg-muted"
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
                <span className="min-w-0 flex-1 truncate text-xs font-medium">
                  {selected?.label}
                </span>
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
                {detail || t("task.empty")}
              </pre>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
