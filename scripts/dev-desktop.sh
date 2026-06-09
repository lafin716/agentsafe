#!/usr/bin/env bash
set -euo pipefail

# Runs the agentsafe desktop app in development mode (hot reload).
# Installs the Wails CLI on first run if it is missing.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
DESKTOP_DIR="${ROOT_DIR}/apps/desktop"

GOBIN="$(go env GOPATH)/bin"
export PATH="${GOBIN}:${PATH}"

if ! command -v wails >/dev/null 2>&1; then
  echo "Wails CLI not found. Installing github.com/wailsapp/wails/v2/cmd/wails@latest ..."
  go install github.com/wailsapp/wails/v2/cmd/wails@latest
fi

if ! command -v pnpm >/dev/null 2>&1; then
  echo "Error: pnpm is required but was not found in PATH." >&2
  exit 1
fi

echo "Starting desktop app in dev mode..."
cd "${DESKTOP_DIR}"
exec wails dev
