package gateways

import (
	"encoding/json"
	"fmt"

	"gorm.io/gorm"

	"github.com/go-gormigrate/gormigrate/v2"
	"github.com/openshift-online/hypershell/components/api-server/pkg/dbmigrate"
	"github.com/openshift-online/hypershell/components/api-server/plugins/managedClusters"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/db"
)

func migrationAddTraceContext() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "2026082500000002",
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec(`
				ALTER TABLE gateways
					ADD COLUMN IF NOT EXISTS traceparent TEXT,
					ADD COLUMN IF NOT EXISTS tracestate TEXT
			`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`
				ALTER TABLE gateways
					DROP COLUMN IF EXISTS traceparent,
					DROP COLUMN IF EXISTS tracestate
			`).Error
		},
	}
}

func migration() *gormigrate.Migration {
	type Gateway struct {
		db.Model
		Name        string
		FleetId     string
		ClusterId   string
		ReleaseId   string
		DatabaseId  string
		Namespace   string
		ExternalDns *string
		TlsMode     *string
		ServiceType *string
		Status      *string
		Phase       *string
	}

	return &gormigrate.Migration{
		ID: "2026080312546877",
		Migrate: func(tx *gorm.DB) error {
			return dbmigrate.CreateTable(tx, &Gateway{})
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Migrator().DropTable(&Gateway{})
		},
	}
}

// Column-adding migrations use raw idempotent DDL rather than tx.AutoMigrate.
// AutoMigrate on an already-existing table calls the postgres migrator's
// ColumnTypes introspection, whose query places a clause.Expr (CURRENT_SCHEMA())
// after a scalar bind parameter. Under the framework's testcontainer stack
// (gorm PreferSimpleProtocol + lib/pq), that ordering breaks placeholder
// renumbering and fails with "pq: got 2 parameters but the statement requires 1".
// Explicit "ADD COLUMN IF NOT EXISTS" avoids the introspection path entirely and
// stays schema-equivalent to what AutoMigrate produced in already-migrated
// environments.
func migrationAddProvisioningFields() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "2026080712000001",
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec(`ALTER TABLE gateways
				ADD COLUMN IF NOT EXISTS image TEXT,
				ADD COLUMN IF NOT EXISTS server_dns_names JSONB,
				ADD COLUMN IF NOT EXISTS route_address TEXT,
				ADD COLUMN IF NOT EXISTS oidc JSONB,
				ADD COLUMN IF NOT EXISTS route JSONB`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`ALTER TABLE gateways
				DROP COLUMN IF EXISTS image,
				DROP COLUMN IF EXISTS server_dns_names,
				DROP COLUMN IF EXISTS route_address,
				DROP COLUMN IF EXISTS oidc,
				DROP COLUMN IF EXISTS route`).Error
		},
	}
}

func migrationAddCredentialDriver() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "2026081112000005",
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec("ALTER TABLE gateways ADD COLUMN IF NOT EXISTS credential_driver JSONB").Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec("ALTER TABLE gateways DROP COLUMN IF EXISTS credential_driver").Error
		},
	}
}

// migrationBackfillLegacyCredentialDriverKEK pins pre-existing gateways that have no
// explicit credential_driver to kek-db, so the HYPERSHELL-369 flat vault default applies
// only to gateways created after it ships (HYPERSHELL-369, CWE-664). Before the flip an
// empty driver meant kek-db, and those gateways stored provider credentials in the gateway
// database; resolving them to vault on the next reconcile would make that data unreadable,
// and the immutability check does not catch an empty-to-vault transition. New gateways are
// unaffected -- they are created after this one-time backfill. Idempotent: rows with an
// explicit type (vault, kek-db, kubernetes-secrets) are left untouched, so a re-run is a
// no-op. Runs after migrationAddCredentialDriver (the column must exist first).
func migrationBackfillLegacyCredentialDriverKEK() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "2026102000000001",
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec(`UPDATE gateways
				SET credential_driver = '{"type":"kek-db"}'::jsonb
				WHERE credential_driver IS NULL
				   OR (credential_driver->>'type') IS NULL
				   OR (credential_driver->>'type') = ''`).Error
		},
		// Irreversible: the original NULL/empty values are not recoverable per-row, and
		// reverting would wrongly clear legitimately kek-db gateways. No-op rollback.
		Rollback: func(tx *gorm.DB) error { return nil },
	}
}

// migrationAddGatewayWorkspaceMode adds the per-gateway OpenShell workspace
// mode column (shared/managed/operator). Rows that predate it are backfilled by
// migrationBackfillGatewayWorkspaceModeShared.
func migrationAddGatewayWorkspaceMode() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "2026102012000006",
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec("ALTER TABLE gateways ADD COLUMN IF NOT EXISTS workspace_mode TEXT").Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec("ALTER TABLE gateways DROP COLUMN IF EXISTS workspace_mode").Error
		},
	}
}

// migrationBackfillGatewayWorkspaceModeShared records the mode that gateways
// created before the workspace_mode column already run in, then makes the column
// required. Those gateways were provisioned with the Helm value omitted, so the
// vendored chart default applied: shared. Writing "shared" (not the "managed"
// create default) keeps their sandboxes in the gateway namespace; the next Helm
// upgrade renders the same gateway.toml mode they run today. The update is
// idempotent, and the NOT NULL constraint means a stored mode is always present
// (openshell-gateway.spec.md, "OSG-CORE-18 -- Gateway Workspace Mode").
func migrationBackfillGatewayWorkspaceModeShared() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "2026110212000001",
		Migrate: func(tx *gorm.DB) error {
			if err := tx.Exec(`UPDATE gateways
				SET workspace_mode = 'shared'
				WHERE workspace_mode IS NULL OR btrim(workspace_mode) = ''`).Error; err != nil {
				return err
			}
			return tx.Exec("ALTER TABLE gateways ALTER COLUMN workspace_mode SET NOT NULL").Error
		},
		// The backfilled rows are not distinguishable from gateways whose stored
		// mode was already shared, so rollback only relaxes the constraint.
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec("ALTER TABLE gateways ALTER COLUMN workspace_mode DROP NOT NULL").Error
		},
	}
}

// migrationAddGatewayDedicatedRoute persists the create-only dedicated_route
// request flag. It is NULL for every gateway created without the request (and for
// every pre-existing gateway), which means "not requested": the dedicated
// per-gateway route and certificate path is still selected by the platform-managed
// route.certificate sub-key, so the control plane is unaffected. Raw idempotent
// ALTER TABLE is used instead of AutoMigrate because AutoMigrate's add-column
// introspection breaks under the test session's lib/pq driver with
// PreferSimpleProtocol (see migrationAddProvisioningFields).
func migrationAddGatewayDedicatedRoute() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "2026102812000001",
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec("ALTER TABLE gateways ADD COLUMN IF NOT EXISTS dedicated_route BOOLEAN").Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec("ALTER TABLE gateways DROP COLUMN IF EXISTS dedicated_route").Error
		},
	}
}

func migrationAddSupervisorImage() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "2026080712000002",
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec("ALTER TABLE gateways ADD COLUMN IF NOT EXISTS supervisor_image TEXT").Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec("ALTER TABLE gateways DROP COLUMN IF EXISTS supervisor_image").Error
		},
	}
}

func migrationAddConsoleAddress() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "2026081112000006",
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec("ALTER TABLE gateways ADD COLUMN IF NOT EXISTS console_address TEXT").Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec("ALTER TABLE gateways DROP COLUMN IF EXISTS console_address").Error
		},
	}
}

func migrationAddActiveSandboxCount() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "2026081712000006",
		Migrate: func(tx *gorm.DB) error {
			// gorm maps Go int to bigint on postgres; match that so already-migrated
			// environments keep an identical column type.
			return tx.Exec("ALTER TABLE gateways ADD COLUMN IF NOT EXISTS active_sandbox_count BIGINT").Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec("ALTER TABLE gateways DROP COLUMN IF EXISTS active_sandbox_count").Error
		},
	}
}

func migrationDropDatabaseConfig() *gormigrate.Migration {
	type Gateway struct{ db.Model }

	return &gormigrate.Migration{
		ID: "2026082012000001",
		Migrate: func(tx *gorm.DB) error {
			if tx.Migrator().HasColumn(&Gateway{}, "database_config") {
				return tx.Migrator().DropColumn(&Gateway{}, "database_config")
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			return nil
		},
	}
}

func migrationDropFleetId() *gormigrate.Migration {
	type Gateway struct{ db.Model }

	return &gormigrate.Migration{
		ID: "2026082813000001",
		Migrate: func(tx *gorm.DB) error {
			if tx.Migrator().HasColumn(&Gateway{}, "fleet_id") {
				return tx.Migrator().DropColumn(&Gateway{}, "fleet_id")
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			return nil
		},
	}
}

func migrationAddProvisioningConditions() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "2026091012000001",
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec("ALTER TABLE gateways ADD COLUMN IF NOT EXISTS provisioning_conditions JSONB").Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec("ALTER TABLE gateways DROP COLUMN IF EXISTS provisioning_conditions").Error
		},
	}
}

func migrationDropFleetsTable() *gormigrate.Migration {
	// The Fleet resource was removed platform-wide; drop its orphaned table.
	type Fleet struct{ db.Model }

	return &gormigrate.Migration{
		ID: "2026082813000007",
		Migrate: func(tx *gorm.DB) error {
			if tx.Migrator().HasTable(&Fleet{}) {
				return tx.Migrator().DropTable(&Fleet{})
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			return nil
		},
	}
}

// migrationAddRepositoryOwnership adds the nullable repository_id import ownership
// mark. It is NULL for gateways created directly through the REST API or hsctl
// apply, and is set server-side by the control-plane declarative-import reconciler
// for gateways it imports from a Repository's .hypershell directory, scoping prune
// to the owning Repository. Raw idempotent ALTER TABLE is used instead of
// AutoMigrate because AutoMigrate's add-column introspection breaks under the test
// session's lib/pq driver with PreferSimpleProtocol (see migrationAddProvisioningFields).
func migrationAddRepositoryOwnership() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "2026101600000001",
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec("ALTER TABLE gateways ADD COLUMN IF NOT EXISTS repository_id TEXT").Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec("ALTER TABLE gateways DROP COLUMN IF EXISTS repository_id").Error
		},
	}
}

// migrationAddPlacement adds the nullable placement column that persists a
// late-bound gateway's capability-label placement selector (JSON-encoded
// GatewayPlacementIntent). It is NULL for a gateway pinned with an explicit
// cluster_id. Raw idempotent ALTER TABLE is used instead of AutoMigrate because
// AutoMigrate's add-column introspection breaks under the test session's lib/pq
// driver with PreferSimpleProtocol (see migrationAddProvisioningFields).
func migrationAddPlacement() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "2026101900000001",
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec("ALTER TABLE gateways ADD COLUMN IF NOT EXISTS placement JSONB").Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec("ALTER TABLE gateways DROP COLUMN IF EXISTS placement").Error
		},
	}
}

func migrationAddGatewayVersion() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "2026082712000001",
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec("ALTER TABLE gateways ADD COLUMN IF NOT EXISTS gateway_version TEXT").Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec("ALTER TABLE gateways DROP COLUMN IF EXISTS gateway_version").Error
		},
	}
}

func migrationAddGenerationTracking() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "2026081912000006",
		Migrate: func(tx *gorm.DB) error {
			// Backfill existing rows to generation=1/observed_generation=1 (converged)
			// so the convergence gate does not re-provision already-running gateways
			// when this migration first runs.
			if err := tx.Exec("ALTER TABLE gateways ADD COLUMN IF NOT EXISTS generation BIGINT NOT NULL DEFAULT 1").Error; err != nil {
				return err
			}
			return tx.Exec("ALTER TABLE gateways ADD COLUMN IF NOT EXISTS observed_generation BIGINT NOT NULL DEFAULT 1").Error
		},
		Rollback: func(tx *gorm.DB) error {
			if err := tx.Exec("ALTER TABLE gateways DROP COLUMN IF EXISTS generation").Error; err != nil {
				return err
			}
			return tx.Exec("ALTER TABLE gateways DROP COLUMN IF EXISTS observed_generation").Error
		},
	}
}

// migrationDropDatabaseId removes the Gateway.database_id placement column.
// Gateway databases are provisioned by the control plane from admin
// credentials mounted into the controller pod, so the API server no longer
// tracks a database per gateway.
func migrationDropDatabaseId() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "2026091600000002",
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec("ALTER TABLE gateways DROP COLUMN IF EXISTS database_id").Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec("ALTER TABLE gateways ADD COLUMN IF NOT EXISTS database_id TEXT NOT NULL DEFAULT ''").Error
		},
	}
}

// migrationDropReleaseFK drops release_id and observed_release_id from gateways
// after GatewayRelease is removed from the API.
func migrationDropReleaseFK() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "2026100600000001",
		Migrate: func(tx *gorm.DB) error {
			if err := tx.Exec("ALTER TABLE gateways DROP COLUMN IF EXISTS release_id").Error; err != nil {
				return err
			}
			return tx.Exec("ALTER TABLE gateways DROP COLUMN IF EXISTS observed_release_id").Error
		},
		Rollback: func(tx *gorm.DB) error {
			if err := tx.Exec("ALTER TABLE gateways ADD COLUMN IF NOT EXISTS release_id TEXT NOT NULL DEFAULT ''").Error; err != nil {
				return err
			}
			return tx.Exec("ALTER TABLE gateways ADD COLUMN IF NOT EXISTS observed_release_id TEXT").Error
		},
	}
}

// migrationDropGatewayReleasesTable drops the gateway_releases table after the
// GatewayRelease API kind is removed.
func migrationDropGatewayReleasesTable() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "2026100600000002",
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec("DROP TABLE IF EXISTS gateway_releases").Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`
				CREATE TABLE IF NOT EXISTS gateway_releases (
					id TEXT PRIMARY KEY,
					created_at TIMESTAMP WITH TIME ZONE,
					updated_at TIMESTAMP WITH TIME ZONE,
					deleted_at TIMESTAMP WITH TIME ZONE,
					name TEXT,
					image TEXT,
					status TEXT,
					rollout_strategy TEXT,
					canary_percent INTEGER,
					canary_duration TEXT,
					traceparent TEXT,
					tracestate TEXT
				)
			`).Error
		},
	}
}

// migrationDropGatewayNetworksTable drops the gateway_networks table after the
// GatewayNetwork API kind is removed.
func migrationDropGatewayNetworksTable() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "2026100600000003",
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec("DROP TABLE IF EXISTS gateway_networks").Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`
				CREATE TABLE IF NOT EXISTS gateway_networks (
					id TEXT PRIMARY KEY,
					created_at TIMESTAMP WITH TIME ZONE,
					updated_at TIMESTAMP WITH TIME ZONE,
					deleted_at TIMESTAMP WITH TIME ZONE,
					name TEXT,
					topology TEXT,
					tunnel_mode TEXT,
					hub_gateway_id TEXT,
					status TEXT,
					traceparent TEXT,
					tracestate TEXT
				)
			`).Error
		},
	}
}

// migrationAddConvergenceTracking creates the durable change-convergence state
// the API server owns for SLO 2 (service-level-indicators.spec.md, OBS-SLI-01
// through OBS-SLI-04). Four tables are created with raw idempotent DDL (never
// AutoMigrate, per the file header): none of them is an api.Meta Kind model, so
// they are plain internal bookkeeping tables, not plugin resources.
//
//   - gateway_convergence     one row per gateway: the single open convergence
//     window (change type, judged generation, clock
//     start) plus the create timestamp the provision
//     duration is computed from. Restarts read it back
//     unchanged (OBS-SLI-01.04).
//   - gateway_status_transitions
//     an append log of every persisted phase and
//     provisioning-condition transition timestamp, so
//     the window and provision duration are computable
//     from persisted data rather than process memory
//     (OBS-SLI-01).
//   - gateway_convergence_counters
//     durable per-(change_type, outcome) totals backing
//     hypershell_change_convergence_total so the exported
//     value is monotonic across restarts (OBS-SLI-03).
//   - gateway_provisions      one row per gateway recording the first
//     create-to-Running duration and the first terminal
//     provision outcome, backing the provision metrics
//     the API server now owns (OBS-SLI-04).
func migrationAddConvergenceTracking() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "2026112300000001",
		Migrate: func(tx *gorm.DB) error {
			stmts := []string{
				`CREATE TABLE IF NOT EXISTS gateway_convergence (
					gateway_id TEXT PRIMARY KEY,
					gateway_created_at TIMESTAMPTZ NOT NULL,
					open_change_type TEXT,
					open_generation BIGINT NOT NULL DEFAULT 0,
					clock_start TIMESTAMPTZ,
					last_phase TEXT NOT NULL DEFAULT '',
					resolved_outcome TEXT,
					resolved_at TIMESTAMPTZ,
					deletion_requested_at TIMESTAMPTZ,
					created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
					updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
				)`,
				`CREATE INDEX IF NOT EXISTS idx_gateway_convergence_open
					ON gateway_convergence (clock_start)
					WHERE open_change_type IS NOT NULL`,
				`CREATE TABLE IF NOT EXISTS gateway_status_transitions (
					id BIGSERIAL PRIMARY KEY,
					gateway_id TEXT NOT NULL,
					generation BIGINT NOT NULL DEFAULT 0,
					transition_type TEXT NOT NULL,
					name TEXT NOT NULL,
					value TEXT NOT NULL,
					transitioned_at TIMESTAMPTZ NOT NULL
				)`,
				`CREATE INDEX IF NOT EXISTS idx_gateway_status_transitions_gateway
					ON gateway_status_transitions (gateway_id)`,
				`CREATE TABLE IF NOT EXISTS gateway_convergence_counters (
					change_type TEXT NOT NULL,
					outcome TEXT NOT NULL,
					total BIGINT NOT NULL DEFAULT 0,
					PRIMARY KEY (change_type, outcome)
				)`,
				`CREATE TABLE IF NOT EXISTS gateway_provisions (
					gateway_id TEXT PRIMARY KEY,
					duration_seconds DOUBLE PRECISION,
					outcome TEXT,
					recorded_at TIMESTAMPTZ NOT NULL DEFAULT now()
				)`,
			}
			for _, stmt := range stmts {
				if err := tx.Exec(stmt).Error; err != nil {
					return err
				}
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			stmts := []string{
				`DROP TABLE IF EXISTS gateway_provisions`,
				`DROP TABLE IF EXISTS gateway_convergence_counters`,
				`DROP TABLE IF EXISTS gateway_status_transitions`,
				`DROP TABLE IF EXISTS gateway_convergence`,
			}
			for _, stmt := range stmts {
				if err := tx.Exec(stmt).Error; err != nil {
					return err
				}
			}
			return nil
		},
	}
}

// migrationDropManagedDatabasesTable drops the orphaned table of the removed
// ManagedDatabase resource. The rollback recreates a minimal table with the
// framework's Model columns so older binaries can start against the schema.
func migrationDropManagedDatabasesTable() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "2026091600000003",
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec("DROP TABLE IF EXISTS managed_databases").Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`
				CREATE TABLE IF NOT EXISTS managed_databases (
					id TEXT PRIMARY KEY,
					created_at TIMESTAMPTZ,
					updated_at TIMESTAMPTZ,
					deleted_at TIMESTAMPTZ,
					name TEXT,
					connection_secret TEXT,
					status TEXT
				)
			`).Error
		},
	}
}

// migrationReplaceRetiredPlacementLabels rewrites stored placement selectors that
// still hold a retired capability label (runtime/release-check,
// runtime/release-matrix) to its purpose/ replacement, in both the required and
// the excluded selector (PLAT-CLUS-06.06). Selector validation now rejects the
// retired names, so a stored selector left unchanged could never match a cluster
// again and would fail validation when late binding re-resolves it. A rewritten
// required label is not re-checked against SEC-RBAC-35: the caller chose it when
// no role was needed, and the gateway is already placed or awaiting placement.
// Irreversible: the old names are rejected, so the rollback is a no-op.
func migrationReplaceRetiredPlacementLabels() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "2026112518000002",
		Migrate: func(tx *gorm.DB) error {
			return dbmigrate.RewriteJSONColumn(tx, "gateways", "placement", "runtime/release-", replaceRetiredPlacementJSON)
		},
		Rollback: func(tx *gorm.DB) error { return nil },
	}
}

// replaceRetiredPlacementJSON rewrites the selector and excludedSelector arrays
// of one stored placement and keeps every other key as it is.
func replaceRetiredPlacementJSON(raw []byte) ([]byte, bool, error) {
	var placement map[string]json.RawMessage
	if err := json.Unmarshal(raw, &placement); err != nil {
		return nil, false, fmt.Errorf("decode placement: %w", err)
	}
	changed := false
	for _, key := range []string{"selector", "excludedSelector"} {
		value, ok := placement[key]
		if !ok {
			continue
		}
		var labels []string
		if err := json.Unmarshal(value, &labels); err != nil {
			return nil, false, fmt.Errorf("decode placement %s: %w", key, err)
		}
		replaced, keyChanged := managedClusters.ReplaceRetiredCapabilityLabels(labels)
		if !keyChanged {
			continue
		}
		encoded, err := json.Marshal(replaced)
		if err != nil {
			return nil, false, fmt.Errorf("encode placement %s: %w", key, err)
		}
		placement[key] = encoded
		changed = true
	}
	if !changed {
		return raw, false, nil
	}
	out, err := json.Marshal(placement)
	if err != nil {
		return nil, false, fmt.Errorf("encode placement: %w", err)
	}
	return out, true, nil
}

// migrationAddTargetAndDeployedBundle adds the platform-admin desired
// target_bundle (PLAT-DATA-33) and the control-plane-observed deployed_bundle
// (PLAT-DATA-34). Both are nullable; existing gateways start with neither.
func migrationAddTargetAndDeployedBundle() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "2026112712000001",
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec(`ALTER TABLE gateways
  ADD COLUMN IF NOT EXISTS target_bundle TEXT,
  ADD COLUMN IF NOT EXISTS deployed_bundle TEXT`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`ALTER TABLE gateways
  DROP COLUMN IF EXISTS target_bundle,
  DROP COLUMN IF EXISTS deployed_bundle`).Error
		},
	}
}
