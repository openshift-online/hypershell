# Agent SDLC Visibility Dashboard

**Status:** Draft
**Applies to:** `components/fleet-dashboard` BFF (new SDLC plane, shared bundle source), `packages/fleet-dashboard-ui` (tab shell, SDLC domain and presentation), `specs/web-console/mockups/dashboards` (approved mockup)
**Tracks:** [HYPERSHELL-401](https://redhat.atlassian.net/browse/HYPERSHELL-401)

## Purpose

Make the label-driven agent SDLC funnel observable in the fleet dashboard (the
"glass" dashboard, `glass.infra.hypershell.app`). The funnel today exists only as
GitHub issue labels on execution records: each issue moves through a single-valued
phase label state machine (`agent/workable` -> `agent/reviewable-spec` ->
`agent/review-spec-approved` -> `agent/reviewable-code` -> `agent/review-code-approved`
-> release), with reject/retry loops, paused states, and the `agent/allowed`
authorization gate (`specs/standards/sdlc/jira-github-sync.spec.md`,
`.hypershell/README.md`). There is no way to see where each item sits, how long it
has been there, what is stalled and needs a human, or which issue shipped in which
release bundle onto which hub cluster.

This specification adds an **Agent SDLC** tab beside the existing Fleet Map. The
tab renders an aging work-in-progress board (time-in-phase is the vertical axis),
a rework meter toward each in-review item's block threshold, an escalation lane
for blocked/paused/past-SLE work, and a release-by-hub-cluster matrix that closes
the loop from a merged issue to the bundle it shipped in and the cluster that bundle
runs on. The tab shares the dashboard chrome and the single release freight bar with
the Fleet Map. Making this tab's scope, search, selected card, escalations drawer,
and shipped selection reconstructible from the URL is specified separately in
[`glass-url-addressable-navigation.spec.md`](glass-url-addressable-navigation.spec.md).

The dashboard adds **no new data store**. SDLC state is read directly from GitHub
(Issues plus the Issue Events / Timeline API) behind an application-owned port, and
the issue-to-bundle mapping and bundle-to-hub-cluster deployment facts reuse the
BFF's existing, cached bundle truth that already feeds the promotion map. The state
source sits behind a provider port so a future Jira adapter can replace the GitHub
adapter with no domain or UI change.

### Relationship to the existing fleet dashboard

| Concern | Fleet Map (existing) | Agent SDLC (this spec) |
| --- | --- | --- |
| BFF plane | `/api/promotion`, `/api/fleet`, `/api/topology`, `/api/instances` | new `/api/sdlc` |
| Source | `pkg/sources/promotion.go`, `prometheus.go`, `topology.go` | new SDLC source + **reused** bundle source (`lock.go`, `bundles.go`, promotion cluster attribution) |
| UI view | promotion-topology map + freight bar | aging board + escalation lane + release x hub-cluster matrix |
| Port method | `FleetApi.getPromotion/getFleet/getTopology/getInstances` | `FleetApi.getSdlc` |
| Freight bar | above the stage | **the same** shared control, same location, drives both tabs |

### Source feasibility (scope boundary)

| Fact | Reachable | Notes |
| --- | --- | --- |
| Current phase / column, blocked and paused set | Yes (easy) | GitHub issues-by-label on the product repo, one query |
| Which bundle on which hub cluster | Yes (easy) | Already resolved by the promotion plane (`lock.go` release lock + `argoByInstance` cluster attribution) |
| Bundle contents (tracked issues + one-off PRs) | Yes (medium) | Existing bundle -> PR enrichment (`bundles.go`, compare API + `(#1234)` parsing) grouped under the work item each PR resolves to; the work-item -> PRs resolver only groups, adding no new GitHub calls |
| State over time, aging, rework count | Yes (hard, the crux) | GitHub Issue Events / Timeline API per issue, reconstructed live; label history is mutable and retained nowhere today |
| Durable SDLC analytics / history store | No | A retained history store would be a new api-server plugin and migration, out of scope for this BFF-only tab |

## Requirements

### Requirement: SDLC-VIS-01 -- BFF SDLC Plane

The fleet-dashboard BFF SHALL expose a new read-only plane `GET /api/sdlc` that
serves the agent SDLC funnel state. The plane SHALL be served through the existing
`pkg/cache.Source` envelope (`Snapshot` with `Data`, `GeneratedAt`, `Stale`,
`Error`) and the existing handler wrapper, so a never-refreshed source returns HTTP
`503`, a stale-but-serving source returns HTTP `200` with the `stale` flag set, and
a healthy source returns HTTP `200`. The plane SHALL sit behind the same
authenticated `/api/` middleware and SAR authorization firewall as the existing
planes; it SHALL NOT introduce a new auth path.

The plane SHALL be registered and run as its own refresh goroutine alongside the
existing sources, with its own configurable refresh interval
(`FD_REFRESH_SDLC`) and the shared metrics observer.

#### Scenario: Healthy SDLC snapshot is served

- GIVEN the SDLC source has refreshed at least once
- WHEN a client requests `GET /api/sdlc`
- THEN the BFF SHALL respond with HTTP `200` and the SDLC snapshot envelope

#### Scenario: SDLC source not yet refreshed

- GIVEN the SDLC source has never completed a refresh
- WHEN a client requests `GET /api/sdlc`
- THEN the BFF SHALL respond with HTTP `503`

#### Scenario: SDLC source stale but serving

- GIVEN the SDLC source last refresh failed but a prior snapshot exists
- WHEN a client requests `GET /api/sdlc`
- THEN the BFF SHALL respond with HTTP `200`
- AND the envelope SHALL carry `stale: true` and the last `error`

### Requirement: SDLC-VIS-02 -- SDLC State Behind a Provider Port

The SDLC state source SHALL sit behind an application-owned provider port in the
BFF (for example `SdlcStateProvider`) that the plane depends on. The port SHALL
express, in domain terms independent of any source system: the set of in-scope
execution records with their current phase, the per-issue label-event history
(state over time), and the per-issue rework count. A **GitHub-label adapter** SHALL
be the only implementation shipped by this spec.

The port SHALL be shaped so that a future **Jira adapter** is a drop-in behind the
same interface with no domain or UI change, consistent with the direction in
`specs/standards/sdlc/jira-github-sync.spec.md`. The Jira adapter SHALL NOT be
implemented by this spec; only the seam SHALL exist. Issue-to-bundle membership
SHALL NOT be part of this port: it is a separate GitHub-sourced concern
(SDLC-VIS-03) regardless of which state adapter is active.

#### Scenario: GitHub adapter satisfies the port

- GIVEN the BFF is configured with the GitHub-label adapter
- WHEN the SDLC plane refreshes
- THEN it SHALL obtain in-scope records, label-event history, and rework counts
  through the `SdlcStateProvider` port only
- AND it SHALL NOT call GitHub directly from the plane or the domain

#### Scenario: Port carries no bundle membership

- GIVEN the `SdlcStateProvider` port definition
- WHEN a reader inspects its surface
- THEN it SHALL expose issue state, label-event history, and scope
- AND it SHALL NOT expose bundle membership, which stays the shared GitHub bundle
  concern of SDLC-VIS-03

### Requirement: SDLC-VIS-03 -- Shared, Cached Bundle Truth

The SDLC plane SHALL build the release-by-hub-cluster matrix and the issue-to-bundle
mapping on the **same** cached bundle source the promotion plane already uses
(`pkg/sources/lock.go` release-lock resolution and history, `pkg/sources/bundles.go`
bundle-to-PR enrichment, and the promotion source's `argoByInstance` cluster
attribution). It SHALL NOT re-derive bundles, re-read the release lock, or re-call
the GitHub compare API in a parallel pipeline.

Each bundle row's contents SHALL be driven by that bundle's own merged pull requests
as the shared bundle source already resolves them (`bundles.go` bundle-to-PR
enrichment, the same PR set the Fleet Map freight bar shows). The plane SHALL NOT
introduce any additional GitHub request to populate bundle contents, and it SHALL NOT
hide a pull request that shipped in a bundle because its issue link is unrecoverable.

The plane SHALL split each bundle's pull requests into two groups:

- **Tracked issues** -- pull requests that resolve to an in-scope execution-record
  issue, grouped under that issue. A single issue MAY contribute several pull requests
  to one bundle; it SHALL appear once in that bundle with its pull requests grouped
  beneath it.
- **Other pull requests** -- pull requests with no in-scope execution-record issue,
  listed individually.

The grouping SHALL use a pull-request-to-work-item lookup inverted from the
`WorkItemPRResolver` port: the resolver returns each in-scope work item's associated
pull requests (shaped work item -> PRs, since a single work item MAY be delivered by
several pull requests), and the plane inverts those results into a
pull-request-to-issue map. The port SHALL be used only to **group** a bundle's pull
requests under the tracked issue they belong to; it SHALL NOT be used to attribute
issues to bundles, and bundle contents SHALL NOT depend on a closing keyword parsed
out of an individual PR body. The plane SHALL NOT stand up a duplicate
bundle-resolution pipeline for this grouping.

This hop SHALL live behind the `WorkItemPRResolver` port so the unit of work can change
without a domain or matrix change: the GitHub adapter resolves an issue's linked pull
requests (authoritatively via GraphQL `closedByPullRequestsReferences` when an upstream
token is configured, and via the REST issue timeline's `cross-referenced` events when
unauthenticated), and a future **Jira adapter** SHALL satisfy the same port by
returning the PRs linked to a Jira item. The matrix SHALL depend only on the port, not
on any issue-tracker specifics.

A tracked work item SHALL be treated as released when any of its pull requests appears
in a resolved bundle's SHA compare window, OR when it merged at or before the oldest
resolved bundle's build date (SDLC-VIS-11a); a pre-horizon item is contained in that
oldest bundle and every newer one even though no compare window reaches back to it. Only
a tracked work item that is neither window-placed nor pre-horizon -- genuinely merged
after the newest resolved bundle -- SHALL collect in the Unreleased row (SDLC-VIS-11),
which holds only genuinely in-flight tracked work.

The bundle-to-hub-cluster facts (which bundle runs on hyp0/hyp1/hyp2, and drift between
them) SHALL be the promotion plane's existing release-plus-deployment facts, not a
re-resolution.

#### Scenario: Matrix reuses promotion bundle and cluster facts

- GIVEN the promotion plane has resolved each hub cluster's current bundle
- WHEN the SDLC plane builds the release x hub-cluster matrix
- THEN each cell's bundle-per-hub-cluster data SHALL come from the shared bundle
  source
- AND the SDLC plane SHALL NOT independently resolve the release lock or call the
  compare API

#### Scenario: Bundle contents come from the bundle's pull requests

- GIVEN the bundle-to-PR enrichment has resolved a bundle's merged PRs
- WHEN the SDLC plane builds that bundle's contents
- THEN every pull request in the bundle's PR set SHALL appear in the bundle, grouped
  as a tracked issue or listed as an other pull request
- AND the plane SHALL NOT issue an additional GitHub request to populate the contents

#### Scenario: Tracked and one-off pull requests are grouped

- GIVEN a bundle whose PR set contains a pull request linked to an execution-record
  issue and a pull request with no such issue
- WHEN the SDLC plane groups the bundle's pull requests
- THEN the linked pull request SHALL appear under its tracked issue
- AND the unlinked pull request SHALL appear under the bundle's other pull requests

#### Scenario: Tracked issue delivered by multiple PRs in one bundle

- GIVEN a bundle whose PR set contains two pull requests that resolve to the same
  execution-record issue
- WHEN the SDLC plane groups the bundle's pull requests
- THEN that issue SHALL appear once under the bundle's tracked issues
- AND both of its pull requests SHALL be grouped beneath it

#### Scenario: Unauthenticated deployment resolves associations

- GIVEN no upstream token is configured
- WHEN the resolver looks up a work item's associated pull requests
- THEN it SHALL read them from the REST issue timeline's cross-referenced pull requests
- AND it SHALL NOT require GitHub GraphQL, which is unavailable unauthenticated

### Requirement: SDLC-VIS-04 -- Shared Refresh Across All Consumers

The SDLC plane SHALL fetch from GitHub once per refresh cycle and serve every
dashboard viewer from the cached snapshot. A viewer loading the dashboard SHALL NOT
trigger a GitHub re-request. "Near real time" SHALL be achieved by the BFF refresh
interval plus the UI's TanStack Query `refetchInterval` against the cached plane,
not by push. The refresh loop SHALL be bounded so the dashboard never approaches
GitHub rate limits regardless of the number of concurrent viewers.

#### Scenario: Viewer load does not call GitHub

- GIVEN the SDLC snapshot is fresh in cache
- WHEN additional viewers open the dashboard and poll `/api/sdlc`
- THEN each request SHALL be served from the cached snapshot
- AND no additional GitHub request SHALL be made on account of the viewer load

#### Scenario: Request volume is independent of viewer count

- GIVEN any number of concurrent dashboard viewers
- WHEN the dashboard runs
- THEN the GitHub request rate SHALL be bounded by the refresh loop alone

### Requirement: SDLC-VIS-05 -- Fleet-Identity Firewall Preserved

The SDLC plane SHALL obtain the source repository slug and any allowlist from
`FD_*` environment variables, never compiled in, consistent with the fleet-identity
firewall (`pkg/config`). The funnel's execution records live on the public product
repository, which is configured through `FD_UPSTREAM_REPO`; issue titles and labels
are public product-repo data and are permitted at runtime. The gitops repository
slug (`FD_GITHUB_REPO`) and bundle resolution SHALL remain governed by the existing
firewall rules.

#### Scenario: Source repo comes from env

- GIVEN the BFF starts
- WHEN the SDLC adapter resolves the source repository
- THEN it SHALL read the repository slug from `FD_*` configuration
- AND no fleet-identifying slug SHALL be compiled into the binary

### Requirement: SDLC-VIS-06 -- Hexagonal UI Boundary

The Agent SDLC tab SHALL follow the narrow hexagonal UI boundary
(`specs/standards/ui/hexagonal-architecture.spec.md`). The addition SHALL comprise:
a new framework-free SDLC domain type, a new `FleetApi.getSdlc(signal?)` port method
returning the `Plane<SdlcData>` freshness envelope, a new wire mapper in the
anti-corruption layer (`adapters/api/wire.ts`) that is the only place that knows the
`/api/sdlc` wire shape, and a new TanStack Query hook with its own
`refetchInterval`. React, TanStack Query, the HTTP adapter, and the generated wire
shapes SHALL stay outside the port; the domain and presentation SHALL depend only on
the port and domain types.

#### Scenario: SDLC data flows through the port and mapper

- GIVEN the UI renders the Agent SDLC tab
- WHEN it loads SDLC data
- THEN it SHALL call `getSdlc` through the application port
- AND the BFF JSON SHALL be translated to the domain type only in the wire mapper
- AND the `AbortSignal` SHALL propagate to the port method

#### Scenario: Freshness envelope is preserved

- GIVEN the BFF serves a stale SDLC snapshot
- WHEN the UI maps it
- THEN `generatedAt`, `stale`, and `error` SHALL survive mapping unchanged
- AND only `data` SHALL be translated to the domain type

### Requirement: SDLC-VIS-07 -- Tab Shell and Shared Freight Bar

The dashboard SHALL present the existing Fleet Map and the new Agent SDLC view as
two peer tabs sharing the dashboard chrome. The release **freight bar** SHALL be a
single shared control in the same location (above the stage) on both tabs:
selecting a bundle SHALL filter the SDLC board and SHALL drive the Fleet Map's open
bundle, so the two planes stay in lockstep. Switching tabs SHALL NOT discard the
selected bundle.

The tab control SHALL be keyboard accessible, expose an accessible name, and mark
the current tab as selected.

#### Scenario: Freight selection drives both tabs

- GIVEN the operator selects a bundle in the freight bar on the Agent SDLC tab
- WHEN they switch to the Fleet Map tab
- THEN the Fleet Map SHALL open that same bundle
- AND returning to the Agent SDLC tab SHALL keep the SDLC board filtered to it

#### Scenario: Both tabs reachable from the chrome

- GIVEN the dashboard is loaded
- WHEN the operator activates the Agent SDLC tab
- THEN the Agent SDLC view SHALL render in the shared chrome with the freight bar
  in its existing location

### Requirement: SDLC-VIS-08 -- Aging Work-In-Progress Board

The Agent SDLC view SHALL render an aging work-in-progress board whose columns are
the SDLC phases in funnel order (Intake, Workable, Spec, Spec approved, Code, Code
approved, Shipped). A card's **vertical position SHALL encode its time-in-phase**:
fresher work rides the top of a column and staler work sinks toward the bottom. Each
column SHALL draw a dashed **service-level-expectation (SLE) line** at that phase's
expected time-in-phase. Time-in-phase SHALL NOT be conveyed by color alone.

Clicking a card SHALL open that issue's full **label-event history** (its state over
time), reconstructed per SDLC-VIS-14. When a card changes phase between snapshots, the
whole card SHALL fly between its columns per SDLC-VIS-12a rather than vanish and
reappear.

#### Scenario: Card position reflects time-in-phase

- GIVEN two issues in the Spec column, one labeled an hour ago and one labeled a
  week ago
- WHEN the board renders
- THEN the week-old card SHALL sit lower in the column than the hour-old card

#### Scenario: Column shows its SLE line

- GIVEN a phase with a configured SLE (SDLC-VIS-13)
- WHEN its column renders
- THEN a dashed SLE line SHALL appear at the position corresponding to that
  time-in-phase

#### Scenario: Card opens its label-event history

- GIVEN a card on the board
- WHEN the operator activates it
- THEN its full label-event history SHALL open

### Requirement: SDLC-VIS-09 -- Rework Meter

Each in-review card SHALL show a **rework meter** that encodes its progress toward
the phase block threshold: spec review blocks at 3 rejections and code review blocks
at 10 (`specs/standards/sdlc/jira-github-sync.spec.md`, `.hypershell`). The meter
SHALL be a single proportional bar for all thresholds, so the encoding is identical
whether the limit is 3 or 10, and SHALL NOT be discrete pips. The meter SHALL
escalate in severity as it approaches the limit so that "one more strike and it
blocks" is visible, and the severity SHALL NOT be conveyed by color alone.

The rework count SHALL come from counting the issue's `labeled` events for the
reject labels (SDLC-VIS-14), because the funnel removes a reject label on retry and
the current label set does not retain the count.

#### Scenario: Spec and code meters share one encoding

- GIVEN a spec-review card at 2 of 3 rejections and a code-review card at 7 of 10
- WHEN the board renders
- THEN both SHALL use the same single proportional-bar encoding
- AND each SHALL show its fill proportional to its own threshold

#### Scenario: Near-threshold escalation

- GIVEN a spec-review card at 2 of 3 rejections
- WHEN its rework meter renders
- THEN it SHALL present the near-threshold escalated state
- AND that state SHALL be distinguishable without relying on color alone

### Requirement: SDLC-VIS-10 -- Escalation Lane

When a card crosses its column SLE, or is paused or blocked
(`agent/blocked`, `agent/needs-input`, `agent/duplicate`), it SHALL leave the board
and drop into a dedicated **Escalation lane** so columns stay bounded and the lane
reads as an action queue. The lane SHALL order items blocked first, then
needs-input and duplicate, then past-SLE. When the lane is empty, it SHALL present a
cleared state. The cleared state SHALL NOT be conveyed by color alone.

#### Scenario: Blocked item leaves the board for the lane

- GIVEN an issue carrying `agent/blocked`
- WHEN the view renders
- THEN that issue SHALL appear in the Escalation lane, not in a phase column

#### Scenario: Past-SLE item escalates

- GIVEN an issue whose time-in-phase has crossed its column SLE
- WHEN the view renders
- THEN that issue SHALL appear in the Escalation lane

#### Scenario: Lane ordering

- GIVEN the lane holds a blocked issue, a needs-input issue, and a past-SLE issue
- WHEN the lane renders
- THEN the blocked issue SHALL sort first, then needs-input, then past-SLE

#### Scenario: Empty lane shows cleared state

- GIVEN no blocked, paused, or past-SLE issues
- WHEN the lane renders
- THEN it SHALL present its cleared state, not conveyed by color alone

### Requirement: SDLC-VIS-11 -- Shipped Release by Hub-Cluster Matrix

The Shipped column SHALL render a release-by-hub-cluster matrix: rows are bundles
ordered newest to oldest (release history), columns are the hub clusters (hyp0
production, hyp1 stage, hyp2 int), and a marker SHALL sit in each hub cluster's
**current** bundle row so deployment and drift (how far behind a cluster runs) read
at a glance. Deployment and drift SHALL NOT be conveyed by color alone.

A bundle row SHALL expand to its contents (PR-driven, per SDLC-VIS-03) shown as two
groups: **Tracked issues** (pull requests grouped under the execution-record issue
they resolve to) and **Other pull requests** (one-off pull requests with no tracked
issue). Each entry SHALL show a "merged Nd ago" relative timestamp. The row summary
SHALL show both counts, the number of tracked issues and the number of pull requests
in the bundle (for example "2 issues, 4 PRs").

The **"Unreleased"** row SHALL hold only genuinely in-flight tracked work: shipped
execution-record issues merged after the newest resolved bundle's build boundary, whose
pull requests are therefore absent from every resolved bundle's compare window and which
did not merge at or before the oldest resolved bundle's build date (SDLC-VIS-11a). A
shipped issue whose delivering pull requests are already contained in a bundle -- whether
a recent bundle's compare window placed them or the issue merged before the oldest
resolved bundle -- SHALL NOT appear in the Unreleased row. A one-off pull request SHALL
NOT appear in the Unreleased row; it appears only under the bundle that shipped it. The
Unreleased row SHALL carry no version and no hub-cluster markers. When a bundle is
released whose PR set contains an Unreleased issue's pull requests, that issue SHALL move
from the Unreleased row into that bundle's row. The matrix SHALL NOT present a named
"building" bundle.

#### Scenario: Current bundle marked per hub cluster

- GIVEN hyp0 runs bundle v20260901 and hyp1 runs v20260815
- WHEN the matrix renders
- THEN a marker SHALL sit in the hyp0 column at the v20260901 row and in the hyp1
  column at the v20260815 row
- AND the drift between them SHALL be readable without relying on color alone

#### Scenario: Row expands to tracked issues and other PRs

- GIVEN a released bundle row whose PR set includes a pull request linked to an
  execution-record issue and a one-off pull request
- WHEN the operator expands it
- THEN it SHALL list the issue under Tracked issues and the one-off pull request under
  Other pull requests
- AND each entry SHALL show a "merged Nd ago" timestamp

#### Scenario: Row summary shows issue and PR counts

- GIVEN a bundle with two tracked issues and four pull requests in its PR set
- WHEN the bundle row renders
- THEN its summary SHALL show both counts, the tracked-issue count and the pull-request
  count

#### Scenario: One-off PR appears under its bundle, not Unreleased

- GIVEN a pull request that shipped in a bundle but resolves to no execution-record
  issue
- WHEN the matrix renders
- THEN that pull request SHALL appear under that bundle's Other pull requests
- AND it SHALL NOT appear in the Unreleased row

#### Scenario: Unreleased holds only in-flight tracked work

- GIVEN a shipped execution-record issue whose pull requests merged after the newest
  resolved bundle's build boundary and appear in no resolved bundle's compare window
- WHEN the matrix renders
- THEN that issue SHALL appear in the single Unreleased row
- AND the Unreleased row SHALL carry no version and no hub-cluster markers

#### Scenario: Next bundle absorbs Unreleased work

- GIVEN an issue in the Unreleased row
- WHEN a bundle is released whose PR set contains that issue's pull requests
- THEN that issue SHALL appear in that bundle's row instead of Unreleased

### Requirement: SDLC-VIS-11a -- Released Decision Independent of the Compare-Window Horizon

Whether a tracked execution-record issue is **released** or **Unreleased** SHALL be
decided by whether its delivering pull requests are actually contained in a shipped
bundle, not solely by whether a pull request falls inside one of the resolved bundles'
SHA compare windows. The resolved bundle set is bounded (the currently deployed bundles
plus the last `FD_RELEASE_HISTORY_LIMIT` distinct bundles from history), and the compare
windows span only from the oldest to the newest bundle in that set. A pull request merged
before the oldest resolved bundle therefore falls inside no window even though it is an
ancestor of every resolved bundle and is running on every hub cluster. Deciding
"released" by window membership alone wrongly reports such pre-horizon work as never
shipped, and the error grows with build cadence because a faster release cadence shortens
the span the resolved set covers.

The two conflated concerns SHALL be separated:

- **Per-bundle grouping** of recent pull requests (which recent bundle first introduced a
  pull request) SHALL continue to use the exact SHA-based compare windows, unchanged.
  Those windows remain correct for the recent frontier, including build lag near it.
- **The released-vs-unreleased decision** SHALL additionally treat a tracked work item as
  released when its merge time (`ClosedAt` / `MergedAt`) is at or before the **oldest
  resolved bundle's build date** (`Release.Date`). An item merged before the oldest bundle
  the dashboard can resolve is contained in that bundle and every newer one, so it SHALL
  be reported released, not Unreleased.

A tracked work item released only by this build-date boundary (window-unplaced but
pre-horizon) SHALL remain visible in a released position rather than being dropped: it
SHALL be grouped under the oldest resolved bundle row, or under a single collapsed
"Earlier releases" row. It SHALL NOT appear in the Unreleased row and SHALL NOT be
silently omitted from the matrix.

The decision SHALL be computed in-memory from the `Release.Date` fields already held on
the resolved bundles and the records' own merge times. It SHALL NOT issue any additional
GitHub request, open any new cache entry, or change the refresh cost or the
unauthenticated rate budget (SDLC-VIS-04). Per-pull-request ancestry probing against the
newest bundle's source revision SHALL NOT be used, because it would cost one GitHub call
per unplaced pull request on every refresh for a result the build-date boundary already
gives where precision matters.

Because the build-date boundary does not depend on pull-request-to-issue association
linkage, a pre-horizon item SHALL be reported released even when its association
cross-reference (SDLC-VIS-03) is momentarily unrecoverable; this also mitigates the
missing-association contributor for deep-tail work.

#### Scenario: Pre-horizon item is released, not Unreleased

- GIVEN a tracked execution-record issue whose delivering pull request merged at or before
  the oldest resolved bundle's build date
- AND none of its pull requests appear in any resolved bundle's compare window
- WHEN the matrix renders
- THEN the issue SHALL appear in a released position, not in the Unreleased row

#### Scenario: Pre-horizon released item stays visible

- GIVEN a tracked issue released only by the build-date boundary
- WHEN the matrix renders
- THEN the issue SHALL be grouped under the oldest resolved bundle row or a single
  collapsed "Earlier releases" row
- AND it SHALL NOT be silently dropped from the matrix

#### Scenario: Recent per-bundle grouping is unchanged

- GIVEN a pull request that falls inside a resolved bundle's SHA compare window
- WHEN the matrix groups bundle contents
- THEN that pull request SHALL be grouped under the bundle whose window contains it,
  exactly as before the fix

#### Scenario: Decision adds no GitHub calls

- GIVEN the SDLC plane recomputes the matrix on refresh
- WHEN it decides released versus Unreleased for every tracked record
- THEN it SHALL use only in-memory `Release.Date` and record merge-time comparisons
- AND it SHALL NOT issue any additional GitHub request on account of the decision

### Requirement: SDLC-VIS-12 -- Fleet Map Cross-Link

Activating a shipped item, or a bundle in the freight bar, SHALL jump to the Fleet
Map with that bundle opened (consistent with the shared freight bar, SDLC-VIS-07).

#### Scenario: Shipped item jumps to the map

- GIVEN a shipped issue in the matrix
- WHEN the operator activates it
- THEN the Fleet Map SHALL open with that issue's bundle selected

### Requirement: SDLC-VIS-12a -- Card Phase-Transition Flights

When an execution-record card changes phase between two SDLC snapshots, the whole
card SHALL fly from its old slot to its new slot rather than disappear from one
column and reappear in the next on the following refetch. The flying object SHALL be
a full-size clone of the card itself (its real size and content), not a shrunken
identicon or a glyph. The flight SHALL travel along an arc and land with an impact
("dust") burst, mirroring the Fleet Map promotion flights so the two surfaces read as
one motion language. A mid-flight scale swell MAY be used but SHALL stay subtle for a
full-size card. The flight SHALL respect the user's reduced-motion preference
(`specs/standards/ui/accessibility.spec.md`): when reduced motion is preferred, no
flight SHALL play and cards SHALL snap to their new slots.

This behavior SHALL reuse the Fleet Map's flight motion rather than fork it: the arc
path, the impact burst, and the shared flight duration (`FLIGHT_MS`) SHALL be a
single shared presentation primitive used by both the Fleet Map (SVG, flying an
identicon) and the Agent SDLC board (DOM, flying a full card). The board SHALL NOT
duplicate the arc, dust, or timing.

Flights SHALL cover every phase change, not forward moves alone:

- A forward move (for example Spec -> Spec approved) SHALL fly forward.
- A backward move (a reject loop, for example Spec approved -> Spec) SHALL fly
  backward.
- A card entering the board at intake SHALL fly in (the board analogue of the Fleet
  Map's release-tray special case).
- The last phase -> Shipped hop SHALL fly as a card into the release matrix, and this
  flight SHALL replace the previous separate corner-to-corner ship glyph animation.
  The board SHALL retain no separate ship-glyph animation.

Each simultaneous phase change in one refetch SHALL get its own flight.

#### Scenario: Forward phase change flies the whole card

- GIVEN a card whose phase advances between two snapshots
- WHEN the board renders the new snapshot
- THEN a full-size clone of the card SHALL fly along an arc from its old column slot
  to its new column slot
- AND it SHALL land with the same impact burst as a Fleet Map promotion flight

#### Scenario: Reject loop flies backward

- GIVEN a card whose phase moves to an earlier phase (a reject loop)
- WHEN the board renders the new snapshot
- THEN the card SHALL fly from its old slot back to the earlier phase's slot

#### Scenario: Shipped hop uses the same flight

- GIVEN a card whose phase reaches Shipped
- WHEN the board renders the new snapshot
- THEN the card SHALL fly into the release matrix using the shared flight primitive
- AND no separate ship-glyph animation SHALL play

#### Scenario: Reduced motion suppresses flights

- GIVEN the user prefers reduced motion
- WHEN any card changes phase
- THEN no flight SHALL play
- AND the card SHALL snap to its new slot

### Requirement: SDLC-VIS-12b -- Flight Detection and Stability

Phase-transition flights SHALL be derived purely by diffing the already-fetched SDLC
snapshot across polls (the board analogue of the Fleet Map's `computeFlights` pure
diff). A flight SHALL be emitted when a card's server **phase** changes between the
previous snapshot and the current one. Detection SHALL key off the server phase, NOT
the continuously aged copy, so the board's one-second continuous-aging re-render does
not spuriously emit or restart flights. No phase-transition flight SHALL cause a new
network call.

Active flights SHALL be registered with stable keys and self-expiring timers held in
a ref, with no effect-cleanup cancellation, mirroring the Fleet Map's flight registry
(`useFlights`). An unrelated poll or the one-second aging re-render landing mid-flight
SHALL NOT restart, cancel, or strand an in-flight flight.

While a flight is in the air, the destination SHALL NOT also show the resting card
(no double render): the destination slot SHALL be held (reserved placeholder or
skipped) until the flying copy lands, the board analogue of the Fleet Map's
`holdSeedById`. The source slot SHALL clear as the card departs so the card appears
to physically leave. When the flight lands, the hold SHALL release to the card's
current (aged) slot, not a stale one. The landing MAY occur at the computed
destination rect even when the destination column is scrolled such that the slot is
off-screen; the board SHALL NOT auto-scroll to it.

#### Scenario: Flight is derived from the snapshot diff, not aging

- GIVEN the one-second continuous-aging re-render runs with no phase change
- WHEN the board re-renders
- THEN no flight SHALL be emitted
- AND no new network call SHALL be made

#### Scenario: Aging re-render does not restart an in-flight flight

- GIVEN a card is mid-flight between columns
- WHEN the one-second aging re-render or an unrelated poll lands
- THEN the flight SHALL NOT restart, cancel, or strand

#### Scenario: Destination holds until the card lands

- GIVEN a card is flying to its new column
- WHEN the board renders during the flight
- THEN the destination SHALL NOT show the resting card until the flight lands
- AND the source slot SHALL clear as the card departs

#### Scenario: Simultaneous transitions each fly

- GIVEN two cards change phase in the same refetch
- WHEN the board renders the new snapshot
- THEN each card SHALL fly on its own flight

### Requirement: SDLC-VIS-13 -- SLE Values Are Configuration

Per-phase SLE values SHALL be configuration, not code, supplied as `FD_*`
environment variables set in hypershell-gitops, consistent with the dashboard's
existing `FD_*` configuration and the "separate configuration from code"
convention. The BFF SHALL ship sensible per-phase defaults overridable per
environment. Changing an SLE SHALL NOT require a code change.

#### Scenario: SLE overridden per environment

- GIVEN a per-phase SLE environment variable is set in hypershell-gitops
- WHEN the dashboard computes that column's SLE line and escalation threshold
- THEN it SHALL use the configured value
- AND omitting the variable SHALL fall back to the built-in default

### Requirement: SDLC-VIS-14 -- Label-Event History and Rework From the Timeline

The GitHub-label adapter SHALL reconstruct each in-scope issue's label-event history
from the GitHub Issue Events / Timeline API, reading `labeled` and `unlabeled`
events with their timestamps. Time-in-phase (SDLC-VIS-08), the per-column SLE
crossing (SDLC-VIS-10), and the rework count (SDLC-VIS-09) SHALL all be derived from
this reconstructed history. The rework count SHALL specifically be the count of
`labeled` events for the reject labels, since the funnel removes a reject label on
retry and the live label set does not retain it.

This reconstruction SHALL run inside the bounded, shared refresh loop (SDLC-VIS-04),
paginated and rate-limit-aware, and SHALL NOT be performed per viewer request.

#### Scenario: Time-in-phase from the latest phase label event

- GIVEN an issue whose current phase label was applied at a known time
- WHEN the adapter builds the snapshot
- THEN the issue's time-in-phase SHALL be measured from that `labeled` event

#### Scenario: Rework count survives reject-label removal

- GIVEN an issue that was labeled `agent/review-spec-rejected`, had it removed on
  retry, and was labeled rejected again
- WHEN the adapter computes the rework count
- THEN the count SHALL be 2, derived from the two `labeled` events

### Requirement: SDLC-VIS-15 -- Domain Observability Probes

The Agent SDLC tab SHALL publish typed workflow and dependency facts through the
dashboard's fan-out observability port, consistent with the "domain probes for UI
observability" convention and `specs/standards/ui/domain-observability.spec.md`.
SDLC load SHALL publish a workflow probe with outcomes `started`, `succeeded`,
`failed`, and `cancelled`. Production browser and BFF code SHALL NOT make raw
console or direct telemetry calls.

#### Scenario: SDLC load publishes workflow outcomes

- GIVEN the Agent SDLC tab loads its data
- WHEN the load succeeds, fails, or is cancelled
- THEN a workflow probe SHALL be published with the corresponding outcome
- AND no raw console or direct telemetry call SHALL be made in production code

### Requirement: SDLC-VIS-16 -- Accessibility and Localization

Every signal on the Agent SDLC tab SHALL be perceivable without color as the sole
channel (`specs/standards/ui/accessibility.spec.md`): time-in-phase, SLE crossing,
rework severity, escalation state, and deployment/drift SHALL each carry a
non-color cue (text, icon, shape, or position). All user-visible strings, including
relative timestamps such as "merged Nd ago" and any caller-supplied fallback labels
in reusable presentation helpers, SHALL be localized messages. Interactive elements
SHALL be keyboard operable and expose accessible names.

#### Scenario: Non-color cue for escalation

- GIVEN a card in the Escalation lane
- WHEN it renders
- THEN its escalated state SHALL be conveyed by a non-color cue in addition to any
  color

#### Scenario: Relative timestamp is localized

- GIVEN a shipped issue showing "merged Nd ago"
- WHEN the UI renders it
- THEN the string SHALL come from the localized message catalog

### Requirement: SDLC-VIS-17 -- Access Control

The `/api/sdlc` plane and the Agent SDLC view SHALL use the fleet dashboard's
existing authentication middleware and SAR authorization firewall; they SHALL NOT
introduce a separate access-control path. When auth is disabled (dev mode), the
plane and view SHALL remain open, matching the other planes.

#### Scenario: SDLC plane honors the existing SAR gate

- GIVEN auth is enabled and the caller is not authorized by the SAR firewall
- WHEN the caller requests `GET /api/sdlc`
- THEN the BFF SHALL deny the request through the same middleware as the other
  planes

### Requirement: SDLC-VIS-18 -- Approved Mockup and Design-Review Stories

Because the Agent SDLC tab is a new production page, its approved visual design
SHALL be represented by a mockup trio under
`specs/web-console/mockups/dashboards/` following the existing dashboard-mockup
convention: a dashboard surface (`agent-sdlc-dashboard.tsx`), a shell-wrapped
design-review Storybook story titled under `Mockups/Dashboards/*`, and a shell-free
parity Storybook story titled under `Parity/Dashboards/*`. The mockup SHALL defer to
PatternFly 6 components and design tokens, reuse canonical shared components rather
than bespoke ones, and follow the repo UI standards; where the attached issue spike
and an existing pattern or spec disagree, the existing pattern or spec SHALL win.
The mockup TSX and CSS SHALL pass
`skills/web-console/ui-spec/scripts/check_mockup_patternfly.py`, and the production
page SHALL go through `skills/build/ui-build-gate/SKILL.md`. The spike attached to
HYPERSHELL-401 is illustrative only and SHALL NOT be copied literally.

#### Scenario: Approved design exists as a parity story

- GIVEN the Agent SDLC tab is built
- WHEN the production page is verified against its approved design
- THEN a `Parity/Dashboards` Storybook story for the Agent SDLC tab SHALL exist and
  pass the mockup PatternFly check

### Requirement: SDLC-VIS-19 -- Tests and Storybook

The repository SHALL include: BFF tests for the SDLC source (snapshot envelope,
never-refreshed 503, stale-serving 200, PR-driven bundle contents grouped into tracked
issues and other pull requests, a tracked issue delivered by multiple PRs in one
bundle grouped once, the Unreleased row reduced to shipped issues merged after the newest
resolved bundle, a shipped record whose merge time precedes the oldest resolved bundle's
build date landing in a released row rather than Unreleased with no additional GitHub call
(the compare-window horizon regression, SDLC-VIS-11a), rework-from-timeline counting) and
for the `SdlcStateProvider` port
with its GitHub adapter; UI tests for the wire mapper, the tab shell and shared
freight-bar coupling, the aging-board position and SLE line, the rework meter encoding,
the escalation lane ordering, the phase-transition flight detection (a flight is emitted
on a server-phase change, forward and backward, and not on the one-second aging
re-render), the flight registry stability (an aging re-render or unrelated poll does not
restart or strand an in-flight flight) and reduced-motion suppression, and the matrix
including both bundle groups and the Unreleased row; and Storybook stories for default,
loading, stale, empty-lane, and mid-flight states using a mock SDLC port.

#### Scenario: Storybook shows the board and matrix from a mock port

- GIVEN a mock SDLC port fixture with issues across phases and a release history
- WHEN the default Agent SDLC Storybook story renders
- THEN it SHALL show the aging board, the rework meters, the escalation lane, and
  the release x hub-cluster matrix with its bundle contents grouped into tracked
  issues and other pull requests, including the Unreleased row

## Non-Goals

- A durable SDLC history or analytics store. Label history is reconstructed live
  from the GitHub timeline; a retained store would be a new api-server plugin and
  migration, out of scope for this BFF-only tab.
- Implementing the Jira adapter. This spec defines the `SdlcStateProvider` seam and
  ships only the GitHub adapter.
- Deriving SLE values from historical percentiles. SLEs are per-phase configuration
  with sensible defaults; percentile derivation depends on a retained history store
  and is a future enhancement.
- Push / websocket updates. "Near real time" is bounded polling against the cached
  plane; no push infrastructure exists.
- Changing the funnel label state machine, the promotion mechanics, or the
  bundle-resolution pipeline themselves.
- Moving bundle membership behind the state provider port; bundle membership stays
  the shared GitHub bundle concern whichever state adapter is active.
