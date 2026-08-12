#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "${SCRIPT_DIR}/lib.sh"

header "HyperShell Local Development Environment"
echo ""

# --- Early sudo acquisition ---
# Several steps need elevated privileges (cloud-provider-kind, DNS resolver,
# port forwarding). Prompt once now so the rest of the script runs unattended.
if [[ "${KIND_NO_SUDO:-}" == "true" ]]; then
  warn "KIND_NO_SUDO=true - will use kubectl port-forward if cloud-provider-kind needs sudo"
  HAVE_SUDO=false
else
  info "This script may use sudo for cloud-provider-kind, DNS resolver, and port forwarding."
  info "Set KIND_NO_SUDO=true to skip sudo (services will use kubectl port-forward instead)."
  if sudo -v 2>/dev/null; then
    HAVE_SUDO=true
    success "sudo credentials cached"
  else
    warn "sudo unavailable - will use kubectl port-forward as fallback"
    HAVE_SUDO=false
  fi
fi
echo ""

# --- Podman + kind compatibility check ---
# kind v0.32.0 has a ListClusters bug with podman 6+ (kubernetes-sigs/kind#4231).
# Build patched binaries into ./bin/ automatically if needed.
if [[ "$(basename "${CONTAINER_ENGINE}")" == "podman" ]]; then
  kind_ver="$(kind version 2>/dev/null | grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+' || true)"
  if [[ "${kind_ver}" == "v0.32.0" ]]; then
    warn "kind ${kind_ver} is incompatible with podman 6+ (kubernetes-sigs/kind#4231)"
    info "Building patched cloud-provider-kind into ./bin/..."
    make -C "${REPO_ROOT}" kind-prereqs
    export PATH="${REPO_ROOT}/bin:${PATH}"
  fi
fi

# --- Cluster creation (idempotent) ---
header "Cluster"
if cluster_exists; then
  warn "Cluster '${KIND_CLUSTER_NAME}' already exists, reusing"
else
  info "Creating Kind cluster '${KIND_CLUSTER_NAME}'..."
  rendered=$(mktemp /tmp/kind-config-XXXXXX)
  sed "s|__KIND_HOST_MOUNT_PATH__|${KIND_HOST_MOUNT_PATH:-$(git rev-parse --show-toplevel 2>/dev/null || pwd)}|g" \
    "${KIND_CONFIG}" > "${rendered}"
  kind create cluster --name "${KIND_CLUSTER_NAME}" --config "${rendered}"
  rm -f "${rendered}"
  success "Cluster created"
fi

# --- Set kubectl context and wait for API server ---
kube cluster-info >/dev/null 2>&1 || \
  kubectl config use-context "$(kctx)"

info "Waiting for Kubernetes API server..."
for i in $(seq 1 15); do
  if kube get nodes >/dev/null 2>&1; then break; fi
  info "API server not ready, retrying (${i}/15)..."
  sleep 2
done
echo ""

# --- Verify and start cloud-provider-kind ---
header "cloud-provider-kind"
CPK_RUNNING=false
if ! command -v cloud-provider-kind >/dev/null 2>&1; then
  if [[ "${HAVE_SUDO}" == "true" ]]; then
    error "cloud-provider-kind not found in PATH"
    info "Install via: make kind-prereqs"
    exit 1
  else
    warn "cloud-provider-kind not found - will use kubectl port-forward instead"
  fi
elif pgrep -f "cloud-provider-kind" >/dev/null 2>&1; then
  warn "cloud-provider-kind already running"
  CPK_RUNNING=true
else
  info "Starting cloud-provider-kind..."
  if nohup cloud-provider-kind --enable-lb-port-mapping >/tmp/cloud-provider-kind.log 2>&1 &
     sleep 2 && pgrep -f "cloud-provider-kind" >/dev/null 2>&1; then
    CPK_RUNNING=true
    success "cloud-provider-kind started (without sudo)"
  elif [[ "${HAVE_SUDO}" == "true" ]]; then
    info "Retrying with sudo..."
    sudo -E nohup cloud-provider-kind --enable-lb-port-mapping >/tmp/cloud-provider-kind.log 2>&1 &
    sleep 2
    if pgrep -f "cloud-provider-kind" >/dev/null 2>&1; then
      CPK_RUNNING=true
      success "cloud-provider-kind started (with sudo)"
    else
      error "cloud-provider-kind failed to start - check /tmp/cloud-provider-kind.log"
      exit 1
    fi
  else
    warn "cloud-provider-kind requires sudo on this system - will use kubectl port-forward instead"
  fi
fi
echo ""

# --- Install infrastructure prerequisites via kustomize ---
header "Infrastructure"
info "Installing CRDs and controllers (cert-manager, Gateway API, Agent Sandbox)..."
kustomize build --load-restrictor=LoadRestrictionsNone deploy/kind/infrastructure | \
  kube apply --server-side --force-conflicts -f -
info "Waiting for cert-manager..."
kube wait --for=condition=available deployment/cert-manager -n cert-manager --timeout=120s
kube wait --for=condition=available deployment/cert-manager-webhook -n cert-manager --timeout=120s
info "Waiting for agent-sandbox controller..."
kube wait --for=condition=available deployment/agent-sandbox-controller -n agent-sandbox-system --timeout=120s
success "Infrastructure ready"
echo ""

# --- Build and load local images (offline mode) ---
FORCE_ROLLOUT=""
if [[ "${LOCAL_IMAGES:-}" == "true" ]]; then
  header "Local Images"
  info "Building baseline images from origin/main..."
  "${SCRIPT_DIR}/build-images.sh"
  FORCE_ROLLOUT=true
  echo ""
fi

# --- Apply pull secret (if configured) ---
if [[ -n "${KIND_PULL_SECRET:-}" ]]; then
  header "Pull Secret"
  kube create namespace "${KIND_NAMESPACE}" --dry-run=client -o yaml | \
    kube apply -f -
  info "Applying pull secret from ${KIND_PULL_SECRET}..."
  kube apply -f "${KIND_PULL_SECRET}" -n "${KIND_NAMESPACE}"
  SECRET_NAME=$(kube get -f "${KIND_PULL_SECRET}" -n "${KIND_NAMESPACE}" -o jsonpath='{.metadata.name}')
  if [[ -n "${SECRET_NAME}" ]]; then
    info "Waiting for default ServiceAccount in ${KIND_NAMESPACE}..."
    for i in $(seq 1 30); do
      if kube get serviceaccount default -n "${KIND_NAMESPACE}" >/dev/null 2>&1; then break; fi
      sleep 1
    done
    info "Patching default ServiceAccount with imagePullSecrets..."
    kube patch serviceaccount default -n "${KIND_NAMESPACE}" \
      -p "{\"imagePullSecrets\":[{\"name\":\"${SECRET_NAME}\"}]}"
  fi
  echo ""
fi

# --- Deploy all components via kustomize ---
header "Deploying Components"
info "Applying Kind manifests via kustomize..."
kustomize build deploy/kind | kube apply -f -

info "Waiting for PostgreSQL..."
kube wait --for=condition=available deployment/hypershell-postgres -n "${KIND_NAMESPACE}" --timeout=300s
success "PostgreSQL ready"

if [[ -z "${KIND_KEYCLOAK_URL:-}" ]]; then
  info "Waiting for Keycloak..."
  kube wait --for=condition=available deployment/keycloak -n keycloak --timeout=180s
  success "Keycloak ready"
fi

# The controller's gRPC watch streams must connect to a running API server.
# With simultaneous deployment the controller may start before the API server
# is ready, fail the first connection, and sit in a 16s backoff -- missing
# any gateway events created during that window.  Wait for the API server
# first, then restart the controller so it connects immediately.
if ! is_swapped api-server; then
  info "Waiting for API server..."
  kube wait --for=condition=available deployment/hypershell-api-server -n "${KIND_NAMESPACE}" --timeout=120s
fi
if ! is_swapped control-plane; then
  info "Restarting control plane to establish watch streams..."
  kube rollout restart deployment/hypershell-controller -n "${KIND_NAMESPACE}"
  kube wait --for=condition=available deployment/hypershell-controller -n "${KIND_NAMESPACE}" --timeout=120s
fi

if is_swapped api-server; then
  warn "API server is swapped -- scaling to zero"
  kube scale deployment/hypershell-api-server -n "${KIND_NAMESPACE}" --replicas=0
fi

if is_swapped control-plane; then
  warn "Control plane is swapped -- scaling to zero"
  kube scale deployment/hypershell-controller -n "${KIND_NAMESPACE}" --replicas=0
fi

if is_swapped web-console; then
  warn "Web console is swapped -- scaling to zero"
  kube scale deployment/hypershell-web-console -n "${KIND_NAMESPACE}" --replicas=0
fi

local_registry="${IMAGE_REGISTRY:-quay.io/redhat-services-prod/hcm-eng-prod-tenant/hypershell-main}"
_api_img="${API_SERVER_IMAGE:-}"
_cp_img="${CONTROL_PLANE_IMAGE:-}"
_wc_img="${WEB_CONSOLE_IMAGE:-}"
if [[ "${IMAGE_TAG:-latest}" != "latest" ]]; then
  : "${_api_img:=${local_registry}/hypershell-api-server-main:${IMAGE_TAG}}"
  : "${_cp_img:=${local_registry}/hypershell-control-plane-main:${IMAGE_TAG}}"
  : "${_wc_img:=${local_registry}/hypershell-web-console-main:${IMAGE_TAG}}"
fi

if [[ -n "${_api_img}" || -n "${_cp_img}" || -n "${_wc_img}" ]]; then
  info "Overriding component images..."
  if [[ -n "${_api_img}" ]] && ! is_swapped api-server; then
    info "  api-server  -> ${_api_img}"
    kube set image "deployment/hypershell-api-server" \
      "api-server=${_api_img}" \
      "migrate=${_api_img}" \
      -n "${KIND_NAMESPACE}"
  fi
  if [[ -n "${_cp_img}" ]] && ! is_swapped control-plane; then
    info "  controller  -> ${_cp_img}"
    kube set image "deployment/hypershell-controller" \
      "controller=${_cp_img}" \
      -n "${KIND_NAMESPACE}"
  fi
  if [[ -n "${_wc_img}" ]] && ! is_swapped web-console; then
    info "  web-console -> ${_wc_img}"
    kube set image "deployment/hypershell-web-console" \
      "web-console=${_wc_img}" \
      -n "${KIND_NAMESPACE}"
  fi
fi


if [[ "${FORCE_ROLLOUT}" == "true" ]]; then
  info "Rolling out non-swapped deployments to pick up rebuilt images..."
  for pair in "hypershell-api-server:api-server" \
              "hypershell-controller:control-plane" \
              "hypershell-web-console:web-console"; do
    dep="${pair%%:*}"
    comp="${pair##*:}"
    if ! is_swapped "${comp}"; then
      kube rollout restart "deployment/${dep}" -n "${KIND_NAMESPACE}"
    fi
  done
fi
echo ""

# --- Gateway address discovery ---
header "TLS & Networking"

GATEWAY_PORT=""
KEYCLOAK_HTTP_PORT=""
if [[ "${CPK_RUNNING}" == "true" ]]; then
  info "Waiting for networking Gateway to get an address..."
  for i in $(seq 1 30); do
    GW_ADDR=$(kube get gateway hypershell-gw -n "${KIND_NAMESPACE}" \
      -o jsonpath='{.status.addresses[0].value}' 2>/dev/null || true)
    if [[ -n "${GW_ADDR}" ]]; then break; fi
    if (( i % 5 == 0 )); then
      info "Gateway not ready yet (${i}/30)... is cloud-provider-kind running?"
    fi
    sleep 2
  done

  if [[ -n "${GW_ADDR}" ]]; then
    success "Networking Gateway address: ${GW_ADDR}"

    patch_cluster_coredns "${GW_ADDR}"

    # cloud-provider-kind exposes Gateways via Docker proxy containers.
    # On macOS the container IPs are not routable, so --enable-lb-port-mapping
    # publishes an ephemeral host port.  Discover it for the banner URLs.
    info "Discovering Gateway proxy port..."
    for j in $(seq 1 15); do
      PROXY_CONTAINER=$(${CONTAINER_ENGINE} ps -q --filter "name=kindccm-gw" 2>/dev/null | head -1)
      if [[ -n "${PROXY_CONTAINER}" ]]; then
        GATEWAY_PORT=$(${CONTAINER_ENGINE} port "${PROXY_CONTAINER}" 443 2>/dev/null | head -1 | cut -d: -f2)
        KEYCLOAK_HTTP_PORT=$(${CONTAINER_ENGINE} port "${PROXY_CONTAINER}" 8080 2>/dev/null | head -1 | cut -d: -f2)
        if [[ -n "${GATEWAY_PORT}" ]]; then break; fi
      fi
      sleep 2
    done

    if [[ -n "${GATEWAY_PORT}" ]]; then
      success "Gateway HTTPS on host port ${GATEWAY_PORT}"
      if [[ -n "${KEYCLOAK_HTTP_PORT}" ]]; then
        success "Gateway HTTP (Keycloak) on host port ${KEYCLOAK_HTTP_PORT}"
      fi
      start_port_forward "${GATEWAY_PORT}" "${KEYCLOAK_HTTP_PORT:-}"
    else
      warn "Could not discover Gateway proxy port - check '${CONTAINER_ENGINE} ps --filter name=kindccm-gw'"
    fi
  else
    warn "Gateway has no address after 60s - cloud-provider-kind may not be running"
  fi
else
  info "Skipping Gateway address discovery (no cloud-provider-kind)"
  info "Services will be accessible via kubectl port-forward"
fi
echo ""

# --- OIDC configuration (opt-in, after port discovery) ---
if oidc_enabled; then
  header "OIDC Configuration"

  # Determine the external Keycloak OIDC issuer URL.  When running behind the
  # Gateway API HTTPS listener the browser reaches Keycloak on the same
  # ephemeral port as every other service.  We set KC_HOSTNAME so Keycloak
  # generates URLs that match what the browser sees.
  PORT_SUFFIX=""
  if [[ -z "${PORT_FORWARD_ACTIVE:-}" ]] && [[ -n "${GATEWAY_PORT:-}" ]]; then
    PORT_SUFFIX=":${GATEWAY_PORT}"
  fi
  OIDC_EXTERNAL_ISSUER="https://${KEYCLOAK_HOSTNAME}${PORT_SUFFIX}/realms/hypershell"

  # Route in-cluster traffic for the ephemeral port to the gateway's port 443.
  # Pods resolve keycloak.hypershell.localhost to the gateway IP via CoreDNS,
  # but the gateway only listens on 443 internally.  A PREROUTING rule inside
  # the Kind node maps the ephemeral port to 443 so the OIDC issuer URL is
  # the same for both browser and in-cluster services.
  if [[ -n "${GATEWAY_PORT:-}" ]] && [[ -n "${GW_ADDR:-}" ]] && [[ -z "${PORT_FORWARD_ACTIVE:-}" ]]; then
    info "Routing in-cluster port ${GATEWAY_PORT} to gateway port 443..."
    ${CONTAINER_ENGINE} exec "${KIND_CLUSTER_NAME}-control-plane" \
      iptables -t nat -C PREROUTING -p tcp -d "${GW_ADDR}" --dport "${GATEWAY_PORT}" \
        -j DNAT --to-destination "${GW_ADDR}:443" 2>/dev/null || \
    ${CONTAINER_ENGINE} exec "${KIND_CLUSTER_NAME}-control-plane" \
      iptables -t nat -A PREROUTING -p tcp -d "${GW_ADDR}" --dport "${GATEWAY_PORT}" \
        -j DNAT --to-destination "${GW_ADDR}:443"
    success "In-cluster OIDC routing: ${GW_ADDR}:${GATEWAY_PORT} -> ${GW_ADDR}:443"
  fi

  if [[ -z "${KIND_KEYCLOAK_URL:-}" ]]; then
    info "Setting Keycloak hostname to ${OIDC_EXTERNAL_ISSUER%/realms/hypershell}..."
    kube set env deployment/keycloak -n keycloak \
      KC_HOSTNAME="https://${KEYCLOAK_HOSTNAME}${PORT_SUFFIX}"
    kube rollout restart deployment/keycloak -n keycloak
    kube wait --for=condition=available deployment/keycloak -n keycloak --timeout=120s
    success "Keycloak configured for OIDC"
  fi

  if ! is_swapped api-server; then
    info "Patching API server for OIDC..."
    kube set env deployment/hypershell-api-server -n "${KIND_NAMESPACE}" -c api-server \
      API_ENV=development_oidc
    kube patch deployment hypershell-api-server -n "${KIND_NAMESPACE}" --type=json \
      -p '[{"op":"add","path":"/spec/template/spec/containers/0/command/-","value":"--jwk-cert-url=http://keycloak-service.keycloak.svc.cluster.local:8080/realms/hypershell/protocol/openid-connect/certs"}]'
    success "API server patched for OIDC"
  fi

  info "Creating OIDC session secret..."
  SESSION_SECRET=$(openssl rand -hex 32)
  kube create secret generic hypershell-oidc-session \
    -n "${KIND_NAMESPACE}" \
    --from-literal=session-secret="${SESSION_SECRET}" \
    --dry-run=client -o yaml | kube apply -f -
  success "OIDC session secret created"

  if ! is_swapped web-console; then
    info "Patching web console for OIDC..."
    # Remove any stale SESSION_SECRET entries before adding the secretKeyRef.
    kube set env deployment/hypershell-web-console -n "${KIND_NAMESPACE}" -c web-console \
      OIDC_ISSUER="${OIDC_EXTERNAL_ISSUER}" \
      OIDC_CLIENT_ID="${KEYCLOAK_OIDC_CLIENT_ID}" \
      OIDC_REDIRECT_URI="https://${CONSOLE_HOSTNAME}${PORT_SUFFIX}/auth/callback" \
      NODE_TLS_REJECT_UNAUTHORIZED="0" `# local dev only: cert-manager self-signed CA` \
      SESSION_SECRET-
    kube set env deployment/hypershell-web-console -n "${KIND_NAMESPACE}" -c web-console \
      --from=secret/hypershell-oidc-session --keys=session-secret --prefix="" 2>/dev/null || \
    kube patch deployment hypershell-web-console -n "${KIND_NAMESPACE}" --type=json \
      -p '[{"op":"add","path":"/spec/template/spec/containers/0/env/-","value":{"name":"SESSION_SECRET","valueFrom":{"secretKeyRef":{"name":"hypershell-oidc-session","key":"session-secret"}}}}]'
    success "Web console patched for OIDC"
  fi

  if ! is_swapped control-plane; then
    info "Patching control plane for OIDC..."
    kube create secret generic hypershell-cp-oidc \
      -n "${KIND_NAMESPACE}" \
      --from-literal=client-secret=control-plane-secret \
      --dry-run=client -o yaml | kube apply -f -
    kube set env deployment/hypershell-controller -n "${KIND_NAMESPACE}" -c controller \
      OIDC_ISSUER="http://keycloak-service.keycloak.svc.cluster.local:8080/realms/hypershell" \
      OIDC_CLIENT_ID=hypershell-control-plane
    kube patch deployment hypershell-controller -n "${KIND_NAMESPACE}" --type=json \
      -p '[{"op":"add","path":"/spec/template/spec/containers/0/env/-","value":{"name":"OIDC_CLIENT_SECRET","valueFrom":{"secretKeyRef":{"name":"hypershell-cp-oidc","key":"client-secret"}}}}]'
    success "Control plane patched for OIDC"
  fi

  echo ""
fi

# --- Wait for readiness ---
header "Readiness"
info "Waiting for API server..."
kube wait --for=condition=available deployment/hypershell-api-server -n "${KIND_NAMESPACE}" --timeout=120s
success "API server ready"

info "Waiting for control plane..."
kube wait --for=condition=available deployment/hypershell-controller -n "${KIND_NAMESPACE}" --timeout=120s
success "Control plane ready"

info "Waiting for web console..."
kube wait --for=condition=available deployment/hypershell-web-console -n "${KIND_NAMESPACE}" --timeout=120s
success "Web console ready"
echo ""

# --- Seed Gateway via REST API ---
header "Gateway Provisioning"
API_URL="http://localhost:8000"
info "Port-forwarding to API server..."
kube port-forward svc/hypershell-api-server -n "${KIND_NAMESPACE}" 8000:8000 >/dev/null 2>&1 &
PF_PID=$!
cleanup_pf() { kill "${PF_PID}" 2>/dev/null || true; wait "${PF_PID}" 2>/dev/null || true; }
trap cleanup_pf EXIT
sleep 2

# When OIDC is enabled, obtain a Bearer token for API calls.
API_AUTH_HEADER=""
if oidc_enabled; then
  info "Obtaining API token from Keycloak..."
  KC_TOKEN_URL="http://localhost:8080/realms/hypershell/protocol/openid-connect/token"
  kube port-forward svc/keycloak-service -n keycloak 8080:8080 >/dev/null 2>&1 &
  KC_PF_PID=$!
  cleanup_pf_orig=$(declare -f cleanup_pf | tail -n +2)
  cleanup_pf() { kill "${KC_PF_PID}" 2>/dev/null || true; eval "${cleanup_pf_orig}"; }
  sleep 2
  TOKEN_RESP=$(curl -sS -X POST "${KC_TOKEN_URL}" \
    -d "grant_type=client_credentials" \
    -d "client_id=hypershell-control-plane" \
    -d "client_secret=control-plane-secret" 2>&1 || true)
  API_TOKEN=$(echo "${TOKEN_RESP}" | grep -o '"access_token":"[^"]*"' | cut -d'"' -f4 || true)
  if [[ -n "${API_TOKEN}" ]]; then
    API_AUTH_HEADER="Authorization: Bearer ${API_TOKEN}"
    success "API token obtained"
  else
    warn "Could not obtain API token: ${TOKEN_RESP:0:200}"
  fi
  kill "${KC_PF_PID}" 2>/dev/null || true
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

extract_id() {
  local id
  id=$(echo "$1" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4 || true)
  echo "${id}"
}

seed_failed=""

info "Creating default Fleet..."
FLEET_RAW=$(api_post "${API_URL}/api/hypershell/v1/fleets" \
  '{"name":"default","description":"Local development fleet"}')
FLEET_HTTP=$(echo "${FLEET_RAW}" | tail -1)
FLEET_RESP=$(echo "${FLEET_RAW}" | sed '$d')
FLEET_ID=$(extract_id "${FLEET_RESP}")

if [[ -z "${FLEET_ID}" ]]; then
  warn "Fleet creation failed (HTTP ${FLEET_HTTP}): ${FLEET_RESP:-no response}"
  seed_failed=true
fi

if [[ -z "${seed_failed}" ]]; then
  success "Fleet created: ${FLEET_ID}"

  info "Creating ManagedCluster..."
  MC_RAW=$(api_post "${API_URL}/api/hypershell/v1/managed_clusters" \
    "{\"name\":\"local-kind\",\"fleet_id\":\"${FLEET_ID}\",\"provider\":\"kind\",\"kubeconfig_secret\":\"kind-kubeconfig\"}")
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

if [[ -z "${seed_failed}" ]]; then
  info "Creating GatewayRelease..."
  GR_RAW=$(api_post "${API_URL}/api/hypershell/v1/gateway_releases" \
    "{\"name\":\"dev-release\",\"fleet_id\":\"${FLEET_ID}\",\"image\":\"${GATEWAY_IMAGE}\"}")
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

if [[ -z "${seed_failed}" ]]; then
  info "Creating ManagedDatabase..."
  MD_RAW=$(api_post "${API_URL}/api/hypershell/v1/managed_databases" \
    "{\"name\":\"local-postgres\",\"fleet_id\":\"${FLEET_ID}\",\"provider\":\"local\"}")
  MD_HTTP=$(echo "${MD_RAW}" | tail -1)
  MD_RESP=$(echo "${MD_RAW}" | sed '$d')
  DATABASE_ID=$(extract_id "${MD_RESP}")

  if [[ -z "${DATABASE_ID}" ]]; then
    warn "ManagedDatabase creation failed (HTTP ${MD_HTTP}): ${MD_RESP:-no response}"
    seed_failed=true
  else
    success "ManagedDatabase created: ${DATABASE_ID}"
  fi
fi

if [[ -z "${seed_failed}" ]] && oidc_enabled; then
  info "Creating Gateway with OIDC..."
  OIDC_JSON="{\\\"issuer\\\":\\\"${KEYCLOAK_OIDC_ISSUER}\\\",\\\"audience\\\":\\\"${KEYCLOAK_OIDC_AUDIENCE}\\\",\\\"roles_claim\\\":\\\"groups\\\",\\\"admin_role\\\":\\\"hypershell-admins\\\",\\\"user_role\\\":\\\"hypershell-users\\\"}"
  GW_RAW=$(api_post "${API_URL}/api/hypershell/v1/gateways" \
    "{\"name\":\"dev-gateway\",\"fleet_id\":\"${FLEET_ID}\",\"cluster_id\":\"${CLUSTER_ID}\",\"release_id\":\"${RELEASE_ID}\",\"database_id\":\"${DATABASE_ID}\",\"namespace\":\"openshell-dev\",\"oidc\":\"${OIDC_JSON}\"}")
  GW_HTTP=$(echo "${GW_RAW}" | tail -1)
  GW_RESP=$(echo "${GW_RAW}" | sed '$d')
  GATEWAY_ID=$(extract_id "${GW_RESP}")

  if [[ -z "${GATEWAY_ID}" ]]; then
    warn "Gateway creation failed (HTTP ${GW_HTTP}): ${GW_RESP:-no response}"
  else
    success "Gateway created with OIDC: ${GATEWAY_ID}"
  fi
fi

if [[ -n "${seed_failed}" ]]; then
  warn "Automatic seeding incomplete - create resources manually after API server is ready"
fi

cleanup_pf
trap - EXIT
echo ""

# --- kubectl port-forward (no cloud-provider-kind fallback) ---
if [[ "${CPK_RUNNING}" == "false" ]]; then
  header "kubectl Port Forwarding"
  start_kubectl_port_forwards
  echo ""
fi

# --- DNS resolution ---
header "DNS"
start_dns
setup_resolver
success "DNS configured - *.hypershell.localhost resolves to 127.0.0.1"
echo ""

# --- Summary banner ---
header "HyperShell is running!"
echo ""

if [[ "${CPK_RUNNING}" == "true" ]]; then
  PORT_SUFFIX=""
  if [[ -z "${PORT_FORWARD_ACTIVE:-}" ]] && [[ -n "${GATEWAY_PORT:-}" ]]; then
    PORT_SUFFIX=":${GATEWAY_PORT}"
  fi

  info "HTTP API:     https://${API_HOSTNAME}${PORT_SUFFIX}"
  info "Web Console:  https://${CONSOLE_HOSTNAME}${PORT_SUFFIX}"
  info "Health:       https://${HEALTH_HOSTNAME}${PORT_SUFFIX}"

  if [[ -z "${KIND_KEYCLOAK_URL:-}" ]]; then
    info "Keycloak:     https://${KEYCLOAK_HOSTNAME}${PORT_SUFFIX} (admin/admin)"
  else
    info "Keycloak:     ${KIND_KEYCLOAK_URL}"
  fi
else
  info "HTTP API:     http://localhost:8000"
  info "Web Console:  http://localhost:3000"
  info "Health:       http://localhost:8000/healthz"

  if [[ -z "${KIND_KEYCLOAK_URL:-}" ]]; then
    info "Keycloak:     http://localhost:8080 (admin/admin)"
    info "OIDC Issuer:  ${KEYCLOAK_OIDC_ISSUER}"
  else
    info "Keycloak:     ${KIND_KEYCLOAK_URL}"
  fi

  echo ""
  warn "Running without cloud-provider-kind - no TLS or hostname-based routing."
  warn "Services are available via kubectl port-forward on the ports above."
  if [[ "${HAVE_SUDO}" == "false" ]]; then
    info "To use full Gateway routing, run: cloud-provider-kind --enable-lb-port-mapping"
    info "Then: make kind-fix-ports"
  fi
fi

if oidc_enabled; then
  echo ""
  info "OIDC Authentication: ENABLED"
  info "Keycloak:            https://${KEYCLOAK_HOSTNAME}${PORT_SUFFIX:-} (admin/admin)"
  info "Login:               https://${CONSOLE_HOSTNAME}${PORT_SUFFIX:-}/auth/login"
  info "Test users:          admin/admin (admins + users), developer/developer (users only)"
fi

echo ""
info "API Server Logs:    kubectl logs -f -l app=hypershell-api-server -n ${KIND_NAMESPACE}"
info "Control Plane Logs: kubectl logs -f -l app=hypershell-controller -n ${KIND_NAMESPACE}"
info "Web Console Logs:   kubectl logs -f -l app=hypershell-web-console -n ${KIND_NAMESPACE}"
