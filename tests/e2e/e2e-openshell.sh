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
#   E2E_MODE               Run depth: long (default, every step) or short (essential steps)
#   E2E_SANDBOX_TIMEOUT    Seconds to wait for sandbox (default: 120)
#   E2E_PROVISION_TIMEOUT  Seconds to wait for gateway provisioning (default: 180)
#   E2E_GC_TIMEOUT         Seconds to wait for namespace GC after delete (default: 180)
#   E2E_ORPHAN_GC_TIMEOUT  Seconds to wait for periodic orphan namespace GC (default: 90)
#   E2E_SKIP_CLEANUP       Set to 1 to keep test resources after run (default: 0)
#   DATABASE_PROVIDER      Database provider: deployment or cnpg (default: deployment)
#   E2E_CNPG_NAMESPACE     Namespace where the CNPG operator runs (default: cnpg-system)
#   OPENSHELL_BIN          Path to the openshell CLI binary (default: openshell)
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# --- Source shared utilities ---
# shellcheck source=lib.sh
source "${SCRIPT_DIR}/lib.sh"

# --- Database provider selection ---
# deployment = plain Kubernetes Deployment + PVC + Service (no CNPG operator,
#              default: unset/empty DATABASE_PROVIDER means deployment, see
#              specs/platform/openshell-gateway-database.spec.md)
# cnpg = CloudNativePG operator (CRDs: Cluster, Database, DatabaseRole)
DB_PROVIDER="${DATABASE_PROVIDER:-deployment}"

# --- Driver selection and validation ---

e2e_validate_mode

if [[ -z "${E2E_INFRA_DRIVER:-}" ]]; then
  e2e_die_unknown_driver "E2E_INFRA_DRIVER is not set."
fi

DRIVER_FILE="${SCRIPT_DIR}/drivers/${E2E_INFRA_DRIVER}.sh"
if [[ ! -f "$DRIVER_FILE" ]]; then
  e2e_die_unknown_driver "Unknown driver '${E2E_INFRA_DRIVER}'. Driver file not found: ${DRIVER_FILE}"
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
ORPHAN_NS=""
ORPHAN_GC_DEADLINE=0
SANDBOX_NAME=""
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
  if [[ -n "${SB2_CREATE_PID:-}" ]]; then
    kill "$SB2_CREATE_PID" 2>/dev/null || true
    wait "$SB2_CREATE_PID" 2>/dev/null || true
  fi
  if [[ -n "${SB2_CREATE_LOG:-}" ]]; then
    rm -f "$SB2_CREATE_LOG" 2>/dev/null || true
  fi
  if [[ -n "${E2E_GW_PF_PID:-}" ]]; then
    kill "$E2E_GW_PF_PID" 2>/dev/null || true
    wait "$E2E_GW_PF_PID" 2>/dev/null || true
  fi
  # Short mode never deletes the supplied/reused gateway: checkpoints and
  # canary runs must leave it standing. E2E_SKIP_CLEANUP also preserves it.
  if [[ "$E2E_MODE" != "short" && "$E2E_SKIP_CLEANUP" != "1" && -n "$GW_ID" ]]; then
    dim "  Cleaning up gateway ${GW_NAME}..."
    # JWT is enforced, so the DELETE needs a bearer token. The token acquired
    # earlier may have expired during provisioning, so refresh best-effort before
    # deleting; cleanup is non-fatal, so ignore failures.
    acquire_oidc_token 2>/dev/null || true
    api_curl -X DELETE "${API_HOST}/api/hypershell/v1/gateways/${GW_ID}" &>/dev/null || true
  fi
}
trap cleanup EXIT

# --- Discover API host via driver ---

if ! discover_api_host; then
  red "ERROR: Could not discover HyperShell API host over the gateway HTTPS route"
  exit 1
fi
API_HOST="${_DISCOVER_API_HOST}"

# --- Banner ---

echo ""
bold "HyperShell OpenShell Gateway End-to-End Test"
sep
echo ""
printf '  %s\n' "1. Infrastructure validation + OIDC verification"
printf '  %s\n' "2. Gateway provisioning via HyperShell API (OIDC)"
printf '  %s\n' "3. Gateway infrastructure verification"
printf '  %s\n' "4. OIDC token acquisition + CA certificate setup"
printf '  %s\n' "5. Route discovery + openshell CLI registration"
printf '  %s\n' "6. Gateway connectivity"
printf '  %s\n' "7. Sandbox lifecycle (create → ready)"
printf '  %s\n' "8. Sandbox interaction + active sandbox count"
printf '  %s\n' "9. Developer user RBAC verification"
printf '  %s\n' "10. Platform admin RBAC verification"
printf '  %s\n' "11. Gateway deletion + namespace garbage collection"
echo ""
dim  "  Driver:            ${E2E_INFRA_DRIVER}"
dim  "  Mode:              ${E2E_MODE}"
dim  "  Database provider: ${DB_PROVIDER}"
dim  "  HyperShell API:    ${API_HOST}"
dim  "  Gateway name:      ${GW_NAME}"
dim  "  OIDC issuer:       ${E2E_OIDC_ISSUER}"
dim  "  Admin user:        ${E2E_OIDC_USERNAME}"
dim  "  Developer user:    ${E2E_DEV_USERNAME}"
dim  "  Platform admin:    ${E2E_PLATFORM_ADMIN_USERNAME}"
dim  "  Sandbox timeout:   ${E2E_SANDBOX_TIMEOUT}s"
echo ""
sep

# ── 1. infrastructure validation + OIDC verification ─────────────────────

echo ""
bold "1. Infrastructure Validation + OIDC Verification"
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

if e2e_step long; then
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

if [[ "${DB_PROVIDER}" == "cnpg" ]]; then
  E2E_CNPG_NAMESPACE="${E2E_CNPG_NAMESPACE:-cnpg-system}"
  show_cmd "$CLI get deployment cnpg-controller-manager -n $E2E_CNPG_NAMESPACE"
  CNPG_REPLICAS=$($CLI get deployment cnpg-controller-manager -n "$E2E_CNPG_NAMESPACE" -o jsonpath='{.status.readyReplicas}' 2>/dev/null || echo "0")
  if [[ "${CNPG_REPLICAS:-0}" -ge 1 ]]; then
    pass "CloudNativePG operator is ready"
  else
    fail_test "CloudNativePG operator is not ready (readyReplicas=${CNPG_REPLICAS:-0})"
  fi

  show_cmd "$CLI get crd clusters.postgresql.cnpg.io"
  if $CLI get crd clusters.postgresql.cnpg.io &>/dev/null; then
    pass "CloudNativePG CRDs installed"
  else
    fail_test "CloudNativePG CRDs not found"
  fi
else
  dim "  CNPG checks skipped (DATABASE_PROVIDER=deployment)"
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
fi
sep

# ── 2. gateway provisioning ────────────────────────────────────────────────

echo ""
bold "2. Gateway Provisioning via HyperShell API"
echo ""

# JWT enforcement means every gateway CRUD call below needs a bearer token.
# Refresh the token here so the full Keycloak access-token lifetime covers the
# create plus the provisioning poll (up to E2E_PROVISION_TIMEOUT seconds).
if ! acquire_oidc_token; then
  fail_test "Could not acquire OIDC token for gateway provisioning"
  exit 1
fi

show_cmd "api_curl ${API_HOST}/api/hypershell/v1/gateways?search=name%3D${GW_NAME}"
EXISTING_GW=$(api_curl "${API_HOST}/api/hypershell/v1/gateways?search=name%3D${GW_NAME}" 2>/dev/null || true)
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
  # database_id is a required request property but its value is server-owned.
  # CNPG placement resolves the sole ManagedDatabase; deployment
  # placement ignores the empty placeholder and creates a new dedicated one.
  show_cmd "api_curl ${API_HOST}/api/hypershell/v1/managed_databases"
  E2E_MD_RESP=$(api_curl "${API_HOST}/api/hypershell/v1/managed_databases" 2>/dev/null || true)
  PREEXISTING_DATABASE_IDS=$(echo "$E2E_MD_RESP" | python3 -c "
import json, sys
try:
    data = json.load(sys.stdin)
except Exception:
    data = {}
for item in data.get('items', []):
    if item.get('id'):
        print(item['id'])
" 2>/dev/null || true)

  if [[ "${DB_PROVIDER}" == "cnpg" ]]; then
    E2E_DATABASE_ID=$(echo "$E2E_MD_RESP" | python3 -c "
import json, sys
data = json.load(sys.stdin)
items = data.get('items', [])
print(items[0].get('id', '') if items else '')
" 2>/dev/null || true)
    if [[ -z "$E2E_DATABASE_ID" ]]; then
      fail_test "Could not discover CNPG database_id from ManagedDatabase API"
      exit 1
    fi
  else
    E2E_DATABASE_ID=""
  fi
  dim "  database_id is assigned by ${DB_PROVIDER} placement"

  show_cmd "api_curl -X POST ${API_HOST}/api/hypershell/v1/gateways -d '{name: ${GW_NAME}, database_id: <placement placeholder>, oidc: ...}'"
  GW_CREATE_BODY=$(GW_NAME="$GW_NAME" E2E_OIDC_ISSUER="$E2E_OIDC_ISSUER" \
    E2E_OIDC_CLIENT_ID="$E2E_OIDC_CLIENT_ID" \
    E2E_DATABASE_ID="$E2E_DATABASE_ID" python3 -c "
import json, os
body = {
    'name': os.environ['GW_NAME'],
    'cluster_id': 'e2e-cluster',
    'release_id': 'e2e-release',
    'database_id': os.environ['E2E_DATABASE_ID'],
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
  CREATE_RESPONSE=$(api_curl -X POST "${API_HOST}/api/hypershell/v1/gateways" \
    -H "Content-Type: application/json" \
    -d "${GW_CREATE_BODY}" 2>/dev/null || true)

  # A successful create returns kind="Gateway" with a ksuid id. A failure returns
  # kind="Error" with a numeric id (e.g. "9"=ErrorGeneral/500, "17"=malformed/400).
  # Detect the error case explicitly: otherwise the error object's id is mistaken
  # for a gateway id and the provisioning poll spins on a nonexistent gateway until
  # timeout, masking the real api-server failure.
  IFS=$'\t' read -r CREATE_KIND CREATE_F1 CREATE_F2 CREATE_F3 <<< "$(echo "$CREATE_RESPONSE" | python3 -c "
import json, sys
try:
    d = json.load(sys.stdin)
except Exception:
    print('PARSE\t\t\t'); sys.exit(0)
if d.get('kind') == 'Error':
    print('ERROR\t%s\t%s\t' % (d.get('code', ''), d.get('reason', ''))); sys.exit(0)
print('OK\t%s\t%s\t%s' % (d.get('id', ''), d.get('namespace', ''), d.get('database_id', '')))
" 2>/dev/null)" || true

  if [[ "$CREATE_KIND" == "OK" && -n "$CREATE_F1" ]]; then
    GW_ID="$CREATE_F1"
    GW_NAMESPACE="$CREATE_F2"
    if [[ -z "$CREATE_F3" ]]; then
      fail_test "Gateway creation succeeded without a server-assigned database_id"
      exit 1
    fi
    if [[ "${DB_PROVIDER}" == "deployment" ]] && grep -Fxq "$CREATE_F3" <<< "$PREEXISTING_DATABASE_IDS"; then
      fail_test "Deployment placement reused existing ManagedDatabase ${CREATE_F3}; expected a new per-gateway database"
      exit 1
    fi
    if [[ "${DB_PROVIDER}" == "deployment" ]]; then
      CREATED_DB_PROVIDER=$(api_curl "${API_HOST}/api/hypershell/v1/managed_databases/${CREATE_F3}" 2>/dev/null | \
        python3 -c "import json,sys; print(json.load(sys.stdin).get('provider',''))" 2>/dev/null || true)
      if [[ "$CREATED_DB_PROVIDER" != "deployment" ]]; then
        fail_test "Server-assigned ManagedDatabase ${CREATE_F3} has provider=${CREATED_DB_PROVIDER:-unknown}, expected deployment"
        exit 1
      fi
      pass "Deployment placement created dedicated ManagedDatabase ${CREATE_F3}"
    fi
    pass "Gateway created: ${GW_NAME} (${GW_ID})"
  else
    fail_test "Failed to create gateway"
    if [[ "$CREATE_KIND" == "ERROR" ]]; then
      dim "    api-server error ${CREATE_F1}: ${CREATE_F2}"
    fi
    dim "    ${CREATE_RESPONSE:0:300}"
    exit 1
  fi

  dim "  Waiting for controller to provision (timeout: ${E2E_PROVISION_TIMEOUT}s)..."
  DEADLINE=$(($(date +%s) + E2E_PROVISION_TIMEOUT))
  GW_PHASE=""
  while [[ $(date +%s) -lt $DEADLINE ]]; do
    # Refresh the OIDC token each poll: provisioning can outlast the access
    # token lifetime, and api_curl reads _OIDC_ACCESS_TOKEN on every call.
    acquire_oidc_token 2>/dev/null || true
    GW_PHASE=$(api_curl "${API_HOST}/api/hypershell/v1/gateways/${GW_ID}" 2>/dev/null | \
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
    fail_test "Gateway not running after ${E2E_PROVISION_TIMEOUT}s (phase=${GW_PHASE:-unknown})"
    dim "    last gateway state:"
    api_curl "${API_HOST}/api/hypershell/v1/gateways/${GW_ID}" 2>/dev/null \
      | python3 -m json.tool 2>/dev/null | sed 's/^/      /' || true
    exit 1
  fi
fi

if [[ -z "$GW_NAMESPACE" ]]; then
  fail_test "Gateway response did not include a server-assigned namespace"
  exit 1
fi
dim "  Gateway namespace: ${GW_NAMESPACE}"

# Seed a synthetic orphaned managed namespace for periodic GC. Created here so
# steps 3–10 run while the reaper sweeps; step 11 only validates (no extra wait
# if the reaper already ran during the suite). Long-only: short mode does not
# exercise the periodic reaper.
if e2e_step long && [[ "$E2E_SKIP_CLEANUP" != "1" ]]; then
  ORPHAN_NS="openshell-e2e-orphan-$(date +%s)"
  ORPHAN_ELIGIBLE_SINCE=$(e2e_gc_eligible_since_backdate 3)
  dim "  Seeding periodic GC orphan namespace: ${ORPHAN_NS}"
  show_cmd "$CLI apply -f -  # namespace ${ORPHAN_NS} with management labels and backdated gc-eligible-since"
  $CLI apply -f - <<EOF
apiVersion: v1
kind: Namespace
metadata:
  name: ${ORPHAN_NS}
  labels:
    hypershell.redhat.io/managed: "true"
    app.kubernetes.io/managed-by: hypershell-control-plane
  annotations:
    hypershell.redhat.io/gc-eligible-since: "${ORPHAN_ELIGIBLE_SINCE}"
EOF
  ORPHAN_GC_DEADLINE=$(($(date +%s) + E2E_ORPHAN_GC_TIMEOUT))
fi

# Per-gateway Keycloak client id. When Keycloak provisioning is enabled (the Kind
# path), the control-plane reconciler creates a dedicated public client named
# "${gw.Name}-${gatewayID}" with an audience mapper and overrides the gateway's
# OIDC config to require aud == this client. Gateway and CLI tokens must therefore
# be minted against this client, not the shared frontend client, or Envoy rejects
# them with InvalidAudience. gatewayID is the API resource id (GW_ID).
GW_KC_CLIENT_ID="${GW_NAME}-${GW_ID}"
if [[ "${E2E_INFRA_DRIVER}" == "kind" ]]; then
  dim "  Per-gateway OIDC client: ${GW_KC_CLIENT_ID}"
fi
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

if e2e_step long; then
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

# Resolve the ManagedDatabase namespace (provider-agnostic).
DB_GW_NAMESPACE=""
acquire_oidc_token 2>/dev/null || true
GW_DB_ID=$(api_curl "${API_HOST}/api/hypershell/v1/gateways/${GW_ID}" 2>/dev/null | \
  python3 -c "import json,sys; print(json.load(sys.stdin).get('database_id',''))" 2>/dev/null || true)
if [[ -n "$GW_DB_ID" ]]; then
  DB_GW_NAMESPACE=$(api_curl "${API_HOST}/api/hypershell/v1/managed_databases/${GW_DB_ID}" 2>/dev/null | \
    python3 -c "import json,sys; print(json.load(sys.stdin).get('namespace',''))" 2>/dev/null || true)
fi
if [[ -n "$DB_GW_NAMESPACE" ]]; then
  dim "  Database namespace: ${DB_GW_NAMESPACE}"
else
  fail_test "Could not resolve database namespace for gateway ${GW_ID}"
fi

if [[ "${DB_PROVIDER}" == "cnpg" ]]; then
  # CNPG provider: verify Database CR, DatabaseRole CR, and client TLS
  CNPG_GW_NAMESPACE="${DB_GW_NAMESPACE}"
  CNPG_CR_NAME="gw-$(echo "${GW_ID}" | tr '[:upper:]' '[:lower:]')"

  show_cmd "$CLI get database.postgresql.cnpg.io ${CNPG_CR_NAME} -n ${CNPG_GW_NAMESPACE}"
  DB_APPLIED=$($CLI get database.postgresql.cnpg.io "${CNPG_CR_NAME}" -n "${CNPG_GW_NAMESPACE}" \
    -o jsonpath='{.status.applied}' 2>/dev/null || true)
  if [[ "$DB_APPLIED" == "true" ]]; then
    pass "CNPG Database CR ready: ${CNPG_CR_NAME}"
  else
    fail_test "CNPG Database CR not ready (status.applied=${DB_APPLIED:-unknown})"
  fi

  show_cmd "$CLI get databaserole.postgresql.cnpg.io ${CNPG_CR_NAME} -n ${CNPG_GW_NAMESPACE}"
  if $CLI get databaserole.postgresql.cnpg.io "${CNPG_CR_NAME}" -n "${CNPG_GW_NAMESPACE}" &>/dev/null; then
    pass "CNPG DatabaseRole CR exists: ${CNPG_CR_NAME}"
  else
    fail_test "CNPG DatabaseRole CR not found: ${CNPG_CR_NAME}"
  fi

  show_cmd "$CLI get secret openshell-client-tls -n $GW_NAMESPACE"
  if $CLI get secret openshell-client-tls -n "$GW_NAMESPACE" &>/dev/null; then
    pass "Client TLS secret exists"
  else
    fail_test "Client TLS secret not found"
  fi
else
  # Deployment provider: verify DB Deployment readiness and credentials secret
  show_cmd "$CLI get deployment openshell-gateway-db -n ${DB_GW_NAMESPACE}"
  DB_DEPLOY_READY=$($CLI get deployment openshell-gateway-db -n "${DB_GW_NAMESPACE}" \
    -o jsonpath='{.status.readyReplicas}' 2>/dev/null || echo "0")
  if [[ "${DB_DEPLOY_READY:-0}" -ge 1 ]]; then
    pass "Database deployment ready in ${DB_GW_NAMESPACE}"
  else
    fail_test "Database deployment not ready (readyReplicas=${DB_DEPLOY_READY:-0})"
  fi

  show_cmd "$CLI get secret openshell-db-credentials -n ${DB_GW_NAMESPACE}"
  if $CLI get secret openshell-db-credentials -n "${DB_GW_NAMESPACE}" &>/dev/null; then
    pass "Database credentials secret exists in ${DB_GW_NAMESPACE}"
  else
    fail_test "Database credentials secret not found in ${DB_GW_NAMESPACE}"
  fi
fi

# This check is common to both providers
show_cmd "$CLI get secret openshell-gateway-db-credentials -n $GW_NAMESPACE"
if $CLI get secret openshell-gateway-db-credentials -n "$GW_NAMESPACE" &>/dev/null; then
  pass "Database credentials secret exists in gateway namespace"
else
  fail_test "Database credentials secret not found in gateway namespace"
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

# Kind deliberately skips the per-tenant gateway NetworkPolicies: its Gateway
# data plane is cloud-provider-kind's out-of-cluster Envoy, whose source IP no
# selector can match, so the policies would blackhole gateway ingress. Dev needs
# no tenant isolation (see GATEWAY_SKIP_NETWORK_POLICIES in deploy/kind), so the
# ≥3 assertion does not apply here.
if [[ "${E2E_INFRA_DRIVER}" == "kind" ]]; then
  dim "  Gateway NetworkPolicies intentionally skipped on kind (not applicable)"
else
  show_cmd "$CLI get networkpolicy -n $GW_NAMESPACE"
  GW_NP_COUNT=$($CLI get networkpolicy -n "$GW_NAMESPACE" --no-headers 2>/dev/null | wc -l | tr -d ' ')
  if [[ "${GW_NP_COUNT:-0}" -ge 3 ]]; then
    pass "Gateway NetworkPolicies present (${GW_NP_COUNT} found)"
  else
    fail_test "Expected at least 3 gateway NetworkPolicies, found ${GW_NP_COUNT:-0}"
  fi
fi
fi
sep

# ── 4. OIDC token acquisition + CA certificate setup ─────────────────────

echo ""
bold "4. OIDC Token Acquisition + CA Certificate Setup"
echo ""

# The client the admin's gateway/CLI tokens are minted against. On Kind the
# reconciler forces a per-gateway audience, so we use the per-gateway client and
# wait for the async owner-binding -> openshell-admin role to land in the token.
OIDC_CLIENT_ID_EFFECTIVE="${E2E_OIDC_CLIENT_ID}"
if [[ "${E2E_INFRA_DRIVER}" == "kind" ]]; then
  OIDC_CLIENT_ID_EFFECTIVE="${GW_KC_CLIENT_ID}"

  if e2e_step long; then
  # Exercise the real Keycloak device authorization endpoint for the client
  # provisioned by the control plane. A successful authorization response proves
  # that oauth2.device.authorization.grant.enabled reached Keycloak; polling once
  # after the advertised interval proves that Keycloak recognizes the device code.
  DEVICE_DISCOVERY=$(curl -sk "${E2E_OIDC_ISSUER}/.well-known/openid-configuration" 2>/dev/null || true)
  DEVICE_AUTH_ENDPOINT=$(echo "$DEVICE_DISCOVERY" | python3 -c "import json,sys; print(json.load(sys.stdin).get('device_authorization_endpoint',''))" 2>/dev/null || true)
  if [[ -z "$DEVICE_AUTH_ENDPOINT" ]]; then
    fail_test "OIDC discovery did not advertise a device authorization endpoint"
    exit 1
  fi

  # This public client requires PKCE S256 for every authorization flow. Keep the
  # verifier private and send only its SHA-256 challenge to the device endpoint.
  DEVICE_CODE_VERIFIER=$(python3 -c "import secrets; print(secrets.token_urlsafe(48))")
  DEVICE_CODE_CHALLENGE=$(DEVICE_CODE_VERIFIER="$DEVICE_CODE_VERIFIER" python3 -c "import base64,hashlib,os; print(base64.urlsafe_b64encode(hashlib.sha256(os.environ['DEVICE_CODE_VERIFIER'].encode()).digest()).rstrip(b'=').decode())")

  show_cmd "# OAuth 2.0 Device Authorization Grant with PKCE S256 → ${DEVICE_AUTH_ENDPOINT} (client: ${GW_KC_CLIENT_ID})"
  DEVICE_AUTH_RESPONSE=$(curl -sk -X POST "$DEVICE_AUTH_ENDPOINT" \
    --data-urlencode "client_id=${GW_KC_CLIENT_ID}" \
    --data-urlencode "scope=openid" \
    --data-urlencode "code_challenge=${DEVICE_CODE_CHALLENGE}" \
    --data-urlencode "code_challenge_method=S256" 2>/dev/null || true)
  DEVICE_CODE=$(echo "$DEVICE_AUTH_RESPONSE" | python3 -c "import json,sys; print(json.load(sys.stdin).get('device_code',''))" 2>/dev/null || true)
  DEVICE_USER_CODE=$(echo "$DEVICE_AUTH_RESPONSE" | python3 -c "import json,sys; print(json.load(sys.stdin).get('user_code',''))" 2>/dev/null || true)
  DEVICE_VERIFICATION_URI=$(echo "$DEVICE_AUTH_RESPONSE" | python3 -c "import json,sys; print(json.load(sys.stdin).get('verification_uri',''))" 2>/dev/null || true)
  DEVICE_INTERVAL=$(echo "$DEVICE_AUTH_RESPONSE" | python3 -c "import json,sys; print(json.load(sys.stdin).get('interval',5))" 2>/dev/null || true)
  if [[ -z "$DEVICE_CODE" || -z "$DEVICE_USER_CODE" || -z "$DEVICE_VERIFICATION_URI" ]]; then
    DEVICE_AUTH_ERROR=$(echo "$DEVICE_AUTH_RESPONSE" | python3 -c "import json,sys; print(json.load(sys.stdin).get('error_description','invalid device authorization response'))" 2>/dev/null || echo "invalid device authorization response")
    fail_test "Per-gateway client rejected Device Authorization Grant: ${DEVICE_AUTH_ERROR}"
    exit 1
  fi
  pass "Per-gateway client started OAuth 2.0 Device Authorization Grant"

  if [[ ! "$DEVICE_INTERVAL" =~ ^[0-9]+$ || "$DEVICE_INTERVAL" -gt 30 ]]; then
    fail_test "Device Authorization Grant returned invalid polling interval"
    exit 1
  fi
  sleep "$DEVICE_INTERVAL"

  DEVICE_TOKEN_RESPONSE=$(curl -sk -X POST "${E2E_OIDC_ISSUER}/protocol/openid-connect/token" \
    --data-urlencode "grant_type=urn:ietf:params:oauth:grant-type:device_code" \
    --data-urlencode "client_id=${GW_KC_CLIENT_ID}" \
    --data-urlencode "device_code=${DEVICE_CODE}" \
    --data-urlencode "code_verifier=${DEVICE_CODE_VERIFIER}" 2>/dev/null || true)
  DEVICE_TOKEN_ERROR=$(echo "$DEVICE_TOKEN_RESPONSE" | python3 -c "import json,sys; print(json.load(sys.stdin).get('error',''))" 2>/dev/null || true)
  if [[ "$DEVICE_TOKEN_ERROR" == "authorization_pending" ]]; then
    pass "Keycloak accepted the issued device code"
  else
    DEVICE_TOKEN_DESCRIPTION=$(echo "$DEVICE_TOKEN_RESPONSE" | python3 -c "import json,sys; print(json.load(sys.stdin).get('error_description','unexpected device token response'))" 2>/dev/null || echo "unexpected device token response")
    fail_test "Device code poll did not return authorization_pending: ${DEVICE_TOKEN_DESCRIPTION}"
    exit 1
  fi
  fi

  show_cmd "# resource-owner password grant → ${E2E_OIDC_ISSUER} (client: ${GW_KC_CLIENT_ID}, await role: openshell-admin)"
  if acquire_gateway_token_with_role "$E2E_OIDC_USERNAME" "$E2E_OIDC_PASSWORD" "$GW_KC_CLIENT_ID" openshell-admin; then
    OIDC_TOKEN="${_OIDC_ACCESS_TOKEN}"
    pass "OIDC token acquired with openshell-admin (user: ${E2E_OIDC_USERNAME}, client: ${GW_KC_CLIENT_ID})"
  else
    fail_test "Failed to acquire per-gateway OIDC token with openshell-admin role"
    exit 1
  fi
else
  show_cmd "# resource-owner password grant → ${E2E_OIDC_ISSUER}"
  acquire_oidc_token
  OIDC_TOKEN="${_OIDC_ACCESS_TOKEN}"
  if [[ -n "$OIDC_TOKEN" ]]; then
    pass "OIDC token acquired (user: ${E2E_OIDC_USERNAME})"
  else
    fail_test "Failed to acquire OIDC token from Keycloak"
    exit 1
  fi
fi


show_cmd "$CLI get secret hypershell-ca-secret -n $E2E_HS_NAMESPACE -o jsonpath='{.data.ca\.crt}' | base64 -d > /tmp/e2e-hypershell-ca.crt"
$CLI get secret hypershell-ca-secret -n "$E2E_HS_NAMESPACE" -o jsonpath='{.data.ca\.crt}' 2>/dev/null | base64 -d > /tmp/e2e-hypershell-ca.crt
if [[ -s /tmp/e2e-hypershell-ca.crt ]]; then
  export SSL_CERT_FILE=/tmp/e2e-hypershell-ca.crt
  pass "CA certificate extracted and SSL_CERT_FILE set"
  dim "    CA: /tmp/e2e-hypershell-ca.crt"
else
  fail_test "Failed to extract CA certificate"
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

show_cmd "# write gateway metadata (OIDC mode, client: ${OIDC_CLIENT_ID_EFFECTIVE})"
GW_LOCAL_NAME="$GW_LOCAL_NAME" GW_ENDPOINT="$GW_ENDPOINT" \
  E2E_OIDC_ISSUER="$E2E_OIDC_ISSUER" OIDC_CLIENT_ID_EFFECTIVE="$OIDC_CLIENT_ID_EFFECTIVE" \
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
    'oidc_client_id': os.environ['OIDC_CLIENT_ID_EFFECTIVE']
}
with open(os.path.join(config_dir, 'metadata.json'), 'w') as f:
    json.dump(meta, f, indent=2)
token = {
    'access_token': os.environ['OIDC_TOKEN'],
    'issuer': os.environ['E2E_OIDC_ISSUER'],
    'client_id': os.environ['OIDC_CLIENT_ID_EFFECTIVE']
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

show_cmd "${OPENSHELL_BIN} -g ${GW_LOCAL_NAME} status"
dim "  Waiting for route connectivity (up to 60s)..."
CONNECT_DEADLINE=$(($(date +%s) + 60))
STATUS_OUTPUT=""
CONNECTED=false
while [[ $(date +%s) -lt $CONNECT_DEADLINE ]]; do
  STATUS_OUTPUT=$("${OPENSHELL_BIN}" -g "${GW_LOCAL_NAME}" status 2>&1 || true)
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
  exit 1
fi
sep

# ── 7. sandbox lifecycle ──────────────────────────────────────────────────

echo ""
bold "7. Sandbox Lifecycle"
echo ""

RUN_ID=$(date +%s | tail -c5)
SANDBOX_NAME="e2e-${RUN_ID}"
show_cmd "${OPENSHELL_BIN} -g ${GW_LOCAL_NAME} sandbox create --name ${SANDBOX_NAME}"
dim "  Creating sandbox (timeout: ${E2E_SANDBOX_TIMEOUT}s)..."

SB_CREATE_LOG=$(mktemp)
"${OPENSHELL_BIN}" -g "${GW_LOCAL_NAME}" sandbox create --name "${SANDBOX_NAME}" >"${SB_CREATE_LOG}" 2>&1 &
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

# ── 8. sandbox interaction + active sandbox count ─────────────────────────

echo ""
bold "8. Sandbox Interaction + Active Sandbox Count"
echo ""

GW_FLAG="-g ${GW_LOCAL_NAME}"

# The sandbox pod can report Running while the Sandbox CR is still
# phase=Provisioning, and the openshell CLI gates `sandbox exec` on the CR
# reaching Ready. Poll a no-op exec until it succeeds so the interaction
# commands below don't race the sandbox controller.
SANDBOX_READY=false
SB_READY_ERR=""
READY_DEADLINE=$(($(date +%s) + E2E_SANDBOX_TIMEOUT))
dim "  Waiting for sandbox to become ready (up to ${E2E_SANDBOX_TIMEOUT}s)..."
while [[ $(date +%s) -lt $READY_DEADLINE ]]; do
  if SB_READY_ERR=$("${OPENSHELL_BIN}" -g "${GW_LOCAL_NAME}" sandbox exec -n "${SANDBOX_NAME}" -- true 2>&1); then
    SANDBOX_READY=true
    break
  fi
  sleep 5
done

if [[ "$SANDBOX_READY" != "true" ]]; then
  fail_test "Sandbox did not become ready within ${E2E_SANDBOX_TIMEOUT}s"
  dim "    ${SB_READY_ERR:0:200}"
else
  pass "Sandbox ready"

  show_cmd "${OPENSHELL_BIN} ${GW_FLAG} sandbox exec -n ${SANDBOX_NAME} -- uname -a"
  if SB_EXEC_OUTPUT=$("${OPENSHELL_BIN}" -g "${GW_LOCAL_NAME}" sandbox exec -n "${SANDBOX_NAME}" -- uname -a 2>&1); then
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

  if e2e_step long; then
  show_cmd "${OPENSHELL_BIN} ${GW_FLAG} sandbox exec -n ${SANDBOX_NAME} -- ls -la /workspace"
  if SB_LS_OUTPUT=$("${OPENSHELL_BIN}" -g "${GW_LOCAL_NAME}" sandbox exec -n "${SANDBOX_NAME}" -- ls -la /workspace 2>&1); then
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
  fi
fi

# poll_active_sandbox_count <expected>: poll the HyperShell API until the
# gateway's active_sandbox_count equals <expected>, up to E2E_SANDBOX_TIMEOUT.
# The field is control-plane-owned and advisory (it may lag real time) and is
# omitted from the JSON while NULL, so an absent value is treated as "not yet".
# Echoes the last observed value; returns 0 on match, 1 on timeout.
poll_active_sandbox_count() {
  local expected="$1" last="" deadline
  deadline=$(($(date +%s) + E2E_SANDBOX_TIMEOUT))
  while [[ $(date +%s) -lt $deadline ]]; do
    last=$(api_curl "${API_HOST}/api/hypershell/v1/gateways/${GW_ID}" 2>/dev/null | \
      python3 -c "import json,sys; v=json.load(sys.stdin).get('active_sandbox_count'); print('' if v is None else v)" 2>/dev/null || true)
    [[ "$last" == "$expected" ]] && { echo "$last"; return 0; }
    dim "    active_sandbox_count: ${last:-<unset>} (want ${expected})" >&2
    sleep 5
  done
  echo "$last"
  return 1
}

# Active sandbox count accounting (e2e-testing.spec.md "Active Sandbox Count
# Accounting"; openshell-gateway-sandbox-count.spec.md). The control plane
# observes sandbox pods via an informer and publishes the running count on the
# Gateway. Reuse the sandbox created above (count 1), add a second (count 2),
# then delete it (back to 1), polling the API for each transition because the
# value is advisory and may lag.
if [[ "$SANDBOX_FOUND" == "true" ]]; then
  echo ""
  dim "  Verifying active_sandbox_count accounting..."

  show_cmd "api_curl ${API_HOST}/api/hypershell/v1/gateways/${GW_ID}  # active_sandbox_count == 1"
  if COUNT=$(poll_active_sandbox_count 1); then
    pass "active_sandbox_count reflects the running sandbox (${COUNT})"
  else
    fail_test "active_sandbox_count did not reach 1 within ${E2E_SANDBOX_TIMEOUT}s (last: ${COUNT:-<unset>})"
  fi

  if e2e_step long; then
  SANDBOX_NAME_2="${SANDBOX_NAME}-2"
  show_cmd "${OPENSHELL_BIN} -g ${GW_LOCAL_NAME} sandbox create --name ${SANDBOX_NAME_2}"
  dim "  Creating a second sandbox to assert the count increments..."
  SB2_CREATE_LOG=$(mktemp)
  "${OPENSHELL_BIN}" -g "${GW_LOCAL_NAME}" sandbox create --name "${SANDBOX_NAME_2}" >"${SB2_CREATE_LOG}" 2>&1 &
  SB2_CREATE_PID=$!

  SANDBOX2_RUNNING=false
  DEADLINE=$(($(date +%s) + E2E_SANDBOX_TIMEOUT))
  while [[ $(date +%s) -lt $DEADLINE ]]; do
    SB2_PODS=$($CLI get pods -n "$GW_NAMESPACE" --no-headers 2>/dev/null | grep -i "default--${SANDBOX_NAME_2}" || true)
    if [[ -n "$SB2_PODS" ]]; then
      SB2_STATUS=$(echo "$SB2_PODS" | awk '{print $3}' | head -1)
      if [[ "$SB2_STATUS" == "Running" ]]; then
        SANDBOX2_RUNNING=true
        break
      fi
      dim "    pod: default--${SANDBOX_NAME_2} (${SB2_STATUS})"
    fi
    sleep 5
  done
  kill "$SB2_CREATE_PID" 2>/dev/null || true
  wait "$SB2_CREATE_PID" 2>/dev/null || true
  SB2_CREATE_PID=""
  rm -f "${SB2_CREATE_LOG}" 2>/dev/null || true
  SB2_CREATE_LOG=""

  if [[ "$SANDBOX2_RUNNING" == "true" ]]; then
    show_cmd "api_curl ${API_HOST}/api/hypershell/v1/gateways/${GW_ID}  # active_sandbox_count == 2"
    if COUNT=$(poll_active_sandbox_count 2); then
      pass "active_sandbox_count incremented on sandbox create (${COUNT})"
    else
      fail_test "active_sandbox_count did not reach 2 within ${E2E_SANDBOX_TIMEOUT}s (last: ${COUNT:-<unset>})"
    fi
  else
    fail_test "Second sandbox pod not Running within ${E2E_SANDBOX_TIMEOUT}s; cannot assert count increment"
  fi

  # Deleting the second sandbox must drive the count back down. This runs
  # regardless of E2E_SKIP_CLEANUP because the decrement is the assertion.
  show_cmd "${OPENSHELL_BIN} -g ${GW_LOCAL_NAME} sandbox delete ${SANDBOX_NAME_2}"
  "${OPENSHELL_BIN}" -g "${GW_LOCAL_NAME}" sandbox delete "${SANDBOX_NAME_2}" 2>&1 || true
  if COUNT=$(poll_active_sandbox_count 1); then
    pass "active_sandbox_count decremented on sandbox delete (${COUNT})"
  else
    fail_test "active_sandbox_count did not return to 1 within ${E2E_SANDBOX_TIMEOUT}s (last: ${COUNT:-<unset>})"
  fi
  else
    # Short mode: delete the one sandbox and assert the count returns to 0.
    # Runs even with E2E_SKIP_CLEANUP so a reused canary does not accumulate sandboxes.
    show_cmd "${OPENSHELL_BIN} -g ${GW_LOCAL_NAME} sandbox delete ${SANDBOX_NAME}"
    "${OPENSHELL_BIN}" -g "${GW_LOCAL_NAME}" sandbox delete "${SANDBOX_NAME}" 2>&1 || true
    if COUNT=$(poll_active_sandbox_count 0); then
      pass "active_sandbox_count decremented to 0 on sandbox delete (${COUNT})"
    else
      fail_test "active_sandbox_count did not return to 0 within ${E2E_SANDBOX_TIMEOUT}s (last: ${COUNT:-<unset>})"
    fi
    SANDBOX_FOUND=false
  fi
fi
sep

# ── cleanup ───────────────────────────────────────────────────────────────

if [[ "$E2E_SKIP_CLEANUP" != "1" && "$SANDBOX_FOUND" == "true" ]]; then
  echo ""
  dim "  Cleaning up sandbox..."
  show_cmd "${OPENSHELL_BIN} -g ${GW_LOCAL_NAME} sandbox delete ${SANDBOX_NAME}"
  "${OPENSHELL_BIN}" -g "${GW_LOCAL_NAME}" sandbox delete "${SANDBOX_NAME}" 2>&1 || true
  dim "  Sandbox deleted"
fi
sep

# ── 9. developer user RBAC verification ──────────────────────────────────

echo ""
bold "9. Developer User RBAC Verification"
echo ""

# The developer's gateway/CLI token, like the admin's, must be minted against the
# per-gateway client on Kind. The gateway requires user_role (openshell-user) on
# that client or it rejects the developer outright ("role 'openshell-user'
# required"). In production the RoleBinding reconciler assigns this when a
# gateway:viewer binding is created, but that grant is not expressible through the
# API for a non-owner (no user_id discovery path), so we provision the same end
# state directly in Keycloak -- a test-setup shortcut, not a product change.
DEV_OIDC_CLIENT_ID_EFFECTIVE="${E2E_OIDC_CLIENT_ID}"
if [[ "${E2E_INFRA_DRIVER}" == "kind" ]]; then
  DEV_OIDC_CLIENT_ID_EFFECTIVE="${GW_KC_CLIENT_ID}"
  show_cmd "# grant developer openshell-user on ${GW_KC_CLIENT_ID} (mirrors gateway:viewer RoleBinding)"
  if assign_gateway_client_role "$E2E_DEV_USERNAME" "$GW_KC_CLIENT_ID" openshell-user; then
    pass "Developer granted openshell-user on per-gateway client"
  else
    fail_test "Failed to grant developer openshell-user on per-gateway client"
  fi

  show_cmd "# acquire per-gateway OIDC token for developer (client: ${GW_KC_CLIENT_ID}, await role: openshell-user)"
  if acquire_gateway_token_with_role "$E2E_DEV_USERNAME" "$E2E_DEV_PASSWORD" "$GW_KC_CLIENT_ID" openshell-user; then
    DEV_TOKEN="${_OIDC_ACCESS_TOKEN}"
    pass "Developer OIDC token acquired with openshell-user (user: ${E2E_DEV_USERNAME})"
  else
    DEV_TOKEN=""
    fail_test "Failed to acquire developer per-gateway OIDC token with openshell-user role"
  fi
else
  show_cmd "# acquire OIDC token for developer user"
  acquire_oidc_token "$E2E_DEV_USERNAME" "$E2E_DEV_PASSWORD"
  DEV_TOKEN="${_OIDC_ACCESS_TOKEN}"
  if [[ -n "$DEV_TOKEN" ]]; then
    pass "Developer OIDC token acquired (user: ${E2E_DEV_USERNAME})"
  else
    fail_test "Failed to acquire developer OIDC token"
  fi
fi

if [[ -n "$DEV_TOKEN" ]]; then
  DEV_GW_LOCAL_NAME="${GW_LOCAL_NAME}-dev"
  DEV_CONFIG_DIR="${HOME}/.config/openshell/gateways/${DEV_GW_LOCAL_NAME}"
  mkdir -p "${DEV_CONFIG_DIR}"

  "${OPENSHELL_BIN}" gateway remove "${DEV_GW_LOCAL_NAME}" 2>/dev/null || true
  mkdir -p "${DEV_CONFIG_DIR}"

  show_cmd "# register gateway as developer user (client: ${DEV_OIDC_CLIENT_ID_EFFECTIVE})"
  DEV_GW_LOCAL_NAME="$DEV_GW_LOCAL_NAME" GW_ENDPOINT="$GW_ENDPOINT" \
    E2E_OIDC_ISSUER="$E2E_OIDC_ISSUER" DEV_OIDC_CLIENT_ID_EFFECTIVE="$DEV_OIDC_CLIENT_ID_EFFECTIVE" \
    DEV_TOKEN="$DEV_TOKEN" DEV_CONFIG_DIR="$DEV_CONFIG_DIR" \
    python3 -c "
import json, os
config_dir = os.environ['DEV_CONFIG_DIR']
meta = {
    'name': os.environ['DEV_GW_LOCAL_NAME'],
    'gateway_endpoint': os.environ['GW_ENDPOINT'],
    'is_remote': True,
    'gateway_port': 0,
    'auth_mode': 'oidc',
    'oidc_issuer': os.environ['E2E_OIDC_ISSUER'],
    'oidc_client_id': os.environ['DEV_OIDC_CLIENT_ID_EFFECTIVE']
}
with open(os.path.join(config_dir, 'metadata.json'), 'w') as f:
    json.dump(meta, f, indent=2)
token = {
    'access_token': os.environ['DEV_TOKEN'],
    'issuer': os.environ['E2E_OIDC_ISSUER'],
    'client_id': os.environ['DEV_OIDC_CLIENT_ID_EFFECTIVE']
}
with open(os.path.join(config_dir, 'oidc_token.json'), 'w') as f:
    json.dump(token, f, indent=2)
os.chmod(os.path.join(config_dir, 'metadata.json'), 0o600)
os.chmod(os.path.join(config_dir, 'oidc_token.json'), 0o600)
"

  if [[ -f "${DEV_CONFIG_DIR}/metadata.json" && -f "${DEV_CONFIG_DIR}/oidc_token.json" ]]; then
    pass "Developer gateway registered (OIDC mode)"
  else
    fail_test "Failed to write developer gateway config"
  fi

  if e2e_step long; then
  show_cmd "${OPENSHELL_BIN} -g ${DEV_GW_LOCAL_NAME} status"
  DEV_STATUS=$("${OPENSHELL_BIN}" -g "${DEV_GW_LOCAL_NAME}" status 2>&1 || true)
  DEV_CLEAN=$(echo "$DEV_STATUS" | sed 's/\x1b\[[0-9;]*m//g')
  if echo "$DEV_CLEAN" | grep -qi "Connected"; then
    pass "Developer user: gateway connected"
  else
    fail_test "Developer user: gateway not reachable"
    echo "$DEV_STATUS" | while IFS= read -r line; do dim "    $line"; done
  fi

  # RBAC boundary for the standard-user tier. The developer's OIDC token carries
  # the gateway's user_role (openshell-user on the per-gateway client in Kind;
  # the "hypershell-users" group under the shared-client model), which the gateway
  # maps to a standard OpenShell user, not an admin. Two independent authorization
  # systems apply, and we assert both:
  #   1. OpenShell gateway authz: a user_role principal MAY create sandboxes, but
  #      only in a workspace where it holds an explicit membership record. The
  #      OIDC role alone does NOT confer workspace access and membership is not
  #      claim-derived, so a Platform Admin must first add the developer as a
  #      'user' member of the target workspace (upstream model; see OpenShell
  #      manage-workspaces docs and e2e/rust/tests/oidc_pkce.rs prepare_workspace).
  #      The admin has implicit access to 'default', so it can create sandboxes
  #      there without a membership record; the developer cannot until granted one.
  #   2. HyperShell API RBAC: the developer lacks the platform-scoped
  #      gateway:creator role, so POST /gateways MUST be rejected with 403
  #      (rbac-enforcement.spec.md "User without creator role cannot create
  #      gateways"). Asserted after the sandbox check.

  # ── admin grants the developer 'user' membership on the 'default' workspace ──
  # Resolve the subject the gateway checks membership against. `whoami` reports the
  # gateway-validated identity; fall back to decoding the JWT `sub` claim if the
  # CLI predates `whoami`.
  DEV_SUBJECT=$("${OPENSHELL_BIN}" -g "${DEV_GW_LOCAL_NAME}" whoami --output json 2>/dev/null \
    | python3 -c "import json,sys
try:
    print(json.load(sys.stdin).get('subject','') or '')
except Exception:
    pass" 2>/dev/null || true)
  if [[ -z "$DEV_SUBJECT" ]]; then
    DEV_SUBJECT=$(DEV_TOKEN="$DEV_TOKEN" python3 -c "
import os, json, base64
try:
    part = os.environ['DEV_TOKEN'].split('.')[1]
    part += '=' * (-len(part) % 4)
    print(json.loads(base64.urlsafe_b64decode(part)).get('sub','') or '')
except Exception:
    pass" 2>/dev/null || true)
  fi

  if [[ -z "$DEV_SUBJECT" ]]; then
    fail_test "Developer user: could not resolve OIDC subject for workspace membership"
  else
    show_cmd "${OPENSHELL_BIN} -g ${GW_LOCAL_NAME} workspace member add --workspace default --subject ${DEV_SUBJECT} --role user"
    dim "  Admin grants developer 'user' membership on 'default' (OpenShell requires an explicit membership record; OIDC user role alone does not confer workspace access)..."
    DEV_MEMBER_LOG=$(mktemp)
    if "${OPENSHELL_BIN}" -g "${GW_LOCAL_NAME}" workspace member add \
        --workspace default --subject "${DEV_SUBJECT}" --role user >"${DEV_MEMBER_LOG}" 2>&1; then
      pass "Developer granted 'user' membership on 'default' workspace"
    else
      DEV_MEMBER_ERR=$(sed 's/\x1b\[[0-9;]*m//g' "${DEV_MEMBER_LOG}" 2>/dev/null | tr '\n' ' ' | tr -s ' ')
      if echo "$DEV_MEMBER_ERR" | grep -qiE "already|exists"; then
        pass "Developer already a 'user' member of 'default' workspace"
      else
        fail_test "Developer user: failed to grant workspace membership (admin)"
        dim "    ${DEV_MEMBER_ERR:0:200}"
      fi
    fi
    rm -f "${DEV_MEMBER_LOG}" 2>/dev/null || true
  fi

  # ── positive assertion: a workspace member with user_role MAY create a sandbox ──
  DEV_SANDBOX="e2e-dev-$(date +%s | tail -c5)"
  show_cmd "${OPENSHELL_BIN} -g ${DEV_GW_LOCAL_NAME} sandbox create --name ${DEV_SANDBOX}"
  dim "  Expecting success (developer is now a 'user' member of 'default'; sandbox create is allowed)..."

  DEV_SB_LOG=$(mktemp)
  "${OPENSHELL_BIN}" -g "${DEV_GW_LOCAL_NAME}" sandbox create --name "${DEV_SANDBOX}" >"${DEV_SB_LOG}" 2>&1 &
  DEV_SB_PID=$!

  # sandbox create blocks (interactive), so background it and poll for the pod.
  DEV_POD_CREATED=false
  DEV_DEADLINE=$(($(date +%s) + E2E_SANDBOX_TIMEOUT))
  while [[ $(date +%s) -lt $DEV_DEADLINE ]]; do
    if $CLI get pods -n "$GW_NAMESPACE" --no-headers 2>/dev/null | grep -qi "default--${DEV_SANDBOX}"; then
      DEV_POD_CREATED=true
      break
    fi
    if ! kill -0 "$DEV_SB_PID" 2>/dev/null; then
      break
    fi
    sleep 5
  done

  kill "$DEV_SB_PID" 2>/dev/null || true
  wait "$DEV_SB_PID" 2>/dev/null || true

  DEV_SB_ERR=$(sed 's/\x1b\[[0-9;]*m//g' "${DEV_SB_LOG}" 2>/dev/null | tr '\n' ' ' | tr -s ' ')
  rm -f "${DEV_SB_LOG}" 2>/dev/null || true

  if [[ "$DEV_POD_CREATED" == "true" ]]; then
    pass "Developer user: sandbox create allowed (user_role member of 'default')"
    "${OPENSHELL_BIN}" -g "${DEV_GW_LOCAL_NAME}" sandbox delete "${DEV_SANDBOX}" 2>&1 || true
  elif echo "$DEV_SB_ERR" | grep -qiE "not a member|permissiondenied|permission denied|not authorized|unauthorized|forbidden|denied"; then
    # A granted workspace member was still denied -> membership grant or user_role
    # mapping is misconfigured.
    fail_test "Developer user: sandbox create denied -- a 'user' member of 'default' should be allowed to create sandboxes"
    dim "    ${DEV_SB_ERR:0:200}"
  else
    # Neither created nor a recognizable denial -- surface output so infra
    # failures are not mistaken for an authz result.
    fail_test "Developer user: sandbox not created within ${E2E_SANDBOX_TIMEOUT}s"
    dim "    ${DEV_SB_ERR:0:200}"
  fi
  fi

  # ── negative assertion: openshell-user may NOT create a gateway ──
  # gateway:viewer lacks the platform-scoped gateway:creator role, so
  # POST /gateways MUST be rejected with 403 (rbac-enforcement.spec.md scenario
  # "User without creator role cannot create gateways"). SUCCESS here would mean
  # RBAC is NOT enforced.
  DEV_GW_CREATE_NAME="e2e-dev-gw-$(date +%s | tail -c5)"
  DEV_GW_BODY=$(GW_NAME="$DEV_GW_CREATE_NAME" E2E_OIDC_ISSUER="$E2E_OIDC_ISSUER" \
    E2E_OIDC_CLIENT_ID="$E2E_OIDC_CLIENT_ID" python3 -c "
import json, os
body = {
    'name': os.environ['GW_NAME'],
    'cluster_id': 'e2e-cluster',
    'release_id': 'e2e-release',
    'database_id': 'e2e-database',
    'oidc': json.dumps({
        'issuer': os.environ['E2E_OIDC_ISSUER'],
        'audience': os.environ['E2E_OIDC_CLIENT_ID'],
        'roles_claim': 'groups',
        'admin_role': 'hypershell-admins',
        'user_role': 'hypershell-users'
    }),
    'route': json.dumps({'enabled': True})
}
print(json.dumps(body))
")
  show_cmd "curl -X POST ${API_HOST}/api/hypershell/v1/gateways (as developer) -> expect 403"
  dim "  Expecting 403 Forbidden (developer lacks gateway:creator)..."

  DEV_GW_RESP_FILE=$(mktemp)
  DEV_GW_STATUS=$(curl -sk -o "${DEV_GW_RESP_FILE}" -w '%{http_code}' \
    -X POST "${API_HOST}/api/hypershell/v1/gateways" \
    -H "Authorization: Bearer ${DEV_TOKEN}" \
    -H "Content-Type: application/json" \
    -d "${DEV_GW_BODY}" 2>/dev/null || true)
  DEV_GW_RESP=$(sed 's/\x1b\[[0-9;]*m//g' "${DEV_GW_RESP_FILE}" 2>/dev/null | tr '\n' ' ' | tr -s ' ')

  if [[ "$DEV_GW_STATUS" == "403" ]]; then
    pass "Developer user: gateway create correctly denied (403 Forbidden)"
  elif [[ "$DEV_GW_STATUS" =~ ^2 ]]; then
    fail_test "Developer user: RBAC not enforced -- non-creator created a gateway (HTTP ${DEV_GW_STATUS})"
    # A gateway was wrongly created; the creator auto-owns it, so delete it as the
    # developer to avoid leaking test state.
    DEV_BAD_GW_ID=$(echo "$DEV_GW_RESP" | python3 -c "import json,sys; print(json.load(sys.stdin).get('id',''))" 2>/dev/null || true)
    if [[ -n "$DEV_BAD_GW_ID" ]]; then
      curl -sk -X DELETE "${API_HOST}/api/hypershell/v1/gateways/${DEV_BAD_GW_ID}" \
        -H "Authorization: Bearer ${DEV_TOKEN}" &>/dev/null || true
    fi
  else
    fail_test "Developer user: gateway create did not return 403 (got HTTP ${DEV_GW_STATUS:-none})"
    dim "    ${DEV_GW_RESP:0:200}"
  fi
  rm -f "${DEV_GW_RESP_FILE}" 2>/dev/null || true

  "${OPENSHELL_BIN}" gateway remove "${DEV_GW_LOCAL_NAME}" 2>/dev/null || true
fi
sep

# ── 10. platform admin RBAC verification ─────────────────────────────────

echo ""
bold "10. Platform Admin RBAC Verification"
echo ""

if ! e2e_step long; then
  dim "  Skipped (E2E_MODE=short): platform-admin assertions delete a gateway"
else
# The platform:admin role is a realm role (not a client role) assigned in Keycloak.
# Platform admins can view all gateways and delete any gateway, but cannot modify
# gateways they don't own or create gateways without gateway:creator.

# Assign platform:admin realm role to the platform admin user (best-effort; user may
# already have the role from Keycloak realm import)
if [[ "${E2E_INFRA_DRIVER}" == "kind" ]]; then
  show_cmd "# verify/assign platform:admin realm role to ${E2E_PLATFORM_ADMIN_USERNAME}"
  if assign_realm_role "$E2E_PLATFORM_ADMIN_USERNAME" "platform:admin"; then
    pass "Platform admin has platform:admin realm role"
  else
    dim "  Note: Could not verify platform:admin role assignment (user may already have it from realm import)"
  fi
fi

# Acquire OIDC token for platform admin
show_cmd "# acquire OIDC token for platform admin (user: ${E2E_PLATFORM_ADMIN_USERNAME})"
acquire_oidc_token "$E2E_PLATFORM_ADMIN_USERNAME" "$E2E_PLATFORM_ADMIN_PASSWORD"
PADMIN_TOKEN="${_OIDC_ACCESS_TOKEN}"
if [[ -n "$PADMIN_TOKEN" ]]; then
  pass "Platform admin OIDC token acquired (user: ${E2E_PLATFORM_ADMIN_USERNAME})"
else
  fail_test "Failed to acquire platform admin OIDC token"
fi

if [[ -n "$PADMIN_TOKEN" ]]; then
  # ── positive assertion: platform:admin can list all gateways ──
  show_cmd "curl -H 'Authorization: Bearer ...' ${API_HOST}/api/hypershell/v1/gateways"
  dim "  Expecting 200 OK (platform:admin can view all gateways)..."

  PADMIN_LIST_FILE=$(mktemp)
  PADMIN_LIST_STATUS=$(curl -sk -o "${PADMIN_LIST_FILE}" -w '%{http_code}' \
    -H "Authorization: Bearer ${PADMIN_TOKEN}" \
    "${API_HOST}/api/hypershell/v1/gateways" 2>/dev/null || true)
  PADMIN_LIST_RESP=$(cat "${PADMIN_LIST_FILE}" 2>/dev/null || true)

  if [[ "$PADMIN_LIST_STATUS" == "200" ]]; then
    PADMIN_GW_COUNT=$(echo "$PADMIN_LIST_RESP" | python3 -c "import json,sys; print(len(json.load(sys.stdin).get('items',[])))" 2>/dev/null || echo "0")
    pass "Platform admin: can list all gateways (HTTP 200, ${PADMIN_GW_COUNT} gateways)"
  else
    fail_test "Platform admin: gateway list denied (HTTP ${PADMIN_LIST_STATUS:-none})"
    dim "    ${PADMIN_LIST_RESP:0:200}"
  fi
  rm -f "${PADMIN_LIST_FILE}" 2>/dev/null || true

  # ── positive assertion: platform:admin can delete gateway they don't own ──
  # The platform admin user has NOT been granted gateway:owner on the e2e gateway
  # created by the admin user, but should still be able to delete it via platform:admin.
  show_cmd "curl -X DELETE ${API_HOST}/api/hypershell/v1/gateways/${GW_ID} (as platform admin)"
  dim "  Expecting 204 No Content (platform:admin can delete gateways they don't own)..."

  # Before deleting, verify platform admin is NOT the owner by checking role bindings
  show_cmd "# verify platform admin has NO owner binding on ${GW_NAME}"
  PADMIN_BINDINGS_FILE=$(mktemp)
  PADMIN_BINDINGS_STATUS=$(curl -sk -o "${PADMIN_BINDINGS_FILE}" -w '%{http_code}' \
    -H "Authorization: Bearer ${PADMIN_TOKEN}" \
    "${API_HOST}/api/hypershell/v1/role_bindings?gateway_id=${GW_ID}" 2>/dev/null || true)

  if [[ "$PADMIN_BINDINGS_STATUS" == "200" ]]; then
    PADMIN_HAS_OWNER=$(echo "$(cat "${PADMIN_BINDINGS_FILE}")" | python3 -c "
import json,sys
bindings = json.load(sys.stdin).get('items',[])
has_owner = any(b.get('role_id','').endswith('owner') for b in bindings)
print('true' if has_owner else 'false')
" 2>/dev/null || echo "false")

    if [[ "$PADMIN_HAS_OWNER" == "false" ]]; then
      pass "Platform admin has NO gateway:owner binding on ${GW_NAME} (verified)"
    else
      fail_test "Platform admin unexpectedly has gateway:owner binding (test setup issue)"
    fi
  fi
  rm -f "${PADMIN_BINDINGS_FILE}" 2>/dev/null || true

  # Now attempt delete as platform admin
  PADMIN_DELETE_FILE=$(mktemp)
  PADMIN_DELETE_STATUS=$(curl -sk -o "${PADMIN_DELETE_FILE}" -w '%{http_code}' \
    -X DELETE "${API_HOST}/api/hypershell/v1/gateways/${GW_ID}" \
    -H "Authorization: Bearer ${PADMIN_TOKEN}" 2>/dev/null || true)
  PADMIN_DELETE_RESP=$(cat "${PADMIN_DELETE_FILE}" 2>/dev/null || true)

  if [[ "$PADMIN_DELETE_STATUS" == "204" ]]; then
    pass "Platform admin: can delete gateway without ownership (HTTP 204)"
    # Clear GW_ID so cleanup trap doesn't try to delete it again
    GW_ID=""
  else
    fail_test "Platform admin: gateway delete denied (HTTP ${PADMIN_DELETE_STATUS:-none})"
    dim "    ${PADMIN_DELETE_RESP:0:200}"
  fi
  rm -f "${PADMIN_DELETE_FILE}" 2>/dev/null || true

  # ── negative assertion: platform:admin cannot create gateways without gateway:creator ──
  PADMIN_GW_CREATE_NAME="e2e-padmin-gw-$(date +%s | tail -c5)"
  PADMIN_GW_BODY=$(GW_NAME="$PADMIN_GW_CREATE_NAME" E2E_OIDC_ISSUER="$E2E_OIDC_ISSUER" \
    E2E_OIDC_CLIENT_ID="$E2E_OIDC_CLIENT_ID" python3 -c "
import json, os
body = {
    'name': os.environ['GW_NAME'],
    'cluster_id': 'e2e-cluster',
    'release_id': 'e2e-release',
    'database_id': 'e2e-database',
    'oidc': json.dumps({
        'issuer': os.environ['E2E_OIDC_ISSUER'],
        'audience': os.environ['E2E_OIDC_CLIENT_ID'],
        'roles_claim': 'groups',
        'admin_role': 'hypershell-admins',
        'user_role': 'hypershell-users'
    }),
    'route': json.dumps({'enabled': True})
}
print(json.dumps(body))
")
  show_cmd "curl -X POST ${API_HOST}/api/hypershell/v1/gateways (as platform admin) -> expect 403"
  dim "  Expecting 403 Forbidden (platform:admin lacks gateway:creator)..."

  PADMIN_CREATE_FILE=$(mktemp)
  PADMIN_CREATE_STATUS=$(curl -sk -o "${PADMIN_CREATE_FILE}" -w '%{http_code}' \
    -X POST "${API_HOST}/api/hypershell/v1/gateways" \
    -H "Authorization: Bearer ${PADMIN_TOKEN}" \
    -H "Content-Type: application/json" \
    -d "${PADMIN_GW_BODY}" 2>/dev/null || true)
  PADMIN_CREATE_RESP=$(cat "${PADMIN_CREATE_FILE}" 2>/dev/null || true)

  if [[ "$PADMIN_CREATE_STATUS" == "403" ]]; then
    pass "Platform admin: gateway create correctly denied (403 Forbidden)"
  elif [[ "$PADMIN_CREATE_STATUS" =~ ^2 ]]; then
    fail_test "Platform admin: RBAC not enforced -- platform:admin created gateway without gateway:creator (HTTP ${PADMIN_CREATE_STATUS})"
    # Clean up wrongly created gateway
    PADMIN_BAD_GW_ID=$(echo "$PADMIN_CREATE_RESP" | python3 -c "import json,sys; print(json.load(sys.stdin).get('id',''))" 2>/dev/null || true)
    if [[ -n "$PADMIN_BAD_GW_ID" ]]; then
      curl -sk -X DELETE "${API_HOST}/api/hypershell/v1/gateways/${PADMIN_BAD_GW_ID}" \
        -H "Authorization: Bearer ${PADMIN_TOKEN}" &>/dev/null || true
    fi
  else
    fail_test "Platform admin: gateway create did not return 403 (got HTTP ${PADMIN_CREATE_STATUS:-none})"
    dim "    ${PADMIN_CREATE_RESP:0:200}"
  fi
  rm -f "${PADMIN_CREATE_FILE}" 2>/dev/null || true
fi
fi
sep

# ── 11. gateway deletion + namespace garbage collection ────────────────────

echo ""
bold "11. Gateway Deletion + Namespace Garbage Collection"
echo ""

if [[ "$E2E_MODE" == "short" ]]; then
  # Short mode must not tear down the supplied/reused gateway. Exercise
  # delete-driven GC against a throwaway gateway instead, with a bounded wait.
  THROW_NAME="${GW_NAME}-gc-throwaway"
  dim "  Delete-driven GC on throwaway gateway ${THROW_NAME} (not ${GW_NAME})"
  acquire_oidc_token 2>/dev/null || true
  if [[ -z "${E2E_FLEET_ID:-}" ]]; then
    e2e_discover_seed_ids || true
  fi
  e2e_lookup_gateway_by_name "$THROW_NAME"
  THROW_ID="${_GW_ID}"
  THROW_NS="${_GW_NAMESPACE}"
  if [[ -z "$THROW_ID" ]]; then
    if [[ -z "${E2E_FLEET_ID:-}" || -z "${E2E_CLUSTER_ID:-}" || -z "${E2E_RELEASE_ID:-}" ]]; then
      fail_test "Cannot create throwaway gateway: seeded fleet/cluster/release ids are unknown"
    else
      THROW_BODY=$(e2e_gateway_create_body "$THROW_NAME")
      THROW_RESP=$(api_curl -X POST "${API_HOST}/api/hypershell/v1/gateways" \
        -H "Content-Type: application/json" -d "${THROW_BODY}" 2>/dev/null || true)
      e2e_parse_gateway_response "$THROW_RESP"
      if [[ "$_CREATE_KIND" == "OK" && -n "$_CREATE_ID" ]]; then
        THROW_ID="$_CREATE_ID"
        THROW_NS="$_CREATE_NAMESPACE"
        pass "Throwaway gateway created: ${THROW_NAME} (${THROW_ID})"
      else
        fail_test "Failed to create throwaway gateway ${THROW_NAME}"
        dim "    ${THROW_RESP:0:300}"
      fi
    fi
  else
    pass "Throwaway gateway already exists: ${THROW_NAME} (${THROW_ID})"
  fi

  if [[ -n "$THROW_ID" ]]; then
    if [[ -z "$THROW_NS" ]]; then
      # Namespace may lag the create response; poll briefly.
      THROW_NS_DEADLINE=$(($(date +%s) + 30))
      while [[ $(date +%s) -lt $THROW_NS_DEADLINE ]]; do
        e2e_lookup_gateway_by_name "$THROW_NAME"
        THROW_NS="${_GW_NAMESPACE}"
        [[ -n "$THROW_NS" ]] && break
        sleep 2
      done
    fi
    if [[ -z "$THROW_NS" ]]; then
      fail_test "Throwaway gateway ${THROW_NAME} has no namespace; cannot validate GC"
    else
      if $CLI get namespace "$THROW_NS" &>/dev/null; then
        pass "Throwaway namespace present before delete: ${THROW_NS}"
      else
        fail_test "Throwaway namespace ${THROW_NS} missing before delete"
      fi
      show_cmd "api_curl -X DELETE ${API_HOST}/api/hypershell/v1/gateways/${THROW_ID}"
      THROW_DEL=$(api_curl -o /dev/null -w '%{http_code}' -X DELETE \
        "${API_HOST}/api/hypershell/v1/gateways/${THROW_ID}" 2>/dev/null || true)
      if [[ "$THROW_DEL" == "204" || "$THROW_DEL" == "404" ]]; then
        pass "Throwaway gateway delete accepted (HTTP ${THROW_DEL})"
      else
        fail_test "Expected 204 or 404 deleting throwaway gateway, got ${THROW_DEL:-none}"
      fi
      dim "  Waiting for throwaway namespace ${THROW_NS} to be garbage collected (up to ${E2E_GC_TIMEOUT}s)..."
      THROW_GONE=false
      THROW_DEADLINE=$(($(date +%s) + E2E_GC_TIMEOUT))
      while [[ $(date +%s) -lt $THROW_DEADLINE ]]; do
        if ! $CLI get namespace "$THROW_NS" &>/dev/null; then
          THROW_GONE=true
          break
        fi
        sleep 5
      done
      if [[ "$THROW_GONE" == "true" ]]; then
        pass "Throwaway namespace garbage collected: ${THROW_NS}"
      else
        fail_test "Throwaway namespace ${THROW_NS} not garbage collected after ${E2E_GC_TIMEOUT}s"
        e2e_dump_namespace_gc_logs "${E2E_HS_NAMESPACE}" "$CLI"
      fi
    fi
  fi
elif [[ "$E2E_SKIP_CLEANUP" == "1" ]]; then
  dim "  Skipped (E2E_SKIP_CLEANUP=1): preserving namespace ${GW_NAMESPACE}"
elif [[ -z "$GW_NAMESPACE" ]]; then
  fail_test "Cannot validate namespace GC: gateway namespace is unknown"
else
  # 11b. Delete-driven GC first so the suite is not blocked waiting for the
  # periodic reaper; the orphan was seeded after step 2 and may already be gone.
  # Section 10 deletes the gateway as the platform admin and clears GW_ID.
  # Deleting the Gateway via the API drives the control-plane delete path
  # (watch-delete-events.spec.md): DeleteGatewayResources then
  # DeleteManagedNamespace, best-effort and idempotent. The gateway namespace is
  # managed (carries both hypershell.redhat.io/managed=true and
  # app.kubernetes.io/managed-by=hypershell-control-plane), so it MUST be reaped.
  # Any namespace missed by the delete path is later swept by the
  # NamespaceGCReconciler. See openshell-gateway-namespace-gc.spec.md
  # (HYPERSHELL-96, HYPERSHELL-78).

  # If the gateway was not already deleted (e.g. the platform-admin delete was
  # skipped or failed), delete it now as a fallback so the namespace GC has a
  # trigger. The platform-admin section overwrote the active token, so
  # re-acquire the default admin token before calling the API. Accept 204
  # (deleted now) or 404 (already gone).
  if [[ -n "$GW_ID" ]]; then
    acquire_oidc_token 2>/dev/null || true
    show_cmd "api_curl -X DELETE ${API_HOST}/api/hypershell/v1/gateways/${GW_ID}"
    DEL_STATUS=$(api_curl -o /dev/null -w '%{http_code}' -X DELETE \
      "${API_HOST}/api/hypershell/v1/gateways/${GW_ID}" 2>/dev/null || true)
    if [[ "$DEL_STATUS" == "204" || "$DEL_STATUS" == "404" ]]; then
      pass "Gateway delete accepted (HTTP ${DEL_STATUS})"
    else
      fail_test "Expected 204 or 404 deleting gateway, got ${DEL_STATUS:-none}"
    fi
    GW_ID=""
  else
    dim "  Gateway already deleted by the platform-admin section; validating namespace GC"
  fi

  dim "  11b. Delete-driven gateway namespace GC: ${GW_NAMESPACE}"
  # The managed namespace must be garbage collected by the control plane. Allow
  # headroom for the namespace to enter Terminating and finalize (pods, PVC,
  # certificates).
  show_cmd "$CLI get namespace ${GW_NAMESPACE} (expect NotFound)"
  dim "  Waiting for namespace ${GW_NAMESPACE} to be garbage collected (up to ${E2E_GC_TIMEOUT}s)..."
  NS_GONE=false
  GC_DEADLINE=$(($(date +%s) + E2E_GC_TIMEOUT))
  while [[ $(date +%s) -lt $GC_DEADLINE ]]; do
    if ! $CLI get namespace "$GW_NAMESPACE" &>/dev/null; then
      NS_GONE=true
      break
    fi
    NS_PHASE=$($CLI get namespace "$GW_NAMESPACE" -o jsonpath='{.status.phase}' 2>/dev/null || true)
    dim "    namespace: ${NS_PHASE:-present}"
    sleep 5
  done

  if [[ "$NS_GONE" == "true" ]]; then
    pass "Gateway namespace garbage collected: ${GW_NAMESPACE}"
  else
    fail_test "Namespace ${GW_NAMESPACE} not garbage collected after ${E2E_GC_TIMEOUT}s"
    dim "  --- namespace GC diagnostics ---"
    $CLI get namespace "$GW_NAMESPACE" -o yaml 2>&1 | tail -40 | while IFS= read -r line; do dim "    $line"; done
    dim "  Namespace GC controller logs:"
    e2e_dump_namespace_gc_logs "${E2E_HS_NAMESPACE}" "$CLI"
  fi

  if [[ "${DB_PROVIDER}" == "deployment" && -n "${GW_DB_ID:-}" ]]; then
    dim "  Waiting for dedicated ManagedDatabase ${GW_DB_ID} and namespace ${DB_GW_NAMESPACE} to be deleted..."
    DB_GONE=false
    DB_GC_DEADLINE=$(($(date +%s) + E2E_GC_TIMEOUT))
    while [[ $(date +%s) -lt $DB_GC_DEADLINE ]]; do
      acquire_oidc_token 2>/dev/null || true
      DB_HTTP=$(api_curl -o /dev/null -w '%{http_code}' \
        "${API_HOST}/api/hypershell/v1/managed_databases/${GW_DB_ID}" 2>/dev/null || true)
      if [[ "$DB_HTTP" == "404" ]] && ! $CLI get namespace "$DB_GW_NAMESPACE" &>/dev/null; then
        DB_GONE=true
        break
      fi
      dim "    ManagedDatabase HTTP=${DB_HTTP:-unknown}, namespace=$($CLI get namespace "$DB_GW_NAMESPACE" -o jsonpath='{.status.phase}' 2>/dev/null || echo absent)"
      sleep 5
    done
    if [[ "$DB_GONE" == "true" ]]; then
      pass "Dedicated deployment database deleted with gateway: ${GW_DB_ID}"
    else
      fail_test "ManagedDatabase ${GW_DB_ID} or namespace ${DB_GW_NAMESPACE} remained after gateway deletion"
    fi
  fi

  # 11a. Periodic reaper (NamespaceGCReconciler + recordGCEvent). Orphan namespace
  # was seeded after gateway provisioning; validate reap + Event without blocking
  # earlier steps on the sweep interval.
  if [[ -n "$ORPHAN_NS" && "$ORPHAN_GC_DEADLINE" -gt 0 ]]; then
    dim "  11a. Periodic orphan namespace GC: ${ORPHAN_NS}"
    ORPHAN_GONE=false
    if ! $CLI get namespace "$ORPHAN_NS" &>/dev/null; then
      ORPHAN_GONE=true
    else
      REMAINING=$((ORPHAN_GC_DEADLINE - $(date +%s)))
      if [[ $REMAINING -gt 0 ]]; then
        dim "  Orphan still present; waiting up to ${REMAINING}s (deadline from seed time)..."
      fi
      while [[ $(date +%s) -lt $ORPHAN_GC_DEADLINE ]]; do
        if ! $CLI get namespace "$ORPHAN_NS" &>/dev/null; then
          ORPHAN_GONE=true
          break
        fi
        sleep 5
      done
    fi

    if [[ "$ORPHAN_GONE" == "true" ]]; then
      pass "Periodic reaper garbage collected orphan namespace: ${ORPHAN_NS}"
    else
      fail_test "Orphan namespace ${ORPHAN_NS} not garbage collected within ${E2E_ORPHAN_GC_TIMEOUT}s of seeding"
      dim "  --- orphan namespace GC diagnostics ---"
      $CLI get namespace "$ORPHAN_NS" -o yaml 2>&1 | tail -40 | while IFS= read -r line; do dim "    $line"; done
      dim "  Namespace GC controller logs:"
      e2e_dump_namespace_gc_logs "${E2E_HS_NAMESPACE}" "$CLI"
    fi

    if [[ "$ORPHAN_GONE" == "true" ]]; then
      GC_EVENT=$($CLI get events -n "${E2E_HS_NAMESPACE}" \
        --field-selector="involvedObject.name=${ORPHAN_NS},reason=GarbageCollected" \
        -o jsonpath='{.items[0].reason}' 2>/dev/null || true)
      if [[ "$GC_EVENT" == "GarbageCollected" ]]; then
        pass "GarbageCollected Event recorded for ${ORPHAN_NS} in ${E2E_HS_NAMESPACE}"
      else
        fail_test "Expected GarbageCollected Event for ${ORPHAN_NS} in ${E2E_HS_NAMESPACE}, got ${GC_EVENT:-none}"
      fi
    fi
  fi
fi
sep

# ── results ───────────────────────────────────────────────────────────────

print_results

if [[ $E2E_FAIL -gt 0 ]]; then
  exit 1
fi
