# Gateway Reconcile Concurrency

**Status:** Draft
**Applies to:** `components/control-plane` gateway reconcile queue worker pool and its configuration
**Jira:** HYPERSHELL-XXX

## Purpose

Give operators a supported way to tune how many *distinct* gateways the control
plane provisions concurrently, without changing code. Today the gateway reconcile
queue drains through a fixed pool of worker goroutines whose size is a hardcoded
constant (`gatewayReconcileWorkers = 4` in
`components/control-plane/internal/watcher/requeue.go`), overridable only by a
test-only option. When more gateways than the worker count are created in a short
window, the surplus waits in the queue until a worker frees, so provisioning wait
time grows with burst size - the behavior users reported for a burst of six
gateways.

This specification makes the worker-pool size deployment configuration, separate
from code, with a bounded default and validated input, so the concurrency cap can
be raised (or lowered) per environment. It preserves the two invariants the pool
exists to guarantee: work for a single gateway is never run by two workers at once
(per-gateway serialization), and cross-gateway parallelism stays bounded by the
configured cap (the pool is a throttle, never unbounded fan-out).

This specification covers the control plane component only. It does not change the
API server, the gRPC contract, the data model, or the per-gateway provisioning
steps. It does not reduce the time a single gateway takes to provision; it only
governs how many provisions may run at once. Where
`platform/control-plane-observability.spec.md` (HYPERSHELL-79) defines the
`reconcile.queue.depth` and `reconcile.queue.wait.duration` metrics, those metrics
observe the same queue this specification tunes.

### Non-goals

- **Multi-replica correctness.** The in-process queue guarantees per-gateway
  exclusion only *within one control-plane process*. Making two replicas safe to
  reconcile the same gateway (for example via a PostgreSQL advisory lock keyed by
  gateway id) is a separate high-availability decision and is out of scope. The
  control plane is deployed as a single replica today.
- **Reducing per-gateway provision time.** The multi-minute cost of a single
  provision (image pull, scheduling, TLS-secret wait, readiness waits) is driven
  by the target cluster, not the worker count, and is not addressed here.
- **Changing the queue's other behaviors** (per-key coalescing to latest, capped
  exponential backoff, retry semantics, seed-on-reconnect). This specification
  changes only the pool size and how it is configured.

## Requirements

### Requirement: CP-CONC-01 -- Configurable Gateway Reconcile Worker Count

The control plane SHALL make the size of the gateway reconcile worker pool
deployment configuration, supplied through an environment variable, so the
cross-gateway provisioning concurrency cap can be changed without a code change or
rebuild. Configuration SHALL be separate from code.

| Env Var | Default | Description |
|---------|---------|-------------|
| `GATEWAY_RECONCILE_WORKERS` | (bounded default, see CP-CONC-03) | Maximum number of *distinct* gateways reconciled concurrently by the control-plane worker pool |

The configured value SHALL flow from configuration load through the control-plane
entrypoint into the gateway reconcile queue construction, so the running pool size
reflects the configured value. When the variable is unset or empty, the control
plane SHALL use the bounded default and SHALL operate exactly as it does today.

**Verification:** Start the control plane with `GATEWAY_RECONCILE_WORKERS` set to a
value greater than the default; create a burst of gateways larger than the default;
confirm more than the default number of gateways are in `Provisioning` at once,
bounded by the configured value. Start it unset and confirm the default applies.

#### Scenario: Worker count raised via configuration

- GIVEN `GATEWAY_RECONCILE_WORKERS` is set to `8`
- AND ten gateways are created within a short window
- WHEN the control plane reconciles the burst
- THEN up to eight gateways SHALL be in `Provisioning` simultaneously
- AND the remaining gateways SHALL wait in the queue until a worker frees

#### Scenario: Default applies when unset

- GIVEN `GATEWAY_RECONCILE_WORKERS` is not set
- WHEN the control plane starts
- THEN the gateway reconcile pool SHALL use the bounded default worker count
- AND cross-gateway concurrency SHALL behave as it did before this configuration existed

### Requirement: CP-CONC-02 -- Preserved Concurrency Invariants

Regardless of the configured worker count, the control plane SHALL preserve the
invariants the pool exists to enforce:

- Work for a single gateway SHALL NOT be run by two workers at the same time
  (per-gateway serialization). A key in flight SHALL NOT be handed to a second
  worker, and coalescing to the latest observed state SHALL be preserved.
- Cross-gateway parallelism SHALL be bounded by the configured worker count. The
  pool SHALL remain a throttle; the control plane SHALL NOT spawn an unbounded
  number of concurrent gateway reconciles.

A configured value greater than one SHALL increase only cross-gateway parallelism,
never per-gateway parallelism.

**Verification:** With a raised worker count, drive concurrent updates to the same
gateway and confirm only one reconcile for that gateway runs at a time while
different gateways reconcile in parallel up to the cap.

#### Scenario: Per-gateway serialization holds under a raised cap

- GIVEN `GATEWAY_RECONCILE_WORKERS` is set above one
- AND multiple events arrive for the same gateway
- WHEN the queue dispatches work
- THEN at most one reconcile for that gateway SHALL run at any instant
- AND reconciles for different gateways MAY run concurrently up to the configured cap

#### Scenario: Concurrency stays bounded

- GIVEN `GATEWAY_RECONCILE_WORKERS` is set to `N`
- WHEN more than `N` gateways are ready to reconcile
- THEN no more than `N` gateway reconciles SHALL run concurrently

### Requirement: CP-CONC-03 -- Bounded Default and Input Validation

The control plane SHALL use a bounded, positive default worker count when the
variable is unset or empty, preserving today's behavior. The default SHALL be a
documented, positive integer.

The control plane SHALL validate the configured value. A value that is not a valid
positive integer (non-numeric, zero, or negative) SHALL cause the control plane to
log a warning naming the variable, the offending value, and the default in use, and
SHALL fall back to the bounded default rather than crash - consistent with the
existing configuration helpers (`getEnvBool`, `getEnvDuration`) in
`components/control-plane/internal/config/config.go`. The resolved worker count
SHALL always be a positive integer, so the pool always has at least one worker.

The control plane SHALL NOT permit a configured value to disable reconciliation
(zero or fewer workers). Whether an explicit upper bound is enforced on very large
values is a design decision recorded below; if enforced, an over-large value SHALL
be clamped with a logged warning rather than rejected.

**Verification:** Start the control plane with `GATEWAY_RECONCILE_WORKERS` set to an
invalid value (`abc`, `0`, `-3`); confirm a warning is logged and the default pool
size is used, and that reconciliation proceeds normally.

#### Scenario: Invalid value falls back to the default

- GIVEN `GATEWAY_RECONCILE_WORKERS` is set to `abc`
- WHEN the control plane starts
- THEN it SHALL log a warning naming the variable, the value, and the default
- AND it SHALL use the bounded default worker count
- AND reconciliation SHALL proceed normally

#### Scenario: Non-positive value falls back to the default

- GIVEN `GATEWAY_RECONCILE_WORKERS` is set to `0`
- WHEN the control plane starts
- THEN it SHALL log a warning
- AND it SHALL use the bounded default worker count
- AND the pool SHALL have at least one worker

## Design Decisions

| Decision | Rationale |
| --- | --- |
| Expose worker count as an environment variable, not a flag or code constant | Matches the existing control-plane configuration pattern (`GATEWAY_NAMESPACE_GC_*`, `DATABASE_PROVIDER`); config changes must not require a code change |
| Keep a bounded positive default equal to today's behavior | Zero-surprise upgrade: an unset variable reconciles exactly as before |
| Invalid value warns and falls back rather than failing startup | Consistent with `getEnvBool`/`getEnvDuration`; a mistuned knob should not take the control plane down. (Contrast with `DATABASE_PROVIDER`, which fails fast because a wrong provider silently mis-provisions databases; a wrong worker count only mis-sizes a throttle) |
| Pool stays a bounded throttle, never unbounded | Unbounded parallelism could overwhelm target-cluster scheduling capacity and worsen `Degraded` outcomes; the original request was explicitly "parallel **but throttled**" |
| Raising concurrency only, not reducing per-provision time | Per-provision cost is environment-driven (cluster image pull, scheduling), orthogonal to how many run at once |
| Multi-replica advisory lock deferred | The in-process queue already serializes per gateway within one process; cross-replica exclusion is a separate HA decision and the deployment runs a single replica |

## Open Questions

- **Right default and per-environment values.** Raising the cap clearly helps where
  a single provision is cheap; on a capacity-constrained cluster the useful value is
  bounded by scheduling headroom, so the correct value must be tuned per
  environment, not guessed. The default chosen here should stay conservative.
- **Explicit upper bound.** Whether to clamp very large configured values (and at
  what ceiling) versus trusting the operator. Recorded as a design decision above;
  the safe default is to accept any positive integer and rely on the operator, with
  clamping as an option if an abuse case emerges.

## Primary Basis

- `platform/control-plane.spec.md` (gateway reconciliation, watcher/queue)
- `platform/control-plane-observability.spec.md` (HYPERSHELL-79 - `reconcile.queue.depth`, `reconcile.queue.wait.duration` observe this queue)
- `standards/control-plane/conventions.spec.md` (configuration, error handling, no panic)
- `components/control-plane/internal/watcher/requeue.go` (`gatewayReconcileWorkers`, `newReconcileQueue`, `withWorkers`)
- `components/control-plane/internal/watcher/watcher.go` (`WatchGateways` queue construction)
- `components/control-plane/internal/config/config.go` (`Load`, `getEnvBool`, `getEnvDuration`)
