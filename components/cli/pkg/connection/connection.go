package connection

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/openshift-online/hypershell/components/cli/pkg/config"
	"github.com/openshift-online/hypershell/components/cli/pkg/info"
	"github.com/openshift-online/hypershell/components/cli/pkg/oidc"
)

// HTTPClient is the interface for making HTTP requests.
// Implement this interface to customize HTTP behavior (context support, retries, telemetry, etc.)
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// TokenProvider is the interface for retrieving authentication tokens.
// Implement this interface to customize authentication (OIDC, token refresh, etc.)
type TokenProvider interface {
	GetToken() (string, error)
}

// HTTPClientFactory builds the HTTPClient used by connections created through the builder.
type HTTPClientFactory func(cfg *config.Config) HTTPClient

// TokenProviderFactory builds the TokenProvider used by connections created through the builder.
type TokenProviderFactory func(cfg *config.Config) TokenProvider

var (
	factoriesMu          sync.RWMutex
	httpClientFactory    HTTPClientFactory
	tokenProviderFactory TokenProviderFactory
)

// SetHTTPClientFactory registers a factory that replaces the default HTTP client
// for every connection created through ConnectionBuilder, including those used by
// the generated commands. Call it once at startup, for example from the
// hand-maintained main.go. Passing nil restores the default.
func SetHTTPClientFactory(factory HTTPClientFactory) {
	factoriesMu.Lock()
	defer factoriesMu.Unlock()
	httpClientFactory = factory
}

// SetTokenProviderFactory registers a factory that replaces the default static
// token provider for every connection created through ConnectionBuilder,
// including those used by the generated commands. Call it once at startup, for
// example from the hand-maintained main.go. Passing nil restores the default.
func SetTokenProviderFactory(factory TokenProviderFactory) {
	factoriesMu.Lock()
	defer factoriesMu.Unlock()
	tokenProviderFactory = factory
}

// Connection manages HTTP communication with the API server.
type Connection struct {
	baseURL       string
	httpClient    HTTPClient
	tokenProvider TokenProvider
}

// New creates a connection with a custom HTTP client and token provider.
// This allows injecting custom implementations for testing or enhanced behavior.
func New(httpClient HTTPClient, tokenProvider TokenProvider, baseURL string) *Connection {
	return &Connection{
		baseURL:       strings.TrimRight(baseURL, "/"),
		httpClient:    httpClient,
		tokenProvider: tokenProvider,
	}
}

// Do executes an HTTP request with automatic token injection.
func (c *Connection) Do(method, path string, query url.Values, body io.Reader) (*http.Response, error) {
	token, err := c.tokenProvider.GetToken()
	if err != nil {
		return nil, err
	}

	fullURL := c.baseURL + path
	if query != nil {
		fullURL += "?" + query.Encode()
	}

	req, err := http.NewRequest(method, fullURL, body)
	if err != nil {
		return nil, fmt.Errorf("can't create request: %v", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", "hsctl/"+info.Version)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return c.httpClient.Do(req)
}

// DoContext executes an HTTP request with context support and automatic token injection.
// If the underlying HTTPClient implements DoContext, it will be used for better cancellation support.
func (c *Connection) DoContext(ctx context.Context, method, path string, query url.Values, body io.Reader) (*http.Response, error) {
	token, err := c.tokenProvider.GetToken()
	if err != nil {
		return nil, err
	}

	fullURL := c.baseURL + path
	if query != nil {
		fullURL += "?" + query.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, method, fullURL, body)
	if err != nil {
		return nil, fmt.Errorf("can't create request: %v", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", "hsctl/"+info.Version)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	// If the HTTPClient implements DoContext, use it
	type contextDoer interface {
		DoContext(context.Context, *http.Request) (*http.Response, error)
	}
	if cd, ok := c.httpClient.(contextDoer); ok {
		return cd.DoContext(ctx, req)
	}

	// Otherwise use the standard Do method (context is already on the request)
	return c.httpClient.Do(req)
}

func (c *Connection) Get(path string, query url.Values) (*http.Response, error) {
	return c.Do(http.MethodGet, path, query, nil)
}

func (c *Connection) Post(path string, body io.Reader) (*http.Response, error) {
	return c.Do(http.MethodPost, path, nil, body)
}

func (c *Connection) Put(path string, body io.Reader) (*http.Response, error) {
	return c.Do(http.MethodPut, path, nil, body)
}

func (c *Connection) Patch(path string, body io.Reader) (*http.Response, error) {
	return c.Do(http.MethodPatch, path, nil, body)
}

func (c *Connection) Delete(path string) (*http.Response, error) {
	return c.Do(http.MethodDelete, path, nil, nil)
}

type ListResponse struct {
	Kind  string            `json:"kind"`
	Page  int               `json:"page"`
	Size  int               `json:"size"`
	Total int               `json:"total"`
	Items []json.RawMessage `json:"items"`
}

func (c *Connection) List(path string, page, size int, search, orderBy string) (*ListResponse, error) {
	query := url.Values{}
	query.Set("page", fmt.Sprintf("%d", page))
	query.Set("size", fmt.Sprintf("%d", size))
	if search != "" {
		query.Set("search", search)
	}
	if orderBy != "" {
		query.Set("orderBy", orderBy)
	}

	resp, err := c.Get(path, query)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API returned %d: %s", resp.StatusCode, string(body))
	}

	var result ListResponse
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return nil, fmt.Errorf("can't decode response: %v", err)
	}
	return &result, nil
}

func (c *Connection) Close() error {
	// Close idle connections if the HTTP client supports it
	if closer, ok := c.httpClient.(interface{ CloseIdleConnections() }); ok {
		closer.CloseIdleConnections()
	}
	return nil
}

// defaultHTTPClient is the default HTTP client implementation.
type defaultHTTPClient struct {
	client *http.Client
}

func newDefaultHTTPClient(insecure bool) HTTPClient {
	// A zero-value Transport ignores HTTP_PROXY, HTTPS_PROXY and NO_PROXY, so the
	// proxy function is set explicitly to match http.DefaultTransport.
	transport := &http.Transport{Proxy: http.ProxyFromEnvironment}
	if insecure {
		transport.TLSClientConfig = &tls.Config{
			MinVersion:         tls.VersionTLS12,
			InsecureSkipVerify: true, //nolint:gosec
		}
	}

	return &defaultHTTPClient{
		client: &http.Client{
			Transport: transport,
		},
	}
}

func (c *defaultHTTPClient) Do(req *http.Request) (*http.Response, error) {
	return c.client.Do(req)
}

func (c *defaultHTTPClient) CloseIdleConnections() {
	c.client.CloseIdleConnections()
}

// staticTokenProvider provides a static authentication token.
type staticTokenProvider struct {
	token string
}

func newStaticTokenProvider(token string) TokenProvider {
	return &staticTokenProvider{token: token}
}

func (p *staticTokenProvider) GetToken() (string, error) {
	return p.token, nil
}

// refreshMargin is how long before expiry the access token is renewed.
const refreshMargin = 30 * time.Second

// refreshingTokenProvider returns the saved access token and renews it first
// when it is expired or about to expire, saving the rotated refresh token.
type refreshingTokenProvider struct {
	mu  sync.Mutex
	cfg *config.Config
}

// NewConfigTokenProvider is the default token provider used by ConnectionBuilder:
// it renews the access token when the configuration holds an OIDC session and
// returns the saved token otherwise. It is exported so a project that builds its
// own connection, or passes a TokenProvider to another client such as the TUI,
// can use exactly the refresh, rotation and expiry behavior of the generated
// commands instead of copying it.
func NewConfigTokenProvider(cfg *config.Config) TokenProvider {
	if cfg.CanRefresh() {
		return &refreshingTokenProvider{cfg: cfg}
	}
	return newStaticTokenProvider(cfg.AccessToken)
}

func (p *refreshingTokenProvider) GetToken() (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.cfg.AccessTokenExpiring(refreshMargin) {
		return p.cfg.AccessToken, nil
	}

	// Another invocation may have refreshed already; rotating refresh tokens make
	// reusing a stale one fail, so adopt the stored session when it is newer.
	if latest, err := config.Load(); err == nil && latest != nil && latest.CanRefresh() &&
		latest.AccessToken != p.cfg.AccessToken && !latest.AccessTokenExpiring(refreshMargin) {
		p.cfg.AccessToken, p.cfg.RefreshToken, p.cfg.ExpiresAt = latest.AccessToken, latest.RefreshToken, latest.ExpiresAt
		return p.cfg.AccessToken, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client := oidc.NewClient(p.cfg.IssuerURL, p.cfg.ClientID, p.cfg.Insecure)
	tokens, err := client.Refresh(ctx, p.cfg.RefreshToken)
	if err != nil {
		return "", err
	}
	p.cfg.AccessToken = tokens.AccessToken
	if tokens.RefreshToken != "" {
		p.cfg.RefreshToken = tokens.RefreshToken
	}
	p.cfg.ExpiresAt = tokens.ExpiresAt(time.Now())
	if err := config.Save(p.cfg); err != nil {
		return "", fmt.Errorf("refreshed the session but can't save it: %w", err)
	}
	return p.cfg.AccessToken, nil
}

// ConnectionBuilder provides a backward-compatible builder for creating connections.
type ConnectionBuilder struct {
	cfg *config.Config
}

func NewConnectionBuilder() *ConnectionBuilder {
	return &ConnectionBuilder{}
}

// NewConnection returns a ConnectionBuilder, like NewConnectionBuilder. It keeps
// its original signature so hand-written commands that call
// connection.NewConnection().Config(cfg).Build() keep compiling.
func NewConnection() *ConnectionBuilder {
	return NewConnectionBuilder()
}

func (b *ConnectionBuilder) Config(value *config.Config) *ConnectionBuilder {
	b.cfg = value
	return b
}

func (b *ConnectionBuilder) Build() (result *Connection, err error) {
	if b.cfg == nil {
		b.cfg, err = config.Load()
		if err != nil {
			return
		}
		if b.cfg == nil {
			err = fmt.Errorf("not logged in, run the 'login' command")
			return
		}
	}

	armed, reason := b.cfg.Armed()
	if !armed {
		err = fmt.Errorf("not logged in, %s, run the 'login' command", reason)
		return
	}

	factoriesMu.RLock()
	newHTTPClient, newTokenProvider := httpClientFactory, tokenProviderFactory
	factoriesMu.RUnlock()

	httpClient := newDefaultHTTPClient(b.cfg.Insecure)
	if newHTTPClient != nil {
		httpClient = newHTTPClient(b.cfg)
	}
	tokenProvider := NewConfigTokenProvider(b.cfg)
	if newTokenProvider != nil {
		tokenProvider = newTokenProvider(b.cfg)
	}

	result = New(httpClient, tokenProvider, b.cfg.URL)
	return
}
