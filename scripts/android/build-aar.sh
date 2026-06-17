#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
# shellcheck source=scripts/android/lib.sh
source "$SCRIPT_DIR/lib.sh"

require_command go "Install Go and make sure 'go' is available in PATH."
GOBIN="$(go env GOPATH)/bin"
export PATH="$GOBIN:$PATH"
require_command gomobile "Install gomobile with: go install golang.org/x/mobile/cmd/gomobile@latest && gomobile init"

mkdir -p "$(dirname "$android_aar_path_abs")"

echo "Building gomobile AAR"
echo "  package: ./packages/mobilebridge"
echo "  target:  $gomobile_target"
echo "  output:  $(aar_path_for_display)"

cd "$REPO_ROOT"
if ! gomobile bind -target="$gomobile_target" -o "$android_aar_path_abs" ./packages/mobilebridge; then
  cat >&2 <<MSG

AAR build failed.
Common fixes:
  - Install gomobile: go install golang.org/x/mobile/cmd/gomobile@latest
  - Initialize gomobile once: gomobile init
  - Ensure Android SDK/NDK are installed and ANDROID_HOME or ANDROID_SDK_ROOT points to them.
  - For emulators, override target, for example:
      GOMOBILE_TARGET=android/arm64,android/amd64 ./scripts/android/build-aar.sh
MSG
  exit 1
fi

echo "AAR built: $(aar_path_for_display)"
