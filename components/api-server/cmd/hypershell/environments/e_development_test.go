package environments

import (
	"strings"
	"testing"

	"github.com/openshift-online/rh-trex-ai/pkg/config"
)

// TestDevJWTOverrideWarning covers P2-3: the development environment force-
// disables JWT, and when JWT was explicitly requested that contradiction must be
// reported loudly rather than silently swallowed.
func TestDevJWTOverrideWarning(t *testing.T) {
	if got := devJWTOverrideWarning(false); got != "" {
		t.Fatalf("no JWT requested should not warn, got %q", got)
	}

	warning := devJWTOverrideWarning(true)
	if warning == "" {
		t.Fatal("requesting --enable-jwt=true under development must warn about the override")
	}
	if !strings.Contains(warning, "development_oidc") {
		t.Errorf("warning must point operators at development_oidc, got %q", warning)
	}
	if !strings.Contains(warning, "enable-jwt") {
		t.Errorf("warning must name the overridden flag, got %q", warning)
	}
}

// TestDevOverrideConfigDisablesJWT confirms the environment still enforces the
// dev posture (JWT off, HTTPS off) regardless of the incoming flag value.
func TestDevOverrideConfigDisablesJWT(t *testing.T) {
	env := &DevEnvImpl{}
	c := config.NewApplicationConfig()
	c.Auth.EnableJWT = true
	c.Server.EnableHTTPS = true

	if err := env.OverrideConfig(c); err != nil {
		t.Fatalf("OverrideConfig: %v", err)
	}
	if c.Auth.EnableJWT {
		t.Error("development environment must disable JWT")
	}
	if c.Server.EnableHTTPS {
		t.Error("development environment must disable HTTPS")
	}
}

func TestDevOIDCMetadataBypass(t *testing.T) {
	paths := (&DevOidcEnvImpl{}).Flags()["auth-bypass-paths"]
	if !strings.Contains(paths, "/api/hypershell/v1/metadata") {
		t.Fatalf("auth bypass paths do not contain the metadata endpoint: %q", paths)
	}
}
