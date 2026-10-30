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
