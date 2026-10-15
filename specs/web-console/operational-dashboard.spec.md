# Operational Dashboard

**Status:** Active
**Version:** 0.2.0
**Applies to:** `packages/operational-dashboard-ui`, `components/web-console` SPA and BFF, `packages/gateway-management-ui` (display-status aggregation), `components/sdk-typescript` (gateway list client)

## Purpose

Provide a widgetized operational dashboard in the HyperShell web console where users can assess the health of resources they are authorized to operate. The dashboard composes summary and detail widgets in a customizable grid layout. Live data is loaded through a narrow application port (`DashboardControlPlane`) implemented by the web-console host; widgets without a connected source remain on the page and render a localized unavailable state rather than being hidden. The approved overview design is represented by the shell-free `Parity/Dashboards/Operational` Storybook story in `specs/web-console/mockups/dashboards/operational-dashboard.parity.stories.tsx`.

This specification covers the reusable `operational-dashboard-ui` package, the host adapter that loads operational metrics from BFF Prometheus proxy routes and HyperShell REST where noted below, access-scoped dashboard data, SPA and BFF route surfaces, layout persistence, and the widget catalog. Platform inventory metrics (managed clusters and managed databases) are defined in `platform/platform-inventory.spec.md`. Prometheus gateway phase counts (`hypershell_gateways_total`, BFF `GET /api/metrics/gateways`) are defined in `platform/gateway-metrics-dashboard.spec.md` and consumed by this dashboard for `provisioned-gateways` and `gateway-status`. Gateway provision duration and provision outcomes are defined in `platform/gateway-provision-time.spec.md` and `platform/gateway-provision-outcomes.spec.md`. Gateway release adoption (`gateway-releases`, REST gateway-list aggregation grouped by observed `gateway_version`) is defined in `platform/gateway-release-distribution.spec.md` (OP-DASH-25). The embeddable `GatewayMetricsDashboard` component remains a separate surface. Remaining REST-to-Prometheus migration work is tracked in `packages/operational-dashboard-ui/PROMETHEUS_MIGRATION.md`.

The sibling **reliability dashboard** (`/dashboard/reliability`) lives in the same package and is specified in `web-console/reliability-dashboard.spec.md`. API request rate, 5xx error rate, and median latency are defined in `platform/api-reliability-metrics.spec.md` and SHALL NOT appear on this operational overview. Both dashboards share PatternFly secondary horizontal navigation; the reliability dashboard's existing `platform:admin` access gate remains unchanged.

The current overview is intentionally limited to seven widgets: Resource summary, Gateway status, Sandbox status, Active sandboxes, Provisioning snapshot, Gateway releases, and Attention required. The standalone Provisioning dashboard is specified in `web-console/provisioning-dashboard.spec.md`.

**Data-source direction:** Aggregate operational dashboard widgets SHALL prefer Prometheus-backed BFF routes over direct HyperShell REST list pagination. The API server exposes fleet-wide gauges from database state on its `/metrics` scrape; the BFF queries Prometheus and returns JSON to the browser adapter. HyperShell REST List APIs remain authoritative for resource collection pages (for example, the gateway table) and for metrics explicitly defined on REST list aggregation (for example, `gateway-releases`) but are not the preferred source for dashboard totals otherwise.

### Relationship to gateway metrics

| Concern                | Operational dashboard (this spec)                                                           | Gateway metrics dashboard                                                    |
| ---------------------- | ------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------- |
| Primary route          | `/dashboard` and `dashboard.*` host root (`/`)                                              | `GatewayMetricsDashboard` component (embeddable)                             |
| Peer reliability route | `/dashboard/reliability` (`web-console/reliability-dashboard.spec.md`)                      | Out of scope                                                                 |
| Gateway counts source  | BFF `GET /api/metrics/gateways` (Prometheus `hypershell_gateways_total`)                    | Same BFF route                                                               |
| Scope                  | Resources authorized for the signed-in user; `platform:admin` sees all resources            | Fleet-wide database aggregate (dashboard-operator access only)               |
| Status model           | Display buckets: `healthy`, `provisioning`, `degraded`, `failed` (mapped from phase labels) | Lifecycle phases: `Pending`, `Provisioning`, `Running`, `Degraded`, `Failed` |

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

### Requirement: OP-DASH-04 -- Access Control and Resource Scope

Access to the operational dashboard SHALL require an authenticated user with permission to view at least one dashboard resource. The legacy Keycloak realm role `hypershell-admins` SHALL NOT grant access on its own.

The dashboard SHALL apply authorization scope before calculating or returning aggregate metrics. The browser SHALL NOT receive fleet aggregates and filter those aggregates locally.

Users with gateway- or sandbox-scoped access SHALL see only gateways and sandboxes authorized for the signed-in user. A user with the `platform:admin` realm role SHALL see all gateways and sandboxes in the platform scope. This resource-scope rule is independent of dashboard-navigation visibility: a non-admin sees the dashboard widgets populated from the resources they can access, while an admin sees the platform-wide resource totals and details.

The dashboard SHALL display an `Admin` badge in the resource-summary widget title when the signed-in user has `platform:admin`. The badge SHALL NOT appear for non-admin users.

The SPA route module for `/dashboard` and the dashboard-host root (`/`) SHALL wrap the operational dashboard page in an authorization guard, which:

- Shows a localized access-denied `EmptyState` when OIDC is enabled and the user is unauthenticated
- Shows a localized access-denied `EmptyState` when the user is authenticated but lacks permission to view dashboard resources
- Renders children when OIDC is disabled (no-auth dev mode) or when the user has permission to view dashboard resources

The SPA route module for `/dashboard/reliability` SHALL retain the existing `RequireDashboardAdmin` guard.

When OIDC is enabled, the BFF SHALL enforce the same authenticated-access requirement for browser navigations to `/dashboard`, `/dashboard/reliability`, and for `/` on hosts whose hostname starts with `dashboard.`. Users without dashboard resource access SHALL receive the localized access-denied state. Reliability-route details are also specified in REL-DASH-04 / REL-DASH-05.

When OIDC is enabled, the BFF SHALL enforce authentication and resource authorization on every `GET /api/metrics/*` route consumed by the operational or reliability dashboard host adapters (`cluster-memory`, `cluster-cpu`, `cluster-pods`, `cluster-nodes`, `gateways`, `gateway-sandboxes`, `gateway-provision-duration`, `gateway-provision-outcomes`, `platform-inventory`, `registered-users` per OP-DASH-08, and `api-reliability` per ARM-03). The BFF SHALL apply the signed-in user's resource scope before returning metrics. Unauthenticated callers SHALL receive HTTP `401` or the standard BFF re-authentication response. Authenticated callers without access to any resource represented by a route SHALL receive HTTP `403` or an empty authorized-scope response according to the route contract. When OIDC is disabled (no-auth dev mode), these routes SHALL remain open to unauthenticated callers, matching page behavior.

#### Scenario: User without dashboard resource access is turned away from /dashboard

- GIVEN OIDC is enabled and the signed-in user has no permission to view a dashboard resource
- WHEN the user navigates to `/dashboard`
- THEN the BFF SHALL redirect away from the dashboard route
- AND the SPA SHALL render the access-denied empty state if the route still mounts

#### Scenario: Platform admin can open the dashboard

- GIVEN OIDC is enabled and the signed-in user has `platform:admin`
- WHEN the user navigates to `/dashboard`
- THEN the BFF SHALL serve `index.html` with HTTP `200`
- AND `OperationalDashboardPage` SHALL render

#### Scenario: User cannot fetch metrics outside the authorized scope

- GIVEN OIDC is enabled and the signed-in user is authorized for a subset of gateways
- WHEN the user sends `GET /api/metrics/gateways`
- THEN the BFF SHALL return aggregates calculated only from the authorized gateways

#### Scenario: Legacy hypershell-admins cannot fetch dashboard metrics without resource access

- GIVEN OIDC is enabled and the signed-in user has only `hypershell-admins` (no `platform:admin`)
- WHEN the user sends `GET /api/metrics/gateways`
- THEN the BFF SHALL respond with HTTP `403`

#### Scenario: Platform admin can fetch all dashboard metrics

- GIVEN OIDC is enabled and the signed-in user has `platform:admin`
- AND Prometheus returns successful instant-query results
- WHEN the user sends `GET /api/metrics/gateways`
- THEN the BFF SHALL respond with HTTP `200`

#### Scenario: Non-admin dashboard shows only authorized resources

- GIVEN OIDC is enabled and the signed-in user can view only a subset of gateways and sandboxes
- WHEN the user opens `/dashboard`
- THEN the dashboard SHALL render the Overview destination
- AND every summary, status, attention, release, and trend value SHALL be calculated only from the authorized gateways and sandboxes
- AND the user SHALL NOT see platform-wide totals for resources outside their scope

#### Scenario: Admin dashboard shows all platform resources

- GIVEN OIDC is enabled and the signed-in user has `platform:admin`
- WHEN the user opens `/dashboard`
- THEN every dashboard widget SHALL use the platform-wide authorized scope
- AND the summary, status, attention, release, and trend values SHALL include all platform gateways and sandboxes

---

### Requirement: OP-DASH-05 -- SPA and BFF Route Surfaces

The web console SPA SHALL expose the operational dashboard at `/dashboard` via a React Router route module that renders `OperationalDashboardPage`.

When the browser hostname is `dashboard.hypershell.localhost`, the SPA root route (`/`) SHALL also render `OperationalDashboardPage` (dashboard-dedicated host entry point).

`route-contract.json` SHALL declare `"dashboard": "dashboard"`. The BFF SHALL treat `/dashboard` as a valid application route that returns `index.html` for direct navigation and refresh, alongside `/`, `/login`, `/gateways/new`, and `/gateways/:gatewayId`.

The reliability dashboard route `/dashboard/reliability` is specified in REL-DASH-05. Both dashboard pages SHALL render the shared secondary horizontal navigation defined in OP-DASH-26.

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

### Requirement: OP-DASH-26 -- Secondary Horizontal Navigation to Reliability

`OperationalDashboardPage` SHALL render the shared PatternFly secondary horizontal navigation defined in `web-console/reliability-dashboard.spec.md` REL-DASH-06.

On `/dashboard`, the **Operational** item SHALL be selected and the **Reliability** item SHALL navigate to `/dashboard/reliability`.

This requirement exists so the operational overview participates in the same cross-dashboard chrome as the reliability page without hosting reliability metrics on this route.

#### Scenario: Operational page shows reliability as a peer destination

- GIVEN an authenticated dashboard administrator is viewing `/dashboard`
- WHEN the secondary horizontal navigation renders
- THEN **Operational** SHALL be selected
- AND activating **Reliability** SHALL navigate to `/dashboard/reliability`

### Requirement: OP-DASH-31 -- Dashboard Availability by Role

The dashboard navigation SHALL expose exactly four implemented dashboard destinations:
**Overview**, **Provisioning**, **Inventory**, and **Reliability**.

The navigation SHALL expose all four destinations to a user with the `platform:admin` role.

The navigation SHALL expose only **Overview** to an authenticated user without the `platform:admin` role.

The navigation SHALL not expose an admin-only dashboard destination to a non-admin user as a disabled item or a hidden overflow item.

#### Scenario: Non-admin sees the restricted dashboard set

- GIVEN an authenticated user without the `platform:admin` role
- WHEN the user views any dashboard secondary navigation
- THEN the navigation SHALL show **Overview**
- AND the navigation SHALL NOT show **Provisioning**, **Inventory**, or **Reliability**

#### Scenario: Administrator sees every dashboard

- GIVEN an authenticated user with the `platform:admin` role
- WHEN the user views any dashboard secondary navigation
- THEN the navigation SHALL show **Overview**, **Provisioning**, **Inventory**, and **Reliability**

---

---

### Requirement: OP-DASH-06 -- Gateway Sandbox Metrics Adapter

The host `DashboardControlPlane` adapter SHALL load `provisioned-sandboxes` from BFF `GET /api/metrics/gateway-sandboxes` in the `gateway-metrics` metric source (OP-DASH-23).

The BFF route SHALL query Prometheus for `hypershell_gateways_active_sandboxes_total`, a gauge emitted by the API server metrics collector, and SHALL aggregate only the gateways in the signed-in user's authorized scope (see `openshell-gateway-sandbox-count.spec.md` for field semantics).

The adapter SHALL emit a `provisioned-sandboxes` metric whose `value` is the stringified `active_sandboxes` count from the BFF response. The value SHALL NOT be a non-finite number.

When BFF `hourly_active_sandboxes` and `daily_active_sandboxes` are present, the adapter SHALL also emit optional `hourlyTrend` and `trend` per `platform/gateway-sandbox-active-trends.spec.md` GSAT-06. When a historical field is absent, the adapter SHALL omit the corresponding trend field only.

The `provisioned-sandboxes` metric SHALL remain connected for the Resource summary and Active sandboxes widgets. The standalone `provisioned-sandboxes` grid widget is retired from the default layout and widget catalog.

A non-success BFF response for gateway sandboxes SHALL fail the entire `gateway-metrics` source (including `provisioned-gateways`, `provision-time`, and `provision-reliability`). Access control is enforced at the BFF route (dashboard-operator roles per OP-DASH-04).

#### Scenario: Prometheus sandbox count populates provisioned-sandboxes

- GIVEN `GET /api/metrics/gateway-sandboxes` returns `{ "active_sandboxes": 5 }`
- WHEN `getOperationalMetrics` runs
- THEN the adapter SHALL emit `provisioned-sandboxes` with `value: "5"`
- AND the dashboard SHALL NOT display `NaN`

#### Scenario: Gateway sandboxes failure omits gateway-metrics source

- GIVEN `GET /api/metrics/gateway-sandboxes` fails
- WHEN the adapter processes the `gateway-metrics` source
- THEN the `gateway-metrics` source SHALL be treated as failed
- AND `provisioned-sandboxes`, `provisioned-gateways`, `provision-time`, and `provision-reliability` SHALL be omitted
- AND the dashboard SHALL NOT synthesize a zero sandbox count

---

### Requirement: OP-DASH-23 -- Gateway Prometheus Metrics Adapter

The host `DashboardControlPlane` adapter SHALL load `provisioned-gateways` (see OP-DASH-07), `provisioned-sandboxes` (OP-DASH-06), and optionally `provision-time` and `provision-reliability` from BFF Prometheus proxy routes in the `gateway-metrics` metric source:

- `GET /api/metrics/gateways` - authorized-scope phase counts from `hypershell_gateways_total` (see `platform/gateway-metrics-dashboard.spec.md` DASH-05)
- `GET /api/metrics/gateway-sandboxes` - authorized-scope active sandbox sum from `hypershell_gateways_active_sandboxes_total` (OP-DASH-06)
- `GET /api/metrics/gateway-provision-duration` - provision duration histogram (see `platform/gateway-provision-time.spec.md`)
- `GET /api/metrics/gateway-provision-outcomes` - current provision success/failure counts and daily success-rate series for the selected range (see `platform/gateway-provision-outcomes.spec.md`)

The adapter SHALL reuse the shared gateway-management-ui gateway-metrics data source for normalized phase counts and optional fleet totals. It SHALL NOT duplicate the BFF route fetch or phase normalization. A non-success BFF response for gateways or gateway sandboxes SHALL fail the entire `gateway-metrics` source.

`provision-time` SHALL be appended to the same source only when the provision-duration BFF route succeeds. When the route fails or returns no qualifying samples, `provision-time` SHALL be omitted while `provisioned-gateways` and `provisioned-sandboxes` MAY still be emitted.

`provision-reliability` SHALL be appended to the same source only when the provision-outcomes BFF route succeeds and `success_rate_percent` is non-null (GPO-05, GPO-07). When the route fails or returns `success_rate_percent: null`, `provision-reliability` SHALL be omitted while `provisioned-gateways`, `provisioned-sandboxes`, and `provision-time` MAY still be emitted.

The adapter SHALL fetch provision duration and provision outcomes concurrently. Failure of one route SHALL NOT prevent emission of the other when that route succeeds.

Gateway phase counts SHALL be calculated only from gateways in the signed-in user's authorized scope. A `platform:admin` user SHALL receive all platform gateways. Access control SHALL be enforced before aggregation at the BFF route (OP-DASH-04).

#### Scenario: Prometheus gateway counts populate provisioned-gateways

- GIVEN `GET /api/metrics/gateways` returns `{ "counts": { "Running": 10, "Provisioning": 3, "Degraded": 1, "Failed": 4, "Pending": 2 } }`
- WHEN `getOperationalMetrics` runs
- THEN the adapter SHALL emit `provisioned-gateways` with `value: "20"`
- AND `status` SHALL map phases to display buckets per OP-DASH-07

#### Scenario: Prometheus gateway failure omits gateway-derived Prometheus metrics

- GIVEN `GET /api/metrics/gateways` fails
- WHEN the adapter processes the `gateway-metrics` source
- THEN the `gateway-metrics` source SHALL be treated as failed
- AND `provisioned-gateways`, `provisioned-sandboxes`, `provision-time`, and `provision-reliability` SHALL be omitted

#### Scenario: Provision outcomes populate provision-reliability

- GIVEN `GET /api/metrics/gateway-provision-outcomes` returns current success/failure counts and daily success-rate points for the selected range
- WHEN `getOperationalMetrics` runs
- THEN the adapter SHALL emit `provision-reliability` with `value: "90.0"`
- AND `provisionOutcomes` SHALL include `successCount24h: "9"`, `failureCount24h: "1"`, and `successRatePercent: "90.0"`
- AND `successRateTrend.points` SHALL contain daily entries for the selected range

#### Scenario: Provision outcomes unavailable does not hide provision duration

- GIVEN `GET /api/metrics/gateway-provision-duration` succeeds
- AND `GET /api/metrics/gateway-provision-outcomes` fails
- WHEN `getOperationalMetrics` runs
- THEN `provision-time` SHALL still be emitted
- AND `provision-reliability` SHALL be omitted

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

| Metric ID               | Connected in v1 | Source when connected                                                                                                                                                                   |
| ----------------------- | --------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `provisioned-gateways`  | Yes             | BFF `GET /api/metrics/gateways` (Prometheus); OP-DASH-23, OP-DASH-07; optional 7-day fleet-total `trend` per `platform/gateway-fleet-total-trend.spec.md`                               |
| `provisioned-sandboxes` | Yes             | BFF `GET /api/metrics/gateway-sandboxes` (Prometheus); OP-DASH-06; optional 24-hour hourly and 7-day daily `hourlyTrend` / `trend` per `platform/gateway-sandbox-active-trends.spec.md` |
| `registered-users`      | Yes             | BFF `GET /api/metrics/registered-users` (Prometheus); adoption totals, added counts, unique-login rollups, and daily `trend` per `platform/registered-users.spec.md`                    |
| `memory`                | Yes             | BFF `GET /api/metrics/cluster-memory` (Prometheus node-exporter); daily used GiB trend for the selected 7-day or 30-day range                                                           |
| `nodes`                 | Yes             | BFF `GET /api/metrics/cluster-nodes` (Prometheus kube-state-metrics); see `platform/cluster-nodes.spec.md`                                                                              |
| `cpu`                   | Yes             | BFF `GET /api/metrics/cluster-cpu` (Prometheus node-exporter); daily used cores trend for the selected 7-day or 30-day range                                                            |
| `pods`                  | Yes             | BFF `GET /api/metrics/cluster-pods` (Prometheus kube-state-metrics); current state only on the Provisioning dashboard                                                                   |
| `provision-time`        | Yes             | BFF `GET /api/metrics/gateway-provision-duration` (Prometheus control-plane histogram); daily P50 and P95 trends for the selected 7-day or 30-day range                                 |
| `provision-reliability` | Yes             | BFF `GET /api/metrics/gateway-provision-outcomes` (Prometheus control-plane counter); daily success-rate trend for the selected 7-day or 30-day range                                   |
| `managed-clusters`      | Yes             | BFF `GET /api/metrics/platform-inventory` (Prometheus); see `platform/platform-inventory.spec.md`                                                                                       |
| `gateway-releases`      | Yes             | HyperShell API gateway list aggregation grouped by observed `gateway_version` (SDK); see `platform/gateway-release-distribution.spec.md` (OP-DASH-25)                                   |

Widgets for placeholder metrics SHALL remain in the default layout and in the add-widgets drawer. When a metric ID is missing from the adapter response - whether because the metric is not yet connected or because its data source failed (OP-DASH-19) - the widget body SHALL render a localized "Metric unavailable" empty state (title and recovery guidance) instead of failing the entire dashboard.

Summary rows (`usage-summary`, `system-summary`) that reference a missing metric SHALL render the same localized metric-unavailable message in place of the value instead of omitting the row or showing a blank cell.

Historical trend data SHALL use UTC calendar days. The Provisioning dashboard SHALL load daily trend data for Memory, CPU, Provision time, and Provision reliability for the selected 7-day or 30-day range. The Provisioning dashboard SHALL omit a trend point when the source has no sample for that day and SHALL render a visible gap. The Provisioning dashboard SHALL NOT render a Pods trend.

The package SHALL maintain `DATA_SOURCES.md` documenting connected vs placeholder metrics and the adapter update procedure.

#### Scenario: Placeholder widget shows unavailable state

- GIVEN the adapter returns only `provisioned-gateways` and `provisioned-sandboxes`
- WHEN the `cpu` widget is on the layout
- THEN it SHALL render the localized metric-unavailable empty state
- AND the rest of the dashboard SHALL remain interactive

---

### Requirement: OP-DASH-32 -- Operational Overview Summary

The operational overview SHALL include a **Resource summary** widget containing rows for Gateways, Sandboxes, Healthy resources, and Resources needing attention.

The summary SHALL calculate every row from the same authorized resource scope used by the dashboard metric routes.

The summary SHALL show an up or down trend indicator only for the Gateways and Sandboxes rows. The summary SHALL reserve the same value-column width for rows with and without an indicator so all numeric values align.

The Resources needing attention row SHALL show a red danger icon when its value is greater than zero and SHALL show no danger icon when its value is zero.

When the signed-in user has `platform:admin`, the widget title SHALL display an `Admin` badge. The badge SHALL identify the scope of the displayed data and SHALL NOT change the underlying authorization behavior.

#### Scenario: Summary respects resource scope

- GIVEN the signed-in user can view two gateways and three sandboxes
- WHEN the operational overview renders
- THEN the summary counts SHALL include only those authorized resources

#### Scenario: Summary shows admin scope badge

- GIVEN the signed-in user has `platform:admin`
- WHEN the operational overview renders
- THEN the Resource summary title SHALL include an `Admin` badge

#### Scenario: Summary shows attention danger state

- GIVEN one or more authorized resources need attention
- WHEN the operational overview renders
- THEN the Resources needing attention row SHALL show a red danger icon

---

### Requirement: OP-DASH-28 -- Overview Time Ranges

The operational overview SHALL allow the user to select **Last 7 days** or **Last 30 days**.

The selected range SHALL apply to every widget that displays historical trend data on the overview.

Instant status and inventory values SHALL remain current snapshots and SHALL NOT be replaced with historical totals when the user changes the range.

The overview SHALL NOT offer a 90-day range until the platform confirms the required historical retention and query performance.

When a selected range has insufficient historical data, the affected widget SHALL show the available data or its localized unavailable state; the dashboard SHALL remain usable.

#### Scenario: User selects a 30-day range

- GIVEN the overview is displaying the Last 7 days range
- WHEN the user selects Last 30 days
- THEN historical trend widgets SHALL request and display 30 days of data
- AND current status widgets SHALL continue to show current values

#### Scenario: User cannot select an unsupported range

- GIVEN the overview range control is rendered
- WHEN the user opens the range options
- THEN the available options SHALL be Last 7 days and Last 30 days
- AND Last 90 days SHALL NOT be offered

---

### Requirement: OP-DASH-29 -- Active Sandboxes Widget

The overview SHALL include an **Active sandboxes** widget in place of a generic Recent activity widget.

The widget SHALL display the authorized active-sandbox count over the selected historical range using daily points.

The widget SHALL use the label **Active sandboxes** and SHALL NOT describe the series as generic activity, source freshness, or all-sources-current status.

The widget SHALL remain available when the active-sandbox metric is available even if another dashboard metric source fails.

---

### Requirement: OP-DASH-30 -- Attention Required Widget

The overview SHALL include an **Attention required** widget below the Resource summary widget.

The widget SHALL show grouped counts for authorized resources requiring attention, including failed gateways, degraded gateways, expiring sandboxes, and a combined **Idle or orphaned sandboxes** row when those counts are available.

The widget SHALL show grouped counts rather than individual resource identities in the initial overview release. Individual resource rows are out of scope until the API exposes authorized resource identifiers and attention reasons.

The widget SHALL present grouped counts only. Resource-list navigation is out of scope for this overview release.

#### Scenario: Attention widget shows grouped counts

- GIVEN the authorized scope contains one failed gateway and four expiring sandboxes
- WHEN the overview renders
- THEN the Attention required widget SHALL show those grouped counts

#### Scenario: No resources need attention

- GIVEN every authorized gateway and sandbox is healthy or active
- WHEN the overview renders
- THEN the Attention required widget SHALL show zero attention items and no danger icon in the summary row

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

| Source                                                                                                                                                                                                          | Metric IDs affected                                                                                              |
| --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------- |
| BFF `gateway-metrics` fetch (`GET /api/metrics/gateways`, `GET /api/metrics/gateway-sandboxes`, optional `GET /api/metrics/gateway-provision-duration`, optional `GET /api/metrics/gateway-provision-outcomes`) | `provisioned-gateways`, `provisioned-sandboxes`, `provision-time` (optional), `provision-reliability` (optional) |
| BFF `GET /api/metrics/registered-users` (`registered-users`)                                                                                                                                                    | `registered-users`                                                                                               |
| BFF `GET /api/metrics/cluster-memory`                                                                                                                                                                           | `memory`                                                                                                         |
| BFF `GET /api/metrics/cluster-cpu`                                                                                                                                                                              | `cpu`                                                                                                            |
| BFF `GET /api/metrics/cluster-pods`                                                                                                                                                                             | `pods`                                                                                                           |
| BFF `GET /api/metrics/cluster-nodes`                                                                                                                                                                            | `nodes`                                                                                                          |
| BFF `GET /api/metrics/platform-inventory` (`platform-inventory`)                                                                                                                                                | `managed-clusters`                                                                                               |
| HyperShell API gateway list grouped by observed `gateway_version` (`gateway-release-distribution`)                                                                                                              | `gateway-releases`                                                                                               |

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
- AND gateway, sandbox, inventory, provision-time, provision-reliability, and cluster metric widgets SHALL render the localized metric-unavailable state
- AND a warning `Alert` SHALL explain that some metrics could not be loaded

#### Scenario: Gateway metrics failure does not hide cluster metrics

- GIVEN every BFF cluster-metrics request succeeds
- AND the `gateway-metrics` source fails (for example, `GET /api/metrics/gateways` returns HTTP `502`)
- WHEN the operator opens `/dashboard`
- THEN registered-user, inventory, and cluster metric widgets SHALL display loaded values
- AND gateway count, sandbox, provision-time, and provision-reliability widgets SHALL render the localized metric-unavailable state
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

#### Scenario: No qualifying provision-outcomes omit only provision reliability

- GIVEN `GET /api/metrics/gateways` succeeds
- AND `GET /api/metrics/gateway-provision-outcomes` fails or returns `success_rate_percent: null`
- WHEN `getOperationalMetrics` runs
- THEN `provisioned-gateways` SHALL still be emitted
- AND `provision-reliability` SHALL be omitted
- AND the provision-reliability widget and system-summary success-rate row SHALL render the localized metric-unavailable state
- AND the dashboard SHALL NOT enter the total load-error state

#### Scenario: Gateway metrics failure does not hide release distribution

- GIVEN gateway list and gateway release list aggregation succeed
- AND the `gateway-metrics` Prometheus source fails
- WHEN the operator opens `/dashboard`
- THEN the Gateway releases widget SHALL display loaded release rows
- AND gateway status and usage-summary gateway rows SHALL render the localized metric-unavailable state

#### Scenario: Gateway list failure omits only release distribution

- GIVEN every other metric source succeeds
- AND gateway list aggregation fails
- WHEN the operator opens `/dashboard`
- THEN the Gateway releases widget SHALL render the localized metric-unavailable state
- AND other connected widgets SHALL remain populated

---

### Requirement: OP-DASH-10 -- Widgetized Grid Layout

The dashboard SHALL use PatternFly's `@patternfly/widgetized-dashboard` (`GridLayout`, `WidgetDrawer`, `AddWidgetsButton`) with a four-column grid on `xl`, `lg`, and `md` breakpoints and a single-column stack on `sm`.

The default overview layout template (`defaultDashboardLayoutTemplate`) SHALL place the compact overview widgets from the approved operational parity story. The overview SHALL NOT include the Provisioning dashboard widgets; those widgets belong on `/dashboard/provisioning`.

| Widget type             | Layout item `i`           | Default position (4-column) |
| ----------------------- | ------------------------- | --------------------------- |
| `resource-summary`      | `resource-summary#1`      | Columns 0-1, row 1          |
| `gateway-status`        | `gateway-status#1`        | Column 2, row 1             |
| `sandbox-status`        | `sandbox-status#1`        | Column 3, row 1             |
| `active-sandboxes`      | `active-sandboxes#1`      | Columns 0-1, row 2          |
| `provisioning-snapshot` | `provisioning-snapshot#1` | Column 2, row 2             |
| `gateway-releases`      | `gateway-releases#1`      | Column 3, row 2             |
| `attention-required`    | `attention-required#1`    | Columns 0-1, row 3          |

On `sm`, the seven overview widgets SHALL stack in the table order above. The dashboard SHALL retain widgetized-dashboard drag, resize, remove, add-widget, reset, and sanitized local persistence behavior at every supported breakpoint.

The dashboard SHALL place the manual refresh control and relative update label together beside the secondary navigation. The range selector and reset/add-widget controls SHALL appear above the grid, with reset and add-widget controls aligned to the right when space permits. The update label SHALL use `Updated under 1 minute ago`, `Updated 1 minute ago`, or `Updated X minutes ago` based on elapsed time since the last successful refresh.

Widget titles in the layout template SHALL be localized via `localizeDashboardLayoutTemplate` using the package `messages` catalog.

Users SHALL be able to add widgets from the drawer, drag to rearrange, and remove widgets. The add-widgets button SHALL be hidden when every known widget type is already on the grid. A "Reset to default" link SHALL restore `defaultDashboardLayoutTemplate` and close the drawer.

#### Scenario: Reset restores the default layout

- GIVEN the user has rearranged widgets
- WHEN they activate reset to default
- THEN the grid SHALL return to `defaultDashboardLayoutTemplate`
- AND the updated layout SHALL be persisted (OP-DASH-11)

---

### Requirement: OP-DASH-11 -- Layout Persistence

The dashboard SHALL persist the sanitized layout template to `localStorage` under the key `hypershell.operational-dashboard.layout.v47`.

Each default-layout change that would leave saved positions incompatible with the new template SHALL bump the layout persistence key so browsers load the updated default instead of a stale saved grid. Documented bumps:

| Key   | Trigger                                                                                                                                                                    |
| ----- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `v23` | Section title widgets (OP-DASH-20)                                                                                                                                         |
| `v30` | Platform inventory section and default inventory widgets (OP-DASH-22, OP-DASH-21)                                                                                          |
| `v31` | Taller `registered-users` widget for activity stats rows (RU-10)                                                                                                           |
| `v32` | `registered-users` spans two columns; `gateway-status` compact single column                                                                                               |
| `v35` | Hub cluster two-row grid; legacy `provision-reliability` widget; taller `system-summary`                                                                                   |
| `v37` | Retire `provisioned-sandboxes` grid widget; add `gateway-releases` below `gateway-status` in column 3 (OP-DASH-25)                                                         |
| `v38` | Platform inventory widgets on one row: inventory summary, cluster providers, cluster regions (one column), database status (OP-DASH-22)                                    |
| `v39` | Taller `usage-summary`, `gateway-status`, `memory`, `cpu`, and `pods` widgets for HYPERSHELL-281 trend sparklines (GFT-06, GSAT-07, HCUT-08)                               |
| `v40` | Combined v38 inventory layout and v39 trend sparkline widget heights                                                                                                       |
| `v41` | Hub utilization widgets render through `UtilizationCard` with sparklines; widget mapping defaults use `UTILIZATION_WIDGET_HEIGHT` / `POD_CAPACITY_WIDGET_HEIGHT` (HCUT-08) |
| `v42` | Remove the `managed-database-status` widget and widen `managed-cluster-regions` back across columns 2-3; the ManagedDatabase resource no longer exists (OP-DASH-22)        |
| `v43` | Taller `sandbox-status` for Attention required labels (SSA-09)                                                                                                             |
| `v44` | Align `gateway-releases` height with `sandbox-status` on adoption row 2                                                                                                    |
| `v46` | Replace the legacy multi-section dashboard with the seven-widget operational overview while preserving widgetized customization                                            |
| `v47` | Apply the current overview widget heights and spacing, including the taller resource summary, status, gateway-release, and attention widgets                               |

On mount, a saved template SHALL be loaded when it parses as valid JSON and contains an array entry for every responsive variant (`xl`, `lg`, `md`, `sm`). Invalid or corrupt saved state SHALL fall back to the default template without surfacing an error to the user.

Saved layouts SHALL be passed through `stripRemovedWidgetTypes` before render and persistence. Retired widget types (`provisioned-sandboxes`, `managed-database-status`, legacy inventory number tiles, and other entries in `REMOVED_WIDGET_TYPES`) SHALL be removed from saved templates so stale tiles do not reappear after catalog changes.

Duplicate layout item `i` values in a single variant SHALL be deduplicated on save (first occurrence wins). Duplicate metric `widgetType` entries (every type except `section-title`) SHALL also be deduplicated on save (first occurrence wins). Multiple `section-title` instances with distinct `i` values SHALL be preserved.

When persistence fails (for example, storage quota exceeded), the dashboard SHALL continue to function in memory and SHALL publish a `dashboard.layout.template.persistence-failed` probe with outcome `failed`.

#### Scenario: Saved layout survives reload

- GIVEN the user removed the `pods` widget and the layout was saved
- WHEN they reload the browser
- THEN the grid SHALL render without the `pods` widget

---

### Requirement: OP-DASH-12 -- Gateway Status Widget

The `gateway-status` widget SHALL render a `GatewayStatusChart` donut using the shared `StatusDonutChart` primitive (`@patternfly/react-charts/victory` `ChartDonut`), driven by the `provisioned-gateways` metric's `status` and `value` fields.

The default layout template SHALL place `gateway-status` at `GATEWAY_STATUS_WIDGET_HEIGHT`. The sandbox-status widget SHALL use the same height. Gateway releases SHALL use its compact stat-panel height, currently equal to the status-widget height.

The chart SHALL:

- Include only buckets with count greater than zero
- Use status colors aligned with PatternFly alert/label semantics (healthy: chart green, provisioning: chart blue, degraded: warning yellow, failed: danger red)
- Resize with its container via `ResizeObserver`
- Expose localized `ariaTitle` and `ariaDesc` attributes
- Show the total gateway count as the chart title and a localized "Gateways" subtitle
- Use gateway-specific bucket labels (`Healthy`, `Provisioning`, `Degraded`, `Failed`)

When `status` is absent or all bucket counts are zero, the chart area SHALL render nothing (the widget shell remains).

If the metric includes `trend` data, a `TrendSparklineChart` SHALL render below the donut with caption **Last 7 days** (`platform/gateway-fleet-total-trend.spec.md` GFT-06); otherwise the sparkline SHALL be omitted.

#### Scenario: Degraded and failed counts appear in gateway status

- GIVEN the gateway status metric has 2 failed and 3 degraded gateways
- WHEN the Gateway status widget renders
- THEN failed and degraded segments SHALL appear with danger and warning colors respectively
- AND healthy-only fleets SHALL omit those zero-count segments

---

### Requirement: OP-DASH-16 -- Status Donut Presentation

Gateway and node inventory metrics that expose `OperationalMetric.value` plus `OperationalMetric.status` SHALL share a common status-donut presentation stack in `packages/operational-dashboard-ui`:

| Layer                                            | Responsibility                                                                                 |
| ------------------------------------------------ | ---------------------------------------------------------------------------------------------- |
| `StatusDonutChart`                               | Shared `ChartDonut` shell: resize handling, padding, legend layout, and dark-mode label tokens |
| `buildStatusDonutData`                           | Shared series builder that omits zero-count buckets                                            |
| `buildGatewayStatusData` / `buildNodeStatusData` | Domain-specific bucket order, colors, and localized labels                                     |
| `GatewayStatusChart` / `NodeStatusChart`         | Thin wrappers that map each metric to the shared chart                                         |

`GatewayStatusChart` SHALL remain the presentation for the `gateway-status` widget (`provisioned-gateways` metric). `NodeStatusChart` SHALL be the presentation for the `nodes` widget and metric.

Node status donuts SHALL reuse the same `status.healthy` and `status.failed` keys as the adapter mapping in `platform/cluster-nodes.spec.md` CLN-05, but SHALL render localized **Ready** and **Not ready** labels instead of gateway vocabulary.

`NodeStatusCard` SHALL wrap `NodeStatusChart` with the same card shell used by `GatewayStatusCard`. The default layout template SHALL include a `nodes` widget (OP-DASH-10) at `NODE_STATUS_WIDGET_HEIGHT` (shorter than `memory`, `cpu`, and `pods`, which use sparkline-capable heights per HCUT-08). The `nodes` widget SHALL NOT render a trend sparkline. `NodeStatusChart` SHALL use the compact `StatusDonutChart` size without a chart subtitle (the widget title already identifies the metric) and reduced card-body padding so the donut is not clipped.

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

The default layout template SHALL place the `pods` widget at `POD_CAPACITY_WIDGET_HEIGHT` (taller than `nodes` to fit the capacity donut and 7-day usage trend sparkline per `platform/hub-cluster-utilization-trends.spec.md` HCUT-08).

When `pods.trend.points` contains at least two entries, the `pods` widget SHALL render a `TrendSparklineChart` below the capacity donut with caption **Last 7 days** (HCUT-08). When `trend` is absent or has fewer than two points, the widget SHALL omit the sparkline with no error state.

When `podPhases` is absent, `PodCapacityChart` SHALL render nothing while its widget shell remains.

#### Scenario: Pod capacity donut shows phase and unused segments

- GIVEN the `pods` metric has `value: "548"`, `total: "2000"`, `unit: "pods"`, and `podPhases: { running: 500, pending: 12, succeeded: 20, failed: 16, unknown: 0 }`
- WHEN `PodCapacityChart` renders
- THEN the donut SHALL show phase segments matching `podPhases` and an Unused segment of `1452`
- AND the chart center title SHALL show `548`
- AND the chart subtitle SHALL read "of 2000 pods"
- AND the widget title SHALL be localized "Pods"

#### Scenario: Pods widget shows capacity donut and usage trend sparkline

- GIVEN the `pods` metric has instant capacity and phase fields
- AND `trend.points` contains 7 daily used pod counts
- WHEN the pods widget renders
- THEN a capacity donut SHALL appear
- AND a usage trend sparkline SHALL appear below it with caption **Last 7 days**

---

### Requirement: OP-DASH-13 -- Metric and Utilization Widgets

**Metric cards** (`MetricCard`) SHALL center a large heading with the metric value and an optional subtitle. When `trend.points` is present, a `TrendSparklineChart` SHALL render beneath the heading.

**Utilization widgets** (`cpu`, `memory`) SHALL render `UtilizationCard`, which wraps `UtilizationChart` when the metric includes both `unit` and `total`. Utilization percentage SHALL be `round((value / total) * 100)`. Status icons SHALL use thresholds: warning at `>= 60%`, danger at `>= 90%`.

When `trend.points` contains at least two entries, `UtilizationCard` SHALL render a `TrendSparklineChart` below the utilization chart with caption **Last 7 days** (`platform/hub-cluster-utilization-trends.spec.md` HCUT-08).

The `pods` widget SHALL use `PodCapacityChart` (OP-DASH-17), not `UtilizationChart`. When `pods.trend.points` contains at least two entries, the pods widget SHALL render a `TrendSparklineChart` below the capacity donut with caption **Last 7 days** (HCUT-08). The `system-summary` pods row SHALL continue to show utilization percentage from the same `pods` metric and SHALL show a failed pod count when `podPhases.failed` is non-zero.

**Summary widgets:**

- `usage-summary` - horizontal `DescriptionList` for users (total registered count with optional unique-login trend arrow per `platform/registered-users.spec.md` RU-13), gateways (with exception status counts and optional fleet-total trend arrow per `platform/gateway-fleet-total-trend.spec.md` GFT-07), and sandboxes (with optional hourly and daily active-sandbox sparklines and trend arrow per `platform/gateway-sandbox-active-trends.spec.md` GSAT-07)
- `system-summary` - legacy compatibility widget for saved layouts; Provisioning dashboard requirements are defined in `web-console/provisioning-dashboard.spec.md`

Trend direction indicators in summary rows SHALL appear only when `getMetricTrendChange` detects at least a 5% change between the first and last trend point. The sandboxes row SHALL prefer `hourlyTrend` over `trend` when both qualify (GSAT-07).

When `provisioned-sandboxes.hourlyTrend.points` contains at least two entries, the usage summary sandboxes row SHALL render a `TrendSparklineChart` with caption **Last 24 hours** below the count. When `provisioned-sandboxes.trend.points` contains at least two entries, the row SHALL render a second `TrendSparklineChart` with caption **Last 7 days** below the hourly sparkline when present, or below the count when hourly trend is absent (GSAT-07). These overview rules do not apply to the Provisioning dashboard.

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

- GIVEN the `provision-time` metric has `unit: "sec"`, `value: "315.00"`, and `provisionDuration: { mean: "315.00", p50: "288.00", p95: "726.00" }`
- WHEN the system summary provision-duration rows render
- THEN three localized rows SHALL appear for average, P50, and P95

#### Scenario: Provision success rate appears in system summary

- GIVEN the `provision-reliability` metric has `value: "90.0"` and `provisionOutcomes: { successCount24h: "9", failureCount24h: "1", successRatePercent: "90.0" }`
- WHEN the system summary success-rate row renders
- THEN a localized **Success rate (24h)** row SHALL show `90.0%`

---

---

### Requirement: OP-DASH-14 -- Localization and Accessibility

All user-visible dashboard strings SHALL be declared with `defineMessages` in the operational-dashboard-ui package and rendered through `react-intl`. No literal user-facing string SHALL appear in JSX.

The web-console host SHALL extract dashboard message IDs into its `locales/en.json` catalog for production rendering.

Loading states SHALL use a `Spinner` with a localized `aria-label` on the initial load only (before any metrics are available). Empty, partial-failure, and total-failure states SHALL use PatternFly `EmptyState` or `Alert` with localized titles and bodies. Partial-failure warnings SHALL use the `warning` alert variant; total initial-load failure SHALL use the `danger` variant. The operational-dashboard-ui package SHALL declare localized `partialLoadWarningTitle` and `partialLoadWarningBody` messages (or equivalent IDs) for the warning shown when one or more metric sources fail but the grid still renders. Refresh-time partial failures MAY reuse the same warning copy or dedicated refresh-partial-failure messages; in all cases the alert SHALL state that some metrics could not be loaded or refreshed and that the dashboard may be incomplete or stale. Interactive trend and status icons SHALL expose localized `aria-label` values via `Tooltip` or button labels.

#### Scenario: Page description introduces the operational overview

- GIVEN the dashboard page loads
- WHEN the description paragraph renders
- THEN it SHALL describe the current operational questions: what needs attention, whether gateways and sandboxes are healthy, and whether provisioning or capacity changed enough to investigate
- AND the page controls SHALL provide a manual Refresh action and the selected historical range

---

### Legacy compatibility: Section-title widgets

The `section-title` widget type is retained only so older saved layouts can be sanitized safely. It is not part of the current overview default layout or add-widgets catalog. It does not load metrics.

Legacy layouts may contain these historical section-title instances:

| Layout item `i`                   | Localized title   | Message ID                                    |
| --------------------------------- | ----------------- | --------------------------------------------- |
| `section-title#platform-adoption` | Platform adoption | `app.dashboard.sectionTitle.platformAdoption` |
| `section-title#hub-cluster`       | Hub cluster       | `app.dashboard.sectionTitle.hubCluster`       |

Each `section-title` layout item SHALL span all four columns (`w: 4`), use height `TITLE_WIDGET_HEIGHT` (one grid unit), and sit on its own row above the widgets in that section.

`localizeDashboardLayoutTemplate` SHALL resolve each instance's visible title from its layout item `i` via `SECTION_TITLE_MESSAGE_BY_ID`. Unknown `i` values SHALL keep the template's fallback `title` string.

The `section-title` widget body SHALL render `SectionTitleCard` with the resolved title. Presentation SHALL:

- Hide the widget grid tile header (no drag handle, kabob menu, or title bar)
- Suppress resize handles on the grid item
- Render a borderless, transparent card shell (no box shadow)
- Style the label as uppercase, subtle gray, small-caps typography (`.hypershell-dashboard-section-title`)

Existing saved section-title entries may continue to render using their localized legacy labels, but users cannot add new section-title widgets from the current overview catalog.

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

The operational-dashboard-ui package SHALL ship `mockOperationalDashboardMetrics` containing all widget metric IDs with representative fields matching production adapter output (`status`, `podPhases`, `unit`, `total`, and `trend` where applicable). Fixtures for hub-cluster `memory`, `cpu`, and `pods` SHALL include representative 7-day `trend.points` so Storybook and unit tests exercise usage trend sparklines (`platform/hub-cluster-utilization-trends.spec.md` HCUT-10).

The web console SHALL provide Storybook stories for default, loading, partial-load-warning, total-initial-load-error, and refresh-partial-failure states using mock or stub `DashboardControlPlane` adapters.

The host mock adapter (`createMockDashboardControlPlane`) MAY introduce an artificial delay for demo purposes; production adapters SHALL NOT.

#### Scenario: Storybook renders the full widget grid

- GIVEN `mockOperationalDashboardMetrics` is supplied to `OperationalDashboardPage`
- WHEN the default Storybook story renders
- THEN all default-layout widgets SHALL mount without calling the HyperShell API

---

### Legacy compatibility: Platform Inventory Summary (OP-DASH-22)

Historical layouts may contain an `inventory-summary` widget that renders platform inventory totals and top dimensions from the `managed-clusters` metric defined in `platform/platform-inventory.spec.md` (PI-05, PI-06). It is not part of the current overview default layout or add-widgets catalog.

The default layout template SHALL add a **Platform inventory** section below the hub cluster section:

| Widget type                 | Layout item `i`               | Default position (4-column)                                |
| --------------------------- | ----------------------------- | ---------------------------------------------------------- |
| `inventory-summary`         | `inventory-summary#1`         | Column 0, platform inventory section                       |
| `managed-cluster-providers` | `managed-cluster-providers#1` | Column 1, platform inventory section                       |
| `managed-cluster-regions`   | `managed-cluster-regions#1`   | Columns 2-3 (two columns wide), platform inventory section |

`localizeDashboardLayoutTemplate` SHALL resolve `section-title#platform-inventory` through `SECTION_TITLE_MESSAGE_BY_ID` with message ID `app.dashboard.sectionTitle.platformInventory`.

The inventory summary widget SHALL use the same `DescriptionList` summary presentation stack as `usage-summary` and `system-summary` (OP-DASH-13). It SHALL NOT render trend sparklines.

The `managed-cluster-providers` and `managed-cluster-regions` widgets SHALL use the shared `StatusDonutChart` stack (OP-DASH-16) with labels from `inventoryProviders` and `inventoryRegions` keys respectively.

The default layout changes in this requirement are covered by the layout persistence key bumps documented in OP-DASH-11 (`v30` through `v42`).

#### Scenario: Default layout includes inventory summary

- GIVEN the default layout template is active
- WHEN the dashboard grid renders on a four-column breakpoint
- THEN a localized **Platform inventory** section title SHALL appear below the hub cluster widgets
- AND the inventory summary card SHALL list the managed cluster total and recent count

---

### Legacy compatibility: Optional Platform Inventory Widgets (OP-DASH-21)

The implementation retains optional inventory detail renderers defined in `platform/platform-inventory.spec.md` (PI-07) only for compatibility with historical saved layouts:

- `managed-cluster-providers` - provider donut driven by `managed-clusters.inventoryProviders` (on default layout; OP-DASH-22)
- `managed-cluster-regions` - placement donut driven by `managed-clusters.inventoryRegions` (`{region} ({provider})` keys; on default layout; OP-DASH-22)

The widget catalog SHALL NOT register standalone `managed-clusters`, `managed-cluster-status`, `managed-databases`, or `managed-database-status` widget types: the ManagedDatabase resource no longer exists (see `platform/openshell-gateway-database.spec.md`), and a standalone cluster count or cluster status donut duplicates the totals and breakdowns already covered below. Cluster totals SHALL be presented through `inventory-summary` (OP-DASH-22); dimension breakdowns SHALL use the default-layout provider and region donut widgets above. No inventory status donut widget SHALL exist.

These widget types SHALL not be offered in the current overview add-widgets drawer or default layout. Existing saved entries may render until removed by the normal retired-widget sanitization path.

A dedicated `/dashboard/inventory` route SHALL be available to `platform:admin` callers and SHALL redirect authenticated non-admin callers to `/dashboard`, matching the provisioning dashboard rule. Its data requirements are defined in `platform/platform-inventory.spec.md`.

---

### Requirement: OP-DASH-25 -- Gateway Releases Widget

The operational dashboard SHALL include a `gateway-releases` widget that presents authorized-scope gateway counts grouped by each gateway's observed `gateway_version`. Aggregation rules, the `gateway-releases` metric contract, and dashboard authorization are defined in `platform/gateway-release-distribution.spec.md`.

The widget SHALL load from the independent `gateway-release-distribution` metric source (OP-DASH-19). A failure in gateway list aggregation SHALL omit only `gateway-releases`; Prometheus gateway phase counts and sandbox totals SHALL NOT block release distribution when REST aggregation succeeds.

**Presentation:**

- Widget title SHALL be localized **Gateway releases** (`app.dashboard.widget.gatewayReleases`)
- The add-widgets drawer SHALL register `gateway-releases` with the same localized title and a `CodeBranchIcon` catalog icon (matching other metric widgets that expose title only)
- `GatewayReleasesChart` SHALL render a vertical `DashboardStatPanel` list (same presentation stack as the provision-time widget), not a donut chart
- `GatewayReleasesCard` SHALL reuse the provision-time card body class for compact stat-panel spacing
- Each row SHALL show a version label and gateway count
- Rows SHALL include every version in `releaseDistribution` with count greater than zero, sorted by descending count via `sortInventoryDimensionEntries` (no top-N cap)
- Version labels SHALL be localized through `inventoryDimensionLabel` (including the `unknown` bucket for gateways without `gateway_version`)
- The stat panel SHALL expose a localized accessible name via `app.dashboard.gatewayReleases.summaryAriaLabel` (**Gateways by release**)
- The widget SHALL NOT render a center fleet total or headline number (avoids confusion with gateway status and usage-summary gateway totals)
- The widget SHALL NOT render a trend sparkline
- When `releaseDistribution` is present but every bucket count is zero, the chart area SHALL render nothing while the widget shell remains

**Default layout:**

- The default layout template SHALL place `gateway-releases` in column 3 of the platform adoption section, directly below `gateway-status`
- Widget height SHALL match `GATEWAY_STATUS_WIDGET_HEIGHT` via `GATEWAY_RELEASES_WIDGET_HEIGHT` (compact stat panel)

**Retired sandboxes widget:**

- The standalone `provisioned-sandboxes` grid widget SHALL be removed from `defaultDashboardLayoutTemplate` and from the add-widgets catalog
- `provisioned-sandboxes` SHALL remain a connected metric for the `usage-summary` sandboxes row (OP-DASH-06, OP-DASH-13), including optional hourly and daily trend sparklines (GSAT-07)
- `stripRemovedWidgetTypes` SHALL remove saved `provisioned-sandboxes` and `managed-database-status` tiles on load (OP-DASH-11)

`localizeDashboardLayoutTemplate` SHALL resolve `gateway-releases#1` through `WIDGET_TITLE_MESSAGE_BY_TYPE`. Partial-failure warnings SHALL label the source through `app.dashboard.metricSource.gatewayReleaseDistribution` (**Gateway releases**).

#### Scenario: Release list shows all versions by count

- GIVEN `gateway-releases` has `releaseDistribution: { "v1.0.0": 4, "v1.1.0": 2, "unknown": 1 }`
- WHEN the Gateway releases widget renders
- THEN three stat-panel rows SHALL appear ordered `v1.0.0` (4), `v1.1.0` (2), then the localized unknown label (1)
- AND the widget SHALL NOT render a donut or center total

#### Scenario: Missing release distribution renders empty chart area

- GIVEN the `gateway-releases` metric is absent from the adapter response
- WHEN the Gateway releases widget renders
- THEN the widget body SHALL show the localized metric-unavailable empty state (OP-DASH-08)

#### Scenario: Default layout stacks gateway status above gateway releases

- GIVEN the default layout template is active
- WHEN the dashboard grid renders on a four-column breakpoint
- THEN `gateway-status#1` SHALL occupy column 3 on the platform adoption top row
- AND `gateway-releases#1` SHALL occupy column 3 on the row below gateway status
- AND no `provisioned-sandboxes` widget SHALL appear on the grid
