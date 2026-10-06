package providerBindings

import (
	"gorm.io/gorm"

	"github.com/go-gormigrate/gormigrate/v2"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/db"
)

func migration() *gormigrate.Migration {
	type ProviderBinding struct {
		db.Model
		Name            string
		WorkspaceId     string
		ProviderSpecId  string
		SecretSourceId  string
		RefreshStrategy *string `gorm:"type:jsonb"`
		Status          *string
	}

	return &gormigrate.Migration{
		ID: "2026100600000013",
		Migrate: func(tx *gorm.DB) error {
			return tx.AutoMigrate(&ProviderBinding{})
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Migrator().DropTable(&ProviderBinding{})
		},
	}
}
