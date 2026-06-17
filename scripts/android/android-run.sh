#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
# shellcheck source=scripts/android/lib.sh
source "$SCRIPT_DIR/lib.sh"

resolve_adb_serial_or_fail

app_id="${ANDROID_APP_ID:-$DEFAULT_ANDROID_APP_ID}"

echo "Launching Android app: $app_id"

if adb_shell monkey -p "$app_id" -c android.intent.category.LAUNCHER 1; then
  echo "Launched: $app_id"
  exit 0
fi

warn "monkey launch failed. Trying resolve-activity/am start fallback."

activity="$(adb_shell cmd package resolve-activity --brief "$app_id" 2>/dev/null | tr -d '\r' | awk 'NF { last = $0 } END { print last }' || true)"

if [[ -n "$activity" && "$activity" == */* ]]; then
  echo "Resolved activity: $activity"
  if adb_shell am start -n "$activity"; then
    echo "Launched: $app_id"
    exit 0
  fi
fi

die "Failed to launch $app_id. Check ANDROID_APP_ID, installation status, and launcher activity configuration."
