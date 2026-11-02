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
// lesson). It never uses AutoMigrate (SDLC-CONF-02).
//
// The name is backfilled from the FULL composite natural key
// (workspace_id/subject, matching WorkspaceMembership.DeriveName) so it is unique
// by construction for distinct memberships, and a UNIQUE index makes find-by-name
// resolve exactly one row. Each identity term is COALESCE'd because `a || '/' || b`
// yields NULL if ANY term is NULL. Two rows can still share the same composite
// identity on pre-existing data, so the shared helper de-duplicates colliding names
// collision-safely BEFORE creating the index, so the unique index can never fail
// and brick startup (FINDING 1/3). See dbmigrate.AddUniqueNameColumn.
func migrationAddName() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "2026110213000004",
		Migrate: func(tx *gorm.DB) error {
			return dbmigrate.AddUniqueNameColumn(tx, "workspace_memberships",
				`COALESCE(workspace_id,'') || '/' || COALESCE(subject,'')`)
		},
		Rollback: func(tx *gorm.DB) error {
			return dbmigrate.DropUniqueNameColumn(tx, "workspace_memberships")
		},
	}
}
