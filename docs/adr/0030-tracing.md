# ADR-0030: Web Console Distributed Tracing design decisions

Rationale relocated from [`specs/web-console/tracing.spec.md`](../../specs/web-console/tracing.spec.md) (WEB-TRC). The spec holds the normative behavior; this ADR holds the reasoning. Reference-only; not loaded by reconcile.

| Decision | Rationale |
| --- | --- |
| Derive spans from domain probes, not auto-instrumentation | Keeps one authoritative telemetry model, satisfies the domain-observability standard, and avoids the bundle, privacy, and stability risk that `WEB-OBS-02` cautions against |
| Browser exports through a same-origin BFF endpoint | Keeps the collector origin out of the browser, works with the strict `connect-src 'self'` CSP, and reuses the BFF session and CSRF controls |
| BFF owns W3C Trace Context propagation | The BFF is the trust boundary; it validates or replaces untrusted inbound context and sets a valid value upstream so the API can join the trace |
| Tracing configuration is optional and validated at startup | Configuration is separate from code; a deployment without a collector starts normally with tracing disabled |
| Trace is a probe consumer in the catalog | The catalog stays the single source of truth for who consumes each probe, per `UIS-OBS-10` |
