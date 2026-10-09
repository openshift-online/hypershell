#!/usr/bin/env bash
#
# smoke.sh - Bash user-interaction smoke walkthrough (make e2e-smoke).
#
# Walks the short happy path a user follows and echoes each command (show_cmd)
# before running it, so the captured output reads as the sequence of commands a
# user would type: acquire a token, create a gateway, register + connect the
# openshell CLI, create a sandbox, exec in it, then delete the sandbox and gateway.
#
# This is NOT the authoritative gate -- the Go E2ESuite short mode is the blocking
# quick gate (see specs/platform/e2e-testing.spec.md, Bash Smoke Script). It reuses
# the deployed environment and seeded cluster id, covers the happy path
# only, and supports a demo pause via E2E_PAUSE (default 0). It cleans up the
# gateway it created on exit unless E2E_SKIP_CLEANUP=1.
#
# Kind is the primary target; it MAY run against OpenShift.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=/dev/null
source "${SCRIPT_DIR}/lib.sh"

export E2E_PAUSE="${E2E_PAUSE:-0}"

# --- infra driver selection (slim) ---
e2e_select_infra_driver
# shellcheck source=/dev/null
source "${SCRIPT_DIR}/drivers/${E2E_INFRA_DRIVER}.sh"

GW_NAME="${E2E_GATEWAY_NAME:-smoke-gw}-$(date +%s | tail -c5)"
GW_ID=""
GW_NAMESPACE=""

cleanup() {
  local code=$?
  if [[ "${E2E_SKIP_CLEANUP:-0}" != "1" && -n "${GW_ID}" ]]; then
    show_cmd "DELETE ${_DISCOVER_API_HOST}/api/hypershell/v1/gateways/${GW_ID}"
    api_curl -X DELETE "${_DISCOVER_API_HOST}/api/hypershell/v1/gateways/${GW_ID}" >/dev/null 2>&1 || true
  fi
  exit "${code}"
}
trap cleanup EXIT INT TERM

echo "==> OpenShell smoke walkthrough (driver=${E2E_INFRA_DRIVER})"

# 1. Discover the API and acquire an OIDC token.
discover_api_host
API_HOST="${_DISCOVER_API_HOST}"
export API_HOST
show_cmd "acquire OIDC token for ${E2E_OIDC_USERNAME:-admin}"
acquire_oidc_token
[[ -n "${_OIDC_ACCESS_TOKEN}" ]] || { fail_test "could not acquire OIDC token"; exit 1; }
pass "acquired OIDC token"

# 2. Resolve the seeded cluster id and create a gateway.
e2e_ensure_seed_ids
show_cmd "POST ${_DISCOVER_API_HOST}/api/hypershell/v1/gateways  # ${GW_NAME}"
CREATE_RESP="$(api_curl -X POST -H 'Content-Type: application/json' \
  -d "$(e2e_gateway_create_body "${GW_NAME}")" \
  "${_DISCOVER_API_HOST}/api/hypershell/v1/gateways")"
GW_ID="$(printf '%s' "${CREATE_RESP}" | e2e_json_field id)"
[[ -n "${GW_ID}" ]] || { fail_test "gateway create failed: ${CREATE_RESP}"; exit 1; }
pass "created gateway ${GW_NAME} (${GW_ID})"

# 3. Wait for the gateway to reconcile to Running.
show_cmd "poll gateway ${GW_ID} until phase=Running"
e2e_wait_gateway_running "${GW_ID}" "${E2E_PROVISION_TIMEOUT:-300}"
GW_NAMESPACE="$(api_curl "${_DISCOVER_API_HOST}/api/hypershell/v1/gateways/${GW_ID}" | e2e_json_field namespace)"
pass "gateway Running in namespace ${GW_NAMESPACE}"

# 4. Register and connect the openshell CLI.
wait_for_gateway_route "${GW_NAME}" "${GW_NAMESPACE}"
discover_gateway_endpoint "${GW_NAME}" "${GW_NAMESPACE}"
GW_LOCAL_NAME="${GW_NAMESPACE}-openshell"
GW_KC_CLIENT_ID="${GW_NAME}-${GW_ID}"
acquire_gateway_token_with_role "${E2E_OIDC_USERNAME:-admin}" "${E2E_OIDC_PASSWORD:-admin}" "${GW_KC_CLIENT_ID}" openshell-admin
GW_CONFIG_DIR="${HOME}/.config/openshell/gateways/${GW_LOCAL_NAME}"
mkdir -p "${GW_CONFIG_DIR}"
show_cmd "openshell gateway register ${GW_LOCAL_NAME}  # write ${GW_CONFIG_DIR}"
cat >"${GW_CONFIG_DIR}/metadata.json" <<JSON
{"name":"${GW_LOCAL_NAME}","gateway_endpoint":"${_DISCOVER_GW_ENDPOINT}","is_remote":true,"gateway_port":0,"auth_mode":"oidc","oidc_issuer":"${E2E_OIDC_ISSUER}","oidc_client_id":"${GW_KC_CLIENT_ID}","gateway_insecure":true}
JSON
cat >"${GW_CONFIG_DIR}/oidc_token.json" <<JSON
{"access_token":"${_OIDC_ACCESS_TOKEN}","issuer":"${E2E_OIDC_ISSUER}","client_id":"${GW_KC_CLIENT_ID}"}
JSON
chmod 0600 "${GW_CONFIG_DIR}"/*.json

show_cmd "${OPENSHELL_BIN:-openshell} -g ${GW_LOCAL_NAME} status"
if retry_until 90 5 "${OPENSHELL_BIN:-openshell} -g ${GW_LOCAL_NAME} status 2>/dev/null | grep -qi Connected"; then
  pass "openshell CLI connected"
else
  fail_test "openshell CLI did not connect"
fi

# 5. Create a sandbox, exec in it, delete it.
SANDBOX_NAME="sb-$(date +%s | tail -c6)"
show_cmd "${OPENSHELL_BIN:-openshell} -g ${GW_LOCAL_NAME} sandbox create --name ${SANDBOX_NAME}"
"${OPENSHELL_BIN:-openshell}" -g "${GW_LOCAL_NAME}" sandbox create --name "${SANDBOX_NAME}" || true
if retry_until "${E2E_SANDBOX_TIMEOUT:-300}" 5 "${OPENSHELL_BIN:-openshell} -g ${GW_LOCAL_NAME} sandbox exec -n ${SANDBOX_NAME} -- true 2>/dev/null"; then
  show_cmd "${OPENSHELL_BIN:-openshell} -g ${GW_LOCAL_NAME} sandbox exec -n ${SANDBOX_NAME} -- uname -a"
  "${OPENSHELL_BIN:-openshell}" -g "${GW_LOCAL_NAME}" sandbox exec -n "${SANDBOX_NAME}" -- uname -a || true
  pass "sandbox ${SANDBOX_NAME} created and reachable"
  show_cmd "${OPENSHELL_BIN:-openshell} -g ${GW_LOCAL_NAME} sandbox delete ${SANDBOX_NAME}"
  "${OPENSHELL_BIN:-openshell}" -g "${GW_LOCAL_NAME}" sandbox delete "${SANDBOX_NAME}" || true
else
  fail_test "sandbox ${SANDBOX_NAME} did not become reachable"
fi

# 6. Gateway deletion happens in the cleanup trap.
E2E_COMPLETED=1
print_results
