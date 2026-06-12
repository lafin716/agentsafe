# AGENTS.md

This file provides guidance to Coding AGENTS (claude code, codex, opencode, etc) when working with code in this repository.

## What this is

agentsafe is a multi-repository safe workspace manager for AI coding agents. It clones
several repos into one workspace, creates per-feature Git worktrees, builds *sanitized*
copies (secrets ignored, sensitive content masked) for an AI agent to edit, then syncs
reviewed changes back. Shipped as both a CLI (`agr`) and a Wails desktop app.

## Commands

- Build CLI: `make build-cli` → `go build -o agr ./apps/cli`
- Lint: `make lint` → `go vet ./...`
- Test: `go test ./...`; single package `go test ./internal/agent`; single test `go test ./internal/agent -run TestName`
- Desktop dev (hot reload): `make dev-desktop` (needs `pnpm` and the `wails` CLI; the script installs `wails` if missing)
- Desktop build: `make build-desktop` → output in `apps/desktop/build/bin`
- Frontend only (from `apps/desktop/frontend`): `pnpm dev`, `pnpm build` (`tsc && vite build`)
- Install CLI to /usr/local/bin: `./deploy-local.sh` (builds `./cmd/agentsafe`, not `./apps/cli`)

Git timeout (default 120s, agentsafe runs Git non-interactively): set `AGENTSAFE_GIT_TIMEOUT_SECONDS`.

## Architecture

**One core, two frontends.** All real logic lives in `internal/`. The two frontends are
thin binding layers that delegate to the *same* `internal/` functions so CLI and GUI behave
identically:
- CLI: `internal/app/app.go` wires Cobra commands. Entrypoints `apps/cli/main.go` and
  `cmd/agentsafe/main.go` are identical (both call `app.NewRootCommand()`); the Makefile
  builds `apps/cli`, `deploy-local.sh` builds `cmd/agentsafe`.
- Desktop: `apps/desktop/app.go` exposes methods on `App` that Wails binds to the React
  frontend (`apps/desktop/frontend/src`). Wails-generated TS bindings live in
  `frontend/wailsjs/go/main`.

> When you add a capability to `internal/`, wire it into **both** `internal/app/app.go`
> (a Cobra command) and `apps/desktop/app.go` (an `App` method) to keep CLI/GUI in parity.

**Workspace layout** (created by `config.InitWorkspace`, paths via `internal/config/workspace.go`):
- `.agentsafe/config.yaml` — workspace config; `.agentsafe/features/*.json`,
  `.agentsafe/sessions/*.json` (prepare metadata), `.agentsafe/history/` (sync rollback store)
- `main/<repo>` — full clones
- `feature/<feature>/<repo>` — Git worktrees for a feature branch
- `agent/<feature>/<repo>` — sanitized copies the AI agent edits; `*.bak-<timestamp>` are backups
- `agentsafe.yaml` — unified agent security config (see below)

**Core pipeline** (mirrors the CLI verbs): `pull` (clone/fetch into `main/`) →
`feature create` (worktrees) → `agent prepare` (sanitized copy into `agent/`) →
`agent diff` → `agent sync` (apply reviewed changes back to worktree) → `commit`/`push` →
`mr create` (GitHub/GitLab PR/MR). Packages: `internal/repo`, `internal/feature`,
`internal/agent`, `internal/forge`.

**Agent security model** (`internal/agent/security.go`, `prepare.go`): a single
`agentsafe.yaml` with `ignore` (gitignore-style copy-exclusion) and `mask` (content rules:
`plain`/`regex`/`keypath`). Config is merged from workspace root + per-repo source, plus
`cfg.Agent.DefaultExclude`. Legacy split `.agentignore` + `mask.json` are still read and
auto-migrated (`EnsureSecurityFile`). Stack presets (Spring/React/Vue/Next/Nuxt/K8s) live in
`internal/agent/templates`. Prepare records a per-file stat+hash `FileIndex` in the session
metadata so `diff`/`sync` can detect changes cheaply; risky/masked files are blocked on sync
unless `--include-risky` / `--allow-masked-sync`.

**Cross-cutting packages:**
- `internal/output` — text vs json/yaml. Every CLI command branches on `output.IsStructured()`.
  A `sink` lets the desktop app stream progress; `apps/desktop/app.go`'s `runTask` wraps
  long operations, installs the sink, and emits `task:start`/`task:log`/`task:end` events to
  the progress box.
- `internal/git` — all Git runs through `exec` with a context timeout, `GIT_TERMINAL_PROMPT=0`,
  and (on Windows) `hideWindow` to suppress console flashes. Never shell out to git directly.
- `internal/forge` — detects GitHub vs GitLab from a repo URL; creates PR/MR via API when the
  provider's env token is set, else returns a browser URL.

## Conventions

- Windows is a first-class target: `.gitattributes` pins `*.bat` to CRLF and `*.sh` to LF; keep that.
- New CLI commands must handle both text and structured output (check `output.IsStructured()`,
  emit via `output.Emit`).
- Repo/feature names and URLs are validated in `internal/config` (`ValidateRepoName`,
  `ValidateFeatureName`, `ValidateRepoURL`) — reuse these rather than re-validating.
