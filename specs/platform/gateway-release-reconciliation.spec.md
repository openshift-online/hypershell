# GatewayRelease Reconciliation

**Date:** 2026-09-02
**Status:** Active

## Purpose

This spec defines how the HyperShell control plane reconciles `GatewayRelease`
resources. A `GatewayRelease` is a database-backed record of a versioned gateway
container image; it has **no direct Kubernetes footprint** of its own. Reconciling
a release therefore means (1) validating the release's image reference,
(2) recording a deterministic, observable `status` back on the release, and
(3) propagating an effective image change to every Gateway that references the
release so the cluster converges toward the new desired version.

Today the `GatewayReleaseReconciler` is a no-op: it dedups, opens a trace span,
logs, and returns `nil` without validating the release or affecting any gateway.
This spec replaces that behavior with a deterministic reconciliation contract.

This spec is a sub-spec of [`control-plane.spec.md`](./control-plane.spec.md) and
refines its "Manage release rollouts" and "Update resource status back to the API
server" responsibilities for the release resource specifically.

### Scope Boundary

- **In scope:** validating a release, writing back its deterministic status, and
  requesting reconciliation of Gateways that reference the release when its
  effective image changes.
- **Out of scope (sibling tickets):** how a Gateway resolves its `release_id` to
  a concrete image at deploy time is defined by database-backed gateway version
  selection; safe/progressive rollout ordering and canary traffic strategies are
  defined by the release-rollout and canary specs. This spec only guarantees that
  a release change causes the referencing Gateways to be re-reconciled; the
  resulting deploy behavior is owned by those specs.

## Domain Vocabulary

A `GatewayRelease` carries a `status` field that the control plane owns and keeps
current to reflect the reconciled validation outcome. Allowed control-plane-owned
values:

- **`Available`** - the release's image reference is well-formed and the release
  is eligible to be used by Gateways.
- **`Invalid`** - the release's image reference failed validation; the value
  includes a short human-readable reason. Gateways SHOULD NOT be driven toward an
  `Invalid` release's image.

The `status` string is a short, human-readable descriptor surfaced in the console
and CLI alongside the release.

## Requirements

### Requirement: Release Image Validation

The control plane SHALL validate a `GatewayRelease`'s `image` reference on every
create and update event, using the same image-reference rules applied to gateway
workloads (well-formed reference format; rejection of shell-injection
metacharacters). A release whose `image` is empty or malformed SHALL be treated as
invalid and SHALL NOT be propagated to any Gateway.

#### Scenario: Well-formed image passes validation

- GIVEN a `GatewayRelease` with `image: registry.redhat.io/openshell/gateway:v1.2.0`
- WHEN the control plane reconciles the release
- THEN validation succeeds
- AND the release becomes eligible for propagation

#### Scenario: Malformed image fails validation

- GIVEN a `GatewayRelease` with `image: "gateway:v1; rm -rf /"`
- WHEN the control plane reconciles the release
- THEN validation fails with a reason describing the invalid reference
- AND no Gateway is reconciled as a result of this release

### Requirement: Deterministic Release Status Write-Back

The control plane SHALL write the reconciled release's status back to the API
server so the persisted `status` deterministically reflects the reconcile outcome:
`Available` on successful validation, or `Invalid` with a reason on failed
validation. The write-back SHALL be idempotent: the control plane SHALL NOT issue
a status update when the persisted `status` already equals the desired value.

#### Scenario: Status settles to Available

- GIVEN a `GatewayRelease` whose image passes validation and whose persisted
  `status` is unset or not `Available`
- WHEN the control plane reconciles the release
- THEN the control plane updates the release `status` to `Available`

#### Scenario: Status settles to Invalid with a reason

- GIVEN a `GatewayRelease` whose image fails validation
- WHEN the control plane reconciles the release
- THEN the control plane updates the release `status` to `Invalid` including the
  validation reason

#### Scenario: No redundant status write

- GIVEN a `GatewayRelease` whose persisted `status` is already `Available`
- WHEN the control plane reconciles the release and validation still passes
- THEN the control plane makes no status update call for the release

### Requirement: Change Propagation to Referencing Gateways

When a reconcile determines that a valid release's effective image has changed, the
control plane SHALL request reconciliation of every Gateway whose `release_id`
references that release, so each referencing Gateway converges toward the new
desired version. Propagation SHALL be limited to Gateways that reference the
release by `release_id`; Gateways that pin an explicit `image` and do not
reference the release SHALL NOT be disturbed.

#### Scenario: Image change fans out to referencing gateways

- GIVEN a valid `GatewayRelease` `r1` referenced by Gateways `g1` and `g2` via
  `release_id`
- AND a Gateway `g3` that does not reference `r1`
- WHEN `r1`'s image is updated to a new valid reference
- THEN the control plane requests reconciliation of `g1` and `g2`
- AND the control plane does not request reconciliation of `g3`

#### Scenario: Invalid release does not fan out

- GIVEN a `GatewayRelease` `r1` referenced by Gateway `g1`
- WHEN `r1` is updated to a malformed image
- THEN the release status settles to `Invalid`
- AND `g1` is not driven toward the invalid image

#### Scenario: No image change does not fan out

- GIVEN a valid `GatewayRelease` `r1` referenced by Gateway `g1`
- WHEN `r1` is updated in a way that does not change its effective image
  (for example, a rename)
- THEN the control plane does not request reconciliation of `g1` on account of the
  image

### Requirement: Release Deletion Has No Cluster Footprint

A `GatewayRelease` delete event SHALL NOT remove or disrupt any running Gateway
workload, because a release owns no Kubernetes resources. The control plane SHALL
treat a release delete as a terminal, idempotent no-op with respect to cluster
state, and SHALL NOT error when the release is already absent.

#### Scenario: Deleting a release leaves running gateways untouched

- GIVEN a `GatewayRelease` `r1` that Gateway `g1` was deployed from
- WHEN `r1` is deleted
- THEN `g1`'s running workload is unchanged
- AND the control plane reports the release reconcile as successful

### Requirement: Idempotent, Serialized Reconciliation

Release reconciliation SHALL be idempotent and SHALL be serialized per release so
that a retry never runs concurrently with a live event for the same release.
Re-reconciling an unchanged release SHALL converge to the same status and SHALL NOT
produce redundant status writes or redundant gateway fan-out.

#### Scenario: Repeated reconciles are stable

- GIVEN a valid `GatewayRelease` already reconciled to `Available`
- WHEN the control plane reconciles the same release again with no change
- THEN no status update and no gateway reconciliation are requested

### Requirement: Failure Handling Is Retried, Not Swallowed

When a release reconcile cannot complete because a dependency is transiently
unavailable (for example, the API server rejects the status write, or the set of
referencing Gateways cannot be listed), the control plane SHALL return an error so
the reconcile is requeued and retried, rather than silently succeeding. Partial
failures SHALL NOT be silently swallowed.

#### Scenario: Status write failure is retried

- GIVEN a valid `GatewayRelease` whose status must be updated to `Available`
- WHEN the status write to the API server fails transiently
- THEN the reconcile returns an error and is requeued for retry
