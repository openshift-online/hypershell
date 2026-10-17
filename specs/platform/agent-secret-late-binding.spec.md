# Agent Secret Late Binding

**Date:** 2026-10-17
**Status:** Active
**Jira:** HYPERSHELL-408
**Related:** [`data-model.spec.md`](./data-model.spec.md) (SecretSource model, Secret Submission and Storage, AgentRuntime provisioning), [`control-plane.spec.md`](./control-plane.spec.md) (AgentRuntime Reconciliation, Declarative `.hypershell` Import Reconciliation), [`security/rbac-enforcement.spec.md`](../security/rbac-enforcement.spec.md), [`standards/security/security.spec.md`](../standards/security/security.spec.md) (Secret Store Access Is Single-Directional per Component), [`openshell-gateway-credentials.spec.md`](./openshell-gateway-credentials.spec.md) (the separate gateway-provider Vault plane)

## Purpose

A declarative `.hypershell` agent fleet is imported on the hub before any Gateway
exists, yet an agent's secrets are consumed on whichever `ManagedCluster` its
Gateway is later placed on. The destination is therefore unknown at the moment a
`SecretSource` is declared. This spec defines the late-binding reference model that
removes that ordering constraint: a `SecretSource` is declared by name as a value-less
reference at import time, the value is supplied later directly into the assigned
cluster's local Vault, and the agent converges the moment the value appears. The model
is distributed and eventually correct: the plaintext never touches the hub, no
component reads a global runtime store, and every runtime read is cluster-local.

This spec also governs the agent-secret Vault plane (the convention-addressed,
per-cluster store holding `secret/data/hypershell/secret-sources/*`), which is a
SEPARATE secret plane from the gateway-provider Vault and from application secrets
(Secrets Manager plus External Secrets Operator). The three planes SHALL NOT be
conflated.

## Requirements

### Requirement: Two Secret Resolution Paths

HyperShell SHALL resolve secrets through two distinct, non-overlapping paths, and
every `SecretSource` consumer SHALL use exactly one of them. The two paths SHALL NOT
be conflated and SHALL NOT be collapsed onto one shared store address in the general
multi-cluster fleet.

1. **Hub-side write-then-record path.** For a secret legitimately submitted at the
   hub, the api-server write-only `pkg/secretstore` client writes the value to the
   hub-reachable store and records only the reference (the
   [`data-model.spec.md`](./data-model.spec.md) "Secret Submission and Storage" flow).
   This path serves:
   - the `Repository.secret_source_id` git credential the `RepositoryReconciler`
     reads hub-side to fetch a private `.hypershell` subtree (see
     [`control-plane.spec.md`](./control-plane.spec.md), "Declarative `.hypershell`
     Import Reconciliation"), and
   - the co-located / single-cluster deployment (for example `hyp0`, where the
     api-server, the control plane, and one Vault share a single cluster), in which
     the api-server write client and that cluster's control-plane read client MAY
     share one `HYPERSHELL_SECRET_STORE_*` backend and address because they are the
     same cluster's store.

2. **Distributed AgentRuntime late-binding path.** For an `AgentRuntime`
   `SecretSource` value in a multi-cluster fleet, the value SHALL be written DIRECTLY
   into the ASSIGNED cluster's local Vault by that cluster's per-cluster writer
   identity (see "Scoped Per-Cluster Writer Access"), and the control plane managing
   that cluster SHALL read it from its OWN cluster-local Vault. The value SHALL NOT be
   submitted to the hub api-server and SHALL NOT transit the hub. Each control plane
   SHALL be wired to its own cluster-local Vault; there SHALL be no shared global
   store address across clusters and no hub staging store.

The co-located case is the single point where the two paths coincide, and it does so
only because the hub and the cluster are the same cluster. It SHALL NOT be read as
licensing one global store that a hub api-server writes and every spoke reads.

#### Scenario: Repository git credential resolves hub-side; AgentRuntime value resolves cluster-local

- GIVEN a multi-cluster fleet with a hub api-server and spoke clusters each running a local Vault
- WHEN the `RepositoryReconciler` needs a `Repository.secret_source_id` git credential
- THEN it SHALL resolve that credential through the hub-reachable store (path 1)
- WHEN instead an `AgentRuntime` `SecretSource` value is consumed on a spoke cluster
- THEN the spoke's control plane SHALL read it from that cluster's local Vault (path 2)
- AND the `AgentRuntime` value SHALL NOT be written to or read from a hub store

### Requirement: Reference-Only SecretSource Submission

A `SecretSource` SHALL be a first-class, valid resource with NO value supplied at
creation. On the api-server create path the secret value SHALL be optional: when a
value is supplied the api-server writes it to the configured secret store through its
write-only `pkg/secretstore` client and records only the reference, exactly as
[`data-model.spec.md`](./data-model.spec.md) ("Secret Submission and Storage")
defines; when no value is supplied the api-server SHALL persist the `SecretSource`
reference alone and SHALL perform no `pkg/secretstore` write.

A reference-only `SecretSource` SHALL carry its convention path so the value can be
uploaded later to a known location. When the create request omits `path`, the
api-server SHALL derive it from the submission identity as
`hypershell/secret-sources/<agent_runtime_id>/<name>` (the same deterministic
location a value-bearing submission is written to). The derived path SHALL be stable
across re-submission of the same `SecretSource`.

In the distributed late-binding flow (path 2 of "Two Secret Resolution Paths") the
`AgentRuntime` `SecretSource` value SHALL NOT be submitted to the hub api-server at
all: the reference is declared value-less, and the value is uploaded later directly
into the assigned cluster's local Vault by the per-cluster writer. The api-server
write-then-record path (path 1) carries a value only when a secret is legitimately
submitted at the hub - the `Repository` git credential or the co-located single-cluster
deployment - which is why "plaintext never touches the hub" holds for distributed
`AgentRuntime` secrets.

#### Scenario: Create a value-less SecretSource reference

- GIVEN a `SecretSource` create request with `name`, `agent_runtime_id`, `purpose`, and `backend` and NO `value`
- WHEN the api-server accepts the request
- THEN it SHALL persist the `SecretSource` reference with `path = hypershell/secret-sources/<agent_runtime_id>/<name>`
- AND it SHALL NOT perform any `pkg/secretstore` write
- AND the created `SecretSource` SHALL be a valid, consumable resource

#### Scenario: Hub-submitted value follows the write-then-record flow

- GIVEN a secret legitimately submitted at the hub (a `Repository` git credential, or any secret in a co-located single-cluster deployment)
- WHEN a value is supplied through the api-server secret-submission path
- THEN the api-server SHALL write the value to the store at the `SecretSource` convention path FIRST
- AND it SHALL NOT persist the raw value in the database and SHALL NOT return it on any read

#### Scenario: Distributed AgentRuntime value is never submitted to the api-server

- GIVEN a reference-only `SecretSource` consumed by an `AgentRuntime` placed on a spoke cluster
- WHEN the value is provided
- THEN it SHALL be uploaded directly into the assigned cluster's local Vault by the per-cluster writer
- AND it SHALL NOT be submitted to the hub api-server and SHALL NOT transit the hub

### Requirement: Distributed Per-Cluster Runtime Placement

The referenced value SHALL be read and placed cluster-locally. When the control plane
reconciles an `AgentRuntime` whose effective cluster is its own `cluster_id`, it SHALL
read each `SecretSource` value from the LOCAL Vault of that assigned cluster (the
convention-addressed, per-cluster agent-secret Vault) through its read-only
`pkg/secretstore` client and SHALL place the value directly into the Kubernetes
`Secret` it creates on that cluster. There SHALL be no hub staging store, no
plaintext transfer through the hub, and no global runtime reader that every control
plane consults. Every runtime read SHALL be cluster-local.

The agent-secret Vault plane (holding `secret/data/hypershell/secret-sources/*`) SHALL
be a SEPARATE secret plane from the gateway-provider Vault defined by
[`openshell-gateway-credentials.spec.md`](./openshell-gateway-credentials.spec.md) and
from the application-secret plane (Secrets Manager plus External Secrets Operator). The
three planes SHALL NOT share paths, policies, or identities and SHALL NOT be conflated.
As with all `SecretSource` placement, the control plane SHALL place the Kubernetes
`Secret` directly and SHALL NOT create an `ExternalSecret` and SHALL NOT depend on
External Secrets Operator.

#### Scenario: Value is read from the assigned cluster's local Vault

- GIVEN an `AgentRuntime` placed on `cluster_id: Y` with a `SecretSource` reference
- WHEN the control plane registered as `Y` reconciles the `AgentRuntime` and the value is present in cluster `Y`'s local Vault
- THEN it SHALL read the value from cluster `Y`'s local Vault and place it directly into the Kubernetes `Secret`
- AND no hub store and no other cluster's Vault SHALL be read

#### Scenario: No plaintext is ever written to a hub or global store

- GIVEN a reference-only `SecretSource`
- WHEN its value is supplied and later consumed
- THEN the plaintext SHALL exist only in the assigned cluster's local Vault and the Kubernetes `Secret` placed on that cluster
- AND no hub-side or global store SHALL hold the plaintext

### Requirement: SecretPending Condition and Self-Healing Retry

When a referenced `SecretSource` value is absent from the assigned cluster's local
Vault, the control-plane reconcile SHALL treat this as an expected waiting state, not
a failure. It SHALL:

- set a durable, observable `SecretPending` status condition on the owning
  `AgentRuntime` (and surface it per pending `SecretSource`),
- NOT place an empty or partial credential,
- NOT mark the `AgentRuntime` workspace `Ready` (the runtime stays in `Provisioning`,
  not `Running`),
- NOT crash-loop or exit, and
- retry idempotently on the normal reconcile cadence, converging the moment the value
  appears, with no manual kick (reconcile, do not create-or-skip).

An absent referenced value (`SecretPending`) SHALL be distinct from a store read
failure of a value expected to exist. A reachable store that reports the value missing
SHALL yield `SecretPending`; a store that is unreachable or errors SHALL continue to
degrade the `AgentRuntime` as [`data-model.spec.md`](./data-model.spec.md)
("AgentRuntime Status Lifecycle") defines. `SecretPending` SHALL NOT transition the
`AgentRuntime` to `Degraded`.

Placement SHALL remain one-shot in the sense of
[`security/security.spec.md`](../standards/security/security.spec.md): the control
plane writes the Kubernetes `Secret` once, when the value first appears. Retrying a
reconcile while the value is still absent is convergence toward first placement, not
an ESO-style periodic resync of an already-placed value.

#### Scenario: Missing value sets SecretPending and holds, not Ready

- GIVEN an `AgentRuntime` whose `SecretSource` value has not yet been uploaded to the assigned cluster's Vault
- WHEN the control plane reconciles the `AgentRuntime`
- THEN it SHALL set a `SecretPending` condition naming the pending `SecretSource`
- AND it SHALL NOT place an empty credential and SHALL NOT mark the workspace `Ready`
- AND it SHALL NOT crash-loop

#### Scenario: Converges the moment the value appears

- GIVEN an `AgentRuntime` reporting `SecretPending`
- WHEN the operator writes the value into the assigned cluster's Vault at the given path
- THEN the next reconcile SHALL read the value, place the Kubernetes `Secret`, clear `SecretPending`, and converge the `AgentRuntime` to `Running`
- AND no manual reconcile trigger SHALL be required

#### Scenario: Reachable-but-missing is Pending, unreachable is Degraded

- GIVEN an `AgentRuntime` with a `SecretSource` reference
- WHEN the assigned cluster's Vault is reachable and reports the value missing
- THEN the control plane SHALL report `SecretPending` and SHALL NOT set `Degraded`
- WHEN instead the Vault is unreachable or returns an error
- THEN the control plane SHALL set `Degraded` and surface the failing `SecretSource` name

### Requirement: Discoverable Upload Target

The platform SHALL make each pending secret actionable. On the
`AgentRuntime`/`SecretSource` status the platform SHALL surface the resolved
`ManagedCluster` AND the exact Vault mount plus path at which the value must be
uploaded. The `SecretPending` condition message SHALL read as an actionable next step
(for example, `write <secret-source-name> to <mount>/<path> on cluster <cluster-name>`)
rather than a bare error.

#### Scenario: SecretPending names the cluster and the exact upload path

- GIVEN an `AgentRuntime` reporting `SecretPending` for a `SecretSource`
- WHEN a user reads the `AgentRuntime` status
- THEN the status SHALL identify the resolved `ManagedCluster` by name
- AND it SHALL state the exact Vault mount and path to upload the value to
- AND the condition message SHALL be phrased as an instruction, not a bare error

### Requirement: Scoped Per-Cluster Writer Access

Uploading a value into a cluster's agent-secret Vault SHALL use a per-cluster,
least-privilege WRITER path and identity, separate from any reader identity. The
writer identity SHALL be authorized only to write under
`secret/data/hypershell/secret-sources/*` on that cluster's Vault and SHALL NOT be
granted read access to placed credentials or write access to the gateway-provider
Vault plane. The platform SHALL document (and provision) this writer path and identity
so a human or automation can drop the value in. The gateway-provider Vault policy is
namespace-templated for the gateway ServiceAccount to READ its own secrets; the
agent-secret writer path is a distinct, separately-scoped grant.

#### Scenario: Writer identity is confined to the agent-secret path

- GIVEN the per-cluster writer identity for the agent-secret Vault plane
- WHEN it authenticates to that cluster's Vault
- THEN it SHALL be permitted to write under `secret/data/hypershell/secret-sources/*`
- AND it SHALL NOT be permitted to read placed credentials or to write to the gateway-provider Vault plane

### Requirement: First-Placement Rotation Scope

Placement SHALL occur on the FIRST appearance of a `SecretSource` value. Re-placing
the Kubernetes `Secret` when the stored value LATER changes (rotation) is OUT OF SCOPE
of this spec and SHALL be treated as a separate, explicit decision. This spec SHALL
NOT be read as requiring any periodic resync of a placed value; the retry behavior in
"SecretPending Condition and Self-Healing Retry" converges toward first placement only.

#### Scenario: Later value change is not auto-re-placed

- GIVEN an `AgentRuntime` whose `SecretSource` value was already placed as a Kubernetes `Secret`
- WHEN the stored value is subsequently changed in the cluster's Vault
- THEN the control plane SHALL NOT automatically re-place the Kubernetes `Secret` under this spec
- AND re-placement on change SHALL be governed by a separate, explicit rotation decision

### Requirement: Import Reconciler Enablement

The declarative `.hypershell` import reconciler (`RepositoryReconciler`,
[`control-plane.spec.md`](./control-plane.spec.md), "Declarative `.hypershell` Import
Reconciliation") SHALL be enablable through the configuration flag
`HYPERSHELL_REPOSITORY_IMPORT_ENABLED`. When the flag is unset or `false` the control
plane SHALL log that the import reconciler is disabled and SHALL NOT run the import
loop; when it is `true` the control plane SHALL run the import reconcile loop. Enabling
the import is configuration, not a code change (separate configuration from code).

#### Scenario: Import runs only when enabled

- GIVEN a control plane with `HYPERSHELL_REPOSITORY_IMPORT_ENABLED=false`
- WHEN the control plane starts
- THEN it SHALL log that the import reconciler is disabled and SHALL NOT run the import loop
- WHEN instead `HYPERSHELL_REPOSITORY_IMPORT_ENABLED=true`
- THEN it SHALL run the import reconcile loop

### Requirement: Secret Store Wiring on Both Components

The secret store SHALL be wired through the `HYPERSHELL_SECRET_STORE_*` configuration,
keeping the single-directional split from
[`security/security.spec.md`](../standards/security/security.spec.md) (api-server
write-only, control plane read-only). The wiring SHALL follow the two resolution paths
above, not a single shared global store:

- The api-server SHALL configure its WRITE client from `HYPERSHELL_SECRET_STORE_*` for
  the hub-side write-then-record path (path 1): the `Repository` git credential and,
  in a co-located deployment, locally-submitted secrets.
- Each control plane SHALL configure its READ client from `HYPERSHELL_SECRET_STORE_*`
  pointing at the store it must read: in the general multi-cluster fleet that is its
  OWN cluster-local Vault (path 2, `AgentRuntime` `SecretSource` placement). The hub
  control plane additionally reads the hub-reachable store for the `Repository` git
  credential (path 1).
- The api-server write client and a given control plane's read client MAY share one
  `HYPERSHELL_SECRET_STORE_BACKEND` and address ONLY when they are co-located (the
  same cluster's store). In the general fleet they SHALL point at different, per-cluster
  stores; the `AgentRuntime` `SecretSource` value SHALL NOT be resolvable through a
  single store that the hub api-server writes and every spoke reads.

When the backend is unset, the api-server SHALL reject secret submissions loudly and a
control plane SHALL fail a `SecretSource` placement loudly rather than place an empty
credential (no silent degradation). A `Repository` that declares a `secret_source_id`
SHALL be resolvable only when the hub-side store (path 1) is wired.

#### Scenario: Unwired store fails loudly on both directions

- GIVEN `HYPERSHELL_SECRET_STORE_BACKEND` is unset
- WHEN a user submits a secret value to the api-server
- THEN the api-server SHALL reject the submission with a clear configuration error
- AND when a control plane attempts to read a `SecretSource` value
- THEN it SHALL fail loudly and SHALL NOT place an empty credential

#### Scenario: Fleet control plane reads its own cluster-local store

- GIVEN a multi-cluster fleet where the hub api-server and a spoke control plane are NOT co-located
- WHEN the spoke control plane is wired through `HYPERSHELL_SECRET_STORE_*`
- THEN its read client SHALL point at its own cluster-local Vault
- AND the `AgentRuntime` `SecretSource` value SHALL NOT be resolved through a store the hub api-server writes

### Requirement: Agent-Secret Vault Client Capabilities

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

#### Scenario: Client connects to a privately-signed cluster Vault

- GIVEN a cluster Vault served with a private CA and configured for Kubernetes auth
- WHEN the control plane reads a `SecretSource` value from it
- THEN the client SHALL trust the configured private-CA bundle and complete the TLS handshake
- AND it SHALL authenticate through the Vault Kubernetes auth mount with its ServiceAccount
- AND it SHALL read only under the `secret/data/hypershell/secret-sources/*`-scoped policy

## Design Decisions

| Decision | Rationale |
|----------|-----------|
| Late-binding reference model over hub staging | The destination Vault is unknown at import time because cluster assignment happens only at Gateway provisioning. Declaring a value-less reference and supplying the value later into the assigned cluster's Vault removes all ordering constraints (eventually correct) with no central plaintext store. |
| Distributed per-cluster read, no global runtime store | A single global store every control plane reads from at runtime is a central trust sink with fleet-wide blast radius and cross-boundary auth. Cluster-local reads keep the plaintext on the cluster that consumes it. |
| Two explicit resolution paths, coinciding only when co-located | The hub-side write-then-record path (Repository git credential, co-located single-cluster deployment) and the distributed AgentRuntime late-binding path are different stores with different writers. Stating them separately prevents an implementer from collapsing both onto one shared store address, which would reintroduce the forbidden global reader. The paths coincide only in the co-located case, and only because the hub and cluster are the same cluster. |
| `SecretPending` is a benign waiting condition, not `Degraded` | A reference whose value is not yet uploaded is an expected state during rollout, not a fault. Distinguishing reachable-but-missing (Pending) from unreachable/error (Degraded) gives operators an actionable next step without false alarms, and lets the agent converge automatically once the value appears. |
| Actionable upload target on status | Surfacing the resolved cluster and exact Vault mount and path turns a chicken-and-egg ordering problem into a guided user experience. |
| Agent-secret Vault plane separate from gateway-provider and application planes | Conflating planes would widen each identity's blast radius and couple unrelated rotation and policy lifecycles. A dedicated `secret/data/hypershell/secret-sources/*` path with its own least-privilege writer keeps the three planes isolated. |
| First-placement only; rotation deferred | Re-placing on value change is a distinct lifecycle concern (resync, restart semantics, drift) that deserves its own decision rather than being implied by the placement path. |
