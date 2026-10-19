# Gateway Release Distribution

**Status:** Active
**Applies to:** `components/api-server` gateway list API, `components/api-server/pkg/rbac`, `components/sdk-typescript`, `components/web-console/app/adapters/api/gateway-release-distribution-aggregation.ts`, `components/web-console` dashboard adapter, `packages/operational-dashboard-ui/src/dashboard/gateway-release-distribution-data.ts`, `packages/operational-dashboard-ui/src/dashboard/gateway-releases-chart.tsx`

**Tracks:** [HYPERSHELL-280](https://redhat.atlassian.net/browse/HYPERSHELL-280)

## Purpose

Expose **fleet-wide gateway version adoption** on the operational dashboard so administrators can see how many gateways run each OpenShell version.

The distribution is computed from each gateway's **observed running version** -- the `gateway_version` the gateway reports on its `/health` endpoint, stored on the gateway row (`platform/openshell-gateway-health.spec.md`).

Version 1 aggregates the distribution from a single existing paginated List API:

- `GET /api/hypershell/v1/gateways` (group by each gateway's observed `gateway_version`)

This specification defines aggregation rules, the `gateway-releases` operational metric contract, dashboard-operator authorization expectations, and the **Gateway releases** widget presentation. It does **not** add usage trend graphs, rollout progress percentages, or new inventory APIs.

### Relationship to other specifications

| Concern | Gateway release distribution (this spec) | Operational dashboard |
| --- | --- | --- |
| Data source | HyperShell REST gateway list API (fleet-wide for dashboard operators) | Widget layout, refresh, access control |
| Gateway phase counts | Unrelated (`hypershell_gateways_total`, `platform/gateway-metrics-dashboard.spec.md`) | `gateway-status` widget uses Prometheus phase counts |
| Gateway provision statistics | Sibling work on HYPERSHELL-280 (`platform/gateway-provision-time.spec.md`, provision reliability) | Separate widgets and summary rows |
| Platform inventory | Same paginated-list aggregation pattern (`platform/platform-inventory.spec.md`) | Independent metric source (OP-DASH-19) |
| Running version | Observed `gateway_version` origin (`platform/openshell-gateway-health.spec.md`) | `gateway-releases` widget presents the per-version counts |

Prometheus gateway metrics SHALL NOT be used for release distribution in version 1. The adapter SHALL NOT infer version adoption from container image strings on gateway rows; it SHALL use the observed `gateway_version` field.

## Requirements

### Requirement: GRD-01 -- Version Distribution Scope

Gateway release distribution version 1 SHALL answer: **how many gateways in the fleet run each observed `gateway_version`**.

The adapter SHALL:

- Count every gateway returned by the fleet-wide gateway List API
- Group counts by each gateway's observed `gateway_version`
- Bucket omitted, null, or blank `gateway_version` values as `unknown`

The metric SHALL report gateway counts per version label. It SHALL NOT report any catalog record count.

Historical trend series (`OperationalMetric.trend`) SHALL NOT be loaded in version 1.

#### Scenario: Three gateways on two versions

- GIVEN the fleet contains two gateways reporting `gateway_version` `v2.0.0` and one gateway reporting `v2.1.0`
- WHEN release distribution metrics load
- THEN `releaseDistribution` SHALL be `{ "v2.0.0": 2, "v2.1.0": 1 }`
- AND `value` SHALL be `"3"`

#### Scenario: Gateway without gateway_version buckets as unknown

- GIVEN one gateway has an empty `gateway_version`
- WHEN release distribution metrics load
- THEN the distribution SHALL include an `unknown` bucket with count `1`
- AND the widget SHALL localize the `unknown` bucket label per inventory dimension rules

---

### Requirement: GRD-02 -- Paginated List Aggregation

The web-console dashboard adapter SHALL load gateway version distribution by paginating the gateway List API through the browser TypeScript SDK with page size `100`, ordered by `name asc`, until all pages are retrieved.

| Resource | List endpoint | Purpose |
| --- | --- | --- |
| Gateway | `GET /api/hypershell/v1/gateways` | Count gateways per observed `gateway_version` |

The adapter SHALL validate each list response for internal consistency (requested page number, non-negative `total`, and an `items` length no larger than the page can hold). An inconsistent gateway list response SHALL fail only the `gateway-release-distribution` metric source; other metric sources SHALL still be attempted (`web-console/operational-dashboard.spec.md` OP-DASH-19).

After aggregating all gateway pages, the sum of per-version counts MUST equal the gateway list `total`. A mismatch SHALL fail the source.

The adapter SHALL NOT issue per-gateway Get requests and SHALL NOT call any other List API. Release distribution SHALL use at most one paginated gateway List sequence per metrics refresh.

#### Scenario: Multiple gateway pages are aggregated

- GIVEN the caller can list 150 gateways across two pages
- AND 100 gateways report `gateway_version` `v1.0.0` and 50 report `v1.1.0`
- WHEN release distribution metrics load
- THEN the adapter SHALL aggregate all pages before emitting the metric
- AND `value` SHALL be `"150"`

#### Scenario: Inconsistent gateway list fails the source

- GIVEN a gateway list page returns `total: 10` but only nine items were aggregated across all pages
- WHEN release distribution metrics load
- THEN the `gateway-release-distribution` source SHALL fail
- AND `gateway-releases` SHALL be omitted from the adapter response

---

### Requirement: GRD-03 -- Version Label Handling

The adapter SHALL derive each distribution bucket key directly from the observed `gateway_version` string:

1. When `gateway_version` is omitted, null, or blank after trim, use the bucket key `unknown`
2. Otherwise use the trimmed `gateway_version` string as the bucket key

Gateways reporting the same version after trim SHALL be merged into one distribution entry. The adapter SHALL NOT resolve versions against any external catalog or API.

#### Scenario: Whitespace is trimmed to a single bucket

- GIVEN one gateway reports `gateway_version` `" v1.0.0 "` and another reports `"v1.0.0"`
- WHEN release distribution metrics load
- THEN `releaseDistribution["v1.0.0"]` SHALL be `2`

#### Scenario: Blank version falls back to unknown

- GIVEN a gateway reports an empty or whitespace-only `gateway_version`
- WHEN release distribution metrics load
- THEN that gateway SHALL be counted under the `unknown` bucket

---

### Requirement: GRD-04 -- Operational Metric Contract

The adapter SHALL expose exactly one operational metric:

| Field | Value |
| --- | --- |
| `id` | `"gateway-releases"` |
| `value` | Decimal string of fleet gateway total (same cardinality as the gateway List `total`) |
| `releaseDistribution` | Record mapping observed version label to gateway count |

The metric SHALL NOT emit `status`, `trend`, `total` (as a separate utilization field), or `unit`.

`DATA_SOURCES.md` SHALL document the REST gateway-list source and note that `value` is fleet gateway total while the widget presents per-version counts from `releaseDistribution`.

#### Scenario: Metric maps aggregate to OperationalMetric

- GIVEN aggregation produced total `6` and distribution `{ "v2.0.0": 4, "v2.1.0": 2 }`
- WHEN the adapter builds the metric
- THEN the response SHALL include one metric with `id: "gateway-releases"`, `value: "6"`, and the same `releaseDistribution` object

---

### Requirement: GRD-05 -- Independent Metric Source

The host adapter SHALL load `gateway-releases` from an independent metric source identifier `gateway-release-distribution`.

A failure in gateway list aggregation or adapter validation SHALL:

- Omit `gateway-releases` from the returned `metrics` array
- Record `gateway-release-distribution` in `failedSources` when at least one other source succeeds
- NOT synthesize zero or placeholder distribution entries

A failure in Prometheus gateway metrics or sandbox metrics SHALL NOT prevent `gateway-releases` from loading when the REST aggregation succeeds.

The adapter SHALL honor `AbortSignal` cancellation from `DashboardControlPlane.getOperationalMetrics`.

#### Scenario: Gateway metrics failure does not hide release distribution

- GIVEN `GET /api/hypershell/v1/gateways` succeeds for a dashboard operator
- AND the `gateway-metrics` Prometheus source fails
- WHEN the operator opens `/dashboard`
- THEN the Gateway releases widget SHALL display loaded version rows
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

The adapter SHALL NOT apply additional per-gateway filtering beyond what the List API enforces for the authenticated caller.

#### Scenario: Dashboard operator sees fleet-wide distribution

- GIVEN the signed-in user has `platform:admin`
- AND the fleet contains gateways the user does not individually own
- WHEN release distribution metrics load
- THEN the distribution SHALL include those gateways

---

### Requirement: GRD-07 -- Verification

The web-console adapter SHALL include unit tests in `gateway-release-distribution-aggregation.test.ts` for:

- Version bucketing (`bucketGatewayVersion`, including omitted, null, and whitespace-only versions that fall back to `unknown`, and trimming of surrounding whitespace)
- Aggregation across multiple list pages
- Metric mapping from aggregation output (`buildGatewayReleasesMetric`)
- Inconsistent-list rejection and independent source failure behavior in `createDashboardControlPlaneAdapter` (gateway list failure omits `gateway-releases` and records `gateway-release-distribution` in `failedSources`)

The operational dashboard package SHALL include unit tests in `gateway-release-distribution-data.test.ts` for descending-count sort order and zero-count omission of version list entries via `buildGatewayReleaseListEntries`.

`mockOperationalDashboardMetrics` SHALL include a representative `gateway-releases` metric (including an `unknown` bucket). Storybook fixtures MAY render that metric without calling the HyperShell API.

---

## Non-Goals

- Provisioning summary card aggregating median, P95, and success rate (remaining HYPERSHELL-280 scope)
- Version distribution trend graphs
- Canary rollout percentage or version health scoring
- Replacing the Usage summary sandboxes row (sandbox totals remain on `provisioned-sandboxes` via OP-DASH-06)
