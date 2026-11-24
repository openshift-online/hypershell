package policyinterceptor

import (
	"strings"
	"testing"
)

func envLookup(env map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) {
		v, ok := env[k]
		return v, ok
	}
}

func validEnv() map[string]string {
	return map[string]string{
		EnvGatewayID:         testGatewayID,
		EnvNamespace:         testNamespace,
		EnvJWTIssuer:         testIssuer,
		EnvEventsAddr:        "hypershell-controller.hypershell.svc:9470",
		EnvDefaultCPURequest: "500m",
		EnvDefaultCPULimit:   "2",
		EnvDefaultMemRequest: "1Gi",
		EnvDefaultMemLimit:   "4Gi",
	}
}

// Verifies: PLAT-PGP-08.01
func TestLoadConfigDefaultsAndValidation(t *testing.T) {
	cfg, err := LoadConfig(envLookup(validEnv()))
	if err != nil {
		t.Fatalf("valid env rejected: %v", err)
	}
	if cfg.SocketPath != DefaultSocketPath || cfg.JWTAudience != DefaultAudience || cfg.HealthAddr != DefaultHealthAddr ||
		cfg.EventBuffer != DefaultEventBuffer || cfg.JWTPublicKeyFile != DefaultJWTPublicKey || cfg.EventsTokenFile != DefaultEventsTokenFile {
		t.Fatalf("defaults not applied: %+v", cfg)
	}
	if cfg.Defaults.Parsed.CPURequestMillis != 500 || cfg.Defaults.Parsed.MemoryLimitBytes != 4<<30 || cfg.Defaults.CPULimit != "2" {
		t.Fatalf("fleet defaults not parsed: %+v", cfg.Defaults)
	}

	bad := validEnv()
	bad[EnvDefaultCPURequest] = "lots"
	bad[EnvEventBuffer] = "-1"
	delete(bad, EnvJWTIssuer)
	_, err = LoadConfig(envLookup(bad))
	if err == nil {
		t.Fatalf("invalid env accepted")
	}
	for _, want := range []string{EnvJWTIssuer, EnvEventBuffer, "default CPU request"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}

	inverted := validEnv()
	inverted[EnvDefaultMemRequest] = "8Gi"
	if _, err := LoadConfig(envLookup(inverted)); err == nil || !strings.Contains(err.Error(), EnvDefaultMemRequest) {
		t.Fatalf("request above limit accepted: %v", err)
	}
}
