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

# A suffixed version (a downstream build) is pulled from the odh-openshell-cli
# image instead of a public GitHub release. Mock podman's create/cp/rm enough
# to prove the image reference is built correctly and the extracted binary is
# installed the same way as the release path.
cat > "$TEST_DIR/bin/podman" <<MOCK
#!/bin/sh
case "\$1" in
  create)
    shift
    while [ "\$#" -gt 0 ]; do
      case "\$1" in
        --name) shift; ctr_name=\$1 ;;
        quay.io/*) image=\$1 ;;
      esac
      shift
    done
    [ "\$image" = "quay.io/opendatahub/odh-openshell-cli:v0.0.116-rhaiv.6" ] || { echo "unexpected image: \$image" >&2; exit 1; }
    [ -n "\$ctr_name" ] || { echo "missing --name" >&2; exit 1; }
    ;;
  cp)
    shift
    dest=\$2
    cp "$TEST_DIR/source/openshell" "\$dest"
    ;;
  rm) ;;
  *) echo "unexpected podman subcommand: \$1" >&2; exit 1 ;;
esac
MOCK
chmod +x "$TEST_DIR/bin/podman"
cat > "$TEST_DIR/source/openshell" <<'MOCK'
#!/bin/sh
printf 'openshell 0.0.116-rhaiv.6\n'
MOCK
chmod +x "$TEST_DIR/source/openshell"
OPENSHELL_VERSION=v0.0.116-rhaiv.6 sh "$ROOT/scripts/install-openshell.sh"
[[ "$("$OPENSHELL_INSTALL_DIR/openshell" --version)" == 'openshell 0.0.116-rhaiv.6' ]]

# Restore the plain-release fixtures (the checksum-mismatch tests above left
# openshell-checksums-sha256.txt corrupted) so downstream failure checks below
# observe a stable "existing CLI" baseline again.
cp "$TEST_DIR/checksums" "$TEST_DIR/assets/openshell-checksums-sha256.txt"
cat > "$TEST_DIR/source/openshell" <<'MOCK'
#!/bin/sh
printf 'openshell 0.0.109\n'
MOCK
chmod +x "$TEST_DIR/source/openshell"
for target in x86_64-unknown-linux-musl aarch64-unknown-linux-musl aarch64-apple-darwin; do
  tar -czf "$TEST_DIR/assets/openshell-${target}.tar.gz" -C "$TEST_DIR/source" openshell
done
(cd "$TEST_DIR/assets" && sha256sum ./*.tar.gz | sed 's|  ./|  |') > "$TEST_DIR/assets/openshell-checksums-sha256.txt"
OPENSHELL_VERSION=v0.0.109 sh "$ROOT/scripts/install-openshell.sh"

# A container image's binary is Linux-only regardless of host CPU architecture.
# On a non-Linux host this must fail clearly instead of extracting a binary
# that can never run there (an "exec format error" is a confusing way to find
# this out). The podman mock is still on PATH from above; it must not even be
# invoked once the OS check fails.
expect_failure env TEST_OS=Darwin OPENSHELL_VERSION=v0.0.116-rhaiv.6 sh "$ROOT/scripts/install-openshell.sh"
grep -q 'Linux-only container image' "$TEST_DIR/error.log"

# Without podman or docker on PATH, the downstream path fails clearly instead
# of silently falling back to the (incompatible) GitHub release path. Use a
# PATH built only from the coreutils the script needs before that check, so a
# real podman/docker elsewhere on this machine's PATH can't mask the failure.
mkdir -p "$TEST_DIR/bin-empty"
for tool in sh grep mkdir mktemp rm uname; do
  ln -s "$(command -v "$tool")" "$TEST_DIR/bin-empty/$tool"
done
expect_failure env PATH="$TEST_DIR/bin-empty" OPENSHELL_VERSION=v0.0.116-rhaiv.6 sh "$ROOT/scripts/install-openshell.sh"
grep -q 'Install podman or docker' "$TEST_DIR/error.log"

echo 'CLI archive installation tests passed'
