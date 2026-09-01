#!/usr/bin/env bash
# Seed the platform's baseline resources (ManagedCluster, GatewayRelease,
# ManagedDatabase, Gateway) into a running Kind cluster via the REST API.
#
# Split out of up.sh so it can run AFTER the component image swap in CI, against
# the working-tree image rather than the baseline placeholder image kind-up
# deploys first. Seeding a request-contract change (e.g. a new/removed required
# field) against the stale baseline image would 400 and abort; running it here,
# once the swapped-in images are live, exercises the branch's own contract.
#
# Local `make kind-up` invokes this inline by default. CI defers it: it sets
# KIND_SKIP_SEED=true on kind-up and runs `make kind-seed` after the swap.
#
# Environment:
#   DATABASE_PROVIDER   cnpg | deployment (default: deployment). Must match the
#                       provider kind-up provisioned infrastructure for.
#   KIND_SEED_STRICT    when "true", a seeding failure exits non-zero instead of
#                       only warning. CI sets this so a contract regression fails
#                       the job at the seed step with the real HTTP error, rather
#                       than surfacing later as a confusing discovery failure.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "${SCRIPT_DIR}/lib.sh"

require_cluster

# DATABASE_PROVIDER unset/empty means "deployment" (mirrors up.sh).
DB_PROVIDER="${DATABASE_PROVIDER:-deployment}"
if [[ "${DB_PROVIDER}" != "cnpg" && "${DB_PROVIDER}" != "deployment" ]]; then
  error "DATABASE_PROVIDER must be 'cnpg' or 'deployment', got '${DB_PROVIDER}'"
  exit 1
fi

# --- Seed Gateway via REST API ---
header "Gateway Provisioning"
API_URL="http://localhost:8000"
info "Port-forwarding to API server..."
kube port-forward svc/hypershell-api-server -n "${KIND_NAMESPACE}" 8000:8000 >/dev/null 2>&1 &
PF_PID=$!
cleanup_pf() { kill "${PF_PID}" 2>/dev/null || true; wait "${PF_PID}" 2>/dev/null || true; }
trap cleanup_pf EXIT

# `port-forward` accepts a local TCP connection before it has confirmed the pod
# is serving, so a fixed `sleep` races the REST server coming up. Poll until the
# API answers with *any* HTTP status -- a 401/403 without a token still proves
# the server responded (curl exits 0). An empty reply / dead forward makes curl
# exit non-zero (HTTP 000), so tear the forward down and re-establish it before
# retrying.
info "Waiting for API server to answer through the port-forward..."
api_reachable=""
for _ in $(seq 1 30); do
  if curl -s -o /dev/null -m 3 "${API_URL}/api/hypershell/v1/gateways" 2>/dev/null; then
    api_reachable=true
    break
  fi
  kill "${PF_PID}" 2>/dev/null || true
  wait "${PF_PID}" 2>/dev/null || true
  kube port-forward svc/hypershell-api-server -n "${KIND_NAMESPACE}" 8000:8000 >/dev/null 2>&1 &
  PF_PID=$!
  sleep 2
done
if [[ -z "${api_reachable}" ]]; then
  warn "API server did not answer through the port-forward; seeding may fail"
fi

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

# platform-admin token for GatewayProfile operations (requires platform:admin realm role).
PLATFORM_ADMIN_AUTH_HEADER=""
info "Obtaining platform-admin token..."
for _ in $(seq 1 5); do
  PLATFORM_ADMIN_TOKEN_RESP=$(curl -sSk -m 5 -X POST "${KC_TOKEN_URL}" \
    -d "grant_type=password" \
    -d "client_id=hypershell-frontend" \
    -d "username=platform-admin" \
    -d "password=platform-admin" 2>&1 || true)
  PLATFORM_ADMIN_TOKEN=$(echo "${PLATFORM_ADMIN_TOKEN_RESP}" | grep -o '"access_token":"[^"]*"' | cut -d'"' -f4 || true)
  if [[ -n "${PLATFORM_ADMIN_TOKEN}" ]]; then
    PLATFORM_ADMIN_AUTH_HEADER="Authorization: Bearer ${PLATFORM_ADMIN_TOKEN}"
    success "Platform-admin token obtained"
    break
  fi
  sleep 2
done
if [[ -z "${PLATFORM_ADMIN_AUTH_HEADER}" ]]; then
  warn "Could not obtain platform-admin token; GatewayProfile seeding will be skipped"
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

api_patch() {
  local url="$1" data="$2"
  local auth_args=()
  if [[ -n "${API_AUTH_HEADER}" ]]; then
    auth_args=(-H "${API_AUTH_HEADER}")
  fi
  curl -sS -w "\n%{http_code}" -X PATCH "${url}" \
    -H "Content-Type: application/json" \
    ${auth_args[@]+"${auth_args[@]}"} \
    -d "${data}" 2>&1 || true
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
DATABASE_ID=""
SMALL_PROFILE_ID=""

if [[ -z "${seed_failed}" ]]; then
  # Check for existing ManagedCluster
  info "Checking for existing local-kind ManagedCluster..."
  EXISTING_MC_RAW=$(api_get "${API_URL}/api/hypershell/v1/managed_clusters")
  EXISTING_MC_HTTP=$(echo "${EXISTING_MC_RAW}" | tail -1)
  EXISTING_MC_RESP=$(echo "${EXISTING_MC_RAW}" | sed '$d')

  if [[ "${EXISTING_MC_HTTP}" == "200" ]]; then
    CLUSTER_ID=$(echo "${EXISTING_MC_RESP}" | grep -o '"id":"[^"]*"[^}]*"name":"local-kind"' | grep -o '"id":"[^"]*"' | cut -d'"' -f4 | head -1 || true)
    if [[ -n "${CLUSTER_ID}" ]]; then
      success "local-kind ManagedCluster already exists: ${CLUSTER_ID}"
    fi
  fi

  if [[ -z "${CLUSTER_ID}" ]]; then
    info "Creating ManagedCluster..."
    MC_RAW=$(api_post "${API_URL}/api/hypershell/v1/managed_clusters" \
      "{\"name\":\"local-kind\",\"provider\":\"kind\",\"kubeconfig_secret\":\"kind-kubeconfig\"}")
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

if [[ -z "${seed_failed}" && -n "${PLATFORM_ADMIN_AUTH_HEADER}" ]]; then
  header "GatewayProfile Seeding"

  seed_profile() {
    local prof_name="$1" prof_body="$2"
    info "Creating ${prof_name} GatewayProfile..." >&2
    local raw http resp id
    raw=$(curl -sS -w "\n%{http_code}" -X POST "${API_URL}/api/hypershell/v1/gateway_profiles" \
      -H "Content-Type: application/json" \
      -H "${PLATFORM_ADMIN_AUTH_HEADER}" \
      -d "${prof_body}" 2>&1 || true)
    http=$(echo "${raw}" | tail -1)
    resp=$(echo "${raw}" | sed '$d')
    id=$(extract_id "${resp}")
    if [[ -n "${id}" ]]; then
      success "${prof_name} GatewayProfile created: ${id}" >&2
      printf '%s' "${id}"
      return
    fi
    warn "${prof_name} GatewayProfile creation returned HTTP ${http}; looking up existing..." >&2
    local list_raw list_resp list_http
    list_raw=$(curl -sS -w "\n%{http_code}" -X GET "${API_URL}/api/hypershell/v1/gateway_profiles" \
      -H "${PLATFORM_ADMIN_AUTH_HEADER}" 2>&1 || true)
    list_http=$(echo "${list_raw}" | tail -1)
    list_resp=$(echo "${list_raw}" | sed '$d')
    if [[ "${list_http}" == "200" ]]; then
      id=$(echo "${list_resp}" | grep -o "\"name\":\"${prof_name}\"[^}]*\"id\":\"[^\"]*\"" | grep -o '"id":"[^"]*"' | cut -d'"' -f4 | head -1 || true)
      if [[ -z "${id}" ]]; then
        id=$(echo "${list_resp}" | grep -o "\"id\":\"[^\"]*\"[^}]*\"name\":\"${prof_name}\"" | grep -o '"id":"[^"]*"' | cut -d'"' -f4 | head -1 || true)
      fi
    fi
    if [[ -n "${id}" ]]; then
      success "${prof_name} GatewayProfile found: ${id}" >&2
      printf '%s' "${id}"
    else
      warn "${prof_name} GatewayProfile not found" >&2
    fi
  }

  SMALL_PROFILE_ID=$(seed_profile "small" \
    '{"name":"small","description":"Small: 2 CPU / 4Gi RAM namespace quota","cpu_request_total":"2","cpu_limit_total":"4","memory_request_total":"2Gi","memory_limit_total":"4Gi","ephemeral_storage_total":"10Gi","pod_count":20,"pvc_count":5,"container_cpu_request_default":"100m","container_cpu_limit_max":"1","container_memory_request_default":"128Mi","container_memory_limit_max":"1Gi"}')
  seed_profile "medium" \
    '{"name":"medium","description":"Medium: 4 CPU / 8Gi RAM namespace quota","cpu_request_total":"4","cpu_limit_total":"8","memory_request_total":"4Gi","memory_limit_total":"8Gi","ephemeral_storage_total":"20Gi","pod_count":40,"pvc_count":10,"container_cpu_request_default":"200m","container_cpu_limit_max":"2","container_memory_request_default":"256Mi","container_memory_limit_max":"2Gi"}' >/dev/null
  seed_profile "big" \
    '{"name":"big","description":"Big: 8 CPU / 16Gi RAM namespace quota","cpu_request_total":"8","cpu_limit_total":"16","memory_request_total":"8Gi","memory_limit_total":"16Gi","ephemeral_storage_total":"40Gi","pod_count":80,"pvc_count":20,"container_cpu_request_default":"400m","container_cpu_limit_max":"4","container_memory_request_default":"512Mi","container_memory_limit_max":"4Gi"}' >/dev/null

  if [[ -n "${SMALL_PROFILE_ID}" && -n "${CLUSTER_ID}" ]]; then
    info "Setting small GatewayProfile as default on ManagedCluster ${CLUSTER_ID}..."
    MC_PATCH_RAW=$(api_patch "${API_URL}/api/hypershell/v1/managed_clusters/${CLUSTER_ID}" \
      "{\"profile_id\":\"${SMALL_PROFILE_ID}\"}")
    MC_PATCH_HTTP=$(echo "${MC_PATCH_RAW}" | tail -1)
    if [[ "${MC_PATCH_HTTP}" == "200" ]]; then
      success "ManagedCluster profile_id set to ${SMALL_PROFILE_ID}"
    else
      MC_PATCH_RESP=$(echo "${MC_PATCH_RAW}" | sed '$d')
      warn "Failed to set profile_id on ManagedCluster (HTTP ${MC_PATCH_HTTP}): ${MC_PATCH_RESP:-no response}"
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
    RELEASE_ID=$(echo "${EXISTING_GR_RESP}" | grep -o '"id":"[^"]*"[^}]*"name":"dev-release"' | grep -o '"id":"[^"]*"' | cut -d'"' -f4 | head -1 || true)
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
  if [[ "${DB_PROVIDER}" == "deployment" ]]; then
    info "Skipping ManagedDatabase seed - deployment mode auto-creates per-gateway databases"
    DATABASE_ID=""
  else
    # Check for existing openshell-db ManagedDatabase
    info "Checking for existing openshell-db ManagedDatabase..."
    EXISTING_MD_RAW=$(api_get "${API_URL}/api/hypershell/v1/managed_databases")
    EXISTING_MD_HTTP=$(echo "${EXISTING_MD_RAW}" | tail -1)
    EXISTING_MD_RESP=$(echo "${EXISTING_MD_RAW}" | sed '$d')

    if [[ "${EXISTING_MD_HTTP}" == "200" ]]; then
      DATABASE_ID=$(echo "${EXISTING_MD_RESP}" | grep -o '"id":"[^"]*"[^}]*"name":"openshell-db"' | grep -o '"id":"[^"]*"' | cut -d'"' -f4 | head -1 || true)
      if [[ -n "${DATABASE_ID}" ]]; then
        success "openshell-db ManagedDatabase already exists: ${DATABASE_ID}"
      fi
    fi

    if [[ -z "${DATABASE_ID}" ]]; then
      info "Creating ManagedDatabase (provider=${DB_PROVIDER})..."
      MD_RAW=$(api_post "${API_URL}/api/hypershell/v1/managed_databases" \
        "{\"name\":\"openshell-db\",\"provider\":\"${DB_PROVIDER}\"}")
      MD_HTTP=$(echo "${MD_RAW}" | tail -1)
      MD_RESP=$(echo "${MD_RAW}" | sed '$d')

      if [[ "${MD_HTTP}" != "201" && "${MD_HTTP}" != "200" ]]; then
        warn "ManagedDatabase creation failed (HTTP ${MD_HTTP}): ${MD_RESP:-no response}"
        seed_failed=true
      else
        DATABASE_ID=$(extract_id "${MD_RESP}")
        if [[ -z "${DATABASE_ID}" ]]; then
          warn "ManagedDatabase creation returned success but no ID: ${MD_RESP:-no response}"
          seed_failed=true
        else
          success "ManagedDatabase created: ${DATABASE_ID}"
        fi
      fi
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
    EXISTING_GW_ID=$(echo "${EXISTING_GW_RESP}" | grep -o '"id":"[^"]*"[^}]*"name":"dev-gateway"' | grep -o '"id":"[^"]*"' | cut -d'"' -f4 | head -1 || true)
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
    # Always send database_id; deployment mode uses the empty placeholder.
    GW_BODY="{\"name\":\"dev-gateway\",\"cluster_id\":\"${CLUSTER_ID}\",\"release_id\":\"${RELEASE_ID}\",\"oidc\":\"${OIDC_JSON}\""
    GW_BODY="${GW_BODY},\"database_id\":\"${DATABASE_ID}\""
    if [[ -n "${SMALL_PROFILE_ID}" ]]; then
      GW_BODY="${GW_BODY},\"profile_id\":\"${SMALL_PROFILE_ID}\""
    fi
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
if [[ -n "${seed_failed}" && "${KIND_SEED_STRICT:-}" == "true" ]]; then
  error "Platform seeding failed and KIND_SEED_STRICT=true - failing"
  exit 1
fi
