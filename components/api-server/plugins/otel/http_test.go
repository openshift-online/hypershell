package otel

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// TestClassifyRequestMethod verifies the OBS-SLI-08 read/write classification:
// safe methods are reads, every other method is a write, and the result is always
// one of the two bounded values.
func TestClassifyRequestMethod(t *testing.T) {
	tests := []struct {
		name   string
		method string
		want   string
	}{
		{name: "GET is a read", method: http.MethodGet, want: requestClassRead},
		{name: "HEAD is a read", method: http.MethodHead, want: requestClassRead},
		{name: "OPTIONS is a read", method: http.MethodOptions, want: requestClassRead},
		{name: "POST is a write", method: http.MethodPost, want: requestClassWrite},
		{name: "PUT is a write", method: http.MethodPut, want: requestClassWrite},
		{name: "PATCH is a write", method: http.MethodPatch, want: requestClassWrite},
		{name: "DELETE is a write", method: http.MethodDelete, want: requestClassWrite},
		{name: "CONNECT is a write", method: http.MethodConnect, want: requestClassWrite},
		{name: "TRACE is a write", method: http.MethodTrace, want: requestClassWrite},
		{name: "unknown method is a write", method: "WEIRD", want: requestClassWrite},
		{name: "empty method is a write", method: "", want: requestClassWrite},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyRequestMethod(tc.method)
			if got != tc.want {
				t.Fatalf("classifyRequestMethod(%q) = %q, want %q", tc.method, got, tc.want)
			}
			if got != requestClassRead && got != requestClassWrite {
				t.Fatalf("classifyRequestMethod(%q) = %q, not one of the two bounded values", tc.method, got)
			}
		})
	}
}

// TestHTTPLatencySLOBoundaries verifies the OBS-SLI-07 bucket boundaries include
// the SLO thresholds 0.5 and 2 seconds exactly, are strictly increasing, and that
// a fresh (non-aliased) slice is returned on each call.
func TestHTTPLatencySLOBoundaries(t *testing.T) {
	b := httpLatencySLOBoundaries()

	want := []float64{0.025, 0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10}
	if len(b) != len(want) {
		t.Fatalf("got %d boundaries, want %d", len(b), len(want))
	}
	for i := range want {
		if b[i] != want[i] {
			t.Fatalf("boundary[%d] = %v, want %v", i, b[i], want[i])
		}
	}

	if !containsExactly(b, 0.5) {
		t.Fatalf("boundaries %v do not include 0.5 exactly", b)
	}
	if !containsExactly(b, 2) {
		t.Fatalf("boundaries %v do not include 2 exactly", b)
	}

	for i := 1; i < len(b); i++ {
		if b[i] <= b[i-1] {
			t.Fatalf("boundaries not strictly increasing at index %d: %v", i, b)
		}
	}

	// A fresh slice must be returned each call so callers cannot mutate shared state.
	other := httpLatencySLOBoundaries()
	other[0] = -1
	if b[0] == -1 {
		t.Fatalf("httpLatencySLOBoundaries returned an aliased slice")
	}
}

func containsExactly(s []float64, v float64) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// durationSampleAttrs collects the http.server.request.duration histogram from the
// reader and returns the attribute set of its single data point. It fails the test
// if the instrument is missing or records more than one data point, so a single
// served request maps to a single sample with a deterministic attribute set.
func durationSampleAttrs(t *testing.T, reader *sdkmetric.ManualReader) attribute.Set {
	t.Helper()

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("collecting metrics: %v", err)
	}

	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != httpServerRequestDurationName {
				continue
			}
			hist, ok := m.Data.(metricdata.Histogram[float64])
			if !ok {
				t.Fatalf("instrument %q is %T, want Histogram[float64]", m.Name, m.Data)
			}
			if len(hist.DataPoints) != 1 {
				t.Fatalf("instrument %q recorded %d data points, want exactly 1", m.Name, len(hist.DataPoints))
			}
			return hist.DataPoints[0].Attributes
		}
	}

	t.Fatalf("instrument %q was not recorded", httpServerRequestDurationName)
	return attribute.Set{}
}

// serveThroughOTel wires the production HTTP instrumentation the same way plugin.go
// does -- the otelHTTPHandler creates the server span and the Labeler, and
// routeTagMiddleware runs inside the mux router where mux.CurrentRoute holds the
// templated path -- then serves one GET request against requestPath and returns
// the recorded metrics reader. Only /api/hypershell/v1/gateways/{id} is a
// registered route, so any other path matches no route.
func serveThroughOTel(t *testing.T, requestPath string) *sdkmetric.ManualReader {
	t.Helper()

	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	prev := otel.GetMeterProvider()
	otel.SetMeterProvider(mp)
	t.Cleanup(func() { otel.SetMeterProvider(prev) })

	router := mux.NewRouter()
	router.Use(routeTagMiddleware)
	router.HandleFunc("/api/hypershell/v1/gateways/{id}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}).Methods(http.MethodGet)

	handler := otelHTTPHandler(router)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, requestPath, nil)
	handler.ServeHTTP(rec, req)

	return reader
}

// TestRouteTagMiddlewareRecordsTemplatedRouteOnLatencyMetric verifies OBS-SLI-10.01:
// a request that matches a registered route records http.route on the latency
// histogram as the templated route, never the raw path with the concrete id. It
// also verifies the OBS-SLI-10 "complement, not replace" guarantee: the route
// attribute coexists with the OBS-SLI-08 read/write request-class attribute on the
// same sample.
func TestRouteTagMiddlewareRecordsTemplatedRouteOnLatencyMetric(t *testing.T) {
	reader := serveThroughOTel(t, "/api/hypershell/v1/gateways/abc123")
	attrs := durationSampleAttrs(t, reader)

	route, ok := attrs.Value(semconv.HTTPRouteKey)
	if !ok {
		t.Fatalf("latency sample carries no %q attribute, want the templated route", semconv.HTTPRouteKey)
	}
	const want = "/api/hypershell/v1/gateways/{id}"
	if route.AsString() != want {
		t.Fatalf("http.route = %q, want templated route %q", route.AsString(), want)
	}
	if route.AsString() == "/api/hypershell/v1/gateways/abc123" {
		t.Fatalf("http.route recorded the raw path, want the templated route")
	}

	// OBS-SLI-10: the route attribute complements, and does not replace, the
	// OBS-SLI-08 read/write request-class attribute on the same sample.
	class, ok := attrs.Value(requestClassAttr)
	if !ok {
		t.Fatalf("latency sample dropped the %q attribute; http.route must complement, not replace it", requestClassAttr)
	}
	if class.AsString() != requestClassRead {
		t.Fatalf("http.request.class = %q for a GET, want %q", class.AsString(), requestClassRead)
	}
}

// TestRouteTagMiddlewareOmitsRouteForUnmatchedPath verifies OBS-SLI-10.02: a
// request whose path matches no registered route records no http.route attribute,
// so cardinality stays bounded by the registered routes.
func TestRouteTagMiddlewareOmitsRouteForUnmatchedPath(t *testing.T) {
	reader := serveThroughOTel(t, "/api/hypershell/v1/does-not-exist")
	attrs := durationSampleAttrs(t, reader)

	if route, ok := attrs.Value(semconv.HTTPRouteKey); ok {
		t.Fatalf("unmatched path recorded http.route = %q, want no route attribute", route.AsString())
	}
}
