// Shared "default tool" state: the editor/program used by the tool-open menu and
// the feature page. Backed by localStorage["agentsafe.program"] (default "code")
// and a window event so every consumer updates live when Settings changes it.
import { useEffect, useState } from "react";

export const TOOL_CHANGED_EVENT = "agentsafe:tool-changed";
const TOOL_KEY = "agentsafe.program";

export const TOOL_PRESETS: { value: string; label: string }[] = [
  { value: "code", label: "VS Code" },
  { value: "cursor", label: "Cursor" },
  { value: "subl", label: "Sublime Text" },
  { value: "idea", label: "IntelliJ IDEA" },
  { value: "webstorm", label: "WebStorm" },
];

export const TOOL_PRESET_VALUES = TOOL_PRESETS.map((p) => p.value);

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

export function getDefaultTool(): string {
  try {
    return localStorage.getItem(TOOL_KEY) || "code";
  } catch {
    return "code";
  }
}

export function setDefaultTool(value: string): void {
  try {
    localStorage.setItem(TOOL_KEY, value);
  } catch {
    /* localStorage unavailable */
  }
  window.dispatchEvent(new Event(TOOL_CHANGED_EVENT));
}

// toolLabel resolves a stored value to a display label: a preset label, else the
// basename of an executable path (without .app/.exe).
export function toolLabel(value: string): string {
  const preset = TOOL_PRESETS.find((p) => p.value === value);
  if (preset) return preset.label;
  const base = value.split(/[\\/]/).pop() || value;
  return base.replace(/\.(app|exe)$/i, "");
}

export function useDefaultTool(): { value: string; label: string } {
  const [value, setValue] = useState(getDefaultTool);
  useEffect(() => {
    const onChange = () => setValue(getDefaultTool());
    window.addEventListener(TOOL_CHANGED_EVENT, onChange);
    return () => window.removeEventListener(TOOL_CHANGED_EVENT, onChange);
  }, []);
  return { value, label: toolLabel(value) };
}
