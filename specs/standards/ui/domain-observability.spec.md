# Domain-Oriented UI Observability Standard

**Status:** Active
**Applies to:** HyperShell web-console and BFF application workflows, domain transitions, external dependency calls, recovery decisions, and production observability consumers

## Purpose

Describe system behavior as typed domain facts once, then fan those facts out to logs, metrics, traces, product-health analysis, and test recorders without coupling application code to a telemetry vendor. A domain probe is an immutable observability fact; it is not a Kubernetes health probe and SHALL NOT drive business behavior.

## Requirements

### Requirement UI-OBS-01: Complete Domain Coverage

Every invocation of an application use case SHALL publish one started probe and exactly one terminal probe with a stable outcome such as `succeeded`, `failed`, `cancelled`, `denied`, or `conflicted`. Meaningful domain state transitions, external-dependency attempts and outcomes, and caught failure or recovery decisions SHALL also publish probes. Renders, pointer movements, and pure computations SHALL NOT emit probes unless they independently represent a documented domain fact.

**Verification:** Maintain a workflow-to-probe matrix covering every application entry port, transition, dependency port, and recovery branch. Exercise success, validation, denial, conflict, cancellation, timeout, partial failure, retry, and recovery paths; assert one terminal outcome per logical invocation and no emissions caused only by React rerenders.

### Requirement UI-OBS-02: Typed, Versioned Facts

Each probe SHALL be a member of a closed, typed, discriminated schema with a stable domain name, schema version, occurrence time, outcome or fact fields, and correlation context. Names and fields SHALL use domain language rather than log prose or vendor terminology. Published probes SHALL be immutable; incompatible semantic or field changes SHALL create a new schema version.

**Verification:** Type-check the complete probe union, validate serialized probes at adapter boundaries, snapshot the public schemas, and reject free-form message-only events, unversioned payloads, mutable post-publication values, unknown fields, and incompatible schema changes.

### Requirement UI-OBS-03: Probe Port and Isolation

Application use cases SHALL publish through an application-owned `DomainProbePublisher`-equivalent driven port. Domain/application code SHALL NOT import logging, metrics, tracing, analytics, browser telemetry, or vendor SDKs. Probe consumers SHALL observe facts only: they SHALL NOT mutate domain state, determine a use-case result, or become an implicit event bus for business processing.

**Verification:** Enforce import restrictions and trace each publication to the probe port. Inspect consumers for callbacks into application behavior, state mutation, or returned values that influence the workflow; use a recording fake for isolated use-case tests.

### Requirement UI-OBS-04: Fan-Out by Construction

The production publisher SHALL fan each probe out to a configured collection of independent sinks. Adding, removing, filtering, or replacing a log, metric, trace, product-health, transport, or test consumer SHALL require no producer or use-case change. Each sink SHALL receive the same immutable fact and MAY perform only sink-specific filtering, aggregation, redaction, or format conversion.

**Verification:** Configure at least two recording sinks, publish one probe, and assert both receive the same value exactly once. Add and remove a sink without changing producer code, and fail switch statements in use cases that route probes by consumer or telemetry vendor.

### Requirement UI-OBS-05: Explicit Delivery Semantics

Fan-out SHALL attempt every eligible sink even when another sink fails. Best-effort observability failure SHALL NOT change the domain result, but SHALL surface through a bounded, non-recursive structured diagnostic and health signal. Buffering, concurrency, ordering, backpressure, overflow, retry, and shutdown-flush behavior SHALL be explicit and bounded for each runtime. Security or compliance audit records SHALL use a separately specified durable path or outbox when loss cannot be tolerated; generic probe fan-out SHALL NOT imply audit durability.

**Verification:** Inject a sink that throws, blocks, overflows, and fails during shutdown; confirm other sinks still receive the probe, the workflow result is unchanged, memory remains bounded, and delivery failure becomes visible without recursive publication. Test and document the declared ordering, retry, and flush guarantees.

### Requirement UI-OBS-06: No Raw or Bypassing Telemetry

Production browser and BFF code SHALL NOT call any `console.*` method or equivalent ad hoc diagnostic. Raw standard output/error MAY be accessed only by a named structured log or emergency adapter. Product and domain behavior SHALL NOT call Fastify request loggers, OpenTelemetry APIs, metrics clients, analytics clients, or vendor exporters directly; only observability adapters, composition/bootstrap, and framework-managed technical access instrumentation MAY access those facilities. Technical access signals SHALL NOT substitute for domain probes.

**Verification:** Make lint or a static architecture check fail prohibited console/standard-stream calls and observability-vendor imports outside approved adapter/bootstrap paths. Inspect framework logger use and prove every product workflow fact enters through the probe publisher. Route pre-composition fatal diagnostics through a named structured emergency adapter.

### Requirement UI-OBS-07: Privacy, Security, and Cardinality

Probe schemas SHALL minimize data and classify every field before release. Tokens, cookies, credentials, secrets, raw headers, request or response bodies, user-entered content, and unnecessary personal data are prohibited. Stable route templates, bounded enums, error classes, and resource types SHALL replace raw URLs and messages. High-cardinality identifiers SHALL NOT become metric labels or propagated baggage; any identifier retained in restricted logs or traces SHALL have a documented operational need, access control, retention, and redaction policy.

**Verification:** Run schema allowlist and redaction tests with seeded secrets and user data. Inspect every sink's transformations, metric label cardinality, trace attributes, baggage, access policy, and retention. Fail raw URLs, uncontrolled error strings, payload capture, and unapproved identifiers.

### Requirement UI-OBS-08: Correlation and Causality

Probe context SHALL preserve W3C trace context and a stable correlation or operation identifier across browser, BFF, SDK, API, asynchronous operation polling, and recovery paths where available. Dependency-attempt probes SHALL identify their parent logical invocation and retry attempt without producing duplicate logical terminal outcomes. Propagated baggage SHALL be treated as untrusted input and SHALL contain no sensitive data.

The application invocation context SHALL travel through every application entry port and external-dependency port used by that invocation. Infrastructure adapters SHALL propagate an approved, validated correlation header or W3C trace context on the outbound request. When an API failure supplies an operation identifier, the matching dependency and workflow terminal probes SHALL retain it so support can join the user-visible failure to server evidence.

**Verification:** Execute a representative browser-to-API workflow with retry and polling, then join its domain, dependency, BFF access record, outbound API request, log, metric-exemplar, and trace outputs using approved context. Assert the exact correlation value on the infrastructure adapter's outbound request and the API operation identifier on failure probes. Confirm cancellation and errors retain causality, retry attempts are distinguishable, malformed inbound context is replaced rather than forwarded, and no sensitive baggage crosses a trust boundary.

### Requirement UI-OBS-09: Consumer-Derived Signals

Structured logs, metrics, traces, critical-task monitoring, and product-health analysis SHALL be derived by sinks from domain probes and OpenTelemetry semantic conventions where applicable. Producers SHALL report the domain fact at its authoritative source rather than precomputing vendor-specific log lines, metric names, span formats, or analytics payloads. Metric dimensions SHALL be bounded and operational alerts SHALL map back to documented domain outcomes.

**Verification:** Select representative probes and trace their sink-specific mappings. Confirm a mapping change is confined to a sink, logs are structured, metrics have bounded labels, spans preserve causality, critical-task results match domain outcomes, and alerts identify an owner and remediation path.

### Requirement UI-OBS-10: Probe Catalog and Ownership

The repository SHALL maintain a discoverable catalog generated from or checked against the typed schemas. For every probe it SHALL identify the owning domain, trigger, version, field classifications, allowed consumers, delivery class, and related critical task or service objective. Unused and duplicate probes SHALL be removed through a versioned migration rather than left as competing facts.

**Verification:** Compare the catalog with the schema union, publisher call sites, and sink mappings. Fail undocumented probes, documented probes with no publisher, semantically duplicate names, missing owners or classifications, and consumers outside the allowlist.

### Requirement UI-OBS-11: Observable Architecture Proof

Tests SHALL prove probe coverage, schema safety, fan-out independence, delivery-failure isolation, correlation, and absence of duplicate emission. CI SHALL enforce the raw-console ban, observability import boundary, schema/catalog agreement, and privacy/cardinality rules.

**Verification:** Run use-case tests with a recording publisher; fan-out tests with two healthy sinks and one failing sink; schema, redaction, cardinality, retry, cancellation, rerender, and shutdown tests; static console/import checks; and one correlated end-to-end workflow. Record any behavior that remains unverified in production.

## Primary Basis

- [OpenTelemetry signals](https://opentelemetry.io/docs/concepts/signals/)
- [OpenTelemetry semantic conventions](https://opentelemetry.io/docs/concepts/semantic-conventions/)
- [OpenTelemetry baggage](https://opentelemetry.io/docs/concepts/signals/baggage/)
- [W3C Trace Context](https://www.w3.org/TR/trace-context/)
