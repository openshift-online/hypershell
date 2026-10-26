package ciTrusts

import (
	"net/http"

	"github.com/gorilla/mux"

	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api/presenters"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/handlers"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/services"
)

type ciTrustHandler struct {
	service CITrustService
	generic services.GenericService
}

func NewCITrustHandler(service CITrustService, generic services.GenericService) *ciTrustHandler {
	return &ciTrustHandler{service: service, generic: generic}
}

func (h *ciTrustHandler) List(w http.ResponseWriter, r *http.Request) {
	cfg := &handlers.HandlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()
			listArgs := services.NewListArguments(r.URL.Query())
			var items []CITrust
			paging, err := h.generic.List(ctx, "id", listArgs, &items)
			if err != nil {
				return nil, err
			}
			kindStr := "CITrustList"
			pageVal := int32(paging.Page)
			sizeVal := int32(paging.Size)
			totalVal := int32(paging.Total)
			list := openapi.CITrustList{
				Kind:  &kindStr,
				Page:  &pageVal,
				Size:  &sizeVal,
				Total: &totalVal,
				Items: []openapi.CITrust{},
			}
			for _, item := range items {
				list.Items = append(list.Items, PresentCITrust(&item))
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

func (h *ciTrustHandler) Get(w http.ResponseWriter, r *http.Request) {
	cfg := &handlers.HandlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			id := mux.Vars(r)["id"]
			ctx := r.Context()
			item, err := h.service.Get(ctx, id)
			if err != nil {
				return nil, err
			}
			return PresentCITrust(item), nil
		},
	}
	handlers.HandleGet(w, r, cfg)
}

func (h *ciTrustHandler) Create(w http.ResponseWriter, r *http.Request) {
	var resource openapi.CITrust
	cfg := &handlers.HandlerConfig{
		Body: &resource,
		Action: func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()
			item := ConvertCITrust(resource)
			created, err := h.service.Create(ctx, item)
			if err != nil {
				return nil, err
			}
			return PresentCITrust(created), nil
		},
	}
	handlers.Handle(w, r, cfg, http.StatusCreated)
}

func (h *ciTrustHandler) Patch(w http.ResponseWriter, r *http.Request) {
	var patch CITrustPatchRequest
	cfg := &handlers.HandlerConfig{
		Body: &patch,
		Action: func() (interface{}, *errors.ServiceError) {
			id := mux.Vars(r)["id"]
			ctx := r.Context()
			found, err := h.service.Get(ctx, id)
			if err != nil {
				return nil, err
			}
			if patch.SubjectRef != nil {
				found.SubjectRef = patch.SubjectRef
			}
			if patch.SubjectEnvironment != nil {
				found.SubjectEnvironment = patch.SubjectEnvironment
			}
			if patch.SubjectRefProtected != nil {
				found.SubjectRefProtected = patch.SubjectRefProtected
			}
			if patch.Role != nil {
				found.Role = *patch.Role
			}
			updated, err := h.service.Replace(ctx, found)
			if err != nil {
				return nil, err
			}
			return PresentCITrust(updated), nil
		},
	}
	handlers.Handle(w, r, cfg, http.StatusOK)
}

func (h *ciTrustHandler) Delete(w http.ResponseWriter, r *http.Request) {
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
