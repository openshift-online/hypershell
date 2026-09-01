package managedClusters

import (
	"net/http"

	"github.com/gorilla/mux"

	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/rh-trex-ai/pkg/api/presenters"
	"github.com/openshift-online/rh-trex-ai/pkg/errors"
	"github.com/openshift-online/rh-trex-ai/pkg/handlers"
	"github.com/openshift-online/rh-trex-ai/pkg/services"
)

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

func (h managedClusterHandler) Create(w http.ResponseWriter, r *http.Request) {
	var managedCluster openapi.ManagedCluster
	cfg := &handlers.HandlerConfig{
		Body: &managedCluster,
		Validators: []handlers.Validate{
			handlers.ValidateEmpty(&managedCluster, "Id", "id"),
		},
		Action: func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()
			managedClusterModel := ConvertManagedCluster(managedCluster)
			managedClusterModel, err := h.managedCluster.Create(ctx, managedClusterModel)
			if err != nil {
				return nil, err
			}
			return PresentManagedCluster(managedClusterModel), nil
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
			return PresentManagedCluster(managedClusterModel), nil
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

			for _, managedCluster := range managedClusters {
				converted := PresentManagedCluster(&managedCluster)
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

			return PresentManagedCluster(managedCluster), nil
		},
	}

	handlers.HandleGet(w, r, cfg)
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
