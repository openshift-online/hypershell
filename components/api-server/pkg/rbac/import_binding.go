package rbac

import (
	"context"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// Declarative-import write binding (rbac-enforcement.spec.md "gRPC Authorization",
// "Declarative-import writes"; control-plane.spec.md "Declarative `.hypershell`
// Import Reconciliation"). The control-plane import reconciler is the single bounded
// path by which the control plane writes HyperShell API desired state. Its gRPC
// writes to the owned import ext kinds are authorized only when ALL of the following
// hold:
//
//  1. the caller is a registered control-plane identity (its JWT subject resolves to
//     a ManagedCluster) - NOT merely a bootstrap-service-account allowlist entry;
//  2. the write carries the reconciled Repository's id in trusted gRPC metadata
//     (ImportRepositoryMetadataKey), distinct from any repository_id field in the
//     request body; and
//  3. the server validates that named Repository exists (it is a real, registered
//     Repository, via ImportRepositoryResolver).
//
// This is the concrete, server-validated mechanism that binds a write to "the
// reconciled Repository": authority derives from the control-plane identity AND a
// real named Repository, so a caller cannot forge ownership. The ownership mark on
// the written resource is then set server-side from this metadata (never from the
// request body) and cross-Repository writes are refused in the handlers
// (EnforceImportOwnership).

// ImportRepositoryMetadataKey is the gRPC metadata header that carries the id of the
// Repository the import reconciler is reconciling. It is deliberately separate from
// any repository_id field on a request body, which the server ignores.
const ImportRepositoryMetadataKey = "x-hypershell-import-repository-id"

// ImportRepositoryResolver reports whether a Repository id names a real, registered
// Repository. It is implemented by the repositories plugin and is nil when that
// plugin is not registered, in which case import writes cannot be authorized.
type ImportRepositoryResolver interface {
	RepositoryExists(ctx context.Context, repositoryID string) (bool, error)
}

// importExtKindServices are the ext-kind gRPC services whose writes the declarative
// import reconciler owns. Each carries a repository_id ownership mark. RepositoryService
// is intentionally excluded: a Repository is the owner (it has no ownership mark) and
// is registered and managed by an admin over HTTP, not created/pruned by the import.
var importExtKindServices = map[string]struct{}{
	"SandboxTemplateService": {},
	"ProviderSpecService":    {},
	"ProviderBindingService": {},
	"InferenceRouteService":  {},
	"SecretSourceService":    {},
}

// isImportExtKindService reports whether service is one of the owned import ext kinds.
func isImportExtKindService(service string) bool {
	_, ok := importExtKindServices[service]
	return ok
}

// IsImportExtKindWrite reports whether fullMethod is a write (non-read) method on an
// owned import ext kind. The gRPC interceptor uses it to keep the bootstrap-service-
// account allowlist exemption from reaching these writes: an allowlisted account that
// is not a registered control plane is not an import reconciler.
func IsImportExtKindWrite(fullMethod string) bool {
	service, method, ok := splitHypershellMethod(fullMethod)
	return ok && isImportExtKindService(service) && !isReadMethodName(method)
}

// ImportRepositoryIDFromContext returns the reconciled Repository id carried in the
// trusted gRPC metadata, or "" when absent.
func ImportRepositoryIDFromContext(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	values := md.Get(ImportRepositoryMetadataKey)
	if len(values) == 0 {
		return ""
	}
	return strings.TrimSpace(values[0])
}

// authorizeImportWrite decides a control-plane write to an owned import ext kind. It
// requires the reconciled-Repository metadata to be present and to name a real
// Repository; a missing resolver or a lookup failure is Unavailable (retryable),
// never a silent grant.
func authorizeImportWrite(ctx context.Context, imports ImportRepositoryResolver) error {
	repositoryID := ImportRepositoryIDFromContext(ctx)
	if repositoryID == "" {
		// No reconciled-Repository binding: this is not an authorized import write.
		return denied()
	}
	if imports == nil {
		return status.Error(codes.Unavailable, "repository registry unavailable; cannot verify the reconciled repository")
	}
	exists, err := imports.RepositoryExists(ctx, repositoryID)
	if err != nil {
		return status.Error(codes.Unavailable, "repository lookup failed; cannot verify the reconciled repository")
	}
	if !exists {
		return denied()
	}
	return nil
}

// EnforceImportOwnership verifies, in a write handler for an owned import ext kind,
// that the target resource is owned by the reconciled Repository. existing is the
// target's current ownership mark (nil for a resource created directly through REST
// or hsctl apply). When there is an import context (a reconciled-Repository id in
// metadata), a write is refused unless the target is already owned by that same
// Repository; a resource owned by a different Repository, or unowned, is refused so
// the import cannot adopt or prune resources it does not own. With no import context
// (for example a direct write with RBAC disabled) this is a no-op.
func EnforceImportOwnership(ctx context.Context, existing *string) error {
	repositoryID := ImportRepositoryIDFromContext(ctx)
	if repositoryID == "" {
		return nil
	}
	if existing == nil || *existing != repositoryID {
		return denied()
	}
	return nil
}

// ImportOwnershipMark returns the ownership mark a create handler for an owned import
// ext kind SHALL persist: the reconciled Repository id from trusted metadata, or nil
// when there is no import context. It deliberately ignores any client-supplied
// repository_id on the request body so ownership cannot be forged.
func ImportOwnershipMark(ctx context.Context) *string {
	repositoryID := ImportRepositoryIDFromContext(ctx)
	if repositoryID == "" {
		return nil
	}
	return &repositoryID
}
