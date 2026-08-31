package gatewayReleases

import (
	"gorm.io/gorm"

	"github.com/go-gormigrate/gormigrate/v2"
	"github.com/openshift-online/rh-trex-ai/pkg/db"
)

func migrationAddTraceContext() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "2026082500000004",
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec(`
				ALTER TABLE gateway_releases
					ADD COLUMN IF NOT EXISTS traceparent TEXT,
					ADD COLUMN IF NOT EXISTS tracestate TEXT
			`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`
				ALTER TABLE gateway_releases
					DROP COLUMN IF EXISTS traceparent,
					DROP COLUMN IF EXISTS tracestate
			`).Error
		},
	}
}

func migration() *gormigrate.Migration {
	type GatewayRelease struct {
		db.Model
		Name            string
		FleetId         string
		Image           string
		RolloutStrategy *string
		CanaryPercent   *int
		CanaryDuration  *string
		Status          *string
	}

	return &gormigrate.Migration{
		ID: "2026080312541895",
		Migrate: func(tx *gorm.DB) error {
			return tx.AutoMigrate(&GatewayRelease{})
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Migrator().DropTable(&GatewayRelease{})
		},
	}
}
