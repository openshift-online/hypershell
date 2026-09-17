# Gateway Release Distribution

**Status:** Active
**Applies to:** `components/api-server` gateway and gateway release list APIs, `components/api-server/pkg/rbac`, `components/sdk-typescript`, `components/web-console/app/adapters/api/gateway-release-distribution-aggregation.ts`, `components/web-console` dashboard adapter, `packages/operational-dashboard-ui/src/dashboard/gateway-release-distribution-data.ts`, `packages/operational-dashboard-ui/src/dashboard/gateway-releases-chart.tsx`

**Tracks:** [HYPERSHELL-280](https://redhat.atlassian.net/browse/HYPERSHELL-280)

## Purpose

Expose **fleet-wide gateway release adoption** on the operational dashboard so administrators can see how many gateways run each `GatewayRelease` version.

Version 1 aggregates release distribution from existing paginated List APIs:

- `GET /api/hypershell/v1/gateways` (group by each gateway's `release_id`)
- `GET /api/hypershell/v1/gateway_releases` (resolve release IDs to human-readable `name` values)

This specification defines aggregation rules, the `gateway-releases` operational metric contract, dashboard-operator authorization expectations, and the **Gateway releases** widget presentation. It does **not** add usage trend graphs, rollout progress percentages, or new inventory APIs.

### Relationship to other specifications

| Concern | Gateway release distribution (this spec) | Operational dashboard |
| --- | --- | --- |
| Data source | HyperShell REST list APIs (fleet-wide for dashboard operators) | Widget layout, refresh, access control |
| Gateway phase counts | Unrelated (`hypershell_gateways_total`, `platform/gateway-metrics-dashboard.spec.md`) | `gateway-status` widget uses Prometheus phase counts |
| Gateway provision statistics | Sibling work on HYPERSHELL-280 (`platform/gateway-provision-time.spec.md`, provision reliability) | Separate widgets and summary rows |
| Platform inventory | Same paginated-list aggregation pattern (`platform/platform-inventory.spec.md`) | Independent metric source (OP-DASH-19) |

Prometheus gateway metrics SHALL NOT be used for release distribution in version 1. The adapter SHALL NOT infer release adoption from container image strings on gateway rows when a `release_id` is present.

## Requirements

### Requirement: GRD-01 -- Release Distribution Scope

Gateway release distribution version 1 SHALL answer: **how many gateways in the fleet reference each GatewayRelease**.

The adapter SHALL:

- Count every gateway returned by the fleet-wide gateway List API
- Group counts by each gateway's `release_id`
- Resolve non-empty `release_id` values to the referenced release's `name`
- Bucket omitted, null, or blank `release_id` values as `unknown`

The metric SHALL NOT report the number of GatewayRelease catalog records. It SHALL report gateway counts per release label.

Historical trend series (`OperationalMetric.trend`) SHALL NOT be loaded in version 1.

#### Scenario: Three gateways on two releases

- GIVEN the fleet contains two gateways on release `r1` (`OpenShell 2.0`) and one gateway on release `r2` (`OpenShell 2.1`)
- WHEN release distribution metrics load
- THEN `releaseDistribution` SHALL be `{ "OpenShell 2.0": 2, "OpenShell 2.1": 1 }`
- AND `value` SHALL be `"3"`

#### Scenario: Gateway without release_id buckets as unknown

- GIVEN one gateway has an empty `release_id`
- WHEN release distribution metrics load
- THEN the distribution SHALL include an `unknown` bucket with count `1`
- AND the widget SHALL localize the `unknown` bucket label per inventory dimension rules

---

### Requirement: GRD-02 -- Paginated List Aggregation

The web-console dashboard adapter SHALL load gateway release distribution by paginating both List APIs through the browser TypeScript SDK with page size `100`, ordered by `name asc`, until all pages are retrieved.

| Resource | List endpoint | Purpose |
| --- | --- | --- |
| Gateway | `GET /api/hypershell/v1/gateways` | Count gateways per `release_id` |
| GatewayRelease | `GET /api/hypershell/v1/gateway_releases` | Map release ID to display name |

The adapter SHALL validate each list response for internal consistency (requested page number, `total`, and `items` length). An inconsistent gateway or release list response SHALL fail only the `gateway-release-distribution` metric source; other metric sources SHALL still be attempted (`web-console/operational-dashboard.spec.md` OP-DASH-19).

After aggregating all gateway pages, the sum of per-release counts MUST equal the gateway list `total`. A mismatch SHALL fail the source.

The adapter SHALL NOT issue per-gateway Get requests. Release distribution SHALL use at most one paginated List sequence per resource kind per metrics refresh.

#### Scenario: Multiple gateway pages are aggregated

- GIVEN the caller can list 150 gateways across two pages
- AND 100 gateways reference release `r1` and 50 reference release `r2`
- WHEN release distribution metrics load
- THEN the adapter SHALL aggregate all pages before emitting the metric
- AND `value` SHALL be `"150"`

#### Scenario: Inconsistent gateway list fails the source

- GIVEN a gateway list page returns `total: 10` but only nine items were aggregated across all pages
- WHEN release distribution metrics load
- THEN the `gateway-release-distribution` source SHALL fail
- AND `gateway-releases` SHALL be omitted from the adapter response

---

### Requirement: GRD-03 -- Release Label Resolution

For each distinct `release_id` bucket, the adapter SHALL resolve a display label using this precedence:

1. When `release_id` is omitted, null, or blank after trim, use the bucket key `unknown`
2. When the ID matches a listed GatewayRelease and `name` is non-empty after trim, use that `name`
3. Otherwise use the raw `release_id` string

When multiple release IDs resolve to the same display name, their gateway counts SHALL be merged into one distribution entry.

When a release list row has an empty `name`, the adapter SHALL fall back to the release ID (rule 3).

#### Scenario: Missing release record falls back to ID

- GIVEN a gateway references `release_id = "rel-orphan"`
- AND no GatewayRelease list item exists with `id = "rel-orphan"`
- WHEN release distribution metrics load
- THEN the distribution key SHALL be `"rel-orphan"`

#### Scenario: Duplicate names merge counts

- GIVEN two release IDs both resolve to the display name `OpenShell stable`
- AND one gateway references each ID
- WHEN release distribution metrics load
- THEN `releaseDistribution["OpenShell stable"]` SHALL be `2`

---

### Requirement: GRD-04 -- Operational Metric Contract

The adapter SHALL expose exactly one operational metric:

| Field | Value |
| --- | --- |
| `id` | `"gateway-releases"` |
| `value` | Decimal string of fleet gateway total (same cardinality as the gateway List `total`) |
| `releaseDistribution` | Record mapping resolved release label to gateway count |

The metric SHALL NOT emit `status`, `trend`, `total` (as a separate utilization field), or `unit`.

`DATA_SOURCES.md` SHALL document the REST list sources and note that `value` is fleet gateway total while the widget presents per-release counts from `releaseDistribution`.

#### Scenario: Metric maps aggregate to OperationalMetric

- GIVEN aggregation produced total `6` and distribution `{ "OpenShell 2.0": 4, "OpenShell 2.1": 2 }`
- WHEN the adapter builds the metric
- THEN the response SHALL include one metric with `id: "gateway-releases"`, `value: "6"`, and the same `releaseDistribution` object

---

### Requirement: GRD-05 -- Independent Metric Source

The host adapter SHALL load `gateway-releases` from an independent metric source identifier `gateway-release-distribution`.

A failure in gateway list aggregation, gateway release list aggregation, or adapter validation SHALL:

- Omit `gateway-releases` from the returned `metrics` array
- Record `gateway-release-distribution` in `failedSources` when at least one other source succeeds
- NOT synthesize zero or placeholder distribution entries

A failure in Prometheus gateway metrics or sandbox metrics SHALL NOT prevent `gateway-releases` from loading when the REST aggregation succeeds.

The adapter SHALL honor `AbortSignal` cancellation from `DashboardControlPlane.getOperationalMetrics`.

#### Scenario: Gateway metrics failure does not hide release distribution

- GIVEN `GET /api/hypershell/v1/gateways` and `GET /api/hypershell/v1/gateway_releases` succeed for a dashboard operator
- AND the `gateway-metrics` Prometheus source fails
- WHEN the operator opens `/dashboard`
- THEN the Gateway releases widget SHALL display loaded release rows
- AND the gateway status widget SHALL render the localized metric-unavailable state

#### Scenario: Gateway list failure omits only release distribution

- GIVEN every other metric source succeeds
- AND gateway list aggregation fails
- WHEN the operator opens `/dashboard`
- THEN the Gateway releases widget SHALL render the localized metric-unavailable state
- AND other connected widgets SHALL remain populated

---

### Requirement: GRD-06 -- Dashboard Operator Authorization

Release distribution aggregation SHALL use the same dashboard-operator caller context as other operational dashboard REST-backed aggregates.

When OIDC is enabled, callers without `platform:admin` SHALL NOT receive fleet-wide gateway list totals through the dashboard adapter. The API server's fleet-wide gateway list bypass for dashboard operators is defined in `security/rbac-enforcement.spec.md`.

The adapter SHALL NOT apply additional per-gateway filtering beyond what the List APIs enforce for the authenticated caller.

#### Scenario: Dashboard operator sees fleet-wide distribution

- GIVEN the signed-in user has `platform:admin`
- AND the fleet contains gateways the user does not individually own
- WHEN release distribution metrics load
- THEN the distribution SHALL include those gateways

---

### Requirement: GRD-07 -- Verification

The web-console adapter SHALL include unit tests in `gateway-release-distribution-aggregation.test.ts` for:

- Release ID bucketing and label resolution helpers (`bucketReleaseId`, `resolveReleaseLabel`, including empty release names that fall back to the raw ID)
- Metric mapping from aggregation output (`buildGatewayReleasesMetric`)
- Independent source failure behavior in `createDashboardControlPlaneAdapter` (gateway list failure omits `gateway-releases` and records `gateway-release-distribution` in `failedSources`)

The operational dashboard package SHALL include unit tests in `gateway-release-distribution-data.test.ts` for descending sort order of release list entries via `buildGatewayReleaseListEntries`.

`mockOperationalDashboardMetrics` SHALL include a representative `gateway-releases` metric (including an `unknown` bucket). Storybook fixtures MAY render that metric without calling the HyperShell API.

---

## Non-Goals

- Provisioning summary card aggregating median, P95, and success rate (remaining HYPERSHELL-280 scope)
- Release distribution trend graphs
- Canary rollout percentage or release health scoring
- Replacing the Usage summary sandboxes row (sandbox totals remain on `provisioned-sandboxes` via OP-DASH-06)
