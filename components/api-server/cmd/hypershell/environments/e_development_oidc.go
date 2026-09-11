package environments

import (
	"github.com/openshift-online/rh-trex-ai/pkg/config"
	"github.com/openshift-online/rh-trex-ai/pkg/db/db_session"
	pkgenv "github.com/openshift-online/rh-trex-ai/pkg/environments"
)

type DevOidcEnvImpl struct {
	Env *pkgenv.Env
}

var _ pkgenv.EnvironmentImpl = &DevOidcEnvImpl{}

func (e *DevOidcEnvImpl) OverrideDatabase(c *pkgenv.Database) error {
	c.SessionFactory = db_session.NewProdFactory(e.Env.Config.Database)
	return nil
}

func (e *DevOidcEnvImpl) OverrideConfig(c *config.ApplicationConfig) error {
	c.Server.EnableHTTPS = false
	return nil
}

func (e *DevOidcEnvImpl) OverrideServices(s *pkgenv.Services) error {
	return nil
}

func (e *DevOidcEnvImpl) OverrideHandlers(h *pkgenv.Handlers) error {
	return nil
}

func (e *DevOidcEnvImpl) OverrideClients(c *pkgenv.Clients) error {
	return nil
}

func (e *DevOidcEnvImpl) Flags() map[string]string {
	return map[string]string{
		"v":                    "10",
		"enable-jwt":           "true",
		"enable-authz":         "false",
		"debug":                "false",
		"enable-mock":          "true",
		"enable-https":         "false",
		"enable-metrics-https": "false",
		"api-server-hostname":  "localhost",
		"auth-bypass-paths":    "/healthcheck,/metrics,/api/hypershell/v1/openapi,/openapi",
		"auth-bypass-methods":  "/grpc.health.v1.Health/,/grpc.reflection.v1alpha.ServerReflection/,/hypershell.v1.GatewayService/WatchGateways,/hypershell.v1.GatewayReleaseService/WatchGatewayReleases,/hypershell.v1.ManagedClusterService/WatchManagedClusters,/hypershell.v1.ManagedDatabaseService/WatchManagedDatabases,/hypershell.v1.GatewayNetworkService/WatchGatewayNetworks",
	}
}
