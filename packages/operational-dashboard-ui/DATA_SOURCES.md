# Operational dashboard data sources

This document tracks which dashboard widgets in the operational-dashboard
package are backed by live data. Widgets without a connected source still
appear on the dashboard; they render the localized "Metric unavailable" empty
state.

Reliability and usage dashboard sources are documented in
[API reliability](#api-reliability) and [Usage dashboard](#usage-dashboard)
below. They load through separate control-plane methods and must not be added
to the operational `metricSources` list.

## Data flow

```
useGetMetricsData
  → dashboard.getOperationalMetrics(signal, timeRange, { isAdmin })
  → components/web-console/app/adapters/api/dashboard-control-plane.ts
```

The adapter loads independent **metric sources** (see below). Each source may
emit one or more `OperationalMetric` values. A source failure records the
source id in `failedSources` (WEB-OPS-19). On refresh, `mergeOperationalDashboardMetrics`
keeps the last successful value for metrics from failed sources so the dashboard
does not flash empty while a dependency is down.

Operational pages expose a 7- or 30-day time range. Range-aware sources send
that selection to the BFF; sources with fixed windows are described below.

## Refresh

All connected metrics share `operationalDashboardRefreshMilliseconds` (15
minutes) via `useGetMetricsData`. Manual refresh re-fetches every active source
immediately (WEB-OPS-09).

When OIDC is configured, all metric BFF routes require an authenticated
session. Admin-only metric routes also require `platform:admin` (WEB-OPS-04).
The gateway and gateway-sandbox routes are not admin-only: administrators get
Prometheus fleet metrics, while non-administrators get an aggregate derived from
the authorized gateway list API. For non-administrators, the adapter requests
only `gateway-metrics` and `gateway-release-distribution`; the gateway source
emits only gateway status and Active sandbox totals. No-auth local development
leaves these routes open.

## Metric sources

| Source ID                      | BFF / API routes                                                                                                                                                | Metrics emitted                                                                                                                  |
| ------------------------------ | --------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------- |
| `gateway-metrics`              | `GET /api/metrics/gateways`, `GET /api/metrics/gateway-sandboxes`, `GET /api/metrics/gateway-provision-duration`, `GET /api/metrics/gateway-provision-outcomes` | `provisioned-gateways`, `provisioned-sandboxes`, `provision-time`, `provision-reliability`                                       |
| `registered-users`             | `GET /api/metrics/registered-users`                                                                                                                             | `registered-users`                                                                                                               |
| `gateway-release-distribution` | SDK `GET /api/hypershell/v1/gateways` (groups each gateway's observed `gateway_version`)                                                                        | `gateway-releases`                                                                                                               |
| `platform-inventory`           | `GET /api/metrics/platform-inventory`                                                                                                                           | `managed-clusters`                                                                                                               |
| `cluster-memory`               | `GET /api/metrics/cluster-memory`                                                                                                                               | `memory` (optional selected-range `daily_used` → `trend`; see [Hub cluster utilization trends](#hub-cluster-utilization-trends)) |
| `cluster-cpu`                  | `GET /api/metrics/cluster-cpu`                                                                                                                                  | `cpu` (optional selected-range `daily_used` → `trend`; see [Hub cluster utilization trends](#hub-cluster-utilization-trends))    |
| `cluster-pods`                 | `GET /api/metrics/cluster-pods`                                                                                                                                 | `pods` (optional fixed 7-day `daily_used` → `trend`; see [Hub cluster utilization trends](#hub-cluster-utilization-trends))      |
| `cluster-nodes`                | `GET /api/metrics/cluster-nodes`                                                                                                                                | `nodes`                                                                                                                          |

If `GET /api/metrics/gateways` or `GET /api/metrics/gateway-sandboxes` fails,
the entire `gateway-metrics` source fails and all four gateway metrics above are
omitted, even when the provision-duration or provision-outcomes routes would
have succeeded (WEB-OPS-23).

For administrators, provision-duration or provision-outcome failures omit only
that metric; they do not mark the whole `gateway-metrics` source as failed.
For non-administrators, those two routes are not requested.

## Connected widgets and metrics

Widget types in the dashboard pages and shared widget components map to
`OperationalMetric` ids from the adapter. Summary cards read multiple metrics
from the same payload.

| Widget type                 | Metric ID               | Notes                                                                                                                           |
| --------------------------- | ----------------------- | ------------------------------------------------------------------------------------------------------------------------------- |
| `gateway-status`            | `provisioned-gateways`  | Fleet-wide phase counts from `hypershell_gateways_total`. Display status via `gatewayPhaseCountsToDisplayStatusCounts`.         |
| `sandbox-status`            | `provisioned-sandboxes` | Active total plus optional lifecycle attention breakdown. Spec: `platform/gateway-sandbox-attention-status.md`.                 |
| `active-sandboxes`          | `provisioned-sandboxes` | Daily Active sandbox sparkline for the selected range.                                                                          |
| `gateway-releases`          | `gateway-releases`      | Per-version counts from the paginated gateway list API; missing or blank versions bucket as `unknown`.                          |
| `registered-users`          | `registered-users`      | See [Registered users](#registered-users) below.                                                                                |
| `provision-time`            | `provision-time`        | See [Provision time](#provision-time) below.                                                                                    |
| `provision-reliability`     | `provision-reliability` | See [Provision reliability](#provision-reliability) below.                                                                      |
| `memory`                    | `memory`                | Node-exporter `node_memory_*` bytes, mapped to GiB.                                                                             |
| `cpu`                       | `cpu`                   | Node-exporter `node_cpu_seconds_total` capacity and 5m non-idle rate, mapped to whole cores.                                    |
| `pods`                      | `pods`                  | kube-state-metrics allocatable pod capacity, live pod count, and phase breakdown.                                               |
| `nodes`                     | `nodes`                 | kube-state-metrics `kube_node_info` total and Ready condition.                                                                  |
| `inventory-summary`         | `managed-clusters`      | Totals and status icons; 30-day creations on the operational page, selected-range creations on the inventory page (WEB-OPS-22). |
| `managed-cluster-providers` | `managed-clusters`      | Provider donut from `inventoryProviders`.                                                                                       |
| `managed-cluster-regions`   | `managed-clusters`      | Region donut from `inventoryRegions` (`{region} ({provider})` keys).                                                            |
| `resource-summary`          | several                 | Gateways, sandboxes, and derived healthy/attention counts.                                                                      |
| `system-summary`            | several                 | Managed clusters, nodes, pods, gateways, memory, and CPU.                                                                       |
| `provisioning-snapshot`     | several                 | Gateway provisioning count, provision time, and provision reliability.                                                          |

### Sandbox status / Active sandboxes

Source: BFF `GET /api/metrics/gateway-sandboxes`. Specs:
`platform/openshell-gateway-sandbox-count.spec.md`,
`platform/gateway-sandbox-active-trends.spec.md`,
`platform/gateway-sandbox-attention-status.spec.md`.

| Field / metric id field | Prometheus series / BFF field                                                           | Meaning                                                               |
| ----------------------- | --------------------------------------------------------------------------------------- | --------------------------------------------------------------------- |
| `value`                 | `hypershell_gateways_active_sandboxes_total` / `active_sandboxes`                       | Fleet Active total (sum of live Gateway `active_sandbox_count`)       |
| `orphanedSandboxes`     | `sum(max by (hypershell_cluster_id) (gateway_sandbox_orphaned))` / `orphaned_sandboxes` | Active sandboxes in namespaces with no live Gateway (optional)        |
| `expiringSandboxes`     | `sum(max by (hypershell_cluster_id) (gateway_sandbox_expiring))` / `expiring_sandboxes` | Active sandboxes with `shutdownTime` overdue or within 24h (optional) |
| `idleSandboxes`         | `sum(max by (hypershell_cluster_id) (gateway_sandbox_idle))` / `idle_sandboxes`         | Idle active sandboxes with a live Gateway (optional)                  |
| `hourlyTrend`           | `hourly_active_sandboxes`                                                               | Last 24 hours Active series (optional)                                |
| `trend`                 | `daily_active_sandboxes`                                                                | Selected 7- or 30-day Active series (optional)                        |

Attention fields are optional within the `gateway-metrics` source (OBS-SBXA-07 /
OBS-SBXA-10). When Active succeeds but an attention gauge fails, the BFF omits only
that field; the adapter does not invent zeros; the Sandbox status widget shows
Attention required as unavailable when all three are absent.

Attention gauges are control-plane OTLP metrics
(`gateway.sandbox.orphaned|expiring|idle` → Prometheus
`gateway_sandbox_*` with `hypershell_cluster_id`). The BFF fleet total is
`sum(max by (hypershell_cluster_id) (...))` so control-plane replicas for the
same managed cluster do not double-count; when `PROMETHEUS_NAMESPACE` is set the
selector uses `k8s_namespace_name` (not scrape `namespace`). Prometheus
staleness drops silent instances so they do not inflate the fleet sum. Hard
derive failures export zeros so sticky last-success values cannot linger while
the process stays up. Active remains an API-server scrape gauge.

For non-administrators, the BFF returns only the Active total from the
authorized gateway list; attention fields and trends are omitted.
`provisioned-sandboxes` also feeds the resource summary Sandboxes row.

### Registered users

Source: BFF `GET /api/metrics/registered-users` (Prometheus `hypershell_users_*`
gauges). Spec: `platform/registered-users.spec.md`.

| Field                    | Prometheus series                                   | Meaning                                                                                                      |
| ------------------------ | --------------------------------------------------- | ------------------------------------------------------------------------------------------------------------ |
| `value`                  | `hypershell_users_registered_total`                 | Total registered users                                                                                       |
| `createdLast7Days`       | `hypershell_users_created_last_7_days_total`        | Users created in the last 7 x 24 hours UTC                                                                   |
| `createdLast30Days`      | `hypershell_users_created_last_30_days_total`       | Users created in the last 30 x 24 hours UTC                                                                  |
| `uniqueLoginsLast7Days`  | `hypershell_users_unique_logins_last_7_days_total`  | Distinct registered users with authorized API activity over the last 7 UTC calendar days inclusive of today  |
| `uniqueLoginsLast30Days` | `hypershell_users_unique_logins_last_30_days_total` | Distinct registered users with authorized API activity over the last 30 UTC calendar days inclusive of today |
| `trend.points`           | `hypershell_users_unique_logins_daily_total`        | Daily unique-login count per UTC calendar day for the last 30 days                                           |

The registered-users widget trend arrow compares the first and last sparkline
points (PLAT-USR-13). Optional fields omitted by the BFF degrade individual stat
cells without synthesizing zero (PLAT-USR-14).

### Provision time

Source: BFF `GET /api/metrics/gateway-provision-duration`. Spec:
`platform/gateway-provision-time.spec.md`.

Prometheus histogram `gateway_provision_duration_seconds` (control-plane OTLP
per OBS-CPL-07). The BFF computes mean, P50, and P95 from increases over the
selected 7- or 30-day window. Values are presented in **seconds**
(`unit: "sec"`).

Requires control-plane metrics export through the in-cluster OpenTelemetry
Collector (`deploy/base/prometheus/otel-collector.yaml`).

### Provision reliability

Source: BFF `GET /api/metrics/gateway-provision-outcomes`. Spec:
`platform/gateway-provision-outcomes.spec.md`.

Success and failure counts from `gateway_provision_outcomes_total`
(`outcome="success|failure"`, OBS-CPL-07) over the selected 7- or 30-day window.
The BFF response field names retain the historical `_24h` suffix. When outcome
counters are empty, the BFF may fall back to
`gateway_provision_duration_seconds_count` for success counts only (OBS-GPO-04); it
does not synthesize failure counts.

The widget includes a daily success-rate sparkline (`successRateTrend`) for the
selected range when at least two daily buckets are present.

### Hub cluster utilization trends

Spec: `platform/hub-cluster-utilization-trends.spec.md` (HYPERSHELL-281 hub scope).

The memory and CPU BFF routes extend their instant JSON with optional
`daily_used`: an array of `{ date, value }` entries for the selected **7 or 30
UTC calendar days** inclusive of today. The pods route always returns seven
days. Values are daily **used** amounts in display units (GiB, cores, pod
count). The adapter maps `daily_used` into `OperationalMetric.trend.points`
(`label` ← `date`, `value` ← `value`).

Range queries use the same PromQL as the instant **used** measurement and
Prometheus `query_range` with **86400s** step. Memory and CPU use
`alignDailySeriesPreservingGaps`, so missing calendar days remain `null`; pods
use `alignDailyIntegerSeries` zero-fill.

| BFF route                         | Range                 | PromQL (daily used)                                                     | Trend unit  |
| --------------------------------- | --------------------- | ----------------------------------------------------------------------- | ----------- |
| `GET /api/metrics/cluster-memory` | Selected 7 or 30 days | `sum(node_memory_MemTotal_bytes) - sum(node_memory_MemAvailable_bytes)` | whole GiB   |
| `GET /api/metrics/cluster-cpu`    | Selected 7 or 30 days | `sum(rate(node_cpu_seconds_total{mode!="idle"}[5m]))`                   | whole cores |
| `GET /api/metrics/cluster-pods`   | Fixed 7 days          | `count(kube_pod_info)`                                                  | whole pods  |

When the range query fails but instant queries succeed, the BFF returns HTTP `200` with instant fields and omits `daily_used`. Widgets omit sparklines when `trend` is absent or has fewer than two points (OBS-HCUT-08).

## Sparklines and trends

| Location                       | Series                             | When shown                                                                    |
| ------------------------------ | ---------------------------------- | ----------------------------------------------------------------------------- |
| `gateway-status` widget        | Selected-range daily fleet totals  | When `provisioned-gateways.trend.points` has at least two points (OBS-GFT-06) |
| `active-sandboxes` widget      | Selected-range daily Active totals | When `provisioned-sandboxes.trend.points` has at least two points             |
| `memory` widget                | Selected-range daily used memory   | When `memory.trend.points` has at least two points (OBS-HCUT-08)              |
| `cpu` widget                   | Selected-range daily used CPU      | When `cpu.trend.points` has at least two points (OBS-HCUT-08)                 |
| `pods` widget                  | Fixed 7-day daily used pods        | When `pods.trend.points` has at least two points (OBS-HCUT-08)                |
| `registered-users` widget      | 30-day daily unique logins         | When `trend.points` has at least two points                                   |
| `provision-reliability` widget | Selected-range daily success rate  | When BFF returns at least two `daily_success_rate` buckets                    |
| Resource summary Gateways row  | Trend arrow on total               | When `provisioned-gateways.trend` is present (OBS-GFT-07)                     |
| Resource summary Sandboxes row | Trend arrows                       | When `provisioned-sandboxes.hourlyTrend` or `trend` is present                |
| System summary Memory row      | Trend arrow on used GiB            | When `memory.trend` change is at least 5% (OBS-HCUT-08)                       |
| System summary CPU row         | Trend arrow on used cores          | When `cpu.trend` change is at least 5% (OBS-HCUT-08)                          |

Other metric cards do not load historical series unless listed above.

## API reliability

Reliability dashboard (`/dashboard/reliability`) loads through
`useGetReliabilityMetricsData` → `dashboard.getReliabilityMetrics` →
`createDashboardControlPlaneAdapter.getReliabilityMetrics`. This source is
**not** part of the operational `metricSources` list (OBS-REL-05).

| Source ID                      | BFF route                                       | Metrics emitted                                                                                                                      |
| ------------------------------ | ----------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------ |
| `api-reliability`              | `GET /api/metrics/api-reliability`              | `api-request-rate`, `api-error-rate`, `api-latency`                                                                                  |
| `control-plane-reconciliation` | `GET /api/metrics/control-plane-reconciliation` | `reconciliation-failures`, `reconciliation-retries`, `reconciliation-successes`, `reconciliation-lag`, `stale-resource-status-count` |

The reliability dashboard SHALL read metric data through the BFF routes above.
The UI SHALL NOT read or write metric data directly to a database. Historical
reconciliation data uses a rolling 24-hour Prometheus range query with
hourly samples.

The Reliability summary lists all three API metrics and all five
reconciliation metrics. On wide layouts, API metrics occupy the first column
and reconciliation metrics occupy the second; the columns stack on narrow
layouts. Dedicated reliability widgets expose the API metrics and
reconciliation failures, retries, lag, and stale-resource status;
reconciliation successes appear in the summary. Current API metrics use a
5-minute lookback; reconciliation failure, retry, and success counts use a
rolling 24-hour lookback, with one-hour count samples for their 24-hour trends.
Reconciliation lag is a 5-minute P50 when a usable histogram observation is
available; otherwise the lag metric is omitted while the other metrics remain
available.

| Metric ID                     | Prometheus type | Current value                  | Unit    |
| ----------------------------- | --------------- | ------------------------------ | ------- |
| `reconciliation-failures`     | Counter         | Previous 24-hour failure count | `count` |
| `reconciliation-retries`      | Counter         | Previous 24-hour retry count   | `count` |
| `reconciliation-successes`    | Counter         | Previous 24-hour success count | `count` |
| `reconciliation-lag`          | Histogram       | P50                            | `sec`   |
| `stale-resource-status-count` | Gauge           | Current count                  | `count` |

Reconciliation count values are displayed as whole numbers; reconciliation lag
is displayed to three fractional digits.

A failed source is recorded in `failedSources` while metrics from the other
source are returned. If both sources fail, the adapter returns both source ids
with an empty metrics list (abort still throws). An empty first load is treated
as total failure; after a successful load, `mergeReliabilityDashboardMetrics`
preserves stale widgets on refresh.

Instant fields come from the API server framework Prometheus histogram
`api_inbound_request_duration_*` scraped via the `hypershell-api-server`
ServiceMonitor (`job="hypershell-api-server"`). The HTTP status label is
`code`. (OTel `http.server.request.duration` /
`http_server_request_duration_seconds` is not required for this dashboard;
Kind does not currently set API-server OTLP export.)

| Instant field (BFF)   | Metric ID          | Unit           | Display rounding | Trend field (BFF)            |
| --------------------- | ------------------ | -------------- | ---------------- | ---------------------------- |
| `request_rate`        | `api-request-rate` | `requests/sec` | 3 fractional     | `hourly_request_rate`        |
| `error_rate_percent`  | `api-error-rate`   | `%`            | 3 fractional     | `hourly_error_rate_percent`  |
| `latency_p50_seconds` | `api-latency`      | `sec`          | 3 fractional     | `hourly_latency_p50_seconds` |

PromQL shapes (5-minute rate window; see
`components/web-console/bff/src/metrics-api-reliability.ts`):

- Request rate: `sum(rate(api_inbound_request_duration_count{job="hypershell-api-server"}[5m]))`
- Error rate (5xx only): `100 * (sum(rate(...{code=~"5.."}[5m])) or vector(0)) / clamp_min(sum(rate(...[5m])), 1e-12)` (idle fleets with zero request rate evaluate to `0%`)
- Latency P50: `histogram_quantile(0.50, sum(rate(..._bucket[5m])) by (le))`, with cumulative histogram fallback when the rate-based quantile is non-finite (instant and hourly)

Hourly trends use Prometheus `query_range` over the last 24 hours with step
`3600s`. Missing historical arrays omit `hourlyTrend` for that metric only;
instant-query failure is a total source failure (HTTP `502`). Specs:
`platform/api-reliability-metrics.spec.md`,
`web-console/reliability-dashboard.spec.md`.

Refresh uses `reliabilityDashboardRefreshMilliseconds` (same 15-minute
interval as the operational dashboard).

## Usage dashboard

The usage dashboard (`/dashboard/usage`) loads through
`useGetUsageMetricsData` → `dashboard.getUsageMetrics` →
`createDashboardControlPlaneAdapter.getUsageMetrics`. It is available to every
authenticated user. Users with `platform:admin` receive fleet aggregates and
the UI adds the localized `System` label to every usage widget. Other users
receive only aggregates calculated from their authorized Gateway inventory.

| Source ID                | BFF route                                             | Metrics emitted                                                                      |
| ------------------------ | ----------------------------------------------------- | ------------------------------------------------------------------------------------ |
| `usage-gateways`         | `GET /api/metrics/usage/gateways?range=7\|30`         | `gateway-request-volume`, `gateway-error-rate`, `active-gateways`, `unused-gateways` |
| `usage-sandboxes`        | `GET /api/metrics/usage/sandboxes?range=7\|30`        | `sandbox-utilization`                                                                |
| `usage-sandbox-lifetime` | `GET /api/metrics/usage/sandbox-lifetime?range=7\|30` | `sandbox-lifetime`                                                                   |

### Authorization scope

When OIDC is enabled, every usage route requires an authenticated session.
Both administrators and non-administrators resolve the current Gateway inventory
from the authorized Gateway list. Before resolving that inventory, each route
refreshes the caller's access token when needed; if refresh fails, the route
returns the standard reauthentication response. Administrators receive the fleet
inventory and omit the usage namespace selector. Non-administrator requests
build the set of authorized `openshell-<id>` namespaces and add that selector to
every usage PromQL query before aggregation. The active sandbox instant value is
also taken from the authorized Gateway list, so the browser never receives a
fleet aggregate for local filtering. Intentional no-auth development, where OIDC
is unconfigured, treats the caller as an administrator; partial OIDC
configuration fails validation rather than falling back to no-auth.

### Gateway usage metrics

Gateway request metrics reuse the gateway-native Prometheus counters
`openshell_server_grpc_requests_total{method,code}` and
`openshell_server_http_requests_total{path,status}`. No additional
`ServiceMonitor` or `PodMonitor` is introduced by this dashboard.
`PROMETHEUS_URL` MUST point to an application metrics endpoint that already
exposes these series for the managed clusters in the fleet.

| Metric ID                | Prometheus type  | Calculation                                                                                                                                                               |
| ------------------------ | ---------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `gateway-request-volume` | Counters         | Sum of 7- or 30-day `increase()` for gRPC and HTTP requests; gRPC excludes internal `Peer*` methods                                                                       |
| `gateway-error-rate`     | Counters         | 100 * (gRPC `code!="OK"` + HTTP `status=~"5.."`) / total qualifying requests; zero when traffic exists and error series are absent, omitted when total is missing or zero |
| `active-gateways`        | Gauge + counters | Count of authorized current Gateway namespaces with positive qualifying request volume in the selected range                                                              |
| `unused-gateways`        | Gauge + counters | Authorized current Gateway count minus active Gateway count                                                                                                               |

Current Gateway namespaces come from the authorized Gateway list for both
administrators and non-administrators. Historical traffic from a deleted
namespace is never counted because active namespaces are intersected with the
current inventory before the totals are calculated.

Daily volume and error-rate series use the same PromQL with a one-day
`increase()` window, UTC calendar-day boundaries, and one-day steps. Missing
days remain `null`; they are never backfilled with zero. If an instant value is
available but a daily range query fails, the route returns the instant value and
omits the trend. A successful absent protocol series contributes zero. If
either protocol query fails, request volume, daily request volume, active
Gateways, and unused Gateways are omitted.

### Sandbox usage metrics

| Metric ID             | Prometheus type | Calculation                                                                                                                                                              |
| --------------------- | --------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `sandbox-utilization` | Gauges          | 100 * active sandboxes / provisioned sandboxes                                                                                                                           |
| `sandbox-lifetime`    | Histogram       | Mean from `gateway_sandbox_lifetime_seconds_sum / _count`; P50 and P95 from `histogram_quantile` over `gateway_sandbox_lifetime_seconds_bucket` selected-range increases |

Active sandboxes use
`hypershell_gateways_active_sandboxes_total`, deduplicated by
`managed_cluster` and `gateway_namespace`. Provisioned sandboxes use the control-plane
gauge `gateway_sandbox_total`, deduplicated by `hypershell_cluster_id` and
`namespace`. The control-plane OTel instrument `gateway.sandbox.total` exports
that gauge. The control-plane OTel instrument `gateway.sandbox.lifetime` exports
the `gateway_sandbox_lifetime_seconds` histogram because the exporter appends
the instrument unit suffix. OTel scope attributes are exported as PromQL labels
`namespace` and `hypershell_cluster_id`, and the OTel collector scrape honors
exported labels. The lifetime histogram records an
observation when a sandbox leaves the active set, using the Sandbox CR creation
timestamp with pod creation as the fallback. It never uses the optional
`spec.lifecycle.shutdownTime` as an observed end time.

Daily sandbox utilization and daily mean lifetime use one-day windows with UTC
boundaries. Lifetime daily mean applies the one-day window to both the histogram
sum and count. Missing days remain `null`. A zero denominator produces an explicit
`0%` utilization; a missing active or total series omits the metric so the
widget shows the localized unavailable state. The lifetime source emits mean,
P50, and P95 independently, so the UI shows whichever values remain.

### Refresh and failure semantics

All usage metrics share `usageDashboardRefreshMilliseconds` (15 minutes).
Manual refresh immediately re-fetches all three sources. Each source fails
independently: the adapter records its source ID in `failedSources`, emits
metrics from successful sources, and shows the shared partial-failure warning.
After a successful load, a later failed refresh preserves the stale metrics for
that source. If all sources fail on the first load, the query fails and the
page shows the total load-failure state.

## Adoption dashboard

The Adoption dashboard uses a separate application workflow:

```
useGetAdoptionMetricsData
  → dashboard.getAdoptionMetrics(signal, timeRange, { isAdmin })
  → GET /api/metrics/adoption?range=7|30
```

The route is admin-only when OIDC is enabled. It returns aggregate values
only; it never returns user identifiers or user-level rows. `registered_users`
is required. Every other field is optional, and omission means unavailable.
The BFF preserves successfully computed fields when an optional Prometheus
subquery fails.

All activity windows use UTC calendar days, and today may be partial.
Qualifying activity is one or more authenticated HyperShell API requests by a
non-deleted registered user on a UTC calendar day, as recorded in
`user_daily_activities`.

| UI field                 | Response field                                  | Source                                                        |
| ------------------------ | ----------------------------------------------- | ------------------------------------------------------------- |
| Registered users         | `registered_users`                              | `hypershell_users_registered_total`                           |
| New registrations        | `created_last_7_days` or `created_last_30_days` | `hypershell_users_created_last_*_days_total`                  |
| Daily active users       | last `daily_unique_logins` point                | `hypershell_users_unique_logins_daily_total`                  |
| Weekly active users      | `unique_logins_last_7_days`                     | `hypershell_users_unique_logins_last_7_days_total`            |
| Monthly active users     | `unique_logins_last_30_days`                    | `hypershell_users_unique_logins_last_30_days_total`           |
| Inactive users           | `inactive_users_last_30_days`                   | `hypershell_users_inactive_last_30_days_total`                |
| First-gateway users      | `users_with_gateway`                            | `hypershell_role_bindings_gateway_owner_users_total`          |
| First-gateway conversion | `gateway_conversion_percent`                    | BFF calculation from gateway-owner users and registered users |
| Conversion donut         | `users_with_gateway` and `registered_users`     | First-gateway users versus registered users without a gateway |
| Registrations trend      | `daily_created_users`                           | `hypershell_users_created_daily_total`                        |
| Active-users trend       | `daily_unique_logins`                           | `hypershell_users_unique_logins_daily_total`                  |
| 7-day retention          | `retention_7_days_percent`                      | `hypershell_users_retention_7_days_percent`                   |

The selected 7- or 30-day range changes the New registrations summary value and
the number of trend points displayed by the Adoption trends widget. Both trend
arrays always contain one point per UTC calendar day for the trailing 30 days;
the widget displays the trailing 7 points when 7 days is selected.

Inactive users are non-deleted users created more than 30 UTC days ago with no
`user_daily_activities` row in the trailing 30 UTC calendar days inclusive of
today. First-gateway conversion counts each distinct non-deleted user once if
that user has ever held a `gateway-owner` role binding, including deleted
bindings. The conversion donut uses that first-gateway-user count as its
converted segment and derives the not-converted segment as `registered_users -
users_with_gateway`; it does not claim to represent first sandbox use or a
combined gateway-or-sandbox first-resource event. Seven-day retention compares
the current and immediately preceding 7 UTC calendar-day active sets.

Unavailable by design:

- First-sandbox conversion: no durable, user-attributed first-sandbox event.
- Median time to first resource: no combined gateway-or-sandbox first-resource event.
- 30-day retention: the activity store retains only 30 days, but a true
  comparison needs 60 days.

These fields render a dash in their summary rows. They are not replaced with
zero or hidden; whole-widget and chart unavailable states continue to render the
localized metric-unavailable treatment.

## Adding a new source

1. Expose a same-origin BFF route (or reuse an existing API route) for browser access.
2. Add an independent source in `createDashboardControlPlaneAdapter` and map the response into `OperationalMetric`.
3. Update this file and the dashboard page description in `messages.ts`.
4. Register the source-to-metric mapping in `dashboard-metric-sources.ts` so partial refreshes preserve stale values.
