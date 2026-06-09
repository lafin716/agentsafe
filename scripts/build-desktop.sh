#!/usr/bin/env bash
set -euo pipefail

# Builds a distributable agentsafe desktop binary into apps/desktop/build/bin.
# Installs the Wails CLI on first run if it is missing.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
DESKTOP_DIR="${ROOT_DIR}/apps/desktop"

GOBIN="$(go env GOPATH)/bin"
export PATH="${GOBIN}:${PATH}"

if ! command -v pnpm >/dev/null 2>&1; then
  echo "Error: pnpm is required but was not found in PATH." >&2
  exit 1
fi

if ! command -v wails >/dev/null 2>&1; then
  echo "Wails CLI not found. Installing github.com/wailsapp/wails/v2/cmd/wails@latest ..."
  go install github.com/wailsapp/wails/v2/cmd/wails@latest
fi

echo "Building desktop app..."
cd "${DESKTOP_DIR}"
wails build "$@"

echo "Done. Output in ${DESKTOP_DIR}/build/bin"
