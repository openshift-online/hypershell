#!/usr/bin/env sh
# install-openshell.sh - init container: download, verify, and install openshell
#
# Required environment variables (set on the init container):
#   OPENSHELL_VERSION      - release tag (e.g. v0.8.2)
#   OPENSHELL_SHA256_AMD64 - expected SHA256 for the linux/amd64 binary
#   OPENSHELL_SHA256_ARM64 - expected SHA256 for the linux/arm64 binary
#   OPENSHELL_DOWNLOAD_URL - base download URL (without filename)
#
# Writes the verified binary to /tools/openshell (shared volume).

set -eu

INSTALL_DIR="${INSTALL_DIR:-/tools}"
ARCH="$(uname -m)"

case "$ARCH" in
    x86_64)  ARCH_LABEL="amd64"; EXPECTED_SHA="$OPENSHELL_SHA256_AMD64" ;;
    aarch64) ARCH_LABEL="arm64"; EXPECTED_SHA="$OPENSHELL_SHA256_ARM64" ;;
    *)       echo "unsupported arch: $ARCH" >&2; exit 1 ;;
esac

FILENAME="openshell-linux-${ARCH_LABEL}"
URL="${OPENSHELL_DOWNLOAD_URL}/${OPENSHELL_VERSION}/${FILENAME}"
DEST="${INSTALL_DIR}/openshell"

echo "downloading openshell ${OPENSHELL_VERSION} (${ARCH_LABEL})"
curl -fsSL -o "$DEST" "$URL"

echo "verifying sha256"
ACTUAL_SHA="$(sha256sum "$DEST" | awk '{print $1}')"
if [ "$ACTUAL_SHA" != "$EXPECTED_SHA" ]; then
    echo "sha256 mismatch: expected $EXPECTED_SHA, got $ACTUAL_SHA" >&2
    rm -f "$DEST"
    exit 1
fi

chmod 0755 "$DEST"
echo "installed openshell ${OPENSHELL_VERSION} to ${DEST}"
"$DEST" version
