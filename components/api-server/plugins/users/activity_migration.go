package users

import (
	"time"

	"gorm.io/gorm"

	"github.com/go-gormigrate/gormigrate/v2"
)

func activityMigration() *gormigrate.Migration {
	type UserDailyActivity struct {
		UserID       string    `gorm:"primaryKey"`
		ActivityDate time.Time `gorm:"primaryKey;type:date"`
	}

	return &gormigrate.Migration{
		ID: "2026091116000001",
		Migrate: func(tx *gorm.DB) error {
			return tx.AutoMigrate(&UserDailyActivity{})
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Migrator().DropTable(&UserDailyActivity{})
		},
	}
}
