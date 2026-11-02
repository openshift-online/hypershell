package rbac

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Gateway field-level write policy (data-model.spec.md "Requirement: Gateway Field
// Ownership and Write Policy" PLAT-DATA-23; security/rbac-enforcement.spec.md
// "Requirement: Gateway Field Write Policy" SEC-RBAC-18). Every gateway write funnels
// through the gateway service (service.Create / service.Replace). Because REST, gRPC,
// and the declarative .hypershell import all reach the same funnel, the authorization
// layer marks each write request in its context with one of three caller kinds, and
// the service enforces the default-deny field allowlist from that marker.

// GatewayWriterKind identifies who authored a gateway write, for field-policy
// purposes.
type GatewayWriterKind int

const (
	// GatewayWriterUser is a normal authenticated REST or gRPC caller (including
	// hsctl apply). It is the default for an unmarked context, so an unclassified
	// write is treated as the most restricted caller (fail-safe, default-deny).
	GatewayWriterUser GatewayWriterKind = iota
	// GatewayWriterImport is a control-plane gRPC request carrying the reconciled
	// Repository id in trusted metadata (the declarative .hypershell import). It is a
	// NON-privileged writer: for field-policy purposes it is held to the same
	// allowlist as a direct user.
	GatewayWriterImport
	// GatewayWriterControlPlane is a control-plane gRPC request WITHOUT import
	// metadata (the internal reconcile write-back path). It alone retains full write
	// access to platform-managed and observed gateway fields.
	GatewayWriterControlPlane
)

// String renders the caller kind for error messages and logs.
func (k GatewayWriterKind) String() string {
	switch k {
	case GatewayWriterImport:
		return "import"
	case GatewayWriterControlPlane:
		return "control-plane"
	default:
		return "user"
	}
}

// Privileged reports whether the caller kind may write platform-managed and observed
// gateway fields. Only the control-plane internal reconcile path is privileged.
func (k GatewayWriterKind) Privileged() bool {
	return k == GatewayWriterControlPlane
}

type gatewayWriterKindKey struct{}

// WithGatewayWriterKind returns a context marked with the gateway writer kind. The
// gRPC interceptor sets it for every gateway write; the API server's own internal
// writes (for example late-binding placement rebind) set GatewayWriterControlPlane
// explicitly.
func WithGatewayWriterKind(ctx context.Context, kind GatewayWriterKind) context.Context {
	return context.WithValue(ctx, gatewayWriterKindKey{}, kind)
}

// GatewayWriterKindFromContext returns the gateway writer kind marked on the context,
// defaulting to GatewayWriterUser (the most restricted, non-privileged caller) when no
// marker is present. The default is deliberate: an unmarked write (for example a direct
// REST caller, which is always a user) is never granted platform-managed field access.
func GatewayWriterKindFromContext(ctx context.Context) GatewayWriterKind {
	if v, ok := ctx.Value(gatewayWriterKindKey{}).(GatewayWriterKind); ok {
		return v
	}
	return GatewayWriterUser
}

// WithResolvedGatewayWriterKind resolves the gateway writer kind for a gRPC caller and
// returns a context marked with it. It is invoked from the gateway write handlers
// (CreateGateway / UpdateGateway) so the marker is present for every gateway write path
// regardless of which authz interceptor chain a deployment installs. The control-plane
// identity is read from the caller's subject: the framework's gRPC auth interceptor has
// already verified the token signature upstream when auth is enabled, so the subject is
// authentic; when auth is off there is no boundary to protect.
//
// A resolver lookup failure is returned as a retryable codes.Unavailable, NOT swallowed:
// silently degrading a possible control-plane caller to the restricted user kind would
// turn a transient database blip into a terminal PermissionDenied on a legitimate
// control-plane write-back. This mirrors isRegisteredClusterCallerGRPC, which returns
// Unavailable for the same lookup so a control plane retries rather than fails closed.
func WithResolvedGatewayWriterKind(ctx context.Context, clusters RegisteredClusterResolver) (context.Context, error) {
	// An import write is identified by metadata alone, so it short-circuits the
	// registered-cluster lookup: an import is held to the user allowlist regardless, and
	// skipping the lookup avoids a per-write database query on the import path.
	if ImportRepositoryIDFromContext(ctx) != "" {
		return WithGatewayWriterKind(ctx, GatewayWriterImport), nil
	}
	registered, err := registeredClusterCaller(ctx, clusters, subjectFromGRPCContext(ctx))
	if err != nil {
		return ctx, status.Error(codes.Unavailable, "managed cluster lookup failed; cannot classify the gateway writer")
	}
	return WithGatewayWriterKind(ctx, ResolveGatewayWriterKind(ctx, registered)), nil
}

// ResolveGatewayWriterKind derives the writer kind for a gRPC caller. An import write
// is identified by the reconciled-Repository metadata (the same signal that binds
// import ownership); a write from a registered control-plane identity without that
// metadata is the internal reconcile path; every other caller is a user. The import
// check comes first so an import write is never mistaken for a privileged control-plane
// write even though both authenticate as the control plane.
func ResolveGatewayWriterKind(ctx context.Context, registeredControlPlane bool) GatewayWriterKind {
	if ImportRepositoryIDFromContext(ctx) != "" {
		return GatewayWriterImport
	}
	if registeredControlPlane {
		return GatewayWriterControlPlane
	}
	return GatewayWriterUser
}
