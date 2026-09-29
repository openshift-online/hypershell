# Gateway Fleet Total Trend

**Status:** Active
**Applies to:** `deploy/base/prometheus/`, `components/web-console` BFF and dashboard adapter, `packages/operational-dashboard-ui`
**Tracks:** [HYPERSHELL-281](https://redhat.atlassian.net/browse/HYPERSHELL-281) (gateway total sparkline scope only)

## Purpose

Add a **7-day usage trend sparkline** for fleet-wide **provisioned gateway totals** on the operational dashboard. The trend shows how many gateways exist in the platform in total over time, regardless of lifecycle phase or display-status bucket.

The existing `gateway-status` widget already renders a status donut from current phase counts and reserves space below the donut for a `TrendSparklineChart` when `provisioned-gateways.trend` is present (`web-console/operational-dashboard.spec.md` OP-DASH-12). This specification defines the Prometheus range query, BFF JSON extension, adapter mapping, and presentation rules needed to populate that field.

Historical series come from Prometheus TSDB samples of `hypershell_gateways_total` (`platform/gateway-metrics-dashboard.spec.md` DASH-01). The trend is **not** per-status (healthy, provisioning, degraded, failed), **not** healthy-only, and **not** a stacked composition chart.

### Relationship to the operational dashboard

| Concern | This spec | Existing behavior |
| --- | --- | --- |
| Metric ID | `provisioned-gateways` | Same metric that powers the gateway status donut and usage-summary **Gateways** row |
| Widget | `gateway-status` sparkline below the donut | Donut already connected via BFF instant phase counts (OP-DASH-07, OP-DASH-23) |
| Summary row | Optional up/down arrow on usage-summary **Gateways** | Uses the same `trend.points` via `getMetricTrendChange` (OP-DASH-13) |
| `/metrics` embeddable dashboard | Out of scope | `GatewayMetricsDashboard` remains phase cards only (DASH-06) |

### Out of scope (HYPERSHELL-281 remainder)

- Active sandboxes trend (specified in `platform/gateway-sandbox-active-trends.spec.md`)
- Per-status or per-phase trend series on gateway metrics
- A separate snapshot job or new API-server collector for gateway totals (Prometheus range over existing scrape data is sufficient when retention allows)

Hub cluster memory, CPU, and pods utilization trends are specified in `platform/hub-cluster-utilization-trends.spec.md`.

## Requirements

### Requirement: GFT-01 -- Prometheus Retention Prerequisite

Daily fleet-total trends SHALL require only the **default Prometheus TSDB retention** configured in `deploy/base/prometheus/prometheus.yaml` (`7d` today; see `platform/gateway-metrics-dashboard.spec.md` DASH-03). Implementations SHALL NOT require a retention bump beyond `7d` for this feature.

Days before gateway metrics instrumentation existed SHALL appear as zero in the returned daily series after rollout, matching the registered-users daily-series convention (`platform/registered-users.spec.md` RU-11).

#### Scenario: Seven-day daily series is queryable

- GIVEN `hypershell_gateways_total` has been scraped continuously for at least 7 UTC calendar days
- AND Prometheus TSDB retention is at least `7d`
- WHEN the BFF evaluates the daily fleet-total range query (GFT-04)
- THEN Prometheus SHALL return enough samples to populate 7 daily trend points

---

### Requirement: GFT-02 -- Fleet Total Definition

The fleet total at any instant SHALL equal the sum of all canonical gateway phase counts from `hypershell_gateways_total`, using the same phase vocabulary as DASH-01.

Display-status buckets (`healthy`, `provisioning`, `degraded`, `failed`) SHALL NOT be used for trend aggregation. Phase labels `Pending`, `Provisioning`, `Running`, `Degraded`, and `Failed` SHALL all contribute to the total.

The trend value SHALL NOT be filtered by per-gateway RoleBindings. Fleet-wide totals remain dashboard-operator data (OP-DASH-04, DASH-05).

#### Scenario: Total equals sum of phase counts

- GIVEN Prometheus returns `hypershell_gateways_total{phase="Running"}=10`, `{phase="Provisioning"}=3`, `{phase="Degraded"}=1`, `{phase="Failed"}=4`, `{phase="Pending"}=2`
- WHEN the BFF computes the current instant total or a daily trend point
- THEN the fleet total SHALL be `20`

---

### Requirement: GFT-03 -- Extended BFF JSON on Gateways Route

The web-console BFF SHALL extend `GET /api/metrics/gateways` (`platform/gateway-metrics-dashboard.spec.md` DASH-05) with an optional historical field while preserving the existing instant `counts` object unchanged.

| Field | Type | Required | Meaning |
| --- | --- | --- | --- |
| `counts` | object | Yes | Existing phase counts (unchanged) |
| `daily_fleet_totals` | array of `{ date, total }` | No | One entry per UTC calendar day for the last 7 days inclusive of today; `date` is `YYYY-MM-DD`, `total` is the fleet gateway count for that day |

The array SHALL be ordered oldest to newest. The BFF SHALL emit exactly 7 entries (one per UTC calendar day in the lookback window), filling days without TSDB samples with `total: 0`.

Dashboard-operator authorization, error handling for Prometheus connectivity, and OIDC behavior SHALL remain as defined in DASH-05.

When instant phase-count queries succeed but the daily range query fails, the BFF SHALL respond with HTTP `200`, the current `counts` object, and SHALL omit `daily_fleet_totals` (GFT-08).

When instant phase-count queries fail, the BFF SHALL continue to respond with HTTP `502` as today; no partial JSON with trend-only data.

#### Scenario: Successful response includes instant counts and daily totals

- GIVEN Prometheus returns current phase counts and a successful 7-day range evaluation
- WHEN an authorized caller sends `GET /api/metrics/gateways`
- THEN the BFF SHALL respond with HTTP `200`
- AND the body SHALL include `counts` with all canonical phases
- AND `daily_fleet_totals` SHALL contain 7 `{ date, total }` entries

#### Scenario: Range failure returns counts without trend data

- GIVEN Prometheus returns current phase counts
- AND the daily fleet-total range query fails or times out
- WHEN an authorized caller sends `GET /api/metrics/gateways`
- THEN the BFF SHALL respond with HTTP `200` and `counts`
- AND `daily_fleet_totals` SHALL be absent

---

### Requirement: GFT-04 -- Prometheus Range Query

The BFF SHALL query Prometheus `GET {PROMETHEUS_URL}/api/v1/query_range` (via `fetchMetricsRange` in `metrics-source.ts`) to build `daily_fleet_totals`.

| Parameter | Value |
| --- | --- |
| PromQL | `sum(hypershell_gateways_total)` when no namespace is configured; `sum(hypershell_gateways_total{namespace="<namespace>"})` when `PROMETHEUS_NAMESPACE` is set (same `namespaceSelector` label as the instant gateway query in DASH-05) |
| Lookback | 7 UTC calendar days inclusive of today |
| Step | `86400s` (one sample per day) |
| End timestamp | Current time (UTC) |
| Start timestamp | Start of the UTC calendar day 6 days before today |

Each range sample SHALL map to a UTC calendar `date` label (`YYYY-MM-DD`) derived from the sample timestamp. The sample value SHALL be rounded to a non-negative integer fleet total.

After mapping range results, the BFF SHALL align output to the full 7-day UTC calendar grid (oldest to newest) and set `total: 0` for any calendar day missing from the range response.

The BFF SHALL export the PromQL string and step constants from `components/web-console/bff/src/metrics-gateways.ts` (or a sibling module) so unit tests assert the documented query contract.

#### Scenario: Daily points cover the last 7 UTC days

- GIVEN today is UTC `2026-09-15`
- AND Prometheus range evaluation succeeds
- WHEN the BFF builds `daily_fleet_totals`
- THEN the first entry SHALL have `date: "2026-09-09"`
- AND the last entry SHALL have `date: "2026-09-15"`
- AND the array length SHALL be `7`

---

### Requirement: GFT-05 -- Adapter Trend Mapping

The host `DashboardControlPlane` adapter (`components/web-console/app/adapters/api/dashboard-control-plane.ts`) SHALL map optional `daily_fleet_totals` from `GET /api/metrics/gateways` into `OperationalMetric.trend` on the existing `provisioned-gateways` metric:

| BFF field | `OperationalMetric.trend` |
| --- | --- |
| `daily_fleet_totals[].date` | `points[].label` |
| `daily_fleet_totals[].total` | `points[].value` |

When `daily_fleet_totals` is absent, the adapter SHALL still emit `provisioned-gateways` with current `value`, `status`, and instant totals unchanged, and SHALL omit `trend`.

The adapter SHALL NOT synthesize trend points locally and SHALL NOT fail the `gateway-metrics` source solely because trend data is missing.

#### Scenario: Adapter maps daily totals into trend points

- GIVEN the BFF returns `daily_fleet_totals: [{ "date": "2026-09-01", "total": 18 }, { "date": "2026-09-02", "total": 20 }]`
- WHEN the adapter builds `provisioned-gateways`
- THEN `trend.points` SHALL equal `[{ label: "2026-09-01", value: 18 }, { label: "2026-09-02", value: 20 }]`

#### Scenario: Missing daily totals omit trend only

- GIVEN the BFF returns `counts` without `daily_fleet_totals`
- WHEN the adapter builds `provisioned-gateways`
- THEN `value` and `status` SHALL still be populated
- AND `trend` SHALL be absent

---

### Requirement: GFT-06 -- Gateway Status Widget Sparkline

The `gateway-status` widget (`GatewayStatusCard`) SHALL render the existing status donut unchanged.

When `provisioned-gateways.trend.points` contains at least two entries, the widget SHALL render `TrendSparklineChart` below the donut (OP-DASH-12). The sparkline tooltip metric title SHALL use the localized **Provisioned gateways** label (`app.dashboard.widget.provisionedGateways`). The sparkline caption SHALL read **Last 7 days** (localized message ID in the operational-dashboard-ui catalog).

When `trend` is absent or has fewer than two points, the widget SHALL omit the sparkline with no error state.

If the default layout height clips the sparkline after this change, the operational dashboard layout template and persistence key SHALL be bumped so the platform adoption `gateway-status` tile is tall enough to show the donut and sparkline together (same pattern as `registered-users` height adjustments in OP-DASH-11).

#### Scenario: Sparkline renders with 7-day fleet total series

- GIVEN `provisioned-gateways` has `trend.points` with 7 daily fleet totals
- WHEN the gateway status widget renders
- THEN a sparkline SHALL appear below the donut
- AND the caption SHALL read **Last 7 days**

#### Scenario: Missing trend omits sparkline without error

- GIVEN `provisioned-gateways` has `value` and `status` but no `trend`
- WHEN the gateway status widget renders
- THEN the donut SHALL still render
- AND no sparkline or error state SHALL appear below it

---

### Requirement: GFT-07 -- Usage Summary Gateways Trend Arrow

The usage summary **Gateways** row SHALL continue to show the current fleet total from `provisioned-gateways.value` with exception status icons unchanged (OP-DASH-12).

When `provisioned-gateways.trend` is present, the row SHALL evaluate `getMetricTrendChange` (5% threshold between first and last trend points) and MAY show the localized increase/decrease indicator (OP-DASH-13). The arrow SHALL reflect **fleet gateway total** change over the sparkline window, not healthy-only or failed-only counts.

When `trend` is absent, the row SHALL show the total and exception icons only, without a trend arrow.

#### Scenario: Fleet growth shows an increase arrow

- GIVEN `provisioned-gateways` has `value: "25"` and `trend.points` where the last day is at least 5% higher than the first day
- WHEN the usage summary **Gateways** row renders
- THEN an increase trend indicator SHALL appear beside the total

---

### Requirement: GFT-08 -- Partial Failure and Refresh Behavior

Trend data SHALL be optional within the `gateway-metrics` metric source (`web-console/operational-dashboard.spec.md` OP-DASH-19). A failure to load `daily_fleet_totals` SHALL NOT omit instant `provisioned-gateways`, `provisioned-sandboxes`, `provision-time`, or `provision-reliability` when those routes succeed.

The dashboard SHALL NOT enter total initial-load error state solely because trend data is missing while instant gateway counts loaded successfully.

Trend data SHALL refresh on the same operational dashboard cadence as other metrics (`operationalDashboardRefreshMilliseconds`, 15 minutes).

#### Scenario: Trend unavailable does not hide the gateway donut

- GIVEN `GET /api/metrics/gateways` returns `counts` without `daily_fleet_totals`
- AND other gateway-metrics routes succeed
- WHEN an authorized operator opens `/dashboard`
- THEN the gateway status donut SHALL display current counts
- AND the sparkline SHALL be omitted
- AND the dashboard SHALL NOT show the total initial-load danger alert

---

### Requirement: GFT-09 -- Verification

The web-console BFF SHALL include unit tests for:

- Successful mapping from mocked Prometheus range responses into `daily_fleet_totals`
- HTTP `200` with `counts` only when the range query fails but instant queries succeed
- Documented PromQL and step parameters (GFT-04)

The web console SHALL include unit tests for the dashboard adapter mapping `daily_fleet_totals` into `provisioned-gateways.trend`, including full and trend-absent payloads.

The operational dashboard package SHALL update `mockOperationalDashboardMetrics` with representative `provisioned-gateways.trend` points for Storybook and SHALL add or extend a Storybook state showing the gateway status widget with the sparkline visible.

#### Scenario: CI exercises adapter trend mapping

- GIVEN a mocked BFF gateways response with known `daily_fleet_totals`
- WHEN dashboard adapter unit tests run
- THEN they SHALL assert `provisioned-gateways` `trend.points` labels and values
- AND assert `trend` is omitted when `daily_fleet_totals` is absent

## Non-Goals

- Per-status or per-phase trend lines on the gateway status widget
- Changes to `GatewayMetricsDashboard` at `/metrics`
- Hub cluster utilization trends (memory, CPU, pods)
- Active sandboxes historical series (see `platform/gateway-sandbox-active-trends.spec.md`)
- A HyperShell REST aggregate or snapshot job when Prometheus range data is sufficient
- Direct browser access to Prometheus
