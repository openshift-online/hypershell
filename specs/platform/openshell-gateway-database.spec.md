# OpenShell Gateway Database Specification

**Date:** 2026-08-05
**Status:** Draft
**Parent:** `openshell-gateway.spec.md` — core gateway provisioning

---

## Purpose

This specification defines PostgreSQL database provisioning for OpenShell gateways. The GatewayReconciler deploys a PostgreSQL instance in the tenant namespace and uses a Deployment for the gateway workload. PostgreSQL is the only supported database backend for HyperShell gateways.

---

## Requirements

### Requirement: Gateway Database Configuration Field

The Gateway API resource SHALL include a `database` object. PostgreSQL is the sole supported backend.

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `database.storageSize` | string | No | `5Gi` | PVC size for PostgreSQL data |
| `database.image` | string | No | `registry.redhat.io/rhel9/postgresql-16:latest` | PostgreSQL container image (Red Hat hardened) |
| `database.externalSecretRef` | string | No | — | Name of Secret with `url` key. Skips DB provisioning. Reserved (Phase 2) |

> **Image policy:** HyperShell uses Red Hat hardened PostgreSQL images from `registry.redhat.io`. Docker Hub images are rate-limited on ROSA/OpenShift. The RHEL image is pre-authenticated via the cluster's pull secret and matches the API server's database deployment pattern. PostgreSQL 16 is the current stable choice; PostgreSQL 18 will be evaluated as the ecosystem matures.

---

### Requirement: PostgreSQL Resource Provisioning

The GatewayReconciler SHALL provision:

1. **Secret** (`openshell-gateway-db-credentials`)
   - `POSTGRESQL_USER` = `openshell`
   - `POSTGRESQL_PASSWORD` = 32-byte cryptographically random hex string (`crypto/rand`)
   - `POSTGRESQL_DATABASE` = `openshell`
   - `url` = `postgresql://openshell:<password>@openshell-gateway-db:5432/openshell?sslmode=disable`
   - Created ONCE (create-or-skip for Secret to avoid password churn)

2. **PVC** (`openshell-gateway-db-data`)
   - Size: `database.storageSize` (default `5Gi`)
   - AccessMode: `ReadWriteOnce`

3. **Deployment** (`openshell-gateway-db`)
   - Image: `database.image` (default RHEL postgresql-16)
   - Env from Secret: `POSTGRESQL_USER`, `POSTGRESQL_PASSWORD`, `POSTGRESQL_DATABASE`
   - Volume mount: PVC at `/var/lib/pgsql/data`
   - SecurityContext: `runAsNonRoot`, drop ALL capabilities, `readOnlyRootFilesystem: false` (postgres needs writable data dir)
   - Readiness probe: TCP on port 5432

4. **Service** (`openshell-gateway-db`)
   - ClusterIP, port 5432 → 5432

5. **NetworkPolicy** (`openshell-gateway-db`)
   - Allow ingress only from gateway pods (same labels)

All database resources SHALL carry ownerReferences to the gateway workload for cascading deletion.

---

### Requirement: Database Credential Security

- Passwords SHALL be generated using `crypto/rand` (32 bytes, hex-encoded)
- Passwords SHALL NEVER appear in log messages or error strings (use `len(password)` for debugging)
- The database Secret SHALL be created with create-or-skip semantics (do NOT update password on re-reconciliation)
- The `url` key in the Secret provides the full connection string for the gateway's `--db-url` argument

---

### Requirement: Manual Credential Rotation

The GatewayReconciler SHALL support manual database credential rotation for security incidents (e.g., accidental credential exposure).

- To trigger rotation, an operator adds the annotation `hypershell.redhat.io/rotate-db-credentials: "<timestamp>"` to the Gateway resource
- When the reconciler detects a new value for this annotation (different from the last-observed value stored in the ConfigMap), it SHALL:
  1. Generate a new password using `crypto/rand`
  2. Update the database Secret with the new credentials
  3. Execute `ALTER USER` on the PostgreSQL instance to apply the new password
  4. Restart the gateway Deployment to pick up new credentials
- Expected downtime: ~30-60 seconds during gateway restart (acceptable for incident response)
- Automatic rotation is not supported initially; it can be added based on operational requirements

---

### Requirement: Gateway Workload Type

The gateway workload SHALL always be deployed as a Deployment (not a StatefulSet). The database has its own PVC; the gateway workload does not require persistent local storage.

#### Scenario: Gateway uses Deployment

- GIVEN a Gateway resource
- WHEN the GatewayReconciler reconciles
- THEN it SHALL create a Deployment (not StatefulSet) for the gateway
- AND the gateway container's `--db-url` argument SHALL reference the credentials Secret via env var
- AND an init container SHALL run `pg_isready` to wait for the database before starting the gateway

---

### Requirement: Database Resource Provisioning Order

Database resources (Secret, PVC, Deployment, Service, NetworkPolicy) SHALL be applied BEFORE the gateway workload. The gateway's init container (`pg_isready`) depends on the database Service being available.

---

### Requirement: Database Field Immutability

The `database` configuration on a Gateway resource SHALL be immutable after initial provisioning. Once a gateway is created with a database configuration, the database fields SHALL NOT be changed.

#### Scenario: Attempt to change database configuration

- GIVEN an existing gateway with a provisioned database
- WHEN a user attempts to modify the `database` field
- THEN the API server SHALL reject the update with a validation error: "database configuration is immutable after provisioning"

---

### Requirement: Gateway Deletion Protection

A Gateway SHALL NOT be deleted if active sandboxes exist. This prevents orphaned sandbox resources (CRs, pods, PVCs).

#### Scenario: Attempt to delete gateway with active sandboxes

- GIVEN a Gateway with one or more active sandboxes
- WHEN a user attempts to delete the Gateway
- THEN the API server SHALL reject the deletion with an error: "gateway cannot be deleted while sandboxes exist; delete all sandboxes first"

#### Scenario: Delete gateway with no sandboxes

- GIVEN a Gateway with no active sandboxes
- WHEN a user deletes the Gateway
- THEN all database resources SHALL be cleaned up via ownerReferences (cascading deletion)
- AND the database PVC SHALL be deleted with the rest of the resources

---

## Configuration Examples

### Gateway with PostgreSQL (RHEL image)

```yaml
kind: Gateway
name: openshell-gateway
project: tenant-a
image: ghcr.io/nvidia/openshell/gateway:0.0.88
serverDnsNames:
  - openshell-gateway.tenant-a.svc.cluster.local
database:
  storageSize: 10Gi
  image: registry.redhat.io/rhel9/postgresql-16:latest
```

### Gateway with default database settings

```yaml
kind: Gateway
name: openshell-gateway
project: tenant-a
image: ghcr.io/nvidia/openshell/gateway:0.0.88
serverDnsNames:
  - openshell-gateway.tenant-a.svc.cluster.local
```

---

## Debugging Reference

| Symptom | Root Cause | Fix |
|---|---|---|
| `hsctl apply` missing database resources | `kustomize.Resource` missing `Database` field | Ensure SDK `Resource` struct includes `Database map[string]any` |
| Docker Hub `toomanyrequests` for postgres | Default image was `postgres:16` | Use `registry.redhat.io/rhel9/postgresql-16:latest` |
| Gateway pod CrashLoopBackOff after provisioning | init container `pg_isready` fails | Check DB Deployment is Running, Service exists |
| Database password changes on re-reconciliation | Secret uses update instead of create-or-skip | Fix to create-or-skip semantics |

---

## References

- [OpenShell Helm Chart — `server.externalDbSecret`](https://github.com/NVIDIA/OpenShell/tree/main/deploy/helm/openshell)
- [OpenShell Kubernetes Setup — External DB](https://docs.nvidia.com/openshell/latest/kubernetes/setup)
