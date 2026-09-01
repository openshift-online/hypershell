package gatewayProfiles

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

type ServiceLocator func() GatewayProfileService

func NewServiceLocator(env *environments.Env) ServiceLocator {
	return func() GatewayProfileService {
		return NewGatewayProfileService(
			db.NewAdvisoryLockFactory(env.Database.SessionFactory),
			NewGatewayProfileDao(&env.Database.SessionFactory),
			events.Service(&env.Services),
		)
	}
}

func Service(s *environments.Services) GatewayProfileService {
	if s == nil {
		return nil
	}
	if obj := s.GetService("GatewayProfiles"); obj != nil {
		locator := obj.(ServiceLocator)
		return locator()
	}
	return nil
}

func init() {
	registry.RegisterService("GatewayProfiles", func(env interface{}) interface{} {
		return NewServiceLocator(env.(*environments.Env))
	})

	pkgserver.RegisterRoutes("gatewayProfiles", func(apiV1Router *mux.Router, services pkgserver.ServicesInterface, authMiddleware environments.JWTMiddleware, authzMiddleware auth.AuthorizationMiddleware) {
		envServices := services.(*environments.Services)
		gatewayProfileHandler := NewGatewayProfileHandler(Service(envServices), generic.Service(envServices))

		gatewayProfilesRouter := apiV1Router.PathPrefix("/gateway_profiles").Subrouter()
		gatewayProfilesRouter.HandleFunc("", gatewayProfileHandler.List).Methods(http.MethodGet)
		gatewayProfilesRouter.HandleFunc("/{id}", gatewayProfileHandler.Get).Methods(http.MethodGet)
		gatewayProfilesRouter.HandleFunc("", gatewayProfileHandler.Create).Methods(http.MethodPost)
		gatewayProfilesRouter.HandleFunc("/{id}", gatewayProfileHandler.Patch).Methods(http.MethodPatch)
		gatewayProfilesRouter.HandleFunc("/{id}", gatewayProfileHandler.Delete).Methods(http.MethodDelete)
		gatewayProfilesRouter.Use(authMiddleware.AuthenticateAccountJWT)
		gatewayProfilesRouter.Use(authzMiddleware.AuthorizeApi)
	})

	pkgserver.RegisterController("GatewayProfiles", func(manager *controllers.KindControllerManager, services pkgserver.ServicesInterface) {
		gatewayProfileServices := Service(services.(*environments.Services))

		manager.Add(&controllers.ControllerConfig{
			Source: "GatewayProfiles",
			Handlers: map[api.EventType][]controllers.ControllerHandlerFunc{
				api.CreateEventType: {gatewayProfileServices.OnUpsert},
				api.UpdateEventType: {gatewayProfileServices.OnUpsert},
				api.DeleteEventType: {gatewayProfileServices.OnDelete},
			},
		})
	})

	pkgserver.RegisterGRPCService("gatewayProfiles", func(grpcServer *grpc.Server, services pkgserver.ServicesInterface) {
		envServices := services.(*environments.Services)
		gatewayProfileService := Service(envServices)
		genericService := generic.Service(envServices)
		pb.RegisterGatewayProfileServiceServer(grpcServer, NewGatewayProfileGRPCHandler(gatewayProfileService, genericService))
	})

	presenters.RegisterPath(GatewayProfile{}, "gateway_profiles")
	presenters.RegisterPath(&GatewayProfile{}, "gateway_profiles")
	presenters.RegisterKind(GatewayProfile{}, "GatewayProfile")
	presenters.RegisterKind(&GatewayProfile{}, "GatewayProfile")

	db.RegisterMigration(migration())
}
