# Control Plane World Synchronization Specification

**Date:** 2026-08-22
**Status:** Draft

## Purpose

The HyperShell control plane uses gRPC watch streams for low-latency delivery of desired-state changes from the API server, while PostgreSQL-backed API resources remain the source of truth. Watch events are transient and can be missed during a stream interruption, subscriber buffer overflow, or controller restart. This specification defines periodic world synchronization: a bounded, repeatable inventory pass that gives every API-backed reconciler another opportunity to converge Kubernetes state with complete desired state, without replacing watch streams, creating reconciliation loops, or performing cleanup from an incomplete inventory.

## Terms

- **Resource revision** - a monotonically increasing, API-server-assigned revision for one immutable resource ID. Every list result and watch snapshot, including a delete tombstone, carries its revision. It is not derived solely from a wall-clock timestamp.
- **Inventory watermark** - an opaque API-server revision that proves a watch subscription contains every event after the watermark, or that a complete inventory reflects every committed change through the watermark. It lets a client close the list-watch gap without re-subscribing for each periodic cycle.
- **Synchronization queue** - the durable, per-resource work queue shared by watch delivery and inventory delivery for a resource kind. It serializes a resource ID, keeps the highest-revision desired snapshot, and applies capped exponential backoff.
- **Complete inventory** - every page for one resource kind was retrieved successfully using a stable ordering or API inventory revision. A partial list, failed page, or unavailable list operation is incomplete.
- **Orphan cleanup pipeline** - a distinct desired-versus-actual comparison for resources absent from the API inventory. It is not an enqueue of a nonexistent API resource.

## Requirements

### Requirement: Periodic World Synchronization

The control plane SHALL run a world synchronization cycle at a configurable interval. The configuration key SHALL be `HYPERSHELL_WORLD_SYNC_INTERVAL`; its default SHALL be five minutes. The configuration parser SHALL reject zero, negative, or syntactically invalid values, fall back to the default, and emit a diagnostic log containing the key, invalid value, and effective interval.

The control plane SHALL perform an initial synchronization after all enabled reconcilers have created their synchronization queues, authenticated API connections are ready, and their watch subscriptions have completed the subscription handshake. It SHALL not wait for the first interval to elapse.

#### Scenario: Invalid synchronization interval

- GIVEN `HYPERSHELL_WORLD_SYNC_INTERVAL` is `0s`, `-1m`, or malformed
- WHEN the control plane loads its configuration
- THEN it SHALL use the five-minute default interval
- AND emit a diagnostic log identifying the invalid configuration

#### Scenario: Initial synchronization discovers an existing resource

- GIVEN a ManagedDatabase already exists in the API server before the control plane starts
- AND the ManagedDatabase create event was emitted before the control plane subscribed
- WHEN the control plane completes its initial synchronization
- THEN it SHALL observe the ManagedDatabase from the API inventory
- AND enqueue it for reconciliation
- AND provision or repair its CNPG resources as required by the ManagedDatabase reconciler

#### Scenario: Periodic synchronization recovers a missed event

- GIVEN a Gateway or ManagedDatabase update event was not delivered to the control plane
- WHEN the next world synchronization cycle lists the API server inventory
- THEN the cycle SHALL enqueue the current resource snapshot
- AND the reconciler SHALL converge the corresponding Kubernetes resources to that snapshot

### Requirement: Configurable Synchronization Cadence

The synchronization interval SHALL be configurable without a code change or image rebuild. The effective interval SHALL be logged when the world synchronization process starts.

A synchronization cycle SHALL NOT start another full cycle while a previous cycle is still running. A timer trigger during an active cycle SHALL coalesce into at most one follow-up cycle after the active cycle completes. The root context cancellation SHALL prevent both a pending follow-up cycle and a new interval-triggered cycle from starting.

#### Scenario: Slow synchronization cycle

- GIVEN a world synchronization cycle takes longer than the configured interval
- WHEN the next interval elapses
- THEN the control plane SHALL not run two full synchronization cycles concurrently
- AND it SHALL perform at most one follow-up cycle after the active cycle completes

### Requirement: Complete API Inventory Contracts

Each world synchronization cycle SHALL obtain the complete inventory from the API server for every enabled API-backed reconciler:

- Fleet
- ManagedCluster
- ManagedDatabase
- GatewayRelease
- Gateway
- GatewayNetwork
- RoleBinding, when role-binding reconciliation is enabled

The cycle SHALL use control-plane-authorized gRPC list operations rather than reading the API server database directly. Every inventory operation SHALL provide deterministic pagination and either a stable ordering or an API-issued inventory revision so that the control plane can detect an unstable or incomplete pass. The cycle SHALL retry an unstable pass rather than treating it as complete. Adding inventory polling for Fleet, ManagedCluster, GatewayRelease, and GatewayNetwork is net-new control-plane work; the existing Gateway list seed does not satisfy their inventory requirements.

Before any resource kind participates, the API server SHALL expose a `resource_revision` field on every list snapshot and watch event for that kind. It SHALL advance atomically on create, update, and delete; a delete event SHALL carry the deleted resource's final revision. The API server SHALL also expose an inventory watermark or stable snapshot contract that lets the control plane determine whether a paginated pass is complete. The control plane SHALL migrate `reconcileQueue[T]` version accessors from `updated_at` to `resource_revision`; timestamps alone SHALL NOT be used to order watch and inventory payloads.

The existing user-scoped `RoleBindingService.ListRoleBindings` operation SHALL NOT be used for world synchronization. Before RoleBinding reconciliation can participate, the API server SHALL provide a distinct control-plane-only `ListRoleBindingsForControlPlane` gRPC operation with the following contract:

- it is authorized only for the configured control-plane service account;
- its request supports `page` and `size` and its response includes `ListMeta` or an opaque continuation token plus a total/complete indicator;
- it returns all active RoleBindings without requiring `user_id` or `gateway_id` filters;
- every returned binding includes immutable ID, resource revision, `user_id`, `gateway_id`, and resolved role name needed to calculate the desired Keycloak roles;
- its results are fleet/RBAC safe for the service account and it never returns secret values.

If an enabled reconciler has no gRPC operation that satisfies this contract, the control plane SHALL mark that kind unsupported for world synchronization at startup, emit a diagnostic error, and SHALL NOT claim a successful full synchronization. It MAY continue synchronizing independent supported kinds.

The cycle SHALL continue listing independent resource kinds when one kind fails, but SHALL report the cycle as incomplete and SHALL NOT use an incomplete kind inventory for orphan cleanup.

#### Scenario: Paginated inventory

- GIVEN the API server contains more resources than one list response can return
- WHEN a world synchronization cycle lists a resource kind
- THEN it SHALL request all pages from its control-plane list operation
- AND it SHALL enqueue every resource returned by the complete inventory
- AND it SHALL not silently omit resources beyond the first page

#### Scenario: RoleBinding inventory prerequisite

- GIVEN role-binding reconciliation is enabled
- AND `ListRoleBindingsForControlPlane` is unavailable or returns an incomplete page sequence
- WHEN world synchronization runs
- THEN it SHALL report the RoleBinding inventory as unsupported or incomplete
- AND it SHALL not report a successful RoleBinding synchronization
- AND it SHALL continue synchronizing independent resource kinds

#### Scenario: Partial API outage

- GIVEN listing ManagedDatabases fails while listing Gateways succeeds
- WHEN the world synchronization cycle runs
- THEN it SHALL report the ManagedDatabase inventory failure
- AND it SHALL still process the successfully listed Gateway inventory
- AND it SHALL retry the incomplete ManagedDatabase inventory on a later cycle
- AND it SHALL not perform ManagedDatabase orphan cleanup based on the incomplete inventory

### Requirement: Watch, Reconnect Seed, and Synchronization Coexistence

World synchronization SHALL complement, not replace, gRPC watch streams. Watch streams SHALL continue delivering low-latency changes between synchronization cycles.

At startup and after each watch reconnect, the control plane SHALL establish and drain a watch subscription before using an inventory snapshot to seed its synchronization queue, or obtain an equivalent API-issued inventory watermark. Subsequent periodic cycles SHALL use the already-confirmed live subscription; they SHALL NOT re-subscribe solely to collect inventory. A live event and an inventory snapshot for the same immutable resource ID SHALL be coalesced by resource revision; a lower-revision snapshot SHALL never replace a higher-revision watch event. A delete tombstone SHALL be terminal for that immutable ID and SHALL block stale non-delete snapshots from recreating Kubernetes state.

The existing Gateway watch reconnect seed SHALL remain a connection-recovery mechanism. Periodic world synchronization SHALL use the same Gateway queue but SHALL NOT force-clear a Gateway phase or otherwise re-run provisioning merely because the periodic inventory observed a healthy or health-reconciler-owned active phase. The Gateway reconciler SHALL separately inspect desired and actual managed resources during synchronization so it can repair drift without overwriting status/phase owned by the Gateway health reconciler.

#### Scenario: Update races with synchronization

- GIVEN a Gateway update occurs while the world synchronization cycle is collecting the Gateway inventory
- WHEN both the inventory snapshot and the live watch event reach the synchronization queue
- THEN the queue SHALL retain the highest-revision desired resource state
- AND the Gateway SHALL not be reconciled concurrently for the same resource
- AND the update SHALL not be lost because it arrived during synchronization

#### Scenario: Gateway reconnect and periodic synchronization

- GIVEN the Gateway watch reconnect seed has recovered a Gateway in `Provisioning`
- WHEN a later periodic synchronization observes the same Gateway in `Running`
- THEN the periodic snapshot SHALL not force-clear its phase or start a duplicate provisioning operation
- AND the reconciler MAY repair a missing explicitly managed Kubernetes resource
- AND the Gateway health reconciler SHALL remain the owner of health status and phase transitions

### Requirement: Shared Durable Reconciliation Queues

Every resource kind enabled for world synchronization SHALL use a synchronization queue for both watch and inventory work. The queue SHALL replace any create-or-skip or active-map pattern that discards an event while the same resource is being handled.

A synchronization queue SHALL:

- serialize reconciliation for each immutable resource ID while allowing bounded concurrency across IDs;
- retain the latest accepted snapshot by resource revision;
- preserve a delete tombstone until it is safe to discard stale work for that immutable ID;
- retry a failed reconciliation with capped exponential backoff until success, a confirmed deletion, or root-context cancellation;
- ensure a newer event updates the next desired state without bypassing active retry backoff;
- clear retry state only after successful reconciliation or an intentional healthy-state no-op;
- use the control plane root context and release workers on shutdown.

The control plane SHALL reuse or extend the existing generic `reconcileQueue[T]` mechanism for every applicable kind rather than introducing resource-specific backoff implementations. Its version accessor SHALL use the API `resource_revision` contract, not `updated_at`.

#### Scenario: Concurrent events are coalesced

- GIVEN a ManagedDatabase or RoleBinding reconciliation is in progress for immutable resource ID `r-1`
- WHEN a higher-revision watch event or periodic inventory snapshot for `r-1` arrives
- THEN the synchronization queue SHALL retain the newer snapshot
- AND it SHALL schedule one subsequent reconciliation after the active attempt
- AND it SHALL not silently discard the newer event

#### Scenario: Dependency failure recovers

- GIVEN a ManagedDatabase reconciliation fails because the CNPG operator or Kubernetes API is temporarily unavailable
- WHEN the dependency becomes available before a later world synchronization cycle
- THEN a queued retry or the next synchronization cycle SHALL retry the current ManagedDatabase state
- AND the ManagedDatabase SHALL converge without requiring a user to rename or otherwise modify it

#### Scenario: Status updates do not hot-loop

- GIVEN a reconciler writes `Provisioning` or `Failed` status to the API server
- AND that write generates a ManagedDatabase update event
- WHEN the update event reaches the synchronization queue
- THEN the queue SHALL coalesce it with existing work for that ManagedDatabase
- AND it SHALL respect any active retry backoff
- AND it SHALL not repeatedly invoke the reconciler without a bounded delay

### Requirement: Idempotent Resource Reconciliation

Each synchronization queue invocation SHALL use the same idempotent reconciliation logic used for watch events. A reconciler SHALL inspect actual Kubernetes state and SHALL create, update, or delete only resources required to reach desired state.

A resource already in the desired healthy state SHALL result in a successful no-op. World synchronization SHALL not cause healthy Gateways, ManagedDatabases, or their dependent resources to be unnecessarily recreated or rolled out.

#### Scenario: Healthy resource is synchronized

- GIVEN a ManagedDatabase has status `Ready`
- AND its CNPG Cluster is healthy
- WHEN a world synchronization cycle enqueues the ManagedDatabase
- THEN the ManagedDatabase reconciler SHALL complete successfully
- AND it SHALL not recreate or update the healthy CNPG Cluster
- AND it SHALL not change the ManagedDatabase status

#### Scenario: Kubernetes drift is repaired

- GIVEN a Gateway exists in the API server
- AND one of its explicitly managed Kubernetes resources was removed outside the control plane
- WHEN a world synchronization cycle enqueues the Gateway
- THEN the Gateway reconciler SHALL recreate the missing managed resource
- AND it SHALL preserve resources that are not owned by HyperShell

### Requirement: Orphan Cleanup Pipelines

Orphan cleanup SHALL use a separate desired-versus-actual cleanup pipeline, not an API-resource synchronization queue. A cleanup pipeline SHALL run only after obtaining a complete inventory for its API kind, list only its explicitly managed Kubernetes resource type, validate exact ownership, persist a grace period, re-confirm the complete API inventory immediately before deletion, and enqueue failed cleanup operations with capped backoff.

The following ownership boundaries apply:

| API kind | World-sync responsibility | Actual resources eligible for cleanup | Required ownership evidence | Cleanup authority |
|---|---|---|---|---|
| Gateway | Reconcile desired Gateway state | Gateway workload namespaces only | The existing Gateway namespace-GC labels and namespace contract defined in `openshell-gateway-namespace-gc.spec.md` | `NamespaceGCReconciler`; this specification does not replace it |
| ManagedDatabase | Reconcile desired database state and detect missed deletions | The database namespace and CNPG `Cluster` only | `app.kubernetes.io/managed-by=hypershell-control-plane`, `hypershell.redhat.io/managed=true`, `hypershell.redhat.io/managed-database-id=<immutable-id>`, and the API-assigned `openshell-db-<id-derived-suffix>` namespace contract | A dedicated ManagedDatabase orphan-cleanup pipeline |
| RoleBinding | Reconcile the RoleBinding-to-Keycloak projection from complete inventory | Only `openshell-admin` and `openshell-user` assignments on a HyperShell-managed gateway Keycloak client | The complete RoleBinding inventory; the immutable Gateway ID and managed client ID; and a complete paginated Keycloak Admin API member list for the two bridge-owned roles | RoleBinding projection reconciler; no Kubernetes namespace cleanup |
| Fleet, ManagedCluster, GatewayRelease, GatewayNetwork | Enqueue desired state | None until a reconciler explicitly owns external cleanup | N/A | No orphan cleanup is permitted |

The ManagedDatabase reconciler SHALL stamp `hypershell.redhat.io/managed-database-id` on every database namespace and CNPG Cluster it creates or updates, as defined in `openshell-gateway-database.spec.md`. The orphan-cleanup pipeline SHALL skip and diagnose a resource that lacks any required ownership evidence; it SHALL never infer ownership from a prefix alone.

For the two gateway client roles owned by the RoleBinding bridge, the control plane SHALL be the sole writer on HyperShell-managed gateway Keycloak clients; direct manual grants of `openshell-admin` or `openshell-user` are prohibited. A RoleBinding projection cycle SHALL derive each user's effective role set as the union of all active bindings for a `(user, gateway)` pair: `gateway:owner` contributes both `openshell-admin` and `openshell-user`, while `gateway:viewer` contributes only `openshell-user`. It SHALL enumerate actual members of only those two roles through paginated Keycloak Admin API calls, and add or remove only the difference. If either the RoleBinding inventory or a Keycloak member list is incomplete, fails, or cannot be authorized, the cycle SHALL make no Keycloak role removals and requeue the projection. It SHALL never enumerate or mutate unrelated Keycloak clients or roles.

Gateway namespace cleanup SHALL retain the existing persisted grace period and pre-delete liveness recheck defined in `openshell-gateway-namespace-gc.spec.md`. ManagedDatabase cleanup SHALL use the same safety pattern. `HYPERSHELL_MANAGED_DATABASE_ORPHAN_GC_ENABLED` SHALL independently enable or disable the destructive ManagedDatabase orphan-cleanup pipeline and default to `false`; an invalid boolean value SHALL fall back to `false` with a diagnostic log. When disabled, the control plane SHALL continue non-destructive ManagedDatabase reconciliation but SHALL neither start new orphan cleanup work nor delete cleanup candidates. `HYPERSHELL_MANAGED_DATABASE_ORPHAN_GC_GRACE_PERIOD` SHALL configure the grace period and default to ten minutes; zero, negative, or malformed values SHALL fall back to that default with a diagnostic log. The pipeline SHALL persist an eligibility timestamp on the namespace, wait for that grace period, and re-list ManagedDatabases immediately before deletion. Failure to obtain that final complete inventory SHALL defer cleanup.

#### Scenario: ManagedDatabase orphan cleanup is disabled independently

- GIVEN `HYPERSHELL_MANAGED_DATABASE_ORPHAN_GC_ENABLED=false`
- AND a ManagedDatabase namespace is a fully owned cleanup candidate
- WHEN world synchronization runs
- THEN it SHALL continue non-destructive ManagedDatabase reconciliation
- AND it SHALL not delete or enqueue the cleanup candidate

#### Scenario: ManagedDatabase deletion event was missed

- GIVEN a ManagedDatabase was deleted from the API server
- AND its delete watch event was missed while the control plane was disconnected
- AND its namespace and CNPG Cluster carry all required ManagedDatabase ownership labels
- AND a complete ManagedDatabase inventory confirms the immutable ID is absent
- WHEN the dedicated orphan-cleanup pipeline completes its grace period and re-confirms the absence
- THEN it SHALL clean up the orphaned namespace and CNPG resources
- AND it SHALL leave unrelated resources untouched

#### Scenario: Unmanaged namespace has a similar name

- GIVEN a namespace name matches the ManagedDatabase namespace prefix
- BUT the namespace does not carry every required ManagedDatabase ownership label
- WHEN world synchronization evaluates orphan cleanup
- THEN it SHALL not delete the namespace
- AND it SHALL emit sufficient diagnostic information to explain why cleanup was skipped

#### Scenario: Incomplete inventory defers cleanup

- GIVEN a ManagedDatabase cleanup candidate exists
- AND any page of the ManagedDatabase inventory fails
- WHEN the orphan-cleanup pipeline runs
- THEN it SHALL not delete the candidate
- AND it SHALL retain or requeue the candidate for a later complete inventory

#### Scenario: Missed RoleBinding deletion removes only bridge-owned access

- GIVEN a gateway-scoped RoleBinding deletion event was missed
- AND a complete RoleBinding inventory confirms no remaining binding grants `openshell-user` to the user on that gateway
- AND the Keycloak member list for the HyperShell-managed gateway client is complete
- WHEN the RoleBinding projection cycle runs
- THEN it SHALL remove only that `openshell-user` bridge-owned assignment
- AND it SHALL not mutate assignments on another client or any role other than `openshell-admin` and `openshell-user`

### Requirement: Resource Deletion and Recreation Safety

A completed deletion SHALL prevent stale synchronization snapshots or delayed watch events from recreating the deleted resource's Kubernetes state. A newly created resource with a different immutable API identifier SHALL be treated as independent desired state, even if it has the same display name.

#### Scenario: Stale snapshot follows deletion

- GIVEN a world synchronization snapshot contains a resource that is deleted before its queued work executes
- WHEN the queue receives a higher-revision deletion tombstone or confirms the immutable ID is absent from a complete inventory
- THEN it SHALL prioritize cleanup or discard the stale non-delete snapshot
- AND it SHALL not recreate the deleted resource's Kubernetes resources

### Requirement: Synchronization Observability

The control plane SHALL emit structured diagnostic logs for each synchronization cycle containing:

- cycle ID, start, and completion
- effective interval and duration
- inventory revision or stable-pass result
- resource counts discovered and enqueued by kind
- unsupported or incomplete inventories and list failures
- reconciliation failures and retry scheduling
- orphan cleanup candidates, actions, deferrals, and ownership-validation skips

The control plane SHOULD expose metrics for cycle duration, cycle failures, resources enqueued, reconciliation retries, inventory completeness, and orphan cleanup actions.

#### Scenario: Operators diagnose a missed event

- GIVEN a ManagedDatabase was not reconciled when its watch event was emitted
- WHEN a later world synchronization cycle discovers it
- THEN the logs SHALL identify the synchronization cycle
- AND identify the ManagedDatabase ID and reconciliation result
- AND make clear whether the resource was newly discovered, repaired, skipped as healthy, or failed and requeued

### Requirement: Synchronization Shutdown

World synchronization SHALL use the control plane root context. On shutdown, it SHALL stop starting new cycles, cancel in-flight list and reconciliation operations according to existing context rules, and release synchronization-queue and orphan-cleanup workers without leaking goroutines.

#### Scenario: Shutdown during inventory collection

- GIVEN the control plane is collecting a paginated inventory
- WHEN the control plane receives a shutdown signal
- THEN the inventory requests SHALL be canceled through context propagation
- AND no new synchronization or cleanup cycle SHALL start
- AND the control plane SHALL terminate without leaving a synchronization or cleanup worker running

## Design Decisions

| Decision | Rationale |
|----------|-----------|
| Periodic inventory in addition to watch streams | Watches provide low latency, while periodic inventory recovers missed events and restart gaps. |
| Control-plane gRPC list operations as the inventory source | PostgreSQL-backed API resources are the desired-state authority; the control plane must not bypass API contracts with direct database access. |
| Separate RoleBinding inventory RPC | The existing list RPC is deliberately user-scoped and non-paginated; a privileged, paginated API avoids leaking data or abusing a per-user authorization operation. |
| RoleBinding bridge owns its two gateway client roles | A complete desired RoleBinding inventory plus complete Keycloak member lists can safely converge assignments only when direct manual grants to the bridge-owned roles are prohibited. |
| Revision-aware per-resource queues | An explicit API revision, rather than a timestamp, lets generic queues coalesce correctly and prevents stale-snapshot resurrection. |
| Gateway reconnect seed remains separate from periodic sync | Reconnect recovery must not be lost, while periodic sync must not force active phases or fight the health reconciler. |
| Orphan cleanup is a separate pipeline | An absent API resource cannot be placed on an ID-keyed desired-state queue; safe cleanup requires a complete desired-versus-actual comparison. |
| Complete inventory plus ownership evidence, independent kill switch, and grace period required before cleanup | A partial list, name similarity, or transient API state cannot prove that a resource is orphaned; destructive database cleanup is disabled by default until an operator enables it. |
| NamespaceGCReconciler remains the Gateway namespace authority | It already owns the Gateway label contract, persisted grace period, and liveness recheck; duplicating it would create competing destructive actors. |
