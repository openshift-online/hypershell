package gatewayNetworks

import (
	"gorm.io/gorm"

	"github.com/go-gormigrate/gormigrate/v2"
	"github.com/openshift-online/rh-trex-ai/pkg/db"
)

func migration() *gormigrate.Migration {
	type GatewayNetwork struct {
		db.Model
		Name         string
		FleetId      string
		Topology     *string
		TunnelMode   *string
		HubGatewayId *string
		Status       *string
	}

	return &gormigrate.Migration{
		ID: "2026080312548062",
		Migrate: func(tx *gorm.DB) error {
			return tx.AutoMigrate(&GatewayNetwork{})
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Migrator().DropTable(&GatewayNetwork{})
		},
	}
}

func migrationDropFleetId() *gormigrate.Migration {
	type GatewayNetwork struct{ db.Model }

	return &gormigrate.Migration{
		ID: "2026082813000002",
		Migrate: func(tx *gorm.DB) error {
			if tx.Migrator().HasColumn(&GatewayNetwork{}, "fleet_id") {
				return tx.Migrator().DropColumn(&GatewayNetwork{}, "fleet_id")
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			return nil
		},
	}
}
