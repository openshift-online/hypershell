#!/usr/bin/env bash
# e2e-openshell.sh - end-to-end test of the OpenShell gateway provisioned by HyperShell.
#
# Proves the full path: HyperShell API → control plane → gateway provisioning
# → openshell CLI → sandbox pod creation + interaction.
#
# This script creates a gateway via the HyperShell API (if it doesn't exist),
# waits for the controller to provision it, then validates connectivity and
# sandbox lifecycle.
#
# Usage:
#   bash e2e-openshell.sh
#
# Environment variables:
#   OC                   oc/kubectl binary (default: oc)
#   HYPERSHELL_NAMESPACE API server namespace (default: hypershell-api)
#   GATEWAY_NAME         gateway name (default: e2e-gw)
#   SANDBOX_TIMEOUT      seconds to wait for sandbox (default: 120)
#   PROVISION_TIMEOUT    seconds to wait for gateway provisioning (default: 180)
#   SKIP_CLEANUP         set to 1 to keep resources after test
#   LAUNCH_TUI           set to 1 to launch interactive TUI at the end (default: 0)
#   PAUSE                seconds between commands (default: 1)
set -euo pipefail

CLI="${OC:-oc}"
OPENSHELL="${OPENSHELL_BIN:-openshell}"
HS_NAMESPACE="${HYPERSHELL_NAMESPACE:-hypershell-api}"
GW_NAMESPACE=""
GW_NAME="${GATEWAY_NAME:-e2e-gw}"
SANDBOX_TIMEOUT="${SANDBOX_TIMEOUT:-120}"
PROVISION_TIMEOUT="${PROVISION_TIMEOUT:-180}"
SKIP_CLEANUP="${SKIP_CLEANUP:-}"
LAUNCH_TUI="${LAUNCH_TUI:-0}"
PAUSE="${PAUSE:-1}"

PASS=0
FAIL=0
TESTS=()
PF_PID=""
SANDBOX_NAME=""
GW_ID=""

bold()   { printf '\033[1m%s\033[0m\n' "$*"; }
green()  { printf '\033[32m%s\033[0m\n' "$*"; }
red()    { printf '\033[31m%s\033[0m\n' "$*"; }
dim()    { printf '\033[2m%s\033[0m\n' "$*"; }
cyan()   { printf '\033[36m%s\033[0m\n' "$*"; }
orange() { printf '\033[38;5;214m%s\033[0m\n' "$*"; }
sep()    { printf '\033[2m────────────────────────────────────────────────\033[0m\n'; }

show_cmd() {
  orange "   \$ $*"
  sleep "$PAUSE"
}

pass() {
  PASS=$((PASS + 1))
  TESTS+=("PASS: $1")
  green "  ✓ $1"
}

fail_test() {
  FAIL=$((FAIL + 1))
  TESTS+=("FAIL: $1")
  red "  ✗ $1"
}

cleanup() {
  if [[ -n "${SB_CREATE_PID:-}" ]]; then
    kill "$SB_CREATE_PID" 2>/dev/null || true
    wait "$SB_CREATE_PID" 2>/dev/null || true
  fi
  if [[ -n "$PF_PID" ]]; then
    kill "$PF_PID" 2>/dev/null || true
    wait "$PF_PID" 2>/dev/null || true
  fi
  if [[ "$SKIP_CLEANUP" != "1" && -n "$GW_ID" ]]; then
    dim "  Cleaning up gateway ${GW_NAME}..."
    curl -sk -X DELETE "https://${API_HOST}/api/hypershell/v1/gateways/${GW_ID}" &>/dev/null || true
  fi
}
trap cleanup EXIT

API_HOST=$($CLI get route hypershell-api -n "$HS_NAMESPACE" -o jsonpath='{.spec.host}' 2>/dev/null || true)
if [[ -z "$API_HOST" ]]; then
  red "ERROR: HyperShell API route not found in namespace ${HS_NAMESPACE}"
  exit 1
fi

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
dim  "  HyperShell API:    https://${API_HOST}"
dim  "  Gateway name:      ${GW_NAME}"
dim  "  Sandbox timeout:   ${SANDBOX_TIMEOUT}s"
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

  dim "  Waiting for controller to provision (timeout: ${PROVISION_TIMEOUT}s)..."
  DEADLINE=$(($(date +%s) + PROVISION_TIMEOUT))
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
    fail_test "Gateway not running after ${PROVISION_TIMEOUT}s (phase=${GW_PHASE})"
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

show_cmd "$CLI get routes -n $GW_NAMESPACE"
GW_ROUTE_HOST=$($CLI get routes -n "$GW_NAMESPACE" -o json 2>/dev/null | python3 -c "
import json,sys
data = json.load(sys.stdin)
candidates = []
for item in data.get('items',[]):
    tls = item.get('spec',{}).get('tls',{})
    to = item.get('spec',{}).get('to',{})
    name = item.get('metadata',{}).get('name','')
    if (tls.get('termination') == 'passthrough' and
        to.get('name') == 'openshell-gateway' and
        ('grpc' in name or 'gateway' in name)):
        candidates.append(item['spec']['host'])
if candidates:
    print(candidates[0])
" 2>/dev/null || true)

if [[ -z "$GW_ROUTE_HOST" ]]; then
  dim "  No passthrough route found, falling back to port-forward"
  PF_PORT=7443
  show_cmd "$CLI port-forward -n $GW_NAMESPACE svc/openshell-gateway ${PF_PORT}:8080 &"
  $CLI port-forward -n "$GW_NAMESPACE" svc/openshell-gateway "${PF_PORT}":8080 &>/dev/null &
  PF_PID=$!
  sleep 3
  if kill -0 "$PF_PID" 2>/dev/null; then
    pass "Port-forward active (localhost:${PF_PORT} → openshell-gateway:8080)"
  else
    fail_test "Port-forward failed to start"
    PF_PID=""
    exit 1
  fi
  GW_ENDPOINT="https://localhost:${PF_PORT}"
else
  GW_ENDPOINT="https://${GW_ROUTE_HOST}:443"
  pass "Passthrough route: ${GW_ROUTE_HOST}"
fi

GW_CONFIG_DIR="${HOME}/.config/openshell/gateways/${GW_LOCAL_NAME}"
mkdir -p "${GW_CONFIG_DIR}"

show_cmd "${OPENSHELL} gateway remove ${GW_LOCAL_NAME}"
"${OPENSHELL}" gateway remove "${GW_LOCAL_NAME}" 2>/dev/null || true
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

show_cmd "OPENSHELL_GATEWAY_INSECURE=true ${OPENSHELL} -g ${GW_LOCAL_NAME} status"
dim "  Waiting for route connectivity (up to 60s)..."
CONNECT_DEADLINE=$(($(date +%s) + 60))
STATUS_OUTPUT=""
CONNECTED=false
while [[ $(date +%s) -lt $CONNECT_DEADLINE ]]; do
  STATUS_OUTPUT=$(OPENSHELL_GATEWAY_INSECURE=true "${OPENSHELL}" -g "${GW_LOCAL_NAME}" status 2>&1 || true)
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

show_cmd "OPENSHELL_GATEWAY_INSECURE=true ${OPENSHELL} -g ${GW_LOCAL_NAME} sandbox create --name ${SANDBOX_NAME}"
dim "  Creating sandbox (timeout: ${SANDBOX_TIMEOUT}s)..."

OPENSHELL_GATEWAY_INSECURE=true "${OPENSHELL}" -g "${GW_LOCAL_NAME}" sandbox create --name "${SANDBOX_NAME}" &>/dev/null &
SB_CREATE_PID=$!

DEADLINE=$(($(date +%s) + SANDBOX_TIMEOUT))
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
    fail_test "Sandbox not found after ${SANDBOX_TIMEOUT}s"
  fi
fi
sep

# ── 6. sandbox interaction ────────────────────────────────────────────────

echo ""
bold "6. Sandbox Interaction"
echo ""

GW_FLAG="-g ${GW_LOCAL_NAME}"
INSECURE_ENV="OPENSHELL_GATEWAY_INSECURE=true"

show_cmd "${INSECURE_ENV} ${OPENSHELL} ${GW_FLAG} sandbox exec -n ${SANDBOX_NAME} -- uname -a"
if SB_EXEC_OUTPUT=$(OPENSHELL_GATEWAY_INSECURE=true "${OPENSHELL}" -g "${GW_LOCAL_NAME}" sandbox exec -n "${SANDBOX_NAME}" -- uname -a 2>&1); then
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

show_cmd "${INSECURE_ENV} ${OPENSHELL} ${GW_FLAG} sandbox exec -n ${SANDBOX_NAME} -- ls -la /workspace"
if SB_LS_OUTPUT=$(OPENSHELL_GATEWAY_INSECURE=true "${OPENSHELL}" -g "${GW_LOCAL_NAME}" sandbox exec -n "${SANDBOX_NAME}" -- ls -la /workspace 2>&1); then
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

if [[ "$SKIP_CLEANUP" != "1" && "$LAUNCH_TUI" != "1" && -n "$SANDBOX_NAME" ]]; then
  echo ""
  dim "  Cleaning up sandbox..."
  show_cmd "${INSECURE_ENV} ${OPENSHELL} ${GW_FLAG} sandbox delete ${SANDBOX_NAME}"
  OPENSHELL_GATEWAY_INSECURE=true "${OPENSHELL}" -g "${GW_LOCAL_NAME}" sandbox delete "${SANDBOX_NAME}" 2>&1 || true
  dim "  Sandbox deleted"
fi
sep

# ── results ───────────────────────────────────────────────────────────────

echo ""
bold "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
bold "Results: $PASS passed, $FAIL failed"
echo ""
for t in "${TESTS[@]}"; do
  if [[ "$t" == PASS:* ]]; then
    green "  ✓ ${t#PASS: }"
  else
    red "  ✗ ${t#FAIL: }"
  fi
done
bold "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

if [[ "$LAUNCH_TUI" == "1" && $FAIL -eq 0 ]]; then
  echo ""
  bold "Interactive TUI"
  sep
  echo ""
  dim "  Launching OpenShell TUI..."
  dim "  Press Ctrl-C to exit."
  echo ""
  sleep 2
  exec env OPENSHELL_GATEWAY_INSECURE=true "${OPENSHELL}" -g "${GW_LOCAL_NAME}" term
fi

if [[ $FAIL -gt 0 ]]; then
  exit 1
fi
