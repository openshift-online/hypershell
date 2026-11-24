# ADR-0008: Control-Plane Reconciliation Metrics design decisions

Rationale relocated from [`specs/platform/control-plane-reconciliation-metrics.spec.md`](../../specs/platform/control-plane-reconciliation-metrics.spec.md) (OBS-CRM). The spec holds the normative behavior; this ADR holds the reasoning. Reference-only; not loaded by reconcile.

| Decision | Rationale |
| --- | --- |
| Four-value outcome taxonomy | Covers the full reconcile lifecycle without unbounded enumeration; maps cleanly to operator actions (noop = healthy, success = converged, retryable = wait, failed = investigate). |
| Closed reason-code enumeration | Prevents label cardinality from growing with fleet size or error diversity; new failure modes require a spec change and review. |
| Reason on retry/failed counters but not duration histogram | Duration x outcome is already 48 series (6 kinds x 2 event types x 4 outcomes); adding `reason` (9 codes: 5 retryable + 3 failed + `unknown`) would expand to 6 x 2 x 9 = 108 series total. Reason-correlated duration is available via trace spans (OBS-CPL-02). |
| `noop` records duration | Slow no-op paths (e.g. expensive phase-gate evaluation) are a real operational concern; silent omission hides them. |
| `unknown` fallback reason | New failure modes surface in dashboards immediately (via `unknown` spikes) rather than silently dropping observations until the spec is updated. |
| Backward compatibility with existing counters | `reconcile.errors` and `hypershell.reconciliation.failures` remain so existing dashboards and alerts continue to work; `reconcile.outcomes` is the forward-looking canonical metric. |
