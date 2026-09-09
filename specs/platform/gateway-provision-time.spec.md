# Gateway Provision Time

**Status:** Active
**Applies to:** `deploy/base` and `deploy/kind` control-plane metrics pipeline and Prometheus configuration, `components/web-console` BFF and dashboard adapter, `packages/operational-dashboard-ui`, `components/control-plane` (`gateway.provision.duration` histogram per `platform/control-plane-observability.spec.md` CP-OBS-07)

## Purpose

Expose **gateway provision duration statistics** on the operational dashboard so dashboard operators can see how long it typically takes for gateways in the fleet to reach the `Running` phase.

This is **HyperShell application telemetry** recorded by the control plane when a gateway completes its first successful transition to `Running`. It is **not** cluster infrastructure metrics, per-gateway SLA tracking in the UI, or control-plane reconcile latency in isolation.

Version 2 aggregates three fleet-wide statistics from the control-plane histogram:

| Statistic | Meaning |
| --- | --- |
| **Mean** | Arithmetic mean of all recorded provision durations |
| **P50 (median)** | 50th percentile of recorded provision durations |
| **P95** | 95th percentile of recorded provision durations |

All three values SHALL be presented in **minutes** on the dashboard. The underlying histogram unit is **seconds** (CP-OBS-07).

The operational dashboard `system-summary` card already renders a provision-time row (OP-DASH-13). Version 2 extends that card with separate rows for mean, median (P50), and P95. There is no standalone `provision-time` widget in the default layout.

### Relationship to other specifications

- **Control-plane observability** (`platform/control-plane-observability.spec.md` CP-OBS-07) owns recording `gateway.provision.duration` as an OTLP histogram in seconds with explicit bucket boundaries from 1 second through 15 minutes.
- **Operational dashboard** (`web-console/operational-dashboard.spec.md`) owns the `provision-time` system-summary rows, refresh policy (OP-DASH-09), independent metric sources (OP-DASH-19), and dashboard-operator access (OP-DASH-04).
- **Cluster memory/CPU/pods/nodes** specs follow the same BFF Prometheus proxy pattern used here.
- **Gateway list metrics** (`provisioned-gateways`, `provisioned-sandboxes`) remain REST-driven and are unrelated to provision duration.
- **Gateway metrics dashboard** (`platform/gateway-metrics-dashboard.spec.md`) exposes Prometheus gateway phase counts, not provision duration.

### Scope note: fleet-wide vs RBAC-filtered

The histogram reflects every provision the control plane has observed across the platform. It is **not** filtered to gateways visible in the caller's paginated gateway list. This matches other hub-cluster infrastructure metrics (memory, CPU, pods, nodes) and gives operators a stable fleet-wide provision-time signal independent of per-user list visibility.

Historical trend series, per-gateway duration breakdown, and gateway-list timestamp proxies are out of scope (see Non-Goals).

## Requirements

### Requirement: GPT-00 -- Prometheus Histogram Availability

The Prometheus instance configured for the web-console BFF (`PROMETHEUS_URL`, same origin as cluster metrics) SHALL expose the control-plane provision-duration histogram in Prometheus exposition format.

The canonical series prefix SHALL be `gateway_provision_duration_seconds` with the standard histogram suffixes `_bucket`, `_count`, and `_sum`, derived from the OTLP metric `gateway.provision.duration` (CP-OBS-07).

Deploy and local-development configuration SHALL route control-plane OTLP metrics into that Prometheus instance. When the histogram is absent or has zero observations, provision-time collection SHALL fail per GPT-07 rather than synthesizing values.

`DATA_SOURCES.md` SHALL document the canonical PromQL expressions once scrape or collector wiring is chosen during implementation.

#### Scenario: Histogram present after gateway provisions

- GIVEN the control plane has recorded at least one `gateway.provision.duration` observation
- AND the metrics pipeline exposes `gateway_provision_duration_seconds_count` greater than zero
- WHEN the BFF evaluates provision-time PromQL
- THEN mean, P50, and P95 SHALL be computable from the histogram series

#### Scenario: Zero observations fails collection

- GIVEN `gateway_provision_duration_seconds_count` is zero or absent
- WHEN a dashboard operator loads operational metrics
- THEN provision-time collection SHALL fail
- AND the BFF SHALL respond with HTTP `502` for `GET /api/metrics/gateway-provision-duration`

---

### Requirement: GPT-01 -- Provision Duration Measurement Contract

The platform SHALL compute three non-negative durations in **seconds** from the fleet-wide histogram at a single Prometheus instant-evaluation time:

| Field | PromQL (canonical) |
| --- | --- |
| `mean_seconds` | `gateway_provision_duration_seconds_sum / gateway_provision_duration_seconds_count` |
| `p50_seconds` | `histogram_quantile(0.50, sum(gateway_provision_duration_seconds_bucket) by (le))` |
| `p95_seconds` | `histogram_quantile(0.95, sum(gateway_provision_duration_seconds_bucket) by (le))` |

All three expressions SHALL use the same evaluation timestamp. The BFF SHALL convert each value to **minutes** by dividing by `60`.

The adapter SHALL expose the result on the `provision-time` `OperationalMetric` as:

| Field | Value |
| --- | --- |
| `value` | Decimal string of `mean_seconds / 60`, rounded to **two** fractional digits |
| `unit` | `"minutes"` |
| `provisionDuration.mean` | Same string as `value` |
| `provisionDuration.p50` | Decimal string of `p50_seconds / 60`, rounded to two fractional digits |
| `provisionDuration.p95` | Decimal string of `p95_seconds / 60`, rounded to two fractional digits |

Version 2 SHALL NOT emit `total`, `status`, or `trend` on the `provision-time` metric.

When `gateway_provision_duration_seconds_count` is zero, any PromQL result is non-finite (`NaN` or `+Inf`), or any converted minute value is non-finite, collection SHALL fail (GPT-07).

#### Scenario: Histogram yields mean five point two five, P50 four point eight, P95 twelve point one minutes

- GIVEN Prometheus returns `mean_seconds = 315`, `p50_seconds = 288`, and `p95_seconds = 726`
- WHEN the adapter maps the BFF response
- THEN `value` and `provisionDuration.mean` SHALL be `"5.25"`
- AND `provisionDuration.p50` SHALL be `"4.80"`
- AND `provisionDuration.p95` SHALL be `"12.10"`
- AND `unit` SHALL be `"minutes"`

---

### Requirement: GPT-02 -- Control-Plane Semantics

Provision-duration samples SHALL follow CP-OBS-07:

- One observation per gateway on its **first** successful transition to `Running`
- Duration computed from API-server `created_at` and `updated_at` on the successful phase update
- No Gateway identifier label on the metric
- Recoveries from `Degraded` to `Running` SHALL NOT produce additional observations

The dashboard SHALL NOT recompute provision duration from gateway list timestamps. The gateway-list `updated_at - created_at` proxy used in the superseded version 1 approach is retired.

#### Scenario: Degraded recovery does not shift percentiles

- GIVEN a gateway already contributed one provision-duration observation when it first reached `Running`
- WHEN that gateway later recovers from `Degraded` to `Running`
- THEN the histogram count SHALL NOT increase
- AND dashboard percentiles SHALL remain unchanged by that recovery

---

### Requirement: GPT-03 -- Prometheus Data Source

Hub-cluster provision duration SHALL be derived from Prometheus instant queries executed by the web-console BFF against `PROMETHEUS_URL` (same configuration as `platform/cluster-memory.spec.md` CM-03).

The BFF SHALL evaluate the three PromQL expressions in GPT-01 as instant queries. Queries SHALL aggregate with `sum(...)` across all histogram series so unlabeled control-plane replicas contribute to one fleet-wide histogram.

The BFF SHALL NOT contact the HyperShell API server or Kubernetes API directly for provision duration in version 2.

#### Scenario: Prometheus unavailable fails collection

- GIVEN Prometheus is unreachable from the BFF within the query timeout
- WHEN a dashboard operator loads operational metrics
- THEN the BFF provision-duration route SHALL respond with HTTP `502`
- AND the `provision-time` metric SHALL be omitted from the adapter response (OP-DASH-19)

---

### Requirement: GPT-04 -- BFF Gateway Provision Duration Route

The web-console BFF SHALL expose `GET /api/metrics/gateway-provision-duration` as a same-origin route that:

1. Requires an authenticated session when OIDC is enabled (same session gate as `GET /api/metrics/cluster-memory`)
2. Executes Prometheus instant queries for mean, P50, and P95 (GPT-01)
3. Returns JSON:

```json
{
  "mean_seconds": 315,
  "p50_seconds": 288,
  "p95_seconds": 726,
  "observation_count": 42
}
```

`observation_count` SHALL be the integer value of `gateway_provision_duration_seconds_count` at the same evaluation time.

The route SHALL NOT forward to the HyperShell API server and SHALL NOT require a HyperShell API bearer token.

On Prometheus failure, timeout, non-success Prometheus response status, zero observation count, or non-finite computed quantiles, the BFF SHALL respond with HTTP `502` and `{ "error": "Metrics unavailable", "statusCode": 502 }`. The BFF SHALL NOT return zeroed duration figures as a fallback.

#### Scenario: Authenticated dashboard operator receives duration seconds

- GIVEN OIDC is enabled and the caller has a valid session
- AND Prometheus returns successful instant-query results with `observation_count` greater than zero
- WHEN the caller sends `GET /api/metrics/gateway-provision-duration`
- THEN the BFF SHALL respond with HTTP `200` and the GPT-04 JSON body

#### Scenario: Unauthenticated caller is rejected

- GIVEN OIDC is enabled and the caller has no session
- WHEN the caller sends `GET /api/metrics/gateway-provision-duration`
- THEN the BFF SHALL respond with HTTP `401` or redirect to login per existing metrics-route policy

---

### Requirement: GPT-05 -- Operational Dashboard Metric Mapping

The operational dashboard host adapter (`createDashboardControlPlaneAdapter`) SHALL fetch `GET /api/metrics/gateway-provision-duration` as an independent metric source (OP-DASH-19) and emit a `provision-time` `OperationalMetric` per GPT-01.

The `OperationalMetric` type SHALL include an optional `provisionDuration` object:

```typescript
interface OperationalMetricProvisionDuration {
  mean: string;
  p50: string;
  p95: string;
}
```

The `system-summary` card SHALL render **three** provision-duration rows sourced from this metric (OP-DASH-13):

| Row label (localized) | Field |
| --- | --- |
| Provision time (average) | `value` / `provisionDuration.mean` with `unit` |
| Provision time (P50) | `provisionDuration.p50` with `unit` |
| Provision time (P95) | `provisionDuration.p95` with `unit` |

When `provisionDuration` is absent but `value` and `unit` are present, the UI MAY render only the mean row for backward compatibility during rollout.

#### Scenario: System summary shows three minute-labeled rows

- GIVEN `provisionDuration` is `{ mean: "5.25", p50: "4.80", p95: "12.10" }` and `unit` is `"minutes"`
- WHEN the system-summary provision-duration rows render
- THEN the operator SHALL see localized labels for average, P50, and P95
- AND each row SHALL show the corresponding value with the minutes unit presentation

---

### Requirement: GPT-06 -- Decoupling from Gateway List

Provision-time collection SHALL NOT depend on the paginated gateway list (`GET /api/hypershell/v1/gateways`).

Gateway list pagination failure SHALL omit `provisioned-gateways` and `provisioned-sandboxes` only. It SHALL NOT omit `provision-time` when the BFF provision-duration route succeeds.

Conversely, provision-duration BFF failure SHALL omit only `provision-time`. It SHALL NOT affect gateway-list-derived metrics (OP-DASH-19).

#### Scenario: Gateway list down does not hide provision time

- GIVEN the gateway list request fails
- AND `GET /api/metrics/gateway-provision-duration` succeeds
- WHEN the operator opens `/dashboard`
- THEN `provision-time` SHALL appear in the adapter response with mean, P50, and P95
- AND gateway and sandbox widgets SHALL render the localized metric-unavailable state

---

### Requirement: GPT-07 -- Refresh and Error Semantics

Provision time SHALL load through the existing operational dashboard metrics query (`useGetMetricsData`) and SHALL inherit its refresh policy (`operationalDashboardRefreshMilliseconds`, currently 15 minutes) and manual refresh behavior (OP-DASH-09).

When provision-duration collection fails (BFF `502`, zero observations, non-finite quantiles, or adapter validation error), the adapter SHALL omit only the `provision-time` metric. Other metric sources SHALL still contribute when they succeed (OP-DASH-19). The dashboard SHALL NOT display `0` minutes as a fallback.

#### Scenario: No histogram observations omits provision time only

- GIVEN every other metric source succeeds
- AND `gateway_provision_duration_seconds_count` is zero
- WHEN the operator opens `/dashboard`
- THEN cluster and gateway-list metrics SHALL still load
- AND the provision-time summary rows SHALL render the localized metric-unavailable state
- AND the dashboard SHALL NOT enter the total load-error state

---

### Requirement: GPT-08 -- Verification

The web console SHALL include unit tests for:

- BFF route PromQL mapping and JSON response formatting (including `observation_count`)
- BFF `502` on zero count, Prometheus errors, and non-finite quantiles
- Adapter mapping from BFF JSON to `provision-time` with `provisionDuration.mean`, `.p50`, and `.p95`
- Independent source behavior: gateway list failure does not block provision time and vice versa (GPT-06)

The operational dashboard package SHALL include unit tests or Storybook fixtures for the three-row system-summary presentation when `provisionDuration` is present.

#### Scenario: CI exercises adapter mapping

- GIVEN a mocked BFF response with `mean_seconds: 315`, `p50_seconds: 288`, `p95_seconds: 726`, and `observation_count: 2`
- WHEN dashboard adapter unit tests run
- THEN they SHALL assert the `provision-time` metric `id`, `value: "5.25"`, `unit: "minutes"`, and `provisionDuration` with `p50: "4.80"` and `p95: "12.10"`

## Non-Goals

- Per-gateway provision duration in the UI or API
- Historical trend / sparkline for provision time
- RBAC-scoped provision duration derived from the gateway list
- Gateway-list timestamp proxy (`updated_at - created_at`) as a data source
- Including `Degraded` gateways as provision samples
- Persisting a dedicated `provisioned_at` timestamp on the Gateway resource
- Cluster infrastructure metrics (see cluster memory/CPU/pods/nodes specs)
- P99, min, max, or arbitrary custom quantiles beyond mean, P50, and P95 in version 2

## Supersedes

Version 1 of this spec (gateway-list mean from `updated_at - created_at`, GPT-01 through GPT-04 and GPT-06 list-consistency requirements) is superseded by version 2. Implementation that still uses the gateway-list proxy SHALL be treated as drift until GPT-W2 lands.

## Primary Basis

- `platform/control-plane-observability.spec.md` (CP-OBS-07)
- `web-console/operational-dashboard.spec.md` (OP-DASH-08, OP-DASH-13, OP-DASH-19)
- `platform/cluster-memory.spec.md` (BFF Prometheus proxy pattern)
- `platform/gateway-metrics-dashboard.spec.md` (`PROMETHEUS_URL` configuration)
