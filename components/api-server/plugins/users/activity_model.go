package users

import "time"

// UserDailyActivity records that a registered user made at least one
// authenticated API request on a UTC calendar day.
type UserDailyActivity struct {
	UserID       string    `gorm:"primaryKey"`
	ActivityDate time.Time `gorm:"primaryKey;type:date"`
}

type UserDailyActivityList []*UserDailyActivity
