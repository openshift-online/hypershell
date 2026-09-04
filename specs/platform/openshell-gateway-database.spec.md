# OpenShell Gateway Database Specification

**Date:** 2026-08-24
**Status:** Draft
**Parent:** `openshell-gateway.spec.md` - core gateway provisioning

---

## Purpose

This specification defines PostgreSQL database provisioning for OpenShell gateways. The control plane supports two parallel deployment modes, selected via the `DATABASE_PROVIDER` environment variable:

| Mode | `DATABASE_PROVIDER` | Description |
|---|---|---|
| **Deployment** (default) | unset/empty or `deployment` | Uses a standalone PostgreSQL Deployment per gateway. No operator required. Each gateway gets its own dedicated ManagedDatabase (and thus its own PostgreSQL pod) created automatically at gateway creation time. Suitable for environments where installing the CNPG operator is not feasible (e.g. minimal dev clusters), and requires no CNPG APIs at all. |
| **CNPG** | `cnpg` | Uses the [CloudNativePG](https://cloudnative-pg.io/) operator. Multiple gateways share one ManagedDatabase's CNPG Cluster; each gateway gets its own logical database inside it. Requires the exact CNPG CRDs this code depends on (`Cluster`, `Database`, `DatabaseRole` in `postgresql.cnpg.io/v1`) to be installed on the cluster. |
| **External** | `external` | The PostgreSQL server is provisioned outside HyperShell as a cloud-managed database (AWS RDS/Aurora, IBM Cloud Databases). HyperShell registers the endpoint and provisions one dedicated database and login role per gateway inside it, issuing the DDL itself. No operator and no in-cluster PostgreSQL workload. Specified in full by [`openshell-gateway-database-external.spec.md`](./openshell-gateway-database-external.spec.md). |

Any `DATABASE_PROVIDER` value other than unset/empty, `deployment`, `cnpg`, or `external` is a startup configuration error: the API server and control plane SHALL fail to start rather than silently selecting a provider.

PostgreSQL is the only supported database backend for HyperShell gateways.

---

## Prerequisites

### CNPG Mode

The CNPG operator SHALL be installed on every managed cluster that opts into `DATABASE_PROVIDER=cnpg`, similar to cert-manager. The operator provides the `Cluster`, `Database`, and `DatabaseRole` CRDs in `postgresql.cnpg.io/v1`.

- In the Kind development environment with CNPG mode (`DATABASE_PROVIDER=cnpg`), `make kind-up` installs the operator (see `local-development.spec.md`)
- In production environments, the platform administrator installs the operator before setting `DATABASE_PROVIDER=cnpg` and registering the cluster with HyperShell

The control plane SHALL verify the exact CNPG API resources it depends on at startup: `clusters`, `databases`, and `databaseroles` in `postgresql.cnpg.io/v1`. It is not sufficient for some `postgresql.cnpg.io` API group to exist; all three resources must be served. If `DATABASE_PROVIDER=cnpg` and any of them is unavailable, the control plane SHALL fail to start with an explicit, contextual error (return/propagate the error and exit non-zero) rather than starting in a partially-functional state or panicking. See Requirement: CNPG Operator Detection below.

### Deployment Mode

No operator prerequisites. The control plane provisions a standard PostgreSQL Deployment using the `postgres:18` image (or the image configured via `OPENSHELL_DATABASE_IMAGE`).

---

## Architecture

### ManagedDatabase as the Database Abstraction Layer

In both modes, a `ManagedDatabase` resource is the indirection between a gateway and its PostgreSQL infrastructure. A `Gateway` has a `database_id` foreign key pointing to a `ManagedDatabase`. The `ManagedDatabase` has a `provider` field (`cnpg` or `deployment`) that determines how the ManagedDatabaseReconciler provisions its infrastructure, and a `namespace` field (auto-assigned at creation) that locates that infrastructure in the cluster.

### CNPG Mode Architecture

```
ManagedDatabase (provider=cnpg)
  │  ManagedDatabaseReconciler
  ▼
Namespace: openshell-db-<hex16>
  └── CNPG Cluster: openshell-db
        │  GatewayReconciler (per gateway)
        ├── DatabaseRole: gw-<gatewayID>
        ├── Database: gw-<gatewayID>
        └── Secret: gw-<gatewayID>-credentials

Gateway A ──database_id──→ ManagedDatabase ──namespace──→ CNPG Cluster
Gateway B ──database_id──→ ManagedDatabase ──namespace──→ CNPG Cluster (same)
Gateway C ──database_id──→ ManagedDatabase (different) ──→ different CNPG Cluster
```

Multiple gateways can share one ManagedDatabase. The ManagedDatabaseReconciler creates the CNPG Cluster once; the GatewayReconciler adds per-gateway `DatabaseRole`, `Database`, and password `Secret` CRs to it.

### Deployment Mode Architecture

```
Gateway created
  │  API server (deploymentPlacement)
  ▼
ManagedDatabase (provider=deployment) ← auto-created per gateway
  │  ManagedDatabaseReconciler
  ▼
Namespace: openshell-db-<hex16>
  ├── PVC: openshell-gateway-db-data
  ├── Deployment: openshell-gateway-db (postgres:18)
  ├── Service: openshell-gateway-db
  └── Secret: openshell-db-credentials

GatewayReconciler
  └── copies Secret → openshell-gateway-db-credentials (tenant namespace)
```

Each gateway gets its own dedicated ManagedDatabase (and thus its own PostgreSQL pod). The API server auto-creates the ManagedDatabase at gateway creation time; no pre-existing database resource is required.

### ManagedDatabase Namespace Naming

In both modes, the ManagedDatabase namespace is derived from the ManagedDatabase's KSUID using the same pattern as gateway namespaces: `openshell-db-<hex16>`, where `<hex16>` is the lowercase hexadecimal encoding of 8 bytes from the KSUID's random payload. The API server assigns this namespace in `BeforeCreate`. Example: `openshell-db-a1b2c3d4e5f67890`.

### Automatic Database Assignment

The Gateway create contract keeps `cluster_id` and `database_id`. The API server stores `cluster_id` but SHALL NOT resolve, validate, or use it for database placement in any mode. The `database_id` value is server-owned in every mode. Clients send an empty string, and the API server SHALL ignore and replace any non-empty value. Omitting the property is invalid. The server-side `database_id` placement strategy depends on `DATABASE_PROVIDER`:

**CNPG mode (`cnpgPlacement`):** For every Gateway creation, the API server ignores the requested `database_id` and queries all ManagedDatabases. If exactly one ManagedDatabase exists, the API server assigns its ID. If zero or more than one exist, the API server rejects the request.

**Deployment mode (`deploymentPlacement`):** For every Gateway creation, the API server ignores the requested `database_id`, creates a new ManagedDatabase (provider=deployment) for that gateway, and assigns its ID. No existing ManagedDatabase is necessary. A caller cannot select a database that another gateway uses.

**External mode:** For every Gateway creation, the API server ignores the requested `database_id` and selects the sole registered `external` ManagedDatabase (same exactly-one constraint as CNPG mode, filtered to `provider=external`). Zero or more than one is rejected. No ManagedDatabase is created - external servers are registered by an administrator, never provisioned by HyperShell.

The admin workflows differ accordingly:

| | CNPG mode | Deployment mode | External mode |
|---|---|---|---|
| Pre-requisite | Create one ManagedDatabase (provider=cnpg) | None | Provision the server out-of-band, create its admin Secret, and register exactly one ManagedDatabase (provider=external) |
| Gateway creation | Provide `name`; database auto-resolved; `cluster_id` has no placement effect | Provide `name`; ManagedDatabase auto-created; `cluster_id` has no placement effect | Provide `name`; sole external ManagedDatabase auto-selected; `cluster_id` has no placement effect |

In the Kind development environment, `make kind-up` seeds a single `openshell-db` ManagedDatabase in CNPG mode; no seeding is needed in deployment mode.

---

## Requirements

### Requirement: ManagedDatabase Provider Validation and Immutability

The API server SHALL accept only `cnpg`, `deployment`, and `external` as ManagedDatabase provider values. The `external` provider carries additional create-time validation specified in [`openshell-gateway-database-external.spec.md`](./openshell-gateway-database-external.spec.md). Once a ManagedDatabase has a supported provider, that provider is immutable: REST patches, gRPC updates, and internal callers MAY resend the same value but SHALL NOT transition the resource to another provider. Status-only and other mutable-field updates SHALL preserve the provider. A legacy resource whose persisted provider is unsupported MAY be corrected once to a supported provider; after correction, normal immutability applies.

#### Scenario: Attempt to change a supported provider

- GIVEN an existing ManagedDatabase with `provider: "deployment"`
- WHEN a caller attempts to update its provider to `cnpg` or an unsupported value
- THEN the API server SHALL reject the update as invalid input
- AND the persisted provider SHALL remain `deployment`

### Requirement: ManagedDatabase Reconciliation (provider=cnpg)

The ManagedDatabaseReconciler SHALL provision CNPG infrastructure for each ManagedDatabase with `provider: "cnpg"`.

For each such ManagedDatabase, the reconciler SHALL:

1. **Create the namespace** derived from the ManagedDatabase ID (`openshell-db-<hex16>`), if it does not exist
2. **Create a CNPG Cluster CR** in that namespace:
   - `metadata.name` = `openshell-db` (fixed name; isolation is per-namespace, not per-cluster-name)
   - `spec.instances` = 1 (fixed default)
   - `spec.storage.size` = `1Gi` (fixed default)
   - `spec.resources` = `requests: {memory: 256Mi}`, `limits: {memory: 512Mi}`
   - `spec.imageName` = value of `OPENSHELL_DATABASE_IMAGE` env var (omitted when unset; CNPG uses its default image)
3. **Wait for the CNPG Cluster** to reach `Ready` status (all instances running)
4. **Update the ManagedDatabase status** in the API server to reflect readiness

#### Scenario: New ManagedDatabase created (provider=cnpg)

- GIVEN a new ManagedDatabase resource with `provider: "cnpg"`
- WHEN the ManagedDatabaseReconciler processes the event
- THEN it SHALL create the namespace `openshell-db-<hex16>`
- AND it SHALL create a CNPG Cluster CR in that namespace
- AND it SHALL wait for the Cluster to reach Ready
- AND it SHALL update the ManagedDatabase status

#### Scenario: ManagedDatabase already exists (idempotent, cnpg)

- GIVEN a ManagedDatabase whose CNPG Cluster is already running
- WHEN the ManagedDatabaseReconciler re-processes the event
- THEN it SHALL verify the namespace and Cluster exist
- AND it SHALL NOT recreate them

---

### Requirement: ManagedDatabase Reconciliation (provider=deployment)

The ManagedDatabaseReconciler SHALL provision a standalone PostgreSQL Deployment for each ManagedDatabase with `provider: "deployment"`.

For each such ManagedDatabase, the reconciler SHALL:

1. **Create the namespace** derived from the ManagedDatabase ID (`openshell-db-<hex16>`), if it does not exist
2. **Create or update a credentials Secret** (`openshell-db-credentials`) in that namespace:
   - `user` = `openshell`
   - `password` = 32-byte cryptographically random hex string (`crypto/rand`), generated once and preserved on subsequent reconciliations
   - `dbname` = `openshell`
   - `host`, `port`, and `uri` are recomputed and converged on every reconciliation
   - `uri` = `postgresql://openshell:<password>@openshell-gateway-db.<namespace>.svc.cluster.local:5432/openshell?sslmode=disable`
3. **Create or update a PVC** (`openshell-gateway-db-data`, `1Gi`)
4. **Create or update a Deployment** (`openshell-gateway-db`) running `postgres:18` (or `OPENSHELL_DATABASE_IMAGE`):
   - Mounts the PVC at `/var/lib/postgresql/data`
   - Reads `POSTGRES_PASSWORD` from the credentials Secret
   - On vanilla Kubernetes, uses image-specific PostgreSQL UID/GID (`999` for upstream `postgres`, `26` for Red Hat PostgreSQL images), a matching pod `fsGroup`, and `fsGroupChangePolicy: OnRootMismatch` so a freshly provisioned volume is writable
   - On OpenShift, omits fixed `runAsUser`, `runAsGroup`, `fsGroup`, and `fsGroupChangePolicy` values so the restricted SCC assigns namespace-valid identities; all other restricted security controls remain enabled
   - Uses upstream `POSTGRES_*` variables and `/var/lib/postgresql/data` paths by default; legacy RHEL `postgresql-*` images use `POSTGRESQL_*` variables and `/var/lib/pgsql/data`
   - Mounts writable `emptyDir` volumes for `/var/run/postgresql` and `/tmp` while keeping the container root filesystem read-only
   - `securityContext`: `runAsNonRoot: true`, seccomp `RuntimeDefault`, no privilege escalation, and drops `ALL` capabilities
5. **Create or update a Service** (`openshell-gateway-db`, port 5432, ClusterIP)
6. **Wait up to 2 minutes** for the Deployment to become ready (all replicas available)
7. **Update the ManagedDatabase status** in the API server to reflect readiness

All resources carry label `hypershell.redhat.io/managed: "true"` and the `app: openshell-gateway-db` selector.

#### Scenario: New ManagedDatabase created (provider=deployment)

- GIVEN a new ManagedDatabase resource with `provider: "deployment"`
- WHEN the ManagedDatabaseReconciler processes the event
- THEN it SHALL create the namespace, credentials Secret, PVC, Deployment, and Service
- AND it SHALL wait for the Deployment to become ready (2-minute timeout)
- AND it SHALL update the ManagedDatabase status

#### Scenario: ManagedDatabase already exists (idempotent, deployment)

- GIVEN a ManagedDatabase whose Deployment is already running
- WHEN the ManagedDatabaseReconciler re-processes the event
- THEN it SHALL reconcile each resource (create-or-update) without regenerating the password

#### Scenario: Deployment readiness timeout (provider=deployment)

- GIVEN a ManagedDatabase with `provider: "deployment"`
- WHEN the Deployment does not become ready within 2 minutes
- THEN the reconciler SHALL return an error
- AND the ManagedDatabase phase SHALL remain `Provisioning`
- AND the next reconciliation SHALL retry

---

### Requirement: ManagedDatabase Deletion Protection

A ManagedDatabase SHALL NOT be deleted if any Gateway references it via `database_id`. This prevents orphaned gateway databases.

A ManagedDatabase delete watch event SHALL carry the soft-deleted resource as a tombstone, including at least its ID, provider, and namespace. The API server SHALL retain soft-deleted rows and expose their tombstones through a dedicated paginated replay mode of the watch RPC so cleanup survives control-plane restarts and watch disconnections. When RBAC is enabled, this fleet-unscoped historical replay mode SHALL be restricted to the configured control-plane service-account allowlist; ordinary gateway roles SHALL retain no access to it. The control plane SHALL start and continuously drain the live subscription before requesting historical replay, preventing replay backpressure from filling the live broker buffer. The watch subscription handshake SHALL advertise the `hypershell-managed-database-delete-tombstones: v1` capability; a control plane that requires tombstones SHALL reject a watch lacking that capability rather than silently accepting an incompatible stream. Release rollout SHALL update and confirm the API server Ready before updating the control plane. The control plane SHALL use the tombstone to select the cleanup path, retain failed cleanup in a watch-lifetime retry queue until it succeeds, and treat already-absent resources as successful cleanup. It SHALL propagate every non-NotFound cleanup failure and SHALL NOT guess a namespace from a resource name or ID.

#### Scenario: Attempt to delete ManagedDatabase with referencing gateways

- GIVEN a ManagedDatabase referenced by one or more Gateways
- WHEN a user attempts to delete the ManagedDatabase
- THEN the API server SHALL reject the deletion with HTTP 409: "managed database cannot be deleted while gateways reference it; reassign or delete all referencing gateways first"

#### Scenario: Delete ManagedDatabase (cnpg) with no referencing gateways

- GIVEN a ManagedDatabase (provider=cnpg) with no Gateway references
- WHEN a user deletes the ManagedDatabase
- THEN the ManagedDatabaseReconciler SHALL delete the CNPG Cluster CR
- AND delete the namespace `openshell-db-<hex16>`

#### Scenario: Delete ManagedDatabase (deployment) with no referencing gateways

- GIVEN a ManagedDatabase (provider=deployment) with no Gateway references
- WHEN a user deletes the ManagedDatabase
- THEN the delete watch event SHALL include the soft-deleted ManagedDatabase tombstone
- AND the ManagedDatabaseReconciler SHALL delete the Deployment, Service, PVC, and credentials Secret
- AND delete the namespace `openshell-db-<hex16>`
- AND replaying the same delete event after those resources are absent SHALL succeed

---

### Requirement: Gateway Database Resolution

The GatewayReconciler SHALL resolve the gateway's `database_id` to a ManagedDatabase resource to determine the provider and the infrastructure location. The global `CNPG_CLUSTER_NAME`/`CNPG_CLUSTER_NAMESPACE` environment variables are removed; each gateway's database target is derived from its ManagedDatabase. Every gateway MUST have a `database_id`; a missing value is an error.

#### Resolution flow:

1. Read the gateway's `database_id` from the gRPC watch event
2. Look up the ManagedDatabase resource via the API server
3. Extract `namespace` and `provider`
4. Dispatch based on the ManagedDatabase's `provider` field (this is independent of the control plane's `DATABASE_PROVIDER` startup setting, which only gates the CNPG-availability precondition at process startup -- see Requirement: CNPG Operator Detection -- so existing CNPG-backed gateways keep reconciling correctly even when `DATABASE_PROVIDER` defaults to `deployment`)

#### Scenario: Gateway with valid database_id (cnpg)

- GIVEN a Gateway with a `database_id` pointing to a ManagedDatabase (provider=cnpg)
- WHEN the GatewayReconciler processes the event
- THEN it SHALL resolve the ManagedDatabase's namespace and cluster name
- AND proceed with per-gateway CNPG resource provisioning in that namespace

#### Scenario: Gateway with valid database_id (deployment)

- GIVEN a Gateway with a `database_id` pointing to a ManagedDatabase (provider=deployment)
- WHEN the GatewayReconciler processes the event
- THEN it SHALL wait for the live `openshell-gateway-db` Deployment in the ManagedDatabase namespace to become ready
- AND only after readiness SHALL it read the credentials Secret from the ManagedDatabase's namespace
- AND copy it into the gateway's tenant namespace as `openshell-gateway-db-credentials`

#### Scenario: Gateway created with database auto-resolved (cnpg mode)

- GIVEN `DATABASE_PROVIDER=cnpg` and a Gateway with any `cluster_id` and blank `database_id`
- WHEN the API server processes the creation request
- THEN the API server SHALL query all ManagedDatabases
- AND if exactly one exists, assign its ID as the gateway's `database_id`
- AND if zero or more than one exist, reject the creation with an error
- AND preserve the requested `cluster_id` value

#### Scenario: Gateway created (deployment mode, ManagedDatabase auto-created)

- GIVEN `DATABASE_PROVIDER=deployment` (or unset/empty, the default) and a Gateway request containing the required `database_id` property
- WHEN the API server processes the creation request
- THEN the API server SHALL ignore the property's value, including any non-empty caller-selected ID
- AND auto-create a new ManagedDatabase (provider=deployment) for this gateway
- AND assign the new ManagedDatabase's ID as the gateway's `database_id`

---

### Requirement: Per-Gateway Database Provisioning (CNPG Mode)

In CNPG mode, the GatewayReconciler SHALL provision a dedicated PostgreSQL database and role for each gateway using CNPG custom resources. All CNPG resources (DatabaseRole, Database, and the password Secret) SHALL be created in the ManagedDatabase's namespace.

For each gateway, the reconciler SHALL create three resources in the ManagedDatabase's namespace:

1. **Password Secret** (`gw-<gatewayID>-credentials`)
   - Type: `kubernetes.io/basic-auth`
   - `username` = `gw_<gatewayID>` (the PostgreSQL role name)
   - `password` = 32-byte cryptographically random hex string (`crypto/rand`)
   - Label: `cnpg.io/reload: "true"` (ensures CNPG applies password changes immediately)
   - Label: `hypershell.redhat.io/managed: "true"`
   - Created with create-or-skip semantics (do NOT update password on re-reconciliation)

2. **DatabaseRole** (`gw-<gatewayID>`)
   - Kind: `DatabaseRole` (apiVersion: `postgresql.cnpg.io/v1`)
   - `spec.cluster.name` = the ManagedDatabase's CNPG Cluster name
   - `spec.name` = `gw_<gatewayID>` (the PostgreSQL role name; underscores for valid SQL identifiers)
   - `spec.login` = `true`
   - `spec.passwordSecret.name` = `gw-<gatewayID>-credentials`
   - `spec.databaseRoleReclaimPolicy` = `delete` (drop role when CR is deleted)
   - Label: `hypershell.redhat.io/managed: "true"`
   - Label: `hypershell.redhat.io/gateway-namespace: "<tenant-namespace>"` (for cleanup)

3. **Database** (`gw-<gatewayID>`)
   - Kind: `Database` (apiVersion: `postgresql.cnpg.io/v1`)
   - `spec.cluster.name` = the ManagedDatabase's CNPG Cluster name
   - `spec.name` = `gw_<gatewayID>` (the PostgreSQL database name)
   - `spec.owner` = `gw_<gatewayID>` (same as the role)
   - `spec.databaseReclaimPolicy` = `delete` (drop database when CR is deleted)
   - Label: `hypershell.redhat.io/managed: "true"`
   - Label: `hypershell.redhat.io/gateway-namespace: "<tenant-namespace>"` (for cleanup)

> **Naming convention:** `<gatewayID>` is the Gateway's full resource ID (lowercased). The database and role use underscores (`gw_<gatewayID>`) because PostgreSQL identifiers conventionally avoid hyphens. The Kubernetes CR names use hyphens (`gw-<gatewayID>`) per Kubernetes naming conventions.

The reconciler SHALL then wait for the CNPG `Database` CR to reach `status.applied: true` (2-minute timeout) before creating the tenant-namespace credentials Secret and proceeding to deploy the gateway workload.

---

### Requirement: Per-Gateway Database Provisioning (Deployment Mode)

In deployment mode, the GatewayReconciler SHALL copy the shared credentials Secret from the ManagedDatabase's namespace into the gateway's tenant namespace. The PostgreSQL instance is already provisioned by the ManagedDatabaseReconciler; the gateway reconciler only propagates access.

The reconciler SHALL:

1. Observe the live `openshell-gateway-db` Deployment in the ManagedDatabase's namespace and wait up to 2 minutes for it to become ready
2. Read the `openshell-db-credentials` Secret from the ManagedDatabase's namespace
3. Create or update `openshell-gateway-db-credentials` in the gateway's tenant namespace with the same connection details
4. Proceed to apply the gateway workload only after the readiness check and credential copy succeed

No CNPG CRs are created. A timeout or canceled wait SHALL return a contextual error without copying credentials or creating a new gateway workload; transient observation errors MAY be retried within the bounded readiness window.

---

### Requirement: Gateway Credentials Secret

After per-gateway database provisioning, the GatewayReconciler SHALL ensure a credentials Secret named `openshell-gateway-db-credentials` exists in the gateway's tenant namespace. The gateway workload consumes this Secret for its database connection.

The Secret contents differ by provider:

**CNPG mode:**

| Key | Value |
|---|---|
| `host` | `openshell-db-rw.<managed-db-namespace>.svc.cluster.local` |
| `port` | `5432` |
| `dbname` | `gw_<gatewayID>` |
| `user` | `gw_<gatewayID>` |
| `password` | generated password |
| `uri` | `postgresql://gw_<gatewayID>:<password>@<host>:5432/gw_<gatewayID>?sslmode=require` |

**Deployment mode:**

| Key | Value |
|---|---|
| `host` | `openshell-gateway-db.<managed-db-namespace>.svc.cluster.local` |
| `port` | `5432` |
| `dbname` | `openshell` |
| `user` | `openshell` |
| `password` | generated password (from the ManagedDatabase's credentials Secret) |
| `uri` | `postgresql://openshell:<password>@<host>:5432/openshell?sslmode=disable` |

The `uri` key provides the full connection string for the gateway's `--db-url` argument. The gateway Deployment SHALL reference this Secret via environment variable.

> **TLS:** CNPG clusters enable TLS by default (`sslmode=require`). Deployment mode uses a plain TCP connection without TLS (`sslmode=disable`). Upgrading deployment mode to TLS is a future hardening step.

---

### Requirement: Database Provisioning Readiness

**CNPG mode:** The GatewayReconciler SHALL wait for the CNPG `Database` CR to reach `status.applied: true` (2-minute timeout) before proceeding to deploy the gateway workload.

**Deployment mode:** The ManagedDatabaseReconciler waits for the Deployment to become ready (2-minute timeout) before marking the ManagedDatabase ready. Because ManagedDatabase and Gateway events are reconciled independently, the GatewayReconciler SHALL also observe the live Deployment and wait up to 2 minutes for readiness before copying credentials or creating a new gateway workload. A credentials Secret alone is not proof of readiness.

#### Scenario: Database provisioning completes successfully (cnpg)

- GIVEN a new Gateway resource (cnpg mode)
- WHEN the GatewayReconciler creates the DatabaseRole and Database CRs
- AND the CNPG operator reconciles them (`status.applied: true`)
- THEN the reconciler SHALL proceed to deploy the gateway workload

#### Scenario: Database provisioning times out (cnpg)

- GIVEN a new Gateway resource (cnpg mode)
- WHEN the CNPG Database CR does not reach `status.applied: true` within 2 minutes
- THEN the reconciler SHALL return an error
- AND the Gateway phase SHALL remain `Provisioning`
- AND the next reconciliation SHALL retry

---

### Requirement: Database Credential Security

- Passwords SHALL be generated using `crypto/rand` (32 bytes, hex-encoded)
- Passwords SHALL NEVER appear in log messages or error strings
- Password Secrets SHALL be created with create-or-skip semantics (do NOT update password on re-reconciliation)
- In CNPG mode, the `cnpg.io/reload: "true"` label on the password Secret ensures CNPG picks up password changes immediately when rotation occurs

---

### Requirement: Manual Credential Rotation

The GatewayReconciler SHALL support manual database credential rotation for CNPG mode gateways.

- To trigger rotation, an operator adds the annotation `hypershell.redhat.io/rotate-db-credentials: "<timestamp>"` to the Gateway resource
- When the reconciler detects a new value for this annotation (different from the last-observed value stored on the gateway credentials Secret), it SHALL:
  1. Generate a new password using `crypto/rand`
  2. Update the password Secret (`gw-<gatewayID>-credentials`) in the ManagedDatabase namespace
  3. The CNPG operator applies the password change to PostgreSQL automatically (via the `cnpg.io/reload` label)
  4. Update the gateway credentials Secret (`openshell-gateway-db-credentials`) in the tenant namespace
  5. Set annotation `hypershell.redhat.io/last-db-rotation` on the gateway credentials Secret to match the trigger annotation
  6. The config-hash annotation on the Deployment changes, triggering a rolling restart

The full rotation design (procedure, failure handling, config-hash coverage) is specified in [`openshell-gateway-secret-rotation.spec.md`](./openshell-gateway-secret-rotation.spec.md).

---

### Requirement: Gateway Workload Type

The gateway workload SHALL always be deployed as a Deployment (not a StatefulSet). In both modes, the gateway workload does not require persistent local storage; database storage is managed by the ManagedDatabase's infrastructure (CNPG Cluster or standalone Deployment).

---

### Requirement: Database Resource Provisioning Order

In CNPG mode, CNPG resources (password Secret, DatabaseRole, Database) SHALL be created BEFORE the gateway workload. After creating the Database CR, the control plane SHALL wait for `status.applied: true` (2-minute timeout) before deploying the gateway.

In deployment mode, the ManagedDatabase's Deployment SHALL be ready before the GatewayReconciler attempts to copy the credentials Secret.

---

### Requirement: Gateway Deletion Cleanup

When a Gateway is deleted:

**CNPG mode:** The control plane SHALL delete the CNPG resources in the ManagedDatabase's namespace:

1. Delete the `Database` CR (`gw-<gatewayID>`) -- CNPG drops the PostgreSQL database (`databaseReclaimPolicy: delete`)
2. Delete the `DatabaseRole` CR (`gw-<gatewayID>`) -- CNPG drops the PostgreSQL role (`databaseRoleReclaimPolicy: delete`)
3. Delete the password Secret (`gw-<gatewayID>-credentials`) in the ManagedDatabase namespace

CNPG resources in the ManagedDatabase namespace are identified for cleanup using the label `hypershell.redhat.io/gateway-namespace: "<tenant-namespace>"`.

**Deployment mode:** Because each gateway has its own dedicated ManagedDatabase, the ManagedDatabase is deleted along with the gateway (since it will have no remaining references). ManagedDatabase deletion triggers cleanup of the Deployment, Service, PVC, and credentials Secret in the ManagedDatabase namespace.

**Both modes:** Resources in the gateway's tenant namespace (including `openshell-gateway-db-credentials`) are cleaned up by the existing label-based cleanup (`hypershell.redhat.io/managed: "true"`).

#### Scenario: Delete gateway (cnpg mode)

- GIVEN a Gateway (cnpg mode) with no active sandboxes
- WHEN a user deletes the Gateway
- THEN the control plane SHALL delete CNPG resources (Database, DatabaseRole, password Secret) from the ManagedDatabase namespace
- AND delete all resources with label `hypershell.redhat.io/managed: "true"` from the tenant namespace

#### Scenario: Delete gateway (deployment mode)

- GIVEN a Gateway (deployment mode) with no active sandboxes
- WHEN a user deletes the Gateway
- THEN the control plane SHALL delete the gateway's dedicated ManagedDatabase
- AND the ManagedDatabaseReconciler SHALL delete the Deployment, Service, PVC, credentials Secret, and namespace
- AND delete all resources with label `hypershell.redhat.io/managed: "true"` from the tenant namespace

---

### Requirement: Gateway Deletion With Active Sandboxes (Advisory)

Active sandboxes SHALL NOT block Gateway deletion. Before an operator deletes a
Gateway, the active sandbox count is surfaced as a warning so they can see how
many running sessions the deletion would disrupt (see
[`openshell-gateway-namespace-gc.spec.md`](./openshell-gateway-namespace-gc.spec.md)
§ Surface Active Sandbox Count Before Deletion and
[`openshell-gateway-sandbox-count.spec.md`](./openshell-gateway-sandbox-count.spec.md)),
but the count is advisory only: deletion proceeds regardless and reclaims the
gateway's resources.

#### Scenario: Delete gateway that has active sandboxes

- GIVEN a Gateway with one or more active sandboxes
- WHEN a user deletes the Gateway (having been warned of the active sandbox count)
- THEN the API server SHALL accept the deletion and SHALL NOT reject it on account of the active sandboxes
- AND the control plane SHALL reclaim the gateway's namespace, disrupting those sandboxes and cascading removal of their in-namespace resources

---

### Requirement: DATABASE_PROVIDER Selection And Validation

Both the API server and the control plane read `DATABASE_PROVIDER` independently at startup and resolve it with the same rule:

- Unset or empty resolves to `deployment` (the default).
- `deployment` selects deployment-backed ManagedDatabase placement, which never requires any CNPG API.
- `cnpg` selects CNPG-backed placement, subject to the exact-resource startup check below.
- `external` selects external-backed placement and per-gateway DDL against a registered cloud-managed server. It imposes no CNPG startup check; its preconditions (reachability, a valid admin connection Secret) are evaluated at reconcile time, not at startup.
- Any other value is a startup configuration error: the process SHALL fail to start (return/propagate a contextual error and exit non-zero -- never panic) rather than silently falling back to `cnpg` or any other provider.

#### Scenario: DATABASE_PROVIDER unset defaults to deployment

- GIVEN `DATABASE_PROVIDER` is unset or empty
- WHEN the API server or the control plane starts
- THEN it SHALL resolve the database provider to `deployment`
- AND it SHALL NOT require any CNPG API to be present

#### Scenario: DATABASE_PROVIDER=deployment selects deployment placement

- GIVEN `DATABASE_PROVIDER=deployment`
- WHEN the API server or the control plane starts
- THEN it SHALL select deployment-backed ManagedDatabase placement
- AND it SHALL NOT require any CNPG API to be present

#### Scenario: Unsupported DATABASE_PROVIDER value fails startup

- GIVEN `DATABASE_PROVIDER` is set to a value other than unset/empty, `deployment`, `cnpg`, or `external` (e.g. `postgres`, or a differently-cased `CNPG`)
- WHEN the API server or the control plane starts
- THEN it SHALL fail to start with an explicit, contextual error naming the invalid value and the supported values
- AND it SHALL NOT silently select `cnpg` (or any other provider) as a fallback

---

### Requirement: CNPG Operator Detection

When `DATABASE_PROVIDER=cnpg`, the control plane SHALL verify at startup that the exact CNPG API resources this codebase depends on are served: `clusters`, `databases`, and `databaseroles` in `postgresql.cnpg.io/v1`. Checking only that some `postgresql.cnpg.io` API group exists is NOT sufficient -- a partial or version-mismatched CNPG install could serve one of these resources without serving the others, and that must be caught at startup rather than surfacing later as an unstructured-apply failure deep inside gateway or ManagedDatabase provisioning.

If any of the three required resources is unavailable, the control plane SHALL fail to start: return/propagate a contextual error and exit non-zero. It SHALL NOT panic, and it SHALL NOT start in a partially-functional state that later fails opaquely.

When `DATABASE_PROVIDER=deployment` (including the default, unset case), this check is skipped entirely and no CNPG API is required.

This startup check is independent of the per-resource CNPG capability detection the reconcilers use to gate individual ManagedDatabase/Gateway resources whose `provider` is `cnpg` (see Requirement: Gateway Database Resolution): that detection remains a best-effort, non-fatal check so existing CNPG-backed gateways keep working when the cluster has CNPG installed, even if the control plane's own `DATABASE_PROVIDER` default is `deployment`.

#### Scenario: DATABASE_PROVIDER=cnpg with all required CNPG resources present

- GIVEN `DATABASE_PROVIDER=cnpg` and the cluster serves `clusters`, `databases`, and `databaseroles` in `postgresql.cnpg.io/v1`
- WHEN the control plane starts
- THEN it SHALL start successfully and select CNPG-backed placement

#### Scenario: DATABASE_PROVIDER=cnpg with the CNPG operator absent

- GIVEN `DATABASE_PROVIDER=cnpg` and no `postgresql.cnpg.io` API group is served
- WHEN the control plane starts
- THEN it SHALL fail to start with an explicit, contextual error
- AND it SHALL exit non-zero rather than panicking or starting in a degraded mode

#### Scenario: DATABASE_PROVIDER=cnpg with a partial CNPG install

- GIVEN `DATABASE_PROVIDER=cnpg` and `postgresql.cnpg.io/v1` serves `clusters` and `databases` but not `databaseroles`
- WHEN the control plane starts
- THEN it SHALL fail to start with an explicit, contextual error naming the missing `databaseroles` resource
- AND it SHALL NOT treat the partial install as CNPG being available

---

## Configuration Reference

| Variable | Type | Default | Description |
|---|---|---|---|
| `DATABASE_PROVIDER` | env var | `deployment` (unset/empty resolves to it) | Selects the database deployment mode. Valid values: `deployment`, `cnpg`, `external`. Any other value is a startup configuration error. |
| `OPENSHELL_DATABASE_IMAGE` | env var | (unset) | PostgreSQL image override. In CNPG mode: sets `spec.imageName` on the CNPG Cluster CR. In deployment mode: sets the container image for the PostgreSQL Deployment. When unset, CNPG uses its default image; deployment mode uses `postgres:18`. Not used in external mode, which runs no in-cluster PostgreSQL workload. |

> **Removed:** `CNPG_CLUSTER_NAME` and `CNPG_CLUSTER_NAMESPACE` environment variables are no longer used. The database location is derived per-gateway from the ManagedDatabase resource referenced by the gateway's `database_id`.

---

## Configuration Examples

### CNPG Mode

ManagedDatabase (created via API, reconciled by ManagedDatabaseReconciler):

```json
{
  "name": "openshell-db",
  "provider": "cnpg"
}
```

The ManagedDatabaseReconciler creates:

```yaml
# Namespace
apiVersion: v1
kind: Namespace
metadata:
  name: openshell-db-a1b2c3d4e5f67890
  labels:
    hypershell.redhat.io/managed: "true"
---
# CNPG Cluster
apiVersion: postgresql.cnpg.io/v1
kind: Cluster
metadata:
  name: openshell-db
  namespace: openshell-db-a1b2c3d4e5f67890
  labels:
    hypershell.redhat.io/managed: "true"
spec:
  instances: 1
  storage:
    size: 1Gi
  resources:
    requests:
      memory: 256Mi
    limits:
      memory: 512Mi
```

Per-gateway CNPG resources (created by GatewayReconciler in ManagedDatabase namespace):

```yaml
# Password Secret
apiVersion: v1
kind: Secret
metadata:
  name: gw-2j5k7m9pqrstvwxyz-credentials
  namespace: openshell-db-a1b2c3d4e5f67890
  labels:
    cnpg.io/reload: "true"
    hypershell.redhat.io/managed: "true"
    hypershell.redhat.io/gateway-namespace: openshell-a1b2c3d4e5f67890
type: kubernetes.io/basic-auth
stringData:
  username: gw_2j5k7m9pqrstvwxyz
  password: <32-byte-hex-random>
---
# DatabaseRole
apiVersion: postgresql.cnpg.io/v1
kind: DatabaseRole
metadata:
  name: gw-2j5k7m9pqrstvwxyz
  namespace: openshell-db-a1b2c3d4e5f67890
  labels:
    hypershell.redhat.io/managed: "true"
    hypershell.redhat.io/gateway-namespace: openshell-a1b2c3d4e5f67890
spec:
  cluster:
    name: openshell-db
  name: gw_2j5k7m9pqrstvwxyz
  login: true
  passwordSecret:
    name: gw-2j5k7m9pqrstvwxyz-credentials
  databaseRoleReclaimPolicy: delete
---
# Database
apiVersion: postgresql.cnpg.io/v1
kind: Database
metadata:
  name: gw-2j5k7m9pqrstvwxyz
  namespace: openshell-db-a1b2c3d4e5f67890
  labels:
    hypershell.redhat.io/managed: "true"
    hypershell.redhat.io/gateway-namespace: openshell-a1b2c3d4e5f67890
spec:
  cluster:
    name: openshell-db
  name: gw_2j5k7m9pqrstvwxyz
  owner: gw_2j5k7m9pqrstvwxyz
  databaseReclaimPolicy: delete
---
# Gateway credentials Secret (in tenant namespace)
apiVersion: v1
kind: Secret
metadata:
  name: openshell-gateway-db-credentials
  namespace: openshell-a1b2c3d4e5f67890
  labels:
    hypershell.redhat.io/managed: "true"
type: Opaque
stringData:
  host: openshell-db-rw.openshell-db-a1b2c3d4e5f67890.svc.cluster.local
  port: "5432"
  dbname: gw_2j5k7m9pqrstvwxyz
  user: gw_2j5k7m9pqrstvwxyz
  password: <32-byte-hex-random>
  uri: postgresql://gw_2j5k7m9pqrstvwxyz:<password>@openshell-db-rw.openshell-db-a1b2c3d4e5f67890.svc.cluster.local:5432/gw_2j5k7m9pqrstvwxyz?sslmode=require
```

### Deployment Mode

ManagedDatabase (auto-created by the API server per gateway):

```json
{
  "name": "openshell-db",
  "provider": "deployment"
}
```

The ManagedDatabaseReconciler creates (all in namespace `openshell-db-a1b2c3d4e5f67890`). The following Deployment security context is the vanilla-Kubernetes form; on OpenShift, fixed UID/GID/FSGroup fields are omitted and assigned by SCC admission:

```yaml
# Credentials Secret
apiVersion: v1
kind: Secret
metadata:
  name: openshell-db-credentials
  namespace: openshell-db-a1b2c3d4e5f67890
  labels:
    hypershell.redhat.io/managed: "true"
type: Opaque
stringData:
  user: openshell
  password: <32-byte-hex-random>
  dbname: openshell
  uri: postgresql://openshell:<password>@openshell-gateway-db.openshell-db-a1b2c3d4e5f67890.svc.cluster.local:5432/openshell?sslmode=disable
---
# PVC
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: openshell-gateway-db-data
  namespace: openshell-db-a1b2c3d4e5f67890
  labels:
    hypershell.redhat.io/managed: "true"
spec:
  accessModes: [ReadWriteOnce]
  resources:
    requests:
      storage: 1Gi
---
# Deployment
apiVersion: apps/v1
kind: Deployment
metadata:
  name: openshell-gateway-db
  namespace: openshell-db-a1b2c3d4e5f67890
  labels:
    hypershell.redhat.io/managed: "true"
    app: openshell-gateway-db
spec:
  replicas: 1
  selector:
    matchLabels:
      app: openshell-gateway-db
  template:
    spec:
      securityContext:
        runAsNonRoot: true
        runAsUser: 999
        runAsGroup: 999
        fsGroup: 999
        fsGroupChangePolicy: OnRootMismatch
        seccompProfile:
          type: RuntimeDefault
      containers:
        - name: postgres
          image: postgres:18
          env:
            - name: POSTGRES_PASSWORD
              valueFrom:
                secretKeyRef:
                  name: openshell-db-credentials
                  key: password
          volumeMounts:
            - name: data
              mountPath: /var/lib/postgresql/data
            - name: postgres-run
              mountPath: /var/run/postgresql
              subPath: postgresql
            - name: tmp
              mountPath: /tmp
          securityContext:
            allowPrivilegeEscalation: false
            readOnlyRootFilesystem: true
            runAsNonRoot: true
            runAsUser: 999
            runAsGroup: 999
            capabilities:
              drop: [ALL]
      volumes:
        - name: data
          persistentVolumeClaim:
            claimName: openshell-gateway-db-data
        - name: postgres-run
          emptyDir: {}
        - name: tmp
          emptyDir: {}
---
# Service
apiVersion: v1
kind: Service
metadata:
  name: openshell-gateway-db
  namespace: openshell-db-a1b2c3d4e5f67890
  labels:
    hypershell.redhat.io/managed: "true"
spec:
  selector:
    app: openshell-gateway-db
  ports:
    - port: 5432
      targetPort: 5432
---
# Gateway credentials Secret (copied by GatewayReconciler into tenant namespace)
apiVersion: v1
kind: Secret
metadata:
  name: openshell-gateway-db-credentials
  namespace: openshell-a1b2c3d4e5f67890
  labels:
    hypershell.redhat.io/managed: "true"
type: Opaque
stringData:
  host: openshell-gateway-db.openshell-db-a1b2c3d4e5f67890.svc.cluster.local
  port: "5432"
  dbname: openshell
  user: openshell
  password: <32-byte-hex-random>
  uri: postgresql://openshell:<password>@openshell-gateway-db.openshell-db-a1b2c3d4e5f67890.svc.cluster.local:5432/openshell?sslmode=disable
```

---

## Debugging Reference

| Symptom | Mode | Root Cause | Fix |
|---|---|---|---|
| Database CR `status.applied: false` | cnpg | CNPG operator not running or Cluster not ready | Check CNPG operator pods and Cluster status |
| DatabaseRole stuck in `Terminating` | cnpg | Role owns objects that prevent DROP | Manually drop owned objects or delete database first |
| Gateway pod cannot connect to database | all | Credentials Secret not created or wrong host | Verify `openshell-gateway-db-credentials` in tenant namespace |
| ManagedDatabase namespace not found | deployment, cnpg | ManagedDatabaseReconciler has not yet processed the resource | Check ManagedDatabase status and reconciler logs |
| Control plane exits at startup with "DATABASE_PROVIDER=cnpg requires..." | cnpg | `DATABASE_PROVIDER=cnpg` but the CNPG operator (or one of the `clusters`/`databases`/`databaseroles` resources) is not installed | Install/upgrade the CloudNativePG operator, or switch to `DATABASE_PROVIDER=deployment` (also the default when unset) |
| API server or control plane exits at startup with "invalid DATABASE_PROVIDER" | all | `DATABASE_PROVIDER` set to a value other than unset/empty, `deployment`, `cnpg`, or `external` | Set `DATABASE_PROVIDER` to `deployment`, `cnpg`, or `external`, or unset it |
| PostgreSQL Deployment not ready | deployment | Image pull failure or PVC not bound | Check Deployment events and PVC status in ManagedDatabase namespace |
| `openshell-db-credentials` Secret missing | deployment | ManagedDatabaseReconciler has not yet completed | Check ManagedDatabase status and reconciler logs |
| Password rotation not applied | cnpg | Missing `cnpg.io/reload: "true"` label on password Secret | Add the label to the Secret |

---

## Resources Removed (vs. Pre-ManagedDatabase Spec)

Before the ManagedDatabase model was introduced, each gateway namespace directly contained a standalone PostgreSQL pod. Those resources were moved to the ManagedDatabase's namespace:

| Resource | Previous Location | Current Location |
|---|---|---|
| PVC (`openshell-gateway-db-data`) | gateway tenant namespace | ManagedDatabase namespace (deployment mode) |
| Deployment (`openshell-gateway-db`) | gateway tenant namespace | ManagedDatabase namespace (deployment mode) |
| Service (`openshell-gateway-db`) | gateway tenant namespace | ManagedDatabase namespace (deployment mode) |

In CNPG mode, the Deployment/PVC/Service do not exist; CNPG manages PostgreSQL pods internally.

The `database.yaml` manifest template (per-gateway PostgreSQL resources applied directly by `deployGateway()`) is removed. In deployment mode, the ManagedDatabaseReconciler creates the equivalent resources in the ManagedDatabase namespace.

Both modes use the single `OPENSHELL_DATABASE_IMAGE` override. Deployment mode derives the required runtime UID/GID, environment variable names, data mount, and `PGDATA` path from that image: upstream and Red Hat Hardened PostgreSQL use upstream conventions, while legacy RHEL `postgresql-*` images retain their `POSTGRESQL_*` and `/var/lib/pgsql/data` conventions.

The global `CNPG_CLUSTER_NAME` and `CNPG_CLUSTER_NAMESPACE` environment variables are removed. The database location is resolved per-gateway from the ManagedDatabase resource.

---

## References

- [CloudNativePG Documentation](https://cloudnative-pg.io/docs/)
- [CNPG Database CRD](https://cloudnative-pg.io/docs/devel/declarative_database_management)
- [CNPG DatabaseRole CRD](https://cloudnative-pg.io/docs/devel/declarative_role_management)
