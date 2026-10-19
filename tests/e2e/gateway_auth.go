package e2e

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	sdktypes "github.com/openshift-online/hypershell/components/sdk-go/types"
	"github.com/openshift-online/hypershell/tests/e2e/driver"
	"github.com/openshift-online/hypershell/tests/e2e/harness"
)

// E2E_GATEWAY_AUTH selects how the suite authenticates to gateways it creates.
// identity (the default) uses the driver's per-gateway token, which is a Keycloak
// token exchange when E2E_OIDC_GRANT=client_credentials. service_account (short
// mode only) creates a short-lived gateway service account through the public
// API as the gateway owner and uses its client_credentials token, so the OIDC
// client needs no exchange rights, impersonation, platform:admin, or Keycloak
// admin access. It is the Go form of the retired Bash suite's mode of that name.
const (
	envGatewayAuth            = "E2E_GATEWAY_AUTH"
	gatewayAuthIdentity       = "identity"
	gatewayAuthServiceAccount = "service_account"

	// A leaked credential is usable for at most this long if teardown never runs.
	gatewayServiceAccountLifetime = 2 * time.Hour
	// Access tokens last about five minutes; replace one before it can expire in
	// the middle of a CLI operation.
	gatewayTokenRenewMargin = time.Minute
)

// resolveGatewayAuth validates the E2E_GATEWAY_AUTH value for the run mode. An
// empty value selects identity.
func resolveGatewayAuth(value, mode string) (string, error) {
	switch value {
	case "", gatewayAuthIdentity:
		return gatewayAuthIdentity, nil
	case gatewayAuthServiceAccount:
		if mode != modeShort {
			return "", fmt.Errorf("%s=%s requires E2E_MODE=%s (E2E_MODE is %q)", envGatewayAuth, value, modeShort, mode)
		}
		return value, nil
	default:
		return "", fmt.Errorf("%s must be %s or %s (got %q)", envGatewayAuth, gatewayAuthIdentity, gatewayAuthServiceAccount, value)
	}
}

// gatewayOIDCIssuer is the issuer gateway credentials must name, without a
// trailing slash.
func gatewayOIDCIssuer() string {
	return strings.TrimSuffix(envOrDefault("E2E_OIDC_ISSUER", "https://keycloak.hypershell.localhost/realms/hypershell"), "/")
}

// gatewayServiceAccountClient is the part of the SDK's service-account API the
// store uses; tests substitute a fake.
type gatewayServiceAccountClient interface {
	Create(ctx context.Context, gatewayID string, request *sdktypes.OpenShellGatewayServiceAccountCreateRequest) (*sdktypes.OpenShellGatewayServiceAccountCreateResponse, error)
	Get(ctx context.Context, gatewayID, serviceAccountID string) (*sdktypes.OpenShellGatewayServiceAccountGetResponse, error)
	Revoke(ctx context.Context, gatewayID, serviceAccountID string) (*sdktypes.OpenShellGatewayServiceAccountListItem, error)
	Delete(ctx context.Context, gatewayID, serviceAccountID string) (*sdktypes.OpenShellGatewayServiceAccountListItem, error)
}

// gatewayCredential is a gateway service account's current access token.
type gatewayCredential struct {
	AccessToken string
	ClientID    string
	ExpiresAt   time.Time
}

// gatewayServiceAccountStore owns the gateway service accounts a short run
// creates. Each account's secret lives in a 0600 file in a 0700 run-private
// directory and is read only to mint a token. Access tokens are cached in memory
// and renewed before they expire.
type gatewayServiceAccountStore struct {
	issuer string
	runID  string

	// request posts one client_credentials grant. The default never follows a
	// redirect, so the secret goes only to the credential's token endpoint.
	request func(context.Context, sdktypes.OpenShellGatewayServiceAccountCredential) (tokenEndpointResult, error)
	// interval and timeout bound the revocation and deletion polls.
	interval, timeout time.Duration

	mu      sync.Mutex
	dir     string
	files   int
	order   []string
	entries map[string]*gatewayServiceAccountEntry
}

type gatewayServiceAccountEntry struct {
	mu  sync.Mutex
	ref driver.GatewayRef

	attempted bool // create was tried; createErr is its outcome
	createErr error
	path      string
	accountID string
	clientID  string
	revoked   bool
	retired   bool

	token     string
	expiresAt time.Time
}

func newGatewayServiceAccountStore(issuer, runID string) *gatewayServiceAccountStore {
	client := &http.Client{
		Timeout:       2 * time.Minute,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	return &gatewayServiceAccountStore{
		issuer:   strings.TrimSuffix(issuer, "/"),
		runID:    runID,
		interval: 2 * time.Second,
		timeout:  90 * time.Second,
		entries:  map[string]*gatewayServiceAccountEntry{},
		request: func(ctx context.Context, c sdktypes.OpenShellGatewayServiceAccountCredential) (tokenEndpointResult, error) {
			return postClientCredentials(ctx, client, c)
		},
	}
}

// Acquire returns the gateway's current access token, creating its service
// account on first use and minting a new token when the cached one is within
// the renewal margin of expiry.
func (st *gatewayServiceAccountStore) Acquire(ctx context.Context, api gatewayServiceAccountClient, ref driver.GatewayRef) (gatewayCredential, error) {
	e := st.entry(ref)
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := st.ensureAccount(ctx, api, e); err != nil {
		return gatewayCredential{}, fmt.Errorf("gateway %s service account: %w", ref.ID, err)
	}
	if e.retired {
		return gatewayCredential{}, fmt.Errorf("gateway %s service account was already retired", ref.ID)
	}
	if e.token == "" || !time.Now().Add(gatewayTokenRenewMargin).Before(e.expiresAt) {
		if err := st.mint(ctx, e); err != nil {
			return gatewayCredential{}, fmt.Errorf("gateway %s service-account token: %w", ref.ID, err)
		}
	}
	return gatewayCredential{AccessToken: e.token, ClientID: e.clientID, ExpiresAt: e.expiresAt}, nil
}

// Gateways returns the gateways that have, or tried to create, a service account.
func (st *gatewayServiceAccountStore) Gateways() []driver.GatewayRef {
	st.mu.Lock()
	defer st.mu.Unlock()
	refs := make([]driver.GatewayRef, 0, len(st.order))
	for _, id := range st.order {
		refs = append(refs, st.entries[id].ref)
	}
	return refs
}

// Retire revokes the gateway's service account, requires the token endpoint to
// reject it, and deletes it. The delete runs even when the check fails, and both
// failures are reported. A gateway with no account, or one already retired, is a
// no-op.
func (st *gatewayServiceAccountStore) Retire(ctx context.Context, api gatewayServiceAccountClient, gatewayID string) error {
	st.mu.Lock()
	e := st.entries[gatewayID]
	st.mu.Unlock()
	if e == nil {
		return nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return st.retire(ctx, api, e)
}

// Close retires every account still live and removes the credential directory.
// The directory goes on every path, including a failed retire and
// E2E_SKIP_CLEANUP=1. api may be nil when no management client could be built;
// the accounts then expire on their own and the error says so.
func (st *gatewayServiceAccountStore) Close(ctx context.Context, api gatewayServiceAccountClient) error {
	var errs []error
	for _, ref := range st.Gateways() {
		st.mu.Lock()
		e := st.entries[ref.ID]
		st.mu.Unlock()
		e.mu.Lock()
		switch {
		case e.retired || e.accountID == "":
		case api == nil:
			errs = append(errs, fmt.Errorf("gateway %s service account %s was not revoked (no management client); it expires within %s",
				ref.ID, e.accountID, gatewayServiceAccountLifetime))
		default:
			if err := st.retire(ctx, api, e); err != nil {
				errs = append(errs, err)
			}
		}
		e.mu.Unlock()
	}
	st.mu.Lock()
	dir := st.dir
	st.dir = ""
	st.mu.Unlock()
	if dir != "" {
		if err := os.RemoveAll(dir); err != nil {
			errs = append(errs, fmt.Errorf("remove gateway service-account credential directory: %w", err))
		}
	}
	return errors.Join(errs...)
}

func (st *gatewayServiceAccountStore) entry(ref driver.GatewayRef) *gatewayServiceAccountEntry {
	st.mu.Lock()
	defer st.mu.Unlock()
	if e, ok := st.entries[ref.ID]; ok {
		return e
	}
	e := &gatewayServiceAccountEntry{ref: ref}
	st.entries[ref.ID] = e
	st.order = append(st.order, ref.ID)
	return e
}

// ensureAccount creates the account once. A failed create is remembered rather
// than posted again: the request may have succeeded, and a retry would orphan an
// account. The one exception is a 409 gateway_not_ready: the server rejects that
// request outright before creating anything (the gateway's Status has not yet
// caught up to its Phase, a known control-plane health-reconciler race - see
// requireGatewayToken), so it carries no orphan risk and is left retryable for
// the caller.
func (st *gatewayServiceAccountStore) ensureAccount(ctx context.Context, api gatewayServiceAccountClient, e *gatewayServiceAccountEntry) error {
	if e.attempted {
		return e.createErr
	}
	if api == nil {
		return errors.New("no management API client to create the account")
	}
	expires := time.Now().UTC().Add(gatewayServiceAccountLifetime)
	account, err := api.Create(ctx, e.ref.ID, &sdktypes.OpenShellGatewayServiceAccountCreateRequest{
		Name:           "e2e-short-" + st.runID,
		Role:           sdktypes.OpenShellGatewayServiceAccountRoleOpenshellAdmin,
		CredentialType: "client_secret",
		ExpiresAt:      &expires,
	})
	if err != nil {
		wrapped := fmt.Errorf("create: %w", err)
		if apiStatus(err) == http.StatusConflict {
			return wrapped
		}
		e.attempted = true
		e.createErr = wrapped
		return e.createErr
	}
	e.attempted = true
	// Record the account before checking it, so teardown can delete a partly
	// successful setup.
	e.accountID, e.clientID = account.ID, account.ClientID
	if e.path, err = st.credentialPath(); err == nil {
		err = writeCredentialFile(e.path, account)
	}
	if err == nil {
		err = validateGatewayServiceAccountCredential(account, e.ref, st.issuer)
	}
	e.createErr = err
	return err
}

// mint requests a token and checks it. Transport errors and server errors get a
// few attempts; a client-side refusal is final.
func (st *gatewayServiceAccountStore) mint(ctx context.Context, e *gatewayServiceAccountEntry) error {
	account, err := readCredentialFile(e.path)
	if err != nil {
		return err
	}
	// Check the destination again every time the secret is about to be sent.
	if err := validateGatewayServiceAccountCredential(account, e.ref, st.issuer); err != nil {
		return err
	}
	var result tokenEndpointResult
	err = harness.Retry(ctx, 3, st.interval, func() (err error) {
		if result, err = st.request(ctx, account.Credential); err == nil && result.Status >= http.StatusInternalServerError {
			err = fmt.Errorf("token endpoint returned status %d", result.Status)
		}
		return err
	})
	if err != nil {
		return err
	}
	if result.Status != http.StatusOK {
		return fmt.Errorf("token endpoint refused the credential (status %d, error %q)", result.Status, result.OAuthError)
	}
	claims, err := decodeJWTClaims(result.AccessToken)
	if err != nil {
		return err
	}
	if err := validateGatewayServiceAccountClaims(claims, account); err != nil {
		return fmt.Errorf("token claims: %w", err)
	}
	exp, _ := claims["exp"].(float64) // validated above
	e.token, e.expiresAt = result.AccessToken, time.Unix(int64(exp), 0)
	return nil
}

func (st *gatewayServiceAccountStore) retire(ctx context.Context, api gatewayServiceAccountClient, e *gatewayServiceAccountEntry) error {
	if e.retired || e.accountID == "" {
		return nil
	}
	var errs []error
	if !e.revoked {
		_, err := api.Revoke(ctx, e.ref.ID, e.accountID)
		switch {
		case apiStatus(err) == http.StatusNotFound: // already gone
			e.finish()
			return nil
		case err != nil:
			// Still try to delete below: deleting the account removes its credential.
			errs = append(errs, fmt.Errorf("revoke gateway %s service account: %w", e.ref.ID, err))
		default:
			e.revoked = true
		}
	}
	if e.revoked {
		if err := st.verifyRevoked(ctx, e); err != nil {
			errs = append(errs, err)
		}
	}
	if err := st.deleteAccount(ctx, api, e); err != nil {
		return errors.Join(append(errs, err)...)
	}
	e.finish()
	return errors.Join(errs...)
}

func (e *gatewayServiceAccountEntry) finish() {
	e.retired, e.token = true, ""
	if e.path != "" {
		_ = os.Remove(e.path)
	}
}

// verifyRevoked polls until the token endpoint rejects the revoked credential.
// Only an OAuth client rejection proves revocation: a transport failure, a server
// error, or any other response fails the check.
func (st *gatewayServiceAccountStore) verifyRevoked(ctx context.Context, e *gatewayServiceAccountEntry) error {
	account, err := readCredentialFile(e.path)
	if err == nil {
		err = validateGatewayServiceAccountCredential(account, e.ref, st.issuer)
	}
	if err != nil {
		return fmt.Errorf("verify gateway %s service-account revocation: %w", e.ref.ID, err)
	}
	err = harness.Poll(ctx, st.interval, st.timeout, func(ctx context.Context) (bool, error) {
		result, err := st.request(ctx, account.Credential)
		switch {
		case err != nil:
			return false, fmt.Errorf("token request after revocation: %w", err)
		case result.Status == http.StatusOK:
			return false, nil // still issuing; revocation has not propagated
		case (result.Status == http.StatusBadRequest || result.Status == http.StatusUnauthorized) &&
			(result.OAuthError == "invalid_client" || result.OAuthError == "unauthorized_client"):
			return true, nil
		default:
			return false, fmt.Errorf("unexpected token endpoint response after revocation (status %d, error %q)", result.Status, result.OAuthError)
		}
	})
	if err != nil {
		return fmt.Errorf("revoked gateway %s service account still issues tokens or could not be verified: %w", e.ref.ID, err)
	}
	return nil
}

// deleteAccount deletes the account and waits for the API to report it gone.
func (st *gatewayServiceAccountStore) deleteAccount(ctx context.Context, api gatewayServiceAccountClient, e *gatewayServiceAccountEntry) error {
	err := harness.Poll(ctx, st.interval, st.timeout, func(ctx context.Context) (bool, error) {
		_, err := api.Delete(ctx, e.ref.ID, e.accountID)
		switch status := apiStatus(err); {
		case err == nil, status == http.StatusNotFound:
			return true, nil
		case status >= http.StatusInternalServerError:
			return false, nil
		default:
			return false, fmt.Errorf("delete gateway %s service account: %w", e.ref.ID, err)
		}
	})
	if err != nil {
		return err
	}
	err = harness.Poll(ctx, st.interval, st.timeout, func(ctx context.Context) (bool, error) {
		_, err := api.Get(ctx, e.ref.ID, e.accountID)
		return apiStatus(err) == http.StatusNotFound, nil
	})
	if err != nil {
		return fmt.Errorf("gateway %s service account was not deleted: %w", e.ref.ID, err)
	}
	return nil
}

// apiStatus returns the HTTP status of an SDK API error, or 0 for other errors.
func apiStatus(err error) int {
	var apiErr *sdktypes.APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode
	}
	return 0
}

// validateGatewayServiceAccountCredential checks, before the secret is used, that
// the created account is the one requested and that its credential sends the
// secret only to the expected issuer's HTTPS token endpoint.
func validateGatewayServiceAccountCredential(account *sdktypes.OpenShellGatewayServiceAccountCreateResponse, ref driver.GatewayRef, issuer string) error {
	c := account.Credential
	switch {
	case !strings.HasPrefix(c.TokenEndpoint, "https://") || c.TokenEndpoint != issuer+"/protocol/openid-connect/token":
		return fmt.Errorf("credential token endpoint is not the expected issuer's HTTPS token endpoint")
	case c.Issuer != issuer:
		return fmt.Errorf("credential issuer %q is not the expected issuer", c.Issuer)
	case c.Audience != fmt.Sprintf("%s-%s", ref.Name, ref.ID):
		return fmt.Errorf("credential audience %q is not the gateway's client", c.Audience)
	case c.GrantType != "client_credentials":
		return fmt.Errorf("credential grant type %q is not client_credentials", c.GrantType)
	case c.ClientID == "" || c.ClientID != account.ClientID || c.ClientSecret == "" || account.ID == "" || account.Subject == "":
		return fmt.Errorf("credential is missing its client id, client secret, account id, or subject")
	case account.GatewayID != ref.ID ||
		account.Role != sdktypes.OpenShellGatewayServiceAccountRoleOpenshellAdmin ||
		account.Status != sdktypes.OpenShellGatewayServiceAccountStatusReady:
		return fmt.Errorf("account is not this gateway's ready openshell-admin account")
	}
	return nil
}

func writeCredentialFile(path string, account *sdktypes.OpenShellGatewayServiceAccountCreateResponse) error {
	data, err := json.Marshal(account)
	if err != nil {
		return fmt.Errorf("encode credential: %w", err)
	}
	return writePrivateFile(filepath.Dir(path), filepath.Base(path), data)
}

func readCredentialFile(path string) (*sdktypes.OpenShellGatewayServiceAccountCreateResponse, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read credential: %w", err)
	}
	var account sdktypes.OpenShellGatewayServiceAccountCreateResponse
	if err := json.Unmarshal(data, &account); err != nil {
		return nil, fmt.Errorf("decode credential: %w", err)
	}
	return &account, nil
}

// credentialPath reserves a file path in the run-private directory, creating the
// directory (mode 0700) on first use.
func (st *gatewayServiceAccountStore) credentialPath() (string, error) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.dir == "" {
		dir, err := os.MkdirTemp("", "e2e-gateway-sa-*")
		if err != nil {
			return "", fmt.Errorf("create credential directory: %w", err)
		}
		st.dir = dir
	}
	st.files++
	return filepath.Join(st.dir, fmt.Sprintf("credential-%d.json", st.files)), nil
}

// writePrivateFile atomically replaces dir/name with data at mode 0600, so a
// concurrent reader sees the old or the new content and never a partial write.
func writePrivateFile(dir, name string, data []byte) (err error) {
	tmp, err := os.CreateTemp(dir, "."+name+"-*") // created 0600
	if err != nil {
		return fmt.Errorf("create private file: %w", err)
	}
	defer func() {
		if err != nil {
			_ = os.Remove(tmp.Name())
		}
	}()
	if _, err = tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write private file: %w", err)
	}
	if err = tmp.Close(); err != nil {
		return fmt.Errorf("close private file: %w", err)
	}
	return os.Rename(tmp.Name(), filepath.Join(dir, name))
}

// --- suite wiring ---

func (s *E2ESuite) usesGatewayServiceAccount() bool { return s.serviceAccounts != nil }

// serviceAccountClient is the management identity's service-account API, or nil
// when the suite has no management client.
func (s *E2ESuite) serviceAccountClient() gatewayServiceAccountClient {
	if s.admin == nil {
		return nil
	}
	return s.admin.OpenShellGatewayServiceAccounts()
}

// gatewayToken returns the per-gateway admin token for ref. Identity mode calls
// the driver exactly as before; service_account mode uses the gateway service
// account's client_credentials token, with no token exchange.
func (s *E2ESuite) gatewayToken(ctx context.Context, ref driver.GatewayRef) (driver.Token, error) {
	if !s.usesGatewayServiceAccount() {
		clientID := fmt.Sprintf("%s-%s", ref.Name, ref.ID)
		return s.driver.AcquireGatewayTokenWithRole(ctx, s.adminCreds(), clientID, "openshell-admin")
	}
	cred, err := s.serviceAccounts.Acquire(ctx, s.serviceAccountClient(), ref)
	return driver.Token{AccessToken: cred.AccessToken}, err
}

// requireGatewayToken calls gatewayToken and fails the test on error. A 409
// gateway_not_ready response means the gateway's Status has not yet caught up
// to its Phase - a known control-plane race (health.go's
// observedGatewayHealthUpdate deliberately preserves a stale Status alongside
// a Phase-only update while a Keycloak-client marker is set, and the fix-up
// that clears that marker runs on the next reconcile tick rather than
// atomically with the Phase flip). Since waitGatewayRunning only waits for
// Phase, a single attempt right after it returns can lose that race, so this
// retries for a bounded window instead of failing immediately. It logs the
// observed phase/status on every retry and dumps full gateway diagnostics
// (status, route, conditions, controller log tail) on the final failure, so a
// run that exhausts the window is debuggable from the CI log alone.
func (s *E2ESuite) requireGatewayToken(t *testing.T, ctx context.Context, ref driver.GatewayRef, msgAndArgs ...any) driver.Token {
	t.Helper()
	var tok driver.Token
	var lastErr error
	var lastGW *sdktypes.Gateway
	timeout := durationSecondsEnv("E2E_GATEWAY_READY_TIMEOUT", 30*time.Second)
	pollErr := harness.Poll(ctx, 3*time.Second, timeout, func(ctx context.Context) (bool, error) {
		var err error
		tok, err = s.gatewayToken(ctx, ref)
		if err == nil {
			return true, nil
		}
		lastErr = err
		lastGW, _ = s.admin.Gateways().Get(ctx, ref.ID)
		if apiStatus(err) != http.StatusConflict {
			return false, err // not the known gateway_not_ready race; fail immediately
		}
		phase, status := "<unknown>", "<unknown>"
		if lastGW != nil {
			phase, status = lastGW.Phase, lastGW.Status
		}
		s.Logf("gateway %s not ready for service-account provisioning yet (phase=%q status=%q); retrying", ref.ID, phase, status)
		return false, nil
	})
	if lastErr != nil {
		s.dumpGatewayDiagnostics(t, ref.ID, lastGW)
		pollErr = lastErr
	}
	s.Require().NoError(pollErr, msgAndArgs...)
	return tok
}

// renewGatewayCLIToken rewrites the CLI's oidc_token.json for local with the
// gateway service account's current token. It runs before every CLI call and is
// a no-op in identity mode and for handles that are not service-account gateways.
func (s *E2ESuite) renewGatewayCLIToken(ctx context.Context, local string) error {
	if !s.usesGatewayServiceAccount() {
		return nil
	}
	for _, ref := range s.serviceAccounts.Gateways() {
		if cliLocalName(ref.Namespace) != local {
			continue
		}
		cred, err := s.serviceAccounts.Acquire(ctx, s.serviceAccountClient(), ref)
		if err != nil {
			return err
		}
		dir, err := cliGatewayConfigDir(local)
		if err != nil {
			return err
		}
		return writeCLIOIDCToken(dir, cliOIDCToken{
			AccessToken: cred.AccessToken,
			Issuer:      gatewayOIDCIssuer(),
			ClientID:    cred.ClientID,
			ExpiresAt:   cred.ExpiresAt.Unix(),
		})
	}
	return nil
}

// retireGatewayServiceAccount runs the revoke, verify, and delete check for ref
// as a named subtest. It does nothing in identity mode, so that report is
// unchanged.
func (s *E2ESuite) retireGatewayServiceAccount(ref driver.GatewayRef) {
	if !s.usesGatewayServiceAccount() {
		return
	}
	s.Run("gateway-service-account-revocation", func() {
		s.runner.Comment("revoke gateway %s service account, require token issuance to stop, then delete it", ref.ID)
		s.Require().NoError(s.serviceAccounts.Retire(s.T().Context(), s.serviceAccountClient(), ref.ID),
			"gateway service account revocation and deletion")
	})
}

// cleanupGatewayServiceAccounts retires every remaining account and removes the
// local credentials: the secret directory and the CLI token files. It runs on
// every exit path, including E2E_SKIP_CLEANUP=1, because a service account is a
// credential and not a resource kept for inspection.
func (s *E2ESuite) cleanupGatewayServiceAccounts(ctx context.Context, client gatewayServiceAccountClient) {
	if s.serviceAccounts == nil {
		return
	}
	if err := s.serviceAccounts.Close(ctx, client); err != nil {
		reportingTest{T: s.T(), logger: s.logger}.Errorf("gateway service-account cleanup: %v", err)
	}
	for _, ref := range s.serviceAccounts.Gateways() {
		dir, err := cliGatewayConfigDir(cliLocalName(ref.Namespace))
		if err != nil {
			reportingTest{T: s.T(), logger: s.logger}.Errorf("remove CLI gateway token: %v", err)
			continue
		}
		if err := os.Remove(filepath.Join(dir, "oidc_token.json")); err != nil && !errors.Is(err, os.ErrNotExist) {
			reportingTest{T: s.T(), logger: s.logger}.Errorf("remove CLI gateway token: %v", err)
		}
	}
}
