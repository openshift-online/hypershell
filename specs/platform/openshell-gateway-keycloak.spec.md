# OpenShell Gateway Keycloak Provisioning Specification

**Date:** 2026-08-11
**Status:** Draft
**Parent:** `openshell-gateway.spec.md` - core gateway provisioning
**Related:** `openshell-gateway-oidc.spec.md` - OIDC configuration injection into gateway.toml; `security/security.spec.md` - secret management and isolation; `data-model.spec.md` - Gateway kind definition; `web-console/architecture.spec.md` - gateway visibility
**Upstream:** [Keycloak Admin REST API](https://www.keycloak.org/docs-api/latest/rest-api/)

---

## Purpose

This specification defines automated per-gateway Keycloak OIDC client provisioning. When a gateway is created, the API server provisions a dedicated OIDC client in Keycloak with client-scoped roles and protocol mappers, assigns the admin role to each user listed in the request's `admin_users` field, and populates the gateway's OIDC configuration. This establishes per-gateway authentication isolation: each gateway has its own audience, roles, and token claims, and users can only access gateways where they hold a role.

---

## Architecture

### Provisioning Flow

```
Caller (UI, hsctl, CI pipeline, curl)
    |  POST /api/hypershell/v1/gateways
    |  Body includes: name, admin_users (required, >= 1 Keycloak username)
    v
API Server
    |  1. Validates admin_users (non-empty)
    |  2. Calls Keycloak Admin REST API:
    |     a. Creates OIDC client (clientId = gateway name)
    |     b. Creates client roles (openshell-admin, openshell-user)
    |     c. Creates protocol mappers (audience, sub, client-roles)
    |     d. Assigns openshell-admin role to each user in admin_users
    |  3. Persists Gateway with admin_users and auto-populated oidc config
    |  4. Emits gRPC watch event
    v
Control Plane
    |  Receives Gateway with OIDC config already populated
    |  Injects OIDC section into gateway.toml (existing behavior per oidc spec)
    v
Gateway Pod
    |  Validates JWTs against the provisioned Keycloak client
    v
Authorized Access (admin/user roles scoped to this gateway only)
```

### Keycloak Service Account

```
HyperShell Namespace
    |
    +-- Secret: hypershell-keycloak-admin
    |   +-- server-url:    https://keycloak.example.com
    |   +-- realm:         hypershell
    |   +-- client-id:     hypershell-admin-sa
    |   +-- client-secret: <service account secret>
    |
    +-- API Server Pod
        +-- Reads Secret -> authenticates to Keycloak Admin REST API
```

### Per-Gateway Isolation Model

```
User A creates "gw-alpha"            User B creates "gw-beta"
    admin_users = ["user-a"]              admin_users = ["user-b"]
    KC client: gw-alpha                   KC client: gw-beta
    admin role -> user-a                  admin role -> user-b
                                          |
    GET /gateways                         GET /gateways
    Returns: [gw-alpha]                   Returns: [gw-beta]
                                          |
    GET /gateways/{gw-beta-id}            GET /gateways/{gw-alpha-id}
    Returns: 404                          Returns: 404

CI Pipeline creates "gw-shared"
    admin_users = ["user-a", "user-b"]
    KC client: gw-shared
    admin role -> user-a, user-b

    user-a GET /gateways -> [gw-alpha, gw-shared]
    user-b GET /gateways -> [gw-beta, gw-shared]
```

Each gateway's Keycloak client has `fullScopeAllowed = false`, preventing role leakage across gateways. A token obtained for `gw-alpha` contains only `gw-alpha`'s roles and audience — never `gw-beta`'s.

Visibility is scoped by `admin_users` membership — a user sees a gateway if and only if they appear in its `admin_users` list. This decouples visibility from the caller's identity, enabling programmatic creation (CI, `hsctl apply`, automation) where the caller is not the intended gateway admin.

---

## Requirements

### Requirement: Keycloak Service Account Access

The API server SHALL authenticate to the Keycloak Admin REST API using a service account with admin permissions in the `hypershell` realm. The service account credentials SHALL be stored in a Kubernetes Secret named `hypershell-keycloak-admin` in the HyperShell namespace.

The Secret SHALL contain the following keys:

| Key | Description |
|---|---|
| `server-url` | Keycloak base URL (e.g., `https://keycloak.example.com`) |
| `realm` | Keycloak realm name (e.g., `hypershell`) |
| `client-id` | Service account client ID with realm admin permissions |
| `client-secret` | Service account client secret |

The API server SHALL obtain an access token from Keycloak using the client credentials grant (`grant_type=client_credentials`) before making Admin REST API calls. The API server SHOULD cache and refresh the service account token to avoid per-request token acquisition.

#### Scenario: Service account credentials available

- GIVEN the `hypershell-keycloak-admin` Secret exists in the HyperShell namespace
- WHEN the API server starts
- THEN it SHALL read the Secret and validate that all required keys are present
- AND it SHALL verify connectivity to the Keycloak Admin REST API at startup

#### Scenario: Service account credentials missing

- GIVEN the `hypershell-keycloak-admin` Secret does not exist
- WHEN a user attempts to create a gateway
- THEN the API server SHALL return an error indicating Keycloak integration is not configured
- AND the gateway SHALL NOT be created

---

### Requirement: Per-Gateway OIDC Client Provisioning

When a Gateway is created, the API server SHALL create a dedicated OIDC client in the configured Keycloak realm via the Admin REST API (`POST /admin/realms/{realm}/clients`).

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

#### Scenario: Create gateway provisions Keycloak client

- GIVEN an authenticated user with subject `user-a`
- WHEN the user creates a Gateway named `my-gateway`
- THEN the API server SHALL create a Keycloak client with `clientId = "my-gateway"`
- AND the client SHALL have `fullScopeAllowed = false`
- AND the client SHALL have `publicClient = true` with `pkce.code.challenge.method = S256`
- AND the `gateway-roles` client scope SHALL be included in `defaultClientScopes`

#### Scenario: Duplicate client ID in Keycloak

- GIVEN a Keycloak client with `clientId = "my-gateway"` already exists in the realm
- WHEN a user attempts to create a Gateway named `my-gateway`
- THEN the API server SHALL return a conflict error
- AND the Gateway SHALL NOT be created

---

### Requirement: Client Role Provisioning

After creating the OIDC client, the API server SHALL create two client-scoped roles via the Admin REST API (`POST /admin/realms/{realm}/clients/{client-uuid}/roles`).

| Role Name | Purpose |
|---|---|
| `openshell-admin` | Full administrative access to the gateway |
| `openshell-user` | Standard user access to the gateway |

#### Scenario: Client roles created

- GIVEN a Keycloak client has been created for gateway `my-gateway`
- WHEN the API server provisions client roles
- THEN it SHALL create the `openshell-admin` role on the `my-gateway` client
- AND it SHALL create the `openshell-user` role on the `my-gateway` client

---

### Requirement: Protocol Mapper Provisioning

After creating the client and roles, the API server SHALL create three protocol mappers on the OIDC client via the Admin REST API (`POST /admin/realms/{realm}/clients/{client-uuid}/protocol-mappers/models`).

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
- WHEN the API server provisions protocol mappers
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

### Requirement: Admin Users Field

The Gateway create request SHALL accept a required `admin_users` field — a list of Keycloak usernames who will receive the `openshell-admin` client role on the provisioned gateway. The list MUST contain at least one entry.

| Field | Type | Required | Description |
|---|---|---|---|
| `admin_users` | string[] | Yes | Keycloak usernames to assign `openshell-admin`. Must have at least one entry. |

The `admin_users` field SHALL be persisted on the Gateway resource and included in Gateway responses.

#### Scenario: UI auto-populates admin_users

- GIVEN an authenticated user with identity `user-a` opens the gateway provisioning form
- WHEN the UI renders the form
- THEN `admin_users` SHALL be pre-populated with `["user-a"]` (the authenticated user's identity)
- AND the user MAY add or remove entries before submitting

#### Scenario: Programmatic creation with explicit admin_users

- GIVEN a CI pipeline creates a gateway via `curl` or `hsctl`
- WHEN the request includes `admin_users: ["user-a", "user-b"]`
- THEN both `user-a` and `user-b` SHALL receive the `openshell-admin` role
- AND both `user-a` and `user-b` SHALL see the gateway in their gateway list

#### Scenario: Empty admin_users rejected

- GIVEN a gateway create request with `admin_users: []` or without `admin_users`
- WHEN the API server validates the request
- THEN it SHALL return a validation error
- AND the gateway SHALL NOT be created

---

### Requirement: Admin Role Assignment

After provisioning the client, roles, and mappers, the API server SHALL assign the `openshell-admin` client role to every user listed in `admin_users`.

For each username in `admin_users`, the API server SHALL:
1. Look up the user in Keycloak (`GET /admin/realms/{realm}/users?username={username}`)
2. Retrieve the `openshell-admin` role UUID (`GET /admin/realms/{realm}/clients/{client-uuid}/roles?search=openshell-admin`)
3. Assign the role to the user (`POST /admin/realms/{realm}/users/{user-uuid}/role-mappings/clients/{client-uuid}`)

#### Scenario: Admin role assigned to all listed users

- GIVEN a gateway create request with `admin_users: ["user-a", "user-b"]`
- WHEN the API server completes Keycloak provisioning
- THEN both `user-a` and `user-b` SHALL have the `openshell-admin` role on the gateway's client
- AND both SHALL be able to obtain tokens with `hypershell.roles: ["openshell-admin"]`

#### Scenario: User in admin_users not found in Keycloak

- GIVEN a gateway create request with `admin_users: ["user-a", "nonexistent-user"]`
- WHEN the API server looks up `nonexistent-user` in Keycloak and finds no match
- THEN the API server SHALL return an error identifying the unresolvable username
- AND the Gateway SHALL NOT be created
- AND any partially provisioned Keycloak resources SHALL be rolled back

---

### Requirement: Auto-Populated OIDC Configuration

After successful Keycloak provisioning, the API server SHALL automatically populate the Gateway resource's `oidc` field with values derived from the provisioned client. The user SHALL NOT supply OIDC configuration when creating a gateway — it is system-managed.

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

The API server SHALL scope gateway visibility by `admin_users` membership. A user SHALL only see and operate on gateways where their username appears in the `admin_users` list. This is enforced at the API layer — the UI receives only the gateways the authenticated user is an admin of.

#### Scenario: List gateways returns only gateways where user is an admin

- GIVEN `gw-alpha` has `admin_users: ["user-a"]`
- AND `gw-beta` has `admin_users: ["user-a", "user-b"]`
- AND `gw-gamma` has `admin_users: ["user-b"]`
- WHEN `user-a` calls `GET /api/hypershell/v1/gateways`
- THEN the response SHALL contain `gw-alpha` and `gw-beta`
- AND the response SHALL NOT contain `gw-gamma`

#### Scenario: Get gateway where user is not an admin returns 404

- GIVEN `gw-gamma` has `admin_users: ["user-b"]`
- WHEN `user-a` calls `GET /api/hypershell/v1/gateways/{gw-gamma-id}`
- THEN the API server SHALL return 404
- AND the response SHALL NOT reveal that the gateway exists

#### Scenario: Mutate gateway where user is not an admin returns 404

- GIVEN `gw-gamma` has `admin_users: ["user-b"]`
- WHEN `user-a` calls `PATCH /api/hypershell/v1/gateways/{gw-gamma-id}` or `DELETE /api/hypershell/v1/gateways/{gw-gamma-id}`
- THEN the API server SHALL return 404
- AND no mutation SHALL occur

---

### Requirement: Keycloak Client Cleanup

When a Gateway is deleted, the API server SHALL delete the corresponding Keycloak OIDC client to prevent orphaned clients in the realm. Deleting a Keycloak client automatically cascades to its roles and protocol mappers.

#### Scenario: Gateway deletion cleans up Keycloak

- GIVEN a Gateway `my-gateway` with a corresponding Keycloak client
- WHEN the Gateway is deleted via the API
- THEN the API server SHALL look up the client by `clientId` (`GET /admin/realms/{realm}/clients?clientId=my-gateway`)
- AND it SHALL delete the client (`DELETE /admin/realms/{realm}/clients/{client-uuid}`)
- AND the client's roles and mappers SHALL be automatically removed by Keycloak

#### Scenario: Keycloak cleanup failure is non-blocking

- GIVEN a Gateway deletion request
- WHEN the Keycloak client deletion fails (e.g., network error, Keycloak unavailable)
- THEN the Gateway SHALL still be deleted from the API server database
- AND the API server SHALL log an error with the orphaned `clientId` for manual cleanup
- AND the deletion response SHALL succeed (Keycloak cleanup is best-effort on delete)

---

### Requirement: Provisioning Atomicity

Keycloak provisioning SHALL be treated as an atomic operation during gateway creation. If any provisioning step fails, the API server SHALL roll back all completed Keycloak steps and return an error without persisting the Gateway.

#### Scenario: Mapper creation fails mid-provisioning

- GIVEN the API server has created the Keycloak client and roles
- WHEN mapper creation fails
- THEN the API server SHALL delete the created client from Keycloak (cascading roles)
- AND the API server SHALL return an error to the user
- AND no Gateway SHALL be persisted in PostgreSQL

#### Scenario: Role assignment fails for one admin user

- GIVEN the API server has created the client, roles, and mappers
- AND `admin_users` contains `["user-a", "user-b"]`
- WHEN admin role assignment succeeds for `user-a` but fails for `user-b`
- THEN the API server SHALL delete the client from Keycloak (rolling back all assignments)
- AND the API server SHALL return an error identifying the failed user
- AND no Gateway SHALL be persisted in PostgreSQL

---

## Keycloak Realm Prerequisites

The following resources SHALL exist in the Keycloak `hypershell` realm before gateways can be created. These are configuration prerequisites — the API server does not create them.

1. **Service account client** — A confidential client (e.g., `hypershell-admin-sa`) with the `realm-management` client role `realm-admin` or equivalent permissions to create clients, roles, mappers, and manage user role mappings.

2. **`gateway-roles` client scope** — A client scope containing an `oidc-usermodel-client-role-mapper` that emits `resource_access.${client_id}.roles`. This scope SHALL be included in `defaultClientScopes` for every provisioned gateway client so client roles appear in tokens.

3. **User accounts** — Users who create gateways must have accounts in the Keycloak realm so the API server can look them up and assign roles.

---

## Keycloak Admin API Call Sequence

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

7. **Get openshell-admin role UUID:**
   ```
   GET /admin/realms/{realm}/clients/{client-uuid}/roles?search=openshell-admin
   ```

8. **For each username in `admin_users`:**

   a. **Look up user:**
      ```
      GET /admin/realms/{realm}/users?username={username}
      ```

   b. **Assign admin role:**
      ```
      POST /admin/realms/{realm}/users/{user-uuid}/role-mappings/clients/{client-uuid}
      [{"name": "openshell-admin", "id": "{role-uuid}"}]
      ```

For gateway deletion:

1. **Look up client by clientId:**
   ```
   GET /admin/realms/{realm}/clients?clientId={gateway-name}
   ```

2. **Delete client (cascades roles and mappers):**
   ```
   DELETE /admin/realms/{realm}/clients/{client-uuid}
   ```

---

## Data Model Changes

The Gateway kind in `data-model.spec.md` SHALL include an `admin_users` field:

```
Gateway {
    ...existing fields...
    string[] admin_users "non-null - Keycloak usernames assigned openshell-admin on the gateway's client"
}
```

Database migration:

```sql
ALTER TABLE gateways ADD COLUMN admin_users TEXT[] NOT NULL DEFAULT '{}';
```

The `admin_users` column SHALL be indexed (GIN) to support efficient membership queries:

```sql
CREATE INDEX idx_gateways_admin_users ON gateways USING GIN (admin_users);
```

---

## Key Gotchas

| Pitfall | Detail | Mitigation |
|---|---|---|
| `fullScopeAllowed` defaults to `true` | Leaks every client's roles into every token, breaking per-gateway isolation | Always set `fullScopeAllowed = false` explicitly at client creation |
| Bulk realm import does not scale | Importing a realm with thousands of clients runs as a single transaction | Use incremental Admin REST API calls for each gateway |
| Audience resolve mapper leaks audiences | The built-in `oidc-audience-resolve-mapper` adds all clients' IDs to `aud` when `fullScopeAllowed = true` | Omit the audience-resolve mapper from the realm; use per-client audience mappers |
| User lookup by subject | Keycloak user search uses `username`, which may differ from the OIDC `sub` claim depending on identity provider federation | Ensure Keycloak user identity attributes align with the authentication token's `sub` claim |

---

## References

- [Keycloak Admin REST API](https://www.keycloak.org/docs-api/latest/rest-api/) — client, role, mapper, and user management endpoints
- [Keycloak Client Scope Configuration](https://www.keycloak.org/docs/latest/server_admin/#_client_scopes) — gateway-roles scope setup
- [Keycloak Protocol Mappers](https://www.keycloak.org/docs/latest/server_admin/#_protocol-mappers) — audience, sub, and client-role mapper types
- [PKCE (RFC 7636)](https://datatracker.ietf.org/doc/html/rfc7636) — S256 challenge method for public clients
- [Multi-Gateway OIDC Isolation](https://gist.github.com/jhjaggars/e17c2b094008c14682e3b448eca405eb) — scale testing and isolation verification for per-gateway Keycloak provisioning
