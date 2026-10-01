package sources

import (
	"math"
	"testing"
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
