package workspaceMemberships

import (
	"gorm.io/gorm"

	"github.com/go-gormigrate/gormigrate/v2"
	"github.com/openshift-online/hypershell/components/api-server/pkg/dbmigrate"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/db"
)

func migration() *gormigrate.Migration {
	type WorkspaceMembership struct {
		db.Model
		WorkspaceId string
		Subject     string
		Role        string
	}

	return &gormigrate.Migration{
		ID: "2026101700000002",
		Migrate: func(tx *gorm.DB) error {
			return dbmigrate.CreateTable(tx, &WorkspaceMembership{})
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Migrator().DropTable(&WorkspaceMembership{})
		},
	}
}

// migrationAddName adds the queryable name column (SDLC-CONF-13) under a NEW
// migration ID, because workspace_memberships already shipped its table under ID
// 2026101700000002; editing that body is skipped on upgraded hubs (the #862
// lesson). The add and backfill use idempotent raw SQL safe under the production
// lib/pq + PreferSimpleProtocol driver (never AutoMigrate, SDLC-CONF-02).
// Existing rows are backfilled with a stable identity derived from
// workspace_id/subject (matching WorkspaceMembership.DeriveName) so a
// find-by-name search resolves pre-existing records.
func migrationAddName() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "2026110213000004",
		Migrate: func(tx *gorm.DB) error {
			if err := tx.Exec(`ALTER TABLE workspace_memberships ADD COLUMN IF NOT EXISTS name text`).Error; err != nil {
				return err
			}
			// COALESCE each identity term: in SQL `a || '/' || b` yields NULL if ANY
			// term is NULL, which would leave the row's name NULL and unresolvable
			// by find-by-name. COALESCE(col,'') matches
			// WorkspaceMembership.DeriveName, which joins the Go string fields
			// (empty, never NULL) (SDLC-CONF-13).
			return tx.Exec(`UPDATE workspace_memberships SET name = COALESCE(workspace_id,'') || '/' || COALESCE(subject,'') WHERE name IS NULL OR name = ''`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`ALTER TABLE workspace_memberships DROP COLUMN IF EXISTS name`).Error
		},
	}
}
