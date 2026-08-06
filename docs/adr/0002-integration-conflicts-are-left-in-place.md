# 2. Integration conflicts are left in place

Date: 2026-08-03

## Status

Accepted

## Context

`feature.Rebase` used to abort on conflict:

```go
if err := aggit.RebaseOnto(p, upstream); err != nil {
    _ = aggit.RebaseAbort(p)
    rr.Status = "failed"
    rr.Detail = "rebase onto %s failed (conflict); aborted, resolve manually"
}
```

That was defensible while rebase was a single button on a status screen: the
Repo Worktree is left exactly as it was, and no other part of the app has to
understand a half-finished rebase.

It stops being defensible once a Commit Graph page makes rebase and merge the
primary way to move branches around. Most interesting rebases conflict. A
feature that gives up on every conflict and tells the user to start over in a
terminal is a feature that only works on the cases the user did not need help
with.

Leaving the conflict in place has its own cost, and it is not confined to the
new page. A Repo Worktree mid-rebase has a detached HEAD, so
`git.CurrentBranch` returns an empty string and the existing status screens
report a Feature with no branch. Nothing detects the state, so the app would
quietly show wrong information rather than explain it.

We considered keeping both behaviours — abort on the old paths, leave the
conflict on the new page. Same Feature, same repository, same conflict, two
different outcomes depending on which button was pressed. That is not
explainable to a user.

## Decision

Rebase and merge leave a conflicted Repo Worktree in its conflicted state. The
new domain term for it is an Interrupted Integration.

This is the default everywhere: the graph page, the status screen's rebase
button, and `agr feature rebase`. The old behaviour stays reachable as opt-in,
`--abort-on-conflict` on the CLI and the equivalent option on the Go API, for
scripted use where an untouched worktree matters more than progress.

Because the state is now reachable, it has to be legible. `internal/git` gains
detection for it, resolved from the filesystem rather than by shelling out, so
the check is cheap enough to include in the per-repository status that the
Features screens already load on every render.

`RebaseRepoResult.Status` gains `conflicted` alongside `rebased`,
`up-to-date`, `skipped`, and `failed`. `failed` now means what its name says —
the operation could not run — instead of doubling as "conflicted".

## Consequences

Every consumer of a Repo Worktree's state has to tolerate an Interrupted
Integration, and say so rather than rendering an empty branch name. The
resolution controls (continue, abort, open a terminal) appear wherever the
state is surfaced, so a user who hits a conflict from one screen can finish
from another.

Conflict resolution itself stays outside the app. agentsafe reports which files
conflict and where; the user edits them in a terminal or editor. Building a
three-way merge editor is a separate piece of work, and this decision does not
depend on it.

An Interrupted Integration blocks agent workspace operations for that
repository, because the working tree does not correspond to any commit. That is
a real narrowing of what the user can do while a conflict is open, and it is
the price of not silently destroying rebase progress.
