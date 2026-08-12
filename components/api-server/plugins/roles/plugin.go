package roles

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

type ServiceLocator func() RoleService

func NewServiceLocator(env *environments.Env) ServiceLocator {
	return func() RoleService {
		return NewRoleService(
			NewRoleDao(&env.Database.SessionFactory),
		)
	}
}

func Service(s *environments.Services) RoleService {
	if s == nil {
		return nil
	}
	if obj := s.GetService("Roles"); obj != nil {
		locator := obj.(ServiceLocator)
		return locator()
	}
	return nil
}

func init() {
	registry.RegisterService("Roles", func(env interface{}) interface{} {
		return NewServiceLocator(env.(*environments.Env))
	})

	pkgserver.RegisterRoutes("roles", func(apiV1Router *mux.Router, services pkgserver.ServicesInterface, authMiddleware environments.JWTMiddleware, authzMiddleware auth.AuthorizationMiddleware) {
		envServices := services.(*environments.Services)
		roleHandler := NewRoleHandler(Service(envServices), generic.Service(envServices))

		rolesRouter := apiV1Router.PathPrefix("/roles").Subrouter()
		rolesRouter.HandleFunc("", roleHandler.List).Methods(http.MethodGet)
		rolesRouter.HandleFunc("/{id}", roleHandler.Get).Methods(http.MethodGet)
		rolesRouter.Use(authMiddleware.AuthenticateAccountJWT)
		rolesRouter.Use(authzMiddleware.AuthorizeApi)
	})

	presenters.RegisterPath(Role{}, "roles")
	presenters.RegisterPath(&Role{}, "roles")
	presenters.RegisterKind(Role{}, "Role")
	presenters.RegisterKind(&Role{}, "Role")

	db.RegisterMigration(migration())
	db.RegisterMigration(migrationSeedBuiltInRoles())
}
