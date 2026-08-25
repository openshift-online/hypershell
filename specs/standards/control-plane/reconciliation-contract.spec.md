# Control Plane Reconciliation Contract

## Purpose

This specification defines the guarantees that all HyperShell control-plane reconcilers provide. It applies to API resources reconciled into Kubernetes or external systems. Resource-specific specifications define the desired resources; this contract defines how reconciliation remains correct during duplicate or missed events, concurrent updates, partial failures, deletion, and process restart.

## Requirements

### Requirement: Desired State Is Authoritative

A reconciler SHALL derive work from the current API resource and observed actual state. Watch events SHALL schedule reconciliation but SHALL NOT be the source of truth. The control plane SHALL recover work through an initial resource list and a periodic resync, unless the watch API provides equivalent durable replay.

Events MAY be duplicated, delayed, coalesced, or delivered out of order without changing the final result. A missed event SHALL NOT permanently prevent convergence.

#### Scenario: Event is missed

- GIVEN a resource exists but its watch event is missed
- WHEN the control plane starts or performs its periodic resync
- THEN it SHALL discover and reconcile the resource

### Requirement: Reconciliation Is Level-Based and Idempotent

Each pass SHALL fetch current desired state, observe relevant actual state, and move owned state toward the desired state. Repeating a pass with unchanged inputs SHALL be safe. A process MAY stop after any external operation; the next pass SHALL continue without creating duplicate resources or losing required cleanup.

A healthy or previously converged status SHALL NOT by itself suppress drift reconciliation. If owned actual state changes after initial convergence, a later pass SHALL repair it.

#### Scenario: Process stops after an external write

- GIVEN a reconcile pass creates an owned external resource
- AND the process stops before it records status
- WHEN reconciliation runs again
- THEN it SHALL adopt or update that resource without creating a duplicate

#### Scenario: Actual state drifts

- GIVEN a resource reports healthy at its current generation
- AND an owned actual resource is later removed or changed
- WHEN reconciliation runs again
- THEN it SHALL restore the desired state

### Requirement: Work Is Serialized and Retried

The control plane SHALL run at most one reconcile pass for one resource UID at a time. It MAY reconcile different resources concurrently with a bounded worker count. A slow or failed resource SHALL NOT block watch consumption or unrelated resources.

A failed required operation SHALL return an error and SHALL be retried with bounded exponential backoff. A newer event SHALL cause another pass against current state. A delete request SHALL take precedence over older queued updates for the same UID.

If more than one control-plane replica can mutate state, the deployment SHALL provide one active leader or equivalent cross-process exclusion.

#### Scenario: Update arrives during reconciliation

- GIVEN a reconcile pass is running for a resource
- WHEN a newer update arrives for the same UID
- THEN the two passes SHALL NOT overlap
- AND a later pass SHALL reconcile the current resource state

### Requirement: Resource Versions Prevent Stale Commits

Every reconciled API resource SHALL have an immutable UID, a resource version that changes on every write, and a generation that changes only when desired state changes. Controller status SHALL identify the generation it observed.

Status and finalizer writes SHALL be conditional on the resource identity and relevant version or generation. A stale pass SHALL NOT publish success or complete deletion for a newer generation. Controllers SHALL update only fields they own and SHALL NOT use whole-resource replacement for independent status fields.

#### Scenario: Desired state changes during a pass

- GIVEN a pass observes generation 4
- AND desired state changes to generation 5 before the pass records success
- WHEN the pass attempts to write status for generation 4
- THEN the API SHALL reject or supersede that stale write
- AND a later pass SHALL reconcile generation 5

### Requirement: Deletion Uses Durable Finalization

A resource that owns Kubernetes or external state SHALL remain readable and listable after deletion is requested. The API SHALL record deletion intent and SHALL retain the resource while a control-plane finalizer remains.

The reconciler SHALL converge owned state to absence, confirm that required cleanup is complete, and only then remove its finalizer. Partial cleanup or process restart SHALL leave deletion pending and retryable. Queued deletion SHALL be terminal for that UID so an older event cannot recreate the resource.

The resource SHALL retain the immutable identity and cleanup data required for finalization. Desired state SHALL NOT be resurrected after deletion begins.

#### Scenario: Deletion occurs during downtime

- GIVEN deletion is requested while the control plane is unavailable
- WHEN the control plane restarts and lists deleting resources
- THEN it SHALL perform the required cleanup
- AND the API SHALL remove the resource only after finalization succeeds

#### Scenario: Cleanup partially fails

- GIVEN deletion requires several cleanup operations
- WHEN one required operation fails
- THEN the finalizer SHALL remain
- AND reconciliation SHALL retry until all required state is confirmed absent

### Requirement: Destructive Actions Fail Closed

A reconciler SHALL modify or delete only resources whose immutable identity or ownership markers prove that it owns them. A failed read, incomplete list, timeout, or authorization error SHALL mean that state is unknown, not absent.

Before an irreversible delete, the reconciler SHALL confirm current deletion intent and resource UID. If it cannot confirm liveness, ownership, or absence, it SHALL defer the action and return an error.

#### Scenario: Observation fails before deletion

- GIVEN cleanup requires proof that an owned resource is no longer desired
- WHEN the required API read fails
- THEN the reconciler SHALL NOT delete the resource
- AND it SHALL retry the observation

### Requirement: Status Is Current and Truthful

Status SHALL describe observed state, not planned work. A healthy condition SHALL apply only to the generation recorded as observed. A required operation or status write that fails SHALL NOT be reported as successful.

Transient failures SHALL produce a retryable error and MAY report a progressing or degraded condition. A stable invalid specification SHALL report a terminal condition and MAY wait for a new generation instead of retrying rapidly. Each controller SHALL own distinct status fields or condition types.

#### Scenario: Required operation fails

- GIVEN a reconcile pass cannot complete a required external operation
- WHEN it reports the result
- THEN it SHALL NOT report the resource healthy for the current generation
- AND it SHALL preserve enough status for an operator to identify the failed operation

### Requirement: External Calls Are Bounded and Observable

Every external call SHALL honor cancellation and use a bounded timeout. The control plane SHALL expose enough logs and metrics to identify queue depth, reconcile duration, retries, and resources that remain deleting or unconverged.

#### Scenario: Dependency does not respond

- GIVEN an external dependency does not respond
- WHEN a reconcile pass calls it
- THEN the call SHALL end at its timeout
- AND other resource keys SHALL continue to reconcile

## Guarantee Boundaries

The reconciliation contract provides eventual convergence when desired state becomes stable and dependencies eventually respond. It does not provide exactly-once event handling, global event order, immediate convergence, or one atomic transaction across PostgreSQL, Kubernetes, and external systems.

This contract standardizes behavior, not one resource-shaped Go interface. A shared driver MAY provide keying, serialization, retry, and resync while typed domain reconcilers retain their own desired-state and actual-state logic.
