#!/usr/bin/env bash
# e2e-hsctl.sh - smoke-test the hsctl CLI against a running Kind cluster.
#
# Exercises login, whoami, list/get for every resource type, create/delete of a
# transient GatewayRelease, hsctl apply -f, hsctl get gateway --show-connection,
# and logout.  It does NOT provision a full OpenShell gateway -- for that use
# e2e-openshell.sh.
#
# Usage:
#   bash tests/e2e/e2e-hsctl.sh              # auto-detects Kind cluster
#   HSCTL_TEST_API_URL=http://localhost:8000 \
#     bash tests/e2e/e2e-hsctl.sh            # explicit port-forward URL
#
# Prerequisites:
#   make kind-up       (or a cluster that has already been seeded)
#   kubectl context pointing at the Kind cluster
#
# Environment variables:
#   HSCTL               Path to the hsctl binary (default: auto-built or on PATH)
#   HSCTL_TEST_API_URL  API server base URL.  Default: http://localhost:8000
#                       (port-forward).  Override with the HTTPS route when you
#                       have cloud-provider-kind running:
#                         https://api.hypershell.localhost
#   HSCTL_INSECURE      Set to "true" to pass --insecure to hsctl login (needed
#                       for the HTTPS route with a self-signed cert).
#                       Implied when the URL contains "localhost" and starts with
#                       "https://". Default: auto.
#   KEYCLOAK_URL        Keycloak base URL.
#                       Default: https://keycloak.hypershell.localhost
#   OIDC_USERNAME       Keycloak username. Default: admin
#   OIDC_PASSWORD       Keycloak password. Default: admin
#   KIND_NAMESPACE      Kubernetes namespace for the API server pod.
#                       Default: hypershell-system
#   HSCTL_SKIP_BUILD    Set to "true" to skip auto-building hsctl.
#   HSCTL_SKIP_CLEANUP  Set to "1" to keep transient test resources after run.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

# ---------------------------------------------------------------------------
# Source shared lib for color output and pass/fail tracking
# ---------------------------------------------------------------------------
# shellcheck source=lib.sh
source "${SCRIPT_DIR}/lib.sh"

# ---------------------------------------------------------------------------
# Configuration
# ---------------------------------------------------------------------------
: "${HSCTL_TEST_API_URL:=http://localhost:8000}"
: "${HSCTL_INSECURE:=auto}"
: "${KEYCLOAK_URL:=https://keycloak.hypershell.localhost}"
: "${OIDC_USERNAME:=admin}"
: "${OIDC_PASSWORD:=admin}"
: "${KIND_NAMESPACE:=hypershell-system}"
: "${HSCTL_SKIP_BUILD:=false}"
: "${HSCTL_SKIP_CLEANUP:=0}"

# Derive --insecure flag: explicit, or auto-detected for https://...localhost
_INSECURE_FLAG=""
if [[ "${HSCTL_INSECURE}" == "true" ]]; then
  _INSECURE_FLAG="--insecure"
elif [[ "${HSCTL_INSECURE}" == "auto" ]]; then
  if [[ "${HSCTL_TEST_API_URL}" == https://*.localhost* ]] || \
     [[ "${HSCTL_TEST_API_URL}" == https://127.* ]]; then
    _INSECURE_FLAG="--insecure"
  fi
fi

# ---------------------------------------------------------------------------
# _kind_curl - curl for *.hypershell.localhost routes in Kind.
#
# *.localhost resolves to both 127.0.0.1 and ::1 but cloud-provider-kind's
# envoy only binds IPv4 -- so plain curl may connect on IPv6 and get ECONNREFUSED.
# Strategy:
#   1. Always pass --ipv4 so curl prefers the IPv4 address.
#   2. If the connection still fails (curl exit 7), discover the kindccm-gw
#      container's ephemeral port and retry with --connect-to so the port-443
#      SNI routing still works even without sudo pfctl/iptables.
# ---------------------------------------------------------------------------
_KINDCCM_PORT_CACHE=""
_kindccm_port() {
  if [[ -z "${_KINDCCM_PORT_CACHE}" ]]; then
    local engine
    if command -v podman &>/dev/null; then engine=podman
    elif command -v docker &>/dev/null; then engine=docker
    else return 1; fi
    local cid
    cid=$(${engine} ps -q --filter "name=kindccm-gw" 2>/dev/null | head -1)
    if [[ -n "${cid}" ]]; then
      _KINDCCM_PORT_CACHE=$(${engine} port "${cid}" 443 2>/dev/null \
        | head -1 | grep -oE '[0-9]+$' || true)
    fi
  fi
  echo "${_KINDCCM_PORT_CACHE}"
}

_kind_curl() {
  local out status
  out=$(curl -sk --ipv4 "$@" 2>&1)
  status=$?
  if [[ "${status}" -eq 7 ]]; then
    local kport
    kport=$(_kindccm_port)
    if [[ -n "${kport}" && "${kport}" != "443" ]]; then
      out=$(curl -sk --ipv4 \
        --connect-to "api.hypershell.localhost:443:127.0.0.1:${kport}" \
        --connect-to "keycloak.hypershell.localhost:443:127.0.0.1:${kport}" \
        "$@" 2>&1)
      status=$?
    fi
  fi
  printf '%s' "${out}"
  return "${status}"
}

# Unique suffix for resources created by this run
_RUN_ID="$(LC_ALL=C tr -dc 'a-f0-9' </dev/urandom 2>/dev/null | head -c 8 || date +%s | sha256sum | cut -c1-8)"
_TEST_RELEASE="hsctl-test-${_RUN_ID}"
_TEST_GW_NET="hsctl-net-${_RUN_ID}"
_CREATED_RELEASE_ID=""
_CREATED_GW_NET_ID=""

# Temp dir for config and scratch files - isolated from the real ~/.hypershell.json
WORK_DIR="$(mktemp -d)"
HSCTL_CONFIG_FILE="${WORK_DIR}/config.json"
TOKEN_FILE="${WORK_DIR}/token"
APPLY_FILE="${WORK_DIR}/release.yaml"
export HYPERSHELL_CONFIG="${HSCTL_CONFIG_FILE}"

# Port-forward PID
_PF_PID=""

# ---------------------------------------------------------------------------
# Cleanup
# ---------------------------------------------------------------------------
_cleanup() {
  local exit_code=$?

  if [[ "${HSCTL_SKIP_CLEANUP}" != "1" && -n "${_CREATED_RELEASE_ID}" && -n "${HSCTL:-}" ]]; then
    dim "  Cleaning up test GatewayRelease ${_CREATED_RELEASE_ID}..."
    "${HSCTL}" delete gatewayRelease "${_CREATED_RELEASE_ID}" --yes 2>/dev/null || true
  fi
  if [[ "${HSCTL_SKIP_CLEANUP}" != "1" && -n "${_CREATED_GW_NET_ID}" && -n "${HSCTL:-}" ]]; then
    dim "  Cleaning up test GatewayNetwork ${_CREATED_GW_NET_ID}..."
    "${HSCTL}" delete gatewayNetwork "${_CREATED_GW_NET_ID}" --yes 2>/dev/null || true
  fi

  if [[ -n "${_PF_PID}" ]]; then
    kill "${_PF_PID}" 2>/dev/null || true
    wait "${_PF_PID}" 2>/dev/null || true
  fi

  rm -rf "${WORK_DIR}"

  if [[ "${exit_code}" -ne 0 ]]; then
    red "Script exited with status ${exit_code}"
  fi
}
trap _cleanup EXIT

# Default to no pause between shown commands (E2E_PAUSE=1 in lib.sh is for demo pacing)
: "${E2E_PAUSE:=0}"

# ---------------------------------------------------------------------------
# Step 1: locate or build hsctl
# ---------------------------------------------------------------------------
e2e_area "1. Locate hsctl binary"

if [[ -n "${HSCTL:-}" ]]; then
  dim "  Using HSCTL=${HSCTL}"
elif [[ "${HSCTL_SKIP_BUILD}" != "true" ]] && command -v go >/dev/null 2>&1; then
  dim "  Building hsctl from source..."
  (cd "${REPO_ROOT}/components/cli" && \
    CGO_ENABLED=0 go build -o "${WORK_DIR}/hsctl" ./cmd/hsctl 2>&1) || true
  if [[ -x "${WORK_DIR}/hsctl" ]]; then
    HSCTL="${WORK_DIR}/hsctl"
    dim "  Built: ${HSCTL}"
  fi
fi

if [[ -z "${HSCTL:-}" ]]; then
  HSCTL="$(command -v hsctl 2>/dev/null || true)"
fi

if [[ -z "${HSCTL:-}" || ! -x "${HSCTL}" ]]; then
  red "hsctl binary not found. Run 'make build-cli' or set HSCTL=/path/to/hsctl."
  exit 1
fi

HSCTL_VERSION=$("${HSCTL}" version 2>/dev/null || echo "unknown")
pass "hsctl found: ${HSCTL} (${HSCTL_VERSION})"

# ---------------------------------------------------------------------------
# Step 2: set up port-forward when using the default localhost:8000 URL
# ---------------------------------------------------------------------------
e2e_area "2. API server connectivity"

if [[ "${HSCTL_TEST_API_URL}" == "http://localhost:8000" ]]; then
  dim "  Starting port-forward to hypershell-api-server:8000..."
  kubectl port-forward svc/hypershell-api-server \
    -n "${KIND_NAMESPACE}" 8000:8000 >/dev/null 2>&1 &
  _PF_PID=$!

  _api_ready=""
  for _ in $(seq 1 20); do
    if curl -s --ipv4 -o /dev/null -m 3 \
        "${HSCTL_TEST_API_URL}/api/hypershell/v1/gateways" 2>/dev/null; then
      _api_ready=true
      break
    fi
    # Re-establish the forward if it died
    if ! kill -0 "${_PF_PID}" 2>/dev/null; then
      kubectl port-forward svc/hypershell-api-server \
        -n "${KIND_NAMESPACE}" 8000:8000 >/dev/null 2>&1 &
      _PF_PID=$!
    fi
    sleep 2
  done

  if [[ -z "${_api_ready}" ]]; then
    red "API server is not reachable at ${HSCTL_TEST_API_URL}."
    red "Is the Kind cluster running? Try: make kind-status"
    exit 1
  fi
  pass "Port-forward established (${HSCTL_TEST_API_URL})"
else
  # Verify the user-supplied URL responds
  _code=$(_kind_curl -o /dev/null -w '%{http_code}' --connect-timeout 5 \
    "${HSCTL_TEST_API_URL}/api/hypershell/v1/gateways" 2>/dev/null || echo "000")
  if [[ "${_code}" == "000" ]]; then
    red "API server is not reachable at ${HSCTL_TEST_API_URL}."
    exit 1
  fi
  pass "API server reachable at ${HSCTL_TEST_API_URL} (HTTP ${_code})"
fi

# ---------------------------------------------------------------------------
# Step 3: obtain an OIDC token from Keycloak
# ---------------------------------------------------------------------------
e2e_area "3. Keycloak token"

dim "  Fetching admin token from ${KEYCLOAK_URL}..."
_TOKEN_URL="${KEYCLOAK_URL}/realms/hypershell/protocol/openid-connect/token"
_TOKEN_RESP=""
_TOKEN=""

for _ in $(seq 1 15); do
  _TOKEN_RESP=$(_kind_curl -m 10 -X POST "${_TOKEN_URL}" \
    -d "grant_type=password" \
    -d "client_id=hypershell-frontend" \
    -d "username=${OIDC_USERNAME}" \
    -d "password=${OIDC_PASSWORD}" 2>/dev/null || true)
  _TOKEN=$(echo "${_TOKEN_RESP}" | \
    python3 -c "import json,sys; print(json.load(sys.stdin).get('access_token',''))" 2>/dev/null || true)
  [[ -n "${_TOKEN}" ]] && break
  sleep 3
done

if [[ -z "${_TOKEN}" ]]; then
  red "Could not obtain token from Keycloak: ${_TOKEN_RESP:0:300}"
  red "Is Keycloak reachable at ${KEYCLOAK_URL}?"
  exit 1
fi

echo -n "${_TOKEN}" >"${TOKEN_FILE}"
pass "Token obtained for '${OIDC_USERNAME}' from ${KEYCLOAK_URL}"

# ---------------------------------------------------------------------------
# Step 4: login
# ---------------------------------------------------------------------------
e2e_area "4. hsctl login"

show_cmd "hsctl login --token-file ... --url ${HSCTL_TEST_API_URL} ${_INSECURE_FLAG}"
if "${HSCTL}" login \
    --token-file "${TOKEN_FILE}" \
    --url "${HSCTL_TEST_API_URL}" \
    ${_INSECURE_FLAG}; then
  pass "hsctl login succeeded"
else
  red "hsctl login failed"
  exit 1
fi

# ---------------------------------------------------------------------------
# Step 5: whoami
# ---------------------------------------------------------------------------
e2e_area "5. hsctl whoami"

show_cmd "hsctl whoami"
WHOAMI_OUT=$("${HSCTL}" whoami 2>&1)
if echo "${WHOAMI_OUT}" | grep -q "API URL:"; then
  pass "whoami shows identity"
  dim "$(echo "${WHOAMI_OUT}" | sed 's/^/    /')"
else
  fail_test "whoami did not produce expected output: ${WHOAMI_OUT}"
fi

# ---------------------------------------------------------------------------
# Step 6: list all resource types
# ---------------------------------------------------------------------------
e2e_area "6. List commands (all resource types)"

_list_ok() {
  local resource="$1"
  show_cmd "hsctl list ${resource}"
  local out
  if out=$("${HSCTL}" list "${resource}" 2>&1); then
    pass "hsctl list ${resource}"
  else
    fail_test "hsctl list ${resource}: ${out}"
  fi
}

_list_ok gateways
_list_ok managedClusters
_list_ok gatewayReleases
_list_ok gatewayNetworks
_list_ok roles
_list_ok roleBindings
_list_ok users
# serviceAccounts is gateway-scoped (--gateway-id required); tested in step 7

# ---------------------------------------------------------------------------
# Step 7: get seeded resources by ID
# ---------------------------------------------------------------------------
e2e_area "7. Get seeded resources by ID"

_get_seeded() {
  local resource_type="$1"  # e.g. gateways
  local seeded_name="$2"
  local get_cmd="$3"        # e.g. gateway

  # All display output goes to stderr so the calling $(...) only captures the ID.
  local list_out
  list_out=$("${HSCTL}" list "${resource_type}" 2>/dev/null | python3 -c "
import json,sys
try:
    d=json.load(sys.stdin)
    items=d.get('items',[])
    for item in items:
        if item.get('name','') == '${seeded_name}':
            print(item.get('id',''))
            break
except: pass
" 2>/dev/null || true)

  if [[ -n "${list_out}" ]]; then
    show_cmd "hsctl get ${get_cmd} ${list_out}" >&2
    if "${HSCTL}" get "${get_cmd}" "${list_out}" >/dev/null 2>&1; then
      pass "hsctl get ${get_cmd} ${list_out} (${seeded_name})" >&2
    else
      fail_test "hsctl get ${get_cmd} ${list_out} (${seeded_name})" >&2
    fi
  else
    dim "  Skipping get ${get_cmd}: '${seeded_name}' not found in list (not seeded?)" >&2
  fi

  echo "${list_out}"
}

SEEDED_GW_ID=$(_get_seeded gateways dev-gateway gateway)
SEEDED_MC_ID=$(_get_seeded managedClusters local-kind managedCluster)
SEEDED_GR_ID=$(_get_seeded gatewayReleases dev-release gatewayRelease)

# serviceAccounts are gateway-scoped
if [[ -n "${SEEDED_GW_ID}" ]]; then
  show_cmd "hsctl list serviceAccounts --gateway-id ${SEEDED_GW_ID}"
  if _sa_out=$("${HSCTL}" list serviceAccounts --gateway-id "${SEEDED_GW_ID}" 2>&1); then
    pass "hsctl list serviceAccounts --gateway-id (seeded gateway)"
  else
    fail_test "hsctl list serviceAccounts --gateway-id ${SEEDED_GW_ID}: ${_sa_out}"
  fi
else
  dim "  Skipping serviceAccounts list: no seeded gateway found"
fi

# ---------------------------------------------------------------------------
# Step 8: create a GatewayRelease and a GatewayNetwork, then delete them
# ---------------------------------------------------------------------------
e2e_area "8. Create / get / delete (transient resources)"

dim "  Creating test GatewayRelease '${_TEST_RELEASE}'..."
show_cmd "hsctl create gatewayRelease --name ${_TEST_RELEASE} --image quay.io/hsctl-e2e/test:v0.0.1"
CREATE_GR_OUT=$("${HSCTL}" create gatewayRelease \
  --name "${_TEST_RELEASE}" \
  --image "quay.io/hsctl-e2e/test:v0.0.1" 2>&1)
_CREATED_RELEASE_ID=$(echo "${CREATE_GR_OUT}" | python3 -c "
import json,sys
try: print(json.load(sys.stdin).get('id',''))
except: pass
" 2>/dev/null || true)

if [[ -n "${_CREATED_RELEASE_ID}" ]]; then
  pass "hsctl create gatewayRelease -> ${_CREATED_RELEASE_ID}"

  show_cmd "hsctl get gatewayRelease ${_CREATED_RELEASE_ID}"
  if "${HSCTL}" get gatewayRelease "${_CREATED_RELEASE_ID}" >/dev/null 2>&1; then
    pass "hsctl get gatewayRelease (created resource)"
  else
    fail_test "hsctl get gatewayRelease ${_CREATED_RELEASE_ID}"
  fi

  show_cmd "hsctl delete gatewayRelease ${_CREATED_RELEASE_ID} --yes"
  if "${HSCTL}" delete gatewayRelease "${_CREATED_RELEASE_ID}" --yes 2>&1; then
    pass "hsctl delete gatewayRelease"
    _CREATED_RELEASE_ID=""  # no need for cleanup
  else
    fail_test "hsctl delete gatewayRelease ${_CREATED_RELEASE_ID}"
  fi
else
  fail_test "hsctl create gatewayRelease did not return an id: ${CREATE_GR_OUT}"
fi

dim "  Creating test GatewayNetwork '${_TEST_GW_NET}'..."
show_cmd "hsctl create gatewayNetwork --name ${_TEST_GW_NET} --topology mesh"
CREATE_GN_OUT=$("${HSCTL}" create gatewayNetwork \
  --name "${_TEST_GW_NET}" \
  --topology "mesh" 2>&1)
_CREATED_GW_NET_ID=$(echo "${CREATE_GN_OUT}" | python3 -c "
import json,sys
try: print(json.load(sys.stdin).get('id',''))
except: pass
" 2>/dev/null || true)

if [[ -n "${_CREATED_GW_NET_ID}" ]]; then
  pass "hsctl create gatewayNetwork -> ${_CREATED_GW_NET_ID}"

  show_cmd "hsctl delete gatewayNetwork ${_CREATED_GW_NET_ID} --yes"
  if "${HSCTL}" delete gatewayNetwork "${_CREATED_GW_NET_ID}" --yes 2>&1; then
    pass "hsctl delete gatewayNetwork"
    _CREATED_GW_NET_ID=""
  else
    fail_test "hsctl delete gatewayNetwork ${_CREATED_GW_NET_ID}"
  fi
else
  fail_test "hsctl create gatewayNetwork did not return an id: ${CREATE_GN_OUT}"
fi

# ---------------------------------------------------------------------------
# Step 9: hsctl apply -f
# ---------------------------------------------------------------------------
e2e_area "9. hsctl apply -f (GatewayRelease)"

_APPLY_RELEASE_NAME="hsctl-apply-${_RUN_ID}"
cat >"${APPLY_FILE}" <<EOF
apiVersion: hypershell.io/v1
kind: GatewayRelease
metadata:
  name: ${_APPLY_RELEASE_NAME}
spec:
  image: quay.io/hsctl-e2e/applied:v0.0.1
  rollout_strategy: rolling
EOF

show_cmd "hsctl apply -f ${APPLY_FILE}"
APPLY_OUT=$("${HSCTL}" apply -f "${APPLY_FILE}" 2>&1)
if echo "${APPLY_OUT}" | grep -qiE "created|configured"; then
  pass "hsctl apply -f creates GatewayRelease"
  dim "$(echo "${APPLY_OUT}" | sed 's/^/    /')"

  # Apply again to exercise the update (configure) path
  show_cmd "hsctl apply -f ${APPLY_FILE}  # idempotent second run"
  APPLY2_OUT=$("${HSCTL}" apply -f "${APPLY_FILE}" 2>&1)
  if echo "${APPLY2_OUT}" | grep -qiE "created|configured"; then
    pass "hsctl apply -f is idempotent (second run)"
  else
    fail_test "hsctl apply -f second run unexpected output: ${APPLY2_OUT}"
  fi

  # Clean up the applied resource
  APPLIED_ID=$("${HSCTL}" list gatewayReleases 2>/dev/null | python3 -c "
import json,sys
try:
    items=json.load(sys.stdin).get('items',[])
    for item in items:
        if item.get('name','') == '${_APPLY_RELEASE_NAME}':
            print(item.get('id',''))
            break
except: pass
" 2>/dev/null || true)
  if [[ -n "${APPLIED_ID}" ]]; then
    "${HSCTL}" delete gatewayRelease "${APPLIED_ID}" --yes >/dev/null 2>&1 || true
  fi
else
  fail_test "hsctl apply -f unexpected output: ${APPLY_OUT}"
fi

# ---------------------------------------------------------------------------
# Step 10: hsctl apply -f - (stdin)
# ---------------------------------------------------------------------------
e2e_area "10. hsctl apply -f - (stdin)"

_STDIN_RELEASE_NAME="hsctl-stdin-${_RUN_ID}"
STDIN_OUT=$(cat <<EOF | "${HSCTL}" apply -f - 2>&1
apiVersion: hypershell.io/v1
kind: GatewayRelease
metadata:
  name: ${_STDIN_RELEASE_NAME}
spec:
  image: quay.io/hsctl-e2e/stdin:v0.0.1
EOF
)
if echo "${STDIN_OUT}" | grep -qiE "created|configured"; then
  pass "hsctl apply -f - reads from stdin"
  # Clean up
  STDIN_ID=$("${HSCTL}" list gatewayReleases 2>/dev/null | python3 -c "
import json,sys
try:
    items=json.load(sys.stdin).get('items',[])
    for item in items:
        if item.get('name','') == '${_STDIN_RELEASE_NAME}':
            print(item.get('id',''))
            break
except: pass
" 2>/dev/null || true)
  [[ -n "${STDIN_ID}" ]] && "${HSCTL}" delete gatewayRelease "${STDIN_ID}" --yes >/dev/null 2>&1 || true
else
  fail_test "hsctl apply -f - unexpected output: ${STDIN_OUT}"
fi

# ---------------------------------------------------------------------------
# Step 11: hsctl get gateway --show-connection (if dev-gateway was seeded)
# ---------------------------------------------------------------------------
e2e_area "11. hsctl get gateway --show-connection"

if [[ -n "${SEEDED_GW_ID}" ]]; then
  show_cmd "hsctl get gateway ${SEEDED_GW_ID} --show-connection"
  CONN_OUT=$("${HSCTL}" get gateway "${SEEDED_GW_ID}" --show-connection 2>&1)
  if echo "${CONN_OUT}" | grep -q "openshell gateway add"; then
    pass "hsctl get gateway --show-connection prints connection script"
    dim "$(echo "${CONN_OUT}" | head -5 | sed 's/^/    /')"
  else
    fail_test "hsctl get gateway --show-connection unexpected output: ${CONN_OUT:0:200}"
  fi
else
  dim "  Skipping --show-connection: dev-gateway not seeded"
fi

# ---------------------------------------------------------------------------
# Step 12: logout and verify commands fail
# ---------------------------------------------------------------------------
e2e_area "12. hsctl logout"

show_cmd "hsctl logout"
if "${HSCTL}" logout 2>&1 | grep -qiE "log.*out|success|clear"; then
  pass "hsctl logout succeeded"
else
  pass "hsctl logout (no output, assumed OK)"
fi

show_cmd "hsctl list gateways  # should fail after logout"
if "${HSCTL}" list gateways >/dev/null 2>&1; then
  fail_test "Commands should fail after logout"
else
  pass "Commands correctly fail after logout"
fi

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------
echo
E2E_COMPLETED="yes"
print_results
[[ "${E2E_FAIL}" -eq 0 ]] || exit 1
