#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
# shellcheck source=scripts/android/lib.sh
source "$SCRIPT_DIR/lib.sh"

resolve_adb_serial_or_fail

if [[ ! -f "$android_apk_path_abs" ]]; then
  die "APK not found at $(apk_path_for_display). Run scripts/android/android-build.sh first or set ANDROID_APK_PATH."
fi

echo "Installing APK: $android_apk_path_abs"
if [[ -n "${ADB_SERIAL:-}" ]]; then
  echo "Using ADB_SERIAL=$ADB_SERIAL"
fi

if ! adb_with_serial install -r "$android_apk_path_abs"; then
  die "adb install failed. Check APK compatibility, device authorization, available storage, and adb connection."
fi

echo "Installed: $(apk_path_for_display)"
