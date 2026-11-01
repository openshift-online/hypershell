# Runbook: gateway databases

Operator procedures for the PostgreSQL server that hosts one database and role per
gateway. The control plane reaches that server through the
`hypershell-gateway-database-admin` Secret in the HyperShell namespace.

The spec is [`openshell-gateway-database.spec.md`](../../specs/platform/openshell-gateway-database.spec.md).

Contents:

1. [Rotating the server CA](#rotating-the-server-ca)
2. [Changing the admin password](#changing-the-admin-password)
3. [Finding and dropping leftover gateway databases](#finding-and-dropping-leftover-gateway-databases)
4. [Debugging reference](#debugging-reference)

## Rotating the server CA

Use this procedure when the server moves to a certificate signed by a new CA.
Typical causes are a cloud provider CA rollover (AWS RDS, IBM Cloud Databases) or a
self-managed CA change. The spec requirement is OSG-DBS-05.

### Who verifies the CA

- **The control plane verifies the CA.** Its admin connection uses
  `sslmode=verify-full` with the `sslrootcert` bundle from the
  `hypershell-gateway-database-admin` Secret.
- **Gateways do not verify the CA.** They connect with `sslmode=require`. The
  connection is encrypted, but the server certificate is not checked (OSG-DBS-04).

So a CA rotation touches only the admin Secret. You do not need to restart gateways
or change the tenant Secrets (`openshell-gateway-db-credentials`).

The control plane re-reads the mounted admin files on every database operation. You
do not need to restart the controller either.

### Steps

1. **Build a bundle with both CAs.** Concatenate the current CA and the new CA into
   one PEM file:
   ```sh
   cat current-ca.pem new-ca.pem > bundle.pem
   ```

2. **Put the bundle in the admin Secret.** Update the `sslrootcert` key of
   `hypershell-gateway-database-admin` in the HyperShell namespace.
   - If the Secret is synced by External Secrets Operator
     (`deploy/components/gateway-database-admin-secret`), update the `sslrootcert`
     field in the secret manager instead.
   - Otherwise, patch the Secret directly:
     ```sh
     kubectl -n <hypershell-namespace> patch secret hypershell-gateway-database-admin \
       --type merge \
       -p "{\"data\":{\"sslrootcert\":\"$(base64 < bundle.pem | tr -d '\n')\"}}"
     ```

3. **Wait for the kubelet to refresh the mounted file.** This usually takes under a
   minute. The controller uses the new bundle on its next database operation.

4. **Switch the server to the new certificate chain.**

5. **Check that provisioning still works.** Create or re-reconcile a gateway. Its
   `DatabaseReady` condition must not go to `Failed`, and the controller log must
   not show a certificate verification error.

6. **Remove the old CA.** Once the server presents only the new chain, repeat
   step 2 with a bundle that holds only the new CA.

## Changing the admin password

1. Change the password on the server (`ALTER ROLE hypershell_admin PASSWORD ...`, or
   the cloud provider's console).
2. Update `password` in `hypershell-gateway-database-admin` (or in the secret manager
   when the External Secrets component is used).
3. Wait for the kubelet to refresh the mounted Secret (typically under a minute). No
   controller restart is needed; the next database operation uses the new value.
   Gateway connections are unaffected: they use their own `gw_<id>` roles.

## Finding and dropping leftover gateway databases

Per-gateway cleanup retries while the controller is running, but the retry queue is
in-memory and a failed attempt is recorded as an `IncompleteFinalization` Event
(OSG-DBS-07). Detect and recover leftovers as follows.

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

## Debugging reference


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
| Gateway pod `CrashLoopBackOff`, log shows a TLS error connecting to PostgreSQL | The server does not accept TLS (the gateway uses `sslmode=require` and never falls back to plaintext) | Enable TLS on the server; the gateway does not verify the CA, so a CA change alone cannot cause this |
| Gateway delete never completes; `IncompleteFinalization` Event with `PostgreSQLDatabase gw_<id>` | Cleanup cannot reach the server or lacks `pg_signal_backend` to terminate sessions | Restore reachability (the retry completes) or grant `pg_signal_backend`; if the controller restarted, follow [Finding and dropping leftover gateway databases](#finding-and-dropping-leftover-gateway-databases) |
| `gw_*` database or role left on the server after gateway deletion | Cleanup failed and the controller restarted before a retry succeeded | Follow [Finding and dropping leftover gateway databases](#finding-and-dropping-leftover-gateway-databases) |
| Admin password changed but the controller still authenticates with the old one | Kubelet has not yet refreshed the mounted Secret, or the Secret was mounted via `subPath` (which is never refreshed) | Wait for the refresh; never mount the admin Secret with `subPath` |
