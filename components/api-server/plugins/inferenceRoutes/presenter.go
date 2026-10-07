package inferenceRoutes

import (
	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api/presenters"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/util"
)

func ConvertInferenceRoute(ir openapi.InferenceRoute) *InferenceRoute {
	c := &InferenceRoute{
		Meta: api.Meta{
			ID: util.NilToEmptyString(ir.Id),
		},
		WorkspaceId:       ir.WorkspaceId,
		ProviderBindingId: ir.ProviderBindingId,
		Model:             ir.Model,
	}
	if ir.CreatedAt != nil {
		c.CreatedAt = *ir.CreatedAt
		c.UpdatedAt = *ir.UpdatedAt
	}
	return c
}

func PresentInferenceRoute(ir *InferenceRoute) openapi.InferenceRoute {
	reference := presenters.PresentReference(ir.ID, ir)
	return openapi.InferenceRoute{
		Id:                reference.Id,
		Kind:              reference.Kind,
		Href:              reference.Href,
		CreatedAt:         openapi.PtrTime(ir.CreatedAt),
		UpdatedAt:         openapi.PtrTime(ir.UpdatedAt),
		WorkspaceId:       ir.WorkspaceId,
		ProviderBindingId: ir.ProviderBindingId,
		Model:             ir.Model,
	}
}
