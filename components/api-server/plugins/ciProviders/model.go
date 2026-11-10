package ciProviders

import (
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"gorm.io/gorm"
)

type CIProvider struct {
	api.Meta
	// Name is the queryable find-by-name key (SDLC-CONF-13). hsctl apply resolves
	// an existing record with search=name = '<name>', so every ext Kind carries a
	// name column. It defaults to Forge when a client does not supply one. A PARTIAL
	// UNIQUE index (live rows only) enforces the domain rule "one LIVE provider per
	// forge" without blocking delete-then-recreate: it is created by
	// migrationAddName, NOT by a model-level uniqueIndex tag. A full tag would make
	// CreateTable build a full index covering soft-deleted rows, so re-applying a
	// deleted provider would 409 on the lingering row (SDLC-CONF-13, FINDING 1).
	Name  string `json:"name"`
	Forge string `json:"forge"`
	// Issuer is unique per LIVE provider: a distinct forge instance registers a
	// distinct issuer (PLAT-CITR-07). The uniqueness is a PARTIAL UNIQUE index
	// (WHERE deleted_at IS NULL), created by migrationPartialIssuerIndex, NOT a
	// model-level uniqueIndex tag. A full tag would make CreateTable build a full
	// index covering soft-deleted rows, so re-applying a deleted provider that
	// reuses the same issuer would 409 on the lingering row and break reconcile
	// convergence (SDLC-CONF-13, FINDING 1, same root cause as the name index).
	Issuer   string `json:"issuer"`
	JwksUrl  string `json:"jwks_url"`
	Audience string `json:"audience"`
	// IdpAlias is the explicit (spec) Keycloak IdP alias this provider adopts, the
	// single declarative source of truth shared with the platform federation (the
	// terraform ci_forges map key, e.g. "github-actions"). When set, the control
	// plane adopts the IdP of that alias; when unset, it derives the alias from
	// forge and issuer (PLAT-DATA-24, PLAT-CITR-01). Distinct from StatusIdpAlias,
	// which records the alias actually realized.
	IdpAlias       string  `json:"idp_alias"`
	StatusIdpAlias *string `json:"status_idp_alias"`
}

// TableName pins the table to the clean plural "ci_providers" that the OpenAPI
// path and the ci_trusts cross-reference DAO use. Without it GORM's default
// naming mangles "CIProvider" to "c_iproviders", so ci_trust creation could
// never resolve ci_provider_id (SDLC-CONF-08).
func (CIProvider) TableName() string { return "ci_providers" }

type CIProviderList []*CIProvider

type CIProviderPatchRequest struct {
	JwksUrl  *string `json:"jwks_url"`
	IdpAlias *string `json:"idp_alias"`
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
