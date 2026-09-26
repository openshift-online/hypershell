#!/usr/bin/env bash
# Unit tests for the pure helpers behind e2e-console.sh (browser-lib.sh).
# No cluster and no browser: fixtures stand in for agent-browser --json output.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "${SCRIPT_DIR}/lib.sh"
# shellcheck source=browser-lib.sh
source "${SCRIPT_DIR}/browser-lib.sh"

PASS=0
FAIL=0

assert_eq() {
  local want="$1" got="$2" label="$3"
  if [[ "$want" == "$got" ]]; then
    PASS=$((PASS + 1))
  else
    FAIL=$((FAIL + 1))
    printf 'FAIL: %s (want=%q got=%q)\n' "$label" "$want" "$got"
  fi
}

assert_ok() {
  local label="$1"
  shift
  if "$@"; then
    PASS=$((PASS + 1))
  else
    FAIL=$((FAIL + 1))
    printf 'FAIL: %s (expected success)\n' "$label"
  fi
}

assert_not_ok() {
  local label="$1"
  shift
  if "$@"; then
    FAIL=$((FAIL + 1))
    printf 'FAIL: %s (expected failure)\n' "$label"
  else
    PASS=$((PASS + 1))
  fi
}

# --- ab_ref_from_snapshot ---
# Shape captured from agent-browser 0.37 `snapshot -i --json`: refs is a map,
# document order lives in the snapshot text (refs are not numbered in order).
SNAPSHOT='{"success":true,"data":{"origin":"https://console.hypershell.localhost/gateways/x","refs":{"e1":{"name":"Skip to content","role":"link"},"e3":{"name":"Actions for dev-gateway","role":"button"},"e5":{"name":"Actions","role":"button"},"e7":{"name":"Open console for gw-1 in a new tab","role":"link"},"e9":{"name":"Provision gateway","role":"button"},"e12":{"name":"local-kind Provider: kind; region: kind-local","role":"option"},"e10":{"name":"Actions for gw-1","role":"button"}},"snapshot":"- link \"Skip to content\" [ref=e1]\n- button \"Actions for gw-1\" [expanded=false, ref=e10]\n- button \"Actions\" [expanded=false, ref=e5]\n- button \"Actions for dev-gateway\" [ref=e3]\n- link \"Open console for gw-1 in a new tab\" [ref=e7]\n- button \"Provision gateway\" [ref=e9]\n- option \"local-kind Provider: kind; region: kind-local\" [ref=e12]"},"error":null}'

assert_eq '@e5' "$(ab_ref_from_snapshot button '^Actions$' <<< "$SNAPSHOT")" 'exact-name regex picks the detail Actions toggle'
assert_eq '@e10' "$(ab_ref_from_snapshot button '^actions for' <<< "$SNAPSHOT")" 'first match in document order, case-insensitive'
assert_eq '@e7' "$(ab_ref_from_snapshot LINK 'open console for gw-1' <<< "$SNAPSHOT")" 'role match ignores case'
assert_eq '@e12' "$(ab_ref_from_snapshot option "^$(ab_regex_escape local-kind)( |$)" <<< "$SNAPSHOT")" 'cluster option by leading name'
assert_eq '' "$(ab_ref_from_snapshot link '^Provision gateway$' <<< "$SNAPSHOT")" 'role must match, not just name'
assert_eq '' "$(ab_ref_from_snapshot button '^Delete gateway$' <<< "$SNAPSHOT")" 'absent element prints nothing'
assert_eq '' "$(ab_ref_from_snapshot button 'x' <<< 'not json')" 'unparseable snapshot prints nothing'

# --- _ab_json_get ---
assert_eq 'https://example.test/' "$(_ab_json_get url <<< '{"success":true,"data":{"url":"https://example.test/"},"error":null}')" 'string field printed raw'
assert_eq '{"a": 1}' "$(_ab_json_get result <<< '{"success":true,"data":{"result":{"a":1}}}')" 'object field printed as JSON'
assert_not_ok 'failed envelope exits non-zero' _ab_json_get url 2>/dev/null <<< '{"success":false,"data":null,"error":"Element not found"}'
assert_not_ok 'missing key exits non-zero' _ab_json_get url <<< '{"success":true,"data":{}}'

# --- ab_evidence_tag ---
assert_eq '5-Console-whoami-want-admin-got-x-y' "$(ab_evidence_tag '5-Console whoami: want admin, got x/y')" 'spaces, colons, slashes collapsed'
assert_eq 'failure' "$(ab_evidence_tag '///')" 'nothing usable falls back to failure'
TAG="$(ab_evidence_tag "$(printf 'a%.0s' {1..100})")"
assert_eq '60' "${#TAG}" 'tag is capped at 60 characters'
assert_eq '' "$(tr -d 'A-Za-z0-9-' <<< "$(ab_evidence_tag 'a b/c\d"e'\''f')")" 'tag has only safe characters'

# --- e2e_console_version_matches ---
assert_ok 'identical raw version' e2e_console_version_matches 'Version 0.0.116-rhaiv.6' '0.0.116-rhaiv.6'
assert_ok 'v-prefixed base in UI' e2e_console_version_matches 'Gateway version v0.0.116' '0.0.116-rhaiv.6'
assert_ok 'multi-line card text' e2e_console_version_matches $'Version\n0.0.116-rh1\nkubernetes' 'v0.0.116'
assert_not_ok 'different patch version' e2e_console_version_matches 'Version 0.0.115' '0.0.116-rhaiv.6'
assert_not_ok 'prefix is not a match' e2e_console_version_matches 'Version 0.0.11' '0.0.110'
assert_not_ok 'no version in UI text' e2e_console_version_matches 'Version unknown' '0.0.116'
assert_not_ok 'empty API version' e2e_console_version_matches 'Version 0.0.116' ''

# --- e2e_console_validate_mode ---
assert_ok 'short is valid' env E2E_MODE=short bash -c "source '${SCRIPT_DIR}/lib.sh'; source '${SCRIPT_DIR}/browser-lib.sh'; e2e_console_validate_mode"
assert_ok 'long is valid' env E2E_MODE=long bash -c "source '${SCRIPT_DIR}/lib.sh'; source '${SCRIPT_DIR}/browser-lib.sh'; e2e_console_validate_mode"
assert_not_ok 'perf is rejected' env E2E_MODE=perf bash -c "source '${SCRIPT_DIR}/lib.sh'; source '${SCRIPT_DIR}/browser-lib.sh'; e2e_console_validate_mode 2>/dev/null >/dev/null"
assert_not_ok 'unknown mode is rejected' env E2E_MODE=quick bash -c "source '${SCRIPT_DIR}/lib.sh'; source '${SCRIPT_DIR}/browser-lib.sh'; e2e_console_validate_mode 2>/dev/null >/dev/null"
PERF_MSG="$(E2E_MODE=perf e2e_console_validate_mode 2>&1 || true)"
assert_ok 'rejection names the valid modes' grep -q "valid modes are 'short' and 'long'" <<< "$PERF_MSG"

# --- JSON field helpers ---
assert_eq $'true\tadmin\tAdmin User' "$(e2e_console_session_fields <<< '{"authenticated":true,"roles":["platform:admin"],"user":{"preferred_username":"admin","name":"Admin User"}}')" 'BFF session fields'
assert_eq $'false\t\t' "$(e2e_console_session_fields <<< '{"authenticated":false}')" 'unauthenticated session'
assert_eq $'false\t\t' "$(e2e_console_session_fields <<< 'upstream connect error')" 'non-JSON session body is unauthenticated'
assert_eq $'admin\topenshell-user,openshell-admin' "$(e2e_console_whoami_fields <<< '{"subject":"s","displayName":"admin","roles":["openshell-user","openshell-admin"]}')" 'console whoami fields'
assert_eq $'\t' "$(e2e_console_whoami_fields <<< '{"error":"forbidden"}')" 'whoami error body has no roles'
assert_ok 'role present in list' e2e_console_list_has 'openshell-user,openshell-admin' openshell-admin
assert_not_ok 'role prefix is not membership' e2e_console_list_has 'openshell-administrator' openshell-admin
assert_eq 'https://console-openshell-ab.gw.localhost' "$(e2e_json_field console_address <<< '{"console_address":"https://console-openshell-ab.gw.localhost","active_sandbox_count":null}')" 'string field'
assert_eq '' "$(e2e_json_field active_sandbox_count <<< '{"active_sandbox_count":null}')" 'null field is empty'
assert_eq '0' "$(e2e_json_field active_sandbox_count <<< '{"active_sandbox_count":0}')" 'zero is kept'
assert_eq 'keycloak.hypershell.localhost' "$(ab_url_host 'https://keycloak.hypershell.localhost/realms/hypershell')" 'URL host'
assert_eq 'console.example.com:8443' "$(ab_url_host 'https://console.example.com:8443/')" 'URL host keeps a non-default port'
assert_eq "\"'x'\"" "$(ab_js_string "'x'")" 'JS string literal'
assert_eq '"a\"b"' "$(ab_js_string 'a"b')" 'JS string literal escapes quotes'

# --- Pinned version is read from dependency-age-tools.json ---
PIN="$(ab_pinned_version)"
assert_ok 'agent-browser pin is an exact semantic version' grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+$' <<< "$PIN"
assert_eq "npm install -g agent-browser@${PIN} && agent-browser install" "$(ab_install_hint)" 'install hint uses the pin'

# --- TLS: pins only certificates that chain to the given CA ---
if command -v openssl >/dev/null 2>&1; then
  TLS_DIR="$(mktemp -d)"
  trap 'rm -rf "$TLS_DIR"' EXIT
  openssl req -x509 -newkey rsa:2048 -nodes -keyout "$TLS_DIR/ca.key" -out "$TLS_DIR/ca.crt" \
    -days 1 -subj '/CN=e2e-test-ca' >/dev/null 2>&1
  openssl req -newkey rsa:2048 -nodes -keyout "$TLS_DIR/leaf.key" -out "$TLS_DIR/leaf.csr" \
    -subj '/CN=console.test' >/dev/null 2>&1
  openssl x509 -req -in "$TLS_DIR/leaf.csr" -CA "$TLS_DIR/ca.crt" -CAkey "$TLS_DIR/ca.key" \
    -CAcreateserial -out "$TLS_DIR/leaf.crt" -days 1 >/dev/null 2>&1
  openssl req -x509 -newkey rsa:2048 -nodes -keyout "$TLS_DIR/other.key" -out "$TLS_DIR/other.crt" \
    -days 1 -subj '/CN=console.test' >/dev/null 2>&1

  LEAF_PIN="$(ab_spki_pin < "$TLS_DIR/leaf.crt")"
  assert_ok 'SPKI pin is base64 of a sha256' grep -Eq '^[A-Za-z0-9+/]{43}=$' <<< "$LEAF_PIN"

  # Stand in for the network fetch: serve fixture certificates per host.
  _ab_fetch_leaf() {
    case "$1" in
      console.test) cat "$TLS_DIR/leaf.crt" ;;
      rogue.test) cat "$TLS_DIR/other.crt" ;;
      *) return 1 ;;
    esac
  }
  AB_WORK_DIR="$TLS_DIR/work"
  mkdir -p "$AB_WORK_DIR"
  assert_eq "$LEAF_PIN" "$(ab_verified_pin "$TLS_DIR/ca.crt" console.test)" 'CA-verified leaf is pinned'
  assert_not_ok 'leaf not signed by the CA is refused' ab_verified_pin "$TLS_DIR/ca.crt" rogue.test 2>/dev/null
  assert_not_ok 'unreachable host is refused' ab_verified_pin "$TLS_DIR/ca.crt" missing.test 2>/dev/null

  AGENT_BROWSER_EXECUTABLE_PATH="/opt/chromium/chrome"
  E2E_BROWSER_NO_SANDBOX=0
  E2E_BROWSER_INSECURE=0
  ab_tls_setup "$TLS_DIR/ca.crt" console.test '?missing.test' >/dev/null
  assert_eq 'spki-pin' "$AB_TLS_MODE" 'CA bundle selects SPKI pinning'
  assert_eq "$LEAF_PIN" "$AB_PINS" 'optional host that fails is skipped'
  assert_eq '--executable-path' "${AB_LAUNCH_ARGS[0]}" 'pins reach Chromium through a wrapper'
  WRAPPER="${AB_LAUNCH_ARGS[1]}"
  assert_ok 'wrapper passes the full pin list and forwards arguments' \
    grep -qF "exec /opt/chromium/chrome --ignore-certificate-errors-spki-list=${LEAF_PIN} \"\$@\"" "$WRAPPER"
  assert_not_ok 'required host that fails aborts TLS setup' ab_tls_setup "$TLS_DIR/ca.crt" console.test rogue.test 2>/dev/null
  assert_ok 'trusted host check uses the pins' ab_host_trusted console.test
  assert_not_ok 'untrusted host check fails' ab_host_trusted rogue.test

  E2E_BROWSER_INSECURE=1
  ab_tls_setup "$TLS_DIR/ca.crt" rogue.test >/dev/null
  assert_eq 'insecure' "$AB_TLS_MODE" 'E2E_BROWSER_INSECURE selects insecure mode'
  assert_eq '--ignore-https-errors' "${AB_LAUNCH_ARGS[*]}" 'insecure mode passes --ignore-https-errors only'
  E2E_BROWSER_INSECURE=0
fi

echo "e2e_console_test: ${PASS} passed, ${FAIL} failed"
[[ "$FAIL" -eq 0 ]]
