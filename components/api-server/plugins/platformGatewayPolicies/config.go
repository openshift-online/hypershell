package platformGatewayPolicies

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/openshift-online/hypershell/components/api-server/pkg/cedar"
)

// Fleet-default environment variables (PLAT-PGP-08). They are configured in
// exactly one place, the api-server Deployment. The api-server accepts them at
// startup (refusing a change an enforced policy forbids), persists the
// accepted value with the PlatformPolicySettings singleton, and distributes it
// to every control plane through the settings watch.
const (
	EnvDefaultCPURequest    = "HYPERSHELL_POLICY_DEFAULT_CPU_REQUEST"
	EnvDefaultCPULimit      = "HYPERSHELL_POLICY_DEFAULT_CPU_LIMIT"
	EnvDefaultMemoryRequest = "HYPERSHELL_POLICY_DEFAULT_MEMORY_REQUEST"
	EnvDefaultMemoryLimit   = "HYPERSHELL_POLICY_DEFAULT_MEMORY_LIMIT"
	EnvFindingsRetention    = "HYPERSHELL_POLICY_FINDINGS_RETENTION_DAYS"
)

// Built-in fleet defaults used when the environment does not set a value.
const (
	DefaultCPURequest    = "500m"
	DefaultCPULimit      = "2"
	DefaultMemoryRequest = "1Gi"
	DefaultMemoryLimit   = "4Gi"
	defaultRetentionDays = 30
)

// FleetDefaults are fleet defaults as written (quantities) and normalized
// (schema units): either the configured value or the accepted one.
type FleetDefaults struct {
	CPURequest    string
	CPULimit      string
	MemoryRequest string
	MemoryLimit   string
	Normalized    cedar.Defaults
}

// Config is the plugin's environment configuration. Defaults are the
// configured fleet defaults; the served value is the accepted one
// (PolicyService.AcceptedDefaults).
type Config struct {
	Defaults  FleetDefaults
	Retention time.Duration
}

func envOr(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

// LoadConfig reads the fleet defaults and the findings retention from the
// environment. An invalid quantity or retention is an error, never silently
// replaced by a built-in value.
func LoadConfig() (Config, error) {
	d := FleetDefaults{
		CPURequest:    envOr(EnvDefaultCPURequest, DefaultCPURequest),
		CPULimit:      envOr(EnvDefaultCPULimit, DefaultCPULimit),
		MemoryRequest: envOr(EnvDefaultMemoryRequest, DefaultMemoryRequest),
		MemoryLimit:   envOr(EnvDefaultMemoryLimit, DefaultMemoryLimit),
	}
	normalized, err := cedar.ParseDefaults(d.CPURequest, d.CPULimit, d.MemoryRequest, d.MemoryLimit)
	if err != nil {
		return Config{}, fmt.Errorf("fleet policy defaults: %w", err)
	}
	d.Normalized = normalized
	days := defaultRetentionDays
	if v := os.Getenv(EnvFindingsRetention); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return Config{}, fmt.Errorf("%s must be a positive integer number of days, got %q", EnvFindingsRetention, v)
		}
		days = n
	}
	return Config{Defaults: d, Retention: time.Duration(days) * 24 * time.Hour}, nil
}
