import { Badge } from "@/components/ui/badge";
import { useI18n } from "@/i18n/I18nProvider";
import type { GraphCommit } from "@/lib/types";

// One rendering of "here are some commits", shared by every place that shows a
// list of them: the unpushed-commit popup on the delivery screen, a repository's
// commit list on the workspace screen, and a Repo Worktree's commit list on the
// status screen. They differ only in where the commits came from.

export function shortSHA(sha: string): string {
  return sha.slice(0, 7);
}

/** Renders an ISO author date as a local, minute-precision timestamp. */
export function commitDate(iso: string): string {
  const at = new Date(iso);
  return Number.isNaN(at.getTime()) ? iso : at.toLocaleString();
}

export function CommitList({
  commits,
  emptyLabel,
  onSelect,
}: {
  commits: GraphCommit[];
  emptyLabel: string;
  /** Set to make rows clickable, e.g. to show which files a commit touched. */
  onSelect?: (commit: GraphCommit) => void;
}) {
  const { t } = useI18n();

  if (commits.length === 0) {
    return <p className="p-4 text-sm text-muted-foreground">{emptyLabel}</p>;
  }

  return (
    <ul className="divide-y">
      {commits.map((c) => {
        const body = (
          <>
            <div className="flex min-w-0 items-baseline gap-2">
              <span className="shrink-0 font-mono text-xs text-muted-foreground">
                {shortSHA(c.sha)}
              </span>
              <span className="truncate text-sm">{c.subject}</span>
            </div>
            <div className="mt-0.5 flex flex-wrap items-center gap-2 pl-[4.25rem] text-xs text-muted-foreground">
              <span className="truncate">{c.authorName}</span>
              <span>·</span>
              <span>{commitDate(c.authorDate)}</span>
              {(c.refs ?? []).map((ref) => (
                <Badge
                  key={ref.kind + ref.name}
                  variant={ref.kind === "remote" ? "outline" : "secondary"}
                  className="font-mono text-[10px]"
                >
                  {ref.name}
                </Badge>
              ))}
              {c.parents.length > 1 && (
                <Badge variant="outline" className="text-[10px]">
                  {t("commits.merge")}
                </Badge>
              )}
            </div>
          </>
        );
        return (
          <li key={c.sha}>
            {onSelect ? (
              <button
                type="button"
                className="w-full px-4 py-2 text-left hover:bg-accent/50"
                onClick={() => onSelect(c)}
              >
                {body}
              </button>
            ) : (
              <div className="px-4 py-2">{body}</div>
            )}
          </li>
        );
      })}
    </ul>
  );
}
