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
- **Control-plane instance** - one HyperShell control plane (the
  `hypershell-controller` in its platform namespace). Its identity is unique to
  that controller: the Kubernetes namespace the controller pod runs in, which is
  unique on the cluster. In cluster this is `HYPERSHELL_NAMESPACE`, taken from the
  pod's own namespace via the downward API so two controllers cannot accidentally
  share a copied static value. Multiple instances MAY share a cluster (for
  example stage alongside an e2e run, or two developers' local-dev environments).
  Each instance has its own API server and therefore its own set of live Gateways.
- **Managed namespace** - a namespace this control-plane instance created and is
  responsible for, identified by carrying ALL of the labels the control plane
  stamps at creation:
  - `app.kubernetes.io/managed-by=hypershell-control-plane`
  - `hypershell.redhat.io/managed=true`
  - `hypershell.redhat.io/instance=<HYPERSHELL_NAMESPACE>`
  Periodic garbage collection sweeps only namespaces owned by this instance whose
  names match the gateway prefix (`openshell-<hex>`) and excludes ManagedDatabase
  namespaces (`openshell-db-<hex>`). A namespace owned by a different instance is
  never listed, annotated, or reaped. A gateway namespace that carries both
  management labels but no instance label is unlabeled leftover. Periodic GC
  SHALL leave it unlabeled. An operator labels it for the owning instance when
  they want that instance's sweep to consider it. ManagedDatabase namespaces and
  namespaces already labeled for another instance are never claimed.
  A Gateway pointed at a pre-existing or shared namespace can never cause that
  namespace to be reaped.
- **Orphaned namespace** - a gateway namespace (matching the gateway prefix, not
  the database prefix) for which no live Gateway exists (no Gateway in the API
  server maps to it). This is the sole trigger for garbage collection.
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
managed by this control-plane instance SHALL NOT be deleted. A namespace that
carries `hypershell.redhat.io/instance` set to a different instance SHALL NOT be
deleted. A legacy namespace that carries the two management labels but no
instance label MAY still be deleted on this path, because the delete is keyed to
a Gateway from this instance's API server rather than a cluster-wide sweep.

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

#### Scenario: Delete does not touch another instance's namespace

- GIVEN a Gateway delete event whose namespace carries
  `hypershell.redhat.io/instance` set to a different control-plane instance
- WHEN the control plane processes the Gateway delete event
- THEN it SHALL NOT delete that namespace

### Requirement: Periodic Garbage Collection of Orphaned Namespaces

The control plane SHALL run a background reconciler that periodically lists
namespaces owned by this control-plane instance and reaps gateway workload
namespaces (`openshell-<hex>`, excluding ManagedDatabase namespaces
`openshell-db-<hex>`) that have been orphaned (no live Gateway in this instance's
API server) for at least the grace period. The list selector SHALL include
`hypershell.redhat.io/instance=<HYPERSHELL_NAMESPACE>` in addition to the two
management labels, so a namespace created by another HyperShell instance on the
same cluster is never observed as an orphan of this instance. If
`HYPERSHELL_NAMESPACE` is empty, the reconciler SHALL abort the sweep rather than
list by the generic management labels alone.

The sweep SHALL NOT stamp `hypershell.redhat.io/instance` onto unlabeled
namespaces. Unlabeled leftovers stay unlabeled until an operator assigns the
owning instance. Live Gateways this instance reconciles still receive the
instance label through `EnsureManagedNamespace`. The sweep interval defaults to
5 minutes and the grace period defaults to 10 minutes. Garbage collection SHALL
be enabled by default and configurable without code changes via environment
variables:

- `GATEWAY_NAMESPACE_GC_ENABLED` (default `true`)
- `GATEWAY_NAMESPACE_GC_INTERVAL` (default `5m`)
- `GATEWAY_NAMESPACE_GC_GRACE_PERIOD` (default `10m`)

Reaping SHALL be best-effort and idempotent, and SHALL only ever delete gateway
workload namespaces owned by this instance (matching the gateway prefix, not the
database prefix, and carrying this instance's identity label).

#### Scenario: Orphaned gateway namespace reaped after grace period

- GIVEN a namespace owned by this control-plane instance with a gateway-prefixed
  name (`openshell-<hex>`, not `openshell-db-<hex>`) with no live Gateway in this
  instance's API server
- AND it has been continuously orphaned for longer than the grace period
- WHEN the garbage-collection reconciler sweeps
- THEN it SHALL delete the namespace

#### Scenario: Another instance's gateway namespace is not treated as orphaned

- GIVEN two HyperShell control-plane instances share a cluster
- AND instance A has live Gateways whose namespaces are labeled
  `hypershell.redhat.io/instance=<A's HYPERSHELL_NAMESPACE>`
- WHEN instance B's garbage-collection reconciler sweeps
- THEN it SHALL NOT list, annotate, or delete instance A's namespaces
- AND it SHALL NOT record them as orphaned of instance B

#### Scenario: Unlabeled leftover is left for an operator

- GIVEN a namespace that carries the two management labels but not
  `hypershell.redhat.io/instance`
- AND its name is gateway-prefixed (`openshell-<hex>`, not `openshell-db-<hex>`)
- AND no live Gateway in this instance's API server maps to it
- WHEN this instance's garbage-collection reconciler sweeps
- THEN it SHALL NOT stamp `hypershell.redhat.io/instance`
- AND it SHALL NOT stamp `gc-eligible-since` or delete that namespace

#### Scenario: Live unlabeled gateway namespace is labeled on reconcile, not by GC

- GIVEN a namespace that carries the two management labels but not
  `hypershell.redhat.io/instance`
- AND a live Gateway in this instance's API server maps to it
- WHEN the control plane reconciles that Gateway
- THEN it SHALL stamp `hypershell.redhat.io/instance` for this instance
- AND the garbage-collection sweep SHALL NOT stamp that label on its own

#### Scenario: Unlabeled ManagedDatabase namespace is not claimed

- GIVEN a namespace named `openshell-db-<hex>` that carries the two management
  labels but not `hypershell.redhat.io/instance`
- WHEN this instance's garbage-collection reconciler sweeps
- THEN it SHALL NOT stamp an instance label on that namespace
- AND it SHALL NOT annotate or delete that namespace

#### Scenario: ManagedDatabase namespace is not reaped

- GIVEN a managed namespace named `openshell-db-<hex>` with no live Gateway
- WHEN the garbage-collection reconciler sweeps
- THEN it SHALL NOT delete that namespace

#### Scenario: Failed-to-bootstrap gateway namespace is reclaimed

- GIVEN a gateway that failed to bootstrap and whose Gateway was then deleted,
  leaving its managed namespace behind
- WHEN the namespace has been orphaned past the grace period
- THEN the garbage-collection reconciler SHALL delete the namespace

#### Scenario: Delete event missed during downtime is recovered

- GIVEN the control plane was down when a Gateway is deleted, so the namespace
  was never reaped by the delete path
- AND the namespace already carries this instance's identity label
- WHEN the control plane restarts and the garbage-collection reconciler sweeps
- THEN it SHALL observe the namespace as orphaned and, once the grace period has
  elapsed, delete it

### Requirement: Stamp This Instance's Identity on Namespaces It Manages

When the control plane creates or reconciles a namespace it owns, it SHALL stamp
`hypershell.redhat.io/instance` with a value unique to that controller (the
namespace the controller pod runs in, `HYPERSHELL_NAMESPACE`) together with the
two management labels. If the namespace already carries a different instance
identity, the control plane SHALL NOT adopt, relabel, or delete it. Reconciling a
live Gateway SHALL add the instance label to a legacy unlabeled namespace this
instance is actively managing, so a later orphan can be reaped by this instance's
periodic GC. Periodic GC SHALL NOT stamp instance labels onto unlabeled
namespaces. An operator who wants an unlabeled leftover swept assigns
`hypershell.redhat.io/instance` to the owning control-plane namespace. An empty
`HYPERSHELL_NAMESPACE` SHALL NOT create or relabel a managed namespace. Two
controllers on the same cluster SHALL NOT share an instance identity.

#### Scenario: Created gateway namespace is labeled for this instance

- GIVEN a Gateway ADDED event for a namespace that does not yet exist
- WHEN the control plane creates the namespace
- THEN the namespace SHALL carry `hypershell.redhat.io/instance` equal to this
  controller's unique identity (`HYPERSHELL_NAMESPACE`, the namespace the
  controller pod runs in)

#### Scenario: Live reconcile labels a legacy namespace this instance owns

- GIVEN a live Gateway whose namespace carries the two management labels but no
  instance label
- WHEN the control plane reconciles that Gateway
- THEN it SHALL stamp `hypershell.redhat.io/instance` for this instance
- AND it SHALL NOT change an instance label that already identifies a different
  instance

#### Scenario: Foreign instance namespace is not adopted

- GIVEN a namespace labeled `hypershell.redhat.io/instance` for a different
  control-plane instance
- WHEN this instance would create or reconcile a Gateway in that namespace
- THEN it SHALL NOT overwrite the instance label
- AND it SHALL NOT deploy into or delete that namespace

#### Scenario: Periodic GC backfills unlabeled legacy gateway namespaces

- GIVEN a gateway-prefixed namespace that carries the two management labels
  but no instance label
- WHEN the garbage-collection reconciler sweeps
- THEN it SHALL stamp `hypershell.redhat.io/instance` for this instance
- AND it SHALL NOT change an instance label that already identifies a different
  instance

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

The liveness set that seeds a sweep is a point-in-time snapshot and can be many
minutes stale by the time a given namespace is evaluated for deletion. To close
that window, the control plane SHALL re-confirm liveness immediately before the
destructive delete: if a Gateway now maps to the namespace, it SHALL NOT delete
it and SHALL clear the `gc-eligible-since` annotation instead. If it cannot
re-confirm liveness at that point (the Gateway list fails), it SHALL defer the
delete to a later sweep rather than delete on a stale view.

#### Scenario: Gateway list failure aborts the sweep

- GIVEN the API server is unreachable
- WHEN the garbage-collection reconciler attempts a sweep
- THEN it SHALL NOT delete any namespace

#### Scenario: Degraded gateway's namespace is preserved

- GIVEN a Gateway with `phase` `Degraded` whose namespace is managed
- WHEN the garbage-collection reconciler sweeps
- THEN it SHALL NOT delete that namespace, because the Gateway still exists

#### Scenario: Gateway created after the sweep began is spared at delete time

- GIVEN a managed namespace observed orphaned past its grace period when the
  sweep captured its liveness snapshot
- AND a Gateway is created for that namespace before the reconciler reaches the
  delete
- WHEN the reconciler re-confirms liveness immediately before deleting
- THEN it SHALL NOT delete the namespace
- AND it SHALL clear the `gc-eligible-since` annotation

#### Scenario: Liveness re-check failure defers the delete

- GIVEN a managed namespace orphaned past its grace period by the sweep snapshot
- AND the Gateway list fails when liveness is re-confirmed before the delete
- WHEN the reconciler handles that namespace
- THEN it SHALL NOT delete the namespace in that pass
- AND it SHALL defer the reap to a later sweep

### Requirement: Preserve a Durable Record Before Deletion

Before deleting an orphaned namespace, the control plane SHALL record a
Kubernetes Event describing the reap so operators retain a durable record after
the namespace is gone. The Event SHALL be created in the control-plane namespace
(so it outlives the deleted namespace), with reason `GarbageCollected`, and its
message SHALL summarize how long the namespace was orphaned, the state of its
pods, and the number of active sandboxes it held. Gathering the summary is
best-effort: failure to collect it SHALL NOT block the reap. If recording the
Event itself fails, the reconciler SHALL defer deletion and retry in a later
sweep so the durable record is preserved.

#### Scenario: GC event recorded on reap

- GIVEN an orphaned managed namespace past its grace period containing pods
- WHEN the reconciler reaps it
- THEN it SHALL create a `GarbageCollected` Event in the control-plane namespace
  identifying the namespace and summarizing its workloads and active sandbox count
- AND then delete the namespace

#### Scenario: Event write failure defers deletion

- GIVEN an orphaned managed namespace past its grace period
- AND Event creation in the control-plane namespace fails
- WHEN the reconciler handles that namespace
- THEN it SHALL return an error for the current sweep item
- AND it SHALL NOT delete the namespace in that pass

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
| Exclude `openshell-db-*` managed namespaces from periodic GC | ManagedDatabase CNPG namespaces share the management labels but are owned by the ManagedDatabase reconciler; the stable `openshell-db-` prefix distinguishes them without requiring a label migration on existing gateway namespaces. |
| Require this instance's identity label before periodic GC | Two HyperShell controllers on one cluster share the generic management labels. Without a value unique to that controller (`hypershell.redhat.io/instance=<the controller's namespace>`), instance B's sweep treats instance A's live gateways as orphans (they are absent from B's API server) and would delete them after the grace period. The identity is the controller pod's namespace from the downward API so it cannot be a copied static string. |
| Claim unlabeled legacy gateway namespaces at the start of each GC sweep | The instance label was added after namespaces already existed. `EnsureManagedNamespace` only stamps live Gateways; a missed-delete orphan never hits that path and would leak forever if unlabeled namespaces stayed excluded from the sweep. Each sweep lists namespaces that carry both management labels and no instance label, stamps this instance on gateway-prefixed names, then proceeds with the instance-scoped orphan sweep. Namespaces already labeled for another instance are never claimed. Two HyperShell instances that both still have unlabeled namespaces on the same cluster (neither has been upgraded to stamp the instance label) are unsupported: upgrade both, or label namespaces before enabling GC. |
| Require BOTH management labels plus this instance's identity before deleting | Defense in depth: even if a label selector over-returns, a namespace not created by this control-plane instance (another HyperShell, a shared namespace, or a pre-existing namespace) is never deleted by periodic GC. |
| Stamp the instance label on create, on live reconcile, and on GC backfill | Periodic GC can only reap namespaces this instance labeled. Stamping at create covers new gateways; stamping on live reconcile migrates this instance's still-live pre-label namespaces; GC backfill migrates unlabeled leftovers that have no live Gateway so the missed-delete path can proceed. |
| Grace period persisted on the namespace annotation | The delay must survive control-plane restarts; storing `gc-eligible-since` on the namespace makes the timer durable without a separate store. |
| Abort the whole sweep if Gateways cannot be listed | An empty or failed Gateway list would make every managed namespace look orphaned; aborting is the only safe response to avoid mass reaping of live namespaces. |
| Delete is best-effort and not gated on sandbox count | Deletion is idempotent - process the delete, remove the namespace, and if it is already gone consider the delete done. The sandbox count is a warning surfaced to the operator, not a backend precondition. |
| Record the GC Event in the control-plane namespace | An Event stored in the namespace being deleted would be destroyed with it; recording it in the control-plane namespace gives operators a durable audit trail. |
| Active sandbox count maintained event-driven, not by this reconciler | The count is maintained from a control-plane sandbox pod watch with periodic self-heal (see [`openshell-gateway-sandbox-count.spec.md`](./openshell-gateway-sandbox-count.spec.md)), avoiding a repeated full-namespace pod poll. This reconciler only consumes the published value to warn operators before a deletion. |
| Sandbox pods reclaimed by the namespace cascade, not reaped pod by pod | Sandboxes run in the gateway's own namespace, so deleting the namespace reclaims them along with the gateway's workloads; no separate per-pod sandbox reaper is needed, and the sandbox count stays a warning signal rather than a cleanup driver. |
| Out-of-namespace cleanup is an explicit, extensible list | Namespace deletion cannot reach resources outside the namespace, so each must be deleted explicitly. The list is expected to grow (e.g. external secret stores such as Vault); new out-of-namespace state is added here under the same best-effort, idempotent contract. |
