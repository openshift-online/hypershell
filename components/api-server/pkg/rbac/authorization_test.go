package rbac

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang-jwt/jwt/v4"
	"github.com/gorilla/mux"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/auth"
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

	if !isAuthorized(http.MethodPost, "gateways", "", "", bindings, nil, false) {
		t.Error("gateway:creator should be authorized for POST /gateways")
	}
}

func TestIsAuthorized_GatewayCreatorCanCheckPlacementAvailability(t *testing.T) {
	bindings := []BindingSummary{{RoleName: "gateway:creator", Scope: "global"}}
	if !isAuthorized(http.MethodPost, "gateway_placement_availability", "", "", bindings, nil, false) {
		t.Error("gateway:creator should be authorized for POST /gateways/placement-availability")
	}
}

func TestIsAuthorized_GatewayCreatorCannotGetGatewayWithoutBinding(t *testing.T) {
	bindings := []BindingSummary{
		{RoleName: "gateway:creator", Scope: "global"},
	}

	if isAuthorized(http.MethodGet, "gateways", "gw-1", "gw-1", bindings, nil, false) {
		t.Error("gateway:creator without per-gateway binding should not GET a specific gateway")
	}
}

func TestIsAuthorized_GatewayOwnerCanReadOwnGateway(t *testing.T) {
	gwID := "gw-aaa"
	bindings := []BindingSummary{
		{RoleName: "gateway:owner", Scope: "gateway", GatewayID: &gwID},
	}

	if !isAuthorized(http.MethodGet, "gateways", gwID, gwID, bindings, nil, false) {
		t.Error("gateway:owner should be authorized for GET on owned gateway")
	}
}

func TestIsAuthorized_GatewayOwnerCanDeleteOwnGateway(t *testing.T) {
	gwID := "gw-aaa"
	bindings := []BindingSummary{
		{RoleName: "gateway:owner", Scope: "gateway", GatewayID: &gwID},
	}

	if !isAuthorized(http.MethodDelete, "gateways", gwID, gwID, bindings, nil, false) {
		t.Error("gateway:owner should be authorized for DELETE on owned gateway")
	}
}

func TestIsAuthorized_GatewayOwnerCannotAccessOtherGateway(t *testing.T) {
	gwA := "gw-aaa"
	gwB := "gw-bbb"
	bindings := []BindingSummary{
		{RoleName: "gateway:owner", Scope: "gateway", GatewayID: &gwA},
	}

	if isAuthorized(http.MethodGet, "gateways", gwB, gwB, bindings, nil, false) {
		t.Error("gateway:owner must not access another gateway")
	}
}

func TestIsAuthorized_GatewayViewerCanReadOwnGateway(t *testing.T) {
	gwID := "gw-aaa"
	bindings := []BindingSummary{
		{RoleName: "gateway:viewer", Scope: "gateway", GatewayID: &gwID},
	}

	if !isAuthorized(http.MethodGet, "gateways", gwID, gwID, bindings, nil, false) {
		t.Error("gateway:viewer should be authorized for GET on own gateway")
	}
}

// Verifies: SEC-RBAC-05.02
func TestIsAuthorized_GatewayViewerCannotMutateGateway(t *testing.T) {
	gwID := "gw-aaa"
	bindings := []BindingSummary{
		{RoleName: "gateway:viewer", Scope: "gateway", GatewayID: &gwID},
	}

	if isAuthorized(http.MethodPatch, "gateways", gwID, gwID, bindings, nil, false) {
		t.Error("gateway:viewer must not PATCH a gateway")
	}
	if isAuthorized(http.MethodDelete, "gateways", gwID, gwID, bindings, nil, false) {
		t.Error("gateway:viewer must not DELETE a gateway")
	}
}

// TestCanDeleteGateway covers the Gateway.can_delete capability the REST API
// advertises to clients. It must match isGatewayAuthorized(DELETE, ...) exactly,
// including the case from the bug report: an owner who demotes their own access
// to the "user" tier (gateway:viewer) can no longer delete, while a platform
// admin retains the delete override.
// Verifies: GTW-ACC-01.03
func TestCanDeleteGateway(t *testing.T) {
	gwID := "gw-aaa"
	otherID := "gw-bbb"
	ptr := func(s string) *string { return &s }

	tests := []struct {
		name     string
		bindings []BindingSummary
		want     bool
	}{
		{
			name:     "owner can delete",
			bindings: []BindingSummary{{RoleName: "gateway:owner", Scope: "gateway", GatewayID: ptr(gwID)}},
			want:     true,
		},
		{
			name:     "demoted owner now viewer cannot delete",
			bindings: []BindingSummary{{RoleName: "gateway:viewer", Scope: "gateway", GatewayID: ptr(gwID)}},
			want:     false,
		},
		{
			name:     "granted admin cannot delete",
			bindings: []BindingSummary{{RoleName: "gateway:admin", Scope: "gateway", GatewayID: ptr(gwID)}},
			want:     false,
		},
		{
			name:     "platform admin can delete any gateway",
			bindings: []BindingSummary{{RoleName: "platform:admin", Scope: "global"}},
			want:     true,
		},
		{
			name:     "owner of a different gateway cannot delete",
			bindings: []BindingSummary{{RoleName: "gateway:owner", Scope: "gateway", GatewayID: ptr(otherID)}},
			want:     false,
		},
		{
			name:     "no bindings cannot delete",
			bindings: nil,
			want:     false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := CanDeleteGateway(tc.bindings, gwID); got != tc.want {
				t.Errorf("CanDeleteGateway() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestCanEditGateway covers the Gateway.can_edit capability (PATCH, e.g. rename).
// Owners and granted admins may edit; viewers ("plain users") may not, and a
// platform admin without a per-gateway binding may not (PATCH is not in the
// platform-admin override).
// Verifies: GTW-ACC-01.04
func TestCanEditGateway(t *testing.T) {
	gwID := "gw-aaa"
	otherID := "gw-bbb"
	ptr := func(s string) *string { return &s }

	tests := []struct {
		name     string
		bindings []BindingSummary
		want     bool
	}{
		{
			name:     "owner can edit",
			bindings: []BindingSummary{{RoleName: "gateway:owner", Scope: "gateway", GatewayID: ptr(gwID)}},
			want:     true,
		},
		{
			name:     "granted admin can edit",
			bindings: []BindingSummary{{RoleName: "gateway:admin", Scope: "gateway", GatewayID: ptr(gwID)}},
			want:     true,
		},
		{
			name:     "viewer (plain user) cannot edit",
			bindings: []BindingSummary{{RoleName: "gateway:viewer", Scope: "gateway", GatewayID: ptr(gwID)}},
			want:     false,
		},
		{
			name:     "platform admin without a binding cannot edit",
			bindings: []BindingSummary{{RoleName: "platform:admin", Scope: "global"}},
			want:     false,
		},
		{
			name:     "admin of a different gateway cannot edit",
			bindings: []BindingSummary{{RoleName: "gateway:admin", Scope: "gateway", GatewayID: ptr(otherID)}},
			want:     false,
		},
		{
			name:     "no bindings cannot edit",
			bindings: nil,
			want:     false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := CanEditGateway(tc.bindings, gwID); got != tc.want {
				t.Errorf("CanEditGateway() = %v, want %v", got, tc.want)
			}
		})
	}
}

// Verifies: SEC-RBAC-04.02
func TestIsAuthorized_NonCreatorCannotCreateGateways(t *testing.T) {
	gwID := "gw-aaa"
	bindings := []BindingSummary{
		{RoleName: "gateway:owner", Scope: "gateway", GatewayID: &gwID},
	}

	if isAuthorized(http.MethodPost, "gateways", "", "", bindings, nil, false) {
		t.Error("gateway:owner without gateway:creator must not POST /gateways")
	}
}

func TestIsAuthorized_RoleBindingsRequireAnyBinding(t *testing.T) {
	bindings := []BindingSummary{
		{RoleName: "gateway:viewer", Scope: "gateway", GatewayID: strPtr("gw-1")},
	}

	if !isAuthorized(http.MethodGet, "role_bindings", "", "", bindings, nil, false) {
		t.Error("any binding should authorize role_bindings access")
	}
}

func TestIsAuthorized_NoBindingsDenied(t *testing.T) {
	bindings := []BindingSummary{}

	if isAuthorized(http.MethodGet, "gateways", "gw-1", "gw-1", bindings, nil, false) {
		t.Error("empty bindings must not GET a specific gateway")
	}
	if isAuthorized(http.MethodPost, "gateways", "", "", bindings, nil, false) {
		t.Error("empty bindings must not POST /gateways")
	}
}

func TestIsAuthorized_NoBindingsCanListGateways(t *testing.T) {
	// A caller whose JWT carries no platform role (just hypershell-users)
	// syncs no RoleBindings. Collection GET must still be allowed: the list
	// handler returns 200 with an empty items array (rbac-enforcement Error
	// Response Opacity). Denying the list is 403 and the web console shows
	// "Gateways could not be loaded".
	bindings := []BindingSummary{}

	if !isAuthorized(http.MethodGet, "gateways", "", "", bindings, nil, false) {
		t.Error("empty bindings must be allowed to GET /gateways (empty list)")
	}
}

func TestIsAuthorized_GatewayOwnerCannotAccessManagedClusters(t *testing.T) {
	bindings := []BindingSummary{
		{RoleName: "gateway:owner", Scope: "gateway", GatewayID: strPtr("gw-1")},
	}

	if isAuthorized(http.MethodGet, "managed_clusters", "", "", bindings, nil, false) {
		t.Error("gateway:owner must not access managed_clusters without gateway:creator")
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

	if !isAuthorized(http.MethodGet, "gateways", "", "", bindings, nil, false) {
		t.Error("platform:admin should be authorized to list all gateways")
	}
}

// Verifies: SEC-RBAC-07.02
func TestIsAuthorized_PlatformAdminCanReadAnyGateway(t *testing.T) {
	gwID := "gw-xyz"
	bindings := []BindingSummary{
		{RoleName: "platform:admin", Scope: "global"},
	}

	if !isAuthorized(http.MethodGet, "gateways", gwID, gwID, bindings, nil, false) {
		t.Error("platform:admin should be authorized to read any gateway")
	}
}

// Verifies: SEC-RBAC-07.03
func TestIsAuthorized_PlatformAdminCanDeleteAnyGateway(t *testing.T) {
	gwID := "gw-xyz"
	bindings := []BindingSummary{
		{RoleName: "platform:admin", Scope: "global"},
	}

	if !isAuthorized(http.MethodDelete, "gateways", gwID, gwID, bindings, nil, false) {
		t.Error("platform:admin should be authorized to delete any gateway")
	}
}

// Verifies: SEC-RBAC-07.05
func TestIsAuthorized_PlatformAdminCannotModifyGateway(t *testing.T) {
	gwID := "gw-xyz"
	bindings := []BindingSummary{
		{RoleName: "platform:admin", Scope: "global"},
	}

	if isAuthorized(http.MethodPatch, "gateways", gwID, gwID, bindings, nil, false) {
		t.Error("platform:admin must not be able to PATCH gateways without gateway:owner")
	}
}

// Verifies: SEC-RBAC-07.06
func TestIsAuthorized_PlatformAdminCannotCreateGateways(t *testing.T) {
	bindings := []BindingSummary{
		{RoleName: "platform:admin", Scope: "global"},
	}

	if isAuthorized(http.MethodPost, "gateways", "", "", bindings, nil, false) {
		t.Error("platform:admin must not be able to create gateways without gateway:creator")
	}
}

func TestIsAuthorized_PlatformAdminWithGatewayCreatorCanCreate(t *testing.T) {
	bindings := []BindingSummary{
		{RoleName: "platform:admin", Scope: "global"},
		{RoleName: "gateway:creator", Scope: "global"},
	}

	if !isAuthorized(http.MethodPost, "gateways", "", "", bindings, nil, false) {
		t.Error("platform:admin + gateway:creator should be able to create gateways")
	}
}

func TestIsAuthorized_PlatformAdminWithOwnershipCanModify(t *testing.T) {
	gwID := "gw-owned"
	bindings := []BindingSummary{
		{RoleName: "platform:admin", Scope: "global"},
		{RoleName: "gateway:owner", Scope: "gateway", GatewayID: &gwID},
	}

	if !isAuthorized(http.MethodPatch, "gateways", gwID, gwID, bindings, nil, false) {
		t.Error("platform:admin + gateway:owner should be able to modify owned gateway")
	}
}

func TestIsAuthorized_PlatformAdminCanAccessRoleBindings(t *testing.T) {
	bindings := []BindingSummary{
		{RoleName: "platform:admin", Scope: "global"},
	}

	if !isAuthorized(http.MethodGet, "role_bindings", "", "", bindings, nil, false) {
		t.Error("platform:admin should be able to access role_bindings")
	}
}

func TestServiceAccountAuthorizationRequiresExactGatewayBinding(t *testing.T) {
	gatewayID := "gw-a"
	otherGatewayID := "gw-b"
	for _, role := range []string{"gateway:owner", "gateway:viewer"} {
		bindings := []BindingSummary{{RoleName: role, Scope: "gateway", GatewayID: &gatewayID}}
		for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodDelete} {
			if !isAuthorized(method, "service_accounts", "sa-1", gatewayID, bindings, nil, false) {
				t.Errorf("%s should authorize %s on the bound gateway", role, method)
			}
			if isAuthorized(method, "service_accounts", "sa-1", otherGatewayID, bindings, nil, false) {
				t.Errorf("%s must not authorize %s on another gateway", role, method)
			}
		}
	}
	platformOnly := []BindingSummary{{RoleName: "platform:admin", Scope: "global"}}
	if isAuthorized(http.MethodGet, "service_accounts", "sa-1", gatewayID, platformOnly, nil, false) {
		t.Error("platform:admin without an exact gateway binding must be denied")
	}
}

// Verifies: PLAT-USR-03.01, PLAT-USR-03.02, PLAT-USR-03.03
func TestIsAuthorized_UsersInventoryRequiresDashboardOperator(t *testing.T) {
	creatorOnly := []BindingSummary{{RoleName: "gateway:creator", Scope: "global"}}
	if isAuthorized(http.MethodGet, "users", "", "", creatorOnly, nil, false) {
		t.Error("gateway:creator without platform:admin must not list users")
	}

	platformAdmin := []BindingSummary{{RoleName: "platform:admin", Scope: "global"}}
	if !isAuthorized(http.MethodGet, "users", "", "", platformAdmin, nil, false) {
		t.Error("platform:admin should list users")
	}

	if isAuthorized(http.MethodGet, "users", "", "", nil, []string{HypershellAdminRole}, false) {
		t.Error("hypershell-admins JWT role alone must not list users")
	}

	if isAuthorized(http.MethodGet, "users", "user-1", "user-1", creatorOnly, nil, false) {
		t.Error("gateway:creator must not get user by id")
	}
}

// Verifies: PLAT-INV-03.01, PLAT-INV-03.02, PLAT-INV-03.03
func TestIsAuthorized_ManagedInventoryListRequiresDashboardOperatorOrCreator(t *testing.T) {
	ownerOnly := []BindingSummary{
		{RoleName: "gateway:owner", Scope: "gateway", GatewayID: strPtr("gw-1")},
	}
	if isAuthorized(http.MethodGet, "managed_clusters", "", "", ownerOnly, nil, false) {
		t.Error("gateway:owner must not list managed_clusters")
	}

	creatorOnly := []BindingSummary{{RoleName: "gateway:creator", Scope: "global"}}
	if !isAuthorized(http.MethodGet, "managed_clusters", "", "", creatorOnly, nil, false) {
		t.Error("gateway:creator should list managed_clusters")
	}

	if isAuthorized(http.MethodGet, "managed_clusters", "", "", nil, []string{HypershellAdminRole}, false) {
		t.Error("hypershell-admins JWT role alone must not list managed_clusters")
	}

	platformAdmin := []BindingSummary{{RoleName: "platform:admin", Scope: "global"}}
	if !isAuthorized(http.MethodGet, "managed_clusters", "", "", platformAdmin, nil, false) {
		t.Error("platform:admin should list managed_clusters")
	}

	// A read by id follows the list rule: the list already returns every record.
	if !isAuthorized(http.MethodGet, "managed_clusters", "cluster-1", "", platformAdmin, nil, false) {
		t.Error("platform:admin should get a managed cluster by id")
	}
	if isAuthorized(http.MethodGet, "managed_clusters", "cluster-1", "", ownerOnly, nil, false) {
		t.Error("gateway:owner must not get a managed cluster by id")
	}

	// Record writes are platform:admin only; gateway:creator is not enough.
	for _, method := range []string{http.MethodPost, http.MethodPatch, http.MethodDelete} {
		id := "cluster-1"
		if method == http.MethodPost {
			id = ""
		}
		if isAuthorized(method, "managed_clusters", id, "", creatorOnly, nil, false) {
			t.Errorf("gateway:creator must not %s managed_clusters", method)
		}
		if !isAuthorized(method, "managed_clusters", id, "", platformAdmin, nil, false) {
			t.Errorf("platform:admin should %s managed_clusters", method)
		}
	}
}

// Verifies: PLAT-INV-03.01
func TestAuthorizeApi_ManagedClustersListRequiresDashboardOperator(t *testing.T) {
	tests := []struct {
		name       string
		bindings   []BindingSummary
		wantReach  bool
		wantStatus int
	}{
		{
			name: "platform admin reaches the list handler",
			bindings: []BindingSummary{
				{RoleName: "platform:admin", Scope: "global"},
			},
			wantReach:  true,
			wantStatus: http.StatusOK,
		},
		{
			name:       "caller without dashboard-operator access is denied",
			bindings:   nil,
			wantReach:  false,
			wantStatus: http.StatusForbidden,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			middleware := NewRBACAuthzMiddleware(
				authorizationLookup{bindings: test.bindings},
				AuthzConfig{EnforceRBAC: true},
				nil,
				nil,
			)
			reached := false
			router := mux.NewRouter()
			router.Handle("/api/hypershell/v1/managed_clusters", middleware.AuthorizeApi(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				reached = true
				w.WriteHeader(http.StatusOK)
			}))).Methods(http.MethodGet)

			request := httptest.NewRequest(http.MethodGet, "/api/hypershell/v1/managed_clusters", nil)
			token := &jwt.Token{Claims: jwt.MapClaims{
				"preferred_username": "dashboard-operator",
			}}
			ctx := context.WithValue(request.Context(), ContextUserIDKey, "dashboard-operator")
			ctx = context.WithValue(ctx, auth.ContextAuthKey, token)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request.WithContext(ctx))

			if reached != test.wantReach {
				t.Fatalf("handler reached = %t, want %t", reached, test.wantReach)
			}
			if recorder.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d", recorder.Code, test.wantStatus)
			}
		})
	}
}

func TestExtractResourceInfoFromPath_Users(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/hypershell/v1/users/user-1", nil)
	resource, resourceID := extractResourceInfo(request)
	if resource != "users" || resourceID != "user-1" {
		t.Fatalf("resource = %q, id = %q", resource, resourceID)
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
	middleware := NewRBACAuthzMiddleware(authorizationLookup{}, AuthzConfig{EnforceRBAC: true}, nil, nil)
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
	middleware := NewRBACAuthzMiddleware(lookup, AuthzConfig{EnforceRBAC: true}, nil, nil)

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

func TestAuthorizeApiAllowsGatewayCreatorPlacementAvailability(t *testing.T) {
	lookup := authorizationLookup{bindings: []BindingSummary{{RoleName: "gateway:creator", Scope: "global"}}}
	middleware := NewRBACAuthzMiddleware(lookup, AuthzConfig{EnforceRBAC: true}, nil, nil)

	router := mux.NewRouter()
	reached := false
	router.Handle("/api/hypershell/v1/gateways/placement-availability", middleware.AuthorizeApi(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	}))).Methods(http.MethodPost)

	request := httptest.NewRequest(http.MethodPost, "/api/hypershell/v1/gateways/placement-availability", nil)
	token := &jwt.Token{Claims: jwt.MapClaims{"preferred_username": "creator-user"}}
	ctx := context.WithValue(request.Context(), auth.ContextAuthKey, token)
	ctx = context.WithValue(ctx, ContextUserIDKey, "user-id")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request.WithContext(ctx))

	if !reached || recorder.Code != http.StatusOK {
		t.Fatalf("placement availability reached=%v status=%d, want true/200", reached, recorder.Code)
	}
}

// Verifies: SEC-RBAC-10.01
func TestAuthorizeApiAllowsUserWithNoBindingsToListGateways(t *testing.T) {
	middleware := NewRBACAuthzMiddleware(authorizationLookup{}, AuthzConfig{EnforceRBAC: true}, nil, nil)

	reached := false
	router := mux.NewRouter()
	router.Handle("/api/hypershell/v1/gateways", middleware.AuthorizeApi(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	}))).Methods(http.MethodGet)

	request := httptest.NewRequest(http.MethodGet, "/api/hypershell/v1/gateways", nil)
	token := &jwt.Token{Claims: jwt.MapClaims{"preferred_username": "developer"}}
	ctx := context.WithValue(request.Context(), auth.ContextAuthKey, token)
	ctx = context.WithValue(ctx, ContextUserIDKey, "user-id")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request.WithContext(ctx))

	if !reached {
		t.Fatal("authenticated user with no RoleBindings did not reach GET /gateways")
	}
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
}

// Verifies: PLAT-USR-03.01
func TestAuthorizeApiDeniesGatewayCreatorOnUsersList(t *testing.T) {
	lookup := authorizationLookup{bindings: []BindingSummary{{RoleName: "gateway:creator", Scope: "global"}}}
	middleware := NewRBACAuthzMiddleware(lookup, AuthzConfig{EnforceRBAC: true}, nil, nil)

	router := mux.NewRouter()
	router.Handle("/api/hypershell/v1/users", middleware.AuthorizeApi(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("denied request reached the handler")
	}))).Methods(http.MethodGet)

	request := httptest.NewRequest(http.MethodGet, "/api/hypershell/v1/users", nil)
	token := &jwt.Token{Claims: jwt.MapClaims{"preferred_username": "creator-user"}}
	ctx := context.WithValue(request.Context(), auth.ContextAuthKey, token)
	ctx = context.WithValue(ctx, ContextUserIDKey, "user-id")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request.WithContext(ctx))

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", recorder.Code)
	}
}

// Verifies: SEC-RBAC-16.01
func TestIsAuthorized_RegistrarRoleAllowsRegistration(t *testing.T) {
	jwtRoles := []string{roleManagedClusterRegistrar}
	if !isAuthorized(http.MethodPost, "registration", "", "", nil, jwtRoles, false) {
		t.Error("managed-cluster-registrar JWT role must authorize POST /managed_clusters/registration")
	}
}

// Verifies: SEC-RBAC-16.02, PLAT-CLUS-04.01
func TestIsAuthorized_RegistrationDeniedWithoutRole(t *testing.T) {
	if isAuthorized(http.MethodPost, "registration", "", "", nil, nil, false) {
		t.Error("POST /managed_clusters/registration must be denied without managed-cluster-registrar role")
	}
	if isAuthorized(http.MethodPost, "registration", "", "", nil, []string{"gateway:creator"}, false) {
		t.Error("gateway:creator must not authorize managed_cluster registration")
	}
}

func TestIsAuthorized_RegistrationOnlyForPost(t *testing.T) {
	jwtRoles := []string{roleManagedClusterRegistrar}
	for _, method := range []string{http.MethodGet, http.MethodPatch, http.MethodDelete} {
		if isAuthorized(method, "registration", "", "", nil, jwtRoles, false) {
			t.Errorf("%s on registration resource must not be authorized via managed-cluster-registrar", method)
		}
	}
}

// Verifies: SEC-RBAC-16.01
func TestAuthorizeApi_RegistrationBypasesUserIDGate(t *testing.T) {
	lookup := authorizationLookup{bindings: nil}
	middleware := NewRBACAuthzMiddleware(lookup, AuthzConfig{EnforceRBAC: true}, nil, nil)

	router := mux.NewRouter()
	reached := false
	router.Handle("/api/hypershell/v1/managed_clusters/registration", middleware.AuthorizeApi(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached = true
		w.WriteHeader(http.StatusCreated)
	}))).Methods(http.MethodPost)

	request := httptest.NewRequest(http.MethodPost, "/api/hypershell/v1/managed_clusters/registration", nil)
	// Inject JWT token with managed-cluster-registrar role; intentionally do NOT
	// set ContextUserIDKey to simulate a transient user-provisioning failure.
	token := &jwt.Token{Claims: jwt.MapClaims{
		"preferred_username": "spoke-sa",
		"realm_access": map[string]interface{}{
			"roles": []interface{}{roleManagedClusterRegistrar},
		},
	}}
	ctx := context.WithValue(request.Context(), auth.ContextAuthKey, token)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request.WithContext(ctx))

	if !reached {
		t.Fatalf("registration request with correct role did not reach handler; status = %d", recorder.Code)
	}
}

// Verifies: SEC-RBAC-16.02, PLAT-CLUS-04.01
func TestAuthorizeApi_RegistrationDeniedWithoutRole(t *testing.T) {
	lookup := authorizationLookup{bindings: nil}
	middleware := NewRBACAuthzMiddleware(lookup, AuthzConfig{EnforceRBAC: true}, nil, nil)

	router := mux.NewRouter()
	router.Handle("/api/hypershell/v1/managed_clusters/registration", middleware.AuthorizeApi(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("request without role must not reach handler")
	}))).Methods(http.MethodPost)

	request := httptest.NewRequest(http.MethodPost, "/api/hypershell/v1/managed_clusters/registration", nil)
	token := &jwt.Token{Claims: jwt.MapClaims{
		"preferred_username": "spoke-sa",
		"realm_access": map[string]interface{}{
			"roles": []interface{}{"gateway:creator"},
		},
	}}
	ctx := context.WithValue(request.Context(), auth.ContextAuthKey, token)
	ctx = context.WithValue(ctx, ContextUserIDKey, "user-id")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request.WithContext(ctx))

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", recorder.Code)
	}
}

// Verifies: PLAT-USR-03.04
func TestAuthorizeApiConcealsDeniedUsersGet(t *testing.T) {
	lookup := authorizationLookup{bindings: []BindingSummary{{RoleName: "gateway:creator", Scope: "global"}}}
	middleware := NewRBACAuthzMiddleware(lookup, AuthzConfig{EnforceRBAC: true}, nil, nil)

	router := mux.NewRouter()
	router.Handle("/api/hypershell/v1/users/{id}", middleware.AuthorizeApi(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("denied request reached the handler")
	}))).Methods(http.MethodGet)

	request := httptest.NewRequest(http.MethodGet, "/api/hypershell/v1/users/user-1", nil)
	token := &jwt.Token{Claims: jwt.MapClaims{"preferred_username": "creator-user"}}
	ctx := context.WithValue(request.Context(), auth.ContextAuthKey, token)
	ctx = context.WithValue(ctx, ContextUserIDKey, "user-id")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request.WithContext(ctx))

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", recorder.Code)
	}
}

// Verifies: OSG-SEVT-05.01, OSG-SEVT-05.02
func TestAuthorizeApiSandboxEventsRouteHidesUnboundCallerAndAllowsViewer(t *testing.T) {
	gatewayID := "gw-events"
	for _, test := range []struct {
		name     string
		bindings []BindingSummary
		wantCode int
		wantHit  bool
	}{
		{name: "unbound caller receives not found", wantCode: http.StatusNotFound},
		{name: "gateway viewer reaches handler", bindings: []BindingSummary{{RoleName: "gateway:viewer", Scope: "gateway", GatewayID: &gatewayID}}, wantCode: http.StatusOK, wantHit: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			middleware := NewRBACAuthzMiddleware(authorizationLookup{bindings: test.bindings}, AuthzConfig{EnforceRBAC: true}, nil, nil)
			hit := false
			router := mux.NewRouter()
			router.Handle("/api/hypershell/v1/gateways/{gateway_id}/sandbox_events", middleware.AuthorizeApi(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				hit = true
				w.WriteHeader(http.StatusOK)
			}))).Methods(http.MethodGet)
			request := httptest.NewRequest(http.MethodGet, "/api/hypershell/v1/gateways/"+gatewayID+"/sandbox_events", nil)
			token := &jwt.Token{Claims: jwt.MapClaims{"preferred_username": "viewer"}}
			ctx := context.WithValue(request.Context(), auth.ContextAuthKey, token)
			ctx = context.WithValue(ctx, ContextUserIDKey, "viewer-id")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request.WithContext(ctx))
			if response.Code != test.wantCode || hit != test.wantHit {
				t.Fatalf("status=%d hit=%t, want %d/%t", response.Code, hit, test.wantCode, test.wantHit)
			}
		})
	}
}

// Verifies: OSG-SEVT-05.01
func TestSandboxEventsAuthorizationRequiresGatewayBinding(t *testing.T) {
	gatewayID := "gw-events"
	for _, role := range []string{"gateway:viewer", "gateway:admin", "gateway:owner"} {
		if !isAuthorized(http.MethodGet, "sandbox_events", "", gatewayID, []BindingSummary{{RoleName: role, Scope: "gateway", GatewayID: &gatewayID}}, nil, false) {
			t.Fatalf("%s must read sandbox events", role)
		}
	}
	if isAuthorized(http.MethodGet, "sandbox_events", "", gatewayID, []BindingSummary{{RoleName: "platform:admin", Scope: "global"}}, nil, false) {
		t.Fatal("platform admin without a gateway binding must not read sandbox events")
	}
	request := httptest.NewRequest(http.MethodGet, "/api/hypershell/v1/gateways/gw-events/sandbox_events", nil)
	resource, resourceID := extractResourceInfo(request)
	if resource != "sandbox_events" || resourceID != "" || extractGatewayID(request, resource) != "gw-events" {
		t.Fatalf("route extraction = %q, %q, %q", resource, resourceID, extractGatewayID(request, resource))
	}
}

// Verifies: SEC-RBAC-19
func TestExtractResourceInfoFromPath_ExtKinds(t *testing.T) {
	cases := []struct {
		path       string
		resource   string
		resourceID string
	}{
		{"/api/hypershell/ext/secret_sources", "secret_sources", ""},
		{"/api/hypershell/ext/secret_sources/ss-1", "secret_sources", "ss-1"},
		{"/api/hypershell/ext/agent_runtimes", "agent_runtimes", ""},
		{"/api/hypershell/ext/agent_runtimes/ar-1", "agent_runtimes", "ar-1"},
		{"/api/hypershell/ext/repositories/repo-1", "repositories", "repo-1"},
	}
	for _, c := range cases {
		resource, resourceID := extractResourceInfoFromPath(c.path)
		if resource != c.resource || resourceID != c.resourceID {
			t.Errorf("extractResourceInfoFromPath(%q) = (%q, %q), want (%q, %q)", c.path, resource, resourceID, c.resource, c.resourceID)
		}
	}
}

// Verifies: SEC-RBAC-19
func TestIsAuthorized_ExtKindsRequireCreatorOrPlatformAdmin(t *testing.T) {
	creator := []BindingSummary{{RoleName: "gateway:creator", Scope: "global"}}
	platformAdmin := []BindingSummary{{RoleName: "platform:admin", Scope: "global"}}
	viewerOnly := []BindingSummary{{RoleName: "gateway:viewer", Scope: "gateway", GatewayID: strPtr("gw-1")}}

	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodPatch, http.MethodDelete} {
		if !isAuthorized(method, "secret_sources", "", "", creator, nil, true) {
			t.Errorf("gateway:creator should be authorized for %s /ext/secret_sources", method)
		}
		if !isAuthorized(method, "repositories", "", "", platformAdmin, nil, true) {
			t.Errorf("platform:admin should be authorized for %s /ext/repositories", method)
		}
		if isAuthorized(method, "secret_sources", "ss-1", "", viewerOnly, nil, true) {
			t.Errorf("gateway:viewer must not %s /ext/secret_sources", method)
		}
		if isAuthorized(method, "secret_sources", "", "", nil, nil, true) {
			t.Errorf("unbound caller must not %s /ext/secret_sources", method)
		}
	}
}

// Verifies: SEC-RBAC-19.05
func TestIsAuthorized_UnknownExtResourceFailsClosed(t *testing.T) {
	creator := []BindingSummary{{RoleName: "gateway:creator", Scope: "global"}}
	if isAuthorized(http.MethodGet, "mystery_kind", "", "", creator, nil, true) {
		t.Error("an unknown ext resource must fail closed even for gateway:creator")
	}
	// The same resource name on the /v1 surface keeps the creator fallback.
	if !isAuthorized(http.MethodGet, "mystery_kind", "", "", creator, nil, false) {
		t.Error("unknown /v1 resources keep the gateway:creator fallback")
	}
}

// Verifies: SEC-RBAC-19
func TestAuthorizeApi_ExtRequestRequiresCreator(t *testing.T) {
	viewerLookup := authorizationLookup{bindings: []BindingSummary{{RoleName: "gateway:viewer", Scope: "gateway", GatewayID: strPtr("gw-1")}}}
	middleware := NewRBACAuthzMiddleware(viewerLookup, AuthzConfig{EnforceRBAC: true}, nil, nil)

	router := mux.NewRouter()
	router.Handle("/api/hypershell/ext/secret_sources", middleware.AuthorizeApi(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("denied request reached the handler")
	}))).Methods(http.MethodPost)

	request := httptest.NewRequest(http.MethodPost, "/api/hypershell/ext/secret_sources", nil)
	token := &jwt.Token{Claims: jwt.MapClaims{"preferred_username": "viewer-user"}}
	ctx := context.WithValue(request.Context(), auth.ContextAuthKey, token)
	ctx = context.WithValue(ctx, ContextUserIDKey, "user-id")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request.WithContext(ctx))

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", recorder.Code)
	}
}

// Verifies: SEC-RBAC-19.01, SEC-RBAC-19.03, SEC-RBAC-19.04
func TestCanManageGatewayResources(t *testing.T) {
	gw := "gw-1"
	cases := []struct {
		name     string
		bindings []BindingSummary
		want     bool
	}{
		{"owner", []BindingSummary{{RoleName: "gateway:owner", Scope: "gateway", GatewayID: strPtr(gw)}}, true},
		{"admin", []BindingSummary{{RoleName: "gateway:admin", Scope: "gateway", GatewayID: strPtr(gw)}}, true},
		{"viewer", []BindingSummary{{RoleName: "gateway:viewer", Scope: "gateway", GatewayID: strPtr(gw)}}, false},
		{"platform admin", []BindingSummary{{RoleName: "platform:admin", Scope: "global"}}, true},
		{"creator only", []BindingSummary{{RoleName: "gateway:creator", Scope: "global"}}, false},
		{"owner of another gateway", []BindingSummary{{RoleName: "gateway:owner", Scope: "gateway", GatewayID: strPtr("gw-2")}}, false},
		{"no bindings", nil, false},
	}
	for _, c := range cases {
		if got := CanManageGatewayResources(c.bindings, gw); got != c.want {
			t.Errorf("%s: CanManageGatewayResources = %v, want %v", c.name, got, c.want)
		}
	}
	if CanManageGatewayResources([]BindingSummary{{RoleName: "platform:admin", Scope: "global"}}, "") {
		t.Error("an empty gateway id must never be manageable")
	}
}

// --- Extension kind authorization gate: CI federation (SEC-RBAC-19/21/22) ---

func gwBinding(role, gw string) BindingSummary {
	return BindingSummary{RoleName: role, Scope: "gateway", GatewayID: &gw}
}

func globalBinding(role string) BindingSummary {
	return BindingSummary{RoleName: role, Scope: "global"}
}

// TestIsAuthorized_CIProvidersRequireCIFederationAdmin covers SEC-RBAC-21: the
// ci_providers ext kind is gated on the narrow ci-federation:admin role (or
// platform:admin) for every method, never on gateway:creator.
func TestIsAuthorized_CIProvidersRequireCIFederationAdmin(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodPatch, http.MethodDelete} {
		if !isAuthorized(method, "ci_providers", "", "", []BindingSummary{globalBinding("ci-federation:admin")}, nil, true) {
			t.Errorf("%s: ci-federation:admin must authorize ci_providers", method)
		}
		if !isAuthorized(method, "ci_providers", "", "", []BindingSummary{globalBinding("platform:admin")}, nil, true) {
			t.Errorf("%s: platform:admin must authorize ci_providers", method)
		}
		if isAuthorized(method, "ci_providers", "", "", []BindingSummary{globalBinding("gateway:creator")}, nil, true) {
			t.Errorf("%s: gateway:creator must NOT authorize ci_providers", method)
		}
		if isAuthorized(method, "ci_providers", "", "", []BindingSummary{gwBinding("gateway:owner", "gw-1")}, nil, true) {
			t.Errorf("%s: a gateway owner must NOT authorize ci_providers", method)
		}
	}
}

// TestIsAuthorized_CIFederationAdminGrantsNothingElse covers SEC-RBAC-21.03: the
// role authorizes ci_providers only - not gateway creation, not ci_trusts, not
// any other ext kind.
func TestIsAuthorized_CIFederationAdminGrantsNothingElse(t *testing.T) {
	b := []BindingSummary{globalBinding("ci-federation:admin")}
	if isAuthorized(http.MethodPost, "gateways", "", "", b, nil, false) {
		t.Error("ci-federation:admin must not create gateways")
	}
	if isAuthorized(http.MethodPost, "ci_trusts", "", "", b, nil, true) {
		t.Error("ci-federation:admin must not author ci_trusts")
	}
	if isAuthorized(http.MethodPost, "agent_runtimes", "", "", b, nil, true) {
		t.Error("ci-federation:admin must not author agent_runtimes")
	}
	if isAuthorized(http.MethodPost, "secret_sources", "", "", b, nil, true) {
		t.Error("ci-federation:admin must not author secret_sources")
	}
}

// TestIsAuthorized_CITrustsAdmitGatewayBindings covers the SEC-RBAC-22 middleware
// gate: a caller with any gateway binding (owner/admin/viewer), a gateway:creator,
// or platform:admin is admitted here; the per-gateway tenancy is enforced
// in-handler. A caller with no bindings at all is denied.
func TestIsAuthorized_CITrustsAdmitGatewayBindings(t *testing.T) {
	admit := [][]BindingSummary{
		{gwBinding("gateway:owner", "gw-1")},
		{gwBinding("gateway:admin", "gw-1")},
		{gwBinding("gateway:viewer", "gw-1")},
		{globalBinding("gateway:creator")},
		{globalBinding("platform:admin")},
	}
	for i, b := range admit {
		if !isAuthorized(http.MethodPost, "ci_trusts", "", "", b, nil, true) {
			t.Errorf("case %d: expected ci_trusts admit at middleware gate", i)
		}
	}
	if isAuthorized(http.MethodPost, "ci_trusts", "", "", nil, nil, true) {
		t.Error("a caller with no bindings must be denied ci_trusts")
	}
	if isAuthorized(http.MethodPost, "ci_trusts", "", "", []BindingSummary{globalBinding("ci-federation:admin")}, nil, true) {
		t.Error("ci-federation:admin alone must NOT be admitted to ci_trusts")
	}
}

// TestIsAuthorized_UnknownExtKindFailsClosed is the regression guard for the
// fail-closed omission that motivated SEC-RBAC-19: an ext resource not in
// extKinds is denied for everyone, including platform:admin.
func TestIsAuthorized_UnknownExtKindFailsClosed(t *testing.T) {
	if isAuthorized(http.MethodGet, "not_a_real_kind", "", "", []BindingSummary{globalBinding("platform:admin")}, nil, true) {
		t.Error("an unknown ext kind must fail closed even for platform:admin")
	}
}

// TestExtKinds_IncludesCIKinds guards the allow-list hygiene requirement that the
// two previously-omitted kinds are now present.
func TestExtKinds_IncludesCIKinds(t *testing.T) {
	if !extKinds["ci_providers"] {
		t.Error("ci_providers must be in the extKinds allow-list")
	}
	if !extKinds["ci_trusts"] {
		t.Error("ci_trusts must be in the extKinds allow-list")
	}
}

// TestCanReadGatewayResources covers the shared read-tenancy helper used by the
// ci_trusts read path.
func TestCanReadGatewayResources(t *testing.T) {
	gw := "gw-1"
	cases := []struct {
		name string
		b    []BindingSummary
		want bool
	}{
		{"owner", []BindingSummary{gwBinding("gateway:owner", gw)}, true},
		{"admin", []BindingSummary{gwBinding("gateway:admin", gw)}, true},
		{"viewer", []BindingSummary{gwBinding("gateway:viewer", gw)}, true},
		{"platform admin", []BindingSummary{globalBinding("platform:admin")}, true},
		{"creator only", []BindingSummary{globalBinding("gateway:creator")}, false},
		{"other gateway", []BindingSummary{gwBinding("gateway:owner", "gw-2")}, false},
		{"unbound", nil, false},
	}
	for _, c := range cases {
		if got := CanReadGatewayResources(c.b, gw); got != c.want {
			t.Errorf("%s: CanReadGatewayResources = %v, want %v", c.name, got, c.want)
		}
	}
	if CanReadGatewayResources([]BindingSummary{globalBinding("platform:admin")}, "") {
		t.Error("an empty gateway id must never be readable")
	}
}
