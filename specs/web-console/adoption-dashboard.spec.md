# Adoption Dashboard

**Status:** Draft
**Version:** 0.1.0
**Created:** 2026-10-16
**Applies to:** `packages/operational-dashboard-ui`, `components/web-console` SPA and BFF
**Tracks:** [GitHub issue 550](https://github.com/openshift-online/hypershell/issues/550), [HYPERSHELL-283](https://redhat.atlassian.net/browse/HYPERSHELL-283), [HYPERSHELL-290](https://redhat.atlassian.net/browse/HYPERSHELL-290)

## Purpose

Provide an admin-only **Adoption dashboard** at `/dashboard/adoption` where platform administrators can assess HyperShell user adoption, activity, conversion, and retention from aggregate data. The dashboard follows the operational-dashboard architecture: browser presentation code calls an application-owned dashboard port, the host adapter calls a same-origin BFF route, and the BFF obtains aggregates server-side.

The approved visual reference is the `Mockups/Dashboards/Adoption dashboard` story in `specs/web-console/mockups/dashboards/adoption-dashboard.stories.tsx`. The page surface is `AdoptionDashboardPageSurface` in `specs/web-console/mockups/dashboards/adoption-dashboard.tsx`.

## Requirements

### Requirement: ADO-DASH-01 -- Route and Access Control

The web console SPA SHALL expose the Adoption dashboard at `/dashboard/adoption` only to users with `platform:admin`. Non-administrators SHALL NOT see the Adoption destination in dashboard navigation and SHALL NOT be able to render its page content.

When OIDC is enabled, the BFF SHALL enforce the same role requirement for browser navigations to `/dashboard/adoption` and for `GET /api/metrics/adoption`. Authenticated non-administrators SHALL receive HTTP `403` on the metrics route. Unauthenticated callers SHALL receive HTTP `401` or the standard BFF re-authentication response. When OIDC is disabled in no-auth development mode, both routes SHALL remain open.

#### Scenario: Non-admin is turned away from the metrics route

- GIVEN OIDC is enabled
- AND an authenticated user does not have `platform:admin`
- WHEN the browser calls `GET /api/metrics/adoption`
- THEN the BFF SHALL return HTTP `403`
- AND the response SHALL NOT contain adoption metrics

#### Scenario: Direct navigation to the adoption dashboard

- GIVEN an authenticated platform administrator
- WHEN the browser requests `GET /dashboard/adoption`
- THEN the BFF SHALL return the SPA shell
- AND the SPA SHALL render the Adoption dashboard

---

### Requirement: ADO-DASH-02 -- Time Ranges

The dashboard SHALL support 7-day and 30-day ranges. The selected range SHALL update registration and activity metrics without a full page reload. The browser SHALL NOT add a dashboard-specific 90-day option or workaround.

The trend response SHALL retain one point per UTC calendar day for the trailing 30 days. Seven-day range selection SHALL select the corresponding current-period summary values but SHALL NOT truncate the 30-point trend contract.

#### Scenario: Changing the selected range

- GIVEN the Adoption dashboard is loaded with a 7-day range
- WHEN the user selects 30 days
- THEN the SPA SHALL request adoption metrics for the 30-day range through its application port
- AND new registrations and activity values SHALL update without a full page reload
- AND the trend response SHALL continue to contain 30 daily points

---

### Requirement: ADO-DASH-03 -- Presentation and Application Boundary

The `@openshift-online/hypershell-operational-dashboard-ui` package SHALL host `AdoptionDashboardPage` and export it for the web console host. The page SHALL render the four widget areas represented by the approved mockup:

1. Adoption summary
2. Registration to first resource
3. Organizations and fleets
4. Adoption trends

Presentation code SHALL NOT call `fetch`, the SDK, or a BFF route directly. The host SHALL inject an adoption metrics workflow through the existing dashboard provider boundary. The dashboard secondary navigation SHALL include Adoption for administrators and hide it for non-administrators.

#### Scenario: Host imports the adoption page

- GIVEN the web console route module renders `AdoptionDashboardPage`
- WHEN the page mounts inside `DashboardUiProvider`
- THEN the page SHALL load metrics only through the host-injected dashboard workflow

---

### Requirement: ADO-DASH-04 -- BFF Response Contract

The BFF SHALL expose an admin-only, same-origin `GET /api/metrics/adoption` route that returns snake_case JSON and supports the `range` query parameter with values `7` and `30`. The route SHALL accept only aggregate data sources and SHALL NOT return user-level records, usernames, identifiers, or personally identifiable information.

`registered_users` SHALL be required. Every other field SHALL be optional. An omitted optional field SHALL mean that the metric is unavailable. The route SHALL preserve successfully computed fields if another aggregate subquery fails, so the page can render a localized unavailable state for that field without blanking the rest of the page.

The response SHALL use this contract:

```json
{
  "registered_users": 450,
  "created_last_7_days": 48,
  "created_last_30_days": 123,
  "unique_logins_last_7_days": 312,
  "unique_logins_last_30_days": 398,
  "inactive_users_last_30_days": 52,
  "users_with_gateway": 306,
  "gateway_conversion_percent": 68.0,
  "daily_created_users": [{ "date": "2026-10-01", "count": 18 }],
  "daily_unique_logins": [{ "date": "2026-10-01", "count": 42 }],
  "retention_7_days_percent": 78.0
}
```

#### Scenario: Partial metric-source failure

- GIVEN the registered-users aggregate succeeds
- AND the retention aggregate fails
- WHEN the browser calls `GET /api/metrics/adoption`
- THEN the BFF SHALL return `registered_users` and every other successfully computed field
- AND the response SHALL omit `retention_7_days_percent`
- AND the dashboard SHALL render localized unavailable states only for affected fields

#### Scenario: Aggregate-only response shape

- GIVEN any successful adoption metrics response
- WHEN the BFF serializes the response
- THEN the response SHALL contain only the documented aggregate fields and nested daily points
- AND the response SHALL NOT contain user identifiers or user-level rows

---

### Requirement: ADO-DASH-05 -- Registration and Activity Definitions

All windows SHALL use UTC calendar days. Today means the current UTC calendar day and may be partial.

Qualifying activity SHALL be one or more authenticated HyperShell API requests by a non-deleted registered user on a UTC calendar day, as recorded in `user_daily_activity`.

The BFF SHALL compute:

- **Registered users:** count of non-deleted rows in `users`.
- **New registrations, 7 days:** count of non-deleted users whose `users.created_at` is within the rolling 7 by 24-hour UTC window.
- **New registrations, 30 days:** count of non-deleted users whose `users.created_at` is within the rolling 30 by 24-hour UTC window.
- **Daily active users:** distinct users with qualifying activity on the current UTC day-to-date.
- **Weekly active users:** distinct users with qualifying activity in the rolling 7 UTC calendar days inclusive of today.
- **Monthly active users:** distinct users with qualifying activity in the rolling 30 UTC calendar days inclusive of today.
- **Inactive users, 30+ days:** non-deleted users whose `created_at` is older than 30 UTC days and who have no `user_daily_activity` row in the trailing 30 UTC calendar days inclusive of today.
- **Daily registration trend:** one point per UTC calendar day for the trailing 30 days, where each point is the count of non-deleted users created on that day.

The response field `unique_logins_last_7_days` SHALL represent weekly active users. The response field `unique_logins_last_30_days` SHALL represent monthly active users. The response field `daily_unique_logins` SHALL represent daily active users by UTC day. These names remain in the BFF contract for compatibility with the existing operational metric model.

#### Scenario: Weekly active users

- GIVEN users performed qualifying activity on the current UTC day and each of the previous six UTC days
- WHEN the BFF computes weekly active users
- THEN it SHALL count each distinct non-deleted user once across all seven UTC calendar days

---

### Requirement: ADO-DASH-06 -- Conversion Definitions

The BFF SHALL compute **first-gateway conversion** as the count of distinct non-deleted users who have ever held a `gateway-owner` role binding. The calculation SHALL count all `role_bindings` rows whose role is `gateway-owner`, regardless of whether the binding or gateway has since been deleted, join those rows to non-deleted users, and count each user once.

The BFF SHALL compute first-gateway conversion percentage as `users_with_gateway / registered_users * 100`, rounded to one decimal place. If `registered_users` is zero, the route SHALL omit `gateway_conversion_percent` and the dashboard SHALL render the localized unavailable state.

First-sandbox conversion, median time to first resource, and organizations/fleets metrics are unavailable. The route SHALL omit their response fields and the page SHALL render the existing localized metric-unavailable title and body for each field.

#### Scenario: Gateway-owner role was later removed

- GIVEN a non-deleted user held a deleted `gateway-owner` role binding
- WHEN the BFF computes first-gateway conversion
- THEN it SHALL count that user once

---

### Requirement: ADO-DASH-07 -- Retention Definitions

The BFF SHALL compute 7-day retention as `|A intersection B| / |B| * 100`, rounded to one decimal place, where `A` is the distinct users with qualifying activity in the current 7 UTC calendar days inclusive of today and `B` is the distinct users with qualifying activity in the immediately preceding 7 UTC calendar days.

If `B` is empty, the route SHALL omit `retention_7_days_percent` and the dashboard SHALL render the localized unavailable state. The route SHALL NOT substitute zero.

Thirty-day retention is unavailable because a true previous-30-day comparison requires 60 days of activity history while the current activity store retains only 30 days. The route SHALL omit that field and the page SHALL render the localized unavailable state with the data limitation documented in `DATA_SOURCES.md`.

#### Scenario: Previous active set is empty

- GIVEN no users had qualifying activity in the preceding 7 UTC calendar days
- WHEN the BFF builds the adoption response
- THEN the response SHALL omit `retention_7_days_percent`
- AND the UI SHALL render the localized unavailable state

---

### Requirement: ADO-DASH-08 -- Adapter and Unavailable-State Behavior

The web console adapter SHALL map the adoption response into the existing `OperationalMetric` model and SHALL expose a distinct adoption metric source identifier for partial-failure reporting. The adapter SHALL propagate the selected 7-day or 30-day range.

When an optional response field is missing, the corresponding dashboard field or widget SHALL render the existing localized unavailable states `app.dashboard.metricUnavailable.title` and `app.dashboard.metricUnavailable.body`. The UI SHALL NOT substitute zero, invent data, or silently hide a metric.

#### Scenario: Optional field is omitted

- GIVEN the adoption response omits `users_with_gateway`
- WHEN the dashboard renders the conversion widget
- THEN the corresponding value SHALL show the localized unavailable state
- AND all unaffected metrics SHALL continue to render

---

### Requirement: ADO-DASH-09 -- Documentation and Fixtures

The repository SHALL document adoption metric sources, UTC windows, availability limitations, and response-field semantics in `packages/operational-dashboard-ui/DATA_SOURCES.md`. The operational dashboard package SHALL provide deterministic adoption fixtures and Storybook production coverage for the page's available and unavailable states.

#### Scenario: Storybook renders deterministic adoption data

- GIVEN the web console Storybook adoption story uses the deterministic dashboard control-plane adapter
- WHEN the story renders
- THEN it SHALL show aggregate fixture values without live network dependencies

---

### Requirement: ADO-DASH-10 -- Tests

Tests SHALL cover:

- Admin-only page access
- BFF access control
- Aggregate metric query mapping
- Adapter response mapping
- Partial metric-source failures
- 7-day and 30-day range propagation
- Unavailable-state handling for omitted fields
- Aggregate-only response shape with no user identifiers

#### Scenario: Range propagates to the BFF

- GIVEN the Adoption dashboard is set to 30 days
- WHEN the host adapter loads metrics
- THEN it SHALL request `/api/metrics/adoption?range=30`
