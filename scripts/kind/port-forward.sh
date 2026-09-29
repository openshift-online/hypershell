#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "${SCRIPT_DIR}/lib.sh"

header "Port Forwarding"

require_cluster

# Check for cloud-provider-kind proxy container first.
PROXY_CONTAINER=$(${CONTAINER_ENGINE} ps -q --filter "name=kindccm-gw" 2>/dev/null | head -1)

if [[ -n "${PROXY_CONTAINER}" ]]; then
  GATEWAY_PORT=$(${CONTAINER_ENGINE} port "${PROXY_CONTAINER}" 443 2>/dev/null | head -1 | cut -d: -f2)

  if [[ -z "${GATEWAY_PORT}" ]]; then
    error "Could not discover HTTPS port mapping from proxy container"
    exit 1
  fi

  info "Discovered port: 443 -> ${GATEWAY_PORT}"

  stop_port_forward
  start_port_forward "${GATEWAY_PORT}"

  # Sync in-cluster CoreDNS with the current gateway IP. cloud-provider-kind can
  # assign a different container IP on restart, making the BFF unable to reach
  # Keycloak for OIDC code exchange (EHOSTUNREACH -> login 401).
  GW_IP=$(kube get gateway hypershell-gw -n "${KIND_NAMESPACE}" \
    -o jsonpath='{.status.addresses[0].value}' 2>/dev/null || true)
  if [[ -n "${GW_IP}" ]]; then
    patch_cluster_coredns "${GW_IP}"
  else
    warn "Could not read gateway IP from status - skipping in-cluster CoreDNS sync"
  fi
else
  info "No cloud-provider-kind proxy container found - using kubectl port-forward"
  start_kubectl_port_forwards
fi
