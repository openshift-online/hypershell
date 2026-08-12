# OpenShell Gateway Credential Storage Specification

**Date:** 2026-08-10
**Status:** Draft
**Parent:** `openshell-gateway.spec.md` - core gateway provisioning
**Upstream:** [OpenShell PR #2437](https://github.com/NVIDIA/OpenShell/pull/2437) - provider credential storage drivers

---

## Purpose

OpenShell gateways store provider credentials (API keys, tokens, secrets) on behalf of users. Upstream OpenShell v0.0.101 introduced pluggable credential storage drivers: an encrypted database store (default), a Kubernetes Secrets backend, and a HashiCorp Vault backend. HyperShell currently provisions only the default encrypted DB driver (KEK Secret + TOML config). This specification defines how the GatewayReconciler SHALL support per-gateway selection of an external credential storage driver, including the required API fields, TOML configuration, RBAC, and deployment changes.

Credential rotation (KEK re-encryption, DB password rotation) is out of scope and is addressed in [`openshell-gateway-secret-rotation.spec.md`](./openshell-gateway-secret-rotation.spec.md).

---

## Requirements

### Requirement: Credential Driver Configuration Field

The Gateway API resource SHALL include an optional `credential_driver` JSONB field. When omitted or null, the gateway uses the default encrypted database credential store (existing behavior). When present, it selects an external credential storage backend.

| Field | Type | Required | Description |
|---|---|---|---|
| `credential_driver.type` | string | Yes | Driver type: `kubernetes-secrets` or `vault` |
| `credential_driver.kubernetes_secrets.namespace` | string | No | Namespace for credential Secrets. Defaults to the tenant namespace |
| `credential_driver.vault.address` | string | Yes (if vault) | Vault server URL |
| `credential_driver.vault.mount` | string | No | Secrets engine mount path. Default: `secret` |
| `credential_driver.vault.auth_method` | string | No | Authentication method. Default: `kubernetes` |
| `credential_driver.vault.role` | string | Yes (if vault) | Vault role for authentication |
| `credential_driver.vault.kubernetes_auth_mount` | string | No | Vault Kubernetes auth mount path. Default: `kubernetes` |
| `credential_driver.vault.timeout_secs` | int | No | Vault client timeout in seconds. Default: `30` |

#### Scenario: Gateway with default credential storage

- GIVEN a Gateway resource with no `credential_driver` field
- WHEN the GatewayReconciler reconciles
- THEN the gateway.toml SHALL include `[openshell.gateway.credential_storage]` with `key_encryption_key_env`
- AND the KEK Secret SHALL be provisioned (existing behavior)

#### Scenario: Gateway with Kubernetes Secrets driver

- GIVEN a Gateway resource with `credential_driver.type` = `kubernetes-secrets`
- WHEN the GatewayReconciler reconciles
- THEN the gateway.toml SHALL include `credential_drivers = ["kubernetes-secrets"]`
- AND the `[openshell.gateway.credential_storage]` section SHALL be omitted
- AND the KEK Secret SHALL NOT be provisioned

#### Scenario: Gateway with Vault driver

- GIVEN a Gateway resource with `credential_driver.type` = `vault`
- WHEN the GatewayReconciler reconciles
- THEN the gateway.toml SHALL include `credential_drivers = ["vault"]`
- AND the `[openshell.gateway.credential_storage]` section SHALL be omitted
- AND the KEK Secret SHALL NOT be provisioned

---

### Requirement: Credential Driver Validation

The GatewayReconciler SHALL validate the `credential_driver` configuration before reconciliation.

- `credential_driver.type` SHALL be one of: `kubernetes-secrets`, `vault`
- When `type` is `vault`, the fields `address` and `role` SHALL be required
- Unknown fields SHALL be rejected

#### Scenario: Invalid credential driver type

- GIVEN a Gateway resource with `credential_driver.type` = `s3`
- WHEN the GatewayReconciler validates the configuration
- THEN it SHALL reject the configuration with error: `unsupported credential driver type "s3"; supported: kubernetes-secrets, vault`

#### Scenario: Vault driver missing required fields

- GIVEN a Gateway resource with `credential_driver.type` = `vault` and no `address`
- WHEN the GatewayReconciler validates the configuration
- THEN it SHALL reject the configuration with error: `vault credential driver requires "address" and "role"`

---

### Requirement: Credential Driver TOML Generation

The GatewayReconciler SHALL generate the `gateway.toml` credential configuration based on the selected driver.

**Default (no `credential_driver`):**

```toml
[openshell.gateway.credential_storage]
key_encryption_key_env = "OPENSHELL_GATEWAY_CREDENTIAL_KEY_ENCRYPTION_KEY"
```

**Kubernetes Secrets:**

```toml
credential_drivers = ["kubernetes-secrets"]

[openshell.credential_drivers.kubernetes-secrets]
namespace = "<configured-or-tenant-namespace>"
```

**Vault:**

```toml
credential_drivers = ["vault"]

[openshell.credential_drivers.vault]
address = "<address>"
mount = "<mount>"
auth_method = "<auth_method>"
role = "<role>"
kubernetes_auth_mount = "<kubernetes_auth_mount>"
timeout_secs = <timeout_secs>
```

The `[openshell.gateway.credential_storage]` section and the `[openshell.credential_drivers.*]` sections are mutually exclusive. When an external driver is selected, only the driver section SHALL appear.

---

### Requirement: RBAC for Kubernetes Secrets Driver

When `credential_driver.type` is `kubernetes-secrets`, the GatewayReconciler SHALL provision a Role and RoleBinding granting the gateway ServiceAccount access to Secrets in the credential namespace.

**Role:**

```yaml
rules:
  - apiGroups: [""]
    resources: ["secrets"]
    verbs: ["get", "create", "patch", "delete"]
```

**RoleBinding:**
- Subject: ServiceAccount `openshell-gateway` in the tenant namespace
- RoleRef: the credential secrets Role

These resources SHALL carry the label `hypershell.redhat.io/managed: "true"` and SHALL be deleted when the credential driver is changed or the gateway is deleted.

When the credential namespace differs from the tenant namespace, the Role and RoleBinding SHALL be created in the credential namespace.

#### Scenario: RBAC provisioned for Kubernetes Secrets

- GIVEN a Gateway with `credential_driver.type` = `kubernetes-secrets`
- WHEN the GatewayReconciler reconciles
- THEN a Role `openshell-gateway-credential-secrets` SHALL exist in the credential namespace
- AND a RoleBinding `openshell-gateway-credential-secrets` SHALL bind it to the `openshell-gateway` ServiceAccount

#### Scenario: RBAC cleaned up on driver change

- GIVEN a Gateway that previously used `kubernetes-secrets` and is updated to use `vault`
- WHEN the GatewayReconciler reconciles
- THEN the credential secrets Role and RoleBinding SHALL be deleted

#### Scenario: Default driver does not create credential RBAC

- GIVEN a Gateway with no `credential_driver` field
- WHEN the GatewayReconciler reconciles
- THEN no credential secrets Role or RoleBinding SHALL exist

---

### Requirement: Vault Driver Deployment Configuration

When `credential_driver.type` is `vault` with `auth_method` = `kubernetes`, the GatewayReconciler SHALL mount a projected ServiceAccount token volume in the gateway Deployment for Vault authentication.

```yaml
volumes:
  - name: vault-sa-token
    projected:
      sources:
        - serviceAccountToken:
            path: token
            expirationSeconds: 3600
            audience: vault

volumeMounts:
  - name: vault-sa-token
    mountPath: /var/run/secrets/vault
    readOnly: true
```

The TOML config SHALL set `service_account_token_path = "/var/run/secrets/vault/token"`.

When the credential driver is not `vault`, this volume and mount SHALL NOT be present.

#### Scenario: Vault SA token mounted

- GIVEN a Gateway with `credential_driver.type` = `vault` and `auth_method` = `kubernetes`
- WHEN the GatewayReconciler reconciles
- THEN the gateway Deployment SHALL include a projected ServiceAccount token volume with audience `vault`
- AND the volume SHALL be mounted at `/var/run/secrets/vault`

---

### Requirement: KEK Provisioning Conditional on Driver

The GatewayReconciler SHALL only provision the KEK Secret (`openshell-gateway-credential-kek`) when the default encrypted database credential store is in use.

- When `credential_driver` is absent or null: provision KEK (existing behavior)
- When `credential_driver` is present: skip KEK provisioning
- When `credential_driver` is removed from an existing gateway (revert to default): provision KEK if it does not exist

The KEK env var (`OPENSHELL_GATEWAY_CREDENTIAL_KEY_ENCRYPTION_KEY`) in the Deployment SHALL only be present when the default driver is in use.

#### Scenario: KEK skipped for external driver

- GIVEN a Gateway with `credential_driver.type` = `kubernetes-secrets`
- WHEN the GatewayReconciler reconciles
- THEN the Secret `openshell-gateway-credential-kek` SHALL NOT be created
- AND the env var `OPENSHELL_GATEWAY_CREDENTIAL_KEY_ENCRYPTION_KEY` SHALL NOT be in the Deployment

---

### Requirement: Credential Driver Immutability

The `credential_driver` configuration on a Gateway SHALL be immutable after the gateway has stored provider credentials. Changing the driver would orphan credentials in the previous backend.

#### Scenario: Attempt to change credential driver

- GIVEN a Gateway with `credential_driver.type` = `vault` that has active provider credentials
- WHEN a user attempts to change `credential_driver.type` to `kubernetes-secrets`
- THEN the API server SHALL reject the update with error: `credential driver cannot be changed after provider credentials have been stored; migrate credentials manually first`

> **Phase 2 (reserved):** Automated credential migration between drivers.

---

## Configuration Examples

### Gateway with Kubernetes Secrets credential storage

```yaml
kind: Gateway
name: openshell-gateway
project: tenant-a
image: ghcr.io/nvidia/openshell/gateway:0.0.101
serverDnsNames:
  - openshell-gateway.tenant-a.svc.cluster.local
credential_driver:
  type: kubernetes-secrets
  kubernetes_secrets:
    namespace: tenant-a
```

### Gateway with Vault credential storage

```yaml
kind: Gateway
name: openshell-gateway
project: tenant-a
image: ghcr.io/nvidia/openshell/gateway:0.0.101
serverDnsNames:
  - openshell-gateway.tenant-a.svc.cluster.local
credential_driver:
  type: vault
  vault:
    address: https://vault.example.com
    role: openshell-gateway-tenant-a
    mount: secret
    auth_method: kubernetes
```

### Gateway with default credential storage (no change from today)

```yaml
kind: Gateway
name: openshell-gateway
project: tenant-a
image: ghcr.io/nvidia/openshell/gateway:0.0.101
serverDnsNames:
  - openshell-gateway.tenant-a.svc.cluster.local
```

---

## References

- [OpenShell PR #2437 - Provider credential storage drivers](https://github.com/NVIDIA/OpenShell/pull/2437)
- [OpenShell Helm Chart - Credential driver values](https://github.com/NVIDIA/OpenShell/tree/main/deploy/helm/openshell)
- [`openshell-gateway-database.spec.md`](./openshell-gateway-database.spec.md) - KEK provisioning (default driver)
