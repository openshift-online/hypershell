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

func TestLoadAuthRequiresSAR(t *testing.T) {
	t.Setenv("FD_PROM_URL", "https://prom.example")
	t.Setenv("FD_PROMOTER_NAMESPACE", "ns")
	t.Setenv("FD_AUTH_ENABLED", "true")
	// No SAR attributes set.
	if _, err := Load(); err == nil {
		t.Fatal("expected error when auth enabled without SAR attributes")
	}
}
