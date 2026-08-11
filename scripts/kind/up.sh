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
  warn "KIND_NO_SUDO=true - skipping sudo; port forwarding and DNS resolver setup will be skipped"
  HAVE_SUDO=false
else
  info "This script needs sudo for cloud-provider-kind, DNS, and port forwarding."
  info "Set KIND_NO_SUDO=true to skip (services will use ephemeral ports)."
  if sudo -v 2>/dev/null; then
    HAVE_SUDO=true
    success "sudo credentials cached"
  else
    warn "sudo unavailable - port forwarding and DNS resolver setup will be skipped"
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
    info "Building patched kind and cloud-provider-kind into ./bin/..."
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

# --- Create namespace ---
header "Namespace"
kube create namespace "${KIND_NAMESPACE}" --dry-run=client -o yaml | \
  kube apply -f -

if [[ -n "${KIND_PULL_SECRET:-}" ]]; then
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
fi
echo ""

# --- Install Gateway API CRDs ---
header "Gateway API CRDs"
info "Installing Gateway API CRDs (${GATEWAY_API_VERSION}, experimental channel)..."
kube apply --server-side --force-conflicts -f "https://github.com/kubernetes-sigs/gateway-api/releases/download/${GATEWAY_API_VERSION}/experimental-install.yaml"
success "Gateway API CRDs installed"
echo ""

# --- Verify and start cloud-provider-kind ---
header "cloud-provider-kind"
if ! command -v cloud-provider-kind >/dev/null 2>&1; then
  error "cloud-provider-kind not found in PATH"
  info "Install via: brew install cloud-provider-kind"
  info "         or: go install sigs.k8s.io/cloud-provider-kind@${CLOUD_PROVIDER_KIND_VERSION}"
  exit 1
fi

if ! pgrep -f "cloud-provider-kind" >/dev/null 2>&1; then
  if [[ "${HAVE_SUDO}" == "true" ]]; then
    info "Starting cloud-provider-kind..."
    sudo -E nohup cloud-provider-kind --enable-lb-port-mapping >/tmp/cloud-provider-kind.log 2>&1 &
    sleep 2
    if pgrep -f "cloud-provider-kind" >/dev/null 2>&1; then
      success "cloud-provider-kind started"
    else
      error "cloud-provider-kind failed to start - check /tmp/cloud-provider-kind.log"
      exit 1
    fi
  else
    warn "Skipping cloud-provider-kind (no sudo) - Gateway routing will not work"
    warn "Start manually: sudo nohup cloud-provider-kind --enable-lb-port-mapping >/tmp/cloud-provider-kind.log 2>&1 &"
  fi
else
  warn "cloud-provider-kind already running"
fi
echo ""

# --- Install cert-manager ---
header "cert-manager"
info "Installing cert-manager ${CERT_MANAGER_VERSION}..."
if kube get namespace cert-manager >/dev/null 2>&1; then
  warn "cert-manager namespace exists, skipping install"
else
  kube apply -f "https://github.com/cert-manager/cert-manager/releases/download/${CERT_MANAGER_VERSION}/cert-manager.yaml"
fi
info "Waiting for cert-manager..."
kube wait --for=condition=available deployment/cert-manager -n cert-manager --timeout=120s
kube wait --for=condition=available deployment/cert-manager-webhook -n cert-manager --timeout=120s
success "cert-manager ready"
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

# --- Deploy Keycloak (cluster-wide, one instance shared by all namespaces) ---
header "Keycloak"
if [[ -z "${KIND_KEYCLOAK_URL:-}" ]]; then
  info "Deploying local Keycloak in 'keycloak' namespace..."
  kube create namespace keycloak --dry-run=client -o yaml | \
    kube apply -f -
  kube apply -f deploy/kind/prerequisites/keycloak.yaml
  info "Waiting for Keycloak..."
  kube wait --for=condition=available deployment/keycloak -n keycloak --timeout=180s
  success "Keycloak ready"
else
  warn "Using external Keycloak: ${KIND_KEYCLOAK_URL}"
fi
echo ""

# --- Apply manifests (skip swapped components) ---
header "Deploying Components"
kube apply -f deploy/kind/namespace.yaml

info "Deploying API server database..."
kube apply -f deploy/kind/postgres.yaml
info "Waiting for PostgreSQL..."
kube wait --for=condition=available deployment/hypershell-postgres -n "${KIND_NAMESPACE}" --timeout=300s
success "PostgreSQL ready"

if ! is_swapped api-server; then
  info "Deploying API server..."
  kube apply -f deploy/kind/api-server.yaml
else
  warn "API server is swapped - skipping manifest"
fi

if ! is_swapped control-plane; then
  info "Deploying control plane RBAC..."
  kube apply -f deploy/kind/controller-rbac.yaml
  info "Deploying control plane..."
  kube apply -f deploy/kind/controller.yaml
else
  warn "Control plane is swapped - skipping manifest"
fi

if ! is_swapped web-console; then
  info "Deploying web console..."
  kube apply -f deploy/kind/web-console.yaml
else
  warn "Web console is swapped - skipping manifest"
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

# --- Certificates and networking ---
header "TLS & Networking"
info "Setting up TLS certificates..."
kube apply -f deploy/kind/prerequisites/certificates.yaml

info "Setting up Gateway API networking..."
kube apply -f deploy/kind/prerequisites/networking-gateway.yaml
kube apply -f deploy/kind/prerequisites/httproutes.yaml

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
  GATEWAY_PORT=""
  KEYCLOAK_HTTP_PORT=""
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
  warn "Install: go install sigs.k8s.io/cloud-provider-kind@${CLOUD_PROVIDER_KIND_VERSION}"
  warn "Start:   sudo nohup cloud-provider-kind >/tmp/cloud-provider-kind.log 2>&1 &"
fi
echo ""

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

# Helper: POST a JSON resource; prints the response body on success or failure.
api_post() {
  local url="$1" data="$2"
  curl -sS -w "\n%{http_code}" -X POST "${url}" \
    -H "Content-Type: application/json" \
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

if [[ -n "${seed_failed}" ]]; then
  warn "Automatic seeding incomplete - create resources manually after API server is ready"
fi

cleanup_pf
trap - EXIT
echo ""

# --- DNS resolution ---
header "DNS"
start_dns
setup_resolver
success "DNS configured - *.hypershell.localhost resolves to 127.0.0.1"
echo ""

# --- Summary banner ---
header "HyperShell is running!"
echo ""

PORT_SUFFIX=""
if [[ -z "${PORT_FORWARD_ACTIVE:-}" ]] && [[ -n "${GATEWAY_PORT:-}" ]]; then
  PORT_SUFFIX=":${GATEWAY_PORT}"
fi

info "HTTP API:     https://${API_HOSTNAME}${PORT_SUFFIX}"
info "Web Console:  https://${CONSOLE_HOSTNAME}${PORT_SUFFIX}"
info "Health:       https://${HEALTH_HOSTNAME}${PORT_SUFFIX}"


if [[ -z "${KIND_KEYCLOAK_URL:-}" ]]; then
  info "Keycloak:     https://${KEYCLOAK_HOSTNAME}${PORT_SUFFIX} (admin/admin)"
  info "Keycloak HTTP: http://${KEYCLOAK_HOSTNAME}:8080 (admin/admin)"
  info "OIDC Issuer:  ${KEYCLOAK_OIDC_ISSUER}"
else
  info "Keycloak:     ${KIND_KEYCLOAK_URL}"
fi

echo ""
info "API Server Logs:    kubectl logs -f -l app=hypershell-api-server -n ${KIND_NAMESPACE}"
info "Control Plane Logs: kubectl logs -f -l app=hypershell-controller -n ${KIND_NAMESPACE}"
info "Web Console Logs:   kubectl logs -f -l app=hypershell-web-console -n ${KIND_NAMESPACE}"
