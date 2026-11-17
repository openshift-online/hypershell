# ADR-0006: API Server Observability design decisions

Rationale relocated from [`specs/platform/api-server-observability.spec.md`](../../specs/platform/api-server-observability.spec.md) (API-OBS). The spec holds the normative behavior; this ADR holds the reasoning. Reference-only; not loaded by reconcile.

| Decision | Rationale |
| --- | --- |
| Standard OTel environment variables | Follows the OTel configuration specification, matching what operators already know and keeping configuration separate from code |
| Opt-in by collector-endpoint presence | Zero overhead when no collector is configured; no code change is needed to enable it in production |
| Parent-based trace-id-ratio sampler | Preserves the upstream sampling decision across the browser, BFF, and API so a trace is never decapitated |
| Server-level middleware and interceptors, not per-plugin | Plugins register routes and services; instrumentation is a cross-cutting concern applied once at the server layer |
| Method plus templated route as the span name | Groups spans by endpoint in Jaeger and keeps span-name cardinality bounded; a static or catch-all name would make traces unreadable |
| Extraction before authentication | A propagated trace joins regardless of the auth outcome, so authentication failures are still traceable |
| OTLP/gRPC on 4317 for the API server | The development Jaeger reserves 4317 (OTLP/gRPC) for the API server and 4318 (OTLP/HTTP) for the web console; exporting over gRPC matches that contract |
| Correlate the existing operation identifier onto the span | The framework already returns `operation_id`; recording it on the span joins a user-visible failure to server evidence without new caller-facing surface |
| One slog logger shared with the framework, installed by `serve` before the environment initializes | The framework keeps a program's own slog default (`SetDefaultIfUnset`), so one logger receives every record with its context; the OTel slog bridge reads the span from that context |
| Strip HTTP headers and bodies in the logger, not by capping the level | The framework adds them at `debug`; removing them from its two HTTP records keeps every other debug record available, where the previous glog verbosity cap had to give up a whole verbosity level |
| OTLP export for traces, Prometheus preserved for metrics | Traces require push-based export over OTLP; the existing framework Prometheus metrics are kept as-is |
| Database spans via a GORM plugin on the base connection, not per data-access object | A GORM plugin installs callbacks once on the base connection, so every plugin's queries are traced without per-plugin changes and callbacks are never duplicated per request; this mirrors the server-level HTTP and gRPC instrumentation |
| Database instrumentation registered through a framework plugin registry | The base connection is owned by the rh-trex-ai `SessionFactory`; a registry (mirroring the framework's pre-auth middleware and interceptor registries) lets the API server opt into query tracing without coupling every framework consumer to the tracing dependency |
| Operation-and-table span names, parameterized statement as an attribute | Keeps span-name cardinality bounded like the HTTP route templates while still allowing per-trace query detail; bound values and secrets are excluded per API-OBS-06 |
| Placeholder-and-continue when a log field is non-serializable, not drop the record | A single non-serializable field value (for example a `func(int64)`) must never blank the whole request/response record; the logger that masks the data needed to debug a failure is worse than the failure. A bounded placeholder keeps the rest of the record, so the root error (such as the failing SQL behind a `25P02`) stays visible |
| API-OBS-11 delivered upstream (rh-trex-ai) plus a pin bump, not in-repo | Both defects (the swallowed `Count()` error and the `func(int64)` logger drop) are rh-trex-ai framework code this repository only consumes through its pinned dependency, so they cannot be fixed here; tracked by GitHub issue #895 |

## Additional rationale

- The logger strips request and response bodies (not only credential headers) because some API responses return one-time credentials in the body, and the framework only redacts credential headers.
