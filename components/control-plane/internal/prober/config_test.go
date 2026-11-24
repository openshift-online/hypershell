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

func TestLoadDefaultAuthModeIsClientAssertion(t *testing.T) {
	// Unset HYPERSHELL_PROBE_AUTH_MODE defaults to client-assertion (OBS-PRB-06).
	cfg := Load()
	if cfg.AuthMode != AuthModeClientAssertion {
		t.Errorf("auth mode = %q, want default %q", cfg.AuthMode, AuthModeClientAssertion)
	}
}

func TestLoadAuthModeSelection(t *testing.T) {
	cases := []struct {
		set  string
		want string
	}{
		{"client-assertion", AuthModeClientAssertion},
		{"token-exchange", AuthModeTokenExchange},
		// An unknown value falls back to the default rather than failing startup.
		{"bogus", AuthModeClientAssertion},
	}
	for _, tc := range cases {
		t.Run(tc.set, func(t *testing.T) {
			t.Setenv("HYPERSHELL_PROBE_AUTH_MODE", tc.set)
			if got := Load().AuthMode; got != tc.want {
				t.Errorf("auth mode = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestAuthenticatorSelectsConfiguredMode covers OBS-PRB-06: the mode is chosen by
// configuration, not code, and an unconfigured mode yields a nil authenticator.
func TestAuthenticatorSelectsConfiguredMode(t *testing.T) {
	clientAssertion := ClientAssertionConfig{TokenEndpoint: "e", ClientID: "c", SATokenFile: "f"}
	tokenExchange := TokenExchangeConfig{TokenEndpoint: "e", ClientID: "c", SATokenFile: "f"}

	t.Run("client-assertion", func(t *testing.T) {
		cfg := &Config{AuthMode: AuthModeClientAssertion, ClientAssertion: clientAssertion}
		if _, ok := cfg.Authenticator().(*ClientAssertionAuthenticator); !ok {
			t.Errorf("client-assertion mode = %T, want *ClientAssertionAuthenticator", cfg.Authenticator())
		}
	})
	t.Run("token-exchange", func(t *testing.T) {
		cfg := &Config{AuthMode: AuthModeTokenExchange, TokenExchange: tokenExchange}
		if _, ok := cfg.Authenticator().(*TokenExchanger); !ok {
			t.Errorf("token-exchange mode = %T, want *TokenExchanger", cfg.Authenticator())
		}
	})
	t.Run("unconfigured is nil", func(t *testing.T) {
		cfg := &Config{AuthMode: AuthModeClientAssertion}
		if a := cfg.Authenticator(); a != nil {
			t.Errorf("unconfigured authenticator = %v, want nil", a)
		}
	})
}

func TestClientAssertionConfigEnabled(t *testing.T) {
	cases := []struct {
		name string
		cfg  ClientAssertionConfig
		want bool
	}{
		{"all set", ClientAssertionConfig{TokenEndpoint: "e", ClientID: "c", SATokenFile: "f"}, true},
		{"missing endpoint", ClientAssertionConfig{ClientID: "c", SATokenFile: "f"}, false},
		// ClientID is optional in client-assertion mode: the prober authenticates
		// with the assertion alone and never sends a client_id (OBS-PRB-06).
		{"missing client is still enabled", ClientAssertionConfig{TokenEndpoint: "e", SATokenFile: "f"}, true},
		{"missing file", ClientAssertionConfig{TokenEndpoint: "e", ClientID: "c"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.cfg.Enabled(); got != tc.want {
				t.Errorf("Enabled() = %t, want %t", got, tc.want)
			}
		})
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
