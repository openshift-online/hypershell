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

_api_list='{"kind":"GatewayList","items":[{"id":"2FhMpQzXBzABC","kind":"Gateway","name":"dev-gateway","cluster_id":"c1"}]}'
assert_eq "2FhMpQzXBzABC" "$(printf '%s' "${_api_list}" | json_named_id dev-gateway)" \
  "json_named_id finds id-before-name list items"
assert_eq "" "$(printf '%s' "${_api_list}" | json_named_id missing-gateway)" \
  "json_named_id is empty when the name is absent"

_mc_list='{"kind":"ManagedClusterList","items":[{"id":"2manual","name":"local-kind","oidc_subject":""}]}'
assert_eq "" "$(printf '%s' "${_mc_list}" | json_registered_cluster_id local-kind)" \
  "json_registered_cluster_id ignores an unregistered (manually created) record"
_mc_list='{"kind":"ManagedClusterList","items":[{"id":"2reg","name":"local-kind","oidc_subject":"sub-a"}]}'
assert_eq "2reg" "$(printf '%s' "${_mc_list}" | json_registered_cluster_id local-kind)" \
  "json_registered_cluster_id returns the control plane's registered record"
pin_fn_defined() { declare -F pin_controller_gateway_db_hosts >/dev/null; }
assert_ok "pin_controller_gateway_db_hosts is defined" pin_fn_defined

# Coverage build arguments follow KIND_COVERAGE.
# Verifies: DEV-LOC-18.01, DEV-LOC-18.02
KIND_COVERAGE=""
assert_fail "coverage is off by default" coverage_enabled
assert_eq "" "$(coverage_build_args api-server)" "no build args when coverage is off"
KIND_COVERAGE=true
assert_ok "KIND_COVERAGE=true enables coverage" coverage_enabled
assert_eq "--build-arg GO_COVERAGE=true" "$(coverage_build_args api-server)" "api-server gets a -cover build"
assert_eq "--build-arg GO_COVERAGE=true" "$(coverage_build_args control-plane)" "control-plane gets a -cover build"
assert_eq "--build-arg BFF_SOURCE_MAPS=true" "$(coverage_build_args web-console)" "web-console gets BFF source maps"
assert_fail "unknown component is rejected" coverage_build_args unknown 2>/dev/null
read -r -a _cov_args <<<"$(coverage_build_args api-server)"
assert_eq "2" "${#_cov_args[@]}" "build args split into two words for the container engine"
KIND_COVERAGE=false
assert_fail "KIND_COVERAGE=false keeps coverage off" coverage_enabled
unset KIND_COVERAGE

echo "Kind swap ledger tests: ${PASS} passed, ${FAIL} failed"
if ((FAIL > 0)); then
  exit 1
fi
