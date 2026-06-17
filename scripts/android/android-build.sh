#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
# shellcheck source=scripts/android/lib.sh
source "$SCRIPT_DIR/lib.sh"

require_java

if [[ "${ANDROID_SKIP_AAR_BUILD:-0}" != "1" ]]; then
  echo "Building Go mobile bridge AAR ..."
  "$SCRIPT_DIR/build-aar.sh"
  echo
fi

if [[ ! -d "$android_project_dir_abs" ]]; then
  die "Android project not found at ${ANDROID_PROJECT_DIR:-$DEFAULT_ANDROID_PROJECT_DIR}. Create Android app first or set ANDROID_PROJECT_DIR."
fi

if ! gradlew="$(gradle_executable)"; then
  die "Gradle wrapper not found. Expected $android_project_dir_rel/gradlew or ./gradlew. Create Android app first or add a Gradle wrapper."
fi

if [[ ! -x "$gradlew" ]]; then
  warn "Gradle wrapper is not executable: $gradlew"
  warn "Run: chmod +x $gradlew"
fi

echo "Android project: $android_project_dir_abs"
echo "Gradle wrapper: $gradlew"
echo "Gradle task: $android_gradle_task"

gradle_workdir="$(gradle_workdir_for "$gradlew")"
echo "Gradle workdir: $gradle_workdir"

cd "$gradle_workdir"
if ! run_gradle_wrapper "$gradlew" "$android_gradle_task"; then
  cat >&2 <<MSG

Android build failed.
Check that the Android SDK/JDK are installed and configured for this project.
Common fixes: install Android Studio command-line tools, set ANDROID_HOME or ANDROID_SDK_ROOT if your Gradle setup requires it, and accept SDK licenses.
MSG
  exit 1
fi

echo
echo "Build finished. Expected APK: $(apk_path_for_display)"
if [[ -n "${ANDROID_APK_PATH:-}" ]]; then
  echo "ANDROID_APK_PATH override is active."
fi
