package config

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

// DefaultGatewayDatabaseAdminDir is where the controller Deployment mounts the
// PostgreSQL admin credentials Secret used to provision gateway databases. One
// file per Secret key: host, port, user, password, sslrootcert, and optionally
// dbname and sslmode. See specs/platform/openshell-gateway-database.spec.md.
const DefaultGatewayDatabaseAdminDir = "/etc/hypershell/gateway-database"

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

	// ManagedClusterName is the human-readable name this control plane
	// registers under (e.g. hyp0-mc1, or local-kind in the Kind overlay), unique
	// per control plane. It is REQUIRED: every control plane is a registered
	// spoke, and its cluster_id is resolved at startup by calling
	// POST /managed_clusters/registration with this name. There is no
	// unregistered (unfiltered) mode, and a cluster id is never read from
	// configuration. Sourced from HYPERSHELL_MANAGED_CLUSTER_NAME.
	// See specs/platform/control-plane.spec.md ("Mandatory Cluster Identity").
	ManagedClusterName string

	// OIDC client credentials used both for registration and as per-RPC bearer
	// credentials on every gRPC call (including the watch streams). All three
	// are REQUIRED. OIDCTokenEndpoint optionally overrides the token endpoint
	// discovered from the issuer.
	OIDCIssuer        string
	OIDCClientID      string
	OIDCClientSecret  string
	OIDCTokenEndpoint string

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

	// GatewayDatabaseAdminDir is the directory holding the mounted PostgreSQL
	// admin credentials for gateway database provisioning. Sourced from
	// GATEWAY_DATABASE_ADMIN_DIR; the controller refuses to start unless it holds
	// a complete, verify-full credential set (see gateway.ValidateAdminCredentialsDir).
	GatewayDatabaseAdminDir string
}

func Load() (*Config, error) {
	cfg := &Config{
		GRPCServerAddr:                   getEnv("HYPERSHELL_GRPC_SERVER_ADDR", "localhost:9000"),
		APIServerURL:                     getEnv("HYPERSHELL_API_SERVER_URL", "http://localhost:8000"),
		Namespace:                        getEnv("HYPERSHELL_NAMESPACE", "hypershell"),
		LogLevel:                         strings.ToLower(getEnv("HYPERSHELL_LOG_LEVEL", "info")),
		ManagedClusterName:               getEnv("HYPERSHELL_MANAGED_CLUSTER_NAME", ""),
		OIDCIssuer:                       getEnv("OIDC_ISSUER", ""),
		OIDCClientID:                     getEnv("OIDC_CLIENT_ID", ""),
		OIDCClientSecret:                 getEnv("OIDC_CLIENT_SECRET", ""),
		OIDCTokenEndpoint:                getEnv("OIDC_TOKEN_ENDPOINT", ""),
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

		GatewayDatabaseAdminDir: getEnv("GATEWAY_DATABASE_ADMIN_DIR", DefaultGatewayDatabaseAdminDir),
	}

	if cfg.GRPCServerAddr == "" {
		return nil, fmt.Errorf("HYPERSHELL_GRPC_SERVER_ADDR is required")
	}

	// Every control plane is a registered spoke: without a cluster name and OIDC
	// client credentials it cannot register, so it cannot scope its watches, and
	// it must not start at all. Check in a fixed order so the error always names
	// the first missing variable.
	required := []struct{ name, value string }{
		{"HYPERSHELL_MANAGED_CLUSTER_NAME", cfg.ManagedClusterName},
		{"OIDC_ISSUER", cfg.OIDCIssuer},
		{"OIDC_CLIENT_ID", cfg.OIDCClientID},
		{"OIDC_CLIENT_SECRET", cfg.OIDCClientSecret},
	}
	for _, r := range required {
		if strings.TrimSpace(r.value) == "" {
			return nil, fmt.Errorf("%s is required: every control plane registers with the hub as a managed cluster (specs/platform/control-plane.spec.md, Mandatory Cluster Identity)", r.name)
		}
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
