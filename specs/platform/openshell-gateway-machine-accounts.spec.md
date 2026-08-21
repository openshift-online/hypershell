# OpenShell Gateway Machine Accounts (Client Secret) Specification

**Date:** 2026-08-21
**Status:** Draft
**Tracks:** [HYPERSHELL-49](https://redhat.atlassian.net/browse/HYPERSHELL-49) - Service account provisioning via federated Keycloak
**Parent:** `openshell-gateway-keycloak.spec.md` - per-gateway Keycloak clients and role mapping
**Related:** `openshell-gateway-oidc.spec.md`, `security/rbac-enforcement.spec.md`, `standards/security/security.spec.md`, and `data-model.spec.md`
**Upstream:** [Keycloak service accounts](https://www.keycloak.org/docs/latest/server_admin/#_service_accounts), [Keycloak Admin REST API](https://www.keycloak.org/docs-api/latest/rest-api/), and [OpenShell gateway authentication](https://docs.nvidia.com/openshell/reference/gateway-auth)

---

## Purpose

This specification defines MachineAccounts for non-interactive access to an OpenShell gateway. Each MachineAccount uses OAuth 2.0 Client Credentials.

A user selects a gateway, an OpenShell role, and an expiration time. HyperShell returns a Keycloak client ID and a new client secret. It returns the client secret only once. Automation exchanges these client credentials for short-lived JWT access tokens.

HyperShell derives the gateway audience. It also limits the selected role to the creator's current gateway role. Callers cannot set audiences, claims, or access-token lifetimes.

Version 1 does not support WIF, signed client assertions, opaque access tokens, personal access tokens, or multi-gateway client credentials. It also excludes configurable OpenShell scopes and automatic workspace-membership management.

## Product Decisions

| Question | Decision |
|---|---|
| What does HyperShell create in Keycloak? | One confidential OIDC client with an enabled service account for each MachineAccount. |
| What does automation store? | A client ID and an opaque client secret. HyperShell returns the secret once. Neither value is a JWT. |
| What does CI use at the gateway? | A short-lived signed JWT. Keycloak issues it on demand through `grant_type=client_credentials`. |
| Can a bearer JWT last 12 months? | No. A MachineAccount can be valid for up to 365 days, but each access token lasts five minutes by default. |
| How is an access token limited to one gateway? | Its `aud` claim contains only the selected gateway client ID. The machine client has no role scope for another gateway. |
| Can the access token call the HyperShell management API? | No. This slice of HYPERSHELL-49 authorizes the selected OpenShell gateway only. |
| What role can a user select? | `openshell-user` or `openshell-admin`. An owner may select either role. A viewer may select only `openshell-user`. |
| Can one set of client credentials cover multiple gateways? | No. Automation needs one MachineAccount and one audience-specific access token for each gateway. |
| Does `openshell-user` inherit workspace access? | No. An OpenShell gateway administrator must grant the machine subject membership in each workspace. |
| How are client credentials replaced? | Create and verify a replacement MachineAccount. Then, revoke the old MachineAccount. Delete it when you no longer need its metadata. |
| Are revoke and delete the same action? | No. Revoke permanently stops access-token issuance and retains metadata. Delete removes the Keycloak identity and the visible resource. |
| Are OpenShell scopes configurable? | No. The gateway has `scopes_claim = ""`. Roles and workspace memberships control access. |

## Terminology

- **MachineAccount** is a HyperShell resource for one automation identity on one gateway. HyperShell can revoke it independently. It is not a human user or a Kubernetes ServiceAccount.
- **Client credentials** are the client ID and client secret that authenticate the MachineAccount to Keycloak. The client ID is not secret. The client secret is opaque.
- **Credential bundle** is the one-time `credential` object in the create response. It contains the client credentials and non-secret connection metadata.
- **Access token** is the short-lived JWT that Keycloak returns after a successful Client Credentials grant. The OpenShell gateway validates it locally.
- **Creator** is the authenticated HyperShell user that creates the MachineAccount. The MachineAccount remains limited by this user's gateway role. A creator cannot transfer it.
- **Gateway client** is the existing public Keycloak client `{gateway-name}-{gateway-id}`. Its client ID is the configured gateway audience.
- **Machine client** is the confidential Keycloak client for one MachineAccount. Keycloak uses it to issue access tokens for one gateway audience.
- **Reconcile** means compare the MachineAccount record with Keycloak and correct differences.
- **Soft-delete** means set `deleted_at` and hide the record from normal reads. It does not immediately remove the database row.

## Architecture

### Provisioning and Use

```text
User (management OIDC session)
    |
    | POST /api/hypershell/v1/gateways/{gateway_id}/machine_accounts
    | { name, description?, role, expires_at? }
    v
HyperShell API
    | 1. Resolve the caller's gateway RoleBinding
    | 2. Cap role at owner -> openshell-admin, viewer -> openshell-user
    | 3. Reserve non-secret MachineAccount metadata in provisioning state
    | 4. Create a confidential Keycloak client and service-account user
    | 5. Scope and assign only the target gateway's roles
    | 6. Verify a test access token's audience, subject, and roles
    | 7. Mark the MachineAccount ready
    v
201 Created (Cache-Control: no-store)
    { machine_account metadata, client_id, client_secret, token_endpoint }
                         |
                         | user stores the client secret after this one response
                         v
CI job / bot
    |
    | POST {token_endpoint}
    | grant_type=client_credentials + client_id + client_secret
    v
Keycloak
    |
    | short-lived signed access JWT
    | aud = selected gateway client ID
    | sub = machine service-account user ID
    | hypershell.roles = authorized OpenShell role(s)
    v
OpenShell Gateway
    | validates signature, issuer, audience, expiration, and roles
    v
Authorized machine request
```

The client secret authenticates the MachineAccount to Keycloak. A client SHALL never send it to the gateway.

The access token is a bearer token. HyperShell SHALL not store it in the MachineAccount record. Later HyperShell responses SHALL not return it.

### Credential and Token Lifetimes

```text
MachineAccount validity period:           1 hour .. 365 days (default 90 days)
Access JWT lifetime:                       5 minutes by default (operator-configurable,
                                           absolute maximum 15 minutes)

create -------------- request access tokens --------------- expires/revokes
       |----- JWT -----|  |----- JWT -----|  |----- JWT -----|
```

Disabling or deleting a machine client prevents new access-token issuance. The gateway validates access tokens locally with the issuer's JWKS. It does not introspect each request.

An access token issued before disablement remains valid until its `exp`. Thus, the access-token lifetime sets the maximum normal revocation delay.

### Component Responsibilities

| Component | Responsibility |
|---|---|
| API server | Authorize nested routes, store non-secret metadata, call the Keycloak provisioner synchronously, and return the new secret once. |
| Keycloak provisioner | Create, verify, disable, and delete machine clients. Manage their service-account roles and client scope mappings. |
| Control plane | Reconcile gateway and console clients. Remove all related machine clients when it deletes a gateway. |
| Management web console | Create and manage MachineAccounts through the API. Show the credential bundle once without caching it. |
| HyperShell CLI | Support create, list, get, revoke, and delete operations. Pass the credential bundle to CI without logging it. |
| Keycloak | Store the machine identity and client secret. Issue short-lived access tokens. |
| OpenShell gateway | Validate access tokens locally. Require separate workspace membership for an `openshell-user` subject. |

The create operation SHALL provision the machine client synchronously. This path lets HyperShell return the client secret once.

The API server SHALL NOT place the client secret on an asynchronous gRPC watch or event path. HyperShell MAY use an API-server adapter or an authenticated internal service for provisioning. This internal choice SHALL NOT change the public API behavior.

Both implementations SHALL enforce the same authorization, rollback, one-time delivery, and redaction rules. Browsers, CLIs, and BFFs SHALL never receive Keycloak administration credentials.

## Data Model

MachineAccount is a first-class, gateway-scoped API resource.

```text
MachineAccount {
    string ID PK
    string gateway_id FK
    string name
    string description nullable
    string credential_type       "client_secret"
    string role                  "selected highest OpenShell role"
    string status                "provisioning | ready | expired | revoking | revoked | deleting | error"
    string created_by_user_id FK
    string keycloak_client_id
    string keycloak_client_uuid
    string subject               "Keycloak service-account user ID / JWT sub"
    time   expires_at
    time   revoked_at nullable
    string last_error nullable
    time   created_at
    time   updated_at
    time   deleted_at nullable
}
```

The model SHALL reference the Gateway and the creator User with foreign keys. The tuple `(gateway_id, name)` SHALL be unique among records that are not deleted.

A user-provided name is display metadata. HyperShell SHALL NOT include it in the Keycloak `clientId`. Generated resource IDs provide uniqueness and prevent identifier injection.

The database SHALL NOT contain:

- A client secret
- An access token
- A hash that Keycloak can accept as the client secret
- A serialized token-endpoint response

The model does not include `last_used_at`. The gateway validates access tokens locally and does not report each use to HyperShell. Usage reporting requires a separate audited event design.

### Schema and Migration

The API server SHALL add the MachineAccount table through a forward migration. The migration SHALL add foreign keys and indexes for status and expiration. It SHALL also add a partial unique index for active `(gateway_id, name)` values.

The migration SHALL contain no secret column. Existing Gateways and RoleBindings require no backfill. Applying only the schema migration SHALL create no Keycloak clients.

MachineAccount and Gateway deletion SHALL preserve referential integrity. They SHALL also preserve non-secret audit records.

Operators SHALL remove or reconcile all MachineAccounts before a database rollback. This step prevents the rollback from leaving an enabled machine client in Keycloak.

## REST API Contract

All endpoints are nested beneath the selected gateway.

| Method | Path | Operation |
|---|---|---|
| `POST` | `/api/hypershell/v1/gateways/{gateway_id}/machine_accounts` | Create a MachineAccount and return its credential bundle once |
| `GET` | `/api/hypershell/v1/gateways/{gateway_id}/machine_accounts` | List visible MachineAccount metadata |
| `GET` | `/api/hypershell/v1/gateways/{gateway_id}/machine_accounts/{machine_account_id}` | Get visible MachineAccount metadata |
| `POST` | `/api/hypershell/v1/gateways/{gateway_id}/machine_accounts/{machine_account_id}/revoke` | Permanently stop access-token issuance. Return `200` after disablement or `202` while disablement is pending. |
| `DELETE` | `/api/hypershell/v1/gateways/{gateway_id}/machine_accounts/{machine_account_id}` | Revoke when necessary and remove the identity. Return `204` after removal or `202` while removal is pending. |

Version 1 has no public endpoint to patch a MachineAccount, read a secret, issue an access token, re-enable an account, or transfer ownership. It also has no in-place secret-rotation endpoint.

Credential replacement uses four operations: create a new MachineAccount, update the consumer, revoke the old account, and delete the old account. Revocation is the immediate security action. Deletion removes the identity and visible resource.

The list operation SHALL accept `page` and `size` query parameters. The default page size SHALL be 20. The maximum page size SHALL be 100. The response SHALL include the total number of visible items.

The operation SHALL support `status` and `search` filters. Search SHALL match name, client ID, and subject without regard to letter case. It SHALL treat SQL wildcard and escape characters as literal text.

The API SHALL apply authorization filters before it calculates pages and totals. The default order SHALL be `created_at DESC, id DESC`.

### Create Request

```json
{
  "name": "nightly-build",
  "description": "Build and test the main branch",
  "credential_type": "client_secret",
  "role": "openshell-user",
  "expires_at": "2027-08-21T12:00:00Z"
}
```

The caller MAY omit `credential_type`. Its default is `client_secret`. The API SHALL reject every other value.

The caller MAY omit `role`. Its default is `openshell-user`. The API SHALL accept only `openshell-user` and `openshell-admin`.

The caller MAY omit `expires_at`. Its default is 90 days after creation. The value MUST be between one hour and 365 days after creation. Operators MAY configure a shorter default or maximum. The configured maximum SHALL NOT exceed 365 days.

### One-Time Credential Bundle

```json
{
  "id": "35GtY7ExampleMachineAccount",
  "gateway_id": "35GtW1ExampleGateway",
  "name": "nightly-build",
  "description": "Build and test the main branch",
  "credential_type": "client_secret",
  "role": "openshell-user",
  "status": "ready",
  "created_by_user_id": "35GtU3ExampleUser",
  "subject": "7d39dc7b-83c0-46f5-b2bf-27b93115a05f",
  "expires_at": "2027-08-21T12:00:00Z",
  "created_at": "2026-08-21T12:00:00Z",
  "credential": {
    "issuer": "https://keycloak.example.com/realms/hypershell",
    "token_endpoint": "https://keycloak.example.com/realms/hypershell/protocol/openid-connect/token",
    "grant_type": "client_credentials",
    "client_id": "hs-ma-35GtW1ExampleGateway-35GtY7ExampleMachineAccount",
    "client_secret": "<returned-once>",
    "audience": "my-gateway-35GtW1ExampleGateway",
    "access_token_lifetime_seconds": 300
  }
}
```

Only a successful `201 Created` response SHALL contain the `credential` object. This object is the credential bundle. The response SHALL include `Cache-Control: no-store` and `Pragma: no-cache`.

List and get responses SHALL omit the credential bundle. These responses SHALL expose the non-secret `keycloak_client_id` as `client_id`.

HyperShell cannot recover a lost client secret through the public API. If the caller loses the create response, the caller SHALL replace the MachineAccount.

An active duplicate name SHALL return `409 Conflict`. A network retry SHALL NOT create multiple machine clients with the same gateway and name.

If provisioning fails, HyperShell SHALL first verify whether the machine client exists. If it does not exist, HyperShell SHALL soft-delete the provisioning reservation. The caller can then retry the name.

If HyperShell cannot prove absence, it SHALL retain a visible resource with status `error`. Its `last_error` SHALL contain only a stable error code and a safe summary. It SHALL NOT contain client credentials or raw Keycloak responses.

HyperShell SHALL disable and remove the partial machine client through reconciliation.

The error resource SHALL reserve its name and count against quota until cleanup. This rule prevents creation of a second identity with an uncertain predecessor.

### Response and Error Semantics

| Condition | Response |
|---|---|
| Gateway does not exist or caller has no gateway binding | `404 Not Found` |
| Caller is a viewer and explicitly requests `openshell-admin` | `403 Forbidden` |
| Gateway exists but its OIDC client is not ready | `409 Conflict` with `gateway_not_ready` |
| Name already exists on the gateway | `409 Conflict` with `machine_account_name_exists` |
| Lifetime is outside policy | `400 Bad Request` with the allowed range |
| Active MachineAccount quota is reached | `429 Too Many Requests` |
| Keycloak is unavailable or HyperShell cannot verify the result | `503 Service Unavailable`. The response contains no client secret. |
| Revoke request is stored but disablement is not yet verified | `202 Accepted` with a standard MachineAccount response in `revoking` state |
| Delete request is stored but removal is not yet verified | `202 Accepted` with a standard MachineAccount response in `deleting` state |

Error bodies SHALL NOT reveal a gateway that is hidden from the caller. They SHALL NOT contain access tokens, client secrets, or raw Keycloak Admin API responses.

### OpenAPI, SDK, and Event Contracts

The OpenAPI contract SHALL define separate schemas for create responses and standard MachineAccount responses. Only the create-response schema SHALL contain `credential.client_secret`.

The property SHALL be response-only (`readOnly`). Generators that support sensitive-property markers SHALL mark it as sensitive. List and get models SHALL not inherit it. Generated SDK string methods and debug helpers SHALL redact it.

gRPC watches and controller events SHALL contain only non-secret MachineAccount metadata. Protobuf resource messages and generic event payloads SHALL never contain a client secret.

Reconciliation events MAY contain these fields: resource ID, gateway ID, Keycloak client identifiers, role, status, subject, and expiration. They SHALL contain no client credentials.

## Authorization Model

MachineAccount authorization SHALL evaluate the `gateway_id` in the nested route. The generic viewer rule SHALL NOT authorize these operations.

A viewer can create a MachineAccount for personal use. The API limits that MachineAccount to `openshell-user`.

| Caller on selected gateway | Create | Maximum selectable role | List/Get | Revoke/Delete |
|---|---|---|---|---|
| `gateway:owner` | Yes | `openshell-admin` | All MachineAccounts on the gateway | Any MachineAccount on the gateway |
| `gateway:viewer` | Yes | `openshell-user` | Only MachineAccounts they created | Only MachineAccounts they created |
| `platform:admin` without a gateway binding | No | None | No | No |
| `gateway:creator` without a gateway binding | No | None | No | No |
| No applicable binding | No | None | No | No |

The `gateway:owner` binding permits a maximum role of `openshell-admin`. The `gateway:viewer` binding permits a maximum role of `openshell-user`.

The `role` field records the selected highest OpenShell privilege. For `openshell-admin`, the access token contains both supported roles. This behavior matches the existing owner-role mapping. For `openshell-user`, the access token contains only `openshell-user`.

The API SHALL accept only the two specified role values. It SHALL compare the selected role with the creator's maximum role. The API SHALL reject a higher role instead of silently lowering it. It SHALL reject all other Keycloak role names.

The `platform:admin` role does not grant OpenShell gateway access. A platform administrator needs a gateway binding to create or inspect MachineAccounts.

The API SHALL enforce creation quotas to limit the number of Keycloak clients. The default limit SHALL be 10 active MachineAccounts for each creator on each gateway. The default gateway quota SHALL be 100 active MachineAccounts.

Operators MAY lower either quota. Expired and revoked MachineAccounts do not count toward these quotas.

## Keycloak Representation

### Federated Identity Boundary

The configured Keycloak realm issues access tokens and stores MachineAccount identities. [HYPERSHELL-100](https://redhat.atlassian.net/browse/HYPERSHELL-100) defines one global, public Keycloak deployment. That deployment brokers human login to Red Hat SSO.

HyperShell SHALL create each machine client and service-account user directly in this realm. It SHALL NOT create a human identity in Red Hat SSO.

Keycloak is authoritative for machine identities, client credentials, role mappings, and issued access tokens. PostgreSQL is authoritative for the non-secret MachineAccount record and its requested lifecycle state.

[HYPERSHELL-130](https://redhat.atlassian.net/browse/HYPERSHELL-130) defines infrastructure-controller identities as a separate authorization domain. These identities SHALL NOT receive or reuse tenant MachineAccount roles or client credentials.

### One Confidential Client per MachineAccount

HyperShell SHALL create a separate machine client for each MachineAccount. MachineAccounts SHALL NOT share a client secret. Separate clients permit independent attribution, expiration, role changes, and revocation.

The `clientId` SHALL be `hs-ma-{gateway-id}-{machine-account-id}`. The Keycloak display name SHALL contain the user-visible MachineAccount name.

Client attributes SHALL contain the MachineAccount ID, gateway ID, and creator User ID. Reconciliation uses these identifiers to find orphan machine clients. Logs SHALL treat attribute values as untrusted input.

| Keycloak client property | Required value |
|---|---|
| `enabled` | `true` during final verification and while ready. `false` when expired, revoking, revoked, deleting, or error. |
| `protocol` | `openid-connect` |
| `publicClient` | `false` |
| `clientAuthenticatorType` | `client-secret` |
| `serviceAccountsEnabled` | `true` |
| `standardFlowEnabled` | `false` |
| `implicitFlowEnabled` | `false` |
| `directAccessGrantsEnabled` | `false` |
| Device Authorization Grant | disabled |
| `fullScopeAllowed` | `false` |
| redirect URIs / web origins | empty |
| access-token lifetime override | `300` seconds by default. Never greater than `900` seconds. |

The machine client SHALL NOT receive these permissions:

- Browser login
- Password grant
- Device flow
- Offline access or refresh tokens
- Realm management
- HyperShell management API access
- Access to an unrelated gateway

### Service-Account Roles and Client Scope

Keycloak includes a role only when both the service-account assignment and the machine-client scope permit it. HyperShell SHALL configure both controls:

1. Resolve the selected gateway client `{gateway-name}-{gateway-id}` and its `openshell-user` and `openshell-admin` roles.
2. Add only the required gateway-client roles to the machine client's role scope mappings while keeping `fullScopeAllowed = false`.
3. Resolve the machine client's service-account user through the Keycloak Admin API.
4. For `role=openshell-user`, assign only the selected gateway's `openshell-user` role to that service-account user.
5. For `role=openshell-admin`, assign the selected gateway's `openshell-admin` and supporting `openshell-user` roles to that service-account user.
6. Add an audience mapper for the selected gateway client.
7. Add a client-role mapper that writes the gateway roles to `hypershell.roles`.
8. Use Keycloak's standard service-account subject as `sub`. Do not override it with user input.

The machine client SHALL have no scope mapping for another gateway. Realm defaults and client scopes SHALL NOT add another gateway audience or role.

### Expected Access Token

An access token for an administrator MachineAccount has this security-relevant form:

```json
{
  "iss": "https://keycloak.example.com/realms/hypershell",
  "sub": "7d39dc7b-83c0-46f5-b2bf-27b93115a05f",
  "azp": "hs-ma-35GtW1ExampleGateway-35GtY7ExampleMachineAccount",
  "aud": "my-gateway-35GtW1ExampleGateway",
  "exp": 1787313900,
  "hypershell.roles": ["openshell-admin", "openshell-user"]
}
```

Keycloak controls the exact JWT serialization and all other claims. HyperShell defines these claims: issuer, subject, authorized party, gateway audience, expiration, and `hypershell.roles`.

The access token is a signed JWT, not an opaque token. The gateway SHALL validate it locally with the issuer's JWKS.

## Requirements

### Requirement: Synchronous Provisioning and One-Time Delivery

The create operation SHALL provision and validate the machine client before it returns `201 Created`. HyperShell SHALL test the Client Credentials grant before it sets status `ready`.

The test access token SHALL have the expected issuer, subject, gateway audience, and roles. After this test succeeds, HyperShell SHALL set status `ready` and return the client secret.

HyperShell SHALL transmit the generated client secret only through the authenticated create response. Internal provisioning calls SHALL use an authenticated synchronous path.

The client secret MUST NOT enter:

- The PostgreSQL record
- A gRPC watch or event broker
- A Kubernetes object
- A controller work queue
- An audit field, metric label, or trace attribute
- An application log

The security standard permits the create endpoint to return a new client secret once. No other public endpoint MAY return it.

#### Scenario: Owner creates an administrator MachineAccount

- GIVEN user-a has `gateway:owner` on gateway `gw-alpha`
- AND `gw-alpha` is running with a provisioned Keycloak gateway client
- WHEN user-a creates MachineAccount `deploy-bot` with `role=openshell-admin`
- THEN HyperShell SHALL create one confidential Keycloak service-account client
- AND the test access token SHALL have only the `gw-alpha` audience
- AND `hypershell.roles` SHALL contain `openshell-admin` and `openshell-user`
- AND the API SHALL return the client secret once in a no-store `201` response
- AND subsequent GET requests SHALL never return the secret

#### Scenario: Provisioning fails before delivery

- GIVEN Keycloak returns an error while a machine client is being configured
- WHEN the create request cannot prove the desired client, scope mappings, roles, and access-token claims
- THEN the API SHALL return no client secret
- AND HyperShell SHALL delete every partial Keycloak object
- AND it SHALL disable an object first if immediate deletion fails
- AND HyperShell SHALL record enough non-secret identifiers to find and remove any remaining machine client

### Requirement: Federated Keycloak Is the Identity System of Record

HyperShell SHALL provision the machine client and service-account user in the configured Keycloak realm. This operation SHALL require no manual Keycloak administration.

Keycloak SHALL remain the system of record for machine identities, client credentials, role mappings, and access-token issuance. HyperShell SHALL store only the non-secret MachineAccount record.

The upstream human identity provider SHALL NOT create or store a machine user. Client names, attributes, authorization, and audit records SHALL distinguish these identity types:

- Tenant MachineAccounts
- Human identities
- Infrastructure-controller identities

#### Scenario: Machine identity uses the global Keycloak issuer

- GIVEN production HyperShell uses the global Keycloak issuer with Red Hat SSO as its upstream human identity provider
- WHEN an authorized user creates a MachineAccount
- THEN HyperShell SHALL create a local Keycloak client and service-account user in the configured realm
- AND it SHALL not create a user in Red Hat SSO
- AND the resulting JWT `iss` SHALL be the same global Keycloak issuer trusted by the selected gateway

### Requirement: Client Credentials Token Issuance

Automation SHALL authenticate to Keycloak's discovered token endpoint. It SHALL send `grant_type=client_credentials`, the client ID, and the client secret.

Keycloak SHALL return a short-lived access token. It SHALL NOT return a refresh token or create a browser user session for this flow.

The MachineAccount `expires_at` controls how long Keycloak accepts the client credentials. It does not set the access token's `exp`. Callers SHALL NOT set a custom access-token lifetime.

HyperShell SHALL enforce `expires_at` by disabling the machine client. Version 1 SHALL NOT depend on Keycloak's preview client-secret-expiration capability. The opaque client secret does not expire by itself.

#### Scenario: A 12-month MachineAccount receives a short access token

- GIVEN a MachineAccount expires 365 days after creation
- WHEN its CI job performs a Client Credentials grant while the MachineAccount is ready
- THEN Keycloak SHALL return an access JWT whose lifetime is five minutes by default
- AND the JWT SHALL NOT remain valid for 365 days
- AND the CI job MAY request another short-lived access token until the MachineAccount expires or is revoked

#### Scenario: Access token expires during a long job

- GIVEN a CI job still holds valid MachineAccount client credentials
- AND its current access token reaches `exp`
- WHEN the job needs another gateway request
- THEN the integration SHALL perform another Client Credentials grant
- AND it SHALL NOT attempt a refresh-token grant

### Requirement: Single-Gateway Isolation

Every MachineAccount SHALL target exactly one Gateway. Its audience mapper SHALL add only that gateway's OIDC audience. Its role mapper and scope mappings SHALL reference only that gateway client.

The API SHALL NOT accept `audience`, `gateway_ids`, `client_id`, `issuer`, mapper configuration, or raw role claims from a create request. These values are derived from the selected Gateway and trusted platform configuration.

#### Scenario: Access token is rejected by a different gateway

- GIVEN Keycloak issued an access token for a MachineAccount on `gw-alpha`
- AND `gw-alpha` and `gw-beta` have distinct OIDC audiences
- WHEN the access token is presented to `gw-beta`
- THEN `gw-beta` SHALL reject it for an audience mismatch
- AND the access token SHALL contain no `gw-beta` role

#### Scenario: Gateway access token is presented to the management API

- GIVEN Keycloak issued an access token for a gateway MachineAccount
- WHEN it is presented to the HyperShell management API
- THEN the management API SHALL reject it because it has neither the management API audience nor a management-plane role
- AND the MachineAccount SHALL gain no ability to create gateways, RoleBindings, or other MachineAccounts

#### Scenario: Automation needs two gateways

- GIVEN a CI workflow needs both `gw-alpha` and `gw-beta`
- WHEN the workflow is configured
- THEN an authorized user SHALL create one MachineAccount on each gateway
- AND the workflow SHALL store two independently revocable sets of client credentials
- AND it SHALL request a separate access token for each gateway

### Requirement: User-Selected, RBAC-Capped OpenShell Role

The creator SHALL select `openshell-user` or `openshell-admin`. The default SHALL be `openshell-user`.

The selected role SHALL NOT exceed the creator's current gateway role. A `gateway:owner` can select either value. A `gateway:viewer` can select only `openshell-user`.

Each active MachineAccount SHALL remain subject to the creator's current gateway role. HyperShell SHALL reconcile the MachineAccount when that role changes.

#### Scenario: Owner deliberately selects the lower role

- GIVEN user-a has `gateway:owner` on `gw-alpha`
- WHEN user-a creates a MachineAccount with `role=openshell-user`
- THEN HyperShell SHALL assign only `openshell-user` to the machine identity
- AND the access token SHALL not contain `openshell-admin`
- AND ownership SHALL define the maximum selectable role, not force every owner-created client to be an administrator

#### Scenario: Viewer creates a user MachineAccount

- GIVEN user-b has only `gateway:viewer` on `gw-alpha`
- WHEN user-b creates a MachineAccount with the default role
- THEN its role SHALL be `openshell-user`
- AND its service-account user SHALL have only `openshell-user` for `gw-alpha`
- AND the access token SHALL not contain `openshell-admin`

#### Scenario: Viewer requests administrator access

- GIVEN user-b has only `gateway:viewer` on `gw-alpha`
- WHEN user-b requests `role=openshell-admin`
- THEN the API SHALL return `403 Forbidden`
- AND it SHALL create neither a MachineAccount record nor a Keycloak client

#### Scenario: Creator is downgraded from owner to viewer

- GIVEN user-a created an `openshell-admin` MachineAccount on `gw-alpha`
- WHEN user-a's effective binding on `gw-alpha` changes from owner to viewer
- THEN HyperShell SHALL remove `openshell-admin` from the machine service-account user and client scope
- AND the MachineAccount role SHALL become `openshell-user`
- AND the downgrade SHALL be permanent for that MachineAccount even if user-a is later promoted
- AND an already-issued admin JWT MAY remain usable only until its short `exp`

#### Scenario: Creator loses gateway access

- GIVEN user-a created a MachineAccount on `gw-alpha`
- WHEN user-a has no remaining owner or viewer binding on `gw-alpha`
- THEN HyperShell SHALL permanently revoke that MachineAccount
- AND restoring user-a's binding SHALL NOT re-enable the old client secret

### Requirement: Expiration, Revocation, and Deletion

HyperShell SHALL check expiration at least once per minute. It SHALL disable the machine client no later than one minute after `expires_at`. It SHALL then mark the MachineAccount `expired`.

An expired, revoked, or deleted machine client SHALL NOT issue new access tokens. HyperShell SHALL enforce expiration without Keycloak's preview client-secret-expiration feature. A delay or failure SHALL produce an operator alert.

Revocation and deletion are separate operations. Revocation is explicit and permanent. `POST .../{machine_account_id}/revoke` SHALL disable the machine client before it marks the MachineAccount `revoked`.

A revoked MachineAccount retains its non-secret metadata and audit events. A successful revoke response means that Keycloak cannot issue another access token. Repeated revocation SHALL succeed without changing this result. HyperShell SHALL provide no re-enable operation.

Deletion performs final identity cleanup. `DELETE .../{machine_account_id}` SHALL revoke a ready MachineAccount first. It SHALL then delete the machine client and its service-account user. Finally, it SHALL soft-delete the MachineAccount record.

List and get operations SHALL exclude soft-deleted MachineAccounts. A separate audit event SHALL remain after deletion. This specification does not set the audit-retention period.

Deletion of an already revoked MachineAccount SHALL be valid. A `204 No Content` response means that the machine client is absent. It also means that normal read operations cannot find the MachineAccount.

The API SHALL persist a revoke or delete request before it calls Keycloak. If Keycloak is unavailable, the API SHALL return `202 Accepted`. The response SHALL contain a standard MachineAccount with status `revoking` or `deleting`.

Background reconciliation SHALL continue the operation. If the API cannot persist the request, it SHALL return `503 Service Unavailable`.

The API SHALL return `200` for revoke only after it observes the disabled machine client. It SHALL return `204` for delete only after it observes that the machine client is absent.

An access token issued before disablement MAY remain valid until its `exp`.

#### Scenario: MachineAccount expires

- GIVEN a ready MachineAccount reaches `expires_at`
- WHEN the expiration reconciler runs
- THEN it SHALL disable the Keycloak client within one minute
- AND subsequent Client Credentials grants SHALL fail
- AND an access token issued before expiration MAY remain usable only until its `exp`

#### Scenario: Owner revokes a viewer-created MachineAccount

- GIVEN user-b created an `openshell-user` MachineAccount as a viewer
- AND user-a is an owner of the same gateway
- WHEN user-a revokes that MachineAccount
- THEN HyperShell SHALL disable its Keycloak client and retain non-secret MachineAccount metadata
- AND user-b SHALL not be able to request another access token with the old client secret

#### Scenario: Owner deletes a revoked MachineAccount

- GIVEN an owner previously revoked a MachineAccount
- WHEN the owner deletes that MachineAccount
- THEN HyperShell SHALL delete the disabled Keycloak client and its service-account user
- AND the API SHALL return `204 No Content` only after observing that cleanup
- AND the audit trail SHALL retain no client secret or access token

### Requirement: Replacement-Based Credential Rotation

Version 1 SHALL replace a MachineAccount instead of changing its client secret. Use this replacement flow:

1. Create a MachineAccount with the required role and expiration.
2. Install its client credentials in the consumer.
3. Verify that the consumer can request a valid access token.
4. Revoke the old MachineAccount.
5. Delete the old MachineAccount when its metadata is no longer needed.

The old and replacement MachineAccounts MAY remain active at the same time. Both accounts count toward quota. This overlap prevents downtime during the change.

Revocation stops new access-token issuance with the old client secret. Deletion is the later cleanup step. HyperShell SHALL NOT enable Keycloak's preview dual-secret rotation feature.

#### Scenario: CI client credentials are replaced without downtime

- GIVEN `nightly-build-v1` is an active MachineAccount used by CI
- WHEN an authorized user creates `nightly-build-v2` with the intended role and expiration
- AND CI verifies an access token issued with the new client credentials
- THEN the user SHALL revoke `nightly-build-v1`
- AND new grants with the old secret SHALL fail
- AND the user MAY delete `nightly-build-v1` after revocation
- AND no in-place secret regeneration SHALL be required

### Requirement: Gateway Lifecycle Cleanup

Gateway deletion SHALL revoke every related MachineAccount. It SHALL also delete the gateway client, console client, and all related machine clients.

Deleting only the gateway client is not sufficient. An issued access token already contains its audience. The gateway does not check whether that audience client still exists.

Machine-client attributes SHALL contain HyperShell resource IDs. Reconciliation SHALL use these IDs to find machine clients without a MachineAccount or Gateway. It SHALL disable and remove each orphan machine client.

Repeated cleanup SHALL be safe and produce the same Keycloak state. Cleanup SHALL never log a client secret.

#### Scenario: Gateway deletion removes machine clients

- GIVEN `gw-alpha` has three active MachineAccounts
- WHEN `gw-alpha` is deleted
- THEN HyperShell SHALL disable all three machine clients before gateway teardown is considered complete
- AND it SHALL delete those clients and their service-account users through Keycloak client deletion
- AND normal read operations SHALL exclude the soft-deleted MachineAccounts
- AND separate non-secret audit events SHALL remain

### Requirement: Secret-Safe Management UI

The management web console SHALL show MachineAccounts for the selected gateway. The create form SHALL collect a name, optional description, OpenShell role, and expiration.

The form SHALL default to `openshell-user`. It SHALL offer both roles to an owner. It SHALL offer only `openshell-user` to a viewer. The form SHALL explain that MachineAccount expiration is different from access-token expiration.

After creation, the UI SHALL show the client ID and client secret in a one-time view. The view SHALL provide copy and download actions. It SHALL state that HyperShell cannot show the secret again.

When the user leaves or closes this view, the UI SHALL remove the client secret from application state. The following locations SHALL never contain it:

- Browser storage
- Query caches
- Telemetry or error reports
- URLs

The UI SHALL present revoke and delete as separate actions. The revoke action SHALL explain that Keycloak will stop issuing access tokens permanently. The MachineAccount row SHALL remain available for audit and later deletion.

The delete action SHALL explain that it removes the Keycloak identity and the visible MachineAccount. Deleting a ready MachineAccount SHALL revoke it first.

The collection SHALL use server pagination and filtering. Search SHALL wait 300 milliseconds after the last input change. A new search SHALL cancel the old request and reset the page to one. Wildcard characters SHALL pass to the API as literal text.

The UI SHALL distinguish an empty gateway from a search with no matches. It SHALL NOT apply a second client-side authorization filter.

The UI SHALL NOT retry a create request automatically after an uncertain response. A retry cannot recover the one-time client secret.

The UI SHALL explain the uncertain result. It SHALL direct the user to refresh the list. If the MachineAccount exists, the user SHALL delete and replace it.

After a confirmed create, revoke, or delete operation, the UI SHALL refetch the MachineAccount list. It SHALL poll `revoking` until the status becomes `revoked`. It SHALL poll `deleting` until the resource disappears. If either operation fails, the UI SHALL show an actionable error.

#### Scenario: Viewer sees a constrained form

- GIVEN a viewer opens MachineAccounts for a gateway
- WHEN the create form renders
- THEN `openshell-user` SHALL be the only permitted role value
- AND the UI SHALL explain that an OpenShell workspace membership may still be required for the machine subject

### Requirement: CLI and CI Workflow

The HyperShell CLI SHALL provide create, list, get, revoke, and delete commands. The create command SHALL support these options:

- `--gateway-id`
- `--name`
- `--description`
- `--role`
- `--expires-at` or `--expires-in`
- Structured output

The `--role` option SHALL accept only `openshell-user` and `openshell-admin`. The CLI SHALL write the credential bundle only to the requested standard output or file.

The CLI SHALL never write the credential bundle to standard error, debug logs, shell configuration, or the HyperShell login-token store.

The supported CI pattern SHALL be:

1. Create the MachineAccount once through HyperShell.
2. Store the client ID and client secret in the CI platform's secret manager.
3. Configure non-secret issuer, token endpoint, gateway endpoint, and audience metadata.
4. On each job, exchange the client credentials for a short-lived access token.
5. Run Client Credentials again when the access token expires.

This workflow SHALL NOT require a human browser login.

The supported OpenShell integration SHALL use `OPENSHELL_OIDC_CLIENT_SECRET`. If the access token is absent or expired, the integration SHALL run another Client Credentials grant.

The grant does not return a refresh token. The integration SHALL NOT depend on `grant_type=refresh_token`.

#### Scenario: CI starts without a cached access token

- GIVEN a CI secret store injects the MachineAccount client ID and secret
- AND no prior OIDC token file exists
- WHEN the job invokes the supported OpenShell connection flow
- THEN it SHALL request a new access token without user interaction
- AND the client secret SHALL not appear in command output
- AND the gateway SHALL receive only the short-lived access token

### Requirement: Workspace Membership Is a Separate Grant

Each machine client has its own service-account user and stable JWT `sub`. It does not inherit the creator's OpenShell workspace memberships.

An `openshell-admin` MachineAccount can perform OpenShell administrative operations. OpenShell permits this role to bypass workspace membership.

An `openshell-user` MachineAccount can authenticate and call identity or status operations. Workspace operations require a gateway administrator to add the MachineAccount subject to each workspace.

HyperShell SHALL return the non-secret subject. UI and CLI output SHALL explain the separate workspace grant.

Version 1 excludes automatic workspace discovery, selection, membership creation, and membership cleanup. HyperShell SHALL NOT report that `openshell-user` grants access to the creator's workspaces.

#### Scenario: openshell-user MachineAccount has no workspace membership

- GIVEN a viewer creates an `openshell-user` MachineAccount
- AND no OpenShell workspace contains the returned machine subject
- WHEN its access token calls `whoami`
- THEN the gateway SHALL authenticate it as `openshell-user`
- BUT workspace-scoped operations SHALL remain denied until an authorized gateway administrator adds that subject as a workspace member

### Requirement: Scopes Are Not Configurable in Version 1

The MachineAccount create request SHALL contain no OpenShell scope list. HyperShell configures gateways with an empty `scopes_claim`. This setting disables OpenShell scope enforcement.

Access-token scopes provide no additional control while scope enforcement is disabled. A later scope design SHALL define one gateway-wide contract. Tests SHALL cover human, console, and machine access tokens together.

### Requirement: Auditability and Secret Redaction

HyperShell SHALL audit creation, creation failure, expiration, role downgrade, revocation, and deletion. Each audit event SHALL include:

- Actor user ID and creator user ID
- Gateway ID and MachineAccount ID
- Selected role and expiration
- Outcome and correlation ID

Audit events SHALL NOT contain:

- Client secrets or access tokens
- Authorization headers
- Raw token-endpoint response bodies
- Decoded claims other than required non-secret identifiers

HTTP logs, BFF logs, OpenTelemetry spans, metrics, and SDK diagnostics SHALL redact the `credential` field. HyperShell SHALL redact client-authentication data from Keycloak Admin API errors.

HyperShell SHALL disable body capture for MachineAccount create requests and Keycloak token-endpoint requests.

### Requirement: Reconciliation and Drift Repair

The PostgreSQL MachineAccount record SHALL define the required Keycloak state. Reconciliation SHALL restore these settings:

- `fullScopeAllowed=false`
- Disabled interactive grants
- The required audience mapper
- The required gateway-role scope and role assignments
- The required enabled state
- The access-token lifetime limit

If Keycloak grants broader access, reconciliation SHALL remove the unexpected roles or audiences. It SHALL complete this removal before it re-enables the machine client.

Reconciliation SHALL never regenerate or fetch a client secret. Such a change would invalidate the consumer's stored client credentials without delivering a replacement.

#### Scenario: Machine client drifts to full scope

- GIVEN an administrator manually changes a machine client to `fullScopeAllowed=true`
- WHEN HyperShell reconciles that MachineAccount
- THEN it SHALL set `fullScopeAllowed=false`
- AND it SHALL remove every unrelated role scope mapping
- AND it SHALL verify that a new access token contains only the selected gateway audience and effective roles

### Requirement: Verification Coverage

Automated verification SHALL cover:

- Owner and viewer create authorization, including an owner's lower-role selection and viewer rejection for `openshell-admin`.
- Platform-admin-without-binding rejection.
- One-time secret presence on create and absence from every GET, event, log, trace, and database row.
- Keycloak client properties, service-account role mappings, client scope mappings, mappers, and five-minute access-token lifetime.
- Exact `aud`, `sub`, `azp`, `exp`, and `hypershell.roles` claims for `openshell-user` and `openshell-admin` selections.
- Cross-gateway audience rejection.
- Owner-to-viewer downgrade and binding-removal revocation.
- Expiration, explicit revocation, final deletion, gateway cascade cleanup, retries, and orphan reconciliation.
- CI access-token renewal through another Client Credentials grant.
- The distinct workspace-membership requirement for `openshell-user` MachineAccounts.

No test fixture, golden response, failure message, or CI artifact SHALL retain a real client secret or access token.

## Explicitly Deferred

| Capability | Reason deferred |
|---|---|
| WIF / external OIDC token exchange | It requires a different trust and policy model. A later design can add another authentication method. |
| One set of client credentials or one access token for multiple gateways | It increases the effect of a leaked secret. The current flat `hypershell.roles` claim cannot express different gateway roles safely. |
| User-configurable access-token lifetime | It increases the revocation delay. Jobs can request new access tokens when needed. |
| Arbitrary Keycloak/OpenShell roles | The selected role is restricted to `openshell-user` or `openshell-admin` and capped by the creator's binding. |
| OpenShell scope selection | Gateways currently disable scope enforcement with `scopes_claim = ""`. |
| Automatic workspace memberships | Requires a separate gateway administration and lifecycle contract. |
| In-place secret rotation | Keycloak 26.2 dual-secret rotation is a disabled preview feature. Replacement MachineAccounts provide a stable workflow without downtime. |
| Audit-retention period | A platform-wide audit policy must define the period. This specification requires an audit event but does not set its retention period. |
| Public secret recovery | Conflicts with one-time delivery and expands the consequence of a management-session compromise. |
| `last_used_at` | Offline gateway validation provides no reliable synchronous usage signal to HyperShell. |

## References

- [HYPERSHELL-49: Service account provisioning via federated Keycloak](https://redhat.atlassian.net/browse/HYPERSHELL-49)
- [HYPERSHELL-100: Establish global Keycloak federation with Red Hat SSO](https://redhat.atlassian.net/browse/HYPERSHELL-100)
- [HYPERSHELL-130: Adopt controller-initiated infrastructure reconciliation](https://redhat.atlassian.net/browse/HYPERSHELL-130)
- [Keycloak service accounts and Client Credentials](https://www.keycloak.org/docs/latest/server_admin/#_service_accounts)
- [Keycloak role scope mappings](https://www.keycloak.org/docs/latest/server_admin/#_role_scope_mappings)
- [Keycloak client secret rotation](https://www.keycloak.org/docs/latest/server_admin/#_client-secret-rotation)
- [Keycloak Admin REST API](https://www.keycloak.org/docs-api/latest/rest-api/)
- [OAuth 2.0 Client Credentials Grant (RFC 6749, section 4.4)](https://www.rfc-editor.org/rfc/rfc6749#section-4.4)
- [JWT expiration claim (RFC 7519, section 4.1.4)](https://www.rfc-editor.org/rfc/rfc7519#section-4.1.4)
- [OpenShell gateway authentication](https://docs.nvidia.com/openshell/reference/gateway-auth)
- [OpenShell workspace access model](https://docs.nvidia.com/openshell/latest/sandboxes/manage-workspaces)
