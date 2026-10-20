# AgentWorkspace Realization via OpenShell's Existing API Specification

**Date:** 2026-10-18
**Status:** Active
**Jira:** HYPERSHELL-412
**Supersedes the mechanism of:** HYPERSHELL-410 (#604) and its activation (#606/#607) - the bespoke `AgentWorkspaceProvisioning` gateway gRPC contract
**Related:** [`extension-api-openshell-api-mapping.spec.md`](./extension-api-openshell-api-mapping.spec.md) (the 1:1 Extension-API to OpenShell-API map), [`data-model.spec.md`](./data-model.spec.md) (AgentRuntime, AgentWorkspace, WorkspaceMembership, ProviderBinding, InferenceRoute kinds; "Gateway-Side Provisioning"), [`control-plane.spec.md`](./control-plane.spec.md) ("AgentRuntime Reconciliation", the `ProviderRealizer` seam), [`agent-secret-late-binding.spec.md`](./agent-secret-late-binding.spec.md) (credentials realized as OpenShell providers; rotation ownership split by strategy - OpenShell for native strategies, a bounded control-plane rotator for GitHub), [`openshell-gateway-database.spec.md`](./openshell-gateway-database.spec.md) (the per-gateway admin-connection provisioning channel this reuses the discipline of), [`openshell-gateway-credentials.spec.md`](./openshell-gateway-credentials.spec.md) (the gateway-provider Vault plane), [`openshell-inference-routing.spec.md`](./openshell-inference-routing.spec.md) (the `inference.local` route this configures)

## Purpose

This specification defines how the control plane realizes an `AgentRuntime`'s workspace on the
OpenShell gateway it is placed on: by **consuming OpenShell's existing provisioning API** - the
same interface the `openshell` CLI drives (`workspace`, `workspace member`, `provider`,
`provider refresh`, `inference`) - authenticated as a gateway administrator identity. HyperShell
Extension API kinds (`AgentWorkspace`, `WorkspaceMembership`, `ProviderBinding`,
`InferenceRoute`) are a **convenience and extension layer over OpenShell**; the control plane
reconciles each one into its OpenShell native resource through OpenShell's existing API. It does
NOT define a new contract OpenShell must implement.

The HyperShell API server holds the **desired state** of these objects as API resources; the
control plane reconciles that desired state onto the gateway. An OpenShell gateway is the system
of record for what is actually provisioned on it: the workspace, its memberships, the OpenShell
**providers** realized from durable credential material, and the inference route that binds
`inference.local` to a provider and model. The `ProviderRealizer` seam in the
`AgentRuntimeReconciler` ([`control-plane.spec.md`](./control-plane.spec.md)) is the client of
OpenShell's existing API.

### Supersession: the bespoke gateway gRPC contract is retired

HYPERSHELL-410 (#604) defined a new `AgentWorkspaceProvisioning` gRPC service (proto, generated
stubs, and a control-plane client that dials a dedicated endpoint), which #606/#607 activated.
That mechanism is backwards: it implies OpenShell must grow a server for a HyperShell-specific
contract, when OpenShell already exposes the exact provisioning operations through its existing
API (the operations the `openshell` CLI drives, which `bases/agents/sdlc-coordinator/run.sh`
uses in production today). This specification supersedes that mechanism. It keeps the
`ProviderRealizer` seam, the provider model, the `app-installation-token` resolver, cluster-local
Vault material, the `SecretPending` gate, and realize-before-prune; only the realizer's **adapter**
changes - from "dial a bespoke `AgentWorkspaceProvisioning` endpoint" to "call OpenShell's
existing provisioning API as the gateway admin identity."

The following artifacts of the superseded mechanism SHALL be removed in the code/reconcile phase
(this spec is design-only and defines no Go, proto, or OpenAPI code):

- the `AgentWorkspaceProvisioning` proto
  (`components/api-server/proto/hypershell/gateway/v1/workspace_provisioning.proto`) and its
  generated stubs;
- the control-plane client/adapter that dials a bespoke provisioning endpoint
  (the `workspace_provisioner` dialing path and its `WorkspaceProvisioningPort` bespoke binding);
- the `HYPERSHELL_GATEWAY_PROVISIONING_ENDPOINT` configuration gate.

### Scope boundary

- **This spec owns:** how the control plane consumes OpenShell's existing provisioning API to
  realize the gateway-side workspace objects (the realizer operations, their idempotency, retry,
  enforcement surfacing, and cascade deletion), the OpenShell gateway admin session the control
  plane opens, and the gateway-side identity and naming of the realized objects.
- **This spec does not own:** the exact OpenShell wire API (REST or gRPC endpoints and request
  field names behind each CLI operation), which is OpenShell upstream territory and is referenced
  by the operations the `openshell` CLI drives, with the 1:1 field map in
  [`extension-api-openshell-api-mapping.spec.md`](./extension-api-openshell-api-mapping.spec.md);
  the REST/gRPC API-resource schemas of the Extension kinds (owned by
  [`data-model.spec.md`](./data-model.spec.md)); the cluster-side scheduling resources (owned by
  [`control-plane.spec.md`](./control-plane.spec.md)); the provider-credential Vault plane (owned
  by [`openshell-gateway-credentials.spec.md`](./openshell-gateway-credentials.spec.md)); and the
  cluster-local material read path (owned by
  [`agent-secret-late-binding.spec.md`](./agent-secret-late-binding.spec.md)).

## Architecture

```
HyperShell API server (desired state, PostgreSQL)
  AgentRuntime --owns--> AgentWorkspace --> WorkspaceMembership[*]
                                       --> ProviderBinding[*] --> ProviderSpec, SecretSource
                                       --> InferenceRoute (0..1)
        |  gRPC watch stream (filtered by cluster_id)
        v
Control plane (AgentRuntimeReconciler, on the assigned ManagedCluster)
  |-- reads SecretSource material cluster-local (read-only pkg/secretstore)
  `-- ProviderRealizer (client of OpenShell's EXISTING API)
        |  OpenShell gateway admin session, short-lived per operation
        |  same operations the `openshell` CLI drives (workspace / member / provider / refresh / inference)
        v
OpenShell gateway (existing API; system of record for provisioned workspace state)
  |-- workspace                 (named per AgentRuntime, unique on the gateway)
  |-- workspace member[*]       (OIDC subject -> role: admin | member)
  |-- provider[*] + refresh     (realized from material + refresh_strategy; OpenShell rotates its native strategies, control-plane GitHub rotator rotates app-installation-token)
  `-- inference route (0..1)    (binds inference.local to a provider + model alias)
```

### Why reuse the database channel's discipline (not its contract)

The per-gateway database channel ([`openshell-gateway-database.spec.md`](./openshell-gateway-database.spec.md))
provisions a gateway-scoped external object from a short-lived administrative connection,
idempotently, and cleans it up unconditionally on deletion. Workspace realization reuses that
operational **discipline** - connection re-resolved per operation, existence-check-then-act,
structured retryable errors, no secrets in logs - but NOT a new bespoke contract: the target is
OpenShell's existing provisioning API rather than a HyperShell-defined one.

## Requirements

### Requirement: AGT-WSP-01 -- OpenShell Gateway Admin Session

The control plane SHALL realize gateway-side workspace objects by calling OpenShell's existing
provisioning API over a short-lived administrative session to the gateway, opened per
provisioning operation and closed afterward. The session SHALL target the gateway's routable
OpenShell API endpoint - the same endpoint the `openshell` CLI reaches through its
`--gateway-endpoint` - resolved from the `Gateway` the `AgentRuntime` references and re-resolved
per operation rather than cached, so a gateway endpoint change takes effect on the next reconcile
without a controller restart.

The session SHALL authenticate as a gateway administrator identity scoped to workspace
provisioning on that gateway (an OpenShell admin OIDC service-account identity, as the
`sdlc-coordinator` mints and logs in with today); it SHALL NOT use a workspace member identity
and SHALL NOT use the coordinator's own `gateway-identity`. The control plane SHALL call
OpenShell's existing API directly as a client of it; it SHALL NOT shell out to the `openshell`
CLI binary, and it SHALL NOT require any new OpenShell-side contract.

The exact OpenShell wire protocol and request shapes behind the provisioning operations are
OpenShell upstream territory; the control plane SHALL use the operations the `openshell` CLI
drives as the authoritative surface, with the field map in
[`extension-api-openshell-api-mapping.spec.md`](./extension-api-openshell-api-mapping.spec.md).

The administrative identity's credential material SHALL NEVER appear in logs, error strings,
telemetry, Kubernetes Events, or API responses. Error messages MAY name the gateway and the
operation that failed; they SHALL NOT include the credential or the durable provider material.

#### Scenario: AGT-WSP-01.01 -- Endpoint re-resolved per operation

- GIVEN a running control plane that has realized a workspace on a gateway
- WHEN the gateway's routable OpenShell API endpoint changes and the reconciler next realizes or updates the workspace
- THEN it SHALL resolve the current endpoint from the `Gateway` resource
- AND no controller restart SHALL be required

#### Scenario: AGT-WSP-01.02 -- Realizer calls OpenShell's existing API, not a bespoke contract

- GIVEN an `AgentRuntime` to realize
- WHEN the control plane provisions its workspace graph
- THEN it SHALL call OpenShell's existing provisioning API (the operations the `openshell` CLI drives) as the gateway admin identity
- AND it SHALL NOT dial a bespoke `AgentWorkspaceProvisioning` endpoint and SHALL NOT shell out to the `openshell` CLI

#### Scenario: AGT-WSP-01.03 -- Admin credential never leaks

- GIVEN a gateway admin session that fails with a transport error embedding the credential
- WHEN the control plane logs the failure and sets the `AgentRuntime` status
- THEN the log line, the status message, and any Event SHALL NOT contain the admin credential or the durable provider material
- AND the message MAY name the gateway and the failed operation

---

### Requirement: AGT-WSP-02 -- Gateway-Side AgentWorkspace Representation and Identity

Each `AgentRuntime` SHALL own exactly one OpenShell workspace on its `Gateway`, realized from its
`AgentWorkspace`. The control plane SHALL bind the realized workspace to the owning
`agent_runtime_id` (in HyperShell desired state) and SHALL realize it under the
`AgentWorkspace.name`, which is unique on the gateway (see Requirement: Workspace Name Collision
Enforcement). The API-resource schema of `AgentWorkspace` and `WorkspaceMembership` is owned by
[`data-model.spec.md`](./data-model.spec.md); this requirement defines only how those resources
are represented and named on the gateway.

Binding the realized workspace to the owning `agent_runtime_id` is what reconciles idempotency
with collision detection: a repeated realization for the SAME `AgentRuntime` and name is a no-op
success, while a DIFFERENT `AgentRuntime` requesting an already-taken name on the same gateway is
a collision. The workspace lifecycle SHALL be create-or-update on reconcile and cascade delete on
`AgentRuntime` deletion; there is no independent gateway-side workspace create or delete outside
the owning `AgentRuntime`'s reconcile.

#### Scenario: AGT-WSP-02.01 -- Workspace carries its owner and name

- GIVEN an `AgentRuntime` whose `AgentWorkspace.name` is `amber-reviews`
- WHEN the control plane realizes the workspace on the gateway
- THEN it SHALL realize an OpenShell workspace named `amber-reviews` bound to the owning `agent_runtime_id`
- AND a second reconcile of the same `AgentRuntime` SHALL match the same workspace as a no-op

---

### Requirement: AGT-WSP-03 -- Idempotent Realization Through OpenShell's Existing Operations

The control plane SHALL realize the workspace and each sub-resource through idempotent
create-or-update ("ensure") calls to OpenShell's existing operations, as part of its normal
reconcile loop. Re-running an ensure against an object that already exists SHALL succeed without
error and SHALL update the object's mutable fields in place (reconcile, do not create-or-skip).
The realizer SHALL provide at least the following logical operations, each realized by the named
OpenShell operation (the operation the `openshell` CLI drives; see
[`extension-api-openshell-api-mapping.spec.md`](./extension-api-openshell-api-mapping.spec.md)):

| Realizer operation | OpenShell operation | Idempotent behavior |
|---|---|---|
| Ensure workspace | `workspace get` then `workspace create --name` | Absent: create. Present and owned by the same `agent_runtime_id`: no-op success. Present under a different owner: collision |
| Reconcile memberships | `workspace member add` / `member remove` to converge the full set | Add missing, update changed roles, remove memberships no longer declared |
| Ensure provider | `provider get` then `provider create` / `provider update`, plus `provider refresh configure` (+ `rotate`) when a refresh strategy applies | Absent: create. Present: update provider config and refresh metadata in place |
| Ensure inference route | `inference set --provider --model` | Converge the at-most-one route to the declared provider and model; changing it redirects inference without restarting sandboxes |
| Delete workspace | `workspace delete` (cascade) | Unconditional and idempotent (see Cascade Deletion Semantics) |

Membership realization SHALL be declarative over the full set, so a membership removed from
desired state is removed on the gateway, matching the prune discipline the control plane applies
elsewhere. Inference-route realization SHALL enforce the at-most-one-route-per-workspace
invariant. Provider and inference calls SHALL carry gateway-agnostic fields only (the
`ProviderSpec` type, the `refresh_strategy` type and its non-secret parameters, the model alias)
plus the durable material for secret-valued fields; they SHALL NOT carry HyperShell-internal
identifiers or backend-specific field names, which are OpenShell's own translation concern.

#### Scenario: AGT-WSP-03.01 -- Realization is idempotent across reconciles

- GIVEN an `AgentRuntime` whose workspace, memberships, providers, and inference route are already realized on the gateway
- WHEN the control plane reconciles it again with unchanged desired state
- THEN every ensure call SHALL succeed and make no change
- AND no error SHALL be returned

#### Scenario: AGT-WSP-03.02 -- Mutable field updated in place

- GIVEN a realized inference route bound to model alias `claude-sonnet-4-5`
- WHEN the `AgentRuntime`'s route is changed to a different model alias
- THEN the control plane SHALL update the single route in place through `inference set`
- AND running sandboxes in the workspace SHALL NOT be restarted

#### Scenario: AGT-WSP-03.03 -- Removed membership is pruned

- GIVEN a workspace with memberships for subjects `A` (admin) and `B` (member)
- WHEN the desired membership set no longer includes `B`
- THEN the control plane SHALL remove `B`'s membership on the gateway
- AND SHALL leave `A`'s admin membership in place

---

### Requirement: AGT-WSP-04 -- Provider Realization Coherence With the Late-Binding Model

The control plane SHALL realize each `ProviderBinding` as an OpenShell **provider** on the
agent's workspace through OpenShell's existing provider operations, from the binding's
`provider_spec_id`, its `refresh_strategy`, and the durable **material** referenced through its
`SecretSource`s, consistent with
[`agent-secret-late-binding.spec.md`](./agent-secret-late-binding.spec.md). It SHALL read that
material cluster-local from the assigned cluster's local store through its read-only
`pkg/secretstore` client and supply it to OpenShell as provider configuration.

The control plane SHALL NOT render the provider credential as a raw Kubernetes `Secret`, SHALL
NOT create an `ExternalSecret`, and SHALL NOT depend on External Secrets Operator. It SHALL send
only the durable material (for example a GitHub App PEM plus `app_id` / `installation_id`, or a
Vertex service-account key) and, for the OpenShell-refreshed strategies, the `refresh_strategy`
type. Rotation ownership follows the strategy: for the strategies OpenShell supports natively
(`google-service-account-jwt`, `oauth2-refresh-token`, `oauth2-client-credentials`,
`aws-sts-assume-role`, `static-token`) OpenShell SHALL mint and rotate the usable credential and
the control plane SHALL NOT mint, refresh, or rotate it; for GitHub App credentials (the
`app-installation-token` intent, which the installed OpenShell CLI does not refresh) a bounded
control-plane GitHub rotator SHALL mint the installation token from the PEM and set it on the
OpenShell provider through `provider update --credential`, re-minting before expiry. The control
plane SHALL NOT run a control-plane rotator for the OpenShell-refreshed strategies. This split is
mapped field-by-field in
[`extension-api-openshell-api-mapping.spec.md`](./extension-api-openshell-api-mapping.spec.md)
(Refresh Strategy Mapping).

When referenced `SecretSource` material is not yet present in the assigned cluster's reachable
local store, provider realization SHALL hold under a `SecretPending` condition rather than fail:
the control plane SHALL NOT create the provider with empty or partial material, SHALL NOT mark the
workspace `Ready`, and SHALL converge the moment the material appears (see
[`agent-secret-late-binding.spec.md`](./agent-secret-late-binding.spec.md), "SecretPending
Condition Gates Provider Creation").

#### Scenario: AGT-WSP-04.01 -- Provider realized from durable material, no raw Secret

- GIVEN a `ProviderBinding` whose `SecretSource` material (a service-account key) is present in the assigned cluster's local store
- WHEN the control plane reconciles the owning `AgentRuntime`
- THEN it SHALL realize the provider through OpenShell's existing provider operations with the material and its refresh strategy
- AND OpenShell SHALL mint and rotate the usable credential thereafter
- AND the control plane SHALL render no raw Kubernetes `Secret` and create no `ExternalSecret` for the credential

#### Scenario: AGT-WSP-04.02 -- Missing material holds provider creation, does not fail

- GIVEN a `ProviderBinding` whose referenced material has not yet been uploaded to the assigned cluster's reachable local store
- WHEN the control plane reconciles the owning `AgentRuntime`
- THEN it SHALL NOT create the provider and SHALL set a `SecretPending` condition
- AND it SHALL NOT mark the workspace `Ready` and SHALL NOT set `Degraded`

---

### Requirement: AGT-WSP-05 -- Admin-Membership-Required Enforcement

OpenShell restricts provider credential updates and rotation to workspace administrators.
Rotation itself is owned by OpenShell for its native strategies and by the bounded control-plane
GitHub rotator for GitHub App credentials (which updates the provider credential as a workspace
admin). The control plane SHALL reconcile memberships before realizing a provider whose
`refresh_strategy` is non-empty, so a `role = admin` membership exists on the workspace before the
refresh-bearing provider is created. When no admin membership is present, OpenShell's
existing API SHALL reject the provider operation; the HyperShell API server surfaces this as HTTP
`422` on the create or update path ([`data-model.spec.md`](./data-model.spec.md),
"WorkspaceMembership").

The HyperShell API server SHALL additionally validate this invariant ahead of realization (reject
the create or update with `422` when a `ProviderBinding` with a non-empty `refresh_strategy` has
no admin membership), so the error is reported at request time; OpenShell's own check is the
authoritative enforcement at the provisioning boundary.

#### Scenario: AGT-WSP-05.01 -- Refresh-bearing provider without an admin membership is rejected

- GIVEN a workspace whose memberships are all `role = member`
- WHEN a provider with a non-empty `refresh_strategy` is realized on it
- THEN OpenShell SHALL reject the operation
- AND the HyperShell API surface SHALL report `422`
- AND the provider SHALL NOT be created on the gateway

#### Scenario: AGT-WSP-05.02 -- Admin membership present allows the refresh-bearing provider

- GIVEN a workspace with at least one `role = admin` membership
- WHEN a provider with a non-empty `refresh_strategy` is realized on it
- THEN OpenShell SHALL accept the operation and realize the provider
- AND the usable credential SHALL be kept current by its rotation owner (OpenShell for its native strategies, the bounded control-plane GitHub rotator for GitHub App credentials)

---

### Requirement: AGT-WSP-06 -- Workspace Name Collision Enforcement

A workspace name SHALL be unique per gateway. When an `AgentRuntime` requests a workspace name
already taken on that gateway by a workspace owned by a DIFFERENT `agent_runtime_id`, the
realization SHALL be a collision, surfaced as HTTP `409`
([`data-model.spec.md`](./data-model.spec.md), "Gateway-Side Provisioning", Workspace Name
Collision). Re-realizing the SAME `agent_runtime_id` with the same name SHALL NOT be treated as a
collision; it is the idempotent no-op of Requirement: Idempotent Realization Through OpenShell's
Existing Operations.

The HyperShell API server SHALL reject, with `409`, an `AgentRuntime` create whose
`workspace.name` collides with an existing workspace on the same gateway, and SHALL persist
neither the `AgentRuntime` nor the `AgentWorkspace` record, so the collision is reported at
request time; OpenShell's own uniqueness is the authoritative enforcement at the provisioning
boundary.

#### Scenario: AGT-WSP-06.01 -- Name taken by another runtime is rejected

- GIVEN a workspace named `amber-reviews` on a gateway owned by `AgentRuntime` `R1`
- WHEN `AgentRuntime` `R2` requests a workspace named `amber-reviews` on the same gateway
- THEN the realization SHALL be rejected as a collision
- AND the HyperShell API surface SHALL report `409`
- AND no workspace SHALL be created for `R2`

#### Scenario: AGT-WSP-06.02 -- Re-realizing the same runtime and name is not a collision

- GIVEN a workspace named `amber-reviews` owned by `AgentRuntime` `R1`
- WHEN the control plane reconciles `R1` again with name `amber-reviews`
- THEN it SHALL match the existing workspace as a no-op success

---

### Requirement: AGT-WSP-07 -- Idempotency and Retry Semantics

Every realization operation SHALL be safe to retry. A transient failure (the gateway unreachable,
a timeout, a transport error) SHALL be a retryable error: the control plane SHALL return a
contextual error so the `AgentRuntime` reconcile is retried by the reconcile queue with bounded
backoff, and SHALL set the `AgentRuntime` status to `Degraded` with a user-facing message that
names the gateway-side failure ([`control-plane.spec.md`](./control-plane.spec.md), "AgentRuntime
Reconciliation"). A retry SHALL re-run the ensure operations and converge; a partially-applied
workspace (for example a workspace created but a provider not yet realized) SHALL be completed on
the next reconcile, never left in a create-and-ignore state.

The control plane SHALL NOT silently swallow a partial failure: when any ensure operation in the
sequence fails, the error SHALL propagate (or be collected and propagated) so the reconcile is
retried; it SHALL NOT report the workspace `Ready` while any sub-resource realization failed.

A permanent rejection (a `409` name collision or a `422` admin-required precondition) SHALL NOT be
retried as a transient failure; it SHALL degrade the `AgentRuntime` with a message that identifies
the cause, and SHALL be resolved by correcting the desired state rather than by backoff.

#### Scenario: AGT-WSP-07.01 -- Gateway unreachable during realization is retried

- GIVEN an `AgentRuntime` whose gateway is unreachable
- WHEN the control plane attempts to realize the workspace
- THEN it SHALL return a contextual error without secrets in its text
- AND set the `AgentRuntime` status to `Degraded` with a user-facing message
- AND the reconcile queue SHALL retry, converging when the gateway is reachable

#### Scenario: AGT-WSP-07.02 -- Partial application completes on retry

- GIVEN a reconcile that created the workspace but failed before realizing a provider
- WHEN the reconcile is retried
- THEN the control plane SHALL detect the workspace exists, make no destructive change, and realize the outstanding provider
- AND the workspace SHALL be reported `Ready` only after all sub-resources are realized

#### Scenario: AGT-WSP-07.03 -- Permanent rejection is not retried as transient

- GIVEN a workspace realization rejected for a name collision
- WHEN the control plane handles the rejection
- THEN it SHALL degrade the `AgentRuntime` with a message identifying the collision
- AND SHALL NOT retry the call under transient backoff

---

### Requirement: AGT-WSP-08 -- Cascade Deletion Semantics

When an `AgentRuntime` is deleted, the control plane SHALL delete its OpenShell workspace and all
sub-resources through OpenShell's existing delete operations: the workspace, its memberships, the
OpenShell providers realized from its `ProviderBinding`s, and its inference route. Deletion SHALL
be **unconditional** (no retention policy, no operator confirmation) and SHALL cascade to every
sub-resource, mirroring the unconditional per-gateway cleanup of
[`openshell-gateway-database.spec.md`](./openshell-gateway-database.spec.md).

Deletion SHALL be idempotent: an already-absent workspace (or already-absent sub-resources) counts
as successful cleanup, so replaying a delete is safe. After the gateway-side cascade, the
`AgentWorkspace`, `WorkspaceMembership`, `ProviderBinding`, and `InferenceRoute` records SHALL be
deleted from the HyperShell database ([`data-model.spec.md`](./data-model.spec.md), "AgentRuntime
Cluster Resource Provisioning"). The control plane SHALL NOT delete the shared `ProviderSpec`
records, which are standalone resources referenced across workspaces.

A deletion failure SHALL be handled like any retryable error: the control plane SHALL return a
contextual error so the delete is retried, and SHALL NOT report the `AgentRuntime` finalized while
gateway-side objects remain. Durable provider material in the gateway's own credential Vault plane
is reclaimed by the gateway's own credential-revocation path
([`openshell-gateway-credentials.spec.md`](./openshell-gateway-credentials.spec.md)); this
realization removes the workspace and provider registrations on the gateway, not the Vault plane
itself.

#### Scenario: AGT-WSP-08.01 -- AgentRuntime deletion cascades on the gateway

- GIVEN an `AgentRuntime` with a realized workspace, two memberships, a provider, and an inference route
- WHEN the `AgentRuntime` is deleted
- THEN the control plane SHALL delete the workspace, both memberships, the provider, and the inference route on the gateway through OpenShell's existing delete operations
- AND it SHALL delete the corresponding `AgentWorkspace`, `WorkspaceMembership`, `ProviderBinding`, and `InferenceRoute` records from the database
- AND it SHALL NOT delete any shared `ProviderSpec`

#### Scenario: AGT-WSP-08.02 -- Replay a delete whose objects are already gone

- GIVEN a delete for an `AgentRuntime` whose gateway-side workspace is already absent
- WHEN the control plane runs the delete
- THEN it SHALL treat the workspace and sub-resources as successfully cleaned up and SHALL NOT return an error

#### Scenario: AGT-WSP-08.03 -- Deletion failure is retried, not silently finalized

- GIVEN an `AgentRuntime` delete whose gateway is unreachable
- WHEN the control plane attempts the delete
- THEN it SHALL return a contextual error so the delete is retried
- AND it SHALL NOT report the `AgentRuntime` finalized while the gateway-side workspace may remain

---

### Requirement: AGT-WSP-09 -- No Raw-Secret Placement Through This Realization

This realization SHALL NOT place, render, or rotate a raw Kubernetes `Secret` for any provider
credential, and SHALL NOT reintroduce the superseded raw-`Secret` credential-materialization
model ([`agent-secret-late-binding.spec.md`](./agent-secret-late-binding.spec.md), Supersedes).
No OpenShell operation SHALL return a usable, minted credential to the control plane.

For the strategies OpenShell refreshes, the realization SHALL carry only durable **material** plus
the **refresh strategy**, and no field SHALL carry a control-plane-minted token: OpenShell holds
and rotates the usable credential. For GitHub App credentials the control plane DOES set a
short-lived installation token it minted from the stored PEM, through the OpenShell provider API
(`provider update --credential`), and re-mints it before expiry; this SHALL remain a live provider
credential set through OpenShell's own API, NOT a raw Kubernetes `Secret` the control plane places,
and the durable PEM SHALL remain the only stored GitHub material.

#### Scenario: AGT-WSP-09.01 -- No minted credential crosses the boundary for an OpenShell-refreshed strategy

- GIVEN a provider realized with a `google-service-account-jwt` strategy through OpenShell's existing provider operations
- WHEN the control plane inspects what it sent and received
- THEN it SHALL have sent durable material and a refresh strategy only
- AND it SHALL NOT have received or stored a minted, rotatable credential
- AND no raw credential `Secret` SHALL have been placed by the control plane

#### Scenario: AGT-WSP-09.02 -- GitHub minted token is set through the provider API, not a raw Secret

- GIVEN a GitHub provider whose durable material is a stored GitHub App PEM
- WHEN the control-plane GitHub rotator mints an installation token and sets it
- THEN it SHALL set the token through the OpenShell provider API (`provider update --credential`)
- AND it SHALL NOT place a raw Kubernetes `Secret` and SHALL keep only the PEM stored

---

### Requirement: AGT-WSP-10 -- Fully Declarative Over OpenShell Kinds

Realizing the Extension kinds through OpenShell's existing API SHALL reproduce the complete
OpenShell workspace graph an agent needs, so the `sdlc-coordinator`'s imperative
self-provisioning (`provision_workspace_providers`) is no longer required. The 1:1 field map, the
two-direction gap analysis, and the enumeration of any residual imperative steps are owned by
[`extension-api-openshell-api-mapping.spec.md`](./extension-api-openshell-api-mapping.spec.md).
This spec SHALL remain consistent with that mapping.

#### Scenario: AGT-WSP-10.01 -- Realization replaces coordinator self-provisioning

- GIVEN an `AgentRuntime` whose workspace, memberships, provider bindings, and inference route are declared in HyperShell
- WHEN the control plane reconciles it against OpenShell's existing API
- THEN the complete OpenShell workspace graph SHALL be realized without the coordinator running `provision_workspace_providers`
- AND any residual imperative step SHALL be an enumerated gap in the mapping spec, not an undocumented side effect

## Design Decisions

| Decision | Rationale |
|----------|-----------|
| Consume OpenShell's existing API, not a bespoke gateway gRPC contract | OpenShell already exposes workspace, member, provider, refresh, and inference operations (the `openshell` CLI drives them; `sdlc-coordinator` uses them in production). A HyperShell-specific `AgentWorkspaceProvisioning` service would make OpenShell implement a contract for a convenience layer that should consume, not extend, OpenShell's own API |
| Keep the `ProviderRealizer` seam; change only its adapter | The seam, the provider model, the `SecretPending` gate, realize-before-prune, and cluster-local material all stay; only the realizer's target changes from a bespoke endpoint to OpenShell's existing API, minimizing churn |
| Gateway admin OIDC identity, short-lived session per operation | Matches how the coordinator authenticates to the gateway and keeps one operational and failure/retry discipline, reusing the per-operation admin-connection pattern of the database channel without reusing a bespoke contract |
| Use the `openshell` CLI operation surface as the authoritative API, wire details CONFIRM-AGAINST-UPSTREAM | OpenShell's server source is not reachable from this repository; grounding in the CLI operations keeps the spec real and defers exact endpoints/field names to OpenShell upstream without fabricating them |
| Retire the bespoke proto, client, and `HYPERSHELL_GATEWAY_PROVISIONING_ENDPOINT` gate | Leaving them would preserve the superseded "OpenShell must implement our contract" mechanism; removing them makes the OpenShell-API-consumption model the only path |
| Carry durable material; mint only the GitHub token, through the provider API | Keeps the realization consistent with `agent-secret-late-binding.spec.md`: for OpenShell-refreshed strategies OpenShell holds and rotates the usable credential; for GitHub the control plane mints the installation token from the PEM and sets it through `provider update --credential`. In no case does the control plane place or rotate a raw Kubernetes `Secret` |

## References

- [`extension-api-openshell-api-mapping.spec.md`](./extension-api-openshell-api-mapping.spec.md) - the 1:1 Extension-API to OpenShell-API field and lifecycle map, two-direction gap analysis, and fully-declarative assertion
- [`data-model.spec.md`](./data-model.spec.md) - AgentRuntime, AgentWorkspace, WorkspaceMembership, ProviderSpec, ProviderBinding, InferenceRoute kinds; "Gateway-Side Provisioning"; the `409` and `422` request-time validations
- [`control-plane.spec.md`](./control-plane.spec.md) - the `AgentRuntimeReconciler`, the `ProviderRealizer` seam, status lifecycle and retry
- [`agent-secret-late-binding.spec.md`](./agent-secret-late-binding.spec.md) - credentials realized as OpenShell providers; rotation ownership split by strategy (OpenShell native vs control-plane GitHub rotator); the `SecretPending` gate; cluster-local material read
- [`openshell-gateway-database.spec.md`](./openshell-gateway-database.spec.md) - the per-gateway admin-connection provisioning channel whose discipline this reuses
- [`openshell-gateway-credentials.spec.md`](./openshell-gateway-credentials.spec.md) - the gateway-provider Vault plane and credential revocation on deletion
- [`openshell-inference-routing.spec.md`](./openshell-inference-routing.spec.md) - the `inference.local` route the inference route configures
