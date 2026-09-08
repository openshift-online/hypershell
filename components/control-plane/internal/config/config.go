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
// DatabaseProviderExternal selects an externally-managed PostgreSQL server;
// the control plane issues DDL in-process and requires no CNPG operator.
const (
	DatabaseProviderDeployment = "deployment"
	DatabaseProviderCNPG       = "cnpg"
	DatabaseProviderExternal   = "external"
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
	// gateway. Sourced from HYPERSHELL_CLUSTER_ID.
	ClusterID string

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

	// DatabaseProvider is the control-plane-wide default ManagedDatabase
	// provider, resolved from DATABASE_PROVIDER by resolveDatabaseProvider.
	// It is always one of DatabaseProviderDeployment, DatabaseProviderCNPG or
	// DatabaseProviderExternal; Load returns an error for any other
	// DATABASE_PROVIDER value instead of silently falling back. Existing ManagedDatabase resources keep
	// reconciling per their own Provider field regardless of this default
	// (see internal/reconciler.ManagedDatabaseReconciler), so gateways backed
	// by CNPG remain compatible even when this default is "deployment".
	DatabaseProvider string
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
		ClusterID:                        getEnv("HYPERSHELL_CLUSTER_ID", ""),
		ServiceAccountProvisionerAddress: getEnv("HYPERSHELL_SERVICE_ACCOUNT_PROVISIONER_BIND_ADDRESS", ""),

		NamespaceGCEnabled:     getEnvBool("GATEWAY_NAMESPACE_GC_ENABLED", true),
		NamespaceGCInterval:    getEnvDuration("GATEWAY_NAMESPACE_GC_INTERVAL", 5*time.Minute),
		NamespaceGCGracePeriod: getEnvDuration("GATEWAY_NAMESPACE_GC_GRACE_PERIOD", 10*time.Minute),

		GatewayReconcileWorkers: getEnvInt("GATEWAY_RECONCILE_WORKERS", DefaultGatewayReconcileWorkers, 1),

		DatabaseProvider: databaseProvider,
	}

	if cfg.GRPCServerAddr == "" {
		return nil, fmt.Errorf("HYPERSHELL_GRPC_SERVER_ADDR is required")
	}

	return cfg, nil
}

// resolveDatabaseProvider validates a raw DATABASE_PROVIDER value into one of
// the three supported providers. Unset or empty means DatabaseProviderDeployment
// (deployment-backed ManagedDatabase placement is the default and requires no
// CNPG APIs); any value other than "deployment", "cnpg" or "external" is a
// startup configuration error rather than a silent fallback.
func resolveDatabaseProvider(raw string) (string, error) {
	switch raw {
	case "", DatabaseProviderDeployment:
		return DatabaseProviderDeployment, nil
	case DatabaseProviderCNPG:
		return DatabaseProviderCNPG, nil
	case DatabaseProviderExternal:
		return DatabaseProviderExternal, nil
	default:
		return "", fmt.Errorf("invalid DATABASE_PROVIDER %q: must be %q, %q, or %q (unset defaults to %q)",
			raw, DatabaseProviderCNPG, DatabaseProviderDeployment, DatabaseProviderExternal, DatabaseProviderDeployment)
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
