package sandboxEvents

import (
	"os"
	"strconv"
	"time"

	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/environments"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/registry"
)

const serviceName = "SandboxEventStore"

func Store(s *environments.Services) SandboxEventStore {
	if s == nil {
		return nil
	}
	store, _ := s.GetService(serviceName).(SandboxEventStore)
	return store
}

func init() {
	registry.RegisterService(serviceName, func(interface{}) interface{} {
		return NewMemoryStore(durationEnv("HYPERSHELL_SANDBOX_EVENT_TTL", DefaultTTL), intEnv("HYPERSHELL_SANDBOX_EVENT_LIMIT", DefaultEventLimit), intEnv("HYPERSHELL_SANDBOX_EVENT_MAX_ENTRIES", DefaultMaxEntries))
	})
}
func durationEnv(name string, fallback time.Duration) time.Duration {
	if raw := os.Getenv(name); raw != "" {
		if value, err := time.ParseDuration(raw); err == nil && value > 0 {
			return value
		}
	}
	return fallback
}
func intEnv(name string, fallback int) int {
	if raw := os.Getenv(name); raw != "" {
		if value, err := strconv.Atoi(raw); err == nil {
			return value
		}
	}
	return fallback
}
