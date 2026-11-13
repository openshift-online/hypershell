# Generated Gateway Configuration Validation

**Date:** 2026-09-08
**Status:** Active
**Jira:** HYPERSHELL-179

## Purpose

The control plane configures each gateway's runtime through Helm values on the
vendored OpenShell chart. The chart renders those values into the `gateway.toml`
carried in the `openshell-gateway-config` ConfigMap: the TLS `server_sans`, the
`[openshell.gateway.oidc]` section that every gateway carries, and a
credential-driver section when a driver is configured.

Helm renders that file, not the control plane, so the control plane never holds
the rendered `gateway.toml`. It validates the *generated configuration* instead:
the gateway configuration it builds and turns into Helm values. The chart renders
each control-plane-managed section from those values, so a value that is wrong
or missing before rollout is a section that is wrong or missing after it.

A defect that reaches the workload is costly. The ConfigMap's content hash is
stamped onto the Deployment pod template, so a bad configuration also rolls the
workload. This spec defines a validation gate on the generated configuration,
applied before the control plane installs or upgrades the gateway's Helm release.
That release writes the `openshell-gateway-config` ConfigMap and the workload
Deployment and rolls the workload. It refines the "validate the TOML config"
clause of the GatewayReconciler requirement in
[`openshell-gateway.spec.md`](./openshell-gateway.spec.md). It reuses the
canonical phase vocabulary defined in
[`gateway-phase-vocabulary.spec.md`](./gateway-phase-vocabulary.spec.md) rather
than introducing new outcome values, and it is a sub-spec of
[`control-plane.spec.md`](./control-plane.spec.md).

## Scope Boundary

- **In scope:** validating the generated gateway configuration for the
  control-plane-managed sections the chart renders from it; gating the
  per-gateway workload rollout on that validation so an invalid configuration is
  never written to the cluster; preserving a running gateway's last-good
  configuration when a new one is invalid; surfacing failures through the
  canonical `Failed` phase with a human-readable reason; and doing so
  idempotently so a corrected gateway self-heals on the next reconcile.
- **Out of scope (deferred or covered elsewhere):**
  - Parsing or inspecting the rendered `gateway.toml`. Helm renders it outside the
    control plane; correct rendering of a given value is the vendored chart's
    responsibility.
  - Fleet-wide or version-driven rollout safety across gateways - this spec
    gates the per-gateway workload rollout inside a single reconcile, not the
    propagation of a new gateway version across the fleet.
  - Runtime verification that the gateway *process* accepts the configuration;
    continuous readiness and degradation are defined in
    [`openshell-gateway-health.spec.md`](./openshell-gateway-health.spec.md).
    This spec is a static, control-plane-side check, not a live probe.
  - Semantic validation that requires an external call (OIDC issuer
    reachability, JWKS resolution, verifying the realm emits a claim); input
    validation already defers these deliberately, and this spec does not add
    them.
  - Adding any new value to the Gateway phase or status vocabulary.

## Requirements

### Requirement: GTW-CFG-01 -- Validate the Generated Configuration Before Rollout

The control plane SHALL validate the generated gateway configuration on every
create and update reconcile, before it installs or upgrades the gateway's Helm
release. This is additional to the existing input-field validation (image
reference, DNS names, OIDC field coherence, credential-driver fields).

Every gateway has an OIDC configuration, because Keycloak integration is
mandatory ([`openshell-gateway-keycloak.spec.md`](./openshell-gateway-keycloak.spec.md),
OSG-KCLK-01). Validation SHALL therefore always require the generated OIDC
configuration to carry a non-empty `issuer` and a non-empty `audience`. The chart
renders the `[openshell.gateway.oidc]` section from these values. The control
plane never sets `allow_unauthenticated_users = true`.

A generated configuration without OIDC SHALL fail the reconcile as an invalid
gateway configuration. The reason is that a gateway without an auth mechanism
cannot run: since OpenShell `v0.1.5-rhaiv.4` the gateway refuses to start without
one and crash-loops (openshift-online/hypershell#809). Rejecting the
configuration is better than shipping that gateway.

#### Scenario: GTW-CFG-01.01 -- Complete generated configuration passes

- GIVEN a Gateway whose generated OIDC configuration has an issuer and an audience
- WHEN the control plane reconciles the Gateway
- THEN generated-configuration validation succeeds
- AND the control plane proceeds to apply the gateway's resources

#### Scenario: GTW-CFG-01.04 -- Generated configuration without OIDC is rejected

- GIVEN a Gateway whose generated OIDC configuration has an empty `issuer` or an
  empty `audience`
- WHEN the control plane reconciles the Gateway
- THEN generated-configuration validation fails as an invalid gateway
  configuration with a reason that names the missing OIDC values
- AND the control plane installs or upgrades no Helm release for the Gateway
- AND no gateway workload is rolled out, so no crash-looping gateway is shipped

### Requirement: GTW-CFG-02 -- A Validation Failure Blocks Rollout Without Disrupting a Running Gateway

When generated-configuration validation fails, the control plane SHALL NOT apply
the gateway's ConfigMap and SHALL NOT roll its workload; it SHALL apply no
Kubernetes resources derived from the invalid configuration. A Gateway that is already
`Running` SHALL continue serving its last successfully applied configuration and
SHALL NOT be restarted onto, or degraded by, the invalid configuration.

#### Scenario: GTW-CFG-02.01 -- New gateway with an invalid generated configuration

- GIVEN a newly created Gateway whose generated configuration is invalid
- WHEN the control plane reconciles the Gateway
- THEN no ConfigMap, Deployment, or other configuration-derived resource is
  created
- AND no workload is rolled out for the Gateway

#### Scenario: GTW-CFG-02.02 -- Invalid update does not disturb a running gateway

- GIVEN a `Running` Gateway serving a previously validated configuration
- AND a subsequent update whose generated configuration is invalid
- WHEN the control plane reconciles the update
- THEN the control plane does not write the invalid ConfigMap
- AND it does not trigger a rolling restart of the workload
- AND the Gateway keeps serving its last successfully applied configuration

### Requirement: GTW-CFG-03 -- Validation Failures Are Observable

On a generated-configuration validation failure the control plane SHALL set the
Gateway `phase` to the canonical `Failed` value with a human-readable `status`
reason that identifies the failure as a generated-configuration validation error,
and SHALL log the error together with the Gateway name and its assigned
namespace. It SHALL NOT introduce a phase or status value outside the canonical
vocabulary.

#### Scenario: GTW-CFG-03.01 -- Failure settles the gateway to Failed with a reason

- GIVEN a Gateway whose generated configuration fails validation
- WHEN the control plane reconciles the Gateway
- THEN it SHALL set the Gateway `phase` to `Failed`
- AND it SHALL set a human-readable `status` reason identifying a
  generated-configuration validation failure

#### Scenario: GTW-CFG-03.02 -- Failure is logged with identifying context

- GIVEN a generated-configuration validation failure
- WHEN the control plane records the failure
- THEN the log entry SHALL include the Gateway name and its assigned namespace

### Requirement: GTW-CFG-04 -- Validation Is Idempotent and Self-Correcting

Generated-configuration validation SHALL be deterministic for a given Gateway
state and SHALL leave no partial artifacts behind on failure. When a later
reconcile generates a valid configuration - for example after the Gateway's
declared configuration is corrected - the control plane SHALL proceed with the
normal rollout and settle the Gateway toward its healthy phase, with no manual
cleanup required.

#### Scenario: GTW-CFG-04.01 -- Corrected gateway rolls out on the next reconcile

- GIVEN a Gateway previously settled to `Failed` by a validation failure
- WHEN its configuration is corrected so the generated configuration is valid
- AND the control plane reconciles the Gateway again
- THEN validation succeeds
- AND the control plane applies the gateway's resources and drives it toward its
  healthy phase

#### Scenario: GTW-CFG-04.02 -- Repeated reconciles of the same invalid state are stable

- GIVEN a Gateway whose generated configuration is invalid
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
- **Validate before any configuration write.** Because the ConfigMap
  content hash is stamped onto the Deployment pod template to trigger rolling
  restarts, writing an invalid ConfigMap would also roll a healthy workload onto
  a broken configuration. Gating before the Helm install or upgrade keeps a running
  gateway on its last-good configuration and confines the failure to a status
  update.
- **Validate the generated configuration, not the rendered file.** Helm renders
  `gateway.toml` outside the control plane, so the control plane cannot parse it
  without rendering the chart a second time. The chart renders each
  control-plane-managed section from the generated values, so checking those
  values before rollout gives the same protection at no extra cost. This refines
  the "TOML config" clause of the GatewayReconciler validation-failure scenario
  in [`openshell-gateway.spec.md`](./openshell-gateway.spec.md).

## Primary Basis

- [`openshell-gateway.spec.md`](./openshell-gateway.spec.md) - GatewayReconciler
  provisioning and validation-failure behavior.
- [`gateway-phase-vocabulary.spec.md`](./gateway-phase-vocabulary.spec.md) -
  canonical phase and status vocabulary and its single source of truth.
- [`control-plane.spec.md`](./control-plane.spec.md) - overall control-plane
  reconciliation responsibilities.
