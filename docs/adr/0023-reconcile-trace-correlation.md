# ADR-0023: Reconcile-to-Request Trace Correlation design decisions

Rationale relocated from [`specs/platform/reconcile-trace-correlation.spec.md`](../../specs/platform/reconcile-trace-correlation.spec.md) (CTRL-TRC). The spec holds the normative behavior; this ADR holds the reasoning. Reference-only; not loaded by reconcile.

| Decision | Rationale |
| --- | --- |
| Span link, not parent-child | Reconciliation is asynchronous and may happen long after the request returns. A parent-child relationship would create a span tree where the child outlives the parent, breaking trace semantics. A span link preserves causal reference without implying temporal containment. |
| Local `TraceMeta` embed alongside `api.Meta` | The upstream `api.Meta` is owned by the `rh-trex-ai` framework and cannot be modified in-tree. A local embeddable struct keeps the change self-contained. Each plugin model embeds it, and a single migration adds the columns to all tables. |
| `traceparent` and `tracestate` as separate text columns | Matches the W3C Trace Context header structure. Two columns are simpler than a JSON blob and allow direct extraction without parsing. |
| Fields on `ObjectReference` in protobuf, not on each resource message | `ObjectReference` is the shared metadata message embedded in every resource. Adding the fields there propagates to all watch responses without per-resource proto changes. |
| Capture from active span context, not raw header | The active span context reflects the actual sampled span that performed the write. The raw header might not match if the server started a new root or the header was malformed. |
| Overwrite on update, not append | The most recent mutation is the one the control plane will reconcile. Maintaining a history of trace contexts would complicate the schema and provide limited value. |
| Silent ignore on missing or malformed traceparent | Pre-existing resources and disabled-telemetry deployments must work without errors. The link is best-effort observability, not a correctness requirement. |
| Trace context not exposed in REST API | The trace context is an internal observability concern. Exposing it in the REST API would leak infrastructure details and create an unnecessary contract. |

## Additional rationale

- Motivation (relocated from the spec Purpose): when an operator investigates a reconcile failure they need the causal chain - which user action triggered the change the control plane is now reconciling.
- The control plane already produces reconcile spans (OBS-CPL-02) and the API server produces request spans (API-OBS-02/03), but the two are disconnected because reconciliation is temporally decoupled: the control plane observes the change later via the gRPC watch stream, possibly after resync, batching, or retries.
- A synchronous parent-child relationship would produce a child span that starts long after its parent ended, or one request span with many reconcile children from resyncs; the span link preserves the causal reference without implying temporal containment.
