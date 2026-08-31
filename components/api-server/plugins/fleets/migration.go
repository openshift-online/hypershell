package fleets

import (
	"gorm.io/gorm"

	"github.com/go-gormigrate/gormigrate/v2"
	"github.com/openshift-online/rh-trex-ai/pkg/db"
)

func migrationAddTraceContext() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "2026082500000001",
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec(`
				ALTER TABLE fleets
					ADD COLUMN IF NOT EXISTS traceparent TEXT,
					ADD COLUMN IF NOT EXISTS tracestate TEXT
			`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`
				ALTER TABLE fleets
					DROP COLUMN IF EXISTS traceparent,
					DROP COLUMN IF EXISTS tracestate
			`).Error
		},
	}
}

func migration() *gormigrate.Migration {
	type Fleet struct {
		db.Model
		Name        string
		Description *string
		Status      *string
	}

	return &gormigrate.Migration{
		ID: "2026080312501188",
		Migrate: func(tx *gorm.DB) error {
			return tx.AutoMigrate(&Fleet{})
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Migrator().DropTable(&Fleet{})
		},
	}
}
