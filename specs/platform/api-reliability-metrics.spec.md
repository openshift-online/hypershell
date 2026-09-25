# API Reliability Metrics

**Status:** Draft
**Applies to:** API-server Prometheus scrape (`api_inbound_request_duration` via ServiceMonitor), `components/web-console` BFF and dashboard adapter, `packages/operational-dashboard-ui`
**Tracks:** [HYPERSHELL-285](https://redhat.atlassian.net/browse/HYPERSHELL-285) (API request rate, error rate, and latency only)

## Purpose

Expose **API server HTTP reliability metrics** - request rate, 5xx error rate, and median latency - to the reliability dashboard so dashboard operators can assess API health with current values and 24-hour hourly trend sparklines.

These metrics are derived from the API server framework Prometheus histogram `api_inbound_request_duration_*`, scraped into the BFF Prometheus instance with `job="hypershell-api-server"`. When OpenTelemetry `http.server.request.duration` is also exported as `http_server_request_duration_seconds_*` (API-OBS-05), implementations MAY query that series instead; Kind and default ServiceMonitor scrapes today expose `api_inbound_request_duration`.

Authentication failure rate and login failure rate are **out of scope** for this specification (deferred from HYPERSHELL-285).

### Relationship to other specifications

| Concern | Owner |
| --- | --- |
| Preferred scrape series and optional OTel alternative | ARM-00 below; OTel recording in `platform/api-server-observability.spec.md` API-OBS-05 |
| Reliability dashboard widgets, route, admin access, secondary nav | `web-console/reliability-dashboard.spec.md` |
| Operational dashboard (fleet / hub cluster) | `web-console/operational-dashboard.spec.md` - does **not** host these metrics |
| Prometheus TSDB retention | Default `7d` per `platform/gateway-fleet-total-trend.spec.md` GFT-01 (sufficient for a 24-hour lookback) |

### Scope note: fleet-wide API traffic

Metrics cover inbound HTTP traffic to the HyperShell API server (`OTEL_SERVICE_NAME` default `hypershell-api-server`). They are **not** filtered by caller identity or per-gateway RoleBindings. Access control is enforced at the BFF route (dashboard-operator roles per REL-DASH-04 / OP-DASH-04).

## Requirements

### Requirement: ARM-00 -- Prometheus Histogram Availability

The Prometheus instance configured for the web-console BFF (`PROMETHEUS_URL`) SHALL expose an API server HTTP request duration histogram in Prometheus exposition format.

The preferred series for the reliability dashboard SHALL be the framework Prometheus histogram scraped from the API server `/metrics` endpoint:

| Item | Value |
| --- | --- |
| Series prefix | `api_inbound_request_duration` (`_bucket`, `_count`, `_sum`) |
| Job selector | `job="hypershell-api-server"` |
| Status-code label | `code` |

When OpenTelemetry `http.server.request.duration` is also present (as `http_server_request_duration_seconds_*`), implementations MAY query that series instead, but Kind and default ServiceMonitor scrapes today expose `api_inbound_request_duration`. `DATA_SOURCES.md` SHALL document the live series and status-code label.

Deploy and local-development configuration SHALL ensure the API server metrics endpoint is scraped into the BFF Prometheus instance. When the histogram is absent or has zero observations in the evaluation window such that required instant fields cannot be computed, collection SHALL fail per ARM-06 rather than synthesizing values.

#### Scenario: Histogram present after API traffic

- GIVEN the API server has recorded at least one `api_inbound_request_duration` observation
- AND the metrics scrape exposes `api_inbound_request_duration_count` greater than zero
- WHEN the BFF evaluates API reliability PromQL
- THEN request rate, error rate, and median latency SHALL be computable from the histogram series

#### Scenario: Zero observations fails collection

- GIVEN `api_inbound_request_duration_count` is absent or required instant PromQL yields empty or non-finite results without a valid fallback
- WHEN a dashboard operator loads reliability metrics
- THEN API reliability collection SHALL fail
- AND the BFF SHALL respond with HTTP `502` for `GET /api/metrics/api-reliability`

---

### Requirement: ARM-01 -- Instant Measurement Contract

The platform SHALL compute three fleet-wide statistics from the histogram at a single Prometheus instant-evaluation time:

| Field | Unit | Meaning | Canonical PromQL shape |
| --- | --- | --- | --- |
| `request_rate` | requests per second | Aggregate HTTP request rate | `sum(rate(api_inbound_request_duration_count{job="hypershell-api-server"}[5m]))` |
| `error_rate_percent` | percent | Share of requests with HTTP **5xx** status | `100 * (sum(rate(...{code=~"5.."}[5m])) or vector(0)) / clamp_min(sum(rate(...[5m])), 1e-12)` |
| `latency_p50_seconds` | seconds | Median request latency | `histogram_quantile(0.50, sum(rate(..._bucket[5m])) by (le))`, with cumulative histogram fallback when the rate-based quantile is non-finite |

Notes:

- The rate window for instant queries SHALL be **5 minutes**.
- **Error rate SHALL count 5xx responses only.** 4xx responses (including 401/403) SHALL NOT contribute to the numerator.
- When recent request rate is zero (idle Kind / low-traffic fleets), `clamp_min` on the denominator makes `error_rate_percent` evaluate to `0`. That is an intentional idle reading, not a synthetic substitute for a failed PromQL evaluation. Collection SHALL still fail per ARM-06 when the instant query returns empty or non-finite results (for example, the histogram series is absent).
- Latency SHALL be median (P50) in **seconds**. The rate-based quantile MAY fall back to the cumulative histogram when the rate window has no observations (instant and hourly series).
- All three expressions SHALL use the same evaluation timestamp.

The BFF SHALL export the PromQL strings, rate window, service selector, and status-label constants from `components/web-console/bff/src/metrics-api-reliability.ts` so unit tests assert the documented query contract.

#### Scenario: Instant values from known rates

- GIVEN Prometheus returns `request_rate = 12.5`, `error_rate_percent = 1.25`, and `latency_p50_seconds = 0.084`
- WHEN the BFF formats the response
- THEN `request_rate` SHALL be `12.5`
- AND `error_rate_percent` SHALL be `1.25`
- AND `latency_p50_seconds` SHALL be `0.084`

#### Scenario: 4xx does not inflate error rate

- GIVEN only HTTP 401 and 200 responses exist in the rate window
- AND no 5xx responses exist
- WHEN the BFF evaluates `error_rate_percent`
- THEN the numerator SHALL be zero
- AND `error_rate_percent` SHALL be `0` when the denominator is greater than zero

---

### Requirement: ARM-02 -- 24-Hour Hourly Trend Series

The BFF SHALL also evaluate Prometheus `query_range` for each of the three statistics over a rolling **24-hour** window so the reliability dashboard can render hourly sparklines.

| Parameter | Value |
| --- | --- |
| Lookback | Rolling last 24 hours ending at current time (UTC) |
| Step | `3600s` (one sample per hour) |
| End timestamp | Current time (UTC), Unix seconds |
| Start timestamp | End minus `86400` seconds |
| Instant PromQL at each step | Same shapes as ARM-01 (5-minute rate / quantile window inside each sample). For hourly latency, when the rate-based quantile range fails or yields no finite samples, the BFF SHALL fall back to the cumulative histogram range query (same fallback as instant P50). |

Each series SHALL map to an array of `{ hour, value }` entries:

| Field | Type | Meaning |
| --- | --- | --- |
| `hour` | string | UTC hour label `YYYY-MM-DDTHH:00` (same formatting as `formatHourLabel` in provision-outcomes) |
| `value` | number | Series value at that hour in the same unit as the instant field |

| Historical field | Instant field it trends | Unit |
| --- | --- | --- |
| `hourly_request_rate` | `request_rate` | requests per second |
| `hourly_error_rate_percent` | `error_rate_percent` | percent |
| `hourly_latency_p50_seconds` | `latency_p50_seconds` | seconds |

Arrays SHALL be ordered oldest to newest. The BFF SHALL omit hours with invalid or non-finite samples rather than synthesizing zeros for rate/latency series.

When instant queries succeed but one or more hourly range queries fail, the BFF SHALL respond with HTTP `200`, the current instant fields, and SHALL omit only the failed historical field(s). A successful hourly series SHALL NOT be omitted because a sibling hourly series failed.

When instant queries fail, the BFF SHALL respond with HTTP `502` per ARM-06. The BFF SHALL NOT return trend-only JSON without instant fields.

#### Scenario: Successful response includes instant values and hourly series

- GIVEN Prometheus returns successful instant evaluations and successful hourly range evaluations for all three statistics
- WHEN an authorized caller sends `GET /api/metrics/api-reliability`
- THEN the BFF SHALL respond with HTTP `200`
- AND the body SHALL include `request_rate`, `error_rate_percent`, and `latency_p50_seconds`
- AND `hourly_request_rate`, `hourly_error_rate_percent`, and `hourly_latency_p50_seconds` SHALL each contain hourly `{ hour, value }` entries

#### Scenario: Partial range failure keeps instant values

- GIVEN Prometheus returns successful instant evaluations
- AND the hourly request-rate range succeeds
- AND the hourly latency range fails
- WHEN an authorized caller sends `GET /api/metrics/api-reliability`
- THEN the BFF SHALL respond with HTTP `200` with instant fields and `hourly_request_rate`
- AND `hourly_latency_p50_seconds` SHALL be absent

#### Scenario: Hourly points use UTC hour labels

- GIVEN a range sample at Unix time corresponding to UTC `2026-09-15T14:37:00`
- WHEN the BFF maps the sample into an hourly array
- THEN the entry SHALL use `hour: "2026-09-15T14:00"`

---

### Requirement: ARM-03 -- BFF API Reliability Route

The web-console BFF SHALL expose `GET /api/metrics/api-reliability` as a same-origin route that:

1. Requires an authenticated session when OIDC is enabled (same session gate as other `GET /api/metrics/*` routes)
2. Requires dashboard-admin authorization (`platform:admin`) when OIDC is enabled (OP-DASH-04 / REL-DASH-04)
3. Executes the PromQL expressions in ARM-01 and ARM-02
4. Returns JSON:

```json
{
  "request_rate": 12.5,
  "error_rate_percent": 1.25,
  "latency_p50_seconds": 0.084,
  "hourly_request_rate": [
    { "hour": "2026-09-15T12:00", "value": 10.2 }
  ],
  "hourly_error_rate_percent": [
    { "hour": "2026-09-15T12:00", "value": 0.5 }
  ],
  "hourly_latency_p50_seconds": [
    { "hour": "2026-09-15T12:00", "value": 0.09 }
  ]
}
```

Optional historical arrays MAY be omitted independently per ARM-02.

The route SHALL NOT forward to the HyperShell API server and SHALL NOT require a HyperShell API bearer token.

On Prometheus failure or timeout for the instant queries, the BFF SHALL respond with HTTP `502` and `{ "error": "Metrics unavailable", "statusCode": 502 }`.

When OIDC is disabled (no-auth dev mode), the route SHALL remain open to unauthenticated callers, matching other dashboard metrics routes.

#### Scenario: Authenticated dashboard operator receives reliability metrics

- GIVEN OIDC is enabled and the caller has `platform:admin`
- AND Prometheus returns successful query results
- WHEN the caller sends `GET /api/metrics/api-reliability`
- THEN the BFF SHALL respond with HTTP `200` and the ARM-03 JSON body

#### Scenario: Non-admin is rejected

- GIVEN OIDC is enabled and the caller has only `hypershell-users`
- WHEN the caller sends `GET /api/metrics/api-reliability`
- THEN the BFF SHALL respond with HTTP `403`

#### Scenario: Unauthenticated caller is rejected

- GIVEN OIDC is enabled and the caller has no session
- WHEN the caller sends `GET /api/metrics/api-reliability`
- THEN the BFF SHALL respond with HTTP `401` or the standard BFF re-authentication response

---

### Requirement: ARM-04 -- Adapter Metric Mapping

The host dashboard adapter SHALL map `GET /api/metrics/api-reliability` into three `OperationalMetric` values (or the reliability-dashboard equivalent metric type in the same package) for the `api-reliability` metric source:

| Metric ID | `value` | `unit` | Instant source | Trend source |
| --- | --- | --- | --- | --- |
| `api-request-rate` | Decimal string of `request_rate` | `"requests/sec"` | `request_rate` | `hourly_request_rate` → `hourlyTrend.points` |
| `api-error-rate` | Decimal string of `error_rate_percent` | `"%"` | `error_rate_percent` | `hourly_error_rate_percent` → `hourlyTrend.points` |
| `api-latency` | Decimal string of `latency_p50_seconds` | `"sec"` | `latency_p50_seconds` | `hourly_latency_p50_seconds` → `hourlyTrend.points` |

Trend mapping:

| BFF field | Metric field |
| --- | --- |
| `hourly_*[].hour` | `hourlyTrend.points[].label` |
| `hourly_*[].value` | `hourlyTrend.points[].value` (string or number consistent with existing trend point typing) |

Rounding:

| Metric | Display rounding |
| --- | --- |
| `api-request-rate` | Three fractional digits |
| `api-error-rate` | Three fractional digits |
| `api-latency` | Three fractional digits |

When a BFF historical field is absent, the adapter SHALL omit `hourlyTrend` for that metric only and SHALL still emit the metric with the instant `value`.

The three metrics SHALL load as one independent metric source (`api-reliability`). Instant-query failure SHALL omit all three. A missing hourly series SHALL NOT omit sibling metrics when instant values succeeded.

#### Scenario: Adapter maps full payload

- GIVEN the BFF returns the ARM-03 example JSON
- WHEN the adapter builds reliability metrics
- THEN `api-request-rate` SHALL have `value: "12.500"`, `unit: "requests/sec"`, and `hourlyTrend` populated
- AND `api-error-rate` SHALL have `value: "1.250"`, `unit: "%"`
- AND `api-latency` SHALL have `value: "0.084"`, `unit: "sec"`

#### Scenario: Missing hourly latency omits only latency trend

- GIVEN the BFF returns instant fields and `hourly_request_rate` / `hourly_error_rate_percent` without `hourly_latency_p50_seconds`
- WHEN the adapter builds reliability metrics
- THEN `api-latency` SHALL still be emitted with its instant `value`
- AND `api-latency.hourlyTrend` SHALL be absent
- AND `api-request-rate.hourlyTrend` SHALL be present

---

### Requirement: ARM-05 -- Refresh and Independent Loading

API reliability metrics SHALL load through the reliability dashboard metrics query and SHALL inherit its refresh policy (15 minutes and manual refresh per REL-DASH-09 / OP-DASH-09).

The `api-reliability` source SHALL load independently from operational-dashboard metric sources. Failure of `GET /api/metrics/api-reliability` SHALL omit only `api-request-rate`, `api-error-rate`, and `api-latency`. It SHALL NOT omit operational metrics on `/dashboard`.

Conversely, operational metric source failures SHALL NOT omit reliability metrics on `/dashboard/reliability`.

#### Scenario: Reliability source failure is isolated

- GIVEN `GET /api/metrics/api-reliability` returns HTTP `502`
- AND other reliability sources (if any) succeed
- WHEN the reliability dashboard loads
- THEN `api-request-rate`, `api-error-rate`, and `api-latency` SHALL be omitted
- AND the page SHALL follow partial-failure or total-failure rules in `web-console/reliability-dashboard.spec.md`

---

### Requirement: ARM-06 -- Failure Semantics

Instant PromQL failure, timeout, empty result set, or non-finite values for required instant fields SHALL cause the BFF to respond with HTTP `502`.

The adapter SHALL treat a non-success BFF response as failure of the entire `api-reliability` source.

The dashboard SHALL NOT display `0 requests/sec`, `0%`, or `0 sec` as a fallback when the source failed.

#### Scenario: Prometheus timeout yields 502

- GIVEN Prometheus does not respond before the BFF query timeout
- WHEN an authorized caller sends `GET /api/metrics/api-reliability`
- THEN the BFF SHALL respond with HTTP `502`
- AND the body SHALL indicate metrics are unavailable

---

### Requirement: ARM-07 -- Tests and Documentation

The web-console BFF SHALL include unit tests for `GET /api/metrics/api-reliability` covering:

- Successful mapping from mocked Prometheus instant and range responses into the ARM-03 JSON shape
- 5xx-only error-rate numerator (4xx excluded)
- Partial hourly range failure omitting only the failed historical field
- Instant failure returning HTTP `502`
- Authorization gates (401 / 403 / 200) consistent with other metrics routes

The web console SHALL include unit tests for adapter mapping into `api-request-rate`, `api-error-rate`, and `api-latency`, including full, partial-trend, and trend-absent payloads.

`packages/operational-dashboard-ui/DATA_SOURCES.md` SHALL document the `api-reliability` source, BFF route, PromQL shapes, units, and the verified Prometheus status-code label name.

#### Scenario: CI exercises error-rate 5xx filter

- GIVEN mocked Prometheus series that include 401 and 500 status labels
- WHEN BFF unit tests evaluate `error_rate_percent`
- THEN they SHALL assert only the 5xx series contribute to the numerator

---

## Non-Goals

- Authentication failure rate and login failure rate widgets or BFF fields
- Adding these metrics to the operational overview dashboard (`/dashboard`)
- Per-route or per-method breakdown widgets in version 1 (labels MAY remain on the underlying series for future drill-down)
- gRPC `rpc.server.duration` reliability widgets in version 1
- Changing API-server OTel instrumentation beyond what API-OBS-05 already requires
- Bumping Prometheus retention beyond the default `7d`
