package providerBindings

import (
	"gorm.io/gorm"

	"github.com/go-gormigrate/gormigrate/v2"
	"github.com/openshift-online/hypershell/components/api-server/pkg/dbmigrate"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/db"
)

func migration() *gormigrate.Migration {
	type ProviderBinding struct {
		db.Model
		Name            string
		WorkspaceId     string
		ProviderSpecId  string
		SecretSourceId  string
		RefreshStrategy *string `gorm:"type:jsonb"`
		Status          *string
	}

	return &gormigrate.Migration{
		ID: "2026100600000013",
		Migrate: func(tx *gorm.DB) error {
			return dbmigrate.CreateTable(tx, &ProviderBinding{})
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Migrator().DropTable(&ProviderBinding{})
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
		ID: "2026101500000003",
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec(`
				ALTER TABLE provider_bindings
					ADD COLUMN IF NOT EXISTS repository_id TEXT
			`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`
				ALTER TABLE provider_bindings
					DROP COLUMN IF EXISTS repository_id
			`).Error
		},
	}
}
