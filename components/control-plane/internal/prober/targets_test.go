package prober

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func timePtr(t time.Time) *time.Time { return &t }

// TestDegradedGatewayStaysEligible covers OBS-PRB-02.01: a Degraded gateway
// created outside the grace window stays eligible; phase does not exclude it.
func TestDegradedGatewayStaysEligible(t *testing.T) {
	now := time.Now()
	gateways := []Gateway{{
		ID:           "g-degraded",
		Phase:        "Degraded",
		ClusterID:    "c-1",
		RouteAddress: "grpcs://g.example:443",
		CreatedAt:    timePtr(now.Add(-30 * time.Minute)),
	}}
	clusters := []ManagedCluster{{ID: "c-1", Name: "hyp0-spoke0"}}

	targets := EligibleGateways(gateways, clusters, now, DefaultGraceWindow)
	if len(targets) != 1 {
		t.Fatalf("eligible = %d, want the Degraded gateway to stay eligible", len(targets))
	}
	if targets[0].ManagedCluster != "hyp0-spoke0" {
		t.Errorf("managed_cluster = %q, want hyp0-spoke0", targets[0].ManagedCluster)
	}
}

// TestRecentlyCreatedGatewayNotEligible covers OBS-PRB-02.02.
func TestRecentlyCreatedGatewayNotEligible(t *testing.T) {
	now := time.Now()
	gateways := []Gateway{{
		ID:           "g-new",
		ClusterID:    "c-1",
		RouteAddress: "grpcs://g.example:443",
		CreatedAt:    timePtr(now.Add(-2 * time.Minute)),
	}}
	if targets := EligibleGateways(gateways, nil, now, DefaultGraceWindow); len(targets) != 0 {
		t.Fatalf("eligible = %d, want 0 for a 2-minute-old gateway", len(targets))
	}
}

// TestDeletingGatewayDropped covers OBS-PRB-02.03.
func TestDeletingGatewayDropped(t *testing.T) {
	now := time.Now()
	gateways := []Gateway{{
		ID:           "g-deleting",
		ClusterID:    "c-1",
		RouteAddress: "grpcs://g.example:443",
		CreatedAt:    timePtr(now.Add(-30 * time.Minute)),
		DeletedAt:    timePtr(now.Add(-time.Minute)),
	}}
	if targets := EligibleGateways(gateways, nil, now, DefaultGraceWindow); len(targets) != 0 {
		t.Fatalf("eligible = %d, want 0 for a deleting gateway", len(targets))
	}
}

// TestUnknownClusterAttribution covers OBS-PRB-02: an unresolved cluster_id is
// attributed to "unknown", not dropped.
func TestUnknownClusterAttribution(t *testing.T) {
	now := time.Now()
	gateways := []Gateway{{
		ID:           "g-1",
		ClusterID:    "c-missing",
		RouteAddress: "grpcs://g.example:443",
		CreatedAt:    timePtr(now.Add(-30 * time.Minute)),
	}}
	targets := EligibleGateways(gateways, []ManagedCluster{{ID: "c-other", Name: "spoke"}}, now, DefaultGraceWindow)
	if len(targets) != 1 {
		t.Fatalf("eligible = %d, want 1 (not dropped)", len(targets))
	}
	if targets[0].ManagedCluster != managedClusterUnknown {
		t.Errorf("managed_cluster = %q, want %q", targets[0].ManagedCluster, managedClusterUnknown)
	}
}

func TestGatewayWithoutRouteDropped(t *testing.T) {
	now := time.Now()
	gateways := []Gateway{{
		ID:        "g-noroute",
		ClusterID: "c-1",
		CreatedAt: timePtr(now.Add(-30 * time.Minute)),
	}}
	if targets := EligibleGateways(gateways, nil, now, DefaultGraceWindow); len(targets) != 0 {
		t.Fatalf("eligible = %d, want 0 for a gateway with no route address", len(targets))
	}
}

func TestListGatewaysSendsBearerToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("Authorization = %q, want Bearer test-token", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(gatewayListResponse{Items: []Gateway{{ID: "g-1"}}})
	}))
	defer srv.Close()

	client := NewAPIClient(srv.URL, staticToken("test-token"), 5*time.Second)
	gateways, err := client.ListGateways(context.Background())
	if err != nil {
		t.Fatalf("ListGateways: %v", err)
	}
	if len(gateways) != 1 || gateways[0].ID != "g-1" {
		t.Fatalf("gateways = %v, want one gateway g-1", gateways)
	}
}

func TestListManagedClusters(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(managedClusterListResponse{Items: []ManagedCluster{{ID: "c-1", Name: "spoke"}}})
	}))
	defer srv.Close()

	client := NewAPIClient(srv.URL, staticToken("t"), 5*time.Second)
	clusters, err := client.ListManagedClusters(context.Background())
	if err != nil {
		t.Fatalf("ListManagedClusters: %v", err)
	}
	if len(clusters) != 1 || clusters[0].Name != "spoke" {
		t.Fatalf("clusters = %v, want one cluster named spoke", clusters)
	}
}

// staticToken is a tokenSource returning a fixed token.
type staticToken string

func (s staticToken) Token(context.Context) (string, error) { return string(s), nil }
