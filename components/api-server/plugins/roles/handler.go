package roles

import (
	"net/http"

	"github.com/gorilla/mux"

	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/rh-trex-ai/pkg/api/presenters"
	"github.com/openshift-online/rh-trex-ai/pkg/errors"
	"github.com/openshift-online/rh-trex-ai/pkg/handlers"
	"github.com/openshift-online/rh-trex-ai/pkg/services"
)

type roleHandler struct {
	role    RoleService
	generic services.GenericService
}

func NewRoleHandler(role RoleService, generic services.GenericService) *roleHandler {
	return &roleHandler{
		role:    role,
		generic: generic,
	}
}

func (h roleHandler) List(w http.ResponseWriter, r *http.Request) {
	cfg := &handlers.HandlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()

			listArgs := services.NewListArguments(r.URL.Query())
			var roles []Role
			paging, err := h.generic.List(ctx, "id", listArgs, &roles)
			if err != nil {
				return nil, err
			}
			kindStr := "RoleList"
			pageVal := int32(paging.Page)
			sizeVal := int32(paging.Size)
			totalVal := int32(paging.Total)
			roleList := openapi.RoleList{
				Kind:  &kindStr,
				Page:  &pageVal,
				Size:  &sizeVal,
				Total: &totalVal,
				Items: []openapi.Role{},
			}

			for _, role := range roles {
				converted := PresentRole(&role)
				roleList.Items = append(roleList.Items, converted)
			}
			if listArgs.Fields != nil {
				filteredItems, err := presenters.SliceFilter(listArgs.Fields, roleList.Items)
				if err != nil {
					return nil, err
				}
				return filteredItems, nil
			}
			return roleList, nil
		},
	}

	handlers.HandleList(w, r, cfg)
}

func (h roleHandler) Get(w http.ResponseWriter, r *http.Request) {
	cfg := &handlers.HandlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			id := mux.Vars(r)["id"]
			ctx := r.Context()
			role, err := h.role.Get(ctx, id)
			if err != nil {
				return nil, err
			}

			return PresentRole(role), nil
		},
	}

	handlers.HandleGet(w, r, cfg)
}
