package gateways

import (
	"gorm.io/gorm"

	"github.com/go-gormigrate/gormigrate/v2"
	"github.com/openshift-online/rh-trex-ai/pkg/db"
)

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

func migrationAddProvisioningFields() *gormigrate.Migration {
	type Gateway struct {
		db.Model
		Image          *string
		ServerDnsNames *string `gorm:"type:jsonb"`
		RouteAddress   *string
		Oidc           *string `gorm:"type:jsonb"`
		Route          *string `gorm:"type:jsonb"`
		DatabaseConfig *string `gorm:"type:jsonb"`
	}

	return &gormigrate.Migration{
		ID: "2026080712000001",
		Migrate: func(tx *gorm.DB) error {
			return tx.AutoMigrate(&Gateway{})
		},
		Rollback: func(tx *gorm.DB) error {
			for _, col := range []string{"image", "server_dns_names", "route_address", "oidc", "route", "database_config"} {
				if err := tx.Migrator().DropColumn(&Gateway{}, col); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func migrationAddCredentialDriver() *gormigrate.Migration {
	type Gateway struct {
		db.Model
		CredentialDriver *string `gorm:"type:jsonb"`
	}

	return &gormigrate.Migration{
		ID: "2026081112000005",
		Migrate: func(tx *gorm.DB) error {
			return tx.AutoMigrate(&Gateway{})
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Migrator().DropColumn(&Gateway{}, "credential_driver")
		},
	}
}

func migrationAddSupervisorImage() *gormigrate.Migration {
	type Gateway struct {
		db.Model
		SupervisorImage *string
	}

	return &gormigrate.Migration{
		ID: "2026080712000002",
		Migrate: func(tx *gorm.DB) error {
			return tx.AutoMigrate(&Gateway{})
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Migrator().DropColumn(&Gateway{}, "supervisor_image")
		},
	}
}

func migrationAddConsoleAddress() *gormigrate.Migration {
	type Gateway struct {
		db.Model
		ConsoleAddress *string
	}

	return &gormigrate.Migration{
		ID: "2026081112000006",
		Migrate: func(tx *gorm.DB) error {
			return tx.AutoMigrate(&Gateway{})
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Migrator().DropColumn(&Gateway{}, "console_address")
		},
	}
}

func migrationAddActiveSandboxCount() *gormigrate.Migration {
	type Gateway struct {
		db.Model
		ActiveSandboxCount *int
	}

	return &gormigrate.Migration{
		ID: "2026081712000006",
		Migrate: func(tx *gorm.DB) error {
			return tx.AutoMigrate(&Gateway{})
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Migrator().DropColumn(&Gateway{}, "active_sandbox_count")
		},
	}
}
