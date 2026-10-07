package providerBindings

import (
	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api/presenters"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/util"
)

func ConvertProviderBinding(pb openapi.ProviderBinding) *ProviderBinding {
	c := &ProviderBinding{
		Meta: api.Meta{
			ID: util.NilToEmptyString(pb.Id),
		},
		Name:            pb.Name,
		WorkspaceId:     pb.WorkspaceId,
		ProviderSpecId:  pb.ProviderSpecId,
		RefreshStrategy: pb.RefreshStrategy,
	}
	if pb.SecretSourceId != nil {
		c.SecretSourceId = *pb.SecretSourceId
	}
	if pb.CreatedAt != nil {
		c.CreatedAt = *pb.CreatedAt
		c.UpdatedAt = *pb.UpdatedAt
	}
	return c
}

func PresentProviderBinding(pb *ProviderBinding) openapi.ProviderBinding {
	reference := presenters.PresentReference(pb.ID, pb)
	result := openapi.ProviderBinding{
		Id:              reference.Id,
		Kind:            reference.Kind,
		Href:            reference.Href,
		CreatedAt:       openapi.PtrTime(pb.CreatedAt),
		UpdatedAt:       openapi.PtrTime(pb.UpdatedAt),
		Name:            pb.Name,
		WorkspaceId:     pb.WorkspaceId,
		ProviderSpecId:  pb.ProviderSpecId,
		RefreshStrategy: pb.RefreshStrategy,
		Status:          pb.Status,
	}
	if pb.SecretSourceId != "" {
		result.SecretSourceId = &pb.SecretSourceId
	}
	return result
}
