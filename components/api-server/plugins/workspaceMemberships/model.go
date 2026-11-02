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
	// Name is the queryable find-by-name key (SDLC-CONF-13). It defaults to a
	// stable identity derived from workspace_id/subject when a client does not
	// supply one.
	Name        string `json:"name"`
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
	// Guarantee a find-by-name key on every row regardless of entry path (REST or
	// gRPC). When a client does not supply a name, derive a stable identity from
	// the composite identity columns, mirroring the migration backfill for
	// pre-existing rows (SDLC-CONF-13).
	if d.Name == "" {
		d.Name = DeriveName(d.WorkspaceId, d.Subject)
	}
	return nil
}

// DeriveName builds the default find-by-name key for a WorkspaceMembership from
// its composite identity (workspace_id/subject). The migration backfill derives
// the same shape for pre-existing rows.
func DeriveName(workspaceID, subject string) string {
	return workspaceID + "/" + subject
}
