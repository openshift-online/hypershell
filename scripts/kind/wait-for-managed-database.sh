#!/usr/bin/env bash
# Wait for the openshell-db ManagedDatabase CNPG cluster that kind-up seeds.
#
# After component image swaps the api-server may restart while the controller's
# ManagedDatabase watch stream is disconnected. That stream does not replay
# existing resources on reconnect, so the create event can be missed and the
# openshell-db-<hex> namespace never appears. Poll for the CNPG cluster and,
# while it is absent, nudge reconciliation with a no-op PATCH so the controller
# receives an Updated event.
#
# Environment (all optional):
#   MANAGED_DB_TIMEOUT   overall wait ceiling as a Go duration (default 300s)
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "${SCRIPT_DIR}/lib.sh"

require_cluster

: "${MANAGED_DB_TIMEOUT:=300s}"

timeout_seconds() {
  local raw="$1"
  if [[ "${raw}" =~ ^([0-9]+)s$ ]]; then
    echo "${BASH_REMATCH[1]}"
  elif [[ "${raw}" =~ ^[0-9]+$ ]]; then
    echo "${raw}"
  else
    error "Unsupported duration format: ${raw} (expected e.g. 300s)"
    exit 1
  fi
}

managed_db_namespace() {
  kube get ns -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}' 2>/dev/null \
    | grep '^openshell-db-' | head -1 || true
}

cnpg_cluster_ready() {
  local namespace="$1"
  [[ -n "${namespace}" ]] || return 1
  kube wait --for=condition=Ready "cluster/openshell-db" -n "${namespace}" --timeout=10s >/dev/null 2>&1
}

obtain_api_token() {
  local token_resp token
  KC_TOKEN_URL="https://${KEYCLOAK_HOSTNAME}/realms/hypershell/protocol/openid-connect/token"
  token_resp="$(curl -sSk -m 5 -X POST "${KC_TOKEN_URL}" \
    -d "grant_type=password" \
    -d "client_id=hypershell-frontend" \
    -d "username=admin" \
    -d "password=admin" 2>&1 || true)"
  token="$(echo "${token_resp}" | grep -o '"access_token":"[^"]*"' | cut -d'"' -f4 || true)"
  [[ -n "${token}" ]] || return 1
  printf '%s' "${token}"
}

nudge_managed_database_reconcile() {
  local api_url="http://localhost:8000"
  local pf_pid="" token="" md_id="" raw http body auth_header

  info "Nudging ManagedDatabase reconcile via API..."
  kube port-forward svc/hypershell-api-server -n "${KIND_NAMESPACE}" 8000:8000 >/dev/null 2>&1 &
  pf_pid=$!
  cleanup_pf() {
    kill "${pf_pid}" 2>/dev/null || true
    wait "${pf_pid}" 2>/dev/null || true
  }
  trap cleanup_pf RETURN

  for _ in $(seq 1 15); do
    if curl -s -o /dev/null -m 3 "${api_url}/api/hypershell/v1/fleets" 2>/dev/null; then
      break
    fi
    sleep 2
  done

  token="$(obtain_api_token || true)"
  if [[ -z "${token}" ]]; then
    warn "Could not obtain API token to nudge ManagedDatabase reconcile"
    return 1
  fi
  auth_header="Authorization: Bearer ${token}"

  raw="$(curl -sS -w "\n%{http_code}" -H "${auth_header}" \
    "${api_url}/api/hypershell/v1/managed_databases" 2>&1 || true)"
  http="$(echo "${raw}" | tail -1)"
  body="$(echo "${raw}" | sed '$d')"
  if [[ "${http}" != "200" ]]; then
    warn "ManagedDatabase list failed (HTTP ${http}): ${body:0:200}"
    return 1
  fi

  md_id="$(echo "${body}" | grep -o '"name":"openshell-db"[^}]*"id":"[^"]*"' \
    | grep -o '"id":"[^"]*"' | cut -d'"' -f4 | head -1 || true)"
  if [[ -z "${md_id}" ]]; then
    warn "openshell-db ManagedDatabase not found in API"
    return 1
  fi

  raw="$(curl -sS -w "\n%{http_code}" -X PATCH \
    -H "Content-Type: application/json" \
    -H "${auth_header}" \
    -d '{}' \
    "${api_url}/api/hypershell/v1/managed_databases/${md_id}" 2>&1 || true)"
  http="$(echo "${raw}" | tail -1)"
  body="$(echo "${raw}" | sed '$d')"
  if [[ "${http}" != "200" ]]; then
    warn "ManagedDatabase PATCH failed (HTTP ${http}): ${body:0:200}"
    return 1
  fi

  info "  PATCH sent for ManagedDatabase ${md_id}"
}

header "ManagedDatabase CNPG Readiness"

deadline=$((SECONDS + $(timeout_seconds "${MANAGED_DB_TIMEOUT}")))
last_nudge=0

info "Waiting for API server rollout..."
kube rollout status deployment/hypershell-api-server -n "${KIND_NAMESPACE}" --timeout=120s
info "Waiting for control plane rollout..."
kube rollout status deployment/hypershell-controller -n "${KIND_NAMESPACE}" --timeout=120s

while (( SECONDS < deadline )); do
  ns="$(managed_db_namespace)"
  if [[ -n "${ns}" ]] && cnpg_cluster_ready "${ns}"; then
    success "ManagedDatabase CNPG cluster ready in ${ns}"
    exit 0
  fi

  if (( SECONDS - last_nudge >= 30 )); then
    nudge_managed_database_reconcile || true
    last_nudge=$SECONDS
  fi

  if [[ -n "${ns}" ]]; then
    info "  Waiting for CNPG cluster/openshell-db in ${ns}..."
  else
    info "  Waiting for openshell-db-* namespace..."
  fi
  sleep 5
done

error "Timed out after ${MANAGED_DB_TIMEOUT} waiting for ManagedDatabase CNPG cluster"
exit 1
