package sources

import (
	"context"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// TestSampleValue covers the Prometheus value parser, including the non-finite
// guard: PromQL can emit NaN (0/0, empty histogram_quantile) and +/-Inf (x/0),
// none of which encoding/json can marshal. Those must collapse to 0 so a single
// bad sub-query never blanks the whole /api/fleet payload.
func TestSampleValue(t *testing.T) {
	tests := []struct {
		name string
		in   []any
		want float64
	}{
		{"normal", []any{1.0, "233.5"}, 233.5},
		{"zero", []any{1.0, "0"}, 0},
		{"nan", []any{1.0, "NaN"}, 0},
		{"posinf", []any{1.0, "+Inf"}, 0},
		{"neginf", []any{1.0, "-Inf"}, 0},
		{"too short", []any{1.0}, 0},
		{"non-string value", []any{1.0, 2.0}, 0},
		{"unparseable", []any{1.0, "abc"}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sampleValue(tt.in)
			if math.IsNaN(got) || math.IsInf(got, 0) {
				t.Fatalf("sampleValue returned non-finite %v for %q", got, tt.name)
			}
			if got != tt.want {
				t.Errorf("sampleValue(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

// TestQueryRange covers the range-query helper that feeds the per-instance gateway
// sparkline: it must hit /api/v1/query_range, format the step as whole seconds
// (clamped to >=1s), and return the per-series Values time-series intact so the
// caller can map each sample through sampleValue.
func TestQueryRange(t *testing.T) {
	var gotPath string
	var gotQuery url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"matrix","result":[` +
			`{"metric":{"namespace":"inst-a"},"values":[[1,"2"],[2,"NaN"],[3,"5"]]}` +
			`]}}`))
	}))
	defer srv.Close()

	p := &Prometheus{
		base:      srv.URL,
		instLabel: "namespace",
		client:    srv.Client(),
		logger:    slog.Default(),
	}

	end := time.Unix(1000, 0)
	start := end.Add(-15 * time.Minute)
	res, err := p.queryRange(context.Background(), "up", start, end, 90*time.Second)
	if err != nil {
		t.Fatalf("queryRange: %v", err)
	}
	if gotPath != "/api/v1/query_range" {
		t.Errorf("path = %q, want /api/v1/query_range", gotPath)
	}
	if gotQuery.Get("step") != "90s" {
		t.Errorf("step = %q, want 90s", gotQuery.Get("step"))
	}
	if gotQuery.Get("start") != "100" || gotQuery.Get("end") != "1000" {
		t.Errorf("start/end = %q/%q, want 100/1000", gotQuery.Get("start"), gotQuery.Get("end"))
	}
	if len(res) != 1 {
		t.Fatalf("got %d series, want 1", len(res))
	}
	if res[0].Metric["namespace"] != "inst-a" {
		t.Errorf("series label = %q, want inst-a", res[0].Metric["namespace"])
	}
	// The NaN sample must collapse to 0 via sampleValue (non-finite guard).
	want := []float64{2, 0, 5}
	if len(res[0].Values) != len(want) {
		t.Fatalf("got %d samples, want %d", len(res[0].Values), len(want))
	}
	for i, v := range res[0].Values {
		if got := sampleValue(v); got != want[i] {
			t.Errorf("sample[%d] = %v, want %v", i, got, want[i])
		}
	}
}

// TestFleetSandboxesByCluster covers the sandbox sub-query: the per-gateway active
// sandbox gauge must be deduped+summed into per-cluster rows, the per-instance total
// summed from those rows in Go, and the rows ordered busiest-first (ties by name).
// Every other sub-query is stubbed empty so a partial snapshot still assembles.
func TestFleetSandboxesByCluster(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		q := r.URL.Query().Get("query")
		switch {
		case r.URL.Path == "/api/v1/query_range":
			// Gateway-history range query: no history in this test.
			_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"matrix","result":[]}}`))
		case strings.Contains(q, "group by"):
			// Instance discovery.
			_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[` +
				`{"metric":{"namespace":"inst-a"},"value":[1,"1"]}` +
				`]}}`))
		case strings.Contains(q, "active_sandboxes_total"):
			// Two clusters, listed count-ascending on the wire to prove we re-sort
			// busiest-first; "c-tie" shares c1's count to prove the name tie-break.
			_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[` +
				`{"metric":{"namespace":"inst-a","cluster":"c1"},"value":[1,"3"]},` +
				`{"metric":{"namespace":"inst-a","cluster":"c-tie"},"value":[1,"3"]},` +
				`{"metric":{"namespace":"inst-a","cluster":"c2"},"value":[1,"7"]}` +
				`]}}`))
		default:
			_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[]}}`))
		}
	}))
	defer srv.Close()

	p := &Prometheus{
		base:          srv.URL,
		metric:        "hypershell_gateways_total",
		instLabel:     "namespace",
		sandboxMetric: "hypershell_gateways_active_sandboxes_total",
		clusterLabel:  "cluster",
		client:        srv.Client(),
		logger:        slog.Default(),
	}

	out, err := p.Fleet(context.Background())
	if err != nil {
		t.Fatalf("Fleet: %v", err)
	}
	fleet, ok := out.(map[string]InstanceFleet)
	if !ok {
		t.Fatalf("Fleet returned %T, want map[string]InstanceFleet", out)
	}
	f, ok := fleet["inst-a"]
	if !ok {
		t.Fatalf("no inst-a in fleet: %+v", fleet)
	}
	if f.Sandboxes != 13 {
		t.Errorf("Sandboxes total = %d, want 13", f.Sandboxes)
	}
	want := []SandboxClusterCount{{"c2", 7}, {"c-tie", 3}, {"c1", 3}}
	if len(f.SandboxesByCluster) != len(want) {
		t.Fatalf("SandboxesByCluster = %+v, want %+v", f.SandboxesByCluster, want)
	}
	for i, w := range want {
		if f.SandboxesByCluster[i] != w {
			t.Errorf("SandboxesByCluster[%d] = %+v, want %+v", i, f.SandboxesByCluster[i], w)
		}
	}
}

// TestFleetUsers covers the user-count sub-queries: the configurable registered-user
// gauge folds into Users and the rolling unique-login gauge into Logins, each summed
// per instance. Every other sub-query is stubbed empty so a partial snapshot still
// assembles.
func TestFleetUsers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		q := r.URL.Query().Get("query")
		switch {
		case r.URL.Path == "/api/v1/query_range":
			// History range queries: no history in this test.
			_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"matrix","result":[]}}`))
		case strings.Contains(q, "group by"):
			_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[` +
				`{"metric":{"namespace":"inst-a"},"value":[1,"1"]}` +
				`]}}`))
		case strings.Contains(q, "users_registered_total"):
			_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[` +
				`{"metric":{"namespace":"inst-a"},"value":[1,"31"]}` +
				`]}}`))
		case strings.Contains(q, "unique_logins"):
			_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[` +
				`{"metric":{"namespace":"inst-a"},"value":[1,"8"]}` +
				`]}}`))
		default:
			_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[]}}`))
		}
	}))
	defer srv.Close()

	p := &Prometheus{
		base:             srv.URL,
		metric:           "hypershell_gateways_total",
		instLabel:        "namespace",
		sandboxMetric:    "hypershell_gateways_active_sandboxes_total",
		clusterLabel:     "cluster",
		userMetric:       "hypershell_users_registered_total",
		userLoginsMetric: "hypershell_users_unique_logins_last_7_days_total",
		client:           srv.Client(),
		logger:           slog.Default(),
	}

	out, err := p.Fleet(context.Background())
	if err != nil {
		t.Fatalf("Fleet: %v", err)
	}
	fleet, ok := out.(map[string]InstanceFleet)
	if !ok {
		t.Fatalf("Fleet returned %T, want map[string]InstanceFleet", out)
	}
	f, ok := fleet["inst-a"]
	if !ok {
		t.Fatalf("no inst-a in fleet: %+v", fleet)
	}
	if f.Users != 31 {
		t.Errorf("Users = %v, want 31", f.Users)
	}
	if f.Logins != 8 {
		t.Errorf("Logins = %v, want 8", f.Logins)
	}
}

// TestFleetHistoryShared covers the unified history block: the four range series
// (gateway phase mix, sandbox total, users, logins) must land on ONE sorted per-
// instance timestamp axis (HistoryTimes) with every array index-aligned to it, so
// HistoryTimes[i] indexes the same moment in all of them. The gateway phases stack
// per timestamp; a phase absent at a step contributes zero.
func TestFleetHistoryShared(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		q := r.URL.Query().Get("query")
		switch {
		case r.URL.Path == "/api/v1/query_range" && strings.Contains(q, "phase"):
			// Gateway phase mix: running at both steps, failed only at the first.
			_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"matrix","result":[` +
				`{"metric":{"namespace":"inst-a","phase":"Running"},"values":[[100,"5"],[200,"6"]]},` +
				`{"metric":{"namespace":"inst-a","phase":"failed"},"values":[[100,"1"]]}` +
				`]}}`))
		case r.URL.Path == "/api/v1/query_range" && strings.Contains(q, "active_sandboxes_total"):
			_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"matrix","result":[` +
				`{"metric":{"namespace":"inst-a"},"values":[[100,"2"],[200,"3"]]}` +
				`]}}`))
		case r.URL.Path == "/api/v1/query_range" && strings.Contains(q, "users_registered_total"):
			_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"matrix","result":[` +
				`{"metric":{"namespace":"inst-a"},"values":[[100,"7"],[200,"8"]]}` +
				`]}}`))
		case r.URL.Path == "/api/v1/query_range" && strings.Contains(q, "unique_logins"):
			_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"matrix","result":[` +
				`{"metric":{"namespace":"inst-a"},"values":[[100,"4"],[200,"5"]]}` +
				`]}}`))
		case strings.Contains(q, "group by"):
			_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[` +
				`{"metric":{"namespace":"inst-a"},"value":[1,"1"]}` +
				`]}}`))
		default:
			_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[]}}`))
		}
	}))
	defer srv.Close()

	p := &Prometheus{
		base:             srv.URL,
		metric:           "hypershell_gateways_total",
		instLabel:        "namespace",
		sandboxMetric:    "hypershell_gateways_active_sandboxes_total",
		clusterLabel:     "cluster",
		userMetric:       "hypershell_users_registered_total",
		userLoginsMetric: "hypershell_users_unique_logins_last_7_days_total",
		client:           srv.Client(),
		logger:           slog.Default(),
	}

	out, err := p.Fleet(context.Background())
	if err != nil {
		t.Fatalf("Fleet: %v", err)
	}
	f := out.(map[string]InstanceFleet)["inst-a"]

	if got := f.HistoryTimes; len(got) != 2 || got[0] != 100 || got[1] != 200 {
		t.Fatalf("HistoryTimes = %v, want [100 200]", got)
	}
	wantGW := []GatewayHistorySample{{Running: 5, Failed: 1}, {Running: 6}}
	if len(f.GatewayHistory) != len(wantGW) {
		t.Fatalf("GatewayHistory = %+v, want %+v", f.GatewayHistory, wantGW)
	}
	for i, w := range wantGW {
		if f.GatewayHistory[i] != w {
			t.Errorf("GatewayHistory[%d] = %+v, want %+v", i, f.GatewayHistory[i], w)
		}
	}
	// Every series is aligned to the same two-column axis.
	if want := []float64{2, 3}; !floatsEqual(f.SandboxHistory, want) {
		t.Errorf("SandboxHistory = %v, want %v", f.SandboxHistory, want)
	}
	if want := []float64{7, 8}; !floatsEqual(f.UserHistory, want) {
		t.Errorf("UserHistory = %v, want %v", f.UserHistory, want)
	}
	if want := []float64{4, 5}; !floatsEqual(f.LoginsHistory, want) {
		t.Errorf("LoginsHistory = %v, want %v", f.LoginsHistory, want)
	}
}

func floatsEqual(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestQueryRangeStepFloor ensures a sub-second step is clamped to 1s so Prometheus
// never receives a "0s" step (which it rejects).
func TestQueryRangeStepFloor(t *testing.T) {
	var gotStep string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotStep = r.URL.Query().Get("step")
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"matrix","result":[]}}`))
	}))
	defer srv.Close()

	p := &Prometheus{base: srv.URL, client: srv.Client(), logger: slog.Default()}
	if _, err := p.queryRange(context.Background(), "up", time.Unix(0, 0), time.Unix(1, 0), 100*time.Millisecond); err != nil {
		t.Fatalf("queryRange: %v", err)
	}
	if gotStep != "1s" {
		t.Errorf("step = %q, want 1s (floored)", gotStep)
	}
}
