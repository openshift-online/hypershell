package gateways

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/hypershell/components/api-server/pkg/rbac"
	svcerrors "github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
)

func pinGateBindings(names ...string) []rbac.BindingSummary {
	out := make([]rbac.BindingSummary, 0, len(names))
	for _, name := range names {
		out = append(out, rbac.BindingSummary{RoleName: name, Scope: "global"})
	}
	return out
}

func pinGateUser() context.Context {
	return context.WithValue(context.Background(), rbac.ContextUserIDKey, "user-1")
}

// TestPinnedClusterAuthorize is the cluster-targeting gate on pinning a gateway by
// cluster_id (SEC-RBAC-34): gateway:cluster-targeting or platform:admin passes, a
// gateway:creator or anonymous caller is denied 403 with the pin reason, a write that
// pins nothing needs no entitlement, a control-plane write bypasses the gate, and an
// import is denied.
// Verifies: SEC-RBAC-34.01, SEC-RBAC-34.02, SEC-RBAC-34.03, SEC-RBAC-34.04
func TestPinnedClusterAuthorize(t *testing.T) {
	tests := []struct {
		name        string
		lookup      rbac.RoleBindingLookup
		enforceRBAC bool
		ctx         context.Context
		pinned      bool
		wantErr     string
	}{
		{name: "gateway:cluster-targeting may pin", lookup: fakeBindingLookup{bindings: pinGateBindings("gateway:creator", "gateway:cluster-targeting")}, enforceRBAC: true, ctx: pinGateUser(), pinned: true},
		{name: "platform:admin may pin", lookup: fakeBindingLookup{bindings: pinGateBindings("platform:admin")}, enforceRBAC: true, ctx: pinGateUser(), pinned: true},
		{name: "gateway:creator may not pin", lookup: fakeBindingLookup{bindings: pinGateBindings("gateway:creator")}, enforceRBAC: true, ctx: pinGateUser(), pinned: true, wantErr: "forbidden"},
		{name: "gateway:creator placement create is unaffected", lookup: fakeBindingLookup{bindings: pinGateBindings("gateway:creator")}, enforceRBAC: true, ctx: pinGateUser(), pinned: false},
		{name: "control-plane write bypasses the gate", lookup: fakeBindingLookup{bindings: pinGateBindings("gateway:creator")}, enforceRBAC: true, ctx: rbac.WithGatewayWriterKind(pinGateUser(), rbac.GatewayWriterControlPlane), pinned: true},
		{name: "import write is denied", lookup: fakeBindingLookup{bindings: pinGateBindings("platform:admin")}, enforceRBAC: true, ctx: rbac.WithGatewayWriterKind(pinGateUser(), rbac.GatewayWriterImport), pinned: true, wantErr: "import"},
		{name: "RBAC off allows any caller", lookup: fakeBindingLookup{bindings: pinGateBindings("gateway:creator")}, enforceRBAC: false, ctx: pinGateUser(), pinned: true},
		{name: "anonymous caller fails closed", lookup: fakeBindingLookup{bindings: pinGateBindings("platform:admin")}, enforceRBAC: true, ctx: context.Background(), pinned: true, wantErr: "forbidden"},
		{name: "unwired lookup fails closed", lookup: nil, enforceRBAC: true, ctx: pinGateUser(), pinned: true, wantErr: "forbidden"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svcErr := newClusterTargetingAuthorizer(tc.lookup, tc.enforceRBAC).authorizePin(tc.ctx, tc.pinned)
			if tc.wantErr == "" {
				if svcErr != nil {
					t.Fatalf("expected authorization, got %v", svcErr)
				}
				return
			}
			if svcErr == nil || !svcErr.IsForbidden() {
				t.Fatalf("expected Forbidden, got %v", svcErr)
			}
			if tc.wantErr == "import" {
				if svcErr.Reason != importClusterTargetingForbiddenReason {
					t.Fatalf("reason = %q, want the import reason", svcErr.Reason)
				}
				return
			}
			if svcErr.Reason != pinnedClusterForbiddenReason {
				t.Fatalf("reason = %q, want %q", svcErr.Reason, pinnedClusterForbiddenReason)
			}
			if !strings.Contains(svcErr.Reason, "gateway:cluster-targeting") || !strings.Contains(svcErr.Reason, "placement selector") {
				t.Fatalf("reason %q must name gateway:cluster-targeting and the placement selector", svcErr.Reason)
			}
		})
	}
}

// recordingClusterLookup records whether the cluster reference was resolved, so a
// test can prove the gate runs BEFORE resolution. It reports every cluster as
// unregistered, so a caller that passes the gate gets the 400 / InvalidArgument.
type recordingClusterLookup struct{ called bool }

func (r *recordingClusterLookup) lookup(context.Context, string) (bool, *svcerrors.ServiceError) {
	r.called = true
	return false, nil
}

// TestRESTCreatePinnedClusterRequiresAdmin drives the REST create handler: a
// non-admin pin is 403 before the cluster is resolved; an admin pin passes the gate
// and reaches cluster validation.
// Verifies: SEC-RBAC-34.01, SEC-RBAC-34.02
func TestRESTCreatePinnedClusterRequiresAdmin(t *testing.T) {
	for _, tc := range []struct {
		name       string
		roles      []string
		wantStatus int
		wantLookup bool
	}{
		{name: "non-admin", roles: []string{"gateway:creator"}, wantStatus: http.StatusForbidden, wantLookup: false},
		{name: "admin", roles: []string{"platform:admin"}, wantStatus: http.StatusBadRequest, wantLookup: true},
		{name: "cluster-targeting", roles: []string{"gateway:creator", "gateway:cluster-targeting"}, wantStatus: http.StatusBadRequest, wantLookup: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lookup := fakeBindingLookup{bindings: pinGateBindings(tc.roles...)}
			clusters := &recordingClusterLookup{}
			h := gatewayHandler{
				clusters:          clusters.lookup,
				enforceRBAC:       true,
				dedicatedRoute:    newDedicatedRouteAuthorizer(lookup, true),
				operatorWorkspace: newOperatorWorkspaceAuthorizer(lookup, true),
				clusterTargeting:  newClusterTargetingAuthorizer(lookup, true),
			}
			body := bytes.NewBufferString(`{"name":"pinned","cluster_id":"cluster-1"}`)
			req := httptest.NewRequest(http.MethodPost, "/api/hypershell/v1/gateways", body).WithContext(pinGateUser())
			rec := httptest.NewRecorder()
			h.Create(rec, req)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body: %s)", rec.Code, tc.wantStatus, rec.Body.String())
			}
			if clusters.called != tc.wantLookup {
				t.Fatalf("cluster lookup called = %v, want %v", clusters.called, tc.wantLookup)
			}
			if tc.wantStatus == http.StatusForbidden && !strings.Contains(rec.Body.String(), "gateway:cluster-targeting") {
				t.Fatalf("403 body must name gateway:cluster-targeting: %s", rec.Body.String())
			}
		})
	}
}

// TestGRPCCreatePinnedClusterRequiresAdmin drives the gRPC create handler: a
// non-admin pin is PERMISSION_DENIED before the cluster is resolved; an admin pin
// passes the gate and reaches cluster validation.
// Verifies: SEC-RBAC-34.01, SEC-RBAC-34.02
func TestGRPCCreatePinnedClusterRequiresAdmin(t *testing.T) {
	for _, tc := range []struct {
		name       string
		roles      []string
		wantCode   codes.Code
		wantLookup bool
	}{
		{name: "non-admin", roles: []string{"gateway:creator"}, wantCode: codes.PermissionDenied, wantLookup: false},
		{name: "admin", roles: []string{"platform:admin"}, wantCode: codes.InvalidArgument, wantLookup: true},
		{name: "cluster-targeting", roles: []string{"gateway:creator", "gateway:cluster-targeting"}, wantCode: codes.InvalidArgument, wantLookup: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lookup := fakeBindingLookup{bindings: pinGateBindings(tc.roles...)}
			clusters := &recordingClusterLookup{}
			h := &gatewayGRPCHandler{
				clusters:          clusters.lookup,
				dedicatedRoute:    newDedicatedRouteAuthorizer(lookup, true),
				operatorWorkspace: newOperatorWorkspaceAuthorizer(lookup, true),
				clusterTargeting:  newClusterTargetingAuthorizer(lookup, true),
			}
			_, err := h.CreateGateway(pinGateUser(), &pb.CreateGatewayRequest{Name: "pinned", ClusterId: "cluster-1"})
			if status.Code(err) != tc.wantCode {
				t.Fatalf("code = %v, want %v (%v)", status.Code(err), tc.wantCode, err)
			}
			if clusters.called != tc.wantLookup {
				t.Fatalf("cluster lookup called = %v, want %v", clusters.called, tc.wantLookup)
			}
		})
	}
}

// TestPlacementLabelAuthorize is the gate on requiring a placement/ label
// (SEC-RBAC-35): a gateway:creator is denied with the placement/ reason, the role or
// platform:admin passes, excluding the label needs no role, a control-plane write
// bypasses the gate, and an import is denied.
// Verifies: SEC-RBAC-35.01, SEC-RBAC-35.02, SEC-RBAC-35.03
func TestPlacementLabelAuthorize(t *testing.T) {
	required := openapi.GatewayPlacementIntent{Selector: []string{"provider/ibm", "placement/release-check"}}
	excluded := openapi.GatewayPlacementIntent{Selector: []string{"provider/ibm"}, ExcludedSelector: []string{"placement/release-check"}}
	for _, tc := range []struct {
		name    string
		roles   []string
		ctx     context.Context
		intent  openapi.GatewayPlacementIntent
		wantErr string
	}{
		{name: "gateway:creator may not require placement/", roles: []string{"gateway:creator"}, ctx: pinGateUser(), intent: required, wantErr: placementLabelForbiddenReason},
		{name: "gateway:cluster-targeting may require placement/", roles: []string{"gateway:creator", "gateway:cluster-targeting"}, ctx: pinGateUser(), intent: required},
		{name: "platform:admin may require placement/", roles: []string{"platform:admin"}, ctx: pinGateUser(), intent: required},
		{name: "gateway:creator may exclude placement/", roles: []string{"gateway:creator"}, ctx: pinGateUser(), intent: excluded},
		{name: "control-plane write bypasses the gate", roles: []string{"gateway:creator"}, ctx: rbac.WithGatewayWriterKind(pinGateUser(), rbac.GatewayWriterControlPlane), intent: required},
		{name: "import is denied", roles: []string{"platform:admin"}, ctx: rbac.WithGatewayWriterKind(pinGateUser(), rbac.GatewayWriterImport), intent: required, wantErr: importClusterTargetingForbiddenReason},
	} {
		t.Run(tc.name, func(t *testing.T) {
			authorizer := newClusterTargetingAuthorizer(fakeBindingLookup{bindings: pinGateBindings(tc.roles...)}, true)
			svcErr := authorizer.authorizePlacement(tc.ctx, tc.intent)
			if tc.wantErr == "" {
				if svcErr != nil {
					t.Fatalf("expected authorization, got %v", svcErr)
				}
				return
			}
			if svcErr == nil || !svcErr.IsForbidden() || svcErr.Reason != tc.wantErr {
				t.Fatalf("expected Forbidden %q, got %v", tc.wantErr, svcErr)
			}
		})
	}
}
