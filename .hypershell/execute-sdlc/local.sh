#!/usr/bin/env bash
# Friendly Make entry point. The existing dispatcher owns runtime behavior.
set -euo pipefail
local_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
action="${1:-watch}"
shift || true
if [[ "$action" == help ]]; then
  cat "$local_dir/local-help.txt"
 exit 0
fi
config="${FULLSEND_LAUNCH_ENV_FILE:-.hypershell/fullsend.env}"
if [[ "$config" == '~/'* ]]; then config="$HOME/${config:2}"; fi
if [[ -n "${FULLSEND_LAUNCH_ENV_FILE:-}" && ! -f "$config" ]]; then
  echo "error: ENV_FILE does not exist: $config" >&2
  exit 2
fi
if [[ -f "$config" ]]; then
  set -a
  # Local, trusted shell configuration; never parsed as Make syntax.
  # shellcheck disable=SC1090
  source "$config"
  set +a
fi
scope="${FULLSEND_LAUNCH_SCOPE:-${FULLSEND_SCOPE:-}}"
if [[ -z "$scope" ]]; then
  echo 'error: set FULLSEND_SCOPE in .hypershell/fullsend.env or run make fullsend SCOPE=agent/scope/YOUR_NAME/fullsend' >&2
  exit 2
fi
if [[ "$scope" != agent/scope/* || "$scope" == agent/scope/ ]]; then
  echo 'error: scope must start with agent/scope/ and identify your work queue' >&2
  exit 2
fi
args=("$scope")
[[ -z "${FULLSEND_LAUNCH_MODEL:-}" ]] || args+=(--model "$FULLSEND_LAUNCH_MODEL")
[[ -z "${FULLSEND_LAUNCH_CONCURRENCY:-}" ]] || args+=(--concurrency "$FULLSEND_LAUNCH_CONCURRENCY")
[[ -z "${FULLSEND_LIFECYCLE:-}" ]] || args+=(--lifecycle "$FULLSEND_LIFECYCLE")
case "$action" in
  watch) python3 "$local_dir/local_scope.py" "$scope"; args+=(--watch) ;;
  status) export FULLSEND_HUMAN_OUTPUT=1; args+=(--status) ;;
  doctor) export FULLSEND_CHECK_SCOPE="$scope"; exec python3 "$local_dir/local_doctor.py" ;;
  *) echo "error: unknown local Fullsend action: $action" >&2; exit 2 ;;
esac
exec "$local_dir/funnel-dispatch.sh" "${args[@]}" "$@"
