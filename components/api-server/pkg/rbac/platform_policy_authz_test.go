package rbac

import (
	"context"
	"net/http"
	"testing"

	"google.golang.org/grpc/codes"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
)

func global(role string) BindingSummary { return BindingSummary{RoleName: role, Scope: "global"} }

func onGateway(role, gatewayID string) BindingSummary {
	id := gatewayID
	return BindingSummary{RoleName: role, Scope: "gateway", GatewayID: &id}
}

// Verifies: SEC-RBAC-30.01, SEC-RBAC-30.02, SEC-RBAC-30.04
func TestPlatformPolicyExtGate(t *testing.T) {
	policyAdmin := []BindingSummary{global(rolePlatformPolicyAdmin)}
	platformAdmin := []BindingSummary{global("platform:admin")}
	fleetReader := []BindingSummary{global(roleFleetReader)}
	owner := []BindingSummary{onGateway("gateway:owner", "gw-1"), global("gateway:creator")}

	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodPatch, http.MethodDelete} {
		for name, bindings := range map[string][]BindingSummary{"policy admin": policyAdmin, "platform admin": platformAdmin} {
			if !isAuthorized(method, "platform_gateway_policies", "", "", bindings, nil, true) {
				t.Errorf("%s %s must be authorized for platform_gateway_policies", name, method)
			}
			if method == http.MethodGet || method == http.MethodPatch {
				if !isAuthorized(method, "platform_policy_settings", "", "", bindings, nil, true) {
					t.Errorf("%s %s must be authorized for the toggle", name, method)
				}
			}
		}
		if isAuthorized(method, "platform_gateway_policies", "", "", owner, nil, true) {
			t.Errorf("gateway owner/creator %s must be denied on platform_gateway_policies", method)
		}
		if isAuthorized(method, "platform_policy_settings", "", "", owner, nil, true) {
			t.Errorf("gateway owner/creator %s must be denied on the toggle", method)
		}
		wantReader := method == http.MethodGet
		if got := isAuthorized(method, "platform_gateway_policies", "", "", fleetReader, nil, true); got != wantReader {
			t.Errorf("fleet:reader %s on platform_gateway_policies = %v, want %v", method, got, wantReader)
		}
	}
	// The role grants nothing else (SEC-RBAC-30.04).
	if isAuthorized(http.MethodPost, "gateways", "", "", policyAdmin, nil, false) {
		t.Error("platform-policy:admin must not create gateways")
	}
	if isAuthorized(http.MethodPost, "sandbox_templates", "", "", policyAdmin, nil, true) {
		t.Error("platform-policy:admin must not write other ext kinds")
	}
	if isAuthorized(http.MethodGet, "gateways", "gw-1", "gw-1", policyAdmin, nil, false) {
		t.Error("platform-policy:admin must not read gateways")
	}
}

// Verifies: SEC-RBAC-31.01, SEC-RBAC-31.02, SEC-RBAC-31.03, PLAT-PGP-19
func TestEffectivePoliciesGate(t *testing.T) {
	for _, role := range []string{"gateway:owner", "gateway:admin", "gateway:viewer"} {
		bindings := []BindingSummary{onGateway(role, "gw-g")}
		if !isAuthorized(http.MethodGet, "effective_policies", "", "gw-g", bindings, nil, false) {
			t.Errorf("%s of G must read G's effective policies", role)
		}
		if isAuthorized(http.MethodGet, "effective_policies", "", "gw-h", bindings, nil, false) {
			t.Errorf("%s of G must not read H's effective policies", role)
		}
		if CanSeePolicyTargets(bindings) {
			t.Errorf("%s must not see policy targets", role)
		}
	}
	for _, role := range []string{rolePlatformPolicyAdmin, roleFleetReader, "platform:admin"} {
		bindings := []BindingSummary{global(role)}
		if !isAuthorized(http.MethodGet, "effective_policies", "", "gw-h", bindings, nil, false) {
			t.Errorf("%s must read any gateway's effective policies", role)
		}
		if !CanSeePolicyTargets(bindings) {
			t.Errorf("%s must see policy targets", role)
		}
	}
	req, _ := http.NewRequest(http.MethodGet, "/api/hypershell/v1/gateways/gw-g/effective_policies", nil)
	if resource, _ := extractResourceInfoFromPath(req.URL.Path); resource != "effective_policies" {
		t.Errorf("path resource = %q, want effective_policies", resource)
	}
}

// Verifies: SEC-RBAC-32.01, SEC-RBAC-32.02, CTRL-CORE-22.02, CTRL-CORE-22.03
func TestControlPlanePolicyGRPC(t *testing.T) {
	ctx := context.Background()
	gws := fakeGateways{}
	for _, m := range []string{
		"/hypershell.v1.PlatformGatewayPolicyService/ListPlatformGatewayPolicies",
		"/hypershell.v1.PlatformGatewayPolicyService/WatchPlatformGatewayPolicies",
		"/hypershell.v1.PlatformPolicySettingsService/WatchPlatformPolicySettings",
		"/hypershell.v1.PlatformPolicySettingsService/GetPlatformPolicySettings",
	} {
		if err := authorizeControlPlaneGRPC(ctx, "cluster-spoke3", m, nil, gws, nil); err != nil {
			t.Errorf("control plane read %s must be allowed: %v", m, err)
		}
	}
	report := "/hypershell.v1.PolicyAuditFindingService/ReportPolicyAuditFindings"
	own := &pb.ReportPolicyAuditFindingsRequest{GatewayId: "gw-own", Findings: []*pb.PolicyAuditFinding{{Id: "e1", GatewayId: "gw-own"}}}
	if err := authorizeControlPlaneGRPC(ctx, "cluster-spoke3", report, own, gws, nil); err != nil {
		t.Errorf("findings for an own-cluster gateway must be accepted: %v", err)
	}
	foreign := &pb.ReportPolicyAuditFindingsRequest{GatewayId: "gw-foreign"}
	if code := codeOf(authorizeControlPlaneGRPC(ctx, "cluster-spoke3", report, foreign, gws, nil)); code != codes.PermissionDenied {
		t.Errorf("findings for a foreign gateway = %v, want PermissionDenied", code)
	}
	smuggled := &pb.ReportPolicyAuditFindingsRequest{GatewayId: "gw-own", Findings: []*pb.PolicyAuditFinding{{Id: "e1", GatewayId: "gw-foreign"}}}
	if code := codeOf(authorizeControlPlaneGRPC(ctx, "cluster-spoke3", report, smuggled, gws, nil)); code != codes.PermissionDenied {
		t.Errorf("a finding naming another gateway = %v, want PermissionDenied", code)
	}
	status := "/hypershell.v1.PlatformGatewayPolicyService/UpdatePlatformGatewayPolicyStatus"
	okStatus := &pb.UpdatePlatformGatewayPolicyStatusRequest{Id: "p", GatewayStatuses: []*pb.PlatformGatewayPolicyGatewayStatus{{GatewayId: "gw-own"}}}
	if err := authorizeControlPlaneGRPC(ctx, "cluster-spoke3", status, okStatus, gws, nil); err != nil {
		t.Errorf("own-cluster status write-back must be allowed: %v", err)
	}
	badStatus := &pb.UpdatePlatformGatewayPolicyStatusRequest{Id: "p", RemovedGatewayIds: []string{"gw-foreign"}}
	if code := codeOf(authorizeControlPlaneGRPC(ctx, "cluster-spoke3", status, badStatus, gws, nil)); code != codes.PermissionDenied {
		t.Errorf("foreign status write-back = %v, want PermissionDenied", code)
	}

	// Users never reach the findings endpoint or the status write-back; reads
	// need the policy role.
	admin := []BindingSummary{global("platform:admin")}
	if code := codeOf(authorizeUserGRPC(report, own, admin)); code != codes.PermissionDenied {
		t.Errorf("user ReportPolicyAuditFindings = %v, want PermissionDenied", code)
	}
	if code := codeOf(authorizeUserGRPC(status, okStatus, admin)); code != codes.PermissionDenied {
		t.Errorf("user status write-back = %v, want PermissionDenied", code)
	}
	list := "/hypershell.v1.PlatformGatewayPolicyService/ListPlatformGatewayPolicies"
	if err := authorizeUserGRPC(list, nil, []BindingSummary{global(rolePlatformPolicyAdmin)}); err != nil {
		t.Errorf("policy admin gRPC read must be allowed: %v", err)
	}
	if code := codeOf(authorizeUserGRPC(list, nil, []BindingSummary{onGateway("gateway:owner", "gw-own")})); code != codes.PermissionDenied {
		t.Errorf("gateway owner gRPC policy read = %v, want PermissionDenied", code)
	}
}
