package ciTrusts

import (
	"gorm.io/gorm"

	"github.com/go-gormigrate/gormigrate/v2"
	"github.com/openshift-online/hypershell/components/api-server/pkg/dbmigrate"
)

func migration() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "2026102600000002",
		Migrate: func(tx *gorm.DB) error {
			// Raw CREATE TABLE via the GORM schema (SDLC-CONF-02): AutoMigrate's
			// column-diff introspection fails closed on the production lib/pq +
			// PreferSimpleProtocol driver and aborted every ci_trusts query with
			// SQLSTATE 25P02 on hyp0. The CITrust model carries its own
			// TableName() and column/index tags, so the migration reuses it
			// directly rather than duplicating the field list.
			return dbmigrate.CreateTable(tx, &CITrust{})
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Migrator().DropTable(&CITrust{})
		},
	}
}

// migrationConverge is the new-ID convergence migration for ci_trusts
// (SDLC-CONF-09, SDLC-CONF-11, issue #862). Like ci_providers, the original
// migration above added TableName()+CreateTable under its already-shipped ID
// 2026102600000002, so upgraded hubs skipped the corrected body and kept the
// GORM-default-mangled "c_itrusts" table. A body fix under the shipped ID cannot
// reach those environments; convergence must run under a NEW ID that runs on
// every environment regardless of prior state.
//
// CITrust is an unused Kind and the mis-shipped "c_itrusts" table is empty
// (CI-trust writes could never resolve the mangled schema), so the simplest
// correct convergence is to drop the wrong table and create the clean one - no
// row copying. On a fresh or already-converged database the DROP IF EXISTS is a
// no-op and CreateTable recreates the clean table idempotently.
func migrationConverge() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "2026103000000002",
		Migrate: func(tx *gorm.DB) error {
			if err := tx.Exec("DROP TABLE IF EXISTS c_itrusts").Error; err != nil {
				return err
			}
			return dbmigrate.CreateTable(tx, &CITrust{})
		},
		// Convergence is not reversible: the stale table is intentionally dropped.
		Rollback: func(tx *gorm.DB) error { return nil },
	}
}
