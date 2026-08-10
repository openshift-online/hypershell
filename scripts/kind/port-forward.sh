#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "${SCRIPT_DIR}/lib.sh"

header "Port Forwarding"

require_cluster

PROXY_CONTAINER=$(${CONTAINER_ENGINE} ps -q --filter "name=kindccm-gw" 2>/dev/null | head -1)
if [[ -z "${PROXY_CONTAINER}" ]]; then
  error "No Gateway proxy container found — is cloud-provider-kind running?"
  exit 1
fi

GATEWAY_PORT=$(${CONTAINER_ENGINE} port "${PROXY_CONTAINER}" 443 2>/dev/null | head -1 | cut -d: -f2)
KEYCLOAK_HTTP_PORT=$(${CONTAINER_ENGINE} port "${PROXY_CONTAINER}" 8080 2>/dev/null | head -1 | cut -d: -f2)

if [[ -z "${GATEWAY_PORT}" ]]; then
  error "Could not discover HTTPS port mapping from proxy container"
  exit 1
fi

info "Discovered ports: 443 -> ${GATEWAY_PORT}, 8080 -> ${KEYCLOAK_HTTP_PORT:-none}"

stop_port_forward
start_port_forward "${GATEWAY_PORT}" "${KEYCLOAK_HTTP_PORT:-}"
