#!/usr/bin/env bash
# Check CLI installation with local release fixtures.
set -euo pipefail
ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
TEST_DIR=$(mktemp -d)
trap 'rm -rf "$TEST_DIR"' EXIT
mkdir -p "$TEST_DIR/bin" "$TEST_DIR/assets" "$TEST_DIR/source" "$TEST_DIR/install"
export TEST_DIR
cat > "$TEST_DIR/bin/uname" <<'MOCK'
#!/bin/sh
case "$1" in
  -s) printf '%s\n' "${TEST_OS:-Linux}" ;;
  -m) printf '%s\n' "${TEST_ARCH:-x86_64}" ;;
esac
MOCK
cat > "$TEST_DIR/bin/curl" <<'MOCK'
#!/bin/sh
while [ "$#" -gt 0 ]; do
  case "$1" in
    https://*) url=$1 ;;
    -o) shift; output=$1 ;;
  esac
  shift
done
cp "$TEST_DIR/assets/${url##*/}" "$output"
MOCK
cat > "$TEST_DIR/source/openshell" <<'MOCK'
#!/bin/sh
printf 'openshell 0.0.109\n'
MOCK
chmod +x "$TEST_DIR/bin/"* "$TEST_DIR/source/openshell"
for target in x86_64-unknown-linux-musl aarch64-unknown-linux-musl aarch64-apple-darwin; do
  tar -czf "$TEST_DIR/assets/openshell-${target}.tar.gz" -C "$TEST_DIR/source" openshell
done
(cd "$TEST_DIR/assets" && sha256sum ./*.tar.gz | sed 's|  ./|  |') > "$TEST_DIR/assets/openshell-checksums-sha256.txt"
export PATH="$TEST_DIR/bin:$PATH"
export OPENSHELL_VERSION=v0.0.109 OPENSHELL_INSTALL_DIR="$TEST_DIR/install"
for platform in Linux/x86_64 Linux/aarch64 Darwin/arm64; do
  TEST_OS=${platform%/*} TEST_ARCH=${platform#*/} sh "$ROOT/scripts/install-openshell.sh"
  [[ "$("$OPENSHELL_INSTALL_DIR/openshell" --version)" == 'openshell 0.0.109' ]]
done
expect_failure() {
  if "$@" > "$TEST_DIR/error.log" 2>&1; then
    echo 'Installer accepted invalid input or a damaged release'
    exit 1
  fi
  # Failure must preserve the existing CLI.
  [[ "$("$OPENSHELL_INSTALL_DIR/openshell" --version)" == 'openshell 0.0.109' ]]
}
expect_failure env OPENSHELL_VERSION='v0.0.109; false' sh "$ROOT/scripts/install-openshell.sh"
expect_failure env TEST_OS=Darwin TEST_ARCH=x86_64 sh "$ROOT/scripts/install-openshell.sh"
cp "$TEST_DIR/assets/openshell-checksums-sha256.txt" "$TEST_DIR/checksums"
printf 'damaged archive\n' > "$TEST_DIR/assets/openshell-x86_64-unknown-linux-musl.tar.gz"
expect_failure sh "$ROOT/scripts/install-openshell.sh"
grep -q 'Checksum mismatch' "$TEST_DIR/error.log"
: > "$TEST_DIR/assets/openshell-checksums-sha256.txt"
expect_failure sh "$ROOT/scripts/install-openshell.sh"
grep -q 'no unique checksum' "$TEST_DIR/error.log"
cat "$TEST_DIR/checksums" "$TEST_DIR/checksums" > "$TEST_DIR/assets/openshell-checksums-sha256.txt"
expect_failure sh "$ROOT/scripts/install-openshell.sh"
grep -q 'no unique checksum' "$TEST_DIR/error.log"
echo 'CLI archive installation tests passed'
