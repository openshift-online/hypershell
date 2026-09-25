# Ephemeral CI Secrets Specification

**Date:** 2026-09-21
**Status:** Draft
**Related:** `ephemeral-pr-environments.spec.md` (HYPERSHELL-240) -- origin
             pull-request OpenShift CI, the GitHub OAuth App, the cluster login the
             workflow needs, and the origin-only trust boundary;
             `ephemeral-test-credentials.spec.md` -- test-tier Keycloak passwords in
             the same cloud store, aligned in-cluster by ESO; this spec does not
             restate that payload;
             `openshift-development.spec.md` (HYPERSHELL-44) -- `make openshift-up`
             after the workflow has a cluster context;
             `e2e-testing.spec.md` (HYPERSHELL-18) -- Kind CI and local OpenShift
             e2e, which do not use these standing cluster secrets;
             `../standards/security/security.spec.md` -- no secrets in logs

Standing credentials the OpenShift pull-request workflow needs today would live as
GitHub Actions repository or organization secrets. Those secrets do not rotate with
the rest of the fleet, sit outside the cloud operator's store, and are a second
long-lived copy of cluster and OAuth material. This spec moves that standing set into
AWS Secrets Manager on `hysh-aws-01` and removes it from GitHub Actions.

## Purpose

The origin-repository OpenShift pull-request workflow must log in to `hysh-aws-01`
and must supply the GitHub OAuth App that Keycloak brokers to. Those values are
rotated, shared with in-cluster consumers, and too privileged to keep as GitHub
Actions secrets.

The desired state is that AWS Secrets Manager is the durable store for every
standing secret that workflow needs. GitHub Actions authenticates to AWS with GitHub
OIDC (the same issuer pattern `hypershell-gitops` already uses for Renovate), fetches
only runner-only cluster login, and masks it before any later step. External
Secrets Operator copies the subset that Keycloak needs into the cluster. GitHub's
built-in `github.token` stays GitHub-issued. Per-PR Keycloak client secrets stay
in the deployed realm. Kind CI and local OpenShift e2e are unchanged.

### Scope

This spec covers:

- the inventory of standing secrets the OpenShift pull-request workflow needs,
- their AWS Secrets Manager names and JSON properties on `hysh-aws-01`,
- GitHub OIDC to IAM as the only runner authentication to AWS,
- ESO alignment for secrets the cluster itself must hold,
- rotation ownership, and
- what SHALL remain outside AWS Secrets Manager.

This spec does not define pull-request namespace naming, timeboxing, or GitHub
brokering behavior (`ephemeral-pr-environments.spec.md`). It does not define
test-tier principal passwords (`ephemeral-test-credentials.spec.md`). It does not
prescribe Terraform or workflow YAML.

### Reserved Terms

"Standing CI secret" means a long-lived credential the OpenShift pull-request
workflow or the PR-environment cluster needs across many pull requests. "Runner-only
secret" means a standing CI secret only GitHub Actions consumes. "In-cluster secret"
means a standing CI secret Keycloak or another in-cluster workload also consumes.
`github.token` is GitHub's job-scoped token, not a standing CI secret.

## Requirements

### Requirement: Standing CI Secrets Live in the Cloud Store, Not GitHub Actions

Every standing CI secret for the OpenShift pull-request workflow SHALL live in AWS
Secrets Manager on `hysh-aws-01`. The origin repository and the GitHub organization
SHALL NOT store those values as Actions secrets (`secrets.*` other than the
platform-provided `github.token`). Terraform SHALL own the secret containers and the
IAM role the workflow assumes. Secret values SHALL stay out of Git and out of
Terraform state.

A later cloud cluster SHALL keep the same split (cloud store for standing secrets,
OIDC into that store from CI) and swap only the store and the identity provider.

#### Scenario: GitHub Actions has no standing cluster or OAuth secret

- GIVEN a reader inspects the origin repository's Actions secrets
- WHEN they look for the PR-environment kubeconfig, cluster token, or GitHub OAuth
  client secret
- THEN none of those values SHALL be present as Actions secrets
- AND `github.token` MAY still be present as the platform-provided job token

#### Scenario: Rotation does not require a GitHub UI change

- GIVEN an operator rotates the GitHub OAuth client secret in AWS Secrets Manager
- WHEN ESO's refresh interval elapses
- THEN the in-cluster Secret `hypershell-github-oauth` SHALL match the new value
- AND no GitHub Actions secret update SHALL have been required
- AND the OpenShift pull-request workflow SHALL NOT have fetched that client secret

### Requirement: Secret Inventory and Ownership

On `hysh-aws-01` the standing CI secrets SHALL be:

| AWS secret name | JSON properties | Class | Consumers |
| --- | --- | --- | --- |
| `hysh-aws-01/ci/cluster-login` | `server`, `token` | Runner-only | OpenShift pull-request workflow (`oc login --server --token`) |
| `hysh-aws-01/ci/github-oauth` | `client_id`, `client_secret`, `callback_url` | In-cluster | Keycloak GitHub identity provider via ESO |

`callback_url` is not confidential; it travels with the OAuth secret so one object
holds the App's current registration. Organization name and allowlist remain
non-secret configuration and MAY live in Git.

These secrets SHALL NOT include:

- `github.token` -- GitHub issues it per job.
- The per-PR `hypershell-e2e` client secret -- CI reads it from the deployed Keycloak
  namespace after `make openshift-up`, as `ephemeral-pr-environments.spec.md`
  already requires. A repo-held copy cannot match a per-PR realm.
- Test-tier principal passwords -- `hysh-aws-01/e2e/test-users`, owned by
  `ephemeral-test-credentials.spec.md`.

#### Scenario: Cluster login is runner-only

- GIVEN secret `hysh-aws-01/ci/cluster-login`
- WHEN ESO reconciles ephemeral namespaces
- THEN it SHALL NOT copy that server URL or token into a pull-request namespace
- AND only the GitHub Actions job SHALL read it

#### Scenario: OAuth App material is shared with Keycloak

- GIVEN secret `hysh-aws-01/ci/github-oauth`
- WHEN a per-PR Keycloak is brought up
- THEN ESO SHALL deliver `client_id`, `client_secret`, and `callback_url` into that
  Keycloak namespace
- AND the GitHub identity provider SHALL use those values

### Requirement: GitHub Actions Authenticates to AWS with OIDC

The OpenShift pull-request workflow SHALL assume an IAM role with GitHub's OIDC
token (`token.actions.githubusercontent.com`, audience `sts.amazonaws.com`). Static
AWS access keys SHALL NOT be stored as Actions secrets for this purpose.

Fork pull requests SHALL NOT receive a PR environment. Tests / E2E is one
caller job so Kind still runs on forks. OpenShift cluster login SHALL refuse a
`pull_request` whose `head.repo.full_name` is not `github.repository` before
assuming the CI IAM role. Deploy OpenShift Environment and OpenShift e2e
already skip on that same check; command and destroy jobs have their own
origin gates. Withholding `id-token: write` from a second E2E caller, and any
maintainer-gated ok-to-test path that later lets forks have an environment, is
a later trust-boundary change.

Standing secrets in AWS via OIDC are not GitHub Actions `secrets.*`, so
GitHub's automatic withhold of repository secrets from fork jobs does not
apply. GitHub's default OIDC subject for a `pull_request` job is
`repo:<owner>/<repo>:pull_request` for every pull request against that
repository; it does not encode whether the head branch lives in a fork. IAM
conditions on that subject therefore SHALL NOT be treated as a fork deny.

Tests / E2E SHALL set `id-token: write` on its one caller job so origin
OpenShift cluster login can assume the CI IAM role. The reusable `e2e.yml`
SHALL NOT declare a workflow-level `permissions` block: an explicit block that
omits `id-token` sets the permission to none and strips the caller's OIDC
token, and a block that includes it would grant a mintable token to Kind.
Kind and plan-images SHALL declare job-level permissions that omit
`id-token: write`. Deploy OpenShift Environment and OpenShift e2e SHALL
declare `id-token: write` on those jobs only.

The IAM trust policy SHALL require audience `sts.amazonaws.com`, the origin
repository, `job_workflow_ref` pinning this workflow file, and a `pull_request`
subject (or equivalent origin ref). If the job later uses a GitHub Environment,
the subject claim becomes `repo:<owner>/<repo>:environment:<name>` instead of
`repo:<owner>/<repo>:pull_request`; the trust policy SHALL match the subject the
job actually issues.

The role SHALL be allowed only `secretsmanager:GetSecretValue` and
`secretsmanager:DescribeSecret`, and only on `hysh-aws-01/ci/cluster-login`. It
SHALL NOT read `hysh-aws-01/ci/github-oauth`; ESO is that secret's only
cloud-store client, and bring-up SHALL wait on in-cluster Secret
`hypershell-github-oauth` rather than fetch the App secret onto the runner. It
SHALL NOT read `hysh-aws-01/e2e/test-users`; those passwords stay
`ephemeral-test-credentials.spec.md`. ESO is the only cloud-store client for
that secret. Password-grant against a CI-owned PR environment reads the
in-cluster ESO Secret after cluster login. The role SHALL NOT put, delete, or
list other HyperShell secrets.

After fetch, the workflow SHALL register each secret value as a masked secret in
the CI runner immediately, before any later step can echo it. Masking in the
runner is not a GitHub Actions repository or organization secret; those SHALL
remain absent as Standing CI Secrets Live in the Cloud Store already requires.
Fetched values SHALL NOT appear in logs, pull-request comments, or public
artifacts.

This matches the existing GitHub OIDC issuer and `AssumeRoleWithWebIdentity` pattern
already used to read AWS Secrets Manager from Actions in `hypershell-gitops`.

#### Scenario: Origin workflow can read cluster login

- GIVEN an origin-repository pull request runs the OpenShift environment workflow
- AND `github.event.pull_request.head.repo.full_name` equals `github.repository`
- WHEN the job needs cluster login
- THEN it SHALL exchange its GitHub OIDC token for the CI IAM role
- AND SHALL read `hysh-aws-01/ci/cluster-login`
- AND SHALL mask the `server` and `token` values in the runner before `oc login`
- AND it SHALL NOT read `hysh-aws-01/ci/github-oauth`

#### Scenario: Reusable e2e workflow inherits the caller's OIDC grant

- GIVEN Tests / E2E calls `.github/workflows/e2e.yml` with `id-token: write`
- WHEN Deploy OpenShift Environment runs `openshift-cluster-login`
- THEN the called workflow SHALL NOT declare a workflow-level `permissions` key
- AND that job SHALL declare `id-token: write`
- AND Kind SHALL declare job permissions that omit `id-token: write`
- AND the OpenShift job SHALL receive a GitHub OIDC token it can exchange for the
  CI IAM role

#### Scenario: CI IAM cannot read test-tier passwords

- GIVEN the OpenShift pull-request workflow's IAM role
- WHEN it attempts `GetSecretValue` on `hysh-aws-01/e2e/test-users`
- THEN AWS SHALL deny the call
- AND any password-grant step SHALL read Secret `hypershell-e2e-test-users`
  from the CI-owned Keycloak namespace after cluster login instead

#### Scenario: CI IAM cannot read the GitHub OAuth App secret

- GIVEN the OpenShift pull-request workflow's IAM role
- WHEN it attempts `GetSecretValue` on `hysh-aws-01/ci/github-oauth`
- THEN AWS SHALL deny the call
- AND Keycloak SHALL still receive that material from ESO as ESO Aligns
  In-Cluster Standing Secrets requires

#### Scenario: Fork pull request does not get a PR environment

- GIVEN a pull request whose head branch lives in a fork
- WHEN GitHub evaluates Deploy OpenShift Environment or OpenShift cluster login
- THEN that job SHALL skip, or cluster login SHALL refuse before assuming the
  CI IAM role
- AND it SHALL NOT create or update a `hypershell-ci-pr-*` environment

#### Scenario: No static AWS key in Actions

- GIVEN a reader inspects the workflow and the repository's Actions secrets
- WHEN they look for AWS access keys used to fetch these secrets
- THEN none SHALL be present
- AND the job SHALL use GitHub OIDC

### Requirement: ESO Aligns In-Cluster Standing Secrets

ESO SHALL copy in-cluster standing CI secrets from AWS Secrets Manager into the
namespaces that need them, using the same IRSA SecretStore class
`ephemeral-test-credentials.spec.md` uses for test-tier passwords. GitOps SHALL
provide a `ClusterExternalSecret` (or equivalent) that copies
`hysh-aws-01/ci/github-oauth` into Secret `hypershell-github-oauth` in every
CI-owned Keycloak namespace (`hypershell-ci-pr-*-keycloak`) with keys `client_id`,
`client_secret`, and `callback_url`. A newly created Keycloak namespace SHALL
receive that Secret without a GitOps commit per pull request. The selector SHALL
NOT include developer-owned namespaces. Keycloak's GitHub identity provider SHALL
read that Secret.

Runner-only secrets SHALL NOT be projected into ephemeral namespaces. The IAM role
ESO assumes for OAuth material SHALL read only `hysh-aws-01/ci/github-oauth`, not
`hysh-aws-01/ci/cluster-login` and not `hysh-aws-01/e2e/test-users`.

Bring-up SHALL fail before the access comment is posted if the OAuth Secret is
missing or not Ready, rather than leave an environment nobody can log into -- the
same fail-closed rule `ephemeral-pr-environments.spec.md` already states for unset
OAuth configuration.

#### Scenario: Keycloak receives OAuth material from ESO, not from Actions

- GIVEN `make openshift-up` creates `hypershell-ci-pr-232-keycloak`
- WHEN ESO has reconciled
- THEN Secret `hypershell-github-oauth` SHALL exist in that namespace
- AND Keycloak SHALL configure the GitHub identity provider from it
- AND the workflow SHALL NOT have written that Secret from an Actions secret
- AND no GitOps commit unique to pull request 232 SHALL have been required

#### Scenario: Cluster login never appears in a PR namespace

- GIVEN ESO is reconciling `hypershell-ci-pr-232`
- WHEN a reader lists Secrets in that namespace
- THEN no Secret SHALL contain the cluster-login `server` or `token`

### Requirement: Rotation Stays in AWS

AWS Secrets Manager (or an out-of-band rotator with write access to it) SHALL own
rotation of standing CI secrets. HyperShell workflows, ESO, and seed SHALL hold
read-only access. After a rotation, the next workflow run and the next ESO refresh
SHALL observe the new value without a GitHub Actions secret change and without a
Git commit.

A compromised GitHub Actions secret is not an applicable recovery step; recovery is
rotate the AWS secret and, if the IAM trust was wrong, correct the role.

#### Scenario: OAuth client secret rotation reaches Keycloak

- GIVEN the GitHub OAuth `client_secret` is rotated in
  `hysh-aws-01/ci/github-oauth`
- WHEN ESO's refresh interval elapses
- THEN the in-cluster Secret SHALL match the new value
- AND a subsequent GitHub broker login SHALL succeed with the rotated App secret

## Design Decisions

| Decision | Rationale |
|----------|-----------|
| Secondary spec, not a fold-in to test-tier credentials | Test-tier passwords, cluster kubeconfig, and the GitHub OAuth App are different credential classes with different consumers. One spec per class keeps rotation and IAM boundaries reviewable |
| AWS Secrets Manager, not GitHub Actions secrets | Actions secrets do not rotate with the fleet, are a second copy of cluster and OAuth material, and cannot be the in-cluster source Keycloak needs. The cloud operator's store already backs ESO on this cluster |
| GitHub OIDC into IAM, no static AWS keys in Actions | Same issuer already used from Actions in `hypershell-gitops`. Removing standing secrets from GitHub is wasted if the replacement is a long-lived AWS key stored as an Actions secret |
| Fork deny is an origin-repo check on OpenShift jobs, not a second E2E caller | GitHub's default `pull_request` subject does not encode fork vs origin, so IAM `sub` is not a fork filter. One Tests / E2E job keeps Kind on forks. Cluster login and deploy skip or refuse when `head.repo.full_name` is not the origin. Withholding `id-token` via a duplicate caller is later ok-to-test work |
| CI IAM reads only `hysh-aws-01/ci/cluster-login` | OAuth is ESO-only. Fetching the App secret onto the runner re-creates the Actions-secret blast radius this spec removes |
| Runner-only vs in-cluster split | The cluster login must never land in a public-internet PR namespace. The OAuth App secret must land in Keycloak. ESO for the second, OIDC fetch for the first |
| Cluster-login JSON is `server` plus `token` | One pinned shape so Terraform and the workflow cannot disagree. `oc login --server --token` is the OpenShift-native path; a kubeconfig blob is a second encoding of the same facts |
| `ClusterExternalSecret` into `hypershell-ci-pr-*-keycloak` | Keycloak namespaces are created at runtime. A cluster-scoped ESO object projects the OAuth Secret into each new companion namespace without a Git commit per pull request |
| Per-PR `hypershell-e2e` secret stays in Keycloak | A standing AWS or Actions copy cannot match a realm that is created per pull request. CI already reads it after bring-up |
| `github.token` stays GitHub-issued | It is short-lived, job-scoped, and not a standing fleet secret |
| Fail closed when OAuth material is missing | An environment nobody can log into is worse than a failed job; this is the same rule the PR-environment spec already has |
