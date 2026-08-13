package roles

import (
	"encoding/json"

	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/rh-trex-ai/pkg/api"
	"github.com/openshift-online/rh-trex-ai/pkg/api/presenters"
	"github.com/openshift-online/rh-trex-ai/pkg/util"
)

func ConvertRole(role openapi.Role) *Role {
	c := &Role{
		Meta: api.Meta{
			ID: util.NilToEmptyString(role.Id),
		},
	}
	c.Name = role.Name
	c.DisplayName = role.DisplayName
	c.Description = role.Description

	if role.CreatedAt != nil {
		c.CreatedAt = *role.CreatedAt
		c.UpdatedAt = *role.UpdatedAt
	}

	return c
}

func PresentRole(role *Role) openapi.Role {
	reference := presenters.PresentReference(role.ID, role)
	r := openapi.Role{
		Id:          reference.Id,
		Kind:        reference.Kind,
		Href:        reference.Href,
		CreatedAt:   openapi.PtrTime(role.CreatedAt),
		UpdatedAt:   openapi.PtrTime(role.UpdatedAt),
		Name:        role.Name,
		DisplayName: role.DisplayName,
		Description: role.Description,
		BuiltIn:     &role.BuiltIn,
	}

	if role.Permissions != nil {
		var perms map[string]interface{}
		if err := json.Unmarshal([]byte(*role.Permissions), &perms); err == nil {
			r.Permissions = perms
		}
	}

	return r
}
