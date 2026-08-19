# OpenShell Gateway Active Sandbox Count

**Date:** 2026-08-18
**Status:** Active

## Purpose

This spec defines how the HyperShell control plane derives, maintains, and
surfaces the number of active agent sandboxes running against a Gateway. The
count is published on the Gateway as the read-only `active_sandbox_count` field
so operators can see how many running sessions a gateway is serving (for example,
in the main gateways table and in the delete-confirmation dialog) before they act
on it.

The count is maintained **event-driven** from a control-plane watch on sandbox
pods, rather than by repeatedly LISTing every pod in every gateway namespace. It
increments as sandbox pods become active and decrements as they leave active or
are deleted, and it self-heals by periodically reconciling the stored value to
the truth held in the control plane's in-memory watch cache. This gives an
accurate count without a costly, repeated full-namespace poll of Kubernetes.

This spec is a sub-spec of [`control-plane.spec.md`](./control-plane.spec.md).
It owns the `active_sandbox_count` mechanism, which was previously attributed to
the health reconciler in
[`openshell-gateway-health.spec.md`](./openshell-gateway-health.spec.md). The
delete-confirmation warning that consumes this count is defined in
[`openshell-gateway-namespace-gc.spec.md`](./openshell-gateway-namespace-gc.spec.md)
(§ Surface Active Sandbox Count Before Deletion). Namespace assignment and
provisioning mechanics are defined in
[`openshell-gateway.spec.md`](./openshell-gateway.spec.md).

## Domain Vocabulary

- **Active sandbox** - an agent sandbox pod in the `Running` or `Pending` phase.
  Sandbox pods are created by the upstream OpenShell gateway (via the
  agent-sandbox controller), not by this control plane, and are identified by the
  `agents.x-k8s.io/sandbox-name-hash` label the agent-sandbox controller stamps on
  them.
- **Gateway namespace** - the Kubernetes namespace a gateway's workloads and its
  sandboxes run in. Its name is API-assigned at Gateway creation and is prefixed
  `openshell-`. A sandbox pod is attributed to a Gateway solely by the namespace
  it runs in; sandboxes carry no back-reference to the Gateway.
- **`active_sandbox_count`** - the read-only Gateway field carrying the number of
  active sandboxes most recently observed in that gateway's namespace. It is an
  advisory recent value, not a real-time guarantee.
- **Sandbox pod watch** - the control plane's informer over sandbox pods in
  managed gateway namespaces. Its in-memory cache (lister) is the source of truth
  for both the incremental adjustments and the periodic self-heal, so neither
  path needs to poll the Kubernetes API in steady state.

## Requirements

### Requirement: Event-Driven Active Sandbox Accounting

The control plane SHALL maintain each Gateway's `active_sandbox_count` from a
watch on sandbox pods in that gateway's namespace. When a sandbox pod becomes
active (enters `Running` or `Pending`), the control plane SHALL increment the
owning Gateway's count; when a sandbox pod leaves the active set (terminates,
fails, or is deleted), it SHALL decrement the count. A pod that transitions
between two active phases (for example `Pending` to `Running`) SHALL NOT change
the count. In steady state the control plane SHALL NOT depend on a periodic
full-namespace pod LIST to keep the count current.

#### Scenario: Sandbox pod becomes active

- GIVEN a Gateway whose `active_sandbox_count` is 2
- WHEN a new sandbox pod in that gateway's namespace enters `Pending` or `Running`
- THEN the control plane SHALL report `active_sandbox_count = 3` on the Gateway

#### Scenario: Sandbox pod is deleted

- GIVEN a Gateway whose `active_sandbox_count` is 3
- WHEN one of its active sandbox pods terminates or is deleted
- THEN the control plane SHALL report `active_sandbox_count = 2` on the Gateway

#### Scenario: Active-to-active transition does not double-count

- GIVEN an active sandbox pod already counted for its Gateway
- WHEN that pod transitions from `Pending` to `Running`
- THEN the control plane SHALL NOT change the Gateway's `active_sandbox_count`

#### Scenario: Non-sandbox pod is ignored

- GIVEN a pod in a gateway namespace that does not carry a sandbox label
- WHEN that pod is created, changes phase, or is deleted
- THEN the control plane SHALL NOT change any Gateway's `active_sandbox_count`

### Requirement: Atomic, Non-Negative Updates

Sandbox count adjustments SHALL be applied atomically at the source of truth (the
API server database) so that concurrent adjustments do not lose updates. The
stored count SHALL NOT go below zero, and an unset (NULL) count SHALL be treated
as zero for the purpose of an adjustment.

#### Scenario: Concurrent increments do not lose updates

- GIVEN a Gateway whose `active_sandbox_count` is 0
- WHEN two sandbox pods become active at nearly the same time, each applying a +1
  adjustment
- THEN the resulting `active_sandbox_count` SHALL be 2, not 1

#### Scenario: Count floors at zero

- GIVEN a Gateway whose `active_sandbox_count` is 0 (or unset)
- WHEN a decrement adjustment is applied (for example from a duplicate delete
  event)
- THEN the stored `active_sandbox_count` SHALL remain 0 and SHALL NOT go negative

### Requirement: Convergence and Self-Heal

The control plane SHALL periodically reconcile each Gateway's stored
`active_sandbox_count` to the number of active sandbox pods observed for that
gateway in its in-memory watch cache, correcting drift caused by missed events or
a control-plane restart. This reconciliation SHALL read the watch cache rather
than poll the Kubernetes API. Because it sets the count to an observed absolute
value, it SHALL be the mechanism that recovers an accurate count after a restart,
without requiring any intervening sandbox create or delete.

#### Scenario: Count converges after a control-plane restart

- GIVEN a gateway namespace with N active sandbox pods
- AND the control plane restarts, losing any in-flight incremental state
- WHEN the control plane's sandbox pod watch has synced and the periodic
  reconciliation runs
- THEN it SHALL set the Gateway's `active_sandbox_count` to N without any
  intervening sandbox create or delete event

#### Scenario: Drift from a missed event is corrected

- GIVEN a Gateway whose stored `active_sandbox_count` has drifted from the actual
  number of active sandbox pods (for example an event was missed)
- WHEN the periodic reconciliation runs against the watch cache
- THEN it SHALL set the Gateway's `active_sandbox_count` to the observed number of
  active sandbox pods

### Requirement: Control-Plane-Owned, Read-Only Surfacing

`active_sandbox_count` SHALL be owned and written only by the control plane. It
SHALL be read-only over the REST API (OpenAPI `readOnly`), SHALL be excluded from
the Gateway patch request, and SHALL be written only over the control plane's
gRPC path. A client SHALL NOT be able to set or alter the count.

#### Scenario: REST client cannot modify the count

- GIVEN a Gateway with a reported `active_sandbox_count`
- WHEN a client submits a REST update or patch that attempts to set
  `active_sandbox_count`
- THEN the API server SHALL NOT change the stored count from that request

### Requirement: Console Surfaces the Count in the Gateways Table

The web console SHALL display `active_sandbox_count` as a column in the main
gateways table, adjacent to the gateway name, so an operator can see the active
sandbox load of every gateway at a glance. The value SHALL be presented as an
advisory recent count. When the count is not available for a gateway (unset), the
console SHALL render a localized not-available fallback rather than a misleading
zero or a blank cell.

#### Scenario: Count shown in the gateways table

- GIVEN a Gateway reporting `active_sandbox_count = 3`
- WHEN the gateways table is rendered in the console
- THEN the row for that gateway SHALL show `3` in the active sandboxes column,
  adjacent to the gateway name

#### Scenario: Unknown count shows a fallback

- GIVEN a Gateway whose `active_sandbox_count` has not been reported (unset)
- WHEN the gateways table is rendered
- THEN the active sandboxes cell SHALL show a localized not-available fallback,
  not `0`

### Requirement: Advisory Semantics

The reported `active_sandbox_count` reflects the control plane's most recent
observation and MAY lag real time; consumers SHALL treat it as an advisory recent
count, not a real-time guarantee. The count SHALL NOT gate any backend decision,
including gateway deletion or namespace deletion (see
[`openshell-gateway-namespace-gc.spec.md`](./openshell-gateway-namespace-gc.spec.md)).
It surfaces load to operators; it never blocks an action.

#### Scenario: Count never blocks deletion

- GIVEN a Gateway reporting a non-zero `active_sandbox_count`
- WHEN the gateway (and its namespace) is deleted
- THEN the deletion SHALL proceed regardless of the reported count, which is used
  only to warn the operator beforehand

## Design Decisions

| Decision | Rationale |
|----------|-----------|
| Maintain the count from a sandbox pod watch, not a periodic full-namespace LIST | A repeated LIST of every pod in every gateway namespace on a fixed cadence is a costly poll of Kubernetes that scales with the fleet. A single informer over sandbox pods delivers create/update/delete events and keeps a local cache, so the count stays current without repeatedly asking the API server. |
| Event source is a control-plane watch, not a controller callback | The agent-sandbox controller that creates sandbox pods is upstream (`kubernetes-sigs/agent-sandbox`) and cannot call the HyperShell API. The only reliable, in-house signal for sandbox lifecycle is a control-plane watch on the pods it can already observe. |
| Attribute sandboxes to a Gateway by namespace | Sandbox pods carry no back-reference to their Gateway; the gateway namespace (`openshell-<id>`) is the sole, deterministic linkage, set at Gateway creation. |
| Count active as `Running` or `Pending` pods | This preserves the existing meaning of `active_sandbox_count` and matches what an operator cares about before a disruptive action: sessions that are up or coming up. |
| Adjust the count atomically at the source of truth, floored at zero | Increments and decrements arrive concurrently from independent pod events; a read-modify-write would lose updates. An atomic adjustment with a zero floor keeps the stored value correct and never negative even under duplicate or out-of-order events. |
| Self-heal by reconciling to the watch cache, not by polling | Incremental deltas can drift after a missed event or a restart. Periodically setting the count to the absolute number observed in the in-memory cache converges it to the truth without reintroducing a Kubernetes poll. |
| Keep the field control-plane-owned and read-only | The count is an observed fact about cluster state, not user intent; only the control plane observes it, so only the control plane writes it. |
