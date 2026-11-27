#!/bin/sh
# Build the hsctl browser module for the web console terminal
# (specs/platform/hsctl-browser-build.spec.md, CLI-WASM-01 and CLI-WASM-05):
# hsctl.wasm from components/cli/cmd/hsctl-wasm and wasm_exec.js from the same
# Go toolchain, both in one output directory. The web console build reads them
# from HSCTL_RUNTIME_DIR (default components/web-console/.hsctl-runtime).
#
# Usage: scripts/build-hsctl-wasm.sh [outdir]
#   outdir  defaults to $HSCTL_RUNTIME_DIR, then components/web-console/.hsctl-runtime
#
# POSIX sh, so the web console image build can run it in the Go builder stage.
set -eu

root="$(cd "$(dirname "$0")/.." && pwd)"
outdir="${1:-${HSCTL_RUNTIME_DIR:-$root/components/web-console/.hsctl-runtime}}"
mkdir -p "$outdir"
outdir="$(cd "$outdir" && pwd)"

cd "$root/components/cli"
CGO_ENABLED=0 GOOS=js GOARCH=wasm go build -trimpath -ldflags="-s -w" \
  -o "$outdir/hsctl.wasm" ./cmd/hsctl-wasm

# Run go env in the module, so a go.mod toolchain line selects the same
# toolchain that compiled the module. Go 1.24 moved wasm_exec.js to lib/wasm.
goroot="$(go env GOROOT)"
wasm_exec="$goroot/lib/wasm/wasm_exec.js"
if [ ! -f "$wasm_exec" ]; then
  wasm_exec="$goroot/misc/wasm/wasm_exec.js"
fi
if [ ! -f "$wasm_exec" ]; then
  echo "wasm_exec.js not found under $goroot/lib/wasm or $goroot/misc/wasm" >&2
  exit 1
fi
# The module cache keeps the toolchain read-only; replace, do not overwrite.
rm -f "$outdir/wasm_exec.js"
cp "$wasm_exec" "$outdir/wasm_exec.js"
chmod 0644 "$outdir/wasm_exec.js"

echo "Built $outdir/hsctl.wasm and $outdir/wasm_exec.js with $(go env GOVERSION)"
