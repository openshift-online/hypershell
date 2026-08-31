package managedClusters

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

type ServiceLocator func() ManagedClusterService

func NewServiceLocator(env *environments.Env) ServiceLocator {
	return func() ManagedClusterService {
		return NewManagedClusterService(
			db.NewAdvisoryLockFactory(env.Database.SessionFactory),
			NewManagedClusterDao(&env.Database.SessionFactory),
			events.Service(&env.Services),
		)
	}
}

func Service(s *environments.Services) ManagedClusterService {
	if s == nil {
		return nil
	}
	if obj := s.GetService("ManagedClusters"); obj != nil {
		locator := obj.(ServiceLocator)
		return locator()
	}
	return nil
}

func init() {
	registry.RegisterService("ManagedClusters", func(env interface{}) interface{} {
		return NewServiceLocator(env.(*environments.Env))
	})

	pkgserver.RegisterRoutes("managedClusters", func(apiV1Router *mux.Router, services pkgserver.ServicesInterface, authMiddleware environments.JWTMiddleware, authzMiddleware auth.AuthorizationMiddleware) {
		envServices := services.(*environments.Services)
		managedClusterHandler := NewManagedClusterHandler(Service(envServices), generic.Service(envServices))

		managedClustersRouter := apiV1Router.PathPrefix("/managed_clusters").Subrouter()
		managedClustersRouter.HandleFunc("", managedClusterHandler.List).Methods(http.MethodGet)
		managedClustersRouter.HandleFunc("/{id}", managedClusterHandler.Get).Methods(http.MethodGet)
		managedClustersRouter.HandleFunc("", managedClusterHandler.Create).Methods(http.MethodPost)
		managedClustersRouter.HandleFunc("/{id}", managedClusterHandler.Patch).Methods(http.MethodPatch)
		managedClustersRouter.HandleFunc("/{id}", managedClusterHandler.Delete).Methods(http.MethodDelete)
		managedClustersRouter.Use(authMiddleware.AuthenticateAccountJWT)
		managedClustersRouter.Use(authzMiddleware.AuthorizeApi)
	})

	pkgserver.RegisterController("ManagedClusters", func(manager *controllers.KindControllerManager, services pkgserver.ServicesInterface) {
		managedClusterServices := Service(services.(*environments.Services))

		manager.Add(&controllers.ControllerConfig{
			Source: "ManagedClusters",
			Handlers: map[api.EventType][]controllers.ControllerHandlerFunc{
				api.CreateEventType: {managedClusterServices.OnUpsert},
				api.UpdateEventType: {managedClusterServices.OnUpsert},
				api.DeleteEventType: {managedClusterServices.OnDelete},
			},
		})
	})

	pkgserver.RegisterGRPCService("managedClusters", func(grpcServer *grpc.Server, services pkgserver.ServicesInterface) {
		envServices := services.(*environments.Services)
		managedClusterService := Service(envServices)
		genericService := generic.Service(envServices)
		brokerFunc := func() *pkgserver.EventBroker {
			if obj := envServices.GetService("EventBroker"); obj != nil {
				return obj.(*pkgserver.EventBroker)
			}
			return nil
		}
		pb.RegisterManagedClusterServiceServer(grpcServer, NewManagedClusterGRPCHandler(managedClusterService, genericService, brokerFunc))
	})

	presenters.RegisterPath(ManagedCluster{}, "managed_clusters")
	presenters.RegisterPath(&ManagedCluster{}, "managed_clusters")
	presenters.RegisterKind(ManagedCluster{}, "ManagedCluster")
	presenters.RegisterKind(&ManagedCluster{}, "ManagedCluster")

	db.RegisterMigration(migration())
	db.RegisterMigration(migrationAddTraceContext())
}
