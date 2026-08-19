# API Server Observability

**Status:** Active
**Applies to:** `components/api-server` HTTP and gRPC servers, the OTel bootstrap and instrumentation it registers, and the local development observability workflow
**Jira:** HYPERSHELL-26

## Purpose

Give the HyperShell API server distributed tracing and request-level metrics through OpenTelemetry (OTel), so a single user workflow can be followed from the browser, through the web-console BFF, into the API server, and operators can observe request latency and error rates across the platform. This specification is the API-server counterpart to `web-console/tracing.spec.md` (HYPERSHELL-27): the browser and BFF already produce spans and propagate W3C Trace Context, and this specification makes the API server extract that context and join the same trace. Instrumentation is a cross-cutting concern applied once at the server layer, transparent to plugin authors: HTTP middleware and gRPC interceptors create spans and record metrics without per-handler changes.

This specification covers the API server component only. Control-plane observability and gateway-level telemetry are out of scope. Where `standards/security/security.spec.md` imposes a stricter rule on what may appear in telemetry, that rule governs.

## Requirements

### Requirement: API-OBS-01 -- OTel SDK Bootstrap and Configuration

The API server SHALL initialize the OpenTelemetry SDK at startup, configuring a `TracerProvider` and a `MeterProvider` that export telemetry via OTLP to any OTel-compatible collector. Configuration SHALL be deployment configuration, separate from code, supplied through the standard OTel environment variables:

| Env Var | Default | Description |
|---------|---------|-------------|
| `OTEL_EXPORTER_OTLP_ENDPOINT` | (unset -- telemetry disabled) | OTLP collector endpoint (for example `http://jaeger.hypershell-system.svc.cluster.local:4317`) |
| `OTEL_EXPORTER_OTLP_PROTOCOL` | `grpc` | OTLP transport (`grpc` or `http/protobuf`); the API server exports over OTLP/gRPC by default |
| `OTEL_TRACES_SAMPLER_ARG` | `1.0` | Root-trace sampling ratio (`0.0` to `1.0`) for the parent-based sampler |
| `OTEL_SERVICE_NAME` | `hypershell-api-server` | Service name reported in spans and metrics |

The SDK SHALL use a parent-based trace-id-ratio sampler so a child span inherits the parent's sampling decision and only a root span is subject to the configured ratio. The SDK SHALL install a W3C Trace Context propagator, composed with baggage, as the global propagator.

Telemetry SHALL be opt-in by the presence of a collector endpoint. When `OTEL_EXPORTER_OTLP_ENDPOINT` is not set, the SDK SHALL NOT be initialized, no tracing or metrics middleware SHALL be registered, and the server SHALL run with no observability overhead. When the endpoint is set but the collector is unreachable, the server SHALL continue to serve requests normally; export failures SHALL be logged and SHALL NOT crash or degrade request handling. On termination the SDK SHALL flush buffered spans and metrics, bounded by a timeout so shutdown never blocks indefinitely.

**Verification:** Start the server with the endpoint set, absent, and set-but-unreachable; confirm export when configured, no initialization and no overhead when absent, and continued request handling with logged failures when unreachable. Send SIGTERM after serving traffic and confirm a bounded flush.

#### Scenario: Telemetry enabled with a collector endpoint

- GIVEN `OTEL_EXPORTER_OTLP_ENDPOINT` is set to `http://collector:4317`
- AND `OTEL_TRACES_SAMPLER_ARG` is set to `0.5`
- WHEN the API server starts
- THEN the OTel SDK SHALL be initialized with an OTLP/gRPC exporter targeting `http://collector:4317`
- AND the sampler SHALL sample 50 percent of root traces
- AND spans and metrics SHALL be exported to the configured endpoint

#### Scenario: Telemetry disabled when no endpoint is configured

- GIVEN `OTEL_EXPORTER_OTLP_ENDPOINT` is not set
- WHEN the API server starts
- THEN the OTel SDK SHALL NOT be initialized
- AND no tracing or metrics middleware SHALL be registered
- AND the server SHALL operate with no observability overhead

#### Scenario: Collector unreachable at runtime

- GIVEN `OTEL_EXPORTER_OTLP_ENDPOINT` is set but the collector is unreachable
- WHEN the API server handles requests
- THEN the server SHALL continue to serve requests normally
- AND telemetry export failures SHALL be logged rather than surfaced to the caller
- AND the server SHALL NOT crash or degrade request handling because of export errors

#### Scenario: Graceful shutdown flushes telemetry

- GIVEN the OTel SDK is initialized and the server has served requests
- WHEN the server receives SIGTERM
- THEN the SDK SHALL flush buffered spans and metrics before the process exits
- AND the flush SHALL be bounded by a timeout so shutdown does not block indefinitely

### Requirement: API-OBS-02 -- HTTP Server Spans With Templated Route Names

The API server SHALL wrap its HTTP handler with OpenTelemetry middleware that creates one server span per inbound request and extracts inbound W3C Trace Context. The middleware SHALL be applied once at the server level so every route, including routes registered by plugins, is instrumented without per-plugin changes.

The span name SHALL combine the request method and the templated route (for example `GET /api/hypershell/v1/gateways/{id}`) so Jaeger groups spans by endpoint. A single static span name SHALL NOT be used for every request, and a catch-all pattern SHALL NOT be the span name or the `http.route` value. The templated route SHALL collapse every resource identifier to a bounded placeholder and SHALL NOT carry a query string, so span-name cardinality stays fixed per `API-OBS-06`. Each span SHALL follow the OpenTelemetry semantic conventions for HTTP and SHALL record at least the request method, the matched route template, the request path, and the response status code.

**Verification:** Send representative requests to fixed and parameterized routes; confirm one server span each, named by method and templated route with resource identifiers collapsed to placeholders, carrying the HTTP semantic-convention attributes, and that plugin-registered routes are instrumented without plugin changes.

#### Scenario: Inbound HTTP request produces a templated span

- GIVEN the OTel SDK is initialized
- WHEN a client sends `GET /api/hypershell/v1/gateways/abc123`
- THEN one server span SHALL be created named `GET /api/hypershell/v1/gateways/{id}`
- AND the span SHALL record the request method, matched route template, and response status code
- AND the concrete identifier `abc123` SHALL NOT appear in the span name or `http.route`

### Requirement: API-OBS-03 -- gRPC Server Spans

The API server SHALL register OpenTelemetry gRPC server interceptors, both unary and streaming, that create a span for each inbound RPC and extract W3C Trace Context from inbound gRPC metadata. The interceptors SHALL be registered once at the server level so every gRPC service, including Watch streams registered by plugins, is instrumented without per-plugin changes. Trace-context extraction SHALL run before authentication so a propagated trace is joined regardless of the auth outcome.

Each span SHALL be named by the fully qualified RPC method and SHALL follow the OpenTelemetry semantic conventions for RPC, recording at least the RPC system, service, method, and gRPC status code. A streaming span SHALL cover the lifetime of the stream and SHALL record the final status when the stream closes.

**Verification:** Issue a unary call and open a Watch stream with propagated trace metadata; confirm a span per RPC named by full method, carrying the RPC semantic-convention attributes, that the stream span spans the stream lifetime and records its terminal status, and that extraction precedes auth.

#### Scenario: Unary gRPC call produces a span

- GIVEN the OTel SDK is initialized
- WHEN a control-plane client calls a unary RPC
- THEN one span SHALL be created named by the fully qualified method
- AND the span SHALL record the RPC service, method, and gRPC status code

#### Scenario: Streaming Watch produces a lifetime span

- GIVEN the OTel SDK is initialized
- WHEN the control plane opens a Watch stream
- THEN one span SHALL cover the lifetime of the stream
- AND the span SHALL record the final gRPC status code when the stream closes

### Requirement: API-OBS-04 -- Cross-Service Trace Continuation

The API server SHALL treat an inbound `traceparent`/`tracestate` pair, whether carried in HTTP headers or gRPC metadata, as the parent context and SHALL continue that trace. When inbound context is absent or malformed, the API server SHALL start a new root span whose sampling is governed by `API-OBS-01`; it SHALL NOT adopt a malformed value. This is the API-server counterpart to `web-console/tracing.spec.md` (`WEB-TRACE-04`, `WEB-TRACE-05`): a browser workflow span, the BFF server span, and the API server span SHALL join a single trace identified by one trace identifier.

**Verification:** Drive a representative browser-to-API workflow through the deployed console and confirm one trace whose spans include the browser workflow span, the BFF server span, and the API server span sharing one trace identifier. Send an API request with a malformed `traceparent` and confirm a new root span rather than a decapitated child.

#### Scenario: API joins the trace propagated by the BFF

- GIVEN the OTel SDK is initialized
- AND the BFF proxies a request to `/api/hypershell/v1/*` carrying a valid `traceparent`
- WHEN the API server handles the request
- THEN the API server span SHALL be a child of the trace identified in `traceparent`
- AND the browser, BFF, and API spans SHALL share one trace identifier

#### Scenario: Absent context starts a new trace

- GIVEN the OTel SDK is initialized
- AND a request arrives without a `traceparent`
- WHEN the API server handles the request
- THEN a new root span SHALL be created
- AND the sampling decision SHALL be governed by `OTEL_TRACES_SAMPLER_ARG`

### Requirement: API-OBS-05 -- Request Metrics via OTLP

The API server SHALL export OpenTelemetry request metrics for both HTTP and gRPC traffic over OTLP alongside traces. Metrics SHALL be labeled with the same semantic-convention attributes used on spans (method, route or service, and status code) so traces and metrics correlate, and SHALL respect the cardinality bounds of `API-OBS-06`. These metrics complement, and SHALL NOT replace, the existing Prometheus metrics exposed by the framework metrics server.

| Metric | Type | Unit | Description |
|--------|------|------|-------------|
| `http.server.request.duration` | Histogram | `s` | Latency of inbound HTTP requests |
| `http.server.active_requests` | UpDownCounter | `{request}` | In-flight HTTP requests |
| `rpc.server.duration` | Histogram | `ms` | Latency of inbound gRPC calls |

**Verification:** Complete representative HTTP and gRPC calls and confirm the duration histograms record them labeled by method or service and status code, that the active-request counter tracks in-flight requests, and that the existing Prometheus metrics are still exposed.

#### Scenario: HTTP latency metric recorded

- GIVEN the OTel SDK is initialized
- WHEN a client completes a `POST /api/hypershell/v1/gateways` request
- THEN `http.server.request.duration` SHALL record the request duration
- AND the sample SHALL be labeled with the request method, templated route, and response status code

#### Scenario: gRPC latency metric recorded

- GIVEN the OTel SDK is initialized
- WHEN a unary gRPC call completes
- THEN `rpc.server.duration` SHALL record the call duration
- AND the sample SHALL be labeled with the RPC service and gRPC status code

### Requirement: API-OBS-06 -- Telemetry Privacy, Cardinality, and Operation Correlation

Trace spans, span events, and metric attributes SHALL NOT include sensitive data. Authorization headers, bearer tokens, cookie values, database credentials, connection strings, secret references, and request or response bodies SHALL NOT appear in telemetry. Raw resource identifiers SHALL NOT become span-name segments or propagated baggage; stable route templates and bounded enumerations SHALL replace raw URLs, and a high-cardinality value SHALL NOT be a span name segment. This aligns with `standards/security/security.spec.md` and with `web-console/tracing.spec.md` (`WEB-TRACE-07`).

The API server already returns a per-request operation identifier (the `X-Operation-ID` response header and the `operation_id` field of the error envelope). The request span SHALL record that operation identifier as a bounded attribute so a user-visible failure can be joined to the server-side trace evidence.

**Verification:** Run redaction checks with seeded secrets, tokens, and an `Authorization` header and confirm none appear in spans or metric attributes; confirm span names use route templates with identifiers collapsed; trigger a failing request and confirm the returned `operation_id` matches the operation-identifier attribute on the request span.

#### Scenario: Authorization header excluded from telemetry

- GIVEN the OTel SDK is initialized
- WHEN a client sends a request with an `Authorization: Bearer <token>` header
- THEN no span attribute or metric label SHALL contain the header value or the token

#### Scenario: Failure operation identifier correlates to the span

- GIVEN the OTel SDK is initialized
- WHEN a request fails and the response envelope carries an `operation_id`
- THEN the request span SHALL carry that same operation identifier as a bounded attribute
- AND support SHALL be able to find the server trace from the user-visible `operation_id`

### Requirement: API-OBS-07 -- Development Trace Export

When local development tracing is enabled (`KIND_JAEGER=true`), the API server Deployment SHALL be configured with `OTEL_EXPORTER_OTLP_ENDPOINT` pointing at the in-cluster Jaeger OTLP/gRPC receiver on port `4317`, so a developer can view the API server spans joined with the browser and BFF spans in one trace. The Jaeger deployment, its ports, the `KIND_JAEGER` gate, and the UI hostname are defined by `platform/local-development.spec.md`; this requirement only binds the API server to that endpoint. When `KIND_JAEGER` is unset, the API server Deployment SHALL NOT receive a collector endpoint and SHALL run without telemetry. The automated cross-service trace check defined by `platform/e2e-testing.spec.md` (`WEB-TRACE-11`) SHOULD be extended to assert the API server span joins the browser and BFF spans once the API server is instrumented.

**Verification:** Bring up the local cluster with `KIND_JAEGER=true`, drive a workflow that reaches the API, and confirm the API server span appears in Jaeger joined to the browser and BFF spans; bring it up without `KIND_JAEGER` and confirm the API server Deployment has no collector endpoint and emits no telemetry.

#### Scenario: API spans reach the development Jaeger

- GIVEN the local cluster is running with `KIND_JAEGER=true`
- WHEN a developer completes a gateway workflow through the console
- THEN the API server SHALL export its spans to the in-cluster Jaeger OTLP/gRPC endpoint on port `4317`
- AND one Jaeger trace SHALL contain the browser workflow span, the BFF server span, and the API server span

#### Scenario: No export when development tracing is disabled

- GIVEN `KIND_JAEGER` is unset
- WHEN a developer runs `make kind-up`
- THEN the API server Deployment SHALL NOT be given a collector endpoint
- AND the API server SHALL run without OTel instrumentation

## Design Decisions

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
| OTLP export for traces, Prometheus preserved for metrics | Traces require push-based export over OTLP; the existing framework Prometheus metrics are kept as-is |

## Primary Basis

- `standards/security/security.spec.md`
- `web-console/tracing.spec.md` (`WEB-TRACE-04`, `WEB-TRACE-05`, `WEB-TRACE-07`, `WEB-TRACE-11`)
- `platform/local-development.spec.md` (development Jaeger, `KIND_JAEGER`)
- `platform/e2e-testing.spec.md` (automated cross-service trace verification)
- [OpenTelemetry Go](https://opentelemetry.io/docs/languages/go/)
- [OpenTelemetry semantic conventions](https://opentelemetry.io/docs/concepts/semantic-conventions/)
- [OpenTelemetry SDK environment variables](https://opentelemetry.io/docs/specs/otel/configuration/sdk-environment-variables/)
- [W3C Trace Context](https://www.w3.org/TR/trace-context/)
- [OTLP specification](https://opentelemetry.io/docs/specs/otlp/)
