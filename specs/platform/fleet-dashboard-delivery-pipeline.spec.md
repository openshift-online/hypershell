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
with estimates and arrival times taken from recent history. Above the changes,
a **Pipeline now** panel (FLT-DLV-23) states the situation at this moment:
the head of both repositories, what is in flight, and what should happen next.
It reads only sources the dashboard can already reach, with credentials it
already holds.

One merge rarely moves as one unit. Each Konflux build that finishes produces
its own snapshot, release, and bundle, so one change with five rebuilt
components can publish five bundles within ten minutes. Each bundle that
reaches an environment restarts that environment's gates. A component whose
build failed ships later, inside the next successful build of that component.
The requirements below follow components and bundles, not only changes, so the
tab can say what really runs where.

### Terms

| Term | Meaning |
| --- | --- |
| Change | One first-parent commit on the application repository's `main` branch (`FD_UPSTREAM_REPO`), usually a squash-merged pull request. |
| Bundle | A Konflux release bundle: an OCI artifact tagged `release-bundle-*` whose `bundle.json` records six component images, each with its `source.git.revision`, and `manifests.git.revision`. |
| Locked bundle | A bundle that appears in the GitOps release-lock history. The shared bundle source (SDLC-VIS-03) resolves it. |
| Pending bundle | A published bundle newer than the newest locked bundle. It is not in the GitOps repository yet. |
| Bundled component | A component that appears in `bundle.components[]` of any known bundle. |
| Outside-bundle component | A component that Konflux builds on push but that no known bundle lists. It ships by its own route, not through the bundle. |
| Rebuilt components | The bundled components whose build check run on the change's commit exists (FLT-DLV-05), whatever its result. |
| First bundle | The oldest bundle that contains the change (FLT-DLV-06). |
| Contains | A bundle contains a change when it is the change's first bundle or any newer bundle. |
| Phase | One stage after Merged, as one step of the pipeline: Build, Policy check, Release, GitOps update, then one phase per environment group. |
| Run | One execution of a phase: one component build or policy check run, one bundle's release, one bundle's GitOps update, or one bundle's promotion through an environment group. |
| Estimate | The median duration of the last 5 completed runs of a phase (FLT-DLV-09). |
| Slowest | The longest duration among the same last 5 completed runs. |
| Environment group | The promotion environments that share one `delivery.hypershell.app/env` label value (for example the int hub and its spokes). |
| Delivered | A change is delivered when every promotion environment's active release contains it. |
| Delivery time | The finish time of the change in the last environment to finish it (FLT-DLV-08). |
| Superseded build | A failed component build of change C whose component was later built successfully at a later commit on `main`. That later image includes C's code (FLT-DLV-05). |
| Partly shipped | A bundle carries some, but not all, of a change's rebuilt bundled components at or after the change (FLT-DLV-22). |
| Bundle train | The bundles published from one burst of builds: one bundle per finished component build, each locked into GitOps `main` in turn (FLT-DLV-22). |
| Depends on | The environments whose gates an environment waits for before it promotes, from the promotion strategy's `dependsOn` (FLT-DLV-08). |

### Data reachability

| Stage | Source | Credential |
| --- | --- | --- |
| Merged | Application repository commits API | `FD_UPSTREAM_TOKEN_FILE` (existing) |
| Build, Policy check | Check runs on the change's commit, reported by Konflux Pipelines as Code. The Build popup also reads the image check run (`FD_DELIVERY_IMAGE_CHECK`) from the same list, and its annotations when it failed. | `FD_UPSTREAM_TOKEN_FILE` (existing) |
| Release | Locked bundles: the shared bundle source (release lock and compare windows). Pending bundles only: tag list and `bundle.json` of `FD_DELIVERY_BUNDLE_REPOSITORY` (public registry API). Downloads: the matching application-repository GitHub release. | existing tokens; none for the public registry |
| GitOps update | Pull requests and release-lock history of `FD_GITHUB_REPO` | `FD_GITHUB_TOKEN_FILE` (existing) |
| Environment groups | The existing promotion source (`/api/promotion`): environments, active and proposed releases, gates, analysis | existing BFF service account |

The step between a passed policy check and a published bundle (the Konflux
managed release and its final pipeline) has no source the dashboard can read
without Konflux cluster access. The Release stage infers it (FLT-DLV-06). When
the Release phase is Overdue, the "Where to look" popup (FLT-DLV-20) points the
operator to the places that can explain it. It links to the Konflux UI and to
the release GitHub App settings from settings; the dashboard does not read them.

## Requirements

### Requirement: FLT-DLV-01 -- Delivery tab

Glass SHALL provide a fourth top-level tab, **Delivery**, at path `/delivery`,
after Fleet Map, Agent SDLC, and E2E Runs. The tab SHALL use the shared Glass
shell and its tab navigation. Its page surface SHALL have a header region with
the title "Delivery", a one-sentence description, and a refresh action, and a
body region with the Pipeline now panel (FLT-DLV-23), the Now and next summary
(FLT-DLV-10), and the change table (FLT-DLV-03), in that order.

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
a `(#n)` trailer. A delivered change SHALL carry its delivery time. When the
promotion history (FLT-DLV-18) does not reach the change's last finish, the
delivery time SHALL be omitted. It SHALL NOT be the time the BFF first saw the
change delivered.

#### Scenario: FLT-DLV-02.03 -- Delivery time is not the first observation

- GIVEN change C finished its last environment at 01:20
- AND the BFF started at 13:10
- WHEN the BFF builds the delivery payload
- THEN C's delivery time SHALL be 01:20, or omitted when the history does not reach it
- AND it SHALL NOT be 13:10

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

A Running or Waiting stage SHALL add a warning treatment reading **Overdue**
once its elapsed time exceeds the phase's slowest of the last 5 completed runs
(FLT-DLV-09) plus 5 minutes. A stage whose phase has no estimate SHALL never be
Overdue. While any stage of a change is Overdue, the change SHALL show no
arrival time (FLT-DLV-10). Overdue replaces the earlier "Slower than usual"
rule (twice the typical duration): replayed over 3-13 November it raised 73
flags with 45 false alarms, while slowest-of-5 plus 5 minutes raised 31 flags
with 6 false alarms and caught 22 of 23 real stalls.
Unknown SHALL never be rendered as Passed.

#### Scenario: FLT-DLV-04.01 -- Overdue stage is flagged

- GIVEN the last 5 completed Build runs of the rebuilt component took 9, 10, 10, 11, and 14 minutes
- AND a change's Build stage has been Running for 20 minutes
- WHEN the row renders
- THEN the Build cell SHALL show Running with "20 min"
- AND it SHALL show a warning "Overdue"
- AND the change SHALL show no arrival time

#### Scenario: FLT-DLV-04.02 -- Within the slowest run plus the margin

- GIVEN the same last 5 Build runs
- AND a change's Build stage has been Running for 18 minutes
- WHEN the row renders
- THEN the Build cell SHALL NOT show "Overdue"

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

Builds and policy checks of outside-bundle components SHALL still show in the
Build and Policy check stages and in the drawer, marked "Ships outside the
bundle" in the drawer. They SHALL NOT decide bundle containment (FLT-DLV-06).

**Superseded builds.** An image is built from the whole repository at one
commit, so a later successful build of a component carries every earlier
change. A failed component build of change C SHALL count as superseded when a
build of the same component succeeded at a later commit on the application
repository's linear `main` history. For that component, C SHALL then follow the
superseding build: its policy check, and its revision in bundles. When every
failed build of C is superseded, C's Build stage SHALL be Passed with "Rebuilt
in <short sha> (#n)", naming the newest superseding commit, and the later
stages SHALL proceed as for a passed build. While some failed builds of C are
not superseded, Build SHALL stay Failed and SHALL name only those components.
The BFF SHALL find superseding builds in the check runs it already reads for
the listed changes and for the head of `main` (FLT-DLV-23). It SHALL make no
extra call for them.

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
- AND every later stage of that change SHALL read Not started until a later build supersedes the failed one

#### Scenario: FLT-DLV-05.04 -- A later build supersedes a failed one

- GIVEN change C's api-server and cli builds failed
- AND change D, merged after C, built api-server and cli successfully
- WHEN C's row renders
- THEN C's Build cell SHALL read Passed with "Rebuilt in <D's short sha>"
- AND C's Policy check and Release SHALL follow D's api-server and cli policy checks and bundles

### Requirement: FLT-DLV-06 -- Release from the shared bundle source

The delivery source SHALL take locked bundles, their aliases, revisions,
component revisions, and compare windows from the shared bundle source that
already feeds the Fleet Map and the Agent SDLC Shipped matrix (SDLC-VIS-03,
SDLC-VIS-22, FLT-BPR). It SHALL NOT re-read the release lock. The shared lock
resolver SHALL keep each component's `bundle.components[].source.git.revision`
from the lock it already reads, keyed by component name.

**Containment by component revision.** A bundle's `manifests.git.revision` is
the newest component source commit in its Snapshot. It is not proof that every
component the change rebuilt is in the bundle: a component build can fail, or
still be running, when another component's build produces the Snapshot. So:

1. When the change has rebuilt components (FLT-DLV-05), a bundle SHALL contain
   the change only when, for every rebuilt component the bundle lists, the
   bundle's revision of that component is the change's commit or a later commit
   on the application repository's linear `main` history.
2. When the change has no rebuilt components (no build at all, every build
   replaced, or only outside-bundle builds), the manifest-revision rule below
   SHALL decide.

The first bundle is the oldest bundle that contains the change. Replayed over
3-13 November, the manifest-revision rule alone placed 13 of 67 changes with
builds in the wrong first bundle, one of them 38 hours early.

The registry SHALL be read only to find pending bundles: `release-bundle-*`
tags of `FD_DELIVERY_BUNDLE_REPOSITORY` newer than the newest locked bundle. For
each pending bundle, the source SHALL read `bundle.json` once by digest and
cache its manifests revision and component revisions permanently, because
bundle content is immutable.

**Manifest-revision rule.** The source SHALL place changes in a locked bundle
with the shared source's compare windows, and in a pending bundle with the
shared source's compare-window rule,
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
the Konflux managed release is not observable. When every component the change
built is an outside-bundle component and no bundle contains it yet, the Release
stage SHALL be Not needed with "Ships outside the bundle", not "Releasing
(inferred)". When the matching
application-repository GitHub release is still a draft or missing after the
bundle is published, the change details (FLT-DLV-11) SHALL show "Downloads not
published yet". This SHALL NOT change the Release stage state, because GitOps
consumes the registry bundle. While the GitOps source is Unknown, a change that
no bundle contains SHALL have an Unknown Release stage, not an inferred one,
because locked placement cannot be read.

While some, but not all, of the change's rebuilt bundled components are in a
published bundle at or after the change, the change is partly shipped
(FLT-DLV-22). Its Release stage SHALL then be Running with "n of m components
released", and SHALL name each missing component with its own state: building,
policy check, releasing (inferred), or failed.

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

- GIVEN a change C with no rebuilt components
- AND pending bundle B places C
- WHEN B enters the GitOps release lock and the shared bundle source resolves it
- THEN C's first bundle SHALL still be B
- AND the Delivery tab and the Agent SDLC Shipped matrix SHALL list C under the same bundle

#### Scenario: FLT-DLV-06.04 -- No extra reads for a locked bundle

- GIVEN a locked bundle B
- WHEN the delivery source needs B's compare window and component revisions
- THEN it SHALL read them from the shared bundle source and the shared lock resolver
- AND it SHALL NOT call the compare API or read the release lock for B itself

#### Scenario: FLT-DLV-06.05 -- A failed build is not in the next bundle

- GIVEN change C rebuilt the web-console component and that build failed
- AND bundle B1, published after C, has `manifests.git.revision` after C but a web-console revision before C
- AND bundle B2 has a web-console revision that is a later commit than C
- WHEN the BFF builds the payload
- THEN C's first bundle SHALL be B2, not B1

#### Scenario: FLT-DLV-06.07 -- Partly released

- GIVEN change C rebuilt api-server, cli, control-plane, agent-runtime-slim, and agent-runtime
- AND a published bundle carries the first four at C's commit and agent-runtime at an older commit
- AND agent-runtime's build at C is still running
- WHEN C's row renders
- THEN the Release cell SHALL read Running with "4 of 5 components released"
- AND it SHALL name agent-runtime as building

#### Scenario: FLT-DLV-06.06 -- Outside-bundle component

- GIVEN change C built only a component that no known bundle lists
- AND C's policy check passed
- AND no bundle contains C
- WHEN the row renders
- THEN the Release cell SHALL read Not needed with "Ships outside the bundle"
- AND it SHALL NOT read "Releasing (inferred)"

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
group finished. An environment finishes a change when the gates of its first
passing promotion of a bundle containing the change passed: the latest, across
gates, of each gate's first successful completion on that promotion's commit
(FLT-DLV-18). A gate that reports again after it passed, such as a health check
re-posted on the active commit, SHALL NOT move that time. The group
finishes when its last environment finishes. When gate times are unknown, the
environment's promotion time SHALL stand in. The promotion time of an
environment SHALL be the merge time of the first
promotion in that environment's history (FLT-DLV-18) whose bundle contains the
change. When the history does not reach back that far, the time SHALL be
unknown and omitted. A group that is Passed but had a failed attempt in its
history SHALL show "n failed attempts" next to its time. While only some environments in the group contain the
change, the stage SHALL be Running and SHALL show "n of m". When none contains it
yet, the stage SHALL be Waiting with the blocking condition taken from the
promotion payload, in this order of precedence:

1. "Waiting for hydration" when the change is on GitOps `main` but an
   environment's proposed release does not contain it yet.
2. "Waiting for <environments>" naming, among the environments that the
   group's environments depend on (the promotion strategy's `dependsOn`), those
   whose gates have not passed for a release that contains the change. Group
   order is for display only. An environment SHALL wait only for its own
   `dependsOn`, never for a whole group, because the graph can join
   environments from different groups.
3. "Release analysis running" or "Health check running" when the group's own
   gate check is in progress for a release that contains the change.

A Failed gate for a release that contains the change SHALL make the stage Failed
and name the gate.

An environment whose promotion history (FLT-DLV-18) shows a passed promotion of
a bundle containing the change SHALL count as Passed for that change. The gates
of a newer bundle that is active there SHALL NOT change it: they belong to the
rows of the changes that bundle adds. Without that history, the gates of the
active release decide, as above.

When the change is partly shipped (FLT-DLV-22), the environment-group stages
SHALL follow the bundles that carry the shipped components, and each SHALL say
"Partly: n of m components". The group SHALL be Passed for the change only once
a bundle that contains all of the change's components has finished there.

#### Scenario: FLT-DLV-08.04 -- An environment waits for its own dependencies

- GIVEN environment S in the Prod group depends on the Int hub and one Int spoke only
- AND both have passed their gates for a release containing C
- AND another Int spoke has not
- WHEN C's row renders
- THEN S SHALL NOT read "Waiting for Int"
- AND S SHALL be promoted or waiting on its own gates

#### Scenario: FLT-DLV-08.05 -- Partly shipped in a group

- GIVEN change C is partly shipped: 4 of its 5 components are active in every Int environment
- WHEN C's row renders
- THEN the Int cell SHALL read Running with "Partly: 4 of 5 components"
- AND it SHALL NOT read Passed until a bundle with all 5 components finishes in Int

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

### Requirement: FLT-DLV-09 -- Phase estimates

The BFF SHALL compute, for each phase, an **estimate** (the median duration of
the last 5 completed runs of that phase) and a **slowest** value (the longest
of the same runs). Runs are ordered by their end time. A phase with fewer than 3
completed runs SHALL have no estimate and no slowest value; the UI SHALL then
omit "usually" text, the Overdue warning, and every arrival time that depends on
that phase.

A run SHALL be sampled once per execution, not once per change. Many changes
ride one bundle; counting each of them would weight one slow bundle many times.

| Phase | One run is | Starts | Ends |
| --- | --- | --- | --- |
| Build | one component's build check run, per component | the change's merge | the check run's completion |
| Policy check | one component's policy check run, per component | that component's build completion on the same commit | the check run's completion |
| Release | one bundle | the latest policy check completion on the bundle's `manifests.git.revision` commit | the bundle's publication |
| GitOps update | one bundle | the bundle's publication | the merge of the first release-lock commit for that bundle |
| Environment | one passed promotion in one environment | the promotion's merge on the environment branch | its gates' pass: the latest, across gates, of each gate's first successful completion |

Only successful runs count. An environment phase ends when its gates pass, not
when the promotion pull request merges. A promotion whose gate ran more than
once before its first pass (a failed attempt that was retried) SHALL NOT be
sampled, because its duration includes the retry. Runs after the first pass
re-confirm it and SHALL NOT count as a re-run.

An environment group's estimate is not sampled as a whole. Its arrival comes
from the environment estimates along the `dependsOn` graph (FLT-DLV-10). Most
promotions in a busy hour are replaced before their gates finish, so sampling
whole groups leaves too few runs; each environment's passed promotions are
enough. The promotion history (FLT-DLV-18) SHALL reach back far enough to hold
the last 5 passed promotions of each environment, up to
`FD_DELIVERY_HISTORY_DAYS`.

Build and Policy check are per component. A change's Build and Policy check
estimate and slowest value SHALL be those of the slowest component the change
rebuilt (the largest estimate, and the largest slowest value). When the change
rebuilt bundled components, only they count; otherwise every built component
counts. When any of those components has no estimate, the phase has none.

The BFF SHALL put each stage's estimate and slowest value on the stage of every
listed change, so the UI needs no per-component lookup.

#### Scenario: FLT-DLV-09.01 -- Too little history

- GIVEN only 2 completed Policy check runs of the rebuilt component
- WHEN the Policy check stage renders as Running
- THEN it SHALL show elapsed time only, with no "usually" text

#### Scenario: FLT-DLV-09.02 -- Median of the last 5 runs

- GIVEN 7 completed Release runs of 30, 31, 32, 40, 41, 42, and 90 minutes, in that order of end time
- WHEN the BFF computes the Release estimate
- THEN the estimate SHALL be 41 minutes, from the last 5 runs
- AND the slowest value SHALL be 90 minutes

#### Scenario: FLT-DLV-09.03 -- One sample per bundle

- GIVEN 6 changes that all rode bundle B
- WHEN the BFF samples the GitOps update phase
- THEN bundle B SHALL contribute one run

#### Scenario: FLT-DLV-09.04 -- Slowest rebuilt component decides

- GIVEN change C rebuilt components A and B
- AND A's Build estimate is 8 minutes and B's is 14 minutes
- WHEN C's Build stage renders
- THEN its estimate SHALL be 14 minutes

#### Scenario: FLT-DLV-09.06 -- Busy hour still yields environment estimates

- GIVEN environment E had 3 promotions replaced in the last hour and 5 passed promotions in the last 3 days
- WHEN the BFF computes E's estimate
- THEN it SHALL use the 5 passed promotions
- AND the replaced promotions SHALL NOT be sampled

#### Scenario: FLT-DLV-09.05 -- Re-run gate is not sampled

- GIVEN a promotion whose analysis gate failed once and passed on a second run on its commit
- WHEN the BFF samples the environment phase
- THEN that promotion SHALL NOT contribute a run
- AND a health check re-posted after its pass SHALL NOT make a promotion a re-run

### Requirement: FLT-DLV-10 -- Now and next summary

Above the change table, the tab SHALL show a **Now and next** summary with one
row per undelivered change, newest first, at most 3 rows, and a link to the full
table when more exist. Each row SHALL name the change (short SHA, pull request,
title) and SHALL state:

- **Now:** the earliest stage that is not Passed or Not needed, its state, its
  elapsed time, and its estimate when known.
- **Next:** the following stage, with an expected time equal to the remaining
  time of the current stage plus the estimate of the next stage, written as
  "usually about <time>". The remaining time of a stage is its estimate minus
  its elapsed time, never below zero. When either estimate is unknown, the
  expected time SHALL be omitted. When the current stage is Failed, Next SHALL
  state what restarts the pipeline instead (for a failed build: "Re-run the
  failed pipeline, or merge a change to the same component", because component
  builds run only for changes under their own paths).
- **Arrival:** the progress bar (FLT-DLV-19) and, per environment group, the
  expected arrival of the change in that group.

**Arrival time.** The expected arrival in an environment SHALL be now, plus
the remaining time of the current phase, plus the estimates of every later
pipeline phase, plus, along the `dependsOn` graph, the latest arrival among the
environments it depends on, plus its own estimate. An environment group's
arrival is the latest arrival among its environments. Phases already Passed or
Not needed add nothing.

When a newer bundle of the train (FLT-DLV-22) is expected to reach an
environment before that environment's running gate ends, the gate will
restart. The arrival SHALL then start from that newer bundle's expected
arrival, and Next SHALL say "Restarts when the <components> bundle lands".

- **Before the bundle is published** (the Release stage is not Passed), the
  arrival SHALL be a range, because the release step is not observable. The
  early end is now plus 0.8 times the sum above. The late end is now plus the
  remaining slowest time of the current phase (its slowest value minus its
  elapsed time, never below zero) plus the slowest values of every later phase
  up to that group.
- **After the bundle is published**, the arrival SHALL be one time.
- **No arrival** SHALL be shown for a group when any phase up to it has no
  estimate, when any stage of the change is Failed, or while any stage of the
  change is Overdue (FLT-DLV-04). A group the change already reached shows its
  finish time instead.

Replayed over 56 changes from 3 to 13 November, merge to the last environment
took a median of 97 minutes (fastest 43). The typical arrival miss was 36
minutes at merge, 7 minutes once the bundle was published, and under 2 minutes
in the last environment group.

The summary icon SHALL follow the highest severity among its rows: Failed over
Overdue over in-progress. When every listed change is delivered, the
summary SHALL read "Everything on main is delivered" with the newest change's
delivery time and its total time from merge.

#### Scenario: FLT-DLV-10.01 -- Change building

- GIVEN change `a15cc91` (#997) has Build Running for 6 minutes with a Build estimate of 10 minutes
- AND the Policy check estimate is 3 minutes
- WHEN the summary renders at 15:17
- THEN the row SHALL read Now "Building control-plane - 6 min, usually 10 min"
- AND Next "Policy check - usually about 15:24"

#### Scenario: FLT-DLV-10.04 -- Arrival range before the bundle is published

- GIVEN a change whose Build has been Running for 4 minutes with estimate 10 and slowest 15 minutes
- AND estimates Policy check 5, Release 30, GitOps update 10, Int 20 minutes
- AND slowest values Policy check 8, Release 45, GitOps update 15, Int 30 minutes
- WHEN the summary renders at 12:00
- THEN the Int arrival SHALL read from 12:57 to 13:49
- BECAUSE the early end is 0.8 x (6 + 5 + 30 + 10 + 20) = 56.8 minutes and the late end is 11 + 8 + 45 + 15 + 30 = 109 minutes

#### Scenario: FLT-DLV-10.05 -- One arrival time after publication

- GIVEN a change whose bundle is published and whose GitOps update has been Waiting for 4 minutes with estimate 10 minutes
- AND the Int estimate is 20 minutes
- WHEN the summary renders at 12:00
- THEN the Int arrival SHALL read 12:26

#### Scenario: FLT-DLV-10.06 -- Overdue withdraws the arrival

- GIVEN a change whose Release stage is Overdue
- WHEN the summary renders
- THEN no environment group SHALL show an arrival time for that change
- AND the Release cell SHALL offer the "Where to look" popup (FLT-DLV-20)

#### Scenario: FLT-DLV-10.07 -- A newer bundle will restart the gate

- GIVEN change C is in Int and the Int hub's release analysis has run for 17 minutes with estimate 25 minutes
- AND agent-runtime's build finished 4 minutes ago and its bundle is expected on GitOps `main` in 8 minutes
- WHEN C's summary row renders
- THEN Next SHALL read "Restarts when the agent-runtime bundle lands"
- AND the Stage arrival SHALL start from that bundle's expected arrival in Int, not from the running analysis

#### Scenario: FLT-DLV-10.02 -- Nothing in flight

- GIVEN every listed change is delivered
- WHEN the summary renders
- THEN it SHALL read "Everything on main is delivered" with the newest change's delivery time and time from merge

#### Scenario: FLT-DLV-10.03 -- Failure takes precedence

- GIVEN one undelivered change has a Failed Build and another has an Overdue GitOps update
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
definitions, the changes with their stage states and per-stage estimates, the
phase estimates (FLT-DLV-09), the bundle train (FLT-DLV-22), the heads of both
repositories and each environment's bundle and gate now (FLT-DLV-23), the
release diagnostics when the Release phase is Overdue (FLT-DLV-20), the omitted-change count, and a per-source `stale` and `error` flag for `upstream`,
`registry`, `gitops`, `promotion`, and `history`.

Per refresh, the BFF SHALL make at most one commits-list call, one check-runs
call per change whose check runs have not settled, one environment-branch
commits call per environment whose active hydrated commit changed since the
last read, one hydration-metadata read and one check-runs call per new
promotion it has not yet settled (FLT-DLV-18),
one gate check-runs call for an active promotion once its gates passed and
until its gate times are read, one gate check-runs call per refresh for an
active promotion whose gate is running, to read the attempt the gate names in
its check run summary ("Attempt n of m", FLT-DLV-23), the check-runs call for
the head of the application repository's `main` (FLT-DLV-05, FLT-DLV-23),
one registry tag-list call, one `bundle.json` read per new pending bundle,
and one pull-request list call. While a listed change's Release phase is
Overdue, it MAY add one path-filtered commits call per path in
`FD_DELIVERY_RELEASE_PATHS` and one releases-list call (FLT-DLV-20). The image
check run comes from the check-runs call already made. The BFF MAY add one
annotations call per failed image check run, cached by check run id, because a
re-run creates a new check run (FLT-DLV-20). Locked-bundle data SHALL come from the shared
bundle source's cache with no extra call. Completed check runs and pending
bundle contents SHALL be cached by full SHA or digest, because they are
immutable. Pending-bundle compare results SHALL use the shared compare cache.

| Setting | Required | Default | Meaning |
| --- | --- | --- | --- |
| `FD_DELIVERY_BUNDLE_REPOSITORY` | yes, to enable the plane | none | Registry repository that holds `release-bundle-*` tags. When unset, `/api/delivery` returns `enabled: false`. |
| `FD_DELIVERY_RELEASE_PR_BRANCH` | no | none | Head branch of the automated bundle update pull request in `FD_GITHUB_REPO`. When unset, the GitOps update stage has no Running state. |
| `FD_DELIVERY_CHECK_APP` | no | `red-hat-konflux` | GitHub App slug whose check runs are builds and policy checks. |
| `FD_DELIVERY_IMAGE_CHECK` | no | `E2E / Wait for Konflux images` | Name of the check run, from any GitHub App, that waits for the Konflux builds and confirms their images are published. The Build popup shows it (FLT-DLV-20). When empty, the popup omits it. |
| `FD_DELIVERY_HISTORY_DAYS` | no | `7` | How far back the promotion history may read to hold 5 passed promotions per environment (FLT-DLV-09, FLT-DLV-18). |
| `FD_DELIVERY_WINDOW` | no | `30` | Maximum changes in the payload. |
| `FD_REFRESH_DELIVERY` | no | `60s` | Cache refresh interval. |
| `FD_DELIVERY_RELEASE_LOG_URL` | no | none | Link to the Konflux release list or log for the application. When unset, the Release popup omits the link. |
| `FD_DELIVERY_RELEASE_APP_URL` | no | none | Link to the release GitHub App's installation settings, where pending permission requests show. When unset, the popup omits the link. |
| `FD_DELIVERY_RELEASE_PATHS` | no | `pipelines/release-bundle/,scripts/release_bundle.py` | Comma-separated application-repository paths of the bundle publisher. The Release popup lists commits since the last good bundle that touch them. |

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
`InFlight`, `ChangeSelected`, `BuildFailed`, `ReleaseOverdue`,
`AllDelivered`, `PromotionRetried`, `BundleTrain`, `SourceUnavailable`,
`Loading`, `NotConfigured`, and `Error`. `InFlight` SHALL show the progress bar
and arrival range (FLT-DLV-19, FLT-DLV-10); `ReleaseOverdue` SHALL show the
Overdue Release phase with its "Where to look" popup open (FLT-DLV-20).
`BundleTrain` SHALL show the Pipeline now panel during a bundle train, a partly
shipped change, a superseded build, and an analysis that a newer bundle will
restart (FLT-DLV-22, FLT-DLV-23). Every state SHALL show the Pipeline now
panel.
The production page SHALL pass `skills/build/ui-build-gate/SKILL.md`.

#### Scenario: FLT-DLV-16.01 -- Every mockup state has a production story

- GIVEN the parity stories above
- WHEN the production Delivery tab is implemented
- THEN a `Production/Glass/Delivery` story SHALL exist for each parity state name

### Requirement: FLT-DLV-17 -- Tests

The implementation SHALL include tests that cite these IDs in `Verifies:`
markers: BFF unit tests for check-run classification (FLT-DLV-05), first-bundle
selection including a change that rides a later bundle, containment by
component revision with a failed build, outside-bundle components, and
pending-bundle placement that matches the shared bundle source once the bundle
is locked (FLT-DLV-06), environment-group states, waiting reasons, and gate
finish times (FLT-DLV-08), estimates from the last 5 runs sampled once per
execution, with fewer than 3 samples and re-run gates (FLT-DLV-09), the
per-refresh call budget (FLT-DLV-12), per-source degradation (FLT-DLV-13), and
release diagnostics (FLT-DLV-20); UI domain tests for the Overdue rule
(FLT-DLV-04), the Now and next text, arrival times, and severity (FLT-DLV-10),
and progress-bar cells and percent (FLT-DLV-19); route-codec tests for
`change` (FLT-DLV-14); and the replay (FLT-DLV-21). They SHALL also cover
superseded builds (FLT-DLV-05), delivery time (FLT-DLV-02), waiting on
`dependsOn` (FLT-DLV-08), per-environment estimates (FLT-DLV-09), arrival
along the graph and gate restarts (FLT-DLV-10), partly shipped changes and the
bundle train (FLT-DLV-22), and the Pipeline now panel (FLT-DLV-23).

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
the oldest listed change, and SHALL reach further back, up to
`FD_DELIVERY_HISTORY_DAYS`, until it holds the last 5 passed promotions of each
environment (FLT-DLV-09). Results for a replaced promotion SHALL be cached
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

### Requirement: FLT-DLV-19 -- Progress bar

Each Now and next row SHALL show a progress bar for the change with one cell
per phase, in stage order (FLT-DLV-03), all cells of equal width. Each cell
SHALL carry its phase number (1, 2, ...). A short vertical line SHALL connect
each cell to its own label below it. The label SHALL state the phase name, its
state as text, and one time: the finish time for a Passed phase, the elapsed
time for a Running or Waiting phase, and the arrival time or range for an
environment group not reached yet (FLT-DLV-10). The cell's color SHALL follow
the state (FLT-DLV-04) and SHALL never be the only carrier of it. On a narrow
screen the cells and their labels SHALL wrap to further lines instead of
scrolling sideways.

The bar SHALL state a percent complete: elapsed / (elapsed + remaining), where
elapsed is the time since the merge, and remaining is the remaining time of the
current phase plus the estimates of every later phase. When any of those
estimates is unknown, or while a stage is Failed or Overdue, the percent SHALL
be omitted. The bar SHALL expose the percent and each cell's label to assistive
technology; the change table stays the full accessible representation
(FLT-DLV-03).

#### Scenario: FLT-DLV-19.01 -- Cells and labels

- GIVEN a promotion graph with groups Int, Stage, and Prod
- WHEN a Now and next row renders
- THEN the bar SHALL show 7 numbered cells: Build, Policy check, Release, GitOps update, Int, Stage, Prod
- AND each cell SHALL have a label naming the phase, its state, and one time

#### Scenario: FLT-DLV-19.02 -- Percent complete

- GIVEN a change merged 30 minutes ago whose remaining time to the last group is 90 minutes
- WHEN the row renders
- THEN the bar SHALL read "25%"

#### Scenario: FLT-DLV-19.03 -- Narrow screen

- GIVEN a 390 pixel wide viewport
- WHEN the row renders
- THEN the cells SHALL wrap onto more than one line
- AND the page SHALL NOT scroll sideways

### Requirement: FLT-DLV-20 -- Where to look

An Overdue or Failed phase SHALL offer a **Where to look** popup from its
progress-bar cell and from its table cell. The popup SHALL open on hover, on
keyboard focus, and on tap, and SHALL close on Escape. Its content depends on
the phase:

- **Build, Failed:** "Re-run the failed pipeline, or merge a change to the same
  component", with a link to each failed check run. Component builds run only
  for changes under their own paths, so a later unrelated change does not
  rebuild the component.
- **Build or Policy check, Overdue:** the running check runs, with links.
- **Build, Failed or Overdue, image check:** when the change's commit has the
  image check run named by `FD_DELIVERY_IMAGE_CHECK`, the Build popup SHALL
  also show its state with a link to it. When it failed, the popup SHALL show
  the messages of its failure annotations that have a title, such as which
  component has no usable image or which built image is not in the registry.
  The image check is a CI job that waits for the Konflux builds and confirms
  their images are published. It measures the same wait as Build, so it SHALL
  NOT be a phase or a progress-bar cell of its own.
- **Release, Overdue:** the BFF SHALL supply these release diagnostics:
  1. the last good release: the newest published bundle's alias and
     publication time;
  2. a link to the Konflux release log (`FD_DELIVERY_RELEASE_LOG_URL`), when
     set;
  3. the commits on the application repository since the last good bundle was
     published that touch any `FD_DELIVERY_RELEASE_PATHS` path, with links;
  4. a link to the release GitHub App's settings, to check for pending
     permission requests (`FD_DELIVERY_RELEASE_APP_URL`), when set. The
     dashboard cannot read pending permissions with its credentials, so it
     SHALL NOT claim whether any are pending;
  5. the newest registry bundle tag next to the newest published GitHub release
     tag, flagged when they differ.
- **GitOps update, Overdue or Failed:** the bundle update pull request and its
  check state, or "No bundle update pull request yet".
- **Environment group, Overdue or Failed:** per environment not finished, the
  gate that is running or failed, with the environment's analysis and Argo CD
  links from the promotion payload.

The release diagnostics SHALL be read only while a listed change's Release
phase is Overdue, within the call budget of FLT-DLV-12. A diagnostic whose read
failed SHALL say so; the popup SHALL still show the others.

#### Scenario: FLT-DLV-20.01 -- Release overdue

- GIVEN change C's Release phase is Overdue
- AND a commit touching the bundle publisher merged after the last good bundle
- WHEN the operator hovers, focuses, or taps C's Release cell
- THEN the popup SHALL show the last good release, the release log link, that commit, the release app link, and the registry and GitHub release tags

#### Scenario: FLT-DLV-20.02 -- Failed build

- GIVEN change C's web-console build failed
- WHEN the operator opens the popup on C's Build cell
- THEN it SHALL read "Re-run the failed pipeline, or merge a change to the same component"
- AND it SHALL link to the failed check run

#### Scenario: FLT-DLV-20.03 -- Failed image check

- GIVEN change C's Build phase is Failed
- AND C's image check run failed with the annotation "No usable image for: web-console"
- WHEN the operator opens the popup on C's Build cell
- THEN it SHALL show the image check as Failed with a link to it
- AND it SHALL show "No usable image for: web-console"

### Requirement: FLT-DLV-21 -- Replay validation

The repository SHALL include a replay that checks the estimate, arrival, and
Overdue rules against past runs. It SHALL rebuild each past change's phases
from the GitHub API: application-repository commits, check runs, and releases;
the GitOps repository's release-lock history; and the promotion pull requests
on the environment branches. It SHALL estimate each run only from runs that
ended before it. It SHALL estimate environment groups as the delivery source
does: one sample per passed promotion of each environment, combined along the
promotion graph (FLT-DLV-09, FLT-DLV-10), with each gate's first pass as its
time (FLT-DLV-08). It SHALL report, for a date range:

- merge to last environment: median and fastest;
- the median absolute arrival miss for the last group, at merge, once the
  bundle is published, and once the change reached the last group's phase;
- per Overdue rule (twice the estimate, slowest of 5, slowest of 5 plus 5
  minutes): flags raised, false alarms, and real stalls caught. A **real stall**
  is a run that finally took more than its slowest of 5 plus 15 minutes, or
  never finished. A **false alarm** is a flagged run that is not a real stall.
  A stall is caught when the rule flagged it before it finished.

Repository names, branch prefixes, and the date range SHALL be arguments, not
literals in the replay, so it obeys the fleet-identity firewall (FLT-DLV-15)
when run against any fleet. The pure replay logic SHALL have unit tests that
run without network access.

#### Scenario: FLT-DLV-21.01 -- Estimates use only the past

- GIVEN runs R1, R2, R3, R4 ending in that order
- WHEN the replay estimates R4
- THEN it SHALL use R1, R2, and R3 only

### Requirement: FLT-DLV-22 -- Partly shipped changes and bundle trains

The delivery source SHALL follow each of a change's rebuilt bundled components
on its own. For each component it SHALL know the newest published bundle that
carries the component at or after the change, and where that bundle is active.

A change is **partly shipped** while some, but not all, of those components
are in a published bundle. The row SHALL then show, per stage, which
components have reached it ("n of m components"), and SHALL name the missing
ones with their state. A partly shipped change SHALL NOT show Release Not
started: some of its code is already released.

The **bundle train** is the sequence of bundles published after the newest
bundle whose gates passed in the first environment group. The payload SHALL
list the train: for each bundle, its alias, publication time, what it changed
against the previous bundle (component names and revisions, or "same
components" when only its release name differs), and where it is active. A
bundle in flight is also part of the train when it is inferred from a
finished component build whose bundle is not published yet: "agent-runtime
bundle releasing (inferred)".

Each bundle of the train that reaches an environment restarts that
environment's gates. The tab SHALL say so where it matters: in Next (FLT-DLV-10),
in the Pipeline now panel (FLT-DLV-23), and in the environment's promotion
history, where the restarted promotion reads Replaced (FLT-DLV-18).

#### Scenario: FLT-DLV-22.01 -- One merge, several bundles

- GIVEN change C rebuilt five bundled components
- AND four builds finished between 12:47 and 12:52 and each produced a bundle
- AND agent-runtime's build finished at 13:24
- WHEN the BFF builds the payload at 13:28
- THEN the train SHALL list the published bundles with the components each changed
- AND it SHALL end with "agent-runtime bundle releasing (inferred)"

#### Scenario: FLT-DLV-22.02 -- A repeat bundle is named as one

- GIVEN bundle B2 has the same component revisions as B1 and only a new release name
- WHEN the train renders
- THEN B2 SHALL read "same components"
- AND B2 SHALL still be shown as restarting the gates where it lands

### Requirement: FLT-DLV-23 -- Pipeline now

Above the Now and next summary, the tab SHALL show a **Pipeline now** panel
that states the situation at this moment, without the operator reading rows.
It SHALL answer three questions:

1. **Where are the heads?** The application repository's `main` head (short
   SHA, pull request, title, merge time) and the GitOps repository's `main`
   head (the locked bundle's alias, publication time, and the time it was
   locked).
2. **What is in flight?** Component builds and policy checks running on
   `main`, with elapsed time: only the checks a change's stages use (its
   builds, and the policy checks of components built on that commit), and
   only for changes not yet delivered; bundles releasing (inferred) and pending
   bundles; an open bundle update pull request; and, per environment, the
   bundle it runs, the gate in progress with its elapsed time and attempt
   number ("attempt 1 of 3"), and the environments waiting with what each
   waits for (its `dependsOn`).
3. **What happens next?** The expected next events in order, each with an
   expected time when estimates allow: the next bundle published, the next
   GitOps lock, gates that a new bundle will restart, and the next
   environments to promote.

Each environment SHALL appear once, with the bundle it runs now, even when no
listed change is in flight there. The panel SHALL use only data the payload
already carries, and SHALL make no extra source call. When a source is
Unknown (FLT-DLV-13), the panel SHALL say which answer it cannot give.

#### Scenario: FLT-DLV-23.01 -- A busy moment

- GIVEN `main` head is #1189, whose agent-runtime build finished at 13:24
- AND GitOps `main` locks bundle B3, published at 13:00
- AND every Int environment runs B3, and the Int hub's release analysis is on attempt 1 of 3 for 17 minutes
- AND Stage and Prod environments have open promotions waiting for Int
- WHEN the panel renders
- THEN it SHALL name #1189 and B3 as the heads
- AND it SHALL list the Int hub's analysis with "17 min, attempt 1 of 3"
- AND it SHALL list "agent-runtime bundle releasing (inferred)" in flight
- AND Next SHALL say the agent-runtime bundle will restart the Int analysis, then Stage promotes

#### Scenario: FLT-DLV-23.02 -- Quiet pipeline

- GIVEN nothing is building, releasing, or gating
- WHEN the panel renders
- THEN it SHALL show both heads and each environment's bundle
- AND it SHALL read "Nothing in flight"

#### Scenario: FLT-DLV-23.03 -- Only stage checks are in flight

- GIVEN a change whose merge queue ran policy checks for components it did not build on push, and a group policy check
- AND a delivered change with a policy check that never finished
- WHEN the panel lists what is in flight
- THEN it SHALL list neither, and SHALL list the change's own running builds
