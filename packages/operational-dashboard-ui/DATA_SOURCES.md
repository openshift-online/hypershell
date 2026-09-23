# Operational dashboard data sources

This document tracks which operational dashboard widgets are backed by live
data. Widgets without a connected source still appear on the dashboard; they
render the localized "Metric unavailable" empty state from
`operational-dashboard-page.tsx`.

Reliability dashboard sources are documented in
[API reliability](#api-reliability) below. They load through a separate
control-plane method and must not be added to the operational `metricSources`
list.

## Data flow

```
useGetMetricsData
  → dashboard.getOperationalMetrics
  → components/web-console/app/adapters/api/dashboard-control-plane.ts
```

The adapter loads independent **metric sources** (see below). Each source may
emit one or more `OperationalMetric` values. A source failure records the
source id in `failedSources` (OP-DASH-19). On refresh, `mergeOperationalDashboardMetrics`
keeps the last successful value for metrics from failed sources so the dashboard
does not flash empty while a dependency is down.

## Refresh

All connected metrics share `operationalDashboardRefreshMilliseconds` (15
minutes) via `useGetMetricsData`. Manual refresh re-fetches every source
immediately (OP-DASH-09).

Dashboard-operator BFF routes require `platform:admin` (OP-DASH-04).

## Metric sources

| Source ID                      | BFF / API routes                                                                                                                                                | Metrics emitted                                                                                                         |
| ------------------------------ | --------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------- |
| `gateway-metrics`              | `GET /api/metrics/gateways`, `GET /api/metrics/gateway-sandboxes`, `GET /api/metrics/gateway-provision-duration`, `GET /api/metrics/gateway-provision-outcomes` | `provisioned-gateways`, `provisioned-sandboxes`, `provision-time`, `provision-reliability`                              |
| `registered-users`             | `GET /api/metrics/registered-users`                                                                                                                             | `registered-users`                                                                                                      |
| `gateway-release-distribution` | SDK `GET /api/hypershell/v1/gateways`, `GET /api/hypershell/v1/gateway_releases`                                                                                | `gateway-releases`                                                                                                      |
| `platform-inventory`           | `GET /api/metrics/platform-inventory`                                                                                                                           | `managed-clusters`                                                                                                      |
| `cluster-memory`               | `GET /api/metrics/cluster-memory`                                                                                                                               | `memory` (optional 7-day `daily_used` → `trend`; see [Hub cluster utilization trends](#hub-cluster-utilization-trends)) |
| `cluster-cpu`                  | `GET /api/metrics/cluster-cpu`                                                                                                                                  | `cpu` (optional 7-day `daily_used` → `trend`; see [Hub cluster utilization trends](#hub-cluster-utilization-trends))    |
| `cluster-pods`                 | `GET /api/metrics/cluster-pods`                                                                                                                                 | `pods` (optional 7-day `daily_used` → `trend`; see [Hub cluster utilization trends](#hub-cluster-utilization-trends))   |
| `cluster-nodes`                | `GET /api/metrics/cluster-nodes`                                                                                                                                | `nodes`                                                                                                                 |

If `GET /api/metrics/gateways` or `GET /api/metrics/gateway-sandboxes` fails,
the entire `gateway-metrics` source fails and all four gateway metrics above are
omitted, even when the provision-duration or provision-outcomes routes would
have succeeded (OP-DASH-23).

## Connected widgets and metrics

Widget types in `operational-dashboard-page.tsx` map to `OperationalMetric` ids
from the adapter. Summary cards (`usage-summary`, `system-summary`) read
multiple metrics from the same payload.

| Widget type                 | Metric ID               | Notes                                                                                                                           |
| --------------------------- | ----------------------- | ------------------------------------------------------------------------------------------------------------------------------- |
| `gateway-status`            | `provisioned-gateways`  | Fleet-wide phase counts from `hypershell_gateways_total`. Display status via `gatewayPhaseCountsToDisplayStatusCounts`.         |
| `sandbox-status`            | `provisioned-sandboxes` | Active total plus optional attention labels and Active trends. Spec: `platform/gateway-sandbox-attention-status.spec.md`.       |
| `gateway-releases`          | `gateway-releases`      | Per-release counts from paginated gateway and gateway-release list APIs. Spec: `platform/gateway-release-distribution.spec.md`. |
| `registered-users`          | `registered-users`      | See [Registered users](#registered-users) below.                                                                                |
| `provision-time`            | `provision-time`        | See [Provision time](#provision-time) below.                                                                                    |
| `provision-reliability`     | `provision-reliability` | See [Provision reliability](#provision-reliability) below.                                                                      |
| `memory`                    | `memory`                | Node-exporter `node_memory_*` bytes, mapped to GiB.                                                                             |
| `cpu`                       | `cpu`                   | Node-exporter `node_cpu_seconds_total` capacity and 5m non-idle rate, mapped to whole cores.                                    |
| `pods`                      | `pods`                  | kube-state-metrics allocatable pod capacity, live pod count, and phase breakdown.                                               |
| `nodes`                     | `nodes`                 | kube-state-metrics `kube_node_info` total and Ready condition.                                                                  |
| `inventory-summary`         | `managed-clusters`      | Totals, 30-day cluster creations, and status icons (OP-DASH-22).                                                                |
| `managed-cluster-providers` | `managed-clusters`      | Provider donut from `inventoryProviders`.                                                                                       |
| `managed-cluster-regions`   | `managed-clusters`      | Region donut from `inventoryRegions` (`{region} ({provider})` keys).                                                            |
| `usage-summary`             | several                 | Gateways (`provisioned-gateways`), sandboxes (`provisioned-sandboxes`), users (`registered-users`).                             |
| `system-summary`            | several                 | Provision time and reliability metrics from `gateway-metrics` source.                                                           |

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
| `hourlyTrend`           | `hourly_active_sandboxes`                                                               | Last 24 hours Active sparkline (optional)                             |
| `trend`                 | `daily_active_sandboxes`                                                                | Last 7 days Active sparkline (optional)                               |

Attention fields are optional within the `gateway-metrics` source (SSA-07 /
SSA-10). When Active succeeds but an attention gauge fails, the BFF omits only
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

`provisioned-sandboxes` also feeds the usage summary Sandboxes row.

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

The usage summary Users row trend arrow compares the first and last sparkline
points (RU-13). Optional fields omitted by the BFF degrade individual stat
cells without synthesizing zero (RU-14).

### Provision time

Source: BFF `GET /api/metrics/gateway-provision-duration`. Spec:
`platform/gateway-provision-time.spec.md`.

Prometheus histogram `gateway_provision_duration_seconds` (control-plane OTLP
per CP-OBS-07). Values are presented in **seconds** (`unit: "sec"`): mean in
the system summary; mean, P50, and P95 in the `provision-time` widget.

Requires control-plane metrics export through the in-cluster OpenTelemetry
Collector (`deploy/base/prometheus/otel-collector.yaml`).

### Provision reliability

Source: BFF `GET /api/metrics/gateway-provision-outcomes`. Spec:
`platform/gateway-provision-outcomes.spec.md`.

24-hour success and failure counts from `gateway_provision_outcomes_total`
(`outcome="success|failure"`, CP-OBS-07). When outcome counters are empty, the
BFF may fall back to `gateway_provision_duration_seconds_count` for historical
success counts only (GPO-04); it does not synthesize failure counts.

The widget includes an hourly success-rate sparkline (`successRateTrend`) when
at least two hourly buckets are present.

### Hub cluster utilization trends

Spec: `platform/hub-cluster-utilization-trends.spec.md` (HYPERSHELL-281 hub scope).

Each cluster BFF route extends its instant JSON with optional `daily_used`: an array of `{ date, value }` entries for the last **7 UTC calendar days** inclusive of today. Values are daily **used** amounts in display units (GiB, cores, pod count). The adapter maps `daily_used` into `OperationalMetric.trend.points` (`label` ← `date`, `value` ← `value`).

Range queries use the same PromQL as the instant **used** measurement, Prometheus `query_range` with **86400s** step, and `alignDailyIntegerSeries` zero-fill for missing calendar days (same pattern as gateway fleet total trends).

| BFF route                         | Range PromQL (daily used)                                               | Trend unit  |
| --------------------------------- | ----------------------------------------------------------------------- | ----------- |
| `GET /api/metrics/cluster-memory` | `sum(node_memory_MemTotal_bytes) - sum(node_memory_MemAvailable_bytes)` | whole GiB   |
| `GET /api/metrics/cluster-cpu`    | `sum(rate(node_cpu_seconds_total{mode!="idle"}[5m]))`                   | whole cores |
| `GET /api/metrics/cluster-pods`   | `count(kube_pod_info)`                                                  | whole pods  |

When the range query fails but instant queries succeed, the BFF returns HTTP `200` with instant fields and omits `daily_used`. Widgets omit sparklines when `trend` is absent or has fewer than two points (HCUT-08).

## Sparklines and trends

| Location                        | Series                      | When shown                                                                      |
| ------------------------------- | --------------------------- | ------------------------------------------------------------------------------- |
| `gateway-status` widget         | 7-day daily fleet totals    | When `provisioned-gateways.trend.points` has at least two points (GFT-06)       |
| `memory` widget                 | 7-day daily used memory     | When `memory.trend.points` has at least two points (HCUT-08)                    |
| `cpu` widget                    | 7-day daily used CPU        | When `cpu.trend.points` has at least two points (HCUT-08)                       |
| `pods` widget                   | 7-day daily used pods       | When `pods.trend.points` has at least two points (HCUT-08)                      |
| `registered-users` widget       | 30-day daily unique logins  | When `trend.points` has at least two points                                     |
| `provision-reliability` widget  | 24-hour hourly success rate | When BFF returns at least two `hourly_success_rate` buckets                     |
| Usage summary Gateways row      | Trend arrow on total        | When `provisioned-gateways.trend` is present (GFT-07)                           |
| Usage summary Sandboxes row     | Trend arrows                | When `provisioned-sandboxes.hourlyTrend` or `trend` is present                  |
| Usage summary Users row         | Trend arrow on total        | When `registered-users.trend` is present (`getMetricTrendChange`, 5% threshold) |
| System summary Memory row       | Trend arrow on used GiB     | When `memory.trend` change is at least 5% (HCUT-08)                             |
| System summary CPU row          | Trend arrow on used cores   | When `cpu.trend` change is at least 5% (HCUT-08)                                |
| System summary Pods row         | Trend arrow on used pods    | When `pods.trend` change is at least 5% (HCUT-08)                               |
| System summary Success rate row | Trend arrow on success rate | When `provision-reliability.successRateTrend` change is at least 5%             |

Other metric cards do not load historical series unless listed above.

## API reliability

Reliability dashboard (`/dashboard/reliability`) loads through
`useGetReliabilityMetricsData` → `dashboard.getReliabilityMetrics` →
`createDashboardControlPlaneAdapter.getReliabilityMetrics`. This source is
**not** part of the operational `metricSources` list (ARM-05).

| Source ID         | BFF route                          | Metrics emitted                                     |
| ----------------- | ---------------------------------- | --------------------------------------------------- |
| `api-reliability` | `GET /api/metrics/api-reliability` | `api-request-rate`, `api-error-rate`, `api-latency` |
| `control-plane-reconciliation` | `GET /api/metrics/control-plane-reconciliation` | `reconciliation-failures`, `reconciliation-retries`, `reconciliation-lag`, `stale-resource-status-count` |

The reliability dashboard SHALL read metric data through the BFF routes above.
The UI SHALL NOT read or write metric data directly to a database. Historical
reconciliation data uses a rolling 7 x 24-hour Prometheus range query with
hourly samples.

| Metric ID | Prometheus type | Current value | Unit |
| --- | --- | --- | --- |
| `reconciliation-failures` | Counter | 5-minute rate | `failures/s` |
| `reconciliation-retries` | Counter | 5-minute rate | `retries/s` |
| `reconciliation-lag` | Histogram | P50 | `sec` |
| `stale-resource-status-count` | Gauge | Current count | `count` |

Fetch failures soft-return `failedSources: ["api-reliability"]` with an empty
metrics list (abort still throws). `mergeReliabilityDashboardMetrics` then
preserves stale widgets on refresh; an empty merge after first load is treated
as total failure.

Instant fields come from the API server framework Prometheus histogram
`api_inbound_request_duration_*` scraped via the `hypershell-api-server`
ServiceMonitor (`job="hypershell-api-server"`). The HTTP status label is
`code`. (OTel `http.server.request.duration` /
`http_server_request_duration_seconds` is not required for this dashboard;
Kind does not currently set API-server OTLP export.)

| Instant field (BFF)   | Metric ID          | Unit    | Display rounding | Trend field (BFF)            |
| --------------------- | ------------------ | ------- | ---------------- | ---------------------------- |
| `request_rate`        | `api-request-rate` | `req/s` | 2 fractional     | `hourly_request_rate`        |
| `error_rate_percent`  | `api-error-rate`   | `%`     | 2 fractional     | `hourly_error_rate_percent`  |
| `latency_p50_seconds` | `api-latency`      | `sec`   | 3 fractional     | `hourly_latency_p50_seconds` |

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

## Adding a new source

1. Expose a same-origin BFF route (or reuse an existing API route) for browser access.
2. Add an independent source in `createDashboardControlPlaneAdapter` and map the response into `OperationalMetric`.
3. Update this file and the dashboard page description in `messages.ts`.
4. Register the source in `dashboard-metric-sources.ts` when layout persistence should track it.
