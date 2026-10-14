# Fleet User Usage

**Status:** Active
**Applies to:** `components/api-server` users plugin Prometheus metrics, `components/fleet-dashboard` BFF, `packages/fleet-dashboard-ui`

## Purpose

Expose per-user fleet usage - how many distinct registered users hold how many gateways and how many active sandboxes - on the fleet dashboard, so operators can see platform adoption and per-user footprint without querying the database or Prometheus directly.

A user's gateway count is the number of Gateways they own via `gateway:owner` RoleBindings (the binding created with each gateway); viewer bindings do not count. A user's sandbox count is the sum of `active_sandbox_count` across the gateways they own. Registered users who own nothing are included with zero counts so adoption (registered vs owning) stays visible.

The pipeline mirrors the existing fleet planes: the API server emits per-user Prometheus gauges queried from the database on each scrape; the fleet-dashboard BFF aggregates them across discovered instances into a cached `/api/users` JSON plane; the dashboard UI renders a per-user usage page behind a top-level tab.

### Relationship to registered users

`platform/registered-users.spec.md` covers aggregate adoption metrics (totals, sign-ups, unique logins) surfaced through the web console's operational dashboard. This specification covers per-user ownership counts surfaced through the fleet dashboard. Both read the same `users` table; neither exposes RoleBindings or user records directly.

## Requirements

### Requirement: FUA-01 -- Per-User Usage Metrics

The API server users plugin SHALL expose a custom Prometheus Collector emitting two gauges with `username` and `cluster` labels:

- `hypershell_users_gateways_total` - the number of gateways the user owns on that spoke cluster
- `hypershell_users_sandboxes_total` - the total active sandboxes across the gateways the user owns on that spoke cluster

The Collector SHALL query the database once per scrape via a single join of `users`, `role_bindings` (scope `gateway`, role `gateway:owner`, deduplicated per user-gateway pair), `gateways`, and `managed_clusters` (resolving each gateway's spoke cluster name), so a user holding multiple bindings for the same gateway is counted once and sandbox sums never double-count. Every registered user SHALL be emitted on every scrape, including users who own no gateways (both gauges zero, no cluster label), so downstream aggregation never silently drops inactive users. Summing a gauge over the `cluster` label SHALL yield the per-user fleet total.

When the database query fails, the Collector SHALL emit a `prometheus.NewInvalidMetric` for both descriptors so the scrape registers as failed rather than silently dropping the series.

The Collector SHALL be registered exactly once using the existing `sync.Once` in `RegisterUserMetrics`.

#### Scenario: Per-user gauges on a healthy scrape

- GIVEN users alice (7 owned gateways, 64 active sandboxes across them), bob (1 gateway, 0 sandboxes), and carol (no gateways)
- WHEN Prometheus scrapes the metrics endpoint
- THEN the response SHALL contain `hypershell_users_gateways_total{username="alice"} 7`, `hypershell_users_gateways_total{username="bob"} 1`, and `hypershell_users_gateways_total{username="carol"} 0`
- AND `hypershell_users_sandboxes_total{username="alice"} 64` with bob and carol at `0`

#### Scenario: Viewer bindings do not count

- GIVEN user dave holds only a `gateway:viewer` binding for one gateway
- WHEN Prometheus scrapes the metrics endpoint
- THEN dave's `hypershell_users_gateways_total` SHALL be `0`

#### Scenario: Database error surfaces as a scrape failure

- GIVEN the database is unreachable at scrape time
- WHEN Prometheus scrapes the metrics endpoint
- THEN the scrape SHALL record a failure rather than silently returning stale or absent series

---

### Requirement: FUA-02 -- BFF /api/users Plane

The fleet-dashboard BFF SHALL expose `GET /api/users` as an authenticated route (same auth middleware and cache.Snapshot envelope as the other `/api/*` planes) returning `{ "users": [{ "username", "gateways", "sandboxes", "clusters" }] }` sorted by username, where `clusters` is the sorted list of distinct spoke clusters (ManagedCluster names) the user's owned gateways run on, empty for users owning nothing.

The source SHALL aggregate `sum by (username, cluster)` over both FUA-01 gauges, scoped to the instances discovered from the gateway metric (the same instance filter as `/api/fleet`), merging the two gauge result sets so a username present in only one still yields a record with the missing side zero. Per-user totals SHALL be the sum over the cluster dimension; a cluster label is absent for zero-ownership users.

The source SHALL refresh on its own interval (`FD_REFRESH_USERS`, default 60s) and, unlike `/api/fleet`, SHALL fail the whole snapshot when either gauge query fails: a half-built usage table would silently under-report ownership. On failure the cache serves the last-good value flagged stale, matching the per-source degradation model (one dead source never blanks the dashboard).

#### Scenario: Aggregation across instances

- GIVEN two discovered instances where alice owns 5 gateways on one and 2 on the other
- WHEN the BFF refreshes the users source
- THEN the payload SHALL contain one record for alice with `gateways: 7`

#### Scenario: Spoke cluster attribution

- GIVEN alice owns gateways on spoke clusters useast-1 and euwest-1, across the discovered instances
- WHEN the BFF refreshes the users source
- THEN alice's record SHALL list `clusters: ["euwest-1", "useast-1"]` while her counts stay the fleet-wide sums

#### Scenario: One gauge failing fails the snapshot

- GIVEN the gateways gauge query succeeds but the sandboxes gauge query fails
- WHEN the BFF refreshes the users source
- THEN the refresh SHALL be recorded as an error and the previous snapshot served flagged stale

---

### Requirement: FUA-03 -- Fleet Users Page

The fleet-dashboard UI SHALL add a top-level tab ("Fleet users") next to the promotion topology map, rendering the users plane without unmounting the map's data planes.

The page SHALL show four summary cards - distinct users, gateway owners (users with at least one gateway), total gateways, total sandboxes - above a sortable table with columns username, gateways, and sandboxes plus a spoke-clusters column listing the distinct clusters each user's gateways run on (the localized none-value when the user owns nothing). The default sort SHALL be gateways descending. Sorting SHALL be client-side over the plane data.

The page SHALL use the shared `PlaneSection` loading/error/stale handling so a failed or stale users source degrades on its own without blanking the page, and TanStack Query with a 30s refetch interval.

All user-visible strings SHALL be declared in the central message catalog and rendered through `react-intl`. No username or fleet identity SHALL be hard-coded in the UI source (data-architecture.spec §3.5).

#### Scenario: Page renders cards and per-user rows

- GIVEN `/api/users` returns three users (one owning 7 gateways with 64 sandboxes, one owning 1 gateway, one owning nothing)
- WHEN the operator opens the Fleet users tab
- THEN the summary cards SHALL show distinct users 3, gateway owners 2, gateways 8, sandboxes 64
- AND the table SHALL list all three users, heaviest first

#### Scenario: Stale source degrades in place

- GIVEN the users source's last refresh failed and the BFF serves the last-good snapshot flagged stale
- WHEN the operator opens the Fleet users tab
- THEN the table SHALL render the last-known values with the stale-data warning, not an error state
