# Gateway Release Rollout

**Date:** 2026-09-10
**Status:** Active
**Jira:** HYPERSHELL-175

## Purpose

When a Gateway's effective image changes - because its `release_id` is repointed
to a different `GatewayRelease`, or because the referenced release's image is
updated and fanned out to referencing gateways - the control plane must roll the
running workload onto the new version *safely*: the last-good workload keeps
serving until the new revision passes its health gates, the rollout's progress
and the release actually serving are observable, and a rollout that times out or
whose new pods never become Ready is never reported as a successful move to the
new release.

Today the workload rolls via a static Kubernetes `RollingUpdate` strategy, but
readiness is judged only on `Deployment.Status.ReadyReplicas >= desired`. With
`maxUnavailable: 0`, `maxSurge: 1`, and a single replica, a still-Ready *old* pod
satisfies that check while the new pod is still starting or crash-looping, so a
defective new release can be reported `Running` (or left `Provisioning`) on the
old revision rather than surfaced as a failed rollout. This spec closes that gap:
it makes rollout readiness revision-aware, requires the update strategy that
preserves the last-good workload, and defines how rollout state and the observed
release are reported.

This spec is a sub-spec of [`control-plane.spec.md`](./control-plane.spec.md) and
refines its "Manage release rollouts" responsibility. It builds on
[`gateway-version-selection.spec.md`](./gateway-version-selection.spec.md) (how a
Gateway's `release_id` resolves to an effective image) and
[`gateway-release-reconciliation.spec.md`](./gateway-release-reconciliation.spec.md)
(how a release image change fans out to referencing gateways), and it reuses the
canonical phase vocabulary in
[`gateway-phase-vocabulary.spec.md`](./gateway-phase-vocabulary.spec.md).

## Scope Boundary

- **In scope:**
  - Rolling a Gateway workload onto a changed effective image while preserving the
    last-good revision until the new revision passes its health gates.
  - Judging rollout readiness on the *new* revision, not on any still-Ready old
    pod.
  - Reporting rollout state (through the canonical `phase`/`status`) and the
    release currently rolled out (the observed release) back to the API server.
  - Deterministic, retried handling of a rollout that times out or whose new pods
    never become Ready, without reporting the new release as ready and without
    silently swallowing the failure.
- **Out of scope (sibling tickets):**
  - **Canary / progressive traffic strategies** (`rollout_strategy`,
    `canary_percent`, `canary_duration` on `GatewayRelease`). Those fields are
    persisted and API-served today but are not consumed by the control plane;
    this spec governs the safe single-track rolling update, not traffic
    splitting. Canary progression remains a later task.
  - How `release_id` resolves to an effective image (owned by
    gateway-version-selection).
  - What triggers a re-reconcile of a steady-state Gateway (owned by the
    release fan-out and the provisioning/convergence gate); this spec governs how
    a rollout proceeds safely *once triggered*.
  - Multi-replica surge tuning and per-release replica counts.

## Domain Vocabulary

- **Effective image**: the image the control plane writes into the rendered
  Deployment after applying the version-selection precedence rules (release image
  over direct image over platform default).
- **New revision**: the Deployment pod template produced by the current
  effective image (and current config-hash); Kubernetes rolls the Deployment to
  this template.
- **Last-good workload**: the previously-Ready revision that keeps serving until
  the new revision is available.
- **Observed release**: the release whose effective image the control plane has
  successfully rolled out and observed Ready - reported on the Gateway as
  `observed_release_id`, distinct from the desired `release_id`. It is
  control-plane-owned and empty until the first successful rollout (and empty for
  a gateway deployed from a direct `image` with no `release_id`).
- **Health gate**: the readiness evaluation that must pass before the new
  revision is reported as the running one (workload readiness of the new
  revision, plus route readiness for a routed gateway).

## Requirements

### Requirement: Last-Good Workload Preserved Until the New Revision Is Ready

The control plane SHALL roll a changed effective image onto the Gateway using a
Deployment update strategy that never reduces serving capacity below the desired
replica count while the new revision is not yet available: `RollingUpdate` with
`maxUnavailable: 0` and a positive `maxSurge`. A new revision that never becomes
Ready SHALL leave the last-good workload serving; it SHALL NOT tear down the
last-good pods to make room for an unready new revision.

#### Scenario: New revision surges before the old is removed

- GIVEN a `Running` Gateway serving release `r1` (image `:v1`) with one ready pod
- WHEN its effective image changes to `:v2` and the control plane applies the
  Deployment
- THEN the Deployment SHALL create the `:v2` pod while the `:v1` pod keeps serving
- AND the `:v1` pod SHALL NOT be removed until the `:v2` pod is Ready

#### Scenario: Crash-looping new image does not drop serving capacity

- GIVEN a `Running` Gateway serving release `r1` (image `:v1`)
- WHEN its effective image changes to `:v2` and the `:v2` pod crash-loops and
  never becomes Ready
- THEN the `:v1` pod SHALL keep serving throughout
- AND serving capacity SHALL NOT fall below the desired replica count on account
  of the failed rollout

### Requirement: Rollout Readiness Is Judged On the New Revision

The control plane SHALL judge a rollout complete only when the *new* revision's
pods are available, not when any pod (including a still-Ready old pod) is ready.
Concretely, readiness of a rollout SHALL require that the Deployment's observed
generation has caught up to its desired generation AND that the updated replicas
are available at the desired count, so a still-Ready old pod cannot satisfy the
gate while the new revision is unready.

#### Scenario: Old pod ready, new pod not yet ready is not "rolled out"

- GIVEN a rollout in progress with the old `:v1` pod Ready and the new `:v2` pod
  not yet Ready
- WHEN the control plane evaluates rollout readiness
- THEN it SHALL NOT report the rollout as complete
- AND the Gateway SHALL remain `Provisioning` for the rollout rather than
  `Running` on the new release

#### Scenario: New revision available completes the rollout

- GIVEN a rollout in progress
- WHEN the Deployment's updated replicas become available at the desired count
  and its observed generation has caught up
- THEN the control plane SHALL report the rollout as complete
- AND (for a non-routed gateway) set `phase` to `Running` and `status` to
  `Healthy`

### Requirement: Rollout State and Observed Release Are Reported

The control plane SHALL report rollout progress through the canonical Gateway
`phase`/`status` and SHALL report the release currently rolled out as the
Gateway's `observed_release_id`. While a new revision is rolling, `phase` SHALL be
`Provisioning`. The control plane SHALL advance `observed_release_id` to the
Gateway's desired `release_id` ONLY after the new revision has passed its health
gates; until then `observed_release_id` SHALL continue to report the last
successfully rolled-out release. `observed_release_id` is control-plane-owned:
it SHALL be read-only to REST clients and writable by the control plane through
the same gRPC back-channel used for `phase`, `status`, and `route_address`.

#### Scenario: Observed release advances only after health gates pass

- GIVEN a `Running` Gateway with `release_id = r1` and `observed_release_id = r1`
- WHEN `release_id` is repointed to `r2` and the control plane rolls it out
- THEN `observed_release_id` SHALL remain `r1` while the `r2` revision is rolling
- AND `observed_release_id` SHALL become `r2` only after the `r2` revision is
  observed Ready (and, for a routed gateway, its route is Ready)

#### Scenario: Rollout state is visible during the roll

- GIVEN a Gateway whose effective image has just changed
- WHEN the control plane applies the new revision and waits for readiness
- THEN the Gateway `phase` SHALL be `Provisioning` during the roll
- AND `status` SHALL carry a human-readable descriptor of the rollout progress

### Requirement: Timeout Or Failed Readiness Does Not Report the New Release As Ready

When a new revision does not become Ready within the provisioning readiness
window, the control plane SHALL NOT advance `observed_release_id` to the new
release and SHALL NOT report the Gateway as `Running` on the new release. It SHALL
set `phase` to `Degraded` with a human-readable `status` naming the stalled
rollout (for example, the unready-replicas reason), leave `observed_release_id`
at the last successfully rolled-out release, and allow the reconcile to be
retried so a subsequently-healthy new revision still converges. A rendered-config
or otherwise non-recoverable rollout failure SHALL settle the Gateway to `Failed`
per the existing provisioning failure semantics, likewise without advancing
`observed_release_id`.

#### Scenario: Readiness window elapses on the new revision

- GIVEN a rollout to release `r2` whose new pod does not become Ready within the
  readiness window
- WHEN the window elapses
- THEN the control plane SHALL set `phase` to `Degraded` with a reason describing
  the unready new revision
- AND `observed_release_id` SHALL remain the last good release (`r1`)
- AND the Gateway SHALL NOT be reported `Running` on `r2`

#### Scenario: Later-healthy new revision converges on retry

- GIVEN a rollout to `r2` that was reported `Degraded` because the new pod was not
  Ready in time
- WHEN a later reconcile observes the `r2` revision available at the desired count
- THEN the control plane SHALL set `phase` to `Running` and `status` to `Healthy`
- AND advance `observed_release_id` to `r2`

### Requirement: Unchanged Release Does Not Re-Roll

Re-reconciling a Gateway whose effective image and config are unchanged SHALL NOT
roll the workload and SHALL NOT rewrite `observed_release_id`. A duplicate watch
event, a health tick, or a fan-out for an unrelated release SHALL be idempotent
with respect to the rollout.

#### Scenario: Duplicate reconcile of a converged rollout is a no-op

- GIVEN a `Running` Gateway with `observed_release_id == release_id` whose
  effective image and config are unchanged
- WHEN the control plane reconciles it again
- THEN it SHALL NOT roll the Deployment
- AND it SHALL NOT issue a redundant `observed_release_id` write

### Requirement: Rollout Failures Are Surfaced, Not Swallowed

The control plane SHALL NOT silently swallow a partial rollout failure. Every
rollout failure path SHALL either return an error so the reconcile is requeued
and retried, or record it on the Gateway as `Degraded`/`Failed` with a
human-readable reason; none SHALL be reported as a successful rollout. A transient
failure to observe Deployment state or to write the Gateway's rollout report back
to the API server SHALL be returned as an error and retried, not treated as
success.

#### Scenario: Deployment status observation fails transiently

- GIVEN a rollout in progress
- WHEN observing the Deployment's rollout status returns a transient error
- THEN the control plane SHALL return an error so the reconcile is retried
- AND it SHALL NOT report the rollout as complete or advance
  `observed_release_id`

#### Scenario: Observed-release write-back fails

- GIVEN a new revision that has passed its health gates
- WHEN writing `observed_release_id` back to the API server fails transiently
- THEN the control plane SHALL return an error so the write is retried
- AND it SHALL NOT leave the Gateway reporting the new release as rolled out until
  the write succeeds
