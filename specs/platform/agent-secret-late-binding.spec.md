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
reference at import time, the durable material is supplied later directly into the
assigned cluster's local Vault, and the agent realizes its providers the moment the
material appears. The model is distributed and eventually correct: the material never
touches the hub, no component reads a global runtime store, and every runtime read is
cluster-local.

This spec also governs the agent-secret Vault plane (the convention-addressed,
per-cluster store holding `secret/data/hypershell/secret-sources/*`), which is a
SEPARATE secret plane from the gateway-provider Vault and from application secrets
(Secrets Manager plus External Secrets Operator). The three planes SHALL NOT be
conflated.

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
materialize it. The durable material SHALL be read cluster-local (see "Distributed
Per-Cluster Material Read") and supplied to OpenShell as provider configuration; the
usable credential lives inside OpenShell, not in a `Secret` the control plane placed.

The coordinator's own `gateway-identity` credential - the OIDC identity the
coordinator `CronJob` uses to authenticate TO the gateway so it can reach its
providers - is the one credential the coordinator consumes directly rather than as a
provider, and it is read cluster-local and wired as coordinator environment per
[`data-model.spec.md`](./data-model.spec.md) ("SecretSource"). It is NOT a provider
and is NOT a rendered agent credential `Secret` for provider material.

#### Scenario: AGT-SEC-01.01 -- ProviderBindings become OpenShell providers, no credential Secret

- GIVEN an `AgentRuntime` whose `.hypershell` declares `ProviderBinding`s with `refresh_strategy` values
- WHEN the control plane reconciles it and the referenced material is present in the assigned cluster's local Vault
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

### Requirement: AGT-SEC-03 -- Two Material Resolution Paths

HyperShell SHALL resolve credential material through two distinct, non-overlapping
paths, and every `SecretSource` consumer SHALL use exactly one of them. The two paths
SHALL NOT be conflated and SHALL NOT be collapsed onto one shared store address in the
general multi-cluster fleet.

1. **Hub-side write-then-record path.** For material legitimately submitted at the
   hub, the api-server write-only `pkg/secretstore` client writes the value to the
   hub-reachable store and records only the reference (the
   [`data-model.spec.md`](./data-model.spec.md) "Secret Submission and Storage" flow).
   This path serves:
   - the `Repository` import credential the `RepositoryReconciler` reads hub-side to
     fetch a private `.hypershell` subtree (see "GitHub App Credentials via
     app-installation-token" below and [`control-plane.spec.md`](./control-plane.spec.md),
     "Declarative `.hypershell` Import Reconciliation"); the material stored is the
     durable GitHub App PEM (plus `app_id` / `installation_id`), not a static
     installation token, and
   - the co-located / single-cluster deployment (for example `hyp0`, where the
     api-server, the control plane, and one Vault share a single cluster), in which
     the api-server write client and that cluster's control-plane read client MAY
     share one `HYPERSHELL_SECRET_STORE_*` backend and address because they are the
     same cluster's store.

2. **Distributed AgentRuntime late-binding path.** For an `AgentRuntime`
   `SecretSource` material in a multi-cluster fleet, the durable material SHALL be
   written DIRECTLY into the ASSIGNED cluster's local Vault by that cluster's
   per-cluster writer identity (see "Scoped Per-Cluster Writer Access"), and the
   control plane managing that cluster SHALL read it from its OWN cluster-local Vault
   to configure the agent's OpenShell providers. The material SHALL NOT be submitted
   to the hub api-server and SHALL NOT transit the hub. Each control plane SHALL be
   wired to its own cluster-local Vault; there SHALL be no shared global store address
   across clusters and no hub staging store.

The co-located case is the single point where the two paths coincide, and it does so
only because the hub and the cluster are the same cluster. It SHALL NOT be read as
licensing one global store that a hub api-server writes and every spoke reads.

#### Scenario: AGT-SEC-03.01 -- Repository credential resolves hub-side; AgentRuntime material resolves cluster-local

- GIVEN a multi-cluster fleet with a hub api-server and spoke clusters each running a local Vault
- WHEN the `RepositoryReconciler` needs the `Repository` import credential
- THEN it SHALL resolve that material through the hub-reachable store (path 1)
- WHEN instead an `AgentRuntime` `SecretSource` material is consumed on a spoke cluster
- THEN the spoke's control plane SHALL read it from that cluster's local Vault (path 2)
- AND the `AgentRuntime` material SHALL NOT be written to or read from a hub store

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

In the distributed late-binding flow (path 2 of "Two Material Resolution Paths") the
`AgentRuntime` `SecretSource` material SHALL NOT be submitted to the hub api-server at
all: the reference is declared value-less, and the material is uploaded later directly
into the assigned cluster's local Vault by the per-cluster writer. The api-server
write-then-record path (path 1) carries material only when it is legitimately
submitted at the hub - the `Repository` import credential or the co-located
single-cluster deployment - which is why "material never touches the hub" holds for
distributed `AgentRuntime` credentials.

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

#### Scenario: AGT-SEC-04.03 -- Distributed AgentRuntime material is never submitted to the api-server

- GIVEN a reference-only `SecretSource` consumed by an `AgentRuntime` placed on a spoke cluster
- WHEN the material is provided
- THEN it SHALL be uploaded directly into the assigned cluster's local Vault by the per-cluster writer
- AND it SHALL NOT be submitted to the hub api-server and SHALL NOT transit the hub

### Requirement: AGT-SEC-05 -- Distributed Per-Cluster Material Read

The referenced material SHALL be read cluster-locally and used to configure the
agent's OpenShell providers. When the control plane reconciles an `AgentRuntime` whose
effective cluster is its own `cluster_id`, it SHALL read each referenced
`SecretSource` material from the LOCAL Vault of that assigned cluster (the
convention-addressed, per-cluster agent-secret Vault) through its read-only
`pkg/secretstore` client, and SHALL supply that material to OpenShell as provider
configuration over the gateway gRPC API. The control plane SHALL NOT render the
material into a raw Kubernetes `Secret` as the agent credential. There SHALL be no hub
staging store, no material transfer through the hub, and no global runtime reader that
every control plane consults. Every runtime read SHALL be cluster-local.

The agent-secret Vault plane (holding `secret/data/hypershell/secret-sources/*`) SHALL
be a SEPARATE secret plane from the gateway-provider Vault defined by
[`openshell-gateway-credentials.spec.md`](./openshell-gateway-credentials.spec.md) and
from the application-secret plane (Secrets Manager plus External Secrets Operator). The
three planes SHALL NOT share paths, policies, or identities and SHALL NOT be conflated.

#### Scenario: AGT-SEC-05.01 -- Material is read from the assigned cluster's local Vault to configure providers

- GIVEN an `AgentRuntime` placed on `cluster_id: Y` with a `SecretSource` reference
- WHEN the control plane registered as `Y` reconciles the `AgentRuntime` and the material is present in cluster `Y`'s local Vault
- THEN it SHALL read the material from cluster `Y`'s local Vault and configure the OpenShell provider with it
- AND no hub store and no other cluster's Vault SHALL be read
- AND no raw credential `Secret` SHALL be rendered by the control plane

#### Scenario: AGT-SEC-05.02 -- No material is ever written to a hub or global store

- GIVEN a reference-only `SecretSource`
- WHEN its material is supplied and later consumed
- THEN the material SHALL exist only in the assigned cluster's local Vault (and inside the OpenShell provider configured from it)
- AND no hub-side or global store SHALL hold the material

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

- GIVEN an `AgentRuntime` whose `SecretSource` material has not yet been uploaded to the assigned cluster's Vault
- WHEN the control plane reconciles the `AgentRuntime`
- THEN it SHALL set a `SecretPending` condition naming the pending `SecretSource`
- AND it SHALL NOT create the dependent OpenShell provider and SHALL NOT mark the workspace `Ready`
- AND it SHALL NOT crash-loop

#### Scenario: AGT-SEC-06.02 -- Converges the moment the material appears

- GIVEN an `AgentRuntime` reporting `SecretPending`
- WHEN the operator writes the material into the assigned cluster's Vault at the given path
- THEN the next reconcile SHALL read the material, realize the OpenShell provider, clear `SecretPending`, and converge the `AgentRuntime` to `Running`
- AND no manual reconcile trigger SHALL be required

#### Scenario: AGT-SEC-06.03 -- Reachable-but-missing is Pending, unreachable is Degraded

- GIVEN an `AgentRuntime` with a `SecretSource` reference
- WHEN the assigned cluster's Vault is reachable and reports the material missing
- THEN the control plane SHALL report `SecretPending` and SHALL NOT set `Degraded`
- WHEN instead the Vault is unreachable or returns an error
- THEN the control plane SHALL set `Degraded` and surface the failing `SecretSource` name

### Requirement: AGT-SEC-07 -- Discoverable Upload Target

The platform SHALL make each pending credential actionable. On the
`AgentRuntime`/`SecretSource` status the platform SHALL surface the resolved
`ManagedCluster` AND the exact Vault mount plus path at which the material must be
uploaded. The `SecretPending` condition message SHALL read as an actionable next step
(for example, `write <secret-source-name> to <mount>/<path> on cluster <cluster-name>`)
rather than a bare error.

#### Scenario: AGT-SEC-07.01 -- SecretPending names the cluster and the exact upload path

- GIVEN an `AgentRuntime` reporting `SecretPending` for a `SecretSource`
- WHEN a user reads the `AgentRuntime` status
- THEN the status SHALL identify the resolved `ManagedCluster` by name
- AND it SHALL state the exact Vault mount and path to upload the material to
- AND the condition message SHALL be phrased as an instruction, not a bare error

### Requirement: AGT-SEC-08 -- Scoped Per-Cluster Writer Access

Uploading material into a cluster's agent-secret Vault SHALL use a per-cluster,
least-privilege WRITER path and identity, separate from any reader identity. The
writer identity SHALL be authorized only to write under
`secret/data/hypershell/secret-sources/*` on that cluster's Vault and SHALL NOT be
granted read access to provider material or write access to the gateway-provider Vault
plane. The platform SHALL document (and provision) this writer path and identity so a
human or automation can drop the material in. The gateway-provider Vault policy is
namespace-templated for the gateway ServiceAccount to READ its own secrets; the
agent-secret writer path is a distinct, separately-scoped grant.

#### Scenario: AGT-SEC-08.01 -- Writer identity is confined to the agent-secret path

- GIVEN the per-cluster writer identity for the agent-secret Vault plane
- WHEN it authenticates to that cluster's Vault
- THEN it SHALL be permitted to write under `secret/data/hypershell/secret-sources/*`
- AND it SHALL NOT be permitted to read provider material or to write to the gateway-provider Vault plane

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
- WHEN the control plane reconciles it with the PEM present in the assigned cluster's Vault
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
write-only, control plane read-only). The wiring SHALL follow the two resolution paths
above, not a single shared global store:

- The api-server SHALL configure its WRITE client from `HYPERSHELL_SECRET_STORE_*` for
  the hub-side write-then-record path (path 1): the `Repository` import credential and,
  in a co-located deployment, locally-submitted material.
- Each control plane SHALL configure its READ client from `HYPERSHELL_SECRET_STORE_*`
  pointing at the store it must read: in the general multi-cluster fleet that is its
  OWN cluster-local Vault (path 2, `AgentRuntime` provider material). The hub control
  plane additionally reads the hub-reachable store for the `Repository` import
  credential (path 1).
- The api-server write client and a given control plane's read client MAY share one
  `HYPERSHELL_SECRET_STORE_BACKEND` and address ONLY when they are co-located (the
  same cluster's store). In the general fleet they SHALL point at different, per-cluster
  stores; the `AgentRuntime` provider material SHALL NOT be resolvable through a single
  store that the hub api-server writes and every spoke reads.

When the backend is unset, the api-server SHALL reject secret submissions loudly and a
control plane SHALL fail provider realization loudly rather than configure a provider
with empty material (no silent degradation). A `Repository` that declares an import
credential SHALL be resolvable only when the hub-side store (path 1) is wired.

#### Scenario: AGT-SEC-12.01 -- Unwired store fails loudly on both directions

- GIVEN `HYPERSHELL_SECRET_STORE_BACKEND` is unset
- WHEN a user submits credential material to the api-server
- THEN the api-server SHALL reject the submission with a clear configuration error
- AND when a control plane attempts to read `SecretSource` material to realize a provider
- THEN it SHALL fail loudly and SHALL NOT configure a provider with empty material

#### Scenario: AGT-SEC-12.02 -- Fleet control plane reads its own cluster-local store

- GIVEN a multi-cluster fleet where the hub api-server and a spoke control plane are NOT co-located
- WHEN the spoke control plane is wired through `HYPERSHELL_SECRET_STORE_*`
- THEN its read client SHALL point at its own cluster-local Vault
- AND the `AgentRuntime` provider material SHALL NOT be resolved through a store the hub api-server writes

### Requirement: AGT-SEC-13 -- Agent-Secret Vault Client Capabilities

The secret-store Vault client SHALL be able to reach a per-cluster Vault as that
cluster actually serves it. Observable behavior:

- **Private-CA trust.** The client SHALL complete a TLS handshake against a Vault
  served with a PRIVATE certificate authority (for example the cluster Vault's
  `openshell-gateway-vault-tls` serving CA), using a configurable trust bundle rather
  than the system trust store alone. A privately-signed Vault SHALL NOT be rejected for
  an untrusted-CA reason.
- **Cluster auth method.** The client SHALL authenticate using the method that
  cluster's Vault requires. Where the cluster Vault uses Kubernetes auth, the client
  SHALL authenticate with the component ServiceAccount through the Vault Kubernetes auth
  mount; the existing static-token method SHALL remain supported for stores that use it.
- **Scoped policy and path mapping.** The agent-secret plane SHALL be governed by a
  Vault policy and role scoped to the agent-secret path, distinct from the
  gateway-provider Vault policy. The logical, mount-relative path a `SecretSource`
  records is `hypershell/secret-sources/<agent_runtime_id>/<name>`. On a KV v2 engine
  mounted at `secret`, this maps to the physical policy path
  `secret/data/hypershell/secret-sources/*` (`secret` is the KV mount and `data/` is the
  KV v2 API prefix). The write and read clients SHALL apply the SAME mount and `data/`
  prefix, so the stored logical path and the policy path refer to the same entries.

These capabilities SHALL be driven by `HYPERSHELL_SECRET_STORE_*` configuration, not by
code changes. (The current implementation surface is the `pkg/secretstore` write and
read clients.)

#### Scenario: AGT-SEC-13.01 -- Client connects to a privately-signed cluster Vault

- GIVEN a cluster Vault served with a private CA and configured for Kubernetes auth
- WHEN the control plane reads `SecretSource` material from it
- THEN the client SHALL trust the configured private-CA bundle and complete the TLS handshake
- AND it SHALL authenticate through the Vault Kubernetes auth mount with its ServiceAccount
- AND it SHALL read only under the `secret/data/hypershell/secret-sources/*`-scoped policy

## Design Decisions

| Decision | Rationale |
|----------|-----------|
| Realize credentials as OpenShell providers, not rendered raw Secrets | A rendered raw `Secret` is inert, so whoever rendered it must rotate it - the root cause of the stale-token problem in the superseded model. An OpenShell provider is live: given durable material plus a refresh strategy, OpenShell mints and rotates the usable credential for its native strategies, removing any need for a control-plane rotator for those strategies; for GitHub App credentials a bounded GitHub-specific control-plane rotator is required because the installed OpenShell CLI has no `app-installation-token` refresh strategy (see the next row). It is already the declared model (`ProviderBinding.refresh_strategy`) and the control plane's existing gateway provider-provisioning pattern. |
| Rotation ownership split by strategy | Rotation belongs where the live credential can be minted. For strategies OpenShell supports natively, OpenShell mints and rotates, keeping the control plane a configurator rather than a credential issuer. For GitHub App credentials the installed OpenShell CLI has no `app-installation-token` refresh strategy, so a bounded control-plane GitHub rotator mints the installation token from the PEM and updates the provider credential. Splitting ownership this way matches what OpenShell can actually do rather than asserting an OpenShell capability that is not confirmed. |
| SecretSource holds durable material, not a usable credential | The durable material (GitHub App PEM, Vertex SA key, gateway identity) is long-lived and rotation-free; the usable, short-lived credential is derived from it on demand. Storing only the material, never the usable credential, is what lets the rotation owner (OpenShell for its native strategies, the bounded control-plane GitHub rotator for GitHub App credentials) mint the usable credential when needed, and keeps no rotatable secret at rest in a control-plane-placed `Secret`. |
| Late-binding reference model over hub staging | The destination Vault is unknown at import time because cluster assignment happens only at Gateway provisioning. Declaring a value-less reference and supplying the material later into the assigned cluster's Vault removes all ordering constraints (eventually correct) with no central material store. |
| Distributed per-cluster read, no global runtime store | A single global store every control plane reads from at runtime is a central trust sink with fleet-wide blast radius and cross-boundary auth. Cluster-local reads keep the material on the cluster that consumes it. |
| Two explicit resolution paths, coinciding only when co-located | The hub-side write-then-record path (Repository import credential, co-located single-cluster deployment) and the distributed AgentRuntime late-binding path are different stores with different writers. Stating them separately prevents an implementer from collapsing both onto one shared store address, which would reintroduce the forbidden global reader. The paths coincide only in the co-located case, and only because the hub and cluster are the same cluster. |
| GitHub App via app-installation-token for both agent providers and the Repository import credential | Storing only the durable PEM (plus `app_id` / `installation_id`) and minting the ~1h installation token on demand means there is no static installation token to go stale. The control plane mints the token and sets it on the OpenShell provider (agent providers) or uses it directly (the `RepositoryReconciler` import credential). Unifying both on this one PEM model supersedes the static-token resolver and closes the import-credential rotation gap; the token minting is a bounded, GitHub-specific control-plane rotator. |
| `SecretPending` is a benign waiting condition that gates provider creation, not `Degraded` | A reference whose material is not yet uploaded is an expected state during rollout, not a fault. Distinguishing reachable-but-missing (Pending) from unreachable/error (Degraded) gives operators an actionable next step without false alarms, and lets the agent realize its providers automatically once the material appears. |
| Actionable upload target on status | Surfacing the resolved cluster and exact Vault mount and path turns a chicken-and-egg ordering problem into a guided user experience. |
| Agent-secret Vault plane separate from gateway-provider and application planes | Conflating planes would widen each identity's blast radius and couple unrelated rotation and policy lifecycles. A dedicated `secret/data/hypershell/secret-sources/*` path with its own least-privilege writer keeps the three planes isolated. |
