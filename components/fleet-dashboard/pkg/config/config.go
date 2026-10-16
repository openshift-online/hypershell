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
	// SandboxMetric is the per-gateway active-sandbox gauge. ClusterLabel is the
	// SCRAPE-INJECTED label carrying the emitting hub's cluster identity (NOT the
	// spoke a gateway runs on); kept for the legacy per-cluster sandbox breakdown.
	// ManagedClusterLabel is the APPLICATION-EMITTED label naming the managed
	// cluster (spoke) each gateway runs on (== ManagedCluster.Name / GitOps spoke
	// name); it is the correct dimension for per-spoke attribution
	// (gateway-managed-cluster-attribution.spec.md).
	SandboxMetric       string
	ClusterLabel        string
	ManagedClusterLabel string
	// UserMetric is the registered-user total gauge; UserLoginsMetric is the
	// rolling 7-day unique-login count. Both carry the same instance label as the
	// sandbox gauge, so they fold into the per-instance fleet record the same way.
	UserMetric       string
	UserLoginsMetric string

	// Auth (backend defense-in-depth gate, §3.4)
	AuthEnabled bool
	SAR         SubjectAccessReview

	// GitHub (best-effort analysis check-run links; nullable - §5)
	GitHubRepo      string // owner/repo - REQUIRED to enable; no default (firewall)
	GitHubTokenFile string
	GitHubAPIBase   string
	// UpstreamRepo is the PUBLIC product source repo (owner/repo) whose merged
	// pull requests compose a release bundle -- the "In this bundle" list. Unlike
	// GitHubRepo (the gitops repo, a fleet-identifying value that must arrive from
	// config), the product source is public and NOT fleet-identifying, so it
	// carries a compiled-in default. Read UNAUTHENTICATED by default (public repo);
	// UpstreamTokenFile stays empty unless an operator points this at a repo that
	// needs auth (e.g. a mirror).
	UpstreamRepo      string
	UpstreamTokenFile string
	// ReleaseHistoryLimit caps how many distinct release bundles the freight bar
	// shows (currently-deployed plus previously-deployed history cards). Reuses the
	// release-lock resolver, so it only takes effect when GitHubRepo is set.
	ReleaseHistoryLimit int

	// Refresh intervals (§5.1)
	RefreshFleet     time.Duration
	RefreshPromotion time.Duration
	RefreshTopology  time.Duration
	RefreshInstances time.Duration
	// RefreshSDLC is the Agent SDLC plane's own refresh cadence. The SDLC source
	// reconstructs label-event history from the GitHub timeline, so its default is
	// conservative: the whole fleet is served from one cached snapshot and the
	// GitHub request rate is bounded by this loop alone, never by viewer count
	// (agent-sdlc-visibility-dashboard.spec SDLC-VIS-04/14).
	RefreshSDLC time.Duration

	// SDLC per-phase service-level expectations (agent-sdlc-visibility-dashboard
	// spec SDLC-VIS-13). These are policy thresholds, not fleet-identifying values,
	// so sensible defaults are compiled in and overridable per environment through
	// FD_SLE_* in the gitops deployment config. A card whose time-in-phase crosses its phase
	// SLE escalates off the board into the escalation lane (SDLC-VIS-10); the dashed
	// SLE line is drawn at this value (SDLC-VIS-08). Keyed by SDLC phase key.
	SDLCPhaseSLE map[string]time.Duration
}

// SDLCPhaseKeys are the aging-board phase columns in funnel order, excluding the
// terminal Shipped column (which is rendered as the release-by-hub-cluster matrix,
// not a timed column). Each has a per-phase SLE (SDLCPhaseSLE) and an FD_SLE_*
// override. Kept here so config and the SDLC source agree on the canonical set.
var SDLCPhaseKeys = []string{
	"intake",
	"workable",
	"spec",
	"spec-approved",
	"code",
	"code-approved",
}

// sdlcSLEDefaults are the built-in per-phase SLE defaults (SDLC-VIS-13: "sensible
// per-phase defaults overridable per environment"). Review phases carry the
// longest expectation because rework loops legitimately take time; approval
// hand-off phases are expected to move quickly.
var sdlcSLEDefaults = map[string]time.Duration{
	"intake":        24 * time.Hour,
	"workable":      24 * time.Hour,
	"spec":          72 * time.Hour,
	"spec-approved": 24 * time.Hour,
	"code":          120 * time.Hour,
	"code-approved": 24 * time.Hour,
}

// sdlcSLEEnvVar maps a phase key to its FD_SLE_* override variable.
func sdlcSLEEnvVar(phase string) string {
	return "FD_SLE_" + strings.ToUpper(strings.ReplaceAll(phase, "-", "_"))
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
		ManagedClusterLabel:   env("FD_MANAGED_CLUSTER_LABEL", "managed_cluster"),
		UserMetric:            env("FD_USER_METRIC", "hypershell_users_registered_total"),
		UserLoginsMetric:      env("FD_USER_LOGINS_METRIC", "hypershell_users_unique_logins_last_7_days_total"),
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
		// openshift-online/hypershell is the public product source and is NOT
		// fleet-identifying, so it is safe to compile in as the default (firewall).
		UpstreamRepo:      env("FD_UPSTREAM_REPO", "openshift-online/hypershell"),
		UpstreamTokenFile: env("FD_UPSTREAM_TOKEN_FILE", ""),

		ReleaseHistoryLimit: envInt("FD_RELEASE_HISTORY_LIMIT", 10),
		RefreshFleet:        envDuration("FD_REFRESH_FLEET", 15*time.Second),
		RefreshPromotion:    envDuration("FD_REFRESH_PROMOTION", 30*time.Second),
		RefreshTopology:     envDuration("FD_REFRESH_TOPOLOGY", 30*time.Second),
		RefreshInstances:    envDuration("FD_REFRESH_INSTANCES", 60*time.Second),
		RefreshSDLC:         envDuration("FD_REFRESH_SDLC", 120*time.Second),
		SDLCPhaseSLE:        loadSDLCPhaseSLE(),
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

// loadSDLCPhaseSLE resolves each aging-board phase's SLE, applying the compiled-in
// default (sdlcSLEDefaults) unless the phase's FD_SLE_* override is set. Changing
// an SLE is therefore a configuration change, never a code change (SDLC-VIS-13).
func loadSDLCPhaseSLE() map[string]time.Duration {
	out := make(map[string]time.Duration, len(SDLCPhaseKeys))
	for _, phase := range SDLCPhaseKeys {
		out[phase] = envDuration(sdlcSLEEnvVar(phase), sdlcSLEDefaults[phase])
	}
	return out
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
