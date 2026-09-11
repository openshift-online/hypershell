package managedClusters

import (
	"gorm.io/gorm"

	"github.com/go-gormigrate/gormigrate/v2"
	"github.com/openshift-online/rh-trex-ai/pkg/db"
)

func migrationAddRegistrationFields() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "2026091000000001",
		Migrate: func(tx *gorm.DB) error {
			if err := tx.Exec(`
				ALTER TABLE managed_clusters
					ADD COLUMN IF NOT EXISTS oidc_subject TEXT,
					ADD COLUMN IF NOT EXISTS last_seen_at TIMESTAMP WITH TIME ZONE
			`).Error; err != nil {
				return err
			}
			return tx.Exec(`
				CREATE UNIQUE INDEX IF NOT EXISTS uix_managed_clusters_oidc_subject_name
					ON managed_clusters (oidc_subject, name)
					WHERE oidc_subject IS NOT NULL AND oidc_subject <> ''
			`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			if err := tx.Exec(`DROP INDEX IF EXISTS uix_managed_clusters_oidc_subject_name`).Error; err != nil {
				return err
			}
			return tx.Exec(`
				ALTER TABLE managed_clusters
					DROP COLUMN IF EXISTS oidc_subject,
					DROP COLUMN IF EXISTS last_seen_at
			`).Error
		},
	}
}

func migrationAddTraceContext() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "2026082500000005",
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec(`
				ALTER TABLE managed_clusters
					ADD COLUMN IF NOT EXISTS traceparent TEXT,
					ADD COLUMN IF NOT EXISTS tracestate TEXT
			`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`
				ALTER TABLE managed_clusters
					DROP COLUMN IF EXISTS traceparent,
					DROP COLUMN IF EXISTS tracestate
			`).Error
		},
	}
}

func migration() *gormigrate.Migration {
	type ManagedCluster struct {
		db.Model
		Name             string
		FleetId          string
		Provider         string
		Region           *string
		KubeconfigSecret string
		Status           *string
		ApiServerUrl     *string
	}

	return &gormigrate.Migration{
		ID: "2026080312543137",
		Migrate: func(tx *gorm.DB) error {
			return tx.AutoMigrate(&ManagedCluster{})
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Migrator().DropTable(&ManagedCluster{})
		},
	}
}

func migrationDropFleetId() *gormigrate.Migration {
	type ManagedCluster struct{ db.Model }

	return &gormigrate.Migration{
		ID: "2026082813000004",
		Migrate: func(tx *gorm.DB) error {
			if tx.Migrator().HasColumn(&ManagedCluster{}, "fleet_id") {
				return tx.Migrator().DropColumn(&ManagedCluster{}, "fleet_id")
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			return nil
		},
	}
}
