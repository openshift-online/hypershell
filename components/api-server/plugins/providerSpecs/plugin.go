package providerSpecs

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

type ServiceLocator func() ProviderSpecService

func NewServiceLocator(env *environments.Env) ServiceLocator {
	dao := NewProviderSpecDao(&env.Database.SessionFactory)
	return func() ProviderSpecService {
		return NewProviderSpecService(dao)
	}
}

func Service(s *environments.Services) ProviderSpecService {
	if s == nil {
		return nil
	}
	if obj := s.GetService("ProviderSpecs"); obj != nil {
		locator := obj.(ServiceLocator)
		return locator()
	}
	return nil
}

func init() {
	registry.RegisterService("ProviderSpecs", func(env interface{}) interface{} {
		return NewServiceLocator(env.(*environments.Env))
	})

	pkgserver.RegisterPrefixedRoutes("providerSpecs", "ext", func(extRouter *mux.Router, services pkgserver.ServicesInterface, authMiddleware environments.JWTMiddleware, authzMiddleware auth.AuthorizationMiddleware) {
		envServices := services.(*environments.Services)
		h := NewProviderSpecHandler(Service(envServices), generic.Service(envServices))

		router := extRouter.PathPrefix("/provider_specs").Subrouter()
		router.HandleFunc("", h.List).Methods(http.MethodGet)
		router.HandleFunc("/{id}", h.Get).Methods(http.MethodGet)
		router.HandleFunc("", h.Create).Methods(http.MethodPost)
		router.HandleFunc("/{id}", h.Patch).Methods(http.MethodPatch)
		router.HandleFunc("/{id}", h.Delete).Methods(http.MethodDelete)
		router.Use(authMiddleware.AuthenticateAccountJWT)
		router.Use(authzMiddleware.AuthorizeApi)
	})

	presenters.RegisterPath(ProviderSpec{}, "provider_specs")
	presenters.RegisterPath(&ProviderSpec{}, "provider_specs")
	presenters.RegisterKind(ProviderSpec{}, "ProviderSpec")
	presenters.RegisterKind(&ProviderSpec{}, "ProviderSpec")

	db.RegisterMigration(migration())
}
