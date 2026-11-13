package ciProviders

import (
	"gorm.io/gorm"

	"github.com/go-gormigrate/gormigrate/v2"
	"github.com/openshift-online/hypershell/components/api-server/pkg/dbmigrate"
)

func migration() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "2026102600000001",
		Migrate: func(tx *gorm.DB) error {
			// Raw CREATE TABLE via the GORM schema (SDLC-CONF-02): AutoMigrate's
			// column-diff introspection fails closed under PreferSimpleProtocol
			// (production runs the pgx driver, gorm.io/driver/postgres) and aborted
			// every ci_providers query with SQLSTATE 25P02 on hyp0. The CIProvider model carries its own
			// TableName() and column/index tags, so the migration reuses it
			// directly rather than duplicating the field list.
			return dbmigrate.CreateTable(tx, &CIProvider{})
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Migrator().DropTable(&CIProvider{})
		},
	}
}

// migrationConverge is the new-ID convergence migration for ci_providers
// (SDLC-CONF-09, SDLC-CONF-11, issue #862). The original migration above added
// TableName()+CreateTable under its already-shipped ID 2026102600000001, so
// upgraded hubs (which had already recorded that ID under the old AutoMigrate
// body) skipped the corrected body: they kept the GORM-default-mangled
// "c_iproviders" table and never created the clean "ci_providers" table. A body
// fix under the shipped ID cannot reach those environments; convergence must run
// under a NEW ID that runs on every environment regardless of prior state.
//
// CIProvider is an unused Kind and the mis-shipped "c_iproviders" table is empty
// (CI-trust writes could never resolve the mangled schema), so the simplest
// correct convergence is to drop the wrong table and create the clean one - no
// row copying. On a fresh or already-converged database the DROP IF EXISTS is a
// no-op and CreateTable recreates the clean table idempotently.
func migrationConverge() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "2026103000000001",
		Migrate: func(tx *gorm.DB) error {
			if err := tx.Exec("DROP TABLE IF EXISTS c_iproviders").Error; err != nil {
				return err
			}
			return dbmigrate.CreateTable(tx, &CIProvider{})
		},
		// Convergence is not reversible: the stale table is intentionally dropped.
		Rollback: func(tx *gorm.DB) error { return nil },
	}
}

// migrationAddName adds the queryable name column (SDLC-CONF-13). ci_providers
// already shipped its table under migration ID 2026102600000001, so the column
// MUST be added under a NEW migration ID that runs on every environment,
// regardless of prior state - editing the already-shipped body is skipped on
// upgraded hubs (the #862 lesson), and AutoMigrate is forbidden (SDLC-CONF-02).
//
// The name is backfilled from the forge (matching CIProvider.BeforeCreate, which
// defaults a missing name to the Go string forge, never NULL) and a UNIQUE index
// enforces the domain rule "one provider per forge": without it two providers on
// the same forge share the default name and find-by-name is ambiguous. Because a
// pre-existing forge is NOT guaranteed unique, the shared helper de-duplicates
// colliding names collision-safely BEFORE creating the index, so a duplicate forge
// can never fail CREATE UNIQUE INDEX, roll the migration back, and brick startup
// (FINDING 1). See dbmigrate.AddUniqueNameColumn for the derivation.
func migrationAddName() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "2026110213000001",
		Migrate: func(tx *gorm.DB) error {
			return dbmigrate.AddUniqueNameColumn(tx, "ci_providers", "COALESCE(forge,'')")
		},
		Rollback: func(tx *gorm.DB) error {
			return dbmigrate.DropUniqueNameColumn(tx, "ci_providers")
		},
	}
}

// migrationPartialIssuerIndex converges the issuer unique index to a PARTIAL index
// scoped to LIVE rows (SDLC-CONF-13, FINDING 1, same root cause as the name index).
//
// The original ci_providers table shipped with a FULL unique index on issuer
// (idx_ci_providers_issuer, built by CreateTable from the model's former
// `gorm:"uniqueIndex"` tag on Issuer). A full index also constrains soft-deleted
// rows, so a delete-then-recreate that reuses the SAME issuer - exactly what a
// gitops reconcile / re-apply of a CIProvider does - would 409 on the lingering
// soft-deleted row and break convergence. GORM soft-delete filters
// deleted_at IS NULL on reads, so find-by-issuer sees nothing after a delete and
// the reconcile re-issues CREATE with the same issuer.
//
// This runs under a NEW migration ID so it reaches upgraded hubs where the full
// index was already built under an earlier shipped body (the #862 lesson), and the
// model tag is removed so CreateTable never rebuilds a full index on a fresh
// database. It drops any pre-existing index of that name (full or partial) and
// recreates it PARTIAL. Raw SQL, idempotent, safe under lib/pq +
// PreferSimpleProtocol (SDLC-CONF-02); a re-run is a drop+recreate no-op.
func migrationPartialIssuerIndex() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "2026110214000001",
		Migrate: func(tx *gorm.DB) error {
			if err := tx.Exec(`DROP INDEX IF EXISTS idx_ci_providers_issuer`).Error; err != nil {
				return err
			}
			return tx.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_ci_providers_issuer ON ci_providers (issuer) WHERE deleted_at IS NULL`).Error
		},
		// Converging to the live-scoped constraint is intentionally not reversed to
		// the broken full index.
		Rollback: func(tx *gorm.DB) error { return nil },
	}
}

// migrationAddIdpAlias adds the explicit (spec) idp_alias column (PLAT-DATA-24,
// Bug B of #1012). ci_providers already shipped its table under an earlier ID, so
// the column MUST be added under a NEW migration ID that runs on every environment
// regardless of prior state - editing an already-shipped body is skipped on upgraded
// hubs (the #862 lesson), and AutoMigrate is forbidden (SDLC-CONF-02): its
// add-column path is unreliable under the test session's lib/pq driver with
// PreferSimpleProtocol. The column is nullable TEXT with no default, so a
// pre-existing provider keeps idp_alias="" (unset) and the reconciler falls back to
// the derived alias, matching the model's zero value. A fresh CreateTable builds the
// same column from the model's string field, so a fresh and an upgraded schema
// converge. Raw, idempotent (IF NOT EXISTS), safe to re-run.
func migrationAddIdpAlias() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "2026111000000001",
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec(`
				ALTER TABLE ci_providers
					ADD COLUMN IF NOT EXISTS idp_alias TEXT
			`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`
				ALTER TABLE ci_providers
					DROP COLUMN IF EXISTS idp_alias
			`).Error
		},
	}
}
