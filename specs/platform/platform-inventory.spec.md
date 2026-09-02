# Platform Inventory

**Status:** Active
**Applies to:** `components/api-server` managed cluster and managed database list APIs, `components/api-server/pkg/rbac`, `components/sdk-typescript`, `components/web-console` dashboard adapter, `packages/operational-dashboard-ui`

**Tracks:** [HYPERSHELL-278](https://redhat.atlassian.net/browse/HYPERSHELL-278)

## Purpose

Expose **platform inventory counts** — totals and breakdowns for HyperShell infrastructure resources registered in the API server — on the operational dashboard so administrators can assess platform footprint at a glance.

Version 1 sources inventory from existing paginated List APIs:

- `GET /api/hypershell/v1/managed_clusters`
- `GET /api/hypershell/v1/managed_databases`

This specification defines aggregation rules, dashboard-operator authorization, operational dashboard metric IDs, and the **Inventory summary** presentation. It does **not** add usage trend graphs or new inventory APIs.

### Fleet resource exclusion

HyperShell removed the top-level Fleet resource and `fleet_id` scoping from all kinds (`platform/data-model.spec.md`). There is no `GET /api/hypershell/v1/fleets` endpoint. Fleet totals and fleet creation windows from HYPERSHELL-278 are **out of scope** for this specification.

### Relationship to other specifications

| Concern | Platform inventory (this spec) | Operational dashboard |
| --- | --- | --- |
| Data source | HyperShell REST list APIs (RBAC-filtered) | Widget layout, refresh, access control |
| Managed cluster placement UI | Gateway provisioning uses bounded list search (`web-console/architecture.spec.md`) | Inventory adapter paginates the full collection for aggregates |
| Hub cluster nodes | Unrelated (Prometheus kube-state-metrics; `platform/cluster-nodes.spec.md`) | `nodes` widget counts hub-cluster Kubernetes nodes, not ManagedCluster registrations |
| Registered users | Same list-total pattern (`platform/registered-users.spec.md`) | Both appear on the operational overview |

Prometheus gateway metrics (`platform/gateway-metrics-dashboard.spec.md`) are unrelated.

## Requirements

### Requirement: PI-01 -- Inventory Resource Scope

Platform inventory version 1 SHALL cover:

| Resource | List endpoint | Aggregated fields |
| --- | --- | --- |
| ManagedCluster | `GET /api/hypershell/v1/managed_clusters` | `total`, `status`, `provider`, `region`, `created_at` |
| ManagedDatabase | `GET /api/hypershell/v1/managed_databases` | `total`, `status`, `created_at` |

The adapter SHALL NOT invent counts from gateway `cluster_id` or `database_id` references. Only registered ManagedCluster and ManagedDatabase records returned by their List APIs SHALL contribute to inventory metrics.

Historical trend series (`OperationalMetric.trend`) SHALL NOT be loaded in version 1.

#### Scenario: Inventory excludes gateway-only references

- GIVEN a gateway references a `cluster_id` that no longer exists as a ManagedCluster row
- WHEN platform inventory metrics load
- THEN the managed cluster total SHALL reflect only ManagedCluster list items
- AND the gateway reference SHALL NOT increment the count

---

### Requirement: PI-02 -- Paginated List Aggregation

The web-console dashboard adapter SHALL load managed cluster and managed database inventory by paginating each List API through the browser TypeScript SDK with page size `100`, ordered by `name asc`, until all pages are retrieved.

The adapter SHALL validate each list response for internal consistency (requested page number, `total`, and `items` length). An inconsistent response SHALL fail only the `platform-inventory` metric source; other metric sources SHALL still be attempted (`web-console/operational-dashboard.spec.md` OP-DASH-19).

For each resource kind the adapter SHALL compute:

| Aggregate | Rule |
| --- | --- |
| `total` | List `total` after the final page (MUST match the number of aggregated items) |
| `status` buckets | Group by each item's `status` field; omitted or null `status` SHALL bucket as `unknown` |
| `provider` buckets (clusters only) | Group by each item's `provider` field; omitted or null `provider` SHALL bucket as `unknown` |
| `region` buckets (clusters only) | Group by each item's `region` field; omitted or null `region` SHALL bucket as `unknown` |
| `created_last_30_days` | Count items whose `created_at` is greater than or equal to the start of the 30-day lookback window |

The 30-day lookback window SHALL be **30 × 24 hours** ending at adapter evaluation time (UTC). Items with missing or unparsable `created_at` SHALL be excluded from the recent count but SHALL still contribute to `total` and breakdown buckets.

The adapter SHALL NOT issue per-resource Get requests. Inventory aggregation SHALL use at most one paginated List sequence per resource kind per metrics refresh.

#### Scenario: Multiple pages are aggregated

- GIVEN the caller can list 150 managed clusters
- WHEN `getOperationalMetrics` runs
- THEN the adapter SHALL issue two paginated managed cluster list requests
- AND the `managed-clusters` metric `value` SHALL be `"150"`

#### Scenario: Inconsistent pagination omits inventory metrics

- GIVEN a managed database list response reports `page: 2` when page `1` was requested
- AND at least one other metric source succeeds
- WHEN the adapter processes the response
- THEN the `platform-inventory` source SHALL be treated as failed
- AND `managed-clusters` and `managed-databases` SHALL be omitted from the adapter response
- AND the dashboard SHALL NOT synthesize zero counts for those metrics
- AND the dashboard SHALL NOT enter the total load-error state

---

### Requirement: PI-03 -- Dashboard-Operator Authorization

Managed cluster and managed database List endpoints SHALL be readable by **dashboard operators**, matching the operational dashboard audience (`web-console/operational-dashboard.spec.md` OP-DASH-04) and registered user inventory (`platform/registered-users.spec.md` RU-03):

- Caller holds an effective `platform:admin` RoleBinding (including JWT-synced realm role), **or**
- Caller presents a JWT whose `realm_access.roles` includes `hypershell-admins`, **or**
- Caller holds an effective `gateway:creator` RoleBinding (existing behavior)

All other callers SHALL be denied List access with HTTP `403`.

The RBAC middleware SHALL treat `managed_clusters` and `managed_databases` collection List (`GET` with empty resource ID) with the same dashboard-inventory access helper used for `users`. Singleton Get authorization for these resources MAY retain the existing `gateway:creator` requirement.

#### Scenario: Hypershell admin without gateway:creator can list clusters

- GIVEN a caller presents a JWT with `hypershell-admins` and no `gateway:creator` binding
- WHEN the caller sends `GET /api/hypershell/v1/managed_clusters`
- THEN the API SHALL respond with HTTP `200`

#### Scenario: Gateway owner without creator cannot list clusters

- GIVEN a caller with only `gateway:owner` on one gateway
- WHEN the caller sends `GET /api/hypershell/v1/managed_clusters`
- THEN the API SHALL respond with HTTP `403`

---

### Requirement: PI-04 -- Status and Dimension Mapping

Inventory breakdowns SHALL use **exact** API field values as bucket keys after trimming leading and trailing ASCII whitespace. Mapping SHALL NOT reinterpret lifecycle phases from other resources.

| Field | Bucket key when absent | Display label rule |
| --- | --- | --- |
| `status` | `unknown` | Localize `unknown` as **Unknown**; otherwise show the API value verbatim |
| `provider` (clusters) | `unknown` | Show the API value verbatim (providers are operator-defined strings such as `aws`, `gcp`, `ibm`, `openshift`) |
| `region` (clusters) | `unknown` | Placement donut keys SHALL be `{region} ({provider})` using PI-04 bucket keys for each field (for example `us-east-1 (aws)`, `unknown (openshift)`) |

Status donut widgets SHALL render only buckets with count greater than zero. When more than five distinct non-zero status buckets exist for a resource kind, the status donut widget for that kind SHALL NOT render segments (the widget shell remains; the inventory summary still shows the total).

Provider breakdown SHALL appear in the `managed-cluster-providers` donut widget (PI-07). Region breakdown SHALL appear in the `managed-cluster-regions` donut widget (PI-07).

#### Scenario: Null status aggregates to unknown

- GIVEN two managed databases where one has `status: "Ready"` and one omits `status`
- WHEN inventory metrics are computed
- THEN the `managed-databases` metric `inventoryStatus` SHALL include buckets `Ready: 1` and `unknown: 1`
- AND the inventory summary total SHALL be `2`

---

### Requirement: PI-05 -- Operational Dashboard Metrics

The host `DashboardControlPlane` adapter SHALL populate these connected metrics:

| Metric ID | `value` | Additional fields |
| --- | --- | --- |
| `managed-clusters` | Total managed cluster count (decimal string) | `inventoryStatus`, `createdLast30Days`, `inventoryProviders`, `inventoryRegions` |
| `managed-databases` | Total managed database count (decimal string) | `inventoryStatus` |

Metrics SHALL NOT include `trend`, `unit`, `total`, or gateway-style `status` buckets (`healthy`, `provisioning`, `degraded`, `failed`) in version 1.

`OperationalMetric` SHALL gain optional fields:

- `inventoryStatus?: Record<string, number>` — per-status bucket counts keyed by PI-04 bucket keys (for example `Ready`, `unknown`)
- `inventoryProviders?: Record<string, number>` — per-provider bucket counts keyed by PI-04 bucket keys (for example `aws`, `kind`, `unknown`)
- `inventoryRegions?: Record<string, number>` — per-placement bucket counts keyed as `{region} ({provider})` using PI-04 bucket keys (for example `us-east-1 (aws)`, `unknown (openshift)`)
- `createdLast30Days?: string` — managed clusters created within the PI-02 lookback window

Inventory presentation code SHALL read status breakdowns from `inventoryStatus`, not from `OperationalMetric.status`.

Provider donut widgets SHALL read breakdowns from `inventoryProviders`. Region donut widgets SHALL read breakdowns from `inventoryRegions`. Legend entries for dimension donuts SHALL be ordered by descending count then ascending `label`.

#### Scenario: Provider donut legend is ordered by count

- GIVEN managed clusters aggregate to `aws: 5`, `gcp: 5`, `ibm: 2`, `openshift: 1`
- WHEN the `managed-cluster-providers` widget renders
- THEN legend entries SHALL appear in order `aws: 5`, `gcp: 5`, `ibm: 2`, `openshift: 1`
- AND `inventoryProviders` on the metric SHALL include all four buckets

---

### Requirement: PI-06 -- Inventory Summary Card

The operational dashboard SHALL add an `inventory-summary` widget that renders an `InventorySummaryCard` DescriptionList sourced from the `managed-clusters` and `managed-databases` metrics.

The card SHALL include these rows:

| Row | Source |
| --- | --- |
| Clusters | `managed-clusters.value` |
| Clusters created (30 days) | `managed-clusters.createdLast30Days` |
| Databases | `managed-databases.value` |

When a managed cluster or database metric exposes non-zero failure or warning `inventoryStatus` buckets, the summary SHALL show an exception row beneath the total using the same danger/warning icon semantics as gateway and node summary rows: any bucket whose label matches `/fail/i` uses the danger icon; any bucket whose label matches `/degrad/i` or `/pending/i` uses the warning icon. Healthy-only inventories (no matching failure or warning buckets) SHALL omit the exception status row, including when multiple non-exception status buckets exist.

When a metric is absent from the adapter response, the corresponding inventory summary rows SHALL render the localized **Metric unavailable** empty-state message for that row group (not a silent zero).

The default layout template SHALL place the inventory summary on the operational overview (`/dashboard`) without creating a separate Platform inventory route in version 1.

#### Scenario: Inventory summary shows totals

- GIVEN inventory metrics loaded with `managed-clusters.value: "8"`, `createdLast30Days: "2"`, and `managed-databases.value: "3"`
- WHEN the inventory summary widget renders
- THEN the card SHALL list cluster total `8`, recent count `2`, and database total `3`

---

### Requirement: PI-07 -- Optional Inventory Detail Widgets

The widget catalog SHALL add these types:

| Widget type | Metric ID | Default layout | Presentation |
| --- | --- | --- | --- |
| `managed-cluster-providers` | `managed-clusters` | Yes | Provider donut from `inventoryProviders`; legend ordered by descending count (PI-05) |
| `managed-cluster-regions` | `managed-clusters` | Yes | Placement donut from `inventoryRegions` (`{region} ({provider})` keys); legend ordered by descending count (PI-05) |
| `managed-clusters` | `managed-clusters` | No | `MetricCard` large number |
| `managed-cluster-status` | `managed-clusters` | No | Status donut when ≤5 non-zero status buckets (PI-04) |
| `managed-databases` | `managed-databases` | No | `MetricCard` large number |
| `managed-database-status` | `managed-databases` | Yes | Status donut when ≤5 non-zero status buckets (PI-04) |

Users MAY add optional widgets from the add-widgets drawer. Status and provider donut widgets SHALL omit sparklines.

A dedicated **Platform inventory** dashboard route (`/dashboard/inventory`) SHALL NOT be introduced in version 1. Future work MAY add that route when the inventory summary exceeds eight rows or multiple full-width breakdown charts are required.

#### Scenario: Status donut is suppressed for many statuses

- GIVEN managed clusters have six distinct non-zero `status` values
- WHEN the `managed-cluster-status` widget renders
- THEN the donut area SHALL render nothing
- AND the widget title bar SHALL remain

---

### Requirement: PI-08 -- Refresh and Error Semantics

Platform inventory metrics SHALL load through the existing operational dashboard metrics query (`useGetMetricsData`) and SHALL inherit its refresh policy (`operationalDashboardRefreshMilliseconds`, currently 15 minutes) and manual refresh behavior (`web-console/operational-dashboard.spec.md` OP-DASH-09).

A failed managed cluster or managed database List request SHALL fail only the `platform-inventory` metric source (`managed-clusters`, `managed-databases`). The adapter SHALL NOT synthesize zero, empty, or placeholder values for failed inventory metrics. When at least one other metric source succeeds, the dashboard SHALL render available metrics and show inventory widgets in the localized metric-unavailable state (`web-console/operational-dashboard.spec.md` OP-DASH-08, OP-DASH-19).

`getOperationalMetrics` SHALL throw only when every metric source fails or when the request is aborted.

`AbortSignal` cancellation SHALL propagate to in-flight list requests.

#### Scenario: Unauthorized inventory list omits inventory metrics

- GIVEN the signed-in user lacks dashboard-operator API authorization for managed clusters
- AND at least one other metric source succeeds
- WHEN the host adapter calls `GET /api/hypershell/v1/managed_clusters`
- THEN the `platform-inventory` source SHALL be treated as failed
- AND inventory widgets SHALL render the localized metric-unavailable state
- AND a warning `Alert` SHALL explain that some metrics could not be loaded

---

### Requirement: PI-09 -- Documentation and Verification

`packages/operational-dashboard-ui/DATA_SOURCES.md` SHALL document `managed-clusters` and `managed-databases` metric sources, pagination, aggregation rules, and the inventory summary widget.

The web console SHALL include unit tests for the dashboard adapter that cover:

- Full pagination aggregation
- `unknown` bucketing for omitted `status`, `provider`, and `region`
- `created_last_30_days` counting with a fixed clock or injected timestamps
- Provider and region bucket mapping into `inventoryProviders` and `inventoryRegions`
- Metric ID and field mapping into `OperationalDashboardMetrics`

The API server SHALL include RBAC tests for dashboard-operator List access to `managed_clusters` and `managed_databases`.

The operational dashboard package SHALL extend `mockOperationalDashboardMetrics` with representative inventory fields and add Storybook coverage for the inventory summary widget.

#### Scenario: CI exercises adapter mapping

- GIVEN a managed cluster list fixture with two pages and mixed field presence
- WHEN dashboard adapter unit tests run
- THEN they SHALL assert the stringified total, `inventoryStatus` buckets, `createdLast30Days`, `inventoryProviders`, and `inventoryRegions` mapping
