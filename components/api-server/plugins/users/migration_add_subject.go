package users

import (
	"gorm.io/gorm"

	"github.com/go-gormigrate/gormigrate/v2"
)

// migrationAddSubject adds the internal-only subject column (SEC-RBAC-03,
// PLAT-DATA-27) under a NEW migration ID: users already shipped its table
// under ID 2026081112000001, and that body is skipped on upgraded hubs (the
// #862 lesson). Raw ALTER TABLE is used, never AutoMigrate (SDLC-CONF-02).
// subject has no unique index: it is nullable and empty on every pre-existing
// row, and uniqueness is not a requirement of SEC-RBAC-03.
func migrationAddSubject() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "2026110600000002",
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec(`ALTER TABLE users ADD COLUMN IF NOT EXISTS subject TEXT NOT NULL DEFAULT ''`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`ALTER TABLE users DROP COLUMN IF EXISTS subject`).Error
		},
	}
}
