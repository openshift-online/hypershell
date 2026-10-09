package apiclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
