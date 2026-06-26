import { type ReactNode, useEffect, useRef, useState } from "react";
import {
  ChevronDown,
  ChevronRight,
  ExternalLink,
  Folder,
  Terminal as TerminalIcon,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { useI18n } from "@/i18n/I18nProvider";
import { TERMINAL_PRESETS, TOOL_PRESETS, useDefaultTool } from "@/lib/tool";
import { cn } from "@/lib/utils";

// ToolOpenMenu renders an external-open button that opens a dropdown with
// three actions: folder / terminal / <configured tool>. The terminal and tool
// rows carry an arrow sub-button that expands an inline list of alternative
// terminals / programs. It is purely presentational — each host wires the
// actions to its own context.
export function ToolOpenMenu({
  onFolder,
  onTerminal,
  onTool,
  onToolBrowse,
  disabled,
  align = "end",
  iconOnly = false,
}: {
  onFolder: () => void;
  // program omitted => host's default; provided => open with that program.
  onTerminal: (program?: string) => void;
  onTool: (program?: string) => void;
  // When provided, an extra "choose program…" item is shown that lets the host
  // pick an arbitrary executable (e.g. via a file dialog).
  onToolBrowse?: () => void;
  disabled?: boolean;
  align?: "start" | "end";
  iconOnly?: boolean;
}) {
  const { t } = useI18n();
  const { label } = useDefaultTool();
  const [open, setOpen] = useState(false);
  const [expanded, setExpanded] = useState<"terminal" | "tool" | null>(null);
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

  // Collapse any expanded sub-list whenever the menu closes.
  useEffect(() => {
    if (!open) setExpanded(null);
  }, [open]);

  const choose = (fn: () => void) => {
    setOpen(false);
    fn();
  };

  const toggleExpand = (key: "terminal" | "tool") =>
    setExpanded((prev) => (prev === key ? null : key));

  return (
    <div ref={ref} className="relative inline-block">
      {iconOnly ? (
        <Button
          variant="ghost"
          size="sm"
          className="w-8 px-0"
          disabled={disabled}
          onClick={() => setOpen((v) => !v)}
          title={t("toolOpen.title")}
          aria-haspopup="menu"
          aria-expanded={open}
        >
          <ExternalLink className="size-4" />
        </Button>
      ) : (
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
      )}
      {open && (
        <div
          role="menu"
          className={cn(
            "absolute z-50 mt-1 min-w-48 overflow-hidden rounded-md border bg-card text-card-foreground p-1 shadow-md",
            align === "end" ? "right-0" : "left-0"
          )}
        >
          <ToolMenuItem
            icon={<Folder className="size-4" />}
            label={t("toolOpen.folder")}
            onClick={() => choose(onFolder)}
          />
          <ToolMenuRow
            icon={<TerminalIcon className="size-4" />}
            label={t("toolOpen.terminal")}
            expanded={expanded === "terminal"}
            onClick={() => choose(() => onTerminal())}
            onToggle={() => toggleExpand("terminal")}
            toggleTitle={t("toolOpen.otherTerminals")}
          />
          {expanded === "terminal" &&
            TERMINAL_PRESETS.map((p) => (
              <ToolSubItem
                key={p.value}
                label={p.label}
                onClick={() => choose(() => onTerminal(p.value))}
              />
            ))}
          <ToolMenuRow
            icon={<ExternalLink className="size-4" />}
            label={label}
            expanded={expanded === "tool"}
            onClick={() => choose(() => onTool())}
            onToggle={() => toggleExpand("tool")}
            toggleTitle={t("toolOpen.otherPrograms")}
          />
          {expanded === "tool" && (
            <>
              {TOOL_PRESETS.map((p) => (
                <ToolSubItem
                  key={p.value}
                  label={p.label}
                  onClick={() => choose(() => onTool(p.value))}
                />
              ))}
              {onToolBrowse && (
                <ToolSubItem
                  label={t("toolOpen.browse")}
                  onClick={() => choose(onToolBrowse)}
                />
              )}
            </>
          )}
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

// ToolMenuRow is a menu item with a trailing expand/collapse arrow. The main
// area triggers the default action; the arrow toggles the inline sub-list.
function ToolMenuRow({
  icon,
  label,
  expanded,
  onClick,
  onToggle,
  toggleTitle,
}: {
  icon: ReactNode;
  label: string;
  expanded: boolean;
  onClick: () => void;
  onToggle: () => void;
  toggleTitle: string;
}) {
  return (
    <div className="flex items-center rounded hover:bg-accent hover:text-accent-foreground">
      <button
        type="button"
        role="menuitem"
        onClick={onClick}
        className="flex min-w-0 flex-1 items-center gap-2 rounded-l px-2 py-1.5 text-left text-sm"
      >
        {icon}
        <span className="truncate">{label}</span>
      </button>
      <button
        type="button"
        onClick={onToggle}
        title={toggleTitle}
        aria-label={toggleTitle}
        aria-expanded={expanded}
        className="flex size-7 shrink-0 items-center justify-center rounded-r hover:bg-accent-foreground/10"
      >
        {expanded ? (
          <ChevronDown className="size-4" />
        ) : (
          <ChevronRight className="size-4" />
        )}
      </button>
    </div>
  );
}

function ToolSubItem({
  label,
  onClick,
}: {
  label: string;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      role="menuitem"
      onClick={onClick}
      className="flex w-full items-center rounded py-1.5 pl-8 pr-2 text-left text-sm text-muted-foreground hover:bg-accent hover:text-accent-foreground"
    >
      <span className="truncate">{label}</span>
    </button>
  );
}
