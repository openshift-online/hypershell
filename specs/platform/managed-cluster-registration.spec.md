# Managed Cluster Self-Registration

**Date:** 2026-09-09
**Status:** Draft
**Ticket:** HYPERSHELL-326
**Related:** `security/rbac-enforcement.spec.md` (managed-cluster-registrar role), `platform/data-model.spec.md` (ManagedCluster entity), `platform/control-plane.spec.md` (control plane startup flow)

---

## Purpose

A control plane must determine its own `cluster_id` at runtime without requiring a human to pre-register it and distribute the resulting KSUID out-of-band. The `/registration` sub-resource within the `managedClusters` plugin enables a control plane to register itself and receive a stable `cluster_id` in return. When OIDC is configured, the control plane registers using its `client_credentials` identity; when the API server runs with authentication disabled (local development), the control plane registers by `name` alone.

The same endpoint serves as a health ping. The control plane calls it on a loop; registration calls after the first are no-ops that update `last_seen_at`, giving the API server passive fleet-health visibility with no additional infrastructure.

---

## API

### POST /api/hypershell/v1/managed_clusters/registration

Idempotent. Creates a `ManagedCluster` record on first call; returns the existing record on subsequent calls with the same name. Updates `last_seen_at` on every call.

**Authentication:** When the API server has authentication enabled, the caller SHALL present an OIDC `client_credentials` JWT carrying the `managed-cluster-registrar` role in `realm_access.roles` (see `security/rbac-enforcement.spec.md`). When the API server runs with authentication disabled (local development), no token is required and the endpoint accepts the call unauthenticated.

**Identity resolution:** When a validated JWT is present, the API server extracts its `sub` claim and stores it as `oidc_subject` on the `ManagedCluster` record (audit trail only). When no token is present (authentication disabled), `oidc_subject` is left empty. The caller never supplies `oidc_subject` directly.

**Request:**

```json
{
  "name": "hyp0-mc1",
  "description": "optional"
}
```

**Response (201 Created on first call, 200 OK on subsequent calls):**

```json
{
  "cluster_id": "<KSUID>"
}
```

**Upsert key:** `name`. Name uniqueness across all registered control planes is a provisioning responsibility (GitOps or operator configuration). Any caller presenting the same name receives the same `cluster_id`; this intentionally allows a control plane to restart and re-register without special handling, and allows multiple control plane processes per physical node as long as each carries a distinct name.

---

## Data Model Impact

`ManagedCluster` gains two fields to support self-registration and fleet health:

| Field | Type | Description |
|-------|------|-------------|
| `oidc_subject` | string | OIDC `sub` claim of the caller on the first registration call, or empty when the API server runs with authentication disabled. Stored for audit; not the upsert key. Set server-side; not writable via PATCH. |
| `last_seen_at` | timestamp | Updated on every `/registration` call. Null until first registration. |

`last_seen_at` enables passive fleet health without active probing. API-server-side consumers (dashboard, alerting) derive control plane health from staleness:

| `last_seen_at` age | Derived health |
|--------------------|---------------|
| < 5 min | Healthy |
| 5 - 30 min | Unknown |
| > 30 min | Offline |

These thresholds are informational and may be tuned per deployment. The `status` field on `ManagedCluster` continues to reflect the control-plane reconciler's view of cluster state; `last_seen_at` is a separate, control-plane-reported liveness signal.

---

## Control Plane Startup and Loop

```
startup:
  cluster_id = POST /registration { name: HYPERSHELL_MANAGED_CLUSTER_NAME }
  open WatchGateways(cluster_id=cluster_id)

loop every 60s:
  POST /registration { name: HYPERSHELL_MANAGED_CLUSTER_NAME }
  # returns same cluster_id; API updates last_seen_at
```

Configuration required per control plane:

| Env var | Description |
|---------|-------------|
| `HYPERSHELL_MANAGED_CLUSTER_NAME` | Human-readable name, unique per control plane (e.g. `hyp0-mc1`, or `local` in dev) |
| `OIDC_CLIENT_ID` / `OIDC_CLIENT_SECRET` | Existing control plane credentials; no new secret types. Omitted in local development, where the API server runs with authentication disabled and registration is by name alone. |

`HYPERSHELL_CLUSTER_ID` is resolved at runtime and SHALL NOT appear in gitops.

---

## RBAC

When the API server has authentication enabled, the `managed-cluster-registrar` role is required on both the initial registration call and every subsequent loop call. A control plane without the role receives 403 on its first call and cannot start. When authentication is disabled (local development), no role is required.

**Enforcement mechanism:** `managed-cluster-registrar` is a JWT-direct role. The `isAuthorized` function in the HTTP authorization middleware has a dedicated case for `POST managed_clusters/registration` that checks the JWT claim directly, bypassing the `hasGatewayCreator` fallback. The role is NOT in `JWTSyncedRoles` and has no DB RoleBinding lifecycle. See `security/rbac-enforcement.spec.md` for the implementation contract.

An administrator assigns `managed-cluster-registrar` to the control plane's OIDC client in Keycloak before the control plane is deployed. This is an explicit, out-of-band admin step - it is not automated. Keycloak is the trusted source of truth; the API server does not re-verify role assignment beyond reading the JWT claim.

**Isolation guarantee:** In production (`RBAC_DEFAULT_ROLES=`, `RBAC_ENFORCE=true`), a control plane holding only `managed-cluster-registrar` has no gateway permissions. All permissions flow exclusively from Keycloak. A control plane gains gateway access only if an administrator also explicitly grants `gateway:creator` or a gateway-scoped binding in Keycloak.

---

## Requirements

### Requirement: Idempotent Registration

`POST /managed_clusters/registration` SHALL be idempotent on `name`. `name` is the sole upsert key; `oidc_subject` is stored as an audit field and plays no part in record lookup.

- On first call: create a `ManagedCluster` record with a new KSUID, set `oidc_subject` from the JWT `sub` claim (empty when authentication is disabled), set `last_seen_at` to now. Return 201 with `cluster_id`.
- On subsequent calls with the same name: update `last_seen_at` to now. Return 200 with the existing `cluster_id`.

The upsert SHALL use database-level locking to handle concurrent first-time requests safely.

#### Scenario: First-time registration

- GIVEN a control plane with `managed-cluster-registrar` that has never registered
- WHEN it calls `POST /managed_clusters/registration` with `name: hyp0-mc1`
- THEN a new `ManagedCluster` record is created with a stable KSUID
- AND `oidc_subject` is set to the JWT `sub` claim
- AND `last_seen_at` is set to now
- AND the response is 201 with `cluster_id`

#### Scenario: Re-registration is idempotent

- GIVEN a control plane that previously registered and received `cluster_id: X`
- WHEN it calls `POST /managed_clusters/registration` again with the same `name`
- THEN no new record is created
- AND `last_seen_at` is updated to now
- AND the response is 200 with `cluster_id: X`

#### Scenario: Two control planes sharing a name resolve to the same cluster_id

- GIVEN two separately-deployed control planes both configured with `name: hyp0-mc1`
- WHEN each calls `POST /managed_clusters/registration`
- THEN both receive the same `cluster_id` (the record created by whichever registered first)
- AND both will reconcile gateways assigned to that cluster_id simultaneously
- NOTE: the API permits this; preventing duplicate names is a provisioning responsibility

#### Scenario: Local development without authentication

- GIVEN the API server runs with authentication disabled
- AND a control plane that has never registered
- WHEN it calls `POST /managed_clusters/registration` with `name: local` and no token
- THEN a new `ManagedCluster` record is created with a stable KSUID
- AND `oidc_subject` is empty
- AND the record is keyed on `name` for subsequent idempotent calls

### Requirement: Role Enforcement

When the API server has authentication enabled, the `/registration` endpoint SHALL require the `managed-cluster-registrar` role in the caller's JWT. A caller without the role SHALL receive 403 Forbidden before any database operation. When authentication is disabled, this requirement does not apply.

#### Scenario: Missing role rejected

- GIVEN a control plane service account without `managed-cluster-registrar` in Keycloak
- WHEN it calls `POST /managed_clusters/registration`
- THEN the response is 403 Forbidden
- AND no `ManagedCluster` record is created

### Requirement: last_seen_at Updated on Every Call

Every successful call to `/registration` SHALL update `last_seen_at` on the matching `ManagedCluster` record, including calls that are no-ops for the registration data itself.

#### Scenario: Heartbeat loop updates last_seen_at

- GIVEN a registered control plane calling `/registration` every 60 seconds
- WHEN each call completes successfully
- THEN `last_seen_at` on the `ManagedCluster` record is updated to the call time
- AND the response returns the same `cluster_id` each time

### Requirement: oidc_subject Is Server-Assigned and Immutable

`oidc_subject` SHALL be set by the API server from the validated JWT `sub` claim when a token is present, and left empty when authentication is disabled. It SHALL NOT be accepted as an input field on any request. It SHALL NOT be modifiable via `PATCH /managed-clusters/{id}`.

### Requirement: Fail-Closed Startup

The control plane SHALL NOT open the `WatchGateways` gRPC stream until a successful `/registration` response has been received. On registration failure at startup, the control plane SHALL retry with exponential backoff indefinitely, logging the error on each attempt. It SHALL NOT proceed with an unresolved `cluster_id`.

The only exception is a non-retryable response (403 Forbidden): if the API server returns 403, the control plane lacks the required Keycloak role and retrying will not help. In this case the control plane SHALL log the error and exit, surfacing a clear message that `managed-cluster-registrar` must be assigned in Keycloak. (403 can occur only when the API server has authentication enabled.)

#### Scenario: Transient failure retried with backoff

- GIVEN the API server is temporarily unreachable (network partition, restart)
- WHEN the control plane attempts to register at startup
- THEN it SHALL retry with exponential backoff
- AND it SHALL NOT open `WatchGateways` until registration succeeds

#### Scenario: 403 exits immediately

- GIVEN the API server returns 403 (role not yet assigned in Keycloak)
- WHEN the control plane attempts to register at startup
- THEN it SHALL NOT retry
- AND it SHALL log a clear error identifying the missing `managed-cluster-registrar` role and exit

---

## Design Decisions

| Decision | Rationale |
|----------|-----------|
| Single `/registration` endpoint for both register and heartbeat | Eliminates a separate heartbeat endpoint. The idempotent registration call already has all the information needed to update `last_seen_at`. Fewer endpoints, simpler RBAC surface. |
| Narrow response body (`{ "cluster_id" }` only, not full ManagedCluster) | The control plane needs exactly one thing from registration: its stable `cluster_id` to use as the `WatchGateways` filter. Returning the full ManagedCluster object would expose fields the control plane cannot and should not act on. The narrow shape is intentional and differs from the standard `GET /managed_clusters/{id}` response by design. |
| 403 exits immediately; other failures retry with backoff | A 403 means the Keycloak role is absent -- retrying is pointless and delays operator awareness. Network or 5xx errors are transient; exponential backoff recovers automatically without operator intervention. |
| `name` as sole upsert key | Using `(oidc_subject, name)` ties uniqueness to the OIDC provider, creating a dependency on Keycloak for cluster identity and preventing multiple control planes per node. With `name` alone, uniqueness is a provisioning contract (GitOps assigns each CP a distinct name), the API server is OIDC-agnostic for identity purposes, and any process with the right name and `managed-cluster-registrar` role can recover or replace a failed control plane without admin intervention. |
| `last_seen_at` as passive liveness, not a status field | Keeps the control plane's self-reported liveness separate from the reconciler's view of cluster state. The `status` field remains the reconciler's domain. |
| No active health probing from the API server | Control planes call in; the API server does not need to reach out. Avoids reverse credential management and works across network topologies where the API server cannot initiate connections to control planes. |
| Role assigned by admin, not auto-granted | The `managed-cluster-registrar` role is a privilege gate. A Keycloak admin must explicitly grant it before a control plane can self-register, providing a human control point for fleet membership. |
| JWT-direct enforcement, not DB-synced | `managed-cluster-registrar` is not in `JWTSyncedRoles` because it should never be auto-assigned (unlike `gateway:creator`) and does not need a DB binding lifecycle. A live JWT claim check in `isAuthorized` is sufficient and avoids polluting the sync table with a role that applies to a narrow class of service accounts. |
