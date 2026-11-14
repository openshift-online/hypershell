// Package config loads the sandbox-resource-interceptor sidecar configuration
// from its environment. The service is stateless: the policy mode and
// listener locations arrive as configuration and never change at runtime.
package config

import (
	"fmt"
	"net"
	"os"

	"github.com/openshift-online/hypershell/components/sandbox-resource-interceptor/internal/policy"
)

// Defaults matching the gateway-side registration rendered by the control
// plane: grpc_endpoint = "unix:///run/openshell/interceptors/sandbox-resources.sock".
const (
	// DefaultSocketPath is the gateway-local Unix socket the sidecar serves on.
	DefaultSocketPath = "/run/openshell/interceptors/sandbox-resources.sock"
	// DefaultPublicKeyPath is where the gateway's JWT Secret mounts its public
	// key into the sidecar container (gateway_jwt.public_key_path).
	DefaultPublicKeyPath = "/etc/openshell/interceptor/gateway-jwt-public.pem"
	// DefaultMetricsAddr is the TCP address the Prometheus /metrics and
	// /readyz endpoints serve on. Port 9091 matches the vendored chart's
	// interceptor sidecar example (the gateway's own metrics use 9090).
	DefaultMetricsAddr = ":9091"
)

// Config is the resolved sidecar configuration.
type Config struct {
	// SocketPath is the absolute path of the Unix socket to serve on.
	SocketPath string
	// PublicKeyPath is the PEM file holding the gateway's Ed25519 public key.
	PublicKeyPath string
	// Mode is the policy mode: off allows everything unchanged, enforce
	// fills missing requests at 250m/1Gi and denies requests below the
	// floor. The control plane's GATEWAY_RESOURCE_POLICY_MODE flag flows
	// here through the deployment wiring (S4).
	Mode policy.Mode
	// MetricsAddr is the TCP address for the Prometheus metrics and
	// readiness endpoint.
	MetricsAddr string
}

// Load reads the configuration from the environment and validates it.
func Load() (Config, error) {
	cfg := Config{
		SocketPath:    envOr("SANDBOX_RESOURCE_INTERCEPTOR_SOCKET", DefaultSocketPath),
		PublicKeyPath: envOr("SANDBOX_RESOURCE_INTERCEPTOR_PUBLIC_KEY", DefaultPublicKeyPath),
		Mode:          policy.Mode(envOr("SANDBOX_RESOURCE_INTERCEPTOR_MODE", string(policy.ModeOff))),
		MetricsAddr:   envOr("SANDBOX_RESOURCE_INTERCEPTOR_METRICS_ADDR", DefaultMetricsAddr),
	}
	if cfg.SocketPath == "" {
		return Config{}, fmt.Errorf("SANDBOX_RESOURCE_INTERCEPTOR_SOCKET must not be empty")
	}
	if len(cfg.SocketPath) > 0 && cfg.SocketPath[0] != '/' {
		return Config{}, fmt.Errorf("SANDBOX_RESOURCE_INTERCEPTOR_SOCKET must be an absolute path: %q", cfg.SocketPath)
	}
	if cfg.PublicKeyPath == "" {
		return Config{}, fmt.Errorf("SANDBOX_RESOURCE_INTERCEPTOR_PUBLIC_KEY must not be empty")
	}
	switch cfg.Mode {
	case policy.ModeOff, policy.ModeEnforce:
	default:
		return Config{}, fmt.Errorf("SANDBOX_RESOURCE_INTERCEPTOR_MODE must be %q or %q, got %q",
			policy.ModeOff, policy.ModeEnforce, cfg.Mode)
	}
	if cfg.MetricsAddr == "" {
		return Config{}, fmt.Errorf("SANDBOX_RESOURCE_INTERCEPTOR_METRICS_ADDR must not be empty")
	}
	if _, _, err := net.SplitHostPort(cfg.MetricsAddr); err != nil {
		return Config{}, fmt.Errorf("SANDBOX_RESOURCE_INTERCEPTOR_METRICS_ADDR must be a valid TCP address: %w", err)
	}
	return cfg, nil
}

func envOr(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok && value != "" {
		return value
	}
	return fallback
}
