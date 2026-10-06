package agentRuntimes

import (
	"gorm.io/gorm"

	"github.com/go-gormigrate/gormigrate/v2"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/db"
)

func migration() *gormigrate.Migration {
	type AgentRuntime struct {
		db.Model
		Name                string
		ClusterId           string
		GatewayId           string
		SandboxTemplateId   string
		Description         *string
		Cron                *string
		CoordinatorImage    *string
		ConcurrencyPolicy   *string
		LoginRefreshSeconds *int32
		Parameters          *string `gorm:"type:jsonb"`
		Status              *string
	}

	return &gormigrate.Migration{
		ID: "2026100600000010",
		Migrate: func(tx *gorm.DB) error {
			return tx.AutoMigrate(&AgentRuntime{})
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Migrator().DropTable(&AgentRuntime{})
		},
	}
}
