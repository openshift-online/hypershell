package rbac

import (
	"context"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gorilla/mux"

	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/auth"
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

// ServiceAccountsFromEnv reads RBAC_SERVICE_ACCOUNTS as a comma-separated allowlist.
func ServiceAccountsFromEnv() []string {
	serviceAccountEnv := os.Getenv("RBAC_SERVICE_ACCOUNTS")
	if serviceAccountEnv == "" {
		return nil
	}

	serviceAccounts := make([]string, 0)
	for _, entry := range strings.Split(serviceAccountEnv, ",") {
		if trimmed := strings.TrimSpace(entry); trimmed != "" {
			serviceAccounts = append(serviceAccounts, trimmed)
		}
	}
	return serviceAccounts
}

type rbacAuthzMiddleware struct {
	lookup           RoleBindingLookup
	config           AuthzConfig
	activityRecorder DailyActivityRecorder
	clusters         RegisteredClusterResolver
}

var _ auth.AuthorizationMiddleware = &rbacAuthzMiddleware{}

// NewRBACAuthzMiddleware builds the REST authorization middleware. clusters
// resolves the caller's JWT subject to a registered ManagedCluster so that a
// control plane is treated like an RBAC_SERVICE_ACCOUNTS entry: it is never
// recorded as a daily-active user. As for allowlisted accounts, it gets no REST
// role-binding bypass (control_plane_identity.go). clusters may be nil.
func NewRBACAuthzMiddleware(lookup RoleBindingLookup, config AuthzConfig, activityRecorder DailyActivityRecorder, clusters RegisteredClusterResolver) auth.AuthorizationMiddleware {
	return &rbacAuthzMiddleware{
		lookup:           lookup,
		config:           config,
		activityRecorder: activityRecorder,
		clusters:         clusters,
	}
}

func (m *rbacAuthzMiddleware) AuthorizeApi(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !m.config.EnforceRBAC {
			recordAuthorizedDailyActivity(r.Context(), r, m.config.ServiceAccounts, m.clusters, m.activityRecorder)
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

		// Registration is JWT-direct: managed-cluster-registrar is checked from the
		// JWT claim, never from DB role bindings. This runs before the userID gate so
		// a transient user-provisioning DB failure never produces a fatal non-retryable
		// 403 that causes the spoke to exit instead of retrying.
		if strings.HasSuffix(r.URL.Path, "/managed_clusters/registration") && r.Method == http.MethodPost {
			if hasManagedClusterRegistrar(extractJWTRoles(r)) {
				next.ServeHTTP(w, r)
				return
			}
			http.Error(w, "Forbidden", http.StatusForbidden)
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

		// Self-deregistration: a control plane may delete the ManagedCluster
		// record registered under its own JWT subject (bin/teardown-cluster in
		// hypershell-gitops deregisters a spoke this way). Any other record
		// needs platform:admin (isAuthorized).
		if resource == "managed_clusters" && r.Method == http.MethodDelete && resourceID != "" {
			own, err := m.isOwnManagedCluster(r.Context(), resourceID)
			if err != nil {
				http.Error(w, "Service Unavailable: managed cluster lookup failed", http.StatusServiceUnavailable)
				return
			}
			if own {
				next.ServeHTTP(w, r)
				return
			}
		}

		if !isAuthorized(r.Method, resource, resourceID, gatewayID, bindings, jwtRoles) {
			if resource == "service_accounts" || resource == "access" || (r.Method == http.MethodGet && resourceID != "") {
				http.Error(w, "Not Found", http.StatusNotFound)
			} else {
				http.Error(w, "Forbidden", http.StatusForbidden)
			}
			return
		}

		recordAuthorizedDailyActivity(r.Context(), r, m.config.ServiceAccounts, m.clusters, m.activityRecorder)
		next.ServeHTTP(w, r)
	})
}

// isOwnManagedCluster reports whether clusterID is the ManagedCluster the
// caller registered under its verified JWT subject.
func (m *rbacAuthzMiddleware) isOwnManagedCluster(ctx context.Context, clusterID string) (bool, error) {
	if m.clusters == nil {
		return false, nil
	}
	subject := subjectFromVerifiedToken(ctx)
	if subject == "" {
		return false, nil
	}
	registeredID, found, err := m.clusters.RegisteredClusterIDForSubject(ctx, subject)
	if err != nil {
		return false, err
	}
	return found && registeredID == clusterID, nil
}

// recordAuthorizedDailyActivity records the caller as a daily-active user
// unless it is a control-plane identity: an allowlisted account, or a
// registered managed cluster per clusters. A cluster lookup failure skips the
// record; it never fails the request.
func recordAuthorizedDailyActivity(ctx context.Context, r *http.Request, serviceAccounts []string, clusters RegisteredClusterResolver, activityRecorder DailyActivityRecorder) {
	if activityRecorder == nil || isExemptEndpoint(r) {
		return
	}

	payload, err := auth.GetAuthPayload(r)
	if err != nil || payload == nil || payload.Username == "" {
		return
	}
	if isServiceAccount(payload.Username, serviceAccounts) {
		return
	}

	userID := GetUserIDFromContext(ctx)
	if userID == "" {
		return
	}
	if registered, err := registeredClusterCaller(ctx, clusters, subjectFromVerifiedToken(ctx)); err != nil || registered {
		return
	}

	activityRecorder.RecordDailyActivity(ctx, userID, time.Now().UTC())
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

// roleManagedClusterRegistrar mirrors roles.RoleManagedClusterRegistrar; kept
// local to avoid an import cycle with the managedClusters plugin package.
const roleManagedClusterRegistrar = "managed-cluster-registrar"

func hasManagedClusterRegistrar(jwtRoles []string) bool {
	for _, role := range jwtRoles {
		if role == roleManagedClusterRegistrar {
			return true
		}
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

func hasUsersInventoryAccess(bindings []BindingSummary, _ []string) bool {
	return hasPlatformAdmin(bindings)
}

func hasDashboardInventoryAccess(bindings []BindingSummary, jwtRoles []string) bool {
	return hasUsersInventoryAccess(bindings, jwtRoles) || hasGatewayCreator(bindings)
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
	if strings.Contains(pathTemplate, "/gateways/{gateway_id}/access") {
		// Collection, item (/access/{user_id}), and directory (/access/directory)
		// all resolve to the "access" resource; user_id is empty for the
		// collection and directory sub-paths.
		return "access", mux.Vars(r)["user_id"]
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
	if strings.Contains(remainder, "gateways/") && strings.Contains(remainder, "/access") {
		for i, part := range parts {
			if part == "access" {
				if i+1 < len(parts) && parts[i+1] != "directory" {
					return "access", parts[i+1]
				}
				return "access", ""
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
	if resource == "service_accounts" || resource == "access" {
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
	// JWT-direct: managed-cluster-registrar is never DB-synced; check JWT claim only.
	if resource == "registration" && method == http.MethodPost {
		return hasManagedClusterRegistrar(jwtRoles)
	}

	if resource == "users" {
		return hasUsersInventoryAccess(bindings, jwtRoles)
	}

	if resource == "managed_clusters" {
		// Reads back gateway placement (the console's cluster picker) and the
		// dashboard inventory. Writes are operator functions: a placeholder
		// squats a name so the control plane that registers under it gets a
		// permanent 409, a rename breaks its control plane's re-registration,
		// and a delete detaches every gateway on the cluster. A control plane
		// deregistering itself is decided in AuthorizeApi before this.
		if method == http.MethodGet {
			return hasDashboardInventoryAccess(bindings, jwtRoles)
		}
		return hasPlatformAdmin(bindings)
	}

	if resource == "gateways" && method == http.MethodPost && resourceID == "" {
		return hasGatewayCreator(bindings)
	}

	if resource == "gateways" && gatewayID != "" {
		return isGatewayAuthorized(method, gatewayID, bindings)
	}

	if resource == "gateways" && gatewayID == "" {
		// Collection GET is allowed for any authenticated user. The list
		// handler filters to accessible IDs and returns 200 with an empty
		// items array when there are none. Requiring a RoleBinding here
		// 403s developers on OpenShift (RBAC_DEFAULT_ROLES empty) and the
		// web console shows "Gateways could not be loaded".
		if method == http.MethodGet {
			return true
		}
		return hasPlatformAdmin(bindings) || len(bindings) > 0
	}

	if resource == "service_accounts" && gatewayID != "" {
		// Any gateway role can reach the service-accounts API. The handler applies
		// the finer owner/admin-all vs viewer-own visibility and role-cap rules
		// (admins are capped like owners -- GAM-11).
		for _, binding := range bindings {
			if binding.Scope == "gateway" && binding.GatewayID != nil && *binding.GatewayID == gatewayID &&
				(binding.RoleName == "gateway:owner" || binding.RoleName == "gateway:admin" || binding.RoleName == "gateway:viewer") {
				return true
			}
		}
		return false
	}

	if resource == "access" && gatewayID != "" {
		return isAccessFacadeAuthorized(gatewayID, bindings)
	}

	if resource == "role_bindings" {
		return len(bindings) > 0
	}

	return hasGatewayCreator(bindings)
}

// CanDeleteGateway reports whether a caller holding bindings may DELETE the
// gateway. It is the single source of truth shared by the DELETE authorization
// middleware (isGatewayAuthorized) and the Gateway.can_delete capability the REST
// API advertises, so a client's delete affordance never diverges from the check
// the server actually enforces.
func CanDeleteGateway(bindings []BindingSummary, gatewayID string) bool {
	return isGatewayAuthorized(http.MethodDelete, gatewayID, bindings)
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
		case "gateway:admin":
			// Granted administrator: read and update, but never delete the
			// gateway (GAM-01). Delete requires gateway:owner.
			if method != http.MethodDelete {
				return true
			}
		case "gateway:viewer":
			if method == http.MethodGet {
				return true
			}
		}
	}
	return false
}

// isAccessFacadeAuthorized is the coarse gate for the gateway access facade
// (/gateways/{gateway_id}/access...). Any caller with a binding on the gateway
// (owner, admin, or viewer) or platform:admin passes; everyone else gets 404
// (existence is not disclosed, per rbac-enforcement.spec.md). The fine-grained
// rules -- owner/admin-only management, owner-tier gating, and last-owner
// protection -- are enforced in the access facade service (403/409), which can
// return descriptive messages the middleware cannot.
func isAccessFacadeAuthorized(gatewayID string, bindings []BindingSummary) bool {
	if hasPlatformAdmin(bindings) {
		return true
	}
	for _, b := range bindings {
		if b.Scope == "gateway" && b.GatewayID != nil && *b.GatewayID == gatewayID {
			return true
		}
	}
	return false
}
