package managedDatabases

import (
	"net/http"

	"github.com/gorilla/mux"

	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/rh-trex-ai/pkg/api/presenters"
	"github.com/openshift-online/rh-trex-ai/pkg/errors"
	"github.com/openshift-online/rh-trex-ai/pkg/handlers"
	"github.com/openshift-online/rh-trex-ai/pkg/services"
)

var _ handlers.RestHandler = managedDatabaseHandler{}

type managedDatabaseHandler struct {
	managedDatabase ManagedDatabaseService
	generic         services.GenericService
}

func NewManagedDatabaseHandler(managedDatabase ManagedDatabaseService, generic services.GenericService) *managedDatabaseHandler {
	return &managedDatabaseHandler{
		managedDatabase: managedDatabase,
		generic:         generic,
	}
}

func (h managedDatabaseHandler) Create(w http.ResponseWriter, r *http.Request) {
	var managedDatabase openapi.ManagedDatabase
	cfg := &handlers.HandlerConfig{
		Body: &managedDatabase,
		Validators: []handlers.Validate{
			handlers.ValidateEmpty(&managedDatabase, "Id", "id"),
		},
		Action: func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()
			managedDatabaseModel := ConvertManagedDatabase(managedDatabase)
			managedDatabaseModel, err := h.managedDatabase.Create(ctx, managedDatabaseModel)
			if err != nil {
				return nil, err
			}
			return PresentManagedDatabase(managedDatabaseModel), nil
		},
		ErrorHandler: handlers.HandleError,
	}

	handlers.Handle(w, r, cfg, http.StatusCreated)
}

func (h managedDatabaseHandler) Patch(w http.ResponseWriter, r *http.Request) {
	var patch openapi.ManagedDatabasePatchRequest

	cfg := &handlers.HandlerConfig{
		Body:       &patch,
		Validators: []handlers.Validate{},
		Action: func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()
			id := mux.Vars(r)["id"]
			found, err := h.managedDatabase.Get(ctx, id)
			if err != nil {
				return nil, err
			}

			if patch.Name != nil {
				found.Name = *patch.Name
			}
			if patch.Provider != nil {
				found.Provider = *patch.Provider
			}
			if patch.Region != nil {
				found.Region = patch.Region
			}
			if patch.Engine != nil {
				found.Engine = patch.Engine
			}
			if patch.EngineVersion != nil {
				found.EngineVersion = patch.EngineVersion
			}
			if patch.InstanceClass != nil {
				found.InstanceClass = patch.InstanceClass
			}
			if patch.ConnectionSecret != nil {
				found.ConnectionSecret = patch.ConnectionSecret
			}
			if patch.Status != nil {
				found.Status = patch.Status
			}

			managedDatabaseModel, err := h.managedDatabase.Replace(ctx, found)
			if err != nil {
				return nil, err
			}
			return PresentManagedDatabase(managedDatabaseModel), nil
		},
		ErrorHandler: handlers.HandleError,
	}

	handlers.Handle(w, r, cfg, http.StatusOK)
}

func (h managedDatabaseHandler) List(w http.ResponseWriter, r *http.Request) {
	cfg := &handlers.HandlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()

			listArgs := services.NewListArguments(r.URL.Query())
			var managedDatabases []ManagedDatabase
			paging, err := h.generic.List(ctx, "id", listArgs, &managedDatabases)
			if err != nil {
				return nil, err
			}
			kindStr := "ManagedDatabaseList"
			pageVal := int32(paging.Page)
			sizeVal := int32(paging.Size)
			totalVal := int32(paging.Total)
			managedDatabaseList := openapi.ManagedDatabaseList{
				Kind:  &kindStr,
				Page:  &pageVal,
				Size:  &sizeVal,
				Total: &totalVal,
				Items: []openapi.ManagedDatabase{},
			}

			for _, managedDatabase := range managedDatabases {
				converted := PresentManagedDatabase(&managedDatabase)
				managedDatabaseList.Items = append(managedDatabaseList.Items, converted)
			}
			if listArgs.Fields != nil {
				filteredItems, err := presenters.SliceFilter(listArgs.Fields, managedDatabaseList.Items)
				if err != nil {
					return nil, err
				}
				return filteredItems, nil
			}
			return managedDatabaseList, nil
		},
	}

	handlers.HandleList(w, r, cfg)
}

func (h managedDatabaseHandler) Get(w http.ResponseWriter, r *http.Request) {
	cfg := &handlers.HandlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			id := mux.Vars(r)["id"]
			ctx := r.Context()
			managedDatabase, err := h.managedDatabase.Get(ctx, id)
			if err != nil {
				return nil, err
			}

			return PresentManagedDatabase(managedDatabase), nil
		},
	}

	handlers.HandleGet(w, r, cfg)
}

func (h managedDatabaseHandler) Delete(w http.ResponseWriter, r *http.Request) {
	cfg := &handlers.HandlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			id := mux.Vars(r)["id"]
			ctx := r.Context()
			err := h.managedDatabase.Delete(ctx, id)
			if err != nil {
				return nil, err
			}
			return nil, nil
		},
	}
	handlers.HandleDelete(w, r, cfg, http.StatusNoContent)
}
