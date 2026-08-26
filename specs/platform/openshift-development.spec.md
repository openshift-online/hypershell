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

This spec also defines automated end-to-end testing on OpenShift. This spec owns the
OpenShift lifecycle, the `deploy/openshift/` overlay, the cluster bootstrap, and the
OpenShift CI workflow. `e2e-testing.spec.md` owns the e2e driver interface contract
and the `tests/e2e/drivers/openshift.sh` driver file that implements the OpenShift
side of that contract. It defines a CI workflow that deploys HyperShell to an ephemeral
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

Both contexts use the same lifecycle driver -- `make openshift-up` and the same
driver functions -- and the same e2e driver, so that a local deployment and a CI
deployment cannot drift.

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

```text
Makefile (single entry point)
    │
    ├── cluster lifecycle (infra-agnostic)
    │       │
    │       ├── sources scripts/cluster/lib.sh (shared seams)
    │       │
    │       └── the up-target picks the driver (kind-up / openshift-up)
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
infrastructure-specific logic through a driver model. The make target name SHALL
select the driver: `make kind-up` selects the Kind driver and `make openshift-up`
selects the OpenShift driver. Existing Kind commands keep their current behavior.
Each driver SHALL implement a fixed set of lifecycle operations: `cluster_up`,
`cluster_down`, `cluster_teardown`, `cluster_status`, `component_swap`, and
`component_revert`.

Because OpenShift does not create the cluster, the OpenShift driver's
`cluster_teardown` SHALL behave the same as `cluster_down` -- it removes the
environment namespace group -- rather than destroy a cluster. The Kind driver's
`cluster_teardown` SHALL keep its current behavior of destroying the Kind cluster.

The shared lifecycle library SHALL centralize the seams that differ per
infrastructure, the same way `scripts/kind/lib.sh` centralizes the Kubernetes
context and the swap tracking today. The Kind driver SHALL reuse the existing
`scripts/kind/` logic without behavior change.

Each infrastructure target SHALL use its own driver for both lifecycle operations
and e2e operations. The Kind target SHALL use the Kind lifecycle driver and the
Kind e2e driver (`tests/e2e/drivers/kind.sh`). The OpenShift target SHALL use the
OpenShift lifecycle driver and the OpenShift e2e driver
(`tests/e2e/drivers/openshift.sh`).

The lifecycle driver and the e2e driver for a target SHALL select the same
infrastructure. The lifecycle driver comes from the make target name. The e2e
driver comes from `E2E_INFRA_DRIVER`, which the test entry points require (see
`e2e-testing.spec.md`). The test entry points (`make e2e`, `make e2e-performance`)
are each one infrastructure-agnostic target, so they need an explicit selector. The
lifecycle targets do not need a selector, because the target name already fixes the
infrastructure.

#### Scenario: Default driver preserves Kind behavior

- GIVEN a developer runs `make kind-up`
- WHEN the lifecycle scripts run
- THEN the scripts use the Kind driver
- AND the deployment result is identical to the current Kind behavior

#### Scenario: A new infrastructure target adds only a driver

- GIVEN the lifecycle driver contract is defined
- WHEN a developer adds support for a new infrastructure target
- THEN the developer adds one lifecycle driver file that implements the fixed
  operations
- AND the developer does not change the infrastructure-agnostic lifecycle logic

#### Scenario: OpenShift teardown removes the namespace group

- GIVEN an OpenShift environment exists
- WHEN the framework calls `cluster_teardown` for the OpenShift target
- THEN the driver removes the environment namespace group
- AND the driver does not attempt to destroy the OpenShift cluster

### Requirement: OpenShift Lifecycle Up and Down

The `make openshift-up` command SHALL deploy the full HyperShell stack to an
ephemeral namespace on a target OpenShift cluster with a single command, the same
way `make kind-up` deploys to Kind. The command SHALL connect to the cluster that
the developer's current kubeconfig context selects. The command SHALL NOT create
the OpenShift cluster; the cluster is a precondition for the ephemeral-namespace
workflow. When no target OpenShift cluster is available, the command SHALL stop
with a clear error that tells the developer to provide an OpenShift cluster target,
rather than deploy nothing or fail without guidance.

The `make openshift-up` command SHALL render the overlay with `kustomize build
deploy/openshift/` and SHALL apply the rendered manifests to the cluster (for
example by piping them to `oc apply`), rather than only render them, so that the
command cannot report success after it produced YAML alone. The overlay's platform
namespace (`hypershell-system` in the base) SHALL map to `OPENSHIFT_NAMESPACE`, and
the overlay's Keycloak namespace (`keycloak` in the base) SHALL map to
`${OPENSHIFT_NAMESPACE}-keycloak`, through the namespace parameterization that the
Blessed OpenShift Overlay requirement defines. The command SHALL be idempotent: a
second run SHALL reconcile the environment to the current overlay and SHALL prune
resources the overlay no longer defines, with pruning scoped to the environment,
rather than leave stale resources behind. The reconcile SHALL preserve an active
per-namespace component swap: it SHALL keep the working-tree image on a swapped
Deployment rather than reset it to the overlay's baseline image. When a reconcile
intentionally resets a swapped Deployment to the baseline image, it SHALL clear that
component's entry in the per-namespace swap state, so the state cannot keep claiming
the working-tree image is active.

Like `make kind-up`, `make openshift-up` SHALL seed the domain resources a
developer needs for a working gateway -- a Fleet, a ManagedCluster, a
GatewayRelease, a ManagedDatabase, and a Gateway -- with the OpenShift Route and
OIDC values for the environment, so that one command produces a working gateway and
the OpenShift workflow matches the Kind workflow.

The `make openshift-down` command SHALL delete the applied manifests and SHALL
remove every namespace in the environment namespace group, subject to the ownership
check that the Ephemeral Namespace Isolation requirement defines. The
`make openshift-status` command SHALL report the cluster, the environment
namespaces, the pods, the services, the Routes, the Gateway status, and the
component swap state, the same categories that `make kind-status` reports.

The command names SHALL mirror the Kind command names by replacing the `kind`
prefix with `openshift`.

#### Scenario: Deploy the full stack to OpenShift

- GIVEN a developer has a kubeconfig context for an OpenShift cluster
- AND the developer has permission to create a namespace
- WHEN the developer runs `make openshift-up`
- THEN the scripts render and apply the API server, the control plane, the web
  console, PostgreSQL, and Keycloak from `kustomize build deploy/openshift/`
- AND the scripts seed a Fleet, a ManagedCluster, a GatewayRelease, a
  ManagedDatabase, and a Gateway
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
same way `KIND_NAMESPACE` names the Kind target namespace. Unlike Kind, which
targets a single-tenant local cluster and defaults `KIND_NAMESPACE` to
`hypershell-system`, an OpenShift target is shared, so there SHALL be no shared
default namespace. When `OPENSHIFT_NAMESPACE` is unset, the command SHALL derive a
unique per-developer default or SHALL stop with a clear error, rather than fall back
to a shared name that collides. A derived default SHALL be a valid RFC 1123 DNS label
-- for example, an `oc whoami` value lowercased, with every character outside
`[a-z0-9-]` replaced by `-`, and truncated to the length limit -- because raw
identity values such as `kube:admin` or `user@redhat.com` are not valid DNS labels.

`OPENSHIFT_NAMESPACE` SHALL be a valid RFC 1123 DNS label of at most 54 characters,
so that the derived `${OPENSHIFT_NAMESPACE}-keycloak` namespace stays within the
63-character DNS-label limit for Kubernetes namespaces. The command SHALL validate
the name and the derived name before it creates any resource, and SHALL stop with a
clear error when either name is invalid. The scripts SHALL derive the `-keycloak`
namespace from that name (see the Keycloak Namespace requirement). The scripts SHALL
create the namespaces if they do not exist.

The scripts SHALL stamp every namespace in the group, at creation, with an
ownership label that marks the namespace as HyperShell-owned and with an immutable
environment identifier that ties the namespace to one deployment, so that
`make openshift-status` and cleanup tooling can find every HyperShell namespace and
can tell which namespaces belong to the same environment. Before it deploys, the
command SHALL refuse to adopt an existing namespace whose ownership label or
environment identifier does not match the current environment, so that a deployment
cannot take over a namespace that another environment or another team owns. The
scripts SHALL derive per-tenant gateway hostnames from the gateway base domain (the
configured `GATEWAY_API_BASE_DOMAIN`) and the platform namespace, so that two
deployments on one cluster do not share a hostname.

Before it deletes, `make openshift-down` SHALL verify the ownership label and the
environment identifier on each namespace and SHALL delete only namespaces that match
the current environment, so that it cannot delete unrelated workloads. The command
SHALL refuse a mismatch and report it.

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
- THEN the scripts verify the ownership label and the environment identifier
- AND the scripts remove only that environment's namespaces
- AND the other environment stays intact

#### Scenario: Deployment refuses a foreign namespace

- GIVEN a namespace with the name `OPENSHIFT_NAMESPACE` already exists and carries a
  different environment identifier
- WHEN a developer runs `make openshift-up`
- THEN the command refuses to adopt the namespace
- AND the command deploys nothing into, and deletes nothing in, that namespace

### Requirement: Keycloak Namespace

The deployment SHALL place Keycloak in its own namespace, separate from the other
HyperShell components, the same way the Kind environment runs Keycloak in a
dedicated `keycloak` namespace. The OpenShift deployment SHALL name this namespace
`${OPENSHIFT_NAMESPACE}-keycloak`, so that each ephemeral environment has its own
Keycloak, and two environments on one cluster do not share a Keycloak or collide on
a fixed namespace name. Every other HyperShell component SHALL deploy into the
platform namespace that `OPENSHIFT_NAMESPACE` names.

Folding Keycloak into the platform namespace would remove one namespace per
environment, but `deploy/base/keycloak/` already assigns Keycloak its own
`keycloak` namespace, and both `deploy/kind/` and `deploy/hub/` build on that
placement. A per-environment `-keycloak` namespace reuses that base structure with
only a namespace parameterization, so the overlay stays derived from the base
rather than patching Keycloak into a shared namespace. This is why the spec keeps
Keycloak in its own namespace.

Together, the platform namespace and its `-keycloak` namespace form the
deployment's namespace group. The two namespaces SHALL share one lifecycle: the
scripts create them together, and `make openshift-down` (for local development) or
the release step (for CI) removes them together.

The OpenShift OIDC configuration SHALL derive from the Keycloak Route in the
`-keycloak` namespace through one hostname formula: the driver reads the Keycloak
Route host from the `-keycloak` namespace. The Keycloak `KC_HOSTNAME` SHALL be the
server base URL `https://<route-host>` with no realm path, because Keycloak treats
`KC_HOSTNAME` as the base URL and appends the realm path itself when it builds the
OIDC discovery and issuer URLs. The e2e `E2E_OIDC_ISSUER` and the gateway OIDC issuer
the control plane provisions SHALL be `https://<route-host>/realms/hypershell` -- the
base URL plus the realm path -- so that they match the issuer that Keycloak
advertises in its discovery document, and so that token acquisition and token
validation target the same endpoint. Embedding the realm path in `KC_HOSTNAME` would
produce incorrect discovery and issuer URLs and break token validation. This mirrors
the Kind configuration, where `KC_HOSTNAME` is host-only and only the issuer values
carry `/realms/hypershell`.

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

Because more than one developer can share one cluster, each working-tree image
SHALL have an immutable identity scoped to the source commit and to
`OPENSHIFT_NAMESPACE` -- a digest, or a tag that is unique per commit and per
environment -- rather than a shared mutable tag that another environment could
overwrite. The driver SHALL update the component deployment to that exact identity
and SHALL trigger a rollout when the identity changes, so that the running pods use
the working-tree build.

The scripts SHALL track the swap state per namespace, the same way `.kind-swaps`
tracks the Kind swap state, and SHALL record the exact image identity that is
deployed, so that `make openshift-status` reports which components run a working-tree
build, which run the baseline image, and the exact image each one runs.

#### Scenario: Swap the API server from the working tree

- GIVEN a HyperShell deployment exists on OpenShift with baseline images
- WHEN the developer runs `make openshift-api-server-up`
- THEN the scripts build the API server image from the working tree
- AND the pushed image has an immutable identity scoped to the commit and
  `OPENSHIFT_NAMESPACE`
- AND the scripts push the image to a registry that the cluster can pull
- AND the scripts record that image identity in the per-namespace swap state
- AND the scripts roll out the API server deployment to the pushed image
- AND `make openshift-status` reports the API server as a working-tree build with
  its exact image identity

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

The driver SHALL implement each function with OpenShift constructs, and SHALL scope
every resource lookup to the target namespace, so that concurrent environments on
one cluster do not read each other's resources:

- `discover_api_host` SHALL read the `hypershell-api` Route host in the platform
  namespace with `oc get route hypershell-api -n "${OPENSHIFT_NAMESPACE}"
  -o jsonpath='{.spec.host}'`, and SHALL set `_DISCOVER_API_HOST` to the HTTPS URL.
- `discover_gateway_endpoint` SHALL take the gateway name and the gateway namespace
  as arguments, the same as the Kind driver, and SHALL discover the endpoint the
  same way: read the tenant GRPCRoute hostname and confirm the parent Gateway
  reports `Programmed=True`, then set `_DISCOVER_GW_ENDPOINT` to
  `https://<grpc-host>:443`. The driver SHALL NOT expect a per-gateway OpenShift
  Route; per-tenant gateway traffic uses Gateway API, as
  `openshell-gateway-routing.spec.md` defines.
- `get_cluster_domain` SHALL return the configured gateway base domain -- the same
  `GATEWAY_API_BASE_DOMAIN` value the control plane uses -- so that the driver
  builds gateway hostnames that match the tenant GRPCRoute hostname and the wildcard
  certificate on the shared Gateway. The driver SHALL NOT read the cluster apps
  domain from `ingresses.config.openshift.io`: that value is not the gateway base
  domain, and a namespace-scoped ephemeral environment does not have permission to
  read that cluster-scoped resource.
- `get_cli_binary` SHALL return `oc`.
- `wait_for_gateway_route` SHALL take the gateway name and the gateway namespace as
  arguments and SHALL wait, up to `E2E_PROVISION_TIMEOUT`, until the parent Gateway
  reports `Programmed=True` and the tenant GRPCRoute parent reports `Accepted=True`,
  the same two conditions the Kind driver waits for. The driver SHALL NOT wait for an
  OpenShift Route `Admitted` condition, because no per-gateway Route exists.

The driver SHALL set the OIDC issuer from the Keycloak Route in the `-keycloak`
namespace, as the Keycloak Namespace requirement defines, rather than the Kind
default `keycloak.hypershell.localhost`.

The OpenShift suite SHALL use the same per-gateway Keycloak client and role wait
that the Kind suite uses, because per-gateway audience is production behavior, not a
Kind-only path: a token minted for the shared frontend client carries the wrong
`aud` claim and the control plane rejects it. The driver SHALL provide the same
Keycloak admin and role helpers the Kind driver provides -- `assign_realm_role`,
`assign_gateway_client_role`, and `acquire_gateway_token_with_role` -- so that the
OpenShift suite acquires a per-gateway-client token with the required role the same
way the Kind suite does, and so that no weaker OpenShift OIDC path remains.

The driver SHALL establish trusted TLS without an insecure bypass, the same rule
`e2e-testing.spec.md` sets. When the shared Gateway serves a publicly trusted or
cluster-wildcard certificate, the suite SHALL rely on the system trust store. When
the shared Gateway serves a private CA, the driver SHALL extract that CA and point
`SSL_CERT_FILE` at it. The suite SHALL NOT set `OPENSHELL_GATEWAY_INSECURE`.

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

#### Scenario: Gateway readiness uses Gateway API status

- GIVEN the control plane provisions a Gateway and a tenant GRPCRoute
- WHEN the e2e suite calls `wait_for_gateway_route` with the gateway name and
  namespace
- THEN the driver waits until the parent Gateway reports `Programmed=True`
- AND the driver waits until the tenant GRPCRoute parent reports `Accepted=True`
- AND the driver does not wait for an OpenShift Route `Admitted` condition
- AND the driver returns success only when both conditions are true, or fails after
  `E2E_PROVISION_TIMEOUT`

#### Scenario: OpenShift uses the per-gateway client token

- GIVEN a gateway has its own Keycloak client with a scoped audience
- WHEN the e2e suite acquires a token on OpenShift
- THEN the suite uses `acquire_gateway_token_with_role` with the per-gateway client
- AND the token carries the gateway's audience
- AND the control plane accepts the token

#### Scenario: OpenShift trusts TLS without an insecure bypass

- GIVEN the shared Gateway serves the gateway TLS certificate
- WHEN the e2e suite connects to a gateway on OpenShift
- THEN the suite verifies the certificate through the system trust store or an
  extracted CA
- AND the suite does not set `OPENSHELL_GATEWAY_INSECURE`

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
environment and redeploy with the same `make openshift-up` reconcile that the first
run uses. The redeploy SHALL deploy the images that Konflux built for the pull
request, injecting those image references by digest into the deployment the same way
the Kind job does through `scripts/kind/set-component-images.sh`, rather than rebuild
images from the working tree, so that the PR e2e job tests the images that ship. The
redeploy SHALL let the overlay reconcile bring the running environment to the new
desired state, including pruning resources that the overlay no longer declares, so
that the environment does not drift from the overlay and so that the environment
serves as a live development and debug environment across the life of the pull
request.

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
- AND the workflow reruns `make openshift-up` to reconcile the full overlay
- AND the workflow deploys the Konflux-built images by digest, without rebuilding from the working tree
- AND the reconcile prunes resources that the overlay no longer declares

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
can inspect a failing test and do live work on the environment. The access details
have two parts: non-secret facts and a credential. The non-secret facts are the
environment namespaces, the OpenShift console URL for the platform namespace, the
API Route URL, and the web-console Route URL. The credential is a short-lived,
namespace-scoped token or kubeconfig that grants access to the environment.

The workflow SHALL deliver the non-secret facts through a pull request comment, and
SHALL keep the comment current across runs, so that the comment reflects the live
environment for the pull request. The comment MAY include an `oc login` command
template, but the template SHALL show the credential as redacted, for example
`oc login --server=<api-url> --token=<redacted>`. The comment SHALL NOT contain the
credential itself.

The workflow SHALL deliver the credential only through a channel that only an
authorized developer can read, such as a masked secret or a restricted artifact.
The workflow SHALL NOT print a kubeconfig, a token, or a password into a pull
request comment, into the job logs, or into a public artifact. The credential SHALL
be short-lived and namespace-scoped, so that a leak has a bounded blast radius.

#### Scenario: Developer receives environment links in a pull request comment

- GIVEN the CI workflow deploys the ephemeral environment
- WHEN the deployment is ready
- THEN the workflow posts a pull request comment with the environment namespaces,
  the console URL, the API Route URL, and the web-console Route URL
- AND the comment shows any `oc login` template with the credential redacted
- AND the workflow delivers the credential through a secure channel, not the comment

#### Scenario: Credentials do not leak

- GIVEN the workflow delivers environment access details
- WHEN a reader inspects the pull request comment, the job logs, and the public
  artifacts
- THEN no kubeconfig, token, or password appears in the comment, the logs, or the
  public artifacts
- AND the credential is available only through a secure channel

### Requirement: Blessed OpenShift Overlay

The `deploy/openshift/` overlay SHALL derive from `deploy/base/` and SHALL NOT
duplicate the base resources. The `deploy/base/` overlay is the independent baseline
for drift validation: both `deploy/openshift/` and the production `deploy/hub/`
overlay derive from it, and it is not derived from either, so a change in one overlay
cannot flow into the reference before the check runs. The drift check SHALL NOT use
`deploy/hub/` as the reference for `deploy/openshift/`, because `deploy/hub/` derives
from `deploy/openshift/` and a change would reach the reference before comparison.

The overlay SHALL parameterize the namespace, so that a deployment into an ephemeral
namespace does not require an overlay edit. The overlay SHALL NOT hardcode
`hypershell-system`; the deployment SHALL set the namespace from configuration, the
same way `make openshift-up` maps the platform namespace to `OPENSHIFT_NAMESPACE`.

Each overlay SHALL differ from `deploy/base/` only within a declared allowlist. For
`deploy/openshift/`, the allowed differences are the OpenShift-specific additions the
overlay layers on the base (the Route, the SecurityContextConstraints binding, the
certificates, and the network policies) together with the namespace, the name prefix,
the image references, the gateway base domain `GATEWAY_API_BASE_DOMAIN`, and the SSO
configuration. Ephemeral OpenShift and hub intentionally differ: ephemeral OpenShift
bundles a per-environment Keycloak and the bundled CNPG `Cluster` that the base
provides, while `deploy/hub/` (production) shares one Keycloak per cluster and uses a
managed database, so `deploy/hub/` deletes the bundled Keycloak unit and the bundled
CNPG `Cluster`. The `deploy/hub/` allowlist SHALL therefore additionally permit those
deletions. These declared differences are intentional, not drift.

The overlay SHALL replace its placeholder values with values that a real
environment supplies through configuration, not through code. The known placeholder
to reconcile is the gateway base domain
`GATEWAY_API_BASE_DOMAIN=openshell.stage.example.com`, which SHALL come from
configuration, so that a deployment to a different cluster does not require an
overlay edit. The bundled CNPG `Cluster` in `deploy/base/hypershell-db-cluster.yaml`
sets no explicit `imageName`, so it runs the CloudNativePG operator's default
PostgreSQL image, which is not pinned for this deployment. For a reproducible
deployment, the overlay SHALL set an explicit `spec.imageName` on the CNPG `Cluster`
pinned to a digest (`@sha256:...`), rather than rely on the operator default.

A drift check SHALL compare each overlay against `deploy/base/` after a defined
normalization step, and SHALL fail when an overlay differs from the base outside its
declared allowlist. The drift check SHALL run in CI, so that a pull request that
changes an overlay cannot merge an unintended drift. Because both overlays are
checked against the same independent base, `deploy/openshift/` and `deploy/hub/`
cannot silently diverge on any resource that neither allowlist covers.

#### Scenario: Base domain comes from configuration

- GIVEN the overlay needs a gateway base domain
- WHEN a developer deploys to a cluster with a different base domain
- THEN the deployment reads the base domain from configuration
- AND the developer does not edit the overlay to change the base domain

#### Scenario: Database image is pinned

- GIVEN the CNPG `Cluster` sets an explicit `spec.imageName`
- WHEN a maintainer inspects the overlay
- THEN the image reference includes a digest
- AND a deployment pulls the same image bytes every time

#### Scenario: Drift check fails on unintended drift

- GIVEN `deploy/base/` is the independent baseline for the overlays
- WHEN a pull request changes `deploy/openshift/` outside its declared allowlist
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
ephemeral environment for the pull request; deploy with `make openshift-up`, which
reconciles the full overlay, then inject the Konflux-built image references by digest
(the same digest-injection pattern the Kind job uses through
`scripts/kind/set-component-images.sh`) rather than rebuild from the working tree; run
`E2E_INFRA_DRIVER=openshift bash tests/e2e/e2e-openshell.sh`; collect diagnostics on
failure; and post or update the pull request comment with the environment access
details. The job SHALL keep the
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
Gateway, the GatewayClass, the certificate issuer, and the wildcard certificate for
the gateway base domain. The shared Gateway name and namespace SHALL come from
configuration -- `GATEWAY_API_GATEWAY_NAME` (default `openshell-grpc-gateway`) and
`GATEWAY_API_GATEWAY_NAMESPACE` (default `openshift-ingress`) -- so that a deployment
to a cluster with a different shared Gateway does not require a code change. An
administrator provisions this infrastructure once per cluster, as
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

The OpenShift overlay grants SecurityContextConstraints through the built-in SCC
ClusterRoles, not through custom SCC objects. Two different grants are involved, and
they have different scopes.

The first grant is SCC *use*: a workload runs under an SCC. A namespace-scoped
RoleBinding to a built-in SCC ClusterRole grants use within one namespace. Each
ephemeral namespace SHALL attach its controller service account to the built-in
`system:openshift:scc:restricted-v2` ClusterRole and its sandbox service account to
the built-in `system:openshift:scc:privileged` ClusterRole through namespace-scoped
RoleBindings, which namespace-scoped access permits.

The second grant is the `bind` verb on the privileged SCC ClusterRole, which the
controller needs so that it can itself create the per-namespace privileged RoleBinding
for a sandbox at runtime. `bind` on `clusterroles` is a cluster-scoped permission. A
namespace-scoped RoleBinding CANNOT grant it, and a single pre-created
ClusterRoleBinding with a fixed service-account subject cannot cover the controller
service account of an unknown future ephemeral namespace. Therefore a privileged
actor SHALL grant each ephemeral controller service account the cluster-scoped `bind`
on the privileged SCC. For example, the namespace-provisioning step (the
ephemeral-namespace operator, or an equivalent cluster-privileged step that creates
the namespace and its service accounts) SHALL, at namespace-create time, add that
controller service account as a subject of a `bind` ClusterRoleBinding or create a
per-namespace ClusterRoleBinding for it. Alternatively, a privileged CI deployer that
is not the in-namespace controller SHALL create the per-namespace privileged
RoleBindings for the sandbox, so that the in-namespace controller never needs `bind`.

The deployment into the ephemeral namespace SHALL NOT attempt to grant the
cluster-scoped `bind` through a namespace-scoped RoleBinding, and SHALL NOT assume the
namespace-scoped RoleBindings alone let the controller bind the privileged SCC per
namespace. The cluster-scoped `bind` ClusterRole and the grant of `bind` to each
ephemeral controller service account SHALL be pre-created or provisioned by the
privileged actor, as above, before the controller reconciles a sandbox.

#### Scenario: Ephemeral namespace has the permissions the overlay needs

- GIVEN the target cluster pre-creates the cluster-scoped `bind` ClusterRole and its binding
- AND a privileged actor grants the ephemeral controller service account the cluster-scoped `bind` on the privileged SCC at namespace-create time
- WHEN the workflow deploys the OpenShift overlay into an ephemeral namespace
- THEN the deployment creates only namespace-scoped RoleBindings that attach the
  controller and sandbox service accounts to the built-in SCC ClusterRoles for SCC use
- AND the controller can create the per-namespace privileged RoleBinding for a sandbox because the privileged actor granted it `bind`
- AND the deployment does not attempt to grant the cluster-scoped `bind` through a namespace-scoped RoleBinding

### Requirement: OpenShift Security Context and RBAC Parity

The OpenShift deployment SHALL keep the security posture that the OpenShift overlay
defines: the controller bound to the built-in `system:openshift:scc:restricted-v2`
ClusterRole, the built-in `system:openshift:scc:privileged` ClusterRole granted only
to the sandbox pods, and a per-namespace privileged SCC binding that the controller
creates at runtime once a privileged actor has granted the controller service account
the cluster-scoped `bind` on the privileged SCC (see Cluster-Scoped Resource
Permissions). The overlay SHALL NOT define custom SCC objects. A component swap or an
ephemeral-namespace deployment SHALL NOT relax this posture.

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
