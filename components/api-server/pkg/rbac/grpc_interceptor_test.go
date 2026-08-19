package rbac

import (
	"context"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/openshift-online/rh-trex-ai/pkg/auth"
)

// fakeLookup and fakeProvisioner stand in for the DB-backed role-binding lookup
// and JWT user provisioner so the interceptor can be exercised without a live
// database or auth stack.
type fakeLookup struct {
	bindings []BindingSummary
}

func (f fakeLookup) FindBindingsByUserID(_ context.Context, _ string) ([]BindingSummary, error) {
	return f.bindings, nil
}

type fakeProvisioner struct{ userID string }

func (f fakeProvisioner) UpsertFromJWT(_ context.Context, _ *auth.Payload) (string, error) {
	return f.userID, nil
}

func TestGrpcMethodIsRead(t *testing.T) {
	tests := []struct {
		method string
		want   bool
	}{
		{"/hypershell.v1.FleetService/GetFleet", true},
		{"/hypershell.v1.FleetService/ListFleets", true},
		{"/hypershell.v1.FleetService/WatchFleets", true},
		{"/hypershell.v1.FleetService/CreateFleet", false},
		{"/hypershell.v1.FleetService/UpdateFleet", false},
		{"/hypershell.v1.FleetService/DeleteFleet", false},
		{"/hypershell.v1.GatewayService/GetGateway", true},
		{"/hypershell.v1.GatewayService/CreateGateway", false},
	}

	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			if got := isGRPCReadMethod(tt.method); got != tt.want {
				t.Errorf("isGRPCReadMethod(%q) = %v, want %v", tt.method, got, tt.want)
			}
		})
	}
}

func TestIsGRPCAuthorized_CreatorCanDoAnything(t *testing.T) {
	bindings := []BindingSummary{
		{RoleName: "gateway:creator", Scope: "global"},
	}

	if !isGRPCAuthorized("/hypershell.v1.GatewayService/CreateGateway", bindings) {
		t.Error("gateway:creator should be authorized for Create")
	}
	if !isGRPCAuthorized("/hypershell.v1.GatewayService/GetGateway", bindings) {
		t.Error("gateway:creator should be authorized for Get")
	}
	if !isGRPCAuthorized("/hypershell.v1.GatewayService/DeleteGateway", bindings) {
		t.Error("gateway:creator should be authorized for Delete")
	}
}

func TestIsGRPCAuthorized_OwnerCanDoAnything(t *testing.T) {
	bindings := []BindingSummary{
		{RoleName: "gateway:owner", Scope: "gateway", GatewayID: strPtr("gw-1")},
	}

	if !isGRPCAuthorized("/hypershell.v1.GatewayService/GetGateway", bindings) {
		t.Error("gateway:owner should be authorized for Get")
	}
	if !isGRPCAuthorized("/hypershell.v1.GatewayService/CreateGateway", bindings) {
		t.Error("gateway:owner should be authorized for Create")
	}
	if !isGRPCAuthorized("/hypershell.v1.GatewayService/DeleteGateway", bindings) {
		t.Error("gateway:owner should be authorized for Delete")
	}
}

func TestIsGRPCAuthorized_ViewerCanOnlyRead(t *testing.T) {
	bindings := []BindingSummary{
		{RoleName: "gateway:viewer", Scope: "gateway", GatewayID: strPtr("gw-1")},
	}

	if !isGRPCAuthorized("/hypershell.v1.GatewayService/GetGateway", bindings) {
		t.Error("gateway:viewer should be authorized for Get")
	}
	if !isGRPCAuthorized("/hypershell.v1.GatewayService/ListGateways", bindings) {
		t.Error("gateway:viewer should be authorized for List")
	}
	if isGRPCAuthorized("/hypershell.v1.GatewayService/CreateGateway", bindings) {
		t.Error("gateway:viewer must not be authorized for Create")
	}
	if isGRPCAuthorized("/hypershell.v1.GatewayService/DeleteGateway", bindings) {
		t.Error("gateway:viewer must not be authorized for Delete")
	}
}

func TestIsGRPCAuthorized_NoBindingsDenied(t *testing.T) {
	bindings := []BindingSummary{}

	if isGRPCAuthorized("/hypershell.v1.FleetService/GetFleet", bindings) {
		t.Error("empty bindings must be denied")
	}
}

func TestIsServiceAccount_MatchesConfiguredAccount(t *testing.T) {
	accounts := []string{"service-account-hypershell-control-plane"}
	if !isServiceAccount("service-account-hypershell-control-plane", accounts) {
		t.Error("configured service account should match")
	}
}

func TestIsServiceAccount_RejectsUnknownUsername(t *testing.T) {
	accounts := []string{"service-account-hypershell-control-plane"}
	if isServiceAccount("some-human-user", accounts) {
		t.Error("unknown username must not match service accounts")
	}
}

func TestIsServiceAccount_EmptyUsernameNeverMatches(t *testing.T) {
	accounts := []string{"service-account-hypershell-control-plane"}
	if isServiceAccount("", accounts) {
		t.Error("empty username must not match")
	}
}

func TestIsServiceAccount_EmptyListNeverMatches(t *testing.T) {
	if isServiceAccount("service-account-hypershell-control-plane", nil) {
		t.Error("nil service accounts list must not match")
	}
}

func TestIsServiceAccountOnlyMethod(t *testing.T) {
	tests := []struct {
		method string
		want   bool
	}{
		{"/hypershell.v1.GatewayService/AdjustActiveSandboxCount", true},
		{"/hypershell.v1.GatewayService/SetActiveSandboxCount", true},
		{"/hypershell.v1.GatewayService/UpdateGateway", false},
		{"/hypershell.v1.GatewayService/CreateGateway", false},
		{"/hypershell.v1.GatewayService/GetGateway", false},
		{"malformed", false},
	}
	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			if got := isServiceAccountOnlyMethod(tt.method); got != tt.want {
				t.Errorf("isServiceAccountOnlyMethod(%q) = %v, want %v", tt.method, got, tt.want)
			}
		})
	}
}

// TestUnaryInterceptor_SandboxCountRestrictedToServiceAccount locks in the guard
// that keeps the control-plane-only sandbox-count writes out of reach of ordinary
// role bindings: with an allowlist configured, only an allowlisted service account
// may call them; an owner/creator (who otherwise passes isGRPCAuthorized for every
// non-read method) is denied. With no allowlist, the guard falls back to the
// standard role check so single-tenant/dev deployments keep working.
func TestUnaryInterceptor_SandboxCountRestrictedToServiceAccount(t *testing.T) {
	const adjust = "/hypershell.v1.GatewayService/AdjustActiveSandboxCount"
	const create = "/hypershell.v1.GatewayService/CreateGateway"
	const sa = "service-account-hypershell-control-plane"

	// An owner binding authorizes every non-read method under isGRPCAuthorized.
	ownerLookup := fakeLookup{bindings: []BindingSummary{{RoleName: "gateway:owner", Scope: "gateway", GatewayID: strPtr("gw-1")}}}
	prov := fakeProvisioner{userID: "user-1"}

	tests := []struct {
		name            string
		username        string
		method          string
		serviceAccounts []string
		wantAllowed     bool
	}{
		{"owner denied Adjust when allowlist set", "human-owner", adjust, []string{sa}, false},
		{"service account allowed Adjust", sa, adjust, []string{sa}, true},
		{"owner allowed Adjust when no allowlist", "human-owner", adjust, nil, true},
		{"owner still allowed non-restricted method", "human-owner", create, []string{sa}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			interceptor := RBACUnaryInterceptor(ownerLookup, prov, nil, AuthzConfig{
				EnforceRBAC:     true,
				ServiceAccounts: tt.serviceAccounts,
			})
			ctx := auth.SetUsernameContext(context.Background(), tt.username)
			called := false
			handler := func(context.Context, interface{}) (interface{}, error) {
				called = true
				return "ok", nil
			}
			_, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{FullMethod: tt.method}, handler)

			if tt.wantAllowed {
				if err != nil {
					t.Fatalf("expected call allowed, got error %v", err)
				}
				if !called {
					t.Error("expected handler to be invoked")
				}
				return
			}
			if called {
				t.Error("handler must not be invoked when denied")
			}
			if status.Code(err) != codes.PermissionDenied {
				t.Errorf("got %v, want PermissionDenied", err)
			}
		})
	}
}

func TestIsGRPCDeleteMethod(t *testing.T) {
	tests := []struct {
		method string
		want   bool
	}{
		{"/hypershell.v1.FleetService/DeleteFleet", true},
		{"/hypershell.v1.GatewayService/DeleteGateway", true},
		{"/hypershell.v1.FleetService/GetFleet", false},
		{"/hypershell.v1.FleetService/CreateFleet", false},
	}

	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			if got := isGRPCDeleteMethod(tt.method); got != tt.want {
				t.Errorf("isGRPCDeleteMethod(%q) = %v, want %v", tt.method, got, tt.want)
			}
		})
	}
}

// Platform Admin gRPC tests
func TestIsGRPCAuthorized_PlatformAdminCanRead(t *testing.T) {
	bindings := []BindingSummary{
		{RoleName: "platform:admin", Scope: "global"},
	}

	if !isGRPCAuthorized("/hypershell.v1.GatewayService/GetGateway", bindings) {
		t.Error("platform:admin should be authorized for Get")
	}
	if !isGRPCAuthorized("/hypershell.v1.GatewayService/ListGateways", bindings) {
		t.Error("platform:admin should be authorized for List")
	}
	if !isGRPCAuthorized("/hypershell.v1.GatewayService/WatchGateways", bindings) {
		t.Error("platform:admin should be authorized for Watch")
	}
}

func TestIsGRPCAuthorized_PlatformAdminCanDelete(t *testing.T) {
	bindings := []BindingSummary{
		{RoleName: "platform:admin", Scope: "global"},
	}

	if !isGRPCAuthorized("/hypershell.v1.GatewayService/DeleteGateway", bindings) {
		t.Error("platform:admin should be authorized for Delete")
	}
}

func TestIsGRPCAuthorized_PlatformAdminCannotCreate(t *testing.T) {
	bindings := []BindingSummary{
		{RoleName: "platform:admin", Scope: "global"},
	}

	if isGRPCAuthorized("/hypershell.v1.GatewayService/CreateGateway", bindings) {
		t.Error("platform:admin must not be authorized for Create without gateway:creator")
	}
}

func TestIsGRPCAuthorized_PlatformAdminCannotUpdate(t *testing.T) {
	bindings := []BindingSummary{
		{RoleName: "platform:admin", Scope: "global"},
	}

	if isGRPCAuthorized("/hypershell.v1.GatewayService/UpdateGateway", bindings) {
		t.Error("platform:admin must not be authorized for Update without gateway:owner")
	}
	if isGRPCAuthorized("/hypershell.v1.GatewayService/PatchGateway", bindings) {
		t.Error("platform:admin must not be authorized for Patch without gateway:owner")
	}
}

func TestIsGRPCAuthorized_PlatformAdminWithCreatorCanCreate(t *testing.T) {
	bindings := []BindingSummary{
		{RoleName: "platform:admin", Scope: "global"},
		{RoleName: "gateway:creator", Scope: "global"},
	}

	if !isGRPCAuthorized("/hypershell.v1.GatewayService/CreateGateway", bindings) {
		t.Error("platform:admin + gateway:creator should be authorized for Create")
	}
}
