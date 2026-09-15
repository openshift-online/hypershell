# Gateway Deletion and Finalization Specification

**Date:** 2026-09-14
**Status:** Draft

## Purpose

This spec defines the authoritative, cross-cutting contract for **complete
deletion and finalization of a Gateway**: how the platform reclaims every
resource a Gateway owns, how deletion behaves safely across transient API and
gRPC disconnections, and when a Gateway is considered fully finalized. It is an
umbrella spec that unifies the deletion behavior already specified per resource
class and closes the gaps between those specs so that no owned resource is
orphaned silently.

HyperShell does not model Gateway deletion with a Kubernetes finalizer or a
`deletionTimestamp`. A Gateway is deleted by soft-deleting its Postgres row and
emitting a watch `DELETED` event; the control plane performs the actual resource
teardown asynchronously, driven by that event through the reconcile queue.
"Finalization" in this spec therefore means the platform-level equivalent of a
finalizer: the ordered set of guarantees that gate a Gateway's record from being
finally removed, and that keep its teardown durable until it completes or is
handed to an explicit recovery path.

This spec is a sub-spec of [`control-plane.spec.md`](./control-plane.spec.md)
and coordinates the deletion requirements defined in:

- [`openshell-gateway-namespace-gc.spec.md`](./openshell-gateway-namespace-gc.spec.md)
  - namespace cascade delete and the periodic namespace garbage-collection recovery path.
- [`watch-delete-events.spec.md`](./watch-delete-events.spec.md)
  - the delete event carries the resource snapshot the teardown needs.
- [`openshell-gateway-keycloak.spec.md`](./openshell-gateway-keycloak.spec.md)
  - gateway and console Keycloak client cleanup and recorded-identity resolution.
- [`openshell-gateway-service-accounts.spec.md`](./openshell-gateway-service-accounts.spec.md)
  - service-account client revocation and deletion, and the API-server pre-delete barrier.
- [`openshell-gateway-database.spec.md`](./openshell-gateway-database.spec.md)
  - per-gateway database cleanup and ManagedDatabase deletion.
- [`openshell-gateway-credentials.spec.md`](./openshell-gateway-credentials.spec.md)
  - credential-namespace RBAC cleanup.

Where a per-resource spec already defines a resource's cleanup behavior, that
spec remains authoritative for that resource. This spec defines the enumeration
that binds them together, the resume-on-disconnect contract, and the
finalization and no-silent-orphan guarantees that span all of them.

## Domain Vocabulary

- **Gateway-owned resource** - any resource whose lifecycle is bound to a single
  Gateway and which the platform created for that Gateway. Owned resources fall
  into two location classes:
  - **In-namespace resources** - resources inside the gateway's managed
    namespace (`openshell-<hex>`). These are normally reclaimed by the namespace
    delete cascade.
  - **Out-of-namespace resources** - resources that live outside the gateway's
    namespace and are not reached by the namespace cascade: the cluster-scoped
    ClusterRoleBinding, the gateway/console/service-account Keycloak clients, the
    per-gateway database resources (in the ManagedDatabase namespace or the
    dedicated ManagedDatabase), and any credential-namespace RBAC.
- **Finalization** - the point at which a Gateway's teardown is complete: every
  owned resource has been reclaimed or confirmed already absent, or its residue
  has been handed to an explicit, documented recovery path. The soft-deleted row
  and its delete tombstone are the durable record that keeps teardown retrying
  until finalization.
- **Finalization barrier** - a pre-removal gate that prevents a Gateway record
  from being finally removed until a required cleanup precondition is met. Today
  the only such barrier is the API-server service-account cleaner
  (`cleanBeforeDeletion`); this spec generalizes the concept.
- **Recovery path** - an explicit, durable mechanism that reclaims an owned
  resource whose teardown could not complete inline: the periodic namespace
  garbage collector, and Keycloak orphan-client reconciliation keyed on client
  attributes. A recovery path is a specified, operable behavior, not a manual
  ad-hoc step.
- **Silent orphan** - an owned resource that is left behind after finalization
  without any durable, operator-visible signal and without a recovery path that
  will reclaim it. Silent orphans are prohibited.

## Requirements

### Requirement: Deletion Enumerates Every Gateway-Owned Resource Class

When the control plane processes a Gateway delete event, it SHALL reconcile
every gateway-owned resource class toward absence. The complete set of owned
resource classes is:

1. The gateway's managed namespace and, by cascade, all in-namespace resources
   (Deployment, Service, Secrets including the KEK and database-credentials
   Secrets, ConfigMaps, PVCs, ServiceAccount, Roles/RoleBindings,
   NetworkPolicies, cert-manager Certificates, Gateway API objects, the
   OpenShift Route, console workloads, and the gateway's agent sandbox pods and
   their Sandbox resources).
2. The cluster-scoped ClusterRoleBinding created for the gateway.
3. The gateway's Keycloak clients: the gateway client, the console client, and
   every service-account client.
4. The gateway's database resources: in CNPG mode the per-gateway `Database`,
   `DatabaseRole`, and password Secret in the ManagedDatabase namespace; in
   deployment mode the gateway's dedicated ManagedDatabase (whose own deletion
   reclaims the Deployment, Service, PVC, credentials Secret, and namespace).
5. Any credential-namespace RBAC (Role and RoleBinding) the gateway created in a
   separate credential namespace.

The enumeration SHALL be authoritative and SHALL NOT depend on the namespace
delete cascade reaching a resource. Every out-of-namespace resource class SHALL
be reconciled explicitly on the delete-event path, because the namespace cascade
cannot reach it.

#### Scenario: Normal deletion reclaims all owned resource classes

- GIVEN a running Gateway with a managed namespace, a ClusterRoleBinding,
  provisioned Keycloak clients, per-gateway database resources, and (if
  configured) credential-namespace RBAC
- WHEN the control plane processes the Gateway delete event
- THEN it SHALL delete the managed namespace, cascading removal of its
  in-namespace resources
- AND it SHALL delete the ClusterRoleBinding, the Keycloak gateway, console, and
  service-account clients, the per-gateway database resources, and the
  credential-namespace RBAC
- AND when every class is absent it SHALL report the delete complete

#### Scenario: Owned resource is reclaimed even when the namespace survives

- GIVEN a Gateway whose managed namespace cannot be deleted by the cascade
  (for example the namespace is shared, pre-existing, or already terminating)
- WHEN the control plane processes the Gateway delete event
- THEN it SHALL still reconcile every in-namespace owned resource toward absence
  by an owned-resource sweep rather than relying on the namespace cascade
- AND it SHALL still reconcile every out-of-namespace owned resource toward absence

### Requirement: In-Namespace Owned-Resource Sweep Covers Every Owned Type

When the managed namespace is not deleted (it survives, is unmanaged, is shared,
or belongs to another instance), the control plane SHALL reclaim the gateway's
in-namespace owned resources by an owned-resource sweep. The sweep SHALL cover
every owned resource type the platform can create in a gateway namespace, so a
newly introduced owned type is not leaked when the cascade does not run. The
sweep SHALL identify owned resources by the management label
(`hypershell.redhat.io/managed=true`) and SHALL treat an absent resource type
(for example a missing optional CRD) and an already-absent resource as success.

#### Scenario: Surviving namespace still has its gateway resources swept

- GIVEN a Gateway delete event whose managed namespace survives the delete
- WHEN the control plane sweeps the namespace for owned resources
- THEN it SHALL delete every resource carrying the management label across all
  owned resource types
- AND a resource type whose CRD is not installed SHALL be skipped without failing
  the sweep

#### Scenario: New owned resource type is not leaked

- GIVEN the platform gains a new owned in-namespace resource type
- WHEN a Gateway whose namespace survives is deleted
- THEN the owned-resource sweep SHALL include that new type
- AND the resource SHALL NOT be left behind

### Requirement: Partial Deletion Failures Are Aggregated, Retried, and Never Silently Swallowed

Deletion SHALL attempt every owned-resource class regardless of an individual
class's failure, and SHALL aggregate the failures rather than stop at the first
one. A cleanup step whose failure is transient or retryable SHALL cause the
delete event to be retried until it succeeds; the delete SHALL NOT be reported
complete while a required owned resource remains present and reclaimable. An
already-absent resource (Kubernetes `NotFound`, or an empty Keycloak lookup)
SHALL be treated as success for that class. No owned-resource cleanup failure
SHALL be swallowed without either propagating for retry or being recorded through
the no-silent-orphan signal below.

#### Scenario: One resource class fails, the rest still run

- GIVEN a Gateway delete where deleting the database resources fails transiently
- WHEN the control plane processes the delete
- THEN it SHALL still attempt namespace, ClusterRoleBinding, Keycloak, and
  credential-RBAC cleanup
- AND it SHALL aggregate the database failure and return an error so the delete
  event is retried
- AND the delete SHALL NOT be reported complete until the database resources are
  absent

#### Scenario: Already-absent resources make deletion succeed

- GIVEN a Gateway delete event replayed after its owned resources were already
  reclaimed
- WHEN the control plane processes the replayed delete
- THEN every cleanup step SHALL treat its target as already absent
- AND the delete SHALL be reported complete without error

### Requirement: Transient API and gRPC Disconnection Pauses Safely

Deletion SHALL be safe across a transient loss of the API server or the watch
stream. When a read the teardown depends on cannot be confirmed (for example
resolving the gateway's database configuration, or confirming liveness), the
control plane SHALL NOT make a destructive assumption from the unconfirmed read;
it SHALL defer and retry rather than proceed on a stale or failed view. The watch
client SHALL reconnect with bounded backoff, and the delete event SHALL NOT be
silently dropped: a delete event whose enrichment cannot be loaded SHALL be
surfaced (logged as a warning and, where a recovery path does not otherwise
cover it, recorded through the no-silent-orphan signal) rather than discarded
such that only the namespace garbage collector could ever recover it.

#### Scenario: API unreachable during teardown defers the delete

- GIVEN a Gateway delete is in progress
- WHEN the API server becomes unreachable so a required read fails
- THEN the control plane SHALL NOT treat the unread state as "already cleaned"
- AND it SHALL return an error so the delete event is retried later
- AND it SHALL NOT delete or skip a resource based on the failed read

#### Scenario: Watch stream drops mid-deletion and reconnects

- GIVEN a Gateway delete has partially completed when the watch stream drops
- WHEN the watch client reconnects with bounded backoff
- THEN the pending delete SHALL still be retried from the reconcile queue
- AND the completed steps SHALL be idempotent so re-running them causes no error

### Requirement: Delete Intent Survives Reconnects and Process Restarts

A Gateway delete, once observed, SHALL remain durable until finalization. The
reconcile queue SHALL retry a failed delete indefinitely with bounded backoff,
and pending delete retries SHALL survive a watch reconnect. A delete SHALL win
event coalescing: a non-delete event SHALL NOT overwrite or resurrect a pending
delete, and after a successful delete a tombstone SHALL prevent a stale inventory
snapshot from re-provisioning the gone Gateway. Because the delete event carries
the resource snapshot, teardown SHALL be possible after a control-plane restart
with no in-memory state.

#### Scenario: Delete retries persist across a restart via the snapshot

- GIVEN the control plane restarts with no in-memory state while a Gateway delete
  is pending
- WHEN it receives the Gateway delete event carrying the resource snapshot
- THEN it SHALL read the namespace and other fields from the snapshot
- AND it SHALL reclaim the owned resources without depending on any in-memory cache

#### Scenario: A stale create snapshot cannot resurrect a deleted gateway

- GIVEN a Gateway was deleted and its teardown completed, leaving a tombstone
- WHEN a stale inventory seed still lists that Gateway
- THEN the control plane SHALL NOT re-provision it
- AND the tombstone SHALL be preserved

### Requirement: Finalization Gates Record Removal on Required Cleanup or a Recovery Path

A Gateway SHALL NOT be considered finalized until each owned resource class is
either reclaimed (or confirmed absent) or has been handed to an explicit,
documented recovery path. Required-inline cleanup and recovery-path cleanup are
distinguished as follows:

- **Required-inline** classes SHALL be reclaimed by the delete-event teardown and
  keep the delete retrying until they are absent: the service-account Keycloak
  clients (which SHALL be disabled and deleted before the gateway and console
  clients are removed), the per-gateway database resources, and the managed
  namespace or the in-namespace owned-resource sweep.
- **Recovery-path** classes MAY be handed to a documented recovery path when
  inline cleanup cannot complete: the managed namespace is recoverable by the
  periodic namespace garbage collector, and leaked Keycloak clients are
  recoverable by orphan-client reconciliation keyed on client attributes.

Where an API-server pre-delete barrier exists, it SHALL gate the Postgres
soft-delete on its required precondition and SHALL return a retryable error
(rather than deleting the row) when that precondition cannot be met. The
service-account barrier SHALL remain authoritative for service-account cleanup as
specified in
[`openshell-gateway-service-accounts.spec.md`](./openshell-gateway-service-accounts.spec.md).

#### Scenario: Service accounts are cleaned before the gateway client and record

- GIVEN a Gateway with active service-account clients
- WHEN the Gateway is deleted
- THEN the service-account clients SHALL be disabled and deleted before the
  gateway and console Keycloak clients are removed
- AND the Postgres row SHALL NOT be finalized while the service-account cleanup
  precondition is unmet, which SHALL surface as a retryable error rather than a
  silent success

#### Scenario: Namespace teardown that cannot complete is owned by the recovery path

- GIVEN a Gateway delete whose managed namespace deletion could not be completed
  inline
- WHEN inline teardown finishes its other classes
- THEN the surviving namespace SHALL be recoverable by the periodic namespace
  garbage collector
- AND that hand-off SHALL be a specified recovery path, not a manual step

### Requirement: No Owned Resource Is Orphaned Silently

The platform SHALL NOT leave credentials, databases, routes, or namespaces (or
any other owned resource) behind without either a recovery path that will reclaim
them or a durable, operator-visible signal that records the residue. When
deletion deliberately downgrades a failure to non-fatal (for example an invalid
or unresolvable stored Keycloak identity, or a deconfigured Keycloak provisioner)
and no recovery path will reclaim the affected resource, the control plane SHALL
record a durable, operator-visible signal that names the owned resource kind, its
identifier, and the reason it was not reclaimed. A controller log line alone is
not a sufficient signal for a resource with no recovery path. The durable signal
SHALL be recorded where it outlives the deleted resources (for example a
Kubernetes Event in the control-plane namespace, consistent with the garbage
collector's `GarbageCollected` Event) and SHALL NOT contain secrets.

#### Scenario: Leaked Keycloak client with no recovery is recorded durably

- GIVEN a Gateway delete where the Keycloak provisioner is deconfigured, so the
  gateway's Keycloak client cannot be deleted inline
- WHEN the delete otherwise completes
- THEN the control plane SHALL record a durable, operator-visible signal naming
  the Keycloak client identifier and the reason it was not reclaimed
- AND that signal SHALL survive the deletion of the gateway's namespace
- AND it SHALL NOT contain any secret

#### Scenario: Invalid stored identity does not silently orphan the client

- GIVEN a Gateway delete whose stored OIDC client identity is invalid or not
  owned by the gateway
- WHEN cleanup declines to call Keycloak for that identity
- THEN it SHALL continue namespace and database cleanup
- AND it SHALL record a durable, operator-visible signal of the unreclaimed
  client for operator recovery
- AND it SHALL NOT report the client reclaimed

### Requirement: Deletion Is Idempotent and Re-runnable

Every deletion step SHALL be idempotent: re-running the delete against
already-absent, already-terminating, or partially-cleaned state SHALL converge to
absence without error. Replaying the delete event any number of times SHALL
produce the same final state and SHALL NOT recreate, resurrect, or partially
recreate any owned resource.

#### Scenario: Repeated delete replays converge without error

- GIVEN a Gateway delete event delivered more than once
- WHEN the control plane processes each delivery
- THEN each SHALL reconcile the owned resources toward absence
- AND once all are absent, each subsequent delivery SHALL succeed as a no-op

#### Scenario: Already-terminating namespace is treated as success

- GIVEN a Gateway delete whose managed namespace is already terminating
- WHEN the control plane processes the delete
- THEN it SHALL treat the terminating namespace as success and SHALL NOT error

## Design Decisions

| Decision | Rationale |
|----------|-----------|
| Finalization is modeled on soft-delete + delete tombstone + reconcile-queue retry, not a Kubernetes finalizer | The Gateway's source of truth is Postgres, not a Kubernetes object, so there is no object to carry a finalizer or `deletionTimestamp`. Durability comes from the soft-deleted row, the delete event's resource snapshot, and the reconcile queue's indefinite bounded-backoff retry with anti-resurrection tombstones. |
| Enumeration is authoritative and independent of the namespace cascade | The namespace cascade cannot reach out-of-namespace resources, and it does not run at all when the namespace is shared, pre-existing, or already terminating. Enumerating every owned class explicitly prevents a leak whenever the cascade is absent or incomplete. |
| In-namespace sweep must cover every owned type | The label-based fallback sweep is the only cleanup when the cascade does not run. If it enumerates a fixed subset of types, a newly added owned type leaks silently. Requiring full coverage keeps the fallback honest as the resource set grows. |
| Required-inline vs recovery-path split | Some residue has a real recovery path (namespaces via periodic GC; Keycloak clients via attribute-keyed orphan reconciliation) and can be safely handed off; other residue (databases, service-account clients) has no sweep and must be retried inline until it succeeds. The split makes explicit which failures keep the delete retrying and which are legitimately deferred. |
| No-silent-orphan requires a durable signal, not just a log | Once the Postgres row is soft-deleted, a controller log line is the only trace of a leaked out-of-namespace resource with no recovery path, and it is easily lost. A durable Kubernetes Event in the control-plane namespace (mirroring the GC `GarbageCollected` Event) gives operators a queryable record without exposing secrets. |
| Unconfirmed reads defer rather than assume | Treating a failed read as "already cleaned" during an API outage would let a transient disconnection cause a real orphan. Deferring to a later retry is the only safe response, consistent with the namespace GC's abort-the-sweep-on-list-failure rule. |
| Delete events must not be silently dropped | Dropping a delete event for a cluster-scoped subscriber when enrichment fails shifts all recovery onto the namespace GC, which cannot reclaim Keycloak clients or databases. Surfacing the drop (and recording it when no recovery path covers it) preserves the no-silent-orphan guarantee. |
