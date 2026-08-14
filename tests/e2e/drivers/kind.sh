#!/usr/bin/env bash
# kind.sh - Kind infrastructure driver for e2e tests.
#
# Implements the driver interface contract using kubectl, Gateway API status,
# and Kind-specific conventions (HTTPRoute hostnames, GRPCRoute discovery).
#
# discover_api_host and discover_gateway_endpoint set global result variables
# (_DISCOVER_API_HOST, _DISCOVER_GW_ENDPOINT) instead of echoing, because
# these functions start background port-forward processes that must survive
# in the parent shell -- $() subshells kill orphaned background children.

E2E_KC_PF_PID="${E2E_KC_PF_PID:-}"

# discover_api_host - find the HyperShell API server base URL.
# Sets _DISCOVER_API_HOST to a full URL (scheme + host). Tries the HTTPRoute
# hostname with a reachability check first, then falls back to port-forward.
discover_api_host() {
  _DISCOVER_API_HOST=""
  local host
  host=$(kubectl get httproute -A -o jsonpath='{range .items[*]}{.spec.hostnames[0]}{"\n"}{end}' 2>/dev/null \
    | grep -m1 'api\.hypershell\.localhost' || true)
  if [[ -n "$host" ]]; then
    if curl -sk --connect-timeout 3 "https://${host}/api/hypershell/v1" &>/dev/null; then
      _DISCOVER_API_HOST="https://${host}"
      return
    fi
    dim "  HTTPRoute hostname ${host} not reachable, falling back to port-forward"
  fi

  local pf_port=8000
  if curl -s --connect-timeout 2 "http://localhost:${pf_port}/api/hypershell/v1" &>/dev/null; then
    dim "  Reusing existing port-forward on port ${pf_port}"
    _DISCOVER_API_HOST="http://localhost:${pf_port}"
    return
  fi
  kubectl port-forward svc/hypershell-api-server -n "${E2E_HS_NAMESPACE:-hypershell-system}" "${pf_port}:8000" >/dev/null 2>&1 &
  E2E_PF_PID=$!
  sleep 2
  if kill -0 "$E2E_PF_PID" 2>/dev/null; then
    _DISCOVER_API_HOST="http://localhost:${pf_port}"
  else
    E2E_PF_PID=""
    return 1
  fi
}

# discover_gateway_endpoint - find the gateway gRPC endpoint.
# Sets _DISCOVER_GW_ENDPOINT from the GRPCRoute hostname once the
# parent Gateway is Programmed.
discover_gateway_endpoint() {
  _DISCOVER_GW_ENDPOINT=""
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
        _DISCOVER_GW_ENDPOINT="https://${grpc_host}:443"
        return
      fi
    fi
  fi

  dim "  No programmed Gateway route found for ${gw_name}"
  return 1
}

# acquire_oidc_token - get an OIDC access token from Keycloak.
# Sets _OIDC_ACCESS_TOKEN via resource-owner password grant.
# Usage: acquire_oidc_token [username] [password]
#   Falls back to E2E_OIDC_USERNAME / E2E_OIDC_PASSWORD when not provided.
acquire_oidc_token() {
  _OIDC_ACCESS_TOKEN=""
  local username="${1:-${E2E_OIDC_USERNAME}}"
  local password="${2:-${E2E_OIDC_PASSWORD}}"
  local kc_pf_port=18080

  if curl -s --connect-timeout 2 "http://localhost:${kc_pf_port}/health/ready" &>/dev/null; then
    dim "  Reusing existing Keycloak port-forward on port ${kc_pf_port}"
  else
    kubectl port-forward svc/keycloak-service -n "${E2E_KEYCLOAK_NAMESPACE}" "${kc_pf_port}:8080" >/dev/null 2>&1 &
    E2E_KC_PF_PID=$!
    sleep 2
    if ! kill -0 "$E2E_KC_PF_PID" 2>/dev/null; then
      E2E_KC_PF_PID=""
      return 1
    fi
  fi

  local token_endpoint="http://localhost:${kc_pf_port}/realms/hypershell/protocol/openid-connect/token"
  local response
  response=$(curl -s -X POST "${token_endpoint}" \
    -d "grant_type=password" \
    -d "client_id=${E2E_OIDC_CLIENT_ID}" \
    -d "username=${username}" \
    -d "password=${password}" 2>/dev/null || true)

  _OIDC_ACCESS_TOKEN=$(echo "$response" | python3 -c "import json,sys; print(json.load(sys.stdin).get('access_token',''))" 2>/dev/null || true)

  if [[ -z "$_OIDC_ACCESS_TOKEN" || "$_OIDC_ACCESS_TOKEN" == "None" ]]; then
    _OIDC_ACCESS_TOKEN=""
    dim "  Token error: $(echo "$response" | python3 -c "import json,sys; print(json.load(sys.stdin).get('error_description','unknown'))" 2>/dev/null || echo 'no response')"
    return 1
  fi
}

# api_curl - curl wrapper that adds the OIDC bearer token acquired by
# acquire_oidc_token. Mirrors the flags used for unauthenticated calls
# (-s silent, -k insecure for the gateway's self-signed cert) and forwards any
# additional arguments to curl.
api_curl() {
  curl -sk -H "Authorization: Bearer ${_OIDC_ACCESS_TOKEN}" "$@"
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

  local timeout="${E2E_PROVISION_TIMEOUT:-180}"
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
