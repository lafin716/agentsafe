// App-wide Tool Entry registry plus per-location last-used Open Tool state.
// The legacy agentsafe.program key is retained only for migration compatibility.
import { useEffect, useState } from "react";

export const TOOL_CHANGED_EVENT = "agentsafe:tool-changed";
const TOOL_KEY = "agentsafe.program";
const TOOL_SETTINGS_KEY = "agentsafe.toolSettings";

export type ToolLocation = "workspace" | "worktree" | "agent" | "explorer";

export interface ToolEntry {
  id: string;
  label: string;
  command: string;
}

export interface ToolSettings {
  version: 1;
  tools: ToolEntry[];
  defaultToolId: string;
  lastToolIds: Partial<Record<ToolLocation, string>>;
}

export type ToolSettingsErrorCode =
  | "requiredLabel"
  | "requiredCommand"
  | "invalidCommand"
  | "duplicateLabel"
  | "duplicateCommand"
  | "toolNotFound"
  | "invalidToolOrder"
  | "lastTool";

export class ToolSettingsError extends Error {
  constructor(readonly code: ToolSettingsErrorCode) {
    super(code);
    this.name = "ToolSettingsError";
  }
}

const VS_CODE_TOOL: ToolEntry = {
  id: "vscode",
  label: "VS Code",
  command: "code",
};

const LEGACY_TOOL_LABELS: Record<string, string> = {
  cursor: "Cursor",
  subl: "Sublime Text",
  idea: "IntelliJ IDEA",
  webstorm: "WebStorm",
};

function initialToolSettings(): ToolSettings {
  const legacy = localStorage.getItem(TOOL_KEY)?.trim() || "code";
  const settings: ToolSettings = {
    version: 1,
    tools: [{ ...VS_CODE_TOOL }],
    defaultToolId: VS_CODE_TOOL.id,
    lastToolIds: {},
  };
  if (comparableCommand(legacy) !== comparableCommand("code")) {
    const legacyKey = legacy.toLocaleLowerCase();
    const entry: ToolEntry = {
      id: LEGACY_TOOL_LABELS[legacyKey] ? legacyKey : "legacy-custom",
      label: LEGACY_TOOL_LABELS[legacyKey] ?? toolLabel(legacy),
      command: legacy,
    };
    settings.tools.push(entry);
    settings.defaultToolId = entry.id;
  }
  localStorage.setItem(TOOL_SETTINGS_KEY, JSON.stringify(settings));
  return settings;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function parseToolSettings(raw: string): ToolSettings | null {
  const value: unknown = JSON.parse(raw);
  if (!isRecord(value) || value.version !== 1 || !Array.isArray(value.tools)) {
    return null;
  }
  const tools: ToolEntry[] = [];
  const ids = new Set<string>();
  const labels = new Set<string>();
  const commands = new Set<string>();
  for (const candidate of value.tools) {
    if (
      !isRecord(candidate) ||
      typeof candidate.id !== "string" ||
      typeof candidate.label !== "string" ||
      typeof candidate.command !== "string" ||
      !candidate.id.trim() ||
      !candidate.label.trim() ||
      !candidate.command.trim()
    ) {
      return null;
    }
    const label = comparableLabel(candidate.label);
    const command = comparableCommand(candidate.command);
    if (ids.has(candidate.id) || labels.has(label) || commands.has(command)) {
      return null;
    }
    ids.add(candidate.id);
    labels.add(label);
    commands.add(command);
    tools.push({
      id: candidate.id,
      label: candidate.label.trim(),
      command: candidate.command.trim(),
    });
  }
  if (
    tools.length === 0 ||
    typeof value.defaultToolId !== "string" ||
    !ids.has(value.defaultToolId)
  ) {
    return null;
  }
  const lastToolIds: ToolSettings["lastToolIds"] = {};
  if (isRecord(value.lastToolIds)) {
    for (const location of ["workspace", "worktree", "agent", "explorer"] as const) {
      const id = value.lastToolIds[location];
      if (typeof id === "string" && ids.has(id)) lastToolIds[location] = id;
    }
  }
  return {
    version: 1,
    tools,
    defaultToolId: value.defaultToolId,
    lastToolIds,
  };
}

export function getToolSettings(): ToolSettings {
  try {
    const raw = localStorage.getItem(TOOL_SETTINGS_KEY);
    if (raw) {
      const settings = parseToolSettings(raw);
      if (settings) return settings;
    }
    return initialToolSettings();
  } catch {
    return {
      version: 1,
      tools: [{ ...VS_CODE_TOOL }],
      defaultToolId: VS_CODE_TOOL.id,
      lastToolIds: {},
    };
  }
}

function saveToolSettings(settings: ToolSettings): ToolSettings {
  localStorage.setItem(TOOL_SETTINGS_KEY, JSON.stringify(settings));
  const defaultTool = settings.tools.find(
    (entry) => entry.id === settings.defaultToolId
  );
  if (defaultTool) localStorage.setItem(TOOL_KEY, defaultTool.command);
  window.dispatchEvent(new Event(TOOL_CHANGED_EVENT));
  return settings;
}

function comparableLabel(value: string): string {
  return value.trim().toLocaleLowerCase();
}

function comparableCommand(value: string): string {
  return value.trim().replace(/^(["'])(.*)\1$/, "$2").replace(/\//g, "\\").toLocaleLowerCase();
}

function normalizeCommandInput(value: string): string {
  const command = value.trim().replace(/^(["'])(.*)\1$/, "$2");
  if (!command) throw new ToolSettingsError("requiredCommand");
  const isPath = /^(?:[a-z]:[\\/]|\\\\|\/|\.{1,2}[\\/])/i.test(command);
  if (!isPath && (/\s/.test(command) || /[|;&><]/.test(command))) {
    throw new ToolSettingsError("invalidCommand");
  }
  return command;
}

function validateToolEntryInput(
  settings: ToolSettings,
  label: string,
  command: string,
  excludedId?: string
): Pick<ToolEntry, "label" | "command"> {
  const nextLabel = label.trim();
  if (!nextLabel) throw new ToolSettingsError("requiredLabel");
  const nextCommand = normalizeCommandInput(command);
  if (
    settings.tools.some(
      (entry) =>
        entry.id !== excludedId &&
        comparableLabel(entry.label) === comparableLabel(nextLabel)
    )
  ) {
    throw new ToolSettingsError("duplicateLabel");
  }
  if (
    settings.tools.some(
      (entry) =>
        entry.id !== excludedId &&
        comparableCommand(entry.command) === comparableCommand(nextCommand)
    )
  ) {
    throw new ToolSettingsError("duplicateCommand");
  }
  return { label: nextLabel, command: nextCommand };
}

function nextToolId(label: string): string {
  const slug = label
    .toLocaleLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-|-$/g, "") || "tool";
  const current = getToolSettings().tools;
  if (!current.some((entry) => entry.id === slug)) return slug;
  let suffix = 2;
  while (current.some((entry) => entry.id === `${slug}-${suffix}`)) suffix++;
  return `${slug}-${suffix}`;
}

export function addToolEntry(label: string, command: string): ToolEntry {
  const settings = getToolSettings();
  const input = validateToolEntryInput(settings, label, command);
  const entry = { id: nextToolId(input.label), ...input };
  saveToolSettings({ ...settings, tools: [...settings.tools, entry] });
  return entry;
}

function requireTool(settings: ToolSettings, id: string): ToolEntry {
  const entry = settings.tools.find((tool) => tool.id === id);
  if (!entry) throw new ToolSettingsError("toolNotFound");
  return entry;
}

export function setDefaultToolId(id: string): void {
  const settings = getToolSettings();
  requireTool(settings, id);
  saveToolSettings({ ...settings, defaultToolId: id });
}

export function setLastToolForLocation(location: ToolLocation, id: string): void {
  const settings = getToolSettings();
  requireTool(settings, id);
  saveToolSettings({
    ...settings,
    lastToolIds: { ...settings.lastToolIds, [location]: id },
  });
}

export function getToolForLocation(location: ToolLocation): ToolEntry {
  const settings = getToolSettings();
  const id = settings.lastToolIds[location] ?? settings.defaultToolId;
  return (
    settings.tools.find((entry) => entry.id === id) ??
    requireTool(settings, settings.defaultToolId)
  );
}

export function updateToolEntry(
  id: string,
  label: string,
  command: string
): ToolEntry {
  const settings = getToolSettings();
  requireTool(settings, id);
  const input = validateToolEntryInput(settings, label, command, id);
  const updated = { id, ...input };
  saveToolSettings({
    ...settings,
    tools: settings.tools.map((entry) => (entry.id === id ? updated : entry)),
  });
  return updated;
}

export function reorderToolEntries(ids: string[]): void {
  const settings = getToolSettings();
  if (
    ids.length !== settings.tools.length ||
    new Set(ids).size !== ids.length ||
    ids.some((id) => !settings.tools.some((entry) => entry.id === id))
  ) {
    throw new ToolSettingsError("invalidToolOrder");
  }
  const byId = new Map(settings.tools.map((entry) => [entry.id, entry]));
  saveToolSettings({
    ...settings,
    tools: ids.map((id) => byId.get(id) as ToolEntry),
  });
}

export function deleteToolEntry(id: string): void {
  const settings = getToolSettings();
  requireTool(settings, id);
  if (settings.tools.length === 1) throw new ToolSettingsError("lastTool");
  const tools = settings.tools.filter((entry) => entry.id !== id);
  const lastToolIds = Object.fromEntries(
    Object.entries(settings.lastToolIds).filter(([, toolId]) => toolId !== id)
  ) as ToolSettings["lastToolIds"];
  saveToolSettings({
    ...settings,
    tools,
    defaultToolId:
      settings.defaultToolId === id ? tools[0].id : settings.defaultToolId,
    lastToolIds,
  });
}

// Terminal programs offered by the tool-open menu and the settings selects.
// Values match what the Go backend's TerminalOpenWithProgram understands.
export const TERMINAL_PRESETS: { value: string; label: string }[] = [
  { value: "powershell", label: "PowerShell" },
  { value: "pwsh", label: "PowerShell 7" },
  { value: "cmd", label: "Command Prompt" },
  { value: "git-bash", label: "Git Bash" },
  { value: "wt", label: "Windows Terminal" },
  { value: "default", label: "System default" },
];

// toolLabel resolves a legacy command/path to an initial Tool Entry label.
export function toolLabel(value: string): string {
  const normalized = comparableCommand(value);
  if (normalized === "code") return VS_CODE_TOOL.label;
  const legacyLabel = LEGACY_TOOL_LABELS[value.toLocaleLowerCase()];
  if (legacyLabel) return legacyLabel;
  const base = value.split(/[\\/]/).pop() || value;
  return base.replace(/\.(app|exe)$/i, "");
}

export function useToolSettings(): ToolSettings {
  const [settings, setSettings] = useState(getToolSettings);
  useEffect(() => {
    const onChange = () => setSettings(getToolSettings());
    window.addEventListener(TOOL_CHANGED_EVENT, onChange);
    return () => window.removeEventListener(TOOL_CHANGED_EVENT, onChange);
  }, []);
  return settings;
}

export function useLocationTool(location: ToolLocation): {
  tool: ToolEntry;
  settings: ToolSettings;
} {
  const settings = useToolSettings();
  const id = settings.lastToolIds[location] ?? settings.defaultToolId;
  const tool =
    settings.tools.find((entry) => entry.id === id) ?? settings.tools[0];
  return { tool, settings };
}
