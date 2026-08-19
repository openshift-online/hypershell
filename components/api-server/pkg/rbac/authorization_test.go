package rbac

import (
	"net/http"
	"testing"
)

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
