package providerSpecs

import (
	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api/presenters"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/util"
)

func ConvertProviderSpec(ps openapi.ProviderSpec) *ProviderSpec {
	c := &ProviderSpec{
		Meta: api.Meta{
			ID: util.NilToEmptyString(ps.Id),
		},
		Name:       ps.Name,
		Category:   ps.Category,
		Capability: ps.Capability,
		Profile:    ps.Profile,
	}
	if ps.CreatedAt != nil {
		c.CreatedAt = *ps.CreatedAt
		c.UpdatedAt = *ps.UpdatedAt
	}
	return c
}

func PresentProviderSpec(ps *ProviderSpec) openapi.ProviderSpec {
	reference := presenters.PresentReference(ps.ID, ps)
	return openapi.ProviderSpec{
		Id:         reference.Id,
		Kind:       reference.Kind,
		Href:       reference.Href,
		CreatedAt:  openapi.PtrTime(ps.CreatedAt),
		UpdatedAt:  openapi.PtrTime(ps.UpdatedAt),
		Name:       ps.Name,
		Category:   ps.Category,
		Capability: ps.Capability,
		Profile:    ps.Profile,
		Status:     ps.Status,
	}
}
