package rbac

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang-jwt/jwt/v4"
	"github.com/gorilla/mux"
	"github.com/openshift-online/rh-trex-ai/pkg/auth"
)

type authorizationLookup struct {
	bindings []BindingSummary
}

func (l authorizationLookup) FindBindingsByUserID(context.Context, string) ([]BindingSummary, error) {
	return l.bindings, nil
}

func TestIsExemptEndpoint_RolesListIsExempt(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "/api/hypershell/v1/roles", nil)
	if !isExemptEndpoint(req) {
		t.Error("GET /roles should be auth-exempt")
	}
}

func TestIsExemptEndpoint_RolesGetByIDIsExempt(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "/api/hypershell/v1/roles/some-id", nil)
	if !isExemptEndpoint(req) {
		t.Error("GET /roles/{id} should be auth-exempt")
	}
}

func TestIsExemptEndpoint_RoleBindingsNotExempt(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "/api/hypershell/v1/role_bindings", nil)
	if isExemptEndpoint(req) {
		t.Error("GET /role_bindings should not be auth-exempt")
	}
}

func TestIsAuthorized_GatewayCreatorCanCreateGateways(t *testing.T) {
	bindings := []BindingSummary{
		{RoleName: "gateway:creator", Scope: "global"},
	}

	if !isAuthorized(http.MethodPost, "gateways", "", "", bindings) {
		t.Error("gateway:creator should be authorized for POST /gateways")
	}
}

func TestIsAuthorized_GatewayCreatorCannotGetGatewayWithoutBinding(t *testing.T) {
	bindings := []BindingSummary{
		{RoleName: "gateway:creator", Scope: "global"},
	}

	if isAuthorized(http.MethodGet, "gateways", "gw-1", "gw-1", bindings) {
		t.Error("gateway:creator without per-gateway binding should not GET a specific gateway")
	}
}

func TestIsAuthorized_GatewayOwnerCanReadOwnGateway(t *testing.T) {
	gwID := "gw-aaa"
	bindings := []BindingSummary{
		{RoleName: "gateway:owner", Scope: "gateway", GatewayID: &gwID},
	}

	if !isAuthorized(http.MethodGet, "gateways", gwID, gwID, bindings) {
		t.Error("gateway:owner should be authorized for GET on owned gateway")
	}
}

func TestIsAuthorized_GatewayOwnerCanDeleteOwnGateway(t *testing.T) {
	gwID := "gw-aaa"
	bindings := []BindingSummary{
		{RoleName: "gateway:owner", Scope: "gateway", GatewayID: &gwID},
	}

	if !isAuthorized(http.MethodDelete, "gateways", gwID, gwID, bindings) {
		t.Error("gateway:owner should be authorized for DELETE on owned gateway")
	}
}

func TestIsAuthorized_GatewayOwnerCannotAccessOtherGateway(t *testing.T) {
	gwA := "gw-aaa"
	gwB := "gw-bbb"
	bindings := []BindingSummary{
		{RoleName: "gateway:owner", Scope: "gateway", GatewayID: &gwA},
	}

	if isAuthorized(http.MethodGet, "gateways", gwB, gwB, bindings) {
		t.Error("gateway:owner must not access another gateway")
	}
}

func TestIsAuthorized_GatewayViewerCanReadOwnGateway(t *testing.T) {
	gwID := "gw-aaa"
	bindings := []BindingSummary{
		{RoleName: "gateway:viewer", Scope: "gateway", GatewayID: &gwID},
	}

	if !isAuthorized(http.MethodGet, "gateways", gwID, gwID, bindings) {
		t.Error("gateway:viewer should be authorized for GET on own gateway")
	}
}

func TestIsAuthorized_GatewayViewerCannotMutateGateway(t *testing.T) {
	gwID := "gw-aaa"
	bindings := []BindingSummary{
		{RoleName: "gateway:viewer", Scope: "gateway", GatewayID: &gwID},
	}

	if isAuthorized(http.MethodPatch, "gateways", gwID, gwID, bindings) {
		t.Error("gateway:viewer must not PATCH a gateway")
	}
	if isAuthorized(http.MethodDelete, "gateways", gwID, gwID, bindings) {
		t.Error("gateway:viewer must not DELETE a gateway")
	}
}

func TestIsAuthorized_NonCreatorCannotCreateGateways(t *testing.T) {
	gwID := "gw-aaa"
	bindings := []BindingSummary{
		{RoleName: "gateway:owner", Scope: "gateway", GatewayID: &gwID},
	}

	if isAuthorized(http.MethodPost, "gateways", "", "", bindings) {
		t.Error("gateway:owner without gateway:creator must not POST /gateways")
	}
}

func TestIsAuthorized_RoleBindingsRequireAnyBinding(t *testing.T) {
	bindings := []BindingSummary{
		{RoleName: "gateway:viewer", Scope: "gateway", GatewayID: strPtr("gw-1")},
	}

	if !isAuthorized(http.MethodGet, "role_bindings", "", "", bindings) {
		t.Error("any binding should authorize role_bindings access")
	}
}

func TestIsAuthorized_NoBindingsDenied(t *testing.T) {
	bindings := []BindingSummary{}

	if isAuthorized(http.MethodGet, "gateways", "", "", bindings) {
		t.Error("empty bindings must be denied")
	}
}

func TestIsAuthorized_GatewayViewerCannotAccessFleets(t *testing.T) {
	bindings := []BindingSummary{
		{RoleName: "gateway:viewer", Scope: "gateway", GatewayID: strPtr("gw-1")},
	}

	if isAuthorized(http.MethodGet, "fleets", "", "", bindings) {
		t.Error("gateway:viewer must not access fleets")
	}
}

func TestIsAuthorized_GatewayOwnerCannotAccessManagedClusters(t *testing.T) {
	bindings := []BindingSummary{
		{RoleName: "gateway:owner", Scope: "gateway", GatewayID: strPtr("gw-1")},
	}

	if isAuthorized(http.MethodGet, "managed_clusters", "", "", bindings) {
		t.Error("gateway:owner must not access managed_clusters without gateway:creator")
	}
}

func TestIsAuthorized_GatewayCreatorCanAccessFleets(t *testing.T) {
	bindings := []BindingSummary{
		{RoleName: "gateway:creator", Scope: "global"},
	}

	if !isAuthorized(http.MethodGet, "fleets", "", "", bindings) {
		t.Error("gateway:creator should access fleets")
	}
}

func strPtr(s string) *string {
	return &s
}

// Platform Admin tests
func TestIsAuthorized_PlatformAdminCanListAllGateways(t *testing.T) {
	bindings := []BindingSummary{
		{RoleName: "platform:admin", Scope: "global"},
	}

	if !isAuthorized(http.MethodGet, "gateways", "", "", bindings) {
		t.Error("platform:admin should be authorized to list all gateways")
	}
}

func TestIsAuthorized_PlatformAdminCanReadAnyGateway(t *testing.T) {
	gwID := "gw-xyz"
	bindings := []BindingSummary{
		{RoleName: "platform:admin", Scope: "global"},
	}

	if !isAuthorized(http.MethodGet, "gateways", gwID, gwID, bindings) {
		t.Error("platform:admin should be authorized to read any gateway")
	}
}

func TestIsAuthorized_PlatformAdminCanDeleteAnyGateway(t *testing.T) {
	gwID := "gw-xyz"
	bindings := []BindingSummary{
		{RoleName: "platform:admin", Scope: "global"},
	}

	if !isAuthorized(http.MethodDelete, "gateways", gwID, gwID, bindings) {
		t.Error("platform:admin should be authorized to delete any gateway")
	}
}

func TestIsAuthorized_PlatformAdminCannotModifyGateway(t *testing.T) {
	gwID := "gw-xyz"
	bindings := []BindingSummary{
		{RoleName: "platform:admin", Scope: "global"},
	}

	if isAuthorized(http.MethodPatch, "gateways", gwID, gwID, bindings) {
		t.Error("platform:admin must not be able to PATCH gateways without gateway:owner")
	}
}

func TestIsAuthorized_PlatformAdminCannotCreateGateways(t *testing.T) {
	bindings := []BindingSummary{
		{RoleName: "platform:admin", Scope: "global"},
	}

	if isAuthorized(http.MethodPost, "gateways", "", "", bindings) {
		t.Error("platform:admin must not be able to create gateways without gateway:creator")
	}
}

func TestIsAuthorized_PlatformAdminWithGatewayCreatorCanCreate(t *testing.T) {
	bindings := []BindingSummary{
		{RoleName: "platform:admin", Scope: "global"},
		{RoleName: "gateway:creator", Scope: "global"},
	}

	if !isAuthorized(http.MethodPost, "gateways", "", "", bindings) {
		t.Error("platform:admin + gateway:creator should be able to create gateways")
	}
}

func TestIsAuthorized_PlatformAdminWithOwnershipCanModify(t *testing.T) {
	gwID := "gw-owned"
	bindings := []BindingSummary{
		{RoleName: "platform:admin", Scope: "global"},
		{RoleName: "gateway:owner", Scope: "gateway", GatewayID: &gwID},
	}

	if !isAuthorized(http.MethodPatch, "gateways", gwID, gwID, bindings) {
		t.Error("platform:admin + gateway:owner should be able to modify owned gateway")
	}
}

func TestIsAuthorized_PlatformAdminCanAccessRoleBindings(t *testing.T) {
	bindings := []BindingSummary{
		{RoleName: "platform:admin", Scope: "global"},
	}

	if !isAuthorized(http.MethodGet, "role_bindings", "", "", bindings) {
		t.Error("platform:admin should be able to access role_bindings")
	}
}

func TestServiceAccountAuthorizationRequiresExactGatewayBinding(t *testing.T) {
	gatewayID := "gw-a"
	otherGatewayID := "gw-b"
	for _, role := range []string{"gateway:owner", "gateway:viewer"} {
		bindings := []BindingSummary{{RoleName: role, Scope: "gateway", GatewayID: &gatewayID}}
		for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodDelete} {
			if !isAuthorized(method, "service_accounts", "sa-1", gatewayID, bindings) {
				t.Errorf("%s should authorize %s on the bound gateway", role, method)
			}
			if isAuthorized(method, "service_accounts", "sa-1", otherGatewayID, bindings) {
				t.Errorf("%s must not authorize %s on another gateway", role, method)
			}
		}
	}
	platformOnly := []BindingSummary{{RoleName: "platform:admin", Scope: "global"}}
	if isAuthorized(http.MethodGet, "service_accounts", "sa-1", gatewayID, platformOnly) {
		t.Error("platform:admin without an exact gateway binding must be denied")
	}
}

func TestExtractResourceInfoRecognizesNestedServiceAccountRoutes(t *testing.T) {
	router := mux.NewRouter()
	router.HandleFunc("/api/hypershell/v1/gateways/{gateway_id}/service_accounts/{service_account_id}/revoke", func(w http.ResponseWriter, r *http.Request) {
		resource, resourceID := extractResourceInfo(r)
		if resource != "service_accounts" || resourceID != "sa-1" {
			t.Errorf("resource = %q, id = %q", resource, resourceID)
		}
		if gatewayID := extractGatewayID(r, resource); gatewayID != "gw-1" {
			t.Errorf("gateway id = %q", gatewayID)
		}
	}).Methods(http.MethodPost)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/hypershell/v1/gateways/gw-1/service_accounts/sa-1/revoke", nil)
	router.ServeHTTP(recorder, request)
}

func TestServiceAccountAuthorizationConcealsDeniedMutations(t *testing.T) {
	middleware := NewRBACAuthzMiddleware(authorizationLookup{}, AuthzConfig{EnforceRBAC: true})
	for _, test := range []struct {
		method string
		path   string
		route  string
	}{
		{method: http.MethodPost, path: "/api/hypershell/v1/gateways/gw-1/service_accounts", route: "/api/hypershell/v1/gateways/{gateway_id}/service_accounts"},
		{method: http.MethodPost, path: "/api/hypershell/v1/gateways/gw-1/service_accounts/sa-1/revoke", route: "/api/hypershell/v1/gateways/{gateway_id}/service_accounts/{service_account_id}/revoke"},
		{method: http.MethodDelete, path: "/api/hypershell/v1/gateways/gw-1/service_accounts/sa-1", route: "/api/hypershell/v1/gateways/{gateway_id}/service_accounts/{service_account_id}"},
	} {
		t.Run(test.method+" "+test.path, func(t *testing.T) {
			router := mux.NewRouter()
			router.Handle(test.route, middleware.AuthorizeApi(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				t.Fatal("denied request reached the handler")
			}))).Methods(test.method)
			request := httptest.NewRequest(test.method, test.path, nil)
			// Inject identity the way the real HTTP flow does: the framework's
			// global jwtHandler validates the token and places it in the request
			// context under ContextAuthKey. AuthorizeApi reads it via GetAuthPayload,
			// NOT via GetUsernameFromContext (which is only set later, on the child
			// subrouter, and therefore empty at this parent-router middleware).
			token := &jwt.Token{Claims: jwt.MapClaims{"preferred_username": "unbound-user"}}
			ctx := context.WithValue(request.Context(), auth.ContextAuthKey, token)
			ctx = context.WithValue(ctx, ContextUserIDKey, "user-id")
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request.WithContext(ctx))
			if recorder.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want 404", recorder.Code)
			}
		})
	}
}

// TestAuthorizeApiAllowsBoundUserFromJWTContext is the regression guard for the
// middleware-ordering bug: AuthorizeApi is attached on the PARENT apiV1Router,
// which runs before the child-subrouter AuthenticateAccountJWT that sets the
// username context. Deriving identity via GetUsernameFromContext therefore yielded
// an empty username and rejected every authenticated request with 401. Identity
// must come from the validated JWT token in context (GetAuthPayload) instead. This
// test drives the full middleware with a bound gateway:creator and asserts the
// request reaches the handler.
func TestAuthorizeApiAllowsBoundUserFromJWTContext(t *testing.T) {
	lookup := authorizationLookup{bindings: []BindingSummary{{RoleName: "gateway:creator", Scope: "global"}}}
	middleware := NewRBACAuthzMiddleware(lookup, AuthzConfig{EnforceRBAC: true})

	router := mux.NewRouter()
	reached := false
	router.Handle("/api/hypershell/v1/gateways", middleware.AuthorizeApi(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	}))).Methods(http.MethodPost)

	request := httptest.NewRequest(http.MethodPost, "/api/hypershell/v1/gateways", nil)
	token := &jwt.Token{Claims: jwt.MapClaims{"preferred_username": "creator-user"}}
	ctx := context.WithValue(request.Context(), auth.ContextAuthKey, token)
	ctx = context.WithValue(ctx, ContextUserIDKey, "user-id")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request.WithContext(ctx))

	if !reached {
		t.Fatal("bound gateway:creator request did not reach the handler")
	}
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
}
