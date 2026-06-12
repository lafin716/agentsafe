import * as React from "react";
import {
  AlertCircle,
  CheckCircle2,
  ChevronDown,
  ChevronUp,
  Loader2,
  Maximize2,
  X,
} from "lucide-react";
import { cn } from "@/lib/utils";
import { useI18n } from "@/i18n/I18nProvider";

type TaskStatus = "running" | "done" | "error";

type Task = {
  id: number;
  label: string;
  status: TaskStatus;
  error: string;
};

type WailsRuntime = {
  EventsOn: (event: string, cb: (...data: unknown[]) => void) => () => void;
};

function runtime(): WailsRuntime | null {
  const rt = (window as unknown as { runtime?: WailsRuntime }).runtime;
  return rt && typeof rt.EventsOn === "function" ? rt : null;
}

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
  const [modalOpen, setModalOpen] = React.useState(false);
  const hideTimer = React.useRef<ReturnType<typeof setTimeout> | null>(null);
  const logRef = React.useRef<HTMLPreElement | null>(null);

  const clearHideTimer = React.useCallback(() => {
    if (hideTimer.current) {
      clearTimeout(hideTimer.current);
      hideTimer.current = null;
    }
  }, []);

  React.useEffect(() => {
    const rt = runtime();
    if (!rt) return;

    const offStart = rt.EventsOn("task:start", (...data: unknown[]) => {
      const p = data[0] as { id: number; label: string };
      clearHideTimer();
      setLog("");
      setExpanded(false);
      setModalOpen(false);
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
  }, [clearHideTimer]);

  React.useEffect(() => {
    if (expanded && logRef.current) {
      logRef.current.scrollTop = logRef.current.scrollHeight;
    }
  }, [log, expanded]);

  React.useEffect(() => {
    if (!modalOpen) return;
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") setModalOpen(false);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [modalOpen]);

  if (!task) return null;

  const close = () => {
    clearHideTimer();
    setModalOpen(false);
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
  const fullMessage = [
    log.trimEnd(),
    task.error && !log.includes(task.error) ? task.error : "",
  ]
    .filter(Boolean)
    .join("\n\n");

  const openModal = () => {
    clearHideTimer();
    setModalOpen(true);
  };

  return (
    <>
      <div className="fixed bottom-4 right-4 z-40 w-96 max-w-[90vw]">
        <div
          className={cn(
            "overflow-hidden rounded-lg border shadow-lg",
            task.status === "error"
              ? "border-destructive/50 bg-card"
              : "border-border bg-card"
          )}
        >
          <div className="flex w-full items-center gap-2 px-3 py-2">
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
              <button
                type="button"
                className="flex max-w-full items-center gap-1.5 text-left hover:text-primary"
                onClick={openModal}
                title={t("task.openDetails")}
              >
                <span className="truncate text-sm font-medium">{task.label}</span>
                <Maximize2 className="size-3.5 shrink-0" />
              </button>
              <div className="truncate text-xs text-muted-foreground">
                {current}
              </div>
            </div>
            <button
              type="button"
              className="text-muted-foreground hover:text-foreground"
              onClick={() => setExpanded((value) => !value)}
              title={expanded ? t("task.collapse") : t("task.expand")}
            >
              {expanded ? (
                <ChevronDown className="size-4" />
              ) : (
                <ChevronUp className="size-4" />
              )}
            </button>
            <button
              type="button"
              className="text-muted-foreground hover:text-foreground"
              onClick={close}
              title={t("common.close")}
            >
              <X className="size-4" />
            </button>
          </div>

          {expanded && (
            <pre
              ref={logRef}
              className="max-h-72 overflow-auto whitespace-pre-wrap break-words border-t bg-muted/40 px-3 py-2 text-xs leading-relaxed"
            >
              {fullMessage || t("task.empty")}
            </pre>
          )}
        </div>
      </div>

      {modalOpen && (
        <div
          className="fixed inset-0 z-[80] flex items-center justify-center bg-black/55 p-4"
          onClick={() => setModalOpen(false)}
        >
          <div
            className="flex max-h-[85vh] w-full max-w-4xl flex-col overflow-hidden rounded-lg border bg-card shadow-2xl"
            onClick={(event) => event.stopPropagation()}
          >
            <div className="flex items-center gap-3 border-b px-5 py-4">
              <div className="min-w-0 flex-1">
                <h2 className="truncate text-base font-semibold">{task.label}</h2>
                <p className="text-xs text-muted-foreground">{statusText}</p>
              </div>
              <button
                type="button"
                className="rounded-md p-1 text-muted-foreground hover:bg-muted hover:text-foreground"
                onClick={() => setModalOpen(false)}
                title={t("common.close")}
              >
                <X className="size-5" />
              </button>
            </div>
            <pre className="min-h-48 flex-1 overflow-auto whitespace-pre-wrap break-words bg-muted/35 p-5 text-xs leading-relaxed">
              {fullMessage || t("task.empty")}
            </pre>
          </div>
        </div>
      )}
    </>
  );
}
