# Gateway database admin Secret

Add this component to the Kustomize overlay that deploys the controller. Set the
namespace to the same value as `HYPERSHELL_NAMESPACE`. Change the remote key and
`ClusterSecretStore` reference for your environment. For an overlay at
`deploy/<environment>/kustomization.yaml`, use this relative path:

```yaml
components:
  - ../components/gateway-database-admin-secret
```

Terraform can create the cloud PostgreSQL server, network rules, admin account,
and secret-manager entry. Install External Secrets Operator and configure the
`gateway-databases` ClusterSecretStore with workload identity and read access to
that entry. The [ESO provider documentation](https://external-secrets.io/latest/provider/aws-secrets-manager/)
describes the AWS setup. Other providers use the same ExternalSecret contract.

The remote entry must be a JSON object with these fields:

| Field | Value |
| --- | --- |
| `host` | PostgreSQL DNS name that matches its server certificate |
| `port` | PostgreSQL port, for example `5432` |
| `user` | Provisioning account name |
| `password` | Provisioning account password |
| `sslrootcert` | PEM CA bundle for the server certificate |
| `dbname` | Optional admin database; default `postgres` |

Keep credential values out of Git and Terraform output. If Terraform manages a
password, protect its state as a credential store. Use the cloud service's secret
integration where available. The repository needs only the store reference,
remote key, and controller environment variable.

The admin account needs `CREATEDB`, `CREATEROLE`, and permission to manage each
gateway role and terminate its sessions (`pg_signal_backend` on PostgreSQL).
The controller grants each gateway role to the admin account so that a
non-superuser can create and drop the gateway database. Cloud services can impose
additional restrictions. Verify these permissions on your chosen service before
production use. The tests use PostgreSQL 18 with a non-superuser account.

Apply the store and ExternalSecret before the controller configuration. Wait for
`ExternalSecret/gateway-database-admin` to report `Ready=True`. A missing or
invalid Secret makes gateway provisioning fail and retry. There is no fallback
to an in-cluster database. Both database connections use `verify-full`. The
gateway receives only its own credentials and the public CA bundle.

The API keeps its existing ManagedDatabase records and `Gateway.database_id`
behavior. These records do not select the server while the override is enabled.
This component does not move existing gateway data. Use a new installation, or
complete a separate data migration before enabling or disabling it.

## Credential and CA updates

ESO refreshes the Kubernetes Secret every minute. The controller reads current
admin credentials for each database operation; no controller restart is required
after an admin password update. A Secret update alone does not enqueue all
gateways. Running gateways skip full provisioning. For a CA change, update the
`sslrootcert` field in each existing gateway's `openshell-gateway-db-credentials`
Secret with the public bundle and restart its `openshell-gateway` Deployment.
A GitOps job can perform these two operations without access to the admin password.
Keep both old and new CA certificates in the bundle during a CA transition. Changing the server host requires a data migration.

## Failed deletion

A failed database cleanup returns an error and records an `IncompleteFinalization`
warning Event. The live controller queue retries the deletion. Fix credentials,
network access, or SQL permissions and check that the database and role disappear.
The retry queue does not survive a controller restart.

After a restart, use the gateway ID from the Event to check the shared PostgreSQL
server. The database and role names are `gw_` plus the lowercase gateway ID. Verify
that the gateway is absent from the API and that the database is no longer in use.
Connect with the admin account through a protected credential file. Drop the
identified database with `DROP DATABASE "gw_<id>" WITH (FORCE)`, then drop the role
with `DROP ROLE "gw_<id>"`. Do not drop the shared server. Check both `pg_database`
and `pg_roles` to confirm removal.

## Tests

Run `bash scripts/test-controller-database-secret.sh` for real PostgreSQL lifecycle
and TLS tests. The `secret` Kind CI case applies this ExternalSecret through ESO's
Fake provider, then runs the deployed gateway API and CLI tests. The Fake provider
is only a test fixture. It does not test cloud identity or cloud network access.
