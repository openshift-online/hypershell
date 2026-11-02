#!/usr/bin/env bash
# Run the Playwright live-console suite inside the official Playwright image on
# the Kind network (E2E_CONSOLE_CONTAINER=true make e2e-console).
# Spec: specs/platform/e2e-console-browser-testing.spec.md (Container mode).
#
# Why: on the host, the suite needs port 443 forwarding (sudo) to reach
# *.hypershell.localhost, and on Linux, Chromium's system libraries. Inside the
# Playwright image, Chromium and its libraries are present, and on the `kind`
# network the cloud-provider-kind Envoy answers on port 443 directly.
#
# What runs where:
#   - The test runner is the workspace's own Playwright (the repository is
#     mounted at its host path), so the test code and versions do not change.
#   - The image tag matches that Playwright version, so its bundled Chromium is
#     the revision the runner expects.
#   - Node-side requests (page.request) resolve the console host through
#     --add-host. Chromium sends *.localhost names to loopback whatever
#     /etc/hosts says, so the browser gets a --host-resolver-rules mapping
#     instead (E2E_BROWSER_HOST_RESOLVER_RULES, read by playwright.live.config.ts).
#
# Environment:
#   CONTAINER_ENGINE                podman (preferred) or docker
#   E2E_CONSOLE_KIND_NETWORK        container network of the Kind nodes (default: kind)
#   E2E_HS_NAMESPACE                namespace of the hypershell-gw Gateway (default: hypershell-system)
#   E2E_CONSOLE_PLAYWRIGHT_IMAGE    image override (default: matches the workspace Playwright)
#   E2E_CONSOLE_CONTAINER_DRY_RUN   true prints the container command instead of running it
#   E2E_CONSOLE_PLAYWRIGHT_PACKAGE  package.json the Playwright version is read from (tests)
# All other E2E_* variables (and CI) are passed into the container.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
WEB_CONSOLE_DIR="${REPO_ROOT}/components/web-console"

CONTAINER_ENGINE="${CONTAINER_ENGINE:-$(command -v podman >/dev/null 2>&1 && echo podman || echo docker)}"
KIND_NETWORK="${E2E_CONSOLE_KIND_NETWORK:-kind}"
HS_NAMESPACE="${E2E_HS_NAMESPACE:-hypershell-system}"

die() {
  echo "e2e-console-container: $*" >&2
  exit 1
}

# Overridable so tests can run without pnpm install.
playwright_pkg="${E2E_CONSOLE_PLAYWRIGHT_PACKAGE:-${WEB_CONSOLE_DIR}/node_modules/@playwright/test/package.json}"
[[ -f "${playwright_pkg}" ]] || die "Playwright is not installed; run pnpm install first"
playwright_version="$(python3 -c 'import json, sys; print(json.load(open(sys.argv[1]))["version"])' "${playwright_pkg}")"
image="${E2E_CONSOLE_PLAYWRIGHT_IMAGE:-mcr.microsoft.com/playwright:v${playwright_version}-noble}"

# The cloud-provider-kind Envoy address, as published on the shared Gateway.
lb_ip="$(kubectl get gateway hypershell-gw -n "${HS_NAMESPACE}" \
  -o jsonpath='{.status.addresses[0].value}' 2>/dev/null || true)"
[[ -n "${lb_ip}" ]] || die "could not read the hypershell-gw Gateway address (is the Kind cluster up?)"

console_url="${E2E_CONSOLE_URL:-https://console.hypershell.localhost}"
console_host="$(python3 -c 'import sys, urllib.parse; print(urllib.parse.urlparse(sys.argv[1]).hostname or "")' "${console_url}")"
[[ -n "${console_host}" ]] || die "cannot read a host name from E2E_CONSOLE_URL=${console_url}"

run_args=(
  --rm
  --network "${KIND_NETWORK}"
  # Chromium needs more shared memory than the container default (64 MB).
  # A private /dev/shm, not --ipc=host: podman refuses --ipc=host together with
  # --userns=keep-id ("mount mqueue: Operation not permitted", podman 6).
  --shm-size=2g
  --add-host "${console_host}:${lb_ip}"
)
# Run as the invoking user so artifacts written to the mounted repository keep
# the user's ownership. Rootless podman maps the host user to container root by
# default; keep-id maps it to itself (same as scripts/kind/openshell-container.sh).
if [[ "$(basename "${CONTAINER_ENGINE}")" == podman ]]; then
  run_args+=(--userns=keep-id)
fi
run_args+=(
  --user "$(id -u):$(id -g)"
  --tmpfs "/tmp/pw-home:mode=1777"
  -e HOME=/tmp/pw-home
  -e E2E_CONSOLE_URL="${console_url}"
  -e "E2E_BROWSER_HOST_RESOLVER_RULES=MAP *.localhost ${lb_ip}"
  # :z relabels the mount on SELinux hosts; shared, so concurrent runs are safe.
  -v "${REPO_ROOT}:${REPO_ROOT}:z"
  -w "${WEB_CONSOLE_DIR}"
)
while IFS= read -r name; do
  case "${name}" in
    E2E_CONSOLE_URL | E2E_BROWSER_HOST_RESOLVER_RULES | E2E_CONSOLE_CONTAINER_DRY_RUN | E2E_CONSOLE_PLAYWRIGHT_PACKAGE) ;;
    E2E_* | CI) run_args+=(-e "${name}") ;;
  esac
done < <(compgen -e)

command=(./node_modules/.bin/playwright test --config playwright.live.config.ts console.live.spec.ts)

if [[ "${E2E_CONSOLE_CONTAINER_DRY_RUN:-false}" == "true" ]]; then
  printf '%s\n' "${CONTAINER_ENGINE}" run "${run_args[@]}" "${image}" "${command[@]}"
  exit 0
fi

if ! "${CONTAINER_ENGINE}" network inspect "${KIND_NETWORK}" >/dev/null 2>&1; then
  die "container network '${KIND_NETWORK}' not found (is the Kind cluster up?)"
fi
echo "Running the console suite in ${image} on the '${KIND_NETWORK}' network (Envoy ${lb_ip})"
exec "${CONTAINER_ENGINE}" run "${run_args[@]}" "${image}" "${command[@]}"
