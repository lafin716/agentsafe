import { describe, expect, it } from "vitest";
import { summarizePush } from "./push";
import type { PushRepoResult, PushResult } from "./types";

function repo(
  name: string,
  status: string,
  extra: Partial<PushRepoResult> = {}
): PushRepoResult {
  return {
    name,
    branch: "feature/x",
    status,
    detail: "",
    forced: false,
    ...extra,
  };
}

function result(...repositories: PushRepoResult[]): PushResult {
  return { feature: "coupon-v2", repositories };
}

describe("summarizePush", () => {
  it("reports a failure even when other repositories succeeded", () => {
    // The bug this replaces: the backend logged each repository's failure and
    // returned nil, so the UI said "pushed" regardless.
    const summary = summarizePush(
      result(repo("api", "pushed"), repo("web", "failed"))
    );

    expect(summary.level).toBe("error");
    expect(summary.messageKey).toBe("push.toast.failed");
    expect(summary.params).toEqual({ count: 1, repos: "web" });
  });

  it("reports failure when every repository failed", () => {
    const summary = summarizePush(
      result(repo("api", "failed"), repo("web", "failed"))
    );

    expect(summary.level).toBe("error");
    expect(summary.params.count).toBe(2);
    expect(summary.params.repos).toBe("api, web");
  });

  it("reports success when repositories were pushed", () => {
    const summary = summarizePush(
      result(repo("api", "pushed"), repo("web", "up-to-date"))
    );

    expect(summary.level).toBe("success");
    expect(summary.messageKey).toBe("push.toast.pushed");
    expect(summary.params).toEqual({ count: 1, repos: "api" });
  });

  it("warns when everything was skipped", () => {
    // Skipped means a precondition refused it — an open Interrupted
    // Integration, most often — which the user has to act on.
    const summary = summarizePush(
      result(repo("api", "skipped"), repo("web", "skipped"))
    );

    expect(summary.level).toBe("warning");
    expect(summary.messageKey).toBe("push.toast.skipped");
  });

  it("treats an all-up-to-date push as success", () => {
    const summary = summarizePush(result(repo("api", "up-to-date")));

    expect(summary.level).toBe("success");
    expect(summary.messageKey).toBe("push.toast.upToDate");
  });

  it("handles an empty repository list", () => {
    const summary = summarizePush({ feature: "x", repositories: null });

    expect(summary.messageKey).toBe("push.toast.nothing");
    expect(summary.failed).toEqual([]);
  });

  it("groups every repository by status", () => {
    const summary = summarizePush(
      result(
        repo("a", "pushed"),
        repo("b", "up-to-date"),
        repo("c", "skipped"),
        repo("d", "failed", { error: "stale info", gitOutput: "$ git push\n..." })
      )
    );

    expect(summary.pushed.map((r) => r.name)).toEqual(["a"]);
    expect(summary.upToDate.map((r) => r.name)).toEqual(["b"]);
    expect(summary.skipped.map((r) => r.name)).toEqual(["c"]);
    expect(summary.failed.map((r) => r.name)).toEqual(["d"]);
    // The raw git output survives, which is where a refused lease says so.
    expect(summary.failed[0].gitOutput).toContain("git push");
  });
});
