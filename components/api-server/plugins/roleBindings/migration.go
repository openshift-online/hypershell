package roleBindings

import (
	"gorm.io/gorm"

	"github.com/go-gormigrate/gormigrate/v2"
	"github.com/openshift-online/rh-trex-ai/pkg/db"
)

func migration() *gormigrate.Migration {
	type RoleBinding struct {
		db.Model
		RoleID    string `gorm:"index"`
		Scope     string
		UserID    *string `gorm:"index"`
		GatewayID *string `gorm:"index"`
	}

	return &gormigrate.Migration{
		ID: "2026081112000003",
		Migrate: func(tx *gorm.DB) error {
			return tx.AutoMigrate(&RoleBinding{})
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Migrator().DropTable(&RoleBinding{})
		},
	}
}
