package ciProviders

import (
	"net/http"

	"github.com/gorilla/mux"

	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api/presenters"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/handlers"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/services"
)

type ciProviderHandler struct {
	service CIProviderService
	generic services.GenericService
}

func NewCIProviderHandler(service CIProviderService, generic services.GenericService) *ciProviderHandler {
	return &ciProviderHandler{service: service, generic: generic}
}

func (h *ciProviderHandler) List(w http.ResponseWriter, r *http.Request) {
	cfg := &handlers.HandlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()
			listArgs := services.NewListArguments(r.URL.Query())
			var items []CIProvider
			paging, err := h.generic.List(ctx, "id", listArgs, &items)
			if err != nil {
				return nil, err
			}
			kindStr := "CIProviderList"
			pageVal := int32(paging.Page)
			sizeVal := int32(paging.Size)
			totalVal := int32(paging.Total)
			list := openapi.CIProviderList{
				Kind:  &kindStr,
				Page:  &pageVal,
				Size:  &sizeVal,
				Total: &totalVal,
				Items: []openapi.CIProvider{},
			}
			for _, item := range items {
				list.Items = append(list.Items, PresentCIProvider(&item))
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

func (h *ciProviderHandler) Get(w http.ResponseWriter, r *http.Request) {
	cfg := &handlers.HandlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			id := mux.Vars(r)["id"]
			ctx := r.Context()
			item, err := h.service.Get(ctx, id)
			if err != nil {
				return nil, err
			}
			return PresentCIProvider(item), nil
		},
	}
	handlers.HandleGet(w, r, cfg)
}

func (h *ciProviderHandler) Create(w http.ResponseWriter, r *http.Request) {
	var resource openapi.CIProvider
	cfg := &handlers.HandlerConfig{
		Body: &resource,
		Action: func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()
			item := ConvertCIProvider(resource)
			created, err := h.service.Create(ctx, item)
			if err != nil {
				return nil, err
			}
			return PresentCIProvider(created), nil
		},
	}
	handlers.Handle(w, r, cfg, http.StatusCreated)
}

func (h *ciProviderHandler) Patch(w http.ResponseWriter, r *http.Request) {
	var patch CIProviderPatchRequest
	cfg := &handlers.HandlerConfig{
		Body: &patch,
		Action: func() (interface{}, *errors.ServiceError) {
			id := mux.Vars(r)["id"]
			ctx := r.Context()
			found, err := h.service.Get(ctx, id)
			if err != nil {
				return nil, err
			}
			if patch.JwksUrl != nil {
				found.JwksUrl = *patch.JwksUrl
			}
			updated, err := h.service.Replace(ctx, found)
			if err != nil {
				return nil, err
			}
			return PresentCIProvider(updated), nil
		},
	}
	handlers.Handle(w, r, cfg, http.StatusOK)
}

func (h *ciProviderHandler) Delete(w http.ResponseWriter, r *http.Request) {
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
