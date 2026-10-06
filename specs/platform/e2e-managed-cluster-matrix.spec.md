# E2E Per-ManagedCluster Matrix

**Date:** 2026-10-06
**Status:** Superseded
**Related:** `e2e-testing.spec.md` (owns the matrix runner contract);
             `managed-cluster-registration.spec.md` (ManagedCluster fields, `oidc_subject` as the "registered" marker, the `last_seen_at` staleness table, `/managed_clusters`, gateway `cluster_id` placement);
             `gateway-managed-cluster-attribution.spec.md` (per-cluster gateway/sandbox metric attribution);
             `gateway-release-rollout.spec.md` (release promotion area 13, distinct from the environment promotion this gate guards)

---

## Purpose

This spec is **superseded**. The per-ManagedCluster matrix was originally designed
as a Bash orchestrator (`tests/e2e/e2e-matrix.sh`) that ran the single-cluster
shell suite once per cluster as a separate child process to isolate the suite's
process-global counters.

The e2e framework has since been rewritten from Bash to Go (`testify/suite`); see
the **Test Framework and Suite Structure** and **Managed-Cluster Matrix Runner**
requirements in `e2e-testing.spec.md`, which now own the matrix contract in full.
The Go rewrite lands first, so this dedicated spec is retired rather than
maintained separately.

What changed, and why the dedicated spec is no longer needed:

- **Runner is Go, not Bash.** The matrix is `TestMatrix` (`tests/e2e/matrix_test.go`),
  run via `make e2e-matrix`, reusing the in-process `E2ESuite` per cluster. There is
  no `tests/e2e/e2e-matrix.sh`.
- **Goroutines, not child processes.** Each cluster runs as its own `E2ESuite`
  instance with its own per-suite state, fanned out as bounded goroutines
  (`E2E_MANAGED_CONCURRENCY`, default `4`). The Bash child-process isolation existed
  only to work around process-global counters, which the Go suite no longer has.
- **Everything else carries over unchanged** and is specified in the Managed-Cluster
  Matrix Runner requirement in `e2e-testing.spec.md`: the single matrix dimension
  (registered ManagedClusters from `GET /v1/managed_clusters`), `E2E_MODE`
  pass-through (`short` is the smoke subset), per-cluster uniquely named gateways
  with guaranteed teardown, `E2E_MANAGED_KUBECONTEXT_<name>`-gated kube-level steps
  that otherwise skip, fail-closed handling of unregistered/stale/unreachable
  clusters (`E2E_MANAGED_SKIP_UNHEALTHY`, `E2E_MANAGED_HEALTHY_WINDOW`), the
  allowlist (`E2E_MANAGED_CLUSTERS`), aggregated per-cluster reporting with a single
  verdict, and the Kargo dev->prod promotion gate contract.

No requirements remain in this file. Author new matrix behavior against the
Managed-Cluster Matrix Runner requirement in `e2e-testing.spec.md`.
