# OpenShell Gateway Database Specification - External Provider

**Date:** 2026-09-04
**Status:** Draft
**Parent:** [`openshell-gateway-database.spec.md`](./openshell-gateway-database.spec.md) - gateway database provisioning (deployment/cnpg modes)

---

## Purpose

This specification defines a third PostgreSQL provisioning mode for OpenShell
gateways: **`external`**. It extends the two modes in the parent spec
(`deployment`, `cnpg`) and shares that spec's abstractions (`ManagedDatabase`,
`database_id`, the tenant-namespace `openshell-gateway-db-credentials` Secret, the
gateway `--db-url` contract).

In `external` mode, PostgreSQL server infrastructure is **provisioned outside
HyperShell** - a cloud-managed database (AWS RDS / Aurora for PostgreSQL, IBM
Cloud Databases for PostgreSQL) created out-of-band by a platform team or IaC.
HyperShell does **not** create, resize, or delete the server. HyperShell
**registers** the pre-existing endpoint and, within it, provisions **one
dedicated PostgreSQL database and one dedicated login role per gateway**, so each
gateway is isolated from every other gateway sharing the same external server.

| Mode | `DATABASE_PROVIDER` | Server lifecycle | Per-gateway isolation |
|---|---|---|---|
| Deployment | `deployment` | HyperShell runs an in-cluster Postgres `Deployment` per gateway | Separate instance |
| CNPG | `cnpg` | HyperShell runs a shared in-cluster CNPG `Cluster` | Separate logical DB + role in the shared cluster |
| **External** | `external` | **Owned externally (AWS/IBM); HyperShell only registers it** | **Separate logical DB + login role in the shared external server** |

The isolation model is intentionally identical to CNPG's (shared server, per-gateway
`gw_<gatewayID>` database + role). The essential difference is that **no operator
runs the DDL** - HyperShell's control plane issues the `CREATE DATABASE` /
`CREATE ROLE` / `GRANT` statements itself against the external server.

`external` is a new value of the existing install-wide `DATABASE_PROVIDER`
selector. An install runs exactly one provider (see Requirement: DATABASE_PROVIDER
Selection And Validation, amended below). PostgreSQL is the only supported backend.

### Relationship to global-architecture.spec.md

This spec deliberately reintroduces cloud-managed databases as an **opt-in**
alternative for operators who must consume a managed DB offering (for compliance,
backups, HA, or existing cloud investment). CNPG remains the default, portable
choice.

[`global-architecture.spec.md`](./global-architecture.spec.md) previously stated that
CNPG "replaces per-gateway cloud-managed databases (RDS, Cloud SQL)," which read as
abandoning that direction outright. Its § Database Strategy has been amended to
present all three providers and to describe `external` as supported and opt-in, so
the two specs no longer conflict.

---

## Prerequisites

External mode is **register-only**. The following are the platform administrator's
responsibility, out-of-band, before an `external` ManagedDatabase is created:

1. **A running PostgreSQL server** (AWS RDS/Aurora or IBM Cloud Databases for
   PostgreSQL) reachable from the control-plane cluster.
2. **Network egress / reachability.** The control-plane cluster SHALL have a
   network path to the server endpoint (VPC peering, private endpoint/service
   endpoint, security-group / ACL allow rules). Loss of reachability is a
   provisioning failure, not a silent skip (see readiness below).
3. **An administrative role** on the server with at least `CREATEDB` and
   `CREATEROLE` privileges. Full superuser is **not** required; the AWS RDS
   `rds_superuser` role and the IBM Cloud Databases administrative user are both
   acceptable. The admin role does not need, and SHOULD NOT be granted, the ability
   to alter server-level configuration.
4. **An admin credential Secret** (the "connection Secret") in a control-plane
   readable location (see Requirement: External Connection Secret).
5. **Server-side log verbosity restricted.** `CREATE ROLE` and `ALTER ROLE`
   statements carry the plaintext password in the statement text (the PostgreSQL
   wire protocol has no separate credential-binding channel for these statements).
   Operators MUST set `log_statement` to `'mod'` or lower, or enable server-side
   log redaction, on any external server used with this provider. HyperShell
   redacts credentials in its own application logs and error messages; server-side
   redaction is the operator's responsibility.

There is **no CNPG operator prerequisite** and **no in-cluster Postgres workload**
in external mode.

### Assumption: exactly one external server per HyperShell install

An install with `DATABASE_PROVIDER=external` is expected to have exactly one
registered `external` ManagedDatabase (the sole external server). Per-gateway
database/role names use `gw_<gatewayID>` (KSUID-unique) with no install-scoped
prefix. Sharing one external server across multiple HyperShell installs is **out
of scope** for this spec; doing so risks name collisions and is not supported.

---

## Architecture

### ManagedDatabase as a registration of an external server

An `external` ManagedDatabase is a **registration record**, not a provisioning
target. It carries:

- `provider: "external"`
- `connection_secret` - reference to the admin credential Secret (**required**)
- `region` - the cloud region of the server (informational metadata; not used for placement)
- `engine`, `engine_version`, `instance_class` - descriptive metadata (informational;
  MAY influence connection defaults, e.g. engine-specific TLS handling)
- `namespace` - auto-assigned `openshell-db-<hex16>` as in other modes; used only as
  a stable location for the admin Secret reference resolution and status, **not** for
  any Postgres workload.

```
Platform team (out of band)
  └── AWS RDS / IBM Cloud Databases for PostgreSQL  (endpoint + admin user)

Admin credential Secret (created by operator)  ──referenced by──┐
                                                                 ▼
ManagedDatabase (provider=external, connection_secret=...)
  │  ManagedDatabaseReconciler
  ▼  (no namespace workload; validates connectivity + admin capability → status)

Gateway A ──database_id──→ ManagedDatabase (sole external)
Gateway B ──database_id──→ ManagedDatabase (same)

  │  GatewayReconciler (per gateway), admin connection to external server
  ▼
External server:
  ├── ROLE     gw_<gatewayID>  (LOGIN, owns its database)
  └── DATABASE gw_<gatewayID>  (owner gw_<gatewayID>; CONNECT revoked from PUBLIC)

  └── Secret openshell-gateway-db-credentials (tenant namespace) → gateway --db-url
```

### DDL execution: in-process, in the control plane

The control plane issues DDL **in-process** using a PostgreSQL client, opening a
short-lived admin connection per reconciliation and closing it afterward. It does
**not** maintain a long-lived connection pool and does **not** launch Kubernetes
Jobs to run SQL.

Rationale: per-gateway provisioning is a database operation, not a Kubernetes one.
In-process execution gives synchronous, structured errors (required by the control
plane conventions - `fmt.Errorf` with context, status set on every error path),
trivially idempotent reconcile (query `pg_database`/`pg_roles`, then act), clean
status transitions, straightforward rotation, and testability against a
throwaway PostgreSQL (testcontainers). A Job-based approach would reintroduce the
asynchronous log/exit-code parsing that the gRPC-watch reconciler pattern avoids.

**DDL and server-side logging caveat:** `CREATE ROLE` and `ALTER ROLE` statements
carry the plaintext password in the statement text (the PostgreSQL wire protocol
does not offer a separate credential-binding channel for these statements). On
servers configured with `log_statement = 'all'` or `log_statement = 'ddl'`, the
password will appear in the server's activity log. Operators SHOULD restrict
`log_statement` to `'mod'` or lower, or enable server-side log redaction, on any
external server used with this provider. HyperShell redacts credentials in its own
application logs and error messages; server-side redaction is the operator's
responsibility.

### Selection: sole external ManagedDatabase

Placement selects the single registered `external` ManagedDatabase, applying
the same exactly-one constraint as CNPG mode (see Requirement: External Server
Selection). An install with `DATABASE_PROVIDER=external` is expected to have
exactly one `external` ManagedDatabase; zero or more than one is a validation
error on gateway creation.

---

## Requirements

### Requirement: External Provider Validation and Immutability

The API server SHALL accept `external` as a ManagedDatabase provider value, in
addition to `cnpg` and `deployment`. Provider immutability from the parent spec
applies unchanged: once set to `external`, the provider SHALL NOT transition to any
other value; status-only and other mutable-field updates SHALL preserve it.

An `external` ManagedDatabase SHALL be rejected at create/replace time if
`connection_secret` is empty. `region` is optional metadata (informational only;
it is not used for placement).

#### Scenario: Create external ManagedDatabase without a connection secret

- GIVEN a create request with `provider: "external"` and empty `connection_secret`
- WHEN the API server validates the request
- THEN it SHALL reject it as invalid input naming the missing `connection_secret`
- AND SHALL NOT persist the ManagedDatabase

#### Scenario: Attempt to change provider away from external

- GIVEN an existing ManagedDatabase with `provider: "external"`
- WHEN a caller attempts to update its provider to `cnpg` or `deployment`
- THEN the API server SHALL reject the update as invalid input
- AND the persisted provider SHALL remain `external`

---

### Requirement: External Connection Secret

The `connection_secret` field SHALL reference a Kubernetes Secret holding the
administrative connection to the external server. Per the security standards
(secret references, not inline secrets), no admin credential is stored in the API
server database - only the reference.

The operator provisions this Secret out-of-band (as they provision the server
itself).

#### Reference format

`connection_secret` is a **Secret name, not a path**. It SHALL satisfy all of:

1. It SHALL NOT contain `/`. A `namespace/name` form SHALL be rejected.
2. It SHALL begin with the reserved prefix `hypershell-managed-db-`.
3. It SHALL be a valid DNS-1123 subdomain.

The control plane SHALL resolve it **only** in its own instance namespace. Per
[`naming-multitenancy.spec.md`](../standards/platform/naming-multitenancy.spec.md)
§1, namespaced resources carry constant names and are isolated by namespace, so this
reference needs no instance prefix and SHALL NOT be rewritten by a Kustomize
`namePrefix`.

These rules SHALL be enforced in two places: the API server SHALL validate them at
create, replace and patch and reject a violation as invalid input (so the operator
gets an immediate, actionable error), and the control plane SHALL re-check them at
resolution time and refuse to read a Secret that does not satisfy them, before
issuing any read.

> **Why the reference is constrained.** The control-plane ServiceAccount holds a
> ClusterRole granting `get`/`list`/`watch` on Secrets across all namespaces, because
> the deployment and CNPG providers legitimately create and copy Secrets into
> ManagedDatabase and tenant namespaces. Narrowing that grant is not an option here.
> An unconstrained `connection_secret` would therefore let anyone who can create a
> ManagedDatabase name **any** Secret in the cluster and have the control plane read
> it and open a PostgreSQL connection with its contents - including
> `hypershell-db-app`, the API server's own database credentials, which sits in the
> same namespace with exactly the key shape this Secret expects. The control plane
> would then run `CREATE ROLE` / `CREATE DATABASE` inside the platform's own database
> and hand a tenant working credentials to it. HyperShell platform administrator is
> an API-level role that does not imply permission to read Secrets in the hub
> cluster, so this would be a genuine privilege escalation. The reserved prefix plus
> the fixed namespace reduce the reachable set to Secrets an operator deliberately
> named as external database credentials, and §6 of the naming standard governs that
> name space.

The Secret SHALL be registered in the naming standard's resource inventory as
`hypershell-managed-db-<name>` (Control Plane Instance scope).

The Secret SHALL contain:

| Key | Required | Meaning |
|---|---|---|
| `host` | yes | External server hostname/endpoint |
| `port` | yes | External server port (typically `5432`) |
| `user` | yes | Admin role with `CREATEDB` + `CREATEROLE` |
| `password` | yes | Admin role password |
| `dbname` | no | Maintenance/admin database to connect to (default `postgres`) |
| `sslmode` | no | Admin connection TLS mode (default `require`) |
| `sslrootcert` | no | Absolute file-system path to a PEM CA bundle mounted in the control-plane pod (e.g. via a volume from a ConfigMap or Secret); passed verbatim as the `sslrootcert` DSN parameter (`lib/pq` expects a path, not inline PEM) |

Admin credentials SHALL NEVER appear in logs, error strings, telemetry, or API
responses.

#### Scenario: Reject a namespace-qualified connection secret

- GIVEN a create request with `provider: "external"` and
  `connection_secret: "kube-system/hypershell-managed-db-x"`
- WHEN the API server validates the request
- THEN it SHALL reject it as invalid input because the reference contains `/`
- AND SHALL NOT persist the ManagedDatabase

#### Scenario: Reject a connection secret outside the reserved prefix

- GIVEN a create request with `provider: "external"` and
  `connection_secret: "hypershell-db-app"`
- WHEN the API server validates the request
- THEN it SHALL reject it as invalid input naming the required
  `hypershell-managed-db-` prefix
- AND SHALL NOT persist the ManagedDatabase

#### Scenario: Control plane refuses a non-conforming reference at resolve time

- GIVEN a persisted `external` ManagedDatabase whose `connection_secret` does not
  satisfy the reference format (for example, written before this rule existed)
- WHEN the ManagedDatabaseReconciler resolves it
- THEN it SHALL set status `Failed: secret_invalid` and SHALL NOT read any Secret
- AND SHALL NOT open a connection to any server

#### Scenario: Connection secret missing at reconcile time

- GIVEN an `external` ManagedDatabase whose `connection_secret` cannot be resolved
- WHEN the ManagedDatabaseReconciler processes it
- THEN it SHALL set status `Failed: secret_invalid`
- AND SHALL NOT proceed to any gateway provisioning against that server
- AND the next reconciliation SHALL retry

---

### Requirement: ManagedDatabase Reconciliation (provider=external)

The ManagedDatabaseReconciler SHALL treat an `external` ManagedDatabase as a
**connectivity + capability check**, not an infrastructure-provisioning step. It
SHALL NOT create a Namespace workload, CNPG `Cluster`, `Deployment`, `Service`, or
`PVC`.

For each `external` ManagedDatabase, the reconciler SHALL:

1. Resolve and read the admin connection Secret (`connection_secret`).
2. Open a short-lived admin connection to the external server using the Secret's
   TLS settings.
3. Verify the admin role can create databases and roles (e.g. confirm
   `rolcreatedb` and `rolcreaterole`, or attempt a harmless capability probe).
4. Set status from the closed reason vocabulary below.
5. Close the connection.

The check SHALL be idempotent and side-effect-free on the external server.

#### Status reason vocabulary

The ManagedDatabase `status` field is returned by every read of the resource. In
external mode the reconciler SHALL set it to exactly one of the following values and
SHALL NOT interpolate a driver error into it:

| Status | Meaning |
|---|---|
| `Provisioning` | The probe has not yet completed |
| `Ready` | Connected, and the admin role has `CREATEDB` and `CREATEROLE` |
| `Failed: secret_invalid` | `connection_secret` does not resolve, is missing a required key, or fails the naming/namespace rules |
| `Failed: unreachable` | No network path, DNS failure, or connection timeout |
| `Failed: auth_failed` | The server rejected the admin credentials |
| `Failed: insufficient_privilege` | Connected, but the admin role lacks `CREATEDB` or `CREATEROLE` |
| `Failed: tls_failed` | TLS negotiation or certificate verification failed |

The reconciler SHALL map every underlying error onto one of these values, defaulting
to `Failed: unreachable` for an unrecognised connection-time error. PostgreSQL driver
errors routinely embed the host, the admin user, and sometimes the full DSN, so the
driver's own message SHALL NOT reach `status`. It MAY be logged, redacted per the
security standards, to give operators a diagnostic path.

> This is a deliberate departure from the CNPG and deployment branches, which format
> the underlying error into the status with `fmt.Sprintf("Failed: %v", err)`. Those
> errors come from the Kubernetes API; external-mode errors come from a database
> driver holding admin credentials, and are not safe to echo.

#### Scenario: External server reachable with a capable admin

- GIVEN an `external` ManagedDatabase whose admin Secret connects successfully
  and whose admin role has `CREATEDB` and `CREATEROLE`
- WHEN the ManagedDatabaseReconciler processes it
- THEN it SHALL set status `Ready`
- AND SHALL create no Kubernetes workload in the ManagedDatabase namespace

#### Scenario: External server unreachable

- GIVEN an `external` ManagedDatabase whose endpoint is not reachable from the
  control-plane cluster
- WHEN the ManagedDatabaseReconciler processes it
- THEN it SHALL set status `Failed: unreachable`
- AND the driver's error text SHALL NOT appear in the status
- AND the next reconciliation SHALL retry

#### Scenario: Driver error is not echoed into status

- GIVEN an `external` ManagedDatabase whose admin connection fails with a driver
  error containing the endpoint hostname and admin username
- WHEN the ManagedDatabaseReconciler sets the resource status
- THEN the status SHALL be one of the closed reason values
- AND SHALL NOT contain the driver's message, the hostname, the username, or any
  part of the connection string

#### Scenario: Admin role lacks required privileges

- GIVEN an `external` ManagedDatabase whose admin role lacks `CREATEDB` or `CREATEROLE`
- WHEN the ManagedDatabaseReconciler processes it
- THEN it SHALL set status `Failed: insufficient_privilege`
- AND SHALL NOT attempt per-gateway provisioning

---

### Requirement: External Server Selection

When `DATABASE_PROVIDER=external`, the API server SHALL resolve a new Gateway's
`database_id` (server-owned; caller value ignored and replaced, as in all modes) by
finding the sole registered `external` ManagedDatabase:

1. Collect all `ManagedDatabase` records with `provider=external`.
2. If **exactly one** exists, assign its ID.
3. If **zero** exist, reject the creation with a contextual error.
4. If **more than one** exists, reject the creation as ambiguous.

This mirrors CNPG's placement constraint. `cluster_id` has no effect on placement
in external mode (consistent with the parent spec's general rule).

#### Scenario: Gateway placement selects the sole external database

- GIVEN `DATABASE_PROVIDER=external` and exactly one `external` ManagedDatabase
- WHEN the API server processes the create request
- THEN it SHALL assign that ManagedDatabase's ID as the gateway's `database_id`

#### Scenario: Gateway placement finds no external database

- GIVEN `DATABASE_PROVIDER=external` and no `external` ManagedDatabase exists
- WHEN the API server processes the create request
- THEN it SHALL reject the creation with a contextual error and create no gateway

#### Scenario: Gateway placement is ambiguous

- GIVEN `DATABASE_PROVIDER=external` and more than one `external` ManagedDatabase
- WHEN the API server processes the create request
- THEN it SHALL reject the creation as ambiguous rather than picking one arbitrarily

---

### Requirement: Per-Gateway Database Provisioning (External Mode)

In external mode, the GatewayReconciler SHALL provision a dedicated PostgreSQL
database and login role for each gateway by issuing idempotent DDL against the
external server over an admin connection. It SHALL NOT create CNPG CRs and SHALL
NOT copy credentials from a ManagedDatabase namespace Secret.

Naming follows the parent spec: role and database are both `gw_<gatewayID>`
(underscores; PostgreSQL identifiers avoid hyphens). `<gatewayID>` is the gateway's
full resource ID, lowercased.

For each gateway, the reconciler SHALL:

1. Resolve the gateway's `database_id` to the `external` ManagedDatabase and read
   its admin connection Secret.
2. Determine the per-gateway password: if the tenant-namespace Secret
   `openshell-gateway-db-credentials` already exists with a `password`, **reuse it**
   (create-or-skip semantics - do not regenerate on re-reconciliation); otherwise
   generate a 32-byte cryptographically random hex password (`crypto/rand`) and treat
   it as authoritative, forcing it onto the role in step 3.
3. Open a short-lived admin connection and reconcile, idempotently:
   - **Role:** if `gw_<gatewayID>` is absent (`SELECT 1 FROM pg_roles ...`), create it
     with `LOGIN` and the password. If the role is present **and** the password was
     reused from an existing tenant Secret, leave it alone (password otherwise
     reconciled only on rotation - see Manual Credential Rotation). If the role is
     present but the password was newly generated - because the tenant Secret was
     absent - the reconciler SHALL `ALTER ROLE gw_<gatewayID> PASSWORD '<new>'` so the
     role matches the Secret it is about to write. Writing a freshly generated
     password into the tenant Secret without applying it to an existing role produces
     a gateway that can never authenticate and that re-reconciliation would not
     repair.
   - **Database:** if `gw_<gatewayID>` is absent (`SELECT 1 FROM pg_database ...`),
     create it with `OWNER gw_<gatewayID>`. (`CREATE DATABASE` cannot run inside a
     transaction block and has no `IF NOT EXISTS`; the reconciler SHALL guard it with
     an existence check rather than relying on catching an error.)
   - **Isolation:** `REVOKE CONNECT ON DATABASE gw_<gatewayID> FROM PUBLIC` and grant
     `CONNECT` only to `gw_<gatewayID>`, so no other gateway's role can connect. The
     owning role's default `public` schema privileges SHALL be scoped so tenants
     cannot read or write each other's databases.
4. Write/refresh the tenant-namespace Secret `openshell-gateway-db-credentials`
   (see Requirement: Gateway Credentials Secret (External Mode)).
5. Proceed to deploy the gateway workload only after DDL and the credentials Secret
   succeed.

All DDL SHALL be idempotent and reconcile-not-create-or-skip: re-running against an
already-provisioned gateway SHALL make no destructive change and SHALL NOT
regenerate the password. Every non-benign SQL error SHALL be propagated with context
(never swallowed); credentials SHALL NOT appear in error text.

#### Scenario: New gateway provisioned on an external server

- GIVEN a new Gateway resolved to an `external` ManagedDatabase
- WHEN the GatewayReconciler processes the event
- THEN it SHALL create role and database `gw_<gatewayID>` on the external server if absent
- AND revoke `CONNECT` from `PUBLIC` on that database
- AND write `openshell-gateway-db-credentials` into the tenant namespace
- AND proceed to deploy the gateway workload

#### Scenario: Re-reconcile an already-provisioned external gateway

- GIVEN a Gateway whose external role and database already exist
- WHEN the GatewayReconciler re-processes the event
- THEN it SHALL detect both exist and make no destructive change
- AND SHALL NOT regenerate or alter the password

#### Scenario: Tenant credentials Secret lost while the role still exists

- GIVEN a Gateway whose external role `gw_<gatewayID>` exists on the server
- AND whose tenant-namespace `openshell-gateway-db-credentials` Secret is absent
  (for example after tenant-namespace garbage collection or a cluster rebuild)
- WHEN the GatewayReconciler processes the event
- THEN it SHALL generate a new password, apply it to the existing role with
  `ALTER ROLE`, and write the matching tenant Secret
- AND the gateway SHALL be able to authenticate without operator intervention

#### Scenario: External server unreachable during gateway provisioning

- GIVEN a Gateway resolved to an `external` ManagedDatabase whose server is unreachable
- WHEN the GatewayReconciler attempts DDL
- THEN it SHALL return a contextual error, leave the gateway phase `Provisioning`,
  and retry on the next reconciliation, without creating the gateway workload

---

### Requirement: Gateway Credentials Secret (External Mode)

After per-gateway DDL, the GatewayReconciler SHALL ensure the tenant-namespace
Secret `openshell-gateway-db-credentials` exists, consumed by the gateway workload
via `--db-url $(OPENSHELL_DB_URL)` exactly as in the parent spec.

| Key | Value |
|---|---|
| `host` | external server endpoint (from the admin Secret `host`) |
| `port` | external server port (from the admin Secret `port`) |
| `dbname` | `gw_<gatewayID>` |
| `user` | `gw_<gatewayID>` |
| `password` | generated per-gateway password |
| `uri` | `postgresql://gw_<gatewayID>:<password>@<host>:<port>/gw_<gatewayID>?sslmode=<mode>` |
| `sslrootcert` | (optional) PEM CA bundle, present when `verify-full` is used |

**TLS:** the connection to a cloud-managed server SHALL be encrypted. Default
`sslmode=require` (encrypt without certificate verification), which needs no extra
files in the tenant namespace. `sslmode=verify-full` is the recommended hardening and
is opt-in: when the admin Secret carries `sslrootcert`, the reconciler SHALL
propagate the CA into the tenant namespace and set `verify-full`, which requires the
gateway workload to mount and reference the CA. Distributing/mounting the CA into the
gateway workload is tracked as a follow-up; v1 MAY ship with `require` as the
enforced default and `verify-full` behind that follow-up.

---

### Requirement: Per-Gateway Cleanup

When a Gateway backed by an `external` ManagedDatabase is deleted, the control plane
SHALL destroy that gateway's external database and role. Deletion is unconditional:
there is no retention policy and no per-database configuration. This matches the
other two providers - CNPG reclaims with `databaseReclaimPolicy: delete`, and
deployment mode deletes the ManagedDatabase namespace outright - so gateway deletion
means the same thing under every provider.

> **Recovery is the external provider's responsibility.** AWS RDS/Aurora and IBM
> Cloud Databases both provide automated backups and point-in-time recovery, which is
> among the reasons an operator selects a managed offering. HyperShell's drop is not
> the last line of defence against accidental deletion, and HyperShell SHALL NOT
> attempt to be one by retaining orphaned tenant databases on a shared server.

Over an admin connection, the control plane SHALL:

1. Terminate active backends connected to `gw_<gatewayID>` (`pg_terminate_backend`
   over `pg_stat_activity`), so the drop is not blocked by the gateway's own
   lingering connections.
2. `DROP DATABASE gw_<gatewayID>` (guarded by an existence check; `DROP DATABASE`
   cannot run inside a transaction block).
3. `DROP ROLE gw_<gatewayID>`.

Cleanup SHALL be tombstone-driven and idempotent, reusing the parent spec's
tombstone/retry machinery (`hypershell-managed-database-delete-tombstones` watch
capability, watch-lifetime retry queue, already-absent treated as success). Every
non-benign cleanup failure SHALL be propagated; already-absent objects SHALL count as
successful cleanup. Because both object names derive from the gateway ID carried on
the tombstone, cleanup requires no state beyond the tombstone itself.

The tenant-namespace `openshell-gateway-db-credentials` Secret is removed by the
existing label-based tenant-namespace cleanup.

HyperShell SHALL NEVER drop, resize, or delete the external **server** itself;
`external` ManagedDatabase deletion (allowed only when no gateway references it)
performs no destructive action on the external server.

#### Scenario: Delete external-backed gateway

- GIVEN a Gateway backed by an `external` ManagedDatabase
- WHEN the Gateway is deleted
- THEN the control plane SHALL terminate active connections to `gw_<gatewayID>`,
  drop the database, then drop the role
- AND SHALL remove the tenant-namespace credentials Secret

#### Scenario: Replay a cleanup whose objects are already gone

- GIVEN a delete tombstone for a Gateway whose external database and role are absent
- WHEN the control plane replays the cleanup
- THEN it SHALL treat both as successfully cleaned up and SHALL NOT return an error

#### Scenario: Cleanup fails while the external server is unreachable

- GIVEN a delete tombstone for a Gateway whose external server is unreachable
- WHEN the control plane attempts cleanup
- THEN it SHALL propagate a contextual error and retain the tombstone in the
  watch-lifetime retry queue until the drop succeeds

#### Scenario: Delete external ManagedDatabase leaves the server untouched

- GIVEN an `external` ManagedDatabase with no referencing gateways
- WHEN it is deleted
- THEN the control plane SHALL perform no destructive action on the external server
- AND SHALL clean up only HyperShell-owned registration state (namespace, if any)

#### Operator runbook: identifying and recovering from orphaned objects

The `Delete` lifecycle for external gateways is best-effort: the `DatabaseReconciler`
interface carries no return value on deletion (the gateway is already removed from the
API server, so the event cannot be retried at the reconciler level). If the external
server is unreachable or the drop fails, the role and database persist with valid
credentials. Operators can detect and recover orphaned objects as follows:

1. **Identify orphaned databases** - Connect as the admin user and query:
   ```sql
   SELECT datname FROM pg_database WHERE datname LIKE 'gw_%';
   ```
   Compare the result to the active gateway IDs currently registered in HyperShell.
   Any `gw_<id>` database whose gateway ID is no longer in HyperShell is orphaned.

2. **Identify orphaned roles** - Similarly:
   ```sql
   SELECT rolname FROM pg_roles WHERE rolname LIKE 'gw_%';
   ```

3. **Revoke credentials and drop** - For each orphaned object:
   ```sql
   SELECT pg_terminate_backend(pid)
     FROM pg_stat_activity WHERE datname = 'gw_<id>';
   DROP DATABASE "gw_<id>";
   DROP ROLE "gw_<id>";
   ```

4. **Tenant Secret** - The `openshell-gateway-db-credentials` Secret in the gateway's
   tenant namespace is removed by the platform's label-based namespace cleanup when
   the gateway namespace is reclaimed. If the namespace was already deleted, the
   Secret is gone. If it persists, delete it manually.

---

### Requirement: Manual Credential Rotation (External Mode)

External mode SHALL support the parent spec's annotation-triggered rotation
(`hypershell.redhat.io/rotate-db-credentials`). On a new trigger value the
GatewayReconciler SHALL: generate a new `crypto/rand` password; `ALTER ROLE
gw_<gatewayID> PASSWORD '<new>'` on the external server over the admin connection;
update the tenant-namespace `openshell-gateway-db-credentials` Secret; and set
`hypershell.redhat.io/last-db-rotation` to the trigger value so the config-hash
changes and the gateway rolls. Rotation failure SHALL be propagated with context and
retried; the password SHALL never be logged.

---

### Requirement: Database Credential Security (External Mode)

The parent spec's credential-security requirements apply unchanged, plus:

- Admin credentials are held only for the duration of a reconciliation and never
  persisted by HyperShell beyond the referenced Secret.
- Per-gateway passwords SHALL be generated with `crypto/rand` (32-byte hex),
  create-or-skip on re-reconciliation, and never logged.
- The connection to the external server SHOULD always be TLS-encrypted (minimum
  `sslmode=require`). `sslmode=disable` is insecure and SHOULD NOT be used in
  production. The control plane emits a WARN log when `sslmode=disable` is read from
  the admin Secret so operators see the misconfiguration without the reconciler
  failing. Development and CI environments that use a local PostgreSQL server without
  TLS may set `sslmode=disable`; this is explicitly not recommended for any external
  server reachable from outside the cluster.

---

### Requirement: DATABASE_PROVIDER Selection And Validation (Amended)

This spec amends the parent spec's identical requirement to add `external`:

- `external` selects external-backed placement and per-gateway DDL. It requires a
  reachable external server and a valid connection Secret at reconcile time, but
  imposes **no CNPG API startup check**.
- Valid `DATABASE_PROVIDER` values are now: unset/empty (→ `deployment`),
  `deployment`, `cnpg`, `external`. Any other value remains a fatal startup
  configuration error (contextual error, exit non-zero, never panic, never silently
  fall back).

#### Scenario: DATABASE_PROVIDER=external selects external placement

- GIVEN `DATABASE_PROVIDER=external`
- WHEN the API server or control plane starts
- THEN it SHALL select external-backed placement
- AND SHALL NOT require any CNPG API to be present

#### Scenario: Unsupported DATABASE_PROVIDER value still fails startup

- GIVEN `DATABASE_PROVIDER` set to a value other than unset/empty, `deployment`,
  `cnpg`, or `external`
- WHEN the API server or control plane starts
- THEN it SHALL fail to start with a contextual error naming the invalid value and
  the supported values, and SHALL NOT silently fall back

---

## Configuration Reference

| Variable | Type | Default | Description |
|---|---|---|---|
| `DATABASE_PROVIDER` | env var | `deployment` | Now also accepts `external`. Valid values: `deployment`, `cnpg`, `external`. Any other value is a startup configuration error. |

No new environment variables are introduced; the external server endpoint,
credentials, and TLS material all live in the admin connection Secret referenced by
`connection_secret`. `OPENSHELL_DATABASE_IMAGE` is not used in external mode (no
in-cluster Postgres workload).

---

## Configuration Examples

Admin connection Secret (created out-of-band by the operator; here in the control
plane instance namespace, referenced as
`connection_secret: "hypershell-managed-db-us-east-1"`):

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: hypershell-managed-db-us-east-1
  namespace: hyp0                # the control plane instance namespace
type: Opaque
stringData:
  host: mydb.abc123.us-east-1.rds.amazonaws.com
  port: "5432"
  dbname: postgres
  user: hypershell_admin        # rds_superuser / IBM admin; has CREATEDB + CREATEROLE
  password: <admin-password>
  sslmode: verify-full
  sslrootcert: |
    -----BEGIN CERTIFICATE-----
    ...cloud provider CA bundle...
    -----END CERTIFICATE-----
```

ManagedDatabase (registration, created via API):

```json
{
  "name": "rds-us-east-1",
  "provider": "external",
  "region": "us-east-1",
  "engine": "postgres",
  "engine_version": "16",
  "connection_secret": "hypershell-managed-db-us-east-1"
}
```

Per-gateway objects the GatewayReconciler creates on the external server (illustrative SQL):

```sql
-- role
CREATE ROLE gw_2j5k7m9pqrstvwxyz LOGIN PASSWORD '<32-byte-hex-random>';
-- database owned by the role
CREATE DATABASE gw_2j5k7m9pqrstvwxyz OWNER gw_2j5k7m9pqrstvwxyz;
-- isolation
REVOKE CONNECT ON DATABASE gw_2j5k7m9pqrstvwxyz FROM PUBLIC;
GRANT  CONNECT ON DATABASE gw_2j5k7m9pqrstvwxyz TO gw_2j5k7m9pqrstvwxyz;
```

Gateway credentials Secret (tenant namespace):

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: openshell-gateway-db-credentials
  namespace: openshell-a1b2c3d4e5f67890
  labels:
    hypershell.redhat.io/managed: "true"
type: Opaque
stringData:
  host: mydb.abc123.us-east-1.rds.amazonaws.com
  port: "5432"
  dbname: gw_2j5k7m9pqrstvwxyz
  user: gw_2j5k7m9pqrstvwxyz
  password: <32-byte-hex-random>
  uri: postgresql://gw_2j5k7m9pqrstvwxyz:<password>@mydb.abc123.us-east-1.rds.amazonaws.com:5432/gw_2j5k7m9pqrstvwxyz?sslmode=require
```

---

## Debugging Reference

| Symptom | Root Cause | Fix |
|---|---|---|
| ManagedDatabase status `Failed: unreachable` | No network path from control-plane cluster to endpoint | Fix VPC peering / security groups / private endpoint |
| ManagedDatabase status `Failed: authentication` | Wrong admin credentials in connection Secret | Correct the admin Secret |
| ManagedDatabase status `Failed: insufficient privilege` | Admin role lacks CREATEDB/CREATEROLE | Grant `rds_superuser` (AWS) / admin role (IBM) or the two privileges |
| Gateway create rejected: no eligible external database | No `external` ManagedDatabase is registered | Register an `external` ManagedDatabase |
| Gateway create rejected: ambiguous | More than one `external` ManagedDatabase is registered | Consolidate to exactly one |
| Gateway pod cannot connect | TLS mismatch or wrong host in tenant Secret | Verify `sslmode`/CA and `openshell-gateway-db-credentials` |

---

## Affected Components (Implementation Impact)

Spec-driven summary of the code surface a `/reconcile` wave would touch (for
planning only; authoritative locations verified against the current tree):

- **API server - ManagedDatabase**
  - `plugins/managedDatabases/service.go` - add `external` to supported providers;
    require `connection_secret` and enforce the reference format (no `/`, reserved
    `hypershell-managed-db-` prefix, DNS-1123); keep provider immutable.
  - **No data-model change.** `provider`, `region`, `engine`, `engine_version`,
    `instance_class` and `connection_secret` already exist in `model.go`,
    `migration.go`, `openapi.managedDatabases.yaml` and
    `proto/hypershell/v1/managed_databases.proto`. No new column, no migration, no
    regenerated stubs. Only documentation of `provider: external` is added.
- **API server - Gateway placement**
  - `plugins/gateways/provider.go`, `plugin.go`, `placement.go` - accept
    `DATABASE_PROVIDER=external`; reuses the CNPG sole-DB lookup pattern, filtered
    to `provider=external`. No cluster/region lookup; `cluster_id` has no placement
    effect in external mode (consistent with the parent spec's general rule).
- **Control plane**
  - Add a PostgreSQL client dependency and an admin-connection helper (per-reconcile,
    short-lived, credentials from the connection Secret).
  - `internal/config/config.go` - accept `external` value.
  - `internal/reconciler/reconciler.go` - `ManagedDatabaseReconciler`: `external`
    branch = connectivity/capability check (no K8s workload); `GatewayReconciler`:
    external per-gateway DDL, tenant credentials Secret, rotation, deletion policy.
  - Tombstone cleanup path for external gateways (drop database, then role).
- **Related specs (already amended alongside this one)**
  - `openshell-gateway-database.spec.md` - mode table, `DATABASE_PROVIDER` values and
    validation, `externalPlacement` in Automatic Database Assignment, the `cluster_id`
    placement-effect exception, admin-workflow table, configuration and debugging
    references.
  - `global-architecture.spec.md` - § Database Strategy now covers all three
    providers and presents `external` as supported and opt-in; the CNPG operator
    requirement is scoped to `DATABASE_PROVIDER=cnpg`.
  - `naming-multitenancy.spec.md` - `hypershell-managed-db-<name>` registered in the
    §6 resource inventory as a reserved prefix.
  - `data-model.spec.md` - ManagedDatabase entry records the three `provider` values
    and notes that `external` adds no columns.

---

## References

- Parent: [`openshell-gateway-database.spec.md`](./openshell-gateway-database.spec.md)
- [`security.spec.md`](../standards/security/security.spec.md) - secret references, not inline secrets
- [`control-plane/conventions.spec.md`](../standards/control-plane/conventions.spec.md) - reconciler error handling, no panic
- [AWS RDS PostgreSQL - master user privileges (`rds_superuser`)](https://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/CHAP_PostgreSQL.html)
- [IBM Cloud Databases for PostgreSQL - administration](https://cloud.ibm.com/docs/databases-for-postgresql)
- [PostgreSQL - `CREATE DATABASE`, `CREATE ROLE`, `GRANT`/`REVOKE`](https://www.postgresql.org/docs/current/sql-createdatabase.html)
