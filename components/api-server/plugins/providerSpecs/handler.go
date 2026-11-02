package providerSpecs

import (
	"net/http"

	"github.com/gorilla/mux"

	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api/presenters"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/handlers"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/services"
)

type providerSpecHandler struct {
	service ProviderSpecService
	generic services.GenericService
}

func NewProviderSpecHandler(service ProviderSpecService, generic services.GenericService) *providerSpecHandler {
	return &providerSpecHandler{service: service, generic: generic}
}

func (h *providerSpecHandler) List(w http.ResponseWriter, r *http.Request) {
	cfg := &handlers.HandlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()
			listArgs := services.NewListArguments(r.URL.Query())
			var items []ProviderSpec
			paging, err := h.generic.List(ctx, "id", listArgs, &items)
			if err != nil {
				return nil, err
			}
			kindStr := "ProviderSpecList"
			pageVal := int32(paging.Page)
			sizeVal := int32(paging.Size)
			totalVal := int32(paging.Total)
			list := openapi.ProviderSpecList{
				Kind:  &kindStr,
				Page:  &pageVal,
				Size:  &sizeVal,
				Total: &totalVal,
				Items: []openapi.ProviderSpec{},
			}
			for _, item := range items {
				list.Items = append(list.Items, PresentProviderSpec(&item))
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

func (h *providerSpecHandler) Get(w http.ResponseWriter, r *http.Request) {
	cfg := &handlers.HandlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			id := mux.Vars(r)["id"]
			ctx := r.Context()
			item, err := h.service.Get(ctx, id)
			if err != nil {
				return nil, err
			}
			return PresentProviderSpec(item), nil
		},
	}
	handlers.HandleGet(w, r, cfg)
}

func (h *providerSpecHandler) Create(w http.ResponseWriter, r *http.Request) {
	var resource openapi.ProviderSpec
	cfg := &handlers.HandlerConfig{
		Body: &resource,
		Action: func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()
			item := ConvertProviderSpec(resource)
			created, err := h.service.Create(ctx, item)
			if err != nil {
				return nil, err
			}
			return PresentProviderSpec(created), nil
		},
	}
	handlers.Handle(w, r, cfg, http.StatusCreated)
}

func (h *providerSpecHandler) Patch(w http.ResponseWriter, r *http.Request) {
	var patch ProviderSpecPatchRequest
	cfg := &handlers.HandlerConfig{
		Body: &patch,
		Action: func() (interface{}, *errors.ServiceError) {
			id := mux.Vars(r)["id"]
			ctx := r.Context()
			found, err := h.service.Get(ctx, id)
			if err != nil {
				return nil, err
			}
			if patch.Name != nil {
				found.Name = *patch.Name
			}
			if patch.Capability != nil {
				found.Capability = patch.Capability
			}
			if patch.Profile != nil {
				found.Profile = patch.Profile
			}
			updated, err := h.service.Replace(ctx, found)
			if err != nil {
				return nil, err
			}
			return PresentProviderSpec(updated), nil
		},
	}
	handlers.Handle(w, r, cfg, http.StatusOK)
}

func (h *providerSpecHandler) Delete(w http.ResponseWriter, r *http.Request) {
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
