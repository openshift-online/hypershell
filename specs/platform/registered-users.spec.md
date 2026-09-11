# Registered Users

**Status:** Active
**Applies to:** `components/api-server` users plugin and RBAC middleware, `components/sdk-typescript`, `components/web-console` dashboard adapter, `packages/operational-dashboard-ui`

## Purpose

Expose **registered users** - HyperShell `User` records auto-provisioned from JWT claims on first authenticated API access - to the operational dashboard so administrators can see platform adoption: total registered identities, recent sign-ups, and authenticated API activity among registered users.

A registered user is a durable row in the API server database (`username`, optional `email` and `name`, `created_at`). This is **not** a live session count, Keycloak concurrent login metric, or "users online now" signal. Users who exist only in Keycloak and have never triggered HyperShell auto-provisioning SHALL NOT appear in any registered-user metric.

User creation remains middleware-driven (see `security/rbac-enforcement.spec.md` User Auto-Provisioning). This specification adds a **read-only** inventory surface for dashboard operators, defines daily activity recording for registered users, and defines how the web console displays adoption metrics.

### Relationship to the operational dashboard

The operational dashboard metric ID is `registered-users`. It connects to Prometheus gauges emitted by the API server users metrics collector via BFF `GET /api/metrics/registered-users`.

The **Users** widget (widget type `registered-users`) SHALL present:

- Hero total: registered user count
- **Added** counts for rolling 7-day and 30-day windows (from `created_at`)
- **Unique logins** rollups for rolling 7-day and 30-day windows (sum of daily unique-login counts; see RU-10)
- A 30-day sparkline of daily unique logins (UTC calendar days)

The usage summary **Users** row SHALL show the same total registered user count with an optional trend arrow driven by unique-login activity (RU-13).

The users List API remains available for future collection workflows but is not the preferred source for operational dashboard aggregates.

## Requirements

### Requirement: RU-01 -- Read-Only User Inventory API

The API server SHALL expose read-only HTTP endpoints:

- `GET /api/hypershell/v1/users` - paginated List
- `GET /api/hypershell/v1/users/{id}` - singleton Get

The endpoints SHALL NOT support Create, Patch, or Delete. User records SHALL continue to be created and updated only through JWT auto-provisioning middleware.

OpenAPI definitions SHALL live in `components/api-server/openapi/openapi.users.yaml` and SHALL be embedded in the composite OpenAPI document. `make generate` SHALL regenerate the Go and TypeScript SDK clients.

#### Scenario: Authenticated dashboard operator lists users

- GIVEN a caller authorized for user inventory (RU-03)
- WHEN the caller sends `GET /api/hypershell/v1/users?page=1&size=20&orderBy=username asc`
- THEN the API SHALL respond with HTTP `200` and a `UserList` body
- AND `items` SHALL contain at most 20 `User` resources ordered by username ascending
- AND `total` SHALL equal the number of registered users in the database

#### Scenario: User creation via API is not supported

- GIVEN an authenticated caller
- WHEN the caller sends `POST /api/hypershell/v1/users`
- THEN the API SHALL respond with HTTP `404` or `405` (route not registered)

---

### Requirement: RU-02 -- User Resource Schema

The `User` OpenAPI schema SHALL expose:

| Field | Type | Notes |
| --- | --- | --- |
| `id` | string | KSUID; read-only |
| `kind` | string | `"User"` |
| `href` | string | Self link |
| `username` | string | Required; unique; sourced from JWT `preferred_username` at provisioning |
| `email` | string | Optional |
| `name` | string | Optional display name |
| `created_at` | date-time | Read-only; first provisioning timestamp |

The schema SHALL NOT expose internal RBAC bindings, Keycloak subject identifiers, or session metadata.

#### Scenario: Auto-provisioned user appears in List output

- GIVEN user `alice` authenticated for the first time and auto-provisioned by middleware
- WHEN an authorized caller lists users
- THEN the response SHALL include a `User` with `username: "alice"` and a non-empty `created_at`

---

### Requirement: RU-03 -- User Inventory Authorization

User inventory endpoints SHALL require **dashboard-operator authorization**, matching the operational dashboard audience (`web-console/operational-dashboard.spec.md` OP-DASH-04):

- Caller holds an effective `platform:admin` RoleBinding (including JWT-synced realm role)

All other callers SHALL be denied:

- `GET /users` (collection) → HTTP `403`
- `GET /users/{id}` (singleton) → HTTP `404` (opaque denial per RBAC-11)

Service-account callers SHALL NOT bypass this check unless explicitly granted `platform:admin`.

The RBAC middleware SHALL treat resource `users` explicitly; user inventory SHALL NOT fall through to `gateway:creator` authorization.

Holding only the legacy Keycloak realm role `hypershell-admins` SHALL NOT grant user inventory access when `platform:admin` is absent.

#### Scenario: Gateway creator without platform admin cannot list users

- GIVEN a caller with only `gateway:creator`
- WHEN the caller sends `GET /api/hypershell/v1/users`
- THEN the API SHALL respond with HTTP `403`

#### Scenario: Platform admin can list users

- GIVEN a caller with effective `platform:admin`
- WHEN the caller sends `GET /api/hypershell/v1/users`
- THEN the API SHALL respond with HTTP `200`

#### Scenario: Legacy hypershell-admins role alone cannot list users

- GIVEN a caller whose JWT includes `hypershell-admins` but who lacks effective `platform:admin`
- WHEN the caller sends `GET /api/hypershell/v1/users`
- THEN the API SHALL respond with HTTP `403`

#### Scenario: Unauthorized singleton lookup is opaque

- GIVEN a caller without dashboard-operator authorization
- WHEN the caller sends `GET /api/hypershell/v1/users/{id}`
- THEN the API SHALL respond with HTTP `404`

---

### Requirement: RU-04 -- Paginated List Contract

`GET /api/hypershell/v1/users` SHALL support the standard HyperShell list query parameters: `page`, `size`, `orderBy`, `search`, and `fields`.

Default ordering SHALL be `username asc`. Maximum page `size` SHALL follow the same upper bound as other List endpoints (100).

The List response SHALL include accurate `page`, `size`, `total`, and `items` fields per the shared `List` schema.

#### Scenario: Total count is available without fetching every page

- GIVEN 250 registered users exist
- WHEN an authorized caller sends `GET /api/hypershell/v1/users?page=1&size=1&orderBy=username asc`
- THEN the response SHALL include `total: 250`
- AND `items` SHALL contain exactly one user

---

### Requirement: RU-05 -- Operational Dashboard Metric

The operational dashboard host adapter SHALL populate an `OperationalMetric` with `id: "registered-users"` and fields mapped from BFF `GET /api/metrics/registered-users` per RU-12.

| `OperationalMetric` field | Source (when present) | Meaning |
| --- | --- | --- |
| `value` | `total_registered` | Total registered users (decimal string) |
| `createdLast7Days` | `created_last_7_days` | Users whose `created_at` falls in the last 7 × 24 hours UTC |
| `createdLast30Days` | `created_last_30_days` | Users whose `created_at` falls in the last 30 × 24 hours UTC |
| `uniqueLoginsLast7Days` | `unique_logins_last_7_days` | Sum of daily unique-login counts over the last 7 UTC calendar days inclusive of today |
| `uniqueLoginsLast30Days` | `unique_logins_last_30_days` | Sum of daily unique-login counts over the last 30 UTC calendar days inclusive of today |
| `trend.points` | `daily_unique_logins` | One point per UTC calendar day for the last 30 days; `label` is `YYYY-MM-DD`, `value` is the daily unique-login count |

The adapter SHALL obtain all fields from the BFF Prometheus proxy. The adapter SHALL NOT paginate the users List API for dashboard aggregates.

When optional adoption fields are absent from the BFF response, the adapter SHALL still emit the `registered-users` metric if `total_registered` is present and SHALL omit only the missing optional fields (RU-14).

#### Scenario: Dashboard shows registered user total and adoption fields

- GIVEN 450 registered users exist with complete adoption metrics
- AND an authorized dashboard operator opens `/dashboard`
- WHEN operational metrics load successfully
- THEN the `registered-users` metric SHALL have `value: "450"`
- AND the usage summary **Users** row SHALL display `450`
- AND the Users widget SHALL show hero copy equivalent to `450 Registered users`
- AND added, unique-login rollups, and `trend.points` SHALL render when present

#### Scenario: Unauthorized BFF call omits registered-users metric

- GIVEN the signed-in user lacks dashboard-operator BFF authorization
- AND at least one other metric source succeeds
- WHEN the host adapter calls `GET /api/metrics/registered-users`
- THEN the `registered-users` metric SHALL be omitted from the adapter response
- AND the dashboard SHALL show its localized partial-load warning (OP-DASH-09)
- AND the Users widget and usage-summary row SHALL render the localized metric-unavailable state (not a silent zero count)

---

### Requirement: RU-06 -- Users Widget Presentation

The `registered-users` widget SHALL render through a dedicated **Users** card presentation (`UsersCard`), not the generic `MetricCard`.

The widget title SHALL read **Users** (localized). The hero line SHALL read `{value} Registered users` using the metric `value` (total registered count).

Below the hero, the widget SHALL render a 2×2 stat grid:

| Stat label | Field |
| --- | --- |
| Added (7 days) | `createdLast7Days` |
| Added (30 days) | `createdLast30Days` |
| Unique logins (7 days) | `uniqueLoginsLast7Days` |
| Unique logins (30 days) | `uniqueLoginsLast30Days` |

When `trend.points` is present, a `TrendSparklineChart` SHALL render below the stat grid with localized title **Unique logins per day** and subtitle **Last 30 days**.

When an optional field is absent (RU-14), the corresponding stat cell or sparkline SHALL render the localized metric-unavailable state for that subsection; the widget shell SHALL remain.

The usage summary card SHALL include a **Users** row sourced from the same metric `value`. User-facing copy SHALL use **Users** in the summary row label (not "Active users").

All user-visible strings SHALL use `defineMessages` in `operational-dashboard-ui` and SHALL be extracted into the web-console `locales/en.json` catalog.

The default layout template SHALL place `registered-users` at the same grid anchor as today (column 3, platform adoption second row; OP-DASH-10) with height `REGISTERED_USERS_WIDGET_HEIGHT` (taller than `METRIC_WIDGET_HEIGHT` to fit the stat grid and sparkline). Neighboring widget positions SHALL NOT change in this wave.

#### Scenario: Users widget shows adoption layout

- GIVEN `registered-users` is on the dashboard layout and metrics loaded successfully with all adoption fields
- WHEN the widget renders
- THEN the hero SHALL display the total registered count
- AND the four stat cells SHALL display their respective values
- AND the sparkline SHALL render with 30 daily points

#### Scenario: Missing login fields degrade the widget

- GIVEN `registered-users` has `value` and `createdLast7Days` but lacks `uniqueLoginsLast7Days` and `trend`
- WHEN the widget renders
- THEN the hero and available added stats SHALL display
- AND unique-login stat cells and the sparkline SHALL show localized unavailable states
- AND the widget SHALL NOT display zero as a fallback for missing login metrics

---

### Requirement: RU-07 -- Refresh and Error Semantics

Registered user metrics SHALL load through the existing operational dashboard metrics query (`useGetMetricsData`) and SHALL inherit its refresh policy (`operationalDashboardRefreshMilliseconds`, currently 15 minutes) and manual refresh behavior defined in `web-console/operational-dashboard.spec.md` OP-DASH-09.

A failed `GET /api/metrics/registered-users` request (HTTP non-success or missing `total_registered`) SHALL fail only the `registered-users` metric source (OP-DASH-19); the dashboard SHALL NOT display `0` as a fallback count.

A successful BFF response with `total_registered` but missing optional adoption fields SHALL NOT fail the metric source (RU-14).

#### Scenario: Refresh updates the displayed total

- GIVEN the dashboard previously showed `42` registered users
- AND a new user auto-provisioned since the last refresh
- WHEN the operator activates manual refresh and the adapter succeeds
- THEN the displayed count SHALL update to `43`

---

### Requirement: RU-08 -- Verification

The API server SHALL include integration tests for:

- Authorized List and Get
- Forbidden List for non-admin callers (including `hypershell-admins` without `platform:admin`)
- Opaque 404 on unauthorized singleton Get
- Accurate `total` with `size=1`
- Daily activity upsert idempotency (at most one row per user per UTC day)
- Activity retention pruning

The web console SHALL include unit tests for the dashboard adapter mapping BFF `GET /api/metrics/registered-users` responses into `registered-users`, including full and degraded payloads.

The operational dashboard package SHALL update Storybook fixtures and `mockOperationalDashboardMetrics` with adoption fields and `UsersCard` presentation.

#### Scenario: CI exercises authorization and mapping

- GIVEN the integration test suite runs with RBAC enforcement enabled
- WHEN user inventory and activity tests execute
- THEN they SHALL cover both allow and deny paths
- AND the dashboard adapter unit tests SHALL assert metric ID, stringified totals, optional adoption fields, and degraded omission behavior

---

### Requirement: RU-09 -- Registered Users Prometheus Collector and BFF Route

The API server SHALL register a Prometheus collector that emits user adoption gauges by querying the users DAO and daily-activity store on each scrape (RU-10, RU-11). When a required database query fails, the collector SHALL emit `prometheus.NewInvalidMetric` for the affected series.

The web-console BFF SHALL expose `GET /api/metrics/registered-users` as a same-origin proxy route that queries Prometheus instant vectors/scalars and returns JSON per RU-12. Dashboard-operator authorization SHALL match `web-console/operational-dashboard.spec.md` OP-DASH-04 (`platform:admin` only).

When Prometheus is unreachable or `hypershell_users_registered_total` cannot be resolved, the BFF SHALL respond with HTTP `502`.

When `total_registered` resolves but optional login-series queries fail, the BFF SHALL respond with HTTP `200` and a degraded JSON body per RU-14.

#### Scenario: Collector emits registered user gauge on scrape

- GIVEN 42 registered users exist in the database
- WHEN Prometheus scrapes the API server `/metrics` endpoint
- THEN a sample for `hypershell_users_registered_total` with value `42` SHALL be present

#### Scenario: BFF returns registered user adoption JSON

- GIVEN Prometheus returns current user adoption gauge values
- WHEN an authorized caller sends `GET /api/metrics/registered-users`
- THEN the BFF SHALL respond with HTTP `200` and JSON containing at least `total_registered`

---

### Requirement: RU-10 -- Registered User Daily Activity Recording

The API server SHALL record authenticated API activity for **registered users only** in RBAC middleware after JWT validation and user resolution.

For each authenticated request from a registered user, the server SHALL upsert at most one **daily activity** row per `(user_id, activity_date)` pair, where `activity_date` is the UTC calendar date of the request timestamp.

Requests from callers who are authenticated but not yet registered (no `users` row) SHALL NOT create activity rows.

Activity recording SHALL be best-effort: persistence failures SHALL be logged and SHALL NOT fail the API request.

#### Scenario: Repeated requests dedupe to one daily row

- GIVEN registered user `alice` sends multiple authenticated API requests on UTC day `2026-09-11`
- WHEN activity recording runs
- THEN exactly one daily activity row SHALL exist for `alice` on `2026-09-11`

#### Scenario: Same user on two UTC days counts twice in rollups

- GIVEN registered user `alice` has activity on UTC days `2026-09-10` and `2026-09-11`
- WHEN unique-login rollups are computed
- THEN the daily sparkline SHALL show `1` on each day
- AND the 7-day unique-login rollup SHALL count `2` (sum of daily counts, not distinct users across the window)

---

### Requirement: RU-11 -- User Adoption Prometheus Metrics

The users metrics collector SHALL emit the following Prometheus series on each scrape:

| Metric | Type | Meaning |
| --- | --- | --- |
| `hypershell_users_registered_total` | Gauge | Total registered users |
| `hypershell_users_created_last_7_days_total` | Gauge | Users with `created_at` in the last 7 × 24 hours UTC |
| `hypershell_users_created_last_30_days_total` | Gauge | Users with `created_at` in the last 30 × 24 hours UTC |
| `hypershell_users_unique_logins_daily_total` | Gauge | Daily unique registered users with API activity; label `activity_date` (`YYYY-MM-DD` UTC) |
| `hypershell_users_unique_logins_last_7_days_total` | Gauge | Sum of `hypershell_users_unique_logins_daily_total` over the last 7 UTC calendar days inclusive of today |
| `hypershell_users_unique_logins_last_30_days_total` | Gauge | Sum of daily counts over the last 30 UTC calendar days inclusive of today |

There SHALL be no historical backfill for login metrics before this instrumentation ships. Days without instrumentation SHALL appear as zero in the sparkline after rollout.

#### Scenario: Added windows use rolling UTC hours

- GIVEN a user was auto-provisioned 8 days ago UTC
- WHEN the collector scrapes
- THEN `hypershell_users_created_last_7_days_total` SHALL NOT include that user
- AND `hypershell_users_created_last_30_days_total` SHALL include that user

---

### Requirement: RU-12 -- Extended BFF JSON and Adapter Mapping

`GET /api/metrics/registered-users` SHALL return JSON with the following fields when Prometheus data is available:

| Field | Type | Required | Source |
| --- | --- | --- | --- |
| `total_registered` | number | Yes | `hypershell_users_registered_total` |
| `created_last_7_days` | number | No | `hypershell_users_created_last_7_days_total` |
| `created_last_30_days` | number | No | `hypershell_users_created_last_30_days_total` |
| `unique_logins_last_7_days` | number | No | `hypershell_users_unique_logins_last_7_days_total` |
| `unique_logins_last_30_days` | number | No | `hypershell_users_unique_logins_last_30_days_total` |
| `daily_unique_logins` | array of `{ date, count }` | No | `hypershell_users_unique_logins_daily_total` for the last 30 UTC calendar days, oldest first |

The host adapter SHALL map optional numeric fields to string optional fields on `OperationalMetric` and SHALL build `trend.points` from `daily_unique_logins`.

`OperationalMetric` SHALL gain optional `uniqueLoginsLast7Days` and `uniqueLoginsLast30Days` string fields and optional `createdLast7Days` for this metric (reusing existing `createdLast30Days` where applicable).

#### Scenario: Adapter maps daily series into trend points

- GIVEN the BFF returns `daily_unique_logins: [{ "date": "2026-09-01", "count": 5 }, { "date": "2026-09-02", "count": 7 }]`
- WHEN the adapter builds `registered-users`
- THEN `trend.points` SHALL equal `[{ label: "2026-09-01", value: 5 }, { label: "2026-09-02", value: 7 }]`

---

### Requirement: RU-13 -- Usage Summary Trend Arrow

The usage summary **Users** row SHALL display `registered-users.value` (total registered users).

When `registered-users.trend` is present, the row SHALL evaluate `getMetricTrendChange` (5% threshold between first and last trend points) and SHALL show the localized increase/decrease indicator used by other summary rows (OP-DASH-13).

The trend arrow SHALL reflect **unique login activity** (daily unique logins), not registered-user growth.

When `trend` is absent, the row SHALL show the total only without a trend indicator.

#### Scenario: Login activity decrease shows down arrow

- GIVEN `registered-users` has `value: "450"` and `trend.points` where the last day is at least 5% lower than the first day
- WHEN the usage summary renders
- THEN the Users row SHALL show `450` with a decrease trend indicator

---

### Requirement: RU-14 -- Degraded Adoption Payload

Partial availability SHALL be expressed by omitting optional JSON fields or adapter fields, not by synthesizing zero for missing login data.

| Condition | Behavior |
| --- | --- |
| `total_registered` available, login fields missing | Emit metric with `value` and any present added fields; omit login rollups and `trend` |
| `total_registered` missing | Treat `registered-users` source as failed (OP-DASH-19) |
| Login rollups present, `daily_unique_logins` missing | Emit rollups; omit `trend` and sparkline |
| Widget stat field missing | Localized unavailable state in that cell only |

The dashboard SHALL NOT fail the entire `registered-users` metric source solely because login-series queries failed when `total_registered` succeeded.

#### Scenario: Degraded BFF response still populates the metric

- GIVEN the BFF returns HTTP `200` with `total_registered: 450` and `created_last_30_days: 12` but without login fields
- WHEN the adapter processes the response
- THEN `registered-users` SHALL be present with `value: "450"` and `createdLast30Days: "12"`
- AND unique-login fields and `trend` SHALL be absent
- AND the Users widget SHALL render in degraded mode per RU-06

---

### Requirement: RU-15 -- Activity Retention

Daily activity rows SHALL be retained for **30 UTC calendar days** (aligned with the sparkline window). A scheduled or scrape-driven pruning job SHALL delete rows older than the retention window.

Pruning failures SHALL be logged and SHALL NOT block API requests or metric scrapes.

#### Scenario: Pruning removes expired rows

- GIVEN a daily activity row with `activity_date` older than 30 UTC days before today
- WHEN retention pruning runs
- THEN that row SHALL be deleted
