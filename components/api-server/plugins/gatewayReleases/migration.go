package gatewayReleases

import (
	"gorm.io/gorm"

	"github.com/go-gormigrate/gormigrate/v2"
	"github.com/openshift-online/rh-trex-ai/pkg/db"
)

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

func migrationDropFleetId() *gormigrate.Migration {
	type GatewayRelease struct{ db.Model }

	return &gormigrate.Migration{
		ID: "2026082813000003",
		Migrate: func(tx *gorm.DB) error {
			if tx.Migrator().HasColumn(&GatewayRelease{}, "fleet_id") {
				return tx.Migrator().DropColumn(&GatewayRelease{}, "fleet_id")
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			return nil
		},
	}
}
