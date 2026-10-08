// Package oidc implements the public-client OpenID Connect flows the generated
// CLI needs: discovery, browser login with PKCE on a loopback redirect
// (RFC 7636, RFC 8252), the device authorization grant (RFC 8628), refresh and
// revocation (RFC 7009). It never logs or returns token values in errors.
package oidc

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// ErrSessionExpired is returned when the issuer rejects a refresh token, so
// the user has to log in again.
var ErrSessionExpired = errors.New("session expired, run 'login' again")

// ErrNoRevocationEndpoint is returned by Revoke when the issuer does not
// advertise a revocation endpoint.
var ErrNoRevocationEndpoint = errors.New("the issuer does not advertise a revocation endpoint")

const (
	maxResponseBytes = 1 << 20
	deviceGrantType  = "urn:ietf:params:oauth:grant-type:device_code"
)

// Endpoints is the subset of the discovery document the CLI uses.
type Endpoints struct {
	Issuer                      string `json:"issuer"`
	AuthorizationEndpoint       string `json:"authorization_endpoint"`
	TokenEndpoint               string `json:"token_endpoint"`
	DeviceAuthorizationEndpoint string `json:"device_authorization_endpoint"`
	RevocationEndpoint          string `json:"revocation_endpoint"`
}

// Tokens is a token endpoint response.
type Tokens struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	ExpiresIn    int64  `json:"expires_in"`
}

// ExpiresAt returns the Unix time the access token expires, or 0 when unknown.
func (t *Tokens) ExpiresAt(now time.Time) int64 {
	if t.ExpiresIn <= 0 {
		return 0
	}
	return now.Add(time.Duration(t.ExpiresIn) * time.Second).Unix()
}

// Client talks to one issuer as one public client.
type Client struct {
	Issuer   string
	ClientID string
	HTTP     *http.Client
	// Sleep waits between device-flow polls; tests replace it.
	Sleep func(time.Duration)
	// Now returns the current time; tests replace it.
	Now func() time.Time

	endpoints *Endpoints
}

// NewClient builds a client for an issuer. When insecure is set TLS
// verification is skipped and a plain-HTTP issuer is accepted.
func NewClient(issuer, clientID string, insecure bool) *Client {
	transport := &http.Transport{Proxy: http.ProxyFromEnvironment}
	if insecure {
		transport.TLSClientConfig = &tls.Config{
			MinVersion:         tls.VersionTLS12,
			InsecureSkipVerify: true, //nolint:gosec
		}
	}
	return &Client{
		Issuer:   strings.TrimRight(issuer, "/"),
		ClientID: clientID,
		HTTP:     &http.Client{Transport: transport, Timeout: 30 * time.Second},
		Sleep:    time.Sleep,
		Now:      time.Now,
	}
}

func validateIssuer(issuer string, insecure bool) error {
	parsed, err := url.Parse(issuer)
	if err != nil || parsed.Host == "" {
		return fmt.Errorf("invalid issuer URL %q", issuer)
	}
	if parsed.Scheme == "https" {
		return nil
	}
	if parsed.Scheme == "http" && (insecure || isLoopbackHost(parsed.Hostname())) {
		return nil
	}
	return fmt.Errorf("issuer %q must use https (plain http is accepted only for loopback hosts or with --insecure)", issuer)
}

func isLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// Discover fetches and caches <issuer>/.well-known/openid-configuration.
func (c *Client) Discover(ctx context.Context) (*Endpoints, error) {
	if c.endpoints != nil {
		return c.endpoints, nil
	}
	insecure := false
	if transport, ok := c.HTTP.Transport.(*http.Transport); ok && transport.TLSClientConfig != nil {
		insecure = transport.TLSClientConfig.InsecureSkipVerify
	}
	if err := validateIssuer(c.Issuer, insecure); err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Issuer+"/.well-known/openid-configuration", nil)
	if err != nil {
		return nil, err
	}
	response, err := c.HTTP.Do(request)
	if err != nil {
		return nil, fmt.Errorf("can't reach the issuer: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("can't read the discovery document: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("issuer discovery failed with status %d", response.StatusCode)
	}
	var endpoints Endpoints
	if err := json.Unmarshal(body, &endpoints); err != nil {
		return nil, fmt.Errorf("can't parse the discovery document: %w", err)
	}
	if strings.TrimRight(endpoints.Issuer, "/") != c.Issuer {
		return nil, fmt.Errorf("discovery document issuer %q does not match %q", endpoints.Issuer, c.Issuer)
	}
	if endpoints.TokenEndpoint == "" {
		return nil, errors.New("the discovery document has no token endpoint")
	}
	c.endpoints = &endpoints
	return c.endpoints, nil
}

type tokenError struct {
	Code        string `json:"error"`
	Description string `json:"error_description"`
}

func (e *tokenError) Error() string {
	if e.Description != "" {
		return e.Code + ": " + e.Description
	}
	return e.Code
}

// postForm posts a form and decodes either a JSON result or an OAuth error.
func (c *Client) postForm(ctx context.Context, endpoint string, form url.Values, result any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "application/json")
	response, err := c.HTTP.Do(request)
	if err != nil {
		return fmt.Errorf("request to the issuer failed: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes))
	if err != nil {
		return fmt.Errorf("can't read the issuer response: %w", err)
	}
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		if result == nil || len(body) == 0 {
			return nil
		}
		if err := json.Unmarshal(body, result); err != nil {
			return fmt.Errorf("can't parse the issuer response: %w", err)
		}
		return nil
	}
	var failure tokenError
	if json.Unmarshal(body, &failure) == nil && failure.Code != "" {
		return &failure
	}
	return fmt.Errorf("the issuer returned status %d", response.StatusCode)
}

func (c *Client) exchange(ctx context.Context, form url.Values) (*Tokens, error) {
	endpoints, err := c.Discover(ctx)
	if err != nil {
		return nil, err
	}
	form.Set("client_id", c.ClientID)
	var tokens Tokens
	if err := c.postForm(ctx, endpoints.TokenEndpoint, form, &tokens); err != nil {
		return nil, err
	}
	if tokens.AccessToken == "" {
		return nil, errors.New("the issuer response has no access token")
	}
	return &tokens, nil
}

func randomString(bytes int) (string, error) {
	buffer := make([]byte, bytes)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

// PKCEChallenge returns the S256 code challenge for a verifier.
func PKCEChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// BrowserOptions configures LoginBrowser.
type BrowserOptions struct {
	Scopes []string
	// OpenBrowser, when set, is called with the authorization URL instead of the
	// platform opener. The URL is also printed to Out so the user can open it.
	OpenBrowser func(authorizationURL string) error
	Out         io.Writer
	Timeout     time.Duration
}

// LoginBrowser runs the authorization code flow with PKCE on a loopback redirect.
func (c *Client) LoginBrowser(ctx context.Context, options BrowserOptions) (*Tokens, error) {
	endpoints, err := c.Discover(ctx)
	if err != nil {
		return nil, err
	}
	if endpoints.AuthorizationEndpoint == "" {
		return nil, errors.New("the issuer has no authorization endpoint, try --no-browser")
	}
	verifier, err := randomString(32)
	if err != nil {
		return nil, err
	}
	state, err := randomString(16)
	if err != nil {
		return nil, err
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("can't start the local callback listener: %w", err)
	}
	defer listener.Close()
	redirectURI := fmt.Sprintf("http://127.0.0.1:%d/callback", listener.Addr().(*net.TCPAddr).Port)

	type result struct {
		code string
		err  error
	}
	results := make(chan result, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		var outcome result
		switch {
		case subtle.ConstantTimeCompare([]byte(query.Get("state")), []byte(state)) != 1:
			outcome.err = errors.New("the callback state does not match, login aborted")
		case query.Get("error") != "":
			outcome.err = &tokenError{Code: query.Get("error"), Description: query.Get("error_description")}
		case query.Get("code") == "":
			outcome.err = errors.New("the callback has no authorization code")
		default:
			outcome.code = query.Get("code")
		}
		if outcome.err != nil {
			http.Error(w, "Login failed. You can close this window.", http.StatusBadRequest)
		} else {
			fmt.Fprintln(w, "Login complete. You can close this window and return to the terminal.")
		}
		select {
		case results <- outcome:
		default:
		}
	})
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = server.Serve(listener) }()
	defer server.Close()

	query := url.Values{}
	query.Set("response_type", "code")
	query.Set("client_id", c.ClientID)
	query.Set("redirect_uri", redirectURI)
	query.Set("scope", strings.Join(options.Scopes, " "))
	query.Set("state", state)
	query.Set("code_challenge", PKCEChallenge(verifier))
	query.Set("code_challenge_method", "S256")
	authorizationURL := endpoints.AuthorizationEndpoint
	if strings.Contains(authorizationURL, "?") {
		authorizationURL += "&" + query.Encode()
	} else {
		authorizationURL += "?" + query.Encode()
	}

	if options.Out != nil {
		fmt.Fprintf(options.Out, "Opening your browser to log in. If it does not open, visit:\n  %s\n", authorizationURL)
	}
	open := options.OpenBrowser
	if open == nil {
		open = openInBrowser
	}
	go func() { _ = open(authorizationURL) }()

	timeout := options.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case outcome := <-results:
		if outcome.err != nil {
			return nil, outcome.err
		}
		return c.exchange(ctx, url.Values{
			"grant_type":    {"authorization_code"},
			"code":          {outcome.code},
			"redirect_uri":  {redirectURI},
			"code_verifier": {verifier},
		})
	case <-timer.C:
		return nil, errors.New("timed out waiting for the browser login")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func openInBrowser(target string) error {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		command = exec.Command("open", target)
	case "windows":
		command = exec.Command("rundll32", "url.dll,FileProtocolHandler", target)
	default:
		command = exec.Command("xdg-open", target)
	}
	return command.Start()
}

// LoginDevice runs the device authorization grant, for terminals without a browser.
func (c *Client) LoginDevice(ctx context.Context, scopes []string, out io.Writer) (*Tokens, error) {
	endpoints, err := c.Discover(ctx)
	if err != nil {
		return nil, err
	}
	if endpoints.DeviceAuthorizationEndpoint == "" {
		return nil, errors.New("the issuer does not support the device authorization grant")
	}
	var device struct {
		DeviceCode              string `json:"device_code"`
		UserCode                string `json:"user_code"`
		VerificationURI         string `json:"verification_uri"`
		VerificationURIComplete string `json:"verification_uri_complete"`
		ExpiresIn               int64  `json:"expires_in"`
		Interval                int64  `json:"interval"`
	}
	form := url.Values{"client_id": {c.ClientID}, "scope": {strings.Join(scopes, " ")}}
	if err := c.postForm(ctx, endpoints.DeviceAuthorizationEndpoint, form, &device); err != nil {
		return nil, fmt.Errorf("can't start the device login: %w", err)
	}
	if device.DeviceCode == "" || device.UserCode == "" {
		return nil, errors.New("the issuer returned an incomplete device authorization response")
	}
	target := device.VerificationURIComplete
	if target == "" {
		target = device.VerificationURI
	}
	if out != nil {
		fmt.Fprintf(out, "To log in, open this URL in a browser:\n  %s\nand enter the code: %s\n", target, device.UserCode)
	}
	interval := time.Duration(device.Interval) * time.Second
	if interval <= 0 {
		interval = 5 * time.Second
	}
	lifetime := time.Duration(device.ExpiresIn) * time.Second
	if lifetime <= 0 {
		lifetime = 10 * time.Minute
	}
	deadline := c.Now().Add(lifetime)
	for {
		if c.Now().After(deadline) {
			return nil, errors.New("the device code expired before login completed")
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		c.Sleep(interval)
		tokens, err := c.exchange(ctx, url.Values{
			"grant_type":  {deviceGrantType},
			"device_code": {device.DeviceCode},
		})
		if err == nil {
			return tokens, nil
		}
		var failure *tokenError
		if !errors.As(err, &failure) {
			return nil, err
		}
		switch failure.Code {
		case "authorization_pending":
		case "slow_down":
			interval += 5 * time.Second
		default:
			return nil, fmt.Errorf("device login failed: %w", err)
		}
	}
}

// Refresh exchanges a refresh token for new tokens. A rejected refresh token
// maps to ErrSessionExpired. The returned RefreshToken is empty when the issuer
// does not rotate it, in which case the caller keeps the old one.
func (c *Client) Refresh(ctx context.Context, refreshToken string) (*Tokens, error) {
	tokens, err := c.exchange(ctx, url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
	})
	if err != nil {
		var failure *tokenError
		if errors.As(err, &failure) && (failure.Code == "invalid_grant" || failure.Code == "invalid_token" || failure.Code == "unauthorized_client") {
			return nil, ErrSessionExpired
		}
		return nil, fmt.Errorf("can't refresh the session: %w", err)
	}
	return tokens, nil
}

// Revoke asks the issuer to revoke a refresh token (RFC 7009).
func (c *Client) Revoke(ctx context.Context, refreshToken string) error {
	endpoints, err := c.Discover(ctx)
	if err != nil {
		return err
	}
	if endpoints.RevocationEndpoint == "" {
		return ErrNoRevocationEndpoint
	}
	return c.postForm(ctx, endpoints.RevocationEndpoint, url.Values{
		"token":           {refreshToken},
		"token_type_hint": {"refresh_token"},
		"client_id":       {c.ClientID},
	}, nil)
}
