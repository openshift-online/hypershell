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

// BenchmarkGatewayProvisioning is the performance harness: a true Go benchmark
// that provisions a fleet of gateways in bounded-concurrency batches, reports
// provision-latency and throughput via b.ReportMetric (so `go test -bench` output
// and benchstat give native reporting + run comparison), writes a results JSON for
// dev->prod promotion gating, tears the fleet down, and fails the benchmark when
// optional SLOs are not met.
//
// It is a one-shot scale test, so run it with -benchtime 1x (make e2e-performance
// does): the fleet is provisioned once per benchmark iteration. It is
// infra-agnostic -- it uses the resolved E2EInfraDriver and runs against whatever
// cluster the KUBECONFIG context selects.
//
// Env: E2E_PERF_GATEWAY_COUNT (default 5), E2E_PERF_BATCH_SIZE (default 5),
// E2E_CONCURRENCY (default 4), E2E_PROVISION_TIMEOUT (seconds, default 180),
// E2E_PERF_RESULTS_DIR (default perf-results), E2E_PERF_MIN_SUCCESS_RATE (optional,
// percent), E2E_PERF_MAX_PROVISION_P99 (optional, seconds).
func BenchmarkGatewayProvisioning(b *testing.B) {
	h := newPerfHarness(b)
	b.Cleanup(h.cleanup)

	b.ResetTimer() // exclude harness/token/seed setup from the benchmark timer
	for range b.N {
		h.scaleUp(b)
	}
	b.StopTimer() // exclude reporting + teardown

	stats := computeStats(h.runningLat)
	provisioned := len(h.runningLat)
	successRate := 0.0
	if h.count > 0 {
		successRate = float64(provisioned) / float64(h.count) * 100
	}
	throughputPerMin := 0.0
	if stats.Max > 0 {
		throughputPerMin = float64(provisioned) / stats.Max * 60
	}

	// Native benchmark metrics (b.ReportMetric replaces the old custom report; the
	// default ns/op reflects total fleet provisioning wall time).
	b.ReportMetric(stats.P50, "ttr_p50_sec")
	b.ReportMetric(stats.P99, "ttr_p99_sec")
	b.ReportMetric(stats.Max, "ttr_max_sec")
	b.ReportMetric(throughputPerMin, "gw/min")
	b.ReportMetric(successRate, "success%")

	h.writeResults(b, successRate, throughputPerMin, stats)
	h.gateSLOs(b, provisioned, stats, successRate)
}

type perfHarness struct {
	driver    driver.E2EInfraDriver
	clients   *harness.Clients
	admin     *apiclient.Client
	clusterID string

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

func newPerfHarness(b *testing.B) *perfHarness {
	ctx := context.Background()
	clients, err := harness.NewClients()
	if err != nil {
		b.Fatalf("build Kubernetes clients: %v", err)
	}
	d, err := driver.Resolve(ctx, clients, os.Getenv("E2E_INFRA_DRIVER"))
	if err != nil {
		b.Fatalf("resolve infra driver: %v", err)
	}
	adminTok, err := d.AcquireOIDCToken(ctx, driver.Credentials{
		Username: envOrDefault("E2E_OIDC_USERNAME", "admin"),
		Password: envOrDefault("E2E_OIDC_PASSWORD", "admin"),
	})
	if err != nil {
		b.Fatalf("acquire admin token: %v", err)
	}
	api, err := d.APIClient(ctx, adminTok)
	if err != nil {
		b.Fatalf("build API client: %v", err)
	}
	clusterID, err := perfSeedIDs(ctx, api, d.Name())
	if err != nil {
		b.Fatalf("discover seed ids: %v", err)
	}

	h := &perfHarness{
		driver:      d,
		clients:     clients,
		admin:       api,
		clusterID:   clusterID,
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
	b.Logf("perf harness: driver=%s count=%d batch=%d concurrency=%d", d.Name(), h.count, h.batchSize, h.concurrency)
	return h
}

// scaleUp provisions the fleet in batches, each batch fanned out to the
// concurrency limit, recording per-gateway create and time-to-Running latency.
func (h *perfHarness) scaleUp(b *testing.B) {
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
				h.provisionOne(ctx, b, fmt.Sprintf("perf-%s-%d", h.runID, i))
			}()
		}
		wg.Wait()
	}
}

func (h *perfHarness) provisionOne(ctx context.Context, b *testing.B, name string) {
	body := map[string]string{"name": name, "cluster_id": h.clusterID, "route": `{"enabled":true}`}
	createStart := time.Now()
	status, resp, err := h.admin.RawJSON(ctx, "POST", "/gateways", body)
	if err != nil || status != 201 {
		b.Logf("perf: create %s failed (status %d): %v", name, status, err)
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
		b.Logf("perf: gateway %s never reached Running", name)
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
	ThroughputPerMin float64   `json:"throughput_per_min"`
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

// writeResults writes the results JSON consumed by promotion gating, and logs a
// one-line summary.
func (h *perfHarness) writeResults(b *testing.B, successRate, throughputPerMin float64, ttr perfStats) {
	h.mu.Lock()
	provisioned := len(h.runningLat)
	res := perfResults{
		SchemaVersion:    1,
		Driver:           h.driver.Name(),
		Timestamp:        time.Now().UTC().Format(time.RFC3339),
		Requested:        h.count,
		Provisioned:      provisioned,
		Failed:           h.failures,
		SuccessRatePct:   successRate,
		ThroughputPerMin: throughputPerMin,
		Concurrency:      h.concurrency,
		BatchSize:        h.batchSize,
		TimeToRunningSec: ttr,
		CreateLatencySec: computeStats(h.createLat),
	}
	h.mu.Unlock()

	b.Logf("perf results: provisioned=%d/%d failed=%d success=%.1f%% ttr(p50=%.1fs p99=%.1fs max=%.1fs) throughput=%.1f gw/min",
		provisioned, h.count, res.Failed, successRate, ttr.P50, ttr.P99, ttr.Max, throughputPerMin)

	dir := envOrDefault("E2E_PERF_RESULTS_DIR", "perf-results")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		b.Logf("WARN create perf results dir: %v", err)
		return
	}
	path := filepath.Join(dir, fmt.Sprintf("%s-%s.json", h.driver.Name(), time.Now().UTC().Format("20060102-150405")))
	payload, _ := json.MarshalIndent(res, "", "  ")
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		b.Logf("WARN write perf results: %v", err)
		return
	}
	b.Logf("perf results written to %s", path)
}

// gateSLOs fails the benchmark when optional SLO thresholds are set and not met.
func (h *perfHarness) gateSLOs(b *testing.B, provisioned int, stats perfStats, successRate float64) {
	if provisioned == 0 {
		b.Fatalf("perf: no gateways provisioned (failed=%d)", h.failures)
	}
	if v := os.Getenv("E2E_PERF_MIN_SUCCESS_RATE"); v != "" {
		var minRate float64
		if _, err := fmt.Sscanf(v, "%f", &minRate); err == nil && successRate < minRate {
			b.Errorf("perf SLO: success rate %.1f%% < required %.1f%%", successRate, minRate)
		}
	}
	if v := os.Getenv("E2E_PERF_MAX_PROVISION_P99"); v != "" {
		var maxP99 float64
		if _, err := fmt.Sscanf(v, "%f", &maxP99); err == nil && stats.P99 > maxP99 {
			b.Errorf("perf SLO: time-to-running p99 %.1fs > max %.1fs", stats.P99, maxP99)
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

// perfSeedIDs resolves the seeded cluster id for the perf fleet.
func perfSeedIDs(ctx context.Context, api *apiclient.Client, driverName string) (clusterID string, err error) {
	clusterName := "local-kind"
	if driverName == "openshift" {
		clusterName = "local-openshift"
	}
	clusterName = envOrDefault("E2E_SEED_CLUSTER_NAME", clusterName)

	clusters, err := api.ManagedClusters().List(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("list managed clusters: %w", err)
	}
	for _, c := range clusters.Items {
		if c.OidcSubject == "" {
			continue
		}
		if c.Name == clusterName || clusterID == "" {
			clusterID = c.ID
		}
	}
	if clusterID == "" {
		return "", fmt.Errorf("missing seed id (cluster=%q)", clusterID)
	}
	return clusterID, nil
}
