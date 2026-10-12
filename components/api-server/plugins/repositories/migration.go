package repositories

import (
	"gorm.io/gorm"

	"github.com/go-gormigrate/gormigrate/v2"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/db"
)

func migration() *gormigrate.Migration {
	type Repository struct {
		db.Model
		Name           string
		Url            string
		Provider       string
		SecretSourceId string
		DefaultBranch  *string
		ImportPath     *string
		Status         *string
	}

	return &gormigrate.Migration{
		ID: "2026101000000001",
		Migrate: func(tx *gorm.DB) error {
			return tx.AutoMigrate(&Repository{})
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Migrator().DropTable(&Repository{})
		},
	}
}
