package serviceAccounts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/openshift-online/hypershell/components/api-server/pkg/keycloak"
	"github.com/openshift-online/hypershell/components/api-server/pkg/rbac"
	"github.com/openshift-online/hypershell/components/api-server/plugins/gateways"
	"github.com/openshift-online/rh-trex-ai/pkg/api"
	"github.com/openshift-online/rh-trex-ai/pkg/db"
	trexerrors "github.com/openshift-online/rh-trex-ai/pkg/errors"
	"github.com/openshift-online/rh-trex-ai/pkg/logger"
	"gorm.io/gorm"
)

const (
	DefaultExpiration = 90 * 24 * time.Hour
	MinimumExpiration = time.Hour
	MaximumExpiration = 365 * 24 * time.Hour

	CreatorActiveQuota         = 10
	GatewayActiveQuota         = 100
	AccessTokenLifetimeSeconds = 300

	serviceAccountsLockType db.LockType = "gateway-service-accounts"
)

// APIError carries a stable public error code without exposing provider
// responses or credentials.
type APIError struct {
	Status  int
	Code    string
	Message string
}

func (e *APIError) Error() string { return e.Message }

type BindingLookup interface {
	FindBindingsByUserID(context.Context, string) ([]rbac.BindingSummary, error)
}

type GatewayLookup interface {
	Get(context.Context, string) (*gateways.Gateway, *trexerrors.ServiceError)
}

type KeycloakProvisioner interface {
	Configured() bool
	ProvisionServiceAccount(context.Context, keycloak.ServiceAccountSpec) (*keycloak.ProvisionedServiceAccount, error)
	ReconcileServiceAccount(context.Context, keycloak.ServiceAccountSpec, string, string, bool) error
	DisableServiceAccount(context.Context, string) error
	DeleteServiceAccount(context.Context, string) error
	DeleteManagedServiceAccount(context.Context, string, string) error
	DeleteGatewayServiceAccounts(context.Context, string) error
}

type Access struct {
	CanCreate    bool
	CanManageAll bool
	AllowedRoles []string
	Role         string
}

type GatewayOIDC struct {
	Issuer   string `json:"issuer"`
	ClientID string `json:"client_id"`
	Audience string `json:"audience"`
}

type Connection struct {
	GatewayName                string `json:"gateway_name"`
	GatewayEndpoint            string `json:"gateway_endpoint,omitempty"`
	Issuer                     string `json:"issuer"`
	TokenEndpoint              string `json:"token_endpoint"`
	GrantType                  string `json:"grant_type"`
	ClientID                   string `json:"client_id"`
	Audience                   string `json:"audience"`
	AccessTokenLifetimeSeconds int    `json:"access_token_lifetime_seconds"`
}

type Credential struct {
	Connection
	ClientSecret string `json:"client_secret"`
}

type CreateInput struct {
	Name           string
	Description    *string
	CredentialType string
	Role           string
	ExpiresAt      *time.Time
}

type CreateResult struct {
	Account    *OpenShellGatewayServiceAccount
	Credential Credential
}

type Service interface {
	Capabilities(context.Context, string, string) (Access, *APIError)
	Create(context.Context, string, string, CreateInput) (*CreateResult, *APIError)
	Get(context.Context, string, string, string) (*OpenShellGatewayServiceAccount, Connection, *APIError)
	List(context.Context, string, string, ListOptions) ([]OpenShellGatewayServiceAccount, int64, Access, *APIError)
	Revoke(context.Context, string, string, string) (*OpenShellGatewayServiceAccount, bool, *APIError)
	Delete(context.Context, string, string, string) (*OpenShellGatewayServiceAccount, bool, *APIError)
	CleanupGateway(context.Context, string) error
	ReconcileOnce(context.Context) error
}

type service struct {
	dao         ServiceAccountDao
	gateways    GatewayLookup
	bindings    BindingLookup
	keycloak    KeycloakProvisioner
	lockFactory db.LockFactory
	now         func() time.Time
}

func NewService(dao ServiceAccountDao, gatewayService GatewayLookup, bindings BindingLookup, provisioner KeycloakProvisioner, lockFactory db.LockFactory) Service {
	return &service{dao: dao, gateways: gatewayService, bindings: bindings, keycloak: provisioner, lockFactory: lockFactory, now: time.Now}
}

func (s *service) Capabilities(ctx context.Context, gatewayID, userID string) (Access, *APIError) {
	if _, _, problem := s.gateway(ctx, gatewayID, false); problem != nil {
		return Access{}, problem
	}
	access, err := s.access(ctx, gatewayID, userID)
	if err != nil {
		return Access{}, internalProblem()
	}
	if !access.CanCreate {
		return Access{}, notFoundProblem()
	}
	return access, nil
}

func (s *service) Create(ctx context.Context, gatewayID, userID string, input CreateInput) (*CreateResult, *APIError) {
	gateway, oidc, problem := s.readyGateway(ctx, gatewayID)
	if problem != nil {
		return nil, problem
	}
	access, err := s.access(ctx, gatewayID, userID)
	if err != nil {
		return nil, internalProblem()
	}
	if !access.CanCreate {
		return nil, notFoundProblem()
	}
	if problem = validateCreateInput(&input, access, s.now()); problem != nil {
		return nil, problem
	}
	if s.keycloak == nil || !s.keycloak.Configured() {
		return nil, &APIError{Status: http.StatusServiceUnavailable, Code: "keycloak_unavailable", Message: "Service-account provisioning is unavailable"}
	}

	lockOwner, lockErr := s.lockFactory.NewAdvisoryLock(ctx, gatewayID, serviceAccountsLockType)
	if lockErr != nil {
		return nil, internalProblem()
	}
	defer s.lockFactory.Unlock(ctx, lockOwner)

	gatewayCount, creatorCount, err := s.dao.CountActive(ctx, gatewayID, userID)
	if err != nil {
		return nil, internalProblem()
	}
	if creatorCount >= CreatorActiveQuota {
		return nil, &APIError{Status: http.StatusTooManyRequests, Code: "creator_quota_exceeded", Message: "The creator service-account quota is exhausted"}
	}
	if gatewayCount >= GatewayActiveQuota {
		return nil, &APIError{Status: http.StatusTooManyRequests, Code: "gateway_quota_exceeded", Message: "The gateway service-account quota is exhausted"}
	}
	if duplicate, err := s.activeNameExists(ctx, gatewayID, input.Name); err != nil {
		return nil, internalProblem()
	} else if duplicate {
		return nil, &APIError{Status: http.StatusConflict, Code: "service_account_name_conflict", Message: "An active service account already uses this name"}
	}

	expiresAt := s.now().UTC().Add(DefaultExpiration)
	if input.ExpiresAt != nil {
		expiresAt = input.ExpiresAt.UTC()
	}
	account := &OpenShellGatewayServiceAccount{
		GatewayID: gatewayID, Name: input.Name, Description: input.Description,
		CredentialType: CredentialTypeClientSecret, Role: input.Role, Status: StatusProvisioning,
		CreatedByUserID: userID, ExpiresAt: expiresAt,
	}
	account.ID = api.NewID()
	account.KeycloakClientID = fmt.Sprintf("hs-sa-%s-%s", gatewayID, account.ID)
	if err := s.dao.Create(ctx, account); err != nil {
		if duplicate, _ := s.activeNameExists(ctx, gatewayID, input.Name); duplicate {
			return nil, &APIError{Status: http.StatusConflict, Code: "service_account_name_conflict", Message: "An active service account already uses this name"}
		}
		return nil, internalProblem()
	}
	if err := s.audit(ctx, account, userID, "create", "started"); err != nil {
		return nil, internalProblem()
	}

	spec := serviceAccountSpec(account, gateway, oidc)
	provisioned, err := s.keycloak.ProvisionServiceAccount(ctx, spec)
	if err != nil {
		safe := "keycloak_provisioning_failed"
		account.Status = StatusError
		account.LastError = &safe
		_ = s.dao.Update(ctx, account)
		_ = s.audit(ctx, account, userID, "create", "failed")
		return nil, &APIError{Status: http.StatusServiceUnavailable, Code: safe, Message: "Keycloak could not provision and verify the service account"}
	}

	// Re-check the binding after the external operation. A binding revoked while
	// provisioning must not result in a usable credential being returned.
	currentAccess, accessErr := s.access(ctx, gatewayID, userID)
	if accessErr != nil || !currentAccess.CanCreate || !roleAllowed(input.Role, currentAccess.AllowedRoles) {
		_ = s.keycloak.DeleteServiceAccount(ctx, provisioned.ClientUUID)
		now := s.now().UTC()
		account.Status = StatusRevoked
		account.RevokedAt = &now
		reason := "creator_access_changed"
		account.LastError = &reason
		_ = s.dao.Update(ctx, account)
		_ = s.audit(ctx, account, userID, "create", "revoked")
		return nil, &APIError{Status: http.StatusForbidden, Code: "role_not_allowed", Message: "Gateway access changed while the service account was being created"}
	}

	account.KeycloakClientUUID = provisioned.ClientUUID
	account.Subject = provisioned.Subject
	account.Status = StatusReady
	account.LastError = nil
	if err := s.dao.Update(ctx, account); err != nil {
		_ = s.keycloak.DeleteServiceAccount(ctx, provisioned.ClientUUID)
		return nil, internalProblem()
	}
	if err := s.audit(ctx, account, userID, "create", "succeeded"); err != nil {
		_ = s.keycloak.DeleteServiceAccount(ctx, provisioned.ClientUUID)
		account.Status = StatusError
		reason := "audit_persistence_failed"
		account.LastError = &reason
		_ = s.dao.Update(ctx, account)
		return nil, internalProblem()
	}
	connection := connectionFor(account, gateway, oidc)
	return &CreateResult{Account: account, Credential: Credential{Connection: connection, ClientSecret: provisioned.ClientSecret}}, nil
}

func (s *service) Get(ctx context.Context, gatewayID, id, userID string) (*OpenShellGatewayServiceAccount, Connection, *APIError) {
	gateway, oidc, problem := s.gateway(ctx, gatewayID, true)
	if problem != nil {
		return nil, Connection{}, problem
	}
	account, problem := s.visibleAccount(ctx, gatewayID, id, userID)
	if problem != nil {
		return nil, Connection{}, problem
	}
	return account, connectionFor(account, gateway, oidc), nil
}

func (s *service) List(ctx context.Context, gatewayID, userID string, options ListOptions) ([]OpenShellGatewayServiceAccount, int64, Access, *APIError) {
	if _, _, problem := s.gateway(ctx, gatewayID, false); problem != nil {
		return nil, 0, Access{}, problem
	}
	access, err := s.access(ctx, gatewayID, userID)
	if err != nil {
		return nil, 0, Access{}, internalProblem()
	}
	if !access.CanCreate {
		return nil, 0, Access{}, notFoundProblem()
	}
	if !access.CanManageAll {
		options.CreatorUserID = userID
	}
	accounts, total, err := s.dao.List(ctx, gatewayID, options)
	if err != nil {
		return nil, 0, Access{}, internalProblem()
	}
	return accounts, total, access, nil
}

func (s *service) Revoke(ctx context.Context, gatewayID, id, userID string) (*OpenShellGatewayServiceAccount, bool, *APIError) {
	account, problem := s.visibleAccount(ctx, gatewayID, id, userID)
	if problem != nil {
		return nil, false, problem
	}
	if account.Status == StatusRevoked || account.Status == StatusExpired {
		return account, true, nil
	}
	account.Status = StatusRevoking
	account.LastError = nil
	if err := s.dao.Update(ctx, account); err != nil {
		return nil, false, internalProblem()
	}
	_ = s.audit(ctx, account, userID, "revoke", "started")
	if account.KeycloakClientUUID == "" || s.keycloak == nil || !s.keycloak.Configured() || s.keycloak.DisableServiceAccount(ctx, account.KeycloakClientUUID) != nil {
		reason := "keycloak_disable_pending"
		account.LastError = &reason
		_ = s.dao.Update(ctx, account)
		return account, false, nil
	}
	now := s.now().UTC()
	account.Status = StatusRevoked
	account.RevokedAt = &now
	account.LastError = nil
	if err := s.dao.Update(ctx, account); err != nil {
		return nil, false, internalProblem()
	}
	_ = s.audit(ctx, account, userID, "revoke", "succeeded")
	return account, true, nil
}

func (s *service) Delete(ctx context.Context, gatewayID, id, userID string) (*OpenShellGatewayServiceAccount, bool, *APIError) {
	account, problem := s.visibleAccount(ctx, gatewayID, id, userID)
	if problem != nil {
		return nil, false, problem
	}
	account.Status = StatusDeleting
	account.LastError = nil
	if err := s.dao.Update(ctx, account); err != nil {
		return nil, false, internalProblem()
	}
	_ = s.audit(ctx, account, userID, "delete", "started")
	if account.KeycloakClientUUID != "" && (s.keycloak == nil || !s.keycloak.Configured() || s.keycloak.DeleteServiceAccount(ctx, account.KeycloakClientUUID) != nil) {
		reason := "keycloak_delete_pending"
		account.LastError = &reason
		_ = s.dao.Update(ctx, account)
		return account, false, nil
	}
	if err := s.dao.SoftDelete(ctx, account); err != nil {
		return nil, false, internalProblem()
	}
	_ = s.audit(ctx, account, userID, "delete", "succeeded")
	return account, true, nil
}

func (s *service) CleanupGateway(ctx context.Context, gatewayID string) error {
	accounts, _, err := s.dao.List(ctx, gatewayID, ListOptions{Page: 1, Size: 100, Sort: "created_at", Order: "asc"})
	if err != nil {
		return err
	}
	if s.keycloak == nil || !s.keycloak.Configured() {
		if len(accounts) == 0 {
			return nil
		}
		return errors.New("Keycloak service-account cleanup is unavailable")
	}
	if err := s.keycloak.DeleteGatewayServiceAccounts(ctx, gatewayID); err != nil {
		return err
	}
	for len(accounts) > 0 {
		for i := range accounts {
			account := &accounts[i]
			if err := s.dao.SoftDelete(ctx, account); err != nil {
				return err
			}
			_ = s.audit(ctx, account, "system", "gateway_cleanup", "succeeded")
		}
		accounts, _, err = s.dao.List(ctx, gatewayID, ListOptions{Page: 1, Size: 100, Sort: "created_at", Order: "asc"})
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *service) ReconcileOnce(ctx context.Context) error {
	if s.keycloak == nil || !s.keycloak.Configured() {
		return nil
	}
	accounts, err := s.dao.ListReconcilable(ctx, 1000)
	if err != nil {
		return err
	}
	for i := range accounts {
		if err := s.reconcileOne(ctx, &accounts[i]); err != nil {
			// Persist a stable code only. Provider response bodies are intentionally
			// absent from both LastError and the caller-visible error.
			reason := "keycloak_reconciliation_failed"
			accounts[i].LastError = &reason
			_ = s.dao.Update(ctx, &accounts[i])
		}
	}
	return nil
}

func (s *service) reconcileOne(ctx context.Context, account *OpenShellGatewayServiceAccount) error {
	now := s.now().UTC()
	if account.Status == StatusDeleting {
		if account.KeycloakClientUUID != "" {
			if err := s.keycloak.DeleteServiceAccount(ctx, account.KeycloakClientUUID); err != nil {
				return err
			}
		}
		if err := s.dao.SoftDelete(ctx, account); err != nil {
			return err
		}
		return s.audit(ctx, account, "system", "delete", "succeeded")
	}
	if account.Status == StatusProvisioning {
		// The only safe recovery after a process dies before one-time delivery is
		// removal. Recreating or reading a secret would create an undisclosed live
		// credential.
		if account.KeycloakClientUUID != "" {
			_ = s.keycloak.DeleteServiceAccount(ctx, account.KeycloakClientUUID)
		} else {
			_ = s.keycloak.DeleteManagedServiceAccount(ctx, account.GatewayID, account.ID)
		}
		account.Status = StatusError
		reason := "credential_delivery_incomplete"
		account.LastError = &reason
		if err := s.dao.Update(ctx, account); err != nil {
			return err
		}
		return s.audit(ctx, account, "system", "create", "failed")
	}
	if account.Status == StatusExpired || account.Status == StatusRevoked {
		if account.KeycloakClientUUID != "" {
			return s.keycloak.DisableServiceAccount(ctx, account.KeycloakClientUUID)
		}
		return nil
	}
	if account.ExpiresAt.Before(now) || account.ExpiresAt.Equal(now) {
		if account.KeycloakClientUUID != "" {
			if err := s.keycloak.DisableServiceAccount(ctx, account.KeycloakClientUUID); err != nil {
				account.Status = StatusRevoking
				return err
			}
		}
		account.Status = StatusExpired
		account.RevokedAt = &now
		account.LastError = nil
		if err := s.dao.Update(ctx, account); err != nil {
			return err
		}
		return s.audit(ctx, account, "system", "expire", "succeeded")
	}
	access, err := s.access(ctx, account.GatewayID, account.CreatedByUserID)
	if err != nil {
		return err
	}
	if !access.CanCreate || account.Status == StatusRevoking {
		outcome := "binding_removed"
		if account.Status == StatusRevoking {
			outcome = "succeeded"
		}
		if account.KeycloakClientUUID != "" {
			if err := s.keycloak.DisableServiceAccount(ctx, account.KeycloakClientUUID); err != nil {
				return err
			}
		}
		account.Status = StatusRevoked
		account.RevokedAt = &now
		account.LastError = nil
		if err := s.dao.Update(ctx, account); err != nil {
			return err
		}
		return s.audit(ctx, account, "system", "revoke", outcome)
	}
	if account.Status == StatusError {
		// An error record may represent an undisclosed credential. Replacement is
		// required; reconciliation never fetches or regenerates its secret.
		if account.KeycloakClientUUID != "" {
			_ = s.keycloak.DisableServiceAccount(ctx, account.KeycloakClientUUID)
		}
		return nil
	}
	downgraded := account.Role == RoleAdmin && !roleAllowed(RoleAdmin, access.AllowedRoles)
	if downgraded {
		account.Role = RoleUser
		if err := s.dao.Update(ctx, account); err != nil {
			return err
		}
	}
	gateway, oidc, problem := s.gateway(ctx, account.GatewayID, true)
	if problem != nil {
		if account.KeycloakClientUUID != "" {
			_ = s.keycloak.DisableServiceAccount(ctx, account.KeycloakClientUUID)
		}
		account.Status = StatusRevoked
		account.RevokedAt = &now
		return s.dao.Update(ctx, account)
	}
	if account.KeycloakClientUUID == "" {
		account.Status = StatusError
		reason := "keycloak_client_missing"
		account.LastError = &reason
		return s.dao.Update(ctx, account)
	}
	if err := s.keycloak.ReconcileServiceAccount(ctx, serviceAccountSpec(account, gateway, oidc), account.KeycloakClientUUID, account.Subject, true); err != nil {
		if errors.Is(err, keycloak.ErrNotFound) {
			account.Status = StatusError
			reason := "keycloak_client_missing"
			account.LastError = &reason
			return s.dao.Update(ctx, account)
		}
		return err
	}
	account.Status = StatusReady
	account.LastError = nil
	if err := s.dao.Update(ctx, account); err != nil {
		return err
	}
	if downgraded {
		return s.audit(ctx, account, "system", "role_downgrade", "succeeded")
	}
	return nil
}

func (s *service) visibleAccount(ctx context.Context, gatewayID, id, userID string) (*OpenShellGatewayServiceAccount, *APIError) {
	access, err := s.access(ctx, gatewayID, userID)
	if err != nil {
		return nil, internalProblem()
	}
	if !access.CanCreate {
		return nil, notFoundProblem()
	}
	account, err := s.dao.Get(ctx, gatewayID, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, notFoundProblem()
	}
	if err != nil {
		return nil, internalProblem()
	}
	if !access.CanManageAll && account.CreatedByUserID != userID {
		return nil, notFoundProblem()
	}
	return account, nil
}

func (s *service) access(ctx context.Context, gatewayID, userID string) (Access, error) {
	if userID == "" || s.bindings == nil {
		return Access{}, nil
	}
	bindings, err := s.bindings.FindBindingsByUserID(ctx, userID)
	if err != nil {
		return Access{}, err
	}
	result := Access{}
	for _, binding := range bindings {
		if binding.Scope != "gateway" || binding.GatewayID == nil || *binding.GatewayID != gatewayID {
			continue
		}
		if binding.RoleName == "gateway:owner" {
			return Access{CanCreate: true, CanManageAll: true, AllowedRoles: []string{RoleUser, RoleAdmin}, Role: "gateway:owner"}, nil
		}
		if binding.RoleName == "gateway:viewer" {
			result = Access{CanCreate: true, CanManageAll: false, AllowedRoles: []string{RoleUser}, Role: "gateway:viewer"}
		}
	}
	return result, nil
}

func (s *service) readyGateway(ctx context.Context, gatewayID string) (*gateways.Gateway, GatewayOIDC, *APIError) {
	gateway, oidc, problem := s.gateway(ctx, gatewayID, true)
	if problem != nil {
		return nil, GatewayOIDC{}, problem
	}
	if gateway.Phase == nil || !strings.EqualFold(*gateway.Phase, "Running") || gateway.Status == nil || !strings.EqualFold(*gateway.Status, "Healthy") {
		return nil, GatewayOIDC{}, &APIError{Status: http.StatusConflict, Code: "gateway_not_ready", Message: "The gateway is not ready for service-account provisioning"}
	}
	return gateway, oidc, nil
}

func (s *service) gateway(ctx context.Context, gatewayID string, requireOIDC bool) (*gateways.Gateway, GatewayOIDC, *APIError) {
	if s.gateways == nil {
		return nil, GatewayOIDC{}, internalProblem()
	}
	gateway, svcErr := s.gateways.Get(ctx, gatewayID)
	if svcErr != nil {
		return nil, GatewayOIDC{}, notFoundProblem()
	}
	var oidc GatewayOIDC
	if gateway.Oidc != nil {
		_ = json.Unmarshal([]byte(*gateway.Oidc), &oidc)
	}
	if requireOIDC && (oidc.Issuer == "" || oidc.ClientID == "" || oidc.Audience == "") {
		return nil, GatewayOIDC{}, &APIError{Status: http.StatusConflict, Code: "gateway_oidc_not_ready", Message: "The gateway OIDC client is not ready"}
	}
	return gateway, oidc, nil
}

func validateCreateInput(input *CreateInput, access Access, now time.Time) *APIError {
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || len(input.Name) > 128 {
		return validationProblem("name must contain 1 to 128 characters")
	}
	if input.Description != nil && len(*input.Description) > 1024 {
		return validationProblem("description must not exceed 1024 characters")
	}
	if input.CredentialType == "" {
		input.CredentialType = CredentialTypeClientSecret
	}
	if input.CredentialType != CredentialTypeClientSecret {
		return validationProblem("credential_type must be client_secret")
	}
	if input.Role == "" {
		input.Role = RoleUser
	}
	if input.Role != RoleUser && input.Role != RoleAdmin {
		return validationProblem("role must be openshell-user or openshell-admin")
	}
	if !roleAllowed(input.Role, access.AllowedRoles) {
		return &APIError{Status: http.StatusForbidden, Code: "role_not_allowed", Message: "The requested role exceeds the caller's gateway role"}
	}
	if input.ExpiresAt != nil {
		duration := input.ExpiresAt.Sub(now)
		if duration < MinimumExpiration || duration > MaximumExpiration {
			return validationProblem("expires_at must be between 1 hour and 365 days from now")
		}
	}
	return nil
}

func (s *service) activeNameExists(ctx context.Context, gatewayID, name string) (bool, error) {
	items, _, err := s.dao.List(ctx, gatewayID, ListOptions{Page: 1, Size: GatewayActiveQuota, Search: name, Sort: "created_at", Order: "asc"})
	if err != nil {
		return false, err
	}
	for _, item := range items {
		if strings.EqualFold(item.Name, name) && item.Status != StatusExpired && item.Status != StatusRevoked {
			return true, nil
		}
	}
	return false, nil
}

func serviceAccountSpec(account *OpenShellGatewayServiceAccount, gateway *gateways.Gateway, oidc GatewayOIDC) keycloak.ServiceAccountSpec {
	return keycloak.ServiceAccountSpec{
		ClientID: account.KeycloakClientID, DisplayName: account.Name,
		GatewayClientID: oidc.ClientID, GatewayID: account.GatewayID,
		ServiceAccountID: account.ID, CreatorUserID: account.CreatedByUserID,
		Role: account.Role, ExpectedIssuer: oidc.Issuer,
		AccessTokenLifetimeSeconds: AccessTokenLifetimeSeconds,
	}
}

func connectionFor(account *OpenShellGatewayServiceAccount, gateway *gateways.Gateway, oidc GatewayOIDC) Connection {
	endpoint := ""
	if gateway.RouteAddress != nil {
		endpoint = *gateway.RouteAddress
	}
	return Connection{
		GatewayName: gateway.Name, GatewayEndpoint: endpoint, Issuer: oidc.Issuer,
		TokenEndpoint: strings.TrimRight(oidc.Issuer, "/") + "/protocol/openid-connect/token",
		GrantType:     "client_credentials", ClientID: account.KeycloakClientID,
		Audience: oidc.Audience, AccessTokenLifetimeSeconds: AccessTokenLifetimeSeconds,
	}
}

func (s *service) audit(ctx context.Context, account *OpenShellGatewayServiceAccount, actor, action, outcome string) error {
	return s.dao.CreateAudit(ctx, &AuditEvent{
		ServiceAccountID: account.ID, GatewayID: account.GatewayID,
		ActorUserID: actor, CreatorUserID: account.CreatedByUserID,
		Action: action, Outcome: outcome, Role: account.Role, ExpiresAt: account.ExpiresAt,
		CorrelationID: logger.GetOperationID(ctx),
	})
}

func roleAllowed(role string, allowed []string) bool {
	for _, item := range allowed {
		if item == role {
			return true
		}
	}
	return false
}

func validationProblem(message string) *APIError {
	return &APIError{Status: http.StatusBadRequest, Code: "invalid_request", Message: message}
}

func notFoundProblem() *APIError {
	return &APIError{Status: http.StatusNotFound, Code: "not_found", Message: "The gateway service account was not found"}
}

func internalProblem() *APIError {
	return &APIError{Status: http.StatusInternalServerError, Code: "internal_error", Message: "The request could not be completed"}
}
