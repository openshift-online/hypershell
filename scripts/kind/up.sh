#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "${SCRIPT_DIR}/lib.sh"

header "HyperShell Local Development Environment"
echo ""

# --- Cluster creation (idempotent) ---
header "Cluster"
if cluster_exists; then
  warn "Cluster '${KIND_CLUSTER_NAME}' already exists, reusing"
else
  info "Creating Kind cluster '${KIND_CLUSTER_NAME}'..."
  sed "s|__KIND_HOST_MOUNT_PATH__|${KIND_HOST_MOUNT_PATH:-$(git rev-parse --show-toplevel 2>/dev/null || pwd)}|g" \
    "${KIND_CONFIG}" > /tmp/kind-config-rendered.yaml
  kind create cluster --name "${KIND_CLUSTER_NAME}" --config /tmp/kind-config-rendered.yaml
  rm -f /tmp/kind-config-rendered.yaml
  success "Cluster created"
fi

# --- Set kubectl context and wait for API server ---
kubectl cluster-info --context "$(kctx)" >/dev/null 2>&1 || \
  kubectl config use-context "$(kctx)"

info "Waiting for Kubernetes API server..."
for i in $(seq 1 15); do
  if kubectl get nodes --context "$(kctx)" >/dev/null 2>&1; then break; fi
  info "API server not ready, retrying (${i}/15)..."
  sleep 2
done
echo ""

# --- Create namespace ---
header "Namespace"
kubectl --context "$(kctx)" create namespace "${KIND_NAMESPACE}" --dry-run=client -o yaml | \
  kubectl --context "$(kctx)" apply -f -

if [[ -n "${KIND_PULL_SECRET:-}" ]]; then
  info "Applying pull secret from ${KIND_PULL_SECRET}..."
  kubectl apply -f "${KIND_PULL_SECRET}" -n "${KIND_NAMESPACE}"
fi
echo ""

# --- Install Gateway API CRDs ---
header "Gateway API CRDs"
info "Installing Gateway API CRDs (${GATEWAY_API_VERSION}, experimental channel)..."
kubectl apply -f "https://github.com/kubernetes-sigs/gateway-api/releases/download/${GATEWAY_API_VERSION}/experimental-install.yaml"
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
  info "Starting cloud-provider-kind..."
  nohup cloud-provider-kind >/tmp/cloud-provider-kind.log 2>&1 &
  success "cloud-provider-kind started (PID: $!)"
else
  warn "cloud-provider-kind already running"
fi
echo ""

# --- Install cert-manager ---
header "cert-manager"
info "Installing cert-manager ${CERT_MANAGER_VERSION}..."
if kubectl get namespace cert-manager >/dev/null 2>&1; then
  warn "cert-manager namespace exists, skipping install"
else
  kubectl apply -f "https://github.com/cert-manager/cert-manager/releases/download/${CERT_MANAGER_VERSION}/cert-manager.yaml"
fi
info "Waiting for cert-manager..."
kubectl wait --for=condition=available deployment/cert-manager -n cert-manager --timeout=120s
kubectl wait --for=condition=available deployment/cert-manager-webhook -n cert-manager --timeout=120s
success "cert-manager ready"
echo ""

# --- Build and load local images (offline mode) ---
if [[ "${LOCAL_IMAGES:-}" == "true" ]]; then
  header "Local Images"
  info "Building images from working tree..."
  "${SCRIPT_DIR}/build-images.sh"
  echo ""
fi

# --- Deploy Keycloak (cluster-wide, one instance shared by all namespaces) ---
header "Keycloak"
if [[ -z "${KIND_KEYCLOAK_URL:-}" ]]; then
  info "Deploying local Keycloak in 'keycloak' namespace..."
  kubectl --context "$(kctx)" create namespace keycloak --dry-run=client -o yaml | \
    kubectl --context "$(kctx)" apply -f -
  kubectl apply -f deploy/kind/prerequisites/keycloak.yaml
  info "Waiting for Keycloak..."
  kubectl wait --for=condition=available deployment/keycloak -n keycloak --timeout=180s
  success "Keycloak ready"
else
  warn "Using external Keycloak: ${KIND_KEYCLOAK_URL}"
fi
echo ""

# --- Apply manifests (skip swapped components) ---
header "Deploying Components"
kubectl apply -f deploy/kind/namespace.yaml

info "Deploying API server database..."
kubectl apply -f deploy/kind/postgres.yaml
info "Waiting for PostgreSQL..."
kubectl wait --for=condition=available deployment/hypershell-postgres -n "${KIND_NAMESPACE}" --timeout=120s
success "PostgreSQL ready"

if ! is_swapped api-server; then
  info "Deploying API server..."
  kubectl apply -f deploy/kind/api-server.yaml
else
  warn "API server is swapped — skipping manifest"
fi

if ! is_swapped control-plane; then
  info "Deploying control plane RBAC..."
  kubectl apply -f deploy/kind/controller-rbac.yaml
  info "Deploying control plane..."
  kubectl apply -f deploy/kind/controller.yaml
else
  warn "Control plane is swapped — skipping manifest"
fi

if ! is_swapped web-console; then
  info "Deploying web console..."
  kubectl apply -f deploy/kind/web-console.yaml
else
  warn "Web console is swapped — skipping manifest"
fi
echo ""

# --- Certificates and networking ---
header "TLS & Networking"
info "Setting up TLS certificates..."
kubectl apply -f deploy/kind/prerequisites/certificates.yaml

if [[ -z "${KIND_USE_NODEPORT:-}" ]]; then
  info "Setting up Gateway API networking..."
  kubectl apply -f deploy/kind/prerequisites/networking-gateway.yaml
  kubectl apply -f deploy/kind/prerequisites/httproutes.yaml
else
  info "Setting up NodePort services..."
  kubectl apply -f deploy/kind/prerequisites/nodeport-services.yaml
fi
success "Networking configured"
echo ""

# --- Wait for readiness ---
header "Readiness"
info "Waiting for API server..."
kubectl wait --for=condition=available deployment/hypershell-api-server -n "${KIND_NAMESPACE}" --timeout=120s
success "API server ready"

info "Waiting for control plane..."
kubectl wait --for=condition=available deployment/hypershell-controller -n "${KIND_NAMESPACE}" --timeout=120s
success "Control plane ready"

info "Waiting for web console..."
kubectl wait --for=condition=available deployment/hypershell-web-console -n "${KIND_NAMESPACE}" --timeout=120s
success "Web console ready"
echo ""

# --- Seed Gateway via REST API ---
header "Gateway Provisioning"
API_URL="http://localhost:8000"
info "Port-forwarding to API server..."
kubectl port-forward svc/hypershell-api-server -n "${KIND_NAMESPACE}" 8000:8000 >/dev/null 2>&1 &
PF_PID=$!
sleep 2

info "Creating default Fleet..."
FLEET_RESP=$(curl -sf -X POST "${API_URL}/api/hypershell/v1/fleets" \
  -H "Content-Type: application/json" \
  -d '{"name":"default","description":"Local development fleet"}' 2>/dev/null || true)
FLEET_ID=$(echo "${FLEET_RESP}" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)

if [[ -n "${FLEET_ID}" ]]; then
  success "Fleet created: ${FLEET_ID}"

  info "Creating Gateway (controller will provision database)..."
  GW_RESP=$(curl -sf -X POST "${API_URL}/api/hypershell/v1/gateways" \
    -H "Content-Type: application/json" \
    -d "{\"name\":\"openshell-gateway\",\"fleet_id\":\"${FLEET_ID}\",\"namespace\":\"${KIND_NAMESPACE}\"}" 2>/dev/null || true)
  GW_ID=$(echo "${GW_RESP}" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)

  if [[ -n "${GW_ID}" ]]; then
    success "Gateway created: ${GW_ID} — controller will reconcile"
  else
    warn "Gateway creation returned: ${GW_RESP:-no response}"
    warn "Create manually: curl -X POST ${API_URL}/api/hypershell/v1/gateways -H 'Content-Type: application/json' -d '{\"name\":\"openshell-gateway\",\"fleet_id\":\"<fleet-id>\",\"namespace\":\"${KIND_NAMESPACE}\"}'"
  fi
else
  warn "Fleet creation returned: ${FLEET_RESP:-no response}"
  warn "Create manually after API server is ready"
fi

kill "${PF_PID}" 2>/dev/null || true
echo ""

# --- Configure /etc/hosts (hostname mode only) ---
if [[ -z "${KIND_USE_NODEPORT:-}" ]]; then
  header "/etc/hosts"
  for h in "${API_HOSTNAME}" "${CONSOLE_HOSTNAME}" "${HEALTH_HOSTNAME}"; do
    if ! grep -q "${h}" /etc/hosts 2>/dev/null; then
      info "Adding ${h} to /etc/hosts (requires sudo)"
      sudo sh -c "echo '127.0.0.1 ${h}' >> /etc/hosts"
    fi
  done
  success "Host entries configured"
  echo ""
fi

# --- Summary banner ---
header "HyperShell is running!"
echo ""
if [[ -z "${KIND_USE_NODEPORT:-}" ]]; then
  info "HTTP API:     https://${API_HOSTNAME}"
  info "Web Console:  https://${CONSOLE_HOSTNAME}"
  info "Health:       https://${HEALTH_HOSTNAME}"
  info "gRPC:         https://openshell-gateway.gw.localhost"
else
  info "HTTP API:     http://localhost:${KIND_API_PORT}"
  info "gRPC:         localhost:${KIND_GRPC_PORT}"
  info "Health:       http://localhost:${KIND_HEALTH_PORT}"
  info "Web Console:  http://localhost:${KIND_CONSOLE_PORT}"
fi

if [[ -z "${KIND_KEYCLOAK_URL:-}" ]]; then
  info "Keycloak:     http://localhost:8080 (admin/admin)"
  info "OIDC Issuer:  http://keycloak-service.keycloak.svc.cluster.local:8080/realms/hypershell"
else
  info "Keycloak:     ${KIND_KEYCLOAK_URL}"
fi

echo ""
info "API Server Logs:    kubectl logs -f -l app=hypershell-api-server -n ${KIND_NAMESPACE}"
info "Control Plane Logs: kubectl logs -f -l app=hypershell-controller -n ${KIND_NAMESPACE}"
info "Web Console Logs:   kubectl logs -f -l app=hypershell-web-console -n ${KIND_NAMESPACE}"
