# User Documentation - public docs, executable runbooks, and refusal help in the web console

**ID prefix:** WEB-UDOC

**Date:** 2026-11-24
**Status:** Draft
**Source mockups:** None approved yet. The `docs-preview` prototype branch is a visual reference only and is not an approved mockup. Until approved mockups exist under `specs/web-console/mockups/docs/`, the mockup-to-production parity checks of UIS-VER-10 do not apply to these pages; every non-parity check still applies (WEB-UDOC-13).
**Related:** [`architecture.spec.md`](./architecture.spec.md) (routing WEB-ARCH-01, BFF WEB-BFF-01, session WEB-AUTH-02/03, runtime configuration WEB-DPLY-02, CSP WEB-SEC-01, connection commands WEB-UIEX-03/05), [`hsctl-browser-terminal.spec.md`](./hsctl-browser-terminal.spec.md) (site-wide `hsctl` terminal WEB-TERM), [`homepage.spec.md`](./homepage.spec.md) (route registry WEB-HOME-02, support channel WEB-HOME-09, Quick create placement and name generation WEB-HOME-12/13), [`gateway-provision-placement.spec.md`](./gateway-provision-placement.spec.md) (placement intent and availability WEB-PLC-01/04/07), [`platform-policy-admin.spec.md`](./platform-policy-admin.spec.md) (documentation link suggestion WEB-PADM-13), [`../platform/platform-gateway-policy.spec.md`](../platform/platform-gateway-policy.spec.md) (denial text and `documentationUrl` PLAT-PGP-14, fleet defaults PLAT-PGP-04/08), [`../platform/hsctl-release.spec.md`](../platform/hsctl-release.spec.md) (release assets and `SHA256SUMS` CLI-REL-01/03), [`../platform/hsctl-placement.spec.md`](../platform/hsctl-placement.spec.md) (CLI placement flags CLI-PLC), [`../standards/ui/`](../standards/ui/) (UI standards)

## Purpose

The web console hosts a small, calm user documentation section under `/docs`.
It serves two kinds of reader. The first reader wants to finish a task, such as
creating a sandbox that reaches GitHub and Jira, or internal Red Hat GitLab and
Jira over the Red Hat VPN. Runbooks guide this reader one step at a time. A
HyperShell step runs directly in the page, like a notebook cell, and its result
fills in the OpenShell commands of the later steps. The second reader arrives
from a link, often in a refusal message printed by the OpenShell CLI, and needs a
plain answer without first signing in. Docs pages therefore render for signed-out
readers, while every HyperShell API request keeps its full authentication,
CSRF protection, and authorization.

The section also contains an `hsctl` installation guide. Signed-in readers can
open the site-wide browser `hsctl` terminal from any page
([`hsctl-browser-terminal.spec.md`](./hsctl-browser-terminal.spec.md)).

## Requirements

### Requirement: WEB-UDOC-01 -- Docs Routes and Route Contract

The web console SHALL declare these docs routes in the internal route registry
(WEB-HOME-02) and in the shared browser and BFF route contract
(`components/web-console/route-contract.json`, WEB-BFF-01):

| Route | Page |
|-------|------|
| `/docs` | Docs home |
| `/docs/runbooks/github-jira` | GitHub and Jira sandbox runbook |
| `/docs/runbooks/gitlab-jira` | GitLab and Jira sandbox runbook |
| `/docs/cli/install-hsctl` | Install `hsctl` |
| `/docs/refusals/resource-limits` | Resource limit refusal help |

`/docs/` SHALL render the docs home, the same as `/docs`. Each docs route SHALL
be a lazily loaded route module, so code used only by docs pages loads only when
a docs route is visited and adds no bytes to the route bundle of `/gateways` or
of any other non-docs route. Direct navigation and refresh of every docs route
SHALL return the SPA document (WEB-ARCH-01). A path under `/docs/` that is not a
declared route SHALL receive the same response as any other undeclared path.

#### Scenario: WEB-UDOC-01.01 -- Direct navigation to a docs route

- GIVEN the production BFF serves the built console
- WHEN a browser requests `/docs/cli/install-hsctl` directly
- THEN the BFF SHALL return the SPA document
- AND React Router SHALL render the Install `hsctl` page

#### Scenario: WEB-UDOC-01.02 -- Docs code stays out of other route bundles

- GIVEN a production build
- WHEN the route chunks loaded by `/gateways` are inspected
- THEN they SHALL contain no module that only docs routes import

### Requirement: WEB-UDOC-02 -- Public Docs Navigation at the BFF

When OIDC authentication is configured, the BFF SHALL return the SPA document
for a `GET` navigation to a declared docs route (WEB-UDOC-01) without a session
and without a redirect to `/auth/login`. The public set SHALL be exactly the
declared docs routes, matched on the normalized request path, plus the public
web app manifest (WEB-TERM-11). Every other application route SHALL keep its
current sign-in redirect and role checks.

This exception covers only the SPA document. The `/api/*` proxy SHALL keep
requiring a session, CSRF protection (WEB-AUTH-02), and server-side
authorization for every request, whether or not the request comes from a docs
page. The SPA document SHALL stay identical for signed-in and signed-out
requests, so a public docs navigation discloses no protected data.

#### Scenario: WEB-UDOC-02.01 -- Signed-out reader opens a refusal link

- GIVEN a browser with no console session
- WHEN it requests `/docs/refusals/resource-limits?policy=sandbox-cpu-limit`
- THEN the BFF SHALL return `200` with the SPA document
- AND it SHALL NOT redirect to `/auth/login`

#### Scenario: WEB-UDOC-02.02 -- Other routes and the API stay protected

- GIVEN a browser with no console session
- WHEN it requests `/gateways`
- THEN the BFF SHALL redirect it to sign in, as before
- AND a request to `/api/hypershell/v1/gateways` without a session SHALL be refused, as before

#### Scenario: WEB-UDOC-02.03 -- Look-alike paths are not public

- GIVEN a browser with no console session
- WHEN it requests `/docsx`, `/docs/../gateways`, or `/docs/%2e%2e/gateways`
- THEN none of these requests SHALL receive the public docs exception

### Requirement: WEB-UDOC-03 -- Session-Aware Docs Rendering

Docs pages SHALL find the session state from the browser session resource
(WEB-AUTH-03). A signed-out docs page SHALL send no `/api/*` request, so the
console's re-authentication redirect never starts on page load.

For a signed-out reader, every docs page SHALL render its full prose, diagrams,
and copyable commands. Each executable cell SHALL show a link labeled "Sign in to
run this step here" in place of its Run action. The link SHALL go to
`/auth/login?return_to=<path and query of the current docs page>`, so the reader
returns to the same page after sign-in. The application masthead on a docs page
SHALL show a sign-in action in place of the user identity. When the session
resource cannot be read, the page SHALL render as signed out.

For a signed-in reader, executable cells (WEB-UDOC-05) and the `hsctl` terminal
(WEB-TERM) SHALL run against the HyperShell API through the existing session
cookie. When a cell request needs re-authentication, the cell SHALL show the same
sign-in link with the current docs path as `return_to`.

#### Scenario: WEB-UDOC-03.01 -- Signed-out runbook renders without API traffic

- GIVEN a signed-out reader
- WHEN the reader opens `/docs/runbooks/github-jira`
- THEN all steps, prose, and copyable commands SHALL render
- AND each executable cell SHALL show "Sign in to run this step here" linked to `/auth/login?return_to=%2Fdocs%2Frunbooks%2Fgithub-jira`
- AND the page SHALL send no `/api/*` request

#### Scenario: WEB-UDOC-03.02 -- Sign-in returns the reader to the page

- GIVEN a signed-out reader on `/docs/refusals/resource-limits?policy=sandbox-cpu-limit`
- WHEN the reader signs in through the cell link
- THEN the console SHALL return the reader to `/docs/refusals/resource-limits?policy=sandbox-cpu-limit`

### Requirement: WEB-UDOC-04 -- Calm Layout and Docs Home

Docs pages SHALL use a single readable column no wider than 760 CSS pixels, with
generous spacing and PatternFly 6 components and tokens (UIS-PFLY-01). Docs pages
SHALL NOT show status labels or badges as decoration, gateway status summaries,
or a search control. Each runbook step SHALL show at most one primary button.

The docs home SHALL contain, in order:

1. A title and a one-line subtitle.
2. Two clickable cards, each with a title, a one-sentence description, and a
   trailing arrow:
   - **GitHub + Jira sandbox**: "Set up a sandbox that can reach GitHub and Jira
     Cloud." It links to `/docs/runbooks/github-jira`.
   - **GitLab + Jira sandbox**: "Set up a sandbox that can reach internal Red Hat
     GitLab and Jira. Requires Red Hat VPN." It links to
     `/docs/runbooks/gitlab-jira`.
3. One muted line of text with links to **Install hsctl**
   (`/docs/cli/install-hsctl`) and **View resource limit help**
   (`/docs/refusals/resource-limits`).

The docs home SHALL send no HyperShell API request.

#### Scenario: WEB-UDOC-04.01 -- Docs home shows two task cards

- GIVEN any reader
- WHEN the reader opens `/docs`
- THEN the page SHALL show the title, the subtitle, the two runbook cards, and the muted help line
- AND each card SHALL be one link target with the described title and description
- AND the page SHALL send no `/api/hypershell/*` request

### Requirement: WEB-UDOC-05 -- Notebook-Style Executable Cells

Each runbook step SHALL be a cell that contains a step number, a short title,
short prose, a statement of where the step runs ("Runs here in your browser" or
"Run in your local terminal"), and a command block with a Copy action. A cell
that runs in the browser SHALL show the equivalent `hsctl` command in its command
block, built with the CLI placement flags (CLI-PLC), and SHALL have one Run
action.

Run SHALL perform the step through the gateway management UI package application
ports (WEB-PKG-04, UIS-HEX). It SHALL NOT call the generated SDK directly and
SHALL NOT need the browser `hsctl` runtime. While the step runs, the cell SHALL
show a dark, read-only output pane directly below the command, and Run SHALL be
disabled to prevent a duplicate submission. The output pane SHALL render text
only.

When a browser cell produces a gateway, every later cell SHALL update its
commands with that gateway's authorized values. Until those values exist, a later
cell SHALL show clearly marked placeholders and SHALL name the step that supplies
them. After Step 1 reaches `Running`, its Run action SHALL become a secondary
**Re-check** action, so the primary focus moves to Step 2.

Every cell run SHALL append its command and output to the shared terminal history
tagged `[runbook]` (WEB-TERM-07), and the cell SHALL offer an **Open in terminal**
link that opens the terminal at that entry.

URL query parameters and runbook defaults MAY prefill cell inputs. A cell SHALL
send a state-changing request only after the reader activates Run, never on page
load, on navigation, or when a prefilled value changes.

#### Scenario: WEB-UDOC-05.01 -- Prefill never submits

- GIVEN a signed-in reader opens a runbook URL whose query prefills a gateway name
- WHEN the page loads
- THEN the name field SHALL show the prefilled value
- AND no gateway create request SHALL be sent until the reader activates Run

#### Scenario: WEB-UDOC-05.02 -- Running Step 1 updates later steps and demotes Run

- GIVEN a signed-in reader runs Step 1
- WHEN the created gateway reaches `Running` with its connection values
- THEN Step 2 SHALL show commands that contain that gateway's authorized values
- AND the Step 1 primary Run action SHALL be replaced by a secondary Re-check action
- AND the run SHALL appear in the terminal history tagged `[runbook]`

### Requirement: WEB-UDOC-06 -- GitHub and Jira Sandbox Runbook

`/docs/runbooks/github-jira` SHALL contain three steps.

1. **Choose or create a gateway (runs in the browser).** The reader SHALL be
   able to select an existing gateway they can access, or create one. The new
   gateway name SHALL be prefilled with a name from the Quick create name generator (WEB-HOME-13), and the reader MAY edit it. The
   placement intent SHALL be the first available of: `selector: ["provider/kind"]`
   (local development, as in Quick create, WEB-HOME-12);
   `selector: ["network/public", "provider/ibm"]` with
   `excludedSelector: ["runtime/kata"]`; and
   `selector: ["network/public", "provider/aws"]` (WEB-PLC-01). Availability
   SHALL come from one batch availability request (WEB-PLC-04). When no public
   placement is available, Run SHALL be disabled and the cell SHALL state why.
   After create, the cell SHALL show the lifecycle as it moves through `Pending`,
   `Provisioning`, and `Running`, using the bounded polling rules of
   WEB-UIEX-03. A gateway that is waiting for a matching cluster (WEB-PLC-07)
   SHALL be shown as waiting for placement.
2. **Connect the OpenShell CLI (run in your local terminal).** The cell SHALL show
   the OpenShell CLI installation command and the `openshell gateway add` command,
   built with the same builders, shell-argument encoding, readiness rule, and
   missing-value handling as the gateway Connection tab (WEB-UIEX-03,
   WEB-UIEX-05).
3. **Create the sandbox (run in your local terminal).** The cell SHALL show
   OpenShell provider commands for GitHub and Jira that read their credentials
   from the reader's own environment, then an `openshell sandbox create` command
   that attaches those providers, sets the console's standard sandbox resource
   requests and limits (WEB-UIEX-05), and connects to the sandbox. The page SHALL
   NOT ask for, display, or store a GitHub or Jira credential.

#### Scenario: WEB-UDOC-06.01 -- Public placement unavailable

- GIVEN the availability response reports no eligible Kind, IBM Cloud public, or AWS public cluster
- WHEN a signed-in reader views Step 1
- THEN the create Run action SHALL be disabled
- AND the cell SHALL state that no public placement is currently available

#### Scenario: WEB-UDOC-06.02 -- Step 2 uses the Connection tab command

- GIVEN Step 1 produced a `Running` gateway with endpoint and OIDC values
- WHEN Step 2 renders
- THEN its `openshell gateway add` command SHALL equal the command the gateway Connection tab builds for the same gateway

### Requirement: WEB-UDOC-07 -- GitLab and Jira Sandbox Runbook

`/docs/runbooks/gitlab-jira` SHALL state "Requires Red Hat VPN" in plain text in
its subtitle and SHALL contain three steps.

1. **Choose or create a Red Hat VPN gateway (runs in the browser).** The step
   SHALL behave as WEB-UDOC-06 Step 1, except that an existing gateway is
   eligible only when its placement requires `network/vpn`, and a new gateway uses
   `selector: ["network/vpn", "provider/aws"]` (WEB-PLC-01). When the availability
   response reports no eligible VPN cluster, Run SHALL be disabled and the cell
   SHALL show the unavailability reason (WEB-PLC-04).
2. **Check the VPN and connect the OpenShell CLI (run in your local terminal).**
   The cell SHALL first show a `curl` command that requests the internal Red Hat
   GitLab URL from the shared external-link registry and explain that HTTP `200`
   or `302` means the reader is on the Red Hat VPN, and that any other result
   means the reader must connect to the VPN first. The cell SHALL then show the
   gateway connection commands of WEB-UDOC-06 Step 2.
3. **Create the sandbox (run in your local terminal).** The step SHALL behave as
   WEB-UDOC-06 Step 3, with GitLab in place of GitHub.

#### Scenario: WEB-UDOC-07.01 -- VPN placement unavailable

- GIVEN the availability response reports no eligible cluster for `["network/vpn", "provider/aws"]`
- WHEN a signed-in reader views Step 1
- THEN the create Run action SHALL be disabled
- AND the cell SHALL show why VPN placement is unavailable

### Requirement: WEB-UDOC-08 -- Install hsctl Page

`/docs/cli/install-hsctl` SHALL present macOS and Linux tabs. The selected tab
SHALL use the validated `os` search parameter with the values `macos` and
`linux` (WEB-DATA-02). When the parameter is absent or invalid, the page SHALL
select the tab for the operating system it detects from the browser
(`navigator.userAgentData.platform`, then `navigator.platform`, then
`navigator.userAgent`). When detection finds neither macOS nor Linux, the page
SHALL select Linux. A manual tab selection SHALL override detection. A note SHALL
name `hsctl-windows-amd64.exe` for Windows readers.

Each tab SHALL show one copyable command block that:

1. finds the CPU architecture on the reader's machine with `uname -m` and maps it
   to `amd64` or `arm64`, and stops with a clear message for any other value;
2. downloads the matching release asset (`hsctl-darwin-arm64`,
   `hsctl-darwin-amd64`, `hsctl-linux-amd64`, or `hsctl-linux-arm64`, CLI-REL-01)
   and `SHA256SUMS` from the same release (CLI-REL-03);
3. verifies the asset against `SHA256SUMS` and stops when verification fails;
4. makes the file executable, installs it as `hsctl` in `~/.local/bin`, and adds
   that directory to `PATH` for the current shell; and
5. runs `hsctl version` to confirm the installation.

The release location SHALL come from the optional BFF setting
`HSCTL_DOWNLOAD_BASE_URL`: the absolute `https` URL of one release's download
directory (for example
`https://github.com/openshift-online/hypershell/releases/download/<release-bundle-tag>`),
with no query or fragment. The BFF SHALL fail startup when the value is set and
invalid, and SHALL expose a valid value to the browser through the runtime
configuration allowlist as `hsctl_download_base_url` (WEB-DPLY-02). When the
setting is omitted, the commands SHALL read the release tag from an `HSCTL_RELEASE`
shell variable, and the page SHALL link to the HyperShell repository releases page
from the shared external-link registry and tell the reader to choose the release
that matches their HyperShell deployment. The page SHALL NOT link to a release
through GitHub's `latest` designation, because publication does not move it
(CLI-REL-03).

The page SHALL include an **Open hsctl terminal** button that opens the site-wide
terminal (WEB-TERM-01).

#### Scenario: WEB-UDOC-08.01 -- Detected macOS selects the macOS tab

- GIVEN a browser that reports a macOS platform and a URL without `os`
- WHEN the reader opens `/docs/cli/install-hsctl`
- THEN the macOS tab SHALL be selected
- AND selecting the Linux tab SHALL set `os=linux` and show the Linux command

#### Scenario: WEB-UDOC-08.02 -- Configured release location

- GIVEN `HSCTL_DOWNLOAD_BASE_URL` is a valid release download URL
- WHEN the Linux command renders
- THEN it SHALL download the asset and `SHA256SUMS` from that URL
- AND it SHALL verify the checksum before it makes the file executable

#### Scenario: WEB-UDOC-08.03 -- No configured release location

- GIVEN `HSCTL_DOWNLOAD_BASE_URL` is not set
- WHEN the page renders
- THEN the commands SHALL use the `HSCTL_RELEASE` shell variable for the release tag
- AND the page SHALL link to the repository releases page and explain how to choose the release

### Requirement: WEB-UDOC-09 -- Resource Limit Refusal Page

`/docs/refusals/resource-limits` is the direct-link help page for a sandbox
request that a platform gateway policy refused because of its CPU or memory
values (PLAT-PGP-14). Its copy SHALL use plain user language and SHALL NOT
contain the terms `interceptor`, `Cedar`, `ConfigMap`, `gRPC`, or `sidecar`.

The page SHALL read the optional query parameters `policy` and `gateway`. A value
SHALL be shown only when it is 1 to 128 characters of ASCII letters, digits,
`.`, `_`, or `-`; the page SHALL ignore any other value. Shown values SHALL appear
as text in a quiet metadata line near the title.

In order, the page SHALL show:

1. A short statement that a platform rule refused the sandbox because of its CPU
   or memory values, and that the refusal message names each value and its limit.
2. Above the fold, a copyable `openshell sandbox create` command with no resource
   settings, with the explanation that the gateway then applies the platform
   default resources, which every policy allows (PLAT-PGP-04, PLAT-PGP-08). It
   SHALL also show a second command with the console's standard resource values
   (WEB-UIEX-05) marked as values to change so they stay within the limit in the
   refusal message.
3. A Blocked and Allowed comparison of one refused and one allowed request.
4. A copyable exception request message for the platform team, prefilled with the
   shown `policy` and `gateway` values or clearly marked placeholders, and a link
   to the HyperShell support channel from the shared external-link registry
   (WEB-HOME-09).
5. A three-node diagram (your request, the platform rule check, and sandbox
   created or refused) rendered as SVG with `role="img"`, a `<title>`, a `<desc>`,
   and text or shape cues in addition to color.

#### Scenario: WEB-UDOC-09.01 -- Valid query context is shown as text

- GIVEN a reader opens `/docs/refusals/resource-limits?policy=sandbox-cpu-limit&gateway=team-a`
- WHEN the page renders
- THEN the metadata line SHALL show `sandbox-cpu-limit` and `team-a` as text
- AND the exception request message SHALL contain both values

#### Scenario: WEB-UDOC-09.02 -- Invalid query value is ignored

- GIVEN a reader opens the page with `policy=<script>x</script>`
- WHEN the page renders
- THEN the page SHALL NOT show the value anywhere
- AND the exception request message SHALL show the policy placeholder

#### Scenario: WEB-UDOC-09.03 -- Copy contains no internal terms

- GIVEN the localized message catalog for the refusal page
- WHEN it is checked for `interceptor`, `Cedar`, `ConfigMap`, `gRPC`, and `sidecar`
- THEN no message SHALL contain any of them

### Requirement: WEB-UDOC-10 -- Command Blocks and Content Safety

Every command block on a docs page SHALL use the shared command block component
and shell syntax highlighting of the gateway Connection tab (WEB-UIEX-03), which
renders React elements. Docs pages SHALL NOT add a second highlighter or
tokenizer. Docs pages SHALL NOT use React's raw-HTML prop, DOM HTML properties or
methods, or any other HTML-injection path, and SHALL pass
`make check-forbidden-dom-injection`. Every value from an API response, the URL,
or reader input SHALL be encoded as a safe shell argument before it enters a
command. Each Copy action SHALL have a localized accessible name and visible
success or failure feedback.

#### Scenario: WEB-UDOC-10.01 -- Shell metacharacters stay inert

- GIVEN a gateway name or query value that contains `$(`, a backtick, `;`, or a quote
- WHEN a command that uses the value renders and is copied
- THEN the value SHALL appear in the command as one safely quoted shell argument

### Requirement: WEB-UDOC-11 -- Accessibility and Motion

Docs pages SHALL meet UIS-A11Y. Runbook steps SHALL use an ordered list structure
with a visible label for each step. Tabs, cards, cells, and Copy actions SHALL be
fully operable by keyboard with a visible focus indicator that has at least 3:1
contrast. Lifecycle changes in a cell SHALL be announced through a polite live
region and SHALL NOT move focus. Status SHALL be conveyed by text as well as
color. Animation and smooth scrolling SHALL run only under
`prefers-reduced-motion: no-preference` (UIS-A11Y-08).

#### Scenario: WEB-UDOC-11.01 -- Status changes are announced without moving focus

- GIVEN keyboard focus is on the Step 1 Run action
- WHEN the created gateway moves from `Provisioning` to `Running`
- THEN a screen reader SHALL hear the new status
- AND keyboard focus SHALL stay where it was

### Requirement: WEB-UDOC-12 -- Boundary, Probes, and Localization

Docs workflows SHALL keep external effects behind application-owned ports and
SHALL keep React, TanStack Query, the BFF, and the generated SDK outside the
boundary (UIS-HEX). Gateway data in runbook cells SHALL use TanStack Query with
explicit freshness and SHALL be invalidated after a successful create
(WEB-DATA-01, WEB-DATA-03). Docs pages SHALL publish typed domain facts through
the fan-out probe port (UIS-OBS) for docs page viewed, sign-in prompt shown, and
runbook step run started, succeeded, and failed. Probe payloads SHALL use route
templates and SHALL NOT contain query values, gateway or policy names, commands,
or command output. All user-facing copy SHALL be FormatJS messages in
`components/web-console/locales/en.json` (WEB-I18N-01, UIS-I18N).

#### Scenario: WEB-UDOC-12.01 -- Probe payload excludes reader context

- GIVEN a reader opens `/docs/refusals/resource-limits?policy=sandbox-cpu-limit&gateway=team-a`
- WHEN the docs page viewed fact is published
- THEN its payload SHALL contain the route template `/docs/refusals/resource-limits`
- AND it SHALL NOT contain `sandbox-cpu-limit` or `team-a`

### Requirement: WEB-UDOC-13 -- Verification

Each docs page SHALL have Storybook stories for its signed-out, signed-in, and
error states, and runbook cells SHALL have stories for idle, running, `Running`,
placement unavailable, and failed states. The Install `hsctl` page SHALL have
stories with and without `HSCTL_DOWNLOAD_BASE_URL`. The production BFF smoke test
(WEB-QUAL-03) SHALL request every docs route directly without a session and
SHALL confirm the SPA document, and SHALL confirm that `/gateways` and `/api/*`
still require a session. Playwright journeys SHALL cover a signed-out refusal
link, sign-in return, and a signed-in Step 1 run. When approved mockups are added
under `specs/web-console/mockups/docs/`, these pages SHALL pass the web-console
completion gate, including the parity checks of UIS-VER-10.

#### Scenario: WEB-UDOC-13.01 -- Smoke test covers public docs and protected routes

- GIVEN the built production BFF with OIDC configured
- WHEN the smoke test runs without a session
- THEN each docs route SHALL return the SPA document
- AND `/gateways` SHALL redirect to sign in and `/api/hypershell/v1/gateways` SHALL be refused
