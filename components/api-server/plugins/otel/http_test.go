package otel

import (
	"net/http"
	"testing"
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
