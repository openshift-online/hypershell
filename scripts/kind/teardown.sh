#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "${SCRIPT_DIR}/lib.sh"

header "Tearing down Kind cluster '${KIND_CLUSTER_NAME}'"

if pgrep -f "cloud-provider-kind" >/dev/null 2>&1; then
  info "Stopping cloud-provider-kind..."
  sudo pkill -f "cloud-provider-kind" 2>/dev/null || pkill -f "cloud-provider-kind" || true
  success "cloud-provider-kind stopped"
fi
# Drop the running-instance marker so the next `make kind-up` doesn't compare
# against a SHA from a torn-down environment.
rm -f "${CPK_SHA_MARKER}"

stop_port_forward
stop_kubectl_port_forwards
stop_dns
cleanup_resolver

if cluster_exists; then
  info "Deleting cluster..."
  kind delete cluster --name "${KIND_CLUSTER_NAME}"
  success "Cluster deleted"
else
  warn "Cluster '${KIND_CLUSTER_NAME}' not found"
fi

rm -f "${SWAP_FILE}"
success "Done."
