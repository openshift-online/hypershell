# OpenShell Gateway Namespace Garbage Collection

**Date:** 2026-08-17
**Status:** Active

## Purpose

This spec defines how the HyperShell control plane reclaims the Kubernetes
namespaces it creates for gateways, so that deleting a Gateway (or a gateway
that never bootstrapped) does not leave orphaned `openshell-*` namespaces behind.
It covers two complementary paths:

1. **Delete-driven cleanup** - when a Gateway is deleted, the control plane
   deletes that gateway's namespace as part of processing the delete event.
2. **Periodic garbage collection** - a background reconciler sweeps managed
   namespaces and reaps any that no longer have a live Gateway backing them
   after a grace period. This recovers namespaces orphaned by a delete event
   missed while the control plane was down, and namespaces whose gateway failed
   to bootstrap and was subsequently deleted.

It also defines how the number of active agent sandboxes in a gateway namespace
is observed and surfaced so an operator can see how many running sessions a
deletion would disrupt before they confirm it.

This spec is a sub-spec of [`control-plane.spec.md`](./control-plane.spec.md)
and complements [`watch-delete-events.spec.md`](./watch-delete-events.spec.md)
(which guarantees delete events carry the resource snapshot the cleanup path
needs) and [`openshell-gateway-health.spec.md`](./openshell-gateway-health.spec.md)
(which owns the Gateway `phase`/`status`). How the active-sandbox count itself is
maintained and published is defined in
[`openshell-gateway-sandbox-count.spec.md`](./openshell-gateway-sandbox-count.spec.md);
this spec only consumes that count to warn an operator before a deletion.
Namespace creation and provisioning mechanics are defined in
[`openshell-gateway.spec.md`](./openshell-gateway.spec.md).

## Domain Vocabulary

- **Gateway namespace** - the Kubernetes namespace a gateway's workloads run in.
  Its name is API-assigned from the Gateway identifier and is prefixed
  `openshell-` (e.g. `openshell-a14873d1631f1b74`).
- **Managed namespace** - a namespace the control plane created and is
  responsible for, identified by carrying BOTH of the labels the control plane
  stamps at creation:
  - `app.kubernetes.io/managed-by=hypershell-control-plane`
  - `hypershell.redhat.io/managed=true`
  Garbage collection acts ONLY on namespaces carrying both labels; a Gateway
  pointed at a pre-existing or shared namespace can never cause that namespace to
  be reaped.
- **Orphaned namespace** - a managed namespace for which no live Gateway exists
  (no Gateway in the API server maps to it). This is the sole trigger for
  garbage collection.
- **GC grace period** - the minimum time a namespace must remain continuously
  orphaned before it is reaped. It is measured from a timestamp persisted on the
  namespace so it survives control-plane restarts.
- **Active sandbox** - an agent sandbox pod in the `Running` or `Pending` phase.
  Sandbox pods are created by the upstream OpenShell gateway (via the
  agent-sandbox controller), not by this control plane, and are identified by the
  `agents.x-k8s.io/sandbox-name-hash` label the agent-sandbox controller stamps on
  them.

## Requirements

### Requirement: Gateway Deletion Reaps the Gateway Namespace

When the control plane processes a Gateway delete event, it SHALL delete the
gateway's managed namespace. Deleting the namespace cascades removal of every
resource inside it, so in-namespace workloads (Deployments, Services, Secrets,
ConfigMaps, PVCs, Jobs, Roles, RoleBindings, cert-manager / Gateway API objects,
and the gateway's agent sandbox pods and their `agents.x-k8s.io` Sandbox
resources) are reclaimed by the namespace deletion itself and need not be deleted
individually. Sandbox pods run in the gateway's own namespace (see
[`openshell-gateway-sandbox-count.spec.md`](./openshell-gateway-sandbox-count.spec.md)),
so they are reclaimed by this cascade and are never garbage-collected pod by pod.

The control plane SHALL additionally clean up the resources a gateway owns that
live outside its namespace, because namespace deletion does not reach them:

- the cluster-scoped ClusterRoleBinding created for the gateway,
- the gateway's external Keycloak client, and
- any credential RBAC the gateway created in a separate credential namespace.

> **Future work (not yet implemented):** As gateways gain ownership of
> out-of-namespace external state that namespace deletion cannot reach (for
> example secrets written to an external secret store such as HashiCorp Vault),
> this delete path will need to be extended to reclaim that state too, under the
> same best-effort, idempotent contract used for the resources above. No such
> external store is provisioned today, so there is nothing beyond the listed
> resources to clean up yet.

Namespace deletion SHALL be best-effort and idempotent: an already-absent or
already-terminating namespace is treated as success, and a namespace that is not
managed by this control plane SHALL NOT be deleted.

Namespace deletion SHALL NOT be gated on the number of active sandboxes. A delete
is processed and the namespace is removed; if the namespace no longer exists when
the delete is processed, the delete is considered complete.

#### Scenario: Gateway deleted with a managed namespace

- GIVEN a Gateway with a managed namespace `openshell-a14873d1631f1b74`
- WHEN the control plane receives the Gateway delete event
- THEN it SHALL delete the namespace `openshell-a14873d1631f1b74`, cascading
  removal of the gateway's in-namespace resources
- AND it SHALL clean up the gateway's out-of-namespace resources (its
  cluster-scoped ClusterRoleBinding, Keycloak client, and any cross-namespace
  credential RBAC)

#### Scenario: Namespace already gone

- GIVEN a Gateway delete event whose namespace has already been deleted
- WHEN the control plane processes the delete
- THEN the delete SHALL be treated as complete (no error)

#### Scenario: Delete does not touch an unmanaged namespace

- GIVEN a Gateway whose namespace does not carry both management labels
- WHEN the control plane processes the Gateway delete event
- THEN it SHALL NOT delete that namespace

### Requirement: Periodic Garbage Collection of Orphaned Namespaces

The control plane SHALL run a background reconciler that periodically lists
managed namespaces and reaps any that have been orphaned (no live Gateway)
for at least the grace period. The sweep interval defaults to 5 minutes and the
grace period defaults to 10 minutes. Garbage collection SHALL be enabled by
default and configurable without code changes via environment variables:

- `GATEWAY_NAMESPACE_GC_ENABLED` (default `true`)
- `GATEWAY_NAMESPACE_GC_INTERVAL` (default `5m`)
- `GATEWAY_NAMESPACE_GC_GRACE_PERIOD` (default `10m`)

Reaping SHALL be best-effort and idempotent, and SHALL only ever delete managed
namespaces.

#### Scenario: Orphaned namespace reaped after grace period

- GIVEN a managed namespace with no live Gateway
- AND it has been continuously orphaned for longer than the grace period
- WHEN the garbage-collection reconciler sweeps
- THEN it SHALL delete the namespace

#### Scenario: Failed-to-bootstrap gateway namespace is reclaimed

- GIVEN a gateway that failed to bootstrap and whose Gateway was then deleted,
  leaving its managed namespace behind
- WHEN the namespace has been orphaned past the grace period
- THEN the garbage-collection reconciler SHALL delete the namespace

#### Scenario: Delete event missed during downtime is recovered

- GIVEN the control plane was down when a Gateway was deleted, so the namespace
  was never reaped by the delete path
- WHEN the control plane restarts and the garbage-collection reconciler sweeps
- THEN it SHALL observe the namespace as orphaned and, once the grace period has
  elapsed, delete it

### Requirement: Grace Period Prevents Premature Deletion

The control plane SHALL NOT delete an orphaned namespace immediately. It SHALL
record, on the namespace itself, the time it was first observed orphaned (in the
`hypershell.redhat.io/gc-eligible-since` annotation, RFC3339) and measure the
grace period from that timestamp, so the delay survives control-plane restarts.
If a Gateway reappears for that namespace, the control plane SHALL clear the
annotation so the timer resets.

#### Scenario: Namespace within grace period is deferred

- GIVEN a managed namespace first observed orphaned less than the grace period ago
- WHEN the reconciler sweeps
- THEN it SHALL NOT delete the namespace
- AND it SHALL leave the `gc-eligible-since` annotation in place

#### Scenario: Grace timer resets when the gateway returns

- GIVEN a managed namespace marked `gc-eligible-since`
- WHEN a live Gateway is again observed for that namespace
- THEN the control plane SHALL clear the `gc-eligible-since` annotation

### Requirement: Do Not Reap Namespaces of Live Gateways

The garbage collector SHALL determine liveness from the set of Gateways reported
by the API server. If it cannot list Gateways, it SHALL abort the entire sweep
rather than treat namespaces as orphaned, so that a transient API failure can
never cause a live gateway's namespace to be reaped. A namespace whose Gateway
still exists SHALL NOT be reaped regardless of the gateway's `phase` (including
`Degraded` or `Failed`); the health of an existing gateway is owned by the health
reconciler, and reclamation is triggered only by the Gateway ceasing to exist.

#### Scenario: Gateway list failure aborts the sweep

- GIVEN the API server is unreachable
- WHEN the garbage-collection reconciler attempts a sweep
- THEN it SHALL NOT delete any namespace

#### Scenario: Degraded gateway's namespace is preserved

- GIVEN a Gateway with `phase` `Degraded` whose namespace is managed
- WHEN the garbage-collection reconciler sweeps
- THEN it SHALL NOT delete that namespace, because the Gateway still exists

### Requirement: Preserve a Durable Record Before Deletion

Before deleting an orphaned namespace, the control plane SHALL record a
Kubernetes Event describing the reap so operators retain a durable record after
the namespace is gone. The Event SHALL be created in the control-plane namespace
(so it outlives the deleted namespace), with reason `GarbageCollected`, and its
message SHALL summarize how long the namespace was orphaned, the state of its
pods, and the number of active sandboxes it held. Gathering the summary is
best-effort: failure to collect it SHALL NOT block the reap.

#### Scenario: GC event recorded on reap

- GIVEN an orphaned managed namespace past its grace period containing pods
- WHEN the reconciler reaps it
- THEN it SHALL create a `GarbageCollected` Event in the control-plane namespace
  identifying the namespace and summarizing its workloads and active sandbox count
- AND then delete the namespace

### Requirement: Surface Active Sandbox Count Before Deletion

So an operator can see how many running sessions a deletion would disrupt, the
Gateway's read-only `active_sandbox_count` field SHALL be surfaced as a warning
before the gateway is deleted. How that count is maintained and published is
defined in
[`openshell-gateway-sandbox-count.spec.md`](./openshell-gateway-sandbox-count.spec.md)
(event-driven from a sandbox pod watch, with periodic self-heal); this spec only
consumes it. The count is an observability signal only: it SHALL NOT gate
namespace deletion.

The reported value reflects the control plane's most recent observation and MAY
lag real time; consumers SHALL treat it as an advisory recent count, not a
real-time guarantee.

#### Scenario: Count surfaced in the delete confirmation

- GIVEN a Gateway reporting `active_sandbox_count = 3`
- WHEN an operator initiates deletion of that gateway in the console
- THEN the console SHALL surface the active sandbox count as a warning
- BUT it SHALL NOT block the deletion on that count

#### Scenario: Count is self-healing, not clobbered by a missed observation

- GIVEN a gateway namespace containing three active sandbox pods
- WHEN an event is missed or the control plane restarts
- THEN the control plane SHALL converge `active_sandbox_count` back to the actual
  number of active sandbox pods (per the sandbox-count spec), so the delete
  warning is based on a self-correcting value rather than a stale or zeroed one

## Design Decisions

| Decision | Rationale |
|----------|-----------|
| GC triggers on orphaning (no live Gateway), not on `phase` being `Degraded`/`Failed` | A gateway that still exists - even if unhealthy - is the health reconciler's and the operator's concern; only the absence of a backing Gateway unambiguously means the namespace is garbage. This avoids reaping a namespace an operator is still debugging. |
| Require BOTH management labels before deleting | Defense in depth: even if a label selector over-returns, a namespace not created by this control plane (e.g. a shared or pre-existing namespace) is never deleted. |
| Grace period persisted on the namespace annotation | The delay must survive control-plane restarts; storing `gc-eligible-since` on the namespace makes the timer durable without a separate store. |
| Abort the whole sweep if Gateways cannot be listed | An empty or failed Gateway list would make every managed namespace look orphaned; aborting is the only safe response to avoid mass reaping of live namespaces. |
| Delete is best-effort and not gated on sandbox count | Deletion is idempotent - process the delete, remove the namespace, and if it is already gone consider the delete done. The sandbox count is a warning surfaced to the operator, not a backend precondition. |
| Record the GC Event in the control-plane namespace | An Event stored in the namespace being deleted would be destroyed with it; recording it in the control-plane namespace gives operators a durable audit trail. |
| Active sandbox count maintained event-driven, not by this reconciler | The count is maintained from a control-plane sandbox pod watch with periodic self-heal (see [`openshell-gateway-sandbox-count.spec.md`](./openshell-gateway-sandbox-count.spec.md)), avoiding a repeated full-namespace pod poll. This reconciler only consumes the published value to warn operators before a deletion. |
| Sandbox pods reclaimed by the namespace cascade, not reaped pod by pod | Sandboxes run in the gateway's own namespace, so deleting the namespace reclaims them along with the gateway's workloads; no separate per-pod sandbox reaper is needed, and the sandbox count stays a warning signal rather than a cleanup driver. |
| Out-of-namespace cleanup is an explicit, extensible list | Namespace deletion cannot reach resources outside the namespace, so each must be deleted explicitly. The list is expected to grow (e.g. external secret stores such as Vault); new out-of-namespace state is added here under the same best-effort, idempotent contract. |
