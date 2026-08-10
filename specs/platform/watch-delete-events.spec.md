# Watch Delete Events Specification

## Purpose

gRPC watch streams deliver resource events (create, update, delete) from the API server to the control plane. Today, delete events carry only the `resource_id` — the resource payload is nil. This forces the control plane to maintain in-memory caches (e.g., the gateway-to-namespace map) to perform cleanup. When the control plane restarts, these caches are lost and cleanup is silently skipped.

This spec defines the behavior for including the full resource snapshot in delete watch events so that reconcilers always have the information they need to clean up, regardless of process restarts.

## Requirements

### Requirement: Delete Events SHALL Include the Resource Snapshot

The gRPC watch handler for each Kind SHALL include the full resource in delete events, matching the behavior of create and update events. The resource snapshot SHALL reflect the state of the resource immediately before deletion.

#### Scenario: Gateway delete event carries the resource

- GIVEN a Gateway exists with `id=gw-1`, `namespace=openshell-tenant-a`, `cluster_id=cluster-1`
- WHEN the Gateway is deleted via `DELETE /api/hypershell/v1/gateways/gw-1`
- THEN the `WatchGatewaysResponse` event SHALL have `type=DELETED`, `resource_id=gw-1`
- AND the `gateway` field SHALL be populated with the full Gateway resource (name, namespace, cluster_id, etc.)

#### Scenario: Control plane cleans up after restart

- GIVEN the control plane has restarted and has no in-memory state
- WHEN it receives a Gateway delete event with the resource populated
- THEN it SHALL read the namespace from `event.Resource.Namespace`
- AND it SHALL clean up all associated K8s resources in that namespace

### Requirement: Soft-Deleted Records SHALL Be Readable for Event Enrichment

The API server's gRPC watch handler processes events asynchronously from the event broker. Because the `Delete()` service method soft-deletes the database row before creating the event, the watch handler SHALL use an unscoped query (bypassing the `deleted_at IS NULL` filter) to load the resource for delete events. A new `GetUnscoped` method on the DAO SHALL provide this capability.

#### Scenario: Watch handler loads soft-deleted gateway

- GIVEN a Gateway has been soft-deleted (row exists with `deleted_at` set)
- WHEN the watch handler receives the delete event for that Gateway
- THEN it SHALL load the Gateway using an unscoped query
- AND include it in the `WatchGatewaysResponse`

#### Scenario: Soft-deleted record no longer exists

- GIVEN a Gateway's row has been hard-deleted or is otherwise missing
- WHEN the watch handler receives a delete event for that Gateway
- THEN it SHALL log a warning
- AND send the delete event without the resource payload (graceful degradation)

### Requirement: Control Plane SHALL Prefer Event Resource Over In-Memory Cache

The GatewayReconciler SHALL read the namespace (and other fields) from `event.Resource` when available. The in-memory `namespaces` map SHALL be removed because it is unreliable across restarts and adds complexity.

#### Scenario: Delete event with resource populated

- GIVEN a Gateway delete event where `event.Resource` is non-nil
- WHEN the GatewayReconciler processes it
- THEN it SHALL use `event.Resource.Namespace` to locate the K8s resources to delete
- AND it SHALL NOT depend on any in-memory cache

#### Scenario: Delete event with nil resource (graceful degradation)

- GIVEN a Gateway delete event where `event.Resource` is nil (e.g., hard-deleted before event processed)
- WHEN the GatewayReconciler processes it
- THEN it SHALL log a warning that cleanup cannot be performed
- AND it SHALL NOT silently succeed

## Design Decisions

| Decision | Rationale |
|----------|-----------|
| Unscoped DAO query rather than stashing before delete | Simpler: no changes to the `Delete()` service method or HTTP handler; the watch handler is the only consumer that needs the deleted state. |
| Apply to Gateways only (initially) | Gateways are the only Kind where delete cleanup requires resource fields. Other Kinds can adopt the pattern when needed. |
| Graceful degradation on missing record | Hard deletes or data purges should not crash the watch stream; a warning is sufficient since manual cleanup is already the fallback today. |
| Remove in-memory namespace cache | The cache is the root cause of the bug; keeping it alongside the fix adds dead code and a false sense of redundancy. |
