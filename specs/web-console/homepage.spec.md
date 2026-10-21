# Homepage

**Status:** Active
**Version:** 1.0.0
**Applies to:** `components/web-console` SPA and BFF, `packages/gateway-management-ui`, `packages/operational-dashboard-ui`, `components/api-server` placement contract
**Tracks:** [GitHub issue 662](https://github.com/openshift-online/hypershell/issues/662), [HYPERSHELL-358](https://redhat.atlassian.net/browse/HYPERSHELL-358)

## Purpose

Define the authenticated HyperShell homepage, its route relationship to the gateway list, the authorized data sources for every dynamic value, and the route and external-link registries used by the page. The homepage is the normal entry point for the web console. It gives users a concise gateway-health view, entry to gateway creation and dashboards, configured support contact, and curated external learning links. The production surface follows the approved design at `specs/web-console/mockups/homepage/homepage.tsx`, reviewed through the `Mockups/Homepage` and `Parity/Homepage` Storybook stories.

## Requirements

### Requirement: WEB-HOME-01 -- Routes

The web console SHALL render `HomepagePage` at `/`. The existing gateway-list experience SHALL move to `/gateways` without changing its filtering, sorting, pagination, query-string state, deleted-gateway and renamed-gateway handling, or authorization behavior. The existing `/gateways/new`, `/gateways/:gatewayId`, `/dashboard`, dashboard subroutes, `/metrics`, and `/login` routes SHALL remain available.

The dashboard-dedicated hostname exception SHALL remain: when the browser hostname selects the dashboard host, `/` SHALL render the existing operational dashboard instead of the homepage.

The BFF SHALL serve the SPA shell for direct navigation and refresh to `/`, `/gateways`, and every route declared in the browser route contract.

#### Scenario: WEB-HOME-01.01 -- Root renders the homepage

- GIVEN an authenticated user navigates to `/`
- WHEN the SPA renders
- THEN the homepage SHALL be visible
- AND the gateway list SHALL NOT be rendered at `/`

#### Scenario: WEB-HOME-01.02 -- Gateway list remains available

- GIVEN an authenticated user navigates directly to `/gateways?search=team&page=2`
- WHEN the SPA renders
- THEN the gateway list SHALL restore the query-string state
- AND filtering, sorting, pagination, and authorization SHALL behave as before

#### Scenario: WEB-HOME-01.03 -- Dashboard host keeps its root behavior

- GIVEN a dashboard administrator opens `/` on the dashboard-dedicated hostname
- WHEN the SPA renders
- THEN the operational dashboard SHALL be rendered
- AND the homepage SHALL NOT be rendered

### Requirement: WEB-HOME-02 -- Internal Route Registry

The web console SHALL expose one internal route registry for the homepage, gateway list, gateway create, gateway detail, dashboard, dashboard subroutes, metrics, login, and other routes referenced by affected production components. React Router definitions, BFF SPA-route handling, and all component-level links and navigations SHALL use constants or helper functions from that registry. Gateway detail navigation SHALL use a helper such as `gatewayDetailPath(gatewayId)`.

Shared UI packages SHALL NOT import web-console route modules. The web-console composition root SHALL pass concrete route values through the existing shared navigation-provider contracts.

A static or unit check SHALL reject new hard-coded production path literals for the affected routes outside the registry and generated route contract.

#### Scenario: WEB-HOME-02.01 -- Links use route constants

- GIVEN the homepage, application shell, gateway list, gateway creation, gateway detail, and dashboard components are compiled
- WHEN the route-registry check runs
- THEN every affected internal link and navigation SHALL resolve to a registry constant or helper

#### Scenario: WEB-HOME-02.02 -- Shared packages remain route agnostic

- GIVEN `gateway-management-ui` and `operational-dashboard-ui` are compiled
- WHEN module boundaries are checked
- THEN neither package SHALL import a web-console route module
- AND both packages SHALL receive href values through their provider contracts

### Requirement: WEB-HOME-03 -- Homepage Layout

The homepage SHALL follow the approved `Parity/Homepage` surface. On extra-large viewports it SHALL use a two-column grid whose first column is twice as wide as the second. Column one SHALL contain the primary CTA card, Gateway health, and the configured support-channel card. Column two SHALL contain the role-dependent dashboard or platform-overview card and Learn and explore. Learn and explore SHALL list HyperShell repository, OpenShell docs, and OpenShell CLI links in one column.

The production page SHALL preserve the approved mockup's icon choices, descriptions, card hierarchy, spacing, responsive stacking, and PatternFly dark-mode chart text treatment. It SHALL NOT include Platform pulse.

#### Scenario: WEB-HOME-03.01 -- Cards follow the approved hierarchy

- GIVEN the homepage renders with all data available
- WHEN the page is inspected
- THEN the CTA, Gateway health, support channel, role-dependent card, and Learn and explore cards SHALL appear in the approved order and proportions

#### Scenario: WEB-HOME-03.02 -- Narrow viewports stack cards

- GIVEN the homepage renders at a narrow viewport
- WHEN the grid wraps
- THEN the cards SHALL stack without clipping controls or losing card boundaries

### Requirement: WEB-HOME-04 -- Role-Dependent Card

Users with the `platform:admin` role SHALL see Platform overview. Users without that role SHALL see Your dashboard. The card SHALL remain neutral while session loading is in progress so it does not flash the wrong role's card. The implementation SHALL reuse the existing session-role helper and role constant.

The platform-overview query SHALL be enabled only after the session is loaded and the user has `platform:admin`. It SHALL NOT be issued for a non-admin user.

#### Scenario: WEB-HOME-04.01 -- Administrator sees Platform overview

- GIVEN a loaded session with `platform:admin`
- WHEN the homepage renders
- THEN Platform overview SHALL be visible
- AND Your dashboard SHALL NOT be visible

#### Scenario: WEB-HOME-04.02 -- Non-admin sees Your dashboard

- GIVEN a loaded session without `platform:admin`
- WHEN the homepage renders
- THEN Your dashboard SHALL be visible
- AND Platform overview SHALL NOT be visible
- AND no platform-overview HTTP request SHALL be issued

#### Scenario: WEB-HOME-04.03 -- Session loading does not flash a role card

- GIVEN the session query is pending
- WHEN the homepage renders
- THEN the role-dependent card SHALL show a stable loading state
- AND neither role-specific card title SHALL be asserted as final content

### Requirement: WEB-HOME-05 -- Homepage Data Boundary

The browser SHALL obtain every dynamic homepage value from authorized same-origin BFF queries through application-owned adapters. Presentation components SHALL NOT call the SDK, browser `fetch`, Prometheus, or the API server directly. The browser SHALL NOT synthesize gateway totals, status counts, sandbox counts, cluster counts, cluster names, or attention lists from hard-coded production values.

The BFF SHALL expose:

- `GET /api/homepage/summary` for authenticated users, returning gateway health and attention entries.
- `GET /api/homepage/platform-overview` only for users with `platform:admin`, returning gateway status counts, active-sandbox status counts, and cluster availability.

Both routes SHALL return aggregate JSON, preserve the signed-in user's authorized resource scope, and expose independent failure facts when a source fails. The platform-overview route SHALL reject a non-admin with HTTP `403`. The implementation MAY derive the aggregates from the existing gateway operations, operational-dashboard, metrics, or platform-inventory adapters when those adapters already provide the required facts.

Queries SHALL use TanStack Query keys that invalidate with the gateway and dashboard data they represent. The page SHALL keep retry and refetch behavior consistent with existing console queries.

#### Scenario: WEB-HOME-05.01 -- Values come from API responses

- GIVEN the homepage test harness supplies distinct gateway, sandbox, and cluster API responses
- WHEN the homepage renders
- THEN every visible dynamic value SHALL match the supplied response
- AND no mockup fixture value SHALL be used as a production fallback

#### Scenario: WEB-HOME-05.02 -- Non-admin cannot request platform overview

- GIVEN a loaded non-admin session
- WHEN the homepage mounts
- THEN the SPA SHALL NOT request `GET /api/homepage/platform-overview`
- AND a direct request to that route SHALL return HTTP `403`

### Requirement: WEB-HOME-06 -- Gateway Health

The Gateway health card SHALL show a status alert, a healthy-count progress bar, and the gateways that currently need attention. The BFF SHALL derive total, healthy, and attention counts and attention entries from authorized gateway data. Each attention entry SHALL identify the gateway, provide a localized reason and status, and link to that gateway's detail route.

When no gateway needs attention, the card SHALL show an all-healthy state and omit the attention list. When the summary source fails, only that card SHALL show a localized unavailable state and recovery guidance.

#### Scenario: WEB-HOME-06.01 -- Attention entries are data driven

- GIVEN the summary response contains two gateways that need attention
- WHEN the homepage renders
- THEN both entries SHALL show the API-supplied names and reasons
- AND each entry SHALL navigate to the correct gateway detail route

#### Scenario: WEB-HOME-06.02 -- All gateways are healthy

- GIVEN the summary response contains five gateways and no attention entries
- WHEN the homepage renders
- THEN the card SHALL report that all five gateways are healthy
- AND no attention entry SHALL be rendered

#### Scenario: WEB-HOME-06.03 -- Gateway source failure is isolated

- GIVEN `GET /api/homepage/summary` fails
- WHEN the homepage renders
- THEN Gateway health SHALL show a localized unavailable state
- AND the CTA, Learn and explore, and role-dependent card behavior SHALL remain usable

### Requirement: WEB-HOME-07 -- Platform Overview

Platform overview SHALL show a gateway status donut and an active-sandbox status donut. Each donut SHALL show the total in its center, status labels and counts in its legend, and colors aligned with the approved status treatment. The BFF SHALL derive gateway and sandbox counts from authorized aggregate sources; the browser SHALL NOT combine paginated list pages to synthesize platform totals.

When one platform-overview source fails, the affected chart or cluster section SHALL show a localized unavailable state while unaffected sections remain visible.

#### Scenario: WEB-HOME-07.01 -- Donut totals and legends match the response

- GIVEN the platform-overview response contains 42 gateways and 128 active sandboxes with distinct status counts
- WHEN an administrator opens the homepage
- THEN the gateway donut SHALL show 42 in its center
- AND the sandbox donut SHALL show 128 in its center
- AND every legend label and count SHALL match the response

#### Scenario: WEB-HOME-07.02 -- Partial platform data does not blank the card

- GIVEN the sandbox status source fails while gateway and cluster sources succeed
- WHEN an administrator opens the homepage
- THEN the sandbox chart SHALL show an unavailable state
- AND the gateway and cluster sections SHALL render their API-supplied values

### Requirement: WEB-HOME-08 -- Cluster Availability

Platform overview SHALL show cluster availability as a healthy-cluster count, progress bar, and details for unhealthy clusters. The BFF SHALL derive total and healthy cluster counts and unhealthy-cluster details from authorized platform-inventory data. Cluster names, reasons, and status SHALL come from the response.

#### Scenario: WEB-HOME-08.01 -- Unhealthy cluster details are data driven

- GIVEN the platform-overview response reports five of six clusters healthy and one unreachable cluster
- WHEN an administrator opens the homepage
- THEN the progress SHALL show five of six
- AND the unhealthy cluster SHALL show the API-supplied name and reason

### Requirement: WEB-HOME-09 -- Support Channel

The support-channel card SHALL render static localized channel content. The production Slack URL SHALL come from the shared external-link registry. The card SHALL NOT depend on BFF configuration or the homepage summary response.

#### Scenario: WEB-HOME-09.01 -- Support channel is always available

- GIVEN the homepage renders with or without gateway-summary data
- WHEN the homepage renders
- THEN the support card SHALL show the localized HyperShell support copy
- AND the action SHALL open the shared Slack channel URL from the external-link registry


### Requirement: WEB-HOME-10 -- Loading and Failure States

Each data-backed card SHALL render a stable loading state that avoids layout shift where practical. One failed source SHALL NOT blank the homepage. Every unavailable state SHALL be localized and explain that the card data cannot be loaded.

#### Scenario: WEB-HOME-10.01 -- Independent source failures

- GIVEN the summary route succeeds and the platform-overview route fails
- WHEN an administrator opens the homepage
- THEN the gateway and support cards SHALL render normally
- AND Platform overview SHALL show a localized unavailable state

### Requirement: WEB-HOME-11 -- Homepage Actions

Create a gateway SHALL navigate to `/gateways/new`. View gateways and View my gateways SHALL navigate to `/gateways`. Dashboard links SHALL navigate to `/dashboard`. Every action SHALL use the internal route registry and preserve normal browser behavior for modifier keys and middle-click navigation.

#### Scenario: WEB-HOME-11.01 -- Primary actions navigate correctly

- GIVEN the homepage is rendered
- WHEN the user activates Create a gateway, View gateways, or a dashboard link
- THEN the SPA SHALL navigate to the corresponding registry-backed route

### Requirement: WEB-HOME-12 -- Quick Create Gateway

Quick create SHALL create a real Gateway through the existing gateway operations application port. It SHALL use IBM Cloud public placement and SHALL exclude Kata capability. The submitted placement intent SHALL require both `network/public` and `provider/ibm` and SHALL exclude `runtime/kata`.

On success, the workflow SHALL invalidate gateway queries and navigate to the created gateway detail route. On failure, the modal SHALL remain open, show a useful localized error, and allow retry or cancel. A name-conflict response SHALL generate a new unique candidate and retry a bounded number of times.

#### Scenario: WEB-HOME-12.01 -- Quick create submits the intended placement

- GIVEN the user activates Create gateway in Quick create
- WHEN the provision request is inspected
- THEN the placement intent SHALL require `network/public` and `provider/ibm`
- AND the intent SHALL exclude `runtime/kata`
- AND the request SHALL be sent through the gateway operations port

#### Scenario: WEB-HOME-12.02 -- Success navigates to the created gateway

- GIVEN Quick create receives a created gateway with id `gateway-17`
- WHEN the mutation succeeds
- THEN gateway queries SHALL be invalidated
- AND the SPA SHALL navigate to the gateway detail route for `gateway-17`

#### Scenario: WEB-HOME-12.03 -- Failure keeps the modal usable

- GIVEN the provision request fails
- WHEN the mutation settles
- THEN the modal SHALL remain open
- AND a useful localized error SHALL be shown
- AND retry and cancel SHALL remain available

### Requirement: WEB-HOME-13 -- Quick Create Name Generation

Quick create SHALL generate a two-word, hyphenated, human-readable base name and add enough entropy to make accidental collision unlikely. Generation SHALL use a cryptographically secure random source and SHALL accept a deterministic generator for tests.

Before submission, the workflow SHALL reject a candidate that does not satisfy the existing gateway-name validation. It SHALL avoid a name already present in efficiently available known-gateway data and SHALL retry a bounded number of times for a conflict response. If uniqueness cannot be achieved, it SHALL stop and show a useful error without silently using a duplicate.

#### Scenario: WEB-HOME-13.01 -- Generated name has required shape

- GIVEN the deterministic test generator returns two words
- WHEN a candidate name is created
- THEN the name SHALL be lowercase, hyphenated, unique, and valid under the gateway-name contract

#### Scenario: WEB-HOME-13.02 -- Known collision is avoided

- GIVEN known gateway data already contains the generated candidate
- WHEN the workflow prepares a submission
- THEN a different valid candidate SHALL be generated

#### Scenario: WEB-HOME-13.03 -- Conflict retry is bounded

- GIVEN the API returns a name conflict for every generated candidate
- WHEN the retry limit is reached
- THEN no further request SHALL be sent
- AND a useful localized error SHALL be shown

### Requirement: WEB-HOME-14 -- External Link Registry

The web console SHALL keep production external URLs in one named registry. The HyperShell repository, OpenShell docs, OpenShell CLI installation, and every affected production external link SHALL be referenced through that registry. Test fixtures, generated code, and dependency code are excluded.

A static or unit check SHALL reject new literal production `https://` URLs outside the registry and allowed exclusion paths. External links SHALL open in a new tab only where established behavior requires it.

#### Scenario: WEB-HOME-14.01 -- Production external links use the registry

- GIVEN production source is scanned
- WHEN the external-link registry check runs
- THEN every affected external link SHALL resolve to a named registry entry

### Requirement: WEB-HOME-15 -- External Link Checker

The repository SHALL provide a command that verifies every URL in the external-link registry. The checker SHALL read the registry, follow redirects, try `HEAD` first and fall back to `GET`, use a realistic browser-like `User-Agent` when needed, apply a timeout, and use bounded concurrency. It SHALL fail with a non-zero exit code for any 4xx response, network error, or final non-2xx or non-3xx status. It SHALL print one concise result per URL and a final summary.

The command SHALL be documented in the existing command documentation and covered by tests with mocked HTTP requests and registry-integrity checks.

#### Scenario: WEB-HOME-15.01 -- Checker rejects a 404 result

- GIVEN the registry contains a URL whose final response is HTTP 404
- WHEN the checker runs
- THEN it SHALL report that URL as failed
- AND the command SHALL exit non-zero

#### Scenario: WEB-HOME-15.02 -- HEAD rejection falls back to GET

- GIVEN a registry URL rejects `HEAD` with HTTP 405 but returns HTTP 200 for `GET`
- WHEN the checker runs
- THEN the URL SHALL pass after one `GET` fallback

### Requirement: WEB-HOME-16 -- Verification

The implementation SHALL include focused tests for route changes, route-registry usage, role-dependent rendering, query gating, API-backed values, isolated unavailable states, Quick create, name generation, external-link registry integrity, and link-checker status handling. The web-console format, architecture, lint, typecheck, unit-test, build, i18n, and Storybook checks SHALL pass.

The homepage is a production page with an approved mockup. The repository UI completion gate SHALL be executed. When visual comparison is unavailable in the current run, the gate SHALL complete without visual review, skip screenshot capture and comparison, record that capability reason, and continue all non-parity checks.

#### Scenario: WEB-HOME-16.01 -- Required checks pass

- GIVEN the implementation is complete
- WHEN the web-console check command runs
- THEN format, architecture, lint, typecheck, unit-test, build, i18n, and Storybook checks SHALL pass

#### Scenario: WEB-HOME-16.02 -- Visual comparison capability is recorded

- GIVEN the current runtime is prohibited from taking or comparing snapshots
- WHEN the UI completion gate runs
- THEN screenshot capture and comparison SHALL be skipped
- AND the handoff SHALL record that visual review was unavailable
- AND all non-parity checks SHALL still run

## Non-Goals

- This specification does not move the dashboard-dedicated hostname root to the homepage.
- This specification does not add Platform pulse or dashboard widgets to the homepage.
- This specification does not replace the full gateway provisioning page; Quick create is an additional shortcut.
- This specification does not make external-link availability a runtime page dependency.
