#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
# shellcheck source=scripts/android/lib.sh
source "$SCRIPT_DIR/lib.sh"

require_adb

connect_addr="${1:-${ANDROID_DEVICE_ADDR:-}}"
if [[ -z "$connect_addr" ]]; then
  read -r -p "Device IP:PORT from Wireless debugging screen: " connect_addr
fi

if [[ -z "$connect_addr" ]]; then
  die "Device address is required. Example: $0 100.64.1.23:42123"
fi

if [[ "$connect_addr" != *:* ]]; then
  die "Device address must be host:port. Got: $connect_addr"
fi

echo "Connecting to $connect_addr ..."
adb connect "$connect_addr"

echo
echo "Current adb devices:"
print_adb_devices

echo
if adb devices | awk -v serial="$connect_addr" 'NR > 1 && $1 == serial && $2 == "device" { found = 1 } END { exit found ? 0 : 1 }'; then
  echo "Connected. You can use this serial:"
  echo "  export ADB_SERIAL=$connect_addr"
else
  state="$(adb devices | awk -v serial="$connect_addr" 'NR > 1 && $1 == serial { print $2; found = 1 } END { if (!found) print "missing" }')"
  case "$state" in
    unauthorized)
      echo "Device is unauthorized. Unlock the phone and approve the debugging prompt, then retry."
      ;;
    offline)
      echo "Device is offline. Toggle Wireless debugging or reconnect with the current IP:PORT."
      ;;
    missing)
      echo "Device did not appear as '$connect_addr'. Check the IP:PORT, VPN reachability, and Wireless debugging screen."
      ;;
    *)
      echo "Device is not ready (state: $state). Check adb devices output above."
      ;;
  esac
  exit 1
fi
