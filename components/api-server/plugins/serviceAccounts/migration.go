package serviceAccounts

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"github.com/openshift-online/rh-trex-ai/pkg/db"
	"gorm.io/gorm"
)

func migration() *gormigrate.Migration {
	funcTable := "open_shell_gateway_service_accounts"
	return &gormigrate.Migration{
		ID: "2026082115000001",
		Migrate: func(tx *gorm.DB) error {
			if err := tx.AutoMigrate(&OpenShellGatewayServiceAccount{}, &AuditEvent{}); err != nil {
				return err
			}
			if err := db.CreateFK(tx,
				db.FKMigration{Model: funcTable, Dest: "gateway", Field: "gateway_id", Reference: "gateways(id)"},
				db.FKMigration{Model: funcTable, Dest: "creator_user", Field: "created_by_user_id", Reference: "users(id)"},
			); err != nil {
				return err
			}
			if err := tx.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_gateway_service_accounts_active_name ON " + funcTable + " (gateway_id, lower(name)) WHERE deleted_at IS NULL AND status NOT IN ('expired', 'revoked')").Error; err != nil {
				return err
			}
			if err := tx.Exec("CREATE INDEX IF NOT EXISTS idx_gateway_service_accounts_reconcile ON " + funcTable + " (status, expires_at) WHERE deleted_at IS NULL").Error; err != nil {
				return err
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			if err := tx.Migrator().DropTable(&AuditEvent{}); err != nil {
				return err
			}
			return tx.Migrator().DropTable(&OpenShellGatewayServiceAccount{})
		},
	}
}
