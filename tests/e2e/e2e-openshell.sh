#!/usr/bin/env bash
# e2e-openshell.sh - infrastructure-agnostic end-to-end test of the OpenShell
# gateway provisioned by HyperShell.
#
# Proves the full path: HyperShell API -> control plane -> gateway provisioning
# -> openshell CLI -> sandbox pod creation + interaction.
#
# The E2E_INFRA_DRIVER environment variable selects the infrastructure driver.
# Each driver (tests/e2e/drivers/<driver>.sh) implements a fixed set of
# functions that abstract infrastructure-specific operations.
#
# Usage:
#   E2E_INFRA_DRIVER=kind bash tests/e2e/e2e-openshell.sh
#
# Environment variables:
#   E2E_INFRA_DRIVER      (required) Infra driver: kind, openshift (follow-up)
#   E2E_NAMESPACE          Namespace for e2e resources (default: openshell-e2e)
#   E2E_GATEWAY_NAME       Gateway name (default: e2e-gw)
#   E2E_SANDBOX_TIMEOUT    Seconds to wait for sandbox (default: 120)
#   E2E_PROVISION_TIMEOUT  Seconds to wait for gateway provisioning (default: 180)
#   E2E_SKIP_CLEANUP       Set to 1 to keep test resources after run (default: 0)
#   OPENSHELL_BIN          Path to the openshell CLI binary (default: openshell)
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# --- Source shared utilities ---
# shellcheck source=lib.sh
source "${SCRIPT_DIR}/lib.sh"

# --- Driver selection and validation ---

list_available_drivers() {
  local drivers_dir="${SCRIPT_DIR}/drivers"
  if [[ -d "$drivers_dir" ]]; then
    for f in "${drivers_dir}"/*.sh; do
      [[ -f "$f" ]] && basename "$f" .sh
    done
  fi
}

if [[ -z "${E2E_INFRA_DRIVER:-}" ]]; then
  red "ERROR: E2E_INFRA_DRIVER is not set."
  echo ""
  echo "Available drivers:"
  list_available_drivers | while read -r d; do echo "  - $d"; done
  exit 1
fi

DRIVER_FILE="${SCRIPT_DIR}/drivers/${E2E_INFRA_DRIVER}.sh"
if [[ ! -f "$DRIVER_FILE" ]]; then
  red "ERROR: Unknown driver '${E2E_INFRA_DRIVER}'. Driver file not found: ${DRIVER_FILE}"
  echo ""
  echo "Available drivers:"
  list_available_drivers | while read -r d; do echo "  - $d"; done
  exit 1
fi

# shellcheck source=drivers/kind.sh
source "$DRIVER_FILE"

REQUIRED_FUNCTIONS=(discover_api_host discover_gateway_endpoint get_cluster_domain get_cli_binary wait_for_gateway_route)
for fn in "${REQUIRED_FUNCTIONS[@]}"; do
  if ! declare -f "$fn" >/dev/null 2>&1; then
    red "ERROR: Driver '${E2E_INFRA_DRIVER}' does not implement required function: ${fn}"
    exit 1
  fi
done

# --- Configuration ---

CLI=$(get_cli_binary)
GW_NAME="${E2E_GATEWAY_NAME}"
GW_NAMESPACE=""
GW_ID=""
SANDBOX_NAME=""
E2E_PF_PID="${E2E_PF_PID:-}"
E2E_GW_PF_PID="${E2E_GW_PF_PID:-}"
E2E_HS_NAMESPACE="${E2E_HS_NAMESPACE:-hypershell-system}"

if ! command -v "${OPENSHELL_BIN}" >/dev/null 2>&1; then
  red "ERROR: openshell CLI not found (OPENSHELL_BIN=${OPENSHELL_BIN})"
  red "Install: curl -LsSf https://raw.githubusercontent.com/NVIDIA/OpenShell/main/install.sh | sh"
  exit 1
fi

# --- Cleanup trap ---

cleanup() {
  if [[ -n "${SB_CREATE_PID:-}" ]]; then
    kill "$SB_CREATE_PID" 2>/dev/null || true
    wait "$SB_CREATE_PID" 2>/dev/null || true
  fi
  if [[ -n "${E2E_GW_PF_PID:-}" ]]; then
    kill "$E2E_GW_PF_PID" 2>/dev/null || true
    wait "$E2E_GW_PF_PID" 2>/dev/null || true
  fi
  if [[ -n "${E2E_PF_PID:-}" ]]; then
    kill "$E2E_PF_PID" 2>/dev/null || true
    wait "$E2E_PF_PID" 2>/dev/null || true
  fi
  if [[ "$E2E_SKIP_CLEANUP" != "1" && -n "$GW_ID" ]]; then
    dim "  Cleaning up gateway ${GW_NAME}..."
    curl -sk -X DELETE "https://${API_HOST}/api/hypershell/v1/gateways/${GW_ID}" &>/dev/null || true
  fi
}
trap cleanup EXIT

# --- Discover API host via driver ---

API_HOST=$(discover_api_host)
if [[ -z "$API_HOST" ]]; then
  red "ERROR: Could not discover HyperShell API host"
  exit 1
fi

# --- Banner ---

echo ""
bold "HyperShell OpenShell Gateway End-to-End Test"
sep
echo ""
printf '  %s\n' "1. Gateway provisioning via HyperShell API"
printf '  %s\n' "2. Gateway infrastructure verification"
printf '  %s\n' "3. Route discovery + openshell CLI registration"
printf '  %s\n' "4. Gateway connectivity"
printf '  %s\n' "5. Sandbox lifecycle (create → ready)"
printf '  %s\n' "6. Sandbox interaction"
echo ""
dim  "  Driver:            ${E2E_INFRA_DRIVER}"
dim  "  HyperShell API:    https://${API_HOST}"
dim  "  Gateway name:      ${GW_NAME}"
dim  "  Sandbox timeout:   ${E2E_SANDBOX_TIMEOUT}s"
echo ""
sep

# ── 1. gateway provisioning ────────────────────────────────────────────────

echo ""
bold "1. Gateway Provisioning via HyperShell API"
echo ""

show_cmd "curl -sk https://${API_HOST}/api/hypershell/v1/gateways?search=name%3D${GW_NAME}"
EXISTING_GW=$(curl -sk "https://${API_HOST}/api/hypershell/v1/gateways?search=name%3D${GW_NAME}" 2>/dev/null || true)
EXISTING_ID=$(echo "$EXISTING_GW" | python3 -c "
import json,sys
data = json.load(sys.stdin)
items = data.get('items', [])
for gw in items:
    if gw.get('name','') == '${GW_NAME}':
        print(gw['id'])
        break
" 2>/dev/null || true)

if [[ -n "$EXISTING_ID" ]]; then
  GW_ID="$EXISTING_ID"
  GW_NAMESPACE=$(echo "$EXISTING_GW" | python3 -c "
import json,sys
data = json.load(sys.stdin)
for gw in data.get('items', []):
    if gw.get('id','') == '${GW_ID}':
        print(gw.get('namespace',''))
        break
" 2>/dev/null || true)
  GW_PHASE=$(echo "$EXISTING_GW" | python3 -c "
import json,sys
data = json.load(sys.stdin)
for gw in data.get('items', []):
    if gw.get('id','') == '${GW_ID}':
        print(gw.get('phase',''))
        break
" 2>/dev/null || true)
  pass "Gateway already exists: ${GW_NAME} (${GW_ID}, phase=${GW_PHASE})"
else
  show_cmd "curl -sk -X POST https://${API_HOST}/api/hypershell/v1/gateways -d '{name: ${GW_NAME}, ...}'"
  CREATE_RESPONSE=$(curl -sk -X POST "https://${API_HOST}/api/hypershell/v1/gateways" \
    -H "Content-Type: application/json" \
    -d "{
      \"name\": \"${GW_NAME}\",
      \"fleet_id\": \"e2e-fleet\",
      \"cluster_id\": \"e2e-cluster\",
      \"release_id\": \"e2e-release\",
      \"database_id\": \"e2e-db\"
    }" 2>/dev/null || true)

  GW_ID=$(echo "$CREATE_RESPONSE" | python3 -c "import json,sys; print(json.load(sys.stdin).get('id',''))" 2>/dev/null || true)
  GW_NAMESPACE=$(echo "$CREATE_RESPONSE" | python3 -c "import json,sys; print(json.load(sys.stdin).get('namespace',''))" 2>/dev/null || true)

  if [[ -n "$GW_ID" ]]; then
    pass "Gateway created: ${GW_NAME} (${GW_ID})"
  else
    fail_test "Failed to create gateway"
    dim "    ${CREATE_RESPONSE:0:300}"
    exit 1
  fi

  dim "  Waiting for controller to provision (timeout: ${E2E_PROVISION_TIMEOUT}s)..."
  DEADLINE=$(($(date +%s) + E2E_PROVISION_TIMEOUT))
  GW_PHASE=""
  while [[ $(date +%s) -lt $DEADLINE ]]; do
    GW_PHASE=$(curl -sk "https://${API_HOST}/api/hypershell/v1/gateways/${GW_ID}" 2>/dev/null | \
      python3 -c "import json,sys; print(json.load(sys.stdin).get('phase',''))" 2>/dev/null || true)
    if [[ "$GW_PHASE" == "Running" ]]; then
      break
    fi
    dim "    phase: ${GW_PHASE:-unknown}"
    sleep 5
  done

  if [[ "$GW_PHASE" == "Running" ]]; then
    pass "Gateway provisioned and running"
  else
    fail_test "Gateway not running after ${E2E_PROVISION_TIMEOUT}s (phase=${GW_PHASE})"
    exit 1
  fi
fi

if [[ -z "$GW_NAMESPACE" ]]; then
  fail_test "Gateway response did not include a server-assigned namespace"
  exit 1
fi
dim "  Gateway namespace: ${GW_NAMESPACE}"
sep

# ── 2. gateway infrastructure ──────────────────────────────────────────────

echo ""
bold "2. Gateway Infrastructure"
echo ""

show_cmd "$CLI get deployment openshell-gateway -n $GW_NAMESPACE"
if $CLI get deployment openshell-gateway -n "$GW_NAMESPACE" &>/dev/null; then
  dim "  Waiting for gateway pod to be ready (up to 90s)..."
  GW_READY=0
  GW_READY_DEADLINE=$(($(date +%s) + 90))
  while [[ $(date +%s) -lt $GW_READY_DEADLINE ]]; do
    GW_READY=$($CLI get deployment openshell-gateway -n "$GW_NAMESPACE" -o jsonpath='{.status.readyReplicas}' 2>/dev/null || echo 0)
    if [[ "${GW_READY:-0}" -ge 1 ]]; then
      break
    fi
    sleep 5
  done
  GW_IMAGE=$($CLI get deployment openshell-gateway -n "$GW_NAMESPACE" -o jsonpath='{.spec.template.spec.containers[0].image}' 2>/dev/null || echo unknown)
  if [[ "${GW_READY:-0}" -ge 1 ]]; then
    pass "Gateway pod ready ($GW_IMAGE)"
  else
    fail_test "Gateway pod not ready after 90s (${GW_READY:-0} replicas)"
  fi
else
  fail_test "Gateway Deployment not found in $GW_NAMESPACE"
fi

show_cmd "$CLI get service openshell-gateway -n $GW_NAMESPACE"
GW_SVC=$($CLI get service openshell-gateway -n "$GW_NAMESPACE" -o jsonpath='{.spec.clusterIP}' 2>/dev/null || true)
if [[ -n "$GW_SVC" ]]; then
  pass "Gateway service: ${GW_SVC}:8080"
else
  fail_test "Gateway service not found"
fi

show_cmd "$CLI get secret openshell-server-tls -n $GW_NAMESPACE"
HAS_TLS=$($CLI get secret openshell-server-tls -n "$GW_NAMESPACE" 2>/dev/null && echo yes || true)
if [[ -n "$HAS_TLS" ]]; then
  pass "TLS certificates provisioned"
else
  dim "  - TLS secret not found (certgen job may still be running)"
fi

show_cmd "$CLI get jobs -n $GW_NAMESPACE"
CERTGEN_STATUS=$($CLI get job openshell-gateway-certgen -n "$GW_NAMESPACE" -o jsonpath='{.status.succeeded}' 2>/dev/null || echo 0)
if [[ "${CERTGEN_STATUS:-0}" -ge 1 ]]; then
  pass "Certificate generation job completed"
else
  dim "  - Certgen job status: ${CERTGEN_STATUS:-unknown}"
fi
sep

# ── 3. route discovery + CLI registration ─────────────────────────────────

echo ""
bold "3. Route Discovery + CLI Registration"
echo ""

GW_LOCAL_NAME="${GW_NAMESPACE}-openshell"

GW_ENDPOINT=$(discover_gateway_endpoint "$GW_NAME" "$GW_NAMESPACE")
if [[ -n "$GW_ENDPOINT" ]]; then
  pass "Gateway endpoint: ${GW_ENDPOINT}"
else
  fail_test "Could not discover gateway endpoint"
  exit 1
fi

wait_for_gateway_route "$GW_NAME" "$GW_NAMESPACE" && \
  pass "Gateway route is ready" || \
  dim "  - Route readiness check timed out (continuing)"

GW_CONFIG_DIR="${HOME}/.config/openshell/gateways/${GW_LOCAL_NAME}"
mkdir -p "${GW_CONFIG_DIR}"

show_cmd "${OPENSHELL_BIN} gateway remove ${GW_LOCAL_NAME}"
"${OPENSHELL_BIN}" gateway remove "${GW_LOCAL_NAME}" 2>/dev/null || true
mkdir -p "${GW_CONFIG_DIR}"

show_cmd "# write gateway metadata (no-auth mode)"
python3 -c "
import json, os
meta = {
    'name': '${GW_LOCAL_NAME}',
    'gateway_endpoint': '${GW_ENDPOINT}',
    'is_remote': True,
    'gateway_port': 0,
    'auth_mode': 'none'
}
with open('${GW_CONFIG_DIR}/metadata.json', 'w') as f:
    json.dump(meta, f, indent=2)
"

if [[ -f "${GW_CONFIG_DIR}/metadata.json" ]]; then
  pass "openshell CLI registered (no-auth mode)"
else
  fail_test "Failed to write gateway config"
fi
sep

# ── 4. gateway connectivity ───────────────────────────────────────────────

echo ""
bold "4. Gateway Connectivity"
echo ""

show_cmd "OPENSHELL_GATEWAY_INSECURE=true ${OPENSHELL_BIN} -g ${GW_LOCAL_NAME} status"
dim "  Waiting for route connectivity (up to 60s)..."
CONNECT_DEADLINE=$(($(date +%s) + 60))
STATUS_OUTPUT=""
CONNECTED=false
while [[ $(date +%s) -lt $CONNECT_DEADLINE ]]; do
  STATUS_OUTPUT=$(OPENSHELL_GATEWAY_INSECURE=true "${OPENSHELL_BIN}" -g "${GW_LOCAL_NAME}" status 2>&1 || true)
  CLEAN_STATUS=$(echo "$STATUS_OUTPUT" | sed 's/\x1b\[[0-9;]*m//g')
  if echo "$CLEAN_STATUS" | grep -qi "Connected"; then
    CONNECTED=true
    break
  fi
  sleep 5
done

if [[ "$CONNECTED" == "true" ]]; then
  GW_VERSION=$(echo "$CLEAN_STATUS" | grep -oP 'Version:\s*\K\S+' || echo "unknown")
  pass "Gateway connected (version: ${GW_VERSION})"
  echo "$STATUS_OUTPUT" | while IFS= read -r line; do
    dim "    $line"
  done
else
  fail_test "Gateway not reachable"
  echo "$STATUS_OUTPUT" | while IFS= read -r line; do
    dim "    $line"
  done
fi
sep

# ── 5. sandbox lifecycle ──────────────────────────────────────────────────

echo ""
bold "5. Sandbox Lifecycle"
echo ""

RUN_ID=$(date +%s | tail -c5)
SANDBOX_NAME="e2e-${RUN_ID}"

show_cmd "OPENSHELL_GATEWAY_INSECURE=true ${OPENSHELL_BIN} -g ${GW_LOCAL_NAME} sandbox create --name ${SANDBOX_NAME}"
dim "  Creating sandbox (timeout: ${E2E_SANDBOX_TIMEOUT}s)..."

OPENSHELL_GATEWAY_INSECURE=true "${OPENSHELL_BIN}" -g "${GW_LOCAL_NAME}" sandbox create --name "${SANDBOX_NAME}" &>/dev/null &
SB_CREATE_PID=$!

DEADLINE=$(($(date +%s) + E2E_SANDBOX_TIMEOUT))
SANDBOX_FOUND=false
POD_NAME=""
POD_STATUS=""
while [[ $(date +%s) -lt $DEADLINE ]]; do
  SANDBOX_PODS=$($CLI get pods -n "$GW_NAMESPACE" --no-headers 2>/dev/null | grep -i "default--${SANDBOX_NAME}" || true)
  if [[ -n "$SANDBOX_PODS" ]]; then
    POD_STATUS=$(echo "$SANDBOX_PODS" | awk '{print $3}' | head -1)
    POD_NAME=$(echo "$SANDBOX_PODS" | awk '{print $1}' | head -1)
    if [[ "$POD_STATUS" == "Running" ]]; then
      SANDBOX_FOUND=true
      break
    fi
    dim "    pod: ${POD_NAME} (${POD_STATUS})"
  fi
  sleep 5
done

kill "$SB_CREATE_PID" 2>/dev/null || true
wait "$SB_CREATE_PID" 2>/dev/null || true
SB_CREATE_PID=""

show_cmd "$CLI get pods -n $GW_NAMESPACE --no-headers | grep ${SANDBOX_NAME}"

if [[ "$SANDBOX_FOUND" == "true" ]]; then
  pass "Sandbox pod created: ${POD_NAME} (${POD_STATUS})"
else
  SANDBOX_PODS=$($CLI get pods -n "$GW_NAMESPACE" --no-headers 2>/dev/null | grep -i "default--${SANDBOX_NAME}" || true)
  if [[ -n "$SANDBOX_PODS" ]]; then
    POD_STATUS=$(echo "$SANDBOX_PODS" | awk '{print $3}' | head -1)
    POD_NAME=$(echo "$SANDBOX_PODS" | awk '{print $1}' | head -1)
    pass "Sandbox pod created: ${POD_NAME} (${POD_STATUS})"
  else
    fail_test "Sandbox not found after ${E2E_SANDBOX_TIMEOUT}s"
  fi
fi
sep

# ── 6. sandbox interaction ────────────────────────────────────────────────

echo ""
bold "6. Sandbox Interaction"
echo ""

GW_FLAG="-g ${GW_LOCAL_NAME}"
INSECURE_ENV="OPENSHELL_GATEWAY_INSECURE=true"

show_cmd "${INSECURE_ENV} ${OPENSHELL_BIN} ${GW_FLAG} sandbox exec -n ${SANDBOX_NAME} -- uname -a"
if SB_EXEC_OUTPUT=$(OPENSHELL_GATEWAY_INSECURE=true "${OPENSHELL_BIN}" -g "${GW_LOCAL_NAME}" sandbox exec -n "${SANDBOX_NAME}" -- uname -a 2>&1); then
  CLEAN_EXEC=$(echo "$SB_EXEC_OUTPUT" | sed 's/\x1b\[[0-9;]*m//g' | grep -v '^ *$' | grep -v 'WARN' | tail -3)
  if [[ -n "$CLEAN_EXEC" ]]; then
    pass "Sandbox exec: command executed inside sandbox"
    echo "$CLEAN_EXEC" | while IFS= read -r line; do
      dim "    $line"
    done
  else
    fail_test "Sandbox exec: no output from uname command"
    dim "    ${SB_EXEC_OUTPUT:0:200}"
  fi
else
  fail_test "Sandbox exec: openshell command failed"
  dim "    ${SB_EXEC_OUTPUT:0:200}"
fi

show_cmd "${INSECURE_ENV} ${OPENSHELL_BIN} ${GW_FLAG} sandbox exec -n ${SANDBOX_NAME} -- ls -la /workspace"
if SB_LS_OUTPUT=$(OPENSHELL_GATEWAY_INSECURE=true "${OPENSHELL_BIN}" -g "${GW_LOCAL_NAME}" sandbox exec -n "${SANDBOX_NAME}" -- ls -la /workspace 2>&1); then
  CLEAN_LS=$(echo "$SB_LS_OUTPUT" | sed 's/\x1b\[[0-9;]*m//g' | grep -v '^ *$' | grep -v 'WARN' | tail -5)
  if [[ -n "$CLEAN_LS" ]]; then
    pass "Sandbox workspace: /workspace directory listing"
    echo "$CLEAN_LS" | while IFS= read -r line; do
      dim "    $line"
    done
  else
    fail_test "Sandbox workspace: no output from ls command"
    dim "    ${SB_LS_OUTPUT:0:200}"
  fi
else
  if echo "$SB_LS_OUTPUT" | grep -q "No such file or directory"; then
    dim "  - /workspace not available (using default working directory)"
  else
    fail_test "Sandbox workspace: openshell ls command failed"
    dim "    ${SB_LS_OUTPUT:0:200}"
  fi
fi

# ── cleanup ───────────────────────────────────────────────────────────────

if [[ "$E2E_SKIP_CLEANUP" != "1" && -n "$SANDBOX_NAME" ]]; then
  echo ""
  dim "  Cleaning up sandbox..."
  show_cmd "${INSECURE_ENV} ${OPENSHELL_BIN} ${GW_FLAG} sandbox delete ${SANDBOX_NAME}"
  OPENSHELL_GATEWAY_INSECURE=true "${OPENSHELL_BIN}" -g "${GW_LOCAL_NAME}" sandbox delete "${SANDBOX_NAME}" 2>&1 || true
  dim "  Sandbox deleted"
fi
sep

# ── results ───────────────────────────────────────────────────────────────

print_results

if [[ $E2E_FAIL -gt 0 ]]; then
  exit 1
fi
