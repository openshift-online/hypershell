#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "${SCRIPT_DIR}/lib.sh"

clusters=("${KIND_CLUSTER_NAME}")
if [[ "${KIND_CLUSTER_NAME}" == "hypershell-dev" ]]; then
  clusters+=(hypershell-oce)
fi

if pgrep -f "cloud-provider-kind" >/dev/null 2>&1; then
  info "Stopping cloud-provider-kind..."
  sudo pkill -f "cloud-provider-kind" 2>/dev/null || pkill -f "cloud-provider-kind" || true
  success "cloud-provider-kind stopped"
fi
for cluster in "${clusters[@]}"; do
  (
    export KIND_CLUSTER_NAME="${cluster}"
    # OCE always uses Docker, even when the default Kind provider is Podman.
    if [[ "${cluster}" == "hypershell-oce" ]]; then
      export CONTAINER_ENGINE=docker KIND_EXPERIMENTAL_PROVIDER=docker
    fi
    source "${SCRIPT_DIR}/lib.sh"
    header "Tearing down Kind cluster '${KIND_CLUSTER_NAME}'"

    # Forget the stopped cloud-provider-kind instance for each cluster.
    rm -f "${CPK_SHA_MARKER}"
    stop_port_forward
    stop_kubectl_port_forwards
    stop_dns

    if cluster_exists; then
      info "Deleting cluster..."
      kind delete cluster --name "${KIND_CLUSTER_NAME}"
      success "Cluster deleted"
    else
      warn "Cluster '${KIND_CLUSTER_NAME}' not found"
    fi
  )
done

cleanup_resolver
rm -f "${SWAP_FILE}"
success "Done."
