package authz_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/openshift-online/hypershell/components/api-server/test"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/environments"
	pkgserver "github.com/openshift-online/rh-trex-ai/components/api-server/pkg/server"
)

// newCandidateServer boots a second API server from a copy of the test
// environment with EnableJWT overridden, mirroring login_discovery_test.go.
// The RBAC middleware resolves enforcement (rbac.Enforced) from the
// process-global environment at route-registration time, so the global
// EnableJWT is flipped for the candidate's wiring and restored afterwards; the
// helper's own server keeps the enforcement decision it was wired with.
func newCandidateServer(t *testing.T, h *test.Helper, enableJWT bool) string {
	t.Helper()
	env := *h.Env()
	cfg := *env.Config
	auth := *cfg.Auth
	serverConfig := *cfg.Server
	auth.EnableJWT = enableJWT
	serverConfig.BindAddress = "127.0.0.1:0"
	cfg.Auth, cfg.Server = &auth, &serverConfig
	env.Config = &cfg

	global := environments.Environment()
	saved := global.Config.Auth.EnableJWT
	global.Config.Auth.EnableJWT = enableJWT
	candidate := pkgserver.NewDefaultAPIServer(&env, []byte(`{}`))
	global.Config.Auth.EnableJWT = saved

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
	return "http://" + listener.Addr().String() + "/api/hypershell/v1"
}

// Verifies: SEC-RBAC-14.02
//
// Authentication off means not enforced: a server run with JWT off (the
// make run-no-auth posture) must not evaluate requests against bindings: an
// unauthenticated caller is answered by the handler, not rejected by
// authentication or authorization.
func TestAuthOffMeansNotEnforced(t *testing.T) {
	h := test.NewHelper(t)
	base := newCandidateServer(t, h, false)

	client := &http.Client{Timeout: 5 * time.Second}
	response, err := client.Get(base + "/gateways")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET /gateways without a token on a JWT-off server: status = %d; want 200 (not enforced)", response.StatusCode)
	}
}

// Verifies: SEC-RBAC-14.01
//
// Authentication on means enforced: on a server run with JWT on, an
// unauthenticated caller gets 401 and an authenticated caller without
// bindings gets 403.
func TestAuthOnMeansEnforced(t *testing.T) {
	h := test.NewHelper(t)
	base := newCandidateServer(t, h, true)

	client := &http.Client{Timeout: 5 * time.Second}
	response, err := client.Get(base + "/gateways")
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("GET /gateways without a token on a JWT-on server: status = %d; want 401", response.StatusCode)
	}

	token := h.CreateJWTString(h.NewAccount("enforced-roleless", "Enforced Roleless", "enforced-roleless@example.com"))
	request, err := http.NewRequest(http.MethodPost, base+"/gateways", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	createResponse, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = createResponse.Body.Close() }()
	if createResponse.StatusCode != http.StatusForbidden {
		t.Fatalf("POST /gateways with a roleless token on a JWT-on server: status = %d; want 403 (enforced)", createResponse.StatusCode)
	}
}
