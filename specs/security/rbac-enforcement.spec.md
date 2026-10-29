# RBAC Enforcement

**Date:** 2026-08-12
**Status:** Draft
**Related:** `platform/data-model.spec.md` (domain model), `platform/openshell-gateway-oidc.spec.md` (gateway OIDC), `standards/security/security.spec.md` (security standards)
**OpenShell gateway service accounts:** `platform/openshell-gateway-service-accounts.spec.md` (delegated service-account identities)

---

## Purpose

The HyperShell API server SHALL enforce authorization on all API endpoints (HTTP and
gRPC) using a five-role model backed by Keycloak as the source of truth for platform-wide
roles and a PostgreSQL-backed RoleBinding model for per-gateway grants, plus the
create-time `gateway:dedicated-route-creator` entitlement (SEC-RBAC-20).

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
Keycloak, or by being granted a per-gateway binding (`gateway:owner`, `gateway:admin`,
`gateway:viewer`). Owners can grant any of the three; admins can grant only
`gateway:admin` and `gateway:viewer`. See `platform/gateway-access-management.spec.md`.

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

Built-in roles are seeded at migration time. Six roles are DB-backed (the per-gateway
tier plus the JWT-synced platform-wide roles, `gateway:dedicated-route-creator` among
them); one additional role (`managed-cluster-registrar`) is seeded for discoverability
but enforced via JWT-direct check (not a DB-backed RoleBinding).

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
no resource FK. For `gateway:owner`, `gateway:admin`, and `gateway:viewer`, scope is
`gateway` with `gateway_id` identifying the bound gateway.

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
| `platform:admin` | global | Keycloak JWT | Platform-wide administration; can view and delete any gateway, and manage any gateway's access (grant/change/revoke any tier, including owner) via the access facade -- without gaining gateway login |
| `gateway:creator` | global | Keycloak JWT | Can create gateways; auto-becomes `gateway:owner` on creation |
| `gateway:owner` | per gateway | DB (app logic) | Gateway owner; one or more; grantable and removable (but a gateway always keeps at least one owner). Full CRUD including delete; can grant/change/revoke any role including `gateway:owner`. The creator is the first owner |
| `gateway:admin` | per gateway | DB (app logic) | Granted administrator; zero or more; removable. Read/update the gateway (no delete); can grant/change/revoke `gateway:admin` and `gateway:viewer` only (not `gateway:owner`) |
| `gateway:viewer` | per gateway | DB (app logic) | Read-only access to one gateway, including its sandbox Events |
| `managed-cluster-registrar` | global | Keycloak JWT (direct, no DB binding) | Allows a control-plane service account (every control plane, the hub's co-located one included) to call `POST /managed_clusters/registration`; checked live from JWT claim, not via `JWTSyncedRoles` or DB RoleBinding |
| `gateway:dedicated-route-creator` | global | Keycloak JWT | Entitlement to set `dedicated_route = true` on a gateway create (the dedicated per-gateway route and wildcard certificate, PLAT-DATA-26 / OSG-ROUT-12); grants nothing else by itself. See "Requirement: Gateway Dedicated Route Creator Role" (SEC-RBAC-20) |
| `ci-federation:admin` | global | Keycloak JWT | Authorizes the `ci_providers` extension kind (register/read/update/delete a CI OIDC forge, PLAT-DATA-24) and nothing else. The gitops CI-federation seeder holds only this role instead of `platform:admin`. It confers no gateway, platform, or other ext-kind authority. See "Requirement: CI Federation Admin Role" (SEC-RBAC-21) |

### Permission Matrix

| Role | Gateways | Gateway CRUD | RBAC Grants | OpenShell Mapping | OpenShellGatewayServiceAccounts | ManagedCluster Registration |
|------|----------|-------------|-------------|-------------------|-----------------|-----------------|
| `platform:admin` | view all, delete any | view all + delete any | grant/change/revoke owner/admin/viewer on any gateway (via the access facade, GTW-ACC-04/05/06) | None without a gateway binding | None without a gateway binding | View all records; create, update, or delete any record |
| `gateway:creator` | create + own gateways | full (as owner) | grant owner/admin/viewer on own gateways | `openshell-admin` + `openshell-user` on own gateways | Through the resulting owner binding | View all records; no writes |
| `gateway:owner` | full (one gateway) | full incl. delete | grant owner/admin/viewer on that gateway | `openshell-admin` + `openshell-user` on that gateway | Select `openshell-user` or `openshell-admin`. Manage all OpenShellGatewayServiceAccounts on the gateway. | -- |
| `gateway:admin` | full except delete (one gateway) | read + update (no delete) | grant admin/viewer on that gateway | `openshell-admin` + `openshell-user` on that gateway | Select `openshell-user` or `openshell-admin`. Manage all OpenShellGatewayServiceAccounts on the gateway. | -- |
| `gateway:viewer` | read (one gateway) | read only | -- | `openshell-user` on that gateway | Select only `openshell-user`. Manage only their own OpenShellGatewayServiceAccounts. | -- |
| `managed-cluster-registrar` | none (unless also granted `gateway:creator` via defaults) | none | none | none | none | `POST /registration` (register + heartbeat loop) -- enforced by JWT-direct check in `isAuthorized`, not a DB binding. Once registered, the caller may delete its own record (self-deregistration). |
| `gateway:dedicated-route-creator` | none by itself (marker for `dedicated_route` on create) | none (does not create or own gateways without `gateway:creator`) | none | none | none | none |
| `ci-federation:admin` | none | none | none | none | none | none (authorizes only the `ci_providers` ext kind; see SEC-RBAC-21) |

`GET /api/hypershell/v1/gateways/{gateway_id}/sandbox_events` is a gateway-bound diagnostic read. It requires a `gateway:viewer`, `gateway:admin`, or `gateway:owner` binding on the requested Gateway. A `platform:admin` without such a binding SHALL receive `404`, so the endpoint does not disclose Gateway or sandbox-event existence. See `platform/gateway-sandbox-events.spec.md` (OSG-SEVT-05).

### OpenShell Role Bridge

When a user accesses a gateway directly via the `openshell` CLI, the control plane
projects HyperShell roles onto per-gateway Keycloak client roles. A gateway
administrator (owner or admin) receives both client roles; a standard user receives
only `openshell-user`:

| HyperShell Role | OpenShell (Keycloak client) roles |
|-----------------|-----------------------------------|
| `gateway:owner` | `openshell-admin`, `openshell-user` |
| `gateway:admin` | `openshell-admin`, `openshell-user` |
| `gateway:viewer` | `openshell-user` |

The control plane reconciles each user's client roles to the union implied by their
surviving bindings on every RoleBinding event; see
`platform/gateway-access-management.spec.md` and `platform/openshell-gateway-keycloak.spec.md`.

The `platform:admin` role provides visibility and lifecycle management through the
HyperShell web console but does NOT grant OpenShell CLI access to gateways. Platform
administrators who need to use the `openshell` CLI for a specific gateway must be
granted `gateway:admin` or `gateway:viewer` on that gateway.

---

## Requirements

### Requirement: SEC-RBAC-01 -- Keycloak as Authority for Platform Roles

Keycloak is the source of truth for the `gateway:creator`, `gateway:dedicated-route-creator`, `ci-federation:admin`, and `platform:admin` roles. A
Keycloak administrator assigns these roles to users and service accounts via Keycloak's
admin console or API.

The API server middleware SHALL extract roles from the JWT `realm_access.roles` claim
(or an equivalent configurable claim path) and lazily create or update the corresponding
RoleBinding records in the database on each authenticated request.

This ensures that Keycloak role changes take effect on the next API request without
requiring a separate sync process.

#### Scenario: SEC-RBAC-01.01 -- Keycloak admin assigns gateway:creator

- GIVEN a Keycloak admin assigns the `gateway:creator` realm role to user A
- WHEN user A makes their first API request with a JWT containing `gateway:creator`
- THEN the middleware creates a User record and a `gateway:creator` RoleBinding
- AND user A can create gateways

#### Scenario: SEC-RBAC-01.02 -- Keycloak admin revokes gateway:creator

- GIVEN production deployment with `RBAC_DEFAULT_ROLES=`
- AND user A previously had `gateway:creator` assigned in Keycloak
- WHEN the Keycloak admin removes the role
- THEN user A's next API request carries a JWT without `gateway:creator`
- AND the middleware removes the corresponding RoleBinding
- AND user A can no longer create new gateways
- AND existing `gateway:owner` bindings on previously-created gateways are unaffected

### Requirement: SEC-RBAC-02 -- Service Account Support

Service accounts (e.g., the control plane) are Keycloak clients using the
`client_credentials` grant. They receive the same JWT structure and role claims as
human users. The middleware provisions them identically -- a User record is created
for the service account's client ID, and RoleBindings are created from their JWT roles.

The control plane service account SHALL be assigned `gateway:creator` in Keycloak,
enabling it to create, watch, and manage all gateways it creates.

#### Scenario: SEC-RBAC-02.01 -- Control plane authenticates and operates

- GIVEN the control plane service account has `gateway:creator` in Keycloak
- AND its client ID and secret are stored in a Kubernetes Secret
- WHEN the control plane connects via gRPC with a `client_credentials` JWT
- THEN the middleware provisions a User for the service account
- AND creates a `gateway:creator` RoleBinding
- AND the control plane can create and manage gateways

### Requirement: SEC-RBAC-03 -- User Auto-Provisioning

The middleware SHALL automatically create a User record when a JWT-authenticated caller
is seen for the first time. The User record SHALL be populated from standard OIDC claims
(`preferred_username`, `email`, `given_name`, `family_name`).

Auto-provisioning SHALL use insert-or-update semantics keyed on `username` to handle
concurrent first-time requests without a race: when no row exists yet, two concurrent
first requests for the same username SHALL both resolve to the single row that gets
created, instead of one failing on a unique-constraint error.

When a User row already exists, auto-provisioning SHALL write to a stored profile field
(`email`, `name`) only when the JWT carries a claim for that field and the claim's value
differs from the stored one. It SHALL NOT write to the row, and SHALL NOT take a row
lock on it, on a request whose claims already match what is stored. A JWT that omits a
profile claim (for example, a service-account token with no `email` or `name`) SHALL
leave the corresponding stored field unchanged; auto-provisioning SHALL NEVER overwrite
a stored non-empty profile field with an empty or absent claim.

This conditional-write behavior keeps a caller's own row from being locked on every
request the caller makes -- including the control plane's gRPC write-backs -- so a slow
downstream call inside one request cannot stall that caller's other concurrent requests.

When a request DOES need to write a changed profile field, it SHALL serialize that write
through one shared advisory lock rather than relying on the row lock's acquisition order.
This prevents a lock-ordering deadlock when two callers concurrently provision each
other's row, such as two users granting each other gateway access at the same time (see
`platform/gateway-access-management.spec.md` GTW-ACC-04): with a single shared lock in play,
the two requests can only ever queue behind it, never each hold a different row lock
while waiting on the other's -- so the guarantee holds even when both callers' directory
profiles changed at the same moment.

The middleware SHALL also sync platform-wide RoleBindings (the JWT-synced platform-wide
roles: `gateway:creator`, `gateway:dedicated-route-creator`, and `platform:admin`)
from the JWT on every request, creating or removing bindings as
needed to match the JWT claims.

#### Scenario: SEC-RBAC-03.01 -- First-time user auto-provisioned

- GIVEN a user authenticates via SSO for the first time
- WHEN any authenticated API request is processed
- THEN a User record is created from the JWT claims
- AND RoleBindings are created to match the JWT's role claims
- AND the request proceeds to authorization evaluation

#### Scenario: SEC-RBAC-03.02 -- Concurrent first-time requests resolve to one row

- GIVEN no User record exists for username `newuser`
- WHEN two authenticated requests for `newuser` are processed at the same time
- THEN exactly one User record is created for `newuser`
- AND both requests resolve to that record's `user_id`

#### Scenario: SEC-RBAC-03.03 -- Unchanged profile issues no write

- GIVEN an existing User record whose `email` and `name` already match the caller's JWT claims
- WHEN an authenticated request from that caller is processed
- THEN auto-provisioning SHALL NOT update the User record
- AND SHALL NOT take a row lock on it

#### Scenario: SEC-RBAC-03.04 -- Missing claim does not clear a stored profile field

- GIVEN an existing User record with `email` set from a prior request
- WHEN an authenticated request carries a JWT with no `email` claim
- THEN the stored `email` SHALL remain unchanged

#### Scenario: SEC-RBAC-03.05 -- Mutual access grant does not deadlock

- GIVEN user A and user B each already have a User record
- AND each caller's JWT carries a directory profile claim that differs from what is stored, so each request's auto-provisioning self-upsert and Grant's target pre-provisioning both write
- WHEN user A grants user B gateway access and user B grants user A gateway access at the same time
- THEN both grants SHALL complete without a database deadlock error

### Requirement: SEC-RBAC-04 -- Gateway Creation Bootstrap

Any authenticated user with the `gateway:creator` role SHALL be able to create gateways.
On successful gateway creation, the system SHALL automatically create a `gateway:owner`
RoleBinding for the authenticated user, scoped to the new gateway. This user is the
creator and the first owner. Owners MAY subsequently grant `gateway:owner`,
`gateway:admin`, or `gateway:viewer` to others, so a gateway may have multiple owners;
a gateway SHALL always retain at least one owner (last-owner protection -- see
`platform/gateway-access-management.spec.md` GTW-ACC-07). The creator holds no capability
beyond any other owner and is tracked only for display/audit.

This binding is created in the same database transaction as the gateway.

#### Scenario: SEC-RBAC-04.01 -- Creator creates a gateway and becomes owner

- GIVEN user A has `gateway:creator` (from Keycloak)
- WHEN user A calls `POST /api/hypershell/v1/gateways`
- THEN the gateway is created
- AND a `gateway:owner` RoleBinding is created for user A on the new gateway
- AND user A can immediately manage the gateway

#### Scenario: SEC-RBAC-04.02 -- User without creator role cannot create gateways

- GIVEN production deployment with `RBAC_DEFAULT_ROLES=`
- AND user A has only `gateway:viewer` on some gateway
- WHEN user A calls `POST /api/hypershell/v1/gateways`
- THEN the request returns 403 Forbidden

### Requirement: SEC-RBAC-05 -- Per-Gateway Authorization

The authorization middleware SHALL evaluate permissions against the binding's gateway
scope. A binding with `scope=gateway` and `gateway_id=gw-1` SHALL only authorize
access to gateway `gw-1`.

`gateway:creator` grants the ability to create new gateways and acts as `gateway:owner`
on all gateways the user owns (has a `gateway:owner` binding for).

#### Scenario: SEC-RBAC-05.01 -- Gateway owner can manage their gateway

- GIVEN user A has `gateway:owner` on gw-1
- WHEN user A calls `PATCH /api/hypershell/v1/gateways/gw-1`
- THEN the request is authorized

#### Scenario: SEC-RBAC-05.02 -- Gateway viewer cannot modify

- GIVEN user A has `gateway:viewer` on gw-1
- WHEN user A calls `PATCH /api/hypershell/v1/gateways/gw-1`
- THEN the response is 403 Forbidden

#### Scenario: SEC-RBAC-05.03 -- No binding returns 404

- GIVEN user A has no binding covering gw-2
- WHEN user A calls `GET /api/hypershell/v1/gateways/gw-2`
- THEN the response is 404 (existence not disclosed)

### Requirement: SEC-RBAC-06 -- RoleBinding Grants

Grants follow the hierarchy **owner > admin > viewer**. Gateway owners can grant, change,
or revoke any role -- `gateway:owner`, `gateway:admin`, or `gateway:viewer` -- on gateways
they own. Gateway admins can grant, change, or revoke only `gateway:admin` and
`gateway:viewer`; they SHALL NOT assign, change, or revoke the owner tier. A gateway SHALL
always retain at least one owner (last-owner protection, GTW-ACC-07). The console performs
these grants through the gateway access facade (`GTW-ACC-04`/`GTW-ACC-05`/`GTW-ACC-06`), which writes
the same RoleBinding records. The low-level generic `/role_bindings` resource writes the
same records but under the default-deny write policy of SEC-RBAC-23, which requires
`gateway:owner` on the target gateway and denies every role with no explicit rule.

#### Scenario: SEC-RBAC-06.01 -- Owner invites a viewer

- GIVEN user A has `gateway:owner` on gw-1
- WHEN user A grants `role=gateway:viewer`, `gateway_id=gw-1`, `user_id=B`
- THEN the binding is created
- AND user B gains read-only access to gw-1

#### Scenario: SEC-RBAC-06.02 -- Owner assigns a co-owner

- GIVEN user A has `gateway:owner` on gw-1
- WHEN user A grants `role=gateway:owner`, `gateway_id=gw-1`, `user_id=B`
- THEN the binding is created
- AND user B gains full owner access to gw-1 (including deleting the gateway and assigning owners)

#### Scenario: SEC-RBAC-06.03 -- Admin can grant admin and viewer

- GIVEN user B has `gateway:admin` on gw-1
- WHEN user B grants `role=gateway:viewer` (or `gateway:admin`), `gateway_id=gw-1`, `user_id=D`
- THEN the binding is created

#### Scenario: SEC-RBAC-06.04 -- Admin cannot assign an owner

- GIVEN user B has `gateway:admin` on gw-1 (not an owner)
- WHEN user B attempts to grant `role=gateway:owner` on gw-1
- THEN the request returns 403 Forbidden

#### Scenario: SEC-RBAC-06.05 -- Viewer cannot grant

- GIVEN user A has only `gateway:viewer` on gw-1
- WHEN user A attempts to grant any role on gw-1
- THEN the request returns 403 Forbidden

#### Scenario: SEC-RBAC-06.06 -- Cannot grant on a gateway you don't administer

- GIVEN user A administers gw-1 only
- WHEN user A attempts to grant a binding with `gateway_id=gw-2`
- THEN the request returns 403 Forbidden

### Requirement: SEC-RBAC-07 -- Platform Admin Global Access

Users with the `platform:admin` role SHALL have global view, delete, and access-management
permissions across all gateways in the platform, regardless of per-gateway RoleBindings. The
sandbox-events diagnostic route is the explicit exception: it requires a per-gateway binding
and returns `404` to a `platform:admin` who lacks one (OSG-SEVT-05).
Access management (grant/change/revoke any tier, including owner, and directory search)
is owner-equivalent and exercised through the access facade; it creates no gateway-scoped
binding for the platform admin, so it confers no gateway login
(`platform/gateway-access-management.spec.md` GTW-ACC-08). This role is assigned via Keycloak
and synced to the database identically to `gateway:creator`.

Platform administrators SHALL be able to:

- List all gateways across the platform (GET `/api/hypershell/v1/gateways` returns all)
- View any specific gateway (GET `/api/hypershell/v1/gateways/{id}` succeeds for any ID)
- Delete any gateway (DELETE `/api/hypershell/v1/gateways/{id}` succeeds for any ID)
- Manage any gateway's access through the access facade (`/api/hypershell/v1/gateways/{id}/access`): list, grant, change, and revoke any tier including `gateway:owner`, and search the directory -- owner-equivalent, without holding a per-gateway binding, and subject to last-owner protection (`platform/gateway-access-management.spec.md` GTW-ACC-07/GTW-ACC-08)

Platform administrators SHALL NOT be able to:

- Modify gateway configuration (PATCH/PUT operations require `gateway:owner` or `gateway:admin`)
- Create or delete `RoleBinding` records through the low-level generic `/role_bindings` resource (that resource enforces the default-deny write policy of SEC-RBAC-23, which requires `gateway:owner` on the target gateway and grants `platform:admin` no bypass; platform administrators manage access through the access facade instead)
- Create gateways (requires `gateway:creator` role)
- Read sandbox Events through `GET /api/hypershell/v1/gateways/{gateway_id}/sandbox_events` without a per-gateway binding; the response SHALL be `404` (OSG-SEVT-05)
- Obtain gateway login from managing access: performing access management creates no gateway-scoped binding for the platform administrator, so they receive no `openshell` CLI access unless separately granted a per-gateway binding (see OpenShell Role Bridge above)

The `platform:admin` role is orthogonal to `gateway:creator`, `gateway:owner`, and
`gateway:viewer`. A user may hold multiple roles (e.g., `platform:admin` + `gateway:creator`).

The `platform:admin` role covers gateway **view**, **delete**, and **access management** (the access facade; `platform/gateway-access-management.spec.md`). It does NOT grant permissions to:

- Modify gateway configuration (PATCH), or create gateways (requires `gateway:creator`)
- View or modify ManagedClusters
- Create or delete `RoleBinding` records through the generic `/role_bindings` resource (that resource requires `gateway:owner` on the target gateway under the SEC-RBAC-23 write policy; access is managed through the access facade instead)
- Access platform-level configuration or system administration functions

Future iterations may expand platform:admin permissions to include these resources.

#### Scenario: SEC-RBAC-07.01 -- Platform admin views all gateways

- GIVEN user A has `platform:admin` (from Keycloak)
- AND there are 50 gateways in the platform owned by various users
- WHEN user A calls `GET /api/hypershell/v1/gateways`
- THEN the response includes all 50 gateways
- AND the response is paginated
- AND includes metadata for total count and pagination

#### Scenario: SEC-RBAC-07.02 -- Platform admin views a specific gateway

- GIVEN user A has `platform:admin`
- AND user A has no `gateway:owner` or `gateway:viewer` binding for gw-1
- WHEN user A calls `GET /api/hypershell/v1/gateways/gw-1`
- THEN the response is 200 OK with the gateway details

#### Scenario: SEC-RBAC-07.03 -- Platform admin deletes any gateway

- GIVEN user A has `platform:admin`
- AND user B owns gw-1 (has `gateway:owner` binding)
- AND user A has no binding for gw-1
- WHEN user A calls `DELETE /api/hypershell/v1/gateways/gw-1`
- THEN the gateway is deleted
- AND the response is 204 No Content

#### Scenario: SEC-RBAC-07.04 -- Platform admin manages gateway access without a binding

- GIVEN user A has `platform:admin`
- AND user A has no binding for gw-1 (owned by user B)
- WHEN user A grants user C the `owner` role on gw-1 via `POST /api/hypershell/v1/gateways/gw-1/access`
- THEN the grant SHALL succeed
- AND no gateway-scoped binding SHALL be created for user A (user A gains no `openshell` login on gw-1)

#### Scenario: SEC-RBAC-07.05 -- Platform admin cannot modify gateways without ownership

- GIVEN user A has `platform:admin` only
- AND user A has no `gateway:owner` binding for gw-1
- WHEN user A calls `PATCH /api/hypershell/v1/gateways/gw-1`
- THEN the response is 403 Forbidden

#### Scenario: SEC-RBAC-07.06 -- Platform admin cannot create gateways without creator role

- GIVEN production deployment with `RBAC_DEFAULT_ROLES=`
- AND user A has `platform:admin` only (no `gateway:creator` in Keycloak)
- WHEN user A calls `POST /api/hypershell/v1/gateways`
- THEN the response is 403 Forbidden

#### Scenario: SEC-RBAC-07.07 -- Platform admin cannot use the low-level role_bindings resource

- GIVEN user A has `platform:admin` only
- AND user A has no `gateway:owner` binding for gw-1
- WHEN user A calls `POST /api/hypershell/v1/role_bindings` with `gateway_id=gw-1`
- THEN the response is 403 Forbidden
- (platform administrators manage access through the access facade `/gateways/{id}/access`, not this generic resource)

#### Scenario: SEC-RBAC-07.08 -- User with platform:admin and gateway:creator can create and view all

- GIVEN user A has both `platform:admin` and `gateway:creator`
- WHEN user A calls `POST /api/hypershell/v1/gateways`
- THEN the gateway is created
- AND user A becomes `gateway:owner` of the new gateway
- AND user A can still view all other gateways via `platform:admin`

### Requirement: SEC-RBAC-08 -- Platform Admin UI Experience

The web UI SHALL support `platform:admin` users as defined in `web-console/architecture.spec.md` requirement WEB-ARCH-02 and WEB-UIEX-06.

Platform administrators SHALL see all gateways in the gateway list, which SHALL use API-backed pagination and search as specified in WEB-DATA-01.

Each gateway row SHALL display a delete action for platform administrators. Delete actions SHALL require confirmation via a modal dialog before calling `DELETE /api/hypershell/v1/gateways/{id}`.

### Requirement: SEC-RBAC-09 -- gRPC Authorization

gRPC handlers SHALL enforce the same authorization rules as HTTP handlers. The gRPC
authorization interceptor SHALL extract the caller identity from the request metadata
and decide every `hypershell.v1` method explicitly; a method it does not know SHALL be
denied. A role binding SHALL NOT authorize a method by role alone: a global
`gateway:creator` binding authorizes `CreateGateway`, not every method.

The middleware SHALL provision users and sync JWT roles on gRPC requests identically
to HTTP requests. The framework's gRPC auth interceptor keeps only the username in the
context, so the realm roles SHALL be read from the same authorization metadata value it
verified; otherwise role sync would see no roles on gRPC and revoke a caller's
`platform:admin` binding on every gRPC call.

For a caller that is not a control-plane identity, the rules mirror HTTP:

- `GetGateway`, `UpdateGateway`, `DeleteGateway`: the per-gateway rules of
  "Per-Gateway Authorization" and "Platform Admin Global Access" for the request's
  gateway id. `GetGateway` without access returns `NOT_FOUND`; a denied write returns
  `PERMISSION_DENIED`.
- `CreateGateway`: `gateway:creator`. A request body that sets `dedicated_route`
  additionally requires `gateway:dedicated-route-creator` or `platform:admin`
  (SEC-RBAC-20); because that check is body-dependent it is enforced in the
  `CreateGateway` handler, not in this interceptor.
- `ListGateways` and `WatchGateways`: `platform:admin` only. The HTTP list is filtered
  to the caller's gateways; the gRPC list and watch are not, so no other role may use
  them.
- `AdjustActiveSandboxCount`, `SetActiveSandboxCount`, `SetGatewayVersion`, `ReportSandboxEvents`: denied
  whatever the caller's bindings. Only a control-plane identity writes these fields.
- `ManagedClusterService`: reads for `gateway:creator` or `platform:admin`; writes for
  `platform:admin` (`platform/managed-cluster-registration.spec.md`, "Record Writes and
  Deregistration").
- `RoleBindingService`: denied. Its list and watch are not filtered by user or gateway;
  users manage bindings over HTTP.
- Platform-wide ext kinds (`RepositoryService`, `SandboxTemplateService`, `ProviderSpecService`,
  `ProviderBindingService`, `InferenceRouteService`, `SecretSourceService`): list, watch,
  and get for `platform:admin`; all writes (create/update/delete) denied for user callers.
  These kinds are platform-wide, so a user caller never writes them over gRPC. For the
  import-write kinds among them (all except `RepositoryService`), declarative-import writes
  are authorized only for the control-plane identity (see below); `RepositoryService` is NOT
  on the import-write path because the import never upserts `Repository` resources.
- `CIProviderService` and `CITrustService`: authored over HTTP (`hsctl` and the SDKs),
  like `AgentWorkspaceService`. A registered control-plane identity MAY READ them
  (list/watch/get) to drive `CIProvider`/`CITrust` reconciliation; `platform:admin`
  MAY also list, watch, and get them. All writes (create/update/delete) over gRPC
  SHALL be denied for every caller: `CIProvider` is authored by a `ci-federation:admin`
  over HTTP (SEC-RBAC-21) and `CITrust` by a gateway owner/admin over HTTP
  (SEC-RBAC-22), and neither is on the declarative-import write path.
- Under `RBAC_ENFORCE=true` an unauthenticated call reaches no `hypershell.v1` method.

Control-plane identities are exempt from role bindings but not from scoping. A caller
whose JWT `sub` is the `oidc_subject` of a registered ManagedCluster is authorized per
`platform/managed-cluster-registration.spec.md`, "Control-Plane Identity": its own
cluster's gateways, platform-wide reads, and deleting its own record. A username on the `RBAC_SERVICE_ACCOUNTS` allowlist that is not
registered keeps the bootstrap exemption, except for ManagedCluster record writes; an
allowlisted account that is registered is scoped as a control plane.

**Declarative-import writes (control-plane identity only).** The control-plane
declarative `.hypershell` import reconciler (see
[`platform/control-plane.spec.md`](../platform/control-plane.spec.md), "Declarative
`.hypershell` Import Reconciliation") is the single bounded path by which the control
plane writes HyperShell API desired state. The gRPC `Create`, `Update`, and `Delete`
methods for the import-write kinds (`SandboxTemplate`, `ProviderSpec`,
`ProviderBinding`, `InferenceRoute`, `SecretSource`) SHALL be authorized for the
control-plane identity and SHALL remain denied for every other caller. `Repository`
is NOT an import-write kind: the import reconciler reconciles the `Repository` Kind but
SHALL NEVER upsert `Repository` resources discovered in an import (it is excluded from
the canonical importable set in
[`platform/data-model.spec.md`](../platform/data-model.spec.md) to prevent recursive
import), so the import write path SHALL NOT create, update, or delete `Repository`
resources. These writes
are `Repository`-scoped through the `repository_id` ownership mark, not an open
bootstrap bypass:

- On create the server SHALL set `repository_id` from the `Repository` the reconciler
  is reconciling and SHALL ignore any client-supplied `repository_id`, so a caller
  cannot forge ownership of an imported resource.
- A create, update, or delete SHALL act only on resources owned by the reconciled
  `Repository`; a write that would affect a resource owned by a different
  `Repository`, or a resource created directly through the REST API or `hsctl apply`
  (no ownership mark), SHALL be refused.
- The bootstrap-service-account allowlist exemption remains a distinct, pre-existing
  mechanism for fully-trusted internal identities; the import write authority derives
  from the control-plane identity and `Repository` ownership, not from that allowlist.
  A bootstrap service account that is not a registered control plane SHALL NOT be
  treated as an import reconciler.

The remaining importable Kinds, `Gateway` and `AgentRuntime`, are cluster-assigned and
their gRPC `Create`, `Update`, and `Delete` are also authorized for the control-plane
identity on the same `Repository`-scoped import terms: a write marked with the
reconciled `Repository` id in trusted metadata is authorized as an import write, the
server SHALL set `repository_id` from that metadata (ignoring any client value), and a
write to a `Gateway` or `AgentRuntime` owned by a different `Repository`, or unowned,
SHALL be refused. A `Gateway` or `AgentRuntime` write that carries NO import metadata
stays on its existing non-import path (the control plane's cluster-scoped health and
status write-backs), unchanged.

Method authorization is NOT field authorization. For a `Gateway`, an import-marked
write is authorized to CALL `CreateGateway`/`UpdateGateway` on the terms above, but is
STILL subject to the field-level write policy of "Requirement: Gateway Field Write
Policy" below: the import caller kind is treated as non-privileged and may write only
`name` + placement, exactly like a direct user. Only a control-plane write WITHOUT
import metadata may write the platform-managed and observed Gateway fields.

**Declarative-import reads (control-plane identity only).** `Gateway` and
`AgentRuntime` list reads are normally held to the caller's own `cluster_id` by the
watch-stream caller binding. Because the single import reconciler must enumerate and
prune a `Repository`'s gateways and agent runtimes across clusters, a `ListGateways`
or `ListAgentRuntimes` read SHALL be exempted from the own-cluster requirement, and
served across clusters, ONLY when ALL hold: the caller is the registered
control-plane identity; the call carries the reconciled-`Repository` id in trusted
metadata; the request sets the `repository_id` read filter equal to that id; and the
named `Repository` exists. The server then filters the result to that `Repository`
server-side. Any other read keeps the own-cluster binding, so this exemption does not
widen a tenant's or a spoke's view beyond one named `Repository` it is reconciling.

The hub's gRPC API is reachable from outside the cluster (`platform/hub-grpc-tls.spec.md`),
so these rules are the only boundary between tenants, and between clusters, on that API.

#### Scenario: SEC-RBAC-09.01 -- Platform admin watches gateways via gRPC

- GIVEN user A has `platform:admin` (from Keycloak)
- AND the control plane watches gateway events via gRPC
- WHEN a gRPC client with user A's credentials calls `WatchGateways`
- THEN the stream includes events for all gateways in the platform
- AND the authorization interceptor evaluates `platform:admin` identically to HTTP handlers

#### Scenario: SEC-RBAC-09.02 -- Platform admin cannot modify via gRPC without ownership

- GIVEN user A has `platform:admin` but no `gateway:owner` binding for gateway gw-1
- WHEN a gRPC client with user A's credentials attempts to update gateway gw-1
- THEN the request returns PermissionDenied
- AND the response does not leak that gw-1 exists

#### Scenario: SEC-RBAC-09.03 -- Gateway creator cannot touch another tenant's gateway via gRPC

- GIVEN user B holds `gateway:creator` and no binding on gateway gw-1, owned by user A
- WHEN user B calls `UpdateGateway` (for example setting `image`) or `DeleteGateway` for gw-1
- THEN the request returns PermissionDenied
- AND `GetGateway` for gw-1 returns NotFound
- AND `ListGateways` returns PermissionDenied

#### Scenario: SEC-RBAC-09.04 -- Users cannot read role bindings over gRPC

- GIVEN any user without a control-plane identity
- WHEN they call `ListRoleBindings` or `WatchRoleBindings`
- THEN the request returns PermissionDenied

#### Scenario: SEC-RBAC-09.05 -- User cannot write an import ext kind over gRPC

- GIVEN a caller that is not a control-plane identity
- WHEN they call `CreateSandboxTemplate`, `UpdateProviderSpec`, or `DeleteSecretSource` over gRPC
- THEN the request returns PermissionDenied
- AND `platform:admin` MAY still list, watch, and get those kinds

#### Scenario: SEC-RBAC-09.06 -- Import reconciler writes an owned ext resource and ownership cannot be forged

- GIVEN the control-plane import reconciler reconciling `Repository` R1
- WHEN it calls `CreateSandboxTemplate` with a client-supplied `repository_id` of R2
- THEN the server SHALL set `repository_id` to R1 from the reconciled `Repository`
- AND it SHALL ignore the client-supplied R2 value

#### Scenario: SEC-RBAC-09.07 -- Import write is refused outside the owning Repository

- GIVEN a `SandboxTemplate` owned by `Repository` R2 or created directly with no ownership mark
- WHEN the import reconciler for `Repository` R1 attempts to update or delete it
- THEN the write SHALL be refused

#### Scenario: SEC-RBAC-09.08 -- Control plane reads CI kinds but no one writes them over gRPC

- GIVEN a registered control-plane identity reconciling CI federation
- WHEN it calls `WatchCITrusts`, `ListCIProviders`, or `GetCIProvider`
- THEN the read SHALL be authorized
- WHEN any caller (user or control plane) calls `CreateCIProvider`, `UpdateCITrust`, or `DeleteCITrust` over gRPC
- THEN the request SHALL return PermissionDenied (these kinds are authored over HTTP)

### Requirement: SEC-RBAC-18 -- Gateway Field Write Policy

The gateway write surface SHALL enforce a single, default-deny, field-level write
policy. The field ownership and allowlist are defined canonically in
[`platform/data-model.spec.md`](../platform/data-model.spec.md) ("Requirement: Gateway
Field Ownership and Write Policy", PLAT-DATA-23); this requirement defines how the
authorization layer identifies the caller and where the policy is enforced so REST,
gRPC, and `hsctl apply` are all covered at ONE chokepoint.

The authorization layer SHALL mark every gateway write request in the request context
with one of three caller kinds, and the gateway service write funnel
(`service.Replace` for update and `service.Create` for create) SHALL enforce the
field policy from that marker:

- **`user`** - a normal authenticated REST or gRPC caller (including `hsctl apply`).
- **`import`** - a control-plane gRPC request carrying the
  `x-hypershell-import-repository-id` metadata (the declarative `.hypershell` import),
  detected by the same mechanism that enforces import ownership in this spec
  (SEC-RBAC-09, "Declarative-import writes").
- **`control-plane`** - a control-plane gRPC request WITHOUT import metadata (the
  internal reconcile write-back path).

A `user` or `import` caller SHALL be allowed to write only the allowlist (`name` on
update; `name` + exactly one of `placement`/`cluster_id` + the create-only
`dedicated_route` request flag on create, PLAT-DATA-26). Any other field
that differs from the stored value (update) or is set (create) SHALL be rejected,
naming the field: `403 Forbidden` over REST, `PERMISSION_DENIED` over gRPC. Re-sending
an unchanged value SHALL be a no-op. Only a `control-plane` (non-import) caller SHALL
bypass the field policy and retain full write of platform-managed and observed fields.
An `import` caller SHALL NOT set `dedicated_route`: the service funnel rejects an
import create that requests it with its own entitlement-specific message (PLAT-DATA-26),
and the create-boundary entitlement check itself is defined in "Requirement: Gateway
Dedicated Route Creator Role" (SEC-RBAC-20).

Enforcing the policy in the service funnel - not per handler - closes the
whole-row-replace gap whereby a gRPC `UpdateGateway` could set more than a REST
`PATCH`, and closes the import loophole whereby a GitOps manifest could declare
platform-managed fields the direct user API forbids. This field policy is enforced
regardless of `RBAC_ENFORCE`: it is an ownership boundary on who authored the value,
not an access grant, so it does not relax when role enforcement is off; a caller that
needs to author platform-managed fields SHALL do so as the control-plane identity.

#### Scenario: SEC-RBAC-18.01 -- User gRPC UpdateGateway of a locked field is denied

- GIVEN a `user` gRPC `UpdateGateway` that changes a non-allowlisted field (for example `image` or `tls_mode`)
- WHEN the gateway service evaluates the field policy
- THEN it SHALL return `PERMISSION_DENIED` naming the field
- AND the whole-row replace SHALL NOT persist the change

#### Scenario: SEC-RBAC-18.02 -- Import write is held to name + placement

- GIVEN an `import`-marked gRPC `CreateGateway`/`UpdateGateway` that declares a platform-managed field
- WHEN the gateway service evaluates the field policy
- THEN it SHALL reject the write naming the field, exactly as for a `user` caller

#### Scenario: SEC-RBAC-18.03 -- Control-plane internal write bypasses the field policy

- GIVEN a `control-plane` gRPC `UpdateGateway` with NO import metadata
- WHEN it writes platform-managed or observed fields
- THEN the field policy SHALL permit the write

### Requirement: SEC-RBAC-19 -- Extension Kind REST Authorization

The Extension API kinds served under `/api/hypershell/ext/` (`AgentRuntime`,
`AgentWorkspace`, `WorkspaceMembership`, `SandboxTemplate`, `ProviderSpec`,
`ProviderBinding`, `InferenceRoute`, `SecretSource`, `Repository`, `CIProvider`,
`CITrust`) SHALL sit
behind the same authentication and authorization middleware as the `/v1`
kinds. The framework's route builder mounts the `ext` prefix on a subrouter
separate from `/v1`, so the RBAC middleware SHALL be registered on that
subrouter as well (`RegisterPrefixedRoutes("rbac", "ext", ...)`, exercised by
`components/api-server/test/authz`). The framework's own authorizer checks
identity presence only and SHALL NOT be treated as an authorization boundary
for these routes.

Every extension kind registered by a plugin with
`RegisterPrefixedRoutes(_, "ext", ...)` SHALL be present in the RBAC
allow-list (`extKinds`), so an ext path whose resource is not a known
extension kind SHALL be denied (fail closed) rather than falling back to the
`gateway:creator` default that unknown `/v1` resources receive. The allow-list
and the set of ext-registering plugins SHALL be kept in sync: a plugin that
registers an ext route but is absent from the allow-list is fail-closed for
every caller including `platform:admin`, which is a latent outage, not a
deliberate deny. `CIProvider` (`ci_providers`) and `CITrust` (`ci_trusts`)
SHALL be in the allow-list; they are the kinds whose absence previously made
every request to them return 403.

At the middleware gate, every ext method (read and write) SHALL require
`gateway:creator` or `platform:admin` by default: these kinds are authored over HTTP
(`hsctl` and the generated SDKs), which is the authoring posture the gRPC
rules defer to ("these kinds are authored over HTTP", gRPC Authorization). Two
kinds use a different gate instead of the `gateway:creator` default:

- `ci_providers` SHALL require the narrow `ci-federation:admin` role (or
  `platform:admin`) for every method, NOT `gateway:creator`; a forge
  registration is a platform operation, not a gateway-authoring one
  (SEC-RBAC-21).
- `ci_trusts` is a gateway-scoped tenancy kind authorized per referenced
  gateway (SEC-RBAC-22); a bare `gateway:creator` with no binding on the
  referenced gateway SHALL NOT author one.

The gateway-scoped ext kinds SHALL additionally enforce per-resource tenancy
in their handlers, because the creator gate alone cannot confine a creator to
their own gateways:

- `AgentRuntime` create, update, and delete SHALL require `gateway:owner` or
  `gateway:admin` on the referenced gateway, or `platform:admin`
  (owner-equivalent for management, GTW-ACC-08).
- `SecretSource` create, update, and delete SHALL resolve the target
  `agent_runtime_id` to its `AgentRuntime` and require `gateway:owner` or
  `gateway:admin` on that runtime's gateway, or `platform:admin`. A submission
  naming an `AgentRuntime` that does not exist SHALL be rejected with `404`
  and SHALL write nothing to the secret store.
- The `SecretSource` secret-store write SHALL happen only after the tenancy
  check passes; a rejected submission SHALL leave the store and the database
  unchanged.
- `CITrust` create, update, and delete SHALL require `gateway:owner` or
  `gateway:admin` on the referenced gateway, or `platform:admin`, through the
  same `CanManageGatewayResources` check the other gateway-scoped kinds use.
  The `gateway_id` SHALL be read from the request body on create and from the
  stored record on update and delete, mirroring how `AgentRuntime` resolves and
  enforces its gateway scope in-handler. The full tenancy contract and its
  scenarios are in SEC-RBAC-22.

The `SecretSource` `path` SHALL be derived server-side from the submission
identity on every create, value-bearing and reference-only alike; a
client-supplied `path` SHALL be ignored on create and SHALL NOT be settable on
update. Honoring a client path would let any creator point a `SecretSource` at
another tenant's store location, which the control plane would then place
into the caller's namespace. The control-plane declarative import reconciler
writes its own paths over gRPC and is unaffected by this rule.

#### Scenario: SEC-RBAC-19.01 -- Unbound creator cannot overwrite another tenant's secret

- GIVEN user A holds `gateway:owner` on gw-1 and has declared an AgentRuntime
  on gw-1 with a submitted secret
- AND user B holds `gateway:creator` and no binding on gw-1
- WHEN user B submits a secret value naming that AgentRuntime and its secret
  name
- THEN the response SHALL be `403 Forbidden`
- AND the secret store entry at the AgentRuntime's deterministic location
  SHALL be unchanged

#### Scenario: SEC-RBAC-19.02 -- Reference-only create ignores a client-supplied path

- GIVEN a caller authorized on the target AgentRuntime's gateway
- WHEN the caller creates a reference-only `SecretSource` with a
  client-supplied `path` pointing at another resource's store location
- THEN the created record's `path` SHALL be the server-derived convention
  location, not the client-supplied value

#### Scenario: SEC-RBAC-19.03 -- Unbound creator cannot plant an AgentRuntime on another tenant's gateway

- GIVEN user B holds `gateway:creator` and no binding on gw-1 (owned by user A)
- WHEN user B creates an `AgentRuntime` referencing gw-1
- THEN the response SHALL be `403 Forbidden`

#### Scenario: SEC-RBAC-19.04 -- platform:admin manages gateway-scoped ext kinds

- GIVEN user P holds `platform:admin` and no binding on gw-1
- WHEN user P creates, updates, or deletes an `AgentRuntime` or `SecretSource`
  scoped to gw-1
- THEN the request SHALL be authorized (owner-equivalent, GTW-ACC-08)

#### Scenario: SEC-RBAC-19.05 -- Unknown ext resource fails closed

- GIVEN a request to `/api/hypershell/ext/<unknown-kind>`
- WHEN the RBAC middleware evaluates it
- THEN the request SHALL be denied even for a `gateway:creator` caller

### Requirement: SEC-RBAC-20 -- Gateway Dedicated Route Creator Role

A platform-wide role `gateway:dedicated-route-creator` SHALL gate the create-only
`dedicated_route` request field (the dedicated per-gateway route and wildcard
certificate path,
[`platform/data-model.spec.md`](../platform/data-model.spec.md) "Requirement:
Gateway Dedicated Route Request Field", PLAT-DATA-26, and
[`platform/openshell-gateway-routing.spec.md`](../platform/openshell-gateway-routing.spec.md)
OSG-ROUT-12). The role exists because a dedicated per-gateway route and certificate
is a rate-limited resource (one certificate order, one load balancer, and one
wildcard DNS record per gateway), so the entitlement is deliberately separate from
`gateway:creator` and is not granted to every creator.

**Assignment (Keycloak-owned, JWT-synced).** The role SHALL be JWT-synced, like
`gateway:creator` and `platform:admin`: assignment is owned by Keycloak (a realm
role synced to a global-scope RoleBinding on sign-in), the RoleBinding API SHALL
reject a direct assignment with 403 and a gateway-scoped binding with 400 (the role
requires scope=global), and a binding whose role the JWT no longer carries SHALL be
removed on the caller's next sign-in. The role record SHALL be seeded by migration
with `built_in: true` (SEC-RBAC-15).

**Grants nothing else.** Holding `gateway:dedicated-route-creator` SHALL grant
nothing beyond the right to set `dedicated_route = true` on a gateway create. It
does not create or own gateways (that still requires `gateway:creator`), confers no
per-gateway access, no OpenShell mapping, and no ManagedCluster or ext-kind
permissions: it is a marker/entitlement role checked only at the gateway create
boundary. `platform:admin` SHALL also qualify to request `dedicated_route`.

**Enforcement (body-dependent, at the create boundaries).** The entitlement SHALL
be enforced in the REST gateway Create handler and the gRPC `CreateGateway` handler,
NOT in the path authorization middleware: the request is carried in the body, which
the middleware never sees. The handler SHALL resolve the caller's role bindings
through the role-binding service lookup and fail closed: a missing caller identity,
an unwired lookup, or bindings holding neither `gateway:dedicated-route-creator` nor
`platform:admin` SHALL reject the create with 403 (`PermissionDenied` over gRPC)
naming the role and the rate-limit rationale. A binding-lookup failure SHALL surface
as a server error rather than an authorization denial, so a transient database
problem does not tell the caller to request a role it may already hold. The check
SHALL be a no-op exactly when request-level RBAC enforcement is off
(`RBAC_ENFORCE=false`). A privileged control-plane write bypasses the check; a
declarative-import create that sets the flag is rejected by the service funnel with
its own message (PLAT-DATA-26), because a control-plane identity holds no user
bindings.

#### Scenario: SEC-RBAC-20.01 -- Entitled caller requests a dedicated route

- GIVEN a caller holding `gateway:creator` and `gateway:dedicated-route-creator`
  (both from Keycloak)
- WHEN the caller creates a gateway with `dedicated_route = true` over REST or gRPC
- THEN the create SHALL be accepted and the route opt-in applied (PLAT-DATA-26)

#### Scenario: SEC-RBAC-20.02 -- Plain creator is rejected naming the role

- GIVEN a caller holding only `gateway:creator`
- WHEN the caller creates a gateway with `dedicated_route = true`
- THEN the response SHALL be 403 Forbidden over REST and PermissionDenied over gRPC
- AND the reason SHALL name `gateway:dedicated-route-creator` and state the
  rate-limit rationale
- AND no gateway SHALL be persisted
- AND a create from the same caller that does not set the flag is unaffected

#### Scenario: SEC-RBAC-20.03 -- platform:admin qualifies

- GIVEN a caller holding `platform:admin` (and `gateway:creator`)
- WHEN the caller creates a gateway with `dedicated_route = true`
- THEN the create SHALL be accepted

#### Scenario: SEC-RBAC-20.04 -- The role grants nothing else

- GIVEN a caller holding only `gateway:dedicated-route-creator`
- WHEN the caller attempts to create a gateway
- THEN the create SHALL be rejected with 403 (creating gateways still requires
  `gateway:creator`)
- AND the caller SHALL hold no per-gateway access through the role

#### Scenario: SEC-RBAC-20.05 -- Assignment is Keycloak-owned

- GIVEN an attempt to create a `gateway:dedicated-route-creator` RoleBinding through
  the RoleBinding API
- WHEN the binding is global-scoped
- THEN the create SHALL be rejected with 403 (the role can only be assigned via
  Keycloak)
- AND a gateway-scoped binding SHALL be rejected with 400 (the role requires
  scope=global)
- AND a JWT carrying the realm role SHALL sync the global binding the create path
  checks

#### Scenario: SEC-RBAC-20.06 -- The gate fails closed

- GIVEN a create requesting `dedicated_route` from a user caller
- WHEN the caller identity is missing or the binding lookup is unwired
- THEN the create SHALL be rejected with the 403 entitlement rejection
- WHEN the binding lookup itself fails
- THEN the failure SHALL surface as a server error, not an authorization denial

### Requirement: SEC-RBAC-21 -- CI Federation Admin Role

A platform-wide role `ci-federation:admin` SHALL authorize the `ci_providers`
extension kind (`CIProvider`, PLAT-DATA-24: register, read, update, and delete a
CI OIDC forge) and SHALL authorize nothing else. The role exists so the gitops
CI-federation seeder can register forges with a least-privilege credential
instead of `platform:admin`: forge registration is a narrow platform operation,
and granting the seeder blanket platform administration to perform it would be
over-broad.

**Authorization.** At the ext middleware gate, every `ci_providers` method
(create, read, update, delete) SHALL require `ci-federation:admin` or
`platform:admin`. A caller holding neither SHALL be denied: a `ci_providers`
collection request without the role SHALL be rejected with 403, and a request
to a specific `ci_providers/{id}` MAY be masked as 404 per the not-found-masking
convention (SEC-RBAC-10). `gateway:creator` alone SHALL NOT authorize
`ci_providers`; the generic ext creator gate does not apply to this kind
(SEC-RBAC-19).

**Assignment (Keycloak-owned, JWT-synced).** The role SHALL be JWT-synced, like
`gateway:creator`, `gateway:dedicated-route-creator`, and `platform:admin`:
assignment is owned by Keycloak (a realm role synced to a global-scope
RoleBinding on sign-in), the RoleBinding API SHALL reject a direct assignment
with 403 and a gateway-scoped binding with 400 (the role requires scope=global),
and a binding whose role the JWT no longer carries SHALL be removed on the
caller's next sign-in. The role record SHALL be seeded by an additive,
idempotent migration with `built_in: true`, mirroring how `gateway:admin` and
`gateway:dedicated-route-creator` were added (SEC-RBAC-15); the seed SHALL be
re-runnable without error and SHALL NOT alter existing bindings.

**Grants nothing else.** Holding `ci-federation:admin` SHALL grant no gateway
creation, no per-gateway access, no OpenShell mapping, no ManagedCluster
authority, and no other ext-kind authority (it does not authorize `ci_trusts`,
`agent_runtimes`, `secret_sources`, or any other kind). It is a marker
entitlement checked only at the `ci_providers` ext gate. `platform:admin`
remains owner-equivalent and also qualifies.

#### Scenario: SEC-RBAC-21.01 -- CI federation admin registers a provider

- GIVEN a caller (for example the gitops seeder service account) holding only `ci-federation:admin`
- WHEN the caller creates, updates, reads, or deletes a `CIProvider`
- THEN the request SHALL be authorized

#### Scenario: SEC-RBAC-21.02 -- Caller without the role is denied

- GIVEN a caller holding only `gateway:creator` (and no `ci-federation:admin` or `platform:admin`)
- WHEN the caller creates a `CIProvider`
- THEN the response SHALL be 403 Forbidden
- AND a request to a specific `ci_providers/{id}` MAY be masked as 404

#### Scenario: SEC-RBAC-21.03 -- The role grants nothing else

- GIVEN a caller holding only `ci-federation:admin`
- WHEN the caller attempts to create a `Gateway`, create a `CITrust`, or write any other ext kind
- THEN every such request SHALL be denied (the role authorizes only `ci_providers`)

#### Scenario: SEC-RBAC-21.04 -- platform:admin qualifies

- GIVEN a caller holding `platform:admin` and no `ci-federation:admin`
- WHEN the caller manages a `CIProvider`
- THEN the request SHALL be authorized (owner-equivalent)

#### Scenario: SEC-RBAC-21.05 -- Assignment is Keycloak-owned

- GIVEN an attempt to create a `ci-federation:admin` RoleBinding through the RoleBinding API
- WHEN the binding is global-scoped
- THEN the create SHALL be rejected with 403 (the role can only be assigned via Keycloak)
- AND a gateway-scoped binding SHALL be rejected with 400 (the role requires scope=global)
- AND a JWT carrying the realm role SHALL sync the global binding the gate checks

### Requirement: SEC-RBAC-22 -- CITrust Per-Gateway Tenancy

The `ci_trusts` extension kind (`CITrust`, PLAT-DATA-25) SHALL be authorized per
referenced gateway, reusing the existing per-gateway tenancy model rather than a
new platform role. A `CITrust` declares that a repository's CI may federate onto
a gateway the caller already controls, so the right to author one SHALL be the
right to manage that gateway's resources.

**Write authorization.** `CITrust` create, update, and delete SHALL require
`gateway:owner` or `gateway:admin` on the referenced gateway, or `platform:admin`
(owner-equivalent for management, GTW-ACC-08), through the shared
`CanManageGatewayResources` check. The `gateway_id` SHALL be resolved from the
request body on create and from the stored record on update and delete, so the
enforced gateway is always the one the `CITrust` actually targets. This is the
same resolution-and-enforcement pattern `agent_runtimes` and `secret_sources`
use. The `role` a `CITrust` grants SHALL NOT exceed the caller's own rights on
that gateway (PLAT-CITR-03 / PLAT-DATA-25.03).

**Deny-by-default.** A caller with no `gateway:owner`, `gateway:admin`, or
`platform:admin` authority over the referenced gateway SHALL be denied: an
unbound caller, a `gateway:creator` with no binding on that gateway, and a
`gateway:viewer` on that gateway SHALL all be rejected on write. A caller bound
only to a different gateway SHALL NOT manage another gateway's trusts.

**Reads.** A `gateway:owner`, `gateway:admin`, or `gateway:viewer` on the
referenced gateway (or `platform:admin`) MAY read that gateway's `CITrust`
records; a caller with no binding on the referenced gateway SHALL NOT, and a
request to a specific `ci_trusts/{id}` MAY be masked as 404 per SEC-RBAC-10.

#### Scenario: SEC-RBAC-22.01 -- Gateway owner creates a trust on their own gateway

- GIVEN caller A holds `gateway:owner` on gw-1
- WHEN caller A creates a `CITrust` referencing gw-1
- THEN the request SHALL be authorized

#### Scenario: SEC-RBAC-22.02 -- Gateway admin may manage trusts

- GIVEN caller A holds `gateway:admin` on gw-1
- WHEN caller A creates, updates, or deletes a `CITrust` referencing gw-1
- THEN the request SHALL be authorized

#### Scenario: SEC-RBAC-22.03 -- Viewer cannot write, deny-by-default holds

- GIVEN caller V holds `gateway:viewer` on gw-1 and caller U holds no binding on gw-1
- WHEN either creates, updates, or deletes a `CITrust` referencing gw-1
- THEN the response SHALL be 403 Forbidden
- AND no Keycloak mapping or RBAC binding SHALL be created

#### Scenario: SEC-RBAC-22.04 -- Cross-gateway isolation

- GIVEN caller A holds `gateway:owner` on gw-a and no binding on gw-b
- WHEN caller A creates, updates, or deletes a `CITrust` referencing gw-b
- THEN the response SHALL be 403 Forbidden (a specific-id request MAY be 404)

#### Scenario: SEC-RBAC-22.05 -- Owner, admin, or viewer may read a gateway's trusts

- GIVEN caller R holds `gateway:viewer` on gw-1
- WHEN caller R reads the `CITrust` records for gw-1
- THEN the read SHALL be authorized
- AND a caller with no binding on gw-1 SHALL NOT read them

### Requirement: SEC-RBAC-23 -- Low-Level RoleBinding Write Authorization

The low-level generic `RoleBinding` resource (`POST /api/hypershell/v1/role_bindings`
and `DELETE /api/hypershell/v1/role_bindings/{id}`) SHALL authorize every create and
every delete against an explicit, default-deny write policy. A caller SHALL change a
binding only when a rule grants that exact operation on that exact target role. Being
authenticated, or holding an unrelated role binding, SHALL NOT by itself authorize a
write. A role for which no rule grants the operation SHALL be denied, so adding a role
to the database never silently makes it assignable or removable through this resource.

This requirement governs write authorization only. It SHALL NOT broaden or narrow read
access to roles, role bindings, or any other resource.

**Write policy.** The resource SHALL enforce this table for both create and delete:

| Target role | Create | Delete |
| --- | --- | --- |
| `gateway:owner`, `gateway:admin`, `gateway:viewer` | Require the caller to hold `gateway:owner` on the exact target gateway | Require the caller to hold `gateway:owner` on the stored binding's actual gateway |
| Keycloak-owned platform roles (`platform:admin`, `gateway:creator`, `gateway:dedicated-route-creator`, `ci-federation:admin`) | Reject; these are JWT-synced and managed through Keycloak | Reject; these are JWT-synced and managed through Keycloak |
| `managed-cluster-registrar` | Reject direct user assignment; registration stays JWT-controlled (SEC-RBAC-16) | Reject direct user deletion |
| Any other or future role | Reject until an explicit policy is added | Reject until an explicit policy is added |

The gateway-role rows SHALL reuse the same gateway ownership and owner-removal rules as
the supported access-management workflow (`platform/gateway-access-management.spec.md`
GTW-ACC-04/GTW-ACC-05/GTW-ACC-06), including last-owner protection (GTW-ACC-07): a
delete that would remove a gateway's last `gateway:owner` SHALL be refused. The
low-level resource SHALL NOT be a weaker path than the access facade and SHALL NOT let a
`gateway:admin` or `gateway:viewer` bypass the owner requirement. Richer delegation (a
`gateway:admin` granting `gateway:admin`/`gateway:viewer`) remains available through the
access facade, not this generic resource.

`platform:admin` SHALL NOT receive a blanket bypass on this low-level resource; a
platform administrator manages access through the facade at
`/api/hypershell/v1/gateways/{id}/access` instead (SEC-RBAC-07). A `platform:admin` who
also holds `gateway:owner` on the target gateway qualifies through that ownership, like
any other owner.

**Shared enforcement in the service.** A single shared authorization check SHALL be
used by both `Create` and `Delete`. It SHALL evaluate the operation, the authenticated
caller, the stored role, the scope, and the target gateway, and SHALL run before any row
or event is written. For delete, the service SHALL load the existing binding and
authorize against its stored role, scope, and gateway; it SHALL NOT trust a
caller-supplied scope or gateway to decide which permissions apply. The HTTP RBAC
middleware MAY remain as an additional coarse gate, but the service check SHALL be the
authoritative decision and SHALL hold even when the service is called outside the REST
handler. These rules SHALL apply equally to create and delete and SHALL NOT depend on
the middleware having run first.

**Missing identity.** A user-facing write with no authenticated caller identity SHALL be
rejected. The ownership check SHALL NOT treat a missing caller user id as permission;
the previous success-on-missing-identity path SHALL be removed.

**Target validation.** A create SHALL require a valid, existing target user and an
existing role. A gateway-scoped binding SHALL reference an existing gateway and use
`scope=gateway`; a global role SHALL use `scope=global`. Missing or empty user, role, or
gateway ids, and scope/role combinations that do not match, SHALL be rejected before
persistence.

**No persistent effect on denial.** A rejected create or delete SHALL leave both the
RoleBinding rows and the Event stream unchanged.

**Trusted internal operations stay explicit.** Server-initiated writes -- initial
gateway-owner creation on gateway create (SEC-RBAC-04), JWT role synchronization
(SEC-RBAC-03), control-plane reconciliation, and cleanup -- SHALL be performed through
dedicated internal methods with a defined caller and purpose, NOT by permitting
anonymous service calls or a general client-controlled bypass of this policy. Existing
gRPC restrictions (SEC-RBAC-09: `RoleBindingService` is denied to user callers) and
managed-cluster boundaries SHALL remain intact.

Regression coverage SHALL exercise this policy through the real RBAC middleware and the
role-binding service (`components/api-server/plugins/roleBindings`,
`components/api-server/test/authz`), proving that denied operations make no persistent
change and that the legitimate workflows above remain functional.

#### Operator Note: Pre-Deployment Binding Inventory

Before this policy is deployed, an operator SHALL inspect whether any existing
RoleBinding uses a role the new policy would reject. Such bindings SHALL be reported for
review; the rollout SHALL NOT automatically delete or rewrite them. Reconciliation may
later restore a binding a caller deletes; that restoration does not make the deletion
authorized.

#### Scenario: SEC-RBAC-23.01 -- Unauthenticated caller cannot create or delete

- GIVEN a caller with no authenticated identity
- WHEN the caller attempts `POST /api/hypershell/v1/role_bindings` or `DELETE /api/hypershell/v1/role_bindings/{id}`
- THEN the request SHALL be rejected
- AND no RoleBinding row and no Event SHALL be written

#### Scenario: SEC-RBAC-23.02 -- Default creator cannot grant itself a Keycloak-owned role

- GIVEN user A holds only the default `gateway:creator` role
- WHEN user A creates a `role_bindings` entry for itself requesting `platform:admin`, `gateway:creator`, `gateway:dedicated-route-creator`, or `ci-federation:admin`
- THEN the response SHALL be 403 Forbidden naming the role as Keycloak-managed
- AND no binding SHALL be created

#### Scenario: SEC-RBAC-23.03 -- Unrelated user cannot delete another user's global binding

- GIVEN user A holds only `gateway:creator`
- AND user B holds a Keycloak-owned or otherwise global binding
- WHEN user A deletes user B's binding through the low-level resource
- THEN the response SHALL be 403 Forbidden
- AND user B's binding SHALL remain

#### Scenario: SEC-RBAC-23.04 -- Gateway owner grants and revokes supported roles on their gateway

- GIVEN user A holds `gateway:owner` on gw-1
- WHEN user A creates a `gateway:viewer`, `gateway:admin`, or `gateway:owner` binding on gw-1 for user B
- THEN the create SHALL be authorized
- WHEN user A later deletes one of those bindings
- THEN the delete SHALL be authorized, subject to last-owner protection (GTW-ACC-07)
- AND a delete that would remove gw-1's last `gateway:owner` SHALL be refused

#### Scenario: SEC-RBAC-23.05 -- Admin or viewer cannot bypass the owner requirement

- GIVEN user A holds `gateway:owner` on gw-1 only
- AND user C holds `gateway:admin` on gw-2 and user V holds `gateway:viewer` on gw-2
- WHEN user A creates or deletes a binding on gw-2 (a gateway A does not own)
- THEN the response SHALL be 403 Forbidden
- AND when user C or user V attempts the same operation on gw-2 through the low-level resource
- THEN the response SHALL also be 403 Forbidden (the low-level resource requires `gateway:owner`)

#### Scenario: SEC-RBAC-23.06 -- Role with no explicit policy is rejected for create and delete

- GIVEN the seeded `managed-cluster-registrar` role (or any role the policy does not grant)
- WHEN a caller creates a binding for that role through the low-level resource
- THEN the response SHALL be 403 Forbidden
- AND when a caller deletes an existing binding for that role
- THEN the response SHALL be 403 Forbidden
- AND no row or Event SHALL change

#### Scenario: SEC-RBAC-23.07 -- Invalid target is rejected before persistence

- GIVEN a create through the low-level resource
- WHEN the caller identity is missing, the target user id is missing or unknown, the referenced gateway does not exist, or the scope and role do not match
- THEN the request SHALL be rejected before any row or Event is written

#### Scenario: SEC-RBAC-23.08 -- Delete authorizes against stored values

- GIVEN an existing `gateway:viewer` binding on gw-1 for user B
- WHEN user A (who does not own gw-1) deletes it while supplying a scope or gateway id that would otherwise pass
- THEN the service SHALL authorize against the binding's stored role, scope, and gw-1 ownership
- AND the delete SHALL be refused
- AND the binding SHALL remain

#### Scenario: SEC-RBAC-23.09 -- Trusted internal and facade workflows still work

- GIVEN the server-initiated paths: gateway-owner creation on gateway create, JWT role synchronization, reconciliation, cleanup, and the access facade
- WHEN each runs through its dedicated internal method or the facade
- THEN each SHALL continue to create or remove bindings as before
- AND none SHALL depend on an anonymous service call or a general client-controlled bypass

### Requirement: SEC-RBAC-10 -- Error Response Opacity

For singleton resource endpoints (`GET /gateways/{id}`), the middleware SHALL return 404
when the caller has no binding that covers the requested resource. Returning 403 on a
singleton GET leaks resource existence.

For list endpoints, the middleware SHALL return 200 with an empty items array when the
caller has no matching resources. Collection `GET /gateways` SHALL be authorized for
any authenticated caller, including a user whose JWT carries only `hypershell-users`
and who has no `gateway:creator`, `platform:admin`, or per-gateway RoleBinding
(OpenShift `RBAC_DEFAULT_ROLES=`). The list handler then returns the empty collection.
Denying that list with 403 is a product bug: the web console treats it as
"Gateways could not be loaded".

#### Scenario: SEC-RBAC-10.01 -- Developer with no gateway bindings lists gateways

- GIVEN production deployment with `RBAC_DEFAULT_ROLES=`
- AND user A is authenticated
- AND user A's JWT does not carry `gateway:creator` or `platform:admin`
- AND user A has no per-gateway RoleBinding
- WHEN user A calls `GET /api/hypershell/v1/gateways`
- THEN the response is 200
- AND `items` is an empty array

For mutation endpoints where the caller lacks write permission, the middleware SHALL
return 403.

### Requirement: SEC-RBAC-11 -- Auth-Exempt Endpoints

The following endpoints SHALL require only authentication (valid JWT), not authorization:

| Endpoint | Reason |
|----------|--------|
| `GET /api/hypershell/v1/roles` | Discovery -- users need to see available roles |
| `GET /api/hypershell/v1/roles/{id}` | Discovery -- read a specific role's permissions |

Health, metrics, and version endpoints are already bypassed at the authentication
layer.

### Requirement: SEC-RBAC-12 -- Audit Logging

Platform administrator actions SHALL be logged with:

- Action performed (view, delete)
- Actor identity (platform:admin user)
- Target resource (gateway ID and name)
- Timestamp and correlation ID

High-privilege operations (gateway deletion by platform:admin) SHALL be logged at INFO
level or higher to ensure visibility in operational monitoring and security audits.

### Requirement: SEC-RBAC-13 -- Default Role Bootstrap (Development Mode Only)

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

#### Scenario: SEC-RBAC-13.01 -- Local development with defaults (NOT for production)

- GIVEN `RBAC_DEFAULT_ROLES=gateway:creator` (local dev only)
- AND a developer authenticates with a JWT carrying no Keycloak realm roles
- WHEN any authenticated API request is processed
- THEN the middleware creates a `gateway:creator` RoleBinding automatically
- AND the developer can create gateways without Keycloak configuration

#### Scenario: SEC-RBAC-13.02 -- Production mode -- no defaults, Keycloak is authoritative

- GIVEN `RBAC_DEFAULT_ROLES=` (production)
- AND `RBAC_ENFORCE=true`
- AND a user has no Keycloak realm roles assigned
- WHEN the user makes an authenticated API request
- THEN no default binding is created
- AND the user cannot create gateways until a Keycloak admin assigns `gateway:creator`

### Requirement: SEC-RBAC-14 -- Production Rollout

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

### Requirement: SEC-RBAC-15 -- Database Migration

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

A migration SHALL also seed the `gateway:dedicated-route-creator` role record with
`built_in: true` ("Gateway Dedicated Route Creator", display name and description
carrying the rate-limit rationale; see "Requirement: Gateway Dedicated Route
Creator Role", SEC-RBAC-20). Unlike `managed-cluster-registrar`, it IS JWT-synced:
its bindings are assigned via Keycloak and reconciled by `SyncJWTRoles` exactly like
`gateway:creator` and `platform:admin`.

A migration SHALL also seed the `ci-federation:admin` role record with
`built_in: true` ("CI Federation Administrator", a display name and description
naming its scope as `ci_providers` registration only; see "Requirement: CI
Federation Admin Role", SEC-RBAC-21). Like `gateway:dedicated-route-creator`, it
IS JWT-synced: its bindings are assigned via Keycloak and reconciled by
`SyncJWTRoles`. The migration SHALL be additive and idempotent, mirroring the
`gateway:admin` seed: re-running it SHALL NOT error and SHALL NOT alter existing
bindings.

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
4. Assign `managed-cluster-registrar` to every control-plane service account, including the control plane co-located with the hub (every control plane registers; see `platform/control-plane.spec.md`).
5. Ensure Keycloak emits these roles in the `realm_access.roles` claim.

With `RBAC_DEFAULT_ROLES=` set, no authenticated principal receives any role
automatically. Every permission flows exclusively from Keycloak. Callers without an
appropriate role receive 403 on mutation endpoints and 404 on singleton GETs.

### Requirement: SEC-RBAC-16 -- Managed Cluster Self-Registration RBAC

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

Assigning `managed-cluster-registrar` to a control-plane service account is a Keycloak
admin function performed out-of-band before that control plane is deployed; the
development realms ship it pre-assigned to the `hypershell-control-plane` client. Keycloak is the trusted
source of truth; the API server does not re-verify role assignment beyond reading the
JWT claim.

**Isolation guarantee:** In production (`RBAC_DEFAULT_ROLES=`, `RBAC_ENFORCE=true`),
a spoke service account holding only `managed-cluster-registrar` has no gateway
permissions through the HTTP API. No role is auto-assigned; the spoke's HTTP access is
exactly what Keycloak grants. This is the required production configuration. Once the
spoke registers, its JWT `sub` is a control-plane identity on the gRPC API: the gRPC
interceptor exempts it from role bindings, like an `RBAC_SERVICE_ACCOUNTS` entry (see
`platform/managed-cluster-registration.spec.md`, "Control-Plane Identity").

#### Scenario: SEC-RBAC-16.01 -- Spoke with role can self-register

- GIVEN a spoke service account with `managed-cluster-registrar` assigned in Keycloak
- WHEN it calls `POST /managed_clusters/registration`
- THEN the `isAuthorized` JWT-direct check passes
- AND the request proceeds to the handler
- AND a `ManagedCluster` record is created (or the existing one is returned)

#### Scenario: SEC-RBAC-16.02 -- Spoke without role is rejected

- GIVEN a spoke service account without `managed-cluster-registrar` in Keycloak
- WHEN it calls `POST /managed_clusters/registration`
- THEN the RBAC middleware returns 403 Forbidden
- AND no `ManagedCluster` record is created or modified

#### Scenario: SEC-RBAC-16.03 -- managed-cluster-registrar grants no gateway access

- GIVEN production deployment with `RBAC_DEFAULT_ROLES=` and `RBAC_ENFORCE=true`
- AND a spoke service account has only `managed-cluster-registrar` assigned in Keycloak
- WHEN it calls `GET /api/hypershell/v1/gateways`
- THEN the response is 200 with an empty items array
- AND the spoke cannot create, modify, or delete any gateway through the HTTP API

### Requirement: SEC-RBAC-17 -- Integration Test Coverage

Integration tests SHALL exercise RBAC enforcement with the new five-role model, including
`managed-cluster-registrar` grant and deny scenarios, and the JWT-direct enforcement path.
They SHALL also run the api-server as the GitOps hubs run it (`RBAC_ENFORCE=true`,
`RBAC_DEFAULT_ROLES` unset, a one-entry allowlist) and show that an ordinary user and a
foreign control plane are refused ManagedCluster writes and cross-tenant or cross-cluster
gRPC writes (`components/api-server/test/authz`).

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
| gRPC decides each method explicitly, default-deny | The previous rule granted `gateway:creator` or `gateway:owner` every gRPC method, so any signed-in user could rewrite or delete any gateway once the hub's gRPC API became public. A per-method table matches the HTTP rules and fails closed for methods added later. |
| `platform:admin` is view + delete only | Platform admins handle operational tasks (viewing all gateways, cleaning up orphaned resources). Full modification requires ownership to prevent accidental changes. Separation of concerns: visibility ≠ modification authority. |
| `platform:admin` orthogonal to `gateway:creator` | A platform admin may or may not create gateways. Roles compose: `platform:admin` + `gateway:creator` allows both operational oversight and resource creation. |
| JWT roles synced to DB on every request | DB is the projection, Keycloak is the authority. Revocations in Keycloak take effect immediately. Existing per-gateway bindings are unaffected by platform role changes. |
| Service accounts treated identically to users | Control plane gets `gateway:creator` in Keycloak, provisions like any user. No special bypass logic needed. |
| `managed-cluster-registrar` is separate from gateway roles | A spoke service account only needs fleet membership rights, not gateway creation rights. Keeping the roles separate limits blast radius if a spoke credential is compromised. Operators who want strict isolation must also set `RBAC_DEFAULT_ROLES=` to disable the universal `gateway:creator` default. |
| `gateway:dedicated-route-creator` separate from `gateway:creator` | A dedicated per-gateway route and certificate is a rate-limited resource (one certificate order, load balancer, and wildcard DNS record per gateway). A distinct platform-wide entitlement lets operators grant it narrowly instead of to every creator; it is a marker role checked at the create boundary and grants nothing else (SEC-RBAC-20). |
| `managed-cluster-registrar` is JWT-direct, not DB-synced | The role is for specific service accounts, not users in general. There is no reason to maintain a DB binding for it. Checking from the live JWT claim in `isAuthorized` is sufficient, consistent with other JWT-direct roles, and avoids `JWTSyncedRoles` entanglement. |
| Role assigned by admin, not auto-granted | Provides a human control point for fleet membership. A new spoke cannot join the fleet without an explicit Keycloak admin action. |
| Gateway owners can grant co-owners | No hierarchy restriction. Team leads assign `gateway:creator` to team members or invite them as owners/viewers per gateway. Simple mental model. |
| Auto-assign `gateway:owner` on creation | Creator automatically owns what they create. No separate grant step needed. |
| `gateway:creator` assigned exclusively via Keycloak in production | Production deployments set `RBAC_DEFAULT_ROLES=` so only Keycloak-assigned roles apply. `RBAC_DEFAULT_ROLES=gateway:creator` exists as a local-dev escape hatch only. The role cannot be self-assigned via the API. |
| Per-gateway bindings stored in DB | Gateway-scoped access requires per-resource granularity that JWT claims cannot provide (you'd need dynamic claim values per gateway ID). |
| 404 on unauthorized singleton GETs | Returning 403 confirms the resource exists. 404 prevents ID enumeration. |
| OpenShell role bridge | `gateway:owner` maps to `openshell-admin`, `gateway:viewer` maps to `openshell-user`. Ensures consistent access via CLI. |
| OpenShellGatewayServiceAccount role limit | A gateway binding limits the selected OpenShell role. Owners can select `openshell-user` or `openshell-admin`. Viewers can select only `openshell-user`. Each OpenShellGatewayServiceAccount remains bound to its creator. A binding change can downgrade or revoke it. |
