package ciTrusts

import (
	"gorm.io/gorm"

	"github.com/go-gormigrate/gormigrate/v2"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/db"
)

func migration() *gormigrate.Migration {
	type CITrust struct {
		db.Model
		CIProviderID           string `gorm:"column:ci_provider_id;index"`
		GatewayID              string `gorm:"column:gateway_id;index"`
		SubjectRepository      *string
		SubjectProjectPath     *string
		SubjectRef             *string
		SubjectEnvironment     *string
		SubjectRefProtected    *bool
		Role                   string
		StatusExchangeEndpoint *string
		StatusWorkspace        *string
		StatusWorkflowHint     *string
	}

	return &gormigrate.Migration{
		ID: "2026102600000002",
		Migrate: func(tx *gorm.DB) error {
			return tx.AutoMigrate(&CITrust{})
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Migrator().DropTable(&CITrust{})
		},
	}
}
