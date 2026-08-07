#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "${SCRIPT_DIR}/lib.sh"

require_cluster

ACTION="${1:-deploy}"

# --- Resolve target namespace ---
# KIND_NAMESPACE drives both add and remove. When it is the default
# (hypershell-system), derive from the current git branch instead.
if [[ "${KIND_NAMESPACE}" != "hypershell-system" ]]; then
  NS="${KIND_NAMESPACE}"
else
  BRANCH="$(git rev-parse --abbrev-ref HEAD 2>/dev/null)"
  NS="hypershell-$(echo "${BRANCH}" | sed 's/[^a-z0-9-]/-/g' | cut -c1-63)"
fi

# --- Remove ---
if [[ "${ACTION}" == "undeploy" ]]; then
  if [[ "${NS}" == "hypershell-system" ]]; then
    error "Cannot remove the default namespace. Use 'make kind-down' instead."
    exit 1
  fi
  header "Removing namespace ${NS}"
  kube delete namespace "${NS}" --ignore-not-found
  success "Done."
  exit 0
fi

# --- Add ---
SANITIZED="${NS#hypershell-}"

header "Deploy to namespace ${NS}"

kube create namespace "${NS}" --dry-run=client -o yaml | kube apply -f -

info "Applying manifests to ${NS}..."
for f in deploy/kind/api-server.yaml deploy/kind/controller.yaml deploy/kind/web-console.yaml; do
  sed "s/namespace: hypershell-system/namespace: ${NS}/g" "${f}" | kube apply -f -
done

header "HTTPRoutes"
info "Creating namespace-scoped HTTPRoutes..."
for svc_host in "api-server:api.${SANITIZED}.hypershell.localhost:8000" \
                "web-console:console.${SANITIZED}.hypershell.localhost:3000" \
                "health:health.${SANITIZED}.hypershell.localhost:8434"; do
  SVC="$(echo "${svc_host}" | cut -d: -f1)"
  HOST="$(echo "${svc_host}" | cut -d: -f2)"
  PORT="$(echo "${svc_host}" | cut -d: -f3)"
  kube apply -f - <<EOF
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: ${SVC}
  namespace: ${NS}
spec:
  parentRefs:
  - name: hypershell-gw
    namespace: hypershell-system
  hostnames:
  - ${HOST}
  rules:
  - backendRefs:
    - name: hypershell-${SVC}
      port: ${PORT}
EOF
done
success "HTTPRoutes created"

header "Readiness"
info "Waiting for components in ${NS}..."
kube wait --for=condition=available deployment/hypershell-api-server -n "${NS}" --timeout=120s
kube wait --for=condition=available deployment/hypershell-controller -n "${NS}" --timeout=120s
kube wait --for=condition=available deployment/hypershell-web-console -n "${NS}" --timeout=120s
success "All components ready"

echo ""
header "Namespace ${NS} deployed"
info "API:     https://api.${SANITIZED}.hypershell.localhost"
info "Console: https://console.${SANITIZED}.hypershell.localhost"
info "Health:  https://health.${SANITIZED}.hypershell.localhost"
