package roleBindings

import (
	"net/http"

	"github.com/gorilla/mux"

	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/rh-trex-ai/pkg/api/presenters"
	"github.com/openshift-online/rh-trex-ai/pkg/errors"
	"github.com/openshift-online/rh-trex-ai/pkg/handlers"
	"github.com/openshift-online/rh-trex-ai/pkg/services"
)

type roleBindingHandler struct {
	roleBinding RoleBindingService
	generic     services.GenericService
}

func NewRoleBindingHandler(roleBinding RoleBindingService, generic services.GenericService) *roleBindingHandler {
	return &roleBindingHandler{
		roleBinding: roleBinding,
		generic:     generic,
	}
}

func (h roleBindingHandler) Create(w http.ResponseWriter, r *http.Request) {
	var rb openapi.RoleBinding
	cfg := &handlers.HandlerConfig{
		Body: &rb,
		Validators: []handlers.Validate{
			handlers.ValidateEmpty(&rb, "Id", "id"),
		},
		Action: func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()
			rbModel := ConvertRoleBinding(rb)
			rbModel, err := h.roleBinding.Create(ctx, rbModel)
			if err != nil {
				return nil, err
			}
			return PresentRoleBinding(rbModel), nil
		},
		ErrorHandler: handlers.HandleError,
	}

	handlers.Handle(w, r, cfg, http.StatusCreated)
}

func (h roleBindingHandler) List(w http.ResponseWriter, r *http.Request) {
	cfg := &handlers.HandlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()

			listArgs := services.NewListArguments(r.URL.Query())
			var bindings []RoleBinding
			paging, err := h.generic.List(ctx, "id", listArgs, &bindings)
			if err != nil {
				return nil, err
			}
			kindStr := "RoleBindingList"
			pageVal := int32(paging.Page)
			sizeVal := int32(paging.Size)
			totalVal := int32(paging.Total)
			rbList := openapi.RoleBindingList{
				Kind:  &kindStr,
				Page:  &pageVal,
				Size:  &sizeVal,
				Total: &totalVal,
				Items: []openapi.RoleBinding{},
			}

			for _, binding := range bindings {
				converted := PresentRoleBinding(&binding)
				rbList.Items = append(rbList.Items, converted)
			}
			if listArgs.Fields != nil {
				filteredItems, err := presenters.SliceFilter(listArgs.Fields, rbList.Items)
				if err != nil {
					return nil, err
				}
				return filteredItems, nil
			}
			return rbList, nil
		},
	}

	handlers.HandleList(w, r, cfg)
}

func (h roleBindingHandler) Get(w http.ResponseWriter, r *http.Request) {
	cfg := &handlers.HandlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			id := mux.Vars(r)["id"]
			ctx := r.Context()
			rb, err := h.roleBinding.Get(ctx, id)
			if err != nil {
				return nil, err
			}

			return PresentRoleBinding(rb), nil
		},
	}

	handlers.HandleGet(w, r, cfg)
}

func (h roleBindingHandler) Delete(w http.ResponseWriter, r *http.Request) {
	cfg := &handlers.HandlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			id := mux.Vars(r)["id"]
			ctx := r.Context()
			err := h.roleBinding.Delete(ctx, id)
			if err != nil {
				return nil, err
			}
			return nil, nil
		},
	}
	handlers.HandleDelete(w, r, cfg, http.StatusNoContent)
}
