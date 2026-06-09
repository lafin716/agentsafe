#!/usr/bin/env bash
set -euo pipefail

BINARY_NAME="agr"
INSTALL_PATH="/usr/local/bin/${BINARY_NAME}"
BUILD_TARGET="./apps/cli"

echo "Building ${BINARY_NAME}..."
go build -o "${BINARY_NAME}" "${BUILD_TARGET}"

echo "Installing to ${INSTALL_PATH}..."
install -m 755 "${BINARY_NAME}" "${INSTALL_PATH}"
rm "${BINARY_NAME}"

echo "Done. Installed at ${INSTALL_PATH}"
