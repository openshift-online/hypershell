#!/usr/bin/env bash
# lib.sh - shared e2e test utilities.
#
# Provides pass/fail tracking, colored output, retry helpers, driver
# selection, and common environment defaults. Sourced by e2e-openshell.sh
# and e2e-performance.sh.

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
E2E_CURRENT_AREA=""
E2E_COMPLETED=""

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

# e2e_area - announce a numbered test area and record it as the current one,
# so print_results can name where the run stopped if it never reaches the end.
e2e_area() {
  E2E_CURRENT_AREA="$1"
  bold "$1"
}

print_results() {
  echo ""
  bold "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  if [[ -z "$E2E_COMPLETED" ]]; then
    if [[ -n "$E2E_CURRENT_AREA" ]]; then
      red "⚠ Run aborted during Area ${E2E_CURRENT_AREA} -- later areas did not run and are not reflected below."
    else
      red "⚠ Run aborted before any test area started -- no checks ran."
    fi
    echo ""
  fi
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

# Keep this rule consistent with buildOpenShellInstallCommand in
# packages/gateway-management-ui/src/gateways/gateway-connections.ts.
# Remove surrounding space and the first "-" suffix. Add a leading "v".
openshell_installer_version() {
  local raw="$1"
  # trim leading/trailing whitespace (mirrors the TS .trim())
  raw="${raw#"${raw%%[![:space:]]*}"}"
  raw="${raw%"${raw##*[![:space:]]}"}"
  [[ -z "$raw" ]] && return 1
  # Strip the first "-" and all following text (v0.0.109-rh9a8f8 -> v0.0.109).
  local base="${raw%%-*}"
  [[ -z "$base" ]] && return 1
  if [[ "$base" == v* ]]; then
    printf '%s' "$base"
  else
    printf 'v%s' "$base"
  fi
}

# Compare the version token, not a substring such as 0.0.11 in 0.0.110.
openshell_cli_matches_version() {
  local name version rest
  read -r name version rest <<< "$1"
  [[ "$name" == "openshell" && "${version#v}" == "${2#v}" ]]
}

e2e_validate_openshell_install() {
  case "$E2E_OPENSHELL_INSTALL" in
    auto|always|never) ;;
    *) red "ERROR: E2E_OPENSHELL_INSTALL must be auto, always, or never"; return 1 ;;
  esac
  if [[ ! "$E2E_GATEWAY_VERSION_TIMEOUT" =~ ^[1-9][0-9]*$ ]]; then
    red "ERROR: E2E_GATEWAY_VERSION_TIMEOUT must be a positive integer"
    return 1
  fi
}

# --- Environment defaults ---

: "${E2E_NAMESPACE:=openshell-e2e}"
: "${E2E_GATEWAY_NAME:=e2e-gw-$(head -c4 /dev/urandom | od -An -tx1 | tr -d ' \n')}"
: "${E2E_MODE:=long}"
: "${E2E_SANDBOX_TIMEOUT:=300}"
: "${E2E_PROVISION_TIMEOUT:=300}"
: "${E2E_GC_TIMEOUT:=300}"
: "${E2E_ORPHAN_GC_TIMEOUT:=300}"
: "${E2E_SKIP_CLEANUP:=0}"
: "${E2E_AUTO_SEED:=1}"
: "${E2E_PAUSE:=1}"
_E2E_LIB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
: "${OPENSHELL_BIN:=openshell}"
# How the e2e test obtains the openshell CLI:
#   auto   - install the gateway-matched version via the console-recommended
#            command if the CLI is not already present (default)
#   always - always install the gateway-matched version, even if one is present
#   never  - require a pre-installed CLI; do not install
: "${E2E_OPENSHELL_INSTALL:=auto}"
# Upstream install script the console links to (installScriptUrl in the UI).
: "${OPENSHELL_INSTALL_SCRIPT_URL:=https://raw.githubusercontent.com/openshift-online/hypershell/main/scripts/install-openshell.sh}"
# Bounded wait for the control plane to reconcile gateway_version from the
# gateway's health endpoint before deriving the install command.
: "${E2E_GATEWAY_VERSION_TIMEOUT:=300}"
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

# Token grant for acquire_oidc_token / acquire_gateway_token_with_role, per
# ephemeral-pr-environments.spec.md (HYPERSHELL-240). "password" (default) is the
# Kind and manual OpenShift path: a resource-owner password grant against seeded
# users. "client_credentials" is the GitHub-brokered pull-request path -- brokered
# GitHub users have no password grant, so CI authenticates through the confidential
# hypershell-e2e service-account client (client-credentials for the admin path) and
# Keycloak token exchange (impersonating the seeded developer principal). CI reads
# the hypershell-e2e secret from the deployed Keycloak namespace and exports it as
# E2E_OIDC_SA_CLIENT_SECRET; it never comes from a repo secret.
: "${E2E_OIDC_GRANT:=password}"
: "${E2E_OIDC_SA_CLIENT_ID:=hypershell-e2e}"
: "${E2E_OIDC_SA_CLIENT_SECRET:=}"

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

# Print available drivers and exit 1. Used when E2E_INFRA_DRIVER names a
# missing driver file.
e2e_die_unknown_driver() {
  local reason="$1"
  red "ERROR: ${reason}"
  echo ""
  echo "Available drivers:"
  e2e_list_available_drivers | while read -r d; do echo "  - $d"; done
  exit 1
}

# Detects OpenShift by checking whether the current KUBECONFIG context serves
# the route.openshift.io API group, an API only OpenShift clusters expose.
# Any other cluster is assumed to be Kind. Prints the driver name to stdout.
e2e_detect_infra_driver() {
  local api_versions
  if ! api_versions=$(kubectl api-versions 2>&1); then
    red "ERROR: 'kubectl api-versions' failed against the current KUBECONFIG context" >&2
    red "$api_versions" >&2
    exit 1
  fi
  if echo "$api_versions" | grep -q '^route\.openshift\.io/'; then
    echo "openshift"
  else
    echo "kind"
  fi
}

# Auto-detect E2E_INFRA_DRIVER from the current KUBECONFIG context when unset.
# An explicit E2E_INFRA_DRIVER value is left unchanged.
e2e_select_infra_driver() {
  if [[ -z "${E2E_INFRA_DRIVER:-}" ]]; then
    E2E_INFRA_DRIVER="$(e2e_detect_infra_driver)"
    dim "  Detected infra driver: ${E2E_INFRA_DRIVER} (from KUBECONFIG context; set E2E_INFRA_DRIVER to override)"
  fi
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
if isinstance(data, dict):
    if data.get('kind') == 'Error':
        sys.exit(0)
    items = data.get('items') or []
elif isinstance(data, list):
    items = data
else:
    items = []
for it in items:
    if isinstance(it, dict) and (not name or it.get('name', '') == name):
        print(it.get('id', '') or '')
        break
"
}

# One-line summary of a HyperShell list (or error) JSON body on stdin.
# Distinguishes empty lists from 401/403 Error payloads and unparseable bodies
# so seed-discovery failures are not just "id=<empty>".
e2e_json_list_summary() {
  python3 -c '
import json, sys
raw = sys.stdin.read()
if not raw.strip():
    print("empty-body")
    raise SystemExit(0)
try:
    data = json.loads(raw)
except Exception:
    print("unparseable")
    raise SystemExit(0)
if isinstance(data, list):
    print("kind=<list> items=%s" % len(data))
    raise SystemExit(0)
if not isinstance(data, dict):
    print("non-object")
    raise SystemExit(0)
if data.get("kind") == "Error":
    print("error code=%s reason=%s" % (data.get("code") or "", data.get("reason") or ""))
    raise SystemExit(0)
items = data.get("items") or []
if not isinstance(items, list):
    items = []
total = data.get("total")
total_s = "" if total is None else total
print("kind=%s total=%s items=%s" % (data.get("kind") or "", total_s, len(items)))
'
}

# Effective RBAC_DEFAULT_ROLES from an api-server container env JSON array
# (kubectl/oc jsonpath of .spec.template.spec.containers[?(@.name=="api-server")].env).
# Unset matches the Go default (gateway:creator). An explicit empty value is
# the OpenShift/production isolation posture and must not be treated as unset.
e2e_effective_rbac_default_roles_from_env_json() {
  python3 -c '
import json, sys
raw = sys.stdin.read().strip()
if not raw:
    print("gateway:creator")
    raise SystemExit(0)
try:
    env = json.loads(raw)
except Exception:
    print("gateway:creator")
    raise SystemExit(0)
if not isinstance(env, list):
    print("gateway:creator")
    raise SystemExit(0)
for item in env:
    if isinstance(item, dict) and item.get("name") == "RBAC_DEFAULT_ROLES":
        print(item.get("value") or "")
        raise SystemExit(0)
print("gateway:creator")
'
}

e2e_read_api_server_container_env() {
  local cli="${CLI:-kubectl}"
  local ns="${E2E_HS_NAMESPACE:-hypershell-system}"
  "$cli" get deployment hypershell-api-server -n "$ns" \
    -o jsonpath='{.spec.template.spec.containers[?(@.name=="api-server")].env}' \
    2>/dev/null || true
}

e2e_effective_rbac_default_roles() {
  local raw
  raw="$(e2e_read_api_server_container_env)"
  if [[ -z "${raw}" ]]; then
    if [[ "${E2E_INFRA_DRIVER:-}" == "openshift" ]]; then
      printf ''
      return 0
    fi
    printf '%s' 'gateway:creator'
    return 0
  fi
  printf '%s' "${raw}" | e2e_effective_rbac_default_roles_from_env_json
}

e2e_rbac_default_includes_creator() {
  if [[ -z "${_E2E_RBAC_DEFAULT_INCLUDES_CREATOR:-}" ]]; then
    local roles
    roles="$(e2e_effective_rbac_default_roles)"
    if [[ ",${roles}," == *",gateway:creator,"* ]]; then
      _E2E_RBAC_DEFAULT_INCLUDES_CREATOR=yes
    else
      _E2E_RBAC_DEFAULT_INCLUDES_CREATOR=no
    fi
  fi
  [[ "${_E2E_RBAC_DEFAULT_INCLUDES_CREATOR}" == "yes" ]]
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

# Discover the seeded cluster, release, and managed-database ids via the API.
# Sets E2E_CLUSTER_ID, E2E_RELEASE_ID, E2E_DATABASE_ID.
# Requires API_HOST and api_curl. Never hardcodes ids.
#
# Name pins (optional): E2E_SEED_CLUSTER_NAME, E2E_SEED_RELEASE_NAME.
# On kind these default to the make kind-up seeds (local-kind, dev-release).
# On openshift they default to the make openshift-seed names (local-openshift,
# dev-release). When a name is unset, the first list item is used - that matches
# single-seed CI/dev; multi-seed clusters should set the name pins instead
# of relying on API order.
#
# When both cluster and release lists are empty collections (not an API Error),
# and E2E_AUTO_SEED is not 0, discovery runs `SEED_STRICT=true make <driver>-seed`
# once and retries. That recovers a PR env whose last openshift-up wiped the
# database and failed before the seed step.
e2e_summary_is_empty_list() {
  [[ "${1:-}" == kind=*List* && "${1:-}" == *"items=0"* ]]
}

e2e_inventory_unseeded() {
  e2e_summary_is_empty_list "${_E2E_CLUSTER_LIST_SUMMARY:-}" \
    && e2e_summary_is_empty_list "${_E2E_RELEASE_LIST_SUMMARY:-}"
}

e2e_auto_seed_enabled() {
  case "${E2E_AUTO_SEED:-1}" in
    0|false|FALSE|no|NO) return 1 ;;
    *) return 0 ;;
  esac
}

e2e_run_platform_seed() {
  local root
  root="$(cd "${_E2E_LIB_DIR}/../.." && pwd)"
  case "${E2E_INFRA_DRIVER:-}" in
    kind)
      (cd "${root}" && SEED_STRICT=true make kind-seed)
      ;;
    openshift)
      (cd "${root}" && SEED_STRICT=true make openshift-seed)
      ;;
    *)
      red "ERROR: no platform seed target for driver '${E2E_INFRA_DRIVER:-}'"
      return 1
      ;;
  esac
}

e2e_print_seed_discovery_error() {
  red "ERROR: could not discover seeded cluster/release ids from the API"
  dim "  cluster=${E2E_SEED_CLUSTER_NAME:-<first>} id=${E2E_CLUSTER_ID:-<empty>} (${_E2E_CLUSTER_LIST_SUMMARY:-unknown})"
  dim "  release=${E2E_SEED_RELEASE_NAME:-<first>} id=${E2E_RELEASE_ID:-<empty>} (${_E2E_RELEASE_LIST_SUMMARY:-unknown})"
  dim "  database=${E2E_DATABASE_ID:-<empty>} (${_E2E_DATABASE_LIST_SUMMARY:-unknown})"
  dim "  Re-seed once the API is healthy: SEED_STRICT=true make openshift-seed"
  dim "  (Kind: SEED_STRICT=true make kind-seed)"
}

e2e_fetch_seed_ids() {
  local clusters releases databases
  if [[ "${E2E_INFRA_DRIVER:-}" == "kind" ]]; then
    : "${E2E_SEED_CLUSTER_NAME:=local-kind}"
    : "${E2E_SEED_RELEASE_NAME:=dev-release}"
  elif [[ "${E2E_INFRA_DRIVER:-}" == "openshift" ]]; then
    : "${E2E_SEED_CLUSTER_NAME:=local-openshift}"
    : "${E2E_SEED_RELEASE_NAME:=dev-release}"
  else
    : "${E2E_SEED_CLUSTER_NAME:=}"
    : "${E2E_SEED_RELEASE_NAME:=}"
  fi

  clusters=$(api_curl "${API_HOST}/api/hypershell/v1/managed_clusters" 2>/dev/null || true)
  releases=$(api_curl "${API_HOST}/api/hypershell/v1/gateway_releases" 2>/dev/null || true)
  databases=$(api_curl "${API_HOST}/api/hypershell/v1/managed_databases" 2>/dev/null || true)

  E2E_CLUSTER_ID=$(echo "$clusters" | e2e_json_first_id "${E2E_SEED_CLUSTER_NAME}")
  E2E_RELEASE_ID=$(echo "$releases" | e2e_json_first_id "${E2E_SEED_RELEASE_NAME}")
  E2E_DATABASE_ID=$(echo "$databases" | e2e_json_first_id)
  _E2E_CLUSTER_LIST_SUMMARY=$(echo "$clusters" | e2e_json_list_summary)
  _E2E_RELEASE_LIST_SUMMARY=$(echo "$releases" | e2e_json_list_summary)
  _E2E_DATABASE_LIST_SUMMARY=$(echo "$databases" | e2e_json_list_summary)

  e2e_seed_ids_ready
}

e2e_discover_seed_ids() {
  if e2e_fetch_seed_ids; then
    return 0
  fi
  if e2e_inventory_unseeded && e2e_auto_seed_enabled; then
    dim "  Seed inventory is empty; running SEED_STRICT=true make ${E2E_INFRA_DRIVER}-seed..."
    if ! e2e_run_platform_seed; then
      red "ERROR: platform seed failed; cannot discover cluster/release ids"
      e2e_print_seed_discovery_error
      return 1
    fi
    if e2e_fetch_seed_ids; then
      return 0
    fi
  fi
  e2e_print_seed_discovery_error
  return 1
}

e2e_seed_ids_ready() {
  [[ -n "${E2E_CLUSTER_ID:-}" && -n "${E2E_RELEASE_ID:-}" ]]
}

# Fill any missing seed ids from the API. Rediscover when cluster is set but
# release is not (or the reverse).
e2e_ensure_seed_ids() {
  e2e_seed_ids_ready && return 0
  e2e_discover_seed_ids
}

# Copy cluster/release/database ids from a gateway JSON object or list.
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
print('%s\t%s\t%s' % (
    obj.get('cluster_id', '') or '',
    obj.get('release_id', '') or '',
    obj.get('database_id', '') or '',
))
" 2>/dev/null || true)
  local cluster release database
  IFS=$'\t' read -r cluster release database <<< "$parsed" || true
  [[ -z "${E2E_CLUSTER_ID:-}" && -n "$cluster" ]] && E2E_CLUSTER_ID="$cluster"
  [[ -z "${E2E_RELEASE_ID:-}" && -n "$release" ]] && E2E_RELEASE_ID="$release"
  [[ -z "${E2E_DATABASE_ID:-}" && -n "$database" ]] && E2E_DATABASE_ID="$database"
}

# Print a gateway create body that reuses the seeded cluster/release/database ids.
e2e_gateway_create_body() {
  local name="${1:?gateway name required}"
  GW_NAME="$name" E2E_OIDC_ISSUER="$E2E_OIDC_ISSUER" \
    E2E_OIDC_CLIENT_ID="$E2E_OIDC_CLIENT_ID" \
    E2E_CLUSTER_ID="${E2E_CLUSTER_ID}" \
    E2E_RELEASE_ID="${E2E_RELEASE_ID}" E2E_DATABASE_ID="${E2E_DATABASE_ID:-}" python3 -c "
import json, os
body = {
    'name': os.environ['GW_NAME'],
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
