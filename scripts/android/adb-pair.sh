#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
# shellcheck source=scripts/android/lib.sh
source "$SCRIPT_DIR/lib.sh"

require_adb

pair_addr="${1:-}"
if [[ -z "$pair_addr" ]]; then
  read -r -p "Pairing IP:PORT from phone (for example 100.64.1.23:37123): " pair_addr
fi

if [[ -z "$pair_addr" ]]; then
  die "Pairing address is required. Example: $0 100.64.1.23:37123"
fi

if [[ "$pair_addr" != *:* ]]; then
  die "Pairing address must be host:port. Got: $pair_addr"
fi

echo "Pairing with $pair_addr ..."
echo "When adb asks for it, enter the pairing code shown on the phone."
adb pair "$pair_addr"

cat <<MSG

Pair command finished.
Next steps:
  1. On the phone, keep Wireless debugging open and find the normal connection IP:PORT.
  2. Note that the pairing port and connection port can be different.
  3. Run:
     scripts/android/adb-connect.sh <device-ip:device-port>
MSG
