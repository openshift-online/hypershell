#!/usr/bin/env bash
# e2e-console.sh - browser end-to-end test of the HyperShell web console and the
# per-gateway OpenShell console, driven by agent-browser (headless Chromium).
#
# Same shape as e2e-openshell.sh: infra driver, lib.sh, E2E_MODE short|long.
# Every state the UI shows is re-read through the HyperShell API, which remains
# the source of truth. See specs/platform/e2e-console-browser-testing.spec.md.
#
# Usage:
#   bash tests/e2e/e2e-console.sh                 # long (default)
#   E2E_MODE=short make e2e-console               # CI gate
#
# Requirements: agent-browser (pinned in dependency-age-tools.json) with a
# Chromium, python3, curl, openssl. On Kind the browser must reach port 443
# directly (make kind-up forwards it; KIND_NO_SUDO=true does not).
#
# Environment variables (in addition to the shared e2e ones in lib.sh):
#   E2E_MODE                    short or long (default: long); perf is rejected
#   E2E_GATEWAY_NAME            Gateway name (default: e2e-console-gw-<random8hex>)
#   E2E_CONSOLE_READY_TIMEOUT   Seconds to wait for console_address (default: 300)
#   E2E_CONSOLE_ARTIFACT_DIR    Screenshots and failure evidence (default: ./e2e-console-artifacts)
#   E2E_CONSOLE_URL             Override the web console origin (default: discovered)
#   AGENT_BROWSER_BIN           agent-browser CLI (default: agent-browser)
#   AGENT_BROWSER_EXECUTABLE_PATH  Chromium to use instead of agent-browser's own
#   E2E_BROWSER_TIMEOUT_MS      Per-action timeout (default: 30000)
#   E2E_BROWSER_PAGE_TIMEOUT_MS Page-load timeout (default: 90000)
#   E2E_BROWSER_INSECURE        1 = --ignore-https-errors instead of CA-verified pins
#   E2E_BROWSER_HEADED          1 = show the browser window
#   E2E_BROWSER_NO_SANDBOX      1 = launch Chromium with --no-sandbox (default: 1 on Linux CI)
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# The console suite owns gateways named e2e-console-gw-*; set before lib.sh
# applies its own e2e-gw-* default.
: "${E2E_GATEWAY_NAME:=e2e-console-gw-$(head -c4 /dev/urandom | od -An -tx1 | tr -d ' \n')}"

# shellcheck source=lib.sh
source "${SCRIPT_DIR}/lib.sh"
# shellcheck source=browser-lib.sh
source "${SCRIPT_DIR}/browser-lib.sh"

: "${E2E_CONSOLE_READY_TIMEOUT:=300}"
: "${E2E_CONSOLE_URL:=}"
E2E_HS_NAMESPACE="${E2E_HS_NAMESPACE:-hypershell-system}"
RUN_ID="$(head -c3 /dev/urandom | od -An -tx1 | tr -d ' \n')"
PAGE_TIMEOUT_S=$((E2E_BROWSER_PAGE_TIMEOUT_MS / 1000))

# --- Mode and driver selection ---

e2e_console_validate_mode || exit 1
e2e_select_infra_driver

DRIVER_FILE="${SCRIPT_DIR}/drivers/${E2E_INFRA_DRIVER}.sh"
if [[ ! -f "$DRIVER_FILE" ]]; then
  e2e_die_unknown_driver "Unknown driver '${E2E_INFRA_DRIVER}'. Driver file not found: ${DRIVER_FILE}"
fi

# shellcheck source=drivers/kind.sh
source "$DRIVER_FILE"

REQUIRED_FUNCTIONS=(discover_api_host discover_console_host acquire_oidc_token api_curl get_cli_binary get_cluster_domain)
for fn in "${REQUIRED_FUNCTIONS[@]}"; do
  if ! declare -f "$fn" >/dev/null 2>&1; then
    red "ERROR: Driver '${E2E_INFRA_DRIVER}' does not implement required function: ${fn}"
    exit 1
  fi
done

# --- Configuration ---

CLI=$(get_cli_binary)
GW_NAME="${E2E_GATEWAY_NAME}"
GW_ID=""
GW_NAMESPACE=""
CONSOLE_URL=""
BROWSER_CA_FILE=""
ADMIN_SESSION="hs-e2e-admin-${RUN_ID}"
DEV_SESSION="hs-e2e-dev-${RUN_ID}"
SANDBOX_NAME="e2esb-${RUN_ID}"

# --- Cleanup trap ---

cleanup() {
  ab_session_close_all
  if [[ "$E2E_SKIP_CLEANUP" != "1" && -n "$GW_ID" ]]; then
    dim "  Cleaning up gateway ${GW_NAME} through the API..."
    # The token acquired earlier may have expired, or belong to the developer;
    # refresh the admin token best-effort before deleting.
    acquire_oidc_token 2>/dev/null || true
    api_curl -X DELETE "${API_HOST}/api/hypershell/v1/gateways/${GW_ID}" &>/dev/null || true
  fi
  if [[ -n "$BROWSER_CA_FILE" && "$(basename "$BROWSER_CA_FILE")" == hypershell-e2e-browser-ca.* ]]; then
    rm -f "$BROWSER_CA_FILE"
  fi
  print_results
}
trap cleanup EXIT

# --- API helpers ---

# gateway_json - print the gateway JSON for GW_ID (refreshing the admin token).
gateway_json() {
  acquire_oidc_token 2>/dev/null || true
  api_curl "${API_HOST}/api/hypershell/v1/gateways/${GW_ID}" 2>/dev/null || true
}

# gateway_field <key> - one field of the current gateway.
gateway_field() {
  gateway_json | e2e_json_field "$1"
}

# gateway_total - total gateways visible to the admin (for no-create checks).
gateway_total() {
  api_curl "${API_HOST}/api/hypershell/v1/gateways?size=1" 2>/dev/null | e2e_json_field total
}

# poll_active_sandbox_count <expected> - same contract as e2e-openshell.sh: poll
# the advisory, control-plane-owned count until it equals <expected>. Echoes the
# last value; returns 0 on match.
poll_active_sandbox_count() {
  local expected="$1" last="" deadline
  deadline=$(($(date +%s) + E2E_SANDBOX_TIMEOUT))
  while [[ $(date +%s) -lt $deadline ]]; do
    last=$(gateway_field active_sandbox_count)
    [[ "$last" == "$expected" ]] && { echo "$last"; return 0; }
    dim "    active_sandbox_count: ${last:-<unset>} (want ${expected})" >&2
    sleep 5
  done
  echo "$last"
  return 1
}

# gateway_detail_url - the web console detail route for GW_ID.
gateway_detail_url() {
  printf '%s/gateways/%s' "$CONSOLE_HOST" "$GW_ID"
}

# --- Discovery ---

if ! discover_api_host; then
  red "ERROR: Could not discover HyperShell API host over the gateway HTTPS route"
  exit 1
fi
API_HOST="${_DISCOVER_API_HOST}"

if [[ -n "$E2E_CONSOLE_URL" ]]; then
  CONSOLE_HOST="${E2E_CONSOLE_URL%/}"
elif discover_console_host; then
  CONSOLE_HOST="${_DISCOVER_CONSOLE_HOST}"
else
  red "ERROR: Could not discover the HyperShell web console host"
  exit 1
fi
CONSOLE_HOSTNAME="$(ab_url_host "$CONSOLE_HOST")"
ISSUER_HOSTNAME="$(ab_url_host "$E2E_OIDC_ISSUER")"
CLUSTER_DOMAIN="$(get_cluster_domain 2>/dev/null || true)"

# --- Banner ---

echo ""
bold "HyperShell Console Browser End-to-End Test"
sep
echo ""
printf '  %s\n' "1. Preflight (tools, port 443, TLS trust, API token, seeds)"
printf '  %s\n' "2. Web console login through Keycloak"
printf '  %s\n' "3. Gateway provisioning from the web console"
printf '  %s\n' "4. Console address published and linked"
printf '  %s\n' "5. OpenShell console login + gateway visibility"
printf '  %s\n' "6. Sandboxes from the OpenShell console"
printf '  %s\n' "7. Developer RBAC boundary [long]"
printf '  %s\n' "8. Gateway deletion from the web console + namespace GC"
printf '  %s\n' "9. Accessibility report + logout [long]"
echo ""
dim  "  Driver:            ${E2E_INFRA_DRIVER}"
dim  "  Mode:              ${E2E_MODE}"
dim  "  HyperShell API:    ${API_HOST}"
dim  "  Web console:       ${CONSOLE_HOST}"
dim  "  OIDC issuer:       ${E2E_OIDC_ISSUER}"
dim  "  Gateway name:      ${GW_NAME}"
dim  "  Admin user:        ${E2E_OIDC_USERNAME}"
dim  "  Developer user:    ${E2E_DEV_USERNAME}"
dim  "  Provision timeout: ${E2E_PROVISION_TIMEOUT}s"
dim  "  Console timeout:   ${E2E_CONSOLE_READY_TIMEOUT}s"
dim  "  Sandbox timeout:   ${E2E_SANDBOX_TIMEOUT}s"
dim  "  Artifacts:         ${E2E_CONSOLE_ARTIFACT_DIR}"
if [[ "$E2E_BROWSER_INSECURE" == "1" ]]; then
  red "  TLS:               INSECURE (--ignore-https-errors; E2E_BROWSER_INSECURE=1)"
fi
echo ""
sep

# ── 1. Preflight ─────────────────────────────────────────────────────────

echo ""
e2e_area "1. Preflight"
echo ""

if [[ "${E2E_OIDC_GRANT}" != "password" ]]; then
  fail_test "Browser login needs seeded passworded users (E2E_OIDC_GRANT=password), got '${E2E_OIDC_GRANT}'"
  exit 1
fi

if ! command -v "$AGENT_BROWSER_BIN" >/dev/null 2>&1; then
  fail_test "agent-browser not found (AGENT_BROWSER_BIN=${AGENT_BROWSER_BIN})"
  red "  Install the pinned version: $(ab_install_hint)"
  exit 1
fi
MISSING_TOOLS=()
for tool in python3 curl openssl; do
  command -v "$tool" >/dev/null 2>&1 || MISSING_TOOLS+=("$tool")
done
if ((${#MISSING_TOOLS[@]} > 0)); then
  fail_test "Missing required tools: ${MISSING_TOOLS[*]}"
  exit 1
fi
AB_VERSION="$("$AGENT_BROWSER_BIN" --version 2>/dev/null | awk '{print $NF}' || true)"
AB_PIN="$(ab_pinned_version)"
pass "Tools present: agent-browser ${AB_VERSION:-unknown}, python3, curl, openssl"
if [[ -n "$AB_PIN" && "$AB_VERSION" != "$AB_PIN" ]]; then
  dim "  Note: agent-browser ${AB_VERSION} differs from the pinned ${AB_PIN} ($(ab_install_hint))"
fi

show_cmd "agent-browser doctor --offline --quick --json  # locate Chromium"
if ! ab_browser_available; then
  dim "  No Chromium found; running agent-browser install..."
  "$AGENT_BROWSER_BIN" install >/dev/null 2>&1 || true
fi
if ab_browser_available; then
  pass "Chromium available: $(_ab_browser_executable)"
else
  fail_test "No Chromium available (set AGENT_BROWSER_EXECUTABLE_PATH or run: agent-browser install)"
  exit 1
fi

# A browser cannot use curl's --connect-to port remapping, so both origins must
# answer on port 443 as-is. Probe directly, never through _driver_curl.
PREFLIGHT_OK=1
for probe in "${CONSOLE_HOST}/auth/session" "${E2E_OIDC_ISSUER}/.well-known/openid-configuration"; do
  show_cmd "curl -sk --ipv4 -o /dev/null -w '%{http_code}' ${probe}"
  code=$(curl -sk --ipv4 --connect-timeout 5 -o /dev/null -w '%{http_code}' "$probe" 2>/dev/null || true)
  if [[ -z "$code" || "$code" == "000" ]]; then
    fail_test "$(ab_url_host "$probe") does not answer on port 443"
    PREFLIGHT_OK=0
  fi
done
if [[ "$PREFLIGHT_OK" != "1" ]]; then
  if [[ "$E2E_INFRA_DRIVER" == "kind" ]]; then
    _kind_discover_port 2>/dev/null || true
    red "  The browser suite requires the port 443 forwarding that 'make kind-up' sets up."
    red "  cloud-provider-kind listens on ephemeral port ${_KINDCCM_PORT:-<unknown>}; KIND_NO_SUDO=true does not forward 443."
  fi
  exit 1
fi
pass "Web console and Keycloak answer on port 443 without port remapping"

if acquire_oidc_token; then
  pass "API token acquired for ${E2E_OIDC_USERNAME}"
else
  fail_test "Could not acquire an API token for ${E2E_OIDC_USERNAME}"
  exit 1
fi

if ! e2e_ensure_seed_ids; then
  fail_test "Could not discover seeded cluster/release ids"
  exit 1
fi
SEED_CLUSTER_NAME="${E2E_SEED_CLUSTER_NAME:-}"
if [[ -z "$SEED_CLUSTER_NAME" ]]; then
  SEED_CLUSTER_NAME=$(api_curl "${API_HOST}/api/hypershell/v1/managed_clusters/${E2E_CLUSTER_ID}" 2>/dev/null | e2e_json_field name)
fi
pass "Seeded cluster: ${SEED_CLUSTER_NAME} (${E2E_CLUSTER_ID})"

if declare -f get_browser_ca_bundle >/dev/null 2>&1; then
  BROWSER_CA_FILE="$(get_browser_ca_bundle || true)"
fi
# The gateway console host is unknown until a gateway exists; pin a probe name
# under the gateway base domain (served by the same wildcard certificate).
TLS_HOSTS=("$CONSOLE_HOSTNAME" "$ISSUER_HOSTNAME")
[[ -n "$CLUSTER_DOMAIN" ]] && TLS_HOSTS+=("?console-e2e-probe.${CLUSTER_DOMAIN}")
show_cmd "openssl s_client -servername <host> | openssl verify -CAfile ${BROWSER_CA_FILE:-<none>}  # ${TLS_HOSTS[*]}"
if ab_tls_setup "$BROWSER_CA_FILE" "${TLS_HOSTS[@]}"; then
  case "$AB_TLS_MODE" in
    spki-pin) pass "Browser trusts only certificates verified against the cluster CA ($(tr ',' '\n' <<< "$AB_PINS" | wc -l | tr -d ' ') pinned)" ;;
    system) pass "Browser uses the system trust store (no cluster CA bundle)" ;;
    insecure) orange "  ⚠ Browser TLS verification disabled (E2E_BROWSER_INSECURE=1)" ;;
  esac
else
  fail_test "Could not establish browser TLS trust for ${TLS_HOSTS[*]}"
  exit 1
fi

if ab_session_start "$ADMIN_SESSION"; then
  pass "Browser session started: ${ADMIN_SESSION}"
else
  fail_test "Could not start the agent-browser session ${ADMIN_SESSION}"
  exit 1
fi
sep

# ── 2. Web console login ─────────────────────────────────────────────────

echo ""
e2e_area "2. Web Console Login Through Keycloak"
echo ""

if ! ab_open "${CONSOLE_HOST}/"; then
  ab_fail "Could not open the web console at ${CONSOLE_HOST}/"
  exit 1
fi
if ab_keycloak_login "$E2E_OIDC_USERNAME" "$E2E_OIDC_PASSWORD"; then
  pass "Web console redirected to the Keycloak login form; submitted as ${E2E_OIDC_USERNAME}"
else
  ab_fail "Keycloak login form did not appear for the web console"
  exit 1
fi
if ab_wait_url "${CONSOLE_HOST}/**" "$E2E_BROWSER_PAGE_TIMEOUT_MS" \
    && ab_wait_text "OpenShell Gateways" "$E2E_BROWSER_PAGE_TIMEOUT_MS"; then
  pass "Landed on ${CONSOLE_HOSTNAME} with the 'OpenShell Gateways' heading"
else
  ab_fail "Did not land on the web console gateway list after login"
  exit 1
fi

show_cmd "agent-browser eval \"fetch('/auth/session')\""
IFS=$'\t' read -r SESSION_AUTH SESSION_USER SESSION_DISPLAY <<< "$(ab_fetch_json /auth/session 2>/dev/null | e2e_console_session_fields)"
if [[ "$SESSION_AUTH" == "true" && "$SESSION_USER" == "$E2E_OIDC_USERNAME" ]]; then
  pass "BFF /auth/session: authenticated as ${SESSION_USER}"
else
  ab_fail "BFF /auth/session not authenticated as ${E2E_OIDC_USERNAME} (authenticated=${SESSION_AUTH}, user=${SESSION_USER})"
  exit 1
fi
if acquire_oidc_token "$E2E_OIDC_USERNAME" "$E2E_OIDC_PASSWORD" \
    && [[ "$(_token_debug_claims "$_OIDC_ACCESS_TOKEN")" == "preferred_username=${E2E_OIDC_USERNAME} "* ]]; then
  pass "API-side identity matches the browser session (${E2E_OIDC_USERNAME})"
else
  fail_test "API token identity does not match ${E2E_OIDC_USERNAME}"
fi
ab_shot "02-gateway-list"
sep

# ── 3. Provision gateway ─────────────────────────────────────────────────

echo ""
e2e_area "3. Gateway Provisioning From the Web Console"
echo ""

NAME_TEXTBOX='^Gateway name$'

if e2e_step long; then
  dim "  Validation guard: submit with an empty name"
  TOTAL_BEFORE=$(gateway_total)
  if ab_open "${CONSOLE_HOST}/gateways/new" \
      && ab_wait_ref textbox "$NAME_TEXTBOX" "$PAGE_TIMEOUT_S" >/dev/null \
      && SUBMIT_REF=$(ab_ref button '^Provision gateway$' form) && [[ -n "$SUBMIT_REF" ]] \
      && ab_ok click "$SUBMIT_REF" \
      && ab_wait_text "This field is required."; then
    pass "Empty gateway name shows 'This field is required.'"
  else
    ab_fail "Empty-name submission did not show the required-field helper text"
  fi
  sleep 2
  TOTAL_AFTER=$(gateway_total)
  if [[ -n "$TOTAL_BEFORE" && "$TOTAL_BEFORE" == "$TOTAL_AFTER" ]]; then
    pass "No gateway created by the invalid submission (total ${TOTAL_AFTER})"
  else
    fail_test "Gateway total changed after an invalid submission (${TOTAL_BEFORE:-?} -> ${TOTAL_AFTER:-?})"
  fi
fi

if ! ab_open "${CONSOLE_HOST}/gateways/new"; then
  ab_fail "Could not open ${CONSOLE_HOST}/gateways/new"
  exit 1
fi
NAME_REF=$(ab_wait_ref textbox "$NAME_TEXTBOX" "$PAGE_TIMEOUT_S" || true)
if [[ -n "$NAME_REF" ]] && show_cmd "agent-browser fill ${NAME_REF} ${GW_NAME}  # Gateway name" && ab_ok fill "$NAME_REF" "$GW_NAME"; then
  pass "Filled 'Gateway name' with ${GW_NAME}"
else
  ab_fail "Could not fill the 'Gateway name' field"
  exit 1
fi

CLUSTER_OPTION_RE="^$(ab_regex_escape "$SEED_CLUSTER_NAME")( |$)"
TOGGLE_REF=$(ab_ref button '^Select a cluster$' || true)
OPTION_REF=""
if [[ -n "$TOGGLE_REF" ]]; then
  show_cmd "agent-browser click ${TOGGLE_REF}  # Select a cluster"
  ab_ok click "$TOGGLE_REF" || true
  OPTION_REF=$(ab_wait_ref option "$CLUSTER_OPTION_RE" 15 || true)
fi
if [[ -n "$OPTION_REF" ]] && show_cmd "agent-browser click ${OPTION_REF}  # ${SEED_CLUSTER_NAME}" && ab_ok click "$OPTION_REF"; then
  pass "Selected cluster ${SEED_CLUSTER_NAME} in the Cluster typeahead"
else
  ab_fail "Cluster option ${SEED_CLUSTER_NAME} not found in the Cluster typeahead"
  exit 1
fi

SUBMIT_REF=$(ab_ref button '^Provision gateway$' form || true)
show_cmd "agent-browser click ${SUBMIT_REF:-<missing>}  # Provision gateway"
if [[ -n "$SUBMIT_REF" ]] && ab_ok click "$SUBMIT_REF" \
    && ab_wait_fn "/^\\/gateways\\/[^/]+\$/.test(location.pathname) && location.pathname !== '/gateways/new'" "$E2E_BROWSER_PAGE_TIMEOUT_MS"; then
  UI_GW_ID="$(ab_url)"
  UI_GW_ID="${UI_GW_ID%%\?*}"
  UI_GW_ID="${UI_GW_ID##*/}"
  pass "Form submitted; navigated to /gateways/${UI_GW_ID}"
else
  ab_fail "Submitting 'Provision gateway' did not navigate to the gateway detail page"
  exit 1
fi

# Own the gateway from here on (the cleanup trap deletes it on any exit).
GW_ID="$UI_GW_ID"
show_cmd "api_curl ${API_HOST}/api/hypershell/v1/gateways?search=name='${GW_NAME}'"
LOOKUP_DEADLINE=$(($(date +%s) + 60))
while [[ $(date +%s) -lt $LOOKUP_DEADLINE ]]; do
  acquire_oidc_token 2>/dev/null || true
  e2e_lookup_gateway_by_name "$GW_NAME"
  [[ -n "$_GW_ID" && -n "$_GW_NAMESPACE" ]] && break
  sleep 3
done
if [[ -n "$_GW_ID" && "$_GW_ID" == "$UI_GW_ID" ]]; then
  GW_NAMESPACE="$_GW_NAMESPACE"
  pass "API lists ${GW_NAME} with the id the UI navigated to (${GW_ID}, namespace ${GW_NAMESPACE:-<pending>})"
else
  fail_test "API lookup of ${GW_NAME} returned id '${_GW_ID}', UI navigated to '${UI_GW_ID}'"
  [[ -n "$_GW_ID" ]] && GW_ID="$_GW_ID"
  exit 1
fi
GW_CLUSTER_ID=$(gateway_field cluster_id)
if [[ "$GW_CLUSTER_ID" == "$E2E_CLUSTER_ID" ]]; then
  pass "Gateway placed on the selected cluster (${SEED_CLUSTER_NAME})"
else
  fail_test "Gateway cluster_id '${GW_CLUSTER_ID}' != selected ${E2E_CLUSTER_ID}"
fi

dim "  Waiting for phase Running (timeout: ${E2E_PROVISION_TIMEOUT}s)..."
show_cmd "api_curl ${API_HOST}/api/hypershell/v1/gateways/${GW_ID}  # phase == Running"
if PHASE=$(e2e_wait_gateway_running "$GW_ID" "$E2E_PROVISION_TIMEOUT"); then
  pass "API reports phase Running"
else
  ab_fail "Gateway did not reach Running within ${E2E_PROVISION_TIMEOUT}s (last phase: ${PHASE})"
  exit 1
fi
[[ -z "$GW_NAMESPACE" ]] && GW_NAMESPACE=$(gateway_field namespace)

# The UI label for a Running gateway is its health status (e.g. "Healthy"),
# falling back to the phase (resolveGatewayDisplayStatus in gateway-data.ts).
DISPLAY_STATUS=$(gateway_field status)
DISPLAY_STATUS="${DISPLAY_STATUS:-Running}"
if ab_open "$(gateway_detail_url)" \
    && ab_wait_ref heading "^$(ab_regex_escape "$GW_NAME")\$" "$PAGE_TIMEOUT_S" >/dev/null \
    && ab_wait_text "$DISPLAY_STATUS" "$E2E_BROWSER_PAGE_TIMEOUT_MS"; then
  pass "Detail page shows ${GW_NAME} with status '${DISPLAY_STATUS}' (API phase Running)"
else
  ab_fail "Detail page did not show ${GW_NAME} with status '${DISPLAY_STATUS}'"
fi
ab_shot "03-gateway-running"
sep

# ── 4. Console address + button ──────────────────────────────────────────

echo ""
e2e_area "4. Console Address Published and Linked"
echo ""

CONSOLE_OK=0
dim "  Waiting for console_address (timeout: ${E2E_CONSOLE_READY_TIMEOUT}s)..."
show_cmd "api_curl ${API_HOST}/api/hypershell/v1/gateways/${GW_ID}  # console_address != ''"
CONSOLE_DEADLINE=$(($(date +%s) + E2E_CONSOLE_READY_TIMEOUT))
while [[ $(date +%s) -lt $CONSOLE_DEADLINE ]]; do
  CONSOLE_URL=$(gateway_field console_address)
  [[ -n "$CONSOLE_URL" ]] && break
  sleep 5
done
CONSOLE_URL="${CONSOLE_URL%/}"

LINK_NAME_RE="^Open console for $(ab_regex_escape "$GW_NAME") in a new tab\$"
if [[ -z "$CONSOLE_URL" ]]; then
  fail_test "API never published console_address within ${E2E_CONSOLE_READY_TIMEOUT}s"
  if ab_open "$(gateway_detail_url)"; then
    BUTTON_REF=$(ab_wait_ref button "$LINK_NAME_RE" 30 || true)
    if [[ -n "$BUTTON_REF" ]]; then
      ab_ok hover "$BUTTON_REF" || true
      sleep 1
      if ab_wait_text "Console unavailable for this gateway" 5000; then
        ab_fail "Detail page reports 'Console unavailable for this gateway'"
      else
        ab_fail "Console button still disabled ('Provisioning console...')"
      fi
    fi
  fi
else
  EXPECTED_CONSOLE="https://console-${GW_NAMESPACE}.${CLUSTER_DOMAIN}"
  if [[ "$E2E_INFRA_DRIVER" != "kind" || "$CONSOLE_URL" == "$EXPECTED_CONSOLE" ]]; then
    pass "API published console_address ${CONSOLE_URL}"
  else
    fail_test "console_address ${CONSOLE_URL} != expected ${EXPECTED_CONSOLE}"
  fi

  # The detail page polls the gateway; reload it so the enabled link renders.
  LINK_REF=""
  if ab_open "$(gateway_detail_url)"; then
    LINK_REF=$(ab_wait_ref link "$LINK_NAME_RE" "$PAGE_TIMEOUT_S" || true)
  fi
  if [[ -n "$LINK_REF" ]]; then
    show_cmd "agent-browser get attr ${LINK_REF} href  # Open gateway console"
    LINK_HREF=$(ab_json get attr "$LINK_REF" href | _ab_json_get value 2>/dev/null || true)
    if [[ "${LINK_HREF%/}" == "$CONSOLE_URL" ]]; then
      pass "'Open gateway console' is an enabled link to console_address"
      CONSOLE_OK=1
    else
      ab_fail "'Open gateway console' href '${LINK_HREF}' != console_address '${CONSOLE_URL}'"
    fi
  else
    ab_fail "'Open gateway console' is not an enabled link although console_address is published"
  fi
fi
ab_shot "04-console-link"
sep

# ── 5. OpenShell console ─────────────────────────────────────────────────

echo ""
e2e_area "5. OpenShell Console Login and Gateway Visibility"
echo ""

GW_CLIENT_ID="${GW_NAME}-${GW_ID}"
if [[ "$CONSOLE_OK" != "1" ]]; then
  fail_test "Skipped: no console link to follow (see area 4)"
else
  CONSOLE_HOSTNAME_GW="$(ab_url_host "$CONSOLE_URL")"
  if ! ab_host_trusted "$CONSOLE_HOSTNAME_GW"; then
    fail_test "Browser does not trust ${CONSOLE_HOSTNAME_GW}: its certificate was not pinned in preflight"
    CONSOLE_OK=0
  fi
fi

if [[ "$CONSOLE_OK" == "1" ]]; then
  # The owner binding -> reconciler -> client-role bridge is asynchronous. The
  # console session captures roles at login, so wait for the role first.
  show_cmd "acquire_gateway_token_with_role ${E2E_OIDC_USERNAME} **** ${GW_CLIENT_ID} openshell-admin"
  if acquire_gateway_token_with_role "$E2E_OIDC_USERNAME" "$E2E_OIDC_PASSWORD" "$GW_CLIENT_ID" openshell-admin "$E2E_CONSOLE_READY_TIMEOUT"; then
    pass "${E2E_OIDC_USERNAME} holds openshell-admin on ${GW_CLIENT_ID}"
  else
    fail_test "${E2E_OIDC_USERNAME} never received openshell-admin on ${GW_CLIENT_ID}"
  fi
  acquire_oidc_token 2>/dev/null || true

  if ab_open "${CONSOLE_URL}/"; then
    if ab_on_keycloak_form; then
      dim "  Keycloak form shown for the console (no SSO reuse); logging in again"
      ab_keycloak_login "$E2E_OIDC_USERNAME" "$E2E_OIDC_PASSWORD" || true
    fi
    if ab_wait_url "${CONSOLE_URL}/**" "$E2E_BROWSER_PAGE_TIMEOUT_MS"; then
      pass "oauth2-proxy login completed; browser is on ${CONSOLE_HOSTNAME_GW}"
    else
      ab_fail "Browser did not return to ${CONSOLE_URL} after the oauth2-proxy login"
      CONSOLE_OK=0
    fi
  else
    ab_fail "Could not open ${CONSOLE_URL}"
    CONSOLE_OK=0
  fi
fi

if [[ "$CONSOLE_OK" == "1" ]]; then
  show_cmd "agent-browser eval \"fetch('/api/v1/auth/whoami')\""
  IFS=$'\t' read -r WHO_NAME WHO_ROLES <<< "$(ab_fetch_json /api/v1/auth/whoami 2>/dev/null | e2e_console_whoami_fields)"
  if [[ "$WHO_NAME" == "$E2E_OIDC_USERNAME" ]] && e2e_console_list_has "$WHO_ROLES" openshell-admin; then
    pass "Console whoami: ${WHO_NAME} with roles ${WHO_ROLES}"
  else
    ab_fail "Console whoami: want ${E2E_OIDC_USERNAME} with openshell-admin, got '${WHO_NAME}' roles '${WHO_ROLES}'"
  fi

  if ab_open "${CONSOLE_URL}/gateway" \
      && ab_wait_fn "!!document.querySelector('[data-testid=\"gateway-status-card\"]') && !!document.querySelector('[data-testid=\"gateway-version-card\"]')" "$E2E_BROWSER_PAGE_TIMEOUT_MS"; then
    pass "Console /gateway shows gateway-status-card and gateway-version-card"
  else
    ab_fail "Console /gateway did not render the status and version cards"
  fi
  if ab_wait_text "Cannot reach the OpenShell gateway" 8000; then
    ab_fail "Console reports 'Cannot reach the OpenShell gateway'"
  else
    pass "Console reaches the OpenShell gateway (no connection error)"
  fi

  API_GW_VERSION=""
  VERSION_DEADLINE=$(($(date +%s) + E2E_GATEWAY_VERSION_TIMEOUT))
  while [[ $(date +%s) -lt $VERSION_DEADLINE ]]; do
    API_GW_VERSION=$(gateway_field gateway_version)
    [[ -n "$API_GW_VERSION" ]] && break
    sleep 5
  done
  UI_VERSION_TEXT=$(ab_testid_text gateway-version-card 2>/dev/null || true)
  if e2e_console_version_matches "$UI_VERSION_TEXT" "$API_GW_VERSION"; then
    pass "Console version matches API gateway_version ${API_GW_VERSION}"
  else
    ab_fail "Console version '$(tr '\n' ' ' <<< "$UI_VERSION_TEXT")' does not match API gateway_version '${API_GW_VERSION}'"
  fi
  ab_shot "05-console-gateway"
fi
sep

# ── 6. Sandboxes from the console ────────────────────────────────────────

echo ""
e2e_area "6. Sandboxes From the OpenShell Console"
echo ""

WORKSPACE=""
if [[ "$CONSOLE_OK" != "1" ]]; then
  fail_test "Skipped: OpenShell console not reachable (see areas 4-5)"
else
  if ab_open "${CONSOLE_URL}/workspaces" \
      && ab_wait_fn "!!document.querySelector('[data-testid=\"workspace-table\"]')" "$E2E_BROWSER_PAGE_TIMEOUT_MS"; then
    WORKSPACE=$(ab_eval "(() => { const l = document.querySelector('[data-testid=\"workspace-link-default\"]') || document.querySelector('[data-testid^=\"workspace-link-\"]'); return l ? l.dataset.testid.replace('workspace-link-', '') : ''; })()" 2>/dev/null || true)
  fi
  if [[ -n "$WORKSPACE" ]] \
      && show_cmd "agent-browser click [data-testid=workspace-link-${WORKSPACE}]" \
      && ab_ok click "[data-testid=\"workspace-link-${WORKSPACE}\"]" \
      && ab_wait_url "**/workspaces/${WORKSPACE}*"; then
    pass "Entered workspace '${WORKSPACE}'"
  else
    ab_fail "Could not enter a workspace from ${CONSOLE_URL}/workspaces"
    WORKSPACE=""
  fi
fi

SANDBOX_LIST_READY="!!document.querySelector('[data-testid=\"sandbox-table\"]') || document.body.innerText.includes('No sandboxes')"
if [[ -n "$WORKSPACE" ]]; then
  if ab_wait_fn "$SANDBOX_LIST_READY"; then
    pass "Sandbox list renders (table or 'No sandboxes')"
  else
    ab_fail "Sandbox list did not render in workspace ${WORKSPACE}"
  fi
  ab_shot "06-sandbox-list"
fi

if [[ -n "$WORKSPACE" ]] && ! e2e_step long; then
  COUNT=$(gateway_field active_sandbox_count)
  if [[ -z "$COUNT" || "$COUNT" == "0" ]]; then
    pass "active_sandbox_count is ${COUNT:-absent} for a gateway with no sandboxes"
  else
    fail_test "active_sandbox_count is ${COUNT}, want 0 or absent"
  fi
fi

if [[ -n "$WORKSPACE" ]] && e2e_step long; then
  SANDBOX_POD="${WORKSPACE}--${SANDBOX_NAME}"
  CREATE_ID="create-sandbox"
  ab_testid_present "$CREATE_ID" || CREATE_ID="create-sandbox-empty"
  show_cmd "agent-browser click [data-testid=${CREATE_ID}]; fill sandbox-name-input ${SANDBOX_NAME}; fill sandbox-image-input base; click create-sandbox-submit"
  if ab_ok click "[data-testid=\"${CREATE_ID}\"]" \
      && ab_wait_fn "!!document.querySelector('[data-testid=\"sandbox-name-input\"]')" \
      && ab_ok fill "[data-testid=\"sandbox-name-input\"]" "$SANDBOX_NAME" \
      && ab_ok fill "[data-testid=\"sandbox-image-input\"]" "base" \
      && ab_ok click "[data-testid=\"create-sandbox-submit\"]"; then
    pass "Submitted the Create sandbox dialog for ${SANDBOX_NAME}"
  else
    ab_fail "Could not submit the Create sandbox dialog"
  fi

  dim "  Waiting for pod ${SANDBOX_POD} in ${GW_NAMESPACE} (timeout: ${E2E_SANDBOX_TIMEOUT}s)..."
  show_cmd "$CLI get pod ${SANDBOX_POD} -n ${GW_NAMESPACE} -o jsonpath='{.status.phase}'"
  POD_PHASE=""
  POD_DEADLINE=$(($(date +%s) + E2E_SANDBOX_TIMEOUT))
  while [[ $(date +%s) -lt $POD_DEADLINE ]]; do
    POD_PHASE=$($CLI get pod "$SANDBOX_POD" -n "$GW_NAMESPACE" --request-timeout=10s -o jsonpath='{.status.phase}' 2>/dev/null || true)
    [[ "$POD_PHASE" == "Running" ]] && break
    sleep 5
  done
  if [[ "$POD_PHASE" == "Running" ]]; then
    pass "Sandbox pod ${SANDBOX_POD} is Running"
  else
    ab_fail "Sandbox pod ${SANDBOX_POD} not Running within ${E2E_SANDBOX_TIMEOUT}s (phase: ${POD_PHASE:-absent})"
  fi

  SB_ROW_JS="Array.from(document.querySelectorAll('[data-testid=\"sandbox-table\"] tbody tr')).find(r => r.innerText.includes($(ab_js_string "$SANDBOX_NAME")))"
  ab_open "${CONSOLE_URL}/workspaces/${WORKSPACE}" >/dev/null || true
  if ab_wait_fn "(() => { const r = ${SB_ROW_JS}; return !!r && /running|ready/i.test(r.innerText); })()" "$((E2E_SANDBOX_TIMEOUT * 1000))"; then
    pass "Sandbox row for ${SANDBOX_NAME} shows a running status"
  else
    ab_fail "Sandbox row for ${SANDBOX_NAME} never showed a running status"
  fi
  ab_shot "06-sandbox-running"

  show_cmd "api_curl ${API_HOST}/api/hypershell/v1/gateways/${GW_ID}  # active_sandbox_count == 1"
  if COUNT=$(poll_active_sandbox_count 1); then
    pass "active_sandbox_count reflects the console-created sandbox (${COUNT})"
  else
    fail_test "active_sandbox_count did not reach 1 within ${E2E_SANDBOX_TIMEOUT}s (last: ${COUNT:-<unset>})"
  fi

  show_cmd "agent-browser click row kebab (Kebab toggle) for ${SANDBOX_NAME}; Delete; type name; confirm"
  DELETE_OK=0
  if ab_eval "(() => { const r = ${SB_ROW_JS}; const k = r && r.querySelector('button[aria-label=\"Kebab toggle\"]'); if (k) { k.click(); return 'ok'; } return ''; })()" 2>/dev/null | grep -q ok; then
    ITEM_REF=$(ab_wait_ref menuitem '^Delete' 10 || true)
    if [[ -n "$ITEM_REF" ]] && ab_ok click "$ITEM_REF"; then
      # The modal requires typing the sandbox name before the Delete button is enabled
      if ab_ok fill "[data-testid=\"confirm-delete-name-input\"]" "$SANDBOX_NAME"; then
        CONFIRM_REF=$(ab_wait_ref button '^Delete$' 10 '[role=dialog]' || true)
        if [[ -n "$CONFIRM_REF" ]] && ab_ok click "$CONFIRM_REF"; then
          DELETE_OK=1
        fi
      fi
    fi
  fi
  if [[ "$DELETE_OK" == "1" ]]; then
    pass "Deleted ${SANDBOX_NAME} from the row actions"
  else
    ab_fail "Could not delete ${SANDBOX_NAME} from the sandbox row actions"
  fi

  POD_GONE=false
  POD_DEADLINE=$(($(date +%s) + E2E_SANDBOX_TIMEOUT))
  while [[ $(date +%s) -lt $POD_DEADLINE ]]; do
    if ! $CLI get pod "$SANDBOX_POD" -n "$GW_NAMESPACE" --request-timeout=10s &>/dev/null; then
      POD_GONE=true
      break
    fi
    sleep 5
  done
  if [[ "$POD_GONE" == "true" ]]; then
    pass "Sandbox pod ${SANDBOX_POD} deleted"
  else
    fail_test "Sandbox pod ${SANDBOX_POD} still present after ${E2E_SANDBOX_TIMEOUT}s"
  fi
  show_cmd "api_curl ${API_HOST}/api/hypershell/v1/gateways/${GW_ID}  # active_sandbox_count == 0"
  if COUNT=$(poll_active_sandbox_count 0); then
    pass "active_sandbox_count returned to 0 (${COUNT})"
  else
    fail_test "active_sandbox_count did not return to 0 within ${E2E_SANDBOX_TIMEOUT}s (last: ${COUNT:-<unset>})"
  fi
fi
sep

# ── 7. Developer RBAC boundary ───────────────────────────────────────────

echo ""
e2e_area "7. Developer RBAC Boundary"
echo ""

if ! e2e_step long || ! e2e_multi_identity; then
  dim "  Skipped (E2E_MODE=${E2E_MODE}: single identity)"
elif [[ "$CONSOLE_OK" != "1" ]]; then
  fail_test "Skipped: OpenShell console not reachable (see areas 4-5)"
elif ! ab_session_start "$DEV_SESSION"; then
  fail_test "Could not start the developer browser session"
else
  pass "Isolated developer session started: ${DEV_SESSION}"
  if ab_open "${CONSOLE_HOST}/" \
      && ab_keycloak_login "$E2E_DEV_USERNAME" "$E2E_DEV_PASSWORD" \
      && ab_wait_url "${CONSOLE_HOST}/**" "$E2E_BROWSER_PAGE_TIMEOUT_MS" \
      && ab_wait_text "OpenShell Gateways" "$E2E_BROWSER_PAGE_TIMEOUT_MS"; then
    pass "Developer logged in to the web console"
  else
    ab_fail "Developer could not log in to the web console"
  fi
  DEV_LIST_STATUS=$(ab_fetch_status /api/hypershell/v1/gateways 2>/dev/null || true)
  if [[ "$DEV_LIST_STATUS" == "200" ]]; then
    pass "Developer gateway list loads through the BFF (HTTP 200)"
  else
    ab_fail "Developer gateway list returned HTTP ${DEV_LIST_STATUS:-none}"
  fi

  # The developer holds no RoleBinding on the admin's gateway.
  if ab_open "${CONSOLE_URL}/"; then
    ab_on_keycloak_form && { ab_keycloak_login "$E2E_DEV_USERNAME" "$E2E_DEV_PASSWORD" || true; }
    ab_wait_url "${CONSOLE_URL}/**" "$E2E_BROWSER_PAGE_TIMEOUT_MS" || true
  fi
  DEV_GATEWAY_STATUS=$(ab_fetch_status /api/v1/gateway 2>/dev/null || true)
  IFS=$'\t' read -r DEV_WHO_NAME DEV_WHO_ROLES <<< "$(ab_fetch_json /api/v1/auth/whoami 2>/dev/null | e2e_console_whoami_fields)"
  dim "  Developer console: /api/v1/gateway HTTP ${DEV_GATEWAY_STATUS:-none}; whoami '${DEV_WHO_NAME}' roles '${DEV_WHO_ROLES}'"
  if [[ -n "$DEV_GATEWAY_STATUS" && "$DEV_GATEWAY_STATUS" != "200" ]] \
      && ! e2e_console_list_has "$DEV_WHO_ROLES" openshell-admin \
      && ! e2e_console_list_has "$DEV_WHO_ROLES" openshell-user; then
    pass "Console denies the developer (no gateway role; /api/v1/gateway HTTP ${DEV_GATEWAY_STATUS})"
  else
    ab_fail "Console did not deny the developer (/api/v1/gateway HTTP ${DEV_GATEWAY_STATUS:-none}, roles '${DEV_WHO_ROLES}')"
  fi
  if ab_testid_present gateway-status-card; then
    ab_fail "Developer can see the gateway status card"
  else
    pass "Developer does not see the gateway status card"
  fi
  ab_shot "07-developer-denied"
  ab_session_close "$DEV_SESSION"
  ab_use "$ADMIN_SESSION"
fi
sep

# ── 8. Delete gateway ────────────────────────────────────────────────────

echo ""
e2e_area "8. Gateway Deletion From the Web Console + Namespace GC"
echo ""

if [[ "$E2E_SKIP_CLEANUP" == "1" ]]; then
  dim "  Skipped (E2E_SKIP_CLEANUP=1): keeping ${GW_NAME} (${GW_ID})"
elif [[ -z "$GW_ID" ]]; then
  fail_test "Cannot delete: gateway id is unknown"
else
  ab_use "$ADMIN_SESSION"
  DELETED_UI=0
  if ab_open "$(gateway_detail_url)"; then
    ACTIONS_REF=$(ab_wait_ref button '^Actions$' "$PAGE_TIMEOUT_S" || true)
    if [[ -n "$ACTIONS_REF" ]] && show_cmd "agent-browser click ${ACTIONS_REF}  # Actions" && ab_ok click "$ACTIONS_REF"; then
      ITEM_REF=$(ab_wait_ref menuitem '^Delete gateway$' 10 || true)
      if [[ -n "$ITEM_REF" ]] && show_cmd "agent-browser click ${ITEM_REF}  # Delete gateway" && ab_ok click "$ITEM_REF" \
          && ab_wait_text "Delete ${GW_NAME}?"; then
        pass "Delete dialog 'Delete ${GW_NAME}?' opened"
        CONFIRM_REF=$(ab_ref button '^Delete gateway$' '[role=dialog]' || true)
        if [[ -n "$CONFIRM_REF" ]] && show_cmd "agent-browser click ${CONFIRM_REF}  # confirm Delete gateway" && ab_ok click "$CONFIRM_REF" \
            && ab_wait_fn "document.body.innerText.includes($(ab_js_string "Gateway ${GW_NAME} deleted")) || !/^\\/gateways\\/[^/]+\$/.test(location.pathname)" "$E2E_BROWSER_PAGE_TIMEOUT_MS"; then
          pass "UI confirmed the deletion of ${GW_NAME}"
          DELETED_UI=1
        fi
      fi
    fi
  fi
  [[ "$DELETED_UI" == "1" ]] || ab_fail "Could not delete ${GW_NAME} through the detail page actions"
  ab_shot "08-gateway-deleted"

  show_cmd "api_curl ${API_HOST}/api/hypershell/v1/gateways/${GW_ID}  # expect 404"
  DEL_HTTP=""
  DEL_DEADLINE=$(($(date +%s) + 60))
  while [[ $(date +%s) -lt $DEL_DEADLINE ]]; do
    acquire_oidc_token 2>/dev/null || true
    DEL_HTTP=$(api_curl -o /dev/null -w '%{http_code}' "${API_HOST}/api/hypershell/v1/gateways/${GW_ID}" 2>/dev/null || true)
    [[ "$DEL_HTTP" == "404" ]] && break
    sleep 3
  done
  if [[ "$DEL_HTTP" == "404" ]]; then
    pass "API returns 404 for the deleted gateway"
    GW_ID=""
  else
    fail_test "API still returns HTTP ${DEL_HTTP:-none} for ${GW_ID} after the UI delete"
  fi

  if [[ -z "$GW_ID" && -n "$GW_NAMESPACE" ]]; then
    dim "  Waiting for namespace ${GW_NAMESPACE} to be garbage collected (up to ${E2E_GC_TIMEOUT}s)..."
    show_cmd "$CLI get namespace ${GW_NAMESPACE} (expect NotFound)"
    NS_GONE=false
    GC_DEADLINE=$(($(date +%s) + E2E_GC_TIMEOUT))
    while [[ $(date +%s) -lt $GC_DEADLINE ]]; do
      if ! $CLI get namespace "$GW_NAMESPACE" --request-timeout=10s &>/dev/null; then
        NS_GONE=true
        break
      fi
      sleep 5
    done
    if [[ "$NS_GONE" == "true" ]]; then
      pass "Gateway namespace garbage collected: ${GW_NAMESPACE}"
    else
      fail_test "Namespace ${GW_NAMESPACE} not garbage collected after ${E2E_GC_TIMEOUT}s"
      e2e_dump_namespace_gc_logs "${E2E_HS_NAMESPACE}" "$CLI"
    fi
  fi
fi
sep

# ── 9. Accessibility report + logout ─────────────────────────────────────

echo ""
e2e_area "9. Accessibility Report + Logout"
echo ""

if ! e2e_step long; then
  dim "  Skipped (E2E_MODE=${E2E_MODE})"
else
  ab_use "$ADMIN_SESSION"
  if ab_open "${CONSOLE_HOST}/" && ab_wait_text "OpenShell Gateways" "$E2E_BROWSER_PAGE_TIMEOUT_MS"; then
    mkdir -p "$E2E_CONSOLE_ARTIFACT_DIR"
    A11Y_FILE="${E2E_CONSOLE_ARTIFACT_DIR}/a11y-gateway-list.json"
    show_cmd "agent-browser a11y --tags wcag2a,wcag2aa --json > ${A11Y_FILE}"
    _ab a11y --tags wcag2a,wcag2aa --json > "$A11Y_FILE" 2>/dev/null || true
    A11Y_VIOLATIONS=$(python3 -c '
import json, sys
try:
    d = json.load(open(sys.argv[1]))
    print((d.get("data") or {}).get("counts", {}).get("violations", "?"))
except Exception:
    print("?")
' "$A11Y_FILE")
    dim "  a11y (report only, not gating): ${A11Y_VIOLATIONS} WCAG 2 A/AA violation rule(s) on the gateway list; see ${A11Y_FILE}"

    USER_MENU_REF=$(ab_ref button "^$(ab_regex_escape "${SESSION_DISPLAY:-$E2E_OIDC_USERNAME}")\$" || true)
    LOGOUT_REF=""
    if [[ -n "$USER_MENU_REF" ]] && show_cmd "agent-browser click ${USER_MENU_REF}  # user menu" && ab_ok click "$USER_MENU_REF"; then
      LOGOUT_REF=$(ab_wait_ref menuitem '^Log out$' 10 || true)
    fi
    if [[ -n "$LOGOUT_REF" ]] && show_cmd "agent-browser click ${LOGOUT_REF}  # Log out" && ab_ok click "$LOGOUT_REF" \
        && ab_wait_fn "location.host !== $(ab_js_string "$CONSOLE_HOSTNAME") || location.pathname !== '/'" "$E2E_BROWSER_PAGE_TIMEOUT_MS"; then
      pass "Clicked 'Log out' in the user menu"
    else
      ab_fail "Could not log out through the user menu"
    fi
    sleep 2
    SESSION_AFTER=""
    if ab_open "${CONSOLE_HOST}/auth/session"; then
      SESSION_AFTER=$(ab_eval "document.body.innerText" 2>/dev/null | e2e_console_session_fields | cut -f1)
    fi
    if [[ "$SESSION_AFTER" == "false" ]]; then
      pass "BFF /auth/session reports authenticated: false after logout"
    else
      ab_fail "BFF /auth/session after logout: authenticated=${SESSION_AFTER:-unknown}"
    fi
    if ab_open "${CONSOLE_HOST}/" && ab_wait_url "https://${ISSUER_HOSTNAME}/**" "$E2E_BROWSER_PAGE_TIMEOUT_MS"; then
      pass "Loading / redirects to Keycloak again"
    else
      ab_fail "Loading / after logout did not redirect to Keycloak"
    fi
  else
    ab_fail "Could not reopen the gateway list for the accessibility report"
  fi
fi
sep

# ── results ──────────────────────────────────────────────────────────────

# Reached every planned area without a fatal abort; cleanup's EXIT trap prints
# the results.
E2E_COMPLETED=1

if [[ $E2E_FAIL -gt 0 ]]; then
  exit 1
fi
