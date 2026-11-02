# Gateway Provision Time

**Status:** Active
**Applies to:** `deploy/base` and `deploy/kind` control-plane metrics pipeline and Prometheus configuration, `components/web-console` BFF and dashboard adapter, `packages/operational-dashboard-ui`, `components/control-plane` (`gateway.provision.duration` histogram per `platform/control-plane-observability.spec.md` OBS-CPL-07)

## Purpose

Expose **gateway provision duration statistics** on the operational dashboard so dashboard operators can see how long it typically takes for gateways in the fleet to reach the `Running` phase.

This is **HyperShell application telemetry** recorded by the control plane when a gateway completes its first successful transition to `Running`. It is **not** cluster infrastructure metrics, per-gateway SLA tracking in the UI, or control-plane reconcile latency in isolation.

Version 3 aggregates four fleet-wide statistics from the control-plane histogram:

| Statistic | Meaning |
| --- | --- |
| **Mean** | Arithmetic mean of all recorded provision durations |
| **P50 (median)** | 50th percentile of recorded provision durations |
| **P95** | 95th percentile of recorded provision durations |
| **P99** | 99th percentile during the selected supported window |

All four values SHALL be presented in **seconds** on the dashboard, matching the underlying histogram unit (OBS-CPL-07). P99 SHALL use the supported 7-day window, a 300-second target, and optional daily P99 points. A 30-day request SHALL leave existing fields unchanged and mark the new P99 fields unavailable because Prometheus retains only 7 days.

The operational dashboard `system-summary` card keeps its mean, median (P50), and P95 rows (WEB-OPS-13). Version 3 extends the Provisioning dashboard's `provision-time` widget with P99, the 5-minute target, warning state, and daily P99 trend. The default layout also includes the existing standalone `provision-time` widget (WEB-OPS-10).

### Relationship to other specifications

- **Control-plane observability** (`platform/control-plane-observability.spec.md` OBS-CPL-07) owns recording `gateway.provision.duration` as an OTLP histogram in seconds with explicit bucket boundaries from 1 second through 15 minutes and the companion `gateway.provision.outcomes` counter (`platform/gateway-provision-outcomes.spec.md`).
- **Operational dashboard** (`web-console/operational-dashboard.spec.md`) owns the `provision-time` system-summary rows, refresh policy (WEB-OPS-09), independent metric sources (WEB-OPS-19), and dashboard-operator access (WEB-OPS-04).
- **Cluster memory/CPU/pods/nodes** specs follow the same BFF Prometheus proxy pattern used here.
- **Gateway list metrics** (`provisioned-gateways`, `provisioned-sandboxes`) remain REST-driven and are unrelated to provision duration.
- **Gateway metrics dashboard** (`platform/gateway-metrics-dashboard.spec.md`) exposes Prometheus gateway phase counts, not provision duration.

### Scope note: fleet-wide vs RBAC-filtered

The histogram reflects every provision the control plane has observed across the platform. It is **not** filtered to gateways visible in the caller's paginated gateway list. This matches other hub-cluster infrastructure metrics (memory, CPU, pods, nodes) and gives operators a stable fleet-wide provision-time signal independent of per-user list visibility.

Historical trend series other than daily P99, per-gateway duration breakdown, and gateway-list timestamp proxies are out of scope (see Non-Goals).

## Requirements

### Requirement: OBS-GPT-09 -- Prometheus Histogram Availability

The Prometheus instance configured for the web-console BFF (`PROMETHEUS_URL`, same origin as cluster metrics) SHALL expose the control-plane provision-duration histogram in Prometheus exposition format.

The canonical series prefix SHALL be `gateway_provision_duration_seconds` with the standard histogram suffixes `_bucket`, `_count`, and `_sum`, derived from the OTLP metric `gateway.provision.duration` (OBS-CPL-07).

Deploy and local-development configuration SHALL route control-plane OTLP metrics into that Prometheus instance. When the histogram is absent or has zero observations, provision-time collection SHALL fail per OBS-GPT-07 rather than synthesizing values.

`DATA_SOURCES.md` SHALL document the canonical PromQL expressions once scrape or collector wiring is chosen during implementation.

#### Scenario: OBS-GPT-09.01 -- Histogram present after gateway provisions

- GIVEN the control plane has recorded at least one `gateway.provision.duration` observation
- AND the metrics pipeline exposes `gateway_provision_duration_seconds_count` greater than zero
- WHEN the BFF evaluates provision-time PromQL
- THEN mean, P50, P95, and 7-day P99 SHALL be computable from the histogram series

#### Scenario: OBS-GPT-09.02 -- Zero observations fails collection

- GIVEN `gateway_provision_duration_seconds_count` is zero or absent
- WHEN a dashboard operator loads operational metrics
- THEN provision-time collection SHALL fail
- AND the BFF SHALL respond with HTTP `502` for `GET /api/metrics/gateway-provision-duration`

---

### Requirement: OBS-GPT-01 -- Provision Duration Measurement Contract

The platform SHALL compute four non-negative durations in **seconds** from the fleet-wide histogram:

| Field | PromQL (canonical) |
| --- | --- |
| `mean_seconds` | `gateway_provision_duration_seconds_sum / gateway_provision_duration_seconds_count` |
| `p50_seconds` | `histogram_quantile(0.50, sum(gateway_provision_duration_seconds_bucket) by (le))` |
| `p95_seconds` | `histogram_quantile(0.95, sum(gateway_provision_duration_seconds_bucket) by (le))` |
| `p99_seconds` | `histogram_quantile(0.99, sum by (le) (increase(gateway_provision_duration_seconds_bucket[7d])))` |

The existing mean, P50, and P95 expressions SHALL use the same evaluation timestamp. The adapter SHALL format each value in **seconds** without unit conversion. P99 SHALL use the trailing 7-day increase and SHALL expose `target_seconds: 300`.

The adapter SHALL expose the result on the `provision-time` `OperationalMetric` as:

| Field | Value |
| --- | --- |
| `value` | Decimal string of `mean_seconds`, rounded to **two** fractional digits |
| `unit` | `"sec"` |
| `provisionDuration.mean` | Same string as `value` |
| `provisionDuration.p50` | Decimal string of `p50_seconds`, rounded to two fractional digits |
| `provisionDuration.p95` | Decimal string of `p95_seconds`, rounded to two fractional digits |
| `provisionDuration.p99` | Decimal string of `p99_seconds`, rounded to two fractional digits |
| `provisionDuration.target` | Decimal string of `target_seconds` |
| `provisionDuration.dailyP99` | Optional oldest-to-newest daily `{ date, p99Seconds }` points |

Version 3 SHALL emit a warning status when P99 exceeds the 300-second target. It SHALL NOT emit a healthy status when P99 is unavailable.

When `gateway_provision_duration_seconds_count` is zero, any PromQL result is non-finite (`NaN` or `+Inf`), or any formatted second value is non-finite, collection SHALL fail (OBS-GPT-07).

#### Scenario: OBS-GPT-01.01 -- Histogram yields mean three fifteen, P50 two eighty-eight, P95 seven twenty-six seconds

- GIVEN Prometheus returns `mean_seconds = 315`, `p50_seconds = 288`, and `p95_seconds = 726`
- WHEN the adapter maps the BFF response
- THEN `value` and `provisionDuration.mean` SHALL be `"315.00"`
- AND `provisionDuration.p50` SHALL be `"288.00"`
- AND `provisionDuration.p95` SHALL be `"726.00"`
- AND `provisionDuration.p99` SHALL contain the 7-day 99th-percentile value
- AND `provisionDuration.target` SHALL be `"300"`
- AND `unit` SHALL be `"sec"`

---

### Requirement: OBS-GPT-02 -- Control-Plane Semantics

Provision-duration samples SHALL follow OBS-CPL-07:

- One observation per gateway on its **first** successful transition to `Running`
- Duration computed from API-server `created_at` and `updated_at` on the successful phase update
- No Gateway identifier label on the metric
- Recoveries from `Degraded` to `Running` SHALL NOT produce additional observations

The dashboard SHALL NOT recompute provision duration from gateway list timestamps. The gateway-list `updated_at - created_at` proxy used in the superseded version 1 approach is retired.

#### Scenario: OBS-GPT-02.01 -- Degraded recovery does not shift percentiles

- GIVEN a gateway already contributed one provision-duration observation when it first reached `Running`
- WHEN that gateway later recovers from `Degraded` to `Running`
- THEN the histogram count SHALL NOT increase
- AND dashboard percentiles SHALL remain unchanged by that recovery

---

### Requirement: OBS-GPT-03 -- Prometheus Data Source

Hub-cluster provision duration SHALL be derived from Prometheus instant queries executed by the web-console BFF against `PROMETHEUS_URL` (same configuration as `platform/cluster-memory.spec.md` OBS-MEM-03).

The BFF SHALL evaluate the existing three PromQL expressions as instant queries and the P99 expression over the selected 7-day window. Queries SHALL aggregate with `sum(...)` across all histogram series so control-plane replicas contribute to one fleet-wide histogram.

The BFF SHALL NOT contact the HyperShell API server or Kubernetes API directly for provision duration in version 2.

#### Scenario: OBS-GPT-03.01 -- Prometheus unavailable fails collection

- GIVEN Prometheus is unreachable from the BFF within the query timeout
- WHEN a dashboard operator loads operational metrics
- THEN the BFF provision-duration route SHALL respond with HTTP `502`
- AND the `provision-time` metric SHALL be omitted from the adapter response (WEB-OPS-19)

---

### Requirement: OBS-GPT-04 -- BFF Gateway Provision Duration Route

The web-console BFF SHALL expose `GET /api/metrics/gateway-provision-duration` as a same-origin route that:

1. Requires an authenticated session when OIDC is enabled (same session gate as `GET /api/metrics/cluster-memory`)
2. Executes Prometheus queries for mean, P50, P95, and supported 7-day P99 (OBS-GPT-01)
3. Returns JSON:

```json
{
  "mean_seconds": 315,
  "p50_seconds": 288,
  "p95_seconds": 726,
  "p99_seconds": 810,
  "target_seconds": 300,
  "daily_p99": [{ "date": "2026-10-28", "p99_seconds": 810 }],
  "observation_count": 42
}
```

`observation_count` SHALL be the integer value of `gateway_provision_duration_seconds_count` at the same evaluation time.

`daily_p99` MAY be omitted when daily points are unavailable. For a 30-day request, `p99_seconds` and `daily_p99` SHALL be omitted while the existing fields retain their current behavior.

The route SHALL NOT forward to the HyperShell API server and SHALL NOT require a HyperShell API bearer token.

On Prometheus failure, timeout, non-success Prometheus response status, zero observation count, or non-finite required mean/P50/P95 values, the BFF SHALL respond with HTTP `502` and `{ "error": "Metrics unavailable", "statusCode": 502 }`. A missing or non-finite P99 SHALL omit only P99 and daily P99 fields. The BFF SHALL NOT return zeroed duration figures as a fallback.

#### Scenario: OBS-GPT-04.01 -- Authenticated dashboard operator receives duration seconds

- GIVEN OIDC is enabled and the caller has a valid session
- AND Prometheus returns successful instant-query results with `observation_count` greater than zero
- WHEN the caller sends `GET /api/metrics/gateway-provision-duration`
- THEN the BFF SHALL respond with HTTP `200` and the OBS-GPT-04 JSON body

#### Scenario: OBS-GPT-04.02 -- Unauthenticated caller is rejected

- GIVEN OIDC is enabled and the caller has no session
- WHEN the caller sends `GET /api/metrics/gateway-provision-duration`
- THEN the BFF SHALL respond with HTTP `401` or redirect to login per existing metrics-route policy

---

### Requirement: OBS-GPT-05 -- Operational Dashboard Metric Mapping

The operational dashboard host adapter (`createDashboardControlPlaneAdapter`) SHALL fetch `GET /api/metrics/gateway-provision-duration` as an independent metric source (WEB-OPS-19) and emit a `provision-time` `OperationalMetric` per OBS-GPT-01.

The `OperationalMetric` type SHALL include an optional `provisionDuration` object:

```typescript
interface OperationalMetricProvisionDuration {
  mean: string;
  p50: string;
  p95: string;
  p99?: string;
  target?: string;
  dailyP99?: Array<{ date: string; p99Seconds: number }>;
}
```

The `system-summary` card SHALL render **three** provision-duration rows sourced from this metric (WEB-OPS-13):

| Row label (localized) | Field |
| --- | --- |
| Provision time (average) | `value` / `provisionDuration.mean` with `unit` |
| Provision time (P50) | `provisionDuration.p50` with `unit` |
| Provision time (P95) | `provisionDuration.p95` with `unit` |

When `provisionDuration` is absent but `value` and `unit` are present, the UI MAY render only the mean row for backward compatibility during rollout.

The Provisioning dashboard SHALL additionally render P99, the localized 5-minute target, a warning when P99 exceeds the target, and a daily P99 sparkline when points are available. Missing observations SHALL render unavailable rather than `0`.

#### Scenario: OBS-GPT-05.01 -- System summary shows three second-labeled rows

- GIVEN `provisionDuration` is `{ mean: "315.00", p50: "288.00", p95: "726.00" }` and `unit` is `"sec"`
- WHEN the system-summary provision-duration rows render
- THEN the operator SHALL see localized labels for average, P50, and P95
- AND each row SHALL show the corresponding value with the `sec` unit presentation

---

### Requirement: OBS-GPT-06 -- Decoupling from Gateway List

Provision-time collection SHALL NOT depend on the paginated gateway list (`GET /api/hypershell/v1/gateways`).

A non-success BFF response for `GET /api/metrics/gateways` or `GET /api/metrics/gateway-sandboxes` SHALL fail the entire `gateway-metrics` source and omit `provisioned-gateways`, `provisioned-sandboxes`, `provision-time`, and `provision-reliability`, even when provision-duration or provision-outcomes routes would have succeeded on their own (WEB-OPS-23).

Conversely, provision-duration BFF failure SHALL omit only `provision-time`. Provision-outcomes BFF failure SHALL omit only `provision-reliability`. It SHALL NOT omit `provisioned-gateways` or `provisioned-sandboxes` when their BFF routes succeed (WEB-OPS-19, WEB-OPS-23).

#### Scenario: OBS-GPT-06.01 -- Prometheus gateway counts down omits entire gateway-metrics source

- GIVEN `GET /api/metrics/gateways` returns HTTP `502`
- AND `GET /api/metrics/gateway-provision-duration` would succeed if queried
- WHEN the operator opens `/dashboard`
- THEN the `gateway-metrics` source SHALL be treated as failed
- AND `provisioned-gateways`, `provisioned-sandboxes`, `provision-time`, and `provision-reliability` SHALL be omitted
- AND registered-user, inventory, and cluster metrics SHALL still load when their sources succeed

---

### Requirement: OBS-GPT-07 -- Refresh and Error Semantics

Provision time SHALL load through the existing operational dashboard metrics query (`useGetMetricsData`) and SHALL inherit its refresh policy (`operationalDashboardRefreshMilliseconds`, currently 15 minutes) and manual refresh behavior (WEB-OPS-09).

When provision-duration collection fails (BFF `502`, zero observations, non-finite quantiles, or adapter validation error), the adapter SHALL omit only the `provision-time` metric. Other metric sources SHALL still contribute when they succeed (WEB-OPS-19). The dashboard SHALL NOT display `0` seconds as a fallback.

#### Scenario: OBS-GPT-07.01 -- No histogram observations omits provision time only

- GIVEN every other metric source succeeds
- AND `gateway_provision_duration_seconds_count` is zero
- WHEN the operator opens `/dashboard`
- THEN gateway-metrics and cluster metrics SHALL still load
- AND the provision-time summary rows SHALL render the localized metric-unavailable state
- AND the dashboard SHALL NOT enter the total load-error state

---

### Requirement: OBS-GPT-08 -- Verification

The web console SHALL include unit tests for:

- BFF route PromQL mapping and JSON response formatting (including P99, target, daily P99, and `observation_count`)
- BFF `502` on zero count, Prometheus errors, and non-finite quantiles
- Adapter mapping from BFF JSON to `provision-time` with `provisionDuration.mean`, `.p50`, and `.p95`
- Partial gateway-metrics source behavior: gateway-count or sandbox route failure omits the entire gateway-metrics source; provision-duration or provision-outcomes route failure omits only the corresponding metric (OBS-GPT-06)

The operational dashboard package SHALL include unit tests or Storybook fixtures for the three-row system-summary presentation when `provisionDuration` is present.

#### Scenario: OBS-GPT-08.01 -- CI exercises adapter mapping

- GIVEN a mocked BFF response with `mean_seconds: 315`, `p50_seconds: 288`, `p95_seconds: 726`, and `observation_count: 2`
- WHEN dashboard adapter unit tests run
- THEN they SHALL assert the `provision-time` metric `id`, `value: "315.00"`, `unit: "sec"`, and `provisionDuration` with `p50: "288.00"` and `p95: "726.00"`

## Non-Goals

- Per-gateway provision duration in the UI or API
- Historical provision-time trends other than the 7-day daily P99 series
- RBAC-scoped provision duration derived from the gateway list
- Gateway-list timestamp proxy (`updated_at - created_at`) as a data source
- Including `Degraded` gateways as provision samples
- Persisting a dedicated `provisioned_at` timestamp on the Gateway resource
- Cluster infrastructure metrics (see cluster memory/CPU/pods/nodes specs)
- Min, max, or arbitrary custom quantiles beyond mean, P50, P95, and P99

## Supersedes

Version 1 of this spec (gateway-list mean from `updated_at - created_at`, OBS-GPT-01 through OBS-GPT-04 and OBS-GPT-06 list-consistency requirements) is superseded by version 2. Implementation that still uses the gateway-list proxy SHALL be treated as drift until GPT-W2 lands.

## Primary Basis

- `platform/control-plane-observability.spec.md` (OBS-CPL-07)
- `web-console/operational-dashboard.spec.md` (WEB-OPS-08, WEB-OPS-13, WEB-OPS-19)
- `platform/cluster-memory.spec.md` (BFF Prometheus proxy pattern)
- `platform/gateway-metrics-dashboard.spec.md` (`PROMETHEUS_URL` configuration)
