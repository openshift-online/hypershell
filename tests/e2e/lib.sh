#!/usr/bin/env bash
# lib.sh - shared e2e test utilities.
#
# Provides pass/fail tracking, colored output, retry helpers, and common
# environment defaults. Sourced by e2e-openshell.sh.

set -euo pipefail

# --- Color output (respects NO_COLOR and non-TTY) ---

if [[ -z "${NO_COLOR:-}" ]] && [[ -t 1 ]]; then
  _BOLD='\033[1m'
  _GREEN='\033[32m'
  _RED='\033[31m'
  _DIM='\033[2m'
  _CYAN='\033[36m'
  _ORANGE='\033[38;5;214m'
  _NC='\033[0m'
else
  _BOLD='' _GREEN='' _RED='' _DIM='' _CYAN='' _ORANGE='' _NC=''
  # Non-interactive (CI, piped, or NO_COLOR set): force child CLIs into plain
  # mode too. In particular the openshell CLI renders diagnostics with a
  # miette-style graphical handler that, off a TTY, assumes an 80-column width
  # and wraps long errors with a box-drawing gutter (e.g. "... the specified │
  # operation"). Exporting NO_COLOR and TERM=dumb makes those renderers emit
  # flat, single-stream text so captured logs stay clean and greppable.
  export NO_COLOR=1
  export TERM=dumb
fi

bold()   { printf "${_BOLD}%s${_NC}\n" "$*"; }
green()  { printf "${_GREEN}%s${_NC}\n" "$*"; }
red()    { printf "${_RED}%s${_NC}\n" "$*"; }
dim()    { printf "${_DIM}%s${_NC}\n" "$*"; }
cyan()   { printf "${_CYAN}%s${_NC}\n" "$*"; }
orange() { printf "${_ORANGE}%s${_NC}\n" "$*"; }
sep()    { printf "${_DIM}────────────────────────────────────────────────${_NC}\n"; }

# --- Test tracking ---

E2E_PASS=0
E2E_FAIL=0
E2E_TESTS=()

pass() {
  E2E_PASS=$((E2E_PASS + 1))
  E2E_TESTS+=("PASS: $1")
  green "  ✓ $1"
}

fail_test() {
  E2E_FAIL=$((E2E_FAIL + 1))
  E2E_TESTS+=("FAIL: $1")
  red "  ✗ $1"
}

show_cmd() {
  orange "   \$ $*"
  sleep "${E2E_PAUSE:-1}"
}

print_results() {
  echo ""
  bold "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  bold "Results: $E2E_PASS passed, $E2E_FAIL failed"
  echo ""
  for t in "${E2E_TESTS[@]}"; do
    if [[ "$t" == PASS:* ]]; then
      green "  ✓ ${t#PASS: }"
    else
      red "  ✗ ${t#FAIL: }"
    fi
  done
  bold "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  echo ""
}

# --- Retry helper ---

retry_until() {
  local timeout="$1" interval="${2:-5}" cmd="${*:3}"
  local deadline=$(($(date +%s) + timeout))
  while [[ $(date +%s) -lt $deadline ]]; do
    if eval "$cmd"; then
      return 0
    fi
    sleep "$interval"
  done
  return 1
}

# --- Environment defaults ---

: "${E2E_NAMESPACE:=openshell-e2e}"
: "${E2E_GATEWAY_NAME:=e2e-gw}"
: "${E2E_MODE:=long}"
: "${E2E_SANDBOX_TIMEOUT:=120}"
: "${E2E_PROVISION_TIMEOUT:=180}"
: "${E2E_GC_TIMEOUT:=180}"
: "${E2E_ORPHAN_GC_TIMEOUT:=90}"
: "${E2E_SKIP_CLEANUP:=0}"
: "${E2E_PAUSE:=1}"
_E2E_LIB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
: "${OPENSHELL_BIN:=openshell}"
: "${E2E_KEYCLOAK_NAMESPACE:=keycloak}"
: "${E2E_OIDC_ISSUER:=https://keycloak.hypershell.localhost/realms/hypershell}"
: "${E2E_OIDC_CLIENT_ID:=hypershell-frontend}"
: "${E2E_OIDC_USERNAME:=admin}"
: "${E2E_OIDC_PASSWORD:=admin}"
: "${E2E_DEV_USERNAME:=developer}"
: "${E2E_DEV_PASSWORD:=developer}"
: "${E2E_PLATFORM_ADMIN_USERNAME:=platform-admin}"
: "${E2E_PLATFORM_ADMIN_PASSWORD:=platform-admin}"
# Keycloak admin credentials for test-setup helpers that provision per-gateway
# client role grants (e.g. granting the developer openshell-user on the gateway's
# own Keycloak client, mirroring what the RoleBinding reconciler does in prod).
: "${E2E_KC_ADMIN_USER:=admin}"
: "${E2E_KC_ADMIN_PASSWORD:=admin}"

# RFC3339 timestamp N minutes in the past (macOS BSD date and GNU date).
e2e_gc_eligible_since_backdate() {
  local minutes="${1:-3}"
  if date -u -v-"${minutes}"M +%Y-%m-%dT%H:%M:%SZ >/dev/null 2>&1; then
    date -u -v-"${minutes}"M +%Y-%m-%dT%H:%M:%SZ
  else
    date -u -d "${minutes} minutes ago" +%Y-%m-%dT%H:%M:%SZ
  fi
}

# Print allowlisted namespace-GC lines from controller logs. Avoids dumping raw
# controller output that may contain OIDC endpoints or other sensitive config.
e2e_dump_namespace_gc_logs() {
  local hs_namespace="${1:-hypershell-system}"
  local cli="${2:-kubectl}"
  "$cli" logs -l app=hypershell-controller -n "$hs_namespace" --tail=200 2>/dev/null \
    | grep -E 'namespace gc:|GarbageCollected|recordGCEvent|deleted namespace' \
    | tail -20 \
    | while IFS= read -r line; do dim "    $line"; done || true
}

# --- E2E_MODE (short | long) ---
#
# Each suite step declares a minimum mode. short-tagged steps run in both
# modes; long-tagged steps run only in long mode. Default is long so existing
# CI invocations are unchanged. See e2e-testing.spec.md "E2E Short and Long Modes".

e2e_validate_mode() {
  case "${E2E_MODE}" in
    short|long) ;;
    *)
      red "ERROR: E2E_MODE must be 'short' or 'long' (got '${E2E_MODE}')"
      exit 1
      ;;
  esac
}

# e2e_step <short|long> - return 0 if the current mode should run this step.
e2e_step() {
  local min_mode="${1:?mode tag required}"
  case "${E2E_MODE}" in
    long) return 0 ;;
    short)
      [[ "${min_mode}" == "short" ]] && return 0
      return 1
      ;;
  esac
}

e2e_utc_now() {
  date -u +%Y-%m-%dT%H:%M:%SZ
}

e2e_utc_stamp() {
  date -u +%Y%m%dT%H%M%SZ
}

# List driver names from tests/e2e/drivers/*.sh (basename without .sh).
e2e_list_available_drivers() {
  local drivers_dir="${1:-${_E2E_LIB_DIR}/drivers}"
  if [[ -d "$drivers_dir" ]]; then
    local f drivers=()
    for f in "${drivers_dir}"/*.sh; do
      [[ -f "$f" ]] && drivers+=("$(basename "$f" .sh)")
    done
    ((${#drivers[@]} > 0)) && printf '%s\n' "${drivers[@]}"
  fi
}

# Print available drivers and exit 1. Used when E2E_INFRA_DRIVER is unset or unknown.
e2e_die_unknown_driver() {
  local reason="$1"
  red "ERROR: ${reason}"
  echo ""
  echo "Available drivers:"
  e2e_list_available_drivers | while read -r d; do echo "  - $d"; done
  exit 1
}

# First item id from a HyperShell list JSON on stdin. Optional name match.
# Usage: echo "$json" | e2e_json_first_id [name]
e2e_json_first_id() {
  WANT_NAME="${1:-}" python3 -c "
import json, sys, os
name = os.environ.get('WANT_NAME', '')
try:
    data = json.load(sys.stdin)
except Exception:
    sys.exit(0)
items = data.get('items', []) if isinstance(data, dict) else []
for it in items:
    if not name or it.get('name', '') == name:
        print(it.get('id', '') or '')
        break
"
}

# Look up a gateway by exact name. Sets _GW_ID, _GW_NAMESPACE, _GW_PHASE (empty if missing).
# Requires API_HOST and api_curl.
e2e_lookup_gateway_by_name() {
  local name="${1:?gateway name required}"
  _GW_ID=""
  _GW_NAMESPACE=""
  _GW_PHASE=""
  local resp
  resp=$(api_curl "${API_HOST}/api/hypershell/v1/gateways?search=name%3D${name}" 2>/dev/null || true)
  IFS=$'\t' read -r _GW_ID _GW_NAMESPACE _GW_PHASE <<< "$(echo "$resp" | WANT_NAME="$name" python3 -c "
import json, sys, os
name = os.environ['WANT_NAME']
try:
    data = json.load(sys.stdin)
except Exception:
    print('\t\t'); sys.exit(0)
for gw in data.get('items', []) or []:
    if gw.get('name', '') == name:
        print('%s\t%s\t%s' % (gw.get('id',''), gw.get('namespace',''), gw.get('phase','')))
        break
else:
    print('\t\t')
" 2>/dev/null)" || true
}

# Discover the seeded fleet, cluster, release, and managed-database ids via the API.
# Sets E2E_FLEET_ID, E2E_CLUSTER_ID, E2E_RELEASE_ID, E2E_DATABASE_ID.
# Requires API_HOST and api_curl. Never hardcodes ids.
#
# Name pins (optional): E2E_SEED_FLEET_NAME, E2E_SEED_CLUSTER_NAME,
# E2E_SEED_RELEASE_NAME. On kind these default to the make kind-up seeds
# (default, local-kind, dev-release). When a name is unset, the first list
# item is used — that matches single-seed CI/dev; multi-seed clusters should
# set the name pins instead of relying on API order.
e2e_discover_seed_ids() {
  local fleets clusters releases databases
  if [[ "${E2E_INFRA_DRIVER:-}" == "kind" ]]; then
    : "${E2E_SEED_FLEET_NAME:=default}"
    : "${E2E_SEED_CLUSTER_NAME:=local-kind}"
    : "${E2E_SEED_RELEASE_NAME:=dev-release}"
  else
    : "${E2E_SEED_FLEET_NAME:=}"
    : "${E2E_SEED_CLUSTER_NAME:=}"
    : "${E2E_SEED_RELEASE_NAME:=}"
  fi

  fleets=$(api_curl "${API_HOST}/api/hypershell/v1/fleets" 2>/dev/null || true)
  clusters=$(api_curl "${API_HOST}/api/hypershell/v1/managed_clusters" 2>/dev/null || true)
  releases=$(api_curl "${API_HOST}/api/hypershell/v1/gateway_releases" 2>/dev/null || true)
  databases=$(api_curl "${API_HOST}/api/hypershell/v1/managed_databases" 2>/dev/null || true)

  E2E_FLEET_ID=$(echo "$fleets" | e2e_json_first_id "${E2E_SEED_FLEET_NAME}")
  E2E_CLUSTER_ID=$(echo "$clusters" | e2e_json_first_id "${E2E_SEED_CLUSTER_NAME}")
  E2E_RELEASE_ID=$(echo "$releases" | e2e_json_first_id "${E2E_SEED_RELEASE_NAME}")
  E2E_DATABASE_ID=$(echo "$databases" | e2e_json_first_id)

  if [[ -z "${E2E_FLEET_ID}" || -z "${E2E_CLUSTER_ID}" || -z "${E2E_RELEASE_ID}" ]]; then
    red "ERROR: could not discover seeded fleet/cluster/release ids from the API"
    dim "  fleet=${E2E_SEED_FLEET_NAME:-<first>} id=${E2E_FLEET_ID:-<empty>}"
    dim "  cluster=${E2E_SEED_CLUSTER_NAME:-<first>} id=${E2E_CLUSTER_ID:-<empty>}"
    dim "  release=${E2E_SEED_RELEASE_NAME:-<first>} id=${E2E_RELEASE_ID:-<empty>}"
    return 1
  fi
}

e2e_seed_ids_ready() {
  [[ -n "${E2E_FLEET_ID:-}" && -n "${E2E_CLUSTER_ID:-}" && -n "${E2E_RELEASE_ID:-}" ]]
}

# Fill any missing seed ids from the API. Rediscover when fleet is set but
# cluster/release are not (the create path in e2e-openshell.sh only sets fleet).
e2e_ensure_seed_ids() {
  e2e_seed_ids_ready && return 0
  e2e_discover_seed_ids
}

# Copy fleet/cluster/release/database ids from a gateway JSON object or list.
# Does not overwrite ids that are already set.
e2e_apply_seed_ids_from_gateway_json() {
  local json="${1:-}" name="${2:-}"
  local parsed
  parsed=$(echo "$json" | WANT_NAME="$name" python3 -c "
import json, sys, os
name = os.environ.get('WANT_NAME', '')
try:
    data = json.load(sys.stdin)
except Exception:
    sys.exit(0)
obj = data
if isinstance(data, dict) and 'items' in data:
    obj = None
    for it in data.get('items') or []:
        if not name or it.get('name', '') == name:
            obj = it
            break
if not isinstance(obj, dict):
    sys.exit(0)
print('%s\t%s\t%s\t%s' % (
    obj.get('fleet_id', '') or '',
    obj.get('cluster_id', '') or '',
    obj.get('release_id', '') or '',
    obj.get('database_id', '') or '',
))
" 2>/dev/null || true)
  local fleet cluster release database
  IFS=$'\t' read -r fleet cluster release database <<< "$parsed" || true
  [[ -z "${E2E_FLEET_ID:-}" && -n "$fleet" ]] && E2E_FLEET_ID="$fleet"
  [[ -z "${E2E_CLUSTER_ID:-}" && -n "$cluster" ]] && E2E_CLUSTER_ID="$cluster"
  [[ -z "${E2E_RELEASE_ID:-}" && -n "$release" ]] && E2E_RELEASE_ID="$release"
  [[ -z "${E2E_DATABASE_ID:-}" && -n "$database" ]] && E2E_DATABASE_ID="$database"
}

# Print a gateway create body that reuses the seeded fleet/cluster/release/database ids.
e2e_gateway_create_body() {
  local name="${1:?gateway name required}"
  GW_NAME="$name" E2E_OIDC_ISSUER="$E2E_OIDC_ISSUER" \
    E2E_OIDC_CLIENT_ID="$E2E_OIDC_CLIENT_ID" \
    E2E_FLEET_ID="${E2E_FLEET_ID}" E2E_CLUSTER_ID="${E2E_CLUSTER_ID}" \
    E2E_RELEASE_ID="${E2E_RELEASE_ID}" E2E_DATABASE_ID="${E2E_DATABASE_ID:-}" python3 -c "
import json, os
body = {
    'name': os.environ['GW_NAME'],
    'fleet_id': os.environ['E2E_FLEET_ID'],
    'cluster_id': os.environ['E2E_CLUSTER_ID'],
    'release_id': os.environ['E2E_RELEASE_ID'],
    'database_id': os.environ.get('E2E_DATABASE_ID', ''),
    'oidc': json.dumps({
        'issuer': os.environ['E2E_OIDC_ISSUER'],
        'audience': os.environ['E2E_OIDC_CLIENT_ID'],
        'roles_claim': 'groups',
        'admin_role': 'hypershell-admins',
        'user_role': 'hypershell-users'
    }),
    'route': json.dumps({'enabled': True})
}
print(json.dumps(body))
"
}

# Poll until a gateway reports Running, or timeout. Echoes the last phase.
# Returns 0 on Running, 1 on timeout. Requires API_HOST and api_curl.
e2e_wait_gateway_running() {
  local gw_id="${1:?gateway id required}"
  local timeout="${2:-${E2E_PROVISION_TIMEOUT}}"
  local deadline=$(($(date +%s) + timeout))
  local phase=""
  while [[ $(date +%s) -lt $deadline ]]; do
    acquire_oidc_token 2>/dev/null || true
    phase=$(api_curl "${API_HOST}/api/hypershell/v1/gateways/${gw_id}" 2>/dev/null | \
      python3 -c "import json,sys; print(json.load(sys.stdin).get('phase',''))" 2>/dev/null || true)
    if [[ "$phase" == "Running" ]]; then
      echo "$phase"
      return 0
    fi
    sleep 5
  done
  echo "${phase:-unknown}"
  return 1
}

# Parse a gateway create/get JSON. Sets _CREATE_KIND (OK|ERROR|PARSE), _CREATE_ID,
# _CREATE_NAMESPACE, _CREATE_DATABASE_ID (or error code/reason in the ERROR case).
e2e_parse_gateway_response() {
  local json="${1:-}"
  _CREATE_KIND=""
  _CREATE_ID=""
  _CREATE_NAMESPACE=""
  _CREATE_DATABASE_ID=""
  IFS=$'\t' read -r _CREATE_KIND _CREATE_ID _CREATE_NAMESPACE _CREATE_DATABASE_ID <<< "$(echo "$json" | python3 -c "
import json, sys
try:
    d = json.load(sys.stdin)
except Exception:
    print('PARSE\t\t\t'); sys.exit(0)
if d.get('kind') == 'Error':
    print('ERROR\t%s\t%s\t' % (d.get('code', ''), d.get('reason', ''))); sys.exit(0)
print('OK\t%s\t%s\t%s' % (d.get('id', ''), d.get('namespace', ''), d.get('database_id', '')))
" 2>/dev/null)" || true
}
