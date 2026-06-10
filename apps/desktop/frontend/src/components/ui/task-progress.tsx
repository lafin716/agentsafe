import * as React from "react";
import { ChevronDown, ChevronUp, Loader2, CheckCircle2, AlertCircle, X } from "lucide-react";
import { cn } from "@/lib/utils";
import { useI18n } from "@/i18n/I18nProvider";

type TaskStatus = "running" | "done" | "error";

type Task = {
  id: number;
  label: string;
  status: TaskStatus;
  error: string;
};

// Minimal shape of the Wails-injected runtime event API. Guarded so the app
// still renders in a plain browser preview where window.runtime is absent.
type WailsRuntime = {
  EventsOn: (event: string, cb: (...data: unknown[]) => void) => () => void;
};

function runtime(): WailsRuntime | null {
  const rt = (window as unknown as { runtime?: WailsRuntime }).runtime;
  return rt && typeof rt.EventsOn === "function" ? rt : null;
}

// lastLine returns the last non-empty line of the accumulated log, used as the
// collapsed "current step" text.
function lastLine(log: string): string {
  const lines = log.split("\n");
  for (let i = lines.length - 1; i >= 0; i--) {
    if (lines[i].trim() !== "") return lines[i].trim();
  }
  return "";
}

export function TaskProgress() {
  const { t } = useI18n();
  const [task, setTask] = React.useState<Task | null>(null);
  const [log, setLog] = React.useState("");
  const [expanded, setExpanded] = React.useState(false);
  const hideTimer = React.useRef<ReturnType<typeof setTimeout> | null>(null);
  const logRef = React.useRef<HTMLPreElement | null>(null);

  const clearHideTimer = () => {
    if (hideTimer.current) {
      clearTimeout(hideTimer.current);
      hideTimer.current = null;
    }
  };

  React.useEffect(() => {
    const rt = runtime();
    if (!rt) return;

    const offStart = rt.EventsOn("task:start", (...data: unknown[]) => {
      const p = data[0] as { id: number; label: string };
      clearHideTimer();
      setLog("");
      setExpanded(false);
      setTask({ id: p.id, label: p.label, status: "running", error: "" });
    });

    const offLog = rt.EventsOn("task:log", (...data: unknown[]) => {
      const p = data[0] as { id: number; chunk: string };
      setLog((prev) => prev + p.chunk);
    });

    const offEnd = rt.EventsOn("task:end", (...data: unknown[]) => {
      const p = data[0] as { id: number; status: TaskStatus; error: string };
      setTask((prev) =>
        prev && prev.id === p.id
          ? { ...prev, status: p.status, error: p.error ?? "" }
          : prev
      );
      // Auto-hide on success; keep on error until dismissed.
      if (p.status === "done") {
        clearHideTimer();
        hideTimer.current = setTimeout(() => setTask(null), 4000);
      }
    });

    return () => {
      offStart();
      offLog();
      offEnd();
      clearHideTimer();
    };
  }, []);

  // Keep the expanded log scrolled to the bottom as it streams.
  React.useEffect(() => {
    if (expanded && logRef.current) {
      logRef.current.scrollTop = logRef.current.scrollHeight;
    }
  }, [log, expanded]);

  if (!task) return null;

  const close = () => {
    clearHideTimer();
    setTask(null);
  };

  const statusText =
    task.status === "running"
      ? t("task.running")
      : task.status === "done"
        ? t("task.done")
        : t("task.failed");

  const current =
    task.status === "error" && task.error
      ? task.error
      : lastLine(log) || statusText;

  return (
    <div className="fixed bottom-4 right-4 z-40 w-96 max-w-[90vw]">
      <div
        className={cn(
          "overflow-hidden rounded-lg border shadow-lg",
          task.status === "error"
            ? "border-destructive/50 bg-card"
            : "border-border bg-card"
        )}
      >
        {/* Header row — click to expand/collapse the log. */}
        <button
          onClick={() => setExpanded((e) => !e)}
          className="flex w-full items-center gap-2 px-3 py-2 text-left hover:bg-muted/50"
        >
          {task.status === "running" && (
            <Loader2 className="size-4 shrink-0 animate-spin text-primary" />
          )}
          {task.status === "done" && (
            <CheckCircle2 className="size-4 shrink-0 text-emerald-600" />
          )}
          {task.status === "error" && (
            <AlertCircle className="size-4 shrink-0 text-destructive" />
          )}
          <div className="min-w-0 flex-1">
            <div className="truncate text-sm font-medium">{task.label}</div>
            <div className="truncate text-xs text-muted-foreground">
              {current}
            </div>
          </div>
          {expanded ? (
            <ChevronDown className="size-4 shrink-0 text-muted-foreground" />
          ) : (
            <ChevronUp className="size-4 shrink-0 text-muted-foreground" />
          )}
          <X
            className="size-4 shrink-0 text-muted-foreground hover:text-foreground"
            onClick={(e) => {
              e.stopPropagation();
              close();
            }}
          />
        </button>

        {expanded && (
          <pre
            ref={logRef}
            className="max-h-72 overflow-auto border-t bg-muted/40 px-3 py-2 text-xs leading-relaxed"
          >
            {log.trimEnd() || t("task.empty")}
          </pre>
        )}
      </div>
    </div>
  );
}
