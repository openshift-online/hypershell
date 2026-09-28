package rbac

import (
	"context"

	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/openshift-online/rh-trex-ai/pkg/auth"
)

// Control-Plane Identity (managed-cluster-registration.spec.md): a caller whose
// JWT subject is the oidc_subject of a registered ManagedCluster is the control
// plane of that cluster. On gRPC it is authorized by authorizeControlPlaneGRPC
// (grpc_authorization.go), not by role bindings: it may read the fleet-wide
// records it reconciles from, write only gateways assigned to its own cluster
// (including the control-plane-only sandbox-count and runtime-version writes),
// write only the status of releases and networks, and delete only its own
// ManagedCluster record. On either transport it is never recorded as a
// daily-active user. Registration itself stays gated by the
// managed-cluster-registrar JWT role, so a control plane becomes a
// control-plane identity by registering, with no hub-side configuration.
// RBAC_SERVICE_ACCOUNTS remains the bootstrap allowlist for callers that have
// not registered; it does not cover ManagedCluster record writes.
//
// On the REST path neither an allowlisted account nor a registered cluster
// bypasses role bindings. The one REST exception is self-deregistration: a
// registered caller may DELETE its own ManagedCluster record (authorization.go).

// registeredClusterCaller reports whether subject is the OIDC subject of a
// registered ManagedCluster. A nil resolver or an empty subject reports false
// without error: a deployment without the managedClusters plugin keeps the
// allowlist-only behaviour. The resolver's error is returned unchanged.
func registeredClusterCaller(ctx context.Context, resolver RegisteredClusterResolver, subject string) (bool, error) {
	if resolver == nil || subject == "" {
		return false, nil
	}
	clusterID, found, err := resolver.RegisteredClusterIDForSubject(ctx, subject)
	if err != nil {
		return false, err
	}
	return found && clusterID != "", nil
}

// isRegisteredClusterCallerGRPC reports whether the authenticated gRPC caller
// is a registered managed cluster. The exemption is granted only when the
// framework's auth interceptor accepted the call and set a username: that
// interceptor parses the same authorization metadata value that
// subjectFromGRPCContext reads, so the subject carries exactly the trust the
// username (and therefore the RBAC_SERVICE_ACCOUNTS allowlist) does. An
// unauthenticated call (JWT disabled) is never a control-plane identity.
//
// A lookup failure is codes.Unavailable, never a grant and never
// PermissionDenied: a control plane treats Unavailable as retryable, so a
// transient database error delays it instead of failing it closed for good.
func isRegisteredClusterCallerGRPC(ctx context.Context, username string, resolver RegisteredClusterResolver) (bool, error) {
	if username == "" || resolver == nil {
		return false, nil
	}
	registered, err := registeredClusterCaller(ctx, resolver, subjectFromGRPCContext(ctx))
	if err != nil {
		return false, status.Error(codes.Unavailable, "managed cluster lookup failed; cannot verify control-plane identity")
	}
	return registered, nil
}

// registeredClusterIDGRPC returns the id of the ManagedCluster the
// authenticated gRPC caller registered, under the same conditions as
// isRegisteredClusterCallerGRPC.
func registeredClusterIDGRPC(ctx context.Context, username string, resolver RegisteredClusterResolver) (string, bool, error) {
	if username == "" || resolver == nil {
		return "", false, nil
	}
	subject := subjectFromGRPCContext(ctx)
	if subject == "" {
		return "", false, nil
	}
	clusterID, found, err := resolver.RegisteredClusterIDForSubject(ctx, subject)
	if err != nil {
		return "", false, status.Error(codes.Unavailable, "managed cluster lookup failed; cannot verify control-plane identity")
	}
	if !found || clusterID == "" {
		return "", false, nil
	}
	return clusterID, true, nil
}

// subjectFromVerifiedToken returns the "sub" claim of the JWT the framework's
// HTTP jwtHandler validated and stored in the request context, or "" when
// there is none.
func subjectFromVerifiedToken(ctx context.Context) string {
	token, err := auth.TokenFromContext(ctx)
	if err != nil || token == nil {
		return ""
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return ""
	}
	sub, _ := claims["sub"].(string)
	return sub
}
