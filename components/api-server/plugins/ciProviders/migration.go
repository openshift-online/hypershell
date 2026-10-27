package ciProviders

import (
	"gorm.io/gorm"

	"github.com/go-gormigrate/gormigrate/v2"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/db"
)

func migration() *gormigrate.Migration {
	type CIProvider struct {
		db.Model
		Forge          string
		Issuer         string `gorm:"uniqueIndex"`
		JwksUrl        string
		Audience       string
		StatusIdpAlias *string
	}

	return &gormigrate.Migration{
		ID: "2026102600000001",
		Migrate: func(tx *gorm.DB) error {
			return tx.AutoMigrate(&CIProvider{})
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Migrator().DropTable(&CIProvider{})
		},
	}
}
