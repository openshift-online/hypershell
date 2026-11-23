package platformGatewayPolicies

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"

	"github.com/openshift-online/hypershell/components/api-server/pkg/dbmigrate"
)

// canaryCedar is the built-in e2e enforcement canary (PLAT-PGP-20). It applies
// only to sandboxes named with the reserved prefix, so it never affects a real
// user.
const canaryCedar = `forbid (principal, action == OpenShell::Action::"CreateSandbox", resource)
when { context.sandboxName like "hypershell-e2e-policy-*" && context.cpuRequestMillis > 1000 };`

const canaryUserMessage = "Used by HyperShell release checks. Applies only to sandboxes named hypershell-e2e-policy-*: they may request at most 1 CPU."

// migration creates the three PlatformGatewayPolicy tables (PLAT-DATA-28/29/30)
// with raw SQL (SDLC-CONF-02, never AutoMigrate). The policy table reuses the
// GORM schema of the model; the settings and findings tables are explicit DDL
// because findings use a text[] column with a GIN index. A partial unique index
// keeps one live policy per name without blocking delete-then-recreate
// (SDLC-CONF-13). Idempotent.
func migration() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "2026112310000001",
		Migrate: func(tx *gorm.DB) error {
			if err := dbmigrate.CreateTable(tx, &PlatformGatewayPolicy{}); err != nil {
				return err
			}
			stmts := []string{
				`CREATE UNIQUE INDEX IF NOT EXISTS idx_platform_gateway_policies_name ON platform_gateway_policies (name) WHERE deleted_at IS NULL`,
				`CREATE TABLE IF NOT EXISTS platform_policy_settings (
					id TEXT PRIMARY KEY,
					mode TEXT NOT NULL,
					updated_by TEXT NOT NULL DEFAULT '',
					updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
				)`,
				`CREATE TABLE IF NOT EXISTS policy_audit_findings (
					id TEXT PRIMARY KEY,
					occurred_at TIMESTAMPTZ NOT NULL,
					gateway_id TEXT NOT NULL,
					policy_names TEXT[] NOT NULL DEFAULT '{}',
					bundle_revision TEXT NOT NULL DEFAULT '',
					principal_subject TEXT NOT NULL DEFAULT '',
					operation TEXT NOT NULL DEFAULT '',
					outcome TEXT NOT NULL,
					request_summary JSONB NOT NULL DEFAULT '{}'::jsonb,
					created_at TIMESTAMPTZ NOT NULL DEFAULT now()
				)`,
				`CREATE INDEX IF NOT EXISTS idx_policy_audit_findings_gateway_time ON policy_audit_findings (gateway_id, occurred_at DESC)`,
				`CREATE INDEX IF NOT EXISTS idx_policy_audit_findings_policy_names ON policy_audit_findings USING GIN (policy_names)`,
				`CREATE INDEX IF NOT EXISTS idx_policy_audit_findings_occurred_at ON policy_audit_findings (occurred_at)`,
			}
			for _, s := range stmts {
				if err := tx.Exec(s).Error; err != nil {
					return err
				}
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			for _, s := range []string{
				`DROP TABLE IF EXISTS policy_audit_findings`,
				`DROP TABLE IF EXISTS platform_policy_settings`,
				`DROP TABLE IF EXISTS platform_gateway_policies`,
			} {
				if err := tx.Exec(s).Error; err != nil {
					return err
				}
			}
			return nil
		},
	}
}

// migrationSeedCanary seeds the built-in e2e-enforcement-canary policy
// (PLAT-PGP-20): target all, enforcement_mode enforce, built_in true. It is
// additive and idempotent: an existing live row with the name is left as is, so
// a re-run never alters the policy or its status.
func migrationSeedCanary() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID:      "2026112310000002",
		Migrate: SeedCanary,
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`DELETE FROM platform_gateway_policies WHERE name = ? AND built_in = true`, CanaryPolicyName).Error
		},
	}
}

// SeedCanary inserts the built-in canary when no live policy has its name. It is
// the body of migrationSeedCanary and is reused by the integration harness after
// it truncates the database.
func SeedCanary(tx *gorm.DB) error {
	return tx.Exec(`
				INSERT INTO platform_gateway_policies
					(id, created_at, updated_at, name, description, target, enforcement_mode, user_message, documentation_url, cedar, built_in, status)
				SELECT ?, now(), now(), ?, ?, '{"all":true}'::jsonb, 'enforce', ?, '', ?, true, '{}'::jsonb
				WHERE NOT EXISTS (
					SELECT 1 FROM platform_gateway_policies WHERE name = ? AND deleted_at IS NULL
				)`,
		"e2e-enforcement-canary-builtin",
		CanaryPolicyName,
		"Built-in release-check canary. Rejects sandboxes named hypershell-e2e-policy-* that request more than 1 CPU, so the e2e gate proves enforcement.",
		canaryUserMessage,
		canaryCedar,
		CanaryPolicyName,
	).Error
}
