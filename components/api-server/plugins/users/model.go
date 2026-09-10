package users

import (
	"time"

	"github.com/openshift-online/rh-trex-ai/pkg/api"
	"gorm.io/gorm"
)

type User struct {
	api.Meta
	Username    string     `json:"username" gorm:"uniqueIndex"`
	Email       *string    `json:"email"`
	Name        *string    `json:"name"`
	LastLoginAt *time.Time `json:"last_login_at"`
}

type UserLoginDay struct {
	UserID    string    `gorm:"primaryKey"`
	LoginDate time.Time `gorm:"primaryKey;type:date"`
}

type UserList []*User
type UserIndex map[string]*User

func (l UserList) Index() UserIndex {
	index := UserIndex{}
	for _, o := range l {
		index[o.ID] = o
	}
	return index
}

func (d *User) BeforeCreate(tx *gorm.DB) error {
	if d.ID == "" {
		d.ID = api.NewID()
	}
	return nil
}
