package prober

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// managedClusterUnknown is the attribution for a gateway whose cluster_id does
// not resolve to a registered ManagedCluster (OBS-PRB-02, consistent with
// gateway-managed-cluster-attribution.spec.md). Such a gateway is attributed to
// "unknown", not dropped.
const managedClusterUnknown = "unknown"

// tokenSource produces a bearer token for an authenticated API read. The
// TokenExchanger satisfies it (OBS-PRB-06).
type tokenSource interface {
	Token(ctx context.Context) (string, error)
}

// Gateway is the subset of a gateway the prober reads to build its target set
// (OBS-PRB-02): enough to decide eligibility and to probe the gateway health RPC.
type Gateway struct {
	ID           string     `json:"id"`
	Phase        string     `json:"phase"`
	ClusterID    string     `json:"cluster_id"`
	RouteAddress string     `json:"route_address"`
	CreatedAt    *time.Time `json:"created_at"`
	// DeletedAt is set on a gateway being deleted. The API list already excludes
	// soft-deleted gateways, but the eligibility filter honours it defensively so
	// a deleting gateway is never probed (OBS-PRB-02.03).
	DeletedAt *time.Time `json:"deleted_at"`
}

// ManagedCluster maps a cluster_id (its id) to a human-readable spoke name used
// as the managed_cluster metric label (OBS-PRB-02, OBS-PRB-07).
type ManagedCluster struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// GatewayTarget is one eligible gateway health probe target (OBS-PRB-02): the
// gateway to probe, its public route, and its resolved managed-cluster
// attribution for the metric label.
type GatewayTarget struct {
	GatewayID      string
	ManagedCluster string
	RouteAddress   string
}

// APIClient reads the HyperShell API with the prober's keyless identity
// (OBS-PRB-06) to build the gateway target set (OBS-PRB-02). It is also the
// transport the API availability and authentication probes reuse.
type APIClient struct {
	baseURL    string
	httpClient *http.Client
	tokens     tokenSource
}

// NewAPIClient builds an APIClient for the given base URL. tokens may be nil when
// no keyless identity is configured; List calls then send no Authorization header
// and fail closed on the API's 401.
func NewAPIClient(baseURL string, tokens tokenSource, timeout time.Duration) *APIClient {
	return &APIClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{Timeout: timeout},
		tokens:     tokens,
	}
}

type gatewayListResponse struct {
	Items []Gateway `json:"items"`
}

type managedClusterListResponse struct {
	Items []ManagedCluster `json:"items"`
}

// ListGateways reads every gateway from the API with the prober identity
// (OBS-PRB-02). It pages through the list so a fleet larger than one page is
// fully covered.
func (c *APIClient) ListGateways(ctx context.Context) ([]Gateway, error) {
	var all []Gateway
	for page := 1; ; page++ {
		path := fmt.Sprintf("/api/hypershell/v1/gateways?page=%d&size=100", page)
		var resp gatewayListResponse
		if err := c.getJSON(ctx, path, &resp); err != nil {
			return nil, err
		}
		all = append(all, resp.Items...)
		if len(resp.Items) < 100 {
			break
		}
	}
	return all, nil
}

// ListManagedClusters reads every managed cluster from the API with the prober
// identity, for the cluster_id to managed_cluster attribution (OBS-PRB-02).
func (c *APIClient) ListManagedClusters(ctx context.Context) ([]ManagedCluster, error) {
	var all []ManagedCluster
	for page := 1; ; page++ {
		path := fmt.Sprintf("/api/hypershell/v1/managed_clusters?page=%d&size=100", page)
		var resp managedClusterListResponse
		if err := c.getJSON(ctx, path, &resp); err != nil {
			return nil, err
		}
		all = append(all, resp.Items...)
		if len(resp.Items) < 100 {
			break
		}
	}
	return all, nil
}

// getJSON performs an authenticated GET and decodes the JSON body.
func (c *APIClient) getJSON(ctx context.Context, path string, out any) error {
	resp, err := c.get(ctx, path)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			slog.WarnContext(ctx, "Failed to close API response body", "error", closeErr)
		}
	}()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response from %s: %w", path, err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s returned %d", path, resp.StatusCode)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("parse response from %s: %w", path, err)
	}
	return nil
}

// get performs an authenticated GET and returns the raw response for the caller
// to classify. The caller owns closing the body.
func (c *APIClient) get(ctx context.Context, path string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("build request for %s: %w", path, err)
	}
	req.Header.Set("Accept", "application/json")
	if c.tokens != nil {
		token, tokenErr := c.tokens.Token(ctx)
		if tokenErr != nil {
			return nil, fmt.Errorf("acquire token for %s: %w", path, tokenErr)
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", path, err)
	}
	return resp, nil
}

// EligibleGateways derives the gateway health target set from desired state
// (OBS-PRB-02). A gateway is eligible when it exists, is not being deleted, and
// was created more than graceWindow ago. Phase is NOT an eligibility input: a
// Degraded or Failed gateway stays eligible so its unanswered Health call is
// recorded as a failed probe. A gateway whose cluster_id does not resolve to a
// registered ManagedCluster is attributed to "unknown", not dropped. A gateway
// with no usable created_at or route address is dropped (it cannot be probed).
func EligibleGateways(gateways []Gateway, clusters []ManagedCluster, now time.Time, graceWindow time.Duration) []GatewayTarget {
	names := make(map[string]string, len(clusters))
	for _, c := range clusters {
		names[c.ID] = c.Name
	}

	targets := make([]GatewayTarget, 0, len(gateways))
	for _, g := range gateways {
		if g.DeletedAt != nil && !g.DeletedAt.IsZero() {
			continue // being deleted (OBS-PRB-02.03)
		}
		if g.CreatedAt == nil {
			continue // no creation time; cannot apply the grace window
		}
		if now.Sub(*g.CreatedAt) <= graceWindow {
			continue // inside the grace window (OBS-PRB-02.02)
		}
		if strings.TrimSpace(g.RouteAddress) == "" {
			continue // no public route to probe
		}

		managedCluster := managedClusterUnknown
		if name, ok := names[g.ClusterID]; ok && name != "" {
			managedCluster = name
		}

		targets = append(targets, GatewayTarget{
			GatewayID:      g.ID,
			ManagedCluster: managedCluster,
			RouteAddress:   g.RouteAddress,
		})
	}
	return targets
}
