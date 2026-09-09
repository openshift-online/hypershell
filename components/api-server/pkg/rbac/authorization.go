package rbac

import (
	"context"
	"net/http"
	"strings"

	"github.com/gorilla/mux"

	"github.com/openshift-online/rh-trex-ai/pkg/auth"
)

type RoleBindingLookup interface {
	FindBindingsByUserID(ctx context.Context, userID string) ([]BindingSummary, error)
}

type BindingSummary struct {
	RoleName  string
	Scope     string
	GatewayID *string
}

type AuthzConfig struct {
	EnforceRBAC     bool
	ServiceAccounts []string
}

type rbacAuthzMiddleware struct {
	lookup RoleBindingLookup
	config AuthzConfig
}

var _ auth.AuthorizationMiddleware = &rbacAuthzMiddleware{}

func NewRBACAuthzMiddleware(lookup RoleBindingLookup, config AuthzConfig) auth.AuthorizationMiddleware {
	return &rbacAuthzMiddleware{
		lookup: lookup,
		config: config,
	}
}

func (m *rbacAuthzMiddleware) AuthorizeApi(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !m.config.EnforceRBAC {
			next.ServeHTTP(w, r)
			return
		}

		// Derive identity from the JWT payload directly (like
		// UserProvisioningMiddleware), NOT from GetUsernameFromContext. Both this
		// middleware and UserProvisioningMiddleware are attached on the parent
		// apiV1Router, which runs BEFORE the child-subrouter AuthenticateAccountJWT
		// that populates the username context. On HTTP the framework's global
		// jwtHandler has already validated the token and placed it in the request
		// context, so GetAuthPayload works at this level while GetUsernameFromContext
		// is still empty. (The gRPC path is unaffected: its post-auth interceptor
		// runs after AuthUnaryInterceptor, which sets the username.)
		payload, err := auth.GetAuthPayload(r)
		if err != nil || payload == nil || payload.Username == "" {
			http.Error(w, "Unauthorized: missing identity", http.StatusUnauthorized)
			return
		}

		if isExemptEndpoint(r) {
			next.ServeHTTP(w, r)
			return
		}

		userID := GetUserIDFromContext(r.Context())
		if userID == "" {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		bindings, err := m.lookup.FindBindingsByUserID(r.Context(), userID)
		if err != nil {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		resource, resourceID := extractResourceInfo(r)
		gatewayID := extractGatewayID(r, resource)
		jwtRoles := GetJWTRolesFromContext(r.Context())

		if !isAuthorized(r.Method, resource, resourceID, gatewayID, bindings, jwtRoles) {
			if resource == "service_accounts" || (r.Method == http.MethodGet && resourceID != "") {
				http.Error(w, "Not Found", http.StatusNotFound)
			} else {
				http.Error(w, "Forbidden", http.StatusForbidden)
			}
			return
		}

		next.ServeHTTP(w, r)
	})
}

func isExemptEndpoint(r *http.Request) bool {
	path := r.URL.Path

	if strings.HasSuffix(path, "/metadata") {
		return true
	}

	if strings.HasSuffix(path, "/openapi") || strings.HasSuffix(path, "/openapi.html") {
		return true
	}

	if strings.HasSuffix(path, "/errors") {
		return true
	}

	if r.Method == http.MethodGet && (strings.HasSuffix(path, "/roles") || strings.Contains(path, "/roles/")) {
		return true
	}

	return false
}

func hasGatewayCreator(bindings []BindingSummary) bool {
	for _, b := range bindings {
		if b.RoleName == "gateway:creator" {
			return true
		}
	}
	return false
}

func hasPlatformAdmin(bindings []BindingSummary) bool {
	for _, b := range bindings {
		if b.RoleName == "platform:admin" {
			return true
		}
	}
	return false
}

func hasUsersInventoryAccess(bindings []BindingSummary, jwtRoles []string) bool {
	return hasPlatformAdmin(bindings) || HasHypershellAdminRole(jwtRoles)
}

func extractResourceInfo(r *http.Request) (resource string, resourceID string) {
	resource, resourceID = extractResourceInfoFromRoute(r)
	if resource != "" {
		return resource, resourceID
	}
	return extractResourceInfoFromPath(r.URL.Path)
}

func extractResourceInfoFromRoute(r *http.Request) (resource string, resourceID string) {
	route := mux.CurrentRoute(r)
	if route == nil {
		return "", ""
	}

	pathTemplate, err := route.GetPathTemplate()
	if err != nil {
		return "", ""
	}
	if strings.Contains(pathTemplate, "/gateways/{gateway_id}/service_accounts") {
		return "service_accounts", mux.Vars(r)["service_account_id"]
	}

	parts := strings.Split(pathTemplate, "/")
	for i := len(parts) - 1; i >= 0; i-- {
		if parts[i] == "{id}" {
			if i > 0 {
				resource = parts[i-1]
			}
			vars := mux.Vars(r)
			resourceID = vars["id"]
			return
		}
	}

	if len(parts) > 0 {
		resource = parts[len(parts)-1]
	}
	return resource, ""
}

func extractResourceInfoFromPath(path string) (resource string, resourceID string) {
	const prefix = "/api/hypershell/v1/"
	if !strings.HasPrefix(path, prefix) {
		return "", ""
	}

	remainder := strings.Trim(strings.TrimPrefix(path, prefix), "/")
	if remainder == "" {
		return "", ""
	}

	parts := strings.Split(remainder, "/")
	if strings.Contains(remainder, "gateways/") && strings.Contains(remainder, "/service_accounts") {
		for i, part := range parts {
			if part == "service_accounts" && i+1 < len(parts) {
				return "service_accounts", parts[i+1]
			}
		}
	}

	resource = parts[0]
	if len(parts) > 1 {
		resourceID = parts[1]
	}
	return resource, resourceID
}

func extractGatewayID(r *http.Request, resource string) string {
	if resource == "service_accounts" {
		if gatewayID := mux.Vars(r)["gateway_id"]; gatewayID != "" {
			return gatewayID
		}
		_, gatewayID := extractGatewayIDFromPath(r.URL.Path)
		return gatewayID
	}
	if resource == "gateways" {
		vars := mux.Vars(r)
		if gatewayID := vars["id"]; gatewayID != "" {
			return gatewayID
		}
		_, gatewayID := extractGatewayIDFromPath(r.URL.Path)
		return gatewayID
	}
	return ""
}

func extractGatewayIDFromPath(path string) (resource string, gatewayID string) {
	const prefix = "/api/hypershell/v1/"
	if !strings.HasPrefix(path, prefix) {
		return "", ""
	}

	remainder := strings.Trim(strings.TrimPrefix(path, prefix), "/")
	parts := strings.Split(remainder, "/")
	if len(parts) >= 2 && parts[0] == "gateways" {
		return "gateways", parts[1]
	}
	return "", ""
}

func isAuthorized(method string, resource string, resourceID string, gatewayID string, bindings []BindingSummary, jwtRoles []string) bool {
	if resource == "users" {
		return hasUsersInventoryAccess(bindings, jwtRoles)
	}

	if resource == "gateways" && method == http.MethodPost && resourceID == "" {
		return hasGatewayCreator(bindings)
	}

	if resource == "gateways" && gatewayID != "" {
		return isGatewayAuthorized(method, gatewayID, bindings)
	}

	if resource == "gateways" && gatewayID == "" {
		return hasPlatformAdmin(bindings) || len(bindings) > 0
	}

	if resource == "service_accounts" && gatewayID != "" {
		// Both gateway roles can create service accounts. The handler applies the
		// finer owner-all/viewer-own visibility and role-cap rules.
		for _, binding := range bindings {
			if binding.Scope == "gateway" && binding.GatewayID != nil && *binding.GatewayID == gatewayID &&
				(binding.RoleName == "gateway:owner" || binding.RoleName == "gateway:viewer") {
				return true
			}
		}
		return false
	}

	if resource == "role_bindings" {
		return len(bindings) > 0
	}

	return hasGatewayCreator(bindings)
}

func isGatewayAuthorized(method string, gatewayID string, bindings []BindingSummary) bool {
	if hasPlatformAdmin(bindings) && (method == http.MethodGet || method == http.MethodDelete) {
		return true
	}

	for _, b := range bindings {
		if b.Scope != "gateway" || b.GatewayID == nil || *b.GatewayID != gatewayID {
			continue
		}
		switch b.RoleName {
		case "gateway:owner":
			return true
		case "gateway:viewer":
			if method == http.MethodGet {
				return true
			}
		}
	}
	return false
}
