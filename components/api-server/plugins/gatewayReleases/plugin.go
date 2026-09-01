package gatewayReleases

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

type ServiceLocator func() GatewayReleaseService

func NewServiceLocator(env *environments.Env) ServiceLocator {
	return func() GatewayReleaseService {
		return NewGatewayReleaseService(
			db.NewAdvisoryLockFactory(env.Database.SessionFactory),
			NewGatewayReleaseDao(&env.Database.SessionFactory),
			events.Service(&env.Services),
		)
	}
}

func Service(s *environments.Services) GatewayReleaseService {
	if s == nil {
		return nil
	}
	if obj := s.GetService("GatewayReleases"); obj != nil {
		locator := obj.(ServiceLocator)
		return locator()
	}
	return nil
}

func init() {
	registry.RegisterService("GatewayReleases", func(env interface{}) interface{} {
		return NewServiceLocator(env.(*environments.Env))
	})

	pkgserver.RegisterRoutes("gatewayReleases", func(apiV1Router *mux.Router, services pkgserver.ServicesInterface, authMiddleware environments.JWTMiddleware, authzMiddleware auth.AuthorizationMiddleware) {
		envServices := services.(*environments.Services)
		gatewayReleaseHandler := NewGatewayReleaseHandler(Service(envServices), generic.Service(envServices))

		gatewayReleasesRouter := apiV1Router.PathPrefix("/gateway_releases").Subrouter()
		gatewayReleasesRouter.HandleFunc("", gatewayReleaseHandler.List).Methods(http.MethodGet)
		gatewayReleasesRouter.HandleFunc("/{id}", gatewayReleaseHandler.Get).Methods(http.MethodGet)
		gatewayReleasesRouter.HandleFunc("", gatewayReleaseHandler.Create).Methods(http.MethodPost)
		gatewayReleasesRouter.HandleFunc("/{id}", gatewayReleaseHandler.Patch).Methods(http.MethodPatch)
		gatewayReleasesRouter.HandleFunc("/{id}", gatewayReleaseHandler.Delete).Methods(http.MethodDelete)
		gatewayReleasesRouter.Use(authMiddleware.AuthenticateAccountJWT)
		gatewayReleasesRouter.Use(authzMiddleware.AuthorizeApi)
	})

	pkgserver.RegisterController("GatewayReleases", func(manager *controllers.KindControllerManager, services pkgserver.ServicesInterface) {
		gatewayReleaseServices := Service(services.(*environments.Services))

		manager.Add(&controllers.ControllerConfig{
			Source: "GatewayReleases",
			Handlers: map[api.EventType][]controllers.ControllerHandlerFunc{
				api.CreateEventType: {gatewayReleaseServices.OnUpsert},
				api.UpdateEventType: {gatewayReleaseServices.OnUpsert},
				api.DeleteEventType: {gatewayReleaseServices.OnDelete},
			},
		})
	})

	pkgserver.RegisterGRPCService("gatewayReleases", func(grpcServer *grpc.Server, services pkgserver.ServicesInterface) {
		envServices := services.(*environments.Services)
		gatewayReleaseService := Service(envServices)
		genericService := generic.Service(envServices)
		brokerFunc := func() *pkgserver.EventBroker {
			if obj := envServices.GetService("EventBroker"); obj != nil {
				return obj.(*pkgserver.EventBroker)
			}
			return nil
		}
		pb.RegisterGatewayReleaseServiceServer(grpcServer, NewGatewayReleaseGRPCHandler(gatewayReleaseService, genericService, brokerFunc))
	})

	presenters.RegisterPath(GatewayRelease{}, "gateway_releases")
	presenters.RegisterPath(&GatewayRelease{}, "gateway_releases")
	presenters.RegisterKind(GatewayRelease{}, "GatewayRelease")
	presenters.RegisterKind(&GatewayRelease{}, "GatewayRelease")

	db.RegisterMigration(migration())
	db.RegisterMigration(migrationDropFleetId())
}
