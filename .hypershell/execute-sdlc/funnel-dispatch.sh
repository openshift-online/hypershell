#!/usr/bin/env bash
# Keep exports-only env files compatible with the local execute-sdlc command.
set -euo pipefail
args=("$@")
while (( $# )); do
  case "$1" in
    --env-file)
      [[ $# -ge 2 && -f "$2" ]] || { echo 'error: --env-file requires an existing file' >&2; exit 2; }
      set -a
      # shellcheck disable=SC1090
      source "$2"
      set +a
      shift
      ;;
  esac
  shift
done
dispatch_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
export FUNNEL_ADAPTER="$dispatch_dir/dispatch_adapter.py"
# Content-addressed binaries remain available to detached workers across edits.
# Tests can supply a prebuilt binary, including when PATH contains fake CLIs.
if [[ -n "${FUNNEL_DISPATCH_BINARY:-}" ]]; then
  exec "$FUNNEL_DISPATCH_BINARY" "${args[@]}"
fi
dispatch_hash="$(python3 - "$dispatch_dir/dispatcher" <<'PY'
import hashlib
from pathlib import Path
import sys
root = Path(sys.argv[1])
digest = hashlib.sha256()
for path in sorted(root.iterdir()):
    if path.suffix == '.go' or path.name in ('go.mod', 'go.sum'):
        digest.update(path.name.encode())
        digest.update(path.read_bytes())
print(digest.hexdigest()[:20])
PY
)"
dispatch_cache="${XDG_CACHE_HOME:-$HOME/.cache}/hypershell/funnel/$dispatch_hash"
dispatch_binary="$dispatch_cache/funnel-dispatch"
if [[ ! -x "$dispatch_binary" ]]; then
  command -v go >/dev/null || { echo 'error: Go is required to build the local dispatcher' >&2; exit 1; }
  mkdir -p "$dispatch_cache"
  dispatch_build="$(mktemp "$dispatch_cache/build.XXXXXX")"
  trap 'rm -f "$dispatch_build"' EXIT
  echo 'Building the local dispatcher...' >&2
  (cd "$dispatch_dir/dispatcher" && go build -mod=readonly -o "$dispatch_build" .)
  mv "$dispatch_build" "$dispatch_binary"
  trap - EXIT
fi
exec "$dispatch_binary" "${args[@]}"
