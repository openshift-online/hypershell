package rbac

import (
	"testing"
)

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
