# Generated Gateway Configuration Validation

**Date:** 2026-09-08
**Status:** Active
**Jira:** HYPERSHELL-179

## Purpose

The control plane assembles each gateway's runtime configuration (the
`gateway.toml` carried in the `openshell-gateway-config` ConfigMap) by mutating a
base configuration in place: it rewrites the TLS `server_sans` line, appends an
`[openshell.gateway.oidc]` section when OIDC is enabled, and appends a
credential-driver section when a driver is configured. Today the control plane
validates only the *input* it was given (image reference, DNS names, OIDC field
coherence, credential-driver fields) and then writes the *generated* artifact to
the cluster without ever parsing or checking it. A defect in generation - or an
input combination that yields a structurally incoherent artifact - is therefore
delivered straight to the workload, and because the ConfigMap's content hash is
stamped onto the Deployment pod template, a bad artifact also rolls the workload
onto itself.

This spec defines a validation gate on the *rendered* configuration artifact
itself, applied before the control plane writes any config-derived resource for
the gateway (the `openshell-gateway-config` ConfigMap and the workload
Deployment) or rolls its workload. It refines the "validate the TOML config" clause
of the GatewayReconciler requirement in
[`openshell-gateway.spec.md`](./openshell-gateway.spec.md) and closes the
long-standing gap that input validation alone does not cover the produced
artifact. It reuses the canonical phase vocabulary defined in
[`gateway-phase-vocabulary.spec.md`](./gateway-phase-vocabulary.spec.md) rather
than introducing new outcome values, and it is a sub-spec of
[`control-plane.spec.md`](./control-plane.spec.md).

## Scope Boundary

- **In scope:** validating the fully rendered gateway configuration artifact
  (the generated `gateway.toml`) for well-formedness and structural coherence;
  gating the per-gateway workload rollout on that validation so an invalid
  artifact is never written to the cluster; preserving a running gateway's
  last-good configuration when a new render is invalid; surfacing failures
  through the canonical `Failed` phase with a human-readable reason; and doing so
  idempotently so a corrected gateway self-heals on the next reconcile.
- **Out of scope (deferred or covered elsewhere):**
  - Fleet-wide or version-driven *release* rollout safety, including progressive
    and canary rollout - this spec gates the per-gateway workload rollout inside
    a single reconcile, not the propagation of a `GatewayRelease` across the
    fleet (see HYPERSHELL-175 safe release rollout and HYPERSHELL-176 canary).
  - Runtime verification that the gateway *process* accepts the configuration;
    continuous readiness and degradation are defined in
    [`openshell-gateway-health.spec.md`](./openshell-gateway-health.spec.md).
    This spec is a static, control-plane-side check of the artifact, not a live
    probe.
  - Semantic validation that requires an external call (OIDC issuer
    reachability, JWKS resolution, verifying the realm emits a claim); input
    validation already defers these deliberately, and this spec does not add
    them.
  - Adding any new value to the Gateway phase or status vocabulary.

## Requirements

### Requirement: Validate the Rendered Configuration Before Rollout

The control plane SHALL validate the fully rendered gateway configuration
artifact on every create and update reconcile, before it writes that artifact to
the cluster and before it rolls the gateway workload onto it. This validation is
distinct from and additional to the existing input-field validation: input
validation checks the declared configuration the control plane was given, whereas
this checks the artifact the control plane produced from it.

Validation SHALL at minimum confirm that the rendered configuration is
well-formed (it parses successfully as the configuration format the gateway
consumes) and that its control-plane-managed sections are structurally coherent
with the declared intent (for example, when OIDC is enabled the rendered artifact
carries the OIDC section and marks the gateway as requiring authentication rather
than allowing unauthenticated users).

#### Scenario: Well-formed rendered configuration passes

- GIVEN a Gateway whose declared configuration renders to a well-formed,
  structurally coherent artifact
- WHEN the control plane reconciles the Gateway
- THEN rendered-configuration validation succeeds
- AND the control plane proceeds to apply the gateway's resources

#### Scenario: Malformed rendered configuration fails validation

- GIVEN a Gateway whose rendered configuration artifact does not parse as
  well-formed
- WHEN the control plane reconciles the Gateway
- THEN rendered-configuration validation fails
- AND the control plane applies no Kubernetes resource derived from that artifact

#### Scenario: Structurally incoherent rendered configuration fails validation

- GIVEN a Gateway with OIDC enabled whose rendered configuration omits the OIDC
  section or still permits unauthenticated users
- WHEN the control plane reconciles the Gateway
- THEN rendered-configuration validation fails with a reason describing the
  structural inconsistency

### Requirement: A Validation Failure Blocks Rollout Without Disrupting a Running Gateway

When rendered-configuration validation fails, the control plane SHALL NOT apply
the gateway's ConfigMap and SHALL NOT roll its workload; it SHALL apply no
Kubernetes resources derived from the invalid artifact. A Gateway that is already
`Running` SHALL continue serving its last successfully applied configuration and
SHALL NOT be restarted onto, or degraded by, the invalid artifact.

#### Scenario: New gateway with an invalid rendered configuration

- GIVEN a newly created Gateway whose rendered configuration is invalid
- WHEN the control plane reconciles the Gateway
- THEN no ConfigMap, Deployment, or other configuration-derived resource is
  created
- AND no workload is rolled out for the Gateway

#### Scenario: Update that renders invalid does not disturb a running gateway

- GIVEN a `Running` Gateway serving a previously validated configuration
- AND a subsequent update whose rendered configuration is invalid
- WHEN the control plane reconciles the update
- THEN the control plane does not write the invalid ConfigMap
- AND it does not trigger a rolling restart of the workload
- AND the Gateway keeps serving its last successfully applied configuration

### Requirement: Validation Failures Are Observable

On a rendered-configuration validation failure the control plane SHALL set the
Gateway `phase` to the canonical `Failed` value with a human-readable `status`
reason that identifies the failure as a generated-configuration validation error,
and SHALL log the error together with the Gateway name and its assigned
namespace. It SHALL NOT introduce a phase or status value outside the canonical
vocabulary.

#### Scenario: Failure settles the gateway to Failed with a reason

- GIVEN a Gateway whose rendered configuration fails validation
- WHEN the control plane reconciles the Gateway
- THEN it SHALL set the Gateway `phase` to `Failed`
- AND it SHALL set a human-readable `status` reason identifying a
  generated-configuration validation failure

#### Scenario: Failure is logged with identifying context

- GIVEN a rendered-configuration validation failure
- WHEN the control plane records the failure
- THEN the log entry SHALL include the Gateway name and its assigned namespace

### Requirement: Validation Is Idempotent and Self-Correcting

Rendered-configuration validation SHALL be deterministic for a given Gateway
state and SHALL leave no partial artifacts behind on failure. When a later
reconcile renders a valid configuration - for example after the Gateway's
declared configuration is corrected - the control plane SHALL proceed with the
normal rollout and settle the Gateway toward its healthy phase, with no manual
cleanup required.

#### Scenario: Corrected gateway rolls out on the next reconcile

- GIVEN a Gateway previously settled to `Failed` by a validation failure
- WHEN its declared configuration is corrected so the artifact renders valid
- AND the control plane reconciles the Gateway again
- THEN validation succeeds
- AND the control plane applies the gateway's resources and drives it toward its
  healthy phase

#### Scenario: Repeated reconciles of the same invalid state are stable

- GIVEN a Gateway whose rendered configuration is invalid
- WHEN the control plane reconciles it repeatedly without any change
- THEN each reconcile produces the same `Failed` outcome
- AND no partial or leftover configuration-derived resources accumulate

## Design Decisions

- **Reuse `Failed`, do not add a phase.** A validation failure means provisioning
  cannot complete and requires a change to recover, which is exactly the meaning
  of the canonical `Failed` phase. Adding a dedicated phase would violate the
  single-source-of-truth requirement in
  [`gateway-phase-vocabulary.spec.md`](./gateway-phase-vocabulary.spec.md); the
  specific cause is carried in the human-readable `status` reason instead.
- **Validate the artifact up front, before any write.** Because the ConfigMap
  content hash is stamped onto the Deployment pod template to trigger rolling
  restarts, writing an invalid ConfigMap would also roll a healthy workload onto
  a broken configuration. Gating before the first resource write keeps a running
  gateway on its last-good configuration and confines the failure to a status
  update.
- **Distinct from input validation.** This refines the "TOML config" clause of
  the GatewayReconciler validation-failure scenario in
  [`openshell-gateway.spec.md`](./openshell-gateway.spec.md). Input validation
  stays as-is; this adds a check on the produced artifact, which input validation
  cannot cover.

## Primary Basis

- [`openshell-gateway.spec.md`](./openshell-gateway.spec.md) - GatewayReconciler
  provisioning and validation-failure behavior.
- [`gateway-phase-vocabulary.spec.md`](./gateway-phase-vocabulary.spec.md) -
  canonical phase and status vocabulary and its single source of truth.
- [`control-plane.spec.md`](./control-plane.spec.md) - overall control-plane
  reconciliation responsibilities.
