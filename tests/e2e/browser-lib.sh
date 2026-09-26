#!/usr/bin/env bash
# browser-lib.sh - agent-browser (headless Chromium) helpers for e2e-console.sh.
#
# Sourced after lib.sh. Every function is prefixed ab_. Machine-readable
# agent-browser output (--json) is parsed with python3, never grep, and page
# interactions address elements by accessibility role/name, label, or test id
# (see e2e-console-browser-testing.spec.md "Selector catalogue").
#
# TLS trust (CON-E2E-02). agent-browser's --ca-cert works only with a local
# Chromium on Linux, and --args splits its value on commas, so neither can carry
# a trust decision portably. Instead the suite pins the SubjectPublicKeyInfo of
# each edge certificate the browser will meet, and only after `openssl verify`
# proves that certificate chains to the cluster CA the driver supplies
# (get_browser_ca_bundle). The pins reach Chromium through a small
# --executable-path wrapper that passes --ignore-certificate-errors-spki-list
# intact. Certificates that do not chain to that CA are never trusted.
# E2E_BROWSER_INSECURE=1 replaces all of this with --ignore-https-errors.

: "${AGENT_BROWSER_BIN:=agent-browser}"
: "${E2E_BROWSER_TIMEOUT_MS:=30000}"
# First load of the web console SPA can be slow on a CPU-limited Kind pod.
: "${E2E_BROWSER_PAGE_TIMEOUT_MS:=90000}"
: "${E2E_CONSOLE_ARTIFACT_DIR:=./e2e-console-artifacts}"
: "${E2E_BROWSER_INSECURE:=0}"
: "${E2E_BROWSER_HEADED:=0}"
# Constant for the whole run; see ab_session_start for why it must not vary.
export AGENT_BROWSER_DEFAULT_TIMEOUT="$E2E_BROWSER_TIMEOUT_MS"
# Chromium's sandbox needs unprivileged user namespaces, which CI Linux runners
# (Ubuntu 24.04 AppArmor) restrict. Disable it only there by default.
if [[ -z "${E2E_BROWSER_NO_SANDBOX:-}" ]]; then
  if [[ "$(uname -s)" == "Linux" && -n "${CI:-}" ]]; then
    E2E_BROWSER_NO_SANDBOX=1
  else
    E2E_BROWSER_NO_SANDBOX=0
  fi
fi

AB_SESSION=""        # active session name, set by ab_session_start / ab_use
AB_SESSIONS=()       # every session this run opened (closed by ab_session_close_all)
AB_LAUNCH_ARGS=()    # launch options for the first command of each session
AB_WORK_DIR=""       # private scratch dir (CA bundle, pinned-browser wrapper)
AB_PINS=""           # comma-separated base64 SPKI sha256 pins
AB_TLS_MODE=""       # spki-pin | system | insecure (for the banner)

# ab_pinned_version - the agent-browser npm version pinned in
# dependency-age-tools.json (the single source of truth, also used by CI).
ab_pinned_version() {
  python3 - "${_E2E_REPO_ROOT}/dependency-age-tools.json" <<'PY'
import json, sys
try:
    tools = json.load(open(sys.argv[1]))
except Exception:
    sys.exit(0)
for t in tools:
    if t.get("kind") == "npm" and t.get("name") == "agent-browser":
        print(t.get("version", ""))
        break
PY
}

ab_install_hint() {
  local pin
  pin="$(ab_pinned_version)"
  printf 'npm install -g agent-browser@%s && agent-browser install' "${pin:-<pin>}"
}

ab_work_dir() {
  if [[ -z "$AB_WORK_DIR" ]]; then
    AB_WORK_DIR="$(mktemp -d "${TMPDIR:-/tmp}/hypershell-e2e-browser.XXXXXX")"
  fi
  printf '%s' "$AB_WORK_DIR"
}

# --- Pure helpers (unit-tested in e2e_console_test.sh) ---

# _ab_json_get <key> - read an agent-browser --json envelope
# ({"success":bool,"data":{...},"error":...}) on stdin and print data.<key>:
# strings raw, anything else as JSON. Exit 1 (error on stderr) when the command
# failed or the key is absent.
_ab_json_get() {
  AB_KEY="$1" python3 -c '
import json, os, sys
try:
    d = json.load(sys.stdin)
except Exception:
    sys.stderr.write("unparseable agent-browser output\n")
    sys.exit(1)
if not isinstance(d, dict) or not d.get("success"):
    err = d.get("error") if isinstance(d, dict) else None
    sys.stderr.write("%s\n" % (err or "agent-browser command failed"))
    sys.exit(1)
v = (d.get("data") or {}).get(os.environ["AB_KEY"])
if v is None:
    sys.exit(1)
print(v if isinstance(v, str) else json.dumps(v))
'
}

# ab_ref_from_snapshot <role> <name-regex> - read `snapshot -i --json` on stdin
# and print the first ref (e.g. "@e12"), in document order, whose role equals
# <role> (case-insensitive) and whose accessible name matches <name-regex>
# (Python regex, case-insensitive, search). Prints nothing when absent.
ab_ref_from_snapshot() {
  AB_ROLE="$1" AB_NAME="$2" python3 -c '
import json, os, re, sys
try:
    d = json.load(sys.stdin)
except Exception:
    sys.exit(0)
data = (d.get("data") or {}) if isinstance(d, dict) else {}
refs = data.get("refs") or {}
text = data.get("snapshot") or ""
role = os.environ["AB_ROLE"].lower()
name = re.compile(os.environ["AB_NAME"], re.IGNORECASE)
def order(ref):
    pos = text.find("ref=%s]" % ref)
    if pos < 0:
        pos = text.find("ref=%s," % ref)
    return pos if pos >= 0 else len(text) + int(re.sub(r"\D", "", ref) or 0)
for ref in sorted(refs, key=order):
    node = refs[ref] or {}
    if str(node.get("role", "")).lower() == role and name.search(str(node.get("name", ""))):
        print("@" + ref)
        break
'
}

# ab_evidence_tag <text> - reduce a failure message to a safe artifact file stem:
# [A-Za-z0-9-] only, runs of anything else collapsed to "-", at most 60 chars.
ab_evidence_tag() {
  local tag
  tag="$(printf '%s' "$1" | tr -c 'A-Za-z0-9' '-' | tr -s '-' | cut -c1-60)"
  tag="${tag#-}"
  tag="${tag%-}"
  printf '%s' "${tag:-failure}"
}

# ab_js_string <text> - print <text> as a JavaScript string literal.
ab_js_string() {
  python3 -c 'import json, sys; print(json.dumps(sys.argv[1]))' "$1"
}

# ab_spki_pin - read a PEM certificate on stdin and print the base64 sha256 of
# its SubjectPublicKeyInfo (the format --ignore-certificate-errors-spki-list uses).
ab_spki_pin() {
  openssl x509 -pubkey -noout 2>/dev/null \
    | openssl pkey -pubin -outform der 2>/dev/null \
    | openssl dgst -sha256 -binary \
    | openssl enc -base64
}

# openshell_installer_version <version-token> - print the installer base
# version (major.minor.patch, no leading 'v', no trailing qualifier) for a
# version string, e.g. "v0.0.116-rhaiv.6" -> "0.0.116". Fails when <version-token>
# has no leading major.minor.patch.
openshell_installer_version() {
  local v="${1#v}"
  [[ "$v" =~ ^[0-9]+\.[0-9]+\.[0-9]+ ]] || return 1
  printf '%s' "${BASH_REMATCH[0]}"
}

# e2e_console_version_matches <ui-text> <api-gateway-version> - succeed when a
# version token rendered by the OpenShell console has the same installer base
# version (openshell_installer_version) as the API gateway_version, e.g.
# "Version 0.0.116-rhaiv.6" vs "0.0.116-rhaiv.6", or "v0.0.116" vs "0.0.116-rh1".
# Compares whole tokens so 0.0.11 never matches 0.0.110.
e2e_console_version_matches() {
  local ui_text="$1" api_version="$2" want token got
  want="$(openshell_installer_version "$api_version")" || return 1
  while IFS= read -r token; do
    [[ -z "$token" ]] && continue
    got="$(openshell_installer_version "$token")" || continue
    [[ "$got" == "$want" ]] && return 0
  done < <(printf '%s\n' "$ui_text" | grep -oE 'v?[0-9]+\.[0-9]+\.[0-9]+(-[A-Za-z0-9.]+)?' || true)
  return 1
}

# e2e_console_validate_mode - the console suite supports short and long only;
# the perf harness does not drive a browser.
e2e_console_validate_mode() {
  case "${E2E_MODE}" in
    short|long) return 0 ;;
    *)
      red "ERROR: e2e-console.sh valid modes are 'short' and 'long' (got '${E2E_MODE}'); perf does not drive a browser"
      return 1
      ;;
  esac
}

# ab_url_host <url> - print the host (and non-default port) of a URL.
ab_url_host() {
  python3 -c 'import sys; from urllib.parse import urlsplit; print(urlsplit(sys.argv[1]).netloc)' "$1"
}

# ab_regex_escape <text> - escape <text> for use inside a name regex.
ab_regex_escape() {
  python3 -c 'import re, sys; print(re.escape(sys.argv[1]))' "$1"
}

# e2e_json_field <key> - print a top-level field of a JSON object on stdin
# (strings raw, lists comma-joined, null/absent as empty).
e2e_json_field() {
  WANT_KEY="$1" python3 -c '
import json, os, sys
try:
    d = json.load(sys.stdin)
except Exception:
    sys.exit(0)
v = d.get(os.environ["WANT_KEY"]) if isinstance(d, dict) else None
if v is None:
    print("")
elif isinstance(v, list):
    print(",".join(str(x) for x in v))
elif isinstance(v, bool):
    print("true" if v else "false")
else:
    print(v)
'
}

# e2e_console_session_fields - read the BFF /auth/session JSON on stdin and
# print "<authenticated true|false>\t<preferred_username>\t<display name>".
e2e_console_session_fields() {
  python3 -c '
import json, sys
try:
    d = json.load(sys.stdin)
except Exception:
    d = {}
if not isinstance(d, dict):
    d = {}
user = d.get("user") or {}
print("%s\t%s\t%s" % (
    "true" if d.get("authenticated") is True else "false",
    user.get("preferred_username", "") or "",
    user.get("name", "") or "",
))
'
}

# e2e_console_whoami_fields - read the OpenShell console /api/v1/auth/whoami
# JSON on stdin and print "<displayName>\t<comma-separated roles>".
e2e_console_whoami_fields() {
  python3 -c '
import json, sys
try:
    d = json.load(sys.stdin)
except Exception:
    d = {}
if not isinstance(d, dict):
    d = {}
roles = d.get("roles") or []
if not isinstance(roles, list):
    roles = []
print("%s\t%s" % (d.get("displayName", "") or "", ",".join(str(r) for r in roles)))
'
}

# e2e_console_list_has <csv> <item> - return 0 when <item> is in the CSV list.
e2e_console_list_has() {
  [[ ",$1," == *",$2,"* ]]
}

# --- TLS setup ---

# _ab_fetch_leaf <host> - print the PEM leaf certificate served for <host>:443
# (SNI = host). Prefers IPv4: Kind's *.localhost names resolve to ::1 too, but
# the cloud-provider-kind envoy only listens on IPv4.
_ab_fetch_leaf() {
  local host="$1" pem
  pem=$(openssl s_client -4 -connect "${host}:443" -servername "$host" </dev/null 2>/dev/null \
    | openssl x509 2>/dev/null || true)
  if [[ -z "$pem" ]]; then
    pem=$(openssl s_client -connect "${host}:443" -servername "$host" </dev/null 2>/dev/null \
      | openssl x509 2>/dev/null || true)
  fi
  [[ -n "$pem" ]] || return 1
  printf '%s\n' "$pem"
}

# ab_verified_pin <ca-file> <host> - print the SPKI pin of the certificate
# <host> serves, only when that certificate verifies against <ca-file>.
ab_verified_pin() {
  local ca="$1" host="$2" dir leaf
  dir="$(ab_work_dir)"
  leaf="${dir}/leaf-${host}.pem"
  if ! _ab_fetch_leaf "$host" > "$leaf"; then
    red "  Could not read the TLS certificate served for ${host}:443" >&2
    return 1
  fi
  if ! openssl verify -CAfile "$ca" "$leaf" >/dev/null 2>&1; then
    red "  Certificate served for ${host} does not chain to the cluster CA (${ca})" >&2
    return 1
  fi
  ab_spki_pin < "$leaf"
}

# ab_host_trusted <host> - return 0 when the browser session will trust <host>.
ab_host_trusted() {
  local host="$1" pin
  case "$AB_TLS_MODE" in
    insecure|system) return 0 ;;
  esac
  pin=$(_ab_fetch_leaf "$host" 2>/dev/null | ab_spki_pin 2>/dev/null || true)
  [[ -n "$pin" && ",${AB_PINS}," == *",${pin},"* ]]
}

# _ab_browser_executable - the Chromium the pinned wrapper execs:
# AGENT_BROWSER_EXECUTABLE_PATH, else the one `agent-browser doctor` reports.
_ab_browser_executable() {
  if [[ -n "${AGENT_BROWSER_EXECUTABLE_PATH:-}" ]]; then
    printf '%s' "$AGENT_BROWSER_EXECUTABLE_PATH"
    return 0
  fi
  "$AGENT_BROWSER_BIN" doctor --offline --quick --json 2>/dev/null | python3 -c '
import json, re, sys
try:
    d = json.load(sys.stdin)
except Exception:
    sys.exit(1)
for c in d.get("checks") or []:
    if c.get("id") == "chrome.installed" and c.get("status") == "pass":
        m = re.search(r" at (/.+)$", c.get("message", ""))
        if m:
            print(m.group(1))
            sys.exit(0)
sys.exit(1)
'
}

# ab_browser_available - return 0 when a Chromium is available to launch.
ab_browser_available() {
  local exe
  exe="$(_ab_browser_executable 2>/dev/null || true)"
  [[ -n "$exe" && -x "$exe" ]]
}

# _ab_write_wrapper <real-executable> <extra-arg...> - write an executable
# wrapper that runs the real browser with the extra arguments, and print its path.
_ab_write_wrapper() {
  local real="$1"
  shift
  local wrapper arg
  wrapper="$(ab_work_dir)/chromium-e2e"
  {
    printf '#!/bin/sh\n'
    printf '# Generated by tests/e2e/browser-lib.sh for one e2e-console run.\n'
    printf 'exec %q' "$real"
    for arg in "$@"; do
      printf ' %q' "$arg"
    done
    printf ' "$@"\n'
  } > "$wrapper"
  chmod 700 "$wrapper"
  printf '%s' "$wrapper"
}

# ab_tls_setup <ca-file|""> <host...> - decide how the browser trusts the
# cluster's edge certificates and set AB_LAUNCH_ARGS / AB_TLS_MODE / AB_PINS.
#   E2E_BROWSER_INSECURE=1 -> --ignore-https-errors (warned)
#   CA file given          -> SPKI pins of each host's CA-verified certificate
#   no CA file             -> require every host to pass system trust
# A host prefixed with "?" is optional: pinned when it verifies, skipped (with a
# note) otherwise. Used for a wildcard probe name whose real host is not yet known.
ab_tls_setup() {
  local ca="$1"
  shift
  local entry host pin extra=() wrapper exe
  AB_LAUNCH_ARGS=()
  AB_PINS=""
  # Create the scratch dir in this shell; helpers below run in $(...) subshells
  # and would otherwise each create (and leak) their own.
  ab_work_dir >/dev/null

  if [[ "$E2E_BROWSER_INSECURE" == "1" ]]; then
    AB_TLS_MODE="insecure"
    AB_LAUNCH_ARGS+=(--ignore-https-errors)
  elif [[ -z "$ca" ]]; then
    for entry in "$@"; do
      [[ "$entry" == \?* ]] && continue
      if ! curl -s -o /dev/null --connect-timeout 5 "https://${entry}/" 2>/dev/null; then
        red "  No cluster CA bundle is available and ${entry} is not trusted by the system store."
        red "  Provide one through the driver (get_browser_ca_bundle) or set E2E_BROWSER_INSECURE=1."
        return 1
      fi
    done
    AB_TLS_MODE="system"
  else
    for entry in "$@"; do
      host="${entry#\?}"
      if [[ "$entry" == \?* ]]; then
        if ! pin="$(ab_verified_pin "$ca" "$host" 2>/dev/null)"; then
          dim "  Optional TLS probe ${host} not pinned (no CA-verified certificate)"
          continue
        fi
      else
        pin="$(ab_verified_pin "$ca" "$host")" || return 1
      fi
      [[ ",${AB_PINS}," == *",${pin},"* ]] || AB_PINS="${AB_PINS:+${AB_PINS},}${pin}"
    done
    AB_TLS_MODE="spki-pin"
    extra+=("--ignore-certificate-errors-spki-list=${AB_PINS}")
  fi

  [[ "$E2E_BROWSER_NO_SANDBOX" == "1" ]] && extra+=(--no-sandbox)
  if ((${#extra[@]} > 0)); then
    exe="$(_ab_browser_executable)" || {
      red "  Could not locate a Chromium executable; set AGENT_BROWSER_EXECUTABLE_PATH or run: agent-browser install"
      return 1
    }
    wrapper="$(_ab_write_wrapper "$exe" "${extra[@]}")"
    AB_LAUNCH_ARGS+=(--executable-path "$wrapper")
    # agent-browser also reads AGENT_BROWSER_EXECUTABLE_PATH directly (the same
    # name used above to discover the real Chromium to wrap). Only the first
    # command of a session passes --executable-path (AB_LAUNCH_ARGS); every
    # later command in this session omits it, and if this env var is still
    # set, agent-browser treats that as a changed launch option and silently
    # relaunches Chromium without the pinned wrapper, dropping AB_PINS.
    unset AGENT_BROWSER_EXECUTABLE_PATH
  fi
  [[ "$E2E_BROWSER_HEADED" == "1" ]] && AB_LAUNCH_ARGS+=(--headed)
  return 0
}

# --- Session lifecycle ---

# _ab <args...> - run agent-browser in the active session.
_ab() {
  "$AGENT_BROWSER_BIN" --session "$AB_SESSION" "$@"
}

# ab <args...> - log the command (show_cmd) and run it in the active session.
ab() {
  show_cmd "agent-browser $*"
  _ab "$@"
}

# ab_json <args...> - run quietly with --json (machine output only).
ab_json() {
  _ab "$@" --json 2>/dev/null
}

# ab_ok <args...> - run quietly; succeed only when agent-browser reports success.
ab_ok() {
  ab_json "$@" | python3 -c '
import json, sys
try:
    d = json.load(sys.stdin)
except Exception:
    sys.exit(1)
if not d.get("success"):
    sys.stderr.write("    agent-browser: %s\n" % (d.get("error") or "failed"))
    sys.exit(1)
'
}

# ab_session_start <name> - launch a fresh browser session with the TLS launch
# options and make it active. Names must be unique per run (callers suffix a run
# id): a `close` immediately before `open` on the same name races the daemon
# shutdown and the open fails with "Failed to connect".
#
# Launch options are passed only here. Every later command must see the same
# AGENT_BROWSER_* environment: agent-browser hashes its launch configuration
# (environment included) and silently relaunches Chromium with the current
# command's options when it changes, which drops --executable-path and with it
# the TLS pins. Per-command timeouts therefore use --timeout, and
# AGENT_BROWSER_DEFAULT_TIMEOUT is exported once when this file is sourced.
ab_session_start() {
  AB_SESSION="$1"
  AB_SESSIONS+=("$AB_SESSION")
  show_cmd "agent-browser --session ${AB_SESSION} ${AB_LAUNCH_ARGS[*]} open about:blank"
  "$AGENT_BROWSER_BIN" --session "$AB_SESSION" "${AB_LAUNCH_ARGS[@]}" open about:blank --json 2>/dev/null \
    | _ab_json_get url >/dev/null || return 1
  _ab set viewport 1400 900 >/dev/null 2>&1 || true
}

# ab_use <name> - make an already-started session active.
ab_use() {
  AB_SESSION="$1"
}

ab_session_close() {
  "$AGENT_BROWSER_BIN" --session "${1:-$AB_SESSION}" close >/dev/null 2>&1 || true
}

ab_session_close_all() {
  local s
  for s in "${AB_SESSIONS[@]+"${AB_SESSIONS[@]}"}"; do
    ab_session_close "$s"
  done
  AB_SESSIONS=()
  if [[ -n "$AB_WORK_DIR" ]]; then
    rm -rf "$AB_WORK_DIR"
    AB_WORK_DIR=""
  fi
}

# --- Navigation, queries, and waits ---

# ab_open <url> - navigate (page-load timeout); succeed when it loaded.
ab_open() {
  show_cmd "agent-browser open $1"
  ab_ok open "$1" --timeout "$E2E_BROWSER_PAGE_TIMEOUT_MS"
}

ab_url() {
  ab_json get url | _ab_json_get url
}

# ab_ref <role> <name-regex> [css-scope] - first matching ref from a fresh
# interactive snapshot (optionally scoped to a CSS selector).
ab_ref() {
  if [[ -n "${3:-}" ]]; then
    ab_json snapshot -i -s "$3" | ab_ref_from_snapshot "$1" "$2"
  else
    ab_json snapshot -i | ab_ref_from_snapshot "$1" "$2"
  fi
}

# ab_wait_ref <role> <name-regex> <timeout-seconds> [css-scope] - poll
# snapshots until a matching ref appears; print it.
ab_wait_ref() {
  local role="$1" name="$2" timeout="$3" scope="${4:-}" ref deadline
  deadline=$(($(date +%s) + timeout))
  while [[ $(date +%s) -lt $deadline ]]; do
    ref="$(ab_ref "$role" "$name" "$scope" || true)"
    if [[ -n "$ref" ]]; then
      printf '%s' "$ref"
      return 0
    fi
    sleep 2
  done
  return 1
}

# ab_wait_text <text> [timeout-ms] - wait for a substring of the page text.
ab_wait_text() {
  show_cmd "agent-browser wait --text \"$1\""
  ab_ok wait --text "$1" --timeout "${2:-$E2E_BROWSER_TIMEOUT_MS}"
}

# ab_wait_url <glob> [timeout-ms]
ab_wait_url() {
  show_cmd "agent-browser wait --url \"$1\""
  ab_ok wait --url "$1" --timeout "${2:-$E2E_BROWSER_TIMEOUT_MS}"
}

# ab_wait_fn <js-expression> [timeout-ms]
ab_wait_fn() {
  ab_ok wait --fn "$1" --timeout "${2:-$E2E_BROWSER_TIMEOUT_MS}"
}

# ab_eval <js> - evaluate in the page; print the result.
ab_eval() {
  ab_json eval "$1" | _ab_json_get result
}

# ab_fetch_json <path> - same-origin GET with the page's cookies; print the body.
ab_fetch_json() {
  ab_eval "fetch($(ab_js_string "$1"),{credentials:'include'}).then(r=>r.text())"
}

# ab_fetch_status <path> - same-origin GET; print the HTTP status.
ab_fetch_status() {
  ab_eval "fetch($(ab_js_string "$1"),{credentials:'include',redirect:'manual'}).then(r=>String(r.status))"
}

# ab_testid_present <id> - return 0 when an element with data-testid=<id> exists.
ab_testid_present() {
  [[ "$(ab_eval "String(!!document.querySelector('[data-testid=\"$1\"]'))" 2>/dev/null)" == "true" ]]
}

# ab_testid_text <id> - print the text of the first data-testid=<id> element.
ab_testid_text() {
  ab_json get text "[data-testid=\"$1\"]" | _ab_json_get text
}

# ab_keycloak_login <username> <password> [timeout-ms] - complete the Keycloak
# login form (confirmed selectors #username / #password / #kc-login). Never logs
# the password.
ab_keycloak_login() {
  local user="$1" pass="$2" timeout="${3:-$E2E_BROWSER_PAGE_TIMEOUT_MS}"
  ab_ok wait "#username" --timeout "$timeout" || return 1
  show_cmd "agent-browser fill #username ${user}; fill #password ****; click #kc-login"
  ab_ok fill "#username" "$user" || return 1
  ab_ok fill "#password" "$pass" || return 1
  ab_ok click "#kc-login"
}

# ab_on_keycloak_form - return 0 when the current page shows the Keycloak form.
ab_on_keycloak_form() {
  [[ "$(ab_eval "String(!!document.querySelector('#kc-login'))" 2>/dev/null)" == "true" ]]
}

# --- Evidence (CON-E2E-11) ---

ab_shot() {
  mkdir -p "$E2E_CONSOLE_ARTIFACT_DIR"
  _ab screenshot "${E2E_CONSOLE_ARTIFACT_DIR}/$1.png" >/dev/null 2>&1 || true
}

# ab_fail_evidence <tag> - capture a full-page screenshot, URL, snapshot, and
# console/page errors. Never captures cookies, tokens, or storage state.
ab_fail_evidence() {
  local tag="$1"
  [[ -n "$AB_SESSION" ]] || return 0
  mkdir -p "$E2E_CONSOLE_ARTIFACT_DIR"
  _ab screenshot --full "${E2E_CONSOLE_ARTIFACT_DIR}/${tag}-full.png" >/dev/null 2>&1 || true
  ab_url > "${E2E_CONSOLE_ARTIFACT_DIR}/${tag}-url.txt" 2>/dev/null || true
  _ab snapshot -c > "${E2E_CONSOLE_ARTIFACT_DIR}/${tag}-snapshot.txt" 2>/dev/null || true
  _ab console --json > "${E2E_CONSOLE_ARTIFACT_DIR}/${tag}-console.json" 2>/dev/null || true
  _ab errors > "${E2E_CONSOLE_ARTIFACT_DIR}/${tag}-errors.txt" 2>/dev/null || true
}

# ab_fail <message> - fail_test plus browser evidence named after the message.
ab_fail() {
  fail_test "$1"
  ab_fail_evidence "$(ab_evidence_tag "${E2E_CURRENT_AREA%%.*}-$1")"
}
