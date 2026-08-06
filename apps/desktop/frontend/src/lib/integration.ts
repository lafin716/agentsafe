import type { IntegrationRepoResult, IntegrationResult } from "./types";

// Classifying a rebase or merge outcome, shared by the commit graph and the
// worktree status screen so the same operation reads the same either way.
//
// The statuses are not a severity ladder. A conflict is the most important thing
// to say — an Interrupted Integration is open and the user has to finish it — but
// it is not a failure: the work is staged and waiting. "failed" now means only
// that the operation could not run, and "skipped" means a precondition refused
// it, most often unreviewed Agent Changes.

export type IntegrationLevel = "success" | "warning" | "error";

export type IntegrationSummary = {
  operation: string;
  succeeded: IntegrationRepoResult[];
  upToDate: IntegrationRepoResult[];
  conflicted: IntegrationRepoResult[];
  blocked: IntegrationRepoResult[];
  failed: IntegrationRepoResult[];
  /** Total unmerged paths across every conflicted repository. */
  conflictCount: number;
  /** True when an Interrupted Integration is now open and awaiting the user. */
  needsResolution: boolean;
  level: IntegrationLevel;
  messageKey: string;
  params: { count: number; repos: string };
};

const SUCCESS_KEY: Record<string, string> = {
  rebased: "integration.toast.rebased",
  merged: "integration.toast.merged",
  continued: "integration.toast.continued",
  aborted: "integration.toast.aborted",
};

export function summarizeIntegration(
  res: IntegrationResult
): IntegrationSummary {
  const repos = res.repositories ?? [];
  const of = (...statuses: string[]) =>
    repos.filter((r) => statuses.includes(r.status));

  const succeeded = of("rebased", "merged", "continued", "aborted");
  const upToDate = of("up-to-date");
  const conflicted = of("conflicted");
  const blocked = of("skipped");
  const failed = of("failed");
  const conflictCount = conflicted.reduce(
    (total, r) => total + (r.conflicts?.length ?? 0),
    0
  );

  const summary: IntegrationSummary = {
    operation: res.operation,
    succeeded,
    upToDate,
    conflicted,
    blocked,
    failed,
    conflictCount,
    needsResolution: conflicted.length > 0,
    level: "success",
    messageKey: "integration.toast.nothing",
    params: { count: 0, repos: "" },
  };

  const describe = (
    level: IntegrationLevel,
    messageKey: string,
    group: IntegrationRepoResult[]
  ): IntegrationSummary => ({
    ...summary,
    level,
    messageKey,
    params: {
      count: group.length,
      repos: group.map((r) => r.name).join(", "),
    },
  });

  // A conflict outranks a failure: it is the one outcome with a next step.
  if (conflicted.length > 0) {
    return describe("warning", "integration.toast.conflicted", conflicted);
  }
  if (failed.length > 0) {
    return describe("error", "integration.toast.failed", failed);
  }
  if (blocked.length > 0) {
    return describe("warning", "integration.toast.skipped", blocked);
  }
  if (succeeded.length > 0) {
    const key = SUCCESS_KEY[succeeded[0].status] ?? "integration.toast.rebased";
    return describe("success", key, succeeded);
  }
  if (upToDate.length > 0) {
    return describe("success", "integration.toast.upToDate", upToDate);
  }
  return describe("warning", "integration.toast.nothing", []);
}
