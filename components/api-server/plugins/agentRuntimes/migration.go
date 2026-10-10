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

// migrationAddRepositorySelector adds the nullable repository_id foreign key and
// the jsonb selector label list. Raw ALTER TABLE is used instead of AutoMigrate
// because AutoMigrate's add-column path is unreliable under the test session's
// lib/pq driver with PreferSimpleProtocol.
func migrationAddRepositorySelector() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "2026101000000002",
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec(`
				ALTER TABLE agent_runtimes
					ADD COLUMN IF NOT EXISTS repository_id TEXT,
					ADD COLUMN IF NOT EXISTS selector JSONB
			`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`
				ALTER TABLE agent_runtimes
					DROP COLUMN IF EXISTS repository_id,
					DROP COLUMN IF EXISTS selector
			`).Error
		},
	}
}
