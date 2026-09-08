# OpenShell Gateway Database Specification - External Provider

**Date:** 2026-09-07
**Status:** Active
**Parent:** [`openshell-gateway-database.spec.md`](./openshell-gateway-database.spec.md) - gateway database provisioning

---

## Purpose

This specification defines the **`external`** PostgreSQL provisioning mode for
OpenShell gateways, selected by `DATABASE_PROVIDER=external`.

In `external` mode, PostgreSQL server infrastructure is **provisioned outside
HyperShell** - a cloud-managed database (AWS RDS / Aurora for PostgreSQL, IBM
Cloud Databases for PostgreSQL) created out-of-band by a platform team or IaC.
HyperShell does **not** create, resize, or delete the server. HyperShell
**registers** the pre-existing endpoint as a `ManagedDatabase` and, within it,
provisions **one dedicated PostgreSQL database and one dedicated login role per
gateway**, so each gateway is isolated from every other gateway sharing the same
external server.

The control plane issues the `CREATE DATABASE` / `CREATE ROLE` / `GRANT`
statements itself, over a short-lived administrative connection. No operator is
required and no in-cluster PostgreSQL workload is created.

PostgreSQL is the only supported backend.

### Contracts this spec builds on

External mode participates in the shared gateway-database contracts defined by the
parent spec, restated here so this document stands on its own:

- **`ManagedDatabase`** - the resource a `Gateway` points at through its
  `database_id` foreign key. It carries a `provider` field that determines how it
  is reconciled.
- **`database_id`** - server-owned in every mode. Clients send an empty string;
  the API server ignores and replaces any non-empty value. Omitting the property is
  invalid. `cluster_id` is stored but has no effect on database placement.
- **`openshell-gateway-db-credentials`** - the Secret in the gateway's tenant
  namespace that the gateway workload consumes as `--db-url $(OPENSHELL_DB_URL)`.
- **`DATABASE_PROVIDER`** - the install-wide provider selector, read independently
  at startup by both the API server and the control plane.

### Relationship to global-architecture.spec.md

[`global-architecture.spec.md`](./global-architecture.spec.md) § Database Strategy
presents `external` as a supported, **opt-in** alternative for operators who must
consume a managed database offering - for compliance, backups, HA, or an existing
cloud investment. Nothing about `external` is on the default path.

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
4. **A credentials namespace and Secret** - a Kubernetes Namespace whose name
   carries the reserved prefix `hypershell-managed-db-`, containing a Secret named
   `hypershell-managed-db-credentials` (see Requirement: External Connection
   Namespace And Secret).
5. **Server-side log verbosity restricted.** `CREATE ROLE` and `ALTER ROLE`
   statements carry the plaintext password in the statement text (the PostgreSQL
   wire protocol has no separate credential-binding channel for these statements).
   Operators SHOULD set `log_statement` to `'mod'` or lower, or enable server-side
   log redaction, on any external server used with this provider. HyperShell
   redacts credentials in its own application logs and error messages; server-side
   redaction is the operator's responsibility and HyperShell cannot enforce it.

### Assumption: one HyperShell install per external server

Per-gateway database and role names are `gw_<gatewayID>` (KSUID-unique) with no
install-scoped prefix. Sharing one external server across multiple HyperShell
installs is **out of scope** for this spec; doing so risks name collisions and is
not supported.

---

## Architecture

### ManagedDatabase as a registration of an external server

An `external` ManagedDatabase is a **registration record**, not a provisioning
target. It carries:

- `provider: "external"`
- `connection_secret` - the **namespace** holding the administrative credentials
  Secret (**required**; see the requirement below for why this field names a
  namespace rather than a Secret)
- `region` - the cloud region of the server (informational metadata; not used for
  placement)
- `engine`, `engine_version`, `instance_class` - descriptive metadata,
  informational only
- `namespace` - assigned by the shared `BeforeCreate` hook but **unused** in
  external mode. No Namespace is created for an `external` ManagedDatabase and no
  resource is placed in one.

```
Platform team (out of band)
  ├── AWS RDS / IBM Cloud Databases for PostgreSQL  (endpoint + admin user)
  └── Namespace hypershell-managed-db-<name>
        └── Secret hypershell-managed-db-credentials   ──referenced by──┐
                                                                         ▼
ManagedDatabase (provider=external, connection_secret=hypershell-managed-db-<name>)
  │  ManagedDatabaseReconciler
  ▼  (no namespace, no workload; validates connectivity + admin capability → status)

Gateway A ──database_id──→ ManagedDatabase (external)
Gateway B ──database_id──→ ManagedDatabase (same, or another registered external)

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
status transitions, and testability against a throwaway PostgreSQL
(testcontainers). A Job-based approach would reintroduce the asynchronous
log/exit-code parsing that the gRPC-watch reconciler pattern avoids.

**DDL and server-side logging caveat:** see Prerequisite 5. `CREATE ROLE` and
`ALTER ROLE` carry the plaintext password in the statement text, so on a server
configured with `log_statement = 'all'` or `'ddl'` the password reaches the
server's activity log. Restricting that is the operator's responsibility.

---

## Requirements

### Requirement: External Provider Validation and Immutability

The API server SHALL accept `external` as a ManagedDatabase provider value. Once
set to `external`, the provider SHALL NOT transition to any other value;
status-only and other mutable-field updates SHALL preserve it.

An `external` ManagedDatabase SHALL be rejected at create/replace time if
`connection_secret` is empty. `region`, `engine`, `engine_version` and
`instance_class` are optional metadata, informational only, and SHALL NOT affect
placement or connection behaviour.

#### Scenario: Create external ManagedDatabase without a connection secret

- GIVEN a create request with `provider: "external"` and empty `connection_secret`
- WHEN the API server validates the request
- THEN it SHALL reject it as invalid input naming the missing `connection_secret`
- AND SHALL NOT persist the ManagedDatabase

#### Scenario: Attempt to change provider away from external

- GIVEN an existing ManagedDatabase with `provider: "external"`
- WHEN a caller attempts to update its provider to another value
- THEN the API server SHALL reject the update as invalid input
- AND the persisted provider SHALL remain `external`

---

### Requirement: External Connection Namespace And Secret

The `connection_secret` field SHALL name a **Kubernetes Namespace**, not a Secret.
Within that namespace the administrative connection lives in a Secret whose name is
**fixed**: `hypershell-managed-db-credentials`. Per the security standards (secret
references, not inline secrets), no admin credential is stored in the API server
database - only the namespace reference.

The operator provisions the namespace and the Secret out-of-band, as they provision
the server itself.

> **Why the reference names a namespace.** The credentials are created before
> HyperShell is installed, by whoever provisions the server. Requiring them to live
> in the HyperShell control-plane instance namespace would force that namespace to
> exist first, inverting the intended install order. A dedicated, prefixed namespace
> lets the platform team stage external database credentials independently of any
> HyperShell deployment, and lets them be managed, RBAC-scoped and lifecycled by the
> team that owns the server.

#### Reference format

`connection_secret` SHALL satisfy all of:

1. It SHALL NOT contain `/`. A `namespace/name` form SHALL be rejected.
2. It SHALL begin with the reserved prefix `hypershell-managed-db-`.
3. It SHALL be a valid DNS-1123 **label** (Kubernetes namespace names are labels,
   maximum 63 characters), and therefore also a valid Namespace name.

The control plane SHALL read exactly one Secret for an `external` ManagedDatabase:
`hypershell-managed-db-credentials` in the namespace named by `connection_secret`.
It SHALL NOT read any other Secret in that namespace and SHALL NOT resolve the
reference in any other namespace. Per
[`naming-multitenancy.spec.md`](../standards/platform/naming-multitenancy.spec.md)
§1, the Secret carries a constant name and is isolated by namespace, so it needs no
instance prefix and SHALL NOT be rewritten by a Kustomize `namePrefix`.

These rules SHALL be enforced in two places: the API server SHALL validate them at
create, replace and patch and reject a violation as invalid input (so the operator
gets an immediate, actionable error), and the control plane SHALL re-check them at
resolution time and refuse to read a Secret that does not satisfy them, before
issuing any read.

> **Why the reference is constrained.** The control-plane ServiceAccount holds a
> ClusterRole granting `get`/`list`/`watch` on Secrets across all namespaces, because
> other providers legitimately create and copy Secrets into ManagedDatabase and
> tenant namespaces. Narrowing that grant is not an option here. An unconstrained
> reference would therefore let anyone who can create a ManagedDatabase name **any**
> Secret in the cluster and have the control plane read it and open a PostgreSQL
> connection with its contents - including `hypershell-db-app`, the API server's own
> database credentials, which has exactly the key shape this Secret expects. The
> control plane would then run `CREATE ROLE` / `CREATE DATABASE` inside the
> platform's own database and hand a tenant working credentials to it. HyperShell
> platform administrator is an API-level role that does not imply permission to read
> Secrets in the hub cluster, so this would be a genuine privilege escalation.
>
> The reserved namespace prefix **plus the fixed Secret name** reduce the reachable
> set to a single, deliberately named Secret inside namespaces an operator
> deliberately created as external database credential holders. §6 of the naming
> standard governs that name space.

The namespace SHALL be registered in the naming standard's resource inventory as
`hypershell-managed-db-<name>` (Namespace), and the Secret as
`hypershell-managed-db-credentials` within it.

The Secret SHALL contain:

| Key | Required | Meaning |
|---|---|---|
| `host` | yes | External server hostname/endpoint |
| `port` | yes | External server port (typically `5432`) |
| `user` | yes | Admin role with `CREATEDB` + `CREATEROLE` |
| `password` | yes | Admin role password |
| `dbname` | no | Maintenance/admin database to connect to (default `postgres`) |
| `sslmode` | no | Admin connection TLS mode (default `require`) |
| `sslrootcert` | no | Inline PEM CA bundle used to verify the server certificate when `sslmode` is `verify-ca` or `verify-full`. The control plane materialises it for the driver; operators supply PEM text, never a path |

Admin credentials SHALL NEVER appear in logs, error strings, telemetry, or API
responses.

#### Scenario: Reject a namespace-qualified connection secret reference

- GIVEN a create request with `provider: "external"` and
  `connection_secret: "kube-system/hypershell-managed-db-x"`
- WHEN the API server validates the request
- THEN it SHALL reject it as invalid input because the reference contains `/`
- AND SHALL NOT persist the ManagedDatabase

#### Scenario: Reject a reference outside the reserved prefix

- GIVEN a create request with `provider: "external"` and
  `connection_secret: "default"`
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

#### Scenario: Credentials Secret missing at reconcile time

- GIVEN an `external` ManagedDatabase whose `connection_secret` namespace exists but
  contains no `hypershell-managed-db-credentials` Secret
- WHEN the ManagedDatabaseReconciler processes it
- THEN it SHALL set status `Failed: secret_invalid`
- AND SHALL NOT read any other Secret in that namespace
- AND SHALL NOT proceed to any gateway provisioning against that server
- AND the next reconciliation SHALL retry

---

### Requirement: ManagedDatabase Reconciliation (provider=external)

The ManagedDatabaseReconciler SHALL treat an `external` ManagedDatabase as a
**connectivity + capability check**, not an infrastructure-provisioning step. It
SHALL NOT create a Namespace, a workload, a Service, or a PVC, and SHALL create no
Kubernetes resource of any kind.

For each `external` ManagedDatabase, the reconciler SHALL:

1. Validate `connection_secret` against the reference format and read
   `hypershell-managed-db-credentials` from that namespace.
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
| `Failed: secret_invalid` | `connection_secret` fails the reference format, the namespace or the `hypershell-managed-db-credentials` Secret does not resolve, or the Secret is missing a required key |
| `Failed: unreachable` | No network path, DNS failure, or connection timeout |
| `Failed: auth_failed` | The server rejected the admin credentials |
| `Failed: insufficient_privilege` | Connected, but the admin role lacks `CREATEDB` or `CREATEROLE` |
| `Failed: tls_failed` | TLS negotiation or certificate verification failed |

The reconciler SHALL map every underlying error onto one of these values, defaulting
to `Failed: unreachable` for an unrecognised connection-time error. PostgreSQL driver
errors routinely embed the host, the admin user, and sometimes the full DSN, so the
driver's own message SHALL NOT reach `status`. It MAY be logged, redacted per the
security standards, to give operators a diagnostic path.

#### Scenario: External server reachable with a capable admin

- GIVEN an `external` ManagedDatabase whose admin Secret connects successfully
  and whose admin role has `CREATEDB` and `CREATEROLE`
- WHEN the ManagedDatabaseReconciler processes it
- THEN it SHALL set status `Ready`
- AND SHALL create no Kubernetes resource

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
`database_id` (server-owned; caller value ignored and replaced) by selecting from
the registered `external` ManagedDatabases:

1. Collect all `ManagedDatabase` records with `provider=external`.
2. If **zero** exist, reject the creation with a contextual error.
3. Otherwise, select the ManagedDatabase that was **created first**: order the
   candidates by creation timestamp ascending and take the first. Ties SHALL be
   broken by ID ascending, which is deterministic and, because IDs are
   time-sortable KSUIDs, agrees with creation order.

More than one registered `external` ManagedDatabase is **not** an error. Selection
is deterministic: the same candidate set always yields the same choice, so
concurrent gateway creations agree without coordination.

Selection happens **only at gateway creation**. Once assigned, a gateway's
`database_id` is fixed for its lifetime: registering a further `external`
ManagedDatabase never moves an existing gateway, and the first-created
ManagedDatabase remains the placement target for every new gateway while it exists.
A ManagedDatabase cannot be deleted while any Gateway references it, so the
selected registration is guaranteed to remain resolvable for the gateways placed on
it.

`cluster_id` has no effect on placement in external mode.

> **Why oldest-wins rather than rejecting ambiguity.** An operator may register a
> second external server ahead of a migration, or to document a standby, without
> intending to change where new gateways land. Rejecting gateway creation whenever a
> second registration exists turns a benign inventory action into an outage for
> gateway provisioning. Oldest-wins keeps placement stable and predictable while
> leaving the registry free.

#### Scenario: Gateway placement with a single external database

- GIVEN `DATABASE_PROVIDER=external` and exactly one `external` ManagedDatabase
- WHEN the API server processes the create request
- THEN it SHALL assign that ManagedDatabase's ID as the gateway's `database_id`

#### Scenario: Gateway placement with several external databases picks the oldest

- GIVEN `DATABASE_PROVIDER=external` and three `external` ManagedDatabases
  registered at different times
- WHEN the API server processes the create request
- THEN it SHALL assign the ID of the ManagedDatabase created first
- AND SHALL NOT reject the creation as ambiguous

#### Scenario: A newly registered external database does not move existing gateways

- GIVEN an existing Gateway placed on the first-created `external` ManagedDatabase
- WHEN a further `external` ManagedDatabase is registered
- THEN the existing Gateway's `database_id` SHALL be unchanged
- AND subsequent gateway creations SHALL still select the first-created ManagedDatabase

#### Scenario: Gateway placement finds no external database

- GIVEN `DATABASE_PROVIDER=external` and no `external` ManagedDatabase exists
- WHEN the API server processes the create request
- THEN it SHALL reject the creation with a contextual error and create no gateway

---

### Requirement: Per-Gateway Database Provisioning (External Mode)

In external mode, the GatewayReconciler SHALL provision a dedicated PostgreSQL
database and login role for each gateway by issuing idempotent DDL against the
external server over an admin connection.

Role and database are both named `gw_<gatewayID>` (underscores; PostgreSQL
identifiers avoid hyphens). `<gatewayID>` is the gateway's full resource ID,
lowercased.

For each gateway, the reconciler SHALL:

1. Resolve the gateway's `database_id` to the `external` ManagedDatabase and read
   `hypershell-managed-db-credentials` from its `connection_secret` namespace.
2. Determine the per-gateway password: if the tenant-namespace Secret
   `openshell-gateway-db-credentials` already exists with a `password`, **reuse it**
   (create-or-skip semantics - do not regenerate on re-reconciliation); otherwise
   generate a 32-byte cryptographically random hex password (`crypto/rand`) and treat
   it as authoritative, forcing it onto the role in step 3.
3. Open a short-lived admin connection and reconcile, idempotently:
   - **Role:** if `gw_<gatewayID>` is absent (`SELECT 1 FROM pg_roles ...`), create it
     with `LOGIN` and the password. If the role is present **and** the password was
     reused from an existing tenant Secret, leave it alone. If the role is present but
     the password was newly generated - because the tenant Secret was absent - the
     reconciler SHALL `ALTER ROLE gw_<gatewayID> PASSWORD '<new>'` so the role matches
     the Secret it is about to write. Writing a freshly generated password into the
     tenant Secret without applying it to an existing role produces a gateway that can
     never authenticate and that re-reconciliation would not repair.
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

All DDL SHALL be idempotent: re-running against an already-provisioned gateway SHALL
make no destructive change and SHALL NOT regenerate the password. Every non-benign
SQL error SHALL be propagated with context (never swallowed); credentials SHALL NOT
appear in error text.

> **This is provisioning repair, not credential rotation.** The `ALTER ROLE` in step
> 3 exists solely so a gateway whose tenant Secret was lost can authenticate again.
> External mode has no credential rotation - see Requirement: No Credential Rotation
> (External Mode).

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
via `--db-url $(OPENSHELL_DB_URL)`.

| Key | Value |
|---|---|
| `host` | external server endpoint (from the admin Secret `host`) |
| `port` | external server port (from the admin Secret `port`) |
| `dbname` | `gw_<gatewayID>` |
| `user` | `gw_<gatewayID>` |
| `password` | generated per-gateway password |
| `uri` | `postgresql://gw_<gatewayID>:<password>@<host>:<port>/gw_<gatewayID>?sslmode=<mode>` |
| `sslrootcert` | (optional) inline PEM CA bundle, present when `verify-full` is used |

**TLS:** the connection to a cloud-managed server SHALL be encrypted. Default
`sslmode=require` (encrypt without certificate verification), which needs no extra
files in the tenant namespace. `sslmode=verify-full` is the recommended hardening and
is opt-in: when the admin Secret carries `sslrootcert`, the reconciler SHALL
propagate the CA into the tenant namespace and set `verify-full`, which requires the
gateway workload to mount and reference the CA. Distributing/mounting the CA into the
gateway workload is tracked as a follow-up; v1 MAY ship with `require` as the
enforced default and `verify-full` behind that follow-up.

#### Scenario: Credentials Secret written with the default TLS mode

- GIVEN an `external` ManagedDatabase whose admin Secret carries no `sslrootcert`
- WHEN the GatewayReconciler writes the tenant credentials Secret
- THEN the `uri` SHALL carry `sslmode=require`
- AND no `sslrootcert` key SHALL be written into the tenant namespace

---

### Requirement: Per-Gateway Cleanup

When a Gateway backed by an `external` ManagedDatabase is deleted, the control plane
SHALL destroy that gateway's external database and role. Deletion is
**unconditional**: there is no retention policy, no per-database configuration, and
no operator confirmation.

> **Recovery is the external provider's responsibility.** AWS RDS/Aurora and IBM
> Cloud Databases both provide automated backups and point-in-time recovery, which is
> among the reasons an operator selects a managed offering. HyperShell's drop is not
> the last line of defence against accidental deletion, and HyperShell SHALL NOT
> attempt to be one by retaining orphaned tenant databases on a shared server.

The Gateway delete watch event SHALL carry at least the gateway ID and its
`database_id`, which is all the state cleanup needs: both PostgreSQL object names
derive from the gateway ID, and the admin connection is resolved through the
`database_id`. Because a ManagedDatabase cannot be deleted while any Gateway
references it, the registration is still resolvable at the moment its last gateway
is deleted.

Over an admin connection, the control plane SHALL:

1. Terminate active backends connected to `gw_<gatewayID>` (`pg_terminate_backend`
   over `pg_stat_activity`), so the drop is not blocked by the gateway's own
   lingering connections.
2. `DROP DATABASE gw_<gatewayID>` (guarded by an existence check; `DROP DATABASE`
   cannot run inside a transaction block).
3. `DROP ROLE gw_<gatewayID>`.

Cleanup SHALL be idempotent: an already-absent database or role counts as successful
cleanup, so replaying a delete is safe.

#### Cleanup is best-effort; there is no tombstone or retry queue

External-mode cleanup runs **once**, on the delete event. There is no tombstone
record and no cross-restart retry queue: the gateway is already removed from the API
server, so no later event re-delivers the work.

Consequently, if the external server is unreachable or the drop otherwise fails, the
role and database **persist on the server with valid credentials**. The control plane
SHALL propagate the failure as a contextual error and SHALL log it at error level,
naming the gateway ID and the ManagedDatabase ID (never the credentials), so the
orphan is discoverable. Operators recover with the runbook below.

This is a deliberate trade: unconditional, single-shot deletion keeps the delete path
simple and free of persistent state, at the cost of an operator-visible orphan when
the external server is down at exactly the wrong moment.

#### Scenario: Delete external-backed gateway

- GIVEN a Gateway backed by an `external` ManagedDatabase
- WHEN the Gateway is deleted
- THEN the control plane SHALL terminate active connections to `gw_<gatewayID>`,
  drop the database, then drop the role
- AND SHALL remove the tenant-namespace credentials Secret

#### Scenario: Replay a cleanup whose objects are already gone

- GIVEN a delete event for a Gateway whose external database and role are absent
- WHEN the control plane runs the cleanup
- THEN it SHALL treat both as successfully cleaned up and SHALL NOT return an error

#### Scenario: Cleanup fails while the external server is unreachable

- GIVEN a Gateway delete event whose external server is unreachable
- WHEN the control plane attempts cleanup
- THEN it SHALL propagate a contextual error and log the orphaned gateway ID and
  ManagedDatabase ID at error level
- AND SHALL NOT retry the cleanup on a later event
- AND the role and database SHALL remain on the external server until an operator
  removes them

#### Scenario: Delete external ManagedDatabase leaves the server untouched

- GIVEN an `external` ManagedDatabase with no referencing gateways
- WHEN it is deleted
- THEN the control plane SHALL perform no destructive action on the external server
- AND SHALL create and delete no Kubernetes resource, there being none to reclaim

HyperShell SHALL NEVER drop, resize, or delete the external **server** itself, and
SHALL NEVER delete the `connection_secret` namespace or the credentials Secret inside
it - both are operator-owned.

The tenant-namespace `openshell-gateway-db-credentials` Secret is removed by the
existing label-based tenant-namespace cleanup.

---

### Requirement: No Credential Rotation (External Mode)

External mode SHALL NOT implement database credential rotation. The
`hypershell.redhat.io/rotate-db-credentials` annotation SHALL have **no effect** on a
gateway backed by an `external` ManagedDatabase: the GatewayReconciler SHALL NOT
generate a new password, SHALL NOT issue `ALTER ROLE` in response to it, and SHALL
NOT write `hypershell.redhat.io/last-db-rotation`.

An operator who must change a gateway's database password does so out-of-band on the
external server, or by deleting and recreating the gateway.

This is a deliberate scope reduction, not an omission. The `ALTER ROLE` in
Requirement: Per-Gateway Database Provisioning is the provisioning repair path for a
lost tenant Secret and SHALL be retained; it is not reachable from the rotation
annotation.

#### Scenario: Rotation annotation on an external-backed gateway is inert

- GIVEN a Gateway backed by an `external` ManagedDatabase
- AND a new value for the annotation `hypershell.redhat.io/rotate-db-credentials`
- WHEN the GatewayReconciler processes the event
- THEN it SHALL NOT generate a new password
- AND SHALL NOT alter the role on the external server
- AND the tenant-namespace `openshell-gateway-db-credentials` Secret SHALL be unchanged

---

### Requirement: Database Credential Security (External Mode)

- Admin credentials are held only for the duration of a reconciliation and never
  persisted by HyperShell beyond the operator-owned Secret they were read from.
- Per-gateway passwords SHALL be generated with `crypto/rand` (32-byte hex),
  create-or-skip on re-reconciliation, and never logged.
- Passwords SHALL NEVER appear in log messages, error strings, telemetry, or API
  responses.
- The connection to the external server SHOULD always be TLS-encrypted (minimum
  `sslmode=require`). `sslmode=disable` is insecure and SHOULD NOT be used in
  production. The control plane emits a WARN log when `sslmode=disable` is read from
  the admin Secret so operators see the misconfiguration without the reconciler
  failing. Development and CI environments that use a local PostgreSQL server without
  TLS may set `sslmode=disable`; this is explicitly not recommended for any external
  server reachable from outside the cluster.

#### Scenario: Insecure TLS mode is warned about, not rejected

- GIVEN an `external` ManagedDatabase whose admin Secret sets `sslmode: disable`
- WHEN the ManagedDatabaseReconciler reads it
- THEN it SHALL emit a WARN log naming the ManagedDatabase
- AND SHALL proceed with the connectivity check rather than failing the resource

---

### Requirement: DATABASE_PROVIDER Selection (External)

`DATABASE_PROVIDER=external` selects external-backed placement and per-gateway DDL.
It requires a reachable external server and valid credentials at **reconcile** time;
it imposes no startup precondition beyond reading the value itself.

The full set of valid `DATABASE_PROVIDER` values, and the requirement that any other
value is a fatal startup configuration error, is defined once in the parent spec's
Requirement: DATABASE_PROVIDER Selection And Validation and is not restated here.

#### Scenario: DATABASE_PROVIDER=external selects external placement

- GIVEN `DATABASE_PROVIDER=external`
- WHEN the API server or control plane starts
- THEN it SHALL select external-backed placement
- AND SHALL start without contacting any external server

---

## Configuration Reference

| Variable | Type | Default | Description |
|---|---|---|---|
| `DATABASE_PROVIDER` | env var | `deployment` | Set to `external` to select this provider. |

No new environment variables are introduced. The external server endpoint,
credentials, and TLS material all live in the `hypershell-managed-db-credentials`
Secret inside the namespace named by `connection_secret`.
`OPENSHELL_DATABASE_IMAGE` is not used in external mode, which runs no in-cluster
PostgreSQL workload.

---

## Configuration Examples

Credentials namespace and Secret (created out-of-band by the operator, before
HyperShell is installed; referenced as
`connection_secret: "hypershell-managed-db-us-east-1"`):

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: hypershell-managed-db-us-east-1
---
apiVersion: v1
kind: Secret
metadata:
  name: hypershell-managed-db-credentials   # fixed name
  namespace: hypershell-managed-db-us-east-1
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

## Operator Runbook: identifying and recovering orphaned objects

Per-gateway cleanup is best-effort and single-shot (see Requirement: Per-Gateway
Cleanup). If the external server was unreachable when a gateway was deleted, its role
and database persist with valid credentials. Detect and recover them as follows.

1. **Identify orphaned databases** - connect as the admin user and query:
   ```sql
   SELECT datname FROM pg_database WHERE datname LIKE 'gw\_%';
   ```
   Compare the result to the gateway IDs currently registered in HyperShell. Any
   `gw_<id>` database whose gateway ID is no longer in HyperShell is orphaned.
   (`_` is a `LIKE` wildcard, hence the backslash escape.)

2. **Identify orphaned roles** - similarly:
   ```sql
   SELECT rolname FROM pg_roles WHERE rolname LIKE 'gw\_%';
   ```

3. **Terminate, then drop** - for each orphaned object:
   ```sql
   SELECT pg_terminate_backend(pid)
     FROM pg_stat_activity WHERE datname = 'gw_<id>';
   DROP DATABASE "gw_<id>";
   DROP ROLE "gw_<id>";
   ```

4. **Tenant Secret** - `openshell-gateway-db-credentials` in the gateway's tenant
   namespace is removed by the platform's label-based namespace cleanup when the
   gateway namespace is reclaimed. If the namespace was already deleted, the Secret is
   gone. If it persists, delete it manually.

---

## Debugging Reference

| Symptom | Root Cause | Fix |
|---|---|---|
| ManagedDatabase status `Failed: secret_invalid` | `connection_secret` does not carry the `hypershell-managed-db-` prefix, the namespace does not exist, or it holds no `hypershell-managed-db-credentials` Secret | Correct the reference, or create the namespace and Secret with the fixed name |
| ManagedDatabase status `Failed: unreachable` | No network path from control-plane cluster to endpoint | Fix VPC peering / security groups / private endpoint |
| ManagedDatabase status `Failed: auth_failed` | Wrong admin credentials in the credentials Secret | Correct `user`/`password` in `hypershell-managed-db-credentials` |
| ManagedDatabase status `Failed: insufficient_privilege` | Admin role lacks CREATEDB/CREATEROLE | Grant `rds_superuser` (AWS) / admin role (IBM) or the two privileges |
| ManagedDatabase status `Failed: tls_failed` | `sslmode` requires verification but `sslrootcert` is missing, wrong, or does not match the server certificate | Supply the provider's CA bundle as inline PEM, or lower `sslmode` to `require` |
| Gateway create rejected: no eligible external database | No `external` ManagedDatabase is registered | Register an `external` ManagedDatabase |
| New gateways land on an unexpected external server | Placement selects the **first-created** `external` ManagedDatabase, not the most recent | Check registration timestamps; delete the older registration once its gateways are gone |
| Gateway pod cannot connect | TLS mismatch or wrong host in tenant Secret | Verify `sslmode`/CA and `openshell-gateway-db-credentials` |
| `gw_*` database or role left on the server after gateway deletion | Cleanup ran while the server was unreachable; there is no retry | Follow the Operator Runbook above |

---

## Affected Components (Implementation Impact)

Spec-driven summary of the code surface a `/reconcile` wave would touch (for
planning only):

- **API server - ManagedDatabase**
  - `plugins/managedDatabases/service.go` - add `external` to supported providers;
    require `connection_secret` and enforce the reference format (no `/`, reserved
    `hypershell-managed-db-` prefix, DNS-1123 label); keep provider immutable.
  - **No data-model change.** `provider`, `region`, `engine`, `engine_version`,
    `instance_class` and `connection_secret` already exist in `model.go`,
    `migration.go`, `openapi.managedDatabases.yaml` and
    `proto/hypershell/v1/managed_databases.proto`. No new column, no migration, no
    regenerated stubs.
  - **Naming debt (known, deferred):** `connection_secret` now holds a *namespace*
    name, not a Secret name. The field, the CLI flag `--connection-secret` and the
    OpenAPI/proto field keep their existing names to avoid a migration; the
    description in each SHALL state that the value is a namespace.
- **API server - Gateway placement**
  - `plugins/gateways/provider.go`, `plugin.go`, `placement.go` - accept
    `DATABASE_PROVIDER=external`; `externalPlacement` queries `provider=external`
    ordered by creation timestamp then ID ascending, takes the first, and rejects
    only the empty result. No cluster/region lookup.
- **Control plane**
  - Add a PostgreSQL client dependency and an admin-connection helper (per-reconcile,
    short-lived, credentials read as `hypershell-managed-db-credentials` from the
    `connection_secret` namespace).
  - `internal/config/config.go` - accept `external` value.
  - `internal/reconciler/reconciler.go` - `ManagedDatabaseReconciler`: `external`
    branch = connectivity/capability check creating no Kubernetes resource;
    `GatewayReconciler`: external per-gateway DDL, tenant credentials Secret,
    unconditional single-shot deletion.
  - The Gateway delete watch event must carry `database_id` alongside the gateway ID.
  - **Removals:** any external-mode credential-rotation branch, and any use of
    ManagedDatabase delete tombstones or a retry queue for external per-gateway
    cleanup. The `ALTER ROLE` in the provisioning repair path is retained.
- **Related specs (amended alongside this one)**
  - `openshell-gateway-database.spec.md` - external placement description and admin
    workflow row.
  - `naming-multitenancy.spec.md` - `hypershell-managed-db-<name>` registered as a
    reserved **Namespace** prefix, with the fixed `hypershell-managed-db-credentials`
    Secret inside it.
  - `data-model.spec.md` - `connection_secret` documented as a namespace reference.
  - `global-architecture.spec.md` - § Database Strategy already covers `external`.

---

## References

- Parent: [`openshell-gateway-database.spec.md`](./openshell-gateway-database.spec.md)
- [`security.spec.md`](../standards/security/security.spec.md) - secret references, not inline secrets
- [`naming-multitenancy.spec.md`](../standards/platform/naming-multitenancy.spec.md) - reserved names
- [`control-plane/conventions.spec.md`](../standards/control-plane/conventions.spec.md) - reconciler error handling, no panic
- [AWS RDS PostgreSQL - master user privileges (`rds_superuser`)](https://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/CHAP_PostgreSQL.html)
- [IBM Cloud Databases for PostgreSQL - administration](https://cloud.ibm.com/docs/databases-for-postgresql)
- [PostgreSQL - `CREATE DATABASE`, `CREATE ROLE`, `GRANT`/`REVOKE`](https://www.postgresql.org/docs/current/sql-createdatabase.html)
