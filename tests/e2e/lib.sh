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
: "${E2E_SKIP_CLEANUP:=0}"
: "${E2E_PAUSE:=1}"
: "${OPENSHELL_BIN:=openshell}"
: "${E2E_KEYCLOAK_NAMESPACE:=keycloak}"
: "${E2E_OIDC_ISSUER:=http://keycloak.hypershell.localhost:8080/realms/hypershell}"
: "${E2E_OIDC_ISSUER_INTERNAL:=http://keycloak-service.keycloak.svc.cluster.local:8080/realms/hypershell}"
: "${E2E_OIDC_CLIENT_ID:=hypershell-frontend}"
: "${E2E_OIDC_USERNAME:=admin}"
: "${E2E_OIDC_PASSWORD:=admin}"
