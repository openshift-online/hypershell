package inferenceRoutes

import (
	"net/http"

	"github.com/gorilla/mux"

	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api/presenters"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/handlers"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/services"
)

type inferenceRouteHandler struct {
	service InferenceRouteService
	generic services.GenericService
}

func NewInferenceRouteHandler(service InferenceRouteService, generic services.GenericService) *inferenceRouteHandler {
	return &inferenceRouteHandler{service: service, generic: generic}
}

func (h *inferenceRouteHandler) List(w http.ResponseWriter, r *http.Request) {
	cfg := &handlers.HandlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()
			listArgs := services.NewListArguments(r.URL.Query())
			var items []InferenceRoute
			paging, err := h.generic.List(ctx, "id", listArgs, &items)
			if err != nil {
				return nil, err
			}
			kindStr := "InferenceRouteList"
			pageVal := int32(paging.Page)
			sizeVal := int32(paging.Size)
			totalVal := int32(paging.Total)
			list := openapi.InferenceRouteList{
				Kind:  &kindStr,
				Page:  &pageVal,
				Size:  &sizeVal,
				Total: &totalVal,
				Items: []openapi.InferenceRoute{},
			}
			for _, item := range items {
				list.Items = append(list.Items, PresentInferenceRoute(&item))
			}
			if listArgs.Fields != nil {
				filteredItems, err := presenters.SliceFilter(listArgs.Fields, list.Items)
				if err != nil {
					return nil, err
				}
				return filteredItems, nil
			}
			return list, nil
		},
	}
	handlers.HandleList(w, r, cfg)
}

func (h *inferenceRouteHandler) Get(w http.ResponseWriter, r *http.Request) {
	cfg := &handlers.HandlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			id := mux.Vars(r)["id"]
			ctx := r.Context()
			item, err := h.service.Get(ctx, id)
			if err != nil {
				return nil, err
			}
			return PresentInferenceRoute(item), nil
		},
	}
	handlers.HandleGet(w, r, cfg)
}

func (h *inferenceRouteHandler) Create(w http.ResponseWriter, r *http.Request) {
	var resource openapi.InferenceRoute
	cfg := &handlers.HandlerConfig{
		Body: &resource,
		Action: func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()
			item := ConvertInferenceRoute(resource)
			created, err := h.service.Create(ctx, item)
			if err != nil {
				return nil, err
			}
			return PresentInferenceRoute(created), nil
		},
	}
	handlers.Handle(w, r, cfg, http.StatusCreated)
}

func (h *inferenceRouteHandler) Delete(w http.ResponseWriter, r *http.Request) {
	cfg := &handlers.HandlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			id := mux.Vars(r)["id"]
			ctx := r.Context()
			if _, err := h.service.Get(ctx, id); err != nil {
				return nil, err
			}
			if err := h.service.Delete(ctx, id); err != nil {
				return nil, err
			}
			return nil, nil
		},
	}
	handlers.HandleDelete(w, r, cfg, http.StatusNoContent)
}
