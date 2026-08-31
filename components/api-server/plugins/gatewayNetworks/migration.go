package gatewayNetworks

import (
	"gorm.io/gorm"

	"github.com/go-gormigrate/gormigrate/v2"
	"github.com/openshift-online/rh-trex-ai/pkg/db"
)

func migrationAddTraceContext() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "2026082500000003",
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec(`
				ALTER TABLE gateway_networks
					ADD COLUMN IF NOT EXISTS traceparent TEXT,
					ADD COLUMN IF NOT EXISTS tracestate TEXT
			`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`
				ALTER TABLE gateway_networks
					DROP COLUMN IF EXISTS traceparent,
					DROP COLUMN IF EXISTS tracestate
			`).Error
		},
	}
}

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
