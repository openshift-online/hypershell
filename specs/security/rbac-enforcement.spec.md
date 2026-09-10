# RBAC Enforcement

**Date:** 2026-08-12
**Status:** Draft
**Related:** `platform/data-model.spec.md` (domain model), `platform/openshell-gateway-oidc.spec.md` (gateway OIDC), `standards/security/security.spec.md` (security standards)
**OpenShell gateway service accounts:** `platform/openshell-gateway-service-accounts.spec.md` (delegated service-account identities)

---

## Purpose

The HyperShell API server SHALL enforce authorization on all API endpoints (HTTP and
gRPC) using a five-role model backed by Keycloak as the source of truth for platform-wide
roles and a PostgreSQL-backed RoleBinding model for per-gateway grants.

Keycloak is the authority for identity and platform-wide role assignment. The API server
middleware reads JWT claims, lazily provisions User and RoleBinding records, and evaluates
authorization against the database projection.

**Production posture: Keycloak-only.** Production deployments SHALL run with
`RBAC_ENFORCE=true` and `RBAC_DEFAULT_ROLES=` (explicit empty string). No role is
auto-assigned; every gateway and fleet permission must be explicitly granted in Keycloak.
This applies equally to human users, control-plane service accounts, and spoke service
accounts.

`RBAC_DEFAULT_ROLES=gateway:creator` exists as a local-development convenience to avoid
stranding users before Keycloak is fully configured. It SHALL NOT be set in any
production or staging overlay.

Users gain gateway access by being assigned `gateway:creator` or `platform:admin` in
Keycloak, or by being granted a per-gateway binding (`gateway:owner`, `gateway:viewer`)
by an existing gateway owner.

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

Built-in roles are seeded at migration time. Four roles are DB-backed; one additional
role (`managed-cluster-registrar`) is seeded for discoverability but enforced via
JWT-direct check (not a DB-backed RoleBinding).

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
| `managed-cluster-registrar` | global | Keycloak JWT (direct, no DB binding) | Allows a spoke control-plane service account to call `POST /managed_clusters/registration`; checked live from JWT claim, not via `JWTSyncedRoles` or DB RoleBinding |

### Permission Matrix

| Role | Gateways | Gateway CRUD | RBAC Grants | OpenShell Mapping | OpenShellGatewayServiceAccounts | ManagedCluster Registration |
|------|----------|-------------|-------------|-------------------|-----------------|-----------------|
| `platform:admin` | view all, delete any | view all + delete any | -- | -- | None without a gateway binding | -- |
| `gateway:creator` | create + own gateways | full (as owner) | grant owner/viewer on own gateways | `openshell-admin` on own gateways | Through the resulting owner binding | -- |
| `gateway:owner` | full (one gateway) | full | grant owner/viewer on that gateway | `openshell-admin` on that gateway | Select `openshell-user` or `openshell-admin`. Manage all OpenShellGatewayServiceAccounts on the gateway. | -- |
| `gateway:viewer` | read (one gateway) | read only | -- | `openshell-user` on that gateway | Select only `openshell-user`. Manage only their own OpenShellGatewayServiceAccounts. | -- |
| `managed-cluster-registrar` | none (unless also granted `gateway:creator` via defaults) | none | none | none | none | `POST /registration` (register + heartbeat loop) -- enforced by JWT-direct check in `isAuthorized`, not a DB binding |

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

- GIVEN production deployment with `RBAC_DEFAULT_ROLES=`
- AND user A previously had `gateway:creator` assigned in Keycloak
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

- GIVEN production deployment with `RBAC_DEFAULT_ROLES=`
- AND user A has only `gateway:viewer` on some gateway
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

- GIVEN production deployment with `RBAC_DEFAULT_ROLES=`
- AND user A has `platform:admin` only (no `gateway:creator` in Keycloak)
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

### Requirement: Default Role Bootstrap (Development Mode Only)

The API server supports a configurable set of default roles applied to every authenticated
user, controlled by `RBAC_DEFAULT_ROLES` (comma-separated role names). This feature
exists solely as a local-development convenience.

- Production and staging deployments SHALL set `RBAC_DEFAULT_ROLES=` (explicit empty).
- Local development may set `RBAC_DEFAULT_ROLES=gateway:creator` to avoid configuring
  Keycloak roles before testing. This MUST NOT reach any production or staging environment.
- Default roles are merged alongside JWT-carried roles on every request.
- Only roles in `JWTSyncedRoles` are eligible; a startup warning is emitted otherwise.
- Because defaults re-apply on every request, Keycloak cannot revoke a role that is also
  a default. This is the defining reason defaults must be disabled in production.

#### Scenario: Local development with defaults (NOT for production)

- GIVEN `RBAC_DEFAULT_ROLES=gateway:creator` (local dev only)
- AND a developer authenticates with a JWT carrying no Keycloak realm roles
- WHEN any authenticated API request is processed
- THEN the middleware creates a `gateway:creator` RoleBinding automatically
- AND the developer can create gateways without Keycloak configuration

#### Scenario: Production mode -- no defaults, Keycloak is authoritative

- GIVEN `RBAC_DEFAULT_ROLES=` (production)
- AND `RBAC_ENFORCE=true`
- AND a user has no Keycloak realm roles assigned
- WHEN the user makes an authenticated API request
- THEN no default binding is created
- AND the user cannot create gateways until a Keycloak admin assigns `gateway:creator`

### Requirement: Production Rollout

RBAC enforcement SHALL be gated behind the `RBAC_ENFORCE` configuration flag. When
disabled, all authenticated requests pass. When enabled, all requests are evaluated
against bindings.

Production deployments SHALL enable enforcement and disable defaults:
- `RBAC_ENFORCE=true`
- `RBAC_DEFAULT_ROLES=` (empty)

No database migration or CLI command is needed to bootstrap users -- built-in Role
records are seeded via migration; RoleBindings are created dynamically from JWT claims
on every authenticated request. The first privileged users are provisioned by assigning
`gateway:creator` or `platform:admin` in Keycloak before enforcement is enabled.

### Requirement: Database Migration

A database migration SHALL seed the `platform:admin` role record with:

- `name: "platform:admin"`
- `display_name: "Platform Administrator"`
- `description: "Platform-wide view and delete access for all gateways"`
- `built_in: true`

A separate migration SHALL seed the `managed-cluster-registrar` role record with:

- `name: "managed-cluster-registrar"`
- `display_name: "Managed Cluster Registrar"`
- `description: "Allows a spoke control-plane service account to self-register via POST /managed_clusters/registration"`
- `built_in: true`

`managed-cluster-registrar` is seeded for role discoverability (`GET /roles`) only. It is
NOT added to `JWTSyncedRoles` and has no DB RoleBinding lifecycle; enforcement is
JWT-direct in `isAuthorized`.

These migrations SHALL run alongside the existing migrations that seed `gateway:creator`,
`gateway:owner`, and `gateway:viewer` roles.

RoleBindings from JWT claims are synced regardless of whether enforcement is enabled,
ensuring bindings exist before enforcement is turned on.

#### Operator Note: Production Deployment Requirements

The OpenShift overlay (`deploy/openshift/kustomization.yaml`) SHALL ship with both:
- `RBAC_ENFORCE=true`
- `RBAC_DEFAULT_ROLES=` (empty)

Applying this to a cluster requires that Keycloak realm roles are configured first:

1. Define `gateway:creator`, `platform:admin`, and `managed-cluster-registrar` realm roles in Keycloak.
2. Assign `gateway:creator` to users and service accounts that need to create gateways.
3. Assign `platform:admin` to operators who need global view/delete access.
4. Assign `managed-cluster-registrar` to each spoke control-plane service account.
5. Ensure Keycloak emits these roles in the `realm_access.roles` claim.

With `RBAC_DEFAULT_ROLES=` set, no authenticated principal receives any role
automatically. Every permission flows exclusively from Keycloak. Callers without an
appropriate role receive 403 on mutation endpoints and 404 on singleton GETs.

### Requirement: Managed Cluster Self-Registration RBAC

The `POST /api/hypershell/v1/managed_clusters/registration` endpoint SHALL require the
`managed-cluster-registrar` role in the caller's JWT `realm_access.roles` claim.

**Enforcement mechanism:** `managed-cluster-registrar` is a JWT-direct role -- it is
checked live from the JWT claim in `isAuthorized`, NOT via the `JWTSyncedRoles` sync
lifecycle and NOT via a DB-backed RoleBinding lookup. The `isAuthorized` function SHALL
have a dedicated case:

```
if resource == "managed_clusters" && resourceID == "registration" && method == POST:
    return hasJWTRole(jwtRoles, "managed-cluster-registrar")
```

This case takes precedence over the `hasGatewayCreator` fallback that would otherwise
allow any user to call the endpoint.

`managed-cluster-registrar` is NOT in `JWTSyncedRoles`. No DB RoleBinding is created for
it. The role is seeded as a built-in role record (for discoverability via `GET /roles`)
but has no DB binding lifecycle.

Assigning `managed-cluster-registrar` to a spoke service account is a Keycloak admin
function performed out-of-band before the spoke is deployed. Keycloak is the trusted
source of truth; the API server does not re-verify role assignment beyond reading the
JWT claim.

**Isolation guarantee:** In production (`RBAC_DEFAULT_ROLES=`, `RBAC_ENFORCE=true`),
a spoke service account holding only `managed-cluster-registrar` has no gateway
permissions. No role is auto-assigned; the spoke's access is exactly what Keycloak
grants. This is the required production configuration.

#### Scenario: Spoke with role can self-register

- GIVEN a spoke service account with `managed-cluster-registrar` assigned in Keycloak
- WHEN it calls `POST /managed_clusters/registration`
- THEN the `isAuthorized` JWT-direct check passes
- AND the request proceeds to the handler
- AND a `ManagedCluster` record is created (or the existing one is returned)

#### Scenario: Spoke without role is rejected

- GIVEN a spoke service account without `managed-cluster-registrar` in Keycloak
- WHEN it calls `POST /managed_clusters/registration`
- THEN the RBAC middleware returns 403 Forbidden
- AND no `ManagedCluster` record is created or modified

#### Scenario: managed-cluster-registrar grants no gateway access

- GIVEN production deployment with `RBAC_DEFAULT_ROLES=` and `RBAC_ENFORCE=true`
- AND a spoke service account has only `managed-cluster-registrar` assigned in Keycloak
- WHEN it calls `GET /api/hypershell/v1/gateways`
- THEN the response is 200 with an empty items array
- AND the spoke cannot create, modify, or delete any gateway

### Requirement: Integration Test Coverage

Integration tests SHALL exercise RBAC enforcement with the new five-role model, including
`managed-cluster-registrar` grant and deny scenarios, and the JWT-direct enforcement path.

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
| `managed-cluster-registrar` is separate from gateway roles | A spoke service account only needs fleet membership rights, not gateway creation rights. Keeping the roles separate limits blast radius if a spoke credential is compromised. Operators who want strict isolation must also set `RBAC_DEFAULT_ROLES=` to disable the universal `gateway:creator` default. |
| `managed-cluster-registrar` is JWT-direct, not DB-synced | The role is for specific service accounts, not users in general. There is no reason to maintain a DB binding for it. Checking from the live JWT claim in `isAuthorized` is sufficient, consistent with `HypershellAdminRole`, and avoids `JWTSyncedRoles` entanglement. |
| Role assigned by admin, not auto-granted | Provides a human control point for fleet membership. A new spoke cannot join the fleet without an explicit Keycloak admin action. |
| Gateway owners can grant co-owners | No hierarchy restriction. Team leads assign `gateway:creator` to team members or invite them as owners/viewers per gateway. Simple mental model. |
| Auto-assign `gateway:owner` on creation | Creator automatically owns what they create. No separate grant step needed. |
| `gateway:creator` assigned exclusively via Keycloak in production | Production deployments set `RBAC_DEFAULT_ROLES=` so only Keycloak-assigned roles apply. `RBAC_DEFAULT_ROLES=gateway:creator` exists as a local-dev escape hatch only. The role cannot be self-assigned via the API. |
| Per-gateway bindings stored in DB | Gateway-scoped access requires per-resource granularity that JWT claims cannot provide (you'd need dynamic claim values per gateway ID). |
| No resource grouping as a security boundary | The Sector/Fleet grouping was removed. RBAC operates at platform level (creator) and gateway level (owner/viewer); there is no fleet-scoped isolation. |
| 404 on unauthorized singleton GETs | Returning 403 confirms the resource exists. 404 prevents ID enumeration. |
| OpenShell role bridge | `gateway:owner` maps to `openshell-admin`, `gateway:viewer` maps to `openshell-user`. Ensures consistent access via CLI. |
| OpenShellGatewayServiceAccount role limit | A gateway binding limits the selected OpenShell role. Owners can select `openshell-user` or `openshell-admin`. Viewers can select only `openshell-user`. Each OpenShellGatewayServiceAccount remains bound to its creator. A binding change can downgrade or revoke it. |
