# Cluster Nodes

**Status:** Active
**Applies to:** `deploy/base` and `deploy/kind` Prometheus scrape configuration, `components/web-console` BFF and dashboard adapter, `packages/operational-dashboard-ui`

## Purpose

Expose **hub cluster node inventory** for the Kubernetes cluster that hosts the HyperShell platform on the operational dashboard so dashboard operators can see how many nodes exist and how many are Ready.

This is **cluster infrastructure telemetry**, not HyperShell application state. It reflects Kubernetes `Node` objects on the hub cluster where the API server, control plane, Prometheus, gateway namespaces, and related platform workloads run. It is **not** per-gateway sandbox placement, ManagedCluster registration counts, or node inventory on remote clusters registered in a Fleet.

Node semantics for version 1:

| Concept | Meaning |
| --- | --- |
| **Total** | All Kubernetes nodes registered on the hub cluster |
| **Ready** | Nodes whose `Ready` condition is `True` |
| **Not ready** | Nodes that exist but are not Ready (`total - ready`) |

The operational dashboard `system-summary` row and `nodes` widget render node inventory using the same total + `status` presentation model as `provisioned-gateways` (OP-DASH-07): `value` is the total count and `status` carries ready/not-ready buckets.

### Relationship to other specifications

- **Operational dashboard** (`web-console/operational-dashboard.spec.md`) owns the `nodes` system-summary row, refresh policy (OP-DASH-09), and dashboard-operator access (OP-DASH-04). Node presentation SHALL follow the gateway total + `status` pattern (OP-DASH-07, OP-DASH-13).
- **Cluster memory**, **cluster CPU**, and **cluster pods** (`platform/cluster-memory.spec.md`, `platform/cluster-cpu.spec.md`, `platform/cluster-pods.spec.md`) follow the same Prometheus/BFF/adapter pattern but expose **utilization** metrics with `unit` and `total`. Cluster nodes exposes **inventory + health buckets** (`value` + `status`), not utilization.
- **Provisioned gateways** (`web-console/operational-dashboard.spec.md` OP-DASH-07) is the presentation reference: total in `value`, per-bucket counts in `status`, exception icons in summary rows when `failed` or `degraded` counts are non-zero.
- **Cluster pods** (`platform/cluster-pods.spec.md`) reuses the same kube-state-metrics scrape target deployed for pod capacity series.
- **Registered users** (`platform/registered-users.spec.md`) sources HyperShell REST APIs instead of Prometheus.
- **Gateway metrics dashboard** (`platform/gateway-metrics-dashboard.spec.md`) is unrelated to hub-cluster node inventory.

Provision-time metrics and per-node CPU/memory breakdown are out of scope for this spec (see Non-Goals).

## Requirements

### Requirement: CLN-01 -- Hub Cluster Scope

Cluster node metrics SHALL count **all Kubernetes Node objects** on the hub cluster where HyperShell platform components are deployed.

The count SHALL include control-plane nodes, worker nodes, and any other node roles present in the hub cluster API (for example, a single-node Kind control-plane node).

The metric SHALL NOT be scoped to individual gateway tenant namespaces, individual HyperShell microservice Deployments, or managed clusters registered in a Fleet.

#### Scenario: Kind hub cluster reports one node

- GIVEN HyperShell is running on a Kind cluster with one control-plane node
- WHEN cluster node metrics are collected
- THEN `total_nodes` SHALL be `1`
- AND `ready_nodes` SHALL be `1` when that node is Ready

---

### Requirement: CLN-02 -- Node Measurement Contract

The platform SHALL compute three non-negative integer values in **nodes** (Kubernetes Node objects):

| Field | Definition |
| --- | --- |
| `total_nodes` | Count of all Node objects on the hub cluster |
| `ready_nodes` | Count of nodes whose `Ready` condition is `True` |
| `not_ready_nodes` | `total_nodes - ready_nodes` |

Values SHALL be whole numbers derived from kube-state-metrics node series (see CLN-03). The BFF SHALL NOT round fractional Prometheus samples (results SHALL be integral).

All three values SHALL use the same collection timestamp (Prometheus instant query evaluation time).

`ready_nodes` SHALL NOT exceed `total_nodes`. When `total_nodes` is zero (no scrape data), the collection SHALL be treated as a failure (CLN-06).

#### Scenario: Not-ready nodes are implied by total minus ready

- GIVEN `total_nodes` is `8` and `ready_nodes` is `7`
- WHEN the BFF formats the response
- THEN `not_ready_nodes` SHALL be `1`
- AND `ready_nodes` SHALL remain `7`

---

### Requirement: CLN-03 -- Prometheus Data Source

Hub-cluster node inventory SHALL be derived from Prometheus instant queries executed by the web-console BFF against the configured `PROMETHEUS_URL` (same origin configuration as `platform/gateway-metrics-dashboard.spec.md` DASH-05).

Version 1 SHALL use **kube-state-metrics** node series scraped into the hub Prometheus stack (CLN-06). Canonical PromQL expressions SHALL be documented in `packages/operational-dashboard-ui/DATA_SOURCES.md` during implementation. Version 1 baseline:

| Measurement | PromQL (baseline) |
| --- | --- |
| Total nodes | `count(kube_node_info)` |
| Ready nodes | `sum(kube_node_status_condition{condition="Ready",status="true"})` |

The BFF SHALL NOT contact the Kubernetes API directly for node counts in version 1.

#### Scenario: Prometheus unavailable fails collection

- GIVEN Prometheus is unreachable from the BFF within the query timeout
- WHEN a dashboard operator loads operational metrics
- THEN the BFF nodes route SHALL respond with HTTP `502`
- AND the operational metrics workflow SHALL fail (CLN-07)

---

### Requirement: CLN-04 -- BFF Cluster Nodes Route

The web-console BFF SHALL expose `GET /api/metrics/cluster-nodes` as a same-origin route that:

1. Requires **dashboard-operator authorization** when OIDC is enabled (same role gate as `web-console/operational-dashboard.spec.md` OP-DASH-04: `hypershell-admins` or `platform:admin`)
2. Executes Prometheus instant queries for hub-cluster nodes (CLN-03)
3. Returns JSON:

```json
{
  "total_nodes": 8,
  "ready_nodes": 8,
  "not_ready_nodes": 0
}
```

The route SHALL NOT forward to the HyperShell API server and SHALL NOT require a HyperShell API bearer token.

On Prometheus failure, timeout, non-success Prometheus response status, or inconsistent samples (`ready_nodes > total_nodes`), the BFF SHALL respond with HTTP `502` and `{ "error": "Metrics unavailable", "statusCode": 502 }`. The BFF SHALL NOT return zeroed node figures as a fallback.

#### Scenario: Dashboard administrator receives node counts

- GIVEN OIDC is enabled and the caller has `hypershell-admins` or `platform:admin`
- AND Prometheus returns successful instant-query results
- WHEN the caller sends `GET /api/metrics/cluster-nodes`
- THEN the BFF SHALL respond with HTTP `200` and the CLN-04 JSON body

#### Scenario: Authenticated non-admin is rejected

- GIVEN OIDC is enabled and the caller has only `hypershell-users`
- WHEN the caller sends `GET /api/metrics/cluster-nodes`
- THEN the BFF SHALL respond with HTTP `403`

#### Scenario: Unauthenticated caller is rejected

- GIVEN OIDC is enabled and the caller has no session
- WHEN the caller sends `GET /api/metrics/cluster-nodes`
- THEN the BFF SHALL respond with HTTP `401` or the standard BFF re-authentication response used by other protected routes

---

### Requirement: CLN-05 -- Operational Dashboard Metric Mapping

The operational dashboard host adapter (`createDashboardControlPlaneAdapter`) SHALL fetch `GET /api/metrics/cluster-nodes` with same-origin credentials and emit an `OperationalMetric` using the same total + `status` shape as `provisioned-gateways` (OP-DASH-07):

| Field | Value |
| --- | --- |
| `id` | `"nodes"` |
| `value` | Decimal string of **total** nodes (`total_nodes`) |
| `status.healthy` | `ready_nodes` |
| `status.failed` | `not_ready_nodes` |

The adapter SHALL NOT emit `status.provisioning`, `status.degraded`, `unit`, `total`, or `trend` for the `nodes` metric in version 1.

`status.healthy` plus `status.failed` SHALL equal `total_nodes` for every successful response.

The `system-summary` card SHALL source its nodes row from the same `nodes` metric (OP-DASH-13). The row SHALL render the total count and, when `status.failed` is greater than zero, SHALL show the same exception-status icon treatment used for gateways in summary rows.

When the `nodes` metric is rendered as a status donut (`NodeStatusChart`, OP-DASH-16), ready nodes SHALL appear under the localized **Ready** label and not-ready nodes under **Not ready**, while still using `status.healthy` and `status.failed` in the adapter contract.

#### Scenario: Dashboard system-summary shows total node count when all nodes are ready

- GIVEN `total_nodes` is `8` and `ready_nodes` is `8`
- WHEN an authorized dashboard operator opens `/dashboard` and metrics load successfully
- THEN the `nodes` metric SHALL have `value: "8"` and `status: { healthy: 8, failed: 0 }`
- AND the `system-summary` nodes row SHALL display `8` without exception status icons

#### Scenario: Not-ready nodes appear in status.failed

- GIVEN `total_nodes` is `8` and `ready_nodes` is `7`
- WHEN metrics load successfully
- THEN the `nodes` metric SHALL have `value: "8"` and `status: { healthy: 7, failed: 1 }`
- AND the `system-summary` nodes row SHALL display `8` with a failed-count indicator for `1` not-ready node

---

### Requirement: CLN-06 -- Prometheus Scrape Prerequisites

Cluster nodes SHALL require **kube-state-metrics** scraped into the hub Prometheus stack. Node inventory series are not available from the node-exporter DaemonSet alone.

Version 1 SHALL reuse the kube-state-metrics Deployment and `ServiceMonitor` deployed for cluster pods (`platform/cluster-pods.spec.md` CLP-06). No additional scrape target is required beyond compatible `kube_node_info` and `kube_node_status_condition` series.

`DATA_SOURCES.md` SHALL document the CLN-03 PromQL expressions and note the kube-state-metrics dependency.

#### Scenario: Fresh Kind install exposes node series

- GIVEN a developer runs `make kind-up` with the documented Prometheus overlay and kube-state-metrics
- WHEN an operator queries Prometheus for the documented total-nodes expression
- THEN Prometheus SHALL return a positive `total_nodes` sample for the hub cluster

---

### Requirement: CLN-07 -- Refresh and Error Semantics

Cluster nodes SHALL load through the existing operational dashboard metrics query (`useGetMetricsData`) and SHALL inherit its refresh policy (`operationalDashboardRefreshMilliseconds`, currently 15 minutes) and manual refresh behavior (OP-DASH-09).

A failed `GET /api/metrics/cluster-nodes` request SHALL fail the entire `getOperationalMetrics` workflow. The dashboard SHALL NOT display `0` nodes as a fallback count.

#### Scenario: Nodes query failure surfaces dashboard error

- GIVEN Prometheus is down
- WHEN the operator opens `/dashboard`
- THEN the dashboard SHALL show its localized load-error state
- AND the `system-summary` nodes row SHALL NOT silently show zero

---

### Requirement: CLN-08 -- Verification

The web-console BFF SHALL include unit tests for:

- Successful JSON mapping from mocked Prometheus responses
- HTTP `502` when Prometheus fails
- Session requirement when OIDC is enabled
- Rejection when `ready_nodes` exceeds `total_nodes` or `total_nodes` is zero

The web console SHALL include unit tests for the dashboard adapter mapping `total_nodes`, `ready_nodes`, and `not_ready_nodes` into the `nodes` `OperationalMetric` `value` and `status` fields.

The operational dashboard package SHALL update `mockOperationalDashboardMetrics` to include representative `status` on the `nodes` metric for Storybook fixtures.

#### Scenario: CI exercises adapter mapping

- GIVEN a mocked BFF response with `total_nodes: 8`, `ready_nodes: 7`, and `not_ready_nodes: 1`
- WHEN dashboard adapter unit tests run
- THEN they SHALL assert the `nodes` metric `id`, `value: "8"`, and `status: { healthy: 7, failed: 1 }`

## Non-Goals

- CPU, memory, and pod utilization metrics (covered by other cluster specs)
- Per-node labels, roles, zones, or provider metadata in the UI
- `provisioning` or `degraded` node buckets in version 1 (only `healthy` and `failed` are mapped)
- A standalone `nodes` donut chart widget in version 1 (only the `system-summary` row is in scope)
- Historical trend series or sparklines for nodes in version 1
- Node metrics for registered **managed clusters**
- Cordoned/unschedulable breakdown separate from Ready condition in version 1
- A HyperShell REST `/cluster_nodes` OpenAPI resource in version 1
- Direct browser access to Prometheus
- Combining nodes with other cluster metrics into a single BFF route in version 1
