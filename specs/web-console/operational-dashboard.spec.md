# Operational Dashboard

**Status:** Active
**Applies to:** `packages/operational-dashboard-ui`, `components/web-console` SPA and BFF, `packages/gateway-management-ui` (display-status aggregation), `components/sdk-typescript` (gateway list client)

## Purpose

Provide a widgetized operational dashboard in the HyperShell web console where administrators can assess fleet health at a glance. The dashboard composes summary and detail widgets in a customizable grid layout. Live data is loaded through a narrow application port (`DashboardControlPlane`) implemented by the web-console host; widgets without a connected source remain on the page and render a localized unavailable state rather than being hidden.

This specification covers the reusable `operational-dashboard-ui` package, the host adapter that aggregates gateway list data, admin-only access controls, SPA and BFF route surfaces, layout persistence, and the widget catalog. It does **not** cover the Prometheus metrics pipeline (`hypershell_gateways_total`, BFF `GET /api/metrics/gateways`, or `GatewayMetricsDashboard`); those are defined in `platform/gateway-metrics-dashboard.spec.md`.

### Relationship to gateway metrics

| Concern | Operational dashboard (this spec) | Gateway metrics dashboard |
| --- | --- | --- |
| Primary route | `/dashboard` and `dashboard.*` host root (`/`) | `GatewayMetricsDashboard` component (embeddable) |
| Gateway counts source | Paginated HyperShell REST `GET /api/hypershell/v1/gateways` | Prometheus `hypershell_gateways_total` via BFF proxy |
| Scope | RBAC-filtered: counts reflect gateways visible to the signed-in caller | Fleet-wide database aggregate |
| Status model | Display buckets: `healthy`, `provisioning`, `degraded`, `failed` | Lifecycle phases: `Running`, `Provisioning`, `Degraded`, `Failed` |

The two surfaces MAY coexist. The operational dashboard SHALL NOT be required to consume the Prometheus BFF route.

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

Workflow probes SHALL be published for `get-operational-metrics` with names `dashboard.workflow.started` and `dashboard.workflow.completed`, recording outcomes `started`, `succeeded`, `failed`, or `cancelled`.

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

When OIDC is enabled, the BFF SHALL enforce the same dashboard-admin role requirement on every `GET /api/metrics/*` route consumed by the operational dashboard host adapter (`cluster-memory`, `cluster-cpu`, `cluster-pods`, `cluster-nodes`, and `gateway-provision-duration` per OP-DASH-08). Authenticated callers without a dashboard-admin role SHALL receive HTTP `403`. Unauthenticated callers SHALL receive HTTP `401` or the standard BFF re-authentication response. When OIDC is disabled (no-auth dev mode), these routes SHALL remain open to unauthenticated callers, matching page behavior.

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

### Requirement: OP-DASH-06 -- Gateway List Metrics Adapter

The host `DashboardControlPlane` adapter SHALL load operational metrics by paginating `GET /api/hypershell/v1/gateways` through the browser TypeScript SDK with page size `100`, ordered by `name asc`, until all pages are retrieved.

The adapter SHALL validate each list response for internal consistency (page number, total count, and item count). An inconsistent response SHALL fail the metrics load with an error.

The adapter SHALL return `OperationalDashboardMetrics` containing:

- `lastSuccessfulRefresh` set to the current time
- A `provisioned-gateways` metric (see OP-DASH-07)
- A `provisioned-sandboxes` metric whose `value` is the stringified sum of `active_sandbox_count` across all gateways in the aggregated list

When summing `active_sandbox_count`, the adapter SHALL treat an omitted or null field on a gateway as `0` (matching database `COALESCE` semantics in `openshell-gateway-sandbox-count.spec.md`). The sum SHALL NOT produce a non-finite numeric result.

Gateway list results SHALL reflect the caller's RBAC visibility (the API applies gateway visibility filtering; the dashboard does not bypass it).

#### Scenario: Omitted sandbox counts do not break the aggregate

- GIVEN the aggregated list contains gateways where some omit `active_sandbox_count` and others report `2` and `3`
- WHEN the adapter computes `provisioned-sandboxes`
- THEN the metric `value` SHALL be `"5"`
- AND the dashboard SHALL NOT display `NaN`

#### Scenario: Multiple pages are aggregated

- GIVEN the caller can see 150 gateways
- WHEN `getOperationalMetrics` runs
- THEN the adapter SHALL issue two paginated list requests
- AND the `provisioned-gateways` metric `value` SHALL be `"150"`

#### Scenario: Inconsistent pagination fails the load

- GIVEN a list response reports `page: 2` when page `1` was requested
- WHEN the adapter processes the response
- THEN it SHALL throw an error
- AND the dashboard SHALL enter its load-error state

---

### Requirement: OP-DASH-07 -- Gateway Display Status Aggregation

Gateway status widgets SHALL use the same display-status presentation rules as the gateway list. The host adapter SHALL pass each gateway's `phase` and `status` to `aggregateGatewayDisplayStatusCounts` from `@openshift-online/hypershell-gateway-management-ui`.

The `provisioned-gateways` metric SHALL include:

- `value` - total gateway count as a decimal string
- `status` - counts for `healthy`, `provisioning`, `degraded`, and `failed` display buckets

Display buckets SHALL NOT be confused with raw lifecycle `phase` values. Mapping from phase/status to display buckets SHALL remain owned by the gateway-management-ui package so the dashboard and gateway list stay aligned.

#### Scenario: Gateway status widget reflects list presentation

- GIVEN the aggregated list contains gateways whose resolved display statuses are 5 healthy, 2 provisioning, 1 degraded, and 0 failed
- WHEN the gateway status widget renders
- THEN the donut chart SHALL show segments for healthy, provisioning, and degraded with those counts
- AND the chart center title SHALL show `8`

---

### Requirement: OP-DASH-08 -- Connected and Placeholder Metrics

Version 1 of the operational dashboard SHALL distinguish **connected** metrics (populated by the host adapter today) from **placeholder** metrics (declared in the widget catalog but absent from the adapter response).

| Metric ID | Connected in v1 | Source when connected |
| --- | --- | --- |
| `provisioned-gateways` | Yes | Gateway list aggregate (OP-DASH-06, OP-DASH-07) |
| `provisioned-sandboxes` | Yes | Sum of `active_sandbox_count` from gateway list |
| `registered-users` | Yes | See `platform/registered-users.spec.md` |
| `memory` | Yes | BFF `GET /api/metrics/cluster-memory` (Prometheus node-exporter); see `platform/cluster-memory.spec.md` |
| `nodes` | Yes | BFF `GET /api/metrics/cluster-nodes` (Prometheus kube-state-metrics); see `platform/cluster-nodes.spec.md` |
| `cpu` | Yes | BFF `GET /api/metrics/cluster-cpu` (Prometheus node-exporter); see `platform/cluster-cpu.spec.md` |
| `pods` | Yes | BFF `GET /api/metrics/cluster-pods` (Prometheus kube-state-metrics); see `platform/cluster-pods.spec.md` |
| `provision-time` | Yes | BFF `GET /api/metrics/gateway-provision-duration` (Prometheus control-plane histogram); see `platform/gateway-provision-time.spec.md` |

Widgets for placeholder metrics SHALL remain in the default layout and in the add-widgets drawer. When a metric ID is missing from the adapter response, the widget body SHALL render a localized "Metric unavailable" empty state (title and recovery guidance) instead of failing the entire dashboard.

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

The page SHALL expose a manual refresh control that calls `refetch()` on the query. While a refetch is in flight, the refresh button SHALL expose a localized refreshing state.

#### Scenario: Initial load failure blocks the grid

- GIVEN no metrics have ever loaded successfully
- WHEN the query fails
- THEN a danger `Alert` with localized title and body SHALL be shown
- AND the widget grid SHALL NOT render

#### Scenario: Refresh failure preserves last data

- GIVEN metrics loaded successfully on a prior fetch
- WHEN a subsequent refetch fails
- THEN a warning `Alert` SHALL be shown
- AND the widget grid SHALL continue displaying the last successful metrics

---

### Requirement: OP-DASH-10 -- Widgetized Grid Layout

The dashboard SHALL use PatternFly's `@patternfly/widgetized-dashboard` (`GridLayout`, `WidgetDrawer`, `AddWidgetsButton`) with a four-column grid on `xl`, `lg`, and `md` breakpoints and a single-column stack on `sm`.

The default layout template (`defaultDashboardLayoutTemplate`) SHALL place these widgets:

| Widget type | Default position (4-column) |
| --- | --- |
| `usage-summary` | Column 0, top |
| `system-summary` | Column 0, below usage summary |
| `gateway-status` | Column 1, spans two metric rows |
| `provisioned-sandboxes` | Column 2, top |
| `memory` | Column 3, top |
| `active-users` | Column 2, second row |

**Note:** Widget type `active-users` is superseded by `registered-users` per `platform/registered-users.spec.md` RU-05. New implementations SHALL use `registered-users` only.
| `cpu` | Column 3, second row |
| `pods` | Column 3, third row (one grid unit taller than `cpu`; same height as `nodes`) |
| `nodes` | Column 2, third row (one grid unit taller than `cpu`) |

Widget titles in the layout template SHALL be localized via `localizeDashboardLayoutTemplate` using the package `messages` catalog.

Users SHALL be able to add widgets from the drawer, drag to rearrange, and remove widgets. The add-widgets button SHALL be hidden when every known widget type is already on the grid. A "Reset to default" link SHALL restore `defaultDashboardLayoutTemplate` and close the drawer.

#### Scenario: Reset restores the default layout

- GIVEN the user has rearranged widgets
- WHEN they activate reset to default
- THEN the grid SHALL return to `defaultDashboardLayoutTemplate`
- AND the updated layout SHALL be persisted (OP-DASH-11)

---

### Requirement: OP-DASH-11 -- Layout Persistence

The dashboard SHALL persist the sanitized layout template to `localStorage` under the key `hypershell.operational-dashboard.layout.v20`.

On mount, a saved template SHALL be loaded when it parses as valid JSON and contains an array entry for every responsive variant (`xl`, `lg`, `md`, `sm`). Invalid or corrupt saved state SHALL fall back to the default template without surfacing an error to the user.

Duplicate `widgetType` entries in a single variant SHALL be deduplicated on save (first occurrence wins).

When persistence fails (for example, storage quota exceeded), the dashboard SHALL continue to function in memory and SHALL publish a `dashboard.layout.template.persistence-failed` probe with outcome `failed`.

#### Scenario: Saved layout survives reload

- GIVEN the user removed the `pods` widget and the layout was saved
- WHEN they reload the browser
- THEN the grid SHALL render without the `pods` widget

---

### Requirement: OP-DASH-12 -- Gateway Status Widget

The `gateway-status` widget SHALL render a `GatewayStatusChart` donut using the shared `StatusDonutChart` primitive (`@patternfly/react-charts/victory` `ChartDonut`), driven by the `provisioned-gateways` metric's `status` and `value` fields.

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

Loading states SHALL use a `Spinner` with a localized `aria-label`. Empty and error states SHALL use PatternFly `EmptyState` or `Alert` with localized titles and bodies. Interactive trend and status icons SHALL expose localized `aria-label` values via `Tooltip` or button labels.

#### Scenario: Page description introduces live metrics and refresh behavior

- GIVEN the dashboard page loads
- WHEN the description paragraph renders
- THEN it SHALL summarize gateway fleet health, hub cluster capacity, and platform usage
- AND it SHALL state that metrics refresh every 15 minutes and that Refresh updates them immediately

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

The web console SHALL provide Storybook stories for default, loading, initial-load-error, and refresh-error states using mock or stub `DashboardControlPlane` adapters.

The host mock adapter (`createMockDashboardControlPlane`) MAY introduce an artificial delay for demo purposes; production adapters SHALL NOT.

#### Scenario: Storybook renders the full widget grid

- GIVEN `mockOperationalDashboardMetrics` is supplied to `OperationalDashboardPage`
- WHEN the default Storybook story renders
- THEN all default-layout widgets SHALL mount without calling the HyperShell API
