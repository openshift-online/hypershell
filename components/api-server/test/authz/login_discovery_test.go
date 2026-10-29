package authz_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/openshift-online/hypershell/components/api-server/test"
	pkgserver "github.com/openshift-online/rh-trex-ai/components/api-server/pkg/server"
)

// Verifies: SEC-OIDC-12.01
func TestPublicLoginDiscoveryWithRBAC(t *testing.T) {
	h := test.NewHelper(t)
	// Copy configuration so the existing integration server and its callers
	// retain their issuer settings. Reuse the registered HyperShell services.
	env := *h.Env()
	cfg := *env.Config
	auth := *cfg.Auth
	serverConfig := *cfg.Server
	auth.EnableJWT = true
	auth.JWTIssuer = "https://login.example.com/realms/hypershell"
	serverConfig.BindAddress = "127.0.0.1:0"
	cfg.Auth, cfg.Server = &auth, &serverConfig
	env.Config = &cfg
	candidate := pkgserver.NewDefaultAPIServer(&env, []byte(`{}`))
	listener, err := candidate.Listen()
	if err != nil {
		t.Fatal(err)
	}
	go candidate.Serve(listener)
	t.Cleanup(func() {
		if err := candidate.Stop(); err != nil {
			t.Error(err)
		}
	})
	client := &http.Client{Timeout: 5 * time.Second}
	base := "http://" + listener.Addr().String() + "/api/hypershell/v1"
	response, err := client.Get(base + "/auth/config")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("discovery status = %d", response.StatusCode)
	}
	var metadata map[string]string
	if err := json.NewDecoder(response.Body).Decode(&metadata); err != nil {
		t.Fatal(err)
	}
	if len(metadata) != 1 || metadata["issuer_url"] != auth.JWTIssuer {
		t.Fatalf("metadata = %v", metadata)
	}
	for _, path := range []string{"/gateways", "/auth/config/private"} {
		response, err := client.Get(base + path)
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
		if response.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s status = %d; want 401", path, response.StatusCode)
		}
	}
}
