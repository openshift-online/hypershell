# Web Console Distributed Tracing

**Status:** Active
**Applies to:** `components/web-console` browser application, the web-console BFF, `components/web-console/domain-probes`, the gateway management UI probe catalog, and the local development observability workflow
**Jira:** HYPERSHELL-27

## Purpose

Give the web console distributed tracing that follows one user workflow from the browser, through the BFF, to the HyperShell REST API. Traces are derived from the typed domain probes already defined by `standards/ui/domain-observability.spec.md`; they are not a second, parallel telemetry path. This specification narrows the tracing requirements in `web-console/architecture.spec.md` (`WEB-OBS-01`, `WEB-OBS-02`) and the standard in `standards/ui/domain-observability.spec.md` into a concrete OpenTelemetry (OTel) wiring. It does not replace those documents; where they impose a stricter rule, that rule governs.

The API server is instrumented separately (HYPERSHELL-26). This specification requires the browser and BFF to produce spans and to propagate W3C Trace Context so the API server can join the same trace once it extracts that context.

## Requirements

### Requirement: WEB-TRACE-01 -- Browser Spans Derived From Domain Probes

The browser SHALL produce spans from the typed domain probes published through the `DomainProbePublisher` fan-out, not from ad hoc instrumentation. A trace sink SHALL implement the `DomainProbeSink` contract and register through the existing `additionalSinks` extension point of the gateway observability adapter. The sink SHALL map a workflow `started` probe and its matching terminal probe (`succeeded`, `failed`, `cancelled`, `denied`, or `conflicted`) to one span, and each dependency-attempt probe pair to one child span. Span status SHALL reflect the terminal outcome. The sink SHALL NOT create spans from React renders, pointer movement, or pure computation.

The workflow span SHALL be the root of the browser-originated trace. It MAY adopt the caller-chosen trace identifier so the trace joins the id the browser propagates to the BFF and API, but it SHALL NOT descend from a synthetic or remote parent span. A manufactured parent that no service records would leave the exported trace decapitated (a workflow span whose parent is missing from the trace), so the sink SHALL make the workflow span a true root while still adopting the chosen trace identifier.

Browser application, domain, and API-adapter code SHALL NOT import an OpenTelemetry package. Only the observability adapter and the composition root MAY import OTel, as enforced by `eslint.architecture.mjs`. Experimental browser auto-instrumentation SHALL NOT be a foundational dependency.

**Verification:** Run each gateway use case with a recording publisher plus the trace sink; assert one workflow span per invocation, one child span per dependency attempt, and a span status that matches the terminal outcome. Confirm no span is emitted for a rerender. Run the architecture lint and confirm an OTel import outside the approved paths fails.

#### Scenario: Workflow Produces One Span

- GIVEN a user runs a gateway workflow (for example list, provision, rename, or delete)
- WHEN the workflow publishes its started probe and one terminal probe
- THEN the trace sink SHALL produce exactly one workflow span with a status derived from the terminal outcome
- AND each control-plane dependency attempt SHALL produce one child span

### Requirement: WEB-TRACE-02 -- Same-Origin Browser Telemetry Export

The browser SHALL export spans to a same-origin BFF telemetry endpoint using OTLP over HTTP. The browser SHALL NOT export directly to a collector origin and SHALL NOT require a Content Security Policy `connect-src` value other than `'self'`. The BFF SHALL expose the ingest endpoint, apply a request-body size limit, require the same session and CSRF protection as other state-changing browser requests, and forward accepted spans to the configured collector. The endpoint SHALL reject a body that is not valid OTLP.

**Verification:** Load the built console behind the BFF with the enforcing CSP; confirm the browser exporter posts to the same-origin path and that no cross-origin telemetry request is attempted. Post an oversized and a malformed body and confirm rejection. Confirm the BFF forwards a well-formed body to the collector.

#### Scenario: Browser Exports Through the BFF

- GIVEN the console has produced one or more spans
- WHEN the browser exporter flushes
- THEN it SHALL post OTLP data to the same-origin BFF telemetry endpoint
- AND the BFF SHALL forward the accepted spans to the configured collector
- AND no telemetry request SHALL go to a cross-origin destination

### Requirement: WEB-TRACE-03 -- Browser Outbound Trace Context

The browser SHALL set the W3C `traceparent` header, and `tracestate` when present, on every outbound `/api/*` request at the single correlated-fetch chokepoint. The `traceparent` value SHALL reference the active workflow span so the BFF and API spans join the same trace. The browser SHALL keep sending the existing `x-hypershell-correlation-id` header. The trace context and the correlation identifier SHALL both appear in the `DomainProbeContext` so probe consumers can join a workflow to its trace.

**Verification:** Run a representative browser-to-API workflow; capture the outbound request and assert a well-formed `traceparent` that references the workflow span, plus the correlation header. Confirm the probe context carries the same trace identifier.

### Requirement: WEB-TRACE-04 -- BFF W3C Trace Context Propagation

The BFF SHALL treat an inbound `traceparent`/`tracestate` pair as untrusted input. It SHALL extract valid context, continue that trace, and set a valid `traceparent` (and forward a valid `tracestate`) on the upstream API request. When the inbound context is absent or malformed, the BFF SHALL start a new trace and set a valid `traceparent`; it SHALL NOT forward a malformed value. The upstream request header allowlist SHALL include `traceparent` and `tracestate`. The BFF SHALL keep validating, echoing, and propagating `x-hypershell-correlation-id` as it does today.

**Verification:** Send requests with valid, absent, and malformed inbound trace context; assert the upstream request always carries a valid `traceparent`, a malformed value is replaced rather than forwarded, and the correlation identifier is preserved. Confirm `traceparent` and `tracestate` are in the upstream allowlist.

#### Scenario: BFF Continues an Inbound Trace

- GIVEN a browser request arrives with a valid `traceparent`
- WHEN the BFF proxies the request to the API
- THEN the upstream request SHALL carry a `traceparent` that continues the same trace
- AND a valid inbound `tracestate` SHALL be forwarded

#### Scenario: BFF Replaces Malformed Context

- GIVEN a browser request arrives with a malformed `traceparent`
- WHEN the BFF proxies the request to the API
- THEN the BFF SHALL start a new trace and set a valid `traceparent`
- AND it SHALL NOT forward the malformed value

### Requirement: WEB-TRACE-05 -- BFF Server Spans

The BFF SHALL produce one server span per proxied `/api/*` request as a child of the extracted inbound context, and export it by OTLP to the configured collector. The BFF OTel SDK SHALL be initialized once in the server bootstrap path, which is exempt from the observability import ban. Product and route code SHALL NOT call the OTel API directly; only the observability adapter, the composition root, and the bootstrap MAY. Span attributes SHALL record the templated route, the request method, the upstream outcome class, and the correlation identifier. The BFF SHALL keep its existing structured request log.

The span name SHALL combine the request method and the templated route (for example `GET /api/hypershell/v1/gateways/{id}`), so Jaeger groups spans by endpoint. The catch-all proxy pattern (for example `/api/*`) SHALL NOT be the span name or the `http.route` value. The templated route SHALL collapse every resource identifier to a bounded placeholder and SHALL NOT carry a query string, so cardinality stays fixed per `WEB-TRACE-07`.

**Verification:** Proxy a representative request and confirm one BFF server span that is a child of the inbound context, carries the templated-route and outcome attributes, is named by method and templated route with resource identifiers collapsed, and reaches the collector. Confirm route and product code contain no direct OTel API calls.

### Requirement: WEB-TRACE-06 -- Configurable Export and Sampling

The collector endpoint and the trace sample rate SHALL be deployment configuration, validated at BFF startup through the existing configuration schema. Configuration SHALL be separate from code. When tracing configuration is absent, the BFF SHALL start normally with tracing disabled and SHALL NOT fail readiness. A browser-visible value SHALL be limited to the non-sensitive, allowlisted schema required by `WEB-DEPLOY-02`; a collector origin or secret SHALL NOT be embedded in the browser bundle.

**Verification:** Start the BFF with valid, absent, and invalid tracing configuration; confirm valid configuration exports to the collector, absent configuration disables tracing without failing readiness, and invalid configuration fails startup with a clear message. Inspect the browser bundle for an embedded collector origin or secret.

### Requirement: WEB-TRACE-07 -- Trace Privacy and Cardinality

Spans and their attributes SHALL follow the privacy and cardinality rules of `standards/ui/domain-observability.spec.md` (`UI-OBS-07`). Tokens, cookies, credentials, secrets, raw headers, request or response bodies, user-entered content, and raw resource identifiers in span names SHALL be prohibited. Stable route templates, bounded enums, and error classes SHALL replace raw URLs and messages. A high-cardinality identifier SHALL NOT become a span name segment or propagated baggage. When an API failure returns an operation identifier, the matching workflow and dependency terminal spans SHALL retain it so support can join a user-visible failure to server evidence.

**Verification:** Run redaction tests with seeded secrets and user data across the browser sink and the BFF span exporter. Confirm span names use route templates, attributes are bounded, no prohibited value appears, and an API operation identifier is retained on failure spans.

### Requirement: WEB-TRACE-08 -- Trace Consumer in the Probe Catalog

The gateway probe catalog SHALL declare `trace` as an allowed consumer for every probe the trace sink reads, and the observability import allowlist SHALL permit the trace sink to consume those probes. The catalog SHALL remain in agreement with the typed probe union and the sink mappings. A probe that no consumer reads and a duplicate probe name SHALL NOT be introduced.

**Verification:** Compare the catalog with the probe union and the trace sink mapping; confirm every probe the sink reads lists `trace` in `allowedConsumers`, and that catalog and code agree in CI.

### Requirement: WEB-TRACE-09 -- Bounded Delivery and Flush

The browser trace sink and the BFF exporter SHALL each own a bounded buffer and an explicit flush policy; they SHALL NOT grow without limit. The browser SHALL flush on page hide and visibility change so spans are not lost on navigation or tab close. A tracing export failure SHALL be best-effort: it SHALL NOT change a workflow result and SHALL surface through the existing bounded delivery-failure diagnostic rather than a recursive publication. Fan-out SHALL still attempt every other sink when the trace sink fails.

**Verification:** Inject a trace sink and an exporter that throw, block, and overflow; confirm other sinks still receive the probe, the workflow result is unchanged, memory stays bounded, and the failure is visible without recursion. Hide the page mid-workflow and confirm buffered spans flush.

### Requirement: WEB-TRACE-10 -- End-to-End Trace in Development

The local development environment SHALL provide a collector and a Jaeger instance so a developer can view one trace that joins the browser and the BFF (and the API server once it is instrumented). The development collector endpoint SHALL be supplied to the BFF by configuration. The details of the development deployment are defined in `platform/local-development.spec.md`.

**Verification:** Run the local environment, complete a representative gateway workflow in the browser, and confirm one trace in Jaeger that contains the browser workflow span and the BFF server span joined by W3C Trace Context.

#### Scenario: Developer Views the Trace

- GIVEN the local development environment is running with the collector and Jaeger
- WHEN a developer completes a gateway workflow in the browser
- THEN one trace SHALL be visible in Jaeger
- AND it SHALL contain the browser workflow span and the BFF server span joined by the same trace identifier

### Requirement: WEB-TRACE-11 -- Automated End-to-End Trace Verification

The end-to-end trace defined by `WEB-TRACE-10` SHALL be verified automatically, not only by manual inspection. The CI end-to-end workflow SHALL bring the cluster up with tracing enabled, drive a representative gateway workflow through the deployed browser console, and assert that Jaeger holds one trace joining the browser and the BFF. The check SHALL confirm the browser spans use the bounded workflow and dependency span names and share a single trace identifier with the BFF server span. A missing trace, a browser-only trace, or a BFF-only trace SHALL fail the workflow. The verification details and its place in the e2e suite are defined in `platform/e2e-testing.spec.md`.

**Verification:** Run the e2e workflow against a cluster with tracing enabled; confirm the trace check drives a browser workflow, finds a cross-service trace in Jaeger, and fails when no such trace is present.

#### Scenario: CI Verifies the Cross-Service Trace

- GIVEN the e2e cluster is running with tracing enabled and the console is reachable
- WHEN the trace verification drives a gateway workflow in a real browser
- THEN it SHALL find one trace in Jaeger whose spans include a bounded browser workflow span and the BFF server span
- AND those spans SHALL share the same trace identifier
- AND the workflow SHALL fail if no such cross-service trace appears within a bounded polling window

## Design Decisions

| Decision | Rationale |
| --- | --- |
| Derive spans from domain probes, not auto-instrumentation | Keeps one authoritative telemetry model, satisfies the domain-observability standard, and avoids the bundle, privacy, and stability risk that `WEB-OBS-02` cautions against |
| Browser exports through a same-origin BFF endpoint | Keeps the collector origin out of the browser, works with the strict `connect-src 'self'` CSP, and reuses the BFF session and CSRF controls |
| BFF owns W3C Trace Context propagation | The BFF is the trust boundary; it validates or replaces untrusted inbound context and sets a valid value upstream so the API can join the trace |
| Tracing configuration is optional and validated at startup | Configuration is separate from code; a deployment without a collector starts normally with tracing disabled |
| Trace is a probe consumer in the catalog | The catalog stays the single source of truth for who consumes each probe, per `UI-OBS-10` |

## Primary Basis

- `standards/ui/domain-observability.spec.md`
- `web-console/architecture.spec.md` (`WEB-OBS-01`, `WEB-OBS-02`, `WEB-BFF-01`, `WEB-DEPLOY-02`, `WEB-SEC-01`)
- [OpenTelemetry JavaScript](https://opentelemetry.io/docs/languages/js/)
- [OpenTelemetry semantic conventions](https://opentelemetry.io/docs/concepts/semantic-conventions/)
- [W3C Trace Context](https://www.w3.org/TR/trace-context/)
- [OTLP specification](https://opentelemetry.io/docs/specs/otlp/)
