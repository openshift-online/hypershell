# Gateway Version Selection Specification

Date: 2026-09-03
Status: Draft

## Purpose

A Gateway declares the container image it runs in one of two ways: a direct
`image` reference, or a `release_id` that points at a `GatewayRelease` — the
platform's versioned, database-backed record of a gateway image. This spec
defines **database-backed gateway version selection**: how the cluster-local
control plane resolves a Gateway's `release_id` to the concrete image published
by its referenced `GatewayRelease` at reconcile time, and how that resolved
image relates to any direct `image` on the Gateway. This makes the
`GatewayRelease` table the source of truth for which image a Gateway deploys,
so operators change a gateway's version by pointing it at a release rather than
by editing a raw image string on every gateway.

## Scope Boundary

In scope:

- Resolving `gateway.release_id` to the `image` of the referenced
  `GatewayRelease` during gateway reconciliation.
- Precedence of the resolved release image over a Gateway's direct `image`.
- Deterministic, retried failure handling when the release cannot be resolved.

Out of scope (owned by sibling work):

- `GatewayRelease` image validation and status write-back (`Available` /
  `Invalid`) — see gateway-release reconciliation (HYPERSHELL-173).
- Rollout strategy and canary progression (`rollout_strategy`,
  `canary_percent`, `canary_duration`) — later tasks.
- Gating deployment on release `status` — a possible **future additive
  refinement** once release status write-back is in place; today the resolver
  MUST NOT gate on `status`, because a control plane that has not yet observed a
  release would otherwise refuse to deploy any release-backed gateway.
- Image reference *format* validation, which already occurs when the manifest is
  rendered.

## Domain Vocabulary

- **Resolved image**: the `image` field of the `GatewayRelease` identified by a
  Gateway's `release_id`.
- **Direct image**: the optional `image` field set directly on a Gateway.
- **Effective image**: the image the control plane writes into the rendered
  Deployment after applying the precedence rules below.

## Requirements

### Requirement: Release Image Resolution

The control plane SHALL, when a Gateway has a non-empty `release_id`, resolve
that `release_id` to its `GatewayRelease` and use that release's `image` as the
Gateway's effective image for the rendered Deployment.

#### Scenario: Gateway references a release

- GIVEN a `GatewayRelease` `r1` with image `registry.redhat.io/openshell/gateway:v2`
- AND a Gateway `g1` with `release_id = r1` and no direct image
- WHEN the control plane reconciles `g1`
- THEN the rendered Deployment's container image is `registry.redhat.io/openshell/gateway:v2`

### Requirement: Release Image Takes Precedence Over Direct Image

The control plane SHALL treat the resolved release image as taking precedence
over a Gateway's direct `image` when both are present, so that `release_id` is
authoritative for version selection.

#### Scenario: Both release_id and direct image set

- GIVEN a `GatewayRelease` `r1` with image `registry.redhat.io/openshell/gateway:v2`
- AND a Gateway `g1` with `release_id = r1` AND direct `image = registry.redhat.io/openshell/gateway:v1`
- WHEN the control plane reconciles `g1`
- THEN the rendered Deployment's container image is `registry.redhat.io/openshell/gateway:v2`
- AND the direct `image` value does not appear in the rendered Deployment

### Requirement: Direct Image Fallback When No Release Referenced

The control plane SHALL use a Gateway's direct `image` when the Gateway has no
`release_id`, and SHALL fall back to the platform default image when neither a
`release_id` nor a direct `image` is present, preserving today's behavior for
gateways that predate release-based selection.

#### Scenario: Only a direct image is set

- GIVEN a Gateway `g1` with an empty `release_id` and direct `image = registry.redhat.io/openshell/gateway:v1`
- WHEN the control plane reconciles `g1`
- THEN the rendered Deployment's container image is `registry.redhat.io/openshell/gateway:v1`

#### Scenario: Neither release nor image is set

- GIVEN a Gateway `g1` with an empty `release_id` and no direct `image`
- WHEN the control plane reconciles `g1`
- THEN the rendered Deployment uses the platform default gateway image

### Requirement: Unresolvable Release Fails The Reconcile, Not Silently

The control plane SHALL treat a `release_id` that cannot be resolved to a
release with a non-empty image as a reconcile failure: it MUST return an error
so the reconcile is retried, and MUST NOT deploy an empty or default image in
place of the intended release image.

#### Scenario: Referenced release does not exist

- GIVEN a Gateway `g1` with `release_id = missing`
- AND no `GatewayRelease` with that id
- WHEN the control plane reconciles `g1`
- THEN the reconcile returns an error and is retried
- AND no Deployment is rendered with a default or empty image for `g1`

#### Scenario: Referenced release has an empty image

- GIVEN a `GatewayRelease` `r1` whose `image` is empty
- AND a Gateway `g1` with `release_id = r1`
- WHEN the control plane reconciles `g1`
- THEN the reconcile returns an error and is retried

#### Scenario: Release lookup transiently fails

- GIVEN a Gateway `g1` with `release_id = r1`
- AND the release lookup RPC returns a transient error
- WHEN the control plane reconciles `g1`
- THEN the reconcile returns an error and is retried
- AND a later reconcile, once the lookup succeeds, deploys the resolved image

### Requirement: Version Change Through Release Reference Is Applied On Reconcile

Whenever a reconcile of a Gateway reaches image selection, the control plane
SHALL use the image of the release currently referenced by the Gateway's
`release_id`, so that repointing `release_id` to a different release changes the
Gateway's effective image. Whether an already-`Running` Gateway is re-reconciled
after such a change is governed by the provisioning gate and re-trigger
mechanisms defined elsewhere (see the [health spec](./openshell-gateway-health.spec.md)
and gateway-release fan-out, HYPERSHELL-173); this spec does not itself
guarantee a re-reconcile of a steady-state gateway.

#### Scenario: Reconcile after release_id repointed to a newer release

- GIVEN a Gateway `g1` previously deployed from release `r1` (image `:v1`)
- AND a `GatewayRelease` `r2` with image `:v2`
- AND `g1`'s `release_id` has been changed to `r2`
- WHEN the control plane reconciles `g1` and reaches image selection
- THEN the rendered Deployment's container image is `:v2`
