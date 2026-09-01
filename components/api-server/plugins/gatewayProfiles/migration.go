package gatewayProfiles

import (
	"gorm.io/gorm"

	"github.com/go-gormigrate/gormigrate/v2"
	"github.com/openshift-online/rh-trex-ai/pkg/db"
)

func migration() *gormigrate.Migration {
	type GatewayProfile struct {
		db.Model
		Name                          string
		Description                   *string
		CpuRequestTotal               *string
		CpuLimitTotal                 *string
		MemoryRequestTotal            *string
		MemoryLimitTotal              *string
		EphemeralStorageTotal         *string
		PodCount                      *int32
		PvcCount                      *int32
		ContainerCpuRequestDefault    *string
		ContainerCpuLimitMax          *string
		ContainerMemoryRequestDefault *string
		ContainerMemoryLimitMax       *string
	}

	return &gormigrate.Migration{
		ID: "2026082800000001",
		Migrate: func(tx *gorm.DB) error {
			return tx.AutoMigrate(&GatewayProfile{})
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Migrator().DropTable(&GatewayProfile{})
		},
	}
}
