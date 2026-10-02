package gateways

import (
	"context"
	stderrors "errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/gorilla/mux"
	"google.golang.org/grpc"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/hypershell/components/api-server/pkg/rbac"
	"github.com/openshift-online/hypershell/components/api-server/plugins/managedClusters"
	"github.com/openshift-online/hypershell/components/api-server/plugins/roleBindings"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api/presenters"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/auth"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/controllers"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/db"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/environments"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/registry"
	pkgserver "github.com/openshift-online/rh-trex-ai/components/api-server/pkg/server"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/services"
	"github.com/openshift-online/rh-trex-ai/components/api-server/plugins/events"
	"github.com/openshift-online/rh-trex-ai/components/api-server/plugins/generic"
)

type ServiceLocator struct {
	gateway func() GatewayService
	list    services.GenericService
}

func NewServiceLocator(env *environments.Env) ServiceLocator {
	dao := NewGatewayDao(&env.Database.SessionFactory)
	RegisterGatewayMetrics(dao)

	return ServiceLocator{
		gateway: func() GatewayService {
			return NewGatewayService(
				db.NewAdvisoryLockFactory(env.Database.SessionFactory),
				dao,
				events.Service(&env.Services),
			)
		},
		list: newGatewayListService(&env.Database.SessionFactory),
	}
}

// The roleBindings plugin resolves a binding's gateway to its cluster through
// this locator (it cannot import this package, which imports roleBindings).
var _ roleBindings.GatewayClusterLookupSource = ServiceLocator{}

// GatewayClusterLookup returns the managed cluster a gateway is assigned to,
// including soft-deleted gateways, so the RoleBinding watch and list can scope
// bindings to a cluster by their gateway (managed-cluster-registration.spec.md,
// "Watch Stream Caller Binding").
func (l ServiceLocator) GatewayClusterLookup() roleBindings.GatewayClusterLookup {
	return func(ctx context.Context, gatewayID string) (string, bool, *errors.ServiceError) {
		gateway, svcErr := l.gateway().GetUnscoped(ctx, gatewayID)
		if svcErr != nil {
			if svcErr.Is404() {
				return "", false, nil
			}
			return "", false, svcErr
		}
		return gateway.ClusterId, true, nil
	}
}

func Service(s *environments.Services) GatewayService {
	if s == nil {
		return nil
	}
	if obj := s.GetService("Gateways"); obj != nil {
		locator := obj.(ServiceLocator)
		return locator.gateway()
	}
	return nil
}

func listService(s *environments.Services) services.GenericService {
	if s == nil {
		return nil
	}
	if obj := s.GetService("Gateways"); obj != nil {
		locator := obj.(ServiceLocator)
		return locator.list
	}
	return nil
}

// registeredClusterLookup resolves cluster_id references through the
// managedClusters plugin's service, looked up per call so it follows the
// environment's service registry. It returns nil when the plugin is absent, which
// validateClusterReference treats as fail-closed.
func registeredClusterLookup(s *environments.Services) RegisteredClusterLookup {
	if s == nil || s.GetService("ManagedClusters") == nil {
		return nil
	}
	return func(ctx context.Context, id string) (bool, *errors.ServiceError) {
		svc := managedClusters.Service(s)
		if svc == nil {
			return false, errors.GeneralError("managed cluster service is not available")
		}
		cluster, svcErr := svc.GetRegistered(ctx, id)
		if svcErr != nil {
			return false, svcErr
		}
		return cluster != nil, nil
	}
}

type providerPlacementState struct {
	candidates []PlacementCandidate
}

type placementSnapshot struct {
	providers map[string]*providerPlacementState
	localID   string
}

type registeredPlacementService struct {
	services *environments.Services
	now      func() time.Time
}

func newRegisteredPlacementService(s *environments.Services) *registeredPlacementService {
	return &registeredPlacementService{services: s, now: time.Now}
}

func normalizePlacementProvider(provider string) string {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == "ibm cloud" {
		return "ibm"
	}
	return provider
}

func (p *registeredPlacementService) snapshot(ctx context.Context) (placementSnapshot, *errors.ServiceError) {
	result := placementSnapshot{providers: map[string]*providerPlacementState{"aws": {}, "ibm": {}}}
	svc := managedClusters.Service(p.services)
	if svc == nil {
		return result, errors.GeneralError("managed cluster service is not available")
	}
	clusters, svcErr := svc.All(ctx)
	if svcErr != nil {
		return result, svcErr
	}
	for _, cluster := range clusters {
		if cluster == nil || cluster.OIDCSubject == "" || !PlacementControlPlaneConnected(cluster.LastSeenAt, p.now()) {
			continue
		}
		provider := normalizePlacementProvider(cluster.Provider)
		isLocal := cluster.Name == "local-kind"
		if isLocal {
			if os.Getenv("HYPERSHELL_PLATFORM_KIND") == "true" {
				result.localID = cluster.ID
			}
			continue
		}
		state := result.providers[provider]
		if state == nil {
			continue
		}
		state.candidates = append(state.candidates, PlacementCandidate{ID: cluster.ID, Provider: provider, Visibility: cluster.Visibility, Connected: true})
	}
	return result, nil
}

func (p *registeredPlacementService) selectManaged(snapshot placementSnapshot, network, provider string) (string, error) {
	state := snapshot.providers[provider]
	if state == nil {
		return "", fmt.Errorf("unsupported provider %q", provider)
	}
	return ResolvePlacement(PlacementIntent{Network: network, Provider: provider}, state.candidates)
}

func managedPlacementAvailable(snapshot placementSnapshot, network, provider string) bool {
	state := snapshot.providers[provider]
	if !PlacementSupported(network, provider) || state == nil {
		return false
	}
	for _, candidate := range state.candidates {
		if candidate.Visibility == network {
			return true
		}
	}
	return false
}

func (p *registeredPlacementService) Resolve(ctx context.Context, intent openapi.GatewayPlacementIntent) (string, *errors.ServiceError) {
	snapshot, svcErr := p.snapshot(ctx)
	if svcErr != nil {
		return "", svcErr
	}
	if intent.Mode != nil && *intent.Mode == "local-kind" {
		if os.Getenv("HYPERSHELL_PLATFORM_KIND") != "true" {
			return "", errors.Validation("local-kind placement is only available in the Kind development environment")
		}
		if snapshot.localID == "" {
			return "", errors.Validation("local-kind placement is unavailable")
		}
		return snapshot.localID, nil
	}
	if intent.Network == nil || intent.Provider == nil || !PlacementSupported(*intent.Network, *intent.Provider) {
		return "", errors.Validation("the requested network and provider placement is unsupported")
	}
	selected, err := p.selectManaged(snapshot, *intent.Network, *intent.Provider)
	if err != nil {
		if stderrors.Is(err, errNoEligiblePlacement) {
			return "", errors.Validation("no eligible managed cluster is available for the requested placement")
		}
		glog.Errorf("failed to resolve managed cluster placement provider=%s network=%s: %v", *intent.Provider, *intent.Network, err)
		return "", errors.GeneralError("failed to resolve managed cluster placement")
	}
	return selected, nil
}

func (p *registeredPlacementService) Availability(ctx context.Context) (openapi.GatewayPlacementAvailability, *errors.ServiceError) {
	snapshot, svcErr := p.snapshot(ctx)
	if svcErr != nil {
		return openapi.GatewayPlacementAvailability{}, svcErr
	}
	availability := openapi.GatewayPlacementAvailability{}
	availability.AwsPublic = managedPlacementAvailable(snapshot, "public", "aws")
	availability.AwsVpn = managedPlacementAvailable(snapshot, "vpn", "aws")
	availability.IbmPublic = managedPlacementAvailable(snapshot, "public", "ibm")
	availability.IbmVpn = false
	if !availability.AwsPublic && !availability.AwsVpn {
		reason := "no-eligible-cluster"
		availability.AwsReason = &reason
	}
	if !availability.IbmPublic {
		reason := "no-eligible-cluster"
		availability.IbmReason = &reason
	}
	availability.LocalKind = os.Getenv("HYPERSHELL_PLATFORM_KIND") == "true" && snapshot.localID != ""
	return availability, nil
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
		placement := newRegisteredPlacementService(envServices)
		gatewayHandler := NewGatewayHandler(Service(envServices), listService(envServices), ownerBinding, visibilityFilter, ownerLookup, registeredClusterLookup(envServices), placement.Resolve, placement.Availability)

		gatewaysRouter := apiV1Router.PathPrefix("/gateways").Subrouter()
		gatewaysRouter.HandleFunc("", gatewayHandler.List).Methods(http.MethodGet)
		gatewaysRouter.HandleFunc("/placement-availability", gatewayHandler.GetPlacementAvailability).Methods(http.MethodGet)
		gatewaysRouter.HandleFunc("/{id}", gatewayHandler.Get).Methods(http.MethodGet)
		gatewaysRouter.HandleFunc("", gatewayHandler.Create).Methods(http.MethodPost)
		gatewaysRouter.HandleFunc("/{id}", gatewayHandler.Patch).Methods(http.MethodPatch)
		gatewaysRouter.HandleFunc("/{id}", gatewayHandler.Delete).Methods(http.MethodDelete)
		gatewaysRouter.Use(authMiddleware.AuthenticateAccountJWT)
		gatewaysRouter.Use(authzMiddleware.AuthorizeApi)

		metricsRouter := apiV1Router.PathPrefix("/metrics").Subrouter()
		metricsRouter.HandleFunc("/gateways", gatewayHandler.MetricsGateways).Methods(http.MethodGet)
		metricsRouter.Use(authMiddleware.AuthenticateAccountJWT)
		metricsRouter.Use(authzMiddleware.AuthorizeApi)
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
		placement := newRegisteredPlacementService(envServices)
		pb.RegisterGatewayServiceServer(grpcServer, NewGatewayGRPCHandler(gatewayService, genericService, brokerFunc, registeredClusterLookup(envServices), placement.Resolve))
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
	db.RegisterMigration(migrationAddGatewayVersion())
	db.RegisterMigration(migrationAddObservedReleaseId())
	db.RegisterMigration(migrationDropFleetId())
	db.RegisterMigration(migrationDropFleetsTable())
	db.RegisterMigration(migrationAddTraceContext())
	db.RegisterMigration(migrationAddProvisioningConditions())
	db.RegisterMigration(migrationAddGenerationTracking())
	db.RegisterMigration(migrationDropDatabaseId())
	db.RegisterMigration(migrationDropManagedDatabasesTable())
}
