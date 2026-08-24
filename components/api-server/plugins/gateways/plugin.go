package gateways

import (
	"context"
	"net/http"
	"os"

	"github.com/golang/glog"
	"github.com/gorilla/mux"
	"google.golang.org/grpc"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/api-server/pkg/rbac"
	"github.com/openshift-online/hypershell/components/api-server/plugins/fleets"
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

type fleetLookupAdapter struct {
	fleetSvc   fleets.FleetService
	clusterSvc managedClusters.ManagedClusterService
}

func (a *fleetLookupAdapter) FleetIDForCluster(ctx context.Context, clusterID string) (string, error) {
	if a.clusterSvc == nil {
		return "", nil
	}
	cluster, err := a.clusterSvc.Get(ctx, clusterID)
	if err != nil {
		return "", err
	}
	return cluster.FleetId, nil
}

func (a *fleetLookupAdapter) FindSoleFleet(ctx context.Context) (string, error) {
	if a.fleetSvc == nil {
		return "", nil
	}
	all, err := a.fleetSvc.All(ctx)
	if err != nil {
		return "", err
	}
	if len(all) == 1 {
		return all[0].ID, nil
	}
	return "", nil
}

type dbLookupAdapter struct {
	svc managedDatabases.ManagedDatabaseService
}

func (a *dbLookupAdapter) FindSoleInFleet(ctx context.Context, fleetID string) (string, error) {
	d, err := a.svc.FindSoleInFleet(ctx, fleetID)
	if err != nil {
		return "", err
	}
	if d == nil {
		return "", nil
	}
	return d.ID, nil
}

func (a *dbLookupAdapter) FindSole(ctx context.Context) (string, string, error) {
	all, err := a.svc.All(ctx)
	if err != nil {
		return "", "", err
	}
	if len(all) == 1 {
		return all[0].ID, all[0].FleetId, nil
	}
	return "", "", nil
}

type dbCreatorAdapter struct {
	svc      managedDatabases.ManagedDatabaseService
	provider string
}

func (a *dbCreatorAdapter) CreateForGateway(ctx context.Context, gatewayName, fleetID string) (string, error) {
	d, err := a.svc.Create(ctx, &managedDatabases.ManagedDatabase{
		Name:     "gw-" + gatewayName + "-db",
		FleetId:  fleetID,
		Provider: a.provider,
	})
	if err != nil {
		return "", err
	}
	return d.ID, nil
}

func NewServiceLocator(env *environments.Env) ServiceLocator {
	dao := NewGatewayDao(&env.Database.SessionFactory)
	RegisterGatewayMetrics(dao)

	// Resolved once at server startup (this factory runs a single time via
	// registry.LoadDiscoveredServices), not per request: an unsupported
	// DATABASE_PROVIDER value is a startup configuration error, so it must
	// fail here rather than surface later as a silently-wrong placement
	// choice on the first gateway creation request.
	databaseProvider, err := resolveDatabaseProvider(os.Getenv("DATABASE_PROVIDER"))
	if err != nil {
		glog.Fatalf("gateways: %v", err)
	}

	return func() GatewayService {
		fleetLookup := &fleetLookupAdapter{
			fleetSvc:   fleets.Service(&env.Services),
			clusterSvc: managedClusters.Service(&env.Services),
		}

		var placement PlacementResolver
		if mdSvc := managedDatabases.Service(&env.Services); mdSvc != nil {
			if databaseProvider == ProviderDeployment {
				placement = NewDeploymentPlacement(fleetLookup, &dbCreatorAdapter{svc: mdSvc, provider: databaseProvider})
			} else {
				placement = NewCNPGPlacement(fleetLookup, &dbLookupAdapter{svc: mdSvc})
			}
		}

		return NewGatewayService(
			db.NewAdvisoryLockFactory(env.Database.SessionFactory),
			dao,
			events.Service(&env.Services),
			placement,
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
