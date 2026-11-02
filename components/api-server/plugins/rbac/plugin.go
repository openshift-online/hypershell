package rbac

import (
	"github.com/gorilla/mux"

	"github.com/openshift-online/hypershell/components/api-server/pkg/rbac"
	"github.com/openshift-online/hypershell/components/api-server/plugins/roleBindings"
	"github.com/openshift-online/hypershell/components/api-server/plugins/users"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/auth"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/environments"
	pkgserver "github.com/openshift-online/rh-trex-ai/components/api-server/pkg/server"
)

func init() {
	pkgserver.RegisterRoutes("rbac", func(apiV1Router *mux.Router, services pkgserver.ServicesInterface, authMiddleware environments.JWTMiddleware, authzMiddleware auth.AuthorizationMiddleware) {
		envServices := services.(*environments.Services)
		useRBACMiddleware(apiV1Router, envServices)
	})

	// The Extension API kinds are served under /api/hypershell/ext/, on a
	// subrouter the /v1 middleware never wraps. Without this registration the
	// framework's authorizer was the only gate on those routes, and it checks
	// nothing beyond identity presence, so any authenticated caller could
	// write every ext kind. All "ext"-prefixed plugins share one subrouter
	// (pkgserver.LoadDiscoveredPrefixedRoutes), so middlewares added here
	// apply to every ext route regardless of registration order (gorilla/mux
	// builds the middleware chain at match time).
	pkgserver.RegisterPrefixedRoutes("rbac", "ext", func(extRouter *mux.Router, services pkgserver.ServicesInterface, authMiddleware environments.JWTMiddleware, authzMiddleware auth.AuthorizationMiddleware) {
		envServices := services.(*environments.Services)
		useRBACMiddleware(extRouter, envServices)
	})
}

// useRBACMiddleware attaches the user-provisioning and authorization
// middlewares to a router serving one API surface (/v1 or /ext). It is
// idempotent per router only because each surface registers it exactly once;
// the services are resolved per registration so the wiring follows the
// environment's service registry.
func useRBACMiddleware(router *mux.Router, envServices *environments.Services) {
	userService := users.Service(envServices)
	rbService := roleBindings.Service(envServices)
	activityRecorder := users.ActivityRecorder(envServices)

	if userService != nil {
		provisioner := rbac.NewUserProvisioner(userService)
		var syncer rbac.JWTRoleSyncer
		if rbService != nil {
			syncer = rbService
		}
		router.Use(rbac.UserProvisioningMiddleware(provisioner, syncer))
	}
	if rbService != nil {
		authzConfig := rbac.AuthzConfig{
			EnforceRBAC:     rbac.Enforced(),
			ServiceAccounts: rbac.ServiceAccountsFromEnv(),
		}
		// nil when the managedClusters plugin is absent: registered-cluster
		// callers are then not recognised as control planes.
		controlPlanes := newControlPlaneResolver(envServices)
		rbacMiddleware := rbac.NewRBACAuthzMiddleware(rbService, authzConfig, activityRecorder, controlPlanes)
		router.Use(rbacMiddleware.AuthorizeApi)
	}
}
