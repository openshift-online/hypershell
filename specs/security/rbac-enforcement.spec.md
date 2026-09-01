# RBAC Enforcement

**Date:** 2026-08-12
**Status:** Draft
**Related:** `platform/data-model.spec.md` (domain model), `platform/openshell-gateway-oidc.spec.md` (gateway OIDC), `standards/security/security.spec.md` (security standards)
**OpenShell gateway service accounts:** `platform/openshell-gateway-service-accounts.spec.md` (delegated service-account identities)

---

## Purpose

The HyperShell API server SHALL enforce authorization on all API endpoints (HTTP and
gRPC) using a four-role model backed by Keycloak as the source of truth for platform-wide
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

Built-in roles are seeded at migration time. Four roles cover all required access
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
| `platform:admin` | global | Keycloak JWT | Platform-wide administration; can view and delete any gateway |
| `gateway:creator` | global | Keycloak JWT | Can create gateways; auto-becomes `gateway:owner` on creation |
| `gateway:owner` | per gateway | DB (app logic) | Full CRUD on one gateway; can grant `gateway:owner` and `gateway:viewer` to others |
| `gateway:viewer` | per gateway | DB (app logic) | Read-only access to one gateway |

### Permission Matrix

| Role | Gateways | Gateway CRUD | RBAC Grants | OpenShell Mapping | OpenShellGatewayServiceAccounts |
|------|----------|-------------|-------------|-------------------|-----------------|
| `platform:admin` | view all, delete any | view all + delete any | -- | -- | None without a gateway binding |
| `gateway:creator` | create + own gateways | full (as owner) | grant owner/viewer on own gateways | `openshell-admin` on own gateways | Through the resulting owner binding |
| `gateway:owner` | full (one gateway) | full | grant owner/viewer on that gateway | `openshell-admin` on that gateway | Select `openshell-user` or `openshell-admin`. Manage all OpenShellGatewayServiceAccounts on the gateway. |
| `gateway:viewer` | read (one gateway) | read only | -- | `openshell-user` on that gateway | Select only `openshell-user`. Manage only their own OpenShellGatewayServiceAccounts. |

### OpenShell Role Bridge

When a user accesses a gateway directly via the `openshell` CLI, the gateway's OIDC
configuration maps HyperShell roles to OpenShell roles:

| HyperShell Role | OpenShell Role |
|-----------------|----------------|
| `gateway:owner` | `openshell-admin` |
| `gateway:viewer` | `openshell-user` |

The `platform:admin` role provides visibility and lifecycle management through the
HyperShell web console but does NOT grant OpenShell CLI access to gateways. Platform
administrators who need to use the `openshell` CLI for a specific gateway must be
granted `gateway:owner` or `gateway:viewer` on that gateway.

---

## Requirements

### Requirement: Keycloak as Authority for Platform Roles

Keycloak is the source of truth for the `gateway:creator` and `platform:admin` roles. A
Keycloak administrator assigns these roles to users and service accounts via Keycloak's
admin console or API.

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

The middleware SHALL also sync platform-wide RoleBindings (both `gateway:creator` and
`platform:admin`) from the JWT on every request, creating or removing bindings as
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

### Requirement: Platform Admin Global Access

Users with the `platform:admin` role SHALL have global view and delete permissions across
all gateways in the platform, regardless of per-gateway RoleBindings. This role is assigned
via Keycloak and synced to the database identically to `gateway:creator`.

Platform administrators SHALL be able to:

- List all gateways across the platform (GET `/api/hypershell/v1/gateways` returns all)
- View any specific gateway (GET `/api/hypershell/v1/gateways/{id}` succeeds for any ID)
- Delete any gateway (DELETE `/api/hypershell/v1/gateways/{id}` succeeds for any ID)

Platform administrators SHALL NOT be able to:

- Modify gateway configuration (PATCH/PUT operations require `gateway:owner`)
- Grant or revoke RoleBindings (requires `gateway:owner` on that specific gateway)
- Create gateways (requires `gateway:creator` role)

The `platform:admin` role is orthogonal to `gateway:creator`, `gateway:owner`, and
`gateway:viewer`. A user may hold multiple roles (e.g., `platform:admin` + `gateway:creator`).

In the initial implementation, the `platform:admin` role is **limited to gateway view and delete operations**. It does NOT grant permissions to:

- View or modify GatewayNetworks, GatewayReleases, ManagedClusters, or ManagedDatabases
- View or modify Users or RoleBindings
- Access platform-level configuration or system administration functions

Future iterations may expand platform:admin permissions to include these resources.

#### Scenario: Platform admin views all gateways

- GIVEN user A has `platform:admin` (from Keycloak)
- AND there are 50 gateways in the platform owned by various users
- WHEN user A calls `GET /api/hypershell/v1/gateways`
- THEN the response includes all 50 gateways
- AND the response is paginated
- AND includes metadata for total count and pagination

#### Scenario: Platform admin views a specific gateway

- GIVEN user A has `platform:admin`
- AND user A has no `gateway:owner` or `gateway:viewer` binding for gw-1
- WHEN user A calls `GET /api/hypershell/v1/gateways/gw-1`
- THEN the response is 200 OK with the gateway details

#### Scenario: Platform admin deletes any gateway

- GIVEN user A has `platform:admin`
- AND user B owns gw-1 (has `gateway:owner` binding)
- AND user A has no binding for gw-1
- WHEN user A calls `DELETE /api/hypershell/v1/gateways/gw-1`
- THEN the gateway is deleted
- AND the response is 204 No Content

#### Scenario: Platform admin cannot modify gateways without ownership

- GIVEN user A has `platform:admin` only
- AND user A has no `gateway:owner` binding for gw-1
- WHEN user A calls `PATCH /api/hypershell/v1/gateways/gw-1`
- THEN the response is 403 Forbidden

#### Scenario: Platform admin cannot create gateways without creator role

- GIVEN user A has `platform:admin` only
- AND user A does NOT have `gateway:creator`
- WHEN user A calls `POST /api/hypershell/v1/gateways`
- THEN the response is 403 Forbidden

#### Scenario: Platform admin cannot grant role bindings

- GIVEN user A has `platform:admin` only
- AND user A has no `gateway:owner` binding for gw-1
- WHEN user A calls `POST /api/hypershell/v1/role_bindings` with `gateway_id=gw-1`
- THEN the response is 403 Forbidden

#### Scenario: User with platform:admin and gateway:creator can create and view all

- GIVEN user A has both `platform:admin` and `gateway:creator`
- WHEN user A calls `POST /api/hypershell/v1/gateways`
- THEN the gateway is created
- AND user A becomes `gateway:owner` of the new gateway
- AND user A can still view all other gateways via `platform:admin`

### Requirement: Platform Admin UI Experience

The web UI SHALL support `platform:admin` users as defined in `web-console/architecture.spec.md` requirement WEB-ARCH-02 and WEB-UI-03A.

Platform administrators SHALL see all gateways in the gateway list, which SHALL use API-backed pagination and search as specified in WEB-DATA-01.

Each gateway row SHALL display a delete action for platform administrators. Delete actions SHALL require confirmation via a modal dialog before calling `DELETE /api/hypershell/v1/gateways/{id}`.

### Requirement: gRPC Authorization

gRPC handlers SHALL enforce the same authorization rules as HTTP handlers. The gRPC
authorization interceptor SHALL extract the caller identity from the request metadata
and evaluate permissions using the same role-based logic as the HTTP middleware.

The middleware SHALL provision users and sync JWT roles on gRPC requests identically
to HTTP requests.

#### Scenario: Platform admin watches gateways via gRPC

- GIVEN user A has `platform:admin` (from Keycloak)
- AND the control plane watches gateway events via gRPC
- WHEN a gRPC client with user A's credentials calls `WatchGateways`
- THEN the stream includes events for all gateways in the platform
- AND the authorization interceptor evaluates `platform:admin` identically to HTTP handlers

#### Scenario: Platform admin cannot modify via gRPC without ownership

- GIVEN user A has `platform:admin` but no `gateway:owner` binding for gateway gw-1
- WHEN a gRPC client with user A's credentials attempts to update gateway gw-1
- THEN the request returns PermissionDenied
- AND the response does not leak that gw-1 exists

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

### Requirement: Audit Logging

Platform administrator actions SHALL be logged with:

- Action performed (view, delete)
- Actor identity (platform:admin user)
- Target resource (gateway ID and name)
- Timestamp and correlation ID

High-privilege operations (gateway deletion by platform:admin) SHALL be logged at INFO
level or higher to ensure visibility in operational monitoring and security audits.

### Requirement: Production Rollout

RBAC enforcement SHALL be gated behind the `RBAC_ENFORCE` configuration flag. When
disabled, all authenticated requests pass. When enabled, all requests are evaluated
against bindings.

The first `gateway:creator` and `platform:admin` users are provisioned by assigning the
roles in Keycloak. No database migration or CLI command is needed for bootstrapping users
-- only the built-in Role records are seeded via migration; RoleBindings are created
dynamically from JWT claims.

### Requirement: Database Migration

A database migration SHALL seed the `platform:admin` role record with:

- `name: "platform:admin"`
- `display_name: "Platform Administrator"`
- `description: "Platform-wide view and delete access for all gateways"`
- `built_in: true`

This migration SHALL run alongside the existing migrations that seed `gateway:creator`,
`gateway:owner`, and `gateway:viewer` roles.

RoleBindings from JWT claims are synced regardless of whether enforcement is enabled,
ensuring bindings exist before enforcement is turned on.

#### Operator Note: Enabling Enforcement Is a Breaking Change

The OpenShift overlay (`deploy/openshift/kustomization.yaml`) ships with
`RBAC_ENFORCE=true`. Applying it to an existing cluster is a breaking change: from
that point every gateway operation requires the caller's token to carry
`gateway:creator` (create) or a matching per-gateway RoleBinding (read/write).

On a real cluster the IdP is external SSO (not the bundled Keycloak), so the
following MUST be in place BEFORE -- or atomically with -- the rollout:

- The external SSO defines both `gateway:creator` and `platform:admin` realm roles.
- Platform operators who need to create gateways are assigned `gateway:creator`.
- Platform operators who need global view/delete access are assigned `platform:admin`.
- The SSO emits these roles in the `realm_access.roles` claim of issued access tokens
  (or the equivalent configurable claim path the API server reads).

If enforcement is turned on before the roles are wired, gateway operations fail
immediately after upgrade with no other symptom -- a silent, hard-to-diagnose outage:
- Gateway create calls return 403 without `gateway:creator`
- Gateway reads return 404 without appropriate ownership or `platform:admin`
- Gateway deletes return 403 without ownership or `platform:admin`

Coordinate the SSO role mapping and the overlay rollout together, and call out these
SSO prerequisites in the release notes for the version that makes the overlay default
enforce RBAC.

### Requirement: Integration Test Coverage

Integration tests SHALL exercise RBAC enforcement with the new four-role model.

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
| Four roles model | Minimal model that covers the use cases: platform administration (view/delete all), create gateways, own gateways, view gateways. No fleet-scoped RBAC needed. |
| `platform:admin` is view + delete only | Platform admins handle operational tasks (viewing all gateways, cleaning up orphaned resources). Full modification requires ownership to prevent accidental changes. Separation of concerns: visibility ≠ modification authority. |
| `platform:admin` orthogonal to `gateway:creator` | A platform admin may or may not create gateways. Roles compose: `platform:admin` + `gateway:creator` allows both operational oversight and resource creation. |
| JWT roles synced to DB on every request | DB is the projection, Keycloak is the authority. Revocations in Keycloak take effect immediately. Existing per-gateway bindings are unaffected by platform role changes. |
| Service accounts treated identically to users | Control plane gets `gateway:creator` in Keycloak, provisions like any user. No special bypass logic needed. |
| Gateway owners can grant co-owners | No hierarchy restriction. Team leads assign `gateway:creator` to team members or invite them as owners/viewers per gateway. Simple mental model. |
| Auto-assign `gateway:owner` on creation | Creator automatically owns what they create. No separate grant step needed. |
| `gateway:creator` from Keycloak only | Cannot be self-assigned via the API. A Keycloak admin decides who can create gateways. |
| Per-gateway bindings stored in DB | Gateway-scoped access requires per-resource granularity that JWT claims cannot provide (you'd need dynamic claim values per gateway ID). |
| No resource grouping as a security boundary | The Sector/Fleet grouping was removed. RBAC operates at platform level (creator) and gateway level (owner/viewer); there is no fleet-scoped isolation. |
| 404 on unauthorized singleton GETs | Returning 403 confirms the resource exists. 404 prevents ID enumeration. |
| OpenShell role bridge | `gateway:owner` maps to `openshell-admin`, `gateway:viewer` maps to `openshell-user`. Ensures consistent access via CLI. |
| OpenShellGatewayServiceAccount role limit | A gateway binding limits the selected OpenShell role. Owners can select `openshell-user` or `openshell-admin`. Viewers can select only `openshell-user`. Each OpenShellGatewayServiceAccount remains bound to its creator. A binding change can downgrade or revoke it. |
