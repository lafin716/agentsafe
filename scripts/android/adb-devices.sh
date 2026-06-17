#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
# shellcheck source=scripts/android/lib.sh
source "$SCRIPT_DIR/lib.sh"

require_adb

print_adb_devices

count="$(count_adb_devices_in_state device)"

echo
if [[ "$count" -eq 0 ]]; then
  echo "No adb device in 'device' state was found."
  non_devices="$(list_adb_non_device_states || true)"
  if [[ -n "$non_devices" ]]; then
    echo "Non-ready devices:"
    echo "$non_devices"
    echo "Approve unauthorized devices on the phone or reconnect offline devices."
  else
    echo "Connect a wireless device first:"
    echo "  scripts/android/adb-connect.sh <device-ip:device-port>"
  fi
elif [[ "$count" -eq 1 ]]; then
  serial="$(selected_adb_serial)"
  echo "One adb device is ready: $serial"
  echo "Optional: export ADB_SERIAL=$serial"
else
  echo "Multiple adb devices are ready. Set ADB_SERIAL before install/run/logcat:"
  while IFS= read -r serial; do
    [[ -n "$serial" ]] && echo "  export ADB_SERIAL=$serial"
  done < <(list_adb_devices_in_state device)
fi
