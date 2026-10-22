# E2E Run and Log Visibility

**Status:** Draft
**Applies to:** `components/fleet-dashboard` BFF (new e2e-run plane, Loki source, live stream), `packages/fleet-dashboard-ui` (e2e-run view, domain and presentation)
**Tracks:** [HYPERSHELL-402](https://redhat.atlassian.net/browse/HYPERSHELL-402), [HYPERSHELL-448](https://redhat.atlassian.net/browse/HYPERSHELL-448) (paginated + live-streamed raw logs)

## Purpose

Make the release end-to-end test runs observable in the fleet dashboard (the
"glass" dashboard, `glass.infra.hypershell.app`), both live while a run executes
and as retained history after the run's Pod is gone. The release delivery path
runs the current Go E2E suite (`tests/e2e`, `TestE2E`) as an Argo Rollouts
AnalysisRun metric Job on each workload hub. Today the dashboard shows only the
promotion gate's pass/fail outcome and deep links to Argo and the GitHub check; it
cannot show which steps ran, where a run is now, why a run failed, or the streamed
command output, and once the Job's Pod is reaped there is no retained detail at all.

This specification adds an **E2E run view** reachable from the promotion gate /
environment, backed by a new read-only BFF plane. Initial recent-run history is
read from a central Loki receiver with bounded queries, and live steps and log
lines are streamed to the browser by following Loki's tail API and bridging it to
the snapshot without losing events. The suite side -- the opt-in structured NDJSON
reporting the dashboard consumes -- is specified in `e2e-testing.spec.md`
([Opt-In Structured Run Reporting](e2e-testing.spec.md#requirement-opt-in-structured-run-reporting)).
The log pipeline (the Loki receiver, retention, and the ClusterLogForwarder that
copies the suite's Pod logs) is owned by `openshift-online/hypershell-gitops`; this
spec consumes that pipeline and SHALL NOT restate or own it. Making this view's
selected run, step, log scope, and bundle/target context reconstructible from the
URL is specified separately in
[`glass-url-addressable-navigation.spec.md`](glass-url-addressable-navigation.spec.md).

### Relationship to the existing fleet dashboard

| Concern | Promotion gate (existing) | E2E run view (this spec) |
| --- | --- | --- |
| BFF plane | `/api/promotion` | new `/api/e2e/runs`, `/api/e2e/runs/{id}`, `/api/e2e/logs?run=<id>`, `/api/e2e/stream` |
| Source | `pkg/sources/promotion.go` (Kubernetes dynamic client) | new Loki source (`query_range` history + `tail` live) |
| Transport | cached JSON snapshot + client poll | snapshot + server-streamed live updates (SSE / authenticated streaming fetch) |
| UI | gate badge + Argo/analysis deep links | recent runs, ordered steps, selectable inline log pane, live/disconnected/history-truncated states |
| Port method | `FleetApi.getPromotion` | `FleetApi.getE2eRuns` / `getE2eRun` / `getE2eRunLogs` / `streamE2eRun` |

### Cross-repo contract (consumed, not owned)

| Fact | Owner | Notes |
| --- | --- | --- |
| Structured NDJSON event schema (`hypershell.e2e.v1`), run/step/log/terminal events, `runId`/`revision`/`sourceRevision`/`cluster`/`namespace`/`sequence` | this repo (`e2e-testing.spec.md`) | The suite emits it to stdout; the collector copies it |
| Central Loki receiver, seven-day retention, persistent storage; only Pods labeled `delivery.hypershell.app/check` in {`e2e`, `release`, `postsync`} copied | hypershell-gitops | Existing per-hub Loki unchanged |
| ClusterLogForwarder over fleet mTLS ingress; `X-Scope-OrgID` overridden by an allowlisted client-cert identity | hypershell-gitops | Browser input never selects an arbitrary tenant or URL |
| ViaQ JSON envelope: `message` holds the suite's emitted JSON line; Loki labels `log_type`, `kubernetes_namespace_name`, `kubernetes_container_name`, `kubernetes_pod_name`, collector-added `kubernetes_host` | hypershell-gitops / ViaQ | Run ids, commit SHAs, and step ids live in the JSON body, not index labels |

## Requirements

### Requirement: E2E-LOG-01 -- BFF E2E Run Planes

The fleet-dashboard BFF SHALL expose read-only e2e-run endpoints: recent runs
(`GET /api/e2e/runs`), a single run's detail with its ordered steps and verdict
(`GET /api/e2e/runs/{id}`), a paginated raw-log history for one run
(`GET /api/e2e/logs?run=<id>`, see E2E-LOG-19), and a live stream
(`GET /api/e2e/stream`). The run-detail endpoint SHALL serve steps and verdict
from the small lifecycle query and SHALL NOT embed the full raw-log history (see
E2E-LOG-20); the raw-log history SHALL be read a bounded page at a time through
the logs endpoint. The exact
route shapes MAY be adapted to the deployed identity, but a run SHALL be addressed
by an identifier that includes its tenant/namespace so run ids cannot collide across
clusters. The snapshot planes SHALL use the existing `pkg/cache.Source` freshness
envelope (`Snapshot` with `Data`, `GeneratedAt`, `Stale`, `Error`) and the existing
handler wrapper, so a never-refreshed source returns HTTP `503`, a stale-but-serving
source returns HTTP `200` with the `stale` flag set, and a healthy source returns
HTTP `200`. All e2e-run endpoints SHALL sit behind the same authenticated `/api/`
middleware and SAR authorization firewall as the existing planes; they SHALL NOT
introduce a new auth path.

The feature SHALL be disabled when no Loki endpoint is configured (see
E2E-LOG-02): with it disabled, the endpoints SHALL report the feature off rather
than error, and the rest of the dashboard SHALL be unaffected.

#### Scenario: E2E-LOG-01.01 -- Recent runs served from history

- GIVEN the Loki endpoint is configured and the source has refreshed at least once
- WHEN a client requests `GET /api/e2e/runs`
- THEN the BFF SHALL respond with HTTP `200` and the recent-runs snapshot envelope

#### Scenario: E2E-LOG-01.02 -- Run detail addressed without cross-cluster collision

- GIVEN two runs on different clusters that share the suite-emitted `runId`
- WHEN a client requests each run's detail
- THEN each request SHALL resolve to the correct run because the run address includes its tenant/namespace

#### Scenario: E2E-LOG-01.03 -- Feature disabled when Loki is unconfigured

- GIVEN no Loki endpoint is configured
- WHEN a client requests any `/api/e2e/*` endpoint
- THEN the BFF SHALL report the feature disabled, not an error
- AND the promotion dashboard SHALL be unaffected

### Requirement: E2E-LOG-02 -- Fleet-Identity Firewall and Configuration

The e2e-run feature SHALL obtain every endpoint, tenant, and tuning value from
`FD_*` environment variables, never compiled in, consistent with the fleet-identity
firewall (`pkg/config`) and the "separate configuration from code" convention. At
minimum: `FD_E2E_LOKI_URL` (the internal Loki base URL, for example
`http://e2e-loki.hypershell-observability.svc:3100`; empty disables the feature),
`FD_E2E_LOKI_TENANTS` (comma-separated allowlist of permitted tenants),
`FD_E2E_HISTORY_WINDOW` (lookback, default `168h`), `FD_E2E_RECENT_RUN_LIMIT`
(default `20`), and `FD_E2E_LOG_PAGE_SIZE` (the raw-log history page size, default
`150`, bounded by a server-side maximum so a client cannot re-introduce the
all-at-once timeout; see E2E-LOG-19). The deployment configuration carries the actual fleet identities;
the binary SHALL carry none. Changing an endpoint, tenant allowlist, window, or
limit SHALL NOT require a code change.

#### Scenario: E2E-LOG-02.01 -- Empty Loki URL disables the feature

- GIVEN `FD_E2E_LOKI_URL` is empty
- WHEN the BFF starts
- THEN the e2e-run source SHALL NOT run and the endpoints SHALL report the feature disabled

#### Scenario: E2E-LOG-02.02 -- Config comes from env

- GIVEN the BFF starts with `FD_E2E_LOKI_URL`, `FD_E2E_LOKI_TENANTS`, `FD_E2E_HISTORY_WINDOW`, and `FD_E2E_RECENT_RUN_LIMIT` set
- WHEN the source resolves its endpoint, tenants, window, and limit
- THEN it SHALL read them from configuration
- AND no fleet-identifying endpoint or tenant SHALL be compiled into the binary

### Requirement: E2E-LOG-03 -- Loki Access Behind an Application Port

The Loki history and tail access SHALL sit behind an application-owned port in the
BFF, so the domain and the plane depend on an interface expressed in e2e-run terms
(recent runs, a run's events, a live subscription) rather than on Loki HTTP shapes.
A **Loki adapter** SHALL be the only implementation shipped by this spec. The BFF
SHALL build LogQL only from validated inputs and SHALL NOT expose an arbitrary
query, URL, or tenant proxy to the browser; the Loki service and its credentials
SHALL NOT be reachable from the browser.

#### Scenario: E2E-LOG-03.01 -- Plane depends only on the port

- GIVEN the BFF is configured with the Loki adapter
- WHEN the e2e-run plane refreshes or streams
- THEN it SHALL obtain recent runs, run events, and live subscriptions through the port only
- AND it SHALL NOT call Loki directly from the plane or the domain

#### Scenario: E2E-LOG-03.02 -- No arbitrary query proxy

- GIVEN a browser request carrying filter parameters
- WHEN the BFF builds the Loki query
- THEN it SHALL validate every filter and build LogQL safely
- AND it SHALL NOT proxy an arbitrary query, URL, or tenant to Loki

### Requirement: E2E-LOG-04 -- Bounded History From query_range

Initial recent-run history SHALL be read via bounded Loki `query_range` calls over
the configured lookback window. The adapter SHALL enforce the lookback and
line/byte/page/connection limits, SHALL paginate without silently dropping runs,
and SHALL explicitly report when history is truncated or incomplete rather than
present a partial list as complete. Historical ingestion SHALL be understood to
begin only after the log pipeline is configured; runs that predate structured
reporting (for example older Bash-suite runs) MAY have logs but no structured steps,
and the view SHALL say so rather than render them as stepless passes.

#### Scenario: E2E-LOG-04.01 -- History respects the lookback and limit

- GIVEN `FD_E2E_HISTORY_WINDOW=168h` and `FD_E2E_RECENT_RUN_LIMIT=20`
- WHEN the source reads recent runs
- THEN it SHALL query at most the 168h window and return at most 20 runs
- AND paging SHALL not drop runs within the window

#### Scenario: E2E-LOG-04.02 -- Truncated history is reported

- GIVEN more runs exist in the window than the configured limits allow to be returned
- WHEN the source serves recent runs
- THEN it SHALL flag the result as truncated/incomplete, not present it as the complete set

#### Scenario: E2E-LOG-04.03 -- Pre-reporting run is marked stepless

- GIVEN a run whose logs exist but contain no `hypershell.e2e.v1` structured events
- WHEN it appears in the run list or detail
- THEN it SHALL be marked as having no structured steps, not rendered as a stepless pass

### Requirement: E2E-LOG-05 -- Live Streaming via Loki Tail

Live updates SHALL be produced by following Loki's `/loki/api/v1/tail` WebSocket and
pushing updates to browsers over Server-Sent Events or a properly authenticated
streaming fetch. The BFF SHALL NOT substitute repeated `query_range` polling for the
requested live behavior. Because Loki tail is single-tenant, the BFF SHALL open an
authorized tail per configured tenant rather than assume a pipe-separated tenant
header works for tail. Within each backend replica, upstream tails SHALL be bounded
and shared/aggregated across subscribed browsers rather than opening one upstream
feed per browser. The stream SHALL provide heartbeats, honor cancellation, apply
backpressure with bounded buffers, and bound replay.

#### Scenario: E2E-LOG-05.01 -- Live steps stream, not poll

- GIVEN a run is executing and emitting events
- WHEN a browser subscribes to `GET /api/e2e/stream` for that run
- THEN the BFF SHALL deliver new step and log events from a Loki tail
- AND it SHALL NOT fall back to repeated `query_range` polling for the live path

#### Scenario: E2E-LOG-05.02 -- Per-tenant tail

- GIVEN two configured tenants
- WHEN the BFF tails live logs
- THEN it SHALL open an authorized tail per tenant
- AND it SHALL NOT send a pipe-separated multi-tenant header to the tail endpoint

#### Scenario: E2E-LOG-05.03 -- Shared upstream within a replica

- GIVEN several browsers subscribed to runs on the same tenant on one replica
- WHEN the replica tails Loki
- THEN it SHALL share bounded upstream tails across those browsers
- AND it SHALL NOT open one upstream feed per browser

### Requirement: E2E-LOG-06 -- Snapshot-to-Tail Bridge Without Lost Events

The BFF SHALL bridge the initial snapshot and the live tail without a race that
silently loses events. On disconnect and reconnect it SHALL backfill an overlapping
window, deduplicate by `runId` and `sequence`, then continue live. It SHALL surface
dropped entries and ingestion/reconnect gaps to the consumer, and SHALL NEVER
represent a disconnected feed as healthy or live. Because two BFF replicas exist and
there is no sticky routing, the bridge SHALL NOT depend on session stickiness or on
process-local history being durable.

#### Scenario: E2E-LOG-06.01 -- Reconnect backfills and deduplicates

- GIVEN a live stream drops and reconnects
- WHEN the BFF resumes
- THEN it SHALL backfill an overlapping window, deduplicate by `runId`/`sequence`, and continue live
- AND no event in the overlap SHALL be delivered twice or lost

#### Scenario: E2E-LOG-06.02 -- Dropped entries are surfaced

- GIVEN Loki reports dropped entries or an ingestion gap
- WHEN the BFF streams to the browser
- THEN it SHALL surface the drop/gap
- AND it SHALL NOT represent the feed as healthy or live during the gap

#### Scenario: E2E-LOG-06.03 -- No dependence on stickiness

- GIVEN a browser's requests are served by either of the two replicas
- WHEN it loads history and subscribes to the stream
- THEN the result SHALL be correct without sticky routing
- AND it SHALL NOT rely on process-local history being durable

### Requirement: E2E-LOG-07 -- ViaQ Envelope Parsing

The adapter SHALL parse the ViaQ record envelope first, then the suite's JSON line
inside its `message` field; it SHALL NOT assume the Loki line is directly the E2E
event JSON. Run ids, commit revisions, and step ids SHALL be read from the parsed
JSON body, not from Loki index labels. A malformed envelope or message SHALL be
handled without dropping the rest of the stream.

#### Scenario: E2E-LOG-07.01 -- Envelope then message

- GIVEN a ViaQ record whose `message` field holds the suite's `hypershell.e2e.v1` line
- WHEN the adapter decodes it
- THEN it SHALL parse the ViaQ envelope, then parse the `message` body
- AND it SHALL read `runId`, `revision`, and `stepId` from the body, not from index labels

#### Scenario: E2E-LOG-07.02 -- Malformed record is isolated

- GIVEN one record whose envelope or message is malformed
- WHEN the adapter processes the stream
- THEN it SHALL skip or mark that record
- AND it SHALL continue delivering the remaining well-formed records

### Requirement: E2E-LOG-08 -- Trusted Identity From Tenant, Not Body

The BFF SHALL derive a run's trusted cluster identity from the configured Loki
tenant it was read from; the cluster value in the record body SHALL be treated as
metadata for display, not as an authorization input. The application source revision
SHALL be displayed separately from the deployed/hydrated revision. Access to all
e2e-run endpoints SHALL remain behind the existing OAuth plus backend TokenReview /
SAR. A long-lived stream SHALL terminate or revalidate when its authorization
expires, and the UI SHALL follow the existing session-expiry convention.

#### Scenario: E2E-LOG-08.01 -- Cluster identity is the tenant, not the body

- GIVEN a record whose body `cluster` differs from the tenant it was read from
- WHEN the BFF establishes the run's trusted cluster
- THEN it SHALL use the configured tenant
- AND the body `cluster` SHALL be display metadata only

#### Scenario: E2E-LOG-08.02 -- Expired authorization ends the stream

- GIVEN an authorized browser holding an open e2e stream
- WHEN its authorization expires
- THEN the BFF SHALL terminate or revalidate the stream
- AND the UI SHALL follow the existing session-expiry takeover

### Requirement: E2E-LOG-09 -- Run-to-Promotion Correlation

A run SHALL be joined to a promotion by environment/namespace AND exact
hydrated/deployed revision, then by reporting attempt id; the application source
revision SHALL be shown separately. The correlation SHALL NOT replace an older
release's evidence with a newer run's results for a different revision. Because the
AnalysisRun health override in hypershell-gitops intentionally decouples Argo
Application health from the test verdict, a run's failed or unknown state SHALL be
preserved and shown even when the Argo Application reports Healthy.

#### Scenario: E2E-LOG-09.01 -- Join by environment and exact revision

- GIVEN a promotion at a known environment/namespace and hydrated revision
- WHEN the view correlates runs to it
- THEN it SHALL match on environment/namespace and exact hydrated revision, then attempt id
- AND it SHALL display the application source revision separately

#### Scenario: E2E-LOG-09.02 -- Failed run shown under Healthy Argo

- GIVEN a run that failed or is unknown while the Argo Application reports Healthy
- WHEN the view renders the run against its promotion
- THEN the failed/unknown state SHALL be preserved and shown, not masked by Argo health

#### Scenario: E2E-LOG-09.03 -- Newer run does not overwrite older evidence

- GIVEN an older released revision with its own run evidence
- WHEN a newer run for a different revision is ingested
- THEN the older revision's evidence SHALL remain
- AND it SHALL NOT be replaced by the newer run's results

### Requirement: E2E-LOG-10 -- Degraded States Are Distinguished

A Loki outage SHALL degrade only the test-details pane; it SHALL NEVER blank the
promotion dashboard or alter promotion gates. The view SHALL distinguish, as
separate states: test failed, reporting incomplete, backend unavailable, and live
disconnected. Log history SHALL survive Job/Pod TTL because it lives in Loki, not in
the Pod.

#### Scenario: E2E-LOG-10.01 -- Loki outage degrades only the pane

- GIVEN Loki is unavailable
- WHEN the dashboard renders
- THEN only the e2e test-details pane SHALL degrade
- AND the promotion dashboard and its gates SHALL render unchanged

#### Scenario: E2E-LOG-10.02 -- Four states are distinct

- GIVEN a run that failed, a run with missing terminal records, a backend outage, and a dropped live feed
- WHEN each is rendered
- THEN the view SHALL show test-failed, reporting-incomplete, backend-unavailable, and live-disconnected as four distinct states

#### Scenario: E2E-LOG-10.03 -- History survives Pod deletion

- GIVEN a completed run whose Pod has been reaped
- WHEN an operator opens that run
- THEN its steps and logs SHALL still render from Loki

### Requirement: E2E-LOG-11 -- Hexagonal UI Boundary

The e2e-run view SHALL follow the narrow hexagonal UI boundary
(`specs/standards/ui/hexagonal-architecture.spec.md`). The addition SHALL comprise:
framework-free e2e-run domain types (runs, steps, log lines, states), new
`FleetApi` port methods for recent runs, run detail, and the live subscription (each
returning the freshness envelope for snapshot data and a cancellable subscription
for the stream), a new wire mapper in the anti-corruption layer
(`adapters/api/wire.ts`) as the only place that knows the `/api/e2e/*` wire shape,
and TanStack Query / streaming adapters outside the port. React, TanStack Query, the
HTTP and streaming adapters, and the generated wire shapes SHALL stay outside the
port; the domain and presentation SHALL depend only on the port and domain types.
Event reduction (ordering, deduplication, step-state derivation) SHALL live in the
framework-free domain, not in a component or adapter.

#### Scenario: E2E-LOG-11.01 -- E2E data flows through the port and mapper

- GIVEN the UI renders the e2e-run view
- WHEN it loads runs or subscribes to a stream
- THEN it SHALL call the port methods, and the BFF JSON/stream SHALL be translated to domain types only in the wire mapper
- AND the `AbortSignal` SHALL propagate to the port and cancel the subscription

#### Scenario: E2E-LOG-11.02 -- Reduction is framework-free

- GIVEN a sequence of run/step/log events including duplicates and reconnect overlap
- WHEN the view derives per-step state and ordering
- THEN the reduction SHALL be performed by the framework-free domain, not in a component or adapter

### Requirement: E2E-LOG-12 -- E2E Run View

The dashboard SHALL present an e2e-run view reachable from the promotion gate /
environment. It SHALL list the most recent runs newest-first, each showing revision,
attempt, start/end/duration, overall verdict, and the live/disconnected/
history-truncated state. Selecting a run SHALL show its ordered steps with
passed/failed/skipped/not-run/running state, per-step durations, and a selectable
inline log pane. The ordered steps and verdict SHALL render as soon as the small
lifecycle query resolves, independently of the raw-log history, so a slow or failed
log fetch never blanks the panel (see E2E-LOG-20). The log pane SHALL load raw
logs a bounded page at a time by infinite scroll rather than in one fetch
(see E2E-LOG-19, E2E-LOG-22): it SHALL render the most recent page first, load
older pages automatically as the operator scrolls toward the oldest end with a
visible loading indicator, and preserve the operator's scroll position when older
lines are prepended, without a "Load more" button. The view SHALL offer
follow/pause, bounded rendering of long logs,
a choice between a selected step's logs and the whole run's logs, and retained
history after the Pod is deleted. New runs, steps, and log lines SHALL arrive live.
A run's explicit failed/unknown state SHALL be preserved even when the Argo
Application health is Healthy (E2E-LOG-09).

Selecting a promotion gate SHALL open an execution drawer with Progress and Logs
tabs sharing one live subscription. Selecting a step SHALL open its scoped logs;
switching tabs SHALL preserve scope and search. Operators SHALL be able to search
logs, toggle timestamps, and expand the log viewer. Gate checks SHALL remain
visible independently of the suite verdict. Correlation SHALL require the full
hydrated revision, trusted cluster, and workload namespace; source revision and
short revision prefixes SHALL NOT select a run. The selected attempt SHALL stay
pinned when a retry arrives, with an explicit newer-attempt notice. The E2E Runs
tab SHALL remain available for searchable history, including navigation from the
gate's selected attempt. History SHALL use a closable, resizable PatternFly drawer
with independent pane scrolling and an overlay on narrow screens. Closing it SHALL
preserve the history filter, sorting, and selected row, and restore trigger focus.
All run and progress table columns SHALL support ascending and descending sorting
using source values; history defaults to newest-first, steps to execution order,
and unknown durations remain last in either sort direction.

The history table SHALL show sortable Bundle and Target columns. Bundle identity
SHALL use the same release digest, identicon, and name as Fleet Map. Historical
resolution SHALL follow the run's full hydrated commit through `hydrator.metadata`
to the dry commit's release lock; it SHALL never substitute a currently deployed
bundle for an older run. Missing metadata SHALL show bundle unavailable. Live
updates SHALL preserve resolved bundle identity. The target SHALL show an instance
name only for a unique trusted cluster/namespace match, with cluster and namespace
as supporting context; an unknown or ambiguous instance SHALL fall back to cluster
and namespace. Search SHALL include instance names and bundle versions.

History queries SHALL isolate lifecycle events from the bounded log window so
chatty output cannot evict the run verdict or steps. Raw-log history SHALL be read
by cursor-paged queries whose per-page size stays well below the Loki query
timeout, never by a single all-at-once query over the whole run (see E2E-LOG-19).
The stream SHALL subscribe
before fetching backfill to avoid a snapshot-to-tail race, deduplicate sequence
overlap, and surface truncated history or dropped events.

#### Scenario: E2E-LOG-12.01 -- Recent runs newest-first

- GIVEN several runs in the history window
- WHEN the view opens
- THEN it SHALL list them newest-first with revision, attempt, start/end/duration, and verdict

#### Scenario: E2E-LOG-12.02 -- Ordered steps with state and logs

- GIVEN a selected run
- WHEN its detail renders
- THEN it SHALL show ordered steps with passed/failed/skipped/not-run/running state and durations
- AND a selectable inline log pane for a chosen step or the whole run

#### Scenario: E2E-LOG-12.03 -- Live arrival with follow and pause

- GIVEN a run is executing
- WHEN the operator is viewing it
- THEN new steps and log lines SHALL arrive live
- AND the operator SHALL be able to follow or pause, with long logs bounded in rendering

#### Scenario: E2E-LOG-12.04 -- Not-run distinguished from skipped

- GIVEN a fail-fast run where later steps were never reached and a mode-skipped step
- WHEN the detail renders
- THEN not-run steps SHALL be distinct from skipped steps, and neither SHALL read as passed

### Requirement: E2E-LOG-13 -- Domain Observability Probes

The e2e-run view SHALL publish typed workflow and dependency facts through the
dashboard's fan-out observability port, consistent with the "domain probes for UI
observability" convention and `specs/standards/ui/domain-observability.spec.md`.
Loading recent runs and run detail SHALL publish a workflow probe with outcomes
`started`, `succeeded`, `failed`, and `cancelled`; the live subscription SHALL
publish connect, disconnect, reconnect, and dropped-entry facts. Production browser
and BFF code SHALL NOT make raw console or direct telemetry calls.

#### Scenario: E2E-LOG-13.01 -- Stream lifecycle publishes facts

- GIVEN the e2e-run view subscribes to a live stream
- WHEN the stream connects, drops, reconnects, or reports dropped entries
- THEN a corresponding probe fact SHALL be published
- AND no raw console or direct telemetry call SHALL be made in production code

### Requirement: E2E-LOG-14 -- Accessibility and Localization

Every signal in the e2e-run view SHALL be perceivable without color as the sole
channel (`specs/standards/ui/accessibility.spec.md`): step state
(passed/failed/skipped/not-run/running), run verdict, and the live/disconnected/
truncated/incomplete states SHALL each carry a non-color cue (text, icon, or shape).
All user-visible strings, including relative timestamps and any caller-supplied
fallback labels in reusable presentation helpers, SHALL be localized messages.
Interactive elements (run selection, step selection, follow/pause) SHALL be keyboard
operable and expose accessible names. Log output SHALL be escaped so record content
cannot inject markup.

#### Scenario: E2E-LOG-14.01 -- Non-color cue for step state

- GIVEN a failed step and a skipped step
- WHEN they render
- THEN each state SHALL carry a non-color cue in addition to any color

#### Scenario: E2E-LOG-14.02 -- Log content is escaped

- GIVEN a log line containing markup-like characters
- WHEN it renders in the log pane
- THEN it SHALL be escaped and SHALL NOT inject markup

### Requirement: E2E-LOG-15 -- Access Control

The `/api/e2e/*` endpoints and the e2e-run view SHALL use the fleet dashboard's
existing authentication middleware and SAR authorization firewall; they SHALL NOT
introduce a separate access-control path. When auth is disabled (dev mode), the
endpoints and view SHALL remain open, matching the other planes.

#### Scenario: E2E-LOG-15.01 -- E2E endpoints honor the existing SAR gate

- GIVEN auth is enabled and the caller is not authorized by the SAR firewall
- WHEN the caller requests any `/api/e2e/*` endpoint
- THEN the BFF SHALL deny the request through the same middleware as the other planes

### Requirement: E2E-LOG-16 -- No Approved Mockup

No approved mockup exists for the e2e test-run view. Per the repository no-mockup
rule, the view SHALL be built against the UI standards and canonical shared
components without inventing an approval block or a placeholder mockup, and the
`ui-build-gate` mockup-to-production parity checks SHALL be skipped while all
non-parity checks (PatternFly 6 usage, hexagonal boundary, accessibility,
localization, tests) still run. If an approved mockup is later supplied, the parity
path applies.

#### Scenario: E2E-LOG-16.01 -- Built without a fabricated mockup

- GIVEN no approved mockup for the e2e-run view
- WHEN the view is built and verified
- THEN no mockup or approval block SHALL be fabricated
- AND the non-parity UI checks SHALL still be run

### Requirement: E2E-LOG-17 -- Tests and Fixtures

The repository SHALL include tests that validate behavior without a live fleet,
using fixtures for the structured event stream and the Loki responses. BFF tests
SHALL exercise mocked HTTP `query_range` and WebSocket `tail`, the snapshot/tail gap
and reconnect/backfill, dropped messages, tenant allowlisting, bounded history and
buffers, cancellation, and expired authorization. They SHALL also exercise raw-log
pagination: a multi-page walk that terminates when `hasMore` is false, a page
boundary where entries share a nanosecond timestamp proving none are dropped or
duplicated, the `limit` clamp, and run detail that still returns steps when the
raw-log query errors (non-fatal logs, E2E-LOG-20). UI tests SHALL cover event
reduction and ordering, duplicate and reconnect behavior, parallel steps, the
skipped-versus-not-run distinction, missing terminal records, malformed messages,
output escaping, auth/session expiry, recent attempts, exact-SHA correlation, live
steps and logs, reconnect state, failure, and retained history. UI tests SHALL also
cover infinite scroll: the older-page load firing from the scroll observer rather
than a click, the loading indicator while a page is in flight, preserved scroll
position on prepend, stopping at `hasMoreOlder` false, and merging live-streamed
lines without duplicating the history-to-live seam.

#### Scenario: E2E-LOG-17.01 -- BFF gap and reconnect are tested

- GIVEN a test harness with mocked `query_range` and `tail`
- WHEN a snapshot/tail gap and a reconnect are simulated
- THEN the tests SHALL assert backfill, deduplication, surfaced drops, and no lost or duplicated events

#### Scenario: E2E-LOG-17.02 -- UI reduction cases are tested

- GIVEN fixtures for duplicates, reconnect overlap, parallel steps, skipped versus not-run, missing terminal records, and malformed messages
- WHEN the domain reduces them
- THEN the tests SHALL assert correct ordering, deduplication, step state, and the preserved failed/unknown states

### Requirement: E2E-LOG-18 -- Documentation and GitOps Linkage

The change SHALL document the config variables (`FD_E2E_LOKI_URL`,
`FD_E2E_LOKI_TENANTS`, `FD_E2E_HISTORY_WINDOW`, `FD_E2E_RECENT_RUN_LIMIT`,
`FD_E2E_LOG_PAGE_SIZE`), the
consumed event schema (`hypershell.e2e.v1`), the existing suite launch-command
compatibility, and the retention and limits. The pull request SHALL link the
hypershell-gitops logging/configuration integration. The feature SHALL NOT be
claimed deployed or live until the application release and the dashboard image are
built and hypershell-gitops pins them.

#### Scenario: E2E-LOG-18.01 -- Deployment not claimed prematurely

- GIVEN the application and dashboard changes are merged but not yet built and pinned by hypershell-gitops
- WHEN the work is reported
- THEN it SHALL NOT be claimed deployed or live
- AND the PR SHALL link the hypershell-gitops integration

### Requirement: E2E-LOG-19 -- Paginated Raw-Log History

Raw-log history for one run SHALL be read through a dedicated, cursor-paged
endpoint (`GET /api/e2e/logs?run=<id>&before=<ts>&limit=<n>`), never by a single
all-at-once query over the whole run, so every request stays well below the Loki
query timeout even on a single-CPU Loki. The run id rides a `run` query value
rather than a path segment because a `ServeMux` trailing wildcard (the multi-segment
`tenant/namespace/runId` address) must be the final path segment, so it cannot be
followed by a literal `/logs`; this mirrors the live-stream endpoint
(`GET /api/e2e/stream?run=<id>`). The endpoint SHALL reuse the exact run address,
tenant allowlist, and authorization as `GET /api/e2e/runs/{id}` (the same
`ErrRunNotFound` on an unknown tenant), and SHALL wrap each page in the `pkg/cache`
freshness envelope.

A page SHALL be requested with an optional `before` cursor (a nanosecond timestamp,
exclusive upper bound; omitted means the newest page) and an optional `limit`
(default `FD_E2E_LOG_PAGE_SIZE`, clamped to a server-side maximum so a client
cannot re-introduce the timeout). The source SHALL issue one bounded raw-log
`query_range` per page over the configured history window, read `direction=backward`
(newest first) and reverse to chronological-ascending for display, and return the
page together with a `nextCursor` (the oldest timestamp in the page) and a `hasMore`
signal (the page was full). Because Loki has no opaque offset token and multiple
entries can share one nanosecond timestamp, the server SHALL de-duplicate across a
page boundary (for example by querying with a one-nanosecond inclusive overlap and
de-duping on `(timestampNs, line)`, or by a stable per-entry `sequence`) so a page
boundary neither drops nor duplicates an entry; it SHALL NOT assume unique
timestamps. Paginated collection reads SHALL conform to the trust-and-performance
collection contract (`specs/standards/ui/trust-performance.spec.md`): one request
per page, a bound that does not grow with the number of rendered lines.

#### Scenario: E2E-LOG-19.01 -- Newest page then older pages by cursor

- GIVEN a chatty run with more log lines than one page
- WHEN a client requests `GET /api/e2e/logs?run=<id>` with no cursor, then again with the returned `nextCursor`
- THEN the first response SHALL return the most recent page with `hasMore` true and a `nextCursor`
- AND the second response SHALL return the immediately older page, continuing until `hasMore` is false

#### Scenario: E2E-LOG-19.02 -- Shared-timestamp boundary is neither dropped nor duplicated

- GIVEN two log entries that share the exact same nanosecond timestamp across a page boundary
- WHEN the client pages across that boundary
- THEN both entries SHALL appear exactly once across the two pages, with none dropped or duplicated

#### Scenario: E2E-LOG-19.03 -- Limit is clamped

- GIVEN a client requests a `limit` above the server-side maximum
- WHEN the endpoint serves the page
- THEN it SHALL clamp the page to the maximum rather than issue an unbounded query

#### Scenario: E2E-LOG-19.04 -- Logs endpoint honors the tenant allowlist

- GIVEN a run id whose tenant is not in the allowlist
- WHEN a client requests `GET /api/e2e/logs?run=<id>`
- THEN the BFF SHALL respond `ErrRunNotFound`, the same as the run-detail endpoint

### Requirement: E2E-LOG-20 -- Run Detail Decoupled From Raw Logs

A failure or timeout of the raw-log query SHALL NOT fail the whole run detail.
`GET /api/e2e/runs/{id}` SHALL return the run's ordered steps and verdict from the
small, always-fast lifecycle query even when the raw-log read errors, and SHALL
mark logs degraded rather than return `ErrLogBackendUnavailable` for the whole
detail. The genuine backend-unavailable state (E2E-LOG-10) SHALL be reserved for
the case where even the lifecycle/steps query cannot load. This supersedes the
earlier behavior in which one slow raw-log query blanked the entire detail panel,
including the steps that were cheap to compute.

#### Scenario: E2E-LOG-20.01 -- Steps render when the raw-log query fails

- GIVEN the lifecycle query succeeds but the raw-log query errors or times out
- WHEN a client requests `GET /api/e2e/runs/{id}`
- THEN the response SHALL carry the ordered steps and verdict
- AND it SHALL mark logs degraded rather than fail the whole detail

#### Scenario: E2E-LOG-20.02 -- Backend-unavailable only when steps cannot load

- GIVEN even the lifecycle/steps query cannot load
- WHEN the detail is served
- THEN the view SHALL show the backend-unavailable state (E2E-LOG-10), distinct from a logs-only degradation

### Requirement: E2E-LOG-21 -- Live Log Lines on the Stream, In-Progress Only

The live stream SHALL carry individual raw log lines for an in-progress run so the
browser appends them to the same rendered log list it paginates. The existing SSE
framing SHALL gain a `log` frame kind, in addition to the current `event`,
`status`, and `heartbeat` kinds, carrying one raw log line with its `sequence` and
`timestamp`. The BFF SHALL open a live tail for a run only while that run is in
progress (no `run_finished` / no `endedAt`); a completed run SHALL be read by
pagination alone, because Loki `tail` does not replay history and tailing a finished
run would render nothing. On handoff, the newest history page and the live stream
SHALL be reconciled by `sequence` (or `(timestampNs, line)`) so the seam between the
last history line and the first live line neither drops nor duplicates a line,
consistent with the snapshot-to-tail bridge (E2E-LOG-06).

#### Scenario: E2E-LOG-21.01 -- Live log frames append for an in-progress run

- GIVEN a run that is still in progress and emitting log lines
- WHEN a browser subscribes to `GET /api/e2e/stream` for that run
- THEN the BFF SHALL deliver new lines as `log` frames carrying `sequence` and `timestamp`
- AND the browser SHALL append them to the same log list it paginates

#### Scenario: E2E-LOG-21.02 -- No live tail for a finished run

- GIVEN a completed run (its terminal record is present)
- WHEN its logs are viewed
- THEN the view SHALL read them by pagination only and SHALL NOT open a live tail that would render nothing

#### Scenario: E2E-LOG-21.03 -- History-to-live seam is de-duplicated

- GIVEN the newest history page overlaps the first live lines
- WHEN the browser hands off from history to the live stream
- THEN the overlap SHALL be de-duplicated by `sequence` so no line is dropped or duplicated at the seam

### Requirement: E2E-LOG-22 -- Infinite-Scroll Log Rendering and Degraded Log Pane

The log pane SHALL load older history by scroll position, not by a button: a
sentinel at the top of the scroll container SHALL trigger the next older page when
it enters view, guarded so only one page is in flight and stopping when no older
page remains. While an older page is in flight, the pane SHALL render an accessible
loading indicator; when older lines are prepended, the pane SHALL preserve the
operator's visual position so the viewport does not jump. For an in-progress run,
live-appended lines SHALL keep the view pinned to the bottom while the operator is
at the bottom (auto-follow); if the operator scrolls up, auto-follow SHALL stop and
a "jump to latest" affordance SHALL appear. The in-memory log buffer and rendered
rows SHALL be bounded so a very long run does not grow the DOM or heap without
bound. When steps load but logs are unavailable, the pane SHALL show an inline,
non-blocking notice on the log pane only; it SHALL NOT blank the whole panel, and it
SHALL reserve the full backend-unavailable message for the case where even steps
cannot load (E2E-LOG-20). The scroll position, loading flag, auto-follow state, and
log buffer are transient view state and SHALL NOT be encoded in the URL, consistent
with `glass-url-addressable-navigation.spec.md`.

#### Scenario: E2E-LOG-22.01 -- Older pages auto-load on scroll with an indicator

- GIVEN a run whose log history spans several pages
- WHEN the operator scrolls toward the oldest end
- THEN the next older page SHALL load automatically with a visible loading indicator and no "Load more" button
- AND the operator's scroll position SHALL be preserved when older lines are prepended

#### Scenario: E2E-LOG-22.02 -- Auto-follow for an in-progress run

- GIVEN an in-progress run streaming new lines while the operator is at the bottom
- WHEN new lines arrive
- THEN the view SHALL stay pinned to the bottom
- AND if the operator scrolls up, auto-follow SHALL stop and a "jump to latest" affordance SHALL appear

#### Scenario: E2E-LOG-22.03 -- Degraded log pane does not blank the panel

- GIVEN steps load but the raw-log history is unavailable
- WHEN the panel renders
- THEN it SHALL show the steps with an inline notice on the log pane only, not the full backend-unavailable message

## Non-Goals

- Owning the log pipeline. The central Loki receiver, retention, the
  ClusterLogForwarder, and the `X-Scope-OrgID` override are owned by
  hypershell-gitops; this spec consumes them.
- Restoring the removed Bash E2E suite or adding a migration/fallback runner. The
  current Go `TestE2E` suite is the single production suite
  (`e2e-testing.spec.md`).
- Manufacturing a successful promotion. Observability SHALL NOT become a dependency
  for test success or alter the Promoter decision path.
- Exposing Loki, its queries, or its credentials to the browser.
- A new durable analytics store. History is Loki over the configured retention
  window; longer-term analytics are out of scope.
- Push for the promotion plane. Only the e2e-run view streams; the existing planes
  keep their cached-poll model.
