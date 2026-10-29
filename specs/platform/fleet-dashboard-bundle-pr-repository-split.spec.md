# Fleet Dashboard Bundle Pull Requests by Repository

**ID prefix:** FLT-BPR
**Status:** Draft
**Applies to:** `components/fleet-dashboard` BFF bundle source (`pkg/sources/bundles.go`, `promotion.go`, `lock.go`) and promotion response; `packages/fleet-dashboard-ui` promotion domain, wire adapter, and Fleet Map bundle-details presentation (`src/adapters/presentation/map/map-details.tsx`)
**Tracks:** [HYPERSHELL-464](https://redhat.atlassian.net/browse/HYPERSHELL-464)

## Purpose

Make the source of every change in a Glass (`glass.infra.hypershell.app`) bundle
clear by showing pull requests separately for the two repositories that shape a
deployed bundle: the application repository `openshift-online/hypershell` and the
deployment (GitOps) repository `openshift-online/hypershell-gitops`.

Today a bundle's "in this bundle" list is a single flat pull-request set derived
from one compare window over the application repository. GitOps pull requests are
absent even though they change configuration, deployment manifests, infrastructure,
and operational behavior, and even when a GitOps change rolls out without changing
the application bundle digest. Pull-request records carry no repository identity, so
two pull requests that share a number in different repositories cannot be
distinguished and the Fleet Map bundle-details list keys rows by pull-request number
alone.

This spec defines the desired behavior: one card per application bundle digest, with
two clearly named and repository-qualified pull-request groups inside the bundle
details, correct counts and links for each, GitOps-only deployment changes made
visible against bounded deployment-revision ranges, and honest handling of the cases
where environments diverge, a baseline is missing, or one repository is unavailable.
It is an application and data-model change, not a missing repository setting: the
deployment already configures the GitOps repository and the dashboard token already
holds `contents:read` on it.

The companion deployment specifications for Glass live in
`openshift-online/hypershell-gitops` and describe the same target behavior from the
deployment side; this spec is the authority for the application (BFF and UI)
behavior.

## Requirements

### Requirement: FLT-BPR-01 -- Two Repository-Identified Pull-Request Groups Per Bundle

The fleet dashboard SHALL keep one bundle card per application bundle digest, with
the existing bundle digest identity and timeline unchanged, and SHALL present the
bundle's pull requests as two distinct groups inside the bundle details:

- an **application** group for `openshift-online/hypershell` changes, and
- a **GitOps** group for `openshift-online/hypershell-gitops` deployment changes.

Each group SHALL carry its own repository identity and its own pull-request count.
The GitOps group SHALL be labeled as deployment changes; the dashboard SHALL NOT
present GitOps changes as embedded in the application artifact. The application
group SHALL retain the existing bundle-contents behavior.

#### Scenario: FLT-BPR-01.01 -- Both repositories contribute to one card

- GIVEN a bundle whose deployment revision range contains merged pull requests in
  both the application and the GitOps repositories
- WHEN an operator opens that bundle in Glass
- THEN the bundle details SHALL show an application group and a GitOps group
- AND each group SHALL show its own count and its own pull requests
- AND the single bundle card SHALL remain keyed by the application bundle digest

#### Scenario: FLT-BPR-01.02 -- GitOps changes are labeled as deployment changes

- GIVEN a bundle with GitOps pull requests
- WHEN the bundle details render the GitOps group
- THEN the group SHALL be labeled as deployment changes for the deployment
  repository
- AND the rendering SHALL NOT imply the GitOps changes are part of the application
  bundle artifact

### Requirement: FLT-BPR-02 -- Repository Identity on Every Pull-Request Record

Every pull-request record SHALL carry a repository identifier end to end: in the
BFF promotion response, the UI wire schema, and the UI domain model. Pull requests
SHALL be deduplicated by repository plus pull-request number (equivalently, by
canonical pull-request URL), never by pull-request number alone. The response SHALL
preserve the existing pull-request fields where practical during rollout so an older
client continues to render application pull requests.

#### Scenario: FLT-BPR-02.01 -- Same number in both repositories stays distinct

- GIVEN application pull request `#42` and GitOps pull request `#42` both belong to
  one bundle
- WHEN the bundle details render
- THEN both pull requests SHALL appear, each under its own repository group
- AND neither SHALL be dropped or merged as a duplicate of the other

#### Scenario: FLT-BPR-02.02 -- Deduplication uses repository plus number

- GIVEN a repository's compare range yields the same pull request twice
- WHEN the bundle's pull-request set is assembled
- THEN that pull request SHALL appear once for that repository
- AND deduplication SHALL key on repository plus number (or canonical URL)

### Requirement: FLT-BPR-03 -- Application Group Preserves Existing Compare Semantics

The application pull-request group SHALL continue to be the merged pull requests in
the compare window between the previous bundle's application source revision and the
current bundle's application source revision (`bundle.manifests.git.revision`), as
the shared bundle source resolves them today. This change SHALL NOT alter the
application-bundle freshness semantics or the "since previous build" meaning of the
application group.

#### Scenario: FLT-BPR-03.01 -- Application group unchanged from prior behavior

- GIVEN two consecutive bundles with distinct application source revisions
- WHEN the bundle details render the application group for the newer bundle
- THEN the application group SHALL contain exactly the pull requests in the compare
  window between the previous and current application source revisions
- AND the application group's membership SHALL match the prior single-list behavior

### Requirement: FLT-BPR-04 -- GitOps Group From Explicit Deployment-Revision Ranges

The GitOps pull-request group SHALL be derived from explicit GitOps source commit
ranges tied to the deployment revisions that selected or re-deployed the bundle, not
from bundle build timestamps. The GitOps group SHALL include the lock-update pull
request that promoted the bundle and any configuration-only pull requests in those
ranges. Specifically:

- For a bundle's introduction, the range SHALL compare the previous bundle's
  introduction commit to the GitOps commit that selected this bundle, including that
  selecting commit.
- For subsequent deployment revisions that keep the same bundle digest, the range
  SHALL extend to the relevant environment's current source commit so that
  configuration-only pull requests appear without waiting for a new bundle digest.

GitOps-only changes SHALL be made visible without changing the application-bundle
freshness semantics defined in FLT-BPR-03.

#### Scenario: FLT-BPR-04.01 -- Lock-update and config pull requests at introduction

- GIVEN a bundle whose selecting GitOps commit range contains the lock-update pull
  request and a configuration-only pull request
- WHEN the bundle details render the GitOps group
- THEN both pull requests SHALL appear in the GitOps group

#### Scenario: FLT-BPR-04.02 -- GitOps-only change with unchanged bundle digest

- GIVEN a deployment revision that merges a configuration-only GitOps pull request
  while the application bundle digest stays unchanged
- WHEN the operator opens that bundle
- THEN the GitOps group SHALL include the configuration-only pull request for the
  extended deployment-revision range
- AND the application bundle card and its freshness semantics SHALL be unchanged

### Requirement: FLT-BPR-05 -- Deployment-Revision Provenance Retained Before Digest Grouping

The bundle source SHALL retain the full GitOps source commit SHAs and their
environment associations before releases are grouped by bundle digest. A single
representative SHA on a bundle card SHALL NOT be treated as sufficient provenance for
building GitOps ranges.

#### Scenario: FLT-BPR-05.01 -- Full source SHAs survive digest grouping

- GIVEN several deployment revisions that resolve to one bundle digest across
  environments
- WHEN releases are grouped by digest for the bundle card
- THEN each contributing GitOps source SHA and its environment association SHALL
  remain available to build the GitOps ranges
- AND the grouping SHALL NOT collapse them to a single representative SHA

### Requirement: FLT-BPR-06 -- Environment-Divergent GitOps Revisions Stay Honest

When environments run different GitOps revisions for the same bundle digest, the
dashboard SHALL retain that distinction. A combined GitOps pull-request list for the
bundle SHALL identify which revisions or environments include each change, and SHALL
NOT imply that every environment has received a change it has not yet received.

#### Scenario: FLT-BPR-06.01 -- Divergent environments attribute each change

- GIVEN two environments on the same bundle digest at different GitOps revisions
- AND a GitOps pull request present in only one environment's revision
- WHEN the bundle details render the GitOps group
- THEN that pull request SHALL be attributed to the revisions or environments that
  include it
- AND the display SHALL NOT claim the pull request has reached the environment that
  lacks it

### Requirement: FLT-BPR-07 -- GitOps Source Follows Hydration Provenance

The GitOps pull-request source SHALL follow hydration provenance back to the source
(dry) commits. The dashboard SHALL NOT use generated hydrated commits as the pull-
request source.

#### Scenario: FLT-BPR-07.01 -- Hydrated commits are not the PR source

- GIVEN a deployment revision pinned to a hydrated (rendered) commit
- WHEN the GitOps range for that revision is built
- THEN the range SHALL resolve through hydration metadata to the source (dry)
  commits
- AND the range SHALL NOT be computed from the hydrated commits directly

### Requirement: FLT-BPR-08 -- Bounded Historical Ranges and Honest Baselines

Historical bundle cards SHALL use recorded, bounded GitOps source ranges. The
dashboard SHALL NOT compare a historical bundle against the current `main` of either
repository. If no valid baseline exists for a group's range, the dashboard SHALL
show that the range is unavailable rather than invent a range. Rollbacks and
divergent histories SHALL be handled explicitly, not by ordering GitOps commits by
application build date.

#### Scenario: FLT-BPR-08.01 -- Historical card uses a recorded range

- GIVEN a historical bundle with a recorded deployment-revision range
- WHEN its GitOps group is built
- THEN the group SHALL use the recorded bounded range
- AND it SHALL NOT compare against the current `main` of the GitOps repository

#### Scenario: FLT-BPR-08.02 -- Missing baseline shows unavailable

- GIVEN a bundle for which no valid GitOps baseline commit can be determined
- WHEN its GitOps group is built
- THEN the GitOps range SHALL be reported as unavailable
- AND the dashboard SHALL NOT fabricate a range

#### Scenario: FLT-BPR-08.03 -- Rollback is handled without build-date ordering

- GIVEN a rollback that places an earlier GitOps revision after a later one by
  application build date
- WHEN the GitOps ranges are built
- THEN the dashboard SHALL handle the divergent history explicitly
- AND it SHALL NOT order the GitOps commits by application build date

### Requirement: FLT-BPR-09 -- Repository-Aware Compare, Token, Cache, Pagination, and Merge Formats

The bundle source SHALL reuse a repository-aware compare helper and the existing
GitOps installation token to read the GitOps repository; it SHALL NOT require a new
repository setting. It SHALL cache only fixed commit ranges, with repository
identity in the cache key, and SHALL refresh deployed-revision associations so that
same-digest GitOps changes are discovered between refreshes. The pull-request
extraction SHALL cover the merge formats used in both repositories and SHALL handle
compare pagination so a long range does not silently lose changes. If an
implementation needs the GitHub pull-request association API instead of the compare
approach, it SHALL document and separately request the required GitHub App
permissions; the compare approach uses contents read access that already exists.

#### Scenario: FLT-BPR-09.01 -- GitOps reads reuse the existing token

- GIVEN the dashboard token already has `contents:read` on the GitOps repository
- WHEN the bundle source reads GitOps compare ranges
- THEN it SHALL use the repository-aware compare helper with the existing GitOps
  token
- AND it SHALL NOT require a new repository or credential setting

#### Scenario: FLT-BPR-09.02 -- Cache key carries repository identity

- GIVEN a cached GitOps compare range for the GitOps repository
- WHEN a compare over the same numeric range is requested for the application
  repository
- THEN the two results SHALL be cached under distinct keys that include repository
  identity
- AND only fixed (bounded) ranges SHALL be cached

#### Scenario: FLT-BPR-09.03 -- Long comparisons paginate without loss

- GIVEN a GitOps or application compare range whose commit list spans multiple pages
- WHEN the pull-request set for that range is built
- THEN pagination SHALL be followed to completion
- AND no pull request in the range SHALL be silently lost

#### Scenario: FLT-BPR-09.04 -- Supported merge formats in both repositories

- GIVEN a repository whose merged pull requests use a merge format other than the
  squash-merge subject trailer
- WHEN the pull-request set for a range in that repository is built
- THEN the extraction SHALL recognize the merge formats used in that repository

### Requirement: FLT-BPR-10 -- Independent Repository Failures and Unavailable Versus Empty

A failure to load one repository's pull requests SHALL NOT hide the other
repository's pull requests and SHALL NOT break promotion data. The dashboard SHALL
distinguish an unavailable comparison (an error or missing baseline) from a
successful comparison that contains no pull requests.

#### Scenario: FLT-BPR-10.01 -- GitOps failure still renders application PRs

- GIVEN the GitOps compare fails for a bundle
- WHEN the bundle details render
- THEN the application group SHALL still render its pull requests
- AND the promotion data for the bundle SHALL remain intact
- AND the GitOps group SHALL show an error state, not an empty success

#### Scenario: FLT-BPR-10.02 -- Successful empty comparison is not an error

- GIVEN a repository's bounded compare range succeeds and contains no pull requests
- WHEN that repository's group renders
- THEN it SHALL show an empty state, distinct from an unavailable state

### Requirement: FLT-BPR-11 -- Bundle-Details Rendering of the Two Repository Sections

The Fleet Map bundle details SHALL render the application and GitOps pull-request
groups as two sections, each with an independent count and independent empty and
error states. Each pull-request row SHALL use a repository-qualified key and a
repository-qualified link. The rendering SHALL retain the existing row truncation
and expand behavior, accessibility, and localization, and SHALL localize the section
and state labels.

#### Scenario: FLT-BPR-11.01 -- Repository-qualified keys and links

- GIVEN the bundle details render both groups
- WHEN pull-request rows are produced
- THEN each row's key SHALL be qualified by repository plus number
- AND each row's link SHALL target the pull request in its own repository

#### Scenario: FLT-BPR-11.02 -- Independent counts and states per section

- GIVEN one repository's group has pull requests and the other is empty
- WHEN the bundle details render
- THEN each section SHALL show its own count
- AND the empty section SHALL show its own empty or error state without affecting the
  other section

#### Scenario: FLT-BPR-11.03 -- Existing presentation behavior retained

- GIVEN a pull-request row with a long title
- WHEN it renders in either section
- THEN the existing truncation, expand, tooltip, accessibility, and localization
  behavior SHALL be preserved

### Requirement: FLT-BPR-12 -- Delivery Through the Independent Dashboard Image Track

This feature SHALL be delivered by building the `components/fleet-dashboard` image
and rolling it out through the existing independent dashboard image and Renovate
digest track. Updating this specification or the application release bundle alone
SHALL NOT deploy this feature.

#### Scenario: FLT-BPR-12.01 -- Spec or bundle change alone does not deploy

- GIVEN this specification is merged and an application release bundle is cut
- WHEN no new fleet-dashboard image is built and rolled out
- THEN the Glass bundle details SHALL NOT yet show the two repository groups
- AND delivery SHALL require the fleet-dashboard image to roll out through its own
  digest track
