package providerBindings

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

type ServiceLocator func() ProviderBindingService

func NewServiceLocator(env *environments.Env) ServiceLocator {
	dao := NewProviderBindingDao(&env.Database.SessionFactory)
	return func() ProviderBindingService {
		return NewProviderBindingService(dao)
	}
}

func Service(s *environments.Services) ProviderBindingService {
	if s == nil {
		return nil
	}
	if obj := s.GetService("ProviderBindings"); obj != nil {
		locator := obj.(ServiceLocator)
		return locator()
	}
	return nil
}

func init() {
	registry.RegisterService("ProviderBindings", func(env interface{}) interface{} {
		return NewServiceLocator(env.(*environments.Env))
	})

	pkgserver.RegisterRoutes("providerBindings", func(apiV1Router *mux.Router, services pkgserver.ServicesInterface, authMiddleware environments.JWTMiddleware, authzMiddleware auth.AuthorizationMiddleware) {
		envServices := services.(*environments.Services)
		h := NewProviderBindingHandler(Service(envServices), generic.Service(envServices))

		router := apiV1Router.PathPrefix("/provider_bindings").Subrouter()
		router.HandleFunc("", h.List).Methods(http.MethodGet)
		router.HandleFunc("/{id}", h.Get).Methods(http.MethodGet)
		router.HandleFunc("", h.Create).Methods(http.MethodPost)
		router.HandleFunc("/{id}", h.Patch).Methods(http.MethodPatch)
		router.HandleFunc("/{id}", h.Delete).Methods(http.MethodDelete)
		router.Use(authMiddleware.AuthenticateAccountJWT)
		router.Use(authzMiddleware.AuthorizeApi)
	})

	presenters.RegisterPath(ProviderBinding{}, "provider_bindings")
	presenters.RegisterPath(&ProviderBinding{}, "provider_bindings")
	presenters.RegisterKind(ProviderBinding{}, "ProviderBinding")
	presenters.RegisterKind(&ProviderBinding{}, "ProviderBinding")

	db.RegisterMigration(migration())
}
