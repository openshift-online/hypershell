package rbac

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/gorilla/mux"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/openshift-online/rh-trex-ai/pkg/auth"
)

// These tests cover the control-plane identity exemption
// (control_plane_identity.go): a caller whose JWT sub is a registered
// ManagedCluster is treated like an RBAC_SERVICE_ACCOUNTS entry.

const (
	spokeSub      = "spoke3-sub"
	spokeUsername = "service-account-hypershell-hyp201-spoke3"
	hubSA         = "service-account-hypershell-control-plane"
	createMethod  = "/hypershell.v1.GatewayService/CreateGateway"
	watchReleases = "/hypershell.v1.GatewayReleaseService/WatchGatewayReleases"
)

var controlPlaneOnlyMethods = []string{
	"/hypershell.v1.GatewayService/AdjustActiveSandboxCount",
	"/hypershell.v1.GatewayService/SetActiveSandboxCount",
	"/hypershell.v1.GatewayService/SetGatewayVersion",
}

type fakeActivityRecorder struct{ users []string }

func (f *fakeActivityRecorder) RecordDailyActivity(_ context.Context, userID string, _ time.Time) {
	f.users = append(f.users, userID)
}

func registeredSpokes() *fakeResolver {
	return &fakeResolver{clusters: map[string]string{spokeSub: "cluster-spoke3"}}
}

// grpcCallerContext is an authenticated gRPC caller: the framework auth
// interceptor has set the username, and the bearer token carries sub.
func grpcCallerContext(t *testing.T, username, sub string) context.Context {
	t.Helper()
	return auth.SetUsernameContext(bearerContext(t, sub), username)
}

func runUnary(t *testing.T, resolver RegisteredClusterResolver, lookup RoleBindingLookup, recorder DailyActivityRecorder, config AuthzConfig, ctx context.Context, method string) (bool, error) {
	t.Helper()
	interceptor := RBACUnaryInterceptor(lookup, fakeProvisioner{userID: "user-1"}, nil, recorder, resolver, config)
	called := false
	_, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{FullMethod: method}, func(context.Context, interface{}) (interface{}, error) {
		called = true
		return "ok", nil
	})
	return called, err
}

func runStream(t *testing.T, resolver RegisteredClusterResolver, lookup RoleBindingLookup, config AuthzConfig, ctx context.Context, method string) (bool, error) {
	t.Helper()
	interceptor := RBACStreamInterceptor(lookup, fakeProvisioner{userID: "user-1"}, nil, nil, resolver, config)
	called := false
	err := interceptor(nil, &fakeServerStream{ctx: ctx}, &grpc.StreamServerInfo{FullMethod: method}, func(interface{}, grpc.ServerStream) error {
		called = true
		return nil
	})
	return called, err
}

var enforcedWithAllowlist = AuthzConfig{EnforceRBAC: true, ServiceAccounts: []string{hubSA}}

func TestRegisteredClusterBypassesBindingsOnNormalMethod(t *testing.T) {
	noBindings := fakeLookup{}
	ctx := grpcCallerContext(t, spokeUsername, spokeSub)

	called, err := runUnary(t, registeredSpokes(), noBindings, nil, enforcedWithAllowlist, ctx, createMethod)
	if err != nil || !called {
		t.Fatalf("unary: registered cluster without bindings: handler=%v err=%v", called, err)
	}
	called, err = runStream(t, registeredSpokes(), noBindings, enforcedWithAllowlist, ctx, watchReleases)
	if err != nil || !called {
		t.Fatalf("stream: registered cluster without bindings: handler=%v err=%v", called, err)
	}

	// The same caller, unregistered, has no bindings and is denied.
	ctx = grpcCallerContext(t, spokeUsername, "unregistered-sub")
	if called, err := runUnary(t, registeredSpokes(), noBindings, nil, enforcedWithAllowlist, ctx, createMethod); called || status.Code(err) != codes.PermissionDenied {
		t.Fatalf("unary: unregistered caller without bindings: handler=%v code=%s", called, status.Code(err))
	}
}

func TestRegisteredClusterAllowedOnControlPlaneOnlyMethods(t *testing.T) {
	ctx := grpcCallerContext(t, spokeUsername, spokeSub)
	for _, method := range controlPlaneOnlyMethods {
		t.Run(method, func(t *testing.T) {
			called, err := runUnary(t, registeredSpokes(), fakeLookup{}, nil, enforcedWithAllowlist, ctx, method)
			if err != nil || !called {
				t.Fatalf("unary: handler=%v err=%v", called, err)
			}
			called, err = runStream(t, registeredSpokes(), fakeLookup{}, enforcedWithAllowlist, ctx, method)
			if err != nil || !called {
				t.Fatalf("stream: handler=%v err=%v", called, err)
			}
		})
	}
}

func TestUnregisteredCallerStillDeniedControlPlaneOnlyMethods(t *testing.T) {
	// An owner binding would pass isGRPCAuthorized for every non-read method.
	owner := fakeLookup{bindings: []BindingSummary{{RoleName: "gateway:owner", Scope: "gateway", GatewayID: strPtr("gw-1")}}}
	cases := map[string]context.Context{
		"unregistered sub": grpcCallerContext(t, "human-owner", "human-sub"),
		// The token names a registered subject but the auth interceptor set no
		// username: never a control-plane identity.
		"unauthenticated": bearerContext(t, spokeSub),
	}
	for name, ctx := range cases {
		for _, method := range controlPlaneOnlyMethods {
			t.Run(name+method, func(t *testing.T) {
				called, err := runUnary(t, registeredSpokes(), owner, nil, enforcedWithAllowlist, ctx, method)
				if called || status.Code(err) != codes.PermissionDenied {
					t.Fatalf("unary: handler=%v code=%s, want PermissionDenied", called, status.Code(err))
				}
				called, err = runStream(t, registeredSpokes(), owner, enforcedWithAllowlist, ctx, method)
				if called || status.Code(err) != codes.PermissionDenied {
					t.Fatalf("stream: handler=%v code=%s, want PermissionDenied", called, status.Code(err))
				}
			})
		}
	}
}

func TestRegisteredClusterLookupErrorIsUnavailable(t *testing.T) {
	failing := &fakeResolver{err: errors.New("db down")}
	ctx := grpcCallerContext(t, spokeUsername, spokeSub)
	for _, method := range append([]string{createMethod}, controlPlaneOnlyMethods...) {
		t.Run(method, func(t *testing.T) {
			called, err := runUnary(t, failing, fakeLookup{}, nil, enforcedWithAllowlist, ctx, method)
			if called || status.Code(err) != codes.Unavailable {
				t.Fatalf("unary: handler=%v code=%s, want Unavailable", called, status.Code(err))
			}
			called, err = runStream(t, failing, fakeLookup{}, enforcedWithAllowlist, ctx, method)
			if called || status.Code(err) != codes.Unavailable {
				t.Fatalf("stream: handler=%v code=%s, want Unavailable", called, status.Code(err))
			}
		})
	}

	// The allowlisted bootstrap account never consults the resolver.
	failing.calls = 0
	ctx = grpcCallerContext(t, hubSA, "hub-sub")
	if called, err := runUnary(t, failing, fakeLookup{}, nil, enforcedWithAllowlist, ctx, controlPlaneOnlyMethods[0]); err != nil || !called {
		t.Fatalf("allowlisted account: handler=%v err=%v", called, err)
	}
	if failing.calls != 0 {
		t.Fatalf("resolver consulted %d times for an allowlisted account", failing.calls)
	}
}

func TestNilResolverPreservesAllowlistOnlyBehaviour(t *testing.T) {
	ctx := grpcCallerContext(t, spokeUsername, spokeSub)
	for _, method := range controlPlaneOnlyMethods {
		if called, err := runUnary(t, nil, fakeLookup{}, nil, enforcedWithAllowlist, ctx, method); called || status.Code(err) != codes.PermissionDenied {
			t.Fatalf("%s: handler=%v code=%s, want PermissionDenied", method, called, status.Code(err))
		}
	}
	if called, err := runStream(t, nil, fakeLookup{}, enforcedWithAllowlist, ctx, watchReleases); called || status.Code(err) != codes.PermissionDenied {
		t.Fatalf("stream without bindings: handler=%v code=%s, want PermissionDenied", called, status.Code(err))
	}
	// The allowlist still works on its own.
	ctx = grpcCallerContext(t, hubSA, "hub-sub")
	if called, err := runUnary(t, nil, fakeLookup{}, nil, enforcedWithAllowlist, ctx, controlPlaneOnlyMethods[0]); err != nil || !called {
		t.Fatalf("allowlisted account: handler=%v err=%v", called, err)
	}
}

func TestRegisteredClusterNotRecordedAsDailyActiveGRPC(t *testing.T) {
	creator := fakeLookup{bindings: []BindingSummary{{RoleName: "gateway:creator", Scope: "global"}}}
	for _, config := range []AuthzConfig{enforcedWithAllowlist, {EnforceRBAC: false}} {
		recorder := &fakeActivityRecorder{}
		if _, err := runUnary(t, registeredSpokes(), creator, recorder, config, grpcCallerContext(t, spokeUsername, spokeSub), createMethod); err != nil {
			t.Fatalf("enforce=%v: registered call failed: %v", config.EnforceRBAC, err)
		}
		if len(recorder.users) != 0 {
			t.Fatalf("enforce=%v: registered cluster recorded as daily active: %v", config.EnforceRBAC, recorder.users)
		}
		if _, err := runUnary(t, registeredSpokes(), creator, recorder, config, grpcCallerContext(t, "human", "human-sub"), createMethod); err != nil {
			t.Fatalf("enforce=%v: user call failed: %v", config.EnforceRBAC, err)
		}
		if len(recorder.users) != 1 {
			t.Fatalf("enforce=%v: user not recorded: %v", config.EnforceRBAC, recorder.users)
		}
	}

	// With RBAC not enforced a lookup failure must not fail the call; it only
	// suppresses the record.
	recorder := &fakeActivityRecorder{}
	called, err := runUnary(t, &fakeResolver{err: errors.New("db down")}, creator, recorder, AuthzConfig{}, grpcCallerContext(t, spokeUsername, spokeSub), createMethod)
	if err != nil || !called || len(recorder.users) != 0 {
		t.Fatalf("unenforced lookup failure: handler=%v err=%v recorded=%v", called, err, recorder.users)
	}
}

// HTTP: the REST path gets the daily-activity exemption but, like an
// allowlisted account, no role-binding bypass.

func serveREST(t *testing.T, resolver RegisteredClusterResolver, lookup RoleBindingLookup, recorder DailyActivityRecorder, config AuthzConfig, method, route, path string, claims jwt.MapClaims) (bool, int) {
	t.Helper()
	middleware := NewRBACAuthzMiddleware(lookup, config, recorder, resolver)
	reached := false
	router := mux.NewRouter()
	router.Handle(route, middleware.AuthorizeApi(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	}))).Methods(method)
	request := httptest.NewRequest(method, path, nil)
	ctx := context.WithValue(request.Context(), auth.ContextAuthKey, &jwt.Token{Claims: claims})
	ctx = context.WithValue(ctx, ContextUserIDKey, "user-id")
	recorder2 := httptest.NewRecorder()
	router.ServeHTTP(recorder2, request.WithContext(ctx))
	return reached, recorder2.Code
}

func spokeClaims() jwt.MapClaims {
	return jwt.MapClaims{"preferred_username": spokeUsername, "sub": spokeSub}
}

func TestAuthorizeApiRegisteredClusterNotRecordedAsDailyActive(t *testing.T) {
	creator := authorizationLookup{bindings: []BindingSummary{{RoleName: "gateway:creator", Scope: "global"}}}
	for _, config := range []AuthzConfig{{EnforceRBAC: true}, {EnforceRBAC: false}} {
		recorder := &fakeActivityRecorder{}
		reached, code := serveREST(t, registeredSpokes(), creator, recorder, config, http.MethodPost, "/api/hypershell/v1/gateways", "/api/hypershell/v1/gateways", spokeClaims())
		if !reached || code != http.StatusOK {
			t.Fatalf("enforce=%v: registered cluster with creator binding: reached=%v status=%d", config.EnforceRBAC, reached, code)
		}
		if len(recorder.users) != 0 {
			t.Fatalf("enforce=%v: registered cluster recorded as daily active: %v", config.EnforceRBAC, recorder.users)
		}

		// A human user is still recorded, and so is the spoke when no resolver
		// is wired (nil resolver keeps the old behaviour).
		serveREST(t, registeredSpokes(), creator, recorder, config, http.MethodPost, "/api/hypershell/v1/gateways", "/api/hypershell/v1/gateways", jwt.MapClaims{"preferred_username": "human", "sub": "human-sub"})
		serveREST(t, nil, creator, recorder, config, http.MethodPost, "/api/hypershell/v1/gateways", "/api/hypershell/v1/gateways", spokeClaims())
		if len(recorder.users) != 2 {
			t.Fatalf("enforce=%v: recorded %v, want the user and the unresolved spoke", config.EnforceRBAC, recorder.users)
		}
	}

	// A lookup failure never fails the request; it only suppresses the record.
	recorder := &fakeActivityRecorder{}
	reached, code := serveREST(t, &fakeResolver{err: errors.New("db down")}, creator, recorder, AuthzConfig{EnforceRBAC: true}, http.MethodPost, "/api/hypershell/v1/gateways", "/api/hypershell/v1/gateways", spokeClaims())
	if !reached || code != http.StatusOK || len(recorder.users) != 0 {
		t.Fatalf("lookup failure: reached=%v status=%d recorded=%v", reached, code, recorder.users)
	}
}

func TestAuthorizeApiRegisteredClusterGetsNoRESTBypass(t *testing.T) {
	// No bindings: a registered spoke may not create gateways or read the user
	// inventory over REST, exactly like an allowlisted account.
	resolver := registeredSpokes()
	config := AuthzConfig{EnforceRBAC: true, ServiceAccounts: []string{hubSA}}
	if reached, code := serveREST(t, resolver, authorizationLookup{}, nil, config, http.MethodPost, "/api/hypershell/v1/gateways", "/api/hypershell/v1/gateways", spokeClaims()); reached || code != http.StatusForbidden {
		t.Fatalf("POST /gateways: reached=%v status=%d, want 403", reached, code)
	}
	if reached, code := serveREST(t, resolver, authorizationLookup{}, nil, config, http.MethodGet, "/api/hypershell/v1/users", "/api/hypershell/v1/users", spokeClaims()); reached || code != http.StatusForbidden {
		t.Fatalf("GET /users: reached=%v status=%d, want 403", reached, code)
	}

	// Registration stays JWT-role based and never consults the resolver.
	resolver.calls = 0
	claims := spokeClaims()
	claims["realm_access"] = map[string]interface{}{"roles": []interface{}{roleManagedClusterRegistrar}}
	if reached, _ := serveREST(t, resolver, authorizationLookup{}, nil, config, http.MethodPost, "/api/hypershell/v1/managed_clusters/registration", "/api/hypershell/v1/managed_clusters/registration", claims); !reached {
		t.Fatal("registration with managed-cluster-registrar did not reach the handler")
	}
	if reached, code := serveREST(t, resolver, authorizationLookup{}, nil, config, http.MethodPost, "/api/hypershell/v1/managed_clusters/registration", "/api/hypershell/v1/managed_clusters/registration", spokeClaims()); reached || code != http.StatusForbidden {
		t.Fatalf("registered cluster without registrar role: reached=%v status=%d, want 403", reached, code)
	}
	if resolver.calls != 0 {
		t.Fatalf("registration consulted the resolver %d times", resolver.calls)
	}
}
