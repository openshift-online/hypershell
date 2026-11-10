package registration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// healthProviderStub returns a fixed health value.
type healthProviderStub struct{ value any }

func (s healthProviderStub) LatestHealth() any { return s.value }

// captureHealth runs Register against a stub hub and returns the raw decoded
// request body so a test can assert whether the `health` field was present.
func captureHealth(t *testing.T, client *Client) map[string]any {
	t.Helper()
	var decoded map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&decoded); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"cluster_id":"2abc"}`))
	}))
	defer srv.Close()
	client.apiServerURL = srv.URL
	if _, err := client.Register(context.Background()); err != nil {
		t.Fatalf("Register() error: %v", err)
	}
	return decoded
}

func TestRegisterOmitsHealthWithoutProvider(t *testing.T) {
	// PLAT-CLUS-16.02: no provider -> health-less heartbeat so the hub preserves
	// the stored verdict.
	client := NewClient("http://placeholder", "mc1", nil, staticToken("tok"))
	body := captureHealth(t, client)
	if _, present := body["health"]; present {
		t.Fatalf("health field present without a provider: %+v", body)
	}
}

func TestRegisterOmitsHealthWhenProviderReturnsNil(t *testing.T) {
	// A provider that has no verdict yet (nil) yields a health-less heartbeat.
	client := NewClient("http://placeholder", "mc1", nil, staticToken("tok")).
		WithHealthProvider(healthProviderStub{value: nil})
	body := captureHealth(t, client)
	if _, present := body["health"]; present {
		t.Fatalf("health field present for a nil verdict: %+v", body)
	}
}

func TestRegisterCarriesHealthVerdict(t *testing.T) {
	// PLAT-HLTH-05 / PLAT-CLUS-16.01: a produced verdict is carried on the heartbeat.
	verdict := map[string]any{
		"overall":     "degraded",
		"observed_at": "2026-11-09T12:00:00Z",
		"duration_ms": 42,
		"conditions": []any{
			map[string]any{"name": "oidc-issuer-reachable", "status": "warn"},
		},
	}
	client := NewClient("http://placeholder", "mc1", nil, staticToken("tok")).
		WithHealthProvider(healthProviderStub{value: verdict})
	body := captureHealth(t, client)
	health, present := body["health"].(map[string]any)
	if !present {
		t.Fatalf("health field missing: %+v", body)
	}
	if health["overall"] != "degraded" {
		t.Fatalf("overall = %v, want degraded", health["overall"])
	}
}
