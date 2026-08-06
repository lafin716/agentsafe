import * as React from "react";
import { X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { useI18n } from "@/i18n/I18nProvider";
import { cn } from "@/lib/utils";

// There is no dialog primitive in this project; the file-diff viewer grew its
// own overlay and every popup since has copied it. This is that pattern
// extracted — a fixed backdrop that closes on click or Escape, with the panel
// stopping propagation so clicks inside do not dismiss it.
//
// Deliberately not a focus trap: these are read-mostly panels over a desktop
// app, and a trap that has to be escaped correctly is worse than none when the
// backdrop and Escape both already close.

export function Modal({
  title,
  description,
  onClose,
  children,
  footer,
  headerActions,
  size = "md",
}: {
  title: React.ReactNode;
  description?: React.ReactNode;
  onClose: () => void;
  children: React.ReactNode;
  footer?: React.ReactNode;
  headerActions?: React.ReactNode;
  size?: "md" | "lg" | "xl";
}) {
  const { t } = useI18n();

  React.useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4"
      onClick={onClose}
    >
      <div
        className={cn(
          "flex max-h-[85vh] w-full flex-col overflow-hidden rounded-lg border bg-card shadow-xl",
          size === "md" && "max-w-xl",
          size === "lg" && "max-w-3xl",
          size === "xl" && "max-w-5xl"
        )}
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-start justify-between gap-3 border-b p-4">
          <div className="min-w-0">
            <h2 className="truncate text-base font-semibold">{title}</h2>
            {description && (
              <p className="mt-0.5 text-xs text-muted-foreground">{description}</p>
            )}
          </div>
          <div className="flex shrink-0 items-center gap-1">
            {headerActions}
            <Button
              variant="ghost"
              size="icon"
              className="size-8"
              onClick={onClose}
              title={t("common.close")}
            >
              <X className="size-4" />
            </Button>
          </div>
        </div>
        <div className="min-h-0 flex-1 overflow-auto">{children}</div>
        {footer && <div className="border-t p-3">{footer}</div>}
      </div>
    </div>
  );
}
