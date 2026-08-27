# Gateway Metrics Dashboard

**Status:** Active
**Applies to:** `components/api-server` Prometheus metrics endpoint, `components/web-console` BFF and SPA, `packages/gateway-management-ui` shared component library, `deploy/base` and `deploy/kind` Kubernetes manifests

## Purpose

Expose the live count of Gateway instances by phase  -  Running, Provisioning, Degraded, and Failed  -  as a built-in dashboard in the HyperShell web console, so operators can assess fleet health at a glance without leaving the product UI or querying Prometheus directly. The metric is collected by a custom Prometheus Collector in the API server that queries the database on each scrape; it is scraped by a Prometheus instance deployed alongside the platform; and it is surfaced in the web console through a BFF proxy route that shields the browser from direct Prometheus access.

This specification covers the full vertical slice: the API server metric, the Prometheus deployment and scrape configuration, the BFF metrics route, the shared dashboard component, the SPA route and navigation, and the Kubernetes manifests required to deploy and operate the pipeline on any cluster.

## Requirements

### Requirement: DASH-01 -- Gateway Phase Metric

The API server SHALL expose a custom Prometheus Collector named `hypershell_gateways_total` with a `phase` label that reports the current count of Gateway instances in each of the four known phases: `Running`, `Provisioning`, `Degraded`, and `Failed`.

The Collector SHALL query the database once per scrape via `CountByPhase`  -  a single `SELECT phase, count(*) FROM gateways GROUP BY phase`  -  and emit one `GaugeValue` sample per phase. All four phases SHALL be emitted on every scrape, even when a phase count is zero, so Prometheus never observes a gap in the series.

When the database query fails, the Collector SHALL emit a `prometheus.NewInvalidMetric` so the scrape registers as failed and the error is surfaced to Prometheus rather than silently dropped.

The Collector SHALL be registered exactly once using `sync.Once`; subsequent calls to `RegisterGatewayMetrics` SHALL be no-ops.

#### Scenario: All phases emitted on a healthy scrape

- GIVEN the API server is running with a reachable database
- WHEN Prometheus scrapes `GET :4433/metrics`
- THEN the response SHALL contain `hypershell_gateways_total{phase="Running"}`, `hypershell_gateways_total{phase="Provisioning"}`, `hypershell_gateways_total{phase="Degraded"}`, and `hypershell_gateways_total{phase="Failed"}`
- AND each value SHALL equal the current count of Gateways in that phase

#### Scenario: Zero-count phases still appear

- GIVEN no Gateways are in the `Failed` phase
- WHEN Prometheus scrapes the metrics endpoint
- THEN `hypershell_gateways_total{phase="Failed"}` SHALL be present with value `0`

#### Scenario: Database error surfaces as a scrape failure

- GIVEN the database is unreachable at scrape time
- WHEN Prometheus scrapes the metrics endpoint
- THEN the scrape SHALL record a failure rather than silently returning stale or absent data

---

### Requirement: DASH-02 -- Metrics Server Reachability

The API server metrics HTTP server SHALL bind to `0.0.0.0:4433` so it is reachable from Prometheus running in a separate pod.

The default framework bind address (`localhost:4433`) restricts the metrics server to the pod's loopback interface and makes it unreachable from any other pod. The `--metrics-server-bindaddress=0.0.0.0:4433` flag SHALL be set in the API server Deployment manifest to override this default.

The `/metrics` path SHALL remain unauthenticated, consistent with the existing `auth-bypass-paths` configuration.

#### Scenario: Prometheus can reach the metrics endpoint

- GIVEN the API server Deployment includes `--metrics-server-bindaddress=0.0.0.0:4433`
- WHEN Prometheus sends `GET http://hypershell-api-server:4433/metrics` from another pod
- THEN the API server SHALL respond with HTTP 200 and the Prometheus text format

#### Scenario: Metrics endpoint requires no authentication

- GIVEN a request arrives at `/metrics` without an `Authorization` header
- WHEN the API server handles the request
- THEN it SHALL respond with HTTP 200 and SHALL NOT redirect to login or return 401

---

### Requirement: DASH-03 -- Prometheus Operator and Instance

Every cluster to which HyperShell is deployed SHALL include a Prometheus Operator and a Prometheus instance capable of scraping the `hypershell_gateways_total` metric without requiring cluster-specific manual configuration.

The Prometheus Operator SHALL be vendored as `deploy/kind/infrastructure/prometheus-operator-bundle.yaml` (pinned to a specific release) and applied as a cluster-level infrastructure dependency before application manifests, in the same layer as cert-manager and CNPG.

A `Prometheus` CR, a `ServiceAccount`, a `ClusterRole`, and a `ClusterRoleBinding` SHALL be declared in `deploy/base/prometheus/` and applied as application-level resources. The `Prometheus` CR SHALL:

- Use the `prometheus` `ServiceAccount`
- Set `runAsNonRoot: true` with a non-zero `runAsUser`
- Scope `serviceMonitorSelector` to `matchLabels: app: hypershell-api-server`
- Scope `serviceMonitorNamespaceSelector` to `hypershell-system`
- Retain data for `7d`
- Request persistent storage via a `volumeClaimTemplate`

The `ClusterRole` SHALL grant `get`, `list`, and `watch` on `nodes`, `nodes/metrics`, `services`, `endpoints`, `pods`, `configmaps`, `ingresses`, and the monitoring CRDs (`servicemonitors`, `podmonitors`, `prometheusrules`), and `GET` on the `/metrics` non-resource URL.

#### Scenario: Fresh cluster receives a working Prometheus stack

- GIVEN a cluster with no pre-existing Prometheus installation
- WHEN `deploy/kind/infrastructure` and `deploy/base` are applied
- THEN the Prometheus Operator SHALL be running
- AND a Prometheus instance SHALL be running in `hypershell-system`
- AND it SHALL successfully scrape `hypershell_gateways_total` from the API server

---

### Requirement: DASH-04 -- ServiceMonitor Scrape Configuration

A `ServiceMonitor` resource named `hypershell-api-server` SHALL exist in `hypershell-system` and SHALL be included in the base kustomization so it is applied to every cluster. The `ServiceMonitor` SHALL:

- Select Services with label `app: hypershell-api-server`
- Scrape the named port `metrics` at path `/metrics` over plain HTTP
- Use a scrape interval of `30s`

The named port `metrics` on the `hypershell-api-server` Service SHALL map to port `4433` on the pod, matching `DASH-02`.

#### Scenario: ServiceMonitor is applied as part of base

- GIVEN `deploy/base/kustomization.yaml` references `servicemonitor.yaml`
- WHEN kustomize builds the base
- THEN the `ServiceMonitor` resource SHALL be included in the rendered manifests

#### Scenario: Prometheus discovers and scrapes the API server

- GIVEN the `ServiceMonitor` and `Prometheus` CR are both present
- WHEN 30 seconds elapse after deployment
- THEN Prometheus SHALL have a scrape target for `hypershell-api-server:4433/metrics`
- AND `hypershell_gateways_total` SHALL appear in Prometheus's TSDB

---

### Requirement: DASH-05 -- BFF Metrics Proxy Route

The web-console BFF SHALL expose `GET /api/metrics/gateways` as a same-origin proxy route that queries Prometheus and returns the `hypershell_gateways_total` phase counts as JSON. The browser SHALL never contact Prometheus directly.

The BFF SHALL accept a `PROMETHEUS_URL` environment variable (validated as an HTTP or HTTPS origin, no credentials, no path) defaulting to `http://127.0.0.1:9090`. The route SHALL query `GET {PROMETHEUS_URL}/api/v1/query?query=hypershell_gateways_total` with a 10-second timeout.

The response SHALL be `{ "counts": { "Running": N, "Provisioning": N, "Degraded": N, "Failed": N } }` where each value is an integer. Phases absent from the Prometheus response SHALL NOT be omitted from the JSON; the client is responsible for defaulting absent phases to zero.

When Prometheus is unreachable or returns a non-`200` status, or when the Prometheus response body has `status != "success"`, the BFF SHALL respond with HTTP `502` and `{ "error": "Metrics unavailable", "statusCode": 502 }`. The BFF SHALL NOT propagate raw Prometheus error messages to the browser.

The `/api/metrics/gateways` route SHALL be exempt from the general `/api/*` proxy handler: it does not forward to the API server, it does not require or forward a bearer token, and it does not require a Prometheus authentication header in its current form.

When OIDC is enabled, the route SHALL still require a valid session (the existing session-enforcement hook applies to all non-exempt paths). When OIDC is disabled, no session is required.

#### Scenario: Successful metrics fetch

- GIVEN Prometheus is reachable at `PROMETHEUS_URL` and returns `hypershell_gateways_total` samples
- WHEN the SPA calls `GET /api/metrics/gateways`
- THEN the BFF SHALL respond with HTTP `200` and `{ "counts": { "Running": N, "Provisioning": N, "Degraded": N, "Failed": N } }`

#### Scenario: Prometheus unreachable

- GIVEN the BFF cannot connect to Prometheus within 10 seconds
- WHEN the SPA calls `GET /api/metrics/gateways`
- THEN the BFF SHALL respond with HTTP `502` and `{ "error": "Metrics unavailable", "statusCode": 502 }`
- AND no Prometheus error detail SHALL appear in the response body

#### Scenario: Prometheus returns a non-success status

- GIVEN Prometheus is reachable but returns `{ "status": "error", ... }`
- WHEN the BFF processes the response
- THEN the BFF SHALL respond with HTTP `502` and `{ "error": "Metrics unavailable", "statusCode": 502 }`

---

### Requirement: DASH-06 -- Dashboard Component

The `gateway-management-ui` shared library SHALL export a `GatewayMetricsDashboard` React component that renders the `hypershell_gateways_total` phase counts as four PatternFly `Card` components inside a `Gallery`.

Each card SHALL display the phase name (localized via `react-intl`) in the PatternFly semantic status color for that phase, the numeric count in a large heading, and a pluralized gateway count label. The phase-to-color mapping SHALL be:

| Phase | PatternFly token |
|---|---|
| Running | `--pf-t--global--color--status--success--default` |
| Provisioning | `--pf-t--global--color--status--info--default` |
| Degraded | `--pf-t--global--color--status--warning--default` |
| Failed | `--pf-t--global--color--status--danger--default` |

The component SHALL use TanStack Query with `queryKey: ["gateways", "metrics"]` and SHALL refetch every `30_000` ms to stay aligned with the Prometheus scrape interval.

While loading, the component SHALL render a PatternFly `Spinner` with a localized `aria-label`. When the query has errored or produced no data, the component SHALL render a PatternFly `EmptyState` with a localized error title and recovery guidance. When data is available, the component SHALL render all four phase cards, defaulting absent phases from the API response to `0`.

All user-visible strings SHALL be declared with `defineMessages` and rendered through `FormattedMessage` or `intl.formatMessage`. No literal string SHALL appear in JSX.

#### Scenario: Dashboard renders all four phase cards

- GIVEN `GET /api/metrics/gateways` returns `{ counts: { Running: 5, Provisioning: 2, Degraded: 1, Failed: 0 } }`
- WHEN `GatewayMetricsDashboard` mounts
- THEN four cards SHALL be rendered, one per phase, each showing the correct count
- AND the `Failed` card SHALL show `0`

#### Scenario: Loading state shows a spinner

- GIVEN the query has not yet resolved
- WHEN `GatewayMetricsDashboard` renders
- THEN a spinner with a localized `aria-label` SHALL be visible
- AND no phase cards SHALL be rendered

#### Scenario: Error state shows recovery guidance

- GIVEN `GET /api/metrics/gateways` returns HTTP `502`
- WHEN `GatewayMetricsDashboard` renders after the error
- THEN a localized error title and recovery guidance SHALL be shown
- AND no phase cards SHALL be rendered

#### Scenario: Dashboard auto-refetches

- GIVEN `GatewayMetricsDashboard` is mounted and has rendered successfully
- WHEN 30 seconds elapse
- THEN TanStack Query SHALL re-issue `GET /api/metrics/gateways`
- AND the displayed counts SHALL update to reflect the new response

---

### Requirement: DASH-07 -- Dashboard SPA Route and Navigation

The web console SPA SHALL expose the dashboard at the `/dashboard` route. The route SHALL be included in the React Router route tree under the authenticated application layout.

The application shell SHALL include a persistent sidebar navigation (`PageSidebar` / `Nav`) with links to both the Gateways list (`/`) and the Dashboard (`/dashboard`). The active link SHALL be highlighted based on the current `pathname`. All navigation labels SHALL be localized via `react-intl`.

The BFF SHALL recognise `/dashboard` as a valid application route (returning `index.html` for direct navigation and browser refresh) alongside the existing `/`, `/login`, `/gateways/new`, and `/gateways/:gatewayId` routes.

The `route-contract.json` file SHALL declare `"dashboard": "dashboard"` so the BFF and SPA share a single source of truth for the path.

#### Scenario: Direct navigation to /dashboard

- GIVEN a user navigates directly to `https://console.hypershell.localhost/dashboard`
- WHEN the BFF handles the `GET /dashboard` request
- THEN it SHALL respond with `index.html` and HTTP `200`
- AND the SPA SHALL render `GatewayMetricsDashboard`

#### Scenario: Dashboard link is active on the dashboard page

- GIVEN the current pathname is `/dashboard`
- WHEN the application shell renders the sidebar
- THEN the Dashboard nav item SHALL be marked active
- AND the Gateways nav item SHALL NOT be marked active

#### Scenario: Gateways link is active on the home page

- GIVEN the current pathname is `/`
- WHEN the application shell renders the sidebar
- THEN the Gateways nav item SHALL be marked active
- AND the Dashboard nav item SHALL NOT be marked active

---

### Requirement: DASH-08 -- Kind Local Development Configuration

When running locally under Kind, the web-console Deployment SHALL be patched with `PROMETHEUS_URL` set to the in-cluster Prometheus service address (`http://prometheus-operated.hypershell-system.svc.cluster.local:9090`) so the BFF metrics route resolves without manual configuration.

This patch SHALL live in `deploy/kind/kustomization.yaml` alongside the existing web-console OIDC patch. It SHALL NOT appear in `deploy/base` because the Prometheus service address is environment-specific.

#### Scenario: BFF resolves Prometheus in Kind

- GIVEN the Kind cluster is running with `make kind-up`
- WHEN the BFF handles `GET /api/metrics/gateways`
- THEN it SHALL forward the query to `http://prometheus-operated.hypershell-system.svc.cluster.local:9090`
- AND the dashboard SHALL display live gateway phase counts
