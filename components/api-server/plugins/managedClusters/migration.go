package managedClusters

import (
	"fmt"
	"strings"

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

// migrationUniqueName makes managed_clusters.name unique across all live
// records, registered or not, so discovery by name (seed scripts, e2e,
// operators) is unambiguous and a registration can never create a second
// record with a taken name (managed-cluster-registration.spec.md). Soft-deleted
// rows are excluded so an operator deleting a colliding record frees the name.
//
// Pre-existing duplicates are never deleted or renamed here: the migration
// fails with a message listing them, and an operator resolves them first.
func migrationUniqueName() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "2026092400000001",
		Migrate: func(tx *gorm.DB) error {
			type duplicate struct {
				Name string
				IDs  string
			}
			var dups []duplicate
			if err := tx.Raw(`
				SELECT name, string_agg(id, ', ' ORDER BY created_at) AS ids
				FROM managed_clusters
				WHERE deleted_at IS NULL
				GROUP BY name
				HAVING count(*) > 1
				ORDER BY name
			`).Scan(&dups).Error; err != nil {
				return fmt.Errorf("check managed_clusters for duplicate names: %w", err)
			}
			if len(dups) > 0 {
				parts := make([]string, 0, len(dups))
				for _, d := range dups {
					parts = append(parts, fmt.Sprintf("%q (ids: %s)", d.Name, d.IDs))
				}
				return fmt.Errorf("cannot add unique index on managed_clusters.name: duplicate live names %s; delete or rename the extra records (and re-point gateways that reference them) and restart", strings.Join(parts, "; "))
			}
			return tx.Exec(`
				CREATE UNIQUE INDEX IF NOT EXISTS uix_managed_clusters_name
					ON managed_clusters (name)
					WHERE deleted_at IS NULL
			`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`DROP INDEX IF EXISTS uix_managed_clusters_name`).Error
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
