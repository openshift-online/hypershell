# OpenShell Gateway Secret Rotation Specification

**Date:** 2026-08-12
**Status:** Draft
**Parent:** `openshell-gateway.spec.md` - core gateway provisioning
**Related:** `openshell-gateway-database.spec.md` - database credentials; `openshell-gateway-credentials.spec.md` - credential storage drivers; `openshell-gateway-tls.spec.md` - TLS certificates

---

## Purpose

This specification records how each secret of a gateway is renewed today, and who renews it. HyperShell itself rotates none of them.

| Secret | Created by | Renewal today |
|---|---|---|
| Database password (`openshell-gateway-db-credentials`) | Control plane | Not rotated; see [`openshell-gateway-database.spec.md`](./openshell-gateway-database.spec.md) Requirement: No Credential Rotation. When provisioning repair writes a new password for a lost Secret, the running gateway is not restarted. It keeps the old password, which it reads from the `OPENSHELL_DB_URL` environment variable, until its pod next restarts. |
| Credential KEK (`openshell-gateway-credential-storage-key-encryption-key`) | OpenShell chart, on first install of a gateway that uses the KEK-encrypted database store | Not rotated. Every later Helm upgrade reuses the existing value. |
| TLS certificates (`openshell-server-tls`, `openshell-client-tls`) | cert-manager, from the chart's Certificate resources | Renewed by cert-manager; the gateway reloads them without a restart (OSG-ROT-03) |
| Provider credentials | The gateway, through its credential driver | Depends on the driver (OSG-ROT-04) |

---

## Requirements

### Requirement: OSG-ROT-03 -- TLS Certificate Renewal

cert-manager renews the gateway's TLS certificates. The gateway picks up a renewed certificate without a pod restart. HyperShell takes no action.

#### How it works

1. cert-manager renews each Certificate before it expires. The chart defaults are a lifetime of `8760h` and renewal `720h` before expiry.
2. cert-manager writes the new certificate and key into the TLS Secret (`openshell-server-tls`).
3. The gateway pod mounts that Secret as a volume, without `subPath`, so the kubelet updates the mounted files in place.
4. The OpenShell gateway watches its certificate, key and CA files and reloads them when they change.

#### Scenario: OSG-ROT-03.01 -- cert-manager renews server certificate

- GIVEN cert-manager renews the `openshell-server` Certificate
- AND the Secret `openshell-server-tls` is updated with new cert/key data
- WHEN the kubelet updates the files mounted in the gateway pod
- THEN the gateway SHALL serve the renewed certificate without a pod restart
- AND the control plane SHALL take no action

#### Operational note: Cert rotation is destructive

When TLS certificates are regenerated, connected sandbox supervisors lose trust in the gateway. Their client certificates, signed by the old CA, will cause `DecryptError`. Sandboxes must be recreated after cert rotation. This is documented in `openshell-gateway-tls.spec.md` and is inherent to the per-tenant CA model.

---

### Requirement: OSG-ROT-04 -- Provider Credential Rotation by Driver Type

When a gateway uses an external credential storage driver (`kubernetes-secrets` or `vault`), the provider credentials (API keys, tokens) are stored outside the gateway database. Each driver has its own rotation model. The control plane's responsibility varies by driver.

#### Rotation matrix by driver

| Driver | What stores credentials | Who rotates | HyperShell responsibility |
|---|---|---|---|
| Default (KEK) | Gateway PostgreSQL (encrypted) | Nobody: the KEK is not rotated | None |
| `kubernetes-secrets` | Kubernetes Secrets in credential namespace | Upstream gateway or external operator | RBAC only: ensure Role/RoleBinding remain valid |
| `vault` | HashiCorp Vault | Vault (dynamic secrets, lease renewal, TTL) | Auth token only: projected SA token auto-rotates |

#### Kubernetes Secrets driver

Provider credentials are stored as individual Kubernetes Secrets in the credential namespace. The gateway reads/writes them directly via the Kubernetes API using its ServiceAccount.

**What rotates automatically:**
- The gateway ServiceAccount token is managed by Kubernetes and rotates automatically (bound service account tokens have a configurable lifetime, default ~1 hour)
- The RBAC (Role/RoleBinding) provisioned by HyperShell does not expire

**What does NOT rotate automatically:**
- The provider credentials themselves (API keys, tokens stored as Secrets). These are managed by the gateway on behalf of users. Rotation is the user's responsibility -- they update or re-register their provider credentials through the gateway API

**HyperShell control plane actions:**
- None required for rotation. The RBAC is persistent and the SA token is auto-managed by Kubernetes
- If the ServiceAccount is deleted and recreated (e.g., namespace recreated), HyperShell re-provisions the RBAC on the next reconciliation cycle

#### Scenario: OSG-ROT-04.01 -- Kubernetes Secrets driver -- SA token rotation

- GIVEN a gateway with `credential_driver.type` = `kubernetes-secrets`
- WHEN the Kubernetes control plane rotates the bound ServiceAccount token
- THEN the gateway SHALL automatically use the new token (mounted via projected volume)
- AND no HyperShell intervention is required

#### Vault driver

Provider credentials are stored in HashiCorp Vault. The gateway authenticates to Vault using a projected Kubernetes ServiceAccount token and accesses secrets via the Vault API.

**What rotates automatically:**
- The projected SA token (configured with `expirationSeconds: 3600`) is auto-rotated by Kubernetes and the kubelet re-projects a fresh token before expiry
- Vault leases and dynamic secrets are managed by Vault's own TTL and renewal mechanisms
- The Vault client in the gateway handles token renewal transparently

**What does NOT rotate automatically:**
- The Vault role binding and policy -- these are configured by the Vault administrator, outside HyperShell's scope
- Provider credentials stored as static Vault secrets (KV v2) -- rotation is the user's or Vault operator's responsibility

**What HyperShell must ensure:**
- The projected SA token volume is correctly configured with the `vault` audience
- The control plane sets the `credential_driver` field on the Gateway API resource from its configuration (`credential_driver` is control-plane-managed and is not user- or import-writable; see `data-model.spec.md` PLAT-DATA-23). A changed Vault configuration reaches a gateway only when that gateway is next deployed (see OSG-ROT-04.03)

#### Scenario: OSG-ROT-04.02 -- Vault driver -- SA token renewal

- GIVEN a gateway with `credential_driver.type` = `vault` and `auth_method` = `kubernetes`
- AND the projected SA token has `expirationSeconds: 3600`
- WHEN the token approaches expiry
- THEN the kubelet SHALL project a fresh token at the mount path
- AND the gateway's Vault client SHALL re-authenticate using the new token
- AND no HyperShell intervention is required

#### Scenario: OSG-ROT-04.03 -- Vault driver -- Vault configuration change

- GIVEN a gateway with `credential_driver.type` = `vault`
- AND the Vault server address changes (e.g., migration to a new Vault cluster)
- WHEN the cluster operator updates the control-plane deployment `GATEWAY_VAULT_ADDRESS`
  override (Vault is a black box; there is no per-gateway `credential_driver.vault.address` field)
- THEN gateways provisioned after the change SHALL use the new address
- AND an existing gateway SHALL keep the old address until a desired-state change re-deploys it, because the GatewayReconciler skips converged gateways
- AND when a gateway is re-deployed, the Helm upgrade SHALL render the new address into the gateway config, and the chart's `checksum/gateway-config` pod annotation SHALL roll the gateway pod

#### Scenario: OSG-ROT-04.04 -- Vault dynamic secret lease renewal

- GIVEN a gateway using Vault dynamic secrets with a TTL
- WHEN a dynamic secret lease approaches expiry
- THEN the gateway's Vault client SHALL renew the lease directly with Vault
- AND no HyperShell intervention is required (this is entirely between the gateway and Vault)

---

## References

- [`openshell-gateway-database.spec.md`](./openshell-gateway-database.spec.md) - Database provisioning, credential security, and the no-rotation rule
- [`openshell-gateway-credentials.spec.md`](./openshell-gateway-credentials.spec.md) - Credential storage drivers (KEK conditional provisioning)
- [`openshell-gateway-tls.spec.md`](./openshell-gateway-tls.spec.md) - TLS certificate management via cert-manager
- [`security.spec.md`](../standards/security/security.spec.md) - Security standards
