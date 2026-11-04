# Watch Delete Events Specification

## Purpose

gRPC watch streams deliver resource events (create, update, delete) from the API server to the control plane. Today, delete events carry only the `resource_id` - the resource payload is nil. This forces the control plane to maintain in-memory caches (e.g., the gateway-to-namespace map) to perform cleanup. When the control plane restarts, these caches are lost and cleanup is silently skipped.

This spec defines the behavior for including the full resource snapshot in delete watch events so that reconcilers always have the information they need to clean up, regardless of process restarts.

This spec refines the durable-deletion guarantee of the control-plane reconciliation contract ([`../standards/control-plane/reconciliation-contract.spec.md`](../standards/control-plane/reconciliation-contract.spec.md), STD-RCON-07); it refines that contract and does not weaken it.

## Requirements

### Requirement: CTRL-WDEL-01 -- Delete Events SHALL Include the Resource Snapshot

The gRPC watch handler for each Kind SHALL include the full resource in delete events, matching the behavior of create and update events. The resource snapshot SHALL reflect the state of the resource immediately before deletion.

#### Scenario: CTRL-WDEL-01.01 -- Gateway delete event carries the resource

- GIVEN a Gateway exists with `id=gw-1`, `namespace=openshell-tenant-a`, `cluster_id=cluster-1`
- WHEN the Gateway is deleted via `DELETE /api/hypershell/v1/gateways/gw-1`
- THEN the `WatchGatewaysResponse` event SHALL have `type=DELETED`, `resource_id=gw-1`
- AND the `gateway` field SHALL be populated with the full Gateway resource (name, namespace, cluster_id, etc.)

#### Scenario: CTRL-WDEL-01.02 -- Control plane cleans up after restart

- GIVEN the control plane has restarted and has no in-memory state
- WHEN it receives a Gateway delete event with the resource populated
- THEN it SHALL read the namespace from `event.Resource.Namespace`
- AND it SHALL clean up all associated K8s resources in that namespace

### Requirement: CTRL-WDEL-02 -- Soft-Deleted Records SHALL Be Readable for Event Enrichment

The API server's gRPC watch handler processes events asynchronously from the event broker. Because the `Delete()` service method soft-deletes the database row before creating the event, the watch handler SHALL use an unscoped query (bypassing the `deleted_at IS NULL` filter) to load the resource for delete events. A new `GetUnscoped` method on the DAO SHALL provide this capability.

#### Scenario: CTRL-WDEL-02.01 -- Watch handler loads soft-deleted gateway

- GIVEN a Gateway has been soft-deleted (row exists with `deleted_at` set)
- WHEN the watch handler receives the delete event for that Gateway
- THEN it SHALL load the Gateway using an unscoped query
- AND include it in the `WatchGatewaysResponse`

#### Scenario: CTRL-WDEL-02.02 -- Soft-deleted record no longer exists

- GIVEN a Gateway's row has been hard-deleted or is otherwise missing
- WHEN the watch handler receives a delete event for that Gateway
- THEN it SHALL log a warning
- AND send the delete event without the resource payload (graceful degradation)

### Requirement: CTRL-WDEL-03 -- Control Plane SHALL Prefer Event Resource Over In-Memory Cache

The GatewayReconciler SHALL read the namespace (and other fields) from `event.Resource` when available.

#### Scenario: CTRL-WDEL-03.01 -- Delete event with resource populated

- GIVEN a Gateway delete event where `event.Resource` is non-nil
- WHEN the GatewayReconciler processes it
- THEN it SHALL use `event.Resource.Namespace` to locate the K8s resources to delete
- AND it SHALL NOT depend on any in-memory cache

#### Scenario: CTRL-WDEL-03.02 -- Delete event with nil resource is re-read, not silently succeeded

- GIVEN a Gateway delete event where `event.Resource` is nil (for example the API server delivered a nil payload to an unscoped subscriber)
- WHEN the GatewayReconciler processes it
- THEN it SHALL re-read the deleting Gateway record with the unscoped query and reconcile teardown from it per CTRL-WDEL-04
- AND it SHALL NOT return success while owned resources remain unreconciled

### Requirement: CTRL-WDEL-04 -- Nil-Payload Delete Re-reads the Record and Never Silently Succeeds

A Gateway delete event can arrive with no resource payload (`event.Resource` is
nil). This is reachable in normal operation: the API server delivers
nil-payload delete events to unscoped subscribers. The control plane SHALL NOT
mark such a delete done by logging a warning and returning success, because the
gateway's out-of-namespace resources (its database and Keycloak clients) would
then be left behind with no recovery path and no operator-visible signal.

On a nil-payload delete the control plane SHALL first re-read the deleting
Gateway record with the unscoped query (CTRL-WDEL-02), which bypasses the
soft-delete filter. When the record loads, the control plane SHALL reconcile
every owned resource class toward absence from that record, exactly as it would
from an inline payload (see
[`gateway-deletion-finalization.spec.md`](./gateway-deletion-finalization.spec.md),
GTW-DEL-01). When the record cannot be loaded, the control plane SHALL treat the
teardown as an unresolved precondition: it SHALL return a retryable error so the
delete event is retried (GTW-DEL-04), and it SHALL record the no-silent-orphan
signal for the owned resources it could not reconcile (GTW-DEL-07). A nil-payload
delete SHALL NOT be reported complete while owned resources may remain and were
never reconciled.

#### Scenario: CTRL-WDEL-04.01 -- Nil payload reconciles from the unscoped re-read

- GIVEN a Gateway delete event whose `event.Resource` is nil
- AND the deleting Gateway record is still loadable with the unscoped query
- WHEN the GatewayReconciler processes the delete
- THEN it SHALL load the record with the unscoped query
- AND it SHALL reconcile the gateway's owned resource classes toward absence from that record

#### Scenario: CTRL-WDEL-04.02 -- Unloadable record returns a retryable error and records the orphan signal

- GIVEN a Gateway delete event whose `event.Resource` is nil
- AND the deleting Gateway record cannot be loaded (for example the unscoped read fails)
- WHEN the GatewayReconciler processes the delete
- THEN it SHALL NOT return success
- AND it SHALL return a retryable error so the delete event is retried
- AND it SHALL record the no-silent-orphan signal for the owned resources it could not reconcile

## Design Decisions

| Decision | Rationale |
|----------|-----------|
| Unscoped DAO query rather than stashing before delete | Simpler: no changes to the `Delete()` service method or HTTP handler; the watch handler is the only consumer that needs the deleted state. |
| Apply to Gateways only (initially) | Gateways are the only Kind where delete cleanup requires resource fields. Other Kinds can adopt the pattern when needed. |
| Nil-payload delete re-reads rather than degrading to a warning | A nil payload is reachable in normal operation (unscoped subscribers receive nil-payload deletes), so treating it as a best-effort warning silently orphans the gateway's database and Keycloak clients, which have no namespace-GC recovery path. The control plane re-reads the deleting record with the unscoped query and reconciles from it; when the record cannot be loaded it returns a retryable error and records the no-silent-orphan signal, consistent with GTW-DEL-04 and GTW-DEL-07. |
