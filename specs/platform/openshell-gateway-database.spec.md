# OpenShell Gateway Database Specification

**Date:** 2026-08-19
**Status:** Draft
**Parent:** `openshell-gateway.spec.md` - core gateway provisioning

---

## Purpose

This specification defines PostgreSQL database provisioning for OpenShell gateways using the [CloudNativePG](https://cloudnative-pg.io/) (CNPG) operator. Each ManagedDatabase resource (provider=cnpg) provisions its own CNPG `Cluster` in a dedicated namespace. The GatewayReconciler then provisions individual databases and roles within the ManagedDatabase's CNPG Cluster using CNPG custom resources (`Database` and `DatabaseRole` CRDs). Different gateways can use different ManagedDatabases, enabling per-tenant or per-environment database isolation.

PostgreSQL is the only supported database backend for HyperShell gateways.

---

## Prerequisites

### CNPG Operator

The CNPG operator SHALL be installed on every managed cluster as a cluster-level prerequisite, similar to cert-manager. The operator provides the `Cluster`, `Database`, and `DatabaseRole` CRDs.

- In the Kind development environment, `make kind-up` installs the operator (see `local-development.spec.md`)
- In production environments, the platform administrator installs the operator before registering the cluster with HyperShell

The control plane SHALL detect the CNPG operator presence by checking for the `postgresql.cnpg.io/v1` API group, following the same pattern used for cert-manager and Gateway API detection.

---

## Architecture

### ManagedDatabase as CNPG Provider

Each ManagedDatabase resource with `provider: "cnpg"` represents a CNPG `Cluster` that one or more gateways can share. The ManagedDatabaseReconciler creates the infrastructure; the GatewayReconciler creates per-gateway logical databases within it.

```
ManagedDatabase (provider=cnpg, name="openshell-db")
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

### ManagedDatabase Namespace Naming

The ManagedDatabase namespace is derived from the ManagedDatabase's KSUID using the same pattern as gateway namespaces: `openshell-db-<hex16>`, where `<hex16>` is the lowercase hexadecimal encoding of 8 bytes from the KSUID's random payload. The API server assigns this namespace in `BeforeCreate`. Example: `openshell-db-a1b2c3d4e5f67890`.

### Automatic Fleet and Database Assignment

The API server resolves missing relationship fields at gateway creation time, following a resolution chain: `cluster_id` → `fleet_id` → `database_id`.

**Fleet resolution:** When a Gateway is created with a blank `fleet_id` but a non-empty `cluster_id`, the API server looks up the ManagedCluster and copies its `fleet_id` to the gateway.

**Database resolution:** When a Gateway is created with a blank `database_id`:
1. If `fleet_id` is present (explicitly or resolved from `cluster_id`), the API server queries all ManagedDatabases in that fleet. If there is exactly one, its ID is auto-assigned. If there are zero or more than one, the request is rejected.
2. If `fleet_id` is also blank (hub cluster deployment with no fleet context), the API server queries all ManagedDatabases globally. If there is exactly one, its ID and `fleet_id` are both auto-assigned. If there are zero or more than one, the request is rejected.

This means a client only needs to provide `name` to create a gateway on the hub cluster (when a single ManagedDatabase exists), or `name` and `cluster_id` to target a specific cluster. The API server derives the rest.

The admin workflow is:
1. Create a Fleet
2. Create a ManagedDatabase (provider=cnpg) in that fleet
3. Create Gateways with just `name` - `fleet_id`, `cluster_id`, and `database_id` are auto-assigned

In the Kind development environment, `make kind-up` seeds a single `openshell-db` ManagedDatabase per fleet.

---

## Requirements

### Requirement: ManagedDatabase Reconciliation (provider=cnpg)

The ManagedDatabaseReconciler SHALL provision CNPG infrastructure for each ManagedDatabase with `provider: "cnpg"`.

For each such ManagedDatabase, the reconciler SHALL:

1. **Create the namespace** derived from the ManagedDatabase ID (`openshell-db-<hex16>`), if it does not exist. The namespace SHALL carry `app.kubernetes.io/managed-by=hypershell-control-plane`, `hypershell.redhat.io/managed=true`, and `hypershell.redhat.io/managed-database-id=<immutable-managed-database-id>`.
2. **Create a CNPG Cluster CR** in that namespace with the same three ownership labels:
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
- THEN it SHALL create the namespace `openshell-db-<hex16>` with its control-plane and immutable ManagedDatabase ownership labels
- AND it SHALL create a CNPG Cluster CR in that namespace with the same ownership labels
- AND it SHALL wait for the Cluster to reach Ready
- AND it SHALL update the ManagedDatabase status

#### Scenario: ManagedDatabase already exists (idempotent)

- GIVEN a ManagedDatabase whose CNPG Cluster is already running
- WHEN the ManagedDatabaseReconciler re-processes the event
- THEN it SHALL verify the namespace and Cluster exist
- AND it SHALL patch any missing control-plane and immutable ManagedDatabase ownership labels onto both resources
- AND it SHALL NOT recreate them

#### Scenario: Existing ManagedDatabase receives cleanup ownership labels

- GIVEN a CNPG ManagedDatabase namespace and Cluster created before the immutable ManagedDatabase ownership label was introduced
- AND both resources carry the historical `hypershell.redhat.io/managed=true` label
- AND the namespace exactly matches the API-assigned ManagedDatabase namespace and contains the expected `openshell-db` CNPG Cluster
- WHEN the ManagedDatabaseReconciler next reconciles that ManagedDatabase
- THEN it SHALL add `app.kubernetes.io/managed-by=hypershell-control-plane`, `hypershell.redhat.io/managed=true`, and `hypershell.redhat.io/managed-database-id=<immutable-managed-database-id>`
- AND it SHALL preserve the healthy CNPG Cluster configuration and status

#### Scenario: Unmanaged matching namespace is not adopted

- GIVEN a namespace matches a ManagedDatabase namespace name
- BUT the namespace or expected CNPG Cluster lacks the historical `hypershell.redhat.io/managed=true` label
- WHEN the ManagedDatabaseReconciler reconciles the ManagedDatabase
- THEN it SHALL not add ownership labels to the existing namespace or Cluster
- AND it SHALL report the ownership conflict rather than adopting or deleting the unmanaged resources

#### Scenario: Non-CNPG provider

- GIVEN a ManagedDatabase with `provider` other than `"cnpg"`
- WHEN the ManagedDatabaseReconciler processes the event
- THEN it SHALL log a warning and take no action

---

### Requirement: ManagedDatabase Deletion Protection

A ManagedDatabase SHALL NOT be deleted if any Gateway references it via `database_id`. This prevents orphaned gateway databases.

#### Scenario: Attempt to delete ManagedDatabase with referencing gateways

- GIVEN a ManagedDatabase referenced by one or more Gateways
- WHEN a user attempts to delete the ManagedDatabase
- THEN the API server SHALL reject the deletion with HTTP 409: "managed database cannot be deleted while gateways reference it; reassign or delete all referencing gateways first"

#### Scenario: Delete ManagedDatabase with no referencing gateways

- GIVEN a ManagedDatabase with no Gateway references
- WHEN a user deletes the ManagedDatabase
- THEN the ManagedDatabaseReconciler SHALL delete the CNPG Cluster CR
- AND delete the namespace `openshell-db-<hex16>`

---

### Requirement: Gateway Database Resolution

The GatewayReconciler SHALL resolve the gateway's `database_id` to a ManagedDatabase resource to determine the CNPG Cluster location. The global `CNPG_CLUSTER_NAME`/`CNPG_CLUSTER_NAMESPACE` environment variables are removed; each gateway's CNPG target is derived from its ManagedDatabase.

#### Resolution flow:

1. Read the gateway's `database_id` from the gRPC watch event
2. Look up the ManagedDatabase resource via the API server (gRPC or cache)
3. Extract `namespace` (the ManagedDatabase's auto-assigned namespace) and `name` (the CNPG Cluster name)
4. Use these as the CNPG Cluster reference for per-gateway Database/DatabaseRole provisioning

#### Scenario: Gateway with valid database_id

- GIVEN a Gateway with a `database_id` pointing to a ManagedDatabase (provider=cnpg)
- WHEN the GatewayReconciler processes the event
- THEN it SHALL resolve the ManagedDatabase's namespace and cluster name
- AND proceed with per-gateway CNPG resource provisioning in that namespace

#### Scenario: Gateway created with cluster_id only (fleet and database auto-resolved)

- GIVEN a Gateway with a non-empty `cluster_id`, blank `fleet_id`, and blank `database_id`
- WHEN the API server processes the creation request
- THEN the API server SHALL look up the ManagedCluster by `cluster_id`
- AND assign the cluster's `fleet_id` to the gateway
- AND query all ManagedDatabases in the resolved fleet
- AND if exactly one exists, assign its ID as the gateway's `database_id`
- AND if zero or more than one exist, reject the creation with an error

#### Scenario: Gateway with missing database_id and valid fleet_id

- GIVEN a Gateway with a blank `database_id` and a non-empty `fleet_id`
- WHEN the API server processes the creation request
- THEN the API server SHALL query all ManagedDatabases in the fleet
- AND if exactly one exists, assign its ID as the gateway's `database_id`
- AND if zero or more than one exist, reject the creation with an error: "database_id is required: fleet has zero or multiple ManagedDatabases"

#### Scenario: Gateway with all relationship fields blank (global fallback)

- GIVEN a Gateway with a blank `database_id`, blank `fleet_id`, and blank `cluster_id`
- WHEN the API server processes the creation request
- THEN the API server SHALL query all ManagedDatabases globally
- AND if exactly one exists, assign its ID as the gateway's `database_id` and its `fleet_id` as the gateway's `fleet_id`
- AND if zero or more than one exist, reject the creation with an error: "database_id is required: zero or multiple ManagedDatabases exist"

#### Scenario: Gateway references non-CNPG ManagedDatabase

- GIVEN a Gateway with a `database_id` pointing to a ManagedDatabase with `provider` other than `"cnpg"`
- WHEN the GatewayReconciler processes the event
- THEN it SHALL fail with an explicit error: "gateway database_id references a non-CNPG managed database; only provider=cnpg is supported"

---

### Requirement: Per-Gateway Database Provisioning

The GatewayReconciler SHALL provision a dedicated PostgreSQL database and role for each gateway using CNPG custom resources. All CNPG resources (DatabaseRole, Database, and the password Secret) SHALL be created in the ManagedDatabase's namespace (resolved via `database_id`), since CNPG requires the CRs and their referenced Cluster to reside in the same namespace.

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

---

### Requirement: Gateway Credentials Secret

After creating the CNPG resources, the reconciler SHALL create a credentials Secret in the **gateway's tenant namespace** (not the ManagedDatabase namespace) that the gateway workload consumes.

**Secret** (`openshell-gateway-db-credentials`) in the tenant namespace:
- `host` = `<cnpg-cluster-name>-rw.<managed-db-namespace>.svc.cluster.local`
- `port` = `5432`
- `dbname` = `gw_<gatewayID>`
- `user` = `gw_<gatewayID>`
- `password` = the generated password (from the basic-auth Secret)
- `uri` = `postgresql://gw_<gatewayID>:<password>@<host>:5432/gw_<gatewayID>?sslmode=require`
- Labels: `hypershell.redhat.io/managed: "true"`, standard app labels

The `uri` key provides the full connection string for the gateway's `--db-url` argument. The gateway Deployment SHALL reference this Secret via environment variable.

> **TLS:** CNPG clusters enable TLS by default. The connection string uses `sslmode=require` (encrypted, no server cert verification). Upgrading to `sslmode=verify-ca` (with CNPG's CA certificate mounted into the gateway pod) is a future hardening step.

---

### Requirement: Database Provisioning Readiness

The GatewayReconciler SHALL wait for the CNPG `Database` CR to reach ready state (`status.applied: true`) before proceeding to deploy the gateway workload. The timeout SHALL be 2 minutes.

#### Scenario: Database provisioning completes successfully

- GIVEN a new Gateway resource
- WHEN the GatewayReconciler creates the DatabaseRole and Database CRs
- AND the CNPG operator reconciles them (`status.applied: true`)
- THEN the reconciler SHALL proceed to deploy the gateway workload

#### Scenario: Database provisioning times out

- GIVEN a new Gateway resource
- WHEN the CNPG Database CR does not reach `status.applied: true` within 2 minutes
- THEN the reconciler SHALL return an error
- AND the Gateway phase SHALL remain `Provisioning`
- AND the next reconciliation SHALL retry

---

### Requirement: Database Credential Security

- Passwords SHALL be generated using `crypto/rand` (32 bytes, hex-encoded)
- Passwords SHALL NEVER appear in log messages or error strings
- The password Secret SHALL be created with create-or-skip semantics (do NOT update password on re-reconciliation)
- The `cnpg.io/reload: "true"` label ensures CNPG picks up password changes immediately when rotation occurs

---

### Requirement: Manual Credential Rotation

The GatewayReconciler SHALL support manual database credential rotation for security incidents.

- To trigger rotation, an operator adds the annotation `hypershell.redhat.io/rotate-db-credentials: "<timestamp>"` to the Gateway resource
- When the reconciler detects a new value for this annotation (different from the last-observed value stored on the gateway credentials Secret), it SHALL:
  1. Generate a new password using `crypto/rand`
  2. Update the password Secret (`gw-<gatewayID>-credentials`) in the ManagedDatabase namespace with the new password
  3. The CNPG operator applies the password change to PostgreSQL automatically (via the `cnpg.io/reload` label)
  4. Update the gateway credentials Secret (`openshell-gateway-db-credentials`) in the tenant namespace with the new password and connection URI
  5. Set annotation `hypershell.redhat.io/last-db-rotation` on the gateway credentials Secret to match the trigger annotation
  6. The config-hash annotation on the Deployment changes, triggering a rolling restart

> **Compared to the previous rotation model:** The reconciler no longer executes `ALTER ROLE` SQL directly. Instead, it updates the Kubernetes password Secret and CNPG handles the PostgreSQL role update. This is safer because the CNPG operator executes the password change within a transaction that suppresses logging.

The full rotation design (procedure, failure handling, config-hash coverage) is specified in [`openshell-gateway-secret-rotation.spec.md`](./openshell-gateway-secret-rotation.spec.md).

---

### Requirement: Gateway Workload Type

The gateway workload SHALL always be deployed as a Deployment (not a StatefulSet). The database is managed by the ManagedDatabase's CNPG Cluster; the gateway workload does not require persistent local storage.

---

### Requirement: Database Resource Provisioning Order

CNPG resources (password Secret, DatabaseRole, Database) SHALL be created BEFORE the gateway workload. After creating the Database CR, the control plane SHALL wait for `status.applied: true` (2-minute timeout) before proceeding to deploy the gateway.

---

### Requirement: Gateway Deletion Cleanup

When a Gateway is deleted, the control plane SHALL delete the CNPG resources in the ManagedDatabase's namespace:

1. Delete the `Database` CR (`gw-<gatewayID>`) -- CNPG drops the PostgreSQL database (`databaseReclaimPolicy: delete`)
2. Delete the `DatabaseRole` CR (`gw-<gatewayID>`) -- CNPG drops the PostgreSQL role (`databaseRoleReclaimPolicy: delete`)
3. Delete the password Secret (`gw-<gatewayID>-credentials`) in the ManagedDatabase namespace

Resources in the gateway's tenant namespace (including `openshell-gateway-db-credentials`) are cleaned up by the existing label-based cleanup (`hypershell.redhat.io/managed: "true"`).

CNPG resources in the ManagedDatabase namespace are identified for cleanup using the label `hypershell.redhat.io/gateway-namespace: "<tenant-namespace>"`.

#### Scenario: Delete gateway with no sandboxes

- GIVEN a Gateway with no active sandboxes
- WHEN a user deletes the Gateway
- THEN the control plane SHALL delete CNPG resources (Database, DatabaseRole, password Secret) from the ManagedDatabase namespace
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
gateway's resources. Deleting the gateway namespace cascades removal of its
in-namespace sandbox resources (pods, PVCs) along with the gateway's own
workloads, so a deletion cannot leave those orphaned.

#### Scenario: Delete gateway that has active sandboxes

- GIVEN a Gateway with one or more active sandboxes
- WHEN a user deletes the Gateway (having been warned of the active sandbox count)
- THEN the API server SHALL accept the deletion and SHALL NOT reject it on account
  of the active sandboxes
- AND the control plane SHALL reclaim the gateway's namespace, disrupting those
  sandboxes and cascading removal of their in-namespace resources

#### Scenario: Delete gateway cleans up managed resources

- GIVEN a Gateway being deleted, with any number of active sandboxes (including none)
- WHEN a user deletes the Gateway
- THEN the control plane SHALL delete all resources with label `hypershell.redhat.io/managed: "true"` from the tenant namespace (explicit label-based cleanup)
- AND the database PVC SHALL be deleted with the rest of the labelled resources

---

### Requirement: CNPG Operator Detection

The control plane SHALL detect the CNPG operator at startup by checking for the `postgresql.cnpg.io/v1` API group in the cluster's discovery information. If the CNPG operator is not available, gateway provisioning SHALL fail with an explicit error: "CNPG operator is required but not available on the cluster."

---

## Configuration Reference

| Variable | Type | Default | Description |
|---|---|---|---|
| `OPENSHELL_DATABASE_IMAGE` | env var | (unset - CNPG default) | PostgreSQL image for ManagedDatabase CNPG Clusters; when set, the ManagedDatabaseReconciler adds `spec.imageName` to the CNPG Cluster CR |

> **Removed:** `CNPG_CLUSTER_NAME` and `CNPG_CLUSTER_NAMESPACE` environment variables are no longer used. The CNPG Cluster location is derived from the ManagedDatabase resource referenced by the gateway's `database_id`.

---

## Configuration Examples

### ManagedDatabase (created via API, reconciled by ManagedDatabaseReconciler)

```json
{
  "name": "openshell-db",
  "fleet_id": "<fleet-id>",
  "provider": "cnpg"
}
```

The ManagedDatabaseReconciler creates the following infrastructure:

```yaml
# Namespace (created by ManagedDatabaseReconciler)
apiVersion: v1
kind: Namespace
metadata:
  name: openshell-db-a1b2c3d4e5f67890
  labels:
    app.kubernetes.io/managed-by: hypershell-control-plane
    hypershell.redhat.io/managed: "true"
    hypershell.redhat.io/managed-database-id: <immutable-managed-database-id>
---
# CNPG Cluster (created by ManagedDatabaseReconciler)
apiVersion: postgresql.cnpg.io/v1
kind: Cluster
metadata:
  name: openshell-db
  namespace: openshell-db-a1b2c3d4e5f67890
  labels:
    app.kubernetes.io/managed-by: hypershell-control-plane
    hypershell.redhat.io/managed: "true"
    hypershell.redhat.io/managed-database-id: <immutable-managed-database-id>
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

### Per-Gateway CNPG Resources (created by GatewayReconciler)

```yaml
# Password Secret (in ManagedDatabase namespace)
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
# DatabaseRole (in ManagedDatabase namespace)
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
# Database (in ManagedDatabase namespace)
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

---

## Debugging Reference

| Symptom | Root Cause | Fix |
|---|---|---|
| Database CR `status.applied: false` | CNPG operator not running or Cluster not ready | Check CNPG operator pods and Cluster status |
| DatabaseRole stuck in `Terminating` | Role owns objects that prevent DROP | Manually drop owned objects or delete database first |
| Gateway pod cannot connect to database | Credentials Secret not created or wrong host | Verify `openshell-gateway-db-credentials` in tenant namespace |
| ManagedDatabase namespace not found | ManagedDatabaseReconciler has not yet processed the resource | Check ManagedDatabase status and reconciler logs |
| Gateway fails with "non-CNPG managed database" | Gateway's `database_id` points to a ManagedDatabase with unsupported provider | Change the ManagedDatabase provider to `cnpg` or reassign the gateway |
| Password rotation not applied | Missing `cnpg.io/reload: "true"` label on password Secret | Add the label to the Secret |

---

## Resources Removed (vs. Previous Spec)

The following resources are NO LONGER created by the GatewayReconciler, as they are replaced by CNPG:

| Resource | Previous Name | Reason Removed |
|---|---|---|
| PVC | `openshell-gateway-db-data` | CNPG manages its own storage |
| Deployment | `openshell-gateway-db` | CNPG manages PostgreSQL pods |
| Service | `openshell-gateway-db` | CNPG creates `<cluster>-rw` service |
| NetworkPolicy | `openshell-gateway-db` | CNPG manages network access to its pods |

The `database.yaml` manifest template is removed from the control plane manifests directory.

The RHEL/upstream PostgreSQL image detection logic (`isRHELPostgres`, `postgresEnvKeys`, `postgresDataPath`, `postgresPGDataPath`) is removed. CNPG manages the PostgreSQL image and configuration.

The global `CNPG_CLUSTER_NAME` and `CNPG_CLUSTER_NAMESPACE` environment variables are removed. The CNPG Cluster location is resolved per-gateway from the ManagedDatabase resource.

The static `deploy/kind/cnpg-cluster.yaml` is removed. The ManagedDatabaseReconciler creates CNPG Clusters dynamically.

---

## References

- [CloudNativePG Documentation](https://cloudnative-pg.io/docs/)
- [CNPG Database CRD](https://cloudnative-pg.io/docs/devel/declarative_database_management)
- [CNPG DatabaseRole CRD](https://cloudnative-pg.io/docs/devel/declarative_role_management)
