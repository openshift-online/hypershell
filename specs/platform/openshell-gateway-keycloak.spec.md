# OpenShell Gateway Keycloak Provisioning Specification

**Date:** 2026-08-11
**Status:** Draft
**Parent:** `openshell-gateway.spec.md` - core gateway provisioning
**Related:** `openshell-gateway-oidc.spec.md` - OIDC configuration injection into gateway.toml; `security/security.spec.md` - secret management and isolation; `data-model.spec.md` - Gateway kind definition; `web-console/architecture.spec.md` - gateway visibility; `security/rbac-enforcement.spec.md` - scope-aware RBAC model, role hierarchy, and Gateway OIDC Role Bridge
**Upstream:** [Keycloak Admin REST API](https://www.keycloak.org/docs-api/latest/rest-api/)

---

## Purpose

This specification defines automated per-gateway Keycloak OIDC client provisioning. Keycloak integration has two distinct lifecycles:

1. **Client lifecycle** — When a gateway is created, the control plane provisions a dedicated OIDC client in Keycloak with client-scoped roles and protocol mappers, and populates the gateway's OIDC configuration. When a gateway is deleted, the control plane deletes the Keycloak client. This lifecycle is tied to Gateway ADDED/DELETED events.

2. **Role assignment lifecycle** — When a user's effective RBAC tier for a fleet changes (via RoleBinding creation or deletion), the control plane propagates that change to the Keycloak client roles on all gateways in that fleet. This implements the Gateway OIDC Role Bridge defined in [`rbac-enforcement.spec.md`](../security/rbac-enforcement.spec.md). The mapping is fleet-scoped: a `fleet:editor` binding on fleet-1 results in `openshell-admin` Keycloak client role assignments on every gateway in fleet-1.

This establishes per-gateway authentication isolation: each gateway has its own audience, roles, and token claims. Visibility and access control are RBAC concerns defined in the RBAC spec — this spec defines how RBAC decisions are projected into Keycloak role assignments so that users who connect to gateways directly via the `openshell` CLI receive the same access level as in the management plane.

---

## Architecture

### Provisioning Flow

```
Gateway Lifecycle (Gateway ADDED/DELETED events):

    Caller (UI, hsctl, CI pipeline, curl)
        |  POST /api/hypershell/v1/gateways
        |  Body includes: name, fleet_id (OIDC config is NOT part of the create request)
        v
    API Server
        |  1. Authorizes via RBAC (caller must have gateway create permission on the fleet)
        |  2. Persists Gateway with fleet_id (oidc field empty at this point)
        |  3. Emits gRPC watch event
        v
    Control Plane - GatewayReconciler
        |  1. Receives Gateway ADDED event
        |  2. Provisions Keycloak via Admin REST API:
        |     a. Creates OIDC client (clientId = gateway name)
        |     b. Creates client roles (openshell-admin, openshell-user)
        |     c. Creates protocol mappers (audience, sub, client-roles)
        |  3. Populates Gateway oidc config (PATCH via API or internal state)
        |  4. Resolves existing RoleBindings for the gateway's fleet and assigns
        |     Keycloak client roles to all users with fleet-level bindings
        |  5. Injects OIDC section into gateway.toml
        |  6. Deploys gateway K8s resources
        v
    Gateway Pod
        |  Validates JWTs against the provisioned Keycloak client
        v
    Authorized Access (admin/user roles scoped to this gateway only)


OIDC Role Bridge (RoleBinding ADDED/DELETED events):

    Caller (UI, hsctl, admin)
        |  POST /api/hypershell/v1/role_bindings
        |  Body: { role_id, scope, user_id, fleet_id }
        v
    API Server
        |  1. Authorizes (caller must have RBAC grant permission)
        |  2. Persists RoleBinding
        |  3. Emits gRPC watch event
        v
    Control Plane
        |  1. Receives RoleBinding ADDED/DELETED event
        |  2. Resolves the RBAC role to a Keycloak client role:
        |     - platform:admin, fleet:owner, fleet:editor → openshell-admin
        |     - platform:viewer, fleet:viewer, gateway:viewer → openshell-user
        |  3. Resolves the affected gateways:
        |     - scope=global  → all gateways
        |     - scope=fleet   → all gateways in that fleet
        |     - scope=gateway → that specific gateway
        |  4. For each affected gateway's Keycloak client:
        |     - ADDED:   assigns the Keycloak client role to the user
        |     - DELETED: removes the Keycloak client role from the user
        v
    User can now obtain tokens with the assigned role for affected gateways
```

### Keycloak Service Account

```
HyperShell Namespace
    |
    +-- Secret: hypershell-keycloak-admin
    |   +-- server-url:    https://keycloak.example.com
    |   +-- realm:         hypershell (configurable per environment)
    |   +-- client-id:     hypershell-admin-sa
    |   +-- client-secret: <service account secret>
    |
    +-- API Server Pod
    |   +-- Reads Secret (optional, for future user validation at RoleBinding creation)
    |
    +-- Control Plane Pod
        +-- Reads Secret -> provisions Keycloak clients, roles, mappers (read-write)
        +-- Reads Secret -> assigns/removes Keycloak client roles on RoleBinding events (read-write)
```

### Per-Gateway Isolation Model

```
Fleet "production" contains gw-alpha and gw-shared
Fleet "staging" contains gw-beta

user-a creates fleet "production" → auto fleet:owner binding
    KC clients: gw-alpha, gw-shared
    openshell-admin → user-a (on both, via fleet:owner)

fleet:owner user-a grants fleet:editor to user-b on "production"
    openshell-admin → user-b (on both gw-alpha and gw-shared)

user-c creates fleet "staging" → auto fleet:owner binding
    KC client: gw-beta
    openshell-admin → user-c

    user-a GET /gateways → [gw-alpha, gw-shared]   (fleet:owner on production)
    user-b GET /gateways → [gw-alpha, gw-shared]   (fleet:editor on production)
    user-c GET /gateways → [gw-beta]                (fleet:owner on staging)

platform:admin sees all gateways across all fleets
gateway:viewer on gw-alpha sees only gw-alpha
```

Each gateway's Keycloak client has `fullScopeAllowed = false`, preventing role leakage across gateways. A token obtained for `gw-alpha` contains only `gw-alpha`'s roles and audience — never `gw-beta`'s.

Gateway visibility is an RBAC concern — a user sees gateways in fleets where they have a RoleBinding. The RBAC spec defines scope-aware list filtering; this spec defines only the Keycloak projection that enables gateway-level token validation.

---

## RBAC Role Bridge

This specification implements the **Gateway OIDC Role Bridge** defined in [`rbac-enforcement.spec.md`](../security/rbac-enforcement.spec.md). The bridge ensures that users who connect to gateways directly via the `openshell` CLI receive the same access level as in the HyperShell management plane.

### Role Mapping

The control plane maps HyperShell RBAC roles to per-gateway Keycloak client roles:

| HyperShell Role | Keycloak Client Role | Scope |
|---|---|---|
| `platform:admin` | `openshell-admin` | All gateways across all fleets |
| `fleet:owner` | `openshell-admin` | All gateways in the bound fleet |
| `fleet:editor` | `openshell-admin` | All gateways in the bound fleet |
| `platform:viewer` | `openshell-user` | All gateways across all fleets |
| `fleet:viewer` | `openshell-user` | All gateways in the bound fleet |
| `gateway:viewer` | `openshell-user` | The specific bound gateway |

### Effective Role Resolution

When a user has multiple RoleBindings that cover the same gateway, the **highest-privilege** Keycloak client role wins. A user with both `fleet:viewer` (→ `openshell-user`) and `fleet:editor` (→ `openshell-admin`) on the same fleet SHALL have the `openshell-admin` client role on that fleet's gateways.

### Cascade Behavior

Role assignment cascades through the fleet hierarchy:

- **RoleBinding created/deleted for a fleet** → the control plane resolves all gateways in that fleet and assigns/removes the Keycloak client role on each gateway's Keycloak client.
- **Gateway created in a fleet** → the control plane resolves all RoleBindings for that fleet and assigns the appropriate Keycloak client roles to each user.
- **Gateway deleted** → Keycloak client deletion cascades all role assignments automatically (see Client Cleanup requirement).
- **Global-scoped RoleBinding** → the control plane resolves all gateways across all fleets.

---

## Requirements

### Requirement: Keycloak Service Account Access

Both the API server and the control plane SHALL authenticate to the Keycloak Admin REST API using a shared service account with admin permissions in the configured realm. The service account credentials SHALL be stored in a Kubernetes Secret named `hypershell-keycloak-admin` in the HyperShell namespace.

The Secret SHALL contain the following keys:

| Key | Description |
|---|---|
| `server-url` | Keycloak base URL (e.g., `https://keycloak.example.com`) |
| `realm` | Keycloak realm name (configurable per environment, e.g., `hypershell`, `hypershell-stage`) |
| `client-id` | Service account client ID with realm admin permissions |
| `client-secret` | Service account client secret |

Both components SHALL obtain an access token from Keycloak using the client credentials grant (`grant_type=client_credentials`) before making Admin REST API calls. Each component SHOULD cache and refresh the service account token independently.

- **Control plane** uses the Secret for read-write operations: provisioning Keycloak clients, roles, mappers during gateway reconciliation, and assigning/removing client roles during the OIDC Role Bridge.

#### Scenario: Service account credentials available

- GIVEN the `hypershell-keycloak-admin` Secret exists in the HyperShell namespace
- WHEN the control plane starts
- THEN it SHALL read the Secret and validate that all required keys are present
- AND it SHALL verify connectivity to the Keycloak Admin REST API at startup

#### Scenario: Service account credentials missing at control plane

- GIVEN the `hypershell-keycloak-admin` Secret does not exist
- WHEN the GatewayReconciler receives a Gateway event
- THEN it SHALL log an error indicating Keycloak integration is not configured
- AND it SHALL NOT deploy the gateway
- AND it SHALL retry on the next reconciliation cycle

---

### Requirement: Per-Gateway OIDC Client Provisioning

When the GatewayReconciler receives a Gateway ADDED event, it SHALL create a dedicated OIDC client in the configured Keycloak realm via the Admin REST API (`POST /admin/realms/{realm}/clients`). Keycloak provisioning occurs as part of the reconciliation loop, before deploying the gateway K8s resources.

The client SHALL be created with the following properties:

| Property | Value | Notes |
|---|---|---|
| `clientId` | Gateway name | Unique within the realm; used as OIDC `client_id` and `aud` |
| `name` | Gateway name | Display name in Keycloak admin console |
| `publicClient` | `true` | PKCE flow, no client secret required |
| `standardFlowEnabled` | `true` | Authorization code flow for browser/CLI |
| `directAccessGrantsEnabled` | `true` | Resource owner password grant for non-interactive CI pipelines that cannot use browser-based PKCE flow |
| `fullScopeAllowed` | `false` | **CRITICAL** — prevents cross-gateway role leakage |
| `redirectUris` | `["http://127.0.0.1:*", "http://localhost:*"]` | CLI callback URIs |
| `attributes.pkce.code.challenge.method` | `S256` | PKCE challenge method |
| `defaultClientScopes` | `["openid", "profile", "email", "roles", "gateway-roles", "web-origins", "acr"]` | Standard scopes plus `gateway-roles` |

> **`fullScopeAllowed` MUST be `false`.** Keycloak defaults to `true`, which leaks every client's roles into every token. Combined with the built-in `oidc-audience-resolve-mapper`, a token from any client would carry all other gateways' client IDs in `aud` plus their admin roles, breaking per-gateway isolation entirely.

The `gateway-roles` client scope is a realm prerequisite. It SHALL contain an `oidc-usermodel-client-role-mapper` that emits client roles under `resource_access.${client_id}.roles`. This scope SHALL exist in the Keycloak realm before gateways are created.

After creating the client, the GatewayReconciler SHALL resolve all existing RoleBindings that cover the new gateway (fleet-scoped bindings for the gateway's fleet, plus global-scoped bindings) and assign the corresponding Keycloak client roles to each user. This ensures that users with pre-existing fleet access receive Keycloak roles on newly created gateways.

#### Scenario: Reconciler provisions Keycloak client

- GIVEN the GatewayReconciler receives an ADDED event for Gateway `my-gateway`
- WHEN the reconciler provisions Keycloak
- THEN it SHALL create a Keycloak client with `clientId = "my-gateway"`
- AND the client SHALL have `fullScopeAllowed = false`
- AND the client SHALL have `publicClient = true` with `pkce.code.challenge.method = S256`
- AND the `gateway-roles` client scope SHALL be included in `defaultClientScopes`

#### Scenario: Existing fleet users receive roles on new gateway

- GIVEN user-a has `fleet:editor` on fleet-1
- AND user-b has `fleet:viewer` on fleet-1
- WHEN a new Gateway `gw-new` is created in fleet-1
- THEN the GatewayReconciler SHALL assign `openshell-admin` to user-a on the `gw-new` client
- AND it SHALL assign `openshell-user` to user-b on the `gw-new` client

#### Scenario: Duplicate client ID in Keycloak

- GIVEN a Keycloak client with `clientId = "my-gateway"` already exists in the realm
- WHEN the GatewayReconciler attempts to create the client
- THEN the reconciler SHALL log an error and retry on the next reconciliation cycle

---

### Requirement: Client Role Provisioning

After creating the OIDC client, the GatewayReconciler SHALL create two client-scoped roles via the Admin REST API (`POST /admin/realms/{realm}/clients/{client-uuid}/roles`).

| Role Name | Purpose |
|---|---|
| `openshell-admin` | Full administrative access to the gateway |
| `openshell-user` | Standard user access to the gateway |

#### Scenario: Client roles created

- GIVEN a Keycloak client has been created for gateway `my-gateway`
- WHEN the GatewayReconciler provisions client roles
- THEN it SHALL create the `openshell-admin` role on the `my-gateway` client
- AND it SHALL create the `openshell-user` role on the `my-gateway` client

---

### Requirement: Protocol Mapper Provisioning

After creating the client and roles, the GatewayReconciler SHALL create three protocol mappers on the OIDC client via the Admin REST API (`POST /admin/realms/{realm}/clients/{client-uuid}/protocol-mappers/models`).

#### Mapper: Audience

Sets the `aud` claim in the access token to match the client ID so the gateway can verify the token audience.

| Config Key | Value |
|---|---|
| `name` | `audience` |
| `protocol` | `openid-connect` |
| `protocolMapper` | `oidc-audience-mapper` |
| `config.included.client.audience` | `{clientId}` (e.g., `my-gateway`) |
| `config.id.token.claim` | `false` |
| `config.access.token.claim` | `true` |

#### Mapper: Sub

Ensures the `sub` claim is present in the access token so the gateway can identify the authenticated user.

| Config Key | Value |
|---|---|
| `name` | `sub` |
| `protocol` | `openid-connect` |
| `protocolMapper` | `oidc-sub-mapper` |
| `config.access.token.claim` | `true` |

#### Mapper: Client Roles

Maps the client's roles from `resource_access.{clientId}.roles` to a fixed `hypershell.roles` claim path in the access token. The gateway reads roles from this fixed path regardless of client ID.

| Config Key | Value |
|---|---|
| `name` | `client-roles` |
| `protocol` | `openid-connect` |
| `protocolMapper` | `oidc-usermodel-client-role-mapper` |
| `config.claim.name` | `hypershell.roles` |
| `config.multivalued` | `true` |
| `config.jsonType.label` | `String` |
| `config.id.token.claim` | `true` |
| `config.access.token.claim` | `true` |
| `config.usermodel.clientRoleMapping.clientId` | `{clientId}` (e.g., `my-gateway`) |

#### Scenario: All mappers provisioned

- GIVEN a Keycloak client `my-gateway` has been created with roles
- WHEN the GatewayReconciler provisions protocol mappers
- THEN the audience mapper SHALL set `aud` to `my-gateway` in access tokens
- AND the sub mapper SHALL ensure `sub` is present in access tokens
- AND the client-roles mapper SHALL map `resource_access.my-gateway.roles` to the `hypershell.roles` claim

#### Scenario: Token contains correct claims after provisioning

- GIVEN user `user-a` has the `openshell-admin` role on client `my-gateway`
- WHEN `user-a` obtains a token using `client_id = my-gateway`
- THEN the access token SHALL contain `aud: "my-gateway"`
- AND the access token SHALL contain `sub: "user-a-sub-id"`
- AND the access token SHALL contain `hypershell.roles: ["openshell-admin"]`
- AND the access token SHALL NOT contain roles from any other gateway's client

---

### Requirement: RBAC-Driven Keycloak Role Assignment (OIDC Role Bridge)

Keycloak client role assignments SHALL be driven by RoleBinding events, implementing the Gateway OIDC Role Bridge defined in [`rbac-enforcement.spec.md`](../security/rbac-enforcement.spec.md). The control plane SHALL assign or remove Keycloak client roles when RoleBindings are created or deleted, using the role mapping and scope resolution defined in the RBAC Role Bridge section above.

#### RoleBinding ADDED

For each RoleBinding ADDED event, the control plane SHALL:
1. Map the HyperShell role to a Keycloak client role (`openshell-admin` or `openshell-user`)
2. Resolve the affected gateways based on scope:
   - `scope=global` → all gateways across all fleets
   - `scope=fleet` → all gateways with `fleet_id` matching the binding's `fleet_id`
   - `scope=gateway` → the single gateway matching the binding's `gateway_id`
3. Resolve the User's Keycloak identity (the `username` from the User record, which is populated from the JWT `preferred_username` claim at auto-provisioning time)
4. Look up the user in Keycloak (`GET /admin/realms/{realm}/users?username={username}`)
5. For each affected gateway's Keycloak client:
   - Retrieve the client role UUID
   - Assign the role to the user (`POST /admin/realms/{realm}/users/{user-uuid}/role-mappings/clients/{client-uuid}`)

#### RoleBinding DELETED

For each RoleBinding DELETED event, the control plane SHALL:
1. Resolve the affected gateways (same scope logic as ADDED)
2. Check whether the user has any remaining RoleBindings that cover each gateway
3. If the user still has a binding that maps to the same or higher Keycloak role, take no action on that gateway
4. Otherwise, remove the Keycloak client role mapping (`DELETE /admin/realms/{realm}/users/{user-uuid}/role-mappings/clients/{client-uuid}`)

#### Scenario: Fleet editor receives admin role on all fleet gateways

- GIVEN fleet-1 contains gateways `gw-alpha` and `gw-beta`
- AND a User `user-a` exists in both HyperShell and Keycloak
- WHEN a RoleBinding is created: `role=fleet:editor`, `scope=fleet`, `fleet_id=fleet-1`, `user_id=user-a`
- THEN the control plane SHALL assign `openshell-admin` to `user-a` on both `gw-alpha` and `gw-beta` Keycloak clients
- AND `user-a` SHALL be able to obtain tokens with `hypershell.roles: ["openshell-admin"]` for either gateway

#### Scenario: Fleet viewer receives user role on all fleet gateways

- GIVEN fleet-1 contains gateway `gw-alpha`
- AND a User `user-b` exists in both HyperShell and Keycloak
- WHEN a RoleBinding is created: `role=fleet:viewer`, `scope=fleet`, `fleet_id=fleet-1`, `user_id=user-b`
- THEN the control plane SHALL assign `openshell-user` to `user-b` on the `gw-alpha` Keycloak client

#### Scenario: Gateway viewer receives user role on one gateway

- GIVEN gateway `gw-alpha` exists with a provisioned Keycloak client
- WHEN a RoleBinding is created: `role=gateway:viewer`, `scope=gateway`, `gateway_id=gw-alpha`, `user_id=user-c`
- THEN the control plane SHALL assign `openshell-user` to `user-c` on the `gw-alpha` Keycloak client only

#### Scenario: Platform admin receives admin role on all gateways

- GIVEN gateways exist across multiple fleets
- WHEN a RoleBinding is created: `role=platform:admin`, `scope=global`, `user_id=user-d`
- THEN the control plane SHALL assign `openshell-admin` to `user-d` on every gateway's Keycloak client

#### Scenario: RoleBinding deletion with remaining coverage

- GIVEN user-a has `fleet:editor` (→ `openshell-admin`) on fleet-1
- AND user-a also has `fleet:viewer` (→ `openshell-user`) on fleet-1
- WHEN the `fleet:editor` RoleBinding is deleted
- THEN the control plane SHALL downgrade user-a to `openshell-user` on fleet-1's gateways
- AND user-a SHALL NOT lose Keycloak roles entirely (the viewer binding still covers them)

#### Scenario: RoleBinding deletion with no remaining coverage

- GIVEN user-a has only `fleet:viewer` (→ `openshell-user`) on fleet-1
- WHEN the `fleet:viewer` RoleBinding is deleted
- THEN the control plane SHALL remove `openshell-user` from user-a on all fleet-1 gateways

#### Scenario: RoleBinding created before Keycloak client exists

- GIVEN fleet-1 has no gateways yet
- WHEN a RoleBinding is created: `role=fleet:editor`, `scope=fleet`, `fleet_id=fleet-1`
- THEN no Keycloak role assignments occur (no gateways to assign to)
- AND when a gateway is later created in fleet-1, the GatewayReconciler SHALL resolve this binding and assign `openshell-admin`

#### Scenario: User in RoleBinding not found in Keycloak

- GIVEN a RoleBinding references a User whose Keycloak identity cannot be resolved
- WHEN the control plane attempts to assign the Keycloak client role
- THEN it SHALL log an error identifying the unresolvable user
- AND it SHALL retry on the next reconciliation cycle

---

### Requirement: Auto-Populated OIDC Configuration

After successful Keycloak provisioning, the GatewayReconciler SHALL populate the Gateway resource's `oidc` field with values derived from the provisioned client and the Keycloak service account configuration. The user SHALL NOT supply OIDC configuration when creating a gateway — it is system-managed.

The auto-populated OIDC values SHALL be:

| OIDC Field | Value | Source |
|---|---|---|
| `issuer` | `{server-url}/realms/{realm}` | Keycloak service account config |
| `audience` | `{clientId}` | Provisioned client ID (= gateway name) |
| `jwks_ttl` | `3600` | Default |
| `roles_claim` | `hypershell.roles` | Fixed claim path from client-roles mapper |
| `admin_role` | `openshell-admin` | Fixed role name |
| `user_role` | `openshell-user` | Fixed role name |
| `scopes_claim` | `""` | Default empty — upstream gateway configuration field |

The OIDC fields on the Gateway resource SHALL be read-only — not settable or updatable via the REST or gRPC API. The control plane SHALL inject these values into `gateway.toml` using the existing OIDC injection behavior defined in `openshell-gateway-oidc.spec.md`.

#### Scenario: Gateway created with auto-populated OIDC

- GIVEN a user creates a Gateway named `my-gateway`
- AND the Keycloak realm is at `https://keycloak.example.com/realms/hypershell`
- WHEN Keycloak provisioning succeeds
- THEN the persisted Gateway's `oidc` field SHALL be:
  ```json
  {
    "issuer": "https://keycloak.example.com/realms/hypershell",
    "audience": "my-gateway",
    "jwks_ttl": 3600,
    "roles_claim": "hypershell.roles",
    "admin_role": "openshell-admin",
    "user_role": "openshell-user"
  }
  ```
- AND the control plane SHALL generate `gateway.toml` containing:
  ```toml
  [openshell.gateway.oidc]
  issuer        = "https://keycloak.example.com/realms/hypershell"
  audience      = "my-gateway"
  jwks_ttl_secs = 3600
  roles_claim   = "hypershell.roles"
  admin_role    = "openshell-admin"
  user_role     = "openshell-user"
  ```

---

### Requirement: Gateway Visibility Scoping

Gateway visibility is defined by the RBAC spec's scope-aware list filtering. The API server SHALL return only gateways within fleets where the caller has a RoleBinding. This is enforced at the API layer by the RBAC authorization middleware; the UI receives only the gateways the authenticated user has access to.

This keycloak spec does not define the visibility query mechanism — that is the responsibility of [`rbac-enforcement.spec.md`](../security/rbac-enforcement.spec.md). This spec defines only the Keycloak projection: when a user has a RoleBinding that covers a gateway (via fleet scope, gateway scope, or global scope), they also have the corresponding Keycloak client role, enabling them to obtain valid tokens for that gateway.

#### Scenario: List gateways returns only gateways in accessible fleets

- GIVEN `gw-alpha` belongs to fleet-1, user-a has `fleet:editor` on fleet-1
- AND `gw-beta` belongs to fleet-2, user-a has no binding on fleet-2
- WHEN `user-a` calls `GET /api/hypershell/v1/gateways`
- THEN the response SHALL contain `gw-alpha`
- AND the response SHALL NOT contain `gw-beta`

#### Scenario: Get gateway in inaccessible fleet returns 404

- GIVEN `gw-beta` belongs to fleet-2
- AND user-a has no RoleBinding covering fleet-2
- WHEN `user-a` calls `GET /api/hypershell/v1/gateways/{gw-beta-id}`
- THEN the API server SHALL return 404
- AND the response SHALL NOT reveal that the gateway exists

---

### Requirement: Keycloak Client Cleanup

When the GatewayReconciler receives a Gateway DELETED event, it SHALL delete the corresponding Keycloak OIDC client to prevent orphaned clients in the realm. Deleting a Keycloak client automatically cascades to its roles, protocol mappers, and all user role assignments on that client.

#### Scenario: Gateway deletion cleans up Keycloak

- GIVEN a Gateway `my-gateway` with a corresponding Keycloak client
- WHEN the GatewayReconciler receives a DELETED event for the Gateway
- THEN it SHALL look up the client by `clientId` (`GET /admin/realms/{realm}/clients?clientId=my-gateway`)
- AND it SHALL delete the client (`DELETE /admin/realms/{realm}/clients/{client-uuid}`)
- AND the client's roles, mappers, and user role assignments SHALL be automatically removed by Keycloak

#### Scenario: Keycloak cleanup failure is non-blocking

- GIVEN the GatewayReconciler processes a Gateway DELETED event
- WHEN the Keycloak client deletion fails (e.g., network error, Keycloak unavailable)
- THEN the reconciler SHALL log an error with the orphaned `clientId` for manual cleanup
- AND it SHALL continue processing the remaining gateway resource deletion

---

### Requirement: Provisioning Atomicity

Keycloak client provisioning (client, roles, mappers) SHALL be treated as an atomic operation within the reconciliation cycle. If any provisioning step fails, the GatewayReconciler SHALL roll back all completed Keycloak steps and retry on the next reconciliation cycle.

OIDC Role Bridge operations (from RoleBinding events) are separate from client provisioning and do not participate in its atomicity. Role assignment failures are retried independently on the next reconciliation cycle.

#### Scenario: Mapper creation fails mid-provisioning

- GIVEN the GatewayReconciler has created the Keycloak client and roles
- WHEN mapper creation fails
- THEN the reconciler SHALL delete the created client from Keycloak (cascading roles)
- AND it SHALL log the error and retry on the next reconciliation cycle
- AND the gateway SHALL NOT be deployed until Keycloak provisioning succeeds

---

## Keycloak Realm Prerequisites

The following resources SHALL exist in the configured Keycloak realm (as specified by the `realm` key in the `hypershell-keycloak-admin` Secret) before gateways can be created. These are configuration prerequisites — neither the API server nor the control plane creates them.

1. **Service account client** — A confidential client (e.g., `hypershell-admin-sa`) with the `realm-management` client role `realm-admin` or equivalent permissions to create clients, roles, mappers, and manage user role mappings.

2. **`gateway-roles` client scope** — A client scope containing an `oidc-usermodel-client-role-mapper` that emits `resource_access.${client_id}.roles`. This scope SHALL be included in `defaultClientScopes` for every provisioned gateway client so client roles appear in tokens.

3. **User accounts** — Users referenced by RoleBindings must have accounts in the Keycloak realm so the control plane can assign roles. Since HyperShell auto-provisions User records from JWT claims (see RBAC spec), Keycloak user accounts are expected to exist as a precondition of authentication.

---

## Keycloak Admin API Call Sequence

### Gateway Creation (Client Lifecycle)

Complete API call sequence for gateway creation:

1. **Obtain service account token:**
   ```
   POST {server-url}/realms/{realm}/protocol/openid-connect/token
   Content-Type: application/x-www-form-urlencoded

   grant_type=client_credentials&client_id={sa-client-id}&client_secret={sa-client-secret}
   ```

2. **Create client:**
   ```
   POST /admin/realms/{realm}/clients
   ```
   Body includes `clientId`, `publicClient: true`, `fullScopeAllowed: false`, `defaultClientScopes`, PKCE attribute, `redirectUris`, and inline audience mapper via `protocolMappers`.

3. **Create openshell-admin role:**
   ```
   POST /admin/realms/{realm}/clients/{client-uuid}/roles
   {"name": "openshell-admin"}
   ```

4. **Create openshell-user role:**
   ```
   POST /admin/realms/{realm}/clients/{client-uuid}/roles
   {"name": "openshell-user"}
   ```

5. **Create sub mapper:**
   ```
   POST /admin/realms/{realm}/clients/{client-uuid}/protocol-mappers/models
   ```

6. **Create client-roles mapper:**
   ```
   POST /admin/realms/{realm}/clients/{client-uuid}/protocol-mappers/models
   ```

7. **Resolve existing fleet RoleBindings and assign Keycloak roles** (see RoleBinding Creation below for per-user sequence)

### RoleBinding Creation (OIDC Role Bridge)

When a RoleBinding is created, for each affected gateway:

1. **Get the Keycloak client UUID for the gateway:**
   ```
   GET /admin/realms/{realm}/clients?clientId={gateway-name}
   ```

2. **Get the Keycloak client role UUID:**
   ```
   GET /admin/realms/{realm}/clients/{client-uuid}/roles?search={role-name}
   ```
   Where `{role-name}` is `openshell-admin` or `openshell-user` based on the role mapping.

3. **Look up user:**
   ```
   GET /admin/realms/{realm}/users?username={username}
   ```

4. **Assign role:**
   ```
   POST /admin/realms/{realm}/users/{user-uuid}/role-mappings/clients/{client-uuid}
   [{"name": "{role-name}", "id": "{role-uuid}"}]
   ```

### RoleBinding Deletion (OIDC Role Bridge)

When a RoleBinding is deleted, for each affected gateway (after checking no remaining coverage):

1. **Remove role mapping:**
   ```
   DELETE /admin/realms/{realm}/users/{user-uuid}/role-mappings/clients/{client-uuid}
   [{"name": "{role-name}", "id": "{role-uuid}"}]
   ```

### Gateway Deletion (Client Cleanup)

1. **Look up client by clientId:**
   ```
   GET /admin/realms/{realm}/clients?clientId={gateway-name}
   ```

2. **Delete client (cascades roles, mappers, and all user role assignments):**
   ```
   DELETE /admin/realms/{realm}/clients/{client-uuid}
   ```

---

## Key Gotchas

| Pitfall | Detail | Mitigation |
|---|---|---|
| `fullScopeAllowed` defaults to `true` | Leaks every client's roles into every token, breaking per-gateway isolation | Always set `fullScopeAllowed = false` explicitly at client creation |
| Bulk realm import does not scale | Importing a realm with thousands of clients runs as a single transaction | Use incremental Admin REST API calls for each gateway |
| Audience resolve mapper leaks audiences | The built-in `oidc-audience-resolve-mapper` adds all clients' IDs to `aud` when `fullScopeAllowed = true` | Omit the audience-resolve mapper from the realm; use per-client audience mappers |
| User lookup by subject | Keycloak user search uses `username`, which may differ from the OIDC `sub` claim depending on identity provider federation | The RBAC spec auto-provisions Users from `preferred_username`; ensure this matches the Keycloak username |
| Fleet-scoped RoleBinding fans out to many gateways | A fleet with N gateways requires N Keycloak API calls per RoleBinding event | Batch or parallelize role assignments; consider caching client UUIDs |
| Gateway created before RoleBinding event processed | Race between Gateway ADDED and RoleBinding ADDED for the fleet | GatewayReconciler resolves existing fleet RoleBindings at client creation time |
| RoleBinding deletion requires coverage check | Removing one binding may leave the user covered by another | Resolve all remaining bindings for the user+gateway before removing the Keycloak role |
| Gateway deletion cascades role assignments | Deleting a Keycloak client removes all user role assignments for that client | RBAC RoleBindings remain in HyperShell; they cover future gateways in the same fleet |

---

## References

- [Keycloak Admin REST API](https://www.keycloak.org/docs-api/latest/rest-api/) — client, role, mapper, and user management endpoints
- [Keycloak Client Scope Configuration](https://www.keycloak.org/docs/latest/server_admin/#_client_scopes) — gateway-roles scope setup
- [Keycloak Protocol Mappers](https://www.keycloak.org/docs/latest/server_admin/#_protocol-mappers) — audience, sub, and client-role mapper types
- [PKCE (RFC 7636)](https://datatracker.ietf.org/doc/html/rfc7636) — S256 challenge method for public clients
- [Multi-Gateway OIDC Isolation](https://gist.github.com/jhjaggars/e17c2b094008c14682e3b448eca405eb) — scale testing and isolation verification for per-gateway Keycloak provisioning
