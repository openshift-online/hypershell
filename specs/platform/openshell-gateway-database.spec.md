# OpenShell Gateway Database Specification

**Date:** 2026-08-06
**Status:** Draft
**Parent:** `openshell-gateway.spec.md` — core gateway provisioning
**Related:** `data-model.spec.md` — ManagedDatabase kind definition; `control-plane.spec.md` — reconciler architecture

---

## Purpose

This specification defines how the ManagedDatabase Kind drives PostgreSQL provisioning for OpenShell gateways. ManagedDatabase is a first-class HyperShell resource, persisted in PostgreSQL and exposed via the REST+gRPC API. The ManagedDatabaseReconciler watches ManagedDatabase events and provisions in-cluster PostgreSQL instances into target namespaces. Gateways reference a ManagedDatabase via the `database_id` FK, and the GatewayReconciler reads the provisioned Secret to configure the gateway's database connection.

This two-resource model (ManagedDatabase + Gateway) separates database lifecycle from gateway lifecycle:
- Databases can be provisioned before gateways exist
- Multiple gateways can share a database (future)
- External database providers (AWS RDS, CloudSQL) can be added without changing Gateway resources

---

## Architecture

### Provisioning Flow

```
User creates ManagedDatabase
    │  POST /api/hypershell/v1/managed_databases
    │  { name, sector_id, provider: "in-cluster", namespace, engine: "postgres" }
    ▼
API Server (PostgreSQL)
    │  persists ManagedDatabase, status = "Pending"
    │  emits gRPC watch event
    ▼
Control Plane — ManagedDatabaseReconciler
    │  receives ManagedDatabase ADDED event
    │  validates: provider=in-cluster, engine=postgres
    │  provisions K8s resources into target namespace:
    │    Secret, PVC, Deployment, Service, NetworkPolicy
    │  updates ManagedDatabase: status="Ready", secret_name="openshell-gateway-db-credentials"
    ▼
User creates Gateway
    │  POST /api/hypershell/v1/gateways
    │  { name, sector_id, cluster_id, release_id, database_id: "<managed-db-id>", namespace }
    ▼
Control Plane — GatewayReconciler
    │  receives Gateway ADDED event
    │  looks up ManagedDatabase by database_id via gRPC
    │  reads secret_name from ManagedDatabase
    │  deploys gateway as Deployment (not StatefulSet)
    │  mounts OPENSHELL_DB_URL from the referenced Secret
    │  adds pg_isready init container
    ▼
Gateway pod connects to PostgreSQL via in-namespace Secret
```

### Relationship to Gateway

| Gateway has `database_id` | Gateway workload | Database source |
|---|---|---|
| Yes, ManagedDatabase status=Ready | Deployment | `OPENSHELL_DB_URL` from ManagedDatabase.secret_name |
| Yes, ManagedDatabase status!=Ready | Skip reconciliation, retry next cycle | — |
| No (null `database_id`) | StatefulSet with SQLite PVC | Embedded SQLite |

---

## Requirements

### Requirement: ManagedDatabase Resource Schema

ManagedDatabase SHALL be a first-class HyperShell resource kind with the following fields:

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `name` | string | Yes | — | Human-readable name |
| `sector_id` | string | Yes | — | Parent sector FK |
| `provider` | string | Yes | — | Provisioning provider: `in-cluster` (reconciler-managed) or future `aws-rds`, `gcp-cloudsql` |
| `namespace` | string | Yes (for `in-cluster`) | — | Target Kubernetes namespace for database resources |
| `region` | string | No | — | Cloud region (for external providers) |
| `engine` | string | No | `postgres` | Database engine type |
| `engine_version` | string | No | `16` | Engine version |
| `instance_class` | string | No | — | Compute instance class (for external providers) |
| `storage_size` | string | No | `5Gi` | PVC storage request (for `in-cluster`) |
| `secret_name` | string | No (set by reconciler) | — | Name of the Kubernetes Secret holding connection credentials. Populated by the ManagedDatabaseReconciler after provisioning |
| `status` | string | No (set by reconciler) | `Pending` | Provisioning status: `Pending`, `Provisioning`, `Ready`, `Failed` |

#### Scenario: Create ManagedDatabase for in-cluster PostgreSQL

- GIVEN a sector exists
- WHEN a user POSTs to `/api/hypershell/v1/managed_databases`:
  ```json
  {
    "name": "e2e-db",
    "sector_id": "sector-1",
    "provider": "in-cluster",
    "namespace": "openshell-e2e",
    "engine": "postgres",
    "engine_version": "16",
    "storage_size": "5Gi"
  }
  ```
- THEN the API server SHALL persist the ManagedDatabase with `status: Pending`
- AND the API server SHALL emit a gRPC watch event
- AND the ManagedDatabaseReconciler SHALL provision PostgreSQL resources into the `openshell-e2e` namespace

#### Scenario: ManagedDatabase with external provider (future)

- GIVEN a ManagedDatabase with `provider: aws-rds`
- WHEN the ManagedDatabaseReconciler receives the event
- THEN it SHALL log an info message and skip provisioning (not yet implemented)
- AND the status SHALL remain `Pending`

---

### Requirement: ManagedDatabase Lifecycle Phases

| Phase | Description |
|---|---|
| `Pending` | ManagedDatabase created in API, not yet picked up by reconciler |
| `Provisioning` | Reconciler is deploying K8s database resources |
| `Ready` | All database resources deployed, PostgreSQL accepting connections |
| `Failed` | Provisioning failed (see status message or reconciler logs) |

The ManagedDatabaseReconciler SHALL update the phase via the gRPC `UpdateManagedDatabase` RPC as it progresses through provisioning.

---

### Requirement: In-Cluster PostgreSQL Provisioning

When a ManagedDatabase has `provider: in-cluster` and `engine: postgres`, the ManagedDatabaseReconciler SHALL provision the following Kubernetes resources into the `namespace`:

**1. Secret** (`openshell-gateway-db-credentials`)

- `POSTGRESQL_USER` = `openshell`
- `POSTGRESQL_PASSWORD` = 32-byte cryptographically random hex string (`crypto/rand`)
- `POSTGRESQL_DATABASE` = `openshell`
- `url` = `postgresql://openshell:<password>@openshell-gateway-db:5432/openshell?sslmode=disable`
- Created with create-or-skip semantics (do NOT regenerate password on re-reconciliation)

**2. PVC** (`openshell-gateway-db-data`)

- Size: ManagedDatabase `storage_size` (default `5Gi`)
- AccessMode: `ReadWriteOnce`

**3. Deployment** (`openshell-gateway-db`)

- Image: `registry.redhat.io/rhel9/postgresql-16:latest` (Red Hat hardened)
- Replicas: 1
- Env from Secret: `POSTGRESQL_USER`, `POSTGRESQL_PASSWORD`, `POSTGRESQL_DATABASE`
- Volume mount: PVC at `/var/lib/pgsql/data`
- EmptyDir mounts: `/var/run/postgresql`, `/tmp` (PostgreSQL requires writable paths for Unix socket and temp files)
- Container port: 5432
- Readiness probe: TCP on port 5432 (initialDelaySeconds: 10, periodSeconds: 10)
- Liveness probe: TCP on port 5432 (initialDelaySeconds: 30, periodSeconds: 30)
- Resource requests: `cpu: 100m`, `memory: 256Mi`
- Resource limits: `cpu: 500m`, `memory: 512Mi`
- SecurityContext: `runAsNonRoot: true`, `allowPrivilegeEscalation: false`, capabilities `drop: [ALL]`, `readOnlyRootFilesystem: false` (postgres needs writable data dir), `seccompProfile.type: RuntimeDefault`
- Strategy: `Recreate` (single-replica; rolling update is not safe)

**4. Service** (`openshell-gateway-db`)

- Type: `ClusterIP`
- Port: 5432 → 5432

**5. NetworkPolicy** (`openshell-gateway-db`)

- PodSelector: `app.kubernetes.io/name: openshell`, `app.kubernetes.io/component: database`
- Ingress: allow TCP 5432 from pods matching `app.kubernetes.io/name: openshell`, `app.kubernetes.io/instance: openshell-gateway`
- All other ingress denied

All database resources SHALL carry labels:
- `app.kubernetes.io/name: openshell`
- `app.kubernetes.io/component: database`
- `app.kubernetes.io/managed-by: hypershell`

#### Scenario: Provision PostgreSQL into namespace

- GIVEN a ManagedDatabase with `provider: in-cluster`, `engine: postgres`, `namespace: openshell-e2e`
- WHEN the ManagedDatabaseReconciler processes the event
- THEN it SHALL ensure the namespace exists (create if missing, with `app.kubernetes.io/managed-by=hypershell` label)
- AND it SHALL provision Secret, PVC, Deployment, Service, NetworkPolicy in that namespace
- AND it SHALL update the ManagedDatabase: `status=Ready`, `secret_name=openshell-gateway-db-credentials`

#### Scenario: Namespace already exists

- GIVEN the target namespace already exists
- WHEN the ManagedDatabaseReconciler provisions database resources
- THEN it SHALL NOT modify the existing namespace
- AND it SHALL use update-or-create semantics for all database resources

#### Scenario: Idempotent re-reconciliation

- GIVEN database resources already exist in the namespace
- WHEN the ManagedDatabaseReconciler reconciles again
- THEN it SHALL apply the latest configuration using update-or-create semantics
- AND it SHALL NOT regenerate the password (Secret uses create-or-skip)
- AND the status SHALL remain `Ready`

> **Implementation note:** The default database image uses Red Hat hardened PostgreSQL `registry.redhat.io/rhel9/postgresql-16:latest` for security and enterprise support. Docker Hub images have rate limits and lack security hardening. PostgreSQL 16 provides stable, production-ready features; PostgreSQL 18 adoption will be evaluated for future updates.

> **`sslmode=disable` rationale:** The database connection is in-cluster, same-namespace traffic between the gateway pod and the database pod. TLS on this link adds latency and operational complexity with negligible security benefit given the NetworkPolicy isolation.

---

### Requirement: Database Credential Security

- Passwords SHALL be generated using `crypto/rand` (32 bytes, hex-encoded)
- Passwords SHALL NEVER appear in log messages or error strings (use `len(password)` for debugging)
- The database Secret SHALL be created with create-or-skip semantics (do NOT update password on re-reconciliation)
- The `url` key in the Secret provides the full connection string

#### Scenario: Password preservation on reconcile

- GIVEN a Secret `openshell-gateway-db-credentials` already exists
- WHEN the ManagedDatabaseReconciler reconciles
- THEN it SHALL NOT overwrite the Secret
- AND the existing password SHALL be preserved

#### Scenario: Secret deleted externally

- GIVEN the credentials Secret is deleted by an external actor
- WHEN the ManagedDatabaseReconciler reconciles
- THEN it SHALL generate a new password and recreate the Secret
- AND the gateway SHALL pick up the new credentials on its next restart

#### Manual Credential Rotation

For security incident response, database credentials can be rotated manually:
1. Delete the `openshell-gateway-db-credentials` Secret
2. The ManagedDatabaseReconciler will detect the missing Secret and regenerate credentials on next reconciliation
3. Restart the gateway Deployment to pick up new credentials
4. Brief downtime during restart (~30-60 seconds)

---

### Requirement: RHEL Image Detection

When the database image path contains `rhel`, the reconciler SHALL use `POSTGRESQL_*` env var names (`POSTGRESQL_USER`, `POSTGRESQL_PASSWORD`, `POSTGRESQL_DATABASE`). When using standard Docker Hub `postgres:*` images, it SHALL use `POSTGRES_*` env var names (without the `QL` suffix).

---

### Requirement: Gateway Integration with ManagedDatabase

The GatewayReconciler SHALL integrate with ManagedDatabase resources via the `database_id` FK on the Gateway resource.

#### Scenario: Gateway with database_id (PostgreSQL mode)

- GIVEN a Gateway with `database_id` set to a valid ManagedDatabase ID
- AND the ManagedDatabase has `status: Ready` and `secret_name: openshell-gateway-db-credentials`
- WHEN the GatewayReconciler reconciles the Gateway
- THEN it SHALL deploy the gateway as a Deployment (not StatefulSet)
- AND the gateway container SHALL receive `OPENSHELL_DB_URL` from the Secret referenced by `secret_name`:
  ```yaml
  - name: OPENSHELL_DB_URL
    valueFrom:
      secretKeyRef:
        name: openshell-gateway-db-credentials
        key: url
  ```
- AND the Deployment SHALL include an init container that waits for database readiness:
  ```yaml
  initContainers:
  - name: wait-for-db
    image: registry.redhat.io/rhel9/postgresql-16:latest
    command: ["sh", "-c", "until pg_isready -h openshell-gateway-db -p 5432; do sleep 2; done"]
  ```
- AND the Deployment SHALL NOT include a VolumeClaimTemplate for SQLite

#### Scenario: Gateway without database_id (SQLite mode)

- GIVEN a Gateway with no `database_id` (null)
- WHEN the GatewayReconciler reconciles the Gateway
- THEN it SHALL deploy the gateway as a StatefulSet with SQLite PVC (existing behavior)
- AND no database lookup SHALL occur

#### Scenario: Gateway references non-ready ManagedDatabase

- GIVEN a Gateway with `database_id` set
- AND the referenced ManagedDatabase has `status: Provisioning` (not yet Ready)
- WHEN the GatewayReconciler reconciles the Gateway
- THEN it SHALL skip provisioning and log a message: "waiting for ManagedDatabase to be Ready"
- AND it SHALL retry on the next reconciliation cycle

#### Scenario: Gateway references deleted ManagedDatabase

- GIVEN a Gateway with `database_id` set
- AND the referenced ManagedDatabase has been deleted
- WHEN the GatewayReconciler reconciles the Gateway
- THEN it SHALL log an error: "ManagedDatabase not found"
- AND it SHALL NOT provision the gateway
- AND it SHALL set the Gateway phase to `Failed`

---

### Requirement: ManagedDatabase Deletion

When a ManagedDatabase is deleted, the ManagedDatabaseReconciler SHALL clean up all provisioned Kubernetes resources.

#### Scenario: Delete ManagedDatabase

- GIVEN a ManagedDatabase with `provider: in-cluster` and `status: Ready`
- WHEN the ManagedDatabase is deleted via the API
- THEN the ManagedDatabaseReconciler SHALL delete from the target namespace:
  - Deployment `openshell-gateway-db`
  - Service `openshell-gateway-db`
  - PVC `openshell-gateway-db-data`
  - Secret `openshell-gateway-db-credentials`
  - NetworkPolicy `openshell-gateway-db`
- **WARNING**: All database contents will be permanently deleted.

#### Scenario: Delete ManagedDatabase while Gateway still references it

- GIVEN a Gateway references a ManagedDatabase via `database_id`
- WHEN the ManagedDatabase is deleted
- THEN the ManagedDatabaseReconciler SHALL still delete database resources
- AND the GatewayReconciler SHALL detect the missing ManagedDatabase on next reconcile and set Gateway phase to `Failed`

---

### Requirement: Resource Provisioning Order

The ManagedDatabaseReconciler SHALL provision resources in this order:

1. Namespace (ensure exists)
2. Secret (`openshell-gateway-db-credentials`)
3. PVC (`openshell-gateway-db-data`)
4. Deployment (`openshell-gateway-db`)
5. Service (`openshell-gateway-db`)
6. NetworkPolicy (`openshell-gateway-db`)

This order ensures the Secret exists before the Deployment references it, and the Service exists before the gateway's `pg_isready` init container queries it.

---

## Configuration Examples

### ManagedDatabase + Gateway (PostgreSQL)

```json
// Step 1: Create ManagedDatabase
POST /api/hypershell/v1/managed_databases
{
  "name": "e2e-db",
  "sector_id": "sector-1",
  "provider": "in-cluster",
  "namespace": "openshell-e2e",
  "engine": "postgres",
  "engine_version": "16",
  "storage_size": "10Gi"
}

// Response: { "id": "abc123", "status": "Pending", ... }
// Wait for status = "Ready", secret_name = "openshell-gateway-db-credentials"

// Step 2: Create Gateway referencing the database
POST /api/hypershell/v1/gateways
{
  "name": "e2e-gw",
  "sector_id": "sector-1",
  "cluster_id": "cluster-1",
  "release_id": "release-1",
  "database_id": "abc123",
  "namespace": "openshell-e2e"
}
```

### Gateway without database (SQLite, default)

```json
POST /api/hypershell/v1/gateways
{
  "name": "e2e-gw",
  "sector_id": "sector-1",
  "cluster_id": "cluster-1",
  "release_id": "release-1",
  "namespace": "openshell-e2e"
}
// database_id omitted → StatefulSet with SQLite
```

---

## Proto/OpenAPI Changes Required

### ManagedDatabase Proto

Add `namespace` and `storage_size` fields; rename `connection_secret` to `secret_name`:

```protobuf
message ManagedDatabase {
  ObjectReference metadata = 1;
  string name = 2;
  string fleet_id = 3;        // sector_id in the API
  string provider = 4;
  optional string namespace = 5;     // NEW: target namespace for in-cluster provisioning
  optional string region = 6;
  optional string engine = 7;
  optional string engine_version = 8;
  optional string instance_class = 9;
  optional string storage_size = 10; // NEW: PVC size for in-cluster postgres
  optional string secret_name = 11;  // RENAMED from connection_secret
  optional string status = 12;
}
```

### ManagedDatabase OpenAPI

```yaml
ManagedDatabase:
  properties:
    namespace:
      type: string
      description: Target Kubernetes namespace for in-cluster database resources
    storage_size:
      type: string
      description: PVC storage request (e.g., "5Gi"). Only for provider=in-cluster
      default: "5Gi"
    secret_name:
      type: string
      description: Name of the Kubernetes Secret holding connection credentials. Set by the reconciler.
      readOnly: true
```

### ManagedDatabase Model (Go)

```go
type ManagedDatabase struct {
    api.Meta
    Name          string  `json:"name"`
    FleetId       string  `json:"fleet_id"`
    Provider      string  `json:"provider"`
    Namespace     *string `json:"namespace"`
    Region        *string `json:"region"`
    Engine        *string `json:"engine"`
    EngineVersion *string `json:"engine_version"`
    InstanceClass *string `json:"instance_class"`
    StorageSize   *string `json:"storage_size"`
    SecretName    *string `json:"secret_name"`
    Status        *string `json:"status"`
}
```

---

## Debugging Reference

| Symptom | Root Cause | Fix |
|---|---|---|
| ManagedDatabase stuck in `Pending` | ManagedDatabaseReconciler not running or can't reach API | Check control plane logs, API connectivity |
| ManagedDatabase stuck in `Provisioning` | Database Deployment failing (ImagePullBackOff, insufficient RBAC) | Check reconciler logs, verify image availability and RBAC |
| Gateway skips provisioning, logs "waiting for ManagedDatabase" | ManagedDatabase not yet Ready | Wait for ManagedDatabase status=Ready, then Gateway will reconcile |
| Docker Hub `toomanyrequests` for postgres | Default image was `postgres:16` | Use `registry.redhat.io/rhel9/postgresql-16:latest` |
| Gateway pod CrashLoopBackOff with pg_isready failure | Database Deployment not running or Service not created | Check DB Deployment status, verify Service exists |
| Database password changes on re-reconciliation | Secret uses update instead of create-or-skip | Fix to create-or-skip semantics in reconciler |
| Gateway `Failed` phase after ManagedDatabase deletion | Gateway still references deleted database_id | Remove `database_id` from Gateway or create a new ManagedDatabase |

---

## References

- [OpenShell Helm Chart — `server.externalDbSecret`](https://github.com/NVIDIA/OpenShell/tree/main/deploy/helm/openshell)
- [OpenShell Kubernetes Setup — External DB](https://docs.nvidia.com/openshell/latest/kubernetes/setup)
- [HyperShell Data Model](./data-model.spec.md) — ManagedDatabase kind definition
- [HyperShell Control Plane](./control-plane.spec.md) — Reconciler architecture
