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
// under a NEW ID. This runs on every environment regardless of prior state,
// creating ci_providers if missing, copying any rows off c_iproviders, and
// dropping it. It is a no-op on a healthy or fresh database.
func migrationConverge() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "2026103000000001",
		Migrate: func(tx *gorm.DB) error {
			return dbmigrate.ConvergeRenamedTable(tx, &CIProvider{}, "c_iproviders")
		},
		// Convergence is not reversible: the stale table is intentionally dropped.
		Rollback: func(tx *gorm.DB) error { return nil },
	}
}
