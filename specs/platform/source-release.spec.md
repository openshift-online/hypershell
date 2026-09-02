# Source Release Specification

## Purpose

This specification defines source releases for the HyperShell repository. It
defines conventional commit input, the managed release pull request, release
files, GitHub releases, and container build identity. In this specification, a
"release" is a source release of the HyperShell repository. It is not a
`GatewayRelease` API resource. Image promotion and deployment are outside this
specification. Argo image bump continues to control deployment image updates.

## Definitions

- **Release PR**: The long-lived pull request that proposes the next repository
  release.
- **Release version**: The SemVer value in the root `VERSION` file. The file does
  not include the `v` tag prefix.
- **Build version**: The version string embedded in one component image.
- **Revision**: The full Git commit SHA from which an image is built.
- **Short revision**: The first seven lowercase hexadecimal characters of the
  revision.

## Requirements

### Requirement REL-01: Conventional pull request titles

Each pull request that targets `main` SHALL have a title that follows the
Conventional Commits form `<type>(<optional-scope>)<optional-!>: <description>`.
The type and scope SHALL use lowercase characters. The repository SHALL check
the title in a required CI gate. A squash merge SHALL use the pull request title
as the commit subject on `main`.

An optional Jira key MAY be the first part of the description. It SHALL use the
form `[HYPERSHELL-<number>]`. The conventional type SHALL remain the first part
of the title. For example, `feat(api): [HYPERSHELL-123] add service status` is
valid. `[HYPERSHELL-123] feat(api): add service status` is not valid because
Release Please cannot parse that squash commit subject.

`fix` SHALL request a patch increment. `feat` SHALL request a minor increment.
The `!` marker or a `BREAKING CHANGE` footer SHALL request a major increment.
Before version `1.0.0`, a breaking change SHALL request a minor increment.
Other valid types MAY add changelog content, but they SHALL NOT request a
version increment by themselves.

#### Scenario: Invalid pull request title

- GIVEN a pull request targets `main`
- WHEN its title does not follow the required form
- THEN the required lint gate SHALL fail
- AND the failure SHALL show the required title form

#### Scenario: Feature increment

- GIVEN the current release version is `1.2.3`
- WHEN `feat(api): add service status` enters `main`
- THEN the Release PR SHALL propose version `1.3.0`

#### Scenario: Jira-linked pull request title

- GIVEN a pull request tracks Jira issue `HYPERSHELL-123`
- WHEN its title is `feat(api): [HYPERSHELL-123] add service status`
- THEN the required lint gate SHALL pass
- AND the squash commit subject SHALL keep the Jira key

### Requirement REL-02: Managed release pull request

Release Please SHALL maintain one Release PR against `main`. It SHALL open the
Release PR after a commit that requests a version increment enters `main`. It
SHALL update the open Release PR after later commits enter `main`. The Release
PR SHALL use a conventional commit title. It SHALL remain open until a person
selects it for release. The automation SHALL NOT enable automatic merge.

The Release PR is a release decision surface. It is not a SemVer prerelease,
and it SHALL NOT create an `-rc` tag.

Release Please SHALL use the repository `GITHUB_TOKEN` to create and update the
Release PR. GitHub SHALL hold the resulting CI workflow runs for manual
approval. A user with write access SHALL approve the workflow runs for the
current Release PR revision before merge.

#### Scenario: Work enters main while the Release PR is open

- GIVEN an open Release PR proposes version `0.4.0`
- WHEN another conforming commit enters `main`
- THEN Release Please SHALL update the same Release PR
- AND it SHALL update the proposed changelog
- AND it SHALL recalculate the proposed version when necessary
- AND CI workflow runs for the updated revision SHALL wait for manual approval
- AND it SHALL NOT create a GitHub release

#### Scenario: Maintenance-only work

- GIVEN no Release PR is open
- WHEN only commits that do not request a version increment enter `main`
- THEN Release Please SHALL NOT open a Release PR

### Requirement REL-03: Release files

The repository SHALL have root `VERSION` and `CHANGELOG.md` files. `VERSION`
SHALL contain one SemVer value without a `v` prefix. The Release PR SHALL update
`VERSION` to the proposed release version. It SHALL update `CHANGELOG.md` with
all conforming commits since the previous release. Changelog entries SHALL be
grouped by commit type.

Release Please SHALL also update the checked-in CI build prefix from the same
release version. A repository policy check SHALL fail when the CI build prefix
and `VERSION` do not identify the same release version.

#### Scenario: Release file consistency

- GIVEN a Release PR proposes version `1.5.0`
- WHEN its files are inspected
- THEN `VERSION` SHALL contain `1.5.0`
- AND the CI build prefix SHALL contain `v1.5.0`
- AND `CHANGELOG.md` SHALL contain the proposed `1.5.0` entries

### Requirement REL-04: Release publication

Only the merge of a Release PR SHALL authorize a repository release. After the
required `main` CI workflow succeeds, Release Please SHALL create a `vX.Y.Z` Git
tag and a published GitHub release. The tag SHALL identify the merged Release
PR commit. The GitHub release notes SHALL use the matching `CHANGELOG.md`
section. The repository SHALL have immutable GitHub releases enabled.

Release automation SHALL be safe to run again after a failure. It SHALL use a
repository `GITHUB_TOKEN` with write access only for contents, issues, and pull
requests. The repository SHALL allow GitHub Actions to create pull requests.
Each external GitHub Action SHALL use a full commit SHA.

#### Scenario: Ordinary merge

- GIVEN an ordinary pull request merges to `main`
- WHEN the release automation runs
- THEN it MAY create or update the Release PR
- AND it SHALL NOT create a GitHub release for that merge

#### Scenario: Release merge

- GIVEN the Release PR proposes version `1.5.0`
- WHEN the Release PR merges and the required `main` CI workflow succeeds
- THEN Release Please SHALL create tag `v1.5.0`
- AND it SHALL create one published immutable GitHub release for `v1.5.0`
- AND `VERSION` SHALL contain `1.5.0`

### Requirement REL-05: First release baseline

Release Please SHALL start from version `0.0.0`. The first proposed release
SHALL be `0.1.0` when the first included release change is a feature. The first
generated changelog SHALL include only commits after the release strategy
baseline. It SHALL NOT import the earlier repository history.

#### Scenario: First release proposal

- GIVEN no repository release tag exists
- AND the manifest records version `0.0.0`
- WHEN the first included `feat` commit enters `main`
- THEN the Release PR SHALL propose `0.1.0`
- AND its changelog SHALL stop at the configured baseline commit

### Requirement REL-06: Component-selective CI builds

A normal push to `main` SHALL build only the components whose build inputs
changed. An update to the long-lived Release PR SHALL NOT build every component
only because `VERSION`, `CHANGELOG.md`, or the CI build prefix changed. A
version-only merge-queue run SHALL NOT build every component.

When the Release PR merges to `main`, the changed `VERSION` file SHALL cause one
API server build, one control-plane build, and one web-console build from the
final `main` revision. The corresponding `main` end-to-end run SHALL wait for
these three builds. This rule SHALL apply only to a `main` push.

#### Scenario: Normal component change

- GIVEN the current release version is `1.5.0`
- WHEN a normal `main` commit changes only the API server
- THEN CI SHALL build the API server image
- AND it SHALL NOT build the control-plane or web-console image

#### Scenario: Release PR update

- GIVEN the Release PR remains open
- WHEN Release Please updates `VERSION`, `CHANGELOG.md`, or the CI build prefix
- THEN the release PR event SHALL NOT build all component images

#### Scenario: Release merge build

- GIVEN the Release PR changes `VERSION` from `1.5.0` to `1.6.0`
- WHEN that pull request merges to `main`
- THEN CI SHALL build the API server, control-plane, and web-console images once
- AND all three builds SHALL use the final release commit as their revision

### Requirement REL-07: Build version format

A supported local image build from a clean Git work tree SHALL use build
version `dev-<short-revision>`. A local build with staged, unstaged, or
untracked changes SHALL append `-modified`. A CI image build SHALL use build
version `v<release-version>-<short-revision>`. CI SHALL read the release version
from the checked-in release files. It SHALL NOT query for a live Git tag during
the image build. A build SHALL use the triggering full revision and SHALL
derive the short revision from it. Each image build SHALL reject `VCS_REF`
unless it is a 40-character lowercase hexadecimal Git SHA.

A normal CI build after a release SHALL continue to use that release version
until the next Release PR merges. Thus, different component builds MAY have
different short revisions in the same release version. The three builds caused
by a Release PR merge SHALL have the same build version.

#### Scenario: Local image build

- GIVEN local `HEAD` is revision `abcdef0123456789abcdef0123456789abcdef01`
- WHEN a developer uses a supported local image build command
- THEN the image build version SHALL be `dev-abcdef0`

#### Scenario: Modified local image build

- GIVEN local `HEAD` is revision `abcdef0123456789abcdef0123456789abcdef01`
- AND the local Git work tree contains an uncommitted change
- WHEN a developer uses a supported local image build command
- THEN the image build version SHALL be `dev-abcdef0-modified`

#### Scenario: CI image build

- GIVEN `VERSION` contains `1.6.0`
- AND the triggering revision is `1234567890abcdef1234567890abcdef12345678`
- WHEN CI builds a component image
- THEN its build version SHALL be `v1.6.0-1234567`

#### Scenario: Reject an invalid image revision

- GIVEN `VCS_REF` is `abcdef0`
- WHEN a component image build starts
- THEN the build SHALL stop before it shortens the revision
- AND the build SHALL report that a full 40-character Git SHA is required

### Requirement REL-08: Container metadata

Each API server, control-plane, and web-console image SHALL set
`org.opencontainers.image.version` to its build version. It SHALL set
`org.opencontainers.image.revision` to its full revision. The image SHALL also
contain the build version as a runtime environment value. Static placeholder
versions SHALL NOT remain in these Containerfiles. The Containerfiles SHALL
explain how they shorten the full revision.

#### Scenario: Inspect a CI image

- GIVEN CI built an image with build version `v1.6.0-1234567`
- WHEN an operator inspects the image configuration
- THEN `org.opencontainers.image.version` SHALL be `v1.6.0-1234567`
- AND `org.opencontainers.image.revision` SHALL be the full triggering revision
- AND the runtime build-version value SHALL be `v1.6.0-1234567`

### Requirement REL-09: API service metadata

The existing `GET /api/hypershell/v1/metadata` endpoint SHALL return the API
server build version and build time. The `version` value SHALL equal the API
server image build version. The endpoint SHALL be available without user
authentication and SHALL NOT query the database. The OpenAPI contract SHALL
describe its response. Existing liveness and readiness probes SHALL keep their
current paths and meanings. CI SHALL link a focused test with the production
linker flags. The test SHALL fail when the linked version or build time does not
equal its expected value.

#### Scenario: Read API build identity

- GIVEN the API server image build version is `v1.6.0-1234567`
- WHEN a client sends an unauthenticated `GET` request to
  `/api/hypershell/v1/metadata`
- THEN the response status SHALL be `200`
- AND the response `version` SHALL be `v1.6.0-1234567`
- AND the response SHALL include `build_time`

#### Scenario: Detect an invalid linker target

- GIVEN a linker target does not set the framework build identity
- WHEN CI runs the linked build-metadata test
- THEN the test SHALL fail

### Requirement REL-10: Web-console version display

The web-console image SHALL give its build version to the BFF as runtime
configuration. The BFF SHALL include only this non-secret value in the existing
browser runtime-configuration allowlist. The authenticated user menu SHALL show
the localized text `Console version <build-version>` as non-action content. It
SHALL get the API server build version from the existing metadata endpoint
through the same-origin BFF proxy. It SHALL show the localized text
`API version <build-version>` as separate non-action content. The menu SHALL
remain usable when either value is unavailable and SHALL show a localized
unknown value for that source.

The two rows SHALL identify their source. The console SHALL NOT state or imply
that the console and API server use the same revision.

#### Scenario: Show both image build versions

- GIVEN an authenticated user uses console build `v1.6.0-1234567`
- AND the API server returns build version `v1.6.0-7654321`
- WHEN the user opens the user menu
- THEN the menu SHALL show `Console version v1.6.0-1234567`
- AND the menu SHALL show `API version v1.6.0-7654321`
- AND neither version row SHALL start navigation or another action

#### Scenario: Build versions are unavailable

- GIVEN browser runtime configuration has no valid build version
- AND the API metadata request is unavailable
- WHEN an authenticated user opens the user menu
- THEN the menu SHALL show a localized unknown console version
- AND the menu SHALL show a localized unknown API version
- AND the Log out action SHALL remain available

### Requirement REL-11: Registry and deployment scope

CI SHALL keep the existing full-SHA image tags that current tests and Argo image
bump consume. This release strategy SHALL NOT create semantic registry aliases,
promote images, change a Konflux ReleasePlan, or change Argo configuration.
Image promotion and deployment approval SHALL remain separate processes.

#### Scenario: Release images become available

- GIVEN a Release PR merges to `main`
- WHEN the three component builds finish
- THEN each existing component repository SHALL contain an image with the full
  release commit SHA tag
- AND this change SHALL NOT update an Argo deployment reference

### Requirement REL-12: Verification and documentation

Repository checks SHALL verify conventional title validation, release-file
consistency, build-version calculation, API metadata, browser runtime
configuration, user-menu presentation, and event-specific image selection.
Release documentation SHALL describe commit types, release PR operation,
`GITHUB_TOKEN` permissions, manual workflow approval, the manual merge step,
first-release behavior, failure recovery, and the boundary with Argo image
bump.

#### Scenario: Release automation fails after merge

- GIVEN a Release PR has merged
- AND release automation fails before it creates the tag or GitHub release
- WHEN an operator corrects the cause and runs the automation again
- THEN the automation SHALL create at most one tag and one GitHub release
- AND it SHALL use the version already committed in `VERSION`
