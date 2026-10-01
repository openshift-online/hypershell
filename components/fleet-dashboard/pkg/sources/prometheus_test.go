package sources

import (
	"context"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
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
