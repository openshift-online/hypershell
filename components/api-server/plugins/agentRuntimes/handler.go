package agentRuntimes

import (
	"net/http"

	"github.com/gorilla/mux"

	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api/presenters"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/handlers"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/services"
)

type agentRuntimeHandler struct {
	service    AgentRuntimeService
	generic    services.GenericService
	authorizer *manageAuthorizer
}

func NewAgentRuntimeHandler(service AgentRuntimeService, generic services.GenericService, authorizer *manageAuthorizer) *agentRuntimeHandler {
	return &agentRuntimeHandler{service: service, generic: generic, authorizer: authorizer}
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
	var resource openapi.AgentRuntime
	cfg := &handlers.HandlerConfig{
		Body: &resource,
		Action: func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()
			item := ConvertAgentRuntime(resource)
			// Tenancy: the caller must manage the gateway the runtime references
			// (gateway:owner/admin, or platform:admin) before the runtime is
			// created. The gateway id is required by the API contract; refusing an
			// empty id here also fails closed when it is not.
			if svcErr := h.authorizer.authorizeManageGateway(ctx, item.GatewayId); svcErr != nil {
				return nil, svcErr
			}
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
	var patch AgentRuntimePatchRequest
	cfg := &handlers.HandlerConfig{
		Body: &patch,
		Action: func() (interface{}, *errors.ServiceError) {
			id := mux.Vars(r)["id"]
			ctx := r.Context()
			found, err := h.service.Get(ctx, id)
			if err != nil {
				return nil, err
			}
			// Tenancy: the caller must manage the gateway the runtime references
			// before any mutation. gateway_id itself is not patchable, so the
			// check on the fetched runtime covers the whole update.
			if svcErr := h.authorizer.authorizeManageGateway(ctx, found.GatewayId); svcErr != nil {
				return nil, svcErr
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
			found, err := h.service.Get(ctx, id)
			if err != nil {
				return nil, err
			}
			// Tenancy: the caller must manage the gateway the runtime references
			// before the runtime is removed.
			if svcErr := h.authorizer.authorizeManageGateway(ctx, found.GatewayId); svcErr != nil {
				return nil, svcErr
			}
			if err := h.service.Delete(ctx, id); err != nil {
				return nil, err
			}
			return nil, nil
		},
	}
	handlers.HandleDelete(w, r, cfg, http.StatusNoContent)
}
