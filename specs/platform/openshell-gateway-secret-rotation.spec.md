# OpenShell Gateway Secret Rotation Specification

**Date:** 2026-08-12
**Status:** Draft
**Parent:** `openshell-gateway.spec.md` - core gateway provisioning
**Related:** `openshell-gateway-database.spec.md` - database credentials; `openshell-gateway-credentials.spec.md` - credential storage drivers; `openshell-gateway-tls.spec.md` - TLS certificates

---

The SQL rotation procedure below applies to controller-local gateways. Before
migration, CNPG retains its operator-based rotation and deployment/external
annotations retain their existing behavior. Transfer SHALL NOT activate an old,
previously inert rotation request. See the
[migration contract](./gateway-database-migration.spec.md).

## Purpose

The control plane SHALL support explicit rotation of gateway database passwords
without changing the configured PostgreSQL destination. It SHALL preserve gateway
credentials during ordinary reconciliation. This specification also defines the
rotation boundaries for credential encryption keys and TLS certificates.

This specification defines the rotation strategy for the three categories of secrets managed by the control plane:

| Secret | Current Behavior | Rotation Strategy |
|---|---|---|
| Database password (`openshell-gateway-db-credentials`) | Generated once, never rotated | Annotation-triggered; controller updates the SQL role through its local administrative connection |
| Credential KEK (`openshell-gateway-credential-kek`) | Generated once, never rotated | Day-2 follow-up (re-encryption workflow requires gateway cooperation) |
| TLS certificates (`openshell-server-tls`, `openshell-client-tls`) | cert-manager automatic renewal | Already handled; this spec ensures the config-hash annotation covers TLS secrets |
| Provider credentials (kubernetes-secrets driver) | Stored as K8s Secrets by gateway | User/operator responsibility; SA token auto-rotates |
| Provider credentials (vault driver) | Stored in Vault by gateway | Vault manages leases/TTLs; projected SA token auto-rotates |

The database password rotation applies to all gateways regardless of credential driver. Provider credential rotation depends on the driver and is largely delegated to the platform (Kubernetes, Vault) or the user.

---

## Requirements

### Requirement: Database Password Rotation

The GatewayReconciler SHALL support on-demand database credential rotation triggered by an annotation on the Gateway API resource.

#### Trigger

To request rotation, an operator sets the annotation:

```
hypershell.redhat.io/rotate-db-credentials: "<RFC3339-timestamp>"
```

The reconciler compares this value against the annotation `hypershell.redhat.io/last-db-rotation` stored on the database credentials Secret. When the values differ (or `last-db-rotation` is absent), rotation proceeds.

#### Rotation Procedure

For each requested rotation, the assigned controller SHALL:

1. Generate a password with at least 256 bits of cryptographic randomness and
   persist it as a protected pending rotation before changing SQL state.
2. Use its configured administrative connection to change only the gateway's
   SQL login role password. This SHALL use the same behavior on RDS and CNPG.
3. Update `openshell-gateway-db-credentials` with the new password and connection
   URI. Record `hypershell.redhat.io/last-db-rotation` only after SQL and the
   gateway Secret agree. Remove the pending rotation after completion.
4. Cause a rolling restart through the database credential config hash.

The controller SHALL serialize rotation with other operations for that gateway.
A retry SHALL reuse the persisted pending password. It SHALL NOT generate a new
password for every attempt or expose either password in logs, events, or SQL
error text. Pending credentials SHALL have the same access limits as active ones.

#### Scenario: Successful database password rotation

- GIVEN a gateway has a new rotation annotation
- WHEN its assigned controller completes rotation
- THEN the SQL role and gateway Secret SHALL contain the new password
- AND the Secret SHALL record the completed rotation annotation
- AND the gateway Deployment SHALL restart with the new credentials
- AND the database contents and destination SHALL remain unchanged

#### Scenario: Rotation already completed

- GIVEN the requested rotation matches the Secret's completed rotation annotation
- WHEN the controller reconciles the gateway
- THEN it SHALL preserve the current password and skip rotation

#### Scenario: No rotation annotation

- GIVEN a gateway has no rotation annotation
- WHEN the controller reconciles the gateway
- THEN it SHALL preserve its existing credentials

### Requirement: Database Rotation Failure Handling

A failed rotation SHALL remain visible and retryable. Before a confirmed SQL
password change, the controller SHALL NOT publish the new active gateway Secret.
If SQL succeeds but Secret persistence fails, new gateway connections can fail
until the retry completes. The system SHALL report this condition and SHALL NOT
claim uninterrupted service or successful rotation. A controller restart SHALL
resume the same pending rotation on the original destination.

#### Scenario: SQL password update fails

- GIVEN a pending rotation and a failed SQL password update
- WHEN the controller handles the failure
- THEN it SHALL leave the active gateway Secret unchanged
- AND it SHALL retain the pending rotation and retry with a redacted error

#### Scenario: Secret update fails after SQL succeeds

- GIVEN SQL accepted the pending password but the gateway Secret update failed
- WHEN the controller restarts and retries
- THEN it SHALL reuse the pending password and complete the Secret update
- AND it SHALL restart the gateway only with credentials that match SQL

---

### Requirement: Config-Hash Coverage for Database Credentials

The existing `applyConfigHashAnnotation` function SHALL include the database credentials Secret (`openshell-gateway-db-credentials`) in the hash computation, in addition to the ConfigMap and TLS secret it already covers.

This ensures that when the database password is rotated (Secret content changes), the Deployment pod template hash changes, triggering a rolling restart.

#### Scenario: Config-hash includes database credentials

- GIVEN the config-hash is computed from ConfigMap `openshell-gateway-config`, Secret `openshell-server-tls`, and Secret `openshell-gateway-db-credentials`
- WHEN the database credentials Secret is updated during rotation
- THEN the config-hash value SHALL change
- AND the Deployment SHALL perform a rolling restart

---

### Requirement: Rotation Uses the Local PostgreSQL Contract

Rotation SHALL use the controller-local administrative connection and TLS policy
defined in the [database specification](./openshell-gateway-database.spec.md).
It SHALL NOT modify CNPG password Secrets, DatabaseRole resources, or server
infrastructure. SQL statements and errors SHALL NOT disclose passwords.

#### Scenario: Rotate a gateway on RDS

- GIVEN the controller uses an installation-supplied RDS server
- WHEN an operator requests gateway password rotation
- THEN rotation SHALL complete without a CNPG API or operator
- AND it SHALL affect only that gateway's SQL role and credentials

---

### Requirement: KEK Rotation (Day-2, Deferred)

KEK rotation is explicitly deferred to a follow-up specification. This section documents the design constraints and why it is not a day-1 requirement.

#### Why KEK rotation is complex

Rotating the KEK requires re-encrypting all credential handles stored in the gateway's database:

1. The control plane does not have access to the encrypted credentials -- they are stored in the gateway's own PostgreSQL database
2. The gateway itself must decrypt with the old KEK and re-encrypt with the new KEK
3. This requires either:
   - A re-encryption API endpoint on the gateway (the gateway receives the new KEK, performs re-encryption, then the old KEK is removed)
   - Or the control plane connecting directly to the gateway database and performing re-encryption (requires knowledge of the encryption scheme)

Option (a) is preferred because it keeps encryption logic in the gateway where it belongs.

#### Interim mitigation

- The KEK is stored in a Kubernetes Secret with restricted access (only the gateway ServiceAccount)
- Gateways using external credential drivers (`kubernetes-secrets` or `vault`) do not use a KEK at all
- If a KEK is compromised, the operator can provision a new gateway and migrate workloads

#### Day-2 requirements (reserved)

When KEK rotation is implemented, it SHALL:

- Accept a rotation trigger via annotation (same pattern as DB rotation)
- Call the gateway's re-encryption API with the new KEK
- Wait for confirmation that re-encryption completed
- Update the KEK Secret atomically
- Restart the gateway pod to pick up the new KEK

---

### Requirement: TLS Certificate Rotation

TLS certificate rotation is already handled by cert-manager. This section documents the existing behavior and confirms the config-hash mechanism ensures pod restarts.

#### How it works today

1. cert-manager monitors Certificate resources and renews them before expiry (default: 2/3 of the certificate lifetime)
2. cert-manager updates the TLS Secret (`openshell-server-tls`) with new certificate and key material
3. The `applyConfigHashAnnotation` function includes `openshell-server-tls` in the Deployment pod template hash
4. On the next reconciliation cycle, the hash changes and the Deployment rolls out new pods

#### Scenario: cert-manager renews server certificate

- GIVEN cert-manager renews the `openshell-server` Certificate
- AND the Secret `openshell-server-tls` is updated with new cert/key data
- WHEN the GatewayReconciler next reconciles the gateway
- THEN the config-hash SHALL change (because TLS secret content changed)
- AND the gateway Deployment SHALL perform a rolling restart

#### Recommendation: Set `rotationPolicy: Always` on tenant certificates

The Kind development certificates already use `rotationPolicy: Always`. The production tenant certificates created by `reconcileCertManagerResources` SHOULD also set this field on the CA Certificate to ensure private keys are regenerated on renewal.

```yaml
spec:
  rotationPolicy: Always
```

#### Operational note: Cert rotation is destructive

When TLS certificates are regenerated, connected sandbox supervisors lose trust in the gateway. Their client certificates, signed by the old CA, will cause `DecryptError`. Sandboxes must be recreated after cert rotation. This is documented in `openshell-gateway-tls.spec.md` and is inherent to the per-tenant CA model.

---

### Requirement: Provider Credential Rotation by Driver Type

When a gateway uses an external credential storage driver (`kubernetes-secrets` or `vault`), the provider credentials (API keys, tokens) are stored outside the gateway database. Each driver has its own rotation model. The control plane's responsibility varies by driver.

#### Rotation matrix by driver

| Driver | What stores credentials | Who rotates | HyperShell responsibility |
|---|---|---|---|
| Default (KEK) | Gateway PostgreSQL (encrypted) | KEK rotation re-encrypts in-place (Day-2) | Full: KEK Secret, re-encryption workflow |
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

#### Scenario: Kubernetes Secrets driver -- SA token rotation

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
- If the Vault configuration changes (address, role, mount), the operator updates the `credential_driver` field on the Gateway API resource, and the reconciler regenerates the gateway.toml and restarts the pod

#### Scenario: Vault driver -- SA token renewal

- GIVEN a gateway with `credential_driver.type` = `vault` and `auth_method` = `kubernetes`
- AND the projected SA token has `expirationSeconds: 3600`
- WHEN the token approaches expiry
- THEN the kubelet SHALL project a fresh token at the mount path
- AND the gateway's Vault client SHALL re-authenticate using the new token
- AND no HyperShell intervention is required

#### Scenario: Vault driver -- Vault configuration change

- GIVEN a gateway with `credential_driver.type` = `vault`
- AND the Vault server address changes (e.g., migration to a new Vault cluster)
- WHEN the operator updates the Gateway's `credential_driver.vault.address` field
- THEN the GatewayReconciler SHALL regenerate gateway.toml with the new address
- AND the config-hash SHALL change, triggering a rolling restart
- AND the gateway SHALL connect to the new Vault server on startup

#### Scenario: Vault dynamic secret lease renewal

- GIVEN a gateway using Vault dynamic secrets with a TTL
- WHEN a dynamic secret lease approaches expiry
- THEN the gateway's Vault client SHALL renew the lease directly with Vault
- AND no HyperShell intervention is required (this is entirely between the gateway and Vault)

---

### Requirement: Interaction Between Credential Driver and DB Password Rotation

Database password rotation (the `openshell-gateway-db-credentials` Secret) is independent of the credential storage driver. The database password secures the connection between the gateway and its PostgreSQL instance; the credential driver determines where *provider* credentials (user API keys, tokens) are stored.

- DB password rotation SHALL work identically regardless of the credential driver in use
- A gateway using `kubernetes-secrets` or `vault` still has a PostgreSQL database for its own state (sessions, audit log, etc.) and that database password is still subject to rotation

#### Scenario: DB password rotation with Vault credential driver

- GIVEN a gateway with `credential_driver.type` = `vault`
- AND the operator triggers DB password rotation via annotation
- WHEN the GatewayReconciler reconciles
- THEN it SHALL rotate the database password (update SQL role, update gateway credentials Secret, rolling restart)
- AND the Vault credential configuration SHALL remain unchanged
- AND provider credentials in Vault SHALL remain accessible after the gateway restarts

---

## References

- [`openshell-gateway-database.spec.md`](./openshell-gateway-database.spec.md) - Database provisioning and credential security
- [`openshell-gateway-credentials.spec.md`](./openshell-gateway-credentials.spec.md) - Credential storage drivers (KEK conditional provisioning)
- [`openshell-gateway-tls.spec.md`](./openshell-gateway-tls.spec.md) - TLS certificate management via cert-manager
- [`security.spec.md`](../standards/security/security.spec.md) - Security standards
