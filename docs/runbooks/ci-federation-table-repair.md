# Runbook: converge broken `ci_providers` / `ci_trusts` tables (SDLC-CONF-09)

This repair restores a deployed environment where CI-federation queries return
`500` with `SQLSTATE 25P02` (`current transaction is aborted`) - the failure
that hit hyp0 when the `CIProvider` / `CITrust` migrations still used GORM
`AutoMigrate`. It is idempotent: running it when the environment is already
healthy makes no change.

## Why the break happened

- `AutoMigrate`'s column-diff introspection fails closed on the production
  PostgreSQL driver/protocol (`lib/pq` + `PreferSimpleProtocol`), poisoning the
  migration transaction so every later query on the table aborts (`25P02`).
- The `CIProvider` / `CITrust` models carried no `TableName()`, so GORM named
  their tables `c_iproviders` / `c_itrusts`, while the cross-reference DAOs and
  the OpenAPI paths used `ci_providers` / `ci_trusts`. CI-trust creation could
  never resolve `ci_provider_id`.

Both are fixed in code: migrations now create tables with
`dbmigrate.CreateTable` (raw CREATE TABLE, no AutoMigrate), and the models pin
`TableName()` / column tags to the clean `ci_providers` / `ci_trusts` names
(SDLC-CONF-02, SDLC-CONF-08).

### Why the first fix did not converge upgraded hubs (#862)

That first fix rewrote the `CIProvider` / `CITrust` migration **bodies** but kept
their **existing migration IDs** (`2026102600000001` / `2026102600000002`).
gormigrate records a migration ID as applied exactly once. On every hub that had
already recorded those IDs under the old `AutoMigrate` body, the corrected body
was **skipped**, so the clean tables were never created and the stale
`c_iproviders` / `c_itrusts` tables remained. Only fresh databases got the fix.

Convergence therefore does **not** come from re-running the original IDs. It
ships as **new** convergence migrations (`2026103000000001` /
`2026103000000002`, SDLC-CONF-11) that run on every environment regardless of
prior state: they create the clean table if missing, copy any rows off the stale
mangled table, and drop it. A shipped body is immutable; a correction is always a
new ID (SDLC-CONF-10).

## Repair

The primary repair is simply **deploy an API server image that includes the
new-ID convergence migrations and let migrations run** (the API server runs
`NewMigrateCommand` / `db.Migrate` on startup). Because convergence is a new
migration ID, it runs even on hubs that already recorded the original IDs,
creating the correctly named `ci_providers` / `ci_trusts` tables and dropping the
stale `c_iproviders` / `c_itrusts` tables. No hand-typed SQL patch is required,
and re-deploying is a no-op.

Confirm the image carries the convergence migrations before relying on this:

```sql
-- Both convergence IDs must be recorded after the deploy completes.
SELECT id FROM migrations WHERE id IN ('2026103000000001', '2026103000000002');
```

If, exceptionally, you must converge by hand before such an image is available
(belt-and-suspenders only), the stale mangled tables never held usable rows
(CI-trust writes could not succeed), so dropping them is safe. Run against the
gateway's PostgreSQL as the migration role:

```sql
-- Idempotent: no-op if the stale tables are already gone.
DROP TABLE IF EXISTS c_iproviders;
DROP TABLE IF EXISTS c_itrusts;
```

## Verify

```sql
-- Both must return a row; neither may error.
SELECT to_regclass('public.ci_providers');  -- expect: ci_providers
SELECT to_regclass('public.ci_trusts');     -- expect: ci_trusts
SELECT count(*) FROM ci_providers;           -- expect: a count, never 25P02
SELECT count(*) FROM ci_trusts;              -- expect: a count, never 25P02
```

Then confirm over the API:

```shell
# Expect HTTP 200 with a JSON items array, not 500.
curl -sS -H "Authorization: Bearer $TOKEN" \
  https://<api-host>/api/hypershell/ext/ci_providers
```

## Idempotency

Re-running the deploy, the `DROP TABLE IF EXISTS` statements, and the
verification queries a second time changes nothing and leaves both tables
queryable (SDLC-CONF-09.02).
