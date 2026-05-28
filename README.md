# agentsafe

agentsafe is a multi-repository safe workspace manager for AI coding agents.

## Why

Many enterprise services are split across multiple repositories. A single feature often requires changes in backend, admin frontend, and app frontend repositories. At the same time, AI coding agents should not access secrets, local configs, or sensitive files.

agentsafe solves this by:

- managing multiple repositories as one feature workspace
- cloning registered repositories into `main/`
- creating Git worktrees under `feature/{feature-name}/`
- creating sanitized agent workspaces
- syncing agent changes back to real worktrees after review
- preparing future support for GitLab merge requests and terminal sessions

## Basic Workflow

```powershell
agentsafe init --name my-service
agentsafe repo add backend https://gitlab.example.com/company/backend.git --type backend
agentsafe repo add admin-front https://gitlab.example.com/company/admin-front.git --type frontend
agentsafe repo add app-front https://gitlab.example.com/company/app-front.git --type frontend

agentsafe clone
agentsafe feature create coupon-v2 --base develop
agentsafe agent prepare coupon-v2
agentsafe agent open coupon-v2 --editor cursor

# after AI agent work
agentsafe agent diff coupon-v2
agentsafe agent sync coupon-v2 --yes
agentsafe status coupon-v2
agentsafe commit coupon-v2 -m "feat: add coupon v2"
agentsafe push coupon-v2
```

## Commands

- `agentsafe init --name NAME [--root PATH]`
- `agentsafe repo add NAME URL --type TYPE`
- `agentsafe repo list`
- `agentsafe clone`
- `agentsafe feature create NAME --base develop`
- `agentsafe feature list`
- `agentsafe status FEATURE`
- `agentsafe agent prepare FEATURE`
- `agentsafe agent diff FEATURE [--repo NAME]`
- `agentsafe agent sync FEATURE [--repo NAME] [--dry-run] [--yes]`
- `agentsafe commit FEATURE -m "message"`
- `agentsafe push FEATURE`
- `agentsafe mr create FEATURE --target develop`

## Security defaults

`.git`, `.env`, secret key files, local application configs, build outputs and dependency folders are excluded from agent workspaces. `mask.json` supports `plain` and `regex` replacement rules for text files. Risky and masked files are blocked during sync unless explicit override flags are used.

## Git command timeout

agentsafe runs Git commands non-interactively to avoid hanging on credential prompts. The default Git timeout is 120 seconds and can be changed with:

```powershell
$env:AGENTSAFE_GIT_TIMEOUT_SECONDS = "300"
```

## Windows example

```powershell
mkdir D:\workspace\my-service
cd D:\workspace\my-service
agentsafe init --name my-service
```
