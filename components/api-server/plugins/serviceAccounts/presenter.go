package serviceAccounts

import "time"

type listItemResponse struct {
	ID              string     `json:"id"`
	GatewayID       string     `json:"gateway_id"`
	Name            string     `json:"name"`
	Description     *string    `json:"description"`
	CredentialType  string     `json:"credential_type"`
	Role            string     `json:"role"`
	Status          string     `json:"status"`
	CreatedByUserID string     `json:"created_by_user_id"`
	ClientID        string     `json:"client_id"`
	Subject         string     `json:"subject"`
	ExpiresAt       time.Time  `json:"expires_at"`
	RevokedAt       *time.Time `json:"revoked_at"`
	LastError       *string    `json:"last_error"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type createResponse struct {
	listItemResponse
	Credential Credential `json:"credential"`
}

type getResponse struct {
	listItemResponse
	Connection Connection `json:"connection"`
}

type expirationPolicyResponse struct {
	DefaultSeconds int64 `json:"default_seconds"`
	MinimumSeconds int64 `json:"minimum_seconds"`
	MaximumSeconds int64 `json:"maximum_seconds"`
}

type capabilitiesResponse struct {
	CanCreate        bool                     `json:"can_create"`
	AllowedRoles     []string                 `json:"allowed_roles"`
	CanManageAll     bool                     `json:"can_manage_all"`
	ExpirationPolicy expirationPolicyResponse `json:"expiration_policy"`
}

type listResponse struct {
	Page         int                  `json:"page"`
	Size         int                  `json:"size"`
	Total        int64                `json:"total"`
	Capabilities capabilitiesResponse `json:"capabilities"`
	Items        []listItemResponse   `json:"items"`
}

func presentItem(account *OpenShellGatewayServiceAccount) listItemResponse {
	return listItemResponse{
		ID: account.ID, GatewayID: account.GatewayID, Name: account.Name,
		Description: account.Description, CredentialType: account.CredentialType,
		Role: account.Role, Status: account.Status, CreatedByUserID: account.CreatedByUserID,
		ClientID: account.KeycloakClientID, Subject: account.Subject,
		ExpiresAt: account.ExpiresAt, RevokedAt: account.RevokedAt,
		LastError: account.LastError, CreatedAt: account.CreatedAt, UpdatedAt: account.UpdatedAt,
	}
}

func presentCapabilities(access Access) capabilitiesResponse {
	roles := append([]string(nil), access.AllowedRoles...)
	if roles == nil {
		roles = []string{}
	}
	return capabilitiesResponse{
		CanCreate: access.CanCreate, AllowedRoles: roles, CanManageAll: access.CanManageAll,
		ExpirationPolicy: expirationPolicyResponse{
			DefaultSeconds: int64(DefaultExpiration.Seconds()),
			MinimumSeconds: int64(MinimumExpiration.Seconds()),
			MaximumSeconds: int64(MaximumExpiration.Seconds()),
		},
	}
}
