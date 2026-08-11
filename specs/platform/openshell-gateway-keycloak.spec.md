# OpenShell Gateway Keycloak Provisioning Specification

**Date:** 2026-08-11
**Status:** Draft
**Parent:** `openshell-gateway.spec.md` - core gateway provisioning
**Related:** `openshell-gateway-oidc.spec.md` - OIDC configuration injection into gateway.toml; `security/security.spec.md` - secret management and isolation; `data-model.spec.md` - Gateway kind definition; `web-console/architecture.spec.md` - gateway visibility
**Depends on:** RBAC specification (forthcoming) — User and RoleBinding resources
**Upstream:** [Keycloak Admin REST API](https://www.keycloak.org/docs-api/latest/rest-api/)

---

## Purpose

This specification defines automated per-gateway Keycloak OIDC client provisioning. Keycloak integration has two distinct lifecycles:

1. **Client lifecycle** — When a gateway is created, the control plane provisions a dedicated OIDC client in Keycloak with client-scoped roles and protocol mappers, and populates the gateway's OIDC configuration. When a gateway is deleted, the control plane deletes the Keycloak client. This lifecycle is tied to Gateway ADDED/DELETED events.

2. **Role assignment lifecycle** — When a User is bound to a Gateway via a RoleBinding (e.g., `admin` or `user`), the control plane assigns the corresponding Keycloak client role to that user. When a RoleBinding is removed, the control plane removes the role assignment. This lifecycle is driven by RoleBinding events, not Gateway events.

This establishes per-gateway authentication isolation: each gateway has its own audience, roles, and token claims. Visibility and access control are RBAC concerns — the RBAC system determines which users can see and operate on which gateways, and this spec defines how those RBAC decisions are projected into Keycloak role assignments.

---

## Architecture

### Provisioning Flow

```
Gateway Lifecycle (Gateway ADDED/DELETED events):

    Caller (UI, hsctl, CI pipeline, curl)
        |  POST /api/hypershell/v1/gateways
        |  Body includes: name (OIDC config and role assignments are NOT part of the create request)
        v
    API Server
        |  1. Persists Gateway (oidc field empty at this point)
        |  2. Emits gRPC watch event
        v
    Control Plane - GatewayReconciler
        |  1. Receives Gateway ADDED event
        |  2. Provisions Keycloak via Admin REST API:
        |     a. Creates OIDC client (clientId = gateway name)
        |     b. Creates client roles (openshell-admin, openshell-user)
        |     c. Creates protocol mappers (audience, sub, client-roles)
        |  3. Populates Gateway oidc config (PATCH via API or internal state)
        |  4. Injects OIDC section into gateway.toml
        |  5. Deploys gateway K8s resources
        v
    Gateway Pod
        |  Validates JWTs against the provisioned Keycloak client
        v
    Keycloak client ready (no users have roles yet)


Role Assignment Lifecycle (RoleBinding ADDED/DELETED events):

    Caller (UI, hsctl, CI pipeline, curl)
        |  POST /api/hypershell/v1/role_bindings
        |  Body includes: user, gateway, role (admin or user)
        v
    API Server
        |  1. Validates User exists in RBAC (and optionally in Keycloak)
        |  2. Persists RoleBinding
        |  3. Emits gRPC watch event
        v
    Control Plane - GatewayReconciler (or RoleBindingReconciler)
        |  1. Receives RoleBinding ADDED event
        |  2. Looks up the User's Keycloak identity
        |  3. Assigns the corresponding Keycloak client role:
        |     - RoleBinding role "admin" → openshell-admin client role
        |     - RoleBinding role "user"  → openshell-user client role
        v
    User can now obtain tokens with the assigned role for this gateway
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
    |   +-- Reads Secret -> validates User exists in Keycloak (read-only, optional fail-fast)
    |
    +-- Control Plane Pod
        +-- Reads Secret -> provisions Keycloak clients, roles, mappers (read-write)
        +-- Reads Secret -> assigns/removes Keycloak client roles on RoleBinding events (read-write)
```

### Per-Gateway Isolation Model

```
User A creates "gw-alpha"              User B creates "gw-beta"
    KC client: gw-alpha                     KC client: gw-beta
    (no role assignments yet)               (no role assignments yet)

RoleBinding: user-a → gw-alpha (admin)  RoleBinding: user-b → gw-beta (admin)
    admin role → user-a                     admin role → user-b

    GET /gateways                           GET /gateways
    Returns: [gw-alpha]                     Returns: [gw-beta]
    (RBAC filters by RoleBinding)           (RBAC filters by RoleBinding)

RoleBinding: user-a → gw-shared (admin)
RoleBinding: user-b → gw-shared (admin)
    admin role → user-a, user-b

    user-a GET /gateways → [gw-alpha, gw-shared]
    user-b GET /gateways → [gw-beta, gw-shared]
```

Each gateway's Keycloak client has `fullScopeAllowed = false`, preventing role leakage across gateways. A token obtained for `gw-alpha` contains only `gw-alpha`'s roles and audience — never `gw-beta`'s.

Gateway visibility is an RBAC concern — a user sees a gateway if and only if they have a RoleBinding to it. This decouples visibility from gateway creation, enabling programmatic creation (CI, `hsctl apply`, automation) where the creator is not necessarily the intended gateway admin.

---

## RBAC Integration Contract

This specification depends on a forthcoming RBAC specification that defines User and RoleBinding resources. The RBAC spec SHALL provide the following contract for Keycloak integration:

### Required Resources

| Resource | Purpose |
|---|---|
| **User** | Represents an identity in HyperShell. SHALL include a Keycloak username or identifier that the control plane can use to look up the user in Keycloak for role assignment. |
| **RoleBinding** | Binds a User to a Gateway with a specific role (`admin` or `user`). SHALL be a first-class HyperShell resource with its own gRPC watch events. |

### Required Events

The RBAC system SHALL emit gRPC watch events for RoleBinding changes:

| Event | Keycloak Action |
|---|---|
| RoleBinding ADDED (role = `admin`) | Assign `openshell-admin` client role to the User on the Gateway's Keycloak client |
| RoleBinding ADDED (role = `user`) | Assign `openshell-user` client role to the User on the Gateway's Keycloak client |
| RoleBinding DELETED | Remove the corresponding client role from the User on the Gateway's Keycloak client |

### Required Behavior

- **Visibility:** The API server SHALL use RoleBindings to scope gateway visibility. A user sees a gateway if and only if they have a RoleBinding to it (any role).
- **User validation:** The API server SHOULD validate that Users referenced in RoleBindings exist in Keycloak at creation time (fail-fast). This prevents the control plane from encountering missing users during reconciliation.
- **Default RoleBinding:** The UI SHOULD auto-create a RoleBinding for the authenticated user when they provision a gateway, so the creator has admin access by default. This is a UI/API convenience, not a hard requirement — programmatic callers manage RoleBindings explicitly.

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

- **API server** uses the Secret for read-only operations: validating that Users exist in Keycloak when RoleBindings are created (optional fail-fast).
- **Control plane** uses the Secret for read-write operations: provisioning Keycloak clients, roles, mappers during gateway reconciliation, and assigning/removing client roles during RoleBinding reconciliation.

#### Scenario: Service account credentials available

- GIVEN the `hypershell-keycloak-admin` Secret exists in the HyperShell namespace
- WHEN the API server and control plane start
- THEN both SHALL read the Secret and validate that all required keys are present
- AND both SHALL verify connectivity to the Keycloak Admin REST API at startup

#### Scenario: Service account credentials missing at API server

- GIVEN the `hypershell-keycloak-admin` Secret does not exist
- WHEN a user attempts to create a gateway
- THEN the API server SHALL return an error indicating Keycloak integration is not configured
- AND the gateway SHALL NOT be created

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
| `directAccessGrantsEnabled` | `true` | Resource owner password grant for automation |
| `fullScopeAllowed` | `false` | **CRITICAL** — prevents cross-gateway role leakage |
| `redirectUris` | `["http://127.0.0.1:*", "http://localhost:*"]` | CLI callback URIs |
| `attributes.pkce.code.challenge.method` | `S256` | PKCE challenge method |
| `defaultClientScopes` | `["openid", "profile", "email", "roles", "gateway-roles", "web-origins", "acr"]` | Standard scopes plus `gateway-roles` |

> **`fullScopeAllowed` MUST be `false`.** Keycloak defaults to `true`, which leaks every client's roles into every token. Combined with the built-in `oidc-audience-resolve-mapper`, a token from any client would carry all other gateways' client IDs in `aud` plus their admin roles, breaking per-gateway isolation entirely.

The `gateway-roles` client scope is a realm prerequisite. It SHALL contain an `oidc-usermodel-client-role-mapper` that emits client roles under `resource_access.${client_id}.roles`. This scope SHALL exist in the Keycloak realm before gateways are created.

#### Scenario: Reconciler provisions Keycloak client

- GIVEN the GatewayReconciler receives an ADDED event for Gateway `my-gateway`
- WHEN the reconciler provisions Keycloak
- THEN it SHALL create a Keycloak client with `clientId = "my-gateway"`
- AND the client SHALL have `fullScopeAllowed = false`
- AND the client SHALL have `publicClient = true` with `pkce.code.challenge.method = S256`
- AND the `gateway-roles` client scope SHALL be included in `defaultClientScopes`

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
- AND the access token SHALL contain `sub: "user-a"`
- AND the access token SHALL contain `hypershell.roles: ["openshell-admin"]`
- AND the access token SHALL NOT contain roles from any other gateway's client

---

### Requirement: RBAC-Driven Role Assignment

Keycloak client role assignments SHALL be driven by RoleBinding events, not by fields on the Gateway resource. When a RoleBinding binds a User to a Gateway with a specific role, the control plane SHALL assign the corresponding Keycloak client role to that User.

For each RoleBinding ADDED event, the control plane SHALL:
1. Resolve the User's Keycloak identity (username or ID)
2. Look up the User in Keycloak (`GET /admin/realms/{realm}/users?username={username}`)
3. Resolve the Gateway's Keycloak client UUID
4. Map the RoleBinding role to a Keycloak client role:
   - `admin` → `openshell-admin`
   - `user` → `openshell-user`
5. Retrieve the client role UUID (`GET /admin/realms/{realm}/clients/{client-uuid}/roles?search={role-name}`)
6. Assign the role to the user (`POST /admin/realms/{realm}/users/{user-uuid}/role-mappings/clients/{client-uuid}`)

For each RoleBinding DELETED event, the control plane SHALL:
1. Resolve the User's Keycloak identity and the Gateway's Keycloak client UUID
2. Remove the corresponding client role mapping (`DELETE /admin/realms/{realm}/users/{user-uuid}/role-mappings/clients/{client-uuid}`)

#### Scenario: Admin RoleBinding created

- GIVEN a Gateway `my-gateway` with a provisioned Keycloak client
- AND a User `user-a` exists in both HyperShell and Keycloak
- WHEN a RoleBinding is created binding `user-a` to `my-gateway` with role `admin`
- THEN the control plane SHALL assign the `openshell-admin` client role to `user-a` on the `my-gateway` Keycloak client
- AND `user-a` SHALL be able to obtain tokens with `hypershell.roles: ["openshell-admin"]`

#### Scenario: User RoleBinding created

- GIVEN a Gateway `my-gateway` with a provisioned Keycloak client
- AND a User `user-b` exists in both HyperShell and Keycloak
- WHEN a RoleBinding is created binding `user-b` to `my-gateway` with role `user`
- THEN the control plane SHALL assign the `openshell-user` client role to `user-b` on the `my-gateway` Keycloak client

#### Scenario: RoleBinding deleted

- GIVEN `user-a` has the `openshell-admin` role on `my-gateway`'s Keycloak client via a RoleBinding
- WHEN the RoleBinding is deleted
- THEN the control plane SHALL remove the `openshell-admin` client role from `user-a` on the `my-gateway` Keycloak client
- AND `user-a` SHALL no longer be able to obtain tokens with admin roles for `my-gateway`

#### Scenario: RoleBinding created before Keycloak client exists

- GIVEN a RoleBinding is created binding `user-a` to `my-gateway` with role `admin`
- AND the `my-gateway` Keycloak client has not yet been provisioned
- THEN the control plane SHALL retry on the next reconciliation cycle
- AND role assignment SHALL succeed once the Keycloak client is provisioned

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

Gateway visibility is an RBAC concern. The API server SHALL scope gateway visibility by RoleBinding membership — a user SHALL only see and operate on gateways where they have a RoleBinding (any role: `admin` or `user`). This is enforced at the API layer; the UI receives only the gateways the authenticated user has access to.

The Keycloak spec does not define the visibility query mechanism — that is the responsibility of the RBAC specification. This spec defines only the Keycloak projection: when a user has a RoleBinding to a gateway, they also have the corresponding Keycloak client role, enabling them to obtain valid tokens for that gateway.

#### Scenario: List gateways returns only gateways where user has a RoleBinding

- GIVEN `gw-alpha` has a RoleBinding for `user-a` (admin)
- AND `gw-beta` has RoleBindings for `user-a` (user) and `user-b` (admin)
- AND `gw-gamma` has a RoleBinding for `user-b` (admin) only
- WHEN `user-a` calls `GET /api/hypershell/v1/gateways`
- THEN the response SHALL contain `gw-alpha` and `gw-beta`
- AND the response SHALL NOT contain `gw-gamma`

#### Scenario: Get gateway where user has no RoleBinding returns 404

- GIVEN `gw-gamma` has a RoleBinding for `user-b` only
- WHEN `user-a` calls `GET /api/hypershell/v1/gateways/{gw-gamma-id}`
- THEN the API server SHALL return 404
- AND the response SHALL NOT reveal that the gateway exists

---

### Requirement: Keycloak Client Cleanup

When the GatewayReconciler receives a Gateway DELETED event, it SHALL delete the corresponding Keycloak OIDC client to prevent orphaned clients in the realm. Deleting a Keycloak client automatically cascades to its roles and protocol mappers. All role assignments for users on that client are also removed by Keycloak.

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

Role assignment (from RoleBinding events) is a separate operation and does not participate in client provisioning atomicity. Role assignment failures are retried independently on the next reconciliation cycle.

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

3. **User accounts** — Users referenced by RoleBindings must have accounts in the Keycloak realm so the control plane can assign roles.

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

### RoleBinding Creation (Role Assignment)

When a RoleBinding binds a User to a Gateway:

1. **Get the Keycloak client role UUID:**
   ```
   GET /admin/realms/{realm}/clients/{client-uuid}/roles?search={role-name}
   ```
   Where `{role-name}` is `openshell-admin` or `openshell-user` based on the RoleBinding role.

2. **Look up user:**
   ```
   GET /admin/realms/{realm}/users?username={username}
   ```

3. **Assign role:**
   ```
   POST /admin/realms/{realm}/users/{user-uuid}/role-mappings/clients/{client-uuid}
   [{"name": "{role-name}", "id": "{role-uuid}"}]
   ```

### RoleBinding Deletion (Role Removal)

When a RoleBinding is deleted:

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
| User lookup by subject | Keycloak user search uses `username`, which may differ from the OIDC `sub` claim depending on identity provider federation | Ensure Keycloak user identity attributes align with the authentication token's `sub` claim |
| RoleBinding before Keycloak client | RoleBindings can be created before the Gateway's Keycloak client is provisioned | Control plane retries role assignment on the next reconciliation cycle |
| Gateway deletion cascades role assignments | Deleting a Keycloak client removes all user role assignments for that client | RBAC RoleBindings remain in HyperShell; they become inert without a corresponding Keycloak client |

---

## References

- [Keycloak Admin REST API](https://www.keycloak.org/docs-api/latest/rest-api/) — client, role, mapper, and user management endpoints
- [Keycloak Client Scope Configuration](https://www.keycloak.org/docs/latest/server_admin/#_client_scopes) — gateway-roles scope setup
- [Keycloak Protocol Mappers](https://www.keycloak.org/docs/latest/server_admin/#_protocol-mappers) — audience, sub, and client-role mapper types
- [PKCE (RFC 7636)](https://datatracker.ietf.org/doc/html/rfc7636) — S256 challenge method for public clients
- [Multi-Gateway OIDC Isolation](https://gist.github.com/jhjaggars/e17c2b094008c14682e3b448eca405eb) — scale testing and isolation verification for per-gateway Keycloak provisioning
