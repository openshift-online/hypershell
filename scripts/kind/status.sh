#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "${SCRIPT_DIR}/lib.sh"

header "Cluster"
kubectl cluster-info --context "$(kctx)" 2>/dev/null || warn "Cluster '${KIND_CLUSTER_NAME}' is not running"
echo ""

header "Namespaces"
kubectl get namespaces -l app.kubernetes.io/part-of=hypershell 2>/dev/null || \
  kubectl get namespace "${KIND_NAMESPACE}" 2>/dev/null || warn "No HyperShell namespaces found"
echo ""

header "Pods (${KIND_NAMESPACE})"
kubectl get pods -n "${KIND_NAMESPACE}" -o wide 2>/dev/null || warn "Namespace not found"
echo ""

header "Services (${KIND_NAMESPACE})"
kubectl get svc -n "${KIND_NAMESPACE}" 2>/dev/null || warn "Namespace not found"
echo ""

header "Component Swap Status"
if [[ -f "${SWAP_FILE}" ]] && [[ -s "${SWAP_FILE}" ]]; then
  info "Swapped components:"
  while IFS= read -r comp; do
    info "  - ${comp} (local build)"
  done < "${SWAP_FILE}"
  info "Baseline components:"
  for comp in api-server control-plane web-console; do
    if ! grep -q "^${comp}$" "${SWAP_FILE}" 2>/dev/null; then
      info "  - ${comp} (registry image)"
    fi
  done
else
  info "All components running baseline (registry) images"
fi
echo ""

header "Access"
if [[ -z "${KIND_USE_NODEPORT:-}" ]]; then
  info "HTTP API:     https://${API_HOSTNAME}"
  info "Web Console:  https://${CONSOLE_HOSTNAME}"
  info "Health:       https://${HEALTH_HOSTNAME}"
else
  info "HTTP API:     http://localhost:${KIND_API_PORT}"
  info "gRPC:         localhost:${KIND_GRPC_PORT}"
  info "Health:       http://localhost:${KIND_HEALTH_PORT}"
  info "Web Console:  http://localhost:${KIND_CONSOLE_PORT}"
fi
