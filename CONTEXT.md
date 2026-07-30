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
