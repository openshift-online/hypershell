package inferenceRoutes

import (
	"gorm.io/gorm"

	"github.com/go-gormigrate/gormigrate/v2"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/db"
)

func migration() *gormigrate.Migration {
	type InferenceRoute struct {
		db.Model
		WorkspaceId       string
		ProviderBindingId string
		ModelAlias        string `gorm:"column:model"`
	}

	return &gormigrate.Migration{
		ID: "2026100600000014",
		Migrate: func(tx *gorm.DB) error {
			return tx.AutoMigrate(&InferenceRoute{})
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Migrator().DropTable(&InferenceRoute{})
		},
	}
}
