package roleBindings

import (
	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/rh-trex-ai/pkg/api"
	"github.com/openshift-online/rh-trex-ai/pkg/api/presenters"
	"github.com/openshift-online/rh-trex-ai/pkg/util"
)

func ConvertRoleBinding(rb openapi.RoleBinding) *RoleBinding {
	c := &RoleBinding{
		Meta: api.Meta{
			ID: util.NilToEmptyString(rb.Id),
		},
	}
	c.RoleID = rb.RoleId
	c.Scope = rb.Scope
	c.UserID = rb.UserId
	c.GatewayID = rb.GatewayId

	if rb.CreatedAt != nil {
		c.CreatedAt = *rb.CreatedAt
		c.UpdatedAt = *rb.UpdatedAt
	}

	return c
}

func PresentRoleBinding(rb *RoleBinding) openapi.RoleBinding {
	reference := presenters.PresentReference(rb.ID, rb)
	result := openapi.RoleBinding{
		Id:        reference.Id,
		Kind:      reference.Kind,
		Href:      reference.Href,
		CreatedAt: openapi.PtrTime(rb.CreatedAt),
		UpdatedAt: openapi.PtrTime(rb.UpdatedAt),
		RoleId:    rb.RoleID,
		Scope:     rb.Scope,
		UserId:    rb.UserID,
		GatewayId: rb.GatewayID,
	}
	return result
}
