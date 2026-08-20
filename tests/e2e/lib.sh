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
: "${E2E_SANDBOX_TIMEOUT:=120}"
: "${E2E_PROVISION_TIMEOUT:=180}"
: "${E2E_GC_TIMEOUT:=180}"
: "${E2E_ORPHAN_GC_TIMEOUT:=90}"
: "${E2E_SKIP_CLEANUP:=0}"
: "${E2E_PAUSE:=1}"
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
