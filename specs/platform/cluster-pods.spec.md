# Cluster Pods

**Status:** Active
**Applies to:** `deploy/base` and `deploy/kind` Prometheus scrape configuration, `components/web-console` BFF and dashboard adapter, `packages/operational-dashboard-ui`

## Purpose

Expose **cluster pod utilization** for the Kubernetes cluster that hosts the HyperShell platform (the **hub cluster**) on the operational dashboard so dashboard operators can see how many pod slots are in use and how many remain available.

This is **cluster infrastructure telemetry**, not HyperShell application state. It reflects pod scheduling pressure on the hub cluster where the API server, control plane, Prometheus, gateway namespaces, and related platform workloads run. It is **not** per-gateway sandbox counts (`provisioned-sandboxes`), per-tenant namespace quotas, or pod inventory on managed clusters registered in a Fleet.

Pod semantics for version 1:

| Concept | Meaning |
| --- | --- |
| **Capacity** | Total allocatable pod slots across schedulable hub-cluster nodes |
| **Used** | Pods currently present on the hub cluster at collection time |
| **Available** | Unscheduled pod slots (`capacity - used`) |

The operational dashboard `pods` widget and `system-summary` row already exist as placeholders. This spec connects them to live hub-cluster data.

### Relationship to other specifications

- **Operational dashboard** (`web-console/operational-dashboard.spec.md`) owns the `pods` widget, `UtilizationChart` presentation (OP-DASH-13), refresh policy (OP-DASH-09), and dashboard-operator access (OP-DASH-04).
- **Cluster memory** and **cluster CPU** (`platform/cluster-memory.spec.md`, `platform/cluster-cpu.spec.md`) follow the same Prometheus/BFF/adapter pattern for utilization widgets. Cluster pods uses **kube-state-metrics** (not node-exporter) and exposes a separate BFF route and `pods` metric mapping.
- **Provisioned sandboxes** (`platform/openshell-gateway-sandbox-count.spec.md`) counts gateway sandbox pods for product telemetry. Cluster pods counts **all** hub-cluster pods and is unrelated to sandbox lifecycle.
- **Registered users** (`platform/registered-users.spec.md`) sources HyperShell REST APIs instead of Prometheus.
- **Gateway metrics dashboard** (`platform/gateway-metrics-dashboard.spec.md`) uses a different BFF route (`GET /api/metrics/gateways`) and is unrelated to hub-cluster pod capacity.

Node inventory and provision-time metrics are out of scope for this spec (see Non-Goals).

## Requirements

### Requirement: CLP-01 -- Hub Cluster Scope

Cluster pod metrics SHALL aggregate **schedulable nodes** and **all namespaces** on the hub Kubernetes cluster where HyperShell platform components are deployed.

The metric SHALL include pods scheduled on nodes that can accept workload pods. Control-plane nodes MAY be included when they are schedulable in the target environment (for example, single-node Kind clusters).

The metric SHALL count platform, system, gateway, and tenant-namespace pods on the hub cluster as a single aggregate.

The metric SHALL NOT be scoped to a single gateway namespace, a single HyperShell microservice Deployment, or managed clusters registered in a Fleet.

#### Scenario: Kind hub cluster reports pod utilization

- GIVEN HyperShell is running on a Kind cluster with one control-plane node
- WHEN cluster pod metrics are collected
- THEN the capacity figure SHALL reflect that node's allocatable pod slots
- AND the used figure SHALL reflect the current pod count on that cluster

---

### Requirement: CLP-02 -- Pod Measurement Contract

The platform SHALL compute three non-negative integer values in **pods** (schedulable pod slots / running workload objects):

| Field | Definition |
| --- | --- |
| `capacity_pods` | Sum of allocatable pod slots across in-scope nodes |
| `used_pods` | Count of all pod objects on the hub cluster at collection time (all phases) |
| `available_pods` | `capacity_pods - used_pods` |

`used_pods` SHALL be derived from kube-state-metrics pod series (see CLP-03). Values SHALL be whole numbers; the BFF SHALL NOT round fractional samples (Prometheus results SHALL be integral).

Version 1 SHALL count **every** pod object toward `used_pods`, including `Failed` and `Succeeded` pods, while those objects still exist in the API. Failed pods remain in `used_pods` because they typically still hold a schedulable pod slot on a node until deleted.

All three values SHALL use the same collection timestamp (Prometheus instant query evaluation time).

`used_pods` SHALL NOT exceed `capacity_pods`. When `capacity_pods` is zero (no scrape data), the collection SHALL be treated as a failure (CLP-06).

The BFF SHALL also collect per-phase pod counts (`phase_pending_pods`, `phase_running_pods`, `phase_succeeded_pods`, `phase_failed_pods`, `phase_unknown_pods`) from kube-state-metrics. The sum of phase counts SHALL equal `used_pods`; otherwise collection SHALL fail.

#### Scenario: Used and available sum to capacity

- GIVEN `capacity_pods` is `2000` and `used_pods` is `548`
- WHEN the BFF formats the response
- THEN `available_pods` SHALL be `1452`
- AND `used_pods` SHALL remain `548`

---

### Requirement: CLP-03 -- Prometheus Data Source

Hub-cluster pod utilization SHALL be derived from Prometheus instant queries executed by the web-console BFF against the configured `PROMETHEUS_URL` (same origin configuration as `platform/gateway-metrics-dashboard.spec.md` DASH-05).

Version 1 SHALL use **kube-state-metrics** pod and node series scraped into the hub Prometheus stack (CLP-06). Canonical PromQL expressions SHALL be documented in `packages/operational-dashboard-ui/DATA_SOURCES.md` during implementation. Version 1 baseline:

| Measurement | PromQL (baseline) |
| --- | --- |
| Capacity pods | `sum(kube_node_status_allocatable{resource="pods"})` |
| Used pods | `count(kube_pod_info)` |
| Pending pods | `sum(kube_pod_status_phase{phase="Pending"})` |
| Running pods | `sum(kube_pod_status_phase{phase="Running"})` |
| Succeeded pods | `sum(kube_pod_status_phase{phase="Succeeded"})` |
| Failed pods | `sum(kube_pod_status_phase{phase="Failed"})` |
| Unknown pods | `sum(kube_pod_status_phase{phase="Unknown"})` |

PromQL queries SHALL aggregate with `sum(...)` or `count(...)` across in-scope nodes and namespaces so a single capacity/used pair is returned per evaluation.

The BFF SHALL NOT contact the Kubernetes API directly for pod counts in version 1.

#### Scenario: Prometheus unavailable fails collection

- GIVEN Prometheus is unreachable from the BFF within the query timeout
- WHEN a dashboard operator loads operational metrics
- THEN the BFF pods route SHALL respond with HTTP `502`
- AND the operational metrics workflow SHALL fail (CLP-07)

---

### Requirement: CLP-04 -- BFF Cluster Pods Route

The web-console BFF SHALL expose `GET /api/metrics/cluster-pods` as a same-origin route that:

1. Requires **dashboard-operator authorization** when OIDC is enabled (same role gate as `web-console/operational-dashboard.spec.md` OP-DASH-04: `hypershell-admins` or `platform:admin`)
2. Executes Prometheus instant queries for hub-cluster pods (CLP-03)
3. Returns JSON:

```json
{
  "capacity_pods": 2000,
  "available_pods": 1452,
  "used_pods": 548,
  "phase_pending_pods": 12,
  "phase_running_pods": 500,
  "phase_succeeded_pods": 20,
  "phase_failed_pods": 16,
  "phase_unknown_pods": 0
}
```

The route SHALL NOT forward to the HyperShell API server and SHALL NOT require a HyperShell API bearer token.

On Prometheus failure, timeout, or non-success Prometheus response status, the BFF SHALL respond with HTTP `502` and `{ "error": "Metrics unavailable", "statusCode": 502 }`. The BFF SHALL NOT return zeroed pod figures as a fallback.

#### Scenario: Dashboard administrator receives pod counts

- GIVEN OIDC is enabled and the caller has `hypershell-admins` or `platform:admin`
- AND Prometheus returns successful instant-query results
- WHEN the caller sends `GET /api/metrics/cluster-pods`
- THEN the BFF SHALL respond with HTTP `200` and the CLP-04 JSON body

#### Scenario: Authenticated non-admin is rejected

- GIVEN OIDC is enabled and the caller has only `hypershell-users`
- WHEN the caller sends `GET /api/metrics/cluster-pods`
- THEN the BFF SHALL respond with HTTP `403`

#### Scenario: Unauthenticated caller is rejected

- GIVEN OIDC is enabled and the caller has no session
- WHEN the caller sends `GET /api/metrics/cluster-pods`
- THEN the BFF SHALL respond with HTTP `401` or the standard BFF re-authentication response used by other protected routes

---

### Requirement: CLP-05 -- Operational Dashboard Metric Mapping

The operational dashboard host adapter (`createDashboardControlPlaneAdapter`) SHALL fetch `GET /api/metrics/cluster-pods` with same-origin credentials and emit an `OperationalMetric`:

| Field | Value |
| --- | --- |
| `id` | `"pods"` |
| `value` | Decimal string of **used** pods (`used_pods`) |
| `total` | Decimal string of **capacity** pods (`capacity_pods`) |
| `unit` | `"pods"` |
| `podPhases.pending` | `phase_pending_pods` |
| `podPhases.running` | `phase_running_pods` |
| `podPhases.succeeded` | `phase_succeeded_pods` |
| `podPhases.failed` | `phase_failed_pods` |
| `podPhases.unknown` | `phase_unknown_pods` |

The adapter SHALL NOT emit `trend` or `status` for the `pods` metric in version 1.

The `system-summary` card SHALL continue to source its pods row from the same `pods` metric (OP-DASH-13). The summary row SHALL show utilization percentage because `unit` and `total` are present. When `podPhases.failed` is non-zero, the pods row SHALL show a failed count with a danger status icon below the utilization value (same presentation as gateway and node exception counts).

The `pods` widget SHALL render `PodCapacityChart` (OP-DASH-17), not `UtilizationChart`.

Available pods are derivable in the UI as `total - value` and SHALL be shown as an **Unused** segment in the capacity donut (gray).

#### Scenario: Dashboard pods widget shows capacity and phase breakdown

- GIVEN `used_pods` is `548` and `capacity_pods` is `2000`
- AND phase counts sum to `548` (`running: 500`, `pending: 12`, `succeeded: 20`, `failed: 16`, `unknown: 0`)
- WHEN an authorized dashboard operator opens `/dashboard` and metrics load successfully
- THEN the `pods` metric SHALL have `value: "548"`, `total: "2000"`, `unit: "pods"`, and `podPhases` matching the BFF phase fields
- AND the `pods` widget SHALL render a capacity donut with phase segments plus an Unused segment of `1452`
- AND the chart center title SHALL show `548`
- AND the chart subtitle SHALL read "of 2000 pods"

#### Scenario: Available pods are implied by capacity minus used

- GIVEN the `pods` metric has `value: "548"` and `total: "2000"`
- WHEN the operator reads the capacity donut subtitle ("of 2000 pods")
- THEN the implied available pods SHALL be `1452`
- AND the Unused segment SHALL represent `1452` pods

---

### Requirement: CLP-06 -- Prometheus Scrape Prerequisites

Cluster pods SHALL require **kube-state-metrics** scraped into the hub Prometheus stack. Unlike cluster memory and CPU (node-exporter), pod and node allocatable series are not available from the existing node-exporter DaemonSet alone.

Version 1 SHALL deploy kube-state-metrics (or an equivalent collector exposing compatible `kube_pod_info` and `kube_node_status_allocatable` series) with a `ServiceMonitor` selected by the hub Prometheus instance.

`DATA_SOURCES.md` SHALL document the CLP-03 PromQL expressions and note the kube-state-metrics dependency.

#### Scenario: Fresh Kind install exposes pod series

- GIVEN a developer runs `make kind-up` with the documented Prometheus overlay and kube-state-metrics
- WHEN an operator queries Prometheus for the documented capacity expression
- THEN Prometheus SHALL return a positive `capacity_pods` sample for the hub cluster

---

### Requirement: CLP-07 -- Refresh and Error Semantics

Cluster pods SHALL load through the existing operational dashboard metrics query (`useGetMetricsData`) and SHALL inherit its refresh policy (`operationalDashboardRefreshMilliseconds`, currently 15 minutes) and manual refresh behavior (OP-DASH-09).

A failed `GET /api/metrics/cluster-pods` request SHALL fail the entire `getOperationalMetrics` workflow. The dashboard SHALL NOT display `0` pods as a fallback used or capacity figure.

#### Scenario: Pods query failure surfaces dashboard error

- GIVEN Prometheus is down
- WHEN the operator opens `/dashboard`
- THEN the dashboard SHALL show its localized load-error state
- AND the `pods` widget SHALL NOT silently show zero utilization

---

### Requirement: CLP-08 -- Verification

The web-console BFF SHALL include unit tests for:

- Successful JSON mapping from mocked Prometheus responses
- HTTP `502` when Prometheus fails
- Session requirement when OIDC is enabled
- Rejection when `used_pods` exceeds `capacity_pods` or `capacity_pods` is zero
- Rejection when phase counts do not sum to `used_pods`

The web console SHALL include unit tests for the dashboard adapter mapping `used_pods`, `capacity_pods`, and phase fields into the `pods` `OperationalMetric`.

The operational dashboard package SHALL update `mockOperationalDashboardMetrics` only as needed for Storybook fixtures (values may remain representative).

#### Scenario: CI exercises adapter mapping

- GIVEN a mocked BFF response with `used_pods: 548`, `capacity_pods: 2000`, and phase fields summing to `548`
- WHEN dashboard adapter unit tests run
- THEN they SHALL assert the `pods` metric `id`, `value: "548"`, `total: "2000"`, `unit: "pods"`, and `podPhases`

## Non-Goals

- Memory, CPU, and node inventory metrics (see `platform/cluster-nodes.spec.md`)
- Gateway sandbox pod counts (`provisioned-sandboxes` on the operational dashboard)
- Historical trend series or sparklines for pods in version 1
- Per-node, per-namespace, or per-workload pod breakdown in the UI
- Pod metrics for registered **managed clusters** or individual gateway tenant namespaces
- A HyperShell REST `/cluster_pods` OpenAPI resource in version 1
- Direct browser access to Prometheus
- Combining pods with CPU or memory into a single BFF route in version 1
