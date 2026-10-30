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
// GORM-default-mangled "c_itrusts" table. This new-ID migration runs on every
// environment, creating ci_trusts if missing, copying any rows off c_itrusts
// (over the columns common to both; the stale table's ci_provider_id column is
// itself mangled to c_iprovider_id, which the common-column copy handles safely),
// and dropping it. It is a no-op on a healthy or fresh database.
func migrationConverge() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "2026103000000002",
		Migrate: func(tx *gorm.DB) error {
			return dbmigrate.ConvergeRenamedTable(tx, &CITrust{}, "c_itrusts")
		},
		// Convergence is not reversible: the stale table is intentionally dropped.
		Rollback: func(tx *gorm.DB) error { return nil },
	}
}
