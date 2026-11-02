package rbac

import (
	"testing"

	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/environments"
)

// Verifies: SEC-RBAC-14.01, SEC-RBAC-14.02
//
// Enforcement follows authentication: Enforced must report exactly the
// resolved Config.Auth.EnableJWT (the value API_ENV overrides have already
// been applied to).
func TestEnforcedFollowsJWTAuthentication(t *testing.T) {
	env := environments.NewEnvironment(nil)
	if env.Config == nil || env.Config.Auth == nil {
		t.Fatal("test environment has no auth config")
	}

	env.Config.Auth.EnableJWT = true
	if !Enforced() {
		t.Error("Enforced() = false with JWT authentication on; want enforced (SEC-RBAC-14.01)")
	}

	env.Config.Auth.EnableJWT = false
	if Enforced() {
		t.Error("Enforced() = true with JWT authentication off; want not enforced (SEC-RBAC-14.02)")
	}
}

// Verifies the test seam used by plugin test mains pins the decision without
// touching the authentication config.
func TestSetEnforcementForTest(t *testing.T) {
	env := environments.Environment()
	saved := env.Config.Auth.EnableJWT
	t.Cleanup(func() {
		env.Config.Auth.EnableJWT = saved
		SetEnforcementForTest(saved)
	})

	env.Config.Auth.EnableJWT = true
	SetEnforcementForTest(false)
	if Enforced() {
		t.Error("Enforced() = true after SetEnforcementForTest(false)")
	}

	SetEnforcementForTest(true)
	env.Config.Auth.EnableJWT = false
	if !Enforced() {
		t.Error("Enforced() = false after SetEnforcementForTest(true)")
	}
}
