# Ephemeral Pull-Request Environments Specification

**Date:** 2026-09-04
**Status:** Draft
**Jira:** HYPERSHELL-240
**Related:** `openshift-development.spec.md` (HYPERSHELL-44) -- `make openshift-up`
             lifecycle, the blessed `deploy/openshift/` overlay, ephemeral-namespace
             isolation, cluster-scoped RBAC, and the OpenShift e2e driver
             (this spec owns pull-request CI on top of that lifecycle);
             `e2e-testing.spec.md` (HYPERSHELL-18) -- the infra-agnostic e2e suite,
             the grant-agnostic driver interface contract, Kind CI and the
             merge-queue gate, and Konflux image consumption;
             `local-development.spec.md` -- the Keycloak realm/client model this
             spec builds on;
             `oidc-integration.spec.md` -- platform OIDC and RBAC role mapping;
             `../security/rbac-enforcement.spec.md` -- `platform:admin` vs
             `gateway:creator`

## Purpose

HyperShell delivers a live OpenShift environment for every open pull request and
keeps that environment in continuous deployment for the life of the pull request.
When a pull request opens, CI deploys the full stack into a per-PR ephemeral
namespace group on a shared target OpenShift cluster, waits for Konflux to build
the pull request's component images, swaps those images into the environment,
runs the OpenShift e2e suite against it, and posts a pull-request comment telling
the developer how to log in. When a later commit is pushed to the same pull
request, CI does not create a second environment: it reuses the existing one,
waits for Konflux to rebuild the changed images, swaps them in, reruns the e2e
suite, and updates the comment to say the environment now runs that commit. The
environment lives independently of any single CI run so a developer can use it as
a live debug and development target, and it is reaped after a fixed timebox so an
abandoned pull request cannot hold cluster resources.

This spec owns the automated OpenShift pull-request CI workflow. The
per-namespace deployment, reconcile, image swap-by-digest, and OpenShift e2e
driver remain defined in `openshift-development.spec.md`; the infra-agnostic e2e
suite, Kind CI (including the merge-queue gate), and Konflux image gating remain
defined in `e2e-testing.spec.md`. This spec adds the pull-request-scoped
concerns those specs leave open: the deterministic per-PR namespace naming, the
continuous-deployment triggers across the pull request lifecycle, the
deploy-serialization rule, the origin-only trust boundary, the fixed timebox and
external reaping, the per-commit pull-request comment, and a Keycloak
authentication model that brokers to GitHub (restricted to the
`openshift-online` organization plus an allowlist) instead of Red Hat SSO.
GitHub brokering lets an allowlisted outside contributor log in to an
already-deployed origin-repo environment; it does not deploy fork pull requests
onto the shared cluster.

### Scope

This spec covers:

- the per-pull-request ephemeral environment naming and identity,
- the continuous-deployment lifecycle triggered by pull-request open, synchronize
  (push), reopen, and close/merge on the origin repository,
- the origin-only trust boundary (fork pull requests do not receive cluster
  credentials),
- the per-pull-request deploy serialization rule,
- the fixed-duration timebox and the out-of-band reaper,
- the per-commit pull-request comment and secure credential handoff,
- the GitHub-brokered Keycloak authentication model for these environments,
  including the GitHub OAuth App, the stable callback, admin authorization, and
  developer-tier testing by impersonation, and
- the deprecation of the legacy `components/pr-test/e2e-openshell.sh` script in
  favor of the shared e2e harness and this workflow (the ROKS variant is out of
  scope).

This spec does not redefine `make openshift-up`, the `deploy/openshift/` overlay,
the ephemeral-namespace isolation rules, the cluster-scoped RBAC handling, or
the Konflux build pipeline. It depends on all of them. It amends the OpenShift
`acquire_oidc_token` / `acquire_gateway_token_with_role` grant in
`e2e-testing.spec.md` so those functions are grant-agnostic; it does not change
their signatures or the suite's call sites. Kind merge-queue e2e stays in
`e2e-testing.spec.md` and is not this workflow. It is a behavior contract, not
the CI YAML or the Keycloak realm export.

### Reserved Terms

This spec adds no new domain kinds. It refers to the existing kinds (Gateway,
GatewayNetwork, GatewayRelease, ManagedCluster, ManagedDatabase) only where a
scenario provisions one. "Environment" here means the per-pull-request namespace
group (the platform namespace and its companion `-keycloak` namespace) that
`openshift-development.spec.md` defines.

## Requirements

### Requirement: Per-Pull-Request Environment Identity

Each pull request SHALL map to exactly one ephemeral environment on the shared
target cluster, and that mapping SHALL be deterministic from the pull-request
number so that every CI run for the same pull request resolves to the same
environment without storing external state. The CI workflow SHALL set
`OPENSHIFT_NAMESPACE` to `hypershell-ci-pr-<pr-number>` (for example
`hypershell-ci-pr-232`), and the companion Keycloak namespace SHALL therefore be
`hypershell-ci-pr-<pr-number>-keycloak`, following the namespace-group derivation
in `openshift-development.spec.md`. The platform namespace name SHALL remain a
valid RFC 1123 DNS label within the 54-character bound that keeps the derived
`-keycloak` name under the 63-character limit; because pull-request numbers are
short, the `hypershell-ci-pr-` prefix leaves ample room.

The workflow SHALL stamp both namespaces with the same ownership labels the
OpenShift lifecycle driver uses, so the reaper and `make openshift-status` can
attribute every namespace to its pull request:

- `hypershell.redhat.io/owned=true`
- `hypershell.redhat.io/environment=pr-<pr-number>` (for example `pr-232`)
- `app.kubernetes.io/managed-by=hypershell-lifecycle`
- `app.kubernetes.io/part-of=hypershell`

The `pr-` prefix on the environment identifier distinguishes pull-request
environments from local `make openshift-up` environments, whose identifier is
an opaque per-deployment value, not `pr-*`. After `make openshift-up`, the
workflow SHALL set `hypershell.redhat.io/environment=pr-<pr-number>` on both
namespaces, overwriting the identifier that command assigned, so later
reconciles recover `pr-<number>` from the namespace and the reaper can match
it. The CI service account SHALL be able to patch namespace objects;
if labeling either namespace fails, the workflow SHALL fail the job and SHALL
NOT continue. The local-dev "warn and continue" path in
`openshift-development.spec.md` does not apply to this workflow. The workflow
SHALL NOT derive the namespace from the branch name, the commit SHA, or the
developer identity, because those are not stable across the life of one pull
request.

#### Scenario: Namespace derives from the pull-request number

- GIVEN a pull request numbered 232 runs the workflow
- WHEN the workflow resolves the target environment
- THEN it SHALL set `OPENSHIFT_NAMESPACE=hypershell-ci-pr-232`
- AND the Keycloak namespace SHALL be `hypershell-ci-pr-232-keycloak`
- AND both namespaces SHALL carry `hypershell.redhat.io/owned=true` and
  `hypershell.redhat.io/environment=pr-232`

#### Scenario: Labeling failure fails the job

- GIVEN the CI service account cannot patch namespace objects
- WHEN the workflow attempts to stamp the namespace group
- THEN the job SHALL fail
- AND it SHALL NOT leave an unlabeled pull-request environment

#### Scenario: Same pull request always resolves the same environment

- GIVEN pull request 232 already has an environment from an earlier run
- WHEN any later run for pull request 232 starts
- THEN it SHALL resolve `hypershell-ci-pr-232` without consulting external state
- AND it SHALL NOT create a second environment for the same pull request

### Requirement: Continuous Deployment Across the Pull-Request Lifecycle

The workflow SHALL keep the pull request's environment continuously deployed to
the pull request's current head commit for the life of the pull request. It SHALL
trigger on origin-repository pull-request `opened`, `reopened`, and `synchronize`
(a new commit pushed to the pull-request branch), and it SHALL trigger on
`closed` (which covers both merge and close) to release the environment (see the
Timebox and Reaping requirement). It SHALL NOT trigger on `merge_group`. Kind
e2e, as `e2e-testing.spec.md` defines, remains the merge-queue gate; this
workflow does not share a namespace with a merge-queue SHA.

The workflow SHALL run only for pull requests targeting the origin repository.
Fork pull requests SHALL NOT receive cluster credentials and SHALL NOT get an
environment (see Pull-Request Trust Boundary). The workflow SHALL NOT use
`pull_request_target`.

On every deploying trigger (`opened`, `reopened`, `synchronize`), the workflow
SHALL run `make openshift-up` unconditionally, whether or not the environment
already exists. Because `make openshift-up` is idempotent and reconciling
(`openshift-development.spec.md`), one code path both creates the environment on
first run and reconciles it to the current overlay on later runs; the workflow
SHALL NOT branch on a "does the environment exist" check before deciding whether
to deploy. After a successful `make openshift-up`, the workflow SHALL refresh
the environment's timebox (see the Timebox and Reaping requirement), so an
active pull request is continuously renewed while an abandoned one expires.
`make openshift-up` itself SHALL NOT stamp or refresh that timebox; local
environments are not time-boxed by this spec.

Deploying runs for the same pull request SHALL serialize on a per-pull-request
concurrency group. A newer run SHALL cancel or queue an older in-flight run for
that pull request so two swaps cannot leave a mixed digest set. The access
comment's commit SHA SHALL be the commit whose digest swap completed, not a
cancelled run's head.

The reconcile SHALL bring the environment to the current desired state, including
pruning resources the overlay no longer declares, so the long-lived environment
does not drift across the many deployments a pull request accumulates. The
reconcile SHALL preserve any active per-namespace component swap the same way
`openshift-development.spec.md` specifies.

#### Scenario: Pull request opens

- GIVEN a pull request is opened and has no environment yet
- WHEN the workflow runs
- THEN it SHALL run `make openshift-up` with `OPENSHIFT_NAMESPACE=hypershell-ci-pr-<number>`
- AND the environment SHALL be created and deployed
- AND the workflow SHALL post the initial access comment (see Pull-Request Comment)

#### Scenario: Commit pushed to an existing pull request

- GIVEN pull request 232 already has a running environment
- WHEN a new commit is pushed and the `synchronize` trigger fires
- THEN the workflow SHALL reuse `hypershell-ci-pr-232` and SHALL NOT create a new environment
- AND it SHALL run `make openshift-up` to reconcile the environment
- AND it SHALL wait for Konflux to build the new commit's images and swap them in
  by digest (see Image Gating and Swap)
- AND it SHALL rerun the e2e suite against the environment
- AND it SHALL update the access comment to reflect the new head commit (see
  Pull-Request Comment)

#### Scenario: Deploy runs unconditionally

- GIVEN a deploying trigger for a pull request
- WHEN the workflow reaches the deploy step
- THEN it SHALL invoke `make openshift-up` regardless of whether the environment
  already exists
- AND it SHALL NOT skip deployment based on a prior-existence check

#### Scenario: Overlapping runs serialize per pull request

- GIVEN a deploying run for pull request 232 is already in flight
- WHEN a later `synchronize` run for pull request 232 starts
- THEN the workflow SHALL cancel or queue the older run
- AND at most one deploying run SHALL swap images into `hypershell-ci-pr-232` at
  a time
- AND the access comment SHALL name the commit whose digest swap completed

#### Scenario: Merge-queue does not use this environment

- GIVEN a pull request enters the GitHub merge queue
- WHEN CI evaluates which jobs to run
- THEN this workflow SHALL NOT run
- AND the Kind e2e job SHALL remain the merge-queue gate as
  `e2e-testing.spec.md` defines

### Requirement: Image Gating and Swap

The workflow SHALL NOT build component images itself. It SHALL gate on the
Konflux builds for the pull request's head commit and swap the built images into
the environment by digest, reusing the same digest-injection mechanism the Kind
e2e job uses (`scripts/kind/set-component-images.sh`) as
`openshift-development.spec.md` and `e2e-testing.spec.md` define. The workflow
SHALL overlap environment bring-up with the Konflux builds: it MAY run
`make openshift-up` with baseline images while Konflux builds are still in
flight, then wait for each changed component's build to conclude and swap that
component's image by digest, so cluster reconcile time is hidden behind build
time. Unchanged components SHALL keep baseline registry images. The workflow
SHALL determine which components to wait for using the shared change-detection and
Konflux-trigger-mirroring rules that `e2e-testing.spec.md` defines, so it never
falls back to a baseline image while Konflux is building an image the pull request
produced.

The workflow SHALL NOT consume an untrusted, mutable image tag when an immutable
digest is available. Once a component's Konflux build has concluded, the workflow
SHALL resolve that build's output to its manifest digest and inject the image into
the environment by `@sha256:<digest>`, not by a floating tag such as
`on-pr-<head_sha>`, so the environment runs exactly the artifact CI verified and a
tag that is later re-pushed cannot silently change what the environment runs.
Baseline images for unchanged components SHALL likewise be pinned by digest when
the registry exposes one; a tag SHALL be used only as a last resort when no
digest is available, and that fallback SHALL be recorded in the run output rather
than passed silently. This reinforces the cross-cutting image-reference
consistency convention: image references SHALL resolve to the same immutable
artifact across the stack.

#### Scenario: Swap the pull request's images by digest

- GIVEN a pull request changed `components/api-server/`
- AND Konflux built the API server image for the head commit
- WHEN the workflow deploys the environment
- THEN it SHALL wait for that build to conclude
- AND it SHALL swap the API server image into the environment by digest
- AND the control plane and web console SHALL keep baseline registry images

#### Scenario: New commit rebuilds and re-swaps

- GIVEN a new commit changes `components/control-plane/`
- WHEN the `synchronize` trigger deploys the environment
- THEN the workflow SHALL wait for the control plane's Konflux build for the new
  head commit
- AND it SHALL swap the new control plane image into the environment by digest
  before running e2e

#### Scenario: Immutable digest is preferred over a mutable tag

- GIVEN a component's Konflux build has concluded and exposes both a floating
  `on-pr-<head_sha>` tag and a manifest digest
- WHEN the workflow injects that image into the environment
- THEN it SHALL pin the image by `@sha256:<digest>`
- AND it SHALL NOT deploy the image by its mutable tag
- AND when no digest is available it SHALL fall back to the tag and record that
  fallback in the run output

### Requirement: E2E Execution Against the Environment

After the environment is deployed and the pull request's images are swapped in,
the workflow SHALL run the OpenShift e2e suite against it, exactly as
`e2e-testing.spec.md` and `openshift-development.spec.md` define: it SHALL run
`E2E_INFRA_DRIVER=openshift E2E_OIDC_GRANT=client_credentials bash tests/e2e/e2e-openshell.sh` against a KUBECONFIG
context pointed at the environment, exercising the same test areas the Kind suite
exercises. The suite SHALL run on the pull request's first deployment and on every
later deployment for that pull request, so each commit is validated against a live
environment the same way the Kind e2e job validates each commit today. On failure
the workflow SHALL collect the diagnostics `e2e-testing.spec.md` defines. Whether
the suite passes or fails, the environment SHALL survive (see Timebox and
Reaping), so a developer can inspect a failing run on the live environment.

The e2e suite's authentication SHALL set `E2E_OIDC_GRANT=client_credentials` and
use the non-interactive path this spec defines (see Automated E2E Authentication),
because the environment's interactive login is GitHub-brokered and brokered users
have no password grant.

#### Scenario: E2E runs on every deployment

- GIVEN the environment is deployed and the pull request's images are swapped in
- WHEN the workflow reaches the test step
- THEN it SHALL run the OpenShift e2e suite against the environment
- AND it SHALL run the suite again on each later commit's deployment

#### Scenario: Environment survives a failing run

- GIVEN the e2e suite fails
- WHEN the workflow finishes
- THEN it SHALL collect the failure diagnostics
- AND the environment SHALL remain deployed for developer inspection

### Requirement: Timebox and Reaping

Each pull-request environment SHALL be time-boxed to three days and SHALL be
reaped out-of-band, independently of any CI run, so a stale or abandoned pull
request cannot hold cluster resources. After each successful `make openshift-up`,
the CI workflow SHALL stamp both namespaces in the group with the annotation
`hypershell.redhat.io/expires-at` set to an RFC 3339 timestamp three days in the
future (or a reservation duration of three days when the environment is
provisioned through a reservation mechanism such as the
ephemeral-namespace-operator). `make openshift-up` SHALL NOT write that
annotation; local environments this command creates are not time-boxed by this
spec. Because every deploying trigger refreshes the annotation after bring-up,
an actively worked pull request is continuously renewed and never reaped
mid-flight, while a pull request with no activity for three days falls past its
expiry and is reclaimed. The three-day timebox SHALL be configurable through a
single documented workflow setting rather than hardcoded in the workflow logic.

Reaping SHALL be performed by an out-of-band mechanism -- a scheduled reaper job
or the reservation mechanism's own duration -- not by the pull-request workflow's
own teardown step, so that an environment expires even when no further CI runs for
that pull request. The reaper SHALL delete a namespace group whose
`hypershell.redhat.io/expires-at` has passed, and SHALL identify HyperShell-owned
pull-request environments only when all of these match:

- `hypershell.redhat.io/owned=true`
- `hypershell.redhat.io/environment` equal to `pr-<number>`
- namespace name prefixed with `hypershell-ci-pr-`

The reaper SHALL NOT delete namespaces that fail that match, including local
`make openshift-up` environments and any other HyperShell-owned namespace whose
environment identifier is not `pr-*`. It SHALL refuse reserved names
(`default`, `kube-*`, `openshift-*`).

On pull-request `closed` (merge or close), the workflow SHALL release the
environment as the primary path by removing the namespace group the same way
`make openshift-down` does. The timebox SHALL remain the backstop for the case
where the close event does not fire or its release cannot be confirmed; when the
release step cannot confirm the release, the workflow SHALL report the failure so
an operator can free the environment.

#### Scenario: Deploying run refreshes the expiry

- GIVEN a pull-request environment exists
- WHEN a new commit's deploying run finishes `make openshift-up`
- THEN both namespaces SHALL have `hypershell.redhat.io/expires-at` reset to
  three days from that run
- AND `make openshift-up` SHALL NOT have written that annotation
- AND the environment SHALL NOT be reaped while the pull request stays active

#### Scenario: Abandoned environment is reaped by the timebox

- GIVEN a pull-request environment has had no deploying run for three days
- AND the pull request was neither merged nor closed
- WHEN the out-of-band reaper evaluates environments
- THEN it SHALL delete the expired namespace group
- AND it SHALL delete only namespaces matching the pull-request ownership labels
  and `pr-*` environment identifier

#### Scenario: Local environments are not reaped

- GIVEN a developer ran `make openshift-up` into a namespace that is not
  `hypershell-ci-pr-*`
- WHEN the out-of-band reaper evaluates environments
- THEN it SHALL NOT delete that namespace group

#### Scenario: Close releases the environment; timebox backstops

- GIVEN a pull-request environment exists
- WHEN the pull request merges or closes
- THEN the workflow SHALL remove the environment's namespace group as the primary path
- AND when the close event does not fire, the timebox SHALL reclaim the environment
- AND when release cannot be confirmed, the workflow SHALL report the failure

#### Scenario: Idempotent re-run after reaping

- GIVEN a pull request's environment was reaped after its timebox
- WHEN a new commit is pushed to that still-open pull request
- THEN `make openshift-up` SHALL recreate the environment under the same
  `hypershell-ci-pr-<number>` name
- AND the workflow SHALL stamp a fresh `hypershell.redhat.io/expires-at`
- AND the workflow SHALL proceed with image swap, e2e, and comment as on first open

### Requirement: Pull-Request Comment and Access Handoff

The workflow SHALL communicate the live environment to the developer through a
pull-request comment, and SHALL keep exactly one such comment current for the
pull request rather than post a new comment per run, so the pull request shows the
live environment's current state. The comment SHALL contain a stable hidden HTML
marker (`<!-- hypershell-pr-environment -->`) so later runs can find and update
that comment rather than any other comment on the pull request. The comment SHALL
contain the same non-secret access facts `openshift-development.spec.md` defines
-- the environment namespaces, the OpenShift console URL for the platform
namespace, the API Route URL, and the web-console Route URL -- presented as the
same login guidance `make openshift-up` prints at the end of a successful
bring-up, so the comment and the command agree.

On the pull request's first deployment, the workflow SHALL post the initial
comment once the environment is ready. On each later deployment for the same pull
request, the workflow SHALL update the marked comment to state that the
environment has been updated to commit `<sha>` and SHALL refresh the same login
details. The `<sha>` in the comment SHALL be the commit whose digest swap
completed, so the comment never claims a commit the swap did not deploy.

The comment SHALL NOT contain any credential. It MAY include an `oc login`
template with the credential redacted (for example
`oc login --server=<api-url> --token=<redacted>`). The credential itself SHALL be
delivered only through a channel that only an authorized developer can read, and
SHALL be short-lived and namespace-scoped, as `openshift-development.spec.md`
requires. No kubeconfig, token, or password SHALL appear in the comment, the job
logs, or a public artifact.

#### Scenario: Initial comment on pull-request open

- GIVEN a pull request is opened and its environment becomes ready
- WHEN the workflow finishes deploying
- THEN it SHALL post one pull-request comment with the namespaces, console URL,
  API Route URL, and web-console Route URL
- AND the comment SHALL contain the hidden marker `<!-- hypershell-pr-environment -->`
- AND the comment SHALL present the same login details `make openshift-up` prints
- AND the comment SHALL NOT contain a credential

#### Scenario: Comment updated on each new commit

- GIVEN a pull request already has an access comment that carries the marker
- WHEN a new commit's digest swap completes
- THEN the workflow SHALL update that marked comment to say the environment was
  updated to commit `<sha>`
- AND `<sha>` SHALL be the commit whose digest swap completed
- AND the workflow SHALL NOT post a second access comment
- AND the comment SHALL refresh the login details

#### Scenario: Credentials never leak

- GIVEN the workflow delivers access details
- WHEN a reader inspects the comment, the job logs, and public artifacts
- THEN no kubeconfig, token, or password appears in any of them
- AND the credential is available only through a secure channel

### Requirement: Pull-Request Trust Boundary

The workflow SHALL run only on `pull_request` events from the origin repository
(the repository that holds the workflow and the cluster credentials). It SHALL
NOT use `pull_request_target`. Fork pull requests SHALL NOT receive the cluster
kubeconfig, the GitHub OAuth client secret, or any other secret this workflow
needs, and SHALL NOT get an environment. The GitHub organization gate and
allowlist (see GitHub-Brokered Keycloak Authentication) govern interactive
login to an already-deployed origin-repo environment; they SHALL NOT be used as
a reason to deploy untrusted pull-request trees with cluster credentials.

#### Scenario: Origin pull request is deployed

- GIVEN a pull request opened against the origin repository by a repository
  collaborator
- WHEN the workflow runs
- THEN it SHALL deploy the environment with the cluster credentials

#### Scenario: Fork pull request is not deployed

- GIVEN a pull request whose head branch lives in a fork
- WHEN GitHub evaluates this workflow
- THEN the workflow SHALL NOT receive cluster credentials
- AND it SHALL NOT create or update a `hypershell-ci-pr-*` environment

### Requirement: GitHub-Brokered Keycloak Authentication

The per-pull-request environment's Keycloak SHALL broker interactive
authentication to GitHub, and SHALL NOT broker to Red Hat SSO, so an allowlisted
outside contributor can log in to an already-deployed origin-repo environment.
This is the one intentional divergence from the Keycloak model in
`local-development.spec.md` and the downstream Red Hat SSO brokering described
there: the realm structure (realm `hypershell`, the `hypershell-frontend`,
`hypershell-cli`, and `hypershell-provisioner` clients, and the per-gateway
client model) SHALL otherwise match, so the rest of the platform and the e2e
suite behave identically. This spec adds a dedicated confidential client
`hypershell-e2e` for CI (see Automated E2E Authentication).

The target cluster SHALL provide one GitHub OAuth App (or GitHub App used as
the OAuth client) for these environments. The App's client id, client secret,
and a single stable callback URL SHALL come from configuration, not from the
overlay and not from a per-pull-request Route host. GitHub does not accept
wildcard redirect URIs and limits registered callback URLs, so GitHub SHALL
redirect only to that stable callback. The callback is cluster infrastructure,
in the same class as the shared Gateway: it receives GitHub's redirect and
completes the broker login against the Keycloak that the OAuth `state`
identifies (the pull-request number). GitHub SHALL NOT redirect to a per-PR
Keycloak Route host. If the client id, client secret, or stable callback URL is
unset, the workflow SHALL fail before the access comment is posted, rather than
leave an environment nobody can log into.

The Keycloak SHALL configure a GitHub identity provider using that OAuth App
and the OAuth `read:org` scope so it can read the authenticating user's
organization membership. The realm SHALL restrict which GitHub identities may
complete authentication:

- **Organization membership is the default gate.** A GitHub user who is a member
  of the `openshift-online` organization SHALL be allowed to authenticate.
- **An allowlist admits extra usernames outside the organization.** The realm
  SHALL support an allowlist of individual GitHub usernames that MAY authenticate
  even when they are not members of `openshift-online`, so an outside contributor
  can log in without being added to the organization. The allowlist is
  additive: it widens login beyond the organization gate, never narrows it, and
  it does not grant the listed user a CI deploy.
- **Everyone else is denied.** A GitHub user who is neither an `openshift-online`
  member nor on the allowlist SHALL be denied at authentication; the environment
  SHALL NOT create a HyperShell session for them.

The organization gate and the allowlist SHALL be enforced during authentication
(for example through a first-broker-login flow step or an equivalent authenticator
that checks `read:org` membership and the configured allowlist), not merely by
post-hoc role assignment, so a denied user never obtains a token. The organization
name, the allowlist, the GitHub OAuth client id and secret, and the stable
callback URL SHALL come from configuration, not code, so a different
organization, allowlist, or OAuth App does not require an overlay edit.

#### Scenario: Organization member authenticates

- GIVEN a GitHub user who is a member of `openshift-online`
- WHEN they log in to a pull-request environment through GitHub
- THEN Keycloak SHALL allow the authentication
- AND SHALL create their HyperShell session

#### Scenario: Allowlisted non-member authenticates

- GIVEN a GitHub user who is not a member of `openshift-online`
- AND that username is on the environment's allowlist
- AND an origin-repo pull request has already deployed the environment
- WHEN they log in through GitHub
- THEN Keycloak SHALL allow the authentication
- AND that allowlist entry SHALL NOT have caused CI to deploy a fork pull request

#### Scenario: Non-member, non-allowlisted user is denied

- GIVEN a GitHub user who is neither an `openshift-online` member nor allowlisted
- WHEN they attempt to log in through GitHub
- THEN Keycloak SHALL deny the authentication
- AND SHALL NOT issue a token or create a session

#### Scenario: GitHub redirects to the stable callback

- GIVEN a pull-request environment's Keycloak brokers to GitHub
- AND the target cluster provides the configured stable callback URL
- WHEN a user completes GitHub authentication for pull request 232
- THEN GitHub SHALL redirect to that stable callback URL
- AND GitHub SHALL NOT redirect to the `hypershell-ci-pr-232-keycloak` Route host
- AND the callback SHALL complete the broker login against that pull request's
  Keycloak

#### Scenario: Missing GitHub OAuth configuration fails bring-up

- GIVEN the GitHub OAuth client id, client secret, or stable callback URL is unset
- WHEN the workflow deploys the environment
- THEN the job SHALL fail before posting an access comment
- AND it SHALL NOT leave an environment that nobody can log into

#### Scenario: No Red Hat SSO brokering

- GIVEN a pull-request environment's Keycloak
- WHEN a maintainer inspects its identity providers
- THEN GitHub SHALL be the brokered identity provider
- AND Red Hat SSO SHALL NOT be configured as an identity provider

### Requirement: Admin Authorization and Developer-Tier Testing by Impersonation

Every GitHub identity that authenticates to a pull-request environment (whether
by organization membership or by allowlist) SHALL be granted the realm roles
needed to fully drive the environment: `platform:admin` and `gateway:creator`,
as `../security/rbac-enforcement.spec.md` defines those roles.
`platform:admin` grants global gateway view and delete; it does not grant
gateway create. `gateway:creator` grants gateway create. The realm SHALL assign
both roles to brokered GitHub users on first broker login, so no manual role
assignment is required after login.

To let an admin verify the developer permission boundary with the same GitHub
login, the environment SHALL support developer-tier testing by impersonation
rather than by a second interactive account. Keycloak federates one GitHub
identity to exactly one Keycloak user, so a GitHub user cannot "pick" between an
admin and a developer account; instead the environment SHALL seed a
developer-tier principal -- a Keycloak user that holds `gateway:viewer` (mapped
to `openshell-user` on a reachable gateway, per `e2e-testing.spec.md` area 9)
and holds neither `platform:admin` nor `gateway:creator` -- and SHALL enable
impersonation so an admin can obtain tokens for that principal. Impersonation
SHALL be available both interactively (an admin impersonates the developer
principal through the Keycloak account/admin console) and programmatically
through Keycloak token exchange, so the e2e suite can acquire developer-scoped
tokens without an interactive login (see Automated E2E Authentication).

#### Scenario: Authenticated GitHub user can fully drive the environment

- GIVEN a GitHub user authenticates to a pull-request environment
- WHEN their HyperShell session is created
- THEN they SHALL hold `platform:admin` and `gateway:creator`
- AND creating a gateway via the HyperShell API SHALL succeed because they hold
  `gateway:creator`
- AND viewing or deleting any gateway SHALL succeed because they hold
  `platform:admin`

#### Scenario: Admin tests the developer boundary by impersonation

- GIVEN an admin is logged in to a pull-request environment
- AND the environment has seeded a developer-tier principal with `gateway:viewer`
  / `openshell-user` and without `platform:admin` or `gateway:creator`
- WHEN the admin obtains developer-scoped tokens by impersonating that principal
- THEN operations allowed to that tier (creating a sandbox on a reachable
  gateway) SHALL succeed
- AND creating a gateway via the HyperShell API SHALL return `403 Forbidden`

### Requirement: Automated E2E Authentication

Because the pull-request environment's interactive login is GitHub-brokered and
brokered users have no resource-owner password grant, the e2e suite SHALL NOT
authenticate with a username/password grant against a brokered user. The
OpenShift driver's `acquire_oidc_token` and `acquire_gateway_token_with_role`
keep the same signatures and call sites `e2e-testing.spec.md` defines; they
SHALL become grant-agnostic. Kind and manual OpenShift runs default to the
password grant against seeded users. This workflow SHALL set `E2E_OIDC_GRANT` to
the service-account and token-exchange path so the suite still calls those
functions.

The realm SHALL include a dedicated confidential client `hypershell-e2e` whose
service account holds `platform:admin` and `gateway:creator`. The workflow SHALL
NOT reuse `hypershell-provisioner` for e2e (that client holds `manage-clients`
and `manage-users`). After `make openshift-up`, CI SHALL read the `hypershell-e2e`
client secret from the deployed Keycloak namespace (a Kubernetes Secret in
`hypershell-ci-pr-<number>-keycloak`) and SHALL NOT take it from a repo secret
that cannot match a per-PR realm. The e2e suite's admin `acquire_oidc_token`
path SHALL obtain its HyperShell API token through that client's client-
credentials grant, so the CI run authenticates without a human GitHub login.

Area 9 needs both a HyperShell API token (to 403 on gateway create) and a
per-gateway `openshell-user` token (to create a sandbox on a reachable gateway).
Both SHALL come from Keycloak token exchange impersonating the seeded developer
principal: `acquire_oidc_token` for the HyperShell API audience, and
`acquire_gateway_token_with_role` targeting that gateway's Keycloak client for
the gateway audience. Neither call SHALL use a password grant on these
environments. Area 10 SHALL acquire the platform-admin token through the
`hypershell-e2e` service account, not through a passworded `admin` user.

The `hypershell-e2e` client secret and any impersonation credential SHALL NOT
appear in logs, the pull-request comment, or public artifacts.

#### Scenario: CI acquires the admin token without a GitHub login

- GIVEN a pull-request environment whose interactive login is GitHub-brokered
- WHEN the e2e suite acquires its admin token in CI
- THEN `acquire_oidc_token` SHALL use the `hypershell-e2e` client-credentials
  grant, not a password grant
- AND the token SHALL carry `platform:admin` and `gateway:creator`
- AND CI SHALL have read that client secret from the Keycloak namespace after
  `make openshift-up`

#### Scenario: CI acquires the developer HyperShell API token by impersonation

- GIVEN the seeded developer-tier principal exists in the realm
- WHEN the e2e suite needs a developer-scoped HyperShell API token
- THEN `acquire_oidc_token` SHALL obtain it by impersonating that principal
  through token exchange
- AND the token SHALL NOT carry `platform:admin` or `gateway:creator`

#### Scenario: CI acquires the developer gateway token by impersonation

- GIVEN the seeded developer-tier principal exists in the realm
- AND a gateway with a per-gateway Keycloak client is running
- WHEN area 9 needs an `openshell-user` token for that gateway
- THEN `acquire_gateway_token_with_role` SHALL obtain it by token exchange
  targeting that gateway client for the same principal
- AND it SHALL NOT use a password grant
- AND the token SHALL carry `openshell-user` for that gateway

#### Scenario: Automation credentials do not leak

- GIVEN CI holds the `hypershell-e2e` client secret
- WHEN a reader inspects the job logs, the pull-request comment, and public artifacts
- THEN that secret does not appear in any of them

### Requirement: Legacy pr-test Deprecation

This spec's workflow SHALL be the canonical pull-request e2e path, superseding the
legacy `components/pr-test/e2e-openshell.sh` script. That script is a hardcoded
OpenShift pull-request e2e script that predates the infra-agnostic suite and does
the job this workflow now owns. Some team members still run it directly, so it
SHALL NOT be removed yet; it SHALL be marked deprecated and kept working, and it
SHALL be removed in the future once that manual usage has migrated to the shared
harness. `openshift-development.spec.md` and `e2e-testing.spec.md` now defer this
deprecation window to this spec: removal of `e2e-openshell.sh` remains the
eventual goal; this spec is the living document that times it. During the
window those specs SHALL NOT require the script or the `pr_test` component to
already be gone.

The ROKS variant `components/pr-test/e2e-openshell-roks.sh` is OUT OF SCOPE for
this spec. It targets IBM ROKS, which this pull-request ephemeral-environment
workflow does not cover, so this spec neither supersedes nor deprecates it. The
`pr_test` component and its CI wiring (`.github/component-paths.json` `pr_test`
entry, the `lint-pr-test` job, and component detection) SHALL remain in place --
both because the deprecated `e2e-openshell.sh` is retained and because the ROKS
script continues to live under the same component.

Deprecation of `e2e-openshell.sh` means it stays in place and runnable and every
authoritative reference marks it deprecated in favor of the shared harness and this
workflow. The script SHALL carry a deprecation notice at the top of the file that
names the canonical replacement (`tests/e2e/e2e-openshell.sh` with
`E2E_INFRA_DRIVER=openshift`, driven for pull requests by this workflow), and the
prose that documents it -- `CLAUDE.md`, `DEVELOPMENT.md`, and the skills that
mention `components/pr-test` (for example the deploy-cluster skills and
`skills/RECONCILE.md`) -- SHALL mark the pull-request OpenShift e2e usage
deprecated and point at the replacement, while leaving ROKS guidance unchanged.

New work SHALL NOT depend on `components/pr-test/e2e-openshell.sh`. The
pull-request e2e path, new CI jobs, and new documentation SHALL use the shared
harness and this workflow, not the legacy script. The deprecated script SHALL NOT
be extended with new test areas; area coverage grows in the shared harness
(`tests/e2e/`) so the two paths do not diverge further. During the deprecation window,
this spec does NOT require consolidating its logic into
`tests/e2e/drivers/openshift.sh`. Removal is deferred, not cancelled: once manual
usage has migrated, a later change SHALL remove `e2e-openshell.sh`, and it SHALL
remove the `pr_test` component and its CI wiring only once the ROKS variant is also
retired or rehomed (the ROKS script is the other reason the component still
exists).

#### Scenario: Legacy OpenShift script remains runnable but deprecated

- GIVEN a team member still runs `components/pr-test/e2e-openshell.sh` directly
- WHEN this spec is in effect
- THEN the script SHALL remain present and runnable
- AND it SHALL carry a deprecation notice naming the canonical replacement
- AND the `pr_test` CI wiring SHALL remain intact so the script does not rot

#### Scenario: ROKS script is untouched

- GIVEN `components/pr-test/e2e-openshell-roks.sh` targets IBM ROKS
- WHEN this spec is in effect
- THEN the ROKS script SHALL be neither deprecated nor removed by this spec
- AND its documentation and usage guidance SHALL remain unchanged

#### Scenario: Documentation marks the OpenShift pr-test script deprecated

- GIVEN `CLAUDE.md`, `DEVELOPMENT.md`, and the skills reference
  `components/pr-test/e2e-openshell.sh`
- WHEN a reader consults them
- THEN those references SHALL mark that OpenShift pull-request script deprecated
- AND SHALL point at the shared harness and this workflow as the canonical
  pull-request e2e path
- AND ROKS references to `e2e-openshell-roks.sh` SHALL remain unchanged
- AND the `pr_test` component SHALL NOT be marked deprecated while the ROKS
  script still lives there

#### Scenario: Removal is deferred, not cancelled

- GIVEN `e2e-openshell.sh` is deprecated but still in manual use
- WHEN the deprecation window is in effect
- THEN the script SHALL remain until that usage migrates to the shared harness
- AND removal SHALL remain the eventual goal, consistent with
  `openshift-development.spec.md` and `e2e-testing.spec.md`

#### Scenario: New work uses the canonical path

- GIVEN a change adds a pull-request e2e test area or CI job
- WHEN the change is made
- THEN it SHALL use `tests/e2e/` and this spec's workflow
- AND it SHALL NOT extend `components/pr-test/e2e-openshell.sh` with new coverage

## Design Decisions

| Decision | Rationale |
|----------|-----------|
| Namespace name from the pull-request number (`hypershell-ci-pr-<number>`) | A short, stable, collision-free identifier that every run for a pull request derives without external state; fits well within the DNS-label bound that keeps `-keycloak` under 63 characters. Branch names and commit SHAs are not stable for the life of one pull request |
| Same lifecycle labels as `make openshift-up`, with `pr-<number>` as the environment id | Reuses `hypershell.redhat.io/owned` and `hypershell.redhat.io/environment` so status and cleanup tooling stay one selector set; the `pr-` prefix lets the reaper ignore local environments. CI must be able to patch namespaces; failing closed beats an unlabeled environment the reaper cannot see |
| `make openshift-up` on every deploying trigger, unconditionally | The command is already idempotent and reconciling, so one code path creates on first run and reconciles on later runs; branching on "does it exist" would duplicate logic and risk drift |
| CI stamps `hypershell.redhat.io/expires-at`; `make openshift-up` does not | The timebox is a pull-request cost bound, not a local-dev contract. Stamping from the workflow after bring-up refreshes active PRs without time-boxing developer namespaces |
| Origin `pull_request` only; Kind remains the merge-queue gate | `merge_group` has no stable pull-request number the way this namespace is keyed, and would race a `synchronize` swap on the same namespace. Fork PRs must not receive cluster credentials; the allowlist is login, not deploy |
| Per-PR concurrency group | Two in-flight swaps on one namespace can leave mixed digests; cancelling or queuing the older run keeps the comment SHA honest |
| One GitHub OAuth App and one stable callback | GitHub does not allow wildcard redirect URIs and limits callback URLs, so per-PR Keycloak Routes cannot be registered as GitHub callbacks. A cluster-scoped callback, like the shared Gateway, is the identity infrastructure this workflow depends on |
| Hidden HTML comment marker | Later runs have to find "the" access comment; a stable marker avoids editing an unrelated comment or posting duplicates |
| Immutable digests over untrusted tags | The environment runs exactly the artifact CI verified; pinning by `@sha256:` means a tag that is later re-pushed cannot silently change what the environment runs. A tag is a last-resort fallback only when no digest exists, and the fallback is recorded rather than silent |
| Close releases as primary path, timebox as backstop | The merge/close event frees the environment promptly in the common case; the timebox covers the case where the event does not fire or release cannot be confirmed |
| One updated comment per pull request, carrying the completed-swap commit SHA | The pull request shows the live environment's current state instead of a growing list of stale comments; pinning the SHA whose digest swap completed prevents claiming a commit the swap did not deploy |
| GitHub brokering, not Red Hat SSO | These are developer/debug environments; GitHub identity plus an organization gate and allowlist lets an outside contributor log in to an origin-repo environment, where Red Hat SSO would tie the environment to production identity |
| Organization gate by default, allowlist for extras | Organization membership is the common case; the additive allowlist admits outside contributors to login without adding them to the organization. Enforcing both during authentication (not by post-hoc roles) means a denied user never gets a token |
| Authenticated users get `platform:admin` and `gateway:creator`; developer tier by impersonation | `platform:admin` is view and delete only; create requires `gateway:creator`. A single GitHub identity federates to one Keycloak user, so there is no admin-or-developer account picker. A seeded `gateway:viewer` / `openshell-user` principal plus impersonation lets an admin still verify the developer boundary with the same login |
| Dedicated `hypershell-e2e` client; secret read from the deployed Keycloak | Brokered GitHub users have no password grant. A per-PR realm cannot share a repo-held provisioner secret, and `hypershell-provisioner` is too privileged (`manage-clients` / `manage-users`). Token exchange onto the HyperShell API client and onto the per-gateway client covers area 9 without a password grant. `E2E_OIDC_GRANT` keeps Kind and manual OpenShift on the password grant |
| Deprecate `e2e-openshell.sh` now, remove it later; leave ROKS alone | This workflow is the canonical pull-request OpenShift e2e path, so the legacy `e2e-openshell.sh` is superseded. Team members still run it, so it is deprecated first (notice + docs pointing at the shared harness) and removed later once that usage migrates. New coverage lands only in `tests/e2e/`. The ROKS variant is out of scope; the `pr_test` component stays until both scripts are gone |
