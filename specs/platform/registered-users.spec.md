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

### Requirement: RU-05 -- Operational Dashboard Metric

The operational dashboard host adapter SHALL populate an `OperationalMetric` with `id: "registered-users"` and `value` set to the decimal string of the user List `total`.

The adapter SHALL obtain `total` from the HyperShell REST API through the browser TypeScript SDK. It SHOULD use a single List request with `page=1` and `size=1` to minimize payload size; it SHALL NOT paginate through all user records when only the count is required.

The metric SHALL NOT include `trend`, `status`, `unit`, or `total` fields in version 1.

The operational dashboard package SHALL rename the widget and summary labels from **Active users** to **Registered users**. The widget type key SHALL change from `active-users` to `registered-users` in the layout template, widget mapping, usage summary, fixtures, and `DATA_SOURCES.md`.

#### Scenario: Dashboard shows registered user total

- GIVEN 42 registered users exist
- AND an authorized dashboard operator opens `/dashboard`
- WHEN operational metrics load successfully
- THEN the `registered-users` metric SHALL have `value: "42"`
- AND the usage summary row SHALL display `42` under **Registered users**

#### Scenario: Unauthorized adapter call surfaces dashboard error

- GIVEN the signed-in user lacks dashboard-operator API authorization
- WHEN the host adapter calls `GET /api/hypershell/v1/users`
- THEN the operational metrics load SHALL fail
- AND the dashboard SHALL show its localized load-error state (not a silent zero count)

---

### Requirement: RU-06 -- UI Presentation

The `registered-users` widget SHALL render through the existing `MetricCard` presentation (large numeric heading, localized title).

The usage summary card SHALL include a **Registered users** row sourced from the same metric.

All user-visible strings SHALL use `defineMessages` in `operational-dashboard-ui` and SHALL be extracted into the web-console `locales/en.json` catalog.

#### Scenario: Metric card shows the count

- GIVEN `registered-users` is on the dashboard layout and metrics loaded successfully
- WHEN the widget renders
- THEN it SHALL display the metric `value` as the card heading
- AND the title SHALL read **Registered users**

---

### Requirement: RU-07 -- Refresh and Error Semantics

Registered user counts SHALL load through the existing operational dashboard metrics query (`useGetMetricsData`) and SHALL inherit its refresh policy (`operationalDashboardRefreshMilliseconds`, currently 15 minutes) and manual refresh behavior defined in `web-console/operational-dashboard.spec.md` OP-DASH-09.

A failed users List request SHALL fail the entire `getOperationalMetrics` workflow; the dashboard SHALL NOT display `0` as a fallback count.

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

The web console SHALL include unit tests for the dashboard adapter mapping `UserList.total` into `registered-users`.

The operational dashboard package SHALL update Storybook fixtures and `mockOperationalDashboardMetrics` to use `registered-users` instead of `active-users`.

#### Scenario: CI exercises authorization and mapping

- GIVEN the integration test suite runs with RBAC enforcement enabled
- WHEN user inventory tests execute
- THEN they SHALL cover both allow and deny paths
- AND the dashboard adapter unit tests SHALL assert the metric ID and stringified total
