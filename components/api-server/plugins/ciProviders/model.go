package ciProviders

import (
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"gorm.io/gorm"
)

type CIProvider struct {
	api.Meta
	Forge          string  `json:"forge"`
	Issuer         string  `json:"issuer" gorm:"uniqueIndex"`
	JwksUrl        string  `json:"jwks_url"`
	Audience       string  `json:"audience"`
	StatusIdpAlias *string `json:"status_idp_alias"`
}

// TableName pins the table to the clean plural "ci_providers" that the OpenAPI
// path and the ci_trusts cross-reference DAO use. Without it GORM's default
// naming mangles "CIProvider" to "c_iproviders", so ci_trust creation could
// never resolve ci_provider_id (SDLC-CONF-08).
func (CIProvider) TableName() string { return "ci_providers" }

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
