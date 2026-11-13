package roleBindings

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/openshift-online/hypershell/components/api-server/pkg/rbac"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
)

type fakeGatewayLookup struct {
	ids   []string
	err   *errors.ServiceError
	calls int
}

func (f *fakeGatewayLookup) FindGatewayIDsByUserID(context.Context, string) ([]string, *errors.ServiceError) {
	f.calls++
	return f.ids, f.err
}

func enforceForTest(t *testing.T) {
	t.Helper()
	rbac.SetEnforcementForTest(true)
	// TestMain runs this package with enforcement off.
	t.Cleanup(func() { rbac.SetEnforcementForTest(false) })
}

func callerContext(userID string, jwtRoles ...string) context.Context {
	ctx := context.Background()
	if userID != "" {
		ctx = context.WithValue(ctx, rbac.ContextUserIDKey, userID)
	}
	if len(jwtRoles) > 0 {
		ctx = context.WithValue(ctx, rbac.ContextJWTRolesKey, jwtRoles)
	}
	return ctx
}

func TestResolveReadScope(t *testing.T) {
	t.Run("enforcement off sees everything", func(t *testing.T) {
		rbac.SetEnforcementForTest(false)
		lookup := &fakeGatewayLookup{}
		scope, err := resolveReadScope(callerContext("u1"), lookup)
		if err != nil || !scope.all || lookup.calls != 0 {
			t.Fatalf("got scope=%+v err=%v calls=%d, want all without lookup", scope, err, lookup.calls)
		}
	})

	enforceForTest(t)
	for _, tc := range []struct {
		name        string
		ctx         context.Context
		lookup      *fakeGatewayLookup
		wantAll     bool
		wantUser    string
		wantErrCode int
	}{
		{name: "missing identity sees nothing", ctx: callerContext(""), lookup: &fakeGatewayLookup{}},
		{name: "platform admin sees everything", ctx: callerContext("u1", "platform:admin"), lookup: &fakeGatewayLookup{}, wantAll: true},
		{name: "member is scoped", ctx: callerContext("u1", "gateway:creator"), lookup: &fakeGatewayLookup{ids: []string{"gw-1"}}, wantUser: "u1"},
		{name: "lookup error propagates", ctx: callerContext("u1"), lookup: &fakeGatewayLookup{err: errors.GeneralError("db down")}, wantErrCode: http.StatusInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scope, err := resolveReadScope(tc.ctx, tc.lookup)
			if tc.wantErrCode != 0 {
				if err == nil || err.HttpCode != tc.wantErrCode {
					t.Fatalf("err = %v, want HTTP %d", err, tc.wantErrCode)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if scope.all != tc.wantAll || scope.userID != tc.wantUser {
				t.Fatalf("scope = %+v, want all=%v user=%q", scope, tc.wantAll, tc.wantUser)
			}
		})
	}
}

func TestReadScopeCanSee(t *testing.T) {
	str := func(s string) *string { return &s }
	member := readScope{userID: "me", gatewayIDs: []string{"gw-1"}}
	for _, tc := range []struct {
		name  string
		scope readScope
		rb    *RoleBinding
		want  bool
	}{
		{"all sees any binding", readScope{all: true}, &RoleBinding{Scope: ScopeGateway, GatewayID: str("gw-9")}, true},
		{"own global binding", member, &RoleBinding{Scope: ScopeGlobal, UserID: str("me")}, true},
		{"other user's global binding", member, &RoleBinding{Scope: ScopeGlobal, UserID: str("other")}, false},
		{"binding on a visible gateway", member, &RoleBinding{Scope: ScopeGateway, UserID: str("other"), GatewayID: str("gw-1")}, true},
		{"binding on another gateway", member, &RoleBinding{Scope: ScopeGateway, UserID: str("other"), GatewayID: str("gw-2")}, false},
		{"gateway scope without gateway id", member, &RoleBinding{Scope: ScopeGateway, UserID: str("other")}, false},
		{"no identity", readScope{}, &RoleBinding{Scope: ScopeGlobal, UserID: str("")}, false},
		{"nil binding", member, nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.scope.canSee(tc.rb); got != tc.want {
				t.Fatalf("canSee = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestReadScopeRestrictSearch(t *testing.T) {
	for _, tc := range []struct {
		name     string
		scope    readScope
		search   string
		want     string
		wantCode int
	}{
		{name: "all keeps the search", scope: readScope{all: true}, search: "scope = 'global'", want: "scope = 'global'"},
		{name: "no identity is forbidden", scope: readScope{}, wantCode: http.StatusForbidden},
		{name: "own bindings only", scope: readScope{userID: "me"}, want: "user_id = 'me'"},
		{name: "own and gateway bindings", scope: readScope{userID: "me", gatewayIDs: []string{"gw-1", "gw-2"}},
			want: "(user_id = 'me' or gateway_id in ('gw-1','gw-2'))"},
		{name: "search is wrapped and restricted", scope: readScope{userID: "me", gatewayIDs: []string{"gw-1"}}, search: "scope = 'gateway'",
			want: "(scope = 'gateway') and (user_id = 'me' or gateway_id in ('gw-1'))"},
		{name: "escaping search is rejected", scope: readScope{userID: "me"}, search: "gateway_id = 'x') or (gateway_id = 'y'", wantCode: http.StatusBadRequest},
		{name: "unknown column is rejected", scope: readScope{userID: "me"}, search: "no_such_column = 'x'", wantCode: http.StatusBadRequest},
		{name: "quote in a stored id cannot end the literal", scope: readScope{userID: "me'"}, want: "user_id = 'me'"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.scope.restrictSearch(tc.search)
			if tc.wantCode != 0 {
				if err == nil || err.HttpCode != tc.wantCode {
					t.Fatalf("err = %v, want HTTP %d", err, tc.wantCode)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if strings.TrimSpace(got) != tc.want {
				t.Fatalf("search = %q, want %q", got, tc.want)
			}
		})
	}
}
