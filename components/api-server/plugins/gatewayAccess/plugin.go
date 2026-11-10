package gatewayAccess

import (
	"net/http"

	"github.com/openshift-online/hypershell/components/api-server/pkg/routes"
	"github.com/openshift-online/hypershell/components/api-server/plugins/defaultWorkspaceBootstrap"
	"github.com/openshift-online/hypershell/components/api-server/plugins/gateways"
	"github.com/openshift-online/hypershell/components/api-server/plugins/roleBindings"
	"github.com/openshift-online/hypershell/components/api-server/plugins/users"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/db"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/environments"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/registry"
	pkgserver "github.com/openshift-online/rh-trex-ai/components/api-server/pkg/server"
)

type ServiceLocator func() Service

func NewServiceLocator(env *environments.Env) ServiceLocator {
	return func() Service {
		gatewayLookup := gatewayLookupOverride
		if gatewayLookup == nil {
			gatewayLookup = gateways.Service(&env.Services)
		}
		bootstrap := defaultWorkspaceBootstrapOverride
		if bootstrap == nil {
			bootstrap = defaultWorkspaceBootstrap.ServiceFrom(&env.Services)
		}
		return NewService(
			newDao(&env.Database.SessionFactory),
			roleBindings.Service(&env.Services),
			users.Service(&env.Services),
			newDirectoryResolverFromEnvironment(),
			db.NewAdvisoryLockFactory(env.Database.SessionFactory),
			gatewayLookup,
			bootstrap,
		)
	}
}

func ServiceFrom(s *environments.Services) Service {
	if s == nil {
		return nil
	}
	if obj := s.GetService("GatewayAccess"); obj != nil {
		return obj.(ServiceLocator)()
	}
	return nil
}

func init() {
	registry.RegisterService("GatewayAccess", func(env interface{}) interface{} {
		return NewServiceLocator(env.(*environments.Env))
	})

	pkgserver.RegisterRoutes("gatewayAccess", func(apiRouter *pkgserver.APIRouter, services pkgserver.ServicesInterface) {
		envServices := services.(*environments.Services)
		h := NewHandler(ServiceFrom(envServices))
		router := routes.Resource(apiRouter, "access").PathPrefix("/gateways/{gateway_id}/access").Subrouter()
		router.HandleFunc("", h.List).Methods(http.MethodGet)
		router.HandleFunc("", h.Grant).Methods(http.MethodPost)
		// /directory is registered before /{user_id} so the literal segment is
		// not captured as a user id.
		router.HandleFunc("/directory", h.SearchDirectory).Methods(http.MethodGet)
		router.HandleFunc("/{user_id}", h.ChangeRole).Methods(http.MethodPatch)
		router.HandleFunc("/{user_id}", h.Revoke).Methods(http.MethodDelete)
	})
}
