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

REQUIRED_FUNCTIONS=(discover_api_host discover_gateway_endpoint get_cluster_domain get_cli_binary wait_for_gateway_route acquire_oidc_token api_curl)
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
  if [[ -n "${E2E_KC_PF_PID:-}" ]]; then
    kill "$E2E_KC_PF_PID" 2>/dev/null || true
    wait "$E2E_KC_PF_PID" 2>/dev/null || true
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
    curl -sk -X DELETE "${API_HOST}/api/hypershell/v1/gateways/${GW_ID}" &>/dev/null || true
  fi
}
trap cleanup EXIT

# --- Discover API host via driver ---

discover_api_host
API_HOST="${_DISCOVER_API_HOST}"
if [[ -z "$API_HOST" ]]; then
  red "ERROR: Could not discover HyperShell API host"
  exit 1
fi

# --- Banner ---

echo ""
bold "HyperShell OpenShell Gateway End-to-End Test"
sep
echo ""
printf '  %s\n' "1. Infrastructure validation"
printf '  %s\n' "2. Gateway provisioning via HyperShell API (OIDC)"
printf '  %s\n' "3. Gateway infrastructure verification"
printf '  %s\n' "4. OIDC token acquisition"
printf '  %s\n' "5. Route discovery + openshell CLI registration"
printf '  %s\n' "6. Gateway connectivity"
printf '  %s\n' "7. Sandbox lifecycle (create → ready)"
printf '  %s\n' "8. Sandbox interaction"
echo ""
dim  "  Driver:            ${E2E_INFRA_DRIVER}"
dim  "  HyperShell API:    ${API_HOST}"
dim  "  Gateway name:      ${GW_NAME}"
dim  "  OIDC issuer:       ${E2E_OIDC_ISSUER}"
dim  "  Sandbox timeout:   ${E2E_SANDBOX_TIMEOUT}s"
echo ""
sep

# ── 0. OIDC authentication verification ──────────────────────────────────

echo ""
bold "0. OIDC Authentication Verification"
echo ""

# Acquire a token for authenticated API calls
acquire_oidc_token
if [[ -n "${_OIDC_ACCESS_TOKEN}" ]]; then
  _API_AUTH_HEADER="Authorization: Bearer ${_OIDC_ACCESS_TOKEN}"
  pass "OIDC token acquired for API authentication"
else
  fail_test "Could not acquire OIDC token for API authentication"
  exit 1
fi

# Verify: unauthenticated API requests return 401
show_cmd "curl -sk -o /dev/null -w '%{http_code}' ${API_HOST}/api/hypershell/v1/gateways (no auth)"
UNAUTH_STATUS=$(curl -sk -o /dev/null -w '%{http_code}' "${API_HOST}/api/hypershell/v1/gateways" 2>/dev/null || true)
if [[ "$UNAUTH_STATUS" == "401" ]]; then
  pass "API server rejects unauthenticated requests (401)"
else
  fail_test "Expected 401 for unauthenticated request, got ${UNAUTH_STATUS}"
fi

# Verify: authenticated API requests return 200
show_cmd "curl -sk -H 'Authorization: Bearer ...' ${API_HOST}/api/hypershell/v1/gateways"
AUTH_STATUS=$(api_curl -o /dev/null -w '%{http_code}' "${API_HOST}/api/hypershell/v1/gateways" 2>/dev/null || true)
if [[ "$AUTH_STATUS" == "200" ]]; then
  pass "API server accepts authenticated requests (200)"
else
  fail_test "Expected 200 for authenticated request, got ${AUTH_STATUS}"
fi

# Verify: BFF /auth/session returns unauthenticated
CONSOLE_HOST="${API_HOST/api./console.}"
show_cmd "curl -sk ${CONSOLE_HOST}/auth/session"
SESSION_RESP=$(curl -sk "${CONSOLE_HOST}/auth/session" 2>/dev/null || true)
SESSION_AUTH=$(echo "${SESSION_RESP}" | python3 -c "import json,sys; print(json.load(sys.stdin).get('authenticated',''))" 2>/dev/null || true)
if [[ "$SESSION_AUTH" == "False" ]]; then
  pass "BFF /auth/session returns authenticated: false"
else
  fail_test "Expected authenticated: false from /auth/session, got: ${SESSION_RESP:0:100}"
fi

# Verify: BFF /auth/login redirects to Keycloak with PKCE
show_cmd "curl -sk -o /dev/null -w '%{redirect_url}' ${CONSOLE_HOST}/auth/login"
LOGIN_REDIRECT=$(curl -sk -o /dev/null -w '%{redirect_url}' "${CONSOLE_HOST}/auth/login" 2>/dev/null || true)
if echo "${LOGIN_REDIRECT}" | grep -q 'code_challenge_method=S256'; then
  pass "BFF /auth/login redirects to IdP with PKCE"
else
  fail_test "Expected PKCE redirect from /auth/login, got: ${LOGIN_REDIRECT:0:100}"
fi

# Verify: control plane gRPC streams are healthy
show_cmd "kubectl logs -l app=hypershell-controller --tail=20 | grep Unauthenticated"
CP_UNAUTH=$(${CLI} logs -l app=hypershell-controller -n "${E2E_HS_NAMESPACE}" --tail=50 2>/dev/null | grep -c 'Unauthenticated' || true)
if [[ "$CP_UNAUTH" == "0" ]]; then
  pass "Control plane gRPC streams: no Unauthenticated errors"
else
  fail_test "Control plane has ${CP_UNAUTH} Unauthenticated gRPC errors"
fi

sep

# ── 1. infrastructure validation ──────────────────────────────────────────

echo ""
bold "1. Infrastructure Validation"
echo ""

INFRA_NAMESPACE="${E2E_HS_NAMESPACE}"

show_cmd "$CLI get deployment cert-manager -n cert-manager"
CM_REPLICAS=$($CLI get deployment cert-manager -n cert-manager -o jsonpath='{.status.readyReplicas}' 2>/dev/null || echo "0")
if [[ "${CM_REPLICAS:-0}" -ge 1 ]]; then
  pass "cert-manager is ready"
else
  fail_test "cert-manager is not ready (readyReplicas=${CM_REPLICAS:-0})"
fi

show_cmd "$CLI get deployment cert-manager-webhook -n cert-manager"
CMW_REPLICAS=$($CLI get deployment cert-manager-webhook -n cert-manager -o jsonpath='{.status.readyReplicas}' 2>/dev/null || echo "0")
if [[ "${CMW_REPLICAS:-0}" -ge 1 ]]; then
  pass "cert-manager-webhook is ready"
else
  fail_test "cert-manager-webhook is not ready (readyReplicas=${CMW_REPLICAS:-0})"
fi

show_cmd "$CLI get deployment agent-sandbox-controller -n agent-sandbox-system"
AS_REPLICAS=$($CLI get deployment agent-sandbox-controller -n agent-sandbox-system -o jsonpath='{.status.readyReplicas}' 2>/dev/null || echo "0")
if [[ "${AS_REPLICAS:-0}" -ge 1 ]]; then
  pass "agent-sandbox controller is ready"
else
  fail_test "agent-sandbox controller is not ready (readyReplicas=${AS_REPLICAS:-0})"
fi

show_cmd "$CLI get crd gateways.gateway.networking.k8s.io"
if $CLI get crd gateways.gateway.networking.k8s.io &>/dev/null; then
  pass "Gateway API CRDs installed"
else
  fail_test "Gateway API CRDs not found"
fi

show_cmd "$CLI get issuer hypershell-selfsigned -n $INFRA_NAMESPACE"
SS_READY=$($CLI get issuer hypershell-selfsigned -n "$INFRA_NAMESPACE" -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null || echo "")
if [[ "$SS_READY" == "True" ]]; then
  pass "CA selfsigned Issuer is ready"
else
  fail_test "CA selfsigned Issuer is not ready (status=${SS_READY:-unknown})"
fi

show_cmd "$CLI get certificate hypershell-ca -n $INFRA_NAMESPACE"
CA_READY=$($CLI get certificate hypershell-ca -n "$INFRA_NAMESPACE" -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null || echo "")
if [[ "$CA_READY" == "True" ]]; then
  pass "CA Certificate issued"
else
  fail_test "CA Certificate not ready (status=${CA_READY:-unknown})"
fi

show_cmd "$CLI get issuer hypershell-ca-issuer -n $INFRA_NAMESPACE"
CAI_READY=$($CLI get issuer hypershell-ca-issuer -n "$INFRA_NAMESPACE" -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null || echo "")
if [[ "$CAI_READY" == "True" ]]; then
  pass "CA Issuer is ready"
else
  fail_test "CA Issuer is not ready (status=${CAI_READY:-unknown})"
fi

show_cmd "$CLI get deployment keycloak -n $E2E_KEYCLOAK_NAMESPACE"
KC_REPLICAS=$($CLI get deployment keycloak -n "$E2E_KEYCLOAK_NAMESPACE" -o jsonpath='{.status.readyReplicas}' 2>/dev/null || echo "0")
if [[ "${KC_REPLICAS:-0}" -ge 1 ]]; then
  pass "Keycloak is ready"
else
  fail_test "Keycloak is not ready (readyReplicas=${KC_REPLICAS:-0})"
fi

show_cmd "$CLI get networkpolicy -n $INFRA_NAMESPACE"
NP_COUNT=$($CLI get networkpolicy -n "$INFRA_NAMESPACE" --no-headers 2>/dev/null | wc -l | tr -d ' ')
if [[ "${NP_COUNT:-0}" -ge 4 ]]; then
  pass "NetworkPolicies present (${NP_COUNT} found)"
else
  fail_test "Expected at least 4 NetworkPolicies, found ${NP_COUNT:-0}"
fi
sep

# ── 2. gateway provisioning ────────────────────────────────────────────────

echo ""
bold "2. Gateway Provisioning via HyperShell API"
echo ""

show_cmd "curl -sk ${API_HOST}/api/hypershell/v1/gateways?search=name%3D${GW_NAME}"
EXISTING_GW=$(curl -sk "${API_HOST}/api/hypershell/v1/gateways?search=name%3D${GW_NAME}" 2>/dev/null || true)
EXISTING_ID=$(echo "$EXISTING_GW" | GW_NAME="$GW_NAME" python3 -c "
import json, sys, os
data = json.load(sys.stdin)
items = data.get('items', [])
for gw in items:
    if gw.get('name','') == os.environ['GW_NAME']:
        print(gw['id'])
        break
" 2>/dev/null || true)

if [[ -n "$EXISTING_ID" ]]; then
  GW_ID="$EXISTING_ID"
  GW_NAMESPACE=$(echo "$EXISTING_GW" | GW_ID="$GW_ID" python3 -c "
import json, sys, os
data = json.load(sys.stdin)
for gw in data.get('items', []):
    if gw.get('id','') == os.environ['GW_ID']:
        print(gw.get('namespace',''))
        break
" 2>/dev/null || true)
  GW_PHASE=$(echo "$EXISTING_GW" | GW_ID="$GW_ID" python3 -c "
import json, sys, os
data = json.load(sys.stdin)
for gw in data.get('items', []):
    if gw.get('id','') == os.environ['GW_ID']:
        print(gw.get('phase',''))
        break
" 2>/dev/null || true)
  pass "Gateway already exists: ${GW_NAME} (${GW_ID}, phase=${GW_PHASE})"
else
  show_cmd "curl -sk -X POST ${API_HOST}/api/hypershell/v1/gateways -d '{name: ${GW_NAME}, oidc: ...}'"
  GW_CREATE_BODY=$(GW_NAME="$GW_NAME" E2E_OIDC_ISSUER="$E2E_OIDC_ISSUER" \
    E2E_OIDC_CLIENT_ID="$E2E_OIDC_CLIENT_ID" python3 -c "
import json, os
body = {
    'name': os.environ['GW_NAME'],
    'fleet_id': 'e2e-fleet',
    'cluster_id': 'e2e-cluster',
    'release_id': 'e2e-release',
    'database_id': 'e2e-db',
    'oidc': json.dumps({
        'issuer': os.environ['E2E_OIDC_ISSUER'],
        'audience': os.environ['E2E_OIDC_CLIENT_ID'],
        'roles_claim': 'groups',
        'admin_role': 'hypershell-admins',
        'user_role': 'hypershell-users'
    }),
    'route': json.dumps({
        'enabled': True
    })
}
print(json.dumps(body))
")
  CREATE_RESPONSE=$(curl -sk -X POST "${API_HOST}/api/hypershell/v1/gateways" \
    -H "Content-Type: application/json" \
    -d "${GW_CREATE_BODY}" 2>/dev/null || true)

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
    GW_PHASE=$(curl -sk "${API_HOST}/api/hypershell/v1/gateways/${GW_ID}" 2>/dev/null | \
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

# ── 3. gateway infrastructure ──────────────────────────────────────────────

echo ""
bold "3. Gateway Infrastructure"
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
    dim "  --- gateway diagnostics ($GW_NAMESPACE) ---"
    dim "  Image: $GW_IMAGE"
    dim "  Pods:"
    $CLI get pods -n "$GW_NAMESPACE" -o wide 2>&1 | while IFS= read -r line; do dim "    $line"; done
    dim "  Events:"
    $CLI get events --sort-by=.lastTimestamp -n "$GW_NAMESPACE" 2>&1 | tail -20 | while IFS= read -r line; do dim "    $line"; done
    for pod in $($CLI get pods -n "$GW_NAMESPACE" -l app.kubernetes.io/component=gateway -o name 2>/dev/null); do
      dim "  Logs ${pod}:"
      $CLI logs "${pod}" --all-containers --tail=40 -n "$GW_NAMESPACE" 2>&1 | while IFS= read -r line; do dim "    $line"; done
      dim "  Previous logs ${pod}:"
      $CLI logs "${pod}" --all-containers --previous --tail=40 -n "$GW_NAMESPACE" 2>&1 | while IFS= read -r line; do dim "    $line"; done
    done
    dim "  Describe:"
    $CLI describe pods -l app.kubernetes.io/component=gateway -n "$GW_NAMESPACE" 2>&1 | while IFS= read -r line; do dim "    $line"; done
    dim "  ConfigMap:"
    $CLI get configmap openshell-gateway-config -n "$GW_NAMESPACE" -o yaml 2>&1 | while IFS= read -r line; do dim "    $line"; done
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

show_cmd "$CLI get deployment openshell-gateway-db -n $GW_NAMESPACE"
if $CLI get deployment openshell-gateway-db -n "$GW_NAMESPACE" &>/dev/null; then
  dim "  Waiting for database pod to be ready (up to 120s)..."
  DB_READY=0
  DB_READY_DEADLINE=$(($(date +%s) + 120))
  while [[ $(date +%s) -lt $DB_READY_DEADLINE ]]; do
    DB_READY=$($CLI get deployment openshell-gateway-db -n "$GW_NAMESPACE" -o jsonpath='{.status.readyReplicas}' 2>/dev/null || echo 0)
    if [[ "${DB_READY:-0}" -ge 1 ]]; then
      break
    fi
    sleep 5
  done
  DB_IMAGE=$($CLI get deployment openshell-gateway-db -n "$GW_NAMESPACE" -o jsonpath='{.spec.template.spec.containers[0].image}' 2>/dev/null || echo unknown)
  if [[ "${DB_READY:-0}" -ge 1 ]]; then
    pass "Database pod ready ($DB_IMAGE)"
  else
    fail_test "Database pod not ready after 120s (${DB_READY:-0} replicas)"
  fi
else
  fail_test "Database Deployment not found in $GW_NAMESPACE"
fi

show_cmd "$CLI get service openshell-gateway-db -n $GW_NAMESPACE"
DB_SVC=$($CLI get service openshell-gateway-db -n "$GW_NAMESPACE" -o jsonpath='{.spec.clusterIP}' 2>/dev/null || true)
if [[ -n "$DB_SVC" ]]; then
  pass "Database service: ${DB_SVC}:5432"
else
  fail_test "Database service not found"
fi

show_cmd "$CLI get pvc openshell-gateway-db-data -n $GW_NAMESPACE"
PVC_PHASE=$($CLI get pvc openshell-gateway-db-data -n "$GW_NAMESPACE" -o jsonpath='{.status.phase}' 2>/dev/null || true)
if [[ "$PVC_PHASE" == "Bound" ]]; then
  pass "Database PVC bound"
else
  fail_test "Database PVC not bound (phase=${PVC_PHASE:-unknown})"
fi

show_cmd "$CLI get secret openshell-gateway-db-credentials -n $GW_NAMESPACE"
if $CLI get secret openshell-gateway-db-credentials -n "$GW_NAMESPACE" &>/dev/null; then
  pass "Database credentials secret exists"
else
  fail_test "Database credentials secret not found"
fi

show_cmd "$CLI get secret openshell-client-tls -n $GW_NAMESPACE"
if $CLI get secret openshell-client-tls -n "$GW_NAMESPACE" &>/dev/null; then
  pass "Client TLS secret exists"
else
  fail_test "Client TLS secret not found"
fi

show_cmd "$CLI get configmap openshell-gateway-config -n $GW_NAMESPACE"
if $CLI get configmap openshell-gateway-config -n "$GW_NAMESPACE" &>/dev/null; then
  pass "Gateway config ConfigMap exists"
else
  fail_test "Gateway config ConfigMap not found"
fi

show_cmd "$CLI get certificate openshell-ca -n $GW_NAMESPACE"
GW_CA_READY=$($CLI get certificate openshell-ca -n "$GW_NAMESPACE" -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null || echo "")
if [[ "$GW_CA_READY" == "True" ]]; then
  pass "Gateway CA certificate issued"
else
  fail_test "Gateway CA certificate not ready (status=${GW_CA_READY:-unknown})"
fi

show_cmd "$CLI get certificate openshell-server -n $GW_NAMESPACE"
GW_SRV_READY=$($CLI get certificate openshell-server -n "$GW_NAMESPACE" -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null || echo "")
if [[ "$GW_SRV_READY" == "True" ]]; then
  pass "Gateway server certificate issued"
else
  fail_test "Gateway server certificate not ready (status=${GW_SRV_READY:-unknown})"
fi

show_cmd "$CLI get networkpolicy -n $GW_NAMESPACE"
GW_NP_COUNT=$($CLI get networkpolicy -n "$GW_NAMESPACE" --no-headers 2>/dev/null | wc -l | tr -d ' ')
if [[ "${GW_NP_COUNT:-0}" -ge 3 ]]; then
  pass "Gateway NetworkPolicies present (${GW_NP_COUNT} found)"
else
  fail_test "Expected at least 3 gateway NetworkPolicies, found ${GW_NP_COUNT:-0}"
fi
sep

# ── 4. OIDC token acquisition ─────────────────────────────────────────────

echo ""
bold "4. OIDC Token Acquisition"
echo ""

show_cmd "# resource-owner password grant → ${E2E_OIDC_ISSUER}"
acquire_oidc_token
OIDC_TOKEN="${_OIDC_ACCESS_TOKEN}"
if [[ -n "$OIDC_TOKEN" ]]; then
  pass "OIDC token acquired (user: ${E2E_OIDC_USERNAME})"
else
  fail_test "Failed to acquire OIDC token from Keycloak"
  exit 1
fi
sep

# ── 5. route discovery + CLI registration ─────────────────────────────────

echo ""
bold "5. Route Discovery + CLI Registration"
echo ""

GW_LOCAL_NAME="${GW_NAMESPACE}-openshell"

if wait_for_gateway_route "$GW_NAME" "$GW_NAMESPACE"; then
  pass "Gateway route is ready"
else
  fail_test "Gateway route not ready after timeout"
  exit 1
fi

discover_gateway_endpoint "$GW_NAME" "$GW_NAMESPACE"
GW_ENDPOINT="${_DISCOVER_GW_ENDPOINT}"
if [[ -n "$GW_ENDPOINT" ]]; then
  pass "Gateway endpoint: ${GW_ENDPOINT}"
else
  fail_test "Could not discover gateway endpoint"
  exit 1
fi

GW_CONFIG_DIR="${HOME}/.config/openshell/gateways/${GW_LOCAL_NAME}"
mkdir -p "${GW_CONFIG_DIR}"

show_cmd "${OPENSHELL_BIN} gateway remove ${GW_LOCAL_NAME}"
"${OPENSHELL_BIN}" gateway remove "${GW_LOCAL_NAME}" 2>/dev/null || true
mkdir -p "${GW_CONFIG_DIR}"

show_cmd "# write gateway metadata (OIDC mode)"
GW_LOCAL_NAME="$GW_LOCAL_NAME" GW_ENDPOINT="$GW_ENDPOINT" \
  E2E_OIDC_ISSUER="$E2E_OIDC_ISSUER" E2E_OIDC_CLIENT_ID="$E2E_OIDC_CLIENT_ID" \
  OIDC_TOKEN="$OIDC_TOKEN" GW_CONFIG_DIR="$GW_CONFIG_DIR" \
  python3 -c "
import json, os
config_dir = os.environ['GW_CONFIG_DIR']
meta = {
    'name': os.environ['GW_LOCAL_NAME'],
    'gateway_endpoint': os.environ['GW_ENDPOINT'],
    'is_remote': True,
    'gateway_port': 0,
    'auth_mode': 'oidc',
    'oidc_issuer': os.environ['E2E_OIDC_ISSUER'],
    'oidc_client_id': os.environ['E2E_OIDC_CLIENT_ID']
}
with open(os.path.join(config_dir, 'metadata.json'), 'w') as f:
    json.dump(meta, f, indent=2)
token = {
    'access_token': os.environ['OIDC_TOKEN'],
    'issuer': os.environ['E2E_OIDC_ISSUER'],
    'client_id': os.environ['E2E_OIDC_CLIENT_ID']
}
with open(os.path.join(config_dir, 'oidc_token.json'), 'w') as f:
    json.dump(token, f, indent=2)
os.chmod(os.path.join(config_dir, 'metadata.json'), 0o600)
os.chmod(os.path.join(config_dir, 'oidc_token.json'), 0o600)
"

if [[ -f "${GW_CONFIG_DIR}/metadata.json" && -f "${GW_CONFIG_DIR}/oidc_token.json" ]]; then
  pass "openshell CLI registered (OIDC mode)"
else
  fail_test "Failed to write gateway config"
fi
sep

# ── 6. gateway connectivity ───────────────────────────────────────────────

echo ""
bold "6. Gateway Connectivity"
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
  GW_VERSION=$(echo "$CLEAN_STATUS" | sed -n 's/.*Version:[[:space:]]*\([^[:space:]]*\).*/\1/p' | head -1)
  : "${GW_VERSION:=unknown}"
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

# ── 7. sandbox lifecycle ──────────────────────────────────────────────────

echo ""
bold "7. Sandbox Lifecycle"
echo ""

RUN_ID=$(date +%s | tail -c5)
SANDBOX_NAME="e2e-${RUN_ID}"
show_cmd "OPENSHELL_GATEWAY_INSECURE=true ${OPENSHELL_BIN} -g ${GW_LOCAL_NAME} sandbox create --name ${SANDBOX_NAME}"
dim "  Creating sandbox (timeout: ${E2E_SANDBOX_TIMEOUT}s)..."

SB_CREATE_LOG=$(mktemp)
OPENSHELL_GATEWAY_INSECURE=true "${OPENSHELL_BIN}" -g "${GW_LOCAL_NAME}" sandbox create --name "${SANDBOX_NAME}" >"${SB_CREATE_LOG}" 2>&1 &
SB_CREATE_PID=$!

sleep 5
if ! kill -0 "$SB_CREATE_PID" 2>/dev/null; then
  wait "$SB_CREATE_PID" 2>/dev/null || true
  SB_CREATE_PID=""
  SB_CREATE_ERR=$(sed 's/\x1b\[[0-9;]*m//g' "${SB_CREATE_LOG}" 2>/dev/null || true)
  fail_test "Sandbox create failed immediately"
  echo "$SB_CREATE_ERR" | while IFS= read -r line; do dim "    $line"; done
fi

SANDBOX_FOUND=false
POD_NAME=""
POD_STATUS=""
DEADLINE=$(($(date +%s) + E2E_SANDBOX_TIMEOUT))
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
    if [[ -s "${SB_CREATE_LOG}" ]]; then
      dim "  Sandbox create output:"
      sed 's/\x1b\[[0-9;]*m//g' "${SB_CREATE_LOG}" | while IFS= read -r line; do dim "    $line"; done
    fi
  fi
fi
rm -f "${SB_CREATE_LOG}" 2>/dev/null || true
sep

# ── 8. sandbox interaction ────────────────────────────────────────────────

echo ""
bold "8. Sandbox Interaction"
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

if [[ "$E2E_SKIP_CLEANUP" != "1" && "$SANDBOX_FOUND" == "true" ]]; then
  echo ""
  dim "  Cleaning up sandbox..."
  show_cmd "OPENSHELL_GATEWAY_INSECURE=true ${OPENSHELL_BIN} -g ${GW_LOCAL_NAME} sandbox delete ${SANDBOX_NAME}"
  OPENSHELL_GATEWAY_INSECURE=true "${OPENSHELL_BIN}" -g "${GW_LOCAL_NAME}" sandbox delete "${SANDBOX_NAME}" 2>&1 || true
  dim "  Sandbox deleted"
fi
sep

# ── results ───────────────────────────────────────────────────────────────

print_results

if [[ $E2E_FAIL -gt 0 ]]; then
  exit 1
fi
