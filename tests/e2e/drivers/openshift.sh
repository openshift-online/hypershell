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

: "${E2E_OPENSHIFT_KEYCLOAK_ROUTE:=keycloak}"
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
  E2E_KEYCLOAK_NAMESPACE="${OPENSHIFT_NAMESPACE}-keycloak"
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
  local timeout="${E2E_PROVISION_TIMEOUT:-180}"
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

# Ensure direct callers (including unit tests) get the cluster-derived issuer.
acquire_oidc_token() {
  _openshift_configure_oidc || return 1
  _openshift_configure_tls || return 1
  _driver_acquire_oidc_token "$@"
}
