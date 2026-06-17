#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
# shellcheck source=scripts/android/lib.sh
source "$SCRIPT_DIR/lib.sh"

show_log=0
clear_log=0
log_args=()

while [[ $# -gt 0 ]]; do
  case "$1" in
    --log)
      show_log=1
      shift
      ;;
    --clear-log)
      clear_log=1
      shift
      ;;
    --grep)
      if [[ $# -lt 2 ]]; then
        die "--grep requires a keyword."
      fi
      log_args+=(--grep "$2")
      show_log=1
      shift 2
      ;;
    -h|--help)
      cat <<HELP
Usage: $0 [--clear-log] [--log] [--grep <keyword>]

Runs AAR build -> Android APK build -> install -> run, then optionally streams logcat.
HELP
      exit 0
      ;;
    *)
      die "Unknown argument: $1"
      ;;
  esac
done

total=4
if [[ "$show_log" -eq 1 ]]; then
  total=5
fi

echo "[1/$total] Build AAR"
"$SCRIPT_DIR/build-aar.sh"

echo
echo "[2/$total] Build Android APK"
ANDROID_SKIP_AAR_BUILD=1 "$SCRIPT_DIR/android-build.sh"

echo
echo "[3/$total] Install"
"$SCRIPT_DIR/android-install.sh"

if [[ "$clear_log" -eq 1 ]]; then
  echo
  echo "Clearing logcat before run ..."
  resolve_adb_serial_or_fail
  adb_with_serial logcat -c
fi

echo
echo "[4/$total] Run"
"$SCRIPT_DIR/android-run.sh"

if [[ "$show_log" -eq 1 ]]; then
  echo
  echo "[5/$total] Logcat"
  "$SCRIPT_DIR/android-logcat.sh" "${log_args[@]}"
fi
