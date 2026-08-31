package fleets

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

type ServiceLocator func() FleetService

func NewServiceLocator(env *environments.Env) ServiceLocator {
	return func() FleetService {
		return NewFleetService(
			db.NewAdvisoryLockFactory(env.Database.SessionFactory),
			NewFleetDao(&env.Database.SessionFactory),
			events.Service(&env.Services),
		)
	}
}

func Service(s *environments.Services) FleetService {
	if s == nil {
		return nil
	}
	if obj := s.GetService("Fleets"); obj != nil {
		locator := obj.(ServiceLocator)
		return locator()
	}
	return nil
}

func init() {
	registry.RegisterService("Fleets", func(env interface{}) interface{} {
		return NewServiceLocator(env.(*environments.Env))
	})

	pkgserver.RegisterRoutes("fleets", func(apiV1Router *mux.Router, services pkgserver.ServicesInterface, authMiddleware environments.JWTMiddleware, authzMiddleware auth.AuthorizationMiddleware) {
		envServices := services.(*environments.Services)
		fleetHandler := NewFleetHandler(Service(envServices), generic.Service(envServices))

		fleetsRouter := apiV1Router.PathPrefix("/fleets").Subrouter()
		fleetsRouter.HandleFunc("", fleetHandler.List).Methods(http.MethodGet)
		fleetsRouter.HandleFunc("/{id}", fleetHandler.Get).Methods(http.MethodGet)
		fleetsRouter.HandleFunc("", fleetHandler.Create).Methods(http.MethodPost)
		fleetsRouter.HandleFunc("/{id}", fleetHandler.Patch).Methods(http.MethodPatch)
		fleetsRouter.HandleFunc("/{id}", fleetHandler.Delete).Methods(http.MethodDelete)
		fleetsRouter.Use(authMiddleware.AuthenticateAccountJWT)
		fleetsRouter.Use(authzMiddleware.AuthorizeApi)
	})

	pkgserver.RegisterController("Fleets", func(manager *controllers.KindControllerManager, services pkgserver.ServicesInterface) {
		fleetServices := Service(services.(*environments.Services))

		manager.Add(&controllers.ControllerConfig{
			Source: "Fleets",
			Handlers: map[api.EventType][]controllers.ControllerHandlerFunc{
				api.CreateEventType: {fleetServices.OnUpsert},
				api.UpdateEventType: {fleetServices.OnUpsert},
				api.DeleteEventType: {fleetServices.OnDelete},
			},
		})
	})

	pkgserver.RegisterGRPCService("fleets", func(grpcServer *grpc.Server, services pkgserver.ServicesInterface) {
		envServices := services.(*environments.Services)
		fleetService := Service(envServices)
		genericService := generic.Service(envServices)
		brokerFunc := func() *pkgserver.EventBroker {
			if obj := envServices.GetService("EventBroker"); obj != nil {
				return obj.(*pkgserver.EventBroker)
			}
			return nil
		}
		pb.RegisterFleetServiceServer(grpcServer, NewFleetGRPCHandler(fleetService, genericService, brokerFunc))
	})

	presenters.RegisterPath(Fleet{}, "fleets")
	presenters.RegisterPath(&Fleet{}, "fleets")
	presenters.RegisterKind(Fleet{}, "Fleet")
	presenters.RegisterKind(&Fleet{}, "Fleet")

	db.RegisterMigration(migration())
	db.RegisterMigration(migrationAddTraceContext())
}
