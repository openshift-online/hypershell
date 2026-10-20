package workspaceMemberships

import (
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"gorm.io/gorm"
)

// Role values for a WorkspaceMembership. The gateway restricts provider credential
// rotation to workspace administrators, so a workspace whose ProviderBinding carries
// a non-empty refresh_strategy requires at least one admin membership
// (data-model.spec.md "WorkspaceMembership").
const (
	RoleAdmin  = "admin"
	RoleMember = "member"
)

// WorkspaceMembership binds an OIDC subject to a role within an AgentWorkspace. The
// subject is the UUID from the subject claim of the OIDC identity the agent uses to
// connect to the gateway (data-model.spec.md "WorkspaceMembership").
type WorkspaceMembership struct {
	api.Meta
	WorkspaceId string `json:"workspace_id"`
	Subject     string `json:"subject"`
	Role        string `json:"role"`
}

type WorkspaceMembershipList []*WorkspaceMembership

type WorkspaceMembershipPatchRequest struct {
	Role *string `json:"role"`
}

func (d *WorkspaceMembership) BeforeCreate(tx *gorm.DB) error {
	if d.ID == "" {
		d.ID = api.NewID()
	}
	return nil
}
