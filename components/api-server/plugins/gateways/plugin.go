package gateways

import (
	"context"
	stderrors "errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"google.golang.org/grpc"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/hypershell/components/api-server/pkg/extsearch"
	"github.com/openshift-online/hypershell/components/api-server/pkg/rbac"
	"github.com/openshift-online/hypershell/components/api-server/pkg/routes"
	"github.com/openshift-online/hypershell/components/api-server/plugins/managedClusters"
	"github.com/openshift-online/hypershell/components/api-server/plugins/roleBindings"
	"github.com/openshift-online/hypershell/components/api-server/plugins/sandboxEvents"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api/presenters"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/authz"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/controllers"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/db"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/environments"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/registry"
	pkgserver "github.com/openshift-online/rh-trex-ai/components/api-server/pkg/server"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/services"
	"github.com/openshift-online/rh-trex-ai/components/api-server/plugins/events"
)

type ServiceLocator struct {
	gateway   func() GatewayService
	list      services.GenericService
	ownership GatewayOwnershipLookup
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
				gatewayBindingCleaner(&env.Services),
			)
		},
		list:      newGatewayListService(&env.Database.SessionFactory),
		ownership: newGatewayOwnershipDao(&env.Database.SessionFactory),
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

// gatewayBindingCleaner deletes a gateway's role bindings through the role
// bindings plugin, looked up per call so it follows the service registry. It
// is a no-op when that plugin is absent.
func gatewayBindingCleaner(s *environments.Services) GatewayBindingCleaner {
	return func(ctx context.Context, gatewayID string) *errors.ServiceError {
		rbService := roleBindings.Service(s)
		if rbService == nil {
			return nil
		}
		removed, svcErr := rbService.DeleteGatewayBindings(ctx, gatewayID)
		if svcErr != nil {
			return svcErr
		}
		slog.InfoContext(ctx, "Deleted role bindings of deleted gateway", "gateway_id", gatewayID, "count", removed)
		return nil
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

// gatewayBindingLookup resolves a caller's role bindings through the role bindings
// plugin, looked up per call so it follows the environment's service registry. It is
// nil when that plugin is absent; the dedicated_route create gate treats a nil
// lookup as fail-closed.
func gatewayBindingLookup(s *environments.Services) rbac.RoleBindingLookup {
	svc := roleBindings.Service(s)
	if svc == nil {
		return nil
	}
	return svc
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

func ownershipLookup(s *environments.Services) GatewayOwnershipLookup {
	if s == nil {
		return nil
	}
	if obj := s.GetService("Gateways"); obj != nil {
		locator := obj.(ServiceLocator)
		return locator.ownership
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

type placementSnapshot struct {
	candidates []PlacementCandidate
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

// withoutClusters returns a snapshot with the named cluster ids removed from the
// candidate set. It backs the excluded managed-cluster/<name> identity token
// (PLAT-DATA-27): an admin may late-bind by capability while excluding one specific
// cluster. An empty exclusion set returns the snapshot unchanged.
func (s placementSnapshot) withoutClusters(ids map[string]struct{}) placementSnapshot {
	if len(ids) == 0 {
		return s
	}
	filtered := make([]PlacementCandidate, 0, len(s.candidates))
	for _, candidate := range s.candidates {
		if _, excluded := ids[candidate.ID]; excluded {
			continue
		}
		filtered = append(filtered, candidate)
	}
	return placementSnapshot{candidates: filtered}
}

// lookupByName resolves a ManagedCluster by its unique name through the
// managedClusters service, following the environment's service registry. A missing
// service is a server error; a name that matches no record returns (nil, nil).
func (p *registeredPlacementService) lookupByName(ctx context.Context, name string) (*managedClusters.ManagedCluster, *errors.ServiceError) {
	svc := managedClusters.Service(p.services)
	if svc == nil {
		return nil, errors.GeneralError("managed cluster service is not available")
	}
	return svc.FindByName(ctx, name)
}

// identityClusterIDs resolves managed-cluster/<name> excluded-identity names to the
// ids of any matching records. An unknown name contributes nothing (excluding a
// cluster that does not exist is a well-defined no-op), keeping late-binding
// re-resolution robust when an excluded cluster is later deleted.
func (p *registeredPlacementService) identityClusterIDs(ctx context.Context, names []string) (map[string]struct{}, *errors.ServiceError) {
	ids := make(map[string]struct{}, len(names))
	for _, name := range names {
		record, svcErr := p.lookupByName(ctx, name)
		if svcErr != nil {
			return nil, svcErr
		}
		if record != nil {
			ids[record.ID] = struct{}{}
		}
	}
	return ids, nil
}

// resolveIdentitySelection resolves a required managed-cluster/<name> identity token
// deterministically (PLAT-DATA-27): it looks the cluster up by name and applies the
// fail-fast rules. At most one identity target is allowed; more than one is
// contradictory and rejected with 400.
func (p *registeredPlacementService) resolveIdentitySelection(ctx context.Context, requiredIdentity, requiredCaps, excludedIdentity, excludedCaps []string) (string, *errors.ServiceError) {
	if len(requiredIdentity) > 1 {
		return "", errors.Validation("a placement selector may target at most one managed-cluster/<name> identity; got %d", len(requiredIdentity))
	}
	name := requiredIdentity[0]
	normalizedRequiredCaps, svcErr := normalizeSelectorMaybeEmpty(requiredCaps)
	if svcErr != nil {
		return "", svcErr
	}
	normalizedExcludedCaps, svcErr := normalizeSelectorMaybeEmpty(excludedCaps)
	if svcErr != nil {
		return "", svcErr
	}
	record, svcErr := p.lookupByName(ctx, name)
	if svcErr != nil {
		return "", svcErr
	}
	var identity *managedClusterIdentity
	if record != nil {
		identity = &managedClusterIdentity{ID: record.ID, Capabilities: record.Capabilities, Registered: record.OIDCSubject != ""}
	}
	return resolveManagedClusterIdentitySelection(name, identity, normalizedRequiredCaps, excludedIdentity, normalizedExcludedCaps)
}

// identityAvailable reports whether a probed managed-cluster/<name> selector would
// resolve: the named cluster must exist, be registered, and advertise every
// accompanying capability label. An unknown, unregistered, or non-matching target is
// simply unavailable (the availability probe reports, it does not reject); the
// platform-admin gate already ran before this (SEC-RBAC-27.03).
func (p *registeredPlacementService) identityAvailable(ctx context.Context, requiredIdentity, requiredCaps []string) (bool, *errors.ServiceError) {
	if len(requiredIdentity) != 1 {
		return false, nil
	}
	record, svcErr := p.lookupByName(ctx, requiredIdentity[0])
	if svcErr != nil {
		return false, svcErr
	}
	if record == nil || record.OIDCSubject == "" {
		return false, nil
	}
	return capabilitySetContains(record.Capabilities, requiredCaps), nil
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

// Resolve implements PlacementResolver. A capability-label selector that
// matches no eligible cluster yields resolved=false with a nil error so the
// gateway is late-bound (created Pending, awaiting placement) rather than rejected
// (WEB-PLC-07).
func (p *registeredPlacementService) Resolve(ctx context.Context, intent openapi.GatewayPlacementIntent) (string, bool, *errors.ServiceError) {
	requiredIdentity, requiredCaps := splitManagedClusterSelector(intent.Selector)
	excludedIdentity, excludedCaps := splitManagedClusterSelector(intent.ExcludedSelector)

	// Deterministic single-cluster targeting (PLAT-DATA-27): a required
	// managed-cluster/<name> token resolves by name and fails fast, unlike a
	// capability selector that late-binds when no eligible cluster matches yet.
	if len(requiredIdentity) > 0 {
		clusterID, svcErr := p.resolveIdentitySelection(ctx, requiredIdentity, requiredCaps, excludedIdentity, excludedCaps)
		if svcErr != nil {
			return "", false, svcErr
		}
		return clusterID, true, nil
	}

	snapshot, svcErr := p.snapshot(ctx)
	if svcErr != nil {
		return "", false, svcErr
	}
	// An excluded managed-cluster/<name> token removes that specific cluster from the
	// candidate set before capability matching (PLAT-DATA-27).
	if len(excludedIdentity) > 0 {
		excludedIDs, idErr := p.identityClusterIDs(ctx, excludedIdentity)
		if idErr != nil {
			return "", false, idErr
		}
		snapshot = snapshot.withoutClusters(excludedIDs)
	}
	selector, validationErr := managedClusters.NormalizeSelector(requiredCaps)
	if validationErr != nil {
		return "", false, validationErr
	}
	excludedSelector, validationErr := normalizeSelectorMaybeEmpty(excludedCaps)
	if validationErr != nil {
		return "", false, validationErr
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
		normalized, validationErr := normalizePlacementSelector(requested)
		if validationErr != nil {
			return openapi.GatewayPlacementAvailability{}, validationErr
		}
		key := strings.Join(normalized, "\x00")
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		requiredIdentity, requiredCaps := splitManagedClusterSelector(normalized)
		var available bool
		if len(requiredIdentity) > 0 {
			// A probed managed-cluster/<name> selector is available iff the named
			// cluster resolves and advertises every accompanying capability label
			// (PLAT-DATA-27); it never late-binds (WEB-PLC-07.06).
			avail, availErr := p.identityAvailable(ctx, requiredIdentity, requiredCaps)
			if availErr != nil {
				return openapi.GatewayPlacementAvailability{}, availErr
			}
			available = avail
		} else {
			available = managedPlacementAvailable(snapshot, PlacementIntent{Selector: requiredCaps})
		}
		item := openapi.GatewayPlacementAvailabilityItem{Selector: normalized, Available: available}
		if !available {
			reason := "no-eligible-cluster"
			item.Reason = &reason
		}
		availability.Placements = append(availability.Placements, item)
	}
	return availability, nil
}

func init() {
	registry.RegisterService("Gateways", func(env interface{}) interface{} {
		return NewServiceLocator(env.(*environments.Env))
	})

	pkgserver.RegisterRoutes("gateways", func(apiRouter *pkgserver.APIRouter, services pkgserver.ServicesInterface) {
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
		enforceRBAC := rbac.Enforced()
		placement := newRegisteredPlacementService(envServices)
		gatewayService := Service(envServices)
		gatewayHandler := NewGatewayHandler(gatewayService, listService(envServices), ownerBinding, visibilityFilter, ownerLookup, ownershipLookup(envServices), bindingLookup, registeredClusterLookup(envServices), enforceRBAC, placement.Resolve, placement.Availability)

		sandboxEventsHandler := sandboxEvents.NewHandler(sandboxEvents.Store(envServices), func(ctx context.Context, gatewayID string) (string, bool) {
			gateway, serviceErr := gatewayService.Get(ctx, gatewayID)
			if serviceErr != nil || gateway == nil || gateway.Namespace == "" {
				return "", false
			}
			return gateway.Namespace, true
		})
		sandboxEventsRouter := routes.Resource(apiRouter, "sandbox_events").PathPrefix("/gateways/{gateway_id}/sandbox_events").Subrouter()
		sandboxEventsRouter.HandleFunc("", sandboxEventsHandler.List).Methods(http.MethodGet)

		gatewaysRouter := routes.Resource(apiRouter, "gateways").PathPrefix("/gateways").Subrouter()
		gatewaysRouter.HandleFunc("", gatewayHandler.List).Methods(http.MethodGet)
		gatewaysRouter.HandleFunc("/placement-availability", gatewayHandler.GetPlacementAvailability).Methods(http.MethodPost)
		gatewaysRouter.HandleFunc("/{id}", gatewayHandler.Get).Methods(http.MethodGet)
		gatewaysRouter.HandleFunc("", gatewayHandler.Create).Methods(http.MethodPost)
		gatewaysRouter.HandleFunc("/{id}", gatewayHandler.Patch).Methods(http.MethodPatch)
		gatewaysRouter.HandleFunc("/{id}", gatewayHandler.Delete).Methods(http.MethodDelete)

		metricsRouter := routes.Resource(apiRouter, "metrics").PathPrefix("/metrics").Subrouter()
		metricsRouter.HandleFunc("/gateways", gatewayHandler.MetricsGateways).Methods(http.MethodGet)
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
		genericService := extsearch.Service(envServices)
		brokerFunc := func() *pkgserver.EventBroker {
			if obj := envServices.GetService("EventBroker"); obj != nil {
				return obj.(*pkgserver.EventBroker)
			}
			return nil
		}
		placement := newRegisteredPlacementService(envServices)
		grpcHandler := NewGatewayGRPCHandler(gatewayService, genericService, brokerFunc, registeredClusterLookup(envServices), gatewaySubjectResolver{services: envServices}, newDedicatedRouteAuthorizer(gatewayBindingLookup(envServices), rbac.Enforced()), newManagedClusterSelectorAuthorizer(gatewayBindingLookup(envServices), rbac.Enforced()), placement.Resolve)
		SetSandboxEventStore(grpcHandler, sandboxEvents.Store(envServices))
		pb.RegisterGatewayServiceServer(grpcServer, grpcHandler)
	}, gatewayGRPCResources())

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
	db.RegisterMigration(migrationAddGatewayDedicatedRoute())
	db.RegisterMigration(migrationBackfillGatewayWorkspaceModeShared())
}

// gatewayGRPCResources declares every GatewayService method for authorization.
// The control-plane-owned writes are updates of the gateway; sandbox events are
// created under it.
func gatewayGRPCResources() authz.GRPCResources {
	resources := authz.CRUDResources(pb.GatewayService_ServiceDesc.ServiceName, "gateways", "Gateway", "Gateways")
	method := func(name string) string { return "/" + pb.GatewayService_ServiceDesc.ServiceName + "/" + name }
	resources[method("AdjustActiveSandboxCount")] = authz.GRPCResource{Kind: "gateways", Action: authz.ActionUpdate}
	resources[method("SetActiveSandboxCount")] = authz.GRPCResource{Kind: "gateways", Action: authz.ActionUpdate}
	resources[method("SetGatewayVersion")] = authz.GRPCResource{Kind: "gateways", Action: authz.ActionUpdate}
	resources[method("ReportSandboxEvents")] = authz.GRPCResource{Kind: "sandbox_events", Action: authz.ActionCreate}
	return resources
}
