# OpenShell Gateway Keycloak Provisioning Specification

**Date:** 2026-08-11
**Status:** Draft
**Parent:** `openshell-gateway.spec.md` - core gateway provisioning
**Related:** `openshell-gateway-oidc.spec.md` - OIDC configuration injection into gateway.toml; `security/security.spec.md` - secret management and isolation; `data-model.spec.md` - Gateway kind definition; `web-console/architecture.spec.md` - gateway visibility; `security/rbac-enforcement.spec.md` - scope-aware RBAC model, role hierarchy, and Gateway OIDC Role Bridge
**Upstream:** [Keycloak Admin REST API](https://www.keycloak.org/docs-api/latest/rest-api/)

---

## Purpose

This specification defines automated per-gateway Keycloak OIDC client provisioning. Keycloak integration has two distinct lifecycles:

1. **Client lifecycle** -- When a gateway is created, the control plane provisions a dedicated OIDC client in Keycloak with client-scoped roles and protocol mappers, and populates the gateway's OIDC configuration. When a gateway is deleted, the control plane deletes the Keycloak client. This lifecycle is tied to Gateway ADDED/DELETED events.

2. **Role assignment lifecycle** -- When a user's per-gateway RoleBinding changes (created or deleted), the control plane propagates that change to the corresponding Keycloak client role on that gateway. This implements the Gateway OIDC Role Bridge defined in [`rbac-enforcement.spec.md`](../security/rbac-enforcement.spec.md). The mapping is per-gateway: a `gateway:owner` binding on gw-1 results in an `openshell-admin` Keycloak client role assignment on gw-1.

This establishes per-gateway authentication isolation: each gateway has its own audience, roles, and token claims. Visibility and access control are RBAC concerns defined in the RBAC spec -- this spec defines how RBAC decisions are projected into Keycloak role assignments so that users who connect to gateways directly via the `openshell` CLI receive the same access level as in the management plane.

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
        |  1. Authorizes via RBAC (caller must have gateway:creator role)
        |  2. Persists Gateway with fleet_id (oidc field empty at this point)
        |  3. Auto-provisions gateway:owner RoleBinding for the creator (same transaction)
        |  4. Emits gRPC watch event
        v
    Control Plane - GatewayReconciler
        |  1. Receives Gateway ADDED event
        |  2. Provisions Keycloak via Admin REST API:
        |     a. Creates OIDC client (clientId = "{name}-{id}" for uniqueness)
        |     b. Creates client roles (openshell-admin, openshell-user)
        |     c. Creates protocol mappers (audience, sub, client-roles)
        |  3. Populates Gateway oidc config (PATCH via API or internal state)
        |  4. Deploys gateway K8s resources
        |
        |  Note: The creator's gateway:owner binding is assigned via the
        |  OIDC Role Bridge (RoleBinding event path below), not inline.
        v
    Gateway Pod
        |  Validates JWTs against the provisioned Keycloak client
        v
    Authorized Access (admin/user roles scoped to this gateway only)


OIDC Role Bridge (RoleBinding ADDED/DELETED events):

    Caller (UI, hsctl, gateway owner)
        |  POST /api/hypershell/v1/role_bindings
        |  Body: { role_id, scope: "gateway", user_id, gateway_id }
        v
    API Server
        |  1. Authorizes (caller must be gateway:owner on the target gateway)
        |  2. Persists RoleBinding
        |  3. Emits gRPC watch event
        v
    Control Plane
        |  1. Receives RoleBinding ADDED/DELETED event
        |  2. Maps the HyperShell role to a Keycloak client role:
        |     - gateway:owner → openshell-admin
        |     - gateway:viewer → openshell-user
        |  3. Resolves the gateway from the binding's gateway_id
        |  4. On the gateway's Keycloak client:
        |     - ADDED:   assigns the Keycloak client role to the user
        |     - DELETED: removes the Keycloak client role from the user
        v
    User can now obtain tokens with the assigned role for that gateway
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
    |   +-- Reads Secret (optional, for user validation at RoleBinding creation)
    |
    +-- Control Plane Pod
        +-- Reads Secret -> provisions Keycloak clients, roles, mappers (read-write)
        +-- Reads Secret -> assigns/removes Keycloak client roles on RoleBinding events (read-write)
```

### Per-Gateway Isolation Model

```
user-a has gateway:creator (from Keycloak)
user-a creates gw-alpha (id=abc123) → auto gateway:owner on gw-alpha
user-a creates gw-shared (id=def456) → auto gateway:owner on gw-shared
    KC clients: gw-alpha-abc123, gw-shared-def456
    openshell-admin → user-a (on both, via gateway:owner)

gateway:owner user-a grants gateway:viewer to user-b on gw-alpha
    openshell-user → user-b (on gw-alpha-abc123 only)

gateway:owner user-a grants gateway:owner to user-b on gw-shared
    openshell-admin → user-b (on gw-shared-def456 only)

user-c has gateway:creator (from Keycloak)
user-c creates gw-beta (id=ghi789) → auto gateway:owner on gw-beta
    KC client: gw-beta-ghi789
    openshell-admin → user-c

    user-a GET /gateways → [gw-alpha, gw-shared]   (gateway:owner on both)
    user-b GET /gateways → [gw-alpha, gw-shared]   (gateway:viewer on gw-alpha, gateway:owner on gw-shared)
    user-c GET /gateways → [gw-beta]                (gateway:owner on gw-beta)
```

Each gateway's Keycloak client has `fullScopeAllowed = false`, preventing role leakage across gateways. A token obtained for `gw-alpha` contains only `gw-alpha`'s roles and audience -- never `gw-beta`'s.

Gateway visibility is an RBAC concern -- a user sees gateways where they have a RoleBinding. The RBAC spec defines scope-aware list filtering; this spec defines only the Keycloak projection that enables gateway-level token validation.

---

## RBAC Role Bridge

This specification implements the **Gateway OIDC Role Bridge** defined in [`rbac-enforcement.spec.md`](../security/rbac-enforcement.spec.md). The bridge ensures that users who connect to gateways directly via the `openshell` CLI receive the same access level as in the HyperShell management plane.

### Role Mapping

The control plane maps HyperShell RBAC roles to per-gateway Keycloak client roles:

| HyperShell Role | Keycloak Client Role | Scope |
|---|---|---|
| `gateway:owner` | `openshell-admin` | The specific bound gateway |
| `gateway:viewer` | `openshell-user` | The specific bound gateway |

`gateway:creator` is not mapped -- it grants the ability to create gateways but does not confer access to any specific gateway. The creator automatically receives a `gateway:owner` RoleBinding on creation (see [`rbac-enforcement.spec.md`](../security/rbac-enforcement.spec.md)), which provides the Keycloak role through the mapping above.

### Effective Role Resolution

When a user has multiple RoleBindings on the same gateway, the **highest-privilege** Keycloak client role wins. A user with both `gateway:viewer` (→ `openshell-user`) and `gateway:owner` (→ `openshell-admin`) on the same gateway SHALL have the `openshell-admin` client role.

### Lifecycle Interactions

- **Gateway created** → the control plane provisions the Keycloak client and resolves existing RoleBindings for that gateway (initially the auto-provisioned `gateway:owner` for the creator), assigning the corresponding Keycloak client roles.
- **RoleBinding created/deleted for a gateway** → the control plane assigns or removes the Keycloak client role on that gateway's Keycloak client.
- **Gateway deleted** → Keycloak client deletion cascades all role assignments automatically (see Client Cleanup requirement).

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

#### Client ID Format

The Keycloak `clientId` SHALL be `{name}-{id}`, where `{name}` is the user-visible gateway name and `{id}` is the API-server resource ID (KSUID). This prevents name clashes when multiple gateways share the same name across fleets or when a gateway is deleted and recreated with the same name. Example: a gateway named `my-gateway` with ID `2FhMpQzXBz` produces `clientId = "my-gateway-2FhMpQzXBz"`.

The client SHALL be created with the following properties:

| Property | Value | Notes |
|---|---|---|
| `clientId` | `{name}-{id}` | Unique within the realm; used as OIDC `client_id` and `aud` |
| `name` | `{name}-{id}` | Display name in Keycloak admin console |
| `publicClient` | `true` | PKCE flow, no client secret required |
| `standardFlowEnabled` | `true` | Authorization code flow for browser/CLI |
| `directAccessGrantsEnabled` | `true` | Resource owner password grant for non-interactive CI pipelines that cannot use browser-based PKCE flow |
| `fullScopeAllowed` | `false` | **CRITICAL** -- prevents cross-gateway role leakage |
| `redirectUris` | `["http://127.0.0.1:*", "http://localhost:*"]` | CLI callback URIs |
| `attributes.pkce.code.challenge.method` | `S256` | PKCE challenge method |
| `attributes.oauth2.device.authorization.grant.enabled` | `true` | Enables OAuth 2.0 Device Authorization Grant for browserless CLI authentication |
| `defaultClientScopes` | `["openid", "profile", "email", "roles", "gateway-roles", "web-origins", "acr"]` | Standard scopes plus `gateway-roles` |

> **`fullScopeAllowed` MUST be `false`.** Keycloak defaults to `true`, which leaks every client's roles into every token. Combined with the built-in `oidc-audience-resolve-mapper`, a token from any client would carry all other gateways' client IDs in `aud` plus their admin roles, breaking per-gateway isolation entirely.

The `gateway-roles` client scope is a realm prerequisite. It SHALL contain an `oidc-usermodel-client-role-mapper` that emits client roles under `resource_access.${client_id}.roles`. This scope SHALL exist in the Keycloak realm before gateways are created.

After creating the client, the creator's `gateway:owner` role is assigned via the OIDC Role Bridge (RoleBinding event path). The API server creates the RoleBinding in the same transaction as the gateway, and the RoleBindingReconciler picks it up via the watch stream. The RoleBindingReconciler retries with exponential backoff if the Keycloak client is not yet provisioned (see Role Assignment Retry below).

#### Scenario: Reconciler provisions Keycloak client

- GIVEN the GatewayReconciler receives an ADDED event for Gateway `my-gateway` (id=`2FhMpQzXBz`)
- WHEN the reconciler provisions Keycloak
- THEN it SHALL create a Keycloak client with `clientId = "my-gateway-2FhMpQzXBz"`
- AND the client SHALL have `fullScopeAllowed = false`
- AND the client SHALL have `publicClient = true` with `pkce.code.challenge.method = S256`
- AND OAuth 2.0 Device Authorization Grant SHALL be enabled
- AND the `gateway-roles` client scope SHALL be included in `defaultClientScopes`

#### Scenario: Reconciler enables device authorization on an existing client

- GIVEN a gateway's Keycloak client was provisioned before Device Authorization Grant was enabled
- WHEN the GatewayReconciler reconciles the existing gateway
- THEN it SHALL enable OAuth 2.0 Device Authorization Grant on the existing Keycloak client
- AND it SHALL preserve all other client attributes and settings
- AND subsequent reconciliations SHALL NOT update the client when the grant is already enabled

#### Scenario: Creator receives admin role on new gateway

- GIVEN user-a has `gateway:creator` (from Keycloak) and creates Gateway `gw-new` (id=`xyz789`)
- AND the API server auto-provisions a `gateway:owner` RoleBinding for user-a on `gw-new`
- WHEN the RoleBindingReconciler receives the RoleBinding ADDED event
- THEN it SHALL resolve the Keycloak client ID as `gw-new-xyz789`
- AND it SHALL assign `openshell-admin` to user-a on the `gw-new-xyz789` client
- NOTE: If the Keycloak client is not yet provisioned, the reconciler retries with exponential backoff (see Role Assignment Retry)

#### Scenario: Duplicate client ID in Keycloak

- GIVEN a Keycloak client with `clientId = "my-gateway-2FhMpQzXBz"` already exists in the realm
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
| `config.included.client.audience` | `{clientId}` (e.g., `my-gateway-2FhMpQzXBz`) |
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
| `config.usermodel.clientRoleMapping.clientId` | `{clientId}` (e.g., `my-gateway-2FhMpQzXBz`) |

#### Scenario: All mappers provisioned

- GIVEN a Keycloak client `my-gateway-2FhMpQzXBz` has been created with roles
- WHEN the GatewayReconciler provisions protocol mappers
- THEN the audience mapper SHALL set `aud` to `my-gateway-2FhMpQzXBz` in access tokens
- AND the sub mapper SHALL ensure `sub` is present in access tokens
- AND the client-roles mapper SHALL map `resource_access.my-gateway-2FhMpQzXBz.roles` to the `hypershell.roles` claim

#### Scenario: Token contains correct claims after provisioning

- GIVEN user `user-a` has the `openshell-admin` role on client `my-gateway-2FhMpQzXBz`
- WHEN `user-a` obtains a token using `client_id = my-gateway-2FhMpQzXBz`
- THEN the access token SHALL contain `aud: "my-gateway-2FhMpQzXBz"`
- AND the access token SHALL contain `sub: "user-a-sub-id"`
- AND the access token SHALL contain `hypershell.roles: ["openshell-admin"]`
- AND the access token SHALL NOT contain roles from any other gateway's client

---

### Requirement: RBAC-Driven Keycloak Role Assignment (OIDC Role Bridge)

Keycloak client role assignments SHALL be driven by RoleBinding events, implementing the Gateway OIDC Role Bridge defined in [`rbac-enforcement.spec.md`](../security/rbac-enforcement.spec.md). The control plane SHALL assign or remove Keycloak client roles when RoleBindings are created or deleted, using the role mapping defined in the RBAC Role Bridge section above.

All per-gateway RoleBindings (`gateway:owner`, `gateway:viewer`) have `scope=gateway` and reference a specific `gateway_id`. The `gateway:creator` role is global-scoped and sourced from Keycloak JWT claims -- it does not participate in the OIDC Role Bridge.

#### RoleBinding ADDED

For each RoleBinding ADDED event with `scope=gateway`, the control plane SHALL:
1. Map the HyperShell role to a Keycloak client role (`openshell-admin` or `openshell-user`)
2. Resolve the gateway from the binding's `gateway_id` and compute the Keycloak client ID (`{name}-{id}`)
3. Resolve the User's Keycloak identity (the `username` from the User record, which is populated from the JWT `preferred_username` claim at auto-provisioning time)
4. Look up the user in Keycloak (`GET /admin/realms/{realm}/users?username={username}`)
5. On the gateway's Keycloak client:
   - Retrieve the client role UUID
   - Assign the role to the user (`POST /admin/realms/{realm}/users/{user-uuid}/role-mappings/clients/{client-uuid}`)
6. If the Keycloak client does not yet exist (race with gateway provisioning), retry with exponential backoff (see Role Assignment Retry)

#### RoleBinding DELETED

For each RoleBinding DELETED event with `scope=gateway`, the control plane SHALL:
1. Resolve the gateway from the binding's `gateway_id`
2. Check whether the user has any remaining RoleBindings on this gateway
3. If the user still has a binding that maps to the same or higher Keycloak role, take no action
4. If the user has a remaining binding that maps to a lower role, downgrade the Keycloak client role
5. If no remaining bindings exist, remove the Keycloak client role mapping (`DELETE /admin/realms/{realm}/users/{user-uuid}/role-mappings/clients/{client-uuid}`)

#### Scenario: Gateway owner receives admin role

- GIVEN gateway `gw-alpha` (id=`abc123`) exists with a provisioned Keycloak client `gw-alpha-abc123`
- AND a User `user-a` exists in both HyperShell and Keycloak
- WHEN a RoleBinding is created: `role=gateway:owner`, `scope=gateway`, `gateway_id=abc123`, `user_id=user-a`
- THEN the control plane SHALL assign `openshell-admin` to `user-a` on the `gw-alpha-abc123` Keycloak client
- AND `user-a` SHALL be able to obtain tokens with `hypershell.roles: ["openshell-admin"]` for `gw-alpha-abc123`

#### Scenario: Gateway viewer receives user role

- GIVEN gateway `gw-alpha` (id=`abc123`) exists with a provisioned Keycloak client `gw-alpha-abc123`
- WHEN a RoleBinding is created: `role=gateway:viewer`, `scope=gateway`, `gateway_id=abc123`, `user_id=user-b`
- THEN the control plane SHALL assign `openshell-user` to `user-b` on the `gw-alpha-abc123` Keycloak client only

#### Scenario: RoleBinding deletion with remaining coverage

- GIVEN user-a has `gateway:owner` (→ `openshell-admin`) on gw-alpha
- AND user-a also has `gateway:viewer` (→ `openshell-user`) on gw-alpha
- WHEN the `gateway:owner` RoleBinding is deleted
- THEN the control plane SHALL downgrade user-a to `openshell-user` on gw-alpha
- AND user-a SHALL NOT lose Keycloak roles entirely (the viewer binding still covers them)

#### Scenario: RoleBinding deletion with no remaining coverage

- GIVEN user-a has only `gateway:viewer` (→ `openshell-user`) on gw-alpha
- WHEN the `gateway:viewer` RoleBinding is deleted
- THEN the control plane SHALL remove `openshell-user` from user-a on the `gw-alpha` Keycloak client

#### Scenario: User in RoleBinding not found in Keycloak

- GIVEN a RoleBinding references a User whose Keycloak identity cannot be resolved
- WHEN the control plane attempts to assign the Keycloak client role
- THEN it SHALL log an error identifying the unresolvable user
- AND it SHALL retry on the next reconciliation cycle

---

### Requirement: Auto-Populated OIDC Configuration

After successful Keycloak provisioning, the GatewayReconciler SHALL populate the Gateway resource's `oidc` field with values derived from the provisioned client and the Keycloak service account configuration. The user SHALL NOT supply OIDC configuration when creating a gateway -- it is system-managed.

The auto-populated OIDC values SHALL be:

| OIDC Field | Value | Source |
|---|---|---|
| `issuer` | `{server-url}/realms/{realm}` | Keycloak service account config |
| `audience` | `{clientId}` | Provisioned client ID (`{name}-{id}`) |
| `jwks_ttl` | `3600` | Default |
| `roles_claim` | `hypershell.roles` | Fixed claim path from client-roles mapper |
| `admin_role` | `openshell-admin` | Fixed role name |
| `user_role` | `openshell-user` | Fixed role name |
| `scopes_claim` | `""` | Default empty -- upstream gateway configuration field |

The OIDC fields on the Gateway resource SHALL be read-only -- not settable or updatable via the REST or gRPC API. The control plane SHALL inject these values into `gateway.toml` using the existing OIDC injection behavior defined in `openshell-gateway-oidc.spec.md`.

#### Scenario: Gateway created with auto-populated OIDC

- GIVEN a user creates a Gateway named `my-gateway` (assigned id=`2FhMpQzXBz`)
- AND the Keycloak realm is at `https://keycloak.example.com/realms/hypershell`
- WHEN Keycloak provisioning succeeds
- THEN the persisted Gateway's `oidc` field SHALL be:
  ```json
  {
    "issuer": "https://keycloak.example.com/realms/hypershell",
    "client_id": "my-gateway-2FhMpQzXBz",
    "audience": "my-gateway-2FhMpQzXBz",
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
  audience      = "my-gateway-2FhMpQzXBz"
  jwks_ttl_secs = 3600
  roles_claim   = "hypershell.roles"
  admin_role    = "openshell-admin"
  user_role     = "openshell-user"
  ```

---

### Requirement: Gateway Visibility Scoping

Gateway visibility is defined by the RBAC spec's scope-aware list filtering. The API server SHALL return only gateways where the caller has a RoleBinding (`gateway:owner` or `gateway:viewer`). This is enforced at the API layer by the RBAC authorization middleware; the UI receives only the gateways the authenticated user has access to.

This keycloak spec does not define the visibility query mechanism -- that is the responsibility of [`rbac-enforcement.spec.md`](../security/rbac-enforcement.spec.md). This spec defines only the Keycloak projection: when a user has a per-gateway RoleBinding, they also have the corresponding Keycloak client role, enabling them to obtain valid tokens for that gateway.

#### Scenario: List gateways returns only accessible gateways

- GIVEN user-a has `gateway:owner` on `gw-alpha`
- AND user-a has no RoleBinding on `gw-beta`
- WHEN `user-a` calls `GET /api/hypershell/v1/gateways`
- THEN the response SHALL contain `gw-alpha`
- AND the response SHALL NOT contain `gw-beta`

#### Scenario: Get inaccessible gateway returns 404

- GIVEN user-a has no RoleBinding on `gw-beta`
- WHEN `user-a` calls `GET /api/hypershell/v1/gateways/{gw-beta-id}`
- THEN the API server SHALL return 404
- AND the response SHALL NOT reveal that the gateway exists

---

### Requirement: Keycloak Client Cleanup

When the GatewayReconciler receives a Gateway DELETED event, it SHALL delete the corresponding Keycloak OIDC client to prevent orphaned clients in the realm. Deleting a Keycloak client automatically cascades to its roles, protocol mappers, and all user role assignments on that client.

#### Scenario: Gateway deletion cleans up Keycloak

- GIVEN a Gateway `my-gateway` (id=`2FhMpQzXBz`) with a corresponding Keycloak client `my-gateway-2FhMpQzXBz`
- WHEN the GatewayReconciler receives a DELETED event for the Gateway
- THEN it SHALL look up the client by `clientId` (`GET /admin/realms/{realm}/clients?clientId=my-gateway-2FhMpQzXBz`)
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

### Requirement: Role Assignment Retry

The RoleBinding event and the Gateway event are emitted by the API server in the same transaction. Because the control plane processes watch events concurrently, the RoleBindingReconciler MAY attempt to assign a Keycloak client role before the GatewayReconciler has finished provisioning the Keycloak client. The RoleBindingReconciler SHALL handle this race by retrying role assignment with exponential backoff.

The retry policy SHALL be:
- Maximum attempts: 10
- Initial backoff: 2 seconds
- Backoff multiplier: 2x per attempt
- Maximum backoff: 30 seconds
- Context-aware: retries SHALL stop immediately if the context is cancelled

Each retry attempt SHALL be logged at INFO level with the attempt number, target client, and error. A successful assignment after retry SHALL be logged at INFO level. Exhaustion of all retry attempts SHALL return the last error to the watcher, which logs it at ERROR level.

#### Scenario: RoleBinding arrives before Keycloak client exists

- GIVEN user-a creates Gateway `gw-new` (id=`xyz789`)
- AND the API server emits both a Gateway ADDED event and a RoleBinding ADDED event
- AND the RoleBindingReconciler processes the RoleBinding event first
- WHEN the RoleBindingReconciler attempts to assign `openshell-admin` to user-a on `gw-new-xyz789`
- AND the Keycloak client `gw-new-xyz789` does not yet exist
- THEN the RoleBindingReconciler SHALL retry with exponential backoff
- AND it SHALL succeed once the GatewayReconciler provisions the Keycloak client
- AND user-a SHALL have the `openshell-admin` role on the `gw-new-xyz789` client

#### Scenario: Keycloak client never provisioned

- GIVEN the GatewayReconciler fails to provision the Keycloak client (e.g., Keycloak is down)
- WHEN the RoleBindingReconciler exhausts all retry attempts
- THEN it SHALL log the failure at ERROR level
- AND the role assignment SHALL be retried on the next RoleBinding event for this binding

---

## Keycloak Realm Prerequisites

The following resources SHALL exist in the configured Keycloak realm (as specified by the `realm` key in the `hypershell-keycloak-admin` Secret) before gateways can be created. These are configuration prerequisites -- neither the API server nor the control plane creates them.

1. **Service account client** -- A confidential client (e.g., `hypershell-admin-sa`) with the `realm-management` client role `realm-admin` or equivalent permissions to create clients, roles, mappers, and manage user role mappings.

2. **`gateway-roles` client scope** -- A client scope containing an `oidc-usermodel-client-role-mapper` that emits `resource_access.${client_id}.roles`. This scope SHALL be included in `defaultClientScopes` for every provisioned gateway client so client roles appear in tokens.

3. **User accounts** -- Users referenced by RoleBindings must have accounts in the Keycloak realm so the control plane can assign roles. Since HyperShell auto-provisions User records from JWT claims (see RBAC spec), Keycloak user accounts are expected to exist as a precondition of authentication.

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
   Body includes `clientId` (`{name}-{id}`), `publicClient: true`, `fullScopeAllowed: false`, `defaultClientScopes`, PKCE attribute, `redirectUris`, and inline audience mapper via `protocolMappers`.

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

7. **Resolve existing gateway-scoped RoleBindings and assign Keycloak roles** (see RoleBinding Creation below for per-user sequence)

### RoleBinding Creation (OIDC Role Bridge)

When a gateway-scoped RoleBinding is created:

1. **Resolve gateway name and compute Keycloak client ID** (`{name}-{id}`), **then get the Keycloak client UUID:**
   ```
   GET /admin/realms/{realm}/clients?clientId={name}-{id}
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

When a gateway-scoped RoleBinding is deleted (after checking no remaining coverage on that gateway):

1. **Remove role mapping:**
   ```
   DELETE /admin/realms/{realm}/users/{user-uuid}/role-mappings/clients/{client-uuid}
   [{"name": "{role-name}", "id": "{role-uuid}"}]
   ```

### Gateway Deletion (Client Cleanup)

1. **Look up client by clientId:**
   ```
   GET /admin/realms/{realm}/clients?clientId={name}-{id}
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
| Gateway created with auto-provisioned RoleBinding | Race between Gateway ADDED and RoleBinding ADDED events (both emitted from the same API transaction) | RoleBindingReconciler retries role assignment with exponential backoff until the Keycloak client is provisioned |
| RoleBinding deletion requires coverage check | Removing one binding may leave the user covered by another | Resolve all remaining bindings for the user+gateway before removing the Keycloak role |
| Gateway deletion cascades role assignments | Deleting a Keycloak client removes all user role assignments for that client | RBAC RoleBindings in HyperShell are also deleted when the gateway is deleted (gateway_id FK) |

---

## References

- [Keycloak Admin REST API](https://www.keycloak.org/docs-api/latest/rest-api/) -- client, role, mapper, and user management endpoints
- [Keycloak Client Scope Configuration](https://www.keycloak.org/docs/latest/server_admin/#_client_scopes) -- gateway-roles scope setup
- [Keycloak Protocol Mappers](https://www.keycloak.org/docs/latest/server_admin/#_protocol-mappers) -- audience, sub, and client-role mapper types
- [PKCE (RFC 7636)](https://datatracker.ietf.org/doc/html/rfc7636) -- S256 challenge method for public clients
- [Multi-Gateway OIDC Isolation](https://gist.github.com/jhjaggars/e17c2b094008c14682e3b448eca405eb) -- scale testing and isolation verification for per-gateway Keycloak provisioning
