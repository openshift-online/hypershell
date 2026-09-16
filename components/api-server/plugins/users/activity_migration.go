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

func activityIndexMigration() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "2026091618000001",
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec(
				"CREATE INDEX IF NOT EXISTS idx_user_daily_activities_activity_date ON user_daily_activities (activity_date)",
			).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec("DROP INDEX IF EXISTS idx_user_daily_activities_activity_date").Error
		},
	}
}
