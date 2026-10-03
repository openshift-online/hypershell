// Package sources implements the fleet-dashboard data planes: Prometheus fleet
// metrics, GitOps Promoter + Argo promotion state, topology ConfigMaps, and
// instance discovery. All fleet-specific identifiers arrive via config or are
// discovered at runtime (data-architecture.spec §3.5).
package sources

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/openshift-online/hypershell/components/fleet-dashboard/pkg/config"
)

// Prometheus queries the metrics receiver over its HTTP API. The instance
// filter is built from discovery, never hard-coded.
type Prometheus struct {
	base          string
	tokenFile     string
	metric        string
	instLabel     string
	sandboxMetric string
	clusterLabel  string
	client        *http.Client
	logger        *slog.Logger
}

// NewPrometheus builds a Prometheus source from config.
func NewPrometheus(c *config.Config) *Prometheus {
	tr := &http.Transport{}
	if c.PromInsecure {
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // opt-in for self-signed in-cluster endpoints
	}
	return &Prometheus{
		base:          strings.TrimRight(c.PromURL, "/"),
		tokenFile:     c.PromTokenFile,
		metric:        c.GatewayMetric,
		instLabel:     c.InstanceLabel,
		sandboxMetric: c.SandboxMetric,
		clusterLabel:  c.ClusterLabel,
		client:        &http.Client{Timeout: 20 * time.Second, Transport: tr},
		logger:        slog.Default(),
	}
}

type promResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string           `json:"resultType"`
		Result     []promResultItem `json:"result"`
	} `json:"data"`
	Error string `json:"error"`
}

type promResultItem struct {
	Metric map[string]string `json:"metric"`
	Value  []any             `json:"value"`  // instant: [ts, "val"]
	Values [][]any           `json:"values"` // range: [[ts, "val"], ...]
}

func (p *Prometheus) do(ctx context.Context, path string, q url.Values) (*promResponse, error) {
	u := p.base + path + "?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	if p.tokenFile != "" {
		tok, err := os.ReadFile(p.tokenFile)
		if err != nil {
			return nil, fmt.Errorf("read prometheus token: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(string(tok)))
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	var pr promResponse
	if err := json.NewDecoder(resp.Body).Decode(&pr); err != nil {
		return nil, fmt.Errorf("decode prometheus response: %w", err)
	}
	if resp.StatusCode != http.StatusOK || pr.Status != "success" {
		return nil, fmt.Errorf("prometheus query failed (%d): %s", resp.StatusCode, pr.Error)
	}
	return &pr, nil
}

func (p *Prometheus) instant(ctx context.Context, expr string) ([]promResultItem, error) {
	pr, err := p.do(ctx, "/api/v1/query", url.Values{"query": {expr}})
	if err != nil {
		return nil, err
	}
	return pr.Data.Result, nil
}

// queryRange runs a range query over [start, end] at the given step, returning one
// result item per series (each carrying a Values time-series). Stateless: the full
// history is recomputed from Prometheus on every call (no in-process buffer).
func (p *Prometheus) queryRange(ctx context.Context, expr string, start, end time.Time, step time.Duration) ([]promResultItem, error) {
	secs := int(step.Seconds())
	if secs < 1 {
		secs = 1
	}
	pr, err := p.do(ctx, "/api/v1/query_range", url.Values{
		"query": {expr},
		"start": {strconv.FormatInt(start.Unix(), 10)},
		"end":   {strconv.FormatInt(end.Unix(), 10)},
		"step":  {strconv.Itoa(secs) + "s"},
	})
	if err != nil {
		return nil, err
	}
	return pr.Data.Result, nil
}

func sampleValue(v []any) float64 {
	if len(v) < 2 {
		return 0
	}
	s, ok := v[1].(string)
	if !ok {
		return 0
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	// PromQL legitimately yields non-finite results -- 0/0 -> NaN, x/0 -> +Inf,
	// histogram_quantile over empty buckets -> NaN (e.g. an instance with no BFF
	// traffic). encoding/json CANNOT marshal NaN/Inf: a single one makes the
	// whole /api/fleet payload fail to encode, which surfaced as an empty 200
	// body and a blank "Could not load this view" panel. Treat a non-finite
	// sample as 0 (an absent rate/quantile is zero for display purposes).
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0
	}
	return f
}

// sampleTime returns a range sample's unix timestamp (v[0]) as an int64, or 0 if
// absent. Prometheus encodes it as a JSON number (float seconds); we floor to the
// second so matching phase series land on the same step key.
func sampleTime(v []any) int64 {
	if len(v) < 1 {
		return 0
	}
	if f, ok := v[0].(float64); ok {
		return int64(f)
	}
	return 0
}

// DiscoverInstances returns the sorted set of instance keys emitting the gateway
// metric - the dynamic replacement for a hard-coded instance list.
func (p *Prometheus) DiscoverInstances(ctx context.Context) ([]string, error) {
	expr := fmt.Sprintf("group by (%s) (%s)", p.instLabel, p.metric)
	res, err := p.instant(ctx, expr)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []string
	for _, r := range res {
		if v := r.Metric[p.instLabel]; v != "" && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out, nil
}

// InstanceEnvelope is the /api/instances payload data.
type InstanceEnvelope struct {
	Instances []string `json:"instances"`
}

// Instances is the fetch function for the /api/instances data plane.
func (p *Prometheus) Instances(ctx context.Context) (any, error) {
	inst, err := p.DiscoverInstances(ctx)
	if err != nil {
		return nil, err
	}
	return InstanceEnvelope{Instances: inst}, nil
}

// InstanceFleet is the per-instance fleet health/throughput summary.
type InstanceFleet struct {
	Instance       string         `json:"instance"`
	Gateways       map[string]int `json:"gateways"` // phase -> count
	GatewaysTotal  int            `json:"gatewaysTotal"`
	ManagedCluster float64        `json:"managedClusters"`
	Users          float64        `json:"users"`
	RPC            RateStats      `json:"rpc"`
	Reconcile      RateStats      `json:"reconcile"`
	BFF            RateStats      `json:"bff"`
	ProvisionP95Ms float64        `json:"provisionP95Ms"`
	// GatewayHistory is per-phase gateway counts sampled oldest->newest over the
	// last day, feeding the per-instance stacked "sand" sparkline on the map.
	GatewayHistory []GatewayHistorySample `json:"gatewayHistory"`
	// Sandboxes is the instance's total active agent-sandbox count across all its
	// gateways; SandboxesByCluster breaks that total down per managed cluster, for
	// the detail panel's sandbox widget + per-cluster "chin" chart.
	Sandboxes          int                   `json:"sandboxes"`
	SandboxesByCluster []SandboxClusterCount `json:"sandboxesByCluster"`
	// SandboxHistory is the instance's TOTAL active-sandbox count (summed across its
	// clusters) sampled oldest->newest over the last day on the SAME 24h/32-sample grid
	// as GatewayHistory, so the sandbox "sand" sparkline (the lower node-card chin)
	// shares the gateway sparkline's x-axis exactly and the two chins are comparable.
	SandboxHistory []float64 `json:"sandboxHistory"`
}

// SandboxClusterCount is one managed cluster's active-sandbox count within an
// instance. Cluster is the opaque scrape-injected cluster label value.
type SandboxClusterCount struct {
	Cluster string `json:"cluster"`
	Count   int    `json:"count"`
}

// RateStats is a rate + error% + p95 latency triple.
type RateStats struct {
	Rate     float64 `json:"rate"`
	ErrorPct float64 `json:"errorPct"`
	P95Ms    float64 `json:"p95Ms"`
}

// GatewayHistorySample is one time-step of the stacked "sand" sparkline: gateway
// counts split into the three phases the UI layers (running/provisioning/failed).
// Phases the controller reports outside these three are not plotted, mirroring the
// prototype's three-layer stack.
type GatewayHistorySample struct {
	Running      float64 `json:"running"`
	Provisioning float64 `json:"provisioning"`
	Failed       float64 `json:"failed"`
}

// Fleet builds the /api/fleet payload: per-instance gateway phase counts and
// control-plane throughput, ported from the prototype's live.js PromQL. The
// namespace filter is built from discovery.
func (p *Prometheus) Fleet(ctx context.Context) (any, error) {
	inst, err := p.DiscoverInstances(ctx)
	if err != nil {
		return nil, err
	}
	if len(inst) == 0 {
		return map[string]InstanceFleet{}, nil
	}
	nsRE := strings.Join(inst, "|")
	k := "k8s_namespace_name" // trace/rpc metrics carry the real instance here
	byInst := map[string]*InstanceFleet{}
	for _, i := range inst {
		byInst[i] = &InstanceFleet{Instance: i, Gateways: map[string]int{}}
	}
	get := func(key string) *InstanceFleet {
		f, ok := byInst[key]
		if !ok {
			f = &InstanceFleet{Instance: key, Gateways: map[string]int{}}
			byInst[key] = f
		}
		return f
	}

	// Each sub-query below is best-effort: a partially-failing Prometheus must
	// still yield a usable snapshot (data-architecture.spec §5.2). But we must
	// not silently swallow those failures (CLAUDE.md), so collect them and
	// surface the degradation via a single Warn once the snapshot is assembled.
	var subErrs []error
	var subTotal int
	runInstant := func(expr string) ([]promResultItem, bool) {
		subTotal++
		res, err := p.instant(ctx, expr)
		if err != nil {
			subErrs = append(subErrs, err)
			return nil, false
		}
		return res, true
	}

	// Gateways by phase.
	if res, ok := runInstant(fmt.Sprintf("sum by (%s,phase) (%s{%s=~%q})", p.instLabel, p.metric, p.instLabel, nsRE)); ok {
		for _, r := range res {
			f := get(r.Metric[p.instLabel])
			phase := strings.ToLower(r.Metric["phase"])
			n := int(sampleValue(r.Value))
			f.Gateways[phase] += n
			f.GatewaysTotal += n
		}
	}

	// Gateway-count history (newest sample last) for the per-instance "sand"
	// sparkline. Option A: stateless - the full window is recomputed from
	// Prometheus on every snapshot, so no history is buffered in-process.
	// Best-effort like the instant sub-queries above.
	{
		const histWindow = 24 * time.Hour
		const histSamples = 32
		now := time.Now()
		subTotal++
		// Split history by phase so the sand chart can stack running/provisioning/
		// failed over time (parity with the prototype). Range series come back one
		// per (instance, phase); align them on the shared step grid by timestamp,
		// since a phase that only appeared mid-window yields a shorter series.
		expr := fmt.Sprintf("sum by (%s,phase) (%s{%s=~%q})", p.instLabel, p.metric, p.instLabel, nsRE)
		if res, err := p.queryRange(ctx, expr, now.Add(-histWindow), now, histWindow/histSamples); err != nil {
			subErrs = append(subErrs, err)
		} else {
			// instance -> (timestamp -> stacked sample)
			byTS := map[string]map[int64]*GatewayHistorySample{}
			for _, r := range res {
				inst := r.Metric[p.instLabel]
				phase := strings.ToLower(r.Metric["phase"])
				steps, ok := byTS[inst]
				if !ok {
					steps = map[int64]*GatewayHistorySample{}
					byTS[inst] = steps
				}
				for _, v := range r.Values {
					ts := sampleTime(v)
					s, ok := steps[ts]
					if !ok {
						s = &GatewayHistorySample{}
						steps[ts] = s
					}
					val := sampleValue(v)
					switch phase {
					case "running":
						s.Running += val
					case "provisioning":
						s.Provisioning += val
					case "failed":
						s.Failed += val
					}
				}
			}
			for inst, steps := range byTS {
				tss := make([]int64, 0, len(steps))
				for ts := range steps {
					tss = append(tss, ts)
				}
				sort.Slice(tss, func(i, j int) bool { return tss[i] < tss[j] })
				hist := make([]GatewayHistorySample, 0, len(tss))
				for _, ts := range tss {
					hist = append(hist, *steps[ts])
				}
				get(inst).GatewayHistory = hist
			}
		}
	}

	// Simple by-namespace gauges.
	byNS := func(expr string, set func(f *InstanceFleet, v float64)) {
		if res, ok := runInstant(expr); ok {
			for _, r := range res {
				set(get(r.Metric[p.instLabel]), sampleValue(r.Value))
			}
		}
	}
	byNS(fmt.Sprintf("sum by (%s) (hypershell_managed_clusters_total{%s=~%q})", p.instLabel, p.instLabel, nsRE),
		func(f *InstanceFleet, v float64) { f.ManagedCluster = v })
	byNS(fmt.Sprintf("sum by (%s) (hypershell_users_registered_total{%s=~%q})", p.instLabel, p.instLabel, nsRE),
		func(f *InstanceFleet, v float64) { f.Users = v })
	byNS(fmt.Sprintf("1000 * histogram_quantile(0.95, sum by (%s,le) (rate(gateway_provision_duration_seconds_bucket{%s=~%q}[30m])))", p.instLabel, p.instLabel, nsRE),
		func(f *InstanceFleet, v float64) { f.ProvisionP95Ms = v })

	// Active sandboxes, broken down per managed cluster. The gauge is reported
	// per-gateway (the cluster/gateway labels are scrape-injected, not emitted by
	// the metric), so dedupe scrape series with an inner max by (cluster,instance,
	// gateway) then sum the gateways within each cluster - mirroring the fleet
	// Grafana "Sandboxes by cluster" panel. The per-instance total is summed in Go
	// from the per-cluster rows (one query instead of two). Best-effort.
	if res, ok := runInstant(fmt.Sprintf(
		"sum by (%s,%s) (max by (%s,%s,gateway) (%s{%s=~%q}))",
		p.instLabel, p.clusterLabel, p.clusterLabel, p.instLabel, p.sandboxMetric, p.instLabel, nsRE,
	)); ok {
		for _, r := range res {
			f := get(r.Metric[p.instLabel])
			cluster := r.Metric[p.clusterLabel]
			n := int(sampleValue(r.Value))
			f.SandboxesByCluster = append(f.SandboxesByCluster, SandboxClusterCount{Cluster: cluster, Count: n})
			f.Sandboxes += n
		}
		// Stable, meaningful order: busiest cluster first, ties broken by name so a
		// given snapshot always renders the rows the same way.
		for _, f := range byInst {
			sort.Slice(f.SandboxesByCluster, func(i, j int) bool {
				a, b := f.SandboxesByCluster[i], f.SandboxesByCluster[j]
				if a.Count != b.Count {
					return a.Count > b.Count
				}
				return a.Cluster < b.Cluster
			})
		}
	}

	// Sandbox-count history (newest sample last) for the per-instance sandbox "sand"
	// sparkline - the lower node-card chin. Deliberately on the SAME 24h/32-sample grid
	// as GatewayHistory above so the two chins share an x-axis exactly and are directly
	// comparable. One series per instance: the TOTAL across clusters, deduping scrape
	// replicas with the same inner max-by-(cluster,instance,gateway) the snapshot uses
	// before summing the gateways. Best-effort like the other sub-queries.
	{
		const histWindow = 24 * time.Hour
		const histSamples = 32
		now := time.Now()
		subTotal++
		expr := fmt.Sprintf(
			"sum by (%s) (max by (%s,%s,gateway) (%s{%s=~%q}))",
			p.instLabel, p.clusterLabel, p.instLabel, p.sandboxMetric, p.instLabel, nsRE,
		)
		if res, err := p.queryRange(ctx, expr, now.Add(-histWindow), now, histWindow/histSamples); err != nil {
			subErrs = append(subErrs, err)
		} else {
			for _, r := range res {
				f := get(r.Metric[p.instLabel])
				// One series per instance; Prometheus returns values in ascending time
				// order, which is the oldest->newest order the sparkline expects.
				hist := make([]float64, 0, len(r.Values))
				for _, v := range r.Values {
					hist = append(hist, sampleValue(v))
				}
				f.SandboxHistory = hist
			}
		}
	}

	// Rate/error/p95 triples keyed by k8s_namespace_name.
	byK := func(expr string, set func(f *InstanceFleet, v float64)) {
		if res, ok := runInstant(expr); ok {
			for _, r := range res {
				set(get(r.Metric[k]), sampleValue(r.Value))
			}
		}
	}
	byK(fmt.Sprintf("sum by (%s) (rate(rpc_server_duration_milliseconds_count{%s=~%q}[5m]))", k, k, nsRE),
		func(f *InstanceFleet, v float64) { f.RPC.Rate = v })
	byK(fmt.Sprintf("100 * sum by (%s) (rate(rpc_server_duration_milliseconds_count{rpc_grpc_status_code!=\"0\",%s=~%q}[5m])) / sum by (%s) (rate(rpc_server_duration_milliseconds_count{%s=~%q}[5m]))", k, k, nsRE, k, k, nsRE),
		func(f *InstanceFleet, v float64) { f.RPC.ErrorPct = v })
	byK(fmt.Sprintf("histogram_quantile(0.95, sum by (%s,le) (rate(rpc_server_duration_milliseconds_bucket{%s=~%q}[10m])))", k, k, nsRE),
		func(f *InstanceFleet, v float64) { f.RPC.P95Ms = v })

	byK(fmt.Sprintf("60 * sum by (%s) (rate(reconcile_duration_milliseconds_count{%s=~%q}[5m]))", k, k, nsRE),
		func(f *InstanceFleet, v float64) { f.Reconcile.Rate = v })
	byK(fmt.Sprintf("60 * sum by (%s) (rate(reconcile_errors_total{%s=~%q}[5m]))", k, k, nsRE),
		func(f *InstanceFleet, v float64) { f.Reconcile.ErrorPct = v })
	byK(fmt.Sprintf("histogram_quantile(0.95, sum by (%s,le) (rate(reconcile_duration_milliseconds_bucket{%s=~%q}[10m])))", k, k, nsRE),
		func(f *InstanceFleet, v float64) { f.Reconcile.P95Ms = v })

	bff := `service_name="hypershell-web-console-bff"`
	byK(fmt.Sprintf("sum by (%s) (rate(traces_span_metrics_calls_total{%s,%s=~%q}[5m]))", k, bff, k, nsRE),
		func(f *InstanceFleet, v float64) { f.BFF.Rate = v })
	byK(fmt.Sprintf("100 * sum by (%s) (rate(traces_span_metrics_calls_total{%s,status_code=\"STATUS_CODE_ERROR\",%s=~%q}[5m])) / sum by (%s) (rate(traces_span_metrics_calls_total{%s,%s=~%q}[5m]))", k, bff, k, nsRE, k, bff, k, nsRE),
		func(f *InstanceFleet, v float64) { f.BFF.ErrorPct = v })
	byK(fmt.Sprintf("histogram_quantile(0.95, sum by (%s,le) (rate(traces_span_metrics_duration_milliseconds_bucket{%s,%s=~%q}[10m])))", k, bff, k, nsRE),
		func(f *InstanceFleet, v float64) { f.BFF.P95Ms = v })

	if len(subErrs) > 0 {
		// Degraded, not failed: the snapshot is still returned (best-effort), but
		// the partial failure is now visible rather than silently swallowed.
		p.logger.WarnContext(ctx, "fleet snapshot built with partial Prometheus failures",
			"failedSubqueries", len(subErrs),
			"totalSubqueries", subTotal,
			"firstError", subErrs[0].Error())
	}

	out := make(map[string]InstanceFleet, len(byInst))
	for key, f := range byInst {
		out[key] = *f
	}
	return out, nil
}
