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
Vault is **convention-addressed**: its address, auth mount, shared role and serving CA are
fixed coordinates the control plane already knows, and each gateway's KV mount is derived
from its tenant namespace (`openshell-gw/<namespace>`), so a gateway becomes Vault-backed
with **no user input**. The former default - a key-encryption-key (KEK) encrypted database
store - remains available as an explicit opt-in (`kek-db`), as does the Kubernetes Secrets
driver (`kubernetes-secrets`).

The control plane never holds a provider secret value and holds **no Vault admin**: it
cannot write Vault policies, roles, auth config, or any mount outside the per-tenant
`openshell-gw/*` prefix. Only the gateway's own process reads and writes KV secrets,
authenticating to Vault with its ServiceAccount. The control plane acts **as the gateway's
own identity** for the fail-closed reachability probe, and holds a **narrowly scoped**
provisioner identity whose sole power is to create and delete a gateway's own per-tenant KV
mount - never to read a secret or rewrite the isolation boundary (see the Provider
Credential Isolation requirement).

**Isolation is layered, and each layer is enforced where it can be.** Each gateway gets its
**own** KV v2 mount at `openshell-gw/<namespace>`. The OpenShell gateway runtime stores every
provider credential at the flat, hash-addressed suffix
`data/openshell/provider-credentials/<hash>` under that mount, where `<hash>` is a SHA-256
seeded by a random per-provider UUID (so it is unguessable), and it reaches a secret only by
the opaque handle it already holds in its own database. A single **templated** policy scopes
each gateway's token to its own namespace's mount, granting
`create/read/update/delete` on that mount's `provider-credentials/` prefix with **no `list`**:
a token for namespace A can resolve only `openshell-gw/A/...`, so a handle leaked from
namespace B is unusable, and no gateway can enumerate or guess another's path. That per-tenant
mount, on top of the per-cluster Vault boundary (each cluster runs its own Vault), is what
isolates one gateway from another and one cluster from another. **Sandbox-to-sandbox**
isolation within a single gateway is a
different layer and is **not** a Vault-ACL concern: sandboxes never authenticate to Vault
(only the gateway process does, as the single Vault client), so that boundary is enforced
by the OpenShell gateway's own workspace authorization, which binds each credential's path
to its owning workspace (the workspace is an input to the path hash) and rejects any
resolve whose requesting workspace does not match the handle's path. See the Provider
Credential Isolation requirement.

Credential rotation (KEK re-encryption, DB password rotation) is out of scope and is
addressed in [`openshell-gateway-secret-rotation.spec.md`](./openshell-gateway-secret-rotation.spec.md).
Rotating a provider secret's **value** is OpenShell's own `provider` mechanism, not a
HyperShell concern, for every driver.

---

## Requirements

### Requirement: OSG-CRED-01 -- Credential Driver Configuration Field

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
| `credential_driver.vault.mount` | string | No | KV secrets engine mount. Default: `openshell-gw/<tenant-namespace>` (derived per gateway) |
| `credential_driver.vault.auth_method` | string | No | Authentication method. Default: `kubernetes` |
| `credential_driver.vault.role` | string | No | Vault role. Default: `openshell-gateway` |
| `credential_driver.vault.kubernetes_auth_mount` | string | No | Vault Kubernetes auth mount. Default: `kubernetes` |
| `credential_driver.vault.timeout_secs` | int | No | Vault client timeout in seconds. Default: `30` |

#### Scenario: OSG-CRED-01.01 -- Gateway created with no credential_driver defaults to Vault

- GIVEN a Gateway resource with no `credential_driver` field
- WHEN the GatewayReconciler reconciles
- THEN the gateway SHALL be configured for the `vault` driver using the convention defaults
- AND the `gateway.toml` SHALL include `credential_drivers = ["vault"]`
- AND the KEK Secret SHALL NOT be provisioned
- AND the `[openshell.gateway.credential_storage]` section SHALL be omitted

#### Scenario: OSG-CRED-01.02 -- Gateway with explicit Vault driver

- GIVEN a Gateway resource with `credential_driver.type` = `vault`
- WHEN the GatewayReconciler reconciles
- THEN the gateway SHALL be configured identically to the omitted-field case, honoring
  any supplied overrides in `credential_driver.vault`

#### Scenario: OSG-CRED-01.03 -- Gateway with explicit KEK database driver

- GIVEN a Gateway resource with `credential_driver.type` = `kek-db`
- WHEN the GatewayReconciler reconciles
- THEN the `gateway.toml` SHALL include `[openshell.gateway.credential_storage]` with `key_encryption_key_env`
- AND the KEK Secret SHALL be provisioned
- AND no `credential_drivers` array SHALL be emitted

#### Scenario: OSG-CRED-01.04 -- Gateway with Kubernetes Secrets driver

- GIVEN a Gateway resource with `credential_driver.type` = `kubernetes-secrets`
- WHEN the GatewayReconciler reconciles
- THEN the `gateway.toml` SHALL include `credential_drivers = ["kubernetes-secrets"]`
- AND the `[openshell.gateway.credential_storage]` section SHALL be omitted
- AND the KEK Secret SHALL NOT be provisioned

---

### Requirement: OSG-CRED-02 -- Credential Driver Validation

The GatewayReconciler SHALL validate the `credential_driver` configuration before
reconciliation.

- `credential_driver.type`, when present, SHALL be one of: `vault`, `kubernetes-secrets`, `kek-db`.
- A `vault` driver SHALL NOT require any sub-field; supplied sub-fields SHALL be
  validated (`address`, when present, SHALL be a well-formed URL; a non-loopback
  `address` SHALL use `https`).
- Unknown fields SHALL be rejected.

#### Scenario: OSG-CRED-02.01 -- Invalid credential driver type

- GIVEN a Gateway resource with `credential_driver.type` = `s3`
- WHEN the GatewayReconciler validates the configuration
- THEN it SHALL reject the configuration with error: `unsupported credential driver type "s3"; supported: vault, kubernetes-secrets, kek-db`

#### Scenario: OSG-CRED-02.02 -- Vault driver with no sub-fields is valid

- GIVEN a Gateway resource with `credential_driver.type` = `vault` and no `vault` sub-fields
- WHEN the GatewayReconciler validates the configuration
- THEN it SHALL accept the configuration
- AND it SHALL apply the convention defaults for address, mount, role, and auth

#### Scenario: OSG-CRED-02.03 -- Vault override with a plaintext address is rejected

- GIVEN a Gateway with `credential_driver.vault.address` = `http://vault.example.com:8200`
- WHEN the GatewayReconciler validates the configuration
- THEN it SHALL reject the configuration because a non-loopback Vault address MUST use `https`

---

### Requirement: OSG-CRED-03 -- Vault Convention Defaults

When the resolved driver is `vault`, the GatewayReconciler SHALL fill every unset Vault
coordinate from fixed conventions, so a gateway is Vault-backed without user input. The
conventions match the per-cluster Vault stood up by `hypershell-gitops` (see the GitOps
Contract section). The address and serving-CA source MAY be overridden by control-plane
deployment configuration for non-standard topologies; any field MAY be overridden
per-gateway via `credential_driver.vault.*`.

| Coordinate | Convention default | Override |
|---|---|---|
| address | `https://openshell-gateway-vault.openshell-vault.svc:8200` | `GATEWAY_VAULT_ADDRESS` env; `vault.address` |
| mount (KV v2) | `openshell-gw/<tenant-namespace>` (derived from the gateway namespace) | `vault.mount` |
| KV version | `2` | - |
| auth method | `kubernetes` | `vault.auth_method` |
| Vault role | `openshell-gateway` | `GATEWAY_VAULT_ROLE` env; `vault.role` |
| Kubernetes auth mount | `kubernetes` | `vault.kubernetes_auth_mount` |
| SA token path | `/var/run/secrets/vault/token` | - |
| SA token audience | `vault` | - |
| serving-CA source | Secret `openshell-gateway-vault-tls` (key `ca.crt`) in namespace `openshell-vault` | `GATEWAY_VAULT_CA_SECRET` env |

The role name and auth mount are shared, literal constants used identically by every
gateway on the cluster and by `hypershell-gitops`; a mismatch there breaks
authentication outright, so they SHALL match the GitOps Contract values. The **mount** is
**not** shared: it is derived per gateway as `openshell-gw/<tenant-namespace>`, and the
control plane creates it (and deletes it on teardown) through its scoped provisioner
identity (see Provider Credential Isolation).

#### Scenario: OSG-CRED-03.01 -- Convention address used when none is supplied

- GIVEN a Vault-backed Gateway whose `credential_driver.vault.address` is unset
- AND no `GATEWAY_VAULT_ADDRESS` override is configured
- WHEN the GatewayReconciler generates `gateway.toml`
- THEN the `address` SHALL be `https://openshell-gateway-vault.openshell-vault.svc:8200`
- AND the `role` SHALL be `openshell-gateway`, `mount` `openshell-gw/<tenant-namespace>`, `auth_method` `kubernetes`

#### Scenario: OSG-CRED-03.02 -- Deployment override redirects the Vault address

- GIVEN `GATEWAY_VAULT_ADDRESS` is set on the controller to a dev Vault URL
- WHEN the GatewayReconciler generates `gateway.toml` for a Vault-backed gateway
- THEN the `address` SHALL be the override value
- AND all other unset coordinates SHALL still take their convention defaults

---

### Requirement: OSG-CRED-04 -- Credential Driver TOML Generation

The GatewayReconciler SHALL generate the `gateway.toml` credential configuration based
on the resolved driver. The `[openshell.gateway.credential_storage]` section (KEK) and
the `[openshell.credential_drivers.*]` sections are mutually exclusive.

**Vault (default, or explicit `vault`):**

```toml
credential_drivers = ["vault"]

[openshell.credential_drivers.vault]
address = "https://openshell-gateway-vault.openshell-vault.svc:8200"
ca_bundle = "/etc/openshell-tls/vault-ca/ca.crt"
mount = "openshell-gw/<tenant-namespace>"
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

#### Scenario: OSG-CRED-04.01 -- Vault TOML carries coordinates only

- GIVEN a Vault-backed Gateway
- WHEN the GatewayReconciler renders `gateway.toml`
- THEN the `[openshell.credential_drivers.vault]` section SHALL carry the address, mount,
  role, auth mount, CA path and SA token path
- AND it SHALL contain no secret value and no token

---

### Requirement: OSG-CRED-05 -- Vault Serving-CA Propagation

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

#### Scenario: OSG-CRED-05.01 -- Vault CA mirrored into the tenant namespace

- GIVEN a Vault-backed Gateway and a serving-CA Secret `openshell-gateway-vault-tls` in `openshell-vault`
- WHEN the GatewayReconciler reconciles
- THEN a ConfigMap `openshell-gateway-vault-ca` with key `ca.crt` SHALL exist in the tenant namespace
- AND the gateway Deployment SHALL mount it at `/etc/openshell-tls/vault-ca/ca.crt` (the TOML `ca_bundle`)
- AND the ConfigMap SHALL carry `hypershell.redhat.io/managed: "true"`

#### Scenario: OSG-CRED-05.02 -- Vault CA source missing

- GIVEN a Vault-backed Gateway whose serving-CA source Secret is absent on the cluster
- WHEN the GatewayReconciler reconciles
- THEN it SHALL NOT deploy the gateway
- AND it SHALL report a fail-closed credential-backend error naming the missing CA source

#### Scenario: OSG-CRED-05.03 -- Driver is not Vault

- GIVEN a Gateway whose resolved driver is `kek-db` or `kubernetes-secrets`
- WHEN the GatewayReconciler reconciles
- THEN it SHALL NOT create the `openshell-gateway-vault-ca` ConfigMap

---

### Requirement: OSG-CRED-06 -- Vault Driver Deployment Configuration

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

#### Scenario: OSG-CRED-06.01 -- Vault SA token mounted with the vault audience

- GIVEN a Vault-backed Gateway with `auth_method` = `kubernetes`
- WHEN the GatewayReconciler reconciles
- THEN the gateway Deployment SHALL include a projected ServiceAccount token volume with audience `vault`
- AND the volume SHALL be mounted at `/var/run/secrets/vault`

---

### Requirement: OSG-CRED-07 -- Fail Closed When Vault Is Unavailable

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

#### Scenario: OSG-CRED-07.01 -- Vault unreachable at provisioning (pre-deploy)

- GIVEN a new Vault-backed Gateway on a cluster whose provider Vault is unreachable
- WHEN the GatewayReconciler reconciles
- THEN it SHALL set `EnvironmentReady` to `Failed` with a reason naming Vault
- AND it SHALL NOT deploy the gateway workload or mark the gateway `Running`
- AND the reconcile SHALL be retried

#### Scenario: OSG-CRED-07.02 -- Gateway identity cannot authenticate to Vault (before Running)

- GIVEN a Vault-backed Gateway whose workload has deployed but whose identity is not
  authorized by the Vault role (or Vault denies the login)
- WHEN the GatewayReconciler verifies authentication before marking the gateway `Running`
- THEN it SHALL report a `GatewayDeployed`-stage failure with a reason naming Vault
- AND it SHALL NOT mark the gateway `Running`
- AND the reconcile SHALL be retried

#### Scenario: OSG-CRED-07.03 -- Vault outage after the gateway is running

- GIVEN a `Running` Vault-backed Gateway
- WHEN its provider Vault becomes unreachable
- THEN the control plane SHALL set the gateway `phase` to `Degraded` with a reason naming Vault
- AND the gateway SHALL return to `Running` once Vault is reachable again, without recreation

---

### Requirement: OSG-CRED-08 -- RBAC for Kubernetes Secrets Driver

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

#### Scenario: OSG-CRED-08.01 -- RBAC provisioned for Kubernetes Secrets

- GIVEN a Gateway with `credential_driver.type` = `kubernetes-secrets`
- WHEN the GatewayReconciler reconciles
- THEN a Role `openshell-gateway-credential-secrets` SHALL exist in the credential namespace
- AND a RoleBinding `openshell-gateway-credential-secrets` SHALL bind it to the `openshell-gateway` ServiceAccount

#### Scenario: OSG-CRED-08.02 -- Vault and default drivers create no credential RBAC

- GIVEN a Gateway resolved to `vault` or `kek-db`
- WHEN the GatewayReconciler reconciles
- THEN no credential secrets Role or RoleBinding SHALL exist

---

### Requirement: OSG-CRED-09 -- KEK Provisioning Conditional on Driver

The GatewayReconciler SHALL provision the KEK Secret (`openshell-gateway-credential-kek`)
and set the `OPENSHELL_GATEWAY_CREDENTIAL_KEY_ENCRYPTION_KEY` env var **only** when the
resolved driver is `kek-db`.

- Resolved driver `vault` (including the omitted-field default): no KEK Secret, no KEK env var.
- Resolved driver `kubernetes-secrets`: no KEK Secret, no KEK env var.
- Resolved driver `kek-db`: provision the KEK Secret and set the env var (former default behavior).

#### Scenario: OSG-CRED-09.01 -- KEK skipped for the default Vault driver

- GIVEN a Gateway with no `credential_driver` field (resolves to `vault`)
- WHEN the GatewayReconciler reconciles
- THEN the Secret `openshell-gateway-credential-kek` SHALL NOT be created
- AND the env var `OPENSHELL_GATEWAY_CREDENTIAL_KEY_ENCRYPTION_KEY` SHALL NOT be in the Deployment

#### Scenario: OSG-CRED-09.02 -- KEK provisioned for explicit kek-db

- GIVEN a Gateway with `credential_driver.type` = `kek-db`
- WHEN the GatewayReconciler reconciles
- THEN the Secret `openshell-gateway-credential-kek` SHALL be created
- AND the env var `OPENSHELL_GATEWAY_CREDENTIAL_KEY_ENCRYPTION_KEY` SHALL be present

---

### Requirement: OSG-CRED-10 -- Provider Credential Isolation

Each gateway gets its **own** KV v2 mount at `openshell-gw/<tenant-namespace>`. The OpenShell
gateway runtime stores every provider credential at the flat, hash-addressed suffix
`data/openshell/provider-credentials/<hash>` under that mount (with its KV v2 metadata under
`metadata/openshell/provider-credentials/<hash>`), where `<hash>` is a SHA-256 over the owning
workspace, a random per-provider UUID, and the credential coordinates. The runtime fixes the
suffix; the **mount** is the configurable prefix, which the control plane derives from the
gateway namespace and sets through `credential_driver.vault.mount`.

The gateway policy `openshell-gateway-provider` is **templated**: Vault substitutes the
logging-in token's own ServiceAccount namespace into the path, so each gateway is scoped to
its own mount. It SHALL grant the gateway identity exactly `create`, `read`, `update`, and
`delete` on `openshell-gw/{{...service_account_namespace}}/data/openshell/provider-credentials/*`,
and `read` on `openshell-gw/{{...service_account_namespace}}/metadata/openshell/provider-credentials/*`.
It SHALL NOT grant `list` on either path, and SHALL NOT grant `delete` on the metadata path
(a permanent destroy of all versions is not needed for operation; the data-path `delete` is a
reversible KV v2 soft delete). Isolation therefore rests on two reinforcing layers: the
templated path ties a token to its own namespace's mount (a handle leaked from another tenant
is unusable), and the no-`list` capability model means a gateway can reach a secret only by a
handle it already holds, with the random per-provider UUID making the path unguessable. That
per-tenant mount, combined with the per-cluster Vault (each cluster runs its own), is the unit
of gateway-to-gateway and cluster-to-cluster isolation.

The control plane creates and deletes the per-tenant mount through a **narrowly scoped**
provisioner identity (Vault role `openshell-gateway-provisioner`) whose policy grants mount
management under `sys/mounts/openshell-gw/*` and **nothing else**. It grants no `data/`
capability and no policy or role write, so the control plane **cannot read a provider secret**
and **cannot rewrite the isolation boundary**; its worst case is to create or delete a mount
under the prefix (a bounded denial of service, since the control plane can already delete a
whole gateway). The control plane authenticates to this role with a `vault`-audience token it
mints for its **own** ServiceAccount via the Kubernetes TokenRequest API, so no static admin
credential is stored or rotated.

Both Vault roles are additionally bound by a **namespace selector**, a second independent
isolation layer on top of the templated/mounts-only policies: the gateway role admits only
namespaces carrying the control plane's `hypershell.redhat.io/managed` label, and the
provisioner role admits only a namespace carrying `hypershell.redhat.io/control-plane` (the
control plane self-applies this to its own namespace at startup). A namespace-scoped attacker
cannot forge either label - labeling a namespace needs cluster-scoped `patch` on the
`namespaces` resource - so an unrelated namespace cannot even authenticate, which in particular
closes the provisioner mount-delete denial-of-service to any non-control-plane namespace. Vault
evaluates the selectors by reading namespace labels at login, so its server ServiceAccount holds
read-only `get`/`list` on namespaces.

**Sandbox-to-sandbox isolation within one gateway is a separate layer and is not enforced
by Vault.** Sandboxes never authenticate to Vault; the gateway process is the single Vault
client and brokers every credential on a sandbox's behalf. The OpenShell gateway binds each
credential's path to its owning workspace (the workspace is an input to the path hash) and,
on every resolve, recomputes the expected path from the **requesting** workspace and rejects
any handle whose path does not match. A sandbox in one workspace therefore cannot cause the
gateway to resolve another workspace's credential. This authorization is OpenShell's and is
covered by upstream unit tests; HyperShell's contribution is the capability-scoped Vault
policy that backs it (a gateway cannot reach a path it was never handed, even if the
application layer were bypassed).

#### Scenario: OSG-CRED-10.01 -- Gateway policy is templated and capability-scoped with no list

- GIVEN the templated `openshell-gateway-provider` Vault policy
- THEN it SHALL grant `create`, `read`, `update`, `delete` on `openshell-gw/{{...service_account_namespace}}/data/openshell/provider-credentials/*`
- AND `read` on `openshell-gw/{{...service_account_namespace}}/metadata/openshell/provider-credentials/*`
- AND it SHALL NOT grant `list` on either path, SHALL NOT grant `delete` on the metadata path, nor any capability outside the token's own per-tenant mount

#### Scenario: OSG-CRED-10.02 -- Provisioner identity cannot read secrets or rewrite isolation

- GIVEN the `openshell-gateway-provisioner` Vault policy the control plane authenticates with
- THEN it SHALL grant `create`, `read`, `update`, `delete` only on `sys/mounts/openshell-gw/*`
- AND it SHALL NOT grant any capability on a `data/` or `metadata/` secret path
- AND it SHALL NOT grant write on any ACL policy (`sys/policies/acl/*`) or auth role (`auth/*/role/*`)

---

### Requirement: OSG-CRED-11 -- Provider Credential Lifecycle on Deletion

A provider credential's live-value lifecycle is owned by the OpenShell gateway runtime:
the gateway deletes a credential's KV entry (by the handle it holds) when the provider or
credential is removed during normal operation. The gateway's own policy grants the data-path
`delete` (a reversible soft delete) needed for this.

When a whole Vault-backed Gateway is deleted, the control plane removes the gateway's
namespace, ServiceAccount and database, and **deletes the gateway's per-tenant KV mount**
(`openshell-gw/<namespace>`) through its scoped provisioner identity. Deleting the mount
removes every provider-credential entry the gateway stored, in one operation, with no need to
enumerate the entries or hold any handle. The control plane therefore leaves no reachable and
no dead provider-secret storage behind, and records no credential orphan: clean mount deletion
is the expected steady-state outcome of deleting a gateway. Mount deletion is best-effort and
idempotent - if it fails (for example Vault is unreachable at teardown), the control plane
SHALL surface it as a non-fatal signal and a later reconcile retries; a left-behind mount holds
only entries that are already unreferenced (the gateway database is gone) and un-listable from
any tenant identity.

There is no audit-retention exception: if an audit trail is required, it comes from Vault's
own audit log (which records that an access happened, never the value).

#### Scenario: OSG-CRED-11.01 -- Gateway delete removes the per-tenant mount

- GIVEN a Vault-backed Gateway whose runtime stored provider credentials under `openshell-gw/<namespace>/data/openshell/provider-credentials/*`
- WHEN the Gateway is deleted
- THEN the control plane SHALL remove the gateway namespace, ServiceAccount and database
- AND it SHALL delete the gateway's per-tenant KV mount `openshell-gw/<namespace>` through the scoped provisioner identity, removing every entry in one operation
- AND it SHALL NOT record a `VaultCredentialPath` orphan
- AND a failed mount deletion SHALL be a non-fatal signal that a later reconcile retries

---

### Requirement: OSG-CRED-12 -- Credential Secret Hygiene

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

#### Scenario: OSG-CRED-12.01 -- Generated config contains no secret material

- GIVEN a Vault-backed Gateway
- WHEN the control plane renders and applies `gateway.toml`
- THEN the rendered config SHALL contain no provider secret value and no ServiceAccount token

#### Scenario: OSG-CRED-12.02 -- Reachability failure does not log the token

- GIVEN a Vault-backed Gateway whose reachability/auth probe fails
- WHEN the control plane logs the failure and sets the condition
- THEN the log line and the condition message SHALL name the failure (for example "vault auth denied")
- AND SHALL NOT contain the ServiceAccount token or any provider secret

---

### Requirement: OSG-CRED-13 -- Credential Driver Immutability

The `credential_driver` configuration on a Gateway SHALL be immutable after the gateway
has stored provider credentials. Changing the driver would orphan credentials in the
previous backend.

#### Scenario: OSG-CRED-13.01 -- Attempt to change credential driver

- GIVEN a Gateway with `credential_driver.type` = `vault` that has active provider credentials
- WHEN a user attempts to change `credential_driver.type` to `kubernetes-secrets`
- THEN the API server SHALL reject the update with error: `credential driver cannot be changed after provider credentials have been stored; migrate credentials manually first`

> **Phase 2 (reserved):** Automated credential migration between drivers.

---

### Requirement: OSG-CRED-14 -- End-to-End Vault Coverage in CI

The Vault credential path SHALL be proven in CI so `vault` can become the flat default (see
Requirement: Credential Driver Configuration Field) without reddening e2e. Each environment's
bring-up (Kind and OpenShift) SHALL stand up a **stand-in HashiCorp Vault** (the same software
the per-cluster production Vault runs), as one Vault per cluster in namespace `openshell-vault`
at the convention address `openshell-gateway-vault.openshell-vault.svc:8200` - the per-cluster
production shape, so the control plane uses the convention with no address override - configured
to the GitOps Contract below (KV v2, Kubernetes auth, the `openshell-gateway` role and the
templated capability-scoped policy, the `openshell-gateway-provisioner` role and policy, a
per-tenant mount `openshell-gw/<namespace>`, a serving CA). The stand-in differs from production only in
the seal/storage plumbing (file storage + a scripted, idempotent init+unseal, not KMS/Shamir +
managed Postgres), and its setup is idempotent so it is safe as a shared fixture. See
[`e2e-testing.spec.md`](./e2e-testing.spec.md).

Coverage is split so each layer is proven where it belongs, and the e2e suite does not reach
into Vault with a synthetic identity:

- **The templated capability-scoped policy** (for a token from a given namespace, a round-trip
  succeeds under its own `openshell-gw/<namespace>/data/openshell/provider-credentials/*`; `list`
  is denied; metadata `delete` is denied; another tenant's mount is denied; and access revocation
  holds - removing the `openshell-gateway-provider` policy denies the gateway, after which the
  fixture restores it) is the GitOps Contract, authored in `hypershell-gitops`. The stand-in Vault
  fixture SHALL **self-verify** it once at setup, acting as a gateway identity (a `vault`-audience
  ServiceAccount token logged in under the `openshell-gateway` role, against a per-tenant mount it
  enables for the check), and SHALL fail the bring-up if the policy drifts. This keeps the contract
  check in the fixture, not in the product e2e.
- **The real gateway-process resolve** is exercised end-to-end by the suite's own **sandbox
  lifecycle**: creating a sandbox drives the actual OpenShell gateway to resolve a provider
  credential from Vault under its per-tenant `openshell-gw/<namespace>/.../provider-credentials/*`,
  so if the policy or mount were wrong the sandbox would never reach Running.
- **Fail-closed** when Vault is unreachable is asserted by the suite through the **product API**
  only (no Vault fixture or kube access required).

#### Scenario: OSG-CRED-14.01 -- Vault fixture self-verifies the capability policy

- GIVEN a stand-in Vault stood up to the GitOps Contract
- WHEN the bring-up authenticates as a gateway identity (a `vault`-audience ServiceAccount token
  under the `openshell-gateway` role) and checks the policy against its own per-tenant mount
- THEN a write/read round-trip under its own `openshell-gw/<namespace>/data/openshell/provider-credentials/*` SHALL succeed
- AND a `list` on the `provider-credentials` folder SHALL be denied
- AND a `delete` on the metadata path SHALL be denied
- AND a read or write under another tenant's `openshell-gw/<other-namespace>/*` mount SHALL be denied
- AND removing the `openshell-gateway-provider` policy SHALL deny the gateway identity (access
  revocation); the fixture SHALL then restore the policy so it is left usable
- AND the bring-up SHALL fail if any of these do not hold

#### Scenario: OSG-CRED-14.02 -- Fail-closed is observable when Vault is unavailable

- GIVEN a gateway provisioned with a Vault driver pointed at an unreachable address
- WHEN the GatewayReconciler reconciles
- THEN the suite SHALL observe the fail-closed signal through the API (`EnvironmentReady` fails
  with a Vault-named reason), and the gateway SHALL never report `Running`

**Coverage boundaries.** The sandbox lifecycle exercises the real gateway-process resolve at
`provider-credentials/*`; the fixture self-test proves the capability policy (round-trip, no
list, no out-of-prefix) against the GitOps Contract; the suite asserts fail-closed at
provisioning via the API. The following are proven elsewhere and SHOULD NOT be assumed covered
by this suite: sandbox-to-sandbox / cross-workspace resolve denial (OpenShell gateway workspace
authorization, upstream unit tests), the authenticated-probe *rejection* path (Vault reachable
but the identity denied), and the runtime `Degraded` transition when a healthy gateway's Vault
later becomes unreachable.

---

## GitOps Contract

The per-cluster Vault is stood up by `hypershell-gitops`
(`specs/openshell-gateway-vault.spec.md`). These values are shared literally across both
repos; a mismatch breaks authentication. The **authoritative** definition is that spec's
§5 (Vault configuration contract), since GitOps owns and operates the Vault; the table
below mirrors it for convenience, and on any divergence the GitOps spec wins.

| Item | Value |
|---|---|
| In-cluster address | `https://openshell-gateway-vault.openshell-vault.svc:8200` |
| KV engine / mount | KV v2, per-tenant mount `openshell-gw/<tenant-namespace>` (created by the control plane at provision, deleted at teardown) |
| KV path layout | `openshell-gw/<tenant-namespace>/data/openshell/provider-credentials/<hash>` (`<hash>` is a SHA-256 seeded by a random per-provider UUID, so it is unguessable; the suffix is fixed by the OpenShell runtime, the mount is the per-tenant prefix) |
| Kubernetes auth mount | `kubernetes` |
| Vault role (gateway) | `openshell-gateway` |
| Vault role (control-plane provisioner) | `openshell-gateway-provisioner` |
| Isolation | two independent layers: (1) role **namespace selectors** restrict *who may authenticate* (gateway role → namespaces labeled `hypershell.redhat.io/managed`; provisioner role → the control-plane namespace labeled `hypershell.redhat.io/control-plane`); (2) a **templated** ACL policy scopes each gateway's token to its own namespace's mount, with **no `list`**, so a handle leaked from another tenant is unusable. Isolation is per-tenant mount plus the per-cluster Vault boundary. Sandbox-to-sandbox isolation is enforced by the OpenShell gateway's workspace authorization, not by Vault. |
| Namespace labels (selector inputs) | gateway namespaces carry `hypershell.redhat.io/managed=true` (control plane stamps it); the control-plane namespace carries `hypershell.redhat.io/control-plane=true` (control plane self-labels at startup). The Vault server SA holds read-only `get`/`list` on namespaces to evaluate the selectors. |
| Policy capabilities (gateway) | templated `openshell-gateway-provider` grants the gateway identity `create`, `read`, `update`, `delete` on `openshell-gw/{{...service_account_namespace}}/data/openshell/provider-credentials/*` and `read` on the matching `metadata/` path; **no `list`**, **no metadata `delete`**, nothing outside the token's own mount |
| Policy capabilities (provisioner) | `openshell-gateway-provisioner` grants `create`, `read`, `update`, `delete` only on `sys/mounts/openshell-gw/*`; no `data/` capability, no policy or role write |
| Gateway SA token | projected, `audience: vault`, `expirationSeconds: 3600`, mounted at `/var/run/secrets/vault/token` |
| Serving-CA source | Secret `openshell-gateway-vault-tls` (key `ca.crt`) in namespace `openshell-vault` |
| Serving-CA in tenant ns | ConfigMap `openshell-gateway-vault-ca` (key `ca.crt`), wired to the chart's `credentialDrivers.vault.caConfigMapName` |

Who owns what: `hypershell-gitops` pre-provisions the Vault, the KV engine, the Kubernetes auth
mount, both roles (`openshell-gateway`, `openshell-gateway-provisioner`), the templated gateway
policy and the provisioner policy, once, up front. At runtime the control plane creates and
deletes **only** the per-tenant KV mount (`openshell-gw/<namespace>`) through the scoped
provisioner identity; it writes no Vault policy, role, or auth config, and holds no Vault admin.
It authenticates to Vault two ways, both via the Kubernetes TokenRequest API: as a **gateway
identity** (a `vault`-audience token for the gateway's `openshell-gateway` ServiceAccount) for
the pre-deploy and authenticated reachability probes, and as the **provisioner** (a
`vault`-audience token for the controller's **own** ServiceAccount) to manage the mount. The
controller's RBAC MUST therefore allow `create` on the `serviceaccounts/token` subresource,
scoped by `resourceNames` to `openshell-gateway` (the gateway identity) and the controller's own
ServiceAccount (the provisioner identity), and to no other ServiceAccount. The control plane also
self-labels its own namespace with `hypershell.redhat.io/control-plane=true` at startup (so the
provisioner role's namespace selector admits it), which needs `patch` on the `namespaces`
resource; the Vault server ServiceAccount holds read-only namespace `get`/`list` so Vault can
evaluate the role selectors.

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
