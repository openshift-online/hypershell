package secretSources

import (
	"gorm.io/gorm"

	"github.com/go-gormigrate/gormigrate/v2"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/db"
)

func migration() *gormigrate.Migration {
	type SecretSource struct {
		db.Model
		Name           string
		AgentRuntimeId string
		Purpose        string
		Backend        string
		Path           string
		KeyMappings    *string `gorm:"type:jsonb"`
	}

	return &gormigrate.Migration{
		ID: "2026100600000015",
		Migrate: func(tx *gorm.DB) error {
			return tx.AutoMigrate(&SecretSource{})
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Migrator().DropTable(&SecretSource{})
		},
	}
}
