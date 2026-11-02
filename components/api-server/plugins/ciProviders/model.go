package ciProviders

import (
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"gorm.io/gorm"
)

type CIProvider struct {
	api.Meta
	// Name is the queryable find-by-name key (SDLC-CONF-13). hsctl apply resolves
	// an existing record with search=name = '<name>', so every ext Kind carries a
	// name column. It defaults to Forge when a client does not supply one. The
	// uniqueIndex enforces the domain rule "one provider per forge": without it two
	// providers on the same forge would share the default name and find-by-name
	// would be ambiguous. The index name matches the CREATE UNIQUE INDEX in
	// migrationAddName, which carries the constraint onto upgraded hubs where the
	// original CreateTable already ran.
	Name           string  `json:"name" gorm:"uniqueIndex"`
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
	// Guarantee a find-by-name key on every row regardless of entry path (REST or
	// gRPC). When a client does not supply a name, default to the forge. This
	// mirrors the migration backfill for pre-existing rows (SDLC-CONF-13).
	if d.Name == "" {
		d.Name = d.Forge
	}
	return nil
}
