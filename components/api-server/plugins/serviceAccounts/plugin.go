package serviceAccounts

import (
	"context"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/gorilla/mux"
	"github.com/openshift-online/hypershell/components/api-server/pkg/keycloak"
	"github.com/openshift-online/hypershell/components/api-server/plugins/gateways"
	"github.com/openshift-online/hypershell/components/api-server/plugins/roleBindings"
	"github.com/openshift-online/rh-trex-ai/pkg/auth"
	"github.com/openshift-online/rh-trex-ai/pkg/db"
	"github.com/openshift-online/rh-trex-ai/pkg/environments"
	"github.com/openshift-online/rh-trex-ai/pkg/registry"
	pkgserver "github.com/openshift-online/rh-trex-ai/pkg/server"
)

type ServiceLocator func() Service

func NewServiceLocator(env *environments.Env) ServiceLocator {
	var once sync.Once
	var service Service
	var locator ServiceLocator
	locator = func() Service {
		once.Do(func() {
			provisioner := keycloak.NewClient(
				os.Getenv("HYPERSHELL_KEYCLOAK_ADMIN_SERVER_URL"),
				os.Getenv("HYPERSHELL_KEYCLOAK_ADMIN_REALM"),
				os.Getenv("HYPERSHELL_KEYCLOAK_ADMIN_CLIENT_ID"),
				os.Getenv("HYPERSHELL_KEYCLOAK_ADMIN_CLIENT_SECRET"),
			)
			service = NewService(
				NewServiceAccountDao(&env.Database.SessionFactory),
				gateways.Service(&env.Services),
				roleBindings.Service(&env.Services),
				provisioner,
				db.NewAdvisoryLockFactory(env.Database.SessionFactory),
			)
			if provisioner.Configured() {
				go runReconciler(service, reconcileInterval())
			}
		})
		return service
	}
	gateways.RegisterDeletionCleaner(func(ctx context.Context, gatewayID string) error {
		return locator().CleanupGateway(ctx, gatewayID)
	})
	return locator
}

func ServiceFrom(s *environments.Services) Service {
	if s == nil {
		return nil
	}
	if object := s.GetService("OpenShellGatewayServiceAccounts"); object != nil {
		return object.(ServiceLocator)()
	}
	return nil
}

func init() {
	registry.RegisterService("OpenShellGatewayServiceAccounts", func(env interface{}) interface{} {
		return NewServiceLocator(env.(*environments.Env))
	})

	pkgserver.RegisterRoutes("openShellGatewayServiceAccounts", func(apiV1Router *mux.Router, services pkgserver.ServicesInterface, authMiddleware environments.JWTMiddleware, authzMiddleware auth.AuthorizationMiddleware) {
		envServices := services.(*environments.Services)
		handler := NewHandler(ServiceFrom(envServices))
		router := apiV1Router.PathPrefix("/gateways/{gateway_id}/service_accounts").Subrouter()
		router.HandleFunc("", handler.List).Methods(http.MethodGet)
		router.HandleFunc("", handler.Create).Methods(http.MethodPost)
		router.HandleFunc("/{service_account_id}", handler.Get).Methods(http.MethodGet)
		router.HandleFunc("/{service_account_id}", handler.Delete).Methods(http.MethodDelete)
		router.HandleFunc("/{service_account_id}/revoke", handler.Revoke).Methods(http.MethodPost)
		router.Use(authMiddleware.AuthenticateAccountJWT)
		router.Use(authzMiddleware.AuthorizeApi)
	})

	db.RegisterMigration(migration())
}

func reconcileInterval() time.Duration {
	value := os.Getenv("HYPERSHELL_SERVICE_ACCOUNT_RECONCILE_INTERVAL_SECONDS")
	if value == "" {
		return 30 * time.Second
	}
	seconds, err := strconv.Atoi(value)
	if err != nil || seconds < 1 || seconds > 60 {
		return 30 * time.Second
	}
	return time.Duration(seconds) * time.Second
}

func runReconciler(service Service, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		ctx, cancel := context.WithTimeout(context.Background(), interval)
		if err := service.ReconcileOnce(ctx); err != nil {
			glog.Warning("OpenShell gateway service-account reconciliation failed")
		}
		cancel()
		<-ticker.C
	}
}
