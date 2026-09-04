package users

import (
	"net/http"

	"github.com/gorilla/mux"

	"github.com/openshift-online/rh-trex-ai/pkg/api/presenters"
	"github.com/openshift-online/rh-trex-ai/pkg/auth"
	"github.com/openshift-online/rh-trex-ai/pkg/db"
	"github.com/openshift-online/rh-trex-ai/pkg/environments"
	"github.com/openshift-online/rh-trex-ai/pkg/registry"
	pkgserver "github.com/openshift-online/rh-trex-ai/pkg/server"
	"github.com/openshift-online/rh-trex-ai/plugins/generic"
)

type ServiceLocator func() UserService

func NewServiceLocator(env *environments.Env) ServiceLocator {
	return func() UserService {
		return NewUserService(
			NewUserDao(&env.Database.SessionFactory),
		)
	}
}

func Service(s *environments.Services) UserService {
	if s == nil {
		return nil
	}
	if obj := s.GetService("Users"); obj != nil {
		locator := obj.(ServiceLocator)
		return locator()
	}
	return nil
}

func init() {
	registry.RegisterService("Users", func(env interface{}) interface{} {
		return NewServiceLocator(env.(*environments.Env))
	})

	pkgserver.RegisterRoutes("users", func(apiV1Router *mux.Router, services pkgserver.ServicesInterface, authMiddleware environments.JWTMiddleware, authzMiddleware auth.AuthorizationMiddleware) {
		envServices := services.(*environments.Services)
		userHandler := NewUserHandler(Service(envServices), generic.Service(envServices))

		usersRouter := apiV1Router.PathPrefix("/users").Subrouter()
		usersRouter.HandleFunc("", userHandler.List).Methods(http.MethodGet)
		usersRouter.HandleFunc("/{id}", userHandler.Get).Methods(http.MethodGet)
		usersRouter.Use(authMiddleware.AuthenticateAccountJWT)
		usersRouter.Use(authzMiddleware.AuthorizeApi)
	})

	presenters.RegisterPath(User{}, "users")
	presenters.RegisterPath(&User{}, "users")
	presenters.RegisterKind(User{}, "User")
	presenters.RegisterKind(&User{}, "User")

	db.RegisterMigration(migration())
}
