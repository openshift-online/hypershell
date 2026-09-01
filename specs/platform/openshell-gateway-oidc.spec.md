# OpenShell Gateway OIDC Specification

**Date:** 2026-08-11
**Status:** Draft
**Parent:** `openshell-gateway.spec.md` - core gateway provisioning
**Related:** `openshell-gateway-keycloak.spec.md` - automated per-gateway Keycloak provisioning; `openshell-gateway-tls.spec.md` - TLS certificate management; `openshell-gateway-routing.spec.md` - external connectivity
**OpenShell gateway service accounts:** `openshell-gateway-service-accounts.spec.md` - Client Credentials for automation

---

## Purpose

This specification defines per-gateway OIDC authentication for OpenShell gateways. OIDC enables CLI users and external clients to authenticate via Bearer tokens from an identity provider (e.g., Keycloak). OIDC is the sole authentication mechanism for HyperShell gateways in production.

OIDC configuration is auto-provisioned by the control plane when a gateway is reconciled. The GatewayReconciler provisions a dedicated Keycloak OIDC client per gateway and populates the Gateway resource's `oidc` field automatically. See [`openshell-gateway-keycloak.spec.md`](./openshell-gateway-keycloak.spec.md) for provisioning details. This specification covers how the control plane injects the OIDC configuration into `gateway.toml` and validates it.

---

## Architecture

### Authentication Flow

```
CLI User or OpenShellGatewayServiceAccount
    │  1. Obtains OIDC token from IdP (interactive browser/device flow or
    │     non-interactive Client Credentials grant)
    │  2. Connects to gateway with Bearer token in Authorization header
    ▼
OpenShell Gateway
    │  Validates JWT: issuer, audience, signature (JWKS), expiry
    │  Extracts roles from roles_claim (e.g., "hypershell.roles" → ["openshell-admin"])
    │  Maps to admin/user tier
    ▼
Authorized (sandbox create, list, exec, etc.)
```

### Per-Gateway Keycloak Configuration

Each gateway receives a dedicated Keycloak OIDC client provisioned automatically by the control plane (see [`openshell-gateway-keycloak.spec.md`](./openshell-gateway-keycloak.spec.md)).

```
Realm:       configurable via Keycloak service account secret (e.g., hypershell, hypershell-stage)
Client:      {name}-{id} (public, PKCE, fullScopeAllowed=false)
Audience:    {name}-{id} (matches clientId)
Roles claim: hypershell.roles (via client-roles mapper from resource_access.{clientId}.roles)
Roles:       openshell-admin, openshell-user (client-scoped)
```

> **Client ID format:** The Keycloak `clientId` is `{name}-{id}` (e.g., `my-gateway-2FhMpQzXBz`) to prevent name clashes. See [`openshell-gateway-keycloak.spec.md`](./openshell-gateway-keycloak.spec.md) for details.

> **Realm naming:** The realm name is not hardcoded. Each HyperShell instance reads the realm from the `hypershell-keycloak-admin` Secret's `realm` key. Environments MAY use distinct realm names (e.g., `hypershell-int`, `hypershell-stage`, `hypershell-prod`) or share a realm name when regional Keycloak instances federate to a global instance.

> **Per-gateway isolation:** Each gateway has its own Keycloak client with `fullScopeAllowed = false` and a dedicated audience. Tokens obtained for one gateway contain only that gateway's roles and audience -- cross-gateway role leakage is prevented at the IdP level.

---

## Requirements

### Requirement: Gateway OIDC API Fields

The Gateway API resource SHALL have an `oidc` object containing OIDC configuration fields. These fields are auto-populated by the API server during Keycloak provisioning (see [`openshell-gateway-keycloak.spec.md`](./openshell-gateway-keycloak.spec.md)) and are read-only in the REST and gRPC contracts. The fields map directly to the upstream OpenShell `server.oidc.*` helm values.

| Field | Type | Auto-Populated Value | Description |
|---|---|---|---|
| `oidc.issuer` | string | `{keycloak-url}/realms/{realm}` | OIDC issuer URL |
| `oidc.audience` | string | `{name}-{id}` | Expected `aud` claim value in JWT (matches Keycloak clientId) |
| `oidc.jwks_ttl` | integer | `3600` | JWKS key cache retention in seconds |
| `oidc.roles_claim` | string | `"hypershell.roles"` | Dot-delimited path to roles array in JWT claims |
| `oidc.admin_role` | string | `"openshell-admin"` | Role name conferring admin access |
| `oidc.user_role` | string | `"openshell-user"` | Role name conferring standard user access |
| `oidc.scopes_claim` | string | `""` | Dot-delimited path to scopes array in JWT claims |

#### Scenario: OIDC auto-populated after Keycloak provisioning

- GIVEN a Gateway `my-gateway` is created via `hsctl apply -k` or the REST API
- AND the kustomize overlay does NOT include OIDC fields (they are system-managed)
- WHEN the GatewayReconciler provisions the Keycloak client
- THEN the Gateway's `oidc` field SHALL be auto-populated with values derived from the provisioned client
- AND the GatewayReconciler SHALL generate a `gateway.toml` containing the `[openshell.gateway.oidc]` section
- AND `allow_unauthenticated_users` SHALL be `false`

#### Scenario: OIDC fields are read-only

- GIVEN a Gateway with auto-populated OIDC configuration
- WHEN a user attempts to PATCH the Gateway's `oidc` fields via the REST or gRPC API
- THEN the API server SHALL reject the update
- AND the OIDC fields SHALL remain as set by the control plane

---

### Requirement: OIDC Role Validation

The auto-provisioned OIDC configuration always sets both `admin_role` and `user_role` to fixed values (`openshell-admin` and `openshell-user`). The upstream OpenShell gateway requires both to be set or both empty -- setting only one is not supported. Since the control plane always populates both, this constraint is satisfied by construction.

#### Scenario: Auto-provisioned roles are always complete

- GIVEN the control plane auto-populates OIDC configuration
- THEN `oidc.admin_role` SHALL be `"openshell-admin"` and `oidc.user_role` SHALL be `"openshell-user"`
- AND the upstream both-or-neither constraint SHALL be satisfied

---

### Requirement: OIDC Configuration in gateway.toml

When a Gateway has OIDC enabled (non-empty `oidc.issuer`), the GatewayReconciler SHALL inject the OIDC configuration into the `gateway.toml` ConfigMap.

#### Scenario: OIDC section injected into default gateway.toml

- GIVEN a Gateway with OIDC enabled and no custom `config` field
- WHEN the GatewayReconciler generates the ConfigMap
- THEN `gateway.toml` SHALL contain:
  ```toml
  [openshell.gateway.auth]
  allow_unauthenticated_users = false

  [openshell.gateway.oidc]
  issuer        = "https://keycloak.example.com/realms/hypershell"
  audience      = "my-gateway-2FhMpQzXBz"
  jwks_ttl_secs = 3600
  roles_claim   = "hypershell.roles"
  admin_role    = "openshell-admin"
  user_role     = "openshell-user"
  ```
- AND `allow_unauthenticated_users` SHALL be `false`

#### Scenario: Custom config overrides bypass OIDC injection

- GIVEN a Gateway with OIDC enabled AND a custom `config` field (raw TOML)
- THEN the custom `config` SHALL be used verbatim
- AND the GatewayReconciler SHALL NOT inject the OIDC section

#### Scenario: Only non-empty OIDC fields written to TOML

- GIVEN a Gateway with only `oidc.issuer` set (all other fields at zero values)
- THEN `gateway.toml` SHALL contain only the `issuer` key in the OIDC section
- AND the upstream gateway SHALL apply its own defaults for omitted fields

---

### Requirement: OIDC Change Detection

The GatewayReconciler SHALL detect changes to OIDC configuration and trigger a gateway restart. OIDC fields are system-managed; changes occur when the control plane updates them (e.g., Keycloak realm migration, service account secret rotation).

#### Scenario: OIDC configuration changed by control plane

- GIVEN a running gateway with OIDC configured for issuer `https://old-keycloak.example.com/realms/hypershell`
- WHEN the Keycloak service account secret is updated to point to a new Keycloak instance
- AND the control plane re-populates the Gateway's OIDC fields on the next reconciliation cycle
- THEN the ConfigMap SHALL be updated with the new OIDC settings
- AND the gateway pods SHALL be restarted via rolling update

---

## CLI Authentication Flow

```bash
# Interactive users authenticate via Authorization Code + PKCE (browser) or
# Device Authorization (headless). Automation uses a gateway-scoped
# OpenShellGatewayServiceAccount with Client Credentials. No passwords are stored.

# 1a. Login to hsctl -- browser PKCE (opens browser, exchanges code, stores tokens)
hsctl login --url "$API_URL" --issuer-url "$OIDC_ISSUER" --insecure

# 1b. Login to hsctl -- device flow (headless / SSH; prints URL + user code, polls until done)
hsctl login --no-browser --url "$API_URL" --issuer-url "$OIDC_ISSUER" --insecure

# 2. Register the OpenShell CLI with the gateway
hsctl gateway setup-cli --gateway-url "$GATEWAY_URL"

# 3. Use OpenShell interactively
OPENSHELL_GATEWAY_INSECURE=true openshell -g "$GATEWAY_NAME" status
OPENSHELL_GATEWAY_INSECURE=true openshell -g "$GATEWAY_NAME" sandbox create --name demo
```

For CI, create an OpenShellGatewayServiceAccount through HyperShell. Store its client ID and
client secret in the CI secret manager. The supported OpenShell integration
requests short-lived access tokens with `grant_type=client_credentials`.
See [`openshell-gateway-service-accounts.spec.md`](./openshell-gateway-service-accounts.spec.md).

---

## Debugging Reference

| Symptom | Root Cause | Fix |
|---|---|---|
| `role 'openshell-user' required` | OIDC `roles_claim` misconfigured or user lacks the required Keycloak client role | Verify user has a RoleBinding and the OIDC Role Bridge has assigned the Keycloak client role |
| `Invalid client or Invalid client credentials` | The client ID or secret is incorrect. The OpenShellGatewayServiceAccount can also be revoked, expired, or deleted. | Verify the client ID and lifecycle state. Never print the client secret. |
| Token expires after 5 minutes | Keycloak access-token lifetime | Interactive users refresh their session. OpenShellGatewayServiceAccounts perform another Client Credentials grant without a refresh token. |
| `openshell gateway add` opens browser | No `--no-browser` flag | Write `metadata.json` directly, then use `hsctl gateway setup-cli` |
| `GROUPS` env var returns `1000` in bash | Bash builtin collision - `GROUPS` is a reserved readonly array | Use `USER_GROUPS` instead of `GROUPS` for role/group env vars |
| `openshell sandbox create` hangs | Blocking interactive command | Background the command and poll for pod status; use `ExecSandbox` for runner startup |

---

## References

- [OpenShell OIDC User Authentication](https://docs.nvidia.com/openshell/latest/kubernetes/access-control#oidc-user-authentication)
- [OpenShell OIDC Values Reference](https://docs.nvidia.com/openshell/latest/kubernetes/access-control#oidc-values-reference)
- [OpenShell Gateway Auth Reference](https://docs.nvidia.com/openshell/reference/gateway-auth#oidc)
