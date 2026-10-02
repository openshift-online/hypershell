// Package config holds all runtime configuration for the fleet-dashboard BFF.
//
// This package is the fleet-identity firewall boundary (data-architecture.spec
// §3.5): nothing that reveals the fleet's structure - instance names, cluster
// names, hostnames, namespaces, service-account or group names, the gitops repo
// slug, or any allowlist - is baked into the binary. Every such value arrives
// here at runtime, from environment variables (or flags). Product-level names
// that are already public (HyperShell metric names, the promoter CRD group, and
// the delivery.hypershell.app/* label schema) are permitted as defaults.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config is the fully-resolved runtime configuration.
type Config struct {
	// Server
	ListenAddr  string
	MetricsAddr string // dedicated /metrics listener, not fronted by oauth-proxy (§9)
	StaticRoot  string // if set, serve UI from this dir instead of the embedded bundle

	// Prometheus
	PromURL       string
	PromTokenFile string
	PromInsecure  bool

	// Kubernetes
	KubeconfigPath string // empty => in-cluster config

	// Promotion (GitOps Promoter CRDs)
	PromoterNamespace     string
	PromotionStrategyName string
	PromoterGroup         string
	PromoterVersion       string

	// Argo CD
	ArgoNamespaces []string // empty => all namespaces
	ArgoGroup      string
	ArgoVersion    string
	// ArgoBaseURL is the external base URL of the Argo CD UI that aggregates the
	// fleet's Applications (e.g. https://argocd.example). Empty => no argoUrl is
	// emitted (firewall: the host reveals fleet structure, so it has no default
	// and must be supplied at runtime). When set, each environment gets a deep
	// link to its Argo CD Application tree, where the analysis AnalysisRun's
	// Jobs/Pods - and their logs - surface.
	ArgoBaseURL string

	// Topology ConfigMaps
	TopologyNamespace     string // empty => all namespaces
	TopologyLabelSelector string

	// Metric conventions (product-level; safe defaults)
	GatewayMetric string
	InstanceLabel string
	// SandboxMetric is the per-gateway active-sandbox gauge; ClusterLabel is the
	// scrape-injected label carrying the managed-cluster identity, used to break
	// the sandbox count down per cluster.
	SandboxMetric string
	ClusterLabel  string

	// Auth (backend defense-in-depth gate, §3.4)
	AuthEnabled bool
	SAR         SubjectAccessReview

	// GitHub (best-effort analysis check-run links; nullable - §5)
	GitHubRepo      string // owner/repo - REQUIRED to enable; no default (firewall)
	GitHubTokenFile string
	GitHubAPIBase   string
	// ReleaseHistoryLimit caps how many distinct release bundles the freight bar
	// shows (currently-deployed plus previously-deployed history cards). Reuses the
	// release-lock resolver, so it only takes effect when GitHubRepo is set.
	ReleaseHistoryLimit int

	// Refresh intervals (§5.1)
	RefreshFleet     time.Duration
	RefreshPromotion time.Duration
	RefreshTopology  time.Duration
	RefreshInstances time.Duration
}

// SubjectAccessReview describes the resource attributes the forwarded identity
// must satisfy to read /api/*. Supplied entirely by config so no admin
// group/resource identity is compiled in.
type SubjectAccessReview struct {
	Verb      string
	Group     string
	Resource  string
	Namespace string
	Name      string
}

// Load resolves configuration from the environment, applying product-safe
// defaults and validating required fields.
func Load() (*Config, error) {
	c := &Config{
		ListenAddr:            env("FD_LISTEN_ADDR", ":8080"),
		MetricsAddr:           env("FD_METRICS_ADDR", ":9090"),
		StaticRoot:            env("FD_STATIC_ROOT", ""),
		PromURL:               env("FD_PROM_URL", ""),
		PromTokenFile:         env("FD_PROM_TOKEN_FILE", ""),
		PromInsecure:          envBool("FD_PROM_INSECURE", false),
		KubeconfigPath:        env("FD_KUBECONFIG", ""),
		PromoterNamespace:     env("FD_PROMOTER_NAMESPACE", ""),
		PromotionStrategyName: env("FD_PROMOTION_STRATEGY_NAME", "hypershell"),
		PromoterGroup:         env("FD_PROMOTER_GROUP", "promoter.argoproj.io"),
		PromoterVersion:       env("FD_PROMOTER_VERSION", "v1alpha1"),
		ArgoNamespaces:        csv(env("FD_ARGO_NAMESPACES", "")),
		ArgoGroup:             env("FD_ARGO_GROUP", "argoproj.io"),
		ArgoVersion:           env("FD_ARGO_VERSION", "v1alpha1"),
		ArgoBaseURL:           strings.TrimRight(env("FD_ARGO_BASE_URL", ""), "/"),
		TopologyNamespace:     env("FD_TOPOLOGY_NAMESPACE", ""),
		TopologyLabelSelector: env("FD_TOPOLOGY_LABEL_SELECTOR", "delivery.hypershell.app/topology=true"),
		GatewayMetric:         env("FD_GATEWAY_METRIC", "hypershell_gateways_total"),
		InstanceLabel:         env("FD_INSTANCE_LABEL", "namespace"),
		SandboxMetric:         env("FD_SANDBOX_METRIC", "hypershell_gateways_active_sandboxes_total"),
		ClusterLabel:          env("FD_CLUSTER_LABEL", "cluster"),
		AuthEnabled:           envBool("FD_AUTH_ENABLED", true),
		SAR: SubjectAccessReview{
			Verb:      env("FD_SAR_VERB", "get"),
			Group:     env("FD_SAR_GROUP", ""),
			Resource:  env("FD_SAR_RESOURCE", ""),
			Namespace: env("FD_SAR_NAMESPACE", ""),
			Name:      env("FD_SAR_NAME", ""),
		},
		GitHubRepo:      env("FD_GITHUB_REPO", ""),
		GitHubTokenFile: env("FD_GITHUB_TOKEN_FILE", ""),
		GitHubAPIBase:   env("FD_GITHUB_API_BASE", "https://api.github.com"),

		ReleaseHistoryLimit: envInt("FD_RELEASE_HISTORY_LIMIT", 10),
		RefreshFleet:        envDuration("FD_REFRESH_FLEET", 15*time.Second),
		RefreshPromotion:    envDuration("FD_REFRESH_PROMOTION", 30*time.Second),
		RefreshTopology:     envDuration("FD_REFRESH_TOPOLOGY", 30*time.Second),
		RefreshInstances:    envDuration("FD_REFRESH_INSTANCES", 60*time.Second),
	}

	if c.PromURL == "" {
		return nil, fmt.Errorf("FD_PROM_URL is required")
	}
	if c.PromoterNamespace == "" {
		return nil, fmt.Errorf("FD_PROMOTER_NAMESPACE is required")
	}
	if c.AuthEnabled && c.SAR.Resource == "" && c.SAR.Group == "" {
		return nil, fmt.Errorf("auth is enabled but no SAR attributes configured (set FD_SAR_RESOURCE/FD_SAR_GROUP, or FD_AUTH_ENABLED=false for local dev)")
	}
	return c, nil
}

func env(key, def string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return def
}

func envBool(key string, def bool) bool {
	v, ok := os.LookupEnv(key)
	if !ok {
		return def
	}
	b, err := strconv.ParseBool(strings.TrimSpace(v))
	if err != nil {
		return def
	}
	return b
}

func envInt(key string, def int) int {
	v, ok := os.LookupEnv(key)
	if !ok {
		return def
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return def
	}
	return n
}

func envDuration(key string, def time.Duration) time.Duration {
	v, ok := os.LookupEnv(key)
	if !ok {
		return def
	}
	d, err := time.ParseDuration(strings.TrimSpace(v))
	if err != nil {
		return def
	}
	return d
}

func csv(v string) []string {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
