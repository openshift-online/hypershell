package sources

import (
	"context"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
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

// TestUsersPlane covers the /api/users source: it must discover instances from
// the gateway metric, aggregate the per-user per-cluster gauges by username
// across instances, union the distinct spoke clusters per user, keep a user
// present in only one of the two gauges (missing side zero), keep zero-ownership
// users distinguishable (no clusters), sort by username, and fail the whole
// snapshot when either gauge query fails (a half-built usage table would
// silently under-report).
func TestUsersPlane(t *testing.T) {
	responses := map[string]string{
		"group by (namespace) (hypershell_gateways_total)": `{"status":"success","data":{"resultType":"vector","result":[` +
			`{"metric":{"namespace":"inst-a"},"value":[1,"1"]},` +
			`{"metric":{"namespace":"inst-b"},"value":[1,"1"]}` +
			`]}}`,
		"sum by (username, cluster) (hypershell_users_gateways_total{namespace=~\"inst-a|inst-b\"})": `{"status":"success","data":{"resultType":"vector","result":[` +
			`{"metric":{"username":"alice","cluster":"useast-1"},"value":[1,"5"]},` +
			`{"metric":{"username":"alice","cluster":"euwest-1"},"value":[1,"2"]},` +
			`{"metric":{"username":"bob","cluster":"useast-1"},"value":[1,"1"]},` +
			`{"metric":{"username":"dave"},"value":[1,"0"]}` +
			`]}}`,
		"sum by (username, cluster) (hypershell_users_sandboxes_total{namespace=~\"inst-a|inst-b\"})": `{"status":"success","data":{"resultType":"vector","result":[` +
			`{"metric":{"username":"alice","cluster":"useast-1"},"value":[1,"40"]},` +
			`{"metric":{"username":"alice","cluster":"euwest-1"},"value":[1,"24"]},` +
			`{"metric":{"username":"carol"},"value":[1,"3"]}` +
			`]}}`,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := responses[r.URL.Query().Get("query")]
		if !ok {
			t.Errorf("unexpected query: %q", r.URL.Query().Get("query"))
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	p := &Prometheus{
		base:      srv.URL,
		instLabel: "namespace",
		metric:    "hypershell_gateways_total",
		client:    srv.Client(),
		logger:    slog.Default(),
	}
	got, err := p.Users(context.Background())
	if err != nil {
		t.Fatalf("Users: %v", err)
	}
	envelope, ok := got.(UserUsageEnvelope)
	if !ok {
		t.Fatalf("Users returned %T, want UserUsageEnvelope", got)
	}
	want := []UserUsage{
		{Username: "alice", Gateways: 7, Sandboxes: 64, Clusters: []string{"euwest-1", "useast-1"}},
		{Username: "bob", Gateways: 1, Clusters: []string{"useast-1"}},
		{Username: "carol", Sandboxes: 3, Clusters: []string{}},
		{Username: "dave", Clusters: []string{}},
	}
	if len(envelope.Users) != len(want) {
		t.Fatalf("got %d users, want %d: %+v", len(envelope.Users), len(want), envelope.Users)
	}
	for i, u := range want {
		if !reflect.DeepEqual(envelope.Users[i], u) {
			t.Errorf("users[%d] = %+v, want %+v", i, envelope.Users[i], u)
		}
	}
}

// TestUsersPlaneEmptyInstances confirms an instance-less discovery yields an
// empty list, not nil, so the JSON payload stays a stable shape.
func TestUsersPlaneEmptyInstances(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[]}}`))
	}))
	defer srv.Close()

	p := &Prometheus{
		base:      srv.URL,
		instLabel: "namespace",
		metric:    "hypershell_gateways_total",
		client:    srv.Client(),
		logger:    slog.Default(),
	}
	got, err := p.Users(context.Background())
	if err != nil {
		t.Fatalf("Users: %v", err)
	}
	envelope, ok := got.(UserUsageEnvelope)
	if !ok {
		t.Fatalf("Users returned %T, want UserUsageEnvelope", got)
	}
	if envelope.Users == nil || len(envelope.Users) != 0 {
		t.Fatalf("users = %#v, want empty non-nil slice", envelope.Users)
	}
}

// TestUsersPlaneQueryFailureFailsSnapshot is the anti-silent-under-report guard:
// when either per-user gauge query fails, the plane must return an error so the
// cache serves the last-good snapshot flagged stale instead of a partial table.
func TestUsersPlaneQueryFailureFailsSnapshot(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Query().Get("query"), "hypershell_users_sandboxes_total") {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"status":"error","error":"boom"}`))
			return
		}
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[` +
			`{"metric":{"namespace":"inst-a"},"value":[1,"1"]}]}}`))
	}))
	defer srv.Close()

	p := &Prometheus{
		base:      srv.URL,
		instLabel: "namespace",
		metric:    "hypershell_gateways_total",
		client:    srv.Client(),
		logger:    slog.Default(),
	}
	if _, err := p.Users(context.Background()); err == nil {
		t.Fatal("Users should fail when a per-user gauge query fails")
	}
}
