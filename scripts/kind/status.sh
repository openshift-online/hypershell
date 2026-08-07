#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "${SCRIPT_DIR}/lib.sh"

header "Cluster"
kube cluster-info 2>/dev/null || warn "Cluster '${KIND_CLUSTER_NAME}' is not running"
echo ""

header "Namespaces"
NS_OUTPUT=$(kube get namespaces -l app.kubernetes.io/part-of=hypershell --no-headers 2>/dev/null)
if [[ -n "${NS_OUTPUT}" ]]; then
  kube get namespaces -l app.kubernetes.io/part-of=hypershell
else
  kube get namespace "${KIND_NAMESPACE}" 2>/dev/null || warn "No HyperShell namespaces found"
fi
echo ""

header "Pods (${KIND_NAMESPACE})"
kube get pods -n "${KIND_NAMESPACE}" -o wide 2>/dev/null || warn "Namespace not found"
echo ""

header "Services (${KIND_NAMESPACE})"
kube get svc -n "${KIND_NAMESPACE}" 2>/dev/null || warn "Namespace not found"
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

header "DNS"
if dns_container_running 2>/dev/null; then
  info "CoreDNS: running (${DNS_CONTAINER_NAME} on port ${KIND_DNS_PORT})"
else
  warn "CoreDNS: not running"
fi
echo ""

header "Port Forwarding"
PF_ACTIVE=""
case "$(uname -s)" in
  Darwin)
    PF_RULE=$(sudo pfctl -a "${PF_ANCHOR}" -s nat 2>/dev/null | grep "rdr" || true)
    if [[ -n "${PF_RULE}" ]]; then
      PF_ACTIVE=true
      PF_PORT=$(echo "${PF_RULE}" | grep -o 'port [0-9]*$' | awk '{print $2}')
      info "pfctl: active (443 -> ${PF_PORT:-?})"
    else
      warn "pfctl: no forwarding rules"
    fi
    ;;
  Linux)
    IPT_RULE=$(sudo iptables -t nat -L "${IPTABLES_CHAIN}" -n 2>/dev/null | grep "REDIRECT" || true)
    if [[ -n "${IPT_RULE}" ]]; then
      PF_ACTIVE=true
      IPT_PORT=$(echo "${IPT_RULE}" | grep -o 'redir ports [0-9]*' | awk '{print $3}')
      info "iptables: active (443 -> ${IPT_PORT:-?})"
    else
      warn "iptables: no forwarding rules"
    fi
    ;;
esac
echo ""

PORT_SUFFIX=""
if [[ -z "${PF_ACTIVE}" ]]; then
  PROXY_CONTAINER=$(${CONTAINER_ENGINE} ps -q --filter "name=kindccm-gw" 2>/dev/null | head -1)
  if [[ -n "${PROXY_CONTAINER}" ]]; then
    GW_PORT=$(${CONTAINER_ENGINE} port "${PROXY_CONTAINER}" 443 2>/dev/null | head -1 | cut -d: -f2)
    if [[ -n "${GW_PORT}" ]]; then
      PORT_SUFFIX=":${GW_PORT}"
    fi
  fi
fi

header "Access"
info "HTTP API:     https://${API_HOSTNAME}${PORT_SUFFIX}"
info "Web Console:  https://${CONSOLE_HOSTNAME}${PORT_SUFFIX}"
info "Health:       https://${HEALTH_HOSTNAME}${PORT_SUFFIX}"
info "Keycloak:     https://${KEYCLOAK_HOSTNAME}${PORT_SUFFIX}"
