package gateways

import (
	"context"
	"net/http"

	"github.com/gorilla/mux"
	"google.golang.org/grpc"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/api-server/pkg/rbac"
	"github.com/openshift-online/hypershell/components/api-server/plugins/managedClusters"
	"github.com/openshift-online/hypershell/components/api-server/plugins/managedDatabases"
	"github.com/openshift-online/hypershell/components/api-server/plugins/roleBindings"
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

type ServiceLocator func() GatewayService

type managedDatabaseAdapter struct {
	svc managedDatabases.ManagedDatabaseService
}

func (a *managedDatabaseAdapter) FindSoleInFleet(ctx context.Context, fleetID string) (string, error) {
	db, svcErr := a.svc.FindSoleInFleet(ctx, fleetID)
	if svcErr != nil {
		return "", svcErr
	}
	if db == nil {
		return "", nil
	}
	return db.ID, nil
}

func (a *managedDatabaseAdapter) FindSole(ctx context.Context) (string, string, error) {
	all, svcErr := a.svc.All(ctx)
	if svcErr != nil {
		return "", "", svcErr
	}
	if len(all) == 1 {
		return all[0].ID, all[0].FleetId, nil
	}
	return "", "", nil
}

type clusterFleetAdapter struct {
	svc managedClusters.ManagedClusterService
}

func (a *clusterFleetAdapter) FleetIDForCluster(ctx context.Context, clusterID string) (string, error) {
	cluster, svcErr := a.svc.Get(ctx, clusterID)
	if svcErr != nil {
		return "", svcErr
	}
	return cluster.FleetId, nil
}

func NewServiceLocator(env *environments.Env) ServiceLocator {
	dao := NewGatewayDao(&env.Database.SessionFactory)
	RegisterGatewayMetrics(dao)
	return func() GatewayService {
		var dbFinder FleetDatabaseFinder
		if mdSvc := managedDatabases.Service(&env.Services); mdSvc != nil {
			dbFinder = &managedDatabaseAdapter{svc: mdSvc}
		}
		var clusterResolver ClusterFleetResolver
		if mcSvc := managedClusters.Service(&env.Services); mcSvc != nil {
			clusterResolver = &clusterFleetAdapter{svc: mcSvc}
		}
		return NewGatewayService(
			db.NewAdvisoryLockFactory(env.Database.SessionFactory),
			dao,
			events.Service(&env.Services),
			dbFinder,
			clusterResolver,
		)
	}
}

func Service(s *environments.Services) GatewayService {
	if s == nil {
		return nil
	}
	if obj := s.GetService("Gateways"); obj != nil {
		locator := obj.(ServiceLocator)
		return locator()
	}
	return nil
}

func init() {
	registry.RegisterService("Gateways", func(env interface{}) interface{} {
		return NewServiceLocator(env.(*environments.Env))
	})

	pkgserver.RegisterRoutes("gateways", func(apiV1Router *mux.Router, services pkgserver.ServicesInterface, authMiddleware environments.JWTMiddleware, authzMiddleware auth.AuthorizationMiddleware) {
		envServices := services.(*environments.Services)
		var ownerBinding OwnerBindingCreator
		var visibilityFilter GatewayVisibilityFilter
		var ownerLookup GatewayOwnerLookup
		rbService := roleBindings.Service(envServices)
		if rbService != nil {
			ownerBinding = rbac.NewGatewayBootstrapper(rbService)
			visibilityFilter = rbac.NewGatewayVisibilityFilter(func(ctx context.Context, userID string) ([]string, error) {
				ids, svcErr := rbService.FindGatewayIDsByUserID(ctx, userID)
				if svcErr != nil {
					return nil, svcErr
				}
				return ids, nil
			})
			ownerLookup = rbService
		}
		gatewayHandler := NewGatewayHandler(Service(envServices), generic.Service(envServices), ownerBinding, visibilityFilter, ownerLookup)

		gatewaysRouter := apiV1Router.PathPrefix("/gateways").Subrouter()
		gatewaysRouter.HandleFunc("", gatewayHandler.List).Methods(http.MethodGet)
		gatewaysRouter.HandleFunc("/{id}", gatewayHandler.Get).Methods(http.MethodGet)
		gatewaysRouter.HandleFunc("", gatewayHandler.Create).Methods(http.MethodPost)
		gatewaysRouter.HandleFunc("/{id}", gatewayHandler.Patch).Methods(http.MethodPatch)
		gatewaysRouter.HandleFunc("/{id}", gatewayHandler.Delete).Methods(http.MethodDelete)
		gatewaysRouter.Use(authMiddleware.AuthenticateAccountJWT)
		gatewaysRouter.Use(authzMiddleware.AuthorizeApi)
	})

	pkgserver.RegisterController("Gateways", func(manager *controllers.KindControllerManager, services pkgserver.ServicesInterface) {
		gatewayServices := Service(services.(*environments.Services))

		manager.Add(&controllers.ControllerConfig{
			Source: "Gateways",
			Handlers: map[api.EventType][]controllers.ControllerHandlerFunc{
				api.CreateEventType: {gatewayServices.OnUpsert},
				api.UpdateEventType: {gatewayServices.OnUpsert},
				api.DeleteEventType: {gatewayServices.OnDelete},
			},
		})
	})

	pkgserver.RegisterGRPCService("gateways", func(grpcServer *grpc.Server, services pkgserver.ServicesInterface) {
		envServices := services.(*environments.Services)
		gatewayService := Service(envServices)
		genericService := generic.Service(envServices)
		brokerFunc := func() *pkgserver.EventBroker {
			if obj := envServices.GetService("EventBroker"); obj != nil {
				return obj.(*pkgserver.EventBroker)
			}
			return nil
		}
		pb.RegisterGatewayServiceServer(grpcServer, NewGatewayGRPCHandler(gatewayService, genericService, brokerFunc))
	})

	presenters.RegisterPath(Gateway{}, "gateways")
	presenters.RegisterPath(&Gateway{}, "gateways")
	presenters.RegisterKind(Gateway{}, "Gateway")
	presenters.RegisterKind(&Gateway{}, "Gateway")

	db.RegisterMigration(migration())
	db.RegisterMigration(migrationAddProvisioningFields())
	db.RegisterMigration(migrationAddSupervisorImage())
	db.RegisterMigration(migrationAddCredentialDriver())
	db.RegisterMigration(migrationAddConsoleAddress())
	db.RegisterMigration(migrationAddActiveSandboxCount())
	db.RegisterMigration(migrationDropDatabaseConfig())
}
