# Operational dashboard data sources

This document tracks which operational dashboard widgets are backed by live
data. Widgets without a connected source still appear on the dashboard; they
render the localized "Metric unavailable" empty state from
`operational-dashboard-page.tsx`.

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

| Source ID                      | BFF / API routes                                                                                                                                                | Metrics emitted                                                                            |
| ------------------------------ | --------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------ |
| `gateway-metrics`              | `GET /api/metrics/gateways`, `GET /api/metrics/gateway-sandboxes`, `GET /api/metrics/gateway-provision-duration`, `GET /api/metrics/gateway-provision-outcomes` | `provisioned-gateways`, `provisioned-sandboxes`, `provision-time`, `provision-reliability` |
| `registered-users`             | `GET /api/metrics/registered-users`                                                                                                                             | `registered-users`                                                                         |
| `gateway-release-distribution` | SDK `GET /api/hypershell/v1/gateways`, `GET /api/hypershell/v1/gateway_releases`                                                                                | `gateway-releases`                                                                         |
| `platform-inventory`           | `GET /api/metrics/platform-inventory`                                                                                                                           | `managed-clusters`                                                                         |
| `cluster-memory`               | `GET /api/metrics/cluster-memory`                                                                                                                               | `memory`                                                                                   |
| `cluster-cpu`                  | `GET /api/metrics/cluster-cpu`                                                                                                                                  | `cpu`                                                                                      |
| `cluster-pods`                 | `GET /api/metrics/cluster-pods`                                                                                                                                 | `pods`                                                                                     |
| `cluster-nodes`                | `GET /api/metrics/cluster-nodes`                                                                                                                                | `nodes`                                                                                    |

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
| `gateway-releases`          | `gateway-releases`      | Per-release counts from paginated gateway and gateway-release list APIs. Spec: `platform/gateway-release-distribution.spec.md`. |
| `registered-users`          | `registered-users`      | See [Registered users](#registered-users) below.                                                                                |
| `provision-time`            | `provision-time`        | See [Provision time](#provision-time) below.                                                                                    |
| `provision-reliability`     | `provision-reliability` | See [Provision reliability](#provision-reliability) below.                                                                      |
| `memory`                    | `memory`                | Node-exporter `node_memory_*` bytes, mapped to GiB.                                                                             |
| `cpu`                       | `cpu`                   | Node-exporter `node_cpu_seconds_total` capacity and 5m non-idle rate, mapped to whole cores.                                    |
| `pods`                      | `pods`                  | kube-state-metrics allocatable pod capacity, live pod count, and phase breakdown.                                               |
| `nodes`                     | `nodes`                 | kube-state-metrics `kube_node_info` total and Ready condition.                                                                  |
| `inventory-summary`         | `managed-clusters`      | Totals, 30-day cluster creations, and status icons (OP-DASH-20).                                                                |
| `managed-cluster-providers` | `managed-clusters`      | Provider donut from `inventoryProviders`.                                                                                       |
| `managed-cluster-regions`   | `managed-clusters`      | Region donut from `inventoryRegions` (`{region} ({provider})` keys).                                                            |
| `usage-summary`             | several                 | Gateways (`provisioned-gateways`), sandboxes (`provisioned-sandboxes`), users (`registered-users`).                             |
| `system-summary`            | several                 | Provision time and reliability metrics from `gateway-metrics` source.                                                           |

`provisioned-sandboxes` has no standalone widget; it appears only in the usage
summary sandboxes row (`hypershell_gateways_active_sandboxes_total` sum).

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

## Sparklines and trends

| Location                       | Series                      | When shown                                                                      |
| ------------------------------ | --------------------------- | ------------------------------------------------------------------------------- |
| `registered-users` widget      | 30-day daily unique logins  | When `trend.points` has at least two points                                     |
| `provision-reliability` widget | 24-hour hourly success rate | When BFF returns at least two `hourly_success_rate` buckets                     |
| Usage summary Users row        | Trend arrow on total        | When `registered-users.trend` is present (`getMetricTrendChange`, 5% threshold) |

Other metric cards do not load historical series.

## Adding a new source

1. Expose a same-origin BFF route (or reuse an existing API route) for browser access.
2. Add an independent source in `createDashboardControlPlaneAdapter` and map the response into `OperationalMetric`.
3. Update this file and the dashboard page description in `messages.ts`.
4. Register the source in `dashboard-metric-sources.ts` when layout persistence should track it.
