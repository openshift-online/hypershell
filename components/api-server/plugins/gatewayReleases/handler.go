package gatewayReleases

import (
	"net/http"

	"github.com/gorilla/mux"

	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/rh-trex-ai/pkg/api/presenters"
	"github.com/openshift-online/rh-trex-ai/pkg/errors"
	"github.com/openshift-online/rh-trex-ai/pkg/handlers"
	"github.com/openshift-online/rh-trex-ai/pkg/services"
)

var _ handlers.RestHandler = gatewayReleaseHandler{}

type gatewayReleaseHandler struct {
	gatewayRelease GatewayReleaseService
	generic        services.GenericService
}

func NewGatewayReleaseHandler(gatewayRelease GatewayReleaseService, generic services.GenericService) *gatewayReleaseHandler {
	return &gatewayReleaseHandler{
		gatewayRelease: gatewayRelease,
		generic:        generic,
	}
}

func (h gatewayReleaseHandler) Create(w http.ResponseWriter, r *http.Request) {
	var gatewayRelease openapi.GatewayRelease
	cfg := &handlers.HandlerConfig{
		Body: &gatewayRelease,
		Validators: []handlers.Validate{
			handlers.ValidateEmpty(&gatewayRelease, "Id", "id"),
		},
		Action: func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()
			gatewayReleaseModel := ConvertGatewayRelease(gatewayRelease)
			gatewayReleaseModel, err := h.gatewayRelease.Create(ctx, gatewayReleaseModel)
			if err != nil {
				return nil, err
			}
			return PresentGatewayRelease(gatewayReleaseModel), nil
		},
		ErrorHandler: handlers.HandleError,
	}

	handlers.Handle(w, r, cfg, http.StatusCreated)
}

func (h gatewayReleaseHandler) Patch(w http.ResponseWriter, r *http.Request) {
	var patch openapi.GatewayReleasePatchRequest

	cfg := &handlers.HandlerConfig{
		Body:       &patch,
		Validators: []handlers.Validate{},
		Action: func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()
			id := mux.Vars(r)["id"]
			found, err := h.gatewayRelease.Get(ctx, id)
			if err != nil {
				return nil, err
			}

			if patch.Name != nil {
				found.Name = *patch.Name
			}
			if patch.Image != nil {
				found.Image = *patch.Image
			}
			if patch.RolloutStrategy != nil {
				found.RolloutStrategy = patch.RolloutStrategy
			}
			if patch.CanaryPercent != nil {
				canaryPercentVal := int(*patch.CanaryPercent)
				found.CanaryPercent = &canaryPercentVal
			}
			if patch.CanaryDuration != nil {
				found.CanaryDuration = patch.CanaryDuration
			}
			if patch.Status != nil {
				found.Status = patch.Status
			}

			gatewayReleaseModel, err := h.gatewayRelease.Replace(ctx, found)
			if err != nil {
				return nil, err
			}
			return PresentGatewayRelease(gatewayReleaseModel), nil
		},
		ErrorHandler: handlers.HandleError,
	}

	handlers.Handle(w, r, cfg, http.StatusOK)
}

func (h gatewayReleaseHandler) List(w http.ResponseWriter, r *http.Request) {
	cfg := &handlers.HandlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()

			listArgs := services.NewListArguments(r.URL.Query())
			var gatewayReleases []GatewayRelease
			paging, err := h.generic.List(ctx, "id", listArgs, &gatewayReleases)
			if err != nil {
				return nil, err
			}
			kindStr := "GatewayReleaseList"
			pageVal := int32(paging.Page)
			sizeVal := int32(paging.Size)
			totalVal := int32(paging.Total)
			gatewayReleaseList := openapi.GatewayReleaseList{
				Kind:  &kindStr,
				Page:  &pageVal,
				Size:  &sizeVal,
				Total: &totalVal,
				Items: []openapi.GatewayRelease{},
			}

			for _, gatewayRelease := range gatewayReleases {
				converted := PresentGatewayRelease(&gatewayRelease)
				gatewayReleaseList.Items = append(gatewayReleaseList.Items, converted)
			}
			if listArgs.Fields != nil {
				filteredItems, err := presenters.SliceFilter(listArgs.Fields, gatewayReleaseList.Items)
				if err != nil {
					return nil, err
				}
				return filteredItems, nil
			}
			return gatewayReleaseList, nil
		},
	}

	handlers.HandleList(w, r, cfg)
}

func (h gatewayReleaseHandler) Get(w http.ResponseWriter, r *http.Request) {
	cfg := &handlers.HandlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			id := mux.Vars(r)["id"]
			ctx := r.Context()
			gatewayRelease, err := h.gatewayRelease.Get(ctx, id)
			if err != nil {
				return nil, err
			}

			return PresentGatewayRelease(gatewayRelease), nil
		},
	}

	handlers.HandleGet(w, r, cfg)
}

func (h gatewayReleaseHandler) Delete(w http.ResponseWriter, r *http.Request) {
	cfg := &handlers.HandlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			id := mux.Vars(r)["id"]
			ctx := r.Context()
			err := h.gatewayRelease.Delete(ctx, id)
			if err != nil {
				return nil, err
			}
			return nil, nil
		},
	}
	handlers.HandleDelete(w, r, cfg, http.StatusNoContent)
}
