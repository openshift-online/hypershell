package gateways

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/openshift-online/hypershell/components/api-server/pkg/rbac"
	"github.com/openshift-online/hypershell/components/api-server/plugins/roles"
)

// boolPtr is a test shorthand for the optional dedicated_route field.
func boolPtr(v bool) *bool { return &v }

// TestApplyGatewayDedicatedRouteCreateRequest pins the service-funnel translation
// of the create-only dedicated_route request: a request without a route becomes
// the platform-managed route opt-in the control plane keys on, an explicit route
// wins, and a GitOps-import create may not request the flag at all.
func TestApplyGatewayDedicatedRouteCreateRequest(t *testing.T) {
	tests := []struct {
		name        string
		kind        rbac.GatewayWriterKind
		gateway     *Gateway
		wantRoute   string // "" means the route must stay nil
		wantErrText string
	}{
		{
			name:      "user request applies the certificate opt-in",
			kind:      rbac.GatewayWriterUser,
			gateway:   &Gateway{Name: "gw", DedicatedRoute: boolPtr(true)},
			wantRoute: `{"certificate":{},"enabled":true}`,
		},
		{
			name:      "control-plane request with no route applies the certificate opt-in",
			kind:      rbac.GatewayWriterControlPlane,
			gateway:   &Gateway{Name: "gw", DedicatedRoute: boolPtr(true)},
			wantRoute: `{"certificate":{},"enabled":true}`,
		},
		{
			name:      "explicit control-plane route wins",
			kind:      rbac.GatewayWriterControlPlane,
			gateway:   &Gateway{Name: "gw", DedicatedRoute: boolPtr(true), Route: strPtr(`{"enabled":true}`)},
			wantRoute: `{"enabled":true}`,
		},
		{
			name:      "explicit control-plane route with the opt-in is kept verbatim",
			kind:      rbac.GatewayWriterControlPlane,
			gateway:   &Gateway{Name: "gw", DedicatedRoute: boolPtr(true), Route: strPtr(`{"certificate":{},"hosted":true}`)},
			wantRoute: `{"certificate":{},"hosted":true}`,
		},
		{
			name:    "no request leaves the route unset",
			kind:    rbac.GatewayWriterUser,
			gateway: &Gateway{Name: "gw"},
		},
		{
			name:    "explicit false leaves the route unset",
			kind:    rbac.GatewayWriterUser,
			gateway: &Gateway{Name: "gw", DedicatedRoute: boolPtr(false)},
		},
		{
			name:        "import request is rejected",
			kind:        rbac.GatewayWriterImport,
			gateway:     &Gateway{Name: "gw", DedicatedRoute: boolPtr(true)},
			wantErrText: dedicatedRouteImportForbiddenReason,
		},
		{
			name:    "import explicit false is not a request",
			kind:    rbac.GatewayWriterImport,
			gateway: &Gateway{Name: "gw", DedicatedRoute: boolPtr(false)},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := applyGatewayDedicatedRouteCreateRequest(tc.kind, tc.gateway)
			if tc.wantErrText != "" {
				if err == nil {
					t.Fatalf("expected the request to be rejected, got nil")
				}
				if !err.IsForbidden() {
					t.Fatalf("expected a Forbidden rejection, got %v", err)
				}
				if err.Reason != tc.wantErrText {
					t.Fatalf("rejection reason = %q, want %q", err.Reason, tc.wantErrText)
				}
				return
			}
			if err != nil {
				t.Fatalf("expected the request to be accepted, got %v", err)
			}
			if tc.wantRoute == "" {
				if tc.gateway.Route != nil {
					t.Fatalf("route = %v, want nil", *tc.gateway.Route)
				}
				return
			}
			if tc.gateway.Route == nil {
				t.Fatalf("route = nil, want %q", tc.wantRoute)
			}
			if !jsonEq(tc.gateway.Route, &tc.wantRoute) {
				t.Fatalf("route = %q, want %q", *tc.gateway.Route, tc.wantRoute)
			}
			// The server-applied route must satisfy the same boundary shape check
			// an authored route does, so provisioning cannot fail on it later.
			if svcErr := validateRouteCertificateShape(tc.gateway.Route); svcErr != nil {
				t.Fatalf("applied route %q fails the route.certificate shape check: %v", *tc.gateway.Route, svcErr)
			}
		})
	}
}

// TestDedicatedRouteChanged pins the create-only immutability comparison: adding
// or removing the request is a change, while nil vs false (both "not requested")
// is not, so an idempotent client resending a whole object is not rejected.
func TestDedicatedRouteChanged(t *testing.T) {
	tests := []struct {
		name    string
		current *bool
		next    *bool
		want    bool
	}{
		{"both absent", nil, nil, false},
		{"both false", boolPtr(false), boolPtr(false), false},
		{"nil to false is not a request", nil, boolPtr(false), false},
		{"false to nil is not a request", boolPtr(false), nil, false},
		{"request added", nil, boolPtr(true), true},
		{"request kept", boolPtr(true), boolPtr(true), false},
		{"request removed", boolPtr(true), nil, true},
		{"request replaced by false", boolPtr(true), boolPtr(false), true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := dedicatedRouteChanged(tc.current, tc.next); got != tc.want {
				t.Fatalf("dedicatedRouteChanged(%v, %v) = %t, want %t", tc.current, tc.next, got, tc.want)
			}
		})
	}
}

// fakeBindingLookup is a RoleBindingLookup serving a fixed binding set.
type fakeBindingLookup struct {
	bindings []rbac.BindingSummary
	err      error
}

func (f fakeBindingLookup) FindBindingsByUserID(context.Context, string) ([]rbac.BindingSummary, error) {
	return f.bindings, f.err
}

// TestDedicatedRouteAuthorizeCreate is the RBAC gate at the create boundaries: a
// caller holding only gateway:creator gets the 403 naming the entitlement role and
// the rate-limit rationale, while a gateway:dedicated-route-creator or
// platform:admin caller passes. A create that does not request the flag needs no
// entitlement, and RBAC-off, privileged, and import writes are decided elsewhere.
func TestDedicatedRouteAuthorizeCreate(t *testing.T) {
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
		requested   *bool
		wantErr     string // "" means accepted; "forbidden" or "server-error" otherwise
	}{
		{
			name:        "gateway:creator only is forbidden",
			lookup:      fakeBindingLookup{bindings: bindingsOf("gateway:creator")},
			enforceRBAC: true,
			ctx:         user(),
			requested:   boolPtr(true),
			wantErr:     "forbidden",
		},
		{
			name:        "gateway:dedicated-route-creator passes",
			lookup:      fakeBindingLookup{bindings: bindingsOf("gateway:creator", roles.RoleGatewayDedicatedRouteCreator)},
			enforceRBAC: true,
			ctx:         user(),
			requested:   boolPtr(true),
		},
		{
			name:        "platform:admin passes",
			lookup:      fakeBindingLookup{bindings: bindingsOf("platform:admin")},
			enforceRBAC: true,
			ctx:         user(),
			requested:   boolPtr(true),
		},
		{
			name:        "no bindings at all is forbidden",
			lookup:      fakeBindingLookup{},
			enforceRBAC: true,
			ctx:         user(),
			requested:   boolPtr(true),
			wantErr:     "forbidden",
		},
		{
			name:        "an explicit false needs no entitlement",
			lookup:      fakeBindingLookup{bindings: bindingsOf("gateway:creator")},
			enforceRBAC: true,
			ctx:         user(),
			requested:   boolPtr(false),
		},
		{
			name:        "an absent request needs no entitlement",
			lookup:      fakeBindingLookup{bindings: bindingsOf("gateway:creator")},
			enforceRBAC: true,
			ctx:         user(),
			requested:   nil,
		},
		{
			name:        "RBAC off allows any caller",
			lookup:      fakeBindingLookup{bindings: bindingsOf("gateway:creator")},
			enforceRBAC: false,
			ctx:         user(),
			requested:   boolPtr(true),
		},
		{
			name:        "a privileged control-plane write bypasses the check",
			lookup:      fakeBindingLookup{bindings: bindingsOf("gateway:creator")},
			enforceRBAC: true,
			ctx: rbac.WithGatewayWriterKind(
				context.WithValue(context.Background(), rbac.ContextUserIDKey, "user-1"),
				rbac.GatewayWriterControlPlane),
			requested: boolPtr(true),
		},
		{
			name:        "an import write is decided by the service funnel",
			lookup:      fakeBindingLookup{bindings: bindingsOf("gateway:creator")},
			enforceRBAC: true,
			ctx: rbac.WithGatewayWriterKind(
				context.WithValue(context.Background(), rbac.ContextUserIDKey, "user-1"),
				rbac.GatewayWriterImport),
			requested: boolPtr(true),
		},
		{
			name:        "an unidentified caller fails closed",
			lookup:      fakeBindingLookup{bindings: bindingsOf(roles.RoleGatewayDedicatedRouteCreator)},
			enforceRBAC: true,
			ctx:         context.Background(),
			requested:   boolPtr(true),
			wantErr:     "forbidden",
		},
		{
			name:        "an unwired binding lookup fails closed",
			lookup:      nil,
			enforceRBAC: true,
			ctx:         user(),
			requested:   boolPtr(true),
			wantErr:     "forbidden",
		},
		{
			name:        "a lookup failure is a server error, not an authorization denial",
			lookup:      fakeBindingLookup{err: errors.New("database is down")},
			enforceRBAC: true,
			ctx:         user(),
			requested:   boolPtr(true),
			wantErr:     "server-error",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			authorizer := newDedicatedRouteAuthorizer(tc.lookup, tc.enforceRBAC)
			err := authorizer.authorizeCreate(tc.ctx, tc.requested)
			switch tc.wantErr {
			case "":
				if err != nil {
					t.Fatalf("expected the create to be authorized, got %v", err)
				}
			case "forbidden":
				if err == nil || !err.IsForbidden() {
					t.Fatalf("expected a Forbidden rejection, got %v", err)
				}
				if err.Reason != dedicatedRouteForbiddenReason {
					t.Fatalf("rejection reason = %q, want %q", err.Reason, dedicatedRouteForbiddenReason)
				}
				if !strings.Contains(err.Reason, roles.RoleGatewayDedicatedRouteCreator) {
					t.Fatalf("rejection reason %q must name the entitlement role", err.Reason)
				}
				if !strings.Contains(err.Reason, "rate-limited") {
					t.Fatalf("rejection reason %q must state the rate-limit rationale", err.Reason)
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
