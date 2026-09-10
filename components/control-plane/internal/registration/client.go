package registration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// ErrForbidden is returned when the API server responds 403.
// This is non-retryable: the spoke lacks the required Keycloak role.
var ErrForbidden = fmt.Errorf("registration denied: missing managed-cluster-registrar role in Keycloak")

// TokenSource can produce a bearer token.
type TokenSource interface {
	Token() (string, error)
}

// Client registers a spoke control-plane with the hub API server.
type Client struct {
	apiServerURL string
	clusterName  string
	tokens       TokenSource
	httpClient   *http.Client
}

// NewClient creates a registration Client.
func NewClient(apiServerURL, clusterName string, tokens TokenSource) *Client {
	return &Client{
		apiServerURL: strings.TrimRight(apiServerURL, "/"),
		clusterName:  clusterName,
		tokens:       tokens,
		httpClient:   &http.Client{Timeout: 30 * time.Second},
	}
}

type registrationRequest struct {
	Name string `json:"name"`
}

type registrationResponse struct {
	ClusterID string `json:"cluster_id"`
}

// Register calls POST /api/hypershell/v1/managed_clusters/registration.
// Returns (clusterID, nil) on success, (ErrForbidden, nil) on 403, or an
// error for transient failures that should be retried.
func (c *Client) Register(ctx context.Context) (string, error) {
	token, err := c.tokens.Token()
	if err != nil {
		return "", fmt.Errorf("get OIDC token: %w", err)
	}

	body, err := json.Marshal(registrationRequest{Name: c.clusterName})
	if err != nil {
		return "", fmt.Errorf("marshal registration request: %w", err)
	}

	url := c.apiServerURL + "/api/hypershell/v1/managed_clusters/registration"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build registration request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("POST %s: %w", url, err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			log.Printf("WARN closing registration response body: %v", closeErr)
		}
	}()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read registration response: %w", err)
	}

	if resp.StatusCode == http.StatusForbidden {
		return "", ErrForbidden
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("registration returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result registrationResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("parse registration response: %w", err)
	}

	if result.ClusterID == "" {
		return "", fmt.Errorf("registration response missing cluster_id")
	}

	return result.ClusterID, nil
}
