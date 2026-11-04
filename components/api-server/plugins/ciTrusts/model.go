package ciTrusts

import (
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"gorm.io/gorm"
)

type CITrust struct {
	api.Meta
	// Name is the queryable find-by-name key (SDLC-CONF-13). It defaults to a
	// stable identity derived from ci_provider_id/gateway_id/role when a client
	// does not supply one. A PARTIAL UNIQUE index (live rows only) makes find-by-name
	// resolve exactly one live row (an ambiguous name would make hsctl apply resolve
	// the wrong record) while leaving a soft-deleted row's lingering name free, so a
	// reconcile can re-apply a deleted CITrust without a 409. The index is created by
	// migrationAddName, NOT by a model-level uniqueIndex tag: a full tag would make
	// CreateTable build a full index covering deleted rows (SDLC-CONF-13, FINDING 1).
	Name string `json:"name"`
	// The gorm column tag pins the snake_case name; GORM's default naming would
	// mangle the CI initialism to "c_iprovider_id", which the migration never
	// created (SDLC-CONF-08).
	CIProviderID        string  `json:"ci_provider_id" gorm:"column:ci_provider_id;index"`
	GatewayID           string  `json:"gateway_id" gorm:"column:gateway_id;index"`
	SubjectRepository   *string `json:"subject_repository"`
	SubjectProjectPath  *string `json:"subject_project_path"`
	SubjectRef          *string `json:"subject_ref"`
	SubjectEnvironment  *string `json:"subject_environment"`
	SubjectRefProtected *bool   `json:"subject_ref_protected"`
	Role                string  `json:"role"`
	// RepositoryID is the ownership mark set by the control-plane declarative
	// import reconciler (SEC-RBAC-25, PLAT-DATA-25). It is NULL for a CITrust
	// created directly over REST or hsctl apply, and carries the owning
	// Repository id for an imported trust so prune is scoped per-Repository. It is
	// set server-side from trusted metadata, never from a client request body.
	RepositoryID           *string `json:"repository_id" gorm:"column:repository_id;index"`
	StatusExchangeEndpoint *string `json:"status_exchange_endpoint"`
	StatusWorkspace        *string `json:"status_workspace"`
	StatusWorkflowHint     *string `json:"status_workflow_hint"`
}

// TableName pins the table to the clean plural "ci_trusts" that the OpenAPI
// path and the ci_providers delete-guard DAO use, rather than GORM's mangled
// default "c_itrusts" (SDLC-CONF-08).
func (CITrust) TableName() string { return "ci_trusts" }

type CITrustList []*CITrust

type CITrustPatchRequest struct {
	SubjectRef          *string `json:"subject_ref"`
	SubjectEnvironment  *string `json:"subject_environment"`
	SubjectRefProtected *bool   `json:"subject_ref_protected"`
	Role                *string `json:"role"`
}

func (d *CITrust) BeforeCreate(tx *gorm.DB) error {
	if d.ID == "" {
		d.ID = api.NewID()
	}
	// Guarantee a find-by-name key on every row regardless of entry path (REST or
	// gRPC). When a client does not supply a name, derive a stable identity from
	// the composite identity columns. This mirrors the migration backfill for
	// pre-existing rows (SDLC-CONF-13).
	if d.Name == "" {
		d.Name = DeriveName(d.CIProviderID, d.GatewayID, d.Role)
	}
	return nil
}

// DeriveName builds the default find-by-name key for a CITrust from its composite
// identity (ci_provider_id/gateway_id/role). The migration backfill derives the
// same shape for pre-existing rows so new and old rows resolve identically.
func DeriveName(ciProviderID, gatewayID, role string) string {
	return ciProviderID + "/" + gatewayID + "/" + role
}
