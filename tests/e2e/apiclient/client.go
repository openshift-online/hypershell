// Package apiclient is a thin wrapper over the generated HyperShell Go SDK
// (components/sdk-go). It embeds the SDK client so the suite uses the typed
// resource APIs (Gateways, ManagedClusters, Users, RoleBindings) directly, and
// adds a raw authenticated request helper for
// the handful of endpoints the typed SDK does not cover (the unauthenticated 401
// probe, POST /managed_clusters/registration, and phase-write validation).
package apiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	sdkclient "github.com/openshift-online/hypershell/components/sdk-go/client"
)

// APIBasePath is the HyperShell API prefix every resource path sits under. The
// typed SDK adds this itself; the raw helper here adds it too so callers pass a
// resource-relative path like "/managed_clusters/registration".
const APIBasePath = "/api/hypershell/v1"

// Client wraps the generated SDK client with a raw request escape hatch.
type Client struct {
	*sdkclient.Client
	baseURL string
	token   string
	http    *http.Client
}

// New builds a wrapper around the generated SDK client for baseURL authenticated
// with token. httpClient is used for the raw helper only and SHOULD trust the
// cluster CA; the embedded SDK client reaches the API through http.DefaultTransport
// (see harness.InstallDefaultCA). A non-nil httpClient is required.
func New(baseURL, token string, httpClient *http.Client) (*Client, error) {
	sdk, err := sdkclient.NewClient(baseURL, token)
	if err != nil {
		return nil, fmt.Errorf("construct SDK client: %w", err)
	}
	if httpClient == nil {
		return nil, fmt.Errorf("httpClient is required")
	}
	return &Client{
		Client:  sdk,
		baseURL: strings.TrimSuffix(baseURL, "/"),
		token:   token,
		http:    httpClient,
	}, nil
}

// BaseURL reports the API base URL (scheme://host), without the API path prefix.
func (c *Client) BaseURL() string { return c.baseURL }

// RawJSON issues an authenticated JSON request to an API-relative path (for
// example "/managed_clusters/registration") and returns the status code and raw
// response body. body, when non-nil, is marshaled as JSON. It is the escape hatch
// for endpoints the typed SDK does not model; typed CRUD SHOULD use the embedded
// SDK APIs instead.
func (c *Client) RawJSON(ctx context.Context, method, path string, body any) (int, []byte, error) {
	var reader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return 0, nil, fmt.Errorf("marshal request body: %w", err)
		}
		reader = bytes.NewReader(buf)
	}

	url := c.baseURL + APIBasePath + path
	req, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		return 0, nil, fmt.Errorf("build request: %w", err)
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return resp.StatusCode, nil, fmt.Errorf("read response body: %w", err)
	}
	return resp.StatusCode, respBody, nil
}
