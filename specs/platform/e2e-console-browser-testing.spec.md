# E2E Console Browser Testing

**Date:** 2026-09-22
**Status:** Implemented (see [Implementation Notes](#implementation-notes); full live run pending)
**Related:** `e2e-testing.spec.md` (driver contract, modes, CI e2e workflow, TLS trust rule);
             `openshell-gateway-console.spec.md` (per-gateway OpenShell console, oauth2-proxy, `consoleAddress`);
             `ephemeral-pr-environments.spec.md` (GitHub-brokered login, why the browser suite cannot run on PR environments);
             `local-development.spec.md` (Kind DNS, port 443 forwarding, cert-manager CA);
             `specs/standards/ui/verification.spec.md` (UI verification evidence)
**Upstream:** [agent-browser](https://github.com/vercel-labs/agent-browser) (headless browser CLI); [OpenShell Dashboard](https://github.com/Gkrumbach07/openshell-dashboard) (per-gateway console image)

---

## Purpose

`tests/e2e/e2e-openshell.sh` proves the platform through the API and the openshell
CLI. Nothing proves the two browser surfaces a human actually uses:

1. the **HyperShell web console** (`components/web-console/`, fleet-wide management
   UI at `https://console.hypershell.localhost` on Kind), and
2. the **OpenShell gateway console** (the per-gateway dashboard behind oauth2-proxy at
   `https://console-<ns>.<base-domain>`, published on the gateway as `console_address`).

This spec defines `tests/e2e/e2e-console.sh`: a bash suite in the same shape as
`e2e-openshell.sh` (same `lib.sh`, same infra drivers, same pass/fail tracking,
same `E2E_MODE` semantics, same cleanup discipline) that drives a real headless
Chromium through [agent-browser](https://github.com/vercel-labs/agent-browser) to
log in through Keycloak, provision a gateway from the web console, follow the
"Open gateway console" button into the OpenShell console, verify the console
sees the gateway, exercise a sandbox from the console, and delete the gateway from
the web console. Every UI observation is cross-checked against the HyperShell
API, which remains the source of truth.

The suite is a plan for another implementer. Everything below that is marked
**verify at implementation** was derived from reading source (this repo at HEAD
`630a5ed1`, upstream dashboard `main`) and must be confirmed against the deployed
artifacts before being relied on.

---

## Scope

In scope:

- A new bash entry point `tests/e2e/e2e-console.sh` plus a browser helper library
  `tests/e2e/browser-lib.sh` that wraps the `agent-browser` CLI.
- Kind as the primary target (local `make e2e-console`, and the Kind CI job). Manual
  OpenShift runs with the password grant.
- `short` and `long` modes, mirroring `e2e-openshell.sh`.
- CI wiring in `.github/workflows/e2e.yml`, a Makefile target, tool pinning, and
  spec/doc updates.

Out of scope:

- Replacing the Playwright suites in `components/web-console/e2e` (mocked dev server)
  and `components/web-console/e2e-live` (Jaeger trace join). Both stay.
- Running on GitHub-brokered pull-request OpenShift environments (see
  [OpenShift Constraints](#openshift-constraints)).
- Visual regression, performance, or full accessibility gating. An `a11y` audit is
  emitted as a report only.
- Modifying the OpenShell dashboard image.

---

## Architecture

```
tests/e2e/e2e-console.sh                (new; sources lib.sh, browser-lib.sh, drivers/<driver>.sh)
        |
        |  api_curl / acquire_oidc_token      (existing driver contract; API is source of truth)
        |  agent-browser --session <id> ...   (headless Chromium daemon; one session per identity)
        v
  +-------------------------+   Keycloak login   +-----------------------------+
  | HyperShell web console  | <----------------> | keycloak.hypershell.localhost |
  | console.hypershell.localhost (BFF + SPA)     | realm hypershell              |
  +-----------+-------------+                    +-----------------------------+
              | "Open gateway console" (href = gateway.console_address)
              v
  +-------------------------------------------+
  | OpenShell gateway console                 |   oauth2-proxy (client {name}-{id}-console)
  | console-<ns>.gw.localhost (Kind)          |   -> dashboard BFF -> gateway gRPC (mTLS)
  +-------------------------------------------+
```

Design rules carried over from `e2e-openshell.sh`:

- **Driver abstraction.** Infra-specific discovery goes through
  `tests/e2e/drivers/<driver>.sh`. The console suite requires the same
  `REQUIRED_FUNCTIONS` list as `e2e-openshell.sh` and adds one optional driver hook,
  `get_browser_ca_bundle` (see [Driver Additions](#driver-additions)).
- **API is the oracle.** Every state the UI claims (gateway created, Running, console
  address published, deleted) is re-read with `api_curl` before `pass` is recorded.
- **Own what you create.** The run provisions a uniquely named gateway
  (`E2E_GATEWAY_NAME`, default `e2e-console-gw-<random8hex>`), deletes it through
  the UI, and the `cleanup` trap deletes it through the API on any exit. `short`
  leaves nothing behind.
- **No insecure TLS bypass by default.** The browser trusts the cluster CA through
  `agent-browser --ca-cert` (Kind: the cert-manager CA that signs both
  `*.hypershell.localhost` and `*.gw.localhost`). `--ignore-https-errors` is an
  explicit opt-in (`E2E_BROWSER_INSECURE=1`) for clusters whose router CA cannot be
  extracted, and the run prints a warning when it is used.
- **Ref-based interaction.** Elements are addressed by `agent-browser snapshot -i --json`
  refs (`@eN`) or by `agent-browser find role|label|text|testid ...` semantic
  lookups, never by CSS class names. Selectors are catalogued once in
  [Selector Catalogue](#selector-catalogue).

### Why agent-browser

- Single static Rust binary plus a Chrome-for-Testing download; no Node.js
  runtime, no Playwright project, so a bash suite can drive it directly with the
  same `show_cmd` / `pass` / `fail_test` rhythm the existing suite uses.
- A background daemon keeps one browser per `--session`, so cookies and Keycloak
  SSO persist across hundreds of CLI invocations at shell speed.
- `--json` output for every query, `snapshot -i` accessibility refs, `find` by
  role/label/testid, `wait --text|--url|--fn`, `eval` for same-origin `fetch`, and
  `screenshot` for failure evidence.
- `--ca-cert` and `--ignore-https-errors` cover the self-signed Kind CA;
  `--session` isolation gives a second identity (developer) without touching the
  admin's cookies.

---

## Requirements

### Requirement CON-E2E-01: Suite Entry Point and Shape

The system SHALL provide `tests/e2e/e2e-console.sh`. It SHALL source
`tests/e2e/lib.sh`, select and validate the infra driver exactly as
`e2e-openshell.sh` does (`e2e_validate_mode`, `e2e_select_infra_driver`, the
`REQUIRED_FUNCTIONS` check), print the same banner style, use `e2e_area`,
`show_cmd`, `pass`, `fail_test`, and `print_results`, and install an `EXIT` trap
that closes every browser session, deletes the gateway it created (API `DELETE`
after a best-effort token refresh), and prints results.

It SHALL support `E2E_MODE=short|long` with the same tag semantics as
`e2e-openshell.sh` (`e2e_step`, `e2e_multi_identity`). `perf` SHALL be rejected
with a clear message (the performance harness does not drive a browser).

#### Scenario: Invocation parity

- GIVEN a Kind cluster from `make kind-up` (OIDC on, seeds applied)
- WHEN a developer runs `bash tests/e2e/e2e-console.sh` or `make e2e-console`
- THEN the driver is auto-detected from `KUBECONFIG`, the run prints a numbered
  banner of areas, and a results block with pass/fail counts on every exit path

#### Scenario: Perf mode rejected

- GIVEN `E2E_MODE=perf`
- WHEN `e2e-console.sh` starts
- THEN it SHALL exit non-zero stating that valid modes are `short` and `long`

### Requirement CON-E2E-02: Browser Tooling Preflight

Before any browser step the suite SHALL verify: `agent-browser` is on `PATH`
(`AGENT_BROWSER_BIN` override), `agent-browser doctor --offline --quick` passes
(or a browser executable is provided through `AGENT_BROWSER_EXECUTABLE_PATH`),
`python3` and `curl` exist, and both console origins answer on port 443 without
port remapping (a browser cannot use curl's `--connect-to`). On Kind the
preflight SHALL call `curl -sk --ipv4 https://console.hypershell.localhost/auth/session`
directly (not through `_driver_curl`, which silently remaps ports) and fail fast
with guidance when it does not answer (for example `KIND_NO_SUDO=true`, where
port 443 is not forwarded).

The suite SHALL obtain a CA bundle for the browser: the driver's
`get_browser_ca_bundle` when defined, else on Kind the `ca.crt` from
`hypershell-ca-secret` in `E2E_HS_NAMESPACE` (the same extraction
`e2e-openshell.sh` area 4 performs). When no bundle is available the suite SHALL
fail unless `E2E_BROWSER_INSECURE=1`, in which case it passes
`--ignore-https-errors` and prints a warning line in the banner.

#### Scenario: Missing tool

- GIVEN `agent-browser` is not installed
- WHEN the suite starts
- THEN it SHALL fail before creating any gateway, printing the pinned install
  command (`npm install -g agent-browser@<pin> && agent-browser install`)

#### Scenario: Port 443 not reachable

- GIVEN a Kind cluster started with `KIND_NO_SUDO=true`
- WHEN the preflight runs
- THEN it SHALL fail with a message naming the ephemeral cloud-provider-kind port
  and stating that the browser suite requires the port 443 forwarding from
  `make kind-up`

### Requirement CON-E2E-03: Web Console Login Through Keycloak

The suite SHALL open the web console root in a fresh named session, follow the
BFF redirect to Keycloak, submit the seeded admin credentials
(`E2E_OIDC_USERNAME` / `E2E_OIDC_PASSWORD`) on the Keycloak login form, and
assert it lands back on the console origin with the gateway list heading
visible. It SHALL then confirm the BFF session from inside the page
(`fetch('/auth/session')` via `agent-browser eval`) reports
`authenticated: true`, and confirm the API-side identity by acquiring a token
through `acquire_oidc_token` for the same user.

#### Scenario: Admin logs in

- GIVEN the seeded `admin` user
- WHEN the suite fills `#username`, `#password` and clicks `#kc-login`
- THEN the browser URL SHALL match the console host
- AND the heading "OpenShell Gateways" SHALL be visible
- AND `/auth/session` SHALL return `{"authenticated": true, ...}`

### Requirement CON-E2E-04: Gateway Provisioning From the Web Console

The suite SHALL navigate to `/gateways/new`, fill "Gateway name" with
`E2E_GATEWAY_NAME`, choose the seeded managed cluster in the "Cluster"
typeahead (`E2E_SEED_CLUSTER_NAME`, default `local-kind` on Kind), submit
"Provision gateway", and wait for navigation to `/gateways/<id>`. It SHALL then
look the gateway up by name through the API (`e2e_lookup_gateway_by_name`) and
record `GW_ID`, `GW_NAMESPACE`. It SHALL wait for phase `Running` using
`e2e_wait_gateway_running` (API) and, in parallel, assert the detail page shows
the Running status before the provisioning timeout (`E2E_PROVISION_TIMEOUT`).

The create form posts `database_id: ""` and `release_id: ""` with
`route: {"enabled": true}` (see `components/web-console/app/adapters/api/gateway-operations.ts`),
so placement is server-owned exactly as in the bash suite; the suite SHALL NOT try
to pick a release or database in the UI.

#### Scenario: Provision from the form

- GIVEN the admin session
- WHEN the form is submitted with a unique name and the seeded cluster
- THEN the API SHALL list one gateway with that name
- AND its `phase` SHALL become `Running` within `E2E_PROVISION_TIMEOUT`
- AND the detail page SHALL render the same name and a Running status

#### Scenario: Validation guard

- GIVEN the form is submitted with an empty name (long mode only)
- THEN the page SHALL show the required-field helper text and no gateway SHALL be
  created (API list by name is empty)

### Requirement CON-E2E-05: Console Address Published and Linked

The suite SHALL wait (bounded by `E2E_CONSOLE_READY_TIMEOUT`, default 300s) for
the API to publish `console_address` on the gateway, then assert the detail page's
"Open gateway console" control is an enabled link whose `href` equals that
`console_address`. While the address is empty the control is disabled with the
tooltip "Provisioning console..."; the suite SHALL tolerate that state during
the wait and SHALL fail if the control reports "Console unavailable for this
gateway" after the timeout.

#### Scenario: Button gated on readiness

- GIVEN a Running gateway whose console pod is not yet Ready
- THEN the button is disabled and `console_address` is empty
- WHEN the console Deployment becomes Ready
- THEN `console_address` is `https://console-<ns>.<base-domain>`
- AND the button's `href` equals it

### Requirement CON-E2E-06: OpenShell Console Login and Gateway Visibility

The suite SHALL navigate the same session to `console_address`. oauth2-proxy
starts an authorization-code flow against `{name}-{id}-console`; because the
realm SSO cookie already exists from CON-E2E-03 the login is normally silent, but
the suite SHALL handle both outcomes (if the Keycloak form appears, fill it again).
It SHALL assert the final URL is on the console host, then read
`/api/v1/auth/whoami` through `eval fetch` and assert the user is the admin and
the role set includes `openshell-admin` (the `ADMIN_ROLE` the control plane
configures). It SHALL open `/gateway` (admin-only route) and assert the
`gateway-status-card` and `gateway-version-card` test ids are present and that
the rendered version matches the API `gateway_version` (compare the base version
per `openshell_installer_version`).

#### Scenario: Token carries the gateway audience

- GIVEN the admin holds `gateway:owner` on the gateway
- WHEN the console loads
- THEN `whoami` SHALL report the admin role and the gateway page SHALL show the
  gateway's status and version without the error "Cannot reach the OpenShell gateway"

### Requirement CON-E2E-07: Sandbox Lifecycle From the OpenShell Console

In `long` mode the suite SHALL open `/workspaces`, enter the `default` workspace,
create a sandbox through the "Create sandbox" dialog, wait for the Kubernetes pod
`default--<sandbox>` in `GW_NAMESPACE` to be Running (`E2E_SANDBOX_TIMEOUT`),
assert the sandbox row shows a running status, assert the API
`active_sandbox_count` reaches 1 (`poll` pattern from `e2e-openshell.sh`), delete
the sandbox from the row actions, and assert the count returns to 0.

In `short` mode the suite SHALL only assert that the sandbox list renders (empty
state "No sandboxes" or a table) and that `active_sandbox_count` is 0 or absent.

The exact fields of the create dialog (name, image, workspace, policy) depend on the
pinned dashboard image and SHALL be discovered with `snapshot -i` at implementation
time; the suite SHALL prefer the dashboard's own test ids (`create-sandbox`,
`sandbox-table`, `sandbox-actions-kebab`) over text.

#### Scenario: Create and delete a sandbox from the console

- GIVEN the admin console session
- WHEN a sandbox named `e2e-console-sb-<random>` is created
- THEN a pod `default--e2e-console-sb-<random>` SHALL be Running in the gateway namespace
- AND `active_sandbox_count` SHALL become 1
- WHEN the sandbox is deleted from the console
- THEN the pod SHALL disappear and `active_sandbox_count` SHALL become 0

### Requirement CON-E2E-08: Developer RBAC Boundary (long only)

When `e2e_multi_identity` is true the suite SHALL open a second, isolated
agent-browser session as `E2E_DEV_USERNAME`, log in to the web console, assert the
gateway list loads (HTTP 200 through the BFF, possibly empty), and then navigate
directly to the admin's `console_address`. Because the developer holds no
RoleBinding on that gateway, the suite SHALL assert the console denies the
gateway: `whoami` reports no `openshell-admin`/`openshell-user` role for it, the
admin-only `/gateway` route redirects to `/workspaces`, and `/api/v1/gateway`
returns a non-2xx status. The concrete denial surface (403 body vs. error card)
SHALL be confirmed at implementation and encoded as one assertion, not several
guesses.

#### Scenario: No RoleBinding, no console

- GIVEN the developer session
- WHEN it opens the admin gateway's console
- THEN it SHALL NOT see the gateway status card
- AND `/api/v1/gateway` SHALL NOT return 200

### Requirement CON-E2E-09: Gateway Deletion From the Web Console and GC

The suite SHALL return to the gateway detail page in the admin session, open the
actions dropdown, choose "Delete gateway", confirm in the dialog titled
"Delete <name>?", and assert the toast "Gateway <name> deleted" (or navigation to
the list without the row). It SHALL then assert the API returns 404 for the
gateway id and, as `e2e-openshell.sh` area 11 does, wait up to `E2E_GC_TIMEOUT`
for `GW_NAMESPACE` to be garbage collected. On success it SHALL clear `GW_ID` so
the cleanup trap does not attempt a second delete.

#### Scenario: UI delete drives GC

- GIVEN the admin session on the detail page
- WHEN the delete dialog is confirmed
- THEN the API SHALL 404 for the gateway
- AND the namespace SHALL be gone within `E2E_GC_TIMEOUT`

### Requirement CON-E2E-10: Logout (long only)

The suite SHALL open the masthead user menu, click "Log out" (a real navigation
to `/auth/logout`), and assert the session ends: the next `/auth/session` read
returns `authenticated: false` and loading `/` redirects to Keycloak again.

### Requirement CON-E2E-11: Evidence and Diagnostics

Every area SHALL save at least one screenshot into `E2E_CONSOLE_ARTIFACT_DIR`
(default `./e2e-console-artifacts`), named `<area>-<step>.png`. On any
`fail_test` the helper SHALL additionally capture `screenshot --full`, the
current URL, `agent-browser console --json`, `agent-browser errors`, and
`snapshot -c` into the same directory. The suite SHALL never print cookies,
tokens, or `agent-browser state` output. CI SHALL upload the directory as an
artifact on failure and SHALL also dump the console Deployment, oauth2-proxy and
dashboard container logs, and the console HTTPRoute/Route status for the run's
gateway namespace.

### Requirement CON-E2E-12: CI Integration

`.github/workflows/e2e.yml` SHALL run the console suite in the Kind job after
the existing trace verification step, with the same `if: github.event_name != 'merge_group'`
gate and rationale (browser coverage is proven at PR time and re-verified on
push to `main`). It SHALL install the pinned agent-browser through npm using the
Node toolchain already set up for the Playwright step, reuse that step's cached
Chromium via `AGENT_BROWSER_EXECUTABLE_PATH` when present (falling back to
`agent-browser install`), run `E2E_MODE=short`, and wire the artifact upload from
CON-E2E-11. A Makefile target `e2e-console` SHALL exist. `dependency-age-tools.json`
SHALL pin the `agent-browser` npm version.

The OpenShift job SHALL NOT run the console suite (see
[OpenShift Constraints](#openshift-constraints)).

---

## Implementation Plan

### File layout

```
tests/e2e/
  e2e-console.sh            -- new: browser suite entry point (areas 1-9 below)
  browser-lib.sh            -- new: agent-browser wrappers (ab, ab_find, ab_wait_text, ab_shot, ab_fetch_json, ab_fail_evidence)
  e2e_console_test.sh       -- new: unit tests for pure helpers (auto-discovered by make ci-test)
  lib.sh                    -- unchanged (reused)
  drivers/kind.sh           -- add optional get_browser_ca_bundle
  drivers/openshift.sh      -- add optional get_browser_ca_bundle (router CA when extractable)
Makefile                    -- add e2e-console target and help line
.github/workflows/e2e.yml   -- add install + run + diagnostics + artifact steps (Kind job)
dependency-age-tools.json   -- add {"kind": "npm", "name": "agent-browser", "version": "<pin>"}
specs/platform/e2e-testing.spec.md -- file layout + env var table additions
CLAUDE.md                   -- Commands section: make e2e-console
```

`tests/e2e/**` is already registered under the `e2e` component in
`.github/component-paths.json`, so no detection changes are needed. Run
`make check` after edits; `check-dependency-age` validates the new npm pin.

### `tests/e2e/browser-lib.sh`

Sourced after `lib.sh`. All functions are prefixed `ab_`. Design:

```bash
: "${AGENT_BROWSER_BIN:=agent-browser}"
: "${E2E_BROWSER_TIMEOUT_MS:=15000}"        # per-action default timeout
: "${E2E_CONSOLE_ARTIFACT_DIR:=./e2e-console-artifacts}"
: "${E2E_BROWSER_INSECURE:=0}"
AB_SESSION=""                                 # set by ab_session_start
AB_CA_ARGS=()                                 # (--ca-cert <file>) or (--ignore-https-errors)

ab_session_start() {            # ab_session_start <name>
  AB_SESSION="$1"
  export AGENT_BROWSER_SESSION="$AB_SESSION"
  export AGENT_BROWSER_DEFAULT_TIMEOUT="$E2E_BROWSER_TIMEOUT_MS"
  "$AGENT_BROWSER_BIN" "${AB_CA_ARGS[@]}" open about:blank >/dev/null
  "$AGENT_BROWSER_BIN" set viewport 1400 900 >/dev/null
}
ab_session_close() { "$AGENT_BROWSER_BIN" close >/dev/null 2>&1 || true; }

ab() {                          # thin wrapper: logs the command via show_cmd, runs it
  show_cmd "agent-browser $*"
  "$AGENT_BROWSER_BIN" "$@"
}
ab_json() { "$AGENT_BROWSER_BIN" "$@" --json; }          # quiet, machine output

# Find a ref by role + accessible name (regex, case-insensitive) from `snapshot -i --json`.
# Prints the ref (e.g. "@e12") or nothing. Pure bash+python3; unit-tested.
ab_ref() { ab_json snapshot -i | AB_ROLE="$1" AB_NAME="$2" python3 "${_E2E_LIB_DIR}/browser-ref.py"; }

ab_wait_text()  { ab wait --text "$1"; }                 # substring
ab_wait_url()   { ab wait --url "$1"; }                  # glob, e.g. "**/gateways/*"
ab_url()        { ab_json get url | python3 -c 'import json,sys; print(json.load(sys.stdin)["data"]["url"])'; }
ab_fetch_json() {               # same-origin GET with the page's cookies; prints body
  ab_json eval "fetch('$1',{credentials:'include'}).then(r=>r.text())" \
    | python3 -c 'import json,sys; print(json.load(sys.stdin)["data"]["result"])'
}
ab_fetch_status() {             # same-origin GET; prints HTTP status
  ab_json eval "fetch('$1',{credentials:'include'}).then(r=>r.status)" \
    | python3 -c 'import json,sys; print(json.load(sys.stdin)["data"]["result"])'
}
ab_shot() { mkdir -p "$E2E_CONSOLE_ARTIFACT_DIR"; "$AGENT_BROWSER_BIN" screenshot "$E2E_CONSOLE_ARTIFACT_DIR/$1.png" >/dev/null 2>&1 || true; }
ab_fail_evidence() {            # called by a fail_test wrapper (ab_fail) on every failure
  local tag="$1"; mkdir -p "$E2E_CONSOLE_ARTIFACT_DIR"
  "$AGENT_BROWSER_BIN" screenshot --full "$E2E_CONSOLE_ARTIFACT_DIR/${tag}-full.png" >/dev/null 2>&1 || true
  ab_url > "$E2E_CONSOLE_ARTIFACT_DIR/${tag}-url.txt" 2>/dev/null || true
  "$AGENT_BROWSER_BIN" snapshot -c > "$E2E_CONSOLE_ARTIFACT_DIR/${tag}-snapshot.txt" 2>/dev/null || true
  "$AGENT_BROWSER_BIN" console --json > "$E2E_CONSOLE_ARTIFACT_DIR/${tag}-console.json" 2>/dev/null || true
  "$AGENT_BROWSER_BIN" errors > "$E2E_CONSOLE_ARTIFACT_DIR/${tag}-errors.txt" 2>/dev/null || true
}
ab_fail() { fail_test "$1"; ab_fail_evidence "$(echo "$1" | tr -c 'A-Za-z0-9' '-' | cut -c1-60)"; }
```

Notes for the implementer:

- **Verify at implementation** the exact JSON envelope of `--json` output
  (`{"success":true,"data":{...}}` is assumed above) and the ref field names in
  `snapshot -i --json`; adjust `browser-ref.py` (or an inline python heredoc, as
  `lib.sh` does) accordingly. Keep parsing in python3, never `grep` on the tree.
- Prefer `agent-browser find role button click --name "Provision gateway"` for
  single-step interactions; fall back to `ab_ref` when a `wait` on a ref is needed
  or the same element must be read after clicking.
- `--ca-cert` is a launch option; pass `AB_CA_ARGS` on the first `open` of each
  session. Confirm (`agent-browser --help`) whether it must be repeated; if the
  daemon persists it per session, later calls omit it.
- Two sessions never share a daemon state: developer uses `ab_session_start
  "hs-e2e-dev-$RUN_ID"` after the admin session, and the cleanup trap closes both
  (`AGENT_BROWSER_SESSION=<name> agent-browser close`).
- `TERM=dumb`/`NO_COLOR` from `lib.sh` apply; keep agent-browser output quiet
  (`--json`, redirect to files) so logs stay greppable.

### `tests/e2e/e2e-console.sh` skeleton

```bash
#!/usr/bin/env bash
# e2e-console.sh - browser end-to-end test of the HyperShell web console and the
# per-gateway OpenShell console, driven by agent-browser (headless Chromium).
# Same shape as e2e-openshell.sh: infra driver, lib.sh, E2E_MODE short|long.
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lib.sh"
source "${SCRIPT_DIR}/browser-lib.sh"

: "${E2E_GATEWAY_NAME:=e2e-console-gw-$(head -c4 /dev/urandom | od -An -tx1 | tr -d ' \n')}"  # override lib default prefix
: "${E2E_CONSOLE_READY_TIMEOUT:=300}"
: "${E2E_SEED_CLUSTER_NAME:=}"          # lib's e2e_fetch_seed_ids fills per driver
RUN_ID="$(head -c3 /dev/urandom | od -An -tx1 | tr -d ' \n')"

e2e_validate_mode; [[ "$E2E_MODE" == perf ]] && { red "ERROR: E2E_MODE must be short or long for the console suite"; exit 1; }
e2e_select_infra_driver
source "${SCRIPT_DIR}/drivers/${E2E_INFRA_DRIVER}.sh"
REQUIRED_FUNCTIONS=(discover_api_host discover_console_host acquire_oidc_token api_curl get_cli_binary get_cluster_domain)
# ... same declare -f loop as e2e-openshell.sh ...
CLI=$(get_cli_binary); GW_NAME="$E2E_GATEWAY_NAME"; GW_ID=""; GW_NAMESPACE=""; CONSOLE_URL=""

cleanup() {
  local exit_code=$?
  ab_session_close_all            # admin + developer sessions
  if [[ "$E2E_SKIP_CLEANUP" != "1" && -n "$GW_ID" ]]; then
    acquire_oidc_token 2>/dev/null || true
    api_curl -X DELETE "${API_HOST}/api/hypershell/v1/gateways/${GW_ID}" &>/dev/null || true
  fi
  print_results
}
trap cleanup EXIT

discover_api_host || exit 1;      API_HOST="$_DISCOVER_API_HOST"
discover_console_host || exit 1;  CONSOLE_HOST="$_DISCOVER_CONSOLE_HOST"
# banner (areas 1-9, driver, mode, hosts, users, timeouts, TLS mode)
```

Areas (numbered for the banner; `[short]` runs in both modes, `[long]` only in long):

| # | Area | Steps |
|---|------|-------|
| 1 | Preflight | `[short]` tool checks (CON-E2E-02); direct-443 reachability of `CONSOLE_HOST` and `E2E_OIDC_ISSUER`; CA bundle; `agent-browser doctor`; API token via `acquire_oidc_token`; seed ids via `e2e_ensure_seed_ids`; `ab_session_start "hs-e2e-admin-$RUN_ID"` |
| 2 | Web console login | `[short]` open `$CONSOLE_HOST/`, `wait --url "**keycloak**"` or detect `#username`; fill; click `#kc-login`; `wait --url "$CONSOLE_HOST/**"`; `wait --text "OpenShell Gateways"`; `ab_fetch_json /auth/session` -> `authenticated == true`; `ab_shot 02-gateway-list` |
| 3 | Provision gateway via UI | `[long]` empty-name validation guard; `[short]` open `/gateways/new`; `find label "Gateway name" fill "$GW_NAME"`; cluster typeahead (click "Select a cluster", type seed cluster name, pick option); `find role button click --name "Provision gateway"`; `wait --url "**/gateways/*"`; API lookup by name -> `GW_ID`, `GW_NAMESPACE`; `e2e_wait_gateway_running`; UI `wait --text "Running"` on detail; `ab_shot 03-gateway-running` |
| 4 | Console address + button | `[short]` poll API `console_address` up to `E2E_CONSOLE_READY_TIMEOUT` (refresh token each poll); reload detail page; `ab_ref link "Open console for $GW_NAME"`; `get attr @ref href` equals `console_address`; fail if tooltip "Console unavailable" |
| 5 | OpenShell console | `[short]` `open "$CONSOLE_URL"`; if `#username` visible then fill again; `wait --url "$CONSOLE_URL/**"`; `ab_fetch_json /api/v1/auth/whoami` -> user + admin role; `open "$CONSOLE_URL/gateway"`; `find testid gateway-status-card text`; `find testid gateway-version-card text` contains base of API `gateway_version`; `ab_shot 05-console-gateway` |
| 6 | Sandbox from console | `[short]` `open "$CONSOLE_URL/workspaces"`, enter `default` (or first) workspace, assert "No sandboxes" or `sandbox-table`; `[long]` create via `create-sandbox` dialog, pod `default--<name>` Running via `$CLI`, `active_sandbox_count` 1, row status running, delete via `sandbox-actions-kebab`, count 0 |
| 7 | Developer RBAC | `[long]` gated on `e2e_multi_identity`; second session; web console login as developer; list loads; open `$CONSOLE_URL` -> whoami lacks gateway roles; `/gateway` redirects to `/workspaces`; `ab_fetch_status /api/v1/gateway` != 200; close session |
| 8 | Delete gateway via UI | `[short]` back in admin session: detail page; `find role button click --name "Actions for $GW_NAME"`; click "Delete gateway"; dialog "Delete $GW_NAME?"; confirm "Delete gateway"; `wait --text "Gateway $GW_NAME deleted"`; API 404; namespace GC wait (`E2E_GC_TIMEOUT`); `GW_ID=""` |
| 9 | Logout + a11y | `[long]` user menu -> "Log out"; `/auth/session` false; `[long]` `agent-browser a11y --tags wcag2a,wcag2aa --json > $E2E_CONSOLE_ARTIFACT_DIR/a11y-gateway-list.json` on the gateway list (report only, never fails the run) |

Then `E2E_COMPLETED=1` and exit with `E2E_FAIL == 0 ? 0 : 1`, as the bash suite does.

### `tests/e2e/e2e_console_test.sh`

No cluster, no browser. Cover the pure helpers so `make ci-test` guards them:

- `browser-ref.py` (or the inline parser) returns the right ref for a
  fixture snapshot JSON containing several buttons/links, matches the accessible
  name case-insensitively and as a regex, and prints nothing when absent.
- `e2e_console_version_matches` (compare dashboard-rendered version text with
  API `gateway_version` through `openshell_installer_version`).
- Mode validation rejects `perf` and unknown values.
- `ab_fail` sanitises the evidence tag (no spaces or slashes).

### Driver additions

Add to both drivers (optional hook, checked with `declare -f`):

```bash
# get_browser_ca_bundle - print a PEM file path the headless browser should trust,
# or nothing when the cluster's edge certificates are publicly trusted or the CA
# cannot be extracted. Kind: cert-manager CA from hypershell-ca-secret (signs
# *.hypershell.localhost and *.gw.localhost). OpenShift: router CA when readable
# (openshift-ingress-operator/router-ca), else empty.
get_browser_ca_bundle() { ... }
```

Kind implementation reuses the area-4 extraction from `e2e-openshell.sh`
(`kubectl get secret hypershell-ca-secret -n $E2E_HS_NAMESPACE -o jsonpath='{.data.ca\.crt}' | base64 -d`)
into `$E2E_CONSOLE_ARTIFACT_DIR/../e2e-console-ca.crt` (or `mktemp`). Do not
modify `REQUIRED_FUNCTIONS` in `e2e-openshell.sh`.

### Makefile

```make
# Browser-driven e2e of the HyperShell web console and the per-gateway OpenShell
# console (agent-browser headless Chromium). Requires port 443 forwarding from
# `make kind-up` (not KIND_NO_SUDO) and `npm install -g agent-browser`.
.PHONY: e2e-console
e2e-console:
	@echo ""
	@echo "==> Running browser E2E (web console + OpenShell console)"
	@echo ""
	@E2E_PROVISION_TIMEOUT=300 E2E_SANDBOX_TIMEOUT=180 bash tests/e2e/e2e-console.sh
```

Add the help line next to `e2e` and `e2e-tracing`.

### CI (`.github/workflows/e2e.yml`, Kind job)

Insert after "Verify end-to-end traces reach Jaeger":

```yaml
      # Browser e2e of both consoles through agent-browser. Same PR/push-only gate
      # and rationale as the trace verification above; reuses the Node toolchain
      # and the cached Playwright Chromium so no second browser download is needed.
      - name: Install agent-browser
        if: github.event_name != 'merge_group'
        run: npm install -g agent-browser@<pin>   # keep in sync with dependency-age-tools.json
      - name: Run console browser e2e
        if: github.event_name != 'merge_group'
        env:
          E2E_INFRA_DRIVER: kind
          E2E_MODE: short
          E2E_PROVISION_TIMEOUT: "300"
          E2E_CONSOLE_READY_TIMEOUT: "300"
          E2E_CONSOLE_ARTIFACT_DIR: ${{ github.workspace }}/e2e-console-artifacts
          # Reuse the Chromium the Playwright step cached; empty -> agent-browser install.
          AGENT_BROWSER_EXECUTABLE_PATH: ${{ steps.chromium-path.outputs.path }}
          TERM: dumb
          NO_COLOR: "1"
        run: bash tests/e2e/e2e-console.sh
```

Add a small step before it that resolves the Playwright Chromium binary
(`find ~/.cache/ms-playwright -type f -name chrome -path '*chromium*' | head -1`)
into `steps.chromium-path.outputs.path`; when empty the suite runs
`agent-browser install` itself (preflight). **Verify at implementation** that
agent-browser accepts a Playwright-channel Chromium; otherwise cache
agent-browser's own download directory keyed on the pin, mirroring the existing
`Restore Chromium cache` step.

Extend "Collect diagnostics" with, for the run's gateway namespace
(`kubectl get ns -l hypershell.redhat.io/managed=true`): `deploy/openshell-console`
status, logs of both containers (`-c oauth2-proxy`, `-c dashboard`; confirm names in
`components/control-plane/internal/gateway/console.go`), the `openshell-console`
HTTPRoute status, and the web-console deployment logs already collected. Upload
`e2e-console-artifacts/` with `actions/upload-artifact` on failure.

Update the workflow header comment and `specs/platform/e2e-testing.spec.md`
(file layout, env var table, a pointer to this spec under CI E2E Workflow).

### Selector catalogue

All strings come from source at HEAD; they are the accessible names agent-browser
`find`/`snapshot` expose. i18n ids are given so a copy change can be traced.

**Keycloak login page** (confirmed by `components/web-console/e2e-live/tracing.live.spec.ts`)

| Element | Selector |
|---------|----------|
| Username | `#username` |
| Password | `#password` |
| Submit | `#kc-login` (fallback `button[type=submit]`) |

**HyperShell web console** (`packages/gateway-management-ui/src/messages.ts`, `components/web-console/app/i18n/messages.ts`)

| Element | Role / name | Source id |
|---------|-------------|-----------|
| Gateway list heading | heading "OpenShell Gateways" | `app.gateway.gateways` |
| Provision link/button (list) | button/link "Provision gateway" | `provisionGateway` |
| Create form | form aria-label "Provision gateway"; route `/gateways/new` | `route-contract.json: gatewayNew` |
| Name field | textbox labelled "Gateway name" (`#gateway-name`) | `gatewayName` |
| Cluster typeahead | combobox/menu toggle aria-label "Select a cluster"; group label "Cluster"; options "Hub cluster (default)" and managed cluster names | `selectCluster`, `cluster`, `hubDefault` |
| Submit | button "Provision gateway" (type=submit) | `provisionGateway` |
| Detail route | `/gateways/<id>` | `route-contract.json: gatewayDetail` |
| Open console | link aria-label "Open console for {gatewayName} in a new tab", text "Open gateway console"; disabled variant has tooltip "Provisioning console..." / "Console unavailable for this gateway" | `openGatewayConsoleFor`, `openGatewayConsole`, `provisioningGatewayConsole`, `unavailableGatewayConsole` |
| Row/detail actions | button aria-label "Actions for {gatewayName}" | `gatewayRowActions` |
| Delete item | menuitem "Delete gateway" | `deleteGateway` |
| Delete dialog | dialog titled "Delete {gatewayName}?", confirm button "Delete gateway", "Cancel" | `deleteGatewayTitle`, `deleteGateway`, `cancel` |
| Delete toast | text "Gateway {gatewayName} deleted" | `gatewayDeleted` |
| Refresh | button "Refresh gateways" | `refreshGateways` |
| User menu logout | menuitem "Log out" -> `/auth/logout` | `app.logout` |
| Session probe | `GET /auth/session` -> `{authenticated: bool}` | `components/web-console/bff/src/auth.ts` |

The console link opens in a new tab (`target="_blank"`). Do not click it;
read `href` and `open` it in the same tab so the session cookie jar is reused.

**OpenShell gateway console** (upstream `main`; **verify against the pinned image**
`quay.io/gkrumbach07/openshell-dashboard@sha256:c69c1f34...`, tag `sha-978bcb5`,
see `components/control-plane/internal/gateway/config.go`)

| Element | Selector / path |
|---------|-----------------|
| Whoami | `GET /api/v1/auth/whoami` (auth required); `GET /api/v1/auth/config` public; `GET /api/v1/healthz` |
| Gateway info API | `GET /api/v1/gateway` (admin) |
| Gateway page | route `/gateway` (admin-only; non-admin redirected to `/workspaces`); title "Gateway"; test ids `gateway-status-card`, `gateway-version-card`, `gateway-drivers-card`; error "Cannot reach the OpenShell gateway" |
| Workspaces | route `/workspaces`; test ids `workspace-table`, `workspace-link-<name>`, `create-workspace` |
| Sandboxes | route `/workspaces/<ws>` ; test ids `create-sandbox`, `create-sandbox-empty`, `sandbox-table`, `sandbox-actions-kebab`, `delete-selected-sandboxes`; empty title "No sandboxes"; delete dialog "Delete sandbox?"; columns Name, Status, Policy, Providers, Labels, Age |
| Sandbox API | `GET/POST /api/v1/workspaces/{ws}/sandboxes`, `DELETE .../sandboxes/{name}` |
| Logout | `LOGOUT_URL=/oauth2/sign_out` (oauth2-proxy) |

The oauth2-proxy sidecar runs with `skip-provider-button`, so the first hit on
`console_address` 302s straight to Keycloak; the callback is
`/oauth2/callback`. Realm SSO makes this silent after CON-E2E-03.

### Environment variables

| Variable | Default | Purpose |
|----------|---------|---------|
| `E2E_INFRA_DRIVER`, `E2E_MODE`, `E2E_GATEWAY_NAME`, `E2E_PROVISION_TIMEOUT`, `E2E_SANDBOX_TIMEOUT`, `E2E_GC_TIMEOUT`, `E2E_SKIP_CLEANUP`, `E2E_OIDC_*`, `E2E_DEV_*`, `E2E_SEED_CLUSTER_NAME`, `E2E_HS_NAMESPACE` | as in `e2e-testing.spec.md` | Reused unchanged (gateway name prefix defaults to `e2e-console-gw-`) |
| `AGENT_BROWSER_BIN` | `agent-browser` | CLI path |
| `AGENT_BROWSER_EXECUTABLE_PATH` | unset | Reuse an existing Chromium instead of `agent-browser install` |
| `E2E_BROWSER_TIMEOUT_MS` | `15000` | Per-action timeout (`AGENT_BROWSER_DEFAULT_TIMEOUT`) |
| `E2E_CONSOLE_READY_TIMEOUT` | `300` | Seconds to wait for `console_address` |
| `E2E_CONSOLE_ARTIFACT_DIR` | `./e2e-console-artifacts` | Screenshots, snapshots, console logs, a11y report |
| `E2E_BROWSER_INSECURE` | `0` | `1` = `--ignore-https-errors` instead of `--ca-cert` (warned) |
| `E2E_BROWSER_HEADED` | `0` | `1` = `--headed` for local debugging |
| `E2E_CONSOLE_URL` | discovered | Override the web console origin (matches the Playwright live config) |

### Kind constraints (DNS, ports, TLS)

- **DNS.** Chromium resolves any `*.localhost` name to loopback by itself, so the
  CoreDNS container from `make kind-up` is not required for the browser. It is
  still required for `curl`-based preflight and for `acquire_oidc_token`.
- **Port 443.** The browser must reach `console.hypershell.localhost:443`,
  `keycloak.hypershell.localhost:443`, and `console-<ns>.gw.localhost:443`.
  `make kind-up` sets up pfctl/iptables forwarding to the cloud-provider-kind
  ephemeral port; CI has it (the existing Playwright live step depends on the
  same). The browser suite SHALL NOT attempt `--connect-to`-style remapping and
  SHALL fail fast under `KIND_NO_SUDO=true`. A later enhancement MAY add
  `E2E_CONSOLE_PROXY=auto`: a tiny local HTTP CONNECT proxy (python3) that
  rewrites `*:443` to `127.0.0.1:${_KINDCCM_PORT}` and is passed through
  `agent-browser --proxy`; it is out of scope for the first implementation.
- **TLS.** `hypershell-https-tls` (`*.hypershell.localhost`) and `hypershell-gw-tls`
  (`*.gw.localhost`) are both issued by `hypershell-ca-issuer`, so one CA file
  trusts the web console, Keycloak, and every gateway console. Pass it with
  `--ca-cert`. This keeps the "no insecure bypass" rule of `e2e-testing.spec.md`.
- **Console on Kind.** `GATEWAY_API_BASE_DOMAIN=gw.localhost` and
  `GATEWAY_API_HTTP_LISTENER_NAME=grpc` (`deploy/kind/kustomization.yaml`), so
  `console_address` is `https://console-<ns>.gw.localhost` and the HTTPRoute
  attaches to the `grpc` listener. Nothing in the bash suite currently asserts
  `console_address`; area 4 is the first place it is proven end to end.

### OpenShift constraints

- Manual runs (`E2E_INFRA_DRIVER=openshift`, `E2E_OIDC_GRANT=password`, seeded
  users) are supported: `discover_console_host` returns the web console Route,
  `console_address` is the console Route host, and `get_browser_ca_bundle` returns
  the router CA or nothing (public CA). Set `E2E_BROWSER_INSECURE=1` only when
  the router serves a self-signed certificate that cannot be read.
- Pull-request environments broker interactive login to GitHub
  (`ephemeral-pr-environments.spec.md`); brokered users have no password grant and
  a headless browser cannot complete GitHub OAuth. The console suite therefore
  SHALL NOT be added to the `OpenShift` job. If browser coverage on OpenShift is
  wanted later, it needs a Keycloak-local test user on the PR realm, which is a
  separate spec change.

### Verification checklist (implementer)

1. `bash -n tests/e2e/e2e-console.sh tests/e2e/browser-lib.sh` and `shellcheck` clean
   (match the style of `e2e-openshell.sh`: `set -euo pipefail`, `local`, quoting).
2. `make ci-test` discovers and passes `tests/e2e/e2e_console_test.sh`.
3. `make check` passes (no em dashes, pins registered, CI components consistent).
4. Local Kind: `make kind-up` (sudo path), `npm install -g agent-browser@<pin>`,
   `agent-browser install`, then `E2E_MODE=short make e2e-console` passes and
   leaves no `openshell-*` namespace behind; `E2E_MODE=long` passes including the
   developer session and sandbox lifecycle; `E2E_SKIP_CLEANUP=1` keeps the gateway.
5. Failure path: break a selector on purpose and confirm the artifact directory
   holds the full-page screenshot, URL, snapshot, and console log for the failed
   step, and that the results block names the aborted area.
6. CI: the Kind job runs the suite on a pull request and uploads artifacts on
   failure; `merge_group` skips it; the OpenShift job is untouched.
7. `e2e-testing.spec.md`, `CLAUDE.md` Commands, and the `e2e.yml` header comment
   mention the new suite; `skills/RECONCILE.md` coverage table gains this spec.

### Implementation Notes

Confirmed against agent-browser 0.37.0 (pinned) and a live Kind cluster at
implementation. These supersede the assumptions above where they differ.

- **TLS trust.** `--ca-cert` works only with a local Chromium on Linux, and
  `--args` splits its value on commas. The suite instead pins the SPKI (base64
  sha256) of each edge certificate the browser meets, only after
  `openssl verify -CAfile <cluster CA>` succeeds, and passes the full
  `--ignore-certificate-errors-spki-list` through a generated
  `--executable-path` wrapper. The served chains are leaf-only, so the CA's own
  SPKI does not match; leaves are pinned. A probe name under the gateway base
  domain (`console-e2e-probe.<domain>`) pins the wildcard gateway certificate
  before the gateway exists; area 5 fails if `console_address` is not covered.
  No blanket bypass is used unless `E2E_BROWSER_INSECURE=1`.
- **Launch stability.** agent-browser hashes its launch configuration, including
  `AGENT_BROWSER_*` environment variables, and silently relaunches Chromium
  without `--executable-path` when it changes. Per-command timeouts use the
  `--timeout` flag; `AGENT_BROWSER_DEFAULT_TIMEOUT` is exported once. A `close`
  right before `open` on the same session name races the daemon shutdown, so
  session names carry the run id and are never pre-closed.
- **JSON envelope.** `{"success":bool,"data":{...},"error":...}` as assumed.
  `snapshot -i --json` returns `data.refs` (`{"eN":{"role","name"}}`, not in
  document order) and `data.snapshot` (text, document order).
- **Web console.** A Running gateway's status label is the API `status` field
  (for example "Healthy"), not "Running" (`resolveGatewayDisplayStatus`). The
  detail page actions toggle is named "Actions"; "Actions for {gatewayName}"
  is the list row toggle. The cluster option's accessible name is
  "<name> Provider: <provider>; region: <region>".
- **OpenShell console (sha-978bcb5).** Realm SSO makes the console login silent.
  `whoami` is `{"subject","displayName","identityProvider","roles":[...]}`;
  `/api/v1/gateway` is `{"status","gatewayVersion","computeDrivers"}`. The
  create-sandbox dialog uses test ids `sandbox-name-input`,
  `sandbox-image-input` (empty = default image), and `create-sandbox-submit`.
  Console pod containers are `dashboard` and `oauth2-proxy`.
- **Role sync.** The console session captures roles at login, so area 5 waits
  for `openshell-admin` on the gateway client before opening the console.
- **Known environment issue.** On Kind the web-console pod (250m CPU limit, 1s
  liveness timeout) can be restarted during a cold browser load: runtime brotli
  compression of large SPA assets starves the event loop. Tracked separately;
  the suite reports it as a login/landing failure with evidence.

### Open points to confirm at implementation

- agent-browser `--json` envelope, `snapshot -i --json` ref schema, whether
  `--ca-cert` applies per daemon or per call, and the pinned version (choose the
  latest release older than the repository's minimum dependency age).
- The dashboard build in the pinned image may lag upstream `main`: re-derive the
  test ids and the create-sandbox dialog fields from a `snapshot -i` against a
  live console before writing area 6.
- The developer denial surface in the dashboard (CON-E2E-08): confirm whether a
  user with no gateway role gets a 401/403 on `/api/v1/gateway`, an error card,
  or a redirect, and assert exactly that.
- Whether Keycloak SSO makes the console login silent in headless Chromium
  (third-party cookie policy does not apply: it is a top-level navigation), and
  whether `wait --url` needs the callback intermediate glob.
- Container names of the console pod (`dashboard`, `oauth2-proxy`) for diagnostics.
