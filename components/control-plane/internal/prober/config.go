// Package prober implements the outside availability prober (OBS-PRB): a
// black-box probe that runs on a Cloud Hub, separate from the clusters it
// measures, and continuously probes gateways and the production API from the
// outside. It is the data source for the Gateway availability and API
// availability Service Level Objectives.
//
// The prober ships as a run mode of the control-plane binary selected by the
// `probe` subcommand (OBS-PRB-10); it runs only the probe loops and the metrics
// endpoint and never starts the reconciler. See
// specs/platform/outside-prober.spec.md.
package prober

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

// Defaults for the probe cadence and eligibility window (OBS-PRB-08, OBS-PRB-09).
const (
	// DefaultInterval is the probe cadence. 30 seconds with a shorter per-probe
	// timeout guarantees a newly started outage shows as a failed probe within one
	// minute (OBS-PRB-09).
	DefaultInterval = 30 * time.Second
	// DefaultTimeout is the per-probe timeout. It MUST be shorter than the
	// interval (OBS-PRB-08) so one slow target cannot stall the cycle.
	DefaultTimeout = 10 * time.Second
	// DefaultGraceWindow is the gateway eligibility grace window (OBS-PRB-02): a
	// gateway is not probed until it was created more than this long ago.
	DefaultGraceWindow = 10 * time.Minute
	// DefaultWorkers bounds how many targets are probed concurrently so one slow
	// target does not delay the others (OBS-PRB-08). A probe cycle that has many
	// simultaneously timing-out targets takes about
	// ceil(timing_out / workers) * timeout, so to keep a mass outage inside the
	// one-minute budget (OBS-PRB-09) operators SHALL scale the worker count with
	// the fleet size: workers should be at least fleet_size * timeout /
	// (interval - timeout). At the defaults (interval 30s, timeout 10s) 16 workers
	// cover roughly 32 concurrently failing gateways within the budget; raise
	// HYPERSHELL_PROBE_WORKERS for larger fleets.
	DefaultWorkers = 16
	// DefaultMetricsAddr is the bind address for the Prometheus metrics endpoint
	// the Cloud Hub Prometheus scrapes (OBS-PRB-07).
	DefaultMetricsAddr = ":9090"
	// DefaultMetricsPath is the metrics endpoint path.
	DefaultMetricsPath = "/metrics"
	// DefaultSATokenFile is where the prober Deployment mounts its projected
	// Kubernetes service-account token for the Keycloak token exchange
	// (OBS-PRB-06, OBS-PRB-11).
	DefaultSATokenFile = "/var/run/secrets/hypershell/prober-token/token" // #nosec G101 -- file path, not a credential
	// DefaultSubjectTokenType is the RFC 8693 token type of the projected
	// service-account token presented as the exchange subject. It is
	// configurable so the prober is not tied to one Keycloak token-exchange
	// generation (OBS-PRB-06, PLAT-CITR-08).
	DefaultSubjectTokenType = "urn:ietf:params:oauth:token-type:jwt"
	// DefaultAPIReadPath is the database-backed read exercised by the API
	// availability and authentication probes (OBS-PRB-04, OBS-PRB-05). It is a
	// real resource read, never a liveness endpoint that answers without reading
	// the database.
	DefaultAPIReadPath = "/api/hypershell/v1/gateways?size=1"
)

// Config is the prober's deployment configuration (OBS-PRB-08). Every operator
// tunable - cadence, timeouts, the eligibility grace window, the production
// instances, and the Keycloak token-exchange settings - is sourced from the
// environment so operators change them without a code change.
type Config struct {
	// Interval is the probe cadence for every probe family (OBS-PRB-08).
	Interval time.Duration
	// Timeout is the per-probe timeout; it MUST be shorter than Interval.
	Timeout time.Duration
	// GraceWindow is the gateway eligibility grace window (OBS-PRB-02).
	GraceWindow time.Duration
	// Workers bounds the concurrent probe worker pool (OBS-PRB-08).
	Workers int

	// MetricsAddr and MetricsPath locate the Prometheus metrics endpoint
	// (OBS-PRB-07).
	MetricsAddr string
	MetricsPath string

	// GatewayAPIURL is the HyperShell API the prober lists gateways and managed
	// clusters from to build the gateway health target set (OBS-PRB-02). The
	// prober reads it with its keyless identity (OBS-PRB-06).
	GatewayAPIURL string

	// ProductionInstances is the set of production HyperShell API base URLs the
	// API availability (OBS-PRB-04) and authentication (OBS-PRB-05) probes
	// measure. Each entry is a public API endpoint; the instance label on the
	// counters is the base URL.
	ProductionInstances []string
	// APIReadPath is the database-backed read path used by OBS-PRB-04/05.
	APIReadPath string

	// TokenExchange carries the keyless workload-identity settings (OBS-PRB-06).
	TokenExchange TokenExchangeConfig
}

// TokenExchangeConfig configures the keyless token exchange (OBS-PRB-06). The
// prober reads a projected Kubernetes service-account token from a file and
// exchanges it at Keycloak for a short-lived HyperShell API access token. No
// static client secret appears anywhere. Every parameter is configurable so the
// prober is not tied to one Keycloak token-exchange mechanism (PLAT-CITR-08).
type TokenExchangeConfig struct {
	// TokenEndpoint is the Keycloak realm token endpoint the exchange POSTs to.
	TokenEndpoint string
	// ClientID is the shared public (keyless) exchange requester client; no
	// client secret is used.
	ClientID string
	// SATokenFile is the path to the mounted projected service-account token.
	SATokenFile string
	// SubjectTokenType is the RFC 8693 type of the subject token.
	SubjectTokenType string
	// RequestedTokenType optionally requests a specific returned token type.
	RequestedTokenType string
	// SubjectIssuer optionally names the Keycloak identity-provider alias the
	// subject token is brokered through.
	SubjectIssuer string
	// Audience optionally pins the requested token audience.
	Audience string
	// Scope optionally requests specific scopes.
	Scope string
}

// Enabled reports whether the token exchange is configured. When it is not, the
// authentication probe (OBS-PRB-05) cannot run and the prober records failed
// authentication probes; the gateway health probe (OBS-PRB-03) is unauthenticated
// and still runs.
func (t TokenExchangeConfig) Enabled() bool {
	return t.TokenEndpoint != "" && t.ClientID != "" && t.SATokenFile != ""
}

// Load builds the prober Config from the environment (OBS-PRB-08). Invalid
// tunables fall back to their defaults with a warning rather than failing
// startup, so a mistuned knob never stops the prober (consistent with the
// control-plane config package). A per-probe timeout that is not shorter than
// the interval is corrected to a fraction of the interval, preserving the
// OBS-PRB-09 within-one-minute guarantee.
func Load() *Config {
	cfg := &Config{
		Interval:            getEnvDuration("HYPERSHELL_PROBE_INTERVAL", DefaultInterval),
		Timeout:             getEnvDuration("HYPERSHELL_PROBE_TIMEOUT", DefaultTimeout),
		GraceWindow:         getEnvDuration("HYPERSHELL_PROBE_GRACE_WINDOW", DefaultGraceWindow),
		Workers:             getEnvInt("HYPERSHELL_PROBE_WORKERS", DefaultWorkers, 1),
		MetricsAddr:         getEnv("HYPERSHELL_PROBE_METRICS_ADDR", DefaultMetricsAddr),
		MetricsPath:         getEnv("HYPERSHELL_PROBE_METRICS_PATH", DefaultMetricsPath),
		GatewayAPIURL:       getEnv("HYPERSHELL_PROBE_GATEWAY_API_URL", getEnv("HYPERSHELL_API_SERVER_URL", "")),
		ProductionInstances: parseList(getEnv("HYPERSHELL_PROBE_API_INSTANCES", "")),
		APIReadPath:         getEnv("HYPERSHELL_PROBE_API_READ_PATH", DefaultAPIReadPath),
		TokenExchange: TokenExchangeConfig{
			TokenEndpoint:      getEnv("HYPERSHELL_PROBE_TOKEN_ENDPOINT", ""),
			ClientID:           getEnv("HYPERSHELL_PROBE_OIDC_CLIENT_ID", ""),
			SATokenFile:        getEnv("HYPERSHELL_PROBE_SA_TOKEN_FILE", DefaultSATokenFile),
			SubjectTokenType:   getEnv("HYPERSHELL_PROBE_TOKEN_SUBJECT_TOKEN_TYPE", DefaultSubjectTokenType),
			RequestedTokenType: getEnv("HYPERSHELL_PROBE_TOKEN_REQUESTED_TOKEN_TYPE", ""),
			SubjectIssuer:      getEnv("HYPERSHELL_PROBE_TOKEN_SUBJECT_ISSUER", ""),
			Audience:           getEnv("HYPERSHELL_PROBE_TOKEN_AUDIENCE", ""),
			Scope:              getEnv("HYPERSHELL_PROBE_TOKEN_SCOPE", ""),
		},
	}

	// A non-positive probe interval would panic time.NewTicker in the probe loops
	// (prober.go) and crash-loop the pod. A mistuned knob SHALL never stop the
	// prober, so fall back to the default interval with a warning (OBS-PRB-08).
	if cfg.Interval <= 0 {
		slog.Warn("Probe interval must be positive, using default",
			"interval", cfg.Interval, "default", DefaultInterval)
		cfg.Interval = DefaultInterval
	}

	// The metrics path is registered on an http.ServeMux, which panics on a
	// pattern without a leading slash. Normalize a mistuned path rather than
	// crashing the prober (OBS-PRB-07, OBS-PRB-08).
	if !strings.HasPrefix(cfg.MetricsPath, "/") {
		slog.Warn("Metrics path must start with a slash, correcting",
			"path", cfg.MetricsPath, "corrected", "/"+cfg.MetricsPath)
		cfg.MetricsPath = "/" + cfg.MetricsPath
	}

	// The per-probe timeout SHALL be shorter than the probe interval (OBS-PRB-08).
	// A non-positive or too-large timeout is corrected to 1/3 of the interval so
	// a newly started outage is still reflected within one minute (OBS-PRB-09).
	if cfg.Timeout <= 0 || cfg.Timeout >= cfg.Interval {
		corrected := cfg.Interval / 3
		if corrected <= 0 {
			corrected = DefaultTimeout
		}
		slog.Warn("Per-probe timeout must be shorter than the probe interval, correcting",
			"timeout", cfg.Timeout, "interval", cfg.Interval, "corrected", corrected)
		cfg.Timeout = corrected
	}

	return cfg
}

// parseList splits a comma-separated setting into a trimmed, de-duplicated,
// order-preserving slice. Empty entries are dropped so stray commas in a
// manifest cannot produce an empty instance URL.
func parseList(value string) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0)
	for _, raw := range strings.Split(value, ",") {
		item := strings.TrimSpace(raw)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback, min int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(v)
	if err != nil {
		slog.Warn("Invalid int environment variable, using default", "name", key, "value", v, "default", fallback, "error", err)
		return fallback
	}
	if parsed < min {
		slog.Warn("Environment variable is below the minimum, using default", "name", key, "value", parsed, "minimum", min, "default", fallback)
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
		slog.Warn("Invalid duration environment variable, using default", "name", key, "value", v, "default", fallback, "error", err)
		return fallback
	}
	return parsed
}

// Summary returns a human-readable, secret-free description of the effective
// configuration for the startup log. It never includes a token or secret.
func (c *Config) Summary() string {
	return fmt.Sprintf(
		"interval=%s timeout=%s grace_window=%s workers=%d gateway_api=%q production_instances=%d token_exchange_enabled=%t",
		c.Interval, c.Timeout, c.GraceWindow, c.Workers, c.GatewayAPIURL, len(c.ProductionInstances), c.TokenExchange.Enabled(),
	)
}
