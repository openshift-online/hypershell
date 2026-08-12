package gateways

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"

	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/rh-trex-ai/pkg/api/presenters"
	"github.com/openshift-online/rh-trex-ai/pkg/errors"
	"github.com/openshift-online/rh-trex-ai/pkg/handlers"
	"github.com/openshift-online/rh-trex-ai/pkg/services"
)

var _ handlers.RestHandler = gatewayHandler{}

type gatewayHandler struct {
	gateway GatewayService
	generic services.GenericService
}

func NewGatewayHandler(gateway GatewayService, generic services.GenericService) *gatewayHandler {
	return &gatewayHandler{
		gateway: gateway,
		generic: generic,
	}
}

func (h gatewayHandler) Create(w http.ResponseWriter, r *http.Request) {
	var gateway openapi.GatewayCreateRequest
	cfg := &handlers.HandlerConfig{
		Body:       &gateway,
		Validators: []handlers.Validate{},
		Action: func() (interface{}, *errors.ServiceError) {
			// DEBUG: intentional breakage to validate e2e CI pipeline (remove after validation)
			return nil, errors.GeneralError("DEBUG: gateway creation intentionally disabled for e2e pipeline validation")
		},
		ErrorHandler: handlers.HandleError,
	}

	handlers.Handle(w, r, cfg, http.StatusCreated)
}

func (h gatewayHandler) Patch(w http.ResponseWriter, r *http.Request) {
	var patch openapi.GatewayPatchRequest

	cfg := &handlers.HandlerConfig{
		Body:       &patch,
		Validators: []handlers.Validate{},
		Action: func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()
			id := mux.Vars(r)["id"]
			found, err := h.gateway.Get(ctx, id)
			if err != nil {
				return nil, err
			}

			if patch.Name != nil {
				found.Name = *patch.Name
			}
			if patch.FleetId != nil {
				found.FleetId = *patch.FleetId
			}
			if patch.ClusterId != nil {
				found.ClusterId = *patch.ClusterId
			}
			if patch.ReleaseId != nil {
				found.ReleaseId = *patch.ReleaseId
			}
			if patch.DatabaseId != nil {
				found.DatabaseId = *patch.DatabaseId
			}
			if patch.ExternalDns != nil {
				found.ExternalDns = patch.ExternalDns
			}
			if patch.TlsMode != nil {
				found.TlsMode = patch.TlsMode
			}
			if patch.ServiceType != nil {
				found.ServiceType = patch.ServiceType
			}
			if patch.Status != nil {
				found.Status = patch.Status
			}
			if patch.Phase != nil {
				found.Phase = patch.Phase
			}
			if patch.Image != nil {
				found.Image = patch.Image
			}
			if len(patch.ServerDnsNames) > 0 {
				data, _ := json.Marshal(patch.ServerDnsNames)
				s := string(data)
				found.ServerDnsNames = &s
			}
			if patch.RouteAddress != nil {
				found.RouteAddress = patch.RouteAddress
			}
			if patch.Oidc != nil {
				found.Oidc = patch.Oidc
			}
			if patch.Route != nil {
				found.Route = patch.Route
			}
			if patch.DatabaseConfig != nil {
				found.DatabaseConfig = patch.DatabaseConfig
			}

			gatewayModel, err := h.gateway.Replace(ctx, found)
			if err != nil {
				return nil, err
			}
			return PresentGateway(gatewayModel), nil
		},
		ErrorHandler: handlers.HandleError,
	}

	handlers.Handle(w, r, cfg, http.StatusOK)
}

func (h gatewayHandler) List(w http.ResponseWriter, r *http.Request) {
	cfg := &handlers.HandlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()

			listArgs := services.NewListArguments(r.URL.Query())
			var gateways []Gateway
			paging, err := h.generic.List(ctx, "id", listArgs, &gateways)
			if err != nil {
				return nil, err
			}
			kindStr := "GatewayList"
			pageVal := int32(paging.Page)
			sizeVal := int32(paging.Size)
			totalVal := int32(paging.Total)
			gatewayList := openapi.GatewayList{
				Kind:  &kindStr,
				Page:  &pageVal,
				Size:  &sizeVal,
				Total: &totalVal,
				Items: []openapi.Gateway{},
			}

			for _, gateway := range gateways {
				converted := PresentGateway(&gateway)
				gatewayList.Items = append(gatewayList.Items, converted)
			}
			if listArgs.Fields != nil {
				filteredItems, err := presenters.SliceFilter(listArgs.Fields, gatewayList.Items)
				if err != nil {
					return nil, err
				}
				return filteredItems, nil
			}
			return gatewayList, nil
		},
	}

	handlers.HandleList(w, r, cfg)
}

func (h gatewayHandler) Get(w http.ResponseWriter, r *http.Request) {
	cfg := &handlers.HandlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			id := mux.Vars(r)["id"]
			ctx := r.Context()
			gateway, err := h.gateway.Get(ctx, id)
			if err != nil {
				return nil, err
			}

			return PresentGateway(gateway), nil
		},
	}

	handlers.HandleGet(w, r, cfg)
}

func (h gatewayHandler) Delete(w http.ResponseWriter, r *http.Request) {
	cfg := &handlers.HandlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			id := mux.Vars(r)["id"]
			ctx := r.Context()
			err := h.gateway.Delete(ctx, id)
			if err != nil {
				return nil, err
			}
			return nil, nil
		},
	}
	handlers.HandleDelete(w, r, cfg, http.StatusNoContent)
}
