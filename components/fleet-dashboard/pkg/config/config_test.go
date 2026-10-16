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
func TestLoadSDLCDefaultsAndOverrides(t *testing.T) {
	t.Setenv("FD_PROM_URL", "https://prom.example")
	t.Setenv("FD_PROMOTER_NAMESPACE", "ns")
	t.Setenv("FD_AUTH_ENABLED", "false")
	t.Setenv("FD_SLE_SPEC", "10h")

	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.RefreshSDLC != 120*time.Second {
		t.Errorf("default RefreshSDLC = %v, want 120s", c.RefreshSDLC)
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

func TestLoadAuthRequiresSAR(t *testing.T) {
	t.Setenv("FD_PROM_URL", "https://prom.example")
	t.Setenv("FD_PROMOTER_NAMESPACE", "ns")
	t.Setenv("FD_AUTH_ENABLED", "true")
	// No SAR attributes set.
	if _, err := Load(); err == nil {
		t.Fatal("expected error when auth enabled without SAR attributes")
	}
}
