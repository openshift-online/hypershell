# Control Plane Reconciliation Contract

**ID prefix:** STD-RCON
**Status:** Active
**Related:** `platform/control-plane.spec.md` (CTRL-CORE), `platform/data-model.spec.md` (PLAT-DATA-06), `platform/openshell-gateway-health.spec.md` (OSG-HLTH-03), `platform/gateway-reconcile-concurrency.spec.md` (GTW-CONC), `platform/gateway-deletion-finalization.spec.md` (GTW-DEL), `platform/watch-delete-events.spec.md` (CTRL-WDEL), `standards/control-plane/conventions.spec.md` (STD-CTRL, the development quick reference)

## Purpose

This spec is the single, normative behavior contract for every reconciler the
HyperShell control plane runs. Today the reconciliation guarantees are correct
but scattered across six specs, so a new reconciler author has no one place to
learn the required behavior. This contract collects those guarantees as one
coherent set, states each against the system **as actually built**, and is the
bar that new or materially changed reconciliation work must meet.

This spec is a standard, not a plan. Each guarantee below is refined, with the
concrete Kinds and mechanics, by a per-domain spec listed under "Related" above.
A refining spec MAY add detail and MAY tighten a guarantee; it SHALL NOT weaken
one. Where a refining spec deliberately deviates, the deviation is a recorded
exception under this contract (STD-RCON-12), not a silent divergence.

The contract describes the real model, not an idealized one. In particular, and
in deliberate contrast to the superseded design in closed PR #200:

- Deletion uses soft-delete of the PostgreSQL row plus a delete tombstone and a
  delete event that carries the pre-delete snapshot. It does **not** use a
  Kubernetes finalizer or a `deletionTimestamp`. The PostgreSQL row is the source
  of truth (STD-RCON-07).
- Concurrency control uses `generation` and `observed_generation` with a
  convergence-latch range check. It does **not** use a per-write resource-version
  token (STD-RCON-06).
- The control plane runs as a single replica per managed cluster. Cross-process
  locking and leader election are an explicit non-goal (STD-RCON-11).

## Domain Vocabulary

- **Desired state** - the resource rows in the API server's PostgreSQL database.
  This is authoritative. The control plane is a reader of desired state, except
  for the single, ownership-scoped `RepositoryReconciler` import write path
  (`control-plane.spec.md`, CTRL-CORE-06).
- **Reconcile key** - the identity of one resource (for example one Gateway id)
  whose work the queue serializes.
- **Convergence** - a Gateway is *converged* when `observed_generation ==
  generation`; the provisioning gate keys on convergence, not on `phase`.
- **Tombstone** - the durable record (soft-deleted row plus anti-resurrection
  marker) that keeps a delete retrying and stops a stale seed from
  re-provisioning a resource that was deleted.
- **Recorded exception** - a named, spec-level decision that a reconciler
  deviates from a guarantee here, with its rationale (STD-RCON-12).

## Requirements

### Requirement: STD-RCON-01 -- Desired State Is Authoritative; Events Only Schedule Work

The database desired state SHALL be authoritative. A watch event SHALL be
treated as a trigger that schedules a reconcile of a key, not as the source of
truth. A reconcile SHALL converge actual state toward the current desired state
rather than act on an event payload alone, so a duplicated, reordered, or
coalesced event cannot drive an incorrect outcome. The one payload a reconcile
SHALL read directly is the delete event's pre-delete snapshot, which carries the
state teardown needs after the row is gone (STD-RCON-07,
`watch-delete-events.spec.md`). A dropped or missed event SHALL be recoverable
by seed and resync (STD-RCON-05).

**Verification:** Inspection -- the behavior is proven by the refining specs and
their tests (CTRL-CORE-02, OSG-HLTH-03).

#### Scenario: STD-RCON-01.01 -- Redundant event does not change the outcome

- GIVEN a resource whose desired state is unchanged
- WHEN a duplicate or reordered watch event schedules a reconcile
- THEN the reconcile SHALL read the current desired state and converge to it
- AND the duplicate event SHALL NOT produce a different result than a single event

#### Scenario: STD-RCON-01.02 -- A missed event is recovered without the event

- GIVEN a create or update event that was never delivered (the stream was silent)
- WHEN the stream seeds on reconnect or the periodic resync runs
- THEN the resource SHALL still be reconciled from the listed desired state
- AND recovery SHALL NOT depend on replaying the missed event

### Requirement: STD-RCON-02 -- Level-Based, Idempotent Convergence; Drift Repaired on a Later Pass

Reconciliation SHALL be level-based and idempotent. Each reconcile SHALL use
update-or-create ("reconcile, don't create-or-skip"); it SHALL NOT create a
resource and ignore an `AlreadyExists`, and re-running it against unchanged
desired and actual state SHALL make no change. Drift between desired and actual
state SHALL be repaired on a later pass (a health reconcile, a resync, or a
subsequent event), not only at create time.

**Verification:** Inspection -- refined and tested by CTRL-CORE-04, OSG-HLTH-02,
OSG-HLTH-03.

#### Scenario: STD-RCON-02.01 -- Re-running against unchanged state makes no change

- GIVEN a resource already converged to its desired state
- WHEN the reconciler runs again for that key
- THEN it SHALL make no create, update, or delete call
- AND it SHALL report success

#### Scenario: STD-RCON-02.02 -- Drift is repaired on a later pass

- GIVEN a converged resource whose actual cluster state drifts from desired
- WHEN the next reconcile or health pass observes the drift
- THEN the reconciler SHALL repair the actual state toward desired
- AND it SHALL NOT require the resource to be recreated

### Requirement: STD-RCON-03 -- Per-Resource Serialization Within a Bounded Worker Pool

Work for a single reconcile key SHALL NOT run in two workers at once
(per-key serialization), and queued work for a key SHALL coalesce to the latest
observed state. Cross-resource parallelism SHALL be bounded by a configured
worker pool: the pool is a throttle, never unbounded fan-out. A configured
concurrency greater than one SHALL increase only cross-key parallelism, never
per-key parallelism. This contract governs in-process serialization only; it
does not make two replicas safe (STD-RCON-11).

**Verification:** Inspection -- refined and tested by GTW-CONC-02.

#### Scenario: STD-RCON-03.01 -- One key is never reconciled by two workers

- GIVEN multiple events queued for the same reconcile key
- WHEN the pool dispatches work
- THEN at most one reconcile for that key SHALL run at any instant
- AND reconciles for different keys MAY run concurrently up to the configured cap

#### Scenario: STD-RCON-03.02 -- Cross-key parallelism stays bounded

- GIVEN more keys ready to reconcile than the configured worker count `N`
- WHEN the pool drains the queue
- THEN no more than `N` reconciles SHALL run concurrently

### Requirement: STD-RCON-04 -- Bounded Exponential Backoff Retry

A failed reconcile SHALL be retried with bounded (capped) exponential backoff.
A transient failure SHALL keep its key queued until the reconcile succeeds; the
retry SHALL NOT be an unbounded tight loop, and the backoff SHALL have a ceiling
so a persistently failing key does not starve the pool or stop being retried.

**Verification:** Inspection -- refined and tested by CTRL-CORE-02, GTW-DEL-05.

#### Scenario: STD-RCON-04.01 -- Transient failure is retried with capped backoff

- GIVEN a reconcile that fails with a transient error
- WHEN the queue reschedules the key
- THEN it SHALL retry with exponential backoff capped at a ceiling
- AND it SHALL keep retrying until the reconcile succeeds

### Requirement: STD-RCON-05 -- Seed on (Re)connect; Periodic Resync Where Required

Because a watch stream does not replay resources that already exist, every watch
stream SHALL seed from a `List`, scoped to the control plane's own `cluster_id`,
on every (re)connect, and SHALL queue each listed resource for reconciliation
once the subscription is live. Where a single missed event would otherwise leave
state unconverged with no list-driven recovery, the control plane SHALL also run
a periodic resync (for example the RoleBinding resync, the continuous health
reconcile, and the orphaned-namespace garbage collector). Seed and resync SHALL
be idempotent.

**Verification:** Inspection -- refined and tested by CTRL-CORE-02, CTRL-CORE-07,
CTRL-CORE-08.

#### Scenario: STD-RCON-05.01 -- Startup seeds resources created while down

- GIVEN resources for this control plane's cluster that were created while it was down
- WHEN the control plane starts and the watch subscription becomes live
- THEN it SHALL list those resources and queue each for reconciliation
- AND it SHALL NOT wait for a watch event to reconcile them

#### Scenario: STD-RCON-05.02 -- Periodic resync recovers a missed change

- GIVEN a create or update that produced no delivered event
- WHEN the periodic resync interval elapses
- THEN the control plane SHALL re-list and reconcile the affected resources
- AND an already-converged resource SHALL produce no change

### Requirement: STD-RCON-06 -- Convergence Latch; Reject Stale-Generation Writes

Concurrency and change detection SHALL use a monotonic `generation` /
`observed_generation` pair, not a per-write resource-version token. The API
server SHALL increment `generation` on any desired-spec change and SHALL treat
`observed_generation` as a monotonic convergence latch, accepting a new value
only in the range `current observed_generation <= new <= generation` and
rejecting any write that regresses below the current value or exceeds the current
`generation`. The control plane's provisioning gate SHALL suppress
re-provisioning **only when converged** (`observed_generation == generation`); a
`phase` of `Running`, `Provisioning`, or `Degraded` SHALL NOT, by itself,
suppress re-application.

**Verification:** Inspection -- refined and tested by PLAT-DATA-06, OSG-HLTH-03.

#### Scenario: STD-RCON-06.01 -- Spec change re-provisions regardless of phase

- GIVEN a converged resource whose `generation` then advances beyond `observed_generation`
- WHEN the control plane reconciles it
- THEN it SHALL re-apply regardless of the current `phase`
- AND on success it SHALL set `observed_generation` to the applied `generation`

#### Scenario: STD-RCON-06.02 -- Out-of-range convergence write is rejected

- GIVEN a resource with `generation` N+1 and `observed_generation` N
- WHEN a write sets `observed_generation` below N or above N+1
- THEN the API server SHALL reject the write
- AND `observed_generation` SHALL remain N

### Requirement: STD-RCON-07 -- Durable Deletion: Soft-Delete Plus Tombstone, Delete-Wins, Anti-Resurrection

Deletion SHALL be modeled on a soft-deleted PostgreSQL row plus a delete
tombstone and a delete event that carries the pre-delete resource snapshot, not
on a Kubernetes finalizer or `deletionTimestamp`. The delete SHALL be durable:
the reconcile queue SHALL retry a failed delete with bounded backoff until
finalization, and pending delete retries SHALL survive a watch reconnect and a
process restart (teardown reads the snapshot, not in-memory state). A delete
SHALL win event coalescing: a non-delete event SHALL NOT overwrite or resurrect
a pending delete, and after a successful delete the tombstone SHALL prevent a
stale inventory seed from re-provisioning the gone resource. Every deletion step
SHALL be idempotent and re-runnable.

**Verification:** Inspection -- refined and tested by GTW-DEL-05, GTW-DEL-08,
CTRL-WDEL-01.

#### Scenario: STD-RCON-07.01 -- Delete survives a restart via the snapshot

- GIVEN the control plane restarts with no in-memory state while a delete is pending
- WHEN it receives the delete event carrying the resource snapshot
- THEN it SHALL reclaim the owned resources from the snapshot
- AND it SHALL NOT depend on any in-memory cache

#### Scenario: STD-RCON-07.02 -- A stale seed cannot resurrect a deleted resource

- GIVEN a resource whose delete completed, leaving a tombstone
- WHEN a stale inventory seed still lists that resource
- THEN the control plane SHALL NOT re-provision it
- AND the tombstone SHALL be preserved

### Requirement: STD-RCON-08 -- Fail-Closed Destructive Actions; A Failed Read Means Unknown, Not Absent

A destructive action (deleting or pruning a resource) SHALL NOT be taken on the
basis of an unconfirmed or failed read. When a read that a teardown or prune
depends on cannot be confirmed (for example the API server or an external
dependency is unreachable), the control plane SHALL treat the result as
*unknown*, never as *absent*: it SHALL defer and retry rather than delete or skip
a resource from a stale or failed view. A delete event whose enrichment cannot be
loaded SHALL be surfaced and retried, not silently dropped.

**Verification:** Inspection -- refined by GTW-DEL-04; full code compliance is
tracked in the companion reconciliation non-compliance issue.

#### Scenario: STD-RCON-08.01 -- Unreachable dependency defers rather than assumes absence

- GIVEN a teardown or prune whose required read fails because a dependency is unreachable
- WHEN the control plane processes the reconcile
- THEN it SHALL NOT treat the unread state as "already absent"
- AND it SHALL return an error so the work is retried later
- AND it SHALL NOT delete or skip a resource based on the failed read

### Requirement: STD-RCON-09 -- Truthful Status; A Required-Operation Failure Is Never Reported as Success

Status written back to the API server SHALL reflect observed reality. A required
operation that failed SHALL NOT be reported as success or complete. When a
reconcile attempts several steps, failures SHALL be aggregated and either
propagated for retry or recorded through a durable, operator-visible signal; no
partial failure SHALL be silently swallowed. A resource SHALL NOT be reported
converged or finalized while a required operation for it remains unsatisfied.

**Verification:** Inspection -- refined by GTW-DEL-03 and OSG-HLTH-01; full code
compliance is tracked in the companion reconciliation non-compliance issue.

#### Scenario: STD-RCON-09.01 -- Partial failure is aggregated, not reported as success

- GIVEN a reconcile whose required step fails while others succeed
- WHEN the reconcile completes its attempt
- THEN it SHALL aggregate the failure and return an error so the work is retried
- AND it SHALL NOT report the resource complete while the required step is unsatisfied
- AND it SHALL NOT silently swallow the failure

### Requirement: STD-RCON-10 -- External Calls Are Bounded With Timeout and Context

Every external call a reconcile makes (the Kubernetes API, a gateway health
endpoint, Keycloak, a gateway database server, Vault, or the GitHub API) SHALL
carry the reconcile or stream context and a bounded timeout. A reconcile SHALL
NOT issue an unbounded blocking call, and SHALL NOT substitute `context.TODO()`
for the propagated context. A slow or hung external dependency for one key SHALL
NOT block other keys.

**Verification:** Inspection -- refined and tested by OSG-HLTH-04; the context
and no-`panic()` rules are in `standards/control-plane/conventions.spec.md`.

#### Scenario: STD-RCON-10.01 -- A hung dependency does not block other keys

- GIVEN one key whose external dependency does not answer before its timeout
- AND another key ready to reconcile
- WHEN the pool processes both
- THEN the call for the slow key SHALL abort at its bounded timeout
- AND the other key SHALL still be reconciled within the configured worker bound

### Requirement: STD-RCON-11 -- Single-Replica Model; No Cross-Process Locking or Leader Election

The control plane runs as a single replica per managed cluster. Per-key
serialization (STD-RCON-03) is an in-process guarantee only. Cross-process
locking and leader election are an explicit non-goal today; a reconciler SHALL
NOT assume multi-replica safety and SHALL NOT be written to depend on a leader
lease or a distributed lock. Making two replicas safe to reconcile the same key
(for example via a PostgreSQL advisory lock keyed by resource id) is a separate
high-availability decision outside this contract.

**Verification:** Inspection -- a stated architectural non-goal, consistent with
GTW-CONC and `control-plane.spec.md`; no test asserts a non-goal.

#### Scenario: STD-RCON-11.01 -- Per-key exclusion is in-process only

- GIVEN the control plane deployed as a single replica
- WHEN it serializes work for a key
- THEN the exclusion SHALL hold within that one process
- AND the contract SHALL NOT claim exclusion across two replicas

### Requirement: STD-RCON-12 -- Deviations Are Recorded Exceptions, Not Silent Divergence

A reconciler that deviates from any guarantee in this contract SHALL record the
deviation as a named exception in its own spec, with the rationale, so the
exception is a decision on the record rather than a silent divergence. The
currently recorded exception is the declarative `.hypershell` import: the
`RepositoryReconciler` runs inline on its watch-stream goroutine rather than
through the per-key reconcile queue, and recovers upstream drift through an hourly
conditional pass rather than a continuous resync (`control-plane.spec.md`,
CTRL-CORE-06).

**Verification:** Inspection -- the exception is cross-referenced to CTRL-CORE-06.

#### Scenario: STD-RCON-12.01 -- A new deviation is recorded where the reconciler is specified

- GIVEN a reconciler that must deviate from a guarantee in this contract
- WHEN its spec is written or changed
- THEN the deviation SHALL be stated as a named exception with its rationale
- AND it SHALL NOT be left as an unstated divergence from this contract

### Requirement: STD-RCON-13 -- New or Materially Changed Reconciliation Work Complies With This Contract

New reconciliation code, and a material change to existing reconciliation code,
SHALL comply with STD-RCON-01 through STD-RCON-11, or record an explicit
exception under STD-RCON-12. A reviewer SHALL check this contract when reviewing
a change that adds or materially changes a reconciler, covering Gateway,
RoleBinding, AgentRuntime, `Repository` import, service accounts, and per-gateway
databases. The control-plane review checklist in the `review-guidance` skill
carries this checkpoint.

**Verification:** Inspection -- enforced through review, recorded in the
`review-guidance` skill checklist.

#### Scenario: STD-RCON-13.01 -- A new reconciler is reviewed against the contract

- GIVEN a pull request that adds or materially changes a reconciler
- WHEN it is reviewed
- THEN the reviewer SHALL confirm it meets STD-RCON-01 through STD-RCON-11 or records an STD-RCON-12 exception
- AND a reconciler that meets neither SHALL NOT be approved

## Current Compliance Snapshot

This snapshot records how the code stood against this contract when the contract
was written (issue #709), so that a known divergence is a recorded decision, not
a silent one. It is a point-in-time audit, not part of the normative contract;
the requirements above are the desired state regardless of this column.

| Guarantee | Requirement | Code at adoption |
|-----------|-------------|------------------|
| Desired state authoritative; events schedule work | STD-RCON-01 | Compliant |
| Level-based and idempotent; drift repaired on a later pass | STD-RCON-02 | Compliant |
| Per-resource serialization and bounded worker pool | STD-RCON-03 | Compliant |
| Bounded exponential backoff retry | STD-RCON-04 | Compliant |
| Seed on (re)connect; periodic resync where required | STD-RCON-05 | Compliant (Repository import is the recorded exception, STD-RCON-12) |
| Convergence latch; reject stale-generation writes | STD-RCON-06 | Compliant |
| Durable deletion: soft-delete plus tombstone; delete-wins; anti-resurrection | STD-RCON-07 | Compliant |
| Fail-closed destructive actions; failed read means unknown, not absent | STD-RCON-08 | Partial (companion reconciliation non-compliance issue) |
| Truthful status; required-operation failure never reported as success | STD-RCON-09 | Partial (companion reconciliation non-compliance issue) |
| External calls bounded with timeout and context | STD-RCON-10 | Compliant |
| Single-replica; no leader election | STD-RCON-11 | Compliant (non-goal) |

## Design Decisions

| Decision | Rationale |
|----------|-----------|
| One normative contract referenced by every reconciliation spec | A reader assembled the guarantees from six files and a new reconciler author had no single bar to meet. Collecting them once, with each refining spec pointing back, gives one source of truth and a clear review checkpoint. |
| Describe the real model, superseding closed PR #200 | PR #200 proposed a contract against a Kubernetes finalizer, a per-write resource-version token, and leader election - none of which the system uses. A contract must match what is built or it misleads the author it exists to guide. |
| Soft-delete plus tombstone, not a finalizer | The resource's source of truth is PostgreSQL, not a Kubernetes object, so there is no object to carry a finalizer or `deletionTimestamp`. Durability comes from the soft-deleted row, the delete event snapshot, and the queue's bounded-backoff retry with an anti-resurrection tombstone (STD-RCON-07, GTW-DEL). |
| `generation` / `observed_generation` latch, not a resource-version token | Convergence is a monotonic range check the API server enforces on write-back, which both detects drift and rejects stale or reordered updates without a separate optimistic-concurrency token (STD-RCON-06, PLAT-DATA-06). |
| Single replica; leader election out of scope | The in-process queue already serializes per key within one process, and the deployment runs one replica. Cross-replica exclusion is a separate HA decision, kept an explicit non-goal so no reconciler is written to assume a leader lease (STD-RCON-11, GTW-CONC). |
| Record the compliance snapshot and known exceptions | Writing the real state, including the two partial items and the Repository-import exception, keeps a divergence a recorded decision rather than a surprise found later in review (STD-RCON-12). |

## References

- Issue #709 (this contract); supersedes closed PR #200, which proposed a
  contract against a finalizer and resource-version model the system does not use.
- Companion issue: reconciliation non-compliance fixes (STD-RCON-08, STD-RCON-09).
