# OpenShell Branch Build Specification

## Purpose

HyperShell deploys and manages OpenShell gateways, but its local development and
test workflows always run against a pinned, published OpenShell release. This
spec defines a workflow that lets a developer stand up a Kind environment whose
gateways are built from an arbitrary OpenShell branch or pull request, so new
OpenShell changes can be validated end-to-end inside HyperShell before they are
released. A single entry point, `make kind-openshell-up`, checks out the
requested OpenShell source, builds gateway and supervisor images from it, loads
them into the Kind cluster, and provisions a distinctly named gateway
(`openshell-dev-gateway`) that runs those images. This gateway coexists with the
standard `dev-gateway` created by `make kind-up`.

This feature is a local-development and testing capability. It reuses the
existing Kind environment ([`local-development.spec.md`](./local-development.spec.md))
and gateway provisioning flow ([`openshell-gateway.spec.md`](./openshell-gateway.spec.md));
it does not change how production gateways are provisioned.

## Requirements

### Requirement: Branch Build Entry Point

The platform SHALL provide a `make kind-openshell-up` target that brings up a
local Kind environment whose OpenShell gateways are built from a caller-specified
OpenShell source ref instead of the pinned default images.

The target SHALL accept the OpenShell source through configuration variables and
SHALL NOT require code changes to select a different branch, PR, or repository:

- `OPENSHELL_BRANCH` - the git ref (branch name, tag, or commit) to build from.
- `OPENSHELL_PR` - a pull request number that resolves to `refs/pull/<n>/head`.
  This ref exists only in the repository that received the PR (not in forks),
  so `OPENSHELL_PR` SHALL only be used against the repository hosting the pull
  request. When `OPENSHELL_REPO` points to a fork and `OPENSHELL_PR` is set,
  the fetch SHALL fail with an actionable message directing the user to either
  use `OPENSHELL_BRANCH` with the PR's head branch name or fetch from the
  canonical upstream.
- `OPENSHELL_REPO` - the OpenShell repository URL, defaulting to
  `https://github.com/NVIDIA/OpenShell.git`, so that forks and alternate
  sources can be targeted.

The target SHALL require exactly one source ref. When both `OPENSHELL_BRANCH`
and `OPENSHELL_PR` are set, `OPENSHELL_BRANCH` SHALL take precedence and
`OPENSHELL_PR` SHALL be ignored. When neither is set, the target SHALL fail
with an actionable message explaining how to supply a branch or PR.

#### Scenario: Bring up a Kind environment from an OpenShell branch

- GIVEN a developer with a valid `OPENSHELL_BRANCH` value
- WHEN they run `OPENSHELL_BRANCH=my-feature make kind-openshell-up`
- THEN the platform SHALL build OpenShell from `my-feature`
- AND SHALL bring up (or reuse) a Kind cluster running the branch-built gateway

#### Scenario: Target an OpenShell pull request

- GIVEN a developer who wants to validate OpenShell PR #123
- WHEN they run `OPENSHELL_PR=123 make kind-openshell-up`
- THEN the platform SHALL resolve the PR head ref and build OpenShell from it

#### Scenario: Missing source ref is rejected

- GIVEN neither `OPENSHELL_BRANCH` nor `OPENSHELL_PR` is set
- WHEN the developer runs `make kind-openshell-up`
- THEN the target SHALL fail before building or mutating the cluster
- AND SHALL print how to supply a branch or PR

#### Scenario: Both source refs are set

- GIVEN both `OPENSHELL_BRANCH` and `OPENSHELL_PR` are set
- WHEN the developer runs `make kind-openshell-up`
- THEN the platform SHALL use `OPENSHELL_BRANCH` and ignore `OPENSHELL_PR`

#### Scenario: PR ref against a fork is rejected

- GIVEN `OPENSHELL_REPO` pointing to a fork and `OPENSHELL_PR` set
- WHEN the developer runs `make kind-openshell-up`
- THEN the target SHALL fail before fetching
- AND SHALL explain that PR refs only exist in the repository hosting the pull request

#### Scenario: Override the OpenShell repository

- GIVEN a developer building from a fork
- WHEN they run `OPENSHELL_REPO=https://github.com/acme/openshell.git OPENSHELL_BRANCH=wip make kind-openshell-up`
- THEN the platform SHALL fetch the source from the specified repository

### Requirement: Deterministic Source Checkout

The platform SHALL fetch the OpenShell source into an isolated, disposable
location and check out the requested ref without mutating the developer's working
tree, mirroring the external-source build pattern already used for
`cloud-provider-kind`.

The platform SHALL resolve the requested ref to a concrete commit SHA, SHALL
record the resolved SHA, and SHALL rebuild whenever the requested ref resolves to
a commit different from the one currently deployed, so that re-running the target
against a moved branch tip picks up new commits.

#### Scenario: Branch tip advances between runs

- GIVEN a prior `kind-openshell-up` built commit `abc123` from branch `my-feature`
- AND new commits have since landed on `my-feature`
- WHEN the developer re-runs `OPENSHELL_BRANCH=my-feature make kind-openshell-up`
- THEN the platform SHALL fetch the current tip, resolve its SHA, and rebuild
- AND SHALL redeploy the gateway with the newly built images

#### Scenario: Resolved commit is recorded

- GIVEN a successful branch build
- WHEN the build completes
- THEN the platform SHALL record the concrete commit SHA it built from
- AND SHALL surface that SHA to the developer

### Requirement: OpenShell Image Set Built From Source

The platform SHALL build the gateway and supervisor images from the checked-out
OpenShell source using the repository's `docker-build-image.sh` script, which
produces the `gateway` and `supervisor` targets. The sandbox base image SHALL
use the published community image (`ghcr.io/nvidia/openshell-community/sandboxes/base:latest`)
since it is maintained in a separate repository (NVIDIA/OpenShell-Community) not
covered by this checkout.

Each branch-built image SHALL be tagged with a dev tag that encodes the resolved
commit SHA (e.g., `gateway:dev-abc123`, `supervisor:dev-abc123`) so branch builds
are distinguishable from pinned releases and from one another. The platform SHALL
load the branch-built gateway and supervisor images into the Kind cluster. The
sandbox base image SHALL either be pre-pulled into Kind or pulled on first sandbox
launch.

The image references written into the seeded Gateway resource SHALL be:
- `image`: `gateway:dev-<commit-sha>`
- `supervisor_image`: `supervisor:dev-<commit-sha>`
- `sandbox_image`: `ghcr.io/nvidia/openshell-community/sandboxes/base:latest`

#### Scenario: Gateway and supervisor images are built and loaded

- GIVEN a resolved OpenShell source checkout
- WHEN the platform builds OpenShell via `docker-build-image.sh`
- THEN it SHALL produce a gateway image and a supervisor image from that source
- AND SHALL tag each as `gateway:dev-<sha>` and `supervisor:dev-<sha>`
- AND SHALL load both into the Kind cluster so no registry pull is required

#### Scenario: Sandbox base uses published community image

- GIVEN a branch-built gateway
- WHEN the gateway is provisioned
- THEN its `sandbox_image` field SHALL reference the published community base image
- AND sandboxes launched by the gateway SHALL use that published image

### Requirement: Branch-Built Images Wired Into Provisioning

The seeded dev gateway SHALL be provisioned with the branch-built images rather
than the pinned defaults. The branch-built gateway and supervisor images SHALL
be set via the Gateway's `image` and `supervisor_image` fields directly. The
Gateway MUST NOT have a `release_id` set, because per `data-model.spec.md`,
when both `release_id` and `image` are present, `release_id` takes precedence
and the direct image references would be ignored. The `sandbox_image` field
SHALL reference the published community base image.

The control plane SHALL reconcile the gateway workload so that the running
gateway container and supervisor sidecar use the branch-built images, and launched
sandboxes use the community base image.

#### Scenario: Dev gateway runs branch-built images

- GIVEN a completed OpenShell branch build with gateway and supervisor images
  loaded into Kind
- WHEN the dev gateway is seeded and reconciled
- THEN the Gateway resource SHALL have no `release_id` field set
- AND the gateway container SHALL run `gateway:dev-<sha>`
- AND the supervisor sidecar SHALL run `supervisor:dev-<sha>`
- AND sandboxes launched by the gateway SHALL use the published community base
  image

### Requirement: Sandbox Image Configuration

The Gateway's `sandbox_image` field (defined in [`openshell-gateway.spec.md`](./openshell-gateway.spec.md))
SHALL be set to the published community base image for branch-built gateways,
since the sandbox base is maintained in a separate repository and cannot be built
from the OpenShell checkout.

### Requirement: Dev Gateway Identity

The gateway seeded by `kind-openshell-up` SHALL be identifiable as a dev/branch
build, distinguishing it from gateways created by the standard `kind-up` flow.

The seeded Gateway resource SHALL be named `openshell-dev-gateway` (a stable,
DNS-valid name that allows update-or-create semantics on subsequent runs). The
control plane SHALL copy Gateway-level metadata onto the Kubernetes workload
resources it creates. The seeding flow SHALL set two new Gateway fields to mark
branch builds:

- `dev_build`: boolean field set to `true` for branch-built gateways, unset
  (or `false`) otherwise. The control plane SHALL copy this to a label
  `hypershell.redhat.io/openshell-dev-build: "true"` on Deployment, Service,
  and other K8s resources it provisions.
- `dev_build_metadata`: JSONB field carrying provenance:
  `{ref: "<branch/PR>", sha: "<commit>", repo: "<repo-url>"}`. The control plane
  SHALL copy this to annotations `hypershell.redhat.io/openshell-dev-build-ref`,
  `hypershell.redhat.io/openshell-dev-build-sha`, and
  `hypershell.redhat.io/openshell-dev-build-repo` on the K8s resources.

This survives reconcile because the control plane reads the Gateway fields and
re-applies them on every reconciliation. Gateways created by the standard
`kind-up` flow SHALL NOT have `dev_build` set.

#### Scenario: Dev gateway has a stable name

- GIVEN any branch build
- WHEN the dev gateway is seeded
- THEN its Gateway name SHALL be `openshell-dev-gateway`
- AND subsequent runs SHALL update the existing Gateway rather than creating duplicates

#### Scenario: Dev-build label is propagated to K8s resources

- GIVEN a Gateway with `dev_build: true`
- WHEN the control plane reconciles it
- THEN the Deployment, Service, and other K8s resources SHALL carry the label
  `hypershell.redhat.io/openshell-dev-build: "true"`

#### Scenario: Dev-build label is selectable

- GIVEN a running branch-built gateway
- WHEN an operator queries workloads with the selector
  `hypershell.redhat.io/openshell-dev-build=true`
- THEN the gateway's Kubernetes resources SHALL be returned

#### Scenario: Provenance is recorded on the workload

- GIVEN a branch-built gateway built from branch `my-feature` at commit `abc123`
  from `https://github.com/NVIDIA/OpenShell.git`
- WHEN the operator inspects the deployed Deployment
- THEN annotations SHALL record:
  - `hypershell.redhat.io/openshell-dev-build-ref: "my-feature"`
  - `hypershell.redhat.io/openshell-dev-build-sha: "abc123"`
  - `hypershell.redhat.io/openshell-dev-build-repo: "https://github.com/NVIDIA/OpenShell.git"`

#### Scenario: Standard gateways are not labeled as dev builds

- GIVEN a gateway created by the standard `make kind-up` flow (which does not set `dev_build`)
- WHEN an operator queries with `hypershell.redhat.io/openshell-dev-build=true`
- THEN that gateway's resources SHALL NOT be returned

### Requirement: Coexistence With Standard Local Development

`kind-openshell-up` SHALL integrate with the existing Kind environment: it SHALL
reuse a running cluster when present (including the full stack: API server, control
plane, Keycloak, and the standard `dev-gateway`), and otherwise create the complete
Kind environment before provisioning the branch-built gateway. The branch-built
gateway (`openshell-dev-gateway`) SHALL coexist with the standard `dev-gateway`,
allowing both to run simultaneously. Standard teardown (`kind-down`, `kind-teardown`)
SHALL remove both gateways.

The standard `make kind-up` target SHALL remain unchanged and SHALL NOT invoke
branch build logic.

#### Scenario: Reuse an existing cluster with full stack

- GIVEN a Kind cluster already created by `make kind-up` with API server, control
  plane, Keycloak, and `dev-gateway` running
- WHEN the developer runs `OPENSHELL_BRANCH=my-feature make kind-openshell-up`
- THEN the platform SHALL reuse the existing cluster and stack
- AND SHALL provision or update the `openshell-dev-gateway` alongside `dev-gateway`

#### Scenario: Create full stack when cluster does not exist

- GIVEN no Kind cluster exists
- WHEN the developer runs `OPENSHELL_BRANCH=my-feature make kind-openshell-up`
- THEN the platform SHALL create the Kind cluster with the full stack (API server,
  control plane, Keycloak, `dev-gateway`)
- AND SHALL additionally provision the `openshell-dev-gateway` with branch-built images

#### Scenario: Standard flow is unaffected

- GIVEN a developer who runs `make kind-up`
- WHEN the environment comes up
- THEN only `dev-gateway` SHALL be created with the pinned default OpenShell images
- AND `openshell-dev-gateway` SHALL NOT be created
- AND no `dev_build` field SHALL be set on any Gateway

### Requirement: Enablement for PR and Version Validation

The branch build workflow SHALL enable HyperShell's dev and E2E test workflows to
exercise an unreleased OpenShell version. Tests SHALL be able to target the
branch-built gateway via its stable name (`openshell-dev-gateway`) or by querying
for Gateways with `dev_build: true`. The workflow SHALL surface a non-zero exit
status when the OpenShell source cannot be fetched or built so that failures are
not silently masked.

#### Scenario: E2E run against a branch-built gateway

- GIVEN a branch-built gateway named `openshell-dev-gateway` is running in the Kind cluster
- WHEN an E2E or dev workflow targets the gateway by name or queries for gateways
  with `dev_build: true`
- THEN it SHALL exercise the branch-built OpenShell version

#### Scenario: Build failure is surfaced

- GIVEN an OpenShell ref that fails to fetch or build
- WHEN `make kind-openshell-up` runs
- THEN the target SHALL exit non-zero with an actionable error
- AND SHALL NOT seed a dev gateway that references missing images
