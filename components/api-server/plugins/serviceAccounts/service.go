package serviceAccounts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang/glog"
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

// ErrProvisionerNotFound means that the managed Keycloak client no longer
// exists. It is intentionally provider-neutral at the API boundary.
var ErrProvisionerNotFound = errors.New("provisioned service account not found")

// ProvisioningSpec is the complete desired identity state sent to the control
// plane. It contains no client secret.
type ProvisioningSpec struct {
	ClientID                   string
	DisplayName                string
	GatewayClientID            string
	GatewayID                  string
	ServiceAccountID           string
	CreatorUserID              string
	Role                       string
	ExpectedIssuer             string
	AccessTokenLifetimeSeconds int
}

// ProvisionedServiceAccount contains persistent provider identifiers and the
// one-time secret returned synchronously by the control plane.
type ProvisionedServiceAccount struct {
	ClientUUID   string
	ClientID     string
	ClientSecret string
	Subject      string
}

type ManagedClient struct {
	UUID             string
	ClientID         string
	GatewayID        string
	ServiceAccountID string
}

type ServiceAccountProvisioner interface {
	Configured() bool
	ProvisionServiceAccount(context.Context, ProvisioningSpec) (*ProvisionedServiceAccount, error)
	ReconcileServiceAccount(context.Context, ProvisioningSpec, string, string, bool) error
	DisableServiceAccount(context.Context, string) error
	DeleteServiceAccount(context.Context, string) error
	DeleteManagedServiceAccount(context.Context, string, string) error
	DeleteGatewayServiceAccounts(context.Context, string) error
	ListManagedClients(context.Context, string) ([]ManagedClient, error)
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
	provisioner ServiceAccountProvisioner
	lockFactory db.LockFactory
	now         func() time.Time
}

func NewService(dao ServiceAccountDao, gatewayService GatewayLookup, bindings BindingLookup, provisioner ServiceAccountProvisioner, lockFactory db.LockFactory) Service {
	return &service{dao: dao, gateways: gatewayService, bindings: bindings, provisioner: provisioner, lockFactory: lockFactory, now: time.Now}
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
	access, err := s.access(ctx, gatewayID, userID)
	if err != nil {
		return nil, internalProblem()
	}
	if !access.CanCreate {
		return nil, notFoundProblem()
	}
	gateway, oidc, problem := s.readyGateway(ctx, gatewayID)
	if problem != nil {
		return nil, problem
	}
	if problem = validateCreateInput(&input, access, s.now()); problem != nil {
		return nil, problem
	}
	if s.provisioner == nil || !s.provisioner.Configured() {
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
	if duplicate, err := s.dao.ActiveNameExists(ctx, gatewayID, input.Name); err != nil {
		return nil, internalProblem()
	} else if duplicate {
		return nil, &APIError{Status: http.StatusConflict, Code: "service_account_name_exists", Message: "An active service account already uses this name"}
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
		if duplicate, _ := s.dao.ActiveNameExists(ctx, gatewayID, input.Name); duplicate {
			return nil, &APIError{Status: http.StatusConflict, Code: "service_account_name_exists", Message: "An active service account already uses this name"}
		}
		return nil, internalProblem()
	}
	if err := s.audit(ctx, account, userID, "create", "started"); err != nil {
		return nil, internalProblem()
	}

	spec := serviceAccountSpec(account, gateway, oidc)
	provisioned, err := s.provisioner.ProvisionServiceAccount(ctx, spec)
	if err != nil {
		glog.Warningf("Keycloak service-account provisioning failed for gateway %q and account %q: %v", gatewayID, account.ID, err)
		const safe = "keycloak_provisioning_failed"
		_ = s.cleanupUndeliveredAccount(ctx, account, userID, safe)
		return nil, &APIError{Status: http.StatusServiceUnavailable, Code: safe, Message: "Keycloak could not provision and verify the service account"}
	}

	// Re-check the binding after the external operation. A binding revoked while
	// provisioning must not result in a usable credential being returned.
	currentAccess, accessErr := s.access(ctx, gatewayID, userID)
	if accessErr != nil || !currentAccess.CanCreate || !roleAllowed(input.Role, currentAccess.AllowedRoles) {
		account.KeycloakClientUUID = provisioned.ClientUUID
		account.Subject = provisioned.Subject
		now := s.now().UTC()
		reason := "creator_access_changed"
		account.LastError = &reason
		if cleanupErr := s.provisioner.DeleteServiceAccount(ctx, provisioned.ClientUUID); cleanupErr != nil {
			account.Status = StatusError
		} else {
			account.Status = StatusRevoked
			account.RevokedAt = &now
		}
		if auditErr := s.audit(ctx, account, userID, "create", "revoked"); auditErr != nil {
			account.Status = StatusError
			account.RevokedAt = nil
			reason = "audit_persistence_failed"
			account.LastError = &reason
		}
		_ = s.dao.Update(ctx, account)
		return nil, &APIError{Status: http.StatusForbidden, Code: "role_not_allowed", Message: "Gateway access changed while the service account was being created"}
	}

	account.KeycloakClientUUID = provisioned.ClientUUID
	account.Subject = provisioned.Subject
	account.Status = StatusReady
	account.LastError = nil
	if err := s.audit(ctx, account, userID, "create", "succeeded"); err != nil {
		_ = s.provisioner.DeleteServiceAccount(ctx, provisioned.ClientUUID)
		account.Status = StatusError
		reason := "audit_persistence_failed"
		account.LastError = &reason
		_ = s.dao.Update(ctx, account)
		return nil, internalProblem()
	}
	if err := s.dao.Update(ctx, account); err != nil {
		_ = s.provisioner.DeleteServiceAccount(ctx, provisioned.ClientUUID)
		return nil, internalProblem()
	}
	connection := connectionFor(account, gateway, oidc)
	return &CreateResult{Account: account, Credential: Credential{Connection: connection, ClientSecret: provisioned.ClientSecret}}, nil
}

func (s *service) Get(ctx context.Context, gatewayID, id, userID string) (*OpenShellGatewayServiceAccount, Connection, *APIError) {
	account, problem := s.visibleAccount(ctx, gatewayID, id, userID)
	if problem != nil {
		return nil, Connection{}, problem
	}
	gateway, oidc, problem := s.gateway(ctx, gatewayID, true)
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
		if s.provisioner == nil || !s.provisioner.Configured() {
			return account, false, nil
		}
		if account.KeycloakClientUUID != "" {
			if err := s.provisioner.DisableServiceAccount(ctx, account.KeycloakClientUUID); err != nil {
				return account, false, nil
			}
		} else if err := s.provisioner.DeleteManagedServiceAccount(ctx, account.GatewayID, account.ID); err != nil {
			return account, false, nil
		}
		return account, true, nil
	}
	account.Status = StatusRevoking
	account.LastError = nil
	if err := s.dao.Update(ctx, account); err != nil {
		return nil, false, serviceUnavailableProblem("The revocation request could not be stored")
	}
	if err := s.audit(ctx, account, userID, "revoke", "started"); err != nil {
		return account, false, nil
	}
	if s.provisioner == nil || !s.provisioner.Configured() {
		reason := "keycloak_disable_pending"
		account.LastError = &reason
		_ = s.dao.Update(ctx, account)
		return account, false, nil
	}
	var disableErr error
	if account.KeycloakClientUUID != "" {
		disableErr = s.provisioner.DisableServiceAccount(ctx, account.KeycloakClientUUID)
	} else {
		disableErr = s.provisioner.DeleteManagedServiceAccount(ctx, account.GatewayID, account.ID)
	}
	if disableErr != nil {
		reason := "keycloak_disable_pending"
		account.LastError = &reason
		_ = s.dao.Update(ctx, account)
		return account, false, nil
	}
	now := s.now().UTC()
	account.Status = StatusRevoked
	account.RevokedAt = &now
	account.LastError = nil
	if err := s.audit(ctx, account, userID, "revoke", "succeeded"); err != nil {
		account.Status = StatusRevoking
		account.RevokedAt = nil
		return account, false, nil
	}
	if err := s.dao.Update(ctx, account); err != nil {
		account.Status = StatusRevoking
		account.RevokedAt = nil
		return account, false, nil
	}
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
		return nil, false, serviceUnavailableProblem("The deletion request could not be stored")
	}
	if err := s.audit(ctx, account, userID, "delete", "started"); err != nil {
		return account, false, nil
	}
	if s.provisioner == nil || !s.provisioner.Configured() {
		reason := "keycloak_delete_pending"
		account.LastError = &reason
		_ = s.dao.Update(ctx, account)
		return account, false, nil
	}
	var deleteErr error
	if account.KeycloakClientUUID != "" {
		deleteErr = s.provisioner.DeleteServiceAccount(ctx, account.KeycloakClientUUID)
	} else {
		deleteErr = s.provisioner.DeleteManagedServiceAccount(ctx, account.GatewayID, account.ID)
	}
	if deleteErr != nil {
		reason := "keycloak_delete_pending"
		account.LastError = &reason
		_ = s.dao.Update(ctx, account)
		return account, false, nil
	}
	if err := s.audit(ctx, account, userID, "delete", "succeeded"); err != nil {
		return account, false, nil
	}
	if err := s.dao.SoftDelete(ctx, account); err != nil {
		return account, false, nil
	}
	return account, true, nil
}

func (s *service) CleanupGateway(ctx context.Context, gatewayID string) error {
	accounts, _, err := s.dao.List(ctx, gatewayID, ListOptions{Page: 1, Size: 100, Sort: "created_at", Order: "asc"})
	if err != nil {
		return err
	}
	if s.provisioner == nil || !s.provisioner.Configured() {
		if len(accounts) == 0 {
			return nil
		}
		return errors.New("keycloak service-account cleanup is unavailable")
	}
	if err := s.provisioner.DeleteGatewayServiceAccounts(ctx, gatewayID); err != nil {
		return err
	}
	for len(accounts) > 0 {
		for i := range accounts {
			account := &accounts[i]
			if err := s.audit(ctx, account, "system", "gateway_cleanup", "succeeded"); err != nil {
				return err
			}
			if err := s.dao.SoftDelete(ctx, account); err != nil {
				return err
			}
		}
		accounts, _, err = s.dao.List(ctx, gatewayID, ListOptions{Page: 1, Size: 100, Sort: "created_at", Order: "asc"})
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *service) ReconcileOnce(ctx context.Context) error {
	if s.provisioner == nil || !s.provisioner.Configured() {
		return nil
	}
	accounts, err := s.dao.ListReconcilable(ctx, 1000)
	if err != nil {
		return err
	}
	var reconcileErrors []error
	for i := range accounts {
		original := accounts[i]
		if err := s.reconcileOne(ctx, &accounts[i]); err != nil {
			// Persist a stable code only. Provider response bodies are intentionally
			// absent from both LastError and the caller-visible error.
			reason := "keycloak_reconciliation_failed"
			original.LastError = &reason
			if updateErr := s.dao.Update(ctx, &original); updateErr != nil {
				err = errors.Join(err, updateErr)
			}
			reconcileErrors = append(reconcileErrors, err)
		}
	}
	if err := s.cleanupOrphanedClients(ctx); err != nil {
		reconcileErrors = append(reconcileErrors, err)
	}
	return errors.Join(reconcileErrors...)
}

func (s *service) cleanupOrphanedClients(ctx context.Context) error {
	clients, err := s.provisioner.ListManagedClients(ctx, "")
	if err != nil {
		return err
	}
	var cleanupErrors []error
	for _, client := range clients {
		account, getErr := s.dao.Get(ctx, client.GatewayID, client.ServiceAccountID)
		orphaned := errors.Is(getErr, gorm.ErrRecordNotFound)
		if getErr != nil && !orphaned {
			cleanupErrors = append(cleanupErrors, getErr)
			continue
		}
		if account != nil {
			if account.KeycloakClientUUID != "" && account.KeycloakClientUUID != client.UUID {
				orphaned = true
			} else if _, gatewayErr := s.gateways.Get(ctx, client.GatewayID); gatewayErr != nil {
				if gatewayErr.HttpCode == http.StatusNotFound {
					orphaned = true
				} else {
					cleanupErrors = append(cleanupErrors, errors.New("check managed client gateway"))
					continue
				}
			}
		}
		if orphaned {
			if deleteErr := s.provisioner.DeleteServiceAccount(ctx, client.UUID); deleteErr != nil {
				cleanupErrors = append(cleanupErrors, deleteErr)
			}
		}
	}
	return errors.Join(cleanupErrors...)
}

func (s *service) reconcileOne(ctx context.Context, account *OpenShellGatewayServiceAccount) error {
	now := s.now().UTC()
	if account.Status == StatusDeleting {
		if account.KeycloakClientUUID != "" {
			if err := s.provisioner.DeleteServiceAccount(ctx, account.KeycloakClientUUID); err != nil {
				return err
			}
		} else if err := s.provisioner.DeleteManagedServiceAccount(ctx, account.GatewayID, account.ID); err != nil {
			return err
		}
		if err := s.audit(ctx, account, "system", "delete", "succeeded"); err != nil {
			return err
		}
		return s.dao.SoftDelete(ctx, account)
	}
	if account.Status == StatusProvisioning {
		// The only safe recovery after a process dies before one-time delivery is
		// removal. Recreating or reading a secret would create an undisclosed live
		// credential.
		return s.cleanupUndeliveredAccount(ctx, account, "system", "credential_delivery_incomplete")
	}
	if account.Status == StatusExpired || account.Status == StatusRevoked {
		if account.KeycloakClientUUID != "" {
			if err := s.provisioner.DisableServiceAccount(ctx, account.KeycloakClientUUID); err != nil {
				return err
			}
		} else if err := s.provisioner.DeleteManagedServiceAccount(ctx, account.GatewayID, account.ID); err != nil {
			return err
		}
		// Advance updated_at after a successful drift check so terminal records do
		// not permanently occupy the oldest reconciliation page.
		account.LastError = nil
		return s.dao.Update(ctx, account)
	}
	if account.ExpiresAt.Before(now) || account.ExpiresAt.Equal(now) {
		if account.KeycloakClientUUID != "" {
			if err := s.provisioner.DisableServiceAccount(ctx, account.KeycloakClientUUID); err != nil {
				account.Status = StatusRevoking
				return err
			}
		} else if err := s.provisioner.DeleteManagedServiceAccount(ctx, account.GatewayID, account.ID); err != nil {
			return err
		}
		account.Status = StatusExpired
		account.RevokedAt = &now
		account.LastError = nil
		if err := s.audit(ctx, account, "system", "expire", "succeeded"); err != nil {
			return err
		}
		if err := s.dao.Update(ctx, account); err != nil {
			return err
		}
		return nil
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
			if err := s.provisioner.DisableServiceAccount(ctx, account.KeycloakClientUUID); err != nil {
				return err
			}
		} else if err := s.provisioner.DeleteManagedServiceAccount(ctx, account.GatewayID, account.ID); err != nil {
			return err
		}
		account.Status = StatusRevoked
		account.RevokedAt = &now
		account.LastError = nil
		if err := s.audit(ctx, account, "system", "revoke", outcome); err != nil {
			return err
		}
		if err := s.dao.Update(ctx, account); err != nil {
			return err
		}
		return nil
	}
	if account.Status == StatusError {
		// An error record may represent an undisclosed credential. Replacement is
		// required; reconciliation never fetches or regenerates its secret.
		reason := "credential_delivery_incomplete"
		if account.LastError != nil && *account.LastError != "" {
			reason = *account.LastError
		}
		return s.cleanupUndeliveredAccount(ctx, account, "system", reason)
	}
	downgraded := account.Role == RoleAdmin && !roleAllowed(RoleAdmin, access.AllowedRoles)
	gateway, oidc, problem := s.gateway(ctx, account.GatewayID, true)
	if problem != nil {
		if account.KeycloakClientUUID != "" {
			if err := s.provisioner.DisableServiceAccount(ctx, account.KeycloakClientUUID); err != nil {
				return err
			}
		} else if err := s.provisioner.DeleteManagedServiceAccount(ctx, account.GatewayID, account.ID); err != nil {
			return err
		}
		account.Status = StatusRevoked
		account.RevokedAt = &now
		if err := s.audit(ctx, account, "system", "revoke", "gateway_unavailable"); err != nil {
			return err
		}
		return s.dao.Update(ctx, account)
	}
	if account.KeycloakClientUUID == "" {
		account.Status = StatusError
		reason := "keycloak_client_missing"
		account.LastError = &reason
		return s.dao.Update(ctx, account)
	}
	spec := serviceAccountSpec(account, gateway, oidc)
	if downgraded {
		spec.Role = RoleUser
	}
	if err := s.provisioner.ReconcileServiceAccount(ctx, spec, account.KeycloakClientUUID, account.Subject, true); err != nil {
		if errors.Is(err, ErrProvisionerNotFound) {
			account.Status = StatusError
			reason := "keycloak_client_missing"
			account.LastError = &reason
			return s.dao.Update(ctx, account)
		}
		return err
	}
	if downgraded {
		account.Role = RoleUser
		if err := s.audit(ctx, account, "system", "role_downgrade", "succeeded"); err != nil {
			return err
		}
	}
	account.Status = StatusReady
	account.LastError = nil
	if err := s.dao.Update(ctx, account); err != nil {
		return err
	}
	return nil
}

func (s *service) cleanupUndeliveredAccount(ctx context.Context, account *OpenShellGatewayServiceAccount, actor, reason string) error {
	account.Status = StatusError
	account.LastError = &reason
	if err := s.dao.Update(ctx, account); err != nil {
		return err
	}
	if account.KeycloakClientUUID != "" {
		if err := s.provisioner.DeleteServiceAccount(ctx, account.KeycloakClientUUID); err != nil {
			return err
		}
	} else if err := s.provisioner.DeleteManagedServiceAccount(ctx, account.GatewayID, account.ID); err != nil {
		return err
	}
	if err := s.audit(ctx, account, actor, "create", "failed"); err != nil {
		return err
	}
	return s.dao.SoftDelete(ctx, account)
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
		return nil, GatewayOIDC{}, &APIError{Status: http.StatusConflict, Code: "gateway_not_ready", Message: "The gateway OIDC client is not ready"}
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

func serviceAccountSpec(account *OpenShellGatewayServiceAccount, gateway *gateways.Gateway, oidc GatewayOIDC) ProvisioningSpec {
	return ProvisioningSpec{
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
		endpoint = gatewayEndpoint(*gateway.RouteAddress)
	}
	return Connection{
		GatewayName: gateway.Name, GatewayEndpoint: endpoint, Issuer: oidc.Issuer,
		TokenEndpoint: strings.TrimRight(oidc.Issuer, "/") + "/protocol/openid-connect/token",
		GrantType:     "client_credentials", ClientID: account.KeycloakClientID,
		Audience: oidc.Audience, AccessTokenLifetimeSeconds: AccessTokenLifetimeSeconds,
	}
}

func gatewayEndpoint(routeAddress string) string {
	endpoint := strings.TrimSpace(routeAddress)
	if strings.HasPrefix(endpoint, "grpcs://") {
		return "https://" + strings.TrimPrefix(endpoint, "grpcs://")
	}
	if strings.HasPrefix(endpoint, "grpc://") {
		return "http://" + strings.TrimPrefix(endpoint, "grpc://")
	}
	return endpoint
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

func serviceUnavailableProblem(message string) *APIError {
	return &APIError{Status: http.StatusServiceUnavailable, Code: "service_unavailable", Message: message}
}
