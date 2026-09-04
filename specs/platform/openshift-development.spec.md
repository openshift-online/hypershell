# OpenShift Development and Testing Specification

**Date:** 2026-08-19
**Status:** Draft
**Jira:** HYPERSHELL-44
**Related:** `local-development.spec.md` -- Kind lifecycle and component swap;
             `e2e-testing.spec.md` -- driver interface contract and Kind CI;
             `ephemeral-pr-environments.spec.md` (HYPERSHELL-240) -- pull-request
             OpenShift CI, timebox, GitHub-brokered Keycloak, and `e2e-openshell.sh`
             deprecation;
             `control-plane.spec.md` -- reconciler behavior;
             `openshell-gateway-routing.spec.md` -- GRPCRoute provisioning

## Purpose

HyperShell developers must build, deploy, and test the full stack on OpenShift
with the same commands and the same workflow that they use for Kind. This spec
defines a driver model for the cluster lifecycle. The `make openshift-up` command
deploys a complete HyperShell environment to an ephemeral namespace on an
OpenShift cluster. The developer can swap one component at a time from the working
tree, exactly as `make kind-<component>-up` does today.

This spec owns the OpenShift lifecycle, the `deploy/openshift/` overlay, the
cluster bootstrap, and the OpenShift side of the e2e driver contract.
`e2e-testing.spec.md` owns the e2e driver interface and the
`tests/e2e/drivers/openshift.sh` file that implements the OpenShift side of that
contract. Automated pull-request CI on OpenShift -- namespace naming, continuous
deployment, timebox, access comment, GitHub-brokered Keycloak, and the
deprecation of `components/pr-test/e2e-openshell.sh` -- is owned by
`ephemeral-pr-environments.spec.md` (HYPERSHELL-240). This spec supplies the
lifecycle that workflow runs (`make openshift-up` / `make openshift-down`, the
overlay, and the OpenShift e2e driver) so a local deployment and a CI
deployment cannot drift.

HyperShell uses one ephemerality model -- an ephemeral namespace on an existing
OpenShift cluster -- in two contexts:

- **Local development** -- The developer supplies a target OpenShift cluster. The
  `make openshift-up` command deploys into an isolated namespace on that cluster.
- **Pull request CI** -- Specified in `ephemeral-pr-environments.spec.md`. The
  pipeline deploys into an ephemeral namespace on a shared target environment
  using the same `make openshift-up` command this spec defines.

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

This spec covers the OpenShift lifecycle driver, the OpenShift e2e driver, and
the reconciliation of the OpenShift deploy overlay against a production
reference. Pull-request CI, the access handoff comment, the timebox, and the
`e2e-openshell.sh` deprecation window are specified in
`ephemeral-pr-environments.spec.md`.
This spec does not change the Kind driver, the Kind lifecycle scripts, or the Kind
CI job. This spec does not redesign the e2e driver interface contract that
`e2e-testing.spec.md` defines; it implements the OpenShift side of that contract.

This spec is a behavior contract. It does not contain the driver code or the CI
YAML. Those are follow-up implementation work.

### Reserved Terms

This spec adds no new domain kinds. It refers to the existing kinds (Gateway,
GatewayNetwork, GatewayRelease, ManagedCluster, ManagedDatabase) only
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
            └── selects driver by auto-detecting the KUBECONFIG context
                    (E2E_INFRA_DRIVER overrides detection)
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
driver auto-detects from the current KUBECONFIG context, which the test entry
points require to already point at the target infrastructure (see
`e2e-testing.spec.md`); `E2E_INFRA_DRIVER` remains available to override
detection. The test entry points (`make e2e`, `make e2e-performance`) are each
one infrastructure-agnostic target, so a developer who has run `make
openshift-up` and left their `oc` context pointed at that cluster gets the
OpenShift e2e driver with no extra selector. The lifecycle targets do not need
a selector either, because the target name already fixes the infrastructure.

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

#### Scenario: OpenShift teardown is an alias of down

- GIVEN an OpenShift environment exists
- WHEN the developer runs `make openshift-teardown` or `make openshift-down`
- THEN both commands remove the environment namespace group (the platform
  project and the `${OPENSHIFT_NAMESPACE}-keycloak` project)
- AND neither command attempts to destroy the OpenShift cluster

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
developer needs for a working gateway -- a ManagedCluster, a
GatewayRelease, a ManagedDatabase, and a Gateway -- with the OpenShift Route and
OIDC values for the environment, so that one command produces a working gateway and
the OpenShift workflow matches the Kind workflow. When the CloudNativePG operator
is not on the cluster, `make openshift-up` already falls back to the bundled
PostgreSQL Deployment for the API server; seeding SHALL create the ManagedDatabase
with `provider=deployment` in that case, not `provider=cnpg`. When CNPG is
present, seeding MAY use `provider=cnpg`. The OpenShift overlay SHALL set
`GATEWAY_API_HTTP_LISTENER_NAME=grpc` so console HTTPRoutes attach to the shared
Gateway listener of that name. The default `https` sectionName SHALL NOT be used
on this overlay: the shared Gateway has no `https` listener, and that mismatch
reports `NoMatchingParent` and does not self-heal.

The `make openshift-down` command SHALL delete the applied manifests and SHALL
remove every project in the environment namespace group: the platform project
the developer selected with `OPENSHIFT_NAMESPACE` or the current `oc project`,
and the companion `${OPENSHIFT_NAMESPACE}-keycloak` project. `make
openshift-teardown` SHALL be the same command. OpenShift does not create the
cluster, so teardown cannot destroy it; the Kind-shaped target exists for
compatibility. Ownership labels are not a delete gate: typical developers
cannot patch Namespace objects, so down SHALL NOT require those labels. The
command SHALL still refuse reserved names and namespaces labeled as a different
HyperShell environment. The command SHALL wait until each project is gone before it reports success,
rather than return after it has only requested deletion. When `oc delete project`
is forbidden, the command SHALL delete HyperShell resources inside both projects
(including the bundled Keycloak workload, which is unlabeled), wait for those
deletes, and leave the projects. The
`make openshift-status` command SHALL report the cluster, the environment
namespaces, the pods, the services, the Routes, the Gateway status, and the
component swap state, the same categories that `make kind-status` reports.

The command names SHALL mirror the Kind command names by replacing the `kind`
prefix with `openshift`.

Like `make kind-up`, `make openshift-up` SHALL wait until the stack is ready
before it prints the running banner or seeds. The wait SHALL cover Keycloak,
PostgreSQL when it is deployed as a Deployment, the API server, the control
plane, and the web console, including after Route-derived environment updates
and including a swapped working-tree image. The wait SHALL use
`oc rollout status`, not `oc wait --for=condition=available`, so a Deployment
that stays Available during a rolling update cannot report ready while a new
ReplicaSet is still in flight. The control plane Deployment SHALL expose a TCP
readiness probe on the service-account provisioner port so rollout is not
complete until that port is listening. The wait SHALL NOT probe OpenShift
Routes for `/healthcheck`, `/openapi`, or an OIDC token; seeding already retries
those from the developer machine.

#### Scenario: Deploy the full stack to OpenShift

- GIVEN a developer has a kubeconfig context for an OpenShift cluster
- AND the developer has permission to create a namespace
- WHEN the developer runs `make openshift-up`
- THEN the scripts render and apply the API server, the control plane, the web
  console, PostgreSQL, and Keycloak from `kustomize build deploy/openshift/`
- AND the scripts seed a ManagedCluster, a GatewayRelease, a
  ManagedDatabase, and a Gateway
- AND the scripts report the API Route, the web-console Route, and the Keycloak
  Route when the deployment is ready

#### Scenario: Wait until the stack can serve

- GIVEN overlay apply, swap restore, and Route-derived environment updates have
  triggered rollouts
- WHEN the developer runs `make openshift-up`
- THEN the command does not print the running banner until `oc rollout status`
  succeeds for Keycloak, the API server, the control plane, and the web console
- AND the command does not skip that wait for a swapped component

#### Scenario: Console login and API seeding use the OpenShift Routes

The API server image does not include `curl`. Token grants, Keycloak Admin
updates, and seed POSTs SHALL run from the developer machine against the
Keycloak and API Routes, the same way `make kind-up` talks to Keycloak through
its hostname rather than `oc exec`. The imported realm only allows
`https://console.hypershell.localhost` redirect URIs; the driver SHALL set
`hypershell-frontend` redirect URIs to the web-console Route origin
(`https://<web-console-host>/auth/callback` and `https://<web-console-host>`)
so the BFF authorization-code callback succeeds. Wildcard redirect URIs SHALL
NOT be registered.

- GIVEN a developer runs `make openshift-up`
- WHEN the deployment is ready
- THEN `hypershell-frontend` redirect URIs include the web-console Route `/auth/callback`
- AND the driver obtained the seed API token from the Keycloak Route
- AND Keycloak accepts the BFF `redirect_uri` for that console host

#### Scenario: Remove the deployment

- GIVEN a HyperShell deployment exists from `make openshift-up`
- WHEN the developer runs `make openshift-down` or `make openshift-teardown`
- THEN the scripts delete the platform project and the companion `-keycloak` project
- AND the command does not return until both projects are gone, or until project
  deletion is forbidden and HyperShell resources in both projects have been removed
- AND when project deletion is forbidden, the scripts remove HyperShell
  resources from both projects, including Keycloak
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
namespace from that name (see the Keycloak Namespace requirement). Missing projects
SHALL be created with `oc new-project` (an OpenShift ProjectRequest), not
`oc create namespace`. `oc new-project` selects the new project as the current oc
project. After Keycloak is applied in the `-keycloak` project, the scripts SHALL
switch the current project back to the platform project for the rest of the
deployment.

The developer selects the target with `OPENSHIFT_NAMESPACE` or the current
`oc project`. That selection is the authority for `make openshift-up`. The
command SHALL deploy into that namespace group without prompting, and SHALL
NOT require permission to patch namespace objects. Typical developer accounts
on a shared cluster can create resources inside a project and cannot label the
Namespace itself.

When the account can patch namespaces, the scripts SHALL stamp every namespace
in the group with an ownership label that marks the namespace as HyperShell-owned
and with an immutable environment identifier that ties the namespace to one
deployment, so that `make openshift-status` and cleanup tooling can find every
HyperShell namespace and can tell which namespaces belong to the same
environment. When labeling is forbidden, the scripts SHALL warn and continue,
and SHALL recover a previously applied environment identifier from workload
labels when those labels are present.

Before it deploys, the command SHALL refuse an existing namespace whose
ownership label and environment identifier mark it as a different HyperShell
environment, so that a deployment cannot take over a namespace that another
environment owns. An unlabeled existing project is not foreign: the developer
already pointed `make openshift-up` at it. Reserved names (`default`, `kube-*`,
`openshift-*`) SHALL still be refused. The scripts SHALL derive per-tenant
gateway hostnames from the gateway base domain (the configured
`GATEWAY_API_BASE_DOMAIN`) and the platform namespace, so that two deployments
on one cluster do not share a hostname.

Before it deletes, `make openshift-down` SHALL use the same project selection as
`make openshift-up`. It SHALL refuse reserved names and namespaces whose
ownership labels mark them as a different HyperShell environment. An unlabeled
project is the developer's chosen target: the command SHALL attempt
`oc delete project` for the platform namespace and for
`${OPENSHIFT_NAMESPACE}-keycloak`. When project deletion is forbidden, the
command SHALL remove HyperShell resources inside both projects and SHALL leave
the projects. The command SHALL NOT require namespace labels in order to delete.
`make openshift-teardown` SHALL perform the same steps.

#### Scenario: Two developers share one cluster

- GIVEN developer A runs `make openshift-up` with `OPENSHIFT_NAMESPACE=alice`
- AND developer B runs `make openshift-up` with `OPENSHIFT_NAMESPACE=bob`
- WHEN both deployments are ready
- THEN each deployment runs in its own namespace group
- AND each deployment has distinct gateway hostnames
- AND neither deployment changes, conflicts with, or interacts with the other
- AND each environment's ClusterRole and ClusterRoleBinding names are prefixed with `${OPENSHIFT_NAMESPACE}-dev-`
- AND neither environment patches unprefixed names such as `hypershell-controller` that another instance (for example stage) already owns

#### Scenario: Namespace cleanup removes only one deployment

- GIVEN two HyperShell environment namespace groups exist on one cluster
- WHEN a developer runs `make openshift-down` for one environment
- THEN the scripts remove only that environment's platform project and `-keycloak` project, or the HyperShell resources in them
- AND the other environment stays intact

#### Scenario: Deployment refuses a foreign namespace

- GIVEN a namespace with the name `OPENSHIFT_NAMESPACE` already exists and carries a
  different environment identifier
- WHEN a developer runs `make openshift-up`
- THEN the command refuses to adopt the namespace
- AND the command deploys nothing into, and deletes nothing in, that namespace

#### Scenario: Existing unlabeled project is used without labeling

- GIVEN the current oc project already exists and is not HyperShell-labeled
- AND the developer cannot patch namespace objects
- WHEN the developer runs `make openshift-up`
- THEN the command deploys into that project without prompting
- AND the command does not stop because namespace labeling is forbidden
- AND `make openshift-down` for that project deletes the platform and `-keycloak` projects, or the HyperShell resources in them

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
scripts create missing ones with `oc new-project`, apply Keycloak while that
project is selected, switch back to the platform project for the remaining
components, and `make openshift-down` / `make openshift-teardown` (for local development) or the release step
(for CI) removes them together.

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

The API server JWT environment SHALL live in `deploy/openshift/kustomization.yaml`
with the other JWT and RBAC overlay patches, not in the lifecycle script. The
default `development` environment force-disables JWT after flag parsing, so
`--enable-jwt=true` is silent unless `API_ENV=development_oidc`. Route-derived
values (Keycloak `KC_HOSTNAME`, console redirect URIs, gateway OIDC issuer) remain
script-applied after Routes are assigned, because those hosts are not known at
kustomize-build time. The overlay SHALL re-declare any base container env the
JSON6902 env-array replace would otherwise drop, including
`HYPERSHELL_SERVICE_ACCOUNT_PROVISIONER_ADDR`. That address SHALL be the
cluster-local FQDN
`hypershell-controller.$(POD_NAMESPACE).svc.cluster.local:9443`, where
`POD_NAMESPACE` is the API server pod's namespace from the downward API
(`metadata.namespace`). The API server and controller share that namespace, so
the dial target matches the controller's `HYPERSHELL_NAMESPACE` instance
identity without a hardcoded `hypershell-system` segment. Namespace rewrite
does not substitute this value; kubelet expands `$(POD_NAMESPACE)` at pod
start. grpc-go does not apply kube-DNS search domains, so the short name
`hypershell-controller:9443` fails in-cluster even when `nc` to that short
name succeeds.

This spec defines only where Keycloak lands. The broader isolation of other
non-request-serving components (for example the database and observability) into
their own namespaces is out of scope here and belongs to a separate spec.

#### Scenario: Keycloak runs in its own namespace

- GIVEN a developer runs `make openshift-up` with `OPENSHIFT_NAMESPACE=alice`
- AND the `alice-keycloak` project does not exist
- WHEN the deployment is ready
- THEN the scripts have created `alice-keycloak` with `oc new-project`
- AND Keycloak runs in the `alice-keycloak` namespace
- AND every other HyperShell component runs in the `alice` namespace
- AND the current oc project is `alice` after Keycloak is applied
- AND the OIDC issuer points at the Keycloak route in `alice-keycloak`

#### Scenario: API_ENV is declared in the OpenShift overlay

- GIVEN `deploy/openshift/kustomization.yaml` is built
- WHEN the rendered API server Deployment is inspected
- THEN it SHALL set `API_ENV=development_oidc`
- AND it SHALL retain `HYPERSHELL_SERVICE_ACCOUNT_PROVISIONER_ADDR` as
  `hypershell-controller.$(POD_NAMESPACE).svc.cluster.local:9443` with
  `POD_NAMESPACE` from the downward API (`metadata.namespace`)
- AND `make openshift-up` SHALL NOT set `API_ENV` with `oc set env`

#### Scenario: Platform workloads can reach Keycloak across the namespace group

`oc new-project` installs default-deny Ingress NetworkPolicies in each
project (same-namespace pods and the `openshift-ingress` namespace). Keycloak
runs in `${OPENSHIFT_NAMESPACE}-keycloak`, so platform pods cannot reach it
until an additional policy allows that traffic. Without it, the API server
hangs while loading JWKS and the rollout never completes.

- GIVEN a developer runs `make openshift-up` with `OPENSHIFT_NAMESPACE=alice`
- AND `oc new-project` has created default-deny Ingress policies in `alice-keycloak`
- WHEN the deployment is ready
- THEN a NetworkPolicy in `alice-keycloak` allows TCP/8080 from the `alice` namespace
- AND the API server can load JWKS from `keycloak-service.alice-keycloak`
- AND the control plane can reach the Keycloak Admin API on that Service

#### Scenario: The namespace group shares one lifecycle

- GIVEN a deployment has a platform namespace and its `-keycloak` namespace
- WHEN the developer runs `make openshift-down` or `make openshift-teardown`
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
than from a local archive, the OpenShift driver SHALL push the built image to
`SWAP_REGISTRY`, a laptop-reachable registry prefix the cluster can pull, rather
than use the Kind-specific `kind load image-archive`. `SWAP_REGISTRY` SHALL be the registry org prefix only (for example
`quay.io/<org>`), not an image repository. The driver SHALL append a default
repository name for the component being swapped: `hypershell-api-server` for
the API server, `hypershell-controller` for the control plane, and
`hypershell-web-console` for the web console. When `SWAP_REPOSITORY` is set,
the driver SHALL use that repository name instead of the component default.
`SWAP_REGISTRY` SHALL be required: when it is unset, or when it is only a
hostname with no org path, the swap SHALL stop with a clear error. The driver
SHALL NOT default `SWAP_REGISTRY` to `IMAGE_REGISTRY`; `IMAGE_REGISTRY` is the
baseline image prefix and SHALL NOT be the swap push destination.
`OPENSHIFT_IMAGE_REGISTRY` SHALL NOT be used. The driver SHALL NOT push to the
OpenShift internal registry (`image-registry.openshift-image-registry.svc`)
from the laptop: that hostname does not resolve outside the cluster, and many
shared clusters (ROSA/OSD) expose no public registry hostname. The swap SHALL
authenticate the container engine with `PULL_SECRET` (a
`kubernetes.io/dockerconfigjson` Secret YAML, or a raw Docker config JSON).
`KIND_PULL_SECRET` SHALL remain accepted as an alias when `PULL_SECRET` is
unset. When that file contains credentials for the `SWAP_REGISTRY` host, the
swap SHALL log in with those credentials and SHALL NOT require an interactive
`podman login`. After a successful push, the driver SHALL update the component
Deployment image refs to the pushed identity. The digest SHALL be the registry
manifest digest recorded by that push (`podman push --digestfile`, or a
`digest: sha256:...` line from a docker-style push log). The driver SHALL NOT
pin a digest from `inspect` of the local image, and SHALL NOT scrape blob or
config SHAs from push progress: those values are not the digest the registry
stores, so a cluster pull by that digest fails with manifest unknown. When no
registry digest is available, the driver SHALL pin the unique tag.

The swap build SHALL target the OpenShift node architecture, not the laptop
architecture. When `SWAP_PLATFORM` is set (`linux/amd64` or `linux/arm64`),
the driver SHALL use that architecture. When it is unset, the driver SHALL read
the architecture from the cluster nodes. The driver SHALL pass
`--platform linux/<arch>` to the container build. Component Dockerfiles SHALL
pin Red Hat Hardened Image manifests per architecture (`amd64` and `arm64`)
and SHALL select the pin with `TARGETARCH` (and `BUILDARCH` for a native Go
toolchain). A single-arch pin SHALL NOT be used: that produces `Exec format
error` when an arm64 laptop image is pulled by amd64 nodes.

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
- AND the scripts push the image to `${SWAP_REGISTRY}/hypershell-api-server`
  (or `${SWAP_REGISTRY}/${SWAP_REPOSITORY}` when that override is set), not
  `IMAGE_REGISTRY` and not the OpenShift internal registry
- AND the scripts update the API server Deployment image refs to that identity
- AND the scripts record that image identity in the per-namespace swap state
- AND the scripts roll out the API server deployment to the pushed image
- AND `make openshift-status` reports the API server as a working-tree build with
  its exact image identity

#### Scenario: Swap pins the registry digest from the push

- GIVEN the container engine's local image digest differs from the digest the
  registry stored for the pushed tag
- WHEN the developer runs `make openshift-api-server-up`
- THEN the scripts pin the Deployment to the registry manifest digest recorded
  by that push
- AND they SHALL NOT pin a digest from `inspect` of the local image
- AND they SHALL NOT scrape blob or config SHAs from push progress

#### Scenario: Swap builds for the cluster architecture

- GIVEN the developer laptop is arm64
- AND the OpenShift nodes are amd64
- WHEN the developer runs `make openshift-api-server-up`
- THEN the scripts build the API server with `--platform linux/amd64`
- AND the Dockerfiles select the amd64 HI digest pins
- AND the migrate init container SHALL start without `Exec format error`

#### Scenario: Swap without SWAP_REGISTRY stops

- GIVEN a HyperShell deployment exists on OpenShift with baseline images
- AND `SWAP_REGISTRY` is unset
- WHEN the developer runs `make openshift-api-server-up`
- THEN the command SHALL stop with a clear error
- AND it SHALL NOT push to `IMAGE_REGISTRY`
- AND it SHALL NOT push to the OpenShift internal registry

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

The OpenShift e2e suite SHALL run with `bash tests/e2e/e2e-openshell.sh` against
a KUBECONFIG context pointed at the OpenShift cluster -- the suite auto-detects
the OpenShift driver from that context, or a caller MAY force it explicitly
with `E2E_INFRA_DRIVER=openshift bash tests/e2e/e2e-openshell.sh` -- and SHALL
exercise the same test areas that the Kind suite exercises, so that a single
suite validates both infrastructure targets.

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

The legacy `components/pr-test/e2e-openshell.sh` script SHALL be superseded by the
shared e2e harness and the pull-request workflow in
`ephemeral-pr-environments.spec.md`. That spec owns the deprecation window:
the script SHALL stay present and runnable, SHALL carry a deprecation notice,
and SHALL be removed only after manual usage has migrated. The ROKS variant
`components/pr-test/e2e-openshell-roks.sh` is out of scope there and SHALL remain.
The `pr_test` component and its CI wiring SHALL remain until both scripts are
gone.

The eventual end state is unchanged: OpenShift-specific logic lives in
`tests/e2e/drivers/openshift.sh`, infrastructure-agnostic tests live in
`tests/e2e/e2e-openshell.sh`, and no OpenShift e2e logic remains hardcoded
outside the driver model. This spec SHALL NOT require the script or the
`pr_test` component to already be removed.

#### Scenario: Legacy script is deprecated, not yet removed

- GIVEN `components/pr-test/e2e-openshell.sh` still has manual users
- WHEN this spec is in effect
- THEN the script remains present and runnable
- AND `ephemeral-pr-environments.spec.md` is the contract for when it is removed
- AND `components/pr-test/` is not removed while the ROKS variant still lives there

#### Scenario: No dangling references after eventual removal

- GIVEN `e2e-openshell.sh` has been removed after the deprecation window
- AND the ROKS variant has been retired or rehomed
- WHEN a maintainer inspects CI configuration and component registration
- THEN no workflow references the removed path
- AND `.github/component-paths.json` no longer contains a `pr_test` entry that
  points to a removed path

### Requirement: Pull-Request CI Uses This Lifecycle

Automated OpenShift pull-request CI is specified in
`ephemeral-pr-environments.spec.md` (HYPERSHELL-240). That spec owns namespace
naming, deploy triggers, origin-only trust boundary, timebox and reaping, the
access comment, GitHub-brokered Keycloak, e2e grant selection, and the
`e2e-openshell.sh` deprecation window. This spec SHALL NOT restate those
requirements.

The pull-request workflow SHALL deploy and release with the same lifecycle this
spec defines: `make openshift-up` and `make openshift-down`, the
`deploy/openshift/` overlay, the namespace-group derivation, the ownership
labels `hypershell.redhat.io/owned` and `hypershell.redhat.io/environment`, and
the OpenShift e2e driver. A local `make openshift-up` and a CI deploy SHALL NOT
drift. `make openshift-up` SHALL NOT stamp a pull-request timebox; expiry is a
CI annotation defined in `ephemeral-pr-environments.spec.md`.

Kind e2e, including the merge-queue gate, stays in `e2e-testing.spec.md`. The
OpenShift pull-request job SHALL run for origin `pull_request` events that
trigger Kind e2e, and for changes under `deploy/openshift/` or the OpenShift
lifecycle scripts. It SHALL NOT run on `merge_group`.

#### Scenario: CI reuses the local-dev lifecycle

- GIVEN an origin pull request that triggers this workflow
- WHEN the workflow deploys the environment
- THEN it SHALL run `make openshift-up` with `OPENSHIFT_NAMESPACE` set as
  `ephemeral-pr-environments.spec.md` defines
- AND it SHALL run the OpenShift e2e driver this spec defines
- AND it SHALL NOT use a second OpenShift bring-up path

#### Scenario: Merge-queue stays on Kind

- GIVEN a pull request enters the GitHub merge queue
- WHEN CI evaluates which jobs to run
- THEN the Kind e2e job SHALL run as `e2e-testing.spec.md` defines
- AND the OpenShift pull-request environment workflow SHALL NOT run

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
infrastructure. The GitHub OAuth App and stable callback that
`ephemeral-pr-environments.spec.md` requires are additional cluster infrastructure
for pull-request CI only; local `make openshift-up` does not depend on them.

The `make openshift-up` command SHALL check for the required Gateway
infrastructure, and SHALL report a clear error when it is missing, rather than
deploy a broken environment. The pull-request workflow in
`ephemeral-pr-environments.spec.md` SHALL run against a target cluster that
already provides the Gateway infrastructure and the GitHub OAuth callback.

#### Scenario: Missing infrastructure fails fast

- GIVEN an OpenShift cluster without the shared Gateway
- WHEN a developer runs `make openshift-up`
- THEN the command reports that the required infrastructure is missing
- AND the command does not deploy a broken environment

#### Scenario: CI target provides the infrastructure

- GIVEN the CI workflow deploys into an ephemeral environment
- WHEN the workflow prepares the environment for deployment
- THEN the workflow relies on the target cluster for the shared Gateway, the
  GatewayClass, the certificate issuer, and the GitHub OAuth callback
  `ephemeral-pr-environments.spec.md` requires
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
service account of an unknown future ephemeral namespace.

For local-dev on a shared cluster, `make openshift-up` SHALL apply the
ClusterRole and ClusterRoleBinding from `deploy/openshift/` after prefixing
their names with `${OPENSHIFT_NAMESPACE}-dev-`. The ClusterRole SHALL include
`bind` on `clusterroles` and `clusterrolebindings`, matching the stage
controller ClusterRole. The command SHALL NOT create, patch, or delete
unprefixed ClusterRole `hypershell-controller` or ClusterRoleBinding
`hypershell-controller`; those names belong to other instances that share the
cluster, such as stage. Built-in ClusterRoles whose names start with `system:`
SHALL NOT be renamed.

If the prefixed ClusterRole apply is Forbidden (Kubernetes escalation
prevention: the ClusterRole grants verbs the current user does not hold, for
example `routes/custom-host`) and ClusterRole `hypershell-controller` exists,
the command SHALL apply a ClusterRoleBinding named
`${OPENSHIFT_NAMESPACE}-dev-hypershell-controller` whose `roleRef` is that
existing ClusterRole and whose subject is this environment's controller
service account. It SHALL NOT create a ClusterRole on that path, and it SHALL
NOT look for `hypershell-controller-scc-bind`. ClusterRoleBinding `roleRef` is
immutable, so if a ClusterRoleBinding of that name already points at a
different ClusterRole (for example a previous apply created the binding before
the ClusterRole was rejected), the command SHALL delete and recreate it so the
binding points at `hypershell-controller`. The command SHALL apply the
ClusterRole before the ClusterRoleBinding so a failed ClusterRole create does
not leave a binding that cannot be retargeted.

Kubernetes escalation prevention forbids a typical developer from granting
those ClusterRoles, so when both the prefixed ClusterRole and the existing-role
fallback fail, the command SHALL warn and continue with the rest of the stack.
Gateways and sandboxes will not provision until this environment's controller
is bound. `make openshift-down` SHALL delete this environment's
`${OPENSHIFT_NAMESPACE}-dev-*` ClusterRoles and ClusterRoleBindings and SHALL
NOT delete unprefixed `hypershell-controller`.

The deployment into the ephemeral namespace SHALL NOT attempt to grant the
cluster-scoped `bind` through a namespace-scoped RoleBinding, and SHALL NOT assume the
namespace-scoped RoleBindings alone let the controller bind the privileged SCC per
namespace.

#### Scenario: Ephemeral namespace has the permissions the overlay needs

- GIVEN the developer can create ClusterRoles and ClusterRoleBindings prefixed with `${OPENSHIFT_NAMESPACE}-dev-`
- AND the developer can apply the privileged SCC RoleBinding in that namespace
- WHEN the developer runs `make openshift-up`
- THEN the command applies ClusterRole and ClusterRoleBinding `${OPENSHIFT_NAMESPACE}-dev-hypershell-controller` from `deploy/openshift/`
- AND the command does not create or patch ClusterRole `hypershell-controller` or ClusterRoleBinding `hypershell-controller`
- AND applies RoleBinding `hypershell-sandbox-scc` for the sandbox service account
- AND the controller can create the per-namespace privileged RoleBinding for a sandbox because it was granted `bind`
- AND the deployment does not attempt to grant the cluster-scoped `bind` through a namespace-scoped RoleBinding

#### Scenario: Prefixed ClusterRole is forbidden, existing ClusterRole is used

- GIVEN the developer cannot create ClusterRole `${OPENSHIFT_NAMESPACE}-dev-hypershell-controller` because of escalation prevention
- AND ClusterRole `hypershell-controller` already exists on the cluster
- AND the developer can create ClusterRoleBindings prefixed with `${OPENSHIFT_NAMESPACE}-dev-`
- WHEN the developer runs `make openshift-up`
- THEN the command applies ClusterRoleBinding `${OPENSHIFT_NAMESPACE}-dev-hypershell-controller` with `roleRef` `hypershell-controller`
- AND if that ClusterRoleBinding already pointed at a different ClusterRole, the command replaces it
- AND the command does not create a ClusterRole
- AND the command does not create or patch ClusterRoleBinding `hypershell-controller`

#### Scenario: Cluster-scoped RBAC cannot be applied

- GIVEN the current user cannot create ClusterRoles or ClusterRoleBindings
- WHEN the developer runs `make openshift-up`
- THEN the command warns that this environment is not bound
- AND the command continues and applies the namespaced overlay
- AND gateways and sandboxes will not provision until this environment's controller is bound

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
