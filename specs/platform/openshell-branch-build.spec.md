# OpenShell Branch Build Specification

## Purpose

HyperShell deploys and manages OpenShell gateways, but its local development and
test workflows always run against a pinned, published OpenShell release. This
spec defines a workflow that lets a developer stand up a Kind environment whose
gateways are built from an arbitrary OpenShell branch or pull request, so new
OpenShell changes can be validated end-to-end inside HyperShell before they are
released. A single entry point, `make kind-openshell-up`, checks out the
requested OpenShell source, builds the full set of OpenShell images (gateway,
supervisor, and sandbox base) from it, loads them into the Kind cluster, and
seeds a distinctly named, distinctly labeled "dev" gateway that runs those images
in place of the pinned defaults.

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
- `OPENSHELL_PR` - a convenience that resolves to the head ref of the given pull
  request number (e.g. `refs/pull/<n>/head`).
- `OPENSHELL_REPO` - the OpenShell repository URL, defaulting to the canonical
  upstream, so that forks and PR sources can be targeted.

The target SHALL require exactly one source ref (`OPENSHELL_BRANCH` or
`OPENSHELL_PR`) and SHALL fail with an actionable message when neither is
supplied.

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

The platform SHALL build the complete set of OpenShell images required to run a
gateway from the checked-out source: the gateway image, the supervisor image, and
the sandbox base image. It SHALL tag each image with a dev tag derived from the
resolved commit SHA so branch builds are distinguishable from pinned releases and
from one another, and SHALL load all built images into the Kind cluster.

#### Scenario: All three OpenShell images are built and loaded

- GIVEN a resolved OpenShell source checkout
- WHEN the platform builds OpenShell
- THEN it SHALL produce a gateway image, a supervisor image, and a sandbox base
  image from that source
- AND SHALL tag each with a tag that encodes the resolved commit
- AND SHALL load all three into the Kind cluster so no registry pull is required

### Requirement: Branch-Built Images Wired Into Provisioning

The seeded dev gateway SHALL be provisioned with the branch-built images rather
than the pinned defaults. The branch-built gateway image SHALL back the seeded
GatewayRelease and Gateway `image`, the branch-built supervisor image SHALL back
the Gateway `supervisor_image`, and the branch-built sandbox base image SHALL
back the gateway's sandbox default image.

The control plane SHALL reconcile the gateway workload so that the running
gateway container, supervisor sidecar, and launched sandboxes all use the
branch-built images.

#### Scenario: Dev gateway runs branch-built images

- GIVEN a completed OpenShell branch build with gateway, supervisor, and sandbox
  base images loaded into Kind
- WHEN the dev gateway is seeded and reconciled
- THEN the gateway container SHALL run the branch-built gateway image
- AND the supervisor sidecar SHALL run the branch-built supervisor image
- AND sandboxes launched by the gateway SHALL use the branch-built sandbox base
  image

### Requirement: Gateway Sandbox Image Field

A Gateway SHALL expose a `sandbox_image` provisioning field, peer to `image` and
`supervisor_image`, that specifies the sandbox base image the gateway uses when
launching sandboxes. When unset, the control plane SHALL apply the default
community sandbox base image so existing gateways are unchanged. When set, the
control plane SHALL substitute it into the generated gateway configuration in
place of the default.

#### Scenario: Sandbox image override is applied

- GIVEN a Gateway with `sandbox_image` set to a branch-built sandbox base image
- WHEN the control plane reconciles the gateway configuration
- THEN the gateway's sandbox default image SHALL be the specified image

#### Scenario: Sandbox image defaults when unset

- GIVEN a Gateway with no `sandbox_image` value
- WHEN the control plane reconciles the gateway configuration
- THEN the gateway's sandbox default image SHALL be the default community base
  image

### Requirement: Dev Gateway Identity

The gateway seeded by `kind-openshell-up` SHALL be identifiable as a dev/branch
build by both name and label, distinguishing it from gateways created by the
standard `kind-up` flow.

The seeded Gateway's name SHALL clearly denote a branch build rather than reuse
the standard `dev-gateway` name. The Kubernetes resources the control plane
creates for it SHALL carry a HyperShell-namespaced dev-build label
(`hypershell.redhat.io/openshell-dev-build: "true"`) so they can be selected with
a label query, and SHALL carry annotations recording the OpenShell branch/ref and
resolved commit SHA the images were built from. Gateways created by the standard
`kind-up` flow SHALL NOT carry the dev-build label.

#### Scenario: Dev gateway is distinctly named

- GIVEN a branch build for `my-feature`
- WHEN the dev gateway is seeded
- THEN its name SHALL indicate it is an OpenShell dev/branch build

#### Scenario: Dev-build label is selectable

- GIVEN a running branch-built gateway
- WHEN an operator queries workloads with the selector
  `hypershell.redhat.io/openshell-dev-build=true`
- THEN the gateway's Kubernetes resources SHALL be returned

#### Scenario: Provenance is recorded on the workload

- GIVEN a branch-built gateway built from `my-feature` at commit `abc123`
- WHEN the operator inspects the deployed resources
- THEN annotations SHALL record the branch/ref `my-feature` and the commit
  `abc123`

#### Scenario: Standard gateways are not labeled as dev builds

- GIVEN a gateway created by the standard `make kind-up` flow
- WHEN an operator queries with `hypershell.redhat.io/openshell-dev-build=true`
- THEN that gateway's resources SHALL NOT be returned

### Requirement: Coexistence With Standard Local Development

`kind-openshell-up` SHALL integrate with the existing Kind environment: it SHALL
reuse a running cluster when present and otherwise create one, and it SHALL leave
the standard `kind-up` behavior and pinned default images unchanged when invoked
without an OpenShell source ref. Standard teardown (`kind-down`, `kind-teardown`)
SHALL remove a branch-built environment the same way it removes a standard one.

#### Scenario: Reuse an existing cluster

- GIVEN a Kind cluster already created by `make kind-up`
- WHEN the developer runs `OPENSHELL_BRANCH=my-feature make kind-openshell-up`
- THEN the platform SHALL reuse the existing cluster rather than recreating it
- AND SHALL add or update the branch-built dev gateway within it

#### Scenario: Standard flow is unaffected

- GIVEN a developer who runs `make kind-up` with no OpenShell source ref
- WHEN the environment comes up
- THEN gateways SHALL use the pinned default OpenShell images
- AND no dev-build label SHALL be applied

### Requirement: Enablement for PR and Version Validation

The branch build workflow SHALL enable HyperShell's dev and E2E test workflows to
exercise an unreleased OpenShell version. Tests SHALL be able to target the
branch-built gateway via its dev-build label, and the workflow SHALL surface a
non-zero exit status when the OpenShell source cannot be fetched or built so that
failures are not silently masked.

#### Scenario: E2E run against a branch-built gateway

- GIVEN a branch-built gateway is running in the Kind cluster
- WHEN an E2E or dev workflow selects the gateway by its dev-build label
- THEN it SHALL exercise the branch-built OpenShell version

#### Scenario: Build failure is surfaced

- GIVEN an OpenShell ref that fails to fetch or build
- WHEN `make kind-openshell-up` runs
- THEN the target SHALL exit non-zero with an actionable error
- AND SHALL NOT seed a dev gateway that references missing images
