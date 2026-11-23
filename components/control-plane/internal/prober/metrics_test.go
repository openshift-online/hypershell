package prober

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestGatewayProbeIncrementsPerGatewaySeries covers OBS-PRB-07.01: a failed
// gateway health probe increments a per-gateway, per-managed-cluster series.
func TestGatewayProbeIncrementsPerGatewaySeries(t *testing.T) {
	m := NewMetrics()
	m.RecordGatewayHealth("g-123", "hyp0-spoke0", ResultFailure)

	body := scrape(t, m)
	want := `hypershell_probe_gateway_health_total{gateway_id="g-123",managed_cluster="hyp0-spoke0",result="failure"} 1`
	if !strings.Contains(body, want) {
		t.Fatalf("exposition output missing per-gateway series.\nwant line: %s\ngot:\n%s", want, body)
	}
}

// TestCountersAreScrapableInExpositionFormat covers OBS-PRB-07.02: the counters
// are served in Prometheus exposition format.
func TestCountersAreScrapableInExpositionFormat(t *testing.T) {
	m := NewMetrics()
	m.RecordGatewayHealth("g-1", "spoke", ResultSuccess)
	m.RecordAPIRead("https://api.example", ResultSuccess)
	m.RecordAPIAuth("https://api.example", ResultFailure)

	body := scrape(t, m)

	for _, want := range []string{
		"# TYPE hypershell_probe_gateway_health_total counter",
		"# TYPE hypershell_probe_api_read_total counter",
		"# TYPE hypershell_probe_api_auth_total counter",
		`hypershell_probe_api_read_total{instance="https://api.example",result="success"} 1`,
		`hypershell_probe_api_auth_total{instance="https://api.example",result="failure"} 1`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("exposition output missing %q.\ngot:\n%s", want, body)
		}
	}
}

func TestRecordIncrementsOncePerProbe(t *testing.T) {
	m := NewMetrics()
	m.RecordAPIRead("inst", ResultSuccess)
	m.RecordAPIRead("inst", ResultSuccess)
	m.RecordAPIRead("inst", ResultFailure)

	body := scrape(t, m)
	if !strings.Contains(body, `hypershell_probe_api_read_total{instance="inst",result="success"} 2`) {
		t.Errorf("expected success count 2.\ngot:\n%s", body)
	}
	if !strings.Contains(body, `hypershell_probe_api_read_total{instance="inst",result="failure"} 1`) {
		t.Errorf("expected failure count 1.\ngot:\n%s", body)
	}
}

func TestServeHealthz(t *testing.T) {
	m := NewMetrics()
	srv := httptest.NewServer(m.mux())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/healthz status = %d, want 200", resp.StatusCode)
	}
}

func TestServeDoesNotPanicWhenMetricsPathIsHealthz(t *testing.T) {
	// A metrics path of /healthz collides with the built-in health route; Serve
	// must skip the duplicate registration rather than panic the prober.
	m := NewMetrics()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				done <- fmt.Errorf("Serve panicked: %v", r)
			}
		}()
		done <- m.Serve(ctx, "127.0.0.1:0", "/healthz")
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Serve with metrics path /healthz: %v", err)
	}
}

// mux exposes the metrics handler for testing the exposition output and healthz
// without binding a port.
func (m *Metrics) mux() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/metrics", m.Handler())
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	return mux
}

func scrape(t *testing.T, m *Metrics) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	m.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("metrics scrape status = %d, want 200", rec.Code)
	}
	body, err := io.ReadAll(rec.Result().Body)
	if err != nil {
		t.Fatalf("read metrics body: %v", err)
	}
	return string(body)
}
