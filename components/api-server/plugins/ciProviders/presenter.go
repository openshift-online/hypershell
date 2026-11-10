package ciProviders

import (
	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api/presenters"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/util"
)

func ConvertCIProvider(r openapi.CIProvider) *CIProvider {
	c := &CIProvider{
		Meta: api.Meta{
			ID: util.NilToEmptyString(r.Id),
		},
		Name:     util.NilToEmptyString(r.Name),
		Forge:    r.Forge,
		Issuer:   r.Issuer,
		JwksUrl:  r.JwksUrl,
		Audience: r.Audience,
		IdpAlias: util.NilToEmptyString(r.IdpAlias),
	}
	if r.CreatedAt != nil {
		c.CreatedAt = *r.CreatedAt
		c.UpdatedAt = *r.UpdatedAt
	}
	if r.Status != nil {
		c.StatusIdpAlias = r.Status.IdpAlias
	}
	return c
}

func PresentCIProvider(p *CIProvider) openapi.CIProvider {
	reference := presenters.PresentReference(p.ID, p)
	result := openapi.CIProvider{
		Id:        reference.Id,
		Kind:      reference.Kind,
		Href:      reference.Href,
		CreatedAt: openapi.PtrTime(p.CreatedAt),
		UpdatedAt: openapi.PtrTime(p.UpdatedAt),
		Name:      openapi.PtrString(p.Name),
		Forge:     p.Forge,
		Issuer:    p.Issuer,
		JwksUrl:   p.JwksUrl,
		Audience:  p.Audience,
	}
	if p.IdpAlias != "" {
		result.IdpAlias = openapi.PtrString(p.IdpAlias)
	}
	if p.StatusIdpAlias != nil {
		result.Status = &openapi.CIProviderAllOfStatus{
			IdpAlias: p.StatusIdpAlias,
		}
	}
	return result
}
