package config

import (
	"os"
	"strings"
	"testing"
)

// setRequiredEnv sets the configuration every control plane must have to start:
// a managed-cluster name and OIDC client credentials (Mandatory Cluster Identity).
func setRequiredEnv(t *testing.T) {
	t.Helper()
	t.Setenv("HYPERSHELL_GRPC_SERVER_ADDR", "localhost:9000")
	t.Setenv("HYPERSHELL_MANAGED_CLUSTER_NAME", "local-kind")
	t.Setenv("HYPERSHELL_MANAGED_CLUSTER_CAPABILITIES", "provider/kind,network/public,environment/kind")
	t.Setenv("OIDC_ISSUER", "http://keycloak.example/realms/hypershell")
	t.Setenv("OIDC_CLIENT_ID", "hypershell-control-plane")
	t.Setenv("OIDC_CLIENT_SECRET", "secret")
	t.Setenv("HYPERSHELL_CLUSTER_PLATFORM", ClusterPlatformKubernetes)
}

// TestLoadClusterPlatform covers the explicit host-platform contract
// (CTRL-CORE-16): accepted values load, an invalid value and an omitted setting
// both fail startup with an error naming the setting and its accepted values.
// Verifies: CTRL-CORE-16.01, CTRL-CORE-16.02, CTRL-CORE-16.03, CTRL-CORE-16.04
func TestLoadClusterPlatform(t *testing.T) {
	t.Run("accepted values load", func(t *testing.T) {
		for _, platform := range []string{ClusterPlatformOpenShift, ClusterPlatformKubernetes} {
			setRequiredEnv(t)
			t.Setenv("HYPERSHELL_CLUSTER_PLATFORM", platform)
			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load() with platform %q: %v", platform, err)
			}
			if cfg.ClusterPlatform != platform {
				t.Fatalf("ClusterPlatform = %q, want %q", cfg.ClusterPlatform, platform)
			}
		}
	})

	t.Run("case and whitespace are normalized", func(t *testing.T) {
		setRequiredEnv(t)
		t.Setenv("HYPERSHELL_CLUSTER_PLATFORM", "  OpenShift  ")
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load(): %v", err)
		}
		if cfg.ClusterPlatform != ClusterPlatformOpenShift {
			t.Fatalf("ClusterPlatform = %q, want %q", cfg.ClusterPlatform, ClusterPlatformOpenShift)
		}
	})

	t.Run("invalid value fails naming accepted values", func(t *testing.T) {
		setRequiredEnv(t)
		t.Setenv("HYPERSHELL_CLUSTER_PLATFORM", "openstack")
		_, err := Load()
		if err == nil {
			t.Fatal("Load() with invalid platform: want error, got nil")
		}
		for _, want := range []string{"HYPERSHELL_CLUSTER_PLATFORM", ClusterPlatformOpenShift, ClusterPlatformKubernetes} {
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("Load() error %q does not contain %q", err, want)
			}
		}
	})

	t.Run("omitted setting fails naming the setting", func(t *testing.T) {
		for _, mode := range []string{"unset", "blank"} {
			setRequiredEnv(t)
			if mode == "unset" {
				if err := os.Unsetenv("HYPERSHELL_CLUSTER_PLATFORM"); err != nil {
					t.Fatalf("unset: %v", err)
				}
			} else {
				t.Setenv("HYPERSHELL_CLUSTER_PLATFORM", "   ")
			}
			_, err := Load()
			if err == nil {
				t.Fatalf("Load() with %s platform: want error, got nil", mode)
			}
			if !strings.Contains(err.Error(), "HYPERSHELL_CLUSTER_PLATFORM") {
				t.Fatalf("Load() error %q does not name HYPERSHELL_CLUSTER_PLATFORM", err)
			}
		}
	})
}

// TestLoadRequiresClusterIdentity covers "Missing cluster name refuses to start"
// and "Missing OIDC credentials refuse to start": each required variable, when
// unset or blank, fails Load with an error naming that variable.
// Verifies: CTRL-CORE-01.02, CTRL-CORE-01.03
func TestLoadRequiresClusterIdentity(t *testing.T) {
	for _, name := range []string{
		"HYPERSHELL_MANAGED_CLUSTER_NAME",
		"HYPERSHELL_MANAGED_CLUSTER_CAPABILITIES",
		"OIDC_ISSUER",
		"OIDC_CLIENT_ID",
		"OIDC_CLIENT_SECRET",
	} {
		for _, mode := range []string{"unset", "blank"} {
			t.Run(name+"/"+mode, func(t *testing.T) {
				setRequiredEnv(t)
				if mode == "unset" {
					if err := os.Unsetenv(name); err != nil {
						t.Fatalf("unset %s: %v", name, err)
					}
				} else {
					t.Setenv(name, "   ")
				}
				cfg, err := Load()
				if err == nil {
					t.Fatalf("Load() with %s %s: want error, got config %+v", name, mode, cfg)
				}
				if !strings.Contains(err.Error(), name) {
					t.Fatalf("Load() error %q does not name %s", err, name)
				}
			})
		}
	}
}

func TestLoadManagedClusterCapabilities(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("HYPERSHELL_MANAGED_CLUSTER_CAPABILITIES", " provider/aws, network/vpn,provider/aws, region/us-east-1, ")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load(): %v", err)
	}
	want := []string{"provider/aws", "network/vpn", "region/us-east-1"}
	if got := strings.Join(cfg.ManagedClusterCapabilities, ","); got != strings.Join(want, ",") {
		t.Fatalf("ManagedClusterCapabilities = %v, want %v", cfg.ManagedClusterCapabilities, want)
	}
}

// TestLoadGatewayReconcileWorkers covers the GATEWAY_RECONCILE_WORKERS contract:
// unset uses the bounded default; a valid positive integer is honored; and an
// invalid or non-positive value warns and falls back to the default rather than
// disabling the pool. The resolved count is always >= 1.
// Verifies: GTW-CONC-01.01, GTW-CONC-01.02, GTW-CONC-03.01, GTW-CONC-03.02
func TestLoadGatewayReconcileWorkers(t *testing.T) {
	setRequiredEnv(t)

	cases := []struct {
		name     string
		envValue string
		envUnset bool
		want     int
	}{
		{name: "unset uses default", envUnset: true, want: DefaultGatewayReconcileWorkers},
		{name: "empty uses default", envValue: "", want: DefaultGatewayReconcileWorkers},
		{name: "valid value honored", envValue: "8", want: 8},
		{name: "one is honored", envValue: "1", want: 1},
		{name: "non-numeric falls back", envValue: "abc", want: DefaultGatewayReconcileWorkers},
		{name: "zero falls back", envValue: "0", want: DefaultGatewayReconcileWorkers},
		{name: "negative falls back", envValue: "-3", want: DefaultGatewayReconcileWorkers},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.envUnset {
				if err := os.Unsetenv("GATEWAY_RECONCILE_WORKERS"); err != nil {
					t.Fatalf("unset GATEWAY_RECONCILE_WORKERS: %v", err)
				}
			} else {
				t.Setenv("GATEWAY_RECONCILE_WORKERS", tc.envValue)
			}

			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load() with GATEWAY_RECONCILE_WORKERS=%q: unexpected error: %v", tc.envValue, err)
			}
			if cfg.GatewayReconcileWorkers != tc.want {
				t.Fatalf("Load() with GATEWAY_RECONCILE_WORKERS=%q: GatewayReconcileWorkers = %d, want %d", tc.envValue, cfg.GatewayReconcileWorkers, tc.want)
			}
			if cfg.GatewayReconcileWorkers < 1 {
				t.Fatalf("GatewayReconcileWorkers = %d, want >= 1", cfg.GatewayReconcileWorkers)
			}
		})
	}
}

func TestGetEnvInt(t *testing.T) {
	const key = "TEST_GET_ENV_INT"
	tests := []struct {
		name     string
		set      bool
		value    string
		fallback int
		min      int
		want     int
	}{
		{name: "unset uses fallback", set: false, fallback: 4, min: 1, want: 4},
		{name: "empty uses fallback", set: true, value: "", fallback: 4, min: 1, want: 4},
		{name: "valid parses", set: true, value: "12", fallback: 4, min: 1, want: 12},
		{name: "at minimum honored", set: true, value: "1", fallback: 4, min: 1, want: 1},
		{name: "below minimum falls back", set: true, value: "0", fallback: 4, min: 1, want: 4},
		{name: "negative falls back", set: true, value: "-5", fallback: 4, min: 1, want: 4},
		{name: "non-numeric falls back", set: true, value: "x", fallback: 4, min: 1, want: 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.set {
				t.Setenv(key, tt.value)
			} else if err := os.Unsetenv(key); err != nil {
				t.Fatalf("unset %s: %v", key, err)
			}
			if got := getEnvInt(key, tt.fallback, tt.min); got != tt.want {
				t.Fatalf("getEnvInt(%q, %d, %d) = %d, want %d", tt.value, tt.fallback, tt.min, got, tt.want)
			}
		})
	}
}

// TestLoadGatewayResources checks GATEWAY_RESOURCES handling: unset leaves the
// override nil (chart values use the defaults), a valid value is parsed, and an
// invalid value fails startup instead of silently falling back.
func TestLoadGatewayResources(t *testing.T) {
	t.Run("unset", func(t *testing.T) {
		setRequiredEnv(t)
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() error: %v", err)
		}
		if cfg.GatewayResources != nil {
			t.Errorf("GatewayResources = %v, want nil", cfg.GatewayResources)
		}
	})

	t.Run("valid", func(t *testing.T) {
		setRequiredEnv(t)
		t.Setenv("GATEWAY_RESOURCES", `{"requests":{"memory":"1Gi"},"limits":{"memory":"2Gi"}}`)
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() error: %v", err)
		}
		if cfg.GatewayResources == nil {
			t.Fatal("GatewayResources = nil, want parsed value")
		}
		if got := cfg.GatewayResources.Limits.Memory().String(); got != "2Gi" {
			t.Errorf("limits.memory = %s, want 2Gi", got)
		}
	})

	t.Run("invalid", func(t *testing.T) {
		setRequiredEnv(t)
		t.Setenv("GATEWAY_RESOURCES", `{"limits":{"cpu":"1"}}`)
		if _, err := Load(); err == nil || !strings.Contains(err.Error(), "GATEWAY_RESOURCES") {
			t.Fatalf("Load() error = %v, want GATEWAY_RESOURCES error", err)
		}
	})
}

// TestLoadOperatorWorkspaceSettings covers the administrator-owned operator
// settings (OSG-OPWS-02/03): both default to empty and are read from their env
// vars. They are control-plane configuration, never caller input.
func TestLoadOperatorWorkspaceSettings(t *testing.T) {
	t.Run("default empty", func(t *testing.T) {
		setRequiredEnv(t)
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() error: %v", err)
		}
		if cfg.OperatorNamespaceLabelKey != "" {
			t.Errorf("OperatorNamespaceLabelKey = %q, want empty", cfg.OperatorNamespaceLabelKey)
		}
		if cfg.OperatorSandboxRuntimeClass != "" {
			t.Errorf("OperatorSandboxRuntimeClass = %q, want empty", cfg.OperatorSandboxRuntimeClass)
		}
	})

	t.Run("set from env", func(t *testing.T) {
		setRequiredEnv(t)
		t.Setenv("GATEWAY_OPERATOR_NAMESPACE_LABEL_KEY", "oce.example.com/tenant")
		t.Setenv("GATEWAY_OPERATOR_SANDBOX_RUNTIME_CLASS", "kata")
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() error: %v", err)
		}
		if cfg.OperatorNamespaceLabelKey != "oce.example.com/tenant" {
			t.Errorf("OperatorNamespaceLabelKey = %q, want %q", cfg.OperatorNamespaceLabelKey, "oce.example.com/tenant")
		}
		if cfg.OperatorSandboxRuntimeClass != "kata" {
			t.Errorf("OperatorSandboxRuntimeClass = %q, want %q", cfg.OperatorSandboxRuntimeClass, "kata")
		}
	})
}

// TestLoadCITokenExchangeMode covers PLAT-CITR-08: the token-exchange authorization
// mode is config-driven. It defaults to fgap-v1 (the live fleet mode), accepts the
// two valid modes, and rejects an unknown value rather than silently falling back.
func TestLoadCITokenExchangeMode(t *testing.T) {
	t.Run("defaults to fgap-v1", func(t *testing.T) {
		setRequiredEnv(t)
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() error: %v", err)
		}
		if cfg.CITokenExchangeMode != CITokenExchangeModeFGAPv1 {
			t.Errorf("CITokenExchangeMode = %q, want the default %q", cfg.CITokenExchangeMode, CITokenExchangeModeFGAPv1)
		}
	})

	for _, mode := range []string{CITokenExchangeModeFGAPv1, CITokenExchangeModeV2Attribute} {
		t.Run("accepts "+mode, func(t *testing.T) {
			setRequiredEnv(t)
			t.Setenv("HYPERSHELL_CI_TOKEN_EXCHANGE_MODE", mode)
			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load() error: %v", err)
			}
			if cfg.CITokenExchangeMode != mode {
				t.Errorf("CITokenExchangeMode = %q, want %q", cfg.CITokenExchangeMode, mode)
			}
		})
	}

	t.Run("rejects unknown", func(t *testing.T) {
		setRequiredEnv(t)
		t.Setenv("HYPERSHELL_CI_TOKEN_EXCHANGE_MODE", "v3-magic")
		if _, err := Load(); err == nil || !strings.Contains(err.Error(), "HYPERSHELL_CI_TOKEN_EXCHANGE_MODE") {
			t.Fatalf("Load() error = %v, want HYPERSHELL_CI_TOKEN_EXCHANGE_MODE error", err)
		}
	})
}
