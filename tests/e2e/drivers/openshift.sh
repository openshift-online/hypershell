#!/usr/bin/env bash
# OpenShift infrastructure driver for the shared e2e and performance suites.
#
# The environment must already exist. OPENSHIFT_NAMESPACE selects its platform
# namespace; when unset, use the current oc project just like openshift-up.
# Other runtime settings are discovered from that deployment.

# Reuse the infrastructure-neutral OIDC, Keycloak role, and JWT helpers. Every
# infrastructure operation and the TLS policy are overridden below.
# shellcheck source=kind.sh
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/kind.sh"
# The shared Kind helpers enable insecure CLI TLS for their local CA. OpenShift
# uses trusted routes (or E2E_OPENSHIFT_CA_SECRET), so never inherit that bypass.
unset OPENSHELL_GATEWAY_INSECURE

: "${E2E_OPENSHIFT_KEYCLOAK_ROUTE:=keycloak}"
# Namespace holding the Keycloak Route. Defaults to the "<platform>-keycloak"
# convention the PR-environment deploys use; set it when the deployment names
# the Keycloak namespace differently (e.g. a GitOps environment where it is
# "keycloak-<platform>").
: "${E2E_OPENSHIFT_KEYCLOAK_NAMESPACE:=}"
: "${E2E_OPENSHIFT_CA_SECRET:=}"
: "${E2E_OPENSHIFT_CA_NAMESPACE:=}"

_openshift_require_config() {
  if [[ -z "${OPENSHIFT_NAMESPACE:-}" ]]; then
    local project
    project="$(oc project -q 2>/dev/null || true)"
    if [[ -z "${project}" ]]; then
      red "  OPENSHIFT_NAMESPACE is unset and no oc project is selected"
      red "  Run 'oc project <name>' or set OPENSHIFT_NAMESPACE before running make e2e"
      return 1
    fi
    OPENSHIFT_NAMESPACE="${project}"
    dim "  OPENSHIFT_NAMESPACE unset; using oc project '${OPENSHIFT_NAMESPACE}'"
  fi
  E2E_HS_NAMESPACE="${OPENSHIFT_NAMESPACE}"
  E2E_KEYCLOAK_NAMESPACE="${E2E_OPENSHIFT_KEYCLOAK_NAMESPACE:-${OPENSHIFT_NAMESPACE}-keycloak}"
}

_openshift_configure_oidc() {
  _openshift_require_config || return 1

  local host
  host=$(oc get route "${E2E_OPENSHIFT_KEYCLOAK_ROUTE}" \
    -n "${E2E_KEYCLOAK_NAMESPACE}" -o jsonpath='{.spec.host}' 2>/dev/null || true)
  if [[ -z "$host" ]]; then
    red "  Keycloak Route '${E2E_OPENSHIFT_KEYCLOAK_ROUTE}' not found in ${E2E_KEYCLOAK_NAMESPACE}"
    return 1
  fi
  E2E_OIDC_ISSUER="https://${host}/realms/hypershell"
}

_openshift_configure_tls() {
  [[ -z "${E2E_OPENSHIFT_CA_SECRET}" ]] && return 0

  local namespace encoded ca_file
  namespace="${E2E_OPENSHIFT_CA_NAMESPACE:-${OPENSHIFT_NAMESPACE}}"
  encoded=$(oc get secret "${E2E_OPENSHIFT_CA_SECRET}" -n "$namespace" \
    -o jsonpath='{.data.ca\.crt}' 2>/dev/null || true)
  if [[ -z "$encoded" ]]; then
    red "  CA key ca.crt not found in Secret ${namespace}/${E2E_OPENSHIFT_CA_SECRET}"
    return 1
  fi

  ca_file="${TMPDIR:-/tmp}/hypershell-e2e-${OPENSHIFT_NAMESPACE}-ca.crt"
  umask 077
  if ! printf '%s' "$encoded" | openssl base64 -d -A >"$ca_file"; then
    red "  Could not decode CA from Secret ${namespace}/${E2E_OPENSHIFT_CA_SECRET}"
    return 1
  fi
  export SSL_CERT_FILE="$ca_file"
}

# OpenShift must verify Route and Gateway certificates. A private CA can be
# supplied with E2E_OPENSHIFT_CA_SECRET; otherwise curl uses the system store.
_driver_curl() {
  curl -sS "$@"
}

discover_api_host() {
  _DISCOVER_API_HOST=""
  _openshift_require_config || return 1
  _openshift_configure_tls || return 1
  _openshift_configure_oidc || return 1

  local host code
  host=$(oc get route hypershell-api -n "${OPENSHIFT_NAMESPACE}" \
    -o jsonpath='{.spec.host}' 2>/dev/null || true)
  if [[ -z "$host" ]]; then
    red "  HyperShell API Route 'hypershell-api' not found in ${OPENSHIFT_NAMESPACE}"
    return 1
  fi

  _DISCOVER_API_HOST="https://${host}"
  code=$(_driver_curl --connect-timeout 5 -o /dev/null -w '%{http_code}' \
    "${_DISCOVER_API_HOST}/api/hypershell/v1/gateways" 2>/dev/null || true)
  if [[ -z "$code" || "$code" == "000" ]]; then
    red "  HyperShell API Route ${_DISCOVER_API_HOST} returned no HTTP response"
    _DISCOVER_API_HOST=""
    return 1
  fi
}

# discover_console_host - find the HyperShell web console (BFF) base URL.
# The console Route hostname is cluster-generated and unrelated to the API
# Route hostname, so it must be discovered independently rather than derived
# by string substitution on discover_api_host's result.
discover_console_host() {
  _DISCOVER_CONSOLE_HOST=""
  _openshift_require_config || return 1

  local host code
  host=$(oc get route hypershell-web-console -n "${OPENSHIFT_NAMESPACE}" \
    -o jsonpath='{.spec.host}' 2>/dev/null || true)
  if [[ -z "$host" ]]; then
    red "  HyperShell web console Route 'hypershell-web-console' not found in ${OPENSHIFT_NAMESPACE}"
    return 1
  fi

  _DISCOVER_CONSOLE_HOST="https://${host}"
  code=$(_driver_curl --connect-timeout 5 -o /dev/null -w '%{http_code}' \
    "${_DISCOVER_CONSOLE_HOST}/auth/session" 2>/dev/null || true)
  if [[ -z "$code" || "$code" == "000" ]]; then
    red "  HyperShell web console Route ${_DISCOVER_CONSOLE_HOST} returned no HTTP response"
    _DISCOVER_CONSOLE_HOST=""
    return 1
  fi
}

discover_gateway_endpoint() {
  _DISCOVER_GW_ENDPOINT=""
  local gw_name="${1:?gateway name required}"
  local gw_namespace="${2:?gateway namespace required}"
  local grpc_host gw_ref_name gw_ref_ns programmed

  grpc_host=$(oc get grpcroute openshell-gateway -n "$gw_namespace" \
    -o jsonpath='{.spec.hostnames[0]}' 2>/dev/null || true)
  gw_ref_name=$(oc get grpcroute openshell-gateway -n "$gw_namespace" \
    -o jsonpath='{.spec.parentRefs[0].name}' 2>/dev/null || true)
  gw_ref_ns=$(oc get grpcroute openshell-gateway -n "$gw_namespace" \
    -o jsonpath='{.spec.parentRefs[0].namespace}' 2>/dev/null || true)
  programmed=$(oc get gateway "$gw_ref_name" -n "$gw_ref_ns" \
    -o jsonpath='{range .status.conditions[*]}{.type}={.status}{"\n"}{end}' 2>/dev/null \
    | grep -c 'Programmed=True' || true)

  if [[ -n "$grpc_host" && "${programmed:-0}" -ge 1 ]]; then
    _DISCOVER_GW_ENDPOINT="https://${grpc_host}:443"
    return 0
  fi
  dim "  No programmed Gateway route found for ${gw_name}"
  return 1
}

get_cluster_domain() {
  _openshift_require_config >/dev/null || return 1
  local domain
  domain=$(oc get deployment hypershell-controller -n "${OPENSHIFT_NAMESPACE}" \
    -o jsonpath='{.spec.template.spec.containers[?(@.name=="controller")].env[?(@.name=="GATEWAY_API_BASE_DOMAIN")].value}' \
    2>/dev/null || true)
  if [[ -z "$domain" ]]; then
    red "  GATEWAY_API_BASE_DOMAIN is not configured on Deployment ${OPENSHIFT_NAMESPACE}/hypershell-controller"
    return 1
  fi
  printf '%s\n' "$domain"
}

get_cli_binary() {
  echo "oc"
}

wait_for_gateway_route() {
  local gw_name="${1:?gateway name required}"
  local gw_namespace="${2:?gateway namespace required}"
  local timeout="${E2E_PROVISION_TIMEOUT:-300}"
  local deadline=$(($(date +%s) + timeout))

  dim "  Waiting for Gateway route readiness (timeout: ${timeout}s)..."
  while [[ $(date +%s) -lt $deadline ]]; do
    local gw_ref_name gw_ref_ns programmed accepted
    gw_ref_name=$(oc get grpcroute openshell-gateway -n "$gw_namespace" \
      -o jsonpath='{.spec.parentRefs[0].name}' 2>/dev/null || true)
    gw_ref_ns=$(oc get grpcroute openshell-gateway -n "$gw_namespace" \
      -o jsonpath='{.spec.parentRefs[0].namespace}' 2>/dev/null || true)
    programmed=$(oc get gateway "$gw_ref_name" -n "$gw_ref_ns" \
      -o jsonpath='{range .status.conditions[*]}{.type}={.status}{"\n"}{end}' 2>/dev/null \
      | grep -c 'Programmed=True' || true)
    accepted=$(oc get grpcroute openshell-gateway -n "$gw_namespace" \
      -o jsonpath='{range .status.parents[*].conditions[*]}{.type}={.status}{"\n"}{end}' 2>/dev/null \
      | grep -c 'Accepted=True' || true)
    if [[ "${programmed:-0}" -ge 1 && "${accepted:-0}" -ge 1 ]]; then
      return 0
    fi
    dim "    ${gw_name}: Gateway Programmed=${programmed:-0}, GRPCRoute Accepted=${accepted:-0}"
    sleep 5
  done
  return 1
}

_openshift_is_ci_owned() {
  [[ "${OPENSHIFT_NAMESPACE:-}" == hypershell-ci-pr-* ]]
}

_openshift_load_ci_passwords() {
  _openshift_is_ci_owned || return 0
  [[ "${E2E_OIDC_GRANT:-password}" == "password" ]] || return 0
  [[ -z "${_OPENSHIFT_CI_PASSWORDS_LOADED:-}" ]] || return 0
  local ns="${E2E_KEYCLOAK_NAMESPACE}"
  local admin_pw dev_pw pa_pw
  admin_pw="$(oc get secret hypershell-e2e-test-users -n "${ns}" \
    -o jsonpath='{.data.admin}' 2>/dev/null | base64 -d 2>/dev/null || true)"
  dev_pw="$(oc get secret hypershell-e2e-test-users -n "${ns}" \
    -o jsonpath='{.data.developer}' 2>/dev/null | base64 -d 2>/dev/null || true)"
  pa_pw="$(oc get secret hypershell-e2e-test-users -n "${ns}" \
    -o jsonpath='{.data.platform-admin}' 2>/dev/null | base64 -d 2>/dev/null || true)"
  if [[ -z "${admin_pw}" || -z "${dev_pw}" || -z "${pa_pw}" ]]; then
    red "  Secret hypershell-e2e-test-users is missing in ${ns}"
    red "  Password-grant e2e against a CI-owned PR environment cannot fall back to username-equals-password"
    return 1
  fi
  E2E_OIDC_PASSWORD="${admin_pw}"
  E2E_DEV_PASSWORD="${dev_pw}"
  E2E_PLATFORM_ADMIN_PASSWORD="${pa_pw}"
  if [[ -n "${GITHUB_ACTIONS:-}" ]]; then
    echo "::add-mask::${admin_pw}"
    echo "::add-mask::${dev_pw}"
    echo "::add-mask::${pa_pw}"
  fi
  _OPENSHIFT_CI_PASSWORDS_LOADED=1
}

_openshift_password_for() {
  local username="$1"
  local fallback="$2"
  case "${username}" in
    admin) printf '%s' "${E2E_OIDC_PASSWORD}" ;;
    developer) printf '%s' "${E2E_DEV_PASSWORD}" ;;
    platform-admin) printf '%s' "${E2E_PLATFORM_ADMIN_PASSWORD}" ;;
    *) printf '%s' "${fallback}" ;;
  esac
}

# Ensure direct callers (including unit tests) get the cluster-derived issuer.
acquire_oidc_token() {
  local username="${1:-${E2E_OIDC_USERNAME}}"
  local password="${2:-${E2E_OIDC_PASSWORD}}"
  local client_id="${3:-${E2E_OIDC_CLIENT_ID}}"
  _openshift_configure_oidc || return 1
  _openshift_configure_tls || return 1
  _openshift_load_ci_passwords || return 1
  if _openshift_is_ci_owned && [[ "${E2E_OIDC_GRANT:-password}" == "password" ]]; then
    password="$(_openshift_password_for "${username}" "${password}")"
  fi
  _driver_acquire_oidc_token "${username}" "${password}" "${client_id}"
}

de_seed_test_users() {
  # Test-tier principals stay for the whole environment lifetime
  # (ephemeral-test-credentials.spec.md). Namespace destroy removes them.
  return 0
}

# configure_namespace_gc_timing / restore_namespace_gc_timing - resolve the
# OpenShift namespace, then delegate to the shared implementation in kind.sh.
configure_namespace_gc_timing() {
  _openshift_require_config || return 1
  _patch_namespace_gc_timing oc "${OPENSHIFT_NAMESPACE}"
}

restore_namespace_gc_timing() {
  _openshift_require_config || return 1
  _restore_namespace_gc_timing oc "${OPENSHIFT_NAMESPACE}"
}
