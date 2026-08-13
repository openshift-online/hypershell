# OpenShell Gateway Health & Phase Lifecycle

**Date:** 2026-08-13
**Status:** Active

## Purpose

This spec defines how the HyperShell control plane derives and reports a
Gateway's lifecycle `phase` and runtime `status` from the actual state of its
Kubernetes workload. The `phase` a client reads (in the web console or CLI) must
correspond to the observed readiness and ongoing health of the gateway
Deployment - not merely to the fact that the control plane finished applying
manifests. A gateway whose pod is crash-looping must never be reported as
`Running`.

This spec is a sub-spec of [`control-plane.spec.md`](./control-plane.spec.md)
and refines its "Gateway Reconciliation" and "Status Synchronization"
requirements. Provisioning mechanics are defined in
[`openshell-gateway.spec.md`](./openshell-gateway.spec.md).

## Domain Vocabulary

A Gateway carries two independently-observable fields:

- **`phase`** - the lifecycle state managed by the control plane. Allowed
  values:
  - `Pending` - accepted, not yet acted on by the reconciler.
  - `Provisioning` - manifests are being applied and dependent workloads
    (database, gateway Deployment) are not yet Ready.
  - `Running` - the gateway Deployment is observed Ready (ready replicas ≥
    desired replicas).
  - `Degraded` - the gateway was provisioned but its workload is currently
    unhealthy (ready replicas below desired, e.g. CrashLoopBackOff). Recoverable.
  - `Failed` - provisioning could not complete (templating, apply, or a
    non-recoverable error). Requires a change to recover.
- **`status`** - a short human-readable health descriptor for the workload
  (e.g. the reason a gateway is `Degraded`). It complements `phase` and is
  surfaced alongside it in the console.

`Running` is the only phase that asserts the gateway is serving. `Degraded` and
`Failed` are the two distinct unhealthy states: `Degraded` is recoverable
without user action; `Failed` is not.

## Requirements

### Requirement: Phase Reflects Workload Readiness

The control plane SHALL NOT report a Gateway `phase` of `Running` until the
gateway's Kubernetes Deployment is observed Ready - that is, its ready replicas
are greater than or equal to its desired replicas. While the Deployment has been
applied but has not yet reached readiness, the phase SHALL be `Provisioning`.

Readiness observation SHALL apply to the `openshell-gateway` Deployment itself,
not only to its dependencies (such as the gateway database).

#### Scenario: Deployment becomes ready

- GIVEN a Gateway whose manifests have been applied
- WHEN the `openshell-gateway` Deployment reaches ready replicas ≥ desired
- THEN the control plane SHALL set the Gateway `phase` to `Running`

#### Scenario: Gateway pod crash-loops during provisioning

- GIVEN a Gateway whose `openshell-gateway` Deployment has been applied
- AND the gateway pod exits non-zero on startup (e.g. OIDC discovery fails) and
  enters CrashLoopBackOff
- WHEN the Deployment does not reach readiness within the provisioning
  readiness window
- THEN the control plane SHALL NOT set the `phase` to `Running`
- AND it SHALL set the `phase` to `Degraded`
- AND it SHALL record the reason in `status`

#### Scenario: Provisioning fails to apply

- GIVEN a Gateway being reconciled
- WHEN manifest templating or applying a resource returns an error
- THEN the control plane SHALL set the `phase` to `Failed`

### Requirement: Continuous Gateway Health Reconciliation

After a Gateway reaches `Running`, the control plane SHALL continue to observe
the health of its Deployment and keep the `phase` synchronized with actual
workload state. If ready replicas fall below desired replicas (crash, OOM,
eviction, image pull failure), the control plane SHALL set the `phase` to
`Degraded` within one health-reconciliation interval. When the workload returns
to full readiness, the control plane SHALL set the `phase` back to `Running`.

#### Scenario: Running gateway begins crash-looping

- GIVEN a Gateway with `phase` `Running`
- WHEN its gateway pod begins failing and ready replicas drop below desired
- THEN the control plane SHALL set the `phase` to `Degraded` within one
  health-reconciliation interval
- AND record the reason in `status`

#### Scenario: Degraded gateway recovers

- GIVEN a Gateway with `phase` `Degraded`
- WHEN its Deployment returns to ready replicas ≥ desired
- THEN the control plane SHALL set the `phase` back to `Running`

### Requirement: Health Reconciliation Not Suppressed By Phase

The control plane's phase gate SHALL prevent redundant re-provisioning
(re-applying manifests) of a Gateway that is already `Provisioning` or
`Running`, but SHALL NOT prevent phase or status updates that reflect the
Gateway's actual observed workload health. A Gateway that has reached `Running`
SHALL still be able to transition to `Degraded`, and a `Degraded` Gateway SHALL
still be able to return to `Running`.

#### Scenario: Health update proceeds despite provisioning gate

- GIVEN a Gateway with `phase` `Running` whose manifests are unchanged
- WHEN a reconciliation or health check occurs
- THEN the control plane SHALL skip re-applying the gateway manifests
- BUT it SHALL still update the `phase` to `Degraded` if the workload is
  observed unhealthy

### Requirement: Console Reflects Recoverable States

The web console SHALL treat recoverable, non-terminal phases - including
`Pending`, `Provisioning`, and `Degraded` - as states that require continued
status polling, so that a Gateway recovering from `Degraded` to `Running`, or
failing from `Running` to `Degraded`, is reflected without a manual refresh.

#### Scenario: Degraded gateway keeps polling

- GIVEN a Gateway displayed in the console with `phase` `Degraded`
- WHEN the console evaluates whether to poll for status
- THEN it SHALL continue polling until the Gateway settles into a terminal
  healthy (`Running`) or non-recoverable (`Failed`) state
