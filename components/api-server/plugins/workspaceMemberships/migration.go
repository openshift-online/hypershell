package workspaceMemberships

import (
	"gorm.io/gorm"

	"github.com/go-gormigrate/gormigrate/v2"
	"github.com/openshift-online/hypershell/components/api-server/pkg/dbmigrate"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/db"
)

func migration() *gormigrate.Migration {
	type WorkspaceMembership struct {
		db.Model
		WorkspaceId string
		Subject     string
		Role        string
	}

	return &gormigrate.Migration{
		ID: "2026101700000002",
		Migrate: func(tx *gorm.DB) error {
			return dbmigrate.CreateTable(tx, &WorkspaceMembership{})
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Migrator().DropTable(&WorkspaceMembership{})
		},
	}
}
