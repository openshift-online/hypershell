package inferenceRoutes

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

type ServiceLocator func() InferenceRouteService

func NewServiceLocator(env *environments.Env) ServiceLocator {
	dao := NewInferenceRouteDao(&env.Database.SessionFactory)
	return func() InferenceRouteService {
		return NewInferenceRouteService(dao)
	}
}

func Service(s *environments.Services) InferenceRouteService {
	if s == nil {
		return nil
	}
	if obj := s.GetService("InferenceRoutes"); obj != nil {
		locator := obj.(ServiceLocator)
		return locator()
	}
	return nil
}

func init() {
	registry.RegisterService("InferenceRoutes", func(env interface{}) interface{} {
		return NewServiceLocator(env.(*environments.Env))
	})

	pkgserver.RegisterRoutes("inferenceRoutes", func(apiV1Router *mux.Router, services pkgserver.ServicesInterface, authMiddleware environments.JWTMiddleware, authzMiddleware auth.AuthorizationMiddleware) {
		envServices := services.(*environments.Services)
		h := NewInferenceRouteHandler(Service(envServices), generic.Service(envServices))

		router := apiV1Router.PathPrefix("/inference_routes").Subrouter()
		router.HandleFunc("", h.List).Methods(http.MethodGet)
		router.HandleFunc("/{id}", h.Get).Methods(http.MethodGet)
		router.HandleFunc("", h.Create).Methods(http.MethodPost)
		router.HandleFunc("/{id}", h.Delete).Methods(http.MethodDelete)
		router.Use(authMiddleware.AuthenticateAccountJWT)
		router.Use(authzMiddleware.AuthorizeApi)
	})

	presenters.RegisterPath(InferenceRoute{}, "inference_routes")
	presenters.RegisterPath(&InferenceRoute{}, "inference_routes")
	presenters.RegisterKind(InferenceRoute{}, "InferenceRoute")
	presenters.RegisterKind(&InferenceRoute{}, "InferenceRoute")

	db.RegisterMigration(migration())
}
