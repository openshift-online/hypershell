# API Kind Conformance Specification

**ID prefix:** SDLC-CONF

## Purpose

Every HyperShell API Kind -- `/v1` and Extension (`ext`) -- SHALL be complete and
work end to end the first time it ships. A Kind is complete only when it carries
the full set of artifacts that make it queryable, authorizable, scriptable, and
tested over the real database. An incomplete Kind SHALL fail continuous
integration on its FIRST pull request, not in a deployed environment. This is a
Principle-of-Least-Surprise contract: a generated Kind is expected to work 100%,
so the pipeline that produces Kinds SHALL be enforced by an automated gate rather
than by human memory.

This standard turns the full-stack-pipeline skill's Kind Completeness Manifest and
conformance pass (`skills/build/full-stack-pipeline/SKILL.md`) into an enforced
guarantee. It governs how Kinds are built (`PLAT-DATA`, `PLAT-CITR`), how they are
authorized (`SEC-RBAC-19`), how they are scripted (`CLI-EXT`), and how their tests
run in CI (`E2E-CORE-23`, `E2E-CORE-24`). The driving failure was `CIProvider` and
`CITrust` shipping without database-backed tests and with an `AutoMigrate`
migration that fails closed on the production PostgreSQL driver, so that every
query on `ci_providers`/`ci_trusts` aborted with `SQLSTATE 25P02` in a deployed
environment while CI stayed green.

A second failure in the same family drives SDLC-CONF-10 through SDLC-CONF-12: the
first fix changed the `CIProvider`/`CITrust` migration bodies (`AutoMigrate` ->
raw SQL) but kept the same migration IDs. Because the framework records a migration
ID as applied exactly once, the corrected body was skipped on every upgraded hub
that had already recorded the old ID, so the clean tables were never created and
the deployed environments stayed broken while a fresh-database conformance test
passed. Migration corrections therefore SHALL ship as new migration IDs, schema
convergence SHALL run automatically on deploy, and conformance SHALL exercise the
upgrade path, not only a fresh database.

## Requirements

### Requirement: SDLC-CONF-01 -- Kind Completeness Manifest

Every registered API Kind SHALL have every artifact in the Kind Completeness
Manifest. The set of registered Kinds SHALL be enumerated from the plugin
registrations (the API server's side-effect plugin imports and
`RegisterPrefixedRoutes`), the RBAC `extKinds` allow-list, and the generated
`urls` paths. For each such Kind, all of the following SHALL exist:

- **Plugin files** under `components/api-server/plugins/<kind>/`: `model.go`,
  `dao.go`, `service.go`, `handler.go`, `presenter.go`, `grpc_handler.go`,
  `grpc_presenter.go`, `plugin.go` (registering the service, REST routes, gRPC
  service, migration, and any controller), `mock_dao.go`, and `migration.go`.
- **Migration** that creates and evolves the Kind's table with raw SQL through
  `tx.Exec` (see SDLC-CONF-02).
- **OpenAPI** schema and every REST path for the Kind, wired into the aggregate
  `openapi.yaml`, with `required[]` matching the Kind's data-model fields.
- **SDK** Go type and client in `components/sdk-go/` and TypeScript type and
  client in `components/sdk-typescript/`, produced by the generator and never
  hand-edited.
- **CLI** `hsctl create`, `get`, `list`, `delete`, and `apply` for the Kind plus
  its `urls` entry, per `CLI-EXT`. A Kind absent from a spec CLI table still gets
  all five verbs.
- **RBAC** for `ext` Kinds: an `extKinds` allow-list entry AND an explicit
  authorization rule, per `SEC-RBAC-19`; `/v1` Kinds carry their rule. Absence of
  the allow-list entry is a fail-closed 403 for every caller, which is a latent
  outage, not a deliberate deny.
- **Proto/control-plane** message and RPCs regenerated; where the control plane
  watches the Kind, the watcher subscription and reconciler handle it.
- **Tests** that are database-backed, per SDLC-CONF-03.

The manifest SHALL be identical for `/v1` and `ext` Kinds except where a row
states otherwise (RBAC `extKinds` applies only to `ext` Kinds).

#### Scenario: SDLC-CONF-01.01 -- Every registered Kind is enumerated

- GIVEN a Kind registered through a plugin, the `extKinds` allow-list, or `urls`
- WHEN the conformance gate enumerates registered Kinds
- THEN that Kind SHALL be in the enumerated set
- AND the gate SHALL evaluate every manifest artifact for it

#### Scenario: SDLC-CONF-01.02 -- A Kind missing a manifest artifact is non-conformant

- GIVEN a registered Kind that lacks any single manifest artifact (for example a
  missing CLI verb, missing OpenAPI path, missing `extKinds` entry, or missing
  database-backed test)
- WHEN the conformance gate evaluates it
- THEN the Kind SHALL be reported non-conformant
- AND the report SHALL name the Kind and the specific missing artifact

### Requirement: SDLC-CONF-02 -- Migrations Are Raw SQL; AutoMigrate Is Forbidden

Every plugin migration SHALL create and evolve its table with raw SQL executed
through `tx.Exec`, using idempotent statements (`CREATE TABLE IF NOT EXISTS`,
`ALTER TABLE ... ADD COLUMN IF NOT EXISTS`). GORM `AutoMigrate` SHALL NOT be used
in any plugin migration. This is a uniform policy across ALL Kinds, not a
per-Kind exception: `AutoMigrate` can fail closed on the production PostgreSQL
driver and protocol (`lib/pq` with `PreferSimpleProtocol`), which poisons the
transaction and aborts every subsequent query on the table with `SQLSTATE 25P02`.
A uniform raw-SQL rule is the only state in which the conformance pass is
idempotent and the gate's `AutoMigrate` ban is coherent with the whole plugin set.

The ban is reconciled with `skills/build/full-stack-pipeline/SKILL.md` (which
rejects `AutoMigrate`) and with the database-migration convention in
`standards/platform/cross-cutting.spec.md`: all three state the single uniform
rule.

#### Scenario: SDLC-CONF-02.01 -- AutoMigrate in a migration fails the gate

- GIVEN a plugin `migration.go` that calls `AutoMigrate`
- WHEN the static conformance check (SDLC-CONF-05) runs
- THEN the check SHALL fail
- AND it SHALL name the Kind whose migration uses `AutoMigrate`

#### Scenario: SDLC-CONF-02.02 -- Raw-SQL migration is idempotent on re-run

- GIVEN a Kind whose migration uses `CREATE TABLE IF NOT EXISTS` and
  `ALTER TABLE ... ADD COLUMN IF NOT EXISTS`
- WHEN the migration runs twice against the same database
- THEN the second run SHALL make no change and SHALL NOT error
- AND the table SHALL remain queryable with no aborted transaction

### Requirement: SDLC-CONF-03 -- Mandatory Database-Backed Integration and Authorization Tests

Every registered Kind SHALL have a database-backed integration test and an
authorization test. The integration test SHALL run against a real PostgreSQL
instance (testcontainers, `API_ENV=integration_testing`) and drive a full
create -> get -> list -> delete round-trip over the real HTTP and database stack.
The authorization test SHALL assert the Kind's `SEC-RBAC-19` rule (allowed roles
pass, other callers are denied). Mock-only gRPC or DAO tests SHALL NOT satisfy
this requirement: they never execute the migration or touch the schema, which is
precisely the gap through which broken Kinds have shipped.

#### Scenario: SDLC-CONF-03.01 -- A Kind without a database-backed test is non-conformant

- GIVEN a Kind whose only tests are mock-based
- WHEN the conformance gate evaluates it
- THEN the Kind SHALL be reported non-conformant for a missing database-backed
  integration test

#### Scenario: SDLC-CONF-03.02 -- The integration test exercises the real migration

- GIVEN a Kind whose migration would fail on the production driver
- WHEN its database-backed integration test runs on the production driver config
  (SDLC-CONF-04)
- THEN the test SHALL fail at migration or first query rather than passing
- AND the failure SHALL be attributable to that Kind

### Requirement: SDLC-CONF-04 -- Conformance Tests Run on the Production Driver Configuration in CI

The database-backed integration tests (SDLC-CONF-03) and the generic CRUD
conformance test (SDLC-CONF-06) SHALL run against the same PostgreSQL driver and
protocol as production: `lib/pq` with `PreferSimpleProtocol`. They SHALL run in
continuous integration, as part of the CI unit-test stage (`E2E-CORE-24`), not
only on a developer workstation. A migration or query that breaks on the
production driver SHALL therefore break CI. Running these tests on a driver or
protocol that differs from production is the gap that let the `ci_providers`
break reach a deployed environment, so the harness DSN and driver SHALL be
verified to match production.

#### Scenario: SDLC-CONF-04.01 -- Production-driver break is caught in CI

- GIVEN a Kind whose migration aborts under `lib/pq` with `PreferSimpleProtocol`
- WHEN the CI unit-test stage runs the conformance and integration tests
- THEN the stage SHALL fail on that Kind
- AND the failure SHALL occur on the first pull request that introduces the Kind,
  before merge

#### Scenario: SDLC-CONF-04.02 -- Harness driver matches production

- GIVEN the integration-test harness configuration
- WHEN the driver and protocol are inspected
- THEN they SHALL be `lib/pq` with `PreferSimpleProtocol`, matching the deployed
  API server

### Requirement: SDLC-CONF-05 -- Static Conformance Check in `make check`

The system SHALL provide a static conformance check wired into `make check`, run
by the `repository-policy` job of the Checks CI Gate (`E2E-CORE-23`). The check
SHALL enumerate every registered Kind and FAIL when any Kind is non-conformant,
naming each non-conformant Kind and each missing manifest item: a missing plugin
file, a missing OpenAPI path or schema, a missing Go or TypeScript SDK type or
client, a missing CLI verb, a missing `extKinds` entry or authorization rule, an
`AutoMigrate` in a migration, or the absence of a database-backed integration
test. The check SHALL run unconditionally as a whole-repository policy check,
regardless of which components a pull request changed.

#### Scenario: SDLC-CONF-05.01 -- make check fails and names the gaps

- GIVEN a repository with one or more non-conformant Kinds
- WHEN `make check` runs
- THEN it SHALL exit non-zero
- AND its output SHALL name each non-conformant Kind and its missing items

#### Scenario: SDLC-CONF-05.02 -- make check passes when every Kind conforms

- GIVEN a repository in which every registered Kind satisfies the manifest
- WHEN `make check` runs
- THEN the static conformance check SHALL pass

### Requirement: SDLC-CONF-06 -- Generic CRUD Conformance Test in `make test`

The system SHALL provide a single generic CRUD conformance test wired into
`make test` that iterates every registered `ext` Kind and asserts a full
create -> get -> list -> delete round-trip over the real HTTP and database stack,
on the production driver configuration (SDLC-CONF-04). The test SHALL discover
Kinds from the registration sources (SDLC-CONF-01) rather than from a hand-kept
list, so a newly registered Kind is covered without editing the test. A broken
migration or a missing CRUD operation for any `ext` Kind SHALL fail the test.
This operationalizes "100% of generated Kinds work" as an enforced invariant.

#### Scenario: SDLC-CONF-06.01 -- A newly registered Kind is covered automatically

- GIVEN a new `ext` Kind added to the registration sources
- WHEN the generic CRUD conformance test runs
- THEN the test SHALL include that Kind in its round-trip iteration without any
  edit to the test's Kind list

#### Scenario: SDLC-CONF-06.02 -- A broken round-trip fails the test

- GIVEN an `ext` Kind whose create, get, list, or delete fails over the real
  stack
- WHEN the generic CRUD conformance test runs under `make test`
- THEN the test SHALL fail and identify the Kind and the failing operation

### Requirement: SDLC-CONF-07 -- Idempotent Conformance Pass

The pipeline SHALL provide a conformance pass that enumerates ALL registered
Kinds, diffs each against the Kind Completeness Manifest, and additively
generates every missing artifact. The pass SHALL be idempotent: a Kind that
already conforms produces no change, and running the pass a second time yields an
empty diff. Pre-existing incomplete Kinds SHALL be brought into conformance by
running this pass, never by a one-off hand edit to a single Kind. Fixing one Kind
in isolation while leaving the pass unable to reproduce the fix is a defect in the
pass.

#### Scenario: SDLC-CONF-07.01 -- Second run yields no diff

- GIVEN a repository whose Kinds have been brought into conformance by the pass
- WHEN the conformance pass runs again
- THEN it SHALL produce no file changes

#### Scenario: SDLC-CONF-07.02 -- The pass fills every Kind, not one

- GIVEN several registered Kinds each missing one or more manifest artifacts
- WHEN the conformance pass runs once
- THEN it SHALL add the missing artifacts for all of those Kinds, not only the
  Kind that a change happened to touch

### Requirement: SDLC-CONF-08 -- All Existing Extension Kinds Conform

Every existing `ext` Kind SHALL satisfy the Kind Completeness Manifest, reached
through the idempotent conformance pass (SDLC-CONF-07). This explicitly includes
`CIProvider`, `CITrust`, `InferenceRoute`, `ProviderBinding`, `ProviderSpec`, and
`SecretSource` -- which lacked database-backed integration tests, and in some
cases authorization tests, CLI verbs, SDK clients, or `extKinds` entries -- and
the audited `AgentWorkspace` and `WorkspaceMembership`. Each SHALL gain its
database-backed integration test, its authorization test, and any missing CLI
verbs, SDK artifacts, or `extKinds` entries. No `ext` Kind SHALL remain whose
only coverage is mock-based.

#### Scenario: SDLC-CONF-08.01 -- Named Kinds have database-backed and authorization tests

- GIVEN `CIProvider`, `CITrust`, `InferenceRoute`, `ProviderBinding`,
  `ProviderSpec`, and `SecretSource`
- WHEN the conformance gate evaluates each
- THEN each SHALL have a database-backed integration test and an authorization
  test
- AND the gate SHALL report each as conformant

#### Scenario: SDLC-CONF-08.02 -- CIProvider and CITrust are queryable after conformance

- GIVEN `ci_providers` and `ci_trusts` created by raw-SQL migrations (SDLC-CONF-02)
- WHEN the generic CRUD conformance test runs against the production driver
- THEN create, get, list, and delete SHALL each succeed
- AND no query SHALL abort with `SQLSTATE 25P02`

### Requirement: SDLC-CONF-09 -- Deployed-Environment Convergence After a Migration Fix

When a migration fix ships for a Kind whose table is already in a bad state in a
deployed environment (for example `ci_providers`/`ci_trusts` left broken by a
failed `AutoMigrate`), every environment SHALL converge to a queryable state at
deploy time without a manual, ad-hoc SQL patch applied by a human. Convergence
SHALL come from a new-ID convergence migration (SDLC-CONF-11), NOT from a changed
body under the already-shipped migration ID: a migration whose body is edited but
whose ID is unchanged never re-runs on an environment that already recorded that
ID (SDLC-CONF-10), so editing the original migration in place only repairs fresh
databases and leaves every upgraded environment broken. The convergence migration
SHALL run automatically when the fixed API server image is deployed and SHALL
leave the table queryable with no aborted transaction. Any documented runbook
repair SHALL describe this new-migration convergence as the primary path and MUST
NOT claim that re-deploying the image re-runs the original (same-ID) migration;
manual `DROP TABLE IF EXISTS` steps MAY remain only as a belt-and-suspenders
fallback.

#### Scenario: SDLC-CONF-09.01 -- Broken deployed table converges after the fix deploys

- GIVEN a deployed environment where `ci_providers` returns 500 with
  `SQLSTATE 25P02` and the original creating migration ID is already recorded as
  applied
- WHEN the fixed API server image carrying the new-ID convergence migration is
  deployed
- THEN the convergence migration SHALL run because its ID has not been recorded
- AND `ci_providers` SHALL be queryable with no `25P02` with no hand-typed SQL

#### Scenario: SDLC-CONF-09.02 -- Convergence is idempotent across redeploys

- GIVEN an environment already converged by the new-ID convergence migration
- WHEN the fixed image is deployed again
- THEN the convergence migration SHALL NOT re-run (its ID is now recorded)
- AND the table SHALL remain queryable with no change and no error

### Requirement: SDLC-CONF-10 -- Shipped Migration Bodies Are Immutable; Fixes Ship as a New Migration ID

Once a migration ID has shipped, its body SHALL be treated as immutable. The
migration framework (gormigrate) records a migration ID as applied exactly once;
on any environment that already recorded that ID, an edited body is SKIPPED and
never re-runs. Editing an already-shipped migration in place therefore repairs
only fresh databases and silently leaves every upgraded environment on the old
behavior. Any correction to already-shipped migration behavior SHALL ship as a
NEW migration with a new, unique ID so it runs on every environment regardless of
prior state.

This is a convention, upheld by the migration mechanism itself and by code review,
not by a separate static gate, lock file, or developer command. Because an edited
body under an already-shipped ID silently fails to converge upgraded environments,
the upgrade-path conformance test (SDLC-CONF-12) -- which exercises the real
upgraded-hub scenario against the production driver -- is what fails when this
convention is broken: a "fix in place" edit leaves the upgraded clean table
missing and that test fails. Enforcement of this convention therefore lives in the
existing migration path and its upgrade-path test, not in any new check a developer
must run.

#### Scenario: SDLC-CONF-10.01 -- A correction shipped as a new ID converges upgraded environments

- GIVEN an upgraded environment that recorded the original migration ID
- WHEN a corrected migration ships under a new, unique ID
- THEN the new migration SHALL run on that environment because its ID is not
  recorded
- AND the corrected behavior SHALL take effect without editing the original
  migration

### Requirement: SDLC-CONF-11 -- Idempotent Convergence Migration for a Mis-shipped Schema

When a prior migration left a deployed schema wrong (a missing clean table, a
stale mangled table, or both -- for example `ci_providers` missing while a mangled
`c_iproviders` remains), the fix SHALL be a new-ID convergence migration
(SDLC-CONF-10) that, using the raw-SQL / `dbmigrate.CreateTable` path and the
Kind's pinned `TableName()` (SDLC-CONF-02):

- Drops the stale mangled table with `DROP TABLE IF EXISTS` and creates the clean
  table. For a mis-shipped table of an unused Kind the stale table holds no rows,
  so the migration SHALL NOT copy rows out of it; the drop-and-recreate is the
  complete convergence.
- Is fully idempotent: on an already-healthy environment (clean table present,
  stale table absent) it makes no change and does not error, and it SHALL NOT use
  `AutoMigrate` or poison the transaction on the production driver (`lib/pq` with
  `PreferSimpleProtocol`).

Every migration whose body was rewritten in place by a prior "fix in place" change
(the `AutoMigrate` -> raw-SQL conversion) SHALL be audited, and each Kind whose
upgraded-environment schema does not match its intended schema SHALL be brought to
convergence by this pattern. The audit SHALL NOT assume only `ci_providers` and
`ci_trusts` are affected.

#### Scenario: SDLC-CONF-11.01 -- Upgraded hub converges to the clean table

- GIVEN an upgraded environment where the clean table is missing and a stale
  mangled table (for example `c_iproviders`) remains
- WHEN the new-ID convergence migration runs
- THEN the stale mangled table SHALL be dropped and the clean table SHALL exist
- AND no query SHALL abort with `SQLSTATE 25P02`

#### Scenario: SDLC-CONF-11.02 -- No-op on a healthy environment

- GIVEN an environment where the clean table is already present and no stale
  mangled table exists
- WHEN the convergence migration runs
- THEN it SHALL make no change and SHALL NOT error

### Requirement: SDLC-CONF-12 -- Upgrade-Path Conformance Test

The system SHALL provide an upgrade-path conformance test, distinct from the
fresh-database generic CRUD conformance test (SDLC-CONF-06), that runs the
migration suite against a database pre-seeded with the pre-fix state: the stale
mangled tables (for example `c_iproviders`) AND the original migration IDs already
recorded in the `migrations` table. After the migration suite runs, the test SHALL
assert that every ext Kind's clean table exists, that the full create -> get ->
list -> delete round-trip succeeds, and that no stale mangled table remains. This
test SHALL run on the production driver configuration (SDLC-CONF-04) in continuous
integration. Both the fresh-database test (SDLC-CONF-06) and this upgrade-path
test SHALL pass; a convergence regression SHALL fail this test.

#### Scenario: SDLC-CONF-12.01 -- Pre-seeded upgrade path converges and passes

- GIVEN a database pre-seeded with the stale mangled tables and the original
  migration IDs already recorded as applied
- WHEN the migration suite runs and the upgrade-path conformance test executes
- THEN every ext Kind's clean table SHALL exist, every CRUD round-trip SHALL
  succeed, and no stale mangled table SHALL remain

#### Scenario: SDLC-CONF-12.02 -- A convergence regression fails the test

- GIVEN a change that removes or breaks the new-ID convergence migration
- WHEN the upgrade-path conformance test runs
- THEN the test SHALL fail and name the Kind whose clean table is missing or whose
  stale table remains
