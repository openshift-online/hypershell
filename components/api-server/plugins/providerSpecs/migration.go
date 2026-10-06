package providerSpecs

import (
	"gorm.io/gorm"

	"github.com/go-gormigrate/gormigrate/v2"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/db"
)

func migration() *gormigrate.Migration {
	type ProviderSpec struct {
		db.Model
		Name       string
		Category   string
		Capability *string `gorm:"type:jsonb"`
		Profile    *string `gorm:"type:jsonb"`
		Status     *string
	}

	return &gormigrate.Migration{
		ID: "2026100600000012",
		Migrate: func(tx *gorm.DB) error {
			return tx.AutoMigrate(&ProviderSpec{})
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Migrator().DropTable(&ProviderSpec{})
		},
	}
}
