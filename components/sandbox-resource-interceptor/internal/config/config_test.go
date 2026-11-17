package config

import "testing"

func TestLoadDefaults(t *testing.T) {
	t.Setenv("SANDBOX_RESOURCE_INTERCEPTOR_SOCKET", "")
	t.Setenv("SANDBOX_RESOURCE_INTERCEPTOR_PUBLIC_KEY", "")

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
}

func TestLoadOverrides(t *testing.T) {
	t.Setenv("SANDBOX_RESOURCE_INTERCEPTOR_SOCKET", "/tmp/other.sock")
	t.Setenv("SANDBOX_RESOURCE_INTERCEPTOR_PUBLIC_KEY", "/tmp/key.pem")

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
}

func TestLoadRejectsBadSocketPath(t *testing.T) {
	t.Setenv("SANDBOX_RESOURCE_INTERCEPTOR_SOCKET", "relative/path.sock")
	t.Setenv("SANDBOX_RESOURCE_INTERCEPTOR_PUBLIC_KEY", "")

	if _, err := Load(); err == nil {
		t.Error("relative socket path accepted")
	}
}
