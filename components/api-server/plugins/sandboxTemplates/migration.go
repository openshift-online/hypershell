package sandboxTemplates

import (
	"gorm.io/gorm"

	"github.com/go-gormigrate/gormigrate/v2"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/db"
)

func migration() *gormigrate.Migration {
	type SandboxTemplate struct {
		db.Model
		Name       string
		Image      string
		NamePrefix *string
		Policy     *string `gorm:"type:jsonb"`
		Status     *string
	}

	return &gormigrate.Migration{
		ID: "2026100600000011",
		Migrate: func(tx *gorm.DB) error {
			return tx.AutoMigrate(&SandboxTemplate{})
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Migrator().DropTable(&SandboxTemplate{})
		},
	}
}
