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

1. Adds a third per-gateway role, `gateway:admin`, forming the hierarchy **Owner > Admin > Viewer**. Owners may delete the gateway and assign other owners; admins manage the Admin and User tiers only.
2. Defines a gateway-scoped **access facade** (`/api/hypershell/v1/gateways/{gateway_id}/access`) that presents enriched access grants, atomic role changes, and last-owner protection in application language, over the existing RoleBinding storage and watch events.
3. Defines a **Keycloak directory search** so administrators can grant access to any user in the Keycloak realm, including users who have never signed in to HyperShell, pre-provisioning a HyperShell `User` record on grant.
4. Aligns the Keycloak Role Bridge documentation with the implemented behavior (owner and admin both receive `openshell-admin` and `openshell-user`).

### Role vocabulary

The management plane distinguishes three per-gateway roles. The gateway (and therefore the console and the `openshell` CLI) distinguishes only two tiers, **Admin** and **User**, which correspond to the Keycloak client roles `openshell-admin` and `openshell-user`.

The three per-gateway roles form a strict hierarchy, **Owner > Admin > Viewer**:

| Management-plane role | Who holds it | Console label | Capabilities beyond the lower tier | Keycloak client roles |
| --- | --- | --- | --- | --- |
| `gateway:owner` | One or more; grantable; the creator is the first owner | Owner | Delete the gateway; grant/change/revoke any role including `gateway:owner` | `openshell-admin`, `openshell-user` |
| `gateway:admin` | Zero or more; grantable; removable | Admin | Read/update the gateway; grant/change/revoke `gateway:admin` and `gateway:viewer` | `openshell-admin`, `openshell-user` |
| `gateway:viewer` | Zero or more; grantable; removable | User | Read-only | `openshell-user` |

`gateway:owner` and `gateway:admin` are indistinguishable **to the gateway**: both are gateway administrators and receive the same Keycloak client roles. Their difference is entirely a management-plane concern -- only owners may delete the gateway and assign other owners. A gateway always retains at least one owner (GAM-07).

The creator (the auto-provisioned first owner, `security/rbac-enforcement.spec.md`) is tracked for display and audit (`is_creator`, "Created by") but holds no capability beyond any other owner; ownership is shared and transferable among owners.

## Non-Goals

- Identity-provider **group** grants. Access is granted to individual users only. Group-based access remains unresolved (`web-console/user_flows.md`).
- Changing how `platform:admin`, `gateway:creator`, or Keycloak realm roles are assigned. Those remain Keycloak-sourced (`security/rbac-enforcement.spec.md`).
- Using Keycloak Organizations (the Keycloak feature). "Keycloak organization" in requirements means the configured realm's user directory; tenancy stays RBAC-based.

## Requirements

### Requirement: GAM-01 -- Granted Administrator Role

The platform SHALL define a per-gateway built-in role `gateway:admin` representing a granted gateway administrator, below `gateway:owner` in the hierarchy.

`gateway:admin` SHALL confer, on its bound gateway:

- Read and update the gateway (including rename).
- Manage access **for the Admin and User tiers only**: grant, change, and revoke `gateway:admin` and `gateway:viewer` bindings (GAM-08).
- Create and manage all OpenShellGatewayServiceAccounts on the gateway, selecting `openshell-user` or `openshell-admin` (GAM-12).

`gateway:admin` SHALL NOT delete the gateway, and SHALL NOT grant, change, or revoke the Owner tier (`gateway:owner`). Deleting a gateway SHALL require `gateway:owner` or `platform:admin`; assigning an owner SHALL require `gateway:owner` (GAM-08).

The Gateway REST resource SHALL advertise a read-only, per-caller `can_delete` boolean on `GET` (list and single) responses, computed from the authenticated caller's current bindings with the **same check that authorizes `DELETE`** (`gateway:owner` on that gateway, or `platform:admin`). `can_delete` is advisory: it exists so clients (console, CLI) can disable a delete affordance the API would reject, and SHALL NOT weaken enforcement. The API server SHALL remain the authorization boundary and SHALL enforce delete authorization on `DELETE` regardless of `can_delete`. When RBAC enforcement is disabled, every delete is permitted, so `can_delete` SHALL be `true`.

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

#### Scenario: can_delete reflects the caller's delete authorization

- GIVEN user A demoted their own access on gw-1 from `gateway:owner` to the User tier (`gateway:viewer`), and user C remains an owner
- WHEN user A calls `GET /api/hypershell/v1/gateways/gw-1`
- THEN the response `can_delete` SHALL be `false`
- AND a `DELETE /api/hypershell/v1/gateways/gw-1` by user A SHALL return `403`
- WHEN user C (a `gateway:owner`) or a `platform:admin` calls `GET /api/hypershell/v1/gateways/gw-1`
- THEN the response `can_delete` SHALL be `true`

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
| `role_binding_id` | string | The backing RoleBinding id; informational only. Change (GAM-05) and revoke (GAM-06) are keyed by `user_id` in the URL and operate on all of that user's bindings on the gateway, not by `role_binding_id` |
| `user_id` | string | HyperShell User id |
| `username` | string | `User.username` (= Keycloak `preferred_username`); the console "User ID" column |
| `name` | string | `User.name` display name, when present; the console "User name" column |
| `email` | string | Optional |
| `role` | string | `owner`, `admin`, or `user` (the console tier) mapping to `gateway:owner`, `gateway:admin`, `gateway:viewer` |
| `is_creator` | boolean | `true` for the owner who created the gateway (informational only; see GAM-07) |
| `granted_at` | date-time | Binding creation time |

The response SHALL be assembled server-side with the user fields already joined; it SHALL NOT require the caller to perform one lookup per row. The endpoint SHALL support `search` (case-insensitive substring over `username` and `name`), a `role` filter (`owner`, `admin`, or `user`), pagination, and ordering, following the shared list contract. Search terms SHALL be treated as literals (wildcard/escape semantics handled server-side).

Any caller with a binding on the gateway (owner, admin, or viewer) or `platform:admin` SHALL be authorized to read the access list. Callers with no access to the gateway SHALL receive `404` (existence not disclosed, per `security/rbac-enforcement.spec.md`).

#### Scenario: Owner lists access

- GIVEN gw-1 has a creator (owner), one additional `gateway:owner`, one `gateway:admin`, and two `gateway:viewer` grants
- WHEN an owner calls `GET /api/hypershell/v1/gateways/gw-1/access`
- THEN the response SHALL contain five items with `username`, `name`, `role`, and `is_creator` populated
- AND exactly one item SHALL have `is_creator: true` and `role: owner`

#### Scenario: Filter by role and search by name

- WHEN a caller requests the access list with `role=user` and `search=ali`
- THEN only `user`-tier grants whose username or name contains `ali` SHALL be returned

#### Scenario: Non-member cannot read the access list

- GIVEN user C has no binding on gw-1 and is not `platform:admin`
- WHEN user C calls `GET /api/hypershell/v1/gateways/gw-1/access`
- THEN the response SHALL be `404`

---

### Requirement: GAM-04 -- Grant Access

The API server SHALL expose `POST /api/hypershell/v1/gateways/{gateway_id}/access` to grant a user `owner`, `admin`, or `user` access to the gateway.

The request SHALL identify the target user by a directory identity (`username`, or an equivalent Keycloak subject reference returned by GAM-09) and a `role` of `owner`, `admin`, or `user`. Granting `owner` SHALL create a `gateway:owner` binding; granting `admin` SHALL create a `gateway:admin` binding; granting `user` SHALL create a `gateway:viewer` binding. Granting (or changing to) the `owner` tier SHALL require the caller to be a `gateway:owner` (GAM-08); `gateway:admin` callers SHALL NOT assign owners.

The target identity SHALL be resolved against the Keycloak realm directory before any binding is created. If the identity does not correspond to a user in the realm, the request SHALL be rejected with `404 Not Found` and a clear error (the person must exist in the realm to be granted access); no `User` record SHALL be pre-provisioned and no binding SHALL be created. Resolution SHALL reuse the directory projection / lookup of GAM-09.

When the target identity resolves to a realm user that has no existing HyperShell `User` record, the API server SHALL **pre-provision** one from the Keycloak directory record (upsert keyed on `username`, populating `username`, `email`, and `name`) before creating the binding, so the binding references a real `user_id` and the Role Bridge can resolve the Keycloak user by username. Pre-provisioning SHALL use the same upsert semantics as JWT auto-provisioning (`security/rbac-enforcement.spec.md`) and SHALL NOT expand the public users API, which remains read-only (`registered-users.spec.md`).

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

#### Scenario: Owner assigns another owner

- GIVEN user A is an owner of gw-1
- WHEN user A grants user E the `owner` role
- THEN a `gateway:owner` binding SHALL be created for E on gw-1
- AND E SHALL be able to delete gw-1 and assign further owners

#### Scenario: Admin cannot assign an owner

- GIVEN user B holds `gateway:admin` on gw-1 (not an owner)
- WHEN user B grants user E the `owner` role
- THEN the request SHALL be rejected with `403`

#### Scenario: Unknown role is rejected

- WHEN a caller posts an access grant with a role that is not `owner`, `admin`, or `user`
- THEN the request SHALL be rejected with `400`

#### Scenario: Grant to a user not in the realm is rejected

- GIVEN identity `ghost` does not exist in the Keycloak realm
- WHEN an owner of gw-1 grants `ghost` any role
- THEN the request SHALL be rejected with `404` and a clear error
- AND no `User` record SHALL be pre-provisioned and no binding SHALL be created for `ghost`

---

### Requirement: GAM-05 -- Change Role

The API server SHALL expose `PATCH /api/hypershell/v1/gateways/{gateway_id}/access/{user_id}` to change a user's access among `owner`, `admin`, and `user`.

The change SHALL be atomic: the user SHALL NOT be left with zero gateway access or with conflicting bindings at any observable point. The server SHALL converge the user's gateway bindings to exactly the requested tier (a single `gateway:owner`, `gateway:admin`, or `gateway:viewer` binding) within one transaction.

**Mechanism.** When the user holds a single binding on the gateway (the normal case under GAM-10), the change SHALL be implemented by updating that `RoleBinding`'s `role_id` **in place**, emitting one `RoleBinding` `UPDATED` watch event -- not delete-then-create -- so there is no window in which the user has no binding. The generic `role_bindings` REST resource remains create/delete-only publicly (`data-model.spec.md`); the in-place role change is an internal mutation performed by the access facade / service layer, and the control plane's Role Bridge handles the `UPDATED` event by reconciling to the client-role union (GAM-02, `openshell-gateway-keycloak.spec.md`). The reconcile is union-based and reads live bindings, so it remains correct even if an implementation instead converges by adjusting individual bindings.

Promoting a user to `owner`, or demoting a user from `owner`, SHALL require the caller to be a `gateway:owner` (GAM-08). Demoting an owner SHALL be rejected if that owner is the last remaining owner (GAM-07). Changing to a role the user already holds SHALL be a no-op success (GAM-10).

#### Scenario: Promote a user to admin

- GIVEN user B holds `gateway:viewer` on gw-1
- WHEN an admin of gw-1 PATCHes B's access to `admin`
- THEN B SHALL hold exactly `gateway:admin` on gw-1 afterward
- AND the Role Bridge SHALL add `openshell-admin` for B on gw-1

#### Scenario: Owner promotes an admin to owner

- GIVEN user B holds `gateway:admin` on gw-1 and user A is an owner
- WHEN user A PATCHes B's access to `owner`
- THEN B SHALL hold exactly `gateway:owner` on gw-1 afterward

#### Scenario: Admin cannot promote to owner

- GIVEN user B holds `gateway:admin` on gw-1 (not an owner) and user C holds `gateway:viewer`
- WHEN user B PATCHes C's access to `owner`
- THEN the response SHALL be `403`

#### Scenario: Role change never drops access mid-flight

- GIVEN user B holds `gateway:admin` on gw-1
- WHEN B's access is changed to `user`
- THEN at no observable point SHALL B have zero bindings on gw-1
- AND B SHALL end with exactly `gateway:viewer`

---

### Requirement: GAM-06 -- Revoke Access

The API server SHALL expose `DELETE /api/hypershell/v1/gateways/{gateway_id}/access/{user_id}` to revoke a user's access to the gateway.

Revocation SHALL remove the user's `gateway:owner`, `gateway:admin`, and/or `gateway:viewer` bindings on the gateway and SHALL cause the Role Bridge to remove the corresponding Keycloak client roles for that user on that gateway's client. Revoking a user who holds `gateway:owner` SHALL require the caller to be a `gateway:owner` (GAM-08), and SHALL be rejected if that user is the last remaining owner (GAM-07).

#### Scenario: Revoke a granted admin

- GIVEN user B holds `gateway:admin` on gw-1
- WHEN an owner or admin of gw-1 revokes B's access
- THEN B SHALL hold no binding on gw-1
- AND the Role Bridge SHALL remove `openshell-admin` and `openshell-user` for B on gw-1's Keycloak client

#### Scenario: An owner may revoke their own access when another owner remains

- GIVEN users A and E both hold `gateway:owner` on gw-1
- WHEN user A revokes their own access
- THEN the request SHALL succeed and A SHALL lose access to gw-1
- AND E SHALL remain an owner

#### Scenario: Admin cannot revoke an owner

- GIVEN user B holds `gateway:admin` on gw-1 and user A holds `gateway:owner`
- WHEN user B revokes user A's access
- THEN the response SHALL be `403`

---

### Requirement: GAM-07 -- Last-Owner Protection

A gateway SHALL always have at least one `gateway:owner`. The access facade SHALL reject any operation that would remove the last remaining owner, so a gateway can never be orphaned (left with no owner who can delete it or manage ownership):

- Revoking (`DELETE`) a user who is the only remaining owner SHALL be rejected with `409 Conflict`.
- Demoting (`PATCH` to `admin` or `user`) a user who is the only remaining owner SHALL be rejected with `409 Conflict`.

Each `409` response SHALL carry a detailed, human-readable error message explaining the constraint -- that a gateway must retain at least one owner and that another owner must be assigned before this one can be demoted or removed -- so the caller (CLI or console) can surface actionable guidance rather than a bare status code.

These rejections SHALL apply regardless of the caller, including an owner acting on themselves. When more than one owner exists, any owner (including the creator) MAY be demoted or revoked by an owner. Ownership is therefore shared and transferable: the creator is not uniquely protected once another owner exists; it is the **last owner** that cannot be removed.

The creator (the auto-provisioned first owner) is retained as `is_creator` for display and audit only and SHALL NOT, by itself, confer immutability.

#### Scenario: Sole owner cannot demote themselves

- GIVEN user A is the only owner of gw-1
- WHEN user A PATCHes their own access to `user`
- THEN the response SHALL be `409 Conflict`
- AND user A SHALL remain `gateway:owner`

#### Scenario: Sole owner cannot be removed

- GIVEN user A is the only owner of gw-1
- WHEN any caller revokes user A's access
- THEN the response SHALL be `409 Conflict`
- AND user A SHALL remain `gateway:owner`

#### Scenario: An owner may be removed when another owner remains

- GIVEN users A and E both hold `gateway:owner` on gw-1
- WHEN an owner demotes A to `user`
- THEN the request SHALL succeed and A SHALL hold `gateway:viewer`
- AND E SHALL remain the owner

---

### Requirement: GAM-08 -- Access Management Authorization

Granting, changing, and revoking access (GAM-04, GAM-05, GAM-06) and searching the directory (GAM-09) SHALL require `gateway:owner` or `gateway:admin` on the target gateway. Operations that touch the **Owner tier** -- granting `owner`, promoting to `owner`, or demoting/revoking a user who holds `gateway:owner` -- SHALL additionally require the caller to be a `gateway:owner`. A `gateway:admin` caller MAY manage only the Admin and User tiers.

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

Each candidate SHALL expose `username`, `name`, and `email`, and SHALL be usable as the target identity of a grant (GAM-04). Candidates SHALL include realm users who have never signed in to HyperShell (the directory is the Keycloak realm, not the HyperShell users inventory).

**Backing store.** Rather than issuing a live Keycloak Admin REST query on every keystroke, the directory SHALL be served from a **control-plane-maintained projection of realm users** that the control plane refreshes periodically (and MAY refresh on demand). The control plane -- which holds the `hypershell-keycloak-admin` Secret and already runs reconcile loops -- SHALL periodically list realm users via the Keycloak Admin REST API and persist a directory projection (`username`, `name`, `email`, Keycloak subject) that the API server reads and filters for this endpoint. The API server SHALL NOT read the `hypershell-keycloak-admin` Secret (`openshell-gateway-keycloak.spec.md`). This keeps search fast and resilient to transient Keycloak unavailability; the tradeoff is bounded staleness equal to the refresh interval. Because the projection may be stale, grant (GAM-04) SHALL re-validate the chosen identity against the realm at grant time and reject unknown users (`404`). The refresh interval SHALL be configuration, not code (`specs/standards/`), and newly added realm users become selectable within one refresh cycle. Results SHALL be bounded (paginated/capped); the search SHALL apply the query term server-side against the projection.

Authorization SHALL follow GAM-08 (owner or admin on the gateway).

#### Scenario: Directory search returns realm users from the projection

- GIVEN the realm contains users `dana` and `dale`, neither registered in HyperShell
- AND the control plane has refreshed the directory projection
- WHEN an admin of gw-1 calls `GET /api/hypershell/v1/gateways/gw-1/access/directory?search=da`
- THEN the response SHALL include candidates for `dana` and `dale` with `username` and `name`
- AND the API server SHALL serve them from the projection without a live Keycloak call

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

No historical `gateway:owner` binding SHALL be rewritten; existing gateways keep their creator as owner. Going forward, owners may add further owners, admins, or viewers, and admins may add admins or viewers.

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
| GAM-03 list access | `hsctl list gatewayAccess --gateway-id <id> [--role <owner\|admin\|user>] [--search <q>]` |
| GAM-04 grant access | `hsctl create gatewayAccess --gateway-id <id> --user <username> --role <owner\|admin\|user>` |
| GAM-05 change role | `hsctl update gatewayAccess <user_id> --gateway-id <id> --role <owner\|admin\|user>` |
| GAM-06 revoke access | `hsctl delete gatewayAccess <user_id> --gateway-id <id>` |

The directory search (GAM-09) is a deliberate exception to the CLI mirror: it serves the web console "Add users" picker and SHALL NOT have an `hsctl` command. CLI callers already supply the username to `create gatewayAccess --user <username>`.

`--role` SHALL accept `owner`, `admin`, or `user` (mapping to `gateway:owner`, `gateway:admin`, `gateway:viewer`). The commands SHALL use the generated SDK and SHALL surface the same authorization and protection outcomes as the REST API -- a `403` (unauthorized management, including a non-owner touching the Owner tier) or `409` (last-owner protection) SHALL produce a non-zero exit with a clear message rather than a silent success. These commands assign only the role given by `--role`; assigning or removing the Owner tier requires the caller to be an owner (GAM-08).

#### Scenario: Grant then revoke access via hsctl

- GIVEN the caller administers gw-1
- WHEN the caller runs `hsctl create gatewayAccess --gateway-id gw-1 --user dana --role user`
- THEN dana SHALL gain user access to gw-1
- WHEN the caller runs `hsctl delete gatewayAccess <dana-user-id> --gateway-id gw-1`
- THEN dana's access SHALL be revoked

#### Scenario: CLI surfaces last-owner protection

- GIVEN user A is the only owner of gw-1
- WHEN the caller runs `hsctl delete gatewayAccess <A-user-id> --gateway-id gw-1`
- THEN the command SHALL exit non-zero with a message that the last owner cannot be removed (`409`)

---

### Requirement: GAM-14 -- Verification

The API server SHALL include integration tests covering: enriched access list with search and role filter across all three tiers; grant (owner, admin, user) to a registered user and to a never-signed-in directory user (pre-provisioning); grant to an identity absent from the realm rejected with `404` and no pre-provision/binding; atomic role change (promote and demote, including to/from owner) implemented as an in-place `role_id` update (single `UPDATED` event) with no zero-access window; revoke; last-owner-protection rejections (`409`) for demoting or revoking the sole owner, asserting the response carries a descriptive message; owner-tier authorization (a non-owner admin assigning, demoting, or revoking an owner returns `403`); management authorization (viewer `403`, admin allowed for admin/user tiers, non-member `404`); directory-search authorization and serving from the control-plane projection; and idempotent re-grant.

The control plane SHALL include tests covering the `gateway:owner` and `gateway:admin` bridge mappings and reconcile-to-union on create, update, and delete (including demotion stripping `openshell-admin` while retaining `openshell-user`), and the periodic directory-projection refresh.

The `hsctl` CLI SHALL include tests covering the access commands (GAM-13), including a last-owner-protection failure surfacing as a non-zero exit.

`make generate` SHALL regenerate the Go and TypeScript SDK clients for the new endpoints.

#### Scenario: CI exercises last-owner protection and demotion

- GIVEN the integration and control-plane test suites run
- WHEN access-management and Role Bridge tests execute
- THEN they SHALL assert `409` on demoting or revoking the sole owner
- AND assert that a non-owner admin touching the Owner tier returns `403`
- AND assert that demoting an admin removes `openshell-admin` while keeping `openshell-user`
