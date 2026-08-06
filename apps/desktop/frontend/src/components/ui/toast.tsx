import * as React from "react";
import { cn } from "@/lib/utils";

// "warning" is for an outcome the user has to act on that is not a failure —
// an Interrupted Integration is the motivating case: the work is intact and
// waiting to be resolved, so calling it an error would misdescribe it.
type ToastKind = "info" | "success" | "warning" | "error";
type Toast = { id: number; kind: ToastKind; message: string };

type ToastContextValue = {
  notify: (message: string, kind?: ToastKind) => void;
};

const ToastContext = React.createContext<ToastContextValue | null>(null);

export function ToastProvider({ children }: { children: React.ReactNode }) {
  const [toasts, setToasts] = React.useState<Toast[]>([]);

  const notify = React.useCallback((message: string, kind: ToastKind = "info") => {
    const id = Date.now() + Math.random();
    setToasts((prev) => [...prev, { id, kind, message }]);
    setTimeout(() => {
      setToasts((prev) => prev.filter((t) => t.id !== id));
    }, 4500);
  }, []);

  return (
    <ToastContext.Provider value={{ notify }}>
      {children}
      <div className="fixed bottom-20 right-4 z-50 flex w-96 max-w-[90vw] flex-col gap-2">
        {toasts.map((t) => (
          <div
            key={t.id}
            className={cn(
              "rounded-md border px-4 py-3 text-sm shadow-lg",
              t.kind === "error" &&
                "border-destructive/40 bg-destructive text-destructive-foreground",
              t.kind === "success" &&
                "border-emerald-700 bg-emerald-600 text-white",
              t.kind === "warning" && "border-amber-600 bg-amber-500 text-white",
              t.kind === "info" && "border-border bg-card text-card-foreground"
            )}
          >
            {t.message}
          </div>
        ))}
      </div>
    </ToastContext.Provider>
  );
}

export function useToast() {
  const ctx = React.useContext(ToastContext);
  if (!ctx) throw new Error("useToast must be used within ToastProvider");
  return ctx;
}
