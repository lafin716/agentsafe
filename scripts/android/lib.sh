#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

DEFAULT_ANDROID_PROJECT_DIR="apps/android"
DEFAULT_ANDROID_APP_ID="com.example.vibecoding"
DEFAULT_ANDROID_APK_PATH="apps/android/app/build/outputs/apk/debug/app-debug.apk"
DEFAULT_ANDROID_AAR_PATH="apps/android/app/libs/mobilebridge.aar"

android_project_dir_rel="${ANDROID_PROJECT_DIR:-$DEFAULT_ANDROID_PROJECT_DIR}"
android_app_id="${ANDROID_APP_ID:-$DEFAULT_ANDROID_APP_ID}"
android_gradle_task="${ANDROID_GRADLE_TASK:-assembleDebug}"

if [[ "${android_project_dir_rel}" = /* ]]; then
  android_project_dir_abs="${android_project_dir_rel}"
else
  android_project_dir_abs="$REPO_ROOT/${android_project_dir_rel}"
fi

if [[ -n "${ANDROID_APK_PATH:-}" ]]; then
  android_apk_path_rel="${ANDROID_APK_PATH}"
else
  android_apk_path_rel="$DEFAULT_ANDROID_APK_PATH"
fi

if [[ "${android_apk_path_rel}" = /* ]]; then
  android_apk_path_abs="${android_apk_path_rel}"
else
  android_apk_path_abs="$REPO_ROOT/${android_apk_path_rel}"
fi

if [[ -n "${ANDROID_AAR_PATH:-}" ]]; then
  android_aar_path_rel="${ANDROID_AAR_PATH}"
else
  android_aar_path_rel="$DEFAULT_ANDROID_AAR_PATH"
fi

if [[ "${android_aar_path_rel}" = /* ]]; then
  android_aar_path_abs="${android_aar_path_rel}"
else
  android_aar_path_abs="$REPO_ROOT/${android_aar_path_rel}"
fi

gomobile_target="${GOMOBILE_TARGET:-android/arm64}"

die() {
  echo "Error: $*" >&2
  exit 1
}

warn() {
  echo "Warning: $*" >&2
}

require_command() {
  local cmd="$1"
  local hint="${2:-Install it and make sure it is available in PATH.}"
  if ! command -v "$cmd" >/dev/null 2>&1; then
    die "Required command '$cmd' was not found. $hint"
  fi
}

require_adb() {
  require_command adb "Install Android Platform Tools and make sure 'adb' is available in PATH."
}

require_java() {
  require_command java "Install a JDK and make sure 'java' is available in PATH."
}

adb_with_serial() {
  if [[ -n "${ADB_SERIAL:-}" ]]; then
    adb -s "$ADB_SERIAL" "$@"
  else
    adb "$@"
  fi
}

print_adb_devices() {
  adb devices -l
}

count_adb_devices_in_state() {
  local state="${1:-device}"
  adb devices | awk -v state="$state" 'NR > 1 && $2 == state { count++ } END { print count + 0 }'
}

list_adb_devices_in_state() {
  local state="${1:-device}"
  adb devices | awk -v state="$state" 'NR > 1 && $2 == state { print $1 }'
}

list_adb_non_device_states() {
  adb devices | awk 'NR > 1 && NF >= 2 && $2 != "device" { print $1 "\t" $2 }'
}

resolve_adb_serial_or_fail() {
  require_adb

  if [[ -n "${ADB_SERIAL:-}" ]]; then
    return 0
  fi

  local count
  count="$(count_adb_devices_in_state device)"

  if [[ "$count" -eq 0 ]]; then
    print_adb_devices >&2 || true
    local non_devices
    non_devices="$(list_adb_non_device_states || true)"
    if [[ -n "$non_devices" ]]; then
      echo >&2
      echo "Found adb entries that are not ready:" >&2
      echo "$non_devices" >&2
      echo "If the device is unauthorized, approve the debugging prompt on the phone." >&2
      echo "If the device is offline, reconnect with scripts/android/adb-connect.sh <ip:port>." >&2
    else
      echo >&2
      echo "No adb device in 'device' state was found." >&2
      echo "Connect first with: scripts/android/adb-connect.sh <device-ip:device-port>" >&2
    fi
    exit 1
  fi

  if [[ "$count" -gt 1 ]]; then
    print_adb_devices >&2
    echo >&2
    echo "Multiple adb devices are connected. Choose one and set ADB_SERIAL:" >&2
    while IFS= read -r serial; do
      [[ -n "$serial" ]] && echo "  export ADB_SERIAL=$serial" >&2
    done < <(list_adb_devices_in_state device)
    exit 1
  fi
}

selected_adb_serial() {
  if [[ -n "${ADB_SERIAL:-}" ]]; then
    echo "$ADB_SERIAL"
  else
    list_adb_devices_in_state device | head -n 1
  fi
}

adb_shell() {
  adb_with_serial shell "$@"
}

apk_path_for_display() {
  if [[ -n "${ANDROID_APK_PATH:-}" ]]; then
    echo "$ANDROID_APK_PATH"
  else
    echo "$DEFAULT_ANDROID_APK_PATH"
  fi
}

aar_path_for_display() {
  if [[ -n "${ANDROID_AAR_PATH:-}" ]]; then
    echo "$ANDROID_AAR_PATH"
  else
    echo "$DEFAULT_ANDROID_AAR_PATH"
  fi
}

gradle_executable() {
  local project_gradlew="$android_project_dir_abs/gradlew"
  local root_gradlew="$REPO_ROOT/gradlew"

  if [[ -x "$project_gradlew" ]]; then
    echo "$project_gradlew"
    return 0
  fi

  if [[ -f "$project_gradlew" ]]; then
    echo "$project_gradlew"
    return 0
  fi

  if [[ -x "$root_gradlew" ]]; then
    echo "$root_gradlew"
    return 0
  fi

  if [[ -f "$root_gradlew" ]]; then
    echo "$root_gradlew"
    return 0
  fi

  return 1
}

gradle_workdir_for() {
  local gradlew="$1"
  if [[ "$gradlew" == "$REPO_ROOT/gradlew" ]]; then
    echo "$REPO_ROOT"
  else
    echo "$android_project_dir_abs"
  fi
}

run_gradle_wrapper() {
  local gradlew="$1"
  local task="$2"

  if [[ -x "$gradlew" ]]; then
    "$gradlew" "$task"
  else
    bash "$gradlew" "$task"
  fi
}

run_logcat_for_pid() {
  local pid="$1"
  local grep_keyword="${2:-}"

  if [[ -n "$grep_keyword" ]]; then
    set +e
    adb_with_serial logcat --pid="$pid" | grep "$grep_keyword"
    local status=${PIPESTATUS[0]}
    set -e
    return "$status"
  fi

  adb_with_serial logcat --pid="$pid"
}

run_logcat_grep_package() {
  local package_name="$1"
  local grep_keyword="${2:-}"
  local pattern="$package_name"

  if [[ -n "$grep_keyword" ]]; then
    pattern="$grep_keyword"
  fi

  set +e
  adb_with_serial logcat | grep "$pattern"
  local status=${PIPESTATUS[0]}
  set -e
  return "$status"
}
