# OpenShell Gateway Credential Storage Specification

**Date:** 2026-10-05
**Status:** Active
**Parent:** `openshell-gateway.spec.md` - core gateway provisioning
**Upstream:** [OpenShell PR #2437](https://github.com/NVIDIA/OpenShell/pull/2437) - provider credential storage drivers (shipped in this repo's pinned `OPENSHELL_TAG`)
**GitOps counterpart:** `hypershell-gitops` `specs/openshell-gateway-vault.spec.md` - stands up the per-cluster Vault this spec addresses (HYPERSHELL-369)

---

## Purpose

OpenShell gateways store **provider credentials** (the API keys, tokens and secrets a
user registers so their sandboxes can reach external model/tool providers) on behalf of
users. Upstream OpenShell ships pluggable credential storage drivers: an encrypted
database store, a Kubernetes Secrets backend, and a HashiCorp **Vault** backend. This
specification defines how the GatewayReconciler selects and configures a credential
storage driver per gateway: the API field, validation, `gateway.toml` generation, RBAC,
deployment wiring, Vault serving-CA propagation, fail-closed behavior, and cleanup.

**Default backend (HYPERSHELL-369).** New gateways store provider credentials in
**Vault** by default. Each cluster that hosts gateways runs its own dedicated Vault for
provider secrets (stood up by `hypershell-gitops`; see the GitOps counterpart). The
Vault is **convention-addressed**: its address, KV mount, auth mount, shared role and
serving CA are fixed coordinates the control plane already knows, so a gateway becomes
Vault-backed with **no user input**. The former default - a key-encryption-key (KEK)
encrypted database store - remains available as an explicit opt-in (`kek-db`), as does
the Kubernetes Secrets driver (`kubernetes-secrets`).

The control plane never holds a provider secret value and never administers Vault. Only
the gateway's own process reads and writes KV secrets, authenticating to Vault with its
ServiceAccount; the control plane only ever acts **as the gateway's own identity** (for
the fail-closed reachability probe and for KV cleanup on deletion), never as a Vault
administrator. Per-gateway isolation is enforced by a single namespace-**templated**
Vault policy that resolves each caller's own cryptographically-verified namespace at
login, so one gateway can never read another's secrets.

Credential rotation (KEK re-encryption, DB password rotation) is out of scope and is
addressed in [`openshell-gateway-secret-rotation.spec.md`](./openshell-gateway-secret-rotation.spec.md).
Rotating a provider secret's **value** is OpenShell's own `provider` mechanism, not a
HyperShell concern, for every driver.

---

## Requirements

### Requirement: Credential Driver Configuration Field

The Gateway API resource SHALL include an optional `credential_driver` JSONB field that
selects the provider-credential storage backend. The field is set only through
`hsctl create gateway --credential-driver <value>` or a raw `POST`/`PATCH /gateways`
call; it is not exposed in the web console gateway form or in the end-user `openshell`
CLI, and it carries the same RBAC as any other gateway field.

**Default resolution** (what an omitted or explicit field resolves to):

| `credential_driver` | Resolves to | Notes |
|---|---|---|
| omitted / null | `vault` | **Default (HYPERSHELL-369).** Convention-addressed per-cluster Vault; no user input required |
| `type: vault` | `vault` | Explicit; same outcome as omitted. MAY carry optional overrides |
| `type: kubernetes-secrets` | Kubernetes Secrets | Explicit opt-in; unchanged |
| `type: kek-db` | KEK-encrypted database | Explicit opt-in to the former default; provisions the KEK Secret |

> **Default flip (HYPERSHELL-369).** `vault` is the **flat default**: an omitted
> `credential_driver` resolves to `vault` in control-plane code, with no per-deployment
> selector. **Prerequisite:** every gateway-hosting cluster must already run its
> per-cluster Vault (stood up by `hypershell-gitops`) before this default takes effect -
> so the GitOps per-cluster Vault rollout MUST merge before (or together with) this
> change. An *explicit* `credential_driver` is always honored, so a cluster that does
> not yet have its Vault should pin new gateways to `kek-db` explicitly until it does.

An **unspecified** `type` - the field omitted/null, *or* an object with an empty
`type` (`credential_driver: {}`) - is treated the same as omitted and resolves to the
default (`vault`, per the table above). An empty object behaves identically to no
field at all.

When `type` is `vault`, every sub-field is **optional**: omitted sub-fields take the
control-plane convention defaults (see Requirement: Vault Convention Defaults). The
fields exist for non-standard topologies (for example a dev cluster or an external
Vault) and SHALL NOT be required for the common case.

| Field | Type | Required | Description |
|---|---|---|---|
| `credential_driver.type` | string | No | `vault` (default), `kubernetes-secrets`, or `kek-db` |
| `credential_driver.kubernetes_secrets.namespace` | string | No | Namespace for credential Secrets. Defaults to the tenant namespace |
| `credential_driver.vault.address` | string | No | Vault server URL. Default: the convention address |
| `credential_driver.vault.mount` | string | No | KV secrets engine mount. Default: `secret` |
| `credential_driver.vault.auth_method` | string | No | Authentication method. Default: `kubernetes` |
| `credential_driver.vault.role` | string | No | Vault role. Default: `openshell-gateway` |
| `credential_driver.vault.kubernetes_auth_mount` | string | No | Vault Kubernetes auth mount. Default: `kubernetes` |
| `credential_driver.vault.timeout_secs` | int | No | Vault client timeout in seconds. Default: `30` |

#### Scenario: Gateway created with no credential_driver defaults to Vault

- GIVEN a Gateway resource with no `credential_driver` field
- WHEN the GatewayReconciler reconciles
- THEN the gateway SHALL be configured for the `vault` driver using the convention defaults
- AND the `gateway.toml` SHALL include `credential_drivers = ["vault"]`
- AND the KEK Secret SHALL NOT be provisioned
- AND the `[openshell.gateway.credential_storage]` section SHALL be omitted

#### Scenario: Gateway with explicit Vault driver

- GIVEN a Gateway resource with `credential_driver.type` = `vault`
- WHEN the GatewayReconciler reconciles
- THEN the gateway SHALL be configured identically to the omitted-field case, honoring
  any supplied overrides in `credential_driver.vault`

#### Scenario: Gateway with explicit KEK database driver

- GIVEN a Gateway resource with `credential_driver.type` = `kek-db`
- WHEN the GatewayReconciler reconciles
- THEN the `gateway.toml` SHALL include `[openshell.gateway.credential_storage]` with `key_encryption_key_env`
- AND the KEK Secret SHALL be provisioned
- AND no `credential_drivers` array SHALL be emitted

#### Scenario: Gateway with Kubernetes Secrets driver

- GIVEN a Gateway resource with `credential_driver.type` = `kubernetes-secrets`
- WHEN the GatewayReconciler reconciles
- THEN the `gateway.toml` SHALL include `credential_drivers = ["kubernetes-secrets"]`
- AND the `[openshell.gateway.credential_storage]` section SHALL be omitted
- AND the KEK Secret SHALL NOT be provisioned

---

### Requirement: Credential Driver Validation

The GatewayReconciler SHALL validate the `credential_driver` configuration before
reconciliation.

- `credential_driver.type`, when present, SHALL be one of: `vault`, `kubernetes-secrets`, `kek-db`.
- A `vault` driver SHALL NOT require any sub-field; supplied sub-fields SHALL be
  validated (`address`, when present, SHALL be a well-formed URL; a non-loopback
  `address` SHALL use `https`).
- Unknown fields SHALL be rejected.

#### Scenario: Invalid credential driver type

- GIVEN a Gateway resource with `credential_driver.type` = `s3`
- WHEN the GatewayReconciler validates the configuration
- THEN it SHALL reject the configuration with error: `unsupported credential driver type "s3"; supported: vault, kubernetes-secrets, kek-db`

#### Scenario: Vault driver with no sub-fields is valid

- GIVEN a Gateway resource with `credential_driver.type` = `vault` and no `vault` sub-fields
- WHEN the GatewayReconciler validates the configuration
- THEN it SHALL accept the configuration
- AND it SHALL apply the convention defaults for address, mount, role, and auth

#### Scenario: Vault override with a plaintext address is rejected

- GIVEN a Gateway with `credential_driver.vault.address` = `http://vault.example.com:8200`
- WHEN the GatewayReconciler validates the configuration
- THEN it SHALL reject the configuration because a non-loopback Vault address MUST use `https`

---

### Requirement: Vault Convention Defaults

When the resolved driver is `vault`, the GatewayReconciler SHALL fill every unset Vault
coordinate from fixed conventions, so a gateway is Vault-backed without user input. The
conventions match the per-cluster Vault stood up by `hypershell-gitops` (see the GitOps
Contract section). The address and serving-CA source MAY be overridden by control-plane
deployment configuration for non-standard topologies; any field MAY be overridden
per-gateway via `credential_driver.vault.*`.

| Coordinate | Convention default | Override |
|---|---|---|
| address | `https://openshell-gateway-vault.openshell-vault.svc:8200` | `GATEWAY_VAULT_ADDRESS` env; `vault.address` |
| mount (KV v2) | `secret` | `vault.mount` |
| KV version | `2` | - |
| auth method | `kubernetes` | `vault.auth_method` |
| Vault role | `openshell-gateway` | `GATEWAY_VAULT_ROLE` env; `vault.role` |
| Kubernetes auth mount | `kubernetes` | `vault.kubernetes_auth_mount` |
| SA token path | `/var/run/secrets/vault/token` | - |
| SA token audience | `vault` | - |
| serving-CA source | Secret `openshell-gateway-vault-tls` (key `ca.crt`) in namespace `openshell-vault` | `GATEWAY_VAULT_CA_SECRET` env |

The role name and auth mount are shared, literal constants used identically by every
gateway on the cluster and by `hypershell-gitops`; a mismatch there breaks
authentication outright, so they SHALL match the GitOps Contract values.

#### Scenario: Convention address used when none is supplied

- GIVEN a Vault-backed Gateway whose `credential_driver.vault.address` is unset
- AND no `GATEWAY_VAULT_ADDRESS` override is configured
- WHEN the GatewayReconciler generates `gateway.toml`
- THEN the `address` SHALL be `https://openshell-gateway-vault.openshell-vault.svc:8200`
- AND the `role` SHALL be `openshell-gateway`, `mount` `secret`, `auth_method` `kubernetes`

#### Scenario: Deployment override redirects the Vault address

- GIVEN `GATEWAY_VAULT_ADDRESS` is set on the controller to a dev Vault URL
- WHEN the GatewayReconciler generates `gateway.toml` for a Vault-backed gateway
- THEN the `address` SHALL be the override value
- AND all other unset coordinates SHALL still take their convention defaults

---

### Requirement: Credential Driver TOML Generation

The GatewayReconciler SHALL generate the `gateway.toml` credential configuration based
on the resolved driver. The `[openshell.gateway.credential_storage]` section (KEK) and
the `[openshell.credential_drivers.*]` sections are mutually exclusive.

**Vault (default, or explicit `vault`):**

```toml
credential_drivers = ["vault"]

[openshell.credential_drivers.vault]
address = "https://openshell-gateway-vault.openshell-vault.svc:8200"
ca_bundle = "/etc/openshell-tls/vault-ca/ca.crt"
mount = "secret"
kv_version = "2"
auth_method = "kubernetes"
role = "openshell-gateway"
kubernetes_auth_mount = "kubernetes"
service_account_token_path = "/var/run/secrets/vault/token"
timeout_secs = 30
```

**Kubernetes Secrets:**

```toml
credential_drivers = ["kubernetes-secrets"]

[openshell.credential_drivers.kubernetes-secrets]
namespace = "<configured-or-tenant-namespace>"
```

**KEK database (`kek-db`):**

```toml
[openshell.gateway.credential_storage]
key_encryption_key_env = "OPENSHELL_GATEWAY_CREDENTIAL_KEY_ENCRYPTION_KEY"
```

The generated Vault section SHALL contain only non-secret coordinates (address, mount,
role, paths) and SHALL NEVER contain a provider secret value or the ServiceAccount
token.

#### Scenario: Vault TOML carries coordinates only

- GIVEN a Vault-backed Gateway
- WHEN the GatewayReconciler renders `gateway.toml`
- THEN the `[openshell.credential_drivers.vault]` section SHALL carry the address, mount,
  role, auth mount, CA path and SA token path
- AND it SHALL contain no secret value and no token

---

### Requirement: Vault Serving-CA Propagation

When the resolved driver is `vault` and the address is not loopback, the gateway reaches
Vault over HTTPS and SHALL verify the Vault serving certificate against the per-cluster
Vault's CA. The GatewayReconciler SHALL make that CA available to the gateway pod by
mirroring it into the tenant namespace, mirroring the existing trusted-CA-bundle
propagation (`reconcileTrustedCABundle`).

For each Vault-backed gateway, the control plane SHALL:

1. Read the serving-CA source Secret (`openshell-gateway-vault-tls`, key `ca.crt`, in
   namespace `openshell-vault`; overridable via `GATEWAY_VAULT_CA_SECRET`) on the same
   cluster.
2. Create-or-update a ConfigMap `openshell-gateway-vault-ca` in the tenant namespace
   with key `ca.crt`, carrying the management label `hypershell.redhat.io/managed: "true"`.
3. Wire the upstream Helm chart's `credentialDrivers.vault.caConfigMapName` to that
   ConfigMap. The chart mounts that ConfigMap's `ca.crt` at
   `/etc/openshell-tls/vault-ca/ca.crt` and the generated TOML sets
   `ca_bundle = "/etc/openshell-tls/vault-ca/ca.crt"`.

Because the control plane provisions each gateway on its own cluster (it holds no remote
cluster client), the CA source and the gateway always live on the same cluster; no
cross-cluster read is required. When the source Secret is absent, the gateway SHALL NOT
be deployed and the failure SHALL surface as a fail-closed reachability error (see
Requirement: Fail Closed When Vault Is Unavailable), because a missing CA means the
gateway cannot establish trust to Vault.

#### Scenario: Vault CA mirrored into the tenant namespace

- GIVEN a Vault-backed Gateway and a serving-CA Secret `openshell-gateway-vault-tls` in `openshell-vault`
- WHEN the GatewayReconciler reconciles
- THEN a ConfigMap `openshell-gateway-vault-ca` with key `ca.crt` SHALL exist in the tenant namespace
- AND the gateway Deployment SHALL mount it at `/etc/openshell-tls/vault-ca/ca.crt` (the TOML `ca_bundle`)
- AND the ConfigMap SHALL carry `hypershell.redhat.io/managed: "true"`

#### Scenario: Vault CA source missing

- GIVEN a Vault-backed Gateway whose serving-CA source Secret is absent on the cluster
- WHEN the GatewayReconciler reconciles
- THEN it SHALL NOT deploy the gateway
- AND it SHALL report a fail-closed credential-backend error naming the missing CA source

#### Scenario: Driver is not Vault

- GIVEN a Gateway whose resolved driver is `kek-db` or `kubernetes-secrets`
- WHEN the GatewayReconciler reconciles
- THEN it SHALL NOT create the `openshell-gateway-vault-ca` ConfigMap

---

### Requirement: Vault Driver Deployment Configuration

When the resolved driver is `vault` with `auth_method` = `kubernetes`, the
GatewayReconciler SHALL mount a projected ServiceAccount token volume in the gateway
Deployment for Vault authentication, scoped to the `vault` audience.

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
The dedicated `audience: vault` token is a deliberate override of the chart's generic
default token path: a token minted for the `vault` audience is cryptographically useless
against any other audience even if it leaks. When the resolved driver is not `vault`,
this volume and mount SHALL NOT be present.

#### Scenario: Vault SA token mounted with the vault audience

- GIVEN a Vault-backed Gateway with `auth_method` = `kubernetes`
- WHEN the GatewayReconciler reconciles
- THEN the gateway Deployment SHALL include a projected ServiceAccount token volume with audience `vault`
- AND the volume SHALL be mounted at `/var/run/secrets/vault`

---

### Requirement: Fail Closed When Vault Is Unavailable

A Vault-backed gateway SHALL fail closed: there is no fallback to another backend,
because a gateway resolves to exactly one driver at creation. The control plane SHALL
make Vault unavailability observable rather than letting a gateway come up
non-functional.

- **At provisioning (two stages):** a Vault-backed gateway SHALL be verified in two
  stages, because the gateway's ServiceAccount does not exist until its workload is
  deployed.
  - **Reachability (pre-deploy):** during `EnvironmentReady`, before the workload is
    deployed, the control plane SHALL verify the per-cluster Vault is reachable and
    serving TLS trusted by the serving CA (an unauthenticated health check). On failure
    it SHALL set `EnvironmentReady` to `Failed` with a named reason (for example
    `vault unreachable at <address>`) and return the error so the reconcile retries.
  - **Authentication (before `Running`):** after the workload is deployed (so the
    gateway ServiceAccount exists) and before the gateway is marked `Running`, the
    control plane SHALL verify the gateway's identity can authenticate - acting as the
    gateway's own ServiceAccount identity against role `openshell-gateway`, never as a
    Vault administrator. On failure it SHALL report a `GatewayDeployed`-stage failure
    with a named reason (such as `vault auth denied for role openshell-gateway`), SHALL
    NOT mark the gateway `Running`, and SHALL return the error so the reconcile retries.
- **At runtime:** a later Vault outage SHALL move the gateway to `Degraded` through the
  existing health reconciler (see [`openshell-gateway-health.spec.md`](./openshell-gateway-health.spec.md)),
  recoverable without recreation once Vault returns.
- **Observability:** the control plane SHALL expose a named, dashboard-visible signal for
  whether a gateway's Vault backend is reachable, so an operator can distinguish
  Vault-caused failures from other causes.
- A `provider`/`sandbox` call made against an unreachable Vault surfaces as OpenShell's
  own gRPC error to the `openshell` CLI; the control plane SHALL NOT swallow it.

#### Scenario: Vault unreachable at provisioning (pre-deploy)

- GIVEN a new Vault-backed Gateway on a cluster whose provider Vault is unreachable
- WHEN the GatewayReconciler reconciles
- THEN it SHALL set `EnvironmentReady` to `Failed` with a reason naming Vault
- AND it SHALL NOT deploy the gateway workload or mark the gateway `Running`
- AND the reconcile SHALL be retried

#### Scenario: Gateway identity cannot authenticate to Vault (before Running)

- GIVEN a Vault-backed Gateway whose workload has deployed but whose identity is not
  authorized by the Vault role (or Vault denies the login)
- WHEN the GatewayReconciler verifies authentication before marking the gateway `Running`
- THEN it SHALL report a `GatewayDeployed`-stage failure with a reason naming Vault
- AND it SHALL NOT mark the gateway `Running`
- AND the reconcile SHALL be retried

#### Scenario: Vault outage after the gateway is running

- GIVEN a `Running` Vault-backed Gateway
- WHEN its provider Vault becomes unreachable
- THEN the control plane SHALL set the gateway `phase` to `Degraded` with a reason naming Vault
- AND the gateway SHALL return to `Running` once Vault is reachable again, without recreation

---

### Requirement: RBAC for Kubernetes Secrets Driver

When `credential_driver.type` is `kubernetes-secrets`, the GatewayReconciler SHALL
provision a Role and RoleBinding granting the gateway ServiceAccount access to Secrets in
the credential namespace.

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

These resources SHALL carry the label `hypershell.redhat.io/managed: "true"` and SHALL be
deleted when the credential driver is changed or the gateway is deleted. When the
credential namespace differs from the tenant namespace, the Role and RoleBinding SHALL be
created in the credential namespace.

The Vault driver requires no such Role or RoleBinding: it authenticates to Vault with the
projected ServiceAccount token, not the Kubernetes API.

#### Scenario: RBAC provisioned for Kubernetes Secrets

- GIVEN a Gateway with `credential_driver.type` = `kubernetes-secrets`
- WHEN the GatewayReconciler reconciles
- THEN a Role `openshell-gateway-credential-secrets` SHALL exist in the credential namespace
- AND a RoleBinding `openshell-gateway-credential-secrets` SHALL bind it to the `openshell-gateway` ServiceAccount

#### Scenario: Vault and default drivers create no credential RBAC

- GIVEN a Gateway resolved to `vault` or `kek-db`
- WHEN the GatewayReconciler reconciles
- THEN no credential secrets Role or RoleBinding SHALL exist

---

### Requirement: KEK Provisioning Conditional on Driver

The GatewayReconciler SHALL provision the KEK Secret (`openshell-gateway-credential-kek`)
and set the `OPENSHELL_GATEWAY_CREDENTIAL_KEY_ENCRYPTION_KEY` env var **only** when the
resolved driver is `kek-db`.

- Resolved driver `vault` (including the omitted-field default): no KEK Secret, no KEK env var.
- Resolved driver `kubernetes-secrets`: no KEK Secret, no KEK env var.
- Resolved driver `kek-db`: provision the KEK Secret and set the env var (former default behavior).

#### Scenario: KEK skipped for the default Vault driver

- GIVEN a Gateway with no `credential_driver` field (resolves to `vault`)
- WHEN the GatewayReconciler reconciles
- THEN the Secret `openshell-gateway-credential-kek` SHALL NOT be created
- AND the env var `OPENSHELL_GATEWAY_CREDENTIAL_KEY_ENCRYPTION_KEY` SHALL NOT be in the Deployment

#### Scenario: KEK provisioned for explicit kek-db

- GIVEN a Gateway with `credential_driver.type` = `kek-db`
- WHEN the GatewayReconciler reconciles
- THEN the Secret `openshell-gateway-credential-kek` SHALL be created
- AND the env var `OPENSHELL_GATEWAY_CREDENTIAL_KEY_ENCRYPTION_KEY` SHALL be present

---

### Requirement: Provider Credential Revocation on Deletion

When a Vault-backed Gateway is deleted, the control plane SHALL remove every provider
secret stored under that gateway's Vault KV path
(`secret/data/openshell/<tenant-namespace>/*` and the corresponding metadata), as part of
the teardown reconcile. The control plane SHALL act as the gateway's own identity (the
templated policy scopes that identity to its own namespace path), never as a Vault
administrator, and SHALL perform the purge before the gateway ServiceAccount and
namespace are removed, so the identity it authenticates with still exists.

The purge enumerates the namespace path (a Vault `LIST` on
`secret/metadata/openshell/<tenant-namespace>`) and deletes each secret's metadata, so
the templated policy MUST grant the gateway identity `list` on its own namespace path
(see the GitOps Contract), not only on the child entries.

This is an out-of-namespace owned-resource class: the namespace cascade does not reach
Vault. The purge is **best-effort**: it runs once, before the gateway ServiceAccount is
removed by teardown, and if it cannot complete (Vault unreachable, or the identity
already gone) the control plane SHALL record a durable, operator-visible
`VaultCredentialPath` orphan signal naming the tenant namespace and reason (never a
secret value) and SHALL NOT block the rest of teardown - because once the ServiceAccount
is gone the purge is not inline-recoverable, so the orphan signal is the operator
recovery path, consistent with the no-silent-orphan contract in
[`gateway-deletion-finalization.spec.md`](./gateway-deletion-finalization.spec.md).

There is no audit-retention exception: if an audit trail is required, it comes from
Vault's own audit log (which records that an access happened, never the value), not from
leaving live secret material for a deleted tenant.

#### Scenario: KV path purged on deletion

- GIVEN a Vault-backed Gateway with provider secrets under `secret/data/openshell/<ns>/*`
- WHEN the Gateway is deleted
- THEN the control plane SHALL delete all secrets and metadata under that path before removing the namespace
- AND it SHALL authenticate as the gateway's own identity, not as a Vault administrator

#### Scenario: Purge cannot complete inline

- GIVEN a Vault-backed Gateway delete whose Vault is unreachable
- WHEN the control plane attempts the KV purge
- THEN it SHALL return a retryable error so the delete is retried
- AND it SHALL record a durable `VaultCredentialPath` orphan signal naming the tenant namespace and reason
- AND the signal SHALL contain no secret value

---

### Requirement: Credential Secret Hygiene

Provider secret values and the Vault ServiceAccount token SHALL never leak into the API,
logs, telemetry, Kubernetes Events, generated manifests, or `gateway.toml`. This applies
the project secret-handling standard
([`security.spec.md`](../standards/security/security.spec.md)); it introduces no new
standard.

- The control plane never holds a provider secret value: only the gateway process talks
  to Vault's KV endpoints, and the API returns provider metadata and key names only,
  never values.
- `gateway.toml`, as generated for the Vault driver, SHALL carry only non-secret
  coordinates - never a secret value or a token.
- The SA token SHALL never appear in a log line, including on the fail-closed
  reachability-check failure path, which SHALL log that the login failed, never the token.

#### Scenario: Generated config contains no secret material

- GIVEN a Vault-backed Gateway
- WHEN the control plane renders and applies `gateway.toml`
- THEN the rendered config SHALL contain no provider secret value and no ServiceAccount token

#### Scenario: Reachability failure does not log the token

- GIVEN a Vault-backed Gateway whose reachability/auth probe fails
- WHEN the control plane logs the failure and sets the condition
- THEN the log line and the condition message SHALL name the failure (for example "vault auth denied")
- AND SHALL NOT contain the ServiceAccount token or any provider secret

---

### Requirement: Credential Driver Immutability

The `credential_driver` configuration on a Gateway SHALL be immutable after the gateway
has stored provider credentials. Changing the driver would orphan credentials in the
previous backend.

#### Scenario: Attempt to change credential driver

- GIVEN a Gateway with `credential_driver.type` = `vault` that has active provider credentials
- WHEN a user attempts to change `credential_driver.type` to `kubernetes-secrets`
- THEN the API server SHALL reject the update with error: `credential driver cannot be changed after provider credentials have been stored; migrate credentials manually first`

> **Phase 2 (reserved):** Automated credential migration between drivers.

---

### Requirement: End-to-End Vault Coverage in CI

The Vault credential driver SHALL be exercised end-to-end by the Kind e2e suite, so the
whole path is proven in CI and so `vault` can become the flat default (see Requirement:
Credential Driver Configuration Field) without reddening e2e. The suite SHALL stand up a
Vault-compatible server in the Kind environment (an OSS Vault such as OpenBao), configured
to the GitOps Contract below (KV v2 at `secret`, Kubernetes auth, the `openshell-gateway`
role and the namespace-templated policy, a serving CA), so a Vault-backed gateway can
authenticate and store secrets exactly as on a real cluster. See
[`e2e-testing.spec.md`](./e2e-testing.spec.md).

#### Scenario: Vault-backed gateway stores and reads a provider secret

- GIVEN the Kind e2e environment with a Vault stood up to the GitOps Contract
- AND a gateway provisioned with `credential_driver.type` = `vault`
- WHEN the suite stores a provider credential through the gateway and reads it back
- THEN the value SHALL round-trip, and it SHALL be stored under
  `secret/data/openshell/<tenant-namespace>/*` in Vault

  The suite MAY drive this as the gateway's own Vault identity (a `vault`-audience
  ServiceAccount token under the `openshell-gateway` role and templated policy) rather than
  through the product's provider-credential API, since that is the exact mechanism the
  gateway uses to reach its namespace path; the assertion is the round-trip under the
  tenant path, and that listing the tenant path (the purge enumeration) succeeds.

#### Scenario: Namespace isolation holds between two gateways

- GIVEN two Vault-backed gateways in different tenant namespaces
- WHEN each authenticates to Vault as its own identity
- THEN each SHALL reach only its own `secret/data/openshell/<its-namespace>/*` path
- AND SHALL be denied any other gateway's path

#### Scenario: Fail-closed is observable when Vault is unavailable

- GIVEN a Vault-backed gateway on an environment whose Vault is unreachable
- WHEN the gateway is provisioned
- THEN the suite SHALL observe the fail-closed signal (provisioning fails with a
  Vault-named reason / the gateway reports `Degraded`), never a gateway that reports
  healthy while unable to reach Vault

---

## GitOps Contract

The per-cluster Vault is stood up by `hypershell-gitops`
(`specs/openshell-gateway-vault.spec.md`). These values are shared literally across both
repos; a mismatch breaks authentication. This section is the authoritative reference the
GitOps repo implements against.

| Item | Value |
|---|---|
| In-cluster address | `https://openshell-gateway-vault.openshell-vault.svc:8200` |
| KV engine / mount | KV v2, mount `secret` |
| KV path layout | `secret/data/openshell/<tenant-namespace>/<provider-handle>` |
| Kubernetes auth mount | `kubernetes` |
| Shared Vault role | `openshell-gateway` |
| Isolation | one namespace-templated ACL policy resolves each caller's verified `service_account_namespace` at login |
| Policy capabilities | the templated policy grants the gateway identity CRUD+list on `secret/data/openshell/<ns>/*`, read/list/delete on `secret/metadata/openshell/<ns>/*`, **and `list` on the namespace path `secret/metadata/openshell/<ns>`** so the control plane can enumerate the path for purge-on-delete |
| Gateway SA token | projected, `audience: vault`, `expirationSeconds: 3600`, mounted at `/var/run/secrets/vault/token` |
| Serving-CA source | Secret `openshell-gateway-vault-tls` (key `ca.crt`) in namespace `openshell-vault` |
| Serving-CA in tenant ns | ConfigMap `openshell-gateway-vault-ca` (key `ca.crt`), wired to the chart's `credentialDrivers.vault.caConfigMapName` |

Who owns what: `hypershell-gitops` pre-provisions the Vault, the KV engine, the
Kubernetes auth mount, the shared role and the templated policy, once, up front. The
control plane creates no Vault object at runtime and holds no Vault admin credentials; it
only authenticates as a gateway identity for the authenticated probe and the deletion
purge. To do so it mints a `vault`-audience token for the gateway's ServiceAccount via the
TokenRequest API, so the controller's RBAC MUST allow `create` on the
`serviceaccounts/token` subresource in gateway namespaces (a distinct subresource from
`serviceaccounts`).

---

## Configuration Reference

| Setting | Where | Default | Meaning |
|---|---|---|---|
| `GATEWAY_VAULT_ADDRESS` | controller env | the convention svc URL | Override the per-cluster Vault address (dev / external Vault) |
| `GATEWAY_VAULT_ROLE` | controller env | `openshell-gateway` | Override the shared Vault role (must match GitOps) |
| `GATEWAY_VAULT_CA_SECRET` | controller env | `openshell-vault/openshell-gateway-vault-tls` | Override the serving-CA source Secret (namespace/name) |

The API server reads no Vault configuration for gateways; the `credential_driver` field
is the only API-level input, and for the default Vault case it need not be set at all.

---

## Configuration Examples

### Gateway with default (Vault) credential storage - no change from a bare create

```yaml
kind: Gateway
name: openshell-gateway
project: tenant-a
image: quay.io/opendatahub/odh-openshell-gateway:v0.1.3-rhaiv.3@sha256:e59fea2fc97144ed2f402af69be73c6dbbf9a76739f042e86c2237f65852284e
serverDnsNames:
  - openshell-gateway.tenant-a.svc.cluster.local
# credential_driver omitted -> Vault, convention-addressed
```

### Gateway pinned explicitly to the KEK database store (former default)

```yaml
kind: Gateway
name: openshell-gateway
project: tenant-a
credential_driver:
  type: kek-db
```

### Gateway with Kubernetes Secrets credential storage

```yaml
kind: Gateway
name: openshell-gateway
project: tenant-a
credential_driver:
  type: kubernetes-secrets
  kubernetes_secrets:
    namespace: tenant-a
```

### Gateway pointed at an external / non-convention Vault

```yaml
kind: Gateway
name: openshell-gateway
project: tenant-a
credential_driver:
  type: vault
  vault:
    address: https://vault.example.com:8200
    role: openshell-gateway-tenant-a
```

---

## References

- [OpenShell PR #2437 - Provider credential storage drivers](https://github.com/NVIDIA/OpenShell/pull/2437)
- `hypershell-gitops` `specs/openshell-gateway-vault.spec.md` - the per-cluster Vault (HYPERSHELL-369)
- [`openshell-gateway-database.spec.md`](./openshell-gateway-database.spec.md) - KEK/default-driver history, per-gateway DB
- [`openshell-gateway-tls.spec.md`](./openshell-gateway-tls.spec.md) - trusted-CA-bundle propagation pattern
- [`gateway-provisioning-progress.spec.md`](./gateway-provisioning-progress.spec.md) - `EnvironmentReady` condition
- [`openshell-gateway-health.spec.md`](./openshell-gateway-health.spec.md) - `Degraded` runtime phase
- [`gateway-deletion-finalization.spec.md`](./gateway-deletion-finalization.spec.md) - owned-resource cleanup, no-silent-orphan
- [`security.spec.md`](../standards/security/security.spec.md) - secret handling, no secrets in responses or logs
