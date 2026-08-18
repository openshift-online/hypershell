package roles

import (
	"github.com/openshift-online/rh-trex-ai/pkg/api"
	"gorm.io/gorm"
)

type Role struct {
	api.Meta
	Name        string  `json:"name" gorm:"uniqueIndex"`
	DisplayName *string `json:"display_name"`
	Description *string `json:"description"`
	Permissions *string `json:"permissions" gorm:"type:jsonb"`
	BuiltIn     bool    `json:"built_in"`
}

type RoleList []*Role
type RoleIndex map[string]*Role

func (l RoleList) Index() RoleIndex {
	index := RoleIndex{}
	for _, o := range l {
		index[o.ID] = o
	}
	return index
}

func (d *Role) BeforeCreate(tx *gorm.DB) error {
	if d.ID == "" {
		d.ID = api.NewID()
	}
	return nil
}

const (
	RolePlatformAdmin  = "platform:admin"
	RoleGatewayCreator = "gateway:creator"
	RoleGatewayOwner   = "gateway:owner"
	RoleGatewayViewer  = "gateway:viewer"
)

var JWTSyncedRoles = map[string]bool{
	RolePlatformAdmin:  true,
	RoleGatewayCreator: true,
}
