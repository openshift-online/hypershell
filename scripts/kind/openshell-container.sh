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
# This wrapper runs the CLI in a container on the kind network and adds
# --add-host entries so the gateway hostname (e.g. *.gw.localhost) and OIDC
# issuer hostname resolve to the cloud-provider-kind Envoy LB IP inside the
# container. The LB is directly reachable on port 443 from within the kind
# network, so no port-forwarding socat is needed.
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
HOST_CONFIG="${OPENSHELL_HOST_CONFIG:-${HOME}/.config/openshell}"
# Namespace of the shared HyperShell Gateway. Reuses the suite-wide knob
# (E2E_HS_NAMESPACE) so a suite-level override propagates to the LB lookup.
HS_NAMESPACE="${E2E_HS_NAMESPACE:-hypershell-system}"

# Discover the gateway LoadBalancer address published by cloud-provider-kind.
_lb_ip() {
  kubectl get gateway hypershell-gw -n "${HS_NAMESPACE}" \
    -o jsonpath='{.status.addresses[0].value}' 2>/dev/null
}

lb_ip="$(_lb_ip)"
if [[ -z "${lb_ip}" ]]; then
  echo "openshell-container: could not discover gateway LB address (is the cluster up?)" >&2
  exit 1
fi

mkdir -p "${HOST_CONFIG}"

# Build --add-host entries from the registered gateway metadata so the CLI can
# resolve gateway and OIDC hostnames to the kind LB IP inside the container.
ADD_HOST_ARGS=()
while IFS= read -r host; do
  [[ -n "${host}" ]] && ADD_HOST_ARGS+=(--add-host "${host}:${lb_ip}")
done < <(python3 -c "
import json, glob, urllib.parse, sys, os
config_dir = sys.argv[1]
hosts = set()
for path in glob.glob(os.path.join(config_dir, 'gateways', '*', 'metadata.json')):
    try:
        with open(path) as f:
            meta = json.load(f)
    except Exception:
        continue
    for key in ('gateway_endpoint', 'oidc_issuer'):
        h = urllib.parse.urlparse(meta.get(key, '') or '').hostname
        if h:
            hosts.add(h)
print('\n'.join(sorted(hosts)))
" "${HOST_CONFIG}" 2>/dev/null)

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
  --network "${KIND_NETWORK}"
  "${ADD_HOST_ARGS[@]+${ADD_HOST_ARGS[@]}}"
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
