package gateways

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"

	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/hypershell/components/api-server/pkg/gatewayhealth"
	"github.com/openshift-online/hypershell/components/api-server/pkg/rbac"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api/presenters"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/auth"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/handlers"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/services"
)

type OwnerBindingCreator interface {
	CreateOwnerBinding(ctx context.Context, userID string, gatewayID string) error
}

type GatewayVisibilityFilter interface {
	AccessibleGatewayIDs(ctx context.Context, userID string) ([]string, error)
}

var _ handlers.RestHandler = gatewayHandler{}

type GatewayOwnerLookup interface {
	FindOwnerUsernamesByGatewayIDs(ctx context.Context, gatewayIDs []string) (map[string]string, error)
}

type gatewayHandler struct {
	gateway            GatewayService
	generic            services.GenericService
	ownerBinding       OwnerBindingCreator
	visibilityFilter   GatewayVisibilityFilter
	ownerLookup        GatewayOwnerLookup
	bindingLookup      rbac.RoleBindingLookup
	clusters           RegisteredClusterLookup
	clusterVersions    ClusterProvisioningVersionLookup
	enforceRBAC        bool
	placement          PlacementResolver
	availability       PlacementAvailabilityResolver
	versionLogThrottle *logThrottle
}

// provisioningVersionLogInterval throttles the provisioning-version lookup
// failure log. The lookup runs on every gateway read, so an unthrottled warning
// would flood the log on a persistent failure; throttling keeps a persistent
// failure visible without the flood.
const provisioningVersionLogInterval = time.Minute

// logThrottle rate-limits a recurring log line to at most one emission per
// interval. It is held by pointer on the handler so value copies of the handler
// share the same throttle state (and so tests get an isolated instance rather
// than package-global state).
type logThrottle struct {
	interval time.Duration
	mu       sync.Mutex
	last     time.Time
}

// allow reports whether a log line may be emitted now, recording the emission
// time when it returns true. A nil throttle always allows (unthrottled).
func (t *logThrottle) allow() bool {
	if t == nil {
		return true
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now()
	if !t.last.IsZero() && now.Sub(t.last) < t.interval {
		return false
	}
	t.last = now
	return true
}

// provisioningVersions resolves the provisioning version advertised by each of
// the given clusters. A nil lookup or a lookup error yields an empty map, so the
// read-only version enrichment degrades to absent rather than failing the read
// (consistent with owner-username enrichment). A lookup error is logged, at most
// once per provisioningVersionLogInterval, so a persistent failure that silently
// hides every gateway version is observable.
func (h gatewayHandler) provisioningVersions(ctx context.Context, clusterIDs []string) map[string]string {
	if h.clusterVersions == nil || len(clusterIDs) == 0 {
		return map[string]string{}
	}
	versions, svcErr := h.clusterVersions(ctx, clusterIDs)
	if svcErr != nil {
		h.logProvisioningVersionLookupFailure(ctx, svcErr)
		return map[string]string{}
	}
	return versions
}

// logProvisioningVersionLookupFailure records a provisioning-version lookup
// failure at most once per provisioningVersionLogInterval. It logs only the
// error, which carries no secret material.
func (h gatewayHandler) logProvisioningVersionLookupFailure(ctx context.Context, err error) {
	if h.versionLogThrottle.allow() {
		slog.WarnContext(ctx, "gateway provisioning-version enrichment failed; gateway versions degraded to absent", "error", err)
	}
}

// callerBindings loads the current caller's role bindings once, or nil when RBAC
// is not enforced, no lookup is wired, or the caller is unidentified. Errors
// degrade to nil, which is fail-closed for the can_delete capability: a gateway
// the caller cannot be shown to own is reported as not deletable.
func (h gatewayHandler) callerBindings(ctx context.Context) []rbac.BindingSummary {
	if !h.enforceRBAC || h.bindingLookup == nil {
		return nil
	}
	userID := rbac.GetUserIDFromContext(ctx)
	if userID == "" {
		return nil
	}
	bindings, err := h.bindingLookup.FindBindingsByUserID(ctx, userID)
	if err != nil {
		return nil
	}
	return bindings
}

// callerCanDelete reports whether the caller holding bindings may DELETE the
// gateway, mirroring the RBAC middleware's enforced check exactly
// (rbac.CanDeleteGateway). Advertised as Gateway.can_delete so a client can
// disable a delete affordance the server would reject. When RBAC is not enforced
// the server permits every delete, so it reports true.
func (h gatewayHandler) callerCanDelete(bindings []rbac.BindingSummary, gatewayID string) bool {
	if !h.enforceRBAC {
		return true
	}
	return rbac.CanDeleteGateway(bindings, gatewayID)
}

// callerCanEdit reports whether the caller holding bindings may modify (PATCH,
// e.g. rename) the gateway, mirroring the enforced check (rbac.CanEditGateway).
// Advertised as Gateway.can_edit so a client can disable an edit affordance the
// server would reject. When RBAC is not enforced every edit is permitted, so it
// reports true.
func (h gatewayHandler) callerCanEdit(bindings []rbac.BindingSummary, gatewayID string) bool {
	if !h.enforceRBAC {
		return true
	}
	return rbac.CanEditGateway(bindings, gatewayID)
}

func (h gatewayHandler) GetPlacementAvailability(w http.ResponseWriter, r *http.Request) {
	if h.availability == nil {
		handlers.HandleGet(w, r, &handlers.HandlerConfig{Action: func() (interface{}, *errors.ServiceError) {
			return nil, errors.GeneralError("gateway placement resolver is unavailable")
		}, ErrorHandler: handlers.HandleError})
		return
	}
	var request openapi.GatewayPlacementAvailabilityRequest
	handlers.Handle(w, r, &handlers.HandlerConfig{Body: &request, Action: func() (interface{}, *errors.ServiceError) {
		if len(request.Selectors) == 0 || len(request.Selectors) > 32 {
			return nil, errors.Validation("selectors must contain between 1 and 32 selectors")
		}
		availability, svcErr := h.availability(r.Context(), request.Selectors)
		if svcErr != nil {
			return nil, svcErr
		}
		return availability, nil
	}, ErrorHandler: handlers.HandleError}, http.StatusOK)
}

// resolveGatewayPlacement validates and resolves a late-bound placement intent for
// a gateway being created, mutating the model in place. It persists the placement
// selector so the API server can re-evaluate it, assigns a resolved cluster_id when
// an eligible cluster matches, and otherwise leaves the gateway unplaced in the
// Pending phase with an awaiting-placement status so it binds when a matching
// cluster appears (gateway-provision-placement.spec.md WEB-PLC-07).
func (h gatewayHandler) resolveGatewayPlacement(ctx context.Context, intent openapi.GatewayPlacementIntent, model *Gateway) *errors.ServiceError {
	if placementErr := validatePlacementIntent(&intent); placementErr != nil {
		return placementErr
	}
	if h.placement == nil {
		return errors.GeneralError("gateway placement resolver is unavailable")
	}
	placementJSON, marshalErr := marshalPlacement(intent)
	if marshalErr != nil {
		return errors.GeneralError("failed to persist gateway placement: %s", marshalErr)
	}
	model.Placement = placementJSON
	clusterID, resolved, placementErr := h.placement(ctx, intent)
	if placementErr != nil {
		return placementErr
	}
	if resolved {
		model.ClusterId = clusterID
		return nil
	}
	applyAwaitingPlacement(model)
	return nil
}

// applyAwaitingPlacement marks a late-bound gateway that currently matches no
// eligible cluster as unplaced: empty cluster_id, the canonical Pending phase, and
// a human-readable awaiting-placement status. The API server assigns a cluster_id
// when a matching ManagedCluster becomes available (data-model.spec.md, "Gateway
// Placement and Late Binding").
func applyAwaitingPlacement(model *Gateway) {
	model.ClusterId = ""
	pending := string(gatewayhealth.PhasePending)
	model.Phase = &pending
	status := awaitingPlacementStatus
	model.Status = &status
}

func NewGatewayHandler(gateway GatewayService, generic services.GenericService, ownerBinding OwnerBindingCreator, visibilityFilter GatewayVisibilityFilter, ownerLookup GatewayOwnerLookup, bindingLookup rbac.RoleBindingLookup, clusters RegisteredClusterLookup, clusterVersions ClusterProvisioningVersionLookup, enforceRBAC bool, placement PlacementResolver, availability PlacementAvailabilityResolver) *gatewayHandler {
	h := &gatewayHandler{
		gateway:            gateway,
		generic:            generic,
		ownerBinding:       ownerBinding,
		visibilityFilter:   visibilityFilter,
		ownerLookup:        ownerLookup,
		bindingLookup:      bindingLookup,
		clusters:           clusters,
		clusterVersions:    clusterVersions,
		enforceRBAC:        enforceRBAC,
		placement:          placement,
		availability:       availability,
		versionLogThrottle: &logThrottle{interval: provisioningVersionLogInterval},
	}
	return h
}

func (h gatewayHandler) Create(w http.ResponseWriter, r *http.Request) {
	var gateway openapi.GatewayCreateRequest
	cfg := &handlers.HandlerConfig{
		Body:       &gateway,
		Validators: []handlers.Validate{},
		Action: func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()
			gatewayModel := ConvertGateway(gateway)
			// cluster_id is optional: a gateway is either pinned to a cluster
			// (cluster_id) or late-bound by a placement selector. Exactly one must be
			// supplied. See data-model.spec.md, "Gateway Placement and Late Binding".
			pinnedCluster := strings.TrimSpace(gateway.GetClusterId())
			hasCluster := pinnedCluster != ""
			hasPlacement := gateway.HasPlacement()
			if hasCluster && hasPlacement {
				return nil, errors.Validation("provide exactly one of cluster_id or placement, not both")
			}
			if !hasCluster && !hasPlacement {
				return nil, errors.Validation("a gateway must be created with either a cluster_id (pinned) or a placement selector (late-bound)")
			}
			if hasCluster {
				if clusterErr := validateClusterReference(ctx, h.clusters, pinnedCluster); clusterErr != nil {
					return nil, clusterErr
				}
				gatewayModel.ClusterId = pinnedCluster
			} else {
				if clusterErr := h.resolveGatewayPlacement(ctx, gateway.GetPlacement(), gatewayModel); clusterErr != nil {
					return nil, clusterErr
				}
			}
			gatewayModel, err := h.gateway.Create(ctx, gatewayModel)
			if err != nil {
				return nil, err
			}

			userID := rbac.GetUserIDFromContext(ctx)
			if userID != "" && h.ownerBinding != nil {
				if bindErr := h.ownerBinding.CreateOwnerBinding(ctx, userID, gatewayModel.ID); bindErr != nil {
					return nil, errors.GeneralError("failed to create owner binding: %s", bindErr)
				}
			}

			// The creator just received a gateway:owner binding (or RBAC is off), so
			// they can always delete and edit what they just created.
			versions := h.provisioningVersions(ctx, []string{gatewayModel.ClusterId})
			return PresentGateway(gatewayModel, "", true, true, versions[gatewayModel.ClusterId]), nil
		},
		ErrorHandler: handlers.HandleError,
	}

	handlers.Handle(w, r, cfg, http.StatusCreated)
}

func (h gatewayHandler) Patch(w http.ResponseWriter, r *http.Request) {
	var patch openapi.GatewayPatchRequest

	cfg := &handlers.HandlerConfig{
		Body:       &patch,
		Validators: []handlers.Validate{},
		Action: func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()
			id := mux.Vars(r)["id"]
			found, err := h.gateway.Get(ctx, id)
			if err != nil {
				return nil, err
			}

			// name is the only user-writable field on update (PLAT-DATA-23). Every other
			// gateway field is platform-managed / control-plane-only and is not part of
			// the REST PATCH contract (GatewayPatchRequest exposes only name); the service
			// write funnel enforces the default-deny field policy as the authoritative
			// backstop (SEC-RBAC-18).
			if patch.Name != nil {
				found.Name = *patch.Name
			}

			gatewayModel, err := h.gateway.Replace(ctx, found)
			if err != nil {
				return nil, err
			}
			bindings := h.callerBindings(ctx)
			versions := h.provisioningVersions(ctx, []string{gatewayModel.ClusterId})
			return PresentGateway(gatewayModel, "", h.callerCanDelete(bindings, gatewayModel.ID), h.callerCanEdit(bindings, gatewayModel.ID), versions[gatewayModel.ClusterId]), nil
		},
		ErrorHandler: handlers.HandleError,
	}

	handlers.Handle(w, r, cfg, http.StatusOK)
}

func (h gatewayHandler) List(w http.ResponseWriter, r *http.Request) {
	cfg := &handlers.HandlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()

			listArgs := services.NewListArguments(r.URL.Query())

			userID := rbac.GetUserIDFromContext(ctx)
			hasPlatformAdmin := rbac.HasPlatformAdminRole(ctx, userID)
			if userID != "" && h.visibilityFilter != nil && !hasPlatformAdmin {
				accessibleIDs, filterErr := h.visibilityFilter.AccessibleGatewayIDs(ctx, userID)
				if filterErr != nil {
					return nil, errors.GeneralError("failed to check gateway access: %s", filterErr)
				}
				if len(accessibleIDs) == 0 {
					kindStr := "GatewayList"
					pageVal := int32(listArgs.Page)
					sizeVal := int32(0)
					totalVal := int32(0)
					return openapi.GatewayList{
						Kind:  &kindStr,
						Page:  &pageVal,
						Size:  &sizeVal,
						Total: &totalVal,
						Items: []openapi.Gateway{},
					}, nil
				}
				idFilter := visibilitySearchFilter(accessibleIDs)
				if listArgs.Search != "" {
					listArgs.Search = "(" + listArgs.Search + ") and " + idFilter
				} else {
					listArgs.Search = idFilter
				}
			}

			var gateways []Gateway
			paging, err := h.generic.List(ctx, "id", listArgs, &gateways)
			if err != nil {
				return nil, err
			}

			kindStr := "GatewayList"
			pageVal := int32(paging.Page)
			sizeVal := int32(paging.Size)
			totalVal := int32(paging.Total)
			gatewayList := openapi.GatewayList{
				Kind:  &kindStr,
				Page:  &pageVal,
				Size:  &sizeVal,
				Total: &totalVal,
				Items: []openapi.Gateway{},
			}

			gatewayIDs := make([]string, len(gateways))
			clusterIDs := make([]string, 0, len(gateways))
			seenCluster := make(map[string]struct{}, len(gateways))
			for i, gw := range gateways {
				gatewayIDs[i] = gw.ID
				if gw.ClusterId == "" {
					continue
				}
				if _, seen := seenCluster[gw.ClusterId]; seen {
					continue
				}
				seenCluster[gw.ClusterId] = struct{}{}
				clusterIDs = append(clusterIDs, gw.ClusterId)
			}
			ownerUsernames := map[string]string{}
			if h.ownerLookup != nil {
				if owners, lookupErr := h.ownerLookup.FindOwnerUsernamesByGatewayIDs(ctx, gatewayIDs); lookupErr == nil {
					ownerUsernames = owners
				}
			}
			versions := h.provisioningVersions(ctx, clusterIDs)
			bindings := h.callerBindings(ctx)
			for _, gateway := range gateways {
				converted := PresentGateway(&gateway, ownerUsernames[gateway.ID], h.callerCanDelete(bindings, gateway.ID), h.callerCanEdit(bindings, gateway.ID), versions[gateway.ClusterId])
				gatewayList.Items = append(gatewayList.Items, converted)
			}
			if listArgs.Fields != nil {
				filteredItems, err := presenters.SliceFilter(listArgs.Fields, gatewayList.Items)
				if err != nil {
					return nil, err
				}
				return filteredItems, nil
			}
			return gatewayList, nil
		},
	}

	handlers.HandleList(w, r, cfg)
}

func visibilitySearchFilter(ids []string) string {
	quoted := make([]string, len(ids))
	for i, id := range ids {
		quoted[i] = "'" + id + "'"
	}
	return "id in (" + strings.Join(quoted, ",") + ")"
}

func (h gatewayHandler) Get(w http.ResponseWriter, r *http.Request) {
	cfg := &handlers.HandlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			id := mux.Vars(r)["id"]
			ctx := r.Context()
			gateway, err := h.gateway.Get(ctx, id)
			if err != nil {
				return nil, err
			}

			userID := rbac.GetUserIDFromContext(ctx)
			if rbac.HasPlatformAdminRole(ctx, userID) {
				username := auth.GetUsernameFromContext(ctx)
				slog.InfoContext(ctx, "platform:admin action", "action", "view", "gateway_id", gateway.ID, "gateway_name", gateway.Name, "user", username, "user_id", userID)
			}

			bindings := h.callerBindings(ctx)
			versions := h.provisioningVersions(ctx, []string{gateway.ClusterId})
			return PresentGateway(gateway, "", h.callerCanDelete(bindings, gateway.ID), h.callerCanEdit(bindings, gateway.ID), versions[gateway.ClusterId]), nil
		},
	}

	handlers.HandleGet(w, r, cfg)
}

func (h gatewayHandler) Delete(w http.ResponseWriter, r *http.Request) {
	cfg := &handlers.HandlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			id := mux.Vars(r)["id"]
			ctx := r.Context()

			userID := rbac.GetUserIDFromContext(ctx)
			isPlatformAdmin := rbac.HasPlatformAdminRole(ctx, userID)

			var gatewayName string
			if isPlatformAdmin {
				gateway, getErr := h.gateway.Get(ctx, id)
				if getErr == nil {
					gatewayName = gateway.Name
				}
			}

			err := h.gateway.Delete(ctx, id)
			if err != nil {
				return nil, err
			}

			if isPlatformAdmin {
				username := auth.GetUsernameFromContext(ctx)
				slog.InfoContext(ctx, "platform:admin action", "action", "delete", "gateway_id", id, "gateway_name", gatewayName, "user", username, "user_id", userID)
			}

			return nil, nil
		},
	}
	handlers.HandleDelete(w, r, cfg, http.StatusNoContent)
}

func (h gatewayHandler) MetricsGateways(w http.ResponseWriter, r *http.Request) {
	counts, svcErr := h.gateway.CountByPhase(r.Context())
	if svcErr != nil {
		http.Error(w, svcErr.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{"counts": counts}); err != nil {
		slog.ErrorContext(r.Context(), "Failed to encode gateway metrics response", "error", err)
	}
}
