# SDLC Agent Runtime Telemetry

**ID prefix:** SDLC-AGT

**Applies to:** `components/fleet-dashboard` BFF (new `/api/sdlc-agents` plane, Prometheus source), `packages/fleet-dashboard-ui` (board integration, per-cluster agents tab, sparkline cap, health verdict removal, agent activity states), the agent-infrastructure producer in hypershell-gitops (the `sdlc_agent_last_report_timestamp_seconds` freshness fact and its metric lifespan)

## Purpose

Make per-agent runtime telemetry observable in the fleet dashboard. The
existing Agent SDLC funnel board (`/api/sdlc` plane,
`agent-sdlc-visibility-dashboard.spec.md`) shows where work sits in the
pipeline and how long it has been there; this specification adds the
complementary signal: how the autonomous agents that act on each phase are
performing at runtime (replicas running, queue depth, items processed and
failed, provision duration), each with a 24-hour history.

The telemetry is projected from Prometheus series already emitted by the
agent infrastructure and served through a new `/api/sdlc-agents` BFF plane.
The frontend consumes this plane through the hexagonal boundary: a
framework-free domain type, an application port method, a wire mapper, and
an HTTP adapter (SDLC-AGT-06).

The frontend surfaces this telemetry in three places: a compact runtime strip
on each Agent SDLC board column (replicas, queue, failures), an agent runtime
detail panel opened from the board, and a per-cluster Agents tab on the
promotion topology drawer. The runtime signal is integrated directly into the
Agent SDLC board so both work-in-progress and runtime health are visible in
one view.

### Agent liveness: Active, Not active, Monitoring blind

The SDLC agents are batch CronJobs: they push `sdlc_*` series in a short burst
while they run, then exit. Between runs the series expire and vanish, so the
absence of a metric conflates three distinct situations: an agent finished and
is idle (fine), an agent crashed or is not running when it should be (not
fine), and the BFF cannot reach Prometheus at all (the whole board is
untrustworthy). A blank column communicates none of them.

This specification makes agent liveness a first-class, present-tense signal
derived from facts rather than inferred from the absence of a metric. There is
no notion of an expected schedule or an "overdue" state: the only question
asked is "is this agent active right now, or not." The signal has exactly three
states -- **Active**, **Not active**, and **Monitoring blind** -- and it is
distinct from the removed `agentHealth()` verdict (SDLC-AGT-10): activity is
derived from an absolute freshness fact and the plane's own staleness, not from
product-guess thresholds, so it is not a disguised SLO.

History is retained alongside the liveness signal. The producer emits an
absolute freshness timestamp per agent, and the collector's metric lifespan is
extended so the last-pushed series persists between runs; the 24-hour
sparklines continue to populate, and once an agent stops reporting its series
eventually decays. That liveness decay is accepted and communicated plainly (a
"Not active" state plus a "last seen" timestamp) rather than silently blanked.

### Relationship to existing dashboard planes

| Concern | Agent SDLC funnel (SDLC-VIS) | Agent runtime telemetry (this spec) |
| --- | --- | --- |
| BFF plane | `/api/sdlc` | `/api/sdlc-agents` |
| Source | GitHub labels/timeline | Prometheus (`sdlc_agent*` series) |
| Signal | Work-in-progress, aging, rework | Replicas, queue, throughput, failures |
| Domain join | Phase key | `PHASE_AGENT` map (phase to agent name) |
| Port method | `FleetApi.getSdlc` | `FleetApi.getSdlcAgents` |

The join between the two domains is a static map: each funnel phase is acted
on by exactly one runtime agent. The board renders the phase and its agent as
one unit of attention; the runtime detail opens from the agent name.

## Requirements

### Requirement: SDLC-AGT-01 -- BFF SDLC Agents Plane

The fleet-dashboard BFF SHALL expose a read-only plane `GET /api/sdlc-agents`
that serves per-agent runtime telemetry projected from Prometheus. The plane
SHALL be served through the existing `pkg/cache.Source` envelope (`Snapshot`
with `Data`, `GeneratedAt`, `Stale`, `Error`) and the existing `serve`
handler wrapper, so a never-refreshed source returns HTTP 503, a
stale-but-serving source returns HTTP 200 with the `stale` flag set, and a
healthy source returns HTTP 200. The plane SHALL sit behind the same
authenticated `/api/` middleware and SAR authorization firewall as the
existing planes.

Prometheus is required configuration (`FD_PROM_URL` is mandatory), so this
plane SHALL mount unconditionally, like `/api/fleet`, not nil-gated like
`/api/sdlc`.

The plane SHALL be registered and run as its own refresh goroutine alongside
the existing sources, with its own configurable refresh interval
(`FD_REFRESH_SDLC_AGENTS`) and the shared metrics observer.

#### Scenario: SDLC-AGT-01.01 -- Healthy SDLC agents snapshot is served

- GIVEN the SDLC agents source has refreshed at least once
- WHEN a client requests `GET /api/sdlc-agents`
- THEN the BFF SHALL respond with HTTP 200 and the snapshot envelope

#### Scenario: SDLC-AGT-01.02 -- SDLC agents source not yet refreshed

- GIVEN the SDLC agents source has never completed a refresh
- WHEN a client requests `GET /api/sdlc-agents`
- THEN the BFF SHALL respond with HTTP 503

#### Scenario: SDLC-AGT-01.03 -- SDLC agents source stale but serving

- GIVEN the SDLC agents source last refresh failed but a prior snapshot exists
- WHEN a client requests `GET /api/sdlc-agents`
- THEN the BFF SHALL respond with HTTP 200
- AND the envelope SHALL carry `stale: true` and the last `error`

### Requirement: SDLC-AGT-02 -- Wire Contract

The `/api/sdlc-agents` plane payload (the `Snapshot.Data` value) SHALL
conform to the wire contract already pinned by the frontend domain model
(`domain/sdlc-agents.ts`) and wire mapper (`adapters/api/wire.ts`). The Go
structs SHALL use camelCase JSON tags matching the existing fleet DTO
convention.

The payload SHALL contain:

- `envs` -- sorted distinct environment keys (the join key to the promotion
  map's instance identifiers)
- `agents` -- one row per (agent name, environment) pair, each carrying:
  - `name` -- agent name (product vocabulary, not fleet identity)
  - `env` -- environment key
  - `replicasOn` -- count of running replicas
  - `replicasTotal` -- total replica count (running plus not-running)
  - `queueDepth` -- pending work items
  - `itemsProcessed` -- items completed (counter)
  - `itemsFailed` -- items failed (counter)
  - `provisionSeconds` -- latest gateway provision duration
  - `lastReportedAt` -- unix seconds of the agent's most recent freshness
    push (SDLC-AGT-15), `0` when the freshness series is absent
  - `series` -- 24-hour history, index-aligned to a shared time axis:
    - `times` -- unix seconds, oldest first
    - `queueDepth`, `itemsProcessed`, `itemsFailed`, `provisionSeconds` --
      each an array index-aligned to `times`

Slices SHALL be initialised non-nil so an empty plane serialises as `[]`,
not `null`. Every numeric sample SHALL pass through the source layer's
`sampleValue` function to coerce NaN and Inf to 0 before reaching
`json.Marshal`.

#### Scenario: SDLC-AGT-02.01 -- Payload matches the existing wire mapper

- GIVEN the frontend wire mapper (`WireSdlcAgents` / `WireSdlcAgentMetrics` /
  `WireSdlcAgentSeries`)
- WHEN the BFF serialises the plane payload
- THEN every field name and nesting level SHALL match the mapper's expected
  shape

#### Scenario: SDLC-AGT-02.02 -- Empty plane serialises with empty arrays

- GIVEN no Prometheus series are reporting
- WHEN the source builds the payload
- THEN `envs` SHALL be `[]` and `agents` SHALL be `[]`
- AND neither SHALL be `null`

#### Scenario: SDLC-AGT-02.03 -- Non-finite samples are coerced to zero

- GIVEN a Prometheus instant query returns a NaN or Inf sample value
- WHEN the source builds the payload
- THEN that sample SHALL be 0 in the payload

### Requirement: SDLC-AGT-03 -- Prometheus Source Layer

The telemetry SHALL be projected from Prometheus series emitted by the agent
infrastructure on the infra cluster:

| Series | Labels | Purpose |
| --- | --- | --- |
| `sdlc_agent` | `name`, `env`, `replica` | On/off gauge (1 = running) |
| `sdlc_agent_queue_depth` | `name`, `env` | Pending work items |
| `sdlc_items_processed` | `name`, `env` | Items completed |
| `sdlc_items_failed` | `name`, `env` | Items failed |
| `sdlc_gateway_provision_seconds` | `name`, `env` | Provision duration |
| `sdlc_agent_last_report_timestamp_seconds` | `name`, `env` | Freshness fact: unix seconds of the last push (SDLC-AGT-15) |

The source SHALL reuse the existing Prometheus helpers and idioms from
`Prometheus.Fleet`:

- **Instant queries** for current values: `replicasOn` from
  `sum by (name, env) (sdlc_agent)`, `replicasTotal` from
  `count by (name, env) (sdlc_agent)`, and `sum by (name, env)` for queue
  depth, items processed, and items failed. Provision seconds is a per-replica
  duration gauge, so it SHALL aggregate with `max by (name, env)
  (sdlc_gateway_provision_seconds)` -- one representative duration per
  `(name, env)` rather than a total inflated by replica count. The freshness
  fact is a per-replica timestamp gauge, so it SHALL aggregate with
  `max by (name, env) (sdlc_agent_last_report_timestamp_seconds)` -- the most
  recent push per `(name, env)` (SDLC-AGT-15).
- **Range queries** for each `series.*` field, grouped by `(name, env)`.
- **Best-effort subqueries**: the `subErrs` pattern from `Prometheus.Fleet`
  SHALL be reused -- collect per-query errors, warn once, and still return a
  usable snapshot rather than failing the whole plane because one series is
  missing.
- **NaN-safe**: every sample SHALL pass through `sampleValue`.

The source SHALL emit one `SdlcAgentMetrics` row per `(name, env)` pair, and
`Envs` SHALL be the sorted distinct `env` set. The `env` key is the join key
the promotion map uses to attribute agents to a cluster node, so it SHALL
equal the promotion plane's instance keys.

#### Scenario: SDLC-AGT-03.01 -- Per-agent per-env grouping

- GIVEN Prometheus reports `sdlc_agent{name="spec-review", env="integration"}`
  and `sdlc_agent{name="spec-review", env="production"}`
- WHEN the source builds the payload
- THEN the agents array SHALL contain two rows, one for each (name, env) pair

#### Scenario: SDLC-AGT-03.02 -- ReplicasOn vs ReplicasTotal

- GIVEN Prometheus reports three `sdlc_agent` samples for
  `(name="triage", env="integration")` with values 1, 1, and 0
- WHEN the source computes the row
- THEN `replicasOn` SHALL be 2 (sum of the gauge)
- AND `replicasTotal` SHALL be 3 (count of the gauge)

#### Scenario: SDLC-AGT-03.03 -- Partial Prometheus failure degrades gracefully

- GIVEN one instant subquery fails but others succeed
- WHEN the source builds the payload
- THEN the source SHALL return a usable snapshot with the failing metric
  zero-filled
- AND it SHALL log a warning with the failed subquery count

### Requirement: SDLC-AGT-04 -- Shared Time Axis and History Window

The range queries SHALL follow the `Fleet` precedent: a single 24-hour
history window with 32 samples, producing one sorted `times` axis per agent
that every per-agent series array is index-aligned to. This is the
`SdlcAgentSeries.times` plus aligned-arrays contract the frontend's detail
panel scrubber uses.

A timestamp present in any series for an agent SHALL become a column in all
of its series arrays (missing values zero-filled), so the scrubber can read
every metric at the hovered sample.

#### Scenario: SDLC-AGT-04.01 -- History uses a 24h/32-sample grid

- GIVEN the source runs a range query
- WHEN it computes the history window
- THEN the window SHALL span the last 24 hours with 32 samples

#### Scenario: SDLC-AGT-04.02 -- Series arrays are index-aligned to times

- GIVEN an agent has queue-depth history but no items-failed history at a
  given timestamp
- WHEN the source aligns the arrays
- THEN the items-failed value at that index SHALL be 0
- AND both arrays SHALL have the same length as times

### Requirement: SDLC-AGT-05 -- Configuration

The refresh interval SHALL be configurable through `FD_REFRESH_SDLC_AGENTS`,
with a default of 30 seconds. The Prometheus metric names (`sdlc_agent`,
`sdlc_agent_queue_depth`, `sdlc_items_processed`, `sdlc_items_failed`,
`sdlc_gateway_provision_seconds`) are public product terms and are safe as
compiled-in defaults. No fleet-identifying value SHALL be compiled in.

#### Scenario: SDLC-AGT-05.01 -- Refresh interval overridden

- GIVEN `FD_REFRESH_SDLC_AGENTS=45s` is set
- WHEN the BFF starts
- THEN the SDLC agents refresh loop SHALL use a 45-second interval

#### Scenario: SDLC-AGT-05.02 -- Default refresh interval

- GIVEN `FD_REFRESH_SDLC_AGENTS` is not set
- WHEN the BFF starts
- THEN the SDLC agents refresh loop SHALL use a 30-second interval

### Requirement: SDLC-AGT-06 -- Hexagonal UI Boundary

The frontend SHALL follow the narrow hexagonal UI boundary
(`specs/standards/ui/hexagonal-architecture.spec.md`). The addition SHALL
comprise: a framework-free SDLC agents domain type (`SdlcAgentsData`,
`SdlcAgentMetrics` -- including the optional `lastReportedAt` freshness field
(SDLC-AGT-15) -- `SdlcAgentSeries`), a `FleetApi.getSdlcAgents(signal?)`
port method returning the `Plane<SdlcAgentsData>` freshness envelope, a wire
mapper in the anti-corruption layer (`adapters/api/wire.ts`) that is the only
place that knows the `/api/sdlc-agents` wire shape, and a TanStack Query
hook with its own `refetchInterval`. React, TanStack Query, the HTTP adapter,
and the generated wire shapes SHALL stay outside the port. The activity
derivation (SDLC-AGT-16) SHALL live in the domain and SHALL NOT read the clock
or any framework API; the caller SHALL pass in the current time and the plane's
staleness.

#### Scenario: SDLC-AGT-06.01 -- Data flows through the port and mapper

- GIVEN the UI loads SDLC agents data
- WHEN it fetches from `/api/sdlc-agents`
- THEN it SHALL call `getSdlcAgents` through the application port
- AND the BFF JSON SHALL be translated to the domain type only in the wire
  mapper

### Requirement: SDLC-AGT-07 -- Board Agent Integration

The dashboard SHALL NOT present runtime telemetry as a standalone tab. The
`TabKey` union SHALL remain `"map" | "sdlc" | "e2e"`. Runtime telemetry
SHALL be presented inline on the Agent SDLC board, joining work-in-progress
and runtime signal in one view.

The board column headers SHALL lead with the agent name (bold, with
`RobotIcon`) and drop the phase label to a subtle annotation below. The
agent name SHALL be a `<button>` only when there is runtime telemetry to
open; otherwise it SHALL be plain text, so there is never a focusable dead
control.

#### Scenario: SDLC-AGT-07.01 -- No standalone agents tab

- GIVEN the dashboard tab bar
- WHEN it renders
- THEN there SHALL be no standalone "Agents" tab
- AND the tabs SHALL be Map, Agent SDLC, and E2E

#### Scenario: SDLC-AGT-07.02 -- Column header shows agent name and phase

- GIVEN a board column for the "Spec" phase (acting agent: "spec-review")
- WHEN it renders
- THEN the column header SHALL lead with "spec-review" (bold, with RobotIcon)
- AND the phase name "Spec" SHALL appear as a subtle annotation

#### Scenario: SDLC-AGT-07.03 -- Agent name is not a button without telemetry

- GIVEN a board column whose acting agent has no runtime telemetry
- WHEN it renders
- THEN the agent name SHALL be plain text, not a focusable button

### Requirement: SDLC-AGT-08 -- Column Agent Runtime Strip

Each board column that has a matching agent summary SHALL display a compact
runtime metrics strip below the column header. The strip SHALL show three
values: replicas on/total, queue depth, and failure count.

Color SHALL follow the existing fleet dashboard color discipline:

- Replicas: success green when any are on, muted when zero
- Queue: purple at normal depth, warning amber at or above the queue warning
  threshold, muted when zero
- Failures: danger red when non-zero, muted when zero

Color SHALL NOT be the only channel: each value SHALL be paired with a short
lowercase label (`on`, `queue`, `fail`).

Clicking the agent name SHALL open a resizable runtime detail panel. The
selected agent SHALL be local ephemeral state; a route-driven panel (card
detail, shipped detail, notifications) SHALL supersede it. Escape SHALL close
the agent panel first, then route-driven panels.

#### Scenario: SDLC-AGT-08.01 -- Strip shows replicas, queue, and failures

- GIVEN a column whose agent has 2 replicas on, 3 total, queue depth 5, and
  1 failure
- WHEN the strip renders
- THEN it SHALL show "on 2/3", "queue 5", and "fail 1"

#### Scenario: SDLC-AGT-08.02 -- Zero values are muted

- GIVEN a column whose agent has 0 queue depth and 0 failures
- WHEN the strip renders
- THEN queue and failure values SHALL render in the muted grey, not in their
  attention colors

#### Scenario: SDLC-AGT-08.03 -- Agent detail panel opens from the agent name

- GIVEN a column with runtime telemetry
- WHEN the operator clicks the agent name
- THEN a resizable agent runtime detail panel SHALL open

#### Scenario: SDLC-AGT-08.04 -- Route-driven panel supersedes agent panel

- GIVEN the agent detail panel is open
- WHEN a card detail, shipped detail, or notifications panel opens
- THEN the agent panel SHALL close

### Requirement: SDLC-AGT-09 -- Sparkline Tile Height Cap

The shared `AgentMetricTiles` grid tiles SHALL have a `max-height` of 180px.
At default panel widths (tiles approximately 117-164px), the cap SHALL not
take effect. When the panel is dragged wider, tile height SHALL stop growing
at 180px so the sparklines remain glanceable.

#### Scenario: SDLC-AGT-09.01 -- Tile height capped at 180px

- GIVEN the agent detail panel or map node panel is dragged to a wide size
- WHEN the metric tiles render
- THEN each tile's height SHALL not exceed 180px

#### Scenario: SDLC-AGT-09.02 -- Default width unaffected

- GIVEN the panel is at its default width
- WHEN the metric tiles render
- THEN the tiles SHALL size by their aspect ratio, unaffected by the cap

### Requirement: SDLC-AGT-10 -- Health Verdict Removal

The derived health indicator (`agentHealth()` returning
`critical | warning | idle | ok`) SHALL NOT be rendered in any production UI
surface. The board column header SHALL NOT render a `HealthDot`. The agent
detail panel head SHALL NOT render a health chip.

The raw counts (replicas, queue, failures) are the honest signal and are
already shown; a derived verdict built on product-guess thresholds reads as
a measured SLO but is not one. The `agentHealth` function and `HealthDot`
component MAY remain exported but SHALL NOT be rendered.

#### Scenario: SDLC-AGT-10.01 -- No health dot on the board

- GIVEN the Agent SDLC board renders
- WHEN a column header for an agent with telemetry renders
- THEN no HealthDot or health chip SHALL appear

#### Scenario: SDLC-AGT-10.02 -- No health chip in the detail panel

- GIVEN the agent runtime detail panel opens
- WHEN the panel head renders
- THEN no health chip or verdict label SHALL appear

### Requirement: SDLC-AGT-11 -- Per-Cluster Agents Tab

The promotion topology drawer SHALL include an "Agents" tab after "Bundle"
and before "Links". The tab SHALL display a compact table of SDLC agents
reporting on the selected cluster node, with columns: Agent (with
`RobotIcon`), Replicas, Queue, Done, and Failed.

The `NodeTabKey` union SHALL include `"agents"`. The agent rows for a cluster
SHALL be selected by matching the agent's `env` key to the map node's
instance identifier. Each row SHALL carry the agent's activity cell
(SDLC-AGT-17) alongside its replica, queue, done, and failed values.

The empty state and the idle state SHALL be distinct. A cluster whose agents
have never reported (no rows) SHALL show the empty-state message. A cluster
whose agents have reported but are idle now SHALL show rows marked **Not
active** rather than the empty state, which is the honest distinction this
specification exists to make.

#### Scenario: SDLC-AGT-11.01 -- Agents tab shows per-cluster agents

- GIVEN a cluster node "integration" with agents "triage" and "spec-review"
  reporting in that environment
- WHEN the operator opens the Agents tab
- THEN it SHALL show one row per agent with replicas, queue, done, and failed
  values

#### Scenario: SDLC-AGT-11.02 -- No agents on a cluster

- GIVEN a cluster node "production-edge" with no agents reporting
- WHEN the operator opens the Agents tab
- THEN the tab SHALL show an empty-state message

#### Scenario: SDLC-AGT-11.03 -- Agents tab uses color discipline

- GIVEN an agent row with 0 failures and 3 queue depth
- WHEN the row renders
- THEN the failure value SHALL be muted grey, the queue SHALL be purple,
  and each value SHALL use tabular-nums

### Requirement: SDLC-AGT-12 -- Resizable Drawer Panels

All board drawer panels (agent detail, card detail, shipped detail,
notifications) SHALL be resizable using the PatternFly `DrawerPanelContent`
`isResizable` prop, with a default size of 33% and a minimum of 360px. This
establishes parity across every panel the board opens.

#### Scenario: SDLC-AGT-12.01 -- All panels are resizable

- GIVEN any board drawer panel is open
- WHEN the operator drags the resize handle
- THEN the panel width SHALL resize

### Requirement: SDLC-AGT-13 -- Accessibility and Localization

The agent name button SHALL carry an `aria-label` describing its action. All
new user-visible strings (tab titles, table column headers, empty states,
aria labels) SHALL be localized through `react-intl`. The compact metric
labels (`on`, `queue`, `fail`) are glyph-like and paired with their numeric
value; they need not be localized. The per-cluster agents table SHALL carry
`dataLabel` on every cell for the responsive stacked layout and use
tabular-nums for aligned figures. Color SHALL NOT be the sole channel for any
signal.

#### Scenario: SDLC-AGT-13.01 -- Agent button has an accessible name

- GIVEN a board column with runtime telemetry
- WHEN the agent name button renders
- THEN it SHALL carry an `aria-label` describing its action

#### Scenario: SDLC-AGT-13.02 -- New strings are localized

- GIVEN the per-cluster Agents tab renders
- WHEN user-visible strings appear (tab title, column headers, empty state)
- THEN each SHALL come from the `react-intl` message catalog

### Requirement: SDLC-AGT-14 -- Tests

The repository SHALL include:

- **Source tests** (`pkg/sources/sdlc_agents_test.go`): an `httptest.Server`
  returning canned Prometheus JSON, asserting env grouping,
  `replicasOn` vs `replicasTotal`, index-aligned `series` arrays, NaN-safe
  coercion, and `lastReportedAt` mapped per `(name, env)` from
  `sdlc_agent_last_report_timestamp_seconds` and `0` when that series is absent
  (SDLC-AGT-15).
- **Handler tests** (`pkg/api/handlers_test.go`): a cold plane returns 503,
  a NaN payload yields 500 with a JSON error body, following the existing
  `TestServeSDLCColdIs503` and `TestServeEncodeErrorIsVisible` patterns.
- **Domain tests** (`domain/sdlc-agents.test.ts`): table-driven `agentActivity`
  across every branch (SDLC-AGT-16) -- plane stale maps to `blind`;
  `replicasOn > 0` with a fresh timestamp maps to `active`; `replicasOn > 0`
  with a stale timestamp maps to `notActive` (the decayed-crash case);
  `replicasOn = 0` maps to `notActive`; and no timestamp with `replicasOn > 0`
  maps to `active` (the raw-gauge fallback).
- **Frontend tests**: wire mapper round-trip including `lastReportedAt`, board
  column header rendering of each activity state with its label and of the "last
  seen" text when a timestamp is present (SDLC-AGT-17), per-cluster agents table
  with and without rows and its not-active versus never-reported distinction,
  sparkline tile height cap.

#### Scenario: SDLC-AGT-14.01 -- Source test covers env grouping

- GIVEN canned Prometheus responses for two agents across two environments
- WHEN the source builds the payload
- THEN the test SHALL assert four (name, env) rows and two sorted envs

#### Scenario: SDLC-AGT-14.02 -- Handler cold-start returns 503

- GIVEN a never-refreshed SDLC agents source
- WHEN a handler test requests `GET /api/sdlc-agents`
- THEN the response SHALL be HTTP 503

### Requirement: SDLC-AGT-15 -- Agent Freshness Fact

The agent-infrastructure producer SHALL emit an absolute freshness timestamp
per agent, `sdlc_agent_last_report_timestamp_seconds{name, env}`, set to the
unix seconds of each push, as the source of truth for whether an agent's on/off
gauge is still live. The producer SHALL push the on/off gauge `0` on normal
exit (so a finished agent reads "not active" immediately) and SHALL stop
refreshing the freshness timestamp when it crashes (so a crashed agent's stale
gauge decays once the collector metric lifespan elapses). The collector metric
lifespan on the infra cluster SHALL be extended so each agent's last-pushed
series persists between runs, retaining the 24-hour history (SDLC-AGT-04) and
the last on/off value rather than vanishing between runs. The producer and the
collector lifespan change live in hypershell-gitops; this specification owns the
fact's meaning and the dashboard's use of it.

The BFF SHALL query the freshness fact with an instant query
`max by (name, env) (sdlc_agent_last_report_timestamp_seconds)` and map it onto
each `(name, env)` row as `LastReportedAt` (unix seconds), reusing the existing
`runInstant` helper and `sampleValue` coercion. `LastReportedAt` SHALL be
initialised to `0` when the series is absent. The BFF SHALL report this absolute
fact only; it SHALL NOT compute any staleness or activity boolean, because the
plane is cached and a server-computed verdict would itself go stale inside the
snapshot. No new configuration SHALL be introduced for this fact.

The freshness fact introduces no notion of a schedule or an expected interval;
it answers only whether the on/off gauge is still live, not whether the agent
should have run by now.

#### Scenario: SDLC-AGT-15.01 -- Freshness fact maps to LastReportedAt

- GIVEN Prometheus reports
  `sdlc_agent_last_report_timestamp_seconds{name="triage", env="integration"}`
- WHEN the source builds the payload
- THEN that `(name, env)` row's `lastReportedAt` SHALL equal the pushed unix
  seconds

#### Scenario: SDLC-AGT-15.02 -- Absent freshness series defaults to zero

- GIVEN no `sdlc_agent_last_report_timestamp_seconds` series exists for an agent
- WHEN the source builds that agent's row
- THEN its `lastReportedAt` SHALL be `0`

#### Scenario: SDLC-AGT-15.03 -- BFF reports the fact without judging it

- GIVEN the SDLC agents source builds a row
- WHEN it populates the freshness field
- THEN it SHALL set only the absolute `lastReportedAt` timestamp
- AND it SHALL NOT compute any staleness or activity boolean on the server

### Requirement: SDLC-AGT-16 -- Client Activity Derivation

The frontend SHALL derive an agent's present-tense activity in the domain as a
pure function of the agent row, a caller-supplied current time, and the plane's
`stale` flag, yielding exactly one of three states:
`"active" | "notActive" | "blind"`. The derivation SHALL NOT read the clock or
any framework API itself (SDLC-AGT-06).

The derivation SHALL be:

- **blind** when the plane is stale, because the whole snapshot is then
  untrustworthy and no per-agent liveness can be asserted.
- Otherwise the on/off gauge SHALL be treated as **live** when
  `lastReportedAt > 0` and the current time is within a single generic liveness
  window (`HEARTBEAT_STALE`, for example 180 seconds) of `lastReportedAt`; when
  `lastReportedAt` is `0` (no freshness fact yet), liveness SHALL fall back to
  the raw `replicasOn > 0` gauge.
- **active** when any replica is on AND the gauge is live; otherwise
  **notActive**.

`HEARTBEAT_STALE` is a single generic liveness window, NOT a per-agent schedule;
it only answers whether the on/off gauge is still live. The activity derivation
is a fact-based signal distinct from the removed `agentHealth()` verdict
(SDLC-AGT-10) and SHALL NOT reintroduce `agentHealth()`, `HealthDot`, or
`HEALTH_COLOR`.

#### Scenario: SDLC-AGT-16.01 -- Stale plane is blind

- GIVEN the plane's `stale` flag is true
- WHEN the activity of any agent is derived
- THEN the result SHALL be `blind`

#### Scenario: SDLC-AGT-16.02 -- Running agent with a fresh timestamp is active

- GIVEN an agent with `replicasOn > 0` whose `lastReportedAt` is within the
  liveness window of the current time, and a non-stale plane
- WHEN its activity is derived
- THEN the result SHALL be `active`

#### Scenario: SDLC-AGT-16.03 -- Running agent with a stale timestamp is not active

- GIVEN an agent with `replicasOn > 0` whose `lastReportedAt` is older than the
  liveness window (the decayed-crash case), and a non-stale plane
- WHEN its activity is derived
- THEN the result SHALL be `notActive`

#### Scenario: SDLC-AGT-16.04 -- No running replicas is not active

- GIVEN an agent with `replicasOn = 0` and a non-stale plane
- WHEN its activity is derived
- THEN the result SHALL be `notActive`

#### Scenario: SDLC-AGT-16.05 -- Fallback to the raw gauge without a timestamp

- GIVEN an agent with `replicasOn > 0` and `lastReportedAt = 0`, and a non-stale
  plane
- WHEN its activity is derived
- THEN the result SHALL be `active`

### Requirement: SDLC-AGT-17 -- Activity Communication on the Board and Agents Tab

The Agent SDLC board column header and the per-cluster Agents table
(SDLC-AGT-11) SHALL communicate each agent's activity state (SDLC-AGT-16)
explicitly rather than leaving a blank. Each of the three states SHALL render a
non-color cue paired with a text label: `active` as an affirmative dot with
"Active", `notActive` as a muted dot with "Not active", and `blind` as a warning
dot with "Monitoring blind". Color SHALL NOT be the only channel; the label is
always present (SDLC-AGT-13).

Because "Not active" is the normal resting state of a cron agent, it SHALL read
as neutral, not as an alarm; the alarm-worthy states are a real last-run failure
(the existing failure count) and "Monitoring blind".

When `lastReportedAt > 0`, the surface SHALL append a localized relative "last
seen {ago} ago" so the accepted liveness decay (Purpose) is communicated rather
than implied. The sparkline and history row SHALL render in every activity
state. The agent name SHALL remain a button whenever there is an agent summary
to open (SDLC-AGT-07), so the history is reachable even when the agent is not
active.

#### Scenario: SDLC-AGT-17.01 -- Three activity states render with a label

- GIVEN board columns for an active agent, a not-active agent, and (plane stale)
  a blind agent
- WHEN the headers render
- THEN they SHALL show "Active", "Not active", and "Monitoring blind"
  respectively, each paired with a non-color dot cue

#### Scenario: SDLC-AGT-17.02 -- Last seen is shown when a timestamp exists

- GIVEN a not-active agent whose `lastReportedAt` is greater than `0`
- WHEN its column header renders
- THEN it SHALL show a localized "last seen {ago} ago" relative timestamp

#### Scenario: SDLC-AGT-17.03 -- History reachable when not active

- GIVEN a not-active agent that has an agent summary
- WHEN its column header renders
- THEN the agent name SHALL remain a button that opens the runtime detail
- AND the sparkline history SHALL still render

#### Scenario: SDLC-AGT-17.04 -- Idle cluster rows show Not active, not empty state

- GIVEN a cluster whose agents have reported but are idle now
- WHEN the per-cluster Agents tab renders
- THEN it SHALL show rows marked "Not active"
- AND it SHALL NOT show the never-reported empty state (SDLC-AGT-11)

## Non-Goals

- **Alerting or derived health verdicts.** The plane serves raw telemetry and
  the absolute freshness fact (SDLC-AGT-15); thresholds and alerting are out of
  scope. The present-tense activity signal (SDLC-AGT-16) is derived from facts,
  not a threshold-guessed verdict, and the removed `agentHealth` function is not
  rendered (SDLC-AGT-10).
- **An expected schedule or "overdue" state.** Liveness answers only "active or
  not active" in the present tense; no notion of when an agent should have run is
  introduced (SDLC-AGT-15, SDLC-AGT-16).
- **Push or websocket updates.** The plane uses the same bounded-polling
  model as all other fleet dashboard planes.
- **Per-agent drill-down to individual replicas.** The plane aggregates by
  `(name, env)`; replica-level detail is out of scope.
