package secretSources

import (
	"net/http"

	"github.com/gorilla/mux"

	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api/presenters"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/auth"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/db"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/environments"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/registry"
	pkgserver "github.com/openshift-online/rh-trex-ai/components/api-server/pkg/server"
	"github.com/openshift-online/rh-trex-ai/components/api-server/plugins/generic"
)

type ServiceLocator func() SecretSourceService

func NewServiceLocator(env *environments.Env) ServiceLocator {
	dao := NewSecretSourceDao(&env.Database.SessionFactory)
	return func() SecretSourceService {
		return NewSecretSourceService(dao)
	}
}

func Service(s *environments.Services) SecretSourceService {
	if s == nil {
		return nil
	}
	if obj := s.GetService("SecretSources"); obj != nil {
		locator := obj.(ServiceLocator)
		return locator()
	}
	return nil
}

func init() {
	registry.RegisterService("SecretSources", func(env interface{}) interface{} {
		return NewServiceLocator(env.(*environments.Env))
	})

	pkgserver.RegisterPrefixedRoutes("secretSources", "ext", func(extRouter *mux.Router, services pkgserver.ServicesInterface, authMiddleware environments.JWTMiddleware, authzMiddleware auth.AuthorizationMiddleware) {
		envServices := services.(*environments.Services)
		h := NewSecretSourceHandler(Service(envServices), generic.Service(envServices))

		router := extRouter.PathPrefix("/secret_sources").Subrouter()
		router.HandleFunc("", h.List).Methods(http.MethodGet)
		router.HandleFunc("/{id}", h.Get).Methods(http.MethodGet)
		router.HandleFunc("", h.Create).Methods(http.MethodPost)
		router.HandleFunc("/{id}", h.Patch).Methods(http.MethodPatch)
		router.HandleFunc("/{id}", h.Delete).Methods(http.MethodDelete)
		router.Use(authMiddleware.AuthenticateAccountJWT)
		router.Use(authzMiddleware.AuthorizeApi)
	})

	presenters.RegisterPath(SecretSource{}, "secret_sources")
	presenters.RegisterPath(&SecretSource{}, "secret_sources")
	presenters.RegisterKind(SecretSource{}, "SecretSource")
	presenters.RegisterKind(&SecretSource{}, "SecretSource")

	db.RegisterMigration(migration())
}
