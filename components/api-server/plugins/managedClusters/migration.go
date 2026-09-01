package managedClusters

import (
	"gorm.io/gorm"

	"github.com/go-gormigrate/gormigrate/v2"
	"github.com/openshift-online/rh-trex-ai/pkg/db"
)

func migration() *gormigrate.Migration {
	type ManagedCluster struct {
		db.Model
		Name             string
		FleetId          string
		Provider         string
		Region           *string
		KubeconfigSecret string
		Status           *string
		ApiServerUrl     *string
	}

	return &gormigrate.Migration{
		ID: "2026080312543137",
		Migrate: func(tx *gorm.DB) error {
			return tx.AutoMigrate(&ManagedCluster{})
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Migrator().DropTable(&ManagedCluster{})
		},
	}
}

func migrationAddProfileAndDatabaseID() *gormigrate.Migration {
	type ManagedCluster struct {
		db.Model
		ProfileId  *string
		DatabaseId *string
	}

	return &gormigrate.Migration{
		ID: "2026082800000003",
		Migrate: func(tx *gorm.DB) error {
			return tx.AutoMigrate(&ManagedCluster{})
		},
		Rollback: func(tx *gorm.DB) error {
			for _, col := range []string{"profile_id", "database_id"} {
				if err := tx.Migrator().DropColumn(&ManagedCluster{}, col); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func migrationDropFleetId() *gormigrate.Migration {
	type ManagedCluster struct{ db.Model }

	return &gormigrate.Migration{
		ID: "2026082813000004",
		Migrate: func(tx *gorm.DB) error {
			if tx.Migrator().HasColumn(&ManagedCluster{}, "fleet_id") {
				return tx.Migrator().DropColumn(&ManagedCluster{}, "fleet_id")
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			return nil
		},
	}
}
