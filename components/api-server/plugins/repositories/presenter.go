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
		SecretSourceId: util.NilToEmptyString(r.SecretSourceId),
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
	// secret_source_id is optional: present it only when set so a public repository
	// (no credential) serializes without the field.
	var secretSourceID *string
	if r.SecretSourceId != "" {
		secretSourceID = openapi.PtrString(r.SecretSourceId)
	}
	return openapi.Repository{
		Id:             reference.Id,
		Kind:           reference.Kind,
		Href:           reference.Href,
		CreatedAt:      openapi.PtrTime(r.CreatedAt),
		UpdatedAt:      openapi.PtrTime(r.UpdatedAt),
		Name:           r.Name,
		Url:            r.Url,
		Provider:       r.Provider,
		SecretSourceId: secretSourceID,
		DefaultBranch:  r.DefaultBranch,
		ImportPath:     r.ImportPath,
		Status:         r.Status,
	}
}
