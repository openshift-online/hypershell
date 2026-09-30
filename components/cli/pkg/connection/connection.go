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

	"github.com/openshift-online/hypershell/components/cli/pkg/config"
	"github.com/openshift-online/hypershell/components/cli/pkg/info"
)

type Connection struct {
	baseURL    string
	token      string
	httpClient *http.Client

	// refresh, when set, renews the access token before each request so a
	// long-lived session keeps working after the initial token expires.
	refresh bool
	cfg     *config.Config
	mu      sync.Mutex
}

type ConnectionBuilder struct {
	cfg     *config.Config
	refresh bool
}

func NewConnection() *ConnectionBuilder {
	return &ConnectionBuilder{}
}

func (b *ConnectionBuilder) Config(value *config.Config) *ConnectionBuilder {
	b.cfg = value
	return b
}

// RefreshPerRequest makes the connection renew the access token before every
// request instead of only when it is built. Refresh never writes to the
// terminal; when renewal fails, requests return an error wrapping
// config.ErrSessionExpired.
func (b *ConnectionBuilder) RefreshPerRequest(value bool) *ConnectionBuilder {
	b.refresh = value
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

	if b.refresh {
		err = config.EnsureFreshTokenQuietly(b.cfg)
	} else {
		err = config.EnsureFreshToken(b.cfg)
	}
	if err != nil {
		return
	}

	result = &Connection{
		baseURL: strings.TrimRight(b.cfg.URL, "/"),
		token:   b.cfg.AccessToken,
		httpClient: &http.Client{
			Transport: newTransport(b.cfg.Insecure),
		},
		refresh: b.refresh,
		cfg:     b.cfg,
	}
	return
}

// accessToken returns the token to send, renewing it first when the
// connection refreshes per request.
func (c *Connection) accessToken() (string, error) {
	if !c.refresh {
		return c.token, nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := config.EnsureFreshTokenQuietly(c.cfg); err != nil {
		return "", err
	}
	c.token = c.cfg.AccessToken
	return c.token, nil
}

// newTransport sets Proxy explicitly because a zero-value http.Transport,
// unlike http.DefaultTransport, ignores HTTPS_PROXY / HTTP_PROXY / NO_PROXY.
func newTransport(insecure bool) *http.Transport {
	transport := &http.Transport{Proxy: http.ProxyFromEnvironment}
	if insecure {
		transport.TLSClientConfig = &tls.Config{
			MinVersion:         tls.VersionTLS12,
			InsecureSkipVerify: true, //nolint:gosec
		}
	}
	return transport
}

func (c *Connection) Do(method, path string, query url.Values, body io.Reader) (*http.Response, error) {
	return c.DoContext(context.Background(), method, path, query, body)
}

// DoContext is Do bound to ctx, so the request is abandoned when ctx ends.
func (c *Connection) DoContext(ctx context.Context, method, path string, query url.Values, body io.Reader) (*http.Response, error) {
	fullURL := c.baseURL + path
	if query != nil {
		fullURL += "?" + query.Encode()
	}

	token, err := c.accessToken()
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, method, fullURL, body)
	if err != nil {
		return nil, fmt.Errorf("can't create request: %v", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", "hypershell/"+info.Version)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return c.httpClient.Do(req)
}

func (c *Connection) Get(path string, query url.Values) (*http.Response, error) {
	return c.Do(http.MethodGet, path, query, nil)
}

func (c *Connection) Post(path string, body io.Reader) (*http.Response, error) {
	return c.Do(http.MethodPost, path, nil, body)
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
	c.httpClient.CloseIdleConnections()
	return nil
}
