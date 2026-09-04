# Cluster CPU

**Status:** Active
**Applies to:** `deploy/base` and `deploy/kind` Prometheus scrape configuration, `components/web-console` BFF and dashboard adapter, `packages/operational-dashboard-ui`

## Purpose

Expose **cluster CPU utilization** for the Kubernetes cluster that hosts the HyperShell platform (the **hub cluster**) on the operational dashboard so dashboard operators can see how many CPU cores are in use and how many remain available.

This is **cluster infrastructure telemetry**, not HyperShell application state. It reflects schedulable CPU capacity on hub-cluster nodes where the API server, control plane, Prometheus, and related platform workloads run. It is **not** per-gateway sandbox CPU, per-tenant namespace quotas, or per-pod CPU limits.

CPU semantics for version 1:

| Concept | Meaning |
| --- | --- |
| **Capacity** | Total logical CPU cores on in-scope hub-cluster nodes |
| **Used** | Cores currently busy (non-idle), derived from node CPU time counters |
| **Available** | Cores not in use at collection time (`capacity - used`) |

The operational dashboard `cpu` widget and `system-summary` row already exist as placeholders. This spec connects them to live hub-cluster data.

### Relationship to other specifications

- **Operational dashboard** (`web-console/operational-dashboard.spec.md`) owns the `cpu` widget, `UtilizationChart` presentation (OP-DASH-13), refresh policy (OP-DASH-09), and dashboard-operator access (OP-DASH-04).
- **Cluster memory** (`platform/cluster-memory.spec.md`) follows the same Prometheus/BFF/adapter pattern for the `memory` widget. Cluster CPU reuses the same node-exporter scrape targets but exposes a separate BFF route and `cpu` metric mapping.
- **Registered users** (`platform/registered-users.spec.md`) sources HyperShell REST APIs instead of Prometheus.
- **Gateway metrics dashboard** (`platform/gateway-metrics-dashboard.spec.md`) uses a different BFF route (`GET /api/metrics/gateways`) for gateway phase counts and is unrelated to hub-cluster CPU.

Pod capacity and provision-time metrics are out of scope for this spec (see Non-Goals).

## Requirements

### Requirement: CC-01 -- Hub Cluster Scope

Cluster CPU metrics SHALL aggregate **schedulable nodes** on the hub Kubernetes cluster where HyperShell platform components are deployed.

The metric SHALL include CPU from nodes that can accept workload pods. Control-plane nodes MAY be included when they are schedulable in the target environment (for example, single-node Kind clusters).

The metric SHALL NOT be scoped to individual gateway tenant namespaces, individual HyperShell microservice Deployments, or managed clusters registered in a Fleet.

#### Scenario: Kind hub cluster reports node CPU

- GIVEN HyperShell is running on a Kind cluster with one control-plane node
- WHEN cluster CPU metrics are collected
- THEN the capacity figure SHALL reflect that node's logical CPU core count
- AND the used figure SHALL reflect non-idle CPU time on that node

---

### Requirement: CC-02 -- CPU Measurement Contract

The platform SHALL compute three non-negative numeric values in **cores** (logical CPUs):

| Field | Definition |
| --- | --- |
| `capacity_cores` | Sum of logical CPU cores across in-scope nodes |
| `used_cores` | Sum of non-idle CPU core equivalents across in-scope nodes at collection time |
| `available_cores` | `capacity_cores - used_cores` |

`used_cores` SHALL be derived from Prometheus `rate()` over `node_cpu_seconds_total` with `mode!="idle"` (see CC-03). Values MAY be fractional because CPU utilization is a rate; the BFF SHALL NOT round before returning JSON.

All three values SHALL use the same node set and the same collection timestamp (Prometheus instant query evaluation time).

`used_cores` SHALL NOT exceed `capacity_cores` by more than a small floating-point tolerance (0.01 cores). When `capacity_cores` is zero (no scrape data), the collection SHALL be treated as a failure (CC-06).

#### Scenario: Used and available sum to capacity

- GIVEN `capacity_cores` is `60` and `used_cores` is `48.2`
- WHEN the BFF formats the response
- THEN `available_cores` SHALL be `11.8`
- AND `used_cores` SHALL remain `48.2`

---

### Requirement: CC-03 -- Prometheus Data Source

Hub-cluster CPU SHALL be derived from Prometheus instant queries executed by the web-console BFF against the configured `PROMETHEUS_URL` (same origin configuration as `platform/gateway-metrics-dashboard.spec.md` DASH-05).

Version 1 SHALL use node-exporter `node_cpu_seconds_total` series already scraped for cluster memory (CM-06). Canonical PromQL expressions SHALL be documented in `packages/operational-dashboard-ui/DATA_SOURCES.md` during implementation. Version 1 baseline:

| Measurement | PromQL (baseline) |
| --- | --- |
| Capacity cores | `sum(count by (instance) (node_cpu_seconds_total{mode="idle"}))` |
| Used cores | `sum(rate(node_cpu_seconds_total{mode!="idle"}[5m]))` |

PromQL queries SHALL aggregate with `sum(...)` across in-scope nodes so a single capacity/used pair is returned per evaluation.

The BFF SHALL NOT contact the Kubernetes API directly for CPU in version 1.

#### Scenario: Prometheus unavailable fails collection

- GIVEN Prometheus is unreachable from the BFF within the query timeout
- WHEN a dashboard operator loads operational metrics
- THEN the BFF CPU route SHALL respond with HTTP `502`
- AND the operational metrics workflow SHALL fail (CC-07)

---

### Requirement: CC-04 -- BFF Cluster CPU Route

The web-console BFF SHALL expose `GET /api/metrics/cluster-cpu` as a same-origin route that:

1. Requires **dashboard-operator authorization** when OIDC is enabled (same role gate as `web-console/operational-dashboard.spec.md` OP-DASH-04: `hypershell-admins` or `platform:admin`)
2. Executes Prometheus instant queries for hub-cluster CPU (CC-03)
3. Returns JSON:

```json
{
  "capacity_cores": 60,
  "available_cores": 11.8,
  "used_cores": 48.2
}
```

The route SHALL NOT forward to the HyperShell API server and SHALL NOT require a HyperShell API bearer token.

On Prometheus failure, timeout, or non-success Prometheus response status, the BFF SHALL respond with HTTP `502` and `{ "error": "Metrics unavailable", "statusCode": 502 }`. The BFF SHALL NOT return zeroed CPU figures as a fallback.

#### Scenario: Dashboard administrator receives CPU cores

- GIVEN OIDC is enabled and the caller has `hypershell-admins` or `platform:admin`
- AND Prometheus returns successful instant-query results
- WHEN the caller sends `GET /api/metrics/cluster-cpu`
- THEN the BFF SHALL respond with HTTP `200` and the CC-04 JSON body

#### Scenario: Authenticated non-admin is rejected

- GIVEN OIDC is enabled and the caller has only `hypershell-users`
- WHEN the caller sends `GET /api/metrics/cluster-cpu`
- THEN the BFF SHALL respond with HTTP `403`

#### Scenario: Unauthenticated caller is rejected

- GIVEN OIDC is enabled and the caller has no session
- WHEN the caller sends `GET /api/metrics/cluster-cpu`
- THEN the BFF SHALL respond with HTTP `401` or the standard BFF re-authentication response used by other protected routes

---

### Requirement: CC-05 -- Operational Dashboard Metric Mapping

The operational dashboard host adapter (`createDashboardControlPlaneAdapter`) SHALL fetch `GET /api/metrics/cluster-cpu` with same-origin credentials and emit an `OperationalMetric`:

| Field | Value |
| --- | --- |
| `id` | `"cpu"` |
| `value` | Decimal string of **used** cores, rounded to the nearest whole core (`round(used_cores)`) |
| `total` | Decimal string of **capacity** cores, rounded to the nearest whole core (`round(capacity_cores)`) |
| `unit` | `"cores"` |

The adapter SHALL NOT emit `trend` or `status` for the `cpu` metric in version 1.

The `system-summary` card SHALL continue to source its CPU row from the same `cpu` metric (OP-DASH-13). The utilization donut SHALL render because `unit` and `total` are present.

Available cores are derivable in the UI as `total - value` (using the rounded display values) and SHALL NOT be duplicated as a separate metric ID in version 1.

#### Scenario: Dashboard CPU widget shows utilization

- GIVEN `used_cores` is `48.2` and `capacity_cores` is `60`
- WHEN an authorized dashboard operator opens `/dashboard` and metrics load successfully
- THEN the `cpu` metric SHALL have `value: "48"`, `total: "60"`, and `unit: "cores"`
- AND the `cpu` widget SHALL render a utilization donut at approximately 80%

#### Scenario: Available cores are implied by capacity minus used

- GIVEN the `cpu` metric has `value: "48"` and `total: "60"`
- WHEN the operator reads the utilization subtitle ("of 60 cores")
- THEN the implied available cores SHALL be `12`

---

### Requirement: CC-06 -- Prometheus Scrape Prerequisites

Cluster CPU SHALL reuse the hub-cluster node-exporter scrape configuration established for cluster memory (CM-06). No additional scrape targets are required beyond those needed for `node_cpu_seconds_total`.

`DATA_SOURCES.md` SHALL document the CC-03 PromQL expressions and note that CPU and memory share the same node-exporter DaemonSet.

#### Scenario: Fresh Kind install exposes CPU series

- GIVEN a developer runs `make kind-up` with the documented Prometheus overlay and node-exporter
- WHEN an operator queries Prometheus for the documented capacity expression
- THEN Prometheus SHALL return a positive `capacity_cores` sample for the hub cluster

---

### Requirement: CC-07 -- Refresh and Error Semantics

Cluster CPU SHALL load through the existing operational dashboard metrics query (`useGetMetricsData`) and SHALL inherit its refresh policy (`operationalDashboardRefreshMilliseconds`, currently 15 minutes) and manual refresh behavior (OP-DASH-09).

A failed `GET /api/metrics/cluster-cpu` request SHALL fail the entire `getOperationalMetrics` workflow. The dashboard SHALL NOT display `0` cores as a fallback used or capacity figure.

#### Scenario: CPU query failure surfaces dashboard error

- GIVEN Prometheus is down
- WHEN the operator opens `/dashboard`
- THEN the dashboard SHALL show its localized load-error state
- AND the `cpu` widget SHALL NOT silently show zero utilization

---

### Requirement: CC-08 -- Verification

The web-console BFF SHALL include unit tests for:

- Successful JSON mapping from mocked Prometheus responses (including fractional `used_cores`)
- HTTP `502` when Prometheus fails
- Session requirement when OIDC is enabled

The web console SHALL include unit tests for the dashboard adapter mapping `used_cores` and `capacity_cores` into the `cpu` `OperationalMetric`.

The operational dashboard package SHALL update `mockOperationalDashboardMetrics` only as needed for Storybook fixtures (values may remain representative).

#### Scenario: CI exercises adapter mapping

- GIVEN a mocked BFF response with `used_cores: 48.2` and `capacity_cores: 60`
- WHEN dashboard adapter unit tests run
- THEN they SHALL assert the `cpu` metric `id`, `value: "48"`, `total: "60"`, and `unit: "cores"`

## Non-Goals

- Memory, pod utilization (see `platform/cluster-pods.spec.md`), node count, and provision-time metrics (covered by or deferred to other specs)
- Historical trend series or sparklines for CPU
- Per-node or per-namespace CPU breakdown in the UI
- CPU metrics for registered **managed clusters** or gateway tenant namespaces
- A HyperShell REST `/cluster_cpu` OpenAPI resource in version 1
- Direct browser access to Prometheus
- Combining CPU and memory into a single BFF route in version 1 (separate routes keep failure domains and contracts clear)
