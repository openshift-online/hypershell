# Control-Plane Reconciliation Metrics

**Status:** Draft
**Version:** 0.2.0
**Created:** 2026-09-23
**Updated:** 2026-09-28
**Tracks:** [HYPERSHELL-286](https://redhat.atlassian.net/browse/HYPERSHELL-286)

## Purpose

Expose consistent reconciliation outcomes across supported resource kinds so
that HyperShell operators and platform consumers can diagnose duration, retry,
and terminal-failure behavior through Prometheus and the web-console BFF
without introducing a dashboard database.

## Requirements

### Requirement: CRM-001 -- Prometheus Metric Types

The control plane SHALL expose the following fleet-wide Prometheus metrics:

| Metric | Type | Meaning | Unit |
| --- | --- | --- | --- |
| `hypershell_reconciliation_failures_total` | Counter | Failed reconciliation attempts | count |
| `hypershell_reconciliation_retries_total` | Counter | Reconciliation retries | count |
| `hypershell_reconciliation_lag_seconds` | Histogram | Time from reconciliation eligibility to completion | seconds |
| `hypershell_stale_resource_status_count` | Gauge | Resources whose observed status is stale | count |

The control plane SHALL expose the metrics through its Prometheus scrape
endpoint. The dashboard SHALL NOT read reconciliation data from a database.

### Requirement: CRM-002 -- Prometheus Labels

The counters and histogram SHALL use only bounded labels approved by the
control-plane observability conventions. The metrics SHALL NOT include resource
IDs, customer IDs, names, or other unbounded-cardinality labels.

The failure and retry counters MAY include a bounded reconciliation-kind label.
The lag histogram MAY include a bounded reconciliation-kind label. The stale
resource gauge MAY include a bounded resource-kind label.

### Requirement: CRM-003 -- Historical Query Contract

The BFF SHALL query the metrics through Prometheus `query_range` over the most
recent **24 hours** with a **3600-second** step.

The BFF SHALL calculate:

- failure counts from `increase(hypershell_reconciliation_failures_total[24h])` for current values and `increase(hypershell_reconciliation_failures_total[1h])` for hourly samples;
- retry counts from `increase(hypershell_reconciliation_retries_total[24h])` for current values and `increase(hypershell_reconciliation_retries_total[1h])` for hourly samples;
- median lag from `histogram_quantile(0.50, ...)` over the histogram buckets when
  a valid sample exists; and
- stale resource status count from the gauge value.

The BFF SHALL use the same aggregation dimensions for instant and range
queries. The BFF SHALL omit a historical series when Prometheus returns an
invalid or unavailable sample instead of converting the sample to zero.

### Requirement: CRM-004 -- BFF Response

The BFF SHALL expose `GET /api/metrics/control-plane-reconciliation` and SHALL
return current values plus optional historical arrays with UTC hourly labels.
The failure, retry, and stale-resource current queries are required and SHALL
return HTTP `502` when they fail. The current lag query is optional: when it
returns no valid sample, the response SHALL omit the lag field while preserving
the other current values. Failure of one historical query SHALL omit only that
historical array while preserving successful current values and sibling
historical arrays.

### Requirement: CRM-005 -- Verification

Tests SHALL verify metric registration, histogram bucket export, bounded label
sets, PromQL aggregation, 24-hour range parameters, response mapping, and
failure behavior.

### Requirement: CRM-006 -- Reconciliation Outcome Taxonomy

Every reconciliation attempt SHALL be classified into exactly one of the
following outcome values. The taxonomy is the single source of truth shared by
the duration histogram, the outcome counter, and any future metric that labels
by outcome.

| Outcome | Meaning |
| --- | --- |
| `noop` | The reconciler's `Handle` method ran but determined that no work was needed (deduplication, or desired state already matches observed state). |
| `success` | The reconciler completed all work and the resource converged to its desired state. |
| `retryable` | The reconciler failed due to a transient condition (network timeout, API unavailable, conflict) and the queue will schedule a retry with backoff. |
| `failed` | The reconciler failed due to a condition that cannot recover without external intervention (invalid configuration, missing prerequisite, validation failure). The resource phase moves to `Failed`. |

A reconciliation that is skipped before the reconciler's `Handle` method
executes (e.g. a phase-gated event that the watch loop or queue filters out
before calling `Handle`, producing no span per CP-OBS-02) SHALL NOT record any
outcome observation. The `noop` outcome is distinct from this silent skip:
`noop` means `Handle` ran, evaluated the resource, and decided no mutation was
needed. Only code paths inside `Handle` record outcomes.

These outcome values SHALL be string constants defined once in the control-plane
`otel` package and imported by every call site. Adding a new outcome value
requires updating this spec.

### Requirement: CRM-007 -- Bounded Reason Codes

Each `retryable` or `failed` outcome SHALL carry a bounded `reason` label
that aids diagnosis without leaking sensitive data. The reason code set is a
closed enumeration; adding a new code requires updating this spec.

#### Retryable Reason Codes

| Code | When |
| --- | --- |
| `grpc_unavailable` | The HyperShell API server gRPC call returned Unavailable, DeadlineExceeded, or a transport error. |
| `k8s_conflict` | A Kubernetes API write returned Conflict (optimistic-concurrency retry). |
| `k8s_unavailable` | A Kubernetes API call returned a transient server error (5xx, timeout). |
| `dependency_not_ready` | A prerequisite resource (release, namespace, secret) is not yet available. |
| `keycloak_transient` | A Keycloak API call failed with a transient HTTP error. |

#### Failed Reason Codes

| Code | When |
| --- | --- |
| `invalid_config` | The resource's desired state failed validation (malformed image, invalid OIDC, bad route config, missing namespace on a persisted gateway). |
| `missing_prerequisite` | A required dependency does not exist and will not appear without operator action (e.g. a referenced release is deleted). |
| `identity_invalid` | A persisted identity (Keycloak client ID, OIDC audience) failed integrity validation. |

Reason codes SHALL NOT contain resource identifiers, customer identifiers,
namespace names, error messages, or any value whose cardinality grows with
fleet size. Raw error strings SHALL NOT appear as label values. A reconcile
failure that does not match a recognized code SHALL use `unknown` as the reason
and log the detail at ERROR level.

### Requirement: CRM-008 -- Outcome-Labeled Reconciliation Duration

The existing `reconcile.duration` histogram (CP-OBS-07) SHALL add an `outcome`
attribute from the taxonomy defined in CRM-006. The histogram SHALL continue
to carry `resource.kind` and `event.type` attributes. The full attribute set
for each observation SHALL be:

| Attribute | Values | Source |
| --- | --- | --- |
| `resource.kind` | `Gateway`, `GatewayRelease`, `GatewayNetwork`, `ManagedCluster`, `RoleBinding` | Reconciler kind |
| `event.type` | `reconcile`, `delete` | Watch event type |
| `outcome` | `noop`, `success`, `retryable`, `failed` | CRM-006 |

The `reason` attribute (CRM-007) SHALL NOT appear on the duration histogram
to keep cardinality bounded. Duration with reason can be correlated through
traces (CP-OBS-02).

A `noop` observation SHALL still record the time spent evaluating the no-op
decision so operators can detect slow no-op paths.

### Requirement: CRM-009 -- Reconciliation Outcome Counter

The control plane SHALL expose a counter metric:

| Metric | Type | Unit | Description |
| --- | --- | --- | --- |
| `reconcile.outcomes` | Counter | `{outcome}` | Count of reconciliation attempts by outcome |

The counter SHALL carry the following bounded attributes:

| Attribute | Values |
| --- | --- |
| `resource.kind` | `Gateway`, `GatewayRelease`, `GatewayNetwork`, `ManagedCluster`, `RoleBinding` |
| `outcome` | `noop`, `success`, `retryable`, `failed` |

This counter complements `reconcile.errors` (CP-OBS-07) by tracking all
outcomes, not only failures. `reconcile.errors` remains for backward
compatibility; its increment SHALL coincide with either `retryable` or
`failed` increments on `reconcile.outcomes`.

### Requirement: CRM-010 -- Retry Counter with Reason

The existing `hypershell.reconciliation.retries` counter (CRM-001) and the
OTLP retry recording path SHALL add a bounded `reason` attribute from the
retryable reason codes in CRM-007. The counter SHALL continue to carry the
`resource.kind` attribute. The full attribute set SHALL be:

| Attribute | Values |
| --- | --- |
| `resource.kind` | `Gateway`, `GatewayRelease`, `GatewayNetwork`, `ManagedCluster`, `RoleBinding` |
| `reason` | Retryable codes from CRM-007, or `unknown` |

### Requirement: CRM-011 -- Failed Outcome Counter

The control plane SHALL expose a counter metric for reconciliation failures
that cannot self-recover:

| Metric | Type | Unit | Description |
| --- | --- | --- | --- |
| `reconcile.failed` | Counter | `{failure}` | Count of reconciliation attempts that reached a non-recoverable state |

The counter SHALL carry the following bounded attributes:

| Attribute | Values |
| --- | --- |
| `resource.kind` | `Gateway`, `GatewayRelease`, `GatewayNetwork`, `ManagedCluster`, `RoleBinding` |
| `reason` | Failed codes from CRM-007, or `unknown` |

When exported to Prometheus, the counter SHALL appear as
`reconcile_failed_total`. A `failed` outcome observation SHALL also increment
`reconcile.outcomes` with `outcome=failed`,
`hypershell.reconciliation.failures` (CRM-001), and `reconcile.errors`
(CP-OBS-07). The four increments SHALL happen atomically from the caller's
perspective.

### Requirement: CRM-012 -- Cardinality Policy

The total label-value cardinality for any single metric SHALL NOT exceed the
product of its bounded attribute domains. Specifically:

- `reconcile.duration`: at most `|resource.kind| x |event.type| x |outcome|` = 5 x 2 x 4 = 40 series.
- `reconcile.outcomes`: at most `|resource.kind| x |outcome|` = 5 x 4 = 20 series.
- `reconcile.failed`: at most `|resource.kind| x |reason|` = 5 x 4 = 20 series (3 failed codes + `unknown`).
- `hypershell.reconciliation.retries`: at most `|resource.kind| x |reason|` = 5 x 6 = 30 series (5 retryable codes + `unknown`).

The retryable codes are: `grpc_unavailable`, `k8s_conflict`, `k8s_unavailable`, `dependency_not_ready`, `keycloak_transient`.

No metric defined in this spec SHALL accept a label value outside its declared
domain. The recording functions SHALL validate the outcome and reason values at
the call site; an unrecognized value SHALL be replaced with `unknown` and
logged at WARN level.

### Requirement: CRM-013 -- Verification of Outcome Metrics

Tests SHALL verify:

- Each outcome value (`noop`, `success`, `retryable`, `failed`) produces
  exactly one observation on `reconcile.outcomes` and one observation on
  `reconcile.duration` with the matching outcome attribute.
- A `retryable` outcome increments `hypershell.reconciliation.retries` with a
  valid reason code and increments `reconcile.errors`.
- A `failed` outcome increments `reconcile.failed` with a valid reason code,
  increments `reconcile.errors`, and increments
  `hypershell.reconciliation.failures`.
- A `noop` outcome does NOT increment `reconcile.errors`,
  `hypershell.reconciliation.failures`, or `hypershell.reconciliation.retries`.
- A `success` outcome does NOT increment any failure or retry counter.
- No metric observation contains a resource identifier, namespace name, or raw
  error string as a label value.
- The total series count for each metric does not exceed the cardinality bound
  in CRM-012.

#### Scenario: Successful gateway reconciliation

- GIVEN a Gateway in phase `Pending`
- WHEN the reconciler provisions it and transitions it to `Running`
- THEN `reconcile.outcomes` SHALL increment with `resource.kind=Gateway`, `outcome=success`
- AND `reconcile.duration` SHALL record the duration with `outcome=success`
- AND `reconcile.errors` SHALL NOT be incremented

#### Scenario: No-op reconciliation

- GIVEN a Gateway whose desired state matches its observed state
- WHEN the reconciler's `Handle` method evaluates the resource and determines no mutation is needed
- THEN `reconcile.outcomes` SHALL increment with `resource.kind=Gateway`, `outcome=noop`
- AND `reconcile.duration` SHALL record the evaluation time with `outcome=noop`
- AND `reconcile.errors` SHALL NOT be incremented

#### Scenario: Retryable failure with reason

- GIVEN a Gateway reconciliation in progress
- WHEN the HyperShell API server gRPC call returns Unavailable
- THEN `reconcile.outcomes` SHALL increment with `resource.kind=Gateway`, `outcome=retryable`
- AND `reconcile.duration` SHALL record the duration with `outcome=retryable`
- AND `hypershell.reconciliation.retries` SHALL increment with `resource.kind=Gateway`, `reason=grpc_unavailable`
- AND `reconcile.errors` SHALL be incremented

#### Scenario: Failed outcome with reason

- GIVEN a Gateway with an invalid image reference
- WHEN the reconciler validates the configuration and cannot proceed
- THEN `reconcile.outcomes` SHALL increment with `resource.kind=Gateway`, `outcome=failed`
- AND `reconcile.duration` SHALL record the duration with `outcome=failed`
- AND `reconcile.failed` SHALL increment with `resource.kind=Gateway`, `reason=invalid_config`
- AND `hypershell.reconciliation.failures` SHALL be incremented
- AND `reconcile.errors` SHALL be incremented
- AND the Gateway phase SHALL transition to `Failed`

#### Scenario: Unknown reason code fallback

- GIVEN a reconciliation failure that does not match any recognized reason code
- WHEN the outcome is recorded
- THEN the reason attribute SHALL be `unknown`
- AND the failure detail SHALL be logged at ERROR level
- AND no raw error string SHALL appear in the metric label

## Assumptions

- Prometheus retention is at least 24 hours.
- HYPERSHELL-285 API reliability metrics are available to the dashboard. This
  specification intentionally reuses and standardizes their shared dashboard
  presentation as `requests/sec` with three fractional digits; these are
  compatibility changes to the existing reliability-dashboard contract.
- The outcome taxonomy (CRM-006) is authoritative for all reconciliation
  metrics. `control-plane-observability.spec.md` CP-OBS-07 defines the OTLP
  instruments; this spec defines their semantic contract.
- Failed reason codes align with the control-plane conventions
  (`standards/control-plane/conventions.spec.md`): non-recoverable errors update
  the resource status to `Failed` and do not retry.

## Design Decisions

| Decision | Rationale |
| --- | --- |
| Four-value outcome taxonomy | Covers the full reconcile lifecycle without unbounded enumeration; maps cleanly to operator actions (noop = healthy, success = converged, retryable = wait, failed = investigate). |
| Closed reason-code enumeration | Prevents label cardinality from growing with fleet size or error diversity; new failure modes require a spec change and review. |
| Reason on retry/failed counters but not duration histogram | Duration x outcome is already 40 series (5 kinds x 2 event types x 4 outcomes); adding `reason` (9 codes: 5 retryable + 3 failed + `unknown`) would expand to 5 x 2 x 9 = 90 series total. Reason-correlated duration is available via trace spans (CP-OBS-02). |
| `noop` records duration | Slow no-op paths (e.g. expensive phase-gate evaluation) are a real operational concern; silent omission hides them. |
| `unknown` fallback reason | New failure modes surface in dashboards immediately (via `unknown` spikes) rather than silently dropping observations until the spec is updated. |
| Backward compatibility with existing counters | `reconcile.errors` and `hypershell.reconciliation.failures` remain so existing dashboards and alerts continue to work; `reconcile.outcomes` is the forward-looking canonical metric. |
