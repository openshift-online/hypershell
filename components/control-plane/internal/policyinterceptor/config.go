package policyinterceptor

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/openshift-online/hypershell/components/api-server/pkg/cedar"
)

// Environment contract between the controller (which renders the sidecar) and
// the interceptor binary. The controller's chart values set these names.
const (
	EnvSocket              = "HYPERSHELL_POLICY_SOCKET"
	EnvGatewayID           = "HYPERSHELL_POLICY_GATEWAY_ID"
	EnvNamespace           = "HYPERSHELL_POLICY_NAMESPACE"
	EnvJWTIssuer           = "HYPERSHELL_POLICY_JWT_ISSUER"
	EnvJWTAudience         = "HYPERSHELL_POLICY_JWT_AUDIENCE"
	EnvJWTPublicKeyFile    = "HYPERSHELL_POLICY_JWT_PUBLIC_KEY_FILE"
	EnvEventsAddr          = "HYPERSHELL_POLICY_EVENTS_ADDR"
	EnvEventsTokenFile     = "HYPERSHELL_POLICY_EVENTS_TOKEN_FILE"
	EnvDefaultCPURequest   = "HYPERSHELL_POLICY_DEFAULT_CPU_REQUEST"
	EnvDefaultCPULimit     = "HYPERSHELL_POLICY_DEFAULT_CPU_LIMIT"
	EnvDefaultMemRequest   = "HYPERSHELL_POLICY_DEFAULT_MEMORY_REQUEST"
	EnvDefaultMemLimit     = "HYPERSHELL_POLICY_DEFAULT_MEMORY_LIMIT"
	EnvHealthAddr          = "HYPERSHELL_POLICY_HEALTH_ADDR"
	EnvEventBuffer         = "HYPERSHELL_POLICY_EVENT_BUFFER"
	EnvPoliciesURL         = "HYPERSHELL_POLICY_POLICIES_URL"
	DefaultSocketPath      = "/var/run/hypershell-policy/interceptor.sock"
	DefaultJWTPublicKey    = "/etc/hypershell-policy/jwt/public.pem"
	DefaultEventsTokenFile = "/var/run/secrets/hypershell-policy/token"
	DefaultHealthAddr      = ":9466"
	DefaultEventBuffer     = 1000
)

// DefaultQuantities are the fleet default sandbox resources as Kubernetes
// quantity strings, exactly as configured (PLAT-PGP-08). modify_operation
// patches these strings into a request; Cedar evaluation uses Parsed.
type DefaultQuantities struct {
	CPURequest    string
	CPULimit      string
	MemoryRequest string
	MemoryLimit   string
	Parsed        cedar.Defaults
}

// Config is the validated sidecar configuration.
type Config struct {
	SocketPath       string
	GatewayID        string
	Namespace        string
	JWTIssuer        string
	JWTAudience      string
	JWTPublicKeyFile string
	EventsAddr       string
	EventsTokenFile  string
	HealthAddr       string
	EventBuffer      int
	PoliciesURL      string
	Defaults         DefaultQuantities
}

// LoadConfig reads and validates the environment contract. lookup is
// os.LookupEnv in production. Every problem is reported together so a
// misconfigured sidecar fails at startup with one clear message.
func LoadConfig(lookup func(string) (string, bool)) (Config, error) {
	get := func(name, def string) string {
		if v, ok := lookup(name); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
		return def
	}
	var errs []error
	required := func(name string) string {
		v := get(name, "")
		if v == "" {
			errs = append(errs, fmt.Errorf("%s is required", name))
		}
		return v
	}

	cfg := Config{
		SocketPath:       get(EnvSocket, DefaultSocketPath),
		GatewayID:        required(EnvGatewayID),
		Namespace:        required(EnvNamespace),
		JWTIssuer:        required(EnvJWTIssuer),
		JWTAudience:      get(EnvJWTAudience, DefaultAudience),
		JWTPublicKeyFile: get(EnvJWTPublicKeyFile, DefaultJWTPublicKey),
		EventsAddr:       required(EnvEventsAddr),
		EventsTokenFile:  get(EnvEventsTokenFile, DefaultEventsTokenFile),
		HealthAddr:       get(EnvHealthAddr, DefaultHealthAddr),
		EventBuffer:      DefaultEventBuffer,
		PoliciesURL:      get(EnvPoliciesURL, ""),
	}
	if raw := get(EnvEventBuffer, ""); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			errs = append(errs, fmt.Errorf("%s must be a positive integer, got %q", EnvEventBuffer, raw))
		} else {
			cfg.EventBuffer = n
		}
	}

	dq := DefaultQuantities{
		CPURequest:    required(EnvDefaultCPURequest),
		CPULimit:      required(EnvDefaultCPULimit),
		MemoryRequest: required(EnvDefaultMemRequest),
		MemoryLimit:   required(EnvDefaultMemLimit),
	}
	if dq.CPURequest != "" && dq.CPULimit != "" && dq.MemoryRequest != "" && dq.MemoryLimit != "" {
		parsed, err := cedar.ParseDefaults(dq.CPURequest, dq.CPULimit, dq.MemoryRequest, dq.MemoryLimit)
		if err != nil {
			errs = append(errs, fmt.Errorf("invalid fleet default (%s, %s, %s, %s): %w",
				EnvDefaultCPURequest, EnvDefaultCPULimit, EnvDefaultMemRequest, EnvDefaultMemLimit, err))
		} else {
			if parsed.CPURequestMillis > parsed.CPULimitMillis {
				errs = append(errs, fmt.Errorf("%s (%s) exceeds %s (%s)", EnvDefaultCPURequest, dq.CPURequest, EnvDefaultCPULimit, dq.CPULimit))
			}
			if parsed.MemoryRequestBytes > parsed.MemoryLimitBytes {
				errs = append(errs, fmt.Errorf("%s (%s) exceeds %s (%s)", EnvDefaultMemRequest, dq.MemoryRequest, EnvDefaultMemLimit, dq.MemoryLimit))
			}
			dq.Parsed = parsed
		}
	}
	cfg.Defaults = dq

	if err := errors.Join(errs...); err != nil {
		return Config{}, fmt.Errorf("policy interceptor configuration: %w", err)
	}
	return cfg, nil
}
