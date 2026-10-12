package repositories

import (
	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api/presenters"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/util"
)

func ConvertRepository(r openapi.Repository) *Repository {
	c := &Repository{
		Meta: api.Meta{
			ID: util.NilToEmptyString(r.Id),
		},
		Name:           r.Name,
		Url:            r.Url,
		Provider:       r.Provider,
		SecretSourceId: r.SecretSourceId,
		DefaultBranch:  r.DefaultBranch,
		ImportPath:     r.ImportPath,
	}
	if r.CreatedAt != nil {
		c.CreatedAt = *r.CreatedAt
		c.UpdatedAt = *r.UpdatedAt
	}
	return c
}

func PresentRepository(r *Repository) openapi.Repository {
	reference := presenters.PresentReference(r.ID, r)
	return openapi.Repository{
		Id:             reference.Id,
		Kind:           reference.Kind,
		Href:           reference.Href,
		CreatedAt:      openapi.PtrTime(r.CreatedAt),
		UpdatedAt:      openapi.PtrTime(r.UpdatedAt),
		Name:           r.Name,
		Url:            r.Url,
		Provider:       r.Provider,
		SecretSourceId: r.SecretSourceId,
		DefaultBranch:  r.DefaultBranch,
		ImportPath:     r.ImportPath,
		Status:         r.Status,
	}
}
