package agentRuntimes

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

type ServiceLocator func() AgentRuntimeService

func NewServiceLocator(env *environments.Env) ServiceLocator {
	dao := NewAgentRuntimeDao(&env.Database.SessionFactory)
	return func() AgentRuntimeService {
		return NewAgentRuntimeService(dao)
	}
}

func Service(s *environments.Services) AgentRuntimeService {
	if s == nil {
		return nil
	}
	if obj := s.GetService("AgentRuntimes"); obj != nil {
		locator := obj.(ServiceLocator)
		return locator()
	}
	return nil
}

func init() {
	registry.RegisterService("AgentRuntimes", func(env interface{}) interface{} {
		return NewServiceLocator(env.(*environments.Env))
	})

	pkgserver.RegisterPrefixedRoutes("agentRuntimes", "ext", func(extRouter *mux.Router, services pkgserver.ServicesInterface, authMiddleware environments.JWTMiddleware, authzMiddleware auth.AuthorizationMiddleware) {
		envServices := services.(*environments.Services)
		h := NewAgentRuntimeHandler(Service(envServices), generic.Service(envServices))

		router := extRouter.PathPrefix("/agent_runtimes").Subrouter()
		router.HandleFunc("", h.List).Methods(http.MethodGet)
		router.HandleFunc("/{id}", h.Get).Methods(http.MethodGet)
		router.HandleFunc("", h.Create).Methods(http.MethodPost)
		router.HandleFunc("/{id}", h.Patch).Methods(http.MethodPatch)
		router.HandleFunc("/{id}", h.Delete).Methods(http.MethodDelete)
		router.Use(authMiddleware.AuthenticateAccountJWT)
		router.Use(authzMiddleware.AuthorizeApi)
	})

	presenters.RegisterPath(AgentRuntime{}, "agent_runtimes")
	presenters.RegisterPath(&AgentRuntime{}, "agent_runtimes")
	presenters.RegisterKind(AgentRuntime{}, "AgentRuntime")
	presenters.RegisterKind(&AgentRuntime{}, "AgentRuntime")

	db.RegisterMigration(migration())
}
