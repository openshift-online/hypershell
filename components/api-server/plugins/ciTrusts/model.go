package ciTrusts

import (
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"gorm.io/gorm"
)

type CITrust struct {
	api.Meta
	CIProviderID           string  `json:"ci_provider_id"`
	GatewayID              string  `json:"gateway_id"`
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
