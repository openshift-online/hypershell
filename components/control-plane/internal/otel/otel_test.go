package otel

import (
	"testing"
)

func TestSamplerRatio(t *testing.T) {
	tests := []struct {
		name string
		env  string
		want float64
	}{
		{"empty uses default", "", defaultSampleArg},
		{"valid 0.5", "0.5", 0.5},
		{"valid 0", "0", 0},
		{"valid 1", "1", 1},
		{"negative falls back", "-0.1", defaultSampleArg},
		{"above one falls back", "1.5", defaultSampleArg},
		{"NaN falls back", "NaN", defaultSampleArg},
		{"Inf falls back", "Inf", defaultSampleArg},
		{"-Inf falls back", "-Inf", defaultSampleArg},
		{"non-numeric falls back", "abc", defaultSampleArg},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("OTEL_TRACES_SAMPLER_ARG", tt.env)
			got := samplerRatio()
			if got != tt.want {
				t.Errorf("samplerRatio() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSanitizeError(t *testing.T) {
	if sanitizeError(nil) != "" {
		t.Error("expected empty string for nil error")
	}
	msg := sanitizeError(errWith("secret openshell-abc/db-password leaked"))
	if msg != "reconcile failed" {
		t.Errorf("expected bounded message, got %q", msg)
	}
}

type errWith string

func (e errWith) Error() string { return string(e) }
