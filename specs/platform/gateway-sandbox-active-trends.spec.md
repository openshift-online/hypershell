# Gateway Active Sandbox Trends

**Status:** Active
**Applies to:** `deploy/base/prometheus/`, `components/web-console` BFF and dashboard adapter, `packages/operational-dashboard-ui`
**Tracks:** [HYPERSHELL-281](https://redhat.atlassian.net/browse/HYPERSHELL-281) (active sandboxes trend scope)

## Purpose

Add **dual-window usage trends** for fleet-wide **active sandbox counts** on the operational dashboard. Sandboxes are short-lived compared with provisioned gateways, so operators need both:

1. **Hourly** samples for the **last 24 hours** (recent bursts and session churn)
2. **Daily** samples for the **last 7 days** (recent week-level load patterns; aligns with default Prometheus retention)

Each series plots the fleet-wide active sandbox total from Prometheus TSDB samples of `hypershell_gateways_active_sandboxes_total` (`platform/openshell-gateway-sandbox-count.spec.md`). The count is the sum of each gateway's advisory `active_sandbox_count` at the sample timestamp, not a cumulative create/delete counter.

The standalone `provisioned-sandboxes` grid widget remains retired (OP-DASH-25). Trends render in the **`usage-summary` sandboxes row** only. There is no separate sandbox sparkline widget in version 1.

This specification extends the connected BFF route and adapter mapping defined in `web-console/operational-dashboard.spec.md` OP-DASH-06.

### Relationship to other trend specs

| Concern | This spec | Related spec |
| --- | --- | --- |
| Metric ID | `provisioned-sandboxes` | Same instant metric as OP-DASH-06 |
| Prometheus gauge | `hypershell_gateways_active_sandboxes_total` | `openshell-gateway-sandbox-count.spec.md` |
| 7-day daily retention | Uses default Prometheus TSDB retention (`7d` in `deploy/base/prometheus/prometheus.yaml`; see `platform/gateway-metrics-dashboard.spec.md` DASH-03) | Same default retention as gateway fleet totals and hub cluster utilization trends (`platform/gateway-fleet-total-trend.spec.md` GFT-01, `platform/hub-cluster-utilization-trends.spec.md` HCUT-01) |
| Hourly 24h pattern | Reuses rolling window and hour labels | `platform/gateway-provision-outcomes.spec.md` hourly range |
| Gateway fleet total sparkline | Unrelated | `platform/gateway-fleet-total-trend.spec.md` |

## Requirements

### Requirement: GSAT-01 -- Prometheus Retention Prerequisite

Daily and hourly active-sandbox trends SHALL require only the **default Prometheus TSDB retention** configured in `deploy/base/prometheus/prometheus.yaml` (`7d` today; see `platform/gateway-metrics-dashboard.spec.md` DASH-03), matching gateway fleet totals and hub cluster utilization trends (GFT-01, HCUT-01).

Days before `hypershell_gateways_active_sandboxes_total` instrumentation existed SHALL appear as `count: 0` in the daily series after rollout. Hours before instrumentation existed SHALL appear as `count: 0` in the hourly series after rollout.

#### Scenario: Seven-day daily series is queryable

- GIVEN `hypershell_gateways_active_sandboxes_total` has been scraped continuously for at least 7 UTC calendar days
- AND Prometheus TSDB retention is at least `7d`
- WHEN the BFF evaluates the daily active-sandbox range query (GSAT-05)
- THEN Prometheus SHALL return enough samples to populate 7 daily trend points

---

### Requirement: GSAT-02 -- Gauge Snapshot Semantics

Each trend point SHALL represent the **fleet active sandbox total at the range sample timestamp**, not a max, min, average, or increase over the bucket window.

| Series | Unit | Meaning at each point |
| --- | --- | --- |
| Hourly (`hourly_active_sandboxes`) | sandboxes (non-negative integers) | `hypershell_gateways_active_sandboxes_total` at the end of each hourly bucket |
| Daily (`daily_active_sandboxes`) | sandboxes (non-negative integers) | `hypershell_gateways_active_sandboxes_total` at the end of each daily bucket |

Trend points SHALL NOT break down counts per gateway, namespace, or sandbox phase.

Daily values SHALL use the same display rounding as the instant `active_sandboxes` field (non-negative integer).

#### Scenario: Hourly point reflects fleet total not session creates

- GIVEN Prometheus returns `hypershell_gateways_active_sandboxes_total = 12` at an hourly sample timestamp
- WHEN the BFF maps that sample into `hourly_active_sandboxes`
- THEN the corresponding entry SHALL have `count: 12`
- AND it SHALL NOT represent the number of sandboxes created during that hour

---

### Requirement: GSAT-03 -- Extended BFF JSON on Gateway Sandboxes Route

The web-console BFF SHALL extend `GET /api/metrics/gateway-sandboxes` with optional historical fields while preserving the existing instant field unchanged:

| Field | Type | Required | Meaning |
| --- | --- | --- | --- |
| `active_sandboxes` | number | Yes | Existing fleet total (unchanged) |
| `hourly_active_sandboxes` | array of `{ hour, count }` | No | Rolling last 24 hours; `hour` is UTC `YYYY-MM-DDTHH:00`; `count` is the fleet active sandbox total at that hour bucket |
| `daily_active_sandboxes` | array of `{ date, count }` | No | Last 7 UTC calendar days inclusive of today; `date` is `YYYY-MM-DD`; `count` is the fleet active sandbox total at that daily bucket |

Both arrays SHALL be ordered oldest to newest.

The daily array SHALL contain exactly 7 entries (one per UTC calendar day in the lookback window), filling calendar days without TSDB samples with `count: 0`.

The hourly array SHALL contain one entry per hourly bucket returned by the range evaluation for the rolling 24-hour window. The BFF SHALL omit hours with invalid samples rather than synthesizing counts.

When instant query succeeds but one or both range queries fail, the BFF SHALL respond with HTTP `200`, the current `active_sandboxes` value, and SHALL omit only the failed historical field(s). A successful hourly range SHALL NOT be omitted because the daily range failed, and vice versa.

When the instant query fails, the BFF SHALL continue to respond with HTTP `502` as today. The BFF SHALL NOT return trend-only JSON without `active_sandboxes`.

Dashboard-operator authorization and OIDC behavior SHALL remain as defined in OP-DASH-06.

#### Scenario: Successful response includes instant count and both trend arrays

- GIVEN Prometheus returns a successful instant sandbox gauge and successful hourly and daily range evaluations
- WHEN an authorized caller sends `GET /api/metrics/gateway-sandboxes`
- THEN the BFF SHALL respond with HTTP `200`
- AND the body SHALL include `active_sandboxes`
- AND `hourly_active_sandboxes` SHALL contain hourly `{ hour, count }` entries
- AND `daily_active_sandboxes` SHALL contain 7 `{ date, count }` entries

#### Scenario: Daily range failure returns hourly trend only

- GIVEN Prometheus returns a successful instant sandbox gauge and a successful hourly range evaluation
- AND the daily range query fails or times out
- WHEN an authorized caller sends `GET /api/metrics/gateway-sandboxes`
- THEN the BFF SHALL respond with HTTP `200` with `active_sandboxes` and `hourly_active_sandboxes`
- AND `daily_active_sandboxes` SHALL be absent

---

### Requirement: GSAT-04 -- Hourly Range Query

The BFF SHALL query Prometheus `GET {PROMETHEUS_URL}/api/v1/query_range` (via `fetchMetricsRange` in `metrics-source.ts`) to build `hourly_active_sandboxes`.

| Parameter | Value |
| --- | --- |
| PromQL | `hypershell_gateways_active_sandboxes_total` (with the same optional namespace selector as the instant query when configured) |
| Lookback | Rolling last 24 hours ending at current time (UTC) |
| Step | `3600s` (one sample per hour) |
| End timestamp | Current time (UTC), Unix seconds |
| Start timestamp | End minus `86400` seconds |

Each range sample SHALL map to an `hour` label (`YYYY-MM-DDTHH:00` UTC) derived from the sample timestamp using the same formatting convention as `formatHourLabel` in `metrics-gateway-provision-outcomes.ts`. The sample value SHALL be rounded to a non-negative integer before inclusion in `hourly_active_sandboxes`.

The BFF SHALL export the PromQL string, step, and lookback constants from `components/web-console/bff/src/metrics-gateway-sandboxes.ts` so unit tests assert the documented query contract.

#### Scenario: Hourly points use UTC hour labels

- GIVEN a range sample at Unix time corresponding to UTC `2026-09-15T14:37:00`
- WHEN the BFF maps the sample into `hourly_active_sandboxes`
- THEN the entry SHALL use `hour: "2026-09-15T14:00"`

---

### Requirement: GSAT-05 -- Daily Range Query

The BFF SHALL query Prometheus `query_range` to build `daily_active_sandboxes`.

| Parameter | Value |
| --- | --- |
| PromQL | `hypershell_gateways_active_sandboxes_total` (with the same optional namespace selector as the instant query when configured) |
| Lookback | 7 UTC calendar days inclusive of today |
| Step | `86400s` (one sample per day) |
| End timestamp | Current time (UTC) |
| Start timestamp | Start of the UTC calendar day 6 days before today |

Each range sample SHALL map to a UTC calendar `date` label (`YYYY-MM-DD`) derived from the sample timestamp. The sample value SHALL be rounded to a non-negative integer before inclusion in `daily_active_sandboxes`.

After mapping range results, the BFF SHALL align output to the full 7-day UTC calendar grid (oldest to newest) and set `count: 0` for any calendar day missing from the range response.

The BFF SHALL export the PromQL string and step constants from `components/web-console/bff/src/metrics-gateway-sandboxes.ts` so unit tests assert the documented query contract.

#### Scenario: Daily points cover the last 7 UTC days

- GIVEN today is UTC `2026-09-15`
- AND Prometheus range evaluation succeeds
- WHEN the BFF builds `daily_active_sandboxes`
- THEN the first entry SHALL have `date: "2026-09-09"`
- AND the last entry SHALL have `date: "2026-09-15"`
- AND the array length SHALL be `7`

---

### Requirement: GSAT-06 -- Adapter Trend Mapping

The host `DashboardControlPlane` adapter SHALL map optional historical fields from `GET /api/metrics/gateway-sandboxes` onto the existing `provisioned-sandboxes` metric:

| BFF field | OperationalMetric field | Mapping |
| --- | --- | --- |
| `hourly_active_sandboxes[].hour` | `hourlyTrend.points[].label` | |
| `hourly_active_sandboxes[].count` | `hourlyTrend.points[].value` | |
| `daily_active_sandboxes[].date` | `trend.points[].label` | |
| `daily_active_sandboxes[].count` | `trend.points[].value` | |

The adapter SHALL introduce optional `OperationalMetric.hourlyTrend` for the 24-hour series. The existing `OperationalMetric.trend` field SHALL carry the 7-day daily series.

When a BFF historical field is absent, the adapter SHALL omit the corresponding trend field only. The adapter SHALL still emit `provisioned-sandboxes` with current instant `value` unchanged.

The adapter SHALL NOT synthesize trend points locally and SHALL NOT fail the `gateway-metrics` source solely because trend data is missing.

#### Scenario: Adapter maps hourly and daily series

- GIVEN the BFF returns `hourly_active_sandboxes: [{ "hour": "2026-09-15T12:00", "count": 8 }]` and `daily_active_sandboxes: [{ "date": "2026-09-01", "count": 5 }]`
- WHEN the adapter builds `provisioned-sandboxes`
- THEN `hourlyTrend.points` SHALL equal `[{ label: "2026-09-15T12:00", value: 8 }]`
- AND `trend.points` SHALL equal `[{ label: "2026-09-01", value: 5 }]`
- AND `value` SHALL still reflect `active_sandboxes`

#### Scenario: Missing daily series omits trend only

- GIVEN the BFF returns `active_sandboxes: 4` and `hourly_active_sandboxes` without `daily_active_sandboxes`
- WHEN the adapter builds `provisioned-sandboxes`
- THEN `value` SHALL be `"4"`
- AND `hourlyTrend` SHALL be present
- AND `trend` SHALL be absent

---

### Requirement: GSAT-07 -- Usage Summary Sandboxes Presentation

The `usage-summary` **Sandboxes** row SHALL continue to show the current fleet total from `provisioned-sandboxes.value` (OP-DASH-06, OP-DASH-13).

When `hourlyTrend.points` contains at least two entries, the row SHALL render a `TrendSparklineChart` below the count with:

- Sparkline tooltip metric title: localized **Active sandboxes** (reuse or add message ID aligned with `app.dashboard.summary.sandboxes`)
- Caption: **Last 24 hours**

When `trend.points` contains at least two entries, the row SHALL render a second `TrendSparklineChart` below the hourly sparkline (when present) or below the count with:

- Same tooltip metric title
- Caption: **Last 7 days**

When a trend field is absent or has fewer than two points, that sparkline SHALL be omitted with no error state.

**Trend arrow:** When `hourlyTrend.points` contains at least two entries, the sandboxes row SHALL evaluate `getMetricTrendChange` against `hourlyTrend` (5% threshold between first and last hourly points) and MAY show the localized increase/decrease indicator (OP-DASH-13). The arrow SHALL reflect **fleet active sandbox total** change over the **24-hour** window.

When `hourlyTrend` is absent or has fewer than two points but `trend.points` has at least two entries, the row MAY evaluate `getMetricTrendChange` against `trend` instead (7-day window).

When neither trend field qualifies, the row SHALL show the total only without a trend arrow.

If the default layout height clips the sandboxes sparklines after this change, the operational dashboard layout template and persistence key SHALL be bumped so the `usage-summary` tile is tall enough to show the row, optional arrow, and both sparklines (same pattern as registered-users and gateway-status height adjustments in OP-DASH-11).

The retired standalone `provisioned-sandboxes` grid widget SHALL NOT be reintroduced.

#### Scenario: Sandboxes row shows count, hourly sparkline, and daily sparkline

- GIVEN `provisioned-sandboxes` has `value: "12"`
- AND `hourlyTrend.points` contains 24 hourly entries
- AND `trend.points` contains 7 daily entries
- WHEN the usage summary sandboxes row renders
- THEN the row SHALL show `12`
- AND an hourly sparkline SHALL appear with caption **Last 24 hours**
- AND a daily sparkline SHALL appear below it with caption **Last 7 days**

#### Scenario: Hourly trend drives the summary arrow

- GIVEN `provisioned-sandboxes` has `value: "12"`
- AND `hourlyTrend.points` where the last hour is at least 5% higher than the first hour
- WHEN the usage summary sandboxes row renders
- THEN an increase trend indicator SHALL appear beside the total

---

### Requirement: GSAT-08 -- Partial Failure and Refresh Behavior

Trend data SHALL be optional within the `gateway-metrics` metric source (OP-DASH-19). A failure to load `hourly_active_sandboxes` or `daily_active_sandboxes` SHALL NOT omit instant `provisioned-sandboxes` when the instant query succeeds.

A sandbox trend range failure SHALL NOT omit `provisioned-gateways`, `provision-time`, or `provision-reliability` when those routes succeed.

The dashboard SHALL NOT enter total initial-load error state solely because sandbox trend data is missing while instant sandbox and gateway metrics loaded successfully.

Trend data SHALL refresh on the same operational dashboard cadence as other metrics (`operationalDashboardRefreshMilliseconds`, 15 minutes).

#### Scenario: Hourly trend unavailable does not hide the sandboxes total

- GIVEN `GET /api/metrics/gateway-sandboxes` returns `{ "active_sandboxes": 3 }` without historical fields
- AND other gateway-metrics routes succeed
- WHEN an authorized operator opens `/dashboard`
- THEN the usage summary sandboxes row SHALL show `3`
- AND sparklines SHALL be omitted
- AND the dashboard SHALL NOT show the total initial-load danger alert

---

### Requirement: GSAT-09 -- Verification

The web-console BFF SHALL include unit tests for `GET /api/metrics/gateway-sandboxes` covering:

- Successful mapping from mocked Prometheus range responses into `hourly_active_sandboxes` and `daily_active_sandboxes`
- HTTP `200` with instant count only when one or both range queries fail but the instant query succeeds
- Documented PromQL, step, and lookback parameters (GSAT-04, GSAT-05)

The web console SHALL include unit tests for the dashboard adapter mapping historical fields into `provisioned-sandboxes.hourlyTrend` and `provisioned-sandboxes.trend`, including full, partial, and trend-absent payloads.

The operational dashboard package SHALL update `mockOperationalDashboardMetrics` with representative `hourlyTrend` and `trend` points on `provisioned-sandboxes` for Storybook and SHALL add or extend Storybook states showing the usage summary with both sparklines visible.

`packages/operational-dashboard-ui/DATA_SOURCES.md` SHALL document both range-query PromQL expressions and note that instant and historical series share the same fleet-wide gauge semantics.

#### Scenario: CI exercises sandbox adapter hourly trend mapping

- GIVEN a mocked BFF gateway-sandboxes response with known `hourly_active_sandboxes`
- WHEN dashboard adapter unit tests run
- THEN they SHALL assert `provisioned-sandboxes.hourlyTrend.points` labels and values
- AND assert `hourlyTrend` is omitted when `hourly_active_sandboxes` is absent

## Non-Goals

- Per-gateway or per-namespace sandbox trend series on the operational dashboard
- Reintroducing the standalone `provisioned-sandboxes` grid widget
- Sparklines on `GatewayMetricsDashboard` at `/metrics`
- Max, min, average, or session-create-rate aggregates over each bucket
- A HyperShell REST aggregate, control-plane watch replay, or snapshot job when Prometheus range data over the existing gauge is sufficient
- Direct browser access to Prometheus
- Combining sandbox trends into the gateways BFF route (`GET /api/metrics/gateways`)
