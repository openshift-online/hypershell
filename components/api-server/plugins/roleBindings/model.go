package roleBindings

import (
	hypershellapi "github.com/openshift-online/hypershell/components/api-server/pkg/api"
	"github.com/openshift-online/rh-trex-ai/pkg/api"
	"gorm.io/gorm"
)

const (
	ScopeGlobal  = "global"
	ScopeGateway = "gateway"
)

type RoleBinding struct {
	api.Meta
	hypershellapi.TraceMeta
	RoleID    string  `json:"role_id" gorm:"index"`
	Scope     string  `json:"scope"`
	UserID    *string `json:"user_id" gorm:"index"`
	GatewayID *string `json:"gateway_id" gorm:"index"`
}

type RoleBindingList []*RoleBinding
type RoleBindingIndex map[string]*RoleBinding

func (l RoleBindingList) Index() RoleBindingIndex {
	index := RoleBindingIndex{}
	for _, o := range l {
		index[o.ID] = o
	}
	return index
}

func (d *RoleBinding) BeforeCreate(tx *gorm.DB) error {
	if d.ID == "" {
		d.ID = api.NewID()
	}
	return nil
}
