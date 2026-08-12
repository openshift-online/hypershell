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

		username := auth.GetUsernameFromContext(r.Context())
		if username == "" {
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

		if !isAuthorized(r.Method, resource, resourceID, gatewayID, bindings) {
			if r.Method == http.MethodGet && resourceID != "" {
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

func extractResourceInfo(r *http.Request) (resource string, resourceID string) {
	route := mux.CurrentRoute(r)
	if route == nil {
		return "", ""
	}

	pathTemplate, err := route.GetPathTemplate()
	if err != nil {
		return "", ""
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

func extractGatewayID(r *http.Request, resource string) string {
	if resource == "gateways" {
		vars := mux.Vars(r)
		return vars["id"]
	}
	return ""
}

func isAuthorized(method string, resource string, resourceID string, gatewayID string, bindings []BindingSummary) bool {
	if resource == "gateways" && method == http.MethodPost && resourceID == "" {
		return hasGatewayCreator(bindings)
	}

	if resource == "gateways" && gatewayID != "" {
		return isGatewayAuthorized(method, gatewayID, bindings)
	}

	if resource == "gateways" && gatewayID == "" {
		return len(bindings) > 0
	}

	if resource == "role_bindings" {
		return len(bindings) > 0
	}

	return hasGatewayCreator(bindings)
}

func isGatewayAuthorized(method string, gatewayID string, bindings []BindingSummary) bool {
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
