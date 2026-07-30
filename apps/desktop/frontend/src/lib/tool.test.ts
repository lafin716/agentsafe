import { beforeEach, describe, expect, it } from "vitest";
import {
  addToolEntry,
  deleteToolEntry,
  getToolForLocation,
  getToolSettings,
  reorderToolEntries,
  setDefaultToolId,
  setLastToolForLocation,
  updateToolEntry,
} from "./tool";

describe("Open Tool settings", () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it("starts new installations with only VS Code and the code command", () => {
    expect(getToolSettings()).toEqual({
      version: 1,
      tools: [{ id: "vscode", label: "VS Code", command: "code" }],
      defaultToolId: "vscode",
      lastToolIds: {},
    });
  });

  it("preserves the legacy default tool while seeding VS Code", () => {
    localStorage.setItem("agentsafe.program", "cursor");

    const settings = getToolSettings();

    expect(settings.tools).toEqual([
      { id: "vscode", label: "VS Code", command: "code" },
      { id: "cursor", label: "Cursor", command: "cursor" },
    ]);
    expect(settings.defaultToolId).toBe("cursor");
    expect(JSON.parse(localStorage.getItem("agentsafe.toolSettings") ?? "null")).toEqual(
      settings
    );
  });

  it("normalizes a legacy code command without creating a duplicate tool", () => {
    localStorage.setItem("agentsafe.program", "CODE");

    expect(getToolSettings()).toEqual({
      version: 1,
      tools: [{ id: "vscode", label: "VS Code", command: "code" }],
      defaultToolId: "vscode",
      lastToolIds: {},
    });
  });

  it("repairs structurally invalid persisted settings", () => {
    localStorage.setItem(
      "agentsafe.toolSettings",
      JSON.stringify({ version: 1, tools: [], defaultToolId: "missing" })
    );

    expect(getToolSettings()).toEqual({
      version: 1,
      tools: [{ id: "vscode", label: "VS Code", command: "code" }],
      defaultToolId: "vscode",
      lastToolIds: {},
    });
  });

  it("adds a trimmed tool and rejects duplicate labels or commands", () => {
    const added = addToolEntry("  Cursor  ", "  cursor  ");

    expect(added).toMatchObject({ label: "Cursor", command: "cursor" });
    expect(getToolSettings().tools).toContainEqual(added);
    expect(() => addToolEntry("cursor", "other")).toThrow("duplicateLabel");
    expect(() => addToolEntry("Other", "CURSOR")).toThrow("duplicateCommand");
  });

  it("accepts executable paths with spaces but rejects CLI arguments", () => {
    expect(
      addToolEntry("VS Code Insiders", "C:\\Program Files\\Code Insiders\\Code.exe")
    ).toMatchObject({ command: "C:\\Program Files\\Code Insiders\\Code.exe" });
    expect(() => addToolEntry("Reuse", "code --reuse-window")).toThrow(
      "invalidCommand"
    );
    expect(() => addToolEntry("Cmd", "cmd /c calc")).toThrow("invalidCommand");
    expect(() => addToolEntry("Profile", "code --profile/foo")).toThrow(
      "invalidCommand"
    );
    expect(() => addToolEntry("Pipe", "foo|bar")).toThrow("invalidCommand");
    expect(() => addToolEntry("Chain", "foo;bar")).toThrow("invalidCommand");
  });

  it("uses the app default only until a location has its own last tool", () => {
    const cursor = addToolEntry("Cursor", "cursor");
    setLastToolForLocation("worktree", cursor.id);
    setDefaultToolId(cursor.id);

    expect(getToolForLocation("workspace")).toEqual(cursor);
    expect(getToolForLocation("worktree")).toEqual(cursor);

    setDefaultToolId("vscode");

    expect(getToolForLocation("workspace").id).toBe("vscode");
    expect(getToolForLocation("worktree")).toEqual(cursor);
  });

  it("keeps references through edits and falls back safely after deletion", () => {
    const cursor = addToolEntry("Cursor", "cursor");
    const webstorm = addToolEntry("WebStorm", "webstorm");
    setDefaultToolId(cursor.id);
    setLastToolForLocation("agent", cursor.id);
    setLastToolForLocation("explorer", webstorm.id);

    const edited = updateToolEntry(cursor.id, "Cursor IDE", "cursor-cli");
    reorderToolEntries([webstorm.id, "vscode", cursor.id]);

    expect(getToolForLocation("agent")).toEqual(edited);
    expect(getToolSettings().tools.map((tool) => tool.id)).toEqual([
      webstorm.id,
      "vscode",
      cursor.id,
    ]);

    deleteToolEntry(cursor.id);

    expect(getToolSettings().defaultToolId).toBe(webstorm.id);
    expect(getToolSettings().lastToolIds.agent).toBeUndefined();
    expect(getToolForLocation("agent")).toEqual(webstorm);
    expect(getToolForLocation("explorer")).toEqual(webstorm);
  });
});
