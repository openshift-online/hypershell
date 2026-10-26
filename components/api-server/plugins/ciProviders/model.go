package ciProviders

import (
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"gorm.io/gorm"
)

type CIProvider struct {
	api.Meta
	Forge          string  `json:"forge"`
	Issuer         string  `json:"issuer"`
	JwksUrl        string  `json:"jwks_url"`
	Audience       string  `json:"audience"`
	StatusIdpAlias *string `json:"status_idp_alias"`
}

type CIProviderList []*CIProvider

type CIProviderPatchRequest struct {
	JwksUrl *string `json:"jwks_url"`
}

func (d *CIProvider) BeforeCreate(tx *gorm.DB) error {
	if d.ID == "" {
		d.ID = api.NewID()
	}
	return nil
}
