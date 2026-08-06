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
import {
  TERMINAL_PRESETS,
  setLastToolForLocation,
  type ToolEntry,
  type ToolLocation,
  useLocationTool,
} from "@/lib/tool";
import { cn } from "@/lib/utils";

// ToolOpenMenu renders an external-open button that opens a dropdown with
// three actions: folder / terminal / <configured tool>. The terminal and tool
// rows carry an arrow sub-button that expands an inline list of alternative
// terminals / Open Tools. It is purely presentational — each host wires the
// actions to its own context.
export function ToolOpenMenu({
  location,
  onFolder,
  onTerminal,
  onTool,
  onToolBrowse,
  terminalDisabled = false,
  toolDisabled = false,
  disabled,
  align = "end",
  iconOnly = false,
}: {
  location: ToolLocation;
  onFolder: () => void;
  // terminal command omitted => host's default; provided => use that terminal.
  onTerminal: (terminalProgram?: string) => void;
  onTool: (toolCommand: string) => void | Promise<void>;
  // When provided, an executable picker lets the host launch a temporary Tool.
  onToolBrowse?: () => void | Promise<void>;
  terminalDisabled?: boolean;
  toolDisabled?: boolean;
  disabled?: boolean;
  align?: "start" | "end";
  iconOnly?: boolean;
}) {
  const { t } = useI18n();
  const { tool, settings } = useLocationTool(location);
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

  const launchTool = async (entry: ToolEntry) => {
    setOpen(false);
    try {
      await onTool(entry.command);
      setLastToolForLocation(location, entry.id);
    } catch {
      // Hosts own error presentation; a rejected launch must not change history.
    }
  };

  const launchTemporaryTool = async () => {
    if (!onToolBrowse) return;
    setOpen(false);
    try {
      await onToolBrowse();
    } catch {
      // Temporary launches are never remembered; hosts own error presentation.
    }
  };

  const toggleExpand = (key: "terminal" | "tool") =>
    setExpanded((prev) => (prev === key ? null : key));

  return (
    <div
      ref={ref}
      className="relative inline-block"
      onClick={(event) => event.stopPropagation()}
    >
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
            disabled={terminalDisabled}
          />
          {expanded === "terminal" && !terminalDisabled &&
            TERMINAL_PRESETS.map((p) => (
              <ToolSubItem
                key={p.value}
                label={p.label}
                onClick={() => choose(() => onTerminal(p.value))}
              />
            ))}
          <ToolMenuRow
            icon={<ExternalLink className="size-4" />}
            label={tool.label}
            expanded={expanded === "tool"}
            onClick={() => void launchTool(tool)}
            onToggle={() => toggleExpand("tool")}
            toggleTitle={t("toolOpen.otherTools")}
            disabled={toolDisabled}
          />
          {expanded === "tool" && !toolDisabled && (
            <>
              {settings.tools
                .filter((entry) => entry.id !== tool.id)
                .map((entry) => (
                <ToolSubItem
                  key={entry.id}
                  label={entry.label}
                  onClick={() => void launchTool(entry)}
                />
              ))}
              {onToolBrowse && (
                <ToolSubItem
                  label={t("toolOpen.browse")}
                  onClick={() => void launchTemporaryTool()}
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
  disabled = false,
}: {
  icon: ReactNode;
  label: string;
  onClick: () => void;
  disabled?: boolean;
}) {
  return (
    <button
      type="button"
      role="menuitem"
      onClick={onClick}
      disabled={disabled}
      className="flex w-full items-center gap-2 rounded px-2 py-1.5 text-left text-sm hover:bg-accent hover:text-accent-foreground disabled:pointer-events-none disabled:opacity-50"
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
  disabled = false,
}: {
  icon: ReactNode;
  label: string;
  expanded: boolean;
  onClick: () => void;
  onToggle: () => void;
  toggleTitle: string;
  disabled?: boolean;
}) {
  return (
    <div className="flex items-center rounded hover:bg-accent hover:text-accent-foreground">
      <button
        type="button"
        role="menuitem"
        onClick={onClick}
        disabled={disabled}
        className="flex min-w-0 flex-1 items-center gap-2 rounded-l px-2 py-1.5 text-left text-sm disabled:pointer-events-none disabled:opacity-50"
      >
        {icon}
        <span className="truncate">{label}</span>
      </button>
      <button
        type="button"
        onClick={onToggle}
        disabled={disabled}
        title={toggleTitle}
        aria-label={toggleTitle}
        aria-expanded={expanded}
        className="flex size-7 shrink-0 items-center justify-center rounded-r hover:bg-accent-foreground/10 disabled:pointer-events-none disabled:opacity-50"
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
