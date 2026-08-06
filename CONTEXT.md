# agentsafe

agentsafe provides safe, reusable workspace structures for AI-assisted development across repositories and feature worktrees.

## Language

**Worktree Template**:
A reusable snapshot of a file or folder that can be placed into newly created or existing workspace areas.
_Avoid_: Template file, copied asset

**Template Destination**:
A logical workspace area where a Worktree Template is intended to appear, such as feature roots, agent roots, or repository areas.
_Avoid_: Folder, target folder

**Template-derived Item**:
A workspace file or folder that originated from a Worktree Template.
_Avoid_: Registered item, template copy

**Open Tool**:
An external application available from agentsafe's Open With experience for workspace items.
_Avoid_: Editor, program

**Tool Entry**:
A user-named Open Tool available on the current device.
_Avoid_: Application preset, custom program

**Commit Message Template**:
The workspace-level pattern that produces a commit message when a delivery action is run without an explicit one.
_Avoid_: Commit format, default message, message preset

**Agent Change**:
A difference between an agent workspace copy of a file and the Repo Worktree version of it, awaiting review.
_Avoid_: Change, diff entry

**Repo Worktree Change**:
A difference between a Repo Worktree file and the last commit on its branch — what the next commit would contain.
_Avoid_: Change, worktree change, git status entry, dirty file

**Agent Change Resolution**:
The act of ending an Agent Change by choosing which side wins — the agent copy or the Repo Worktree — after which the Agent Change no longer exists.
_Avoid_: Restore, apply, accept, sync one file

**Feature**:
A named unit of work that spans every configured repository, owning one branch and one Repo Worktree per repository.
_Avoid_: Worktree, branch, task

**Main Clone**:
The per-repository full clone that every Repo Worktree is derived from, and the only place a repository's integration branches are kept current.
_Avoid_: Main branch, workspace, origin

**Repo Worktree**:
The per-repository working directory where one Feature's branch is checked out, and where rebase, merge, and commit actually happen.
_Avoid_: Worktree, feature folder, feature worktree

**Base Branch**:
The branch a Feature's work is meant to be integrated into, and therefore the branch its Repo Worktrees are kept on top of.
_Avoid_: Main, upstream, parent branch

**Commit Graph**:
The commits and parent relationships held in one Main Clone's object database, which its Repo Worktrees all share. Its scope is always a single repository.
_Avoid_: History, log, tree

**Interrupted Integration**:
A rebase or merge that stopped on conflict, leaving a Repo Worktree in a partial state that must be resolved or abandoned before other work on it is meaningful.
_Avoid_: Conflict, failed rebase, broken worktree
