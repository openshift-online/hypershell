# Agent Credential Late Binding

**Date:** 2026-10-17
**Status:** Active
**Jira:** HYPERSHELL-409
**Supersedes:** HYPERSHELL-408 credential-materialization model (the raw Kubernetes `Secret` placement, `SecretPending`-on-raw-secret, and control-plane-owned rotation it defined)
**Related:** [`data-model.spec.md`](./data-model.spec.md) (SecretSource material model, Secret Submission and Storage, ProviderBinding, AgentRuntime provisioning), [`control-plane.spec.md`](./control-plane.spec.md) (AgentRuntime Reconciliation, Declarative `.hypershell` Import Reconciliation), [`security/rbac-enforcement.spec.md`](../security/rbac-enforcement.spec.md), [`standards/security/security.spec.md`](../standards/security/security.spec.md) (Secret Store Access Is Single-Directional per Component), [`openshell-gateway-credentials.spec.md`](./openshell-gateway-credentials.spec.md) (the separate gateway-provider Vault plane)

## Purpose

A declarative `.hypershell` agent fleet is imported on the hub before any Gateway
exists, yet an agent's credentials are consumed on whichever `ManagedCluster` its
Gateway is later placed on. The destination is therefore unknown at the moment a
`SecretSource` is declared. This spec defines the late-binding reference model that
removes that ordering constraint, and it fixes the materialization target of that
model.

HyperShell SHALL realize an agent's credentials as OpenShell **Providers** carrying a
**refresh strategy**. The control plane SHALL NOT render a raw Kubernetes `Secret` for
the credential. A raw `Secret` is inert, so whoever renders it must rotate it; an
OpenShell Provider is live. Given the durable **material** (the GitHub App PEM, the
Vertex service-account key, the gateway identity), rotation ownership depends on the
refresh strategy:

- For the strategies OpenShell supports natively (`google-service-account-jwt`,
  `oauth2-refresh-token`, `oauth2-client-credentials`, `aws-sts-assume-role`, and the
  no-refresh `static-token`), **OpenShell owns rotation**: the control plane supplies
  the material and the refresh strategy, and OpenShell mints and rotates the usable
  credential.
- For **GitHub App credentials** (the HyperShell `app-installation-token` intent), the
  installed OpenShell CLI has no `app-installation-token` refresh strategy, so a bounded,
  GitHub-specific **control-plane rotator** owns rotation: the control plane mints the
  short-lived (approximately one hour) installation token from the stored PEM (plus
  `app_id` / `installation_id`) and sets it on the OpenShell provider credential through
  `provider update --credential`, re-minting before expiry. OpenShell does NOT own GitHub
  rotation.

Each `.hypershell` `ProviderBinding` carries these `refresh_strategy` values, and the
control plane realizes each as an OpenShell provider through OpenShell's existing provider
API (the operations the `openshell` CLI drives;
[`agent-workspace-gateway-api.spec.md`](./agent-workspace-gateway-api.spec.md)).

Late binding is preserved: a `SecretSource` is declared by name as a value-less
reference at import time, the durable material is supplied later by the user into the
**central secret store** ([`central-secret-store.spec.md`](./central-secret-store.spec.md),
PLAT-SSS), and the agent realizes its providers the moment the material appears. The
material lives in the central, OIDC-secured source store; the control plane consumes it
in the ExternalSecrets pattern (read source, deliver to an OpenShell provider), reading
the central store with a least-privilege machine identity, cross-cluster if needed
(PLAT-SSS-03/04).

The user-upload store (the central source of truth, PLAT-SSS Store 1) is a SEPARATE
plane from the per-gateway OpenShell runtime store that holds the derived provider copy
([`openshell-gateway-credentials.spec.md`](./openshell-gateway-credentials.spec.md),
Store 2) and from application secrets (Secrets Manager plus External Secrets Operator).
The three planes SHALL NOT be conflated (PLAT-SSS-08). This spec governs how the control
plane CONSUMES the central store to realize an agent's providers; the store service,
its OIDC auth, and the user upload model are defined in PLAT-SSS.

> Supersedes the prior per-cluster late-binding topology (material written directly into
> each assigned cluster's local Vault, "material never transits the hub," every runtime
> read cluster-local). That distributed model is replaced by the single central source
> store of PLAT-SSS; the requirements below are retargeted accordingly.

## Requirements

### Requirement: AGT-SEC-01 -- Credentials Realized as OpenShell Providers, Not Rendered Secrets

On reconcile, the control plane SHALL realize each of an `AgentRuntime`'s declared
`ProviderBinding`s as an OpenShell **provider** on the agent's gateway/workspace,
from the binding's `provider_spec_id`, its `refresh_strategy`, and the durable
material referenced through its `SecretSource`s. The control plane SHALL provision
and update these providers through OpenShell's existing provider API, idempotently, the
same mechanism it uses for the rest of the gateway-side workspace objects.

The control plane SHALL NOT render the agent credential as a raw Kubernetes `Secret`
and SHALL NOT depend on an `ExternalSecret` or External Secrets Operator to
materialize it. The durable material SHALL be read from the central store (see
"Central-Store Material Read", AGT-SEC-05) and supplied to OpenShell as provider
configuration; the usable credential lives inside OpenShell, not in a `Secret` the
control plane placed.

The coordinator's own `gateway-identity` credential - the OIDC identity the
coordinator `CronJob` uses to authenticate TO the gateway so it can reach its
providers - is the one credential the coordinator consumes directly rather than as a
provider, and it is read from the central store and wired as coordinator environment per
[`data-model.spec.md`](./data-model.spec.md) ("SecretSource"). It is NOT a provider
and is NOT a rendered agent credential `Secret` for provider material.

#### Scenario: AGT-SEC-01.01 -- ProviderBindings become OpenShell providers, no credential Secret

- GIVEN an `AgentRuntime` whose `.hypershell` declares `ProviderBinding`s with `refresh_strategy` values
- WHEN the control plane reconciles it and the referenced material is present in the central store
- THEN it SHALL create or update one OpenShell provider per `ProviderBinding` on the agent's gateway/workspace with the declared refresh strategy
- AND it SHALL NOT render a raw Kubernetes `Secret` for the credential and SHALL NOT create an `ExternalSecret`

### Requirement: AGT-SEC-02 -- Rotation Ownership by Refresh Strategy

Rotation ownership SHALL be determined by the provider `refresh_strategy`:

- For the strategies OpenShell supports natively (`google-service-account-jwt`,
  `oauth2-refresh-token`, `oauth2-client-credentials`, `aws-sts-assume-role`, and the
  no-refresh `static-token`), OpenShell SHALL own credential minting and rotation. The
  control plane SHALL supply the durable material and the refresh strategy and SHALL NOT
  mint, refresh, or rotate the usable credential for these strategies, SHALL NOT place or
  rotate a raw `Secret`, and SHALL NOT run a control-plane rotator for them. Once such a
  provider is realized, OpenShell SHALL keep the usable credential current with no
  external rotation.
- For GitHub App credentials (the `app-installation-token` intent), a bounded,
  GitHub-specific control-plane rotator SHALL own rotation, because the installed OpenShell
  CLI has no `app-installation-token` refresh strategy. The control plane SHALL mint the
  installation token from the stored PEM and set it on the OpenShell provider through
  `provider update --credential`, re-minting before expiry. This control-plane rotator is
  permitted and required for GitHub and is scoped to GitHub App credentials only.

The control plane SHALL NOT render or rotate a raw Kubernetes `Secret` for any provider
credential; the usable credential lives inside OpenShell, set either by OpenShell's own
refresh or by the GitHub control-plane rotator's `provider update`.

#### Scenario: AGT-SEC-02.01 -- OpenShell-supported provider keeps working with no external rotation

- GIVEN an OpenShell provider realized from stored material with a `google-service-account-jwt` refresh strategy
- WHEN more than one minted-token lifetime elapses
- THEN the provider SHALL continue to authenticate because OpenShell refreshes the token
- AND no control-plane component SHALL mint or rotate the credential

#### Scenario: AGT-SEC-02.02 -- GitHub provider token is re-minted by the control-plane rotator

- GIVEN an OpenShell GitHub provider realized from a stored GitHub App PEM
- WHEN the installation token approaches its approximately one-hour expiry
- THEN the control-plane GitHub rotator SHALL mint a fresh installation token from the PEM and set it through `provider update --credential`
- AND no raw credential `Secret` SHALL be rendered

### Requirement: AGT-SEC-03 -- Material Resolves From the Central Store

All `SecretSource` material SHALL resolve from the single central secret store
([`central-secret-store.spec.md`](./central-secret-store.spec.md), PLAT-SSS), not from
per-cluster stores. The control plane SHALL read the referenced material from the
central store with its least-privilege machine reader identity (PLAT-SSS-03), including
cross-cluster when it reconciles a gateway on a spoke whose user material lives in the
central hub store (PLAT-SSS-04); there is no requirement that the read be cluster-local.

Material reaches the central store by one of two upload paths, both writing to the SAME
central store at the `SecretSource`'s referenced path:

1. **User-OIDC-direct upload (primary).** The user writes their material to their
   identity-scoped path in the central store, authenticating with their own Red Hat
   SSO/OIDC identity (PLAT-SSS-02). HyperShell is not in this write path.
2. **Api-server passthrough (secondary convenience).** The user submits material to the
   api-server, which writes it to the central store through its write-only
   `pkg/secretstore` client under the same referenced path, authorized by the
   gateway-tenancy check (AGT-SEC-14, SEC-RBAC-19). This serves `hsctl`/API callers who
   do not write to the store directly, and the `Repository` import credential.

Both upload paths and the control-plane read target the one central store; there is no
per-cluster writer, no hub-staging-versus-spoke split, and no "material never transits
the hub" rule (the central store IS the shared source).

#### Scenario: AGT-SEC-03.01 -- Spoke reconcile reads the central store; either upload path lands in it

- GIVEN a multi-cluster fleet with one central hub secret store
- WHEN an `AgentRuntime` `SecretSource` material is consumed on a spoke cluster
- THEN the spoke's control plane SHALL read it from the central store with its machine reader identity (cross-cluster)
- WHEN the user provides the material either by OIDC-direct upload or through the api-server passthrough
- THEN both SHALL write to the same central-store path the `SecretSource` references

### Requirement: AGT-SEC-04 -- Reference-Only SecretSource Submission

A `SecretSource` SHALL be a first-class, valid resource with NO material supplied at
creation. A `SecretSource` holds durable provider **material** (a GitHub App PEM, a
Vertex service-account key, a gateway identity), not a usable, rotatable credential.
On the api-server create path the material SHALL be optional: when material is
supplied the api-server writes it to the configured secret store through its
write-only `pkg/secretstore` client and records only the reference, exactly as
[`data-model.spec.md`](./data-model.spec.md) ("Secret Submission and Storage")
defines; when no material is supplied the api-server SHALL persist the `SecretSource`
reference alone and SHALL perform no `pkg/secretstore` write.

A reference-only `SecretSource` SHALL carry its convention path so the material can be
uploaded later to a known location. The api-server SHALL derive that path from the
submission identity as `hypershell/secret-sources/<agent_runtime_id>/<name>` (the
same deterministic location a material-bearing submission is written to) on EVERY
create, and SHALL ignore any client-supplied `path`: honoring a client path would let
any `gateway:creator` pin a reference at another tenant's store location, which the
control plane would then materialize into the caller's namespace. The `path` SHALL
NOT be settable on update either. The derived path SHALL be stable across
re-submission of the same `SecretSource`.

The material is uploaded later into the central store (PLAT-SSS) at that referenced
path: primarily by the user writing directly with their own OIDC identity (PLAT-SSS-02),
or through the api-server passthrough (AGT-SEC-14). The reference is declared value-less
at import and filled afterward; the control plane reads it from the central store when it
reconciles (AGT-SEC-05).

Material MAY be supplied at create time (the submission flow above) or POST-HOC against
an already-created reference-only `SecretSource`, through either upload path; both write
the same referenced central-store path (see "End-User Material Upload Through the
Api-Server", AGT-SEC-14).

#### Scenario: AGT-SEC-04.01 -- Create a value-less SecretSource reference

- GIVEN a `SecretSource` create request with `name`, `agent_runtime_id`, `purpose`, and `backend` and NO material
- WHEN the api-server accepts the request
- THEN it SHALL persist the `SecretSource` reference with `path = hypershell/secret-sources/<agent_runtime_id>/<name>`, ignoring any client-supplied `path`
- AND it SHALL NOT perform any `pkg/secretstore` write
- AND the created `SecretSource` SHALL be a valid, consumable resource

#### Scenario: AGT-SEC-04.02 -- Hub-submitted material follows the write-then-record flow

- GIVEN material legitimately submitted at the hub (the `Repository` import credential PEM, or any secret in a co-located single-cluster deployment)
- WHEN it is supplied through the api-server secret-submission path
- THEN the api-server SHALL write it to the store at the `SecretSource` convention path FIRST
- AND it SHALL NOT persist the raw material in the database and SHALL NOT return it on any read

#### Scenario: AGT-SEC-04.03 -- Material is uploaded to the central store at the referenced path

- GIVEN a reference-only `SecretSource` consumed by an `AgentRuntime` placed on a spoke cluster
- WHEN the material is provided
- THEN it SHALL be uploaded to the central store at the `SecretSource`'s referenced path (by user-OIDC-direct upload or the api-server passthrough)
- AND the control plane SHALL read it from the central store (cross-cluster) when it reconciles the runtime

### Requirement: AGT-SEC-05 -- Central-Store Material Read

The referenced material SHALL be read from the central secret store
([`central-secret-store.spec.md`](./central-secret-store.spec.md), PLAT-SSS) and used to
configure the agent's OpenShell providers. When the control plane reconciles an
`AgentRuntime` whose effective cluster is its own `cluster_id`, it SHALL read each
referenced `SecretSource` material from the central store through its read-only,
least-privilege `pkg/secretstore` client (PLAT-SSS-03), including cross-cluster when the
gateway is on a spoke and the material lives in the central hub store, and SHALL supply
that material to OpenShell as provider configuration over the gateway gRPC API. The
control plane SHALL NOT render the material into a raw Kubernetes `Secret` as the agent
credential. The read MAY cross clusters (the central store is the shared source); there
is no requirement that it be cluster-local.

The central user-secret store (PLAT-SSS Store 1) SHALL be a SEPARATE secret plane from
the per-gateway OpenShell runtime store that holds the derived provider copy
([`openshell-gateway-credentials.spec.md`](./openshell-gateway-credentials.spec.md),
Store 2) and from the application-secret plane (Secrets Manager plus External Secrets
Operator). The three planes SHALL NOT share paths, policies, or identities and SHALL NOT
be conflated (PLAT-SSS-08).

#### Scenario: AGT-SEC-05.01 -- Material is read from the central store to configure providers

- GIVEN an `AgentRuntime` placed on `cluster_id: Y` with a `SecretSource` reference whose material is in the central store
- WHEN the control plane registered as `Y` reconciles the `AgentRuntime`
- THEN it SHALL read the material from the central store (cross-cluster if the store is served from the hub) and configure the OpenShell provider with it
- AND it SHALL use a least-privilege read-only identity scoped to the referenced path
- AND no raw credential `Secret` SHALL be rendered by the control plane

#### Scenario: AGT-SEC-05.02 -- The derived copy lives only in OpenShell, not a rendered Secret

- GIVEN a `SecretSource` whose material is supplied in the central store and later consumed
- WHEN the control plane realizes the provider
- THEN the usable credential SHALL exist in the central store (source) and inside the OpenShell provider / its per-gateway runtime store (derived copy, Store 2)
- AND the control plane SHALL render no raw credential `Secret`

### Requirement: AGT-SEC-06 -- SecretPending Condition Gates Provider Creation

When referenced `SecretSource` material is absent from the assigned cluster's local
Vault, the control-plane reconcile SHALL treat this as an expected waiting state, not
a failure. It SHALL:

- set a durable, observable `SecretPending` status condition on the owning
  `AgentRuntime` (and surface it per pending `SecretSource`),
- NOT create the OpenShell provider(s) that depend on the absent material and NOT
  configure a provider with empty or partial material,
- NOT mark the `AgentRuntime` workspace `Ready` (the runtime stays in `Provisioning`,
  not `Running`),
- NOT crash-loop or exit, and
- retry idempotently on the normal reconcile cadence, converging the moment the
  material appears, with no manual kick (reconcile, do not create-or-skip).

An absent referenced material (`SecretPending`) SHALL be distinct from a store read
failure of material expected to exist. A reachable store that reports the material
missing SHALL yield `SecretPending`; a store that is unreachable or errors SHALL
continue to degrade the `AgentRuntime` as [`data-model.spec.md`](./data-model.spec.md)
("AgentRuntime Status Lifecycle") defines. `SecretPending` SHALL NOT transition the
`AgentRuntime` to `Degraded`.

Provider creation is self-healing and idempotent: once the material appears the
control plane realizes the provider; re-running reconcile against an already-realized
provider SHALL update it in place without error. Rotation of the realized credential
thereafter is owned by OpenShell for its native strategies and by the bounded
control-plane GitHub rotator for GitHub App credentials (see "Rotation Ownership by
Refresh Strategy"); the control plane does not re-place a raw credential `Secret`.

#### Scenario: AGT-SEC-06.01 -- Missing material sets SecretPending and holds, not Ready

- GIVEN an `AgentRuntime` whose `SecretSource` material has not yet been uploaded to the central store
- WHEN the control plane reconciles the `AgentRuntime`
- THEN it SHALL set a `SecretPending` condition naming the pending `SecretSource`
- AND it SHALL NOT create the dependent OpenShell provider and SHALL NOT mark the workspace `Ready`
- AND it SHALL NOT crash-loop

#### Scenario: AGT-SEC-06.02 -- Converges the moment the material appears

- GIVEN an `AgentRuntime` reporting `SecretPending`
- WHEN the operator writes the material into the central store at the given path
- THEN the next reconcile SHALL read the material, realize the OpenShell provider, clear `SecretPending`, and converge the `AgentRuntime` to `Running`
- AND no manual reconcile trigger SHALL be required

#### Scenario: AGT-SEC-06.03 -- Reachable-but-missing is Pending, unreachable is Degraded

- GIVEN an `AgentRuntime` with a `SecretSource` reference
- WHEN the central store is reachable and reports the material missing
- THEN the control plane SHALL report `SecretPending` and SHALL NOT set `Degraded`
- WHEN instead the Vault is unreachable or returns an error
- THEN the control plane SHALL set `Degraded` and surface the failing `SecretSource` name

### Requirement: AGT-SEC-07 -- Discoverable Upload Target

The platform SHALL make each pending credential actionable. On the
`AgentRuntime`/`SecretSource` status the platform SHALL surface the exact central-store
path at which the material must be uploaded (the `SecretSource`'s referenced path in the
central store, PLAT-SSS). The `SecretPending` condition message SHALL read as an
actionable next step (for example, `upload <secret-source-name> to <central-store-path>`)
rather than a bare error. The target is the central store the user writes to with their
own OIDC identity (PLAT-SSS-02), not a per-cluster mount.

#### Scenario: AGT-SEC-07.01 -- SecretPending names the exact central-store upload path

- GIVEN an `AgentRuntime` reporting `SecretPending` for a `SecretSource`
- WHEN a user reads the `AgentRuntime` status
- THEN the status SHALL state the exact central-store path to upload the material to
- AND the condition message SHALL be phrased as an instruction, not a bare error

### Requirement: AGT-SEC-08 -- Scoped Upload Access to the Central Store

Uploading material SHALL use a least-privilege, upload-only grant on the central store,
scoped to the uploader's own path, separate from any reader identity. The primary
uploader is the end user authenticating with their own Red Hat SSO/OIDC identity,
authorized to write (not read or list) only under their identity- or team-scoped path
(PLAT-SSS-02). The api-server passthrough writer (AGT-SEC-14) is the secondary path and
is likewise confined to writing the `SecretSource`'s referenced path. No uploader
identity SHALL be granted read access to other material or any access to the per-gateway
OpenShell runtime store (Store 2). There is no per-cluster writer identity: all uploads
target the one central store.

#### Scenario: AGT-SEC-08.01 -- Upload is confined to the uploader's central-store path

- GIVEN a user uploading material with their own OIDC identity
- WHEN they authenticate to the central store
- THEN they SHALL be permitted to write only under their identity- or team-scoped path
- AND they SHALL NOT be permitted to read other material or to write to the per-gateway OpenShell runtime store

### Requirement: AGT-SEC-09 -- GitHub App Credentials via app-installation-token

GitHub credentials - BOTH the agent GitHub providers AND the `Repository` import
credential the `RepositoryReconciler` uses to read a private `.hypershell` subtree -
SHALL store only the durable GitHub App **PEM** (plus `app_id` / `installation_id`),
NOT a static installation token. The short-lived (approximately one hour) installation
token SHALL be minted from the PEM on demand by the **control plane**, not stored. The
`app-installation-token` value is a HyperShell refresh-strategy intent; the installed
OpenShell CLI has no matching refresh strategy, so the control plane owns GitHub token
minting and rotation.

- For an agent GitHub provider, the control plane SHALL mint the installation token from
  the stored PEM and set it on the OpenShell provider credential through
  `provider create --credential` / `provider update --credential`, and a bounded
  GitHub-specific control-plane rotator SHALL re-mint and update the credential before
  expiry. OpenShell does NOT own GitHub rotation; the control plane does.
- For the `Repository` import credential, the `RepositoryReconciler` SHALL mint the
  installation token from the stored PEM (plus `app_id` / `installation_id`) when it
  reads the subtree, rather than reading a pre-stored static installation token. This
  SUPERSEDES the static-token credential-resolver path previously used by the
  `RepositoryReconciler`.

#### Scenario: AGT-SEC-09.01 -- Agent GitHub provider is realized from the stored PEM

- GIVEN an `AgentRuntime` with a GitHub `ProviderBinding` whose `refresh_strategy.type = app-installation-token` and whose referenced `SecretSource` holds the GitHub App PEM
- WHEN the control plane reconciles it with the PEM present in the central store
- THEN it SHALL mint an installation token from the PEM, `app_id`, and `installation_id` and set it on the OpenShell provider through `provider create/update --credential`
- AND no static installation token SHALL be stored and no raw credential `Secret` SHALL be rendered
- AND the control-plane GitHub rotator SHALL re-mint and update the credential before expiry

#### Scenario: AGT-SEC-09.02 -- Repository import mints its token from the PEM

- GIVEN a `Repository` whose import credential `SecretSource` holds a GitHub App PEM plus `app_id` / `installation_id`
- WHEN the `RepositoryReconciler` reads the `.hypershell` subtree
- THEN it SHALL mint a short-lived installation token from the PEM and use it to authenticate to GitHub
- AND it SHALL NOT read a pre-stored static installation token

### Requirement: AGT-SEC-10 -- Rotation Ownership Split Between OpenShell and the Control Plane

For the strategies OpenShell supports natively, keeping the realized credential current
SHALL be OpenShell's responsibility through the provider `refresh_strategy`, not a
control-plane concern. For those strategies the control plane realizes the provider when
the material first appears and updates the provider configuration in place when the
`ProviderBinding` or its material changes; it SHALL NOT mint, refresh, or rotate the
usable credential and SHALL NOT re-place a rendered `Secret`. There SHALL be no
control-plane periodic resync of an OpenShell-rotated credential value.

For GitHub App credentials, keeping the realized credential current SHALL be the bounded
control-plane GitHub rotator's responsibility: it re-mints the installation token from the
stored PEM before expiry and sets it on the OpenShell provider through
`provider update --credential`. In all cases the control plane SHALL NOT re-place a raw
Kubernetes `Secret`.

#### Scenario: AGT-SEC-10.01 -- OpenShell-supported credential rotation happens inside OpenShell

- GIVEN an `AgentRuntime` whose Vertex provider was realized from stored material with a `google-service-account-jwt` strategy
- WHEN the short-lived credential OpenShell mints expires
- THEN OpenShell SHALL refresh it under the provider `refresh_strategy`
- AND the control plane SHALL NOT mint, refresh, or rotate the credential and SHALL NOT re-place any `Secret`

#### Scenario: AGT-SEC-10.02 -- GitHub credential rotation happens in the control-plane rotator

- GIVEN an `AgentRuntime` whose GitHub provider was realized from a stored PEM
- WHEN the installation token approaches expiry
- THEN the control-plane GitHub rotator SHALL re-mint the token from the PEM and set it through `provider update --credential`
- AND it SHALL NOT re-place any raw `Secret`

### Requirement: AGT-SEC-11 -- Import Reconciler Enablement

The declarative `.hypershell` import reconciler (`RepositoryReconciler`,
[`control-plane.spec.md`](./control-plane.spec.md), "Declarative `.hypershell` Import
Reconciliation") SHALL be enablable through the configuration flag
`HYPERSHELL_REPOSITORY_IMPORT_ENABLED`. When the flag is unset or `false` the control
plane SHALL log that the import reconciler is disabled and SHALL NOT run the import
loop; when it is `true` the control plane SHALL run the import reconcile loop. Enabling
the import is configuration, not a code change (separate configuration from code).

#### Scenario: AGT-SEC-11.01 -- Import runs only when enabled

- GIVEN a control plane with `HYPERSHELL_REPOSITORY_IMPORT_ENABLED=false`
- WHEN the control plane starts
- THEN it SHALL log that the import reconciler is disabled and SHALL NOT run the import loop
- WHEN instead `HYPERSHELL_REPOSITORY_IMPORT_ENABLED=true`
- THEN it SHALL run the import reconcile loop

### Requirement: AGT-SEC-12 -- Secret Store Wiring on Both Components

The secret store SHALL be wired through the `HYPERSHELL_SECRET_STORE_*` configuration,
keeping the single-directional split from
[`security/security.spec.md`](../standards/security/security.spec.md) (api-server
passthrough write-only, control plane read-only). All components SHALL point at the one
central store (PLAT-SSS):

- The api-server SHALL configure its WRITE client from `HYPERSHELL_SECRET_STORE_*` for
  the passthrough upload path (AGT-SEC-14) and the `Repository` import credential,
  writing to the central store.
- Each control plane SHALL configure its READ client from `HYPERSHELL_SECRET_STORE_*`
  pointing at the central store, which it reads with a least-privilege identity
  (PLAT-SSS-03), cross-cluster if the store is served from the hub.
- The api-server write client and the control-plane read client address the SAME central
  store; there is no per-cluster store to diverge onto.

When the backend is unset, the api-server SHALL reject secret submissions loudly and a
control plane SHALL fail provider realization loudly rather than configure a provider
with empty material (no silent degradation). A `Repository` that declares an import
credential SHALL be resolvable only when the central store is wired.

#### Scenario: AGT-SEC-12.01 -- Unwired store fails loudly on both directions

- GIVEN `HYPERSHELL_SECRET_STORE_BACKEND` is unset
- WHEN a user submits credential material through the api-server passthrough
- THEN the api-server SHALL reject the submission with a clear configuration error
- AND when a control plane attempts to read `SecretSource` material to realize a provider
- THEN it SHALL fail loudly and SHALL NOT configure a provider with empty material

#### Scenario: AGT-SEC-12.02 -- Both components address the central store

- GIVEN a multi-cluster fleet
- WHEN the api-server and a spoke control plane are wired through `HYPERSHELL_SECRET_STORE_*`
- THEN both SHALL address the one central store (the control-plane read being cross-cluster when the store is on the hub)
- AND there SHALL be no per-cluster store the material diverges onto

### Requirement: AGT-SEC-13 -- Central-Store Client Capabilities

The secret-store client SHALL be able to reach the central store (PLAT-SSS) as it is
served. Observable behavior:

- **Private-CA trust.** The client SHALL complete a TLS handshake against a store served
  with a PRIVATE certificate authority, using a configurable trust bundle rather than the
  system trust store alone. A privately-signed store SHALL NOT be rejected for an
  untrusted-CA reason.
- **Store auth method.** The client SHALL authenticate using the method the central store
  requires. The control-plane reader SHALL authenticate with its own machine identity
  (Kubernetes auth with its ServiceAccount, or an OIDC/JWT identity reusing the keyless
  exchange of PLAT-CITR); the existing static-token method SHALL remain supported for
  stores that use it. The control-plane identity SHALL be read-only (PLAT-SSS-03).
- **Scoped policy and path mapping.** The read SHALL be governed by a policy scoped to
  the referenced path, distinct from the per-gateway OpenShell runtime store policy. The
  `SecretSource`'s recorded path is a location in the central store (PLAT-SSS); the write
  (passthrough) and read clients SHALL apply the SAME store, mount, and path so the stored
  reference and the policy path refer to the same entries.

These capabilities SHALL be driven by `HYPERSHELL_SECRET_STORE_*` configuration, not by
code changes. (The current implementation surface is the `pkg/secretstore` write and
read clients.)

#### Scenario: AGT-SEC-13.01 -- Reader connects to the privately-signed central store, read-only and scoped

- GIVEN the central store served with a private CA
- WHEN the control plane reads `SecretSource` material from it
- THEN the client SHALL trust the configured private-CA bundle and complete the TLS handshake
- AND it SHALL authenticate with its own machine identity and read only under the referenced-path policy
- AND it SHALL NOT hold write access to user material

### Requirement: AGT-SEC-14 -- End-User Material Upload Through the Api-Server

HyperShell SHALL let an end user upload the durable material for an already-created,
reference-only `SecretSource` through the api-server, extending the hub-side
write-then-record path (AGT-SEC-04, AGT-SEC-04.02) from a create-time submission to a
POST-HOC upload against an existing reference. Before this operation, material could be
supplied only at `SecretSource` create time; a reference-only `SecretSource` created
first (for example by a `.hypershell` import or `hsctl apply`) had no first-class
end-user upload path. The api-server SHALL expose a write-only material-upload
operation on an existing `SecretSource` (see
[`data-model.spec.md`](./data-model.spec.md), "SecretSource Material Upload
Operation") that carries ONLY the material, passes it through to the api-server's
configured secret store using the EXISTING write-only `pkg/secretstore` write client,
writes it at the `SecretSource`'s server-derived convention path
`hypershell/secret-sources/<agent_runtime_id>/<name>`, and records only the reference.

**Authentication.** The caller SHALL authenticate to the api-server with the same
SSO/OIDC identity model as every other `hsctl`/API call. The operation SHALL NOT
require a platform-administrator identity, a Keycloak administrator identity, or a
direct secret-store (Vault) credential. This is the non-admin end-user upload path.

**Authorization.** A caller MAY upload material for a `SecretSource` only when the
caller is authorized to manage that `SecretSource`'s owner, namely its `AgentRuntime`
and that runtime's gateway tenant: the caller SHALL hold `gateway:owner` or
`gateway:admin` on the `AgentRuntime`'s gateway, or `platform:admin`
(owner-equivalent, GTW-ACC-08), through the same `CanManageGatewayResources` tenancy
check the other gateway-scoped `SecretSource` operations use (SEC-RBAC-19). A caller
lacking that binding SHALL be denied and the store SHALL be left unchanged. The
authorization check SHALL run BEFORE any store write, so a rejected upload writes
nothing.

**Write-only material.** The material SHALL be write-only: it SHALL NOT be returned on
any read of the `SecretSource`, SHALL NOT be persisted raw in the database, and SHALL
NOT be echoed in the upload response, consistent with AGT-SEC-04.02.

**Idempotency and errors.** Re-uploading SHALL overwrite the same convention path (the
deterministic location makes the write idempotent); a retried or repeated upload SHALL
NOT fork the stored material. An upload naming a `SecretSource` that does not exist
SHALL be rejected with `404` and SHALL write nothing. When the secret store backend is
unwired the api-server SHALL reject the upload loudly with a configuration error and
SHALL write nothing (no silent degradation, AGT-SEC-12).

**Secondary to the user-OIDC upload.** This api-server passthrough is the SECONDARY
upload path (AGT-SEC-03): it serves `hsctl`/API callers who do not write to the central
store directly. The PRIMARY path is the user writing to the central store with their own
OIDC identity (PLAT-SSS-02). Both write the SAME central store at the `SecretSource`'s
referenced path, so an upload by either path clears a pending `SecretSource` on the next
reconcile (AGT-SEC-06) and matches the `SecretPending` upload target (AGT-SEC-07). The
passthrough requires the central store to be wired on the api-server; it carries no
per-cluster or hub-versus-spoke distinction (there is one central store).

#### Scenario: AGT-SEC-14.01 -- Authorized upload passes material through to the convention path

- GIVEN a caller with `gateway:owner` on the `AgentRuntime`'s gateway and an already-created reference-only `SecretSource` for that runtime
- AND the api-server secret store is wired and hub-reachable (path 1)
- WHEN the caller uploads material through the api-server material-upload operation
- THEN the api-server SHALL write the material through its write-only `pkg/secretstore` client to `hypershell/secret-sources/<agent_runtime_id>/<name>`
- AND it SHALL NOT change the stored `SecretSource` reference's `path`

#### Scenario: AGT-SEC-14.02 -- Uploaded material is write-only

- GIVEN a successful material upload
- WHEN the `SecretSource` is subsequently read
- THEN the material SHALL NOT appear in any read response and SHALL NOT be echoed in the upload response
- AND the material SHALL NOT be persisted raw in the database

#### Scenario: AGT-SEC-14.03 -- Unauthorized caller is denied and nothing is written

- GIVEN a caller holding `gateway:creator` but no `gateway:owner`/`gateway:admin` on the `AgentRuntime`'s gateway and no `platform:admin`
- WHEN the caller attempts to upload material for that `SecretSource`
- THEN the api-server SHALL deny the upload with `403 Forbidden`
- AND the secret store entry at the `SecretSource`'s convention path SHALL be unchanged

#### Scenario: AGT-SEC-14.04 -- Re-upload overwrites the same path idempotently

- GIVEN a `SecretSource` whose material was already uploaded
- WHEN the caller uploads replacement material for the same `SecretSource`
- THEN the api-server SHALL overwrite the entry at the same convention path
- AND it SHALL NOT create a second, forked store entry

#### Scenario: AGT-SEC-14.05 -- Unknown SecretSource and unwired store fail loudly, writing nothing

- GIVEN an upload naming a `SecretSource` id that does not exist
- WHEN the api-server evaluates it
- THEN it SHALL respond `404` and SHALL write nothing to the store
- WHEN instead the `HYPERSHELL_SECRET_STORE_BACKEND` is unset for an existing `SecretSource`
- THEN the api-server SHALL reject the upload with a clear configuration error and SHALL write nothing

#### Scenario: AGT-SEC-14.06 -- Passthrough and OIDC-direct upload target the same central path

- GIVEN a `SecretSource` whose material can be provided either by user-OIDC-direct upload or by the api-server passthrough
- WHEN the material is uploaded by either path
- THEN it SHALL be written to the same central-store path the `SecretSource` references
- AND the next control-plane reconcile SHALL read it from the central store and clear `SecretPending`

## Design Decisions

| Decision | Rationale |
|----------|-----------|
| Realize credentials as OpenShell providers, not rendered raw Secrets | A rendered raw `Secret` is inert, so whoever rendered it must rotate it - the root cause of the stale-token problem in the superseded model. An OpenShell provider is live: given durable material plus a refresh strategy, OpenShell mints and rotates the usable credential for its native strategies, removing any need for a control-plane rotator for those strategies; for GitHub App credentials a bounded GitHub-specific control-plane rotator is required because the installed OpenShell CLI has no `app-installation-token` refresh strategy (see the next row). It is already the declared model (`ProviderBinding.refresh_strategy`) and the control plane's existing gateway provider-provisioning pattern. |
| Rotation ownership split by strategy | Rotation belongs where the live credential can be minted. For strategies OpenShell supports natively, OpenShell mints and rotates, keeping the control plane a configurator rather than a credential issuer. For GitHub App credentials the installed OpenShell CLI has no `app-installation-token` refresh strategy, so a bounded control-plane GitHub rotator mints the installation token from the PEM and updates the provider credential. Splitting ownership this way matches what OpenShell can actually do rather than asserting an OpenShell capability that is not confirmed. |
| SecretSource holds durable material, not a usable credential | The durable material (GitHub App PEM, Vertex SA key, gateway identity) is long-lived and rotation-free; the usable, short-lived credential is derived from it on demand. Storing only the material, never the usable credential, is what lets the rotation owner (OpenShell for its native strategies, the bounded control-plane GitHub rotator for GitHub App credentials) mint the usable credential when needed, and keeps no rotatable secret at rest in a control-plane-placed `Secret`. |
| Late-binding reference model over the central store | The gateway's destination cluster is unknown at import time, but the material does not need to follow it: a value-less `SecretSource` reference is declared at import, the user later uploads the material to the central store (PLAT-SSS), and the control plane realizes the provider the moment it appears. Late binding is preserved without a per-cluster store. |
| Central store read with least privilege (supersedes "no global store") | The prior model forbade a global runtime store to avoid a central trust sink. That is reversed here: there is one central, OIDC-secured store (PLAT-SSS). The blast-radius concern is met instead by OIDC path-scoping, read-only least-privilege machine consumers, per-path isolation, and an audit device, which also lets other projects share the store (the point of standing it up). |
| One central store, two upload paths | There is one central source store. Material reaches it either by the user writing directly with their OIDC identity (primary, PLAT-SSS-02) or through the api-server passthrough (secondary, AGT-SEC-14). Both target the same referenced path, so neither forks the material nor reintroduces a per-cluster store. |
| GitHub App via app-installation-token for both agent providers and the Repository import credential | Storing only the durable PEM (plus `app_id` / `installation_id`) and minting the ~1h installation token on demand means there is no static installation token to go stale. The control plane mints the token and sets it on the OpenShell provider (agent providers) or uses it directly (the `RepositoryReconciler` import credential). Unifying both on this one PEM model supersedes the static-token resolver and closes the import-credential rotation gap; the token minting is a bounded, GitHub-specific control-plane rotator. |
| `SecretPending` is a benign waiting condition that gates provider creation, not `Degraded` | A reference whose material is not yet uploaded is an expected state during rollout, not a fault. Distinguishing reachable-but-missing (Pending) from unreachable/error (Degraded) gives operators an actionable next step without false alarms, and lets the agent realize its providers automatically once the material appears. |
| Actionable upload target on status | Surfacing the resolved cluster and exact Vault mount and path turns a chicken-and-egg ordering problem into a guided user experience. |
| Central user-secret store separate from the per-gateway runtime store and the application plane | Conflating planes would widen each identity's blast radius and couple unrelated rotation and policy lifecycles. The central user-secret store (PLAT-SSS Store 1), the per-gateway OpenShell runtime store (Store 2), and the application plane (Secrets Manager plus ESO) stay isolated by distinct identities, policies, and paths (PLAT-SSS-08). |
| End-user upload: user-OIDC-direct primary, api-server passthrough secondary | The end user who owns the tenant - not a platform or Vault administrator - fills a reference-only `SecretSource`. Primary is the user writing to their own OIDC path-scoped location in the central store (PLAT-SSS-02), so HyperShell is not in the write path. The api-server passthrough (AGT-SEC-14), reusing the write-only `pkg/secretstore` client and the SEC-RBAC-19 tenancy check, remains as a convenience for `hsctl`/API callers; both target the same central-store path. |
