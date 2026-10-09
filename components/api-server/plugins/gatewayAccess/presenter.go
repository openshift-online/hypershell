package gatewayAccess

import "time"

type grantItemResponse struct {
	RoleBindingID string    `json:"role_binding_id"`
	UserID        string    `json:"user_id"`
	Username      string    `json:"username"`
	Name          *string   `json:"name,omitempty"`
	Email         *string   `json:"email,omitempty"`
	Role          string    `json:"role"`
	GrantedAt     time.Time `json:"granted_at"`
}

type capabilitiesResponse struct {
	CallerRole      string `json:"caller_role,omitempty"`
	CanManageAccess bool   `json:"can_manage_access"`
	CanManageOwners bool   `json:"can_manage_owners"`
}

type listResponse struct {
	Page         int                  `json:"page"`
	Size         int                  `json:"size"`
	Total        int64                `json:"total"`
	Capabilities capabilitiesResponse `json:"capabilities"`
	Items        []grantItemResponse  `json:"items"`
}

type directoryCandidateResponse struct {
	Username string `json:"username"`
	Name     string `json:"name,omitempty"`
	Email    string `json:"email,omitempty"`
	Subject  string `json:"subject,omitempty"`
}

type directoryResponse struct {
	Items []directoryCandidateResponse `json:"items"`
}

// The response types share field names, types, and order with their domain
// counterparts; a direct conversion keeps the JSON tags while avoiding a
// field-by-field copy. A later divergence turns into a compile error here.
func presentGrant(item GrantItem) grantItemResponse {
	return grantItemResponse(item)
}

func presentCapabilities(c Capabilities) capabilitiesResponse {
	return capabilitiesResponse(c)
}

func presentCandidate(c DirectoryCandidate) directoryCandidateResponse {
	return directoryCandidateResponse(c)
}
