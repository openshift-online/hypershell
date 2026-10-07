package sandboxTemplates

import (
	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api/presenters"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/util"
)

func ConvertSandboxTemplate(st openapi.SandboxTemplate) *SandboxTemplate {
	c := &SandboxTemplate{
		Meta: api.Meta{
			ID: util.NilToEmptyString(st.Id),
		},
		Name:       st.Name,
		Image:      st.Image,
		NamePrefix: st.NamePrefix,
		Policy:     st.Policy,
	}
	if st.CreatedAt != nil {
		c.CreatedAt = *st.CreatedAt
		c.UpdatedAt = *st.UpdatedAt
	}
	return c
}

func PresentSandboxTemplate(st *SandboxTemplate) openapi.SandboxTemplate {
	reference := presenters.PresentReference(st.ID, st)
	return openapi.SandboxTemplate{
		Id:         reference.Id,
		Kind:       reference.Kind,
		Href:       reference.Href,
		CreatedAt:  openapi.PtrTime(st.CreatedAt),
		UpdatedAt:  openapi.PtrTime(st.UpdatedAt),
		Name:       st.Name,
		Image:      st.Image,
		NamePrefix: st.NamePrefix,
		Policy:     st.Policy,
		Status:     st.Status,
	}
}
