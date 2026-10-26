package rbac

import (
	"context"
	"errors"
	"testing"

	"github.com/golang-jwt/jwt/v5"
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

// fakeImportReadRequest carries both a cluster_id and a repository_id filter, like
// the generated ListGatewaysRequest / ListAgentRuntimesRequest, for exercising the
// import-scoped read exemption.
type fakeImportReadRequest struct {
	clusterID    string
	repositoryID string
}

func (r *fakeImportReadRequest) GetClusterId() string    { return r.clusterID }
func (r *fakeImportReadRequest) GetRepositoryId() string { return r.repositoryID }

// importBearerContext is a registered-control-plane bearer context that also carries
// the reconciled-Repository import binding metadata.
func importBearerContext(t *testing.T, sub, repositoryID string) context.Context {
	t.Helper()
	ctx := bearerContext(t, sub)
	md, _ := metadata.FromIncomingContext(ctx)
	md = md.Copy()
	md.Set(ImportRepositoryMetadataKey, repositoryID)
	return metadata.NewIncomingContext(ctx, md)
}

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
				err := CheckClusterCallerBindingUnary(tc.ctx, resolver, nil, tc.method, tc.req)
				if got := status.Code(err); got != tc.wantCode {
					t.Fatalf("code = %s (%v), want %s", got, err, tc.wantCode)
				}
			})
		}
	}
}

// TestImportScopedReadExemption pins the B-narrowed exemption: the import
// reconciler's Repository-scoped list read spans clusters ONLY when it carries the
// reconciled-Repository binding, sets the matching repository_id filter, and names
// an existing Repository. Every near-miss falls back to the own-cluster binding or
// fails closed.
func TestImportScopedReadExemption(t *testing.T) {
	resolver := &fakeResolver{clusters: map[string]string{"cp-sub": "cluster-x"}}
	imports := fakeImportResolver{known: map[string]bool{"repo-1": true}}

	for _, method := range []string{listGatewaysMethod, listAgentRuntimesMethod} {
		t.Run(method+"/import read spanning clusters is allowed", func(t *testing.T) {
			// repository_id filter set, no cluster_id, binding names an existing repo.
			req := &fakeImportReadRequest{repositoryID: "repo-1"}
			if err := CheckClusterCallerBindingUnary(importBearerContext(t, "cp-sub", "repo-1"), resolver, imports, method, req); err != nil {
				t.Fatalf("want allowed, got %v", err)
			}
		})
		t.Run(method+"/filter not matching the binding falls back to own-cluster", func(t *testing.T) {
			// Binding repo-1 but filter repo-2: not the exemption, so the own-cluster
			// check applies and an empty cluster_id is InvalidArgument.
			req := &fakeImportReadRequest{repositoryID: "repo-2"}
			if got := status.Code(CheckClusterCallerBindingUnary(importBearerContext(t, "cp-sub", "repo-1"), resolver, imports, method, req)); got != codes.InvalidArgument {
				t.Fatalf("code = %s, want InvalidArgument", got)
			}
		})
		t.Run(method+"/binding naming an unknown repository is denied", func(t *testing.T) {
			req := &fakeImportReadRequest{repositoryID: "ghost"}
			if got := status.Code(CheckClusterCallerBindingUnary(importBearerContext(t, "cp-sub", "ghost"), resolver, imports, method, req)); got != codes.PermissionDenied {
				t.Fatalf("code = %s, want PermissionDenied", got)
			}
		})
		t.Run(method+"/no repository_id filter keeps own-cluster binding", func(t *testing.T) {
			// Import binding present but no repository_id filter: a plain cross-cluster
			// list is still refused (empty cluster_id -> InvalidArgument).
			req := &fakeImportReadRequest{}
			if got := status.Code(CheckClusterCallerBindingUnary(importBearerContext(t, "cp-sub", "repo-1"), resolver, imports, method, req)); got != codes.InvalidArgument {
				t.Fatalf("code = %s, want InvalidArgument", got)
			}
		})
		t.Run(method+"/nil import resolver fails closed", func(t *testing.T) {
			req := &fakeImportReadRequest{repositoryID: "repo-1"}
			if got := status.Code(CheckClusterCallerBindingUnary(importBearerContext(t, "cp-sub", "repo-1"), resolver, nil, method, req)); got != codes.Unavailable {
				t.Fatalf("code = %s, want Unavailable", got)
			}
		})
	}

	t.Run("a non-cluster-bound caller never reaches the exemption", func(t *testing.T) {
		// A user subject is not a registered cluster, so the binding does not apply at
		// all even with a repository_id filter present.
		req := &fakeImportReadRequest{repositoryID: "repo-1"}
		if err := CheckClusterCallerBindingUnary(importBearerContext(t, "user-sub", "repo-1"), resolver, imports, listGatewaysMethod, req); err != nil {
			t.Fatalf("want allowed, got %v", err)
		}
	})
}

// ListGatewaysRequest and ListAgentRuntimesRequest must expose GetRepositoryId so
// the import-scoped read exemption can read the filter off the generated messages.
func TestImportScopedRequestsCarryRepositoryID(t *testing.T) {
	for name, req := range map[string]interface{}{
		"ListGatewaysRequest":      &pb.ListGatewaysRequest{},
		"ListAgentRuntimesRequest": &pb.ListAgentRuntimesRequest{},
	} {
		if _, ok := req.(repositoryScopedRequest); !ok {
			t.Errorf("%s does not implement GetRepositoryId", name)
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

// Verifies: PLAT-CLUS-11.04
func TestCheckClusterCallerBindingFailsClosedOnLookupError(t *testing.T) {
	resolver := &fakeResolver{err: errors.New("db down")}
	err := CheckClusterCallerBindingUnary(bearerContext(t, "cp-sub"), resolver, nil, listGatewaysMethod, &fakeScopedRequest{"cluster-x"})
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("code = %s, want Unavailable", status.Code(err))
	}
	if err := CheckClusterCallerBindingUnary(bearerContext(t, "cp-sub"), nil, nil, listGatewaysMethod, &fakeScopedRequest{"cluster-x"}); status.Code(err) != codes.Unavailable {
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

// Verifies: PLAT-CLUS-10.04, PLAT-CLUS-10.05
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
	ss, err := BindClusterCallerStream(base, "/hypershell.v1.SomethingUnknownService/WatchSomething", resolver)
	if err != nil || ss != grpc.ServerStream(base) || resolver.calls != before {
		t.Fatalf("non-bound method: stream wrapped or resolver called (err=%v calls=%d)", err, resolver.calls-before)
	}
}
