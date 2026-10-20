package gatewayAccess

import (
	"context"
	"errors"
	"log"
	"os"
)

// DirectoryUser is a candidate from the Keycloak realm directory projection
// (GAM-09). Subject is the Keycloak user's `sub` (its user UUID).
type DirectoryUser struct {
	Username string
	Name     string
	Email    string
	Subject  string
}

// DirectoryResolver serves the Keycloak realm directory projection maintained by
// the control plane (GAM-09). The API server never reads the Keycloak admin
// Secret; it reads this projection. Search backs the console "Add users" picker;
// Resolve re-validates a chosen identity against the realm at grant time (GAM-04).
type DirectoryResolver interface {
	// Search returns realm candidates whose username or name match query
	// (case-insensitive substring), bounded by limit.
	Search(ctx context.Context, query string, limit int) ([]DirectoryUser, error)
	// Resolve returns the realm user for an exact username. found is false when
	// no such user exists in the realm.
	Resolve(ctx context.Context, username string) (user DirectoryUser, found bool, err error)
}

// ErrDirectoryUnavailable is returned by the unconfigured resolver so callers
// can surface a clear "directory not available" error rather than a nil panic.
var ErrDirectoryUnavailable = errors.New("keycloak directory projection is not configured")

// directoryResolverOverride, when set, is the resolver the access facade uses.
// The control-plane-backed resolver is installed at startup (Wave C wiring) and
// tests install a fake; when unset the facade uses the unconfigured resolver.
var directoryResolverOverride DirectoryResolver

// SetDirectoryResolver installs the directory resolver used by the access facade.
func SetDirectoryResolver(r DirectoryResolver) { directoryResolverOverride = r }

// newDirectoryResolverFromEnvironment builds the directory resolver for the
// running environment. Until a control-plane-backed resolver is installed,
// grant-by-directory and directory search fail loudly with
// ErrDirectoryUnavailable rather than silently resolving nothing.
func newDirectoryResolverFromEnvironment() DirectoryResolver {
	if directoryResolverOverride != nil {
		return directoryResolverOverride
	}
	// The directory projection rides the same in-cluster control-plane channel as
	// the service-account provisioner, so it reuses that address.
	address := os.Getenv("HYPERSHELL_SERVICE_ACCOUNT_PROVISIONER_ADDR")
	if address == "" {
		return unconfiguredResolver{}
	}
	resolver, err := newGRPCDirectoryResolver(address)
	if err != nil {
		log.Printf("WARN gateway access directory resolver is not available: %v", err)
		return unconfiguredResolver{}
	}
	return resolver
}

// unconfiguredResolver is used when no control-plane directory endpoint is
// configured. It fails loudly rather than silently resolving nothing.
type unconfiguredResolver struct{}

func (unconfiguredResolver) Search(context.Context, string, int) ([]DirectoryUser, error) {
	return nil, ErrDirectoryUnavailable
}

func (unconfiguredResolver) Resolve(context.Context, string) (DirectoryUser, bool, error) {
	return DirectoryUser{}, false, ErrDirectoryUnavailable
}
