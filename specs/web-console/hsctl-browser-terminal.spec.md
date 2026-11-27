# hsctl Browser Terminal - site-wide, session-authenticated hsctl in the web console

**ID prefix:** WEB-TERM

**Date:** 2026-11-24
**Status:** Draft
**Source mockups:** None approved yet. The `docs-preview` prototype branch is a visual reference only. Until an approved mockup exists, the parity checks of UIS-VER-10 do not apply; every non-parity check still applies (WEB-TERM-14).
**Related:** [`../platform/hsctl-browser-build.spec.md`](../platform/hsctl-browser-build.spec.md) (the `js/wasm` `hsctl` runtime and its bridge CLI-WASM), [`user-documentation.spec.md`](./user-documentation.spec.md) (docs pages and runbook cells WEB-UDOC), [`architecture.spec.md`](./architecture.spec.md) (session and CSRF WEB-AUTH-02/03, BFF WEB-BFF-01, storage rules WEB-DATA-02, specialized widgets WEB-UIEX-01, assets and no service worker WEB-DPLY-02, CSP WEB-SEC-01, telemetry WEB-OBS-01), [`../standards/ui/`](../standards/ui/) (UI standards)

## Purpose

A signed-in console user can run `hsctl` commands from any console page in a
terminal drawer, with no installation, no `hsctl login`, and no bearer token in
browser JavaScript. The terminal runs the real `hsctl` command tree, compiled to
WebAssembly ([`../platform/hsctl-browser-build.spec.md`](../platform/hsctl-browser-build.spec.md)),
in a Web Worker. Every request goes to the same-origin BFF `/api` proxy with the
console session cookie, so authentication, CSRF protection, and server-side
authorization stay exactly as they are for the rest of the console. The drawer
adds no clutter while closed, docks at the bottom or right, resizes, and shares
one history with the runbook cells of the docs pages.

## Requirements

### Requirement: WEB-TERM-01 -- Masthead Trigger and Global Toggle

The application masthead SHALL show one quiet icon button next to the user
identity on every console page, including docs pages. Its accessible name SHALL
be "Open hsctl terminal" while the drawer is closed and "Close hsctl terminal"
while it is open. `` Ctrl+` `` (Control and backtick) SHALL toggle the drawer from
anywhere in the console. `Esc` SHALL close the drawer when focus is inside it.
Opening SHALL move focus to the terminal input; closing SHALL return focus to the
element that had it before the drawer opened, or to the trigger.

While closed, the drawer SHALL render nothing except the trigger. When the reader
is signed out, the open drawer SHALL show only a "Sign in to use the hsctl
terminal" link to `/auth/login?return_to=<current path and query>` and SHALL NOT
load the runtime.

#### Scenario: WEB-TERM-01.01 -- Keyboard toggle and focus return

- GIVEN a signed-in user with focus on a gateway list link
- WHEN the user presses `` Ctrl+` `` and then `Esc`
- THEN the drawer SHALL open with focus in the terminal input
- AND then close with focus back on the gateway list link

#### Scenario: WEB-TERM-01.02 -- Signed-out drawer

- GIVEN a signed-out reader on a docs page
- WHEN the reader opens the drawer
- THEN the drawer SHALL show the sign-in link with the current docs path as `return_to`
- AND no runtime asset SHALL be requested

### Requirement: WEB-TERM-02 -- Bottom and Right Docking

The drawer SHALL use the PatternFly Drawer in inline mode (UIS-PFLY-05) and SHALL
dock at the bottom or the right. A header control labeled "Dock right" or "Dock
bottom" SHALL switch the position. A right-docked drawer SHALL place the page and
the terminal side by side so the page content reflows and stays readable, not
covered. Below the PatternFly `md` breakpoint the drawer SHALL dock at the bottom
whatever the saved position. The position SHALL be saved in `localStorage` under
a console-namespaced key; it is a non-sensitive preference (WEB-DATA-02).

#### Scenario: WEB-TERM-02.01 -- Right dock keeps the page readable

- GIVEN a signed-in user reads a runbook on a wide viewport
- WHEN the user selects Dock right
- THEN the runbook and the terminal SHALL show side by side with no overlap
- AND after a reload the drawer SHALL open docked right

### Requirement: WEB-TERM-03 -- Resizing

The drawer edge next to the page SHALL be a resize handle with
`role="separator"`, an accessible name, `aria-orientation`, `aria-valuenow`,
`aria-valuemin`, and `aria-valuemax`. Pointer drag and the arrow keys SHALL
resize the drawer; each arrow key press SHALL change the size by 24 CSS pixels.
The size SHALL be clamped to 180 CSS pixels through 70% of the viewport height
when docked at the bottom, and to 320 CSS pixels through 65% of the viewport
width when docked at the right. The size for each position SHALL be saved in
`localStorage` and SHALL be clamped again when the viewport changes.

#### Scenario: WEB-TERM-03.01 -- Keyboard resize is clamped

- GIVEN a bottom-docked drawer that is 190 CSS pixels tall
- WHEN the user focuses the handle and presses the arrow key that shrinks the drawer
- THEN the drawer SHALL become 180 CSS pixels tall, not 166
- AND `aria-valuenow` SHALL report the new size

### Requirement: WEB-TERM-04 -- Command Entry and Execution

The terminal SHALL show a prompt with one text input and a log of earlier
commands and output. `Enter` SHALL run the input line. The line SHALL be split
into words with single quotes, double quotes, and backslash escapes, as a POSIX
shell splits words. A leading `hsctl` word SHALL be optional. The terminal SHALL
run the remaining words through the browser runtime bridge (CLI-WASM-02).

The terminal SHALL NOT run a shell. When the line contains an unquoted pipe,
redirection, `;`, `&`, `$(`, or backtick, the terminal SHALL say that shell
operators are not supported and SHALL NOT run the line. When the first word is
neither `hsctl` nor an `hsctl` command, the terminal SHALL say that only `hsctl`
commands run here and that other programs, such as `openshell` or `kubectl`,
must run in a local terminal. `clear` SHALL clear the log.

One command SHALL run at a time. While a command runs, `Enter` SHALL NOT start
another command. Standard output and standard error SHALL render as text;
standard error SHALL be marked by a text or icon cue in addition to color. A
non-zero exit code SHALL be shown after the output.

#### Scenario: WEB-TERM-04.01 -- An hsctl command runs with the session

- GIVEN a signed-in user with the runtime loaded
- WHEN the user enters `hsctl list gateways`
- THEN the bridge SHALL receive `["list", "gateways"]`
- AND the log SHALL show the command output as text

#### Scenario: WEB-TERM-04.02 -- Shell operators are refused

- GIVEN a signed-in user with the runtime loaded
- WHEN the user enters `hsctl list gateways | grep prod`
- THEN the terminal SHALL say that shell operators are not supported
- AND the bridge SHALL NOT be called

### Requirement: WEB-TERM-05 -- Line Editing Keys

While the terminal input has focus, the terminal SHALL handle these keys and
SHALL prevent the browser default action when the browser delivers the key event
to the page:

| Keys | Action |
|------|--------|
| `Ctrl+L` | Clear the log |
| `Ctrl+U` | Delete from the cursor to the start of the line |
| `Ctrl+K` | Delete from the cursor to the end of the line |
| `Ctrl+A` | Move the cursor to the start of the line |
| `Ctrl+E` | Move the cursor to the end of the line |
| `Ctrl+C` | With no running command, print the line followed by `^C` and clear the input; with a running command, cancel it (CLI-WASM-02) |
| `Ctrl+W`, `Alt+Backspace`, `Ctrl+Backspace` | Delete the word before the cursor |
| `ArrowUp`, `ArrowDown` | Recall earlier and later commands from the in-memory history |

Outside the terminal input these keys SHALL keep their browser behavior.

#### Scenario: WEB-TERM-05.01 -- Ctrl+C cancels a running command

- GIVEN a command is running in the terminal
- WHEN the user presses `Ctrl+C`
- THEN the command SHALL be cancelled and the log SHALL say it was cancelled
- AND the prompt SHALL accept a new command

### Requirement: WEB-TERM-06 -- Tab-Close Protection

The terminal SHALL register a `beforeunload` guard only while the drawer is open
and the input has focus, the input is not empty, or a command is running. It SHALL
remove the guard as soon as none of these is true. This protects work when a
browser reserves `Ctrl+W` to close the tab.

#### Scenario: WEB-TERM-06.01 -- Guard follows the terminal state

- GIVEN the drawer is open with an empty, unfocused input and no running command
- WHEN the user starts typing in the input
- THEN the `beforeunload` guard SHALL be registered
- AND it SHALL be removed after the user clears the input, moves focus out, and no command runs

### Requirement: WEB-TERM-07 -- Shared In-Memory History

The terminal log SHALL be one chronological history of typed commands and
runbook cell runs (WEB-UDOC-05); a runbook entry SHALL be tagged `[runbook]`.
**Open in terminal** on a runbook cell SHALL open the drawer and scroll to that
entry. The log SHALL keep at most 5,000 lines; when it drops older lines, it SHALL
say so at the top.

The history SHALL exist only in memory for the current page session. It SHALL
NOT be written to `localStorage`, `sessionStorage`, IndexedDB, a cache, or a URL
(WEB-DATA-02), because command output can contain one-time credentials. The
console SHALL clear the history on sign-out and when the session identity
changes.

When a terminal command that sent at least one state-changing request finishes
(CLI-WASM-02), the console SHALL invalidate its cached gateway queries, so open
pages and runbook cells show the change.

#### Scenario: WEB-TERM-07.01 -- Terminal change refreshes a runbook

- GIVEN a runbook shows a gateway in Step 1
- WHEN the user deletes that gateway with `hsctl delete gateway <id> --yes` in the terminal
- THEN the console SHALL invalidate the gateway queries
- AND the runbook SHALL show the refreshed gateway state without a reload

#### Scenario: WEB-TERM-07.02 -- History is not persisted

- GIVEN the user ran commands that printed output
- WHEN browser storage, caches, and the URL are inspected
- THEN none of them SHALL contain a command or its output

### Requirement: WEB-TERM-08 -- Motion and Scrolling

Under `prefers-reduced-motion: no-preference`, each newly added log line SHALL
fade and slide in using only `opacity` and `transform: translateY` over 160
milliseconds. Lines added together SHALL use a short stagger that applies to at
most 8 lines. Existing lines SHALL keep stable keys and SHALL NOT animate again.
When the log is scrolled to the bottom, new output SHALL scroll into view with
`scrollTo({ behavior: "smooth" })`; when the user has scrolled up, the log SHALL
keep its position. Under `prefers-reduced-motion: reduce`, lines SHALL appear
without animation and scrolling SHALL be instant (UIS-A11Y-08).

#### Scenario: WEB-TERM-08.01 -- Reduced motion

- GIVEN the operating system requests reduced motion
- WHEN a command prints output
- THEN new lines SHALL appear with no animation
- AND the log SHALL scroll to the bottom instantly

### Requirement: WEB-TERM-09 -- Lazy Runtime Loading

The console SHALL request the runtime assets (the worker script, `wasm_exec.js`,
and `hsctl.wasm`, CLI-WASM-05) only when a signed-in user opens the drawer for the
first time in a page session. The initial load of any console or docs page SHALL
NOT request them. While the runtime loads, the drawer SHALL show truthful
progress, and the input SHALL NOT accept a command. When loading fails, the
drawer SHALL show a localized error and a Retry action.

#### Scenario: WEB-TERM-09.01 -- No runtime download before the drawer opens

- GIVEN a signed-in user opens `/gateways` and `/docs/runbooks/github-jira` and runs Step 1
- WHEN the network requests are inspected
- THEN no runtime asset SHALL have been requested
- AND opening the drawer SHALL request the runtime assets once

### Requirement: WEB-TERM-10 -- Worker Isolation, CSP, and Session Use

The runtime SHALL run in a dedicated Web Worker, so a long command never blocks
the page. The SPA document CSP SHALL add `worker-src 'self'` and SHALL NOT
allow WebAssembly compilation. The BFF SHALL serve the worker script with its own
CSP that allows `'self'` scripts, `'wasm-unsafe-eval'`, and `connect-src 'self'`.
No CSP SHALL allow `'unsafe-eval'` (WEB-SEC-01).

The worker SHALL send API requests only to the same-origin `/api/hypershell/`
paths, with the session cookie and without an `Authorization` header
(CLI-WASM-03). These requests SHALL pass the existing BFF CSRF check because the
browser marks them same-origin. When a request needs re-authentication, the
terminal SHALL print a localized message with a sign-in link that uses the
current path and query as `return_to`, and SHALL NOT navigate by itself.

#### Scenario: WEB-TERM-10.01 -- Only the worker may compile WebAssembly

- GIVEN the production BFF
- WHEN the SPA document and the worker script responses are inspected
- THEN the document CSP SHALL contain `worker-src 'self'` and SHALL NOT contain `'wasm-unsafe-eval'` or `'unsafe-eval'`
- AND the worker script CSP SHALL contain `'wasm-unsafe-eval'` and SHALL NOT contain `'unsafe-eval'`

#### Scenario: WEB-TERM-10.02 -- Expired session in the terminal

- GIVEN a signed-in user whose session can no longer be renewed
- WHEN the user runs `hsctl list gateways`
- THEN the terminal SHALL print that the session expired with a sign-in link
- AND the page SHALL stay where it is

### Requirement: WEB-TERM-11 -- Installable App Window

The BFF SHALL serve a web app manifest at `/manifest.webmanifest` with media type
`application/manifest+json`, without a session, because browsers request a
manifest without credentials. The manifest SHALL set `name` to `HyperShell`,
`display` to `standalone`, `start_url` to `/`, and `scope` to `/`, and SHALL
list HyperShell brand icons served from `/assets/`. The SPA document SHALL link to
the manifest, and its CSP SHALL add `manifest-src 'self'`. The console SHALL NOT
register a service worker (WEB-DPLY-02). In an installed app window, the
terminal key handling of WEB-TERM-05 SHALL apply to every key event the browser
delivers to the page.

#### Scenario: WEB-TERM-11.01 -- Public manifest without a service worker

- GIVEN a browser with no console session
- WHEN it requests `/manifest.webmanifest`
- THEN the BFF SHALL return the manifest with `display` set to `standalone`
- AND the console SHALL register no service worker

### Requirement: WEB-TERM-12 -- Accessibility and Presentation

The open drawer SHALL be a labeled, non-modal region that does not trap focus.
The log SHALL use `role="log"`, and the completion of each command, with its exit
status, SHALL be announced politely. The terminal SHALL be a text input and a log
rendered as React elements with PatternFly tokens in light and dark themes; it
SHALL NOT add a terminal emulator dependency or interpret ANSI escape sequences.
It SHALL NOT use any raw-HTML injection path. The terminal is a specialized
widget and SHALL carry the admission evidence of WEB-UIEX-01 and UIS-PFLY-06: a
documented decision, an accessibility review, and a bundle assessment.

#### Scenario: WEB-TERM-12.01 -- Output markup stays text

- GIVEN a command prints `<img src=x onerror=alert(1)>`
- WHEN the log renders the output
- THEN the log SHALL show those characters as text
- AND no element SHALL be created from the output

### Requirement: WEB-TERM-13 -- Probes, Localization, and Privacy

The terminal SHALL publish typed domain facts through the fan-out probe port
(UIS-OBS) for drawer opened and closed, runtime load succeeded or failed with its
duration, and command finished with its exit status class. A command-finished
fact SHALL name the command only by its `hsctl` command path from a bounded list
of known command paths (for example `list gateways`), or `other`. No fact SHALL
contain arguments, flag values, identifiers, or output (WEB-OBS-01). Terminal
labels, messages, and errors SHALL be FormatJS messages (WEB-I18N-01); `hsctl`
output SHALL be shown as the runtime returns it.

#### Scenario: WEB-TERM-13.01 -- Command facts carry no arguments

- GIVEN the user runs `hsctl delete gateway 3f2a --yes`
- WHEN the command-finished fact is published
- THEN it SHALL name the command path `delete gateway` and the exit status class
- AND it SHALL NOT contain `3f2a`, the flag, or any output

### Requirement: WEB-TERM-14 -- Verification

The terminal SHALL have Storybook stories for closed, signed out, loading,
load failed, idle bottom dock, idle right dock, running command, failed command,
and long output. Playwright journeys in Chromium, Firefox, and WebKit SHALL cover
the keyboard toggle, keyboard resize, a command through the production BFF with a
recording upstream (which SHALL show the session cookie and no browser-supplied
`Authorization` header), the CSP split of WEB-TERM-10, and the absence of runtime
requests before the drawer opens. When an approved mockup is added, the terminal
SHALL pass the web-console completion gate, including the parity checks of
UIS-VER-10.

#### Scenario: WEB-TERM-14.01 -- Production-shaped command journey

- GIVEN the built BFF with a recording upstream API and a signed-in session
- WHEN the journey runs `hsctl list gateways` in the terminal
- THEN the upstream SHALL receive the request with the BFF-set bearer token
- AND the browser request SHALL carry the session cookie and no `Authorization` header
