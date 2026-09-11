# Operational Dashboard

**Status:** Active
**Applies to:** `packages/operational-dashboard-ui`, `components/web-console` SPA and BFF, `packages/gateway-management-ui` (display-status aggregation)

## Purpose

Provide a widgetized operational dashboard in the HyperShell web console where administrators can assess fleet health at a glance. The dashboard composes summary and detail widgets in a customizable grid layout. Live data is loaded through a narrow application port (`DashboardControlPlane`) implemented by the web-console host; widgets without a connected source remain on the page and render a localized unavailable state rather than being hidden.

This specification covers the reusable `operational-dashboard-ui` package, the host adapter that loads operational metrics from BFF Prometheus proxy routes, admin-only access controls, SPA and BFF route surfaces, layout persistence, and the widget catalog. Platform inventory metrics (managed clusters and managed databases) are defined in `platform/platform-inventory.spec.md`. Prometheus gateway phase counts (`hypershell_gateways_total`, BFF `GET /api/metrics/gateways`) are defined in `platform/gateway-metrics-dashboard.spec.md` and consumed by this dashboard for `provisioned-gateways` and `gateway-status`. The embeddable `GatewayMetricsDashboard` component remains a separate surface.

**Data-source direction:** Aggregate operational dashboard widgets SHALL prefer Prometheus-backed BFF routes over direct HyperShell REST list pagination. The API server exposes fleet-wide gauges from database state on its `/metrics` scrape; the BFF queries Prometheus and returns JSON to the browser adapter. HyperShell REST List APIs remain authoritative for resource collection pages (for example, the gateway table) but are not the preferred source for dashboard totals.

### Relationship to gateway metrics

| Concern | Operational dashboard (this spec) | Gateway metrics dashboard |
| --- | --- | --- |
| Primary route | `/dashboard` and `dashboard.*` host root (`/`) | `GatewayMetricsDashboard` component (embeddable) |
| Gateway counts source | BFF `GET /api/metrics/gateways` (Prometheus `hypershell_gateways_total`) | Same BFF route |
| Scope | Fleet-wide database aggregate (dashboard-operator access only) | Fleet-wide database aggregate (dashboard-operator access only) |
| Status model | Display buckets: `healthy`, `provisioning`, `degraded`, `failed` (mapped from phase labels) | Lifecycle phases: `Pending`, `Provisioning`, `Running`, `Degraded`, `Failed` |

The two surfaces MAY coexist. They share the same Prometheus-backed gateway count route but present different widgets and layouts.

## Requirements

### Requirement: OP-DASH-01 -- Reusable UI Package

The repository SHALL publish a private pnpm workspace package `@openshift-online/hypershell-operational-dashboard-ui` containing the operational dashboard presentation layer, application ports, default layout template, widget renderers, localization messages, Storybook fixtures, and a `check` script for static verification.

The package SHALL export at minimum:

- `OperationalDashboardPage` - the full dashboard page
- `DashboardUiProvider` and `useDashboardUi` - host service injection
- `createDashboardOperations` - application workflow entry port
- `DashboardControlPlane`, `DashboardOperations`, `OperationalDashboardMetrics`, `OperationalMetric`, and related types
- `operationalDashboardMetricsQueryKey` and `operationalDashboardRefreshMilliseconds`
- `dashboardMessages` - canonical `defineMessages` catalog for dashboard strings
- `mockOperationalDashboardMetrics` fixture (via `@openshift-online/hypershell-operational-dashboard-ui/fixtures`)

#### Scenario: Host imports the dashboard page

- GIVEN the web console depends on `@openshift-online/hypershell-operational-dashboard-ui`
- WHEN the host route module renders `OperationalDashboardPage` inside `DashboardUiProvider`
- THEN the dashboard SHALL mount without importing React Router, the SDK, or Fastify directly

---

### Requirement: OP-DASH-02 -- Hexagonal Application Boundary

The operational dashboard package SHALL follow the narrow hexagonal UI boundary defined in `standards/ui/hexagonal-architecture.spec.md`.

- `DashboardControlPlane` SHALL be the application-owned **driven port** for loading `OperationalDashboardMetrics`.
- `DashboardOperations` SHALL be the application-owned **driving port** consumed by presentation hooks.
- `createDashboardOperations` SHALL accept a `DashboardControlPlane` adapter, an optional `DashboardWorkflowRuntime` (default: `crypto.randomUUID()` correlation IDs), and an optional `DashboardProbePublisher`.
- Presentation code SHALL call `dashboard.getOperationalMetrics(signal)` through `useDashboardUi()`; it SHALL NOT call `fetch`, the SDK, or BFF routes directly.

Workflow probes SHALL be published for `get-operational-metrics` with names `dashboard.workflow.started` and `dashboard.workflow.completed`, recording outcomes `started`, `succeeded`, `failed`, or `cancelled`. When one or more metric sources fail but at least one succeeds, a `dashboard.metrics.partial-failure` probe with outcome `failed` SHALL also be published (OP-DASH-19).

#### Scenario: AbortSignal propagates to the adapter

- GIVEN a TanStack Query request is cancelled
- WHEN `getOperationalMetrics` is in flight
- THEN the `AbortSignal` SHALL be passed to `DashboardControlPlane.getOperationalMetrics`
- AND a `dashboard.workflow.completed` probe with outcome `cancelled` SHALL be published

---

### Requirement: OP-DASH-03 -- Host Composition

The web console host SHALL wire dashboard operations in `components/web-console/app/composition/dashboard-composition.ts` by calling `createDashboardOperations` with `createDashboardControlPlaneAdapter(createApiClient)`.

The application shell SHALL wrap authenticated routes in `DashboardUiProvider`, supplying `dashboardOperations` and a `DashboardUiNavigation` contract (`collectionHref`, `navigate`).

#### Scenario: Production host uses the API adapter

- GIVEN the web console is running against the HyperShell API
- WHEN `OperationalDashboardPage` loads metrics
- THEN `createDashboardControlPlaneAdapter` SHALL be the active `DashboardControlPlane` implementation

---

### Requirement: OP-DASH-04 -- Administrator Access Control

Access to the operational dashboard SHALL be restricted to users with the `hypershell-admins` or `platform:admin` realm role.

The SPA route modules for `/dashboard` and the dashboard-host root (`/`) SHALL wrap `OperationalDashboardPage` in `RequireDashboardAdmin`, which:

- Shows a localized access-denied `EmptyState` when OIDC is enabled and the user is unauthenticated
- Shows a localized access-denied `EmptyState` when the user is authenticated but lacks a dashboard-admin role
- Renders children when OIDC is disabled (no-auth dev mode) or when the user holds a dashboard-admin role

When OIDC is enabled, the BFF SHALL enforce the same role requirement for browser navigations to `/dashboard` and for `/` on hosts whose hostname starts with `dashboard.`. Non-admin users SHALL be redirected away (to `/` on the console host, or to the console host when the request arrived on a dashboard subdomain).

When OIDC is enabled, the BFF SHALL enforce the same dashboard-admin role requirement on every `GET /api/metrics/*` route consumed by the operational dashboard host adapter (`cluster-memory`, `cluster-cpu`, `cluster-pods`, `cluster-nodes`, `gateways`, `gateway-sandboxes`, `gateway-provision-duration`, `platform-inventory`, and `registered-users` per OP-DASH-08). Authenticated callers without a dashboard-admin role SHALL receive HTTP `403`. Unauthenticated callers SHALL receive HTTP `401` or the standard BFF re-authentication response. When OIDC is disabled (no-auth dev mode), these routes SHALL remain open to unauthenticated callers, matching page behavior.

#### Scenario: Non-admin is turned away from /dashboard

- GIVEN OIDC is enabled and the signed-in user has only `hypershell-users`
- WHEN the user navigates to `/dashboard`
- THEN the BFF SHALL redirect away from the dashboard route
- AND the SPA SHALL render the access-denied empty state if the route still mounts

#### Scenario: Platform admin can open the dashboard

- GIVEN OIDC is enabled and the signed-in user has `platform:admin`
- WHEN the user navigates to `/dashboard`
- THEN the BFF SHALL serve `index.html` with HTTP `200`
- AND `OperationalDashboardPage` SHALL render

#### Scenario: Non-admin cannot fetch cluster metrics via BFF

- GIVEN OIDC is enabled and the signed-in user has only `hypershell-users`
- WHEN the user sends `GET /api/metrics/cluster-cpu`
- THEN the BFF SHALL respond with HTTP `403`

#### Scenario: Dashboard administrator can fetch cluster metrics

- GIVEN OIDC is enabled and the signed-in user has `hypershell-admins`
- AND Prometheus returns successful instant-query results
- WHEN the user sends `GET /api/metrics/cluster-memory`
- THEN the BFF SHALL respond with HTTP `200`

---

### Requirement: OP-DASH-05 -- SPA and BFF Route Surfaces

The web console SPA SHALL expose the operational dashboard at `/dashboard` via a React Router route module that renders `OperationalDashboardPage`.

When the browser hostname is `dashboard.hypershell.localhost`, the SPA root route (`/`) SHALL also render `OperationalDashboardPage` (dashboard-dedicated host entry point).

`route-contract.json` SHALL declare `"dashboard": "dashboard"`. The BFF SHALL treat `/dashboard` as a valid application route that returns `index.html` for direct navigation and refresh, alongside `/`, `/login`, `/gateways/new`, and `/gateways/:gatewayId`.

#### Scenario: Direct navigation to /dashboard

- GIVEN an authenticated dashboard administrator
- WHEN the browser requests `GET /dashboard`
- THEN the BFF SHALL respond with `index.html` and HTTP `200`
- AND the SPA SHALL render `OperationalDashboardPage`

#### Scenario: Dashboard host serves the dashboard at root

- GIVEN the browser hostname is `dashboard.hypershell.localhost`
- WHEN the user opens `/`
- THEN the SPA SHALL render `OperationalDashboardPage` instead of the gateways list

---

### Requirement: OP-DASH-06 -- Gateway Sandbox Metrics Adapter

The host `DashboardControlPlane` adapter SHALL load `provisioned-sandboxes` from BFF `GET /api/metrics/gateway-sandboxes` in the `gateway-metrics` metric source (OP-DASH-23).

The BFF route SHALL query Prometheus for `hypershell_gateways_active_sandboxes_total`, a fleet-wide gauge emitted by the API server metrics collector that sums `active_sandbox_count` across all gateways on each scrape (see `openshell-gateway-sandbox-count.spec.md` for field semantics).

The adapter SHALL emit a `provisioned-sandboxes` metric whose `value` is the stringified `active_sandboxes` count from the BFF response. The value SHALL NOT be a non-finite number.

A non-success BFF response for gateway sandboxes SHALL fail the entire `gateway-metrics` source (including `provisioned-gateways` and `provision-time`). Access control is enforced at the BFF route (dashboard-operator roles per OP-DASH-04).

#### Scenario: Prometheus sandbox count populates provisioned-sandboxes

- GIVEN `GET /api/metrics/gateway-sandboxes` returns `{ "active_sandboxes": 5 }`
- WHEN `getOperationalMetrics` runs
- THEN the adapter SHALL emit `provisioned-sandboxes` with `value: "5"`
- AND the dashboard SHALL NOT display `NaN`

#### Scenario: Gateway sandboxes failure omits gateway-metrics source

- GIVEN `GET /api/metrics/gateway-sandboxes` fails
- WHEN the adapter processes the `gateway-metrics` source
- THEN the `gateway-metrics` source SHALL be treated as failed
- AND `provisioned-sandboxes`, `provisioned-gateways`, and `provision-time` SHALL be omitted
- AND the dashboard SHALL NOT synthesize a zero sandbox count

---

### Requirement: OP-DASH-23 -- Gateway Prometheus Metrics Adapter

The host `DashboardControlPlane` adapter SHALL load `provisioned-gateways` (see OP-DASH-07), `provisioned-sandboxes` (OP-DASH-06), and optionally `provision-time` from BFF Prometheus proxy routes in the `gateway-metrics` metric source:

- `GET /api/metrics/gateways` - fleet-wide phase counts from `hypershell_gateways_total` (see `platform/gateway-metrics-dashboard.spec.md` DASH-05)
- `GET /api/metrics/gateway-sandboxes` - fleet-wide active sandbox sum from `hypershell_gateways_active_sandboxes_total` (OP-DASH-06)
- `GET /api/metrics/gateway-provision-duration` - provision duration histogram (see `platform/gateway-provision-time.spec.md`)

The adapter SHALL call `fetchGatewayMetrics` from `@openshift-online/hypershell-gateway-management-ui` for phase counts. A non-success BFF response for gateways or gateway sandboxes SHALL fail the entire `gateway-metrics` source.

`provision-time` SHALL be appended to the same source only when the provision-duration BFF route succeeds. When the route fails or returns no qualifying samples, `provision-time` SHALL be omitted while `provisioned-gateways` and `provisioned-sandboxes` MAY still be emitted.

Gateway phase counts SHALL be fleet-wide and SHALL NOT be filtered by per-gateway RoleBindings. Access control is enforced at the BFF route (dashboard-operator roles per OP-DASH-04).

#### Scenario: Prometheus gateway counts populate provisioned-gateways

- GIVEN `GET /api/metrics/gateways` returns `{ "counts": { "Running": 10, "Provisioning": 3, "Degraded": 1, "Failed": 4, "Pending": 2 } }`
- WHEN `getOperationalMetrics` runs
- THEN the adapter SHALL emit `provisioned-gateways` with `value: "20"`
- AND `status` SHALL map phases to display buckets per OP-DASH-07

#### Scenario: Prometheus gateway failure omits gateway-derived Prometheus metrics

- GIVEN `GET /api/metrics/gateways` fails
- WHEN the adapter processes the `gateway-metrics` source
- THEN the `gateway-metrics` source SHALL be treated as failed
- AND `provisioned-gateways`, `provisioned-sandboxes`, and `provision-time` SHALL be omitted

---

### Requirement: OP-DASH-07 -- Gateway Display Status Aggregation

Gateway status widgets SHALL present display buckets (`healthy`, `provisioning`, `degraded`, `failed`) aligned with the gateway list vocabulary. The host adapter SHALL map Prometheus phase counts to display buckets using `gatewayPhaseCountsToDisplayStatusCounts` from `@openshift-online/hypershell-gateway-management-ui`.

Phase-to-bucket mapping SHALL treat `Pending` and `Provisioning` as `provisioning`, `Running` as `healthy`, `Degraded` as `degraded`, and `Failed` as `failed`.

The `provisioned-gateways` metric SHALL include:

- `value` - total gateway count as a decimal string (sum of all phase counts)
- `status` - counts for `healthy`, `provisioning`, `degraded`, and `failed` display buckets

Display buckets SHALL NOT be confused with raw lifecycle `phase` values. Mapping logic SHALL remain owned by the gateway-management-ui package.

**Trade-off:** Prometheus exposes `phase` labels only. The gateway list combines `phase` and `status` (for example, `Running` with an unhealthy status resolves to `degraded`). Phase-only mapping MAY under-count `degraded` when phase is still `Running`. Finer-grained status alignment would require a Prometheus metric with both dimensions or a REST list aggregate.

#### Scenario: Gateway status widget reflects phase-mapped buckets

- GIVEN Prometheus returns phase counts equivalent to 10 healthy, 5 provisioning, 1 degraded, and 4 failed after mapping
- WHEN the gateway status widget renders
- THEN the donut chart SHALL show segments for healthy, provisioning, degraded, and failed with those counts
- AND the chart center title SHALL show `20`

---

### Requirement: OP-DASH-08 -- Connected and Placeholder Metrics

Version 1 of the operational dashboard SHALL distinguish **connected** metrics (populated by the host adapter today) from **placeholder** metrics (declared in the widget catalog but absent from the adapter response).

| Metric ID | Connected in v1 | Source when connected |
| --- | --- | --- |
| `provisioned-gateways` | Yes | BFF `GET /api/metrics/gateways` (Prometheus); OP-DASH-23, OP-DASH-07 |
| `provisioned-sandboxes` | Yes | BFF `GET /api/metrics/gateway-sandboxes` (Prometheus); OP-DASH-06 |
| `registered-users` | Yes | BFF `GET /api/metrics/registered-users` (Prometheus); see `platform/registered-users.spec.md` |
| `memory` | Yes | BFF `GET /api/metrics/cluster-memory` (Prometheus node-exporter); see `platform/cluster-memory.spec.md` |
| `nodes` | Yes | BFF `GET /api/metrics/cluster-nodes` (Prometheus kube-state-metrics); see `platform/cluster-nodes.spec.md` |
| `cpu` | Yes | BFF `GET /api/metrics/cluster-cpu` (Prometheus node-exporter); see `platform/cluster-cpu.spec.md` |
| `pods` | Yes | BFF `GET /api/metrics/cluster-pods` (Prometheus kube-state-metrics); see `platform/cluster-pods.spec.md` |
| `provision-time` | Yes | BFF `GET /api/metrics/gateway-provision-duration` (Prometheus control-plane histogram); see `platform/gateway-provision-time.spec.md` |
| `managed-clusters` | Yes | BFF `GET /api/metrics/platform-inventory` (Prometheus); see `platform/platform-inventory.spec.md` |
| `managed-databases` | Yes | BFF `GET /api/metrics/platform-inventory` (Prometheus); see `platform/platform-inventory.spec.md` |

Widgets for placeholder metrics SHALL remain in the default layout and in the add-widgets drawer. When a metric ID is missing from the adapter response - whether because the metric is not yet connected or because its data source failed (OP-DASH-19) - the widget body SHALL render a localized "Metric unavailable" empty state (title and recovery guidance) instead of failing the entire dashboard.

Summary rows (`usage-summary`, `system-summary`) that reference a missing metric SHALL render the same localized metric-unavailable message in place of the value instead of omitting the row or showing a blank cell.

Historical trend data (`OperationalMetric.trend`) and utilization capacity fields (`unit`, `total`) are not loaded by the production adapter in v1. Widgets SHALL omit trend sparklines and utilization donuts when those fields are absent.

The package SHALL maintain `DATA_SOURCES.md` documenting connected vs placeholder metrics and the adapter update procedure.

#### Scenario: Placeholder widget shows unavailable state

- GIVEN the adapter returns only `provisioned-gateways` and `provisioned-sandboxes`
- WHEN the `cpu` widget is on the layout
- THEN it SHALL render the localized metric-unavailable empty state
- AND the rest of the dashboard SHALL remain interactive

---

### Requirement: OP-DASH-09 -- Metrics Refresh Policy

`useGetMetricsData` SHALL load metrics through TanStack Query with:

- `queryKey` from `operationalDashboardMetricsQueryKey()` (`["operational-dashboard", "metrics"]`)
- `refetchInterval` and `staleTime` of `operationalDashboardRefreshMilliseconds` (`900_000` ms - 15 minutes)
- `enabled` controlled by the page (disabled when static `metrics` props are supplied for Storybook/tests)

The page SHALL expose a manual refresh control that calls `refetch()` on the query. While a refetch is in flight, the refresh button SHALL expose a localized refreshing state and SHALL remain in that state until every metric source fetch in the current invocation has settled (succeeded or failed).

#### Scenario: Total initial load failure blocks the grid

- GIVEN no metrics have ever loaded successfully
- WHEN every metric source fails on the first fetch
- THEN a danger `Alert` with localized title and body SHALL be shown
- AND the widget grid SHALL NOT render

#### Scenario: Partial initial load shows warning and available data

- GIVEN no metrics have ever loaded successfully
- WHEN at least one metric source succeeds and at least one metric source fails
- THEN a warning `Alert` with localized title and body SHALL be shown indicating that some metrics could not be loaded and the dashboard may be incomplete
- AND the widget grid SHALL render with every successfully loaded metric
- AND widgets and summary rows for omitted metrics SHALL render the localized metric-unavailable state (OP-DASH-08)

#### Scenario: Refresh with partial failure preserves last data for failed sources

- GIVEN metrics loaded successfully on a prior fetch
- WHEN a subsequent refetch completes with at least one metric source failure
- THEN a warning `Alert` SHALL be shown indicating that some metrics could not be refreshed and the dashboard may show stale or missing data
- AND the widget grid SHALL continue displaying the last successful value for each metric whose source failed on this fetch
- AND metrics whose sources succeeded on this fetch SHALL display the refreshed values

#### Scenario: Refresh with total failure preserves all last data

- GIVEN metrics loaded successfully on a prior fetch
- WHEN a subsequent refetch fails for every metric source
- THEN a warning `Alert` SHALL be shown
- AND the widget grid SHALL continue displaying the last successful metrics from the prior fetch

---

### Requirement: OP-DASH-19 -- Independent Metric Sources and Partial Failure

The host `DashboardControlPlane` adapter SHALL load operational metrics from independent sources. A failure in one source SHALL NOT prevent other sources from contributing metrics to the same `getOperationalMetrics` invocation.

| Source | Metric IDs affected |
| --- | --- |
| BFF `GET /api/metrics/gateways`, `GET /api/metrics/gateway-sandboxes`, and `GET /api/metrics/gateway-provision-duration` (`gateway-metrics`) | `provisioned-gateways`, `provisioned-sandboxes`, `provision-time` |
| BFF `GET /api/metrics/registered-users` (`registered-users`) | `registered-users` |
| BFF `GET /api/metrics/cluster-memory` | `memory` |
| BFF `GET /api/metrics/cluster-cpu` | `cpu` |
| BFF `GET /api/metrics/cluster-pods` | `pods` |
| BFF `GET /api/metrics/cluster-nodes` | `nodes` |
| BFF `GET /api/metrics/platform-inventory` (`platform-inventory`) | `managed-clusters`, `managed-databases` |

The adapter SHALL fetch these sources concurrently. When a source fails (network error, non-success HTTP status, inconsistent pagination, or other adapter validation error for that source), the adapter SHALL:

- Omit every metric ID owned by that source from the returned `metrics` array
- NOT synthesize zero, empty, or placeholder values for failed metrics
- NOT throw from `getOperationalMetrics` solely because one or more sources failed

`getOperationalMetrics` SHALL throw only when every metric source fails or when the request is aborted.

When at least one source succeeds, the adapter SHALL return `OperationalDashboardMetrics` with:

- `lastSuccessfulRefresh` set to the current time
- `metrics` containing only the metrics from successful sources

Workflow probes for `get-operational-metrics` SHALL record outcome `succeeded` when at least one metric source succeeds and outcome `failed` only when every source fails or the invocation is aborted. When one or more sources fail but at least one succeeds, the adapter or application layer SHALL publish an additional probe (for example `dashboard.metrics.partial-failure`) with outcome `failed`, naming the failed source identifiers.

The dashboard page SHALL derive partial-failure warnings from the adapter result (omitted expected metrics and/or explicit failure metadata) rather than treating a partial response as a query error that blocks the grid.

#### Scenario: Prometheus down does not hide unrelated metric sources

- GIVEN `GET /api/metrics/registered-users` succeeds
- AND every other BFF metrics request fails (gateway metrics, platform inventory, and cluster metrics)
- WHEN the operator opens `/dashboard`
- THEN the registered-user widget SHALL display loaded values
- AND gateway, sandbox, inventory, provision-time, and cluster metric widgets SHALL render the localized metric-unavailable state
- AND a warning `Alert` SHALL explain that some metrics could not be loaded

#### Scenario: Gateway metrics failure does not hide cluster metrics

- GIVEN every BFF cluster-metrics request succeeds
- AND the `gateway-metrics` source fails (for example, `GET /api/metrics/gateways` returns HTTP `502`)
- WHEN the operator opens `/dashboard`
- THEN cluster metric widgets SHALL display loaded values
- AND gateway, sandbox, and provision-time widgets or summary rows SHALL render the localized metric-unavailable state
- AND a warning `Alert` SHALL explain that some metrics could not be loaded

#### Scenario: Platform inventory failure does not hide other metrics

- GIVEN every other metric source succeeds
- AND `GET /api/metrics/platform-inventory` fails
- WHEN the operator opens `/dashboard`
- THEN gateway, sandbox, registered-user, and cluster metric widgets SHALL display loaded values
- AND inventory widgets and summary rows SHALL render the localized metric-unavailable state
- AND a warning `Alert` SHALL explain that some metrics could not be loaded

#### Scenario: No qualifying provision-time samples omit only provision time

- GIVEN `GET /api/metrics/gateways` and `GET /api/metrics/gateway-sandboxes` succeed
- AND `GET /api/metrics/gateway-provision-duration` returns no qualifying histogram observations
- WHEN `getOperationalMetrics` runs
- THEN `provisioned-gateways` and `provisioned-sandboxes` SHALL still be emitted
- AND `provision-time` SHALL be omitted
- AND the provision-time widget or summary row SHALL render the localized metric-unavailable state
- AND the dashboard SHALL NOT enter the total load-error state

---

### Requirement: OP-DASH-10 -- Widgetized Grid Layout

The dashboard SHALL use PatternFly's `@patternfly/widgetized-dashboard` (`GridLayout`, `WidgetDrawer`, `AddWidgetsButton`) with a four-column grid on `xl`, `lg`, and `md` breakpoints and a single-column stack on `sm`.

The default layout template (`defaultDashboardLayoutTemplate`) SHALL place these widgets. Each section begins with a full-width `section-title` row (OP-DASH-20); metric widgets in that section start on the row below the title.

| Widget type | Layout item `i` | Default position (4-column) |
| --- | --- | --- |
| `section-title` | `section-title#platform-adoption` | Full width, row 0 (platform adoption header) |
| `usage-summary` | `usage-summary#1` | Column 0, platform adoption |
| `gateway-status` | `gateway-status#1` | Columns 1–2, platform adoption, spans two columns |
| `provisioned-sandboxes` | `provisioned-sandboxes#1` | Column 3, platform adoption |
| `registered-users` | `registered-users#1` | Column 3, platform adoption second row |
| `section-title` | `section-title#hub-cluster` | Full width, hub cluster header row |
| `system-summary` | `system-summary#1` | Column 0, hub cluster |
| `memory` | `memory#1` | Column 1, hub cluster |
| `provision-time` | `provision-time#1` | Column 1, hub cluster second row (below memory) |
| `cpu` | `cpu#1` | Column 2, hub cluster |
| `pods` | `pods#1` | Column 3, hub cluster |
| `nodes` | `nodes#1` | Column 2, hub cluster second row (below cpu) |

On `sm`, widgets SHALL stack in section order: platform adoption title, adoption metrics, hub cluster title, then hub cluster metrics.

The page header SHALL place the localized last-refreshed timestamp, manual refresh control, reset-to-default link, and add-widgets button in the top-right column, stacked vertically with compact spacing. The grid SHALL NOT use a separate sticky toolbar between the page header and widgets. The last-refreshed timestamp SHALL be formatted from `lastSuccessfulRefresh`. The page description SHALL describe v1 scope and the 15-minute refresh interval without duplicating refresh instructions already shown in the timestamp area.

Widget titles in the layout template SHALL be localized via `localizeDashboardLayoutTemplate` using the package `messages` catalog.

Users SHALL be able to add widgets from the drawer, drag to rearrange, and remove widgets. The add-widgets button SHALL be hidden when every known widget type is already on the grid. A "Reset to default" link SHALL restore `defaultDashboardLayoutTemplate` and close the drawer.

#### Scenario: Reset restores the default layout

- GIVEN the user has rearranged widgets
- WHEN they activate reset to default
- THEN the grid SHALL return to `defaultDashboardLayoutTemplate`
- AND the updated layout SHALL be persisted (OP-DASH-11)

---

### Requirement: OP-DASH-11 -- Layout Persistence

The dashboard SHALL persist the sanitized layout template to `localStorage` under the key `hypershell.operational-dashboard.layout.v23`.

On mount, a saved template SHALL be loaded when it parses as valid JSON and contains an array entry for every responsive variant (`xl`, `lg`, `md`, `sm`). Invalid or corrupt saved state SHALL fall back to the default template without surfacing an error to the user.

Duplicate layout item `i` values in a single variant SHALL be deduplicated on save (first occurrence wins). Duplicate metric `widgetType` entries (every type except `section-title`) SHALL also be deduplicated on save (first occurrence wins). Multiple `section-title` instances with distinct `i` values SHALL be preserved.

When persistence fails (for example, storage quota exceeded), the dashboard SHALL continue to function in memory and SHALL publish a `dashboard.layout.template.persistence-failed` probe with outcome `failed`.

#### Scenario: Saved layout survives reload

- GIVEN the user removed the `pods` widget and the layout was saved
- WHEN they reload the browser
- THEN the grid SHALL render without the `pods` widget

---

### Requirement: OP-DASH-12 -- Gateway Status Widget

The `gateway-status` widget SHALL render a `GatewayStatusChart` donut using the shared `StatusDonutChart` primitive (`@patternfly/react-charts/victory` `ChartDonut`), driven by the `provisioned-gateways` metric's `status` and `value` fields.

The default layout template SHALL place `gateway-status` at `GATEWAY_STATUS_WIDGET_HEIGHT`, equal to `USAGE_SUMMARY_WIDGET_HEIGHT` in the platform adoption section.

The chart SHALL:

- Include only buckets with count greater than zero
- Use status colors aligned with PatternFly alert/label semantics (healthy: chart green, provisioning: chart blue, degraded: warning yellow, failed: danger red)
- Resize with its container via `ResizeObserver`
- Expose localized `ariaTitle` and `ariaDesc` attributes
- Show the total gateway count as the chart title and a localized "Gateways" subtitle
- Use gateway-specific bucket labels (`Healthy`, `Provisioning`, `Degraded`, `Failed`)

When `status` is absent or all bucket counts are zero, the chart area SHALL render nothing (the widget shell remains).

If the metric includes `trend` data, a `TrendSparklineChart` SHALL render below the donut; otherwise the sparkline SHALL be omitted.

#### Scenario: Degraded and failed counts appear in usage summary

- GIVEN the usage summary lists gateways with 2 failed and 3 degraded
- WHEN the summary row renders
- THEN failed and degraded counts SHALL appear with danger and warning status icons respectively
- AND healthy-only fleets SHALL omit the exception status row

---

### Requirement: OP-DASH-16 -- Status Donut Presentation

Gateway and node inventory metrics that expose `OperationalMetric.value` plus `OperationalMetric.status` SHALL share a common status-donut presentation stack in `packages/operational-dashboard-ui`:

| Layer | Responsibility |
| --- | --- |
| `StatusDonutChart` | Shared `ChartDonut` shell: resize handling, padding, legend layout, and dark-mode label tokens |
| `buildStatusDonutData` | Shared series builder that omits zero-count buckets |
| `buildGatewayStatusData` / `buildNodeStatusData` | Domain-specific bucket order, colors, and localized labels |
| `GatewayStatusChart` / `NodeStatusChart` | Thin wrappers that map each metric to the shared chart |

`GatewayStatusChart` SHALL remain the presentation for the `gateway-status` widget (`provisioned-gateways` metric). `NodeStatusChart` SHALL be the presentation for the `nodes` widget and metric.

Node status donuts SHALL reuse the same `status.healthy` and `status.failed` keys as the adapter mapping in `platform/cluster-nodes.spec.md` CLN-05, but SHALL render localized **Ready** and **Not ready** labels instead of gateway vocabulary.

`NodeStatusCard` SHALL wrap `NodeStatusChart` with the same card shell used by `GatewayStatusCard`. The default layout template SHALL include a `nodes` widget (OP-DASH-10) at `NODE_STATUS_WIDGET_HEIGHT` (one grid unit taller than `cpu`/`pods`). The `nodes` widget SHALL NOT render a trend sparkline. `NodeStatusChart` SHALL use the compact `StatusDonutChart` size without a chart subtitle (the widget title already identifies the metric) and reduced card-body padding so the donut is not clipped.

When `status` is absent or all bucket counts are zero, status donut charts SHALL render nothing while their widget shell remains.

#### Scenario: Node status donut shows ready and not-ready segments

- GIVEN the `nodes` metric has `value: "8"` and `status: { healthy: 7, failed: 1 }`
- WHEN `NodeStatusChart` renders
- THEN the donut SHALL show a Ready segment of `7` and a Not ready segment of `1`
- AND the chart center title SHALL show `8`
- AND the widget title SHALL be localized "Nodes"
- AND the chart SHALL NOT render a donut subtitle

---

### Requirement: OP-DASH-17 -- Pod Capacity Widget

The `pods` widget SHALL render `PodCapacityChart` when the metric includes `unit`, `total`, and `podPhases`. `PodCapacityCard` SHALL wrap `PodCapacityChart` with the same card shell used by `GatewayStatusCard` and `NodeStatusCard`.

`PodCapacityChart` SHALL use the compact `StatusDonutChart` size. When a donut subtitle is rendered, the chart SHALL use expanded compact height and padding so the subtitle and legend are not clipped. The chart center title SHALL show **used** pods (`value`). The chart subtitle SHALL read "of {total} pods". Segments SHALL include Running, Pending, Failed, Succeeded, Unknown (from `podPhases`), plus an **Unused** segment (gray) for `total - value` (available capacity).

The default layout template SHALL place the `pods` widget at `POD_CAPACITY_WIDGET_HEIGHT` (same height as `nodes`). The `pods` widget SHALL NOT render a trend sparkline.

When `podPhases` is absent, `PodCapacityChart` SHALL render nothing while its widget shell remains.

#### Scenario: Pod capacity donut shows phase and unused segments

- GIVEN the `pods` metric has `value: "548"`, `total: "2000"`, `unit: "pods"`, and `podPhases: { running: 500, pending: 12, succeeded: 20, failed: 16, unknown: 0 }`
- WHEN `PodCapacityChart` renders
- THEN the donut SHALL show phase segments matching `podPhases` and an Unused segment of `1452`
- AND the chart center title SHALL show `548`
- AND the chart subtitle SHALL read "of 2000 pods"
- AND the widget title SHALL be localized "Pods"

---

### Requirement: OP-DASH-13 -- Metric and Utilization Widgets

**Metric cards** (`MetricCard`) SHALL center a large heading with the metric value and an optional subtitle. When `trend.points` is present, a `TrendSparklineChart` SHALL render beneath the heading.

**Utilization widgets** (`cpu`, `memory`) SHALL render `UtilizationChart` when the metric includes both `unit` and `total`. Utilization percentage SHALL be `round((value / total) * 100)`. Status icons SHALL use thresholds: warning at `>= 60%`, danger at `>= 90%`.

The `pods` widget SHALL use `PodCapacityChart` (OP-DASH-17), not `UtilizationChart`. The `system-summary` pods row SHALL continue to show utilization percentage from the same `pods` metric and SHALL show a failed pod count when `podPhases.failed` is non-zero.

**Summary widgets:**

- `usage-summary` - horizontal `DescriptionList` for active users, gateways (with exception status counts), and sandboxes
- `system-summary` - horizontal `DescriptionList` for memory, CPU, pods (with failed pod count when `podPhases.failed` is non-zero), nodes (with exception status counts when `status.failed` is non-zero), and provision duration (average, P50, and P95 rows when `provisionDuration` is present on the `provision-time` metric; see `platform/gateway-provision-time.spec.md` GPT-05)

Trend direction indicators in summary rows SHALL appear only when `getMetricTrendChange` detects at least a 5% change between the first and last trend point.

#### Scenario: Utilization widget without capacity fields

- GIVEN the `memory` metric has `value` but no `unit` or `total`
- WHEN the memory widget renders
- THEN the utilization donut SHALL NOT render
- AND the summary row SHALL show the raw value only

#### Scenario: Failed pod count appears in system summary

- GIVEN the `pods` metric has `podPhases.failed: 16`
- WHEN the system summary pods row renders
- THEN a failed count of `16` SHALL appear below the utilization value with a danger status icon

#### Scenario: Provision duration shows average, P50, and P95

- GIVEN the `provision-time` metric has `unit: "minutes"`, `value: "5.25"`, and `provisionDuration: { mean: "5.25", p50: "4.80", p95: "12.10" }`
- WHEN the system summary provision-duration rows render
- THEN three localized rows SHALL appear for average, P50, and P95

---

### Requirement: OP-DASH-14 -- Localization and Accessibility

All user-visible dashboard strings SHALL be declared with `defineMessages` in the operational-dashboard-ui package and rendered through `react-intl`. No literal user-facing string SHALL appear in JSX.

The web-console host SHALL extract dashboard message IDs into its `locales/en.json` catalog for production rendering.

Loading states SHALL use a `Spinner` with a localized `aria-label` on the initial load only (before any metrics are available). Empty, partial-failure, and total-failure states SHALL use PatternFly `EmptyState` or `Alert` with localized titles and bodies. Partial-failure warnings SHALL use the `warning` alert variant; total initial-load failure SHALL use the `danger` variant. The operational-dashboard-ui package SHALL declare localized `partialLoadWarningTitle` and `partialLoadWarningBody` messages (or equivalent IDs) for the warning shown when one or more metric sources fail but the grid still renders. Refresh-time partial failures MAY reuse the same warning copy or dedicated refresh-partial-failure messages; in all cases the alert SHALL state that some metrics could not be loaded or refreshed and that the dashboard may be incomplete or stale. Interactive trend and status icons SHALL expose localized `aria-label` values via `Tooltip` or button labels.

#### Scenario: Page description introduces live metrics and refresh behavior

- GIVEN the dashboard page loads
- WHEN the description paragraph renders
- THEN it SHALL summarize gateway fleet health, hub cluster capacity, and platform usage
- AND it SHALL state that metrics refresh every 15 minutes and that Refresh updates them immediately

---

### Requirement: OP-DASH-20 -- Section Title Widgets

The dashboard SHALL support a `section-title` widget type for full-width section headers that group related metric widgets. Section titles are decorative labels only; they do not load metrics.

The default layout template SHALL include two `section-title` instances:

| Layout item `i` | Localized title | Message ID |
| --- | --- | --- |
| `section-title#platform-adoption` | Platform adoption | `app.dashboard.sectionTitle.platformAdoption` |
| `section-title#hub-cluster` | Hub cluster | `app.dashboard.sectionTitle.hubCluster` |

Each `section-title` layout item SHALL span all four columns (`w: 4`), use height `TITLE_WIDGET_HEIGHT` (one grid unit), and sit on its own row above the widgets in that section.

`localizeDashboardLayoutTemplate` SHALL resolve each instance's visible title from its layout item `i` via `SECTION_TITLE_MESSAGE_BY_ID`. Unknown `i` values SHALL keep the template's fallback `title` string.

The `section-title` widget body SHALL render `SectionTitleCard` with the resolved title. Presentation SHALL:

- Hide the widget grid tile header (no drag handle, kabob menu, or title bar)
- Suppress resize handles on the grid item
- Render a borderless, transparent card shell (no box shadow)
- Style the label as uppercase, subtle gray, small-caps typography (`.hypershell-dashboard-section-title`)

Users MAY add additional `section-title` widgets from the add-widgets drawer. New instances SHALL use the localized default label `app.dashboard.widget.sectionTitle` until the layout item `i` is mapped in `SECTION_TITLE_MESSAGE_BY_ID`.

#### Scenario: Section titles label each dashboard region

- GIVEN the default layout template is active
- WHEN the dashboard grid renders on a four-column breakpoint
- THEN a localized **Platform adoption** header SHALL appear full width above the adoption widgets
- AND a localized **Hub cluster** header SHALL appear full width above the hub cluster widgets
- AND neither section title SHALL show a widget card header or border chrome

---

### Requirement: OP-DASH-18 -- Non-Displayable Metric Values

The dashboard SHALL never render the literal strings `NaN`, `Infinity`, or `-Infinity` to users.

When an `OperationalMetric.value` (or utilization `total`) is not a finite decimal string - including when numeric coercion yields `NaN` or another non-finite number - presentation widgets SHALL display the localized message **Metric could not be determined** instead of the raw value.

This rule SHALL apply to metric cards, summary rows, utilization widgets, and status-donut center titles. Widgets that normally combine a value with a label or unit (for example `{value} {label}` or `{value} {unit}`) SHALL show only the fallback message when the value is non-displayable; they SHALL NOT append the metric label or unit after the fallback.

Utilization status icons and percentage calculations SHALL be omitted when `value` or `total` is non-displayable.

The fallback message SHALL be declared in the operational-dashboard-ui `messages` catalog and rendered through `react-intl`.

#### Scenario: Sandboxes widget hides NaN

- GIVEN the `provisioned-sandboxes` metric has `value: "NaN"`
- WHEN the sandboxes metric card or usage-summary sandboxes row renders
- THEN the user SHALL see the localized **Metric could not be determined** message
- AND the user SHALL NOT see `NaN`

#### Scenario: Utilization row omits percentage for non-displayable capacity

- GIVEN the `memory` metric has `value: "4"`, `unit: "GiB"`, and `total: "NaN"`
- WHEN the system-summary memory row renders
- THEN the user SHALL see the localized **Metric could not be determined** message
- AND no utilization status icon SHALL appear

---

### Requirement: OP-DASH-15 -- Verification Fixtures

The operational-dashboard-ui package SHALL ship `mockOperationalDashboardMetrics` containing all widget metric IDs with representative fields matching production adapter output (`status`, `podPhases`, `unit`, `total` where applicable). Fixtures SHALL NOT include `trend` data because historical series are not loaded in version 1.

The web console SHALL provide Storybook stories for default, loading, partial-load-warning, total-initial-load-error, and refresh-partial-failure states using mock or stub `DashboardControlPlane` adapters.

The host mock adapter (`createMockDashboardControlPlane`) MAY introduce an artificial delay for demo purposes; production adapters SHALL NOT.

#### Scenario: Storybook renders the full widget grid

- GIVEN `mockOperationalDashboardMetrics` is supplied to `OperationalDashboardPage`
- WHEN the default Storybook story renders
- THEN all default-layout widgets SHALL mount without calling the HyperShell API

---

### Requirement: OP-DASH-20 -- Platform Inventory Summary

The operational dashboard SHALL include an `inventory-summary` widget that renders platform inventory totals and top dimensions from the `managed-clusters` and `managed-databases` metrics defined in `platform/platform-inventory.spec.md` (PI-05, PI-06).

The default layout template SHALL add a **Platform inventory** section below the hub cluster section:

| Widget type | Layout item `i` | Default position (4-column) |
| --- | --- | --- |
| `inventory-summary` | `inventory-summary#1` | Column 0, platform inventory section |
| `managed-cluster-providers` | `managed-cluster-providers#1` | Column 1, platform inventory section |
| `managed-cluster-regions` | `managed-cluster-regions#1` | Columns 2–3 (two columns wide), platform inventory section |
| `managed-database-status` | `managed-database-status#1` | Column 1, platform inventory second row |

`localizeDashboardLayoutTemplate` SHALL resolve `section-title#platform-inventory` through `SECTION_TITLE_MESSAGE_BY_ID` with message ID `app.dashboard.sectionTitle.platformInventory`.

The inventory summary widget SHALL use the same `DescriptionList` summary presentation stack as `usage-summary` and `system-summary` (OP-DASH-13). It SHALL NOT render trend sparklines.

The `managed-cluster-providers`, `managed-cluster-regions`, and `managed-database-status` widgets SHALL use the shared `StatusDonutChart` stack (OP-DASH-16) with labels from `inventoryProviders`, `inventoryRegions`, and `inventoryStatus` keys respectively.

Adding the two-column region donut to the default layout SHALL bump the layout persistence key to `hypershell.operational-dashboard.layout.v26` (OP-DASH-11). Aligning `gateway-status` height with `usage-summary` SHALL bump the layout persistence key to `hypershell.operational-dashboard.layout.v27`. Adding `managed-database-status` to the default layout SHALL bump the layout persistence key to `hypershell.operational-dashboard.layout.v28`.

#### Scenario: Default layout includes inventory summary

- GIVEN the default layout template is active
- WHEN the dashboard grid renders on a four-column breakpoint
- THEN a localized **Platform inventory** section title SHALL appear below the hub cluster widgets
- AND the inventory summary card SHALL list managed cluster and managed database totals

---

### Requirement: OP-DASH-21 -- Optional Platform Inventory Widgets

The widget catalog SHALL register optional inventory detail widgets defined in `platform/platform-inventory.spec.md` (PI-07):

- `managed-cluster-providers` - provider donut driven by `managed-clusters.inventoryProviders` (on default layout; OP-DASH-20)
- `managed-cluster-regions` - placement donut driven by `managed-clusters.inventoryRegions` (`{region} ({provider})` keys; on default layout; OP-DASH-20)
- `managed-clusters` - large number tile for total managed clusters
- `managed-cluster-status` - status donut driven by `managed-clusters.inventoryStatus`
- `managed-databases` - large number tile for total managed databases
- `managed-database-status` - status donut driven by `managed-databases.inventoryStatus` (on default layout; OP-DASH-20)

These optional widget types SHALL be available in the add-widgets drawer. `managed-cluster-providers`, `managed-cluster-regions`, and `managed-database-status` SHALL also appear in `defaultDashboardLayoutTemplate` (OP-DASH-20).

Status donut widgets SHALL reuse the shared `StatusDonutChart` stack (OP-DASH-16) with inventory-specific bucket labels from `inventoryStatus` keys. They SHALL NOT reuse gateway display-status colors or vocabulary.

A separate `/dashboard/inventory` route SHALL NOT be added in version 1.

#### Scenario: Operator adds managed cluster status donut

- GIVEN the default layout is active and inventory metrics are connected
- WHEN the operator adds `managed-cluster-status` from the widget drawer
- THEN the grid SHALL render a status donut for managed cluster inventory
- AND the widget SHALL omit a trend sparkline
