import type { PushRepoResult, PushResult } from "./types";
import type { IntegrationLevel } from "./integration";

// Turning a per-repository push outcome into one thing to say.
//
// This exists because the screens used to say "pushed" unconditionally: the
// backend logged each repository's failure and returned nothing, so a push
// where every repository failed still produced a success toast. Any caller that
// pushes has to go through here.

export type PushSummary = {
  pushed: PushRepoResult[];
  upToDate: PushRepoResult[];
  skipped: PushRepoResult[];
  failed: PushRepoResult[];
  level: IntegrationLevel;
  messageKey: string;
  params: { count: number; repos: string };
};

export function summarizePush(res: PushResult): PushSummary {
  const repos = res.repositories ?? [];
  const of = (...statuses: string[]) =>
    repos.filter((r) => statuses.includes(r.status));

  const pushed = of("pushed");
  const upToDate = of("up-to-date");
  const skipped = of("skipped");
  const failed = of("failed");

  const base = { pushed, upToDate, skipped, failed };
  const describe = (
    level: IntegrationLevel,
    messageKey: string,
    group: PushRepoResult[]
  ): PushSummary => ({
    ...base,
    level,
    messageKey,
    params: { count: group.length, repos: group.map((r) => r.name).join(", ") },
  });

  // A failure outranks the successes: the point of the message is to stop the
  // user believing the branch reached origin when it did not.
  if (failed.length > 0) return describe("error", "push.toast.failed", failed);
  if (pushed.length > 0) return describe("success", "push.toast.pushed", pushed);
  if (skipped.length > 0) return describe("warning", "push.toast.skipped", skipped);
  if (upToDate.length > 0)
    return describe("success", "push.toast.upToDate", upToDate);
  return describe("warning", "push.toast.nothing", []);
}
