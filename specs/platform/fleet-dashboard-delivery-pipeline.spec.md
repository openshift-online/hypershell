# Fleet Dashboard Delivery Pipeline

**ID prefix:** FLT-DLV
**Status:** Proposed
**Applies to:** `components/fleet-dashboard` BFF (new delivery source and `/api/delivery`), `packages/fleet-dashboard-ui` (new Delivery tab, delivery domain, route codec)
**Mockup:** `specs/web-console/mockups/dashboards/delivery-pipeline/` (`Mockups/Glass/Delivery` and `Parity/Glass/Delivery` stories)

## Purpose

The Glass fleet dashboard shows promotion state from the moment a release bundle
reaches the GitOps repository's `main` branch. The steps before that are not
visible: a change merges into the application repository, Konflux builds the
changed component images, a policy check runs, a managed release publishes the
images, a final pipeline publishes a release bundle, and an automated pull
request moves the bundle into the GitOps repository. An operator who merged a
change cannot tell, from Glass, whether it is building, waiting, failed, or
already running in an environment.

This specification adds a **Delivery** tab. It follows each application change
from merge to the last promotion environment as one row of stages. For every
change in flight, it says what is happening now and what should happen next,
with typical durations taken from recent history. It reads only sources the
dashboard can already reach, with credentials it already holds.

### Terms

| Term | Meaning |
| --- | --- |
| Change | One first-parent commit on the application repository's `main` branch (`FD_UPSTREAM_REPO`), usually a squash-merged pull request. |
| Bundle | A Konflux release bundle: an OCI artifact tagged `release-bundle-*` whose `bundle.json` records six component images and `manifests.git.revision`. |
| Locked bundle | A bundle that appears in the GitOps release-lock history. The shared bundle source (SDLC-VIS-03) resolves it. |
| Pending bundle | A published bundle newer than the newest locked bundle. It is not in the GitOps repository yet. |
| First bundle | The bundle whose compare window places the change, as the shared bundle source defines it (SDLC-VIS-03, SDLC-VIS-22). |
| Contains | A bundle contains a change when it is the change's first bundle or any newer bundle. |
| Environment group | The promotion environments that share one `delivery.hypershell.app/env` label value (for example the int hub and its spokes). |
| Delivered | A change is delivered when every promotion environment's active release contains it. |

### Data reachability

| Stage | Source | Credential |
| --- | --- | --- |
| Merged | Application repository commits API | `FD_UPSTREAM_TOKEN_FILE` (existing) |
| Build, Policy check | Check runs on the change's commit, reported by Konflux Pipelines as Code | `FD_UPSTREAM_TOKEN_FILE` (existing) |
| Release | Locked bundles: the shared bundle source (release lock and compare windows). Pending bundles only: tag list and `bundle.json` of `FD_DELIVERY_BUNDLE_REPOSITORY` (public registry API). Downloads: the matching application-repository GitHub release. | existing tokens; none for the public registry |
| GitOps update | Pull requests and release-lock history of `FD_GITHUB_REPO` | `FD_GITHUB_TOKEN_FILE` (existing) |
| Environment groups | The existing promotion source (`/api/promotion`): environments, active and proposed releases, gates, analysis | existing BFF service account |

The step between a passed policy check and a published bundle (the Konflux
managed release and its final pipeline) has no source the dashboard can read
without Konflux cluster access. The Release stage infers it (FLT-DLV-06).

## Requirements

### Requirement: FLT-DLV-01 -- Delivery tab

Glass SHALL provide a fourth top-level tab, **Delivery**, at path `/delivery`,
after Fleet Map, Agent SDLC, and E2E Runs. The tab SHALL use the shared Glass
shell and its tab navigation. Its page surface SHALL have a header region with
the title "Delivery", a one-sentence description, and a refresh action, and a
body region with the Now and next summary (FLT-DLV-10) followed by the change
table (FLT-DLV-03).

#### Scenario: FLT-DLV-01.01 -- Tab reachable by path

- GIVEN an authorized operator
- WHEN the operator opens `/delivery`
- THEN Glass SHALL select the Delivery tab
- AND the page SHALL show the Now and next summary above the change table

#### Scenario: FLT-DLV-01.02 -- Plane not configured

- GIVEN `FD_DELIVERY_BUNDLE_REPOSITORY` is not set
- WHEN the operator opens the Delivery tab
- THEN the body SHALL show an empty state stating that the delivery source is not configured
- AND the other tabs SHALL be unaffected

### Requirement: FLT-DLV-02 -- Change window

The BFF SHALL list changes newest first. The list SHALL contain every change
that is not yet delivered, followed by the most recent delivered changes, up to
`FD_DELIVERY_WINDOW` rows in total. When undelivered changes alone exceed the
window, the oldest undelivered changes SHALL be dropped and the payload SHALL
report how many were dropped. The BFF SHALL report dropped changes only when
the GitOps and promotion sources are known, because without them no change can
be known delivered. Each change SHALL carry its full and short commit
SHA, merge time, title, author, and pull request number when the title carries
a `(#n)` trailer.

#### Scenario: FLT-DLV-02.01 -- Undelivered first, then recent history

- GIVEN 4 undelivered changes and 40 delivered changes
- AND `FD_DELIVERY_WINDOW` is 30
- WHEN the BFF builds the delivery payload
- THEN it SHALL list the 4 undelivered changes and then the 26 newest delivered changes

#### Scenario: FLT-DLV-02.02 -- Window overflow is reported

- GIVEN 35 undelivered changes and `FD_DELIVERY_WINDOW` is 30
- WHEN the BFF builds the delivery payload
- THEN it SHALL list the 30 newest undelivered changes
- AND it SHALL report 5 omitted undelivered changes

### Requirement: FLT-DLV-03 -- Stages in a fixed order

Each change SHALL be shown as one table row with one column per stage, in this
order: **Merged**, **Build**, **Policy check**, **Release**, **GitOps update**,
then one column per environment group. Environment-group columns SHALL be
derived at runtime from the promotion payload: groups are ordered by the
earliest promotion-graph layer of their environments, and each column is
labelled with the group's label value. The table SHALL be keyboard navigable
and SHALL be the accessible representation of the pipeline; no information may
be conveyed only by a graphic.

#### Scenario: FLT-DLV-03.01 -- Columns follow the promotion graph

- GIVEN a promotion graph whose roots carry env label `int`, whose next layer carries `stage`, and whose last layer carries `prod`
- WHEN the Delivery tab renders
- THEN the environment-group columns SHALL read Int, Stage, Prod in that order

#### Scenario: FLT-DLV-03.02 -- New environment group appears without code change

- GIVEN a new promotion environment with a new env label value is added to the promotion strategy
- WHEN the BFF next refreshes
- THEN the Delivery tab SHALL show an additional column for that group

### Requirement: FLT-DLV-04 -- Stage state vocabulary

Every stage cell SHALL show exactly one state from this set, as an icon paired
with a text label, never color alone:

| State | Meaning | Treatment |
| --- | --- | --- |
| Not started | An earlier stage has not finished. | gray, no icon emphasis |
| Waiting | The stage can start but is waiting on a named condition. | gray pending icon plus the condition |
| Running | The stage is in progress. | info in-progress icon plus elapsed time |
| Passed | The stage finished successfully. | success check icon plus finish time |
| Failed | The stage finished unsuccessfully. | danger icon plus finish time |
| Not needed | The stage does not apply to this change. | gray, with the reason |
| Unknown | The source for this stage is unavailable. | gray question icon |

A Running or Waiting stage that has lasted longer than twice its typical
duration (FLT-DLV-09) SHALL add a warning treatment reading "Slower than usual".
Unknown SHALL never be rendered as Passed.

#### Scenario: FLT-DLV-04.01 -- Slow stage is flagged

- GIVEN the Build stage's typical duration is 10 minutes
- AND a change's Build stage has been Running for 23 minutes
- WHEN the row renders
- THEN the Build cell SHALL show Running with "23 min"
- AND it SHALL show a warning "Slower than usual"

### Requirement: FLT-DLV-05 -- Build and policy check from Konflux check runs

The BFF SHALL read the check runs on each listed change's commit from the
application repository, keeping only check runs created by the GitHub App slug
`FD_DELIVERY_CHECK_APP`. A check run whose name ends in `-on-push` SHALL be a
component build; its component is the name segment after the last `/ ` with
the `-on-push` suffix removed. A check run whose name contains
`-enterprise-contract /` SHALL be a policy check; its component is the last
name segment. The Build stage SHALL be Running while any component build is
queued or in progress, Failed when any finished unsuccessfully, and Passed when
all succeeded. The Policy check stage SHALL follow the same rule over the policy
check runs; a `neutral` conclusion SHALL count as passed, matching how Konflux
reports a successful policy evaluation. Policy check runs SHALL count only for
components whose build succeeded on the same commit; Konflux also reports policy
checks for components it did not build, and those SHALL be ignored. A build or
policy check run that concluded `cancelled`, `skipped`, or `stale` SHALL be
ignored, because a newer run replaced it. When every build on a commit was
replaced this way, Build and Policy check SHALL be Not needed with the reason
"Build replaced". When a change has no component build check run, Build and
Policy check SHALL be Not needed with the reason "No component changed", and
the change SHALL ride with its first bundle.

#### Scenario: FLT-DLV-05.01 -- One component rebuilt

- GIVEN a change whose commit has `on-push` check runs only for the control-plane component
- AND that check run is in progress
- WHEN the BFF builds the delivery payload
- THEN the change's Build stage SHALL be Running with components `[control-plane]`

#### Scenario: FLT-DLV-05.02 -- Documentation-only change

- GIVEN a change whose commit has no `on-push` check run from `FD_DELIVERY_CHECK_APP`
- WHEN the row renders
- THEN Build and Policy check SHALL read Not needed with "No component changed"
- AND the Release stage SHALL follow the change's first bundle

#### Scenario: FLT-DLV-05.03 -- Failed build

- GIVEN a change whose web-console `on-push` check run concluded `failure`
- WHEN the row renders
- THEN the Build cell SHALL show Failed with the failed component named
- AND every later stage of that change SHALL read Not started until a later bundle contains the change

### Requirement: FLT-DLV-06 -- Release from the shared bundle source

The delivery source SHALL take locked bundles, their aliases, revisions, and
which changes they contain from the shared bundle source that already feeds the
Fleet Map and the Agent SDLC Shipped matrix (SDLC-VIS-03, SDLC-VIS-22,
FLT-BPR). It SHALL NOT re-read the release lock or define its own rule for
which changes belong to a locked bundle.

The registry SHALL be read only to find pending bundles: `release-bundle-*`
tags of `FD_DELIVERY_BUNDLE_REPOSITORY` newer than the newest locked bundle. For
each pending bundle, the source SHALL read `bundle.json` once by digest and
cache it permanently, because bundle content is immutable. It SHALL place
changes in the pending bundle with the shared source's compare-window rule,
using the newest locked bundle's revision as the base and the pending bundle's
`manifests.git.revision` as the head, through the shared compare cache. When a
pending bundle becomes locked, the shared source's placement SHALL replace the
pending placement. A change that no compare window holds (a change older than
the oldest known bundle, or one without a `(#n)` trailer) SHALL be placed by
position: its first bundle is the oldest bundle whose revision is the change or
a later commit on the application repository's linear `main` history.

A change's Release stage SHALL be Passed when a locked or pending bundle
contains it, showing the first bundle's alias and publication time. When the
change's Policy check has passed and no bundle contains it yet, the Release
stage SHALL be Running and SHALL be labelled "Releasing (inferred)", because
the Konflux managed release is not observable. When the matching
application-repository GitHub release is still a draft or missing after the
bundle is published, the change details (FLT-DLV-11) SHALL show "Downloads not
published yet". This SHALL NOT change the Release stage state, because GitOps
consumes the registry bundle. While the GitOps source is Unknown, a change that
no bundle contains SHALL have an Unknown Release stage, not an inferred one,
because locked placement cannot be read.

#### Scenario: FLT-DLV-06.01 -- Release inferred after the policy check

- GIVEN a change whose Policy check passed 2 minutes ago
- AND no bundle contains the change
- WHEN the row renders
- THEN the Release cell SHALL read "Releasing (inferred)" with "2 min"

#### Scenario: FLT-DLV-06.02 -- Change rides a later bundle

- GIVEN a change C1 whose Build was Not needed
- AND a later change C2 whose bundle B is the first bundle whose compare window holds C1 and C2
- WHEN the BFF builds the payload
- THEN C1's Release stage SHALL be Passed with first bundle B

#### Scenario: FLT-DLV-06.03 -- Pending placement matches the shared source

- GIVEN pending bundle B places change C
- WHEN B enters the GitOps release lock and the shared bundle source resolves it
- THEN C's first bundle SHALL still be B
- AND the Delivery tab and the Agent SDLC Shipped matrix SHALL list C under the same bundle

#### Scenario: FLT-DLV-06.04 -- No second membership rule

- GIVEN a locked bundle B
- WHEN the delivery source needs the changes B contains
- THEN it SHALL read them from the shared bundle source
- AND it SHALL NOT call the compare API or read the release lock for B itself

### Requirement: FLT-DLV-07 -- GitOps update stage

The GitOps update stage SHALL track the change's first bundle into the GitOps
repository `main` branch. It SHALL be Passed when a commit on `FD_GITHUB_REPO`
`main` sets the release lock to a bundle that contains the change, showing that
commit's merge time and pull request. When `FD_DELIVERY_RELEASE_PR_BRANCH` is set
and an open pull request from that branch proposes a bundle that contains the
change, the stage SHALL be Running and SHALL show the pull request's required
check state. When the bundle is published and no such pull request or commit
exists yet, the stage SHALL be Waiting with "Waiting for the bundle update pull
request". When `FD_DELIVERY_RELEASE_PR_BRANCH` is not set, the stage SHALL go
from Waiting to Passed without a Running state.

#### Scenario: FLT-DLV-07.01 -- Bundle pull request open

- GIVEN `FD_DELIVERY_RELEASE_PR_BRANCH` is set
- AND an open pull request from that branch proposes bundle B
- AND B contains change C
- WHEN C's row renders
- THEN the GitOps update cell SHALL show Running with the pull request number and its check state

### Requirement: FLT-DLV-08 -- Environment-group stages

For each environment group, the stage SHALL be Passed when every environment in
the group has an active release that contains the change, showing the time the
last one was promoted. The promotion time of an environment SHALL be the merge time of the first
promotion in that environment's history (FLT-DLV-18) whose bundle contains the
change. When the history does not reach back that far, the time SHALL be
unknown and omitted. A group that is Passed but had a failed attempt in its
history SHALL show "n failed attempts" next to its time. While only some environments in the group contain the
change, the stage SHALL be Running and SHALL show "n of m". When none contains it
yet, the stage SHALL be Waiting with the blocking condition taken from the
promotion payload, in this order of precedence:

1. "Waiting for hydration" when the change is on GitOps `main` but an
   environment's proposed release does not contain it yet.
2. "Waiting for <groups>" naming the upstream environment groups whose
   promotion gates have not passed for a release that contains the change.
3. "Release analysis running" or "Health check running" when the group's own
   gate check is in progress for a release that contains the change.

A Failed gate for a release that contains the change SHALL make the stage Failed
and name the gate.

An environment whose promotion history (FLT-DLV-18) shows a passed promotion of
a bundle containing the change SHALL count as Passed for that change. The gates
of a newer bundle that is active there SHALL NOT change it: they belong to the
rows of the changes that bundle adds. Without that history, the gates of the
active release decide, as above.

#### Scenario: FLT-DLV-08.01 -- Waiting on the upstream group

- GIVEN change C is active in every Int environment
- AND the Int hub's release analysis is in progress for a release containing C
- WHEN C's row renders
- THEN the Int cell SHALL read Running with "Release analysis running"
- AND the Stage cell SHALL read Waiting with "Waiting for Int"

#### Scenario: FLT-DLV-08.02 -- Partially promoted group

- GIVEN the Prod group has 4 environments and 1 of them is active on a release containing C
- WHEN C's row renders
- THEN the Prod cell SHALL read Running with "1 of 4"

#### Scenario: FLT-DLV-08.03 -- A newer bundle's checks do not reopen a passed group

- GIVEN change C passed every Int environment in bundle B1
- AND a newer bundle B2, which also contains C, is active in Int with its release analysis running
- WHEN C's row renders
- THEN the Int cell SHALL read Passed
- AND the row of a change first shipped in B2 SHALL read Running with "Release analysis running"

### Requirement: FLT-DLV-09 -- Typical durations

The BFF SHALL compute a typical duration for each stage as the median of that
stage's durations over the completed changes in its history window, using at
least 3 samples. A stage's duration is the time from the end of the previous
stage to the end of this stage. Stages with fewer than 3 samples SHALL have no
typical duration, and the UI SHALL then omit "usually" text and the slow-stage
warning for that stage.

#### Scenario: FLT-DLV-09.01 -- Too little history

- GIVEN only 2 completed changes have a Policy check duration
- WHEN the Policy check stage renders as Running
- THEN it SHALL show elapsed time only, with no "usually" text

### Requirement: FLT-DLV-10 -- Now and next summary

Above the change table, the tab SHALL show a **Now and next** summary with one
row per undelivered change, newest first, at most 3 rows, and a link to the full
table when more exist. Each row SHALL name the change (short SHA, pull request,
title) and SHALL state:

- **Now:** the earliest stage that is not Passed or Not needed, its state, its
  elapsed time, and its typical duration when known.
- **Next:** the following stage, with an expected time equal to the remaining
  typical duration of the current stage (its typical duration minus its elapsed
  time, never below zero) plus the typical duration of the next stage, written
  as "usually about <time>". When either typical duration is unknown, the
  expected time SHALL be omitted. When the current stage is Failed, Next SHALL
  state what restarts the pipeline instead (for a failed build: "A new change
  to main starts a new build").

The summary icon SHALL follow the highest severity among its rows: Failed over
"Slower than usual" over in-progress. When every listed change is delivered, the
summary SHALL read "Everything on main is delivered" with the newest change's
delivery time and its total time from merge.

#### Scenario: FLT-DLV-10.01 -- Change building

- GIVEN change `a15cc91` (#997) has Build Running for 6 minutes with a typical Build duration of 10 minutes
- AND the typical Policy check duration is 3 minutes
- WHEN the summary renders at 15:17
- THEN the row SHALL read Now "Building control-plane - 6 min, usually 10 min"
- AND Next "Policy check - usually about 15:24"

#### Scenario: FLT-DLV-10.02 -- Nothing in flight

- GIVEN every listed change is delivered
- WHEN the summary renders
- THEN it SHALL read "Everything on main is delivered" with the newest change's delivery time and time from merge

#### Scenario: FLT-DLV-10.03 -- Failure takes precedence

- GIVEN one undelivered change has a Failed Build and another has a slow GitOps update
- WHEN the summary renders
- THEN the summary heading SHALL use the danger icon

### Requirement: FLT-DLV-11 -- Change details drawer

Selecting a change SHALL open a details drawer that lists every stage with its
state, start and end time, and duration; the per-component builds and policy
checks with links to their GitHub check-run pages; the first bundle with its
alias, digest, publication time, and a link to its GitHub release (or "Downloads
not published yet"); the GitOps update pull request link; and, per environment
group, each environment with its state and a link that opens the Fleet Map with
that node selected, followed by that environment's promotion history for the
change (FLT-DLV-18). The drawer SHALL offer a link that opens the Fleet Map with
the change's first bundle selected (`bundle` parameter, FLT-NAV-08). When the
first bundle is locked, the drawer SHALL also offer a link that opens the Agent
SDLC tab with that bundle's Shipped row expanded (`shiprow` parameter, the
bundle seed, SDLC-VIS-11). A pending bundle has no Shipped row yet, so that
link SHALL be omitted for it.

#### Scenario: FLT-DLV-11.01 -- From a change to its bundle on the map

- GIVEN the drawer is open for change C whose first bundle is B
- WHEN the operator follows "Show bundle on Fleet Map"
- THEN the Fleet Map SHALL open with bundle B selected

#### Scenario: FLT-DLV-11.02 -- From a change to its Shipped row

- GIVEN the drawer is open for change C whose first bundle B is locked
- WHEN the operator follows "Show bundle in Agent SDLC"
- THEN the Agent SDLC tab SHALL open with B's Shipped row expanded

#### Scenario: FLT-DLV-11.03 -- No Shipped link for a pending bundle

- GIVEN the drawer is open for change C whose first bundle is pending
- WHEN the drawer renders
- THEN it SHALL show the Fleet Map link
- AND it SHALL NOT show the Agent SDLC link

### Requirement: FLT-DLV-12 -- Delivery endpoint and settings

The BFF SHALL serve `GET /api/delivery`, admin-gated like the other `/api/*`
endpoints, from an in-memory cache refreshed every `FD_REFRESH_DELIVERY`. The
payload SHALL carry `generatedAt`, the ordered stage and environment-group
definitions, the changes with their stage states, the typical durations, the
omitted-change count, and a per-source `stale` and `error` flag for `upstream`,
`registry`, `gitops`, `promotion`, and `history`.

Per refresh, the BFF SHALL make at most one commits-list call, one check-runs
call per change whose check runs have not settled, one environment-branch
commits call per environment whose active hydrated commit changed since the
last read, one hydration-metadata read and one check-runs call per new
promotion it has not yet settled (FLT-DLV-18),
one registry tag-list call, one `bundle.json` read per new pending bundle,
and one pull-request list call. Locked-bundle data SHALL come from the shared
bundle source's cache with no extra call. Completed check runs and pending
bundle contents SHALL be cached by full SHA or digest, because they are
immutable. Pending-bundle compare results SHALL use the shared compare cache.

| Setting | Required | Default | Meaning |
| --- | --- | --- | --- |
| `FD_DELIVERY_BUNDLE_REPOSITORY` | yes, to enable the plane | none | Registry repository that holds `release-bundle-*` tags. When unset, `/api/delivery` returns `enabled: false`. |
| `FD_DELIVERY_RELEASE_PR_BRANCH` | no | none | Head branch of the automated bundle update pull request in `FD_GITHUB_REPO`. When unset, the GitOps update stage has no Running state. |
| `FD_DELIVERY_CHECK_APP` | no | `red-hat-konflux` | GitHub App slug whose check runs are builds and policy checks. |
| `FD_DELIVERY_WINDOW` | no | `30` | Maximum changes in the payload. |
| `FD_REFRESH_DELIVERY` | no | `60s` | Cache refresh interval. |

The plane SHALL reuse `FD_UPSTREAM_REPO`, `FD_UPSTREAM_TOKEN_FILE`,
`FD_GITHUB_REPO`, and `FD_GITHUB_TOKEN_FILE`, and SHALL NOT require any new
credential or Kubernetes permission.

A change's check runs SHALL count as settled when the runs its stages use (its
component builds and the policy checks of built components) have all completed
and none failed, or, for a change with no build check run, once it was merged
more than one hour ago. A delivered change SHALL count as settled once none of
those runs failed. A change with a failed run SHALL never count as settled,
because Konflux can finish the same check run later with another conclusion.
Other check runs on the commit SHALL NOT affect settling.

#### Scenario: FLT-DLV-12.01 -- Completed checks are not re-read

- GIVEN change C's builds and policy checks all passed in a previous refresh
- AND another check run on C's commit is still queued or failed
- WHEN the BFF refreshes again
- THEN it SHALL NOT call the check-runs API for C

#### Scenario: FLT-DLV-12.02 -- A failed check is read again

- GIVEN change C's build check run read as failed in a previous refresh
- AND Konflux later finished the same check run as a success
- WHEN the BFF refreshes again
- THEN it SHALL read C's check runs again
- AND C's Build stage SHALL read Passed

### Requirement: FLT-DLV-13 -- Per-source degradation

A failure of one source SHALL affect only the stages it feeds. The tab SHALL
show one inline warning alert per stale or failed source, naming the source and
the affected stages, and SHALL keep every other stage live.

- **Stale source.** The BFF SHALL serve that source's last good data with
  `stale: true`. Affected cells keep their last known state and the alert
  states the time of the last good read.
- **Promotion history that has never succeeded.** Stage states SHALL NOT
  change; group promotion times and the drawer's promotion history SHALL be
  Unknown.
- **Registry, GitOps, or promotion source that has never succeeded.** The
  stages it feeds SHALL be Unknown. A registry failure SHALL affect only
  changes that no locked bundle contains, because locked placement comes from
  the shared bundle source. Without GitOps, no change can be placed in a
  locked bundle, so every environment state SHALL be Unknown, not Waiting.
- **Application repository that has never succeeded.** The change list cannot
  be built. The tab SHALL show the warning alert and an empty state in place
  of the summary and the table.

When `/api/delivery` itself fails, the tab SHALL show the standard danger alert
for an API error.

#### Scenario: FLT-DLV-13.01 -- Bundle registry unreachable

- GIVEN the registry source has never succeeded
- WHEN the tab renders
- THEN the Release cell of every change that no locked bundle contains SHALL read Unknown once its Policy check has passed
- AND a change that a locked bundle contains SHALL keep its Passed Release cell
- AND the Merged, Build, Policy check, GitOps update, and environment-group cells SHALL still render from their own sources
- AND a warning alert SHALL name the bundle registry and the Release stage

#### Scenario: FLT-DLV-13.02 -- Application repository stale

- GIVEN the application repository source last succeeded at 15:02 and its reads now fail
- WHEN the tab renders at 15:20
- THEN the change list and the Build and Policy check cells SHALL show their 15:02 state
- AND a warning alert SHALL state that build data is from 15:02

### Requirement: FLT-DLV-14 -- Addressable state

The Delivery tab SHALL encode the selected change as the `change` query
parameter (full commit SHA, push history) and the shared `bundle` parameter
when set, following the route codec, canonicalization, and history rules of
FLT-NAV-02 and FLT-NAV-06. A `change` that is not in the payload SHALL show the
table with no drawer and an informational alert that the change is outside the
current window.

#### Scenario: FLT-DLV-14.01 -- Deep link to a change

- GIVEN a URL `/delivery?change=<sha>` for a listed change
- WHEN the page loads
- THEN the drawer SHALL open for that change

### Requirement: FLT-DLV-15 -- Fleet-identity firewall

The delivery source and the Delivery tab SHALL obey the fleet-identity firewall
(operational-dashboard data architecture, section 3.5): environment groups,
instance names, repository slugs, registry paths, and branch names SHALL come
from settings or runtime payloads, never from source code, fixtures, or tests in
the application repository.

#### Scenario: FLT-DLV-15.01 -- No fleet names in source

- GIVEN the fleet-identity firewall check runs over `components/fleet-dashboard` and `packages/fleet-dashboard-ui`
- WHEN the delivery source and tab are added
- THEN the check SHALL pass without new exceptions

### Requirement: FLT-DLV-16 -- Approved mockup

The production Delivery tab SHALL match the approved mockup stories
`Mockups/Glass/Delivery` (design review) and `Parity/Glass/Delivery` (parity)
in `specs/web-console/mockups/dashboards/delivery-pipeline/`, state for state:
`InFlight`, `ChangeSelected`, `BuildFailed`, `SlowerThanUsual`,
`AllDelivered`, `PromotionRetried`, `SourceUnavailable`, `Loading`,
`NotConfigured`, and `Error`.
The production page SHALL pass `skills/build/ui-build-gate/SKILL.md`.

#### Scenario: FLT-DLV-16.01 -- Every mockup state has a production story

- GIVEN the parity stories above
- WHEN the production Delivery tab is implemented
- THEN a `Production/Glass/Delivery` story SHALL exist for each parity state name

### Requirement: FLT-DLV-17 -- Tests

The implementation SHALL include tests that cite these IDs in `Verifies:`
markers: BFF unit tests for check-run classification (FLT-DLV-05), first-bundle
selection including a change that rides a later bundle, and pending-bundle
placement that matches the shared bundle source once the bundle is locked
(FLT-DLV-06), environment-group states and waiting reasons
(FLT-DLV-08), median typical durations with fewer than 3 samples
(FLT-DLV-09), the per-refresh call budget (FLT-DLV-12), and per-source
degradation (FLT-DLV-13); UI domain tests for the Now and next text and
severity (FLT-DLV-10); and route-codec tests for `change` (FLT-DLV-14).

#### Scenario: FLT-DLV-17.01 -- Markers present

- GIVEN the implementation pull request
- WHEN the spec-ID marker check runs
- THEN every requirement listed above SHALL have at least one test citing it

### Requirement: FLT-DLV-18 -- Promotion history per environment

The delivery source SHALL show what happened to each change in each
environment at the time, not only where it runs now. It SHALL read each
environment's promotions from that environment's branch in `FD_GITHUB_REPO`
(the `branch` of the promotion payload, for example `env/<name>`): every merge
commit on the branch is one promotion, with its merge time and its pull
request. It SHALL find the bundle of each promotion by following that commit's
hydration metadata to the release lock (the shared lock resolver), and SHALL
cache the result, because a merged commit never changes.

Each promotion SHALL have an outcome:

- **Passed**, when every gate check run passed.
- **Failed**, when a gate check run failed.
- **Replaced**, when a newer promotion replaced it while a gate was still
  running.
- **Running**, for the active promotion while a gate is running.
- **Unknown**, when its gate check runs or its bundle cannot be read.

The gates are the commit statuses the promotion strategy defines for that
environment (the keys of the payload's active gates). For the active
promotion, gate results SHALL come from the promotion payload. For a replaced
promotion, they SHALL come from the GitHub check runs named after each gate
key on that promotion's own merge commit: the latest run per name. The BFF
SHALL NOT take gate results from the promoter's history records, because each
of those records the results of the commit it replaced, not its own.

For each change and environment, the history SHALL list the promotions whose
bundle contains the change, oldest first, up to and including the first one
that Passed or the active one. The read SHALL cover promotions merged since
the oldest listed change. Results for a replaced promotion SHALL be cached
once its gate runs completed or once it was replaced more than one hour ago.

#### Scenario: FLT-DLV-18.01 -- First bundle replaced while its analysis ran

- GIVEN change C's first bundle B1 was promoted to environment E at 23:00
- AND E's analysis gate for B1 was still running when bundle B2, which also contains C, was promoted to E at 23:10
- AND E's analysis gate for B2 passed
- WHEN the drawer for C renders
- THEN E's history SHALL list B1 at 23:00 as Replaced and B2 at 23:10 as Passed
- AND E's promotion time for C SHALL be 23:00

#### Scenario: FLT-DLV-18.02 -- Failed attempt is flagged

- GIVEN change C's first bundle B1 failed E's analysis gate
- AND a later bundle B2 containing C passed in E
- AND every environment in E's group contains C
- WHEN C's row renders
- THEN the group cell SHALL read Passed with "1 failed attempt"

#### Scenario: FLT-DLV-18.03 -- Gate results come from the promotion's own commit

- GIVEN the promoter's history record for B2 carries B1's failed analysis result
- WHEN the BFF builds E's history
- THEN B2's outcome SHALL come from the check runs on B2's own merge commit
- AND B1's outcome SHALL come from the check runs on B1's own merge commit
