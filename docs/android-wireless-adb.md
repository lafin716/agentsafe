# Android Wireless ADB Development Loop

This repo supports a phone-only testing loop through a MacBook reachable over VPN:

```text
phone -> VPN -> MacBook SSH -> build-aar -> Gradle build -> adb install -> adb run -> logcat
```

The Android app is under `apps/android`. It calls Go logic through a gomobile AAR generated from `packages/mobilebridge`.

## Phone setup

On the Android phone:

1. Enable Developer options.
2. Enable USB debugging.
3. Enable Wireless debugging.
4. Open **Wireless debugging**.
5. Select **Pair device with pairing code** for the first pairing step.

## First pairing

Use the pairing IP and pairing port shown on the phone:

```bash
./scripts/android/adb-pair.sh <pair-ip:pair-port>
# example
./scripts/android/adb-pair.sh 100.64.1.23:37123
```

`adb pair` asks for the pairing code. Enter the code shown on the phone.

## Normal wireless connection

After pairing, use the normal IP and port shown on the Wireless debugging screen:

```bash
./scripts/android/adb-connect.sh <device-ip:device-port>
# or
ANDROID_DEVICE_ADDR=100.64.1.23:42123 ./scripts/android/adb-connect.sh
```

Check connected devices:

```bash
./scripts/android/adb-devices.sh
```

If multiple devices are connected, set `ADB_SERIAL`:

```bash
export ADB_SERIAL=100.64.1.23:42123
```

## Pairing port vs connect port

The pairing port and connect port can be different.

- Pairing port appears after tapping **Pair device with pairing code**.
- Connect port appears on the main **Wireless debugging** screen.
- Ports can change when Wireless debugging is turned off or the network changes.

## VPN notes

When phone and MacBook are on the same VPN, the MacBook must be able to reach the phone's VPN IP and wireless ADB port.

Common blockers:

- VPN client isolation
- firewall rules
- stale wireless debugging port
- phone switching networks
- Wireless debugging screen closed before reading the current address

## Build AAR

```bash
./scripts/android/build-aar.sh
```

Defaults:

```text
GOMOBILE_TARGET=android/arm64
ANDROID_AAR_PATH=apps/android/app/libs/mobilebridge.aar
```

For emulator support:

```bash
GOMOBILE_TARGET=android/arm64,android/amd64 ./scripts/android/build-aar.sh
```

## Build APK

```bash
./scripts/android/android-build.sh
```

This builds the AAR first, then runs Gradle `assembleDebug`.

Default APK path:

```text
apps/android/app/build/outputs/apk/debug/app-debug.apk
```

## Install

```bash
./scripts/android/android-install.sh
ADB_SERIAL=100.64.1.23:42123 ./scripts/android/android-install.sh
```

## Run

```bash
ANDROID_APP_ID=com.example.vibecoding ./scripts/android/android-run.sh
```

## Full development loop

Build AAR, build APK, install, and run:

```bash
ANDROID_APP_ID=com.example.vibecoding ./scripts/android/android-dev-loop.sh
```

Build, install, run, clear logs, then stream logs:

```bash
ANDROID_APP_ID=com.example.vibecoding ADB_SERIAL=100.64.1.23:42123 \
  ./scripts/android/android-dev-loop.sh --clear-log --log
```

Filter logs:

```bash
ANDROID_APP_ID=com.example.vibecoding ./scripts/android/android-dev-loop.sh --log --grep VibeCodingAndroid
```

## Logs

```bash
ANDROID_APP_ID=com.example.vibecoding ./scripts/android/android-logcat.sh
ANDROID_APP_ID=com.example.vibecoding ./scripts/android/android-logcat.sh --clear
ANDROID_APP_ID=com.example.vibecoding ./scripts/android/android-logcat.sh --grep VibeCodingAndroid
```

The script first finds the app PID with `adb shell pidof "$ANDROID_APP_ID"` and then uses `adb logcat --pid=<pid>`. If the app is not running, it falls back to full logcat filtered by package name or your `--grep` keyword.

## Environment variables

| Variable | Default | Purpose |
| --- | --- | --- |
| `ANDROID_PROJECT_DIR` | `apps/android` | Android project directory. Relative paths are resolved from repo root. |
| `ANDROID_GRADLE_TASK` | `assembleDebug` | Gradle task used by `android-build.sh`. |
| `ANDROID_APK_PATH` | `apps/android/app/build/outputs/apk/debug/app-debug.apk` | APK to install. Relative paths are resolved from repo root. |
| `ANDROID_AAR_PATH` | `apps/android/app/libs/mobilebridge.aar` | gomobile AAR output path. |
| `ANDROID_APP_ID` | `com.example.vibecoding` | Android package/applicationId for run and logcat. |
| `ANDROID_DEVICE_ADDR` | unset | Default `<ip>:<port>` for `adb-connect.sh`. |
| `ADB_SERIAL` | unset | adb serial to use when multiple devices are connected. |
| `GOMOBILE_TARGET` | `android/arm64` | gomobile target ABI list. |

## Recommended aliases

```bash
alias arun='ANDROID_APP_ID=com.example.vibecoding ./scripts/android/android-dev-loop.sh'
alias alog='ANDROID_APP_ID=com.example.vibecoding ./scripts/android/android-logcat.sh'
```

## Executable bit

The scripts are executable. If your checkout loses the bit:

```bash
chmod +x scripts/android/*.sh
```

## Security notes

- Enable Wireless debugging only while developing.
- Do not use Wireless debugging on public Wi-Fi.
- Prefer access only through your private VPN.
- Turn Wireless debugging off when done.
