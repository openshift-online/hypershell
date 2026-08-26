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
REPO_ROOT="$(cd "${SCRIPT_DIR:-$(dirname "${BASH_SOURCE[0]:-$0}")}/../.." && pwd)"

# Prefer locally-built binaries from make kind-prereqs
if [[ -d "${REPO_ROOT}/bin" ]]; then
  export PATH="${REPO_ROOT}/bin:${PATH}"
fi
if [[ "$(basename "${CONTAINER_ENGINE}")" == "podman" ]]; then
  export KIND_EXPERIMENTAL_PROVIDER=podman
fi
: "${GATEWAY_IMAGE:=quay.io/bsquizza/openshell-gateway:9d0f8cc}"
: "${KEYCLOAK_HOSTNAME:=keycloak.hypershell.localhost}"
: "${KEYCLOAK_OIDC_ISSUER:=https://${KEYCLOAK_HOSTNAME}/realms/hypershell}"
: "${KEYCLOAK_OIDC_CLIENT_ID:=hypershell-frontend}"
: "${KEYCLOAK_OIDC_AUDIENCE:=hypershell-frontend}"
: "${KIND_DNS_PORT:=5553}"
: "${CPK_LOG:=/tmp/cloud-provider-kind.log}"
DNS_CONTAINER_NAME="${KIND_CLUSTER_NAME}-dns"

# --- Cluster helpers ---

cluster_exists() {
  kind get clusters 2>/dev/null | grep -q "^${KIND_CLUSTER_NAME}$" ||
    ${CONTAINER_ENGINE} inspect "${KIND_CLUSTER_NAME}-control-plane" >/dev/null 2>&1
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

# --- Cluster CoreDNS patch (in-cluster *.hypershell.localhost resolution) ---

patch_cluster_coredns() {
  local gw_ip="$1"
  local existing
  existing=$(kube get configmap coredns -n kube-system -o jsonpath='{.data.Corefile}' 2>/dev/null || true)
  if echo "${existing}" | grep -q "hypershell.localhost"; then
    info "Cluster CoreDNS already patched for hypershell.localhost"
    return
  fi
  # All *.hypershell.localhost hosts (including keycloak) resolve to the gateway
  # LB IP so in-cluster traffic goes through the gateway's HTTPS listener on :443.
  # The gateway terminates TLS with the *.hypershell.localhost cert and forwards
  # to the backing Service. For keycloak this means the gateway pod validates
  # OIDC tokens against the canonical issuer (https://keycloak.hypershell.localhost)
  # exactly as the host does, trusting the self-signed CA via the
  # gateway-trusted-ca ConfigMap (SSL_CERT_FILE).
  info "Patching cluster CoreDNS: *.hypershell.localhost -> ${gw_ip} (gateway LB)..."
  local hosts_block
  hosts_block="hypershell.localhost:53 {
    hosts {
      ${gw_ip} keycloak.hypershell.localhost
      ${gw_ip} api.hypershell.localhost
      ${gw_ip} console.hypershell.localhost
      ${gw_ip} health.hypershell.localhost
      fallthrough
    }
  }"
  local patched
  patched="${hosts_block}
${existing}"
  kube create configmap coredns -n kube-system \
    --from-literal="Corefile=${patched}" \
    --dry-run=client -o yaml | kube apply -f -
  kube rollout restart deployment/coredns -n kube-system
  kube rollout status deployment/coredns -n kube-system --timeout=60s
  success "Cluster CoreDNS patched"
}

# --- cloud-provider-kind SHA tracking ---

# The expected commit is written to bin/.cloud-provider-kind.sha by
# `make kind-prereqs`; CPK_SHA_MARKER records the commit of the instance that is
# actually running. up.sh compares the two to decide whether a restart is needed:
# cloud-provider-kind republishes the gateway LB on new random host ports every
# time it restarts, so we only pay that port churn when the pinned build has
# genuinely changed. A missing running-marker biases toward restart so the
# pinned build is always guaranteed.
CPK_SHA_FILE="${REPO_ROOT}/bin/.cloud-provider-kind.sha"
CPK_SHA_MARKER="/tmp/hypershell-kind-${KIND_CLUSTER_NAME}-cpk.sha"

# Expected (built) SHA: prefer the marker written by kind-prereqs; fall back to
# the binary's own stamped vcs.revision so this works even if the marker is
# missing (e.g. a binary built by an older Makefile). Always returns 0 (prints
# empty when unknown) so callers under `set -e` don't abort on a lookup miss.
cpk_expected_sha() {
  if [[ -f "${CPK_SHA_FILE}" ]]; then
    tr -d '[:space:]' < "${CPK_SHA_FILE}"
    return 0
  fi
  local bin
  bin="$(command -v cloud-provider-kind 2>/dev/null || echo "${REPO_ROOT}/bin/cloud-provider-kind")"
  if [[ -x "${bin}" ]]; then
    go version -m "${bin}" 2>/dev/null |
      awk -F= '/[[:space:]]vcs.revision=/{print $2}' | tr -d '[:space:]'
  fi
  return 0
}

# Running SHA: the commit recorded when the live instance was last started.
# Always returns 0 (empty output when no marker) for the same reason.
cpk_running_sha() {
  [[ -f "${CPK_SHA_MARKER}" ]] && tr -d '[:space:]' < "${CPK_SHA_MARKER}"
  return 0
}

record_cpk_sha() {
  printf '%s\n' "$1" > "${CPK_SHA_MARKER}"
}

# --- kubectl port-forward (no-sudo fallback) ---

KUBECTL_PF_DIR="/tmp/hypershell-kind-${KIND_CLUSTER_NAME}-pf"

kubectl_port_forwards_running() {
  [[ -d "${KUBECTL_PF_DIR}" ]] || return 1
  for pf in "${KUBECTL_PF_DIR}"/*.pid; do
    [[ -f "${pf}" ]] || continue
    kill -0 "$(cat "${pf}")" 2>/dev/null && return 0
  done
  return 1
}

start_kubectl_port_forwards() {
  stop_kubectl_port_forwards
  mkdir -p "${KUBECTL_PF_DIR}"

  info "Starting kubectl port-forwards..."

  local ctx
  ctx="$(kctx)"

  nohup kubectl --context "${ctx}" port-forward svc/hypershell-api-server \
    -n "${KIND_NAMESPACE}" 8000:8000 >"${KUBECTL_PF_DIR}/api-server.log" 2>&1 &
  echo $! > "${KUBECTL_PF_DIR}/api-server.pid"

  nohup kubectl --context "${ctx}" port-forward svc/hypershell-web-console \
    -n "${KIND_NAMESPACE}" 3000:3000 >"${KUBECTL_PF_DIR}/web-console.log" 2>&1 &
  echo $! > "${KUBECTL_PF_DIR}/web-console.pid"

  nohup kubectl --context "${ctx}" port-forward svc/keycloak-service \
    -n keycloak 8080:8080 >"${KUBECTL_PF_DIR}/keycloak.log" 2>&1 &
  echo $! > "${KUBECTL_PF_DIR}/keycloak.pid"

  sleep 2

  for svc_name in api-server web-console keycloak; do
    local pf="${KUBECTL_PF_DIR}/${svc_name}.pid"
    if [[ -f "${pf}" ]] && kill -0 "$(cat "${pf}")" 2>/dev/null; then
      success "  ${svc_name}: forwarding"
    else
      warn "  ${svc_name}: failed (check ${KUBECTL_PF_DIR}/${svc_name}.log)"
    fi
  done
}

stop_kubectl_port_forwards() {
  if [[ ! -d "${KUBECTL_PF_DIR}" ]]; then return; fi
  for pf in "${KUBECTL_PF_DIR}"/*.pid; do
    [[ -f "${pf}" ]] || continue
    kill "$(cat "${pf}")" 2>/dev/null || true
  done
  rm -rf "${KUBECTL_PF_DIR}"
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
      local rdr_lines
      rdr_lines="rdr pass on lo0 inet proto tcp from any to 127.0.0.1 port 443 -> 127.0.0.1 port ${ephemeral_port}"
      if echo "${pf_rules}" | sudo pfctl -f - 2>/dev/null && \
         echo "${rdr_lines}" | sudo pfctl -a "${PF_ANCHOR}" -f - 2>/dev/null && \
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
  # Tearing down rules needs the same privileges as adding them. Skip when we
  # have no sudo (HAVE_SUDO defaults to true for callers like port-forward.sh
  # that don't set it) so up.sh's no-sudo path never blocks on a password prompt.
  if [[ "${HAVE_SUDO:-true}" == "false" ]]; then
    return
  fi
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
