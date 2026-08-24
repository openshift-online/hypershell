package config

import (
	"os"
	"testing"
)

// TestLoadDatabaseProvider covers the DATABASE_PROVIDER startup contract:
// unset/empty and "deployment" both resolve to DatabaseProviderDeployment,
// "cnpg" resolves to DatabaseProviderCNPG, and any other value is a startup
// configuration error rather than a silent fallback to CNPG.
func TestLoadDatabaseProvider(t *testing.T) {
	t.Setenv("HYPERSHELL_GRPC_SERVER_ADDR", "localhost:9000")

	cases := []struct {
		name       string
		envValue   string
		envUnset   bool
		wantErr    bool
		wantResult string
	}{
		{name: "unset defaults to deployment", envUnset: true, wantResult: DatabaseProviderDeployment},
		{name: "empty defaults to deployment", envValue: "", wantResult: DatabaseProviderDeployment},
		{name: "deployment stays deployment", envValue: "deployment", wantResult: DatabaseProviderDeployment},
		{name: "cnpg selects cnpg", envValue: "cnpg", wantResult: DatabaseProviderCNPG},
		{name: "unsupported value is an error", envValue: "bogus", wantErr: true},
		{name: "case-sensitive: CNPG is an error, not silently cnpg", envValue: "CNPG", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.envUnset {
				t.Setenv("DATABASE_PROVIDER", "")
				if err := os.Unsetenv("DATABASE_PROVIDER"); err != nil {
					t.Fatalf("unset DATABASE_PROVIDER: %v", err)
				}
			} else {
				t.Setenv("DATABASE_PROVIDER", tc.envValue)
			}

			cfg, err := Load()
			if tc.wantErr {
				if err == nil {
					t.Fatalf("Load() with DATABASE_PROVIDER=%q: want error, got nil (provider=%q)", tc.envValue, cfg.DatabaseProvider)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load() with DATABASE_PROVIDER=%q: unexpected error: %v", tc.envValue, err)
			}
			if cfg.DatabaseProvider != tc.wantResult {
				t.Fatalf("Load() with DATABASE_PROVIDER=%q: DatabaseProvider = %q, want %q", tc.envValue, cfg.DatabaseProvider, tc.wantResult)
			}
		})
	}
}

func TestResolveDatabaseProvider(t *testing.T) {
	tests := []struct {
		raw     string
		want    string
		wantErr bool
	}{
		{raw: "", want: DatabaseProviderDeployment},
		{raw: "deployment", want: DatabaseProviderDeployment},
		{raw: "cnpg", want: DatabaseProviderCNPG},
		{raw: "Deployment", wantErr: true},
		{raw: "postgres", wantErr: true},
		{raw: " cnpg", wantErr: true},
	}
	for _, tt := range tests {
		got, err := resolveDatabaseProvider(tt.raw)
		if tt.wantErr {
			if err == nil {
				t.Errorf("resolveDatabaseProvider(%q): want error, got result %q", tt.raw, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("resolveDatabaseProvider(%q): unexpected error: %v", tt.raw, err)
			continue
		}
		if got != tt.want {
			t.Errorf("resolveDatabaseProvider(%q) = %q, want %q", tt.raw, got, tt.want)
		}
	}
}
