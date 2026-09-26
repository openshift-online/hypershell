package rbac

import (
	"context"

	"github.com/golang-jwt/jwt/v4"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/openshift-online/rh-trex-ai/pkg/auth"
)

// Control-Plane Identity (managed-cluster-registration.spec.md): a caller whose
// JWT subject is the oidc_subject of a registered ManagedCluster is a control
// plane, and is treated like an account on the RBAC_SERVICE_ACCOUNTS allowlist:
// on gRPC it bypasses role-binding authorization, including the
// control-plane-only methods (see isServiceAccountOnlyMethod), and on either
// transport it is never recorded as a daily-active user. Registration itself
// stays gated by the managed-cluster-registrar JWT role, so a control plane
// becomes a control-plane identity by registering, with no hub-side
// configuration. RBAC_SERVICE_ACCOUNTS remains the bootstrap allowlist for
// callers that have not registered (or that run without the managedClusters
// plugin).
//
// On the REST path neither an allowlisted account nor a registered cluster
// bypasses role bindings: the only REST call a control plane makes is
// registration, and the HTTP isolation guarantee in rbac-enforcement.spec.md
// (a spoke credential holds no REST gateway or user-inventory access unless
// Keycloak grants it) must keep holding after the spoke registers.
//
// TODO(cluster-scoped control-plane writes): AdjustActiveSandboxCount and
// SetActiveSandboxCount identify the gateway by namespace, SetGatewayVersion by
// gateway id; neither carries cluster_id. Scoping them to the caller's own
// cluster needs a gateway -> cluster_id lookup, which the rbac package has no
// dependency on today (only the subject -> cluster resolver). Until that lookup
// is added, a registered control plane may call these methods for any gateway,
// exactly as an allowlisted account can.

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
