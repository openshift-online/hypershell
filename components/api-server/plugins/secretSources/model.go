package secretSources

import (
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"gorm.io/gorm"
)

type SecretSource struct {
	api.Meta
	Name           string  `json:"name"`
	AgentRuntimeId string  `json:"agent_runtime_id"`
	Purpose        string  `json:"purpose"`
	Backend        string  `json:"backend"`
	Path           string  `json:"path"`
	KeyMappings    *string `json:"key_mappings" gorm:"type:jsonb"`
	// RepositoryId is the ownership mark set by the control-plane declarative
	// import reconciler when this resource originates from a Repository's
	// .hypershell import. It is nil for resources created directly through the
	// REST API or hsctl apply, which are never pruned by the import. It is
	// read-only in the client contract (set server-side, never by a client).
	RepositoryId *string `json:"repository_id"`
}

type SecretSourceList []*SecretSource

type SecretSourcePatchRequest struct {
	Name        *string `json:"name"`
	Purpose     *string `json:"purpose"`
	Backend     *string `json:"backend"`
	Path        *string `json:"path"`
	KeyMappings *string `json:"key_mappings"`
}

// SecretSourceMaterialUploadRequest is the body of the write-only material-upload
// operation (PLAT-DATA-27, AGT-SEC-14). It carries ONLY the durable material: it
// has no path or other reference field, so an upload can never re-point a
// SecretSource at another store location. The value is write-only - it is written
// through the write-only secret store and is never persisted raw in the database
// nor echoed in the response.
type SecretSourceMaterialUploadRequest struct {
	Value *string `json:"value"`
}

func (d *SecretSource) BeforeCreate(tx *gorm.DB) error {
	if d.ID == "" {
		d.ID = api.NewID()
	}
	return nil
}
