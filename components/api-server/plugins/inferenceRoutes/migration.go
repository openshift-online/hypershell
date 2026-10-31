package inferenceRoutes

import (
	"gorm.io/gorm"

	"github.com/go-gormigrate/gormigrate/v2"
	"github.com/openshift-online/hypershell/components/api-server/pkg/dbmigrate"
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
			return dbmigrate.CreateTable(tx, &InferenceRoute{})
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Migrator().DropTable(&InferenceRoute{})
		},
	}
}

// migrationAddRepositoryOwnership adds the nullable repository_id ownership mark.
// It is NULL for resources created directly through the REST API or hsctl apply,
// and is set by the control-plane declarative-import reconciler for resources it
// imports from a Repository's .hypershell directory, scoping prune to the owning
// Repository. Raw ALTER TABLE is used instead of AutoMigrate because AutoMigrate's
// add-column path is unreliable under the test session's lib/pq driver with
// PreferSimpleProtocol.
func migrationAddRepositoryOwnership() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "2026101500000004",
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec(`
				ALTER TABLE inference_routes
					ADD COLUMN IF NOT EXISTS repository_id TEXT
			`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`
				ALTER TABLE inference_routes
					DROP COLUMN IF EXISTS repository_id
			`).Error
		},
	}
}
