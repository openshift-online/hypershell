package secretSources

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/gorilla/mux"

	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/hypershell/components/api-server/pkg/secretstore"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api/presenters"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/handlers"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/services"
)

type secretSourceHandler struct {
	service SecretSourceService
	generic services.GenericService
	store   secretstore.Store
}

func NewSecretSourceHandler(service SecretSourceService, generic services.GenericService, store secretstore.Store) *secretSourceHandler {
	return &secretSourceHandler{service: service, generic: generic, store: store}
}

// deterministicSecretLocation derives the secret-manager location a submitted
// value is written to, purely from the submission identity (agent_runtime_id and
// name). Being deterministic makes the store write idempotent, so a submission
// retried after a database failure overwrites the same entry instead of forking
// the secret (data-model.spec.md "Secret Submission and Storage"). The name is
// path-escaped so it cannot traverse outside the resource's prefix.
func deterministicSecretLocation(agentRuntimeID, name string) string {
	return fmt.Sprintf("hypershell/secret-sources/%s/%s", url.PathEscape(agentRuntimeID), url.PathEscape(name))
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
	var resource openapi.SecretSource
	cfg := &handlers.HandlerConfig{
		Body: &resource,
		Action: func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()
			item := ConvertSecretSource(resource)
			// Secret submission (one-way): when a value is supplied, write it to
			// the configured secret manager FIRST, at a deterministic location, and
			// only then persist the SecretSource reference. ConvertSecretSource
			// never copies the value onto the model, so the raw value never reaches
			// the database and the presenter never returns it.
			if resource.Value != nil && *resource.Value != "" {
				if item.AgentRuntimeId == "" || item.Name == "" {
					return nil, errors.Validation("name and agent_runtime_id are required to submit a secret value")
				}
				location := deterministicSecretLocation(item.AgentRuntimeId, item.Name)
				if err := h.store.Put(ctx, location, *resource.Value); err != nil {
					return nil, errors.GeneralError("Unable to store submitted secret value: %s", err)
				}
				// The reference points at where the value was just written,
				// overriding any client-supplied path.
				item.Path = location
			}
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
	var patch SecretSourcePatchRequest
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
