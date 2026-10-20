# Minimum Usable Experience SLI Specification

**Status:** Draft
**Tracks:** HYPERSHELL-432
**Applies to:** `components/api-server`, `components/control-plane`, the
Prometheus metrics pipeline, and SLI recording rules

## Purpose

Define the first service level indicators (SLIs) from the user's minimum usable
experience. A user must be able to get a Gateway, get a Sandbox in that Gateway,
keep the Sandbox usable, and trust the state that HyperShell reports. The
indicators measure those effects at the user-consumption boundary where possible.
They do not make infrastructure causes into SLIs. This specification defines the
baseline-collection contract only; it does not define an SLO target, error budget,
or alert threshold.

## Domain Vocabulary

- **Acquisition**: acceptance of a new Gateway create request. It ends when the
  API server accepts the request, not when the Gateway reaches `Running`.
- **Gateway establishment**: the interval from Gateway creation to its first
  successful `Running` transition.
- **Sandbox establishment**: the interval from Sandbox CR creation to the
  upstream Sandbox `Ready=True` condition.
- **Continuity departure**: an active Sandbox that leaves the active set.
- **State-trust observation**: one sandbox-count self-heal comparison between
  the count observed in the pod watch cache and the count reported by the API
  server.
- **SLI window**: fifteen minutes. This is a measurement boundary, not an SLO
  target. It matches the existing Gateway provisioning histogram's largest
  explicit bucket. A changed window requires a specification update and matching
  histogram buckets and recording rules.

The five initial indicators are:

| ID | User experience | Good event | Raw evidence |
| --- | --- | --- | --- |
| 0a | I can get a Gateway (accepted) | A Gateway create request is accepted | API-server Gateway create path |
| 0b | I can get a Gateway (established) | The Gateway first reaches `Running` within the SLI window | `gateway.provision.duration` and `gateway.provision.outcomes` |
| 1 | I can get a Sandbox | The Sandbox first reports `Ready=True` within the SLI window | Sandbox CR `status.conditions` |
| 2 | The Sandbox stays usable while I work | An active Sandbox departure is expected | Active-set departure observation |
| 3 | What I am told is true | Observed and API-reported active Sandbox counts are equal | Sandbox-count self-heal comparison |

A user-visible event is either good or poor. An absent, malformed, or unavailable
measurement is **unknown**; it SHALL NOT be recorded as a good event or silently
converted to zero. Supporting metrics such as certificate expiry, Keycloak
availability, queue depth, and reconciliation errors MAY help diagnosis, but
SHALL NOT substitute for these SLIs.

## Requirements

### Requirement: MUE-SLI-01 -- SLI Measurement and Baseline Policy

The platform SHALL collect all five indicators as bounded-cardinality Prometheus
metrics and recording rules. The metrics SHALL measure user-visible effects,
not an inferred infrastructure cause. No SLI metric label SHALL include a
Gateway ID, Sandbox ID, namespace, user ID, request ID, or raw error text.

The platform SHALL collect a baseline for at least two and at most four weeks
before proposing an SLO target. During that period, dashboards and reports MAY
show counts, rates, and latency distributions, but SHALL NOT represent a
percentage, percentile, or the fifteen-minute SLI window as an approved SLO,
error budget, or alert threshold.

A metric pipeline failure, an unavailable Sandbox CR, or an unknown condition
SHALL remain distinguishable from a measured poor event. Recording rules SHALL
preserve that distinction by producing no fabricated success, failure, or zero
sample for missing input.

#### Scenario: Baseline data does not create an SLO

- GIVEN the five indicator recording rules have collected data for ten days
- WHEN an operator views the baseline
- THEN the operator MAY view event counts, rates, and duration distributions
- AND the platform SHALL NOT claim an SLO target or error budget for any indicator

#### Scenario: Missing Sandbox evidence is not success

- GIVEN an active sandbox pod has no readable matching Sandbox CR
- WHEN the platform evaluates Sandbox establishment evidence
- THEN it SHALL record no Sandbox establishment outcome for that observation
- AND it SHALL NOT record `Ready=True` or a synthetic failure

---

### Requirement: MUE-SLI-02 -- Gateway Acquisition and Establishment

The API server SHALL record `gateway.acquisition.outcomes` for Gateway create
requests with the closed `outcome` attribute set:

| Outcome | Meaning |
| --- | --- |
| `accepted` | The Gateway record was created successfully. |
| `quota_exceeded` | The request was rejected by a Gateway creation quota. |
| `placement_unavailable` | The request was rejected because an explicitly requested `local-kind` placement is unavailable. |
| `authz_denied` | Authorization denied the create request. |

The accepted event SHALL occur only after the Gateway create transaction succeeds.
A validation, transport, or internal error that has no outcome in this closed set
SHALL NOT be reclassified as `accepted`, `quota_exceeded`, `placement_unavailable`,
or `authz_denied`; it is supporting API reliability evidence. The metric SHALL
be emitted at the API-server Gateway create and authorization boundaries, so it
covers both REST and gRPC create entry points.

A managed-selector request with no eligible cluster SHALL be `accepted`, not
`placement_unavailable`: GP-07 creates that Gateway in `Pending` and later binds
it. The `placement_unavailable` outcome applies only to the rejected local-kind
case defined by GP-08.

Gateway establishment SHALL use the existing one-per-Gateway
`gateway.provision.duration` histogram and `gateway.provision.outcomes` counter.
A good establishment event is the first successful `Running` transition whose
create-to-`Running` duration is at most fifteen minutes. A first `Failed`
transition or a first `Running` transition that exceeds fifteen minutes is poor.
A later recovery from `Degraded` to `Running` SHALL NOT create another acquisition
or establishment event.

The existing Gateway provision duration and outcomes metrics remain the authority
for their raw data and operational-dashboard presentation. This requirement
makes them the raw evidence for indicator 0b; it does not change their existing
one-per-Gateway semantics.

#### Scenario: Late binding is an accepted acquisition

- GIVEN a valid managed placement selector with no eligible connected cluster
- WHEN the API server creates the Gateway in `Pending` awaiting placement
- THEN `gateway.acquisition.outcomes` SHALL increment with `outcome=accepted`
- AND it SHALL NOT increment with `outcome=placement_unavailable`

#### Scenario: Gateway reaches Running within the SLI window

- GIVEN a newly created Gateway has not recorded a terminal provision outcome
- WHEN it first reaches `Running` twelve minutes after creation
- THEN `gateway.provision.duration` SHALL contain the twelve-minute observation
- AND indicator 0b SHALL count one good establishment event

#### Scenario: Gateway reaches Running after the SLI window

- GIVEN a newly created Gateway has not recorded a terminal provision outcome
- WHEN it first reaches `Running` sixteen minutes after creation
- THEN `gateway.provision.duration` and the successful provision outcome SHALL still be recorded
- AND indicator 0b SHALL count one poor establishment event

---

### Requirement: MUE-SLI-03 -- Sandbox Establishment Uses Ready

Sandbox establishment SHALL be measured from Sandbox CR creation to the first
`Ready=True` condition in the upstream `agents.x-k8s.io/v1beta1` Sandbox CR
`status.conditions` array. The control plane SHALL read that condition through
the existing unstructured Sandbox CR access pattern. It SHALL use the condition's
`lastTransitionTime` as the duration endpoint and the Sandbox CR creation time
as the start time.

A pod in `Running` phase is not a Sandbox establishment success. The good event
is `Ready=True` within fifteen minutes. In upstream agent-sandbox v1beta1,
`Ready=True` requires the backing pod to be Running and PodReady, to have a pod
IP, and, when requested, to have a Service. The control plane SHALL NOT replace
this condition with a pod-Running proxy.

The control plane SHALL export exactly one terminal
`gateway.sandbox.establishment.outcomes` observation for each Sandbox creation.
Its bounded `outcome` value SHALL be one of:

| Outcome | Meaning |
| --- | --- |
| `ready` | The first `Ready=True` condition occurred within the SLI window. |
| `dependencies_not_ready` | At the SLI window end, `Ready=False` has reason `DependenciesNotReady`. |
| `reconciler_error` | At the SLI window end, `Ready=False` has reason `ReconcilerError`. |
| `invalid_configuration` | At the SLI window end, `Ready=False` has reason `InvalidConfiguration`. |
| `sandbox_suspended` | At the SLI window end, `Ready=False` has reason `SandboxSuspended`. |
| `window_expired` | At the SLI window end, Ready is absent, unknown, or has another reason. |

The four named `Ready=False` reasons SHALL map to the matching lower-snake-case
failure outcome. A Sandbox that first becomes Ready after the SLI window SHALL
record `window_expired`, not `ready`. The implementation SHALL preserve the
one-terminal-outcome rule across restarts without using an unbounded metric
attribute. The duration histogram `gateway.sandbox.establishment.duration` SHALL
record only the `ready` event and SHALL use explicit buckets that include fifteen
minutes.

Pod add and update events in the sandbox-count path MAY trigger a correlated
Sandbox CR read, but `classifyPod` and pod phase SHALL be diagnostic evidence
only. They SHALL NOT determine the SLI outcome.

#### Scenario: Ready is stronger than pod Running

- GIVEN a Sandbox backing pod is `Running`
- AND the Sandbox CR does not report `Ready=True`
- WHEN the control plane evaluates Sandbox establishment
- THEN it SHALL NOT record `outcome=ready`

#### Scenario: Ready within the window is good

- GIVEN a Sandbox CR was created at 10:00 UTC
- AND its first `Ready=True` condition has `lastTransitionTime` 10:08 UTC
- WHEN the control plane records the terminal establishment outcome
- THEN it SHALL increment `gateway.sandbox.establishment.outcomes` with `outcome=ready`
- AND record an eight-minute establishment duration

#### Scenario: A Ready false reason is a poor outcome

- GIVEN a Sandbox has not reached `Ready=True` by fifteen minutes after creation
- AND its Ready condition is `False` with reason `InvalidConfiguration`
- WHEN the control plane records the terminal establishment outcome
- THEN it SHALL increment `gateway.sandbox.establishment.outcomes` with `outcome=invalid_configuration`
- AND it SHALL not record an establishment duration

---

### Requirement: MUE-SLI-04 -- Sandbox Continuity

The control plane SHALL classify every departure returned by
`ReconcileActiveSandboxes()` exactly once as expected or unexpected and SHALL
export `gateway.sandbox.continuity.outcomes` with bounded attributes `outcome`
and `reason`.

| Outcome | Reason | Meaning |
| --- | --- | --- |
| `expected` | `shutdown_time` | The observed Sandbox CR has a `spec.lifecycle.shutdownTime` at or before the departure evaluation time. |
| `expected` | `succeeded` | The Sandbox pod completed successfully. |
| `unexpected` | `crash_loop_back_off` | A container is in CrashLoopBackOff. |
| `unexpected` | `failed` | The pod phase is Failed. |
| `unexpected` | `evicted` | The pod was evicted. |
| `unexpected` | `unknown` | No expected-departure evidence or known unexpected reason exists. |

An expected departure is the good event for indicator 2. An unexpected departure
is a poor event. `shutdownTime` is expected only when it is due at departure;
a future shutdown time SHALL NOT make an early departure expected. A succeeded
pod is expected even if `shutdownTime` is absent. A missing or unreadable Sandbox
CR SHALL use the bounded `unexpected/unknown` classification rather than an
unbounded error label.

This indicator is distinct from `gateway.sandbox.lifetime`: the lifetime
histogram measures elapsed active lifetime, while continuity classifies whether
the departure was expected. Both may use the same departure observation.

#### Scenario: Scheduled Sandbox shutdown is expected

- GIVEN an active Sandbox has a shutdown time in the past
- WHEN it leaves the active set
- THEN the continuity counter SHALL record `outcome=expected`, `reason=shutdown_time`

#### Scenario: Crash looping Sandbox departure is unexpected

- GIVEN an active Sandbox leaves the active set
- AND its last observed pod state is CrashLoopBackOff
- WHEN the continuity counter is recorded
- THEN it SHALL record `outcome=unexpected`, `reason=crash_loop_back_off`

---

### Requirement: MUE-SLI-05 -- State Trust

The sandbox-count reconciler SHALL compare, for every self-heal pass and Gateway
namespace, the active Sandbox count observed from its synchronized informer cache
with the count stored and reported by the API server before it applies an absolute
correction. It SHALL export one
`gateway.sandbox.count.reconciliation.outcomes` counter observation with the
closed `outcome` attribute:

| Outcome | Meaning |
| --- | --- |
| `matched` | The observed count equals the API-reported count; this is the good event. |
| `corrected` | The counts differ and the self-heal applies the observed count; this is the poor event. |

The comparison SHALL use the same active-Sandbox definition as
`active_sandbox_count`: a labeled Sandbox pod in `Pending` or `Running` phase.
A failed API read or write SHALL NOT record `matched`; it is an unknown
measurement and SHALL be logged or counted only by supporting reconciliation
telemetry. Incremental `applyDelta` updates remain an accounting mechanism, not
a state-trust success assertion.

The state-trust rate SHALL use `matched / (matched + corrected)` over recording
rule evaluation periods. The correction counter is the required drift-event
count. It SHALL NOT include a Gateway, Sandbox, or namespace label.

#### Scenario: Self-heal finds no drift

- GIVEN the informer cache observes three active Sandboxes for a Gateway namespace
- AND the API server reports `active_sandbox_count = 3`
- WHEN the self-heal compares the counts
- THEN it SHALL increment the state-trust counter with `outcome=matched`
- AND it SHALL not apply a correction

#### Scenario: Self-heal corrects drift

- GIVEN the informer cache observes three active Sandboxes for a Gateway namespace
- AND the API server reports `active_sandbox_count = 1`
- WHEN the self-heal compares the counts
- THEN it SHALL increment the state-trust counter with `outcome=corrected`
- AND it SHALL set the reported count to `3`

---

### Requirement: MUE-SLI-06 -- Prometheus Recording Rules and Verification

The deployed Prometheus configuration SHALL contain recording rules for all five
indicators. The rules SHALL calculate the rate numerator and denominator from
raw event metrics over a documented rolling range and SHALL retain raw metrics
for diagnosis. They SHALL use the following logical expressions, with a common
five-minute rate range unless a query explicitly uses the fifteen-minute
establishment bucket:

| Indicator | Good numerator | Denominator |
| --- | --- | --- |
| 0a | `gateway_acquisition_outcomes_total{outcome="accepted"}` | all four acquisition outcomes |
| 0b | `gateway_provision_duration_seconds_bucket{le="900"}` | `gateway_provision_outcomes_total{outcome="success"}` plus `{outcome="failure"}` |
| 1 | `gateway_sandbox_establishment_outcomes_total{outcome="ready"}` | all Sandbox establishment outcomes |
| 2 | `gateway_sandbox_continuity_outcomes_total{outcome="expected"}` | all continuity outcomes |
| 3 | `gateway_sandbox_count_reconciliation_outcomes_total{outcome="matched"}` | all state-trust outcomes |

The recording rules SHALL use a zero-safe denominator and SHALL expose a missing
or no-observation period as unavailable rather than a one-hundred-percent rate.
For indicator 0b, each first `Running` event contributes one outcome in the
denominator; only its duration-bucket observation can contribute to the good
numerator. Thus a failed Gateway and a Gateway that reaches `Running` after
fifteen minutes are both poor events.

Tests SHALL verify the condition parsing and event classification, the one-event
semantics, metric names and allowed label values, absence of identifier labels,
and each recording rule's good/poor arithmetic. Tests SHALL include a pod that
is Running before its Sandbox is Ready, an overdue Ready=False condition, an
expected shutdown, a CrashLoopBackOff departure, and a sandbox-count correction.

#### Scenario: Recording rule does not call no traffic perfect

- GIVEN no acquisition outcome sample exists in a recording-rule evaluation range
- WHEN the acquisition success-rate rule evaluates
- THEN it SHALL be unavailable
- AND it SHALL NOT emit a value of one hundred percent

## Non-Goals

- Setting an SLO target, error budget, alert threshold, or paging policy.
- Using certificate expiry, Keycloak availability, queue depth, or generic API
  error rate as a substitute for a minimum-experience SLI.
- Treating a Sandbox pod in `Running` phase as equivalent to Sandbox `Ready=True`.
- Changing Gateway late-binding behavior when a managed placement selector has no
  eligible cluster.
- Adding Gateway, Sandbox, namespace, user, or request identifiers to SLI metric
  labels.

## Cross-References

- `platform/gateway-provision-time.spec.md` and
  `platform/gateway-provision-outcomes.spec.md` own the existing raw evidence for
  indicator 0b and its operational-dashboard presentation.
- `platform/openshell-gateway-sandbox-count.spec.md` owns active-count semantics
  and self-heal mechanics.
- `web-console/usage-dashboard.spec.md` owns the separate sandbox-lifetime
  histogram.
- `web-console/gateway-provision-placement.spec.md` GP-07 and GP-08 define
  managed late binding and local-kind rejection.
- `platform/control-plane-observability.spec.md` defines the control-plane OTel
  instrument registration and export pipeline.
