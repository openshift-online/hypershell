package rbac

import (
	"log/slog"
	"os"
	"sync"

	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/environments"
)

// enforcementOverride is the test-only pin of the enforcement decision. See
// SetEnforcementForTest.
var enforcementOverride *bool

// staleEnforceWarnOnce guards the one-release startup warning for overlays
// that still set RBAC_ENFORCE.
var staleEnforceWarnOnce sync.Once

// Enforced reports whether authorization is enforced. Enforcement follows
// authentication (SEC-RBAC-14): it is on exactly when JWT authentication is on
// in the resolved configuration (Config.Auth.EnableJWT, after API_ENV
// overrides), and off when authentication is off (local development,
// `make run-no-auth`), where there is no caller identity to evaluate. There is
// no separate enforcement setting: RBAC_ENFORCE has no effect, and a stale
// value logs a one-time startup warning so outdated overlays are noticed.
//
// This is the single decision point shared by the REST authorization
// middleware, the gRPC interceptor, and every handler-level authorization
// check (the gateway handlers, the dedicated-route authorizer, and the ext-kind
// authorizers). None of them may read an enforcement environment variable.
// The decision is resolved once per wiring (route registration, gRPC service
// registration), which happens after the environment is initialized.
func Enforced() bool {
	staleEnforceWarnOnce.Do(func() {
		if _, set := os.LookupEnv("RBAC_ENFORCE"); set {
			slog.Warn("RBAC_ENFORCE is set but has no effect: authorization is enforced whenever JWT authentication is enabled (SEC-RBAC-14). Remove it from the deployment configuration.")
		}
	})
	if enforcementOverride != nil {
		return *enforcementOverride
	}
	env := environments.Environment()
	if env.Config == nil || env.Config.Auth == nil {
		return false
	}
	return env.Config.Auth.EnableJWT
}

// SetEnforcementForTest pins the process-wide enforcement decision returned by
// Enforced, bypassing the derivation from the resolved authentication config.
// It exists only for test mains: the test environments run with mocked JWT
// (authentication on), so enforcement is on by default; a package whose tests
// exercise plugin behavior rather than authorization pins enforcement off,
// while test/authz and plugins/users run with the real derivation (enforced).
// Production code must never call it.
func SetEnforcementForTest(enforced bool) {
	enforcementOverride = &enforced
}
