# Managed Cluster On-Cluster Self-Healthcheck

**Status:** Draft
**Ticket:** HYPERSHELL-493
**Related:** `platform/managed-cluster-registration.spec.md` (registration/heartbeat path that carries the verdict, PLAT-CLUS-16), `platform/data-model.spec.md` (ManagedCluster `health` field, PLAT-DATA-27), `platform/control-plane-observability.spec.md` (health gauges, OBS-CPL-12), `platform/control-plane.spec.md` (continuous reconcilers, in-cluster credentials), `platform/openshell-gateway-database.spec.md` (gateway database reachability check)

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
cluster can pull the gateway images, reach its OIDC issuer, or reach its gateway
database server. The self-check answers "can this cluster host a gateway right
now" as a per-cluster property, so the cost is O(1) per cluster and independent
of fleet size: N clusters each run O(1) self-checks instead of a promotion gate
running an O(N) end-to-end suite.

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
  "overall": "failing",
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
      "name": "gateway-database-reachable",
      "status": "pass",
      "last_transition_at": "2026-11-08T09:12:00Z",
      "message": "gateway database server reachable"
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
path (PLAT-HLTH-05) and the observability signals (PLAT-HLTH-07).

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

### Requirement: PLAT-HLTH-02 -- Check Catalog

The self-check loop SHALL evaluate a catalog of checks, cheapest first. The
catalog SHALL cover at least: the reconciler/operator is healthy; the gateway
namespace prerequisites and RBAC are present; the required images (gateway,
supervisor, sandbox) are pullable; the OIDC issuer is reachable; the
gateway-database admin secret is present and the database server is reachable
(`openshell-gateway-database.spec.md`); the ingress/route controller is healthy.

Each catalog entry SHALL produce exactly one named condition in the verdict. The
set of check names SHALL be a bounded, stable enumeration so the names are safe
as low-cardinality observability attributes (PLAT-HLTH-07). Adding or removing a
cluster's capability or endpoint SHALL change which checks run through
configuration and the cluster's own state, not through an API-server code change.

#### Scenario: PLAT-HLTH-02.01 -- Each check yields a condition

- GIVEN a self-check run on a healthy cluster
- WHEN the run completes
- THEN the verdict SHALL contain one condition per check
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
is `pass`; `failing` when any condition reports `fail`; `degraded` otherwise (for
example a `warn` condition). The `warn` status is reserved for a check that
detects a degraded-but-serving condition; it rolls up to `degraded` rather than
`failing`. `last_transition_at` on a condition SHALL change only when that
condition's `status` changes, so a steady state does not churn the timestamp.

#### Scenario: PLAT-HLTH-03.01 -- All checks pass rolls up to healthy

- GIVEN a self-check run in which every condition reports `pass`
- WHEN the rollup is computed
- THEN `overall` SHALL be `healthy`

#### Scenario: PLAT-HLTH-03.02 -- A failing check rolls up to failing

- GIVEN a self-check run in which the images-pullable condition reports `fail`
- WHEN the rollup is computed
- THEN `overall` SHALL be `failing`

#### Scenario: PLAT-HLTH-03.03 -- last_transition_at is stable while status holds

- GIVEN a condition that reported `pass` on the previous run
- WHEN the next run also reports `pass` for that condition
- THEN its `last_transition_at` SHALL be unchanged

### Requirement: PLAT-HLTH-04 -- Configurable Cadence

The self-check loop cadence SHALL be deployment configuration, supplied through
an environment variable, not a hard-coded constant. Changing the cadence SHALL be
a configuration change, not a code change. The cadence SHALL have a documented
default, and a zero or unset cadence SHALL fall back to that default.

#### Scenario: PLAT-HLTH-04.01 -- Cadence is configured, not hard-coded

- GIVEN a control plane deployed with a self-check interval set in configuration
- WHEN the deployment configuration changes the interval
- THEN the loop SHALL adopt the new interval without an API-server or control-plane code change

### Requirement: PLAT-HLTH-05 -- Verdict Reporting and Hub Storage

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

#### Scenario: PLAT-HLTH-05.01 -- Verdict reported and surfaced

- GIVEN a registered control plane that computed a `degraded` verdict
- WHEN its next heartbeat carries the `health` object
- THEN the hub SHALL store the verdict on the `ManagedCluster` record
- AND `GET /managed_clusters` SHALL surface the stored verdict
- AND `last_seen_at` SHALL be updated to the call time

#### Scenario: PLAT-HLTH-05.02 -- Health-less heartbeat preserves the stored verdict

- GIVEN a `ManagedCluster` that already stores a verdict
- WHEN a heartbeat arrives with no `health` object
- THEN the stored verdict SHALL remain unchanged
- AND `last_seen_at` SHALL still be updated

### Requirement: PLAT-HLTH-06 -- Scale-Invariant Cost and Consumer Contract

The self-check cost SHALL be per-cluster O(1) and independent of fleet size: each
cluster evaluates its own catalog on its own cadence, and no component SHALL run
a per-cluster end-to-end suite to establish a cluster's operational health.

Consumers of cluster health, including the fleet promotion gate, dashboards, and
alerting, SHALL read the aggregated verdict from the hub (a single
`GET /managed_clusters`) rather than testing each cluster. The promotion gate
SHALL treat the stored per-cluster verdict as the source of operational
readiness instead of provisioning and tearing down a gateway per cluster on every
promotion.

#### Scenario: PLAT-HLTH-06.01 -- Gate reads the aggregated verdict

- GIVEN a fleet of registered clusters each reporting a verdict
- WHEN the promotion gate evaluates operational readiness
- THEN it SHALL read the verdicts through a single `GET /managed_clusters`
- AND it SHALL NOT run a per-cluster end-to-end suite to establish health

#### Scenario: PLAT-HLTH-06.02 -- Cost does not grow with the fleet

- GIVEN the fleet grows from N to 2N clusters
- WHEN each cluster runs its self-check loop
- THEN each cluster's self-check cost SHALL remain O(1)
- AND no component SHALL incur a per-cluster end-to-end cost that grows with the fleet

### Requirement: PLAT-HLTH-07 -- Health Observability Signals

The control plane SHALL emit the health verdict and its per-check conditions as
OpenTelemetry signals, integrating with `control-plane-observability.spec.md`
(OBS-CPL-12), so operators can alert on degraded or failing clusters. The signals
SHALL be low cardinality: the overall verdict gauge labeled only by the
`managed_cluster` attribute (= `HYPERSHELL_MANAGED_CLUSTER_NAME`), and the per-check
condition gauge labeled only by `managed_cluster` and the bounded check-name
enumeration (PLAT-HLTH-02, carried in the `check` attribute), never by raw resource
identifiers, and SHALL respect the telemetry privacy and cardinality rules of
OBS-CPL-06.

When OTLP metrics are disabled (OBS-CPL-01), the health signals SHALL NOT be
exported, and the self-check loop and reporting path (PLAT-HLTH-05) SHALL still
run.

#### Scenario: PLAT-HLTH-07.01 -- Verdict exported as a low-cardinality signal

- GIVEN the OTel SDK is initialized with metrics export enabled
- AND the control plane computed a `failing` verdict with a `fail` OIDC condition
- WHEN the self-check run completes
- THEN the overall verdict SHALL be exported as a gauge labeled by `managed_cluster` (= `HYPERSHELL_MANAGED_CLUSTER_NAME`)
- AND the per-check condition status SHALL be exported labeled by `managed_cluster` and the bounded check name (the `check` attribute)
- AND no raw resource identifier SHALL appear as a label

#### Scenario: PLAT-HLTH-07.02 -- No export when metrics are disabled

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
| Bounded check-name enumeration | Check names become observability attributes; a bounded enumeration keeps metric cardinality safe (OBS-CPL-06) and makes alerts stable across clusters. |

## Primary Basis

- `platform/managed-cluster-registration.spec.md` (heartbeat path, PLAT-CLUS-05, PLAT-CLUS-07, PLAT-CLUS-11)
- `platform/data-model.spec.md` (ManagedCluster `health` field, PLAT-DATA-27)
- `platform/control-plane-observability.spec.md` (OBS-CPL-01, OBS-CPL-06, OBS-CPL-12)
- `platform/control-plane.spec.md` (continuous reconcilers, in-cluster credentials)
- `platform/openshell-gateway-database.spec.md` (gateway database reachability check)
