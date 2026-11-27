package defaultWorkspaceBootstrap

import (
	"time"

	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"gorm.io/gorm"
)

const (
	SubjectTypeUser           = "user"
	SubjectTypeServiceAccount = "service_account"
	OutcomeSucceeded          = "succeeded"
	OutcomeFailed             = "failed"
)

type Attempt struct {
	api.Meta
	GatewayID     string    `gorm:"not null;index"`
	Subject       string    `gorm:"not null;index"`
	SubjectType   string    `gorm:"not null"`
	Role          string    `gorm:"not null"`
	Outcome       string    `gorm:"not null;index"`
	CorrelationID string    `gorm:"not null"`
	LastError     *string   `gorm:"last_error"`
	AttemptedAt   time.Time `gorm:"not null"`
}

func (Attempt) TableName() string {
	return "default_workspace_bootstrap_attempts"
}

func (a *Attempt) BeforeCreate(_ *gorm.DB) error {
	if a.ID == "" {
		a.ID = api.NewID()
	}
	return nil
}
