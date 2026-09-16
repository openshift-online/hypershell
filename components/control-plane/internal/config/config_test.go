package config

import (
	"os"
	"testing"
)

// TestLoadGatewayReconcileWorkers covers the GATEWAY_RECONCILE_WORKERS contract:
// unset uses the bounded default; a valid positive integer is honored; and an
// invalid or non-positive value warns and falls back to the default rather than
// disabling the pool. The resolved count is always >= 1.
func TestLoadGatewayReconcileWorkers(t *testing.T) {
	t.Setenv("HYPERSHELL_GRPC_SERVER_ADDR", "localhost:9000")

	cases := []struct {
		name     string
		envValue string
		envUnset bool
		want     int
	}{
		{name: "unset uses default", envUnset: true, want: DefaultGatewayReconcileWorkers},
		{name: "empty uses default", envValue: "", want: DefaultGatewayReconcileWorkers},
		{name: "valid value honored", envValue: "8", want: 8},
		{name: "one is honored", envValue: "1", want: 1},
		{name: "non-numeric falls back", envValue: "abc", want: DefaultGatewayReconcileWorkers},
		{name: "zero falls back", envValue: "0", want: DefaultGatewayReconcileWorkers},
		{name: "negative falls back", envValue: "-3", want: DefaultGatewayReconcileWorkers},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.envUnset {
				if err := os.Unsetenv("GATEWAY_RECONCILE_WORKERS"); err != nil {
					t.Fatalf("unset GATEWAY_RECONCILE_WORKERS: %v", err)
				}
			} else {
				t.Setenv("GATEWAY_RECONCILE_WORKERS", tc.envValue)
			}

			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load() with GATEWAY_RECONCILE_WORKERS=%q: unexpected error: %v", tc.envValue, err)
			}
			if cfg.GatewayReconcileWorkers != tc.want {
				t.Fatalf("Load() with GATEWAY_RECONCILE_WORKERS=%q: GatewayReconcileWorkers = %d, want %d", tc.envValue, cfg.GatewayReconcileWorkers, tc.want)
			}
			if cfg.GatewayReconcileWorkers < 1 {
				t.Fatalf("GatewayReconcileWorkers = %d, want >= 1", cfg.GatewayReconcileWorkers)
			}
		})
	}
}

func TestGetEnvInt(t *testing.T) {
	const key = "TEST_GET_ENV_INT"
	tests := []struct {
		name     string
		set      bool
		value    string
		fallback int
		min      int
		want     int
	}{
		{name: "unset uses fallback", set: false, fallback: 4, min: 1, want: 4},
		{name: "empty uses fallback", set: true, value: "", fallback: 4, min: 1, want: 4},
		{name: "valid parses", set: true, value: "12", fallback: 4, min: 1, want: 12},
		{name: "at minimum honored", set: true, value: "1", fallback: 4, min: 1, want: 1},
		{name: "below minimum falls back", set: true, value: "0", fallback: 4, min: 1, want: 4},
		{name: "negative falls back", set: true, value: "-5", fallback: 4, min: 1, want: 4},
		{name: "non-numeric falls back", set: true, value: "x", fallback: 4, min: 1, want: 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.set {
				t.Setenv(key, tt.value)
			} else if err := os.Unsetenv(key); err != nil {
				t.Fatalf("unset %s: %v", key, err)
			}
			if got := getEnvInt(key, tt.fallback, tt.min); got != tt.want {
				t.Fatalf("getEnvInt(%q, %d, %d) = %d, want %d", tt.value, tt.fallback, tt.min, got, tt.want)
			}
		})
	}
}
