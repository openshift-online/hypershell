# OpenShell Gateway Database Specification

**Date:** 2026-08-05
**Status:** Draft
**Parent:** `openshell-gateway.spec.md` - core gateway provisioning

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
| `database.image` | string | No | `postgres:16` | PostgreSQL container image. RHEL image (`registry.redhat.io/rhel9/postgresql-16:latest`) recommended on ROSA/OpenShift. Env vars and data path adapt automatically based on image detection. |
| `database.externalSecretRef` | string | No | - | Name of Secret with `url` key. Skips DB provisioning. Reserved (Phase 2) |

> **Image policy:** The default database image is `postgres:16`. On ROSA/OpenShift, operators SHOULD override this to `registry.redhat.io/rhel9/postgresql-16:latest` (pre-authenticated via the cluster pull secret, avoids Docker Hub rate limits). The reconciler dynamically adapts env vars and data paths based on image detection (see below).

---

### Requirement: PostgreSQL Image Detection

The GatewayReconciler SHALL dynamically detect the PostgreSQL image variant and adapt environment variable names, secret keys, and data mount paths accordingly. Detection uses a simple heuristic: if the resolved image string contains `"rhel"`, RHEL conventions apply; otherwise upstream conventions apply.

| Convention | RHEL image (`registry.redhat.io/rhel9/postgresql-16`) | Upstream image (`postgres:16`) |
|---|---|---|
| User env var / secret key | `POSTGRESQL_USER` | `POSTGRES_USER` |
| Password env var / secret key | `POSTGRESQL_PASSWORD` | `POSTGRES_PASSWORD` |
| Database env var / secret key | `POSTGRESQL_DATABASE` | `POSTGRES_DB` |
| Data mount path | `/var/lib/pgsql/data` | `/var/lib/postgresql/data` |
| `PGDATA` env var | Not set (RHEL image handles subdirectory internally) | `<mount_path>/pgdata` (required: upstream `postgres` refuses to init in a directory containing `lost+found`) |

This detection SHALL be applied in both the credential Secret provisioning and the Deployment manifest construction, ensuring the env vars in the Deployment match the keys in the Secret and the data volume mount matches the image's expected data directory.

For upstream images, the reconciler SHALL inject a `PGDATA` environment variable set to `<mount_path>/pgdata` to avoid the `initdb: error: directory exists but is not empty` failure caused by the `lost+found` directory present on ext4-formatted PVC mount points.

> **Reference implementation:** The upstream OpenShell control plane uses this same `strings.Contains(pgImage, "rhel")` heuristic for image variant detection.

---

### Requirement: PostgreSQL Resource Provisioning

The GatewayReconciler SHALL provision:

1. **Secret** (`openshell-gateway-db-credentials`)
   - User key (detected) = `openshell`
   - Password key (detected) = 32-byte cryptographically random hex string (`crypto/rand`)
   - Database key (detected) = `openshell`
   - `url` = `postgresql://openshell:<password>@openshell-gateway-db:5432/openshell?sslmode=disable`
   - Created ONCE (create-or-skip for Secret to avoid password churn)

2. **PVC** (`openshell-gateway-db-data`)
   - Size: `database.storageSize` (default `5Gi`)
   - AccessMode: `ReadWriteOnce`

3. **Deployment** (`openshell-gateway-db`)
   - Image: `database.image` (default `postgres:16`)
   - Env from Secret: user, password, and database keys (detected per image variant)
   - Volume mount: PVC at detected data path
   - SecurityContext: `runAsNonRoot: true`, `allowPrivilegeEscalation: false`, capabilities `drop: [ALL]`, `seccompProfile.type: RuntimeDefault`, `readOnlyRootFilesystem: false` (postgres needs writable data dir)
   - Readiness probe: TCP on port 5432

4. **Service** (`openshell-gateway-db`)
   - ClusterIP, port 5432 → 5432

5. **NetworkPolicy** (`openshell-gateway-db`)
   - Allow ingress only from gateway pods (same labels)

All database resources SHALL carry the label `hypershell.redhat.io/managed: "true"`. On gateway deletion, the control plane SHALL explicitly delete all labelled resources from the tenant namespace (label-based cleanup). ownerReferences are not used because database resources are created before the gateway Deployment exists.

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

The full rotation design (procedure, failure handling, config-hash coverage) is specified in [`openshell-gateway-secret-rotation.spec.md`](./openshell-gateway-secret-rotation.spec.md).

---

### Requirement: Gateway Workload Type

The gateway workload SHALL always be deployed as a Deployment (not a StatefulSet). The database has its own PVC; the gateway workload does not require persistent local storage.

#### Scenario: Gateway uses Deployment

- GIVEN a Gateway resource
- WHEN the GatewayReconciler reconciles
- THEN it SHALL create a Deployment (not StatefulSet) for the gateway
- AND the gateway container's `--db-url` argument SHALL reference the credentials Secret via env var

---

### Requirement: Database Resource Provisioning Order

Database resources (Secret, PVC, Deployment, Service, NetworkPolicy) SHALL be applied BEFORE the gateway workload. After reconciling `database.yaml`, the control plane SHALL wait for the `openshell-gateway-db` Deployment to reach ready state (2-minute timeout) before proceeding to deploy the gateway.

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
- THEN the control plane SHALL delete all resources with label `hypershell.redhat.io/managed: "true"` from the tenant namespace (explicit label-based cleanup)
- AND the database PVC SHALL be deleted with the rest of the labelled resources

---

## Configuration Examples

### Gateway with PostgreSQL (RHEL image)

```yaml
kind: Gateway
name: openshell-gateway
project: tenant-a
image: ghcr.io/nvidia/openshell/gateway:0.0.106
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
image: ghcr.io/nvidia/openshell/gateway:0.0.106
serverDnsNames:
  - openshell-gateway.tenant-a.svc.cluster.local
```

---

## Debugging Reference

| Symptom | Root Cause | Fix |
|---|---|---|
| `hsctl apply` missing database resources | `kustomize.Resource` missing `Database` field | Ensure SDK `Resource` struct includes `Database map[string]any` |
| Docker Hub `toomanyrequests` for postgres | Default image is `postgres:16` | Override `database.image` to `registry.redhat.io/rhel9/postgresql-16:latest` |
| Gateway pod not created after provisioning | `waitForDeploymentReady` timed out for DB | Check DB Deployment events and pod logs |
| Database password changes on re-reconciliation | Secret uses update instead of create-or-skip | Fix to create-or-skip semantics |

---

## References

- [OpenShell Helm Chart - `server.externalDbSecret`](https://github.com/NVIDIA/OpenShell/tree/main/deploy/helm/openshell)
- [OpenShell Kubernetes Setup - External DB](https://docs.nvidia.com/openshell/latest/kubernetes/setup)
