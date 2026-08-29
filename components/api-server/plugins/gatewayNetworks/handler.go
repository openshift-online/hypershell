package gatewayNetworks

import (
	"net/http"

	"github.com/gorilla/mux"

	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/rh-trex-ai/pkg/api/presenters"
	"github.com/openshift-online/rh-trex-ai/pkg/errors"
	"github.com/openshift-online/rh-trex-ai/pkg/handlers"
	"github.com/openshift-online/rh-trex-ai/pkg/services"
)

var _ handlers.RestHandler = gatewayNetworkHandler{}

type gatewayNetworkHandler struct {
	gatewayNetwork GatewayNetworkService
	generic        services.GenericService
}

func NewGatewayNetworkHandler(gatewayNetwork GatewayNetworkService, generic services.GenericService) *gatewayNetworkHandler {
	return &gatewayNetworkHandler{
		gatewayNetwork: gatewayNetwork,
		generic:        generic,
	}
}

func (h gatewayNetworkHandler) Create(w http.ResponseWriter, r *http.Request) {
	var gatewayNetwork openapi.GatewayNetwork
	cfg := &handlers.HandlerConfig{
		Body: &gatewayNetwork,
		Validators: []handlers.Validate{
			handlers.ValidateEmpty(&gatewayNetwork, "Id", "id"),
		},
		Action: func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()
			gatewayNetworkModel := ConvertGatewayNetwork(gatewayNetwork)
			gatewayNetworkModel, err := h.gatewayNetwork.Create(ctx, gatewayNetworkModel)
			if err != nil {
				return nil, err
			}
			return PresentGatewayNetwork(gatewayNetworkModel), nil
		},
		ErrorHandler: handlers.HandleError,
	}

	handlers.Handle(w, r, cfg, http.StatusCreated)
}

func (h gatewayNetworkHandler) Patch(w http.ResponseWriter, r *http.Request) {
	var patch openapi.GatewayNetworkPatchRequest

	cfg := &handlers.HandlerConfig{
		Body:       &patch,
		Validators: []handlers.Validate{},
		Action: func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()
			id := mux.Vars(r)["id"]
			found, err := h.gatewayNetwork.Get(ctx, id)
			if err != nil {
				return nil, err
			}

			if patch.Name != nil {
				found.Name = *patch.Name
			}
			if patch.Topology != nil {
				found.Topology = patch.Topology
			}
			if patch.TunnelMode != nil {
				found.TunnelMode = patch.TunnelMode
			}
			if patch.HubGatewayId != nil {
				found.HubGatewayId = patch.HubGatewayId
			}
			if patch.Status != nil {
				found.Status = patch.Status
			}

			gatewayNetworkModel, err := h.gatewayNetwork.Replace(ctx, found)
			if err != nil {
				return nil, err
			}
			return PresentGatewayNetwork(gatewayNetworkModel), nil
		},
		ErrorHandler: handlers.HandleError,
	}

	handlers.Handle(w, r, cfg, http.StatusOK)
}

func (h gatewayNetworkHandler) List(w http.ResponseWriter, r *http.Request) {
	cfg := &handlers.HandlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()

			listArgs := services.NewListArguments(r.URL.Query())
			var gatewayNetworks []GatewayNetwork
			paging, err := h.generic.List(ctx, "id", listArgs, &gatewayNetworks)
			if err != nil {
				return nil, err
			}
			kindStr := "GatewayNetworkList"
			pageVal := int32(paging.Page)
			sizeVal := int32(paging.Size)
			totalVal := int32(paging.Total)
			gatewayNetworkList := openapi.GatewayNetworkList{
				Kind:  &kindStr,
				Page:  &pageVal,
				Size:  &sizeVal,
				Total: &totalVal,
				Items: []openapi.GatewayNetwork{},
			}

			for _, gatewayNetwork := range gatewayNetworks {
				converted := PresentGatewayNetwork(&gatewayNetwork)
				gatewayNetworkList.Items = append(gatewayNetworkList.Items, converted)
			}
			if listArgs.Fields != nil {
				filteredItems, err := presenters.SliceFilter(listArgs.Fields, gatewayNetworkList.Items)
				if err != nil {
					return nil, err
				}
				return filteredItems, nil
			}
			return gatewayNetworkList, nil
		},
	}

	handlers.HandleList(w, r, cfg)
}

func (h gatewayNetworkHandler) Get(w http.ResponseWriter, r *http.Request) {
	cfg := &handlers.HandlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			id := mux.Vars(r)["id"]
			ctx := r.Context()
			gatewayNetwork, err := h.gatewayNetwork.Get(ctx, id)
			if err != nil {
				return nil, err
			}

			return PresentGatewayNetwork(gatewayNetwork), nil
		},
	}

	handlers.HandleGet(w, r, cfg)
}

func (h gatewayNetworkHandler) Delete(w http.ResponseWriter, r *http.Request) {
	cfg := &handlers.HandlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			id := mux.Vars(r)["id"]
			ctx := r.Context()
			err := h.gatewayNetwork.Delete(ctx, id)
			if err != nil {
				return nil, err
			}
			return nil, nil
		},
	}
	handlers.HandleDelete(w, r, cfg, http.StatusNoContent)
}
