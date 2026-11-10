package gateways

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/openshift-online/hypershell/components/api-server/pkg/rbac"
	"github.com/openshift-online/hypershell/components/api-server/plugins/roles"
)

// TestApplyGatewayWorkspaceModeCreateRequest pins the service-funnel half of the
// workspace_mode operator opt-in: operator is the only caller-settable value
// (shared/managed from a user are rejected naming the field, OSG-OPWS-01.04), an
// import create that sets the field at all is rejected (OSG-OPWS-01.05), and a
// privileged control-plane write keeps full access.
func TestApplyGatewayWorkspaceModeCreateRequest(t *testing.T) {
	tests := []struct {
		name        string
		kind        rbac.GatewayWriterKind
		mode        *string
		wantErr     string // "" accepted, "forbidden" otherwise
		wantImport  bool   // the forbidden reason must be the import-specific one
		wantFieldNm bool   // the forbidden reason must name workspace_mode (non-operator value)
	}{
		{name: "user operator is accepted", kind: rbac.GatewayWriterUser, mode: strPtr("operator")},
		{name: "user shared is forbidden naming the field", kind: rbac.GatewayWriterUser, mode: strPtr("shared"), wantErr: "forbidden", wantFieldNm: true},
		{name: "user managed is forbidden naming the field", kind: rbac.GatewayWriterUser, mode: strPtr("managed"), wantErr: "forbidden", wantFieldNm: true},
		{name: "user unset is accepted (default applied later)", kind: rbac.GatewayWriterUser, mode: nil},
		{name: "user empty is accepted (default applied later)", kind: rbac.GatewayWriterUser, mode: strPtr("  ")},
		{name: "import operator is forbidden with the import reason", kind: rbac.GatewayWriterImport, mode: strPtr("operator"), wantErr: "forbidden", wantImport: true},
		{name: "import shared is forbidden with the import reason", kind: rbac.GatewayWriterImport, mode: strPtr("shared"), wantErr: "forbidden", wantImport: true},
		{name: "control-plane shared is accepted (privileged bypass)", kind: rbac.GatewayWriterControlPlane, mode: strPtr("shared")},
		{name: "control-plane operator is accepted", kind: rbac.GatewayWriterControlPlane, mode: strPtr("operator")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gw := &Gateway{Name: "gw", WorkspaceMode: tc.mode}
			err := applyGatewayWorkspaceModeCreateRequest(tc.kind, gw)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("expected acceptance, got %v", err)
				}
				return
			}
			if err == nil || !err.IsForbidden() {
				t.Fatalf("expected a Forbidden rejection, got %v", err)
			}
			if tc.wantImport && err.Reason != operatorWorkspaceImportForbiddenReason {
				t.Fatalf("reason = %q, want the import reason", err.Reason)
			}
			if tc.wantFieldNm && !strings.Contains(err.Reason, "workspace_mode") {
				t.Fatalf("reason %q must name workspace_mode", err.Reason)
			}
		})
	}
}

// TestOperatorWorkspaceAuthorizeCreate is the RBAC gate at the create boundaries:
// a caller holding only gateway:creator gets the 403 naming the entitlement role,
// while a gateway:operator-workspace-creator or platform:admin caller passes. A
// create that does not select operator mode needs no entitlement, and RBAC-off,
// privileged, and import writes are decided elsewhere.
func TestOperatorWorkspaceAuthorizeCreate(t *testing.T) {
	user := func() context.Context {
		return context.WithValue(context.Background(), rbac.ContextUserIDKey, "user-1")
	}
	bindingsOf := func(names ...string) []rbac.BindingSummary {
		out := make([]rbac.BindingSummary, 0, len(names))
		for _, name := range names {
			out = append(out, rbac.BindingSummary{RoleName: name, Scope: "global"})
		}
		return out
	}

	tests := []struct {
		name        string
		lookup      rbac.RoleBindingLookup
		enforceRBAC bool
		ctx         context.Context
		mode        *string
		wantErr     string // "", "forbidden", or "server-error"
	}{
		{
			name:        "gateway:creator only is forbidden",
			lookup:      fakeBindingLookup{bindings: bindingsOf("gateway:creator")},
			enforceRBAC: true,
			ctx:         user(),
			mode:        strPtr("operator"),
			wantErr:     "forbidden",
		},
		{
			name:        "gateway:operator-workspace-creator passes",
			lookup:      fakeBindingLookup{bindings: bindingsOf("gateway:creator", roles.RoleGatewayOperatorWorkspaceCreator)},
			enforceRBAC: true,
			ctx:         user(),
			mode:        strPtr("operator"),
		},
		{
			name:        "platform:admin passes",
			lookup:      fakeBindingLookup{bindings: bindingsOf("platform:admin")},
			enforceRBAC: true,
			ctx:         user(),
			mode:        strPtr("operator"),
		},
		{
			name:        "no bindings at all is forbidden",
			lookup:      fakeBindingLookup{},
			enforceRBAC: true,
			ctx:         user(),
			mode:        strPtr("operator"),
			wantErr:     "forbidden",
		},
		{
			name:        "a non-operator value needs no entitlement here (funnel decides)",
			lookup:      fakeBindingLookup{bindings: bindingsOf("gateway:creator")},
			enforceRBAC: true,
			ctx:         user(),
			mode:        strPtr("shared"),
		},
		{
			name:        "an absent mode needs no entitlement",
			lookup:      fakeBindingLookup{bindings: bindingsOf("gateway:creator")},
			enforceRBAC: true,
			ctx:         user(),
			mode:        nil,
		},
		{
			name:        "RBAC off allows any caller",
			lookup:      fakeBindingLookup{bindings: bindingsOf("gateway:creator")},
			enforceRBAC: false,
			ctx:         user(),
			mode:        strPtr("operator"),
		},
		{
			name:        "a privileged control-plane write bypasses the check",
			lookup:      fakeBindingLookup{bindings: bindingsOf("gateway:creator")},
			enforceRBAC: true,
			ctx: rbac.WithGatewayWriterKind(
				context.WithValue(context.Background(), rbac.ContextUserIDKey, "user-1"),
				rbac.GatewayWriterControlPlane),
			mode: strPtr("operator"),
		},
		{
			name:        "an import write is decided by the service funnel",
			lookup:      fakeBindingLookup{bindings: bindingsOf("gateway:creator")},
			enforceRBAC: true,
			ctx: rbac.WithGatewayWriterKind(
				context.WithValue(context.Background(), rbac.ContextUserIDKey, "user-1"),
				rbac.GatewayWriterImport),
			mode: strPtr("operator"),
		},
		{
			name:        "an unidentified caller fails closed",
			lookup:      fakeBindingLookup{bindings: bindingsOf(roles.RoleGatewayOperatorWorkspaceCreator)},
			enforceRBAC: true,
			ctx:         context.Background(),
			mode:        strPtr("operator"),
			wantErr:     "forbidden",
		},
		{
			name:        "an unwired binding lookup fails closed",
			lookup:      nil,
			enforceRBAC: true,
			ctx:         user(),
			mode:        strPtr("operator"),
			wantErr:     "forbidden",
		},
		{
			name:        "a lookup failure is a server error, not an authorization denial",
			lookup:      fakeBindingLookup{err: errors.New("database is down")},
			enforceRBAC: true,
			ctx:         user(),
			mode:        strPtr("operator"),
			wantErr:     "server-error",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			authorizer := newOperatorWorkspaceAuthorizer(tc.lookup, tc.enforceRBAC)
			err := authorizer.authorizeCreate(tc.ctx, tc.mode)
			switch tc.wantErr {
			case "":
				if err != nil {
					t.Fatalf("expected the create to be authorized, got %v", err)
				}
			case "forbidden":
				if err == nil || !err.IsForbidden() {
					t.Fatalf("expected a Forbidden rejection, got %v", err)
				}
				if err.Reason != operatorWorkspaceForbiddenReason {
					t.Fatalf("rejection reason = %q, want %q", err.Reason, operatorWorkspaceForbiddenReason)
				}
				if !strings.Contains(err.Reason, roles.RoleGatewayOperatorWorkspaceCreator) {
					t.Fatalf("rejection reason %q must name the entitlement role", err.Reason)
				}
			case "server-error":
				if err == nil || err.IsForbidden() {
					t.Fatalf("expected a server error, got %v", err)
				}
			default:
				t.Fatalf("unknown wantErr %q", tc.wantErr)
			}
		})
	}
}
