package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"

	"github.com/openshift-online/hypershell/components/cli/pkg/config"
)

func subcommand(parent *cobra.Command, name string) *cobra.Command {
	for _, c := range parent.Commands() {
		if c.Name() == name {
			return c
		}
	}
	return nil
}

// main.go and the registration files wire hand-maintained commands into the
// generated command tree; the generator never rewrites them, so guard them.
func TestCommandTree(t *testing.T) {
	for _, path := range [][]string{
		{"tui"}, {"login"}, {"logout"}, {"whoami"}, {"apply"}, {"revoke", "serviceAccount"},
		{"create", "gateway"}, {"get", "gateway"}, {"list", "gateways"},
		{"update", "gateway"}, {"update", "managedCluster"},
		{"delete", "gateway"}, {"delete", "managedCluster"}, {"delete", "roleBinding"}, {"delete", "sandboxTemplate"},
		// Hand-maintained service account commands live beside the generated ones.
		{"create", "serviceAccount"}, {"get", "serviceAccount"}, {"list", "serviceAccounts"}, {"delete", "serviceAccount"},
	} {
		cmd := root
		for _, name := range path {
			if cmd = subcommand(cmd, name); cmd == nil {
				t.Errorf("command %v is not registered", path)
				break
			}
		}
	}
}

// HyperShell's Keycloak client is hypershell-cli; scripts/generate-cli.sh passes
// it to the generator as the login --client-id default.
func TestLoginDefaultsToTheHyperShellOIDCClient(t *testing.T) {
	login := subcommand(root, "login")
	if login == nil {
		t.Fatal("login is not registered")
	}
	flag := login.Flags().Lookup("client-id")
	if flag == nil {
		t.Fatal("login has no --client-id flag")
	}
	if flag.DefValue != "hypershell-cli" || flag.Value.String() != "hypershell-cli" {
		t.Errorf("--client-id default = %q (value %q), want hypershell-cli", flag.DefValue, flag.Value.String())
	}
}

// A login saved by an earlier hsctl lives in the HyperShell config file and uses
// the same JSON keys as the generated config, so it must keep working without
// logging in again. scripts/generate-cli.sh passes --config-name hypershell so
// the generated config looks there.
func TestLoginSavedByAnEarlierCLIStillLoads(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("HYPERSHELL_CONFIG", path)
	saved := `{"access_token":"a","refresh_token":"r","issuer_url":"https://issuer.example.com","client_id":"hypershell-cli","url":"https://api.example.com","insecure":true}`
	if err := os.WriteFile(path, []byte(saved), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AccessToken != "a" || cfg.RefreshToken != "r" || cfg.IssuerURL != "https://issuer.example.com" ||
		cfg.ClientID != "hypershell-cli" || cfg.URL != "https://api.example.com" || !cfg.Insecure {
		t.Errorf("earlier session not read by the generated config: %+v", cfg)
	}
	if !cfg.CanRefresh() {
		t.Error("earlier session cannot refresh")
	}
}

// With no $HOME, commands that never touch the config (version, --help) must
// still start; only a command that needs the config reports the problem.
func TestConfigLocationErrorsOnlyWhenUsed(t *testing.T) {
	t.Setenv("HYPERSHELL_CONFIG", "")
	t.Setenv("HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	if _, err := config.Load(); err == nil {
		t.Error("Load() succeeded with no resolvable config location, want an error")
	}
}
