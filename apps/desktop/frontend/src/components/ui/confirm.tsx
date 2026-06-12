import * as React from "react";
import { Button } from "@/components/ui/button";
import { useI18n } from "@/i18n/I18nProvider";

export type ConfirmOptions = {
  message: string;
  title?: string;
  confirmLabel?: string;
  cancelLabel?: string;
  danger?: boolean;
  checkbox?: {
    label: string;
    defaultChecked?: boolean;
    onChange?: (checked: boolean) => void;
  };
};

type State = ConfirmOptions & { resolve: (ok: boolean) => void };

type ConfirmContextValue = (opts: ConfirmOptions) => Promise<boolean>;

const ConfirmContext = React.createContext<ConfirmContextValue | null>(null);

// ConfirmProvider renders an in-app confirmation modal. It replaces
// window.confirm, which does not work in the macOS WKWebView used by Wails.
export function ConfirmProvider({ children }: { children: React.ReactNode }) {
  const { t } = useI18n();
  const [state, setState] = React.useState<State | null>(null);
  const [checked, setChecked] = React.useState(false);

  const confirm = React.useCallback<ConfirmContextValue>((opts) => {
    return new Promise<boolean>((resolve) => {
      const initial = opts.checkbox?.defaultChecked ?? false;
      setChecked(initial);
      // Sync the caller's initial checkbox value before any toggle.
      opts.checkbox?.onChange?.(initial);
      setState({ ...opts, resolve });
    });
  }, []);

  const toggleChecked = React.useCallback(
    (next: boolean) => {
      setChecked(next);
      setState((s) => {
        s?.checkbox?.onChange?.(next);
        return s;
      });
    },
    []
  );

  const close = React.useCallback(
    (ok: boolean) => {
      setState((s) => {
        s?.resolve(ok);
        return null;
      });
    },
    []
  );

  React.useEffect(() => {
    if (!state) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") close(false);
      if (e.key === "Enter") close(true);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [state, close]);

  return (
    <ConfirmContext.Provider value={confirm}>
      {children}
      {state && (
        <div
          className="fixed inset-0 z-[60] flex items-center justify-center bg-black/50 p-4"
          onClick={() => close(false)}
        >
          <div
            className="w-full max-w-md rounded-lg border bg-card p-5 shadow-xl"
            onClick={(e) => e.stopPropagation()}
          >
            {state.title && (
              <h2 className="mb-1.5 text-base font-semibold">{state.title}</h2>
            )}
            <p className="text-sm text-muted-foreground">{state.message}</p>
            {state.checkbox && (
              <label className="mt-3 flex items-center gap-2 text-sm">
                <input
                  type="checkbox"
                  checked={checked}
                  onChange={(e) => toggleChecked(e.target.checked)}
                />
                <span>{state.checkbox.label}</span>
              </label>
            )}
            <div className="mt-5 flex justify-end gap-2">
              <Button variant="outline" onClick={() => close(false)}>
                {state.cancelLabel ?? t("common.cancel")}
              </Button>
              <Button
                variant={state.danger ? "destructive" : "default"}
                onClick={() => close(true)}
              >
                {state.confirmLabel ?? t("common.confirm")}
              </Button>
            </div>
          </div>
        </div>
      )}
    </ConfirmContext.Provider>
  );
}

export function useConfirm() {
  const ctx = React.useContext(ConfirmContext);
  if (!ctx) throw new Error("useConfirm must be used within ConfirmProvider");
  return ctx;
}
