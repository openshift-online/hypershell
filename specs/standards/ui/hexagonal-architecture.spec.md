# Narrow UI Hexagonal Architecture Standard

**Status:** Active
**Applies to:** HyperShell web-console and BFF application workflows, domain decisions, and integrations with external or nondeterministic capabilities

## Purpose

Keep product behavior testable and independent of React, TanStack Query, Fastify, generated clients, storage, and telemetry vendors without wrapping every function in an interface. The invariant protects the application boundary only where code coordinates a domain workflow or crosses an external-effect boundary.

## Required Shape

```text
React / React Router / TanStack Query        Fastify routes
                 \                            /
                  driving adapters and entry ports
                               |
                  application use cases + domain
                               |
                 application-owned driven ports
                               |
         SDK / auth / storage / probe / runtime adapters
```

The composition root is outside the application and is the only place that assembles concrete adapters into use cases.

## Requirements

### Requirement UI-HEX-01: Narrow Scope

The hexagonal boundary SHALL cover code that coordinates an application use case, makes a domain decision or state transition, or accesses an external or nondeterministic capability. Pure calculations, value objects, schemas, formatting, local component state, and presentational composition SHALL remain ordinary direct dependencies unless they independently cross such a boundary. Contributors SHALL NOT create one port per function merely to claim architectural compliance.

**Verification:** Classify changed non-presentation modules as domain, application, driving adapter, driven adapter, composition, or pure support code. Trace each port to a real workflow or effect and reject ports, interfaces, factories, or wrappers that add no substitutable boundary.

### Requirement UI-HEX-02: Inward Dependency Rule

Driving adapters SHALL invoke application entry ports or use cases; application use cases SHALL own the driven-port contracts they require; and infrastructure adapters SHALL implement those contracts. Domain and application modules SHALL NOT import React, React Router, TanStack Query, PatternFly, Fastify, browser globals, generated SDKs, transport clients, persistence clients, or telemetry vendors. Only composition roots MAY depend on both concrete adapters and application code.

**Verification:** Enforce layer-specific import rules in lint or an architecture test. Produce a dependency graph for changed workflows and fail cycles, inward-to-outward imports, direct framework or vendor imports in domain/application code, and concrete-adapter construction outside a composition root.

### Requirement UI-HEX-03: External-Effect Ports

Application access to APIs, authentication or session state, persistent or cross-session storage, clocks, randomness, runtime configuration or feature decisions, and domain-probe publication SHALL pass through application-owned driven ports. Navigation, transient view state, rendering, query-cache policy, and deterministic utilities SHALL remain in the presentation or owning layer unless an application use case genuinely depends on them.

**Verification:** Search application and domain modules for `fetch`, generated clients, browser storage, cookies, clocks, random sources, environment access, and observability APIs. Confirm each genuine application effect uses an injected port and each excluded presentation concern has not been abstracted speculatively.

### Requirement UI-HEX-04: Purpose-Shaped Ports

Ports SHALL describe a cohesive capability or purposeful domain conversation in application language. They SHALL accept and return stable domain or application types and SHALL be no broader than their consumers require. Ports SHALL NOT mechanically mirror an entire SDK, expose raw transport responses, become generic repositories or service locators, or multiply into one-method interfaces without distinct substitution needs.

**Verification:** Compare every port with its use cases and adapter. Flag unused members, transport-shaped names or types, catch-all `Client` or `Repository` surfaces, generic lookup bags, pass-through adapters, and repeated mappings that do not protect a semantic boundary.

### Requirement UI-HEX-05: API Adapter Isolation

Only the API infrastructure adapter and its composition root MAY import or construct the generated HyperShell SDK. The adapter SHALL preserve cancellation, pagination, concurrency controls, operation identifiers, authentication context, and typed failure semantics. It SHALL translate transport details into application errors or values when their meanings differ, without duplicating generated DTOs solely for layering.

Collection ports SHALL accept the normalized page, page size, search, filter, and sort values required by their consumers and return items with authoritative page metadata. An adapter SHALL NOT exhaust every upstream page merely to recreate pagination in the presentation layer, and an incomplete upstream page sequence SHALL NOT be returned as a successful complete collection.

**Verification:** Search all SDK imports and constructions and confirm they are confined to the adapter/composition surface. Contract-test success, typed errors, abort, one-page collection mapping, search and sort propagation, authoritative page metadata, inconsistent or partial page responses, conditional requests, operation polling, credential propagation, and correlation propagation against the generated client. Fail a list adapter that requests a second page without an explicit application invocation for that page.

### Requirement UI-HEX-06: TanStack Query Boundary

TanStack Query SHALL remain a presentation-side driving adapter for server-state caching and synchronization. Query and mutation functions SHALL call application use cases rather than SDK or transport clients; query keys, cache state, invalidation, freshness, and UI retry policy SHALL remain outside domain/application code. Query `AbortSignal` values SHALL propagate through the use case and port to the SDK. A request path SHALL have one explicit retry owner so TanStack and an adapter do not compound retries.

**Verification:** Trace each query and mutation from hook or options factory through the use case, port, adapter, and SDK. Fail direct SDK calls, TanStack types below the presentation boundary, discarded abort signals, duplicated caches, cache operations inside use cases, and layered retry loops.

### Requirement UI-HEX-07: Explicit Composition and Lifetimes

Each runtime SHALL have an explicit composition root that wires ports to adapters and declares their lifetimes. Mutable global service locators and hidden singleton lookup are prohibited. Browser-wide stateless adapters MAY be shared; BFF request-specific identity, authorization, correlation, and cancellation context SHALL remain request-scoped and SHALL NOT leak between users or requests.

**Verification:** Inspect bootstrap and request construction, enumerate adapter lifetimes, and test concurrent requests with different identities and cancellation. Fail implicit mutable registries, imports that instantiate services as side effects, and request data captured by shared adapters.

### Requirement UI-HEX-08: BFF Boundary

Fastify routes and hooks SHALL act as driving or infrastructure adapters: they MAY perform protocol parsing, HTTP response mapping, framework lifecycle work, and technical access handling, but domain decisions and multi-step product workflows SHALL enter application use cases. Upstream APIs, authentication/session persistence, and domain observability used by those workflows SHALL remain behind driven ports.

A production BFF that hosts a same-origin browser API SHALL exercise that proxy in its production-shaped test path; a development-server proxy is not equivalent evidence. SPA document routing SHALL be derived from, or contract-tested against, the browser route manifest so adding, changing, or removing a browser route cannot leave a stale hand-maintained server allowlist. API, asset, and health namespaces SHALL remain excluded from SPA fallback behavior.

**Verification:** Trace representative BFF routes from request to response. Start the production BFF with a recording upstream, exercise a representative API read and mutation, and verify method, path, query, bounded body, safe status, approved headers, correlation, timeout, and cancellation behavior. Request every declared static browser route plus representative parameterized routes directly from the BFF and verify the application document, while unknown API, asset, health, and server routes retain explicit non-SPA behavior. Flag domain branching in handlers, direct upstream clients or persistence in application code, framework request/reply types crossing entry ports, and abstractions around incidental Fastify helpers that do not protect application behavior.

### Requirement UI-HEX-09: Boundary Proof

Every application use case SHALL run in isolation with fake or in-memory driven adapters. Every production adapter SHALL have contract tests against its port, and each driving integration SHALL test its mapping to the use case. CI SHALL enforce the dependency rule and direct-import restrictions.

**Verification:** For each changed workflow, run an isolated use-case test, relevant adapter contract tests, driving-adapter integration tests, and the import-boundary check. Demonstrate at least one alternate adapter without changing the use case and record any boundary that cannot yet be mechanically enforced.

## Primary Basis

- [Alistair Cockburn: Hexagonal Architecture](https://alistair.cockburn.us/hexagonal-architecture/)
- [TanStack Query: Query cancellation](https://tanstack.com/query/latest/docs/framework/react/guides/query-cancellation)
- [TanStack Query: Query retries](https://tanstack.com/query/latest/docs/framework/react/guides/query-retries)
