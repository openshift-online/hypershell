package secretSources

import (
	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api/presenters"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/util"
)

func ConvertSecretSource(ss openapi.SecretSource) *SecretSource {
	c := &SecretSource{
		Meta: api.Meta{
			ID: util.NilToEmptyString(ss.Id),
		},
		Name:           ss.Name,
		AgentRuntimeId: ss.AgentRuntimeId,
		Purpose:        ss.Purpose,
		Backend:        ss.Backend,
		Path:           ss.Path,
		KeyMappings:    ss.KeyMappings,
	}
	if ss.CreatedAt != nil {
		c.CreatedAt = *ss.CreatedAt
		c.UpdatedAt = *ss.UpdatedAt
	}
	return c
}

func PresentSecretSource(ss *SecretSource) openapi.SecretSource {
	reference := presenters.PresentReference(ss.ID, ss)
	return openapi.SecretSource{
		Id:             reference.Id,
		Kind:           reference.Kind,
		Href:           reference.Href,
		CreatedAt:      openapi.PtrTime(ss.CreatedAt),
		UpdatedAt:      openapi.PtrTime(ss.UpdatedAt),
		Name:           ss.Name,
		AgentRuntimeId: ss.AgentRuntimeId,
		Purpose:        ss.Purpose,
		Backend:        ss.Backend,
		Path:           ss.Path,
		KeyMappings:    ss.KeyMappings,
	}
}
