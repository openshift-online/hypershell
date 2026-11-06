package defaultWorkspaceMemberships

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"github.com/openshift-online/hypershell/components/api-server/pkg/dbmigrate"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/db"
	"gorm.io/gorm"
)

func migration() *gormigrate.Migration {
	table := "default_workspace_memberships"
	auditTable := "default_workspace_membership_audit_events"
	return &gormigrate.Migration{
		ID: "2026110600000001",
		Migrate: func(tx *gorm.DB) error {
			if err := dbmigrate.CreateTable(tx, &DefaultWorkspaceMembership{}, &AuditEvent{}); err != nil {
				return err
			}
			if err := db.CreateFK(tx,
				db.FKMigration{Model: table, Dest: "gateway", Field: "gateway_id", Reference: "gateways(id)"},
			); err != nil {
				return err
			}
			// Unique live (gateway_id, subject) (PLAT-DATA-27, "Uniqueness" /
			// PLAT-DATA-27.01): at most one live row per gateway and subject.
			if err := tx.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_default_workspace_memberships_live ON " + table + " (gateway_id, subject) WHERE deleted_at IS NULL").Error; err != nil {
				return err
			}
			if err := tx.Exec("CREATE INDEX IF NOT EXISTS idx_default_workspace_membership_audit_gateway ON " + auditTable + " (gateway_id)").Error; err != nil {
				return err
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			if err := tx.Migrator().DropTable(&AuditEvent{}); err != nil {
				return err
			}
			return tx.Migrator().DropTable(&DefaultWorkspaceMembership{})
		},
	}
}
