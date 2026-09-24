#!/usr/bin/env bash
# kind.sh - Kind infrastructure driver for e2e tests.
#
# Implements the driver interface contract using kubectl, Gateway API status,
# and Kind-specific conventions (HTTPRoute hostnames, GRPCRoute discovery).
#
# The driver returns results via global variables (_DISCOVER_API_HOST,
# _DISCOVER_GW_ENDPOINT, _OIDC_ACCESS_TOKEN) rather than stdout: some e2e
# operations start background processes (e.g. a gateway port-forward) that must
# survive in the parent shell, which a $() subshell would orphan and kill.

# Kind uses locally issued certificates. Other drivers may override this seam
# while reusing the production OIDC and role-assignment behavior below.

# Auto-detect container engine (docker or podman) if not explicitly set.
if [[ -z "${CONTAINER_ENGINE:-}" ]]; then
  if command -v podman &>/dev/null; then
    export CONTAINER_ENGINE=podman
  elif command -v docker &>/dev/null; then
    export CONTAINER_ENGINE=docker
  fi
fi

# Kind's self-signed CA is not in the system trust store. Instruct the
# openshell CLI to skip TLS verification for gateway connections. curl already
# uses -sk (insecure) for all driver requests; this extends the same treatment
# to the openshell binary.
export OPENSHELL_GATEWAY_INSECURE=true

# _kind_host_is_dual_stack - true when ::1 is a usable local address, i.e. the
# host has IPv6 loopback and *.gw.localhost will resolve dual-stack. Binding to
# ::1 succeeds only when the kernel has the address configured; it fails on the
# IPv4-only hosts CI runs on. Portable across macOS and Linux.
_kind_host_is_dual_stack() {
  python3 -c "import socket, sys
try:
    s = socket.socket(socket.AF_INET6, socket.SOCK_STREAM)
    s.bind(('::1', 0)); s.close(); sys.exit(0)
except OSError:
    sys.exit(1)" 2>/dev/null
}

# Run the openshell CLI inside a container on the kind network when the native
# binary cannot work:
#   - macOS: the CLI is a Linux binary that cannot execute on the host, and even
#     a native build could not reach the gateway LB (cloud-provider-kind
#     publishes it on the kind container network, not routable from macOS).
#   - dual-stack Linux: the CLI (>=0.0.116) breaks on a dual-stack
#     *.gw.localhost DNS answer (prefers ::1, does not fall back, can segfault).
#     The only sudo-free way to strip ::1 is the container's own single-stack
#     resolver (*.localhost -> 127.0.0.1 inside the netns); editing /etc/hosts
#     would need sudo, which the e2e suite must never require.
# scripts/kind/openshell-container.sh runs the Linux CLI in a container on the
# kind network with --add-host entries pointing gateway/OIDC hostnames at the
# cloud-provider-kind Envoy LB IP (see that script's header).
#
# IPv4-only Linux (CI) keeps the fast native-binary path: no ::1 in the answer,
# nothing to strip. Any explicit OPENSHELL_BIN override is honored. Guarded to
# the kind driver: the OpenShift driver sources this file for its shared OIDC
# helpers and manages its own CLI, so the wrapper must not hijack OPENSHELL_BIN
# on a dual-stack OpenShift run.
if [[ "${E2E_INFRA_DRIVER:-}" == "kind" ]] \
   && [[ ( -z "${OPENSHELL_BIN:-}" || "${OPENSHELL_BIN}" == "openshell" ) ]] \
   && { [[ "$(uname -s)" == "Darwin" ]] || _kind_host_is_dual_stack; }; then
  _E2E_OSH_WRAPPER="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)/scripts/kind/openshell-container.sh"
  if [[ ! -x "${_E2E_OSH_WRAPPER}" ]]; then
    red "ERROR: openshell container wrapper not found or not executable: ${_E2E_OSH_WRAPPER}"
    red "  The native CLI cannot be used here ($([[ "$(uname -s)" == "Darwin" ]] && echo macOS || echo 'dual-stack host')) without sudo /etc/hosts edits, which the e2e suite forbids."
    exit 1
  fi
  if [[ -z "${CONTAINER_ENGINE:-}" ]]; then
    red "ERROR: no container engine (podman/docker) found for the openshell CLI wrapper."
    red "  A dual-stack host must run the CLI in a container to avoid a sudo /etc/hosts edit."
    red "  Install podman or docker, or run the suite on an IPv4-only host."
    exit 1
  fi
  export OPENSHELL_BIN="${_E2E_OSH_WRAPPER}"
  # The wrapper runs the Linux CLI straight from the container image, so there
  # is nothing to download or extract onto the host.
  export E2E_OPENSHELL_INSTALL=never
  # The wrapper's in-container forwarder listens on loopback :443, so the CLI
  # gateway endpoint must target :443 rather than the host-published ephemeral
  # port that discover_gateway_endpoint would otherwise bake into metadata.json.
  : "${_KINDCCM_GW_PORT:=443}"
fi

# Force IPv4 and remap *.hypershell.localhost:443 to the cloud-provider-kind
# envoy ephemeral port. Two problems motivate this:
#   1. DNS stub returns both 127.0.0.1 and ::1 for *.localhost; the envoy proxy
#      only binds IPv4, so curl must prefer IPv4 (--ipv4).
#   2. Without sudo, iptables cannot redirect port 443 to the ephemeral port
#      (typically 32768). curl's --connect-to lets us rewrite the TCP target at
#      the connection layer while keeping the SNI as the original hostname, so
#      the envoy proxy can route by hostname as normal.
_KINDCCM_PORT="${_KINDCCM_PORT:-}"
# _KINDCCM_GW_PORT: IPv4-only socat port used exclusively for the openshell CLI
# gateway endpoint. curl requests use _KINDCCM_PORT directly (--ipv4 already
# prevents IPv6). The socat forwarder is started lazily by discover_gateway_endpoint.
_KINDCCM_GW_PORT="${_KINDCCM_GW_PORT:-}"
_KINDCCM_SOCAT_PID="${_KINDCCM_SOCAT_PID:-}"
_kind_discover_port() {
  if [[ -z "${_KINDCCM_PORT}" ]]; then
    local proxy_container
    proxy_container=$(${CONTAINER_ENGINE:-docker} ps -q --filter "name=kindccm-gw" 2>/dev/null | head -1)
    if [[ -n "$proxy_container" ]]; then
      _KINDCCM_PORT=$(${CONTAINER_ENGINE:-docker} port "${proxy_container}" 443 2>/dev/null \
        | head -1 | grep -oE '[0-9]+$' || true)
    fi
  fi
}
# _kind_start_gw_socat - front the cloud-provider-kind envoy port with an
# IPv4-only loopback listener for the openshell CLI gateway endpoint, giving the
# endpoint a stable port instead of the ephemeral kindccm port.
#
# This runs only on the native-binary path, which the driver restricts to
# IPv4-only hosts (see the wrapper-activation block above): a dual-stack host
# routes the CLI through the container wrapper instead, because the CLI
# (>=0.0.116) breaks on a dual-stack *.gw.localhost DNS answer and the only
# sudo-free way to strip ::1 is the container's single-stack resolver. So here
# there is no ::1 in the answer and the IPv4 listener is always the one hit.
# curl is unaffected: it hits the raw envoy port with --ipv4. Sets
# _KINDCCM_GW_PORT.
_kind_start_gw_socat() {
  [[ -n "${_KINDCCM_GW_PORT}" ]] && return
  _kind_discover_port
  local raw_port="${_KINDCCM_PORT}"
  # When sudo set up iptables (port 443 redirected) or the container wrapper
  # handles forwarding, socat isn't needed: the CLI reaches port 443 directly.
  if [[ -z "${raw_port}" || "${raw_port}" == "443" ]]; then
    _KINDCCM_GW_PORT="${raw_port:-443}"
    return
  fi
  if ! command -v socat &>/dev/null; then
    # socat unavailable; fall back to the raw ephemeral port.
    _KINDCCM_GW_PORT="${raw_port}"
    return
  fi
  local socat_port
  socat_port=$(python3 -c "import socket; s=socket.socket(); s.bind(('127.0.0.1',0)); p=s.getsockname()[1]; s.close(); print(p)" 2>/dev/null)
  if [[ -z "${socat_port}" ]]; then
    _KINDCCM_GW_PORT="${raw_port}"
    return
  fi
  socat TCP4-LISTEN:"${socat_port}",bind=127.0.0.1,reuseaddr,fork \
    TCP4:127.0.0.1:"${raw_port}" &>/dev/null &
  _KINDCCM_SOCAT_PID=$!
  _KINDCCM_GW_PORT="${socat_port}"
}

_driver_curl() {
  # Try direct HTTPRoute access first. This works in most setups where the
  # routes are directly accessible on port 443 (docker, podman, or iptables-
  # forwarded 443). If direct access fails (connection refused/timeout), fall
  # back to port-remapping via the ephemeral kindccm-gw port.
  local output status
  output=$(curl -sk --ipv4 "$@" 2>&1)
  status=$?

  # If direct access failed due to connection error, try port-remapping
  if [[ $status -eq 7 ]]; then
    _kind_discover_port
    local connect_args=()
    if [[ -n "${_KINDCCM_PORT}" && "${_KINDCCM_PORT}" != "443" ]]; then
      connect_args+=(
        --connect-to "api.hypershell.localhost:443:127.0.0.1:${_KINDCCM_PORT}"
        --connect-to "keycloak.hypershell.localhost:443:127.0.0.1:${_KINDCCM_PORT}"
        --connect-to "console.hypershell.localhost:443:127.0.0.1:${_KINDCCM_PORT}"
        --connect-to "health.hypershell.localhost:443:127.0.0.1:${_KINDCCM_PORT}"
        --connect-to "observability.hypershell.localhost:443:127.0.0.1:${_KINDCCM_PORT}"
      )
      output=$(curl -sk --ipv4 "${connect_args[@]}" "$@" 2>&1)
      status=$?
    fi
  fi

  echo "$output"
  return $status
}

# discover_api_host - find the HyperShell API server base URL.
# Sets _DISCOVER_API_HOST to the gateway HTTPS route for the API server. There
# is no HTTP/port-forward fallback: the HTTPS HTTPRoute is the only supported
# ingress, so an unreachable route is a real failure that must surface rather
# than be masked by a plain-HTTP port-forward.
discover_api_host() {
  _DISCOVER_API_HOST=""
  local host
  host=$(kubectl get httproute -A -o jsonpath='{range .items[*]}{.spec.hostnames[0]}{"\n"}{end}' 2>/dev/null \
    | grep -m1 'api\.hypershell\.localhost' || true)
  if [[ -z "$host" ]]; then
    red "  No HTTPRoute with hostname api.hypershell.localhost found"
    return 1
  fi

  local url="https://${host}"
  local code
  code=$(_driver_curl --connect-timeout 5 -o /dev/null -w '%{http_code}' \
    "${url}/api/hypershell/v1/gateways" 2>/dev/null || true)

  # Any HTTP response (401 unauthenticated, 200, 404, ...) proves the route
  # reaches the API server. "000" means the connection never completed -- route
  # not programmed, 443->LB mapping down, or the api-server pod not serving.
  # _driver_curl handles port remapping transparently when iptables is unavailable.
  if [[ -z "$code" || "$code" == "000" ]]; then
    red "  API route ${url} is not reachable (no HTTP response)"
    red "  Verify: Gateway Programmed, api-server pod Ready, and 443->LB mapping active"
    return 1
  fi

  _DISCOVER_API_HOST="${url}"
}

# discover_console_host - find the HyperShell web console (BFF) base URL.
# Sets _DISCOVER_CONSOLE_HOST to the gateway HTTPS route for the web console.
discover_console_host() {
  _DISCOVER_CONSOLE_HOST=""
  local host
  host=$(kubectl get httproute -A -o jsonpath='{range .items[*]}{.spec.hostnames[0]}{"\n"}{end}' 2>/dev/null \
    | grep -m1 'console\.hypershell\.localhost' || true)
  if [[ -z "$host" ]]; then
    red "  No HTTPRoute with hostname console.hypershell.localhost found"
    return 1
  fi

  local url="https://${host}"
  local code
  code=$(_driver_curl --connect-timeout 5 -o /dev/null -w '%{http_code}' \
    "${url}/auth/session" 2>/dev/null || true)
  if [[ -z "$code" || "$code" == "000" ]]; then
    red "  Console route ${url} is not reachable (no HTTP response)"
    return 1
  fi

  _DISCOVER_CONSOLE_HOST="${url}"
}

# discover_gateway_endpoint - find the gateway gRPC endpoint.
# Sets _DISCOVER_GW_ENDPOINT from the GRPCRoute hostname once the
# parent Gateway is Programmed. Also sets _DISCOVER_GW_HOST (the bare
# hostname) and _DISCOVER_GW_LB_ADDR (the parent Gateway's own
# status.addresses[0].value, i.e. the cloud-provider-kind Envoy container's
# address on Kind's own podman network) so callers that run the openshell CLI
# in a container on that same network (see e2e-openshell.sh's
# _install_openshell_cli_container_wrapper) can reach it directly, instead of
# through the host-side ephemeral-port remap _DISCOVER_GW_ENDPOINT uses.
discover_gateway_endpoint() {
  _DISCOVER_GW_ENDPOINT=""
  _DISCOVER_GW_HOST=""
  _DISCOVER_GW_LB_ADDR=""
  local gw_name="${1:?gateway name required}"
  local gw_namespace="${2:?gateway namespace required}"

  local grpc_host
  grpc_host=$(kubectl get grpcroute openshell-gateway -n "${gw_namespace}" \
    -o jsonpath='{.spec.hostnames[0]}' 2>/dev/null || true)

  if [[ -n "$grpc_host" ]]; then
    local gw_ref_name gw_ref_ns gw_programmed
    gw_ref_name=$(kubectl get grpcroute openshell-gateway -n "${gw_namespace}" \
      -o jsonpath='{.spec.parentRefs[0].name}' 2>/dev/null || true)
    gw_ref_ns=$(kubectl get grpcroute openshell-gateway -n "${gw_namespace}" \
      -o jsonpath='{.spec.parentRefs[0].namespace}' 2>/dev/null || true)

    if [[ -n "$gw_ref_name" && -n "$gw_ref_ns" ]]; then
      gw_programmed=$(kubectl get gateway "${gw_ref_name}" -n "${gw_ref_ns}" \
        -o jsonpath='{range .status.conditions[*]}{.type}={.status}{"\n"}{end}' 2>/dev/null \
        | grep -c 'Programmed=True' || true)
      if [[ "${gw_programmed:-0}" -ge 1 ]]; then
        _DISCOVER_GW_HOST="$grpc_host"
        _DISCOVER_GW_LB_ADDR=$(kubectl get gateway "${gw_ref_name}" -n "${gw_ref_ns}" \
          -o jsonpath='{.status.addresses[0].value}' 2>/dev/null || true)
        _kind_start_gw_socat
        if [[ -n "${_KINDCCM_GW_PORT}" && "${_KINDCCM_GW_PORT}" != "443" ]]; then
          _DISCOVER_GW_ENDPOINT="https://${grpc_host}:${_KINDCCM_GW_PORT}"
        else
          _DISCOVER_GW_ENDPOINT="https://${grpc_host}:443"
        fi
        return
      fi
    fi
  fi

  dim "  No programmed Gateway route found for ${gw_name}"
  return 1
}

# acquire_oidc_token - get an OIDC access token from Keycloak.
# Sets _OIDC_ACCESS_TOKEN via resource-owner password grant against the gateway
# HTTPS route (the same issuer the API server validates against). No port-forward
# is used -- Keycloak is reached over its external HTTPS route like every other
# component, and -k trusts the Kind self-signed CA.
# Usage: acquire_oidc_token [username] [password] [client_id]
#   Falls back to E2E_OIDC_USERNAME / E2E_OIDC_PASSWORD / E2E_OIDC_CLIENT_ID.
#
# The client_id override is essential in Kind: the gateway reconciler provisions a
# dedicated public Keycloak client per gateway ("${gw.Name}-${gatewayID}") with an
# audience mapper, and the gateway's Envoy validates aud == that client. A token
# from the shared frontend client is rejected with InvalidAudience, so gateway and
# CLI calls must mint tokens against the per-gateway client.
# _driver_token_request - POST the given form fields to the realm token endpoint
# and set _OIDC_ACCESS_TOKEN from the access_token field. Every grant flow funnels
# through here so error handling and JSON parsing stay in one place.
_driver_token_request() {
  local token_endpoint="${E2E_OIDC_ISSUER}/protocol/openid-connect/token"
  local response
  response=$(_driver_curl -X POST "${token_endpoint}" "$@" 2>/dev/null || true)

  _OIDC_ACCESS_TOKEN=$(echo "$response" | python3 -c "import json,sys; print(json.load(sys.stdin).get('access_token',''))" 2>/dev/null || true)

  if [[ -z "$_OIDC_ACCESS_TOKEN" || "$_OIDC_ACCESS_TOKEN" == "None" ]]; then
    _OIDC_ACCESS_TOKEN=""
    dim "  Token error: $(echo "$response" | python3 -c "import json,sys; print(json.load(sys.stdin).get('error_description','unknown'))" 2>/dev/null || echo 'no response')"
    return 1
  fi
}

# _driver_acquire_oidc_token - obtain an OIDC access token, honoring E2E_OIDC_GRANT
# (ephemeral-pr-environments.spec.md). Keeps the same [username] [password]
# [client_id] signature and call sites regardless of grant.
#
#   password (default) -- resource-owner password grant against the seeded user.
#       The Kind path and the default for manual OpenShift runs.
#   client_credentials -- the GitHub-brokered pull-request path. Brokered GitHub
#       users have no password grant, so tokens come from the confidential
#       hypershell-e2e client instead:
#         * admin HyperShell API token (username == E2E_OIDC_USERNAME and
#           client_id == E2E_OIDC_CLIENT_ID) -- client-credentials on
#           hypershell-e2e.
#         * admin per-gateway token (username == E2E_OIDC_USERNAME, a
#           different client_id) -- token-exchange of the hypershell-e2e
#           service account onto that gateway audience. The SA is the
#           gateway owner on this path, so impersonating the seeded
#           passworded admin user would never carry openshell-admin.
#         * every other call (developer, platform-admin, ...) --
#           token-exchange impersonating that principal for the requested
#           audience. Never a password grant.
_driver_acquire_oidc_token() {
  _OIDC_ACCESS_TOKEN=""
  local username="${1:-${E2E_OIDC_USERNAME}}"
  local password="${2:-${E2E_OIDC_PASSWORD}}"
  local client_id="${3:-${E2E_OIDC_CLIENT_ID}}"

  case "${E2E_OIDC_GRANT:-password}" in
    password)
      _driver_token_request \
        -d "grant_type=password" \
        -d "client_id=${client_id}" \
        -d "username=${username}" \
        -d "password=${password}"
      ;;
    client_credentials)
      if [[ -z "${E2E_OIDC_SA_CLIENT_SECRET:-}" ]]; then
        red "  E2E_OIDC_GRANT=client_credentials requires E2E_OIDC_SA_CLIENT_SECRET"
        red "  (the ${E2E_OIDC_SA_CLIENT_ID} client secret read from the Keycloak namespace after openshift-up)"
        return 1
      fi
      if [[ "${username}" == "${E2E_OIDC_USERNAME}" && "${client_id}" == "${E2E_OIDC_CLIENT_ID}" ]]; then
        # Admin HyperShell API token: client-credentials on hypershell-e2e.
        # Per-gateway tokens (a different client_id) must not use this path:
        # the CC token is issued for hypershell-e2e / hypershell-frontend and
        # never carries openshell-admin on the gateway client.
        _driver_token_request \
          -d "grant_type=client_credentials" \
          -d "client_id=${E2E_OIDC_SA_CLIENT_ID}" \
          -d "client_secret=${E2E_OIDC_SA_CLIENT_SECRET}"
      else
        # Token-exchange targeting that audience. Keycloak 26 standard
        # token-exchange rejects requested_subject; impersonation uses the
        # legacy token-exchange feature and needs a subject_token (the
        # hypershell-e2e client-credentials token).
        if ! _driver_token_request \
          -d "grant_type=client_credentials" \
          -d "client_id=${E2E_OIDC_SA_CLIENT_ID}" \
          -d "client_secret=${E2E_OIDC_SA_CLIENT_SECRET}"; then
          return 1
        fi
        local subject_token="$_OIDC_ACCESS_TOKEN"
        local -a exchange_args=(
          -d "grant_type=urn:ietf:params:oauth:grant-type:token-exchange"
          -d "client_id=${E2E_OIDC_SA_CLIENT_ID}"
          -d "client_secret=${E2E_OIDC_SA_CLIENT_SECRET}"
          -d "subject_token=${subject_token}"
          -d "subject_token_type=urn:ietf:params:oauth:token-type:access_token"
        )
        # The CI admin identity is the hypershell-e2e service account, which
        # owns gateways it creates. Impersonating the seeded admin user
        # would mint a token for a principal with no RoleBinding on that
        # gateway. Developer (and other) principals still need impersonation.
        if [[ "${username}" != "${E2E_OIDC_USERNAME}" ]]; then
          exchange_args+=(-d "requested_subject=${username}")
        fi
        exchange_args+=(-d "audience=${client_id}")
        _driver_token_request "${exchange_args[@]}"
      fi
      ;;
    *)
      red "  Unknown E2E_OIDC_GRANT '${E2E_OIDC_GRANT}' (valid: password, client_credentials)"
      return 1
      ;;
  esac
}

acquire_oidc_token() {
  _driver_acquire_oidc_token "$@"
}

# Kind and CI-owned OpenShift users must not be deleted by a test run
# (ephemeral-test-credentials.spec.md). They last until the environment does.
de_seed_test_users() {
  return 0
}

: "${E2E_GATEWAY_NAMESPACE_GC_INTERVAL:=30s}"
: "${E2E_GATEWAY_NAMESPACE_GC_GRACE_PERIOD:=30s}"
_GC_TIMING_PATCHED=""

# _patch_namespace_gc_timing / _restore_namespace_gc_timing - shared
# implementation for configure_namespace_gc_timing / restore_namespace_gc_timing.
# Kind deploys the same manifests as production (deploy/base/), so it also runs
# with production namespace-GC defaults (5m sweep / 10m grace) -- too slow for
# the e2e orphan-GC assertion (area 11a) to wait out. Rather than bake e2e-only
# timing into any deploy overlay, every driver patches the controller
# deployment to a short interval/grace period for the duration of the run and
# restores it afterward. Drivers that need to resolve their own namespace
# first (e.g. OpenShift) override the public functions and delegate here.
_patch_namespace_gc_timing() {
  local cli="$1" namespace="$2"
  dim "  Shortening controller namespace GC timing for e2e (interval=${E2E_GATEWAY_NAMESPACE_GC_INTERVAL}, grace=${E2E_GATEWAY_NAMESPACE_GC_GRACE_PERIOD})..."
  if ! "$cli" set env deployment/hypershell-controller -n "$namespace" -c controller \
      "GATEWAY_NAMESPACE_GC_INTERVAL=${E2E_GATEWAY_NAMESPACE_GC_INTERVAL}" \
      "GATEWAY_NAMESPACE_GC_GRACE_PERIOD=${E2E_GATEWAY_NAMESPACE_GC_GRACE_PERIOD}" >/dev/null; then
    red "  Failed to patch hypershell-controller namespace GC timing"
    return 1
  fi
  _GC_TIMING_PATCHED=1
  if ! "$cli" rollout status deployment/hypershell-controller -n "$namespace" --timeout=300s >/dev/null; then
    red "  hypershell-controller did not roll out after GC timing patch"
    return 1
  fi
}

_restore_namespace_gc_timing() {
  local cli="$1" namespace="$2"
  [[ -n "$_GC_TIMING_PATCHED" ]] || return 0
  dim "  Restoring controller namespace GC timing to deployment defaults..."
  "$cli" set env deployment/hypershell-controller -n "$namespace" -c controller \
    GATEWAY_NAMESPACE_GC_INTERVAL- GATEWAY_NAMESPACE_GC_GRACE_PERIOD- >/dev/null 2>&1 || true
  "$cli" rollout status deployment/hypershell-controller -n "$namespace" --timeout=300s >/dev/null 2>&1 || true
  _GC_TIMING_PATCHED=""
}

configure_namespace_gc_timing() {
  _patch_namespace_gc_timing kubectl "${E2E_HS_NAMESPACE}"
}

restore_namespace_gc_timing() {
  _restore_namespace_gc_timing kubectl "${E2E_HS_NAMESPACE}"
}

# _kc_base / _kc_realm - derive the Keycloak base URL and realm from the issuer.
# E2E_OIDC_ISSUER is "<base>/realms/<realm>".
_kc_base() { echo "${E2E_OIDC_ISSUER%/realms/*}"; }
_kc_realm() { echo "${E2E_OIDC_ISSUER##*/realms/}"; }

# _kc_admin_token - obtain a Keycloak master-realm admin access token for the
# Admin REST API. Sets _KC_ADMIN_TOKEN. Uses the admin-cli public client with the
# resource-owner password grant (the standard bootstrap admin flow).
_kc_admin_token() {
  _KC_ADMIN_TOKEN=""
  local base response
  base="$(_kc_base)"
  response=$(_driver_curl -X POST "${base}/realms/master/protocol/openid-connect/token" \
    -d "grant_type=password" \
    -d "client_id=admin-cli" \
    -d "username=${E2E_KC_ADMIN_USER}" \
    -d "password=${E2E_KC_ADMIN_PASSWORD}" 2>/dev/null || true)
  _KC_ADMIN_TOKEN=$(echo "$response" | python3 -c "import json,sys; print(json.load(sys.stdin).get('access_token',''))" 2>/dev/null || true)
  if [[ -z "$_KC_ADMIN_TOKEN" || "$_KC_ADMIN_TOKEN" == "None" ]]; then
    _KC_ADMIN_TOKEN=""
    return 1
  fi
}

# assign_gateway_client_role - grant a user a client role on a per-gateway
# Keycloak client, mirroring what the control-plane RoleBinding reconciler does in
# production. This is a test-setup shortcut: the developer's openshell-user grant
# cannot be issued through the HyperShell API (there is no user_id discovery path
# for non-owners), so we provision the same end state directly in Keycloak.
# Idempotent: re-granting an existing role mapping is a no-op on the Keycloak side.
# Usage: assign_gateway_client_role <username> <client_id> <role>
assign_gateway_client_role() {
  local username="${1:?username required}"
  local client_id="${2:?client_id required}"
  local role="${3:?role required}"

  local base realm
  base="$(_kc_base)"
  realm="$(_kc_realm)"

  if ! _kc_admin_token; then
    red "  Failed to obtain Keycloak admin token (user=${E2E_KC_ADMIN_USER})"
    return 1
  fi

  local client_uuid user_uuid role_json
  client_uuid=$(_driver_curl -H "Authorization: Bearer ${_KC_ADMIN_TOKEN}" \
    "${base}/admin/realms/${realm}/clients?clientId=${client_id}" 2>/dev/null \
    | python3 -c "import json,sys; a=json.load(sys.stdin); print(a[0]['id'] if a else '')" 2>/dev/null || true)
  if [[ -z "$client_uuid" ]]; then
    red "  Keycloak client not found: ${client_id}"
    return 1
  fi

  user_uuid=$(_driver_curl -H "Authorization: Bearer ${_KC_ADMIN_TOKEN}" \
    "${base}/admin/realms/${realm}/users?username=${username}&exact=true" 2>/dev/null \
    | python3 -c "import json,sys; a=json.load(sys.stdin); print(a[0]['id'] if a else '')" 2>/dev/null || true)
  if [[ -z "$user_uuid" ]]; then
    red "  Keycloak user not found: ${username}"
    return 1
  fi

  role_json=$(_driver_curl -H "Authorization: Bearer ${_KC_ADMIN_TOKEN}" \
    "${base}/admin/realms/${realm}/clients/${client_uuid}/roles/${role}" 2>/dev/null || true)
  local role_id role_name
  role_id=$(echo "$role_json" | python3 -c "import json,sys; print(json.load(sys.stdin).get('id',''))" 2>/dev/null || true)
  role_name=$(echo "$role_json" | python3 -c "import json,sys; print(json.load(sys.stdin).get('name',''))" 2>/dev/null || true)
  if [[ -z "$role_id" || -z "$role_name" ]]; then
    red "  Keycloak client role not found: ${role} on ${client_id}"
    return 1
  fi

  local code
  code=$(_driver_curl -o /dev/null -w '%{http_code}' -X POST \
    -H "Authorization: Bearer ${_KC_ADMIN_TOKEN}" \
    -H "Content-Type: application/json" \
    "${base}/admin/realms/${realm}/users/${user_uuid}/role-mappings/clients/${client_uuid}" \
    -d "[{\"id\":\"${role_id}\",\"name\":\"${role_name}\"}]" 2>/dev/null || true)
  # 204 = created; Keycloak also returns 204 when the mapping already exists.
  if [[ "$code" != "204" && "$code" != "200" ]]; then
    red "  Failed to assign client role ${role} to ${username} on ${client_id} (HTTP ${code})"
    return 1
  fi
}

# assign_realm_role - grant a user a realm role (e.g., platform:admin, gateway:creator).
# Realm roles are global to the realm, unlike client roles which are scoped to a
# specific client. Used for platform-wide RBAC roles.
# Idempotent: re-granting an existing role mapping is a no-op on the Keycloak side.
# Usage: assign_realm_role <username> <role>
assign_realm_role() {
  local username="${1:?username required}"
  local role="${2:?role required}"

  local base realm
  base="$(_kc_base)"
  realm="$(_kc_realm)"

  if ! _kc_admin_token; then
    red "  Failed to obtain Keycloak admin token (user=${E2E_KC_ADMIN_USER})"
    return 1
  fi

  local user_uuid role_json
  user_uuid=$(_driver_curl -H "Authorization: Bearer ${_KC_ADMIN_TOKEN}" \
    "${base}/admin/realms/${realm}/users?username=${username}&exact=true" 2>/dev/null \
    | python3 -c "import json,sys; a=json.load(sys.stdin); print(a[0]['id'] if a else '')" 2>/dev/null || true)
  if [[ -z "$user_uuid" ]]; then
    red "  Keycloak user not found: ${username}"
    return 1
  fi

  role_json=$(_driver_curl -H "Authorization: Bearer ${_KC_ADMIN_TOKEN}" \
    "${base}/admin/realms/${realm}/roles/${role}" 2>/dev/null || true)
  local role_id role_name
  role_id=$(echo "$role_json" | python3 -c "import json,sys; print(json.load(sys.stdin).get('id',''))" 2>/dev/null || true)
  role_name=$(echo "$role_json" | python3 -c "import json,sys; print(json.load(sys.stdin).get('name',''))" 2>/dev/null || true)
  if [[ -z "$role_id" || -z "$role_name" ]]; then
    red "  Keycloak realm role not found: ${role}"
    return 1
  fi

  local code
  code=$(_driver_curl -o /dev/null -w '%{http_code}' -X POST \
    -H "Authorization: Bearer ${_KC_ADMIN_TOKEN}" \
    -H "Content-Type: application/json" \
    "${base}/admin/realms/${realm}/users/${user_uuid}/role-mappings/realm" \
    -d "[{\"id\":\"${role_id}\",\"name\":\"${role_name}\"}]" 2>/dev/null || true)
  # 204 = created; Keycloak also returns 204 when the mapping already exists.
  if [[ "$code" != "204" && "$code" != "200" ]]; then
    red "  Failed to assign realm role ${role} to ${username} (HTTP ${code})"
    return 1
  fi
}

# _token_has_role - check whether a JWT carries a client role in the nested
# hypershell.roles claim (the claim path the gateway is configured with). Falls
# back to a flat "hypershell.roles" key for robustness. Returns 0 if present.
# Usage: _token_has_role <token> <role>
_token_has_role() {
  local token="${1:?token required}"
  local role="${2:?role required}"
  python3 - "$token" "$role" <<'PY'
import base64, json, sys
tok, role = sys.argv[1], sys.argv[2]
try:
    payload = tok.split('.')[1]
    payload += '=' * (-len(payload) % 4)
    claims = json.loads(base64.urlsafe_b64decode(payload))
except Exception:
    sys.exit(1)
roles = []
hs = claims.get('hypershell')
if isinstance(hs, dict):
    roles = hs.get('roles', []) or []
if not roles:
    roles = claims.get('hypershell.roles', []) or []
sys.exit(0 if role in roles else 1)
PY
}

# _token_debug_claims - print the identity and role claims from a JWT so a
# timed-out role wait shows which principal was minted, not just that the
# role was missing. Never prints the raw token.
# Usage: _token_debug_claims <token>
_token_debug_claims() {
  local token="${1:?token required}"
  python3 - "$token" <<'PY'
import base64, json, sys
tok = sys.argv[1]
try:
    payload = tok.split('.')[1]
    payload += '=' * (-len(payload) % 4)
    claims = json.loads(base64.urlsafe_b64decode(payload))
except Exception:
    print("unreadable jwt")
    sys.exit(0)
roles = []
hs = claims.get('hypershell')
if isinstance(hs, dict):
    roles = hs.get('roles', []) or []
if not roles:
    roles = claims.get('hypershell.roles', []) or []
print("preferred_username=%s sub=%s azp=%s aud=%s hypershell.roles=%s" % (
    claims.get('preferred_username', ''),
    claims.get('sub', ''),
    claims.get('azp', ''),
    claims.get('aud', ''),
    ','.join(roles) if roles else '<none>',
))
PY
}

# acquire_gateway_token_with_role - mint a per-gateway-client token and wait until
# the requested client role appears in it. The owner-binding -> reconciler ->
# AssignClientRole bridge is asynchronous, so a token minted immediately after
# gateway creation may not yet carry openshell-admin; poll until it does.
# Sets _OIDC_ACCESS_TOKEN on success.
#
# Grant-agnostic: it delegates to acquire_oidc_token, so E2E_OIDC_GRANT selects
# the flow (password grant on Kind/manual OpenShift; token-exchange of the
# hypershell-e2e service account onto the gateway audience on the GitHub-brokered
# PR admin path, or impersonation of a non-admin principal such as developer).
# Usage: acquire_gateway_token_with_role <user> <pass> <client_id> <role> [timeout]
acquire_gateway_token_with_role() {
  local username="${1:?username required}"
  local password="${2:?password required}"
  local client_id="${3:?client_id required}"
  local role="${4:?role required}"
  local timeout="${5:-300}"

  local deadline=$(($(date +%s) + timeout))
  while [[ $(date +%s) -lt $deadline ]]; do
    if acquire_oidc_token "$username" "$password" "$client_id"; then
      if _token_has_role "$_OIDC_ACCESS_TOKEN" "$role"; then
        return 0
      fi
      dim "    Token acquired but role '${role}' not yet present; waiting for role sync..."
    else
      dim "    Token acquisition failed against client '${client_id}'; retrying..."
    fi
    sleep 5
  done

  if [[ -n "${_OIDC_ACCESS_TOKEN}" ]]; then
    dim "    Last token claims: $(_token_debug_claims "${_OIDC_ACCESS_TOKEN}")"
  fi
  _OIDC_ACCESS_TOKEN=""
  return 1
}

# api_curl - curl wrapper that adds the OIDC bearer token acquired by
# acquire_oidc_token. Mirrors the flags used for unauthenticated calls
# (-s silent, -k insecure for the gateway's self-signed cert) and forwards any
# additional arguments to curl.
api_curl() {
  _driver_curl -H "Authorization: Bearer ${_OIDC_ACCESS_TOKEN}" "$@"
}

# get_cluster_domain - return the base domain for gateway DNS names.
get_cluster_domain() {
  echo "gw.localhost"
}

# get_cli_binary - return the Kubernetes CLI binary path.
get_cli_binary() {
  echo "kubectl"
}

# wait_for_gateway_route - block until the gateway is externally reachable.
wait_for_gateway_route() {
  local gw_name="${1:?gateway name required}"
  local gw_namespace="${2:?gateway namespace required}"

  local timeout="${E2E_PROVISION_TIMEOUT:-300}"
  local deadline=$(($(date +%s) + timeout))

  dim "  Waiting for Gateway route readiness (timeout: ${timeout}s)..."

  while [[ $(date +%s) -lt $deadline ]]; do
    local gw_ref_name gw_ref_ns programmed
    gw_ref_name=$(kubectl get grpcroute openshell-gateway -n "${gw_namespace}" \
      -o jsonpath='{.spec.parentRefs[0].name}' 2>/dev/null || true)
    gw_ref_ns=$(kubectl get grpcroute openshell-gateway -n "${gw_namespace}" \
      -o jsonpath='{.spec.parentRefs[0].namespace}' 2>/dev/null || true)

    programmed=0
    if [[ -n "$gw_ref_name" && -n "$gw_ref_ns" ]]; then
      programmed=$(kubectl get gateway "${gw_ref_name}" -n "${gw_ref_ns}" \
        -o jsonpath='{range .status.conditions[*]}{.type}={.status}{"\n"}{end}' 2>/dev/null \
        | grep -c 'Programmed=True' || true)
    fi

    local accepted
    accepted=$(kubectl get grpcroute -n "${gw_namespace}" -o jsonpath='{range .items[*]}{range .status.parents[*]}{range .conditions[*]}{.type}={.status}{"\n"}{end}{end}{end}' 2>/dev/null \
      | grep -c 'Accepted=True' || true)

    if [[ "${programmed:-0}" -ge 1 && "${accepted:-0}" -ge 1 ]]; then
      return 0
    fi

    dim "    Gateway Programmed=${programmed:-0}, GRPCRoute Accepted=${accepted:-0}"
    sleep 5
  done

  return 1
}
