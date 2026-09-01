package gatewayProfiles

import (
	"net/http"

	"github.com/gorilla/mux"

	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/rh-trex-ai/pkg/api/presenters"
	"github.com/openshift-online/rh-trex-ai/pkg/errors"
	"github.com/openshift-online/rh-trex-ai/pkg/handlers"
	"github.com/openshift-online/rh-trex-ai/pkg/services"
)

var _ handlers.RestHandler = gatewayProfileHandler{}

type gatewayProfileHandler struct {
	gatewayProfile GatewayProfileService
	generic        services.GenericService
}

func NewGatewayProfileHandler(gatewayProfile GatewayProfileService, generic services.GenericService) *gatewayProfileHandler {
	return &gatewayProfileHandler{
		gatewayProfile: gatewayProfile,
		generic:        generic,
	}
}

func (h gatewayProfileHandler) Create(w http.ResponseWriter, r *http.Request) {
	var gatewayProfile openapi.GatewayProfile
	cfg := &handlers.HandlerConfig{
		Body: &gatewayProfile,
		Validators: []handlers.Validate{
			handlers.ValidateEmpty(&gatewayProfile, "Id", "id"),
			handlers.ValidateNotEmpty(&gatewayProfile, "Name", "name"),
		},
		Action: func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()
			gatewayProfileModel := ConvertGatewayProfile(gatewayProfile)
			gatewayProfileModel, err := h.gatewayProfile.Create(ctx, gatewayProfileModel)
			if err != nil {
				return nil, err
			}
			return PresentGatewayProfile(gatewayProfileModel), nil
		},
		ErrorHandler: handlers.HandleError,
	}

	handlers.Handle(w, r, cfg, http.StatusCreated)
}

func (h gatewayProfileHandler) Patch(w http.ResponseWriter, r *http.Request) {
	var patch openapi.GatewayProfilePatchRequest

	cfg := &handlers.HandlerConfig{
		Body:       &patch,
		Validators: []handlers.Validate{},
		Action: func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()
			id := mux.Vars(r)["id"]
			found, err := h.gatewayProfile.Get(ctx, id)
			if err != nil {
				return nil, err
			}

			if patch.Name != nil {
				found.Name = *patch.Name
			}
			if patch.Description != nil {
				found.Description = patch.Description
			}
			if patch.CpuRequestTotal != nil {
				found.CpuRequestTotal = patch.CpuRequestTotal
			}
			if patch.CpuLimitTotal != nil {
				found.CpuLimitTotal = patch.CpuLimitTotal
			}
			if patch.MemoryRequestTotal != nil {
				found.MemoryRequestTotal = patch.MemoryRequestTotal
			}
			if patch.MemoryLimitTotal != nil {
				found.MemoryLimitTotal = patch.MemoryLimitTotal
			}
			if patch.EphemeralStorageTotal != nil {
				found.EphemeralStorageTotal = patch.EphemeralStorageTotal
			}
			if patch.PodCount != nil {
				found.PodCount = patch.PodCount
			}
			if patch.PvcCount != nil {
				found.PvcCount = patch.PvcCount
			}
			if patch.ContainerCpuRequestDefault != nil {
				found.ContainerCpuRequestDefault = patch.ContainerCpuRequestDefault
			}
			if patch.ContainerCpuLimitMax != nil {
				found.ContainerCpuLimitMax = patch.ContainerCpuLimitMax
			}
			if patch.ContainerMemoryRequestDefault != nil {
				found.ContainerMemoryRequestDefault = patch.ContainerMemoryRequestDefault
			}
			if patch.ContainerMemoryLimitMax != nil {
				found.ContainerMemoryLimitMax = patch.ContainerMemoryLimitMax
			}

			gatewayProfileModel, err := h.gatewayProfile.Replace(ctx, found)
			if err != nil {
				return nil, err
			}
			return PresentGatewayProfile(gatewayProfileModel), nil
		},
		ErrorHandler: handlers.HandleError,
	}

	handlers.Handle(w, r, cfg, http.StatusOK)
}

func (h gatewayProfileHandler) List(w http.ResponseWriter, r *http.Request) {
	cfg := &handlers.HandlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()

			listArgs := services.NewListArguments(r.URL.Query())
			var gatewayProfiles []GatewayProfile
			paging, err := h.generic.List(ctx, "id", listArgs, &gatewayProfiles)
			if err != nil {
				return nil, err
			}
			kindStr := "GatewayProfileList"
			pageVal := int32(paging.Page)
			sizeVal := int32(paging.Size)
			totalVal := int32(paging.Total)
			gatewayProfileList := openapi.GatewayProfileList{
				Kind:  &kindStr,
				Page:  &pageVal,
				Size:  &sizeVal,
				Total: &totalVal,
				Items: []openapi.GatewayProfile{},
			}

			for _, gatewayProfile := range gatewayProfiles {
				converted := PresentGatewayProfile(&gatewayProfile)
				gatewayProfileList.Items = append(gatewayProfileList.Items, converted)
			}
			if listArgs.Fields != nil {
				filteredItems, err := presenters.SliceFilter(listArgs.Fields, gatewayProfileList.Items)
				if err != nil {
					return nil, err
				}
				return filteredItems, nil
			}
			return gatewayProfileList, nil
		},
	}

	handlers.HandleList(w, r, cfg)
}

func (h gatewayProfileHandler) Get(w http.ResponseWriter, r *http.Request) {
	cfg := &handlers.HandlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			id := mux.Vars(r)["id"]
			ctx := r.Context()
			gatewayProfile, err := h.gatewayProfile.Get(ctx, id)
			if err != nil {
				return nil, err
			}

			return PresentGatewayProfile(gatewayProfile), nil
		},
	}

	handlers.HandleGet(w, r, cfg)
}

func (h gatewayProfileHandler) Delete(w http.ResponseWriter, r *http.Request) {
	cfg := &handlers.HandlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			id := mux.Vars(r)["id"]
			ctx := r.Context()
			err := h.gatewayProfile.Delete(ctx, id)
			if err != nil {
				return nil, err
			}
			return nil, nil
		},
	}
	handlers.HandleDelete(w, r, cfg, http.StatusNoContent)
}
