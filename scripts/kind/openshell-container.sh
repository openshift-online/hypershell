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
# Pin the forwarder image by digest for reproducibility (repo convention: image
# references are pinned, not floating tags). Overridable via OPENSHELL_FWD_IMAGE.
FWD_IMAGE="${OPENSHELL_FWD_IMAGE:-docker.io/alpine/socat@sha256:24220ef2c80a2a421ea08e4624488e985330c421b6aa3329bae14b0933a1d403}"
HOST_CONFIG="${OPENSHELL_HOST_CONFIG:-${HOME}/.config/openshell}"
# Namespace of the shared HyperShell Gateway. Reuses the suite-wide knob
# (E2E_HS_NAMESPACE) so a suite-level override propagates to the LB lookup.
HS_NAMESPACE="${E2E_HS_NAMESPACE:-hypershell-system}"

# Discover the gateway LoadBalancer address published by cloud-provider-kind.
_lb_ip() {
  kubectl get gateway hypershell-gw -n "${HS_NAMESPACE}" \
    -o jsonpath='{.status.addresses[0].value}' 2>/dev/null
}

# Ensure the loopback->envoy forwarder is running on the kind network and points
# at the CURRENT gateway LB IP. The socat target is baked in at creation, so a
# forwarder left over from a previous cluster would silently relay to a dead IP
# after a kind teardown+recreate. We stamp the target IP as a label and recreate
# the forwarder whenever it drifts (this also self-heals a leaked container).
ensure_forwarder() {
  local lb_ip
  lb_ip="$(_lb_ip)"
  if [[ -z "${lb_ip}" ]]; then
    echo "openshell-container: could not discover gateway LB address (is the cluster up?)" >&2
    return 1
  fi

  if "${CONTAINER_ENGINE}" ps --format '{{.Names}}' 2>/dev/null | grep -qx "${FWD_NAME}"; then
    local running_ip
    running_ip="$("${CONTAINER_ENGINE}" inspect "${FWD_NAME}" \
      --format '{{index .Config.Labels "osh.lb_ip"}}' 2>/dev/null || true)"
    if [[ "${running_ip}" == "${lb_ip}" ]]; then
      return 0
    fi
    echo "openshell-container: gateway LB IP changed (${running_ip:-unknown} -> ${lb_ip}); recreating ${FWD_NAME}" >&2
  fi
  "${CONTAINER_ENGINE}" rm -f "${FWD_NAME}" >/dev/null 2>&1 || true

  # IPv4-only loopback listener: the Rust CLI prefers ::1, gets ECONNREFUSED,
  # and retries 127.0.0.1 (same trick as _kind_start_gw_socat in kind.sh).
  "${CONTAINER_ENGINE}" run -d --name "${FWD_NAME}" --network "${KIND_NETWORK}" \
    --label "osh.lb_ip=${lb_ip}" \
    "${FWD_IMAGE}" \
    TCP4-LISTEN:443,fork,reuseaddr,bind=127.0.0.1 "TCP4:${lb_ip}:443" >/dev/null
}

ensure_forwarder

mkdir -p "${HOST_CONFIG}"

# Run the CLI as the invoking user, not the image's baked-in USER (uid 1000).
# On native Linux, Docker preserves host ownership on bind mounts, so a uid-1000
# process cannot read the host openshell config (mode 0600, owned by the calling
# user unless that user happens to be uid 1000) and the CLI reports "No gateway
# configured". macOS Docker Desktop ignores host uids on shares, so --user is a
# harmless no-op there. Rootless podman maps the host user to container root by
# default, so --user alone would still land on an unreadable mount; --userns=
# keep-id maps the host uid to itself inside the container so the config is
# owned by the same uid the CLI runs as.
RUN_ARGS=(
  --rm
  --network "container:${FWD_NAME}"
)
if [[ "${CONTAINER_ENGINE}" == *podman* ]]; then
  RUN_ARGS+=(--userns=keep-id)
fi
# HOME is a writable tmpfs so any cache/log writes outside the config mount
# succeed; the config bind mount nests under it (Docker mounts the parent tmpfs
# first, then the child bind). mode=1777 (not uid=/gid=) makes it writable by the
# --user process: uid/gid are not in Docker's documented --tmpfs option set and
# some Docker versions reject them ("unknown mount option"), while mode is
# portable and 1777 lets any uid create entries.
RUN_ARGS+=(
  --user "$(id -u):$(id -g)"
  --tmpfs "/tmp/osh-home:mode=1777"
  -e HOME=/tmp/osh-home
  -e OPENSHELL_GATEWAY_INSECURE="${OPENSHELL_GATEWAY_INSECURE:-true}"
  -v "${HOST_CONFIG}:/tmp/osh-home/.config/openshell"
)

exec "${CONTAINER_ENGINE}" run "${RUN_ARGS[@]}" "${CLI_IMAGE}" "$@"
