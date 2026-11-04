package providerSpecs

import (
	"net/http"

	"google.golang.org/grpc"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/api-server/pkg/extsearch"
	"github.com/openshift-online/hypershell/components/api-server/pkg/routes"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api/presenters"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/authz"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/db"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/environments"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/registry"
	pkgserver "github.com/openshift-online/rh-trex-ai/components/api-server/pkg/server"
	"github.com/openshift-online/rh-trex-ai/components/api-server/plugins/events"
)

type ServiceLocator func() ProviderSpecService

func NewServiceLocator(env *environments.Env) ServiceLocator {
	dao := NewProviderSpecDao(&env.Database.SessionFactory)
	return func() ProviderSpecService {
		return NewProviderSpecService(dao, events.Service(&env.Services))
	}
}

func Service(s *environments.Services) ProviderSpecService {
	if s == nil {
		return nil
	}
	if obj := s.GetService("ProviderSpecs"); obj != nil {
		locator := obj.(ServiceLocator)
		return locator()
	}
	return nil
}

func init() {
	registry.RegisterService("ProviderSpecs", func(env interface{}) interface{} {
		return NewServiceLocator(env.(*environments.Env))
	})

	pkgserver.RegisterPrefixedRoutes("providerSpecs", "ext", func(apiRouter *pkgserver.APIRouter, services pkgserver.ServicesInterface) {
		envServices := services.(*environments.Services)
		h := NewProviderSpecHandler(Service(envServices), extsearch.Service(envServices))

		router := routes.Resource(apiRouter, "provider_specs").PathPrefix("/provider_specs").Subrouter()
		router.HandleFunc("", h.List).Methods(http.MethodGet)
		router.HandleFunc("/{id}", h.Get).Methods(http.MethodGet)
		router.HandleFunc("", h.Create).Methods(http.MethodPost)
		router.HandleFunc("/{id}", h.Patch).Methods(http.MethodPatch)
		router.HandleFunc("/{id}", h.Delete).Methods(http.MethodDelete)
	})

	pkgserver.RegisterGRPCService("providerSpecs", func(grpcServer *grpc.Server, services pkgserver.ServicesInterface) {
		envServices := services.(*environments.Services)
		brokerFunc := func() *pkgserver.EventBroker {
			if obj := envServices.GetService("EventBroker"); obj != nil {
				return obj.(*pkgserver.EventBroker)
			}
			return nil
		}
		pb.RegisterProviderSpecServiceServer(grpcServer, NewProviderSpecGRPCHandler(Service(envServices), extsearch.Service(envServices), brokerFunc))
	}, authz.CRUDResources(pb.ProviderSpecService_ServiceDesc.ServiceName, "provider_specs", "ProviderSpec", "ProviderSpecs"))

	presenters.RegisterPath(ProviderSpec{}, "provider_specs")
	presenters.RegisterPath(&ProviderSpec{}, "provider_specs")
	presenters.RegisterKind(ProviderSpec{}, "ProviderSpec")
	presenters.RegisterKind(&ProviderSpec{}, "ProviderSpec")

	db.RegisterMigration(migration())
	db.RegisterMigration(migrationAddRepositoryOwnership())
}
