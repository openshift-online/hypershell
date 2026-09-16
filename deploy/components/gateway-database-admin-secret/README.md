# gateway-database-admin-secret

Optional Kustomize Component that produces the controller's single
gateway-database admin credential Secret, `hypershell-gateway-database-admin`
in `hypershell-system`, with External Secrets Operator (`external-secrets.io/v1`).
The base controller Deployment already mounts that Secret at
`/etc/hypershell/gateway-database`; this component only supplies its contents.

**The controller refuses to start when the Secret is missing or malformed.**
Environments that do not use ESO must create the Secret by other means (dev
scripts generate it: `scripts/kind/up.sh`, `scripts/cluster/drivers/openshift.sh`).

## Enable

```yaml
# deploy/<env>/kustomization.yaml
components:
  - ../components/gateway-database-admin-secret
```

Override `spec.secretStoreRef.name` (default `gateway-databases`) and
`spec.dataFrom[0].extract.key` (default `hypershell/production/gateway-database`)
for the environment.

## Remote JSON document

Every top-level field becomes a Secret key.

| Field | Required | Meaning |
|-------|----------|---------|
| `host` | yes | Server hostname. Must match a SAN of the server certificate (verify-full checks it). |
| `port` | yes | Server port, as a string (for example `"5432"`). |
| `user` | yes | Admin role the controller logs in as. |
| `password` | yes | Admin role password. |
| `sslrootcert` | yes | PEM CA bundle that signs the server certificate. Inline PEM text, never a path. |
| `dbname` | no | Maintenance database for the admin session. Defaults to `postgres`. |
| `sslmode` | no | If present it must be `verify-full`. The component pins it to `verify-full` via `template.data` regardless. |

Do not put the `openshell-gateway-db-credentials` tenant values here; the
controller derives them per gateway.

## Required PostgreSQL privileges

The admin role needs `CREATEDB`, `CREATEROLE` and membership in
`pg_signal_backend` (to terminate a gateway's sessions before dropping its
database). Superuser is not required. The server must serve TLS with a
certificate whose SAN covers `host`.

```sql
CREATE ROLE hypershell_admin LOGIN PASSWORD '...' CREATEDB CREATEROLE;
GRANT pg_signal_backend TO hypershell_admin;
```

## Rotation

- **Password**: update the store; ESO rewrites the Secret on the next refresh.
  The controller re-reads the mounted file for every operation, so no restart
  is needed. Gateway tenant roles have their own passwords and are unaffected.
- **CA (`sslrootcert`)**: the controller picks up a new bundle the same way.
  Gateway pods, however, read their copy (`/etc/openshell-db/ca.crt` from
  `openshell-gateway-db-credentials`) at start-up and need a restart to trust
  a new CA. Keep the old and new CA certificates together in the bundle until
  every gateway has restarted, then drop the old one.

## Manual cleanup after a lost delete

The controller drops a gateway's `gw_<id>` database and login role when the
Gateway is deleted. If the controller restarts while that delete is queued the
objects can be orphaned. Remove them by hand as the admin role:

```sql
SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = 'gw_<id>';
DROP DATABASE IF EXISTS "gw_<id>";
DROP ROLE IF EXISTS "gw_<id>";
```

List candidates with `SELECT datname FROM pg_database WHERE datname LIKE 'gw_%';`
and compare against the API's gateway inventory before dropping anything.
