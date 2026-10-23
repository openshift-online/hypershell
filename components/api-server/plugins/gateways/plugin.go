package gateways

import (
	"context"
	stderrors "errors"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"google.golang.org/grpc"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/hypershell/components/api-server/pkg/rbac"
	"github.com/openshift-online/hypershell/components/api-server/plugins/managedClusters"
	"github.com/openshift-online/hypershell/components/api-server/plugins/roleBindings"
	"github.com/openshift-online/hypershell/components/api-server/plugins/sandboxEvents"
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

// gatewaySubjectResolver adapts the managedClusters service to the
// pkgrbac.RegisteredClusterResolver the gateway gRPC handler uses to classify a caller as
// a control-plane identity for the field-ownership write policy (PLAT-DATA-23). It is
// looked up per call so it follows the environment's service registry; a nil service means
// no caller resolves to a control plane (every caller is a user/import, the safe default).
type gatewaySubjectResolver struct {
	services *environments.Services
}

func (r gatewaySubjectResolver) RegisteredClusterIDForSubject(ctx context.Context, subject string) (string, bool, error) {
	svc := managedClusters.Service(r.services)
	if svc == nil {
		return "", false, nil
	}
	cluster, svcErr := svc.FindRegisteredBySubject(ctx, subject)
	if svcErr != nil {
		return "", false, svcErr
	}
	if cluster == nil {
		return "", false, nil
	}
	return cluster.ID, true, nil
}

// registeredClusterVersions resolves each cluster's advertised provisioning
// version (its gateway-version capability) through the managedClusters plugin,
// looked up per call so it follows the environment's service registry. It returns
// nil when the plugin is absent; the gateway handler treats a nil lookup as "no
// versions" so version enrichment degrades to absent rather than failing a read.
func registeredClusterVersions(s *environments.Services) ClusterProvisioningVersionLookup {
	if s == nil || s.GetService("ManagedClusters") == nil {
		return nil
	}
	return func(ctx context.Context, clusterIDs []string) (map[string]string, *errors.ServiceError) {
		svc := managedClusters.Service(s)
		if svc == nil {
			return nil, errors.GeneralError("managed cluster service is not available")
		}
		clusters, svcErr := svc.FindByIDs(ctx, clusterIDs)
		if svcErr != nil {
			return nil, svcErr
		}
		versions := make(map[string]string, len(clusters))
		for _, cluster := range clusters {
			if cluster == nil {
				continue
			}
			if version := managedClusters.GatewayVersionCapability(cluster.Capabilities); version != "" {
				versions[cluster.ID] = version
			}
		}
		return versions, nil
	}
}

type placementSnapshot struct {
	candidates []PlacementCandidate
	localID    string
}

type registeredPlacementService struct {
	services *environments.Services
	now      func() time.Time
}

func newRegisteredPlacementService(s *environments.Services) *registeredPlacementService {
	return &registeredPlacementService{services: s, now: time.Now}
}

func (p *registeredPlacementService) snapshot(ctx context.Context) (placementSnapshot, *errors.ServiceError) {
	result := placementSnapshot{}
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
		isLocal := cluster.Name == "local-kind"
		if isLocal {
			if os.Getenv("HYPERSHELL_PLATFORM_KIND") == "true" {
				result.localID = cluster.ID
			}
			continue
		}
		if len(cluster.Capabilities) == 0 {
			continue
		}
		result.candidates = append(result.candidates, PlacementCandidate{ID: cluster.ID, Capabilities: cluster.Capabilities, Connected: true})
	}
	return result, nil
}

func (p *registeredPlacementService) selectManaged(snapshot placementSnapshot, intent PlacementIntent) (string, error) {
	return ResolvePlacement(intent, snapshot.candidates)
}

func managedPlacementAvailable(snapshot placementSnapshot, intent PlacementIntent) bool {
	for _, candidate := range snapshot.candidates {
		if capabilitySetContains(candidate.Capabilities, intent.Selector) &&
			capabilitySetExcludes(candidate.Capabilities, intent.ExcludedSelector) {
			return true
		}
	}
	return false
}

// Resolve implements PlacementResolver. A local-kind intent that cannot be
// satisfied is a hard error (WEB-PLC-08), while a capability-label selector that
// matches no eligible cluster yields resolved=false with a nil error so the
// gateway is late-bound (created Pending, awaiting placement) rather than rejected
// (WEB-PLC-07).
func (p *registeredPlacementService) Resolve(ctx context.Context, intent openapi.GatewayPlacementIntent) (string, bool, *errors.ServiceError) {
	snapshot, svcErr := p.snapshot(ctx)
	if svcErr != nil {
		return "", false, svcErr
	}
	if intent.Mode != nil && *intent.Mode == "local-kind" {
		if os.Getenv("HYPERSHELL_PLATFORM_KIND") != "true" {
			return "", false, errors.Validation("local-kind placement is only available in the Kind development environment")
		}
		if snapshot.localID == "" {
			return "", false, errors.Validation("local-kind placement is unavailable")
		}
		return snapshot.localID, true, nil
	}
	selector, validationErr := managedClusters.NormalizeSelector(intent.Selector)
	if validationErr != nil {
		return "", false, validationErr
	}
	excludedSelector := intent.ExcludedSelector
	if excludedSelector != nil {
		excludedSelector, validationErr = managedClusters.NormalizeSelector(excludedSelector)
		if validationErr != nil {
			return "", false, validationErr
		}
	}
	selected, err := p.selectManaged(snapshot, PlacementIntent{
		Selector:         selector,
		ExcludedSelector: excludedSelector,
	})
	if err != nil {
		if stderrors.Is(err, errNoEligiblePlacement) {
			// No connected cluster matches the selector yet. Late-bind: the caller
			// creates the gateway Pending and awaiting placement, and the API server
			// assigns a cluster when a matching ManagedCluster becomes available.
			return "", false, nil
		}
		slog.Error(
			"failed to resolve managed cluster placement",
			"selector",
			selector,
			"excludedSelector",
			excludedSelector,
			"error",
			err,
		)
		return "", false, errors.GeneralError("failed to resolve managed cluster placement")
	}
	return selected, true, nil
}

func (p *registeredPlacementService) Availability(ctx context.Context, selectors [][]string) (openapi.GatewayPlacementAvailability, *errors.ServiceError) {
	snapshot, svcErr := p.snapshot(ctx)
	if svcErr != nil {
		return openapi.GatewayPlacementAvailability{}, svcErr
	}
	availability := openapi.GatewayPlacementAvailability{Placements: make([]openapi.GatewayPlacementAvailabilityItem, 0, len(selectors))}
	seen := make(map[string]struct{}, len(selectors))
	for _, requested := range selectors {
		selector, validationErr := managedClusters.NormalizeSelector(requested)
		if validationErr != nil {
			return openapi.GatewayPlacementAvailability{}, validationErr
		}
		key := strings.Join(selector, "\x00")
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		available := managedPlacementAvailable(snapshot, PlacementIntent{Selector: selector})
		item := openapi.GatewayPlacementAvailabilityItem{Selector: selector, Available: available}
		if !available {
			reason := "no-eligible-cluster"
			item.Reason = &reason
		}
		availability.Placements = append(availability.Placements, item)
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
		var bindingLookup rbac.RoleBindingLookup
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
			bindingLookup = rbService
		}
		enforceRBAC := os.Getenv("RBAC_ENFORCE") == "true"
		placement := newRegisteredPlacementService(envServices)
		gatewayService := Service(envServices)
		gatewayHandler := NewGatewayHandler(gatewayService, listService(envServices), ownerBinding, visibilityFilter, ownerLookup, bindingLookup, registeredClusterLookup(envServices), registeredClusterVersions(envServices), enforceRBAC, placement.Resolve, placement.Availability)

		sandboxEventsHandler := sandboxEvents.NewHandler(sandboxEvents.Store(envServices), func(ctx context.Context, gatewayID string) (string, bool) {
			gateway, serviceErr := gatewayService.Get(ctx, gatewayID)
			if serviceErr != nil || gateway == nil || gateway.Namespace == "" {
				return "", false
			}
			return gateway.Namespace, true
		})
		sandboxEventsRouter := apiV1Router.PathPrefix("/gateways/{gateway_id}/sandbox_events").Subrouter()
		sandboxEventsRouter.HandleFunc("", sandboxEventsHandler.List).Methods(http.MethodGet)
		sandboxEventsRouter.Use(authMiddleware.AuthenticateAccountJWT)
		sandboxEventsRouter.Use(authzMiddleware.AuthorizeApi)

		gatewaysRouter := apiV1Router.PathPrefix("/gateways").Subrouter()
		gatewaysRouter.HandleFunc("", gatewayHandler.List).Methods(http.MethodGet)
		gatewaysRouter.HandleFunc("/placement-availability", gatewayHandler.GetPlacementAvailability).Methods(http.MethodPost)
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

	// Late binding: when a ManagedCluster registers, is restored, or changes its
	// capability set, re-resolve every gateway that is still awaiting placement and
	// assign a cluster_id to any whose selector now matches. ManagedCluster
	// heartbeats that do NOT change the capability set emit no broker event, so this
	// fires on "a matching cluster appears or gains the capability" rather than on
	// every heartbeat. A new registration/restore emits a Create event; a changed
	// capability set emits an Update event (managedClusters.Register). See
	// data-model.spec.md, "Gateway Placement and Late Binding", and control-plane.spec.md.
	pkgserver.RegisterController("GatewayPlacementRebinder", func(manager *controllers.KindControllerManager, services pkgserver.ServicesInterface) {
		envServices := services.(*environments.Services)
		gatewayService := Service(envServices)
		placement := newRegisteredPlacementService(envServices)
		rebind := func(ctx context.Context, _ string) error {
			return resolvePendingPlacements(ctx, gatewayService, placement.Resolve)
		}
		manager.Add(&controllers.ControllerConfig{
			Source: "ManagedClusters",
			Handlers: map[api.EventType][]controllers.ControllerHandlerFunc{
				api.CreateEventType: {rebind},
				api.UpdateEventType: {rebind},
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
		grpcHandler := NewGatewayGRPCHandler(gatewayService, genericService, brokerFunc, registeredClusterLookup(envServices), gatewaySubjectResolver{services: envServices}, placement.Resolve)
		SetSandboxEventStore(grpcHandler, sandboxEvents.Store(envServices))
		pb.RegisterGatewayServiceServer(grpcServer, grpcHandler)
	})

	presenters.RegisterPath(Gateway{}, "gateways")
	presenters.RegisterPath(&Gateway{}, "gateways")
	presenters.RegisterKind(Gateway{}, "Gateway")
	presenters.RegisterKind(&Gateway{}, "Gateway")

	db.RegisterMigration(migration())
	db.RegisterMigration(migrationAddProvisioningFields())
	db.RegisterMigration(migrationAddSupervisorImage())
	db.RegisterMigration(migrationAddCredentialDriver())
	db.RegisterMigration(migrationBackfillLegacyCredentialDriverKEK())
	db.RegisterMigration(migrationAddConsoleAddress())
	db.RegisterMigration(migrationAddActiveSandboxCount())
	db.RegisterMigration(migrationDropDatabaseConfig())
	db.RegisterMigration(migrationAddGatewayVersion())
	db.RegisterMigration(migrationDropFleetId())
	db.RegisterMigration(migrationDropFleetsTable())
	db.RegisterMigration(migrationAddTraceContext())
	db.RegisterMigration(migrationAddProvisioningConditions())
	db.RegisterMigration(migrationAddGenerationTracking())
	db.RegisterMigration(migrationDropDatabaseId())
	db.RegisterMigration(migrationDropManagedDatabasesTable())
	db.RegisterMigration(migrationDropReleaseFK())
	db.RegisterMigration(migrationDropGatewayReleasesTable())
	db.RegisterMigration(migrationDropGatewayNetworksTable())
	db.RegisterMigration(migrationAddRepositoryOwnership())
	db.RegisterMigration(migrationAddPlacement())
	db.RegisterMigration(migrationAddGatewayWorkspaceMode())
}
