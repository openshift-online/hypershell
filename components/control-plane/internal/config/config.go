package config

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/openshift-online/hypershell/components/control-plane/internal/helm"
	corev1 "k8s.io/api/core/v1"
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
	ManagedClusterName         string
	ManagedClusterCapabilities []string

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

	// DirectoryRefreshInterval is the cadence of the Keycloak realm directory
	// projection refresh (GAM-09). Bounded staleness equals this interval.
	DirectoryRefreshInterval time.Duration

	// GatewayReconcileWorkers bounds how many distinct gateways the control
	// plane provisions concurrently: the size of the gateway reconcile queue
	// worker pool. Work for a single gateway is always serialized; this only
	// caps cross-gateway parallelism, so raising it lets a larger create burst
	// provision at once instead of queueing, while the pool stays a bounded
	// throttle. Resolved from GATEWAY_RECONCILE_WORKERS by Load, always >= 1.
	// See specs/platform/gateway-reconcile-concurrency.spec.md.
	GatewayReconcileWorkers int

	// GRPCKeepaliveTime is how long the gRPC client connection may stay idle
	// before it sends an HTTP/2 keepalive ping to the API server, so a wedged or
	// half-open connection surfaces as a transport error the watchers retry rather
	// than an indefinite silent block. Sourced from HYPERSHELL_GRPC_KEEPALIVE_TIME.
	//
	// The default is 5m to match the API server's keepalive enforcement policy.
	// The server (the rh-trex-ai framework gRPC server) sets no explicit
	// EnforcementPolicy, so it uses the gRPC default MinTime of 5m: a client that
	// pings more often than that on an idle connection is struck and, after a few
	// strikes, the server sends GOAWAY (ENHANCE_YOUR_CALM, too_many_pings) and
	// closes the shared connection -- dropping every watch stream. The ping
	// interval therefore SHALL NOT be shorter than the server's MinTime. The idle
	// watchdog (WatchIdleTimeout), not this keepalive, is the backstop for a stream
	// that stays open but goes silent, so a coarse keepalive is sufficient here.
	// See specs/platform/control-plane.spec.md ("Watch Stream Liveness").
	GRPCKeepaliveTime time.Duration
	// GRPCKeepaliveTimeout is how long the client waits for a keepalive ping ack
	// before it treats the connection as dead. Sourced from
	// HYPERSHELL_GRPC_KEEPALIVE_TIMEOUT.
	GRPCKeepaliveTimeout time.Duration

	// WatchIdleTimeout bounds how long a watch stream may deliver neither an event
	// nor an error before the watcher cancels the attempt and reconnects (re-running
	// its seed/resync). It is the stream-layer backstop for a stream that stays open
	// but goes silent, which gRPC keepalive alone cannot detect because the shared
	// connection is still live. It must be long enough that an ordinarily quiet
	// stream (e.g. a RoleBinding stream with no membership changes) is not torn down
	// needlessly. Sourced from HYPERSHELL_WATCH_IDLE_TIMEOUT; a non-positive value
	// disables the watchdog. See specs/platform/control-plane.spec.md
	// ("Watch Stream Liveness").
	WatchIdleTimeout time.Duration

	// RoleBindingResyncInterval is how often, while the RoleBinding watch stream is
	// connected, the control plane re-lists its cluster's role bindings and
	// reconciles them, so an owner grant is recovered even when a create event is
	// missed or the stream goes silent. The reconciler also seeds on every
	// (re)connect. Sourced from HYPERSHELL_ROLE_BINDING_RESYNC_INTERVAL; a
	// non-positive value disables the periodic resync (the per-connect seed still
	// runs). See specs/platform/control-plane.spec.md ("Role Binding Resync").
	RoleBindingResyncInterval time.Duration

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

	// GatewayResources overrides the gateway container requests and limits
	// passed to the Helm chart. Parsed from GATEWAY_RESOURCES (JSON
	// ResourceRequirements); nil when unset, so the chart values use
	// helm.DefaultGatewayResources. See specs/platform/openshell-gateway.spec.md.
	GatewayResources *corev1.ResourceRequirements

	// SandboxRuntimeClass is the Kubernetes RuntimeClass applied to every sandbox
	// pod created by any gateway on this cluster. Sourced from
	// GATEWAY_SANDBOX_RUNTIME_CLASS; empty string uses the cluster default (runc).
	// Set to "kata" or "kata-remote" for hardware-enforced VM isolation.
	SandboxRuntimeClass string

	// RepositoryImportEnabled toggles the declarative `.hypershell` import
	// reconciler (the RepositoryReconciler). It is OFF by default: the reconciler
	// reads repository subtrees over the GitHub API with the platform GitHub App
	// credential, which must be provisioned (GitHubTokenFile) before the loop can
	// do anything useful, so an unconfigured control plane stays inert rather than
	// erroring every tick. Sourced from HYPERSHELL_REPOSITORY_IMPORT_ENABLED. See
	// specs/platform/control-plane.spec.md ("Declarative `.hypershell` Import
	// Reconciliation").
	RepositoryImportEnabled bool
	// RepositoryImportInterval is the reconcile cadence of the import loop. Sourced
	// from HYPERSHELL_REPOSITORY_IMPORT_INTERVAL.
	RepositoryImportInterval time.Duration
	// GitHubAPIBase overrides the GitHub REST API base URL (for GitHub Enterprise
	// or a test server). Sourced from HYPERSHELL_GITHUB_API_BASE; empty uses the
	// public https://api.github.com.
	GitHubAPIBase string
	// GitHubTokenFile is the path to the mounted platform GitHub App credential
	// (an installation access token delivered as a file, for example by External
	// Secrets). It is the DEFAULT transport the import uses to read a repository
	// subtree; a per-Repository secret_source_id credential takes precedence when
	// set. Sourced from HYPERSHELL_GITHUB_TOKEN_FILE.
	GitHubTokenFile string
}

func Load() (*Config, error) {
	cfg := &Config{
		GRPCServerAddr:                   getEnv("HYPERSHELL_GRPC_SERVER_ADDR", "localhost:9000"),
		APIServerURL:                     getEnv("HYPERSHELL_API_SERVER_URL", "http://localhost:8000"),
		Namespace:                        getEnv("HYPERSHELL_NAMESPACE", "hypershell"),
		LogLevel:                         strings.ToLower(getEnv("HYPERSHELL_LOG_LEVEL", "info")),
		ManagedClusterName:               getEnv("HYPERSHELL_MANAGED_CLUSTER_NAME", ""),
		ManagedClusterCapabilities:       parseCapabilities(getEnv("HYPERSHELL_MANAGED_CLUSTER_CAPABILITIES", "")),
		OIDCIssuer:                       getEnv("OIDC_ISSUER", ""),
		OIDCClientID:                     getEnv("OIDC_CLIENT_ID", ""),
		OIDCClientSecret:                 getEnv("OIDC_CLIENT_SECRET", ""),
		OIDCTokenEndpoint:                getEnv("OIDC_TOKEN_ENDPOINT", ""),
		ServiceAccountProvisionerAddress: getEnv("HYPERSHELL_SERVICE_ACCOUNT_PROVISIONER_BIND_ADDRESS", ""),

		NamespaceGCEnabled:     getEnvBool("GATEWAY_NAMESPACE_GC_ENABLED", true),
		NamespaceGCInterval:    getEnvDuration("GATEWAY_NAMESPACE_GC_INTERVAL", 5*time.Minute),
		NamespaceGCGracePeriod: getEnvDuration("GATEWAY_NAMESPACE_GC_GRACE_PERIOD", 10*time.Minute),

		DirectoryRefreshInterval: getEnvDuration("GATEWAY_DIRECTORY_REFRESH_INTERVAL", 5*time.Minute),

		GatewayReconcileWorkers: getEnvInt("GATEWAY_RECONCILE_WORKERS", DefaultGatewayReconcileWorkers, 1),

		GRPCKeepaliveTime:    getEnvDuration("HYPERSHELL_GRPC_KEEPALIVE_TIME", 5*time.Minute),
		GRPCKeepaliveTimeout: getEnvDuration("HYPERSHELL_GRPC_KEEPALIVE_TIMEOUT", 10*time.Second),

		WatchIdleTimeout:          getEnvDuration("HYPERSHELL_WATCH_IDLE_TIMEOUT", 10*time.Minute),
		RoleBindingResyncInterval: getEnvDuration("HYPERSHELL_ROLE_BINDING_RESYNC_INTERVAL", 5*time.Minute),

		HelmChartPath:     getEnv("HELM_CHART_PATH", "/charts/openshell.tgz"),
		HelmChartRegistry: getEnv("HELM_CHART_REGISTRY", ""),
		HelmChartVersion:  getEnv("HELM_CHART_VERSION", ""),

		ExternalCAIssuerName: getEnv("EXTERNAL_CA_ISSUER_NAME", ""),
		ExternalCAIssuerKind: getEnv("EXTERNAL_CA_ISSUER_KIND", "ClusterIssuer"),

		GatewayDatabaseAdminDir: getEnv("GATEWAY_DATABASE_ADMIN_DIR", DefaultGatewayDatabaseAdminDir),
		SandboxRuntimeClass:     getEnv("GATEWAY_SANDBOX_RUNTIME_CLASS", ""),

		RepositoryImportEnabled:  getEnvBool("HYPERSHELL_REPOSITORY_IMPORT_ENABLED", false),
		RepositoryImportInterval: getEnvDuration("HYPERSHELL_REPOSITORY_IMPORT_INTERVAL", 5*time.Minute),
		GitHubAPIBase:            getEnv("HYPERSHELL_GITHUB_API_BASE", ""),
		GitHubTokenFile:          getEnv("HYPERSHELL_GITHUB_TOKEN_FILE", ""),
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
		{"HYPERSHELL_MANAGED_CLUSTER_CAPABILITIES", strings.Join(cfg.ManagedClusterCapabilities, ",")},
		{"OIDC_ISSUER", cfg.OIDCIssuer},
		{"OIDC_CLIENT_ID", cfg.OIDCClientID},
		{"OIDC_CLIENT_SECRET", cfg.OIDCClientSecret},
	}
	for _, r := range required {
		if strings.TrimSpace(r.value) == "" {
			return nil, fmt.Errorf("%s is required: every control plane registers with the hub as a managed cluster (specs/platform/control-plane.spec.md, Mandatory Cluster Identity)", r.name)
		}
	}
	// An invalid GATEWAY_RESOURCES fails startup rather than falling back to the
	// defaults like the other tunables: silently ignoring an operator's sizing
	// could leave gateways under-provisioned and OOMKilled with no signal.
	if v := os.Getenv("GATEWAY_RESOURCES"); v != "" {
		rr, err := helm.ParseGatewayResources(v)
		if err != nil {
			return nil, fmt.Errorf("invalid GATEWAY_RESOURCES: %w", err)
		}
		cfg.GatewayResources = &rr
	}

	// Validate Helm configuration
	if cfg.HelmChartRegistry != "" && cfg.HelmChartVersion == "" {
		return nil, fmt.Errorf("HELM_CHART_VERSION is required when HELM_CHART_REGISTRY is set")
	}

	return cfg, nil
}

// parseCapabilities converts the comma-separated deployment setting into the
// complete ordered set sent on every registration call. Empty entries and
// duplicates are ignored so harmless whitespace or repetition in a manifest
// cannot produce invalid placement metadata.
func parseCapabilities(value string) []string {
	seen := make(map[string]struct{})
	capabilities := make([]string, 0)
	for _, raw := range strings.Split(value, ",") {
		capability := strings.TrimSpace(raw)
		if capability == "" {
			continue
		}
		if _, exists := seen[capability]; exists {
			continue
		}
		seen[capability] = struct{}{}
		capabilities = append(capabilities, capability)
	}
	return capabilities
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
