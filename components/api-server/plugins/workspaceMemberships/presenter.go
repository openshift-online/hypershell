package workspaceMemberships

import (
	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api/presenters"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/util"
)

func ConvertWorkspaceMembership(wm openapi.WorkspaceMembership) *WorkspaceMembership {
	c := &WorkspaceMembership{
		Meta: api.Meta{
			ID: util.NilToEmptyString(wm.Id),
		},
		WorkspaceId: wm.WorkspaceId,
		Subject:     wm.Subject,
		Role:        wm.Role,
	}
	if wm.CreatedAt != nil {
		c.CreatedAt = *wm.CreatedAt
		c.UpdatedAt = *wm.UpdatedAt
	}
	return c
}

func PresentWorkspaceMembership(wm *WorkspaceMembership) openapi.WorkspaceMembership {
	reference := presenters.PresentReference(wm.ID, wm)
	return openapi.WorkspaceMembership{
		Id:          reference.Id,
		Kind:        reference.Kind,
		Href:        reference.Href,
		CreatedAt:   openapi.PtrTime(wm.CreatedAt),
		UpdatedAt:   openapi.PtrTime(wm.UpdatedAt),
		WorkspaceId: wm.WorkspaceId,
		Subject:     wm.Subject,
		Role:        wm.Role,
	}
}
