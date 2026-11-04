package providerBindings

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

type ServiceLocator func() ProviderBindingService

func NewServiceLocator(env *environments.Env) ServiceLocator {
	dao := NewProviderBindingDao(&env.Database.SessionFactory)
	return func() ProviderBindingService {
		return NewProviderBindingService(dao, events.Service(&env.Services))
	}
}

func Service(s *environments.Services) ProviderBindingService {
	if s == nil {
		return nil
	}
	if obj := s.GetService("ProviderBindings"); obj != nil {
		locator := obj.(ServiceLocator)
		return locator()
	}
	return nil
}

func init() {
	registry.RegisterService("ProviderBindings", func(env interface{}) interface{} {
		return NewServiceLocator(env.(*environments.Env))
	})

	pkgserver.RegisterPrefixedRoutes("providerBindings", "ext", func(apiRouter *pkgserver.APIRouter, services pkgserver.ServicesInterface) {
		envServices := services.(*environments.Services)
		h := NewProviderBindingHandler(Service(envServices), extsearch.Service(envServices))

		router := routes.Resource(apiRouter, "provider_bindings").PathPrefix("/provider_bindings").Subrouter()
		router.HandleFunc("", h.List).Methods(http.MethodGet)
		router.HandleFunc("/{id}", h.Get).Methods(http.MethodGet)
		router.HandleFunc("", h.Create).Methods(http.MethodPost)
		router.HandleFunc("/{id}", h.Patch).Methods(http.MethodPatch)
		router.HandleFunc("/{id}", h.Delete).Methods(http.MethodDelete)
	})

	pkgserver.RegisterGRPCService("providerBindings", func(grpcServer *grpc.Server, services pkgserver.ServicesInterface) {
		envServices := services.(*environments.Services)
		brokerFunc := func() *pkgserver.EventBroker {
			if obj := envServices.GetService("EventBroker"); obj != nil {
				return obj.(*pkgserver.EventBroker)
			}
			return nil
		}
		pb.RegisterProviderBindingServiceServer(grpcServer, NewProviderBindingGRPCHandler(Service(envServices), extsearch.Service(envServices), brokerFunc))
	}, authz.CRUDResources(pb.ProviderBindingService_ServiceDesc.ServiceName, "provider_bindings", "ProviderBinding", "ProviderBindings"))

	presenters.RegisterPath(ProviderBinding{}, "provider_bindings")
	presenters.RegisterPath(&ProviderBinding{}, "provider_bindings")
	presenters.RegisterKind(ProviderBinding{}, "ProviderBinding")
	presenters.RegisterKind(&ProviderBinding{}, "ProviderBinding")

	db.RegisterMigration(migration())
	db.RegisterMigration(migrationAddRepositoryOwnership())
}
