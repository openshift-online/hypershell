# Ephemeral E2E Test Credentials Specification

**Date:** 2026-09-21
**Status:** Draft
**Related:** `ephemeral-pr-environments.spec.md` (HYPERSHELL-240) -- per-PR OpenShift
             environments, the GitHub-brokered grant path, the `hypershell-e2e`
             confidential client, and the self-service impersonation model this spec's
             test-tier principals now serve;
             `e2e-testing.spec.md` (HYPERSHELL-18) -- the infra-agnostic e2e suite,
             the driver interface contract, and the `E2E_OIDC_GRANT` grant model;
             `openshift-development.spec.md` (HYPERSHELL-44) -- the `make openshift-*`
             lifecycle and the shared `scripts/cluster/` dispatcher;
             `local-development.spec.md` -- the Kind Keycloak realm/client model and
             static local test users this spec also applies to local
             `make openshift-up`;
             `oidc-integration.spec.md` -- platform OIDC, realm clients, and the
             `make kind-up` banner;
             `../security/rbac-enforcement.spec.md` -- `platform:admin` vs
             `gateway:creator`;
             `../standards/security/security.spec.md` -- secret handling (no secrets
             in logs; short-lived, single-use credentials);
             `ephemeral-ci-secrets.spec.md` -- standing OpenShift pull-request CI
             secrets (cluster login, GitHub OAuth App) in the same cloud store;
             this spec owns only the test-tier password payload

This spec assumes External Secrets Operator (ESO) already runs on the PR-environment
cluster, in the same class of pre-provisioned dependency as the GitHub OAuth App
`ephemeral-pr-environments.spec.md` assumes. Standing CI secrets for that workflow
(cluster login, the OAuth App itself) live in the same cloud store and are owned by
`ephemeral-ci-secrets.spec.md`; this spec owns only the test-tier password payload.
On CI-owned PR environments the cluster's cloud secret store is the durable source
of truth and ESO is the generic in-cluster seam that copies those values into a
Kubernetes Secret. HyperShell seed and CI e2e talk only to that Secret.
Developer-owned environments (Kind and local `make openshift-up`) do not use that
store or that Secret; they seed static username-equals-password values.

On `hysh-aws-01` the cloud secret store is AWS Secrets Manager. A later cluster on
another cloud SHALL keep the same Kubernetes Secret contract and swap only the ESO
`SecretStore` provider.

## Purpose

The ephemeral per-PR OpenShift environments on `hysh-aws-01` are reachable on the
public internet. Their Keycloak and API Routes are not behind a VPN. A realm login
whose password equals its username (`admin`/`admin`, `developer`/`developer`,
`platform-admin`/`platform-admin`) is therefore a live credential, not a local
convenience: anyone who can hit the Route can authenticate as a test-tier principal
and drive that environment.

Today `deploy/base/keycloak/keycloak.yaml` bakes those three human realm users into
the shared realm import, each with a password equal to its username. That one base
is consumed by the local Kind cluster, developer-owned OpenShift environments, the
ephemeral per-PR OpenShift environments, and a persistent Keycloak shared by
integration, staging, and production. Developer-owned environments need those seeds
so a human can run the e2e suite locally. A CI-owned PR environment that inherits
the base publishes them on the public internet.

These seeded realm users - the test-tier principals - exist so the e2e suite can
authenticate, and so a GitHub-authenticated human on a CI-owned PR environment can
self-service impersonate a lower-privilege tier. On those PR environments no human
logs in directly as one of them: interactive access is always GitHub-brokered (see
`ephemeral-pr-environments.spec.md`); a human reaches these tiers only by
impersonating into them from an already-authenticated GitHub session. On a
developer-owned environment a human MAY password-grant as `admin`/`admin` or
`developer`/`developer` - that is how local e2e and manual OpenShift testing work.
The password value has named consumers, split by who owns the environment. Seed on a
CI-owned PR environment is the primary consumer of the durable cloud-store passwords:
it reads Secret `hypershell-e2e-test-users` so the three Keycloak accounts exist with
non-guessable passwords (needed for username-based impersonation, not for a human
password login). Password-grant e2e against a CI-owned PR environment is the
secondary consumer of that same Secret. The canonical origin pull-request workflow
is GitHub-brokered (`E2E_OIDC_GRANT=client_credentials` in
`ephemeral-pr-environments.spec.md`) and SHALL NOT read these passwords. A developer
running e2e locally - Kind or `make openshift-up` against OpenShift - authenticates
with the well-known username-equals-password seeds.

The desired state is that a CI-owned, internet-facing PR environment never presents a
guessable human login, while any locally run test - Kind or OpenShift - keeps the
same `admin`/`admin`, `developer`/`developer`, and `platform-admin`/`platform-admin`
seeds a developer already uses. The shared realm base ships no static human passwords,
so int/stage/prod and a freshly deployed CI PR environment are not born with them.
Developer-owned bring-up (`make kind-up` and local `make openshift-up`) reconciles the
static seeds itself. On CI-owned PR environments the cloud secret store holds and
rotates high-entropy passwords for the three tiers; ESO keeps a stable Kubernetes
Secret in each CI-owned Keycloak namespace (`hypershell-ci-pr-*-keycloak`), not in
the PR app namespace that runs pull-request images; seed reconciles Keycloak to the
Secret at bring-up; those Keycloak accounts remain until the environment is
destroyed (in-run teardown, `/pr-destroy`, close, or reaper). E2e SHALL NOT
de-seed them. `/pr-extend` and other automation against a live PR environment
can keep using the same principals without waiting for another e2e run. Local
e2e also does not de-seed. The password-login window on a public Route is
therefore the environment's lifetime, not only while e2e is running.
The Keycloak master-realm bootstrap admin
(`KC_BOOTSTRAP_ADMIN_USERNAME` / `KC_BOOTSTRAP_ADMIN_PASSWORD`) is Keycloak's own
console login and is explicitly out of scope.

### Scope

This spec covers:

- the removal of the static `admin`, `developer`, and `platform-admin` human realm
  users from the shared realm base so no environment ships them by default,
- the ephemeral seeding of those three realm users (the test-tier principals): on
  CI-owned PR environments, reconciling each password to the ESO-aligned Kubernetes
  Secret; on developer-owned environments (Kind and local `make openshift-up`),
  reconciling each password to the username,
- the cloud-store and ESO contract CI-owned PR environments MUST satisfy so those
  rotated passwords survive restart, stay least-privilege in IAM, and reach every
  CI-owned Keycloak namespace through one in-cluster Secret shape (see Cloud Secret
  Store, ESO Alignment, and In-Cluster Secret Contract), without landing in the PR
  app namespace,
- keeping those Keycloak accounts for the whole CI-owned PR environment lifetime
  so `/pr-extend`, impersonation, password-grant, and other automation (for
  example an agent) can use them after e2e has finished,
- static username-equals-password seeds for every locally run test, Kind or OpenShift,
  reconciled by `make kind-up` / local `make openshift-up`, with no change to local
  developer experience, unaffected by the cloud store or ESO,
- the masking of any password value that touches the OpenShift e2e CI workflow, and
- generalizing the three test-tier principals into shared impersonation targets so any
  authenticated GitHub user, not only an admin, can self-service impersonate only
  `developer` and `platform-admin` (see Self-Service Role Assumption).

This spec does not change the realm's service-account users, the `hypershell-e2e`
confidential client, or the GitHub-brokered authentication model - those remain
defined in `ephemeral-pr-environments.spec.md` and `e2e-testing.spec.md`. It adds
`de_seed_test_users` to the e2e driver contract (owned in `e2e-testing.spec.md`)
as a no-op on every driver so a test run cannot delete live environment users.
It does not govern the Keycloak master-realm bootstrap admin.
It does not prescribe Terraform resource names, IAM JSON, or ESO Helm values; GitOps
owns those artifacts. It is a behavior contract for the cloud store, the ESO seam,
and the Kubernetes Secret HyperShell consumes. An in-cluster Vault service SHALL NOT
be the source for these passwords.

### Reserved Terms

This spec adds no new domain kinds. "Test-tier principal" (also "test user") means one
of the three human realm users (`admin`, `developer`, `platform-admin`) that e2e
testing authenticates as and that a GitHub-authenticated human may impersonate; it is
not a HyperShell resource kind. "Master bootstrap admin" means Keycloak's own
master-realm administrator, which this spec leaves unchanged. "PR-environment
cluster" means the shared OpenShift cluster that hosts per-PR ephemeral environments
(`hysh-aws-01`). "CI-owned PR environment" means a pull-request namespace
`hypershell-ci-pr-<n>` whose environment identifier is `pr-<n>`, as
`ephemeral-pr-environments.spec.md` defines. "Developer-owned environment" means
Kind, or a local `make openshift-up` whose environment identifier is not `pr-*`.
"Cloud secret store" means that cluster's cloud-provider secret
service: AWS Secrets Manager on `hysh-aws-01`. "ESO" means External Secrets Operator.
The in-cluster Secret name `hypershell-e2e-test-users` is the HyperShell seam for
CI-owned PR environments; it lives in the companion Keycloak namespace, not the
PR app namespace.

## Requirements

### Requirement: Shared Realm Ships No Static Human Passwords

The shared Keycloak realm base SHALL NOT ship any human realm user whose credential
is baked into the realm import. The `admin`, `developer`, and `platform-admin` realm
users SHALL NOT be present in the realm import's `users` array, so that no environment
consuming the shared base - the persistent int/stage/prod Keycloak, a CI-owned PR
environment, Kind, or local `make openshift-up` - inherits a static human login by
default. Developer-owned bring-up SHALL put those users back (see Developer-Owned
Environments Retain Static Credentials). The persistent Keycloak reachable at its
external Route SHALL therefore expose no realm login whose password equals its
username merely because it consumed the base. This spec SHALL NOT add a replacement
human password login there; interactive and automated access to int, stage, and
prod remains whatever those environments already use. GitHub-brokered login is a
pull-request-environment concern (`ephemeral-pr-environments.spec.md`).

Removing these users SHALL NOT remove the realm's service-account users (for example
`service-account-hypershell-provisioner` and `service-account-hypershell-e2e`), which
are Keycloak client service accounts with no password and are not human logins.

#### Scenario: Persistent shared Keycloak exposes no static human login

- GIVEN the persistent Keycloak shared by int, stage, and prod is deployed from the
  shared realm base
- WHEN a maintainer inspects the realm's users at its external Route
- THEN there SHALL be no `admin`, `developer`, or `platform-admin` realm user with a
  password equal to its username
- AND no environment SHALL have created such a user merely by consuming the base
- AND this spec SHALL NOT have added a replacement human password login on that
  Keycloak

#### Scenario: Internet-facing PR environment is not born with admin/admin

- GIVEN a per-PR environment is deployed on the PR-environment cluster from the
  shared realm base
- WHEN an unauthenticated caller hits that environment's public Keycloak Route
- THEN password grant as `admin`/`admin`, `developer`/`developer`, or
  `platform-admin`/`platform-admin` SHALL fail
- AND the only interactive login on that Route SHALL be the GitHub-brokered path

#### Scenario: Service accounts are retained

- GIVEN the static human users are removed from the realm base
- WHEN the realm import is applied
- THEN the client service-account users SHALL remain present and functional
- AND the `hypershell-e2e` and `hypershell-provisioner` service accounts SHALL be
  unaffected

### Requirement: Master Bootstrap Admin Unchanged

The Keycloak master-realm bootstrap admin SHALL be out of scope for this spec and
SHALL remain as it is today. This spec SHALL NOT rotate, remove, or regenerate the
bootstrap admin credential, and the Keycloak-console access it provides SHALL be
unchanged. Where a bring-up banner reports the Keycloak console login, that line
SHALL continue to describe the master bootstrap admin, not a seeded realm test user.

#### Scenario: Bootstrap admin is not altered

- GIVEN a deployment whose Keycloak uses the master bootstrap admin for its own
  console
- WHEN this spec is in effect
- THEN the bootstrap admin credential SHALL be unchanged
- AND the Keycloak-console login SHALL still work as before

### Requirement: Cloud Secret Store Is the Durable Source of Truth

The cluster's cloud secret store SHALL be the single durable source of truth for each
test-tier principal's current password. On `hysh-aws-01` that store is AWS Secrets
Manager. The store SHALL hold, and SHALL rotate on its own cadence, a current
password for each of `admin`, `developer`, and `platform-admin`. Neither the seed
step, the e2e suite, nor ESO SHALL generate, choose, or cache a password of its own;
all SHALL treat the store's current value as authoritative. Those passwords SHALL be
high-entropy values the store (or an out-of-band rotator with write access to it)
produces; they SHALL NOT equal the username, SHALL NOT be reused from Kind, and SHALL
NOT be guessable from a public Route.

An in-cluster Vault service SHALL NOT store these passwords. Vault remains available
as a per-instance OpenShell gateway credential driver; that is a different ownership
boundary and is out of scope here.

Terraform SHALL own the AWS secret container and the IAM identity ESO assumes. Secret
*values* SHALL stay out of Git and out of Terraform state.

On `hysh-aws-01` the AWS secret name SHALL be `hysh-aws-01/e2e/test-users`. The JSON
payload SHALL use these properties:

| JSON property | Principal |
| --- | --- |
| `admin` | `admin` |
| `developer` | `developer` |
| `platform-admin` | `platform-admin` |

The secret SHALL use the AWS-managed `aws/secretsmanager` KMS key unless a later
cross-account design requires a customer-managed key.

#### Scenario: AWS Secrets Manager holds the three passwords

- GIVEN the PR-environment cluster is `hysh-aws-01`
- WHEN a reader inspects the cloud secret store
- THEN secret `hysh-aws-01/e2e/test-users` SHALL exist
- AND its current version SHALL contain properties `admin`, `developer`, and
  `platform-admin`
- AND none of those values SHALL equal the property name

#### Scenario: Vault is not the source

- GIVEN the PR-environment cluster GitOps
- WHEN a reader looks for where e2e test-tier passwords are stored
- THEN AWS Secrets Manager SHALL be that store
- AND no Vault KV path SHALL be authoritative for these passwords

### Requirement: ESO Aligns the Cloud Store into a Generic In-Cluster Secret

ESO SHALL be the only component that speaks the cloud secret store's API for these
passwords. Seed, e2e, and CI password-grant steps SHALL read the Kubernetes Secret
ESO creates; they SHALL NOT call AWS Secrets Manager, Vault, or any other backend
directly. That is what keeps the HyperShell lifecycle generic: a cluster on another
cloud swaps the ESO `SecretStore` provider and leaves the Secret name, keys, seed,
and e2e path unchanged.

On `hysh-aws-01` GitOps SHALL provide:

- One ESO ServiceAccount that assumes an IAM role through the cluster OIDC provider
  (`AssumeRoleWithWebIdentity`, audience `sts.amazonaws.com`). Static AWS access keys
  are not permitted.
- One `ClusterSecretStore` (or a namespaced `SecretStore` in a platform namespace
  that every ephemeral environment can reference) whose provider is AWS Secrets
  Manager in `us-east-1`, authenticated by that ServiceAccount. Do not set
  `provider.aws.role` or extra `audiences` on the store; the ServiceAccount
  annotation supplies the role, and a second audience breaks STS.
- One `ClusterExternalSecret` (or equivalent per-namespace `ExternalSecret` applied
  at bring-up) that copies `hysh-aws-01/e2e/test-users` into Secret
  `hypershell-e2e-test-users` in every CI-owned Keycloak namespace
  (`hypershell-ci-pr-*-keycloak`).

The `ClusterExternalSecret` SHALL select only those CI-owned Keycloak namespaces.
It SHALL NOT select the companion PR app namespace (`hypershell-ci-pr-*` without
`-keycloak`), where pull-request images run. It SHALL NOT select developer-owned
OpenShift namespaces. A newly created CI-owned Keycloak namespace SHALL receive
the Secret without a GitOps commit per pull request.

ESO SHALL refresh the Secret on a bounded interval so a cloud-store rotation lands
in-cluster without a redeploy. The IAM role SHALL be allowed only
`secretsmanager:GetSecretValue` and `secretsmanager:DescribeSecret`, and only on
`hysh-aws-01/e2e/test-users`. It SHALL NOT create, update, list, or delete AWS
secrets, and SHALL NOT read other HyperShell AWS secrets.

#### Scenario: ESO is the only cloud-store client in the e2e path

- GIVEN seed or password-grant e2e needs a test-tier password
- WHEN it obtains that password
- THEN it SHALL read Secret `hypershell-e2e-test-users`
- AND it SHALL NOT call AWS Secrets Manager or Vault

#### Scenario: A new PR Keycloak namespace gets the Secret without a new Git commit

- GIVEN `make openshift-up` creates `hypershell-ci-pr-232-keycloak`
- WHEN ESO has reconciled
- THEN Secret `hypershell-e2e-test-users` SHALL exist in that Keycloak namespace
- AND it SHALL contain keys `admin`, `developer`, and `platform-admin`

#### Scenario: PR app namespace does not contain test-tier passwords

- GIVEN ESO is reconciling `hypershell-ci-pr-232`
- WHEN a reader lists Secrets in that app namespace
- THEN Secret `hypershell-e2e-test-users` SHALL NOT be present
- AND no Secret SHALL contain the `admin`, `developer`, or `platform-admin`
  password values

#### Scenario: Another cloud keeps the same Secret contract

- GIVEN a later cluster uses a different cloud secret store
- WHEN GitOps swaps the ESO `SecretStore` provider
- THEN seed and e2e SHALL still read Secret `hypershell-e2e-test-users` with the same
  keys
- AND neither SHALL gain a provider-specific client

#### Scenario: ESO IAM cannot write or over-read

- GIVEN the ESO IAM role for these passwords
- WHEN it attempts `PutSecretValue` on `hysh-aws-01/e2e/test-users`, or
  `GetSecretValue` on a different HyperShell secret
- THEN AWS SHALL deny the call

### Requirement: In-Cluster Secret Contract

The Kubernetes Secret ESO owns SHALL be named `hypershell-e2e-test-users`. Its keys
SHALL be `admin`, `developer`, and `platform-admin`, each holding the current
password for that principal. Seed and password-grant e2e SHALL treat that shape as
the whole interface.

The Secret SHALL exist only in CI-owned Keycloak namespaces
(`hypershell-ci-pr-*-keycloak`). It SHALL NOT exist in the companion PR app
namespace (`hypershell-ci-pr-*` without `-keycloak`). Developer-owned environments
(Kind and local `make openshift-up`) SHALL NOT create this Secret and SHALL NOT
require it. No password value from it SHALL be printed in a CI bring-up banner,
lifecycle output, CI logs, or any artifact. An operator who needs the current
rotated password SHALL read it from AWS Secrets Manager (or the cluster's cloud
store), using their own cloud identity, rather than expect it in command output.

#### Scenario: Secret keys match the three principals

- GIVEN Secret `hypershell-e2e-test-users` is Ready in a CI-owned Keycloak namespace
- WHEN seed or password-grant e2e reads it
- THEN keys `admin`, `developer`, and `platform-admin` SHALL each be present
- AND each value SHALL be the current password for that principal

#### Scenario: Bring-up banner does not print the rotated password

- GIVEN CI runs `make openshift-up` for a CI-owned PR environment
- WHEN bring-up finishes
- THEN the banner SHALL NOT print any test-tier principal's rotated password
- AND the Keycloak-console line SHALL still describe the master bootstrap admin, not
  a seeded test-tier principal

### Requirement: Test-User Seeding Reconciles to the ESO Secret on CI-Owned PR Environments

On every CI-owned PR environment bring-up (`make openshift-up` with environment
identifier `pr-<n>`) and every standalone seed of such an environment
(`make openshift-seed`), the lifecycle SHALL wait until Secret
`hypershell-e2e-test-users` exists in the companion Keycloak namespace
(`hypershell-ci-pr-<n>-keycloak`), read the three passwords from it, and reconcile
each Keycloak realm user to that value: if the user does not exist it SHALL be
created with the Secret's current password, and if the user already exists its
password SHALL be reset to match. Seeding SHALL NOT use a password equal to the
username on a CI-owned PR environment. Because the cloud store - not the seed step -
controls when the value changes, two seeds run before the next rotation SHALL
converge on the same password; this is expected, not a bug.

Developer-owned OpenShift bring-up SHALL NOT wait on this Secret and SHALL NOT read
it; it SHALL seed the static username-equals-password values instead (see
Developer-Owned Environments Retain Static Credentials).

Seeding SHALL assign each test-tier principal the realm roles it needs for its e2e
tier, using an idempotent lookup-then-assign that does not fail when the role is
already assigned:

- `admin`: `hypershell-admins`, `hypershell-users`, `gateway:creator`, and
  `platform:admin`
- `developer`: `hypershell-users`
- `platform-admin`: `hypershell-users` and `platform:admin`

Seeding SHALL NOT assign `gateway:viewer` or `gateway:owner`. Those are
per-gateway DB bindings (`../security/rbac-enforcement.spec.md`), not Keycloak
realm roles. The `developer` principal's path to an `openshell-user` token on a
reachable gateway is owned by `ephemeral-pr-environments.spec.md` (the e2e
driver's per-gateway client-role grant), not by this seed step.

Seeding SHALL authenticate to the Keycloak Admin API as the out-of-scope master
bootstrap admin. On a CI-owned PR environment, a seeding failure - including a
missing or not-Ready Secret - SHALL fail bring-up before the access comment is
posted, the same fail-closed rule `ephemeral-ci-secrets.spec.md` already states
for missing OAuth material. The lifecycle SHALL NOT warn-and-continue, SHALL NOT
send an empty or default password, and SHALL NOT fall back to
username-equals-password. Developer-owned bring-up does not wait on this Secret
(see Developer-Owned Environments Retain Static Credentials).

#### Scenario: Seeding reconciles Keycloak to the Secret

- GIVEN a CI-owned PR environment is brought up or seeded
- AND Secret `hypershell-e2e-test-users` is present
- WHEN the lifecycle seeds the `admin`, `developer`, and `platform-admin` test-tier
  principals
- THEN it SHALL set each Keycloak user's password to the matching Secret key
- AND no seeded password SHALL equal its username

#### Scenario: Seeding reconciles an existing user

- GIVEN the `admin` test-tier principal already exists from an earlier seed
- WHEN the lifecycle seeds again
- THEN it SHALL reset that user's password to the Secret's current `admin` value
  rather than skip because the user exists
- AND it SHALL ensure the user still holds its required realm roles

#### Scenario: Concurrent seeds before rotation converge

- GIVEN the cloud store has not rotated the `developer` password since the last seed
- WHEN two separate OpenShift environments seed around the same time
- THEN both SHALL reconcile the `developer` Keycloak user to the same current
  password
- AND this SHALL NOT be treated as a seeding error

#### Scenario: Seeding assigns tier roles

- GIVEN a fresh OpenShift environment
- WHEN the lifecycle seeds the test-tier principals
- THEN `admin` SHALL hold `hypershell-admins`, `hypershell-users`,
  `gateway:creator`, and `platform:admin`
- AND `developer` SHALL hold `hypershell-users`
- AND `platform-admin` SHALL hold `hypershell-users` and `platform:admin`
- AND none of the three SHALL hold a `gateway:viewer` realm role

#### Scenario: Seeding failure is fail-closed on a CI-owned PR environment

- GIVEN test-user seeding fails during CI-owned PR bring-up because the ESO Secret is
  missing
- WHEN the lifecycle would otherwise continue
- THEN bring-up SHALL exit non-zero before the access comment is posted
- AND it SHALL NOT fall back to username-equals-password on that CI-owned PR
  environment
- AND it SHALL NOT leave seeded users with an empty or default password

### Requirement: Password-Grant E2E Reads the Same Secret on CI-Owned PR Environments

The password-grant path against a CI-owned PR environment SHALL fetch each test-tier
principal's current password from Secret `hypershell-e2e-test-users` in the
companion Keycloak namespace (`hypershell-ci-pr-<n>-keycloak`). It SHALL NOT default
to `admin`/`admin`, `developer`/`developer`, or `platform-admin`/`platform-admin`
on that environment when the Secret is absent; it SHALL fail with a clear error
instead. The canonical origin pull-request workflow SHALL NOT take this path; it
sets `E2E_OIDC_GRANT=client_credentials` as `ephemeral-pr-environments.spec.md`
defines. This requirement applies when a job or a human actually password-grants
against a CI-owned `pr-*` environment (for example a local run with
`E2E_OIDC_GRANT=password` after `oc login`).

The password-grant path against a developer-owned environment (Kind or local
OpenShift) SHALL use the static username-equals-password seeds and SHALL NOT require
Secret `hypershell-e2e-test-users`.

A GitHub-brokered e2e job SHALL NOT read this Secret (see Brokered Environments Seed
Users for Identity Resolution).

#### Scenario: E2E suite fetches its own password from the Secret

- GIVEN a CI-owned PR environment e2e run using the password-grant path
- WHEN the suite needs to authenticate as a test-tier principal
- THEN it SHALL read that principal's key from Secret `hypershell-e2e-test-users`
- AND it SHALL NOT call AWS Secrets Manager or Vault

#### Scenario: Missing Secret fails fast on a CI-owned PR environment

- GIVEN a CI-owned PR environment e2e run using the password-grant path
- AND Secret `hypershell-e2e-test-users` is absent
- WHEN the suite starts
- THEN it SHALL exit non-zero with an error that names the missing Secret
- AND it SHALL NOT fall back to username-equals-password

#### Scenario: Local OpenShift e2e uses static seeds

- GIVEN a developer runs the OpenShift e2e suite against a developer-owned
  environment
- WHEN the suite needs to authenticate as `admin` or `developer`
- THEN it SHALL use `admin`/`admin` and `developer`/`developer`
- AND it SHALL NOT require Secret `hypershell-e2e-test-users`

### Requirement: Test-Tier Principals Persist for Environment Lifetime

Bring-up seed creates the three Keycloak accounts for the life of that
environment. E2e SHALL NOT delete them. `/pr-extend`, impersonation, password-grant,
and other automation against a live PR environment SHALL be able to use those
principals after e2e has finished, without waiting for another deploying run or
`make openshift-seed`. The accounts SHALL remain until the namespace group is
destroyed (in-run teardown of an unretained env, `/pr-destroy`, pull-request
close, or the reaper). Destroying the namespace group removes Keycloak; that is
the only required cleanup of these accounts. E2e SHALL NOT alter, rotate, or
delete anything in AWS Secrets Manager or the ESO Secret.

The suite SHALL still call driver function `de_seed_test_users` from the cleanup
trap as `e2e-testing.spec.md` defines. Every driver SHALL no-op that step so a
test run cannot delete live environment users (Kind static seeds, local OpenShift
static seeds, and CI-owned rotated seeds alike).

#### Scenario: Accounts remain after a passing CI run

- GIVEN a CI-owned PR environment e2e run seeded the three test-tier Keycloak accounts
- WHEN the suite exits successfully
- THEN `admin`, `developer`, and `platform-admin` SHALL still exist in the realm
- AND their passwords SHALL still match Secret `hypershell-e2e-test-users`
- AND the cloud-store and ESO Secret values SHALL be unaffected

#### Scenario: Accounts remain after a failing or aborted CI run

- GIVEN a CI-owned PR environment e2e run seeded the three test-tier Keycloak accounts
- WHEN the suite exits on a test failure or is aborted early
- THEN the three Keycloak accounts SHALL still exist
- AND a later `/pr-extend` or other automation SHALL be able to use them

#### Scenario: Skipped e2e leaves accounts for the environment lifetime

- GIVEN a CI-owned PR environment bring-up has seeded the three test-tier principals
- AND that deploying run skips the e2e suite
- WHEN bring-up finishes
- THEN the three Keycloak accounts SHALL still exist with the Secret's current
  passwords
- AND they SHALL remain until the namespace group is destroyed

#### Scenario: Retained environment keeps accounts after e2e

- GIVEN a CI-owned PR environment is retained with `/pr-extend`
- AND e2e has already finished
- WHEN an agent or other automation password-grants or impersonates `developer`
- THEN that principal SHALL still exist
- AND the grant or impersonation SHALL NOT require another e2e run to re-seed

#### Scenario: Local e2e de-seeding is a no-op

- GIVEN an e2e run against Kind or a developer-owned OpenShift environment
- WHEN the suite reaches its de-seed step
- THEN de-seed SHALL do nothing
- AND the static test users SHALL remain intact so a later local run can
  keep using them

### Requirement: Developer-Owned Environments Retain Static Credentials

Every locally run test SHALL keep the static test credentials - `admin`/`admin`,
`developer`/`developer`, and `platform-admin`/`platform-admin`. That includes Kind
and a developer running e2e against OpenShift after local `make openshift-up`. Those
seeds are how a human drives the suite from a laptop; they are not limited to Kind.

Because the shared realm base no longer supplies these users, developer-owned
bring-up SHALL reconcile them itself. After Keycloak is confirmed ready and before
the summary banner, `make kind-up` and local `make openshift-up` SHALL each
idempotently create each user with its password fixed to its username, or reset it
to that value if the user already exists. This SHALL be a straight reconcile with no
change to local developer experience: the bring-up banner SHALL still print the
static test credentials, and existing local flows that authenticate as
`admin`/`admin` SHALL continue to work. Developer-owned environments SHALL NOT
require ESO, AWS Secrets Manager, or Secret `hypershell-e2e-test-users`.

A developer-owned OpenShift namespace on a shared cluster MAY still be reachable on
the public internet. That exposure is accepted for developer-owned environments so
manual OpenShift e2e keeps the same seeds as Kind. Developer-owned environments
SHALL NOT use environment identifier `pr-*` or namespace `hypershell-ci-pr-*`;
those names are CI-owned only. The guessable-login acceptance therefore cannot
apply to a CI-owned PR environment. CI-owned PR environments SHALL NOT use these
passwords.

#### Scenario: Kind reconciles its static users on bring-up

- GIVEN the shared realm base no longer ships human users
- WHEN a developer runs `make kind-up`
- THEN the cluster SHALL end with `admin`, `developer`, and `platform-admin` present,
  each with its password equal to its username
- AND the reconcile SHALL be idempotent across repeated `make kind-up` runs

#### Scenario: Local OpenShift reconciles the same static users on bring-up

- GIVEN the shared realm base no longer ships human users
- WHEN a developer runs `make openshift-up` into a developer-owned environment
- THEN that environment SHALL end with `admin`, `developer`, and `platform-admin`
  present, each with its password equal to its username
- AND the reconcile SHALL be idempotent across repeated local `make openshift-up`
  runs
- AND bring-up SHALL NOT wait on Secret `hypershell-e2e-test-users`

#### Scenario: Local developer experience is unchanged

- GIVEN a developer runs `make kind-up` or local `make openshift-up`
- WHEN bring-up finishes
- THEN the banner SHALL still show the static `admin/admin`, `developer/developer`,
  and `platform-admin/platform-admin` credentials
- AND local flows that authenticate as `admin`/`admin` SHALL continue to succeed

#### Scenario: Local OpenShift e2e still password-grants

- GIVEN a developer runs the e2e suite against a developer-owned OpenShift
  environment
- WHEN the suite authenticates
- THEN it SHALL use the password grant against the static seeds
- AND it SHALL NOT require GitHub-brokered client-credentials

### Requirement: Password-Grant Jobs Read the Secret Through the Cluster, and Mask What They Fetch

The canonical origin pull-request workflow SHALL NOT read Secret
`hypershell-e2e-test-users`; it authenticates through client-credentials and token
exchange (`ephemeral-pr-environments.spec.md`). Any job that password-grants against
a CI-owned PR environment SHALL already be logged in to the PR-environment cluster.
Cluster login itself SHALL come from AWS Secrets Manager via GitHub OIDC as
`ephemeral-ci-secrets.spec.md` defines, not from a GitHub Actions secret. That job
SHALL wait for Secret `hypershell-e2e-test-users` to become Ready in the companion
Keycloak namespace, read the three passwords from it, SHALL register each fetched
value as a masked secret in the CI runner immediately, before any later step can
echo it, and SHALL expose them to the e2e step as its expected password environment
variables. No password SHALL appear unmasked in the workflow logs, the run output,
or any published artifact. The job SHALL NOT store a static AWS access key or a
long-lived cloud-store token as a GitHub Actions secret for this purpose.

A CI workflow that authenticates the e2e suite through a non-password grant (for
example the GitHub-brokered client-credentials and token-exchange path) SHALL NOT
fetch these passwords at all; it only requires the test-tier principals to exist (see
Brokered Environments Seed Users for Identity Resolution).

#### Scenario: Password-grant job fetches from the Secret and masks before exposing to e2e

- GIVEN a job password-grants against a CI-owned PR environment
- WHEN it runs between the seed step and the test step
- THEN it SHALL read the three current passwords from Secret
  `hypershell-e2e-test-users` in the companion Keycloak namespace
- AND it SHALL mask each value in the runner before exposing it to the e2e step
- AND it SHALL expose them to the e2e step as its password environment variables

#### Scenario: Cloud-sourced passwords never leak in CI

- GIVEN a completed OpenShift e2e CI run
- WHEN a reader inspects the workflow logs and public artifacts
- THEN no test-tier principal's password SHALL appear unmasked in any of them

#### Scenario: No static cloud credential in CI configuration

- GIVEN a reader inspects a password-grant job's credential step
- WHEN they look for how it obtains the test-tier passwords
- THEN it SHALL use the cluster login plus the ESO Secret
- AND it SHALL NOT contain a static AWS access key for this purpose

### Requirement: Brokered Environments Seed Users for Identity Resolution

In GitHub-brokered per-PR environments the e2e suite authenticates through
client-credentials for the admin path and Keycloak token exchange for the developer
and platform-admin paths, and never sends a test-user password. Those environments
SHALL still seed the three test users, purely so that authentication by username -
token-exchange impersonation and role lookup - resolves; the seeded passwords SHALL
NOT be used for a password login there. Because bring-up seeds these users, a brokered
e2e job SHALL NOT need any password wiring to run.

#### Scenario: Brokered run needs the users but not their passwords

- GIVEN a GitHub-brokered per-PR environment whose e2e run uses client-credentials
  and token exchange
- WHEN the e2e suite runs
- THEN the three test users SHALL exist so token exchange and role lookup by username
  resolve
- AND the suite SHALL NOT send a test-user password
- AND the job SHALL require no password wiring beyond the users existing

### Requirement: Self-Service Role Assumption via Shared Test-Tier Principals

The three test-tier principals this spec seeds - `admin`, `developer`, and
`platform-admin` - SHALL double as the shared impersonation targets that
`ephemeral-pr-environments.spec.md` uses for testing a lower privilege tier from a
GitHub-brokered session. Any GitHub-authenticated user in a per-PR or dev environment,
not only a designated admin, SHALL be able to self-service impersonate `developer`
and `platform-admin` through Keycloak token exchange, so they can exercise that
tier's HyperShell API permission boundary without a second interactive account or a
password. They SHALL NOT be able to impersonate another GitHub-brokered user
through token exchange, and they SHALL NOT need to impersonate `admin` (their
own session already carries that tier). Interactive admin-console
impersonation, and the rest of the authorization model for who may impersonate
whom, is owned by `ephemeral-pr-environments.spec.md`; this spec owns only
where the impersonated principals and their credentials come from. Those
principals exist after CI seed and until the environment is destroyed. E2e
SHALL NOT de-seed them.

Because impersonation via Keycloak token exchange never sends a password, this
generalization SHALL NOT require any GitHub-authenticated user to know or fetch a
test-tier principal's password from the cloud store or the ESO Secret.

#### Scenario: Any authenticated GitHub user can self-service impersonate a tier

- GIVEN a GitHub user is authenticated to a per-PR or dev environment
- AND that environment has seeded the `developer` and `platform-admin` test-tier
  principals
- WHEN the user requests an impersonated token for either tier through Keycloak token
  exchange
- THEN Keycloak SHALL issue a token scoped to that tier's roles
- AND the user SHALL NOT need that tier's password to do so

#### Scenario: GitHub users cannot token-exchange as each other

- GIVEN GitHub user A and GitHub user B are both authenticated to a per-PR
  environment
- WHEN user A requests an impersonated token for user B through Keycloak token
  exchange
- THEN Keycloak SHALL deny the request
- AND user A SHALL still be able to impersonate `developer` and `platform-admin`

#### Scenario: Impersonation does not require cloud-store or ESO access

- GIVEN a GitHub-authenticated user self-service impersonates the `platform-admin`
  tier
- WHEN they complete the token exchange
- THEN no AWS, Vault, or ESO credential SHALL be involved
- AND their own GitHub-brokered session remains the only credential they hold

#### Scenario: Impersonation still works after e2e on a live environment

- GIVEN a CI e2e run has finished against a still-live PR environment
- WHEN a GitHub-authenticated user requests an impersonated token for `developer`
  or `platform-admin`
- THEN Keycloak SHALL issue that token
- AND `/pr-extend` SHALL NOT need to re-seed those principals

## Design Decisions

| Decision | Rationale |
|----------|-----------|
| Remove the three human users from the shared realm base entirely | Per-PR environments on `hysh-aws-01` are on the public internet. A realm login whose password equals its username is a live credential for anyone who can hit the Route. Removing the users from the shared base means no internet-facing environment ships them by default. Int/stage/prod get no replacement human password login; those environments keep whatever access they already have, and GitHub brokering is a PR-environment concern |
| AWS Secrets Manager is the durable store on `hysh-aws-01` | The cluster already uses the cloud operator's secret store and ESO IRSA for application secrets. Putting e2e passwords in the same class of store avoids standing up a second secret backend and keeps rotation, IAM, and audit in AWS |
| ESO is the generic in-cluster seam | Seed and e2e should not know AWS vs IBM vs Vault. They read one Kubernetes Secret. GitOps binds that Secret to the cluster's cloud store through a `SecretStore` / `ExternalSecret`. A later cloud swap changes only GitOps |
| In-cluster Secret `hypershell-e2e-test-users` with keys `admin`, `developer`, `platform-admin` | One name and one key set is the HyperShell contract. Matching the AWS JSON properties keeps ESO mapping one-to-one |
| `ClusterExternalSecret` into CI-owned Keycloak namespaces only | Keycloak namespaces are created at runtime. A cluster-scoped ESO object projects the Secret into each new `hypershell-ci-pr-*-keycloak` namespace without a Git commit per pull request. The PR app namespace runs pull-request images and SHALL NOT hold these passwords. Developer-owned OpenShift namespaces must not receive it, or local seed would pick up rotated passwords |
| One IRSA identity, read-only on `hysh-aws-01/e2e/test-users` | Same IRSA pattern as `SecretStore/hypershell-aws` and `keycloak-hub-aws`: bound token, audience `sts.amazonaws.com`, no static keys. Least privilege so ESO cannot rotate the secret or read unrelated HyperShell secrets |
| Vault is not the source for these passwords | In-cluster Vault on other hubs is dev-mode and a different ownership boundary (gateway credential driver). Application and e2e secrets on this AWS cluster follow the cloud store |
| Seed and password-grant e2e on CI-owned PR environments read the Kubernetes Secret, not AWS directly | Keeps lifecycle scripts cluster-portable and avoids putting AWS keys in CI. ESO already has the IRSA path. Local password-grant does not read this Secret |
| Seed is the primary consumer of the durable passwords; password-grant is secondary | Seed must set a non-guessable password so impersonation targets exist without `admin`/`admin`. Canonical PR CI is brokered and never reads the Secret. Password-grant against a CI-owned `pr-*` env is a supported secondary path that reads the same Keycloak-namespace Secret |
| CI-owned OpenShift password-grant does not fall back to username-equals-password | A missing Secret on a public PR environment must fail closed. Falling back to `admin`/`admin` would republish the exposure this spec exists to close. Developer-owned OpenShift keeps those seeds on purpose |
| CI-owned seed failure is fail-closed | Brokered e2e and impersonation need the users to exist. Warning and continuing would post an access comment for an environment whose impersonation targets were never seeded. Same rule as missing OAuth material |
| Seed accounts last for the environment lifetime; e2e does not de-seed | `/pr-extend` and other automation (agents, password-grant, impersonation) need the principals after e2e. Closing the password-login window at e2e exit made a retained environment unusable without another seed. Namespace destroy is the cleanup. The AWS secret and ESO Secret are longer-lived alignment copies |
| Driver `de_seed_test_users` is a no-op on every target | Reuses the suite's established driver-override model (`de_seed_test_users` in `e2e-testing.spec.md`) so the cleanup trap stays one call site. Kind, local OpenShift, and CI-owned PR environments all keep their seeded users until the environment itself is gone |
| Developer-owned environments keep static credentials, Kind or OpenShift | A human running e2e from a laptop - against Kind or after `make openshift-up` - needs `admin`/`admin` and `developer`/`developer`. Restricting those seeds to Kind would break manual OpenShift e2e. AWS and ESO on a laptop would add infrastructure for no local-dev benefit |
| Developer-owned OpenShift may still publish guessable logins | Those namespaces are the developer's isolation boundary (`ephemeral-pr-environments.spec.md` already excludes them from the CI timebox and reaper) and SHALL NOT use `pr-*` / `hypershell-ci-pr-*` names. The guessable-login closure applies to CI-owned `pr-*` environments, not to a developer exercising the suite by hand |
| Master bootstrap admin left out of scope | It is Keycloak's own console login, a different credential class from the realm test users |
| Passwords are never printed in CI; humans on PR envs use GitHub-brokered login | Nobody needs to type `admin`/`admin` against a public CI PR Route. Impersonation never sends a password. Local bring-up still prints the static seeds |
| Password-grant jobs use cluster login plus the ESO Secret, not a static AWS key | A long-lived cloud credential in a GitHub Actions secret is the same class of standing exposure this spec removes from Keycloak. Canonical PR CI is brokered and does not fetch these passwords. Cluster login is `ephemeral-ci-secrets.spec.md`; a password-grant job only reads the in-cluster password Secret after that login |
| Brokered PR environments seed users but never password-login | Client-credentials and token exchange resolve users by username. The users must exist for impersonation; their passwords are never sent on the public internet during that path |
| The three test-tier principals double as shared impersonation targets, generalized to any authenticated user | Reusing the seeded principals avoids a second account concept. A GitHub session already carries the `admin` tier's HyperShell API capability (`platform:admin` plus `gateway:creator`), so impersonation exists to reach `developer` and `platform-admin` without a password. Token-exchange impersonation SHALL NOT include other GitHub-brokered users. Interactive admin-console impersonation is owned by `ephemeral-pr-environments.spec.md` |
| Seed assigns Keycloak realm roles only | `gateway:viewer` is a per-gateway DB binding. A Keycloak seed step cannot grant it. Area 9's `openshell-user` path is a per-gateway client-role grant owned by `ephemeral-pr-environments.spec.md` |
