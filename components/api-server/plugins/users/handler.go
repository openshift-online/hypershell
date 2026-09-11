package users

import (
	"net/http"

	"github.com/gorilla/mux"

	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/rh-trex-ai/pkg/api/presenters"
	"github.com/openshift-online/rh-trex-ai/pkg/errors"
	"github.com/openshift-online/rh-trex-ai/pkg/handlers"
	"github.com/openshift-online/rh-trex-ai/pkg/services"
)

type userHandler struct {
	user    UserService
	generic services.GenericService
}

func NewUserHandler(user UserService, generic services.GenericService) *userHandler {
	return &userHandler{
		user:    user,
		generic: generic,
	}
}

func (h userHandler) List(w http.ResponseWriter, r *http.Request) {
	cfg := &handlers.HandlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()

			listArgs := services.NewListArguments(r.URL.Query())
			if len(listArgs.OrderBy) == 0 {
				listArgs.OrderBy = []string{"username asc"}
			}

			var users []User
			paging, err := h.generic.List(ctx, "id", listArgs, &users)
			if err != nil {
				return nil, err
			}
			kindStr := "UserList"
			pageVal := int32(paging.Page)
			sizeVal := int32(paging.Size)
			totalVal := int32(paging.Total)
			userList := openapi.UserList{
				Kind:  &kindStr,
				Page:  &pageVal,
				Size:  &sizeVal,
				Total: &totalVal,
				Items: []openapi.User{},
			}

			for _, user := range users {
				converted := PresentUser(&user)
				userList.Items = append(userList.Items, converted)
			}
			if listArgs.Fields != nil {
				filteredItems, filterErr := presenters.SliceFilter(listArgs.Fields, userList.Items)
				if filterErr != nil {
					return nil, filterErr
				}
				return filteredItems, nil
			}
			return userList, nil
		},
	}

	handlers.HandleList(w, r, cfg)
}

func (h userHandler) Get(w http.ResponseWriter, r *http.Request) {
	cfg := &handlers.HandlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			id := mux.Vars(r)["id"]
			ctx := r.Context()
			user, err := h.user.Get(ctx, id)
			if err != nil {
				return nil, err
			}

			return PresentUser(user), nil
		},
	}

	handlers.HandleGet(w, r, cfg)
}
