#!/usr/bin/env bash
# openshell-container.sh - run the Linux openshell CLI inside a container on the
# kind network.
#
# Why: the openshell CLI is distributed only as a Linux binary (extracted from
# the CLI container image). On macOS the extracted binary cannot execute
# ("exec format error"), and even a native macOS build cannot reach the gateway,
# because cloud-provider-kind publishes the gateway LoadBalancer on the kind
# container network whose IPs are not routable from the macOS host.
#
# This wrapper sidesteps both problems: it runs the CLI inside a container that
# shares the network namespace of a small socat forwarder attached to the kind
# network. Inside that namespace, *.gw.localhost resolves to 127.0.0.1 (RFC 6761
# loopback), and the forwarder relays 127.0.0.1:443 to the gateway LB envoy on
# the kind bridge, so the CLI reaches the gateway exactly as it would on Linux.
#
# Drop-in for OPENSHELL_BIN in tests/e2e/e2e-openshell.sh. Forwards all args to
# the CLI and mounts the host openshell config so `gateway add/remove/status`
# and `sandbox ...` operate on the same metadata the test writes.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
# shellcheck source=../../OPENSHELL_VERSION
source "${REPO_ROOT}/OPENSHELL_VERSION"

CONTAINER_ENGINE="${CONTAINER_ENGINE:-$(command -v podman >/dev/null 2>&1 && echo podman || echo docker)}"
CLI_IMAGE="${OPENSHELL_CONTAINER_CLI_IMAGE:-${OPENSHELL_CLI_IMAGE}:${OPENSHELL_TAG}}"
KIND_NETWORK="${OPENSHELL_KIND_NETWORK:-kind}"
FWD_NAME="${OPENSHELL_FWD_NAME:-osh-fwd}"
FWD_IMAGE="${OPENSHELL_FWD_IMAGE:-docker.io/alpine/socat:latest}"
HOST_CONFIG="${OPENSHELL_HOST_CONFIG:-${HOME}/.config/openshell}"

# Discover the gateway LoadBalancer address published by cloud-provider-kind.
_lb_ip() {
  kubectl get gateway hypershell-gw -n "${OPENSHELL_HS_NAMESPACE:-hypershell-system}" \
    -o jsonpath='{.status.addresses[0].value}' 2>/dev/null
}

# Ensure the loopback->envoy forwarder is running on the kind network.
ensure_forwarder() {
  if "${CONTAINER_ENGINE}" ps --format '{{.Names}}' 2>/dev/null | grep -qx "${FWD_NAME}"; then
    return 0
  fi
  "${CONTAINER_ENGINE}" rm -f "${FWD_NAME}" >/dev/null 2>&1 || true
  local lb_ip
  lb_ip="$(_lb_ip)"
  if [[ -z "${lb_ip}" ]]; then
    echo "openshell-container: could not discover gateway LB address (is the cluster up?)" >&2
    return 1
  fi
  # IPv4-only loopback listener: the Rust CLI prefers ::1, gets ECONNREFUSED,
  # and retries 127.0.0.1 (same trick as _kind_start_gw_socat in kind.sh).
  "${CONTAINER_ENGINE}" run -d --name "${FWD_NAME}" --network "${KIND_NETWORK}" \
    "${FWD_IMAGE}" \
    TCP4-LISTEN:443,fork,reuseaddr,bind=127.0.0.1 "TCP4:${lb_ip}:443" >/dev/null
}

ensure_forwarder

mkdir -p "${HOST_CONFIG}"

exec "${CONTAINER_ENGINE}" run --rm \
  --network "container:${FWD_NAME}" \
  -e HOME=/tmp/osh-home \
  -e OPENSHELL_GATEWAY_INSECURE="${OPENSHELL_GATEWAY_INSECURE:-true}" \
  -v "${HOST_CONFIG}:/tmp/osh-home/.config/openshell" \
  "${CLI_IMAGE}" "$@"
