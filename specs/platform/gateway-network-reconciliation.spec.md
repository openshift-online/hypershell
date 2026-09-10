# GatewayNetwork Reconciliation

**Date:** 2026-09-04
**Status:** Active

## Purpose

This spec defines how the HyperShell control plane reconciles `GatewayNetwork`
resources. A `GatewayNetwork` is a database-backed record describing the intended
connectivity topology between gateways: its `topology` (network shape), its
`tunnel_mode` (encapsulation method), and, for hub-and-spoke shapes, the
`hub_gateway_id` that designates the hub. Like a `GatewayRelease`, a
`GatewayNetwork` has **no direct Kubernetes footprint** of its own in this
ticket. Reconciling a network therefore means (1) validating the network's
structural and referential coherence and (2) recording a deterministic,
observable `status` back on the network so operators can tell whether the
declared topology is well-formed and whether its designated hub actually exists.

Today the `GatewayNetworkReconciler` is a no-op: it dedups, opens a trace span,
logs, and returns `nil` without validating the network or writing any status.
This spec replaces that behavior with a deterministic reconciliation contract,
mirroring the contract established for
[`gateway-release-reconciliation.spec.md`](./gateway-release-reconciliation.spec.md).

This spec is a sub-spec of [`control-plane.spec.md`](./control-plane.spec.md) and
refines its "Configure network meshes between gateways" and "Update resource
status back to the API server" responsibilities for the network resource
specifically.

### Scope Boundary

- **In scope:** validating a network's topology vocabulary and topology/hub
  coherence, validating that a designated `hub_gateway_id` references an existing
  Gateway, writing back a deterministic `status`, and doing so idempotently and
  serialized per network.
- **Out of scope (future work, pending product definition):** applying any real
  gateway-to-gateway connectivity in the cluster (mesh/tunnel provisioning,
  NetworkPolicy, ServiceExport, or any inter-cluster networking technology); a
  member-gateway list (the model today designates only a single `hub_gateway_id`,
  not a set of members); the enumerated `tunnel_mode` encapsulation vocabulary and
  its semantics; and routing spoke traffic through the hub. Actual connectivity
  provisioning is deferred until product defines the network membership model and
  selects a connectivity technology. This reconciler only guarantees that a
  network's declared configuration is validated and its outcome recorded.

## Domain Vocabulary

A `GatewayNetwork` `topology` SHALL be one of the following canonical values,
which describe the intended network shape:

| Topology | Meaning |
|---|---|
| `mesh` | Every gateway in the network connects to every other; no designated hub. |
| `hub-spoke` | Spoke gateways connect through a single designated hub gateway. |

A `GatewayNetwork` carries a `status` field that the control plane owns and keeps
current to reflect the reconciled validation outcome. Allowed
control-plane-owned values:

- **`Valid`** - the network's topology is a recognized value, its topology/hub
  coherence rules are satisfied, and any designated hub gateway exists. The
  declared configuration is well-formed.
- **`Invalid`** - the network failed validation; the value includes a short
  human-readable reason (for example, an unrecognized topology, a hub-spoke
  network with no hub, or a `hub_gateway_id` that does not reference an existing
  Gateway).

The `status` string is a short, human-readable descriptor surfaced in the console
and CLI alongside the network. It reflects only configuration validity, not the
existence of any provisioned connectivity (which is out of scope, see above).

## Requirements

### Requirement: Network Configuration Validation

The control plane SHALL validate a `GatewayNetwork` on every create and update
event against the following rules, in order:

1. The `topology`, if set, SHALL be one of the canonical values (`mesh`,
   `hub-spoke`). An unrecognized topology SHALL be treated as invalid. An empty
   topology SHALL be treated as invalid, because a network with no declared shape
   cannot be reconciled.
2. A `hub-spoke` network SHALL designate a `hub_gateway_id`; a `hub-spoke`
   network with no hub SHALL be treated as invalid.
3. When a `hub_gateway_id` is set (for any topology), it SHALL reference an
   existing Gateway. A `hub_gateway_id` that does not resolve to an existing
   Gateway SHALL be treated as invalid.

#### Scenario: Well-formed hub-spoke network passes validation

- GIVEN a `GatewayNetwork` with `topology: hub-spoke` and a `hub_gateway_id` that
  references an existing Gateway
- WHEN the control plane reconciles the network
- THEN validation succeeds

#### Scenario: Unrecognized topology fails validation

- GIVEN a `GatewayNetwork` with `topology: ring`
- WHEN the control plane reconciles the network
- THEN validation fails with a reason describing the unrecognized topology

#### Scenario: Hub-spoke without a hub fails validation

- GIVEN a `GatewayNetwork` with `topology: hub-spoke` and no `hub_gateway_id`
- WHEN the control plane reconciles the network
- THEN validation fails with a reason stating a hub-spoke network requires a hub

#### Scenario: Dangling hub reference fails validation

- GIVEN a `GatewayNetwork` whose `hub_gateway_id` references a Gateway that does
  not exist
- WHEN the control plane reconciles the network
- THEN validation fails with a reason describing the missing hub gateway

### Requirement: Deterministic Network Status Write-Back

The control plane SHALL write the reconciled network's status back to the API
server so the persisted `status` deterministically reflects the reconcile
outcome: `Valid` on successful validation, or `Invalid` with a reason on failed
validation. The write-back SHALL be idempotent: the control plane SHALL NOT issue
a status update when the persisted `status` already equals the desired value.

#### Scenario: Status settles to Valid

- GIVEN a `GatewayNetwork` that passes validation and whose persisted `status` is
  unset or not `Valid`
- WHEN the control plane reconciles the network
- THEN the control plane updates the network `status` to `Valid`

#### Scenario: Status settles to Invalid with a reason

- GIVEN a `GatewayNetwork` that fails validation
- WHEN the control plane reconciles the network
- THEN the control plane updates the network `status` to `Invalid` including the
  validation reason

#### Scenario: No redundant status write

- GIVEN a `GatewayNetwork` whose persisted `status` is already `Valid`
- WHEN the control plane reconciles the network and validation still passes
- THEN the control plane makes no status update call for the network

### Requirement: Network Deletion Has No Cluster Footprint

A `GatewayNetwork` delete event SHALL NOT remove or disrupt any running Gateway
workload, because a network owns no Kubernetes resources in this scope. The
control plane SHALL treat a network delete as a terminal, idempotent no-op with
respect to cluster state, and SHALL NOT error when the network is already absent.

#### Scenario: Deleting a network leaves gateways untouched

- GIVEN a `GatewayNetwork` `n1` whose `hub_gateway_id` designates Gateway `g1`
- WHEN `n1` is deleted
- THEN `g1`'s running workload is unchanged
- AND the control plane reports the network reconcile as successful

### Requirement: Idempotent, Serialized Reconciliation

Network reconciliation SHALL be idempotent and SHALL be serialized per network so
that a retry never runs concurrently with a live event for the same network.
Re-reconciling an unchanged network SHALL converge to the same status and SHALL
NOT produce redundant status writes.

#### Scenario: Repeated reconciles are stable

- GIVEN a `GatewayNetwork` already reconciled to `Valid`
- WHEN the control plane reconciles the same network again with no change
- THEN no status update is requested

### Requirement: Transient Failures Surface as Errors, Not Silent Success

When a network reconcile cannot complete because a dependency is transiently
unavailable (for example, the API server rejects the status write, or the hub
gateway lookup fails with a transient error rather than a definitive not-found),
the reconciler SHALL return an error rather than reporting success, so the
failure is surfaced and not silently swallowed. A definitive not-found for the
hub gateway is a deterministic validation failure (status `Invalid`), not a
transient error, and SHALL NOT be reported as an error. The reconciler SHALL NOT
settle a network's status to `Invalid` on account of a transient failure. The
network watch is inline and log-only (there is no reconcile queue for networks,
matching the sibling release reconciler) and does not replay state on reconnect,
so a surfaced error re-converges only when the network is next mutated, not
automatically. Partial failures SHALL NOT be silently swallowed.

#### Scenario: Status write failure surfaces as an error

- GIVEN a `GatewayNetwork` whose status must be updated to `Valid`
- WHEN the status write to the API server fails transiently
- THEN the reconcile returns an error rather than reporting success

#### Scenario: Transient hub lookup failure surfaces as an error

- GIVEN a `GatewayNetwork` with a `hub_gateway_id`
- WHEN the hub gateway lookup fails with a transient error
- THEN the reconcile returns an error rather than reporting success
- AND the network status is not settled to `Invalid` on account of the transient
  failure
