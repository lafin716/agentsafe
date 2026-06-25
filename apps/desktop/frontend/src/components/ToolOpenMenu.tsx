import { type ReactNode, useEffect, useRef, useState } from "react";
import { ExternalLink, Folder, Terminal as TerminalIcon } from "lucide-react";
import { Button } from "@/components/ui/button";
import { useI18n } from "@/i18n/I18nProvider";
import { useDefaultTool } from "@/lib/tool";
import { cn } from "@/lib/utils";

// ToolOpenMenu renders an external-open icon button that opens a dropdown with
// three actions: folder / terminal / <configured tool>. It is purely
// presentational — each host wires the actions to its own context.
export function ToolOpenMenu({
  onFolder,
  onTerminal,
  onTool,
  disabled,
  align = "end",
}: {
  onFolder: () => void;
  onTerminal: () => void;
  onTool: () => void;
  disabled?: boolean;
  align?: "start" | "end";
}) {
  const { t } = useI18n();
  const { label } = useDefaultTool();
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    if (!open) return;
    const onDown = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setOpen(false);
    };
    window.addEventListener("mousedown", onDown);
    window.addEventListener("keydown", onKey);
    return () => {
      window.removeEventListener("mousedown", onDown);
      window.removeEventListener("keydown", onKey);
    };
  }, [open]);

  const choose = (fn: () => void) => {
    setOpen(false);
    fn();
  };

  return (
    <div ref={ref} className="relative inline-block">
      <Button
        variant="outline"
        size="sm"
        disabled={disabled}
        onClick={() => setOpen((v) => !v)}
        title={t("toolOpen.title")}
        aria-haspopup="menu"
        aria-expanded={open}
      >
        <ExternalLink className="size-4" /> {t("toolOpen.title")}
      </Button>
      {open && (
        <div
          role="menu"
          className={cn(
            "absolute z-50 mt-1 min-w-40 overflow-hidden rounded-md border bg-card text-card-foreground p-1 shadow-md",
            align === "end" ? "right-0" : "left-0"
          )}
        >
          <ToolMenuItem
            icon={<Folder className="size-4" />}
            label={t("toolOpen.folder")}
            onClick={() => choose(onFolder)}
          />
          <ToolMenuItem
            icon={<TerminalIcon className="size-4" />}
            label={t("toolOpen.terminal")}
            onClick={() => choose(onTerminal)}
          />
          <ToolMenuItem
            icon={<ExternalLink className="size-4" />}
            label={label}
            onClick={() => choose(onTool)}
          />
        </div>
      )}
    </div>
  );
}

function ToolMenuItem({
  icon,
  label,
  onClick,
}: {
  icon: ReactNode;
  label: string;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      role="menuitem"
      onClick={onClick}
      className="flex w-full items-center gap-2 rounded px-2 py-1.5 text-left text-sm hover:bg-accent hover:text-accent-foreground"
    >
      {icon}
      <span className="truncate">{label}</span>
    </button>
  );
}
