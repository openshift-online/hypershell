package users

import (
	"time"

	"gorm.io/gorm"

	"github.com/go-gormigrate/gormigrate/v2"
	"github.com/openshift-online/rh-trex-ai/pkg/db"
)

func migration() *gormigrate.Migration {
	type User struct {
		db.Model
		Username string `gorm:"uniqueIndex"`
		Email    *string
		Name     *string
	}

	return &gormigrate.Migration{
		ID: "2026081112000001",
		Migrate: func(tx *gorm.DB) error {
			return tx.AutoMigrate(&User{})
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Migrator().DropTable(&User{})
		},
	}
}

func activityStatsMigration() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "2026090816000001",
		Migrate: func(tx *gorm.DB) error {
			type User struct {
				LastLoginAt *time.Time
			}
			if err := tx.AutoMigrate(&User{}); err != nil {
				return err
			}
			return tx.AutoMigrate(&UserLoginDay{})
		},
		Rollback: func(tx *gorm.DB) error {
			if err := tx.Migrator().DropTable(&UserLoginDay{}); err != nil {
				return err
			}
			return tx.Migrator().DropColumn(&User{}, "last_login_at")
		},
	}
}
