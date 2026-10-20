package apiclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
)

func TestGatewayGetDecodesProvisioningConditions(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, APIBasePath+"/gateways/gateway-1"; got != want {
			t.Errorf("request path = %q, want %q", got, want)
		}
		if got, want := r.Header.Get("Authorization"), "Bearer test-token"; got != want {
			t.Errorf("authorization = %q, want %q", got, want)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":        "gateway-1",
			"name":      "gateway",
			"namespace": "openshell-gateway",
			"phase":     "Running",
			"provisioning_conditions": []map[string]string{
				{
					"type":             "GatewayHealthy",
					"condition_status": "Complete",
					"message":          "Gateway is healthy",
				},
			},
		})
	}))
	defer server.Close()

	client, err := New(server.URL, "test-token", server.Client())
	if err != nil {
		t.Fatalf("new API client: %v", err)
	}

	got, err := client.Gateways().Get(context.Background(), "gateway-1")
	if err != nil {
		t.Fatalf("get gateway: %v", err)
	}
	if got.Phase != "Running" {
		t.Errorf("gateway phase = %q, want Running", got.Phase)
	}
}

func TestManagedClusterListDecodesCapabilities(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, APIBasePath+"/managed_clusters"; got != want {
			t.Errorf("request path = %q, want %q", got, want)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{{
				"id":                "cluster-1",
				"name":              "cluster",
				"kubeconfig_secret": "cluster-kubeconfig",
				"provider":          "kind",
				"capabilities":      []string{"network/local", "provider/kind"},
			}},
		})
	}))
	defer server.Close()

	client, err := New(server.URL, "test-token", server.Client())
	if err != nil {
		t.Fatalf("new API client: %v", err)
	}

	clusters, err := client.ManagedClusters().List(context.Background(), nil)
	if err != nil {
		t.Fatalf("list managed clusters: %v", err)
	}
	if got, want := clusters.Items[0].Capabilities, []string{"network/local", "provider/kind"}; !slices.Equal(got, want) {
		t.Errorf("capabilities = %v, want %v", got, want)
	}
}
