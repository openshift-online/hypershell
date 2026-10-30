package agentRuntimes

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

func TestManageAuthorizerRequiresGatewayBinding(t *testing.T) {
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
		{"owner of another gateway", []rbac.BindingSummary{{RoleName: "gateway:owner", Scope: "gateway", GatewayID: strPtrAR("gw-2")}}, true},
	}
	for _, c := range cases {
		a := &manageAuthorizer{bindings: stubBindingLookup{summaries: c.summaries}}
		ctx := context.WithValue(context.Background(), rbac.ContextUserIDKey, "user-1")
		svcErr := a.authorizeManageGateway(ctx, gw)
		if (svcErr != nil) != c.wantErr {
			t.Errorf("%s: authorizeManageGateway error = %v, wantErr %v", c.name, svcErr, c.wantErr)
		}
	}
}

func TestManageAuthorizerRequiresIdentity(t *testing.T) {
	a := &manageAuthorizer{bindings: stubBindingLookup{}}
	if svcErr := a.authorizeManageGateway(context.Background(), "gw-1"); svcErr == nil || !svcErr.IsForbidden() {
		t.Errorf("caller without identity must be forbidden; got %v", svcErr)
	}
}

func TestManageAuthorizerNilAllows(t *testing.T) {
	// A nil authorizer means RBAC is not enforced (authentication off): the
	// handler allows, mirroring the middleware's enforcement gate.
	var a *manageAuthorizer
	if svcErr := a.authorizeManageGateway(context.Background(), "gw-1"); svcErr != nil {
		t.Errorf("nil authorizer must allow; got %v", svcErr)
	}
}

func strPtrAR(s string) *string { return &s }
