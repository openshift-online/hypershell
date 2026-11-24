# hsctl Browser Build - js/wasm runtime for the web console terminal

**ID prefix:** CLI-WASM

**Date:** 2026-11-24
**Status:** Draft
**Applies to:** `components/cli` (a new `cmd/hsctl-wasm` entry point and the shared command registration), `components/web-console/Dockerfile`, `.tekton/hypershell-web-console-main-*.yaml`, and CLI CI checks
**Related:** [`../web-console/hsctl-browser-terminal.spec.md`](../web-console/hsctl-browser-terminal.spec.md) (the terminal that hosts this runtime WEB-TERM), [`hsctl-release.spec.md`](hsctl-release.spec.md) (native release targets and bundle membership CLI-REL), [`hsctl-terminal-ui.spec.md`](hsctl-terminal-ui.spec.md) (`hsctl tui` CLI-TUI), [`hsctl-extension-commands.spec.md`](hsctl-extension-commands.spec.md) (extension commands CLI-EXT), [`../web-console/architecture.spec.md`](../web-console/architecture.spec.md) (session, CSRF, assets, and CSP WEB-AUTH-02, WEB-DPLY-02, WEB-SEC-01)

## Purpose

`hsctl` also builds for `GOOS=js GOARCH=wasm`, so the web console can run the
real `hsctl` command tree in the browser
([`../web-console/hsctl-browser-terminal.spec.md`](../web-console/hsctl-browser-terminal.spec.md)).
The browser build has the same commands, flags, help text, and output as the
native binary. The only differences are the commands that need a local
machine. The browser build has no saved login: it sends every request to the
console's same-origin `/api` proxy, and the console session cookie
authenticates it. The web console image builds and serves the module. It is not
a GitHub release asset and not a release bundle component.

## Requirements

### Requirement: CLI-WASM-01 -- Browser Build Target

`components/cli` SHALL build a browser module from the entry point
`cmd/hsctl-wasm` with `GOOS=js GOARCH=wasm` and `CGO_ENABLED=0`. The native
`hsctl` and the browser module SHALL build their command trees from one shared
registration, so a command added to `hsctl` is also in the browser module with no
second registration, unless the command is local-only (CLI-WASM-04). Build
constraints SHALL exclude `hsctl tui` (CLI-TUI-01) from the browser module, so the
module links neither `github.com/charmbracelet/bubbletea` nor
`github.com/atotto/clipboard`. The native release targets and their contents
(CLI-REL-01) SHALL NOT change.

#### Scenario: CLI-WASM-01.01 -- A new command reaches the browser

- GIVEN a developer adds a new resource command to `hsctl`
- WHEN both the native binary and the browser module are built
- THEN the command SHALL be in both command trees with the same flags and help

#### Scenario: CLI-WASM-01.02 -- The terminal interface is excluded

- GIVEN the browser module build
- WHEN its linked packages are listed
- THEN they SHALL NOT include `bubbletea` or `atotto/clipboard`
- AND the native `hsctl` SHALL still include `hsctl tui`

### Requirement: CLI-WASM-02 -- JavaScript Bridge

The browser module SHALL define exactly one global function in its worker scope:
`hsctlRun(argv)`. `argv` SHALL be an array of strings without the program name.
The function SHALL return a promise that resolves to an object with `stdout`
(string), `stderr` (string), `exitCode` (integer), and `mutated` (boolean: `true`
when the command sent at least one request with a method other than `GET`,
`HEAD`, or `OPTIONS`). A non-array `argv` or a non-string element SHALL resolve
with exit code `2`, a usage message in `stderr`, and no request.

Each call SHALL start from fresh command and flag state, so no flag value or
argument from an earlier call affects a later call. The module SHALL run one call
at a time; a call made while another runs SHALL resolve at once with exit code `1`
and a busy message, and SHALL NOT start the command. Exit codes and error text
SHALL match the native `hsctl` for the same command and API response. The module
SHALL NOT start a pager, and standard input SHALL be empty and not a terminal, so
a command that asks for confirmation behaves as it does in a script and needs
`--yes`. The host cancels a running call by terminating the worker; the next call
then uses a new worker.

#### Scenario: CLI-WASM-02.01 -- Flags do not leak between calls

- GIVEN `hsctlRun(["list", "gateways", "--search", "name like 'prod%'"])` has finished
- WHEN `hsctlRun(["list", "gateways"])` runs
- THEN the second request SHALL carry no search filter

#### Scenario: CLI-WASM-02.02 -- Confirmation needs --yes

- GIVEN a signed-in session
- WHEN `hsctlRun(["delete", "gateway", "3f2a"])` runs
- THEN it SHALL resolve with a non-zero exit code and the native message that asks for `--yes`
- AND no delete request SHALL be sent
- AND `mutated` SHALL be `false`

### Requirement: CLI-WASM-03 -- Console Session Connection

The browser module SHALL send API requests to the page origin with the paths the
native `hsctl` uses: `/api/hypershell/v1` and `/api/hypershell/ext`. It SHALL send
them through the Go `js/wasm` `fetch` transport with same-origin credentials, so
the console session cookie authenticates them. It SHALL NOT set an
`Authorization` header, read or write a configuration file, or hold a token. It
SHALL NOT retry a request with a method other than `GET`, `HEAD`, or `OPTIONS`.
When the BFF answers that re-authentication is needed, the command SHALL fail
with a non-zero exit code and a message that says the console session expired
and the user must sign in to the console again.

#### Scenario: CLI-WASM-03.01 -- Requests use the session, not a token

- GIVEN a signed-in console session
- WHEN `hsctlRun(["list", "gateways"])` runs
- THEN the module SHALL request `/api/hypershell/v1/gateways` on the page origin with same-origin credentials
- AND the request SHALL carry no `Authorization` header

### Requirement: CLI-WASM-04 -- Local-Only Commands

These commands SHALL be local-only in the browser module: `login`, `logout`,
`whoami`, `config`, `completion`, and `tui`; any command or flag that reads or writes a
local file or reads standard input (for example `apply -f` and SecretSource
material upload, CLI-EXT-05); and any command that starts a local process. A
local-only command SHALL send no request and SHALL resolve with exit code `1`
and a message that says the command needs the installed `hsctl`, names the
console page `/docs/cli/install-hsctl`, and, for `login`, `logout`, and `whoami`,
says that the browser terminal uses the console sign-in. `help` and `version` SHALL work in
the browser module.

#### Scenario: CLI-WASM-04.01 -- login explains the browser session

- GIVEN the browser module
- WHEN `hsctlRun(["login"])` runs
- THEN it SHALL resolve with exit code `1`
- AND `stderr` SHALL say that the browser terminal uses the console sign-in and that `hsctl login` needs the installed `hsctl` (`/docs/cli/install-hsctl`)
- AND no request SHALL be sent

### Requirement: CLI-WASM-05 -- Packaging and Delivery

The web console image build SHALL compile the browser module from `components/cli`
at the same source commit, with the Go toolchain that `components/cli/go.mod`
pins. It SHALL copy `wasm_exec.js` from that same toolchain. The BFF SHALL serve
`hsctl.wasm`, `wasm_exec.js`, and the terminal worker script as content-hashed,
compressed, immutable assets under `/assets/` (WEB-DPLY-02), with `hsctl.wasm`
served as `application/wasm`. The compressed transfer size of `hsctl.wasm` SHALL
NOT exceed 8 MiB; CI SHALL fail when a change exceeds it.

The web console Konflux pipelines SHALL prefetch the `components/cli` Go modules
for the hermetic build and SHALL run when `components/cli` changes. The browser
module SHALL NOT be a GitHub release asset or a release bundle component, so the
release asset set (CLI-REL-03) and the required bundle components (CLI-REL-02)
stay the same.

#### Scenario: CLI-WASM-05.01 -- A CLI change rebuilds the console image

- GIVEN a pull request that changes only `components/cli`
- WHEN Konflux evaluates the web console pipeline triggers
- THEN the web console image SHALL build with a browser module from that commit
- AND the hermetic build SHALL resolve the CLI Go modules from the prefetched set

#### Scenario: CLI-WASM-05.02 -- Size budget

- GIVEN a change that makes compressed `hsctl.wasm` larger than 8 MiB
- WHEN CI runs
- THEN the size check SHALL fail and report the measured and allowed sizes

### Requirement: CLI-WASM-06 -- Verification

CLI CI SHALL build and vet `components/cli` for `GOOS=js GOARCH=wasm`. Tests run
under the Go `js/wasm` test runner SHALL cover the bridge input checks, fresh flag
state, the busy result, the `mutated` flag, the session connection (no
`Authorization` header and no configuration file), and the local-only result. A
drift test SHALL fail when the browser command tree differs from the native
command tree by anything other than the local-only set of CLI-WASM-04.

#### Scenario: CLI-WASM-06.01 -- Command tree drift is caught

- GIVEN a command registered only in the native entry point
- WHEN the drift test runs
- THEN it SHALL fail and name the command
