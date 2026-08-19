package gateways

import (
	"gorm.io/gorm"

	"github.com/go-gormigrate/gormigrate/v2"
	"github.com/openshift-online/rh-trex-ai/pkg/db"
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
			return tx.AutoMigrate(&Gateway{})
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

func migrationAddObservedReleaseId() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "2026091412000001",
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec("ALTER TABLE gateways ADD COLUMN IF NOT EXISTS observed_release_id TEXT").Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec("ALTER TABLE gateways DROP COLUMN IF EXISTS observed_release_id").Error
		},
	}
}

func migrationAddGenerationTracking() *gormigrate.Migration {
	type Gateway struct {
		db.Model
		Generation         int64 `gorm:"not null;default:1"`
		ObservedGeneration int64 `gorm:"not null;default:1"`
	}

	return &gormigrate.Migration{
		ID: "2026081912000006",
		Migrate: func(tx *gorm.DB) error {
			return tx.AutoMigrate(&Gateway{})
		},
		Rollback: func(tx *gorm.DB) error {
			for _, col := range []string{"generation", "observed_generation"} {
				if err := tx.Migrator().DropColumn(&Gateway{}, col); err != nil {
					return err
				}
			}
			return nil
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
