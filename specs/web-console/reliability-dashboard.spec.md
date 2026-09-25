# Reliability Dashboard

**Status:** Draft
**Applies to:** `packages/operational-dashboard-ui`, `components/web-console` SPA and BFF
**Tracks:** [HYPERSHELL-285](https://redhat.atlassian.net/browse/HYPERSHELL-285)

## Purpose

Provide a **widgetized reliability dashboard** in the HyperShell web console where administrators can assess API reliability at a glance. Version 1 surfaces API request rate, 5xx error rate, and median latency with current values and 24-hour hourly trend sparklines.

The reliability dashboard lives in the same reusable package as the operational dashboard (`@openshift-online/hypershell-operational-dashboard-ui`) to share hexagonal ports, widgetized-dashboard infrastructure, localization patterns, and presentation helpers - while remaining a **separate route and layout** so reliability metrics do not clutter the operational overview.

Metric collection, PromQL, and BFF contracts are defined in `platform/api-reliability-metrics.spec.md`. Cross-dashboard navigation uses PatternFly **secondary horizontal navigation** so each dashboard keeps a distinct URL ([PatternFly Navigation - secondary horizontal](https://www.patternfly.org/components/navigation/design-guidelines#secondary-horizontal-navigation)).

Authentication failure rate and login failure rate are out of scope for version 1.

### Relationship to the operational dashboard

| Concern | Operational (`/dashboard`) | Reliability (`/dashboard/reliability`) |
| --- | --- | --- |
| Package | `operational-dashboard-ui` | Same package |
| Page export | `OperationalDashboardPage` | `ReliabilityDashboardPage` |
| Metrics port method | `getOperationalMetrics` | `getReliabilityMetrics` |
| Layout persistence key | `hypershell.operational-dashboard.layout.*` | `hypershell.reliability-dashboard.layout.*` (separate) |
| Secondary nav | Shared masthead / page-header nav | Shared masthead / page-header nav |
| Admin access | `platform:admin` (OP-DASH-04) | Same roles (REL-DASH-04) |

## Requirements

### Requirement: REL-DASH-01 -- Package Placement and Exports

The `@openshift-online/hypershell-operational-dashboard-ui` package SHALL host the reliability dashboard presentation layer alongside the operational dashboard.

The package SHALL export at minimum:

- `ReliabilityDashboardPage` - the full reliability dashboard page
- `createReliabilityDashboardOperations` (or an extended `createDashboardOperations` that also exposes reliability workflows) - application workflow entry for reliability metrics
- Types for reliability metrics payloads (MAY reuse `OperationalMetric` / trend shapes)
- A reliability metrics query key and the same refresh interval constant as the operational dashboard (`operationalDashboardRefreshMilliseconds`, 15 minutes)
- Reliability localization messages in the package `messages` catalog
- A `mockReliabilityDashboardMetrics` fixture for Storybook

Presentation code SHALL NOT call `fetch`, the SDK, or BFF routes directly. Reliability loading SHALL go through the host-injected application port.

#### Scenario: Host imports the reliability page

- GIVEN the web console depends on `@openshift-online/hypershell-operational-dashboard-ui`
- WHEN the host route module renders `ReliabilityDashboardPage` inside `DashboardUiProvider`
- THEN the reliability dashboard SHALL mount without importing React Router, the SDK, or Fastify directly

---

### Requirement: REL-DASH-02 -- Hexagonal Application Boundary

The reliability dashboard SHALL follow the narrow hexagonal UI boundary defined in `standards/ui/hexagonal-architecture.spec.md`.

- The host-injected control plane SHALL expose `getReliabilityMetrics(signal)` as the driven-port method for reliability metrics.
- Presentation SHALL call that method through `useDashboardUi()` (or an equivalent reliability operations hook from the same provider).
- Workflow probes SHALL be published for reliability load with outcomes `started`, `succeeded`, `failed`, or `cancelled`, following the operational dashboard probe pattern (OP-DASH-02).

#### Scenario: AbortSignal propagates for reliability metrics

- GIVEN a TanStack Query request is cancelled
- WHEN `getReliabilityMetrics` is in flight
- THEN the `AbortSignal` SHALL be passed to the control-plane method
- AND a completed probe with outcome `cancelled` SHALL be published

---

### Requirement: REL-DASH-03 -- Host Composition

The web console host SHALL extend dashboard composition so `ReliabilityDashboardPage` receives a control-plane adapter that implements `getReliabilityMetrics` by calling BFF `GET /api/metrics/api-reliability` (ARM-03 / ARM-04).

#### Scenario: Production host uses the API adapter for reliability

- GIVEN the web console is running against the HyperShell API
- WHEN `ReliabilityDashboardPage` loads metrics
- THEN the host adapter SHALL fetch `GET /api/metrics/api-reliability` and map the response per ARM-04

---

### Requirement: REL-DASH-04 -- Administrator Access Control

Access to the reliability dashboard SHALL use the **same** dashboard-admin gate as the operational dashboard (OP-DASH-04): users with the `platform:admin` realm role (effective `platform:admin` RoleBinding, including JWT-synced `platform:admin` realm role). The legacy Keycloak realm role `hypershell-admins` SHALL NOT grant access on its own.

The SPA route module for `/dashboard/reliability` SHALL wrap `ReliabilityDashboardPage` in `RequireDashboardAdmin` with the same empty-state behavior as `/dashboard`.

When OIDC is enabled, the BFF SHALL enforce the same role requirement for browser navigations to `/dashboard/reliability` and for `GET /api/metrics/api-reliability`. Authenticated non-admins SHALL receive HTTP `403` on the metrics route. Unauthenticated callers SHALL receive HTTP `401` or the standard BFF re-authentication response. When OIDC is disabled (no-auth dev mode), page and metrics routes SHALL remain open, matching operational dashboard behavior.

#### Scenario: Non-admin is turned away from /dashboard/reliability

- GIVEN OIDC is enabled and the signed-in user has only `hypershell-users`
- WHEN the user navigates to `/dashboard/reliability`
- THEN the BFF SHALL redirect away from the reliability route
- AND the SPA SHALL render the access-denied empty state if the route still mounts

#### Scenario: Platform admin can open the reliability dashboard

- GIVEN OIDC is enabled and the signed-in user has `platform:admin`
- WHEN the user navigates to `/dashboard/reliability`
- THEN the BFF SHALL serve `index.html` with HTTP `200`
- AND `ReliabilityDashboardPage` SHALL render

#### Scenario: Non-admin cannot fetch API reliability metrics

- GIVEN OIDC is enabled and the signed-in user has only `hypershell-users`
- WHEN the user sends `GET /api/metrics/api-reliability`
- THEN the BFF SHALL respond with HTTP `403`

---

### Requirement: REL-DASH-05 -- SPA and BFF Route Surfaces

The web console SPA SHALL expose the reliability dashboard at `/dashboard/reliability` via a React Router route module that renders `ReliabilityDashboardPage`.

`route-contract.json` SHALL declare a reliability dashboard path (for example `"dashboardReliability": "dashboard/reliability"`) and include `/dashboard/reliability` in `directNavigationExamples`. The BFF SHALL treat `/dashboard/reliability` as a valid application route that returns `index.html` for direct navigation and refresh.

The dashboard-dedicated host root (`/` on `dashboard.*` hosts) SHALL continue to render the **operational** dashboard (OP-DASH-05). Reliability SHALL remain available via `/dashboard/reliability` on both console and dashboard hosts.

#### Scenario: Direct navigation to /dashboard/reliability

- GIVEN an authenticated dashboard administrator
- WHEN the browser requests `GET /dashboard/reliability`
- THEN the BFF SHALL respond with `index.html` and HTTP `200`
- AND the SPA SHALL render `ReliabilityDashboardPage`

#### Scenario: Dashboard host root stays operational

- GIVEN the browser hostname is `dashboard.hypershell.localhost`
- WHEN the user opens `/`
- THEN the SPA SHALL render `OperationalDashboardPage`
- AND reliability SHALL remain reachable at `/dashboard/reliability`

---

### Requirement: REL-DASH-06 -- Secondary Horizontal Navigation Between Dashboards

Both `OperationalDashboardPage` and `ReliabilityDashboardPage` SHALL render PatternFly **secondary horizontal navigation** that lets administrators move between the two dashboards.

The navigation SHALL:

- Appear in the page header region beneath (or as part of) the shared dashboards chrome, consistent with PatternFly secondary horizontal navigation usage
- Expose exactly two items in version 1: localized **Operational** (href `/dashboard`) and localized **Reliability** (href `/dashboard/reliability`)
- Mark the item matching the current route as selected
- Use distinct URLs for each item (not in-page Tabs that keep a single URL)
- Update the visible page title to reflect the selected item (for example, page title **Operational** or **Reliability**, or an equivalent localized pair under a shared dashboards heading)
- Be keyboard-accessible and expose an accessible name for the navigation region

The navigation SHALL be visible only to users who can access the dashboards (same admin gate). Non-admin access-denied empty states SHALL NOT show the secondary nav as an alternate entry path.

PatternFly Tabs SHALL NOT be used as the primary cross-dashboard switcher. Tabs remain appropriate inside a single page for local perspective changes; these are peer destinations with distinct routes.

#### Scenario: Operator moves from operational to reliability

- GIVEN an authenticated dashboard administrator is on `/dashboard`
- WHEN they activate the **Reliability** secondary nav item
- THEN the SPA SHALL navigate to `/dashboard/reliability`
- AND `ReliabilityDashboardPage` SHALL render
- AND the **Reliability** nav item SHALL be selected

#### Scenario: Operator moves from reliability to operational

- GIVEN an authenticated dashboard administrator is on `/dashboard/reliability`
- WHEN they activate the **Operational** secondary nav item
- THEN the SPA SHALL navigate to `/dashboard`
- AND `OperationalDashboardPage` SHALL render
- AND the **Operational** nav item SHALL be selected

#### Scenario: Selected state matches the route on deep link

- GIVEN an authenticated dashboard administrator opens `/dashboard/reliability` directly
- WHEN the page renders
- THEN the **Reliability** secondary nav item SHALL be selected
- AND the **Operational** item SHALL not be selected

---

### Requirement: REL-DASH-07 -- Connected Metrics and Widgets

Version 1 SHALL connect the following metrics from the `api-reliability` source (ARM-04):

| Widget type | Metric ID | Presentation |
| --- | --- | --- |
| `reliability-summary` | `api-request-rate`, `api-error-rate`, `api-latency` | Summary card listing current values for all three. When a metric's `hourlyTrend` has at least two points and `getMetricTrendChange` reports a change of at least 5%, the row SHALL show the same up/down trend indicator used on the operational dashboard summary cards (OP-DASH-13) |
| `api-request-rate` | `api-request-rate` | Current rate plus 24-hour hourly usage trend graph when `hourlyTrend` has at least two points |
| `api-error-rate` | `api-error-rate` | Current 5xx error percent plus 24-hour hourly usage trend graph when `hourlyTrend` has at least two points |
| `api-latency` | `api-latency` | Current median latency plus 24-hour hourly usage trend graph when `hourlyTrend` has at least two points |

Widgets without a connected metric SHALL remain on the grid and render the localized metric-unavailable empty state (same pattern as OP-DASH unavailable widgets).

Trend graphs SHALL reuse the shared sparkline presentation used by the operational dashboard where practical (`TrendSparklineChart` or equivalent). Units on tooltips and summary rows SHALL match ARM-04 (`req/s`, `%`, `sec`).

#### Scenario: Summary lists all three current values

- GIVEN `api-request-rate`, `api-error-rate`, and `api-latency` are present
- WHEN the `reliability-summary` widget renders
- THEN it SHALL show the current request rate, error rate, and median latency with their units

#### Scenario: Summary shows trend arrows when hourly change meets threshold

- GIVEN `api-request-rate.hourlyTrend.points` has at least two entries whose first-to-last change is at least 5%
- WHEN the `reliability-summary` widget renders
- THEN the request-rate row SHALL show the localized increase or decrease trend indicator beside the current value
- AND rows whose hourly change is below 5% or that lack `hourlyTrend` SHALL omit the indicator

#### Scenario: Request-rate widget shows hourly sparkline

- GIVEN `api-request-rate.hourlyTrend.points` has at least two entries
- WHEN the `api-request-rate` widget renders
- THEN it SHALL show the current value
- AND it SHALL show a usage trend sparkline for the 24-hour series

#### Scenario: Missing trend omits sparkline only

- GIVEN `api-latency` has a current `value` and no `hourlyTrend`
- WHEN the `api-latency` widget renders
- THEN it SHALL show the current median latency
- AND it SHALL omit the sparkline rather than showing a synthetic flat series

---

### Requirement: REL-DASH-08 -- Default Layout and Widgetized Behavior

The reliability dashboard SHALL use PatternFly widgetized-dashboard (`GridLayout`, widget drawer, add/remove/rearrange) with a dedicated default layout template.

The default layout SHALL include:

| Widget type | Placement (xl default) |
| --- | --- |
| `reliability-summary` | Full-width or leading summary position |
| `api-request-rate` | Trend graph tile |
| `api-error-rate` | Trend graph tile |
| `api-latency` | Trend graph tile |

Exact column coordinates MAY be chosen during implementation; the default SHALL place the summary above or beside the three trend widgets so all four are visible without opening the drawer.

Users SHALL be able to add widgets from the drawer, drag to rearrange, remove widgets, and reset to the reliability default template. Layout persistence SHALL use a **separate** `localStorage` key from the operational dashboard (for example `hypershell.reliability-dashboard.layout.v1`) so customizing one dashboard does not rewrite the other.

The page header SHALL include last-refreshed timestamp, manual refresh, reset-to-default, and add-widgets controls analogous to OP-DASH-10.

#### Scenario: Reset restores the reliability default layout

- GIVEN the user has rearranged reliability widgets
- WHEN they activate reset to default
- THEN the grid SHALL return to the reliability default layout template
- AND the updated layout SHALL be persisted under the reliability layout key

#### Scenario: Operational layout is unaffected

- GIVEN the user customizes the reliability layout
- WHEN they navigate to `/dashboard`
- THEN the operational layout SHALL load from its own persistence key unchanged

---

### Requirement: REL-DASH-09 -- Metrics Refresh Policy

The reliability dashboard SHALL load metrics through TanStack Query with `reliabilityDashboardRefreshMilliseconds` equal to `operationalDashboardRefreshMilliseconds` (15 minutes).

Manual refresh SHALL re-fetch the `api-reliability` source immediately. Refresh-in-flight, partial-failure, and total-failure presentation SHALL follow the operational dashboard patterns (OP-DASH-09): spinner on initial load only; warning alert on partial failure; danger empty state on total initial-load failure; keep last successful values for failed sources on refresh when applicable.

#### Scenario: Manual refresh re-queries Prometheus via BFF

- GIVEN the reliability dashboard has loaded successfully
- WHEN the operator activates Refresh
- THEN the host adapter SHALL call `GET /api/metrics/api-reliability` again
- AND widgets SHALL update from the new payload when the request succeeds

---

### Requirement: REL-DASH-10 -- Localization and Accessibility

All user-visible reliability dashboard strings SHALL come from the package `react-intl` message catalog. Interactive controls and trend icons SHALL expose localized accessible names.

The secondary navigation region SHALL have an accessible name that distinguishes it from other navigation on the page.

#### Scenario: Metric unavailable uses localized copy

- GIVEN `api-error-rate` is omitted from the metrics payload
- WHEN the `api-error-rate` widget renders
- THEN it SHALL show the localized metric-unavailable empty state

---

### Requirement: REL-DASH-11 -- Tests and Storybook

The repository SHALL include:

- Unit tests for secondary nav selection and route targets on both dashboard pages
- Unit tests for adapter mapping (covered jointly with ARM-07)
- Unit or component tests for reliability summary and trend widgets (present value; sparkline when trend exists; unavailable when metric omitted)
- Storybook stories for default, loading, partial-load-warning, and total-initial-load-error states using a mock reliability control plane

BFF and SPA access-control tests SHALL cover `/dashboard/reliability` and `GET /api/metrics/api-reliability` alongside existing operational dashboard admin tests.

#### Scenario: Storybook shows sparklines with mock hourly trends

- GIVEN `mockReliabilityDashboardMetrics` includes `hourlyTrend` points on all three metrics
- WHEN the default reliability Storybook story renders
- THEN the three trend widgets SHALL show sparklines

---

## Non-Goals

- Authentication failure rate and login failure rate
- Embedding reliability widgets on `/dashboard`
- A separate pnpm package for reliability UI in version 1
- Changing the dashboard-host root (`/`) to the reliability page
- Per-route latency or error drill-down pages
- Using PatternFly Tabs as the cross-dashboard primary switcher
