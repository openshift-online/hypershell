# Control Plane Observability

**Status:** Draft
**Applies to:** `components/control-plane` reconciler spans, gRPC watch stream lifecycle, outbound gRPC and Kubernetes API calls, reconcile metrics, and the local development observability workflow
**Jira:** HYPERSHELL-79

## Purpose

Give the HyperShell control plane distributed tracing and reconcile-level metrics through OpenTelemetry (OTel), so an operator can observe reconcile latency, gRPC watch health, Kubernetes API calls, and failures across the fleet. This specification is the control-plane counterpart to `platform/api-server-observability.spec.md` (HYPERSHELL-26) and `web-console/tracing.spec.md` (HYPERSHELL-27): the API server already produces server spans for inbound HTTP and gRPC requests, and this specification makes the control plane produce spans for the asynchronous reconciliation work that follows.

Correlating a reconcile trace back to the originating user request is intentionally deferred to a follow-up story, because reconciliation is asynchronous: the API writes desired state to PostgreSQL and returns; the control plane observes the change later via a watch stream, possibly after resync, batching, or retries. That correlation is tracked separately.

This specification covers the control plane component only. API server instrumentation is defined by `platform/api-server-observability.spec.md`. Where `standards/security/security.spec.md` imposes a stricter rule on what may appear in telemetry, that rule governs.

## Requirements

### Requirement: CP-OBS-01 -- OTel SDK Bootstrap and Configuration

The control plane SHALL initialize the OpenTelemetry SDK at startup, configuring a `TracerProvider` and a `MeterProvider` that export telemetry via OTLP to any OTel-compatible collector. Configuration SHALL be deployment configuration, separate from code, supplied through the standard OTel environment variables:

| Env Var | Default | Description |
|---------|---------|-------------|
| `OTEL_EXPORTER_OTLP_ENDPOINT` | (unset -- telemetry disabled) | OTLP collector endpoint (for example `http://jaeger.hypershell-system.svc.cluster.local:4317`) |
| `OTEL_EXPORTER_OTLP_PROTOCOL` | `grpc` | OTLP transport (`grpc` or `http/protobuf`); the control plane exports over OTLP/gRPC by default |
| `OTEL_TRACES_SAMPLER_ARG` | `1.0` | Root-trace sampling ratio (`0.0` to `1.0`) for the parent-based sampler |
| `OTEL_SERVICE_NAME` | `hypershell-controller` | Service name reported in spans and metrics |
| `OTEL_METRICS_EXPORTER` | (unset -- metrics enabled) | Set to `none` to export traces only and skip the OTLP metric exporter; use against a trace-only backend such as Jaeger |

The SDK SHALL use a parent-based trace-id-ratio sampler so a child span inherits the parent's sampling decision and only a root span is subject to the configured ratio. The SDK SHALL install a W3C Trace Context propagator, composed with baggage, as the global propagator.

Telemetry SHALL be opt-in by the presence of a collector endpoint. When `OTEL_EXPORTER_OTLP_ENDPOINT` is not set, the SDK SHALL NOT be initialized, no tracing or metrics instrumentation SHALL be active, and the control plane SHALL run with no observability overhead. When the endpoint is set but the collector is unreachable, the control plane SHALL continue reconciling normally; export failures SHALL be logged and SHALL NOT crash or degrade reconciliation. On termination the SDK SHALL flush buffered spans and metrics, bounded by a timeout so shutdown never blocks indefinitely.

The SDK initialization SHALL be called from `main()` before launching any goroutines. The returned shutdown function SHALL be called after the signal-driven context is cancelled, before the process exits.

**Verification:** Start the control plane with the endpoint set, absent, and set-but-unreachable; confirm export when configured, no initialization and no overhead when absent, and continued reconciliation with logged failures when unreachable. Send SIGTERM after reconciling traffic and confirm a bounded flush.

#### Scenario: Telemetry enabled with a collector endpoint

- GIVEN `OTEL_EXPORTER_OTLP_ENDPOINT` is set to `http://collector:4317`
- AND `OTEL_TRACES_SAMPLER_ARG` is set to `0.5`
- WHEN the control plane starts
- THEN the OTel SDK SHALL be initialized with an OTLP/gRPC exporter targeting `http://collector:4317`
- AND the sampler SHALL sample 50 percent of root traces
- AND spans and metrics SHALL be exported to the configured endpoint

#### Scenario: Telemetry disabled when no endpoint is configured

- GIVEN `OTEL_EXPORTER_OTLP_ENDPOINT` is not set
- WHEN the control plane starts
- THEN the OTel SDK SHALL NOT be initialized
- AND no tracing or metrics instrumentation SHALL be active
- AND the control plane SHALL operate with no observability overhead

#### Scenario: Collector unreachable at runtime

- GIVEN `OTEL_EXPORTER_OTLP_ENDPOINT` is set but the collector is unreachable
- WHEN the control plane reconciles resources
- THEN the control plane SHALL continue reconciling normally
- AND telemetry export failures SHALL be logged rather than surfaced to reconciler logic
- AND the control plane SHALL NOT crash or degrade reconciliation because of export errors

#### Scenario: Graceful shutdown flushes telemetry

- GIVEN the OTel SDK is initialized and the control plane has reconciled resources
- WHEN the control plane receives SIGTERM
- THEN the SDK SHALL flush buffered spans and metrics before the process exits
- AND the flush SHALL be bounded by a timeout so shutdown does not block indefinitely

### Requirement: CP-OBS-02 -- Reconcile Spans

The control plane SHALL create one span per resource reconciliation, wrapping each `Handle()` invocation on an event-driven reconciler. The span SHALL be named by the resource kind and event type (for example `reconcile Gateway`, `delete Gateway`, `reconcile Fleet`) so Jaeger groups spans by reconciler. A resource identifier SHALL NOT appear in the span name to keep cardinality bounded per `CP-OBS-06`.

Each reconcile span SHALL record at least the resource kind and event type as span attributes. The span status SHALL reflect the reconcile outcome: OK on success, Error with a description on failure. When a reconcile is skipped because of the phase gate or deduplication, no span SHALL be created for the skipped event.

The continuous reconcilers (health, namespace GC, sandbox count) SHALL each create a span per tick wrapping the periodic reconcile cycle, named by reconciler (for example `reconcile gateway-health`, `reconcile namespace-gc`, `reconcile sandbox-count`).

**Verification:** Trigger a gateway create, update, and delete event; confirm one span each named by kind and event type, with the outcome reflected in the span status. Confirm that a phase-gated skip produces no span. Confirm that the health reconciler produces a span per tick.

#### Scenario: Gateway reconcile produces a span

- GIVEN the OTel SDK is initialized
- WHEN a Gateway create event arrives from the watch stream
- THEN one span SHALL be created named `reconcile Gateway`
- AND the span SHALL record the resource kind and event type
- AND the span status SHALL be OK if reconciliation succeeds

#### Scenario: Failed reconcile records error status

- GIVEN the OTel SDK is initialized
- WHEN a Gateway reconciliation fails
- THEN the reconcile span status SHALL be Error
- AND the span SHALL record a description of the failure

#### Scenario: Phase-gated skip produces no span

- GIVEN the OTel SDK is initialized
- AND a Gateway has phase `Running`
- WHEN a Gateway update event arrives
- THEN no reconcile span SHALL be created for the skipped event

#### Scenario: Health reconciler produces periodic spans

- GIVEN the OTel SDK is initialized
- WHEN the gateway health reconciler completes a tick
- THEN one span SHALL be created named `reconcile gateway-health`

### Requirement: CP-OBS-03 -- Outbound gRPC Client Spans

The control plane SHALL register OpenTelemetry gRPC client interceptors, both unary and streaming, on the shared gRPC client connection to the API server. The interceptors SHALL create a client span for each outbound RPC and inject W3C Trace Context into outbound gRPC metadata. The interceptors SHALL be registered once on the dial options so every gRPC service client created from the shared connection is instrumented without per-call changes.

Each client span SHALL be named by the fully qualified RPC method and SHALL follow the OpenTelemetry semantic conventions for RPC, recording at least the RPC system, service, method, and gRPC status code.

When the control plane issues a gRPC call during a reconcile (for example `UpdateGateway` to set the phase), the gRPC client span SHALL be a child of the active reconcile span, joining one trace from the reconcile through the outbound call to the API server.

**Verification:** Trigger a reconcile that updates gateway phase via gRPC; confirm a client span named by the full RPC method as a child of the reconcile span, with RPC semantic-convention attributes. Confirm the API server receives `traceparent` metadata.

#### Scenario: Outbound gRPC call produces a client span

- GIVEN the OTel SDK is initialized
- WHEN the control plane calls `UpdateGateway` during a reconcile
- THEN one client span SHALL be created named by the fully qualified RPC method
- AND the span SHALL be a child of the active reconcile span
- AND the span SHALL record the RPC system, service, method, and gRPC status code

#### Scenario: Trace context propagated to the API server

- GIVEN the OTel SDK is initialized
- WHEN the control plane makes a gRPC call to the API server
- THEN W3C Trace Context SHALL be injected into the outbound gRPC metadata
- AND the API server span SHALL be a child of the control plane client span

### Requirement: CP-OBS-04 -- Watch Stream Lifecycle Spans

The control plane SHALL create a span for each watch stream connection attempt, wrapping the `connectAndRecv` cycle in `watchLoop`. The span SHALL be named by the resource kind (for example `watch Gateway`, `watch ManagedCluster`) and SHALL cover the lifetime of the stream from connection to disconnection. The span status SHALL reflect the stream outcome: OK on graceful EOF, Error on unexpected disconnection.

When a watch stream disconnects and reconnects, each connection attempt SHALL produce a new span. The reconnection backoff period SHALL NOT be included in the span duration; only the active stream lifetime SHALL be spanned.

**Verification:** Start the control plane with tracing enabled; confirm one span per watch connection named by kind. Disconnect the API server and confirm the span records an error status, and that the reconnection produces a new span.

#### Scenario: Watch stream produces a lifecycle span

- GIVEN the OTel SDK is initialized
- WHEN the control plane connects a Gateway watch stream
- THEN one span SHALL be created named `watch Gateway`
- AND the span SHALL cover the stream lifetime from connection to disconnection

#### Scenario: Watch reconnection produces a new span

- GIVEN the OTel SDK is initialized
- AND a Gateway watch stream disconnects
- WHEN the watcher reconnects after backoff
- THEN a new span SHALL be created for the new connection attempt
- AND the previous span SHALL have recorded an error status

### Requirement: CP-OBS-05 -- Kubernetes API Client Spans

The control plane SHALL instrument its Kubernetes API client HTTP transport so that every outbound Kubernetes API call produces a client span. The instrumentation SHALL be applied once on the shared `rest.Config` transport before creating the typed clientset and dynamic client, so every Kubernetes API call is traced without per-call changes.

Each Kubernetes API client span SHALL follow the OpenTelemetry semantic conventions for HTTP and SHALL record at least the request method, URL path, and response status code. The URL path SHALL use the Kubernetes API path pattern (for example `/api/v1/namespaces/{namespace}/deployments/{name}`) to keep span-name cardinality bounded per `CP-OBS-06`.

When the control plane makes a Kubernetes API call during a reconcile, the Kubernetes span SHALL be a child of the active reconcile span, joining one trace from the reconcile through the outbound call to the Kubernetes API server.

**Verification:** Trigger a gateway reconcile that creates Kubernetes resources; confirm client spans for the Kubernetes API calls as children of the reconcile span, with HTTP semantic-convention attributes. Confirm span names use bounded path patterns.

#### Scenario: Kubernetes API call produces a client span

- GIVEN the OTel SDK is initialized
- WHEN the control plane creates a Deployment via the Kubernetes API during a reconcile
- THEN one client span SHALL be created for the Kubernetes API call
- AND the span SHALL be a child of the active reconcile span
- AND the span SHALL record the request method, URL path, and response status code

### Requirement: CP-OBS-06 -- Telemetry Privacy and Cardinality

Trace spans, span events, and metric attributes SHALL NOT include sensitive data. Bearer tokens, OIDC client secrets, Keycloak credentials, kubeconfig contents, database connection strings, and secret references SHALL NOT appear in telemetry. Raw resource identifiers (KSUIDs, namespace names derived from KSUIDs) SHALL NOT become span-name segments; stable kind names and bounded enumerations SHALL replace raw values. This aligns with `standards/security/security.spec.md`.

Resource identifiers MAY appear as bounded span attributes (for example `hypershell.resource_id`) for per-trace debugging, but SHALL NOT be metric label values or span-name segments.

**Verification:** Run redaction checks with seeded secrets and OIDC credentials; confirm none appear in spans or metric attributes. Confirm span names use kind names with identifiers collapsed.

#### Scenario: OIDC credentials excluded from telemetry

- GIVEN the OTel SDK is initialized
- AND the control plane uses OIDC authentication for gRPC
- WHEN the control plane reconciles a resource
- THEN no span attribute or metric label SHALL contain OIDC client secrets, bearer tokens, or Keycloak credentials

#### Scenario: Resource identifiers excluded from span names

- GIVEN the OTel SDK is initialized
- WHEN the control plane reconciles Gateway `abc123`
- THEN the span name SHALL be `reconcile Gateway`, not `reconcile Gateway abc123`
- AND the concrete identifier MAY appear only as a bounded span attribute

### Requirement: CP-OBS-07 -- Reconcile and Watch Metrics via OTLP

The control plane SHALL export OpenTelemetry metrics for reconciliation and watch stream health over OTLP alongside traces. Metrics SHALL be labeled with bounded attributes (resource kind, event type, outcome) and SHALL respect the cardinality bounds of `CP-OBS-06`.

| Metric | Type | Unit | Description |
|--------|------|------|-------------|
| `reconcile.duration` | Histogram | `ms` | Latency of a single resource reconciliation |
| `reconcile.errors` | Counter | `{error}` | Count of failed reconciliations |
| `watch.reconnects` | Counter | `{reconnect}` | Count of watch stream reconnections |

Metrics SHALL complement any future Prometheus metrics endpoint and SHALL NOT prevent adding one later.

**Verification:** Trigger reconciliations and watch reconnections; confirm the duration histogram records reconcile latency, the error counter increments on failure, and the reconnect counter increments on watch stream reconnection, all labeled by resource kind.

#### Scenario: Reconcile duration metric recorded

- GIVEN the OTel SDK is initialized
- WHEN a Gateway reconciliation completes
- THEN `reconcile.duration` SHALL record the reconcile duration
- AND the sample SHALL be labeled with the resource kind and event type

#### Scenario: Reconcile error metric incremented

- GIVEN the OTel SDK is initialized
- WHEN a Gateway reconciliation fails
- THEN `reconcile.errors` SHALL be incremented
- AND the sample SHALL be labeled with the resource kind

#### Scenario: Watch reconnect metric incremented

- GIVEN the OTel SDK is initialized
- WHEN a Gateway watch stream disconnects and reconnects
- THEN `watch.reconnects` SHALL be incremented
- AND the sample SHALL be labeled with the resource kind

### Requirement: CP-OBS-08 -- Development Trace Export

When local development tracing is enabled (`KIND_JAEGER=true`), the control plane Deployment SHALL be configured with `OTEL_EXPORTER_OTLP_ENDPOINT` pointing at the in-cluster Jaeger OTLP/gRPC receiver on port `4317` and `OTEL_METRICS_EXPORTER` set to `none`, so a developer can view the control plane reconcile spans alongside the API server and web console spans in Jaeger. The Jaeger deployment, its ports, the `KIND_JAEGER` gate, and the UI hostname are defined by `platform/local-development.spec.md`; this requirement only binds the control plane to that endpoint.

When `KIND_JAEGER` is unset, the control plane Deployment SHALL NOT receive a collector endpoint and SHALL run without telemetry. When `KIND_JAEGER` is toggled off on a cluster that previously had it, the control plane Deployment SHALL have the endpoint removed, consistent with the reconcile-do-not-skip pattern used for the API server and web console.

**Verification:** Bring up the local cluster with `KIND_JAEGER=true`, trigger a reconcile, and confirm the control plane spans appear in Jaeger. Bring it up without `KIND_JAEGER` and confirm the control plane Deployment has no collector endpoint.

#### Scenario: Control plane spans reach the development Jaeger

- GIVEN the local cluster is running with `KIND_JAEGER=true`
- WHEN a gateway reconcile occurs
- THEN the control plane SHALL export its spans to the in-cluster Jaeger OTLP/gRPC endpoint on port `4317`
- AND the reconcile span and its child gRPC/Kubernetes spans SHALL appear in Jaeger

#### Scenario: No export when development tracing is disabled

- GIVEN `KIND_JAEGER` is unset
- WHEN a developer runs `make kind-up`
- THEN the control plane Deployment SHALL NOT be given a collector endpoint
- AND the control plane SHALL run without OTel instrumentation

## Design Decisions

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
| OTLP/gRPC on port 4317 for the control plane | Matches the API server's transport; the development Jaeger exposes 4317 for OTLP/gRPC |
| Reconcile-trace to request-trace correlation deferred | Reconciliation is asynchronous; the correlation mechanism (span links, trace-context persistence) deserves its own story |

## Primary Basis

- `platform/api-server-observability.spec.md` (HYPERSHELL-26)
- `web-console/tracing.spec.md` (HYPERSHELL-27)
- `platform/control-plane.spec.md`
- `standards/security/security.spec.md`
- `platform/local-development.spec.md` (development Jaeger, `KIND_JAEGER`)
- [OpenTelemetry Go](https://opentelemetry.io/docs/languages/go/)
- [OpenTelemetry semantic conventions](https://opentelemetry.io/docs/concepts/semantic-conventions/)
- [OpenTelemetry SDK environment variables](https://opentelemetry.io/docs/specs/otel/configuration/sdk-environment-variables/)
- [W3C Trace Context](https://www.w3.org/TR/trace-context/)
