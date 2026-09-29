# Hub Cluster Utilization Trends

**Status:** Active
**Applies to:** `deploy/base/prometheus/`, `components/web-console` BFF and dashboard adapter, `packages/operational-dashboard-ui`
**Tracks:** [HYPERSHELL-281](https://redhat.atlassian.net/browse/HYPERSHELL-281) (hub memory, CPU, and pods sparkline scope)

## Purpose

Add **7-day usage trend graphs (sparklines)** for hub-cluster **used** memory, CPU, and pod counts on the operational dashboard. Each sparkline plots how much of the hub cluster resource was **in use** over time. Capacity, utilization percentage, and phase breakdowns remain point-in-time on the primary chart (utilization donut or pod capacity donut).

This specification extends the connected BFF routes and adapter mappings defined in:

- `platform/cluster-memory.spec.md` (`memory` metric)
- `platform/cluster-cpu.spec.md` (`cpu` metric)
- `platform/cluster-pods.spec.md` (`pods` metric)

Trend series come from Prometheus TSDB range queries over the same PromQL expressions used for instant **used** measurements (CM-02, CC-02, CLP-02). Trends SHALL NOT plot capacity, available headroom, utilization percentage, or per-phase pod counts.

### Relationship to the operational dashboard

| Metric ID | Widget | Primary chart (point-in-time) | Trend series (7 days) |
| --- | --- | --- | --- |
| `memory` | `memory` | `UtilizationChart` (used vs capacity GiB) | Daily **used** GiB |
| `cpu` | `cpu` | `UtilizationChart` (used vs capacity cores) | Daily **used** cores |
| `pods` | `pods` | `PodCapacityChart` (used vs capacity pods + phases) | Daily **used** pod count |

Hub cluster scope, authorization, refresh policy, and instant-query failure semantics remain defined in the cluster memory, CPU, and pods specs and `web-console/operational-dashboard.spec.md` OP-DASH-04, OP-DASH-09, and OP-DASH-19.

### Relationship to gateway fleet total trend

Prometheus TSDB retention for Prometheus-backed operational dashboard trends is defined once in `platform/gateway-fleet-total-trend.spec.md` GFT-01 (default `7d`; no retention bump). Hub cluster utilization trends use the same lookback window.

Daily trend semantics SHALL match gateway fleet total trends (`platform/gateway-fleet-total-trend.spec.md` GFT-04): the BFF evaluates the **same instant PromQL** used for the headline **used** measurement via Prometheus `query_range` with a **86400s** step over **7 UTC calendar days** inclusive of today. Each daily point is a UTC calendar-day snapshot of **used** consumption (not capacity, utilization percentage, or a daily average/max aggregate). After mapping range samples, the BFF SHALL align to the full 7-day grid with `alignDailyIntegerSeries` and zero-fill calendar days missing from the range response, matching gateway `daily_fleet_totals` behavior.

## Requirements

### Requirement: HCUT-01 -- Prometheus Retention Prerequisite

Hub cluster utilization trends SHALL require only the **default Prometheus TSDB retention** specified in `platform/gateway-fleet-total-trend.spec.md` GFT-01.

Days before node-exporter or kube-state-metrics scrape data existed SHALL appear as `value: 0` in the returned daily series after rollout, matching the registered-users and gateway fleet-total daily-series conventions.

#### Scenario: Seven-day memory used series is queryable

- GIVEN hub-cluster memory scrape data has been retained for at least 7 UTC calendar days
- WHEN the BFF evaluates the memory daily-used range query (HCUT-04)
- THEN Prometheus SHALL return enough samples to populate 7 daily trend points

---

### Requirement: HCUT-02 -- Used-Only Trend Values

Each trend series SHALL plot **used** resource consumption only:

| Metric ID | Trend unit | Trend meaning |
| --- | --- | --- |
| `memory` | GiB (whole numbers) | Hub-cluster memory in use (`used_bytes` converted with the same rounding as CM-05) |
| `cpu` | cores (whole numbers) | Hub-cluster non-idle CPU cores in use (same rounding as CC-05) |
| `pods` | pods (whole numbers) | Hub-cluster pod objects counted toward `used_pods` (same rounding as CLP-05) |

Trend points SHALL NOT encode capacity, available headroom, utilization percentage, or pod phase breakdown.

Daily trend values SHALL use the same display rounding as the corresponding instant `OperationalMetric.value` so the sparkline Y axis matches the headline used figure on the primary chart.

#### Scenario: Memory trend tracks used GiB not capacity

- GIVEN a daily memory trend point has `value: 220`
- WHEN the operator compares it to the same day's instant `memory` metric
- THEN the trend value SHALL represent used GiB
- AND it SHALL NOT represent capacity or utilization percentage

---

### Requirement: HCUT-03 -- Shared Daily Series JSON Shape

Each hub-cluster BFF route SHALL extend its existing JSON response with an optional historical field while preserving all instant fields unchanged:

| Field | Type | Required | Meaning |
| --- | --- | --- | --- |
| Existing instant fields | varies | Yes | Unchanged (`capacity_*`, `used_*`, phase fields for pods) |
| `daily_used` | array of `{ date, value }` | No | One entry per UTC calendar day for the last 7 days inclusive of today; `date` is `YYYY-MM-DD`; `value` is the **used** amount in display units (HCUT-02) |

The array SHALL be ordered oldest to newest. The BFF SHALL emit exactly 7 entries, filling calendar days without TSDB samples with `value: 0`.

When instant queries succeed but the daily range query fails, the BFF SHALL respond with HTTP `200`, the current instant fields, and SHALL omit `daily_used`.

When instant queries fail, the BFF SHALL continue to respond with HTTP `502` as defined in CM-04, CC-04, and CLP-04. The BFF SHALL NOT return trend-only JSON without instant fields.

#### Scenario: Memory route returns instant bytes and optional daily used series

- GIVEN Prometheus returns successful instant memory samples and a successful 7-day range evaluation
- WHEN an authorized caller sends `GET /api/metrics/cluster-memory`
- THEN the BFF SHALL respond with HTTP `200`
- AND the body SHALL include `capacity_bytes`, `available_bytes`, and `used_bytes`
- AND `daily_used` SHALL contain 7 `{ date, value }` entries with `value` in whole GiB

#### Scenario: Range failure returns instant memory without trend data

- GIVEN Prometheus returns successful instant memory samples
- AND the daily-used range query fails or times out
- WHEN an authorized caller sends `GET /api/metrics/cluster-memory`
- THEN the BFF SHALL respond with HTTP `200` with instant byte fields
- AND `daily_used` SHALL be absent

---

### Requirement: HCUT-04 -- Memory Range Query

The BFF SHALL query Prometheus `GET {PROMETHEUS_URL}/api/v1/query_range` (via `fetchMetricsRange` in `metrics-source.ts`) to build `daily_used` for `GET /api/metrics/cluster-memory`.

| Parameter | Value |
| --- | --- |
| PromQL | `sum(node_memory_MemTotal_bytes) - sum(node_memory_MemAvailable_bytes)` |
| Lookback | 7 UTC calendar days inclusive of today |
| Step | `86400s` (one sample per day) |
| End timestamp | Current time (UTC) |
| Start timestamp | Start of the UTC calendar day 6 days before today |

Each range sample SHALL map to a UTC calendar `date` label (`YYYY-MM-DD`) derived from the sample timestamp. The sample value SHALL be converted to whole GiB with `round(bytes / 1024³)` before inclusion in `daily_used`.

After mapping range results, the BFF SHALL align output to the full 7-day UTC calendar grid (oldest to newest) with `alignDailyIntegerSeries` and set `value: 0` for any calendar day missing from the range response (same convention as GFT-04).

The BFF SHALL export the PromQL string and step constants from `components/web-console/bff/src/metrics-cluster-memory.ts` (or a sibling module) so unit tests assert the documented query contract.

#### Scenario: Daily memory points cover the last 7 UTC days

- GIVEN today is UTC `2026-09-15`
- AND Prometheus range evaluation succeeds
- WHEN the BFF builds memory `daily_used`
- THEN the first entry SHALL have `date: "2026-09-09"`
- AND the last entry SHALL have `date: "2026-09-15"`
- AND the array length SHALL be `7`

---

### Requirement: HCUT-05 -- CPU Range Query

The BFF SHALL query Prometheus `query_range` to build `daily_used` for `GET /api/metrics/cluster-cpu`.

| Parameter | Value |
| --- | --- |
| PromQL | `sum(rate(node_cpu_seconds_total{mode!="idle"}[5m]))` |
| Lookback | 7 UTC calendar days inclusive of today |
| Step | `86400s` |
| End timestamp | Current time (UTC) |
| Start timestamp | Start of the UTC calendar day 6 days before today |

Each range sample SHALL map to a UTC calendar `date` label and SHALL be rounded to the nearest whole core before inclusion in `daily_used`, matching CC-05 instant display rounding.

After mapping range results, the BFF SHALL align output to the full 7-day UTC calendar grid with `alignDailyIntegerSeries` and zero-fill missing calendar days (GFT-04).

The BFF SHALL export the PromQL string and step constants from `components/web-console/bff/src/metrics-cluster-cpu.ts` so unit tests assert the documented query contract.

#### Scenario: CPU trend values are whole cores

- GIVEN a range sample evaluates to `48.6` used cores
- WHEN the BFF maps the sample into `daily_used`
- THEN the corresponding entry SHALL have `value: 49`

---

### Requirement: HCUT-06 -- Pods Range Query

The BFF SHALL query Prometheus `query_range` to build `daily_used` for `GET /api/metrics/cluster-pods`.

| Parameter | Value |
| --- | --- |
| PromQL | `count(kube_pod_info)` |
| Lookback | 7 UTC calendar days inclusive of today |
| Step | `86400s` |
| End timestamp | Current time (UTC) |
| Start timestamp | Start of the UTC calendar day 6 days before today |

Each range sample SHALL map to a UTC calendar `date` label and SHALL be rounded to a non-negative integer pod count before inclusion in `daily_used`, matching CLP-05 instant display rounding.

Instant phase-count fields (`phase_*_pods`) SHALL remain point-in-time only. The range query SHALL NOT emit per-phase historical series in version 1.

After mapping range results, the BFF SHALL align output to the full 7-day UTC calendar grid with `alignDailyIntegerSeries` and zero-fill missing calendar days (GFT-04).

The BFF SHALL export the PromQL string and step constants from `components/web-console/bff/src/metrics-cluster-pods.ts` so unit tests assert the documented query contract.

#### Scenario: Pods trend tracks total used pod count

- GIVEN a daily pods trend point has `value: 548`
- WHEN the operator compares it to the same day's instant `pods` metric `value`
- THEN both SHALL represent `used_pods`
- AND the trend SHALL NOT include phase breakdown

---

### Requirement: HCUT-07 -- Adapter Trend Mapping

The host `DashboardControlPlane` adapter SHALL map optional `daily_used` from each cluster BFF route into `OperationalMetric.trend` on the corresponding metric:

| BFF route | Metric ID | Mapping |
| --- | --- | --- |
| `GET /api/metrics/cluster-memory` | `memory` | `daily_used[].date` → `trend.points[].label`; `daily_used[].value` → `trend.points[].value` |
| `GET /api/metrics/cluster-cpu` | `cpu` | same |
| `GET /api/metrics/cluster-pods` | `pods` | same |

When `daily_used` is absent, the adapter SHALL still emit the metric with current instant `value`, `total`, `unit`, and (for pods) `podPhases` unchanged, and SHALL omit `trend`.

The adapter SHALL NOT synthesize trend points locally and SHALL NOT fail the cluster-memory, cluster-cpu, or cluster-pods metric source solely because trend data is missing.

#### Scenario: Adapter maps memory daily used into trend points

- GIVEN the BFF returns `daily_used: [{ "date": "2026-09-01", "value": 218 }, { "date": "2026-09-02", "value": 220 }]`
- WHEN the adapter builds `memory`
- THEN `trend.points` SHALL equal `[{ label: "2026-09-01", value: 218 }, { label: "2026-09-02", value: 220 }]`
- AND `value`, `total`, and `unit` SHALL still reflect the instant response

#### Scenario: Missing daily used omits trend only

- GIVEN the BFF returns instant CPU fields without `daily_used`
- WHEN the adapter builds `cpu`
- THEN `value`, `total`, and `unit` SHALL still be populated
- AND `trend` SHALL be absent

---

### Requirement: HCUT-08 -- Widget Usage Trend Sparklines

Each hub-cluster utilization widget (`memory`, `cpu`, `pods`) SHALL render a **usage trend graph** (`TrendSparklineChart`) below its primary chart when historical data is available, matching the gateway-status widget pattern (`platform/gateway-fleet-total-trend.spec.md` GFT-06).

**Memory and CPU widgets** (`UtilizationCard`) SHALL render the existing `UtilizationChart` unchanged for point-in-time capacity utilization.

When `trend.points` contains at least two entries, the widget SHALL render `TrendSparklineChart` below the utilization chart plotting daily **used** amounts (HCUT-02). Sparkline tooltip titles SHALL use the localized widget titles for Memory and CPU. Sparkline captions SHALL read **Last 7 days** (localized message IDs in the operational-dashboard-ui catalog).

**Pods widget** (`PodCapacityCard`) SHALL render the existing `PodCapacityChart` unchanged for point-in-time capacity and phase breakdown.

When `pods.trend.points` contains at least two entries, the widget SHALL render `TrendSparklineChart` below the capacity donut plotting daily **used** pod counts with the localized Pods widget title and **Last 7 days** caption.

When `trend` is absent or has fewer than two points, widgets SHALL omit the sparkline with no error state.

If the default layout height clips sparklines after this change, the operational dashboard layout template and persistence key SHALL be bumped so hub-cluster `memory`, `cpu`, and `pods` tiles are tall enough to show the primary chart and sparkline together (same pattern as registered-users and gateway-status height adjustments in OP-DASH-11).

System-summary rows for memory, CPU, and pods SHALL NOT render trend sparklines. When `trend.points` spans at least two entries and first-to-last change meets the usage-summary threshold (`TREND_CHANGE_THRESHOLD_PERCENT`, 5%), those rows SHALL show the same increase/decrease arrow treatment as `usage-summary` (GFT-07 pattern). The provision success-rate row SHALL use `successRateTrend` the same way when hourly success-rate history is present.

#### Scenario: Memory widget shows utilization donut and sparkline

- GIVEN the `memory` metric has instant `value`, `total`, and `unit`
- AND `trend.points` contains 7 daily used GiB entries
- WHEN the memory widget renders
- THEN a utilization donut SHALL appear
- AND a sparkline SHALL appear below it with caption **Last 7 days**

#### Scenario: Pods widget omits sparkline when trend is absent

- GIVEN the `pods` metric has instant capacity and phase fields but no `trend`
- WHEN the pods widget renders
- THEN the capacity donut SHALL still render
- AND no sparkline or error state SHALL appear below it

---

### Requirement: HCUT-09 -- Partial Failure and Refresh Behavior

Trend data SHALL be optional within each cluster metric source (`cluster-memory`, `cluster-cpu`, `cluster-pods`) per OP-DASH-19. A failure to load `daily_used` SHALL NOT omit the instant metric when instant queries succeed.

The dashboard SHALL NOT enter total initial-load error state solely because hub-cluster trend data is missing while instant hub-cluster metrics loaded successfully.

Trend data SHALL refresh on the same operational dashboard cadence as other metrics (`operationalDashboardRefreshMilliseconds`, 15 minutes).

#### Scenario: Memory trend unavailable does not hide the utilization donut

- GIVEN `GET /api/metrics/cluster-memory` returns instant byte fields without `daily_used`
- AND at least one other metric source succeeds
- WHEN an authorized operator opens `/dashboard`
- THEN the memory widget SHALL display the current utilization donut
- AND the sparkline SHALL be omitted
- AND the dashboard SHALL NOT show the total initial-load danger alert

---

### Requirement: HCUT-10 -- Verification

The web-console BFF SHALL include unit tests for each cluster route covering:

- Successful mapping from mocked Prometheus range responses into `daily_used`
- HTTP `200` with instant fields only when the range query fails but instant queries succeed
- Documented PromQL and step parameters (HCUT-04 through HCUT-06)

The web console SHALL include unit tests for the dashboard adapter mapping `daily_used` into `memory`, `cpu`, and `pods` trend fields, including full and trend-absent payloads.

The operational dashboard package SHALL update `mockOperationalDashboardMetrics` with representative `trend` points on `memory`, `cpu`, and `pods` for Storybook and SHALL add or extend Storybook states showing utilization widgets with sparklines visible.

`packages/operational-dashboard-ui/DATA_SOURCES.md` SHALL document the range-query PromQL expressions and note that instant and daily-used series share the same hub-cluster scope.

#### Scenario: CI exercises memory adapter trend mapping

- GIVEN a mocked BFF cluster-memory response with known `daily_used`
- WHEN dashboard adapter unit tests run
- THEN they SHALL assert the `memory` metric `trend.points` labels and values
- AND assert `trend` is omitted when `daily_used` is absent

## Non-Goals

- Historical trends for hub-cluster **capacity**, **available**, or **utilization percentage**
- Per-phase pod historical series
- Node inventory trends (`nodes` metric)
- System-summary trend sparklines for memory, CPU, or pods
- Combining memory, CPU, and pods into one BFF route
- Hub-cluster metrics for registered managed clusters or gateway tenant namespaces
- Direct browser access to Prometheus
