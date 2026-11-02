package extsearch

import (
	"context"
	"reflect"

	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/environments"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/services"
	"github.com/openshift-online/rh-trex-ai/components/api-server/plugins/generic"
)

// validatingService wraps the upstream generic list service and rejects a list
// request whose `search=` filter names an unknown column BEFORE the clause is
// compiled into SQL. Centralizing the guard here (rather than in each Kind's List
// handler) means every Kind - core and extension - is protected uniformly and a
// new Kind cannot omit it: it obtains its generic service from Service below,
// which always returns this wrapper (issue #871, SDLC-CONF-13).
type validatingService struct {
	inner services.GenericService
}

var _ services.GenericService = &validatingService{}

// NewValidatingService wraps a generic list service with search-column
// validation. It returns nil when inner is nil so a missing registration surfaces
// identically to the unwrapped path.
func NewValidatingService(inner services.GenericService) services.GenericService {
	if inner == nil {
		return nil
	}
	return &validatingService{inner: inner}
}

// List validates the search filter against the element type of resourceList, then
// delegates to the wrapped service. resourceList is a pointer to a slice of the
// Kind's model (for example *[]CIProvider); the element type is derived by
// reflection, mirroring how the upstream generic service determines the resource
// type, so no caller needs to pass the model explicitly.
func (s *validatingService) List(ctx context.Context, username string, args *services.ListArguments, resourceList interface{}) (*api.PagingMeta, *errors.ServiceError) {
	if args != nil && args.Search != "" {
		// Search-column validation is a best-effort defense that upgrades a bad
		// search column from a transaction-poisoning 500 to a clean 400. It must
		// never DOWNGRADE availability: if the resource list shape cannot be
		// classified for reflection, fail OPEN and delegate the List unchanged
		// rather than turning an otherwise-valid request into a 500 (FINDING 5).
		if model, ok := modelPointer(resourceList); ok {
			if serr := ValidateSearchColumns(args.Search, model); serr != nil {
				return nil, serr
			}
		}
	}
	return s.inner.List(ctx, username, args, resourceList)
}

// modelPointer returns a freshly allocated pointer to the element type of a
// `*[]T` resource list, suitable for GORM schema parsing, and ok=true. When
// resourceList is not a pointer to a slice its shape cannot be classified for
// reflection; it returns ok=false so the caller can fail OPEN (delegate the List
// without column validation) rather than turn a valid request into a 500. The
// shape mismatch is not an error here - validation is best-effort defense, not a
// correctness gate (FINDING 5).
func modelPointer(resourceList interface{}) (interface{}, bool) {
	t := reflect.TypeOf(resourceList)
	if t == nil || t.Kind() != reflect.Ptr {
		return nil, false
	}
	slice := t.Elem()
	if slice.Kind() != reflect.Slice {
		return nil, false
	}
	return reflect.New(slice.Elem()).Interface(), true
}

// Service returns the shared generic list service wrapped with search-column
// validation. Every HyperShell plugin MUST obtain its generic service here (not
// directly from the upstream generic plugin) so that no list path - core or
// extension, REST or gRPC - can reach SQL with an unverified search column
// (issue #871, SDLC-CONF-13).
func Service(s *environments.Services) services.GenericService {
	return NewValidatingService(generic.Service(s))
}
