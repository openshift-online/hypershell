package environments

import (
	"github.com/golang/glog"
	"github.com/openshift-online/rh-trex-ai/pkg/config"
	"github.com/openshift-online/rh-trex-ai/pkg/db/db_session"
	pkgenv "github.com/openshift-online/rh-trex-ai/pkg/environments"
)

type DevEnvImpl struct {
	Env *pkgenv.Env
}

var _ pkgenv.EnvironmentImpl = &DevEnvImpl{}

func (e *DevEnvImpl) OverrideDatabase(c *pkgenv.Database) error {
	c.SessionFactory = db_session.NewProdFactory(e.Env.Config.Database)
	return nil
}

func (e *DevEnvImpl) OverrideConfig(c *config.ApplicationConfig) error {
	// OverrideConfig runs after CLI flags are parsed, so c.Auth.EnableJWT already
	// reflects an explicit --enable-jwt=true. Force-disabling it here silently
	// would leave every authenticated request returning 401 with no clue why, so
	// surface the contradiction loudly (P2-3) before applying the override. Use
	// API_ENV=development_oidc for JWT/RBAC enforcement.
	if warning := devJWTOverrideWarning(c.Auth.EnableJWT); warning != "" {
		glog.Warning(warning)
	}
	c.Auth.EnableJWT = false
	c.Server.EnableHTTPS = false
	return nil
}

// devJWTOverrideWarning returns the operator-facing warning that should be
// logged when the development environment disables JWT. It is non-empty only
// when JWT was explicitly requested, so the log states why enforcement is off
// despite --enable-jwt=true. Extracted for testability.
func devJWTOverrideWarning(requestedJWT bool) string {
	if requestedJWT {
		return "API_ENV=development force-disables JWT authentication; this overrides --enable-jwt=true. " +
			"Use API_ENV=development_oidc to run with JWT/RBAC enforcement."
	}
	return ""
}

func (e *DevEnvImpl) OverrideServices(s *pkgenv.Services) error {
	return nil
}

func (e *DevEnvImpl) OverrideHandlers(h *pkgenv.Handlers) error {
	return nil
}

func (e *DevEnvImpl) OverrideClients(c *pkgenv.Clients) error {
	return nil
}

func (e *DevEnvImpl) Flags() map[string]string {
	return map[string]string{
		"v":                      "10",
		"enable-authz":           "false",
		"debug":                  "false",
		"enable-mock":            "true",
		"enable-https":           "false",
		"enable-metrics-https":   "false",
		"api-server-hostname":    "localhost",
		"api-server-bindaddress": "localhost:8000",
	}
}
