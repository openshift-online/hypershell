package managedClusters

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"

	"github.com/golang-jwt/jwt/v4"
	"github.com/gorilla/mux"

	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/hypershell/components/api-server/pkg/rbac"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api/presenters"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/auth"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/handlers"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/services"
)

// dns1123LabelRE validates K8s DNS label format (RFC 1123): lowercase alphanumeric
// and hyphens, start/end with alphanumeric, max 63 characters.
var dns1123LabelRE = regexp.MustCompile(`^[a-z0-9]([a-z0-9\-]{0,61}[a-z0-9])?$`)

var _ handlers.RestHandler = managedClusterHandler{}

type managedClusterHandler struct {
	managedCluster ManagedClusterService
	generic        services.GenericService
}

func NewManagedClusterHandler(managedCluster ManagedClusterService, generic services.GenericService) *managedClusterHandler {
	return &managedClusterHandler{
		managedCluster: managedCluster,
		generic:        generic,
	}
}

func (h managedClusterHandler) Register(w http.ResponseWriter, r *http.Request) {
	body, readErr := io.ReadAll(r.Body)
	if readErr != nil {
		handlers.HandleError(r.Context(), w, errors.MalformedRequest("unable to read request body: %s", readErr))
		return
	}

	var req openapi.ManagedClusterRegistrationRequest
	if err := json.Unmarshal(body, &req); err != nil {
		handlers.HandleError(r.Context(), w, errors.MalformedRequest("invalid request format: %s", err))
		return
	}

	if svcErr := handlers.ValidateNotEmpty(&req, "Name", "name")(); svcErr != nil {
		handlers.HandleError(r.Context(), w, svcErr)
		return
	}
	if !dns1123LabelRE.MatchString(req.Name) {
		handlers.HandleError(r.Context(), w, errors.MalformedRequest(
			"name %q is not a valid K8s DNS label: must be lowercase alphanumeric or hyphens, start and end with alphanumeric, max 63 characters",
			req.Name,
		))
		return
	}
	ctx := r.Context()
	token, tokenErr := auth.TokenFromContext(ctx)
	if tokenErr != nil || token == nil {
		handlers.HandleError(r.Context(), w, errors.Unauthenticated("missing identity"))
		return
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		handlers.HandleError(r.Context(), w, errors.Unauthenticated("invalid token claims"))
		return
	}
	oidcSubject, _ := claims["sub"].(string)
	if oidcSubject == "" {
		handlers.HandleError(r.Context(), w, errors.Unauthenticated("missing sub claim"))
		return
	}

	// A capability-less heartbeat is accepted (requireNonEmpty=false) for backward
	// compatibility with control planes that have not yet rolled to a
	// capability-aware image; Register treats the empty set as a no-op and
	// preserves any stored capabilities. See managed-cluster-registration.spec.md,
	// "Capability Deployment Convergence".
	capabilities, svcErr := NormalizeCapabilities(req.Capabilities, false)
	if svcErr != nil {
		handlers.HandleError(r.Context(), w, svcErr)
		return
	}
	cluster, created, svcErr := h.managedCluster.Register(ctx, req.Name, capabilities, oidcSubject)
	if svcErr != nil {
		handlers.HandleError(r.Context(), w, svcErr)
		return
	}

	resp := PresentRegistrationResponse(cluster.ID)
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Vary", "Authorization")
	w.WriteHeader(status)
	if payload, err := json.Marshal(resp); err == nil {
		_, _ = w.Write(payload)
	}
}

func (h managedClusterHandler) Create(w http.ResponseWriter, r *http.Request) {
	var managedCluster openapi.ManagedCluster
	cfg := &handlers.HandlerConfig{
		Body: &managedCluster,
		Validators: []handlers.Validate{
			handlers.ValidateEmpty(&managedCluster, "Id", "id"),
		},
		Action: func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()
			// The managed-cluster namespace is reserved and server-assigned: a generic
			// create may not declare or spoof an identity capability (PLAT-CLUS-14.02).
			if HasReservedCapability(managedCluster.Capabilities) {
				return nil, errors.Validation("capability namespace %q is reserved and server-assigned; it cannot be declared (PLAT-CLUS-14)", IdentityCapabilityNamespace)
			}
			managedClusterModel := ConvertManagedCluster(managedCluster)
			managedClusterModel, err := h.managedCluster.Create(ctx, managedClusterModel)
			if err != nil {
				return nil, err
			}
			return presentForCaller(ctx, managedClusterModel), nil
		},
		ErrorHandler: handlers.HandleError,
	}

	handlers.Handle(w, r, cfg, http.StatusCreated)
}

func (h managedClusterHandler) Patch(w http.ResponseWriter, r *http.Request) {
	var patch openapi.ManagedClusterPatchRequest

	cfg := &handlers.HandlerConfig{
		Body:       &patch,
		Validators: []handlers.Validate{},
		Action: func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()
			id := mux.Vars(r)["id"]
			found, err := h.managedCluster.Get(ctx, id)
			if err != nil {
				return nil, err
			}

			if patch.Name != nil {
				found.Name = *patch.Name
			}
			if patch.Provider != nil {
				found.Provider = *patch.Provider
			}
			if patch.Region != nil {
				found.Region = patch.Region
			}
			if patch.KubeconfigSecret != nil {
				found.KubeconfigSecret = *patch.KubeconfigSecret
			}
			if patch.Status != nil {
				found.Status = patch.Status
			}
			if patch.ApiServerUrl != nil {
				found.ApiServerUrl = patch.ApiServerUrl
			}

			managedClusterModel, err := h.managedCluster.Replace(ctx, found)
			if err != nil {
				return nil, err
			}
			return presentForCaller(ctx, managedClusterModel), nil
		},
		ErrorHandler: handlers.HandleError,
	}

	handlers.Handle(w, r, cfg, http.StatusOK)
}

func (h managedClusterHandler) List(w http.ResponseWriter, r *http.Request) {
	cfg := &handlers.HandlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()

			listArgs := services.NewListArguments(r.URL.Query())
			var managedClusters []ManagedCluster
			paging, err := h.generic.List(ctx, "id", listArgs, &managedClusters)
			if err != nil {
				return nil, err
			}
			kindStr := "ManagedClusterList"
			pageVal := int32(paging.Page)
			sizeVal := int32(paging.Size)
			totalVal := int32(paging.Total)
			managedClusterList := openapi.ManagedClusterList{
				Kind:  &kindStr,
				Page:  &pageVal,
				Size:  &sizeVal,
				Total: &totalVal,
				Items: []openapi.ManagedCluster{},
			}

			includeIdentity := callerSeesIdentityCapability(ctx)
			for _, managedCluster := range managedClusters {
				converted := presentWithIdentity(&managedCluster, includeIdentity)
				managedClusterList.Items = append(managedClusterList.Items, converted)
			}
			if listArgs.Fields != nil {
				filteredItems, err := presenters.SliceFilter(listArgs.Fields, managedClusterList.Items)
				if err != nil {
					return nil, err
				}
				return filteredItems, nil
			}
			return managedClusterList, nil
		},
	}

	handlers.HandleList(w, r, cfg)
}

func (h managedClusterHandler) Get(w http.ResponseWriter, r *http.Request) {
	cfg := &handlers.HandlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			id := mux.Vars(r)["id"]
			ctx := r.Context()
			managedCluster, err := h.managedCluster.Get(ctx, id)
			if err != nil {
				return nil, err
			}

			return presentForCaller(ctx, managedCluster), nil
		},
	}

	handlers.HandleGet(w, r, cfg)
}

// callerSeesIdentityCapability reports whether the caller may see the reserved,
// server-assigned managed-cluster/<name> identity capability in a response. Only a
// platform:admin caller sees it; a non-admin caller cannot enumerate the fleet's
// identity capabilities (PLAT-CLUS-14, defense in depth).
func callerSeesIdentityCapability(ctx context.Context) bool {
	return rbac.HasPlatformAdminRole(ctx, rbac.GetUserIDFromContext(ctx))
}

// presentForCaller presents a cluster with its identity capability included only
// for a platform:admin caller.
func presentForCaller(ctx context.Context, managedCluster *ManagedCluster) openapi.ManagedCluster {
	return presentWithIdentity(managedCluster, callerSeesIdentityCapability(ctx))
}

// presentWithIdentity presents a cluster, adding the derived managed-cluster/<name>
// identity capability when includeIdentity is true and otherwise stripping any
// managed-cluster/* label (defensive: the stored set never carries one, since it is
// rejected as input). The identity capability is derived server-side from the
// unique name and is never persisted (PLAT-CLUS-14).
func presentWithIdentity(managedCluster *ManagedCluster, includeIdentity bool) openapi.ManagedCluster {
	presented := PresentManagedCluster(managedCluster)
	filtered := make([]string, 0, len(presented.Capabilities)+1)
	for _, label := range presented.Capabilities {
		if strings.HasPrefix(label, IdentityCapabilityNamespace+"/") {
			continue
		}
		filtered = append(filtered, label)
	}
	if includeIdentity && managedCluster.Name != "" {
		filtered = append(filtered, IdentityCapability(managedCluster.Name))
	}
	sort.Strings(filtered)
	presented.Capabilities = filtered
	return presented
}

func (h managedClusterHandler) Delete(w http.ResponseWriter, r *http.Request) {
	cfg := &handlers.HandlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			id := mux.Vars(r)["id"]
			ctx := r.Context()
			err := h.managedCluster.Delete(ctx, id)
			if err != nil {
				return nil, err
			}
			return nil, nil
		},
	}
	handlers.HandleDelete(w, r, cfg, http.StatusNoContent)
}
