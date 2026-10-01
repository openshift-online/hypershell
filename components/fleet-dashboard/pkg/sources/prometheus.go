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
	base      string
	tokenFile string
	metric    string
	instLabel string
	client    *http.Client
	logger    *slog.Logger
}

// NewPrometheus builds a Prometheus source from config.
func NewPrometheus(c *config.Config) *Prometheus {
	tr := &http.Transport{}
	if c.PromInsecure {
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // opt-in for self-signed in-cluster endpoints
	}
	return &Prometheus{
		base:      strings.TrimRight(c.PromURL, "/"),
		tokenFile: c.PromTokenFile,
		metric:    c.GatewayMetric,
		instLabel: c.InstanceLabel,
		client:    &http.Client{Timeout: 20 * time.Second, Transport: tr},
		logger:    slog.Default(),
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
}

// RateStats is a rate + error% + p95 latency triple.
type RateStats struct {
	Rate     float64 `json:"rate"`
	ErrorPct float64 `json:"errorPct"`
	P95Ms    float64 `json:"p95Ms"`
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
	byNS(fmt.Sprintf("sum by (%s) (hypershell_users_registered_total{%s=~%q})", p.instLabel, p.instLabel, nsRE),
		func(f *InstanceFleet, v float64) { f.Users = v })
	byNS(fmt.Sprintf("1000 * histogram_quantile(0.95, sum by (%s,le) (rate(gateway_provision_duration_seconds_bucket{%s=~%q}[30m])))", p.instLabel, p.instLabel, nsRE),
		func(f *InstanceFleet, v float64) { f.ProvisionP95Ms = v })

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
