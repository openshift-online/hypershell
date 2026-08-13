package gateways

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gorilla/mux"

	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/hypershell/components/api-server/pkg/rbac"
	"github.com/openshift-online/rh-trex-ai/pkg/api/presenters"
	"github.com/openshift-online/rh-trex-ai/pkg/errors"
	"github.com/openshift-online/rh-trex-ai/pkg/handlers"
	"github.com/openshift-online/rh-trex-ai/pkg/services"
)

type OwnerBindingCreator interface {
	CreateOwnerBinding(ctx context.Context, userID string, gatewayID string) error
}

type GatewayVisibilityFilter interface {
	AccessibleGatewayIDs(ctx context.Context, userID string) ([]string, error)
}

var _ handlers.RestHandler = gatewayHandler{}

type gatewayHandler struct {
	gateway          GatewayService
	generic          services.GenericService
	ownerBinding     OwnerBindingCreator
	visibilityFilter GatewayVisibilityFilter
}

func NewGatewayHandler(gateway GatewayService, generic services.GenericService, ownerBinding OwnerBindingCreator, visibilityFilter GatewayVisibilityFilter) *gatewayHandler {
	return &gatewayHandler{
		gateway:          gateway,
		generic:          generic,
		ownerBinding:     ownerBinding,
		visibilityFilter: visibilityFilter,
	}
}

func (h gatewayHandler) Create(w http.ResponseWriter, r *http.Request) {
	var gateway openapi.GatewayCreateRequest
	cfg := &handlers.HandlerConfig{
		Body:       &gateway,
		Validators: []handlers.Validate{},
		Action: func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()
			gatewayModel := ConvertGateway(gateway)
			gatewayModel, err := h.gateway.Create(ctx, gatewayModel)
			if err != nil {
				return nil, err
			}

			userID := rbac.GetUserIDFromContext(ctx)
			if userID != "" && h.ownerBinding != nil {
				if bindErr := h.ownerBinding.CreateOwnerBinding(ctx, userID, gatewayModel.ID); bindErr != nil {
					return nil, errors.GeneralError("failed to create owner binding: %s", bindErr)
				}
			}

			return PresentGateway(gatewayModel), nil
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

			userID := rbac.GetUserIDFromContext(ctx)
			if userID != "" && h.visibilityFilter != nil {
				accessibleIDs, filterErr := h.visibilityFilter.AccessibleGatewayIDs(ctx, userID)
				if filterErr != nil {
					return nil, errors.GeneralError("failed to check gateway access: %s", filterErr)
				}
				if len(accessibleIDs) == 0 {
					kindStr := "GatewayList"
					pageVal := int32(listArgs.Page)
					sizeVal := int32(0)
					totalVal := int32(0)
					return openapi.GatewayList{
						Kind:  &kindStr,
						Page:  &pageVal,
						Size:  &sizeVal,
						Total: &totalVal,
						Items: []openapi.Gateway{},
					}, nil
				}
				idFilter := visibilitySearchFilter(accessibleIDs)
				if listArgs.Search != "" {
					listArgs.Search = "(" + listArgs.Search + ") and " + idFilter
				} else {
					listArgs.Search = idFilter
				}
			}

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

func visibilitySearchFilter(ids []string) string {
	quoted := make([]string, len(ids))
	for i, id := range ids {
		quoted[i] = "'" + id + "'"
	}
	return "id in (" + strings.Join(quoted, ",") + ")"
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
