# OpenShift Development and Testing Specification

**Date:** 2026-08-19
**Status:** Draft
**Jira:** HYPERSHELL-44
**Related:** `local-development.spec.md` -- Kind lifecycle and component swap;
             `e2e-testing.spec.md` -- driver interface contract and CI pipeline;
             `control-plane.spec.md` -- reconciler behavior;
             `openshell-gateway-routing.spec.md` -- GRPCRoute provisioning

## Purpose

HyperShell developers must build, deploy, and test the full stack on OpenShift
with the same commands and the same workflow that they use for Kind. This spec
defines a driver model for the cluster lifecycle. The `make openshift-up` command
deploys a complete HyperShell environment to an ephemeral namespace on an
OpenShift cluster. The developer can swap one component at a time from the working
tree, exactly as `make kind-<component>-up` does today.

This spec also defines automated end-to-end testing on OpenShift. It defines the
`tests/e2e/drivers/openshift.sh` e2e driver that `e2e-testing.spec.md` leaves as
follow-up work. It defines a CI workflow that deploys HyperShell to an ephemeral
namespace for each pull request, runs the e2e suite, gives the environment access
details to the developer, keeps the environment alive for the life of the pull
request, and releases the environment when the pull request merges or closes. It
defines the consolidation of the legacy `components/pr-test/e2e-openshell.sh`
script into the shared e2e harness.

HyperShell uses one ephemerality model -- an ephemeral namespace on an existing
OpenShift cluster -- in two contexts:

- **Local development** -- The developer supplies a target OpenShift cluster. The
  `make openshift-up` command deploys into an isolated namespace on that cluster.
- **Pull request CI** -- The pipeline deploys into an ephemeral namespace on a
  shared target environment that has capacity for several simultaneous
  pull-request namespaces. The namespace lives for the life of the pull request,
  so it also serves as a live environment to debug e2e failures and to do other
  work.

HyperShell does not provision full clusters for CI. A full cluster (for example a
ROSA cluster) adds 15-20 minutes of provisioning time per run; an ephemeral
namespace on a warm target environment avoids that cost. The namespace MAY be
provisioned through the
[ephemeral-namespace-operator](https://github.com/RedHatInsights/ephemeral-namespace-operator)
or an equivalent mechanism.

Both contexts use the same lifecycle scripts and the same e2e driver.

### Scope

This spec covers the OpenShift lifecycle driver, the OpenShift e2e driver, the
ephemeral-namespace CI workflow, the environment-access handoff to the developer,
and the reconciliation of the OpenShift deploy overlay against a production
reference.
This spec does not change the Kind driver, the Kind lifecycle scripts, or the Kind
CI job. This spec does not redesign the e2e driver interface contract that
`e2e-testing.spec.md` defines; it implements the OpenShift side of that contract.

This spec is a behavior contract. It does not contain the driver code or the CI
YAML. Those are follow-up implementation work.

### Reserved Terms

This spec adds no new domain kinds. It refers to the existing kinds (Fleet,
Gateway, GatewayNetwork, GatewayRelease, ManagedCluster, ManagedDatabase) only
where a scenario provisions one.

## Architecture

The lifecycle scripts follow the same driver model that the e2e tests use. The
root `Makefile` stays the single entry point. Shared lifecycle logic stays
infrastructure-agnostic. Each infrastructure target supplies a driver that
implements the infrastructure-specific operations.

```
Makefile (single entry point)
    │
    ├── cluster lifecycle (infra-agnostic)
    │       │
    │       ├── sources scripts/cluster/lib.sh (shared seams)
    │       │
    │       └── selects driver via CLUSTER_DRIVER
    │               ├── scripts/cluster/drivers/kind.sh       (wraps today's scripts/kind/)
    │               └── scripts/cluster/drivers/openshift.sh  (this spec)
    │
    └── e2e tests (infra-agnostic, unchanged contract)
            │
            └── selects driver via E2E_INFRA_DRIVER
                    ├── tests/e2e/drivers/kind.sh
                    └── tests/e2e/drivers/openshift.sh         (this spec)
```

The lifecycle driver and the e2e driver are two different interfaces for two
different jobs. The lifecycle driver creates and removes the environment. The e2e
driver discovers and drives the running environment during tests. A single
infrastructure target supplies one of each.

## Requirements

### Requirement: Cluster Lifecycle Driver Abstraction

The lifecycle scripts SHALL separate infrastructure-agnostic logic from
infrastructure-specific logic through a driver model. A `CLUSTER_DRIVER` variable
SHALL select the driver. The default value SHALL be `kind`, so that existing Kind
commands keep their current behavior. Each driver SHALL implement a fixed set of
lifecycle operations: `cluster_up`, `cluster_down`, `cluster_teardown`,
`cluster_status`, `component_swap`, and `component_revert`.

The shared lifecycle library SHALL centralize the seams that differ per
infrastructure, the same way `scripts/kind/lib.sh` centralizes the Kubernetes
context and the swap tracking today. The Kind driver SHALL reuse the existing
`scripts/kind/` logic without behavior change.

Each infrastructure target SHALL use its own driver for both lifecycle operations
and e2e operations. The Kind target SHALL use the Kind lifecycle driver and the
Kind e2e driver (`tests/e2e/drivers/kind.sh`). The OpenShift target SHALL use the
OpenShift lifecycle driver and the OpenShift e2e driver
(`tests/e2e/drivers/openshift.sh`). The lifecycle driver and the e2e driver for a
target SHALL agree on the same infrastructure conventions, so that the environment
the lifecycle driver creates is the environment the e2e driver expects.

#### Scenario: Default driver preserves Kind behavior

- GIVEN a developer runs `make kind-up` with no `CLUSTER_DRIVER` set
- WHEN the lifecycle scripts run
- THEN the scripts use the Kind driver
- AND the deployment result is identical to the current Kind behavior

#### Scenario: A new infrastructure target adds only a driver

- GIVEN the lifecycle driver contract is defined
- WHEN a developer adds support for a new infrastructure target
- THEN the developer adds one lifecycle driver file that implements the fixed
  operations
- AND the developer does not change the infrastructure-agnostic lifecycle logic

### Requirement: OpenShift Lifecycle Up and Down

The `make openshift-up` command SHALL deploy the full HyperShell stack to an
ephemeral namespace on a target OpenShift cluster with a single command, the same
way `make kind-up` deploys to Kind. The command SHALL connect to the cluster that
the developer's current kubeconfig context selects. The command SHALL NOT create
the OpenShift cluster; the cluster is a precondition for the ephemeral-namespace
workflow. When no target OpenShift cluster is available, the command SHALL stop
with a clear error that tells the developer to provide an OpenShift cluster target,
rather than deploy nothing or fail without guidance.

The `make openshift-up` command SHALL deploy through `kustomize build
deploy/openshift/` so that the deployed resources match the blessed overlay. The
`make openshift-down` command SHALL remove the deployment, including every
namespace in the environment namespace group. The `make openshift-status` command
SHALL report the cluster, the environment namespaces, the pods, the services, the
Routes, the Gateway status, and the component swap state, the same categories that
`make kind-status` reports.

The command names SHALL mirror the Kind command names by replacing the `kind`
prefix with `openshift`.

#### Scenario: Deploy the full stack to OpenShift

- GIVEN a developer has a kubeconfig context for an OpenShift cluster
- AND the developer has permission to create a namespace
- WHEN the developer runs `make openshift-up`
- THEN the scripts deploy the API server, the control plane, the web console,
  PostgreSQL, and Keycloak through `kustomize build deploy/openshift/`
- AND the scripts report the API Route, the web-console Route, and the Keycloak
  Route when the deployment is ready

#### Scenario: Remove the deployment

- GIVEN a HyperShell deployment exists from `make openshift-up`
- WHEN the developer runs `make openshift-down`
- THEN the scripts remove the HyperShell resources from every namespace in the
  environment
- AND the scripts do not delete resources that belong to other environments or to
  cluster infrastructure

#### Scenario: No target cluster is available

- GIVEN the developer has no reachable OpenShift cluster in the current kubeconfig
  context
- WHEN the developer runs `make openshift-up`
- THEN the command stops with an error that asks for an OpenShift cluster target
- AND the command does not deploy any resources

### Requirement: Ephemeral Namespace Isolation

Each `make openshift-up` deployment SHALL isolate into an environment namespace
group so that more than one developer can share one OpenShift cluster without
collision. An `OPENSHIFT_NAMESPACE` variable SHALL name the platform namespace, the
same way `KIND_NAMESPACE` names the Kind target namespace. The scripts SHALL derive
the `-keycloak` namespace from that name (see the Keycloak Namespace requirement).
The scripts SHALL create the namespaces if they do not exist.

The scripts SHALL label every namespace in the group so that `make openshift-status`
and cleanup tooling can find every namespace that belongs to a HyperShell
deployment, and can tell which namespaces belong to the same environment. The
scripts SHALL derive per-tenant gateway hostnames from the cluster base domain and
the platform namespace, so that two deployments on one cluster do not share a
hostname.

#### Scenario: Two developers share one cluster

- GIVEN developer A runs `make openshift-up` with `OPENSHIFT_NAMESPACE=alice`
- AND developer B runs `make openshift-up` with `OPENSHIFT_NAMESPACE=bob`
- WHEN both deployments are ready
- THEN each deployment runs in its own namespace group
- AND each deployment has distinct gateway hostnames
- AND neither deployment changes, conflicts with, or interacts with the other

#### Scenario: Namespace cleanup removes only one deployment

- GIVEN two HyperShell environment namespace groups exist on one cluster
- WHEN a developer runs `make openshift-down` for one environment
- THEN the scripts remove only that environment's namespaces
- AND the other environment stays intact

### Requirement: Keycloak Namespace

The deployment SHALL place Keycloak in its own namespace, separate from the other
HyperShell components, the same way the Kind environment runs Keycloak in a
dedicated `keycloak` namespace. The OpenShift deployment SHALL name this namespace
`${OPENSHIFT_NAMESPACE}-keycloak`, so that each ephemeral environment has its own
Keycloak, and two environments on one cluster do not share a Keycloak or collide on
a fixed namespace name. Every other HyperShell component SHALL deploy into the
platform namespace that `OPENSHIFT_NAMESPACE` names.

Together, the platform namespace and its `-keycloak` namespace form the
deployment's namespace group. The two namespaces SHALL share one lifecycle: the
scripts create them together, and `make openshift-down` (for local development) or
the release step (for CI) removes them together. The OpenShift OIDC issuer SHALL
point at the Keycloak route in the `-keycloak` namespace.

This spec defines only where Keycloak lands. The broader isolation of other
non-request-serving components (for example the database and observability) into
their own namespaces is out of scope here and belongs to a separate spec.

#### Scenario: Keycloak runs in its own namespace

- GIVEN a developer runs `make openshift-up` with `OPENSHIFT_NAMESPACE=alice`
- WHEN the deployment is ready
- THEN Keycloak runs in the `alice-keycloak` namespace
- AND every other HyperShell component runs in the `alice` namespace
- AND the OIDC issuer points at the Keycloak route in `alice-keycloak`

#### Scenario: The namespace group shares one lifecycle

- GIVEN a deployment has a platform namespace and its `-keycloak` namespace
- WHEN the deployment is removed
- THEN both namespaces are removed together
- AND the `-keycloak` namespace is not left behind

### Requirement: Component Swap on OpenShift

A developer SHALL be able to swap one component at a time from the working tree
into the OpenShift deployment, the same way `make kind-<component>-up` swaps a
component into the Kind deployment. The commands SHALL be
`make openshift-api-server-up`, `make openshift-control-plane-up`, and
`make openshift-web-console-up`, with matching `-down` commands that revert the
component to the baseline registry image.

The swap SHALL build the component image from the working tree and make the image
available to the cluster. Because OpenShift pulls images from a registry rather
than from a local archive, the OpenShift driver SHALL push the built image to a
registry that the cluster can pull, rather than use the Kind-specific
`kind load image-archive`. The driver SHOULD use the OpenShift internal registry
when it is available, so that the swap does not require an external registry.

The scripts SHALL track the swap state per namespace, the same way `.kind-swaps`
tracks the Kind swap state, so that `make openshift-status` reports which
components run a working-tree build and which run the baseline image.

#### Scenario: Swap the API server from the working tree

- GIVEN a HyperShell deployment exists on OpenShift with baseline images
- WHEN the developer runs `make openshift-api-server-up`
- THEN the scripts build the API server image from the working tree
- AND the scripts push the image to a registry that the cluster can pull
- AND the scripts update the API server deployment to use the pushed image
- AND `make openshift-status` reports the API server as a working-tree build

#### Scenario: Revert a swapped component

- GIVEN the API server runs a working-tree build on OpenShift
- WHEN the developer runs `make openshift-api-server-down`
- THEN the scripts update the API server deployment to use the baseline registry
  image
- AND `make openshift-status` reports the API server as a baseline image

### Requirement: OpenShift E2E Driver

The `tests/e2e/drivers/openshift.sh` file SHALL implement the e2e driver interface
that `e2e-testing.spec.md` defines. The driver SHALL implement every required
function: `discover_api_host`, `discover_gateway_endpoint`, `get_cluster_domain`,
`get_cli_binary`, `wait_for_gateway_route`, `acquire_oidc_token`, and `api_curl`.
The driver SHALL return values through the global variables that the contract
defines (`_DISCOVER_API_HOST`, `_DISCOVER_GW_ENDPOINT`, `_OIDC_ACCESS_TOKEN`), so
that background processes survive in the parent shell.

The driver SHALL implement each function with OpenShift constructs:

- `discover_api_host` SHALL read the API Route host with
  `oc get route hypershell-api -o jsonpath='{.spec.host}'`.
- `discover_gateway_endpoint` SHALL find the gateway endpoint from the
  passthrough Route that targets the gateway service.
- `get_cluster_domain` SHALL read the cluster base domain with
  `oc get ingresses.config.openshift.io cluster -o jsonpath='{.spec.domain}'`.
- `get_cli_binary` SHALL return `oc`.
- `wait_for_gateway_route` SHALL wait until the OpenShift Route reports
  `Admitted` in `.status.ingress[].conditions`, and until the gateway GRPCRoute
  parent reports `Accepted`.

The driver SHALL set the OIDC issuer and related OIDC variables from the running
cluster's domain, rather than the Kind default `keycloak.hypershell.localhost`.
The driver SHALL provide the Keycloak admin and role-assignment helpers that the
e2e RBAC scenarios need, the same helpers that the Kind driver provides.

The OpenShift e2e suite SHALL run with `E2E_INFRA_DRIVER=openshift bash
tests/e2e/e2e-openshell.sh`, and SHALL exercise the same test areas that the Kind
suite exercises, so that a single suite validates both infrastructure targets.

#### Scenario: Discover the API host on OpenShift

- GIVEN a HyperShell deployment is ready on OpenShift
- WHEN the e2e suite calls `discover_api_host` with `E2E_INFRA_DRIVER=openshift`
- THEN the driver sets `_DISCOVER_API_HOST` to the HTTPS URL of the API Route
- AND a request to the API URL returns an HTTP response

#### Scenario: Same suite runs on both drivers

- GIVEN the e2e test logic is infrastructure-agnostic
- WHEN a developer runs the suite with `E2E_INFRA_DRIVER=openshift`
- THEN the suite runs the same test areas that it runs with
  `E2E_INFRA_DRIVER=kind`
- AND the test logic is not changed between the two runs

#### Scenario: Gateway route readiness uses OpenShift Route status

- GIVEN the control plane provisions a Gateway and a GRPCRoute
- WHEN the e2e suite calls `wait_for_gateway_route`
- THEN the driver waits until the OpenShift Route reports `Admitted`
- AND the driver waits until the GRPCRoute parent reports `Accepted`
- AND the driver returns success only when both conditions are true

### Requirement: E2E Script Consolidation

The legacy `components/pr-test/e2e-openshell.sh` script SHALL be consolidated into
the shared e2e harness. The OpenShift-specific logic in that script SHALL move into
`tests/e2e/drivers/openshift.sh`. The infrastructure-agnostic test logic SHALL use
the shared `tests/e2e/e2e-openshell.sh` suite. After the consolidation, no
OpenShift e2e logic SHALL remain hardcoded outside the driver model.

The `components/pr-test/` component SHALL be removed after the consolidation. The
removal SHALL update every reference to `components/pr-test/`, including the
`pr_test` entry in `.github/component-paths.json` and any CI workflow that runs the
legacy script, so that no reference points to a removed path.

#### Scenario: Legacy script is consolidated

- GIVEN `components/pr-test/e2e-openshell.sh` runs OpenShift e2e tests today
- WHEN the consolidation is complete
- THEN the OpenShift-specific logic lives in `tests/e2e/drivers/openshift.sh`
- AND the test areas run through `tests/e2e/e2e-openshell.sh`
- AND `components/pr-test/` is removed

#### Scenario: No dangling references remain

- GIVEN `components/pr-test/` is removed
- WHEN a maintainer inspects CI configuration and component registration
- THEN no workflow references the removed path
- AND `.github/component-paths.json` no longer contains a `pr_test` entry that
  points to the removed path

### Requirement: Ephemeral CI Environment Provisioning

The CI workflow SHALL deploy HyperShell to an ephemeral environment on a shared
target OpenShift cluster for each pull request that runs the e2e suite. An
ephemeral environment is the namespace group that the Keycloak Namespace
requirement defines. The workflow SHALL NOT provision a full cluster.
The target cluster SHALL have capacity for several simultaneous pull-request
environments. The workflow MAY provision the environment through the
ephemeral-namespace-operator or an equivalent mechanism.

The workflow SHALL key the ephemeral environment to the pull request, so that every
run for one pull request uses the same environment. On the first run for a pull
request, the workflow SHALL create the environment and deploy HyperShell. On a
later run for the same pull request, the workflow SHALL reuse the existing
environment and redeploy only the changed components on top of the running
environment, so that the environment serves as a live development and debug
environment across the life of the pull request.

The workflow SHALL keep the ephemeral environment alive after the e2e suite
completes, whether the suite passes or fails, so that the developer can inspect the
live environment. The workflow SHALL release the environment when the pull request
merges or closes. The workflow SHALL NOT release the environment at the end of a
single CI run.

The environment SHALL be cost-bounded even though it lives across the pull request.
When the workflow provisions the environment through a reservation mechanism with a
duration, the workflow SHALL renew or set the duration to cover the pull request,
and SHALL rely on that duration as a backstop, so that an abandoned pull request
does not hold the environment forever. The workflow SHALL release the environment
on pull-request close as the primary path, and SHALL let the duration reclaim the
environment when the close event does not fire.

The workflow SHALL NOT leave an orphaned environment. If the release step cannot
confirm the release, the workflow SHALL report the failure so that an operator can
free the environment.

#### Scenario: First run creates the pull-request environment

- GIVEN a pull request runs the e2e suite for the first time
- WHEN the CI workflow runs
- THEN the workflow creates an ephemeral environment keyed to the pull request
- AND the workflow deploys HyperShell into that environment

#### Scenario: Later run reuses the environment

- GIVEN an ephemeral environment already exists for a pull request
- WHEN a later push triggers the CI workflow for the same pull request
- THEN the workflow reuses the existing environment
- AND the workflow redeploys the changed components on top of the running
  environment

#### Scenario: Environment survives after tests complete

- GIVEN the e2e suite completes for a pull request, whether it passes or fails
- WHEN the CI run finishes
- THEN the ephemeral environment stays alive
- AND the developer can access the live environment

#### Scenario: Environment releases on pull-request merge

- GIVEN an ephemeral environment exists for a pull request
- WHEN the pull request merges or closes
- THEN the workflow releases the environment
- AND a reservation duration reclaims the environment when the close event does not
  fire

### Requirement: Environment Access Handoff

The CI workflow SHALL give the developer the access details for the ephemeral
environment, which lives for the life of the pull request, so that the developer
can inspect a failing test and do live work on the environment. The workflow SHALL
surface the environment namespaces, the OpenShift console URL for the
platform namespace, the API Route URL, the web-console Route URL, and the
`oc login` command or kubeconfig that grants namespace-scoped access.

The workflow SHALL deliver the access details through a pull request comment. The
workflow SHALL keep the comment current across runs, so that the comment reflects
the live environment for the pull request. The workflow SHALL handle credentials
securely. The workflow SHALL NOT print a kubeconfig, a token, or a password into
the job logs or into a public artifact. The workflow SHALL deliver credentials
through a channel that only an authorized developer can read, such as a masked
secret or a restricted artifact.

#### Scenario: Developer receives environment links in a pull request comment

- GIVEN the CI workflow deploys the ephemeral environment
- WHEN the deployment is ready
- THEN the workflow posts a pull request comment with the environment namespaces,
  the console URL, the API Route URL, and the web-console Route URL
- AND the comment provides an `oc login` command or a kubeconfig for
  namespace-scoped access

#### Scenario: Credentials do not leak

- GIVEN the workflow delivers environment access details
- WHEN a reader inspects the job logs and the public artifacts
- THEN no kubeconfig, token, or password appears in the logs or the public
  artifacts
- AND the credentials are available only through a secure channel

### Requirement: Blessed OpenShift Overlay

The `deploy/openshift/` overlay SHALL derive from `deploy/base/` and SHALL NOT
duplicate the base resources. The overlay SHALL be validated against a production
reference, so that the overlay a developer deploys is the same shape as the
production deployment.

The overlay SHALL replace its placeholder values with values that a real
environment supplies through configuration, not through code. The known
placeholders to reconcile are the gateway base domain
`GATEWAY_API_BASE_DOMAIN=openshell.stage.example.com` and the unpinned database
image `registry.redhat.io/rhel9/postgresql-16`. The base domain SHALL come from
configuration, so that a deployment to a different cluster does not require an
overlay edit. The database image SHALL be pinned to a digest (`@sha256:...`), so
that a deployment is reproducible.

A drift check SHALL compare the overlay against the production reference and SHALL
fail when the overlay drifts from the reference in a way that the reference does
not allow. The drift check SHALL run in CI, so that a pull request that changes the
overlay cannot merge an unintended drift.

#### Scenario: Base domain comes from configuration

- GIVEN the overlay needs a gateway base domain
- WHEN a developer deploys to a cluster with a different base domain
- THEN the deployment reads the base domain from configuration
- AND the developer does not edit the overlay to change the base domain

#### Scenario: Database image is pinned

- GIVEN the overlay references the PostgreSQL image
- WHEN a maintainer inspects the overlay
- THEN the image reference includes a digest
- AND a deployment pulls the same image bytes every time

#### Scenario: Drift check fails on unintended drift

- GIVEN a production reference for the overlay exists
- WHEN a pull request changes the overlay in a way the reference does not allow
- THEN the drift check fails in CI
- AND the pull request cannot merge until the drift is resolved

### Requirement: OpenShift CI Workflow Shape

The OpenShift e2e CI job SHALL extend the existing e2e workflow structure that
`.github/workflows/e2e.yml` defines, so that both drivers share the same gating and
the same summary pattern. The job SHALL gate on the Konflux image builds the same
way the Kind job does, so that the images tested on OpenShift are the images that
ship. The OpenShift e2e job SHALL run whenever the Kind e2e job runs, so that a
change that triggers the Kind e2e suite also runs the OpenShift e2e suite. A change
under `deploy/openshift/` or the OpenShift lifecycle scripts SHALL also trigger the
job.

The job SHALL run these steps in order: gate on Konflux images; reuse or create the
ephemeral environment for the pull request; deploy with
`kustomize build deploy/openshift/`, redeploying only the changed components when
the environment already exists; run `E2E_INFRA_DRIVER=openshift bash
tests/e2e/e2e-openshell.sh`; collect diagnostics on failure; and post or update the
pull request comment with the environment access details. The job SHALL keep the
environment alive after the run. A separate step, triggered on pull-request merge
or close, SHALL release the environment. The CI summary gate SHALL include the
OpenShift job result, so that the gate reflects both drivers.

The `.github/component-paths.json` registration SHALL include the OpenShift paths in
the e2e component so that the OpenShift e2e job triggers on the same changes as the
Kind e2e job. The registration SHALL add `deploy/openshift/**` and the OpenShift
lifecycle script paths to the e2e component paths.

#### Scenario: OpenShift job gates on Konflux images

- GIVEN a pull request runs the e2e suite
- WHEN the OpenShift CI job runs
- THEN the job waits for the Konflux image builds to complete
- AND the job deploys the images that Konflux built

#### Scenario: OpenShift e2e runs whenever Kind e2e runs

- GIVEN a change triggers the Kind e2e job
- WHEN CI evaluates which e2e jobs to run
- THEN the OpenShift e2e job runs as well
- AND a change under `deploy/openshift/` also triggers the OpenShift e2e job

#### Scenario: Summary gate includes the OpenShift result

- GIVEN the CI workflow runs both the Kind job and the OpenShift job
- WHEN the summary gate evaluates the results
- THEN the gate includes the OpenShift job result
- AND the gate fails when the OpenShift job fails

### Requirement: Cluster Infrastructure Prerequisites

The ephemeral-namespace workflow SHALL treat the cluster infrastructure as a
precondition that the target cluster provides. This infrastructure is the shared
Gateway `openshell-grpc-gateway` in `openshift-ingress`, the `openshift-default`
GatewayClass, the certificate issuer, and the wildcard certificate for the gateway
base domain. An administrator provisions this infrastructure once per cluster, as
`infrastructure/GATEWAY-SETUP.md` describes. Both local development and pull-request
CI depend on this precondition, because an ephemeral namespace grants
namespace-scoped access and does not grant permission to create cluster
infrastructure.

The `make openshift-up` command SHALL check for the required infrastructure, and
SHALL report a clear error when the infrastructure is missing, rather than deploy a
broken environment. The CI workflow SHALL run against a target cluster that already
provides this infrastructure.

#### Scenario: Missing infrastructure fails fast

- GIVEN an OpenShift cluster without the shared Gateway
- WHEN a developer runs `make openshift-up`
- THEN the command reports that the required infrastructure is missing
- AND the command does not deploy a broken environment

#### Scenario: CI target provides the infrastructure

- GIVEN the CI workflow deploys into an ephemeral environment
- WHEN the workflow prepares the environment for deployment
- THEN the workflow relies on the target cluster for the shared Gateway, the
  GatewayClass, and the certificate issuer
- AND the workflow does not attempt to create that cluster infrastructure from the
  ephemeral namespaces

### Requirement: Cluster-Scoped Resource Permissions

The OpenShift overlay creates cluster-scoped resources. These are the SCC
ClusterRoleBindings for the controller and for the sandbox, and the ClusterRole
that lets the controller bind the privileged SCC per namespace. An ephemeral
namespace grants namespace-scoped access, so a deployment into an ephemeral
namespace may not be able to create these cluster-scoped resources.

The deployment SHALL have the permission to create these resources, or the
deployment SHALL get the same end state through resources that the target cluster
already provides. When the deployment cannot create the cluster-scoped resources,
the target cluster SHALL pre-create them, or the overlay SHALL be adapted so that
the controller and the sandbox get their SCC through a namespace-scoped binding that
the ephemeral namespace permits. This spec does not choose one resolution; it
requires that a deployment into an ephemeral namespace does not fail on a missing
permission.

#### Scenario: Ephemeral namespace has the permissions the overlay needs

- GIVEN a deployment into an ephemeral namespace
- WHEN the workflow deploys the OpenShift overlay
- THEN the SCC bindings that the controller and the sandbox need are in place
- AND the deployment does not fail on a missing cluster-scoped permission

### Requirement: OpenShift Security Context and RBAC Parity

The OpenShift deployment SHALL keep the security posture that the OpenShift overlay
defines: a restricted SecurityContextConstraints for the controller, a privileged
SecurityContextConstraints only for the sandbox pods, and a per-namespace
privileged SCC binding that the controller creates at runtime. A component swap or
an ephemeral-namespace deployment SHALL NOT relax this posture.

The OpenShift deployment SHALL enforce RBAC the same way the production overlay
does, with `RBAC_ENFORCE=true` and the control-plane service account in the RBAC
bypass list. The e2e suite on OpenShift SHALL validate the same RBAC scenarios that
the Kind suite validates, including the developer role enforcement and the
platform admin role.

#### Scenario: Sandbox privilege is scoped per namespace

- GIVEN a HyperShell deployment on OpenShift provisions a gateway sandbox
- WHEN the controller reconciles the sandbox
- THEN the sandbox pod runs under the privileged SCC through a per-namespace
  binding
- AND no other workload in the namespace gains privileged access

#### Scenario: RBAC is enforced on OpenShift

- GIVEN the OpenShift deployment runs with `RBAC_ENFORCE=true`
- WHEN a developer without the required role calls a protected API
- THEN the API denies the request
- AND the e2e suite validates the denial the same way it does on Kind
