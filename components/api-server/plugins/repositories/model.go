package repositories

import (
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"gorm.io/gorm"
)

type Repository struct {
	api.Meta
	Name           string  `json:"name"`
	Url            string  `json:"url"`
	Provider       string  `json:"provider"`
	SecretSourceId string  `json:"secret_source_id"`
	DefaultBranch  *string `json:"default_branch"`
	ImportPath     *string `json:"import_path"`
	Status         *string `json:"status"`
}

type RepositoryList []*Repository

type RepositoryPatchRequest struct {
	SecretSourceId *string `json:"secret_source_id"`
	DefaultBranch  *string `json:"default_branch"`
	ImportPath     *string `json:"import_path"`
}

func (d *Repository) BeforeCreate(tx *gorm.DB) error {
	if d.ID == "" {
		d.ID = api.NewID()
	}
	return nil
}
