# Usage Dashboard

**Status:** Draft
**Version:** 0.1.0
**Created:** 2026-10-15
**Applies to:** `packages/operational-dashboard-ui`, `components/web-console` SPA and BFF, control-plane sandbox metrics
**Tracks:** [HYPERSHELL-287](https://redhat.atlassian.net/browse/HYPERSHELL-287), [GitHub issue 544](https://github.com/openshift-online/hypershell/issues/544)

## Purpose

Provide a widgetized usage dashboard at `/dashboard/usage` where any authenticated HyperShell user can see gateway traffic and sandbox utilization for the resources they are authorized to access. A user with the `platform:admin` realm role sees fleet-wide values and a `System` label in every usage widget header. A non-admin sees only authorized gateways and sandboxes, with the same widget components, layout behavior, unavailable states, and dashboard interaction patterns as the operational dashboard.

The dashboard uses the same BFF-to-Prometheus architecture as the other operational dashboards. The browser calls same-origin BFF routes, the BFF applies authorization scope, queries Prometheus or the HyperShell REST API as required, and returns JSON. The browser never queries Prometheus directly.

## Scope

- Add `/dashboard/usage` as a route available to every authenticated user.
- Reuse the operational dashboard's shared widget components, secondary navigation, time-range controls, refresh behavior, and localized unavailable states.
- Show fleet-wide usage statistics to users with `platform:admin`.
- Show non-admin users only data for gateways and sandboxes they are authorized to access.
- Apply non-admin resource scope server-side before aggregation.
- Support 7-day and 30-day time ranges.
- Render six usage metrics:
  - gateway request volume
  - gateway request error rate
  - active gateways
  - provisioned but unused gateways
  - sandbox utilization
  - sandbox lifetime
- Add control-plane Prometheus metrics for total provisioned sandboxes and sandbox lifetime.
- Preserve the approved administrator and non-administrator states in the repository mockup.

## Non-goals

- The Gateway release filter is deferred. It is not part of the initial dashboard or mockup.
- The browser does not query Prometheus directly.
- The dashboard does not attribute usage to individual users.
- The dashboard does not backfill missing daily points with zero.
- The dashboard does not return fleet aggregates to non-admin users for client-side filtering.

## Relationship to the operational dashboard

| Concern         | Operational dashboard                              | Usage dashboard                                |
| --------------- | -------------------------------------------------- | ---------------------------------------------- |
| Route           | `/dashboard`                                       | `/dashboard/usage`                             |
| Access          | Authenticated users with dashboard resource access | All authenticated users                        |
| Admin scope     | Fleet-wide                                         | Fleet-wide with `System` on every usage widget |
| Non-admin scope | Authorized resources                               | Authorized gateways and sandboxes              |
| Metrics port    | `getOperationalMetrics`                            | `getUsageMetrics`                              |
| Time ranges     | 7 or 30 days                                       | 7 or 30 days                                   |
| Refresh         | 15-minute automatic refresh                        | Same                                           |
| Partial failure | Independent metric sources                         | Same                                           |

## Metric sources

| Source ID                | BFF route                                 | Metric IDs                                                                           |
| ------------------------ | ----------------------------------------- | ------------------------------------------------------------------------------------ |
| `usage-gateways`         | `GET /api/metrics/usage/gateways`         | `gateway-request-volume`, `gateway-error-rate`, `active-gateways`, `unused-gateways` |
| `usage-sandboxes`        | `GET /api/metrics/usage/sandboxes`        | `sandbox-utilization`                                                                |
| `usage-sandbox-lifetime` | `GET /api/metrics/usage/sandbox-lifetime` | `sandbox-lifetime`                                                                   |

## Requirements

### Requirement: USAGE-DASH-01 - Package placement and exports

The `@openshift-online/hypershell-operational-dashboard-ui` package SHALL host the usage dashboard presentation layer alongside the operational and reliability dashboards.

The package SHALL export at minimum:

- `UsageDashboardPage`
- `UsageDashboardPageProps`
- `UsageDashboardMetrics`
- `UsageMetricSourceId`
- `UsageMetricId`
- `usageDashboardMetricsQueryKey`
- `usageDashboardRefreshMilliseconds`
- usage dashboard localization messages
- `mockUsageDashboardMetrics`

Presentation code SHALL NOT call `fetch`, the SDK, or BFF routes directly. Usage loading SHALL go through the host-injected application port.

#### Scenario: Host imports the usage page

- GIVEN the web console depends on `@openshift-online/hypershell-operational-dashboard-ui`
- WHEN the host route renders `UsageDashboardPage` inside `DashboardUiProvider`
- THEN the page SHALL mount without importing React Router, the SDK, or Fastify directly

---

### Requirement: USAGE-DASH-02 - Hexagonal application boundary

The usage dashboard SHALL follow the narrow hexagonal UI boundary defined in `standards/ui/hexagonal-architecture.spec.md`.

- The host-injected control plane SHALL expose `getUsageMetrics(context)` as the driven-port method for usage metrics.
- Presentation SHALL call that method through `useGetUsageMetricsData` or an equivalent usage operations hook from the same provider.
- The adapter SHALL pass the `AbortSignal`, selected time range, and administrator flag from the UI context to the control plane.
- Workflow probes SHALL be published for usage load with outcomes `started`, `succeeded`, `failed`, or `cancelled`.
- A partial metric-source failure SHALL publish a dashboard partial-failure probe.

#### Scenario: AbortSignal propagates

- GIVEN a usage metrics request is cancelled
- WHEN `getUsageMetrics` is in flight
- THEN the control plane SHALL receive the same `AbortSignal`
- AND the workflow probe SHALL record the cancellation

---

### Requirement: USAGE-DASH-03 - Authenticated access and resource scope

The `/dashboard/usage` SPA route SHALL be available to every authenticated user. It SHALL use the existing `RequireAuthenticatedDashboard` guard, not `RequireDashboardAdmin`.

When OIDC is enabled, the BFF SHALL require an authenticated session for `/dashboard/usage` and every usage metrics route. Unauthenticated callers SHALL receive the standard reauthentication response.

A user with `platform:admin` SHALL see fleet-wide usage data. Every usage widget header SHALL show the existing `System` label.

A non-admin SHALL see only gateways and sandboxes authorized for the signed-in user. The BFF SHALL:

1. List authorized gateways with the user's access token.
2. Read each gateway's `namespace` from the gateway list response.
3. Derive the authorized namespace set.
4. Constrain every usage PromQL query to that namespace set before aggregation.
5. Return only aggregates calculated from authorized resources.

The browser SHALL NOT receive fleet aggregates and filter them locally.

No-auth development mode SHALL treat the caller as `platform:admin` only when OIDC is intentionally unconfigured. Partial OIDC configuration SHALL fail validation. Configured OIDC startup or runtime failures SHALL NOT fall back to no-auth.

#### Scenario: Authenticated non-admin opens the dashboard

- GIVEN OIDC is enabled and the signed-in user can access two gateways
- WHEN the user opens `/dashboard/usage`
- THEN the SPA SHALL render `UsageDashboardPage`
- AND every metric SHALL be calculated only from those two gateways and their sandboxes
- AND no widget SHALL show the `System` label

#### Scenario: Platform admin opens the dashboard

- GIVEN OIDC is enabled and the signed-in user has `platform:admin`
- WHEN the user opens `/dashboard/usage`
- THEN the dashboard SHALL render fleet-wide values
- AND every usage widget header SHALL show `System`

#### Scenario: Unauthenticated metrics request

- GIVEN OIDC is enabled and no session is present
- WHEN the caller requests a usage metrics route
- THEN the BFF SHALL return the standard reauthentication response

#### Scenario: Partial OIDC configuration

- GIVEN the BFF starts with partial OIDC configuration
- WHEN the configuration is validated
- THEN startup or readiness SHALL fail
- AND the BFF SHALL NOT enable no-auth mode

---

### Requirement: USAGE-DASH-04 - SPA and BFF route surfaces

The web console SHALL add:

- `/dashboard/usage` to `components/web-console/app/route-contract.json`
- a route module that renders `UsageDashboardPage`
- a non-admin-only `Usage` entry in the shared dashboard secondary navigation

The BFF SHALL add same-origin, authenticated routes:

- `GET /api/metrics/usage/gateways?range=7|30`
- `GET /api/metrics/usage/sandboxes?range=7|30`
- `GET /api/metrics/usage/sandbox-lifetime?range=7|30`

Route behavior:

- Invalid or missing `range` defaults to `7`.
- Each route fails independently.
- Platform admins receive fleet-wide aggregates.
- Non-admin requests are constrained server-side before querying or aggregating.
- Missing series or unavailable values omit the corresponding fields.
- Explicit zero is valid only where the source or inventory explicitly supports it.
- Actual data coverage dates are returned so the UI can distinguish partial from full coverage.

#### Scenario: Route defaults to seven days

- GIVEN the caller omits `range`
- WHEN the BFF handles a usage metrics request
- THEN the route SHALL behave as if `range=7`

#### Scenario: Independent route failure

- GIVEN the gateway usage route fails
- WHEN the sandbox usage route succeeds
- THEN the dashboard SHALL render sandbox widgets with data
- AND gateway widgets SHALL render the localized unavailable state

---

### Requirement: USAGE-DASH-05 - Widgetized layout and shared components

The usage dashboard SHALL use the same widgetized dashboard package and layout behavior as the operational dashboard.

The default layout SHALL contain:

| Widget                          | Metric ID                |
| ------------------------------- | ------------------------ |
| Gateway request volume          | `gateway-request-volume` |
| Gateway error rate              | `gateway-error-rate`     |
| Active gateways                 | `active-gateways`        |
| Provisioned but unused gateways | `unused-gateways`        |
| Sandbox utilization             | `sandbox-utilization`    |
| Sandbox lifetime                | `sandbox-lifetime`       |

The page SHALL use:

- the shared dashboard secondary navigation
- the shared 7-day and 30-day time-range selector
- the shared 15-minute automatic refresh interval
- the shared manual refresh behavior
- the shared stale-value preservation behavior on refresh failure
- the shared localized unavailable and partial-data states

The approved visual reference is:

- `specs/web-console/mockups/dashboards/usage-dashboard.tsx`
- `specs/web-console/mockups/dashboards/usage-dashboard.stories.tsx`
- `specs/web-console/mockups/dashboards/usage-dashboard.parity.stories.tsx`

Because the implementation runtime cannot create or compare screenshots, the production page SHALL follow the repository UI build gate's complete-without-visual-review path. All non-parity checks SHALL still run.

#### Scenario: Manual refresh preserves stale values

- GIVEN a successful usage load has completed
- WHEN a later refresh fails for one source
- THEN the dashboard SHALL keep the last successful value for that source
- AND it SHALL show the shared partial-failure warning

#### Scenario: Partial 30-day data

- GIVEN the selected range is 30 days and the source has data for only 18 days
- WHEN the dashboard renders
- THEN available daily points SHALL render
- AND missing daily points SHALL remain `null`
- AND the page SHALL show a localized partial-data notice

---

### Requirement: USAGE-DASH-06 - Gateway request metrics

Gateway request metrics SHALL use the gateway-native Prometheus series:

- `openshell_server_grpc_requests_total{method,code}`
- `openshell_server_http_requests_total{path,status}`

The implementation SHALL reuse the existing gateway-native series where they are already available. It SHALL add or change scrape wiring only if an existing collector path cannot make those series available. It SHALL NOT assume that a new `ServiceMonitor` or `PodMonitor` is required.

The deployment SHALL configure `PROMETHEUS_URL` to an application metrics endpoint that already exposes the gateway-native series for every managed cluster in the fleet. The dashboard does not own Gateway telemetry collection and does not add a `ServiceMonitor` or `PodMonitor`.

The BFF SHALL NOT substitute API-server `api_inbound_request_duration_*`; those series measure the HyperShell API server, not gateway workload traffic.

Calculations:

- Request volume: `sum(increase(request_counter[<range>]))`, excluding internal `Peer*` gRPC methods.
- Daily volume: `sum(increase(request_counter[1d]))` evaluated at UTC daily boundaries.
- Errors: gRPC requests where `code != "OK"` and HTTP requests where `status =~ "5.."`.
- Error rate: `errors / total requests * 100`; unavailable when total requests is zero.
- An absent error series with qualifying traffic represents zero observed errors. A failed error-series query makes the error rate unavailable.

#### Scenario: Peer traffic is excluded

- GIVEN a gateway namespace has only internal `Peer*` gRPC requests in the selected range
- WHEN request volume is calculated
- THEN those requests SHALL NOT count toward the total

#### Scenario: Zero requests

- GIVEN a supported series reports zero total requests
- WHEN the error-rate widget renders
- THEN the widget SHALL show the localized unavailable state for error rate
- AND it SHALL NOT display a zero-percent error rate

---

### Requirement: USAGE-DASH-07 - Active and unused gateways

An active gateway is an authorized current gateway namespace with at least one non-peer gateway request in the covered range.

An unused gateway is an authorized current gateway that is not active in the covered range.

The BFF SHALL:

1. Resolve authorized current gateways.
2. Attribute request activity by the gateway's unique `openshell-<id>` namespace.
3. Intersect active namespaces with the current authorized gateway inventory.
4. Count active gateways from that intersection.
5. Count unused gateways as authorized current gateways minus active gateways.

The BFF SHALL resolve the current authorized Gateway inventory from the Gateway API for both platform administrators and non-administrators. Administrators receive the fleet-wide inventory and omit the namespace selector; non-administrators receive only their authorized inventory.

Deleted gateways SHALL NOT inflate active or unused counts.

#### Scenario: Deleted namespace has historical traffic

- GIVEN a gateway was deleted after generating traffic
- WHEN active and unused gateways are calculated
- THEN that gateway SHALL NOT appear in either count

---

### Requirement: USAGE-DASH-08 - Sandbox utilization and lifetime

The control plane SHALL export:

- OTel instrument `gateway.sandbox.total`, exported to Prometheus as `gateway_sandbox_total`
- OTel instrument `gateway.sandbox.lifetime`, exported to Prometheus as `gateway_sandbox_lifetime`

Both metrics SHALL use the OTel scope attributes `namespace` and `hypershell.cluster_id`. Prometheus SHALL export those attributes as the labels `namespace` and `hypershell_cluster_id`.

The OTel collector Prometheus scrape SHALL honor exported labels so `namespace` remains the Gateway namespace rather than being renamed to `exported_namespace` by the scrape-target namespace.

The API-server active-sandbox gauge SHALL retain `managed_cluster` and SHALL additionally expose the Gateway namespace as `gateway_namespace`. Authorized non-administrator sandbox queries SHALL filter by `gateway_namespace`, never by the API-server scrape-target namespace.

`gateway.sandbox.total` SHALL count provisioned Sandbox custom resources, including sandboxes that are not currently active.

Sandbox utilization SHALL be:

`active sandboxes / total provisioned sandboxes`

`gateway.sandbox.lifetime` SHALL be a histogram recorded when a sandbox leaves the active set. The lifetime start SHALL use the Sandbox CR creation timestamp, with pod creation timestamp only as a fallback. The implementation SHALL NOT use `spec.lifecycle.shutdownTime` because it is optional and may represent a future expiry rather than an observed termination.

Lifetime bucket upper bounds, in seconds:

`60`, `300`, `900`, `1800`, `3600`, `7200`, `14400`, `28800`, `43200`, `86400`, `172800`, `259200`, `604800`, `1209600`, `2592000`.

Lifetime calculations:

- mean: histogram sum / histogram count
- P50: `histogram_quantile(0.50, ...)`
- P95: `histogram_quantile(0.95, ...)`

#### Scenario: Sandbox CR timestamp is unavailable

- GIVEN a sandbox leaves the active set and the Sandbox CR creation timestamp is unavailable
- WHEN lifetime is recorded
- THEN the control plane SHALL use the pod creation timestamp

#### Scenario: Total sandbox count is missing

- GIVEN the total-sandbox series is absent
- WHEN the dashboard renders
- THEN the sandbox utilization widget SHALL show the localized unavailable state
- AND it SHALL NOT substitute zero

---

### Requirement: USAGE-DASH-09 - Data source documentation

`packages/operational-dashboard-ui/DATA_SOURCES.md` SHALL document every usage metric with:

- source ID
- BFF route
- metric ID
- Prometheus type
- calculation
- inclusion and exclusion rules
- authorization scope
- refresh cadence
- known data gaps
- zero versus missing semantics

#### Scenario: Data source documentation is complete

- GIVEN the usage dashboard is implemented
- WHEN a reviewer reads `DATA_SOURCES.md`
- THEN every usage widget's data source and calculation SHALL be discoverable without reading implementation code

---

### Requirement: USAGE-DASH-10 - Localization and accessibility

All usage dashboard text SHALL be localized through the existing dashboard message catalog.

The page SHALL provide:

- localized page title and description
- localized widget titles and metric labels
- localized `System` badge accessible name
- localized unavailable and partial-data states
- localized manual and automatic refresh status
- accessible time-range controls
- accessible chart and summary alternatives

#### Scenario: Non-English locale

- GIVEN the browser locale is not English
- WHEN the usage dashboard renders
- THEN all user-facing text SHALL come from the localized message catalog

---

### Requirement: USAGE-DASH-11 - Tests and verification

The implementation SHALL include tests for:

- authenticated SPA route access
- BFF access control
- platform-admin fleet scope
- `System` on every usage widget for platform admins
- non-admin server-side resource scoping
- no-auth mode only when OIDC is intentionally unconfigured
- Prometheus query mapping
- adapter response mapping
- partial metric-source failures
- missing versus explicit-zero values
- partial 30-day data and `null` daily gaps
- calculation correctness
- Storybook/mockup parity

Mockup verification SHALL run `check_mockup_patternfly.py` for every changed mockup TSX or CSS file.

The production page SHALL pass the repository web-console UI build gate using the complete-without-visual-review path because this model cannot create or compare screenshots.

#### Scenario: Non-admin scope test

- GIVEN a non-admin is authorized for one gateway namespace
- WHEN the usage adapter requests metrics
- THEN the BFF query SHALL include only that namespace
- AND the returned metrics SHALL contain no values from unauthorized gateways

## References

- [HYPERSHELL-287](https://redhat.atlassian.net/browse/HYPERSHELL-287)
- [GitHub issue 544](https://github.com/openshift-online/hypershell/issues/544)
- `specs/web-console/mockups/dashboards/usage-dashboard.tsx`
- `specs/web-console/mockups/dashboards/usage-dashboard.stories.tsx`
- `specs/web-console/mockups/dashboards/usage-dashboard.parity.stories.tsx`
- `packages/operational-dashboard-ui/DATA_SOURCES.md`
