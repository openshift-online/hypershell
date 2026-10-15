# Feature Specification: Provisioning Dashboard

**Status**: Draft
**Version**: 0.2.0
**Created**: 2026-10-11
**Applies to**: `components/web-console`, `packages/operational-dashboard-ui`

## Goals

- Provide platform administrators with a view of hub-cluster capacity and platform-wide gateway provisioning health.
- Let administrators compare the selected last 7 days or last 30 days of daily resource and reliability trend data.
- Reuse the existing operational-dashboard application boundary and platform metric contracts.

## Non-Goals

- This dashboard SHALL NOT provide resource-scoped views for non-administrators.
- This dashboard SHALL NOT replace the operational overview, reliability dashboard, or gateway metrics dashboard.
- This dashboard SHALL NOT display historical node or pod trends.
- This dashboard SHALL NOT add new provisioning metrics when an existing platform metric contract provides the data.

## Actors

- **Platform administrator**: An authenticated user with the `platform:admin` role.
- **Non-administrator**: An authenticated user without the `platform:admin` role.
- **Web-console host**: The SPA and BFF that authenticate the actor, load dashboard data, and render the page.

## User Scenarios & Testing

### User Story 1 - Review platform provisioning health (Priority: P1)

As a platform administrator, I want to see current hub-cluster capacity, node and pod health, and platform-wide gateway provisioning health so that I can identify provisioning risks.

**Why this priority**: The dashboard is an administrator-only platform view and its current-state widgets are the minimum useful capability.
**Independent Test**: Load `/dashboard/provisioning` as a `platform:admin` user with complete current metrics and verify all seven widgets render.

**Acceptance Scenarios**:

1. **Given** an authenticated user has the `platform:admin` role, **when** the user navigates to `/dashboard/provisioning`, **then** the Provisioning dashboard renders with hub-cluster infrastructure metrics and platform-wide gateway provisioning metrics.
2. **Given** an authenticated user does not have the `platform:admin` role, **when** the user navigates to `/dashboard/provisioning`, **then** the web console redirects the user to `/dashboard`.
3. **Given** OIDC is enabled and the user is unauthenticated, **when** the user requests `/dashboard/provisioning`, **then** the BFF denies access and the SPA does not expose platform metrics.

### User Story 2 - Compare historical ranges (Priority: P1)

As a platform administrator, I want to switch between the last 7 days and last 30 days so that I can investigate short-term changes and longer-term trends.

**Why this priority**: Range changes are the primary interaction in the approved mockup and must update historical visualizations together.
**Independent Test**: Start with Last 7 days, select Last 30 days, and verify every historical chart uses the new range while current snapshot values remain current.

**Acceptance Scenarios**:

1. **Given** the dashboard displays Last 7 days, **when** the administrator selects Last 30 days, **then** the Memory, CPU, and Provision reliability charts request and display the 30-day data set when data exists.
2. **Given** the dashboard displays Last 30 days, **when** the administrator selects Last 7 days, **then** all three historical charts request and display the 7-day data set when data exists.
3. **Given** an administrator changes the historical range, **when** the new data loads, **then** current snapshot values remain current values and do not become historical-range aggregates.
4. **Given** a selected range contains no sample for a calendar day, **when** a trend chart renders, **then** the chart shows a visible gap for that day and does not insert a zero or interpolate a value.

### User Story 3 - Understand provisioning trends (Priority: P1)

As a platform administrator, I want trend direction and percentile information to be understandable so that I can distinguish worsening capacity from improving provisioning performance.

**Why this priority**: The dashboard combines resource utilization and provisioning outcomes, whose favorable directions differ.
**Independent Test**: Supply trend data whose first and last values differ by at least 5 percent and verify metric-specific arrow semantics and chart series.

**Acceptance Scenarios**:

1. **Given** Memory used or CPU used changes by at least 5 percent, **when** the widget renders, **then** the dashboard shows an unfavorable indicator for increased usage and a favorable indicator for decreased usage.
2. **Given** Provision reliability changes by at least 5 percent, **when** the widget renders, **then** the dashboard shows a favorable indicator for increased success rate and an unfavorable indicator for decreased success rate.

## Requirements

### Functional Requirements

- **FR-001**: The web console SHALL expose the Provisioning dashboard at `/dashboard/provisioning`.
- **FR-002**: The web console SHALL permit access to the Provisioning dashboard only for authenticated users with the `platform:admin` role when OIDC is enabled.
- **FR-002a**: When an authenticated user without the `platform:admin` role requests `/dashboard/provisioning`, the web console SHALL redirect the user to `/dashboard`.
- **FR-003**: The BFF SHALL enforce the same `platform:admin` authorization on every metrics route used exclusively by the Provisioning dashboard.
- **FR-004**: The dashboard SHALL use hub-cluster infrastructure metrics for current and historical memory, CPU, node, and pod data; gateway provisioning metrics SHALL aggregate across the platform.
- **FR-005**: The dashboard SHALL provide exactly two historical range options: Last 7 days and Last 30 days.
- **FR-006**: The selected historical range SHALL apply to Memory, CPU, and Provision reliability trend data.
- **FR-006a**: Changing the selected historical range SHALL request the newly selected range from the backend rather than loading both ranges initially.
- **FR-006b**: While a newly selected range is loading, the dashboard SHALL retain the previously rendered range data until the new response is available.
- **FR-007**: The dashboard SHALL display a System summary containing managed-cluster, node, pod, and gateway totals, memory-used percentage, and CPU-used percentage.
- **FR-007a**: The System summary gateway count SHALL use the same `provisioned-gateways.value` total used by the operational dashboard.
- **FR-007b**: The System summary SHALL display the gateway total without a gateway status breakdown or historical trend indicator.
- **FR-008**: The dashboard SHALL display a Memory widget with current utilization and daily used-memory trend in GiB.
- **FR-009**: The dashboard SHALL display a CPU widget with current utilization and daily used-CPU trend in cores.
- **FR-010**: The dashboard SHALL display a Nodes widget with current ready and not-ready node counts and no historical trend.
- **FR-011**: The dashboard SHALL display a Pods widget with current pod statuses and counts returned by the backend and no historical trend.
- **FR-011a**: The Pods widget SHALL render every backend-provided status with its backend-provided label and count; the widget SHALL NOT require a fixed status vocabulary.
- **FR-012**: The dashboard SHALL display a Provision time widget with current average, P50, and P95 values. It SHALL NOT display provision-time line graphs until the backend can support them reliably.
- **FR-013**: The dashboard SHALL display a Provision reliability widget with current success and failure counts and a daily success-rate trend.
- **FR-014**: Memory and CPU trend values SHALL represent total used resources on the hub cluster.
- **FR-015**: Memory and CPU utilization percentages SHALL equal total used resource divided by total resource capacity on the hub cluster, expressed as a percentage.
- **FR-016**: Each trend chart SHALL identify the exact selected range in its accessible description or visible caption.
- **FR-017**: The dashboard SHALL show a trend indicator only when the first and last available daily values differ by at least 5 percent.
- **FR-017a**: The dashboard SHALL show Memory and CPU trend indicators in both the System summary and their individual widgets.
- **FR-018**: The dashboard SHALL preserve missing historical samples as gaps and SHALL NOT replace them with zero values or interpolated values.
- **FR-019**: When an individual metric source fails, the dashboard SHALL show a localized unavailable state for the affected widget and SHALL keep unaffected widgets usable.
- **FR-020**: When all initial metric sources fail, the dashboard SHALL show a localized total-failure state with recovery guidance.
- **FR-021**: The dashboard SHALL show an initial loading spinner with a localized accessible label before any metrics are available.
- **FR-022**: The dashboard SHALL not render `NaN`, `Infinity`, or `-Infinity`; non-finite values SHALL use the localized metric-unavailable state.
- **FR-023**: All user-visible strings, chart labels, trend descriptions, error states, and accessible names SHALL use the web console localization system.
- **FR-024**: The dashboard SHALL use the approved `Parity/Dashboards/Provisioning` Storybook story at `specs/web-console/mockups/dashboards/hub-cluster-dashboard.parity.stories.tsx` as its visual reference.

### Data Contracts

- **FR-025**: The dashboard data adapter SHALL reuse the existing operational-dashboard application boundary and SHALL NOT call the SDK, BFF, or browser `fetch` directly from presentation components.
- **FR-026**: The dashboard SHALL consume cluster memory, CPU, node, and pod metrics defined by the existing platform metric specifications.
- **FR-027**: The dashboard SHALL consume provision-duration data defined by `specs/platform/gateway-provision-time.spec.md`.
- **FR-028**: The dashboard SHALL consume provision-outcome data defined by `specs/platform/gateway-provision-outcomes.spec.md`.
- **FR-029**: The dashboard SHALL consume gateway totals from the existing gateway metrics contract and SHALL not depend on the paginated gateway list for platform totals.
- **FR-030**: The dashboard SHALL use UTC calendar days for historical trend points.
- **FR-031**: The dashboard SHALL use the existing operational-dashboard refresh behavior, including a 15-minute automatic refresh interval and a manual Refresh action.
- **FR-032**: The current Provision reliability success rate, successful count, and failed count SHALL use the previous 24 hours.
- **FR-033**: The current Provision time values SHALL use the existing Provision time API window; when the existing API does not define a window, the dashboard SHALL use the previous 24 hours.

## Key Entities

- **Current platform metrics**: Current values for managed-cluster inventory, hub-cluster memory, CPU, nodes, and pods, and platform-wide gateways, provision duration, and provision outcomes.
- **Historical trend series**: Daily values keyed by UTC calendar day for a selected 7-day or 30-day range.
- **Selected historical range**: The dashboard state identifying either 7 days or 30 days.

## Success Criteria

- **SC-001**: A `platform:admin` user can load the dashboard and see all seven widgets when all required metric sources return valid data.
- **SC-002**: Changing the range from 7 days to 30 days updates all four historical charts to the selected range without changing current snapshot values.
- **SC-003**: A non-administrator cannot receive Provisioning dashboard metrics when OIDC is enabled.
- **SC-004**: Missing daily samples render as visible gaps in 100 percent of affected trend charts.
- **SC-005**: A failure in one metric source leaves unaffected dashboard widgets interactive and displays a localized unavailable state for the failed source.
- **SC-006**: The production page matches the approved Provisioning Storybook reference through the repository UI completion gate.

## Assumptions

- The existing BFF and API metric routes provide the current and historical data required by this dashboard or can be extended under the referenced platform metric contracts.
- The existing dashboard secondary navigation already defines Provisioning as an administrator-only destination.
- The existing operational-dashboard package is the shared presentation and application boundary for this dashboard.
- The operational dashboard's `provisioned-gateways.value` already represents the required current gateway total.
