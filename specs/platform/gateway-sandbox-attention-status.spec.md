# Gateway Sandbox Attention Status

**Status:** Draft
**Version:** 0.3.1
**Created:** 2026-09-21
**Applies to:** `components/control-plane` (sandbox attention derivation and OTLP gauges), `deploy/base` and `deploy/kind` control-plane metrics pipeline and Prometheus configuration, `components/web-console` BFF and dashboard adapter, `packages/operational-dashboard-ui`
**Related:** `platform/openshell-gateway-sandbox-count.spec.md`, `platform/gateway-sandbox-active-trends.spec.md`, `platform/openshell-gateway-namespace-gc.spec.md`, `platform/control-plane-observability.spec.md` (CP-OBS-07), `platform/gateway-provision-time.spec.md` (same OTLP→Prometheus→BFF pattern), `web-console/operational-dashboard.spec.md`

## Purpose

Extend the operational dashboard **Sandbox status** widget so operators see three fleet-wide attention counts alongside the existing active total and trend sparklines:

1. **Expiring soon** - active sandboxes whose agent-sandbox lifecycle expiry is overdue or within the next 24 hours
2. **Orphaned** - active sandboxes whose owning Gateway no longer exists
3. **Idle** - active sandboxes with a live owning Gateway that show no recent activity (or appear never-used after a grace period)

Today's fleet Active total (`hypershell_gateways_active_sandboxes_total`) sums each Gateway's `active_sandbox_count` and therefore **excludes** sandboxes in namespaces with no live Gateway. Attention counts require observing sandbox state directly (including those namespaces) and surfacing separate instant fields to the widget.

Attention counts are **control-plane application telemetry**: each control plane derives cluster-local tallies from Kubernetes sandbox pods and Sandbox CRs, exports them as OTLP gauges (CP-OBS-07), and the operational dashboard reads the Prometheus fleet sum through the BFF. Attention tallies SHALL NOT be persisted in HyperShell PostgreSQL or scraped from the API server `/metrics` endpoint.

```
+----------------------------------------------------------+
|  [icon] Sandbox status                                   |
|                                                          |
|           ( 214 )   ■ Active: 214                        |
|          Sandboxes                                       |
|                                                          |
|  ------------------------------------------------------  |
|  ATTENTION REQUIRED                                      |
|  [ (clock) 12 Expiring soon ]                            |
|  [ (disconnect) 3 Orphaned ]  [ (asleep) 7 Idle ]        |
|  ------------------------------------------------------  |
|                                                          |
|  [================= 24h Trend ========================]  |
|  [================= 7d Trend  ========================]  |
+----------------------------------------------------------+
```

Attention chips are PatternFly Labels with icons (SSA-09): Expiring soon = `RhUiClockIcon`, Orphaned = `RhUiDisconnectedIcon`, Idle = `RhUiAsleepIcon`.

## Goals

- Give dashboard operators a single-widget view of active sandboxes that need attention (expiring, orphaned, or idle) without navigating away.
- Define how HyperShell derives, exposes, and displays those counts consistently with existing active-sandbox vocabulary (`Pending` / `Running`).

## Non-Goals

- Click-through from attention chips to a filtered sandbox list (display-only in this version)
- Treating a missing user, session, or agent (without a missing Gateway) as orphaned
- Trend sparklines for attention counts
- Changing Active total semantics to include orphaned sandboxes
- Automatic cleanup or deletion of orphaned, expiring, or idle sandboxes
- Per-gateway or per-namespace attention breakdown on the operational dashboard
- Persisting cluster or fleet attention tallies in HyperShell PostgreSQL (including any `SetSandboxAttentionCounts`-style write path)
- Exposing attention gauges from the API server `/metrics` scrape path
- Deriving attention counts from the OpenShell gateway public sandbox API (`ListSandboxes` / `GetSandbox`)

## Domain Vocabulary

- **Active sandbox** - an agent sandbox pod in the `Pending` or `Running` phase, identified by the `agents.x-k8s.io/sandbox-name-hash` label (same meaning as `openshell-gateway-sandbox-count.spec.md`).
- **Owning Gateway** - the HyperShell Gateway whose assigned `openshell-*` namespace contains the sandbox pod. Attribution is by namespace only.
- **Orphaned sandbox** - an active sandbox whose owning Gateway does not exist in HyperShell (no live Gateway record for that namespace).
- **Sandbox expiry time** - the absolute expiry instant from the upstream agent-sandbox lifecycle field `spec.lifecycle.shutdownTime` on the Sandbox (or SandboxClaim lifecycle equivalent when that is the resource HyperShell observes). When unset, the sandbox has no expiry.
- **Expiring-soon sandbox** - an active sandbox with a sandbox expiry time `T` such that `T <= now + 24 hours` (includes sandboxes whose expiry time is already in the past but whose pod is still active).
- **Last activity time** - the upstream agent-sandbox status field `status.lastActivityTime` (updated by the router/SDK when activity is reported). When unset, HyperShell has no observed activity signal for that sandbox.
- **Sandbox creation time** - the creation timestamp of the observed Sandbox object (or the sandbox pod when that is the observed object), used only for the never-used idle heuristic.
- **Idle sandbox** - an active sandbox that has a live owning Gateway and meets either idle rule in SSA-03.
- **Attention required** - the Sandbox status widget section that presents the expiring-soon, orphaned, and idle counts.

## Actors

- **Dashboard operator** - an authenticated user authorized for operational dashboard metrics (dashboard-admin role when OIDC is enabled; same gate as OP-DASH-04 / OP-DASH-06).
- **Control plane** - observes cluster sandbox state, derives cluster-local attention counts, and exports them as OTLP gauges for the platform metrics pipeline.
- **Web-console BFF** - proxies Prometheus (or equivalent metrics) into `GET /api/metrics/gateway-sandboxes` for the dashboard adapter.

## Requirements

### Requirement: SSA-01 -- Orphaned Sandbox Definition

An orphaned sandbox SHALL be an active sandbox whose namespace has no live owning Gateway in HyperShell.

Succeeded, Failed, and Unknown sandbox pods SHALL NOT count as orphaned.
Active sandboxes whose owning Gateway still exists SHALL NOT count as orphaned, even if the sandbox is idle or unused.

#### Scenario: Active sandbox in a namespace with no Gateway is orphaned

- GIVEN a managed `openshell-*` namespace with no live Gateway
- AND that namespace contains 2 `Running` sandbox pods and 1 `Succeeded` sandbox pod
- WHEN the platform evaluates the orphaned count
- THEN the orphaned count SHALL include `2`
- AND the `Succeeded` pod SHALL NOT be included

#### Scenario: Active sandbox with a live Gateway is not orphaned

- GIVEN a Gateway whose namespace contains 3 active sandbox pods
- WHEN the platform evaluates the orphaned count
- THEN those 3 sandboxes SHALL NOT contribute to the orphaned count

---

### Requirement: SSA-02 -- Expiring-Soon Sandbox Definition

An expiring-soon sandbox SHALL be an active sandbox that has a sandbox expiry time `T` and satisfies `T <= now + 24 hours`.

Active sandboxes with no sandbox expiry time set SHALL NOT count as expiring soon.
Active sandboxes whose expiry time is more than 24 hours in the future SHALL NOT count as expiring soon.
Active sandboxes whose expiry time is already past SHALL count as expiring soon while the pod remains active.

The 24-hour window SHALL be measured in wall-clock time from evaluation time (UTC).

#### Scenario: Shutdown within 24 hours counts

- GIVEN an active sandbox with `shutdownTime` 6 hours from now
- WHEN the platform evaluates the expiring-soon count
- THEN that sandbox SHALL be included

#### Scenario: Already-past shutdown still running counts

- GIVEN an active sandbox with `shutdownTime` 2 hours in the past
- WHEN the platform evaluates the expiring-soon count
- THEN that sandbox SHALL be included

#### Scenario: No shutdownTime does not count

- GIVEN an active sandbox with no sandbox expiry time
- WHEN the platform evaluates the expiring-soon count
- THEN that sandbox SHALL NOT be included

#### Scenario: Far-future shutdown does not count

- GIVEN an active sandbox with `shutdownTime` 48 hours from now
- WHEN the platform evaluates the expiring-soon count
- THEN that sandbox SHALL NOT be included

---

### Requirement: SSA-03 -- Idle Sandbox Definition

An idle sandbox SHALL be an active sandbox that has a live owning Gateway (not orphaned) and satisfies exactly one of the following:

1. **Stale activity:** `lastActivityTime` is set and `now - lastActivityTime >= 1 hour`
2. **Never used:** `lastActivityTime` is unset and `now - sandbox creation time >= 24 hours`

Active sandboxes with a live owning Gateway, unset `lastActivityTime`, and sandbox age under 24 hours SHALL NOT count as idle (activity reporting may still arrive).
Orphaned sandboxes SHALL NOT count as idle; they appear only under Orphaned.
Succeeded, Failed, and Unknown sandbox pods SHALL NOT count as idle.

The 1-hour and 24-hour windows SHALL be measured in wall-clock time from evaluation time (UTC).

#### Scenario: Stale lastActivityTime counts as idle

- GIVEN an active sandbox with a live owning Gateway
- AND `lastActivityTime` is 90 minutes in the past
- WHEN the platform evaluates the idle count
- THEN that sandbox SHALL be included

#### Scenario: Recent lastActivityTime does not count as idle

- GIVEN an active sandbox with a live owning Gateway
- AND `lastActivityTime` is 10 minutes in the past
- WHEN the platform evaluates the idle count
- THEN that sandbox SHALL NOT be included

#### Scenario: Unset lastActivityTime and age over 24 hours counts as idle

- GIVEN an active sandbox with a live owning Gateway
- AND `lastActivityTime` is unset
- AND sandbox creation time is 30 hours in the past
- WHEN the platform evaluates the idle count
- THEN that sandbox SHALL be included

#### Scenario: Unset lastActivityTime and age under 24 hours does not count as idle

- GIVEN an active sandbox with a live owning Gateway
- AND `lastActivityTime` is unset
- AND sandbox creation time is 2 hours in the past
- WHEN the platform evaluates the idle count
- THEN that sandbox SHALL NOT be included

#### Scenario: Orphaned sandbox is not also idle

- GIVEN an active sandbox with no live owning Gateway
- AND `lastActivityTime` is 2 hours in the past
- WHEN the platform evaluates attention counts
- THEN that sandbox SHALL contribute to the orphaned count
- AND that sandbox SHALL NOT contribute to the idle count

---

### Requirement: SSA-04 -- Dual Membership

A single active sandbox SHALL be eligible for multiple attention counts at once when it meets more than one definition, except that idle and orphaned are mutually exclusive by SSA-03 (idle requires a live owning Gateway).

Examples that MAY overlap:

- Orphaned and expiring soon
- Idle and expiring soon

Each attention count SHALL be an independent tally. The platform SHALL NOT force a sandbox into only one attention bucket when multiple non-conflicting definitions apply.

#### Scenario: Orphaned and expiring sandbox appears in both counts

- GIVEN an active sandbox with no live owning Gateway
- AND that sandbox has `shutdownTime` within the next 24 hours
- WHEN the platform evaluates attention counts
- THEN the sandbox SHALL contribute `1` to the orphaned count
- AND the sandbox SHALL contribute `1` to the expiring-soon count
- AND the sandbox SHALL NOT contribute to the idle count

#### Scenario: Idle and expiring sandbox appears in both counts

- GIVEN an active sandbox with a live owning Gateway
- AND `lastActivityTime` is 2 hours in the past
- AND `shutdownTime` is within the next 24 hours
- WHEN the platform evaluates attention counts
- THEN the sandbox SHALL contribute `1` to the idle count
- AND the sandbox SHALL contribute `1` to the expiring-soon count

---

### Requirement: SSA-05 -- Fleet Attention Derivation

The control plane SHALL derive **orphaned**, **expiring-soon**, and **idle** counts for the managed clusters it reconciles by observing active sandbox state in managed gateway namespaces, not by summing Gateway `active_sandbox_count` fields alone.

Orphaned derivation SHALL compare observed active sandbox pods (or equivalent Sandbox objects tied to those pods) against the set of live Gateway namespaces. The control plane MAY obtain that live-namespace set by reading HyperShell Gateway inventory (for example a gRPC list scoped to the control plane's cluster). That read SHALL NOT write attention tallies into HyperShell storage.

Expiring-soon derivation SHALL use each sandbox's sandbox expiry time when present.

Idle derivation SHALL use `lastActivityTime` when set, and otherwise the never-used creation-age heuristic in SSA-03.

When Sandbox CR listing fails, the control plane SHALL still export orphaned counts derived from active sandbox pods and live Gateway namespaces, and SHALL treat expiring-soon and idle as zero for that tick (pod-only degrade). Orphaned derivation MUST NOT depend on Sandbox CR availability.

Each successful attention reconcile tick SHALL update the control plane's exported OTLP attention gauges (SSA-06) to the newly derived cluster-local counts.

Attention counts SHALL be advisory recent values, matching the advisory semantics of `active_sandbox_count` (not a real-time guarantee).

#### Scenario: Orphaned sandboxes are visible even though Active gauge excludes them

- GIVEN `hypershell_gateways_active_sandboxes_total` reports `10` from live Gateways
- AND 3 active sandbox pods exist in namespaces with no live Gateway
- WHEN attention counts are evaluated
- THEN the orphaned count SHALL be `3`
- AND the Active total used by the widget SHALL remain `10`

#### Scenario: Attention derivation does not persist tallies in PostgreSQL

- GIVEN the control plane has derived orphaned `3`, expiring-soon `12`, and idle `7`
- WHEN the control plane finishes an attention reconcile tick
- THEN the control plane SHALL export those counts as OTLP gauges (SSA-06)
- AND the control plane SHALL NOT upsert attention tallies into HyperShell PostgreSQL

---

### Requirement: SSA-06 -- Control-Plane Attention Gauges

The control plane SHALL export cluster-local attention counts as OpenTelemetry gauges over OTLP (CP-OBS-01 / CP-OBS-07):

| OTLP metric | Type | Unit | Meaning |
| --- | --- | --- | --- |
| `gateway.sandbox.orphaned` | Gauge | `{sandbox}` | Cluster-local orphaned active sandbox count (SSA-01) |
| `gateway.sandbox.expiring` | Gauge | `{sandbox}` | Cluster-local expiring-soon active sandbox count (SSA-02) |
| `gateway.sandbox.idle` | Gauge | `{sandbox}` | Cluster-local idle active sandbox count (SSA-03) |

Each gauge SHALL report a non-negative integer reflecting the most recently derived cluster-local count from SSA-05.
Each gauge SHALL include a low-cardinality `hypershell.cluster_id` attribute set to the control plane's managed-cluster ID, or `default` when the control plane runs in single-cluster mode (empty cluster ID). The gauges SHALL NOT include a sandbox identifier, namespace, or Gateway identifier attribute (cardinality bound CP-OBS-06).

When a hard attention reconcile failure prevents derivation (for example live Gateway inventory or sandbox pod listing fails), the control plane SHALL export zero for all three gauges on that tick so a prior successful tick cannot leave stale non-zero values exporting while the process remains healthy. Prometheus staleness still drops series when a control plane stops exporting entirely.

When exported to Prometheus, the gauges SHALL appear as:

| Prometheus series | Meaning |
| --- | --- |
| `gateway_sandbox_orphaned` | Orphaned count from `gateway.sandbox.orphaned` |
| `gateway_sandbox_expiring` | Expiring-soon count from `gateway.sandbox.expiring` |
| `gateway_sandbox_idle` | Idle count from `gateway.sandbox.idle` |

The Prometheus instance configured for the web-console BFF (`PROMETHEUS_URL`, same origin as provision-time metrics) SHALL expose those series. Deploy and local-development configuration SHALL route control-plane OTLP metrics into that Prometheus instance, matching the pipeline used for `gateway_provision_duration_seconds` (GPT-00). Control-plane OTLP resources SHALL include `k8s.namespace.name` set to the instance namespace (`HYPERSHELL_NAMESPACE`) so BFF queries can scope series with `k8s_namespace_name` the same way as provision-time metrics.

Fleet-wide attention totals for the dashboard SHALL dedupe control-plane replicas per managed cluster, then sum across clusters (for example `sum(max by (hypershell_cluster_id) (gateway_sandbox_orphaned))`). When `PROMETHEUS_NAMESPACE` is set, the series selector SHALL use `k8s_namespace_name`, not scrape `namespace`. When a control plane stops exporting, Prometheus staleness SHALL drop that instance from the sum so a silent or removed cluster does not inflate fleet totals indefinitely.

When OTLP metrics are disabled (`OTEL_EXPORTER_OTLP_ENDPOINT` unset, or `OTEL_METRICS_EXPORTER=none`), attention gauges SHALL NOT be exported. The BFF SHALL treat missing attention series as attention unavailable (SSA-07) rather than synthesizing zeros.

The Active gauge `hypershell_gateways_active_sandboxes_total` SHALL remain unchanged in meaning (sum of live Gateway `active_sandbox_count` values) and SHALL remain an API-server scrape metric. Attention gauges SHALL NOT be exposed from the API server `/metrics` endpoint.

#### Scenario: Exported gauges report cluster-local attention totals

- GIVEN the control plane has derived orphaned count `3`, expiring-soon count `12`, and idle count `7` for its managed clusters
- AND OTLP metrics export is enabled
- WHEN Prometheus scrapes the control-plane metrics pipeline
- THEN `gateway_sandbox_orphaned` SHALL report `3` for that control-plane instance
- AND `gateway_sandbox_expiring` SHALL report `12`
- AND `gateway_sandbox_idle` SHALL report `7`

#### Scenario: Fleet sum aggregates multiple control planes

- GIVEN one control-plane instance exports `gateway_sandbox_orphaned = 3` with `hypershell.cluster_id = cluster-a`
- AND another control-plane instance exports `gateway_sandbox_orphaned = 1` with `hypershell.cluster_id = cluster-b`
- WHEN the BFF evaluates the fleet orphaned PromQL
- THEN the orphaned fleet total SHALL be `4`

#### Scenario: Fleet PromQL dedupes control-plane replicas for the same cluster

- GIVEN two control-plane pods for `hypershell.cluster_id = cluster-a` each export `gateway_sandbox_orphaned = 3`
- WHEN the BFF evaluates the fleet orphaned PromQL
- THEN the orphaned fleet total SHALL be `3` (not `6`)

#### Scenario: Hard derive failure exports zeros

- GIVEN the previous successful tick exported orphaned `3`, expiring-soon `12`, and idle `7`
- AND the next tick fails to list live Gateway namespaces
- WHEN that failed tick completes
- THEN the control plane SHALL export orphaned `0`, expiring-soon `0`, and idle `0`

#### Scenario: Attention gauges are absent when OTLP metrics are disabled

- GIVEN `OTEL_EXPORTER_OTLP_ENDPOINT` is not set on the control plane
- WHEN the BFF queries attention PromQL
- THEN the attention series SHALL be absent
- AND the BFF SHALL omit attention fields per SSA-07

---

### Requirement: SSA-07 -- BFF Gateway Sandboxes Attention Fields

The web-console BFF SHALL extend `GET /api/metrics/gateway-sandboxes` with optional attention fields while preserving existing instant and trend fields:

| Field | Type | Required | Meaning |
| --- | --- | --- | --- |
| `active_sandboxes` | number | Yes | Existing fleet Active total (unchanged) |
| `orphaned_sandboxes` | number | No | Fleet orphaned count (SSA-01), `sum(max by (hypershell_cluster_id) (gateway_sandbox_orphaned))` |
| `expiring_sandboxes` | number | No | Fleet expiring-soon count (SSA-02), `sum(max by (hypershell_cluster_id) (gateway_sandbox_expiring))` |
| `idle_sandboxes` | number | No | Fleet idle count (SSA-03), `sum(max by (hypershell_cluster_id) (gateway_sandbox_idle))` |
| `hourly_active_sandboxes` | array | No | Existing (GSAT-03) |
| `daily_active_sandboxes` | array | No | Existing (GSAT-03) |

When the Active instant query succeeds but one or more attention gauges fail or are absent, the BFF SHALL respond with HTTP `200`, include `active_sandboxes`, and SHALL omit only the failed or absent attention field(s).
When the Active instant query fails, the BFF SHALL continue to respond with HTTP `502` and SHALL NOT return attention-only JSON without `active_sandboxes`.

Dashboard-operator authorization SHALL remain as defined in OP-DASH-06.

#### Scenario: Successful response includes Active and all attention fields

- GIVEN Prometheus returns successful Active, orphaned, expiring-soon, and idle instant queries
- WHEN an authorized caller sends `GET /api/metrics/gateway-sandboxes`
- THEN the BFF SHALL respond with HTTP `200`
- AND the body SHALL include `active_sandboxes`, `orphaned_sandboxes`, `expiring_sandboxes`, and `idle_sandboxes`

#### Scenario: Attention gauge failure omits only failed attention fields

- GIVEN Prometheus returns a successful Active instant query
- AND the orphaned, expiring-soon, and idle instant queries fail or return no series
- WHEN an authorized caller sends `GET /api/metrics/gateway-sandboxes`
- THEN the BFF SHALL respond with HTTP `200` with `active_sandboxes`
- AND `orphaned_sandboxes`, `expiring_sandboxes`, and `idle_sandboxes` SHALL be absent

---

### Requirement: SSA-08 -- Adapter Mapping onto provisioned-sandboxes

The host `DashboardControlPlane` adapter SHALL map attention fields from `GET /api/metrics/gateway-sandboxes` onto the existing `provisioned-sandboxes` metric used by the Sandbox status widget:

| BFF field | OperationalMetric field | Mapping |
| --- | --- | --- |
| `active_sandboxes` | `value` | Existing stringified Active total |
| `orphaned_sandboxes` | attention field for orphaned (non-negative integer) | Present only when BFF field is present |
| `expiring_sandboxes` | attention field for expiring soon (non-negative integer) | Present only when BFF field is present |
| `idle_sandboxes` | attention field for idle (non-negative integer) | Present only when BFF field is present |

Exact TypeScript field names on `OperationalMetric` MAY be chosen at implementation time; the adapter SHALL expose all three attention counts to the Sandbox status presentation.

When an attention BFF field is absent, the adapter SHALL omit only that attention field.
The adapter SHALL NOT synthesize attention zeros from a missing field.
The adapter SHALL NOT fail the `gateway-metrics` source solely because attention fields are missing while Active succeeds.

Hourly and daily Active trends SHALL continue to map per GSAT-06.

#### Scenario: Adapter maps attention counts

- GIVEN the BFF returns `{ "active_sandboxes": 214, "orphaned_sandboxes": 3, "expiring_sandboxes": 12, "idle_sandboxes": 7 }`
- WHEN the adapter builds `provisioned-sandboxes`
- THEN `value` SHALL be `"214"`
- AND the orphaned attention count SHALL be `3`
- AND the expiring-soon attention count SHALL be `12`
- AND the idle attention count SHALL be `7`

#### Scenario: Missing attention fields do not invent zeros

- GIVEN the BFF returns `{ "active_sandboxes": 214 }` without attention fields
- WHEN the adapter builds `provisioned-sandboxes`
- THEN `value` SHALL be `"214"`
- AND orphaned, expiring-soon, and idle attention fields SHALL be absent

---

### Requirement: SSA-09 -- Sandbox Status Widget Attention Presentation

The `sandbox-status` widget SHALL continue to show:

1. The Active total and Active legend from `provisioned-sandboxes.value`
2. Optional **Last 24 hours** and **Last 7 days** Active trend sparklines (GSAT / existing Sandbox status card behavior)

The widget SHALL add an **Attention required** section between the Active summary and the trend sparklines when at least one present attention count is greater than `0` (SSA-08).

The Attention required section SHALL show:

- Localized heading **Attention required**
- A display-only PatternFly `Label` for each **non-zero** attention count among:
  - **Expiring soon** with the expiring-soon count and icon `RhUiClockIcon`
  - **Orphaned** with the orphaned count and icon `RhUiDisconnectedIcon`
  - **Idle** with the idle count and icon `RhUiAsleepIcon`

Each attention Label SHALL place the icon before the count and localized text (for example, clock icon then `12 Expiring soon`).
Icon-only presentation SHALL NOT be used; the count and label text remain visible.
Label status or color MAY distinguish severity (for example warning for Expiring soon, danger for Orphaned, info or custom for Idle) but SHALL NOT replace the distinct icons above.
Labels with count `0` SHALL NOT be rendered (PatternFly aggregate-status guidance: only include non-zero exception items).

When all present attention counts equal `0`, the Attention required section SHALL NOT render.
Omit-vs-healthy remains distinguishable because absent attention fields still use the unavailable state below.
The chips SHALL NOT navigate or open a filtered list in this version.

When all attention fields are absent, the widget SHALL render a localized unavailable state for the Attention required section only, and SHALL continue to show Active and any available trends.
When only some attention fields are present, the widget SHALL render chips only for present fields whose counts are greater than `0`, and SHALL NOT invent zeros for absent fields.

Optional Active trend sparklines SHALL follow the Users card pattern: a horizontal rule above the sparkline block, a localized heading above each sparkline (**Active sandboxes per hour** for the 24-hour series, **Active sandboxes per day** for the 7-day series), and the existing window captions (**Last 24 hours** / **Last 7 days**).

Layout height / persistence key bumps SHALL follow the same pattern as other adoption widgets when the default template would clip the new section (OP-DASH-11).

#### Scenario: Attention section shows non-zero counts

- GIVEN `provisioned-sandboxes` has `value: "214"`, orphaned `3`, expiring soon `12`, idle `7`
- WHEN the Sandbox status widget renders
- THEN the Active total SHALL show `214`
- AND Attention required SHALL show Expiring soon `12` with `RhUiClockIcon`
- AND Attention required SHALL show Orphaned `3` with `RhUiDisconnectedIcon`
- AND Attention required SHALL show Idle `7` with `RhUiAsleepIcon`
- AND all three Labels SHALL be non-interactive

#### Scenario: Zero attention counts hide the section

- GIVEN `provisioned-sandboxes` has orphaned `0`, expiring soon `0`, and idle `0`
- WHEN the Sandbox status widget renders
- THEN Attention required SHALL NOT be visible
- AND Active and any available trends SHALL still render

#### Scenario: Mixed zero and non-zero counts show only non-zero chips

- GIVEN `provisioned-sandboxes` has orphaned `3`, expiring soon `0`, and idle `0`
- WHEN the Sandbox status widget renders
- THEN Attention required SHALL be visible
- AND Attention required SHALL show Orphaned `3`
- AND Expiring soon and Idle Labels SHALL NOT render

#### Scenario: Attention unavailable keeps Active visible

- GIVEN `provisioned-sandboxes` has `value: "214"` without attention fields
- AND Active trends are present
- WHEN the Sandbox status widget renders
- THEN Active `214` and trends SHALL render
- AND Attention required SHALL show the localized unavailable state
- AND the dashboard SHALL NOT enter total initial-load error solely for missing attention fields

---

### Requirement: SSA-10 -- Partial Failure and Refresh Behavior

Attention data SHALL be optional within the `gateway-metrics` metric source (OP-DASH-19), matching the optional treatment of sandbox trends (GSAT-08).

A failure to load orphaned, expiring-soon, or idle counts SHALL NOT omit instant Active `provisioned-sandboxes` when the Active query succeeds.
An attention failure SHALL NOT omit `provisioned-gateways`, `provision-time`, or `provision-reliability` when those routes succeed.

Attention counts SHALL refresh on the same operational dashboard cadence as other metrics (`operationalDashboardRefreshMilliseconds`, 15 minutes).

#### Scenario: Attention failure does not hide Active total

- GIVEN `GET /api/metrics/gateway-sandboxes` returns `{ "active_sandboxes": 8 }` without attention fields
- AND other gateway-metrics routes succeed
- WHEN an authorized operator opens `/dashboard`
- THEN the Sandbox status widget SHALL show Active `8`
- AND Attention required SHALL be unavailable
- AND the dashboard SHALL NOT show the total initial-load danger alert

---

### Requirement: SSA-11 -- Verification

The control plane SHALL include tests covering:

- Orphaned counting for active pods in namespaces without a live Gateway
- Exclusion of non-active sandbox pods from all attention counts
- Expiring-soon inclusion for past and within-24h `shutdownTime`, and exclusion when unset or beyond 24 hours
- Idle stale-activity rule (`lastActivityTime` >= 1 hour)
- Idle never-used rule (unset `lastActivityTime` and age >= 24 hours)
- Exclusion of unset `lastActivityTime` when age is under 24 hours
- Mutual exclusion of idle and orphaned
- Dual membership for orphaned+expiring and idle+expiring
- Export of the three OTLP attention gauges after a successful derive tick
- Pod-only degrade: orphaned still exported when Sandbox CR listing fails, with expiring and idle set to zero

The web-console BFF SHALL include unit tests for `GET /api/metrics/gateway-sandboxes` covering successful attention field mapping from `sum(max by (hypershell_cluster_id) (gateway_sandbox_*))` PromQL (including `k8s_namespace_name` scoping when configured) and omission when attention gauges fail or are absent while Active succeeds.

The web console SHALL include unit tests for adapter mapping of attention fields onto `provisioned-sandboxes`, including present and absent payloads.

The operational dashboard package SHALL:

- Extend fixtures / Storybook states for Sandbox status with representative attention counts, zero counts, and attention-unavailable
- Document the control-plane gauges, Prometheus series, and BFF fields in `packages/operational-dashboard-ui/DATA_SOURCES.md`

#### Scenario: CI exercises attention adapter mapping

- GIVEN a mocked BFF gateway-sandboxes response with known `orphaned_sandboxes`, `expiring_sandboxes`, and `idle_sandboxes`
- WHEN dashboard adapter unit tests run
- THEN they SHALL assert those counts on `provisioned-sandboxes`
- AND assert attention fields are omitted when the BFF fields are absent

## Success Criteria

- **SC-001**: An authorized dashboard operator can see Active on the Sandbox status widget, plus Expiring soon / Orphaned / Idle Labels when those counts are greater than `0`, without leaving `/dashboard`.
- **SC-002**: When attention gauges are down but Active is up, Active and trends remain visible and only Attention required is unavailable.
- **SC-003**: Orphaned count reflects active sandboxes in namespaces with no live Gateway, even when those sandboxes are absent from `hypershell_gateways_active_sandboxes_total`.
- **SC-004**: Expiring-soon count includes overdue and next-24-hour expiries, and excludes sandboxes with no expiry time.
- **SC-005**: Idle count includes stale activity (>= 1 hour) and never-used sandboxes (unset activity and age >= 24 hours), and excludes orphans and young unset-activity sandboxes.
- **SC-006**: Attention counts reach Prometheus from control-plane OTLP gauges without HyperShell PostgreSQL attention storage or API-server `/metrics` attention series.

## Assumptions

- Upstream agent-sandbox Sandbox (or SandboxClaim) objects expose `spec.lifecycle.shutdownTime` when a sandbox is configured to expire; HyperShell can observe that field for managed gateway namespaces.
- Upstream agent-sandbox exposes `status.lastActivityTime` when the router/SDK reports activity; HyperShell can observe that field for idle evaluation.
- Managed gateway namespaces remain attributable the same way as namespace GC (`openshell-gateway-namespace-gc.spec.md`): HyperShell can tell which namespaces are (or were) gateway namespaces for this instance.
- The Sandbox status widget continues to bind to the `provisioned-sandboxes` metric (current package behavior).
- Control-plane OTLP metrics are routed into the same Prometheus instance the BFF uses for provision-time and provision-reliability (GPT-00 / GPO).

## Alternatives Considered

| Alternative | Why rejected |
| --- | --- |
| Persist cluster attention rows in HyperShell PostgreSQL and scrape them from the API server | Adds a specialized write path and scrape-time DB dependency for advisory dashboard tallies; control-plane OTLP gauges already match the provision-time / provision-reliability pattern. |
| Derive attention from the OpenShell gateway public sandbox API | Public `SandboxSpec` / `SandboxStatus` lack `shutdownTime` and `lastActivityTime`; orphaned namespaces have no live gateway to query. |
| Always show Attention required with `0` chips | Conflicts with PatternFly aggregate-status guidance (non-zero exceptions only); omit-vs-healthy is covered by the unavailable state when fields are absent. |
| Count only future expiries (exclude already past) | Overdue-but-still-active sandboxes are the highest-urgency attention case. |
| Fold unused sandboxes into Orphaned | Orphaned means missing Gateway; idle-with-live-Gateway is a different operator action. |
| Count every unset `lastActivityTime` as idle | Inflates Idle with false positives while activity reporting is still catching up; the 24-hour never-used grace avoids that. |
| Fold orphaned sandboxes into the Active total | Would change Active semantics and hide the distinction operators need; Active stays gateway-backed. |
| Count orphans as idle as well | Would double-signal the same leftover workload; Orphaned alone is the correct bucket. |
