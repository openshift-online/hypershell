package otel

import (
	"context"
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

func TestStartReconcileSpanDisabled(t *testing.T) {
	prev := enabled
	enabled = false
	defer func() { enabled = prev }()

	ctx := context.Background()
	ctx2, end := StartReconcileSpan(ctx, "Gateway", "reconcile")
	end(nil)

	if ctx2 != ctx {
		t.Error("disabled StartReconcileSpan should return the same context")
	}
}

func TestStartWatchSpanDisabled(t *testing.T) {
	prev := enabled
	enabled = false
	defer func() { enabled = prev }()

	ctx := context.Background()
	ctx2, end := StartWatchSpan(ctx, "Gateway")
	end(nil)

	if ctx2 != ctx {
		t.Error("disabled StartWatchSpan should return the same context")
	}
}

func TestGRPCDialOptionsDisabled(t *testing.T) {
	prev := enabled
	enabled = false
	defer func() { enabled = prev }()

	opts := GRPCDialOptions()
	if opts != nil {
		t.Error("disabled GRPCDialOptions should return nil")
	}
}

func TestInstrumentK8sConfigDisabled(t *testing.T) {
	prev := enabled
	enabled = false
	defer func() { enabled = prev }()

	if InstrumentK8sConfig(nil) != nil {
		t.Error("disabled InstrumentK8sConfig(nil) should return nil")
	}
}
