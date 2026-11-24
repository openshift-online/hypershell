# Service Level Indicators

**Status:** Draft
**Applies to:** `components/api-server` (durable convergence tracking, provision and convergence Prometheus metrics, build metadata), `components/control-plane` (reconciliation-lag and convergence-age metrics, build metadata, removal of the in-memory provision histogram), `components/web-console` (build metadata), the OTel `http.server.request.duration` histogram, its verified caller-class attribute and `HYPERSHELL_SYNTHETIC_CLIENT_IDS` deploy setting, and its Prometheus scrape, and every component build pipeline (`Makefile`, `.tekton/*`, component `Dockerfile`s)
**Tracks:** [HYPERSHELL-548](https://redhat.atlassian.net/browse/HYPERSHELL-548)

## Purpose

Give the production Service Level Objectives (SLOs) reliable indicators, and let
every production component report what revision it runs. The platform defines four
SLOs over a rolling window; this specification owns the HyperShell-side
instrumentation for two of them and the lead-time build metadata that supports
promotion monitoring. The SLO rules, burn-rate alerts, and dashboards live in the
GitOps repository and are out of scope here.

| SLO | Good event | Target | Instrumented by |
| --- | --- | --- | --- |
| Change convergence | A create, spec update, or delete that reaches its desired state within 10 minutes | 99% | OBS-SLI-01 through OBS-SLI-06 |
| API latency | A user read under 500 ms, or a user write under 2 s | 99% | OBS-SLI-07, OBS-SLI-08, OBS-SLI-10, OBS-SLI-11 |

Gateway availability and API availability are measured by an external prober and
are specified separately (`platform/outside-prober.spec.md`). The API availability
objective also counts in-pod user request outcomes from the same
`http.server.request.duration` histogram, which carries the response status code,
the templated route (OBS-SLI-10), and the verified caller class (OBS-SLI-11). Both
user API indicators count user traffic only: requests made by the platform's own
synthetic clients, such as the outside prober, are classified `synthetic` and are
excluded, because the prober has its own objectives and alerts. Lead-time build metadata (OBS-SLI-09) is not an SLO; it
supports the lead-time dashboard that reports the age of the oldest undeployed
commit.

This specification treats the API server as the durable source of truth for
change convergence. The Gateway `generation` / `observed_generation` convergence
latch (`platform/data-model.spec.md` PLAT-DATA-06), the canonical phase vocabulary
(`platform/gateway-phase-vocabulary.spec.md`), and the deletion finalization
contract (`platform/gateway-deletion-finalization.spec.md` GTW-DEL-01) are the
primitives the convergence indicator is built on. Where
`standards/security/security.spec.md` imposes a stricter rule on what may appear in
telemetry, that rule governs.

## Requirements

### Requirement: OBS-SLI-01 -- Durable Change-Convergence Tracking in the API Server

The API server SHALL track change convergence for every Gateway durably in its
PostgreSQL store, so that a restart of either the API server or the control plane
never loses, resets, or double-counts convergence data. Change convergence SHALL
NOT be derived from any in-memory state of the control-plane process.

A **change** is any of the following observable desired-state transitions of a
Gateway:

| Change type | Opens when | Converges when |
| --- | --- | --- |
| `create` | the Gateway is first persisted (`generation = 1`, `observed_generation = 0`, PLAT-DATA-06) | `observed_generation == generation` AND `phase == Running` |
| `update` | a desired-spec field change advances `generation` while the Gateway was previously converged (PLAT-DATA-06) | `observed_generation == generation` AND `phase == Running` |
| `delete` | the Gateway deletion is requested | the Gateway record and every Gateway-owned cluster resource are gone (finalization complete per GTW-DEL-01) |

The API server SHALL record a transition timestamp whenever a Gateway `phase`
changes and whenever a provisioning condition changes, using the authoritative
write time of the control-plane back-channel update, so the convergence window and
provision duration are computable from persisted data rather than process memory.

A change is **converged** only when its convergence condition in the table above
holds for the newest `generation`. A `Degraded` phase, a `Failed` phase, or
`observed_generation < generation` SHALL NOT be treated as converged.

#### Scenario: OBS-SLI-01.01 -- Create opens a convergence window

- GIVEN a valid Gateway create request
- WHEN the API server persists the Gateway with `generation = 1` and `observed_generation = 0`
- THEN a `create` convergence window SHALL open with its clock starting at the create time
- AND the window SHALL remain open until `observed_generation == 1` AND `phase == Running`

#### Scenario: OBS-SLI-01.02 -- Spec update opens a convergence window

- GIVEN a converged Gateway with `generation == observed_generation == N` and `phase == Running`
- WHEN a desired-spec field change advances `generation` to `N+1`
- THEN an `update` convergence window SHALL open with its clock starting at that change
- AND the window SHALL converge when `observed_generation == N+1` AND `phase == Running`

#### Scenario: OBS-SLI-01.03 -- Delete converges only after finalization

- GIVEN a Gateway deletion request
- WHEN the control plane has reclaimed every Gateway-owned cluster resource and signals completion by calling `FinalizeGatewayDeletion` (GTW-DEL-10)
- THEN the `delete` convergence window SHALL converge at the finalization signal
- AND a `delete` window SHALL NOT converge while any owned cluster resource remains

#### Scenario: OBS-SLI-01.04 -- Convergence survives a restart

- GIVEN an open convergence window persisted in the API server store
- WHEN the API server or the control plane restarts
- THEN the window and its clock start SHALL be read back from PostgreSQL unchanged
- AND the convergence counts SHALL NOT be reset or double-counted by the restart

### Requirement: OBS-SLI-02 -- Ten-Minute Convergence Outcome Model

Every change SHALL resolve to exactly one terminal outcome, counted exactly once.
The outcome SHALL be one of:

| Outcome | Meaning |
| --- | --- |
| `within_10m` | The change converged within 10 minutes of its clock start. |
| `late` | The change had not converged when 10 minutes elapsed, and the newest generation was not in a terminal-failure state at that moment. |
| `failed` | The change had not converged when 10 minutes elapsed, and the newest generation was in a terminal-failure state (`phase == Failed`, or a `delete` that reached a terminal deletion error) at that moment. |

The API server SHALL count a change as bad (`late` or `failed`) at the moment its
10 minutes elapse without convergence. It SHALL NOT wait for the change to
converge before counting it. A change that never converges SHALL still be counted
bad at the deadline. Once a change has been counted (as `within_10m` at
convergence, or as `late` or `failed` at the deadline), it SHALL NOT be counted
again, even if it converges later.

A newer change SHALL replace an earlier unconverged change for the same Gateway:
only the newest `generation` is judged. The 10-minute clock SHALL be measured from
the **first** unconverged change in an unbroken unconverged run, so repeated edits
while a Gateway is already unconverged SHALL NOT reset the clock. When the newest
generation finally converges within 10 minutes of that first clock start, the
run SHALL count one `within_10m`; otherwise it SHALL count one `late` or one
`failed` at the deadline.

Good SLO events SHALL be the `within_10m` count. Bad SLO events SHALL be the sum
of `late` and `failed`. Every valid change SHALL contribute to exactly one of the
three counts.

The API server SHALL resolve an unconverged change at its 10-minute deadline
through a periodic deadline evaluation that does not depend on an incoming
status write for that Gateway, so a change that stalls with no further write is
still resolved and counted at its deadline. Outcome resolution SHALL be atomic
with the OBS-SLI-03 counter increment and SHALL be exactly-once across every
API-server replica and across restarts: a change SHALL be resolved and counted
no more than once even when several API-server replicas run concurrently against
the same store. A single-owner coordination mechanism (for example a PostgreSQL
advisory lock) MAY serialize the deadline evaluation across replicas.

#### Scenario: OBS-SLI-02.01 -- Converged within budget counts within_10m

- GIVEN a change whose clock started 4 minutes ago
- WHEN the newest generation converges
- THEN the change SHALL count once as `within_10m`
- AND no other outcome SHALL be counted for that change

#### Scenario: OBS-SLI-02.02 -- Unconverged at the deadline counts bad without waiting

- GIVEN a change whose clock started 10 minutes ago and has not converged
- AND the newest generation is not in a terminal-failure state
- WHEN the 10-minute deadline elapses
- THEN the change SHALL count once as `late` immediately
- AND the count SHALL NOT wait for eventual convergence

#### Scenario: OBS-SLI-02.03 -- Never-converging change still counts

- GIVEN a change whose newest generation reached `phase == Failed` before 10 minutes elapsed
- WHEN the 10-minute deadline elapses without convergence
- THEN the change SHALL count once as `failed`

#### Scenario: OBS-SLI-02.04 -- Later convergence is not counted again

- GIVEN a change already counted as `late` at its deadline
- WHEN the newest generation converges 20 minutes after the clock start
- THEN no additional outcome SHALL be counted for that change

#### Scenario: OBS-SLI-02.05 -- Repeated edits do not reset the clock

- GIVEN a Gateway that became unconverged at time T and is still unconverged
- WHEN a further desired-spec field change advances `generation` at time T plus 3 minutes
- THEN the 10-minute clock SHALL still be measured from T
- AND only the newest generation SHALL be judged for convergence

#### Scenario: OBS-SLI-02.06 -- Outcome resolved exactly once across replicas

- GIVEN two API-server replicas running concurrently against the same PostgreSQL store
- AND a change whose 10-minute deadline has elapsed without convergence
- WHEN both replicas run the periodic deadline evaluation
- THEN the change SHALL be resolved and its outcome counted exactly once
- AND no replica SHALL double-count the outcome or re-resolve the change

### Requirement: OBS-SLI-03 -- Durable Convergence Counters Exported by the API Server

The API server SHALL export the change-convergence outcomes as a Prometheus
counter from its metrics endpoint (`:4433/metrics`, OBS-GMD-02), backed by durable
PostgreSQL state so the exported totals are monotonic across restarts:

| Metric | Type | Unit | Description |
| --- | --- | --- | --- |
| `hypershell_change_convergence_total` | Counter | `{change}` | Count of resolved changes by change type and outcome |

The counter SHALL carry only these bounded attributes:

| Attribute | Values |
| --- | --- |
| `change_type` | `create`, `update`, `delete` |
| `outcome` | `within_10m`, `late`, `failed` |

The counter SHALL NOT include a Gateway identifier, namespace name, customer
identifier, or any label whose cardinality grows with fleet size. The total series
count SHALL NOT exceed `|change_type| x |outcome|` = 3 x 3 = 9 series. When the
metrics endpoint is scraped, the counter SHALL report the durable cumulative
totals rather than a value derived from process uptime.

#### Scenario: OBS-SLI-03.01 -- Convergence counter exported with bounded labels

- GIVEN the API server has resolved at least one change of each type
- WHEN Prometheus scrapes `:4433/metrics`
- THEN `hypershell_change_convergence_total` SHALL be present with `change_type` and `outcome` labels
- AND no sample SHALL carry a Gateway identifier or namespace name as a label

#### Scenario: OBS-SLI-03.02 -- Counter totals survive a restart

- GIVEN `hypershell_change_convergence_total{change_type="create",outcome="within_10m"}` reports a value of 12
- WHEN the API server restarts and is scraped again
- THEN the counter SHALL report at least 12 for that series
- AND the restart SHALL NOT reset the series to zero or double-count resolved changes

### Requirement: OBS-SLI-04 -- Durable Provision Metrics and Removal of the In-Memory Histogram

Gateway provision duration and provision outcomes SHALL be derived from the durable
convergence tracking of OBS-SLI-01 and emitted by the API server, not by the
control-plane process. The API server SHALL export, from its metrics endpoint, the
same Prometheus series the operational dashboard already consumes:

| Series | Semantics |
| --- | --- |
| `gateway_provision_duration_seconds` (`_bucket`, `_count`, `_sum`) | One observation per gateway on its first successful transition to `Running`, computed from the persisted create-to-`Running` transition timestamps; a later recovery from `Degraded` to `Running` SHALL NOT add an observation; explicit bucket boundaries SHALL cover 1 second through 15 minutes |
| `gateway_provision_outcomes_total{outcome="success\|failure"}` | One terminal outcome per gateway: `success` on the first `Running`, `failure` on the first `Failed`; a `Degraded`-to-`Running` recovery SHALL NOT add an outcome |

Because the one-per-gateway claim is now the durable persisted state of OBS-SLI-01,
the control plane SHALL remove its in-memory provision-observation state (the
`observedGatewayProvisions` claim map) and SHALL NOT emit `gateway.provision.duration`
or `gateway.provision.outcomes` over OTLP. The provision series SHALL NOT contain a
Gateway identifier. The operational dashboard provision-time
(`platform/gateway-provision-time.spec.md`) and provision-reliability
(`platform/gateway-provision-outcomes.spec.md`) features consume these series
unchanged through the web-console BFF.

#### Scenario: OBS-SLI-04.01 -- Provision duration recorded once from durable timestamps

- GIVEN a Gateway that reaches `Running` for the first time
- WHEN the API server records the `Running` transition
- THEN `gateway_provision_duration_seconds` SHALL record one observation from the persisted create-to-`Running` duration
- AND a later `Degraded`-to-`Running` recovery SHALL NOT add another observation

#### Scenario: OBS-SLI-04.02 -- Control plane no longer keeps in-memory provision state

- GIVEN a control-plane process that has recorded provision outcomes
- WHEN the control-plane process restarts
- THEN the provision counts SHALL be unaffected because they are owned by the API server
- AND the control plane SHALL NOT maintain an in-process one-per-gateway provision claim

### Requirement: OBS-SLI-05 -- Honest Reconciliation-Lag Metric

The control-plane metric currently named `hypershell_reconciliation_lag_seconds`
SHALL measure what its name states: reconciliation lag, defined as the time from
when a Gateway's desired state last changed (its `generation` advanced) until the
control plane starts the reconcile that first acts on that generation. The metric
name, its specification, and the value it records SHALL agree.

The metric SHALL NOT record the elapsed processing time of a reconcile; that wall
time is already recorded by `reconcile.duration` (OBS-CPL-07). The lag metric SHALL
carry only the bounded `resource.kind` attribute and SHALL NOT carry a resource
identifier.

#### Scenario: OBS-SLI-05.01 -- Lag measures generation-change to reconcile start

- GIVEN a Gateway whose `generation` advanced at time T
- WHEN the control plane starts the reconcile that first observes that generation at time T plus 2 seconds
- THEN `hypershell_reconciliation_lag_seconds` SHALL record approximately 2 seconds
- AND it SHALL NOT record the duration the reconcile itself takes to complete

#### Scenario: OBS-SLI-05.02 -- Lag is distinct from reconcile duration

- GIVEN a reconcile that starts promptly but takes 5 seconds to complete
- WHEN both metrics are recorded
- THEN `reconcile.duration` SHALL reflect the 5-second processing time
- AND `hypershell_reconciliation_lag_seconds` SHALL NOT reflect that processing time

### Requirement: OBS-SLI-06 -- Oldest Unconverged Generation Gauge

The control plane SHALL export a gauge reporting the age of the oldest Gateway
whose `observed_generation < generation`, so operators can see convergence
backlog building before any change breaches its 10-minute budget:

| Metric | Type | Unit | Description |
| --- | --- | --- | --- |
| `hypershell_oldest_unconverged_generation_age_seconds` | Gauge | `s` | Age, in seconds, of the oldest Gateway whose observed generation trails its desired generation |

The gauge SHALL report `0` when every Gateway is converged. It SHALL carry only
bounded attributes (at most a `resource.kind` and the `hypershell.cluster_id`
attribute already used by other control-plane gauges, OBS-CPL-07) and SHALL NOT
carry a Gateway identifier.

#### Scenario: OBS-SLI-06.01 -- Gauge reports the oldest backlog age

- GIVEN two Gateways are unconverged, the oldest having been unconverged for 180 seconds
- WHEN the gauge is scraped
- THEN `hypershell_oldest_unconverged_generation_age_seconds` SHALL report approximately 180

#### Scenario: OBS-SLI-06.02 -- Gauge is zero when all converged

- GIVEN every Gateway has `observed_generation == generation`
- WHEN the gauge is scraped
- THEN the gauge SHALL report `0`

### Requirement: OBS-SLI-07 -- SLO-Aligned API Latency Histogram Buckets

The platform SHALL be able to compute, without interpolation, the fraction of read
requests served under 500 ms and the fraction of write requests served under 2 s.
The API request-duration histogram used for the API latency SLO SHALL therefore
have explicit bucket boundaries that include `0.5` and `2` seconds exactly.

The platform SHALL standardize the API latency SLO on the OpenTelemetry
`http.server.request.duration` histogram that the API server already emits
(`platform/api-server-observability.spec.md` API-OBS-05), because its bucket
boundaries are owned by HyperShell code, whereas the framework's
`api_inbound_request_duration` buckets are owned by the upstream rh-trex-ai
dependency. The API server SHALL register explicit bucket boundaries for
`http.server.request.duration` through an OpenTelemetry view, and those boundaries
SHALL include `0.025, 0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10` seconds. Deploy and
local-development configuration SHALL route that histogram into the Prometheus
instance the web-console BFF reads (exported as
`http_server_request_duration_seconds_*`), so the reliability dashboard
(`platform/api-reliability-metrics.spec.md` OBS-REL-08) can read the SLO series.

#### Scenario: OBS-SLI-07.01 -- Buckets include the SLO thresholds exactly

- GIVEN the API server emits `http.server.request.duration`
- WHEN the histogram view is registered
- THEN its explicit bucket boundaries SHALL include `0.5` and `2` seconds
- AND the fraction of requests under 500 ms and under 2 s SHALL be computable from a bucket edge without interpolation

#### Scenario: OBS-SLI-07.02 -- SLO series reaches Prometheus

- GIVEN deploy and local-development metrics configuration
- WHEN the API server serves requests
- THEN `http_server_request_duration_seconds_bucket` SHALL be scrapeable from the BFF Prometheus instance

### Requirement: OBS-SLI-08 -- Read and Write Request Classification

The API latency histogram SHALL carry a bounded attribute that classifies each
request as a read or a write, so the SLO can apply the 500 ms read target and the
2 s write target separately. The classification SHALL be derived from the request
method: safe methods (`GET`, `HEAD`, `OPTIONS`) classify as a read, and all other
methods (`POST`, `PUT`, `PATCH`, `DELETE`) classify as a write.

The classification attribute SHALL be bounded to the two values `read` and `write`
and SHALL NOT introduce a per-route or per-identifier label that grows with fleet
size. It SHALL NOT replace the existing status-code attribute the reliability
dashboard already uses.

#### Scenario: OBS-SLI-08.01 -- GET classifies as a read

- GIVEN a `GET /api/hypershell/v1/gateways/{id}` request
- WHEN the latency histogram records the request
- THEN the sample SHALL carry the request-class attribute value `read`

#### Scenario: OBS-SLI-08.02 -- POST classifies as a write

- GIVEN a `POST /api/hypershell/v1/gateways` request
- WHEN the latency histogram records the request
- THEN the sample SHALL carry the request-class attribute value `write`

### Requirement: OBS-SLI-10 -- Templated Route Attribute for User-Scoped Latency

The API latency histogram (`http.server.request.duration`) SHALL carry the matched
route as a bounded `http.route` attribute whose value is the templated route (for
example `/api/hypershell/v1/gateways/{id}`), never the raw request path with its
concrete resource identifiers. A request that matches no registered route SHALL
record no `http.route` attribute. The attribute cardinality is therefore bounded by
the number of registered routes. This is the latency-metric counterpart of the span
`http.route` contract in `platform/api-server-observability.spec.md` (API-OBS-05,
API-OBS-06): both the latency histogram sample and the request span carry the same
templated route so traces and the latency SLO agree.

Because the histogram carries the templated route, the API latency SLO SHALL be
able to count only user REST routes. It SHALL exclude the OpenAPI document routes
(`/api/hypershell/v1/openapi*`) and the managed-cluster registration route
(`/api/hypershell/v1/managed_clusters/registration`), so the latency SLO measures
user REST traffic only and not platform heartbeat, registration, or OpenAPI
traffic. This is the same user-REST-traffic scope the API availability measurement
uses. The SLO rules that apply this scope live in the GitOps repository and are out
of scope here (see Purpose); this requirement owns only the route attribute that
makes the user-route scoping possible.

The `http.route` attribute SHALL complement, and SHALL NOT replace, the read/write
request-class attribute (OBS-SLI-08) or the response-status-code attribute the
reliability dashboard already consumes (OBS-REL-08).

#### Scenario: OBS-SLI-10.01 -- Templated route recorded on the latency histogram

- GIVEN a `GET /api/hypershell/v1/gateways/{id}` request that matches a registered route
- WHEN the latency histogram records the request
- THEN the sample SHALL carry `http.route` with the templated value `/api/hypershell/v1/gateways/{id}`
- AND the sample SHALL NOT carry the raw path with the concrete identifier

#### Scenario: OBS-SLI-10.02 -- Unmatched path carries no route attribute

- GIVEN a request whose path matches no registered route
- WHEN the latency histogram records the request
- THEN the sample SHALL NOT carry an `http.route` attribute

#### Scenario: OBS-SLI-10.03 -- Latency SLO counts only user routes

- GIVEN latency samples for user routes and for `/api/hypershell/v1/openapi` and `/api/hypershell/v1/managed_clusters/registration`
- WHEN the API latency SLO selects its good and bad events
- THEN it SHALL count only the user-route samples
- AND it SHALL exclude the OpenAPI and registration routes, so it measures user REST traffic only

### Requirement: OBS-SLI-11 -- Verified Caller Class Attribute for User-Only API Indicators

The API latency histogram (`http.server.request.duration`) SHALL carry a bounded
`hypershell.caller.class` attribute on every sample, exported to Prometheus as the
`hypershell_caller_class` label. The attribute SHALL have exactly two values:

| Value | Meaning |
| --- | --- |
| `synthetic` | The request carries an access token that the API server verified for this request (signature, issuer, audience, and expiry, as `platform/oidc-integration.spec.md` defines), and that token's `azp` claim is a string equal to one entry of the configured synthetic client list |
| `user` | Every other request, including a request with no token, a token the API server did not verify, a token whose claims cannot be read, an `azp` claim that is missing, not a string, or not in the list, and every request when the list is empty |

When the API server cannot prove that a request came from a configured synthetic
client, it SHALL classify the request as `user`, so user traffic is never hidden
from the user API indicators by mistake.

The API server SHALL derive the caller class only from the claims of the token that
its authentication verified for the request. The `azp` claim names the OAuth client
the token was issued to (`platform/oidc-integration.spec.md`), and Keycloak sets it,
so a caller cannot choose it without a valid token for that client. Request
headers, the user agent, the source address, query parameters, and any token claims
read without verification SHALL have no effect on the caller class, because a
caller controls them and could use them to hide or inflate traffic.

The attribute SHALL record only the class. The raw `azp`, `sub`, username, or any
other identity value SHALL NOT appear as a metric attribute
(`platform/api-server-observability.spec.md` API-OBS-06), so the attribute adds
exactly two values of cardinality and no identity data.

The synthetic client list is configuration, not code:

| Setting | Values | Required | Default | Meaning |
| --- | --- | --- | --- | --- |
| `HYPERSHELL_SYNTHETIC_CLIENT_IDS` | Comma-separated OAuth client IDs | No | `hypershell-prober` | The `azp` values whose verified tokens classify as `synthetic` |

The API server SHALL read `HYPERSHELL_SYNTHETIC_CLIENT_IDS` once at startup. It
SHALL trim white space around each entry and ignore empty entries. When the setting
is omitted, the list SHALL be `hypershell-prober`, the outside prober's Keycloak
client (`platform/outside-prober.spec.md` OBS-PRB-06). When the setting is present
and contains no entries (for example, the empty string), the list SHALL be empty
and every request SHALL classify as `user`. The repository API server deployment
(`deploy/base/applications/api-server.yaml`) SHALL set
`HYPERSHELL_SYNTHETIC_CLIENT_IDS` explicitly to `hypershell-prober`.

The user API indicators SHALL count only samples whose caller class is `user`: the
API latency objective (OBS-SLI-07, OBS-SLI-08, OBS-SLI-10) and the in-pod user
request outcomes of the API availability objective. The SLO rules that apply this
filter live in the GitOps repository and are out of scope here (see Purpose); this
requirement owns only the attribute that makes the filter possible.

The caller-class attribute SHALL complement, and SHALL NOT replace, the read/write
request-class attribute (OBS-SLI-08), the `http.route` attribute (OBS-SLI-10), or
the response-status-code attribute the reliability dashboard already consumes
(OBS-REL-08).

#### Scenario: OBS-SLI-11.01 -- Verified prober token classifies as synthetic

- GIVEN `HYPERSHELL_SYNTHETIC_CLIENT_IDS` is `hypershell-prober`
- AND a `GET /api/hypershell/v1/gateways` request carries an access token that the API server verifies, with `azp=hypershell-prober`
- WHEN the latency histogram records the request
- THEN the sample SHALL carry `hypershell.caller.class` with the value `synthetic`
- AND the sample SHALL NOT carry the `azp` or `sub` value of the token

#### Scenario: OBS-SLI-11.02 -- Verified user token classifies as user

- GIVEN `HYPERSHELL_SYNTHETIC_CLIENT_IDS` is `hypershell-prober`
- AND a `GET /api/hypershell/v1/gateways/{id}` request carries an access token that the API server verifies, with `azp=hypershell-frontend`
- WHEN the latency histogram records the request
- THEN the sample SHALL carry `hypershell.caller.class` with the value `user`

#### Scenario: OBS-SLI-11.03 -- Request without a token classifies as user

- GIVEN a `GET /api/hypershell/v1/openapi` request with no `Authorization` header
- WHEN the latency histogram records the request
- THEN the sample SHALL carry `hypershell.caller.class` with the value `user`

#### Scenario: OBS-SLI-11.04 -- Unverified token cannot select synthetic

- GIVEN `HYPERSHELL_SYNTHETIC_CLIENT_IDS` is `hypershell-prober`
- AND a request carries a token with `azp=hypershell-prober` that the API server did not verify (for example, a forged signature, a wrong issuer, or an expired token)
- WHEN the request produces a latency sample
- THEN the sample SHALL carry `hypershell.caller.class` with the value `user`

#### Scenario: OBS-SLI-11.05 -- Unreadable or unlisted azp classifies as user

- GIVEN `HYPERSHELL_SYNTHETIC_CLIENT_IDS` is `hypershell-prober`
- AND a request carries a verified token whose `azp` claim is missing, is not a string, or is a client ID that is not in the list
- WHEN the latency histogram records the request
- THEN the sample SHALL carry `hypershell.caller.class` with the value `user`

#### Scenario: OBS-SLI-11.06 -- Spoofed headers have no effect

- GIVEN a request carries a verified access token with `azp=hypershell-frontend`
- AND the request also sends `User-Agent: hypershell-prober` and an `X-`-prefixed header that claims synthetic traffic
- WHEN the latency histogram records the request
- THEN the sample SHALL carry `hypershell.caller.class` with the value `user`

#### Scenario: OBS-SLI-11.07 -- Empty synthetic client list classifies every request as user

- GIVEN `HYPERSHELL_SYNTHETIC_CLIENT_IDS` is set to the empty string
- AND a request carries a verified access token with `azp=hypershell-prober`
- WHEN the latency histogram records the request
- THEN the sample SHALL carry `hypershell.caller.class` with the value `user`

#### Scenario: OBS-SLI-11.08 -- Omitted setting defaults to the prober client

- GIVEN `HYPERSHELL_SYNTHETIC_CLIENT_IDS` is not set
- AND a request carries a verified access token with `azp=hypershell-prober`
- WHEN the latency histogram records the request
- THEN the sample SHALL carry `hypershell.caller.class` with the value `synthetic`

#### Scenario: OBS-SLI-11.09 -- User API indicators count only user samples

- GIVEN latency samples on user routes with `hypershell_caller_class="synthetic"` and with `hypershell_caller_class="user"`
- WHEN the API latency objective or the in-pod user request outcomes of the API availability objective select their good and bad events
- THEN they SHALL count only the `user` samples
- AND a failure seen only by users SHALL NOT be diluted by successful synthetic requests

### Requirement: OBS-SLI-09 -- Build Revision and Build Time for Every Production Component

Every production component SHALL report the source commit it was built from and the
time it was built, so the lead-time dashboard can attribute a running instance to a
merged commit. This covers the API server, the control plane, and the web console.

The build pipelines SHALL supply both values:

- Konflux pipelines (`.tekton/*`) SHALL pass a `BUILD_TIME` build argument in
  addition to the `GIT_VERSION` build argument they already pass, for every
  component whose image build consumes it. A production image SHALL NOT report
  `build_time: "unknown"`.
- Each component `Dockerfile` SHALL accept `GIT_VERSION` and `BUILD_TIME` build
  arguments and SHALL stamp them into the built artifact (for Go binaries, through
  linker flags; for the web console, through its served runtime configuration).

Each component SHALL expose the stamped revision and build time:

- The API server SHALL continue to serve its revision and build time from
  `GET /api/hypershell/v1/metadata`.
- The control plane SHALL expose its revision and build time (for example on its
  existing health endpoint), following the pattern already used by the
  fleet-dashboard component.
- The web console SHALL expose its revision and build time to operators.

The reported revision SHALL be the Git commit the component was built from.
Reported build metadata SHALL NOT contain secrets.

#### Scenario: OBS-SLI-09.01 -- Konflux passes build time

- GIVEN a Konflux build of a production component image whose Dockerfile consumes `BUILD_TIME`
- WHEN the image is built
- THEN the pipeline SHALL pass a non-empty `BUILD_TIME` build argument
- AND the running component SHALL NOT report `build_time: "unknown"`

#### Scenario: OBS-SLI-09.02 -- Every production component reports its commit

- GIVEN deployed API server, control plane, and web console instances
- WHEN an operator queries each component's revision surface
- THEN each SHALL report the source commit it was built from and its build time

## Assumptions

- The Gateway `generation` / `observed_generation` convergence latch
  (PLAT-DATA-06) is authoritative and is the only input the API server needs to
  decide convergence, together with the persisted `phase` and finalization state.
- The API server metrics endpoint (OBS-GMD-02) and its database-backed collector
  pattern (OBS-GMD-01) are the delivery mechanism for the new durable metrics.
- Prometheus retention for the BFF instance is sufficient for the SLO windows the
  GitOps rules evaluate; those windows and rules are out of scope here.

## Design Decisions

| Decision | Rationale |
| --- | --- |
| API server owns change convergence, not the control plane | PostgreSQL is the source of truth; durable state is the only way a restart cannot change counts, and the convergence latch already lives there (PLAT-DATA-06). |
| Exactly one outcome per change, split at the 10-minute deadline | Satisfies "count a change as bad when its 10 minutes end" and "if it converges later, do not count it again" without double counting; keeps the SLO good/bad partition exact. |
| Clock from the first unconverged change, newest generation judged | Prevents repeated edits from resetting the budget while still measuring the latest desired state. |
| Resolve stalled changes with a single-owner deadline sweep, counting in the same transaction | A change that stalls with no further write must still be counted at its deadline; running the sweep on one replica and incrementing the durable counter in the same transaction keeps outcomes exactly-once across replicas and restarts. |
| Keep the same provision Prometheus series names, move the producer | Removes the in-memory histogram the issue calls out while leaving the operational dashboard provision-time and provision-reliability contracts unchanged. |
| Standardize latency on OTel `http.server.request.duration` | Its buckets are owned by HyperShell code; the framework `api_inbound_request_duration` buckets are owned by the upstream rh-trex-ai dependency and cannot be set here. |
| Read/write class from the HTTP method | Bounded two-value label; reads and writes have different SLO thresholds; avoids a per-route cardinality explosion. |
| Templated `http.route` on the latency histogram, scoped to user routes | The latency histogram reached the hub with method and class only, so a method-only SLI measured platform traffic rather than the handful of user requests per hour. The templated route is bounded by the registered routes and lets the SLO exclude OpenAPI and registration traffic, matching the user-REST-traffic scope of the availability measurement. |
| Caller class from the verified token's `azp`, two values, default `user` | The outside prober makes most production API requests, so a user-only failure would be diluted many times over. Keycloak sets `azp` to the client the token was issued to, so it identifies the prober with no Keycloak change. Reading it only from the verified token means a caller cannot hide or inflate traffic with headers or forged claims. A bounded two-value label keeps cardinality and privacy safe, and defaulting to `user` makes every mistake visible instead of hidden. |
| Build time as a build argument plus a stamped artifact | Separates configuration from code; the api-server and fleet-dashboard already follow this pattern, so the control plane and web console adopt it rather than inventing a new one. |

## Primary Basis

- `platform/data-model.spec.md` (PLAT-DATA-06 generation tracking)
- `platform/gateway-phase-vocabulary.spec.md` (canonical phase set)
- `platform/gateway-deletion-finalization.spec.md` (GTW-DEL-01 finalization, GTW-DEL-10 finalization-completion signal)
- `platform/control-plane-observability.spec.md` (OBS-CPL-07 reconcile metrics)
- `platform/control-plane-reconciliation-metrics.spec.md` (OBS-CRM-01 metric types)
- `platform/gateway-metrics-dashboard.spec.md` (OBS-GMD-01, OBS-GMD-02 API-server collector and metrics endpoint)
- `platform/api-server-observability.spec.md` (API-OBS-05 request metrics, API-OBS-06 telemetry privacy and cardinality)
- `platform/oidc-integration.spec.md` (`azp` names the OAuth client a token was issued to; token verification)
- `platform/outside-prober.spec.md` (OBS-PRB-06 prober identity and its `hypershell-prober` Keycloak client)
- `platform/api-reliability-metrics.spec.md` (OBS-REL-08 latency series)
- `platform/managed-cluster-registration.spec.md` (registration route excluded from the user-route latency scope)
- `platform/gateway-provision-time.spec.md` and `platform/gateway-provision-outcomes.spec.md` (provision series consumers)
