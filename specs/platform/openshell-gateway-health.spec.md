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
requirements. Its provisioning gate and continuous-health behavior refine the
control-plane reconciliation contract
([`../standards/control-plane/reconciliation-contract.spec.md`](../standards/control-plane/reconciliation-contract.spec.md),
STD-RCON-02, STD-RCON-06); it refines that contract and does not weaken it.
Provisioning mechanics are defined in
[`openshell-gateway.spec.md`](./openshell-gateway.spec.md). The canonical
representation of the `phase`/`status` vocabulary used here - the allowed values
and the single shared source of truth every component draws from - is defined in
[`gateway-phase-vocabulary.spec.md`](./gateway-phase-vocabulary.spec.md).

## Domain Vocabulary

A Gateway carries three independently observable fields:

- **`phase`** - the lifecycle state managed by the control plane. Allowed
  values:
  - `Pending` - accepted, not yet acted on by the reconciler.
  - `Provisioning` - manifests are being applied and dependent workloads
    (database, gateway Deployment) are not yet Ready.
  - `Running` - the gateway is fully serving. For a gateway **with route
    configuration**, this requires **both** that the gateway Deployment is
    observed Ready (ready replicas ≥ desired replicas) **and** that its external
    exposure is observed Ready (for the Gateway API adapter: the parent Gateway
    API `Gateway` reports `Programmed=True` with a non-empty
    `.status.addresses`, this gateway's own `GRPCRoute` is `Accepted=True` /
    `ResolvedRefs=True` for that parent, and `routeAddress` has been published -
    see § Phase Reflects Workload and Route Readiness). For a gateway **without
    route configuration**, Deployment readiness alone is sufficient.
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
- **`gateway_version`** - the runtime version from the last successful gateway
  health response. The control plane manages this read-only field.

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

### Requirement: OSG-HLTH-01 -- Phase Reflects Workload and Route Readiness

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
§ Gateway Exposure Port, `OSG-ROUT-08`), so that the health decision is
independent of the concrete exposure backend. This spec defers to `OSG-ROUT-08`
for the full definition of "Ready" for the Gateway API adapter - the parent
Gateway's `Programmed` condition and address, GRPCRoute acceptance checked
across every matching `status.parents` entry with its `sectionName`, the
`observedGeneration` freshness check on each condition, and `routeAddress`
publication - rather than restating that definition here, so the two specs
cannot drift apart.

A parent Gateway reporting `Programmed=True` is not by itself sufficient: the
shared Gateway reports `Programmed=True` for every tenant as soon as any one
tenant's `GRPCRoute` attaches to it, independent of this gateway's own route.
The control plane SHALL NOT report a routed gateway's external exposure as
Ready on parent-Gateway readiness alone.

#### Scenario: OSG-HLTH-01.01 -- Deployment ready but route not yet programmed

- GIVEN a routed Gateway whose `openshell-gateway` Deployment has reached ready
  replicas ≥ desired
- AND its external exposure is not yet Ready (for the Gateway API adapter, the
  parent `Gateway` reports `Programmed=False` or has no assigned address)
- THEN the control plane SHALL keep the Gateway `phase` at `Provisioning`
- AND it SHALL NOT set the `phase` to `Running`

#### Scenario: OSG-HLTH-01.02 -- Deployment ready and route becomes fully accepted

- GIVEN a routed Gateway whose Deployment is Ready and whose phase is
  `Provisioning`
- WHEN its external exposure becomes Ready (for the Gateway API adapter, "Ready"
  per `OSG-ROUT-08`)
- THEN the control plane SHALL set the Gateway `phase` to `Running`

#### Scenario: OSG-HLTH-01.04 -- Route never becomes ready within the grace window

- GIVEN a routed Gateway whose Deployment is Ready
- AND whose external exposure does not become Ready within the route-readiness
  grace window (see routing spec § Gateway Exposure Configuration)
- THEN the control plane SHALL set the `phase` to `Degraded`
- AND it SHALL record the reason in `status` (e.g. "route not programmed after
  <window>")
- AND it SHALL continue to observe the exposure, so that if it later becomes
  Ready the phase returns to `Running`

#### Scenario: OSG-HLTH-01.05 -- Gateway pod crash-loops during provisioning

- GIVEN a Gateway whose `openshell-gateway` Deployment has been applied
- AND the gateway pod exits non-zero on startup (e.g. OIDC discovery fails) and
  enters CrashLoopBackOff
- WHEN the Deployment does not reach readiness within the provisioning
  readiness window
- THEN the control plane SHALL NOT set the `phase` to `Running`
- AND it SHALL set the `phase` to `Degraded`
- AND it SHALL record the reason in `status`

#### Scenario: OSG-HLTH-01.06 -- Gateway pod still starting during provisioning

- GIVEN a Gateway with `phase` `Provisioning` that has never been `Running`
- AND its `openshell-gateway` Deployment has been applied, but its pod is still
  pending, pulling its image, or starting, so the Deployment is not yet Ready
- WHEN the continuous health reconciler observes the gateway within the
  deployment-readiness grace window
- THEN the control plane SHALL keep the `phase` at `Provisioning`
- AND it SHALL NOT set the `phase` to `Degraded`
- AND if the Deployment is still not Ready once the grace window elapses, the
  control plane SHALL set the `phase` to `Degraded` and record the reason in
  `status` (e.g. "deployment not ready after <window>")

The deployment-readiness grace window is configured by
`GATEWAY_DEPLOYMENT_READY_TIMEOUT` (default `10m`). The provisioning path
normally sets `Degraded` itself when its own, shorter readiness window elapses.
The grace window stops the health reconciler from reporting `Degraded` before
then, and still moves the gateway to `Degraded` if the provisioning pass never
completes (e.g. the control plane restarted mid-provision). It applies only to
a gateway that has not yet reached `Running`: a `Running` gateway whose
Deployment loses readiness is moved to `Degraded` without a grace window.

#### Scenario: OSG-HLTH-01.07 -- Provisioning fails to apply

- GIVEN a Gateway being reconciled
- WHEN manifest templating or applying a resource returns an error
- THEN the control plane SHALL set the `phase` to `Failed`

#### Scenario: OSG-HLTH-01.08 -- Shared Gateway already programmed but this gateway's own route is not yet accepted

- GIVEN a routed Gateway, not opted in to a dedicated route, whose
  `openshell-gateway` Deployment has reached ready replicas ≥ desired
- AND the shared Gateway API `Gateway` already reports `Programmed=True` with a
  non-empty `.status.addresses` (established for a different tenant's
  `GRPCRoute`, or left over from before a Helm retry re-applied this gateway's
  own resources)
- AND this gateway's own `GRPCRoute` does not yet exist, or exists but its
  `status.parents` entry for the shared Gateway does not yet report both
  `Accepted=True` and `ResolvedRefs=True`
- THEN the control plane SHALL keep the Gateway `phase` at `Provisioning`
- AND it SHALL NOT set the `phase` to `Running`

#### Scenario: OSG-HLTH-01.09 -- Route accepted but address not yet published

- GIVEN a routed Gateway whose Deployment is Ready, whose parent `Gateway`
  reports `Programmed=True` with a non-empty `.status.addresses`, and whose own
  `GRPCRoute` reports `Accepted=True` and `ResolvedRefs=True` for that parent
- AND the Gateway resource's `routeAddress` field is still empty
- THEN the control plane SHALL keep the Gateway `phase` at `Provisioning`
- AND it SHALL NOT set the `phase` to `Running`

### Requirement: OSG-HLTH-02 -- Continuous Gateway Health Reconciliation

After a Gateway reaches `Running`, the control plane SHALL continue to observe
both the health of its Deployment and, for routed gateways, the readiness of its
external exposure, and keep the `phase` synchronized with actual state. If ready
replicas fall below desired replicas (crash, OOM, eviction, image pull failure),
or a routed gateway's external exposure loses readiness (for the Gateway API
adapter: the parent `Gateway` drops `Programmed` or its address is withdrawn, or
this gateway's own `GRPCRoute` is no longer `Accepted=True`/`ResolvedRefs=True`
for that parent), the control plane SHALL set the `phase` to `Degraded` within
one health-reconciliation interval. When both the workload and (for routed
gateways) the external exposure return to Ready, the control plane SHALL set
the `phase` back to `Running`.

#### Scenario: OSG-HLTH-02.01 -- Running gateway begins crash-looping

- GIVEN a Gateway with `phase` `Running`
- WHEN its gateway pod begins failing and ready replicas drop below desired
- THEN the control plane SHALL set the `phase` to `Degraded` within one
  health-reconciliation interval
- AND record the reason in `status`

#### Scenario: OSG-HLTH-02.02 -- Running gateway loses route readiness

- GIVEN a routed Gateway with `phase` `Running`
- WHEN its external exposure loses readiness (for the Gateway API adapter, the
  parent `Gateway` drops `Programmed=True` or its assigned address is
  withdrawn, or this gateway's own `GRPCRoute` is no longer
  `Accepted=True`/`ResolvedRefs=True` for that parent)
- THEN the control plane SHALL set the `phase` to `Degraded` within one
  health-reconciliation interval
- AND record the reason in `status`

#### Scenario: OSG-HLTH-02.03 -- Running Vault-backed gateway loses its credential backend

- GIVEN a `Running` Vault-backed Gateway (the default credential driver)
- WHEN its per-cluster provider Vault becomes unreachable
- THEN the control plane SHALL set the `phase` to `Degraded` within one
  health-reconciliation interval
- AND record a reason in `status` that names Vault
- AND the gateway SHALL return to `Running` once Vault is reachable again, without
  recreation (see [`openshell-gateway-credentials.spec.md`](./openshell-gateway-credentials.spec.md))

#### Scenario: OSG-HLTH-02.04 -- Degraded gateway recovers

- GIVEN a Gateway with `phase` `Degraded`
- WHEN its Deployment returns to ready replicas ≥ desired
- AND, for a routed gateway, its external exposure is observed Ready
- THEN the control plane SHALL set the `phase` back to `Running`

### Requirement: OSG-HLTH-03 -- Provisioning Gate Keyed On Desired State

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

A reconcile pass SHALL apply one snapshot of the Gateway's desired state and
SHALL acknowledge only the `generation` of that snapshot (PLAT-DATA-06). The
control plane resolves and persists its own generated configuration - for
example the gateway's OIDC configuration (OSG-OIDC-05) - before it captures that
snapshot and the `generation` it applies, so a newly created Gateway converges in
a single provisioning pass rather than leaving a newer `generation` behind that
triggers a second full apply and resets `phase` to `Provisioning` just after the
Gateway first reaches `Running`. When a newer desired `generation` arrives while
a pass is applying, the control plane SHALL acknowledge only the `generation` it
applied and SHALL leave the newer `generation` pending for its own
reconciliation; reading the latest `generation` at the end of a pass is not proof
that its configuration was applied.

The convergence gate SHALL be evaluated against the authoritative stored
`generation` and `observed_generation`, not against a watch event's payload, so a
duplicate or stale queued event for a converged Gateway SHALL NOT cause another
full manifest re-apply (Helm upgrade). While the full re-apply is gated, the
control plane SHALL still run, against authoritative state, its lightweight drift
reconciliations - existing Keycloak client attributes
([`openshell-gateway-keycloak.spec.md`](./openshell-gateway-keycloak.spec.md))
and the gateway's destination-CA trust bundle - together with its continuous
health and runtime-version observation, so external desired state and recoverable
drift converge without a full Helm upgrade. A destination-CA trust change SHALL
still take effect for a converged Gateway without a `generation` advance. A
Gateway whose provisioning did not complete is not converged
(`observed_generation < generation`) and SHALL still re-apply through the normal
gate to finish.

Because the convergence gate suppresses the provisioning body once a Gateway is
converged, the continuous health reconciler - not the provisioning body - owns
finalizing the user-facing provisioning conditions after convergence. When the
health reconciler promotes a Gateway to `phase` `Running` with a healthy status,
it SHALL also set every provisioning condition to `Complete`, so that a
`Running` Gateway never reports an unfinished `GatewayHealthy` (or earlier)
condition even though its `observed_generation` was acknowledged at manifest
apply, before workload and route readiness were observed.

The gate SHALL NOT, in any case, prevent `phase`/`status` updates that reflect
observed workload health: a `Running` Gateway SHALL still be able to transition
to `Degraded`, and a `Degraded` Gateway SHALL still be able to return to
`Running`, independently of convergence.

#### Scenario: OSG-HLTH-03.01 -- Spec change to a Running gateway re-provisions

- GIVEN a converged Gateway with `phase` `Running`
  (`observed_generation == generation`)
- WHEN the desired spec changes (the control plane writes a platform-managed
  field such as `image`, `route`, `server_dns_names`, or `oidc`, or a user
  renames the gateway) and the API server advances `generation`
- THEN the control plane SHALL re-apply the gateway manifests despite the
  `Running` phase
- AND upon success SHALL set `observed_generation` to the applied `generation`

#### Scenario: OSG-HLTH-03.02 -- Converged gateway is not re-applied

- GIVEN a converged Gateway with `phase` `Running` whose desired spec is
  unchanged (`observed_generation == generation`)
- WHEN a duplicate watch event, reconciliation, or health check occurs
- THEN the control plane SHALL skip re-applying the gateway manifests
- BUT it SHALL still update the `phase` to `Degraded` if the workload is
  observed unhealthy

#### Scenario: OSG-HLTH-03.03 -- Degraded gateway re-provisions on spec change

- GIVEN a Gateway with `phase` `Degraded` that is not converged
  (`generation` advanced after a spec fix)
- WHEN the control plane processes the change
- THEN it SHALL re-apply the gateway manifests rather than skip on phase
- AND SHALL set `observed_generation` to the applied `generation` upon success

#### Scenario: OSG-HLTH-03.04 -- New gateway with generated OIDC config converges in one pass

- GIVEN a newly created Gateway whose OIDC configuration the control plane
  generates during provisioning
- WHEN the first provisioning pass resolves and persists that OIDC configuration
  before it captures the snapshot and the `generation` it applies
- THEN the pass SHALL acknowledge the exact `generation` whose complete desired
  configuration it applied
- AND the control plane SHALL NOT run a second full apply that resets the `phase`
  to `Provisioning` after the Gateway first reaches `Running`

#### Scenario: OSG-HLTH-03.05 -- Stale queued event after convergence takes the lightweight path

- GIVEN a converged Gateway (`observed_generation == generation`)
- WHEN a duplicate or stale queued watch event is processed
- THEN the control plane SHALL evaluate the gate against the stored `generation`
  and `observed_generation`, not against the event payload
- AND it SHALL skip the full manifest re-apply
- AND it SHALL still run the lightweight Keycloak client, destination-CA trust,
  health, and runtime-version reconciliations

#### Scenario: OSG-HLTH-03.06 -- Destination-CA trust change applies to a converged gateway

- GIVEN a converged Gateway with `phase` `Running`
- WHEN its destination-CA trust bundle drifts from the desired bundle
- THEN the control plane SHALL update the destination-CA trust for the Gateway
  even though a full manifest re-apply is gated
- AND it SHALL NOT advance `generation` or require a spec change to do so

#### Scenario: OSG-HLTH-03.07 -- Newer generation arrives during a provisioning pass

- GIVEN a provisioning pass applying the snapshot at `generation` N
- WHEN a desired-spec change advances `generation` to N+1 before the pass
  completes
- THEN the pass SHALL set `observed_generation` to N only
- AND `generation` N+1 SHALL remain pending for its own reconciliation pass

### Requirement: OSG-HLTH-04 -- Gateway Runtime Version Is Reconciled

The control plane SHALL read the runtime version from the gateway `/health`
endpoint after the gateway Deployment is Ready. The control plane SHALL store a
valid, non-empty value in the read-only `gateway_version` field. It SHALL
NOT derive this value from an image reference or a release record.

The health reconciler SHALL run an initial list when it starts and SHALL repeat
the list at the health reconciliation interval. Each pass SHALL compare the
observed version with the stored value. It SHALL update only the fields that it
owns and only when a value changes. The request to the gateway health endpoint
SHALL have a bounded timeout and SHALL NOT follow redirects. Before it builds
the health URL, the reconciler SHALL validate the namespace as a Kubernetes
DNS label. The reconciler
SHALL accept a valid version from the healthy HTTP 200 response or the unhealthy
HTTP 503 response because the gateway includes its runtime version in both.

The API SHALL restrict the `SetGatewayVersion` RPC to control-plane identities:
the registered managed cluster the gateway is assigned to
(`managed-cluster-registration.spec.md`, "Control-Plane Identity"), or an
allowlisted `RBAC_SERVICE_ACCOUNTS` account that has not registered. Gateway owner
and creator roles SHALL NOT grant access to this RPC.

The API SHALL provide a dedicated atomic write for `gateway_version`. A general
Gateway replacement SHALL NOT write this field. Thus, an unrelated Gateway
update cannot overwrite the last version that the health reconciler stored. The
health reconciler SHALL use a bounded worker count for different Gateways. It
SHALL keep health and version work for one Gateway in one serial pass. Health
and console work SHALL run before the version request. Thus, a slow or failed
version request does not block the health update for that Gateway or work for
other Gateways.

If the request fails or returns an invalid version, the reconciler SHALL keep
the last verified value. It SHALL record the error in its log and retry during
the next pass. A version observation failure SHALL NOT prevent a valid `phase`
or `status` update.

The dedicated `openshell-gateway-health` Service SHALL expose the health
endpoint on an internal port. The health reconciler SHALL be the only writer
for this Service. Provisioning manifests SHALL NOT contain a second definition
of the resource. Access to the health port is permitted by the default
allow-all network posture (OVN-Kubernetes); the control plane does not create a
NetworkPolicy for this endpoint (see
[`openshell-gateway-helm-adoption.spec.md`](./openshell-gateway-helm-adoption.spec.md),
NetworkPolicy Decision: Do Not Install). Before the first version request, the
health reconciler SHALL reconcile this Service with update-or-create operations.
After success, it SHALL repeat access checks every five minutes. A failed
version observation SHALL force an access check on the next health pass. Each
Service operation SHALL have its own three-second timeout, including conflict
retries. This reconciliation SHALL repair existing gateways after a control
plane upgrade and SHALL repair later drift or deletion.

#### Scenario: OSG-HLTH-04.01 -- Runtime version is observed for the first time

- GIVEN a Gateway with a Ready Deployment and no `gateway_version`
- WHEN its `/health` endpoint returns a non-empty version
- THEN the control plane SHALL store that version in `gateway_version`
- AND it SHALL NOT update an unchanged `phase` or `status`

#### Scenario: OSG-HLTH-04.02 -- Runtime version changes

- GIVEN a Gateway with a stored `gateway_version`
- WHEN its Ready Deployment reports a different version from `/health`
- THEN the control plane SHALL replace `gateway_version` with the observed value

#### Scenario: OSG-HLTH-04.03 -- CI installs the matching CLI

- GIVEN a ready gateway with a reported `gateway_version`
- WHEN the end-to-end test runs in CI
- THEN the test SHALL run the console-recommended installation command
- AND it SHALL remove the first `-` and all following text from the reported
  version before adding a leading `v` if needed
- AND it SHALL compare the complete installed CLI version with that value
- AND it SHALL fail if the API does not report a version within a bounded wait

#### Scenario: OSG-HLTH-04.04 -- Runtime version observation fails

- GIVEN a Gateway with a stored `gateway_version`
- WHEN its `/health` request fails, times out, or returns an invalid value
- THEN the control plane SHALL keep the stored `gateway_version`
- AND it SHALL retry the observation during the next reconciliation pass

#### Scenario: OSG-HLTH-04.05 -- One runtime endpoint is slow

- GIVEN one Gateway runtime does not answer before the request timeout
- AND another Gateway is ready for reconciliation
- WHEN the health reconciler processes both Gateways
- THEN the slow Gateway SHALL NOT block the other Gateway
- AND no more than the configured number of workers SHALL run at the same time

#### Scenario: OSG-HLTH-04.06 -- Reconciler starts after a gateway is ready

- GIVEN a Gateway that was Ready before the health reconciler started
- WHEN the reconciler starts
- THEN its initial list SHALL include the Gateway
- AND the reconciler SHALL observe its runtime version without waiting for an
  event

### Requirement: OSG-HLTH-05 -- Console Reflects Recoverable States

The web console SHALL treat recoverable, non-terminal phases - including
`Pending`, `Provisioning`, and `Degraded` - as states that require continued
status polling, so that a Gateway recovering from `Degraded` to `Running`, or
failing from `Running` to `Degraded`, is reflected without a manual refresh.

#### Scenario: OSG-HLTH-05.01 -- Degraded gateway keeps polling

- GIVEN a Gateway displayed in the console with `phase` `Degraded`
- WHEN the console evaluates whether to poll for status
- THEN it SHALL continue polling until the Gateway settles into a terminal
  healthy (`Running`) or non-recoverable (`Failed`) state

### Requirement: OSG-HLTH-06 -- Connection Command Surfaced Only When Ready

The web console and CLI SHALL present a Gateway as ready-to-connect - and surface
its connection command as a usable "your gateway is ready" affordance - only when
the Gateway `phase` is `Running`. While the phase is `Pending`, `Provisioning`,
or `Degraded`, the console SHALL instead show a provisioning/attention state and
SHALL NOT represent the Gateway as ready to accept connections, even if a
`routeAddress` has already been published. This prevents handing users a command
that targets an endpoint that is not yet programmed and routable.

#### Scenario: OSG-HLTH-06.01 -- Command withheld while provisioning

- GIVEN a routed Gateway with `phase` `Provisioning` whose `routeAddress` is
  already populated
- WHEN the console renders the Gateway detail
- THEN it SHALL show a provisioning state
- AND it SHALL NOT present the Gateway as ready to connect

#### Scenario: OSG-HLTH-06.02 -- Command surfaced once running

- GIVEN a routed Gateway that transitions to `phase` `Running`
- WHEN the console next renders the Gateway detail
- THEN it SHALL present the connection command as ready to use

### Requirement: OSG-HLTH-07 -- Dependency Prerequisite State Is Visible And Recoverable

A Gateway waiting for a required API/resource SHALL expose a dependency condition
through its phase, status, and provisioning conditions. The condition SHALL name
the required group/version/resource and distinguish unknown/unavailable
(retryable) from confirmed absent (a prerequisite to install), following
CTRL-CORE-19.

The controller SHALL report process health separately from Gateway dependency
status. Its reconciliation machinery SHALL remain active through dependency
outages and converge the Gateway after the prerequisite becomes available, in
line with OSG-HLTH-02 and OSG-HLTH-03. Gateway provisioning SHALL complete after
its required resources are reconciled and its workload and configured route meet
the readiness criteria in this specification.

#### Scenario: OSG-HLTH-07.01 -- Unavailable API produces a retryable condition

- GIVEN a Gateway's required ingress API is unknown/unavailable
- WHEN the control plane reconciles the Gateway
- THEN it SHALL expose a waiting/retryable condition naming the API and retry with bounded backoff
- AND the controller SHALL continue reporting process health and running independent reconciliation work

#### Scenario: OSG-HLTH-07.02 -- Installing a prerequisite restores readiness

- GIVEN successful discovery confirms a Gateway prerequisite is absent
- WHEN the control plane reconciles the Gateway
- THEN it SHALL set a Degraded/prerequisite condition identifying the required resource
- AND after the prerequisite is installed, the running controller SHALL reconcile the Gateway to Running once its workload and configured route meet readiness criteria
