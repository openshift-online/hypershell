package defaultWorkspaceBootstrap

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"github.com/openshift-online/hypershell/components/api-server/pkg/dbmigrate"
	"gorm.io/gorm"
)

func migration() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "2026110915000001",
		Migrate: func(tx *gorm.DB) error {
			return dbmigrate.CreateTable(tx, &Attempt{})
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Migrator().DropTable(&Attempt{})
		},
	}
}
