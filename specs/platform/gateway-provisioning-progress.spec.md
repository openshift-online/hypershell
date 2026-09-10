# Gateway Provisioning Progress

**Date:** 2026-09-10
**Status:** Draft
**Applies to:** `components/api-server` (Gateway resource model, gRPC/REST API), `components/control-plane` (GatewayReconciler condition updates), `packages/gateway-management-ui` (provisioning stepper UI)

## Purpose

When a user creates a gateway, the UI currently shows a generic loading state with
no indication of what the platform is doing or how far along provisioning has
progressed. This spec defines a **provisioning progress model** that decomposes
gateway provisioning into a small set of user-meaningful steps, each with an
observable completion or failure signal, so the UI can render a step-by-step
progress view (stepper, flow chart, or equivalent) instead of an opaque spinner.

The progress model is deliberately coarse - it groups internal reconciler
operations into steps that are meaningful to an end user ("Configuring identity
provider") rather than exposing every Kubernetes resource apply. The goal is
user confidence and actionable failure context, not operational telemetry.

### Relationship to other specifications

- [`gateway-phase-vocabulary.spec.md`](./gateway-phase-vocabulary.spec.md) defines
  the top-level `phase` field (`Pending`, `Provisioning`, `Running`, `Failed`,
  `Degraded`). Provisioning conditions are a finer-grained complement to `phase` -
  they describe *where within* `Provisioning` the gateway currently is.
- [`openshell-gateway-health.spec.md`](./openshell-gateway-health.spec.md) defines
  when a gateway transitions between phases based on workload and route readiness.
  This spec adds sub-phase visibility during the `Pending` -> `Provisioning` ->
  `Running` / `Failed` progression.
- [`openshell-gateway.spec.md`](./openshell-gateway.spec.md) defines the full
  provisioning sequence. This spec selects which steps from that sequence are
  surfaced to users.
- [`openshell-gateway-keycloak.spec.md`](./openshell-gateway-keycloak.spec.md)
  defines IdP client provisioning (step 3 in this spec).
- [`openshell-gateway-database.spec.md`](./openshell-gateway-database.spec.md)
  defines database provisioning (step 2 in this spec).

### Scope note

This spec covers the **data model** (provisioning conditions on the Gateway
resource) and the **behavioral contract** (when each condition transitions). UI
layout, animation, and visual design are out of scope - the UI consumes the
conditions and renders them according to its own design system. The spec does
define the user-facing step labels and ordering so that all consumers present a
consistent provisioning narrative.

---

## Domain Vocabulary

A Gateway's provisioning progress is reported through a set of **provisioning
conditions**, each representing a discrete, user-meaningful step of the
provisioning lifecycle. Conditions are orthogonal to the existing `phase` and
`status` fields - they provide sub-phase granularity.

Each condition carries:

| Field | Type | Description |
|---|---|---|
| `type` | string | The condition identifier (e.g., `EnvironmentReady`) |
| `status` | enum | `Pending`, `InProgress`, `Complete`, `Failed` |
| `message` | string | Human-readable detail (empty when `Pending`; failure reason when `Failed`) |

### Provisioning Steps

The following conditions, in order, represent the user-facing provisioning
steps. The ordering reflects the actual dependency chain in the reconciler.

| # | Condition Type | User-Facing Label | What it covers |
|---|---|---|---|
| 1 | `EnvironmentReady` | Preparing environment | Namespace creation, RBAC setup, cluster prerequisites |
| 2 | `DatabaseReady` | Provisioning database | ManagedDatabase resolution, per-gateway DDL, credential Secret |
| 3 | `IdentityProviderReady` | Configuring identity provider | Keycloak client provisioning, OIDC config persistence |
| 4 | `GatewayDeployed` | Deploying gateway | TLS certificates, config validation, Deployment, Service, NetworkPolicy, routing resources |
| 5 | `GatewayHealthy` | Verifying gateway health | Deployment readiness, route readiness, phase transition to `Running` |

**Step grouping rationale:**
- TLS certificate issuance and external routing are grouped into step 4
  ("Deploying gateway") because they are tightly coupled to the deploy sequence
  and not independently meaningful to an end user.
- Step 3 is omitted (condition not emitted) when the gateway has no OIDC
  configuration, since there is no IdP work to perform.

---

## Requirements

### Requirement: GPP-01 -- Provisioning Conditions on the Gateway Resource

The Gateway resource SHALL carry an ordered list of provisioning conditions that
describe sub-phase progress during provisioning. The conditions SHALL be
persisted in the API server and exposed via both the REST and gRPC APIs.

The conditions list SHALL only be present (non-null) while the gateway `phase` is
`Pending`, `Provisioning`, or `Failed`. When a gateway reaches `Running`, the
platform MAY clear the conditions list or retain it with all steps `Complete` -
the UI treats `Running` phase as the authoritative "done" signal regardless.

#### Scenario: Conditions appear on a newly created gateway

- GIVEN a user creates a new Gateway resource
- WHEN the API server persists the Gateway
- THEN the Gateway SHALL have a `provisioning_conditions` field
- AND the field SHALL contain the ordered list of condition types applicable to
  this gateway's configuration
- AND each condition SHALL have `status` set to `Pending`

#### Scenario: Conditions reflect gateway configuration

- GIVEN a Gateway with no OIDC configuration (`oidc` is null or `oidc.issuer` is
  empty)
- WHEN the provisioning conditions are initialized
- THEN the `IdentityProviderReady` condition SHALL be omitted from the list
- AND the remaining conditions SHALL retain their relative order

#### Scenario: Conditions visible via REST API

- GIVEN a Gateway with `phase` `Provisioning`
- WHEN a client fetches the Gateway via `GET /api/hypershell/v1/gateways/{id}`
- THEN the response SHALL include `provisioning_conditions` with the current
  status of each step

---

### Requirement: GPP-02 -- Control Plane Updates Conditions During Reconciliation

The GatewayReconciler SHALL update each provisioning condition as it completes
or fails the corresponding reconciliation step. Conditions SHALL transition in
order - a later condition SHALL NOT move to `InProgress` until all prior
conditions are `Complete`.

#### Scenario: Successful provisioning progresses through all steps

- GIVEN a Gateway with all conditions in `Pending` status
- WHEN the GatewayReconciler begins reconciliation
- THEN it SHALL set `EnvironmentReady` to `InProgress` before creating the
  namespace
- AND it SHALL set `EnvironmentReady` to `Complete` after the namespace and RBAC
  are confirmed
- AND it SHALL proceed to the next applicable condition in order

#### Scenario: Step failure sets condition to Failed

- GIVEN the GatewayReconciler is processing the `DatabaseReady` step
- WHEN the ManagedDatabase resolution or DDL provisioning fails
- THEN it SHALL set `DatabaseReady` to `Failed`
- AND it SHALL populate `message` with the failure reason
- AND subsequent conditions SHALL remain in `Pending` status
- AND the gateway `phase` SHALL be set to `Failed`

#### Scenario: IdP step skipped for non-OIDC gateways

- GIVEN a Gateway with no OIDC configuration
- WHEN the GatewayReconciler completes the `DatabaseReady` step
- THEN it SHALL proceed directly to `GatewayDeployed`
- AND the `IdentityProviderReady` condition SHALL not be present

---

### Requirement: GPP-03 -- UI Renders Provisioning Progress as a Stepper

The gateway management UI SHALL render provisioning conditions as an ordered
stepper (or equivalent flow-chart visualization) when a gateway's `phase` is
`Pending`, `Provisioning`, or `Failed`. Each step SHALL display a visual
indicator corresponding to its condition status.

| Condition Status | Visual Indicator |
|---|---|
| `Pending` | Inactive / not yet reached |
| `InProgress` | Loading / in-progress animation |
| `Complete` | Success indicator (e.g., green check) |
| `Failed` | Failure indicator (e.g., red X) with failure message |

#### Scenario: User sees provisioning progress after creating a gateway

- GIVEN a user has just created a gateway
- WHEN the gateway detail view loads and the gateway `phase` is `Provisioning`
- THEN the UI SHALL display the provisioning stepper
- AND completed steps SHALL show a success indicator
- AND the current step SHALL show an in-progress indicator
- AND future steps SHALL appear inactive

#### Scenario: User sees failure context on a failed step

- GIVEN a gateway with `phase` `Failed`
- AND the `DatabaseReady` condition has `status` `Failed` and a non-empty
  `message`
- WHEN the user views the gateway detail
- THEN the UI SHALL display the stepper with `DatabaseReady` showing a failure
  indicator
- AND the failure `message` SHALL be visible to the user
- AND subsequent steps SHALL appear inactive

#### Scenario: Provisioning completes successfully

- GIVEN a gateway transitions from `phase` `Provisioning` to `Running`
- WHEN the UI polls and receives the updated gateway
- THEN the UI SHALL show all provisioning steps as complete
- AND it MAY transition to the standard gateway detail view (connection command,
  console link, etc.)

---

### Requirement: GPP-04 -- Conditions Are Polling-Compatible

The provisioning conditions SHALL be available on the standard Gateway fetch
endpoints so that the existing console polling mechanism (which already polls
during recoverable phases per `gateway-phase-vocabulary.spec.md`) picks up
condition changes without a separate subscription or endpoint.

#### Scenario: Polling captures step transitions

- GIVEN the UI is polling a Gateway in `Provisioning` phase
- WHEN the control plane advances `DatabaseReady` from `InProgress` to `Complete`
  and `IdentityProviderReady` from `Pending` to `InProgress`
- THEN the next poll response SHALL reflect both condition updates
- AND the UI SHALL update the stepper accordingly

---

## Non-Goals

- Per-step duration tracking or timing estimates ("ETA: 30 seconds")
- Retry controls in the UI (e.g., "Retry database provisioning")
- Provisioning progress for gateway updates (only initial provisioning)
- Sub-step granularity within a condition (e.g., individual cert-manager
  Certificate status within `GatewayDeployed`)
- WebSocket or server-sent event streaming for real-time updates (polling is
  sufficient given the multi-minute provisioning window)
- Provisioning progress on the gateway list view (detail view only)

## Cross-References

- [`gateway-phase-vocabulary.spec.md`](./gateway-phase-vocabulary.spec.md) - canonical phase values
- [`openshell-gateway-health.spec.md`](./openshell-gateway-health.spec.md) - phase lifecycle semantics
- [`openshell-gateway.spec.md`](./openshell-gateway.spec.md) - full provisioning sequence
- [`openshell-gateway-keycloak.spec.md`](./openshell-gateway-keycloak.spec.md) - IdP client provisioning
- [`openshell-gateway-database.spec.md`](./openshell-gateway-database.spec.md) - database provisioning
- [`openshell-gateway-routing.spec.md`](./openshell-gateway-routing.spec.md) - route and external exposure
