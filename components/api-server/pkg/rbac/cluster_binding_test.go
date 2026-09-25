package rbac

import (
	"context"
	"errors"
	"testing"

	"github.com/golang-jwt/jwt/v4"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
)

type fakeResolver struct {
	clusters map[string]string
	err      error
	calls    int
}

func (f *fakeResolver) RegisteredClusterIDForSubject(_ context.Context, subject string) (string, bool, error) {
	f.calls++
	if f.err != nil {
		return "", false, f.err
	}
	id, ok := f.clusters[subject]
	return id, ok, nil
}

type fakeScopedRequest struct{ clusterID string }

func (r *fakeScopedRequest) GetClusterId() string { return r.clusterID }

func bearerContext(t *testing.T, sub string) context.Context {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"sub": sub, "preferred_username": "service-account-hypershell-control-plane"})
	signed, err := tok.SignedString([]byte("test-key"))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "Bearer "+signed))
}

func TestCheckClusterCallerBindingUnary(t *testing.T) {
	resolver := &fakeResolver{clusters: map[string]string{"cp-sub": "cluster-x"}}
	for _, method := range []string{listGatewaysMethod, listRoleBindingsMethod} {
		cases := []struct {
			name     string
			ctx      context.Context
			method   string
			req      interface{}
			wantCode codes.Code
		}{
			{name: "own cluster allowed", ctx: bearerContext(t, "cp-sub"), method: method, req: &fakeScopedRequest{"cluster-x"}, wantCode: codes.OK},
			{name: "foreign cluster denied", ctx: bearerContext(t, "cp-sub"), method: method, req: &fakeScopedRequest{"cluster-y"}, wantCode: codes.PermissionDenied},
			{name: "missing cluster invalid", ctx: bearerContext(t, "cp-sub"), method: method, req: &fakeScopedRequest{""}, wantCode: codes.InvalidArgument},
			{name: "user caller unaffected", ctx: bearerContext(t, "user-sub"), method: method, req: &fakeScopedRequest{""}, wantCode: codes.OK},
			{name: "anonymous caller unaffected", ctx: context.Background(), method: method, req: &fakeScopedRequest{""}, wantCode: codes.OK},
			{name: "other method unaffected", ctx: bearerContext(t, "cp-sub"), method: "/hypershell.v1.GatewayService/GetGateway", req: &fakeScopedRequest{""}, wantCode: codes.OK},
		}
		for _, tc := range cases {
			t.Run(method+"/"+tc.name, func(t *testing.T) {
				err := CheckClusterCallerBindingUnary(tc.ctx, resolver, tc.method, tc.req)
				if got := status.Code(err); got != tc.wantCode {
					t.Fatalf("code = %s (%v), want %s", got, err, tc.wantCode)
				}
			})
		}
	}
}

// The request messages the bound methods receive must expose GetClusterId, or
// checkRequestCluster rejects every registered caller with Internal.
func TestClusterBoundRequestsCarryClusterID(t *testing.T) {
	for name, req := range map[string]interface{}{
		"WatchGatewaysRequest":     &pb.WatchGatewaysRequest{},
		"ListGatewaysRequest":      &pb.ListGatewaysRequest{},
		"WatchRoleBindingsRequest": &pb.WatchRoleBindingsRequest{},
		"ListRoleBindingsRequest":  &pb.ListRoleBindingsRequest{},
	} {
		if _, ok := req.(clusterScopedRequest); !ok {
			t.Errorf("%s does not implement GetClusterId", name)
		}
	}
	for _, method := range []string{watchGatewaysMethod, listGatewaysMethod, watchRoleBindingsMethod, listRoleBindingsMethod} {
		if !isClusterBoundMethod(method) {
			t.Errorf("%s is not cluster bound", method)
		}
	}
}

func TestCheckClusterCallerBindingFailsClosedOnLookupError(t *testing.T) {
	resolver := &fakeResolver{err: errors.New("db down")}
	err := CheckClusterCallerBindingUnary(bearerContext(t, "cp-sub"), resolver, listGatewaysMethod, &fakeScopedRequest{"cluster-x"})
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("code = %s, want Unavailable", status.Code(err))
	}
	if err := CheckClusterCallerBindingUnary(bearerContext(t, "cp-sub"), nil, listGatewaysMethod, &fakeScopedRequest{"cluster-x"}); status.Code(err) != codes.Unavailable {
		t.Fatalf("nil resolver: code = %s, want Unavailable", status.Code(err))
	}
}

type fakeServerStream struct {
	grpc.ServerStream
	ctx  context.Context
	msg  *fakeScopedRequest
	recv int
}

func (s *fakeServerStream) Context() context.Context { return s.ctx }

func (s *fakeServerStream) RecvMsg(m interface{}) error {
	s.recv++
	m.(*fakeScopedRequest).clusterID = s.msg.clusterID
	return nil
}

func TestBindClusterCallerStream(t *testing.T) {
	resolver := &fakeResolver{clusters: map[string]string{"cp-sub": "cluster-x"}}
	cases := []struct {
		name     string
		sub      string
		cluster  string
		wantCode codes.Code
		wrapped  bool
	}{
		{name: "own cluster allowed", sub: "cp-sub", cluster: "cluster-x", wantCode: codes.OK, wrapped: true},
		{name: "foreign cluster denied", sub: "cp-sub", cluster: "cluster-y", wantCode: codes.PermissionDenied, wrapped: true},
		{name: "missing cluster invalid", sub: "cp-sub", cluster: "", wantCode: codes.InvalidArgument, wrapped: true},
		{name: "user caller passes through", sub: "user-sub", cluster: "", wantCode: codes.OK, wrapped: false},
	}
	for _, method := range []string{watchGatewaysMethod, watchRoleBindingsMethod} {
		for _, tc := range cases {
			t.Run(method+"/"+tc.name, func(t *testing.T) {
				base := &fakeServerStream{ctx: bearerContext(t, tc.sub), msg: &fakeScopedRequest{tc.cluster}}
				ss, err := BindClusterCallerStream(base, method, resolver)
				if err != nil {
					t.Fatalf("BindClusterCallerStream: %v", err)
				}
				if _, isWrapped := ss.(*clusterBoundServerStream); isWrapped != tc.wrapped {
					t.Fatalf("wrapped = %v, want %v", isWrapped, tc.wrapped)
				}
				err = ss.RecvMsg(&fakeScopedRequest{})
				if got := status.Code(err); got != tc.wantCode {
					t.Fatalf("RecvMsg code = %s (%v), want %s", got, err, tc.wantCode)
				}
				// Only the first message (the request) is checked.
				if tc.wantCode == codes.OK {
					if err := ss.RecvMsg(&fakeScopedRequest{}); err != nil {
						t.Fatalf("second RecvMsg: %v", err)
					}
				}
			})
		}
	}

	// Non-bound methods are never wrapped and never resolve the caller.
	before := resolver.calls
	base := &fakeServerStream{ctx: bearerContext(t, "cp-sub"), msg: &fakeScopedRequest{}}
	ss, err := BindClusterCallerStream(base, "/hypershell.v1.GatewayReleaseService/WatchGatewayReleases", resolver)
	if err != nil || ss != grpc.ServerStream(base) || resolver.calls != before {
		t.Fatalf("non-bound method: stream wrapped or resolver called (err=%v calls=%d)", err, resolver.calls-before)
	}
}
