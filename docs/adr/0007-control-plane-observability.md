# ADR-0007: Control Plane Observability design decisions

Rationale relocated from [`specs/platform/control-plane-observability.spec.md`](../../specs/platform/control-plane-observability.spec.md) (OBS-CPL). The spec holds the normative behavior; this ADR holds the reasoning. Reference-only; not loaded by reconcile.

| Decision | Rationale |
| --- | --- |
| Same OTel environment variables as the API server | Consistency across the platform; operators configure both components the same way |
| Opt-in by collector-endpoint presence | Zero overhead when no collector is configured; no code change needed to enable in production |
| Parent-based trace-id-ratio sampler | Preserves upstream sampling decisions; only root spans are subject to the ratio |
| SDK init in main() before goroutines | The control plane has no framework plugin system; explicit init is the simplest correct approach |
| One span per reconcile Handle() | The reconciler is the unit of work; sub-spans for gRPC and K8s calls nest naturally as children |
| gRPC client interceptors on the shared connection | Every service client created from the connection inherits instrumentation without per-call changes |
| otelhttp transport wrapper for client-go | Instruments all Kubernetes API calls transparently without modifying reconciler code |
| Bounded span names by kind, not resource ID | Keeps Jaeger grouping useful and prevents cardinality explosion across large deployments |
| Resource ID as a span attribute, not a span name | Enables per-trace debugging without inflating the span-name namespace |
| Queue depth and wait use the shared reconcile queue | One bounded resource-kind label covers each shared reconcile queue without resource identifiers |
| OTLP/gRPC on port 4317 for the control plane | Matches the API server's transport; the development Jaeger exposes 4317 for OTLP/gRPC |
| Reconcile-trace to request-trace correlation via span links | Reconciliation is asynchronous; the correlation mechanism (span links, trace-context persistence) is defined in `platform/reconcile-trace-correlation.spec.md` |
