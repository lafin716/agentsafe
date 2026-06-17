# Android App Architecture

This repo keeps the existing Go CLI and Wails desktop app, and adds Android as a separate native app under `apps/android`.

The Android app does **not** run the Go CLI binary with `exec`. Instead, shared Go logic lives in a reusable package and Android calls it through a gomobile-generated AAR.

```text
CLI ------------\
Wails desktop ----> packages/core ---> packages/mobilebridge ---> gomobile AAR ---> Android app
Android --------/
```

## Why not port Wails to Android?

Wails remains the desktop frontend. Android is a separate native app because:

- Wails is optimized for desktop shells, not Android app packaging/lifecycle.
- Android needs normal APK install/run/logcat flows.
- gomobile gives a cleaner boundary for reusing Go logic without bundling a CLI executable.

## Go shared core

Shared business logic starts in:

```text
packages/core/service.go
```

Initial API:

```go
type RunInput struct {
    Text string `json:"text"`
}

type RunResult struct {
    Output string `json:"output"`
}

type Service struct{}
func NewService() *Service
func (s *Service) Run(ctx context.Context, input RunInput) (*RunResult, error)
```

The first implementation returns:

```text
processed: <text>
```

The CLI exposes this through:

```bash
go run ./cmd/agentsafe core run hello
```

The Wails app exposes a `RunCore(text string)` binding for frontend use.

## Mobile bridge

Android calls Go through:

```text
packages/mobilebridge/bridge.go
```

Public gomobile API:

```go
type MobileService struct{}
func NewMobileService() *MobileService
func (m *MobileService) RunJson(inputJson string) (string, error)
```

The mobile API intentionally uses JSON strings instead of exposing complex Go types. Do not expose `context.Context`, maps, generics, channels, or internal structs directly to gomobile.

Example input:

```json
{"text":"hello"}
```

Example output:

```json
{"output":"processed: hello"}
```

## gomobile AAR build

Install gomobile once if needed:

```bash
go install golang.org/x/mobile/cmd/gomobile@latest
gomobile init
```

Build the AAR:

```bash
./scripts/android/build-aar.sh
```

Defaults:

- `GOMOBILE_TARGET=android/arm64`
- `ANDROID_AAR_PATH=apps/android/app/libs/mobilebridge.aar`

For an emulator or multiple ABIs:

```bash
GOMOBILE_TARGET=android/arm64,android/amd64 ./scripts/android/build-aar.sh
```

The AAR is generated and ignored by git. Rebuild it after changing `packages/core` or `packages/mobilebridge`.

## Android project

Android app path:

```text
apps/android
```

Defaults:

- Kotlin Native Android app
- Gradle Kotlin DSL
- `applicationId = "com.example.vibecoding"`
- app name: `VibeCoding Android`
- `minSdk = 26`
- `compileSdk = 35`
- `targetSdk = 35`
- AAR path: `apps/android/app/libs/mobilebridge.aar`

The UI is intentionally minimal:

- input field
- `Run Go Core` button
- result text
- Logcat output under tag `VibeCodingAndroid`

## Build Android APK

```bash
./scripts/android/android-build.sh
```

This runs:

1. `scripts/android/build-aar.sh`
2. `apps/android/gradlew assembleDebug`

Default APK path:

```text
apps/android/app/build/outputs/apk/debug/app-debug.apk
```

Override examples:

```bash
ANDROID_GRADLE_TASK=:app:assembleDebug ./scripts/android/android-build.sh
ANDROID_APK_PATH=/tmp/app-debug.apk ./scripts/android/android-install.sh
```

## Run on device

Connect the device first, then:

```bash
ANDROID_APP_ID=com.example.vibecoding ./scripts/android/android-dev-loop.sh
```

With logs:

```bash
ANDROID_APP_ID=com.example.vibecoding ./scripts/android/android-dev-loop.sh --clear-log --log
```

## Change package name

Change these together:

1. `apps/android/app/build.gradle.kts`
   - `namespace`
   - `defaultConfig.applicationId`
2. Kotlin package and directory under `apps/android/app/src/main/java/...`
3. `AndroidManifest.xml` activity package if needed
4. shell env/aliases:

```bash
export ANDROID_APP_ID=com.yourcompany.yourapp
```

## Common errors

### `gomobile: command not found`

Install it and ensure `$(go env GOPATH)/bin` is on `PATH`:

```bash
go install golang.org/x/mobile/cmd/gomobile@latest
gomobile init
```

### `missing golang.org/x/mobile dependency`

Current gomobile requires a Go tool directive for `gobind`. This repo records it in `go.mod` with:

```bash
go get -tool golang.org/x/mobile/cmd/gobind
```

### Android SDK not found

Install Android Studio command-line tools or set:

```bash
export ANDROID_HOME=$HOME/Library/Android/sdk
export ANDROID_SDK_ROOT=$HOME/Library/Android/sdk
```

### AAR not found

Run:

```bash
./scripts/android/build-aar.sh
```

### Duplicate class

Check for duplicate AAR/JAR files under `apps/android/app/libs` and duplicate Gradle dependencies.

### minSdk problem

The app uses `minSdk = 26`. If a dependency requires a higher minSdk, either raise `minSdk` or choose a compatible dependency.

### adb device unauthorized

Unlock the phone and approve the debugging prompt, then run:

```bash
./scripts/android/adb-devices.sh
```

### multiple devices connected

Set a serial:

```bash
export ADB_SERIAL=<serial-from-adb-devices>
```
