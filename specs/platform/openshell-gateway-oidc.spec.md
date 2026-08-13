# OpenShell Gateway OIDC Specification

**Date:** 2026-08-05
**Status:** Draft
**Parent:** `openshell-gateway.spec.md` - core gateway provisioning
**Related:** `openshell-gateway-tls.spec.md` - TLS certificate management; `openshell-gateway-routing.spec.md` - external connectivity

---

## Purpose

This specification defines per-gateway OIDC authentication for OpenShell gateways. OIDC enables CLI users and external clients to authenticate via Bearer tokens from an identity provider (e.g., Keycloak). OIDC is the sole authentication mechanism for HyperShell gateways in production.

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

### Verified Keycloak Configuration (ROSA)

```
Realm:      hypershell
Client:     hypershell-frontend (public, standard flow + direct access grants)
Audience:   hypershell-frontend
Roles claim: groups (via group membership, not realm_access.roles)
Groups:     hypershell-admins, hypershell-users
Users:      admin (both groups), developer (hypershell-users only)
```

> **Implementation note:** Keycloak exposes group membership under the `groups` claim (not `realm_access.roles`). The `roles_claim` field in the gateway config maps to wherever the IdP puts role/group information. The naming is upstream OpenShell convention.

---

## Requirements

### Requirement: Gateway OIDC API Fields

The Gateway API resource SHALL accept an optional `oidc` object containing OIDC configuration fields. These fields map directly to the upstream OpenShell `server.oidc.*` helm values.

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `oidc.issuer` | string | Yes (to enable OIDC) | `""` | OIDC issuer URL; empty disables OIDC |
| `oidc.client_id` | string | No | `"openshell-cli"` | Client identifier CLI users present to the IdP; client-facing metadata surfaced by the console, not written to `gateway.toml` server config |
| `oidc.audience` | string | No | `"openshell-cli"` | Expected `aud` claim value in JWT |
| `oidc.jwks_ttl` | integer | No | `3600` | JWKS key cache retention in seconds |
| `oidc.roles_claim` | string | No | `""` | Dot-delimited path to roles array in JWT claims |
| `oidc.admin_role` | string | No | `""` | Role name conferring admin access |
| `oidc.user_role` | string | No | `""` | Role name conferring standard user access |
| `oidc.scopes_claim` | string | No | `""` | Dot-delimited path to scopes array in JWT claims |

The `oidc.issuer`, `oidc.audience`, `oidc.jwks_ttl`, `oidc.roles_claim`, `oidc.admin_role`, `oidc.user_role`, and `oidc.scopes_claim` fields map directly to the upstream OpenShell `server.oidc.*` helm values. `oidc.client_id` is client-facing metadata: the gateway validates `issuer` and `audience`, while CLI users need `client_id` to obtain a token. The console presents `issuer`, `client_id`, and `audience` in the `openshell gateway add` command.

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

#### Scenario: Gateway without OIDC and without platform config

- GIVEN a Gateway resource with no `oidc` field
- AND no platform OIDC issuer is configured on the control plane
- WHEN the GatewayReconciler reconciles
- THEN `gateway.toml` SHALL NOT contain an `[openshell.gateway.oidc]` section
- AND `allow_unauthenticated_users` SHALL remain `true`

---

### Requirement: Default OIDC Configuration from Platform Config

When a Gateway is created without OIDC configuration (empty `oidc.issuer`) and the control plane has a platform OIDC issuer configured, the GatewayReconciler SHALL default the Gateway's `oidc` to the platform issuer with client-facing `client_id` and `audience`, persist the defaulted `oidc` back to the Gateway resource via the API server, and use it to configure the gateway. This makes OIDC the default authentication mode and gives the console and CLI the issuer, client ID, and audience needed to connect. A Gateway that already specifies a non-empty `oidc.issuer` SHALL be left unchanged.

The default values are resolved from control-plane configuration:

| Variable | Default | Description |
|---|---|---|
| `GATEWAY_OIDC_ISSUER` | value of `OIDC_ISSUER` | Issuer URL used to default a gateway's OIDC configuration |
| `GATEWAY_OIDC_CLIENT_ID` | `openshell-cli` | Client identifier surfaced to CLI users |
| `GATEWAY_OIDC_AUDIENCE` | `openshell-cli` | Expected `aud` claim value |

When no platform issuer is resolved (both `GATEWAY_OIDC_ISSUER` and `OIDC_ISSUER` are empty), the reconciler SHALL leave the Gateway unauthenticated as before.

#### Scenario: Gateway without OIDC but with platform issuer

- GIVEN a Gateway resource with no `oidc` field
- AND the control plane has `OIDC_ISSUER` set to `https://keycloak.example.com/realms/hypershell`
- WHEN the GatewayReconciler reconciles
- THEN it SHALL persist `oidc` on the Gateway with issuer `https://keycloak.example.com/realms/hypershell`, `client_id` `openshell-cli`, and `audience` `openshell-cli`
- AND `gateway.toml` SHALL contain the `[openshell.gateway.oidc]` section with `allow_unauthenticated_users = false`
- AND the console `openshell gateway add` command SHALL include the issuer, client ID, and audience

#### Scenario: Gateway with explicit OIDC is not overridden

- GIVEN a Gateway resource whose `oidc.issuer` is already set
- WHEN the GatewayReconciler reconciles
- THEN the reconciler SHALL NOT replace the existing `oidc` configuration with platform defaults

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
  issuer      = "https://keycloak.example.com/realms/hypershell"
  audience    = "hypershell-frontend"
  roles_claim = "groups"
  admin_role  = "hypershell-admins"
  user_role   = "hypershell-users"
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
