package config

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	GRPCServerAddr string
	APIServerURL   string
	Namespace      string
	LogLevel       string

	ServiceAccountProvisionerAddress          string
	ServiceAccountProvisionerTLSCertificate   string
	ServiceAccountProvisionerTLSKey           string
	ServiceAccountProvisionerTLSClientCA      string
	ServiceAccountProvisionerExpectedClientCN string

	// NamespaceGCEnabled toggles the periodic garbage collection of orphaned
	// gateway namespaces (HYPERSHELL-78).
	NamespaceGCEnabled bool
	// NamespaceGCInterval is the cadence of the orphan sweep.
	NamespaceGCInterval time.Duration
	// NamespaceGCGracePeriod is how long a namespace must remain orphaned before
	// it is reaped.
	NamespaceGCGracePeriod time.Duration
}

func Load() (*Config, error) {
	cfg := &Config{
		GRPCServerAddr:                            getEnv("HYPERSHELL_GRPC_SERVER_ADDR", "localhost:9000"),
		APIServerURL:                              getEnv("HYPERSHELL_API_SERVER_URL", "http://localhost:8000"),
		Namespace:                                 getEnv("HYPERSHELL_NAMESPACE", "hypershell"),
		LogLevel:                                  strings.ToLower(getEnv("HYPERSHELL_LOG_LEVEL", "info")),
		ServiceAccountProvisionerAddress:          getEnv("HYPERSHELL_SERVICE_ACCOUNT_PROVISIONER_BIND_ADDRESS", ""),
		ServiceAccountProvisionerTLSCertificate:   getEnv("HYPERSHELL_SERVICE_ACCOUNT_PROVISIONER_TLS_CERT", ""),
		ServiceAccountProvisionerTLSKey:           getEnv("HYPERSHELL_SERVICE_ACCOUNT_PROVISIONER_TLS_KEY", ""),
		ServiceAccountProvisionerTLSClientCA:      getEnv("HYPERSHELL_SERVICE_ACCOUNT_PROVISIONER_TLS_CLIENT_CA", ""),
		ServiceAccountProvisionerExpectedClientCN: getEnv("HYPERSHELL_SERVICE_ACCOUNT_PROVISIONER_EXPECTED_CLIENT_CN", "hypershell-api-server"),

		NamespaceGCEnabled:     getEnvBool("GATEWAY_NAMESPACE_GC_ENABLED", true),
		NamespaceGCInterval:    getEnvDuration("GATEWAY_NAMESPACE_GC_INTERVAL", 5*time.Minute),
		NamespaceGCGracePeriod: getEnvDuration("GATEWAY_NAMESPACE_GC_GRACE_PERIOD", 10*time.Minute),
	}

	if cfg.GRPCServerAddr == "" {
		return nil, fmt.Errorf("HYPERSHELL_GRPC_SERVER_ADDR is required")
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(v)
	if err != nil {
		log.Printf("WARN invalid bool for %s=%q, using default %v: %v", key, v, fallback, err)
		return fallback
	}
	return parsed
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(v)
	if err != nil {
		log.Printf("WARN invalid duration for %s=%q, using default %s: %v", key, v, fallback, err)
		return fallback
	}
	return parsed
}
