# Ephemeral Pull-Request Environments Specification

**Date:** 2026-09-15
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
             `gateway:creator`;
             `ephemeral-test-credentials.spec.md` -- the three test-tier principals
             (`admin`, `developer`, `platform-admin`), their cloud-store passwords
             aligned by External Secrets Operator, and seeding for the environment
             lifetime;
             lifecycle; this spec owns only who may impersonate them and what each
             tier can do;
             `ephemeral-ci-secrets.spec.md` -- standing OpenShift pull-request CI
             secrets (cluster login, GitHub OAuth App) in AWS Secrets Manager,
             GitHub OIDC into that store, and ESO alignment of the in-cluster
             subset; this spec owns only the workflow that consumes them

## Purpose

HyperShell runs a standard ephemeral e2e cycle for every origin pull request
whose e2e-relevant paths changed: it deploys the full stack, runs the OpenShift
e2e suite against it, and destroys the environment in the same run, whether the
suite passed or failed. This is the default, so a pull request cannot silently
retain a shared-cluster environment -- most notably, a failed deploy or a failed
e2e run does not leave an environment behind. An origin PR that changes only
e2e-irrelevant paths SHALL NOT receive an environment: deploying would only
recreate a baseline `main` stack, which is not a distinct target for the author
or for Tests / E2E / OpenShift.

A developer who needs the environment as a live debug or development target opts
in with the `/pr-extend` slash command on the pull request. The deploy comment
always advertises `/pr-extend` so the developer knows the environment is otherwise
transient. Once a write-access collaborator issues `/pr-extend`, the pull request is
marked retained for the rest of its life: `/pr-extend` (re)deploys the environment
if none is up, every later commit redeploys and keeps it, and it lives up to the
standard inactivity timebox so the out-of-band reaper reclaims it only after the
pull request goes quiet. A write-access collaborator can free a retained
environment earlier with `/pr-destroy`, which runs the same teardown as
`make openshift-down`. Closing or merging the pull request also releases it.

When an e2e-relevant pull request opens (or a later commit is pushed), CI deploys
the full stack into a per-PR ephemeral namespace group on a shared target
OpenShift cluster, waits for Konflux to build the pull request's component
images, swaps those images into the environment, and posts a pull-request comment
telling the developer how to log in and how to `/pr-extend`. Tests / E2E / OpenShift
then runs the OpenShift e2e suite against the live namespace (the same
`plan-images` / `should_run` gate Kind uses, which also gates Deploy PR
environment, and only after Unit succeeds so a red unit job never deploys).
Unless the pull request is marked retained, CI then destroys the environment.
The per-PR namespace is deterministic from the
pull-request number, so a retained pull request reuses the same environment
across commits rather than accumulating environments.

This spec owns the automated OpenShift pull-request CI workflow. The
per-namespace deployment, reconcile, image swap-by-digest, teardown, and
OpenShift e2e driver remain defined in `openshift-development.spec.md`; the
infra-agnostic e2e suite, Kind CI (including the merge-queue gate), and Konflux
image gating remain defined in `e2e-testing.spec.md`. This spec adds the
pull-request-scoped concerns those specs leave open: the deterministic per-PR
namespace naming, the ephemeral-by-default deploy/test/destroy cycle, the
`/pr-extend` and `/pr-destroy` slash-command controls and their authorization, the
deploy-serialization rule, the origin-only trust boundary, the inactivity
timebox and external reaping of retained environments, the per-commit
pull-request comment, and a Keycloak authentication model that brokers to GitHub
(restricted to the `openshift-online` organization plus an allowlist) instead of
Red Hat SSO. GitHub brokering lets an allowlisted outside contributor log in to
an already-deployed origin-repo environment; it does not deploy fork pull
requests onto the shared cluster.

### Scope

This spec covers:

- the per-pull-request ephemeral environment naming and identity,
- the ephemeral-by-default deploy/test/destroy cycle triggered by pull-request
  open, synchronize (push), and reopen on the origin repository,
- the `/pr-extend` opt-in that marks a pull request retained, and the `/pr-destroy`
  opt-out that frees a retained environment early, including their authorization,
- the close/merge destroy path,
- the origin-only trust boundary (fork pull requests do not receive cluster
  credentials),
- the per-pull-request deploy serialization rule,
- the inactivity timebox and the out-of-band reaper for retained environments,
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
GatewayNetwork, GatewayRelease, ManagedCluster) only where a scenario
provisions one. "Environment" here means the per-pull-request namespace
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

### Requirement: Ephemeral-by-Default Deploy, Test, and Destroy

The workflow SHALL run a standard ephemeral e2e cycle for an e2e-relevant pull
request: deploy the environment for the current head commit, let Tests / E2E /
OpenShift run against it, and then destroy the environment in the same cycle,
whether the suite passed or failed, UNLESS the pull request is marked retained
(see Extend and Destroy Controls). A non-retained pull request SHALL NOT keep a
`hypershell-ci-pr-*` environment on the shared cluster after the cycle concludes;
in particular, a failed deploy or a failed e2e run SHALL NOT leave an environment
behind. Destroying the environment SHALL use the same teardown as
`make openshift-down` (namespace group, cluster-scoped RBAC, instance-managed
gateway and database namespaces, and per-namespace swaps), so a torn-down
ephemeral cycle leaves no HyperShell-owned residue.

Deploy OpenShift Environment and Tests / E2E / OpenShift SHALL run as jobs of
`.github/workflows/e2e.yml`, the Tests e2e stage invoked after Unit succeeds
(`e2e-testing.spec.md`). They SHALL share that workflow's `plan-images`
`should_run` gate, so an e2e-irrelevant pull request does not consume a
cluster namespace, and a unit failure never deploys. OpenShift SHALL
`needs:` Deploy OpenShift Environment; there SHALL be no cross-workflow poller
between deploy and the suite. `/pr-extend` and `/pr-destroy` live in
`.github/workflows/pr-environment-commands.yml` (`issue_comment`). A dedicated
destroy workflow SHALL trigger on `closed` (which covers both merge and close)
to destroy a retained environment (see the Timebox and Reaping requirement), so
Tests runs do not list a skipped Destroy check. None of these jobs SHALL run
on `merge_group`: this ephemeral-environment lifecycle (GitHub-brokered
OAuth, the access comment, `/pr-extend`/`/pr-destroy`) is pull-request-only,
and this environment does not share a namespace with a merge-queue SHA.
`e2e-testing.spec.md` separately runs Kind and OpenShift as the merge-queue
gate, the latter through the same `Deploy OpenShift Environment`/`OpenShift`
jobs this spec's pull-request path uses, but without any of this spec's
pull-request-only features.

Deploy and OpenShift SHALL run only for pull requests targeting the origin
repository. Fork pull requests SHALL NOT receive cluster credentials and SHALL
NOT get an environment (see Pull-Request Trust Boundary). The workflows SHALL
NOT use `pull_request_target`.

A deploying run is an origin-repository `pull_request` whose `plan-images` job
sets `should_run=true` using the same e2e-relevant path gate
`e2e-testing.spec.md` defines (api-server, control-plane,
web-console/gateway-management-ui Konflux paths, deploy manifests, e2e tests,
and pr-test). When `should_run` is false, or when the e2e stage does not run
because Unit failed, CI SHALL skip `Deploy OpenShift Environment`: it SHALL NOT log
in to the cluster, SHALL NOT run `make openshift-up`, and SHALL NOT post or
update the access comment.

On every deploying run, the workflow SHALL run `make openshift-up`
unconditionally, whether or not the environment already exists. Because
`make openshift-up` is idempotent and reconciling
(`openshift-development.spec.md`), one code path both creates the environment on
first run and reconciles it to the current overlay on later runs; the workflow
SHALL NOT branch on a "does the environment exist" check before deciding whether
to deploy. When the pull request is marked retained, the workflow SHALL, after a
successful `make openshift-up`, refresh the environment's timebox (see the Timebox
and Reaping requirement) so an actively worked retained pull request is
continuously renewed while a quiet one expires.
`make openshift-up` itself SHALL NOT stamp or refresh that timebox; local
environments are not time-boxed by this spec.

The teardown that ends a non-retained cycle SHALL be the last step of
Tests / E2E / OpenShift, after diagnostics, so the suite has a live target.
It SHALL run on success, failure, and cancel of that job. The only skip is
when the pull request is marked retained (`pr-environment/pr-extended`). A
teardown failure SHALL fail Tests / E2E / OpenShift. A failed or cancelled
Deploy OpenShift Environment SHALL likewise destroy an unretained environment,
because OpenShift will not start. An unretained failing run SHALL still be
destroyed, and a developer who wants to inspect a failure SHALL `/pr-extend`,
which redeploys a fresh environment.

Deploying runs for the same pull request SHALL serialize on a per-pull-request
concurrency group. A newer run SHALL cancel or queue an older in-flight run for
that pull request so two swaps cannot leave a mixed digest set. The access
comment's commit SHA SHALL be the commit whose digest swap completed, not a
cancelled run's head.

The reconcile SHALL bring the environment to the current desired state, including
pruning resources the overlay no longer declares, so a retained environment does
not drift across the many deployments a pull request accumulates. The reconcile
SHALL preserve any active per-namespace component swap the same way
`openshift-development.spec.md` specifies.

#### Scenario: Unretained pull request deploys, tests, and is destroyed

- GIVEN an e2e-relevant pull request that is not marked retained
- WHEN the workflow runs and `plan-images` sets `should_run=true`
- THEN it SHALL run `make openshift-up` with `OPENSHIFT_NAMESPACE=hypershell-ci-pr-<number>`
- AND Tests / E2E / OpenShift SHALL run the suite against the environment
- AND after the suite concludes the workflow SHALL destroy the environment using
  the same teardown as `make openshift-down`
- AND no `hypershell-ci-pr-<number>` namespace group or instance-managed
  gateway/database namespace SHALL remain on the cluster

#### Scenario: Failed cycle still tears down

- GIVEN an unretained pull request whose deploy or e2e run fails
- WHEN the cycle concludes
- THEN the workflow SHALL still destroy the environment
- AND it SHALL NOT leave a failed environment on the shared cluster
- AND a teardown failure SHALL fail Tests / E2E / OpenShift

#### Scenario: Commit pushed to a retained pull request

- GIVEN pull request 232 is marked retained and already has a running environment
- WHEN a new commit is pushed and the `synchronize` trigger fires
- THEN the workflow SHALL reuse `hypershell-ci-pr-232` and SHALL NOT create a new environment
- AND it SHALL run `make openshift-up` to reconcile the environment
- AND it SHALL wait for Konflux to build the new commit's images and swap them in
  by digest (see Image Gating and Swap)
- AND Tests / E2E / OpenShift SHALL rerun the e2e suite when `plan-images`
  sets `should_run=true`
- AND it SHALL refresh the timebox and SHALL NOT destroy the environment
- AND it SHALL update the access comment to reflect the new head commit (see
  Pull-Request Comment)

#### Scenario: Deploy runs unconditionally once gated in

- GIVEN a deploying run for a pull request (`should_run=true`)
- WHEN the workflow reaches the deploy step
- THEN it SHALL invoke `make openshift-up` regardless of whether the environment
  already exists
- AND it SHALL NOT skip deployment based on a prior-existence check

#### Scenario: E2e-irrelevant pull request skips deploy

- GIVEN an origin pull request whose three-dot diff contains only
  e2e-irrelevant paths (for example `docs/` or `components/sdk-typescript/`)
- WHEN the Tests e2e stage runs
- THEN `plan-images` SHALL set `should_run=false`
- AND the `Deploy OpenShift Environment` job SHALL be skipped
- AND the workflow SHALL NOT consume a cluster namespace
- AND Tests / E2E / OpenShift SHALL skip as `e2e-testing.spec.md` defines

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
- THEN this pull-request-only ephemeral-environment lifecycle (this spec)
  SHALL NOT run: no GitHub-brokered OAuth, no access comment, no
  `/pr-extend`/`/pr-destroy`
- AND the Kind and OpenShift e2e jobs SHALL still gate the merge queue as
  `e2e-testing.spec.md` defines, OpenShift using a distinct
  `hypershell-ci-mq-<short-sha>` namespace that never collides with a
  pull-request or push-to-main namespace

### Requirement: Extend and Destroy Controls

The pull request's retained state SHALL be authoritative from the pull request's
own command history, not from the order in which comment-triggered runs happen to
execute. A pull request is "marked retained" when its most recently created
authorized command comment is `/pr-extend`, and ephemeral when that comment is
`/pr-destroy` or when no authorized command comment exists. The workflow SHALL
reflect that derived state in a durable `pr-environment/pr-extended` label on the
pull request so a later deploying (`synchronize`) run can read the retained state
cheaply without rescanning comments; the label is a cache of the latest-command
decision, and every command run SHALL reconcile the label to match. Because the
retained state is a property of the pull request rather than of a single run, it
persists across commits, so `/pr-extend` is sticky for the life of the pull request
rather than per commit.

`/pr-extend` SHALL opt the pull request into retention. When an authorized commenter
posts a comment whose body is (or begins with) `/pr-extend`, the workflow SHALL add
the `pr-environment/pr-extended` label, and SHALL ensure the environment is deployed
for the pull request's current head commit -- running the same deploy path as a
`synchronize` run when no environment is currently up, because the ephemeral
cycle may already have destroyed it. After `/pr-extend`, every later deploying run
for that pull request SHALL keep the environment (refresh the timebox, skip the
in-run teardown) until the environment is destroyed. `/pr-extend` SHALL be
idempotent: issuing it on an already-retained pull request SHALL re-confirm the
label and redeploy if nothing is up, and SHALL NOT create a second environment.

`/pr-destroy` SHALL free a retained environment early. When an authorized commenter
posts a comment whose body is (or begins with) `/pr-destroy`, the workflow SHALL run
the same teardown as `make openshift-down` for the pull request's namespace group
and remove the `pr-environment/pr-extended` label, returning the pull request to the
ephemeral default. `/pr-destroy` on a pull request with no environment SHALL be a
no-op that still clears the label and reports success.

Both commands SHALL be authorized: the workflow SHALL honor `/pr-extend` and
`/pr-destroy` only from a commenter who has write, maintain, or admin permission on
the origin repository, verified against GitHub rather than inferred from the
comment's `author_association` alone. A comment from a user without that
permission SHALL NOT change the retained state, deploy, or tear down anything;
the workflow SHALL acknowledge the refusal rather than act silently. Because the
comment-triggered workflow runs with repository credentials, it SHALL perform the
permission check before using any cluster credential, and it SHALL act only on
pull requests targeting the origin repository (fork pull requests receive no
cluster credentials, per Pull-Request Trust Boundary).

When a pull request accumulates more than one command comment (for example
`/pr-extend`, then `/pr-destroy`, then `/pr-extend`), the latest by `created_at` SHALL
win, and only authorized command comments SHALL count toward that decision. A
command run SHALL therefore compute the retained state from the most recently
created authorized command comment and converge the environment and the label to
it, rather than assume the comment that triggered the run is the latest: comment
events can be delivered or processed out of order, and an unauthorized
`/pr-destroy` interleaved with authorized commands SHALL NOT flip the state. When
the triggering comment is not the latest authorized command, the run SHALL still
converge to the latest command's intent rather than act on its own stale body.

Closing or merging the pull request SHALL release the environment regardless of
the retained marker (see Timebox and Reaping); the label does not outlive the
pull request.

#### Scenario: Extend retains and redeploys

- GIVEN an unretained pull request whose ephemeral environment has already been destroyed
- WHEN a write-access collaborator comments `/pr-extend`
- THEN the workflow SHALL add the `pr-environment/pr-extended` label
- AND it SHALL deploy the environment for the current head commit
- AND it SHALL stamp the timebox so the reaper reclaims it only after inactivity

#### Scenario: Extend is sticky across later commits

- GIVEN a pull request already carries the `pr-environment/pr-extended` label
- WHEN a later commit is pushed
- THEN the deploying run SHALL keep the environment rather than destroy it
- AND it SHALL refresh the timebox
- AND no second `/pr-extend` SHALL be required

#### Scenario: Destroy frees the environment early

- GIVEN a retained pull request with a running environment
- WHEN a write-access collaborator comments `/pr-destroy`
- THEN the workflow SHALL tear down the namespace group the same way
  `make openshift-down` does, including instance-managed gateway and database
  namespaces
- AND it SHALL remove the `pr-environment/pr-extended` label
- AND later commits SHALL return to the ephemeral deploy/test/destroy default

#### Scenario: Latest command wins across a sequence

- GIVEN a write-access collaborator comments `/pr-extend`, then `/pr-destroy`, then
  `/pr-extend` again on the same pull request
- WHEN the workflow settles
- THEN the retained state SHALL be taken from the most recently created command
  comment, which is `/pr-extend`
- AND the pull request SHALL be marked retained with a running environment
- AND the intermediate `/pr-destroy` SHALL NOT leave the pull request ephemeral

#### Scenario: Out-of-order or stale command run converges to the latest command

- GIVEN the latest authorized command comment on a pull request is `/pr-destroy`
- WHEN a run triggered by an earlier `/pr-extend` comment executes late
- THEN it SHALL compute the retained state from the latest command (`/pr-destroy`)
- AND it SHALL NOT re-extend the pull request from its own stale trigger body

#### Scenario: Unauthorized command does not flip the state

- GIVEN the latest authorized command comment is `/pr-extend`
- WHEN a commenter without write access later comments `/pr-destroy`
- THEN that comment SHALL NOT count toward the latest-command decision
- AND the pull request SHALL remain retained
- AND the workflow SHALL acknowledge that the command was refused rather than act silently

#### Scenario: Unauthorized command is refused

- GIVEN a commenter without write access to the origin repository
- WHEN they comment `/pr-extend` or `/pr-destroy`
- THEN the workflow SHALL NOT change the retained state, deploy, or tear down
- AND it SHALL NOT use cluster credentials
- AND it SHALL acknowledge that the command was refused rather than act silently

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
time. Bring-up SHALL set `SKIP_SEED=true` so the baseline image never receives the
seed POST (a request-contract change against that stale image would 400). After
the digest swap, the workflow SHALL run `make openshift-seed` so the seed
exercises this pull request's contract. That seed SHALL reuse existing named seed
resources on a later `synchronize` reconcile, except `dev-gateway`, which it SHALL
delete and recreate rather than reuse or duplicate, as `openshift-development.spec.md`
defines. Unchanged components SHALL keep baseline registry images. The workflow
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
  before the `Deploy OpenShift Environment` check succeeds
- AND `make openshift-seed` SHALL reuse the existing `ManagedCluster` and
  `GatewayRelease` seed resources
- AND it SHALL delete and recreate the existing `dev-gateway` rather than reuse
  it or create a second Gateway named `dev-gateway`

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
Tests / E2E / OpenShift SHALL run the OpenShift e2e suite against it, exactly as
`e2e-testing.spec.md` and `openshift-development.spec.md` define: it SHALL run
`E2E_INFRA_DRIVER=openshift E2E_OIDC_GRANT=client_credentials bash tests/e2e/e2e-openshell.sh` against a KUBECONFIG
context pointed at the environment, exercising the same test areas the Kind suite
exercises. The suite SHALL run on the pull request's first e2e-relevant deployment and on
every later e2e-relevant deployment for that pull request, using the same
`plan-images` / `should_run` gate the Kind e2e job uses, so each e2e-relevant
commit is validated against a live environment the same way Kind validates it.
An origin PR that changes only e2e-irrelevant paths SHALL skip Tests / E2E /
OpenShift and SHALL skip `Deploy OpenShift Environment`, using the same
`plan-images` / `should_run` gate. On failure
the job SHALL collect the diagnostics `e2e-testing.spec.md` defines before the
environment is torn down. The environment SHALL survive the run only when the
pull request is marked retained (see Extend and Destroy Controls); otherwise the
ephemeral cycle SHALL destroy it after the suite concludes, including on
failure or cancel. A
developer who wants to inspect a failing run SHALL `/pr-extend`, which redeploys a
fresh environment for the current head commit.

The e2e suite's authentication SHALL set `E2E_OIDC_GRANT=client_credentials` and
use the non-interactive path this spec defines (see Automated E2E Authentication),
because the environment's interactive login is GitHub-brokered and brokered users
have no password grant. Deploy and the suite live as distinct jobs in the Tests
e2e stage so a deploy failure and an e2e failure surface as distinct checks;
teardown is a last step of OpenShift and fails that check if destroy fails.

#### Scenario: E2E runs on every e2e-relevant deployment

- GIVEN the environment is deployed and the pull request's images are swapped in
- AND `plan-images` set `should_run=true` (e2e-relevant paths changed)
- WHEN Tests / E2E / OpenShift starts after `Deploy OpenShift Environment` succeeds
- THEN it SHALL run the OpenShift e2e suite against the environment
- AND it SHALL run the suite again on each later e2e-relevant commit's deployment
- AND an origin PR with `should_run=false` SHALL skip this job and SHALL skip
  `Deploy OpenShift Environment`

#### Scenario: Retained environment survives a failing run

- GIVEN the e2e suite fails
- AND the pull request is marked retained
- WHEN Tests / E2E / OpenShift finishes
- THEN it SHALL collect the failure diagnostics
- AND the environment SHALL remain deployed for developer inspection

#### Scenario: Unretained failing run is torn down

- GIVEN the e2e suite fails
- AND the pull request is not marked retained
- WHEN Tests / E2E / OpenShift finishes
- THEN it SHALL collect the failure diagnostics
- AND the ephemeral cycle SHALL destroy the environment
- AND a developer SHALL `/pr-extend` to redeploy for inspection

### Requirement: Timebox and Reaping

The primary teardown path SHALL be in-band: an unretained cycle destroys its
environment right after the e2e suite concludes (see Ephemeral-by-Default), and
`/pr-destroy` or pull-request close destroys a retained environment. The out-of-band
reaper is the backstop for the cases those paths miss -- a crashed or cancelled
teardown, or a retained pull request that simply goes quiet -- so no
`hypershell-ci-pr-*` environment lingers indefinitely on the shared cluster.

A retained pull request's environment SHALL be time-boxed to a configurable
inactivity window (default three days) and SHALL be reaped out-of-band,
independently of any CI run, so an abandoned but still-extended pull request
cannot hold cluster resources. After each successful `make openshift-up` on a
retained pull request, the CI workflow SHALL stamp both namespaces in the group
with `hypershell.redhat.io/expires-at` set to an RFC 3339 timestamp that window
in the future. Because every deploying trigger refreshes the annotation, an
actively worked retained pull request is continuously renewed and never reaped
mid-flight, while a retained pull request with no activity for the window falls
past its expiry and is reclaimed.

Every deploying run -- retained or not -- SHALL stamp `hypershell.redhat.io/expires-at`
so the reaper can reclaim a leaked environment. An unretained deploying run SHALL
stamp a short backstop expiry (a single documented workflow setting, default 24
hours) rather than the inactivity window, so an ephemeral environment whose
in-run teardown did not complete is still reclaimed promptly instead of lingering.
`make openshift-up` SHALL NOT write that annotation; local environments this
command creates are not time-boxed by this spec. Both the inactivity window and
the backstop expiry SHALL be configurable through single documented workflow
settings rather than hardcoded in the workflow logic.

Reaping SHALL be performed by an out-of-band mechanism -- a scheduled reaper job
-- not by the pull-request workflow's own teardown step, so that an environment
expires even when no further CI runs for that pull request.

> **Note (deployment topology, not a behavior requirement):** the reaper
> CronJob's deployed source of truth is the central
> [`hypershell-gitops`](https://github.com/openshift-online/hypershell-gitops)
> repository, at `clusters/hysh-aws-01/apps/pr-env-reaper`, following the
> pull-based GitOps model `global-architecture.spec.md` defines. This repo's
> `deploy/e2e/reaper` manifest remains the maintained reference copy that this
> spec's requirements describe; it is not itself what runs on the cluster.
> Because the two are not wired together, a change to the reaper's behavior,
> RBAC, or schedule SHALL be made in `deploy/e2e/reaper` first and then manually
> synced into the `hypershell-gitops` copy so the deployed reaper does not drift
> from this spec.

The reaper SHALL delete a namespace group whose `hypershell.redhat.io/expires-at`
has passed, and SHALL identify HyperShell-owned pull-request environments only
when all of these match:

- `hypershell.redhat.io/owned=true`
- `hypershell.redhat.io/environment` equal to `pr-<number>`
- namespace name prefixed with `hypershell-ci-pr-`

The reaper SHALL NOT delete namespaces that fail that match, including local
`make openshift-up` environments and any other HyperShell-owned namespace whose
environment identifier is not `pr-*`. It SHALL refuse reserved names
(`default`, `kube-*`, `openshift-*`).

The reaper SHALL perform the deletion itself through the same teardown code path
`make openshift-down` uses, invoked per expired environment, rather than a
separate reimplementation of that teardown. It therefore SHALL delete the whole
namespace group (platform and `-keycloak`), the environment's cluster-scoped
RBAC, and the instance-managed gateway namespaces the control plane stamped, and
SHALL clear that environment's per-namespace swaps, exactly as
`make openshift-down` does. Sharing the one teardown path keeps the reaper and
`make openshift-down` from drifting: a change to what teardown removes takes
effect in both without a second edit.

Gateway namespaces are not in the namespace group and do not carry
`hypershell.redhat.io/owned`. Periodic GC cannot reap them after the
platform project is gone (`openshell-gateway-namespace-gc.spec.md`). Because the
reaper runs the `make openshift-down` teardown, it SHALL delete namespaces
labeled `hypershell.redhat.io/managed=true`,
`app.kubernetes.io/managed-by=hypershell-control-plane`, and
`hypershell.redhat.io/instance=<that platform namespace>` together with the
platform project. It SHALL also reap those instance-labeled namespaces when the
platform project is already absent and the instance identity is a
`hypershell-ci-pr-<number>` platform name, so a previous incomplete teardown
cannot leave `openshell-*` workloads behind. It SHALL NOT delete namespaces
labeled for a different instance, including `hyp4`, `hyp5`, and local
`make openshift-up` environments.

On pull-request `closed` (merge or close), CI SHALL destroy the environment as
the primary path by running the same teardown as `make openshift-down`, whether
or not the pull request was retained. That destroy SHALL live in a `closed`-only
workflow so open and synchronize runs do not list a skipped Destroy check. The
timebox SHALL remain the backstop for the case where the close event does not
fire or its destroy cannot be confirmed; when the destroy step cannot confirm the
destroy, the workflow SHALL report the failure so an operator can free the
environment.

#### Scenario: Retained deploying run refreshes the inactivity expiry

- GIVEN a retained pull-request environment exists
- WHEN a new commit's deploying run finishes `make openshift-up`
- THEN both namespaces SHALL have `hypershell.redhat.io/expires-at` reset to the
  inactivity window from that run
- AND `make openshift-up` SHALL NOT have written that annotation
- AND the environment SHALL NOT be reaped while the retained pull request stays active

#### Scenario: Unretained deploy stamps only a short backstop expiry

- GIVEN an unretained deploying run
- WHEN it finishes `make openshift-up`
- THEN it SHALL stamp `hypershell.redhat.io/expires-at` with the short backstop
  window, not the inactivity window
- AND the in-run teardown SHALL remain the primary path
- AND if that teardown does not complete, the reaper SHALL reclaim the
  environment once the backstop expiry passes

#### Scenario: Quiet retained environment is reaped by the timebox

- GIVEN a retained pull-request environment has had no deploying run for the
  inactivity window
- AND the pull request was neither merged, closed, nor destroyed with `/pr-destroy`
- WHEN the out-of-band reaper evaluates environments
- THEN it SHALL delete the expired environment through the `make openshift-down`
  teardown path
- AND that teardown SHALL remove the namespace group, cluster-scoped RBAC, and
  the instance's managed gateway and database namespaces
- AND it SHALL delete only namespaces matching the pull-request ownership labels
  and `pr-*` environment identifier, plus that instance's managed namespaces

#### Scenario: Leftover instance namespaces are reaped after the project is gone

- GIVEN the platform project `hypershell-ci-pr-267` is already absent
- AND gateway namespaces remain labeled
  `hypershell.redhat.io/instance=hypershell-ci-pr-267`
- WHEN the out-of-band reaper evaluates environments
- THEN it SHALL delete those leftover instance-managed namespaces via the shared
  teardown path
- AND it SHALL NOT delete namespaces labeled for `hyp4`, `hyp5`, or a local
  `make openshift-up` environment

#### Scenario: Local environments are not reaped

- GIVEN a developer ran `make openshift-up` into a namespace that is not
  `hypershell-ci-pr-*`
- WHEN the out-of-band reaper evaluates environments
- THEN it SHALL NOT delete that namespace group

#### Scenario: Close destroys the environment; timebox backstops

- GIVEN a pull-request environment exists (retained or not)
- WHEN the pull request merges or closes
- THEN the workflow SHALL remove the environment's namespace group via the
  `make openshift-down` teardown as the primary path
- AND when the close event does not fire, the timebox SHALL reclaim the environment
- AND when destroy cannot be confirmed, the workflow SHALL report the failure

#### Scenario: Idempotent re-run after reaping

- GIVEN a retained pull request's environment was reaped after its timebox
- WHEN a new commit is pushed to that still-open, still-retained pull request
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
bring-up, so the comment and the command agree. On a GitHub-brokered
environment the comment SHALL also include the Keycloak hypershell-realm admin
console URL (`https://<keycloak-route>/admin/hypershell/console/`), not the
master-realm `/admin/` path, and SHALL tell the developer to sign in with
GitHub there to impersonate `developer` or `platform-admin`.

The comment SHALL make the environment's lifetime explicit. On an unretained
pull request, including the in-progress deploying placeholder, the comment SHALL
tell the developer to comment `/pr-extend` to keep the environment active, SHALL
state that otherwise it is destroyed once e2e testing concludes, and SHALL state
that `/pr-extend` redeploys the environment if it has already been destroyed.
Once the pull request is retained, the comment SHALL instead state that the
environment is retained, that it is renewed on every commit, and that it is
reclaimed after the inactivity timebox unless destroyed with `/pr-destroy` or
the pull request is closed. The retained comment SHALL include that inactivity
expiry as an RFC 3339 timestamp labelled UTC so the developer can see when
the reaper will reclaim the environment, matching the
`hypershell.redhat.io/expires-at` value stamped on the namespace group. The
comment SHALL never imply an unretained environment will persist.

After an unretained in-run teardown or an authorized `/pr-destroy` confirms the
environment is gone, the workflow SHALL edit that same marked comment in place
to state that the environment has been destroyed and SHALL tell the developer
to comment `/pr-extend` to redeploy it. That edit SHALL NOT post a second
comment, SHALL NOT keep claiming a live environment, and SHALL NOT keep the
access-fact table. If no marked comment exists, the destroy path SHALL NOT
post one. A pull-request `closed` destroy SHALL NOT require that comment
update (the pull request is gone from the open timeline).

The workflow SHALL post the marked comment as the first step of a deploy run,
before cluster login, deploy, or e2e. When the pull request has no marked
comment yet (first deploy), that comment SHALL state that the environment is
deploying to commit `<sha>` and SHALL contain no access facts yet. This keeps
the access comment near the top of the pull request's timeline: because it is
normally the first comment the workflow ever adds, later edits do not need to
reorder it among other bots' checks and comments. Once the environment is
ready, the workflow SHALL edit that same marked comment in place with the
access facts rather than posting a second comment. On each later deployment
for the same pull request, the workflow SHALL repeat this sequence against the
one marked comment: an early edit stating the environment is updating to the
new commit and may not be fully responsive during the update, while retaining
the existing access-fact table (namespaces, console URL, API Route URL,
web-console Route URL, Keycloak hypershell-realm admin console URL, and CLI
login do not change from reconcile to reconcile), then a final edit stating it has been updated to
commit `<sha>` with refreshed login details. The `<sha>` in the final comment
SHALL be the commit whose digest swap completed, so the comment never claims a
commit the swap did not deploy.

The comment SHALL NOT contain any credential. It SHALL include an `oc login`
template using the `--web` flag (for example `oc login --server=<api-url>
--web`), so OpenShift drives the developer's browser through the same
GitHub-organization-gated OAuth flow the web console uses and handles token
issuance and refresh itself. No separate credential delivery step is needed: no
kubeconfig, token, or password SHALL appear in the comment, the job logs, or a
public artifact.

#### Scenario: Deploying placeholder posted first

- GIVEN a pull request is opened
- WHEN the deploy job starts, before cluster login or deploy
- THEN it SHALL post one pull-request comment stating the environment is
  deploying to the head commit
- AND the comment SHALL tell the developer to comment `/pr-extend` to keep it
  active, otherwise it is destroyed once e2e testing concludes
- AND the comment SHALL state that `/pr-extend` redeploys the environment if it
  has already been destroyed
- AND the comment SHALL contain the hidden marker `<!-- hypershell-pr-environment -->`
- AND the comment SHALL contain no access facts or credential

#### Scenario: Initial comment on pull-request open

- GIVEN a pull request is opened and its environment becomes ready
- WHEN the workflow finishes deploying
- THEN it SHALL edit the marked comment in place with the namespaces, console
  URL, API Route URL, web-console Route URL, and Keycloak hypershell-realm
  admin console URL, rather than posting a second comment
- AND the comment SHALL present the same login details `make openshift-up` prints
- AND the comment SHALL tell the developer to sign in with GitHub at that
  Keycloak admin console to impersonate `developer` or `platform-admin`
- AND the comment SHALL NOT link master-realm `/admin/` as the GitHub login
- AND the comment SHALL NOT contain a credential

#### Scenario: Comment updated on each new commit

- GIVEN a pull request already has an access comment that carries the marker
  and an access-fact table
- WHEN a new commit's deploy run starts
- THEN the workflow SHALL edit that marked comment to say the environment is
  updating to the new commit
- AND the comment SHALL note that the environment may not be fully responsive
  during the update
- AND the comment SHALL retain the existing access-fact table
- AND the workflow SHALL NOT replace the comment with the first-deploy
  placeholder that has no access facts
- AND WHEN that commit's digest swap completes
- THEN the workflow SHALL edit the same marked comment again to say the
  environment was updated to commit `<sha>`
- AND `<sha>` SHALL be the commit whose digest swap completed
- AND the workflow SHALL NOT post a second access comment
- AND the comment SHALL refresh the login details

#### Scenario: Credentials never leak

- GIVEN the workflow delivers access details
- WHEN a reader inspects the comment, the job logs, and public artifacts
- THEN no kubeconfig, token, or password appears in any of them
- AND the `oc login` template uses `--web` so OpenShift issues the credential
  interactively through the developer's own browser session

#### Scenario: Comment advertises /pr-extend on an ephemeral environment

- GIVEN an unretained pull request whose environment becomes ready
- WHEN the workflow edits the access comment
- THEN the comment SHALL tell the developer to comment `/pr-extend` to keep it
  active
- AND it SHALL state that otherwise the environment is destroyed once e2e
  testing concludes
- AND it SHALL state that `/pr-extend` redeploys the environment if it has
  already been destroyed

#### Scenario: Comment reflects a retained environment

- GIVEN a pull request has been `/pr-extend`ed
- WHEN the workflow next edits the access comment
- THEN the comment SHALL state the environment is retained and renewed on each commit
- AND it SHALL state it is reclaimed after the inactivity timebox unless
  destroyed with `/pr-destroy` or the pull request is closed
- AND it SHALL include the inactivity expiry as an RFC 3339 timestamp labelled UTC
- AND it SHALL NOT tell the reader the environment is about to be destroyed

#### Scenario: Comment reflects a destroyed environment

- GIVEN a pull request has a marked access comment
- AND an unretained in-run teardown or an authorized `/pr-destroy` has
  confirmed the environment is gone
- WHEN the workflow updates the access comment
- THEN it SHALL edit the marked comment in place rather than posting a second
  comment
- AND the comment SHALL state the environment has been destroyed
- AND the comment SHALL tell the developer to comment `/pr-extend` to redeploy it
- AND the comment SHALL NOT claim the environment is live
- AND the comment SHALL NOT keep the access-fact table

### Requirement: Pull-Request Trust Boundary

The workflow SHALL run only on `pull_request` events from the origin repository
(the repository that holds the workflow). Cluster login and the GitHub OAuth App
secret SHALL NOT live as GitHub Actions secrets; they SHALL live in AWS Secrets
Manager and SHALL be consumed as `ephemeral-ci-secrets.spec.md` defines (GitHub
OIDC into IAM for runner-only cluster login, only after the job confirms the head
repository is the origin; ESO for the in-cluster OAuth copy). OpenShift cluster
login and deploy SHALL refuse or skip when
`github.event.pull_request.head.repo.full_name` is not `github.repository`. The
workflow SHALL NOT use `pull_request_target`. Fork pull requests SHALL NOT receive
the cluster login, the GitHub OAuth client secret, or any other secret this
workflow needs, and SHALL NOT get an environment. Tests / E2E remains one job so
Kind still runs on forks. The GitHub organization gate
and allowlist (see GitHub-Brokered Keycloak Authentication) govern interactive
login to an already-deployed origin-repo environment; they SHALL NOT be used as
a reason to deploy untrusted pull-request trees with cluster credentials.

#### Scenario: Origin pull request is deployed

- GIVEN a pull request opened against the origin repository by a repository
  collaborator
- WHEN the workflow runs
- THEN it SHALL deploy the environment with the cluster credentials

#### Scenario: Fork pull request is not deployed

- GIVEN a pull request whose head branch lives in a fork
- WHEN GitHub evaluates Deploy OpenShift Environment
- THEN that job SHALL skip
- AND cluster login SHALL NOT assume the CI IAM role
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
and a single stable callback URL SHALL come from AWS Secrets Manager via ESO as
`ephemeral-ci-secrets.spec.md` defines (`hysh-aws-01/ci/github-oauth`), not
from a GitHub Actions secret, not from the overlay, and not from a
per-pull-request Route host. GitHub does not accept wildcard redirect URIs and
limits registered callback URLs, so GitHub SHALL redirect only to that stable
callback. The callback is cluster infrastructure, in the same class as the
shared Gateway: it receives GitHub's redirect and completes the broker login
against the Keycloak that the OAuth `state` identifies (the pull-request
number). GitHub SHALL NOT redirect to a per-PR Keycloak Route host. If the
client id, client secret, or stable callback URL is unset, the workflow SHALL
fail before the access comment is posted, rather than leave an environment
nobody can log into.

The Keycloak SHALL configure a GitHub identity provider using that OAuth App
and the OAuth `read:org` scope so HyperShell can read the authenticating user's
organization membership. Interactive login SHALL restrict which GitHub
identities may use the environment:

- **Organization membership is the default gate.** A GitHub user who is a member
  of the `openshift-online` organization SHALL be allowed to authenticate.
- **An allowlist admits extra usernames outside the organization.** The
  environment SHALL support an allowlist of individual GitHub usernames that MAY
  authenticate even when they are not members of `openshift-online`, so an
  outside contributor can log in without being added to the organization. The
  allowlist is additive: it widens login beyond the organization gate, never
  narrows it, and it does not grant the listed user a CI deploy.
- **Everyone else is denied.** A GitHub user who is neither an
  `openshift-online` member nor on the allowlist SHALL be denied a HyperShell
  session.

The organization gate and the allowlist SHALL be enforced by the web-console BFF
after the OIDC callback, using the Keycloak-stored GitHub token
(`storeToken`) to call GitHub `GET /user/memberships/orgs/{org}` (falling back
to `GET /user/orgs`) and comparing `preferred_username` against the allowlist.
This is a weaker guarantee than a Keycloak first-broker-login SPI: Keycloak may
still issue an SSO session, but the BFF SHALL NOT persist a HyperShell session
for a denied user. The console's API bearer is that session's access token, so
a denied login SHALL NOT produce a token the BFF can forward. The API server is
not separately org-gated; e2e and control-plane callers keep using their own
service-account clients. These are developer environments; the BFF check
avoids a custom Keycloak image. Kind and local SHALL leave `GITHUB_ORG_GATE`
unset when the GitHub Secret is absent.

The BFF SHALL NOT decide "does this session have a GitHub identity to gate" by
probing whether Keycloak's `GET /broker/github/token` endpoint returns 403.
That status code is ambiguous: Keycloak returns it both for a session with no
GitHub federated identity at all (seeded password users) and for a genuinely
GitHub-linked session that simply was not granted the broker `read-token` role
(for example, an existing password account later linked to GitHub through
"Handle Existing Account" in the first-broker-login flow, which does not run
the `addReadTokenRoleOnCreate` hook because it fires only when a *new* user is
created). Treating every 403 as "not linked, so admit" collapses that
distinction and lets a real GitHub identity - member or not - through the gate
whenever the role grant did not happen, which defeats the gate entirely. The
realm's "github" identity provider SHALL instead FORCE-sync a dedicated
`github-identity` realm role (via `oidc-hardcoded-role-idp-mapper`) onto every
GitHub login, and the BFF SHALL read that role from the ID token to decide
linkage. A session without `github-identity` (seeded password users) SHALL
still receive a HyperShell session with no GitHub API calls. A session that
carries `github-identity` SHALL have its org membership or allowlist status
positively confirmed against the GitHub API; any failure to do so, including a
403 or 404 from `GET /broker/github/token`, SHALL deny the login rather than
admit it.

The organization name and the allowlist SHALL come from configuration, not
code. The GitHub OAuth client id, client secret, and stable callback URL SHALL
come from the cloud store as `ephemeral-ci-secrets.spec.md` defines, so a
different organization, allowlist, or OAuth App does not require an overlay
edit or a GitHub Actions secret change.

#### Known gap: the gate applies only to the web-console login path

This BFF check governs the `hypershell-frontend` client's `/auth/callback`
route only. It does not extend to Keycloak itself, so any client that
completes the GitHub broker login directly - notably `hypershell-cli`, a
public client with a loopback redirect (`http://127.0.0.1:*`) intended for
local CLI use - obtains a session the same way any OAuth CLI tool does,
without ever calling the BFF. The realm's GitHub identity provider mappers
(`github-grant-platform-admin`, `github-grant-gateway-creator`) grant
`platform:admin` and `gateway:creator` to every GitHub login unconditionally,
so a token obtained this way carries full platform privileges regardless of
`openshift-online` membership.

This is an accepted gap, not an oversight: closing it at the Keycloak level
requires a first-broker-login authenticator (script or custom SPI) that calls
the GitHub org API before granting realm roles, which in turn requires a
custom Keycloak image. Diverging the PR-environment Keycloak image from the
production image was rejected as a bigger risk than the gap itself - these are
throwaway, per-PR developer/debug environments on a shared e2e cluster with no
production data, the cluster does not hand out kubeconfigs or credentials to
anyone outside this workflow's own service accounts, and a bad actor able to
reach this login flow has no path to the underlying Kubernetes/OpenShift
credentials for the shared cluster from it. `oc login --web` is a separate,
cluster-level OAuth flow (against the OpenShift built-in OAuth server, not
`hypershell-frontend` or `hypershell-cli`) and is not gated by this check
either, for the same reason.

#### Scenario: Organization member authenticates

- GIVEN a GitHub user who is a member of `openshift-online`
- WHEN they log in to a pull-request environment through GitHub
- THEN the web console SHALL create their HyperShell session

#### Scenario: Allowlisted non-member authenticates

- GIVEN a GitHub user who is not a member of `openshift-online`
- AND that username is on the environment's allowlist
- AND an origin-repo pull request has already deployed the environment
- WHEN they log in through GitHub
- THEN the web console SHALL create their HyperShell session
- AND that allowlist entry SHALL NOT have caused CI to deploy a fork pull request

#### Scenario: Non-member, non-allowlisted user is denied

- GIVEN a GitHub user who is neither an `openshift-online` member nor allowlisted
- WHEN they attempt to log in through GitHub
- THEN the web console SHALL deny the login
- AND SHALL NOT create a HyperShell session
- AND SHALL show an access-denied error in the console
- AND SHALL NOT forward an API bearer on later `/api/*` calls from that login

#### Scenario: GitHub redirects to the stable callback

- GIVEN a pull-request environment's Keycloak brokers to GitHub
- AND the target cluster provides the configured stable callback URL
- WHEN a user completes GitHub authentication for pull request 232
- THEN GitHub SHALL redirect to that stable callback URL
- AND GitHub SHALL NOT redirect to the `hypershell-ci-pr-232-keycloak` Route host
- AND the callback SHALL complete the broker login against that pull request's
  Keycloak

#### Scenario: Keycloak recycle keeps the console redirect URIs

`start-dev --import-realm` only loads the rendered realm into an empty data
dir. Recycle after an OAuth-secret change SHALL re-import
`hypershell-frontend` redirect URIs for the web-console Route, not the
localhost defaults from the overlay. `make openshift-up` SHALL stamp
`HYPERSHELL_CONSOLE_HOST` on the `render-realm-config` init container so that
import is durable. The workflow SHALL skip the recycle when the oauth-secret
annotation already matches.

- GIVEN `make openshift-up` has stamped the web-console Route host on
  `render-realm-config`
- WHEN CI recycles Keycloak because the GitHub OAuth secret hash changed
- THEN `--import-realm` re-imports `hypershell-frontend` redirect URIs for that
  console host
- AND Keycloak SHALL NOT reject the BFF `redirect_uri` with
  `Invalid parameter: redirect_uri`

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

### Requirement: Admin Authorization and Self-Service Role Assumption by Impersonation

Every GitHub identity that authenticates to a pull-request environment (whether
by organization membership or by allowlist) SHALL be granted the realm roles
needed to fully drive the environment: `platform:admin` and `gateway:creator`,
as `../security/rbac-enforcement.spec.md` defines those roles.
`platform:admin` grants global gateway view and delete; it does not grant
gateway create. `gateway:creator` grants gateway create. The realm SHALL assign
both roles to brokered GitHub users on first broker login, so no manual role
assignment is required after login. That is the same HyperShell API capability
as the seeded `admin` test-tier principal (`ephemeral-test-credentials.spec.md`
assigns that principal `platform:admin` and `gateway:creator`), so a GitHub
session already starts at that tier.

Interactive login to the hypershell realm, including the Keycloak admin
console at `/admin/hypershell/console/`, SHALL authenticate through the GitHub
identity provider. The hypershell-realm login page SHALL present GitHub and
SHALL NOT present a username/password form when that provider is enabled, so
a human cannot fall back to a test-tier principal password. Master-realm
`/admin/` is Keycloak's bootstrap-admin console and is not the GitHub
impersonation path. On first broker login the realm SHALL also grant the
`realm-management` client roles `view-users`, `query-users`, and
`impersonation`, so the GitHub user can open that admin console and
impersonate `developer` and `platform-admin` without a second account.
`impersonation` is Keycloak's realm-wide role, not a target-scoped
permission. The intended interactive targets are those two principals.
Narrowing the role to them would require user-level fine-grained admin
permissions; this workflow does not configure that model. FGAP v1 is
enabled only for token-exchange onto gateway and frontend clients
(`EnsureE2ETokenExchange`). These are org-gated throwaway environments
that already grant every brokered user `platform:admin` and
`gateway:creator`.

To let any authenticated GitHub user verify a lower-privilege permission boundary
with the same GitHub login, the environment SHALL support self-service role
assumption by impersonation rather than by a second interactive account. Keycloak
federates one GitHub identity to exactly one Keycloak user, so a GitHub user
cannot "pick" between tiers by logging in differently; instead the environment
SHALL seed the `developer` and `platform-admin` test-tier principals that
`ephemeral-test-credentials.spec.md` defines and seeds. `developer` holds the
realm role `hypershell-users` and neither `platform:admin` nor `gateway:creator`.
`platform-admin` holds `platform:admin` without `gateway:creator`.
`gateway:viewer` is a per-gateway DB binding (`../security/rbac-enforcement.spec.md`),
not a Keycloak realm role, so seeding SHALL NOT assign it. The path to an
`openshell-user` token on a reachable gateway is the per-gateway Keycloak client
role grant in Automated E2E Authentication.

Impersonation SHALL be enabled so **any** authenticated GitHub user, not only a
designated admin, can obtain tokens for `developer` and `platform-admin`. They
SHALL NOT need to impersonate `admin`; their own session already carries that
HyperShell API capability. Human self-service impersonation covers the HyperShell API
permission boundary (create vs view/delete). Per-gateway `openshell-user` sandbox
create is the e2e driver's `assign_gateway_client_role` path in Automated E2E
Authentication, not a human console path. Impersonation SHALL be available
interactively through the Keycloak admin console (intended for those two
principals; the `impersonation` role is realm-wide, as Admin Authorization
states) and programmatically through Keycloak token exchange, so the e2e
suite can also acquire scoped tokens without an interactive login. A GitHub
user's `hypershell-frontend` token SHALL NOT token-exchange as another
GitHub-brokered user: token-exchange impersonation is the `hypershell-e2e`
client, which requests `developer` or `platform-admin` by username.
Impersonation SHALL NOT require the impersonating user to know or supply the
target principal's password; `ephemeral-test-credentials.spec.md` owns where
that password lives (the cluster's cloud secret store, aligned in-cluster by
ESO) and it is never part of this flow.

Those principals exist after CI seed and until the environment is destroyed
(`ephemeral-test-credentials.spec.md`). E2e SHALL NOT de-seed them. `/pr-extend`
and other automation against a live environment SHALL still be able to impersonate
`developer` and `platform-admin` after e2e has finished.

#### Scenario: GitHub user logs into the Keycloak admin console

- GIVEN a pull-request environment whose Keycloak brokers to GitHub
- WHEN a GitHub user opens `https://<keycloak-route>/admin/hypershell/console/`
- THEN they SHALL authenticate with GitHub
- AND they SHALL NOT be offered a username/password form on that login page
- AND they SHALL reach the hypershell-realm admin console
- AND they SHALL be able to impersonate `developer` and `platform-admin`
- AND they SHALL NOT have used master-realm `/admin/` or the bootstrap admin
  password

#### Scenario: Authenticated GitHub user can fully drive the environment

- GIVEN a GitHub user authenticates to a pull-request environment
- WHEN their HyperShell session is created
- THEN they SHALL hold `platform:admin` and `gateway:creator`
- AND creating a gateway via the HyperShell API SHALL succeed because they hold
  `gateway:creator`
- AND viewing or deleting any gateway SHALL succeed because they hold
  `platform:admin`

#### Scenario: Any authenticated user tests the developer HyperShell API boundary by impersonation

- GIVEN any GitHub user is logged in to a pull-request environment
- AND the environment currently has the seeded `developer` test-tier principal
- AND that principal holds `hypershell-users` and neither `platform:admin` nor
  `gateway:creator`
- WHEN that user obtains developer-scoped tokens by impersonating that principal
- THEN creating a gateway via the HyperShell API SHALL return `403 Forbidden`
- AND the user SHALL NOT have needed the `developer` principal's password to do so

#### Scenario: Human self-service does not include per-gateway sandbox create

- GIVEN any GitHub user is logged in to a pull-request environment
- AND the environment currently has the seeded `developer` test-tier principal
- AND a reachable gateway exists
- AND the e2e driver has not granted that principal `openshell-user` on that
  gateway's Keycloak client
- WHEN that user impersonates `developer`
- THEN creating a gateway via the HyperShell API SHALL return `403 Forbidden`
- AND creating a sandbox on that gateway is not a human self-service path; it
  remains the e2e driver's `assign_gateway_client_role` grant in Automated E2E
  Authentication

#### Scenario: GitHub users cannot token-exchange as each other

- GIVEN GitHub user A and GitHub user B are both authenticated to a pull-request
  environment
- WHEN user A requests an impersonated token for user B through Keycloak token
  exchange
- THEN Keycloak SHALL deny the request
- AND user A SHALL still be able to impersonate `developer` and `platform-admin`
  through the admin console and through `hypershell-e2e` token exchange

#### Scenario: Any authenticated user tests the platform-admin boundary by impersonation

- GIVEN any GitHub user is logged in to a pull-request environment
- AND the environment currently has the seeded `platform-admin` test-tier
  principal
- AND that principal holds `platform:admin` without `gateway:creator`
- WHEN that user obtains platform-admin-scoped tokens by impersonating that
  principal
- THEN viewing or deleting any gateway SHALL succeed
- AND creating a gateway via the HyperShell API SHALL return `403 Forbidden`

#### Scenario: Impersonation still works after e2e on a live environment

- GIVEN a CI e2e run has finished against a still-live PR environment
- WHEN a GitHub-authenticated user requests an impersonated token for `developer`
  or `platform-admin`
- THEN Keycloak SHALL issue that token
- AND `/pr-extend` SHALL NOT need to re-seed those principals

### Requirement: Automated E2E Authentication

Because the pull-request environment's interactive login is GitHub-brokered and
brokered users have no resource-owner password grant, the e2e suite SHALL NOT
authenticate with a username/password grant against a brokered user. The
OpenShift driver's `acquire_oidc_token` and `acquire_gateway_token_with_role`
keep the same signatures and call sites `e2e-testing.spec.md` defines; they
SHALL become grant-agnostic. Kind and local OpenShift runs default to the
password grant against the static seeded users (`admin`/`admin`,
`developer`/`developer`, `platform-admin`/`platform-admin`) that
`ephemeral-test-credentials.spec.md` retains for every locally run test. This
workflow SHALL set `E2E_OIDC_GRANT` to the service-account and token-exchange path
so the suite still calls those functions.

The realm SHALL include a dedicated confidential client `hypershell-e2e` whose
service account holds `platform:admin` and `gateway:creator`. That client, its
service-account user, and the `realm-management: impersonation`
clientScopeMapping SHALL be present in the **imported** realm only when
`HYPERSHELL_E2E_CLIENT_ENABLED` is true (the `hypershell-github-oauth` Secret
in a pull-request environment). Kind, local OpenShift without that Secret, and
hub/ibm SHALL import a realm that omits that client, user, and mapping -- not a
disabled copy. A disabled leftover from an earlier import SHALL still be a
no-op: the control plane SHALL grant FGAP v1 token-exchange onto gateway and
frontend clients only when `hypershell-e2e` exists **and is enabled**. The
workflow SHALL NOT reuse `hypershell-provisioner` for e2e (that client holds
`manage-clients` and `manage-users`). After `make openshift-up`, CI SHALL read
the `hypershell-e2e`
client secret from the deployed Keycloak namespace (a Kubernetes Secret in
`hypershell-ci-pr-<number>-keycloak`) and SHALL NOT take it from AWS Secrets
Manager or a GitHub Actions secret that cannot match a per-PR realm. Standing
cluster login and OAuth App material remain `ephemeral-ci-secrets.spec.md`;
this per-PR client secret is not in that inventory. The e2e suite's admin `acquire_oidc_token`
path SHALL obtain its HyperShell API token through that client's client-
credentials grant, so the CI run authenticates without a human GitHub login.

Area 9 needs both a HyperShell API token (to 403 on gateway create) and a
per-gateway `openshell-user` token (to create a sandbox on a reachable gateway).
Both SHALL come from Keycloak token exchange impersonating the seeded developer
principal: `acquire_oidc_token` for the HyperShell API audience, and
`acquire_gateway_token_with_role` targeting that gateway's Keycloak client for
the gateway audience. Neither call SHALL use a password grant on these
environments.

Before `acquire_gateway_token_with_role` for area 9, the OpenShift e2e driver
SHALL grant the seeded `developer` principal the `openshell-user` client role on
that gateway's Keycloak client. Kind already does this as a test-setup shortcut
(`assign_gateway_client_role`); the OpenShift driver SHALL do the same. That
grant mirrors what the control-plane RoleBinding reconciler does for a
`gateway:viewer` binding. The suite SHALL NOT create a HyperShell
`gateway:viewer` RoleBinding for this: there is no user-id discovery path for a
non-owner through the API, and a Keycloak seed step cannot grant a per-gateway
DB binding. Token exchange then yields an `openshell-user` token because the
client role is already present, not because seed assigned `gateway:viewer`.

Area 10 SHALL acquire the platform-admin token through the
`hypershell-e2e` service account, not through a passworded `admin` user.

The `hypershell-e2e` client secret and any impersonation credential SHALL NOT
appear in logs, the pull-request comment, or public artifacts.

#### Scenario: CI acquires the admin token without a GitHub login

- GIVEN a pull-request environment whose interactive login is GitHub-brokered
- WHEN the e2e suite acquires its admin token in CI
- THEN `acquire_oidc_token` SHALL use the `hypershell-e2e` client-credentials
  grant, not a password grant
- AND the token SHALL carry `platform:admin` and `gateway:creator`
- AND the token's `preferred_username` SHALL be `service-account-hypershell-e2e`
  (the profile username mapper), not a hardcoded `admin` claim
- AND CI SHALL have read that client secret from the Keycloak namespace after
  `make openshift-up`

#### Scenario: Kind and hub omit the e2e identity

- GIVEN Kind, local OpenShift without `hypershell-github-oauth`, or hub/ibm
- WHEN Keycloak imports the rendered realm
- THEN the imported realm SHALL NOT contain client `hypershell-e2e`
- AND SHALL NOT contain user `service-account-hypershell-e2e`
- AND SHALL NOT contain `clientScopeMappings` for `hypershell-e2e`

#### Scenario: Present-but-disabled e2e client does not mutate production realms

- GIVEN a Keycloak realm that still has client `hypershell-e2e` with `enabled: false`
- WHEN the control plane reconciles a gateway client
- THEN `EnsureE2ETokenExchange` SHALL skip
- AND it SHALL NOT enable `admin-fine-grained-authz` on `realm-management`
- AND it SHALL NOT attach a token-exchange policy to the gateway or frontend client

#### Scenario: CI acquires the developer HyperShell API token by impersonation

- GIVEN the seeded developer-tier principal exists in the realm
- WHEN the e2e suite needs a developer-scoped HyperShell API token
- THEN `acquire_oidc_token` SHALL obtain it by impersonating that principal
  through token exchange
- AND the token SHALL NOT carry `platform:admin` or `gateway:creator`

#### Scenario: Area 9 grants openshell-user before token exchange

- GIVEN a reachable gateway with a per-gateway Keycloak client
- AND the seeded `developer` principal exists
- WHEN area 9 prepares to acquire that principal's gateway token
- THEN the OpenShift driver SHALL grant `openshell-user` on that client to
  `developer`
- AND only then SHALL `acquire_gateway_token_with_role` token-exchange onto
  that client

#### Scenario: CI acquires the developer gateway token by impersonation

- GIVEN the seeded developer-tier principal exists in the realm
- AND a gateway with a per-gateway Keycloak client is running
- WHEN area 9 needs an `openshell-user` token for that gateway
- THEN `acquire_gateway_token_with_role` SHALL obtain it by token exchange
  targeting that gateway client for the same principal
- AND it SHALL NOT use a password grant
- AND the token SHALL carry `openshell-user` for that gateway

#### Scenario: CI acquires a per-gateway admin token as the e2e service account

- GIVEN CI authenticates as the `hypershell-e2e` service account
- AND that service account created the gateway under test
- WHEN the suite calls `acquire_gateway_token_with_role` for the admin path
- THEN it SHALL obtain the token by token-exchange of the `hypershell-e2e`
  service account targeting that gateway client
- AND it SHALL NOT impersonate the seeded passworded `admin` user
- AND it SHALL NOT use a password grant
- AND the token SHALL carry `openshell-admin` for that gateway once the
  RoleBinding reconciler has assigned the owner role to the service account

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
| Skip `Deploy OpenShift Environment` when `should_run` is false | An e2e-irrelevant PR would only deploy baseline `main` images. That consumes a shared-cluster namespace without giving the author a distinct environment or the OpenShift e2e suite a distinct target. The same `plan-images` / `should_run` gate Kind uses keeps deploy and Tests / E2E / OpenShift in lockstep |
| `make openshift-up` on every deploying run, unconditionally | The command is already idempotent and reconciling, so one code path creates on first run and reconciles on later runs; branching on "does it exist" would duplicate logic and risk drift |
| Ephemeral by default (deploy, test, destroy); `/pr-extend` to retain | Keeping an environment for every PR let failed deploys and abandoned PRs silently hold shared-cluster resources. Making destroy the default, with an explicit opt-in, means a PR only holds an environment when a developer actually asked for one to debug against |
| Retained state is a `pr-environment/pr-extended` PR label, so `/pr-extend` is sticky | A label on the pull request is durable and derivable each run without external storage, so retention persists across commits (the developer extends once, not per commit) and every workflow run reads the same source of truth |
| Latest authorized command comment by `created_at` wins; the label caches it | `issue_comment` runs can execute out of order or concurrently, so `/pr-extend` then `/pr-destroy` then `/pr-extend` must not depend on which run finishes last. Deriving state from the newest authorized command and reconciling the label to it makes the outcome deterministic and ignores an interleaved unauthorized `/pr-destroy` |
| `/pr-extend` redeploys when nothing is up; failing runs still tear down | The ephemeral cycle may have already destroyed the environment by the time a developer reads the comment. Redeploying on `/pr-extend` avoids a grace-window race and keeps the default aggressive: a fresh environment for the current head is a better debug target than a half-torn-down one |
| `/pr-extend` and `/pr-destroy` require write access, checked before using credentials | The comment-triggered workflow runs with repository and cluster credentials; an arbitrary commenter must not be able to pin or delete shared-cluster environments. A GitHub permission check (not `author_association`) matches the existing origin-only trust boundary |
| Every deploy stamps `expires-at`; retained gets the inactivity window, unretained a short backstop | The reaper keys on `expires-at`. A retained PR wants a multi-day inactivity timebox; an unretained PR relies on in-run teardown but needs a short backstop so a crashed teardown is still reclaimed promptly. `make openshift-up` does not stamp either; local dev is not time-boxed |
| Seed after every image swap; reuse existing named resources, except `dev-gateway` | `SKIP_SEED` on `openshift-up` keeps the baseline image from seeing the seed POST; `make openshift-seed` after the swap exercises this PR's contract. Gateway names are not unique, so later reconciles must look up `dev-gateway` (and the other seed names) rather than POST a second copy. `dev-gateway` is the one exception: Keycloak runs on in-memory storage with no persistent volume, so a Keycloak pod restart discards its dynamically-provisioned OIDC client while the `dev-gateway` row survives untouched in PostgreSQL, and the reconciler deliberately never auto-recreates a missing client (`openshell-gateway-keycloak.spec.md`, "Existing gateway client is missing"). Reusing a `dev-gateway` that predates the current Keycloak instance would permanently strand it in status `Keycloak client is missing`, so seeding deletes and recreates it on every run instead. This is a stopgap until Keycloak has durable storage across restarts |
| This pull-request-scoped lifecycle is origin `pull_request` only; `merge_group` gets OpenShift e2e through `e2e-testing.spec.md`'s own commit-SHA-keyed namespace instead | `merge_group` has no stable pull-request number the way this namespace is keyed, and would race a `synchronize` swap on the same namespace. Fork PRs must not receive cluster credentials; the allowlist is login, not deploy |
| Per-PR concurrency group | Two in-flight swaps on one namespace can leave mixed digests; cancelling or queuing the older run keeps the comment SHA honest |
| One GitHub OAuth App and one stable callback | GitHub does not allow wildcard redirect URIs and limits callback URLs, so per-PR Keycloak Routes cannot be registered as GitHub callbacks. A cluster-scoped callback, like the shared Gateway, is the identity infrastructure this workflow depends on |
| Hidden HTML comment marker | Later runs have to find "the" access comment; a stable marker avoids editing an unrelated comment or posting duplicates |
| Immutable digests over untrusted tags | The environment runs exactly the artifact CI verified; pinning by `@sha256:` means a tag that is later re-pushed cannot silently change what the environment runs. A tag is a last-resort fallback only when no digest exists, and the fallback is recorded rather than silent |
| In-run teardown is primary; close and reaper are the other paths | The ephemeral cycle destroys its own environment as the last step of Tests / E2E / OpenShift unless retained, and close/`/pr-destroy` frees a retained one promptly. The timebox/reaper is the backstop for a crashed teardown or a quiet retained PR, so nothing lingers when an event does not fire |
| Deploy lives in the e2e stage after Unit, not a parallel PR Environment workflow | A separate workflow would deploy even when Unit fails and would need a cross-workflow poller for the suite. Putting Deploy OpenShift Environment in `e2e.yml` behind the same `should_run` gate means unit failure skips deploy, OpenShift can `needs:` deploy, and teardown can be a last step of the suite job |
| Reaper invokes the `make openshift-down` teardown rather than reimplementing it | The reaper and `make openshift-down` must remove the same things (namespace group, cluster RBAC, instance-managed gateway namespaces, swaps). Running one teardown code path per expired environment stops the two from drifting, so adding a resource to teardown does not silently leave the reaper on a stale definition. Gateway namespaces are siblings of the platform project and periodic GC dies with the controller, so this shared path is what keeps e2e leftovers off the shared cluster |
| One updated comment per pull request, carrying the completed-swap commit SHA | The pull request shows the live environment's current state instead of a growing list of stale comments; pinning the SHA whose digest swap completed prevents claiming a commit the swap did not deploy |
| GitHub brokering, not Red Hat SSO | These are developer/debug environments; GitHub identity plus an organization gate and allowlist lets an outside contributor log in to an origin-repo environment, where Red Hat SSO would tie the environment to production identity |
| Organization gate by default, allowlist for extras | Organization membership is the common case; the additive allowlist admits outside contributors to login without adding them to the organization. Enforcing both at BFF login is sufficient: the console API bearer only exists after a HyperShell session is created, so a denied user never receives one. A custom Keycloak image is not required |
| Authenticated users get `platform:admin` and `gateway:creator`; narrower tiers by self-service impersonation | `platform:admin` is view and delete only; create requires `gateway:creator`. A single GitHub identity federates to one Keycloak user, so there is no account picker between tiers. Reusing the shared `developer` and `platform-admin` test-tier principals (`ephemeral-test-credentials.spec.md`) as impersonation targets, open to any authenticated user rather than only a designated admin, lets every contributor verify a narrower HyperShell API boundary with the same login and without a password. The human path is the hypershell-realm Keycloak admin console (`/admin/hypershell/console/`), GitHub login with no password form, then Impersonate on those two principals; master-realm `/admin/` is not that path. The `impersonation` role that mapper grants is realm-wide (Keycloak has no target-scoped form of it without user-level FGAP, which this workflow does not configure). Token-exchange impersonation stays on `hypershell-e2e` and does not include other GitHub-brokered users. Per-gateway sandbox create stays an e2e driver grant. Impersonation works for the environment lifetime; e2e does not de-seed |
| Area 9 grants `openshell-user` on the gateway client before token exchange | Kind already uses `assign_gateway_client_role` because there is no user-id discovery path to create a `gateway:viewer` RoleBinding for a non-owner. The OpenShift driver does the same so a brokered PR env has a specified path to an `openshell-user` token. That grant is not a Keycloak realm-role seed |
| Dedicated `hypershell-e2e` client, imported only when enabled | Brokered GitHub users have no password grant. A per-PR realm cannot share a standing AWS or Actions provisioner secret, and `hypershell-provisioner` is too privileged (`manage-clients` / `manage-users`). Token exchange onto the HyperShell API client and onto the per-gateway client covers area 9 without a password grant. Omitting the client from Kind/local/hub imports (and gating control-plane grants on `enabled==true`) keeps the impersonation identity out of production reconcile paths. `E2E_OIDC_GRANT` keeps Kind and local OpenShift on the password grant against the static seeds |
| Standing CI secrets in AWS Secrets Manager, not GitHub Actions | Cluster login and the GitHub OAuth App must rotate with the fleet and, for OAuth, must also land in Keycloak. `ephemeral-ci-secrets.spec.md` owns that inventory, the origin-only job-level OIDC gate, GitHub OIDC fetch of cluster login, and ESO alignment of OAuth so this spec can treat them as cluster infrastructure |
| Deprecate `e2e-openshell.sh` now, remove it later; leave ROKS alone | This workflow is the canonical pull-request OpenShift e2e path, so the legacy `e2e-openshell.sh` is superseded. Team members still run it, so it is deprecated first (notice + docs pointing at the shared harness) and removed later once that usage migrates. New coverage lands only in `tests/e2e/`. The ROKS variant is out of scope; the `pr_test` component stays until both scripts are gone |
