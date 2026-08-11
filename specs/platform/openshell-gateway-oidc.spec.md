# OpenShell Gateway OIDC Specification

**Date:** 2026-08-11
**Status:** Draft
**Parent:** `openshell-gateway.spec.md` - core gateway provisioning
**Related:** `openshell-gateway-keycloak.spec.md` - automated per-gateway Keycloak provisioning; `openshell-gateway-tls.spec.md` - TLS certificate management; `openshell-gateway-routing.spec.md` - external connectivity

---

## Purpose

This specification defines per-gateway OIDC authentication for OpenShell gateways. OIDC enables CLI users and external clients to authenticate via Bearer tokens from an identity provider (e.g., Keycloak). OIDC is the sole authentication mechanism for HyperShell gateways in production.

OIDC configuration is auto-provisioned by the control plane when a gateway is reconciled. The GatewayReconciler provisions a dedicated Keycloak OIDC client per gateway and populates the Gateway resource's `oidc` field automatically. See [`openshell-gateway-keycloak.spec.md`](./openshell-gateway-keycloak.spec.md) for provisioning details. This specification covers how the control plane injects the OIDC configuration into `gateway.toml` and validates it.

---

## Architecture

### Authentication Flow

```
CLI User
    │  1. Obtains OIDC token from IdP (Keycloak password grant or browser flow)
    │  2. Connects to gateway with Bearer token in Authorization header
    ▼
OpenShell Gateway
    │  Validates JWT: issuer, audience, signature (JWKS), expiry
    │  Extracts roles from roles_claim (e.g., "groups" → ["hypershell-admins", "hypershell-users"])
    │  Maps to admin/user tier
    ▼
Authorized (sandbox create, list, exec, etc.)
```

### Per-Gateway Keycloak Configuration

Each gateway receives a dedicated Keycloak OIDC client provisioned automatically by the control plane (see [`openshell-gateway-keycloak.spec.md`](./openshell-gateway-keycloak.spec.md)).

```
Realm:       configurable via Keycloak service account secret (e.g., hypershell, hypershell-stage)
Client:      {gateway-name} (public, PKCE, fullScopeAllowed=false)
Audience:    {gateway-name} (matches clientId)
Roles claim: hypershell.roles (via client-roles mapper from resource_access.{clientId}.roles)
Roles:       openshell-admin, openshell-user (client-scoped)
```

> **Realm naming:** The realm name is not hardcoded. Each HyperShell instance reads the realm from the `hypershell-keycloak-admin` Secret's `realm` key. Environments MAY use distinct realm names (e.g., `hypershell-int`, `hypershell-stage`, `hypershell-prod`) or share a realm name when regional Keycloak instances federate to a global instance.

> **Per-gateway isolation:** Each gateway has its own Keycloak client with `fullScopeAllowed = false` and a dedicated audience. Tokens obtained for one gateway contain only that gateway's roles and audience — cross-gateway role leakage is prevented at the IdP level.

---

## Requirements

### Requirement: Gateway OIDC API Fields

The Gateway API resource SHALL have an `oidc` object containing OIDC configuration fields. These fields are auto-populated by the API server during Keycloak provisioning (see [`openshell-gateway-keycloak.spec.md`](./openshell-gateway-keycloak.spec.md)) and are read-only in the REST and gRPC contracts. The fields map directly to the upstream OpenShell `server.oidc.*` helm values.

| Field | Type | Auto-Populated Value | Description |
|---|---|---|---|
| `oidc.issuer` | string | `{keycloak-url}/realms/{realm}` | OIDC issuer URL |
| `oidc.audience` | string | `{gateway-name}` | Expected `aud` claim value in JWT (matches Keycloak clientId) |
| `oidc.jwks_ttl` | integer | `3600` | JWKS key cache retention in seconds |
| `oidc.roles_claim` | string | `"hypershell.roles"` | Dot-delimited path to roles array in JWT claims |
| `oidc.admin_role` | string | `"openshell-admin"` | Role name conferring admin access |
| `oidc.user_role` | string | `"openshell-user"` | Role name conferring standard user access |
| `oidc.scopes_claim` | string | `""` | Dot-delimited path to scopes array in JWT claims |

#### Scenario: Gateway with OIDC enabled via kustomize

- GIVEN a Gateway resource in a kustomize overlay:
  ```yaml
  kind: Gateway
  name: openshell-gateway
  project: tenant-a
  image: ghcr.io/nvidia/openshell/gateway:0.0.88
  server_dns_names:
    - openshell-gateway.tenant-a.svc.cluster.local
  oidc:
    issuer: https://keycloak.example.com/realms/hypershell
    audience: hypershell-frontend
    roles_claim: groups
    admin_role: hypershell-admins
    user_role: hypershell-users
  ```
- WHEN the user runs `hsctl apply -k`
- THEN the API server SHALL persist the Gateway with OIDC configuration
- AND the GatewayReconciler SHALL generate a `gateway.toml` containing the `[openshell.gateway.oidc]` section

#### Scenario: Gateway without OIDC (default)

- GIVEN a Gateway resource with no `oidc` field
- WHEN the GatewayReconciler reconciles
- THEN `gateway.toml` SHALL NOT contain an `[openshell.gateway.oidc]` section
- AND `allow_unauthenticated_users` SHALL remain `true`

---

### Requirement: OIDC Role Validation

When OIDC role-based access control is configured, both `admin_role` and `user_role` MUST be set, or both MUST be empty. Setting only one is not supported per the upstream OpenShell constraint.

#### Scenario: Valid RBAC configuration

- GIVEN `oidc.admin_role = "hypershell-admins"` and `oidc.user_role = "hypershell-users"`
- THEN validation SHALL pass

#### Scenario: Invalid partial RBAC configuration

- GIVEN `oidc.admin_role = "hypershell-admins"` and `oidc.user_role = ""`
- THEN validation SHALL fail: both must be set or both must be empty

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
  audience      = "my-gateway"
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

The GatewayReconciler SHALL detect changes to OIDC configuration and trigger a gateway restart.

#### Scenario: OIDC configuration changed

- GIVEN a running gateway with OIDC configured for issuer `https://old-idp.example.com`
- WHEN the Gateway is patched to use issuer `https://new-idp.example.com`
- THEN the ConfigMap SHALL be updated with the new OIDC settings
- AND the gateway pods SHALL be restarted via rolling update

#### Scenario: OIDC enabled on previously unauthenticated gateway

- GIVEN a running gateway with no OIDC configuration
- WHEN the Gateway is patched to add OIDC fields
- THEN `allow_unauthenticated_users` SHALL change from `true` to `false`
- AND the gateway SHALL restart

---

## CLI Authentication Flow

```bash
# 1. Get OIDC token (password grant for automation, browser for interactive)
TOKEN=$(curl -sk -X POST \
  "https://${KC_HOST}/realms/hypershell/protocol/openid-connect/token" \
  -d "grant_type=password" \
  -d "client_id=hypershell-frontend" \
  -d "client_secret=${KC_CLIENT_SECRET}" \
  -d "username=admin" \
  -d "password=admin" | python3 -c "import json,sys; print(json.load(sys.stdin)['access_token'])")

# 2. Login to hsctl
hsctl login --token "$TOKEN" --url "$API_URL" --insecure-skip-tls-verify

# 3. Register openshell CLI with gateway
hsctl gateway setup-cli --project tenant-a --gateway-url "$GATEWAY_URL"

# 4. Use openshell CLI
OPENSHELL_GATEWAY_INSECURE=true openshell -g tenant-a-openshell-gateway status
OPENSHELL_GATEWAY_INSECURE=true openshell -g tenant-a-openshell-gateway sandbox create --name demo
```

---

## Debugging Reference

| Symptom | Root Cause | Fix |
|---|---|---|
| `role 'openshell-user' required` | OIDC `roles_claim` misconfigured - JWT has `groups` not `roles` | Set `roles_claim: groups` |
| `Invalid client or Invalid client credentials` | Wrong client_secret or client_id | Check `sso-credentials` Secret |
| Token expires after 5 minutes | Keycloak access token TTL | Use refresh token or increase session timeout |
| `openshell gateway add` opens browser | No `--no-browser` flag | Write `metadata.json` directly, then use `hsctl gateway setup-cli` |
| `GROUPS` env var returns `1000` in bash | Bash builtin collision - `GROUPS` is a reserved readonly array | Use `USER_GROUPS` instead of `GROUPS` for role/group env vars |
| `openshell sandbox create` hangs | Blocking interactive command | Background the command and poll for pod status; use `ExecSandbox` for runner startup |

---

## References

- [OpenShell OIDC User Authentication](https://docs.nvidia.com/openshell/latest/kubernetes/access-control#oidc-user-authentication)
- [OpenShell OIDC Values Reference](https://docs.nvidia.com/openshell/latest/kubernetes/access-control#oidc-values-reference)
- [OpenShell Gateway Auth Reference](https://docs.nvidia.com/openshell/reference/gateway-auth#oidc)
