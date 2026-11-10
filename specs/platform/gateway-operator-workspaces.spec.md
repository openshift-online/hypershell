# Gateway Operator-Managed Workspaces Specification

**Date:** 2026-11-10
**Status:** Draft
**ID prefix:** OSG-OPWS

**Related:**
- [`openshell-gateway.spec.md`](./openshell-gateway.spec.md) - core gateway provisioning and the `workspace_mode` field (OSG-CORE-18)
- [`data-model.spec.md`](./data-model.spec.md) - Gateway field ownership and write policy (PLAT-DATA-23)
- [`../security/rbac-enforcement.spec.md`](../security/rbac-enforcement.spec.md) - the operator-workspace selector role (SEC-RBAC-27)
- [`gateway-deletion-finalization.spec.md`](./gateway-deletion-finalization.spec.md) - owned-resource enumeration on deletion (GTW-DEL-01)
- [`openshell-gateway-namespace-gc.spec.md`](./openshell-gateway-namespace-gc.spec.md) - gateway namespace reclamation

## Purpose

A Gateway normally provisions its OpenShell workspaces in the mode HyperShell
assigns (`managed` by default, see OSG-CORE-18). Some integrations - the first
being OpenClaw Enterprise (OCE) - require a trusted external operator to own
workspace namespaces, identities, storage, and policies. OpenShell models this as
the Kubernetes driver's `operator` workspace mode.

This spec defines a narrow, authorized, per-gateway opt-in to `operator` workspace
mode, so that an OCE-compatible gateway and an ordinary gateway can be created
through the same supported API/CLI and run side by side on one supported cluster,
each with independent effective settings. It reuses the existing stored
`workspace_mode` field, its validation, and its immutability (OSG-CORE-18); it does
not add a general provisioning-profile service, an arbitrary Helm-values API, or a
plugin framework.

The opt-in replaces the prior workaround of patching the HyperShell API server and
controller Deployments to set deployment-wide operator defaults before creating an
OCE gateway. Deployment-wide defaults are no longer required for a gateway-specific
selection.

## Domain Vocabulary

- **Operator workspace mode** - the OpenShell Kubernetes driver's `operator`
  `workspace_mode`: a trusted external operator (for example OCE) owns the
  workspace namespaces, identities, storage, and policies for the gateway, rather
  than the gateway auto-creating per-workspace namespaces (`managed`) or sharing
  one namespace (`shared`).
- **Operator gateway** - a Gateway whose stored `workspace_mode` is `operator`.
- **Administrator-approved operator settings** - the trust and runtime settings an
  administrator owns for operator-mode gateways on a cluster: the namespace label
  key used to scope tenant namespaces, the allowed custom pod-label patterns, the
  external driver configuration, and the approved sandbox `RuntimeClass`. These are
  control-plane configuration, never caller input.
- **Tenant workspace namespace** - a Kubernetes namespace that holds one operator
  gateway's workspaces. In operator mode the external operator creates and labels
  these namespaces; each carries the administrator-approved namespace label key set
  to a value derived from the owning gateway's own namespace, so the gateway
  discovers only its own tenant namespaces.

## Requirements

### Requirement: OSG-OPWS-01 -- Authorized Per-Gateway Operator Workspace Opt-In

The Gateway create contract SHALL allow an authorized caller to select
`operator` workspace mode by setting the existing `workspace_mode` field to
`operator` at create. The selection reuses the stored `workspace_mode` field, its
accepted vocabulary, and its immutability (OSG-CORE-18); it adds no parallel
request field and no second stored mode.

`workspace_mode` remains platform-managed with one narrow exception
([`data-model.spec.md`](./data-model.spec.md), PLAT-DATA-23): at create, a caller
MAY set it, and the ONLY caller-settable value SHALL be `operator`. Any other
caller-supplied value (`shared` or `managed`) SHALL be rejected naming the field,
because the opt-in surface is operator mode only; the default for a create that
does not set the field SHALL remain `managed` (OSG-CORE-18 is unchanged for that
path). Selecting `operator` SHALL require the platform-wide
`gateway:operator-workspace-creator` entitlement (`platform:admin` also qualifies),
enforced at the create boundaries per
[`../security/rbac-enforcement.spec.md`](../security/rbac-enforcement.spec.md)
(SEC-RBAC-27). The stored value SHALL remain immutable after create for every
writer class (OSG-CORE-18.04): changing a gateway's workspace mode requires delete
and re-create.

The declarative `.hypershell` GitOps import SHALL NOT select `operator` mode: an
Import-kind create that sets `workspace_mode` SHALL be rejected by the service
funnel, because the entitlement is granted per user and an import is authored by a
Repository rather than a user who can hold it (consistent with PLAT-DATA-26 for
`dedicated_route`). A privileged control-plane internal write retains full access
to the field.

#### Scenario: OSG-OPWS-01.01 -- Entitled caller selects operator mode at create

- GIVEN a caller holding `gateway:creator` and `gateway:operator-workspace-creator`
- WHEN the caller creates a Gateway with `workspace_mode = operator` over REST or gRPC
- THEN the API server SHALL accept the create and persist `workspace_mode = operator`
- AND the GatewayReconciler SHALL provision the gateway in operator mode

#### Scenario: OSG-OPWS-01.02 -- Unentitled operator selection is rejected naming the role

- GIVEN a caller holding only `gateway:creator`
- WHEN the caller creates a Gateway with `workspace_mode = operator`
- THEN the create SHALL be rejected with 403 over REST (`PermissionDenied` over
  gRPC) naming `gateway:operator-workspace-creator`
- AND no Gateway SHALL be persisted

#### Scenario: OSG-OPWS-01.03 -- A create that omits the field still defaults to managed

- GIVEN any create request that does not set `workspace_mode`
- WHEN the API server processes the create
- THEN the persisted Gateway SHALL carry `workspace_mode = managed` (OSG-CORE-18.01)
- AND no operator entitlement SHALL be required

#### Scenario: OSG-OPWS-01.04 -- Only operator is caller-settable

- GIVEN a caller (entitled or not)
- WHEN the caller creates a Gateway with `workspace_mode = shared` or
  `workspace_mode = managed`
- THEN the create SHALL be rejected naming `workspace_mode`, because the opt-in
  surface is operator mode only

#### Scenario: OSG-OPWS-01.05 -- GitOps import may not select operator mode

- GIVEN a `.hypershell` Gateway manifest that sets `workspace_mode`
- WHEN the `RepositoryReconciler` imports it (an Import-kind create)
- THEN the service funnel SHALL reject the write directing the author to create the
  gateway directly (hsctl or the web console)

#### Scenario: OSG-OPWS-01.06 -- Workspace mode is immutable after create

- GIVEN an operator gateway
- WHEN any writer class updates it to a different `workspace_mode`
- THEN the update SHALL be rejected with 409 and delete-and-recreate guidance
  (OSG-CORE-18.04)

### Requirement: OSG-OPWS-02 -- Administrator-Owned Operator Settings Preserve the Authorization Boundary

Operator mode changes a gateway's access to external Kubernetes resources and
disables the resource-approval label requirement that ordinary gateways carry.
Selecting operator mode SHALL therefore grant a gateway owner nothing beyond the
mode selection itself. The namespace label key used to scope tenant namespaces, the
allowed custom pod-label patterns, the external driver configuration, and the
approved sandbox `RuntimeClass` SHALL be administrator-owned control-plane
configuration, never caller input. An operator gateway's owner SHALL NOT gain
unrestricted namespace selectors, arbitrary driver configuration, or arbitrary
`RuntimeClass` selection through the opt-in.

The namespace selector value an operator gateway uses to discover its tenant
namespaces SHALL be derived from the gateway's own identity (its API-assigned
namespace), not supplied by the caller, so no caller can direct a gateway at
another tenant's namespaces.

#### Scenario: OSG-OPWS-02.01 -- Operator selection confers only the mode

- GIVEN an entitled caller creating an operator gateway
- WHEN the caller supplies namespace label keys, pod-label patterns, driver
  configuration, or a `RuntimeClass` in the create request
- THEN those fields SHALL be rejected as platform-managed (PLAT-DATA-23), and the
  gateway SHALL use the administrator-approved operator settings only

#### Scenario: OSG-OPWS-02.02 -- Namespace selector is derived from gateway identity

- GIVEN an operator gateway with API-assigned namespace `openshell-<hex>`
- WHEN the control plane resolves the gateway's tenant-namespace selector
- THEN the selector value SHALL be derived from `openshell-<hex>`, not from caller
  input

### Requirement: OSG-OPWS-03 -- Per-Gateway Effective Settings and Coexistence

The control plane SHALL resolve each gateway's effective workspace settings
per-gateway and pass them through the existing reconciliation and Helm values
builder ([`openshell-gateway.spec.md`](./openshell-gateway.spec.md), OSG-CORE-13).
The approved sandbox `RuntimeClass` for operator-mode gateways SHALL come from
administrator-owned control-plane configuration and SHALL apply ONLY to operator
gateways; ordinary (`managed`/`shared`) gateways on the same cluster SHALL retain
their existing sandbox runtime behavior (the cluster-wide
`GATEWAY_SANDBOX_RUNTIME_CLASS` or the cluster default, OSG-CORE-13). One operator
gateway's effective settings SHALL NOT change another gateway's settings or another
operator's namespace access.

Effective settings SHALL be resolved consistently on every reconcile from
administrator configuration, so an administrator configuration change converges
existing operator gateways predictably on their next reconcile and across
control-plane restarts (no frozen per-gateway snapshot of operator settings is
persisted beyond the stored `workspace_mode`).

The supported deployment defaults SHALL remain for compatibility: a control plane
configured with a deployment-wide default workspace mode or sandbox runtime
continues to honor it for gateways that do not carry a per-gateway selection, so an
existing deployment keeps working without change.

#### Scenario: OSG-OPWS-03.01 -- Operator and ordinary gateways coexist with independent settings

- GIVEN an operator gateway and a `managed` gateway placed on the same supported cluster
- WHEN the control plane reconciles both
- THEN the operator gateway SHALL receive the administrator-approved operator
  sandbox `RuntimeClass` and operator driver configuration
- AND the `managed` gateway SHALL retain its existing runtime behavior, unaffected
  by the operator gateway

#### Scenario: OSG-OPWS-03.02 -- Administrator configuration change converges on reconcile

- GIVEN an operator gateway provisioned under one approved operator sandbox `RuntimeClass`
- WHEN an administrator changes the approved operator `RuntimeClass` in control-plane configuration
- THEN the next reconcile (including after a control-plane restart) SHALL resolve
  the gateway's effective settings from the new configuration
- AND the behavior SHALL be the same whether reached by reconcile or by restart

### Requirement: OSG-OPWS-04 -- Placement and Prerequisite Validation

Before provisioning an operator gateway, the control plane SHALL validate that the
selected cluster supports operator workspaces and that the approved sandbox
`RuntimeClass` exists on that cluster. It SHALL reuse the managed-cluster
capability/placement mechanism where appropriate so an operator gateway can be
placed on a cluster advertising operator support. When a prerequisite is missing,
the control plane SHALL report an actionable validation error or provisioning
condition identifying the missing prerequisite, and SHALL NOT provision the
gateway. It SHALL NEVER silently downgrade an operator gateway to another workspace
mode or substitute a different sandbox runtime (this refines OSG-CORE-18.09, which
already forbids falling back to the chart default mode).

> The local `openshell-sandbox` `RuntimeClass` uses `runc`; its name does not imply
> VM isolation. Validating that the named `RuntimeClass` exists is a presence check,
> not an isolation guarantee. Kata-style hardware isolation is separate work (#426).

#### Scenario: OSG-OPWS-04.01 -- Missing cluster support fails with an actionable condition

- GIVEN an operator gateway placed on a cluster that does not support operator workspaces
- WHEN the control plane reconciles it
- THEN it SHALL report an actionable provisioning condition naming the missing
  prerequisite
- AND it SHALL NOT provision the gateway in another workspace mode

#### Scenario: OSG-OPWS-04.02 -- Missing approved RuntimeClass fails without downgrade

- GIVEN an operator gateway whose approved sandbox `RuntimeClass` is absent on the
  selected cluster
- WHEN the control plane reconciles it
- THEN it SHALL report an actionable failure identifying the missing `RuntimeClass`
- AND it SHALL NOT substitute a different runtime or downgrade the workspace mode

### Requirement: OSG-OPWS-05 -- Tenant Namespace Isolation and Transport Security

Each operator gateway SHALL discover only the tenant namespaces assigned to it,
identified by the administrator-approved namespace label key set to the value
derived from that gateway's own namespace (OSG-OPWS-02). An operator gateway SHALL
NOT discover or attach to another operator gateway's tenant namespaces. mTLS
between the gateway and its sandbox supervisors, and sandbox-service hostname
verification, SHALL remain enabled in operator mode; the opt-in SHALL NOT weaken
transport security or certificate verification.

#### Scenario: OSG-OPWS-05.01 -- Operator gateway discovers only its own tenant namespaces

- GIVEN two operator gateways on one cluster, each with tenant namespaces labeled
  with the value derived from its own gateway namespace
- WHEN each gateway discovers its workspace namespaces
- THEN each SHALL discover only the namespaces carrying its own derived label value
- AND neither SHALL attach to the other's tenant namespaces

#### Scenario: OSG-OPWS-05.02 -- Transport security remains enabled

- GIVEN an operator gateway
- WHEN its sandboxes connect to the gateway and sandbox services
- THEN mTLS and sandbox-service hostname verification SHALL remain enabled, as for
  non-operator gateways

### Requirement: OSG-OPWS-06 -- Child Workspace Namespaces Are Reclaimed on Gateway Deletion

When an operator gateway is deleted, the control plane SHALL best-effort delete the
gateway's tenant workspace namespaces, so that removing the gateway from the
platform does not leave its child namespaces behind. The namespaces are identified
by the administrator-approved namespace label key set to the value derived from the
deleted gateway's namespace (OSG-OPWS-02), mirroring the managed-mode cleanup that
reaps `openshell-{gateway_id}-{workspace}` namespaces by the
`openshell.ai/gateway-id` label (OSG-CORE-18.07).

This cleanup SHALL follow the same contract as the other out-of-namespace owned
resources ([`gateway-deletion-finalization.spec.md`](./gateway-deletion-finalization.spec.md),
GTW-DEL-01): it is best-effort and idempotent (an already-absent or
already-terminating namespace is success), it SHALL record a durable orphan signal
when a deletion fails rather than swallowing the failure (GTW-DEL-07), and it SHALL
NOT block the rest of the gateway's teardown. The tenant workspace namespaces are
an out-of-namespace owned-resource class because the gateway's own namespace
cascade does not reach them.

#### Scenario: OSG-OPWS-06.01 -- Operator gateway deletion reaps its tenant namespaces

- GIVEN an operator gateway whose tenant workspace namespaces carry the approved
  label key set to the value derived from the gateway's namespace
- WHEN the control plane processes the Gateway delete event
- THEN it SHALL best-effort delete those tenant workspace namespaces
- AND it SHALL NOT delete the gateway's own managed namespace twice nor any
  namespace not carrying the gateway's derived label value

#### Scenario: OSG-OPWS-06.02 -- Failed tenant-namespace deletion records an orphan and does not block teardown

- GIVEN an operator gateway deletion where one tenant namespace deletion fails
- WHEN the control plane processes the delete
- THEN it SHALL record a durable, operator-visible orphan signal naming the
  namespace (GTW-DEL-07)
- AND it SHALL continue reclaiming the gateway's other owned resources
- AND a replayed delete SHALL converge the remaining namespaces to absence without
  error (GTW-DEL-08)

### Requirement: OSG-OPWS-07 -- Supported Entry Points

The operator opt-in SHALL be reachable through the supported write surfaces with no
deployment-wide patching: the REST gateway Create handler, the gRPC `CreateGateway`
handler, and the generated `hsctl create gateway` command. The CLI SHALL expose an
`--operator-workspaces` flag that sets `workspace_mode = operator` in the create
request; the flag's help SHALL state that it requires the
`gateway:operator-workspace-creator` role. Console exposure MAY follow as separate
work over the same supported API.

#### Scenario: OSG-OPWS-07.01 -- hsctl selects operator mode without deployment patches

- GIVEN an entitled caller
- WHEN they run `hsctl create gateway --name my-gw --placement-selector <label> --operator-workspaces`
- THEN the CLI SHALL send a create request carrying `workspace_mode = operator`
- AND no HyperShell API server or controller Deployment patch SHALL be required for
  that gateway

#### Scenario: OSG-OPWS-07.02 -- Operator setup is separated from gateway creation

- GIVEN a cluster prepared once by an administrator for operator workspaces
  (RuntimeClass and admission-policy installation, approved operator settings)
- WHEN an entitled caller creates operator gateways afterward
- THEN each gateway SHALL be created through the supported create flow alone
- AND the one-time administrator cluster preparation SHALL NOT be repeated per gateway

## Design Decisions

| Decision | Rationale |
|----------|-----------|
| Reuse the stored `workspace_mode` field instead of a new request flag | The target is a single, already-validated enum value (`operator`), not a complex document. Setting `workspace_mode = operator` at create reuses the existing field, its vocabulary, its immutability, and its reconciler path end to end (OSG-CORE-18), so the only change is who may set it at create. The `dedicated_route` flag exists because its target is a multi-field route/certificate document a user should not author; that justification does not apply here. |
| Unlock only `operator` at create, keep `managed` the default | The feature is an opt-in to operator-managed workspaces. `shared` has different trust implications and `managed` is already the default, so unlocking them as a side effect would widen the surface for no benefit. The entitlement unlocks exactly one value. |
| Gate selection behind a dedicated entitlement, mirror `dedicated-route-creator` | Operator mode widens a gateway's access to external Kubernetes resources and disables approval-label requirements, so it must not be available to every creator. A separate, body-dependent, fail-closed entitlement checked at the create boundaries matches the established SEC-RBAC-20 pattern. |
| Administrator owns namespace trust, pod labels, driver config, and RuntimeClass | Operator mode's security properties depend on these settings; letting a gateway owner choose them would let the opt-in escalate access. The caller chooses the mode; the administrator owns everything the mode trusts. |
| Resolve operator effective settings per reconcile, not a persisted snapshot | Re-resolving from administrator configuration each reconcile keeps behavior predictable across config changes and restarts without a new persisted column or migration, consistent with how `GATEWAY_SANDBOX_RUNTIME_CLASS` is already applied at reconcile time. |
| Validate prerequisites and never silently downgrade | Operator mode that silently fell back to `managed`/`shared` or a different runtime would hand a caller a weaker configuration than requested without telling them. Failing with an actionable condition refines the existing OSG-CORE-18.09 no-fallback rule. |
| Reap tenant workspace namespaces on delete by the gateway-derived label | The gateway's own namespace cascade cannot reach operator tenant namespaces, so they are an out-of-namespace owned class, reclaimed best-effort by the gateway-scoping label value the same way managed-mode per-workspace namespaces are reaped by `openshell.ai/gateway-id` (OSG-CORE-18.07). |
