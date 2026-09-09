package roleBindings

import (
	"net/http"

	"github.com/gorilla/mux"
	"google.golang.org/grpc"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/api-server/plugins/roles"
	"github.com/openshift-online/hypershell/components/api-server/plugins/users"
	"github.com/openshift-online/rh-trex-ai/pkg/api"
	"github.com/openshift-online/rh-trex-ai/pkg/api/presenters"
	"github.com/openshift-online/rh-trex-ai/pkg/auth"
	"github.com/openshift-online/rh-trex-ai/pkg/controllers"
	"github.com/openshift-online/rh-trex-ai/pkg/db"
	"github.com/openshift-online/rh-trex-ai/pkg/environments"
	"github.com/openshift-online/rh-trex-ai/pkg/registry"
	pkgserver "github.com/openshift-online/rh-trex-ai/pkg/server"
	"github.com/openshift-online/rh-trex-ai/plugins/events"
	"github.com/openshift-online/rh-trex-ai/plugins/generic"
)

type ServiceLocator func() RoleBindingService

func NewServiceLocator(env *environments.Env) ServiceLocator {
	return func() RoleBindingService {
		return NewRoleBindingService(
			db.NewAdvisoryLockFactory(env.Database.SessionFactory),
			NewRoleBindingDao(&env.Database.SessionFactory),
			roles.NewRoleDao(&env.Database.SessionFactory),
			events.Service(&env.Services),
		)
	}
}

func Service(s *environments.Services) RoleBindingService {
	if s == nil {
		return nil
	}
	if obj := s.GetService("RoleBindings"); obj != nil {
		locator := obj.(ServiceLocator)
		return locator()
	}
	return nil
}

func init() {
	registry.RegisterService("RoleBindings", func(env interface{}) interface{} {
		return NewServiceLocator(env.(*environments.Env))
	})

	pkgserver.RegisterRoutes("roleBindings", func(apiV1Router *mux.Router, services pkgserver.ServicesInterface, authMiddleware environments.JWTMiddleware, authzMiddleware auth.AuthorizationMiddleware) {
		envServices := services.(*environments.Services)
		rbHandler := NewRoleBindingHandler(Service(envServices), generic.Service(envServices))

		rbRouter := apiV1Router.PathPrefix("/role_bindings").Subrouter()
		rbRouter.HandleFunc("", rbHandler.List).Methods(http.MethodGet)
		rbRouter.HandleFunc("/{id}", rbHandler.Get).Methods(http.MethodGet)
		rbRouter.HandleFunc("", rbHandler.Create).Methods(http.MethodPost)
		rbRouter.HandleFunc("/{id}", rbHandler.Delete).Methods(http.MethodDelete)
		rbRouter.Use(authMiddleware.AuthenticateAccountJWT)
		rbRouter.Use(authzMiddleware.AuthorizeApi)
	})

	pkgserver.RegisterController("RoleBindings", func(manager *controllers.KindControllerManager, services pkgserver.ServicesInterface) {
		rbService := Service(services.(*environments.Services))

		manager.Add(&controllers.ControllerConfig{
			Source: "RoleBindings",
			Handlers: map[api.EventType][]controllers.ControllerHandlerFunc{
				api.CreateEventType: {rbService.OnUpsert},
				api.UpdateEventType: {rbService.OnUpsert},
				api.DeleteEventType: {rbService.OnDelete},
			},
		})
	})

	pkgserver.RegisterGRPCService("roleBindings", func(grpcServer *grpc.Server, services pkgserver.ServicesInterface) {
		envServices := services.(*environments.Services)
		rbService := Service(envServices)
		roleService := roles.Service(envServices)
		userService := users.Service(envServices)
		brokerFunc := func() *pkgserver.EventBroker {
			if obj := envServices.GetService("EventBroker"); obj != nil {
				return obj.(*pkgserver.EventBroker)
			}
			return nil
		}
		pb.RegisterRoleBindingServiceServer(grpcServer, NewRoleBindingGRPCHandler(rbService, roleService, userService, brokerFunc))
	})

	presenters.RegisterPath(RoleBinding{}, "role_bindings")
	presenters.RegisterPath(&RoleBinding{}, "role_bindings")
	presenters.RegisterKind(RoleBinding{}, "RoleBinding")
	presenters.RegisterKind(&RoleBinding{}, "RoleBinding")

	db.RegisterMigration(migration())
	db.RegisterMigration(migrationAddTraceContext())
}
