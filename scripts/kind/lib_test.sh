#!/usr/bin/env bash
# Unit tests for scripts/kind/lib.sh swap-ledger helpers. No cluster required.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "${SCRIPT_DIR}/lib.sh"

PASS=0
FAIL=0

assert_ok() {
  local label="$1"
  shift
  if "$@"; then
    PASS=$((PASS + 1))
  else
    FAIL=$((FAIL + 1))
    printf 'FAIL: %s (expected success)\n' "${label}"
  fi
}

assert_fail() {
  local label="$1"
  shift
  if "$@"; then
    FAIL=$((FAIL + 1))
    printf 'FAIL: %s (expected failure)\n' "${label}"
  else
    PASS=$((PASS + 1))
  fi
}

assert_eq() {
  local want="$1" got="$2" label="$3"
  if [[ "${want}" == "${got}" ]]; then
    PASS=$((PASS + 1))
  else
    FAIL=$((FAIL + 1))
    printf 'FAIL: %s (want=%q got=%q)\n' "${label}" "${want}" "${got}"
  fi
}

WORKDIR="$(mktemp -d)"
trap 'rm -rf "${WORKDIR}"' EXIT
cd "${WORKDIR}"

printf 'api-server\ncontrol-plane\n' > "${SWAP_FILE}"
assert_ok "legacy bare api-server is swapped" is_swapped api-server
assert_ok "legacy bare control-plane is swapped" is_swapped control-plane
assert_fail "legacy file does not mark web-console swapped" is_swapped web-console
assert_eq "" "$(swap_image api-server)" "legacy entry has no image"

track_swap api-server "example.local/api:dev"
assert_ok "tab rewrite still reports swapped" is_swapped api-server
assert_eq "example.local/api:dev" "$(swap_image api-server)" "track_swap records the image"
if grep -q '^api-server$' "${SWAP_FILE}"; then
  FAIL=$((FAIL + 1))
  echo 'FAIL: track_swap left a leftover bare api-server line'
else
  PASS=$((PASS + 1))
fi
assert_ok "untouched legacy control-plane remains swapped" is_swapped control-plane

clear_swap control-plane
assert_fail "clear_swap removes a legacy bare line" is_swapped control-plane
assert_ok "tab-format api-server survives clear of the other component" is_swapped api-server

printf 'web-console\thot-reload\n' >> "${SWAP_FILE}"
assert_ok "tab-format web-console is swapped" is_swapped web-console
assert_eq "hot-reload" "$(swap_image web-console)" "tab-format swap_image"
assert_fail "api-server-extra is not matched as api-server" is_swapped api-server-extra

echo "Kind swap ledger tests: ${PASS} passed, ${FAIL} failed"
if ((FAIL > 0)); then
  exit 1
fi
