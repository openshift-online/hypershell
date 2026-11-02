package sandboxTemplates

import (
	"net/http"

	"github.com/gorilla/mux"

	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api/presenters"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/handlers"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/services"
)

type sandboxTemplateHandler struct {
	service SandboxTemplateService
	generic services.GenericService
}

func NewSandboxTemplateHandler(service SandboxTemplateService, generic services.GenericService) *sandboxTemplateHandler {
	return &sandboxTemplateHandler{service: service, generic: generic}
}

func (h *sandboxTemplateHandler) List(w http.ResponseWriter, r *http.Request) {
	cfg := &handlers.HandlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()
			listArgs := services.NewListArguments(r.URL.Query())
			var items []SandboxTemplate
			paging, err := h.generic.List(ctx, "id", listArgs, &items)
			if err != nil {
				return nil, err
			}
			kindStr := "SandboxTemplateList"
			pageVal := int32(paging.Page)
			sizeVal := int32(paging.Size)
			totalVal := int32(paging.Total)
			list := openapi.SandboxTemplateList{
				Kind:  &kindStr,
				Page:  &pageVal,
				Size:  &sizeVal,
				Total: &totalVal,
				Items: []openapi.SandboxTemplate{},
			}
			for _, item := range items {
				list.Items = append(list.Items, PresentSandboxTemplate(&item))
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

func (h *sandboxTemplateHandler) Get(w http.ResponseWriter, r *http.Request) {
	cfg := &handlers.HandlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			id := mux.Vars(r)["id"]
			ctx := r.Context()
			item, err := h.service.Get(ctx, id)
			if err != nil {
				return nil, err
			}
			return PresentSandboxTemplate(item), nil
		},
	}
	handlers.HandleGet(w, r, cfg)
}

func (h *sandboxTemplateHandler) Create(w http.ResponseWriter, r *http.Request) {
	var resource openapi.SandboxTemplate
	cfg := &handlers.HandlerConfig{
		Body: &resource,
		Action: func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()
			item := ConvertSandboxTemplate(resource)
			created, err := h.service.Create(ctx, item)
			if err != nil {
				return nil, err
			}
			return PresentSandboxTemplate(created), nil
		},
	}
	handlers.Handle(w, r, cfg, http.StatusCreated)
}

func (h *sandboxTemplateHandler) Patch(w http.ResponseWriter, r *http.Request) {
	var patch SandboxTemplatePatchRequest
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
			if patch.Image != nil {
				found.Image = *patch.Image
			}
			if patch.NamePrefix != nil {
				found.NamePrefix = patch.NamePrefix
			}
			if patch.Policy != nil {
				found.Policy = patch.Policy
			}
			updated, err := h.service.Replace(ctx, found)
			if err != nil {
				return nil, err
			}
			return PresentSandboxTemplate(updated), nil
		},
	}
	handlers.Handle(w, r, cfg, http.StatusOK)
}

func (h *sandboxTemplateHandler) Delete(w http.ResponseWriter, r *http.Request) {
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
