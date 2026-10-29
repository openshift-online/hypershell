package ciTrusts

import (
	"context"
	"testing"

	"github.com/openshift-online/hypershell/components/api-server/pkg/rbac"
)

// stubBindingLookup satisfies rbac.RoleBindingLookup.
type stubBindingLookup struct {
	summaries []rbac.BindingSummary
	err       error
}

func (s stubBindingLookup) FindBindingsByUserID(context.Context, string) ([]rbac.BindingSummary, error) {
	return s.summaries, s.err
}

func strPtr(s string) *string { return &s }

func ctxWithUser() context.Context {
	return context.WithValue(context.Background(), rbac.ContextUserIDKey, "user-1")
}

func TestAuthorizeCreateRequiresOwnerOrAdminOnReferencedGateway(t *testing.T) {
	gw := "gw-1"
	cases := []struct {
		name      string
		summaries []rbac.BindingSummary
		wantErr   bool
	}{
		{"owner", []rbac.BindingSummary{{RoleName: "gateway:owner", Scope: "gateway", GatewayID: &gw}}, false},
		{"admin", []rbac.BindingSummary{{RoleName: "gateway:admin", Scope: "gateway", GatewayID: &gw}}, false},
		{"platform admin", []rbac.BindingSummary{{RoleName: "platform:admin", Scope: "global"}}, false},
		{"creator only", []rbac.BindingSummary{{RoleName: "gateway:creator", Scope: "global"}}, true},
		{"viewer", []rbac.BindingSummary{{RoleName: "gateway:viewer", Scope: "gateway", GatewayID: &gw}}, true},
		{"owner of another gateway", []rbac.BindingSummary{{RoleName: "gateway:owner", Scope: "gateway", GatewayID: strPtr("gw-2")}}, true},
		{"ci-federation admin cannot manage trusts", []rbac.BindingSummary{{RoleName: "ci-federation:admin", Scope: "global"}}, true},
		{"unbound", nil, true},
	}
	for _, c := range cases {
		a := &tenancyAuthorizer{bindings: stubBindingLookup{summaries: c.summaries}}
		svcErr := a.authorizeCreate(ctxWithUser(), gw)
		if (svcErr != nil) != c.wantErr {
			t.Errorf("%s: authorizeCreate error = %v, wantErr %v", c.name, svcErr, c.wantErr)
		}
		// A create failure is always 403 (the gateway id is caller-supplied; there
		// is no existing record whose existence must be masked).
		if svcErr != nil && !svcErr.IsForbidden() {
			t.Errorf("%s: authorizeCreate failure should be 403; got %v", c.name, svcErr)
		}
	}
}

// TestAuthorizeManageExistingMasksUnreadableAs404 covers the write path for an
// existing trust: a manager passes, a viewer on the same gateway is 403 (the
// record is already visible), and a caller with no visibility is masked as 404.
func TestAuthorizeManageExistingMasksUnreadableAs404(t *testing.T) {
	gw := "gw-1"
	cases := []struct {
		name      string
		summaries []rbac.BindingSummary
		want      string // "ok", "403", or "404"
	}{
		{"owner", []rbac.BindingSummary{{RoleName: "gateway:owner", Scope: "gateway", GatewayID: &gw}}, "ok"},
		{"admin", []rbac.BindingSummary{{RoleName: "gateway:admin", Scope: "gateway", GatewayID: &gw}}, "ok"},
		{"platform admin", []rbac.BindingSummary{{RoleName: "platform:admin", Scope: "global"}}, "ok"},
		{"viewer on same gateway", []rbac.BindingSummary{{RoleName: "gateway:viewer", Scope: "gateway", GatewayID: &gw}}, "403"},
		{"owner of another gateway", []rbac.BindingSummary{{RoleName: "gateway:owner", Scope: "gateway", GatewayID: strPtr("gw-2")}}, "404"},
		{"creator only", []rbac.BindingSummary{{RoleName: "gateway:creator", Scope: "global"}}, "404"},
		{"unbound", nil, "404"},
	}
	for _, c := range cases {
		a := &tenancyAuthorizer{bindings: stubBindingLookup{summaries: c.summaries}}
		svcErr := a.authorizeManageExisting(ctxWithUser(), gw)
		switch c.want {
		case "ok":
			if svcErr != nil {
				t.Errorf("%s: want ok, got %v", c.name, svcErr)
			}
		case "403":
			if svcErr == nil || !svcErr.IsForbidden() {
				t.Errorf("%s: want 403, got %v", c.name, svcErr)
			}
		case "404":
			if svcErr == nil || !svcErr.Is404() {
				t.Errorf("%s: want 404, got %v", c.name, svcErr)
			}
		}
	}
}

func TestAuthorizeReadAllowsOwnerAdminViewerMasksOthers(t *testing.T) {
	gw := "gw-1"
	cases := []struct {
		name      string
		summaries []rbac.BindingSummary
		wantErr   bool
	}{
		{"owner", []rbac.BindingSummary{{RoleName: "gateway:owner", Scope: "gateway", GatewayID: &gw}}, false},
		{"admin", []rbac.BindingSummary{{RoleName: "gateway:admin", Scope: "gateway", GatewayID: &gw}}, false},
		{"viewer", []rbac.BindingSummary{{RoleName: "gateway:viewer", Scope: "gateway", GatewayID: &gw}}, false},
		{"platform admin", []rbac.BindingSummary{{RoleName: "platform:admin", Scope: "global"}}, false},
		{"owner of another gateway", []rbac.BindingSummary{{RoleName: "gateway:owner", Scope: "gateway", GatewayID: strPtr("gw-2")}}, true},
		{"creator only", []rbac.BindingSummary{{RoleName: "gateway:creator", Scope: "global"}}, true},
		{"unbound", nil, true},
	}
	for _, c := range cases {
		a := &tenancyAuthorizer{bindings: stubBindingLookup{summaries: c.summaries}}
		svcErr := a.authorizeRead(ctxWithUser(), gw)
		if (svcErr != nil) != c.wantErr {
			t.Errorf("%s: authorizeRead error = %v, wantErr %v", c.name, svcErr, c.wantErr)
		}
		// A denied read must be masked as 404, not 403, so existence is not disclosed.
		if svcErr != nil && !svcErr.Is404() {
			t.Errorf("%s: authorizeRead should mask as 404; got %v", c.name, svcErr)
		}
	}
}

func TestReadableGatewayScopeConfinesListing(t *testing.T) {
	gwA, gwB := "gw-a", "gw-b"

	// Owner of gw-a (plus viewer of gw-b) is restricted to those two gateways.
	a := &tenancyAuthorizer{bindings: stubBindingLookup{summaries: []rbac.BindingSummary{
		{RoleName: "gateway:owner", Scope: "gateway", GatewayID: &gwA},
		{RoleName: "gateway:viewer", Scope: "gateway", GatewayID: &gwB},
	}}}
	ids, unrestricted, svcErr := a.readableGatewayScope(ctxWithUser())
	if svcErr != nil {
		t.Fatalf("unexpected error: %v", svcErr)
	}
	if unrestricted {
		t.Error("a non-admin caller must be restricted")
	}
	if len(ids) != 2 {
		t.Errorf("expected scope of 2 gateways; got %v", ids)
	}

	// platform:admin is unrestricted (no query filter).
	admin := &tenancyAuthorizer{bindings: stubBindingLookup{summaries: []rbac.BindingSummary{
		{RoleName: "platform:admin", Scope: "global"},
	}}}
	_, unrestricted, svcErr = admin.readableGatewayScope(ctxWithUser())
	if svcErr != nil || !unrestricted {
		t.Errorf("platform:admin must be unrestricted; got unrestricted=%v err=%v", unrestricted, svcErr)
	}

	// Unbound caller is restricted to an empty scope (the handler returns an
	// empty page without querying).
	ids, unrestricted, svcErr = (&tenancyAuthorizer{bindings: stubBindingLookup{}}).readableGatewayScope(ctxWithUser())
	if svcErr != nil || unrestricted || len(ids) != 0 {
		t.Errorf("unbound caller must have empty restricted scope; got ids=%v unrestricted=%v err=%v", ids, unrestricted, svcErr)
	}
}

func TestGatewayIDSearchFilter(t *testing.T) {
	got := gatewayIDSearchFilter([]string{"gw-a", "gw-b"})
	want := "gateway_id in ('gw-a','gw-b')"
	if got != want {
		t.Errorf("gatewayIDSearchFilter = %q, want %q", got, want)
	}
}

func TestTenancyAuthorizerRequiresIdentity(t *testing.T) {
	a := &tenancyAuthorizer{bindings: stubBindingLookup{}}
	if svcErr := a.authorizeCreate(context.Background(), "gw-1"); svcErr == nil || !svcErr.IsForbidden() {
		t.Errorf("caller without identity must be forbidden; got %v", svcErr)
	}
}

func TestNilTenancyAuthorizerAllows(t *testing.T) {
	// A nil authorizer means RBAC is not enforced (dev posture): the handler
	// allows, mirroring the middleware's RBAC_ENFORCE gate.
	var a *tenancyAuthorizer
	if svcErr := a.authorizeCreate(context.Background(), "gw-1"); svcErr != nil {
		t.Errorf("nil authorizer must allow create; got %v", svcErr)
	}
	if svcErr := a.authorizeManageExisting(context.Background(), "gw-1"); svcErr != nil {
		t.Errorf("nil authorizer must allow manage-existing; got %v", svcErr)
	}
	if svcErr := a.authorizeRead(context.Background(), "gw-1"); svcErr != nil {
		t.Errorf("nil authorizer must allow read; got %v", svcErr)
	}
	_, unrestricted, svcErr := a.readableGatewayScope(context.Background())
	if svcErr != nil || !unrestricted {
		t.Errorf("nil authorizer must be unrestricted for listing; got unrestricted=%v err=%v", unrestricted, svcErr)
	}
}
