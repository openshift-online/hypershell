package rbac

import (
	"os"
	"strings"

	"github.com/gorilla/mux"

	"github.com/openshift-online/hypershell/components/api-server/pkg/rbac"
	"github.com/openshift-online/hypershell/components/api-server/plugins/roleBindings"
	"github.com/openshift-online/hypershell/components/api-server/plugins/users"
	"github.com/openshift-online/rh-trex-ai/pkg/auth"
	"github.com/openshift-online/rh-trex-ai/pkg/environments"
	pkgserver "github.com/openshift-online/rh-trex-ai/pkg/server"
)

func init() {
	pkgserver.RegisterRoutes("rbac", func(apiV1Router *mux.Router, services pkgserver.ServicesInterface, authMiddleware environments.JWTMiddleware, authzMiddleware auth.AuthorizationMiddleware) {
		envServices := services.(*environments.Services)

		userService := users.Service(envServices)
		rbService := roleBindings.Service(envServices)
		activityRecorder := users.ActivityRecorder(envServices)

		if userService != nil {
			provisioner := rbac.NewUserProvisioner(userService)
			var syncer rbac.JWTRoleSyncer
			if rbService != nil {
				syncer = rbService
			}
			apiV1Router.Use(rbac.UserProvisioningMiddleware(provisioner, syncer))
		}
		if rbService != nil {
			enforceRBAC := os.Getenv("RBAC_ENFORCE") == "true"
			var serviceAccounts []string
			if sa := os.Getenv("RBAC_SERVICE_ACCOUNTS"); sa != "" {
				for _, s := range strings.Split(sa, ",") {
					if trimmed := strings.TrimSpace(s); trimmed != "" {
						serviceAccounts = append(serviceAccounts, trimmed)
					}
				}
			}
			authzConfig := rbac.AuthzConfig{
				EnforceRBAC:     enforceRBAC,
				ServiceAccounts: serviceAccounts,
			}
			rbacMiddleware := rbac.NewRBACAuthzMiddleware(rbService, authzConfig, activityRecorder)
			apiV1Router.Use(rbacMiddleware.AuthorizeApi)
		}
	})
}
