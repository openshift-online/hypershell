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
  code=$(curl -sk --connect-timeout 5 -o /dev/null -w '%{http_code}' \
    "${url}/api/hypershell/v1/gateways" 2>/dev/null || true)
  # Any HTTP response (401 unauthenticated, 200, 404, ...) proves the route
  # reaches the API server. "000" means the connection never completed -- route
  # not programmed, 443->LB mapping down, or the api-server pod not serving.
  if [[ -z "$code" || "$code" == "000" ]]; then
    red "  API route ${url} is not reachable (no HTTP response)"
    red "  Verify: Gateway Programmed, api-server pod Ready, and 443->LB mapping active"
    return 1
  fi

  _DISCOVER_API_HOST="${url}"
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
acquire_oidc_token() {
  _OIDC_ACCESS_TOKEN=""
  local username="${1:-${E2E_OIDC_USERNAME}}"
  local password="${2:-${E2E_OIDC_PASSWORD}}"
  local client_id="${3:-${E2E_OIDC_CLIENT_ID}}"

  local token_endpoint="${E2E_OIDC_ISSUER}/protocol/openid-connect/token"
  local response
  response=$(curl -sk -X POST "${token_endpoint}" \
    -d "grant_type=password" \
    -d "client_id=${client_id}" \
    -d "username=${username}" \
    -d "password=${password}" 2>/dev/null || true)

  _OIDC_ACCESS_TOKEN=$(echo "$response" | python3 -c "import json,sys; print(json.load(sys.stdin).get('access_token',''))" 2>/dev/null || true)

  if [[ -z "$_OIDC_ACCESS_TOKEN" || "$_OIDC_ACCESS_TOKEN" == "None" ]]; then
    _OIDC_ACCESS_TOKEN=""
    dim "  Token error: $(echo "$response" | python3 -c "import json,sys; print(json.load(sys.stdin).get('error_description','unknown'))" 2>/dev/null || echo 'no response')"
    return 1
  fi
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
  response=$(curl -sk -X POST "${base}/realms/master/protocol/openid-connect/token" \
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
  client_uuid=$(curl -sk -H "Authorization: Bearer ${_KC_ADMIN_TOKEN}" \
    "${base}/admin/realms/${realm}/clients?clientId=${client_id}" 2>/dev/null \
    | python3 -c "import json,sys; a=json.load(sys.stdin); print(a[0]['id'] if a else '')" 2>/dev/null || true)
  if [[ -z "$client_uuid" ]]; then
    red "  Keycloak client not found: ${client_id}"
    return 1
  fi

  user_uuid=$(curl -sk -H "Authorization: Bearer ${_KC_ADMIN_TOKEN}" \
    "${base}/admin/realms/${realm}/users?username=${username}&exact=true" 2>/dev/null \
    | python3 -c "import json,sys; a=json.load(sys.stdin); print(a[0]['id'] if a else '')" 2>/dev/null || true)
  if [[ -z "$user_uuid" ]]; then
    red "  Keycloak user not found: ${username}"
    return 1
  fi

  role_json=$(curl -sk -H "Authorization: Bearer ${_KC_ADMIN_TOKEN}" \
    "${base}/admin/realms/${realm}/clients/${client_uuid}/roles/${role}" 2>/dev/null || true)
  local role_id role_name
  role_id=$(echo "$role_json" | python3 -c "import json,sys; print(json.load(sys.stdin).get('id',''))" 2>/dev/null || true)
  role_name=$(echo "$role_json" | python3 -c "import json,sys; print(json.load(sys.stdin).get('name',''))" 2>/dev/null || true)
  if [[ -z "$role_id" || -z "$role_name" ]]; then
    red "  Keycloak client role not found: ${role} on ${client_id}"
    return 1
  fi

  local code
  code=$(curl -sk -o /dev/null -w '%{http_code}' -X POST \
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
  user_uuid=$(curl -sk -H "Authorization: Bearer ${_KC_ADMIN_TOKEN}" \
    "${base}/admin/realms/${realm}/users?username=${username}&exact=true" 2>/dev/null \
    | python3 -c "import json,sys; a=json.load(sys.stdin); print(a[0]['id'] if a else '')" 2>/dev/null || true)
  if [[ -z "$user_uuid" ]]; then
    red "  Keycloak user not found: ${username}"
    return 1
  fi

  role_json=$(curl -sk -H "Authorization: Bearer ${_KC_ADMIN_TOKEN}" \
    "${base}/admin/realms/${realm}/roles/${role}" 2>/dev/null || true)
  local role_id role_name
  role_id=$(echo "$role_json" | python3 -c "import json,sys; print(json.load(sys.stdin).get('id',''))" 2>/dev/null || true)
  role_name=$(echo "$role_json" | python3 -c "import json,sys; print(json.load(sys.stdin).get('name',''))" 2>/dev/null || true)
  if [[ -z "$role_id" || -z "$role_name" ]]; then
    red "  Keycloak realm role not found: ${role}"
    return 1
  fi

  local code
  code=$(curl -sk -o /dev/null -w '%{http_code}' -X POST \
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

# acquire_gateway_token_with_role - mint a per-gateway-client token and wait until
# the requested client role appears in it. The owner-binding -> reconciler ->
# AssignClientRole bridge is asynchronous, so a token minted immediately after
# gateway creation may not yet carry openshell-admin; poll until it does.
# Sets _OIDC_ACCESS_TOKEN on success.
# Usage: acquire_gateway_token_with_role <user> <pass> <client_id> <role> [timeout]
acquire_gateway_token_with_role() {
  local username="${1:?username required}"
  local password="${2:?password required}"
  local client_id="${3:?client_id required}"
  local role="${4:?role required}"
  local timeout="${5:-120}"

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

  _OIDC_ACCESS_TOKEN=""
  return 1
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
