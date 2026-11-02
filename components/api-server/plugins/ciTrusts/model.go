package ciTrusts

import (
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"gorm.io/gorm"
)

type CITrust struct {
	api.Meta
	// Name is the queryable find-by-name key (SDLC-CONF-13). It defaults to a
	// stable identity derived from ci_provider_id/gateway_id/role when a client
	// does not supply one. The uniqueIndex makes find-by-name resolve exactly one
	// row (an ambiguous name would make hsctl apply resolve the wrong record); its
	// name matches the CREATE UNIQUE INDEX in migrationAddName, which carries the
	// constraint onto upgraded hubs where the original CreateTable already ran.
	Name string `json:"name" gorm:"uniqueIndex"`
	// The gorm column tag pins the snake_case name; GORM's default naming would
	// mangle the CI initialism to "c_iprovider_id", which the migration never
	// created (SDLC-CONF-08).
	CIProviderID           string  `json:"ci_provider_id" gorm:"column:ci_provider_id;index"`
	GatewayID              string  `json:"gateway_id" gorm:"column:gateway_id;index"`
	SubjectRepository      *string `json:"subject_repository"`
	SubjectProjectPath     *string `json:"subject_project_path"`
	SubjectRef             *string `json:"subject_ref"`
	SubjectEnvironment     *string `json:"subject_environment"`
	SubjectRefProtected    *bool   `json:"subject_ref_protected"`
	Role                   string  `json:"role"`
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
