# Registered Users

**Status:** Active
**Applies to:** `components/api-server` users plugin and RBAC middleware, `components/sdk-typescript`, `components/web-console` dashboard adapter, `packages/operational-dashboard-ui`

## Purpose

Expose **registered users** - HyperShell `User` records auto-provisioned from JWT claims on first authenticated API access - to the operational dashboard so administrators can see how many identities have used the platform.

A registered user is a durable row in the API server database (`username`, optional `email` and `name`, `created_at`). This is **not** a live session count, Keycloak concurrent login metric, or "users online now" signal. Users who exist only in Keycloak and have never triggered HyperShell auto-provisioning SHALL NOT appear in the count.

User creation remains middleware-driven (see `security/rbac-enforcement.spec.md` User Auto-Provisioning). This specification adds a **read-only** inventory surface for dashboard operators and defines how the web console displays the total.

### Relationship to the operational dashboard

The operational dashboard widget currently labeled "Active users" (`active-users`) is a placeholder. This spec introduces the metric ID `registered-users`, connects it to the users List API, and renames user-facing copy to **Registered users** so the UI matches the data semantics.

Prometheus gateway metrics (`platform/gateway-metrics-dashboard.spec.md`) are unrelated.

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

- Caller holds an effective `platform:admin` RoleBinding (including JWT-synced realm role), **or**
- Caller presents a JWT whose `realm_access.roles` includes `hypershell-admins`

All other callers SHALL be denied:

- `GET /users` (collection) → HTTP `403`
- `GET /users/{id}` (singleton) → HTTP `404` (opaque denial per RBAC-11)

Service-account callers SHALL NOT bypass this check unless explicitly granted `platform:admin`.

The RBAC middleware SHALL treat resource `users` explicitly; user inventory SHALL NOT fall through to `gateway:creator` authorization.

#### Scenario: Gateway creator without platform admin cannot list users

- GIVEN a caller with only `gateway:creator`
- WHEN the caller sends `GET /api/hypershell/v1/users`
- THEN the API SHALL respond with HTTP `403`

#### Scenario: Platform admin can list users

- GIVEN a caller with effective `platform:admin`
- WHEN the caller sends `GET /api/hypershell/v1/users`
- THEN the API SHALL respond with HTTP `200`

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

### Requirement: RU-09 -- User Activity Statistics API

The API server SHALL expose `GET /api/hypershell/v1/users/stats` returning a `UserActivityStats` document with:

| Field | Meaning |
| --- | --- |
| `total_registered` | Count of all registered users |
| `registered_last_7_days` | Users whose `created_at` is on or after the UTC midnight at the start of the 7th calendar day before the evaluation day (`created_at >= evaluation_day - 6 days`) |
| `registered_last_30_days` | Users whose `created_at` is on or after the UTC midnight at the start of the 30th calendar day before the evaluation day (`created_at >= evaluation_day - 29 days`) |
| `active_last_7_days` | Distinct users with at least one authenticated API access on any of the last 7 UTC calendar days |
| `active_last_30_days` | Distinct users with at least one authenticated API access on any of the last 30 UTC calendar days |
| `registration_daily` | 30-element histogram of new registrations per UTC day (zeros for days with no signups) |
| `active_daily` | 30-element histogram of distinct active users per UTC day (zeros for days with no logins) |

The endpoint SHALL use the same dashboard-operator authorization as user inventory (RU-03).

Authenticated API access SHALL record the caller's user ID in a `user_login_days` table (one row per user per UTC day) and update `last_login_at` on the `User` record. Recording SHALL occur during JWT auto-provisioning on each authenticated request.

#### Scenario: Dashboard operator loads user activity stats

- GIVEN 42 registered users exist and 18 logged in during the last 7 UTC days
- AND an authorized dashboard operator opens `/dashboard`
- WHEN the adapter calls `GET /api/hypershell/v1/users/stats`
- THEN the response SHALL include `total_registered: 42` and `active_last_7_days: 18`
- AND `registration_daily` and `active_daily` SHALL each contain 30 dated buckets

#### Scenario: Registration windows use inclusive UTC midnight boundaries

- GIVEN the evaluation time is `2026-09-08T15:30:00Z`
- AND four users exist with `created_at` at `2026-09-02T00:00:00Z`, `2026-09-01T23:59:59.999999999Z`, `2026-08-10T00:00:00Z`, and `2026-08-09T23:59:59.999999999Z` respectively
- WHEN `GetActivityStats` runs for that evaluation time
- THEN `registered_last_7_days` SHALL be `1` (only the user created at the 7-day window start)
- AND `registered_last_30_days` SHALL be `3` (the 7-day inclusive user, the user one nanosecond before that window, and the 30-day inclusive user)

---

### Requirement: RU-10 -- Users Widget Presentation

The `registered-users` widget SHALL render through `RegisteredUsersCard` with the user-facing title **Users**, showing:

- Total registered users as the primary heading (`value`), labeled **Registered users** (for example, `42 Registered users`)
- Rows for users added during the last 7 and 30 UTC days (`createdLast7Days`, `createdLast30Days`)
- Rows for users who logged in during the last 7 and 30 UTC days (`activeLast7Days`, `activeLast30Days`)
- A sparkline of `active_daily` (`activeTrend`) for the last 30 UTC days (30 data points)

The usage summary card SHALL show only the total registered user count under **Users**. It MAY also show a trend direction indicator derived from `activeTrend` when daily unique logins changed by at least 5% between the first and last points in the 30-day window.

#### Scenario: Widget shows total, additions, and login activity

- GIVEN user activity stats loaded successfully
- WHEN the registered-users widget renders
- THEN it SHALL display the total, 7/30-day addition counts, 7/30-day login counts, and the daily-login sparkline

---

### Requirement: RU-05 -- Operational Dashboard Metric

The operational dashboard host adapter SHALL populate an `OperationalMetric` with `id: "registered-users"` and fields mapped from `UserActivityStats`:

| Metric field | API field |
| --- | --- |
| `value` | `total_registered` |
| `createdLast7Days` | `registered_last_7_days` |
| `createdLast30Days` | `registered_last_30_days` |
| `activeLast7Days` | `active_last_7_days` |
| `activeLast30Days` | `active_last_30_days` |
| `trend` | `registration_daily` |
| `activeTrend` | `active_daily` |

The adapter SHALL obtain stats from `GET /api/hypershell/v1/users/stats` through the browser TypeScript SDK.

#### Scenario: Dashboard shows registered user total

- GIVEN 42 registered users exist
- AND an authorized dashboard operator opens `/dashboard`
- WHEN operational metrics load successfully
- THEN the `registered-users` metric SHALL have `value: "42"`
- AND the usage summary row SHALL display `42` under **Users**

#### Scenario: Unauthorized adapter call omits registered-users metric

- GIVEN the signed-in user lacks dashboard-operator API authorization
- AND at least one other metric source succeeds
- WHEN the host adapter calls `GET /api/hypershell/v1/users/stats`
- THEN the `registered-users` metric SHALL be omitted from the adapter response
- AND the dashboard SHALL show its localized partial-load warning (OP-DASH-09)
- AND the registered-users widget and usage-summary row SHALL render the localized metric-unavailable state (not a silent zero count)

---

### Requirement: RU-06 -- UI Presentation

The `registered-users` widget SHALL render through `RegisteredUsersCard` (see RU-10).

The usage summary card SHALL include a **Users** row sourced from the metric `value` (total only).

All user-visible strings SHALL use `defineMessages` in `operational-dashboard-ui` and SHALL be extracted into the web-console `locales/en.json` catalog.

#### Scenario: Widget shows the total and activity breakdown

- GIVEN `registered-users` is on the dashboard layout and metrics loaded successfully
- WHEN the widget renders
- THEN it SHALL display the metric `value` as the card heading
- AND the title SHALL read **Users**
- AND the 7/30-day addition and login rows and daily-login sparkline SHALL be visible

---

### Requirement: RU-07 -- Refresh and Error Semantics

Registered user stats SHALL load through the existing operational dashboard metrics query (`useGetMetricsData`) and SHALL inherit its refresh policy (`operationalDashboardRefreshMilliseconds`, currently 15 minutes) and manual refresh behavior defined in `web-console/operational-dashboard.spec.md` OP-DASH-09.

A failed `GET /api/hypershell/v1/users/stats` request SHALL fail only the registered-users metric source (OP-DASH-19); the dashboard SHALL NOT display `0` as a fallback count.

#### Scenario: Refresh updates the displayed total

- GIVEN the dashboard previously showed `42` registered users
- AND a new user auto-provisioned since the last refresh
- WHEN the operator activates manual refresh and the adapter succeeds
- THEN the displayed count SHALL update to `43`

---

### Requirement: RU-08 -- Verification

The API server SHALL include integration tests for:

- Authorized List and Get
- Forbidden List for non-admin callers
- Opaque 404 on unauthorized singleton Get
- Accurate `total` with `size=1`
- Authorized and forbidden `GET /users/stats`
- Login recording on authenticated API access
- Registration window boundary counting (`registered_last_7_days` and `registered_last_30_days` at inclusive UTC midnight edges)

The web console SHALL include unit tests for the dashboard adapter mapping `UserActivityStats` into `registered-users`.

The operational dashboard package SHALL update Storybook fixtures and `mockOperationalDashboardMetrics` with extended registered-user fields.

#### Scenario: CI exercises authorization and mapping

- GIVEN the integration test suite runs with RBAC enforcement enabled
- WHEN user inventory tests execute
- THEN they SHALL cover both allow and deny paths
- AND the dashboard adapter unit tests SHALL assert histogram and summary field mapping
