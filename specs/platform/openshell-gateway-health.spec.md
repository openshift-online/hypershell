# OpenShell Gateway Health & Phase Lifecycle

**Date:** 2026-08-14
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
  - `Running` - the gateway is fully serving. For a gateway **with route
    configuration**, this requires **both** that the gateway Deployment is
    observed Ready (ready replicas ≥ desired replicas) **and** that its external
    exposure is observed Ready (for the Gateway API adapter: the per-tenant
    Gateway API `Gateway` reports `Programmed=True` with a non-empty
    `.status.addresses`). For a gateway **without route configuration**,
    Deployment readiness alone is sufficient.
  - `Degraded` - the gateway was provisioned but its workload or its external
    exposure is currently unhealthy (ready replicas below desired, e.g.
    CrashLoopBackOff; or the external exposure failed to become Ready within the
    route-readiness grace window, or lost readiness after reaching `Running`).
    Recoverable.
  - `Failed` - provisioning could not complete (templating, apply, or a
    non-recoverable error). Requires a change to recover.
- **`status`** - a short human-readable health descriptor for the workload
  (e.g. the reason a gateway is `Degraded`). It complements `phase` and is
  surfaced alongside it in the console.

A Gateway additionally carries two generation markers (see
[`data-model.spec.md`](./data-model.spec.md) § Gateway Generation Tracking):

- **`generation`** - the desired-spec version, incremented by the API server on
  any desired-spec change.
- **`observed_generation`** - the `generation` the control plane last
  successfully applied. A Gateway is *converged* when
  `observed_generation == generation`.

`generation`/`observed_generation` describe whether desired state has been
applied; `phase`/`status` describe observed workload health. The two axes are
independent: a Gateway can be `Running` yet not converged (a newer spec has not
been applied).

`Running` is the only phase that asserts the gateway is serving. `Degraded` and
`Failed` are the two distinct unhealthy states: `Degraded` is recoverable
without user action; `Failed` is not.

## Requirements

### Requirement: Phase Reflects Workload and Route Readiness

The control plane SHALL NOT report a Gateway `phase` of `Running` until the
gateway's Kubernetes Deployment is observed Ready - that is, its ready replicas
are greater than or equal to its desired replicas. Additionally, for a gateway
**with route configuration**, the control plane SHALL NOT report `Running` until
the gateway's external exposure is also observed Ready. While the Deployment has
been applied but has not yet reached readiness, or the Deployment is Ready but
the external exposure has not, the phase SHALL be `Provisioning`.

Readiness observation SHALL apply to the `openshell-gateway` Deployment itself,
not only to its dependencies (such as the gateway database).

External-exposure readiness SHALL be observed through the Gateway Exposure port
(see [`openshell-gateway-routing.spec.md`](./openshell-gateway-routing.spec.md)
§ Gateway Exposure Port), so that the health decision is independent of the
concrete exposure backend. For the Gateway API adapter, "Ready" means the
per-tenant Gateway API `Gateway` reports condition `Programmed=True` and has a
non-empty `.status.addresses`.

#### Scenario: Deployment ready but route not yet programmed

- GIVEN a routed Gateway whose `openshell-gateway` Deployment has reached ready
  replicas ≥ desired
- AND its external exposure is not yet Ready (for the Gateway API adapter, the
  per-tenant `Gateway` reports `Programmed=False` or has no assigned address)
- THEN the control plane SHALL keep the Gateway `phase` at `Provisioning`
- AND it SHALL NOT set the `phase` to `Running`

#### Scenario: Deployment ready and route becomes programmed

- GIVEN a routed Gateway whose Deployment is Ready and whose phase is
  `Provisioning`
- WHEN its external exposure becomes Ready (for the Gateway API adapter, the
  per-tenant `Gateway` reports `Programmed=True` with a non-empty
  `.status.addresses`)
- THEN the control plane SHALL set the Gateway `phase` to `Running`

#### Scenario: Non-routed gateway becomes ready

- GIVEN a Gateway with no `route` configuration whose manifests have been applied
- WHEN the `openshell-gateway` Deployment reaches ready replicas ≥ desired
- THEN the control plane SHALL set the Gateway `phase` to `Running`
  without requiring any external-exposure readiness

#### Scenario: Route never becomes ready within the grace window

- GIVEN a routed Gateway whose Deployment is Ready
- AND whose external exposure does not become Ready within the route-readiness
  grace window (see routing spec § Gateway Exposure Configuration)
- THEN the control plane SHALL set the `phase` to `Degraded`
- AND it SHALL record the reason in `status` (e.g. "route not programmed after
  <window>")
- AND it SHALL continue to observe the exposure, so that if it later becomes
  Ready the phase returns to `Running`

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
both the health of its Deployment and, for routed gateways, the readiness of its
external exposure, and keep the `phase` synchronized with actual state. If ready
replicas fall below desired replicas (crash, OOM, eviction, image pull failure),
or a routed gateway's external exposure loses readiness (for the Gateway API
adapter, the `Gateway` drops `Programmed` or its address is withdrawn), the
control plane SHALL set the `phase` to `Degraded` within one
health-reconciliation interval. When both the workload and (for routed gateways)
the external exposure return to Ready, the control plane SHALL set the `phase`
back to `Running`.

#### Scenario: Running gateway begins crash-looping

- GIVEN a Gateway with `phase` `Running`
- WHEN its gateway pod begins failing and ready replicas drop below desired
- THEN the control plane SHALL set the `phase` to `Degraded` within one
  health-reconciliation interval
- AND record the reason in `status`

#### Scenario: Running gateway loses route readiness

- GIVEN a routed Gateway with `phase` `Running`
- WHEN its external exposure loses readiness (for the Gateway API adapter, the
  per-tenant `Gateway` drops `Programmed=True` or its assigned address is
  withdrawn)
- THEN the control plane SHALL set the `phase` to `Degraded` within one
  health-reconciliation interval
- AND record the reason in `status`

#### Scenario: Degraded gateway recovers

- GIVEN a Gateway with `phase` `Degraded`
- WHEN its Deployment returns to ready replicas ≥ desired
- AND, for a routed gateway, its external exposure is observed Ready
- THEN the control plane SHALL set the `phase` back to `Running`

### Requirement: Provisioning Gate Keyed On Desired State

The control plane's provisioning gate SHALL prevent redundant re-provisioning
(re-applying manifests) **only when a Gateway is converged**
(`observed_generation == generation`). A `phase` of `Running`, `Provisioning`,
or `Degraded` SHALL NOT, by itself, suppress re-application.

When a Gateway's desired spec changes (its `generation` advances beyond
`observed_generation`), the control plane SHALL re-run provisioning regardless
of the current `phase`, and SHALL set `observed_generation` to the applied
`generation` upon success. If re-application fails, `observed_generation` SHALL
remain unchanged so the change is retried, and the `phase` SHALL be set per the
existing provisioning failure semantics.

The gate SHALL NOT, in any case, prevent `phase`/`status` updates that reflect
observed workload health: a `Running` Gateway SHALL still be able to transition
to `Degraded`, and a `Degraded` Gateway SHALL still be able to return to
`Running`, independently of convergence.

#### Scenario: Spec change to a Running gateway re-provisions

- GIVEN a converged Gateway with `phase` `Running`
  (`observed_generation == generation`)
- WHEN a client updates its desired spec (e.g. `image`, `route`,
  `server_dns_names`, `oidc`) and the API server advances `generation`
- THEN the control plane SHALL re-apply the gateway manifests despite the
  `Running` phase
- AND upon success SHALL set `observed_generation` to the applied `generation`

#### Scenario: Converged gateway is not re-applied

- GIVEN a converged Gateway with `phase` `Running` whose desired spec is
  unchanged (`observed_generation == generation`)
- WHEN a duplicate watch event, reconciliation, or health check occurs
- THEN the control plane SHALL skip re-applying the gateway manifests
- BUT it SHALL still update the `phase` to `Degraded` if the workload is
  observed unhealthy

#### Scenario: Degraded gateway re-provisions on spec change

- GIVEN a Gateway with `phase` `Degraded` that is not converged
  (`generation` advanced after a spec fix)
- WHEN the control plane processes the change
- THEN it SHALL re-apply the gateway manifests rather than skip on phase
- AND SHALL set `observed_generation` to the applied `generation` upon success

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

### Requirement: Connection Command Surfaced Only When Ready

The web console and CLI SHALL present a Gateway as ready-to-connect - and surface
its connection command as a usable "your gateway is ready" affordance - only when
the Gateway `phase` is `Running`. While the phase is `Pending`, `Provisioning`,
or `Degraded`, the console SHALL instead show a provisioning/attention state and
SHALL NOT represent the Gateway as ready to accept connections, even if a
`routeAddress` has already been published. This prevents handing users a command
that targets an endpoint that is not yet programmed and routable.

#### Scenario: Command withheld while provisioning

- GIVEN a routed Gateway with `phase` `Provisioning` whose `routeAddress` is
  already populated
- WHEN the console renders the Gateway detail
- THEN it SHALL show a provisioning state
- AND it SHALL NOT present the Gateway as ready to connect

#### Scenario: Command surfaced once running

- GIVEN a routed Gateway that transitions to `phase` `Running`
- WHEN the console next renders the Gateway detail
- THEN it SHALL present the connection command as ready to use
