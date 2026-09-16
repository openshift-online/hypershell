package config

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

// DefaultGatewayReconcileWorkers is the fallback size of the gateway reconcile
// worker pool when GATEWAY_RECONCILE_WORKERS is unset or invalid. It matches the
// control plane's historical hardcoded pool size, so an unset variable preserves
// today's cross-gateway provisioning concurrency. See
// specs/platform/gateway-reconcile-concurrency.spec.md.
const DefaultGatewayReconcileWorkers = 4

type Config struct {
	GRPCServerAddr string
	APIServerURL   string
	Namespace      string
	LogLevel       string

	// ClusterID is this control-plane's managed-cluster identity (a Gateway
	// cluster_id / KSUID). When set, the control-plane restricts the gateways it
	// watches, seeds, and health-checks to those whose cluster_id matches, so a
	// managed-cluster spoke only ever provisions its own gateways (the pull
	// model). Empty preserves the single-cluster behaviour of handling every
	// gateway. Sourced from HYPERSHELL_CLUSTER_ID; in production, resolved at
	// runtime via spoke self-registration and should NOT be set in gitops.
	ClusterID string

	// ManagedClusterName is the human-readable name of this spoke cluster, unique
	// per fleet (e.g. hyp0-mc1). When set together with OIDC credentials, the
	// control plane self-registers on startup, resolving ClusterID dynamically.
	// Sourced from HYPERSHELL_MANAGED_CLUSTER_NAME.
	ManagedClusterName string

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

	// GatewayReconcileWorkers bounds how many distinct gateways the control
	// plane provisions concurrently: the size of the gateway reconcile queue
	// worker pool. Work for a single gateway is always serialized; this only
	// caps cross-gateway parallelism, so raising it lets a larger create burst
	// provision at once instead of queueing, while the pool stays a bounded
	// throttle. Resolved from GATEWAY_RECONCILE_WORKERS by Load, always >= 1.
	// See specs/platform/gateway-reconcile-concurrency.spec.md.
	GatewayReconcileWorkers int

	// Helm chart configuration
	HelmChartPath     string
	HelmChartRegistry string
	HelmChartVersion  string

	// External CA issuer configuration for Route passthrough mode
	ExternalCAIssuerName string
	ExternalCAIssuerKind string
}

func Load() (*Config, error) {
	cfg := &Config{
		GRPCServerAddr:                   getEnv("HYPERSHELL_GRPC_SERVER_ADDR", "localhost:9000"),
		APIServerURL:                     getEnv("HYPERSHELL_API_SERVER_URL", "http://localhost:8000"),
		Namespace:                        getEnv("HYPERSHELL_NAMESPACE", "hypershell"),
		LogLevel:                         strings.ToLower(getEnv("HYPERSHELL_LOG_LEVEL", "info")),
		ClusterID:                        getEnv("HYPERSHELL_CLUSTER_ID", ""),
		ManagedClusterName:               getEnv("HYPERSHELL_MANAGED_CLUSTER_NAME", ""),
		ServiceAccountProvisionerAddress: getEnv("HYPERSHELL_SERVICE_ACCOUNT_PROVISIONER_BIND_ADDRESS", ""),

		NamespaceGCEnabled:     getEnvBool("GATEWAY_NAMESPACE_GC_ENABLED", true),
		NamespaceGCInterval:    getEnvDuration("GATEWAY_NAMESPACE_GC_INTERVAL", 5*time.Minute),
		NamespaceGCGracePeriod: getEnvDuration("GATEWAY_NAMESPACE_GC_GRACE_PERIOD", 10*time.Minute),

		GatewayReconcileWorkers: getEnvInt("GATEWAY_RECONCILE_WORKERS", DefaultGatewayReconcileWorkers, 1),

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

// getEnvInt reads an integer environment variable. It falls back to fallback
// when the variable is unset, is not a valid integer, or is below min. min is
// the smallest accepted value (for a worker pool, 1, so the resolved count never
// disables reconciliation). Invalid or out-of-range input logs a warning and
// uses the default rather than failing startup, matching getEnvBool and
// getEnvDuration; a mistuned throttle should not take the control plane down.
func getEnvInt(key string, fallback, min int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(v)
	if err != nil {
		log.Printf("WARN invalid int for %s=%q, using default %d: %v", key, v, fallback, err)
		return fallback
	}
	if parsed < min {
		log.Printf("WARN %s=%d is below the minimum %d, using default %d", key, parsed, min, fallback)
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
