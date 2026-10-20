package agentWorkspaces

import (
	"gorm.io/gorm"

	"github.com/go-gormigrate/gormigrate/v2"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/db"
)

func migration() *gormigrate.Migration {
	type AgentWorkspace struct {
		db.Model
		AgentRuntimeId string
		GatewayId      string
		Name           string
		Status         *string
	}

	return &gormigrate.Migration{
		ID: "2026101700000001",
		Migrate: func(tx *gorm.DB) error {
			return tx.AutoMigrate(&AgentWorkspace{})
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Migrator().DropTable(&AgentWorkspace{})
		},
	}
}
