#!/usr/bin/env bash
# kind.sh - Kind infrastructure driver for e2e tests.
#
# Implements the driver interface contract using kubectl, Gateway API status,
# and Kind-specific conventions (HTTPRoute hostnames, GRPCRoute discovery).

# discover_api_host - find the HyperShell API server URL.
# Returns the HTTPRoute hostname or falls back to port-forward.
discover_api_host() {
  local host
  host=$(kubectl get httproute -A -o jsonpath='{range .items[*]}{.spec.hostnames[0]}{"\n"}{end}' 2>/dev/null \
    | grep -m1 'api\.hypershell\.localhost' || true)
  if [[ -n "$host" ]]; then
    echo "$host"
    return
  fi

  local pf_port=8000
  kubectl port-forward svc/hypershell-api-server -n "${E2E_HS_NAMESPACE:-hypershell-system}" "${pf_port}:8000" >/dev/null 2>&1 &
  E2E_PF_PID=$!
  sleep 2
  if kill -0 "$E2E_PF_PID" 2>/dev/null; then
    echo "localhost:${pf_port}"
  else
    E2E_PF_PID=""
    return 1
  fi
}

# discover_gateway_endpoint - find the gateway gRPC endpoint.
# Derives the endpoint from the GRPCRoute hostname and the networking
# Gateway's status address.
discover_gateway_endpoint() {
  local gw_name="${1:?gateway name required}"
  local gw_namespace="${2:?gateway namespace required}"
  local domain
  domain=$(get_cluster_domain)

  echo "https://${gw_name}.${domain}:443"
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
# Polls the Gateway API Gateway status conditions for Programmed=True and
# verifies the GRPCRoute parent status reports Accepted=True.
wait_for_gateway_route() {
  local gw_name="${1:?gateway name required}"
  local gw_namespace="${2:?gateway namespace required}"
  local timeout="${E2E_PROVISION_TIMEOUT:-180}"
  local deadline=$(($(date +%s) + timeout))

  dim "  Waiting for Gateway route readiness (timeout: ${timeout}s)..."

  while [[ $(date +%s) -lt $deadline ]]; do
    local programmed
    programmed=$(kubectl get gateway -n "${gw_namespace}" -o jsonpath='{range .items[*]}{range .status.conditions[*]}{.type}={.status}{"\n"}{end}{end}' 2>/dev/null \
      | grep -c 'Programmed=True' || true)

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
