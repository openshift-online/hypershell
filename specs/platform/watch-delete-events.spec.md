# Watch Delete Events Specification

## Purpose

This specification defines delete-event behavior for gRPC watch streams. Delete
events carry a resource snapshot to reduce cleanup latency, while the durable
deleting resource and its finalizer remain the source of truth under the shared
[`reconciliation contract`](../standards/control-plane/reconciliation-contract.spec.md).

## Requirements

### Requirement: Delete Events Include a Resource Snapshot

The gRPC watch handler for each managed Kind SHALL include the deleting resource
in its delete event. The snapshot SHALL include the resource ID, immutable UID,
deletion intent, and fields required to locate owned state.

#### Scenario: Gateway delete event carries identity

- GIVEN a Gateway owns Kubernetes and external resources
- WHEN deletion is requested
- THEN its delete event SHALL include the Gateway ID, UID, namespace, and ownership data

### Requirement: Deleting Resources Remain Readable

The API SHALL keep a deleting resource readable and listable until all finalizers
are removed. Event enrichment and reconciliation SHALL read that durable record;
they SHALL NOT depend on process-local caches.

#### Scenario: Control plane restarts during deletion

- GIVEN a resource is waiting for control-plane finalization
- WHEN the control plane restarts with no in-memory state
- THEN its initial list SHALL include the deleting resource
- AND reconciliation SHALL resume cleanup from the durable record

### Requirement: Missing Event Payload Does Not Complete Cleanup

If the watch handler cannot attach the resource snapshot, it SHALL send the
resource ID and UID when available and log the enrichment failure. The
reconciler SHALL fetch current durable state and SHALL NOT treat the missing
payload as successful cleanup.

#### Scenario: Delete event has no snapshot

- GIVEN a delete event contains identity but no resource snapshot
- WHEN the control plane handles the event
- THEN it SHALL enqueue the resource identity and fetch current deleting state
- AND a failed fetch SHALL leave finalization pending for retry
