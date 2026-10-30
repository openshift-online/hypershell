package ciTrusts

import (
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"gorm.io/gorm"
)

type CITrust struct {
	api.Meta
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
	return nil
}
