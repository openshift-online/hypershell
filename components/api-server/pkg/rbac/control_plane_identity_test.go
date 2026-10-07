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

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/auth"
)

// These tests cover the control-plane identity exemption
// (control_plane_identity.go): a caller whose JWT sub is a registered
// ManagedCluster is treated like an RBAC_SERVICE_ACCOUNTS entry.

const (
	spokeSub      = "spoke3-sub"
	spokeUsername = "service-account-hypershell-hyp201-spoke3"
	hubSA         = "service-account-hypershell-control-plane"
	watchGateways = "/hypershell.v1.GatewayService/WatchGateways"
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

// fakeGateways places gw-own (namespace ns-own) on the spoke's cluster and
// gw-foreign (ns-foreign) on another.
type fakeGateways struct{ err error }

func (f fakeGateways) GatewayClusterID(_ context.Context, id string) (string, bool, error) {
	if f.err != nil {
		return "", false, f.err
	}
	switch id {
	case "gw-own":
		return "cluster-spoke3", true, nil
	case "gw-foreign":
		return "cluster-other", true, nil
	}
	return "", false, nil
}

func (f fakeGateways) NamespaceClusterID(_ context.Context, ns string) (string, bool, error) {
	if f.err != nil {
		return "", false, f.err
	}
	switch ns {
	case "ns-own":
		return "cluster-spoke3", true, nil
	case "ns-foreign":
		return "cluster-other", true, nil
	}
	return "", false, nil
}

// grpcCallerContext is an authenticated gRPC caller: the framework auth
// interceptor has set the username, and the bearer token carries sub.
func grpcCallerContext(t *testing.T, username, sub string) context.Context {
	t.Helper()
	return auth.SetUsernameContext(bearerContext(t, sub), username)
}

func runUnary(t *testing.T, resolver RegisteredClusterResolver, lookup RoleBindingLookup, recorder DailyActivityRecorder, config AuthzConfig, ctx context.Context, method string, req interface{}) (bool, error) {
	t.Helper()
	interceptor := RBACUnaryInterceptor(lookup, fakeProvisioner{userID: "user-1"}, nil, recorder, resolver, fakeGateways{}, config)
	called := false
	_, err := interceptor(ctx, req, &grpc.UnaryServerInfo{FullMethod: method}, func(context.Context, interface{}) (interface{}, error) {
		called = true
		return "ok", nil
	})
	return called, err
}

func runStream(t *testing.T, resolver RegisteredClusterResolver, lookup RoleBindingLookup, config AuthzConfig, ctx context.Context, method string) (bool, error) {
	t.Helper()
	interceptor := RBACStreamInterceptor(lookup, fakeProvisioner{userID: "user-1"}, nil, nil, resolver, fakeGateways{}, config)
	called := false
	err := interceptor(nil, &fakeServerStream{ctx: ctx}, &grpc.StreamServerInfo{FullMethod: method}, func(interface{}, grpc.ServerStream) error {
		called = true
		return nil
	})
	return called, err
}

var enforcedWithAllowlist = AuthzConfig{EnforceRBAC: true, ServiceAccounts: []string{hubSA}}

// A registered control plane needs no role bindings, but only for its own
// cluster's gateways, fleet-wide reads, status writes, and its own record.
func TestRegisteredClusterIsScopedToItsCluster(t *testing.T) {
	ctx := grpcCallerContext(t, spokeUsername, spokeSub)
	tests := []struct {
		name   string
		method string
		req    interface{}
		want   codes.Code
	}{
		{"get own gateway", gatewaySvc("GetGateway"), &pb.GetGatewayRequest{Id: "gw-own"}, codes.OK},
		{"get foreign gateway", gatewaySvc("GetGateway"), &pb.GetGatewayRequest{Id: "gw-foreign"}, codes.NotFound},
		{"get unknown gateway", gatewaySvc("GetGateway"), &pb.GetGatewayRequest{Id: "gw-missing"}, codes.NotFound},
		{"update own gateway", gatewaySvc("UpdateGateway"), &pb.UpdateGatewayRequest{Id: "gw-own"}, codes.OK},
		{"update own gateway keeping its cluster", gatewaySvc("UpdateGateway"), &pb.UpdateGatewayRequest{Id: "gw-own", ClusterId: strPtr("cluster-spoke3")}, codes.OK},
		{"move own gateway to another cluster", gatewaySvc("UpdateGateway"), &pb.UpdateGatewayRequest{Id: "gw-own", ClusterId: strPtr("cluster-other")}, codes.PermissionDenied},
		{"update foreign gateway", gatewaySvc("UpdateGateway"), &pb.UpdateGatewayRequest{Id: "gw-foreign"}, codes.PermissionDenied},
		{"pull foreign gateway onto own cluster", gatewaySvc("UpdateGateway"), &pb.UpdateGatewayRequest{Id: "gw-foreign", ClusterId: strPtr("cluster-spoke3")}, codes.PermissionDenied},
		{"version of own gateway", gatewaySvc("SetGatewayVersion"), &pb.SetGatewayVersionRequest{Id: "gw-own"}, codes.OK},
		{"version of foreign gateway", gatewaySvc("SetGatewayVersion"), &pb.SetGatewayVersionRequest{Id: "gw-foreign"}, codes.PermissionDenied},
		{"adjust own namespace", gatewaySvc("AdjustActiveSandboxCount"), &pb.AdjustActiveSandboxCountRequest{Namespace: "ns-own"}, codes.OK},
		{"adjust foreign namespace", gatewaySvc("AdjustActiveSandboxCount"), &pb.AdjustActiveSandboxCountRequest{Namespace: "ns-foreign"}, codes.PermissionDenied},
		{"set foreign namespace", gatewaySvc("SetActiveSandboxCount"), &pb.SetActiveSandboxCountRequest{Namespace: "ns-foreign"}, codes.PermissionDenied},
		{"adjust namespace without a live gateway (service no-op)", gatewaySvc("AdjustActiveSandboxCount"), &pb.AdjustActiveSandboxCountRequest{Namespace: "ns-gone"}, codes.OK},
		{"create gateway", gatewaySvc("CreateGateway"), &pb.CreateGatewayRequest{}, codes.PermissionDenied},
		{"delete gateway", gatewaySvc("DeleteGateway"), &pb.DeleteGatewayRequest{Id: "gw-own"}, codes.PermissionDenied},
		{"list gateways", gatewaySvc("ListGateways"), &pb.ListGatewaysRequest{}, codes.OK},
		{"delete own cluster record", "/hypershell.v1.ManagedClusterService/DeleteManagedCluster", &pb.DeleteManagedClusterRequest{Id: "cluster-spoke3"}, codes.OK},
		{"delete another cluster record", "/hypershell.v1.ManagedClusterService/DeleteManagedCluster", &pb.DeleteManagedClusterRequest{Id: "cluster-other"}, codes.PermissionDenied},
		{"rename own cluster record", "/hypershell.v1.ManagedClusterService/UpdateManagedCluster", &pb.UpdateManagedClusterRequest{Id: "cluster-spoke3"}, codes.PermissionDenied},
		{"list role bindings", "/hypershell.v1.RoleBindingService/ListRoleBindings", &pb.ListRoleBindingsRequest{}, codes.OK},
		{"unknown service", "/hypershell.v1.SomethingNew/DoIt", nil, codes.PermissionDenied},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called, err := runUnary(t, registeredSpokes(), fakeLookup{}, nil, enforcedWithAllowlist, ctx, tt.method, tt.req)
			if got := status.Code(err); got != tt.want || called != (tt.want == codes.OK) {
				t.Fatalf("handler=%v code=%s, want %s", called, got, tt.want)
			}
		})
	}

	for _, method := range []string{watchGateways, gatewaySvc("WatchGateways"), "/hypershell.v1.ManagedClusterService/WatchManagedClusters"} {
		if called, err := runStream(t, registeredSpokes(), fakeLookup{}, enforcedWithAllowlist, ctx, method); err != nil || !called {
			t.Fatalf("stream %s: handler=%v err=%v", method, called, err)
		}
	}
}

// The allowlist no longer outranks registration: an allowlisted account that
// registered is scoped like any control plane.
func TestAllowlistedAndRegisteredIsScoped(t *testing.T) {
	resolver := &fakeResolver{clusters: map[string]string{"hub-sub": "cluster-spoke3"}}
	ctx := grpcCallerContext(t, hubSA, "hub-sub")
	if called, err := runUnary(t, resolver, fakeLookup{}, nil, enforcedWithAllowlist, ctx, gatewaySvc("SetGatewayVersion"), &pb.SetGatewayVersionRequest{Id: "gw-foreign"}); called || status.Code(err) != codes.PermissionDenied {
		t.Fatalf("registered allowlisted account on a foreign gateway: handler=%v code=%s", called, status.Code(err))
	}
	if called, err := runUnary(t, resolver, fakeLookup{}, nil, enforcedWithAllowlist, ctx, gatewaySvc("SetGatewayVersion"), &pb.SetGatewayVersionRequest{Id: "gw-own"}); err != nil || !called {
		t.Fatalf("registered allowlisted account on its own gateway: handler=%v err=%v", called, err)
	}
}

func TestUnregisteredCallerStillDeniedControlPlaneOnlyMethods(t *testing.T) {
	owner := fakeLookup{bindings: []BindingSummary{ownerOf("gw-own"), creatorBinding}}
	cases := map[string]context.Context{
		"unregistered sub": grpcCallerContext(t, "human-owner", "human-sub"),
		// The token names a registered subject but the auth interceptor set no
		// username: never a control-plane identity.
		"unauthenticated": bearerContext(t, spokeSub),
	}
	reqs := map[string]interface{}{
		controlPlaneOnlyMethods[0]: &pb.AdjustActiveSandboxCountRequest{Namespace: "ns-own"},
		controlPlaneOnlyMethods[1]: &pb.SetActiveSandboxCountRequest{Namespace: "ns-own"},
		controlPlaneOnlyMethods[2]: &pb.SetGatewayVersionRequest{Id: "gw-own"},
	}
	for name, ctx := range cases {
		for _, method := range controlPlaneOnlyMethods {
			t.Run(name+method, func(t *testing.T) {
				called, err := runUnary(t, registeredSpokes(), owner, nil, enforcedWithAllowlist, ctx, method, reqs[method])
				if called || status.Code(err) != codes.PermissionDenied {
					t.Fatalf("unary: handler=%v code=%s, want PermissionDenied", called, status.Code(err))
				}
			})
		}
	}
}

func TestRegisteredClusterLookupErrorIsUnavailable(t *testing.T) {
	failing := &fakeResolver{err: errors.New("db down")}
	ctx := grpcCallerContext(t, spokeUsername, spokeSub)
	called, err := runUnary(t, failing, fakeLookup{}, nil, enforcedWithAllowlist, ctx, gatewaySvc("SetGatewayVersion"), &pb.SetGatewayVersionRequest{Id: "gw-own"})
	if called || status.Code(err) != codes.Unavailable {
		t.Fatalf("unary: handler=%v code=%s, want Unavailable", called, status.Code(err))
	}
	called, err = runStream(t, failing, fakeLookup{}, enforcedWithAllowlist, ctx, watchGateways)
	if called || status.Code(err) != codes.Unavailable {
		t.Fatalf("stream: handler=%v code=%s, want Unavailable", called, status.Code(err))
	}

	// An allowlisted account is resolved too (it may be registered, and then it
	// is scoped), so it also sees Unavailable, which a control plane retries.
	ctx = grpcCallerContext(t, hubSA, "hub-sub")
	if called, err := runUnary(t, failing, fakeLookup{}, nil, enforcedWithAllowlist, ctx, controlPlaneOnlyMethods[0], &pb.AdjustActiveSandboxCountRequest{Namespace: "ns-own"}); called || status.Code(err) != codes.Unavailable {
		t.Fatalf("allowlisted account: handler=%v code=%s, want Unavailable", called, status.Code(err))
	}

	// A failing gateway lookup is Unavailable as well, never a grant.
	interceptor := RBACUnaryInterceptor(fakeLookup{}, fakeProvisioner{userID: "user-1"}, nil, nil, registeredSpokes(), fakeGateways{err: errors.New("db down")}, enforcedWithAllowlist)
	_, err = interceptor(grpcCallerContext(t, spokeUsername, spokeSub), &pb.SetGatewayVersionRequest{Id: "gw-own"}, &grpc.UnaryServerInfo{FullMethod: gatewaySvc("SetGatewayVersion")},
		func(context.Context, interface{}) (interface{}, error) { return nil, nil })
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("gateway lookup failure: %v, want Unavailable", err)
	}
}

func TestNilResolverPreservesAllowlistOnlyBehaviour(t *testing.T) {
	ctx := grpcCallerContext(t, spokeUsername, spokeSub)
	if called, err := runUnary(t, nil, fakeLookup{}, nil, enforcedWithAllowlist, ctx, controlPlaneOnlyMethods[0], &pb.AdjustActiveSandboxCountRequest{Namespace: "ns-own"}); called || status.Code(err) != codes.PermissionDenied {
		t.Fatalf("handler=%v code=%s, want PermissionDenied", called, status.Code(err))
	}
	if called, err := runStream(t, nil, fakeLookup{}, enforcedWithAllowlist, ctx, watchGateways); called || status.Code(err) != codes.PermissionDenied {
		t.Fatalf("stream without bindings: handler=%v code=%s, want PermissionDenied", called, status.Code(err))
	}
	// The allowlist still works on its own.
	ctx = grpcCallerContext(t, hubSA, "hub-sub")
	if called, err := runUnary(t, nil, fakeLookup{}, nil, enforcedWithAllowlist, ctx, controlPlaneOnlyMethods[0], &pb.AdjustActiveSandboxCountRequest{Namespace: "ns-own"}); err != nil || !called {
		t.Fatalf("allowlisted account: handler=%v err=%v", called, err)
	}
}

func TestRegisteredClusterNotRecordedAsDailyActiveGRPC(t *testing.T) {
	const listClusters = "/hypershell.v1.ManagedClusterService/ListManagedClusters"
	creator := fakeLookup{bindings: []BindingSummary{creatorBinding}}
	for _, config := range []AuthzConfig{enforcedWithAllowlist, {EnforceRBAC: false}} {
		recorder := &fakeActivityRecorder{}
		if _, err := runUnary(t, registeredSpokes(), creator, recorder, config, grpcCallerContext(t, spokeUsername, spokeSub), listClusters, &pb.ListManagedClustersRequest{}); err != nil {
			t.Fatalf("enforce=%v: registered call failed: %v", config.EnforceRBAC, err)
		}
		if len(recorder.users) != 0 {
			t.Fatalf("enforce=%v: registered cluster recorded as daily active: %v", config.EnforceRBAC, recorder.users)
		}
		if _, err := runUnary(t, registeredSpokes(), creator, recorder, config, grpcCallerContext(t, "human", "human-sub"), listClusters, &pb.ListManagedClustersRequest{}); err != nil {
			t.Fatalf("enforce=%v: user call failed: %v", config.EnforceRBAC, err)
		}
		if len(recorder.users) != 1 {
			t.Fatalf("enforce=%v: user not recorded: %v", config.EnforceRBAC, recorder.users)
		}
	}

	// With RBAC not enforced a lookup failure must not fail the call; it only
	// suppresses the record.
	recorder := &fakeActivityRecorder{}
	called, err := runUnary(t, &fakeResolver{err: errors.New("db down")}, creator, recorder, AuthzConfig{}, grpcCallerContext(t, spokeUsername, spokeSub), listClusters, &pb.ListManagedClustersRequest{})
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

// REST self-deregistration: a registered control plane may delete its own
// ManagedCluster record and no other; other records need platform:admin, and
// gateway:creator (every signed-in user on the GitOps hubs) is not enough.
func TestAuthorizeApiManagedClusterDelete(t *testing.T) {
	const route = "/api/hypershell/v1/managed_clusters/{id}"
	config := AuthzConfig{EnforceRBAC: true, ServiceAccounts: []string{hubSA}}
	creator := authorizationLookup{bindings: []BindingSummary{creatorBinding}}
	admin := authorizationLookup{bindings: []BindingSummary{adminBinding}}
	human := jwt.MapClaims{"preferred_username": "human", "sub": "human-sub"}

	tests := []struct {
		name     string
		resolver RegisteredClusterResolver
		lookup   RoleBindingLookup
		claims   jwt.MapClaims
		id       string
		want     int
	}{
		{"spoke deletes its own record", registeredSpokes(), creator, spokeClaims(), "cluster-spoke3", http.StatusOK},
		{"spoke without bindings deletes its own record", registeredSpokes(), authorizationLookup{}, spokeClaims(), "cluster-spoke3", http.StatusOK},
		{"spoke deletes another record", registeredSpokes(), creator, spokeClaims(), "cluster-other", http.StatusForbidden},
		{"creator deletes a record", registeredSpokes(), creator, human, "cluster-spoke3", http.StatusForbidden},
		{"platform:admin deletes a record", registeredSpokes(), admin, human, "cluster-spoke3", http.StatusOK},
		{"lookup failure", &fakeResolver{err: errors.New("db down")}, creator, spokeClaims(), "cluster-spoke3", http.StatusServiceUnavailable},
		{"no resolver: admin only", nil, creator, spokeClaims(), "cluster-spoke3", http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reached, code := serveREST(t, tt.resolver, tt.lookup, nil, config, http.MethodDelete, route, "/api/hypershell/v1/managed_clusters/"+tt.id, tt.claims)
			if code != tt.want || reached != (tt.want == http.StatusOK) {
				t.Fatalf("reached=%v status=%d, want %d", reached, code, tt.want)
			}
		})
	}
}

func TestAuthorizeApiManagedClusterWrites(t *testing.T) {
	config := AuthzConfig{EnforceRBAC: true}
	creator := authorizationLookup{bindings: []BindingSummary{creatorBinding}}
	admin := authorizationLookup{bindings: []BindingSummary{adminBinding}}
	human := jwt.MapClaims{"preferred_username": "human", "sub": "human-sub"}

	for _, tc := range []struct {
		method, route, path string
	}{
		{http.MethodPost, "/api/hypershell/v1/managed_clusters", "/api/hypershell/v1/managed_clusters"},
		{http.MethodPatch, "/api/hypershell/v1/managed_clusters/{id}", "/api/hypershell/v1/managed_clusters/c1"},
	} {
		if reached, code := serveREST(t, registeredSpokes(), creator, nil, config, tc.method, tc.route, tc.path, human); reached || code != http.StatusForbidden {
			t.Fatalf("creator %s %s: reached=%v status=%d, want 403", tc.method, tc.path, reached, code)
		}
		if reached, code := serveREST(t, registeredSpokes(), creator, nil, config, tc.method, tc.route, tc.path, spokeClaims()); reached || code != http.StatusForbidden {
			t.Fatalf("spoke %s %s: reached=%v status=%d, want 403", tc.method, tc.path, reached, code)
		}
		if reached, code := serveREST(t, registeredSpokes(), admin, nil, config, tc.method, tc.route, tc.path, human); !reached || code != http.StatusOK {
			t.Fatalf("admin %s %s: reached=%v status=%d, want 200", tc.method, tc.path, reached, code)
		}
	}
	// Reads stay open to gateway:creator.
	if reached, code := serveREST(t, registeredSpokes(), creator, nil, config, http.MethodGet, "/api/hypershell/v1/managed_clusters/{id}", "/api/hypershell/v1/managed_clusters/c1", human); !reached || code != http.StatusOK {
		t.Fatalf("creator GET: reached=%v status=%d", reached, code)
	}
}
