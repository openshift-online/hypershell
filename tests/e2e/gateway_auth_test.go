package e2e

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	sdktypes "github.com/openshift-online/hypershell/components/sdk-go/types"
	"github.com/openshift-online/hypershell/tests/e2e/apiclient"
	"github.com/openshift-online/hypershell/tests/e2e/driver"
)

const (
	testSecret    = "s3cr3t-value-never-logged"
	testSubject   = "service-account-subject"
	testAccountID = "sa-1"
	testClientID  = "service-account-client"
)

var testGateway = driver.GatewayRef{Name: "e2e-gw-1", Namespace: "gw-ns", ID: "gw-id"}

// Verifies: E2E-CORE-40.01
func TestResolveGatewayAuth(t *testing.T) {
	for _, tt := range []struct {
		value, mode, want, wantErr string
	}{
		{mode: modeShort, want: gatewayAuthIdentity},
		{mode: modeLong, want: gatewayAuthIdentity},
		{value: "identity", mode: modeLong, want: gatewayAuthIdentity},
		{value: "service_account", mode: modeShort, want: gatewayAuthServiceAccount},
		{value: "service_account", mode: modeLong, wantErr: "E2E_GATEWAY_AUTH=service_account requires E2E_MODE=short"},
		{value: "unknown", mode: modeShort, wantErr: "E2E_GATEWAY_AUTH must be identity or service_account"},
	} {
		got, err := resolveGatewayAuth(tt.value, tt.mode)
		if tt.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("resolveGatewayAuth(%q, %q) error = %v, want %q", tt.value, tt.mode, err, tt.wantErr)
			}
			continue
		}
		if err != nil || got != tt.want {
			t.Errorf("resolveGatewayAuth(%q, %q) = %q, %v, want %q", tt.value, tt.mode, got, err, tt.want)
		}
	}
}

// fakeSAAPI stands in for the SDK's gateway service-account client.
type fakeSAAPI struct {
	issuer   string
	mutate   func(*sdktypes.OpenShellGatewayServiceAccountCreateResponse)
	onRevoke func()

	mu               sync.Mutex
	creates          []*sdktypes.OpenShellGatewayServiceAccountCreateRequest
	createErr        error
	revokes, deletes int
	deleted          bool
}

func (f *fakeSAAPI) Create(_ context.Context, gatewayID string, request *sdktypes.OpenShellGatewayServiceAccountCreateRequest) (*sdktypes.OpenShellGatewayServiceAccountCreateResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.creates = append(f.creates, request)
	if f.createErr != nil {
		return nil, f.createErr
	}
	account := &sdktypes.OpenShellGatewayServiceAccountCreateResponse{
		ID: testAccountID, ClientID: testClientID, GatewayID: gatewayID, Subject: testSubject,
		Role:   sdktypes.OpenShellGatewayServiceAccountRoleOpenshellAdmin,
		Status: sdktypes.OpenShellGatewayServiceAccountStatusReady,
		Credential: sdktypes.OpenShellGatewayServiceAccountCredential{
			Issuer:        f.issuer,
			TokenEndpoint: f.issuer + "/protocol/openid-connect/token",
			Audience:      testGateway.Name + "-" + testGateway.ID,
			GrantType:     "client_credentials",
			ClientID:      testClientID,
			ClientSecret:  testSecret,
		},
	}
	if f.mutate != nil {
		f.mutate(account)
	}
	return account, nil
}

func (f *fakeSAAPI) Get(context.Context, string, string) (*sdktypes.OpenShellGatewayServiceAccountGetResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.deleted {
		return nil, &sdktypes.APIError{StatusCode: http.StatusNotFound}
	}
	return &sdktypes.OpenShellGatewayServiceAccountGetResponse{ID: testAccountID}, nil
}

func (f *fakeSAAPI) Revoke(context.Context, string, string) (*sdktypes.OpenShellGatewayServiceAccountListItem, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.revokes++
	if f.onRevoke != nil {
		f.onRevoke()
	}
	return &sdktypes.OpenShellGatewayServiceAccountListItem{ID: testAccountID}, nil
}

func (f *fakeSAAPI) Delete(context.Context, string, string) (*sdktypes.OpenShellGatewayServiceAccountListItem, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deletes++
	f.deleted = true
	return nil, nil
}

// fakeTokenEndpoint is an HTTPS Keycloak token endpoint for the service
// account's client_credentials grant.
type fakeTokenEndpoint struct {
	server *httptest.Server
	issuer string

	mu       sync.Mutex
	forms    []url.Values
	revoked  bool
	status   int // when set, every request is refused with this status and oauth
	oauth    string
	lifetime time.Duration
	claims   func(map[string]any) // edits the claims before signing
	minted   int
}

func newFakeTokenEndpoint(t *testing.T) *fakeTokenEndpoint {
	t.Helper()
	f := &fakeTokenEndpoint{lifetime: 300 * time.Second}
	f.server = httptest.NewTLSServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.server.Close)
	f.issuer = f.server.URL + "/realms/hypershell"
	return f
}

func (f *fakeTokenEndpoint) serve(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	f.mu.Lock()
	defer f.mu.Unlock()
	f.forms = append(f.forms, r.PostForm)
	w.Header().Set("Content-Type", "application/json")
	if f.status != 0 {
		w.WriteHeader(f.status)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": f.oauth})
		return
	}
	if f.revoked || r.PostForm.Get("client_secret") != testSecret || r.PostForm.Get("client_id") != testClientID {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid_client"})
		return
	}
	f.minted++
	claims := map[string]any{
		"iss": f.issuer, "sub": testSubject, "aud": testGateway.Name + "-" + testGateway.ID,
		"jti":        fmt.Sprintf("token-%d", f.minted),
		"exp":        time.Now().Add(f.lifetime).Unix(),
		"hypershell": map[string]any{"roles": []string{"openshell-admin", "openshell-user"}},
	}
	if f.claims != nil {
		f.claims(claims)
	}
	payload, _ := json.Marshal(claims)
	token := "eyJhbGciOiJub25lIn0." + base64.RawURLEncoding.EncodeToString(payload) + "."
	_ = json.NewEncoder(w).Encode(map[string]string{"access_token": token})
}

func (f *fakeTokenEndpoint) revoke() {
	f.mu.Lock()
	f.revoked = true
	f.mu.Unlock()
}

func (f *fakeTokenEndpoint) requests() []url.Values {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]url.Values(nil), f.forms...)
}

// newTestStore returns a store that trusts the fake endpoint's certificate and
// polls quickly, and a fake API issuing credentials for that endpoint.
func newTestStore(t *testing.T, endpoint *fakeTokenEndpoint) (*gatewayServiceAccountStore, *fakeSAAPI) {
	t.Helper()
	store := newGatewayServiceAccountStore(endpoint.issuer, "run-1")
	store.request = func(ctx context.Context, c sdktypes.OpenShellGatewayServiceAccountCredential) (tokenEndpointResult, error) {
		return postClientCredentials(ctx, endpoint.server.Client(), c)
	}
	store.interval, store.timeout = 5*time.Millisecond, 300*time.Millisecond
	api := &fakeSAAPI{issuer: endpoint.issuer, onRevoke: endpoint.revoke}
	t.Cleanup(func() { _ = store.Close(context.Background(), api) })
	return store, api
}

// Verifies: E2E-CORE-02.08
func TestGatewayServiceAccountTokenWithoutExchange(t *testing.T) {
	endpoint := newFakeTokenEndpoint(t)
	store, api := newTestStore(t, endpoint)

	cred, err := store.Acquire(t.Context(), api, testGateway)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if cred.ClientID != testClientID || cred.AccessToken == "" {
		t.Errorf("credential = %+v", cred)
	}
	create := api.creates[0]
	if create.Role != sdktypes.OpenShellGatewayServiceAccountRoleOpenshellAdmin || create.CredentialType != "client_secret" ||
		create.ExpiresAt == nil || time.Until(*create.ExpiresAt) < gatewayServiceAccountLifetime-time.Minute {
		t.Errorf("create request = %+v", create)
	}
	// The only token request is the account's own plain client_credentials grant.
	forms := endpoint.requests()
	if len(forms) != 1 || forms[0].Get("grant_type") != "client_credentials" {
		t.Fatalf("token requests = %v", forms)
	}
	for _, field := range []string{"subject_token", "requested_subject", "audience", "username", "password"} {
		if _, present := forms[0][field]; present {
			t.Errorf("token request carries %q: not a plain client_credentials grant", field)
		}
	}
}

func TestGatewayServiceAccountReusesThenRenewsToken(t *testing.T) {
	endpoint := newFakeTokenEndpoint(t)
	store, api := newTestStore(t, endpoint)
	acquire := func() gatewayCredential {
		t.Helper()
		cred, err := store.Acquire(t.Context(), api, testGateway)
		if err != nil {
			t.Fatalf("acquire: %v", err)
		}
		return cred
	}

	first := acquire()
	if second := acquire(); second != first {
		t.Error("a token with plenty of lifetime was replaced")
	}

	// Put the cached token inside the renewal margin: the next use replaces it.
	store.mu.Lock()
	e := store.entries[testGateway.ID]
	store.mu.Unlock()
	e.mu.Lock()
	e.expiresAt = time.Now().Add(gatewayTokenRenewMargin / 2)
	e.mu.Unlock()
	if renewed := acquire(); renewed.AccessToken == first.AccessToken {
		t.Error("a token within the renewal margin was reused")
	}
	if len(api.creates) != 1 {
		t.Errorf("service account creates = %d, want 1: renewal must not create another account", len(api.creates))
	}
}

func TestGatewayServiceAccountRejectsUnexpectedTokenClaims(t *testing.T) {
	for name, edit := range map[string]func(map[string]any){
		"wrong audience": func(c map[string]any) { c["aud"] = "another-gateway-client" },
		"wrong subject":  func(c map[string]any) { c["sub"] = "someone-else" },
		"missing role":   func(c map[string]any) { c["hypershell"] = map[string]any{"roles": []string{"openshell-user"}} },
		"extra role": func(c map[string]any) {
			c["hypershell"] = map[string]any{"roles": []string{"openshell-admin", "openshell-user", "platform:admin"}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			endpoint := newFakeTokenEndpoint(t)
			endpoint.claims = edit
			store, api := newTestStore(t, endpoint)
			_, err := store.Acquire(t.Context(), api, testGateway)
			if err == nil || !strings.Contains(err.Error(), "claims") {
				t.Fatalf("error = %v, want a claims failure", err)
			}
			if strings.Contains(err.Error(), testSecret) {
				t.Errorf("error leaks the client secret: %v", err)
			}
		})
	}
}

// Verifies: E2E-CORE-40.05
func TestGatewayServiceAccountSecretGoesOnlyToExpectedEndpoint(t *testing.T) {
	for name, tt := range map[string]struct {
		mutate func(*sdktypes.OpenShellGatewayServiceAccountCreateResponse)
		want   string
	}{
		"foreign endpoint": {want: "token endpoint", mutate: func(a *sdktypes.OpenShellGatewayServiceAccountCreateResponse) {
			a.Credential.TokenEndpoint = "https://collector.example/realms/hypershell/protocol/openid-connect/token"
		}},
		"plain http": {want: "token endpoint", mutate: func(a *sdktypes.OpenShellGatewayServiceAccountCreateResponse) {
			a.Credential.TokenEndpoint = strings.Replace(a.Credential.TokenEndpoint, "https://", "http://", 1)
		}},
		"another gateway's audience": {want: "audience", mutate: func(a *sdktypes.OpenShellGatewayServiceAccountCreateResponse) {
			a.Credential.Audience = "other-gw-other-id"
		}},
		"wrong role": {want: "openshell-admin", mutate: func(a *sdktypes.OpenShellGatewayServiceAccountCreateResponse) {
			a.Role = sdktypes.OpenShellGatewayServiceAccountRoleOpenshellUser
		}},
	} {
		t.Run(name, func(t *testing.T) {
			endpoint := newFakeTokenEndpoint(t)
			store, api := newTestStore(t, endpoint)
			api.mutate = tt.mutate

			_, err := store.Acquire(t.Context(), api, testGateway)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want one naming %q", err, tt.want)
			}
			// Neither use nor teardown may send the secret to an unexpected place.
			_ = store.Close(t.Context(), api)
			if got := len(endpoint.requests()); got != 0 {
				t.Errorf("token endpoint received %d requests; the secret must not be sent", got)
			}
			if !api.deleted {
				t.Error("rejected account was not deleted")
			}
		})
	}
}

func TestGatewayServiceAccountDoesNotFollowRedirects(t *testing.T) {
	var mu sync.Mutex
	var leaked bool
	collector := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		mu.Lock()
		leaked = leaked || r.PostForm.Get("client_secret") != ""
		mu.Unlock()
		_, _ = w.Write([]byte(`{"access_token":"collected"}`))
	}))
	defer collector.Close()
	redirector := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, collector.URL, http.StatusTemporaryRedirect)
	}))
	defer redirector.Close()
	credential := sdktypes.OpenShellGatewayServiceAccountCredential{
		TokenEndpoint: redirector.URL + "/token", GrantType: "client_credentials", ClientID: testClientID, ClientSecret: testSecret,
	}

	// The store's requester dials through http.DefaultTransport, which the suite
	// points at the cluster CA; trust the test servers the same way.
	original := http.DefaultTransport
	http.DefaultTransport = redirector.Client().Transport
	defer func() { http.DefaultTransport = original }()

	result, err := newGatewayServiceAccountStore(redirector.URL, "run-1").request(t.Context(), credential)
	if err != nil || result.Status != http.StatusTemporaryRedirect {
		t.Fatalf("request = %+v, %v; want the redirect itself", result, err)
	}
	mu.Lock()
	defer mu.Unlock()
	if leaked {
		t.Fatal("client secret was re-sent to the redirect target")
	}
}

// Verifies: E2E-CORE-40.03
func TestGatewayServiceAccountRetireVerifiesRevocationThenDeletes(t *testing.T) {
	endpoint := newFakeTokenEndpoint(t)
	store, api := newTestStore(t, endpoint)
	if _, err := store.Acquire(t.Context(), api, testGateway); err != nil {
		t.Fatal(err)
	}
	if err := store.Retire(t.Context(), api, testGateway.ID); err != nil {
		t.Fatalf("retire: %v", err)
	}
	if api.revokes != 1 || api.deletes != 1 || len(endpoint.requests()) < 2 {
		t.Errorf("revokes=%d deletes=%d token requests=%d, want 1, 1, and a post-revocation probe",
			api.revokes, api.deletes, len(endpoint.requests()))
	}
	if _, err := store.Acquire(t.Context(), api, testGateway); err == nil || !strings.Contains(err.Error(), "retired") {
		t.Errorf("acquire after retire = %v, want a retired error", err)
	}
	if err := store.Close(t.Context(), api); err != nil || api.deletes != 1 {
		t.Errorf("close after retire: err=%v deletes=%d, want nothing more to do", err, api.deletes)
	}
}

// Verifies: E2E-CORE-40.03
func TestGatewayServiceAccountRetireFailsButDeletesWhenIssuanceContinues(t *testing.T) {
	endpoint := newFakeTokenEndpoint(t)
	store, api := newTestStore(t, endpoint)
	api.onRevoke = func() {} // the endpoint keeps issuing tokens
	if _, err := store.Acquire(t.Context(), api, testGateway); err != nil {
		t.Fatal(err)
	}
	err := store.Retire(t.Context(), api, testGateway.ID)
	if err == nil || !strings.Contains(err.Error(), "still issues tokens") {
		t.Fatalf("retire error = %v, want a still-issuing failure", err)
	}
	if !api.deleted {
		t.Error("account was not deleted after the revocation check failed")
	}
}

// Only an OAuth client rejection proves revocation; other refusals must not pass.
// Verifies: E2E-CORE-40.03
func TestGatewayServiceAccountRevocationNeedsAnOAuthClientRejection(t *testing.T) {
	for name, refusal := range map[string]struct {
		status int
		oauth  string
	}{
		"server error":       {http.StatusServiceUnavailable, "temporarily_unavailable"},
		"other client error": {http.StatusBadRequest, "invalid_grant"},
	} {
		t.Run(name, func(t *testing.T) {
			endpoint := newFakeTokenEndpoint(t)
			store, api := newTestStore(t, endpoint)
			api.onRevoke = func() {
				endpoint.mu.Lock()
				endpoint.status, endpoint.oauth = refusal.status, refusal.oauth
				endpoint.mu.Unlock()
			}
			if _, err := store.Acquire(t.Context(), api, testGateway); err != nil {
				t.Fatal(err)
			}
			err := store.Retire(t.Context(), api, testGateway.ID)
			if err == nil || !strings.Contains(err.Error(), refusal.oauth) {
				t.Fatalf("retire error = %v, want a failure naming %q", err, refusal.oauth)
			}
			if !api.deleted {
				t.Error("account was not deleted after the revocation check failed")
			}
		})
	}
}

func TestGatewayServiceAccountCreateFailureIsNotRetried(t *testing.T) {
	endpoint := newFakeTokenEndpoint(t)
	store, api := newTestStore(t, endpoint)
	api.createErr = &sdktypes.APIError{StatusCode: http.StatusForbidden, Code: "forbidden"}
	for range 2 {
		if _, err := store.Acquire(t.Context(), api, testGateway); err == nil {
			t.Fatal("acquire succeeded although the account could not be created")
		}
	}
	if len(api.creates) != 1 {
		t.Errorf("create calls = %d, want 1: a credential-creating POST is never retried", len(api.creates))
	}
}

// flakyOnceSAAPI fails the first failFirst Create calls with a 409
// gateway_not_ready before delegating to the wrapped fake, simulating the
// control-plane race where Status lags Phase after a gateway reaches Running.
type flakyOnceSAAPI struct {
	*fakeSAAPI
	failFirst int
	calls     int
}

func (f *flakyOnceSAAPI) Create(ctx context.Context, gatewayID string, request *sdktypes.OpenShellGatewayServiceAccountCreateRequest) (*sdktypes.OpenShellGatewayServiceAccountCreateResponse, error) {
	f.calls++
	if f.calls <= f.failFirst {
		return nil, &sdktypes.APIError{StatusCode: http.StatusConflict, Code: "gateway_not_ready"}
	}
	return f.fakeSAAPI.Create(ctx, gatewayID, request)
}

// TestGatewayServiceAccountCreateConflictIsRetried proves a 409 gateway_not_ready
// does not poison the entry the way TestGatewayServiceAccountCreateFailureIsNotRetried
// shows other failures do: the server rejected the request before creating
// anything, so a later Acquire call must retry the Create rather than replaying a
// cached error.
func TestGatewayServiceAccountCreateConflictIsRetried(t *testing.T) {
	endpoint := newFakeTokenEndpoint(t)
	store, api := newTestStore(t, endpoint)
	flaky := &flakyOnceSAAPI{fakeSAAPI: api, failFirst: 1}

	_, err := store.Acquire(t.Context(), flaky, testGateway)
	if err == nil {
		t.Fatal("first acquire succeeded despite gateway_not_ready")
	}
	if status := apiStatus(err); status != http.StatusConflict {
		t.Fatalf("first acquire error status = %d, want 409 (gateway_not_ready): %v", status, err)
	}

	if _, err := store.Acquire(t.Context(), flaky, testGateway); err != nil {
		t.Fatalf("second acquire (after gateway_not_ready clears) = %v, want success", err)
	}
	if flaky.calls != 2 {
		t.Errorf("create calls = %d, want 2: a 409 must be retried, unlike other failures", flaky.calls)
	}
}

// recordingDriver records gateway-token requests; any other driver call reaches
// the nil embedded interface and fails the test.
type recordingDriver struct {
	driver.E2EInfraDriver
	calls []string
}

func (d *recordingDriver) AcquireGatewayTokenWithRole(_ context.Context, user driver.Credentials, clientID, role string) (driver.Token, error) {
	d.calls = append(d.calls, user.Username+"|"+clientID+"|"+role)
	return driver.Token{AccessToken: "identity-token"}, nil
}

func TestGatewayTokenIdentityModeUsesDriverUnchanged(t *testing.T) {
	t.Setenv("E2E_OIDC_USERNAME", "admin")
	d := &recordingDriver{}
	s := &E2ESuite{driver: d, mode: modeLong}
	s.SetT(t)

	tok, err := s.gatewayToken(t.Context(), testGateway)
	if err != nil || tok.AccessToken != "identity-token" {
		t.Fatalf("gatewayToken = %+v, %v", tok, err)
	}
	if want := "admin|e2e-gw-1-gw-id|openshell-admin"; len(d.calls) != 1 || d.calls[0] != want {
		t.Errorf("driver calls = %v, want [%s]", d.calls, want)
	}
	if s.usesGatewayServiceAccount() {
		t.Error("identity mode reports service accounts in use")
	}
	if err := s.renewGatewayCLIToken(t.Context(), "gw-ns-openshell"); err != nil {
		t.Errorf("CLI renewal in identity mode: %v", err)
	}
}

// newManagementClient serves the create endpoint from fake over HTTP and returns
// an SDK client for it, so the suite's wiring runs against the real client code.
func newManagementClient(t *testing.T, fake *fakeSAAPI) *apiclient.Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer management-token" ||
			r.Method != http.MethodPost || r.URL.Path != "/api/hypershell/v1/gateways/"+testGateway.ID+"/service_accounts" {
			http.NotFound(w, r)
			return
		}
		var request sdktypes.OpenShellGatewayServiceAccountCreateRequest
		_ = json.NewDecoder(r.Body).Decode(&request)
		account, _ := fake.Create(r.Context(), testGateway.ID, &request)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(account)
	}))
	t.Cleanup(server.Close)
	client, err := apiclient.New(server.URL, "management-token", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	return client
}

// TestGatewayServiceAccountModeEndToEnd runs the suite's own wiring: token via
// the SDK-created account with no driver call, the CLI token file, and teardown
// under E2E_SKIP_CLEANUP=1.
// Verifies: E2E-CORE-40.04
func TestGatewayServiceAccountModeEndToEnd(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("E2E_SKIP_CLEANUP", "1")

	endpoint := newFakeTokenEndpoint(t)
	t.Setenv("E2E_OIDC_ISSUER", endpoint.issuer)
	store, api := newTestStore(t, endpoint)
	d := &recordingDriver{}
	s := &E2ESuite{driver: d, admin: newManagementClient(t, api), mode: modeShort, serviceAccounts: store}
	s.SetT(t)

	tok, err := s.gatewayToken(t.Context(), testGateway)
	if err != nil || tok.AccessToken == "" {
		t.Fatalf("gatewayToken = %+v, %v", tok, err)
	}
	if len(d.calls) != 0 {
		t.Errorf("driver was asked for gateway tokens: %v", d.calls)
	}

	local := cliLocalName(testGateway.Namespace)
	if err := s.renewGatewayCLIToken(t.Context(), local); err != nil {
		t.Fatalf("renew CLI token: %v", err)
	}
	tokenPath := filepath.Join(home, ".config", "openshell", "gateways", local, "oidc_token.json")
	info, err := os.Stat(tokenPath)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("CLI token file: %v mode=%v, want 0600", err, info)
	}
	var written cliOIDCToken
	data, _ := os.ReadFile(tokenPath)
	if err := json.Unmarshal(data, &written); err != nil || written.ClientID != testClientID ||
		written.Issuer != endpoint.issuer || written.ExpiresAt == 0 {
		t.Errorf("CLI token = %+v, %v", written, err)
	}

	// The credential file and directory are private while in use.
	store.mu.Lock()
	dir, entry := store.dir, store.entries[testGateway.ID]
	store.mu.Unlock()
	if fi, err := os.Stat(entry.path); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("credential file: %v mode=%v, want 0600", err, fi)
	}
	if di, err := os.Stat(dir); err != nil || di.Mode().Perm() != 0o700 {
		t.Errorf("credential directory: %v mode=%v, want 0700", err, di)
	}

	// E2E_SKIP_CLEANUP=1 keeps gateways, never credentials.
	s.cleanupGatewayServiceAccounts(t.Context(), api)
	for _, path := range []string{dir, tokenPath} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("%s survived cleanup with E2E_SKIP_CLEANUP=1: %v", path, err)
		}
	}
	if api.revokes != 1 || api.deletes != 1 {
		t.Errorf("revokes=%d deletes=%d, want the account revoked and deleted", api.revokes, api.deletes)
	}
}

func TestCLIOIDCTokenOmitsExpiryWhenUnset(t *testing.T) {
	data, err := json.Marshal(cliOIDCToken{AccessToken: "t", Issuer: "i", ClientID: "c"})
	if err != nil || strings.Contains(string(data), "expires_at") {
		t.Errorf("identity-mode token file = %s, %v; must not gain expires_at", data, err)
	}
}
