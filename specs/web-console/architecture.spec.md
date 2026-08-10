# Web Console Architecture

**Date:** 2026-08-05
**Status:** Active
**Applies to:** `components/web-console`, `packages/gateway-management-ui`, the browser-facing TypeScript SDK surface, and web-console build, test, deployment, and operations workflows

## Purpose

Define the technical architecture for a focused HyperShell console that lists, provisions, and connects to OpenShell gateways. This specification selects the implementation stack, establishes browser/server trust boundaries, and defines the evidence required for a secure, accessible, resilient, and maintainable console.

The web console SHALL also satisfy every applicable requirement in `standards/security/` and `standards/ui/`. This specification narrows implementation choices; it does not replace those standards.

## Architecture

```text
Browser
  HyperShell React single-page application host
  React Router + application shell + authentication integration
         │
         │ workspace package API and host adapters
         ▼
  Gateway management UI package
  PatternFly 6 + TanStack Query gateway workflows
         │
         │ same-origin HTTPS, session cookie, CSRF protection
         ▼
Web Console BFF
  Fastify static server + OIDC session + API proxy
         │
         │ server-side bearer token and trace context
         ▼
HyperShell REST API
         │
         ▼
PostgreSQL / Control Plane / Managed Clusters
```

In deployed environments, the browser SHALL communicate with the Backend-for-Frontend (BFF) on the same origin. The BFF SHALL own the OAuth/OIDC tokens, session, browser-facing security controls, static application delivery, and authenticated REST proxy. Initial local development MAY use the same `/api` browser contract through a Vite development proxy while the API server runs in its no-auth development mode. The React application SHALL NOT communicate directly with an OAuth token endpoint or persist bearer tokens.

The HyperShell React application SHALL be the standalone host for the canonical gateway management UI package. The host SHALL own product-level routing, the application shell, authentication/session integration, localization and TanStack Query provider instances, and deployment-specific API construction. The gateway management UI package SHALL own the gateway-domain pages and workflows and SHALL receive host services through explicit typed integration contracts.

## Selected Stack

| Concern                        | Selection                                               | Version line               |
| ------------------------------ | ------------------------------------------------------- | -------------------------- |
| Browser UI                     | React and React DOM                                     | 19.2                       |
| Language                       | TypeScript, strict mode                                 | 6.x                        |
| Routing/application framework  | React Router Framework Mode with SPA output             | 8.x                        |
| Development and build          | Vite                                                    | 8.x                        |
| Design system                  | PatternFly React                                        | 6.x                        |
| REST server state              | TanStack Query                                          | 5.x                        |
| Forms                          | React Hook Form                                         | 7.x                        |
| Runtime validation             | Zod                                                     | 4.x                        |
| Localization                   | React Intl / FormatJS                                   | 10.x                       |
| Unit and integration tests     | Vitest, Testing Library, `user-event`, MSW              | compatible stable releases |
| Component development          | Storybook React with Vite                               | 10.x                       |
| Browser automation             | Playwright                                              | 1.x                        |
| BFF runtime                    | Node.js LTS                                             | 24.x initially             |
| BFF framework                  | Fastify                                                 | 5.x                        |
| OIDC client                    | `openid-client`                                         | 6.x                        |
| Package manager                | pnpm                                                    | 11.x initially             |
| Formatting and static analysis | Prettier, ESLint flat config, typed `typescript-eslint` | compatible stable releases |

Direct dependency declarations SHALL use exact versions selected when the implementation or upgrade is committed. The version lines above define compatibility boundaries, not permission to use ranges in `package.json`. Every selected version and its complete resolved graph SHALL pass repository age, provenance, vulnerability, license, compatibility, and test gates. A newer major requires a deliberate specification update or an approved compatibility decision before adoption.

## Requirements

### Requirement WEB-ARCH-01: Client-Rendered Application

The web console SHALL use React Router Framework Mode configured with `ssr: false`. Routes SHALL use route modules, lazy code splitting, route error boundaries, loading behavior, and document metadata provided by the selected framework. Production delivery SHALL support direct navigation and refresh of every application route without returning a false 404.

Server-side rendering, React Server Components, and a Next.js application SHALL NOT be introduced unless an accepted architecture decision identifies a concrete requirement that SPA mode cannot meet.

**Verification:** Inspect the React Router configuration and production artifact. Open, refresh, and recover from an injected failure on representative nested routes. Confirm that the client bundle does not contain server-only code or secrets.

#### Scenario: Deep Link

- GIVEN an authenticated user opens `/gateways/{gateway_id}` directly
- WHEN the BFF receives the browser request
- THEN it SHALL return the SPA document
- AND React Router SHALL render the gateway route after checking session and resource access
- AND API and asset paths SHALL NOT fall back to the SPA document

### Requirement WEB-ARCH-02: Gateway Management Experience

HyperShell SHALL provide one focused application experience. Its landing page SHALL list every gateway visible to the authenticated user and provide gateway provisioning without a separate directory, administration shell, overview, or cluster collection. Gateway details SHALL combine operational configuration with direct OpenShell console and CLI connection actions.

The console SHALL initially assume that authenticated users can view and provision gateways. The API SHALL remain the authorization boundary for every read and mutation; route presence and visible controls SHALL NOT grant access. Capability-based UI controls MAY be introduced when the API exposes an authorization contract, without adding a separate `/admin` route hierarchy.

The masthead and browser page titles SHALL use HyperShell product branding and the shared product mark. OpenShell terminology SHALL identify gateways, consoles, and CLI connection workflows rather than the enclosing web application.

Resource collections SHALL use a shared PatternFly table pattern with client-side search, sortable data columns, result counts, pagination, responsive row presentation, and explicit empty and no-match states. API-backed pagination and filtering MAY replace the client-side behavior without changing the interaction pattern when collection size requires it.

Every resource collection page SHALL expose a PatternFly refresh action in the page heading. The icon-only action SHALL have a localized accessible label, indicate or disable itself while a refresh is active, refetch the collection through its query boundary, and preserve the user's current filter, sort, and pagination state.

The initial route model SHALL include:

```text
/login
/
/gateways/new
/gateways/:gatewayId
```

Removed `/admin` paths and the former directory experience SHALL NOT be retained as aliases or redirects. Unsupported paths SHALL use the standard not-found experience so the route model remains unambiguous.

The `gatewayId` SHALL be included in gateway-detail query keys, mutations, authorization requests, breadcrumbs, and relevant telemetry dimensions. Route presence, separate shells, and hidden controls SHALL NOT be treated as authorization enforcement. The API SHALL independently authorize every operation and enforce the relationships and scope required by `standards/security/security.spec.md`.

**Verification:** Inspect route definitions and query keys. Confirm that `/` is the only gateway collection, provides provisioning, and does not expose a separate directory, administration shell, or cluster collection. Refresh each resource collection and verify that its query reruns without resetting table state. Attempt unauthorized gateway reads and mutations by changing route parameters and request bodies; the server rejects the operation without exposing protected data.

### Requirement WEB-ARCH-03: Source and Runtime Boundaries

Browser host, gateway management UI package, BFF, shared, generated SDK, and test code SHALL have explicit module boundaries and TypeScript configurations. Browser modules SHALL NOT import Node.js APIs, server environment access, OIDC tokens, session implementation, or server-only dependencies. BFF modules SHALL NOT import browser component code. The gateway management UI package SHALL NOT import application route modules, application-shell components, BFF modules, authentication/session implementations, deployment configuration, or the generated SDK.

TypeScript SHALL use strict checking. Browser and server compilation SHALL use modern ESM. Tests SHALL have a separate configuration so test globals and Node types do not leak into production browser code.

**Verification:** Run browser host, gateway management UI package, and server type checks independently, inspect build outputs, and use boundary lint rules or equivalent tests to reject prohibited imports.

### Requirement WEB-PKG-01: pnpm Workspace

HyperShell JavaScript packages SHALL use one repository-root pnpm workspace. The workspace SHALL include `components/sdk-typescript`, `packages/gateway-management-ui`, `components/web-console`, the web-console BFF, and the web-console domain probes; use one root `pnpm-lock.yaml`; and use the `workspace:` protocol for internal package dependencies. The root `package.json` SHALL be private and SHALL declare an exact pnpm version in `packageManager`.

After migration, npm, Yarn, Bun, nested lockfiles, and per-package installation workflows SHALL NOT be used for repository JavaScript packages. `shamefullyHoist` SHALL NOT be enabled. Packages SHALL declare every dependency they import.

**Verification:** Run a clean root install, enumerate workspace projects, and inspect manifests and tracked files. Confirm that the web console resolves the local SDK through `workspace:` and that no npm/Yarn/Bun lockfile remains.

#### Scenario: Workspace SDK Consumption

- GIVEN the console declares the generated SDK as a workspace dependency
- WHEN a clean frozen install runs
- THEN pnpm SHALL resolve the repository SDK rather than a registry package
- AND a missing or incompatible local SDK version SHALL fail resolution

#### Scenario: Workspace Gateway Management UI Package Consumption

- GIVEN the web console declares the gateway management UI package as a workspace dependency
- WHEN a clean frozen install and production build run
- THEN the console SHALL resolve the repository gateway management UI implementation through its declared public exports
- AND a missing or incompatible local gateway management UI version SHALL fail resolution

### Requirement WEB-PKG-04: Reusable Gateway Feature Package

The canonical gateway management interface SHALL live in the private `packages/gateway-management-ui` workspace package. The package SHALL expose an explicit, documented public surface for its gateway collection, gateway detail, and gateway provisioning pages and for the typed host integration contract they require. The standalone web console SHALL consume those public exports and SHALL NOT retain copied gateway page, mutation, query, table, dialog, or gateway-specific presentation implementations.

The gateway management UI package SHALL own:

- gateway application use cases, their entry-port contract, stable application values, driven gateway-port contract, and typed gateway-probe schemas and catalog;
- gateway and placement-cluster query keys, server-state queries, mutations, and cache invalidation behavior;
- gateway list, detail, provisioning, placement selection, rename, delete, connection, loading, empty, validation, error, success, and recovery presentation;
- canonical gateway-domain components and feature-scoped shared resource components; and
- the localized message descriptors used by those workflows.

The host SHALL own:

- application route declarations, route-parameter parsing, URL/history integration, and the product application shell;
- authentication, session, authorization bootstrap, BFF, API origin, SDK client construction, and the API, workflow-runtime, and domain-probe infrastructure adapters;
- the shared TanStack Query client and React Intl provider instances; and
- product-wide notifications, telemetry, feature flags, and runtime configuration unless an explicit package integration contract delegates a narrow behavior.

The package SHALL receive a purpose-shaped gateway application entry port and navigation behavior through typed props or a typed provider contract. Its application use-case factory SHALL receive an application-owned gateway control-plane port, a workflow-runtime port for time and invocation identity, and a typed domain-probe publisher. The gateway entry and driven ports SHALL express gateway tasks and stable application values, including the managed clusters available as gateway placement targets, rather than mirror SDK resources, transport DTOs, pagination, or a broad generated client. The host API adapter SHALL translate between the driven port and the generated gateway and managed-cluster SDK resources, including any reconciler-owned request defaults, and the host composition root SHALL wire the adapters into the package use cases. Every gateway use-case invocation and API dependency attempt SHALL publish the typed started and terminal facts required by `standards/ui/domain-observability.spec.md`. The package SHALL NOT import or construct the generated SDK, configure integrations through mutable module globals, assume a particular deployment origin, construct a second Query client, construct a second localization provider, or require a particular host router. Shared runtime libraries including React, React DOM, PatternFly, TanStack Query, and React Intl SHALL resolve to host-compatible singleton versions through peer dependency contracts or an equivalent workspace-enforced mechanism.

The internal package MAY expose TypeScript source to workspace consumers while it remains private and has one repository consumer. Publishing it to a registry or supporting external consumers SHALL require compiled JavaScript and declarations, explicit package exports, a versioning and deprecation policy, asset and CSS delivery contracts, compatibility testing against every supported host, and removal of consumer-specific source aliases or transpilation exceptions.

**Verification:** Build and test the gateway management UI package independently, inspect its public exports and dependency graph, and search both packages for duplicate gateway implementations and generated-SDK imports. Render its collection, detail, and provisioning pages in the standalone console through the public package API. Substitute a test gateway entry port and navigation adapter without React Router or a live BFF, and verify that gateway queries and mutations use the injected services and the host's Query client and localization context. Run every gateway application use case in isolation with a fake control-plane port, deterministic workflow runtime, and recording probe publisher; verify one workflow terminal fact and one dependency terminal fact for success, failure, and cancellation. Contract-test the host SDK adapter for DTO mapping, typed failures, pagination, cancellation, and reconciler-owned request defaults, and test the production probe publisher with two sinks plus a failing sink.

#### Scenario: Standalone Console Hosts the Gateway Management UI Package

- GIVEN the standalone console has constructed its SDK-backed gateway adapter, Query client, localization provider, and route adapter
- WHEN a user opens a gateway route
- THEN the route SHALL pass the gateway identifier and host integrations to the gateway management UI package
- AND the package SHALL render the canonical gateway workflow without creating competing provider or router state

#### Scenario: A Second Product Embeds Gateway Management

- GIVEN another React product uses compatible shared runtime dependencies and implements the documented gateway entry-port and navigation contracts
- WHEN it mounts a gateway management UI page within its own shell
- THEN the page SHALL use that product's navigation, localization, Query client, and gateway entry-port implementation
- AND it SHALL NOT require the HyperShell masthead, route tree, BFF implementation, or build-time source aliases

### Requirement WEB-PKG-02: Reproducible and Defensive Resolution

The repository SHALL pin pnpm and all direct npm dependencies exactly. CI and container builds SHALL install with the committed lockfile in frozen mode. The workspace configuration SHALL enforce:

- one shared workspace lockfile;
- exact saved versions;
- strict peer dependency validation;
- failure on workspace dependency cycles;
- a minimum release age of 20,160 minutes (14 days), in strict mode;
- failure when registry publish time is absent;
- lockfile supply-chain verification rather than treating the lockfile as pre-trusted;
- blocking exotic transitive dependency sources;
- explicit allowlisting of dependency lifecycle/build scripts; and
- no broad package-name or scope exception to the release-age policy.

Any exception SHALL identify an exact package version and satisfy the repository exception policy. Integrity mismatches SHALL fail closed and SHALL NOT be bypassed without an independently verified, reviewable lockfile change.

The build SHALL NOT depend on Node's bundled Corepack. Developer tooling MAY use Corepack or another bootstrap mechanism, but CI and container builds SHALL install or copy the exact declared pnpm artifact through an immutable, reviewable mechanism.

**Verification:** Modify a manifest without updating the lockfile, introduce an undeclared import, request a too-new version, remove publish-time metadata in the policy test, and add an unapproved lifecycle script. Each case fails before build or merge.

### Requirement WEB-PKG-03: Policy Migration Completeness

Adopting pnpm SHALL NOT reduce existing dependency-policy coverage. The migration that creates `pnpm-lock.yaml` SHALL also:

1. teach `scripts/check_dependency_age.py` and its tests to inspect every resolved pnpm package and every workspace importer;
2. enforce exact direct declarations while allowing only explicit `workspace:` references for local packages;
3. remove the SDK `package-lock.json`;
4. convert Makefile and CI commands from per-package npm installs to root pnpm filters or recursive commands;
5. update CI cache keys and dependency paths to use the root lockfile; and
6. document the pinned pnpm bootstrap procedure.

Until these changes land together, the existing npm SDK workflow remains the actual state, and the pnpm migration SHALL be considered incomplete.

**Verification:** Run all dependency-policy unit tests and `make check` against a pnpm-only tree. Confirm that a deliberately too-new transitive dependency in `pnpm-lock.yaml` fails the independent repository check.

### Requirement WEB-SDK-01: Browser-Compatible Generated SDK

The generated TypeScript SDK SHALL publish a modern ESM import and type declaration surface suitable for Vite and Node.js. It SHALL expose an injectable Fetch-compatible transport, a relative base URL, optional credentials/authentication behavior, typed errors, and `AbortSignal` support. Browser-facing modules SHALL NOT read `process.env` or require a bearer token.

The browser SHALL configure the SDK for same-origin `/api` requests with cookie credentials. The BFF SHALL add the server-held bearer token when proxying upstream. Generated source SHALL NOT be hand edited; generator templates or stable handwritten adapters SHALL own required behavior.

**Verification:** Build and import the SDK from both browser and BFF TypeScript configurations. Inspect the browser bundle for `process.env`, embedded API origins, tokens, and CommonJS-only shims. Cancel an in-flight route request and verify upstream cancellation where supported.

### Requirement WEB-AUTH-00: Initial No-Auth Development Mode

The initial UI MAY operate against an API server started with JWT authentication and authorization disabled. This mode SHALL be restricted to an explicit local/development environment, and the API SHALL remain bound to a loopback or otherwise isolated development interface. Shared, review, staging, and production deployments SHALL fail startup or readiness when authentication is disabled.

The browser SHALL retain the production-shaped relative `/api` contract. A Vite development proxy or a development-mode BFF SHALL forward requests to the configured local API origin and SHALL omit the bearer header. Authentication mode SHALL be selected and validated by trusted server or development-tool configuration; a browser-visible `VITE_*` flag SHALL NOT enable or disable authentication.

When routes need identity or capability data before OIDC is implemented, the development server MAY return an unmistakably synthetic development session with full development capabilities. Product code SHALL consume the same session interface used by authenticated deployments and SHALL NOT scatter no-auth conditionals through routes, components, or SDK calls.

No-auth execution SHALL NOT count as evidence for login, logout, session rotation or expiry, CSRF protection, role/capability enforcement, 401/403 recovery, or cross-resource authorization. Those behaviors SHALL use contract-faithful mocks during initial development and SHALL pass against an authenticated environment before production availability.

**Verification:** Start the documented local no-auth workflow and complete a representative gateway route without an authorization header. Attempt to start each non-development environment with authentication disabled and confirm that it fails closed. Search browser code and bundles for an authentication-bypass flag, and inspect test evidence so no-auth journeys are not labeled as authentication coverage.

### Requirement WEB-AUTH-01: OIDC Backend-for-Frontend

Production authentication SHALL use OAuth 2.0 / OpenID Connect authorization code flow with PKCE through the BFF. The BFF SHALL validate issuer, client configuration, redirect URI, state, nonce, PKCE verifier, token response, and identity claims. Access and refresh tokens SHALL remain server-side and SHALL NOT be returned to browser JavaScript.

The concrete identity provider, issuer, client registration, scopes, claims mapping, logout behavior, and session lifetime SHALL be deployment configuration with validated startup requirements. The console SHALL fail startup or health readiness when required production authentication configuration is invalid.

**Verification:** Threat-model login, callback, refresh, logout, fixation, replay, redirect, and provider-failure paths. Search source maps, browser storage, DOM, URLs, logs, and network responses for tokens.

### Requirement WEB-AUTH-02: Session and CSRF Protection

The browser session SHALL use an opaque, rotating identifier in a `Secure`, `HttpOnly`, host-only, `SameSite` cookie with the narrowest practical path and lifetime. A `__Host-` cookie name SHOULD be used where deployment constraints allow it. Session contents and OAuth tokens SHALL be stored server-side in a production-capable shared store when more than one BFF replica may serve traffic.

Every state-changing request SHALL require CSRF protection appropriate to the deployment, including origin validation and a server-validated token or equivalent robust mechanism. CORS SHALL default to same-origin only. Login SHALL rotate the session identifier; logout and terminal authentication failure SHALL revoke server-side session state and clear the cookie.

**Verification:** Attempt cross-site form, Fetch, replay, fixation, stale-session, and multi-replica requests. Inspect cookie attributes and confirm that JavaScript cannot read the session identifier.

### Requirement WEB-AUTH-03: Browser Session Contract

The BFF SHALL expose a minimal browser session resource containing only display identity, locale preferences, expiry/re-authentication state, and server-derived capabilities needed to render the console. It SHALL NOT expose provider tokens or use browser-supplied roles as authorization evidence.

The React application SHALL distinguish unauthenticated, expired, forbidden, unavailable, and insufficient-resource-access states. Session expiry during work SHALL preserve unsent form state where safe and present a clear re-authentication path.

**Verification:** Exercise initial load and mid-task expiry for read, edit, create, and destructive flows. Confirm correct recovery without disclosing protected content from a previous session.

### Requirement WEB-BFF-01: Same-Origin Static and API Service

The BFF SHALL use Node.js LTS and Fastify to provide:

- OIDC login, callback, logout, and session endpoints;
- an authenticated `/api/*` reverse proxy to the HyperShell REST API;
- the immutable built SPA assets and non-cacheable application document;
- liveness and readiness endpoints independent of authenticated product routes;
- security headers, request limits, structured logs, and trace propagation; and
- explicit 404 behavior for unknown API, asset, and server routes.

The proxy SHALL use an allowlisted upstream origin, remove untrusted hop-by-hop and identity headers, set the server-held authorization header, impose timeouts and body limits, preserve safe API status semantics, and cancel upstream work after client disconnect where supported.

The production BFF SHALL forward the gateway management UI package's `/api/hypershell/v1/*` requests to the configured HyperShell API; a Vite development proxy SHALL NOT be the only working API path. The BFF SHALL validate or replace the browser correlation identifier, include it in structured request context, and propagate it upstream. Browser application routes SHALL share one route contract with the production BFF or have an automated parity test; a removed route SHALL NOT remain in a server allowlist and a newly declared route SHALL NOT return a server 404 on direct navigation or refresh.

**Verification:** Test routing precedence, header sanitation, correlation validation and propagation, timeouts, cancellation, large bodies, upstream failures, and SPA fallback behavior. Start the built production BFF against a recording API and complete a representative gateway list request through `/api/hypershell/v1/gateways`. Request `/`, `/gateways/new`, and a representative `/gateways/{id}` directly from the BFF and verify application HTML; verify unknown API, asset, health, and server paths do not receive the SPA document. Confirm that an arbitrary URL cannot turn the BFF into an open proxy.

### Requirement WEB-DATA-01: Server-State Ownership

TanStack Query SHALL own REST response data, asynchronous request state, caching, invalidation, and mutations. Query keys SHALL be factories that include resource kind, resource identifier, and normalized request parameters. Mutation success SHALL update or invalidate only affected keys.

Gateway collection queries SHALL request exactly one authoritative API page and SHALL retain `page`, `size`, and `total` metadata. Search and sort values SHALL be normalized before entering both the application port and query key. The default gateway collection experience SHALL NOT call an all-pages helper or loop until the API total has been loaded.

Gateway placement searches SHALL request exactly one authoritative managed-cluster API page for the normalized search value. The placement selector SHALL NOT call an all-pages helper or exhaust the managed-cluster collection before rendering. When more matching clusters exist than the returned page, the interface SHALL tell the user to refine the search rather than presenting the page as the complete result set.

React Router loaders MAY verify session and route access and prefill the Query client. Loader data and React Context SHALL NOT become competing REST caches. Redux, Zustand, MobX, or another global state store SHALL NOT be added until a recorded design decision demonstrates cross-route client-only state that React, URL state, and TanStack Query cannot manage clearly.

Query and mutation functions SHALL call application use cases through the narrow boundary in `standards/ui/hexagonal-architecture.spec.md`; they SHALL NOT call the generated SDK directly. TanStack Query remains the presentation-side owner of server-state policy and SHALL NOT be hidden behind a generic query port.

**Verification:** Inventory state ownership and query keys. Navigate between gateways and list views with different request parameters and confirm that cached data never crosses resource boundaries. With an API total larger than one page, verify initial render issues one list request, page/search/sort changes issue one request for the normalized state, Back/Forward restores that state, and an inconsistent response is surfaced rather than presented as a complete collection.

### Requirement WEB-DATA-02: URL and Local State

Pagination, filtering, search, sorting, selected gateway, and other shareable view state SHALL be encoded in validated URL path or search parameters. Ephemeral interaction state SHALL remain local to the narrowest component. Context SHALL be limited to stable cross-cutting services such as session, locale, feature flags, and the Query client.

Sensitive values, server response caches, bearer tokens, and unsanitized operational data SHALL NOT be persisted in `localStorage`, `sessionStorage`, URL parameters, or an initial service-worker cache.

**Verification:** Copy and reopen representative list URLs, use browser back/forward navigation, switch users in one browser profile, and inspect browser storage and cache contents.

### Requirement WEB-DATA-03: Retry, Refresh, and Cancellation

Queries SHALL have explicit freshness and retry policies by data class. The client SHALL NOT retry authorization, validation, conflict, or other deterministic 4xx responses. Mutations SHALL NOT retry automatically unless the operation is proven idempotent. Network and eligible 5xx query retries SHALL be bounded, use backoff, and expose final recovery guidance.

Resources in non-terminal lifecycle states MAY use bounded adaptive polling while the document is visible. Polling SHALL pause or slow when hidden, stop at terminal state, recover after reconnect, and avoid refetching unrelated resources. Global polling SHALL NOT be used. SSE or WebSocket dependencies SHALL wait for an authenticated API event contract.

Route changes and component disposal SHALL cancel obsolete requests with `AbortSignal` where supported.

Each request path SHALL have one explicit retry owner. A TanStack Query `AbortSignal` SHALL propagate through the application use case and API port to the generated SDK.

**Verification:** Simulate offline, reconnect, tab hiding, 400, 401, 403, 409, 429, 500, timeout, and slow cancellation. Inspect request count and user-visible state.

### Requirement WEB-DATA-04: Forms and Runtime Validation

Nontrivial forms SHALL use React Hook Form with Zod schemas and PatternFly form components. Zod SHALL validate untrusted browser/runtime shapes where compile-time TypeScript types provide no runtime guarantee. The API remains authoritative for domain rules and authorization.

Validation SHALL occur at submit and appropriate blur/interaction boundaries rather than producing disruptive errors for every keystroke. Forms SHALL preserve user input across recoverable failures, prevent accidental duplicate submission, focus or summarize errors accessibly, and map server field and global errors without exposing sensitive details.

Destructive and conflicting changes SHALL follow the interaction and recovery requirements in `standards/ui/`.

**Verification:** Exercise keyboard-only entry, server/client validation disagreement, duplicate submission, network failure, version conflict, session expiry, cancellation, and successful resubmission.

### Requirement WEB-UI-01: PatternFly-First Presentation

The implementation SHALL follow `standards/ui/patternfly.spec.md`. It SHALL use PatternFly 6 packages from a compatible release set and install optional PatternFly packages only when a feature consumes them. CSS Modules MAY fill product-specific layout or presentation gaps using PatternFly semantic tokens and logical properties.

Tailwind, Bootstrap, Material UI, Chakra UI, another general-purpose component system, Sass, and a CSS-in-JS styling system SHALL NOT be introduced. Charts, terminals, editors, and other specialized widgets MAY be added only with a concrete feature, accessibility review, bundle assessment, and documented PatternFly integration.

**Verification:** Inspect dependencies, imports, rendered class names, tokens, and CSS. Map every custom component to the admission evidence required by the PatternFly standard.

### Requirement WEB-UI-02: Shared Component Evidence

Canonical shared components SHALL have discoverable Storybook stories for applicable default, loading, empty, error, permission, overflow, localization, responsive, and interaction states. Gateway-domain stories SHALL import the canonical gateway management UI package public surface rather than a copied or private console implementation. Storybook SHALL be a development and verification surface, not a separately deployed production dependency unless explicitly required.

React components SHALL be implemented semantically and tested through user-observable behavior. Tests SHALL NOT depend primarily on implementation details, internal component state, or snapshots of large markup trees.

**Verification:** Compare the shared component inventory with stories and interaction tests. Run component accessibility checks and manually verify representative keyboard, screen-reader, zoom, touch, reduced-motion, RTL, and long-content states.

### Requirement WEB-UI-03: Gateway Connection Experience

The gateway landing page SHALL make the shortest useful OpenShell workflow available. Every visible gateway SHALL provide its name, readiness, placement cluster, endpoint, a link to gateway details, and a row actions menu. The row actions menu SHALL provide the gateway-console link, a copyable `openshell gateway add` command, gateway renaming, and gateway deletion. Copying from the menu SHALL produce visible success or failure feedback. The same console, CLI connection, rename, and delete capabilities SHALL remain available on the gateway detail page. The full CLI command SHALL appear as a read-only PatternFly Clipboard Copy value in the resource description list rather than as a page-header action. Rename and delete SHALL be grouped in a PatternFly Actions dropdown at the far right of the header so infrequent management operations do not compete with connection workflows.

Gateway renaming SHALL use the existing `PATCH /gateways/{id}` contract and send only the trimmed `name` field. Both rename entry points SHALL use the same required-field validation, prevent unchanged or duplicate submission, preserve user input with recovery guidance on failure, update gateway detail and breadcrumb cache state, invalidate the collection, and provide visible success feedback.

Gateway deletion SHALL use the existing `DELETE /gateways/{id}` contract. Both delete entry points SHALL present the same explicit confirmation before commitment, prevent duplicate submission while deletion is pending, preserve the confirmation with recovery guidance on failure, and provide an accurate success notification after deletion. Successful deletion from a detail page SHALL return the user to the gateway collection and invalidate both collection and deleted-detail query state.

The generated command SHALL include the gateway name, OIDC issuer, OIDC client ID, OIDC audience, and endpoint. Values SHALL be encoded as safe shell arguments before being presented for copying. Copy controls SHALL have an accessible name and visible success feedback, and the full command SHALL remain available at narrow widths without causing page-level horizontal overflow.

Preview placeholders MAY be used while the API contract is under development, but they SHALL be isolated to explicit Storybook, test, or development fixtures and clearly removable. Production data mappers SHALL preserve missing connection values as unavailable and SHALL NOT replace them with a preview gateway's endpoint, console URL, issuer, client ID, or audience. Production connection values SHALL come from an authorized server response and SHALL NOT be inferred from unrelated deployment fields or embedded as build-time environment constants. Console and CLI controls SHALL be absent or disabled with truthful explanation until all values required by that action are available.

Gateway status presentation SHALL use an explicit bounded mapping. Ready/success states MAY use success green; known failure states SHALL use their defined error or warning semantics; pending or transitional states SHALL use neutral or informational semantics; and unknown, absent, or unrecognized values SHALL use gray. Status text SHALL remain visible so color is never the only carrier.

**Verification:** Exercise the landing and detail experiences with zero, one, many, long-named, unauthorized, and unavailable gateways. Feed an API gateway with every connection field absent and verify that no preview URL or command appears in the production-composed page. Exercise every documented gateway status plus an unrecognized future value and verify semantic labels. Copy and execute representative safe commands, inject shell metacharacters into every source field, verify the console destination, and test keyboard, screen-reader, zoom, and narrow viewport behavior.

### Requirement WEB-UI-04: Gateway Placement Selection

The gateway provisioning form SHALL collect a gateway name and one placement cluster. It SHALL use the PatternFly searchable single-select pattern and populate remote placement options from the managed-cluster API. The control SHALL display each managed cluster by name, distinguish options with available provider and region context, and SHALL NOT offer cluster creation or registration.

The gateway collection and gateway details SHALL display the managed-cluster name for every gateway with a non-empty `cluster_id`. They SHALL resolve that identifier through the managed-cluster API, cache the result by stable cluster identifier, and render explicit loading and unavailable states without exposing the raw identifier as a display fallback. Gateways with an empty `cluster_id` SHALL display `Hub cluster` in both experiences.

`Hub cluster (default)` SHALL be the initially selected placement and SHALL represent the cluster that hosts HyperShell. Selecting this option SHALL send an empty `cluster_id`. Selecting a managed cluster SHALL send that cluster's stable identifier as `cluster_id`. Typing text that does not identify a selected option SHALL NOT silently retain or submit a different placement.

The form SHALL NOT collect a Kubernetes namespace, fleet identifier, release identifier, or database identifier. The host adapter SHALL send `openshell` as the namespace and empty fleet, release, and database identifiers until those values have a user-facing or reconciler-defined contract. Loading, no-match, partial-page, managed-cluster API failure, retry, and cancellation states SHALL be explicit. A managed-cluster API failure SHALL NOT prevent provisioning to the hub cluster, but the interface SHALL explain that remote placements are unavailable and provide a retry action.

Managed-cluster search SHALL execute at the API through the gateway application use case and its application-owned port. Search values SHALL be normalized in the query identity, obsolete requests SHALL be cancelled, and the query SHALL request only the first bounded result page. Cluster list and gateway provisioning requests SHALL retain one correlation context each and publish their required workflow and dependency probes.

**Verification:** Inspect the composed provisioning form, application port, query key, SDK adapter, request payload, and probe recordings. Exercise keyboard and screen-reader selection, search, no-match, more-results, slow load, API failure and retry, cancellation, hub provisioning, and managed-cluster provisioning. Confirm that the form contains no namespace or cluster-creation controls, the default payload uses `cluster_id: ""` and `namespace: "openshell"`, and a selected managed cluster uses its identifier without changing the other reconciler-owned identifiers.

#### Scenario: Provision on the Hub Cluster

- GIVEN the provisioning form has loaded
- WHEN the user keeps `Hub cluster (default)` selected and provisions a gateway
- THEN the request SHALL contain an empty `cluster_id`
- AND the request SHALL contain `namespace: "openshell"`

#### Scenario: Provision on a Managed Cluster

- GIVEN the managed-cluster API returns an existing cluster
- WHEN the user searches for and selects that cluster and provisions a gateway
- THEN the request SHALL contain the selected managed cluster identifier as `cluster_id`
- AND the interface SHALL NOT offer to create or register a cluster

#### Scenario: Managed Clusters Are Unavailable

- GIVEN the managed-cluster API request fails
- WHEN the provisioning form reports the failure
- THEN `Hub cluster (default)` SHALL remain available for selection
- AND the user SHALL be able to retry loading managed clusters without losing valid gateway form input

#### Scenario: Gateway Collection Shows Placement Names

- GIVEN a gateway has a non-empty `cluster_id` for an existing managed cluster
- WHEN the gateway collection renders that row
- THEN the Cluster column SHALL show the managed cluster's name
- AND it SHALL NOT show the managed cluster identifier

#### Scenario: Gateway Details Show Placement Names

- GIVEN a gateway has a non-empty `cluster_id` for an existing managed cluster
- WHEN the gateway details render
- THEN the Cluster value SHALL show the managed cluster's name
- AND it SHALL NOT show the managed cluster identifier

### Requirement WEB-I18N-01: Localization from First Implementation

React Intl / FormatJS SHALL own user-facing application messages from the first implementation. Messages SHALL use complete ICU expressions; concatenated translatable fragments and ad hoc pluralization are prohibited. CI SHALL extract and validate message catalogs and reject missing or malformed messages for supported release locales.

Dates, times, time zones, numbers, units, and plural rules SHALL use `Intl` through the localization layer. Moment.js, Day.js, or another general date library SHALL NOT be added without a requirement that native `Intl` and platform date primitives cannot meet.

Pseudo-localization and an RTL locale SHALL be part of regular verification even before a second production locale ships.

**Verification:** Extract messages, build with pseudo-localized and RTL catalogs, and test expansion, mixed direction, time-zone transitions, plural categories, and missing translations on critical routes.

### Requirement WEB-QUAL-01: Static Analysis

The web workspace SHALL run:

- TypeScript strict checking for browser, server, and tests;
- ESLint flat configuration with type-aware `typescript-eslint` rules;
- React Hooks and React Compiler lint rules;
- JSX accessibility rules;
- TanStack Query rules;
- FormatJS message rules; and
- Prettier verification.

Lint suppression SHALL be narrow and explain the violated rule's inapplicability. React Compiler MAY be piloted only after compatibility testing with React Router, PatternFly, Storybook, and the production build. It SHALL NOT be treated as a substitute for profiling, correct state ownership, or deliberate memoization.

**Verification:** Run all checks from the workspace root and introduce representative type, hook, accessibility, query, and message violations to confirm enforcement.

### Requirement WEB-QUAL-02: Test Layers

The web console SHALL use:

- Vitest for unit and route/component integration tests;
- Testing Library and `user-event` for behavior through accessible user interactions;
- MSW for reusable request/response behavior at the network boundary;
- Storybook with interaction and accessibility checks for shared component states; and
- Playwright for end-to-end journeys in Chromium, Firefox, and WebKit.

Jest or Cypress SHALL NOT be added unless an accepted decision demonstrates a test requirement the selected tools do not meet.

Mocks SHALL preserve the API's production status codes, latency, authorization, validation, pagination, and error envelope. A test SHALL NOT claim end-to-end coverage when the BFF or API boundary is mocked.

At least one production-shaped integration test SHALL start the BFF with a recording upstream and exercise the same-origin API path used by the built browser. Browser route coverage SHALL include direct BFF navigation and refresh for every route shape, not only client-side navigation from `/`.

**Verification:** Inspect the test inventory and run representative suites. Confirm that contract drift in an API fixture produces a failure rather than being hidden by permissive mocks.

### Requirement WEB-QUAL-03: Change and Release Gates

Every pull request affecting the web workspace SHALL pass frozen installation, dependency policy, formatting, lint, type checks, unit/integration tests, Storybook build and checks, a production build, a production-BFF API and direct-route smoke test, and critical Chromium journeys. The production smoke test SHALL fail when the browser's API namespace is not proxied, when a declared browser route returns a server 404, or when production gateway mapping supplies preview connection constants. Main/release verification SHALL add Firefox, WebKit, visual regression for critical layouts, pseudo-locale/RTL coverage, and the broader accessibility matrix required by `standards/ui/verification.spec.md`.

Automated accessibility checks SHALL use axe-compatible rules but SHALL NOT be reported as complete accessibility evidence. Manual keyboard, screen reader, zoom/reflow, touch, reduced-motion, and locale checks remain required at the risk-based cadence in the UI verification standard.

**Verification:** Inspect component-aware CI, stable summary gates, test artifacts, browser versions, exclusions, and release evidence.

### Requirement WEB-DEPLOY-01: Reproducible Container

The web console SHALL build in a multi-stage container from digest-pinned images. The build stage SHALL use the pinned Node.js LTS and pnpm versions with a frozen lockfile. The runtime SHALL contain only the BFF production closure, built static assets, required metadata, and trusted certificates.

The runtime container and Kubernetes workload SHALL run as non-root, disallow privilege escalation, drop all capabilities, use the default seccomp profile, and use a read-only root filesystem with explicitly mounted writable paths only when required. It SHALL expose port 8080, define resource requests/limits, and provide independent liveness and readiness probes.

**Verification:** Rebuild from a clean checkout, inspect image contents and dependency closure, scan the image, start it with the production security context, and exercise probes and graceful shutdown.

### Requirement WEB-DEPLOY-02: Assets and Runtime Configuration

Content-hashed static assets SHALL use long-lived immutable caching and compression. The application HTML and deployment configuration SHALL not use long-lived caching. Source maps SHALL be uploaded privately to the selected error system or retained as protected build artifacts; they SHALL NOT be publicly served by default.

`VITE_*` variables SHALL be treated as public compile-time constants and SHALL never contain credentials, tokens, private origins, or other secrets. Environment-specific server settings and sensitive configuration SHALL be read and validated by the BFF at runtime. Any browser-visible runtime configuration endpoint SHALL expose only an allowlisted, non-sensitive schema.

No service worker or offline mutation queue SHALL ship initially. Either feature requires an explicit cache invalidation, privacy, session-transition, update, conflict, and recovery design.

**Verification:** Inspect built JavaScript, source maps, cache headers, compressed responses, runtime configuration, and browser caches before and after logout and deployment replacement.

### Requirement WEB-SEC-01: Browser Security Headers

The BFF SHALL set a reviewed Content Security Policy, frame-ancestor restriction, MIME sniffing protection, referrer policy, permissions policy, and HSTS where TLS deployment permits it. CSP SHALL begin in report-only mode while third-party and framework behavior is inventoried, then become enforcing before production release. Unsafe script execution and unbounded third-party origins SHALL NOT be accepted as permanent defaults.

User- or API-provided text SHALL render as text by default. Raw HTML rendering requires sanitization, a documented need, and adversarial tests. URLs, redirects, downloads, and external links SHALL use allowlisted schemes and destinations appropriate to their purpose.

**Verification:** Run header and CSP tests, inject representative markup and URL payloads, and review CSP reports. Confirm that framing and cross-origin data access fail as designed.

### Requirement WEB-OBS-01: User and Web Performance Signals

The browser SHALL report Core Web Vitals using `web-vitals` and SHALL measure critical-task completion, failure, abandonment, client errors, and recovery outcomes required by `standards/ui/trust-performance.spec.md`. Metrics SHALL use stable route templates rather than raw identifiers and SHALL exclude query strings, secrets, user-entered values, tokens, and sensitive resource content.

Domain and critical-task facts SHALL enter through the typed, fan-out probe publisher required by `standards/ui/domain-observability.spec.md`. Browser application code SHALL NOT bypass it with raw console, logging, analytics, metrics, or tracing calls.

Performance budgets SHALL cover route JavaScript, CSS, initial data, long tasks, and task-specific latency on representative devices, networks, and data volumes. The Core Web Vitals thresholds and percentile rules in the UI performance standard are release requirements.

**Verification:** Inspect emitted events and dimensions, simulate client errors and task failures, and compare field dashboards and lab diagnostics with declared budgets.

### Requirement WEB-OBS-02: Server Telemetry and Correlation

The BFF SHALL emit structured logs, metrics, and traces with request and trace correlation. It SHALL propagate W3C trace context to the API, record sanitized route templates and upstream outcome, and export telemetry through the repository's supported OpenTelemetry/OTLP path. Raw session identifiers, authorization headers, cookies, tokens, user input, and sensitive API bodies SHALL NOT be logged or attached to telemetry.

BFF domain facts SHALL use the same probe contract and fan-out boundary. Framework-managed technical access instrumentation MAY remain an infrastructure concern, but it SHALL NOT substitute for use-case and domain-outcome probes.

Browser errors and metrics SHOULD be accepted through a same-origin BFF endpoint. Experimental browser OpenTelemetry auto-instrumentation SHALL NOT be a foundational dependency; adoption requires a privacy, stability, bundle, and value assessment.

**Verification:** Follow a request from browser signal through BFF and API telemetry, trigger each failure class, and inspect exported data for correlation and prohibited values.

### Requirement WEB-API-01: UI-Supporting API Contracts

Before a feature relies on them, the HyperShell API SHALL define and test:

- a gateway connection list containing a stable identifier, display name, readiness, gateway endpoint, console URL, OIDC issuer, OIDC client ID, and OIDC audience;
- gateway list, search, pagination, sort, and filter semantics;
- a managed-cluster placement list containing a stable identifier, display name, provider, region, and status;
- gateway provisioning, renaming, and deletion contracts;
- authorization behavior and browser-safe capability/permission metadata;
- stable error envelopes with field errors and a support-safe operation identifier;
- idempotency or duplicate-submission behavior for consequential creates;
- optimistic concurrency through an ETag, version, or equivalent precondition contract; and
- lifecycle phase and terminal-state semantics suitable for bounded refresh.

The UI SHALL NOT infer authorization solely from object visibility, guess whether a write conflicted from timestamps, or invent terminal states not defined by the API. An authenticated event contract is required before replacing polling with SSE or WebSockets.

During the initial single-tenant increment, every gateway returned by the API SHALL be treated as visible to every authenticated user, and the provisioning action SHALL be available in the primary gateway experience. Authorization-filtered gateway visibility and mutation capabilities SHALL be defined before a multi-tenant deployment.

The gateway table SHALL identify an empty `cluster_id` as `Hub cluster` in a sortable Cluster column. The provisioning form SHALL expose the hub as `Hub cluster (default)` and list existing managed clusters as remote placement choices, without offering cluster creation or registration. Hub placement SHALL send an empty `cluster_id`; managed-cluster placement SHALL send the selected cluster identifier. The form SHALL NOT collect `fleet_id`, `release_id`, `database_id`, or `namespace`; it SHALL send the first three identifiers as empty strings and `namespace` as `openshell` because the reconciler owns the gateway-image and SQLite database defaults and the namespace is not currently a user choice. The existing API contract and data model SHALL remain unchanged by this UI increment. Preview OIDC and console values MAY support design work, but production builds SHALL NOT use those placeholders as operational defaults.

**Verification:** Run API contract tests and exercise permissions, the initial globally visible gateway list, managed-cluster search and placement, provisioning, valid, empty, unchanged, conflicting, and failed renames from both entry points, confirmed and cancelled deletion from both entry points, deletion failure and duplicate submission, invalid filters, duplicate creates, stale updates, lifecycle transitions, and error mapping through the BFF and UI. Verify that rename sends only the trimmed name; hub placement sends an empty cluster identifier; managed placement sends the selected cluster identifier; and the provisioning form does not expose fleet, release, database, or namespace fields while the API schema remains unchanged.

## Initial Delivery Sequence

Implementation SHOULD proceed in these dependency-ordered increments:

1. Migrate the SDK to the root pnpm workspace and update repository policy checks.
2. Make the generated SDK ESM- and browser-compatible with an injectable transport.
3. Scaffold React Router SPA mode, PatternFly, localization, test tooling, and one root route.
4. Extract the canonical gateway workflows into the private gateway management UI workspace package and consume them through host adapters from the standalone console.
5. Implement the BFF session and API proxy against the selected OIDC provider.
6. Deliver the authenticated HyperShell gateway list and connection detail journey.
7. Add gateway provisioning after the local-cluster, validation, permission, concurrency, CSRF, and recovery contracts are verified.
8. Establish field telemetry, performance budgets, and the full release matrix before production availability.

## Design Decisions

| Decision                                                         | Rationale                                                                                                                                                                                                                                 |
| ---------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| React Router Framework Mode in SPA configuration                 | Typed routes, route splitting, data/error boundaries, and an upgrade path without requiring SSR                                                                                                                                           |
| Same-origin BFF                                                  | Keeps OAuth tokens out of browser JavaScript and centralizes sessions, CSRF, proxy controls, headers, and runtime configuration                                                                                                           |
| pnpm root workspace                                              | Strict dependency visibility, reliable local SDK linking, one lockfile, efficient installs, and supply-chain controls that align with repository policy                                                                                   |
| TanStack Query for REST state                                    | Purpose-built asynchronous cache, invalidation, cancellation, retry, and refresh behavior without a general global store                                                                                                                  |
| PatternFly 6 only                                                | Matches HyperShell standards and prevents competing component/token systems                                                                                                                                                               |
| Vite, Vitest, Storybook Vite, and Playwright                     | One compatible build/test model with fast component feedback and real multi-browser coverage                                                                                                                                              |
| React Intl from the start                                        | Prevents English-only strings and layout assumptions from becoming an expensive later migration                                                                                                                                           |
| Native `Intl` before a date library                              | Covers display/localization needs without a broad dependency until a missing capability is demonstrated                                                                                                                                   |
| REST/OpenAPI SDK rather than GraphQL or Axios                    | Reuses the existing typed API contract and Fetch transport without adding another protocol or request stack                                                                                                                               |
| Adaptive polling for initial lifecycle refresh                   | The current REST API has no authenticated browser event contract; bounded polling meets the immediate need                                                                                                                                |
| One gateway management experience                                | A single gateway list makes connection and provisioning workflows easy to find without introducing premature audience or navigation boundaries                                                                                            |
| Private reusable gateway management UI workspace package         | Establishes one canonical gateway experience that the standalone console hosts today and another product can embed later without coupling feature code to a shell, router, authentication implementation, or mutable global configuration |
| Local-cluster placement first                                    | The initial form presents local placement and omits fleet and cluster values from its request without changing the API contract; remote placement can be added later                                                                      |
| No general client-state store initially                          | URL state, local React state, Context, and TanStack Query have clear non-overlapping ownership                                                                                                                                            |
| No SSR, RSC, service worker, or offline mutation queue initially | The authenticated console has no current SEO/offline requirement that justifies their operational and security complexity                                                                                                                 |
| TypeScript 6 before TypeScript 7                                 | Uses the stable tool API supported across lint, build, test, and generated-code tooling; upgrade follows ecosystem readiness                                                                                                              |
| React Compiler linting before compiler rollout                   | Finds incompatible patterns while allowing profiling and framework/design-system compatibility to establish value                                                                                                                         |

## Primary Basis

- [React: Build a React app from scratch](https://react.dev/learn/build-a-react-app-from-scratch)
- [React versions](https://react.dev/versions)
- [React Router modes](https://reactrouter.com/start/modes)
- [React Router SPA mode](https://reactrouter.com/how-to/spa)
- [Vite build guidance](https://vite.dev/guide/build)
- [Vite environment variables and modes](https://vite.dev/guide/env-and-mode)
- [PatternFly development guidance](https://www.patternfly.org/get-started/develop/)
- [TanStack Query important defaults](https://tanstack.com/query/latest/docs/framework/react/guides/important-defaults)
- [TanStack Query router prefetching](https://tanstack.com/query/latest/docs/framework/react/guides/prefetching)
- [pnpm workspaces](https://pnpm.io/workspaces)
- [pnpm dependency-resolution security settings](https://pnpm.io/settings/dependency-resolution)
- [pnpm frozen installation](https://pnpm.io/cli/install)
- [Node.js Corepack lifecycle](https://nodejs.org/download/release/latest-v25.x/docs/api/corepack.html)
- [OAuth 2.0 for browser-based applications](https://datatracker.ietf.org/doc/draft-ietf-oauth-browser-based-apps/)
- [OWASP Session Management Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html)
- [OWASP Cross-Site Request Forgery Prevention Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Cross-Site_Request_Forgery_Prevention_Cheat_Sheet.html)
- [React Intl](https://formatjs.github.io/docs/react-intl/)
- [Testing Library guiding principles](https://testing-library.com/docs/guiding-principles/)
- [Mock Service Worker](https://mswjs.io/)
- [Playwright browsers and installation](https://playwright.dev/docs/intro)
- [Playwright accessibility testing](https://playwright.dev/docs/accessibility-testing)
- [Storybook accessibility testing](https://storybook.js.org/docs/writing-tests/accessibility-testing)
- [Core Web Vitals](https://web.dev/articles/vitals)
- [OpenTelemetry JavaScript status](https://opentelemetry.io/docs/languages/js/)
