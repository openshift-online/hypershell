# RBAC Enforcement

**Date:** 2026-08-12
**Status:** Draft
**Related:** `platform/data-model.spec.md` (domain model), `platform/openshell-gateway-oidc.spec.md` (gateway OIDC), `standards/security/security.spec.md` (security standards)

---

## Purpose

The HyperShell API server SHALL enforce authorization on all API endpoints (HTTP and
gRPC) using a three-role model backed by Keycloak as the source of truth for platform-wide
roles and a PostgreSQL-backed RoleBinding model for per-gateway grants.

Keycloak is the authority for identity and platform-wide role assignment. A privileged
administrator assigns Keycloak roles (e.g., `gateway:creator`) to users and service
accounts. The API server middleware reads JWT claims, lazily provisions User and
RoleBinding records, and evaluates authorization against the database projection.

Users start with zero permissions and gain access by receiving a Keycloak role
(`gateway:creator`) or being granted a per-gateway binding (`gateway:owner`,
`gateway:viewer`) by an existing gateway owner.

---

## Data Model

### User

Auto-provisioned from JWT claims on first authenticated request. No explicit registration
endpoint is required. Users and service accounts are treated identically.

```
User {
    string ID PK
    string username
    string email
    string name
    time   created_at
    time   updated_at
    time   deleted_at
}
```

### Role

Built-in roles are seeded at migration time. Three roles cover all required access
patterns.

```
Role {
    string ID PK
    string name
    string display_name
    string description
    jsonb  permissions
    bool   built_in
    time   created_at
    time   updated_at
    time   deleted_at
}
```

### RoleBinding

Binds a Role to a User at a given scope. For `gateway:creator`, scope is `global` with
no resource FK. For `gateway:owner` and `gateway:viewer`, scope is `gateway` with
`gateway_id` identifying the bound gateway.

```
RoleBinding {
    string ID PK
    string role_id FK
    string scope         "global | gateway"
    string user_id FK    "who holds the binding"
    string gateway_id FK "nullable -- set when scope=gateway"
    time   created_at
    time   updated_at
    time   deleted_at
}
```

### Entity Relationships

```
User        }o--o{ RoleBinding : "user_id"
Gateway     }o--o{ RoleBinding : "gateway_id"
Role        ||--o{ RoleBinding : "granted_by"
```

---

## Built-in Roles

| Role | Scope | Source | Purpose |
|------|-------|--------|---------|
| `gateway:creator` | global | Keycloak JWT | Can create gateways; auto-becomes `gateway:owner` on creation |
| `gateway:owner` | per gateway | DB (app logic) | Full CRUD on one gateway; can grant `gateway:owner` and `gateway:viewer` to others |
| `gateway:viewer` | per gateway | DB (app logic) | Read-only access to one gateway |

### Permission Matrix

| Role | Gateways | Gateway CRUD | RBAC Grants | OpenShell Mapping |
|------|----------|-------------|-------------|-------------------|
| `gateway:creator` | create + own gateways | full (as owner) | grant owner/viewer on own gateways | `openshell:admin` on own gateways |
| `gateway:owner` | full (one gateway) | full | grant owner/viewer on that gateway | `openshell:admin` on that gateway |
| `gateway:viewer` | read (one gateway) | read only | -- | `openshell:user` on that gateway |

### OpenShell Role Bridge

When a user accesses a gateway directly via the `openshell` CLI, the gateway's OIDC
configuration maps HyperShell roles to OpenShell roles:

| HyperShell Role | OpenShell Role |
|-----------------|----------------|
| `gateway:owner` | `openshell:admin` |
| `gateway:viewer` | `openshell:user` |

---

## Requirements

### Requirement: Keycloak as Authority for Platform Roles

Keycloak is the source of truth for the `gateway:creator` role. A Keycloak administrator
assigns this role to users and service accounts via Keycloak's admin console or API.

The API server middleware SHALL extract roles from the JWT `realm_access.roles` claim
(or an equivalent configurable claim path) and lazily create or update the corresponding
RoleBinding records in the database on each authenticated request.

This ensures that Keycloak role changes take effect on the next API request without
requiring a separate sync process.

#### Scenario: Keycloak admin assigns gateway:creator

- GIVEN a Keycloak admin assigns the `gateway:creator` realm role to user A
- WHEN user A makes their first API request with a JWT containing `gateway:creator`
- THEN the middleware creates a User record and a `gateway:creator` RoleBinding
- AND user A can create gateways

#### Scenario: Keycloak admin revokes gateway:creator

- GIVEN user A previously had `gateway:creator` assigned in Keycloak
- WHEN the Keycloak admin removes the role
- THEN user A's next API request carries a JWT without `gateway:creator`
- AND the middleware removes the corresponding RoleBinding
- AND user A can no longer create new gateways
- AND existing `gateway:owner` bindings on previously-created gateways are unaffected

### Requirement: Service Account Support

Service accounts (e.g., the control plane) are Keycloak clients using the
`client_credentials` grant. They receive the same JWT structure and role claims as
human users. The middleware provisions them identically -- a User record is created
for the service account's client ID, and RoleBindings are created from their JWT roles.

The control plane service account SHALL be assigned `gateway:creator` in Keycloak,
enabling it to create, watch, and manage all gateways it creates.

#### Scenario: Control plane authenticates and operates

- GIVEN the control plane service account has `gateway:creator` in Keycloak
- AND its client ID and secret are stored in a Kubernetes Secret
- WHEN the control plane connects via gRPC with a `client_credentials` JWT
- THEN the middleware provisions a User for the service account
- AND creates a `gateway:creator` RoleBinding
- AND the control plane can create and manage gateways

### Requirement: User Auto-Provisioning

The middleware SHALL automatically create a User record when a JWT-authenticated caller
is seen for the first time. The User record SHALL be populated from standard OIDC claims
(`preferred_username`, `email`, `given_name`, `family_name`).

Auto-provisioning SHALL use upsert semantics (keyed on `username`) to handle concurrent
first-time requests and profile updates.

The middleware SHALL also sync platform-wide RoleBindings (currently only
`gateway:creator`) from the JWT on every request, creating or removing bindings as
needed to match the JWT claims.

#### Scenario: First-time user auto-provisioned

- GIVEN a user authenticates via SSO for the first time
- WHEN any authenticated API request is processed
- THEN a User record is created from the JWT claims
- AND RoleBindings are created to match the JWT's role claims
- AND the request proceeds to authorization evaluation

### Requirement: Gateway Creation Bootstrap

Any authenticated user with the `gateway:creator` role SHALL be able to create gateways.
On successful gateway creation, the system SHALL automatically create a `gateway:owner`
RoleBinding for the authenticated user, scoped to the new gateway.

This binding is created in the same database transaction as the gateway.

#### Scenario: Creator creates a gateway and becomes owner

- GIVEN user A has `gateway:creator` (from Keycloak)
- WHEN user A calls `POST /api/hypershell/v1/gateways`
- THEN the gateway is created
- AND a `gateway:owner` RoleBinding is created for user A on the new gateway
- AND user A can immediately manage the gateway

#### Scenario: User without creator role cannot create gateways

- GIVEN user A has only `gateway:viewer` on some gateway
- WHEN user A calls `POST /api/hypershell/v1/gateways`
- THEN the request returns 403 Forbidden

### Requirement: Per-Gateway Authorization

The authorization middleware SHALL evaluate permissions against the binding's gateway
scope. A binding with `scope=gateway` and `gateway_id=gw-1` SHALL only authorize
access to gateway `gw-1`.

`gateway:creator` grants the ability to create new gateways and acts as `gateway:owner`
on all gateways the user owns (has a `gateway:owner` binding for).

#### Scenario: Gateway owner can manage their gateway

- GIVEN user A has `gateway:owner` on gw-1
- WHEN user A calls `PATCH /api/hypershell/v1/gateways/gw-1`
- THEN the request is authorized

#### Scenario: Gateway viewer cannot modify

- GIVEN user A has `gateway:viewer` on gw-1
- WHEN user A calls `PATCH /api/hypershell/v1/gateways/gw-1`
- THEN the response is 403 Forbidden

#### Scenario: No binding returns 404

- GIVEN user A has no binding covering gw-2
- WHEN user A calls `GET /api/hypershell/v1/gateways/gw-2`
- THEN the response is 404 (existence not disclosed)

### Requirement: RoleBinding Grants

Gateway owners can grant `gateway:owner` or `gateway:viewer` to other users on gateways
they own. There is no hierarchy restriction -- owners can make more owners.

#### Scenario: Owner invites a viewer

- GIVEN user A has `gateway:owner` on gw-1
- WHEN user A calls `POST /api/hypershell/v1/role_bindings` with `role=gateway:viewer`, `gateway_id=gw-1`, `user_id=B`
- THEN the binding is created
- AND user B gains read-only access to gw-1

#### Scenario: Owner invites a co-owner

- GIVEN user A has `gateway:owner` on gw-1
- WHEN user A calls `POST /api/hypershell/v1/role_bindings` with `role=gateway:owner`, `gateway_id=gw-1`, `user_id=B`
- THEN the binding is created
- AND user B gains full access to gw-1

#### Scenario: Viewer cannot grant

- GIVEN user A has only `gateway:viewer` on gw-1
- WHEN user A calls `POST /api/hypershell/v1/role_bindings` with any role on gw-1
- THEN the request returns 403 Forbidden

#### Scenario: Cannot grant on a gateway you don't own

- GIVEN user A has `gateway:owner` on gw-1 only
- WHEN user A calls `POST /api/hypershell/v1/role_bindings` with `gateway_id=gw-2`
- THEN the request returns 403 Forbidden

### Requirement: gRPC Authorization

gRPC handlers SHALL enforce the same authorization rules as HTTP handlers. The gRPC
authorization interceptor SHALL extract the caller identity from the request metadata
and evaluate permissions using the same role-based logic as the HTTP middleware.

The middleware SHALL provision users and sync JWT roles on gRPC requests identically
to HTTP requests.

### Requirement: Error Response Opacity

For singleton resource endpoints (`GET /gateways/{id}`), the middleware SHALL return 404
when the caller has no binding that covers the requested resource. Returning 403 on a
singleton GET leaks resource existence.

For list endpoints, the middleware SHALL return 200 with an empty items array when the
caller has no matching resources.

For mutation endpoints where the caller lacks write permission, the middleware SHALL
return 403.

### Requirement: Auth-Exempt Endpoints

The following endpoints SHALL require only authentication (valid JWT), not authorization:

| Endpoint | Reason |
|----------|--------|
| `GET /api/hypershell/v1/roles` | Discovery -- users need to see available roles |
| `GET /api/hypershell/v1/roles/{id}` | Discovery -- read a specific role's permissions |

Health, metrics, and version endpoints are already bypassed at the authentication
layer.

### Requirement: Production Rollout

RBAC enforcement SHALL be gated behind the `RBAC_ENFORCE` configuration flag. When
disabled, all authenticated requests pass. When enabled, all requests are evaluated
against bindings.

The first `gateway:creator` user is provisioned by assigning the role in Keycloak. No
database migration or CLI command is needed for bootstrapping.

RoleBindings from JWT claims are synced regardless of whether enforcement is enabled,
ensuring bindings exist before enforcement is turned on.

### Requirement: Integration Test Coverage

Integration tests SHALL exercise RBAC enforcement with the new three-role model.

---

## API Reference

| Method | Path | Operation |
|--------|------|-----------|
| GET | `/api/hypershell/v1/roles` | List all roles |
| GET | `/api/hypershell/v1/roles/{id}` | Get a specific role |
| GET | `/api/hypershell/v1/role_bindings` | List role bindings |
| GET | `/api/hypershell/v1/role_bindings/{id}` | Get a role binding |
| POST | `/api/hypershell/v1/role_bindings` | Create a role binding |
| DELETE | `/api/hypershell/v1/role_bindings/{id}` | Delete a role binding |

---

## Design Decisions

| Decision | Rationale |
|----------|-----------|
| Keycloak as authority for platform roles | Centralizes role management. Eliminates need for admin-seeding CLI or DB migration. Role changes take effect on next JWT. |
| Three roles only | Minimal model that covers the use cases: create gateways, own gateways, view gateways. No fleet-scoped RBAC needed. |
| JWT roles synced to DB on every request | DB is the projection, Keycloak is the authority. Revocations in Keycloak take effect immediately. Existing per-gateway bindings are unaffected by platform role changes. |
| Service accounts treated identically to users | Control plane gets `gateway:creator` in Keycloak, provisions like any user. No special bypass logic needed. |
| Gateway owners can grant co-owners | No hierarchy restriction. Team leads assign `gateway:creator` to team members or invite them as owners/viewers per gateway. Simple mental model. |
| Auto-assign `gateway:owner` on creation | Creator automatically owns what they create. No separate grant step needed. |
| `gateway:creator` from Keycloak only | Cannot be self-assigned via the API. A Keycloak admin decides who can create gateways. |
| Per-gateway bindings stored in DB | Gateway-scoped access requires per-resource granularity that JWT claims cannot provide (you'd need dynamic claim values per gateway ID). |
| Fleet is not a security boundary | Fleet is an organizational grouping. RBAC operates at platform level (creator) and gateway level (owner/viewer). |
| 404 on unauthorized singleton GETs | Returning 403 confirms the resource exists. 404 prevents ID enumeration. |
| OpenShell role bridge | `gateway:owner` maps to `openshell:admin`, `gateway:viewer` maps to `openshell:user`. Ensures consistent access via CLI. |
