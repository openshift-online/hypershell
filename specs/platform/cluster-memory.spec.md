# Cluster Memory

**Status:** Active
**Applies to:** `deploy/base` and `deploy/kind` Prometheus scrape configuration, `components/web-console` BFF and dashboard adapter, `packages/operational-dashboard-ui`

## Purpose

Expose **cluster memory utilization** for the Kubernetes cluster that hosts the HyperShell platform (the **hub cluster**) on the operational dashboard so dashboard operators can see how much memory is in use and how much remains available.

This is **cluster infrastructure telemetry**, not HyperShell application state. It reflects worker-node memory on the hub cluster where the API server, control plane, Prometheus, and related platform workloads run. It is **not** per-gateway sandbox memory, per-tenant namespace quotas, or Keycloak/database memory.

Memory semantics for version 1:

| Concept | Meaning |
| --- | --- |
| **Capacity** | Total memory installed on schedulable hub-cluster nodes |
| **Used** | Memory currently in use on those nodes (capacity minus available) |
| **Available** | Memory immediately available for new workloads (`capacity - used`) |

The operational dashboard `memory` widget and `system-summary` row already exist as placeholders. This spec connects them to live hub-cluster data.

### Relationship to other specifications

- **Operational dashboard** (`web-console/operational-dashboard.spec.md`) owns the `memory` widget, `UtilizationChart` presentation (OP-DASH-13), refresh policy (OP-DASH-09), and dashboard-operator access (OP-DASH-04).
- **Registered users** (`platform/registered-users.spec.md`) follows the same dashboard-adapter pattern but sources HyperShell REST APIs. Cluster memory sources **Prometheus** through a BFF proxy because there is no durable HyperShell `Memory` resource in the API server database.
- **Gateway metrics dashboard** (`platform/gateway-metrics-dashboard.spec.md`) also uses a BFF Prometheus proxy (`GET /api/metrics/gateways`) for a different UI (`GatewayMetricsDashboard`). The operational dashboard does **not** reuse that route; it needs aggregate used/capacity bytes, not gateway phase counts.
- **Gateway list metrics** on `/dashboard` (`provisioned-gateways`, etc.) remain REST-driven and are unrelated.

CPU, pod capacity, and node inventory are out of scope for this spec (see Non-Goals).

## Requirements

### Requirement: CM-01 -- Hub Cluster Scope

Cluster memory metrics SHALL aggregate **schedulable nodes** on the hub Kubernetes cluster where HyperShell platform components are deployed.

The metric SHALL include memory from nodes that can accept workload pods. Control-plane nodes MAY be included when they are schedulable in the target environment (for example, single-node Kind clusters).

The metric SHALL NOT be scoped to individual gateway tenant namespaces, individual HyperShell microservice Deployments, or managed clusters registered in a Fleet.

#### Scenario: Kind hub cluster reports node memory

- GIVEN HyperShell is running on a Kind cluster with one control-plane node
- WHEN cluster memory metrics are collected
- THEN the capacity figure SHALL reflect that node's total memory
- AND the used figure SHALL reflect memory in use on that node

---

### Requirement: CM-02 -- Memory Measurement Contract

The platform SHALL compute three non-negative integer values in **bytes**:

| Field | Definition |
| --- | --- |
| `capacity_bytes` | Sum of total installed memory across in-scope nodes |
| `available_bytes` | Sum of memory immediately available for allocation across in-scope nodes |
| `used_bytes` | `capacity_bytes - available_bytes` |

All three values SHALL use the same node set and the same collection timestamp (Prometheus instant query).

`used_bytes` SHALL NOT exceed `capacity_bytes`. When `capacity_bytes` is zero (no scrape data), the collection SHALL be treated as a failure (CM-06).

#### Scenario: Used and available sum to capacity

- GIVEN `capacity_bytes` is `17179869184` and `available_bytes` is `4294967296`
- WHEN the BFF formats the response
- THEN `used_bytes` SHALL be `12884901888`
- AND `available_bytes` SHALL remain `4294967296`

---

### Requirement: CM-03 -- Prometheus Data Source

Hub-cluster memory SHALL be derived from Prometheus instant queries executed by the web-console BFF against the configured `PROMETHEUS_URL` (same origin configuration as `platform/gateway-metrics-dashboard.spec.md` DASH-05).

The deployed Prometheus instance SHALL scrape node-level memory series sufficient to evaluate the measurement contract in CM-02. Version 1 SHALL document the canonical PromQL expressions in `packages/operational-dashboard-ui/DATA_SOURCES.md` once scrape targets are chosen during implementation.

PromQL queries SHALL aggregate with `sum(...)` across in-scope nodes so a single capacity/available pair is returned per scrape.

The BFF SHALL NOT contact the Kubernetes API directly for memory in version 1.

#### Scenario: Prometheus unavailable fails collection

- GIVEN Prometheus is unreachable from the BFF within the query timeout
- WHEN a dashboard operator loads operational metrics
- THEN the BFF memory route SHALL respond with HTTP `502`
- AND the operational metrics workflow SHALL fail (CM-07)

---

### Requirement: CM-04 -- BFF Cluster Memory Route

The web-console BFF SHALL expose `GET /api/metrics/cluster-memory` as a same-origin route that:

1. Requires **dashboard-operator authorization** when OIDC is enabled (same role gate as `web-console/operational-dashboard.spec.md` OP-DASH-04: `hypershell-admins` or `platform:admin`)
2. Executes Prometheus instant queries for hub-cluster memory (CM-03)
3. Returns JSON:

```json
{
  "capacity_bytes": 17179869184,
  "available_bytes": 4294967296,
  "used_bytes": 12884901888
}
```

The route SHALL NOT forward to the HyperShell API server and SHALL NOT require a HyperShell API bearer token.

On Prometheus failure, timeout, or non-success Prometheus response status, the BFF SHALL respond with HTTP `502` and `{ "error": "Metrics unavailable", "statusCode": 502 }`. The BFF SHALL NOT return zeroed memory figures as a fallback.

#### Scenario: Dashboard administrator receives memory bytes

- GIVEN OIDC is enabled and the caller has `hypershell-admins` or `platform:admin`
- AND Prometheus returns successful instant-query results
- WHEN the caller sends `GET /api/metrics/cluster-memory`
- THEN the BFF SHALL respond with HTTP `200` and the CM-04 JSON body

#### Scenario: Authenticated non-admin is rejected

- GIVEN OIDC is enabled and the caller has only `hypershell-users`
- WHEN the caller sends `GET /api/metrics/cluster-memory`
- THEN the BFF SHALL respond with HTTP `403`

#### Scenario: Unauthenticated caller is rejected

- GIVEN OIDC is enabled and the caller has no session
- WHEN the caller sends `GET /api/metrics/cluster-memory`
- THEN the BFF SHALL respond with HTTP `401` or the standard BFF re-authentication response used by other protected routes

---

### Requirement: CM-05 -- Operational Dashboard Metric Mapping

The operational dashboard host adapter (`createDashboardControlPlaneAdapter`) SHALL fetch `GET /api/metrics/cluster-memory` with same-origin credentials and emit an `OperationalMetric`:

| Field | Value |
| --- | --- |
| `id` | `"memory"` |
| `value` | Decimal string of **used** memory in **gibibytes (GiB)**, rounded to the nearest whole GiB (`round(used_bytes / 1024³)`) |
| `total` | Decimal string of **capacity** memory in GiB, same rounding |
| `unit` | `"GiB"` |

The adapter SHALL NOT emit `trend` or `status` for the `memory` metric in version 1.

The `system-summary` card SHALL continue to source its memory row from the same `memory` metric (OP-DASH-13). The utilization donut SHALL render because `unit` and `total` are present.

Available memory in GiB is derivable in the UI as `total - value` and SHALL NOT be duplicated as a separate metric ID in version 1.

#### Scenario: Dashboard memory widget shows utilization

- GIVEN `used_bytes` is `23622320128` and `capacity_bytes` is `25480396800` (approximately 220 GiB used of 237 GiB capacity)
- WHEN an authorized dashboard operator opens `/dashboard` and metrics load successfully
- THEN the `memory` metric SHALL have `value: "220"`, `total: "237"`, and `unit: "GiB"`
- AND the `memory` widget SHALL render a utilization donut at approximately 93%

#### Scenario: Available memory is implied by capacity minus used

- GIVEN the `memory` metric has `value: "220"` and `total: "237"`
- WHEN the operator reads the utilization subtitle ("of 237 GiB")
- THEN the implied available memory SHALL be `17` GiB

---

### Requirement: CM-06 -- Prometheus Scrape Prerequisites

The HyperShell deployment manifests SHALL ensure the in-cluster Prometheus instance can scrape node memory metrics on the hub cluster before cluster memory is considered production-ready.

At minimum, the Prometheus `ClusterRole` (or equivalent scrape credentials) and scrape configuration SHALL allow computing CM-02 on Kind and production hub clusters without manual post-install steps.

`DATA_SOURCES.md` and reconcile wave notes SHALL record the chosen scrape target (for example, node-exporter DaemonSet, kubelet/cAdvisor, or an equivalent platform-supported source).

#### Scenario: Fresh Kind install exposes memory series

- GIVEN a developer runs `make kind-up` with the documented Prometheus overlay
- WHEN an operator queries Prometheus for the documented capacity expression
- THEN Prometheus SHALL return a positive `capacity_bytes` sample for the hub cluster

---

### Requirement: CM-07 -- Refresh and Error Semantics

Cluster memory SHALL load through the existing operational dashboard metrics query (`useGetMetricsData`) and SHALL inherit its refresh policy (`operationalDashboardRefreshMilliseconds`, currently 15 minutes) and manual refresh behavior (OP-DASH-09).

A failed `GET /api/metrics/cluster-memory` request SHALL fail the entire `getOperationalMetrics` workflow. The dashboard SHALL NOT display `0` GiB as a fallback used or capacity figure.

#### Scenario: Memory query failure surfaces dashboard error

- GIVEN Prometheus is down
- WHEN the operator opens `/dashboard`
- THEN the dashboard SHALL show its localized load-error state
- AND the `memory` widget SHALL NOT silently show zero utilization

---

### Requirement: CM-08 -- Verification

The web-console BFF SHALL include unit tests for:

- Successful JSON mapping from mocked Prometheus responses
- HTTP `502` when Prometheus fails
- Session requirement when OIDC is enabled

The web console SHALL include unit tests for the dashboard adapter mapping `used_bytes` and `capacity_bytes` into the `memory` `OperationalMetric`.

The operational dashboard package SHALL update `mockOperationalDashboardMetrics` only as needed for Storybook fixtures (values may remain representative).

#### Scenario: CI exercises adapter mapping

- GIVEN a mocked BFF response with known byte values
- WHEN dashboard adapter unit tests run
- THEN they SHALL assert the `memory` metric `id`, `value`, `total`, and `unit`

## Non-Goals

- CPU, pod count, node count, and provision-time metrics (separate future specs)
- Historical trend series or sparklines for memory
- Per-node or per-namespace memory breakdown in the UI
- Memory metrics for registered **managed clusters** or gateway tenant namespaces
- A HyperShell REST `/cluster_memory` OpenAPI resource in version 1
- Direct browser access to Prometheus
