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

This spec covers three concerns end-to-end: the **data model** (provisioning
conditions on the Gateway resource), the **behavioral contract** (when each
condition transitions), and the **UI presentation** (how the gateway detail view
renders provisioning progress using PatternFly's `ProgressStepper` component).

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
| `condition_status` | enum | `Pending`, `InProgress`, `Complete`, `Failed` |
| `message` | string | Human-readable detail (empty when `Pending`; failure reason when `Failed`) |

**Naming note:** The per-condition status field is named `condition_status` (and
the protobuf enum `ProvisioningConditionStatus`) to avoid collision with the
Gateway's existing top-level `status` field, which carries a different semantic
(a human-readable health descriptor).

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

The conditions list SHALL be retained across all gateway phases, including
`Running`. When a gateway reaches `Running`, all conditions SHALL have
`condition_status` set to `Complete`. Retaining the list allows the UI to show
which provisioning steps the gateway went through, even after provisioning
finishes.

#### Condition Initialization Ownership

The control plane SHALL own initialization of provisioning conditions. When the
GatewayReconciler first processes a newly created Gateway whose
`provisioning_conditions` field is null, it SHALL initialize the conditions list
based on the gateway's configuration (e.g., omitting `IdentityProviderReady`
when OIDC is not configured). The API server persists and exposes conditions but
does not populate them - this avoids duplicating reconciler domain logic (such as
which steps apply for a given configuration) in the API server.

#### Scenario: Control plane initializes conditions on first reconciliation

- GIVEN a user creates a new Gateway resource
- AND the API server persists it with `provisioning_conditions` set to null
- WHEN the GatewayReconciler first processes the Gateway
- THEN it SHALL initialize the `provisioning_conditions` field with the ordered
  list of condition types applicable to this gateway's configuration
- AND each condition SHALL have `condition_status` set to `Pending`
- AND the control plane SHALL persist the initialized conditions via the API
  server

#### Scenario: Conditions reflect gateway configuration

- GIVEN a Gateway with no OIDC configuration (`oidc` is null or `oidc.issuer` is
  empty)
- WHEN the control plane initializes provisioning conditions
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

- GIVEN a Gateway with all conditions at `condition_status` `Pending`
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
- AND it SHALL populate `message` with a high-level, user-facing failure reason
- AND the `message` SHALL NOT expose low-level infrastructure details (e.g.,
  Kubernetes error messages, internal resource names, or stack traces)
- AND subsequent conditions SHALL remain at `condition_status` `Pending`
- AND the gateway `phase` SHALL be set to `Failed`

#### Scenario: IdP step skipped for non-OIDC gateways

- GIVEN a Gateway with no OIDC configuration
- WHEN the GatewayReconciler completes the `DatabaseReady` step
- THEN it SHALL proceed directly to `GatewayDeployed`
- AND the `IdentityProviderReady` condition SHALL not be present

#### Scenario: Conditions reset on re-provisioning

- GIVEN a Gateway with `phase` `Running` and all conditions with `condition_status` `Complete`
- WHEN the gateway is modified (e.g., image change, config update) and the
  reconciler re-enters full provisioning, setting `phase` to `Provisioning`
- THEN the control plane SHALL reset all provisioning conditions to `Pending`
  (re-evaluating which conditions apply based on the updated configuration)
- AND the reconciler SHALL progress through conditions from the beginning as
  with initial provisioning
- AND the UI SHALL reflect the reset, showing the stepper in its initial state

#### Scenario: Failed phase is a terminal stepper state

- GIVEN a Gateway with `phase` `Failed` and a condition with `condition_status` `Failed`
- WHEN the UI renders the gateway detail
- THEN polling SHALL have stopped (per the phase vocabulary spec, `Failed` is
  non-recoverable and stops polling)
- AND the stepper SHALL remain frozen in its current state until the user takes
  corrective action (e.g., updating the gateway configuration) which triggers a
  new reconciliation

---

### Requirement: GPP-03 -- UI Renders Provisioning Progress as a Stepper

The gateway detail view in `gateway-management-ui` SHALL render provisioning
conditions as a vertical `ProgressStepper` (`@patternfly/react-core`) on the
gateway detail page. The stepper SHALL be visible in all gateway phases so the
user can always see which provisioning steps the gateway went through.

#### PatternFly Component Mapping

The UI SHALL use PatternFly's `ProgressStepper`. Each
provisioning condition maps to a `ProgressStep` as follows:

| Condition Status | `ProgressStep` variant | `isCurrent` | Behavior |
|---|---|---|---|
| `Pending` | `default` | `false` | Step appears inactive, not yet reached |
| `InProgress` | `pending` | `true` | Step shows a spinner animation indicating work in progress |
| `Complete` | `success` | `false` | Step shows a green check mark |
| `Failed` | `danger` | `false` | Step shows a red X icon |
| `Failed` (on `GatewayHealthy` when `phase` is `Degraded`) | `warning` | `false` | Step shows a warning icon (recoverable, polling continues) |

**Implementation note:** The `Degraded` row means the variant is not a pure
function of `condition_status` alone - the UI must also inspect the gateway's
`phase` to distinguish `danger` (non-recoverable `Failed` phase) from `warning`
(recoverable `Degraded` phase). Implementers SHOULD encapsulate this logic in a
single helper (e.g., `stepVariant(condition, phase)`) rather than spreading the
branching across the component template.

Each `ProgressStep` SHALL use the condition's user-facing label (from the
Provisioning Steps table) as its title. When a condition has `condition_status` `Failed`
and a non-empty `message`, the `ProgressStep` SHALL display the message using
the `description` prop so the failure reason is visible inline beneath the step
title.

#### Scenario: User sees provisioning progress after creating a gateway

- GIVEN a user has just created a gateway
- WHEN the gateway detail view loads and the gateway `phase` is `Provisioning`
- THEN the UI SHALL display a vertical `ProgressStepper`
- AND completed steps SHALL render as `ProgressStep` with `variant="success"`
- AND the current step SHALL render as `ProgressStep` with `variant="pending"`
  and `isCurrent={true}`
- AND future steps SHALL render as `ProgressStep` with `variant="default"`

#### Scenario: User sees failure context on a failed step

- GIVEN a gateway with `phase` `Failed`
- AND the `DatabaseReady` condition has `condition_status` `Failed` with `message`
  "Database provisioning failed - please verify your database configuration"
- WHEN the user views the gateway detail
- THEN the failed step SHALL render as `ProgressStep` with `variant="danger"`
- AND the `description` prop SHALL display the failure message
- AND subsequent steps SHALL render as `variant="default"` (inactive)

#### Scenario: Provisioning completes successfully

- GIVEN a gateway with `phase` `Running`
- WHEN the user views the gateway detail
- THEN the stepper SHALL show all provisioning steps as `variant="success"`
- AND the gateway detail view SHALL also display the standard ready-state
  affordances (connection command, console link, etc.)

#### Scenario: Stepper visible on a Running gateway

- GIVEN a gateway that completed provisioning and has `phase` `Running`
- WHEN the user views the gateway detail
- THEN the stepper SHALL remain visible with all steps showing `variant="success"`
- AND the user SHALL be able to see which provisioning steps were performed

#### Scenario: Degraded gateway shows health step with warning

- GIVEN a gateway that completed provisioning but later entered `phase` `Degraded`
  (e.g., deployment readiness window timed out, pod crash-looping)
- WHEN the user views the gateway detail
- THEN the `GatewayHealthy` condition SHALL have `condition_status` set to
  `Failed` with a user-facing message describing the health issue
- AND the corresponding `ProgressStep` SHALL render with `variant="warning"`
- AND all prior provisioning steps SHALL remain `variant="success"`
- AND the UI SHALL continue polling (per the phase vocabulary spec, `Degraded` is
  a recoverable phase that keeps polling)

---

### Requirement: GPP-04 -- Conditions Are Polling-Compatible

The provisioning conditions SHALL be available on the standard Gateway fetch
endpoints so that the existing console polling mechanism (which already polls
during recoverable phases per `gateway-phase-vocabulary.spec.md`) picks up
condition changes without a separate subscription or endpoint.

Polling follows the phase vocabulary spec's existing rules: recoverable phases
(`Pending`, `Provisioning`, `Degraded`) keep polling, `Running` stops polling
(stepper is complete), and `Failed` stops polling (stepper is frozen in its
terminal state until the user takes corrective action that triggers a new
reconciliation).

#### Scenario: Polling captures step transitions

- GIVEN the UI is polling a Gateway in `Provisioning` phase
- WHEN the control plane advances `DatabaseReady` from `InProgress` to `Complete`
  and `IdentityProviderReady` from `Pending` to `InProgress`
- THEN the next poll response SHALL reflect both condition updates
- AND the UI SHALL update the stepper accordingly

#### Scenario: Polling stops on Failed phase

- GIVEN a Gateway whose `phase` transitions to `Failed`
- WHEN the UI receives the updated gateway
- THEN polling SHALL stop per the phase vocabulary spec
- AND the stepper SHALL remain frozen showing the failed step and its message
- AND the stepper SHALL only update when the user modifies the gateway
  configuration, triggering a new reconciliation that resets conditions

---

### Requirement: GPP-05 -- Failure Messages Are User-Facing

Condition failure messages SHALL be written for end users, not platform
operators. Messages SHALL describe what went wrong at the level of the
provisioning step ("Database provisioning failed") and MAY include actionable
guidance ("please verify your database configuration"). Messages SHALL NOT
expose low-level infrastructure details such as Kubernetes API error strings,
internal resource names, namespace identifiers, or stack traces.

The control plane MAY log the full infrastructure-level error for operator
debugging, but the `message` field persisted on the condition SHALL contain
only the user-facing summary.

#### Scenario: Database provisioning error produces a user-facing message

- GIVEN the ManagedDatabase DDL provisioning fails because the CNPG Cluster is
  unreachable
- WHEN the control plane sets `DatabaseReady` to `Failed`
- THEN the `message` SHALL be a user-facing summary (e.g., "Database
  provisioning failed - the database service is currently unavailable")
- AND the `message` SHALL NOT contain the raw Kubernetes or PostgreSQL error
  string

#### Scenario: IdP provisioning error produces a user-facing message

- GIVEN Keycloak client creation fails because the Keycloak service returns an
  HTTP 503
- WHEN the control plane sets `IdentityProviderReady` to `Failed`
- THEN the `message` SHALL be a user-facing summary (e.g., "Identity provider
  configuration failed - the authentication service is currently unavailable")
- AND the `message` SHALL NOT contain the HTTP status code, Keycloak API path,
  or internal client name

---

## Non-Goals

- Per-step duration tracking or timing estimates ("ETA: 30 seconds")
- Retry controls in the UI (e.g., "Retry database provisioning")
- Partial re-provisioning (conditions always reset fully on re-provision; there
  is no "only re-run the steps that changed" optimization)
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
