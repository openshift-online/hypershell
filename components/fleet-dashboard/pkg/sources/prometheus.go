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
	base             string
	tokenFile        string
	metric           string
	instLabel        string
	sandboxMetric    string
	clusterLabel     string
	userMetric       string
	userLoginsMetric string
	client           *http.Client
	logger           *slog.Logger
}

// NewPrometheus builds a Prometheus source from config.
func NewPrometheus(c *config.Config) *Prometheus {
	tr := &http.Transport{}
	if c.PromInsecure {
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // opt-in for self-signed in-cluster endpoints
	}
	return &Prometheus{
		base:             strings.TrimRight(c.PromURL, "/"),
		tokenFile:        c.PromTokenFile,
		metric:           c.GatewayMetric,
		instLabel:        c.InstanceLabel,
		sandboxMetric:    c.SandboxMetric,
		clusterLabel:     c.ClusterLabel,
		userMetric:       c.UserMetric,
		userLoginsMetric: c.UserLoginsMetric,
		client:           &http.Client{Timeout: 20 * time.Second, Transport: tr},
		logger:           slog.Default(),
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
	// Users is the instance's registered-user total; Logins is its rolling 7-day
	// unique-login count. UserHistory and LoginsHistory are the same values sampled
	// oldest->newest on the shared 24h/32-sample grid, feeding the detail panel's
	// Users and Logins metric tiles (headline number + mini sparkline). Users is the
	// long-standing scalar; the rest were added alongside the sandbox widget's model.
	Logins        float64   `json:"logins"`
	UserHistory   []float64 `json:"userHistory"`
	LoginsHistory []float64 `json:"loginsHistory"`
	// HistoryTimes is the canonical per-instance time axis (unix seconds, oldest->
	// newest) that EVERY history series above is index-aligned to: HistoryTimes[i] is
	// the timestamp of GatewayHistory[i], SandboxHistory[i], UserHistory[i] and
	// LoginsHistory[i]. One shared 24h/32-sample grid lets the detail panel draw a
	// shared temporal cursor across the sparklines and read each series (including the
	// gateway phase mix for that moment) at the hovered sample. Empty when no history.
	HistoryTimes []int64 `json:"historyTimes"`
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
	byNS(fmt.Sprintf("sum by (%s) (%s{%s=~%q})", p.instLabel, p.userMetric, p.instLabel, nsRE),
		func(f *InstanceFleet, v float64) { f.Users = v })
	byNS(fmt.Sprintf("sum by (%s) (%s{%s=~%q})", p.instLabel, p.userLoginsMetric, p.instLabel, nsRE),
		func(f *InstanceFleet, v float64) { f.Logins = v })
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

	// Per-instance history on ONE shared grid. All four series (the gateway phase mix,
	// sandbox total, registered users and 7-day logins) are range-queried over the same
	// 24h/32-sample window computed once here, then index-aligned to a single sorted
	// timestamp axis (HistoryTimes) per instance. Aligning every series to one axis -
	// rather than trusting each range query to return an identical grid - lets the
	// detail panel draw a shared temporal cursor and read every series at the hovered
	// sample, and keeps the two node-card chins (gateway + sandbox) sharing an x-axis.
	// Each sub-query is best-effort; a failing one leaves its contribution zero-filled.
	{
		const histWindow = 24 * time.Hour
		const histSamples = 32
		now := time.Now()
		start := now.Add(-histWindow)
		step := histWindow / histSamples

		// instance -> timestamp -> accumulated sample across the four series.
		type histPoint struct {
			gw      GatewayHistorySample
			sandbox float64
			users   float64
			logins  float64
		}
		byTS := map[string]map[int64]*histPoint{}
		pointAt := func(inst string, ts int64) *histPoint {
			steps, ok := byTS[inst]
			if !ok {
				steps = map[int64]*histPoint{}
				byTS[inst] = steps
			}
			pt, ok := steps[ts]
			if !ok {
				pt = &histPoint{}
				steps[ts] = pt
			}
			return pt
		}
		// rangeInto runs one range query and folds each (instance, timestamp) sample
		// into the shared point map via add. Best-effort: a query error is collected
		// and that series simply stays zero across the axis.
		rangeInto := func(expr string, add func(pt *histPoint, phase string, v float64)) {
			subTotal++
			res, err := p.queryRange(ctx, expr, start, now, step)
			if err != nil {
				subErrs = append(subErrs, err)
				return
			}
			for _, r := range res {
				inst := r.Metric[p.instLabel]
				phase := strings.ToLower(r.Metric["phase"])
				for _, v := range r.Values {
					add(pointAt(inst, sampleTime(v)), phase, sampleValue(v))
				}
			}
		}

		// Gateway phase mix: one series per (instance, phase); stack the three plotted
		// phases and ignore the rest, matching the node-card sand chart.
		rangeInto(
			fmt.Sprintf("sum by (%s,phase) (%s{%s=~%q})", p.instLabel, p.metric, p.instLabel, nsRE),
			func(pt *histPoint, phase string, v float64) {
				switch phase {
				case "running":
					pt.gw.Running += v
				case "provisioning":
					pt.gw.Provisioning += v
				case "failed":
					pt.gw.Failed += v
				}
			},
		)
		// Sandbox total across clusters, deduping scrape replicas with the same inner
		// max-by-(cluster,instance,gateway) the instant snapshot uses before summing.
		rangeInto(
			fmt.Sprintf("sum by (%s) (max by (%s,%s,gateway) (%s{%s=~%q}))",
				p.instLabel, p.clusterLabel, p.instLabel, p.sandboxMetric, p.instLabel, nsRE),
			func(pt *histPoint, _ string, v float64) { pt.sandbox += v },
		)
		// Registered users and rolling 7-day unique logins.
		rangeInto(
			fmt.Sprintf("sum by (%s) (%s{%s=~%q})", p.instLabel, p.userMetric, p.instLabel, nsRE),
			func(pt *histPoint, _ string, v float64) { pt.users += v },
		)
		rangeInto(
			fmt.Sprintf("sum by (%s) (%s{%s=~%q})", p.instLabel, p.userLoginsMetric, p.instLabel, nsRE),
			func(pt *histPoint, _ string, v float64) { pt.logins += v },
		)

		// Emit each instance's series aligned to its sorted timestamp axis. A timestamp
		// present in any series becomes a column in all of them (missing -> zero), so
		// HistoryTimes[i] indexes the same moment in every array.
		for inst, steps := range byTS {
			tss := make([]int64, 0, len(steps))
			for ts := range steps {
				tss = append(tss, ts)
			}
			sort.Slice(tss, func(i, j int) bool { return tss[i] < tss[j] })
			f := get(inst)
			f.HistoryTimes = tss
			f.GatewayHistory = make([]GatewayHistorySample, 0, len(tss))
			f.SandboxHistory = make([]float64, 0, len(tss))
			f.UserHistory = make([]float64, 0, len(tss))
			f.LoginsHistory = make([]float64, 0, len(tss))
			for _, ts := range tss {
				pt := steps[ts]
				f.GatewayHistory = append(f.GatewayHistory, pt.gw)
				f.SandboxHistory = append(f.SandboxHistory, pt.sandbox)
				f.UserHistory = append(f.UserHistory, pt.users)
				f.LoginsHistory = append(f.LoginsHistory, pt.logins)
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
