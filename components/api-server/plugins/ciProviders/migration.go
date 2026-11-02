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
			// column-diff introspection fails closed on the production lib/pq +
			// PreferSimpleProtocol driver and aborted every ci_providers query
			// with SQLSTATE 25P02 on hyp0. The CIProvider model carries its own
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
// The add and backfill use idempotent raw SQL that is safe under the production
// lib/pq + PreferSimpleProtocol driver. Existing rows are backfilled from the
// forge so a find-by-name search resolves pre-existing records.
func migrationAddName() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "2026110200000001",
		Migrate: func(tx *gorm.DB) error {
			if err := tx.Exec(`ALTER TABLE ci_providers ADD COLUMN IF NOT EXISTS name text`).Error; err != nil {
				return err
			}
			return tx.Exec(`UPDATE ci_providers SET name = forge WHERE name IS NULL OR name = ''`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`ALTER TABLE ci_providers DROP COLUMN IF EXISTS name`).Error
		},
	}
}
