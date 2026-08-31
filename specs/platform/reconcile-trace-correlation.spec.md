# Reconcile-to-Request Trace Correlation

**Status:** Draft
**Applies to:** `components/api-server` resource persistence and gRPC watch messages, `components/control-plane` reconcile spans, and the W3C Trace Context stored on each resource
**Jira:** (to be assigned)

## Purpose

When an operator investigates a reconcile failure, they need to trace the causal chain: which user action triggered the change that the control plane is now reconciling? Today the control plane produces reconcile spans (CP-OBS-02) and the API server produces request spans (API-OBS-02/03), but the two traces are disconnected because reconciliation is temporally decoupled from the user request. The API server writes desired state to PostgreSQL and returns; the control plane observes the change later via a gRPC watch stream, possibly after resync, batching, or retries.

A synchronous parent-child span relationship is therefore incorrect: it would produce a child span that starts long after its parent ended, or one request span with many reconcile children from resyncs. The correct OpenTelemetry model is a **span link** ("caused by"): the reconcile span keeps its own root trace (independently sampled per CP-OBS-02) and carries a link to the originating request trace, so a support engineer can navigate from the reconcile trace to the request trace in Jaeger.

This specification defines how the originating trace context flows from the API server request, through the database, over the gRPC watch stream, into the control plane reconcile span as a link. It extends `platform/api-server-observability.spec.md` (HYPERSHELL-26) and `platform/control-plane-observability.spec.md` (HYPERSHELL-79).

## Requirements

### Requirement: RTC-01 -- Originating Trace Context Persistence

The API server SHALL capture the W3C Trace Context (`traceparent` header value, and `tracestate` when present) from the inbound request context on every create and update write, and SHALL persist both values on the resource row in PostgreSQL. The trace context SHALL be stored as plain text columns (`traceparent` and `tracestate`) on the shared `api.Meta` base, so every resource type inherits the field without per-plugin schema changes.

Because the `api.Meta` base struct is defined in the upstream `rh-trex-ai` framework and cannot be modified in-tree, the trace context columns SHALL be added via a local embeddable struct (for example `TraceMeta`) that each resource model embeds alongside `api.Meta`. A single gormigrate migration SHALL add the columns to all resource tables.

The `traceparent` column SHALL store the W3C Trace Context `traceparent` header value (for example `00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01`). The `tracestate` column SHALL store the W3C `tracestate` header value when present, or be empty when absent. Both columns SHALL be nullable: a resource created before this change, or created when telemetry is disabled, SHALL have NULL trace context and that is a valid state.

The trace context SHALL be captured from the active span context at the point of persistence (not from the raw HTTP header), so it reflects the actual span that performed the write. On an update, the stored trace context SHALL be overwritten with the new request's context, so the trace context always points to the most recent mutation.

**Verification:** Create a resource via the API with a valid inbound `traceparent`; confirm the database row stores the `traceparent` and `tracestate` values. Update the resource with a different request trace; confirm the stored values are overwritten.

#### Scenario: Create persists originating trace context

- GIVEN the OTel SDK is initialized on the API server
- AND a client sends a POST request carrying a valid `traceparent` header
- WHEN the API server creates the resource
- THEN the resource row in PostgreSQL SHALL contain the `traceparent` value from the request's active span context
- AND `tracestate` SHALL be stored when present in the inbound context

#### Scenario: Update overwrites trace context

- GIVEN a resource with a stored `traceparent` from its creation
- WHEN a client sends a PATCH request with a different trace context
- THEN the stored `traceparent` SHALL be overwritten with the new request's active span context
- AND the previous trace context SHALL not be retained

#### Scenario: No trace context when telemetry is disabled

- GIVEN the OTel SDK is not initialized (no `OTEL_EXPORTER_OTLP_ENDPOINT`)
- WHEN a client creates a resource
- THEN the `traceparent` and `tracestate` columns SHALL be NULL
- AND the resource SHALL be created normally

### Requirement: RTC-02 -- Trace Context on gRPC Watch Messages

The resource protobuf messages carried in gRPC watch responses SHALL include the originating trace context so the control plane can read it without a separate lookup. The `traceparent` and `tracestate` fields SHALL be added to the shared `ObjectReference` message so every resource type's watch response carries them uniformly.

The fields SHALL be optional strings. When the stored trace context is NULL (resource created before this change or with telemetry disabled), the fields SHALL be empty in the protobuf message.

**Verification:** Create a resource with an active trace; receive its watch event and confirm the `traceparent` field on the resource's metadata matches the stored value. Create a resource with no trace context and confirm the fields are empty.

#### Scenario: Watch event carries trace context

- GIVEN a resource created with a stored `traceparent`
- WHEN the control plane receives a watch event for that resource
- THEN the resource's `ObjectReference` metadata SHALL contain the `traceparent` value
- AND `tracestate` SHALL be present when the resource has a stored value

#### Scenario: Watch event with no trace context

- GIVEN a resource created before trace context persistence was added
- WHEN the control plane receives a watch event for that resource
- THEN the `traceparent` and `tracestate` fields on `ObjectReference` SHALL be empty strings
- AND the watch event SHALL be processed normally

### Requirement: RTC-03 -- Reconcile Span Link to Originating Trace

The control plane SHALL parse the `traceparent` (and `tracestate` when present) from the watched resource's metadata, extract the trace ID and span ID, and attach them as a span link on the reconcile root span. The span link SHALL use the OpenTelemetry `trace.Link` with the remote span context so Jaeger renders the link as a "caused by" relationship.

The reconcile span SHALL remain a new trace root (per CP-OBS-02) and SHALL NOT become a child of the originating request span. The span link is a causal reference, not a parent-child relationship. The reconcile span's sampling decision SHALL remain independent of the originating trace's sampling decision.

When the resource has no stored trace context (NULL `traceparent`), the reconcile span SHALL be a normal root with no link and no error. A missing or malformed `traceparent` SHALL NOT cause a reconcile failure or produce a warning; it SHALL be silently ignored.

**Verification:** Create a resource with an active trace; trigger reconciliation and confirm the reconcile span in Jaeger carries a link to the originating request trace. Reconcile a resource with no trace context and confirm a normal root span with no link.

#### Scenario: Reconcile span links to originating request

- GIVEN a resource with a stored `traceparent` of `00-{traceID}-{spanID}-01`
- WHEN the control plane reconciles the resource
- THEN the reconcile root span SHALL carry a span link to the trace identified by `{traceID}` and `{spanID}`
- AND the reconcile span SHALL remain a new trace root with its own trace ID
- AND Jaeger SHALL render the link as a navigable reference from the reconcile trace to the request trace

#### Scenario: Reconcile without trace context produces no link

- GIVEN a resource with no stored `traceparent` (NULL or empty)
- WHEN the control plane reconciles the resource
- THEN the reconcile span SHALL be a normal root with no span link
- AND no error or warning SHALL be logged

#### Scenario: Malformed traceparent is silently ignored

- GIVEN a resource with a stored `traceparent` value that does not conform to W3C Trace Context
- WHEN the control plane reconciles the resource
- THEN the reconcile span SHALL be a normal root with no span link
- AND no reconcile error SHALL be raised

### Requirement: RTC-04 -- End-to-End Trace Navigation in Jaeger

In the development Jaeger (when `KIND_JAEGER=true`, per CP-OBS-08 and API-OBS-07), a support engineer SHALL be able to navigate from a reconcile trace to the originating user request trace. The Jaeger UI SHALL show the span link on the reconcile span and allow clicking through to the originating trace. The originating trace SHALL include the browser workflow span, the BFF server span, the API server span, and the database span (when all components have OTel enabled).

This requirement is a verification-only requirement: it does not impose new code beyond RTC-01 through RTC-03, but it validates the end-to-end experience.

**Verification:** With `KIND_JAEGER=true`, create a gateway through the console; wait for reconciliation; find the reconcile span in Jaeger and confirm it has a link to the browser-to-API trace for the same gateway operation.

#### Scenario: End-to-end trace navigation

- GIVEN the local cluster is running with `KIND_JAEGER=true`
- AND the API server, control plane, and web console all have OTel enabled
- WHEN a developer creates a gateway through the console
- AND the control plane reconciles the gateway
- THEN the reconcile trace in Jaeger SHALL show a span link to the originating request trace
- AND clicking the link SHALL navigate to the trace containing the browser, BFF, API server, and database spans

### Requirement: RTC-05 -- Privacy and Cardinality

The trace context stored on a resource and carried in watch messages is opaque W3C Trace Context: a `traceparent` string containing version, trace ID, span ID, and trace flags, and an optional `tracestate` string containing vendor-specific key-value pairs. These values SHALL NOT contain sensitive data by construction (they are hex-encoded identifiers and vendor flags). The trace context SHALL NOT be exposed in the REST API responses; it is internal to the observability pipeline.

The span link on the reconcile span records the linked trace ID and span ID as span attributes. These are bounded hex identifiers and do not affect span-name cardinality or metric label cardinality, consistent with CP-OBS-06.

#### Scenario: Trace context not exposed in REST responses

- GIVEN a resource with a stored `traceparent`
- WHEN a client retrieves the resource via the REST API
- THEN the `traceparent` and `tracestate` fields SHALL NOT appear in the JSON response

#### Scenario: Trace context values do not contain sensitive data

- GIVEN a resource created with an inbound `traceparent`
- WHEN the stored value is inspected
- THEN it SHALL be a W3C Trace Context string containing only hex-encoded identifiers and flags
- AND it SHALL NOT contain bearer tokens, user identifiers, or any sensitive data

## Design Decisions

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

## Primary Basis

- `platform/control-plane-observability.spec.md` (HYPERSHELL-79) -- reconcile spans, `WithNewRoot()`, CP-OBS-02, CP-OBS-06
- `platform/api-server-observability.spec.md` (HYPERSHELL-26) -- API server spans, trace context extraction, API-OBS-04
- `platform/data-model.spec.md` -- `api.Meta` base type, resource schema
- `web-console/tracing.spec.md` (HYPERSHELL-27) -- browser and BFF spans
- `standards/security/security.spec.md` -- telemetry privacy
- [W3C Trace Context](https://www.w3.org/TR/trace-context/) -- `traceparent` and `tracestate` format
- [OpenTelemetry Span Links](https://opentelemetry.io/docs/concepts/signals/traces/#span-links) -- causal reference semantics
