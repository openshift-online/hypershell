# AgentRuntime Gateway-Side Workspace Provisioning API Specification

**Date:** 2026-10-17
**Status:** Active
**Jira:** HYPERSHELL-410
**Related:** [`data-model.spec.md`](./data-model.spec.md) (AgentRuntime, AgentWorkspace, WorkspaceMembership, ProviderBinding, InferenceRoute kinds; "Gateway-Side Provisioning"), [`control-plane.spec.md`](./control-plane.spec.md) ("AgentRuntime Reconciliation", the Wave B reconciler), [`agent-secret-late-binding.spec.md`](./agent-secret-late-binding.spec.md) (credentials realized as OpenShell providers, OpenShell owns rotation), [`openshell-gateway-database.spec.md`](./openshell-gateway-database.spec.md) (the per-gateway database provisioning channel this contract mirrors), [`openshell-gateway-credentials.spec.md`](./openshell-gateway-credentials.spec.md) (the gateway-provider Vault plane), [`openshell-inference-routing.spec.md`](./openshell-inference-routing.spec.md) (the `inference.local` route this contract configures)

## Purpose

This specification defines the gateway-side provisioning contract that lets the control
plane realize an `AgentRuntime`'s workspace on the OpenShell gateway it is placed on.
It is the contract that #521 requirement #4 (gateway-side provisioning) needs and that
the `ProviderRealizer` seam in the Wave B `AgentRuntimeReconciler` (deferred by the
provider-model work) will implement. Before this spec, the `AgentWorkspace` and
`WorkspaceMembership` kinds were referenced by [`data-model.spec.md`](./data-model.spec.md)
but the gateway API that provisions them on a gateway was unspecified.

An OpenShell gateway is the system of record for what is actually provisioned on it: the
workspace, its memberships, the OpenShell **providers** realized from durable credential
material, and the single inference route that binds `inference.local` to a provider and
model. The HyperShell API server holds the **desired state** of these objects as API
resources; the control plane reconciles that desired state onto the gateway. This spec
defines the gateway-side provisioning API the control plane calls to do so,
**idempotently**, and mirrors the shape of the existing per-gateway database provisioning
channel ([`openshell-gateway-database.spec.md`](./openshell-gateway-database.spec.md)):
a short-lived administrative connection per operation, existence-check-then-act
idempotent reconcile, unconditional cascade cleanup on deletion, retryable errors that
carry context and never leak secrets.

This contract is what lets the control plane realize agent credentials as OpenShell
providers rather than raw Kubernetes `Secret`s, with OpenShell owning rotation
(`agent-secret-late-binding.spec.md`). This spec SHALL remain consistent with that model:
it SHALL NOT reintroduce raw-`Secret` placement of a provider credential, and it SHALL
carry only the durable **material** plus a **refresh strategy** to the gateway, never a
minted, rotatable credential the control plane would have to refresh.

### Scope boundary

- **This spec owns:** the gateway-side provisioning API (its RPCs, request/response
  shapes, idempotency, retry, enforcement, and cascade deletion), the gateway admin
  connection the control plane opens to call it, and the gateway-side representation and
  identifiers of `AgentWorkspace` / `WorkspaceMembership` / realized providers / inference
  route.
- **This spec does not own:** the REST/gRPC API-resource schemas of `AgentWorkspace`,
  `WorkspaceMembership`, `ProviderSpec`, `ProviderBinding`, `InferenceRoute`, and
  `SecretSource` (owned by [`data-model.spec.md`](./data-model.spec.md)); the cluster-side
  scheduling resources (`Namespace`, `ServiceAccount`, `NetworkPolicy`, coordinator
  `CronJob`) the Wave B reconciler applies (owned by
  [`control-plane.spec.md`](./control-plane.spec.md)); the provider-credential Vault plane
  OpenShell reads from (owned by
  [`openshell-gateway-credentials.spec.md`](./openshell-gateway-credentials.spec.md)); and
  the cluster-local material read path (owned by
  [`agent-secret-late-binding.spec.md`](./agent-secret-late-binding.spec.md)).

## Architecture

```
HyperShell API server (desired state, PostgreSQL)
  AgentRuntime --owns--> AgentWorkspace --> WorkspaceMembership[*]
                                       --> ProviderBinding[*] --> ProviderSpec, SecretSource
                                       --> InferenceRoute (0..1)
        │  gRPC watch stream (filtered by cluster_id)
        ▼
Control plane (Wave B AgentRuntimeReconciler, on the assigned ManagedCluster)
  ├── reads SecretSource material cluster-local (read-only pkg/secretstore)
  └── ProviderRealizer (this contract's client)
        │  gateway admin gRPC, short-lived connection per operation
        ▼
OpenShell gateway (system of record for provisioned workspace state)
  ├── AgentWorkspace            (named per AgentRuntime, unique on the gateway)
  ├── WorkspaceMembership[*]    (OIDC subject -> role: admin | member)
  ├── OpenShell provider[*]     (realized from material + refresh_strategy; OpenShell rotates)
  └── InferenceRoute (0..1)     (binds inference.local to a provider + model alias)
```

### Why mirror the database channel

The per-gateway database channel provisions a gateway-scoped external object
(a PostgreSQL database and role) from an administrative connection, idempotently, and
cleans it up unconditionally on gateway deletion. Gateway-side workspace provisioning is
the same class of problem one level up: it provisions gateway-scoped objects (a
workspace and its sub-resources) from an administrative connection to the gateway, must
be idempotent under the reconcile-do-not-create-or-skip convention, and must cascade a
clean deletion. Reusing that shape - connection re-resolved per operation,
existence-check-then-act, structured retryable errors, no secrets in logs or status -
keeps one operational model across both channels and one failure/retry discipline in the
reconciler.

### Where it plugs into the Wave B reconciler

The `AgentRuntimeReconciler` (`control-plane.spec.md`, "AgentRuntime Reconciliation")
applies the cluster-side scheduling resources, then invokes the `ProviderRealizer` seam
to provision the gateway-side workspace objects. The client that implements
`ProviderRealizer` against this contract is what this spec enables; until it is wired the
reconciler holds its transitional behavior. The code/reconcile phase implements the
client and the gateway RPCs; this spec is design-only and defines no Go, proto, or
OpenAPI code.

## Requirements

### Requirement: Gateway Admin Connection

The control plane SHALL provision gateway-side workspace objects over a short-lived
administrative connection to the gateway's provisioning gRPC endpoint, opened per
provisioning operation and closed afterward, mirroring the per-operation admin connection
of [`openshell-gateway-database.spec.md`](./openshell-gateway-database.spec.md). The
control plane SHALL NOT maintain a long-lived gateway admin session and SHALL NOT shell
out to the `openshell` CLI to provision workspaces.

The gateway provisioning endpoint SHALL be resolved from the `Gateway` the `AgentRuntime`
references (its routable in-cluster or ingress gRPC address), re-resolved per operation
rather than cached, so a gateway endpoint change takes effect on the next reconcile
without a controller restart. The connection SHALL authenticate as a platform
administrator identity scoped to workspace provisioning on that gateway; it SHALL NOT use
a workspace member identity and SHALL NOT use the coordinator's own `gateway-identity`.

The administrative identity's credential material SHALL NEVER appear in logs, error
strings, telemetry, Kubernetes Events, or API responses. Error messages MAY name the
gateway and the operation that failed; they SHALL NOT include the credential or the
durable provider material.

#### Scenario: Endpoint re-resolved per operation

- GIVEN a running control plane that has provisioned a workspace on a gateway
- WHEN the gateway's routable gRPC endpoint changes and the reconciler next provisions or updates the workspace
- THEN it SHALL resolve the current endpoint from the `Gateway` resource
- AND no controller restart SHALL be required

#### Scenario: Admin credential never leaks

- GIVEN a gateway admin connection that fails with a transport error embedding the credential
- WHEN the control plane logs the failure and sets the `AgentRuntime` status
- THEN the log line, the status message, and any Event SHALL NOT contain the admin credential or the durable provider material
- AND the message MAY name the gateway and the failed operation

---

### Requirement: Gateway-Side AgentWorkspace Representation and Identity

Each `AgentRuntime` SHALL own exactly one `AgentWorkspace` on its `Gateway`. The
gateway-side workspace SHALL be identified by the owning `agent_runtime_id` and SHALL
carry the `AgentWorkspace.name`, which is unique on the gateway (see Requirement:
Workspace Name Collision Enforcement). The API-resource schema of `AgentWorkspace` and
`WorkspaceMembership` is owned by [`data-model.spec.md`](./data-model.spec.md); this
requirement defines only how those resources are represented and named on the gateway.

Binding the gateway-side workspace to the owning `agent_runtime_id` is what reconciles
idempotency with collision detection: a repeated provisioning call for the SAME
`AgentRuntime` and name is a no-op success, while a DIFFERENT `AgentRuntime` requesting an
already-taken name on the same gateway is a collision.

The gateway-side workspace lifecycle SHALL be create-or-update on reconcile and cascade
delete on `AgentRuntime` deletion (see Requirement: Cascade Deletion Semantics). There is
no independent gateway-side workspace create or delete outside the owning `AgentRuntime`'s
reconcile.

#### Scenario: Workspace carries its owner and name

- GIVEN an `AgentRuntime` whose `AgentWorkspace.name` is `amber-reviews`
- WHEN the control plane provisions the workspace on the gateway
- THEN the gateway-side workspace SHALL be identified by the owning `agent_runtime_id` and named `amber-reviews`
- AND a second reconcile of the same `AgentRuntime` SHALL match the same gateway-side workspace

---

### Requirement: Idempotent Workspace Provisioning API

The gateway provisioning API SHALL expose idempotent create-or-update ("ensure")
operations for the workspace and each sub-resource kind, and the control plane SHALL call
them as part of its normal reconcile loop. Re-running an ensure against an object that
already exists SHALL succeed without error and SHALL update the object's mutable fields in
place (reconcile, do not create-or-skip). The API SHALL provide at least:

| Operation | Purpose | Idempotent behavior |
|---|---|---|
| `EnsureWorkspace` | Create-or-update the `AgentWorkspace` for an `agent_runtime_id` with its name | Absent: create. Present and owned by the same `agent_runtime_id`: no-op success. Present under a different owner: collision (see enforcement) |
| `ReconcileMemberships` | Declaratively set the full `WorkspaceMembership` set (subject, role) for the workspace | Converge to the given set: add missing, update changed roles, remove memberships no longer declared |
| `EnsureProvider` | Create-or-update one OpenShell provider realized from a `ProviderBinding` (its `provider_spec_id`, `refresh_strategy`, and durable material) | Absent: create. Present: update the provider configuration and refresh strategy in place |
| `EnsureInferenceRoute` | Create-or-update the single `InferenceRoute` binding `inference.local` to a provider and model alias | Converge the at-most-one route to the declared provider and model; changing it redirects inference without restarting sandboxes |
| `DeleteWorkspace` | Cascade-delete the workspace and all sub-resources | Unconditional and idempotent (see Cascade Deletion Semantics) |

`ReconcileMemberships` SHALL be declarative over the full set rather than a per-membership
add, so a membership removed from desired state is removed on the gateway, matching the
prune discipline the control plane applies elsewhere. `EnsureInferenceRoute` SHALL enforce
the at-most-one-route-per-workspace invariant on the gateway.

Each ensure RPC request SHALL carry the owning `agent_runtime_id` and the workspace name
so the gateway can bind the operation to the correct workspace. Provider and inference
requests SHALL carry gateway-agnostic fields only (the `ProviderSpec` capability, the
`refresh_strategy` type and its non-secret parameters, the model alias) plus the durable
material for secret-valued fields; they SHALL NOT carry backend-specific field names, which
are the gateway's own translation concern.

#### Scenario: Ensure is idempotent across reconciles

- GIVEN an `AgentRuntime` whose workspace, memberships, providers, and inference route are already provisioned on the gateway
- WHEN the control plane reconciles it again with unchanged desired state
- THEN every ensure call SHALL succeed and make no change
- AND no error SHALL be returned

#### Scenario: Mutable field updated in place

- GIVEN a provisioned `InferenceRoute` bound to model alias `claude-sonnet-4-5`
- WHEN the `AgentRuntime`'s route is changed to a different model alias
- THEN `EnsureInferenceRoute` SHALL update the single route in place
- AND running sandboxes in the workspace SHALL NOT be restarted

#### Scenario: Removed membership is pruned

- GIVEN a workspace with memberships for subjects `A` (admin) and `B` (member)
- WHEN the desired membership set no longer includes `B`
- THEN `ReconcileMemberships` SHALL remove `B`'s membership on the gateway
- AND SHALL leave `A`'s admin membership in place

---

### Requirement: Provider Realization Coherence With the Late-Binding Model

The control plane SHALL realize each `ProviderBinding` as an OpenShell **provider** on
the agent's workspace through `EnsureProvider`, from the binding's `provider_spec_id`, its
`refresh_strategy`, and the durable **material** referenced through its `SecretSource`s,
consistent with [`agent-secret-late-binding.spec.md`](./agent-secret-late-binding.spec.md).
It SHALL read that material cluster-local from the assigned cluster's local store through
its read-only `pkg/secretstore` client and supply it to the gateway as provider
configuration.

The control plane SHALL NOT render the provider credential as a raw Kubernetes `Secret`,
SHALL NOT create an `ExternalSecret`, and SHALL NOT depend on External Secrets Operator.
It SHALL send only the durable material (for example a GitHub App PEM plus `app_id` /
`installation_id`, or a Vertex service-account key) and the `refresh_strategy` type
(`app-installation-token`, `google-service-account-jwt`, `static-token`); OpenShell SHALL
mint and rotate the usable credential. The control plane SHALL NOT mint, refresh, or
rotate the usable credential and SHALL NOT run any control-plane token rotator.

When referenced `SecretSource` material is not yet present in the assigned cluster's
reachable local store, provider realization SHALL hold under a `SecretPending` condition
rather than fail: the control plane SHALL NOT call `EnsureProvider` with empty or partial
material, SHALL NOT mark the workspace `Ready`, and SHALL converge the moment the material
appears (see [`agent-secret-late-binding.spec.md`](./agent-secret-late-binding.spec.md),
"SecretPending Condition Gates Provider Creation").

#### Scenario: Provider realized from durable material, no raw Secret

- GIVEN a `ProviderBinding` with `refresh_strategy.type = app-installation-token` whose `SecretSource` material (a GitHub App PEM) is present in the assigned cluster's local store
- WHEN the control plane reconciles the owning `AgentRuntime`
- THEN it SHALL call `EnsureProvider` with the PEM, `app_id`, `installation_id`, and the `app-installation-token` refresh strategy
- AND OpenShell SHALL mint and rotate the installation token thereafter
- AND the control plane SHALL render no raw Kubernetes `Secret` and create no `ExternalSecret` for the credential

#### Scenario: Missing material holds provider creation, does not fail

- GIVEN a `ProviderBinding` whose referenced material has not yet been uploaded to the assigned cluster's reachable local store
- WHEN the control plane reconciles the owning `AgentRuntime`
- THEN it SHALL NOT call `EnsureProvider` and SHALL set a `SecretPending` condition
- AND it SHALL NOT mark the workspace `Ready` and SHALL NOT set `Degraded`

---

### Requirement: Admin-Membership-Required Enforcement

The gateway SHALL reject an `EnsureProvider` call for a provider whose `refresh_strategy`
is non-empty when the workspace has no `WorkspaceMembership` with `role = admin`, because
the gateway restricts provider credential rotation to workspace administrators and
OpenShell performs that rotation. The gateway SHALL return a failed-precondition error for
this case, which the HyperShell API server surfaces as HTTP `422` on the create or update
path ([`data-model.spec.md`](./data-model.spec.md), "WorkspaceMembership"). The control
plane SHALL `ReconcileMemberships` before `EnsureProvider` so an admin membership exists
before a refresh-bearing provider is ensured.

The HyperShell API server SHALL additionally validate this invariant ahead of
provisioning (reject the create or update with `422` when a `ProviderBinding` with a
non-empty `refresh_strategy` has no admin membership), so the error is reported at request
time; the gateway-side check is the authoritative enforcement at the provisioning
boundary.

#### Scenario: Refresh-bearing provider without an admin membership is rejected

- GIVEN a workspace whose memberships are all `role = member`
- WHEN a provider with a non-empty `refresh_strategy` is ensured on it
- THEN the gateway SHALL reject the call with a failed-precondition error
- AND the HyperShell API surface SHALL report `422`
- AND the provider SHALL NOT be created on the gateway

#### Scenario: Admin membership present allows the refresh-bearing provider

- GIVEN a workspace with at least one `role = admin` membership
- WHEN a provider with a non-empty `refresh_strategy` is ensured on it
- THEN the gateway SHALL accept the call and realize the provider
- AND OpenShell SHALL own rotation of the usable credential

---

### Requirement: Workspace Name Collision Enforcement

The gateway SHALL reject an `EnsureWorkspace` call whose name is already taken on that
gateway by a workspace owned by a DIFFERENT `agent_runtime_id`. The gateway SHALL return
an already-exists error for this case, which the HyperShell API server surfaces as HTTP
`409` ([`data-model.spec.md`](./data-model.spec.md), "Gateway-Side Provisioning",
Workspace Name Collision). Re-ensuring the SAME `agent_runtime_id` with the same name SHALL
NOT be treated as a collision; it is the idempotent no-op of Requirement: Idempotent
Workspace Provisioning API.

The HyperShell API server SHALL additionally reject, with `409`, an `AgentRuntime` create
whose `workspace.name` collides with an existing workspace on the same gateway, and SHALL
persist neither the `AgentRuntime` nor the `AgentWorkspace` record, so the collision is
reported at request time; the gateway-side check is the authoritative enforcement at the
provisioning boundary.

#### Scenario: Name taken by another runtime is rejected

- GIVEN a workspace named `amber-reviews` on a gateway owned by `AgentRuntime` `R1`
- WHEN `EnsureWorkspace` is called for `AgentRuntime` `R2` with name `amber-reviews` on the same gateway
- THEN the gateway SHALL reject the call with an already-exists error
- AND the HyperShell API surface SHALL report `409`
- AND no workspace SHALL be created for `R2`

#### Scenario: Re-ensuring the same runtime and name is not a collision

- GIVEN a workspace named `amber-reviews` owned by `AgentRuntime` `R1`
- WHEN `EnsureWorkspace` is called again for `R1` with name `amber-reviews`
- THEN the gateway SHALL return the existing workspace as a no-op success

---

### Requirement: Idempotency and Retry Semantics

Every provisioning operation SHALL be safe to retry. A transient failure (the gateway
unreachable, a timeout, a transport error) SHALL be a retryable provisioning error: the
control plane SHALL return a contextual error so the `AgentRuntime` reconcile is retried
by the reconcile queue with bounded backoff, and SHALL set the `AgentRuntime` status to
`Degraded` with a user-facing message that names the gateway-side failure
([`control-plane.spec.md`](./control-plane.spec.md), "AgentRuntime Reconciliation"). A
retry SHALL re-run the ensure operations and converge; a partially-applied workspace (for
example a workspace created but a provider not yet realized) SHALL be completed on the next
reconcile, never left in a create-and-ignore state.

The control plane SHALL NOT silently swallow a partial failure: when any ensure operation
in the sequence fails, the error SHALL propagate (or be collected and propagated) so the
reconcile is retried; it SHALL NOT report the workspace `Ready` while any sub-resource
provisioning failed.

A permanent rejection (a `409` name collision or a `422` admin-required precondition) SHALL
NOT be retried as a transient failure; it SHALL degrade the `AgentRuntime` with a message
that identifies the cause, and SHALL be resolved by correcting the desired state rather
than by backoff.

#### Scenario: Gateway unreachable during provisioning is retried

- GIVEN an `AgentRuntime` whose gateway is unreachable
- WHEN the control plane attempts to provision the workspace
- THEN it SHALL return a contextual error without secrets in its text
- AND set the `AgentRuntime` status to `Degraded` with a user-facing message
- AND the reconcile queue SHALL retry, converging when the gateway is reachable

#### Scenario: Partial application completes on retry

- GIVEN a reconcile that created the workspace but failed before realizing a provider
- WHEN the reconcile is retried
- THEN the control plane SHALL detect the workspace exists, make no destructive change, and realize the outstanding provider
- AND the workspace SHALL be reported `Ready` only after all sub-resources are provisioned

#### Scenario: Permanent rejection is not retried as transient

- GIVEN an `EnsureWorkspace` that is rejected for a name collision
- WHEN the control plane handles the rejection
- THEN it SHALL degrade the `AgentRuntime` with a message identifying the collision
- AND SHALL NOT retry the call under transient backoff

---

### Requirement: Cascade Deletion Semantics

When an `AgentRuntime` is deleted, the control plane SHALL delete its gateway-side
`AgentWorkspace` and all sub-resources through `DeleteWorkspace`: the workspace, its
`WorkspaceMembership`s, the OpenShell providers realized from its `ProviderBinding`s, and
its `InferenceRoute`. Deletion SHALL be **unconditional** (no retention policy, no
operator confirmation) and SHALL cascade to every sub-resource, mirroring the unconditional
per-gateway cleanup of
[`openshell-gateway-database.spec.md`](./openshell-gateway-database.spec.md).

`DeleteWorkspace` SHALL be idempotent: an already-absent workspace (or already-absent
sub-resources) counts as successful cleanup, so replaying a delete is safe. After the
gateway-side cascade, the `AgentWorkspace`, `WorkspaceMembership`, `ProviderBinding`, and
`InferenceRoute` records SHALL be deleted from the HyperShell database
([`data-model.spec.md`](./data-model.spec.md), "AgentRuntime Cluster Resource
Provisioning"). The control plane SHALL NOT delete the shared `ProviderSpec` records, which
are standalone resources referenced across workspaces.

A deletion failure SHALL be handled like any retryable provisioning error: the control
plane SHALL return a contextual error so the delete is retried, and SHALL NOT report the
`AgentRuntime` finalized while gateway-side objects remain. Durable provider material in
the gateway's own credential Vault plane is reclaimed by the gateway's own
credential-revocation path
([`openshell-gateway-credentials.spec.md`](./openshell-gateway-credentials.spec.md)); this
contract removes the workspace and provider registrations on the gateway, not the Vault
plane itself.

#### Scenario: AgentRuntime deletion cascades on the gateway

- GIVEN an `AgentRuntime` with a provisioned workspace, two memberships, a provider, and an inference route
- WHEN the `AgentRuntime` is deleted
- THEN the control plane SHALL call `DeleteWorkspace`, removing the workspace, both memberships, the provider, and the inference route on the gateway
- AND it SHALL delete the corresponding `AgentWorkspace`, `WorkspaceMembership`, `ProviderBinding`, and `InferenceRoute` records from the database
- AND it SHALL NOT delete any shared `ProviderSpec`

#### Scenario: Replay a delete whose objects are already gone

- GIVEN a delete for an `AgentRuntime` whose gateway-side workspace is already absent
- WHEN the control plane runs `DeleteWorkspace`
- THEN it SHALL treat the workspace and sub-resources as successfully cleaned up and SHALL NOT return an error

#### Scenario: Deletion failure is retried, not silently finalized

- GIVEN an `AgentRuntime` delete whose gateway is unreachable
- WHEN the control plane attempts `DeleteWorkspace`
- THEN it SHALL return a contextual error so the delete is retried
- AND it SHALL NOT report the `AgentRuntime` finalized while the gateway-side workspace may remain

---

### Requirement: No Raw-Secret Placement Through This Contract

This contract SHALL carry only durable provider **material** plus a **refresh strategy**
to the gateway and SHALL NOT be used to place, render, or rotate a raw, usable credential.
It SHALL NOT reintroduce the superseded raw-`Secret` credential-materialization model
([`agent-secret-late-binding.spec.md`](./agent-secret-late-binding.spec.md), Supersedes).
There SHALL be no RPC on this API that returns a usable, minted credential to the control
plane, and no field that carries a control-plane-minted token for the gateway to store
inertly. OpenShell holds the usable credential and owns its rotation.

#### Scenario: No minted credential crosses the contract

- GIVEN a provider realized through `EnsureProvider`
- WHEN the control plane inspects what it sent and received
- THEN it SHALL have sent durable material and a refresh strategy only
- AND it SHALL NOT have received or stored a minted, rotatable credential
- AND no raw credential `Secret` SHALL have been placed by the control plane

---

## Provisioning API Reference (illustrative)

The following sketch is **illustrative** of the gateway-agnostic request and response
shapes, in the manner of the illustrative SQL in
[`openshell-gateway-database.spec.md`](./openshell-gateway-database.spec.md). It is not
the generated contract; the code/reconcile phase defines the actual proto and client. All
secret-valued fields carry durable material only (never a minted credential), and all
ensure operations are idempotent as specified above.

```text
service AgentWorkspaceProvisioning {
  rpc EnsureWorkspace(EnsureWorkspaceRequest) returns (Workspace)
  rpc ReconcileMemberships(ReconcileMembershipsRequest) returns (MembershipSet)
  rpc EnsureProvider(EnsureProviderRequest) returns (Provider)
  rpc EnsureInferenceRoute(EnsureInferenceRouteRequest) returns (InferenceRoute)
  rpc DeleteWorkspace(DeleteWorkspaceRequest) returns (DeleteWorkspaceResponse)
}

EnsureWorkspaceRequest { agent_runtime_id; workspace_name }
  -> ALREADY_EXISTS when workspace_name is owned by a different agent_runtime_id (HTTP 409)

ReconcileMembershipsRequest { agent_runtime_id; workspace_name; memberships[] }
  memberships[] { subject /* OIDC subject UUID */; role /* admin | member */ }
  // declarative: the full desired set; absent memberships are pruned

EnsureProviderRequest {
  agent_runtime_id; workspace_name;
  provider_name;                 // ProviderBinding.name, unique in the workspace
  provider_spec;                 // gateway-agnostic capability from ProviderSpec
  refresh_strategy {             // app-installation-token | google-service-account-jwt | static-token
    type; material;              // non-secret refresh parameters
    secret_material[]            // durable material only (PEM, SA key); never a minted token
  }
}
  -> FAILED_PRECONDITION when refresh_strategy is non-empty and no admin membership (HTTP 422)

EnsureInferenceRouteRequest { agent_runtime_id; workspace_name; provider_name; model_alias }
  // at most one route per workspace; converges the single route

DeleteWorkspaceRequest { agent_runtime_id; workspace_name }
  // unconditional cascade: workspace + memberships + providers + inference route; idempotent
```

## Design Decisions

| Decision | Rationale |
|----------|-----------|
| Mirror the per-gateway database provisioning channel | Gateway-side workspace provisioning is the same class of problem as per-gateway database provisioning one level up: a gateway-scoped external object provisioned from an admin connection, reconciled idempotently, cascade-cleaned on deletion. Reusing that shape keeps one operational and failure/retry model across both channels. |
| Idempotent ensure RPCs, declarative membership set | The reconcile convention is reconcile-do-not-create-or-skip. Ensure operations update mutable fields in place, and a declarative membership set prunes removed memberships, so re-running against unchanged desired state is a no-op and a changed set converges. |
| Bind the gateway-side workspace to the owning `agent_runtime_id` | This is what lets idempotency and collision detection coexist: the same runtime re-ensuring its own name is a no-op, while a different runtime taking a used name is a `409` collision. |
| Carry durable material plus refresh strategy, never a minted credential | Keeps the contract consistent with `agent-secret-late-binding.spec.md`: OpenShell holds and rotates the usable credential, so the control plane never places or rotates a raw `Secret` and there is no stale-token rotator to build. |
| Enforce `422` admin-required and `409` name-collision at both the API server and the gateway | The API server check reports the error at request time for a good user experience; the gateway check is the authoritative enforcement at the provisioning boundary, so an out-of-band or racing provisioning attempt is still rejected. |
| `EnsureProvider` only after `ReconcileMemberships` and only with present material | Ordering guarantees the admin membership exists before a refresh-bearing provider, and the `SecretPending` gate keeps the control plane from registering a provider with empty material, converging automatically once the material is uploaded cluster-local. |
| Unconditional cascade deletion, idempotent | An `AgentRuntime` deletion must not leave an orphaned workspace, membership, provider, or route on the gateway; unconditional cascade mirrors the database channel's unconditional drop, and idempotence makes delete replay safe. Shared `ProviderSpec`s are not deleted because they are referenced across workspaces. |

## References

- [`data-model.spec.md`](./data-model.spec.md) - AgentRuntime, AgentWorkspace, WorkspaceMembership, ProviderSpec, ProviderBinding, InferenceRoute kinds; "Gateway-Side Provisioning"; the `409` and `422` request-time validations
- [`control-plane.spec.md`](./control-plane.spec.md) - the Wave B `AgentRuntimeReconciler`, the `ProviderRealizer` seam, status lifecycle and retry
- [`agent-secret-late-binding.spec.md`](./agent-secret-late-binding.spec.md) - credentials realized as OpenShell providers, OpenShell owns rotation, the `SecretPending` gate, cluster-local material read
- [`openshell-gateway-database.spec.md`](./openshell-gateway-database.spec.md) - the per-gateway provisioning channel this contract mirrors
- [`openshell-gateway-credentials.spec.md`](./openshell-gateway-credentials.spec.md) - the gateway-provider Vault plane and credential revocation on deletion
- [`openshell-inference-routing.spec.md`](./openshell-inference-routing.spec.md) - the `inference.local` route the single `InferenceRoute` configures
