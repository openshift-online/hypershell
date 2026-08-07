#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "${SCRIPT_DIR}/lib.sh"

require_cluster

header "Removing namespace ${KIND_NAMESPACE}"
kube delete namespace "${KIND_NAMESPACE}" --ignore-not-found
success "Namespace ${KIND_NAMESPACE} deleted"

# Check whether any hypershell-* namespaces remain.
remaining=$(kube get namespaces -o name 2>/dev/null \
  | grep -c '^namespace/hypershell' || true)

if (( remaining == 0 )); then
  echo ""
  info "No HyperShell namespaces remain."
  info "Run 'make kind-teardown' to destroy the Kind cluster."
fi
