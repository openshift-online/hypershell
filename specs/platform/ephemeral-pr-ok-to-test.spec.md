# Ephemeral Pull-Request `/ok-to-test` Specification

**Date:** 2026-09-22
**Status:** Draft
**Related:** `ephemeral-pr-environments.spec.md` (HYPERSHELL-240) -- per-PR
             OpenShift namespaces, the ephemeral deploy/test/destroy cycle,
             `/pr-extend` and `/pr-destroy`, GitHub-brokered Keycloak, and the
             access comment; this spec owns who may start that cycle and from
             which workflow definition;
             `ephemeral-ci-secrets.spec.md` -- standing cluster login in AWS
             Secrets Manager and GitHub OIDC into IAM; this spec owns which
             jobs may request that OIDC token;
             `ephemeral-test-credentials.spec.md` -- test-tier passwords ESO
             projects into CI-owned Keycloak namespaces; an approved fork
             environment receives the same projection;
             `e2e-testing.spec.md` (HYPERSHELL-18) -- Kind CI, the
             `plan-images` / `should_run` gate, Konflux image consumption, and
             the merge-queue gate;
             `openshift-development.spec.md` (HYPERSHELL-44) --
             `make openshift-up` / `make openshift-down`;
             `../standards/security/security.spec.md` -- no secrets in logs

## Purpose

Standing OpenShift pull-request credentials live in AWS Secrets Manager and
are fetched with GitHub OIDC (`ephemeral-ci-secrets.spec.md`). GitHub's
automatic withhold of Actions `secrets.*` from fork jobs does not apply to
those values. GitHub's default OIDC subject for a `pull_request` job is
`repo:<owner>/<repo>:pull_request` for every pull request against the origin
repository, including a pull request whose head lives in a fork. A
`pull_request` job that sets `id-token: write` can therefore mint a token IAM
will accept, and a fork can change the workflow YAML that `pull_request`
runs.

The desired state is that fork pull requests can run the same OpenShift
ephemeral e2e cycle origin pull requests run, and that they do so only after
a write-access collaborator comments `/ok-to-test`. Cluster login SHALL be
assumed only by jobs whose workflow definition is the origin default branch.
`/ok-to-test` is the human attestation that the pull request's current tree
may be checked out and executed (`make openshift-up`, the e2e harness) with
that login. It is not a claim that the fork's workflow files are trusted.

Kind e2e and unit tests stay ungated for forks. Merge-queue stays on Kind.
Origin pull requests do not need `/ok-to-test`.

### Scope

This spec covers:

- who may run the OpenShift pull-request deploy/test/destroy cycle,
- the `/ok-to-test` grant and revoke, including authorization and the
  durable label that caches it,
- the rule that cluster login and `id-token: write` belong only to origin
  default-branch workflow definitions,
- the prohibition on `pull_request_target` as a way to give a fork
  credentials,
- how an approved fork reuses the same namespace, ESO projection, Konflux
  image gate, and teardown `ephemeral-pr-environments.spec.md` already
  defines,
- and the IAM trust subjects that replace treating `pull_request` as a
  fork filter.

This spec does not redefine namespace naming, timeboxing, slash-command
retainment, GitHub brokering, seed/de-seed, or the standing secret
inventory. It amends the trust boundary those specs previously stated as
origin-only.

### Reserved Terms

This spec adds no new domain kinds. "Origin repository" means the GitHub
repository that holds the canonical workflow files. "Fork pull request"
means a pull request whose `head.repo.full_name` is not that origin
repository. "Trusted workflow definition" means the workflow YAML GitHub
loads from the origin default branch for that run, not YAML from the pull
request head.

## Requirements

### Requirement: Cluster Login Runs From the Origin Default Branch

Jobs that assume the CI IAM role, request a GitHub OIDC token, or set
`id-token: write` SHALL be defined by a workflow GitHub loaded from the
origin default branch. `pull_request` jobs -- origin or fork -- SHALL NOT
set `id-token: write` and SHALL NOT assume that role.

The OpenShift deploy/test/destroy cycle
(`ephemeral-pr-environments.spec.md`) SHALL start from that default-branch
workflow after the Tests stage has succeeded for the pull request's head
SHA, using the same `plan-images` / `should_run` gate
`e2e-testing.spec.md` defines. Kind e2e SHALL remain a `pull_request`
(and `merge_group`) job of `.github/workflows/e2e.yml` and SHALL NOT
receive cluster login.

The workflows SHALL NOT use `pull_request_target` to check out pull-request
code and then execute it (`make`, the e2e harness, or any other
contributor-supplied script) with cluster credentials. Checking out the
pull request SHA from a default-branch workflow after `/ok-to-test` (fork)
or after Tests success (origin) is the authorized path; the checked-out
tree remains untrusted code that the command authorizes, not trusted
workflow YAML.

IAM SHALL NOT treat a `pull_request` subject as a fork deny. After this
spec is in effect the CI role SHALL NOT trust
`repo:<owner>/<repo>:pull_request` at all. The role SHALL trust only
subjects the default-branch OpenShift workflow actually issues (a
`ref:refs/heads/<default>` subject, an `environment:<name>` subject if
that workflow uses a GitHub Environment, or the equivalent
`workflow_run` / `issue_comment` subject) together with audience
`sts.amazonaws.com`, the origin repository, and `job_workflow_ref`
pinning those default-branch workflow files.

#### Scenario: Origin pull_request job cannot assume the CI role

- GIVEN an origin pull request runs Tests / E2E
- WHEN that `pull_request` job is evaluated
- THEN it SHALL NOT set `id-token: write`
- AND it SHALL NOT assume the CI IAM role
- AND Kind e2e MAY still run in that job as `e2e-testing.spec.md` defines

#### Scenario: Default-branch workflow can assume the CI role

- GIVEN Tests has succeeded for an origin pull request with `should_run=true`
- WHEN the origin default-branch OpenShift workflow starts for that SHA
- THEN it SHALL set `id-token: write`
- AND it SHALL assume the CI IAM role
- AND it SHALL fetch cluster login as `ephemeral-ci-secrets.spec.md` defines
- AND it SHALL deploy `hypershell-ci-pr-<number>` and run Tests / E2E / OpenShift

#### Scenario: pull_request_target is not used for credentials plus untrusted code

- GIVEN a maintainer inspects the OpenShift pull-request workflows
- WHEN they look for `pull_request_target`
- THEN those workflows SHALL NOT use it to check out the pull request and
  then run contributor-supplied scripts with cluster credentials

#### Scenario: IAM does not trust pull_request subjects

- GIVEN the CI IAM role trust policy
- WHEN a job presents an OIDC token whose subject is
  `repo:<origin>:pull_request`
- THEN AWS SHALL deny `AssumeRoleWithWebIdentity`
- AND a default-branch OpenShift job whose subject matches the trusted
  ref, environment, or non-`pull_request` event SHALL be allowed

### Requirement: `/ok-to-test` Authorizes a Fork Pull Request

A fork pull request SHALL NOT receive cluster login, SHALL NOT receive a
`hypershell-ci-pr-*` environment, and SHALL NOT start the OpenShift cycle
unless it is marked `/ok-to-test` approved.

A pull request is marked `/ok-to-test` approved when its most recently
created authorized `/ok-to-test` command comment is a grant, not a
revoke. The workflow SHALL cache that decision in a durable `ok-to-test`
label on the pull request so later default-branch runs can read it
without rescanning comments. The label is a cache: every `/ok-to-test`
command run SHALL reconcile the label to the latest authorized command.
Unauthorized comments SHALL NOT count toward that decision.

`/ok-to-test` SHALL be sticky for the life of the pull request until
revoked. While the label is present, each later e2e-relevant head SHA
SHALL receive the OpenShift cycle from the default-branch workflow
without a second comment. A later commit from the fork is still
untrusted code; stickiness is the maintainer attesting the contributor
for this pull request, not a single SHA.

`/ok-to-test cancel` SHALL revoke the grant. When an authorized commenter
posts a comment whose body is (or begins with) `/ok-to-test cancel`, the
workflow SHALL remove the `ok-to-test` label. Later default-branch runs
SHALL NOT start the OpenShift cycle until a new grant. Removing the
label by hand SHALL have the same effect the next time the workflow
reconciles. An in-flight OpenShift cycle for a SHA that was approved
when it started MAY finish; a newly started cycle SHALL observe the
revoked state.

Closing or merging the pull request SHALL end the grant. The label does
not outlive the pull request.

Origin pull requests SHALL NOT require `/ok-to-test`. Commenting it on
an origin pull request SHALL NOT change whether the OpenShift cycle
runs, SHALL NOT be required to keep it running, and SHALL NOT replace
`/pr-extend`.

Both grant and revoke SHALL be authorized: the workflow SHALL honor them
only from a commenter who has write, maintain, or admin permission on
the origin repository, verified against GitHub rather than inferred from
`author_association`. A comment from a user without that permission
SHALL NOT change the label, start the OpenShift cycle, or use cluster
credentials; the workflow SHALL acknowledge the refusal rather than act
silently. The fork author SHALL NOT be able to self-approve unless they
independently have that origin permission.

The GitHub organization gate and allowlist in
`ephemeral-pr-environments.spec.md` govern interactive login to an
already-deployed environment. They SHALL NOT grant `/ok-to-test` and
SHALL NOT start a deploy.

When an authorized commenter posts `/ok-to-test` on a fork pull request
that is not yet approved, the default-branch workflow SHALL start the
OpenShift cycle for the current head SHA immediately if Tests has
already succeeded and `should_run` would be true; otherwise it SHALL
set the label and wait for the next successful Tests run. It SHALL NOT
wait for a second comment.

#### Scenario: Ungated fork gets Kind and unit only

- GIVEN a pull request whose head branch lives in a fork
- AND the pull request is not marked `/ok-to-test` approved
- WHEN Tests runs on `pull_request`
- THEN unit tests and Kind e2e SHALL run as `e2e-testing.spec.md` defines
- AND that `pull_request` job SHALL NOT set `id-token: write`
- AND the OpenShift cycle SHALL NOT start
- AND no `hypershell-ci-pr-*` environment SHALL be created

#### Scenario: Authorized /ok-to-test starts the OpenShift cycle

- GIVEN a fork pull request whose Tests stage has succeeded
- AND `plan-images` would set `should_run=true`
- AND the pull request is not yet marked approved
- WHEN a write-access collaborator comments `/ok-to-test`
- THEN the workflow SHALL add the `ok-to-test` label
- AND the default-branch OpenShift workflow SHALL deploy
  `hypershell-ci-pr-<number>` for the current head SHA
- AND it SHALL run Tests / E2E / OpenShift against that environment
- AND it SHALL use cluster login from AWS Secrets Manager via OIDC

#### Scenario: Sticky approval covers later commits

- GIVEN a fork pull request already carries the `ok-to-test` label
- WHEN a new commit is pushed and Tests succeeds with `should_run=true`
- THEN the default-branch OpenShift workflow SHALL run for the new SHA
- AND a second `/ok-to-test` comment SHALL NOT be required

#### Scenario: Revoke stops later cycles

- GIVEN a fork pull request carries the `ok-to-test` label
- WHEN a write-access collaborator comments `/ok-to-test cancel`
- THEN the workflow SHALL remove the `ok-to-test` label
- AND later default-branch runs SHALL NOT start the OpenShift cycle
- AND Kind and unit tests SHALL still run on `pull_request`

#### Scenario: Unauthorized /ok-to-test is refused

- GIVEN a commenter without write access to the origin repository
- WHEN they comment `/ok-to-test` or `/ok-to-test cancel`
- THEN the workflow SHALL NOT change the label or start the OpenShift cycle
- AND it SHALL NOT use cluster credentials
- AND it SHALL acknowledge that the command was refused rather than act
  silently

#### Scenario: Fork author cannot self-approve

- GIVEN a fork pull request opened by a user who is not a write-access
  collaborator on the origin repository
- WHEN that user comments `/ok-to-test`
- THEN the workflow SHALL refuse the command
- AND the OpenShift cycle SHALL NOT start

#### Scenario: Origin pull requests ignore /ok-to-test

- GIVEN an origin pull request
- WHEN Tests succeeds with `should_run=true`
- THEN the default-branch OpenShift workflow SHALL run without an
  `/ok-to-test` comment
- AND an `/ok-to-test` comment SHALL NOT be required
- AND it SHALL NOT change retainment (`/pr-extend`)

#### Scenario: Allowlist still does not deploy

- GIVEN a GitHub user is on the environment allowlist
- AND a fork pull request from that user is not `/ok-to-test` approved
- WHEN Tests runs
- THEN that allowlist entry SHALL NOT have caused CI to deploy the fork
  pull request

#### Scenario: Latest authorized /ok-to-test command wins

- GIVEN a write-access collaborator comments `/ok-to-test`, then
  `/ok-to-test cancel`, then `/ok-to-test` again
- WHEN the workflow settles
- THEN the approved state SHALL be taken from the most recently created
  authorized command, which is a grant
- AND the `ok-to-test` label SHALL be present
- AND the intermediate cancel SHALL NOT leave the pull request unapproved

### Requirement: Approved Forks Reuse the Origin OpenShift Cycle

Once a fork pull request is `/ok-to-test` approved, the OpenShift cycle
SHALL be the same cycle `ephemeral-pr-environments.spec.md` defines for
origin pull requests: the same `hypershell-ci-pr-<number>` namespace
group, the same `make openshift-up` / image swap / `make openshift-seed`
path, the same GitHub-brokered Keycloak and ESO-projected Secrets
(`hypershell-github-oauth`, `hypershell-e2e-test-users`), the same
client-credentials e2e grant, the same access comment, the same
ephemeral-by-default teardown, and the same timebox. `/ok-to-test` does
not invent a second environment class.

`/pr-extend` and `/pr-destroy` SHALL remain write-access collaborator
commands and SHALL work on an `/ok-to-test` approved fork pull request
the same way they work on an origin pull request, from the same
default-branch command workflow. A fork author without origin write
access SHALL NOT retain or destroy the environment. An unapproved fork
pull request SHALL NOT honor `/pr-extend`.

The CI service account, ESO projection, and shared `hysh-aws-01`
cluster SHALL be the same as for origin pull requests. `/ok-to-test` is
the attestation that this blast radius is acceptable for this pull
request. This spec does not require a weaker service account, a
separate cluster, or a withheld OAuth Secret for approved forks.

Konflux image gating SHALL be the same as origin: the cycle SHALL wait
for each changed component's on-pull-request build and SHALL swap by
digest; it SHALL NOT silently fall back to baseline images while a
Konflux build for that SHA is expected. `/ok-to-test` does not replace
Konflux's own fork-build policy. If Konflux has not produced the images
that `e2e-testing.spec.md` requires, the OpenShift job SHALL fail
closed rather than deploy a source-changing SHA on baseline images.

`merge_group` SHALL still skip the OpenShift cycle. Kind remains the
merge-queue gate.

Checkout of the pull request SHA SHALL set `persist-credentials` to
false so the untrusted tree does not receive the default-branch
`github.token`. Fetched cluster login SHALL be masked in the runner
before later steps as `ephemeral-ci-secrets.spec.md` already requires.

#### Scenario: Approved fork uses the same namespace and teardown

- GIVEN fork pull request 232 is `/ok-to-test` approved
- AND it is not marked retained
- WHEN the default-branch OpenShift workflow runs
- THEN it SHALL set `OPENSHIFT_NAMESPACE=hypershell-ci-pr-232`
- AND after the suite, including on failure or cancel, it SHALL destroy
  the environment as `ephemeral-pr-environments.spec.md` defines

#### Scenario: Approved fork Keycloak receives the same ESO Secrets

- GIVEN `make openshift-up` creates `hypershell-ci-pr-232-keycloak` for
  an approved fork
- WHEN ESO has reconciled
- THEN Secret `hypershell-github-oauth` SHALL exist in that namespace
- AND Secret `hypershell-e2e-test-users` SHALL exist in that namespace
- AND cluster-login `server` and `token` SHALL NOT appear in that
  namespace

#### Scenario: /pr-extend on an approved fork requires origin write access

- GIVEN an `/ok-to-test` approved fork pull request
- WHEN a write-access collaborator comments `/pr-extend`
- THEN the environment SHALL be retained as
  `ephemeral-pr-environments.spec.md` defines
- AND WHEN the fork author, who lacks origin write access, comments
  `/pr-extend`
- THEN the workflow SHALL refuse the command
- AND it SHALL NOT use cluster credentials for that comment

#### Scenario: Unapproved fork /pr-extend does not deploy

- GIVEN a fork pull request that is not `/ok-to-test` approved
- WHEN a write-access collaborator comments `/pr-extend`
- THEN the workflow SHALL NOT deploy a `hypershell-ci-pr-*` environment
- AND it SHALL NOT assume the CI IAM role for that comment
- AND it MAY acknowledge that `/ok-to-test` is required first

#### Scenario: Missing Konflux images fail closed

- GIVEN an approved fork pull request changed `components/api-server/`
- AND Konflux has not produced an on-pull-request image for that head SHA
- WHEN the OpenShift cycle plans images
- THEN it SHALL wait for that build as `e2e-testing.spec.md` defines
- AND it SHALL NOT swap in a baseline API server image as if the source
  had not changed
- AND if the build never appears the job SHALL fail

#### Scenario: Merge-queue still skips OpenShift

- GIVEN a pull request enters the GitHub merge queue
- WHEN CI evaluates which jobs to run
- THEN the OpenShift cycle SHALL NOT run
- AND Kind e2e SHALL remain the merge-queue gate

## Design Decisions

| Decision | Rationale |
|----------|-----------|
| Secondary spec, not only a fold-in to the origin-only trust boundary | Who may run, from which workflow ref, and what `/ok-to-test` means are a distinct trust model. The namespace cycle, secret inventory, and test-tier passwords stay in their existing specs |
| Cluster login only from origin default-branch workflows | `pull_request` runs YAML from the pull request. A fork (or a compromised origin branch) can edit that YAML to add `id-token: write`. IAM's `pull_request` subject is the same for fork and origin, so it cannot be the fork filter. Loading the OpenShift workflow from the default branch makes `job_workflow_ref` pin files attackers cannot change without merging |
| Do not use `pull_request_target` plus untrusted scripts | `pull_request_target` gives the base-ref workflow credentials and is the usual way those credentials then execute fork `make` / tests. Default-branch `issue_comment` / `workflow_run` plus `/ok-to-test` is the same credential grant without that trigger |
| `/ok-to-test` is sticky until `/ok-to-test cancel` | Matches the common Prow-style command. The maintainer attests the contributor for this pull request. Per-SHA re-approval would be safer against a later malicious push and remains a possible later tightening; the revoke command is the escape hatch |
| Same authorization as `/pr-extend` (write / maintain / admin, GitHub permission API) | One collaborator bar for every command that can spend cluster resources. `author_association` is not enough. The fork author cannot self-approve |
| Same cluster, SA, ESO projection, and namespace scheme as origin | A second cluster or a weaker SA would make fork OpenShift a different product. `/ok-to-test` is explicit acceptance of that blast radius, including OAuth and test-user Secrets in the Keycloak namespace the cycle creates |
| Kind and unit stay ungated on `pull_request` | Forks already run those jobs with no cluster login. Approval is only for the shared OpenShift cluster |
| Origin does not need `/ok-to-test` | Collaborators already push branches in the origin repository. Moving their OpenShift cycle to the default-branch workflow still removes `pull_request` OIDC; it does not add a comment gate |
| `/pr-extend` on forks only after `/ok-to-test`, still write-access only | Retainment puts a public environment on the shared cluster. The same people who can approve tests can retain; the fork author cannot |
| Fail closed if Konflux has not built the SHA | Origin already refuses to silently test baseline images for a source-changing PR. `/ok-to-test` does not lower that bar or replace Konflux's own fork-build approval if it has one |
| Drop IAM trust for `repo:<origin>:pull_request` | Once OpenShift no longer runs as a `pull_request` job, leaving that subject in the trust policy would let a future workflow regression (or a fork-edited `pull_request` workflow that sets `id-token: write`) assume the role |
| `persist-credentials: false` on the pull request checkout | The default-branch `github.token` must not be written into the untrusted worktree |
