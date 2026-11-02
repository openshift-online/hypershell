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
	if args != nil {
		model, serr := newModelPointer(resourceList)
		if serr != nil {
			return nil, serr
		}
		if serr := ValidateSearchColumns(args.Search, model); serr != nil {
			return nil, serr
		}
	}
	return s.inner.List(ctx, username, args, resourceList)
}

// newModelPointer returns a freshly allocated pointer to the element type of a
// `*[]T` resource list, suitable for GORM schema parsing. It fails closed if
// resourceList is not a pointer to a slice.
func newModelPointer(resourceList interface{}) (interface{}, *errors.ServiceError) {
	t := reflect.TypeOf(resourceList)
	if t == nil || t.Kind() != reflect.Ptr {
		return nil, errors.GeneralError("resource list must be a pointer to a slice")
	}
	slice := t.Elem()
	if slice.Kind() != reflect.Slice {
		return nil, errors.GeneralError("resource list must be a pointer to a slice")
	}
	return reflect.New(slice.Elem()).Interface(), nil
}

// Service returns the shared generic list service wrapped with search-column
// validation. Every HyperShell plugin MUST obtain its generic service here (not
// directly from the upstream generic plugin) so that no list path - core or
// extension, REST or gRPC - can reach SQL with an unverified search column
// (issue #871, SDLC-CONF-13).
func Service(s *environments.Services) services.GenericService {
	return NewValidatingService(generic.Service(s))
}
