package users

import (
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"gorm.io/gorm"
)

type User struct {
	api.Meta
	Username string  `json:"username" gorm:"uniqueIndex"`
	Email    *string `json:"email"`
	Name     *string `json:"name"`
	// Subject is the Keycloak OIDC `sub` claim captured at provisioning
	// (security/rbac-enforcement.spec.md, SEC-RBAC-03). It is internal-only:
	// deliberately NOT json-tagged, so it can never be serialized by a generic
	// marshal of this struct, and PresentUser does not reference it, so it never
	// reaches the public User schema (platform/registered-users.spec.md,
	// PLAT-USR-02). Once non-empty it is never overwritten (dao.go,
	// updateChangedFields).
	Subject string `json:"-"`
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
