# E2E Per-ManagedCluster Smoke Matrix

**Date:** 2026-10-06
**Status:** Draft
**Related:** `e2e-testing.spec.md` (the `E2E_MODE=short` self-contained slice this matrix reuses, the driver contract, the single-cluster suite `tests/e2e/e2e-openshell.sh`, and the performance harness pattern of driving repeated suite runs);
             `managed-cluster-registration.spec.md` (ManagedCluster fields, `oidc_subject` as the "registered" marker, the `last_seen_at` staleness table, `/managed_clusters`, gateway `cluster_id` placement);
             `gateway-managed-cluster-attribution.spec.md` (per-cluster gateway/sandbox metric attribution);
             `gateway-release-rollout.spec.md` (release promotion area 13, distinct from the environment promotion this gate guards)

---

## Purpose

HyperShell manages a fleet of ManagedClusters, but the functional e2e suite
(`tests/e2e/e2e-openshell.sh`) provisions and exercises a gateway on exactly one
cluster per run (`E2E_CLUSTER_ID`, resolved from a single
`E2E_SEED_CLUSTER_NAME`). When a change is promoted from dev to prod, running the
suite once validates one cluster and says nothing about the others; a cluster
that is misconfigured, out of capacity, running a stale control plane, or
unreachable is not discovered until a user hits it.

This specification defines a **per-cluster smoke matrix**: a driver that
enumerates every registered ManagedCluster and runs the short e2e slice against
each one, then reports a per-cluster pass/fail/skip matrix and a single
gate verdict. It is intended as a dev-to-prod promotion gate - the exact role the
short mode is already specified to serve ("a post-rollout promotion gate ...
safe to run repeatedly against a live or shared environment", `e2e-testing.spec.md`).

The matrix adds **no new test assertions**. It reuses `E2E_MODE=short` and the
existing single-cluster suite unchanged, driving it once per cluster, the same
way the performance harness drives repeated suite runs. New behavior is confined
to cluster enumeration, per-cluster targeting and isolation, aggregated
reporting, and the gate verdict.

### Scope

In scope: a new matrix entry point (`tests/e2e/e2e-smoke-all.sh`), cluster
enumeration and selection, per-cluster targeting and process isolation, smoke
depth, unhealthy-cluster handling, aggregated matrix reporting, and the promotion
gate contract. Out of scope: the test assertions themselves (owned by the short
slice in `e2e-testing.spec.md`), the dev-to-prod environment topology and the
`/api/promotion` pipeline (owned by the gitops repo, see
`packages/fleet-dashboard-ui/src/domain/promotion.ts`), and per-gateway release
promotion (area 13).

---

## Requirements

### Requirement: Smoke Matrix Entry Point and Suite Reuse

The system SHALL provide `tests/e2e/e2e-smoke-all.sh`: a driver that enumerates
registered ManagedClusters and runs the short e2e slice against each. It SHALL
source `tests/e2e/lib.sh`, select and validate the infra driver exactly as
`e2e-openshell.sh` does, and for each selected cluster invoke the existing
single-cluster suite with `E2E_MODE=short` as a **separate child process**. It
SHALL NOT duplicate any area's assertion code; the per-cluster checks are the
short-tagged steps the single-cluster suite already defines. The matrix driver
itself requires only the driver functions needed to list clusters
(`discover_api_host`, `acquire_oidc_token`, `api_curl`); each child run performs
its own driver selection and `REQUIRED_FUNCTIONS` check.

The driver SHALL reject `E2E_MODE` values other than `short` for the per-cluster
runs: the matrix is a smoke gate, not a full or performance run.

#### Scenario: Matrix runs the short slice per cluster

- GIVEN a fleet with N registered ManagedClusters
- WHEN `bash tests/e2e/e2e-smoke-all.sh` runs
- THEN the suite SHALL run the `short`-tagged slice once per selected cluster, each as its own process with its own gateway
- AND no per-cluster check SHALL be a second copy of an assertion already in `e2e-openshell.sh`

#### Scenario: Only short mode is accepted

- GIVEN `E2E_MODE=long` or `E2E_MODE=perf`
- WHEN the matrix driver starts
- THEN it SHALL exit non-zero stating the matrix runs the short slice only

### Requirement: Cluster Enumeration and Selection

The matrix SHALL enumerate clusters by listing
`GET /api/hypershell/v1/managed_clusters` with the admin token and selecting the
**registered** records - those with a non-empty `oidc_subject`, the same marker
`e2e_json_registered_cluster_id` uses for single-cluster seed discovery. By
default it SHALL select every registered cluster.

The selection SHALL be narrowable and widenable:

- `E2E_SMOKE_CLUSTERS` - a comma-separated allowlist of ManagedCluster names;
  when set, only those clusters run (and a named cluster that is absent or
  unregistered SHALL be treated per
  [Unhealthy and Unreachable Cluster Handling](#requirement-unhealthy-and-unreachable-cluster-handling)).
- `E2E_SMOKE_SKIP_CLUSTERS` - a comma-separated denylist of names to omit,
  recorded as explicit skips.

Each selected cluster SHALL have its health derived from `last_seen_at` using the
staleness table in `managed-cluster-registration.spec.md` (Healthy `< 5 min`,
Unknown `5-30 min`, Offline `> 30 min`); the healthy window is overridable via
`E2E_SMOKE_HEALTHY_WINDOW` (default `5m`). The matrix SHALL print the selected
set, each cluster's derived health, and the reason any cluster was excluded,
before running anything.

#### Scenario: Default selects all registered clusters

- GIVEN a fleet with three registered clusters and one record with an empty `oidc_subject`
- WHEN the matrix runs with no selection variables set
- THEN it SHALL run against the three registered clusters
- AND it SHALL skip the unregistered record, naming the empty `oidc_subject` as the reason

#### Scenario: Allowlist narrows the matrix

- GIVEN `E2E_SMOKE_CLUSTERS=prod-us-east,prod-eu-west`
- WHEN the matrix runs
- THEN it SHALL run only those two clusters and SHALL NOT run any other registered cluster

### Requirement: Per-Cluster Targeting and Isolation

Each per-cluster run SHALL be pinned to its cluster and SHALL NOT interfere with
any other cluster's run.

- **API placement.** The run SHALL place its gateway on the target cluster by
  resolving that cluster's `cluster_id` (by name, as the seed-discovery helper
  does) and passing it as the child's `E2E_CLUSTER_ID` / `E2E_SEED_CLUSTER_NAME`,
  so the gateway create carries that cluster's id (`cluster_reference.go`
  requires a registered `cluster_id`).
- **Unique gateway name.** Each run SHALL use a gateway name unique to the
  cluster and the matrix run (for example `${E2E_SMOKE_GATEWAY_PREFIX}-<cluster>-<runid>`,
  default prefix `smoke`), so concurrent or repeated runs never collide and
  short mode's own-and-teardown guarantee holds per cluster.
- **Process isolation.** Each run SHALL be a separate process, so the flat,
  process-global counters (`E2E_PASS`/`E2E_FAIL`/`E2E_TESTS`) and the gateway
  lifecycle reset cleanly between clusters; a failure or hard `exit 1` in one
  cluster's run SHALL NOT abort the others.

The matrix SHALL run clusters sequentially by default and SHALL support bounded
parallelism via `E2E_SMOKE_CONCURRENCY` (default `1`). Because short mode owns and
tears down its own uniquely-named gateway, parallel per-cluster runs are safe.

#### Scenario: Each run targets its own cluster and gateway

- GIVEN two selected clusters `a` and `b`
- WHEN the matrix runs them
- THEN the run for `a` SHALL create its gateway with `cluster_id` of `a` and a name scoped to `a`, and the run for `b` likewise for `b`
- AND neither run SHALL leave a gateway behind, on success or failure

#### Scenario: One cluster's failure does not abort the matrix

- GIVEN three selected clusters where the second fails its smoke run
- WHEN the matrix runs
- THEN the first, second, and third SHALL each run to completion and be reported
- AND the second's failure SHALL NOT prevent the third from running

### Requirement: Smoke Depth

The per-cluster run SHALL exercise each cluster through the product's own
interfaces as its baseline: provision a gateway on that cluster via the HyperShell
API and wait for `Running`, connect the openshell CLI to the gateway's gRPC
endpoint over trusted TLS, run one sandbox create -> ready -> delete cycle, and
delete the gateway - the short slice's product-interface steps, which need only
API credentials and gateway DNS, not kube access to the target cluster.

The short slice's infrastructure-level steps that require kube access to the
target cluster (area 3 deployment/service verification, area 11 delete-driven
namespace-GC assertion) SHALL run when a kube context for that cluster is
available to the matrix and SHALL degrade to a recorded skip (not a failure) when
it is not. The matrix SHALL accept a per-cluster kube context mapping
(`E2E_SMOKE_KUBECONTEXT_<name>`, or a context derived from the cluster's
`KubeconfigSecret` where the runner can read it); absent a mapping, the baseline
product-interface smoke still proves the cluster can provision and run a working
gateway.

#### Scenario: Product-interface smoke needs no per-cluster kube access

- GIVEN a selected cluster reachable only through the central API and its gateway DNS
- WHEN the matrix runs its smoke
- THEN it SHALL provision a gateway on that cluster, reach `Running`, connect the CLI, run a sandbox lifecycle, and delete the gateway
- AND the kube-level infra-verification and GC-assertion steps SHALL be recorded as skips, not failures

#### Scenario: Kube context enables the full short slice

- GIVEN a selected cluster with a kube context supplied to the matrix
- WHEN the matrix runs its smoke
- THEN the short slice's infra-verification and delete-driven GC assertions SHALL run against that cluster in addition to the product-interface steps

### Requirement: Unhealthy and Unreachable Cluster Handling

A promotion gate SHALL fail closed. A selected cluster that is unregistered (no
`oidc_subject`), stale (its `last_seen_at` is beyond the healthy window), or
unreachable (its smoke run cannot reach the API placement or the gateway) SHALL
be reported as a **failure** of the matrix by default, because a fleet with a
down cluster is not safe to promote onto.

An operator MAY downgrade specific clusters to a skip with
`E2E_SMOKE_SKIP_CLUSTERS` (explicit, named) or flip the global default with
`E2E_SMOKE_SKIP_UNHEALTHY=1` (skip any cluster that is unregistered or stale
rather than fail). A skip SHALL be reported distinctly from a pass and from a
failure, and SHALL never be counted as a pass.

#### Scenario: Stale cluster fails the gate by default

- GIVEN a selected cluster whose `last_seen_at` is older than the healthy window
- WHEN the matrix runs with default settings
- THEN that cluster SHALL be reported as a failure and the matrix verdict SHALL be non-zero

#### Scenario: Explicit skip downgrades a cluster

- GIVEN `E2E_SMOKE_SKIP_CLUSTERS` names a cluster known to be offline for maintenance
- WHEN the matrix runs
- THEN that cluster SHALL be recorded as a skip, not a failure, and SHALL not by itself fail the gate
- AND the skip SHALL be distinct from a pass in the report

### Requirement: Aggregated Matrix Reporting and Verdict

The matrix SHALL print a per-cluster result table - one row per selected cluster
with its name, derived health, result (pass / fail / skip), and the child run's
pass/fail tally - followed by an overall verdict line. The report SHALL make the
three outcomes unambiguous and SHALL name the clusters that failed.

The matrix SHALL exit non-zero if any selected cluster failed (including the
fail-closed unhealthy cases above), and zero only when every selected cluster
passed or was explicitly skipped. The verdict SHALL be the gate signal a
promotion pipeline consumes.

#### Scenario: Matrix verdict reflects the worst cluster

- GIVEN four selected clusters: three pass and one fails
- WHEN the matrix finishes
- THEN the report SHALL show three passes and one failure, naming the failed cluster
- AND the matrix SHALL exit non-zero

#### Scenario: All-pass yields a clean gate

- GIVEN every selected cluster passes its smoke run
- WHEN the matrix finishes
- THEN the report SHALL show all passes and the matrix SHALL exit zero

#### Scenario: Partial run is not reported as success

- GIVEN the matrix aborts partway (for example the initial `/managed_clusters` list fails)
- WHEN it exits
- THEN it SHALL exit non-zero and SHALL NOT report a clean gate, mirroring the single-cluster suite's abort handling

### Requirement: Promotion Gate Contract

A dev-to-prod promotion SHALL run the per-cluster smoke matrix against the target
environment's fleet and SHALL block the promotion when the matrix verdict is
non-zero. The matrix is the mechanism; the promotion pipeline is the consumer.
The dev-to-prod promotion CI already exists outside this repository (gitops), as
does the environment topology (which clusters belong to which environment, gate
ordering); neither is defined or duplicated here. This spec owns only the matrix
the external pipeline invokes. The matrix discovers the fleet from the live
`/managed_clusters` registry of the environment it is pointed at, so the external
pipeline needs no cluster list of its own.

This requirement records the contract so the existing external promotion gate
invokes the matrix in place of running the single-cluster suite once; it does not
add a CI job to this repository.

#### Scenario: Promotion blocks on any cluster failure

- GIVEN a promotion pipeline configured to run the matrix against the target fleet
- WHEN any selected cluster fails its smoke run
- THEN the matrix SHALL exit non-zero and the promotion SHALL be blocked

## Environment Variables

These extend the table in `e2e-testing.spec.md`. The matrix reuses the admin
OIDC variables, `E2E_INFRA_DRIVER`, and the timeout variables unchanged; each
child run reuses the full single-cluster variable set.

| Env Var | Default | Description |
|---------|---------|-------------|
| `E2E_SMOKE_CLUSTERS` | (unset = all registered) | Comma-separated allowlist of ManagedCluster names to smoke |
| `E2E_SMOKE_SKIP_CLUSTERS` | (unset) | Comma-separated denylist of names to skip (recorded as skips, not failures) |
| `E2E_SMOKE_SKIP_UNHEALTHY` | `0` | `1` downgrades unregistered/stale clusters to a skip instead of the fail-closed default |
| `E2E_SMOKE_HEALTHY_WINDOW` | `5m` | `last_seen_at` age within which a cluster is considered Healthy (per the registration staleness table) |
| `E2E_SMOKE_CONCURRENCY` | `1` | Number of per-cluster runs to execute in parallel; `1` is sequential |
| `E2E_SMOKE_GATEWAY_PREFIX` | `smoke` | Prefix for the per-cluster gateway name (`<prefix>-<cluster>-<runid>`) |
| `E2E_SMOKE_KUBECONTEXT_<name>` | (unset) | Kube context for cluster `<name>`, enabling that cluster's kube-level short-slice steps; absent means product-interface smoke only |

## Design Decisions

- **Thin driver, no new assertions.** The matrix loops the existing short slice,
  exactly as `e2e-performance.sh` loops the suite. Short mode is already the
  specified promotion-gate slice (self-contained, owns its gateway, single
  identity), so the per-cluster check is a solved problem; the only new work is
  enumeration, isolation, aggregation, and the verdict.
- **Product-interface smoke is the baseline; kube checks are opt-in.** A
  dev-to-prod gate across a prod fleet should not require a privileged kubeconfig
  for every cluster in the pipeline. The baseline proves each cluster can
  provision and run a working gateway through the product's own API and gRPC
  interfaces; the kube-level infra/GC assertions run only where a context is
  supplied and degrade to skips otherwise. (Set per the user's smoke-depth
  choice; flip to always-require-kube if a stricter gate is wanted.)
- **Fail closed on unhealthy clusters.** A fleet with an unregistered, stale, or
  unreachable cluster is not safe to promote onto, so such a cluster fails the
  gate by default; `E2E_SMOKE_SKIP_CLUSTERS` / `E2E_SMOKE_SKIP_UNHEALTHY` provide
  an explicit, auditable override for known maintenance windows.
- **Fleet discovered from the live registry, topology owned by gitops.** The
  matrix asks `/managed_clusters` what exists rather than hard-coding a cluster
  list, keeping it correct as the fleet changes; the environment-to-cluster
  topology stays in the gitops repo that already owns the promotion pipeline.
