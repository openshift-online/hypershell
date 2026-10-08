package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"testing"
	"time"

	sdktypes "github.com/openshift-online/hypershell/components/sdk-go/types"
	"github.com/openshift-online/hypershell/tests/e2e/apiclient"
	"github.com/openshift-online/hypershell/tests/e2e/driver"
	"github.com/openshift-online/hypershell/tests/e2e/harness"
)

// TestPerformance is the performance harness entry point. It provisions a fleet of
// gateways in bounded-concurrency batches, measures provision latency, writes a
// results JSON, tears the fleet down, and (optionally) gates on SLOs. It is
// infra-agnostic: it uses the resolved E2EInfraDriver and the shared API client,
// and runs against whatever cluster the KUBECONFIG context selects.
//
// Env: E2E_PERF_GATEWAY_COUNT (default 5), E2E_PERF_BATCH_SIZE (default 5),
// E2E_CONCURRENCY (default 4), E2E_PROVISION_TIMEOUT (seconds, default 180),
// E2E_PERF_RESULTS_DIR (default perf-results), E2E_PERF_MIN_SUCCESS_RATE (optional,
// percent), E2E_PERF_MAX_PROVISION_P99 (optional, seconds).
func TestPerformance(t *testing.T) {
	h := newPerfHarness(t)
	defer h.cleanup()

	h.scaleUp(t)
	h.report(t)
	h.gateSLOs(t)
}

type perfHarness struct {
	driver    driver.E2EInfraDriver
	clients   *harness.Clients
	admin     *apiclient.Client
	clusterID string
	releaseID string

	count       int
	batchSize   int
	concurrency int
	provTimeout time.Duration
	runID       string

	mu         sync.Mutex
	created    []string // gateway ids for cleanup
	createLat  []time.Duration
	runningLat []time.Duration
	failures   int
}

func newPerfHarness(t *testing.T) *perfHarness {
	ctx := context.Background()
	clients, err := harness.NewClients()
	if err != nil {
		t.Fatalf("build Kubernetes clients: %v", err)
	}
	d, err := driver.Resolve(ctx, clients, os.Getenv("E2E_INFRA_DRIVER"))
	if err != nil {
		t.Fatalf("resolve infra driver: %v", err)
	}
	adminTok, err := d.AcquireOIDCToken(ctx, driver.Credentials{
		Username: envOrDefault("E2E_OIDC_USERNAME", "admin"),
		Password: envOrDefault("E2E_OIDC_PASSWORD", "admin"),
	})
	if err != nil {
		t.Fatalf("acquire admin token: %v", err)
	}
	api, err := d.APIClient(ctx, adminTok)
	if err != nil {
		t.Fatalf("build API client: %v", err)
	}

	clusterID, releaseID, err := perfSeedIDs(ctx, api, d.Name())
	if err != nil {
		t.Fatalf("discover seed ids: %v", err)
	}

	h := &perfHarness{
		driver:      d,
		clients:     clients,
		admin:       api,
		clusterID:   clusterID,
		releaseID:   releaseID,
		count:       intEnv("E2E_PERF_GATEWAY_COUNT", 5),
		batchSize:   intEnv("E2E_PERF_BATCH_SIZE", 5),
		concurrency: intEnv("E2E_CONCURRENCY", 4),
		provTimeout: durationSecondsEnv("E2E_PROVISION_TIMEOUT", 180*time.Second),
		runID:       fmt.Sprintf("%d", time.Now().Unix()%100000),
	}
	if h.concurrency < 1 {
		h.concurrency = 1
	}
	if h.batchSize < 1 {
		h.batchSize = h.count
	}
	t.Logf("perf harness: driver=%s count=%d batch=%d concurrency=%d", d.Name(), h.count, h.batchSize, h.concurrency)
	return h
}

// scaleUp provisions the fleet in batches, each batch fanned out to the
// concurrency limit, recording per-gateway create and time-to-Running latency.
func (h *perfHarness) scaleUp(t *testing.T) {
	ctx := context.Background()
	for start := 0; start < h.count; start += h.batchSize {
		end := min(start+h.batchSize, h.count)
		sem := make(chan struct{}, h.concurrency)
		var wg sync.WaitGroup
		for i := start; i < end; i++ {
			i := i
			wg.Add(1)
			go func() {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				h.provisionOne(ctx, t, fmt.Sprintf("perf-%s-%d", h.runID, i))
			}()
		}
		wg.Wait()
	}
}

func (h *perfHarness) provisionOne(ctx context.Context, t *testing.T, name string) {
	body := map[string]string{"name": name, "cluster_id": h.clusterID, "release_id": h.releaseID, "route": `{"enabled":true}`}
	createStart := time.Now()
	status, resp, err := h.admin.RawJSON(ctx, "POST", "/gateways", body)
	if err != nil || status != 201 {
		t.Logf("perf: create %s failed (status %d): %v", name, status, err)
		h.recordFailure()
		return
	}
	createLat := time.Since(createStart)
	var gw sdktypes.Gateway
	if json.Unmarshal(resp, &gw) != nil || gw.ID == "" {
		h.recordFailure()
		return
	}
	h.trackCreated(gw.ID)

	err = harness.Poll(ctx, 3*time.Second, h.provTimeout, func(ctx context.Context) (bool, error) {
		g, err := h.admin.Gateways().Get(ctx, gw.ID)
		if err != nil {
			return false, nil
		}
		return g.Phase == "Running", nil
	})
	if err != nil {
		t.Logf("perf: gateway %s never reached Running", name)
		h.recordFailure()
		return
	}
	h.recordSuccess(createLat, time.Since(createStart))
}

func (h *perfHarness) trackCreated(id string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.created = append(h.created, id)
}

func (h *perfHarness) recordFailure() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.failures++
}

func (h *perfHarness) recordSuccess(create, running time.Duration) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.createLat = append(h.createLat, create)
	h.runningLat = append(h.runningLat, running)
}

type perfResults struct {
	SchemaVersion    int       `json:"schema_version"`
	Driver           string    `json:"driver"`
	Timestamp        string    `json:"timestamp"`
	Requested        int       `json:"requested"`
	Provisioned      int       `json:"provisioned"`
	Failed           int       `json:"failed"`
	SuccessRatePct   float64   `json:"success_rate_pct"`
	Concurrency      int       `json:"concurrency"`
	BatchSize        int       `json:"batch_size"`
	TimeToRunningSec perfStats `json:"time_to_running_seconds"`
	CreateLatencySec perfStats `json:"create_latency_seconds"`
}

type perfStats struct {
	Avg float64 `json:"avg"`
	P50 float64 `json:"p50"`
	P90 float64 `json:"p90"`
	P99 float64 `json:"p99"`
	Max float64 `json:"max"`
}

// report computes metrics, logs a summary, and writes a results JSON.
func (h *perfHarness) report(t *testing.T) {
	h.mu.Lock()
	defer h.mu.Unlock()

	provisioned := len(h.runningLat)
	successRate := 0.0
	if h.count > 0 {
		successRate = float64(provisioned) / float64(h.count) * 100
	}
	res := perfResults{
		SchemaVersion:    1,
		Driver:           h.driver.Name(),
		Timestamp:        time.Now().UTC().Format(time.RFC3339),
		Requested:        h.count,
		Provisioned:      provisioned,
		Failed:           h.failures,
		SuccessRatePct:   successRate,
		Concurrency:      h.concurrency,
		BatchSize:        h.batchSize,
		TimeToRunningSec: computeStats(h.runningLat),
		CreateLatencySec: computeStats(h.createLat),
	}

	t.Logf("perf results: provisioned=%d/%d failed=%d success=%.1f%% ttr(p50=%.1fs p99=%.1fs max=%.1fs)",
		provisioned, h.count, h.failures, successRate,
		res.TimeToRunningSec.P50, res.TimeToRunningSec.P99, res.TimeToRunningSec.Max)

	dir := envOrDefault("E2E_PERF_RESULTS_DIR", "perf-results")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Logf("WARN create perf results dir: %v", err)
		return
	}
	path := filepath.Join(dir, fmt.Sprintf("%s-%s.json", h.driver.Name(), time.Now().UTC().Format("20060102-150405")))
	payload, _ := json.MarshalIndent(res, "", "  ")
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		t.Logf("WARN write perf results: %v", err)
		return
	}
	t.Logf("perf results written to %s", path)
}

// gateSLOs fails the test when optional SLO thresholds are set and not met.
func (h *perfHarness) gateSLOs(t *testing.T) {
	h.mu.Lock()
	provisioned := len(h.runningLat)
	stats := computeStats(h.runningLat)
	failures := h.failures
	h.mu.Unlock()

	if provisioned == 0 {
		t.Fatalf("perf: no gateways provisioned (failed=%d)", failures)
	}
	if v := os.Getenv("E2E_PERF_MIN_SUCCESS_RATE"); v != "" {
		var minRate float64
		if _, err := fmt.Sscanf(v, "%f", &minRate); err == nil {
			got := float64(provisioned) / float64(h.count) * 100
			if got < minRate {
				t.Errorf("perf SLO: success rate %.1f%% < required %.1f%%", got, minRate)
			}
		}
	}
	if v := os.Getenv("E2E_PERF_MAX_PROVISION_P99"); v != "" {
		var maxP99 float64
		if _, err := fmt.Sscanf(v, "%f", &maxP99); err == nil {
			if stats.P99 > maxP99 {
				t.Errorf("perf SLO: time-to-running p99 %.1fs > max %.1fs", stats.P99, maxP99)
			}
		}
	}
}

// cleanup deletes every gateway the harness created, concurrently.
func (h *perfHarness) cleanup() {
	if os.Getenv("E2E_SKIP_CLEANUP") == "1" {
		return
	}
	ctx := context.Background()
	h.mu.Lock()
	ids := append([]string(nil), h.created...)
	h.mu.Unlock()

	sem := make(chan struct{}, h.concurrency)
	var wg sync.WaitGroup
	for _, id := range ids {
		id := id
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			_ = h.admin.Gateways().Delete(ctx, id)
		}()
	}
	wg.Wait()
}

// computeStats returns avg/p50/p90/p99/max in seconds (nearest-rank percentiles).
func computeStats(durs []time.Duration) perfStats {
	if len(durs) == 0 {
		return perfStats{}
	}
	secs := make([]float64, len(durs))
	var sum float64
	for i, d := range durs {
		secs[i] = d.Seconds()
		sum += secs[i]
	}
	sort.Float64s(secs)
	pct := func(p float64) float64 {
		rank := int(float64(len(secs))*p/100.0 + 0.5)
		if rank < 1 {
			rank = 1
		}
		if rank > len(secs) {
			rank = len(secs)
		}
		return secs[rank-1]
	}
	return perfStats{
		Avg: sum / float64(len(secs)),
		P50: pct(50),
		P90: pct(90),
		P99: pct(99),
		Max: secs[len(secs)-1],
	}
}

// perfSeedIDs resolves the seeded cluster and release ids for the perf fleet.
func perfSeedIDs(ctx context.Context, api *apiclient.Client, driverName string) (clusterID, releaseID string, err error) {
	clusterName := "local-kind"
	if driverName == "openshift" {
		clusterName = "local-openshift"
	}
	clusterName = envOrDefault("E2E_SEED_CLUSTER_NAME", clusterName)
	releaseName := envOrDefault("E2E_SEED_RELEASE_NAME", "dev-release")

	clusters, err := api.ManagedClusters().List(ctx, nil)
	if err != nil {
		return "", "", fmt.Errorf("list managed clusters: %w", err)
	}
	for _, c := range clusters.Items {
		if c.OidcSubject == "" {
			continue
		}
		if c.Name == clusterName || clusterID == "" {
			clusterID = c.ID
		}
	}
	releases, err := api.GatewayReleases().List(ctx, nil)
	if err != nil {
		return "", "", fmt.Errorf("list gateway releases: %w", err)
	}
	for _, r := range releases.Items {
		if r.Name == releaseName || releaseID == "" {
			releaseID = r.ID
		}
	}
	if clusterID == "" || releaseID == "" {
		return "", "", fmt.Errorf("missing seed ids (cluster=%q release=%q)", clusterID, releaseID)
	}
	return clusterID, releaseID, nil
}
