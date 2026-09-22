#!/usr/bin/env bash
# Seed the platform's baseline resources (ManagedCluster, GatewayRelease,
# Gateway) into a running Kind cluster via the REST API. Gateway databases need
# no seeding: the control plane provisions them on the server named by the
# hypershell-gateway-database-admin Secret that up.sh creates.
#
# Split out of up.sh so it can run AFTER the component image swap in CI, against
# the working-tree image rather than the baseline placeholder image kind-up
# deploys first. Seeding a request-contract change (e.g. a new/removed required
# field) against the stale baseline image would 400 and abort; running it here,
# once the swapped-in images are live, exercises the branch's own contract.
#
# Local `make kind-up` invokes this inline by default. CI defers it: it sets
# SKIP_SEED=true on kind-up and runs `make kind-seed` after the swap.
#
# Environment:
#   KIND_SEED_STRICT    when "true", a seeding failure exits non-zero instead of
#                       only warning. CI sets this so a contract regression fails
#                       the job at the seed step with the real HTTP error, rather
#                       than surfacing later as a confusing discovery failure.
#                       KIND_SEED_STRICT remains an alias.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "${SCRIPT_DIR}/lib.sh"

require_cluster


# --- Seed Gateway via REST API ---
header "Gateway Provisioning"
PF_LOG="$(mktemp -t hypershell-kind-seed-pf.XXXXXX)"
PF_PID=""
LOCAL_PORT=""

# A random local port avoids colliding with a fixed :8000 that some other tool
# or long-lived tunnel already holds on this machine. A collision on a fixed
# port would make kubectl fail to bind; that failure is not merely cosmetic,
# because it used to be discarded (>/dev/null) and the readiness loop below
# would then treat a response from that OTHER, unrelated listener as proof the
# real API server was reachable. Every following request would silently talk
# to the wrong backend and fail with a misleading 401 (see the port-forward log
# on failure below).
start_api_port_forward() {
  LOCAL_PORT=$(( (RANDOM % 20000) + 20000 ))
  API_URL="http://localhost:${LOCAL_PORT}"
  : >"${PF_LOG}"
  kube port-forward svc/hypershell-api-server -n "${KIND_NAMESPACE}" "${LOCAL_PORT}:8000" >"${PF_LOG}" 2>&1 &
  PF_PID=$!
}

cleanup_pf() {
  kill "${PF_PID}" 2>/dev/null || true
  wait "${PF_PID}" 2>/dev/null || true
  rm -f "${PF_LOG}"
}
trap cleanup_pf EXIT

info "Port-forwarding to API server..."
start_api_port_forward

# `port-forward` accepts a local TCP connection before it has confirmed the pod
# is serving, so a fixed `sleep` races the REST server coming up. Confirm
# kubectl's own "Forwarding from" line (proof this process bound the local
# port) before trusting an HTTP response on it. A dead process (bind failure,
# broken pipe) is restarted with a fresh random port rather than retried on the
# same one.
info "Waiting for API server to answer through the port-forward..."
api_reachable=""
for _ in $(seq 1 30); do
  if ! kill -0 "${PF_PID}" 2>/dev/null; then
    start_api_port_forward
    sleep 1
    continue
  fi
  if grep -q "^Forwarding from" "${PF_LOG}" 2>/dev/null &&
    curl -s -o /dev/null -m 3 "${API_URL}/api/hypershell/v1/gateways" 2>/dev/null; then
    api_reachable=true
    break
  fi
  sleep 1
done
if [[ -z "${api_reachable}" ]]; then
  warn "API server did not answer through the port-forward; seeding may fail"
  warn "port-forward log: $(cat "${PF_LOG}" 2>/dev/null)"
fi

# Keycloak --import-realm does not update existing users when keycloak.yaml
# gains new realm roles. Reconcile before minting tokens so admin/admin carries
# platform:admin for dashboard access (OP-DASH-04).
reconcile_keycloak_seed_users

# Obtain a Bearer token from Keycloak for API calls.
API_AUTH_HEADER=""
info "Obtaining API token from Keycloak..."
# Use the Gateway-routed Keycloak URL instead of port-forwarding.
# Keycloak is accessible via HTTPRoute at keycloak.hypershell.localhost.
#
# Seed with the admin resource-owner (password) token, NOT the control-plane
# client-credentials token. The kind overlay enables RBAC_ENFORCE=true, and the
# HTTP authz middleware (unlike the gRPC interceptor) has no service-account
# bypass -- every write requires the caller's JWT to carry the `gateway:creator`
# realm role. The `hypershell-control-plane` client holds no such role, so its
# token 403s on `POST /managed_clusters` onward and (because seeding is non-fatal) would
# leave the cluster with no seeded resources behind a scroll-past warning. The
# `admin` user has `gateway:creator`, and `hypershell-frontend` permits the
# password grant (publicClient + directAccessGrantsEnabled), so this token is
# authorized to create the platform resources below.
#
# Poll rather than fetching once. On a fresh `kind-up` the gateway LB has an
# address (waited on above) and Keycloak is Available, but the gateway's
# Keycloak route/listener may not be accepting on :443 yet -- a single curl
# then fails with (7) "Couldn't connect to server", the token is empty, and
# seeding proceeds unauthenticated (HTTP 401). Re-running `kind-up` "fixes" it
# only because everything is warm by then. Retry until Keycloak answers with a
# token (or we time out) so the first run seeds successfully. Mirrors the
# API-server port-forward readiness loop above.
KC_TOKEN_URL="https://${KEYCLOAK_HOSTNAME}/realms/hypershell/protocol/openid-connect/token"
API_TOKEN=""
TOKEN_RESP=""
for _ in $(seq 1 30); do
  TOKEN_RESP=$(curl -sSk -m 5 -X POST "${KC_TOKEN_URL}" \
    -d "grant_type=password" \
    -d "client_id=hypershell-frontend" \
    -d "username=admin" \
    -d "password=admin" 2>&1 || true)
  API_TOKEN=$(echo "${TOKEN_RESP}" | grep -o '"access_token":"[^"]*"' | cut -d'"' -f4 || true)
  if [[ -n "${API_TOKEN}" ]]; then break; fi
  sleep 2
done
if [[ -n "${API_TOKEN}" ]]; then
  API_AUTH_HEADER="Authorization: Bearer ${API_TOKEN}"
  success "API token obtained"
else
  warn "Could not obtain API token: ${TOKEN_RESP:0:200}"
fi

# Helper: POST a JSON resource; prints the response body on success or failure.
api_post() {
  local url="$1" data="$2"
  local auth_args=()
  if [[ -n "${API_AUTH_HEADER}" ]]; then
    auth_args=(-H "${API_AUTH_HEADER}")
  fi
  curl -sS -w "\n%{http_code}" -X POST "${url}" \
    -H "Content-Type: application/json" \
    ${auth_args[@]+"${auth_args[@]}"} \
    -d "${data}" 2>&1 || true
}

api_get() {
  local url="$1"
  local auth_args=()
  if [[ -n "${API_AUTH_HEADER}" ]]; then
    auth_args=(-H "${API_AUTH_HEADER}")
  fi
  curl -sS -w "\n%{http_code}" -X GET "${url}" \
    ${auth_args[@]+"${auth_args[@]}"} 2>&1 || true
}

extract_id() {
  local resp="$1"
  if echo "$resp" | grep -q '"kind":"Error"'; then
    echo ""
    return
  fi
  local id
  id=$(echo "$resp" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4 || true)
  echo "${id}"
}

seed_failed=""
CLUSTER_ID=""
RELEASE_ID=""

if [[ -z "${seed_failed}" ]]; then
  # Check for existing ManagedCluster
  info "Checking for existing local-kind ManagedCluster..."
  EXISTING_MC_RAW=$(api_get "${API_URL}/api/hypershell/v1/managed_clusters")
  EXISTING_MC_HTTP=$(echo "${EXISTING_MC_RAW}" | tail -1)
  EXISTING_MC_RESP=$(echo "${EXISTING_MC_RAW}" | sed '$d')

  if [[ "${EXISTING_MC_HTTP}" == "200" ]]; then
    CLUSTER_ID=$(printf '%s' "${EXISTING_MC_RESP}" | json_named_id local-kind)
    if [[ -n "${CLUSTER_ID}" ]]; then
      success "local-kind ManagedCluster already exists: ${CLUSTER_ID}"
    fi
  fi

  if [[ -z "${CLUSTER_ID}" ]]; then
    info "Creating ManagedCluster..."
    _mc_body="{\"name\":\"local-kind\",\"provider\":\"kind\",\"kubeconfig_secret\":\"kind-kubeconfig\",\"region\":\"kind-local\"}"
    MC_RAW=$(api_post "${API_URL}/api/hypershell/v1/managed_clusters" "${_mc_body}")
    MC_HTTP=$(echo "${MC_RAW}" | tail -1)
    MC_RESP=$(echo "${MC_RAW}" | sed '$d')
    CLUSTER_ID=$(extract_id "${MC_RESP}")

    if [[ -z "${CLUSTER_ID}" ]]; then
      warn "ManagedCluster creation failed (HTTP ${MC_HTTP}): ${MC_RESP:-no response}"
      seed_failed=true
    else
      success "ManagedCluster created: ${CLUSTER_ID}"
    fi
  fi
fi

if [[ -z "${seed_failed}" ]]; then
  # Check for existing GatewayRelease
  info "Checking for existing dev-release GatewayRelease..."
  EXISTING_GR_RAW=$(api_get "${API_URL}/api/hypershell/v1/gateway_releases")
  EXISTING_GR_HTTP=$(echo "${EXISTING_GR_RAW}" | tail -1)
  EXISTING_GR_RESP=$(echo "${EXISTING_GR_RAW}" | sed '$d')

  if [[ "${EXISTING_GR_HTTP}" == "200" ]]; then
    RELEASE_ID=$(printf '%s' "${EXISTING_GR_RESP}" | json_named_id dev-release)
    if [[ -n "${RELEASE_ID}" ]]; then
      success "dev-release GatewayRelease already exists: ${RELEASE_ID}"
    fi
  fi

  if [[ -z "${RELEASE_ID}" ]]; then
    info "Creating GatewayRelease..."
    GR_RAW=$(api_post "${API_URL}/api/hypershell/v1/gateway_releases" \
      "{\"name\":\"dev-release\",\"image\":\"${GATEWAY_IMAGE}\"}")
    GR_HTTP=$(echo "${GR_RAW}" | tail -1)
    GR_RESP=$(echo "${GR_RAW}" | sed '$d')
    RELEASE_ID=$(extract_id "${GR_RESP}")

    if [[ -z "${RELEASE_ID}" ]]; then
      warn "GatewayRelease creation failed (HTTP ${GR_HTTP}): ${GR_RESP:-no response}"
      seed_failed=true
    else
      success "GatewayRelease created: ${RELEASE_ID}"
    fi
  fi
fi

if [[ -z "${seed_failed}" ]]; then
  # Check if dev-gateway already exists before creating
  info "Checking for existing dev-gateway..."
  GATEWAY_ID=""
  EXISTING_GW_RAW=$(api_get "${API_URL}/api/hypershell/v1/gateways")
  EXISTING_GW_HTTP=$(echo "${EXISTING_GW_RAW}" | tail -1)
  EXISTING_GW_RESP=$(echo "${EXISTING_GW_RAW}" | sed '$d')

  if [[ "${EXISTING_GW_HTTP}" == "200" ]]; then
    EXISTING_GW_ID=$(printf '%s' "${EXISTING_GW_RESP}" | json_named_id dev-gateway)
    if [[ -n "${EXISTING_GW_ID}" ]]; then
      success "dev-gateway already exists: ${EXISTING_GW_ID}"
      GATEWAY_ID="${EXISTING_GW_ID}"
    fi
  fi

  if [[ -z "${GATEWAY_ID}" ]]; then
    info "Creating Gateway with OIDC..."
    OIDC_JSON="{\\\"issuer\\\":\\\"${KEYCLOAK_OIDC_ISSUER}\\\",\\\"audience\\\":\\\"${KEYCLOAK_OIDC_AUDIENCE}\\\",\\\"roles_claim\\\":\\\"groups\\\",\\\"admin_role\\\":\\\"hypershell-admins\\\",\\\"user_role\\\":\\\"hypershell-users\\\"}"
    # namespace is server-derived (BeforeCreate sets openshell-<hex> from the ksuid);
    # sending it is rejected as an unknown field (ErrorMalformedRequest / id 17).
    GW_BODY="{\"name\":\"dev-gateway\",\"cluster_id\":\"${CLUSTER_ID}\",\"release_id\":\"${RELEASE_ID}\",\"oidc\":\"${OIDC_JSON}\""
    GW_BODY="${GW_BODY},\"route\":\"{\\\"enabled\\\":true}\""
    GW_BODY="${GW_BODY}}"
    GW_RAW=$(api_post "${API_URL}/api/hypershell/v1/gateways" "${GW_BODY}")
    GW_HTTP=$(echo "${GW_RAW}" | tail -1)
    GW_RESP=$(echo "${GW_RAW}" | sed '$d')
    GATEWAY_ID=$(extract_id "${GW_RESP}")

    if [[ -z "${GATEWAY_ID}" ]]; then
      warn "Gateway creation failed (HTTP ${GW_HTTP}): ${GW_RESP:-no response}"
    else
      success "Gateway created with OIDC: ${GATEWAY_ID}"
    fi
  fi
fi

if [[ -n "${seed_failed}" ]]; then
  warn "Automatic seeding incomplete - create resources manually after API server is ready"
fi

cleanup_pf
trap - EXIT
echo ""
if [[ -n "${seed_failed}" ]] && seed_strict; then
  error "Platform seeding failed and SEED_STRICT=true - failing"
  exit 1
fi