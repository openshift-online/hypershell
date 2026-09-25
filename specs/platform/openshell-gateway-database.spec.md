# OpenShell Gateway Database Specification

**Date:** 2026-09-16
**Status:** Active
**Parent:** `openshell-gateway.spec.md` - core gateway provisioning

---

## Purpose

This specification defines PostgreSQL database provisioning for OpenShell gateways.

PostgreSQL server infrastructure is **provisioned outside HyperShell** - a
cloud-managed database (AWS RDS / Aurora for PostgreSQL, IBM Cloud Databases for
PostgreSQL) or any other PostgreSQL server created out-of-band by a platform team or
IaC. HyperShell does **not** create, resize, or delete the server. Each HyperShell
installation is given **one** administrative credential set for that server, mounted
into the control plane as a Kubernetes Secret, and the control plane provisions
**one dedicated PostgreSQL database and one dedicated login role per gateway** on it,
so each gateway is isolated from every other gateway sharing the server.

There is no database resource in the HyperShell API. Gateways carry no database
reference, gateway creation performs no database placement, and neither the REST API,
the gRPC API, the SDKs nor the CLI expose anything about databases. The database
server is deployment configuration of the control plane, not a domain object.

The control plane issues the `CREATE DATABASE` / `CREATE ROLE` / `GRANT` statements
itself, over a short-lived administrative connection. No operator is required and no
in-cluster PostgreSQL workload is created. There is a single provisioning model: the
control plane reads no setting that selects an alternative.

PostgreSQL is the only supported database backend for HyperShell gateways.

### Contracts

- **`hypershell-gateway-database-admin`** - the Secret in the HyperShell namespace
  holding the administrative connection to the gateway database server. Mounted
  read-only into the controller Deployment (see Requirement: Admin Credential Mount).
- **`GATEWAY_DATABASE_ADMIN_DIR`** - the only environment variable the control plane
  reads for gateway databases: the directory the admin Secret is mounted at (default
  `/etc/hypershell/gateway-database`).
- **`openshell-gateway-db-credentials`** - the Secret in the gateway's tenant
  namespace whose `uri` key the upstream OpenShell Helm chart injects into the
  gateway workload as `OPENSHELL_DB_URL` (`server.externalDbSecret`).

---

## Prerequisites

The following are the platform administrator's responsibility, out-of-band, before
the control plane is deployed:

1. **A running PostgreSQL server** (for example AWS RDS/Aurora or IBM Cloud Databases
   for PostgreSQL) reachable from the control-plane cluster **and** from every cluster
   that runs gateway workloads.
2. **Network egress / reachability.** The control-plane cluster and the gateway
   clusters SHALL have a network path to the server endpoint (VPC peering, private
   endpoint/service endpoint, security-group / ACL allow rules). Loss of reachability
   is a provisioning failure that is retried, not a silent skip.
3. **TLS on the server, with a CA the operator can distribute.** The server SHALL
   present a certificate whose hostname matches the `host` the control plane
   connects to, issued by a CA whose PEM bundle the operator places in the admin
   Secret. Cloud providers publish their bundles (AWS RDS global bundle, IBM Cloud
   Databases CA). The control plane's own admin connection always verifies the
   server certificate and hostname (`sslmode=verify-full`) and offers no
   downgrade. The gateway workload's own connection is capped at
   `sslmode=require` because the Helm chart cannot mount a database CA into the
   gateway pod (see Requirement: Tenant Credentials Secret).
4. **An administrative role** on the server with `CREATEDB`, `CREATEROLE` and
   membership in `pg_signal_backend` (to terminate a gateway's sessions before its
   database is dropped). Full superuser is **not** required; the AWS RDS
   `rds_superuser` role and the IBM Cloud Databases administrative user are both
   acceptable. The admin role does not need, and SHOULD NOT be granted, the ability
   to alter server-level configuration.
5. **The admin Secret** `hypershell-gateway-database-admin` in the HyperShell
   namespace (see Requirement: Admin Credential Mount). It is created by the
   deployment (dev scripts, a secret-manager sync such as the optional External
   Secrets component, or the operator by hand), never by the control plane.
6. **Server-side log verbosity restricted.** `CREATE ROLE` and `ALTER ROLE` statements
   carry the plaintext password in the statement text (the PostgreSQL wire protocol
   has no separate credential-binding channel for these statements). Operators SHOULD
   set `log_statement` to `'mod'` or lower, or enable server-side log redaction, on
   the server. HyperShell redacts credentials in its own application logs and error
   messages; server-side redaction is the operator's responsibility and HyperShell
   cannot enforce it.

### Assumption: one HyperShell install per server

Per-gateway database and role names are `gw_<gatewayID>` (KSUID-unique) with no
install-scoped prefix. Sharing one PostgreSQL server across multiple HyperShell
installs is **out of scope** for this spec; doing so risks name collisions and is not
supported. One install has exactly one gateway database server.

---

## Architecture

```
Platform team (out of band)
  ├── PostgreSQL server (AWS RDS / IBM Cloud Databases / ...)  (endpoint + admin user + CA)
  └── Secret hypershell-gateway-database-admin  (HyperShell namespace)
        host / port / user / password / sslrootcert [/ dbname / sslmode=verify-full]
              │ mounted read-only at /etc/hypershell/gateway-database
              ▼
      hypershell-controller Deployment
        ├── startup: validate the mounted files, do NOT connect
        └── GatewayReconciler (per gateway): re-read files, short-lived admin connection
              ▼
PostgreSQL server:
  ├── ROLE     gw_<gatewayID>  (LOGIN, owns its database)
  └── DATABASE gw_<gatewayID>  (owner gw_<gatewayID>; CONNECT revoked from PUBLIC)

  └── Secret openshell-gateway-db-credentials (tenant namespace)
        uri (sslmode=require)  →  Helm server.externalDbSecret  →  OPENSHELL_DB_URL
```

### DDL execution: in-process, in the control plane

The control plane issues DDL **in-process** using a PostgreSQL client, opening a
short-lived admin connection per database operation and closing it afterward. It does
**not** maintain a long-lived connection pool and does **not** launch Kubernetes Jobs
to run SQL.

Rationale: per-gateway provisioning is a database operation, not a Kubernetes one.
In-process execution gives synchronous, structured errors (required by the control
plane conventions - `fmt.Errorf` with context, status set on every error path),
trivially idempotent reconcile (query `pg_database`/`pg_roles`, then act), clean
status transitions, and testability against a throwaway PostgreSQL
(testcontainers). A Job-based approach would reintroduce the asynchronous
log/exit-code parsing that the gRPC-watch reconciler pattern avoids.

### Why a mounted Secret and not an API resource

The previous design registered servers through the API and stored a Secret
reference per registration. That required a reserved namespace prefix and a fixed
Secret name to stop an API-level reference from steering the control plane at an
unrelated Secret, a placement step at gateway creation, a foreign key on Gateway, a
status vocabulary, deletion protection, and inventory metrics - all for a value that
is fixed per installation. Mounting the admin Secret into the controller removes
that whole surface: whoever can deploy the controller chooses the server, no API
caller can name a Secret, and there is nothing to place, protect or count.

---

## Requirements

### Requirement: Admin Credential Mount

The controller Deployment SHALL mount a Secret holding the gateway database server's
administrative connection read-only at the directory named by
`GATEWAY_DATABASE_ADMIN_DIR` (default `/etc/hypershell/gateway-database`). The
shipped manifests SHALL mount the Secret named `hypershell-gateway-database-admin`
from the HyperShell namespace; the Secret name is deployment configuration and MAY
be changed in an overlay without any code change.

Each Secret key becomes a file in that directory:

| File | Required | Meaning |
|---|---|---|
| `host` | yes | Server hostname/endpoint. Also the name the server certificate is verified against |
| `port` | yes | Server port, an integer in `1`-`65535` (typically `5432`) |
| `user` | yes | Admin role with `CREATEDB`, `CREATEROLE` and `pg_signal_backend` |
| `password` | yes | Admin role password |
| `sslrootcert` | yes | PEM CA bundle (one or more certificates) used to verify the server certificate. Operators supply PEM text, never a path |
| `dbname` | no | Maintenance database the admin connection opens (default `postgres`). Per-gateway DDL runs from this connection |
| `sslmode` | no | If present, MUST be `verify-full`. Any other value is a startup error |

The effective TLS mode of the admin connection SHALL always be `verify-full`: the
server certificate SHALL be verified against `sslrootcert` and its hostname SHALL be
verified against `host`. There is no configuration that lowers it.

The control plane SHALL re-read the mounted files for **every** database operation
(provisioning and cleanup) rather than caching their contents. The kubelet refreshes
mounted Secrets in place, so an admin password change takes effect on the next
operation without a controller restart.

Admin credential values SHALL NEVER appear in logs, error strings, telemetry, Events,
or API responses. Error messages MAY name the file that failed validation and the
directory path; they SHALL NOT include the file contents.

#### Scenario: Password changed in the mounted Secret

- GIVEN a running control plane and an admin Secret whose `password` is updated
- WHEN the kubelet refreshes the mounted files
- AND the GatewayReconciler next provisions or cleans up a gateway database
- THEN it SHALL authenticate with the new password
- AND no controller restart SHALL be required

#### Scenario: Overlay renames the admin Secret

- GIVEN an overlay that mounts a Secret with a different name at
  `GATEWAY_DATABASE_ADMIN_DIR`
- WHEN the control plane starts
- THEN it SHALL read the credential files from that directory
- AND SHALL NOT look up any Secret by name through the Kubernetes API

---

### Requirement: Startup Precondition

The control plane SHALL validate the admin credential directory before it serves any
watch stream, and SHALL refuse to start - log a fatal error naming the failing file
(never its contents) and exit non-zero - when any of the following holds:

1. The directory does not exist or is not readable.
2. Any required file (`host`, `port`, `user`, `password`, `sslrootcert`) is missing
   or empty after trimming whitespace.
3. `sslrootcert` does not contain at least one parseable PEM certificate.
4. `port` is not an integer in `1`-`65535`.
5. `sslmode` is present and is anything other than `verify-full`.

Validation is **file-only**. The control plane SHALL NOT open a connection to the
server at startup: server reachability, authentication and admin privileges are
checked at reconcile time, per gateway, with the reconcile queue's retries. A server
that is down when the controller starts delays gateway provisioning; it does not
prevent the controller from running.

> **Why fail fast on files but not on connectivity.** A missing or malformed
> credential file is a deployment error that no retry will fix, and every gateway
> reconcile would fail the same way; exiting makes the misconfiguration visible as a
> crash-looping Deployment. An unreachable server is a runtime condition that
> retries do fix, and tying controller liveness to it would take down namespace GC,
> Keycloak reconciliation and every other controller duty for a database outage.

#### Scenario: Required file missing

- GIVEN the admin Secret lacks a `sslrootcert` key
- WHEN the control plane starts
- THEN it SHALL log a fatal error naming `sslrootcert` and the directory
- AND SHALL exit non-zero before serving any watch stream

#### Scenario: sslmode set to a weaker value

- GIVEN the admin Secret carries `sslmode: require`
- WHEN the control plane starts
- THEN it SHALL log a fatal error stating that `sslmode` must be `verify-full`
- AND SHALL exit non-zero

#### Scenario: CA bundle is not PEM

- GIVEN `sslrootcert` contains text that yields no certificate when parsed as PEM
- WHEN the control plane starts
- THEN it SHALL log a fatal error naming `sslrootcert`
- AND SHALL exit non-zero

#### Scenario: Server unreachable at startup

- GIVEN valid credential files and a server that is not reachable
- WHEN the control plane starts
- THEN it SHALL start normally and serve watch streams
- AND the first gateway reconcile that needs the database SHALL fail with a
  contextual error and be retried

---

### Requirement: Per-Gateway Database Provisioning

The GatewayReconciler SHALL provision a dedicated PostgreSQL database and login role
for each gateway by issuing idempotent DDL against the server over an admin
connection opened from the mounted credentials.

Role and database are both named `gw_<gatewayID>` (underscores; PostgreSQL identifiers
avoid hyphens). `<gatewayID>` is the gateway's full resource ID, lowercased.

For each gateway, the reconciler SHALL:

1. Re-read the admin credential files (see Requirement: Admin Credential Mount).
2. Determine the per-gateway password: if the tenant-namespace Secret
   `openshell-gateway-db-credentials` already exists with a `password`, **reuse it**
   (do not regenerate on re-reconciliation); otherwise generate a 32-byte
   cryptographically random hex password (`crypto/rand`) and treat it as
   authoritative, forcing it onto the role in step 3.
3. Open a short-lived admin connection and reconcile, idempotently:
   - **Role:** if `gw_<gatewayID>` is absent (`SELECT 1 FROM pg_roles ...`), create it
     with `LOGIN` and the password. If the role is present **and** the password was
     reused from an existing tenant Secret, leave it alone. If the role is present but
     the password was newly generated - because the tenant Secret was absent - the
     reconciler SHALL `ALTER ROLE gw_<gatewayID> PASSWORD '<new>'` so the role matches
     the Secret it is about to write. Writing a freshly generated password into the
     tenant Secret without applying it to an existing role produces a gateway that can
     never authenticate and that re-reconciliation would not repair.
   - **Ownership grant:** `GRANT gw_<gatewayID> TO <admin user>`. A non-superuser
     admin can only create a database owned by a role it is a member of, so this
     grant is what lets `CREATEDB` + `CREATEROLE` suffice without superuser. It is
     idempotent (re-granting an existing membership is a no-op notice).
   - **Database:** if `gw_<gatewayID>` is absent (`SELECT 1 FROM pg_database ...`),
     create it with `OWNER gw_<gatewayID> TEMPLATE template0`. Copying `template0`
     (not the default `template1`) means concurrent `CREATE DATABASE` from other
     gateways, autovacuum, and leftover sessions on `template1` cannot fail
     provisioning with SQLSTATE 55006 ("source database is being accessed by
     other users"). (`CREATE DATABASE` cannot run inside a transaction block and
     has no `IF NOT EXISTS`; the reconciler SHALL guard it with an existence
     check rather than relying on catching an error.) If a source-busy error
     still occurs, the reconciler SHALL retry the statement.
   - **Isolation:** `REVOKE CONNECT ON DATABASE gw_<gatewayID> FROM PUBLIC` and
     `GRANT CONNECT ON DATABASE gw_<gatewayID> TO gw_<gatewayID>`, so no other
     gateway's role can connect.
4. When this pass established new credentials or a new database (the role was
   created, its password was re-synced, or the database was created), verify them by
   opening a short-lived connection **as the gateway role to the gateway database**
   with the same TLS posture the gateway workload uses (`sslmode=require`) and
   pinging it. DDL returning no error does not prove the role can log in: a wrong
   password, a missing `CONNECT` grant, or a database not yet accepting connections
   surface only on a real login. The probe SHALL retry with exponential backoff to
   absorb the brief window right after `CREATE DATABASE` before failing, and the whole
   probe (all attempts plus per-attempt connect timeouts) SHALL be bounded by a total
   wall-clock cap so a black-holed database host cannot pin the reconcile worker. A
   probe that never succeeds is a retryable provisioning failure handled like any connection
   failure below; the probe error SHALL be categorized (`unreachable` / `tls_failed`
   / `auth_failed`) and SHALL NOT contain the password or DSN. A steady-state
   reconcile that changes nothing SHALL NOT perform this login, so an
   already-provisioned gateway is not re-verified on every reconcile.
5. Write/refresh the tenant-namespace Secret `openshell-gateway-db-credentials` (see
   Requirement: Gateway Credentials Secret). The reconciler SHALL verify (step 4)
   before writing the Secret so `DatabaseReady` is never reported for credentials
   that cannot connect.
6. Proceed to deploy the gateway workload only after DDL, the connection probe and
   the credentials Secret succeed.

All DDL SHALL be idempotent: re-running against an already-provisioned gateway SHALL
make no destructive change and SHALL NOT regenerate the password. Every non-benign SQL
error SHALL be propagated with context (never swallowed); credentials SHALL NOT appear
in error text. A connection, authentication or privilege failure is a retryable
provisioning error: the reconciler SHALL set the `DatabaseReady` condition to
`Failed` with a user-facing message (see
[`gateway-provisioning-progress.spec.md`](./gateway-provisioning-progress.spec.md))
and return the error to the reconcile queue.

> **This is provisioning repair, not credential rotation.** The `ALTER ROLE` in step 3
> exists solely so a gateway whose tenant Secret was lost can authenticate again.
> HyperShell does not rotate gateway database credentials - see Requirement: No
> Credential Rotation.

#### Scenario: New gateway provisioned

- GIVEN a new Gateway
- WHEN the GatewayReconciler processes the event
- THEN it SHALL create role and database `gw_<gatewayID>` on the server if absent
- AND grant `gw_<gatewayID>` to the admin user before creating the database
- AND revoke `CONNECT` from `PUBLIC` on that database and grant it to `gw_<gatewayID>`
- AND verify the gateway role can connect to the gateway database before writing the Secret
- AND write `openshell-gateway-db-credentials` into the tenant namespace
- AND proceed to deploy the gateway workload

#### Scenario: Re-reconcile an already-provisioned gateway

- GIVEN a Gateway whose role and database already exist
- WHEN the GatewayReconciler re-processes the event
- THEN it SHALL detect both exist and make no destructive change
- AND SHALL NOT regenerate or alter the password

#### Scenario: Tenant credentials Secret lost while the role still exists

- GIVEN a Gateway whose role `gw_<gatewayID>` exists on the server
- AND whose tenant-namespace `openshell-gateway-db-credentials` Secret is absent (for
  example after tenant-namespace garbage collection or a cluster rebuild)
- WHEN the GatewayReconciler processes the event
- THEN it SHALL generate a new password, apply it to the existing role with
  `ALTER ROLE`, and write the matching tenant Secret
- AND the gateway SHALL be able to authenticate without operator intervention

#### Scenario: Non-superuser admin creates the database

- GIVEN an admin role that has `CREATEDB` and `CREATEROLE` but is not a superuser
- WHEN the GatewayReconciler provisions a new gateway
- THEN `CREATE DATABASE gw_<gatewayID> OWNER gw_<gatewayID> TEMPLATE template0` SHALL succeed because
  the admin was granted membership in `gw_<gatewayID>` first

#### Scenario: Template1 is occupied during CREATE DATABASE

- GIVEN a session is connected to `template1` (another gateway's `CREATE DATABASE`,
  autovacuum, or a leftover backend)
- WHEN the GatewayReconciler provisions a new gateway
- THEN `CREATE DATABASE ... TEMPLATE template0` SHALL succeed
- AND provisioning SHALL NOT fail with SQLSTATE 55006

#### Scenario: Server unreachable during gateway provisioning

- GIVEN a Gateway whose database server is unreachable
- WHEN the GatewayReconciler attempts DDL
- THEN it SHALL return a contextual error without credentials in its text
- AND set `DatabaseReady` to `Failed` with a user-facing message
- AND SHALL NOT create the gateway workload
- AND the reconcile queue SHALL retry

#### Scenario: Admin role lacks required privileges

- GIVEN an admin role without `CREATEDB` or `CREATEROLE`
- WHEN the GatewayReconciler attempts to create the role or database
- THEN the SQL permission error SHALL be propagated with context and retried
- AND the gateway workload SHALL NOT be deployed

---

### Requirement: Gateway Credentials Secret

After per-gateway DDL, the GatewayReconciler SHALL ensure the tenant-namespace Secret
`openshell-gateway-db-credentials` exists with exactly these keys:

| Key | Value |
|---|---|
| `host` | server endpoint (from the admin `host` file) |
| `port` | server port (from the admin `port` file) |
| `dbname` | `gw_<gatewayID>` |
| `user` | `gw_<gatewayID>` |
| `password` | generated per-gateway password |
| `sslmode` | `require` (constant) |
| `uri` | `postgresql://gw_<gatewayID>:<password>@<host>:<port>/gw_<gatewayID>?sslmode=require` |

The Secret SHALL NOT carry an `sslrootcert` key.

The gateway workload is deployed through the upstream OpenShell Helm chart. The
control plane SHALL point the chart at this Secret with
`server.externalDbSecret`, and the chart injects the Secret's `uri` key into the
gateway container as `OPENSHELL_DB_URL` (see
[`openshell-gateway.spec.md`](./openshell-gateway.spec.md) and
[`openshell-gateway-helm-adoption.spec.md`](./openshell-gateway-helm-adoption.spec.md)).

The chart reads **only** the `uri` key of this Secret and provides no mechanism to
mount an additional CA file into the gateway pod for the database connection; its
`caConfigMapName` values cover the OIDC issuer and the Vault credential driver
only. The gateway connection is therefore capped at `sslmode=require`: the
transport is encrypted, but the server certificate and hostname are **not**
verified by the gateway. This is a deliberate, bounded deviation from the admin
connection, which reads its CA from a file the control plane mounts itself and
stays `verify-full`. Raising the gateway connection to `verify-full` requires an
upstream chart mechanism for a database CA volume.

The tenant Secret SHALL carry only the gateway's own role and database. Admin
`user` and `password` SHALL NEVER be written into a tenant namespace. The tenant
Secret SHALL carry the management label `hypershell.redhat.io/managed: "true"` so
tenant-namespace cleanup reclaims it.

The `sslmode` in the tenant Secret and in `uri` SHALL always be `require`. There is
no configuration, annotation or environment variable that lowers it further, and
none that raises it while the chart cannot mount a database CA.

#### Scenario: Credentials Secret written

- GIVEN a provisioned role and database for a Gateway
- WHEN the GatewayReconciler writes the tenant credentials Secret
- THEN the `uri` SHALL carry `sslmode=require`
- AND the Secret SHALL have no `sslrootcert` key
- AND the Secret SHALL contain no admin `user` or `password`

#### Scenario: Gateway connects over an encrypted connection

- GIVEN a gateway workload deployed by the Helm chart for a provisioned Gateway
- WHEN the gateway pod starts
- THEN `OPENSHELL_DB_URL` SHALL be sourced from the Secret's `uri` key
- AND the connection SHALL negotiate TLS because `sslmode=require`
- AND a server that refuses TLS SHALL make the connection fail rather than fall
  back to plaintext

---

### Requirement: CA Bundle Rotation

When the server's CA changes, the operator SHALL update the admin Secret's
`sslrootcert` to a bundle containing **both** the old and the new CA for the
transition period, then remove the old CA once the server no longer presents the
old chain.

The control plane re-reads the mounted admin files on every database operation, so
a rotated bundle takes effect without a controller restart. Because the tenant
Secret carries no CA bundle, a CA rotation SHALL NOT require rewriting tenant
Secrets, regenerating passwords or restarting gateway Deployments.

#### Scenario: Admin bundle updated

- GIVEN a running control plane provisioning gateway databases
- AND the admin Secret's `sslrootcert` now contains the old and the new CA
- WHEN the GatewayReconciler next performs a database operation
- THEN it SHALL verify the server against the updated bundle without a restart
- AND SHALL leave tenant Secrets, passwords and gateway Deployments untouched
- AND SHALL NOT alter any role on the server

---

### Requirement: Gateway Workload Type

The gateway workload SHALL always be deployed as a Deployment (not a StatefulSet). The
gateway workload does not require persistent local storage; its data lives in its
per-gateway database on the server.

---

### Requirement: Per-Gateway Cleanup

When a Gateway is deleted, the control plane SHALL destroy that gateway's database and
role. Deletion is **unconditional**: there is no retention policy, no per-database
configuration, and no operator confirmation.

> **Recovery is the server owner's responsibility.** AWS RDS/Aurora and IBM Cloud
> Databases both provide automated backups and point-in-time recovery, which is among
> the reasons an operator selects a managed offering. HyperShell's drop is not the last
> line of defence against accidental deletion, and HyperShell SHALL NOT attempt to be
> one by retaining orphaned tenant databases on a shared server.

The Gateway delete watch event carries the gateway ID, which is all the state cleanup
needs: both PostgreSQL object names derive from it, and the admin connection comes
from the mounted credentials.

Over an admin connection opened from freshly re-read credentials, the control plane
SHALL:

1. Terminate active backends connected to `gw_<gatewayID>` (`pg_terminate_backend` over
   `pg_stat_activity`; this is why the admin role needs `pg_signal_backend`), so the
   drop is not blocked by the gateway's own lingering connections.
2. `DROP DATABASE gw_<gatewayID> WITH (FORCE)` (guarded by an existence check;
   `DROP DATABASE` cannot run inside a transaction block).
3. `DROP ROLE gw_<gatewayID>`.

Cleanup SHALL be idempotent: an already-absent database or role counts as successful
cleanup, so replaying a delete is safe.

Resources in the gateway's tenant namespace (including
`openshell-gateway-db-credentials`) are removed by the existing label-based tenant
namespace cleanup (`hypershell.redhat.io/managed: "true"`).

#### Failure handling: retry plus a durable signal

Per-gateway database cleanup is a **required-inline** class in
[`gateway-deletion-finalization.spec.md`](./gateway-deletion-finalization.spec.md):
there is no sweep that reclaims a leftover database later. When the existence query,
the connection, the backend termination or either `DROP` fails, the control plane
SHALL:

1. Return a contextual error (no credentials in its text) so the Gateway delete event
   is retried by the reconcile queue with bounded backoff; and
2. Record an `IncompleteFinalization` Warning Event through the orphan recorder, with
   resource kind `PostgreSQLDatabase` and resource name `gw_<gatewayID>`, in the
   control-plane namespace, naming the reason without secrets.

The Event SHALL be recorded on the **first** failure, not only after retries are
exhausted, because the reconcile queue is in-memory: if the controller restarts while
a retry is pending, the pending retry is lost and the API server does not re-deliver
the delete event for an already soft-deleted row. The Event is therefore the durable
record that an operator must check the server, and the runbook below is the
recovery path. A retry that later succeeds leaves the earlier Event in place; the
Event says a leftover **may** exist, and step 1 of the runbook confirms whether it
does.

#### Scenario: Delete gateway

- GIVEN a Gateway with no active sandboxes
- WHEN the Gateway is deleted
- THEN the control plane SHALL terminate active connections to `gw_<gatewayID>`, drop
  the database with `FORCE`, then drop the role
- AND SHALL delete all resources with label `hypershell.redhat.io/managed: "true"` from
  the tenant namespace, including the credentials Secret

#### Scenario: Replay a cleanup whose objects are already gone

- GIVEN a delete event for a Gateway whose database and role are absent
- WHEN the control plane runs the cleanup
- THEN it SHALL treat both as successfully cleaned up and SHALL NOT return an error
- AND SHALL NOT record an `IncompleteFinalization` Event

#### Scenario: Cleanup fails while the server is unreachable

- GIVEN a Gateway delete event whose database server is unreachable
- WHEN the control plane attempts cleanup
- THEN it SHALL return a contextual error so the delete is retried
- AND it SHALL record an `IncompleteFinalization` Warning Event with kind
  `PostgreSQLDatabase` and name `gw_<gatewayID>` in the control-plane namespace
- AND the Event SHALL NOT contain the admin credentials or the connection string
- AND the role and database SHALL remain on the server until a retry succeeds or an
  operator removes them

#### Scenario: Controller restarts with a cleanup retry pending

- GIVEN a Gateway delete whose database cleanup failed and recorded an Event
- WHEN the controller restarts before the retry runs
- THEN the pending retry is lost with the in-memory queue
- AND the `IncompleteFinalization` Event from the failed attempt SHALL still exist
- AND the operator runbook SHALL be the recovery path for the leftover objects

---

### Requirement: Gateway Deletion With Active Sandboxes (Advisory)

Active sandboxes SHALL NOT block Gateway deletion. Before an operator deletes a
Gateway, the active sandbox count is surfaced as a warning so they can see how many
running sessions the deletion would disrupt (see
[`openshell-gateway-namespace-gc.spec.md`](./openshell-gateway-namespace-gc.spec.md)
§ Surface Active Sandbox Count Before Deletion and
[`openshell-gateway-sandbox-count.spec.md`](./openshell-gateway-sandbox-count.spec.md)),
but the count is advisory only: deletion proceeds regardless and reclaims the gateway's
resources.

#### Scenario: Delete gateway that has active sandboxes

- GIVEN a Gateway with one or more active sandboxes
- WHEN a user deletes the Gateway (having been warned of the active sandbox count)
- THEN the API server SHALL accept the deletion and SHALL NOT reject it on account of
  the active sandboxes
- AND the control plane SHALL reclaim the gateway's namespace, disrupting those
  sandboxes and cascading removal of their in-namespace resources

---

### Requirement: No Credential Rotation

HyperShell SHALL NOT rotate per-gateway database credentials. No Gateway annotation,
API field, or environment variable triggers a new password. An operator who must
change a gateway's database password does so out-of-band on the server, or by deleting
and recreating the gateway.

The `ALTER ROLE` in Requirement: Per-Gateway Database Provisioning is the provisioning
repair path for a lost tenant Secret and SHALL be retained; it is not a rotation
mechanism.

Admin credential changes are not rotation performed by HyperShell either: the operator
updates the mounted Secret and the control plane reads the new value on its next
operation (see Requirement: Admin Credential Mount).

#### Scenario: Re-reconciliation does not change the password

- GIVEN a provisioned Gateway whose tenant credentials Secret exists
- WHEN the GatewayReconciler processes any update event for the Gateway
- THEN it SHALL NOT generate a new password
- AND SHALL NOT alter the role on the server
- AND the tenant-namespace `openshell-gateway-db-credentials` Secret's `password`
  SHALL be unchanged

---

### Requirement: Database Credential Security

- Admin credentials live only in the mounted Secret and in process memory for the
  duration of one database operation. The control plane SHALL NOT persist them
  anywhere else, SHALL NOT copy them into any other namespace, and SHALL NOT expose
  them through any API.
- Per-gateway passwords SHALL be generated with `crypto/rand` (32-byte hex), reused
  on re-reconciliation, and never logged.
- Passwords SHALL NEVER appear in log messages, error strings, telemetry, Kubernetes
  Events, or API responses. PostgreSQL driver errors routinely embed the host, the
  user and sometimes the full DSN; the control plane SHALL wrap driver errors so that
  connection strings and passwords do not reach logs, conditions or Events.
- Both the admin connection and the gateway connection SHALL use `sslmode=verify-full`
  with the CA bundle from the admin Secret. `require`, `prefer`, `verify-ca` and
  `disable` are not accepted anywhere: the control plane refuses to start on a
  weaker admin `sslmode`, and writes only `verify-full` into tenant Secrets.
  Development and CI stand-in servers therefore serve TLS with a self-signed CA
  generated by the dev scripts rather than running without TLS.

#### Scenario: Driver error is not echoed with its connection string

- GIVEN an admin connection that fails with a driver error containing the endpoint,
  the admin user and the DSN
- WHEN the control plane logs the failure and sets the `DatabaseReady` condition
- THEN the log line and the condition message SHALL NOT contain the password or the
  connection string
- AND the condition message SHALL be the user-facing summary defined in
  [`gateway-provisioning-progress.spec.md`](./gateway-provisioning-progress.spec.md)

---

### Requirement: No Database Surface in the API and CLI

The Gateway contract (REST, gRPC, OpenAPI, SDKs, CLI, web console) SHALL NOT carry a
database reference, and the API SHALL expose no database resource kind. Gateway
creation SHALL NOT perform a database placement step and SHALL NOT be rejected for
lack of a registered database. The gateway provisioning form and the gateway details
view SHALL NOT show a database identifier.

#### Scenario: Create a gateway

- GIVEN a valid gateway create request with `name`, `cluster_id` and `release_id`
- WHEN the API server processes it
- THEN it SHALL create the Gateway without consulting any database registry
- AND the response SHALL contain no database field

---

### Requirement: Development Environments Use the Same Path

`make kind-up` and `make openshift-up` SHALL deploy a stand-in PostgreSQL server that
**serves TLS** with a self-signed CA generated by the dev scripts, and SHALL create
`hypershell-gateway-database-admin` in the HyperShell namespace with the stand-in
server's admin connection, `sslmode: verify-full` and that CA as `sslrootcert`. The
control plane and the gateways then exercise exactly the production path
(`verify-full`, per-gateway DDL, tenant Secret with CA). No database resource is
seeded and no credentials namespace is created. See
[`local-development.spec.md`](./local-development.spec.md) and
[`openshift-development.spec.md`](./openshift-development.spec.md).

#### Scenario: kind-up provisions a gateway database over verified TLS

- GIVEN `make kind-up` has deployed the stand-in server and the admin Secret
- WHEN the seeded Gateway is reconciled
- THEN the control plane SHALL provision `gw_<gatewayID>` over a `verify-full`
  connection to the stand-in server
- AND the gateway pod SHALL connect with the self-signed CA from
  `/etc/openshell-db/ca.crt`

---

## Configuration Reference

| Setting | Where | Default | Meaning |
|---|---|---|---|
| `GATEWAY_DATABASE_ADMIN_DIR` | controller env | `/etc/hypershell/gateway-database` | Directory the admin Secret is mounted at. The only database-related environment variable |
| `hypershell-gateway-database-admin` | Secret, HyperShell namespace | (must exist) | Admin connection files: `host`, `port`, `user`, `password`, `sslrootcert` required; `dbname` (default `postgres`), `sslmode` (must be `verify-full`) optional |
| `deploy/components/gateway-database-admin-secret` | Kustomize component (optional) | not included | External Secrets Operator `ExternalSecret` that syncs the admin Secret from a secret manager. Its template forces `sslmode: verify-full` so a weaker value in the secret manager cannot reach the controller |

The API server reads no database-related configuration for gateways. The control
plane reads nothing that selects a provisioning model.

---

## Configuration Examples

Admin Secret (created by the deployment, before the controller starts):

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: hypershell-gateway-database-admin
  namespace: hypershell-system
type: Opaque
stringData:
  host: mydb.abc123.us-east-1.rds.amazonaws.com
  port: "5432"
  dbname: postgres
  user: hypershell_admin        # rds_superuser / IBM admin; CREATEDB + CREATEROLE + pg_signal_backend
  password: <admin-password>
  sslmode: verify-full          # optional; any other value fails startup
  sslrootcert: |
    -----BEGIN CERTIFICATE-----
    ...cloud provider CA bundle (old and new CA during rotation)...
    -----END CERTIFICATE-----
```

Controller Deployment fragment (shipped in `deploy/base`):

```yaml
env:
  - name: GATEWAY_DATABASE_ADMIN_DIR
    value: /etc/hypershell/gateway-database
volumeMounts:
  - name: gateway-database-admin
    mountPath: /etc/hypershell/gateway-database
    readOnly: true
volumes:
  - name: gateway-database-admin
    secret:
      secretName: hypershell-gateway-database-admin
```

Optional External Secrets component (`deploy/components/gateway-database-admin-secret`):

```yaml
apiVersion: external-secrets.io/v1
kind: ExternalSecret
metadata:
  name: hypershell-gateway-database-admin
spec:
  secretStoreRef:
    kind: ClusterSecretStore
    name: <store>
  target:
    name: hypershell-gateway-database-admin
    template:
      data:
        host: "{{ .host }}"
        port: "{{ .port }}"
        user: "{{ .user }}"
        password: "{{ .password }}"
        sslrootcert: "{{ .sslrootcert }}"
        sslmode: verify-full       # forced; not read from the store
  data:
    - secretKey: host
      remoteRef: { key: <path>, property: host }
    # ... port, user, password, sslrootcert
```

Per-gateway objects the GatewayReconciler creates on the server (illustrative SQL):

```sql
-- role
CREATE ROLE gw_2j5k7m9pqrstvwxyz LOGIN PASSWORD '<32-byte-hex-random>';
-- let a non-superuser admin create a database owned by the role
GRANT gw_2j5k7m9pqrstvwxyz TO hypershell_admin;
-- database owned by the role
CREATE DATABASE gw_2j5k7m9pqrstvwxyz OWNER gw_2j5k7m9pqrstvwxyz TEMPLATE template0;
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
  sslmode: verify-full
  sslrootcert: |
    -----BEGIN CERTIFICATE-----
    ...same bundle as the admin Secret...
    -----END CERTIFICATE-----
  uri: postgresql://gw_2j5k7m9pqrstvwxyz:<password>@mydb.abc123.us-east-1.rds.amazonaws.com:5432/gw_2j5k7m9pqrstvwxyz?sslmode=verify-full&sslrootcert=/etc/openshell-db/ca.crt
```

Gateway Deployment fragment (rendered by the control plane):

```yaml
args: ["--config", "/etc/openshell/gateway.toml", "--db-url", "$(OPENSHELL_DB_URL)"]
env:
  - name: OPENSHELL_DB_URL
    valueFrom:
      secretKeyRef:
        name: openshell-gateway-db-credentials
        key: uri
volumeMounts:
  - name: database-ca
    mountPath: /etc/openshell-db
    readOnly: true
volumes:
  - name: database-ca
    secret:
      secretName: openshell-gateway-db-credentials
      items:
        - key: sslrootcert
          path: ca.crt
```

---

## Operator Runbook

### Finding and dropping leftover gateway databases

Per-gateway cleanup retries while the controller is running, but the retry queue is
in-memory and a failed attempt is recorded as an `IncompleteFinalization` Event (see
Requirement: Per-Gateway Cleanup). Detect and recover leftovers as follows.

1. **Check for recorded failures** in the control-plane namespace:
   ```sh
   kubectl -n <hypershell-namespace> get events \
     --field-selector reason=IncompleteFinalization
   ```
   Each Event names the gateway ID and the `PostgreSQLDatabase gw_<id>` that may
   have been left behind. A later retry may already have removed it; confirm on the
   server.

2. **Identify leftover databases** - connect as the admin user and query:
   ```sql
   SELECT datname FROM pg_database WHERE datname LIKE 'gw\_%';
   ```
   Compare the result to the gateway IDs currently registered in HyperShell
   (`GET /api/hypershell/v1/gateways`). Any `gw_<id>` database whose gateway ID is
   no longer in HyperShell is a leftover. (`_` is a `LIKE` wildcard, hence the
   backslash escape.)

3. **Identify leftover roles** - similarly:
   ```sql
   SELECT rolname FROM pg_roles WHERE rolname LIKE 'gw\_%';
   ```

4. **Terminate, then drop** - for each leftover:
   ```sql
   SELECT pg_terminate_backend(pid)
     FROM pg_stat_activity WHERE datname = 'gw_<id>';
   DROP DATABASE "gw_<id>" WITH (FORCE);
   DROP ROLE "gw_<id>";
   ```

5. **Tenant Secret** - `openshell-gateway-db-credentials` in the gateway's tenant
   namespace is removed by the platform's label-based namespace cleanup when the
   gateway namespace is reclaimed. If the namespace was already deleted, the Secret is
   gone. If it persists, delete it manually.

### Changing the admin password

1. Change the password on the server (`ALTER ROLE hypershell_admin PASSWORD ...`, or
   the cloud provider's console).
2. Update `password` in `hypershell-gateway-database-admin` (or in the secret manager
   when the External Secrets component is used).
3. Wait for the kubelet to refresh the mounted Secret (typically under a minute). No
   controller restart is needed; the next database operation uses the new value.
   Gateway connections are unaffected: they use their own `gw_<id>` roles.

### Rotating the server CA

1. Build a PEM bundle containing the **current** CA and the **new** CA.
2. Update `sslrootcert` in the admin Secret to that bundle. The controller uses it on
   its next database operation; on each gateway's next reconcile the tenant Secret's
   `sslrootcert` is rewritten to match.
3. Restart gateway Deployments (`kubectl -n openshell-<hex> rollout restart
   deployment/openshell-gateway`) so every gateway process reads the new bundle from
   `/etc/openshell-db/ca.crt`. Do this before the server switches to the new chain.
4. Switch the server to the new certificate chain.
5. Once every gateway has been restarted and reconciled, replace the bundle with the
   new CA only and repeat steps 2 and 3.

---

## Debugging Reference

| Symptom | Root Cause | Fix |
|---|---|---|
| Controller pod crash-loops; log says a gateway database credential file is missing or empty | `hypershell-gateway-database-admin` is absent, not mounted at `GATEWAY_DATABASE_ADMIN_DIR`, or lacks a required key | Create the Secret with `host`, `port`, `user`, `password`, `sslrootcert`; check the volume mount and env var |
| Controller refuses to start: `sslmode must be verify-full` | The admin Secret sets `sslmode` to `require`, `verify-ca`, `disable` or `prefer` | Remove the key or set it to `verify-full`; there is no downgrade |
| Controller refuses to start: `sslrootcert` is not PEM | The CA was supplied as a path or DER, or the PEM is truncated | Put the full PEM text of the CA bundle in the `sslrootcert` key |
| Controller refuses to start: invalid `port` | `port` is not an integer in `1`-`65535` | Correct the value (usually `5432`) |
| Gateway stuck with `DatabaseReady: Failed` (database service unavailable) | No network path from the control-plane cluster to the server, or DNS failure | Fix VPC peering / security groups / private endpoint; the reconcile retries on its own |
| Gateway `DatabaseReady: Failed`; controller log shows a certificate verification error | The server certificate does not chain to `sslrootcert`, or its hostname does not match `host` | Supply the provider's current CA bundle; connect by the hostname on the certificate, not an IP |
| Gateway `DatabaseReady: Failed`; controller log shows a permission error on `CREATE DATABASE`/`CREATE ROLE` | Admin role lacks `CREATEDB` or `CREATEROLE` | Grant `rds_superuser` (AWS) / admin role (IBM) or the two privileges |
| Gateway `DatabaseReady: Failed`; controller log shows `must be member of role` on `CREATE DATABASE` | The `GRANT gw_<id> TO <admin>` step did not run or was revoked | Check for an older controller image; re-reconcile |
| Gateway pod `CrashLoopBackOff`, log shows TLS or certificate error connecting to PostgreSQL | Tenant `sslrootcert` is stale after a CA rotation, or the pod predates the rotation | Follow the CA rotation runbook; restart the gateway Deployment |
| Gateway pod cannot find `/etc/openshell-db/ca.crt` | Gateway Deployment rendered by an older controller without the CA mount, or the tenant Secret lacks `sslrootcert` | Upgrade the controller and re-reconcile; the Secret is rewritten with `sslrootcert` |
| Gateway delete never completes; `IncompleteFinalization` Event with `PostgreSQLDatabase gw_<id>` | Cleanup cannot reach the server or lacks `pg_signal_backend` to terminate sessions | Restore reachability (the retry completes) or grant `pg_signal_backend`; if the controller restarted, follow the runbook |
| `gw_*` database or role left on the server after gateway deletion | Cleanup failed and the controller restarted before a retry succeeded | Follow the Operator Runbook above |
| Admin password changed but the controller still authenticates with the old one | Kubelet has not yet refreshed the mounted Secret, or the Secret was mounted via `subPath` (which is never refreshed) | Wait for the refresh; never mount the admin Secret with `subPath` |

---

## References

- [`gateway-deletion-finalization.spec.md`](./gateway-deletion-finalization.spec.md) - required-inline cleanup classes and the `IncompleteFinalization` Event
- [`gateway-provisioning-progress.spec.md`](./gateway-provisioning-progress.spec.md) - the `DatabaseReady` condition and user-facing messages
- [`security.spec.md`](../standards/security/security.spec.md) - secret handling, no secrets in responses or logs
- [`naming-multitenancy.spec.md`](../standards/platform/naming-multitenancy.spec.md) - reserved names
- [`control-plane/conventions.spec.md`](../standards/control-plane/conventions.spec.md) - reconciler error handling, no panic
- [AWS RDS PostgreSQL - master user privileges (`rds_superuser`)](https://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/CHAP_PostgreSQL.html)
- [AWS RDS - SSL/TLS certificate bundles](https://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/UsingWithRDS.SSL.html)
- [IBM Cloud Databases for PostgreSQL - administration](https://cloud.ibm.com/docs/databases-for-postgresql)
- [PostgreSQL - `CREATE DATABASE`, `CREATE ROLE`, `GRANT`/`REVOKE`](https://www.postgresql.org/docs/current/sql-createdatabase.html)
- [PostgreSQL - `sslmode` semantics (libpq)](https://www.postgresql.org/docs/current/libpq-ssl.html)
- [External Secrets Operator - `ExternalSecret` templating](https://external-secrets.io/latest/guides/templating/)
