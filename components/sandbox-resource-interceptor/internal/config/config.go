// Package config loads the sandbox-resource-interceptor sidecar configuration
// from its environment. The service is stateless: policy values and mode are
// fixed at build time in this slice and arrive as configuration only when the
// policy engine lands.
package config

import (
	"fmt"
	"os"
)

// Defaults matching the gateway-side registration rendered by the control
// plane: grpc_endpoint = "unix:///run/openshell/interceptors/sandbox-resources.sock".
const (
	// DefaultSocketPath is the gateway-local Unix socket the sidecar serves on.
	DefaultSocketPath = "/run/openshell/interceptors/sandbox-resources.sock"
	// DefaultPublicKeyPath is where the gateway's JWT Secret mounts its public
	// key into the sidecar container (gateway_jwt.public_key_path).
	DefaultPublicKeyPath = "/etc/openshell/interceptor/gateway-jwt-public.pem"
)

// Config is the resolved sidecar configuration.
type Config struct {
	// SocketPath is the absolute path of the Unix socket to serve on.
	SocketPath string
	// PublicKeyPath is the PEM file holding the gateway's Ed25519 public key.
	PublicKeyPath string
}

// Load reads the configuration from the environment and validates it.
func Load() (Config, error) {
	cfg := Config{
		SocketPath:    envOr("SANDBOX_RESOURCE_INTERCEPTOR_SOCKET", DefaultSocketPath),
		PublicKeyPath: envOr("SANDBOX_RESOURCE_INTERCEPTOR_PUBLIC_KEY", DefaultPublicKeyPath),
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
	return cfg, nil
}

func envOr(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok && value != "" {
		return value
	}
	return fallback
}
