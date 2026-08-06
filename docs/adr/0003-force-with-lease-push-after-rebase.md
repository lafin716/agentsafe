# 3. A rebase offers to push with an explicit lease

Date: 2026-08-04

## Status

Accepted

## Context

Rebasing a Feature's branch rewrites its commits. If the branch has been pushed,
`git push` then fails as a non-fast-forward, and the rebase the user just ran
sits on their machine looking done while `origin` still has the old history. The
status screen even agrees it is done: `unpushedCount` compares commit *counts*,
and a rebase does not change how many commits there are, so the branch reads as
up to date when it is not.

So the rebase dialog offers to push afterwards, checked by default. That raises
two questions: how the push is forced, and which repositories it runs for.

**How.** `--force-with-lease` refuses the push when the remote branch has moved
since the last fetch, which is the difference between overwriting your own
outdated commits and overwriting somebody else's work. Used without an argument,
it decides what to compare against from the branch's remote-tracking ref — and
in agentsafe that ref is not reliably the branch's own:

```go
// configureWorktreeUpstream, internal/feature/feature.go
targetBranch := base
if trackFeatureBranch {
    targetBranch = branch
}
```

A newly created Feature branch tracks its **Base Branch** until its first push.
A lease resolved from that upstream is leasing against the wrong ref. Depending
on git version and configuration the result is either a refusal that looks
arbitrary or, worse, a lease that passes because it compared against something
unrelated.

**Which.** A rebase run across a Feature touches every repository, and they do
not all end in the same state: some are rebased, some were already up to date,
some are skipped for uncommitted changes or unreviewed Agent Changes, and some
stop on a conflict as an Interrupted Integration (docs/adr/0002). Only a
repository whose history was actually rewritten needs forcing.

We also considered leaving the push entirely manual. That is what exists today,
and it is what produces the "rebased but not pushed, and nothing says so" state
described above.

## Decision

After a rebase, agentsafe offers to push, defaulting to on.

The push runs **only for repositories whose status is `rebased`**.
`up-to-date`, `skipped`, `conflicted` and `failed` are not pushed. In
particular, pushing a `conflicted` repository would publish a half-finished
rebase.

The lease is always **explicit**. Immediately before pushing, agentsafe reads
what `origin/<branch>` points at and passes that value:

```
git push --force-with-lease=<branch>:<sha> -u origin <branch>
```

When `origin` has no such branch there is nothing to overwrite, so it is a plain
`git push -u origin <branch>` with no force at all.

**A refused lease is never retried with `--force`.** A refusal means the remote
branch moved after the fetch this operation just did — someone else pushed. The
user is told that, and fetches and looks before deciding.

The forced push also skips the "nothing to push" shortcut. That check is a
commit count, and after a rebase the count is unchanged while the commits
themselves are different, so trusting it would silently skip the one push that
was needed.

## Consequences

`internal/git.PushWithLease` takes the expected SHA as a parameter, and
`pushWithLeaseArgs` is a pure function so the argument construction — where the
whole safety property lives — is covered by tests that do not need a remote.

Reading `origin/<branch>` costs one extra `rev-parse` per forced repository.
That is one subprocess on an operation that has already run a fetch and a
rebase.

There is a window between reading the SHA and pushing in which someone else can
push. It is smaller than the window the argument-less form uses (which relies on
whenever the tracking ref was last updated), and the consequence of losing the
race is a refused push, not a lost commit.

A user who genuinely needs to overwrite a remote branch that moved has to do it
in a terminal. That is deliberate: agentsafe has no way to tell that case apart
from the one where a colleague's commits are about to be destroyed.
