package config

import (
	"testing"

	"github.com/openshift-online/hypershell/components/sandbox-resource-interceptor/internal/policy"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("SANDBOX_RESOURCE_INTERCEPTOR_SOCKET", "")
	t.Setenv("SANDBOX_RESOURCE_INTERCEPTOR_PUBLIC_KEY", "")
	t.Setenv("SANDBOX_RESOURCE_INTERCEPTOR_MODE", "")
	t.Setenv("SANDBOX_RESOURCE_INTERCEPTOR_METRICS_ADDR", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.SocketPath != DefaultSocketPath {
		t.Errorf("SocketPath = %q, want %q", cfg.SocketPath, DefaultSocketPath)
	}
	if cfg.PublicKeyPath != DefaultPublicKeyPath {
		t.Errorf("PublicKeyPath = %q, want %q", cfg.PublicKeyPath, DefaultPublicKeyPath)
	}
	if cfg.Mode != policy.ModeOff {
		t.Errorf("Mode = %q, want off: a sidecar without explicit configuration must not change behaviour", cfg.Mode)
	}
	if cfg.MetricsAddr != DefaultMetricsAddr {
		t.Errorf("MetricsAddr = %q, want %q", cfg.MetricsAddr, DefaultMetricsAddr)
	}
}

func TestLoadOverrides(t *testing.T) {
	t.Setenv("SANDBOX_RESOURCE_INTERCEPTOR_SOCKET", "/tmp/other.sock")
	t.Setenv("SANDBOX_RESOURCE_INTERCEPTOR_PUBLIC_KEY", "/tmp/key.pem")
	t.Setenv("SANDBOX_RESOURCE_INTERCEPTOR_MODE", "enforce")
	t.Setenv("SANDBOX_RESOURCE_INTERCEPTOR_METRICS_ADDR", "127.0.0.1:9999")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.SocketPath != "/tmp/other.sock" {
		t.Errorf("SocketPath = %q", cfg.SocketPath)
	}
	if cfg.PublicKeyPath != "/tmp/key.pem" {
		t.Errorf("PublicKeyPath = %q", cfg.PublicKeyPath)
	}
	if cfg.Mode != policy.ModeEnforce {
		t.Errorf("Mode = %q, want enforce", cfg.Mode)
	}
	if cfg.MetricsAddr != "127.0.0.1:9999" {
		t.Errorf("MetricsAddr = %q", cfg.MetricsAddr)
	}
}

func TestLoadRejectsBadSocketPath(t *testing.T) {
	t.Setenv("SANDBOX_RESOURCE_INTERCEPTOR_SOCKET", "relative/path.sock")
	t.Setenv("SANDBOX_RESOURCE_INTERCEPTOR_PUBLIC_KEY", "")

	if _, err := Load(); err == nil {
		t.Error("relative socket path accepted")
	}
}

func TestLoadRejectsBadMode(t *testing.T) {
	t.Setenv("SANDBOX_RESOURCE_INTERCEPTOR_SOCKET", "")
	t.Setenv("SANDBOX_RESOURCE_INTERCEPTOR_PUBLIC_KEY", "")
	t.Setenv("SANDBOX_RESOURCE_INTERCEPTOR_MODE", "audit")

	if _, err := Load(); err == nil {
		t.Error("unknown mode accepted: there is no audit mode")
	}
}

func TestLoadRejectsBadMetricsAddr(t *testing.T) {
	t.Setenv("SANDBOX_RESOURCE_INTERCEPTOR_SOCKET", "")
	t.Setenv("SANDBOX_RESOURCE_INTERCEPTOR_PUBLIC_KEY", "")
	t.Setenv("SANDBOX_RESOURCE_INTERCEPTOR_METRICS_ADDR", "not an address")

	if _, err := Load(); err == nil {
		t.Error("invalid metrics address accepted")
	}
}
