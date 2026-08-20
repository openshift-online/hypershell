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

# ROKS (hysh-ibm-01) defaults - this is the ROKS-adapted copy of e2e-openshell.sh,
# so the defaults target the live IBM cluster layout. Every value is still
# env-overridable for other clusters.
CLI="${OC:-oc}"
# openshell CLI: must be new enough to support `workspace member add` (>= 0.0.98).
# The system /bin/openshell on some hosts is 0.0.55 and lacks the `workspace`
# subcommand, so default to the user-local install that has it.
OPENSHELL="${OPENSHELL_BIN:-$HOME/.local/bin/openshell}"
HSCTL="${HSCTL_BIN:-/home/mturansk/projects/bin/hsctl}"
HS_NAMESPACE="${HYPERSHELL_NAMESPACE:-hypershell}"
GW_NAMESPACE=""
GW_NAME="${GATEWAY_NAME:-e2e-oidc-gw}"
SANDBOX_TIMEOUT="${SANDBOX_TIMEOUT:-240}"
# After the sandbox POD is Running, the openshell runner still needs to reach
# its own Ready phase before exec works. ROKS init is slow, so allow extra time.
SANDBOX_READY_TIMEOUT="${SANDBOX_READY_TIMEOUT:-150}"
PROVISION_TIMEOUT="${PROVISION_TIMEOUT:-180}"
SKIP_CLEANUP="${SKIP_CLEANUP:-}"
LAUNCH_TUI="${LAUNCH_TUI:-0}"
PAUSE="${PAUSE:-0}"

KC_NAMESPACE="${KEYCLOAK_NAMESPACE:-keycloak-system}"
# Management-plane client: used only for the hsctl login below. The ROKS API
# server runs with --enable-jwt=false, so this token is advisory (the API server
# does not validate it), but hsctl still wants a bearer to attach.
MGMT_CLIENT_ID="${MGMT_CLIENT_ID:-${OIDC_CLIENT_ID:-hypershell-frontend}}"
# Keycloak admin service account secret. Read to perform the per-gateway client
# role assignment the control plane's OIDC Role Bridge would normally drive from
# a RoleBinding (see the Role Bridge section below).
KC_ADMIN_SECRET="${KC_ADMIN_SECRET:-hypershell-keycloak-admin}"
OIDC_USERNAME="${OIDC_USERNAME:-admin}"
OIDC_PASSWORD="${OIDC_PASSWORD:-admin}"
DEV_USERNAME="${DEV_USERNAME:-developer}"
DEV_PASSWORD="${DEV_PASSWORD:-developer}"
# Per-gateway Keycloak client id ({name}-{id}); resolved once the gateway id is
# known. This is the OIDC audience the gateway pod validates and the client the
# openshell CLI authenticates against -- NOT the management-plane client.
GW_OIDC_CLIENT_ID=""

# Mirrored gateway images (ROKS nodes can only pull the internal registry). Used
# only when this run has to CREATE the gateway; ignored when it already exists.
REG_MIRROR="${REG_MIRROR:-image-registry.openshift-image-registry.svc:5000/openshift}"
GW_IMAGE="${GW_IMAGE:-${REG_MIRROR}/openshell-gateway:0.0.109}"
GW_SUPERVISOR_IMAGE="${GW_SUPERVISOR_IMAGE:-${REG_MIRROR}/openshell-supervisor:0.0.109}"

PASS=0
FAIL=0
TESTS=()
PF_PID=""
SANDBOX_NAME=""
GW_ID=""
CREATED_GW=""   # only set when THIS run created the gateway; guards cleanup so a
                # pre-existing (persistent) gateway is never deleted on exit.

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
  if [[ "$SKIP_CLEANUP" != "1" && "$CREATED_GW" == "1" && -n "$GW_ID" ]]; then
    dim "  Cleaning up gateway ${GW_NAME}..."
    "${HSCTL}" delete gateway "${GW_ID}" --yes &>/dev/null || true
  fi
}
trap cleanup EXIT

API_HOST=$($CLI get route hypershell-api -n "$HS_NAMESPACE" -o jsonpath='{.spec.host}' 2>/dev/null || true)
if [[ -z "$API_HOST" ]]; then
  red "ERROR: HyperShell API route not found in namespace ${HS_NAMESPACE}"
  exit 1
fi

KC_HOST=$($CLI get route keycloak -n "$KC_NAMESPACE" -o jsonpath='{.spec.host}' 2>/dev/null || true)
if [[ -z "$KC_HOST" ]]; then
  red "ERROR: Keycloak route not found in namespace ${KC_NAMESPACE}"
  exit 1
fi
OIDC_ISSUER="https://${KC_HOST}/realms/hypershell"
TOKEN_ENDPOINT="${OIDC_ISSUER}/protocol/openid-connect/token"

# ── Keycloak admin service account (OIDC Role Bridge equivalent) ────────────
# The gateway pod validates data-plane tokens against its own per-gateway
# Keycloak client ({name}-{id}): audience = that client id, roles read from the
# hypershell.roles claim (openshell-admin / openshell-user). Those client roles
# are normally assigned by the control plane's OIDC Role Bridge, which is driven
# by a gateway:owner / gateway:viewer RoleBinding. On this cluster the API server
# runs with --enable-authz=false --enable-jwt=false, so no authenticated creator
# exists, no RoleBinding is auto-provisioned, and the Role Bridge never fires.
# The e2e therefore performs the same assignment the bridge would, using the
# hypershell-keycloak-admin service account.
# See specs/platform/openshell-gateway-keycloak.spec.md (RBAC Role Bridge).
KC_REALM=""
KC_SA_TOKEN=""
kc_sa_bootstrap() {
  local sec realm cid csec
  sec=$($CLI get secret "$KC_ADMIN_SECRET" -n "$HS_NAMESPACE" -o json 2>/dev/null || true)
  if [[ -z "$sec" ]]; then return 1; fi
  realm=$(echo "$sec" | python3 -c "import json,sys,base64; d=json.load(sys.stdin)['data']; print(base64.b64decode(d['realm']).decode())" 2>/dev/null || true)
  cid=$(echo "$sec" | python3 -c "import json,sys,base64; d=json.load(sys.stdin)['data']; print(base64.b64decode(d['client-id']).decode())" 2>/dev/null || true)
  csec=$(echo "$sec" | python3 -c "import json,sys,base64; d=json.load(sys.stdin)['data']; print(base64.b64decode(d['client-secret']).decode())" 2>/dev/null || true)
  [[ -n "$realm" && -n "$cid" && -n "$csec" ]] || return 1
  KC_REALM="$realm"
  KC_SA_TOKEN=$(curl -sk -X POST "https://${KC_HOST}/realms/${KC_REALM}/protocol/openid-connect/token" \
    -d grant_type=client_credentials -d "client_id=${cid}" -d "client_secret=${csec}" 2>/dev/null \
    | python3 -c "import json,sys; print(json.load(sys.stdin).get('access_token',''))" 2>/dev/null || true)
  [[ -n "$KC_SA_TOKEN" ]]
}

# assign_gateway_role <username> <openshell-admin|openshell-user>
# Idempotently assigns a per-gateway Keycloak client role to a realm user,
# mirroring RoleBindingReconciler.AssignClientRole.
assign_gateway_role() {
  local username="$1" role="$2"
  local uuid user_id role_json
  uuid=$(curl -sk "https://${KC_HOST}/admin/realms/${KC_REALM}/clients?clientId=${GW_OIDC_CLIENT_ID}" \
    -H "Authorization: Bearer ${KC_SA_TOKEN}" 2>/dev/null \
    | python3 -c "import json,sys; d=json.load(sys.stdin); print(d[0]['id'] if d else '')" 2>/dev/null || true)
  user_id=$(curl -sk "https://${KC_HOST}/admin/realms/${KC_REALM}/users?username=${username}&exact=true" \
    -H "Authorization: Bearer ${KC_SA_TOKEN}" 2>/dev/null \
    | python3 -c "import json,sys; d=json.load(sys.stdin); print(d[0]['id'] if d else '')" 2>/dev/null || true)
  if [[ -z "$uuid" || -z "$user_id" ]]; then return 1; fi
  role_json=$(curl -sk "https://${KC_HOST}/admin/realms/${KC_REALM}/clients/${uuid}/roles/${role}" \
    -H "Authorization: Bearer ${KC_SA_TOKEN}" 2>/dev/null || true)
  echo "$role_json" | python3 -c "import json,sys; json.load(sys.stdin)['id']" &>/dev/null || return 1
  curl -sk -o /dev/null -X POST \
    "https://${KC_HOST}/admin/realms/${KC_REALM}/users/${user_id}/role-mappings/clients/${uuid}" \
    -H "Authorization: Bearer ${KC_SA_TOKEN}" -H "Content-Type: application/json" \
    -d "[${role_json}]" 2>/dev/null
}

# mint_gw_token <username> <password> -> prints a data-plane access token for the
# per-gateway Keycloak client (resource-owner password grant; the client has
# directAccessGrantsEnabled=true).
mint_gw_token() {
  curl -sk -X POST "${TOKEN_ENDPOINT}" \
    -d grant_type=password -d "client_id=${GW_OIDC_CLIENT_ID}" \
    -d "username=$1" -d "password=$2" 2>/dev/null \
    | python3 -c "import json,sys; print(json.load(sys.stdin).get('access_token',''))" 2>/dev/null || true
}

# write_openshell_config <local_name> <config_dir> <access_token>
# Writes the openshell CLI gateway metadata + token in OIDC mode against the
# per-gateway client.
write_openshell_config() {
  local local_name="$1" config_dir="$2" token="$3"
  mkdir -p "$config_dir"
  GW_LOCAL_NAME="$local_name" GW_ENDPOINT="$GW_ENDPOINT" \
    OIDC_ISSUER="$OIDC_ISSUER" OIDC_CLIENT_ID="$GW_OIDC_CLIENT_ID" \
    OIDC_TOKEN="$token" GW_CONFIG_DIR="$config_dir" python3 -c "
import json, os
config_dir = os.environ['GW_CONFIG_DIR']
meta = {
    'name': os.environ['GW_LOCAL_NAME'],
    'gateway_endpoint': os.environ['GW_ENDPOINT'],
    'is_remote': True,
    'gateway_port': 0,
    'auth_mode': 'oidc',
    'oidc_issuer': os.environ['OIDC_ISSUER'],
    'oidc_client_id': os.environ['OIDC_CLIENT_ID']
}
with open(os.path.join(config_dir, 'metadata.json'), 'w') as f:
    json.dump(meta, f, indent=2)
token = {
    'access_token': os.environ['OIDC_TOKEN'],
    'issuer': os.environ['OIDC_ISSUER'],
    'client_id': os.environ['OIDC_CLIENT_ID']
}
with open(os.path.join(config_dir, 'oidc_token.json'), 'w') as f:
    json.dump(token, f, indent=2)
os.chmod(os.path.join(config_dir, 'metadata.json'), 0o600)
os.chmod(os.path.join(config_dir, 'oidc_token.json'), 0o600)
"
}

# sandbox_exec <local_name> <sandbox> <ready_timeout> -- <cmd...>
# Runs an openshell exec, retrying while the sandbox runner is still starting
# ("not ready" / "Provisioning"). The k8s pod reaching Running precedes the
# openshell sandbox reaching Ready, so exec must tolerate that lag. Prints the
# command output (stdout+stderr) and returns the exec's exit status.
sandbox_exec() {
  local ln="$1" sb="$2" to="$3"; shift 3
  local deadline=$(( $(date +%s) + to ))
  local out rc
  while :; do
    out=$("${OPENSHELL}" -g "$ln" sandbox exec -n "$sb" "$@" 2>&1); rc=$?
    if [[ $rc -eq 0 ]]; then printf '%s' "$out"; return 0; fi
    if echo "$out" | grep -qiE 'not ready|provisioning|not found' && [[ $(date +%s) -lt $deadline ]]; then
      sleep 5; continue
    fi
    printf '%s' "$out"; return $rc
  done
}

# Log the hypershell CLI into THIS cluster. hsctl 0.1.0 requires --url and a
# bearer token via --token-file; mint one from Keycloak (admin) so the run is
# self-contained rather than depending on a previously-saved config.
dim "Logging in to hypershell CLI (${API_HOST})..."
LOGIN_TOKEN=$(curl -sk -X POST "${TOKEN_ENDPOINT}" \
  -d "grant_type=password" -d "client_id=${MGMT_CLIENT_ID}" \
  -d "username=${OIDC_USERNAME}" -d "password=${OIDC_PASSWORD}" 2>/dev/null \
  | python3 -c "import json,sys; print(json.load(sys.stdin).get('access_token',''))" 2>/dev/null || true)
if [[ -n "$LOGIN_TOKEN" ]]; then
  echo "$LOGIN_TOKEN" | "${HSCTL}" login --url "https://${API_HOST}" --token-file /dev/stdin --insecure &>/dev/null || true
else
  "${HSCTL}" login --url "https://${API_HOST}" --insecure &>/dev/null || true
fi

echo ""
bold "HyperShell OpenShell Gateway End-to-End Test"
sep
echo ""
printf '  %s\n' "1. Gateway provisioning via HyperShell API (OIDC)"
printf '  %s\n' "1b. OIDC Role Bridge (per-gateway client roles)"
printf '  %s\n' "2. Gateway infrastructure verification"
printf '  %s\n' "3. OIDC token acquisition (per-gateway client)"
printf '  %s\n' "3a. CA certificate setup"
printf '  %s\n' "4. Route discovery + openshell CLI registration"
printf '  %s\n' "5. Gateway connectivity"
printf '  %s\n' "6. Sandbox lifecycle (create → ready)"
printf '  %s\n' "7. Sandbox interaction"
printf '  %s\n' "8. Developer user RBAC verification"
echo ""
dim  "  HyperShell API:    https://${API_HOST}"
dim  "  Keycloak:          https://${KC_HOST}"
dim  "  OIDC issuer:       ${OIDC_ISSUER}"
dim  "  Gateway name:      ${GW_NAME}"
dim  "  Sandbox timeout:   ${SANDBOX_TIMEOUT}s"
echo ""
sep

# ── 1. gateway provisioning ────────────────────────────────────────────────

echo ""
bold "1. Gateway Provisioning via HyperShell API (OIDC)"
echo ""

show_cmd "${HSCTL} list gateways --search \"name=${GW_NAME}\" -o json"
EXISTING_GW=$("${HSCTL}" list gateways --search "name = '${GW_NAME}'" -o json 2>/dev/null || true)
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
  show_cmd "curl -sk -X POST https://${API_HOST}/api/hypershell/v1/gateways -d '{name: ${GW_NAME}, oidc: ...}'"
  # OIDC configuration is system-managed: the control plane provisions the
  # per-gateway Keycloak client and populates oidc.* itself (read-only in the
  # API). Per openshell-gateway-keycloak.spec.md the create request MUST NOT
  # supply oidc -- we only ask for the gateway and its Route.
  GW_CREATE_BODY=$(GW_NAME="$GW_NAME" \
    GW_IMAGE="$GW_IMAGE" GW_SUPERVISOR_IMAGE="$GW_SUPERVISOR_IMAGE" python3 -c "
import json, os
body = {
    'name': os.environ['GW_NAME'],
    'fleet_id': 'e2e-fleet',
    'cluster_id': 'e2e-cluster',
    'release_id': 'e2e-release',
    'database_id': 'e2e-db',
    'image': os.environ['GW_IMAGE'],
    'supervisor_image': os.environ['GW_SUPERVISOR_IMAGE'],
    'route': json.dumps({
        'enabled': True
    })
}
print(json.dumps(body))
")
  CREATE_RESPONSE=$(curl -sk -X POST "https://${API_HOST}/api/hypershell/v1/gateways" \
    -H "Content-Type: application/json" \
    -d "${GW_CREATE_BODY}" 2>/dev/null || true)

  GW_ID=$(echo "$CREATE_RESPONSE" | python3 -c "import json,sys; print(json.load(sys.stdin).get('id',''))" 2>/dev/null || true)
  GW_NAMESPACE=$(echo "$CREATE_RESPONSE" | python3 -c "import json,sys; print(json.load(sys.stdin).get('namespace',''))" 2>/dev/null || true)

  if [[ -n "$GW_ID" ]]; then
    CREATED_GW=1
    pass "Gateway created: ${GW_NAME} (${GW_ID})"
  else
    fail_test "Failed to create gateway"
    dim "    ${CREATE_RESPONSE:0:300}"
    exit 1
  fi

  dim "  Waiting for controller to provision (timeout: ${PROVISION_TIMEOUT}s)..."
  DEADLINE=$(($(date +%s) + PROVISION_TIMEOUT))
  while [[ $(date +%s) -lt $DEADLINE ]]; do
    GW_PHASE=$("${HSCTL}" get gateway "${GW_ID}" 2>/dev/null | \
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

# Per-gateway Keycloak client id, per openshell-gateway-keycloak.spec.md.
GW_OIDC_CLIENT_ID="${GW_NAME}-${GW_ID}"
dim "  Per-gateway OIDC client: ${GW_OIDC_CLIENT_ID}"
sep

# ── 1b. OIDC Role Bridge (per-gateway client role assignment) ──────────────

echo ""
bold "1b. OIDC Role Bridge"
echo ""

show_cmd "# assign openshell-admin/openshell-user on client ${GW_OIDC_CLIENT_ID}"
if kc_sa_bootstrap; then
  pass "Keycloak admin service account ready (realm: ${KC_REALM})"
  if assign_gateway_role "$OIDC_USERNAME" openshell-admin; then
    pass "Assigned openshell-admin to ${OIDC_USERNAME} on ${GW_OIDC_CLIENT_ID}"
  else
    fail_test "Failed to assign openshell-admin to ${OIDC_USERNAME}"
  fi
  if assign_gateway_role "$DEV_USERNAME" openshell-user; then
    pass "Assigned openshell-user to ${DEV_USERNAME} on ${GW_OIDC_CLIENT_ID}"
  else
    fail_test "Failed to assign openshell-user to ${DEV_USERNAME}"
  fi
else
  fail_test "Keycloak admin service account not available (${KC_ADMIN_SECRET})"
  dim "  Cannot assign per-gateway client roles; sandbox operations will fail auth"
fi
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

# ── 3. OIDC token acquisition ────────────────────────────────────────────

echo ""
bold "3. OIDC Token Acquisition"
echo ""

show_cmd "# resource-owner password grant (client=${GW_OIDC_CLIENT_ID}) → ${OIDC_ISSUER}"
OIDC_TOKEN=$(mint_gw_token "$OIDC_USERNAME" "$OIDC_PASSWORD")
if [[ -n "$OIDC_TOKEN" && "$OIDC_TOKEN" != "None" ]]; then
  # Confirm the data-plane claims match what the gateway validates: aud must be
  # the per-gateway client and hypershell.roles must carry openshell-admin.
  CLAIMS=$(echo "$OIDC_TOKEN" | python3 -c "
import sys,base64,json
t=sys.stdin.read().strip().split('.')[1]; t+='='*(-len(t)%4)
c=json.loads(base64.urlsafe_b64decode(t))
print(c.get('aud',''), ','.join(c.get('hypershell',{}).get('roles',[])))
" 2>/dev/null || true)
  TOK_AUD=$(echo "$CLAIMS" | awk '{print $1}')
  TOK_ROLES=$(echo "$CLAIMS" | awk '{print $2}')
  if [[ "$TOK_AUD" == "$GW_OIDC_CLIENT_ID" && "$TOK_ROLES" == *"openshell-admin"* ]]; then
    pass "OIDC token acquired (user: ${OIDC_USERNAME}, aud=${TOK_AUD}, roles=${TOK_ROLES})"
  else
    fail_test "OIDC token has wrong claims (aud=${TOK_AUD:-none}, roles=${TOK_ROLES:-none})"
  fi
else
  fail_test "Failed to acquire OIDC token from Keycloak"
  exit 1
fi
sep

# ── 3a. extract and trust the cluster CA ──────────────────────────────────

echo ""
bold "3a. CA Certificate Setup"
echo ""

show_cmd "$CLI get secret hypershell-ca-secret -n $HS_NAMESPACE -o jsonpath='{.data.ca\.crt}' | base64 -d > /tmp/e2e-hypershell-ca.crt"
$CLI get secret openshell-server-tls -n "$GW_NAMESPACE" -o jsonpath="{.data.ca\.crt}" 2>/dev/null | base64 -d > /tmp/e2e-hypershell-ca.crt
if [[ -s /tmp/e2e-hypershell-ca.crt ]]; then
  export SSL_CERT_FILE=/tmp/e2e-hypershell-ca.crt
  pass "CA certificate extracted and SSL_CERT_FILE set"
  dim "    CA: /tmp/e2e-hypershell-ca.crt"
else
  fail_test "Failed to extract CA certificate"
  exit 1
fi
sep

# ── 4. route discovery + CLI registration ─────────────────────────────────

echo ""
bold "4. Route Discovery + CLI Registration"
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

show_cmd "# write gateway metadata (OIDC mode, client=${GW_OIDC_CLIENT_ID})"
write_openshell_config "$GW_LOCAL_NAME" "$GW_CONFIG_DIR" "$OIDC_TOKEN"

if [[ -f "${GW_CONFIG_DIR}/metadata.json" && -f "${GW_CONFIG_DIR}/oidc_token.json" ]]; then
  pass "openshell CLI registered (OIDC mode)"
else
  fail_test "Failed to write gateway config"
fi
sep

# ── 5. gateway connectivity ───────────────────────────────────────────────

echo ""
bold "5. Gateway Connectivity"
echo ""

show_cmd "${OPENSHELL} -g ${GW_LOCAL_NAME} status"
dim "  Waiting for route connectivity (up to 60s)..."
CONNECT_DEADLINE=$(($(date +%s) + 60))
STATUS_OUTPUT=""
CONNECTED=false
while [[ $(date +%s) -lt $CONNECT_DEADLINE ]]; do
  STATUS_OUTPUT=$("${OPENSHELL}" -g "${GW_LOCAL_NAME}" status 2>&1 || true)
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

# ── 6. sandbox lifecycle ──────────────────────────────────────────────────

echo ""
bold "6. Sandbox Lifecycle"
echo ""

RUN_ID=$(date +%s | tail -c5)
SANDBOX_NAME="e2e-${RUN_ID}"

# Refresh the data-plane token so it stays valid across the create+exec window
# (Keycloak access tokens are short-lived).
FRESH_TOKEN=$(mint_gw_token "$OIDC_USERNAME" "$OIDC_PASSWORD")
if [[ -n "$FRESH_TOKEN" && "$FRESH_TOKEN" != "None" ]]; then
  OIDC_TOKEN="$FRESH_TOKEN"
  write_openshell_config "$GW_LOCAL_NAME" "$GW_CONFIG_DIR" "$OIDC_TOKEN"
fi

show_cmd "${OPENSHELL} -g ${GW_LOCAL_NAME} sandbox create --name ${SANDBOX_NAME}"
dim "  Creating sandbox (timeout: ${SANDBOX_TIMEOUT}s)..."

"${OPENSHELL}" -g "${GW_LOCAL_NAME}" sandbox create --name "${SANDBOX_NAME}" &>/dev/null &
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

# ── 7. sandbox interaction ────────────────────────────────────────────────

echo ""
bold "7. Sandbox Interaction"
echo ""

GW_FLAG="-g ${GW_LOCAL_NAME}"

show_cmd "${OPENSHELL} ${GW_FLAG} sandbox exec -n ${SANDBOX_NAME} -- uname -a"
dim "  Waiting for sandbox runner Ready (up to ${SANDBOX_READY_TIMEOUT}s)..."
if SB_EXEC_OUTPUT=$(sandbox_exec "${GW_LOCAL_NAME}" "${SANDBOX_NAME}" "${SANDBOX_READY_TIMEOUT}" -- uname -a); then
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

show_cmd "${OPENSHELL} ${GW_FLAG} sandbox exec -n ${SANDBOX_NAME} -- ls -la /workspace"
if SB_LS_OUTPUT=$("${OPENSHELL}" -g "${GW_LOCAL_NAME}" sandbox exec -n "${SANDBOX_NAME}" -- ls -la /workspace 2>&1); then
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
  show_cmd "${OPENSHELL} ${GW_FLAG} sandbox delete ${SANDBOX_NAME}"
  "${OPENSHELL}" -g "${GW_LOCAL_NAME}" sandbox delete "${SANDBOX_NAME}" 2>&1 || true
  dim "  Sandbox deleted"
fi
sep

# ── 8. developer user RBAC verification ──────────────────────────────────

echo ""
bold "8. Developer User RBAC Verification"
echo ""

show_cmd "# acquire developer OIDC token (client=${GW_OIDC_CLIENT_ID})"
DEV_TOKEN=$(mint_gw_token "$DEV_USERNAME" "$DEV_PASSWORD")
if [[ -n "$DEV_TOKEN" && "$DEV_TOKEN" != "None" ]]; then
  DEV_ROLES=$(echo "$DEV_TOKEN" | python3 -c "
import sys,base64,json
t=sys.stdin.read().strip().split('.')[1]; t+='='*(-len(t)%4)
print(','.join(json.loads(base64.urlsafe_b64decode(t)).get('hypershell',{}).get('roles',[])))
" 2>/dev/null || true)
  pass "Developer OIDC token acquired (user: ${DEV_USERNAME}, roles=${DEV_ROLES:-none})"
else
  fail_test "Failed to acquire developer OIDC token"
fi

if [[ -n "$DEV_TOKEN" && "$DEV_TOKEN" != "None" ]]; then
  DEV_GW_LOCAL_NAME="${GW_LOCAL_NAME}-dev"
  DEV_CONFIG_DIR="${HOME}/.config/openshell/gateways/${DEV_GW_LOCAL_NAME}"
  mkdir -p "${DEV_CONFIG_DIR}"

  "${OPENSHELL}" gateway remove "${DEV_GW_LOCAL_NAME}" 2>/dev/null || true
  mkdir -p "${DEV_CONFIG_DIR}"

  show_cmd "# register gateway as developer user"
  write_openshell_config "$DEV_GW_LOCAL_NAME" "$DEV_CONFIG_DIR" "$DEV_TOKEN"

  if [[ -f "${DEV_CONFIG_DIR}/metadata.json" && -f "${DEV_CONFIG_DIR}/oidc_token.json" ]]; then
    pass "Developer gateway registered (OIDC mode)"
  else
    fail_test "Failed to write developer gateway config"
  fi

  show_cmd "${OPENSHELL} -g ${DEV_GW_LOCAL_NAME} status"
  DEV_STATUS=$("${OPENSHELL}" -g "${DEV_GW_LOCAL_NAME}" status 2>&1 || true)
  DEV_CLEAN=$(echo "$DEV_STATUS" | sed 's/\x1b\[[0-9;]*m//g')
  if echo "$DEV_CLEAN" | grep -qi "Connected"; then
    pass "Developer user: gateway connected"
  else
    fail_test "Developer user: gateway not reachable"
    echo "$DEV_STATUS" | while IFS= read -r line; do dim "    $line"; done
  fi

  # ── admin grants the developer 'user' membership on the 'default' workspace ──
  # OpenShell applies two independent authorization systems. The developer's OIDC
  # token carries the gateway user_role (openshell-user), but that role alone does
  # NOT confer workspace access: membership is a separate, explicit record that is
  # not claim-derived. A Platform Admin (implicit access to 'default') must add the
  # developer as a 'user' member before it can create sandboxes there. Without this
  # the create fails with "not a member of workspace 'default'". Mirrors the Kind
  # e2e (tests/e2e/e2e-openshell.sh) and the upstream oidc_pkce prepare_workspace.
  #
  # Resolve the subject the gateway checks membership against. Prefer `whoami`
  # (the gateway-validated identity); fall back to decoding the JWT `sub` claim if
  # the CLI predates `whoami`.
  DEV_SUBJECT=$("${OPENSHELL}" -g "${DEV_GW_LOCAL_NAME}" whoami --output json 2>/dev/null \
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
    show_cmd "${OPENSHELL} -g ${GW_LOCAL_NAME} workspace member add --workspace default --subject ${DEV_SUBJECT} --role user"
    dim "  Admin grants developer 'user' membership on 'default' (OpenShell requires an explicit membership record; OIDC user role alone does not confer workspace access)..."
    DEV_MEMBER_LOG=$(mktemp)
    if "${OPENSHELL}" -g "${GW_LOCAL_NAME}" workspace member add \
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

  DEV_SANDBOX="e2e-dev-$(date +%s | tail -c5)"
  show_cmd "${OPENSHELL} -g ${DEV_GW_LOCAL_NAME} sandbox create --name ${DEV_SANDBOX}"
  dim "  Creating developer sandbox (timeout: ${SANDBOX_TIMEOUT}s)..."

  "${OPENSHELL}" -g "${DEV_GW_LOCAL_NAME}" sandbox create --name "${DEV_SANDBOX}" &>/dev/null &
  DEV_SB_PID=$!

  DEV_SB_FOUND=false
  DEV_DEADLINE=$(($(date +%s) + SANDBOX_TIMEOUT))
  while [[ $(date +%s) -lt $DEV_DEADLINE ]]; do
    DEV_PODS=$($CLI get pods -n "$GW_NAMESPACE" --no-headers 2>/dev/null | grep -i "default--${DEV_SANDBOX}" || true)
    if [[ -n "$DEV_PODS" ]]; then
      DEV_POD_STATUS=$(echo "$DEV_PODS" | awk '{print $3}' | head -1)
      if [[ "$DEV_POD_STATUS" == "Running" ]]; then
        DEV_SB_FOUND=true
        break
      fi
    fi
    sleep 5
  done

  kill "$DEV_SB_PID" 2>/dev/null || true
  wait "$DEV_SB_PID" 2>/dev/null || true

  if [[ "$DEV_SB_FOUND" == "true" ]]; then
    pass "Developer user: sandbox created"

    show_cmd "${OPENSHELL} -g ${DEV_GW_LOCAL_NAME} sandbox exec -n ${DEV_SANDBOX} -- uname -a"
    dim "  Waiting for developer sandbox runner Ready (up to ${SANDBOX_READY_TIMEOUT}s)..."
    if DEV_EXEC=$(sandbox_exec "${DEV_GW_LOCAL_NAME}" "${DEV_SANDBOX}" "${SANDBOX_READY_TIMEOUT}" -- uname -a); then
      DEV_EXEC_CLEAN=$(echo "$DEV_EXEC" | sed 's/\x1b\[[0-9;]*m//g' | grep -v '^ *$' | grep -v 'WARN' | tail -3)
      if [[ -n "$DEV_EXEC_CLEAN" ]]; then
        pass "Developer user: sandbox exec succeeded"
      else
        fail_test "Developer user: sandbox exec returned no output"
      fi
    else
      fail_test "Developer user: sandbox exec failed"
      dim "    ${DEV_EXEC:0:200}"
    fi
  else
    fail_test "Developer user: sandbox not created after ${SANDBOX_TIMEOUT}s"
  fi

  if [[ "$SKIP_CLEANUP" != "1" && "$DEV_SB_FOUND" == "true" ]]; then
    dim "  Cleaning up developer sandbox..."
    "${OPENSHELL}" -g "${DEV_GW_LOCAL_NAME}" sandbox delete "${DEV_SANDBOX}" 2>&1 || true
  fi

  "${OPENSHELL}" gateway remove "${DEV_GW_LOCAL_NAME}" 2>/dev/null || true
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
  exec "${OPENSHELL}" -g "${GW_LOCAL_NAME}" term
fi

if [[ $FAIL -gt 0 ]]; then
  exit 1
fi
