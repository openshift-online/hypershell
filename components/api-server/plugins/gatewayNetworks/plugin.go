package gatewayNetworks

import (
	"net/http"

	"github.com/gorilla/mux"
	"google.golang.org/grpc"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/rh-trex-ai/pkg/api"
	"github.com/openshift-online/rh-trex-ai/pkg/api/presenters"
	"github.com/openshift-online/rh-trex-ai/pkg/auth"
	"github.com/openshift-online/rh-trex-ai/pkg/controllers"
	"github.com/openshift-online/rh-trex-ai/pkg/db"
	"github.com/openshift-online/rh-trex-ai/pkg/environments"
	"github.com/openshift-online/rh-trex-ai/pkg/registry"
	pkgserver "github.com/openshift-online/rh-trex-ai/pkg/server"
	"github.com/openshift-online/rh-trex-ai/plugins/events"
	"github.com/openshift-online/rh-trex-ai/plugins/generic"
)

type ServiceLocator func() GatewayNetworkService

func NewServiceLocator(env *environments.Env) ServiceLocator {
	return func() GatewayNetworkService {
		return NewGatewayNetworkService(
			db.NewAdvisoryLockFactory(env.Database.SessionFactory),
			NewGatewayNetworkDao(&env.Database.SessionFactory),
			events.Service(&env.Services),
		)
	}
}

func Service(s *environments.Services) GatewayNetworkService {
	if s == nil {
		return nil
	}
	if obj := s.GetService("GatewayNetworks"); obj != nil {
		locator := obj.(ServiceLocator)
		return locator()
	}
	return nil
}

func init() {
	registry.RegisterService("GatewayNetworks", func(env interface{}) interface{} {
		return NewServiceLocator(env.(*environments.Env))
	})

	pkgserver.RegisterRoutes("gatewayNetworks", func(apiV1Router *mux.Router, services pkgserver.ServicesInterface, authMiddleware environments.JWTMiddleware, authzMiddleware auth.AuthorizationMiddleware) {
		envServices := services.(*environments.Services)
		gatewayNetworkHandler := NewGatewayNetworkHandler(Service(envServices), generic.Service(envServices))

		gatewayNetworksRouter := apiV1Router.PathPrefix("/gateway_networks").Subrouter()
		gatewayNetworksRouter.HandleFunc("", gatewayNetworkHandler.List).Methods(http.MethodGet)
		gatewayNetworksRouter.HandleFunc("/{id}", gatewayNetworkHandler.Get).Methods(http.MethodGet)
		gatewayNetworksRouter.HandleFunc("", gatewayNetworkHandler.Create).Methods(http.MethodPost)
		gatewayNetworksRouter.HandleFunc("/{id}", gatewayNetworkHandler.Patch).Methods(http.MethodPatch)
		gatewayNetworksRouter.HandleFunc("/{id}", gatewayNetworkHandler.Delete).Methods(http.MethodDelete)
		gatewayNetworksRouter.Use(authMiddleware.AuthenticateAccountJWT)
		gatewayNetworksRouter.Use(authzMiddleware.AuthorizeApi)
	})

	pkgserver.RegisterController("GatewayNetworks", func(manager *controllers.KindControllerManager, services pkgserver.ServicesInterface) {
		gatewayNetworkServices := Service(services.(*environments.Services))

		manager.Add(&controllers.ControllerConfig{
			Source: "GatewayNetworks",
			Handlers: map[api.EventType][]controllers.ControllerHandlerFunc{
				api.CreateEventType: {gatewayNetworkServices.OnUpsert},
				api.UpdateEventType: {gatewayNetworkServices.OnUpsert},
				api.DeleteEventType: {gatewayNetworkServices.OnDelete},
			},
		})
	})

	pkgserver.RegisterGRPCService("gatewayNetworks", func(grpcServer *grpc.Server, services pkgserver.ServicesInterface) {
		envServices := services.(*environments.Services)
		gatewayNetworkService := Service(envServices)
		genericService := generic.Service(envServices)
		brokerFunc := func() *pkgserver.EventBroker {
			if obj := envServices.GetService("EventBroker"); obj != nil {
				return obj.(*pkgserver.EventBroker)
			}
			return nil
		}
		pb.RegisterGatewayNetworkServiceServer(grpcServer, NewGatewayNetworkGRPCHandler(gatewayNetworkService, genericService, brokerFunc))
	})

	presenters.RegisterPath(GatewayNetwork{}, "gateway_networks")
	presenters.RegisterPath(&GatewayNetwork{}, "gateway_networks")
	presenters.RegisterKind(GatewayNetwork{}, "GatewayNetwork")
	presenters.RegisterKind(&GatewayNetwork{}, "GatewayNetwork")

	db.RegisterMigration(migration())
	db.RegisterMigration(migrationAddTraceContext())
}
