package ciProviders

import (
	"net/http"

	"github.com/gorilla/mux"
	"google.golang.org/grpc"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api/presenters"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/auth"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/db"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/environments"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/registry"
	pkgserver "github.com/openshift-online/rh-trex-ai/components/api-server/pkg/server"
	"github.com/openshift-online/rh-trex-ai/components/api-server/plugins/events"
	"github.com/openshift-online/rh-trex-ai/components/api-server/plugins/generic"
)

type ServiceLocator func() CIProviderService

func NewServiceLocator(env *environments.Env) ServiceLocator {
	dao := NewCIProviderDao(&env.Database.SessionFactory)
	return func() CIProviderService {
		return NewCIProviderService(dao, events.Service(&env.Services))
	}
}

func Service(s *environments.Services) CIProviderService {
	if s == nil {
		return nil
	}
	if obj := s.GetService("CIProviders"); obj != nil {
		locator := obj.(ServiceLocator)
		return locator()
	}
	return nil
}

func init() {
	registry.RegisterService("CIProviders", func(env interface{}) interface{} {
		return NewServiceLocator(env.(*environments.Env))
	})

	pkgserver.RegisterPrefixedRoutes("ci_providers", "ext", func(extRouter *mux.Router, services pkgserver.ServicesInterface, authMiddleware environments.JWTMiddleware, authzMiddleware auth.AuthorizationMiddleware) {
		envServices := services.(*environments.Services)
		h := NewCIProviderHandler(Service(envServices), generic.Service(envServices))

		router := extRouter.PathPrefix("/ci_providers").Subrouter()
		router.HandleFunc("", h.List).Methods(http.MethodGet)
		router.HandleFunc("/{id}", h.Get).Methods(http.MethodGet)
		router.HandleFunc("", h.Create).Methods(http.MethodPost)
		router.HandleFunc("/{id}", h.Patch).Methods(http.MethodPatch)
		router.HandleFunc("/{id}", h.Delete).Methods(http.MethodDelete)
		router.Use(authMiddleware.AuthenticateAccountJWT)
		router.Use(authzMiddleware.AuthorizeApi)
	})

	pkgserver.RegisterGRPCService("ci_providers", func(grpcServer *grpc.Server, services pkgserver.ServicesInterface) {
		envServices := services.(*environments.Services)
		brokerFunc := func() *pkgserver.EventBroker {
			if obj := envServices.GetService("EventBroker"); obj != nil {
				return obj.(*pkgserver.EventBroker)
			}
			return nil
		}
		pb.RegisterCIProviderServiceServer(grpcServer, NewCIProviderGRPCHandler(Service(envServices), generic.Service(envServices), brokerFunc))
	})

	presenters.RegisterPath(CIProvider{}, "ci_providers")
	presenters.RegisterPath(&CIProvider{}, "ci_providers")
	presenters.RegisterKind(CIProvider{}, "CIProvider")
	presenters.RegisterKind(&CIProvider{}, "CIProvider")

	db.RegisterMigration(migration())
	db.RegisterMigration(migrationConverge())
}
