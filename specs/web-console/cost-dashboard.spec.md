# Cost Dashboard

**Status:** Draft
**Version:** 0.4.0
**Created:** 2026-11-26
**Changed:** 0.2.0 adds the per-user "My cost" view and namespace-based workspace attribution. 0.3.0 charges every gateway to a single **billing owner**, the oldest current owner (decided 2026-11-27). 0.4.0 records implementation choices: JSON rate card file, `COST_PROMETHEUS_*` and `COST_CLUSTERS` settings, the four hub recording rules, and the extended mockups
**Applies to:** `packages/operational-dashboard-ui`, `components/web-console` SPA and BFF, hub Prometheus recording rules (gitops `bases/observability`, `clusters/hypc-ibm-infra/.../metrics-hub`)
**Tracks:** _TBD_

## Purpose

Provide a widgetized cost dashboard at `/dashboard/cost` with two views:

- **Platform view** (`platform:admin` only): a **directional estimate** of what
  the platform costs to run, where that cost is concentrated (environment,
  cluster, gateway, sandbox), and how much of it is tied to idle capacity.
- **My cost view** (every authenticated user): the estimated cost of the
  gateways the signed-in user owns, and of the workloads on gateways they can
  access, broken down by gateway and workspace.

Estimates are derived from resource **requests** already collected in Prometheus,
multiplied by a configurable **rate card**. They are for capacity planning and
prioritisation. They are not billing-grade and are not reconciled against a cloud
invoice (see Non-goals).

The dashboard follows the same BFF-to-Prometheus architecture as the usage and
operational dashboards. The browser calls same-origin BFF routes; the BFF queries
Prometheus, applies the rate card, and returns JSON. The browser never queries
Prometheus and never receives the rate card.

## Scope

- Provide `/dashboard/cost` to every authenticated user. Administrators see the
  platform view by default and can switch to My cost; other users see only My
  cost.
- Associate cost to users through existing ownership data: per-gateway
  RoleBindings and OpenShell workspace namespaces (see "Attribution to users").
- Reuse the operational dashboard's widget components, secondary navigation,
  7/30-day time-range controls, 15-minute refresh, stale-value preservation, and
  localized unavailable states.
- Load a rate card from deployment configuration.
- Estimate cost from CPU and memory **requests** against node **allocatable**
  capacity, per cluster and per namespace.
- Attribute namespace cost to gateways through the `openshell-<id>` namespace.
- Render the widgets in the approved mockup:
  - cost summary (estimated monthly total)
  - idle capacity cost
  - cost per active user
  - cost per gateway
  - cost per sandbox
  - cost by environment
  - cost by cluster
  - cost by resource class (gateways, sandboxes, platform overhead)
- Explain the model in an in-page "About these estimates" section.

## Non-goals

- Billing-grade accuracy, invoices, or chargeback to customers.
- Pulling actual spend from IBM Cloud or AWS billing APIs (a later phase can use
  the existing `cost-center` / `app-code` / `service-phase` tags to reconcile).
- LLM / Vertex AI token spend (tracked separately).
- Storage, network egress, load-balancer, and managed-service (Postgres, Secrets
  Manager) costs in this version. They are listed as known gaps.
- Usage-based (actual CPU seconds) costing. This version prices what is
  **reserved** (requests), because reserved capacity is what drives node count.
- Per-sandbox attribution to the user who created it. OpenShell does not record
  a sandbox creator today; this is a later phase (see "Attribution to users",
  level 3).
- Showing a non-admin the cost of gateways they cannot access, platform
  overhead, idle cluster capacity, or rate card values.

## Cost model

### Rate card

The rate card prices one unit of reserved capacity per hour, per provider and
environment. It is deployment configuration, not code, and is not shipped in the
public image.

```json
{
  "currency": "USD",
  "version": "2026-11-26",
  "defaults": { "cpu_core_hour": 0.031, "memory_gib_hour": 0.0042 },
  "providers": {
    "ibm": { "cpu_core_hour": 0.034, "memory_gib_hour": 0.0046 },
    "aws": { "cpu_core_hour": 0.031, "memory_gib_hour": 0.0042 }
  },
  "clusters": { "<cluster-label>": { "cpu_core_hour": 0.04 } },
  "fixed_monthly": { "<cluster-label>": 0 }
}
```

The file named by `COST_RATE_CARD_FILE` is JSON (which is also valid YAML), so
the BFF needs no YAML parser. `clusters` overrides are keyed by the `cluster`
label; `fixed_monthly` adds flat costs per cluster (control plane, managed
cluster fee), pro-rated over the range. The BFF re-reads the file when it
changes, so a mounted ConfigMap update applies without a restart.

Resolution order for a cluster: `clusters.<cluster>`, then `providers.<provider>`, then
`defaults`, per resource. Rates are illustrative in this draft and SHALL be supplied by the
deployment.

### Inputs (already remote-written to the hub)

| Input                             | Series                                                      | Labels used                                  |
| --------------------------------- | ----------------------------------------------------------- | -------------------------------------------- |
| Allocatable CPU / memory per node | `kube_node_status_allocatable{resource=~"cpu\|memory"}`     | `cluster`, `environment`, `provider`, `node` |
| Requested CPU per namespace       | `namespace_cpu:kube_pod_container_resource_requests:sum`    | `cluster`, `environment`, `namespace`        |
| Requested memory per namespace    | `namespace_memory:kube_pod_container_resource_requests:sum` | `cluster`, `environment`, `namespace`        |
| Active sandboxes                  | `gateway_sandbox_total`, API-server active-sandbox gauge    | `namespace` / `gateway_namespace`            |
| Gateway inventory                 | Gateway API (as in WEB-USE-07)                              | `namespace`                                  |
| Active gateways and users         | existing usage + adoption sources                           | -                                            |

These series are already in the `bases/observability/stack/cluster-capacity.yaml`
federation list, so v1 needs **no new remote-written series**.

### Calculations

All amounts are computed per hour and scaled to the selected range; the monthly
headline uses a 730-hour month.

- **Cluster capacity cost** = Σ nodes (allocatable cores × cpu rate + allocatable
  GiB × memory rate) + `fixed_monthly` pro-rated.
- **Namespace cost** = requested cores × cpu rate + requested GiB × memory rate.
- **Idle capacity cost** = cluster capacity cost − Σ namespace cost on that
  cluster (capacity that nodes provide but nothing reserves).
- **Gateway namespaces** = the gateway's API-assigned namespace `openshell-<id>`
  plus, in `managed` workspace mode (the default), every per-workspace namespace
  `openshell-<id>-<workspace>` (OSG-CORE-18, OSG-SBX-01).
- **Gateway server cost** = namespace cost of `openshell-<id>`.
- **Workspace cost** = namespace cost of `openshell-<id>-<workspace>`. In
  `managed` mode sandbox pods run here, so this is the sandbox cost of that
  workspace.
- **Gateway cost** = gateway server cost + Σ workspace cost. For a `shared`-mode
  gateway, sandboxes run in `openshell-<id>` and cannot be split from the server
  in v1. `operator`-mode workspace namespaces are owned by an external operator
  and are excluded unless they carry the gateway's namespace label.
- **Unused gateway cost** = Σ gateway cost of gateways classified as unused by
  WEB-USE-07. Shown inside the idle widget as its own line.
- **Platform overhead** = namespace cost of everything that is not a gateway
  namespace (HyperShell control plane, observability, operators, platform).
- **Cost per gateway** = Σ gateway cost / current gateway count.
- **Cost per sandbox** = Σ workspace cost / mean active sandboxes over the range
  (managed-mode gateways only).
- **Cost by billing owner** = Σ gateway cost grouped by each gateway's billing
  owner (see "Attribution to users"). Sums to the total gateway cost.
- **Cost per active user** = total estimated cost / active users in the range
  (adoption dashboard definition).

Hub recording rules SHALL precompute the hourly cluster and namespace
**reserved quantities** (not money), for example
`hypershell:namespace_cpu_requests:cores` and
`hypershell:cluster_cpu_allocatable:cores`, so the BFF query cost stays
O(clusters + gateway namespaces) and does not grow with pods. Pricing happens in
the BFF so that a rate card change applies to history without rewriting series.

The hub SHALL record these four series, labelled by `cluster`, `environment`,
and `provider` (plus `namespace` for the namespace rules):

| Recording rule                                | Expression                                                                                                                               |
| --------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------- |
| `hypershell:cluster_cpu_allocatable:cores`    | `sum by (cluster, environment, provider) (max by (cluster, environment, provider, node) (kube_node_status_allocatable{resource="cpu"}))` |
| `hypershell:cluster_memory_allocatable:bytes` | same, `resource="memory"`                                                                                                                |
| `hypershell:namespace_cpu_requests:cores`     | `max by (cluster, environment, provider, namespace) (namespace_cpu:kube_pod_container_resource_requests:sum)`                            |
| `hypershell:namespace_memory_requests:bytes`  | `max by (cluster, environment, provider, namespace) (namespace_memory:kube_pod_container_resource_requests:sum)`                         |

The BFF integrates them over the range with `sum_over_time(rule[<N>h:1h])`
(core-hours and byte-hours) and scales to a month with 730 / covered hours.
Coverage starts at the first hourly sample in the range; a later start than the
range start is reported as partial coverage.

### Deployment settings

| Variable                                                | Meaning                                                                                                                                                                         |
| ------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `COST_RATE_CARD_FILE`                                   | Path of the rate card. Unset: every widget shows "cost model not configured".                                                                                                   |
| `COST_PROMETHEUS_URL`                                   | Prometheus that holds the recording rules (the hub). Defaults to `PROMETHEUS_URL`. HTTPS is required outside local development, as for `PROMETHEUS_URL`.                        |
| `COST_PROMETHEUS_TOKEN_FILE`, `COST_PROMETHEUS_CA_FILE` | Bearer token and CA bundle for `COST_PROMETHEUS_URL`.                                                                                                                           |
| `COST_CLUSTERS`                                         | Optional comma-separated `cluster` label values that belong to this HyperShell instance. When set, every cost query is limited to them, so one hub can serve several instances. |

## Attribution to users

Cost is measured on Kubernetes namespaces. Users are linked to namespaces through
ownership data that HyperShell and OpenShell already keep. There are three
levels, from coarse to fine. v1 uses levels 1 and 2.

### Level 1 -- Gateway, through RoleBindings (exists)

Every Gateway has per-gateway RoleBindings (`security/rbac-enforcement.spec.md`,
`platform/gateway-access-management.spec.md`):

| Role                    | Meaning                                | Cost treatment                                |
| ----------------------- | -------------------------------------- | --------------------------------------------- |
| `gateway:owner`, oldest | the billing owner (see below)          | **Charged** for every workload on the gateway |
| `gateway:owner`, others | one or more further owners; can delete | Visible as "shared with me", not charged      |
| `gateway:admin`         | granted administrator, no delete       | Visible as "shared with me", not charged      |
| `gateway:viewer`        | read-only user                         | Visible as "shared with me", not charged      |

The gateway's namespaces are fixed by its ID (PLAT-DATA-03, OSG-CORE-18), so
`user → RoleBinding → gateway → openshell-<id>[-<workspace>] → cost` needs no new
labels. This is the same authorized-gateway lookup the Usage dashboard already
performs (WEB-USE-03).

**Billing owner (decided 2026-11-27).** Each gateway SHALL be charged, in full,
to exactly one user: its **billing owner**, the user holding the current
`gateway:owner` RoleBinding with the earliest `created_at` (ties broken by the
lowest RoleBinding ID). Everything on the gateway is charged to that user: the
gateway server, every workspace namespace, and every sandbox, whoever created it.
Because each gateway has exactly one billing owner, **per-user totals add up** to
the sum of all gateway cost. This replaces the 2026-11-26 rule that each owner sees
the full cost.

Properties that follow from existing rules:

- A gateway always has a billing owner, because the access facade never lets a
  gateway lose its last owner (GTW-ACC-07).
- On a normal gateway the billing owner is the creator, the auto-provisioned
  first owner.
- If the billing owner is revoked or demoted, the next-oldest owner becomes the
  billing owner.
- A role change updates the RoleBinding in place and keeps its `created_at`
  (GTW-ACC role change mechanism). So a user who was a viewer from day one and is
  later promoted to owner counts from the day they were first granted access.
  This is accepted for v0.3.
- Attribution is resolved from current bindings at query time. When the billing
  owner changes, past cost of that gateway moves to the new billing owner too.
  This is accepted for v0.3; see open question 9.

The more precise model (sandbox cost to its creator, projects and teams) is
deferred; see `cost-attribution-design.md`.

### Level 2 -- Workspace, through OpenShell workspace membership (exists)

In `managed` mode each OpenShell workspace has its own namespace
`openshell-<id>-<workspace>`. OpenShell records workspace members as
`WorkspaceMember{principal_subject, role}`, where the subject is the user's OIDC
`sub`. HyperShell `AgentWorkspace` resources carry the same membership
(AGT-WSP-02). So workspace cost can be shown per gateway, and a user who is a
member of a workspace can see that workspace's cost even on a gateway they do not
own.

### Level 3 -- Sandbox creator (later phase, needs OpenShell change)

OpenShell `Sandbox` metadata has `workspace`, `labels`, and `annotations` but no
creator. Per-sandbox, per-user cost needs:

1. OpenShell to stamp the authenticated creator's subject on each sandbox (for
   example label `openshell.io/created-by-sub`) and copy it to the sandbox pod.
2. Hub federation of per-pod requests for `openshell-*` namespaces only, with that
   label. This adds roughly one series per active sandbox, so it is gated on a
   cardinality check against the remote-write budget.

Level 3 is out of scope for this version.

## Metric sources

| Source ID          | BFF route                           | Metric IDs                                                                                  | Access   |
| ------------------ | ----------------------------------- | ------------------------------------------------------------------------------------------- | -------- |
| `cost-summary`     | `GET /api/metrics/cost/summary`     | `cost-total`, `cost-idle`, `cost-per-user`, `cost-per-gateway`, `cost-per-sandbox`          | admin    |
| `cost-breakdown`   | `GET /api/metrics/cost/breakdown`   | `cost-by-environment`, `cost-by-cluster`, `cost-by-resource-class`, `cost-by-billing-owner` | admin    |
| `cost-gateways`    | `GET /api/metrics/cost/gateways`    | `cost-top-gateways`                                                                         | admin    |
| `my-cost-summary`  | `GET /api/metrics/cost/me/summary`  | `my-cost-charged`, `my-cost-shared`, `my-cost-trend`                                        | any user |
| `my-cost-gateways` | `GET /api/metrics/cost/me/gateways` | `my-cost-by-gateway`, `my-cost-by-workspace`                                                | any user |

## Requirements

### Requirement: WEB-COST-01 -- Package placement and exports

The `@openshift-online/hypershell-operational-dashboard-ui` package SHALL host the
cost dashboard presentation layer, exporting at minimum `CostDashboardPage`,
`CostDashboardPageProps`, `CostDashboardMetrics`, `CostMetricSourceId`,
`CostMetricId`, `costDashboardMetricsQueryKey`, cost dashboard messages, and
`mockCostDashboardMetrics`. Presentation code SHALL NOT call `fetch`, the SDK, or
BFF routes directly; loading SHALL go through the host-injected
`getCostMetrics(context)` port, with the same workflow probes and `AbortSignal`
propagation as WEB-USE-02.

#### Scenario: WEB-COST-01.01 -- Host imports the cost page

- GIVEN the web console depends on the operational dashboard package
- WHEN the host route renders `CostDashboardPage` inside `DashboardUiProvider`
- THEN the page SHALL mount without importing React Router, the SDK, or Fastify

---

### Requirement: WEB-COST-02 -- Access by view

The `/dashboard/cost` route SHALL use `RequireAuthenticatedDashboard`. The
platform-view routes (`summary`, `breakdown`, `gateways`) SHALL require
`platform:admin`; other callers SHALL receive the standard forbidden response. The
`me/*` routes SHALL be available to every authenticated user and SHALL follow the
scoping in WEB-COST-09. The `Cost` entry SHALL appear in the dashboard secondary
navigation for every authenticated user. Platform-view widget headers SHALL show
the `System` label; My cost widget headers SHALL NOT.

#### Scenario: WEB-COST-02.01 -- Non-admin requests platform cost

- GIVEN a signed-in user without `platform:admin`
- WHEN the user calls `GET /api/metrics/cost/summary`
- THEN the BFF SHALL return the standard forbidden response
- AND `/dashboard/cost` SHALL render only the My cost view

---

### Requirement: WEB-COST-03 -- Rate card configuration

The BFF SHALL load the rate card from the file named by `COST_RATE_CARD_FILE` at
startup and on file change. A missing variable SHALL disable the cost routes, which
then return a `not-configured` status that the UI renders as a localized
"cost model not configured" state. An invalid rate card SHALL fail readiness. The
rate card SHALL NOT be sent to the browser; responses SHALL carry only `currency`
and the rate card `version`.

#### Scenario: WEB-COST-03.01 -- No rate card

- GIVEN `COST_RATE_CARD_FILE` is unset
- WHEN an administrator opens `/dashboard/cost`
- THEN every widget SHALL show the localized not-configured state
- AND no widget SHALL show `$0`

#### Scenario: WEB-COST-03.02 -- Cluster override

- GIVEN a rate card with a `clusters.<c>.cpu_core_hour` override
- WHEN cost for cluster `<c>` is calculated
- THEN the override SHALL apply to CPU and the provider rate to memory

---

### Requirement: WEB-COST-04 -- Reserved capacity calculation

The BFF SHALL compute cluster capacity, namespace, idle, gateway, and platform
overhead cost exactly as defined in "Calculations", from the hub recording rules.
A cluster whose `provider` label has no rate and no default SHALL be excluded from
totals and listed in the response as `unpriced`.

#### Scenario: WEB-COST-04.01 -- Idle capacity

- GIVEN a cluster with 64 allocatable cores and 40 requested cores
- AND a CPU rate of 0.03 per core-hour and no memory
- WHEN the hourly idle cost is calculated
- THEN it SHALL be 24 × 0.03 = 0.72

#### Scenario: WEB-COST-04.02 -- Over-committed cluster

- GIVEN requests exceed allocatable on a cluster
- WHEN idle cost is calculated
- THEN idle cost SHALL be zero, not negative

---

### Requirement: WEB-COST-05 -- Gateway attribution

Gateway cost SHALL be attributed by the gateway's namespaces (`openshell-<id>`
and, in `managed` mode, `openshell-<id>-<workspace>`) and intersected with the current Gateway inventory, as in WEB-USE-07. Deleted gateways
SHALL count toward historical totals and resource class `gateways` but SHALL NOT
appear in per-gateway lists or in `cost-per-gateway`. The `cost-top-gateways`
source SHALL return the ten most expensive current gateways with their cluster and
whether they are unused.

#### Scenario: WEB-COST-05.01 -- Unused gateway

- GIVEN a current gateway with no non-peer requests in the range
- WHEN the idle widget renders
- THEN its namespace cost SHALL appear in the "unused gateways" line

---

### Requirement: WEB-COST-06 -- Missing versus zero

Missing series SHALL make the dependent metric unavailable, never zero. A range
with partial coverage SHALL return coverage dates, and the UI SHALL show the shared
partial-data notice. Each route fails independently, as in WEB-USE-04.

---

### Requirement: WEB-COST-07 -- Transparency

Every response SHALL include `estimate: true`, `currency`, `rateCardVersion`, and
the list of included and excluded cost categories. The page SHALL render an
"About these estimates" section that states the model (requests × rate card),
what is excluded (storage, egress, managed services, LLM spend), and that values
are not billing-grade.

---

### Requirement: WEB-COST-09 -- My cost scoping

For `me/*` routes the BFF SHALL:

1. List the gateways authorized for the signed-in user with the user's own access
   token, and read each gateway's namespace and the user's role on it.
2. Derive the gateway namespace set (WEB-COST-04 "Gateway namespaces") and
   constrain every PromQL query to that set **before** aggregation.
3. Return `charged` = gateways whose billing owner is the user, and `shared` =
   every other authorized gateway (co-owner, admin or viewer), as separate totals.
   Each `shared` gateway SHALL name its billing owner.
4. Return per-workspace cost only for workspaces on gateways the user owns (billing
   or co-owner), or workspaces where the user's OIDC subject is a workspace member.

The BFF SHALL NOT return platform totals, idle capacity, platform overhead,
other users' gateways, or rates to a non-admin. A `platform:admin` opening My cost
SHALL see only their own bindings, not the fleet.

#### Scenario: WEB-COST-09.01 -- Owner and viewer

- GIVEN a user who is the billing owner of gateway A and a viewer on gateway B
- WHEN the user opens My cost
- THEN `charged` SHALL contain only A's cost and `shared` only B's
- AND no query SHALL include namespaces of any other gateway

#### Scenario: WEB-COST-09.02 -- Several owners

- GIVEN gateway A has owners U1 (granted first), U2 and U3
- WHEN each of them opens My cost
- THEN A's full cost SHALL appear under `charged` for U1 only
- AND U2 and U3 SHALL see A under `shared`, naming U1 as billing owner
- AND the sum of all users' `charged` totals SHALL equal the sum of all gateway cost

#### Scenario: WEB-COST-09.04 -- Billing owner leaves

- GIVEN U1 is A's billing owner and U2 is the next-oldest owner
- WHEN U1's owner binding is revoked
- THEN A SHALL be charged to U2 from the next refresh, including past days in the range

#### Scenario: WEB-COST-09.03 -- User with no gateways

- GIVEN a signed-in user with no gateway bindings
- WHEN the user opens My cost
- THEN the page SHALL show a localized empty state, not `$0`

---

### Requirement: WEB-COST-10 -- My cost layout

The My cost view SHALL contain:

| Widget                                                                 | Metric ID              |
| ---------------------------------------------------------------------- | ---------------------- |
| Cost charged to me (headline, monthly estimate)                        | `my-cost-charged`      |
| Cost of gateways shared with me, not charged to me                     | `my-cost-shared`       |
| Daily trend for the selected range                                     | `my-cost-trend`        |
| Cost by gateway, with my role, billing owner, cluster, and unused flag | `my-cost-by-gateway`   |
| Cost by workspace for owned gateways                                   | `my-cost-by-workspace` |

A new mockup `specs/web-console/mockups/dashboards/my-cost-dashboard.tsx` SHALL
be added and approved before implementation.

---

### Requirement: WEB-COST-08 -- Layout, localization, tests

The page SHALL use the approved mockup
(`specs/web-console/mockups/dashboards/cost-dashboard.tsx`), the shared widget
framework, and the dashboard message catalog for all text and currency formatting.
`DATA_SOURCES.md` SHALL document every cost metric with the same fields as
WEB-USE-09 plus the rate card fields it uses. Tests SHALL cover admin-only access,
rate card loading and resolution order, every calculation in this spec, unpriced
clusters, missing versus zero, partial coverage, and mockup parity.

## Open questions

1. **Shared-mode gateways.** `managed` mode (the default) separates sandboxes into
   workspace namespaces, so the gateway-versus-sandbox split is clean there. For
   `shared`-mode gateways, should v1 report one combined number (draft
   assumption) or subtract the gateway server pod's known requests?
2. ~~**Which Prometheus the BFF reads.**~~ Resolved in 0.4.0: an optional
   `COST_PROMETHEUS_URL` (with its own token and CA) points at the hub and
   defaults to `PROMETHEUS_URL`; `COST_CLUSTERS` scopes a shared hub.
3. **Rates.** Who owns the rate card numbers, and should they be per instance
   type (node-hour) rather than per core and GiB?
4. **Fixed costs.** Should ROSA or IKS cluster fees and the hub cluster itself be
   included as `fixed_monthly` in v1?
5. **Where the dashboard lives.** The product web console (this draft) or the
   operator-facing fleet dashboard? Cost data names clusters, which the fleet
   dashboard's identity firewall already handles at runtime. With the My cost
   view added, the web console is the better fit.
6. ~~**Shared ownership.**~~ Resolved 2026-11-27: one billing owner per gateway, the oldest current owner (replaces the 2026-11-26 full-cost-per-owner rule).
7. **Cost per active user.** Keep this platform-view metric alongside the new
   "cost by billing owner" breakdown, or drop it?
8. **Level 3.** Is per-sandbox creator attribution wanted soon enough to raise
   the OpenShell change (creator label) now?
9. **Stable history.** Should a billing owner change re-attribute past cost
   (v0.3, simplest), or should the control plane stamp the billing owner as a
   namespace label so history stays with whoever was billing owner at the time?

## References

- `specs/web-console/usage-dashboard.spec.md` (WEB-USE)
- `specs/security/rbac-enforcement.spec.md`, `specs/platform/gateway-access-management.spec.md` (gateway roles)
- `specs/platform/data-model.spec.md` PLAT-DATA-03, `specs/platform/openshell-gateway.spec.md` OSG-CORE-18 (namespaces, workspace mode)
- `specs/platform/openshell-gateway-sandbox-count.spec.md` OSG-SBX-01 (workspace namespace attribution)
- `specs/platform/agent-workspace-gateway-api.spec.md` AGT-WSP-02 (workspace membership)
- OpenShell `proto/openshell.proto` `WorkspaceMember`, `proto/datamodel.proto` `ObjectMeta.workspace`
- `specs/web-console/operational-dashboard.spec.md` (WEB-OPS)
- `specs/web-console/mockups/dashboards/cost-dashboard.tsx`, `my-cost-dashboard.tsx`
- gitops `bases/observability/stack/cluster-capacity.yaml`
- gitops `clusters/hypc-ibm-infra/3-bootstrap-apps/06-observability/metrics-hub/cluster-capacity-rules.yaml`
- gitops `specs/observability-metrics-remote-write.spec.md`
