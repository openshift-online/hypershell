package auth

import (
	"context"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// claimsKey is the context key type for verified gateway claims.
type claimsKey struct{}

// ClaimsFromContext returns the claims verified by UnaryInterceptor for the
// current call.
func ClaimsFromContext(ctx context.Context) (Claims, bool) {
	claims, ok := ctx.Value(claimsKey{}).(Claims)
	return claims, ok
}

// UnaryInterceptor returns a grpc.UnaryServerInterceptor that authenticates
// every call with the gateway's extension bearer token. The gateway attaches
// "authorization: Bearer <token>" metadata to each RPC on the Unix socket
// (openshell-extension-core auth.rs); anything else is rejected before the
// handler runs.
func UnaryInterceptor(verifier *Verifier) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		_ *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		token, err := bearerToken(ctx)
		if err != nil {
			return nil, err
		}
		claims, err := verifier.Verify(token)
		if err != nil {
			return nil, status.Errorf(codes.Unauthenticated, "gateway credential rejected: %v", err)
		}
		return handler(context.WithValue(ctx, claimsKey{}, claims), req)
	}
}

// bearerToken extracts the single Bearer credential from incoming metadata.
func bearerToken(ctx context.Context) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", status.Error(codes.Unauthenticated, "no request metadata")
	}
	values := md.Get("authorization")
	if len(values) != 1 {
		return "", status.Error(codes.Unauthenticated, "expected exactly one authorization header")
	}
	const scheme = "Bearer "
	if !strings.HasPrefix(values[0], scheme) {
		return "", status.Error(codes.Unauthenticated, "authorization header is not a Bearer token")
	}
	token := strings.TrimPrefix(values[0], scheme)
	if token == "" {
		return "", status.Error(codes.Unauthenticated, "authorization header carries no token")
	}
	return token, nil
}
