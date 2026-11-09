# Managed Cluster On-Cluster Self-Healthcheck

**Status:** Draft
**Ticket:** HYPERSHELL-493
**Related:** `platform/managed-cluster-registration.spec.md` (registration/heartbeat path that carries the verdict, PLAT-CLUS-16), `platform/data-model.spec.md` (ManagedCluster `health` field, PLAT-DATA-27), `platform/control-plane-observability.spec.md` (health gauges, OBS-CPL-12), `platform/control-plane.spec.md` (continuous reconcilers, in-cluster credentials), `platform/openshell-gateway.spec.md` / `platform/openshell-gateway-health.spec.md` (gateway provisioning and readiness the canary probe reuses), `platform/openshell-gateway-namespace-gc.spec.md` (namespace labels the canary namespace stays clear of)

---

## Purpose

Each managed cluster owns proving that it can actually host a gateway, and it
proves this continuously from inside the cluster rather than being tested from
the fleet on every promotion. The control plane already runs on every managed
cluster and heartbeats to the hub (`managed-cluster-registration.spec.md`); this
specification adds a periodic self-validation loop on that same control plane
that computes a rich, per-check health verdict and reports it to the hub along
the existing heartbeat path.

This replaces fleet-side, per-cluster end-to-end validation of operational
readiness. Passive liveness (`last_seen_at`, PLAT-CLUS-05) proves only that the
control-plane process is alive and talking to the hub; it does not prove the
cluster can pull the gateway images, reach its OIDC issuer, reach its gateway
database server, or provision and serve a gateway. The self-check answers "can
this cluster host a gateway right now" as a per-cluster property, so the cost is
O(1) per cluster and independent of fleet size: N clusters each run O(1)
self-checks instead of a promotion gate running an O(N) end-to-end suite.

The verdict layers on top of liveness and does not replace it. `last_seen_at`
remains the liveness floor, the control-plane reconciler's `status` field remains
its own view of cluster state, and the self-check verdict is a separate,
spoke-reported readiness signal that the hub stores and surfaces without
re-deriving it.

---

## Health Verdict Shape

A verdict is a JSON object reported by the control plane and stored verbatim on
the owning `ManagedCluster` (PLAT-DATA-27):

```json
{
  "overall": "degraded",
  "observed_at": "2026-11-09T12:00:00Z",
  "duration_ms": 842,
  "conditions": [
    {
      "name": "images-pullable",
      "status": "pass",
      "last_transition_at": "2026-11-08T09:12:00Z",
      "message": "gateway, supervisor, and sandbox images pullable"
    },
    {
      "name": "oidc-issuer-reachable",
      "status": "fail",
      "last_transition_at": "2026-11-09T11:58:00Z",
      "message": "issuer https://keycloak.example/realms/hyp0 returned 503"
    },
    {
      "name": "canary-gateway",
      "status": "pass",
      "last_transition_at": "2026-11-09T06:00:00Z",
      "message": "canary reached Running and its route answered in 41s"
    }
  ]
}
```

- `overall` is the rollup enum (`healthy`, `degraded`, `failing`).
- `observed_at` and `duration_ms` describe the last self-check run.
- Each condition carries a stable `name`, a `status` enum
  (`pass`, `warn`, `fail`, `unknown`), a `last_transition_at` timestamp, and a
  human-readable `message`.

---

## Requirements

### Requirement: PLAT-HLTH-01 -- On-Cluster Self-Check Loop

The control plane SHALL run a continuous self-validation loop on each managed
cluster, as a continuous reconciler alongside the existing health, namespace GC,
and sandbox reconcilers (`control-plane.spec.md`). Each run SHALL execute the
check catalog (PLAT-HLTH-02) using the control plane's in-cluster credentials,
compute a verdict (PLAT-HLTH-03), and make the verdict available to the reporting
path (PLAT-HLTH-06) and the observability signals (PLAT-HLTH-08).

A self-check run SHALL NOT crash, block, or degrade the control plane's primary
reconciliation work. A check that errors SHALL be recorded as a condition with
status `fail` or `unknown` and SHALL NOT abort the remaining checks; every check
outcome SHALL be collected rather than silently swallowed on the first failure.

#### Scenario: PLAT-HLTH-01.01 -- Loop runs and produces a verdict

- GIVEN a control plane running on a managed cluster
- WHEN its self-check interval elapses
- THEN the control plane SHALL execute the check catalog
- AND it SHALL compute a verdict with per-check conditions and an overall rollup
- AND the verdict SHALL be available for reporting and observability

#### Scenario: PLAT-HLTH-01.02 -- A failing check does not abort the run

- GIVEN a self-check run in which the OIDC issuer is unreachable
- WHEN the run executes the remaining checks
- THEN the OIDC check SHALL be recorded as a `fail` condition
- AND every other check SHALL still run and record its own condition
- AND the control plane's primary reconciliation SHALL continue unaffected

### Requirement: PLAT-HLTH-02 -- Tiered Check Catalog

The self-check loop SHALL evaluate a tiered catalog of checks, cheapest first.
The catalog SHALL cover at least:

- **Continuous tier (frequent):** the reconciler/operator is healthy; the
  gateway namespace prerequisites and RBAC are present; the required images
  (gateway, supervisor, sandbox) are pullable; the OIDC issuer is reachable; the
  gateway-database admin secret is present and the database server is reachable
  (`openshell-gateway-database.spec.md`); the ingress/route controller is
  healthy.
- **Canary tier (slower cadence):** a real provision proof driven by the canary
  gateway probe (PLAT-HLTH-04).

Each catalog entry SHALL produce exactly one named condition in the verdict. The
set of check names SHALL be a bounded, stable enumeration so the names are safe
as low-cardinality observability attributes (PLAT-HLTH-08). Adding or removing a
cluster's capability or endpoint SHALL change which checks run through
configuration and the cluster's own state, not through an API-server code change.

#### Scenario: PLAT-HLTH-02.01 -- Continuous checks each yield a condition

- GIVEN a self-check run on a healthy cluster
- WHEN the continuous tier completes
- THEN the verdict SHALL contain one condition per continuous check
- AND each condition SHALL carry a name from the bounded check enumeration

#### Scenario: PLAT-HLTH-02.02 -- Database reachability is a distinct condition

- GIVEN the gateway-database admin secret is present but the database server is unreachable
- WHEN the self-check run evaluates the database check
- THEN that check SHALL produce a `fail` condition naming the database reachability failure
- AND the admin-secret-present check SHALL still report its own condition

### Requirement: PLAT-HLTH-03 -- Rich Health Verdict Model

The control plane SHALL compute a verdict with the shape defined in "Health
Verdict Shape": an `overall` rollup enum (`healthy`, `degraded`, `failing`), an
`observed_at` timestamp, a `duration_ms` for the run, and a `conditions` list.
Each condition SHALL carry a stable `name`, a `status` enum
(`pass`, `warn`, `fail`, `unknown`), a `last_transition_at` timestamp, and a
`message`.

The rollup SHALL be derived from the conditions: `healthy` when every condition
is `pass`; `failing` when any condition in the continuous tier reports `fail`;
`degraded` otherwise (for example a `warn` condition, or a `fail` confined to the
canary tier while the continuous tier passes). `last_transition_at` on a
condition SHALL change only when that condition's `status` changes, so a steady
state does not churn the timestamp.

#### Scenario: PLAT-HLTH-03.01 -- All checks pass rolls up to healthy

- GIVEN a self-check run in which every condition reports `pass`
- WHEN the rollup is computed
- THEN `overall` SHALL be `healthy`

#### Scenario: PLAT-HLTH-03.02 -- A continuous-tier failure rolls up to failing

- GIVEN a self-check run in which the images-pullable condition reports `fail`
- WHEN the rollup is computed
- THEN `overall` SHALL be `failing`

#### Scenario: PLAT-HLTH-03.03 -- last_transition_at is stable while status holds

- GIVEN a condition that reported `pass` on the previous run
- WHEN the next run also reports `pass` for that condition
- THEN its `last_transition_at` SHALL be unchanged

### Requirement: PLAT-HLTH-04 -- Canary Gateway Provision Probe

On its own configurable cadence (PLAT-HLTH-05), the control plane SHALL run a
canary provision probe that proves real gateway provisioning on the cluster: it
SHALL provision a throwaway canary gateway, verify it reaches a Running state and
that its route answers, and then delete it. The probe SHALL be self-cleaning: on
success and on failure it SHALL tear down everything it created, so a failed
probe does not leak a namespace or workload.

The canary SHALL be provisioned by the control plane with its in-cluster
credentials, NOT created as a `Gateway` API record. It therefore does not appear
in the fleet inventory, is not counted as a managed gateway, and does not require
the `CreateGateway` RPC that a control-plane identity is refused
(`managed-cluster-registration.spec.md`, PLAT-CLUS-11). The canary namespace
SHALL carry a distinct control-plane-owned canary label and SHALL NOT carry the
managed-gateway namespace prefix or the management labels that the gateway
namespace garbage collector keys on
(`openshell-gateway-namespace-gc.spec.md`), so the garbage collector never lists
or reaps a canary namespace mid-probe.

The probe outcome SHALL be recorded as the `canary-gateway` condition in the
verdict with the measured provision time in its `message`. The probe SHALL run
at most once per cluster per cadence, so one cluster's provision proof is O(1)
and never N-at-once from a gate.

#### Scenario: PLAT-HLTH-04.01 -- Canary proves provisioning and self-cleans

- GIVEN the canary cadence has elapsed on a healthy cluster
- WHEN the probe runs
- THEN it SHALL provision a throwaway canary gateway
- AND it SHALL verify the canary reaches Running and its route answers
- AND it SHALL delete the canary and its namespace afterward
- AND it SHALL record a `pass` `canary-gateway` condition with the provision time

#### Scenario: PLAT-HLTH-04.02 -- Failed canary cleans up and reports fail

- GIVEN a cluster where the canary never reaches Running within the probe timeout
- WHEN the probe runs
- THEN it SHALL record a `fail` `canary-gateway` condition describing the failure
- AND it SHALL still delete any canary resources it created

#### Scenario: PLAT-HLTH-04.03 -- Canary namespace is excluded from namespace GC

- GIVEN a canary provision probe is in progress
- WHEN the gateway namespace garbage collector sweeps
- THEN the canary namespace SHALL NOT be listed, annotated, or reaped
- AND the probe SHALL remain responsible for the canary namespace lifecycle

### Requirement: PLAT-HLTH-05 -- Configurable Cadences

The self-check loop cadence and the canary probe cadence SHALL be deployment
configuration, supplied through environment variables, not hard-coded constants.
Changing a cadence SHALL be a configuration change, not a code change. Each
cadence SHALL have a documented default, and the canary cadence SHALL default to
a slower interval than the continuous self-check cadence.

A zero or unset continuous cadence SHALL fall back to the documented default. A
zero or negative canary cadence SHALL disable the canary probe without disabling
the continuous tier, so an environment that cannot host a throwaway gateway still
reports the continuous verdict.

#### Scenario: PLAT-HLTH-05.01 -- Cadence is configured, not hard-coded

- GIVEN a control plane deployed with a self-check interval set in configuration
- WHEN the deployment configuration changes the interval
- THEN the loop SHALL adopt the new interval without an API-server or control-plane code change

#### Scenario: PLAT-HLTH-05.02 -- Canary disabled by non-positive cadence

- GIVEN a control plane whose canary cadence is configured to zero
- WHEN the self-check loop runs
- THEN the canary probe SHALL NOT run
- AND the continuous tier SHALL still run and report its verdict
- AND the `canary-gateway` condition SHALL report `unknown`

### Requirement: PLAT-HLTH-06 -- Verdict Reporting and Hub Storage

The control plane SHALL report its most recent verdict to the hub along the
existing registration/heartbeat path, carried as an optional `health` object in
the `POST /managed_clusters/registration` request body
(`managed-cluster-registration.spec.md`, PLAT-CLUS-16). The hub SHALL store the
reported verdict on the owning `ManagedCluster` record (PLAT-DATA-27) and SHALL
surface it in `GET /managed_clusters` and `GET /managed_clusters/{id}`.

Reporting SHALL NOT regress liveness: every heartbeat SHALL continue to update
`last_seen_at` (PLAT-CLUS-05) whether or not it carries a `health` object, and a
heartbeat that omits `health` SHALL preserve the stored verdict rather than
clearing it, the same backward-compatible rule capabilities follow
(PLAT-CLUS-07). The narrow registration response (`{ "cluster_id" }` only) SHALL
be unchanged.

#### Scenario: PLAT-HLTH-06.01 -- Verdict reported and surfaced

- GIVEN a registered control plane that computed a `degraded` verdict
- WHEN its next heartbeat carries the `health` object
- THEN the hub SHALL store the verdict on the `ManagedCluster` record
- AND `GET /managed_clusters` SHALL surface the stored verdict
- AND `last_seen_at` SHALL be updated to the call time

#### Scenario: PLAT-HLTH-06.02 -- Health-less heartbeat preserves the stored verdict

- GIVEN a `ManagedCluster` that already stores a verdict
- WHEN a heartbeat arrives with no `health` object
- THEN the stored verdict SHALL remain unchanged
- AND `last_seen_at` SHALL still be updated

### Requirement: PLAT-HLTH-07 -- Scale-Invariant Cost and Consumer Contract

The self-check cost SHALL be per-cluster O(1) and independent of fleet size: each
cluster evaluates its own catalog on its own cadence, and no component SHALL run
a per-cluster end-to-end suite to establish a cluster's operational health.

Consumers of cluster health, including the fleet promotion gate, dashboards, and
alerting, SHALL read the aggregated verdict from the hub (a single
`GET /managed_clusters`) rather than testing each cluster. The promotion gate
SHALL treat the stored per-cluster verdict as the source of operational
readiness instead of provisioning and tearing down a gateway per cluster on every
promotion.

#### Scenario: PLAT-HLTH-07.01 -- Gate reads the aggregated verdict

- GIVEN a fleet of registered clusters each reporting a verdict
- WHEN the promotion gate evaluates operational readiness
- THEN it SHALL read the verdicts through a single `GET /managed_clusters`
- AND it SHALL NOT run a per-cluster end-to-end suite to establish health

#### Scenario: PLAT-HLTH-07.02 -- Cost does not grow with the fleet

- GIVEN the fleet grows from N to 2N clusters
- WHEN each cluster runs its self-check loop
- THEN each cluster's self-check cost SHALL remain O(1)
- AND no component SHALL incur a per-cluster end-to-end cost that grows with the fleet

### Requirement: PLAT-HLTH-08 -- Health Observability Signals

The control plane SHALL emit the health verdict and its per-check conditions as
OpenTelemetry signals, integrating with `control-plane-observability.spec.md`
(OBS-CPL-12), so operators can alert on degraded or failing clusters. The signals
SHALL be low cardinality: labeled only by the `hypershell.cluster_id` and the
bounded check-name enumeration (PLAT-HLTH-02), never by raw resource identifiers,
and SHALL respect the telemetry privacy and cardinality rules of OBS-CPL-06.

When OTLP metrics are disabled (OBS-CPL-01), the health signals SHALL NOT be
exported, and the self-check loop and reporting path (PLAT-HLTH-06) SHALL still
run.

#### Scenario: PLAT-HLTH-08.01 -- Verdict exported as a low-cardinality signal

- GIVEN the OTel SDK is initialized with metrics export enabled
- AND the control plane computed a `failing` verdict with a `fail` OIDC condition
- WHEN the self-check run completes
- THEN the overall verdict SHALL be exported as a gauge labeled by `hypershell.cluster_id`
- AND the per-check condition status SHALL be exported labeled by the bounded check name
- AND no raw resource identifier SHALL appear as a label

#### Scenario: PLAT-HLTH-08.02 -- No export when metrics are disabled

- GIVEN OTLP metrics are disabled
- WHEN the self-check loop runs
- THEN the health signals SHALL NOT be exported
- AND the loop and the reporting path SHALL still run

---

## Design Decisions

| Decision | Rationale |
|----------|-----------|
| Each cluster self-checks and reports; the hub aggregates | "Can this cluster host a gateway right now" is a per-cluster property. Letting each cluster answer it continuously makes the cost O(1) per cluster and scale-invariant, instead of a promotion gate running an O(N) end-to-end suite that blows the gate timeout and churns N gateways per promotion. |
| Verdict layered on top of `last_seen_at`, not replacing it | Liveness (`last_seen_at`) proves the process is alive; the verdict proves operational readiness. Keeping them separate preserves the passive-liveness floor (PLAT-CLUS-05) and the reconciler-owned `status` field while adding the readiness signal the fleet actually needs. |
| Verdict carried on the existing heartbeat, no new endpoint | The registration heartbeat already runs on a loop, already authenticates the spoke, and already writes only the spoke's own record. Carrying the verdict there reuses that path and keeps the RBAC surface unchanged, consistent with the single-`/registration`-endpoint decision. |
| Health-less heartbeat preserves the stored verdict | A mixed-version rolling upgrade must not drop a cluster's verdict. The same backward-compatible rule capabilities follow (PLAT-CLUS-07) keeps an older control plane's heartbeat a no-op for health. |
| Canary provisioned in-cluster, not as a Gateway API record | A control-plane identity is refused `CreateGateway` (PLAT-CLUS-11), and a real fleet gateway would pollute inventory and counts. Provisioning a throwaway canary with in-cluster credentials proves provisioning without an API resource, and keeping it clear of the managed-gateway namespace labels keeps the namespace GC unchanged. |
| Canary cadence slower than the continuous tier, and disablable | A full provision proof is expensive, so it runs rarely and once per cluster rather than N-at-once from a gate. Environments that cannot host a throwaway gateway disable the canary while still reporting the continuous verdict. |
| Bounded check-name enumeration | Check names become observability attributes; a bounded enumeration keeps metric cardinality safe (OBS-CPL-06) and makes alerts stable across clusters. |

## Primary Basis

- `platform/managed-cluster-registration.spec.md` (heartbeat path, PLAT-CLUS-05, PLAT-CLUS-07, PLAT-CLUS-11)
- `platform/data-model.spec.md` (ManagedCluster `health` field, PLAT-DATA-27)
- `platform/control-plane-observability.spec.md` (OBS-CPL-01, OBS-CPL-06, OBS-CPL-12)
- `platform/control-plane.spec.md` (continuous reconcilers, in-cluster credentials)
- `platform/openshell-gateway-health.spec.md` (gateway readiness the canary reuses)
- `platform/openshell-gateway-namespace-gc.spec.md` (namespace labels the canary stays clear of)
- `platform/openshell-gateway-database.spec.md` (gateway database reachability check)
