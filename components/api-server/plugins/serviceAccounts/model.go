package serviceAccounts

import (
	"time"

	"github.com/openshift-online/rh-trex-ai/pkg/api"
	"gorm.io/gorm"
)

const (
	CredentialTypeClientSecret = "client_secret"

	RoleUser  = "openshell-user"
	RoleAdmin = "openshell-admin"

	StatusProvisioning = "provisioning"
	StatusReady        = "ready"
	StatusExpired      = "expired"
	StatusRevoking     = "revoking"
	StatusRevoked      = "revoked"
	StatusDeleting     = "deleting"
	StatusError        = "error"
	// StatusDegraded marks a previously-ready account whose reconciliation mutation
	// failed part-way. The managed Keycloak client may be disabled or only partially
	// repaired, so the record must not report a clean Ready state; the next sweep
	// re-converges it. Unlike StatusError it never triggers credential removal.
	StatusDegraded = "degraded"
)

// OpenShellGatewayServiceAccount is the durable, non-secret control-plane
// record. Client credentials and access tokens must never be added to it.
type OpenShellGatewayServiceAccount struct {
	api.Meta
	GatewayID          string     `json:"gateway_id" gorm:"not null;index"`
	Name               string     `json:"name" gorm:"not null"`
	Description        *string    `json:"description,omitempty"`
	CredentialType     string     `json:"credential_type" gorm:"not null"`
	Role               string     `json:"role" gorm:"not null"`
	Status             string     `json:"status" gorm:"not null;index"`
	CreatedByUserID    string     `json:"created_by_user_id" gorm:"not null;index"`
	KeycloakClientID   string     `json:"client_id" gorm:"not null;uniqueIndex"`
	KeycloakClientUUID string     `json:"-" gorm:"column:keycloak_client_uuid"`
	Subject            string     `json:"subject"`
	ExpiresAt          time.Time  `json:"expires_at" gorm:"not null;index"`
	RevokedAt          *time.Time `json:"revoked_at,omitempty"`
	LastError          *string    `json:"last_error,omitempty"`
}

func (OpenShellGatewayServiceAccount) TableName() string {
	return "open_shell_gateway_service_accounts"
}

func (a *OpenShellGatewayServiceAccount) BeforeCreate(_ *gorm.DB) error {
	if a.ID == "" {
		a.ID = api.NewID()
	}
	return nil
}

// AuditEvent is intentionally separate from the resource and contains only
// non-secret lifecycle metadata.
type AuditEvent struct {
	api.Meta
	ServiceAccountID string `gorm:"not null;index"`
	GatewayID        string `gorm:"not null;index"`
	ActorUserID      string `gorm:"not null"`
	CreatorUserID    string `gorm:"not null"`
	Action           string `gorm:"not null"`
	Outcome          string `gorm:"not null"`
	Role             string `gorm:"not null"`
	CorrelationID    string
	ExpiresAt        time.Time
}

func (AuditEvent) TableName() string {
	return "open_shell_gateway_service_account_audit_events"
}

func (a *AuditEvent) BeforeCreate(_ *gorm.DB) error {
	if a.ID == "" {
		a.ID = api.NewID()
	}
	return nil
}
