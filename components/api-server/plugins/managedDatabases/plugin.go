package managedDatabases

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

type ServiceLocator func() ManagedDatabaseService

func NewServiceLocator(env *environments.Env) ServiceLocator {
	return func() ManagedDatabaseService {
		return NewManagedDatabaseService(
			db.NewAdvisoryLockFactory(env.Database.SessionFactory),
			NewManagedDatabaseDao(&env.Database.SessionFactory),
			events.Service(&env.Services),
		)
	}
}

func Service(s *environments.Services) ManagedDatabaseService {
	if s == nil {
		return nil
	}
	if obj := s.GetService("ManagedDatabases"); obj != nil {
		locator := obj.(ServiceLocator)
		return locator()
	}
	return nil
}

func init() {
	registry.RegisterService("ManagedDatabases", func(env interface{}) interface{} {
		return NewServiceLocator(env.(*environments.Env))
	})

	pkgserver.RegisterRoutes("managedDatabases", func(apiV1Router *mux.Router, services pkgserver.ServicesInterface, authMiddleware environments.JWTMiddleware, authzMiddleware auth.AuthorizationMiddleware) {
		envServices := services.(*environments.Services)
		managedDatabaseHandler := NewManagedDatabaseHandler(Service(envServices), generic.Service(envServices))

		managedDatabasesRouter := apiV1Router.PathPrefix("/managed_databases").Subrouter()
		managedDatabasesRouter.HandleFunc("", managedDatabaseHandler.List).Methods(http.MethodGet)
		managedDatabasesRouter.HandleFunc("/{id}", managedDatabaseHandler.Get).Methods(http.MethodGet)
		managedDatabasesRouter.HandleFunc("", managedDatabaseHandler.Create).Methods(http.MethodPost)
		managedDatabasesRouter.HandleFunc("/{id}", managedDatabaseHandler.Patch).Methods(http.MethodPatch)
		managedDatabasesRouter.HandleFunc("/{id}", managedDatabaseHandler.Delete).Methods(http.MethodDelete)
		managedDatabasesRouter.Use(authMiddleware.AuthenticateAccountJWT)
		managedDatabasesRouter.Use(authzMiddleware.AuthorizeApi)
	})

	pkgserver.RegisterController("ManagedDatabases", func(manager *controllers.KindControllerManager, services pkgserver.ServicesInterface) {
		managedDatabaseServices := Service(services.(*environments.Services))

		manager.Add(&controllers.ControllerConfig{
			Source: "ManagedDatabases",
			Handlers: map[api.EventType][]controllers.ControllerHandlerFunc{
				api.CreateEventType: {managedDatabaseServices.OnUpsert},
				api.UpdateEventType: {managedDatabaseServices.OnUpsert},
				api.DeleteEventType: {managedDatabaseServices.OnDelete},
			},
		})
	})

	pkgserver.RegisterGRPCService("managedDatabases", func(grpcServer *grpc.Server, services pkgserver.ServicesInterface) {
		envServices := services.(*environments.Services)
		managedDatabaseService := Service(envServices)
		genericService := generic.Service(envServices)
		brokerFunc := func() *pkgserver.EventBroker {
			if obj := envServices.GetService("EventBroker"); obj != nil {
				return obj.(*pkgserver.EventBroker)
			}
			return nil
		}
		pb.RegisterManagedDatabaseServiceServer(grpcServer, NewManagedDatabaseGRPCHandler(managedDatabaseService, genericService, brokerFunc))
	})

	presenters.RegisterPath(ManagedDatabase{}, "managed_databases")
	presenters.RegisterPath(&ManagedDatabase{}, "managed_databases")
	presenters.RegisterKind(ManagedDatabase{}, "ManagedDatabase")
	presenters.RegisterKind(&ManagedDatabase{}, "ManagedDatabase")

	db.RegisterMigration(migration())
	db.RegisterMigration(migrationAddNamespace())
	db.RegisterMigration(migrationDropFleetId())
}
