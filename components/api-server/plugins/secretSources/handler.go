package secretSources

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"

	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api/presenters"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/handlers"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/services"
)

type secretSourceHandler struct {
	service SecretSourceService
	generic services.GenericService
}

func NewSecretSourceHandler(service SecretSourceService, generic services.GenericService) *secretSourceHandler {
	return &secretSourceHandler{service: service, generic: generic}
}

func (h *secretSourceHandler) List(w http.ResponseWriter, r *http.Request) {
	cfg := &handlers.HandlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()
			listArgs := services.NewListArguments(r.URL.Query())
			var items []SecretSource
			paging, err := h.generic.List(ctx, "id", listArgs, &items)
			if err != nil {
				return nil, err
			}
			kindStr := "SecretSourceList"
			pageVal := int32(paging.Page)
			sizeVal := int32(paging.Size)
			totalVal := int32(paging.Total)
			list := openapi.SecretSourceList{
				Kind:  &kindStr,
				Page:  &pageVal,
				Size:  &sizeVal,
				Total: &totalVal,
				Items: []openapi.SecretSource{},
			}
			for _, item := range items {
				list.Items = append(list.Items, PresentSecretSource(&item))
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

func (h *secretSourceHandler) Get(w http.ResponseWriter, r *http.Request) {
	cfg := &handlers.HandlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			id := mux.Vars(r)["id"]
			ctx := r.Context()
			item, err := h.service.Get(ctx, id)
			if err != nil {
				return nil, err
			}
			return PresentSecretSource(item), nil
		},
	}
	handlers.HandleGet(w, r, cfg)
}

func (h *secretSourceHandler) Create(w http.ResponseWriter, r *http.Request) {
	cfg := &handlers.HandlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()
			var resource openapi.SecretSource
			if err := json.NewDecoder(r.Body).Decode(&resource); err != nil {
				return nil, errors.MalformedRequest("Unable to decode request body: %s", err)
			}
			item := ConvertSecretSource(resource)
			created, err := h.service.Create(ctx, item)
			if err != nil {
				return nil, err
			}
			return PresentSecretSource(created), nil
		},
	}
	handlers.Handle(w, r, cfg, http.StatusCreated)
}

func (h *secretSourceHandler) Patch(w http.ResponseWriter, r *http.Request) {
	cfg := &handlers.HandlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			id := mux.Vars(r)["id"]
			ctx := r.Context()
			found, err := h.service.Get(ctx, id)
			if err != nil {
				return nil, err
			}
			var patch SecretSourcePatchRequest
			if decodeErr := json.NewDecoder(r.Body).Decode(&patch); decodeErr != nil {
				return nil, errors.MalformedRequest("Unable to decode patch request: %s", decodeErr)
			}
			if patch.Name != nil {
				found.Name = *patch.Name
			}
			if patch.Purpose != nil {
				found.Purpose = *patch.Purpose
			}
			if patch.Backend != nil {
				found.Backend = *patch.Backend
			}
			if patch.Path != nil {
				found.Path = *patch.Path
			}
			if patch.KeyMappings != nil {
				found.KeyMappings = patch.KeyMappings
			}
			updated, err := h.service.Replace(ctx, found)
			if err != nil {
				return nil, err
			}
			return PresentSecretSource(updated), nil
		},
	}
	handlers.Handle(w, r, cfg, http.StatusOK)
}

func (h *secretSourceHandler) Delete(w http.ResponseWriter, r *http.Request) {
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
