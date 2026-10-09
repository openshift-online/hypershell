#!/usr/bin/env bash
# e2e-openshell.sh - infrastructure-agnostic end-to-end test of the OpenShell
# gateway provisioned by HyperShell.
#
# Proves the full path: HyperShell API -> control plane -> gateway provisioning
# -> openshell CLI -> sandbox pod creation + interaction.
#
# The infrastructure driver is auto-detected from the current KUBECONFIG
# context: a cluster that serves the route.openshift.io API group is
# OpenShift, otherwise Kind is assumed. Set E2E_INFRA_DRIVER to override
# detection. Each driver (tests/e2e/drivers/<driver>.sh) implements a fixed
# set of functions that abstract infrastructure-specific operations.
#
# Usage:
#   bash tests/e2e/e2e-openshell.sh
#   OPENSHIFT_NAMESPACE=my-env E2E_INFRA_DRIVER=openshift \
#     bash tests/e2e/e2e-openshell.sh   # override detection
#
# Environment variables:
#   E2E_INFRA_DRIVER      Infra driver override: kind, openshift (default: auto-detected)
#   E2E_NAMESPACE          Namespace for e2e resources (default: openshell-e2e)
#   E2E_GATEWAY_NAME       Gateway name (default: e2e-gw-<random8hex>, unique per run)
#   E2E_MODE               Run depth: long (default, every step), short (core
#                          gateway + sandbox lifecycle; owns+tears down its
#                          gateway, single identity; self-contained check safe
#                          against a live env, e.g. post-rollout promotion gate),
#                          or perf (short subset against a reused canary gateway;
#                          performance harness only)
#   E2E_SANDBOX_TIMEOUT    Seconds to wait for sandbox (default: 300)
#   E2E_PROVISION_TIMEOUT  Seconds to wait for gateway provisioning (default: 300)
#   E2E_GC_TIMEOUT         Seconds to wait for namespace GC after delete (default: 300)
#   E2E_ORPHAN_GC_TIMEOUT  Seconds to wait for periodic orphan namespace GC (default: 300)
#   E2E_SKIP_CLEANUP       Set to 1 to keep test resources after run (default: 0)
#   E2E_GATEWAY_AUTH       identity (default) or service_account (short mode:
#                          create a gateway credential through the public API)
#   OPENSHELL_BIN          Path to the openshell CLI binary (default: openshell)
#   E2E_OPENSHELL_INSTALL  auto, always, or never (default: always)
#   E2E_OPENSHELL_VERSION  Override CLI version/tag to install (e.g. v0.0.116, dev)
#   E2E_OPENSHELL_CLI_IMAGE  Container image to extract the CLI from (skips GitHub download)
#   E2E_OPENSHELL_INSTALL_DIR  Where to install the CLI (default: <repo>/bin, gitignored)
#   E2E_GATEWAY_VERSION_TIMEOUT  Seconds to wait for the runtime version (default: 300)
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# --- Source shared utilities ---
# shellcheck source=lib.sh
source "${SCRIPT_DIR}/lib.sh"

# --- Driver selection and validation ---

e2e_validate_mode
e2e_validate_openshell_install
e2e_select_infra_driver

DRIVER_FILE="${SCRIPT_DIR}/drivers/${E2E_INFRA_DRIVER}.sh"
if [[ ! -f "$DRIVER_FILE" ]]; then
  e2e_die_unknown_driver "Unknown driver '${E2E_INFRA_DRIVER}'. Driver file not found: ${DRIVER_FILE}"
fi

# shellcheck source=drivers/kind.sh
source "$DRIVER_FILE"
# shellcheck source=gateway_service_account.sh
source "${SCRIPT_DIR}/gateway_service_account.sh"
e2e_validate_gateway_auth

REQUIRED_FUNCTIONS=(discover_api_host discover_console_host discover_gateway_endpoint get_cluster_domain get_cli_binary wait_for_gateway_route acquire_oidc_token api_curl configure_namespace_gc_timing restore_namespace_gc_timing de_seed_test_users)
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
# Set by _install_openshell_cli_container_wrapper when OPENSHELL_BIN ends up
# resolving to a container-wrapper script rather than a native binary.
OPENSHELL_CLI_CONTAINERIZED=0
ORPHAN_NS=""
ORPHAN_GC_DEADLINE=0
SANDBOX_NAME=""
# Gateways created by the extended areas (12-13). The cleanup trap deletes each on
# any exit so an aborted run never leaks a provisioned gateway.
E2E_EXTRA_GW_IDS=()
E2E_GW_PF_PID="${E2E_GW_PF_PID:-}"
E2E_HS_NAMESPACE="${E2E_HS_NAMESPACE:-hypershell-system}"

# The installation step runs after gateway provisioning. Only the "never"
# mode requires an installed CLI before the test starts.
if command -v "${OPENSHELL_BIN}" >/dev/null 2>&1; then
  OPENSHELL_PREINSTALLED=1
else
  OPENSHELL_PREINSTALLED=0
  if [[ "${E2E_OPENSHELL_INSTALL}" == "never" ]]; then
    red "ERROR: openshell CLI not found (OPENSHELL_BIN=${OPENSHELL_BIN}) and E2E_OPENSHELL_INSTALL=never"
    red "Install it manually, or set E2E_OPENSHELL_INSTALL=auto to install the"
    red "gateway-matched version via the console-recommended command."
    exit 1
  fi
fi

# --- Cleanup trap ---

# dump_provision_diagnostics prints controller and stand-in postgres logs
# before restore_namespace_gc_timing restarts the controller. Kind e2e
# failures in DatabaseReady otherwise lose the SQL/TLS error: GC-timing
# restore rolls a new ReplicaSet, and CI then collects the replacement pod.
dump_provision_diagnostics() {
  dim "  --- control-plane logs (before GC-timing restore) ---"
  $CLI logs -n "${E2E_HS_NAMESPACE}" -l app=hypershell-controller --all-containers --tail=200 2>&1 | while IFS= read -r line; do dim "    $line"; done || true
  dim "  --- stand-in postgres logs ---"
  $CLI logs -n external-cloud-db -l app=postgres --tail=100 2>&1 | while IFS= read -r line; do dim "    $line"; done || true
  $CLI logs -n "${E2E_HS_NAMESPACE}" -l app=hypershell-postgres --tail=100 2>&1 | while IFS= read -r line; do dim "    $line"; done || true
}

cleanup() {
  local exit_code=$?
  if [[ "$exit_code" -ne 0 ]]; then
    dump_provision_diagnostics || true
  fi
  # A failed OpenShift run is about to tear the environment down (CI) or has
  # already left the controller in a bad state. Waiting on a restore rollout
  # (up to 300s) delays that teardown for no effect. Kind keeps the cluster,
  # so it still restores on every exit.
  if [[ "${E2E_INFRA_DRIVER}" == "openshift" && "${exit_code}" -ne 0 ]]; then
    dim "  Skipping namespace GC timing restore; moving to teardown"
  else
    restore_namespace_gc_timing || true
  fi
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
  if [[ -n "$GATEWAY_SA_DIR" ]]; then
    if [[ -f "${GATEWAY_SA_DIR}/credential.json" ]]; then
      if ! acquire_oidc_token || ! gateway_service_account delete; then
        fail_test "Could not clean up the gateway service account"
        exit_code=1
      fi
    fi
    rm -rf "$GATEWAY_SA_DIR"
    if [[ -n "${GW_CONFIG_DIR:-}" ]]; then
      rm -f "${GW_CONFIG_DIR}/oidc_token.json"
    fi
  fi
  # perf mode never deletes the supplied/reused canary gateway: checkpoints and
  # canary runs must leave it standing. E2E_SKIP_CLEANUP also preserves it.
  if [[ "$E2E_MODE" != "perf" && "$E2E_SKIP_CLEANUP" != "1" && -n "$GW_ID" ]]; then
    dim "  Cleaning up gateway ${GW_NAME}..."
    # JWT is enforced, so the DELETE needs a bearer token. The token acquired
    # earlier may have expired during provisioning, so refresh best-effort before
    # deleting; cleanup is non-fatal, so ignore failures.
    acquire_oidc_token 2>/dev/null || true
    api_curl -X DELETE "${API_HOST}/api/hypershell/v1/gateways/${GW_ID}" &>/dev/null || true
  fi
  # Extended areas (12-13) create their own gateways; delete any that survived.
  if [[ "$E2E_SKIP_CLEANUP" != "1" && "${#E2E_EXTRA_GW_IDS[@]}" -gt 0 ]]; then
    acquire_oidc_token 2>/dev/null || true
    local _xid
    for _xid in "${E2E_EXTRA_GW_IDS[@]}"; do
      [[ -n "$_xid" ]] || continue
      api_curl -X DELETE "${API_HOST}/api/hypershell/v1/gateways/${_xid}" &>/dev/null || true
    done
  fi
  # Stop the kind driver's loopback gateway forwarder, if one was started.
  if [[ -n "${_KINDCCM_SOCAT_PID:-}" ]]; then
    kill "${_KINDCCM_SOCAT_PID}" 2>/dev/null || true
    wait "${_KINDCCM_SOCAT_PID}" 2>/dev/null || true
  fi
  if declare -F de_seed_test_users >/dev/null 2>&1; then
    de_seed_test_users || true
  fi
  # Runs on every exit path -- a fatal exit 1 mid-run included -- so the
  # summary always prints, and print_results itself notes when E2E_COMPLETED
  # was never set (i.e. the run aborted before reaching the results section).
  print_results
  if [[ "$exit_code" -ne 0 ]]; then
    exit "$exit_code"
  fi
}
trap cleanup EXIT

# --- Namespace GC timing ---
# Long mode seeds a synthetic orphan namespace later and waits for the periodic
# reaper to collect it (see area 11a); on drivers whose deployment runs with
# production GC defaults, that wait can't complete in time unless shortened
# first. Done once up front, before any gateway is created, so the controller
# restart this can trigger doesn't land mid-reconciliation.
if e2e_step long; then
  if ! configure_namespace_gc_timing; then
    red "ERROR: Could not configure namespace GC timing for the e2e run"
    exit 1
  fi
fi

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
printf '  %s\n' "9b. Gateway access management + directory pickup of a new Keycloak user [kind, long]"
printf '  %s\n' "10. Platform admin RBAC verification"
printf '  %s\n' "11. Gateway deletion + namespace garbage collection"
printf '  %s\n' "12. ManagedCluster registration + control-plane identity [long]"
printf '  %s\n' "13. Gateway release promotion + reconciled status [skipped - GatewayRelease removed]"
printf '  %s\n' "14. Admin inventory + API validation [long]"
echo ""
dim  "  Driver:            ${E2E_INFRA_DRIVER}"
dim  "  Mode:              ${E2E_MODE}"
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
e2e_area "1. Infrastructure Validation + OIDC Verification"
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
show_cmd "curl -s -o /dev/null -w '%{http_code}' ${API_HOST}/api/hypershell/v1/gateways (driver TLS policy, no auth)"
UNAUTH_STATUS=$(_driver_curl -o /dev/null -w '%{http_code}' "${API_HOST}/api/hypershell/v1/gateways" 2>/dev/null || true)
if [[ "$UNAUTH_STATUS" == "401" ]]; then
  pass "API server rejects unauthenticated requests (401)"
else
  fail_test "Expected 401 for unauthenticated request, got ${UNAUTH_STATUS}"
fi

# Verify: authenticated API requests return 200
show_cmd "curl -s -H 'Authorization: Bearer ...' ${API_HOST}/api/hypershell/v1/gateways (driver TLS policy)"
AUTH_STATUS=$(api_curl -o /dev/null -w '%{http_code}' "${API_HOST}/api/hypershell/v1/gateways" 2>/dev/null || true)
if [[ "$AUTH_STATUS" == "200" ]]; then
  pass "API server accepts authenticated requests (200)"
else
  fail_test "Expected 200 for authenticated request, got ${AUTH_STATUS}"
fi

# Verify: BFF /auth/session returns unauthenticated
if ! discover_console_host; then
  fail_test "Could not discover HyperShell web console host"
  exit 1
fi
CONSOLE_HOST="${_DISCOVER_CONSOLE_HOST}"
show_cmd "curl -s ${CONSOLE_HOST}/auth/session (driver TLS policy)"
SESSION_RESP=$(_driver_curl "${CONSOLE_HOST}/auth/session" 2>/dev/null || true)
SESSION_AUTH=$(echo "${SESSION_RESP}" | python3 -c "import json,sys; print(json.load(sys.stdin).get('authenticated',''))" 2>/dev/null || true)
if [[ "$SESSION_AUTH" == "False" ]]; then
  pass "BFF /auth/session returns authenticated: false"
else
  fail_test "Expected authenticated: false from /auth/session, got: ${SESSION_RESP:0:100}"
fi

# Verify: BFF /auth/login redirects to Keycloak with PKCE
show_cmd "curl -s -o /dev/null -w '%{redirect_url}' ${CONSOLE_HOST}/auth/login (driver TLS policy)"
LOGIN_REDIRECT=$(_driver_curl -o /dev/null -w '%{redirect_url}' "${CONSOLE_HOST}/auth/login" 2>/dev/null || true)
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
e2e_area "2. Gateway Provisioning via HyperShell API"
echo ""

# JWT enforcement means every gateway CRUD call below needs a bearer token.
# Refresh the token here so the full Keycloak access-token lifetime covers the
# create plus the provisioning poll (up to E2E_PROVISION_TIMEOUT seconds).
if ! acquire_oidc_token; then
  fail_test "Could not acquire OIDC token for gateway provisioning"
  exit 1
fi

show_cmd "api_curl ${API_HOST}/api/hypershell/v1/gateways?search=name%3D%27${GW_NAME}%27"
EXISTING_GW=$(api_curl "${API_HOST}/api/hypershell/v1/gateways?search=name%3D%27${GW_NAME}%27" 2>/dev/null || true)
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
  e2e_apply_seed_ids_from_gateway_json "$EXISTING_GW" "$GW_NAME"
else
  if ! e2e_ensure_seed_ids; then
    fail_test "Could not discover seeded cluster id"
    exit 1
  fi
  dim "  Creating gateway with server-side placement; the gateway database is provisioned by the control plane"

  show_cmd "api_curl -X POST ${API_HOST}/api/hypershell/v1/gateways -d '{name: ${GW_NAME}, oidc: ...}'"
  GW_CREATE_BODY=$(e2e_gateway_create_body "$GW_NAME")
  CREATE_RESPONSE=$(api_curl -X POST "${API_HOST}/api/hypershell/v1/gateways" \
    -H "Content-Type: application/json" \
    -d "${GW_CREATE_BODY}" 2>/dev/null || true)

  # A successful create returns kind="Gateway" with a ksuid id. A failure returns
  # kind="Error" with a numeric id (e.g. "9"=ErrorGeneral/500, "17"=malformed/400).
  # Detect the error case explicitly: otherwise the error object's id is mistaken
  # for a gateway id and the provisioning poll spins on a nonexistent gateway until
  # timeout, masking the real api-server failure.
  IFS=$'\t' read -r CREATE_KIND CREATE_F1 CREATE_F2 <<< "$(echo "$CREATE_RESPONSE" | python3 -c "
import json, sys
try:
    d = json.load(sys.stdin)
except Exception:
    print('PARSE\t\t'); sys.exit(0)
if d.get('kind') == 'Error':
    print('ERROR\t%s\t%s' % (d.get('code', ''), d.get('reason', ''))); sys.exit(0)
print('OK\t%s\t%s' % (d.get('id', ''), d.get('namespace', '')))
" 2>/dev/null)" || true

  if [[ "$CREATE_KIND" == "OK" && -n "$CREATE_F1" ]]; then
    GW_ID="$CREATE_F1"
    GW_NAMESPACE="$CREATE_F2"
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
  GW_CONDITIONS_SUMMARY=""
  while [[ $(date +%s) -lt $DEADLINE ]]; do
    # Refresh the OIDC token each poll: provisioning can outlast the access
    # token lifetime, and api_curl reads _OIDC_ACCESS_TOKEN on every call.
    acquire_oidc_token 2>/dev/null || true
    GW_JSON=$(api_curl "${API_HOST}/api/hypershell/v1/gateways/${GW_ID}" 2>/dev/null || true)
    GW_PHASE=$(echo "$GW_JSON" | \
      python3 -c "import json,sys; print(json.load(sys.stdin).get('phase',''))" 2>/dev/null || true)
    GW_CONDITIONS_SUMMARY=$(echo "$GW_JSON" | python3 -c "
import json, sys
try:
    gw = json.load(sys.stdin)
except Exception:
    sys.exit(0)
conditions = gw.get('provisioning_conditions', [])
if not conditions:
    sys.exit(0)
parts = []
for c in conditions:
    parts.append('%s=%s' % (c.get('type','?'), c.get('condition_status','?')))
print(', '.join(parts))
" 2>/dev/null || true)
    if [[ "$GW_PHASE" == "Running" ]]; then
      break
    fi
    if [[ -n "$GW_CONDITIONS_SUMMARY" ]]; then
      dim "    phase: ${GW_PHASE:-unknown}  conditions: [${GW_CONDITIONS_SUMMARY}]"
    else
      dim "    phase: ${GW_PHASE:-unknown}"
    fi
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

  # Verify the control plane published route_address to the API after provisioning.
  # A missing route_address means the web console cannot display connection
  # instructions and the CLI cannot discover the gateway endpoint from the API.
  show_cmd "api_curl ${API_HOST}/api/hypershell/v1/gateways/${GW_ID} | .route_address"
  GW_ROUTE_ADDRESS=$(api_curl "${API_HOST}/api/hypershell/v1/gateways/${GW_ID}" 2>/dev/null | \
    python3 -c "import json,sys; print(json.load(sys.stdin).get('route_address',''))" 2>/dev/null || true)
  if [[ -n "$GW_ROUTE_ADDRESS" ]]; then
    pass "Gateway route_address published: ${GW_ROUTE_ADDRESS}"
  else
    fail_test "Gateway route_address is empty after provisioning (control plane did not publish it)"
  fi
fi

# ── 2b. provisioning conditions validation ─────────────────────────────────
# After the gateway reaches Running, verify that the API exposes provisioning
# conditions and that every condition completed successfully.

show_cmd "api_curl ${API_HOST}/api/hypershell/v1/gateways/${GW_ID}  # verify provisioning_conditions"
# A re-reconcile (watch resync, controller reconnect) re-initializes the
# conditions to Pending and walks them again while the gateway is already
# Running, so a single read can observe InProgress/Pending that settles
# seconds later. Retry within a bounded window; only a settled incomplete
# state is a failure.
GW_COND_DEADLINE=$(($(date +%s) + ${E2E_CONDITIONS_SETTLE_TIMEOUT:-60}))
while true; do
  GW_COND_JSON=$(api_curl "${API_HOST}/api/hypershell/v1/gateways/${GW_ID}" 2>/dev/null || true)
  GW_COND_CHECK=$(echo "$GW_COND_JSON" | python3 -c "
import json, sys
try:
    gw = json.load(sys.stdin)
except Exception:
    print('PARSE_ERROR'); sys.exit(0)
conditions = gw.get('provisioning_conditions')
if conditions is None or not isinstance(conditions, list):
    print('MISSING'); sys.exit(0)
if len(conditions) == 0:
    print('EMPTY'); sys.exit(0)
types = []
incomplete = []
for c in conditions:
    ct = c.get('type', '?')
    cs = c.get('condition_status', '?')
    types.append(ct)
    if cs != 'Complete':
        incomplete.append('%s=%s' % (ct, cs))
# Verify required condition types are present
required = {'EnvironmentReady', 'DatabaseReady', 'GatewayDeployed', 'GatewayHealthy'}
present = set(types)
missing = required - present
if missing:
    print('MISSING_TYPES:%s' % ','.join(sorted(missing))); sys.exit(0)
if incomplete:
    print('INCOMPLETE:%s' % '; '.join(incomplete)); sys.exit(0)
print('OK:%d' % len(conditions))
" 2>/dev/null || echo "SCRIPT_ERROR")
  case "$GW_COND_CHECK" in
    OK:*) break ;;
    INCOMPLETE:*)
      if [[ $(date +%s) -lt $GW_COND_DEADLINE ]]; then
        dim "    conditions not yet settled (${GW_COND_CHECK#INCOMPLETE:}); retrying"
        sleep 5
        continue
      fi
      ;;
  esac
  break
done

case "$GW_COND_CHECK" in
  OK:*)
    COND_COUNT="${GW_COND_CHECK#OK:}"
    pass "Provisioning conditions present (${COND_COUNT} steps, all Complete)"
    ;;
  MISSING)
    fail_test "Gateway is Running but provisioning_conditions field is missing from API response"
    ;;
  EMPTY)
    fail_test "Gateway is Running but provisioning_conditions is an empty array"
    ;;
  MISSING_TYPES:*)
    MISSING_TYPES="${GW_COND_CHECK#MISSING_TYPES:}"
    fail_test "Provisioning conditions missing required types: ${MISSING_TYPES}"
    ;;
  INCOMPLETE:*)
    INCOMPLETE_INFO="${GW_COND_CHECK#INCOMPLETE:}"
    fail_test "Gateway is Running but not all provisioning conditions are Complete: ${INCOMPLETE_INFO}"
    ;;
  *)
    fail_test "Could not parse provisioning conditions from API response"
    dim "    raw check result: ${GW_COND_CHECK}"
    ;;
esac

if [[ -z "$GW_NAMESPACE" ]]; then
  fail_test "Gateway response did not include a server-assigned namespace"
  exit 1
fi
dim "  Gateway namespace: ${GW_NAMESPACE}"

# Seed a synthetic orphaned managed namespace for periodic GC. Created here so
# steps 3–10 run while the reaper sweeps; step 11 only validates (no extra wait
# if the reaper already ran during the suite). Long-only: the quick checks
# (short/perf) do not exercise the periodic reaper.
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
    hypershell.redhat.io/instance: "${E2E_HS_NAMESPACE}"
  annotations:
    hypershell.redhat.io/gc-eligible-since: "${ORPHAN_ELIGIBLE_SINCE}"
EOF
  ORPHAN_GC_DEADLINE=$(($(date +%s) + E2E_ORPHAN_GC_TIMEOUT))
fi

# Per-gateway Keycloak client id. The control-plane reconciler creates a
# dedicated public client named "${gw.Name}-${gatewayID}" with an audience
# mapper and overrides the gateway's OIDC config to require aud == this
# client, on every infra target. Gateway and CLI tokens must therefore be
# minted against this client, not the shared frontend client, or Envoy
# rejects them with InvalidAudience. gatewayID is the API resource id (GW_ID).
GW_KC_CLIENT_ID="${GW_NAME}-${GW_ID}"
dim "  Per-gateway OIDC client: ${GW_KC_CLIENT_ID}"
sep

# ── 3. gateway infrastructure ──────────────────────────────────────────────

echo ""
e2e_area "3. Gateway Infrastructure"
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

# The gateway database itself lives on the PostgreSQL server named by the
# controller's hypershell-gateway-database-admin Secret; the tenant credentials
# Secret is what proves it was provisioned. The gateway workload is deployed by
# the upstream OpenShell Helm chart, which reads only this Secret's uri key and
# cannot mount an extra CA file for the database connection, so the tenant
# connection is capped at sslmode=require (encrypted, not certificate-verified)
# and the Secret carries no sslrootcert. The control plane's own admin
# connection is unaffected and stays verify-full. See
# specs/platform/openshell-gateway-database.spec.md.
show_cmd "$CLI get secret openshell-gateway-db-credentials -n $GW_NAMESPACE"
if $CLI get secret openshell-gateway-db-credentials -n "$GW_NAMESPACE" &>/dev/null; then
  pass "Database credentials secret exists in gateway namespace"
  DB_SSLMODE=$($CLI get secret openshell-gateway-db-credentials -n "$GW_NAMESPACE" \
    -o jsonpath='{.data.sslmode}' 2>/dev/null | base64 -d 2>/dev/null || true)
  DB_URI=$($CLI get secret openshell-gateway-db-credentials -n "$GW_NAMESPACE" \
    -o jsonpath='{.data.uri}' 2>/dev/null | base64 -d 2>/dev/null || true)
  DB_SSLROOTCERT=$($CLI get secret openshell-gateway-db-credentials -n "$GW_NAMESPACE" \
    -o jsonpath='{.data.sslrootcert}' 2>/dev/null || true)
  if [[ "$DB_SSLMODE" == "require" ]]; then
    pass "Database credentials pin sslmode=require"
  else
    fail_test "Database credentials sslmode is '${DB_SSLMODE:-<empty>}', expected require"
  fi
  if [[ "$DB_URI" == *"sslmode=require"* ]]; then
    pass "Database credentials uri requests TLS (sslmode=require)"
  else
    fail_test "Database credentials uri does not carry sslmode=require"
  fi
  if [[ -z "$DB_SSLROOTCERT" ]]; then
    pass "Database credentials carry no sslrootcert (chart cannot mount a DB CA)"
  else
    fail_test "Database credentials unexpectedly carry an sslrootcert key"
  fi
else
  fail_test "Database credentials secret not found in gateway namespace"
fi

show_cmd "$CLI get configmap openshell-gateway-config -n $GW_NAMESPACE"
if $CLI get configmap openshell-gateway-config -n "$GW_NAMESPACE" &>/dev/null; then
  pass "Gateway config ConfigMap exists"
else
  fail_test "Gateway config ConfigMap not found"
fi

show_cmd "$CLI get certificate openshell-gateway-ca -n $GW_NAMESPACE"
GW_CA_READY=$($CLI get certificate openshell-gateway-ca -n "$GW_NAMESPACE" -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null || echo "")
if [[ "$GW_CA_READY" == "True" ]]; then
  pass "Gateway CA certificate issued"
else
  fail_test "Gateway CA certificate not ready (status=${GW_CA_READY:-unknown})"
fi

show_cmd "$CLI get certificate openshell-gateway-server -n $GW_NAMESPACE"
GW_SRV_READY=$($CLI get certificate openshell-gateway-server -n "$GW_NAMESPACE" -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null || echo "")
if [[ "$GW_SRV_READY" == "True" ]]; then
  pass "Gateway server certificate issued"
else
  fail_test "Gateway server certificate not ready (status=${GW_SRV_READY:-unknown})"
fi

# The control plane no longer creates NetworkPolicies for gateway namespaces
# (see openshell-gateway-helm-adoption.spec.md). On OVN-Kubernetes, gateway pods
# use the default allow-all posture; imperative policies caused a cascading
# deny-by-default problem that broke sandbox connectivity.
show_cmd "$CLI get networkpolicy -n $GW_NAMESPACE"
MANAGED_NP_COUNT=$($CLI get networkpolicy -n "$GW_NAMESPACE" -l hypershell.redhat.io/managed=true --no-headers 2>/dev/null | wc -l | tr -d ' ')
if [[ "${MANAGED_NP_COUNT:-0}" -eq 0 ]]; then
  pass "No managed gateway NetworkPolicies (expected per helm-adoption spec)"
else
  fail_test "Found ${MANAGED_NP_COUNT} managed NetworkPolicies in gateway namespace (control plane should not create any)"
fi
fi
sep

# ── 4. OIDC token acquisition + CA certificate setup ─────────────────────

echo ""
e2e_area "4. OIDC Token Acquisition + CA Certificate Setup"
echo ""

# The identity flow uses the per-gateway client. Machine mode below replaces
# the requesting client with an API-created service account; its token still
# has the same per-gateway audience and the gateway's assigned roles.
OIDC_CLIENT_ID_EFFECTIVE="${GW_KC_CLIENT_ID}"

if [[ "${E2E_INFRA_DRIVER}" == "kind" ]] && e2e_step long; then
  # Exercise the real Keycloak device authorization endpoint for the client
  # provisioned by the control plane. A successful authorization response proves
  # that oauth2.device.authorization.grant.enabled reached Keycloak; polling once
  # after the advertised interval proves that Keycloak recognizes the device code.
  DEVICE_DISCOVERY=$(_driver_curl "${E2E_OIDC_ISSUER}/.well-known/openid-configuration" 2>/dev/null || true)
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
  DEVICE_AUTH_RESPONSE=$(_driver_curl -X POST "$DEVICE_AUTH_ENDPOINT" \
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

  DEVICE_TOKEN_RESPONSE=$(_driver_curl -X POST "${E2E_OIDC_ISSUER}/protocol/openid-connect/token" \
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

if [[ "$E2E_GATEWAY_AUTH" == "service_account" ]]; then
  show_cmd "# create a gateway-scoped service account through the HyperShell API"
  GATEWAY_SA_DIR=$(mktemp -d)
  if ! acquire_oidc_token || ! gateway_service_account create; then
    fail_test "Failed to create a gateway-scoped service account"
    exit 1
  fi
  OIDC_CLIENT_ID_EFFECTIVE=$(gateway_service_account client-id)
  # Copy the private token file during CLI registration; never emit it through
  # helper stdout or put this machine token in the shell environment.
  OIDC_TOKEN=""
  pass "Gateway service-account token acquired (single gateway audience, openshell-admin + openshell-user)"
else
  if [[ "${E2E_OIDC_GRANT:-password}" == "client_credentials" ]]; then
    show_cmd "# token-exchange (hypershell-e2e) → ${E2E_OIDC_ISSUER} (audience: ${GW_KC_CLIENT_ID}, await role: openshell-admin)"
  else
    show_cmd "# resource-owner password grant → ${E2E_OIDC_ISSUER} (client: ${GW_KC_CLIENT_ID}, await role: openshell-admin)"
  fi
  if acquire_gateway_token_with_role "$E2E_OIDC_USERNAME" "$E2E_OIDC_PASSWORD" "$GW_KC_CLIENT_ID" openshell-admin; then
    OIDC_TOKEN="${_OIDC_ACCESS_TOKEN}"
    pass "OIDC token acquired with openshell-admin (user: ${E2E_OIDC_USERNAME}, client: ${GW_KC_CLIENT_ID})"
  else
    fail_test "Failed to acquire per-gateway OIDC token with openshell-admin role"
    exit 1
  fi
fi


if [[ "${E2E_INFRA_DRIVER}" == "kind" ]]; then
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
else
  pass "Gateway TLS trust configured by the OpenShift driver"
fi
sep

# ── 5. route discovery + CLI registration ─────────────────────────────────

echo ""
e2e_area "5. Route Discovery + CLI Registration"
echo ""

# _install_openshell_cli_container_wrapper - on a non-Linux host running the
# kind driver, no native CLI build exists for a downstream-tagged image (see
# _extract_openshell_cli_from_image below). Instead of a binary, install a
# wrapper script that execs the exact CLI image as a container on Kind's own
# "kind" podman network -- the same trick scripts/kind/openshell.sh uses
# interactively -- bind-mounting the real ~/.config/openshell so the CLI sees
# exactly the gateways this run registers under OPENSHELL_BIN. Installed
# inside the repo (tests/e2e/.cache/bin), not ~/.local/bin, so it can never be
# mistaken for a real openshell install outside this checkout, and so a
# native Linux CI run is never at risk of picking it up.
_install_openshell_cli_container_wrapper() {
  local image="$1"
  local install_dir="${SCRIPT_DIR}/.cache/bin"
  mkdir -p "${install_dir}"
  dim "  Host OS is $(uname -s); installing a container-wrapper CLI (${image}) at ${install_dir}/openshell"
  cat >"${install_dir}/openshell" <<WRAPPER_EOF
#!/usr/bin/env bash
# Generated by tests/e2e/e2e-openshell.sh -- do not edit by hand.
# Runs the exact-matching openshell CLI image as a container on Kind's own
# "kind" podman network, since no native build exists for this host OS at
# this (possibly downstream-suffixed) tag.
set -euo pipefail
CLI_IMAGE=$(printf '%q' "${image}")
CONTAINER_ENGINE=$(printf '%q' "${CONTAINER_ENGINE}")
GW_NAMESPACE=$(printf '%q' "${GW_NAMESPACE}")
CONFIG_DIR="\${HOME}/.config/openshell"

# The Gateway's own address is looked up fresh on every invocation (cheap
# kubectl reads) rather than baked in at generation time: this wrapper is
# generated before the gateway route/address are discovered.
GW_REF_NAME=\$(kubectl get grpcroute openshell-gateway -n "\${GW_NAMESPACE}" \\
  -o jsonpath='{.spec.parentRefs[0].name}' 2>/dev/null || true)
GW_REF_NS=\$(kubectl get grpcroute openshell-gateway -n "\${GW_NAMESPACE}" \\
  -o jsonpath='{.spec.parentRefs[0].namespace}' 2>/dev/null || true)
GW_ADDR=""
if [[ -n "\${GW_REF_NAME}" && -n "\${GW_REF_NS}" ]]; then
  GW_ADDR=\$(kubectl get gateway "\${GW_REF_NAME}" -n "\${GW_REF_NS}" \\
    -o jsonpath='{.status.addresses[0].value}' 2>/dev/null || true)
fi

ADD_HOSTS=()
if [[ -n "\${GW_ADDR}" && -d "\${CONFIG_DIR}/gateways" ]]; then
  while IFS= read -r host; do
    [[ -n "\${host}" ]] && ADD_HOSTS+=(--add-host "\${host}:\${GW_ADDR}")
  done < <(python3 -c "
import json, glob, urllib.parse
hosts = set()
for path in glob.glob('\${CONFIG_DIR}/gateways/*/metadata.json'):
    try:
        with open(path) as f:
            meta = json.load(f)
    except Exception:
        continue
    for key in ('gateway_endpoint', 'oidc_issuer'):
        h = urllib.parse.urlparse(meta.get(key, '') or '').hostname
        if h:
            hosts.add(h)
print('\n'.join(sorted(hosts)))
" 2>/dev/null)
fi

TTY_FLAGS=()
if [[ -t 0 && -t 1 ]]; then
  TTY_FLAGS=(-it)
fi

exec "\${CONTAINER_ENGINE}" run --rm \${TTY_FLAGS[@]+"\${TTY_FLAGS[@]}"} \\
  --network kind \\
  \${ADD_HOSTS[@]+"\${ADD_HOSTS[@]}"} \\
  -v "\${CONFIG_DIR}:/home/cli/.config/openshell:Z" \\
  -e HOME=/home/cli \\
  -e OPENSHELL_GATEWAY_INSECURE=true \\
  "\${CLI_IMAGE}" "\$@"
WRAPPER_EOF
  chmod 755 "${install_dir}/openshell"
  export PATH="${install_dir}:${PATH}"
  hash -r 2>/dev/null || true
  OPENSHELL_CLI_CONTAINERIZED=1
}

# _extract_openshell_cli_from_image - pull an image and copy /usr/local/bin/openshell
# out of it onto PATH. Used both for an explicit E2E_OPENSHELL_CLI_IMAGE override and
# for the default path below, which resolves an image matching the deployed gateway.
# A container image's binary is always built for Linux; extracting it and running it
# directly only works when the host OS is Linux too, whatever the CPU architecture --
# otherwise it fails with a confusing "exec format error" instead of a clear one. On
# the kind driver with podman available, fall back to running the CLI in a container
# instead of failing outright (see _install_openshell_cli_container_wrapper).
_extract_openshell_cli_from_image() {
  local image="$1"
  if [[ "$(uname -s)" != "Linux" ]]; then
    if [[ "${E2E_INFRA_DRIVER}" == "kind" && "$(basename "${CONTAINER_ENGINE:-}")" == "podman" ]]; then
      _install_openshell_cli_container_wrapper "${image}"
      return
    fi
    fail_test "${image} is a Linux-only container image; there is no native openshell build for $(uname -s) at this tag. Run this on a Linux host, install podman so this driver can run the CLI in a container on Kind's network instead, or build the CLI from source at that exact commit."
    exit 1
  fi
  dim "  Extracting CLI from container image: ${image}"
  local install_dir="${HOME}/.local/bin"
  mkdir -p "${install_dir}"
  local ctr_engine
  ctr_engine="${CONTAINER_ENGINE:-$(command -v podman 2>/dev/null || command -v docker 2>/dev/null || true)}"
  if ! e2e_extract_cli_image "${image}" "${install_dir}" "${ctr_engine}"; then
    fail_test "Failed to extract openshell binary from ${image}"
    exit 1
  fi
  export PATH="${install_dir}:${PATH}"
  hash -r 2>/dev/null || true
}

# _reconcile_gateway_version_or_empty - poll the API for GW_ID's gateway_version
# for up to E2E_GATEWAY_VERSION_TIMEOUT seconds. Echoes the raw version string, or
# nothing on timeout; never fails the test itself, so callers decide whether an
# empty result is fatal.
_reconcile_gateway_version_or_empty() {
  local raw_version="" deadline
  deadline=$(($(date +%s) + E2E_GATEWAY_VERSION_TIMEOUT))
  dim "  Waiting for reconciled gateway_version (timeout: ${E2E_GATEWAY_VERSION_TIMEOUT}s)..."
  while [[ $(date +%s) -lt $deadline ]]; do
    # Provisioning can outlast the access token; api_curl reads it each call.
    acquire_oidc_token 2>/dev/null || true
    raw_version=$(api_curl "${API_HOST}/api/hypershell/v1/gateways/${GW_ID}" 2>/dev/null | \
      python3 -c "import json,sys; print(json.load(sys.stdin).get('gateway_version') or '')" 2>/dev/null || true)
    [[ -n "$raw_version" ]] && break
    sleep 5
  done
  printf '%s' "$raw_version"
}

# Read the runtime version and install a matching CLI.
install_openshell_cli_from_api() {
  if [[ "${E2E_OPENSHELL_INSTALL}" == "never" ]]; then
    dim "  E2E_OPENSHELL_INSTALL=never; using pre-installed openshell CLI."
    return 0
  fi

  local raw_version=""

  if [[ "${E2E_OPENSHELL_INSTALL}" != "always" && -z "${E2E_OPENSHELL_VERSION}" && "${OPENSHELL_PREINSTALLED}" == "1" ]]; then
    # A CLI already on PATH is only trustworthy if it matches the deployed
    # gateway: the gateway/supervisor images can carry a downstream suffix
    # (e.g. "v0.0.116-rhaiv.6") that is proto-incompatible with whatever
    # unrelated openshell build happens to already be installed.
    raw_version=$(_reconcile_gateway_version_or_empty)
    local reported
    reported=$("${OPENSHELL_BIN}" --version 2>&1 || true)
    local wanted_tag=""
    if [[ -n "$raw_version" ]]; then
      wanted_tag=$(openshell_cli_image_tag "$raw_version" || true)
    fi
    if [[ -n "$wanted_tag" ]] && openshell_cli_matches_version "$reported" "$wanted_tag"; then
      pass "Pre-installed openshell CLI matches the gateway (${reported})"
      return 0
    fi
    if [[ -z "$raw_version" ]]; then
      dim "  Could not confirm the gateway's version; trusting the pre-installed openshell CLI (${reported}) as-is."
      return 0
    fi
    dim "  Pre-installed openshell CLI (${reported}) does not match gateway_version '${raw_version}'; installing a matching CLI instead."
  fi

  # Explicit container image override.
  if [[ -n "${E2E_OPENSHELL_CLI_IMAGE}" ]]; then
    _extract_openshell_cli_from_image "${E2E_OPENSHELL_CLI_IMAGE}"
    local reported
    reported=$("${OPENSHELL_BIN}" --version 2>&1 || true)
    dim "  openshell --version: ${reported}"
    pass "openshell CLI extracted from container image (${reported})"
    return 0
  fi

  if [[ -n "${E2E_OPENSHELL_VERSION}" ]]; then
    # Explicit version override: install a stable public release by tag, the
    # one case where a plain upstream OpenShell CLI release is what's wanted
    # (e.g. testing against a specific NVIDIA/OpenShell release directly,
    # independent of whatever the deployed gateway happens to run).
    local installer_version="${E2E_OPENSHELL_VERSION}"
    dim "  Using E2E_OPENSHELL_VERSION override: ${installer_version}"

    local target
    case "$(uname -s)/$(uname -m)" in
      Linux/x86_64)            target=x86_64-unknown-linux-musl ;;
      Linux/aarch64|Linux/arm64) target=aarch64-unknown-linux-musl ;;
      Darwin/arm64)            target=aarch64-apple-darwin ;;
      *) fail_test "Unsupported platform: $(uname -s)/$(uname -m)"; exit 1 ;;
    esac

    # Try the install script first (validates checksums, works for stable releases).
    # Fall back to a direct GitHub release download for non-semver tags (e.g. dev).
    local install_dir="${HOME}/.local/bin"
    if printf '%s\n' "${installer_version}" | LC_ALL=C grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+$'; then
      show_cmd "curl -LsSf ${OPENSHELL_INSTALL_SCRIPT_URL} | OPENSHELL_VERSION=${installer_version} sh"
      if ! curl -LsSf "${OPENSHELL_INSTALL_SCRIPT_URL}" | OPENSHELL_VERSION="${installer_version}" sh; then
        fail_test "openshell install.sh failed for OPENSHELL_VERSION=${installer_version} (recommended command broken)"
        exit 1
      fi
    else
      local asset="openshell-${target}.tar.gz"
      local release_url="https://github.com/NVIDIA/OpenShell/releases/download/${installer_version}"
      show_cmd "curl -fLsS ${release_url}/${asset} | tar -xz -C ${install_dir}"
      mkdir -p "${install_dir}"
      if ! curl --proto '=https' --tlsv1.2 -fLsS --retry 3 "${release_url}/${asset}" | tar -xz -C "${install_dir}" openshell; then
        fail_test "Direct CLI download failed for ${installer_version} (asset: ${asset})"
        exit 1
      fi
      chmod 755 "${install_dir}/openshell"
    fi
    export PATH="${install_dir}:${PATH}"
    hash -r 2>/dev/null || true
    if ! command -v "${OPENSHELL_BIN}" >/dev/null 2>&1; then
      fail_test "openshell CLI not on PATH after install (OPENSHELL_BIN=${OPENSHELL_BIN})"
      exit 1
    fi
    local reported
    reported=$("${OPENSHELL_BIN}" --version 2>&1 || true)
    dim "  openshell --version: ${reported}"
    pass "openshell CLI installed (${reported})"
    return 0
  fi

  # Default: install the CLI build that matches the deployed gateway exactly.
  # The gateway/supervisor images are quay.io/opendatahub/odh-openshell-{gateway,supervisor}
  # and can carry a downstream suffix (e.g. "v0.0.116-rhaiv.6") identifying a build
  # ahead of the last tagged upstream OpenShell release; no public CLI release is
  # guaranteed proto-compatible with it. quay.io/opendatahub/odh-openshell-cli is
  # built by the same pipeline off the same commit under the same tag, so pulling it
  # by that exact tag (never truncated to a base semver) is what actually matches.
  # (The preinstalled-CLI check above may already have fetched this.)
  if [[ -z "$raw_version" ]]; then
    raw_version=$(_reconcile_gateway_version_or_empty)
  fi
  if [[ -z "$raw_version" ]]; then
    fail_test "API never reported gateway_version for ${GW_ID} within ${E2E_GATEWAY_VERSION_TIMEOUT}s"
    exit 1
  fi
  pass "Reconciled gateway_version: ${raw_version}"

  local image_tag
  if ! image_tag=$(openshell_cli_image_tag "$raw_version"); then
    fail_test "Could not derive a CLI image tag from gateway_version='${raw_version}'"
    exit 1
  fi
  local cli_image="quay.io/opendatahub/odh-openshell-cli:${image_tag}"
  dim "  Resolved matching CLI image: ${cli_image} (from '${raw_version}')"

  _extract_openshell_cli_from_image "${cli_image}"

  local reported
  reported=$("${OPENSHELL_BIN}" --version 2>&1 || true)
  dim "  openshell --version: ${reported}"
  if ! openshell_cli_matches_version "$reported" "$image_tag"; then
    fail_test "Installed openshell version '${reported}' does not match requested ${image_tag}"
    exit 1
  fi
  pass "openshell CLI installed (${reported})"
}

install_openshell_cli_from_api

GW_LOCAL_NAME="${GW_NAMESPACE}-openshell"

if wait_for_gateway_route "$GW_NAME" "$GW_NAMESPACE"; then
  pass "Gateway route is ready"
else
  fail_test "Gateway route not ready after timeout"
  exit 1
fi

discover_gateway_endpoint "$GW_NAME" "$GW_NAMESPACE"
GW_ENDPOINT="${_DISCOVER_GW_ENDPOINT}"
if [[ "${OPENSHELL_CLI_CONTAINERIZED}" == "1" ]]; then
  # The CLI runs as a container on Kind's own "kind" network (see
  # _install_openshell_cli_container_wrapper); it reaches the Gateway load
  # balancer directly by container address on its real port, not through the
  # host-side ephemeral-port remap _DISCOVER_GW_ENDPOINT resolves to.
  if [[ -z "${_DISCOVER_GW_HOST}" || -z "${_DISCOVER_GW_LB_ADDR}" ]]; then
    fail_test "Could not resolve the Gateway's in-cluster address for the containerized CLI"
    exit 1
  fi
  GW_ENDPOINT="https://${_DISCOVER_GW_HOST}:443"
fi
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
    'oidc_client_id': os.environ['OIDC_CLIENT_ID_EFFECTIVE'],
    'gateway_insecure': bool(os.environ.get('OPENSHELL_GATEWAY_INSECURE', ''))
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
  if [[ -n "$GATEWAY_SA_DIR" ]]; then
    gateway_service_account install-token
    install_gateway_service_account_cli
  fi
else
  fail_test "Failed to write gateway config"
fi
sep

# ── 6. gateway connectivity ───────────────────────────────────────────────

echo ""
e2e_area "6. Gateway Connectivity"
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
e2e_area "7. Sandbox Lifecycle"
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
e2e_area "8. Sandbox Interaction + Active Sandbox Count"
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
    # api_curl sends _OIDC_ACCESS_TOKEN, which may still hold a per-gateway
    # token (wrong audience for the HyperShell API) or have expired; refresh the
    # management-API token each poll like the other polling loops.
    acquire_oidc_token 2>/dev/null || true
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
e2e_area "9. Developer User RBAC Verification"
echo ""

if ! e2e_multi_identity; then
  dim "  Skipped (E2E_MODE=${E2E_MODE}): developer RBAC needs a second user identity (token-exchange impersonation); short runs as a single principal and is not granted impersonation"
else
# The developer's gateway/CLI token, like the admin's, must be minted against the
# per-gateway client on every infra target. The gateway requires user_role
# (openshell-user) on that client or it rejects the developer outright ("role
# 'openshell-user' required"). In production the RoleBinding reconciler assigns
# this when a gateway:viewer binding is created, but that grant is not
# expressible through the API for a non-owner (no user_id discovery path), so we
# provision the same end state directly in Keycloak -- a test-setup shortcut,
# not a product change.
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

# DEV_TOKEN carries only the per-gateway audience, which the HyperShell API
# rejects (--jwt-audience). Developer calls to the management API use a token
# minted for the API client instead, like the platform admin token below.
show_cmd "# acquire HyperShell API OIDC token for developer (client: ${E2E_OIDC_CLIENT_ID})"
DEV_API_TOKEN=""
if acquire_oidc_token "$E2E_DEV_USERNAME" "$E2E_DEV_PASSWORD"; then
  DEV_API_TOKEN="${_OIDC_ACCESS_TOKEN}"
else
  fail_test "Failed to acquire developer HyperShell API OIDC token"
fi
# Restore the admin management-API token for the api_curl calls that follow.
acquire_oidc_token 2>/dev/null || true

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
    'oidc_client_id': os.environ['DEV_OIDC_CLIENT_ID_EFFECTIVE'],
    'gateway_insecure': bool(os.environ.get('OPENSHELL_GATEWAY_INSECURE', ''))
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
  DEV_SB_EARLY_EXIT=false
  DEV_START=$(date +%s)
  DEV_DEADLINE=$((DEV_START + E2E_SANDBOX_TIMEOUT))
  DEV_TRACE=""
  while [[ $(date +%s) -lt $DEV_DEADLINE ]]; do
    DEV_PODS=$($CLI get pods -n "$GW_NAMESPACE" --no-headers 2>/dev/null | grep -i "default--${DEV_SANDBOX}" || true)
    if [[ -n "$DEV_PODS" ]]; then
      DEV_TRACE+="t+$(($(date +%s) - DEV_START))s pod=$(echo "$DEV_PODS" | awk '{print $1":"$3}' | head -1); "
      DEV_POD_CREATED=true
      break
    fi
    if ! kill -0 "$DEV_SB_PID" 2>/dev/null; then
      DEV_TRACE+="t+$(($(date +%s) - DEV_START))s no pod, CLI process gone; "
      DEV_SB_EARLY_EXIT=true
      break
    fi
    DEV_TRACE+="t+$(($(date +%s) - DEV_START))s no pod, CLI running; "
    sleep 2
  done

  DEV_SB_RC=""
  if [[ "$DEV_SB_EARLY_EXIT" == "true" ]]; then
    # The CLI already ended on its own: its exit status says whether it failed.
    # `|| DEV_SB_RC=$?` captures a non-zero status without tripping errexit.
    DEV_SB_RC=0
    wait "$DEV_SB_PID" 2>/dev/null || DEV_SB_RC=$?
  else
    kill "$DEV_SB_PID" 2>/dev/null || true
    wait "$DEV_SB_PID" 2>/dev/null || true
  fi

  # Drop the containerized CLI's benign start-up warnings (amd64 image on an arm64
  # host, disabled TLS verification) so the 200 character excerpt below is the
  # actual error, not the warnings that precede it.
  # grep -v exits 1 when nothing is left (warning-only or empty log); guard it so
  # pipefail/errexit does not abort the run.
  DEV_SB_ERR=$({ sed 's/\x1b\[[0-9;]*m//g' "${DEV_SB_LOG}" 2>/dev/null \
    | grep -vE '^WARNING: image platform| WARN openshell_cli::tls' | tr '\n' ' ' | tr -s ' '; } || true)
  DEV_SB_FULL_LOG="${TMPDIR:-/tmp}/e2e-dev-sandbox-create.log"
  sed 's/\x1b\[[0-9;]*m//g' "${DEV_SB_LOG}" > "${DEV_SB_FULL_LOG}" 2>/dev/null || true
  rm -f "${DEV_SB_LOG}" 2>/dev/null || true

  # Best-effort cleanup of the developer sandbox. The create assertion above is
  # already decided, so a failed delete (e.g. a gateway timeout) is reported as a
  # warning instead of failing the step, but is never reported as success.
  dev_sandbox_cleanup() {
    local out
    if out=$("${OPENSHELL_BIN}" -g "${DEV_GW_LOCAL_NAME}" sandbox delete "${DEV_SANDBOX}" 2>&1); then
      dim "    Developer sandbox ${DEV_SANDBOX} deleted"
    else
      dim "    WARNING: could not delete developer sandbox ${DEV_SANDBOX} (may leak): ${out:0:200}"
    fi
  }

  if [[ "$DEV_POD_CREATED" == "true" ]]; then
    pass "Developer user: sandbox create allowed (user_role member of 'default')"
    dev_sandbox_cleanup
  elif echo "$DEV_SB_ERR" | grep -qE "Created sandbox: ${DEV_SANDBOX}"; then
    # The gateway accepted the create request, which is what this step asserts: a
    # workspace member with user_role MAY create sandboxes. The CLI has no terminal
    # here, so it can end on its own once the sandbox is allocated, and the sandbox
    # may be gone again before the next pod poll; scheduling the pod is covered by
    # the admin sandbox lifecycle step.
    pass "Developer user: sandbox create allowed (user_role member of 'default'; sandbox allocated)"
    dev_sandbox_cleanup
  elif echo "$DEV_SB_ERR" | grep -qiE "not a member|permissiondenied|permission denied|not authorized|unauthorized|forbidden|denied"; then
    # A granted workspace member was still denied -> membership grant or user_role
    # mapping is misconfigured.
    fail_test "Developer user: sandbox create denied -- a 'user' member of 'default' should be allowed to create sandboxes"
    dim "    ${DEV_SB_ERR:0:200}"
  else
    # Neither created nor a recognizable denial -- surface output so infra
    # failures are not mistaken for an authz result.
    if [[ "$DEV_SB_EARLY_EXIT" == "true" ]]; then
      fail_test "Developer user: sandbox create exited before a pod appeared"
    else
      fail_test "Developer user: sandbox not created within ${E2E_SANDBOX_TIMEOUT}s"
    fi
    dim "    CLI exit status: ${DEV_SB_RC:-still running}; poll timeline: ${DEV_TRACE}"
    dim "    last CLI output: $(printf '%s' "${DEV_SB_ERR}" | tail -c 600)"
    dim "    full CLI output: ${DEV_SB_FULL_LOG}"
  fi
  fi

  # ── gateway list: collection GET must 200 even with no RoleBindings ──
  # OpenShift RBAC_DEFAULT_ROLES= leaves developer with only hypershell-users.
  # The list handler returns an empty items array; 403 is "Gateways could not
  # be loaded" in the web console (rbac-enforcement Error Response Opacity).
  show_cmd "curl ${API_HOST}/api/hypershell/v1/gateways (as developer) -> expect 200"
  DEV_LIST_FILE=$(mktemp)
  DEV_LIST_STATUS=$(_driver_curl -o "${DEV_LIST_FILE}" -w '%{http_code}' \
    "${API_HOST}/api/hypershell/v1/gateways" \
    -H "Authorization: Bearer ${DEV_API_TOKEN}" 2>/dev/null || true)
  DEV_LIST_KIND=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1])).get("kind",""))' \
    "${DEV_LIST_FILE}" 2>/dev/null || true)
  rm -f "${DEV_LIST_FILE}"
  if [[ "${DEV_LIST_STATUS}" == "200" && "${DEV_LIST_KIND}" == "GatewayList" ]]; then
    pass "Developer user: gateway list allowed (HTTP 200 GatewayList)"
  else
    fail_test "Developer user: gateway list returned HTTP ${DEV_LIST_STATUS:-none} kind=${DEV_LIST_KIND:-<none>} (want 200 GatewayList)"
  fi

  # ── gateway create: follows the deployment's RBAC_DEFAULT_ROLES ──
  # Kind leaves RBAC_DEFAULT_ROLES unset, so the API default (gateway:creator)
  # applies and every authenticated user can create (HYPERSHELL-262). OpenShift
  # sets RBAC_DEFAULT_ROLES to empty (production isolation); developer is not a
  # creator and MUST get 403 (e2e-testing.spec.md Openshell User May Not Create
  # a Gateway).
  DEV_GW_CREATE_NAME="e2e-dev-gw-$(date +%s | tail -c5)"
  DEV_GW_BODY=$(GW_NAME="$DEV_GW_CREATE_NAME" E2E_OIDC_ISSUER="$E2E_OIDC_ISSUER" \
    E2E_OIDC_CLIENT_ID="$E2E_OIDC_CLIENT_ID" E2E_CLUSTER_ID="${E2E_CLUSTER_ID:-}" python3 -c "
import json, os
body = {
    'name': os.environ['GW_NAME'],
    'placement': ({'mode': 'local-kind'} if os.environ.get('E2E_INFRA_DRIVER') == 'kind'
                  else {'network': 'public', 'provider': 'aws'}),
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
  if e2e_rbac_default_includes_creator; then
    show_cmd "curl -X POST ${API_HOST}/api/hypershell/v1/gateways (as developer) -> expect 201 (gateway:creator by default)"
    dim "  Expecting 201 Created (developer receives gateway:creator via RBAC_DEFAULT_ROLES)..."
  else
    show_cmd "curl -X POST ${API_HOST}/api/hypershell/v1/gateways (as developer) -> expect 403 (no default gateway:creator)"
    dim "  Expecting 403 Forbidden (RBAC_DEFAULT_ROLES is empty; developer is not a creator)..."
  fi

  DEV_GW_RESP_FILE=$(mktemp)
  DEV_GW_STATUS=$(_driver_curl -o "${DEV_GW_RESP_FILE}" -w '%{http_code}' \
    -X POST "${API_HOST}/api/hypershell/v1/gateways" \
    -H "Authorization: Bearer ${DEV_API_TOKEN}" \
    -H "Content-Type: application/json" \
    -d "${DEV_GW_BODY}" 2>/dev/null || true)
  DEV_GW_RESP=$(sed 's/\x1b\[[0-9;]*m//g' "${DEV_GW_RESP_FILE}" 2>/dev/null | tr '\n' ' ' | tr -s ' ')

  if e2e_rbac_default_includes_creator; then
    if [[ "$DEV_GW_STATUS" =~ ^2 ]]; then
      pass "Developer user: gateway create allowed (gateway:creator default binding active)"
      DEV_DEFAULT_GW_ID=$(echo "$DEV_GW_RESP" | python3 -c "import json,sys; print(json.load(sys.stdin).get('id',''))" 2>/dev/null || true)
      if [[ -n "$DEV_DEFAULT_GW_ID" ]]; then
        _driver_curl -X DELETE "${API_HOST}/api/hypershell/v1/gateways/${DEV_DEFAULT_GW_ID}" \
          -H "Authorization: Bearer ${DEV_API_TOKEN}" &>/dev/null || true
      fi
    elif [[ "$DEV_GW_STATUS" == "403" ]]; then
      fail_test "Developer user: gateway create blocked -- default gateway:creator binding was not assigned (HTTP 403)"
      dim "    ${DEV_GW_RESP:0:200}"
    else
      fail_test "Developer user: unexpected HTTP ${DEV_GW_STATUS:-none} on gateway create"
      dim "    ${DEV_GW_RESP:0:200}"
    fi
  else
    if [[ "$DEV_GW_STATUS" == "403" ]]; then
      pass "Developer user: gateway create denied (HTTP 403, no default gateway:creator)"
    elif [[ "$DEV_GW_STATUS" =~ ^2 ]]; then
      fail_test "Developer user: gateway create succeeded -- RBAC_DEFAULT_ROLES is empty so this must be 403"
      DEV_DEFAULT_GW_ID=$(echo "$DEV_GW_RESP" | python3 -c "import json,sys; print(json.load(sys.stdin).get('id',''))" 2>/dev/null || true)
      if [[ -n "$DEV_DEFAULT_GW_ID" ]]; then
        api_curl -X DELETE "${API_HOST}/api/hypershell/v1/gateways/${DEV_DEFAULT_GW_ID}" &>/dev/null || true
      fi
    else
      fail_test "Developer user: unexpected HTTP ${DEV_GW_STATUS:-none} on gateway create"
      dim "    ${DEV_GW_RESP:0:200}"
    fi
  fi
  rm -f "${DEV_GW_RESP_FILE}" 2>/dev/null || true

  "${OPENSHELL_BIN}" gateway remove "${DEV_GW_LOCAL_NAME}" 2>/dev/null || true
fi
fi
sep

# --- 9b. gateway access management (secondary users) ----------------------
#
# Exercises the gateway-access-management facade end to end with the secondary
# Keycloak users seeded by `make kind-up`
# (scripts/cluster/reconcile-test-users.sh): an owner grants a normal user, a
# granted admin, and a granted owner through POST /gateways/{id}/access, and the
# Owner > Admin > Viewer authorization hierarchy and the Keycloak Role Bridge are
# asserted (gateway-access-management.spec.md GAM-02..GAM-09).
#
#   1. normal user (role "user" = gateway:viewer) can create a sandbox
#   2. granted admin (role "admin" = gateway:admin) can remove the normal user
#   3. granted owner (role "owner" = gateway:owner) can remove the granted admin,
#      and can delete a gateway
#
# Requires a resource-owner password grant against the seeded secondary users.
# Brokered CI (E2E_OIDC_GRANT=client_credentials) cannot password-grant them and
# they are not impersonation targets (ephemeral-test-credentials.spec.md), so this
# area is Kind / local-OpenShift (password grant), long mode only.

echo ""
e2e_area "9b. Gateway Access Management (secondary users)"
echo ""

: "${E2E_GAM_USER:=alice}"    # normal user   -> role "user"  (gateway:viewer)
: "${E2E_GAM_ADMIN:=bob}"     # granted admin -> role "admin" (gateway:admin)
: "${E2E_GAM_OWNER:=carol}"   # granted owner -> role "owner" (gateway:owner)
: "${E2E_GAM_VIEWER:=dana}"   # second normal user the granted admin adds as viewer

if ! e2e_step long; then
  dim "  Skipped (E2E_MODE=${E2E_MODE}): access-management matrix acts as several secondary user identities"
elif [[ "${E2E_INFRA_DRIVER}" != "kind" ]]; then
  dim "  Skipped (E2E_INFRA_DRIVER=${E2E_INFRA_DRIVER}): the secondary users (${E2E_GAM_USER}/${E2E_GAM_ADMIN}/${E2E_GAM_OWNER}) are seeded only by 'make kind-up' (scripts/cluster/reconcile-test-users.sh)"
elif [[ "${E2E_OIDC_GRANT:-password}" != "password" ]]; then
  dim "  Skipped (E2E_OIDC_GRANT=${E2E_OIDC_GRANT}): secondary users (${E2E_GAM_USER}/${E2E_GAM_ADMIN}/${E2E_GAM_OWNER}) need a resource-owner password grant; brokered CI cannot mint their tokens"
elif [[ -z "${GW_ID}" ]]; then
  fail_test "Access management: no gateway id (area 2 did not provision a gateway)"
else
  GAM_LAST_BODY=""

  # --- small helpers over the access facade (keyed by gateway id + bearer) ---
  # Poll until the realm-user projection (GAM-09) lists the user, so a just-seeded
  # user is grantable (the projection has bounded staleness).
  gam_wait_directory() { # <gw> <bearer> <username>
    local gw="$1" token="$2" uname="$3" deadline
    deadline=$(($(date +%s) + 120))
    while [[ $(date +%s) -lt $deadline ]]; do
      if _driver_curl -H "Authorization: Bearer ${token}" \
          "${API_HOST}/api/hypershell/v1/gateways/${gw}/access/directory?search=${uname}" 2>/dev/null \
          | UNAME="$uname" python3 -c "import json,sys,os
u=os.environ['UNAME']
sys.exit(0 if any(i.get('username')==u for i in json.load(sys.stdin).get('items',[])) else 1)" 2>/dev/null; then
        return 0
      fi
      sleep 5
    done
    return 1
  }
  gam_grant() { # <gw> <bearer> <username> <role> -> prints HTTP code; body in GAM_LAST_BODY
    local gw="$1" token="$2" uname="$3" role="$4" f code body
    f=$(mktemp)
    body=$(UNAME="$uname" ROLE="$role" python3 -c "import json,os;print(json.dumps({'username':os.environ['UNAME'],'role':os.environ['ROLE']}))")
    code=$(_driver_curl -o "$f" -w '%{http_code}' -X POST \
      -H "Authorization: Bearer ${token}" -H "Content-Type: application/json" \
      "${API_HOST}/api/hypershell/v1/gateways/${gw}/access" -d "$body" 2>/dev/null || true)
    GAM_LAST_BODY=$(sed 's/\x1b\[[0-9;]*m//g' "$f" 2>/dev/null | tr '\n' ' ' | tr -s ' '); rm -f "$f"
    printf '%s' "$code"
  }
  gam_revoke() { # <gw> <bearer> <user_id> -> prints HTTP code; body in GAM_LAST_BODY
    local gw="$1" token="$2" uid="$3" f code
    f=$(mktemp)
    code=$(_driver_curl -o "$f" -w '%{http_code}' -X DELETE \
      -H "Authorization: Bearer ${token}" \
      "${API_HOST}/api/hypershell/v1/gateways/${gw}/access/${uid}" 2>/dev/null || true)
    GAM_LAST_BODY=$(sed 's/\x1b\[[0-9;]*m//g' "$f" 2>/dev/null | tr '\n' ' ' | tr -s ' '); rm -f "$f"
    printf '%s' "$code"
  }
  gam_user_id() { # <gw> <bearer> <username> -> prints HyperShell user_id (via GAM-03 list)
    local gw="$1" token="$2" uname="$3"
    _driver_curl -H "Authorization: Bearer ${token}" \
      "${API_HOST}/api/hypershell/v1/gateways/${gw}/access" 2>/dev/null \
      | UNAME="$uname" python3 -c "import json,sys,os
u=os.environ['UNAME']
print(next((i.get('user_id','') for i in json.load(sys.stdin).get('items',[]) if i.get('username')==u),''))" 2>/dev/null || true
  }
  gam_user_role() { # <gw> <bearer> <username> -> prints the user's tier (owner/admin/user) or empty
    local gw="$1" token="$2" uname="$3"
    _driver_curl -H "Authorization: Bearer ${token}" \
      "${API_HOST}/api/hypershell/v1/gateways/${gw}/access" 2>/dev/null \
      | UNAME="$uname" python3 -c "import json,sys,os
u=os.environ['UNAME']
print(next((i.get('role','') for i in json.load(sys.stdin).get('items',[]) if i.get('username')==u),''))" 2>/dev/null || true
  }

  # The admin user created the main gateway in area 2, so it is the first
  # gateway:owner (security/rbac-enforcement.spec.md) and may grant every tier.
  acquire_oidc_token 2>/dev/null || true
  GAM_OWNER_API_TOKEN="${_OIDC_ACCESS_TOKEN}"
  [[ -n "$GAM_OWNER_API_TOKEN" ]] || fail_test "Access management: failed to acquire owner (admin) API token"

  # == Scenario 0: a realm user added to Keycloak AFTER the platform is running is
  #    picked up by the control-plane directory projection (GAM-09 periodic refresh)
  #    and becomes searchable, then grantable. Unlike alice/bob/carol/dana, ellen is
  #    NOT seeded by `make kind-up`, so this proves the platform reflects a brand-new
  #    Keycloak user into its directory, and -- on grant -- pre-provisions a HyperShell
  #    User record in its database. The directory refresh interval was shortened up
  #    front (kind driver GC-timing patch) so the refresh lands within the wait. ==
  E2E_GAM_NEW_USER="${E2E_GAM_NEW_USER:-ellen}"
  show_cmd "# create Keycloak realm user ${E2E_GAM_NEW_USER} (not seeded by make kind-up)"
  if kc_create_user "$E2E_GAM_NEW_USER"; then
    pass "Created Keycloak realm user ${E2E_GAM_NEW_USER} at test time"
  else
    fail_test "Access management: could not create Keycloak user ${E2E_GAM_NEW_USER}"
  fi

  show_cmd "# await ${E2E_GAM_NEW_USER} in the directory projection (control plane picks up the new realm user)"
  if gam_wait_directory "$GW_ID" "$GAM_OWNER_API_TOKEN" "$E2E_GAM_NEW_USER"; then
    pass "Directory search resolves ${E2E_GAM_NEW_USER} after it was added to Keycloak (projection refreshed)"
  else
    fail_test "Access management: ${E2E_GAM_NEW_USER} never appeared in the directory projection after Keycloak create"
  fi

  show_cmd "POST ${API_HOST}/api/hypershell/v1/gateways/${GW_ID}/access {user: ${E2E_GAM_NEW_USER}, role: user}"
  GAM_CODE=$(gam_grant "$GW_ID" "$GAM_OWNER_API_TOKEN" "$E2E_GAM_NEW_USER" user)
  if [[ "$GAM_CODE" =~ ^2 ]]; then
    pass "Owner granted ${E2E_GAM_NEW_USER} the 'user' tier (HTTP ${GAM_CODE})"
  else
    fail_test "Access management: grant 'user' to ${E2E_GAM_NEW_USER} returned HTTP ${GAM_CODE:-none}"
    dim "    ${GAM_LAST_BODY:0:200}"
  fi
  GAM_NEW_USER_ID=$(gam_user_id "$GW_ID" "$GAM_OWNER_API_TOKEN" "$E2E_GAM_NEW_USER")
  if [[ -n "$GAM_NEW_USER_ID" ]]; then
    pass "${E2E_GAM_NEW_USER} resolves to a pre-provisioned HyperShell user_id in the access list (${GAM_NEW_USER_ID})"
  else
    fail_test "Access management: ${E2E_GAM_NEW_USER} has no HyperShell user_id after grant (not persisted to the database)"
  fi

  # == Scenario 1: normal user is granted "user" and can create a sandbox ==
  show_cmd "# wait for ${E2E_GAM_USER} in the gateway directory projection (GAM-09)"
  if gam_wait_directory "$GW_ID" "$GAM_OWNER_API_TOKEN" "$E2E_GAM_USER"; then
    pass "Directory search resolves ${E2E_GAM_USER} (realm-user projection populated)"
  else
    fail_test "Access management: ${E2E_GAM_USER} never appeared in the directory projection"
  fi

  show_cmd "POST ${API_HOST}/api/hypershell/v1/gateways/${GW_ID}/access {user: ${E2E_GAM_USER}, role: user}"
  GAM_CODE=$(gam_grant "$GW_ID" "$GAM_OWNER_API_TOKEN" "$E2E_GAM_USER" user)
  if [[ "$GAM_CODE" =~ ^2 ]]; then
    pass "Owner granted ${E2E_GAM_USER} the 'user' tier (gateway:viewer) (HTTP ${GAM_CODE})"
  else
    fail_test "Access management: grant 'user' to ${E2E_GAM_USER} returned HTTP ${GAM_CODE:-none}"
    dim "    ${GAM_LAST_BODY:0:200}"
  fi

  # The Role Bridge (GAM-02) projects gateway:viewer -> openshell-user on the
  # per-gateway Keycloak client. Poll the normal user's per-gateway token until
  # that client role appears: the end-to-end proof the grant reconciled.
  show_cmd "# acquire ${E2E_GAM_USER} per-gateway token, await Role Bridge openshell-user"
  if acquire_gateway_token_with_role "$E2E_GAM_USER" "$E2E_GAM_USER" "$GW_KC_CLIENT_ID" openshell-user; then
    GAM_USER_GW_TOKEN="${_OIDC_ACCESS_TOKEN}"
    pass "Role Bridge projected openshell-user to ${E2E_GAM_USER} (gateway:viewer -> openshell-user)"
  else
    GAM_USER_GW_TOKEN=""
    fail_test "Access management: Role Bridge did not project openshell-user to ${E2E_GAM_USER}"
  fi
  acquire_oidc_token 2>/dev/null || true  # restore owner token

  if [[ -n "$GAM_USER_GW_TOKEN" ]]; then
    # gateway:viewer / openshell-user authorizes sandbox creation only inside a
    # workspace the user is a member of. The access facade grants the gateway
    # role, NOT OpenShell workspace membership (a separate gateway-side concern;
    # see area 9). An owner/admin must still add the user to 'default' first.
    GAM_USER_SUBJECT=$(GAM_TOK="$GAM_USER_GW_TOKEN" python3 -c "
import os,json,base64
try:
    p=os.environ['GAM_TOK'].split('.')[1]; p+='='*(-len(p)%4)
    print(json.loads(base64.urlsafe_b64decode(p)).get('sub','') or '')
except Exception:
    pass" 2>/dev/null || true)
    if [[ -z "$GAM_USER_SUBJECT" ]]; then
      fail_test "Access management: could not resolve ${E2E_GAM_USER} OIDC subject for workspace membership"
    else
      show_cmd "${OPENSHELL_BIN} -g ${GW_LOCAL_NAME} workspace member add --workspace default --subject ${GAM_USER_SUBJECT} --role user"
      GAM_WM_LOG=$(mktemp)
      if "${OPENSHELL_BIN}" -g "${GW_LOCAL_NAME}" workspace member add \
          --workspace default --subject "${GAM_USER_SUBJECT}" --role user >"${GAM_WM_LOG}" 2>&1 \
          || grep -qiE "already|exists" "${GAM_WM_LOG}"; then
        pass "Owner added ${E2E_GAM_USER} as a 'user' member of 'default' workspace"
      else
        fail_test "Access management: failed to add ${E2E_GAM_USER} to 'default' workspace"
        dim "    $(sed 's/\x1b\[[0-9;]*m//g' "${GAM_WM_LOG}" | tr '\n' ' ' | tr -s ' ' | cut -c1-200)"
      fi
      rm -f "${GAM_WM_LOG}"

      # register a CLI config for the normal user and create a sandbox
      GAM_USER_GW_LOCAL="${GW_LOCAL_NAME}-${E2E_GAM_USER}"
      GAM_USER_CFG="${HOME}/.config/openshell/gateways/${GAM_USER_GW_LOCAL}"
      "${OPENSHELL_BIN}" gateway remove "${GAM_USER_GW_LOCAL}" 2>/dev/null || true
      mkdir -p "${GAM_USER_CFG}"
      GAM_USER_GW_LOCAL="$GAM_USER_GW_LOCAL" GW_ENDPOINT="$GW_ENDPOINT" \
        E2E_OIDC_ISSUER="$E2E_OIDC_ISSUER" GW_KC_CLIENT_ID="$GW_KC_CLIENT_ID" \
        GAM_USER_GW_TOKEN="$GAM_USER_GW_TOKEN" GAM_USER_CFG="$GAM_USER_CFG" python3 -c "
import json, os
d=os.environ['GAM_USER_CFG']
json.dump({'name':os.environ['GAM_USER_GW_LOCAL'],'gateway_endpoint':os.environ['GW_ENDPOINT'],
  'is_remote':True,'gateway_port':0,'auth_mode':'oidc','oidc_issuer':os.environ['E2E_OIDC_ISSUER'],
  'oidc_client_id':os.environ['GW_KC_CLIENT_ID'],
  'gateway_insecure':bool(os.environ.get('OPENSHELL_GATEWAY_INSECURE',''))}, open(os.path.join(d,'metadata.json'),'w'), indent=2)
json.dump({'access_token':os.environ['GAM_USER_GW_TOKEN'],'issuer':os.environ['E2E_OIDC_ISSUER'],
  'client_id':os.environ['GW_KC_CLIENT_ID']}, open(os.path.join(d,'oidc_token.json'),'w'), indent=2)
os.chmod(os.path.join(d,'metadata.json'),0o600); os.chmod(os.path.join(d,'oidc_token.json'),0o600)
"
      GAM_SANDBOX="e2e-gam-$(date +%s | tail -c5)"
      show_cmd "${OPENSHELL_BIN} -g ${GAM_USER_GW_LOCAL} sandbox create --name ${GAM_SANDBOX}"
      dim "  Expecting success (${E2E_GAM_USER} is a 'user' member of 'default' with openshell-user)..."
      GAM_SB_LOG=$(mktemp)
      "${OPENSHELL_BIN}" -g "${GAM_USER_GW_LOCAL}" sandbox create --name "${GAM_SANDBOX}" >"${GAM_SB_LOG}" 2>&1 &
      GAM_SB_PID=$!
      GAM_SB_OK=false
      GAM_SB_DEADLINE=$(($(date +%s) + E2E_SANDBOX_TIMEOUT))
      while [[ $(date +%s) -lt $GAM_SB_DEADLINE ]]; do
        if $CLI get pods -n "$GW_NAMESPACE" --no-headers 2>/dev/null | grep -qi "default--${GAM_SANDBOX}"; then
          GAM_SB_OK=true; break
        fi
        kill -0 "$GAM_SB_PID" 2>/dev/null || break
        sleep 5
      done
      kill "$GAM_SB_PID" 2>/dev/null || true; wait "$GAM_SB_PID" 2>/dev/null || true
      if [[ "$GAM_SB_OK" == "true" ]]; then
        pass "Normal user (${E2E_GAM_USER}): sandbox create allowed"
        "${OPENSHELL_BIN}" -g "${GAM_USER_GW_LOCAL}" sandbox delete "${GAM_SANDBOX}" 2>/dev/null || true
      else
        fail_test "Normal user (${E2E_GAM_USER}): sandbox not created within ${E2E_SANDBOX_TIMEOUT}s"
        dim "    $(sed 's/\x1b\[[0-9;]*m//g' "${GAM_SB_LOG}" | tr '\n' ' ' | tr -s ' ' | cut -c1-200)"
      fi
      rm -f "${GAM_SB_LOG}"
      "${OPENSHELL_BIN}" gateway remove "${GAM_USER_GW_LOCAL}" 2>/dev/null || true
    fi
  fi

  # -- negative: a viewer (user tier) cannot manage access (GAM-08). While ${E2E_GAM_USER}
  #    still holds the user tier from scenario 1, a grant attempt must be 403. --
  acquire_oidc_token "$E2E_GAM_USER" "$E2E_GAM_USER" 2>/dev/null || true
  GAM_VIEWER_API_TOKEN="${_OIDC_ACCESS_TOKEN}"
  acquire_oidc_token 2>/dev/null || true  # restore owner token
  if [[ -z "$GAM_VIEWER_API_TOKEN" ]]; then
    fail_test "Access management: could not acquire ${E2E_GAM_USER} API token for the viewer-denied check"
  else
    show_cmd "POST .../gateways/${GW_ID}/access {user: ${E2E_GAM_VIEWER}, role: user} (as viewer ${E2E_GAM_USER}) -> expect 403"
    GAM_CODE=$(gam_grant "$GW_ID" "$GAM_VIEWER_API_TOKEN" "$E2E_GAM_VIEWER" user)
    if [[ "$GAM_CODE" == "403" ]]; then
      pass "Normal user (${E2E_GAM_USER}, viewer) cannot add users (HTTP 403)"
    else
      fail_test "Access management: viewer ${E2E_GAM_USER} grant returned HTTP ${GAM_CODE:-none} (want 403)"
      dim "    ${GAM_LAST_BODY:0:200}"
    fi
  fi

  # == Scenario 2: granted admin can add another user as viewer, and remove a user ==
  gam_wait_directory "$GW_ID" "$GAM_OWNER_API_TOKEN" "$E2E_GAM_ADMIN" || true
  show_cmd "POST .../gateways/${GW_ID}/access {user: ${E2E_GAM_ADMIN}, role: admin}"
  GAM_CODE=$(gam_grant "$GW_ID" "$GAM_OWNER_API_TOKEN" "$E2E_GAM_ADMIN" admin)
  if [[ "$GAM_CODE" =~ ^2 ]]; then
    pass "Owner granted ${E2E_GAM_ADMIN} the 'admin' tier (gateway:admin) (HTTP ${GAM_CODE})"
  else
    fail_test "Access management: grant 'admin' to ${E2E_GAM_ADMIN} returned HTTP ${GAM_CODE:-none}"
    dim "    ${GAM_LAST_BODY:0:200}"
  fi

  acquire_oidc_token "$E2E_GAM_ADMIN" "$E2E_GAM_ADMIN" 2>/dev/null || true
  GAM_ADMIN_API_TOKEN="${_OIDC_ACCESS_TOKEN}"
  acquire_oidc_token 2>/dev/null || true  # restore owner token
  if [[ -z "$GAM_ADMIN_API_TOKEN" ]]; then
    fail_test "Access management: could not acquire ${E2E_GAM_ADMIN} API token for the admin management checks"
  else
    # -- a granted admin may grant the Admin and User tiers (GAM-08). Acting as
    #    bob, add a second user as viewer, searching the directory as the admin. --
    if gam_wait_directory "$GW_ID" "$GAM_ADMIN_API_TOKEN" "$E2E_GAM_VIEWER"; then
      pass "Granted admin (${E2E_GAM_ADMIN}) can search the directory and resolve ${E2E_GAM_VIEWER} (GAM-08)"
    else
      fail_test "Access management: admin ${E2E_GAM_ADMIN} could not resolve ${E2E_GAM_VIEWER} via directory search"
    fi
    show_cmd "POST .../gateways/${GW_ID}/access {user: ${E2E_GAM_VIEWER}, role: user} (as admin ${E2E_GAM_ADMIN})"
    GAM_CODE=$(gam_grant "$GW_ID" "$GAM_ADMIN_API_TOKEN" "$E2E_GAM_VIEWER" user)
    if [[ "$GAM_CODE" =~ ^2 ]]; then
      pass "Granted admin (${E2E_GAM_ADMIN}) added ${E2E_GAM_VIEWER} as viewer (user tier) (HTTP ${GAM_CODE})"
    else
      fail_test "Access management: admin grant of 'user' to ${E2E_GAM_VIEWER} returned HTTP ${GAM_CODE:-none}"
      dim "    ${GAM_LAST_BODY:0:200}"
    fi
    GAM_VIEWER_ROLE=$(gam_user_role "$GW_ID" "$GAM_OWNER_API_TOKEN" "$E2E_GAM_VIEWER")
    if [[ "$GAM_VIEWER_ROLE" == "user" ]]; then
      pass "${E2E_GAM_VIEWER} appears in the access list with role 'user'"
    else
      fail_test "Access management: ${E2E_GAM_VIEWER} role is '${GAM_VIEWER_ROLE:-<none>}', want 'user'"
    fi

    # -- negative: a granted admin may NOT touch the Owner tier (GAM-08). bob
    #    attempting to grant 'owner' must be 403 (no binding is created). --
    show_cmd "POST .../gateways/${GW_ID}/access {user: ${E2E_GAM_OWNER}, role: owner} (as admin ${E2E_GAM_ADMIN}) -> expect 403"
    GAM_CODE=$(gam_grant "$GW_ID" "$GAM_ADMIN_API_TOKEN" "$E2E_GAM_OWNER" owner)
    if [[ "$GAM_CODE" == "403" ]]; then
      pass "Granted admin (${E2E_GAM_ADMIN}) cannot grant the owner tier (HTTP 403)"
    else
      fail_test "Access management: admin ${E2E_GAM_ADMIN} owner-grant returned HTTP ${GAM_CODE:-none} (want 403)"
      dim "    ${GAM_LAST_BODY:0:200}"
    fi

    # -- a granted admin may also revoke a user-tier grant: remove the normal user --
    GAM_USER_ID=$(gam_user_id "$GW_ID" "$GAM_OWNER_API_TOKEN" "$E2E_GAM_USER")
    if [[ -z "$GAM_USER_ID" ]]; then
      fail_test "Access management: could not resolve ${E2E_GAM_USER} user_id for the admin-revoke check"
    else
      show_cmd "DELETE .../gateways/${GW_ID}/access/${GAM_USER_ID} (as admin ${E2E_GAM_ADMIN})"
      GAM_CODE=$(gam_revoke "$GW_ID" "$GAM_ADMIN_API_TOKEN" "$GAM_USER_ID")
      if [[ "$GAM_CODE" == "204" || "$GAM_CODE" == "200" ]]; then
        pass "Granted admin (${E2E_GAM_ADMIN}) removed normal user (${E2E_GAM_USER}) (HTTP ${GAM_CODE})"
      else
        fail_test "Access management: admin revoke of ${E2E_GAM_USER} returned HTTP ${GAM_CODE:-none}"
        dim "    ${GAM_LAST_BODY:0:200}"
      fi
      if [[ -z "$(gam_user_id "$GW_ID" "$GAM_OWNER_API_TOKEN" "$E2E_GAM_USER")" ]]; then
        pass "Normal user (${E2E_GAM_USER}) no longer appears in the gateway access list"
      else
        fail_test "Access management: ${E2E_GAM_USER} still has access after admin revoke"
      fi
    fi
  fi

  # == Scenario 3: granted owner can remove the admin, and can delete a gateway ==
  gam_wait_directory "$GW_ID" "$GAM_OWNER_API_TOKEN" "$E2E_GAM_OWNER" || true
  show_cmd "POST .../gateways/${GW_ID}/access {user: ${E2E_GAM_OWNER}, role: owner}"
  GAM_CODE=$(gam_grant "$GW_ID" "$GAM_OWNER_API_TOKEN" "$E2E_GAM_OWNER" owner)
  if [[ "$GAM_CODE" =~ ^2 ]]; then
    pass "Owner granted ${E2E_GAM_OWNER} the 'owner' tier (gateway:owner) (HTTP ${GAM_CODE})"
  else
    fail_test "Access management: grant 'owner' to ${E2E_GAM_OWNER} returned HTTP ${GAM_CODE:-none}"
    dim "    ${GAM_LAST_BODY:0:200}"
  fi

  acquire_oidc_token "$E2E_GAM_OWNER" "$E2E_GAM_OWNER" 2>/dev/null || true
  GAM_OWNER2_API_TOKEN="${_OIDC_ACCESS_TOKEN}"
  acquire_oidc_token 2>/dev/null || true  # restore owner token
  GAM_ADMIN_ID=$(gam_user_id "$GW_ID" "$GAM_OWNER_API_TOKEN" "$E2E_GAM_ADMIN")
  if [[ -z "$GAM_OWNER2_API_TOKEN" || -z "$GAM_ADMIN_ID" ]]; then
    fail_test "Access management: missing ${E2E_GAM_OWNER} token or ${E2E_GAM_ADMIN} user_id for the owner-revoke check"
  else
    show_cmd "DELETE .../gateways/${GW_ID}/access/${GAM_ADMIN_ID} (as owner ${E2E_GAM_OWNER})"
    GAM_CODE=$(gam_revoke "$GW_ID" "$GAM_OWNER2_API_TOKEN" "$GAM_ADMIN_ID")
    if [[ "$GAM_CODE" == "204" || "$GAM_CODE" == "200" ]]; then
      pass "Granted owner (${E2E_GAM_OWNER}) removed granted admin (${E2E_GAM_ADMIN}) (HTTP ${GAM_CODE})"
    else
      fail_test "Access management: owner revoke of ${E2E_GAM_ADMIN} returned HTTP ${GAM_CODE:-none}"
      dim "    ${GAM_LAST_BODY:0:200}"
    fi
  fi

  # -- an owner may add any tier and remove anyone (GAM-04 / GAM-06). The owner
  #    adds one admin and one viewer, then removes both. ${E2E_GAM_USER} and
  #    ${E2E_GAM_ADMIN} currently hold no binding (both were revoked above), so
  #    these are clean adds. --
  if [[ -n "$GAM_OWNER2_API_TOKEN" ]]; then
    gam_wait_directory "$GW_ID" "$GAM_OWNER2_API_TOKEN" "$E2E_GAM_USER" || true
    show_cmd "POST .../gateways/${GW_ID}/access {user: ${E2E_GAM_USER}, role: admin} (as owner ${E2E_GAM_OWNER})"
    GAM_CODE=$(gam_grant "$GW_ID" "$GAM_OWNER2_API_TOKEN" "$E2E_GAM_USER" admin)
    if [[ "$GAM_CODE" =~ ^2 ]]; then
      pass "Granted owner (${E2E_GAM_OWNER}) added ${E2E_GAM_USER} as admin (HTTP ${GAM_CODE})"
    else
      fail_test "Access management: owner grant of 'admin' to ${E2E_GAM_USER} returned HTTP ${GAM_CODE:-none}"
      dim "    ${GAM_LAST_BODY:0:200}"
    fi

    gam_wait_directory "$GW_ID" "$GAM_OWNER2_API_TOKEN" "$E2E_GAM_ADMIN" || true
    show_cmd "POST .../gateways/${GW_ID}/access {user: ${E2E_GAM_ADMIN}, role: user} (as owner ${E2E_GAM_OWNER})"
    GAM_CODE=$(gam_grant "$GW_ID" "$GAM_OWNER2_API_TOKEN" "$E2E_GAM_ADMIN" user)
    if [[ "$GAM_CODE" =~ ^2 ]]; then
      pass "Granted owner (${E2E_GAM_OWNER}) added ${E2E_GAM_ADMIN} as viewer (HTTP ${GAM_CODE})"
    else
      fail_test "Access management: owner grant of 'user' to ${E2E_GAM_ADMIN} returned HTTP ${GAM_CODE:-none}"
      dim "    ${GAM_LAST_BODY:0:200}"
    fi

    GAM_NEW_ADMIN_ID=$(gam_user_id "$GW_ID" "$GAM_OWNER2_API_TOKEN" "$E2E_GAM_USER")
    GAM_NEW_VIEWER_ID=$(gam_user_id "$GW_ID" "$GAM_OWNER2_API_TOKEN" "$E2E_GAM_ADMIN")
    if [[ -z "$GAM_NEW_ADMIN_ID" || -z "$GAM_NEW_VIEWER_ID" ]]; then
      fail_test "Access management: owner could not resolve the two users it just added for removal"
    else
      show_cmd "DELETE .../gateways/${GW_ID}/access/${GAM_NEW_ADMIN_ID} (as owner ${E2E_GAM_OWNER})"
      GAM_CODE=$(gam_revoke "$GW_ID" "$GAM_OWNER2_API_TOKEN" "$GAM_NEW_ADMIN_ID")
      if [[ "$GAM_CODE" == "204" || "$GAM_CODE" == "200" ]]; then
        pass "Granted owner (${E2E_GAM_OWNER}) removed the added admin (${E2E_GAM_USER}) (HTTP ${GAM_CODE})"
      else
        fail_test "Access management: owner revoke of added admin ${E2E_GAM_USER} returned HTTP ${GAM_CODE:-none}"
        dim "    ${GAM_LAST_BODY:0:200}"
      fi
      show_cmd "DELETE .../gateways/${GW_ID}/access/${GAM_NEW_VIEWER_ID} (as owner ${E2E_GAM_OWNER})"
      GAM_CODE=$(gam_revoke "$GW_ID" "$GAM_OWNER2_API_TOKEN" "$GAM_NEW_VIEWER_ID")
      if [[ "$GAM_CODE" == "204" || "$GAM_CODE" == "200" ]]; then
        pass "Granted owner (${E2E_GAM_OWNER}) removed the added viewer (${E2E_GAM_ADMIN}) (HTTP ${GAM_CODE})"
      else
        fail_test "Access management: owner revoke of added viewer ${E2E_GAM_ADMIN} returned HTTP ${GAM_CODE:-none}"
        dim "    ${GAM_LAST_BODY:0:200}"
      fi
      if [[ -z "$(gam_user_id "$GW_ID" "$GAM_OWNER2_API_TOKEN" "$E2E_GAM_USER")" \
         && -z "$(gam_user_id "$GW_ID" "$GAM_OWNER2_API_TOKEN" "$E2E_GAM_ADMIN")" ]]; then
        pass "Both users the owner added are removed from the access list"
      else
        fail_test "Access management: a user removed by the owner still has access"
      fi
    fi
  fi

  # "finally delete the gateway": the owner tier authorizes gateway deletion. This
  # is verified on a dedicated throwaway gateway rather than the main one, because
  # the main gateway must survive for areas 10-11 (platform-admin delete + the
  # namespace-GC assertion). A delete check needs no provisioning wait.
  if [[ -n "$GAM_OWNER2_API_TOKEN" ]]; then
    GAM_GW_NAME="e2e-gam-gw-$(date +%s | tail -c5)"
    GAM_GW_BODY=$(e2e_gateway_create_body "$GAM_GW_NAME")
    GAM_GW_ID=$(api_curl -X POST "${API_HOST}/api/hypershell/v1/gateways" \
      -H "Content-Type: application/json" -d "$GAM_GW_BODY" 2>/dev/null \
      | python3 -c "import json,sys; print(json.load(sys.stdin).get('id',''))" 2>/dev/null || true)
    if [[ -z "$GAM_GW_ID" ]]; then
      fail_test "Access management: failed to create throwaway gateway for the owner-delete check"
    else
      E2E_EXTRA_GW_IDS+=("$GAM_GW_ID")  # fail-safe cleanup if the delete below fails
      GAM_CODE=$(gam_grant "$GAM_GW_ID" "$GAM_OWNER_API_TOKEN" "$E2E_GAM_OWNER" owner)
      if [[ "$GAM_CODE" =~ ^2 ]]; then
        pass "Owner grant on throwaway gateway ${GAM_GW_NAME} (HTTP ${GAM_CODE})"
      else
        fail_test "Access management: grant owner on throwaway gateway returned HTTP ${GAM_CODE:-none}"
        dim "    ${GAM_LAST_BODY:0:200}"
      fi
      show_cmd "DELETE .../gateways/${GAM_GW_ID} (as owner ${E2E_GAM_OWNER})"
      GAM_DEL_F=$(mktemp)
      GAM_DEL_CODE=$(_driver_curl -o "$GAM_DEL_F" -w '%{http_code}' -X DELETE \
        "${API_HOST}/api/hypershell/v1/gateways/${GAM_GW_ID}" \
        -H "Authorization: Bearer ${GAM_OWNER2_API_TOKEN}" 2>/dev/null || true)
      if [[ "$GAM_DEL_CODE" == "204" || "$GAM_DEL_CODE" == "200" ]]; then
        pass "Granted owner (${E2E_GAM_OWNER}) deleted the gateway (HTTP ${GAM_DEL_CODE})"
        GAM_NEW_EXTRA=(); for _x in "${E2E_EXTRA_GW_IDS[@]}"; do [[ "$_x" != "$GAM_GW_ID" ]] && GAM_NEW_EXTRA+=("$_x"); done
        E2E_EXTRA_GW_IDS=("${GAM_NEW_EXTRA[@]}")
      else
        fail_test "Granted owner (${E2E_GAM_OWNER}): gateway delete returned HTTP ${GAM_DEL_CODE:-none} (want 204)"
        dim "    $(sed 's/\x1b\[[0-9;]*m//g' "$GAM_DEL_F" 2>/dev/null | tr '\n' ' ' | tr -s ' ' | cut -c1-200)"
      fi
      rm -f "$GAM_DEL_F"
    fi
  fi

  # == Scenario 4: a platform:admin manages access on a gateway it does NOT own ==
  #    (gateway-access-management.spec.md GAM-08). A plain creator (${E2E_GAM_USER})
  #    creates her own gateway and is its sole owner; the admin user (platform:admin,
  #    E2E_OIDC_USERNAME) holds no binding on it, yet may add a user through the access
  #    facade exactly like an owner, without being on the access list.
  acquire_oidc_token "$E2E_GAM_USER" "$E2E_GAM_USER" 2>/dev/null || true
  GAM_ALICE_API_TOKEN="${_OIDC_ACCESS_TOKEN}"
  acquire_oidc_token 2>/dev/null || true  # restore admin token (platform:admin)
  if [[ -z "$GAM_ALICE_API_TOKEN" ]]; then
    fail_test "Access management: could not acquire ${E2E_GAM_USER} API token to create her gateway"
  else
    # 1. ${E2E_GAM_USER} creates a gateway (kind grants gateway:creator by default)
    #    and becomes its first and only gateway:owner.
    GAM_PA_GW_NAME="e2e-padmin-gw-$(date +%s | tail -c5)"
    GAM_PA_GW_BODY=$(e2e_gateway_create_body "$GAM_PA_GW_NAME")
    show_cmd "POST ${API_HOST}/api/hypershell/v1/gateways (as ${E2E_GAM_USER}) -> expect 201"
    GAM_PA_RESP_F=$(mktemp)
    GAM_PA_GW_STATUS=$(_driver_curl -o "$GAM_PA_RESP_F" -w '%{http_code}' -X POST \
      "${API_HOST}/api/hypershell/v1/gateways" \
      -H "Authorization: Bearer ${GAM_ALICE_API_TOKEN}" -H "Content-Type: application/json" \
      -d "$GAM_PA_GW_BODY" 2>/dev/null || true)
    GAM_PA_GW_ID=$(python3 -c "import json,sys; print(json.load(open(sys.argv[1])).get('id',''))" "$GAM_PA_RESP_F" 2>/dev/null || true)
    rm -f "$GAM_PA_RESP_F"
    if [[ "$GAM_PA_GW_STATUS" =~ ^2 && -n "$GAM_PA_GW_ID" ]]; then
      E2E_EXTRA_GW_IDS+=("$GAM_PA_GW_ID")  # fail-safe cleanup
      pass "${E2E_GAM_USER} created her own gateway ${GAM_PA_GW_NAME} (HTTP ${GAM_PA_GW_STATUS})"
    else
      fail_test "Access management: ${E2E_GAM_USER} gateway create returned HTTP ${GAM_PA_GW_STATUS:-none}"
      GAM_PA_GW_ID=""
    fi

    if [[ -n "$GAM_PA_GW_ID" ]]; then
      # 2. the admin user (platform:admin) is NOT on ${E2E_GAM_USER}'s access list. It
      #    may read the list (platform:admin, GAM-03); ${E2E_GAM_USER} is the sole owner.
      GAM_PA_ADMIN_ROLE=$(gam_user_role "$GAM_PA_GW_ID" "$GAM_OWNER_API_TOKEN" "$E2E_OIDC_USERNAME")
      GAM_PA_ALICE_ROLE=$(gam_user_role "$GAM_PA_GW_ID" "$GAM_OWNER_API_TOKEN" "$E2E_GAM_USER")
      if [[ -z "$GAM_PA_ADMIN_ROLE" && "$GAM_PA_ALICE_ROLE" == "owner" ]]; then
        pass "Platform admin (${E2E_OIDC_USERNAME}) is not listed on ${E2E_GAM_USER}'s gateway (owner=${E2E_GAM_USER})"
      else
        fail_test "Access management: expected ${E2E_OIDC_USERNAME} absent and ${E2E_GAM_USER}=owner, got admin='${GAM_PA_ADMIN_ROLE}' ${E2E_GAM_USER}='${GAM_PA_ALICE_ROLE}'"
      fi

      # 3. the admin user (platform:admin, no binding here) adds ${E2E_GAM_ADMIN} as a
      #    user -- the owner-equivalent access management of GAM-08.
      gam_wait_directory "$GAM_PA_GW_ID" "$GAM_OWNER_API_TOKEN" "$E2E_GAM_ADMIN" || true
      show_cmd "POST .../gateways/${GAM_PA_GW_ID}/access {user: ${E2E_GAM_ADMIN}, role: user} (as platform admin ${E2E_OIDC_USERNAME})"
      GAM_CODE=$(gam_grant "$GAM_PA_GW_ID" "$GAM_OWNER_API_TOKEN" "$E2E_GAM_ADMIN" user)
      if [[ "$GAM_CODE" =~ ^2 ]]; then
        pass "Platform admin (${E2E_OIDC_USERNAME}) added ${E2E_GAM_ADMIN} as a user on a gateway it does not own (HTTP ${GAM_CODE})"
      else
        fail_test "Access management: platform-admin grant of ${E2E_GAM_ADMIN} returned HTTP ${GAM_CODE:-none} (want 2xx)"
        dim "    ${GAM_LAST_BODY:0:200}"
      fi
      if [[ "$(gam_user_role "$GAM_PA_GW_ID" "$GAM_OWNER_API_TOKEN" "$E2E_GAM_ADMIN")" == "user" ]]; then
        pass "${E2E_GAM_ADMIN} now holds the 'user' tier on ${E2E_GAM_USER}'s gateway"
      else
        fail_test "Access management: ${E2E_GAM_ADMIN} is not listed as 'user' after the platform-admin grant"
      fi

      # cleanup: delete ${E2E_GAM_USER}'s gateway as the platform admin (GAM-01).
      GAM_PA_DEL_F=$(mktemp)
      GAM_PA_DEL_CODE=$(_driver_curl -o "$GAM_PA_DEL_F" -w '%{http_code}' -X DELETE \
        "${API_HOST}/api/hypershell/v1/gateways/${GAM_PA_GW_ID}" \
        -H "Authorization: Bearer ${GAM_OWNER_API_TOKEN}" 2>/dev/null || true)
      rm -f "$GAM_PA_DEL_F"
      if [[ "$GAM_PA_DEL_CODE" == "204" || "$GAM_PA_DEL_CODE" == "200" ]]; then
        GAM_NEW_EXTRA=(); for _x in "${E2E_EXTRA_GW_IDS[@]}"; do [[ "$_x" != "$GAM_PA_GW_ID" ]] && GAM_NEW_EXTRA+=("$_x"); done
        E2E_EXTRA_GW_IDS=("${GAM_NEW_EXTRA[@]}")
      fi
    fi
  fi

  acquire_oidc_token 2>/dev/null || true  # restore admin token for later areas
fi
sep

# ── 10. platform admin RBAC verification ─────────────────────────────────

echo ""
e2e_area "10. Platform Admin RBAC Verification"
echo ""

if ! e2e_step long; then
  dim "  Skipped (E2E_MODE=${E2E_MODE}): platform-admin assertions delete a gateway"
else
# The platform:admin role is a realm role (not a client role) assigned in Keycloak.
# Platform admins can view all gateways and delete any gateway, but cannot modify
# gateways they don't own or create gateways without gateway:creator.

# Assign platform:admin realm role to the platform admin user (best-effort; user may
# already have the role from Keycloak realm import)
show_cmd "# verify/assign platform:admin realm role to ${E2E_PLATFORM_ADMIN_USERNAME}"
if assign_realm_role "$E2E_PLATFORM_ADMIN_USERNAME" "platform:admin"; then
  pass "Platform admin has platform:admin realm role"
else
  dim "  Note: Could not verify platform:admin role assignment (user may already have it from realm import)"
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
  PADMIN_LIST_STATUS=$(_driver_curl -o "${PADMIN_LIST_FILE}" -w '%{http_code}' \
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
  PADMIN_BINDINGS_STATUS=$(_driver_curl -o "${PADMIN_BINDINGS_FILE}" -w '%{http_code}' \
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
  PADMIN_DELETE_STATUS=$(_driver_curl -o "${PADMIN_DELETE_FILE}" -w '%{http_code}' \
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

  # ── gateway create: platform:admin is view and delete, not create ──
  # Kind's default RBAC_DEFAULT_ROLES still grants gateway:creator (HYPERSHELL-262).
  # OpenShift leaves that env empty, so this POST MUST be 403.
  PADMIN_GW_CREATE_NAME="e2e-padmin-gw-$(date +%s | tail -c5)"
  PADMIN_GW_BODY=$(GW_NAME="$PADMIN_GW_CREATE_NAME" E2E_OIDC_ISSUER="$E2E_OIDC_ISSUER" \
    E2E_OIDC_CLIENT_ID="$E2E_OIDC_CLIENT_ID" E2E_CLUSTER_ID="${E2E_CLUSTER_ID:-}" python3 -c "
import json, os
body = {
    'name': os.environ['GW_NAME'],
    'placement': ({'mode': 'local-kind'} if os.environ.get('E2E_INFRA_DRIVER') == 'kind'
                  else {'network': 'public', 'provider': 'aws'}),
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
  if e2e_rbac_default_includes_creator; then
    show_cmd "curl -X POST ${API_HOST}/api/hypershell/v1/gateways (as platform admin) -> expect 201 (gateway:creator by default)"
    dim "  Expecting 201 Created (platform:admin receives gateway:creator via RBAC_DEFAULT_ROLES)..."
  else
    show_cmd "curl -X POST ${API_HOST}/api/hypershell/v1/gateways (as platform admin) -> expect 403 (no default gateway:creator)"
    dim "  Expecting 403 Forbidden (platform:admin is view and delete; create needs gateway:creator)..."
  fi

  PADMIN_CREATE_FILE=$(mktemp)
  PADMIN_CREATE_STATUS=$(_driver_curl -o "${PADMIN_CREATE_FILE}" -w '%{http_code}' \
    -X POST "${API_HOST}/api/hypershell/v1/gateways" \
    -H "Authorization: Bearer ${PADMIN_TOKEN}" \
    -H "Content-Type: application/json" \
    -d "${PADMIN_GW_BODY}" 2>/dev/null || true)
  PADMIN_CREATE_RESP=$(cat "${PADMIN_CREATE_FILE}" 2>/dev/null || true)

  if e2e_rbac_default_includes_creator; then
    if [[ "$PADMIN_CREATE_STATUS" =~ ^2 ]]; then
      pass "Platform admin: gateway create allowed (gateway:creator default binding active)"
      PADMIN_DEFAULT_GW_ID=$(echo "$PADMIN_CREATE_RESP" | python3 -c "import json,sys; print(json.load(sys.stdin).get('id',''))" 2>/dev/null || true)
      if [[ -n "$PADMIN_DEFAULT_GW_ID" ]]; then
        _driver_curl -X DELETE "${API_HOST}/api/hypershell/v1/gateways/${PADMIN_DEFAULT_GW_ID}" \
          -H "Authorization: Bearer ${PADMIN_TOKEN}" &>/dev/null || true
      fi
    elif [[ "$PADMIN_CREATE_STATUS" == "403" ]]; then
      fail_test "Platform admin: gateway create blocked -- default gateway:creator binding was not assigned (HTTP 403)"
      dim "    ${PADMIN_CREATE_RESP:0:200}"
    else
      fail_test "Platform admin: unexpected HTTP ${PADMIN_CREATE_STATUS:-none} on gateway create"
      dim "    ${PADMIN_CREATE_RESP:0:200}"
    fi
  else
    if [[ "$PADMIN_CREATE_STATUS" == "403" ]]; then
      pass "Platform admin: gateway create denied (HTTP 403, no default gateway:creator)"
    elif [[ "$PADMIN_CREATE_STATUS" =~ ^2 ]]; then
      fail_test "Platform admin: gateway create succeeded -- RBAC_DEFAULT_ROLES is empty so this must be 403"
      PADMIN_DEFAULT_GW_ID=$(echo "$PADMIN_CREATE_RESP" | python3 -c "import json,sys; print(json.load(sys.stdin).get('id',''))" 2>/dev/null || true)
      if [[ -n "$PADMIN_DEFAULT_GW_ID" ]]; then
        api_curl -X DELETE "${API_HOST}/api/hypershell/v1/gateways/${PADMIN_DEFAULT_GW_ID}" &>/dev/null || true
      fi
    else
      fail_test "Platform admin: unexpected HTTP ${PADMIN_CREATE_STATUS:-none} on gateway create"
      dim "    ${PADMIN_CREATE_RESP:0:200}"
    fi
  fi
  rm -f "${PADMIN_CREATE_FILE}" 2>/dev/null || true
fi
fi
sep

# ── 11. gateway deletion + namespace garbage collection ────────────────────

echo ""
e2e_area "11. Gateway Deletion + Namespace Garbage Collection"
echo ""

if [[ -n "$GATEWAY_SA_DIR" ]]; then
  if acquire_oidc_token && gateway_service_account revoke; then
    pass "Gateway service account revoked, token issuance rejected, and credential deleted"
  else
    fail_test "Gateway service-account revocation or deletion failed"
    exit 1
  fi
fi

if [[ "$E2E_MODE" == "perf" ]]; then
  # perf mode must not tear down the supplied/reused canary gateway. Exercise
  # delete-driven GC against a throwaway gateway instead, with a bounded wait.
  THROW_NAME="${GW_NAME}-gc-throwaway"
  dim "  Delete-driven GC on throwaway gateway ${THROW_NAME} (not ${GW_NAME})"
  acquire_oidc_token 2>/dev/null || true
  e2e_ensure_seed_ids || true
  e2e_lookup_gateway_by_name "$THROW_NAME"
  THROW_ID="${_GW_ID}"
  THROW_NS="${_GW_NAMESPACE}"
  if [[ -z "$THROW_ID" ]]; then
    if [[ -z "${E2E_CLUSTER_ID:-}" ]] || { ! e2e_seed_ids_ready && ! e2e_allow_unseeded; }; then
      fail_test "Cannot create throwaway gateway: registered cluster / seeded release ids are unknown"
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
  # managed (carries hypershell.redhat.io/managed=true,
  # app.kubernetes.io/managed-by=hypershell-control-plane, and
  # hypershell.redhat.io/instance=<this control plane>), so it MUST be reaped.
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

# ── 12. ManagedCluster registration + control-plane identity ────────────────
# Area 12 validates managed-cluster-registration.spec.md directly (not only by
# consuming the already-registered cluster), plus the control-plane gRPC identity
# binding and reconnect convergence (HYPERSHELL-241). Long only: it mutates fleet
# records and acts as the registrar/control-plane identity.

echo ""
e2e_area "12. ManagedCluster Registration + Control-Plane Identity"
echo ""

if ! e2e_step long; then
  dim "  Skipped (E2E_MODE=${E2E_MODE}): area 12 mutates fleet records and acts as a second identity"
else
  acquire_oidc_token 2>/dev/null || true
  e2e_ensure_seed_ids || true

  # ── 12a. Co-located control plane is registered ──
  show_cmd "api_curl ${API_HOST}/api/hypershell/v1/managed_clusters  # ${E2E_SEED_CLUSTER_NAME} registered + fresh"
  MC_LIST=$(api_curl "${API_HOST}/api/hypershell/v1/managed_clusters" 2>/dev/null || true)
  IFS=$'\t' read -r MC_SUBJECT MC_AGE <<< "$(echo "$MC_LIST" | WANT_NAME="$E2E_SEED_CLUSTER_NAME" python3 -c "
import json, os, sys, datetime
name = os.environ.get('WANT_NAME','')
try:
    data = json.load(sys.stdin)
except Exception:
    print('\t'); sys.exit(0)
rec = None
for it in data.get('items', []) or []:
    if not name or it.get('name','') == name:
        rec = it; break
if not rec:
    print('\t'); sys.exit(0)
subject = rec.get('oidc_subject','') or ''
age = ''
ls = rec.get('last_seen_at','') or ''
if ls:
    try:
        t = datetime.datetime.fromisoformat(ls.replace('Z','+00:00'))
        age = str(int((datetime.datetime.now(datetime.timezone.utc) - t).total_seconds()))
    except Exception:
        age = ''
print('%s\t%s' % (subject, age))
" 2>/dev/null)" || true
  if [[ -n "$MC_SUBJECT" ]]; then
    pass "ManagedCluster ${E2E_SEED_CLUSTER_NAME} has a control-plane oidc_subject"
  else
    fail_test "ManagedCluster ${E2E_SEED_CLUSTER_NAME} has no oidc_subject (control plane never registered)"
  fi
  if [[ -n "$MC_AGE" && "$MC_AGE" -lt 300 ]]; then
    pass "last_seen_at is fresh (${MC_AGE}s < 5m): the registration heartbeat is running"
  else
    fail_test "last_seen_at is stale or missing (age=${MC_AGE:-unknown}s); heartbeat may not be running"
  fi

  # ── 12b. Registration is idempotent ──
  MC_LS_BEFORE=$(api_curl "${API_HOST}/api/hypershell/v1/managed_clusters/${E2E_CLUSTER_ID}" 2>/dev/null | e2e_json_field last_seen_at)
  if acquire_registrar_token; then
    pass "Registrar client-credentials token acquired (${E2E_REGISTRAR_CLIENT_ID})"
    show_cmd "api_curl -X POST ${API_HOST}/api/hypershell/v1/managed_clusters/registration -d '{name: ${E2E_SEED_CLUSTER_NAME}}'"
    REG1=$(api_curl "${API_HOST}/api/hypershell/v1/managed_clusters/registration" \
      -X POST -H "Content-Type: application/json" -d "{\"name\":\"${E2E_SEED_CLUSTER_NAME}\"}" 2>/dev/null || true)
    REG1_ID=$(echo "$REG1" | e2e_json_field cluster_id)
    REG2=$(api_curl "${API_HOST}/api/hypershell/v1/managed_clusters/registration" \
      -X POST -H "Content-Type: application/json" -d "{\"name\":\"${E2E_SEED_CLUSTER_NAME}\"}" 2>/dev/null || true)
    REG2_ID=$(echo "$REG2" | e2e_json_field cluster_id)
    if [[ -n "$REG1_ID" && "$REG1_ID" == "$E2E_CLUSTER_ID" && "$REG2_ID" == "$E2E_CLUSTER_ID" ]]; then
      pass "Re-registration is idempotent (both calls returned the existing cluster_id ${E2E_CLUSTER_ID})"
    else
      fail_test "Re-registration did not return the existing cluster_id (got '${REG1_ID}' / '${REG2_ID}', want ${E2E_CLUSTER_ID})"
    fi
    # Restore the admin token for the inventory read, then confirm the heartbeat advanced.
    acquire_oidc_token 2>/dev/null || true
    MC_LS_AFTER=$(api_curl "${API_HOST}/api/hypershell/v1/managed_clusters/${E2E_CLUSTER_ID}" 2>/dev/null | e2e_json_field last_seen_at)
    if [[ -n "$MC_LS_AFTER" && ( -z "$MC_LS_BEFORE" || ! "$MC_LS_AFTER" < "$MC_LS_BEFORE" ) && "$MC_LS_AFTER" != "$MC_LS_BEFORE" ]]; then
      pass "Registration advanced last_seen_at (${MC_LS_BEFORE} -> ${MC_LS_AFTER})"
    elif [[ -n "$MC_LS_AFTER" ]]; then
      # String compare is lexicographic on RFC3339, which is chronological; a
      # non-advance within the same second is possible but unexpected after two POSTs.
      dim "    last_seen_at did not visibly advance (${MC_LS_BEFORE} -> ${MC_LS_AFTER}); within one second"
      pass "Registration POST accepted and heartbeat timestamp present"
    else
      fail_test "Could not read last_seen_at after registration"
    fi
  else
    fail_test "Could not acquire the registrar client-credentials token (${E2E_REGISTRAR_CLIENT_ID})"
  fi

  # ── 12c. Missing registrar role rejected ──
  # The admin user client has no managed-cluster-registrar role.
  acquire_oidc_token 2>/dev/null || true
  show_cmd "api_curl -X POST .../managed_clusters/registration (as admin, no registrar role) -> expect 403"
  REG_FORBIDDEN=$(e2e_http_status POST "${API_HOST}/api/hypershell/v1/managed_clusters/registration" \
    -H "Content-Type: application/json" -d "{\"name\":\"e2e-noauth-$RANDOM\"}")
  if [[ "$REG_FORBIDDEN" == "403" ]]; then
    pass "Registration without the managed-cluster-registrar role is 403 Forbidden"
  else
    fail_test "Expected 403 registering without the registrar role, got ${REG_FORBIDDEN:-none}"
  fi

  # ── 12d. Name collision rejected ──
  if acquire_registrar_token; then
    COLLIDE_NAME="${E2E_SEED_CLUSTER_NAME}-collide-$RANDOM"
    show_cmd "api_curl -X POST .../managed_clusters/registration -d '{name: ${COLLIDE_NAME}}' (registrar, different name) -> expect 409"
    COLLIDE_CODE=$(e2e_http_status POST "${API_HOST}/api/hypershell/v1/managed_clusters/registration" \
      -H "Content-Type: application/json" -d "{\"name\":\"${COLLIDE_NAME}\"}")
    acquire_oidc_token 2>/dev/null || true
    COLLIDE_PRESENT=$(api_curl "${API_HOST}/api/hypershell/v1/managed_clusters?search=name%3D%27${COLLIDE_NAME}%27" 2>/dev/null | e2e_json_field total)
    if [[ "$COLLIDE_CODE" == "409" ]]; then
      pass "Registering a different name under the same subject is 409 Conflict"
    else
      fail_test "Expected 409 for a name change under the same subject, got ${COLLIDE_CODE:-none}"
    fi
    if [[ "${COLLIDE_PRESENT:-0}" == "0" ]]; then
      pass "No ManagedCluster record was created for the rejected name ${COLLIDE_NAME}"
    else
      fail_test "A ManagedCluster record was created despite the 409 (${COLLIDE_NAME})"
    fi
  fi
  acquire_oidc_token 2>/dev/null || true

  # ── 12e. Gateway create rejects an unregistered cluster ──
  show_cmd "api_curl -X POST ${API_HOST}/api/hypershell/v1/gateways -d '{cluster_id: \"\"}' -> expect 400 naming cluster_id"
  EMPTY_CID_BODY=$(GW_NAME="e2e-empty-cid-$RANDOM" E2E_OIDC_ISSUER="$E2E_OIDC_ISSUER" \
    E2E_OIDC_CLIENT_ID="$E2E_OIDC_CLIENT_ID" python3 -c "
import json, os
print(json.dumps({
    'name': os.environ['GW_NAME'],
    'cluster_id': '',
    'placement': {'mode': 'local-kind'} if os.environ.get('E2E_INFRA_DRIVER') == 'kind' else {'network': 'public', 'provider': 'aws'},
    'oidc': json.dumps({'issuer': os.environ['E2E_OIDC_ISSUER'], 'audience': os.environ['E2E_OIDC_CLIENT_ID'],
                        'roles_claim': 'groups', 'admin_role': 'hypershell-admins', 'user_role': 'hypershell-users'}),
    'route': json.dumps({'enabled': True}),
}))")
  EMPTY_CID_FILE=$(mktemp)
  EMPTY_CID_CODE=$(api_curl -o "$EMPTY_CID_FILE" -w '%{http_code}' -X POST "${API_HOST}/api/hypershell/v1/gateways" \
    -H "Content-Type: application/json" -d "$EMPTY_CID_BODY" 2>/dev/null || true)
  EMPTY_CID_REASON=$(e2e_json_field reason < "$EMPTY_CID_FILE")
  rm -f "$EMPTY_CID_FILE"
  if [[ "$EMPTY_CID_CODE" == "400" && "$EMPTY_CID_REASON" == *cluster_id* ]]; then
    pass "Gateway create with an empty cluster_id is 400 and names cluster_id"
  else
    fail_test "Empty cluster_id: expected 400 naming cluster_id, got ${EMPTY_CID_CODE:-none} (${EMPTY_CID_REASON:0:80})"
  fi

  # Create an inert placeholder ManagedCluster (empty oidc_subject: no control
  # plane serves it), confirm a gateway referencing it is refused, then delete it.
  show_cmd "api_curl -X POST ${API_HOST}/api/hypershell/v1/managed_clusters -d '{placeholder}'  # empty oidc_subject"
  PLACEHOLDER=$(api_curl -X POST "${API_HOST}/api/hypershell/v1/managed_clusters" \
    -H "Content-Type: application/json" \
    -d "{\"name\":\"e2e-placeholder-$RANDOM\",\"provider\":\"none\",\"kubeconfig_secret\":\"none\"}" 2>/dev/null || true)
  PLACEHOLDER_ID=$(echo "$PLACEHOLDER" | e2e_json_field id)
  if [[ -n "$PLACEHOLDER_ID" && "$(echo "$PLACEHOLDER" | e2e_json_field kind)" == "ManagedCluster" ]]; then
    PLACEHOLDER_GW_BODY=$(GW_NAME="e2e-ph-gw-$RANDOM" E2E_OIDC_ISSUER="$E2E_OIDC_ISSUER" \
      E2E_OIDC_CLIENT_ID="$E2E_OIDC_CLIENT_ID" PH_ID="$PLACEHOLDER_ID" python3 -c "
import json, os
print(json.dumps({
    'name': os.environ['GW_NAME'],
    'cluster_id': os.environ['PH_ID'],
    'placement': {'mode': 'local-kind'} if os.environ.get('E2E_INFRA_DRIVER') == 'kind' else {'network': 'public', 'provider': 'aws'},
    'oidc': json.dumps({'issuer': os.environ['E2E_OIDC_ISSUER'], 'audience': os.environ['E2E_OIDC_CLIENT_ID'],
                        'roles_claim': 'groups', 'admin_role': 'hypershell-admins', 'user_role': 'hypershell-users'}),
    'route': json.dumps({'enabled': True}),
}))")
    PH_GW_FILE=$(mktemp)
    PH_GW_CODE=$(api_curl -o "$PH_GW_FILE" -w '%{http_code}' -X POST "${API_HOST}/api/hypershell/v1/gateways" \
      -H "Content-Type: application/json" -d "$PLACEHOLDER_GW_BODY" 2>/dev/null || true)
    PH_GW_REASON=$(e2e_json_field reason < "$PH_GW_FILE")
    rm -f "$PH_GW_FILE"
    if [[ "$PH_GW_CODE" == "400" && "$PH_GW_REASON" == *cluster_id* ]]; then
      pass "Gateway create against an unregistered ManagedCluster is 400 and names cluster_id"
    else
      fail_test "Unregistered cluster: expected 400 naming cluster_id, got ${PH_GW_CODE:-none} (${PH_GW_REASON:0:80})"
    fi
    PH_DEL=$(e2e_http_status DELETE "${API_HOST}/api/hypershell/v1/managed_clusters/${PLACEHOLDER_ID}")
    if [[ "$PH_DEL" == "204" || "$PH_DEL" == "404" ]]; then
      pass "Inert placeholder ManagedCluster deleted (clean state restored)"
    else
      fail_test "Could not delete the placeholder ManagedCluster (HTTP ${PH_DEL:-none})"
    fi
  else
    fail_test "Could not create the inert placeholder ManagedCluster for the unregistered-cluster check"
  fi

  # ── 12f. Unauthorized or revoked gRPC identity is rejected ──
  # Exercises the public gRPC Watch boundary (managed-cluster-registration.spec.md
  # Watch Stream Caller Binding). Needs grpcurl against the api-server gRPC port;
  # the Kind path port-forwards svc/hypershell-api-server:9000 (plaintext by
  # default, as scripts/kind/grpc-tls-smoke.sh documents).
  if [[ "$E2E_INFRA_DRIVER" != "kind" ]]; then
    dim "  Skipped: gRPC identity-rejection checks run on the kind driver (OpenShift gRPC boundary discovery is a follow-up)"
  elif ! command -v grpcurl >/dev/null 2>&1; then
    dim "  Skipped: grpcurl not installed (install github.com/fullstorydev/grpcurl to cover the gRPC identity boundary)"
  else
    GRPC_PF_PORT="${E2E_GRPC_PF_PORT:-19000}"
    GRPC_PF_LOG=$(mktemp)
    $CLI port-forward -n "${E2E_HS_NAMESPACE}" svc/hypershell-api-server "${GRPC_PF_PORT}:9000" >"${GRPC_PF_LOG}" 2>&1 &
    GRPC_PF_PID=$!
    for _ in $(seq 1 30); do grep -q 'Forwarding from' "${GRPC_PF_LOG}" 2>/dev/null && break; sleep 0.5; done
    GRPC_ADDR="127.0.0.1:${GRPC_PF_PORT}"
    GRPC_SVC="hypershell.v1.GatewayService/WatchGateways"

    show_cmd "grpcurl -plaintext ${GRPC_ADDR} ${GRPC_SVC}  # no token -> Unauthenticated"
    NOTOKEN_OUT=$(grpcurl -plaintext -max-time 10 -d '{}' "${GRPC_ADDR}" "${GRPC_SVC}" 2>&1 || true)
    if grep -qi 'Unauthenticated' <<<"$NOTOKEN_OUT"; then
      pass "gRPC WatchGateways without a token is UNAUTHENTICATED"
    else
      fail_test "gRPC WatchGateways without a token did not return UNAUTHENTICATED"
      dim "    ${NOTOKEN_OUT##*$'\n'}"
    fi

    if acquire_registrar_token; then
      GRPC_TOKEN="${_OIDC_ACCESS_TOKEN}"
      show_cmd "grpcurl -H 'authorization: Bearer ...' -d '{cluster_id: other}' ${GRPC_ADDR} ${GRPC_SVC}  # -> PermissionDenied"
      WRONGCID_OUT=$(grpcurl -plaintext -max-time 10 -H "authorization: Bearer ${GRPC_TOKEN}" \
        -d '{"cluster_id":"e2e-not-my-cluster"}' "${GRPC_ADDR}" "${GRPC_SVC}" 2>&1 || true)
      if grep -qi 'PermissionDenied' <<<"$WRONGCID_OUT"; then
        pass "gRPC WatchGateways with a foreign cluster_id is PERMISSION_DENIED"
      else
        fail_test "gRPC WatchGateways with a foreign cluster_id did not return PERMISSION_DENIED"
        dim "    ${WRONGCID_OUT##*$'\n'}"
      fi

      show_cmd "grpcurl -H 'authorization: Bearer ...' -d '{}' ${GRPC_ADDR} ${GRPC_SVC}  # no cluster_id -> InvalidArgument"
      NOCID_OUT=$(grpcurl -plaintext -max-time 10 -H "authorization: Bearer ${GRPC_TOKEN}" \
        -d '{}' "${GRPC_ADDR}" "${GRPC_SVC}" 2>&1 || true)
      if grep -qi 'InvalidArgument' <<<"$NOCID_OUT"; then
        pass "gRPC WatchGateways with no cluster_id filter is INVALID_ARGUMENT"
      else
        fail_test "gRPC WatchGateways with no cluster_id filter did not return INVALID_ARGUMENT"
        dim "    ${NOCID_OUT##*$'\n'}"
      fi
    else
      fail_test "Could not acquire the registrar token for the gRPC cluster-binding checks"
    fi

    kill "$GRPC_PF_PID" 2>/dev/null || true
    wait "$GRPC_PF_PID" 2>/dev/null || true
    rm -f "$GRPC_PF_LOG"
    acquire_oidc_token 2>/dev/null || true
  fi

  # ── 12g. Reconnect converges on the current desired state ──
  # Disconnect the control plane (scale to 0), create a gateway via the API while
  # it is down, reconnect, and confirm the control plane converges from its
  # snapshot: the gateway created during the outage reaches Running. This is the
  # snapshot-delivery guarantee of HYPERSHELL-241 (the control plane re-reads
  # current desired state on reconnect, not only live events).
  #
  # The spec also lists a gateway *deleted* during the outage. That half is not
  # exercised here: the gateway-delete API path performs service-account cleanup
  # through the control plane's in-cluster provisioner, so a DELETE issued while
  # the control plane is scaled to 0 returns 503 ("gateway service-account cleanup
  # is unavailable") -- the deletion cannot be initiated during the outage on this
  # platform. Delete-driven namespace reaping on the live watch is covered by
  # area 11 (watch-delete-events.spec.md) and the periodic reaper.
  #
  # Scaling the control plane to zero stalls every other suite sharing the
  # cluster, so when one runs concurrently (run-parallel.sh) wait for it first.
  e2e_wait_disruptive_gate
  if [[ -z "${E2E_CLUSTER_ID:-}" ]]; then
    fail_test "Skipped reconnect convergence: no registered cluster_id discovered"
  else
    RECON_NEW_NAME="e2e-recon-new-$(date +%s | tail -c6)"

    show_cmd "$CLI scale deployment/hypershell-controller -n ${E2E_HS_NAMESPACE} --replicas=0  # disconnect"
    if $CLI scale deployment/hypershell-controller -n "${E2E_HS_NAMESPACE}" --replicas=0 >/dev/null 2>&1 \
       && $CLI rollout status deployment/hypershell-controller -n "${E2E_HS_NAMESPACE}" --timeout=120s >/dev/null 2>&1; then
      pass "Control plane disconnected (scaled to 0)"
    else
      fail_test "Could not scale the control plane to 0 for the reconnect test"
    fi

    # Change desired state while disconnected: create a new gateway. (Create needs
    # no control plane; only the later reconcile does.)
    acquire_oidc_token 2>/dev/null || true
    RECON_NEW_BODY=$(e2e_gateway_create_body "$RECON_NEW_NAME")
    RECON_NEW_RESP=$(api_curl -X POST "${API_HOST}/api/hypershell/v1/gateways" \
      -H "Content-Type: application/json" -d "$RECON_NEW_BODY" 2>/dev/null || true)
    e2e_parse_gateway_response "$RECON_NEW_RESP"
    RECON_NEW_ID="$_CREATE_ID"
    [[ -n "$RECON_NEW_ID" ]] && E2E_EXTRA_GW_IDS+=("$RECON_NEW_ID")
    if [[ "$_CREATE_KIND" == "OK" && -n "$RECON_NEW_ID" ]]; then
      pass "While disconnected: created ${RECON_NEW_NAME} via the API (${RECON_NEW_ID})"
    else
      fail_test "Could not create the while-disconnected gateway ${RECON_NEW_NAME}"
      dim "    ${RECON_NEW_RESP:0:200}"
    fi

    show_cmd "$CLI scale deployment/hypershell-controller -n ${E2E_HS_NAMESPACE} --replicas=1  # reconnect"
    $CLI scale deployment/hypershell-controller -n "${E2E_HS_NAMESPACE}" --replicas=1 >/dev/null 2>&1 || true
    $CLI rollout status deployment/hypershell-controller -n "${E2E_HS_NAMESPACE}" --timeout=180s >/dev/null 2>&1 || true

    if [[ -n "$RECON_NEW_ID" ]]; then
      RECON_NEW_PHASE=$(e2e_wait_gateway_running "$RECON_NEW_ID" "$E2E_PROVISION_TIMEOUT") || true
      if [[ "$RECON_NEW_PHASE" == "Running" ]]; then
        pass "On reconnect the control plane converged the while-disconnected gateway to Running (snapshot delivery)"
      else
        fail_test "While-disconnected gateway ${RECON_NEW_NAME} did not reach Running after reconnect (phase=${RECON_NEW_PHASE})"
      fi

      # Delete the gateway now that the control plane is back (delete needs it).
      acquire_oidc_token 2>/dev/null || true
      api_curl -X DELETE "${API_HOST}/api/hypershell/v1/gateways/${RECON_NEW_ID}" >/dev/null 2>&1 || true
    fi
  fi

  # ── 12h. Multi-cluster fleet coverage (opt-in) ──
  if e2e_truthy "${E2E_MULTICLUSTER}"; then
    if [[ -n "${E2E_SEED_CLUSTER_NAME_2:-}" ]]; then
      acquire_oidc_token 2>/dev/null || true
      MC2_JSON=$(api_curl "${API_HOST}/api/hypershell/v1/managed_clusters?search=name%3D%27${E2E_SEED_CLUSTER_NAME_2}%27" 2>/dev/null || true)
      MC2_SUBJECT=$(echo "$MC2_JSON" | python3 -c "import json,sys
try:
    items=json.load(sys.stdin).get('items',[])
    print(items[0].get('oidc_subject','') if items else '')
except Exception: pass" 2>/dev/null)
      if [[ -n "$MC2_SUBJECT" ]]; then
        pass "Second cluster ${E2E_SEED_CLUSTER_NAME_2} is registered and selectable (E2E_MULTICLUSTER)"
      else
        fail_test "E2E_MULTICLUSTER=1 but second cluster ${E2E_SEED_CLUSTER_NAME_2} is not registered"
      fi
    else
      fail_test "E2E_MULTICLUSTER=1 requires E2E_SEED_CLUSTER_NAME_2"
    fi
  else
    dim "  Multi-cluster fleet coverage skipped (E2E_MULTICLUSTER unset): the second control plane is deployed by local-development.spec.md"
  fi
fi
sep

# ── 13. Gateway release promotion + reconciled status ───────────────────────
# Area 13 tested GatewayRelease and GatewayNetwork which were removed from the
# data model (feat/adlc-agent-runtime-spec). The area is permanently skipped.

echo ""
e2e_area "13. Gateway Release Promotion + Reconciled Status"
echo ""

if true; then
  dim "  Skipped: GatewayRelease and GatewayNetwork kinds were removed from the data model"
else
  acquire_oidc_token 2>/dev/null || true
  e2e_ensure_seed_ids || true

  # ── 13a. Reconciled status: GatewayRelease ──
  GOOD_IMAGE=""
  if [[ -n "${E2E_RELEASE_ID:-}" ]]; then
    GOOD_IMAGE=$(api_curl "${API_HOST}/api/hypershell/v1/gateway_releases/${E2E_RELEASE_ID}" 2>/dev/null | e2e_json_field image)
  fi
  : "${GOOD_IMAGE:=quay.io/openshift-online/hypershell-gateway:latest}"

  show_cmd "api_curl -X POST ${API_HOST}/api/hypershell/v1/gateway_releases -d '{valid image}'  # status -> Available"
  REL_GOOD=$(api_curl -X POST "${API_HOST}/api/hypershell/v1/gateway_releases" \
    -H "Content-Type: application/json" \
    -d "{\"name\":\"e2e-rel-good-$RANDOM\",\"image\":\"${GOOD_IMAGE}\"}" 2>/dev/null || true)
  REL_GOOD_ID=$(echo "$REL_GOOD" | e2e_json_field id)
  if [[ -n "$REL_GOOD_ID" ]]; then
    REL_GOOD_STATUS=$(e2e_poll_resource_status "${API_HOST}/api/hypershell/v1/gateway_releases/${REL_GOOD_ID}" e2e_status_is_available 120) || true
    if [[ "$REL_GOOD_STATUS" == "Available" ]]; then
      pass "GatewayRelease with a valid image settles to Available"
    else
      fail_test "GatewayRelease with a valid image did not settle to Available (status=${REL_GOOD_STATUS})"
    fi
  else
    fail_test "Could not create the valid GatewayRelease"
  fi

  show_cmd "api_curl -X POST ${API_HOST}/api/hypershell/v1/gateway_releases -d '{malformed image}'  # status -> Invalid"
  REL_BAD=$(api_curl -X POST "${API_HOST}/api/hypershell/v1/gateway_releases" \
    -H "Content-Type: application/json" \
    -d "{\"name\":\"e2e-rel-bad-$RANDOM\",\"image\":\"not a valid image\"}" 2>/dev/null || true)
  REL_BAD_ID=$(echo "$REL_BAD" | e2e_json_field id)
  if [[ -n "$REL_BAD_ID" ]]; then
    REL_BAD_STATUS=$(e2e_poll_resource_status "${API_HOST}/api/hypershell/v1/gateway_releases/${REL_BAD_ID}" e2e_status_is_invalid 120) || true
    if e2e_status_is_invalid "$REL_BAD_STATUS"; then
      pass "GatewayRelease with a malformed image settles to Invalid (${REL_BAD_STATUS})"
    else
      fail_test "GatewayRelease with a malformed image did not settle to Invalid (status=${REL_BAD_STATUS})"
    fi
  else
    fail_test "Could not create the malformed GatewayRelease"
  fi

  # ── 13b. Reconciled status: GatewayNetwork ──
  show_cmd "api_curl -X POST ${API_HOST}/api/hypershell/v1/gateway_networks -d '{topology: mesh}'  # status -> Valid"
  NET_GOOD=$(api_curl -X POST "${API_HOST}/api/hypershell/v1/gateway_networks" \
    -H "Content-Type: application/json" \
    -d "{\"name\":\"e2e-net-good-$RANDOM\",\"topology\":\"mesh\"}" 2>/dev/null || true)
  NET_GOOD_ID=$(echo "$NET_GOOD" | e2e_json_field id)
  if [[ -n "$NET_GOOD_ID" ]]; then
    NET_GOOD_STATUS=$(e2e_poll_resource_status "${API_HOST}/api/hypershell/v1/gateway_networks/${NET_GOOD_ID}" e2e_status_is_valid 120) || true
    if [[ "$NET_GOOD_STATUS" == "Valid" ]]; then
      pass "GatewayNetwork with a coherent topology settles to Valid"
    else
      fail_test "GatewayNetwork with a coherent topology did not settle to Valid (status=${NET_GOOD_STATUS})"
    fi
  else
    fail_test "Could not create the coherent GatewayNetwork"
  fi

  show_cmd "api_curl -X POST ${API_HOST}/api/hypershell/v1/gateway_networks -d '{hub-spoke, dangling hub}'  # status -> Invalid"
  NET_BAD=$(api_curl -X POST "${API_HOST}/api/hypershell/v1/gateway_networks" \
    -H "Content-Type: application/json" \
    -d "{\"name\":\"e2e-net-bad-$RANDOM\",\"topology\":\"hub-spoke\",\"hub_gateway_id\":\"e2e-does-not-exist\"}" 2>/dev/null || true)
  NET_BAD_ID=$(echo "$NET_BAD" | e2e_json_field id)
  if [[ -n "$NET_BAD_ID" ]]; then
    NET_BAD_STATUS=$(e2e_poll_resource_status "${API_HOST}/api/hypershell/v1/gateway_networks/${NET_BAD_ID}" e2e_status_is_invalid 120) || true
    if e2e_status_is_invalid "$NET_BAD_STATUS"; then
      pass "GatewayNetwork with a dangling hub settles to Invalid (${NET_BAD_STATUS})"
    else
      fail_test "GatewayNetwork with a dangling hub did not settle to Invalid (status=${NET_BAD_STATUS})"
    fi
  else
    fail_test "Could not create the incoherent GatewayNetwork"
  fi

  # ── 13c. Gateway release promotion (single-cluster) ──
  if [[ -z "${E2E_CLUSTER_ID:-}" || -z "$REL_GOOD_ID" ]]; then
    fail_test "Skipped promotion: need a registered cluster_id and a valid release"
  else
    # Release A = seeded release (or the valid one just created); Release B = a
    # second valid release with the same image (distinct id drives a rollout).
    REL_A_ID="${E2E_RELEASE_ID:-$REL_GOOD_ID}"
    REL_B="$REL_GOOD_ID"
    PROMO_NAME="e2e-promo-$(date +%s | tail -c6)"
    dim "  Provisioning promotion gateway ${PROMO_NAME} on release A (${REL_A_ID})..."
    PROMO_BODY=$(E2E_RELEASE_ID="$REL_A_ID" e2e_gateway_create_body "$PROMO_NAME")
    PROMO_RESP=$(api_curl -X POST "${API_HOST}/api/hypershell/v1/gateways" \
      -H "Content-Type: application/json" -d "$PROMO_BODY" 2>/dev/null || true)
    e2e_parse_gateway_response "$PROMO_RESP"
    PROMO_ID="$_CREATE_ID"
    [[ -n "$PROMO_ID" ]] && E2E_EXTRA_GW_IDS+=("$PROMO_ID")
    if [[ "$_CREATE_KIND" == "OK" && -n "$PROMO_ID" ]]; then
      PROMO_PHASE=$(e2e_wait_gateway_running "$PROMO_ID" "$E2E_PROVISION_TIMEOUT") || true
      if [[ "$PROMO_PHASE" == "Running" ]]; then
        pass "Promotion gateway reached Running on release A"
      else
        fail_test "Promotion gateway did not reach Running on release A (phase=${PROMO_PHASE})"
      fi

      # Promote A -> B: repoint release_id and wait for observed_release_id == B.
      show_cmd "api_curl -X PATCH ${API_HOST}/api/hypershell/v1/gateways/${PROMO_ID} -d '{release_id: B}'"
      api_curl -X PATCH "${API_HOST}/api/hypershell/v1/gateways/${PROMO_ID}" \
        -H "Content-Type: application/json" -d "{\"release_id\":\"${REL_B}\"}" >/dev/null 2>&1 || true
      PROMO_OBS=""
      PROMO_DEADLINE=$(($(date +%s) + E2E_PROVISION_TIMEOUT))
      while [[ $(date +%s) -lt $PROMO_DEADLINE ]]; do
        acquire_oidc_token 2>/dev/null || true
        PROMO_OBS=$(api_curl "${API_HOST}/api/hypershell/v1/gateways/${PROMO_ID}" 2>/dev/null | e2e_json_field observed_release_id)
        [[ "$PROMO_OBS" == "$REL_B" ]] && break
        sleep 5
      done
      if [[ "$PROMO_OBS" == "$REL_B" ]]; then
        pass "Rollout reports the serving release: observed_release_id == B"
      else
        fail_test "observed_release_id did not advance to B within ${E2E_PROVISION_TIMEOUT}s (got '${PROMO_OBS}')"
      fi

      # Failed rollout: repoint to a bad (unpullable) release. The gateway must go
      # Degraded and keep the last-good observed_release_id (B), never report the
      # bad release as serving.
      REL_BADROLL=$(api_curl -X POST "${API_HOST}/api/hypershell/v1/gateway_releases" \
        -H "Content-Type: application/json" \
        -d "{\"name\":\"e2e-rel-unpullable-$RANDOM\",\"image\":\"quay.io/openshift-online/hypershell-gateway:e2e-nonexistent-tag\"}" 2>/dev/null || true)
      REL_BADROLL_ID=$(echo "$REL_BADROLL" | e2e_json_field id)
      if [[ -n "$REL_BADROLL_ID" ]]; then
        show_cmd "api_curl -X PATCH .../gateways/${PROMO_ID} -d '{release_id: <unpullable>}'  # expect Degraded"
        api_curl -X PATCH "${API_HOST}/api/hypershell/v1/gateways/${PROMO_ID}" \
          -H "Content-Type: application/json" -d "{\"release_id\":\"${REL_BADROLL_ID}\"}" >/dev/null 2>&1 || true
        BADROLL_PHASE=""
        # The core safety guarantee (gateway-release-rollout.spec.md): a failed
        # rollout is NEVER reported as a successful move to the bad release, and the
        # last-good workload keeps serving. Watch the rollout settle and assert
        # observed_release_id never leaves the last-good release (B). The gateway
        # phase surfaces as Degraded only if the workload actually loses its serving
        # pods; a revision-aware rollout that keeps the last-good pods (the new
        # revision stuck ImagePullBackOff, Deployment still Available) stays Running
        # on B, which is equally safe. Either phase is acceptable; a move to the bad
        # release is not.
        BADROLL_OBS=""
        BADROLL_MOVED_TO_BAD=0
        BADROLL_SETTLE=$(($(date +%s) + 120))
        while [[ $(date +%s) -lt $BADROLL_SETTLE ]]; do
          acquire_oidc_token 2>/dev/null || true
          BR_JSON=$(api_curl "${API_HOST}/api/hypershell/v1/gateways/${PROMO_ID}" 2>/dev/null || true)
          BADROLL_PHASE=$(echo "$BR_JSON" | e2e_json_field phase)
          BADROLL_OBS=$(echo "$BR_JSON" | e2e_json_field observed_release_id)
          [[ "$BADROLL_OBS" == "$REL_BADROLL_ID" ]] && BADROLL_MOVED_TO_BAD=1
          [[ "$BADROLL_PHASE" == "Degraded" ]] && break
          sleep 5
        done
        if [[ "$BADROLL_MOVED_TO_BAD" == "0" && "$BADROLL_OBS" == "$REL_B" ]]; then
          pass "Failed rollout never became a successful move to the bad release; last-good (B) keeps serving (phase=${BADROLL_PHASE})"
        else
          fail_test "Failed rollout moved off the last-good release (observed=${BADROLL_OBS}, moved_to_bad=${BADROLL_MOVED_TO_BAD}); a bad rollout must not report the bad release as serving"
        fi

        # Recover: repoint back to B and confirm Running + observed_release_id == B.
        show_cmd "api_curl -X PATCH .../gateways/${PROMO_ID} -d '{release_id: B}'  # recover -> Running"
        api_curl -X PATCH "${API_HOST}/api/hypershell/v1/gateways/${PROMO_ID}" \
          -H "Content-Type: application/json" -d "{\"release_id\":\"${REL_B}\"}" >/dev/null 2>&1 || true
        RECOVER_PHASE=$(e2e_wait_gateway_running "$PROMO_ID" "$E2E_PROVISION_TIMEOUT") || true
        RECOVER_OBS=$(api_curl "${API_HOST}/api/hypershell/v1/gateways/${PROMO_ID}" 2>/dev/null | e2e_json_field observed_release_id)
        if [[ "$RECOVER_PHASE" == "Running" && "$RECOVER_OBS" == "$REL_B" ]]; then
          pass "Repointing back to the last-good release returns the gateway to Running (observed_release_id == B)"
        else
          fail_test "Gateway did not recover to Running on the last-good release (phase=${RECOVER_PHASE}, observed=${RECOVER_OBS})"
        fi
        api_curl -X DELETE "${API_HOST}/api/hypershell/v1/gateway_releases/${REL_BADROLL_ID}" >/dev/null 2>&1 || true
      else
        fail_test "Could not create the unpullable release for the failed-rollout check"
      fi

      if e2e_truthy "${E2E_MULTICLUSTER}"; then
        dim "  Cross-cluster promotion (E2E_MULTICLUSTER) would run here; the second control plane is owned by local-development.spec.md"
      fi

      acquire_oidc_token 2>/dev/null || true
      api_curl -X DELETE "${API_HOST}/api/hypershell/v1/gateways/${PROMO_ID}" >/dev/null 2>&1 || true
    else
      fail_test "Could not create the promotion gateway"
      dim "    ${PROMO_RESP:0:200}"
    fi
  fi

  # Clean up the releases/networks created for the status assertions.
  acquire_oidc_token 2>/dev/null || true
  for _rid in "$REL_GOOD_ID" "$REL_BAD_ID"; do
    [[ -n "$_rid" ]] && api_curl -X DELETE "${API_HOST}/api/hypershell/v1/gateway_releases/${_rid}" >/dev/null 2>&1 || true
  done
  for _nid in "$NET_GOOD_ID" "$NET_BAD_ID"; do
    [[ -n "$_nid" ]] && api_curl -X DELETE "${API_HOST}/api/hypershell/v1/gateway_networks/${_nid}" >/dev/null 2>&1 || true
  done
fi
sep

# ── 14. Admin inventory + API validation ────────────────────────────────────
# Area 14 asserts the admin-only user inventory boundary (registered-users.spec.md)
# and canonical gateway-phase enforcement (gateway-phase-vocabulary.spec.md). Long only.

echo ""
e2e_area "14. Admin Inventory + API Validation"
echo ""

if ! e2e_step long; then
  dim "  Skipped (E2E_MODE=${E2E_MODE}): area 14 uses the developer and platform-admin identities"
else
  # ── 14a. User inventory is admin-only ──
  if acquire_oidc_token "$E2E_DEV_USERNAME" "$E2E_DEV_PASSWORD"; then
    DEV_USERS_TOKEN="${_OIDC_ACCESS_TOKEN}"
  else
    DEV_USERS_TOKEN=""
    fail_test "Could not acquire a developer API token for the /users boundary check"
  fi
  if [[ -n "$DEV_USERS_TOKEN" ]]; then
    show_cmd "curl .../v1/users (as developer) -> expect 403"
    DEV_USERS_CODE=$(_driver_curl -o /dev/null -w '%{http_code}' \
      -H "Authorization: Bearer ${DEV_USERS_TOKEN}" "${API_HOST}/api/hypershell/v1/users" 2>/dev/null || true)
    if [[ "$DEV_USERS_CODE" == "403" ]]; then
      pass "Developer GET /v1/users is 403 Forbidden (admin-only inventory)"
    else
      fail_test "Developer GET /v1/users expected 403, got ${DEV_USERS_CODE:-none}"
    fi
  fi

  # platform-admin: 200 with an accurate total at size=1, and a sample user id.
  if acquire_oidc_token "$E2E_PLATFORM_ADMIN_USERNAME" "$E2E_PLATFORM_ADMIN_PASSWORD"; then
    PADMIN_USERS_TOKEN="${_OIDC_ACCESS_TOKEN}"
  else
    PADMIN_USERS_TOKEN=""
    fail_test "Could not acquire a platform-admin API token for the /users inventory check"
  fi
  SAMPLE_USER_ID=""
  if [[ -n "$PADMIN_USERS_TOKEN" ]]; then
    show_cmd "curl .../v1/users?size=1 (as platform-admin) -> expect 200 + total"
    PADMIN_USERS_FILE=$(mktemp)
    PADMIN_USERS_CODE=$(_driver_curl -o "$PADMIN_USERS_FILE" -w '%{http_code}' \
      -H "Authorization: Bearer ${PADMIN_USERS_TOKEN}" "${API_HOST}/api/hypershell/v1/users?size=1" 2>/dev/null || true)
    PADMIN_USERS_TOTAL=$(e2e_json_field total < "$PADMIN_USERS_FILE")
    SAMPLE_USER_ID=$(python3 -c "import json,sys
try:
    items=json.load(open(sys.argv[1])).get('items',[])
    print(items[0].get('id','') if items else '')
except Exception: pass" "$PADMIN_USERS_FILE" 2>/dev/null)
    rm -f "$PADMIN_USERS_FILE"
    if [[ "$PADMIN_USERS_CODE" == "200" && -n "$PADMIN_USERS_TOTAL" && "$PADMIN_USERS_TOTAL" -ge 1 ]]; then
      pass "Platform-admin GET /v1/users?size=1 is 200 with an accurate total (${PADMIN_USERS_TOTAL})"
    else
      fail_test "Platform-admin GET /v1/users?size=1 expected 200 with a total, got ${PADMIN_USERS_CODE:-none} total=${PADMIN_USERS_TOTAL:-none}"
    fi
  fi

  # Get-by-id for a non-admin is an opaque 404 (not a 403 that reveals existence).
  if [[ -n "$DEV_USERS_TOKEN" && -n "$SAMPLE_USER_ID" ]]; then
    show_cmd "curl .../v1/users/${SAMPLE_USER_ID} (as developer) -> expect opaque 404"
    DEV_GETID_CODE=$(_driver_curl -o /dev/null -w '%{http_code}' \
      -H "Authorization: Bearer ${DEV_USERS_TOKEN}" "${API_HOST}/api/hypershell/v1/users/${SAMPLE_USER_ID}" 2>/dev/null || true)
    if [[ "$DEV_GETID_CODE" == "404" ]]; then
      pass "Developer GET /v1/users/{id} is an opaque 404 (does not reveal existence)"
    else
      fail_test "Developer GET /v1/users/{id} expected opaque 404, got ${DEV_GETID_CODE:-none}"
    fi
  fi

  # Restore the admin token for the phase check.
  acquire_oidc_token 2>/dev/null || true

  # ── 14b. Unknown gateway phase rejected ──
  # Any existing gateway serves as the target; the PATCH is rejected before any
  # write, so the stored phase is unchanged.
  PHASE_GW_JSON=$(api_curl "${API_HOST}/api/hypershell/v1/gateways?size=1" 2>/dev/null || true)
  PHASE_GW_ID=$(echo "$PHASE_GW_JSON" | python3 -c "import json,sys
try:
    items=json.load(sys.stdin).get('items',[])
    print(items[0].get('id','') if items else '')
except Exception: pass" 2>/dev/null)
  if [[ -z "$PHASE_GW_ID" ]]; then
    dim "  Skipped unknown-phase check: no gateway available to target"
  else
    PHASE_BEFORE=$(api_curl "${API_HOST}/api/hypershell/v1/gateways/${PHASE_GW_ID}" 2>/dev/null | e2e_json_field phase)
    show_cmd "api_curl -X PATCH .../gateways/${PHASE_GW_ID} -d '{phase: Bogus}'  # expect 400"
    PHASE_FILE=$(mktemp)
    PHASE_CODE=$(api_curl -o "$PHASE_FILE" -w '%{http_code}' -X PATCH "${API_HOST}/api/hypershell/v1/gateways/${PHASE_GW_ID}" \
      -H "Content-Type: application/json" -d '{"phase":"Bogus-e2e-phase"}' 2>/dev/null || true)
    PHASE_REASON=$(e2e_json_field reason < "$PHASE_FILE")
    rm -f "$PHASE_FILE"
    if [[ "$PHASE_CODE" == "400" && "$PHASE_REASON" == *phase* ]]; then
      pass "An unknown gateway phase is rejected (HTTP 400, names the invalid phase)"
    else
      fail_test "Unknown phase expected 400 naming phase, got ${PHASE_CODE:-none} (${PHASE_REASON:0:80})"
    fi
    PHASE_AFTER=$(api_curl "${API_HOST}/api/hypershell/v1/gateways/${PHASE_GW_ID}" 2>/dev/null | e2e_json_field phase)
    if [[ "$PHASE_AFTER" == "$PHASE_BEFORE" ]]; then
      pass "Stored gateway phase is unchanged after the rejected write (${PHASE_AFTER})"
    else
      fail_test "Stored gateway phase changed after a rejected write (${PHASE_BEFORE} -> ${PHASE_AFTER})"
    fi
  fi
fi
sep

# ── results ───────────────────────────────────────────────────────────────

# Reached every planned area without a fatal abort; cleanup's EXIT trap prints
# the results (see cleanup()), so print_results itself is not called here.
E2E_COMPLETED=1

if [[ $E2E_FAIL -gt 0 ]]; then
  exit 1
fi
