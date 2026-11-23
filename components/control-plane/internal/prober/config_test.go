package prober

import (
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	// No HYPERSHELL_PROBE_* set: defaults apply (OBS-PRB-08).
	t.Setenv("HYPERSHELL_API_SERVER_URL", "http://hub.example")
	cfg := Load()

	if cfg.Interval != DefaultInterval {
		t.Errorf("interval = %s, want %s", cfg.Interval, DefaultInterval)
	}
	if cfg.Timeout != DefaultTimeout {
		t.Errorf("timeout = %s, want %s", cfg.Timeout, DefaultTimeout)
	}
	if cfg.GraceWindow != DefaultGraceWindow {
		t.Errorf("grace window = %s, want %s", cfg.GraceWindow, DefaultGraceWindow)
	}
	if cfg.GatewayAPIURL != "http://hub.example" {
		t.Errorf("gateway API URL = %q, want it to default to HYPERSHELL_API_SERVER_URL", cfg.GatewayAPIURL)
	}
	// Default interval and a shorter timeout keep a new outage inside one minute
	// (OBS-PRB-09).
	if cfg.Timeout >= cfg.Interval {
		t.Errorf("per-probe timeout %s must be shorter than interval %s (OBS-PRB-08)", cfg.Timeout, cfg.Interval)
	}
	if cfg.Interval > time.Minute {
		t.Errorf("interval %s exceeds one minute; a new outage could not surface within one minute (OBS-PRB-09)", cfg.Interval)
	}
}

func TestLoadTimeoutCorrectedWhenNotShorterThanInterval(t *testing.T) {
	// A timeout >= interval violates OBS-PRB-08; Load corrects it to a fraction of
	// the interval.
	t.Setenv("HYPERSHELL_PROBE_INTERVAL", "30s")
	t.Setenv("HYPERSHELL_PROBE_TIMEOUT", "45s")
	cfg := Load()
	if cfg.Timeout >= cfg.Interval {
		t.Fatalf("timeout %s was not corrected to be shorter than interval %s", cfg.Timeout, cfg.Interval)
	}
}

func TestLoadNonPositiveIntervalFallsBackToDefault(t *testing.T) {
	// A non-positive interval would panic time.NewTicker and crash-loop the pod; a
	// mistuned knob must never stop the prober (OBS-PRB-08).
	for _, bad := range []string{"0s", "-30s"} {
		t.Run(bad, func(t *testing.T) {
			t.Setenv("HYPERSHELL_PROBE_INTERVAL", bad)
			cfg := Load()
			if cfg.Interval != DefaultInterval {
				t.Errorf("interval = %s, want fallback to %s", cfg.Interval, DefaultInterval)
			}
			if cfg.Timeout <= 0 || cfg.Timeout >= cfg.Interval {
				t.Errorf("timeout %s must be positive and shorter than interval %s", cfg.Timeout, cfg.Interval)
			}
		})
	}
}

func TestLoadNormalizesMetricsPath(t *testing.T) {
	// A metrics path without a leading slash would panic the http.ServeMux; it is
	// normalized instead of crashing the prober (OBS-PRB-07).
	t.Setenv("HYPERSHELL_PROBE_METRICS_PATH", "metrics")
	cfg := Load()
	if cfg.MetricsPath != "/metrics" {
		t.Errorf("metrics path = %q, want %q", cfg.MetricsPath, "/metrics")
	}
}

func TestLoadParsesProductionInstances(t *testing.T) {
	t.Setenv("HYPERSHELL_PROBE_API_INSTANCES", "https://a.example, https://b.example ,,https://a.example")
	cfg := Load()
	if len(cfg.ProductionInstances) != 2 {
		t.Fatalf("instances = %v, want 2 de-duplicated entries", cfg.ProductionInstances)
	}
	if cfg.ProductionInstances[0] != "https://a.example" || cfg.ProductionInstances[1] != "https://b.example" {
		t.Errorf("instances = %v, want [a b] in order", cfg.ProductionInstances)
	}
}

func TestTokenExchangeEnabled(t *testing.T) {
	cases := []struct {
		name string
		cfg  TokenExchangeConfig
		want bool
	}{
		{"all set", TokenExchangeConfig{TokenEndpoint: "e", ClientID: "c", SATokenFile: "f"}, true},
		{"missing endpoint", TokenExchangeConfig{ClientID: "c", SATokenFile: "f"}, false},
		{"missing client", TokenExchangeConfig{TokenEndpoint: "e", SATokenFile: "f"}, false},
		{"missing file", TokenExchangeConfig{TokenEndpoint: "e", ClientID: "c"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.cfg.Enabled(); got != tc.want {
				t.Errorf("Enabled() = %t, want %t", got, tc.want)
			}
		})
	}
}
