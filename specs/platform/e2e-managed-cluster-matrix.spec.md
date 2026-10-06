# E2E Per-ManagedCluster Matrix

**Date:** 2026-10-06
**Status:** Draft
**Related:** `e2e-testing.spec.md` (the `E2E_MODE` depths this matrix passes through, the driver contract, the single-cluster suite `tests/e2e/e2e-openshell.sh`, and the performance harness pattern of driving repeated suite runs);
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

This specification defines a **per-cluster matrix runner**: a thin orchestrator
that enumerates registered ManagedClusters and runs the existing e2e suite
against each one, then reports a per-cluster pass/fail/skip matrix and a single
gate verdict. The runner is deliberately test-agnostic - it fans the
single-cluster suite out across the fleet and aggregates the results. The depth
of each per-cluster run is whatever `E2E_MODE` the caller selects (`short`,
`long`, or `perf`); the matrix does not constrain it.

The matrix adds **no new test assertions**. It reuses the existing single-cluster
suite unchanged, driving it once per cluster the same way the performance harness
drives repeated suite runs. New behavior is confined to cluster enumeration and
selection, per-cluster targeting and process isolation, bounded parallelism,
aggregated reporting, and the gate verdict.

The matrix is **built into the e2e tests, not a new infra driver.** Infra drivers
(`tests/e2e/drivers/`) are intentionally infra-specific - they teach the suite how
to reach a kind or OpenShift environment. Matrixing across clusters is
orthogonal to that: the runner selects and validates the infra driver exactly as
the single-cluster suite does, then invokes the suite once per selected cluster.

### Scope

In scope: a new matrix entry point (`tests/e2e/e2e-matrix.sh`), cluster
enumeration and selection, per-cluster targeting and process isolation, bounded
parallelism, aggregated matrix reporting, and the promotion gate contract. Out of
scope: the per-cluster test assertions and the `E2E_MODE` depths themselves
(owned by the single-cluster suite in `e2e-testing.spec.md`), the dev-to-prod
environment topology and the Kargo promotion pipeline (owned by the gitops repo,
see its `specs/progressive-delivery.md`; the fleet-dashboard view of promotion
lives in `packages/fleet-dashboard-ui/src/domain/promotion.ts`), and per-gateway
release promotion (area 13).

---

## Requirements

### Requirement: Matrix Entry Point and Suite Reuse

The system SHALL provide `tests/e2e/e2e-matrix.sh`: a runner that enumerates
registered ManagedClusters and runs the existing e2e suite against each. It SHALL
source `tests/e2e/lib.sh`, select and validate the infra driver exactly as
`e2e-openshell.sh` does, and for each selected cluster invoke the existing
single-cluster suite as a **separate child process**. It SHALL NOT duplicate any
area's assertion code and SHALL NOT constrain `E2E_MODE`; each per-cluster run is
the single-cluster suite the repository already defines, at the depth the caller
selects. The runner itself requires only the driver functions needed to list
clusters (`discover_api_host`, `acquire_oidc_token`, `api_curl`); each child run
performs its own driver selection and `REQUIRED_FUNCTIONS` check.

The runner SHALL pass `E2E_MODE` through to each child unchanged, defaulting to
the suite's own default when unset. A promotion gate typically sets
`E2E_MODE=short`, but the runner itself is mode-agnostic: a full (`long`) or
performance (`perf`) matrix is equally valid.

#### Scenario: Matrix runs the suite per cluster

- GIVEN a fleet with N registered ManagedClusters
- WHEN `bash tests/e2e/e2e-matrix.sh` runs
- THEN the suite SHALL run once per selected cluster, each as its own process with its own gateway
- AND no per-cluster check SHALL be a second copy of an assertion already in `e2e-openshell.sh`

#### Scenario: The run depth is the caller's choice

- GIVEN `E2E_MODE=short`, `E2E_MODE=long`, or `E2E_MODE=perf`
- WHEN the matrix runs
- THEN each per-cluster child SHALL run at that depth
- AND the matrix SHALL NOT reject or override the selected mode

### Requirement: Cluster Enumeration and Selection

The matrix SHALL enumerate clusters by listing
`GET /api/hypershell/v1/managed_clusters` with the admin token and selecting the
**registered** records - those with a non-empty `oidc_subject`, the same marker
`e2e_json_registered_cluster_id` uses for single-cluster seed discovery. By
default it SHALL select every registered cluster.

The selection SHALL be narrowable with a single allowlist, mirroring how a GitHub
Actions matrix is declared explicitly:

- `E2E_MANAGED_CLUSTERS` - a comma-separated allowlist of ManagedCluster names;
  when set, only those clusters run (and a named cluster that is absent or
  unregistered SHALL be treated per
  [Unhealthy and Unreachable Cluster Handling](#requirement-unhealthy-and-unreachable-cluster-handling)).

There is no separate denylist: to exclude a cluster, omit it from the allowlist,
or rely on the fail-closed / skip-unhealthy handling below for clusters that are
down.

Each selected cluster SHALL have its health derived from `last_seen_at` using the
staleness table in `managed-cluster-registration.spec.md` (Healthy `< 5 min`,
Unknown `5-30 min`, Offline `> 30 min`); the healthy window is overridable via
`E2E_MANAGED_HEALTHY_WINDOW` (default `5m`). The matrix SHALL print the selected
set, each cluster's derived health, and the reason any cluster was excluded,
before running anything.

#### Scenario: Default selects all registered clusters

- GIVEN a fleet with three registered clusters and one record with an empty `oidc_subject`
- WHEN the matrix runs with no selection variables set
- THEN it SHALL run against the three registered clusters
- AND it SHALL skip the unregistered record, naming the empty `oidc_subject` as the reason

#### Scenario: Allowlist narrows the matrix

- GIVEN `E2E_MANAGED_CLUSTERS=prod-us-east,prod-eu-west`
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
  cluster and the matrix run (for example `${E2E_MANAGED_GATEWAY_PREFIX}-<cluster>-<runid>`,
  default prefix `e2e`), so concurrent or repeated runs never collide and the
  suite's own-and-teardown guarantee holds per cluster.
- **Process isolation.** Each run SHALL be a separate process, so the flat,
  process-global counters (`E2E_PASS`/`E2E_FAIL`/`E2E_TESTS`) and the gateway
  lifecycle reset cleanly between clusters; a failure or hard `exit 1` in one
  cluster's run SHALL NOT abort the others.

The matrix SHALL support bounded parallelism via `E2E_MANAGED_CONCURRENCY`
(default `4`): up to that many per-cluster child processes run at once, the rest
queue. Because each run owns and tears down its own uniquely-named gateway,
parallel per-cluster runs are safe. Set `E2E_MANAGED_CONCURRENCY=1` to force
sequential execution (for example when log interleaving must be avoided).

#### Scenario: Each run targets its own cluster and gateway

- GIVEN two selected clusters `a` and `b`
- WHEN the matrix runs them
- THEN the run for `a` SHALL create its gateway with `cluster_id` of `a` and a name scoped to `a`, and the run for `b` likewise for `b`
- AND neither run SHALL leave a gateway behind, on success or failure

#### Scenario: One cluster's failure does not abort the matrix

- GIVEN three selected clusters where the second fails its run
- WHEN the matrix runs
- THEN the first, second, and third SHALL each run to completion and be reported
- AND the second's failure SHALL NOT prevent the third from running

#### Scenario: Concurrency is bounded

- GIVEN a fleet of ten selected clusters and `E2E_MANAGED_CONCURRENCY=4`
- WHEN the matrix runs
- THEN at most four per-cluster child processes SHALL run at any one time
- AND all ten SHALL be run and reported

### Requirement: Per-Cluster Run Depth and Kube Access

Each per-cluster run exercises the target cluster at the depth its `E2E_MODE`
selects, through the product's own interfaces as its baseline: provision a gateway
on that cluster via the HyperShell API and wait for `Running`, connect the
openshell CLI to the gateway's gRPC endpoint over trusted TLS, run the mode's
sandbox lifecycle, and delete the gateway - the suite steps that need only API
credentials and gateway DNS, not kube access to the target cluster.

The suite's infrastructure-level steps that require kube access to the target
cluster (area 3 deployment/service verification, area 11 delete-driven
namespace-GC assertion) SHALL run when a kube context for that cluster is
available to the matrix and SHALL degrade to a recorded skip (not a failure) when
it is not. The matrix SHALL accept a per-cluster kube context mapping
(`E2E_MANAGED_KUBECONTEXT_<name>`, or a context derived from the cluster's
`KubeconfigSecret` where the runner can read it); absent a mapping, the baseline
product-interface run still proves the cluster can provision and run a working
gateway.

#### Scenario: Product-interface run needs no per-cluster kube access

- GIVEN a selected cluster reachable only through the central API and its gateway DNS
- WHEN the matrix runs its per-cluster suite
- THEN it SHALL provision a gateway on that cluster, reach `Running`, connect the CLI, run a sandbox lifecycle, and delete the gateway
- AND the kube-level infra-verification and GC-assertion steps SHALL be recorded as skips, not failures

#### Scenario: Kube context enables the full infra-level steps

- GIVEN a selected cluster with a kube context supplied to the matrix
- WHEN the matrix runs its per-cluster suite
- THEN the suite's infra-verification and delete-driven GC assertions SHALL run against that cluster in addition to the product-interface steps

### Requirement: Unhealthy and Unreachable Cluster Handling

A promotion gate SHALL fail closed. A selected cluster that is unregistered (no
`oidc_subject`), stale (its `last_seen_at` is beyond the healthy window), or
unreachable (its run cannot reach the API placement or the gateway) SHALL be
reported as a **failure** of the matrix by default, because a fleet with a down
cluster is not safe to promote onto.

An operator MAY flip the default with `E2E_MANAGED_SKIP_UNHEALTHY=1` (skip any
cluster that is unregistered or stale rather than fail), or simply omit a
known-offline cluster from `E2E_MANAGED_CLUSTERS`. A skip SHALL be reported
distinctly from a pass and from a failure, and SHALL never be counted as a pass.

#### Scenario: Stale cluster fails the gate by default

- GIVEN a selected cluster whose `last_seen_at` is older than the healthy window
- WHEN the matrix runs with default settings
- THEN that cluster SHALL be reported as a failure and the matrix verdict SHALL be non-zero

#### Scenario: Skip-unhealthy downgrades a down cluster

- GIVEN `E2E_MANAGED_SKIP_UNHEALTHY=1` and a cluster known to be offline for maintenance
- WHEN the matrix runs
- THEN that cluster SHALL be recorded as a skip, not a failure, and SHALL not by itself fail the gate
- AND the skip SHALL be distinct from a pass in the report

### Requirement: Aggregated Matrix Reporting and Verdict

The matrix SHALL print a per-cluster result table - one row per selected cluster
with its name, derived health, result (pass / fail / skip), and the child run's
pass/fail tally - followed by an overall verdict line. The report SHALL make the
three outcomes unambiguous and SHALL name the clusters that failed. Because runs
may be parallel, per-cluster child output SHALL be captured and attributed to its
cluster in the report rather than interleaved raw.

The matrix SHALL exit non-zero if any selected cluster failed (including the
fail-closed unhealthy cases above), and zero only when every selected cluster
passed or was skipped. The verdict SHALL be the gate signal a promotion pipeline
consumes.

#### Scenario: Matrix verdict reflects the worst cluster

- GIVEN four selected clusters: three pass and one fails
- WHEN the matrix finishes
- THEN the report SHALL show three passes and one failure, naming the failed cluster
- AND the matrix SHALL exit non-zero

#### Scenario: All-pass yields a clean gate

- GIVEN every selected cluster passes its run
- WHEN the matrix finishes
- THEN the report SHALL show all passes and the matrix SHALL exit zero

#### Scenario: Partial run is not reported as success

- GIVEN the matrix aborts partway (for example the initial `/managed_clusters` list fails)
- WHEN it exits
- THEN it SHALL exit non-zero and SHALL NOT report a clean gate, mirroring the single-cluster suite's abort handling

### Requirement: Promotion Gate Contract

A dev-to-prod promotion SHALL run the per-cluster matrix against the target
environment's fleet and SHALL block the promotion when the matrix verdict is
non-zero. The matrix is the mechanism; the promotion pipeline is the consumer.
The dev-to-prod promotion is driven by **Kargo** in the gitops repo (the
canary/main pairing pattern in its `specs/progressive-delivery.md`), which also
owns the environment topology (which clusters belong to which environment, gate
ordering); none of that is defined or duplicated here. This spec owns only the
matrix the external pipeline invokes as a verification step. The matrix discovers
the fleet from the live `/managed_clusters` registry of the environment it is
pointed at, so the external pipeline needs no cluster list of its own.

This requirement records the contract so the existing external promotion gate
invokes the matrix in place of running the single-cluster suite once; it does not
add a CI job to this repository.

#### Scenario: Promotion blocks on any cluster failure

- GIVEN a promotion pipeline configured to run the matrix against the target fleet
- WHEN any selected cluster fails its run
- THEN the matrix SHALL exit non-zero and the promotion SHALL be blocked

## Environment Variables

These extend the table in `e2e-testing.spec.md`. The matrix reuses the admin
OIDC variables, `E2E_INFRA_DRIVER`, `E2E_MODE`, and the timeout variables
unchanged; each child run reuses the full single-cluster variable set.

| Env Var | Default | Description |
|---------|---------|-------------|
| `E2E_MANAGED_CLUSTERS` | (unset = all registered) | Comma-separated allowlist of ManagedCluster names to run |
| `E2E_MANAGED_SKIP_UNHEALTHY` | `0` | `1` downgrades unregistered/stale clusters to a skip instead of the fail-closed default |
| `E2E_MANAGED_HEALTHY_WINDOW` | `5m` | `last_seen_at` age within which a cluster is considered Healthy (per the registration staleness table) |
| `E2E_MANAGED_CONCURRENCY` | `4` | Max per-cluster runs to execute in parallel; `1` forces sequential |
| `E2E_MANAGED_GATEWAY_PREFIX` | `e2e` | Prefix for the per-cluster gateway name (`<prefix>-<cluster>-<runid>`) |
| `E2E_MANAGED_KUBECONTEXT_<name>` | (unset) | Kube context for cluster `<name>`, enabling that cluster's kube-level suite steps; absent means product-interface run only |

## Design Decisions

- **Thin runner, no new assertions, mode-agnostic.** The matrix loops the
  existing suite, exactly as `e2e-performance.sh` loops it, and passes `E2E_MODE`
  through untouched. The per-cluster check is a solved problem; the only new work
  is enumeration, isolation, bounded parallelism, aggregation, and the verdict.
  Scoping the runner to "matrix across managed clusters" rather than to a
  specific smoke slice keeps it reusable for `short` gates, `long` fleet sweeps,
  and `perf` matrices alike.
- **Built into the tests, not a new infra driver.** Infra drivers are
  intentionally infra-specific (kind vs OpenShift); matrixing across clusters is
  orthogonal. The runner selects the driver the same way the single-cluster suite
  does and fans that suite out, so it composes with any driver rather than
  becoming one.
- **Allowlist only, default all.** A single `E2E_MANAGED_CLUSTERS` allowlist
  mirrors how a GitHub Actions matrix is declared and avoids the ambiguity of a
  simultaneous allow/deny pair. Exclusion is expressed by narrowing the allowlist
  or by the fail-closed / skip-unhealthy handling, not a second list.
- **Shell, with bounded parallelism; Go is the escape hatch.** The runner is
  loop-and-collect over an existing shell suite - enumerate once, fan out child
  processes capped at `E2E_MANAGED_CONCURRENCY`, gather exit codes, print a table.
  Bash does coarse-grained process parallelism well, and all the enumeration,
  token, and driver logic already lives in `lib.sh`; a Go runner would either add
  a toolchain to shell out to the same suite or duplicate `lib.sh` and risk
  drift. If the runner ever grows shared concurrent state (cross-cluster rate
  limiting, streaming aggregation, a long-lived control loop), revisiting a Go
  implementation is the escape hatch - but that is out of scope here.
- **Product-interface run is the baseline; kube checks are opt-in.** A dev-to-prod
  gate across a prod fleet should not require a privileged kubeconfig for every
  cluster in the pipeline. The baseline proves each cluster can provision and run
  a working gateway through the product's own API and gRPC interfaces; the
  kube-level infra/GC assertions run only where a context is supplied and degrade
  to skips otherwise.
- **Fail closed on unhealthy clusters.** A fleet with an unregistered, stale, or
  unreachable cluster is not safe to promote onto, so such a cluster fails the
  gate by default; `E2E_MANAGED_SKIP_UNHEALTHY` and narrowing
  `E2E_MANAGED_CLUSTERS` provide explicit, auditable overrides for known
  maintenance windows.
- **Fleet discovered from the live registry, topology owned by gitops.** The
  matrix asks `/managed_clusters` what exists rather than hard-coding a cluster
  list, keeping it correct as the fleet changes; the environment-to-cluster
  topology stays in the gitops repo that already owns the Kargo promotion
  pipeline.
