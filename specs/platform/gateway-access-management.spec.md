# Gateway Access Management Specification

**Date:** 2026-10-05
**Status:** Draft
**Applies to:** `components/api-server` roleBindings/roles/gateways/users plugins and RBAC middleware, `components/control-plane` Keycloak client and RoleBinding reconciler, `components/sdk-typescript`, `components/web-console` BFF
**Parent:** `security/rbac-enforcement.spec.md` -- scope-aware RBAC model and RoleBinding storage
**Related:** `openshell-gateway-keycloak.spec.md` -- per-gateway Keycloak clients and the OIDC Role Bridge; `openshell-gateway-service-accounts.spec.md` -- gateway-nested sub-resource and role capping; `registered-users.spec.md` -- registered User records and the read-only users inventory; `data-model.spec.md` -- Gateway, Role, RoleBinding, and User kinds; `web-console/gateway-access-management.spec.md` -- the Manage access console surface

---

## Purpose

This specification defines **gateway access management**: how a gateway's administrators grant, change, and revoke other users' access to a single gateway, and how those grants are projected into per-gateway Keycloak client roles so the granted users receive the same gateway access through the `openshell` CLI as they do in the management plane.

Access is already modeled as per-gateway `RoleBinding` records and already projected into Keycloak by the control plane's OIDC Role Bridge (`openshell-gateway-keycloak.spec.md`). This specification:

1. Adds a third per-gateway role, `gateway:admin`, so a **granted administrator** is distinct from the **creator** (`gateway:owner`).
2. Defines a gateway-scoped **access facade** (`/api/hypershell/v1/gateways/{gateway_id}/access`) that presents enriched access grants, atomic role changes, and creator protection in application language, over the existing RoleBinding storage and watch events.
3. Defines a **Keycloak directory search** so administrators can grant access to any user in the Keycloak realm, including users who have never signed in to HyperShell, pre-provisioning a HyperShell `User` record on grant.
4. Aligns the Keycloak Role Bridge documentation with the implemented behavior (owner and admin both receive `openshell-admin` and `openshell-user`).

### Role vocabulary

The management plane distinguishes three per-gateway roles. The gateway (and therefore the console and the `openshell` CLI) distinguishes only two tiers, **Admin** and **User**, which correspond to the Keycloak client roles `openshell-admin` and `openshell-user`.

| Management-plane role | Who holds it | Console label | Keycloak client roles |
| --- | --- | --- | --- |
| `gateway:owner` | The creator; exactly one per gateway; immutable | Admin | `openshell-admin`, `openshell-user` |
| `gateway:admin` | A granted administrator; zero or more; removable | Admin | `openshell-admin`, `openshell-user` |
| `gateway:viewer` | A granted standard user; zero or more; removable | User | `openshell-user` |

`gateway:owner` and `gateway:admin` are indistinguishable to the gateway: both are gateway administrators. The distinction exists only in the management plane, to protect the creator (see GAM-07) and to keep ownership singular.

## Non-Goals

- Identity-provider **group** grants. Access is granted to individual users only. Group-based access remains unresolved (`web-console/user_flows.md`).
- **Ownership transfer.** The `gateway:owner` binding is immutable for the life of the gateway; reassigning the creator is out of scope.
- Changing how `platform:admin`, `gateway:creator`, or Keycloak realm roles are assigned. Those remain Keycloak-sourced (`security/rbac-enforcement.spec.md`).
- Using Keycloak Organizations (the Keycloak feature). "Keycloak organization" in requirements means the configured realm's user directory; tenancy stays RBAC-based.

## Requirements

### Requirement: GAM-01 -- Granted Administrator Role

The platform SHALL define a per-gateway built-in role `gateway:admin` representing a granted gateway administrator.

`gateway:admin` SHALL confer, on its bound gateway, the same capabilities as `gateway:owner` EXCEPT deleting the gateway:

- Read and update the gateway (including rename).
- Manage access: grant, change, and revoke `gateway:admin` and `gateway:viewer` bindings (GAM-08).
- Create and manage all OpenShellGatewayServiceAccounts on the gateway, selecting `openshell-user` or `openshell-admin` (GAM-12).

`gateway:admin` SHALL NOT authorize deleting the gateway. Deleting a gateway SHALL require `gateway:owner` or `platform:admin`.

`gateway:admin` SHALL be stored and granted exactly like other per-gateway bindings: a `RoleBinding` with `scope=gateway` and a `gateway_id`. It SHALL NOT be a JWT-synced role and SHALL NOT be assignable via Keycloak realm roles.

A user holding `gateway:admin` on a gateway SHALL see that gateway in list and get results (gateway visibility follows any per-gateway binding).

#### Scenario: Granted admin can manage the gateway but not delete it

- GIVEN user B has `gateway:admin` on gw-1
- WHEN user B calls `PATCH /api/hypershell/v1/gateways/gw-1`
- THEN the request SHALL be authorized
- WHEN user B calls `DELETE /api/hypershell/v1/gateways/gw-1`
- THEN the response SHALL be `403 Forbidden`

#### Scenario: Granted admin sees the gateway

- GIVEN user B has `gateway:admin` on gw-1 and no other bindings
- WHEN user B calls `GET /api/hypershell/v1/gateways`
- THEN the response SHALL include gw-1

---

### Requirement: GAM-02 -- Keycloak Role Bridge Mapping

The control plane SHALL map per-gateway HyperShell roles to per-gateway Keycloak client roles as follows:

| HyperShell role | Keycloak client roles |
| --- | --- |
| `gateway:owner` | `openshell-admin`, `openshell-user` |
| `gateway:admin` | `openshell-admin`, `openshell-user` |
| `gateway:viewer` | `openshell-user` |

A gateway administrator (owner or admin) SHALL therefore hold **both** `openshell-admin` and `openshell-user` on that gateway's Keycloak client; a standard user SHALL hold only `openshell-user`. This supersedes any prior "highest-privilege single role" description.

On every RoleBinding event (created, updated, deleted) for a gateway-scoped binding, the control plane SHALL reconcile the bound user's Keycloak client roles on that gateway to the **exact union** of client roles implied by the user's current surviving bindings on that gateway: it SHALL assign any missing client role and remove any client role no longer backed by a surviving binding. This makes role changes and demotions converge correctly and makes repeated events idempotent.

#### Scenario: Admin grant assigns both client roles

- GIVEN gw-1 has a provisioned Keycloak client
- WHEN a `gateway:admin` binding for user B on gw-1 is created
- THEN the control plane SHALL assign `openshell-admin` and `openshell-user` to user B on gw-1's Keycloak client

#### Scenario: Demotion strips the admin client role

- GIVEN user B holds `gateway:admin` on gw-1 (Keycloak `openshell-admin` + `openshell-user`)
- WHEN user B's access is changed to `gateway:viewer`
- THEN the control plane SHALL remove `openshell-admin` from user B on gw-1's Keycloak client
- AND `openshell-user` SHALL remain

#### Scenario: Overlapping bindings keep the union

- GIVEN user B holds both `gateway:viewer` and `gateway:admin` on gw-1
- WHEN the `gateway:viewer` binding is deleted
- THEN `openshell-user` SHALL remain assigned to user B on gw-1 (still backed by the `gateway:admin` binding)
- AND `openshell-admin` SHALL remain assigned

---

### Requirement: GAM-03 -- Gateway Access List

The API server SHALL expose `GET /api/hypershell/v1/gateways/{gateway_id}/access`, returning the list of access grants on the gateway as an enriched, paginated collection.

Each grant item SHALL expose:

| Field | Type | Notes |
| --- | --- | --- |
| `role_binding_id` | string | The backing RoleBinding id; used by change/revoke |
| `user_id` | string | HyperShell User id |
| `username` | string | `User.username` (= Keycloak `preferred_username`); the console "User ID" column |
| `name` | string | `User.name` display name, when present; the console "User name" column |
| `email` | string | Optional |
| `role` | string | `admin` or `user` (the console tier); `admin` covers both `gateway:owner` and `gateway:admin` |
| `is_creator` | boolean | `true` for the single `gateway:owner` grant |
| `granted_at` | date-time | Binding creation time |

The response SHALL be assembled server-side with the user fields already joined; it SHALL NOT require the caller to perform one lookup per row. The endpoint SHALL support `search` (case-insensitive substring over `username` and `name`), a `role` filter (`admin` or `user`), pagination, and ordering, following the shared list contract. Search terms SHALL be treated as literals (wildcard/escape semantics handled server-side).

Any caller with a binding on the gateway (owner, admin, or viewer) or `platform:admin` SHALL be authorized to read the access list. Callers with no access to the gateway SHALL receive `404` (existence not disclosed, per `security/rbac-enforcement.spec.md`).

#### Scenario: Owner lists access

- GIVEN gw-1 has a creator (owner), one `gateway:admin`, and two `gateway:viewer` grants
- WHEN the owner calls `GET /api/hypershell/v1/gateways/gw-1/access`
- THEN the response SHALL contain four items with `username`, `name`, `role`, and `is_creator` populated
- AND exactly one item SHALL have `is_creator: true` and `role: admin`

#### Scenario: Filter by role and search by name

- WHEN a caller requests the access list with `role=user` and `search=ali`
- THEN only `user`-tier grants whose username or name contains `ali` SHALL be returned

#### Scenario: Non-member cannot read the access list

- GIVEN user C has no binding on gw-1 and is not `platform:admin`
- WHEN user C calls `GET /api/hypershell/v1/gateways/gw-1/access`
- THEN the response SHALL be `404`

---

### Requirement: GAM-04 -- Grant Access

The API server SHALL expose `POST /api/hypershell/v1/gateways/{gateway_id}/access` to grant a user `admin` or `user` access to the gateway.

The request SHALL identify the target user by a directory identity (`username`, or an equivalent Keycloak subject reference returned by GAM-09) and a `role` of `admin` or `user`. Granting `admin` SHALL create a `gateway:admin` binding; granting `user` SHALL create a `gateway:viewer` binding. The endpoint SHALL NOT create `gateway:owner` bindings (GAM-07).

When the target identity has no existing HyperShell `User` record, the API server SHALL **pre-provision** one from the Keycloak directory record (upsert keyed on `username`, populating `username`, `email`, and `name`) before creating the binding, so the binding references a real `user_id` and the Role Bridge can resolve the Keycloak user by username. Pre-provisioning SHALL use the same upsert semantics as JWT auto-provisioning (`security/rbac-enforcement.spec.md`) and SHALL NOT expand the public users API, which remains read-only (`registered-users.spec.md`).

Granting SHALL be idempotent with respect to the effective role (GAM-10). On success the endpoint SHALL return the resulting grant item (GAM-03 shape).

#### Scenario: Grant admin to a previously registered user

- GIVEN user B has a HyperShell User record and no binding on gw-1
- WHEN an owner of gw-1 grants B the `admin` role
- THEN a `gateway:admin` binding SHALL be created for B on gw-1
- AND the Role Bridge SHALL assign `openshell-admin` and `openshell-user` to B on gw-1

#### Scenario: Grant access to a user who has never signed in

- GIVEN directory user `dana` exists in the Keycloak realm with no HyperShell User record
- WHEN an owner of gw-1 grants `dana` the `user` role
- THEN the API server SHALL pre-provision a User record for `dana` from the directory
- AND SHALL create a `gateway:viewer` binding for `dana` on gw-1
- AND the Role Bridge SHALL assign `openshell-user` to `dana` on gw-1

#### Scenario: Cannot grant the owner role

- WHEN a caller posts an access grant with a role that is not `admin` or `user` (for example attempting to grant owner)
- THEN the request SHALL be rejected with `400`

---

### Requirement: GAM-05 -- Change Role

The API server SHALL expose `PATCH /api/hypershell/v1/gateways/{gateway_id}/access/{user_id}` to change a user's access between `admin` and `user`.

The change SHALL be atomic: the user SHALL NOT be left with zero gateway access or with conflicting bindings at any observable point. The server SHALL converge the user's gateway bindings to exactly the requested tier (a single `gateway:admin` or `gateway:viewer` binding) within one transaction, emitting watch events such that the Role Bridge reconciles to the correct Keycloak client-role union (GAM-02).

Changing the creator's role SHALL be rejected (GAM-07). Changing to a role the user already holds SHALL be a no-op success (GAM-10).

#### Scenario: Promote a user to admin

- GIVEN user B holds `gateway:viewer` on gw-1
- WHEN an admin of gw-1 PATCHes B's access to `admin`
- THEN B SHALL hold exactly `gateway:admin` on gw-1 afterward
- AND the Role Bridge SHALL add `openshell-admin` for B on gw-1

#### Scenario: Role change never drops access mid-flight

- GIVEN user B holds `gateway:admin` on gw-1
- WHEN B's access is changed to `user`
- THEN at no observable point SHALL B have zero bindings on gw-1
- AND B SHALL end with exactly `gateway:viewer`

---

### Requirement: GAM-06 -- Revoke Access

The API server SHALL expose `DELETE /api/hypershell/v1/gateways/{gateway_id}/access/{user_id}` to revoke a user's access to the gateway.

Revocation SHALL remove the user's `gateway:admin` and/or `gateway:viewer` bindings on the gateway and SHALL cause the Role Bridge to remove the corresponding Keycloak client roles for that user on that gateway's client. Revoking the creator SHALL be rejected (GAM-07).

#### Scenario: Revoke a granted admin

- GIVEN user B holds `gateway:admin` on gw-1
- WHEN an owner of gw-1 revokes B's access
- THEN B SHALL hold no binding on gw-1
- AND the Role Bridge SHALL remove `openshell-admin` and `openshell-user` for B on gw-1's Keycloak client

#### Scenario: A granted admin may revoke their own access

- GIVEN user B holds `gateway:admin` on gw-1 and is not the creator
- WHEN user B revokes their own access
- THEN the request SHALL succeed and B SHALL lose access to gw-1

---

### Requirement: GAM-07 -- Creator Protection

The `gateway:owner` binding (the creator, auto-created at gateway creation per `security/rbac-enforcement.spec.md`) SHALL be immutable for the life of the gateway.

The access facade SHALL:

- Reject creating a second `gateway:owner` binding (`400`); the console grants `admin` as `gateway:admin`, never `gateway:owner`.
- Reject changing the creator's role (`PATCH` targeting the creator) with `409 Conflict`.
- Reject revoking the creator (`DELETE` targeting the creator) with `409 Conflict`.

These rejections SHALL apply regardless of the caller, including the creator acting on themselves and any other administrator. As a consequence, a creator cannot remove or demote themselves, and a gateway always retains exactly one owner until it is deleted.

#### Scenario: Creator cannot demote themselves

- GIVEN user A is the creator (owner) of gw-1
- WHEN user A PATCHes their own access to `user`
- THEN the response SHALL be `409 Conflict`
- AND user A SHALL remain `gateway:owner`

#### Scenario: Creator cannot be removed by another admin

- GIVEN user A is the creator of gw-1 and user B has `gateway:admin` on gw-1
- WHEN user B revokes user A's access
- THEN the response SHALL be `409 Conflict`
- AND user A SHALL remain `gateway:owner`

---

### Requirement: GAM-08 -- Access Management Authorization

Granting, changing, and revoking access (GAM-04, GAM-05, GAM-06) and searching the directory (GAM-09) SHALL require `gateway:owner` or `gateway:admin` on the target gateway.

`gateway:viewer` callers SHALL NOT manage access or search the directory. `platform:admin` SHALL NOT grant, change, or revoke access unless it also holds `gateway:owner` or `gateway:admin` on that gateway (consistent with `platform:admin` being view/delete-only for RBAC grants in `security/rbac-enforcement.spec.md`). Unauthorized management attempts SHALL return `403`; callers with no access to the gateway SHALL receive `404`.

#### Scenario: Viewer cannot grant access

- GIVEN user C holds `gateway:viewer` on gw-1
- WHEN user C posts an access grant on gw-1
- THEN the response SHALL be `403`

#### Scenario: Granted admin can grant access

- GIVEN user B holds `gateway:admin` on gw-1
- WHEN user B grants user D the `user` role on gw-1
- THEN the grant SHALL be created

---

### Requirement: GAM-09 -- Keycloak Directory Search

The API server SHALL expose `GET /api/hypershell/v1/gateways/{gateway_id}/access/directory?search=` returning candidate users from the configured Keycloak realm for the "Add users" picker.

Each candidate SHALL expose `username`, `name`, and `email`, and SHALL be usable as the target identity of a grant (GAM-04). Candidates SHALL include realm users who have never signed in to HyperShell (the search is against the Keycloak directory, not the HyperShell users inventory).

Because the API server SHALL NOT read the `hypershell-keycloak-admin` Secret (`openshell-gateway-keycloak.spec.md`), the API server SHALL obtain directory results from the control plane over the existing in-cluster gRPC path used for Keycloak-backed provisioning. The control plane SHALL query the Keycloak Admin REST API realm user search and return candidates. Results SHALL be bounded (paginated/capped) and the search SHALL require a non-trivial query term to avoid enumerating the entire realm in one call.

Authorization SHALL follow GAM-08 (owner or admin on the gateway).

#### Scenario: Directory search returns realm users

- GIVEN the realm contains users `dana` and `dale`, neither registered in HyperShell
- WHEN an admin of gw-1 calls `GET /api/hypershell/v1/gateways/gw-1/access/directory?search=da`
- THEN the response SHALL include candidates for `dana` and `dale` with `username` and `name`

#### Scenario: Directory search requires access management authorization

- GIVEN user C holds only `gateway:viewer` on gw-1
- WHEN user C calls the directory search for gw-1
- THEN the response SHALL be `403`

---

### Requirement: GAM-10 -- One Effective Role Per User Per Gateway

A user SHALL have at most one effective access tier on a gateway. The access facade SHALL NOT create duplicate or conflicting bindings for the same user on the same gateway.

Granting a role the user already holds SHALL be an idempotent no-op success. Granting a different tier than the user currently holds SHALL converge to the new tier (equivalent to a change, GAM-05), not accumulate bindings. The control plane Role Bridge SHALL remain correct under any surviving-binding union regardless (GAM-02).

#### Scenario: Re-granting the same role is a no-op

- GIVEN user B holds `gateway:viewer` on gw-1
- WHEN an admin grants B the `user` role again
- THEN the request SHALL succeed
- AND B SHALL still hold exactly one `gateway:viewer` binding on gw-1

#### Scenario: Granting a higher tier converges, not accumulates

- GIVEN user B holds `gateway:viewer` on gw-1
- WHEN an admin grants B the `admin` role
- THEN B SHALL hold exactly `gateway:admin` on gw-1 and no `gateway:viewer` binding

---

### Requirement: GAM-11 -- Service Account Role Capping for Admins

OpenShellGatewayServiceAccount creation (`openshell-gateway-service-accounts.spec.md`) caps the selectable OpenShell role at the creator's gateway tier. A `gateway:admin` caller SHALL be capped identically to `gateway:owner`: it MAY select `openshell-user` or `openshell-admin`. A `gateway:viewer` caller MAY select only `openshell-user`.

#### Scenario: Granted admin creates an admin service account

- GIVEN user B holds `gateway:admin` on gw-1
- WHEN user B creates an OpenShellGatewayServiceAccount on gw-1 with role `openshell-admin`
- THEN the request SHALL be authorized

---

### Requirement: GAM-12 -- Role Migration

Introducing `gateway:admin` SHALL be an additive, idempotent migration. The `gateway:admin` role SHALL be seeded with its permission set (gateway read/update, role_bindings create/read/delete/list, service_accounts manage) alongside the existing built-in roles. Existing `gateway:owner`, `gateway:viewer`, `gateway:creator`, and `platform:admin` bindings SHALL be unaffected, and re-running the migration SHALL be safe.

No historical `gateway:owner` binding SHALL be rewritten to `gateway:admin`; existing gateways keep their single owner, and additional administrators are added going forward as `gateway:admin`.

#### Scenario: Migration seeds the role idempotently

- GIVEN a database already migrated with the existing built-in roles
- WHEN the role seeding migration runs (including on a second run)
- THEN a single `gateway:admin` role SHALL exist with its permissions
- AND no existing binding SHALL be modified

---

### Requirement: GAM-13 -- hsctl CLI Parity

The `hsctl` CLI mirrors the REST API one-for-one (`data-model.spec.md`), as it already does for OpenShellGatewayServiceAccounts. The gateway access operations SHALL therefore have corresponding commands, following the gateway-nested `--gateway-id` pattern established by service accounts:

| REST operation | `hsctl` command |
| --- | --- |
| GAM-03 list access | `hsctl list gatewayAccess --gateway-id <id> [--role <admin\|user>] [--search <q>]` |
| GAM-04 grant access | `hsctl create gatewayAccess --gateway-id <id> --user <username> --role <admin\|user>` |
| GAM-05 change role | `hsctl update gatewayAccess <user_id> --gateway-id <id> --role <admin\|user>` |
| GAM-06 revoke access | `hsctl delete gatewayAccess <user_id> --gateway-id <id>` |

The directory search (GAM-09) is a deliberate exception to the CLI mirror: it serves the web console "Add users" picker and SHALL NOT have an `hsctl` command. CLI callers already supply the username to `create gatewayAccess --user <username>`.

`--role` SHALL accept `admin` or `user` (mapping to `gateway:admin` and `gateway:viewer`). The commands SHALL use the generated SDK and SHALL surface the same authorization and creator-protection outcomes as the REST API -- a `403` (unauthorized management) or `409` (creator mutation) SHALL produce a non-zero exit with a clear message rather than a silent success. These commands manage access only; they SHALL NOT create or modify the creator's `gateway:owner` binding (GAM-07).

#### Scenario: Grant then revoke access via hsctl

- GIVEN the caller administers gw-1
- WHEN the caller runs `hsctl create gatewayAccess --gateway-id gw-1 --user dana --role user`
- THEN dana SHALL gain user access to gw-1
- WHEN the caller runs `hsctl delete gatewayAccess <dana-user-id> --gateway-id gw-1`
- THEN dana's access SHALL be revoked

#### Scenario: CLI surfaces creator protection

- GIVEN user A is the creator of gw-1
- WHEN the caller runs `hsctl delete gatewayAccess <A-user-id> --gateway-id gw-1`
- THEN the command SHALL exit non-zero with a message that the gateway creator cannot be removed (`409`)

---

### Requirement: GAM-14 -- Verification

The API server SHALL include integration tests covering: enriched access list with search and role filter; grant to a registered user and to a never-signed-in directory user (pre-provisioning); atomic role change (promote and demote) with no zero-access window; revoke; creator-protection rejections (`409`) for demote and revoke, including by another admin; management authorization (viewer `403`, admin allowed, non-member `404`); directory-search authorization; and idempotent re-grant.

The control plane SHALL include tests covering the `gateway:admin` bridge mapping and reconcile-to-union on create, update, and delete (including demotion stripping `openshell-admin` while retaining `openshell-user`).

The `hsctl` CLI SHALL include tests covering the access commands (GAM-13), including a creator-protection failure surfacing as a non-zero exit.

`make generate` SHALL regenerate the Go and TypeScript SDK clients for the new endpoints.

#### Scenario: CI exercises creator protection and demotion

- GIVEN the integration and control-plane test suites run
- WHEN access-management and Role Bridge tests execute
- THEN they SHALL assert `409` on creator demote/revoke
- AND assert that demoting an admin removes `openshell-admin` while keeping `openshell-user`
