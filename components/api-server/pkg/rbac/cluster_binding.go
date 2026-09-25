package rbac

import (
	"context"
	"strings"

	"github.com/golang-jwt/jwt/v4"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/openshift-online/rh-trex-ai/pkg/auth"
)

// Watch Stream Caller Binding (managed-cluster-registration.spec.md): a caller
// whose JWT subject is a registered ManagedCluster may list or watch gateways,
// and the role bindings of gateways, only for its own cluster. Every control
// plane is registered, so this turns the cluster_id filter on
// WatchGateways/ListGateways and WatchRoleBindings/ListRoleBindings from
// cooperative scoping into an enforced boundary (role bindings are scoped by
// their gateway's cluster_id). Callers whose subject is not a registered
// cluster (users, hsctl) are unaffected and continue through role-binding
// authorization.

const (
	watchGatewaysMethod     = "/hypershell.v1.GatewayService/WatchGateways"
	listGatewaysMethod      = "/hypershell.v1.GatewayService/ListGateways"
	watchRoleBindingsMethod = "/hypershell.v1.RoleBindingService/WatchRoleBindings"
	listRoleBindingsMethod  = "/hypershell.v1.RoleBindingService/ListRoleBindings"
)

// RegisteredClusterResolver maps an OIDC subject to the id of the ManagedCluster
// a control plane registered under it. found is false when the subject never
// registered.
type RegisteredClusterResolver interface {
	RegisteredClusterIDForSubject(ctx context.Context, subject string) (clusterID string, found bool, err error)
}

// clusterScopedRequest is implemented by the generated WatchGatewaysRequest,
// ListGatewaysRequest, WatchRoleBindingsRequest and ListRoleBindingsRequest
// messages.
type clusterScopedRequest interface {
	GetClusterId() string
}

func isClusterBoundMethod(fullMethod string) bool {
	switch fullMethod {
	case watchGatewaysMethod, listGatewaysMethod, watchRoleBindingsMethod, listRoleBindingsMethod:
		return true
	}
	return false
}

// CheckClusterCallerBindingUnary enforces the binding on a unary call
// (ListGateways, ListRoleBindings). It runs before role-binding authorization and regardless of
// the RBAC_SERVICE_ACCOUNTS allowlist.
func CheckClusterCallerBindingUnary(ctx context.Context, resolver RegisteredClusterResolver, fullMethod string, req interface{}) error {
	if !isClusterBoundMethod(fullMethod) {
		return nil
	}
	clusterID, bound, err := callerRegisteredCluster(ctx, resolver)
	if err != nil || !bound {
		return err
	}
	return checkRequestCluster(req, clusterID)
}

// BindClusterCallerStream enforces the binding on a server-streaming call
// (WatchGateways, WatchRoleBindings). The request message is only available once the handler reads
// it, so for a caller that is a registered cluster the returned stream checks
// the first received message: the generated handler receives the request before
// it calls the handler, so a rejection is returned from RecvMsg before the
// handler subscribes to the event broker. For any other method or caller the
// original stream is returned unchanged.
func BindClusterCallerStream(ss grpc.ServerStream, fullMethod string, resolver RegisteredClusterResolver) (grpc.ServerStream, error) {
	if !isClusterBoundMethod(fullMethod) {
		return ss, nil
	}
	clusterID, bound, err := callerRegisteredCluster(ss.Context(), resolver)
	if err != nil {
		return nil, err
	}
	if !bound {
		return ss, nil
	}
	return &clusterBoundServerStream{ServerStream: ss, clusterID: clusterID}, nil
}

type clusterBoundServerStream struct {
	grpc.ServerStream
	clusterID string
	checked   bool
}

func (s *clusterBoundServerStream) RecvMsg(m interface{}) error {
	if err := s.ServerStream.RecvMsg(m); err != nil {
		return err
	}
	if s.checked {
		return nil
	}
	s.checked = true
	return checkRequestCluster(m, s.clusterID)
}

// callerRegisteredCluster resolves the caller's JWT subject to its registered
// cluster id. bound is false for an anonymous caller or a subject that is not a
// registered cluster. A lookup failure fails closed.
func callerRegisteredCluster(ctx context.Context, resolver RegisteredClusterResolver) (string, bool, error) {
	subject := subjectFromGRPCContext(ctx)
	if subject == "" {
		return "", false, nil
	}
	if resolver == nil {
		return "", false, status.Error(codes.Unavailable, "managed cluster registry unavailable; cannot verify caller cluster binding")
	}
	clusterID, found, err := resolver.RegisteredClusterIDForSubject(ctx, subject)
	if err != nil {
		return "", false, status.Error(codes.Unavailable, "managed cluster lookup failed; cannot verify caller cluster binding")
	}
	if !found || clusterID == "" {
		return "", false, nil
	}
	return clusterID, true, nil
}

func checkRequestCluster(req interface{}, clusterID string) error {
	scoped, ok := req.(clusterScopedRequest)
	if !ok {
		return status.Error(codes.Internal, "cluster-bound request does not carry cluster_id")
	}
	requested := scoped.GetClusterId()
	if requested == "" {
		return status.Error(codes.InvalidArgument, "cluster_id is required: this caller is a registered managed cluster and must scope the request to its own cluster_id")
	}
	if requested != clusterID {
		return status.Error(codes.PermissionDenied, "cluster_id does not match the caller's registered managed cluster")
	}
	return nil
}

// subjectFromGRPCContext returns the caller's JWT "sub" claim. The framework's
// gRPC auth interceptor, which runs before the post-auth interceptors, verifies
// the bearer token's signature but stores only the username in the context, so
// the claim is re-read here from the same authorization metadata without
// re-verifying. That is safe: this function only ever narrows access (a subject
// that resolves to a registered cluster restricts the caller to that cluster's
// id), it never grants any.
func subjectFromGRPCContext(ctx context.Context) string {
	if token, err := auth.TokenFromContext(ctx); err == nil && token != nil {
		if claims, ok := token.Claims.(jwt.MapClaims); ok {
			if sub, _ := claims["sub"].(string); sub != "" {
				return sub
			}
		}
	}
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	values := md.Get("authorization")
	if len(values) == 0 {
		return ""
	}
	raw := values[0]
	tokenStr := strings.TrimPrefix(strings.TrimPrefix(raw, "Bearer "), "bearer ")
	if tokenStr == raw || tokenStr == "" {
		return ""
	}
	parsed, _, err := jwt.NewParser().ParseUnverified(tokenStr, jwt.MapClaims{})
	if err != nil {
		return ""
	}
	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return ""
	}
	sub, _ := claims["sub"].(string)
	return sub
}
