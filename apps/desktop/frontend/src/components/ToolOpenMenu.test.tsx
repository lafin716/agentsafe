import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  addToolEntry,
  getToolForLocation,
  setDefaultToolId,
  setLastToolForLocation,
} from "@/lib/tool";
import { ToolOpenMenu } from "./ToolOpenMenu";

vi.mock("@/i18n/I18nProvider", () => ({
  useI18n: () => ({ t: (key: string) => key }),
}));

describe("ToolOpenMenu", () => {
  beforeEach(() => {
    localStorage.clear();
  });

  afterEach(cleanup);

  it("shows the location tool and remembers a successfully launched alternative", async () => {
    const cursor = addToolEntry("Cursor", "cursor");
    setDefaultToolId(cursor.id);
    const onTool = vi.fn(async () => undefined);
    const user = userEvent.setup();

    render(
      <ToolOpenMenu
        location="worktree"
        onFolder={vi.fn()}
        onTerminal={vi.fn()}
        onTool={onTool}
        onToolBrowse={vi.fn()}
      />
    );

    await user.click(screen.getByRole("button", { name: "toolOpen.title" }));
    expect(screen.getByRole("menuitem", { name: "Cursor" })).toBeTruthy();

    await user.click(
      screen.getByRole("button", { name: "toolOpen.otherTools" })
    );
    await user.click(screen.getByRole("menuitem", { name: "VS Code" }));

    expect(onTool).toHaveBeenCalledWith("code");
    expect(getToolForLocation("worktree").id).toBe("vscode");
  });

  it("launches the location tool from the row body, not the expand arrow", async () => {
    const cursor = addToolEntry("Cursor", "cursor");
    setDefaultToolId(cursor.id);
    const onTool = vi.fn(async () => undefined);
    const user = userEvent.setup();

    render(
      <ToolOpenMenu
        location="worktree"
        onFolder={vi.fn()}
        onTerminal={vi.fn()}
        onTool={onTool}
        onToolBrowse={vi.fn()}
      />
    );

    await user.click(screen.getByRole("button", { name: "toolOpen.title" }));
    await user.click(screen.getByRole("menuitem", { name: "Cursor" }));

    expect(onTool).toHaveBeenCalledWith("cursor");
    // The row body runs the tool and closes the menu instead of expanding.
    expect(screen.queryByRole("menu")).toBeNull();
  });

  it("keeps the previous location tool when an alternative launch fails", async () => {
    addToolEntry("Cursor", "cursor");
    setLastToolForLocation("worktree", "vscode");
    const user = userEvent.setup();

    render(
      <ToolOpenMenu
        location="worktree"
        onFolder={vi.fn()}
        onTerminal={vi.fn()}
        onTool={vi.fn(async () => {
          throw new Error("launch failed");
        })}
      />
    );

    await user.click(screen.getByRole("button", { name: "toolOpen.title" }));
    await user.click(
      screen.getByRole("button", { name: "toolOpen.otherTools" })
    );
    await user.click(screen.getByRole("menuitem", { name: "Cursor" }));

    expect(getToolForLocation("worktree").id).toBe("vscode");
  });

  it("does not remember a temporarily chosen program", async () => {
    const onToolBrowse = vi.fn(async () => undefined);
    const user = userEvent.setup();

    render(
      <ToolOpenMenu
        location="explorer"
        onFolder={vi.fn()}
        onTerminal={vi.fn()}
        onTool={vi.fn()}
        onToolBrowse={onToolBrowse}
      />
    );

    await user.click(screen.getByRole("button", { name: "toolOpen.title" }));
    await user.click(
      screen.getByRole("button", { name: "toolOpen.otherTools" })
    );
    await user.click(screen.getByRole("menuitem", { name: "toolOpen.browse" }));

    expect(onToolBrowse).toHaveBeenCalledOnce();
    expect(getToolForLocation("explorer").id).toBe("vscode");
  });
});
