import { describe, expect, it } from "vitest";
import { summarizeIntegration } from "./integration";
import type { IntegrationRepoResult, IntegrationResult } from "./types";

function result(
  operation: string,
  ...repos: Array<[string, string, string[]?]>
): IntegrationResult {
  return {
    feature: "login-revamp",
    operation,
    repositories: repos.map(
      ([name, status, conflicts]): IntegrationRepoResult => ({
        name,
        branch: "feat/login",
        baseBranch: "main",
        status,
        detail: "",
        conflicts: conflicts ?? null,
      })
    ),
  };
}

describe("summarizeIntegration", () => {
  it("reports a clean rebase as a success", () => {
    const s = summarizeIntegration(result("rebase", ["api", "rebased"]));

    expect(s.level).toBe("success");
    expect(s.messageKey).toBe("integration.toast.rebased");
    expect(s.params.count).toBe(1);
  });

  it("reports a clean merge with its own message", () => {
    const s = summarizeIntegration(result("merge", ["api", "merged"]));

    expect(s.level).toBe("success");
    expect(s.messageKey).toBe("integration.toast.merged");
  });

  it("treats up-to-date as success without claiming work was done", () => {
    const s = summarizeIntegration(result("rebase", ["api", "up-to-date"]));

    expect(s.level).toBe("success");
    expect(s.messageKey).toBe("integration.toast.upToDate");
    expect(s.succeeded).toHaveLength(0);
    expect(s.upToDate).toHaveLength(1);
  });

  it("surfaces a conflict above everything else, since it needs resolving", () => {
    const s = summarizeIntegration(
      result(
        "rebase",
        ["api", "rebased"],
        ["web", "conflicted", ["src/a.ts", "src/b.ts"]],
        ["batch", "failed"]
      )
    );

    expect(s.level).toBe("warning");
    expect(s.messageKey).toBe("integration.toast.conflicted");
    expect(s.params.repos).toBe("web");
    expect(s.conflicted).toHaveLength(1);
    // A conflict is not a failure — the work is still there to finish.
    expect(s.failed).toHaveLength(1);
    expect(s.needsResolution).toBe(true);
  });

  it("reports a repository blocked by agent changes as skipped, not failed", () => {
    const s = summarizeIntegration(result("rebase", ["web", "skipped"]));

    expect(s.level).toBe("warning");
    expect(s.messageKey).toBe("integration.toast.skipped");
    expect(s.blocked).toHaveLength(1);
    expect(s.failed).toHaveLength(0);
    expect(s.needsResolution).toBe(false);
  });

  it("reports an operation that could not run as an error", () => {
    const s = summarizeIntegration(result("rebase", ["api", "failed"]));

    expect(s.level).toBe("error");
    expect(s.messageKey).toBe("integration.toast.failed");
    expect(s.params.repos).toBe("api");
  });

  it("prefers a failure over a skip when both happened", () => {
    const s = summarizeIntegration(
      result("rebase", ["api", "failed"], ["web", "skipped"])
    );

    expect(s.level).toBe("error");
    expect(s.messageKey).toBe("integration.toast.failed");
  });

  it("names every affected repository, not just the first", () => {
    const s = summarizeIntegration(
      result("rebase", ["api", "conflicted"], ["web", "conflicted"])
    );

    expect(s.params.repos).toBe("api, web");
    expect(s.params.count).toBe(2);
  });

  it("handles a continue that finished the integration", () => {
    const s = summarizeIntegration(result("continue", ["api", "continued"]));

    expect(s.level).toBe("success");
    expect(s.messageKey).toBe("integration.toast.continued");
    expect(s.needsResolution).toBe(false);
  });

  it("handles a continue that stopped on the next conflict", () => {
    const s = summarizeIntegration(
      result("continue", ["api", "conflicted", ["src/a.ts"]])
    );

    expect(s.level).toBe("warning");
    expect(s.messageKey).toBe("integration.toast.conflicted");
    expect(s.needsResolution).toBe(true);
  });

  it("handles an abort", () => {
    const s = summarizeIntegration(result("abort", ["api", "aborted"]));

    expect(s.level).toBe("success");
    expect(s.messageKey).toBe("integration.toast.aborted");
  });

  it("survives a null repository list", () => {
    const s = summarizeIntegration({
      feature: "x",
      operation: "rebase",
      repositories: null,
    });

    expect(s.level).toBe("warning");
    expect(s.messageKey).toBe("integration.toast.nothing");
    expect(s.params.count).toBe(0);
  });

  it("collects the conflicted paths across repositories for the banner", () => {
    const s = summarizeIntegration(
      result(
        "rebase",
        ["api", "conflicted", ["src/a.ts"]],
        ["web", "conflicted", ["src/b.ts", "src/c.ts"]]
      )
    );

    expect(s.conflictCount).toBe(3);
  });
});
