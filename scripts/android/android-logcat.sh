#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
# shellcheck source=scripts/android/lib.sh
source "$SCRIPT_DIR/lib.sh"

clear_log=0
grep_keyword=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --clear)
      clear_log=1
      shift
      ;;
    --grep)
      if [[ $# -lt 2 ]]; then
        die "--grep requires a keyword."
      fi
      grep_keyword="$2"
      shift 2
      ;;
    -h|--help)
      cat <<HELP
Usage: $0 [--clear] [--grep <keyword>]

Environment:
  ANDROID_APP_ID   Package/applicationId to inspect. Default: $DEFAULT_ANDROID_APP_ID
  ADB_SERIAL       Optional adb device serial when multiple devices are connected.
HELP
      exit 0
      ;;
    *)
      die "Unknown argument: $1"
      ;;
  esac
done

resolve_adb_serial_or_fail

app_id="${ANDROID_APP_ID:-$DEFAULT_ANDROID_APP_ID}"

if [[ "$clear_log" -eq 1 ]]; then
  echo "Clearing logcat ..."
  adb_with_serial logcat -c
fi

pid="$(adb_shell pidof "$app_id" 2>/dev/null | tr -d '\r' | awk 'NF { print $1; exit }' || true)"

if [[ -n "$pid" ]]; then
  echo "Showing logcat for $app_id (pid: $pid)"
  if [[ -n "$grep_keyword" ]]; then
    echo "Filtering with grep: $grep_keyword"
  fi
  run_logcat_for_pid "$pid" "$grep_keyword"
else
  cat >&2 <<MSG
App '$app_id' is not currently running, so adb logcat --pid cannot be used.
Start it first with:
  ANDROID_APP_ID=$app_id scripts/android/android-run.sh

Falling back to full logcat filtered by ${grep_keyword:-package name}. Press Ctrl-C to stop.
MSG
  run_logcat_grep_package "$app_id" "$grep_keyword"
fi
