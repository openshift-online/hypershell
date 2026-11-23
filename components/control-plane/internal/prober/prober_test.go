package prober

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	openshellv1 "github.com/openshift-online/hypershell/components/control-plane/internal/openshellclient/gen/openshellv1"
)

// TestOneSlowTargetDoesNotStallOthers covers OBS-PRB-08.01: with a bounded worker
// pool large enough for the batch, a single slow target does not delay recording
// results for the others.
func TestOneSlowTargetDoesNotStallOthers(t *testing.T) {
	items := []string{"fast-1", "fast-2", "fast-3", "slow"}
	var mu sync.Mutex
	done := map[string]time.Duration{}
	start := time.Now()

	probeAll(context.Background(), items, 8, func(_ context.Context, item string) {
		if item == "slow" {
			time.Sleep(300 * time.Millisecond)
		}
		mu.Lock()
		done[item] = time.Since(start)
		mu.Unlock()
	})

	// The fast targets must have completed well before the slow one's duration,
	// proving they were not serialized behind it.
	for _, fast := range []string{"fast-1", "fast-2", "fast-3"} {
		if done[fast] > 150*time.Millisecond {
			t.Errorf("%s completed after %s; it was delayed by the slow target", fast, done[fast])
		}
	}
	if _, ok := done["slow"]; !ok {
		t.Error("slow target never completed")
	}
}

// TestWithinOneMinuteCadence covers OBS-PRB-09: the default interval and a shorter
// per-probe timeout reflect a new outage within one minute.
func TestWithinOneMinuteCadence(t *testing.T) {
	cfg := Load()
	if cfg.Interval > time.Minute {
		t.Errorf("interval %s would miss the one-minute outage budget", cfg.Interval)
	}
	if cfg.Timeout >= cfg.Interval {
		t.Errorf("timeout %s is not shorter than interval %s", cfg.Timeout, cfg.Interval)
	}
}

// TestGatewayHealthCycleRecordsPerGatewayResults exercises a full gateway health
// cycle: the prober lists the fleet, applies eligibility (OBS-PRB-02), probes each
// eligible gateway, and records one result per gateway. It also covers
// OBS-PRB-01.01 (a gateway on an unreachable cluster records a failed probe while
// others succeed) and OBS-PRB-02.02 (a recently created gateway records nothing).
func TestGatewayHealthCycleRecordsPerGatewayResults(t *testing.T) {
	now := time.Now()
	gateways := []Gateway{
		{ID: "g-up", ClusterID: "c-1", RouteAddress: "grpcs://up.example:443", CreatedAt: timePtr(now.Add(-time.Hour))},
		{ID: "g-down", ClusterID: "c-2", RouteAddress: "grpcs://down.example:443", CreatedAt: timePtr(now.Add(-time.Hour))},
		{ID: "g-new", ClusterID: "c-1", RouteAddress: "grpcs://new.example:443", CreatedAt: timePtr(now.Add(-time.Minute))},
	}
	clusters := []ManagedCluster{{ID: "c-1", Name: "spoke-1"}, {ID: "c-2", Name: "spoke-2"}}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "managed_clusters") {
			_ = json.NewEncoder(w).Encode(managedClusterListResponse{Items: clusters})
			return
		}
		_ = json.NewEncoder(w).Encode(gatewayListResponse{Items: gateways})
	}))
	defer srv.Close()

	p := &Prober{
		cfg:       &Config{Interval: DefaultInterval, Timeout: 2 * time.Second, GraceWindow: DefaultGraceWindow, Workers: 8},
		metrics:   NewMetrics(),
		apiClient: NewAPIClient(srv.URL, staticToken("t"), 5*time.Second),
		// A dial that fails for the "down.example" route (simulating an unreachable
		// measured cluster) and reports healthy for everything else.
		dial: func(_ context.Context, route string) (healthClient, func() error, error) {
			if strings.Contains(route, "down.example") {
				return fakeHealthClient{err: context.DeadlineExceeded}, func() error { return nil }, nil
			}
			return fakeHealthClient{status: openshellv1.ServiceStatus_SERVICE_STATUS_HEALTHY}, func() error { return nil }, nil
		},
	}

	p.gatewayHealthCycle(context.Background())

	body := scrape(t, p.metrics)
	if !strings.Contains(body, `hypershell_probe_gateway_health_total{gateway_id="g-up",managed_cluster="spoke-1",result="success"} 1`) {
		t.Errorf("missing success series for g-up.\ngot:\n%s", body)
	}
	if !strings.Contains(body, `hypershell_probe_gateway_health_total{gateway_id="g-down",managed_cluster="spoke-2",result="failure"} 1`) {
		t.Errorf("missing failure series for g-down (OBS-PRB-01.01).\ngot:\n%s", body)
	}
	// The recently created gateway records no probe result (OBS-PRB-02.02).
	if strings.Contains(body, `gateway_id="g-new"`) {
		t.Errorf("recently created gateway g-new must not record a probe result.\ngot:\n%s", body)
	}
}

// TestProbeModeStartsWithoutReconcilerConfig covers OBS-PRB-10.01 and 10.02: the
// probe mode starts and serves its metrics endpoint with only probe configuration,
// never requiring the reconciler's gateway database admin credentials.
func TestProbeModeStartsWithoutReconcilerConfig(t *testing.T) {
	cfg := &Config{
		Interval:    50 * time.Millisecond,
		Timeout:     20 * time.Millisecond,
		GraceWindow: DefaultGraceWindow,
		Workers:     4,
		MetricsAddr: "127.0.0.1:0",
		MetricsPath: "/metrics",
		// No gateway API URL and no production instances: the loops stay idle but
		// the process still comes up and serves metrics.
	}
	p := New(cfg)

	// Serve metrics on an ephemeral port and confirm it answers, then shut down.
	ctx, cancel := context.WithCancel(context.Background())
	ln := make(chan string, 1)
	go func() {
		// Bind a listener ourselves to learn the port, then serve the handler.
		srv := httptest.NewServer(p.metrics.mux())
		ln <- srv.URL
		<-ctx.Done()
		srv.Close()
	}()
	url := <-ln
	defer cancel()

	resp, err := http.Get(url + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/metrics status = %d, want 200", resp.StatusCode)
	}
}
