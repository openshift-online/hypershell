# ADR-0022: OpenShell Gateway Active Sandbox Count design decisions

Rationale relocated from [`specs/platform/openshell-gateway-sandbox-count.spec.md`](../../specs/platform/openshell-gateway-sandbox-count.spec.md) (OSG-SBX). The spec holds the normative behavior; this ADR holds the reasoning. Reference-only; not loaded by reconcile.

| Decision | Rationale |
|----------|-----------|
| Maintain the count from a sandbox pod watch, not a periodic full-namespace LIST | A repeated LIST of every pod in every gateway namespace on a fixed cadence is a costly poll of Kubernetes that scales with the fleet. A single informer over sandbox pods delivers create/update/delete events and keeps a local cache, so the count stays current without repeatedly asking the API server. |
| Event source is a control-plane watch, not a controller callback | The agent-sandbox controller that creates sandbox pods is upstream (`kubernetes-sigs/agent-sandbox`) and cannot call the HyperShell API. The only reliable, in-house signal for sandbox lifecycle is a control-plane watch on the pods it can already observe. |
| Attribute sandboxes to a Gateway by namespace | Sandbox pods carry no back-reference to their Gateway; the gateway namespace (`openshell-<id>`) is the sole, deterministic linkage, set at Gateway creation. |
| Count active as `Running` or `Pending` pods | This preserves the existing meaning of `active_sandbox_count` and matches what an operator cares about before a disruptive action: sessions that are up or coming up. |
| Adjust the count atomically at the source of truth, floored at zero | Increments and decrements arrive concurrently from independent pod events; a read-modify-write would lose updates. An atomic adjustment with a zero floor keeps the stored value correct and never negative even under duplicate or out-of-order events. |
| Self-heal by reconciling to the watch cache, not by polling | Incremental deltas can drift after a missed event or a restart. Periodically setting the count to the absolute number observed in the in-memory cache converges it to the truth without reintroducing a Kubernetes poll. |
| Keep the field control-plane-owned and read-only | The count is an observed fact about cluster state, not user intent; only the control plane observes it, so only the control plane writes it. |
