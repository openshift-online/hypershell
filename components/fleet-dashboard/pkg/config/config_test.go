package config

import (
	"testing"
	"time"
)

func TestLoadRequiresPromURL(t *testing.T) {
	t.Setenv("FD_PROMOTER_NAMESPACE", "ns")
	t.Setenv("FD_AUTH_ENABLED", "false")
	if _, err := Load(); err == nil {
		t.Fatal("expected error when FD_PROM_URL is unset")
	}
}

func TestLoadDefaultsAndOverrides(t *testing.T) {
	t.Setenv("FD_PROM_URL", "https://prom.example/")
	t.Setenv("FD_PROMOTER_NAMESPACE", "promoter-ns")
	t.Setenv("FD_AUTH_ENABLED", "false")
	t.Setenv("FD_ARGO_NAMESPACES", "a, b ,,c")
	t.Setenv("FD_REFRESH_FLEET", "7s")

	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.PromotionStrategyName != "hypershell" {
		t.Errorf("default strategy name = %q, want hypershell", c.PromotionStrategyName)
	}
	if c.GatewayMetric != "hypershell_gateways_total" {
		t.Errorf("default gateway metric = %q", c.GatewayMetric)
	}
	if got, want := c.ArgoNamespaces, []string{"a", "b", "c"}; len(got) != len(want) {
		t.Fatalf("csv parse = %v, want %v", got, want)
	}
	if c.RefreshFleet != 7*time.Second {
		t.Errorf("RefreshFleet = %v, want 7s", c.RefreshFleet)
	}
}

// TestLoadSDLCDefaultsAndOverrides confirms the SDLC refresh and per-phase SLE
// values default sensibly and are overridable via FD_* (SDLC-VIS-13).
// Verifies: SDLC-VIS-13.01
func TestLoadSDLCDefaultsAndOverrides(t *testing.T) {
	t.Setenv("FD_PROM_URL", "https://prom.example")
	t.Setenv("FD_PROMOTER_NAMESPACE", "ns")
	t.Setenv("FD_AUTH_ENABLED", "false")
	t.Setenv("FD_SLE_SPEC", "10h")

	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.RefreshSDLC != 60*time.Second {
		t.Errorf("default RefreshSDLC = %v, want 60s", c.RefreshSDLC)
	}
	if got := c.SDLCPhaseSLE["spec"]; got != 10*time.Hour {
		t.Errorf("FD_SLE_SPEC override = %v, want 10h", got)
	}
	if got := c.SDLCPhaseSLE["code"]; got != sdlcSLEDefaults["code"] {
		t.Errorf("unset SLE should fall back to default: code = %v, want %v", got, sdlcSLEDefaults["code"])
	}
	for _, phase := range SDLCPhaseKeys {
		if _, ok := c.SDLCPhaseSLE[phase]; !ok {
			t.Errorf("phase %q missing an SLE value", phase)
		}
	}
}

// TestLoadE2EDisabledByDefault confirms the e2e-run feature is off unless a Loki
// URL is configured (e2e-run-log-visibility.spec E2E-LOG-02): an empty
// FD_E2E_LOKI_URL leaves the feature disabled without failing Load.
func TestLoadE2EDisabledByDefault(t *testing.T) {
	t.Setenv("FD_PROM_URL", "https://prom.example")
	t.Setenv("FD_PROMOTER_NAMESPACE", "ns")
	t.Setenv("FD_AUTH_ENABLED", "false")

	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.E2ELokiURL != "" {
		t.Errorf("E2ELokiURL = %q, want empty (feature disabled by default)", c.E2ELokiURL)
	}
	// Tuning values still carry their product-safe defaults.
	if c.E2EHistoryWindow != 168*time.Hour {
		t.Errorf("default E2EHistoryWindow = %v, want 168h", c.E2EHistoryWindow)
	}
	if c.E2ERecentRunLimit != 20 {
		t.Errorf("default E2ERecentRunLimit = %d, want 20", c.E2ERecentRunLimit)
	}
}

// TestLoadE2EFromEnv confirms every e2e value comes from configuration, never
// compiled in (E2E-LOG-02).
// Verifies: E2E-LOG-02.02
func TestLoadE2EFromEnv(t *testing.T) {
	t.Setenv("FD_PROM_URL", "https://prom.example")
	t.Setenv("FD_PROMOTER_NAMESPACE", "ns")
	t.Setenv("FD_AUTH_ENABLED", "false")
	t.Setenv("FD_E2E_LOKI_URL", "http://e2e-loki.svc:3100/")
	t.Setenv("FD_E2E_LOKI_TENANTS", "hub-00, hub-01 ,")
	t.Setenv("FD_E2E_HISTORY_WINDOW", "48h")
	t.Setenv("FD_E2E_RECENT_RUN_LIMIT", "5")

	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.E2ELokiURL != "http://e2e-loki.svc:3100" { // trailing slash trimmed
		t.Errorf("E2ELokiURL = %q", c.E2ELokiURL)
	}
	if got, want := c.E2ELokiTenants, []string{"hub-00", "hub-01"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("E2ELokiTenants = %v, want %v", got, want)
	}
	if c.E2EHistoryWindow != 48*time.Hour {
		t.Errorf("E2EHistoryWindow = %v, want 48h", c.E2EHistoryWindow)
	}
	if c.E2ERecentRunLimit != 5 {
		t.Errorf("E2ERecentRunLimit = %d, want 5", c.E2ERecentRunLimit)
	}
}

func TestLoadAuthRequiresSAR(t *testing.T) {
	t.Setenv("FD_PROM_URL", "https://prom.example")
	t.Setenv("FD_PROMOTER_NAMESPACE", "ns")
	t.Setenv("FD_AUTH_ENABLED", "true")
	// No SAR attributes set.
	if _, err := Load(); err == nil {
		t.Fatal("expected error when auth enabled without SAR attributes")
	}
}
