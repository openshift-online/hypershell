#!/bin/sh
# Fail when the compressed hsctl browser module is larger than its budget
# (specs/platform/hsctl-browser-build.spec.md, CLI-WASM-05.02). The budget
# applies to the transfer size, measured as gzip -9 output.
#
# Usage: scripts/check-hsctl-wasm-size.sh [path/to/hsctl.wasm]
#   The path defaults to $HSCTL_RUNTIME_DIR/hsctl.wasm, then
#   components/web-console/.hsctl-runtime/hsctl.wasm.
#   HSCTL_WASM_MAX_GZIP_BYTES overrides the budget (tests only).
set -eu

root="$(cd "$(dirname "$0")/.." && pwd)"
wasm="${1:-${HSCTL_RUNTIME_DIR:-$root/components/web-console/.hsctl-runtime}/hsctl.wasm}"
allowed="${HSCTL_WASM_MAX_GZIP_BYTES:-8388608}"

if [ ! -f "$wasm" ]; then
  echo "hsctl.wasm not found at $wasm; build it with scripts/build-hsctl-wasm.sh" >&2
  exit 1
fi

measured="$(gzip -9 -c "$wasm" | wc -c | tr -d ' ')"
if [ "$measured" -gt "$allowed" ]; then
  echo "FAIL: compressed hsctl.wasm is $measured bytes (gzip -9), over the allowed $allowed bytes" >&2
  exit 1
fi
echo "OK: compressed hsctl.wasm is $measured bytes (gzip -9), within the allowed $allowed bytes"
