package agentRuntimes

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

type agentRuntimeHandler struct {
	service AgentRuntimeService
	generic services.GenericService
}

func NewAgentRuntimeHandler(service AgentRuntimeService, generic services.GenericService) *agentRuntimeHandler {
	return &agentRuntimeHandler{service: service, generic: generic}
}

func (h *agentRuntimeHandler) List(w http.ResponseWriter, r *http.Request) {
	cfg := &handlers.HandlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()
			listArgs := services.NewListArguments(r.URL.Query())
			var items []AgentRuntime
			paging, err := h.generic.List(ctx, "id", listArgs, &items)
			if err != nil {
				return nil, err
			}
			kindStr := "AgentRuntimeList"
			pageVal := int32(paging.Page)
			sizeVal := int32(paging.Size)
			totalVal := int32(paging.Total)
			list := openapi.AgentRuntimeList{
				Kind:  &kindStr,
				Page:  &pageVal,
				Size:  &sizeVal,
				Total: &totalVal,
				Items: []openapi.AgentRuntime{},
			}
			for _, item := range items {
				list.Items = append(list.Items, PresentAgentRuntime(&item))
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

func (h *agentRuntimeHandler) Get(w http.ResponseWriter, r *http.Request) {
	cfg := &handlers.HandlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			id := mux.Vars(r)["id"]
			ctx := r.Context()
			item, err := h.service.Get(ctx, id)
			if err != nil {
				return nil, err
			}
			return PresentAgentRuntime(item), nil
		},
	}
	handlers.HandleGet(w, r, cfg)
}

func (h *agentRuntimeHandler) Create(w http.ResponseWriter, r *http.Request) {
	cfg := &handlers.HandlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()
			var resource openapi.AgentRuntime
			if err := json.NewDecoder(r.Body).Decode(&resource); err != nil {
				return nil, errors.MalformedRequest("Unable to decode request body: %s", err)
			}
			item := ConvertAgentRuntime(resource)
			created, err := h.service.Create(ctx, item)
			if err != nil {
				return nil, err
			}
			return PresentAgentRuntime(created), nil
		},
	}
	handlers.Handle(w, r, cfg, http.StatusCreated)
}

func (h *agentRuntimeHandler) Patch(w http.ResponseWriter, r *http.Request) {
	cfg := &handlers.HandlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			id := mux.Vars(r)["id"]
			ctx := r.Context()
			found, err := h.service.Get(ctx, id)
			if err != nil {
				return nil, err
			}
			var patch AgentRuntimePatchRequest
			if decodeErr := json.NewDecoder(r.Body).Decode(&patch); decodeErr != nil {
				return nil, errors.MalformedRequest("Unable to decode patch request: %s", decodeErr)
			}
			if patch.Name != nil {
				found.Name = *patch.Name
			}
			if patch.SandboxTemplateId != nil {
				found.SandboxTemplateId = *patch.SandboxTemplateId
			}
			if patch.RepositoryId != nil {
				found.RepositoryId = patch.RepositoryId
			}
			if patch.Selector != nil {
				found.Selector = patch.Selector
			}
			if patch.Description != nil {
				found.Description = patch.Description
			}
			if patch.Cron != nil {
				found.Cron = patch.Cron
			}
			if patch.CoordinatorImage != nil {
				found.CoordinatorImage = patch.CoordinatorImage
			}
			if patch.ConcurrencyPolicy != nil {
				found.ConcurrencyPolicy = patch.ConcurrencyPolicy
			}
			if patch.LoginRefreshSeconds != nil {
				found.LoginRefreshSeconds = patch.LoginRefreshSeconds
			}
			if patch.Parameters != nil {
				found.Parameters = patch.Parameters
			}
			updated, err := h.service.Replace(ctx, found)
			if err != nil {
				return nil, err
			}
			return PresentAgentRuntime(updated), nil
		},
	}
	handlers.Handle(w, r, cfg, http.StatusOK)
}

func (h *agentRuntimeHandler) Delete(w http.ResponseWriter, r *http.Request) {
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
