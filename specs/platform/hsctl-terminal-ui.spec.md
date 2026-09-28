# hsctl Terminal UI Specification

**Date:** 2026-09-27
**Status:** Draft
**Related:** [`data-model.spec.md`](data-model.spec.md), [`gateway-phase-vocabulary.spec.md`](gateway-phase-vocabulary.spec.md), [`gateway-version-selection.spec.md`](gateway-version-selection.spec.md), [`../web-console/architecture.spec.md`](../web-console/architecture.spec.md), [`../security/rbac-enforcement.spec.md`](../security/rbac-enforcement.spec.md)

## Purpose

`hsctl ui` is an interactive, full-screen terminal interface for the HyperShell
API server, built into the existing `hsctl` binary. It lets an operator watch
Gateways, ManagedClusters, GatewayReleases, and GatewayNetworks refresh in
place, inspect one resource in detail, provision a Gateway, and delete a
Gateway, without leaving the terminal or composing flags. It reuses the CLI's
saved login, token refresh, proxy, and TLS settings, and it talks to the API
server only through the same REST API the other `hsctl` commands use. Data
freshness comes from bounded polling of the active view.

## Scope Boundary

In scope: the `hsctl ui` command, its resource views, polling, detail view,
gateway provisioning form, and gateway deletion.

Not in this version: editing existing resources other than deletion, creating
ManagedClusters, GatewayReleases, GatewayNetworks, roles, role bindings, users,
or service accounts, and metrics dashboards. The `create`, `apply`, and
`delete` commands and the web console remain the interfaces for those
workflows.

## Requirements

### Requirement: TUI-01 - Command Entry Point

`hsctl` SHALL provide a `ui` subcommand, with the alias `tui`, that starts the
terminal interface. The command SHALL load the saved `hsctl` configuration and
build its API connection exactly as the other `hsctl` commands do, including
access-token refresh, proxy environment variables, and the `insecure` setting.

When there is no saved login, or the saved login cannot be refreshed, the
command SHALL exit with a non-zero status and the same "not logged in" error
the other commands print, without entering full-screen mode.

When standard input or standard output is not a terminal, the command SHALL
exit with a non-zero status and an error stating that `hsctl ui` requires an
interactive terminal.

On exit by `q` or `ctrl+c`, and on any internal error, the command SHALL
restore the terminal to its prior mode (screen, cursor, and echo) before the
process ends.

#### Scenario: Start without a login

- GIVEN no `hsctl` configuration file exists
- WHEN the user runs `hsctl ui`
- THEN the command SHALL print `not logged in, server URL isn't set, run the 'login' command`
- AND it SHALL exit with status 1 without switching to the alternate screen

#### Scenario: Output redirected

- GIVEN the user is logged in
- WHEN the user runs `hsctl ui > out.txt`
- THEN the command SHALL exit with status 1
- AND the error SHALL state that an interactive terminal is required

#### Scenario: Quit restores the terminal

- GIVEN the terminal interface is running
- WHEN the user presses `q`
- THEN the process SHALL exit with status 0
- AND the shell prompt SHALL appear on the original screen with echo enabled

### Requirement: TUI-02 - Resource Views and Navigation

The interface SHALL provide four resource views, each a table with one row per
resource the caller is allowed to list:

| Key | Command | View | Columns |
|---|---|---|---|
| `1` | `:gateways`, `:gw` | Gateways (initial view) | Name, Cluster, Phase, Status, Sandboxes, Release, Age |
| `2` | `:clusters`, `:mc` | ManagedClusters | Name, Provider, Region, Status, Last Seen, Age |
| `3` | `:releases`, `:rel` | GatewayReleases | Name, Image, Rollout Strategy, Status, Age |
| `4` | `:networks`, `:net` | GatewayNetworks | Name, Topology, Tunnel Mode, Hub Gateway, Status, Age |

The Gateways view SHALL show the ManagedCluster name in the Cluster column for
a non-empty `cluster_id`, `Hub cluster` for an empty `cluster_id`, and the raw
identifier only when the cluster cannot be resolved, marked as unresolved. The
Release column SHALL show the GatewayRelease name for a non-empty `release_id`
and `default` for an empty one.

Pressing `:` SHALL open a command bar that accepts the view commands above and
`:q` to quit. An unknown command SHALL leave the current view unchanged and
show an error in the status line.

Arrow keys and `j`/`k` SHALL move the selection. The selection SHALL follow the
selected resource by ID across refreshes; when that resource disappears, the
selection SHALL move to the row now at the same position.

The footer SHALL list the keys available in the current context, and `?` SHALL
open a help overlay listing every key binding.

#### Scenario: Switch to clusters

- GIVEN the Gateways view is active
- WHEN the user presses `2`
- THEN the ManagedClusters view SHALL be shown
- AND polling SHALL move to the ManagedClusters collection

#### Scenario: Hub placement label

- GIVEN a gateway `gw-a` has an empty `cluster_id`
- WHEN the Gateways view renders
- THEN the Cluster column for `gw-a` SHALL read `Hub cluster`

#### Scenario: Selection survives reordering

- GIVEN `gw-b` is selected at row 2
- WHEN a refresh returns a new gateway that sorts before `gw-b`
- THEN `gw-b` SHALL remain selected at row 3

### Requirement: TUI-03 - Polling and Freshness

The interface SHALL refresh the active view by polling the REST list endpoint.
The default interval SHALL be 5 seconds, configurable with `--refresh
<duration>`; values below 2 seconds SHALL be rejected at startup with a
non-zero exit and an error naming the minimum.

Each refresh SHALL walk list pages of size 100 until a page returns fewer items
than requested, and SHALL stop after 50 pages. When it stops at that bound, the
view SHALL indicate that the list is truncated. At most one refresh per view
SHALL be in flight; a tick that fires while a refresh is running SHALL be
skipped. Views that are not active SHALL NOT be polled, except that the
Gateways view SHALL refresh the ManagedCluster and GatewayRelease name lookups
it displays at most once every 60 seconds.

After a failed refresh, the view SHALL keep showing the last successful data,
mark it as stale in the header, and double the delay before the next attempt,
up to 60 seconds. The first successful refresh SHALL clear the stale marker
and restore the configured interval. `r` SHALL trigger an immediate refresh
and reset the backoff.

The header SHALL show the API server URL, the caller's identity (username or
service-account client ID from the token), the active view, the item count,
and the time since the last successful refresh.

Before each request, the interface SHALL refresh the access token when it is
near expiry, using the saved refresh token. When the refresh token is rejected,
polling SHALL stop and the interface SHALL show `session expired, run 'hsctl
login'` until the user quits.

#### Scenario: API unavailable

- GIVEN the Gateways view shows 3 gateways from a successful refresh
- WHEN the next two refreshes fail with a connection error
- THEN the view SHALL still show the 3 gateways
- AND the header SHALL mark the data as stale with the time of the last success
- AND the next attempt SHALL be scheduled 20 seconds after the second failure with the default interval

#### Scenario: Recovery clears staleness

- GIVEN the data is marked stale
- WHEN a refresh succeeds
- THEN the stale marker SHALL be removed
- AND the next refresh SHALL be scheduled at the configured interval

#### Scenario: Slow API does not stack requests

- GIVEN a gateway list request takes 12 seconds
- WHEN two 5-second ticks fire during that request
- THEN no additional gateway list request SHALL be sent until the first completes

### Requirement: TUI-04 - Gateway Phase Presentation

The Phase column SHALL display the Gateway `phase` text exactly as returned by
the API. The canonical phases from `gateway-phase-vocabulary.spec.md` SHALL be
styled distinctly: `Running` as healthy, `Pending` and `Provisioning` as in
progress, `Degraded` as warning, and `Failed` as error. Any other value,
including an empty phase, SHALL be shown with a neutral style, with an empty
phase displayed as `-`. Phase meaning SHALL NOT be conveyed by color alone.

When the `NO_COLOR` environment variable is set to a non-empty value, the
interface SHALL render without color.

#### Scenario: Unknown phase

- GIVEN a gateway returns phase `Upgrading`
- WHEN the Gateways view renders
- THEN the Phase column SHALL read `Upgrading` in the neutral style

### Requirement: TUI-05 - Filtering

Pressing `/` SHALL open a filter input for the active view. The filter SHALL
match rows whose name or ID contains the input as a case-insensitive literal
substring; characters such as `%`, `_`, `*`, and `'` SHALL match themselves.
Filtering SHALL apply to the data already fetched and SHALL NOT send an API
request. The header SHALL show both the matching and total counts while a
filter is active. `esc` SHALL clear the filter. The filter SHALL persist across
refreshes of the same view and SHALL be cleared when the view changes.

#### Scenario: Literal underscore

- GIVEN gateways `prod_a` and `prodXa` exist
- WHEN the user filters by `prod_`
- THEN only `prod_a` SHALL be shown

### Requirement: TUI-06 - Resource Detail

Pressing `enter` on a row SHALL open a scrollable detail view of that resource
showing every field returned by the API get endpoint, formatted as indented
YAML with keys in API order. The detail view SHALL refresh on the same polling
schedule as its collection. When the resource returns HTTP 404, the detail view
SHALL state that the resource no longer exists and offer `esc` to return.

For a Gateway, the detail view SHALL also show the openshell connection
instructions produced by `hsctl get gateway <id> --show-connection`, or a note
that connection details are not yet available while the gateway is not
`Running`.

`y` SHALL copy the resource ID to the system clipboard when one is available
through the terminal (OSC 52), and SHALL report in the status line whether the
copy was sent. `esc` SHALL return to the collection with the same row
selected.

#### Scenario: Gateway deleted while viewing

- GIVEN the user is viewing gateway `gw-a` in detail
- WHEN another user deletes `gw-a` and the next refresh returns 404
- THEN the detail view SHALL state that `gw-a` no longer exists

### Requirement: TUI-07 - Gateway Provisioning

Pressing `n` in the Gateways view SHALL open a provisioning form with exactly
three inputs:

1. **Name** - required; leading and trailing whitespace SHALL be trimmed
   before validation and submission, and an empty result SHALL block
   submission with an inline error.
2. **Placement** - a single-select list whose first and initially selected
   option is `Hub cluster (default)`, followed by the ManagedClusters the
   caller can list, each shown by name with provider and region. Typing SHALL
   narrow the list by case-insensitive literal substring of the name.
3. **Release** - a single-select list whose first and initially selected
   option is `Platform default`, followed by the GatewayReleases the caller
   can list, each shown by name and image.

The form SHALL NOT collect namespace, phase, status, image, supervisor image,
OIDC, route, credential driver, TLS, DNS, or service-type values. On submit,
the interface SHALL send one `POST /api/hypershell/v1/gateways` request whose
body contains `name`, `cluster_id` (empty for `Hub cluster (default)`,
otherwise the selected ManagedCluster ID), and `release_id` (empty for
`Platform default`, otherwise the selected GatewayRelease ID), and no other
fields.

While a submission is in flight, further submits SHALL be ignored. On HTTP 201
the form SHALL close, the Gateways view SHALL refresh immediately, the new
gateway SHALL be selected, and the status line SHALL confirm the creation. On
any other response, the form SHALL stay open with every input unchanged and
show the API error `reason`; for HTTP 403 it SHALL add that provisioning
requires the `gateway:creator` role. `esc` SHALL close the form without sending
a request.

When the ManagedCluster or GatewayRelease list fails to load, the form SHALL
keep `Hub cluster (default)` and `Platform default` selectable, state which
list is unavailable, and offer `ctrl+r` to retry loading it without clearing
the other inputs.

#### Scenario: Provision on the hub cluster

- GIVEN the provisioning form is open
- WHEN the user enters `  demo  `, keeps `Hub cluster (default)` and `Platform default`, and submits
- THEN the request body SHALL be `{"name":"demo","cluster_id":"","release_id":""}`
- AND on HTTP 201 the new gateway SHALL be selected in the Gateways view

#### Scenario: Provision on a managed cluster with a release

- GIVEN ManagedCluster `mc-east` with ID `c1` and GatewayRelease `openshell-0.9` with ID `r1` exist
- WHEN the user enters `demo`, selects `mc-east` and `openshell-0.9`, and submits
- THEN the request body SHALL be `{"name":"demo","cluster_id":"c1","release_id":"r1"}`

#### Scenario: Caller lacks the creator role

- GIVEN the caller does not have `gateway:creator`
- WHEN the user submits a valid form
- AND the API returns HTTP 403
- THEN the form SHALL remain open with its inputs unchanged
- AND it SHALL show the API reason and that `gateway:creator` is required

#### Scenario: Double submit

- GIVEN the user has submitted the form and the request is in flight
- WHEN the user presses the submit key again
- THEN no second create request SHALL be sent

#### Scenario: Cluster list unavailable

- GIVEN the ManagedCluster list request fails
- WHEN the provisioning form opens
- THEN `Hub cluster (default)` SHALL be selectable
- AND the form SHALL state that managed clusters could not be loaded and offer `ctrl+r` to retry

### Requirement: TUI-08 - Gateway Deletion

Pressing `d` on a selected gateway SHALL open a confirmation that names the
gateway and requires the user to type the gateway name exactly. Until the typed
value matches, confirmation SHALL be disabled. On confirmation the interface
SHALL send one `DELETE /api/hypershell/v1/gateways/{id}` request, and on HTTP
204 it SHALL report the deletion in the status line and refresh the view. On
any other response it SHALL show the API error `reason` and leave the gateway
listed. `esc` SHALL cancel without sending a request. Deletion SHALL NOT be
offered in any other view.

#### Scenario: Confirmed deletion

- GIVEN gateway `demo` is selected
- WHEN the user presses `d`, types `demo`, and confirms
- THEN one delete request SHALL be sent for that gateway's ID
- AND on HTTP 204 the status line SHALL report that `demo` was deleted

#### Scenario: Mistyped confirmation

- GIVEN the delete confirmation for `demo` is open
- WHEN the user types `dem`
- THEN confirmation SHALL be disabled and no request SHALL be sent

### Requirement: TUI-09 - Terminal Handling and Secrets

The interface SHALL re-render to fit the terminal on resize. Below 80 columns
or 20 rows it SHALL show only a message stating the minimum size until the
terminal is enlarged. It SHALL NOT write log output to the terminal while
running.

The interface SHALL NOT display, log, or copy the access token or refresh
token, including inside error messages and resource detail.

#### Scenario: Small terminal

- GIVEN the interface is running
- WHEN the terminal is resized to 70x30
- THEN the screen SHALL show that at least 80x20 is required
- AND the previous view SHALL return when the terminal is enlarged
