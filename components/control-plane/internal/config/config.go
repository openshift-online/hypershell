package config

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

// Database provider values for DATABASE_PROVIDER. DatabaseProviderDeployment
// is the default (unset or empty DATABASE_PROVIDER resolves to it): a
// standalone PostgreSQL Deployment per gateway, requiring no operator.
// DatabaseProviderCNPG opts into the CloudNativePG-backed placement and
// requires the CNPG operator CRDs to be installed; see
// gateway.RequireCNPGAPI, which the control-plane entrypoint uses to fail
// startup cleanly when they are not.
const (
	DatabaseProviderDeployment = "deployment"
	DatabaseProviderCNPG       = "cnpg"
)

type Config struct {
	GRPCServerAddr string
	APIServerURL   string
	Namespace      string
	LogLevel       string

	// ServiceAccountProvisionerAddress is the in-cluster bind address for the
	// internal service-account provisioner gRPC server. A NetworkPolicy restricts
	// the port to the API server pod, so the channel is plaintext (no mTLS).
	ServiceAccountProvisionerAddress string

	// NamespaceGCEnabled toggles the periodic garbage collection of orphaned
	// gateway namespaces (HYPERSHELL-78).
	NamespaceGCEnabled bool
	// NamespaceGCInterval is the cadence of the orphan sweep.
	NamespaceGCInterval time.Duration
	// NamespaceGCGracePeriod is how long a namespace must remain orphaned before
	// it is reaped.
	NamespaceGCGracePeriod time.Duration

	// DatabaseProvider is the control-plane-wide default ManagedDatabase
	// provider, resolved from DATABASE_PROVIDER by resolveDatabaseProvider.
	DatabaseProvider string

	// Helm chart configuration
	HelmChartPath     string
	HelmChartRegistry string
	HelmChartVersion  string

	// External CA issuer configuration for Route passthrough mode
	ExternalCAIssuerName string
	ExternalCAIssuerKind string
}

func Load() (*Config, error) {
	databaseProvider, err := resolveDatabaseProvider(os.Getenv("DATABASE_PROVIDER"))
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		GRPCServerAddr:                   getEnv("HYPERSHELL_GRPC_SERVER_ADDR", "localhost:9000"),
		APIServerURL:                     getEnv("HYPERSHELL_API_SERVER_URL", "http://localhost:8000"),
		Namespace:                        getEnv("HYPERSHELL_NAMESPACE", "hypershell"),
		LogLevel:                         strings.ToLower(getEnv("HYPERSHELL_LOG_LEVEL", "info")),
		ServiceAccountProvisionerAddress: getEnv("HYPERSHELL_SERVICE_ACCOUNT_PROVISIONER_BIND_ADDRESS", ""),

		NamespaceGCEnabled:     getEnvBool("GATEWAY_NAMESPACE_GC_ENABLED", true),
		NamespaceGCInterval:    getEnvDuration("GATEWAY_NAMESPACE_GC_INTERVAL", 5*time.Minute),
		NamespaceGCGracePeriod: getEnvDuration("GATEWAY_NAMESPACE_GC_GRACE_PERIOD", 10*time.Minute),

		DatabaseProvider: databaseProvider,

		HelmChartPath:     getEnv("HELM_CHART_PATH", "/charts/openshell.tgz"),
		HelmChartRegistry: getEnv("HELM_CHART_REGISTRY", ""),
		HelmChartVersion:  getEnv("HELM_CHART_VERSION", ""),

		ExternalCAIssuerName: getEnv("EXTERNAL_CA_ISSUER_NAME", ""),
		ExternalCAIssuerKind: getEnv("EXTERNAL_CA_ISSUER_KIND", "ClusterIssuer"),
	}

	if cfg.GRPCServerAddr == "" {
		return nil, fmt.Errorf("HYPERSHELL_GRPC_SERVER_ADDR is required")
	}

	// Validate Helm configuration
	if cfg.HelmChartRegistry != "" && cfg.HelmChartVersion == "" {
		return nil, fmt.Errorf("HELM_CHART_VERSION is required when HELM_CHART_REGISTRY is set")
	}

	return cfg, nil
}

// resolveDatabaseProvider validates a raw DATABASE_PROVIDER value into one of
// the two supported providers. Unset or empty means DatabaseProviderDeployment
// (deployment-backed ManagedDatabase placement is the default and requires no
// CNPG APIs); any value other than "deployment" or "cnpg" is a startup
// configuration error rather than a silent fallback to CNPG.
func resolveDatabaseProvider(raw string) (string, error) {
	switch raw {
	case "", DatabaseProviderDeployment:
		return DatabaseProviderDeployment, nil
	case DatabaseProviderCNPG:
		return DatabaseProviderCNPG, nil
	default:
		return "", fmt.Errorf("invalid DATABASE_PROVIDER %q: must be %q or %q (unset defaults to %q)",
			raw, DatabaseProviderCNPG, DatabaseProviderDeployment, DatabaseProviderDeployment)
	}
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
