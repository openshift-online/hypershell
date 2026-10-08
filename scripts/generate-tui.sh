#!/usr/bin/env bash
# Regenerate the hsctl terminal UI descriptor from the API server OpenAPI spec
# with the rh-trex-ai tui-generator, pinned in scripts/rh-trex-ai.ref.
#
# Usage: scripts/generate-tui.sh [--check]
#   --check  fail if the committed descriptor differs from a fresh generation
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=scripts/lib/trex-checkout.sh
source "$root/scripts/lib/trex-checkout.sh"
trex_checkout "$root"

spec="$root/components/api-server/openapi/openapi.yaml"
out="$root/components/cli/data/generated/tui"

target="$out"
if [[ "${1:-}" == "--check" ]]; then
  target="$(mktemp -d)"
  trap 'rm -rf "$target"' EXIT
fi

(cd "$TREX_DIR/scripts/tui-generator" && TERM=dumb go run . --spec "$spec" --out "$target")

if [[ "${1:-}" == "--check" ]]; then
  if ! diff -r "$target" "$out" >/dev/null; then
    echo "TUI descriptor is stale; run 'make generate-tui' and commit the result" >&2
    diff -r "$target" "$out" | head -20 >&2 || true
    exit 1
  fi
fi
