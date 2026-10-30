#!/usr/bin/env bash
# No real gateways, sandboxes, or forge writes are used by this test suite.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."
dispatch_test_dir="$(mktemp -d)"
trap 'rm -rf "$dispatch_test_dir"' EXIT
export FUNNEL_DISPATCH_BINARY="$dispatch_test_dir/funnel-dispatch"
(cd .hypershell/execute-sdlc/dispatcher && go build -o "$FUNNEL_DISPATCH_BINARY" . && go test -race ./... && go vet ./...)
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s scripts -p test_funnel_dispatch.py
