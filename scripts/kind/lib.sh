#!/usr/bin/env bash
set -euo pipefail

# --- Color output (respects NO_COLOR and non-TTY) ---

if [[ -z "${NO_COLOR:-}" ]] && [[ -t 1 ]]; then
  BOLD='\033[1m'
  BLUE='\033[0;34m'
  CYAN='\033[0;36m'
  GREEN='\033[0;32m'
  YELLOW='\033[1;33m'
  RED='\033[0;31m'
  NC='\033[0m'
else
  BOLD='' BLUE='' CYAN='' GREEN='' YELLOW='' RED='' NC=''
fi

header()  { printf "${BOLD}${BLUE}==> %s${NC}\n" "$*"; }
info()    { printf "${CYAN}    %s${NC}\n" "$*"; }
success() { printf "${GREEN}    %s${NC}\n" "$*"; }
warn()    { printf "${YELLOW}    %s${NC}\n" "$*"; }
error()   { printf "${RED}ERROR: %s${NC}\n" "$*" >&2; }

# --- Defaults (defensive - Make exports these, but scripts can run standalone) ---

: "${KIND_CLUSTER_NAME:=hypershell-dev}"
: "${KIND_NAMESPACE:=hypershell-system}"
: "${CONTAINER_ENGINE:=$(command -v podman 2>/dev/null || echo docker)}"
if [[ "$(basename "${CONTAINER_ENGINE}")" == "podman" ]]; then
  export KIND_EXPERIMENTAL_PROVIDER=podman
fi
: "${GATEWAY_IMAGE:=quay.io/redhat-services-prod/hcm-eng-prod-tenant/hypershell-main/hypershell-api-server-main:latest}"
: "${KEYCLOAK_HOSTNAME:=keycloak.hypershell.localhost}"
: "${KIND_DNS_PORT:=5553}"
DNS_CONTAINER_NAME="${KIND_CLUSTER_NAME}-dns"
REPO_ROOT="$(cd "${SCRIPT_DIR:-$(dirname "${BASH_SOURCE[0]}")}/../.." && pwd)"

# --- Cluster helpers ---

cluster_exists() {
  kind get clusters 2>/dev/null | grep -q "^${KIND_CLUSTER_NAME}$"
}

require_cluster() {
  if ! cluster_exists; then
    error "No Kind cluster '${KIND_CLUSTER_NAME}' running. Run 'make kind-up' first."
    exit 1
  fi
}

kctx() {
  echo "kind-${KIND_CLUSTER_NAME}"
}

kube() {
  kubectl --context "$(kctx)" "$@"
}

# --- Swap tracking (.kind-swaps) ---

SWAP_FILE=".kind-swaps"

track_swap() {
  local component="$1"
  grep -q "^${component}$" "${SWAP_FILE}" 2>/dev/null || echo "${component}" >> "${SWAP_FILE}"
}

clear_swap() {
  local component="$1"
  if [[ -f "${SWAP_FILE}" ]]; then
    sed -i.bak "/^${component}$/d" "${SWAP_FILE}" 2>/dev/null
    rm -f "${SWAP_FILE}.bak"
  fi
}

is_swapped() {
  local component="$1"
  grep -q "^${component}$" "${SWAP_FILE}" 2>/dev/null
}

# --- DNS (CoreDNS container) ---

dns_container_running() {
  ${CONTAINER_ENGINE} inspect -f '{{.State.Running}}' "${DNS_CONTAINER_NAME}" 2>/dev/null | grep -q true
}

start_dns() {
  if dns_container_running; then
    warn "CoreDNS already running (${DNS_CONTAINER_NAME})"
    return
  fi
  ${CONTAINER_ENGINE} rm -f "${DNS_CONTAINER_NAME}" 2>/dev/null || true
  info "Starting CoreDNS on 127.0.0.1:${KIND_DNS_PORT}..."
  ${CONTAINER_ENGINE} run -d \
    --name "${DNS_CONTAINER_NAME}" \
    --restart unless-stopped \
    -p "127.0.0.1:${KIND_DNS_PORT}:53/udp" \
    -p "127.0.0.1:${KIND_DNS_PORT}:53/tcp" \
    -v "${REPO_ROOT}/deploy/kind/coredns/Corefile:/Corefile:ro" \
    docker.io/coredns/coredns:1.12.0 >/dev/null
  sleep 1
  if dns_container_running; then
    success "CoreDNS started"
  else
    error "CoreDNS failed to start"
    ${CONTAINER_ENGINE} logs "${DNS_CONTAINER_NAME}" 2>&1 || true
  fi
}

stop_dns() {
  ${CONTAINER_ENGINE} rm -f "${DNS_CONTAINER_NAME}" 2>/dev/null || true
}

setup_resolver() {
  if [[ "${HAVE_SUDO:-true}" == "false" ]]; then
    warn "Skipping resolver setup (no sudo)"
    return
  fi
  case "$(uname -s)" in
    Darwin)
      local resolver_file="/etc/resolver/localhost"
      local expected
      expected="$(printf 'nameserver 127.0.0.1\nport %s\n' "${KIND_DNS_PORT}")"
      if [[ -f "${resolver_file}" ]] && [[ "$(cat "${resolver_file}")" == "${expected}" ]]; then
        info "macOS resolver already configured"
        return
      fi
      info "Configuring macOS resolver (requires sudo)..."
      if sudo mkdir -p /etc/resolver && \
         printf '%s\n' "${expected}" | sudo tee "${resolver_file}" >/dev/null; then
        success "macOS resolver configured: ${resolver_file}"
      else
        warn "Could not configure resolver - manually create ${resolver_file} with:"
        warn "  nameserver 127.0.0.1"
        warn "  port ${KIND_DNS_PORT}"
      fi
      ;;
    Linux)
      if command -v resolvectl >/dev/null 2>&1; then
        info "Configuring systemd-resolved for .localhost..."
        if sudo resolvectl dns lo "127.0.0.1:${KIND_DNS_PORT}" && \
           sudo resolvectl domain lo '~localhost'; then
          success "systemd-resolved configured"
        else
          warn "Could not configure systemd-resolved"
          warn "Add '127.0.0.1 <hostname>' to /etc/hosts as a fallback"
        fi
      else
        warn "resolvectl not found - add hostnames to /etc/hosts manually"
      fi
      ;;
  esac
}

cleanup_resolver() {
  case "$(uname -s)" in
    Darwin)
      # /etc/resolver/localhost is harmless without CoreDNS; skip to avoid sudo on teardown
      ;;
    Linux)
      if command -v resolvectl >/dev/null 2>&1; then
        sudo resolvectl revert lo 2>/dev/null || true
      fi
      ;;
  esac
}

# --- Port forwarding (443 → ephemeral) ---

PF_ANCHOR="com.hypershell/${KIND_CLUSTER_NAME}"
IPTABLES_CHAIN="HS-${KIND_CLUSTER_NAME}"

start_port_forward() {
  local ephemeral_port="$1"
  PORT_FORWARD_ACTIVE=""
  if [[ "${HAVE_SUDO:-true}" == "false" ]]; then
    warn "Skipping port forwarding (no sudo) - use port ${ephemeral_port} directly"
    return
  fi
  case "$(uname -s)" in
    Darwin)
      info "Setting up port forwarding: 443 -> ${ephemeral_port} (pfctl)..."
      local pf_rules
      pf_rules=$(sed '/^rdr-anchor "com\.apple\/\*"/a\
rdr-anchor "com.hypershell/*"' /etc/pf.conf)
      if echo "${pf_rules}" | sudo pfctl -f - 2>/dev/null && \
         echo "rdr pass on lo0 inet proto tcp from any to 127.0.0.1 port 443 -> 127.0.0.1 port ${ephemeral_port}" \
           | sudo pfctl -a "${PF_ANCHOR}" -f - 2>/dev/null && \
         sudo pfctl -E 2>/dev/null; then
        PORT_FORWARD_ACTIVE=true
        success "Port forwarding active: https://localhost:443 -> :${ephemeral_port}"
      else
        warn "pfctl setup failed - access services on port ${ephemeral_port} instead"
      fi
      ;;
    Linux)
      info "Setting up port forwarding: 443 -> ${ephemeral_port} (iptables)..."
      sudo iptables -t nat -N "${IPTABLES_CHAIN}" 2>/dev/null || \
        sudo iptables -t nat -F "${IPTABLES_CHAIN}"
      if sudo iptables -t nat -A "${IPTABLES_CHAIN}" -p tcp -d 127.0.0.1 --dport 443 \
           -j REDIRECT --to-port "${ephemeral_port}" && \
         sudo iptables -t nat -C OUTPUT -j "${IPTABLES_CHAIN}" 2>/dev/null || \
         sudo iptables -t nat -A OUTPUT -j "${IPTABLES_CHAIN}"; then
        PORT_FORWARD_ACTIVE=true
        success "Port forwarding active: https://localhost:443 -> :${ephemeral_port}"
      else
        warn "iptables setup failed - access services on port ${ephemeral_port} instead"
      fi
      ;;
  esac
}

stop_port_forward() {
  case "$(uname -s)" in
    Darwin)
      sudo pfctl -a "${PF_ANCHOR}" -F all 2>/dev/null || true
      ;;
    Linux)
      sudo iptables -t nat -D OUTPUT -j "${IPTABLES_CHAIN}" 2>/dev/null || true
      sudo iptables -t nat -F "${IPTABLES_CHAIN}" 2>/dev/null || true
      sudo iptables -t nat -X "${IPTABLES_CHAIN}" 2>/dev/null || true
      ;;
  esac
}
