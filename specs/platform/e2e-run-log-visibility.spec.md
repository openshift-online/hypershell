# E2E Run and Log Visibility

**Status:** Draft
**Applies to:** `components/fleet-dashboard` BFF (new e2e-run plane, Loki source, live stream), `packages/fleet-dashboard-ui` (e2e-run view, domain and presentation)
**Tracks:** [HYPERSHELL-402](https://redhat.atlassian.net/browse/HYPERSHELL-402)

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
spec consumes that pipeline and SHALL NOT restate or own it.

### Relationship to the existing fleet dashboard

| Concern | Promotion gate (existing) | E2E run view (this spec) |
| --- | --- | --- |
| BFF plane | `/api/promotion` | new `/api/e2e/runs`, `/api/e2e/runs/{id}`, `/api/e2e/stream` |
| Source | `pkg/sources/promotion.go` (Kubernetes dynamic client) | new Loki source (`query_range` history + `tail` live) |
| Transport | cached JSON snapshot + client poll | snapshot + server-streamed live updates (SSE / authenticated streaming fetch) |
| UI | gate badge + Argo/analysis deep links | recent runs, ordered steps, selectable inline log pane, live/disconnected/history-truncated states |
| Port method | `FleetApi.getPromotion` | `FleetApi.getE2eRuns` / `getE2eRun` / `streamE2eRun` |

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
(`GET /api/e2e/runs`), a single run's detail and step/log history
(`GET /api/e2e/runs/{id}`), and a live stream (`GET /api/e2e/stream`). The exact
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

#### Scenario: Recent runs served from history

- GIVEN the Loki endpoint is configured and the source has refreshed at least once
- WHEN a client requests `GET /api/e2e/runs`
- THEN the BFF SHALL respond with HTTP `200` and the recent-runs snapshot envelope

#### Scenario: Run detail addressed without cross-cluster collision

- GIVEN two runs on different clusters that share the suite-emitted `runId`
- WHEN a client requests each run's detail
- THEN each request SHALL resolve to the correct run because the run address includes its tenant/namespace

#### Scenario: Feature disabled when Loki is unconfigured

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
`FD_E2E_HISTORY_WINDOW` (lookback, default `168h`), and `FD_E2E_RECENT_RUN_LIMIT`
(default `20`). The deployment configuration carries the actual fleet identities;
the binary SHALL carry none. Changing an endpoint, tenant allowlist, window, or
limit SHALL NOT require a code change.

#### Scenario: Empty Loki URL disables the feature

- GIVEN `FD_E2E_LOKI_URL` is empty
- WHEN the BFF starts
- THEN the e2e-run source SHALL NOT run and the endpoints SHALL report the feature disabled

#### Scenario: Config comes from env

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

#### Scenario: Plane depends only on the port

- GIVEN the BFF is configured with the Loki adapter
- WHEN the e2e-run plane refreshes or streams
- THEN it SHALL obtain recent runs, run events, and live subscriptions through the port only
- AND it SHALL NOT call Loki directly from the plane or the domain

#### Scenario: No arbitrary query proxy

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

#### Scenario: History respects the lookback and limit

- GIVEN `FD_E2E_HISTORY_WINDOW=168h` and `FD_E2E_RECENT_RUN_LIMIT=20`
- WHEN the source reads recent runs
- THEN it SHALL query at most the 168h window and return at most 20 runs
- AND paging SHALL not drop runs within the window

#### Scenario: Truncated history is reported

- GIVEN more runs exist in the window than the configured limits allow to be returned
- WHEN the source serves recent runs
- THEN it SHALL flag the result as truncated/incomplete, not present it as the complete set

#### Scenario: Pre-reporting run is marked stepless

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

#### Scenario: Live steps stream, not poll

- GIVEN a run is executing and emitting events
- WHEN a browser subscribes to `GET /api/e2e/stream` for that run
- THEN the BFF SHALL deliver new step and log events from a Loki tail
- AND it SHALL NOT fall back to repeated `query_range` polling for the live path

#### Scenario: Per-tenant tail

- GIVEN two configured tenants
- WHEN the BFF tails live logs
- THEN it SHALL open an authorized tail per tenant
- AND it SHALL NOT send a pipe-separated multi-tenant header to the tail endpoint

#### Scenario: Shared upstream within a replica

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

#### Scenario: Reconnect backfills and deduplicates

- GIVEN a live stream drops and reconnects
- WHEN the BFF resumes
- THEN it SHALL backfill an overlapping window, deduplicate by `runId`/`sequence`, and continue live
- AND no event in the overlap SHALL be delivered twice or lost

#### Scenario: Dropped entries are surfaced

- GIVEN Loki reports dropped entries or an ingestion gap
- WHEN the BFF streams to the browser
- THEN it SHALL surface the drop/gap
- AND it SHALL NOT represent the feed as healthy or live during the gap

#### Scenario: No dependence on stickiness

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

#### Scenario: Envelope then message

- GIVEN a ViaQ record whose `message` field holds the suite's `hypershell.e2e.v1` line
- WHEN the adapter decodes it
- THEN it SHALL parse the ViaQ envelope, then parse the `message` body
- AND it SHALL read `runId`, `revision`, and `stepId` from the body, not from index labels

#### Scenario: Malformed record is isolated

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

#### Scenario: Cluster identity is the tenant, not the body

- GIVEN a record whose body `cluster` differs from the tenant it was read from
- WHEN the BFF establishes the run's trusted cluster
- THEN it SHALL use the configured tenant
- AND the body `cluster` SHALL be display metadata only

#### Scenario: Expired authorization ends the stream

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

#### Scenario: Join by environment and exact revision

- GIVEN a promotion at a known environment/namespace and hydrated revision
- WHEN the view correlates runs to it
- THEN it SHALL match on environment/namespace and exact hydrated revision, then attempt id
- AND it SHALL display the application source revision separately

#### Scenario: Failed run shown under Healthy Argo

- GIVEN a run that failed or is unknown while the Argo Application reports Healthy
- WHEN the view renders the run against its promotion
- THEN the failed/unknown state SHALL be preserved and shown, not masked by Argo health

#### Scenario: Newer run does not overwrite older evidence

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

#### Scenario: Loki outage degrades only the pane

- GIVEN Loki is unavailable
- WHEN the dashboard renders
- THEN only the e2e test-details pane SHALL degrade
- AND the promotion dashboard and its gates SHALL render unchanged

#### Scenario: Four states are distinct

- GIVEN a run that failed, a run with missing terminal records, a backend outage, and a dropped live feed
- WHEN each is rendered
- THEN the view SHALL show test-failed, reporting-incomplete, backend-unavailable, and live-disconnected as four distinct states

#### Scenario: History survives Pod deletion

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

#### Scenario: E2E data flows through the port and mapper

- GIVEN the UI renders the e2e-run view
- WHEN it loads runs or subscribes to a stream
- THEN it SHALL call the port methods, and the BFF JSON/stream SHALL be translated to domain types only in the wire mapper
- AND the `AbortSignal` SHALL propagate to the port and cancel the subscription

#### Scenario: Reduction is framework-free

- GIVEN a sequence of run/step/log events including duplicates and reconnect overlap
- WHEN the view derives per-step state and ordering
- THEN the reduction SHALL be performed by the framework-free domain, not in a component or adapter

### Requirement: E2E-LOG-12 -- E2E Run View

The dashboard SHALL present an e2e-run view reachable from the promotion gate /
environment. It SHALL list the most recent runs newest-first, each showing revision,
attempt, start/end/duration, overall verdict, and the live/disconnected/
history-truncated state. Selecting a run SHALL show its ordered steps with
passed/failed/skipped/not-run/running state, per-step durations, and a selectable
inline log pane. The view SHALL offer follow/pause, bounded rendering of long logs,
a choice between a selected step's logs and the whole run's logs, and retained
history after the Pod is deleted. New runs, steps, and log lines SHALL arrive live.
A run's explicit failed/unknown state SHALL be preserved even when the Argo
Application health is Healthy (E2E-LOG-09).

#### Scenario: Recent runs newest-first

- GIVEN several runs in the history window
- WHEN the view opens
- THEN it SHALL list them newest-first with revision, attempt, start/end/duration, and verdict

#### Scenario: Ordered steps with state and logs

- GIVEN a selected run
- WHEN its detail renders
- THEN it SHALL show ordered steps with passed/failed/skipped/not-run/running state and durations
- AND a selectable inline log pane for a chosen step or the whole run

#### Scenario: Live arrival with follow and pause

- GIVEN a run is executing
- WHEN the operator is viewing it
- THEN new steps and log lines SHALL arrive live
- AND the operator SHALL be able to follow or pause, with long logs bounded in rendering

#### Scenario: Not-run distinguished from skipped

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

#### Scenario: Stream lifecycle publishes facts

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

#### Scenario: Non-color cue for step state

- GIVEN a failed step and a skipped step
- WHEN they render
- THEN each state SHALL carry a non-color cue in addition to any color

#### Scenario: Log content is escaped

- GIVEN a log line containing markup-like characters
- WHEN it renders in the log pane
- THEN it SHALL be escaped and SHALL NOT inject markup

### Requirement: E2E-LOG-15 -- Access Control

The `/api/e2e/*` endpoints and the e2e-run view SHALL use the fleet dashboard's
existing authentication middleware and SAR authorization firewall; they SHALL NOT
introduce a separate access-control path. When auth is disabled (dev mode), the
endpoints and view SHALL remain open, matching the other planes.

#### Scenario: E2E endpoints honor the existing SAR gate

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

#### Scenario: Built without a fabricated mockup

- GIVEN no approved mockup for the e2e-run view
- WHEN the view is built and verified
- THEN no mockup or approval block SHALL be fabricated
- AND the non-parity UI checks SHALL still be run

### Requirement: E2E-LOG-17 -- Tests and Fixtures

The repository SHALL include tests that validate behavior without a live fleet,
using fixtures for the structured event stream and the Loki responses. BFF tests
SHALL exercise mocked HTTP `query_range` and WebSocket `tail`, the snapshot/tail gap
and reconnect/backfill, dropped messages, tenant allowlisting, bounded history and
buffers, cancellation, and expired authorization. UI tests SHALL cover event
reduction and ordering, duplicate and reconnect behavior, parallel steps, the
skipped-versus-not-run distinction, missing terminal records, malformed messages,
output escaping, auth/session expiry, recent attempts, exact-SHA correlation, live
steps and logs, reconnect state, failure, and retained history.

#### Scenario: BFF gap and reconnect are tested

- GIVEN a test harness with mocked `query_range` and `tail`
- WHEN a snapshot/tail gap and a reconnect are simulated
- THEN the tests SHALL assert backfill, deduplication, surfaced drops, and no lost or duplicated events

#### Scenario: UI reduction cases are tested

- GIVEN fixtures for duplicates, reconnect overlap, parallel steps, skipped versus not-run, missing terminal records, and malformed messages
- WHEN the domain reduces them
- THEN the tests SHALL assert correct ordering, deduplication, step state, and the preserved failed/unknown states

### Requirement: E2E-LOG-18 -- Documentation and GitOps Linkage

The change SHALL document the config variables (`FD_E2E_LOKI_URL`,
`FD_E2E_LOKI_TENANTS`, `FD_E2E_HISTORY_WINDOW`, `FD_E2E_RECENT_RUN_LIMIT`), the
consumed event schema (`hypershell.e2e.v1`), the existing suite launch-command
compatibility, and the retention and limits. The pull request SHALL link the
hypershell-gitops logging/configuration integration. The feature SHALL NOT be
claimed deployed or live until the application release and the dashboard image are
built and hypershell-gitops pins them.

#### Scenario: Deployment not claimed prematurely

- GIVEN the application and dashboard changes are merged but not yet built and pinned by hypershell-gitops
- WHEN the work is reported
- THEN it SHALL NOT be claimed deployed or live
- AND the PR SHALL link the hypershell-gitops integration

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
