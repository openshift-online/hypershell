# Gateway Provision Outcomes

**Status:** Active
**Applies to:** `deploy/base` and `deploy/kind` control-plane metrics pipeline and Prometheus configuration, `components/web-console` BFF and dashboard adapter, `packages/operational-dashboard-ui`, `components/control-plane` (`gateway.provision.outcomes` counter per `platform/control-plane-observability.spec.md` CP-OBS-07)

## Purpose

Expose **gateway provision success and failure counts** and a rolling **24-hour success rate** on the operational dashboard so dashboard operators can assess provision reliability alongside provision duration.

This is **HyperShell application telemetry** recorded by the control plane when a gateway reaches its first terminal provision outcome (`Running` success or `Failed` failure). It is **not** cluster infrastructure metrics, per-gateway SLA tracking in the UI, or control-plane reconcile error counts in isolation.

Version 1 aggregates fleet-wide counts from the control-plane counter over a rolling 24-hour window:

| Statistic | Meaning |
| --- | --- |
| **Success count (24h)** | Number of gateway provisions that reached `Running` for the first time in the last 24 hours |
| **Failure count (24h)** | Number of gateway provisions that reached `Failed` for the first time in the last 24 hours |
| **Success rate (24h)** | `successes / (successes + failures) * 100`, rounded to one decimal place |
| **Hourly success rate** | Per-hour success rate over the same 24-hour window for sparkline display |

The operational dashboard `system-summary` card SHALL render a success-rate row (OP-DASH-13). The default layout SHALL include a `provision-reliability` widget (OP-DASH-10, OP-DASH-24).

### Relationship to other specifications

- **Control-plane observability** (`platform/control-plane-observability.spec.md` CP-OBS-07) owns recording `gateway.provision.outcomes` as an OTLP counter with `outcome` attribute values `success` and `failure`.
- **Gateway provision time** (`platform/gateway-provision-time.spec.md`) owns the companion `gateway.provision.duration` histogram. Success outcomes SHALL be recorded together with duration observations on the same one-per-gateway claim.
- **Operational dashboard** (`web-console/operational-dashboard.spec.md`) owns the `provision-reliability` widget, system-summary success-rate row, refresh policy (OP-DASH-09), independent metric loading within the `gateway-metrics` source (OP-DASH-19, OP-DASH-23), and dashboard-operator access (OP-DASH-04).
- **Cluster memory/CPU/pods/nodes** specs follow the same BFF Prometheus proxy pattern used here.

### Scope note: fleet-wide vs RBAC-filtered

Outcome counts reflect every terminal provision the control plane has observed across the platform. They are **not** filtered to gateways visible in the caller's paginated gateway list. This matches provision duration (GPT scope note) and other hub-cluster infrastructure metrics.

Per-gateway outcome breakdown, arbitrary lookback windows beyond 24 hours, and gateway-list-derived success proxies are out of scope (see Non-Goals).

## Requirements

### Requirement: GPO-00 -- Prometheus Counter Availability

The Prometheus instance configured for the web-console BFF (`PROMETHEUS_URL`, same origin as cluster metrics) SHALL expose the control-plane provision-outcomes counter in Prometheus exposition format.

The canonical series SHALL be `gateway_provision_outcomes_total` with an `outcome` label (`success` or `failure`), derived from the OTLP metric `gateway.provision.outcomes` (CP-OBS-07).

Deploy and local-development configuration SHALL route control-plane OTLP metrics into that Prometheus instance. When the counter is absent and no qualifying fallback exists (GPO-04), provision-reliability collection SHALL return zero counts with `success_rate_percent: null` per GPO-07 rather than synthesizing a non-null success rate.

`DATA_SOURCES.md` SHALL document the canonical PromQL expressions.

#### Scenario: Counter present after gateway provisions

- GIVEN the control plane has recorded at least one `gateway.provision.outcomes` sample
- AND the metrics pipeline exposes `gateway_provision_outcomes_total`
- WHEN the BFF evaluates provision-outcomes PromQL
- THEN 24-hour success and failure counts SHALL be computable from the counter series

#### Scenario: Zero completed provisions in the window

- GIVEN both 24-hour outcome increases and the GPO-04 fallback resolve to zero total provisions
- WHEN a dashboard operator loads operational metrics
- THEN the BFF SHALL respond with HTTP `200` and `success_rate_percent: null`
- AND the adapter SHALL omit the `provision-reliability` metric (GPO-07)

---

### Requirement: GPO-01 -- Control-Plane Outcome Recording Contract

Provision-outcome samples SHALL follow CP-OBS-07 and share the same one-per-gateway in-process claim used by `gateway.provision.duration`:

- One `outcome=success` increment on the **first** successful transition to `Running`, recorded together with the duration observation
- One `outcome=failure` increment on the **first** transition to `Failed`
- No Gateway identifier label on the metric
- Recoveries from `Degraded` to `Running` SHALL NOT produce additional success observations
- Gateways that already reached `Running` or `Degraded` before the current reconcile SHALL NOT produce new provision observations

Success and failure observations for the same gateway SHALL be mutually exclusive through the shared claim.

When exported to Prometheus, the counter SHALL appear as `gateway_provision_outcomes_total{outcome="success|failure"}`.

#### Scenario: First Running records success

- GIVEN a Gateway has not yet contributed a provision observation
- WHEN the control plane successfully changes its phase to `Running` for the first time
- THEN `gateway.provision.outcomes` SHALL increment once with `outcome=success`
- AND `gateway.provision.duration` SHALL record the provision duration for the same gateway

#### Scenario: First Failed records failure

- GIVEN a Gateway has not yet contributed a provision observation
- WHEN the control plane successfully changes its phase to `Failed` for the first time
- THEN `gateway.provision.outcomes` SHALL increment once with `outcome=failure`
- AND `gateway.provision.duration` SHALL NOT record a duration observation for that gateway

#### Scenario: Degraded recovery does not add outcomes

- GIVEN a gateway already contributed one `outcome=success` observation when it first reached `Running`
- WHEN that gateway later recovers from `Degraded` to `Running`
- THEN the outcomes counter SHALL NOT increment again

---

### Requirement: GPO-02 -- 24-Hour Measurement Contract

The platform SHALL compute fleet-wide provision outcome statistics from Prometheus at evaluation time:

| Field | PromQL (canonical) |
| --- | --- |
| `success_count_24h` | `sum(increase(gateway_provision_outcomes_total{outcome="success"}[24h]))` |
| `failure_count_24h` | `sum(increase(gateway_provision_outcomes_total{outcome="failure"}[24h]))` |
| Hourly success increase | `sum(increase(gateway_provision_outcomes_total{outcome="success"}[1h]))` over a 24-hour range with 1-hour step |
| Hourly failure increase | `sum(increase(gateway_provision_outcomes_total{outcome="failure"}[1h]))` over the same range and step |

All instant queries SHALL use the same evaluation timestamp. Range queries SHALL cover the trailing 24 hours ending at that timestamp with a 1-hour step.

`success_rate_percent` SHALL be `round((success_count_24h / (success_count_24h + failure_count_24h)) * 1000) / 10` when the denominator is greater than zero. When the denominator is zero, `success_rate_percent` SHALL be `null`.

Each `hourly_success_rate` entry SHALL include:

| Field | Meaning |
| --- | --- |
| `hour` | UTC hour label formatted as `YYYY-MM-DDTHH:00` |
| `success_count` | Rounded hourly success increase for that bucket |
| `failure_count` | Rounded hourly failure increase for that bucket |
| `success_rate_percent` | Hourly success rate using the same rounding rule |

Hours where `success_count + failure_count` is zero SHALL be omitted from `hourly_success_rate`.

#### Scenario: Nine successes and one failure yield ninety percent

- GIVEN Prometheus returns `success_count_24h = 9` and `failure_count_24h = 1`
- WHEN the BFF formats the response
- THEN `success_rate_percent` SHALL be `90.0`

---

### Requirement: GPO-03 -- BFF Gateway Provision Outcomes Route

The web-console BFF SHALL expose `GET /api/metrics/gateway-provision-outcomes` as a same-origin route that:

1. Requires an authenticated session when OIDC is enabled (same session gate as `GET /api/metrics/cluster-memory`)
2. Executes the PromQL expressions in GPO-02 (instant queries for 24-hour totals; range queries for hourly buckets)
3. Returns JSON:

```json
{
  "success_count_24h": 9,
  "failure_count_24h": 1,
  "success_rate_percent": 90.0,
  "hourly_success_rate": [
    {
      "hour": "2026-08-09T12:00",
      "success_count": 4,
      "failure_count": 1,
      "success_rate_percent": 80.0
    }
  ]
}
```

When no provisions completed in the window and GPO-04 does not apply, the route SHALL respond with HTTP `200` and:

```json
{
  "success_count_24h": 0,
  "failure_count_24h": 0,
  "success_rate_percent": null,
  "hourly_success_rate": []
}
```

The route SHALL NOT forward to the HyperShell API server and SHALL NOT require a HyperShell API bearer token.

On Prometheus failure or timeout, the BFF SHALL respond with HTTP `502` and `{ "error": "Metrics unavailable", "statusCode": 502 }`. The BFF SHALL NOT return a non-null `success_rate_percent` as a fallback when Prometheus is unavailable.

#### Scenario: Authenticated dashboard operator receives outcomes

- GIVEN OIDC is enabled and the caller has a valid session
- AND Prometheus returns successful query results with at least one completed provision in the window
- WHEN the caller sends `GET /api/metrics/gateway-provision-outcomes`
- THEN the BFF SHALL respond with HTTP `200` and the GPO-03 JSON body

#### Scenario: Unauthenticated caller is rejected

- GIVEN OIDC is enabled and the caller has no session
- WHEN the caller sends `GET /api/metrics/gateway-provision-outcomes`
- THEN the BFF SHALL respond with HTTP `401` or redirect to login per existing metrics-route policy

---

### Requirement: GPO-04 -- Duration Histogram Fallback (Migration)

When both 24-hour outcome increases round to zero, the BFF MAY fall back to the provision-duration histogram to surface historical success counts recorded before the outcomes counter existed:

| Fallback step | PromQL |
| --- | --- |
| 24-hour success proxy | `sum(increase(gateway_provision_duration_seconds_count[24h]))` |
| Lifetime success proxy | `sum(gateway_provision_duration_seconds_count)` when the 24-hour proxy is also zero |

When a fallback yields a positive success count, the BFF SHALL:

- Set `success_count_24h` to the fallback value
- Set `failure_count_24h` to `0`
- Compute `success_rate_percent` as `100.0` when only successes are known
- Build `hourly_success_rate` from `sum(increase(gateway_provision_duration_seconds_count[1h]))` over the same 24-hour range when hourly samples exist

The fallback SHALL NOT invent failure counts. It SHALL NOT run when the outcomes counter already reports a non-zero 24-hour total.

#### Scenario: Outcomes counter empty but duration histogram has history

- GIVEN both 24-hour outcome increases are zero
- AND `gateway_provision_duration_seconds_count` is greater than zero
- WHEN the BFF evaluates `GET /api/metrics/gateway-provision-outcomes`
- THEN `success_count_24h` SHALL reflect the fallback success count
- AND `failure_count_24h` SHALL be `0`
- AND `success_rate_percent` SHALL be `100.0`

---

### Requirement: GPO-05 -- Operational Dashboard Metric Mapping

The operational dashboard host adapter (`createDashboardControlPlaneAdapter`) SHALL fetch `GET /api/metrics/gateway-provision-outcomes` within the `gateway-metrics` source (OP-DASH-23) and emit a `provision-reliability` `OperationalMetric` when `success_rate_percent` is non-null.

The `OperationalMetric` type SHALL include an optional `provisionOutcomes` object:

```typescript
interface OperationalMetricProvisionOutcomes {
  successCount24h: string;
  failureCount24h: string;
  successRatePercent: string;
}
```

The adapter SHALL map the BFF response as:

| Field | Value |
| --- | --- |
| `id` | `"provision-reliability"` |
| `value` | Decimal string of `success_rate_percent`, rounded to **one** fractional digit |
| `provisionOutcomes.successCount24h` | Stringified `success_count_24h` |
| `provisionOutcomes.failureCount24h` | Stringified `failure_count_24h` |
| `provisionOutcomes.successRatePercent` | Same string as `value` |
| `successRateTrend` | Optional; present only when `hourly_success_rate` yields at least two points |

Each `successRateTrend.points` entry SHALL use the hourly `hour` label as `label` and `success_rate_percent` as `value`.

When the BFF route fails or `success_rate_percent` is `null`, the adapter SHALL omit `provision-reliability` while other metrics from the same `gateway-metrics` fetch MAY still be emitted (GPO-06).

#### Scenario: Adapter maps outcomes into provision reliability

- GIVEN the BFF returns `success_count_24h: 9`, `failure_count_24h: 1`, `success_rate_percent: 90.0`, and two hourly points
- WHEN the adapter maps the response
- THEN the `provision-reliability` metric SHALL have `value: "90.0"`
- AND `provisionOutcomes` SHALL include `successCount24h: "9"`, `failureCount24h: "1"`, and `successRatePercent: "90.0"`
- AND `successRateTrend.points` SHALL contain two entries with the hourly labels and success-rate values

---

### Requirement: GPO-06 -- Decoupling from Gateway List and Provision Duration

Provision-outcomes collection SHALL NOT depend on the paginated gateway list (`GET /api/hypershell/v1/gateways`).

Within the `gateway-metrics` source, `provision-reliability` SHALL load independently from `provision-time`:

- Provision-outcomes BFF failure or `success_rate_percent: null` SHALL omit only `provision-reliability`
- Provision-duration BFF failure SHALL omit only `provision-time`
- Either omission SHALL NOT prevent the other metric from being emitted when its route succeeds

Prometheus gateway-count or gateway-sandbox failure SHALL fail the entire `gateway-metrics` source and omit `provisioned-gateways`, `provisioned-sandboxes`, `provision-time`, and `provision-reliability`, even when the provision routes would have succeeded.

#### Scenario: Outcomes unavailable does not hide provision duration

- GIVEN `GET /api/metrics/gateway-provision-duration` succeeds
- AND `GET /api/metrics/gateway-provision-outcomes` fails
- WHEN `getOperationalMetrics` runs
- THEN `provision-time` SHALL still be emitted
- AND `provision-reliability` SHALL be omitted

#### Scenario: Duration unavailable does not hide provision reliability

- GIVEN `GET /api/metrics/gateway-provision-outcomes` succeeds with a non-null success rate
- AND `GET /api/metrics/gateway-provision-duration` fails
- WHEN `getOperationalMetrics` runs
- THEN `provision-reliability` SHALL still be emitted
- AND `provision-time` SHALL be omitted

---

### Requirement: GPO-07 -- Refresh and Error Semantics

Provision reliability SHALL load through the existing operational dashboard metrics query (`useGetMetricsData`) and SHALL inherit its refresh policy (`operationalDashboardRefreshMilliseconds`, currently 15 minutes) and manual refresh behavior (OP-DASH-09).

When provision-outcomes collection fails (BFF `502`, adapter validation error, or `success_rate_percent: null`), the adapter SHALL omit only the `provision-reliability` metric. Other metric sources SHALL still contribute when they succeed (OP-DASH-19). The dashboard SHALL NOT display `0%` as a fallback success rate.

#### Scenario: No completed provisions omits provision reliability only

- GIVEN every other metric source succeeds
- AND the outcomes route returns `success_rate_percent: null`
- WHEN the operator opens `/dashboard`
- THEN cluster and gateway-count metrics SHALL still load
- AND the provision-reliability widget and system-summary success-rate row SHALL render the localized metric-unavailable state
- AND the dashboard SHALL NOT enter the total load-error state

---

### Requirement: GPO-08 -- Verification

The web console SHALL include unit tests for:

- BFF route PromQL mapping and JSON response formatting (including hourly buckets and GPO-04 fallback)
- BFF `502` on Prometheus errors
- Adapter mapping from BFF JSON to `provision-reliability` with `provisionOutcomes` and `successRateTrend`
- Independent loading: provision-duration failure does not block provision reliability and vice versa (GPO-06)

The operational dashboard package SHALL include unit tests or Storybook fixtures for the `provision-reliability` widget and the system-summary success-rate row.

#### Scenario: CI exercises adapter mapping

- GIVEN a mocked BFF response with `success_count_24h: 9`, `failure_count_24h: 1`, `success_rate_percent: 90.0`, and two hourly points
- WHEN dashboard adapter unit tests run
- THEN they SHALL assert the `provision-reliability` metric `id`, `value: "90.0"`, `provisionOutcomes`, and `successRateTrend`

## Non-Goals

- Per-gateway provision outcome breakdown in the UI or API
- Lookback windows other than the rolling 24-hour window in version 1
- RBAC-scoped success rates derived from the gateway list
- Treating `Degraded` as a terminal provision failure
- Synthesizing failure counts during the GPO-04 duration-histogram fallback
- Cluster infrastructure metrics (see cluster memory/CPU/pods/nodes specs)

## Primary Basis

- `platform/control-plane-observability.spec.md` (CP-OBS-07)
- `platform/gateway-provision-time.spec.md` (companion duration histogram and shared one-per-gateway claim)
- `web-console/operational-dashboard.spec.md` (OP-DASH-08, OP-DASH-13, OP-DASH-19, OP-DASH-24)
- `platform/cluster-memory.spec.md` (BFF Prometheus proxy pattern)
- `platform/gateway-metrics-dashboard.spec.md` (`PROMETHEUS_URL` configuration)
