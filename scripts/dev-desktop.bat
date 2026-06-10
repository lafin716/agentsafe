@echo off
setlocal enabledelayedexpansion

rem Runs the agentsafe desktop app in development mode (hot reload).
rem Installs the Wails CLI on first run if it is missing.

rem Resolve repo root (parent of this scripts\ directory) and the desktop dir.
pushd "%~dp0.." || exit /b 1
set "ROOT_DIR=%CD%"
popd
set "DESKTOP_DIR=%ROOT_DIR%\apps\desktop"

where go >nul 2>nul
if errorlevel 1 (
  echo Error: Go is required but was not found in PATH.
  exit /b 1
)

rem Add the Go bin dir to PATH so a freshly installed wails is found.
for /f "delims=" %%i in ('go env GOPATH') do set "GOPATH_DIR=%%i"
set "PATH=%GOPATH_DIR%\bin;%PATH%"

where pnpm >nul 2>nul
if errorlevel 1 (
  echo Error: pnpm is required but was not found in PATH.
  exit /b 1
)

where wails >nul 2>nul
if errorlevel 1 (
  echo Wails CLI not found. Installing github.com/wailsapp/wails/v2/cmd/wails@latest ...
  go install github.com/wailsapp/wails/v2/cmd/wails@latest || exit /b 1
)

echo Starting desktop app in dev mode...
cd /d "%DESKTOP_DIR%" || exit /b 1
wails dev %*
