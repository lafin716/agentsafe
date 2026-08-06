# 1. The Main Clone is not a merge target

Date: 2026-08-03

## Status

Accepted

## Context

The Commit Graph page lets a user act on branches directly, and the obvious
thing to reach for in a graph is "merge this branch into main". Two facts make
that destructive here.

`internal/repo/pull.go` updates a Main Clone with `git pull --ff-only origin
<branch>` and treats a non-fast-forward as an error the user has to see:

```go
if err := run(dest, "pull", "--ff-only", "origin", branch); err != nil {
    return fmt.Errorf("fetched, but pull failed: %w", err)
}
```

A single local merge into the Main Clone's Base Branch makes it diverge from
`origin`. Every later Pull for that repository then fails, permanently, until
somebody resets the branch by hand. Nothing in the product tells the user that
is what happened, or offers to undo it.

Second, every Repo Worktree is created from the Main Clone with `git worktree
add`, and new Features branch off its Base Branch. A diverged Main Clone
silently becomes the base of all future Features.

agentsafe already has an answer for integrating a Feature: `internal/forge`
opens a GitHub PR or GitLab MR. Local integration would be a competing,
lossier path to the same goal.

We considered allowing feature→main merges behind a warning dialog, and
allowing them after first making the Main Clone recoverable (relaxing the
`--ff-only` policy, adding a "restore local main" command, surfacing divergence
in the UI). The first is a warning nobody reads guarding an unrecoverable
state. The second is a larger change to the pull contract than the graph
feature warrants, and it weakens an invariant that currently holds everywhere.

## Decision

The Main Clone is read-only with respect to history. Nothing in agentsafe
commits, merges, or rebases in `main/<repo>`.

Consequently "merge" in the product means one direction only: merging a Base
Branch into a Feature's branch, executed inside that Feature's Repo Worktree.
It is offered as the alternative to rebase for branches that have already been
pushed.

Integrating a Feature into its Base Branch stays with `internal/forge` as a PR
or MR.

## Consequences

The `--ff-only` pull contract keeps holding, so Pull cannot be broken from the
UI and a Main Clone always matches `origin` or is strictly behind it.

Users who want a local integration branch cannot get one through agentsafe.
They can do it in a terminal, and they own the fallout.

The graph's branch context menu is asymmetric in a way that needs explaining in
the UI: a Base Branch offers "rebase this Feature onto here" and "merge here
into this Feature", never the reverse. The one Main Clone operation that
remains is switching which branch it has checked out (`CheckoutRepoBranch`),
which moves a pointer and writes no history.
